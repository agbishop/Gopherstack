package kms

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// isValidGrantOperation reports whether op is a grant operation permitted by AWS KMS.
func isValidGrantOperation(op string) bool {
	switch op {
	case "Decrypt", "Encrypt", "GenerateDataKey", "GenerateDataKeyWithoutPlaintext",
		"ReEncryptFrom", "ReEncryptTo", "Sign", "Verify", "GetPublicKey",
		"CreateGrant", "RetireGrant", "DescribeKey", "GenerateMac", "VerifyMac",
		"DeriveSharedSecret", "GenerateDataKeyPair", "GenerateDataKeyPairWithoutPlaintext":
		return true
	}

	return false
}

// validateGrantPrincipals enforces CreateGrant's principal-selection rules,
// matching the real aws-sdk-go-v2/service/kms@v1.55.4 CreateGrantInput doc
// comments: exactly one of GranteePrincipal/GranteeServicePrincipal must be
// set; RetiringPrincipal and RetiringServicePrincipal are mutually exclusive;
// and specifying a GranteeServicePrincipal additionally requires a SourceArn
// grant constraint plus a retiring principal of either kind. No AWS-service
// simulation exists in this mock (no IAM/authorization layer at all), so
// these fields are validated and round-tripped for wire parity only -- see
// their doc comments on Grant/CreateGrantInput in models.go.
func validateGrantPrincipals(input *CreateGrantInput) error {
	granteeSet := strings.TrimSpace(input.GranteePrincipal) != ""
	granteeServiceSet := strings.TrimSpace(input.GranteeServicePrincipal) != ""

	if granteeSet == granteeServiceSet {
		return fmt.Errorf(
			"%w: exactly one of GranteePrincipal or GranteeServicePrincipal must be specified",
			ErrValidation,
		)
	}

	retiringSet := strings.TrimSpace(input.RetiringPrincipal) != ""
	retiringServiceSet := strings.TrimSpace(input.RetiringServicePrincipal) != ""

	if retiringSet && retiringServiceSet {
		return fmt.Errorf(
			"%w: specify either RetiringPrincipal or RetiringServicePrincipal, not both",
			ErrValidation,
		)
	}

	if !granteeServiceSet {
		return nil
	}

	if input.Constraints == nil || input.Constraints.SourceArn == "" {
		return fmt.Errorf(
			"%w: GranteeServicePrincipal requires a SourceArn grant constraint",
			ErrValidation,
		)
	}

	if !retiringSet && !retiringServiceSet {
		return fmt.Errorf(
			"%w: GranteeServicePrincipal requires RetiringPrincipal or RetiringServicePrincipal",
			ErrValidation,
		)
	}

	return nil
}

// CreateGrant creates a new grant on the specified key.
func (b *InMemoryBackend) CreateGrant(
	ctx context.Context,
	input *CreateGrantInput,
) (*CreateGrantOutput, error) {
	if err := validateGrantPrincipals(input); err != nil {
		return nil, err
	}

	if len(input.Operations) == 0 {
		return nil, fmt.Errorf("%w: Operations must contain at least one entry", ErrValidation)
	}

	// LimitExceededException's doc (kms@v1.55.4 types/errors.go): "a length
	// constraint or quota was exceeded" -- CreateGrant declares it, and this is
	// a length constraint (gopherstack-i4q8).
	if len(input.Name) > maxGrantNameLength {
		return nil, fmt.Errorf(
			"%w: grant name must not exceed %d characters, got %d",
			ErrLimitExceeded, maxGrantNameLength, len(input.Name),
		)
	}

	for _, op := range input.Operations {
		if !isValidGrantOperation(op) {
			return nil, fmt.Errorf(
				"%w: invalid grant operation %q; must be one of the allowed KMS grant operations",
				ErrValidation, op,
			)
		}
	}

	b.mu.Lock("CreateGrant")
	defer b.mu.Unlock()

	// Store the grant in the key's own region (ARN-embedded region for an ARN
	// input), so ListGrants/RevokeGrant/RetireGrant addressing the key the same
	// way find it. All four grant ops recognize InvalidArnException for a
	// malformed KeyId ARN (gopherstack-qxaj).
	key, region, err := b.resolveKeyAndRegion(ctx, input.KeyID, ErrInvalidArn)
	if err != nil {
		return nil, err
	}

	keyID := key.KeyID

	if key.KeyState == KeyStatePendingDeletion || key.KeyState == KeyStatePendingImport ||
		key.KeyState == KeyStatePendingReplicaDeletion {
		return nil, keyStateError(key)
	}

	grantsForKey := b.grantsRegion(region)

	if len(grantsForKey.byKey.Get(keyID)) >= maxGrantsPerKey {
		return nil, fmt.Errorf(
			"%w: grant limit of %d exceeded for key %q",
			ErrLimitExceeded,
			maxGrantsPerKey,
			keyID,
		)
	}

	now := time.Now()
	grantID := uuid.New().String()
	grantToken := uuid.New().String()
	grant := &Grant{
		GrantID:                  grantID,
		KeyID:                    keyID,
		GranteePrincipal:         input.GranteePrincipal,
		GranteeServicePrincipal:  input.GranteeServicePrincipal,
		RetiringPrincipal:        input.RetiringPrincipal,
		RetiringServicePrincipal: input.RetiringServicePrincipal,
		Operations:               input.Operations,
		Name:                     input.Name,
		GrantToken:               grantToken,
		TokenIssuedAt:            now,
		Constraints:              input.Constraints,
		CreationDate:             UnixTimeFloat(now),
		IssuingAccount:           b.accountID,
	}
	// A single Put keeps the byToken and byKey indexes consistent automatically.
	grantsForKey.table.Put(grant)

	return &CreateGrantOutput{GrantID: grantID, GrantToken: grantToken}, nil
}

// grantConstraintsSatisfied reports whether the provided encryption context satisfies
// the grant's constraints. A nil Constraints field always passes.
func grantConstraintsSatisfied(c *GrantConstraints, encCtx map[string]string) bool {
	if c == nil {
		return true
	}

	if len(c.EncryptionContextEquals) > 0 {
		if len(encCtx) != len(c.EncryptionContextEquals) {
			return false
		}

		for k, v := range c.EncryptionContextEquals {
			if encCtx[k] != v {
				return false
			}
		}
	}

	for k, v := range c.EncryptionContextSubset {
		if encCtx[k] != v {
			return false
		}
	}

	return true
}

// findGrantByToken returns the first grant whose GrantToken matches any of the provided tokens.
// Searches all regions. Must be called with at least a read lock held.
func (b *InMemoryBackend) findGrantByToken(grantTokens []string) *Grant {
	for _, gs := range b.grants {
		for _, token := range grantTokens {
			if matches := gs.byToken.Get(token); len(matches) > 0 {
				return matches[0]
			}
		}
	}

	return nil
}

// grantPermitsOperation reports whether the grant's Operations list authorizes operation.
func grantPermitsOperation(grant *Grant, operation string) bool {
	return slices.Contains(grant.Operations, operation)
}

// validateGrantTokenConstraints checks that, if a grant token is provided, it authorizes
// operation and the encryption context satisfies the grant's constraints. No-op when
// grantTokens is empty. Must be called with at least a read lock held.
func (b *InMemoryBackend) validateGrantTokenConstraints(
	_ context.Context,
	grantTokens []string,
	operation string,
	encCtx map[string]string,
) error {
	if len(grantTokens) == 0 {
		return nil
	}

	grant := b.findGrantByToken(grantTokens)
	if grant == nil {
		return fmt.Errorf("%w: grant token not found", ErrInvalidGrantToken)
	}

	// AWS grant tokens are valid for approximately 5 minutes after issuance.
	if !grant.TokenIssuedAt.IsZero() && time.Since(grant.TokenIssuedAt) > grantTokenTTL {
		return fmt.Errorf("%w: grant token has expired", ErrInvalidGrantToken)
	}

	if !grantPermitsOperation(grant, operation) {
		return fmt.Errorf(
			"%w: grant %q does not permit operation %q",
			ErrAccessDenied, grant.GrantID, operation,
		)
	}

	if !grantConstraintsSatisfied(grant.Constraints, encCtx) {
		return fmt.Errorf(
			"%w: encryption context does not satisfy grant constraints",
			ErrKeyInvalidState,
		)
	}

	return nil
}

// validateGrantTokenPresence checks that, if grant tokens are provided, at least one
// resolves to an existing, non-expired grant that authorizes operation. Unlike
// validateGrantTokenConstraints, it does not evaluate EncryptionContext-based grant
// constraints: per AWS KMS docs, EncryptionContextEquals/EncryptionContextSubset
// constraints apply only to operations that support an encryption context. Sign, Verify,
// GetPublicKey, GenerateMac, VerifyMac, and DeriveSharedSecret do not, so only grant-token
// validity (existence + TTL + Operations) is checked. Must be called with at least a read
// lock held.
func (b *InMemoryBackend) validateGrantTokenPresence(grantTokens []string, operation string) error {
	if len(grantTokens) == 0 {
		return nil
	}

	grant := b.findGrantByToken(grantTokens)
	if grant == nil {
		return fmt.Errorf("%w: grant token not found", ErrInvalidGrantToken)
	}

	if !grant.TokenIssuedAt.IsZero() && time.Since(grant.TokenIssuedAt) > grantTokenTTL {
		return fmt.Errorf("%w: grant token has expired", ErrInvalidGrantToken)
	}

	if !grantPermitsOperation(grant, operation) {
		return fmt.Errorf(
			"%w: grant %q does not permit operation %q",
			ErrAccessDenied, grant.GrantID, operation,
		)
	}

	return nil
}

// ListGrants returns the grants for a specified key with optional pagination and GrantId filter.
func (b *InMemoryBackend) ListGrants(
	ctx context.Context,
	input *ListGrantsInput,
) (*ListGrantsOutput, error) {
	b.mu.RLock("ListGrants")
	defer b.mu.RUnlock()

	// Read grants from the key's own region (ARN-embedded region for an ARN input),
	// matching where CreateGrant stored them.
	key, region, err := b.resolveKeyAndRegion(ctx, input.KeyID, ErrInvalidArn)
	if err != nil {
		return nil, err
	}

	keyID := key.KeyID

	var stored []*Grant
	for _, g := range b.grantsRegion(region).byKey.Get(keyID) {
		// Filter by GrantId if specified.
		if input.GrantID != "" && g.GrantID != input.GrantID {
			continue
		}
		stored = append(stored, g)
	}

	sort.Slice(stored, func(i, j int) bool { return stored[i].GrantID < stored[j].GrantID })

	startIdx := parseMarker(input.Marker)
	limit := int32(default50ListLimit)

	if input.Limit != nil && *input.Limit > 0 {
		limit = *input.Limit
	}

	grants := make([]GrantListEntry, len(stored))
	for i, g := range stored {
		grants[i] = toGrantListEntry(g)
	}

	if startIdx >= len(grants) {
		return &ListGrantsOutput{Grants: []GrantListEntry{}}, nil
	}

	end := startIdx + int(limit)

	var nextMarker string
	if end < len(grants) {
		nextMarker = strconv.Itoa(end)
	} else {
		end = len(grants)
	}

	return &ListGrantsOutput{
		Grants:     grants[startIdx:end],
		NextMarker: nextMarker,
		Truncated:  nextMarker != "",
	}, nil
}

// RevokeGrant revokes a grant by ID.
func (b *InMemoryBackend) RevokeGrant(ctx context.Context, input *RevokeGrantInput) error {
	b.mu.Lock("RevokeGrant")
	defer b.mu.Unlock()

	// Resolve against the key's own region (ARN-embedded region for an ARN input)
	// so the grant is found in the store CreateGrant wrote it to.
	key, region, err := b.resolveKeyAndRegion(ctx, input.KeyID, ErrInvalidArn)
	if err != nil {
		return err
	}

	grant, ok := b.grantsStore(region).Get(input.GrantID)
	if !ok || grant.KeyID != key.KeyID {
		return ErrGrantNotFound
	}

	// A single Delete keeps the byToken and byKey indexes consistent
	// automatically (including dropping now-empty index groups on purge).
	b.grantsStore(region).Delete(input.GrantID)

	return nil
}

// RetireGrant retires a grant by grant token or grant ID + key ID.
func (b *InMemoryBackend) RetireGrant(ctx context.Context, input *RetireGrantInput) error {
	b.mu.Lock("RetireGrant")
	defer b.mu.Unlock()

	if input.GrantToken != "" {
		// Search all regions for the grant token.
		for _, gs := range b.grants {
			if matches := gs.byToken.Get(input.GrantToken); len(matches) > 0 {
				g := matches[0]
				gs.table.Delete(g.GrantID)

				return nil
			}
		}

		return ErrGrantNotFound
	}

	if input.GrantID == "" {
		return ErrGrantNotFound
	}

	// When a KeyId is supplied, resolve it to the key's own region (ARN-embedded
	// region for an ARN input) and retire the grant from that region's store, so a
	// grant created via a cross-region ARN is retired consistently. When no KeyId
	// is supplied there is no region hint, so search every region for the grant ID.
	if input.KeyID != "" {
		key, region, err := b.resolveKeyAndRegion(ctx, input.KeyID, ErrInvalidArn)
		if err != nil {
			return err
		}

		grant, ok := b.grantsStore(region).Get(input.GrantID)
		if !ok || grant.KeyID != key.KeyID {
			return ErrGrantNotFound
		}

		b.grantsStore(region).Delete(input.GrantID)

		return nil
	}

	for _, gs := range b.grants {
		if _, ok := gs.table.Get(input.GrantID); ok {
			gs.table.Delete(input.GrantID)

			return nil
		}
	}

	return ErrGrantNotFound
}

// ListRetirableGrants returns all grants for which the given principal is the retiring principal.
// ListRetirableGrantsInput's doc requires exactly one of RetiringPrincipal/
// RetiringServicePrincipal (kms@v1.55.4 api_op_ListRetirableGrants.go: "You
// must specify either RetiringPrincipal or RetiringServicePrincipal, but not
// both.").
func (b *InMemoryBackend) ListRetirableGrants(
	ctx context.Context,
	input *ListRetirableGrantsInput,
) (*ListGrantsOutput, error) {
	if (input.RetiringPrincipal == "") == (input.RetiringServicePrincipal == "") {
		return nil, fmt.Errorf(
			"%w: you must specify either RetiringPrincipal or RetiringServicePrincipal, but not both",
			ErrValidation,
		)
	}

	b.mu.RLock("ListRetirableGrants")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.defaultRegion)

	stored := make([]*Grant, 0)
	for _, g := range b.grantsStore(region).All() {
		matchesPrincipal := input.RetiringPrincipal != "" && g.RetiringPrincipal == input.RetiringPrincipal
		matchesServicePrincipal := input.RetiringServicePrincipal != "" &&
			g.RetiringServicePrincipal == input.RetiringServicePrincipal

		if matchesPrincipal || matchesServicePrincipal {
			stored = append(stored, g)
		}
	}

	sort.Slice(stored, func(i, j int) bool { return stored[i].GrantID < stored[j].GrantID })

	startIdx := parseMarker(input.Marker)
	limit := int32(default50ListLimit)

	if input.Limit != nil && *input.Limit > 0 {
		limit = *input.Limit
	}

	grants := make([]GrantListEntry, len(stored))
	for i, g := range stored {
		grants[i] = toGrantListEntry(g)
	}

	if startIdx >= len(grants) {
		return &ListGrantsOutput{Grants: []GrantListEntry{}}, nil
	}

	end := startIdx + int(limit)

	var nextMarker string
	if end < len(grants) {
		nextMarker = strconv.Itoa(end)
	} else {
		end = len(grants)
	}

	return &ListGrantsOutput{
		Grants:     grants[startIdx:end],
		NextMarker: nextMarker,
		Truncated:  nextMarker != "",
	}, nil
}

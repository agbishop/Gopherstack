package acm

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/page"
	svcTags "github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// iamRoleArnPattern mirrors the real API's documented RoleArn constraint:
// arn:aws[a-z-]*:iam::[0-9]{12}:role/.+ -- narrowed to the same permissive
// partition class certArnPattern/acmeEndpointArnPattern already use.
var iamRoleArnPattern = regexp.MustCompile(`^arn:[\w+=/,.@-]+:iam::[0-9]{12}:role/.+$`)

// AcmeExternalAccountBinding (EAB) authorizes an ACME client to register an
// account with an endpoint, scoped to a single IAM role. Owned by exactly
// one AcmeEndpoint (AcmeEndpointArn), cascade-deleted with it -- see
// DeleteAcmeEndpoint in acme_endpoints.go.
type AcmeExternalAccountBinding struct {
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
	ExpiresAt       *time.Time `json:"expiresAt,omitempty"`
	LastUsedAt      *time.Time `json:"lastUsedAt,omitempty"`
	RevokedAt       *time.Time `json:"revokedAt,omitempty"`
	ARN             string     `json:"arn"`
	Region          string     `json:"region"`
	AcmeEndpointArn string     `json:"acmeEndpointArn"`
	RoleArn         string     `json:"roleArn"`
	// KeyID/MacKey are the synthetic ACME EAB credentials returned by
	// GetAcmeExternalAccountBindingCredentials. Real cryptographic ACME
	// protocol participation is out of scope (see CLAUDE.md parity
	// principles); these are deterministic, per-EAB random values -- not
	// values ACM ever validates a real ACME client's request against, so
	// nothing downstream treats them as cryptographically meaningful.
	KeyID            string `json:"keyId"`
	MacKey           string `json:"macKey"`
	IdempotencyToken string `json:"idempotencyToken,omitempty"`
}

func acmeEABKeyFn(v *AcmeExternalAccountBinding) string         { return v.ARN }
func acmeEABRegionIndexFn(v *AcmeExternalAccountBinding) string { return v.Region }
func acmeEABEndpointIndexFn(v *AcmeExternalAccountBinding) string {
	return v.AcmeEndpointArn
}

func copyAcmeEAB(v *AcmeExternalAccountBinding) AcmeExternalAccountBinding {
	cp := *v
	if v.ExpiresAt != nil {
		t := *v.ExpiresAt
		cp.ExpiresAt = &t
	}

	if v.LastUsedAt != nil {
		t := *v.LastUsedAt
		cp.LastUsedAt = &t
	}

	if v.RevokedAt != nil {
		t := *v.RevokedAt
		cp.RevokedAt = &t
	}

	return cp
}

// CreateAcmeEABParams holds the parsed CreateAcmeExternalAccountBinding input.
type CreateAcmeEABParams struct {
	AcmeEndpointArn  string
	RoleArn          string
	IdempotencyToken string
	ExpirationType   string
	Tags             []svcTags.KV
	ExpirationValue  int64
}

func (p CreateAcmeEABParams) fingerprint() string {
	return p.AcmeEndpointArn + "|" + p.RoleArn + "|" + p.ExpirationType + fmt.Sprintf("|%d", p.ExpirationValue)
}

// expirationDuration converts an Expiration{Type,Value} pair to a
// time.Duration. timeType must already be validated by the caller.
func expirationDuration(timeType string, value int64) time.Duration {
	switch timeType {
	case timeTypeMinutes:
		return time.Duration(value) * time.Minute
	case timeTypeHours:
		return time.Duration(value) * time.Hour
	case timeTypeDays:
		return time.Duration(value) * 24 * time.Hour
	default:
		return 0
	}
}

// validateCreateAcmeEABParams checks the CreateAcmeExternalAccountBinding
// input, factored out of CreateAcmeExternalAccountBinding to keep that
// method's cyclomatic complexity down.
func validateCreateAcmeEABParams(p CreateAcmeEABParams) error {
	if err := validateAcmeEndpointArn(p.AcmeEndpointArn); err != nil {
		return err
	}

	if p.AcmeEndpointArn == "" {
		return fmt.Errorf("%w: AcmeEndpointArn is required", ErrInvalidParameter)
	}

	if p.RoleArn == "" || !iamRoleArnPattern.MatchString(p.RoleArn) {
		return fmt.Errorf("%w: RoleArn %q is not a valid IAM role ARN", ErrInvalidParameter, p.RoleArn)
	}

	if err := validateExpiration(p.ExpirationType, p.ExpirationValue); err != nil {
		return err
	}

	if len(p.Tags) > maxTagsPerResource {
		return fmt.Errorf("%w: maximum of 50 Tags allowed", ErrInvalidParameter)
	}

	return nil
}

// validateExpiration checks an Expiration{Type,Value} pair; an empty
// timeType means "no expiration configured", which is always valid.
func validateExpiration(timeType string, value int64) error {
	if timeType == "" {
		return nil
	}

	if timeType != timeTypeMinutes && timeType != timeTypeHours && timeType != timeTypeDays {
		return fmt.Errorf("%w: invalid Expiration.Type %q", ErrInvalidParameter, timeType)
	}

	if value <= 0 {
		return fmt.Errorf("%w: Expiration.Value must be positive", ErrInvalidParameter)
	}

	return nil
}

// CreateAcmeExternalAccountBinding creates a new EAB under an existing
// endpoint.
func (b *InMemoryBackend) CreateAcmeExternalAccountBinding(
	ctx context.Context, p CreateAcmeEABParams,
) (*AcmeExternalAccountBinding, error) {
	if err := validateCreateAcmeEABParams(p); err != nil {
		return nil, err
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("CreateAcmeExternalAccountBinding")
	defer b.mu.Unlock()

	ep, ok := b.endpoints.Get(p.AcmeEndpointArn)
	if !ok || ep.Region != region {
		return nil, fmt.Errorf("%w: ACME endpoint %s not found", ErrAcmeResourceNotFound, p.AcmeEndpointArn)
	}

	if existingARN, found, err := checkAcmeIdempotency(
		b.eabIdempotencyStore(region), p.IdempotencyToken, p.fingerprint(),
	); err != nil {
		return nil, err
	} else if found {
		if eab, exists := b.eabs.Get(existingARN); exists {
			cp := copyAcmeEAB(eab)

			return &cp, nil
		}
	}

	id := fmt.Sprintf("%x", time.Now().UnixNano())
	eabARN := arn.Build(
		"acm", region, b.accountID,
		acmeResourceTypeEndpoint+"/"+endpointIDFromArn(p.AcmeEndpointArn)+"/"+acmeResourceTypeEAB+"/"+id,
	)
	now := time.Now().UTC()

	keyID, macKey, err := generateEABCredentials()
	if err != nil {
		return nil, fmt.Errorf("failed to generate EAB credentials: %w", err)
	}

	eab := &AcmeExternalAccountBinding{
		ARN:              eabARN,
		Region:           region,
		AcmeEndpointArn:  p.AcmeEndpointArn,
		RoleArn:          p.RoleArn,
		KeyID:            keyID,
		MacKey:           macKey,
		CreatedAt:        now,
		UpdatedAt:        now,
		IdempotencyToken: p.IdempotencyToken,
	}

	if p.ExpirationType != "" {
		exp := now.Add(expirationDuration(p.ExpirationType, p.ExpirationValue))
		eab.ExpiresAt = &exp
	}

	b.eabs.Put(eab)

	if p.IdempotencyToken != "" {
		b.eabIdempotencyStore(region)[p.IdempotencyToken] = acmeIdempotencyEntry{
			ARN: eabARN, Fingerprint: p.fingerprint(), CreatedAt: now,
		}
	}

	cp := copyAcmeEAB(eab)

	return &cp, nil
}

// endpointIDFromArn extracts the trailing "acme-endpoint/<id>" segment's id
// from an endpoint ARN, for building nested EAB/domain-validation ARNs. The
// caller has already validated epARN's shape.
func endpointIDFromArn(epARN string) string {
	idx := strings.LastIndex(epARN, "/")
	if idx < 0 {
		return epARN
	}

	return epARN[idx+1:]
}

// generateEABCredentials returns a deterministic-shape (but randomly
// generated) synthetic KeyId/MacKey pair. Not cryptographically meaningful --
// see AcmeExternalAccountBinding's KeyID/MacKey doc comment.
func generateEABCredentials() (string, string, error) {
	const keyIDBytes = 8

	const macKeyBytes = 32

	kb := make([]byte, keyIDBytes)
	if _, err := cryptorand.Read(kb); err != nil {
		return "", "", err
	}

	mb := make([]byte, macKeyBytes)
	if _, err := cryptorand.Read(mb); err != nil {
		return "", "", err
	}

	return base64.RawURLEncoding.EncodeToString(kb), base64.RawURLEncoding.EncodeToString(mb), nil
}

// DescribeAcmeExternalAccountBinding returns the EAB with the given ARN.
func (b *InMemoryBackend) DescribeAcmeExternalAccountBinding(
	ctx context.Context, eabARN string,
) (*AcmeExternalAccountBinding, error) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("DescribeAcmeExternalAccountBinding")
	defer b.mu.RUnlock()

	eab, ok := b.eabs.Get(eabARN)
	if !ok || eab.Region != region {
		return nil, fmt.Errorf("%w: ACME external account binding %s not found", ErrAcmeResourceNotFound, eabARN)
	}

	cp := copyAcmeEAB(eab)

	return &cp, nil
}

// ListAcmeExternalAccountBindings returns a paginated list of EABs owned by
// epARN.
func (b *InMemoryBackend) ListAcmeExternalAccountBindings(
	ctx context.Context, epARN, nextToken string, maxResults int,
) (page.Page[AcmeExternalAccountBinding], error) {
	return listOwnedByEndpoint(
		ctx, b, "ListAcmeExternalAccountBindings", epARN, nextToken, maxResults,
		b.eabsByEndpoint, copyAcmeEAB, func(v AcmeExternalAccountBinding) string { return v.ARN },
	)
}

// GetAcmeExternalAccountBindingCredentials returns the KeyId/MacKey for an
// active (non-revoked) EAB. Its deserializer declares neither
// InvalidStateException nor ConflictException, only ValidationException --
// a revoked EAB is ErrInvalidParameter, not ErrInvalidState --
// gopherstack-ftkd.
func (b *InMemoryBackend) GetAcmeExternalAccountBindingCredentials(
	ctx context.Context, eabARN string,
) (string, string, error) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("GetAcmeExternalAccountBindingCredentials")
	defer b.mu.RUnlock()

	eab, ok := b.eabs.Get(eabARN)
	if !ok || eab.Region != region {
		return "", "", fmt.Errorf("%w: ACME external account binding %s not found", ErrAcmeResourceNotFound, eabARN)
	}

	if eab.RevokedAt != nil {
		return "", "", fmt.Errorf("%w: ACME external account binding %s is revoked", ErrInvalidParameter, eabARN)
	}

	return eab.KeyID, eab.MacKey, nil
}

// RevokeAcmeExternalAccountBinding marks an EAB revoked.
// RevokeAcmeExternalAccountBinding's deserializer declares
// ConflictException, not InvalidStateException, for an already-revoked EAB
// -- gopherstack-ftkd.
func (b *InMemoryBackend) RevokeAcmeExternalAccountBinding(ctx context.Context, eabARN string) error {
	region := getRegion(ctx, b.region)

	b.mu.Lock("RevokeAcmeExternalAccountBinding")
	defer b.mu.Unlock()

	eab, ok := b.eabs.Get(eabARN)
	if !ok || eab.Region != region {
		return fmt.Errorf("%w: ACME external account binding %s not found", ErrAcmeResourceNotFound, eabARN)
	}

	if eab.RevokedAt != nil {
		return fmt.Errorf("%w: ACME external account binding %s is already revoked", ErrConflict, eabARN)
	}

	now := time.Now().UTC()
	eab.RevokedAt = &now
	eab.UpdatedAt = now

	return nil
}

// DeleteAcmeExternalAccountBinding removes the EAB with the given ARN.
// Unlike Describe/List/Revoke, Delete's deserializer declares no
// ResourceNotFoundException -- a missing ARN is ErrInvalidParameter
// (ValidationException), not ErrAcmeResourceNotFound -- gopherstack-ftkd.
func (b *InMemoryBackend) DeleteAcmeExternalAccountBinding(ctx context.Context, eabARN string) error {
	region := getRegion(ctx, b.region)

	b.mu.Lock("DeleteAcmeExternalAccountBinding")
	defer b.mu.Unlock()

	eab, ok := b.eabs.Get(eabARN)
	if !ok || eab.Region != region {
		return fmt.Errorf("%w: ACME external account binding %s not found", ErrInvalidParameter, eabARN)
	}

	b.eabs.Delete(eabARN)

	for tok, entry := range b.eabIdempotencyStore(region) {
		if entry.ARN == eabARN {
			delete(b.eabIdempotency[region], tok)
		}
	}

	return nil
}

// EABExists reports whether an EAB with the given ARN exists in the request
// region. Used by generic resource tagging (handler_resource_tags.go).
func (b *InMemoryBackend) EABExists(ctx context.Context, eabARN string) bool {
	region := getRegion(ctx, b.region)

	b.mu.RLock("EABExists")
	defer b.mu.RUnlock()

	eab, ok := b.eabs.Get(eabARN)

	return ok && eab.Region == region
}

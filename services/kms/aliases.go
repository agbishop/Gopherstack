package kms

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	gopherarn "github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// isValidAliasName reports whether an alias name uses only the characters allowed by AWS KMS.
// AWS permits letters, digits, colons, forward slashes, underscores, and hyphens.
// The caller is responsible for ensuring the name starts with "alias/" before calling this.
func isValidAliasName(name string) bool {
	for _, ch := range name {
		if (ch < 'a' || ch > 'z') && (ch < 'A' || ch > 'Z') &&
			(ch < '0' || ch > '9') && ch != ':' && ch != '/' && ch != '_' && ch != '-' {
			return false
		}
	}

	return true
}

// CreateAlias creates an alias pointing to a key.
func (b *InMemoryBackend) CreateAlias(ctx context.Context, input *CreateAliasInput) error {
	if !strings.HasPrefix(input.AliasName, "alias/") {
		return fmt.Errorf("%w: alias name must start with alias/", ErrInvalidAliasName)
	}

	if strings.HasPrefix(input.AliasName, "alias/aws/") {
		return fmt.Errorf(
			"%w: alias names that begin with alias/aws/ are reserved for AWS managed keys",
			ErrInvalidAliasName,
		)
	}

	if len(input.AliasName) > maxAliasNameLength {
		return fmt.Errorf(
			"%w: alias name exceeds maximum length of %d characters",
			ErrInvalidAliasName, maxAliasNameLength,
		)
	}

	if !isValidAliasName(input.AliasName) {
		return fmt.Errorf(
			"%w: alias name %q contains invalid characters; "+
				"allowed: letters, numbers, colons, forward slashes, underscores, and hyphens",
			ErrInvalidAliasName, input.AliasName,
		)
	}

	b.mu.Lock("CreateAlias")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.defaultRegion)

	if b.aliasesStore(region).Has(input.AliasName) {
		return ErrAliasAlreadyExists
	}

	// CreateAlias's deserializeOpError does not recognize InvalidArnException
	// (gopherstack-qxaj) -- a malformed TargetKeyID ARN falls back to NotFoundException.
	targetID, _, err := b.resolveKeyID(ctx, input.TargetKeyID, ErrKeyNotFound)
	if err != nil {
		return err
	}

	if !b.keysStore(region).Has(targetID) {
		return ErrKeyNotFound
	}

	now := UnixTimeFloat(time.Now())
	aliasArn := gopherarn.Build("kms", region, b.accountID, input.AliasName)
	b.aliasesStore(region).Put(&Alias{
		AliasName:       input.AliasName,
		AliasArn:        aliasArn,
		TargetKeyID:     targetID,
		CreationDate:    now,
		LastUpdatedDate: now,
	})

	return nil
}

// UpdateAlias redirects an existing alias to a different key.
// The alias must already exist; the target key must exist and not be in PendingDeletion state.
func (b *InMemoryBackend) UpdateAlias(ctx context.Context, input *UpdateAliasInput) error {
	b.mu.Lock("UpdateAlias")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.defaultRegion)

	alias, exists := b.aliasesStore(region).Get(input.AliasName)
	if !exists {
		return ErrAliasNotFound
	}

	// UpdateAlias's deserializeOpError does not recognize InvalidArnException
	// (gopherstack-qxaj) -- a malformed TargetKeyID ARN falls back to NotFoundException.
	targetID, _, err := b.resolveKeyID(ctx, input.TargetKeyID, ErrKeyNotFound)
	if err != nil {
		return err
	}

	targetKey, ok := b.keysStore(region).Get(targetID)
	if !ok {
		return ErrKeyNotFound
	}

	// AWS rejects alias updates pointing to a key in PendingDeletion state.
	if targetKey.KeyState == KeyStatePendingDeletion {
		return fmt.Errorf(
			"%w: cannot update alias to a key in PendingDeletion state (key %q)",
			ErrKeyInvalidState, targetID,
		)
	}

	alias.TargetKeyID = targetID
	alias.LastUpdatedDate = UnixTimeFloat(time.Now())
	b.keyIDResolutionCache.Delete(input.AliasName)
	b.keyIDResolutionCache.Delete(alias.AliasArn)

	return nil
}

// DeleteAlias removes an alias.
// Per AWS KMS behaviour, an alias pointing to a key in PendingDeletion state
// cannot be deleted — the caller must cancel the deletion first.
func (b *InMemoryBackend) DeleteAlias(ctx context.Context, input *DeleteAliasInput) error {
	b.mu.Lock("DeleteAlias")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.defaultRegion)

	alias, exists := b.aliasesStore(region).Get(input.AliasName)
	if !exists {
		return ErrAliasNotFound
	}

	// Prevent deleting an alias that targets a key scheduled for deletion.
	if alias.TargetKeyID != "" {
		if key, ok := b.keysStore(region).Get(alias.TargetKeyID); ok &&
			key.KeyState == KeyStatePendingDeletion {
			return fmt.Errorf(
				"%w: key %s is pending deletion; cancel the deletion before deleting the alias",
				ErrKeyInvalidState, alias.TargetKeyID,
			)
		}
	}

	b.aliasesStore(region).Delete(input.AliasName)
	b.keyIDResolutionCache.Delete(input.AliasName)
	b.keyIDResolutionCache.Delete(alias.AliasArn)

	return nil
}

// ListAliases returns a paginated list of aliases, optionally filtered by key.
func (b *InMemoryBackend) ListAliases(
	ctx context.Context,
	input *ListAliasesInput,
) (*ListAliasesOutput, error) {
	b.mu.RLock("ListAliases")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.defaultRegion)

	var resolvedKeyID string

	if input.KeyID != "" {
		var err error

		// ListAliases's deserializeOpError recognizes InvalidArnException (gopherstack-qxaj).
		resolvedKeyID, _, err = b.resolveKeyID(ctx, input.KeyID, ErrInvalidArn)
		if err != nil {
			return nil, err
		}
	}

	aliases := make([]Alias, 0, b.aliasesStore(region).Len())

	for _, a := range b.aliasesStore(region).All() {
		if resolvedKeyID != "" && a.TargetKeyID != resolvedKeyID {
			continue
		}

		aliases = append(aliases, *a)
	}

	sort.Slice(aliases, func(i, j int) bool {
		return aliases[i].AliasName < aliases[j].AliasName
	})

	startIdx := parseMarker(input.Marker)
	limit := int32(default50ListLimit)

	if input.Limit != nil {
		if *input.Limit < 1 || *input.Limit > 1000 {
			return nil, fmt.Errorf("%w: Limit must be between 1 and 1000", ErrValidation)
		}

		limit = *input.Limit
	}

	if startIdx >= len(aliases) {
		return &ListAliasesOutput{Aliases: []Alias{}}, nil
	}

	end := startIdx + int(limit)

	var nextMarker string

	if end < len(aliases) {
		nextMarker = strconv.Itoa(end)
	} else {
		end = len(aliases)
	}

	return &ListAliasesOutput{
		Aliases:    aliases[startIdx:end],
		NextMarker: nextMarker,
		Truncated:  nextMarker != "",
	}, nil
}

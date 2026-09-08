package kms

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	gopherarn "github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// ReplicateKey creates a multi-region replica for an existing key in the target region.
func (b *InMemoryBackend) ReplicateKey(
	ctx context.Context,
	input *ReplicateKeyInput,
) (*ReplicateKeyOutput, error) {
	if strings.TrimSpace(input.ReplicaRegion) == "" {
		return nil, fmt.Errorf("%w: ReplicaRegion must not be empty", ErrValidation)
	}

	// An inline policy, if supplied, must be a well-formed key policy document
	// (same rule CreateKey applies to its own Policy field).
	if input.Policy != "" && !validKeyPolicyDoc(input.Policy) {
		return nil, ErrMalformedPolicyDocument
	}

	b.mu.Lock("ReplicateKey")
	defer b.mu.Unlock()

	sourceRegion := getRegion(ctx, b.defaultRegion)

	sourceKey, err := b.lookupKeyWrite(ctx, input.KeyID, ErrInvalidArn)
	if err != nil {
		return nil, err
	}

	// Only Enabled keys can be replicated; PendingDeletion / PendingImport / Disabled are rejected.
	if sourceKey.KeyState != KeyStateEnabled {
		return nil, fmt.Errorf(
			"%w: only Enabled keys can be replicated; key %q is in state %s",
			ErrKeyInvalidState, sourceKey.KeyID, sourceKey.KeyState,
		)
	}

	// Only keys created with MultiRegion=true can be replicated.
	if !sourceKey.MultiRegion {
		return nil, fmt.Errorf(
			"%w: only multi-region keys can be replicated; key %q was not created with MultiRegion=true",
			ErrUnsupportedOrigin,
			sourceKey.KeyID,
		)
	}

	newKeyID := uuid.New().String()
	description := sourceKey.Description
	if input.Description != "" {
		description = input.Description
	}

	replicaARN := gopherarn.Build("kms", input.ReplicaRegion, b.accountID, "key/"+newKeyID)
	replica := &Key{
		KeyID:                newKeyID,
		Arn:                  replicaARN,
		Description:          description,
		KeyState:             sourceKey.KeyState,
		KeyUsage:             sourceKey.KeyUsage,
		KeySpec:              sourceKey.KeySpec,
		Origin:               sourceKey.Origin,
		CreationDate:         UnixTimeFloat(time.Now()),
		RotationEnabled:      sourceKey.RotationEnabled,
		RotationPeriodInDays: sourceKey.RotationPeriodInDays,
		MultiRegion:          true,
		PrimaryRegion:        b.keyRegion(sourceKey.Arn),
		Enabled:              sourceKey.Enabled,
	}

	sourceKey.MultiRegion = true
	if sourceKey.PrimaryRegion == "" {
		sourceKey.PrimaryRegion = b.keyRegion(sourceKey.Arn)
	}

	if km := b.keyMaterialsStore(sourceRegion)[sourceKey.KeyID]; km != nil {
		serialized, serErr := marshalKeyMaterial(km)
		if serErr != nil {
			return nil, fmt.Errorf("serializing key material for replication: %w", serErr)
		}

		cloned, cloneErr := unmarshalKeyMaterial(serialized)
		if cloneErr != nil {
			return nil, fmt.Errorf("deserializing replicated key material: %w", cloneErr)
		}

		// Store replica key material in the target region's store.
		b.keyMaterialsStore(input.ReplicaRegion)[replica.KeyID] = cloned
	}

	// Store replica key in the target region's store.
	b.keysStore(input.ReplicaRegion).Put(replica)

	// Persist the caller-supplied policy so GetKeyPolicy on the replica returns
	// it verbatim rather than synthesizing the default -- the key policy is not
	// a shared multi-region property, so the replica does not inherit the
	// source key's policy. Mirrors CreateKey's identical Policy handling; see
	// the CreateKey comment above for why this matters to Terraform's
	// aws_kms_replica_key resource (it polls GetKeyPolicy after apply the same
	// way aws_kms_key does).
	if input.Policy != "" {
		b.policiesStore(input.ReplicaRegion)[replica.KeyID] = input.Policy
	}

	// Record the replica key ID on the source (primary) key so DescribeKey can
	// return the full MultiRegionConfiguration.
	sourceKey.ReplicaKeyIDs = append(sourceKey.ReplicaKeyIDs, replica.KeyID)

	return &ReplicateKeyOutput{ReplicaKeyMetadata: b.keyToMetadata(replica)}, nil
}

// UpdatePrimaryRegion promotes the replica in PrimaryRegion to be the new primary
// and demotes the current primary to a replica. Both keys must be Enabled multi-region keys.
func (b *InMemoryBackend) UpdatePrimaryRegion(
	ctx context.Context,
	input *UpdatePrimaryRegionInput,
) error {
	if strings.TrimSpace(input.PrimaryRegion) == "" {
		return fmt.Errorf("%w: PrimaryRegion must not be empty", ErrValidation)
	}

	b.mu.Lock("UpdatePrimaryRegion")
	defer b.mu.Unlock()

	currentKey, err := b.lookupKeyWrite(ctx, input.KeyID, ErrInvalidArn)
	if err != nil {
		return err
	}

	if !currentKey.MultiRegion {
		return fmt.Errorf(
			"%w: UpdatePrimaryRegion is only valid for multi-region keys; key %q is not multi-region",
			ErrUnsupportedOrigin, currentKey.KeyID,
		)
	}

	currentRegion := extractRegionFromARN(currentKey.Arn)

	if currentRegion == input.PrimaryRegion {
		return nil // already primary in the requested region; no-op
	}

	// Find the replica in the target region.
	var newPrimary *Key
	var newPrimaryID string

	for _, replicaID := range currentKey.ReplicaKeyIDs {
		rk := b.findKeyInAnyRegion(replicaID)
		if rk == nil {
			continue
		}

		if extractRegionFromARN(rk.Arn) == input.PrimaryRegion {
			newPrimary = rk
			newPrimaryID = replicaID

			break
		}
	}

	if newPrimary == nil {
		return fmt.Errorf(
			"%w: no replica found in region %s for key %s",
			ErrUnsupportedOrigin, input.PrimaryRegion, currentKey.KeyID,
		)
	}

	// Snapshot the current replica list before modifying anything.
	oldReplicaIDs := slices.Clone(currentKey.ReplicaKeyIDs)

	// Promote new primary: its replica list = all old replicas (except itself) + old primary.
	newReplicas := make([]string, 0, len(oldReplicaIDs))
	for _, rid := range oldReplicaIDs {
		if rid != newPrimaryID {
			newReplicas = append(newReplicas, rid)
		}
	}
	newReplicas = append(newReplicas, currentKey.KeyID)
	newPrimary.ReplicaKeyIDs = newReplicas
	newPrimary.PrimaryRegion = input.PrimaryRegion

	// Demote old primary to replica.
	currentKey.ReplicaKeyIDs = nil
	currentKey.PrimaryRegion = input.PrimaryRegion

	// Update all other replicas to point to the new primary region.
	for _, rid := range oldReplicaIDs {
		if rid == newPrimaryID {
			continue
		}

		if otherReplica := b.findKeyInAnyRegion(rid); otherReplica != nil {
			otherReplica.PrimaryRegion = input.PrimaryRegion
		}
	}

	return nil
}

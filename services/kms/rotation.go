package kms

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"time"
)

// EnableKeyRotation enables automatic key rotation for the specified key.
// The rotation period defaults to 365 days. Rotation is NOT performed immediately;
// it is scheduled starting from the key's creation date or last rotation date.
// The key must be in the Enabled state.
func (b *InMemoryBackend) EnableKeyRotation(
	ctx context.Context,
	input *EnableKeyRotationInput,
) error {
	b.mu.Lock("EnableKeyRotation")
	defer b.mu.Unlock()

	key, err := b.lookupKeyWrite(ctx, input.KeyID, ErrInvalidArn)
	if err != nil {
		return err
	}

	// Only SYMMETRIC_DEFAULT keys with AWS_KMS origin support rotation.
	if key.KeySpec != keySpecSymmetric {
		return fmt.Errorf(
			"%w: key rotation is only supported for symmetric SYMMETRIC_DEFAULT keys; key %q has spec %s",
			ErrUnsupportedOrigin,
			key.KeyID,
			key.KeySpec,
		)
	}

	if key.Origin == KeyOriginExternal {
		return fmt.Errorf(
			"%w: key rotation is not supported for EXTERNAL-origin keys",
			ErrUnsupportedOrigin,
		)
	}

	// AWS requires the key to be in Enabled state to enable rotation.
	if key.KeyState != KeyStateEnabled {
		return keyStateError(key)
	}

	rotationPeriod := int32(defaultRotationPeriodDays)

	if input.RotationPeriodInDays != nil && *input.RotationPeriodInDays > 0 {
		period := *input.RotationPeriodInDays
		if period < minRotationPeriodDays || period > maxRotationPeriodDays {
			return fmt.Errorf(
				"%w: RotationPeriodInDays must be between %d and %d, got %d",
				ErrValidation, minRotationPeriodDays, maxRotationPeriodDays, period,
			)
		}

		rotationPeriod = period
	}

	key.RotationEnabled = true
	key.RotationPeriodInDays = rotationPeriod

	return nil
}

// DisableKeyRotation disables automatic key rotation for the specified key.
// Asymmetric keys and EXTERNAL-origin keys do not support rotation and return ErrUnsupportedOrigin.
func (b *InMemoryBackend) DisableKeyRotation(
	ctx context.Context,
	input *DisableKeyRotationInput,
) error {
	b.mu.Lock("DisableKeyRotation")
	defer b.mu.Unlock()

	key, err := b.lookupKeyWrite(ctx, input.KeyID, ErrInvalidArn)
	if err != nil {
		return err
	}

	if key.KeySpec != keySpecSymmetric {
		return fmt.Errorf(
			"%w: key rotation is only supported for symmetric SYMMETRIC_DEFAULT keys; key %q has spec %s",
			ErrUnsupportedOrigin,
			key.KeyID,
			key.KeySpec,
		)
	}

	if key.Origin == KeyOriginExternal {
		return fmt.Errorf(
			"%w: key rotation is not supported for EXTERNAL-origin keys",
			ErrUnsupportedOrigin,
		)
	}

	key.RotationEnabled = false
	key.RotationPeriodInDays = 0

	return nil
}

// RotateKeyOnDemand rotates key material immediately without changing automatic rotation status.
func (b *InMemoryBackend) RotateKeyOnDemand(
	ctx context.Context,
	input *RotateKeyOnDemandInput,
) (*RotateKeyOnDemandOutput, error) {
	b.mu.Lock("RotateKeyOnDemand")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.defaultRegion)

	key, err := b.lookupKeyWrite(ctx, input.KeyID, ErrInvalidArn)
	if err != nil {
		return nil, err
	}

	if key.KeySpec != keySpecSymmetric {
		return nil, fmt.Errorf(
			"%w: key rotation is only supported for symmetric SYMMETRIC_DEFAULT keys; key %q has spec %s",
			ErrUnsupportedOrigin,
			key.KeyID,
			key.KeySpec,
		)
	}

	if key.Origin == KeyOriginExternal {
		return nil, fmt.Errorf(
			"%w: key rotation is not supported for EXTERNAL-origin keys",
			ErrUnsupportedOrigin,
		)
	}

	// AWS allows at most maxOnDemandRotationsPerDay on-demand rotations per 24-hour window.
	now := time.Now()
	cutoff := UnixTimeFloat(now.Add(-24 * time.Hour))
	recentCount := 0

	for _, r := range key.Rotations {
		if r.RotationType == rotationTypeImported && r.Date >= cutoff {
			recentCount++
		}
	}

	if recentCount >= maxOnDemandRotationsPerDay {
		return nil, fmt.Errorf(
			"%w: on-demand rotation limit of %d per 24-hour window exceeded for key %q",
			ErrLimitExceeded, maxOnDemandRotationsPerDay, key.KeyID,
		)
	}

	if err = b.rotateKeyMaterialLocked(region, key, rotationTypeImported); err != nil {
		return nil, err
	}

	key.OnDemandRotationDates = append(key.OnDemandRotationDates, UnixTimeFloat(now))

	return &RotateKeyOnDemandOutput{KeyID: key.KeyID}, nil
}

// GetKeyRotationStatus returns rotation configuration and schedule for the specified key.
func (b *InMemoryBackend) GetKeyRotationStatus(
	ctx context.Context,
	input *GetKeyRotationStatusInput,
) (*GetKeyRotationStatusOutput, error) {
	b.mu.RLock("GetKeyRotationStatus")
	defer b.mu.RUnlock()

	key, err := b.lookupKey(ctx, input.KeyID, ErrInvalidArn)
	if err != nil {
		return nil, err
	}

	// AWS raises UnsupportedOperationException for asymmetric or HMAC keys.
	if key.KeySpec != keySpecSymmetric || key.Origin == KeyOriginExternal {
		return nil, fmt.Errorf(
			"%w: GetKeyRotationStatus is only supported for symmetric keys with AWS_KMS origin; key %q has spec %s origin %s",
			ErrUnsupportedOrigin,
			key.KeyID,
			key.KeySpec,
			key.Origin,
		)
	}

	out := &GetKeyRotationStatusOutput{
		KeyRotationEnabled: key.RotationEnabled,
		KeyID:              key.KeyID,
	}

	if key.RotationEnabled {
		b.populateNextRotationDate(key, out)
	}

	out.OnDemandRotationStartDate = b.lastOnDemandRotationDate(key)

	return out, nil
}

// populateNextRotationDate fills NextRotationDate and RotationPeriodInDays on out.
// Must be called with at least a read lock held.
func (b *InMemoryBackend) populateNextRotationDate(key *Key, out *GetKeyRotationStatusOutput) {
	period := key.RotationPeriodInDays
	if period <= 0 {
		period = defaultRotationPeriodDays
	}

	out.RotationPeriodInDays = period

	lastRotation := b.lastScheduledRotationDate(key)
	out.NextRotationDate = lastRotation + float64(period)*float64(24*time.Hour/time.Second)
}

// lastScheduledRotationDate returns the date of the most recent AWS_KMS scheduled rotation,
// falling back to legacy slices and finally the key creation date.
// Must be called with at least a read lock held.
func (b *InMemoryBackend) lastScheduledRotationDate(key *Key) float64 {
	for _, v := range slices.Backward(key.Rotations) {
		if v.RotationType == rotationTypeAWSKMS {
			return v.Date
		}
	}

	if len(key.RotationDates) > 0 {
		return key.RotationDates[len(key.RotationDates)-1]
	}

	return key.CreationDate
}

// lastOnDemandRotationDate returns the date of the most recent on-demand rotation,
// falling back to the legacy OnDemandRotationDates slice.
// Must be called with at least a read lock held.
func (b *InMemoryBackend) lastOnDemandRotationDate(key *Key) float64 {
	for _, v := range slices.Backward(key.Rotations) {
		if v.RotationType == rotationTypeImported {
			return v.Date
		}
	}

	if len(key.OnDemandRotationDates) > 0 {
		return key.OnDemandRotationDates[len(key.OnDemandRotationDates)-1]
	}

	return 0
}

func (b *InMemoryBackend) rotateKeyMaterialLocked(
	region string,
	key *Key,
	rotationType string,
) error {
	if key.KeyState != KeyStateEnabled {
		return keyStateError(key)
	}

	if key.KeySpec != keySpecSymmetric {
		return fmt.Errorf(
			"%w: key rotation is only supported for symmetric SYMMETRIC_DEFAULT keys; key %q has spec %s",
			ErrUnsupportedOrigin,
			key.KeyID,
			key.KeySpec,
		)
	}

	if key.Origin == KeyOriginExternal {
		return fmt.Errorf(
			"%w: key rotation is not supported for EXTERNAL-origin keys; material is managed by the caller",
			ErrUnsupportedOrigin,
		)
	}

	newKM, kmErr := generateKeyMaterial(key.KeySpec)
	if kmErr != nil {
		return fmt.Errorf("rotating key material: %w", kmErr)
	}

	kms := b.keyMaterialsStore(region)
	kmh := b.keyMaterialHistoryStore(region)

	if current := kms[key.KeyID]; current != nil {
		kmh[key.KeyID] = append(kmh[key.KeyID], current)
		// Cap retained history to bound long-running mock memory growth.
		hist := kmh[key.KeyID]
		if len(hist) > maxKeyMaterialHistoryEntries {
			kmh[key.KeyID] = hist[len(hist)-maxKeyMaterialHistoryEntries:]
		}
	}

	kms[key.KeyID] = newKM
	ts := UnixTimeFloat(time.Now())
	key.RotationDates = append(key.RotationDates, ts)
	key.Rotations = append(key.Rotations, RotationRecord{Date: ts, RotationType: rotationType})

	return nil
}

// ListKeyRotations returns observed key material rotation timestamps for a key.
func (b *InMemoryBackend) ListKeyRotations(
	ctx context.Context,
	input *ListKeyRotationsInput,
) (*ListKeyRotationsOutput, error) {
	b.mu.RLock("ListKeyRotations")
	defer b.mu.RUnlock()

	key, err := b.lookupKey(ctx, input.KeyID, ErrInvalidArn)
	if err != nil {
		return nil, err
	}

	// Build rotation entries from the typed Rotations slice. Legacy keys loaded
	// from older snapshots that only have RotationDates (no Rotations) will show
	// empty history; this is acceptable since type information cannot be recovered.
	rotations := make([]KeyRotationEntry, 0, len(key.Rotations))
	for _, r := range key.Rotations {
		rotations = append(rotations, KeyRotationEntry{
			KeyID:        key.KeyID,
			RotationDate: r.Date,
			RotationType: r.RotationType,
		})
	}

	startIdx := parseMarker(input.Marker)
	limit := int32(defaultListLimit)

	if input.Limit != nil && *input.Limit > 0 {
		limit = *input.Limit
	}

	if startIdx >= len(rotations) {
		return &ListKeyRotationsOutput{Rotations: []KeyRotationEntry{}, Truncated: false}, nil
	}

	end := startIdx + int(limit)
	nextMarker := ""
	if end < len(rotations) {
		nextMarker = strconv.Itoa(end)
	} else {
		end = len(rotations)
	}

	return &ListKeyRotationsOutput{
		Rotations:  rotations[startIdx:end],
		NextMarker: nextMarker,
		Truncated:  nextMarker != "",
	}, nil
}

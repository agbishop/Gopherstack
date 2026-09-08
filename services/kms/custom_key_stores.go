package kms

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// CreateCustomKeyStore creates a new in-memory custom key store entry in DISCONNECTED state.
func (b *InMemoryBackend) CreateCustomKeyStore(
	ctx context.Context, input *CreateCustomKeyStoreInput,
) (*CreateCustomKeyStoreOutput, error) {
	if strings.TrimSpace(input.CustomKeyStoreName) == "" {
		return nil, fmt.Errorf("%w: CustomKeyStoreName must not be empty", ErrValidation)
	}

	storeType := input.CustomKeyStoreType
	if storeType == "" {
		storeType = "AWS_CLOUDHSM"
	}

	if storeType != "AWS_CLOUDHSM" && storeType != "EXTERNAL_KEY_STORE" {
		return nil, fmt.Errorf(
			"%w: CustomKeyStoreType must be AWS_CLOUDHSM or EXTERNAL_KEY_STORE",
			ErrValidation,
		)
	}

	b.mu.Lock("CreateCustomKeyStore")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.defaultRegion)

	// Ensure name is unique.
	for _, ks := range b.customKeyStoresStore(region).All() {
		if ks.CustomKeyStoreName == input.CustomKeyStoreName {
			return nil, fmt.Errorf(
				"%w: custom key store with name %q already exists",
				ErrCustomKeyStoreAlreadyExists, input.CustomKeyStoreName,
			)
		}
	}

	storeID := uuid.New().String()

	b.customKeyStoresStore(region).Put(&CustomKeyStore{
		CustomKeyStoreID:   storeID,
		CustomKeyStoreName: input.CustomKeyStoreName,
		ConnectionState:    ConnectionStateDisconnected,
		CreationDate:       UnixTimeFloat(time.Now()),
		CustomKeyStoreType: storeType,
	})

	return &CreateCustomKeyStoreOutput{CustomKeyStoreID: storeID}, nil
}

// DeleteCustomKeyStore removes an existing custom key store. It must be in DISCONNECTED state.
func (b *InMemoryBackend) DeleteCustomKeyStore(
	ctx context.Context,
	input *DeleteCustomKeyStoreInput,
) error {
	b.mu.Lock("DeleteCustomKeyStore")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.defaultRegion)

	ks, ok := b.customKeyStoresStore(region).Get(input.CustomKeyStoreID)
	if !ok {
		return fmt.Errorf(
			"%w: custom key store %q not found",
			ErrCustomKeyStoreNotFound,
			input.CustomKeyStoreID,
		)
	}

	if ks.ConnectionState != ConnectionStateDisconnected {
		return fmt.Errorf(
			"%w: custom key store must be DISCONNECTED before deletion; current state: %s",
			ErrCustomKeyStoreInvalidState, ks.ConnectionState,
		)
	}

	// "The custom key store that you delete cannot contain any KMS keys" (real SDK:
	// api_op_DeleteCustomKeyStore.go doc comment; CustomKeyStoreHasCMKsException).
	for _, k := range b.keysStore(region).All() {
		if k.CustomKeyStoreID == input.CustomKeyStoreID {
			return fmt.Errorf(
				"%w: custom key store %q still contains KMS keys",
				ErrCustomKeyStoreHasKeys, input.CustomKeyStoreID,
			)
		}
	}

	b.customKeyStoresStore(region).Delete(input.CustomKeyStoreID)

	return nil
}

// DescribeCustomKeyStores returns a list of custom key stores matching optional filters.
func (b *InMemoryBackend) DescribeCustomKeyStores(
	ctx context.Context, input *DescribeCustomKeyStoresInput,
) (*DescribeCustomKeyStoresOutput, error) {
	b.mu.RLock("DescribeCustomKeyStores")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.defaultRegion)

	stores := make([]CustomKeyStore, 0, b.customKeyStoresStore(region).Len())

	for _, ks := range b.customKeyStoresStore(region).All() {
		if input.CustomKeyStoreID != "" && ks.CustomKeyStoreID != input.CustomKeyStoreID {
			continue
		}

		if input.CustomKeyStoreName != "" && ks.CustomKeyStoreName != input.CustomKeyStoreName {
			continue
		}

		stores = append(stores, *ks)
	}

	sort.Slice(stores, func(i, j int) bool {
		return stores[i].CustomKeyStoreID < stores[j].CustomKeyStoreID
	})

	startIdx := parseMarker(input.Marker)
	limit := int32(defaultListLimit)

	if input.Limit != nil && *input.Limit > 0 {
		limit = *input.Limit
	}

	if startIdx >= len(stores) {
		return &DescribeCustomKeyStoresOutput{CustomKeyStores: []CustomKeyStore{}}, nil
	}

	end := startIdx + int(limit)

	var nextMarker string
	if end < len(stores) {
		nextMarker = strconv.Itoa(end)
	} else {
		end = len(stores)
	}

	return &DescribeCustomKeyStoresOutput{
		CustomKeyStores: stores[startIdx:end],
		NextMarker:      nextMarker,
		Truncated:       nextMarker != "",
	}, nil
}

// ConnectCustomKeyStore transitions a custom key store from DISCONNECTED to CONNECTED.
func (b *InMemoryBackend) ConnectCustomKeyStore(
	ctx context.Context,
	input *ConnectCustomKeyStoreInput,
) error {
	b.mu.Lock("ConnectCustomKeyStore")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.defaultRegion)

	ks, ok := b.customKeyStoresStore(region).Get(input.CustomKeyStoreID)
	if !ok {
		return fmt.Errorf(
			"%w: custom key store %q not found",
			ErrCustomKeyStoreNotFound,
			input.CustomKeyStoreID,
		)
	}

	if ks.ConnectionState == ConnectionStateConnected {
		return fmt.Errorf(
			"%w: custom key store %q is already connected",
			ErrCustomKeyStoreInvalidState, input.CustomKeyStoreID,
		)
	}

	ks.ConnectionState = ConnectionStateConnected

	return nil
}

// DisconnectCustomKeyStore transitions a custom key store from CONNECTED to DISCONNECTED.
func (b *InMemoryBackend) DisconnectCustomKeyStore(
	ctx context.Context,
	input *DisconnectCustomKeyStoreInput,
) error {
	b.mu.Lock("DisconnectCustomKeyStore")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.defaultRegion)

	ks, ok := b.customKeyStoresStore(region).Get(input.CustomKeyStoreID)
	if !ok {
		return fmt.Errorf(
			"%w: custom key store %q not found",
			ErrCustomKeyStoreNotFound,
			input.CustomKeyStoreID,
		)
	}

	if ks.ConnectionState == ConnectionStateDisconnected {
		return fmt.Errorf(
			"%w: custom key store %q is already disconnected",
			ErrCustomKeyStoreInvalidState, input.CustomKeyStoreID,
		)
	}

	ks.ConnectionState = ConnectionStateDisconnected

	return nil
}

// UpdateCustomKeyStore updates mutable properties for a custom key store.
func (b *InMemoryBackend) UpdateCustomKeyStore(
	ctx context.Context,
	input *UpdateCustomKeyStoreInput,
) error {
	b.mu.Lock("UpdateCustomKeyStore")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.defaultRegion)

	ks, ok := b.customKeyStoresStore(region).Get(input.CustomKeyStoreID)
	if !ok {
		return fmt.Errorf(
			"%w: custom key store %q not found",
			ErrCustomKeyStoreNotFound,
			input.CustomKeyStoreID,
		)
	}

	if input.NewCustomKeyStoreName != "" && input.NewCustomKeyStoreName != ks.CustomKeyStoreName {
		for _, existing := range b.customKeyStoresStore(region).All() {
			if existing.CustomKeyStoreName == input.NewCustomKeyStoreName {
				return fmt.Errorf(
					"%w: custom key store with name %q already exists",
					ErrCustomKeyStoreAlreadyExists,
					input.NewCustomKeyStoreName,
				)
			}
		}

		ks.CustomKeyStoreName = input.NewCustomKeyStoreName
	}

	return nil
}

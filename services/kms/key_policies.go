package kms

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	gopherarn "github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// PutKeyPolicy stores a key policy for a KMS key.
// Only the "default" policy name is supported.
func (b *InMemoryBackend) PutKeyPolicy(ctx context.Context, input *PutKeyPolicyInput) error {
	policyName := input.PolicyName
	if policyName == "" {
		policyName = defaultKeyPolicyName
	}

	// Same fit as the handler-level check in buildGrantPolicyActions
	// (gopherstack-i4q8): UnsupportedOperationException, per its doc "a
	// specified parameter is not supported". Unreachable in production today --
	// the handler already normalizes/rejects PolicyName before calling this
	// method -- but kept correct as defense in depth.
	if policyName != defaultKeyPolicyName {
		return fmt.Errorf(
			"%w: PolicyName must be %q; got %q",
			ErrUnsupportedParameter, defaultKeyPolicyName, policyName,
		)
	}

	b.mu.Lock("PutKeyPolicy")
	defer b.mu.Unlock()

	// Store the policy in the key's own region (ARN-embedded region for an ARN
	// input), so GetKeyPolicy reads it back consistently regardless of the request
	// region.
	key, region, err := b.resolveKeyAndRegion(ctx, input.KeyID, ErrInvalidArn)
	if err != nil {
		return err
	}

	if !validKeyPolicyDoc(input.Policy) {
		return ErrMalformedPolicyDocument
	}

	b.policiesStore(region)[key.KeyID] = input.Policy

	return nil
}

// validKeyPolicyDoc reports whether s is a well-formed KMS key policy document
// (valid JSON with non-empty Version and a Statement). Used by both PutKeyPolicy
// and CreateKey (which accepts an inline Policy).
func validKeyPolicyDoc(s string) bool {
	var policyDoc struct {
		Statement any    `json:"Statement"`
		Version   string `json:"Version"`
	}
	if err := json.Unmarshal([]byte(s), &policyDoc); err != nil {
		return false
	}

	return policyDoc.Version != "" && policyDoc.Statement != nil
}

// GetKeyPolicy retrieves the key policy for a KMS key.
func (b *InMemoryBackend) GetKeyPolicy(
	ctx context.Context,
	input *GetKeyPolicyInput,
) (*GetKeyPolicyOutput, error) {
	b.mu.RLock("GetKeyPolicy")
	defer b.mu.RUnlock()

	// Resolve against the key's own region (ARN-embedded region for an ARN input),
	// not the request region, so a cross-region ARN reads the policy from the store
	// the key actually lives in.
	key, region, err := b.resolveKeyAndRegion(ctx, input.KeyID, ErrInvalidArn)
	if err != nil {
		return nil, err
	}

	keyID := key.KeyID

	policy, ok := b.policiesStore(region)[keyID]
	if !ok {
		// Return default policy
		rootARN := gopherarn.Build("iam", "", b.accountID, "root")
		policy = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow",` +
			`"Principal":{"AWS":"` + rootARN + `"},"Action":"kms:*","Resource":"*"}]}`
	}

	policyName := input.PolicyName
	if policyName == "" {
		policyName = defaultKeyPolicyName
	}

	return &GetKeyPolicyOutput{Policy: policy, PolicyName: policyName}, nil
}

// ListKeyPolicies returns policy names available for a key.
func (b *InMemoryBackend) ListKeyPolicies(
	ctx context.Context,
	input *ListKeyPoliciesInput,
) (*ListKeyPoliciesOutput, error) {
	b.mu.RLock("ListKeyPolicies")
	defer b.mu.RUnlock()

	if _, err := b.lookupKey(ctx, input.KeyID, ErrInvalidArn); err != nil {
		return nil, err
	}

	names := []string{defaultKeyPolicyName}
	startIdx := parseMarker(input.Marker)
	limit := int32(defaultListLimit)

	if input.Limit != nil && *input.Limit > 0 {
		limit = *input.Limit
	}

	if startIdx >= len(names) {
		return &ListKeyPoliciesOutput{PolicyNames: []string{}, Truncated: false}, nil
	}

	end := startIdx + int(limit)
	nextMarker := ""
	if end < len(names) {
		nextMarker = strconv.Itoa(end)
	} else {
		end = len(names)
	}

	return &ListKeyPoliciesOutput{
		PolicyNames: names[startIdx:end],
		NextMarker:  nextMarker,
		Truncated:   nextMarker != "",
	}, nil
}

package sts

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// validateAssumeRoleInput checks the common parameter constraints for AssumeRole.
func validateAssumeRoleInput(input *AssumeRoleInput) error {
	if input.RoleArn == "" {
		return ErrMissingRoleArn
	}

	if err := validateRoleArn(input.RoleArn); err != nil {
		return err
	}

	if input.RoleSessionName == "" {
		return ErrMissingSessionName
	}

	if err := validateRoleSessionName(input.RoleSessionName); err != nil {
		return err
	}

	if err := validateMFAFields(input.SerialNumber, input.TokenCode); err != nil {
		return err
	}

	if len(input.Tags) > MaxTagCount {
		return fmt.Errorf("%w: got %d", ErrTooManyTags, len(input.Tags))
	}

	if err := validateTagConstraints(input.Tags); err != nil {
		return err
	}

	if err := validateSourceIdentity(input.SourceIdentity); err != nil {
		return err
	}

	if err := validatePolicyArns(input.PolicyArns); err != nil {
		return err
	}

	if err := validateProvidedContexts(input.ProvidedContexts); err != nil {
		return err
	}

	if err := validateInlinePolicy(input.Policy); err != nil {
		return err
	}

	return checkPackedPolicyBudget(input.Policy, input.PolicyArns)
}

// AssumeRole generates temporary credentials for the given role.
func (b *InMemoryBackend) AssumeRole(input *AssumeRoleInput) (*AssumeRoleResponse, error) {
	b.cntAssumeRole.Add(1)

	if err := validateAssumeRoleInput(input); err != nil {
		return nil, err
	}

	effectiveMax, err := b.validateAndGetMaxDuration(input)
	if err != nil {
		return nil, err
	}

	if err = b.checkAssumeRoleTrust(input); err != nil {
		return nil, err
	}

	duration := input.DurationSeconds
	if duration == 0 {
		// Clamp default to the role's MaxSessionDuration when it's less than the standard default.
		duration = min(DefaultDurationSeconds, effectiveMax)
	}

	if duration < MinDurationSeconds {
		return nil, fmt.Errorf(
			"%w: DurationSeconds must be at least %d",
			ErrInvalidDuration, MinDurationSeconds,
		)
	}

	if duration > effectiveMax {
		return nil, fmt.Errorf(
			"%w: DurationSeconds must not exceed %d for this role",
			ErrInvalidDuration, effectiveMax,
		)
	}

	return b.issueCredentials(input, duration)
}

// validateAndGetMaxDuration validates ExternalId against the trust policy (when a RoleLookup
// is configured) and returns the effective maximum session duration for the role.
// When the caller uses temporary credentials (role chaining), the effective max is
// capped at MaxRoleChainDurationSeconds (3600s) per AWS rules.
func (b *InMemoryBackend) validateAndGetMaxDuration(input *AssumeRoleInput) (int32, error) {
	effectiveMax, err := b.roleDerivedMaxDuration(input)
	if err != nil {
		return 0, err
	}

	// AWS caps the max session duration at 1 hour when the caller is already using
	// temporary credentials (role chaining). ASIA prefix identifies temporary keys.
	if strings.HasPrefix(input.CallerAccessKeyID, accessKeyIDPrefix) && effectiveMax > MaxRoleChainDurationSeconds {
		effectiveMax = MaxRoleChainDurationSeconds
	}

	return effectiveMax, nil
}

// roleDerivedMaxDuration reads the role's MaxSessionDuration from the RoleLookup (if any)
// and validates ExternalId. Returns MaxDurationSeconds when no lookup is configured or the
// role is not found.
func (b *InMemoryBackend) roleDerivedMaxDuration(input *AssumeRoleInput) (int32, error) {
	b.mu.RLock("RoleDerivedMaxDuration")
	rl := b.roleLookup
	b.mu.RUnlock()

	if rl == nil {
		return int32(MaxDurationSeconds), nil
	}

	meta, _ := rl.GetRoleByArn(input.RoleArn)
	if meta == nil {
		return int32(MaxDurationSeconds), nil
	}

	if err := validateExternalID(meta.TrustPolicy, input.ExternalID); err != nil {
		return 0, err
	}

	if err := validateMFACondition(meta.TrustPolicy, mfaPresent(input)); err != nil {
		return 0, err
	}

	if meta.MaxSessionDuration > 0 {
		return meta.MaxSessionDuration, nil
	}

	return int32(MaxDurationSeconds), nil
}

// checkAssumeRoleTrust evaluates the target role's trust policy against the
// calling principal for sts:AssumeRole. It is a no-op (permissive) unless a
// caller ARN was resolved, a RoleLookup is wired, and the role carries a
// non-empty trust policy — mirroring the emulator's "enforce only what is
// positively known" stance so callers without a resolvable identity are not
// spuriously denied.
func (b *InMemoryBackend) checkAssumeRoleTrust(input *AssumeRoleInput) error {
	if input.CallerArn == "" {
		return nil
	}

	meta := b.lookupRoleMeta(input.RoleArn)
	if meta == nil || meta.TrustPolicy == "" {
		return nil
	}

	b.mu.RLock("checkAssumeRoleTrust")
	strict := b.strictConditions
	b.mu.RUnlock()

	return evaluateAssumeRoleTrust(meta.TrustPolicy, trustEval{
		action:           actionAssumeRole,
		callerArn:        input.CallerArn,
		externalID:       input.ExternalID,
		strictConditions: strict,
		conditionCtx: map[string]string{
			condKeyPrincipalArn: input.CallerArn,
			condKeyMFAPresent:   strconv.FormatBool(mfaPresent(input)),
		},
	})
}

// mfaPresent reports whether the caller supplied a well-formed SerialNumber
// and TokenCode pair. validateMFAFields (called earlier via
// validateAssumeRoleInput) guarantees the two fields are either both set or
// both empty by the time this is evaluated.
func mfaPresent(input *AssumeRoleInput) bool {
	return input.SerialNumber != "" && input.TokenCode != ""
}

// mergeTransitiveTags combines the parent session's transitive tags with the child's
// explicit tags. Parent tags whose key appears in parent.TransitiveTagKeys are
// inherited; the child's own tags take precedence on key conflicts.
func mergeTransitiveTags(parent *SessionInfo, childTags []Tag) []Tag {
	if parent == nil || len(parent.TransitiveTagKeys) == 0 {
		return childTags
	}

	transitiveSet := make(map[string]struct{}, len(parent.TransitiveTagKeys))
	for _, k := range parent.TransitiveTagKeys {
		transitiveSet[k] = struct{}{}
	}

	// Build child key set for conflict resolution.
	childKeys := make(map[string]struct{}, len(childTags))
	for _, t := range childTags {
		childKeys[t.Key] = struct{}{}
	}

	merged := make([]Tag, 0, len(childTags)+len(parent.Tags))
	// Inherit parent transitive tags not overridden by child.
	for _, t := range parent.Tags {
		if _, isTransitive := transitiveSet[t.Key]; !isTransitive {
			continue
		}
		if _, childOverrides := childKeys[t.Key]; childOverrides {
			continue
		}
		merged = append(merged, t)
	}
	merged = append(merged, childTags...)

	return merged
}

// issueCredentials generates credentials, stores the session, and builds the response.
func (b *InMemoryBackend) issueCredentials(
	input *AssumeRoleInput,
	duration int32,
) (*AssumeRoleResponse, error) {
	creds, err := generateCredentialSet()
	if err != nil {
		return nil, err
	}

	expiration := time.Now().UTC().Add(time.Duration(duration) * time.Second)
	roleID := deriveRoleID(input.RoleArn)
	assumedRoleID := roleID + ":" + input.RoleSessionName
	assumedRoleArn := buildAssumedRoleArn(input.RoleArn, input.RoleSessionName)

	account := b.accountID
	if parts := strings.SplitN(input.RoleArn, ":", arnComponentCount); len(
		parts,
	) >= arnComponentCount {
		account = parts[4]
	}

	// Merge parent transitive tags into child session per AWS role-chaining rules:
	// tags marked transitive by the parent session propagate to the child and are
	// inherited even if the child caller does not re-specify them.
	mergedTags := mergeTransitiveTags(input.CallerSession, input.Tags)

	session := &SessionInfo{
		AssumedRoleArn:    assumedRoleArn,
		AccountID:         account,
		SessionName:       input.RoleSessionName,
		AccessKeyID:       creds.AccessKeyID,
		SecretAccessKey:   creds.SecretAccessKey,
		SessionToken:      creds.SessionToken,
		AssumedRoleID:     assumedRoleID,
		SourceIdentity:    input.SourceIdentity,
		Tags:              mergedTags,
		TransitiveTagKeys: input.TransitiveTagKeys,
		Expiration:        expiration,
		IsAssumedRole:     true,
	}

	b.storeSession(session)

	result := AssumeRoleResult{
		AssumedRoleUser: AssumedRoleUser{
			Arn:           assumedRoleArn,
			AssumedRoleID: assumedRoleID,
		},
		Credentials: Credentials{
			AccessKeyID:     creds.AccessKeyID,
			SecretAccessKey: creds.SecretAccessKey,
			SessionToken:    creds.SessionToken,
			Expiration:      expiration.Format(time.RFC3339),
		},
		SourceIdentity:   input.SourceIdentity,
		PackedPolicySize: calculatePackedPolicySizeWithArns(input.Policy, input.PolicyArns),
	}

	return &AssumeRoleResponse{
		Xmlns:            STSNamespace,
		AssumeRoleResult: result,
		ResponseMetadata: ResponseMetadata{RequestID: uuid.NewString()},
	}, nil
}

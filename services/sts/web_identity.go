package sts

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

// webIdentitySubjectPlaceholder is returned when the sub claim cannot be extracted from the token.
const webIdentitySubjectPlaceholder = "WebIdentitySubject"

// isValidWebIdentitySigningAlgorithm reports whether the given signing algorithm is supported.
// Per the real AWS STS GetWebIdentityToken API, only RS256 (RSA with SHA-256) and
// ES384 (ECDSA using the P-384 curve with SHA-384) are valid; any other value
// (including other JOSE algorithms such as ES256 or PS256) is rejected.
func isValidWebIdentitySigningAlgorithm(alg string) bool {
	switch alg {
	case "RS256", "ES384":
		return true
	}

	return false
}

// AssumeRoleWithWebIdentity generates temporary credentials using a web identity token.
// In this mock, the WebIdentityToken is not cryptographically validated; the subject
// is extracted from the token's payload when parseable, otherwise a default is used.
// validateWebIdentityInput checks the parameter constraints for AssumeRoleWithWebIdentity.
func validateWebIdentityInput(input *AssumeRoleWithWebIdentityInput) error {
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

	if input.WebIdentityToken == "" {
		return ErrMissingWebIdentityToken
	}

	if err := validatePolicyArns(input.PolicyArns); err != nil {
		return err
	}

	if err := validateInlinePolicy(input.Policy); err != nil {
		return err
	}

	if err := checkPackedPolicyBudget(input.Policy, input.PolicyArns); err != nil {
		return err
	}

	return validateWebIdentityToken(input.WebIdentityToken)
}

// validateWebIdentityTokenClaims checks the constraints on identity attributes
// extracted from the WebIdentityToken's custom claims (SourceIdentity, session
// tags) — mirroring the constraints AWS applies to the equivalent AssumeRole
// request parameters, since AssumeRoleWithWebIdentity sources these values
// from the token instead of accepting them directly (see jwtClaimTags /
// jwtClaimSourceIdentity in token_validation.go).
func validateWebIdentityTokenClaims(sourceIdentity string, tags []Tag) error {
	if err := validateSourceIdentity(sourceIdentity); err != nil {
		return err
	}

	if len(tags) > MaxTagCount {
		return fmt.Errorf("%w: got %d", ErrTooManyTags, len(tags))
	}

	return validateTagConstraints(tags)
}

func (b *InMemoryBackend) AssumeRoleWithWebIdentity(
	input *AssumeRoleWithWebIdentityInput,
) (*AssumeRoleWithWebIdentityResponse, error) {
	b.cntAssumeRoleWithWebIdentity.Add(1)

	if err := validateWebIdentityInput(input); err != nil {
		return nil, err
	}

	sourceIdentity := extractWebIdentitySourceIdentity(input.WebIdentityToken)
	tags := extractWebIdentityTags(input.WebIdentityToken)

	if err := validateWebIdentityTokenClaims(sourceIdentity, tags); err != nil {
		return nil, err
	}

	effectiveMax := b.getEffectiveMaxDuration(input.RoleArn)

	duration := input.DurationSeconds
	if duration == 0 {
		duration = min(DefaultDurationSeconds, effectiveMax)
	}

	if duration < MinDurationSeconds || duration > effectiveMax {
		return nil, fmt.Errorf(
			"%w: DurationSeconds must be between %d and %d for this role",
			ErrInvalidDuration, MinDurationSeconds, effectiveMax,
		)
	}

	// Validate OIDC provider when a lookup is configured.
	if err := b.validateOIDCProvider(input.WebIdentityToken, input.ProviderID); err != nil {
		return nil, err
	}

	// Enforce the role's trust policy against the federated OIDC identity.
	if err := b.checkWebIdentityTrust(input); err != nil {
		return nil, err
	}

	creds, err := generateCredentialSet()
	if err != nil {
		return nil, err
	}

	return b.buildWebIdentityResponse(input, sourceIdentity, tags, creds, duration), nil
}

// checkOutboundWebIdentityFederationEnabled gates GetWebIdentityToken on the
// account's outbound-web-identity-federation setting (real AWS's
// OutboundWebIdentityFederationDisabledException, thrown when IAM's
// EnableOutboundWebIdentityFederation has not been called for the account --
// see services/iam/account.go). When no AccountSettingsLookup is configured
// (e.g. in isolated unit tests that never call SetOIDCLookup), the check is
// skipped (permissive mock behaviour), matching validateOIDCProvider's same
// "skip when unwired" convention below.
func (b *InMemoryBackend) checkOutboundWebIdentityFederationEnabled() error {
	b.mu.RLock("checkOutboundWebIdentityFederationEnabled")
	asl := b.accountSettingsLookup
	b.mu.RUnlock()

	if asl == nil {
		return nil
	}

	if !asl.OutboundWebIdentityFederationEnabled() {
		return ErrOutboundWebIdentityFederationDisabled
	}

	return nil
}

// validateOIDCProvider checks that the issuer from the token (or providerID override)
// corresponds to an existing OIDC provider in the IAM backend.
// When no OIDCLookup is configured the check is skipped (permissive mock behaviour).
func (b *InMemoryBackend) validateOIDCProvider(token, providerID string) error {
	b.mu.RLock("ValidateOIDCProvider")
	ol := b.oidcLookup
	b.mu.RUnlock()

	if ol == nil {
		return nil
	}

	issuer := providerID
	if issuer == "" {
		issuer = extractWebIdentityIssuer(token)
	}

	if issuer == "" {
		// No issuer to validate against; allow the call.
		return nil
	}

	if !ol.OIDCProviderExists(issuer) {
		return fmt.Errorf(
			"%w: OIDC provider for issuer %q not found in IAM",
			ErrAccessDenied,
			issuer,
		)
	}

	return nil
}

// checkWebIdentityTrust evaluates the target role's trust policy against the
// federated OIDC identity (issuer/audience) for sts:AssumeRoleWithWebIdentity.
// It is permissive unless the token yields an issuer, a RoleLookup is wired, and
// the role carries a non-empty trust policy.
func (b *InMemoryBackend) checkWebIdentityTrust(input *AssumeRoleWithWebIdentityInput) error {
	issuer := input.ProviderID
	if issuer == "" {
		issuer = extractWebIdentityIssuer(input.WebIdentityToken)
	}

	if issuer == "" {
		return nil
	}

	meta := b.lookupRoleMeta(input.RoleArn)
	if meta == nil || meta.TrustPolicy == "" {
		return nil
	}

	host := strings.ToLower(strings.TrimPrefix(strings.TrimPrefix(issuer, "https://"), "http://"))
	condCtx := map[string]string{}

	if aud := extractWebIdentityAudience(input.WebIdentityToken); aud != "" {
		condCtx[host+":aud"] = aud
	}

	if sub := extractWebIdentitySubject(input.WebIdentityToken); sub != "" &&
		sub != webIdentitySubjectPlaceholder {
		condCtx[host+":sub"] = sub
	}

	return evaluateAssumeRoleTrust(meta.TrustPolicy, trustEval{
		action:       actionAssumeRoleWithWebID,
		federatedArn: issuer,
		conditionCtx: condCtx,
	})
}

// buildWebIdentityResponse constructs the AssumeRoleWithWebIdentity response
// and persists the session. sourceIdentity and tags are the values already
// extracted from and validated against the WebIdentityToken's custom claims
// (see validateWebIdentityTokenClaims) — there is no separate SourceIdentity
// or Tags request parameter for this operation.
func (b *InMemoryBackend) buildWebIdentityResponse(
	input *AssumeRoleWithWebIdentityInput,
	sourceIdentity string,
	tags []Tag,
	creds credentialSet,
	duration int32,
) *AssumeRoleWithWebIdentityResponse {
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

	subject := extractWebIdentitySubject(input.WebIdentityToken)
	provider, audience := resolveWebIdentityProvider(input.WebIdentityToken, input.ProviderID)

	session := &SessionInfo{
		Expiration:      expiration,
		AssumedRoleArn:  assumedRoleArn,
		AccountID:       account,
		SessionName:     input.RoleSessionName,
		AccessKeyID:     creds.AccessKeyID,
		SecretAccessKey: creds.SecretAccessKey,
		SessionToken:    creds.SessionToken,
		AssumedRoleID:   assumedRoleID,
		Tags:            tags,
		SourceIdentity:  sourceIdentity,
		IsAssumedRole:   true,
	}

	b.storeSession(session)

	return &AssumeRoleWithWebIdentityResponse{
		Xmlns: STSNamespace,
		AssumeRoleWithWebIdentityResult: AssumeRoleWithWebIdentityResult{
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
			SubjectFromWebIdentityToken: subject,
			Audience:                    audience,
			Provider:                    provider,
			SourceIdentity:              sourceIdentity,
			PackedPolicySize: calculatePackedPolicySizeWithArns(
				input.Policy,
				input.PolicyArns,
			),
		},
		ResponseMetadata: ResponseMetadata{RequestID: uuid.NewString()},
	}
}

// validateGetWebIdentityTokenInput checks the Audience/Tags/SigningAlgorithm
// constraints for GetWebIdentityToken. Split out of GetWebIdentityToken
// purely to keep that function's cyclomatic complexity under the linter's
// threshold.
func validateGetWebIdentityTokenInput(input *GetWebIdentityTokenInput) error {
	if len(input.Audience) == 0 {
		return ErrMissingAudience
	}

	if len(input.Audience) > MaxAudienceCount {
		return fmt.Errorf("%w: got %d", ErrTooManyAudiences, len(input.Audience))
	}

	if len(input.Tags) > MaxTagCount {
		return fmt.Errorf("%w: got %d", ErrTooManyTags, len(input.Tags))
	}

	if err := validateTagConstraints(input.Tags); err != nil {
		return err
	}

	if input.SigningAlgorithm == "" {
		return ErrMissingSigningAlgorithm
	}

	if !isValidWebIdentitySigningAlgorithm(input.SigningAlgorithm) {
		return fmt.Errorf(
			"%w: unsupported signing algorithm %q",
			ErrValidation,
			input.SigningAlgorithm,
		)
	}

	return nil
}

// resolveWebIdentityTokenDuration applies GetWebIdentityToken's
// DurationSeconds default and range validation. Split out of
// GetWebIdentityToken purely to keep that function's cyclomatic complexity
// under the linter's threshold.
func resolveWebIdentityTokenDuration(input *GetWebIdentityTokenInput) (int32, error) {
	duration := input.DurationSeconds
	if duration == 0 {
		duration = DefaultWebIdentityTokenDurationSeconds
	}

	if duration < MinWebIdentityTokenDurationSeconds ||
		duration > MaxWebIdentityTokenDurationSeconds {
		return 0, fmt.Errorf(
			"%w: DurationSeconds must be between %d and %d for GetWebIdentityToken",
			ErrInvalidDuration,
			MinWebIdentityTokenDurationSeconds,
			MaxWebIdentityTokenDurationSeconds,
		)
	}

	return duration, nil
}

// GetWebIdentityToken returns a signed JWT representing the caller's AWS identity.
// In this mock, the token is an unsigned JWT containing the caller's account and audience.
func (b *InMemoryBackend) GetWebIdentityToken(
	input *GetWebIdentityTokenInput,
) (*GetWebIdentityTokenResponse, error) {
	b.cntGetWebIdentityToken.Add(1)

	if err := b.checkOutboundWebIdentityFederationEnabled(); err != nil {
		return nil, err
	}

	if err := validateGetWebIdentityTokenInput(input); err != nil {
		return nil, err
	}

	duration, err := resolveWebIdentityTokenDuration(input)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	expiration := now.Add(time.Duration(duration) * time.Second)

	// A caller using temporary STS credentials cannot request a JWT that
	// outlives their own session (AWS SessionDurationEscalationException).
	if input.CallerSession != nil && expiration.After(input.CallerSession.Expiration) {
		return nil, ErrSessionDurationEscalation
	}

	issuer := "https://sts.mock.aws.com/" + b.accountID

	// Build a minimal mock JWT payload (unsigned, for testing purposes only).
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"mock","typ":"JWT"}`))

	claims := map[string]any{
		jwtClaimSub: MockUserID,
		jwtClaimAud: input.Audience,
		jwtClaimIss: issuer,
		jwtClaimExp: expiration.Unix(),
		jwtClaimIat: now.Unix(),
		jwtClaimNbf: now.Unix(),
		"acc":       b.accountID,
	}

	// Include session tags as custom claims when present.
	if len(input.Tags) > 0 {
		tagMap := make(map[string]string, len(input.Tags))
		for _, t := range input.Tags {
			tagMap[t.Key] = t.Value
		}

		claims[jwtClaimTags] = tagMap
	}

	payload, err := json.Marshal(claims)
	if err != nil {
		return nil, fmt.Errorf("build token payload: %w", err)
	}

	token := header + "." + base64.RawURLEncoding.EncodeToString(payload) + ".mock-signature"

	return &GetWebIdentityTokenResponse{
		Xmlns: STSNamespace,
		GetWebIdentityTokenResult: GetWebIdentityTokenResult{
			WebIdentityToken: token,
			Expiration:       expiration.Format(time.RFC3339),
		},
		ResponseMetadata: ResponseMetadata{RequestID: uuid.NewString()},
	}, nil
}

// extractWebIdentitySubject attempts to extract the "sub" claim from a JWT token's
// payload without validating the signature. If extraction fails, returns a placeholder.
func extractWebIdentitySubject(token string) string {
	claims := parseJWTPayloadClaims(token)
	if claims == nil {
		return webIdentitySubjectPlaceholder
	}

	if sub, ok := claims[jwtClaimSub].(string); ok && sub != "" {
		return sub
	}

	return webIdentitySubjectPlaceholder
}

// extractWebIdentityIssuer attempts to extract the "iss" claim from a JWT token's
// payload without validating the signature. Returns an empty string if extraction fails.
func extractWebIdentityIssuer(token string) string {
	claims := parseJWTPayloadClaims(token)
	if claims == nil {
		return ""
	}

	if iss, ok := claims[jwtClaimIss].(string); ok && iss != "" {
		return iss
	}

	return ""
}

// extractWebIdentityAudience attempts to extract the "aud" claim from a JWT token's
// payload without validating the signature. If the aud is an array, the first element is used.
// If extraction fails, an empty string is returned.
func extractWebIdentityAudience(token string) string {
	claims := parseJWTPayloadClaims(token)
	if claims == nil {
		return ""
	}

	switch v := claims[jwtClaimAud].(type) {
	case string:
		return v
	case []any:
		if len(v) > 0 {
			if s, ok := v[0].(string); ok {
				return s
			}
		}
	}

	return ""
}

// extractWebIdentitySourceIdentity attempts to extract the jwtClaimSourceIdentity
// custom claim from a JWT token's payload without validating the signature.
// Returns an empty string if extraction fails or the claim is absent — AWS
// derives AssumeRoleWithWebIdentity's SourceIdentity from a claim added to the
// token by the identity provider (see jwtClaimSourceIdentity for the exact
// convention this emulator uses).
func extractWebIdentitySourceIdentity(token string) string {
	claims := parseJWTPayloadClaims(token)
	if claims == nil {
		return ""
	}

	if v, ok := claims[jwtClaimSourceIdentity].(string); ok {
		return v
	}

	return ""
}

// extractWebIdentityTags attempts to extract the jwtClaimTags custom claim
// (a JSON object of tag key -> value strings) from a JWT token's payload
// without validating the signature. Returns nil if extraction fails or the
// claim is absent. AWS derives AssumeRoleWithWebIdentity's session tags from a
// claim in the token rather than a separate request parameter (see
// jwtClaimTags).
func extractWebIdentityTags(token string) []Tag {
	claims := parseJWTPayloadClaims(token)
	if claims == nil {
		return nil
	}

	rawTags, ok := claims[jwtClaimTags].(map[string]any)
	if !ok {
		return nil
	}

	keys := make([]string, 0, len(rawTags))
	for k := range rawTags {
		keys = append(keys, k)
	}

	slices.Sort(keys)

	tags := make([]Tag, 0, len(rawTags))

	for _, k := range keys {
		if v, isStr := rawTags[k].(string); isStr {
			tags = append(tags, Tag{Key: k, Value: v})
		}
	}

	return tags
}

// resolveWebIdentityProvider resolves the OIDC provider and audience from the token and input.
// The providerID, when non-empty, overrides the issuer extracted from the JWT.
func resolveWebIdentityProvider(token, providerID string) (string, string) {
	issuer := extractWebIdentityIssuer(token)
	if providerID != "" {
		issuer = providerID
	}

	provider := issuer
	if provider == "" {
		provider = "cognito-identity.amazonaws.com"
	}

	audience := extractWebIdentityAudience(token)
	if audience == "" {
		audience = provider
	}

	return provider, audience
}

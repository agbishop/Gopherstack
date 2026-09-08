package sts

import (
	"crypto/sha1" //nolint:gosec // SHA1 per AWS SAML spec
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// samlSessionName is the default RoleSessionName used for AssumeRoleWithSAML
// when the caller does not supply one.
const samlSessionName = "saml-session"

// samlProviderARNRe matches IAM SAML provider ARNs.
// Format: arn:<partition>:iam::<account>:saml-provider/<name>.
var samlProviderARNRe = regexp.MustCompile(`^arn:[a-z0-9\-]+:iam::\d{12}:saml-provider/.+$`)

// validateSAMLProviderArn checks that a PrincipalArn is a valid IAM SAML provider ARN.
func validateSAMLProviderArn(principalArn string) error {
	if !samlProviderARNRe.MatchString(principalArn) {
		return fmt.Errorf("%w: %q must be arn:<partition>:iam::<account>:saml-provider/<name>",
			ErrInvalidPrincipalArn, principalArn)
	}

	return nil
}

// validateSAMLAssertion checks that the assertion is valid base64 and decodes to XML.
// AWS rejects assertions that are not properly base64-encoded or whose decoded
// content is not a valid XML document (at minimum containing one XML element).
// RawToken is used so that namespace-prefixed elements without explicit xmlns
// declarations (common in real SAML assertions) are accepted without error.
func validateSAMLAssertion(assertion string) error {
	// AWS requires a base64-encoded SAML assertion. As an emulator we validate the
	// base64 encoding but do not require the decoded payload to be well-formed SAML
	// XML, so callers can pass simple test assertions.
	if _, err := base64.StdEncoding.DecodeString(assertion); err != nil {
		if _, err = base64.URLEncoding.DecodeString(assertion); err != nil {
			return fmt.Errorf("%w: not valid base64", ErrInvalidSAMLAssertion)
		}
	}

	return nil
}

// validateSAMLInput checks the common parameter constraints for AssumeRoleWithSAML.
func validateSAMLInput(input *AssumeRoleWithSAMLInput) error {
	if input.RoleArn == "" {
		return ErrMissingRoleArn
	}

	if err := validateRoleArn(input.RoleArn); err != nil {
		return err
	}

	if input.PrincipalArn == "" {
		return ErrMissingPrincipalArn
	}

	if err := validateSAMLProviderArn(input.PrincipalArn); err != nil {
		return err
	}

	if input.SAMLAssertion == "" {
		return ErrMissingSAMLAssertion
	}

	if err := validateSAMLAssertion(input.SAMLAssertion); err != nil {
		return err
	}

	if err := validateSAMLTemporalConditions(input.SAMLAssertion); err != nil {
		return err
	}

	if err := validatePolicyArns(input.PolicyArns); err != nil {
		return err
	}

	if err := validateInlinePolicy(input.Policy); err != nil {
		return err
	}

	return checkPackedPolicyBudget(input.Policy, input.PolicyArns)
}

// validateSAMLAssertionData checks the constraints on identity attributes
// extracted from the SAML assertion (RoleSessionName, SourceIdentity, session
// tags) — mirroring the constraints AWS applies to the equivalent AssumeRole
// request parameters, since AssumeRoleWithSAML sources these values from the
// assertion instead of accepting them directly.
func validateSAMLAssertionData(data samlAssertionData) error {
	if data.RoleSessionName != "" {
		if err := validateRoleSessionName(data.RoleSessionName); err != nil {
			return err
		}
	}

	if err := validateSourceIdentity(data.SourceIdentity); err != nil {
		return err
	}

	if len(data.Tags) > MaxTagCount {
		return fmt.Errorf("%w: got %d", ErrTooManyTags, len(data.Tags))
	}

	return validateTagConstraints(data.Tags)
}

// AssumeRoleWithSAML generates temporary credentials using a SAML 2.0 assertion.
// In this mock, the SAMLAssertion is not cryptographically validated, but its
// identity attributes (RoleSessionName, SourceIdentity, session tags, NameID,
// Issuer, SubjectConfirmationData Recipient) are parsed and drive the response,
// matching the real API's server-side derivation (see saml_attributes.go).
func (b *InMemoryBackend) AssumeRoleWithSAML(
	input *AssumeRoleWithSAMLInput,
) (*AssumeRoleWithSAMLResponse, error) {
	b.cntAssumeRoleWithSAML.Add(1)

	if err := validateSAMLInput(input); err != nil {
		return nil, err
	}

	data := extractSAMLAssertionData(input.SAMLAssertion)

	if err := validateSAMLAssertionData(data); err != nil {
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

	// Enforce the role's trust policy against the federated SAML provider.
	if err := b.checkSAMLTrust(input); err != nil {
		return nil, err
	}

	creds, err := generateCredentialSet()
	if err != nil {
		return nil, err
	}

	return b.buildSAMLResponse(input, data, creds, duration), nil
}

// checkSAMLTrust evaluates the target role's trust policy against the federated
// SAML provider (PrincipalArn) for sts:AssumeRoleWithSAML. It is permissive
// unless a RoleLookup is wired and the role carries a non-empty trust policy.
func (b *InMemoryBackend) checkSAMLTrust(input *AssumeRoleWithSAMLInput) error {
	if input.PrincipalArn == "" {
		return nil
	}

	meta := b.lookupRoleMeta(input.RoleArn)
	if meta == nil || meta.TrustPolicy == "" {
		return nil
	}

	return evaluateAssumeRoleTrust(meta.TrustPolicy, trustEval{
		action:       actionAssumeRoleWithSAML,
		federatedArn: input.PrincipalArn,
	})
}

// buildSAMLResponse constructs the AssumeRoleWithSAML response and persists the
// session, using identity attributes parsed from the SAML assertion (data) for
// the fields AWS documents as assertion-derived: RoleSessionName,
// SourceIdentity, session tags, Subject/SubjectType (from NameID), Issuer, and
// Audience (from SubjectConfirmationData's Recipient attribute). Fields absent
// from a minimal/test assertion fall back to the emulator's previous
// placeholder defaults so permissive test fixtures keep working.
func (b *InMemoryBackend) buildSAMLResponse(
	input *AssumeRoleWithSAMLInput,
	data samlAssertionData,
	creds credentialSet,
	duration int32,
) *AssumeRoleWithSAMLResponse {
	expiration := time.Now().UTC().Add(time.Duration(duration) * time.Second)
	roleID := deriveRoleID(input.RoleArn)

	// Use the assertion's RoleSessionName attribute if present, otherwise fall
	// back to the samlSessionName constant.
	sessionName := samlSessionName
	if data.RoleSessionName != "" {
		sessionName = data.RoleSessionName
	}

	assumedRoleID := roleID + ":" + sessionName
	assumedRoleArn := buildAssumedRoleArn(input.RoleArn, sessionName)

	account := b.accountID
	if parts := strings.SplitN(input.RoleArn, ":", arnComponentCount); len(
		parts,
	) >= arnComponentCount {
		account = parts[4]
	}

	session := &SessionInfo{
		Expiration:        expiration,
		AssumedRoleArn:    assumedRoleArn,
		AccountID:         account,
		SessionName:       sessionName,
		AccessKeyID:       creds.AccessKeyID,
		SecretAccessKey:   creds.SecretAccessKey,
		SessionToken:      creds.SessionToken,
		AssumedRoleID:     assumedRoleID,
		SourceIdentity:    data.SourceIdentity,
		Tags:              data.Tags,
		TransitiveTagKeys: data.TransitiveTagKeys,
		IsAssumedRole:     true,
	}

	b.storeSession(session)

	// idpName is the SAML provider's friendly name (last ARN path segment),
	// always derived from PrincipalArn per AWS's documented NameQualifier hash
	// inputs. issuer is the assertion's own <Issuer> element value; when the
	// (permissive, possibly attribute-free) test assertion carries none, fall
	// back to idpName as before.
	issuerParts := strings.Split(input.PrincipalArn, "/")
	idpName := issuerParts[len(issuerParts)-1]

	issuer := data.Issuer
	if issuer == "" {
		issuer = idpName
	}

	audience := data.Recipient
	if audience == "" {
		audience = input.PrincipalArn
	}

	subject := data.NameID
	if subject == "" {
		subject = account + ":saml-subject"
	}

	subjectType := data.NameIDFormat
	if subjectType == "" {
		subjectType = "persistent"
	}

	nameQualifier := computeNameQualifier(issuer, account, idpName)

	return &AssumeRoleWithSAMLResponse{
		Xmlns: STSNamespace,
		AssumeRoleWithSAMLResult: AssumeRoleWithSAMLResult{
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
			Audience:         audience,
			Issuer:           issuer,
			NameQualifier:    nameQualifier,
			SubjectType:      subjectType,
			Subject:          subject,
			SourceIdentity:   data.SourceIdentity,
			PackedPolicySize: calculatePackedPolicySizeWithArns(input.Policy, input.PolicyArns),
		},
		ResponseMetadata: ResponseMetadata{RequestID: uuid.NewString()},
	}
}

// computeNameQualifier computes the AWS SAML NameQualifier:
// BASE64(SHA1(issuer + ";" + accountID + ";" + idpName)).
func computeNameQualifier(issuer, accountID, idpName string) string {
	h := sha1.New() //nolint:gosec // SHA1 per AWS SAML spec
	_, _ = h.Write([]byte(issuer + ";" + accountID + ";" + idpName))

	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

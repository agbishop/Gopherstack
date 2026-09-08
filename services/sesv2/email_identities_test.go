package sesv2_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/sesv2"
)

// ─── email identity attribute round-trips (HTTP) ────────────────────────────

// TestPutEmailIdentityConfigurationSetAttributes associates a config set with an identity.
func TestPutEmailIdentityConfigurationSetAttributes(t *testing.T) {
	t.Parallel()

	h, backend := newSESv2TestHandler(t)

	_, err := backend.CreateEmailIdentity("cs-attr@example.com", "", nil)
	require.NoError(t, err)

	rec := doReq(t, h, http.MethodPut,
		"/v2/email/identities/cs-attr%40example.com/configuration-set",
		map[string]any{"ConfigurationSetName": "my-cs"})
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = doReq(t, h, http.MethodGet, "/v2/email/identities/cs-attr%40example.com", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "my-cs", out["ConfigurationSetName"])
}

// TestPutEmailIdentityConfigurationSetAttributesNotFound returns 404 for missing identity.
func TestPutEmailIdentityConfigurationSetAttributesNotFound(t *testing.T) {
	t.Parallel()

	h, _ := newSESv2TestHandler(t)

	rec := doReq(t, h, http.MethodPut,
		"/v2/email/identities/no-such%40example.com/configuration-set",
		map[string]any{"ConfigurationSetName": "my-cs"})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestPutEmailIdentityDkimAttributes toggles DKIM signing.
func TestPutEmailIdentityDkimAttributes(t *testing.T) {
	t.Parallel()

	h, backend := newSESv2TestHandler(t)

	_, err := backend.CreateEmailIdentity("dkim@example.com", "", nil)
	require.NoError(t, err)

	// Disable DKIM signing.
	rec := doReq(t, h, http.MethodPut,
		"/v2/email/identities/dkim%40example.com/dkim",
		map[string]any{"SigningEnabled": false})
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = doReq(t, h, http.MethodGet, "/v2/email/identities/dkim%40example.com", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	dkim, ok := out["DkimAttributes"].(map[string]any)
	require.True(t, ok, "DkimAttributes should be present")
	assert.Equal(t, false, dkim["SigningEnabled"])
}

// TestPutEmailIdentityDkimAttributesNotFound returns 404 for missing identity.
func TestPutEmailIdentityDkimAttributesNotFound(t *testing.T) {
	t.Parallel()

	h, _ := newSESv2TestHandler(t)

	rec := doReq(t, h, http.MethodPut,
		"/v2/email/identities/no-such%40example.com/dkim",
		map[string]any{"SigningEnabled": true})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestPutEmailIdentityDkimSigningAttributes tests PUT /v2/email/identities/{identity}/dkim/signing.
func TestPutEmailIdentityDkimSigningAttributes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		identity     string
		wantDkimStat string
		wantStatus   int
		isDomain     bool
		createFirst  bool
		wantTokens   bool
	}{
		{
			name:         "existing_domain_identity",
			identity:     "example.com",
			isDomain:     true,
			createFirst:  true,
			wantStatus:   http.StatusOK,
			wantDkimStat: "SUCCESS",
			wantTokens:   true,
		},
		{
			name:         "existing_email_identity",
			identity:     "user@example.com",
			isDomain:     false,
			createFirst:  true,
			wantStatus:   http.StatusOK,
			wantDkimStat: "SUCCESS",
			wantTokens:   false,
		},
		{
			name:        "missing_identity",
			identity:    "missing@example.com",
			createFirst: false,
			wantStatus:  http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, backend := newSESv2TestHandler(t)
			if tt.createFirst {
				_, err := backend.CreateEmailIdentity(tt.identity, "", nil)
				require.NoError(t, err)
			}

			rec := doReq(t, h, http.MethodPut,
				"/v2/email/identities/"+url.PathEscape(tt.identity)+"/dkim/signing",
				map[string]any{})
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				var resp struct {
					DkimStatus string   `json:"DkimStatus"`
					DkimTokens []string `json:"DkimTokens"`
				}
				err := json.Unmarshal(rec.Body.Bytes(), &resp)
				require.NoError(t, err)
				assert.Equal(t, tt.wantDkimStat, resp.DkimStatus)
				if tt.wantTokens {
					assert.Len(t, resp.DkimTokens, 3)
				} else {
					assert.Empty(t, resp.DkimTokens)
				}
			}
		})
	}
}

// TestPutEmailIdentityFeedbackAttributes toggles feedback forwarding.
func TestPutEmailIdentityFeedbackAttributes(t *testing.T) {
	t.Parallel()

	h, backend := newSESv2TestHandler(t)

	_, err := backend.CreateEmailIdentity("feedback@example.com", "", nil)
	require.NoError(t, err)

	// Disable feedback forwarding.
	rec := doReq(t, h, http.MethodPut,
		"/v2/email/identities/feedback%40example.com/feedback",
		map[string]any{"EmailForwardingEnabled": false})
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = doReq(t, h, http.MethodGet, "/v2/email/identities/feedback%40example.com", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, false, out["FeedbackForwardingStatus"])
}

// TestPutEmailIdentityFeedbackAttributesNotFound returns 404 for missing identity.
func TestPutEmailIdentityFeedbackAttributesNotFound(t *testing.T) {
	t.Parallel()

	h, _ := newSESv2TestHandler(t)

	rec := doReq(t, h, http.MethodPut,
		"/v2/email/identities/no-such%40example.com/feedback",
		map[string]any{"EmailForwardingEnabled": false})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestPutEmailIdentityMailFromAttributes stores mail-from domain.
func TestPutEmailIdentityMailFromAttributes(t *testing.T) {
	t.Parallel()

	h, backend := newSESv2TestHandler(t)

	_, err := backend.CreateEmailIdentity("mailfrom@example.com", "", nil)
	require.NoError(t, err)

	rec := doReq(t, h, http.MethodPut,
		"/v2/email/identities/mailfrom%40example.com/mail-from",
		map[string]any{
			"MailFromDomain":      "bounce.example.com",
			"BehaviorOnMxFailure": "REJECT_MESSAGE",
		})
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = doReq(t, h, http.MethodGet, "/v2/email/identities/mailfrom%40example.com", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	mailFrom, ok := out["MailFromAttributes"].(map[string]any)
	require.True(t, ok, "MailFromAttributes should be present after setting mail-from domain")
	assert.Equal(t, "bounce.example.com", mailFrom["MailFromDomain"])
	assert.Equal(t, "REJECT_MESSAGE", mailFrom["BehaviorOnMxFailure"])
	assert.Equal(t, "PENDING", mailFrom["MailFromDomainStatus"])
}

// TestPutEmailIdentityMailFromAttributesNotFound returns 404 for missing identity.
func TestPutEmailIdentityMailFromAttributesNotFound(t *testing.T) {
	t.Parallel()

	h, _ := newSESv2TestHandler(t)

	rec := doReq(t, h, http.MethodPut,
		"/v2/email/identities/no-such%40example.com/mail-from",
		map[string]any{"MailFromDomain": "bounce.example.com"})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestPutEmailIdentityMailFromClear clears mail-from domain when empty string sent.
func TestPutEmailIdentityMailFromClear(t *testing.T) {
	t.Parallel()

	h, backend := newSESv2TestHandler(t)

	_, err := backend.CreateEmailIdentity("mfclear@example.com", "", nil)
	require.NoError(t, err)

	// Set a domain first.
	doReq(t, h, http.MethodPut,
		"/v2/email/identities/mfclear%40example.com/mail-from",
		map[string]any{"MailFromDomain": "bounce.example.com"})

	// Clear by sending empty string.
	doReq(t, h, http.MethodPut,
		"/v2/email/identities/mfclear%40example.com/mail-from",
		map[string]any{"MailFromDomain": ""})

	rec := doReq(t, h, http.MethodGet, "/v2/email/identities/mfclear%40example.com", nil)
	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	// MailFromAttributes should be absent when domain is empty.
	assert.Nil(t, out["MailFromAttributes"])
}

// ─── GetEmailIdentity / CreateEmailIdentity full-data tests ─────────────────

// TestGetEmailIdentityDefaults verifies default values on a fresh identity.
func TestGetEmailIdentityDefaults(t *testing.T) {
	t.Parallel()

	h, backend := newSESv2TestHandler(t)

	_, err := backend.CreateEmailIdentity("defaults@example.com", "", nil)
	require.NoError(t, err)

	rec := doReq(t, h, http.MethodGet, "/v2/email/identities/defaults%40example.com", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	assert.Equal(t, "defaults@example.com", out["EmailIdentity"])
	assert.Equal(t, "EMAIL_ADDRESS", out["IdentityType"])
	assert.Equal(t, true, out["VerifiedForSendingStatus"])
	assert.Equal(t, true, out["FeedbackForwardingStatus"])
	assert.Equal(t, "SUCCESS", out["VerificationStatus"])

	dkim, ok := out["DkimAttributes"].(map[string]any)
	require.True(t, ok, "DkimAttributes should be present for email address identity")
	assert.Equal(t, true, dkim["SigningEnabled"])
	assert.Equal(t, "SUCCESS", dkim["Status"])
}

// TestGetEmailIdentityDomainHasDkimTokens verifies domain identities have DKIM tokens.
func TestGetEmailIdentityDomainHasDkimTokens(t *testing.T) {
	t.Parallel()

	h, backend := newSESv2TestHandler(t)

	_, err := backend.CreateEmailIdentity("example.com", "", nil)
	require.NoError(t, err)

	rec := doReq(t, h, http.MethodGet, "/v2/email/identities/example.com", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	assert.Equal(t, "DOMAIN", out["IdentityType"])

	dkim, ok := out["DkimAttributes"].(map[string]any)
	require.True(t, ok)

	tokens, ok := dkim["Tokens"].([]any)
	require.True(t, ok, "Domain identity should have DKIM tokens")
	assert.Len(t, tokens, 3, "Should have exactly 3 DKIM tokens")

	for _, tok := range tokens {
		s, isStr := tok.(string)
		require.True(t, isStr)
		assert.Len(t, s, 24, "Each DKIM token should be 24 chars")
	}
}

// TestGetEmailIdentityEmailAddressNoDkimTokens verifies email addresses have no DKIM tokens.
func TestGetEmailIdentityEmailAddressNoDkimTokens(t *testing.T) {
	t.Parallel()

	h, backend := newSESv2TestHandler(t)

	_, err := backend.CreateEmailIdentity("addr@example.com", "", nil)
	require.NoError(t, err)

	rec := doReq(t, h, http.MethodGet, "/v2/email/identities/addr%40example.com", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	dkim, ok := out["DkimAttributes"].(map[string]any)
	require.True(t, ok)
	// Email address identities should have no tokens.
	assert.Nil(t, dkim["Tokens"])
}

// TestCreateEmailIdentityWithConfigSet stores config set name.
func TestCreateEmailIdentityWithConfigSet(t *testing.T) {
	t.Parallel()

	h, _ := newSESv2TestHandler(t)

	rec := doReq(t, h, http.MethodPost, "/v2/email/identities", map[string]any{
		"EmailIdentity":        "tagged@example.com",
		"ConfigurationSetName": "my-cs",
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = doReq(t, h, http.MethodGet, "/v2/email/identities/tagged%40example.com", nil)
	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "my-cs", out["ConfigurationSetName"])
}

// TestCreateEmailIdentityWithTags stores tags accessible via GetEmailIdentity.
func TestCreateEmailIdentityWithTags(t *testing.T) {
	t.Parallel()

	h, _ := newSESv2TestHandler(t)

	rec := doReq(t, h, http.MethodPost, "/v2/email/identities", map[string]any{
		"EmailIdentity": "tagged2@example.com",
		"Tags": []map[string]any{
			{"Key": "env", "Value": "prod"},
			{"Key": "team", "Value": "email"},
		},
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = doReq(t, h, http.MethodGet, "/v2/email/identities/tagged2%40example.com", nil)
	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	tags, ok := out["Tags"].([]any)
	require.True(t, ok, "Tags should be returned")
	assert.Len(t, tags, 2)
}

// TestCreateDomainIdentityReturnsDkimAttributes verifies DkimAttributes in create response.
func TestCreateDomainIdentityReturnsDkimAttributes(t *testing.T) {
	t.Parallel()

	h, _ := newSESv2TestHandler(t)

	rec := doReq(t, h, http.MethodPost, "/v2/email/identities", map[string]any{
		"EmailIdentity": "domain-create.com",
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	// Domain identities should return DkimAttributes with tokens in create response.
	dkim, ok := out["DkimAttributes"].(map[string]any)
	require.True(t, ok, "DkimAttributes should be returned for domain in CreateEmailIdentity")
	tokens, ok := dkim["Tokens"].([]any)
	require.True(t, ok)
	assert.Len(t, tokens, 3)
	assert.Equal(t, "AWS_SES", dkim["SigningAttributesOrigin"])
}

// TestCreateEmailAddressDoesNotReturnDkimAttributes verifies no DkimAttributes for email.
func TestCreateEmailAddressDoesNotReturnDkimAttributes(t *testing.T) {
	t.Parallel()

	h, _ := newSESv2TestHandler(t)

	rec := doReq(t, h, http.MethodPost, "/v2/email/identities", map[string]any{
		"EmailIdentity": "addr-create@example.com",
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	// Email addresses should not return DkimAttributes in create response.
	assert.Nil(t, out["DkimAttributes"], "Email address CreateEmailIdentity should omit DkimAttributes")
}

// TestGetEmailIdentityPoliciesIncludedInGet verifies policies are returned inline.
func TestGetEmailIdentityPoliciesIncludedInGet(t *testing.T) {
	t.Parallel()

	h, backend := newSESv2TestHandler(t)

	_, err := backend.CreateEmailIdentity("policy-get@example.com", "", nil)
	require.NoError(t, err)

	err = backend.CreateEmailIdentityPolicy("policy-get@example.com", "my-policy", `{"Version":"2012-10-17"}`)
	require.NoError(t, err)

	rec := doReq(t, h, http.MethodGet, "/v2/email/identities/policy-get%40example.com", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	policies, ok := out["Policies"].(map[string]any)
	require.True(t, ok, "Policies should be returned in GetEmailIdentity")
	assert.Contains(t, policies, "my-policy")
}

// ─── email identity policy creation ──────────────────────────────────────────

// TestCreateEmailIdentityPolicy tests policy creation.
func TestCreateEmailIdentityPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setup      func(*sesv2.InMemoryBackend)
		identity   string
		policyName string
		wantErr    bool
	}{
		{
			name: "happy_path",
			setup: func(b *sesv2.InMemoryBackend) {
				_, _ = b.CreateEmailIdentity("owner@example.com", "", nil)
			},
			identity:   "owner@example.com",
			policyName: "my-policy",
		},
		{
			name:       "identity_not_found",
			setup:      func(*sesv2.InMemoryBackend) {},
			identity:   "no-such@example.com",
			policyName: "my-policy",
			wantErr:    true,
		},
		{
			name: "duplicate_policy",
			setup: func(b *sesv2.InMemoryBackend) {
				_, _ = b.CreateEmailIdentity("owner@example.com", "", nil)
				_ = b.CreateEmailIdentityPolicy("owner@example.com", "my-policy", "{}")
			},
			identity:   "owner@example.com",
			policyName: "my-policy",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := sesv2.NewInMemoryBackend()
			tt.setup(backend)

			err := backend.CreateEmailIdentityPolicy(
				tt.identity,
				tt.policyName,
				`{"Version":"2012-10-17"}`,
			)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
		})
	}
}

// TestCreateEmailIdentityPolicyHTTP tests policy creation via HTTP.
func TestCreateEmailIdentityPolicyHTTP(t *testing.T) {
	t.Parallel()

	h, backend := newSESv2TestHandler(t)
	_, err := backend.CreateEmailIdentity("owner@example.com", "", nil)
	require.NoError(t, err)

	body := map[string]any{"Policy": `{"Version":"2012-10-17"}`}
	rec := doReq(
		t,
		h,
		http.MethodPost,
		"/v2/email/identities/owner@example.com/policies/my-policy",
		body,
	)
	assert.Equal(t, http.StatusOK, rec.Code)
}

// ─── email identity backend unit tests ───────────────────────────────────────

// TestBackendPutEmailIdentityConfigSetAttributes tests backend method.
func TestBackendPutEmailIdentityConfigSetAttributes(t *testing.T) {
	t.Parallel()

	backend := sesv2.NewInMemoryBackend()

	_, err := backend.CreateEmailIdentity("csattr@example.com", "", nil)
	require.NoError(t, err)

	err = backend.PutEmailIdentityConfigurationSetAttributes("csattr@example.com", "my-cs")
	require.NoError(t, err)

	ei, err := backend.GetEmailIdentity("csattr@example.com")
	require.NoError(t, err)
	assert.Equal(t, "my-cs", ei.ConfigurationSetName)
}

// TestBackendPutEmailIdentityDkimAttributes tests backend method.
func TestBackendPutEmailIdentityDkimAttributes(t *testing.T) {
	t.Parallel()

	backend := sesv2.NewInMemoryBackend()

	_, err := backend.CreateEmailIdentity("dkim@example.com", "", nil)
	require.NoError(t, err)

	// Default is true; disable.
	err = backend.PutEmailIdentityDkimAttributes("dkim@example.com", false)
	require.NoError(t, err)

	ei, err := backend.GetEmailIdentity("dkim@example.com")
	require.NoError(t, err)
	assert.False(t, ei.DkimSigningEnabled)
}

// TestBackendPutEmailIdentityDkimSigningAttributes tests backend method.
func TestBackendPutEmailIdentityDkimSigningAttributes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		identity    string
		createFirst bool
		wantErr     bool
	}{
		{
			name:        "existing_identity",
			identity:    "dkim-signing@example.com",
			createFirst: true,
			wantErr:     false,
		},
		{
			name:        "not_found",
			identity:    "no-such@example.com",
			createFirst: false,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := sesv2.NewInMemoryBackend()
			if tt.createFirst {
				_, err := backend.CreateEmailIdentity(tt.identity, "", nil)
				require.NoError(t, err)
			}

			ei, err := backend.PutEmailIdentityDkimSigningAttributes(tt.identity)
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, ei)
			} else {
				require.NoError(t, err)
				require.NotNil(t, ei)
				assert.Equal(t, tt.identity, ei.Identity)
			}
		})
	}
}

// TestBackendPutEmailIdentityFeedbackAttributes tests backend method.
func TestBackendPutEmailIdentityFeedbackAttributes(t *testing.T) {
	t.Parallel()

	backend := sesv2.NewInMemoryBackend()

	_, err := backend.CreateEmailIdentity("fb@example.com", "", nil)
	require.NoError(t, err)

	// Default is true; disable.
	err = backend.PutEmailIdentityFeedbackAttributes("fb@example.com", false)
	require.NoError(t, err)

	ei, err := backend.GetEmailIdentity("fb@example.com")
	require.NoError(t, err)
	assert.False(t, ei.FeedbackForwarding)
}

// TestBackendPutEmailIdentityMailFromAttributes tests backend method.
func TestBackendPutEmailIdentityMailFromAttributes(t *testing.T) {
	t.Parallel()

	backend := sesv2.NewInMemoryBackend()

	_, err := backend.CreateEmailIdentity("mf@example.com", "", nil)
	require.NoError(t, err)

	err = backend.PutEmailIdentityMailFromAttributes("mf@example.com", "bounce.example.com", "REJECT_MESSAGE")
	require.NoError(t, err)

	ei, err := backend.GetEmailIdentity("mf@example.com")
	require.NoError(t, err)
	assert.Equal(t, "bounce.example.com", ei.MailFromDomain)
	assert.Equal(t, "PENDING", ei.MailFromDomainStatus)
	assert.Equal(t, "REJECT_MESSAGE", ei.BehaviorOnMxFailure)
}

// TestBackendPutEmailIdentityMailFromAttributesDefaultBehavior tests default behavior.
func TestBackendPutEmailIdentityMailFromAttributesDefaultBehavior(t *testing.T) {
	t.Parallel()

	backend := sesv2.NewInMemoryBackend()

	_, err := backend.CreateEmailIdentity("mfdefault@example.com", "", nil)
	require.NoError(t, err)

	// Empty behavior should default to USE_DEFAULT_VALUE.
	err = backend.PutEmailIdentityMailFromAttributes("mfdefault@example.com", "bounce.example.com", "")
	require.NoError(t, err)

	ei, err := backend.GetEmailIdentity("mfdefault@example.com")
	require.NoError(t, err)
	assert.Equal(t, "USE_DEFAULT_VALUE", ei.BehaviorOnMxFailure)
}

// TestBackendCreateEmailIdentityWithConfigSet tests config set name storage.
func TestBackendCreateEmailIdentityWithConfigSet(t *testing.T) {
	t.Parallel()

	backend := sesv2.NewInMemoryBackend()

	ei, err := backend.CreateEmailIdentity("cs@example.com", "my-config-set", nil)
	require.NoError(t, err)
	assert.Equal(t, "my-config-set", ei.ConfigurationSetName)

	fetched, err := backend.GetEmailIdentity("cs@example.com")
	require.NoError(t, err)
	assert.Equal(t, "my-config-set", fetched.ConfigurationSetName)
}

// TestBackendCreateEmailIdentityWithTags tests tag storage.
func TestBackendCreateEmailIdentityWithTags(t *testing.T) {
	t.Parallel()

	backend := sesv2.NewInMemoryBackend()

	tags := map[string]string{"env": "test", "service": "sesv2"}
	ei, err := backend.CreateEmailIdentity("tags@example.com", "", tags)
	require.NoError(t, err)
	assert.Equal(t, "test", ei.Tags["env"])

	fetched, err := backend.GetEmailIdentity("tags@example.com")
	require.NoError(t, err)
	assert.Equal(t, "sesv2", fetched.Tags["service"])
}

// TestBackendCreateEmailIdentityDomainDkimTokens tests DKIM token generation.
func TestBackendCreateEmailIdentityDomainDkimTokens(t *testing.T) {
	t.Parallel()

	backend := sesv2.NewInMemoryBackend()

	ei, err := backend.CreateEmailIdentity("dkim-tokens.com", "", nil)
	require.NoError(t, err)

	assert.Equal(t, "DOMAIN", ei.IdentityType)
	assert.Len(t, ei.DkimTokens, 3, "Domain should have 3 DKIM tokens")

	for _, tok := range ei.DkimTokens {
		assert.Len(t, tok, 24, "Each token should be 24 chars")
	}

	assert.Equal(t, "SUCCESS", ei.DkimStatus)
	assert.True(t, ei.DkimSigningEnabled)
}

// TestBackendCreateEmailIdentityEmailNoDkimTokens verifies email has no DKIM tokens.
func TestBackendCreateEmailIdentityEmailNoDkimTokens(t *testing.T) {
	t.Parallel()

	backend := sesv2.NewInMemoryBackend()

	ei, err := backend.CreateEmailIdentity("no-tokens@example.com", "", nil)
	require.NoError(t, err)

	assert.Equal(t, "EMAIL_ADDRESS", ei.IdentityType)
	assert.Empty(t, ei.DkimTokens, "Email address should have no DKIM tokens")
}

// TestBackendIdentityDefaultsVerificationStatus tests default verification status.
func TestBackendIdentityDefaultsVerificationStatus(t *testing.T) {
	t.Parallel()

	backend := sesv2.NewInMemoryBackend()

	ei, err := backend.CreateEmailIdentity("verify@example.com", "", nil)
	require.NoError(t, err)

	assert.Equal(t, "SUCCESS", ei.VerificationStatus)
	assert.True(t, ei.FeedbackForwarding)
	assert.True(t, ei.VerifiedForSending)
}

// TestGetEmailIdentityDeepCopy verifies GetEmailIdentity returns a copy.
func TestGetEmailIdentityDeepCopy(t *testing.T) {
	t.Parallel()

	backend := sesv2.NewInMemoryBackend()
	_, err := backend.CreateEmailIdentity("copy@example.com", "", nil)
	require.NoError(t, err)

	ei, err := backend.GetEmailIdentity("copy@example.com")
	require.NoError(t, err)
	require.NotNil(t, ei)

	// Modifying the returned pointer should not affect the stored value.
	ei.Identity = "mutated@example.com"

	ei2, err := backend.GetEmailIdentity("copy@example.com")
	require.NoError(t, err)
	assert.Equal(t, "copy@example.com", ei2.Identity)
}

// ─── email identity policy CRUD (HTTP) ──────────────────────────────────────

// TestCreateAndGetEmailIdentityPolicies tests creating a policy and reading it back
// via GetEmailIdentityPolicies.
func TestCreateAndGetEmailIdentityPolicies(t *testing.T) {
	t.Parallel()

	h := newHandler()

	doRequest(t, h, http.MethodPost, "/v2/email/identities", map[string]any{
		"EmailIdentity": "policy@example.com",
	})

	doRequest(
		t,
		h,
		http.MethodPost,
		"/v2/email/identities/policy@example.com/policies/MyPolicy",
		map[string]any{
			"Policy": `{"Version":"2012-10-17","Statement":[` +
				`{"Effect":"Allow","Principal":"*","Action":"ses:SendEmail","Resource":"*"}]}`,
		},
	)

	rec := doRequest(t, h, http.MethodGet, "/v2/email/identities/policy@example.com/policies", nil)
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestDeleteEmailIdentityPolicy tests the DeleteEmailIdentityPolicy operation.
func TestDeleteEmailIdentityPolicy(t *testing.T) {
	t.Parallel()

	h := newHandler()

	doRequest(t, h, http.MethodPost, "/v2/email/identities", map[string]any{
		"EmailIdentity": "delpol@example.com",
	})

	doRequest(
		t,
		h,
		http.MethodPost,
		"/v2/email/identities/delpol@example.com/policies/MyPolicy",
		map[string]any{
			"Policy": `{"Version":"2012-10-17"}`,
		},
	)

	rec := doRequest(
		t,
		h,
		http.MethodDelete,
		"/v2/email/identities/delpol@example.com/policies/MyPolicy",
		nil,
	)
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestUpdateEmailIdentityPolicy tests the UpdateEmailIdentityPolicy operation.
func TestUpdateEmailIdentityPolicy(t *testing.T) {
	t.Parallel()

	h := newHandler()

	doRequest(t, h, http.MethodPost, "/v2/email/identities", map[string]any{
		"EmailIdentity": "updpol@example.com",
	})

	doRequest(
		t,
		h,
		http.MethodPost,
		"/v2/email/identities/updpol@example.com/policies/MyPolicy",
		map[string]any{
			"Policy": `{"Version":"2012-10-17"}`,
		},
	)

	rec := doRequest(
		t,
		h,
		http.MethodPut,
		"/v2/email/identities/updpol@example.com/policies/MyPolicy",
		map[string]any{
			"Policy": `{"Version":"2012-10-17","Statement":[]}`,
		},
	)
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestPutEmailIdentityAttributesAllPaths smoke-tests every PUT attribute-writing endpoint
// under /v2/email/identities/{identity}/... in one pass.
func TestPutEmailIdentityAttributesAllPaths(t *testing.T) {
	t.Parallel()

	h := newHandler()

	doRequest(t, h, http.MethodPost, "/v2/email/identities", map[string]any{
		"EmailIdentity": "attr@example.com",
	})

	paths := []string{
		"/v2/email/identities/attr@example.com/configuration-set",
		"/v2/email/identities/attr@example.com/dkim",
		"/v2/email/identities/attr@example.com/dkim/signing",
		"/v2/email/identities/attr@example.com/feedback",
		"/v2/email/identities/attr@example.com/mail-from",
	}

	for _, path := range paths {
		rec := doRequest(t, h, http.MethodPut, path, map[string]any{})
		assert.Equal(t, http.StatusOK, rec.Code, "path=%s", path)
	}
}

// ─── core identity CRUD (HTTP) ──────────────────────────────────────────────

// TestCreateEmailIdentity tests the CreateEmailIdentity operation.
func TestCreateEmailIdentity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     map[string]any
		name     string
		wantType string
		wantCode int
	}{
		{
			name:     "creates email identity",
			body:     map[string]any{"EmailIdentity": "test@example.com"},
			wantCode: http.StatusOK,
			wantType: "EMAIL_ADDRESS",
		},
		{
			name:     "creates domain identity",
			body:     map[string]any{"EmailIdentity": "example.com"},
			wantCode: http.StatusOK,
			wantType: "DOMAIN",
		},
		{
			name:     "duplicate identity returns conflict",
			body:     map[string]any{"EmailIdentity": "dup@example.com"},
			wantCode: http.StatusConflict,
		},
		{
			name:     "empty identity returns bad request",
			body:     map[string]any{"EmailIdentity": ""},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()

			if tt.name == "duplicate identity returns conflict" {
				// Create once first.
				doRequest(
					t,
					h,
					http.MethodPost,
					"/v2/email/identities",
					map[string]any{"EmailIdentity": "dup@example.com"},
				)
			}

			rec := doRequest(t, h, http.MethodPost, "/v2/email/identities", tt.body)

			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantType != "" {
				var out map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
				assert.Equal(t, tt.wantType, out["IdentityType"])
				assert.Equal(t, true, out["VerifiedForSendingStatus"])
			}
		})
	}
}

// TestGetEmailIdentity tests the GetEmailIdentity operation via HTTP.
func TestGetEmailIdentity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		identity string
		wantCode int
	}{
		{
			name:     "gets existing identity",
			identity: "get@example.com",
			wantCode: http.StatusOK,
		},
		{
			name:     "not found returns 404",
			identity: "notfound@example.com",
			wantCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()

			if tt.wantCode == http.StatusOK {
				doRequest(
					t,
					h,
					http.MethodPost,
					"/v2/email/identities",
					map[string]any{"EmailIdentity": tt.identity},
				)
			}

			rec := doRequest(t, h, http.MethodGet, "/v2/email/identities/"+tt.identity, nil)

			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantCode == http.StatusOK {
				var out map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
				assert.Equal(t, tt.identity, out["EmailIdentity"])
			}
		})
	}
}

// TestListEmailIdentities tests the ListEmailIdentities operation via HTTP.
func TestListEmailIdentities(t *testing.T) {
	t.Parallel()

	h := newHandler()

	doRequest(
		t,
		h,
		http.MethodPost,
		"/v2/email/identities",
		map[string]any{"EmailIdentity": "alice@example.com"},
	)
	doRequest(
		t,
		h,
		http.MethodPost,
		"/v2/email/identities",
		map[string]any{"EmailIdentity": "bob@example.com"},
	)

	rec := doRequest(t, h, http.MethodGet, "/v2/email/identities", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	identities, ok := out["EmailIdentities"].([]any)
	require.True(t, ok)
	assert.Len(t, identities, 2)
}

// TestDeleteEmailIdentity tests the DeleteEmailIdentity operation via HTTP.
func TestDeleteEmailIdentity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		identity string
		wantCode int
	}{
		{
			name:     "deletes existing identity",
			identity: "del@example.com",
			wantCode: http.StatusOK,
		},
		{
			name:     "not found returns 404",
			identity: "notfound@example.com",
			wantCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()

			if tt.wantCode == http.StatusOK {
				doRequest(
					t,
					h,
					http.MethodPost,
					"/v2/email/identities",
					map[string]any{"EmailIdentity": tt.identity},
				)
			}

			rec := doRequest(t, h, http.MethodDelete, "/v2/email/identities/"+tt.identity, nil)

			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

func TestDeleteEmailIdentity_ClearsGhostStateOnRecreate(t *testing.T) {
	t.Parallel()

	h := newHandler()

	_, err := h.Backend.CreateEmailIdentity("reused@example.com", "", map[string]string{"env": "prod"})
	require.NoError(t, err)
	require.NoError(
		t,
		h.Backend.CreateEmailIdentityPolicy("reused@example.com", "my-policy", `{"Version":"2012-10-17"}`),
	)

	require.NoError(t, h.Backend.DeleteEmailIdentity("reused@example.com"))

	_, err = h.Backend.CreateEmailIdentity("reused@example.com", "", nil)
	require.NoError(t, err)

	recreated, err := h.Backend.GetEmailIdentity("reused@example.com")
	require.NoError(t, err)
	assert.Empty(t, recreated.Tags)

	policies, err := h.Backend.GetEmailIdentityPolicies("reused@example.com")
	require.NoError(t, err)
	assert.Empty(t, policies)
}

func TestEmailIdentity_DeepCopy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		mutate func(ei *sesv2.EmailIdentity)
		check  func(t *testing.T, ei *sesv2.EmailIdentity)
		name   string
	}{
		{
			name: "mutate_identity_type",
			mutate: func(ei *sesv2.EmailIdentity) {
				ei.IdentityType = "MUTATED"
			},
			check: func(t *testing.T, ei *sesv2.EmailIdentity) {
				t.Helper()
				assert.Equal(t, "DOMAIN", ei.IdentityType)
			},
		},
		{
			name: "mutate_tags",
			mutate: func(ei *sesv2.EmailIdentity) {
				if ei.Tags != nil {
					ei.Tags["k1"] = "mutated_value"
					ei.Tags["k_new"] = "new_value"
				}
			},
			check: func(t *testing.T, ei *sesv2.EmailIdentity) {
				t.Helper()
				assert.Equal(t, "v1", ei.Tags["k1"])
				assert.NotContains(t, ei.Tags, "k_new")
			},
		},
		{
			name: "mutate_dkim_tokens",
			mutate: func(ei *sesv2.EmailIdentity) {
				if len(ei.DkimTokens) > 0 {
					ei.DkimTokens[0] = "mutated_token"
				}
			},
			check: func(t *testing.T, ei *sesv2.EmailIdentity) {
				t.Helper()
				assert.NotEqual(t, "mutated_token", ei.DkimTokens[0])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := sesv2.NewInMemoryBackend()
			_, err := backend.CreateEmailIdentity("example.com", "", map[string]string{"k1": "v1"})
			require.NoError(t, err)

			ei, err := backend.GetEmailIdentity("example.com")
			require.NoError(t, err)

			tt.mutate(ei)

			ei2, err := backend.GetEmailIdentity("example.com")
			require.NoError(t, err)
			tt.check(t, ei2)
		})
	}
}

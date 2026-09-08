package cloudfront_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	cfsdk "github.com/aws/aws-sdk-go-v2/service/cloudfront"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudfront"
)

// TestFLEReferentialIntegrity verifies referential integrity
// between FLE configs, FLE profiles, and public keys.
func TestFLEReferentialIntegrity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		run  func(t *testing.T, b *cloudfront.InMemoryBackend)
		name string
	}{
		{
			name: "profile_with_unknown_public_key_rejected",
			run: func(t *testing.T, b *cloudfront.InMemoryBackend) {
				t.Helper()
				_, err := b.CreateFieldLevelEncryptionProfile("p", "", []cloudfront.EncryptionEntity{
					{PublicKeyID: "K-DOES-NOT-EXIST", ProviderID: "prov"},
				})
				require.ErrorIs(t, err, cloudfront.ErrPublicKeyNotFound)
			},
		},
		{
			name: "config_with_unknown_profile_rejected",
			run: func(t *testing.T, b *cloudfront.InMemoryBackend) {
				t.Helper()
				_, err := b.CreateFieldLevelEncryption("cfg", "", []cloudfront.FLEQueryArgProfile{
					{QueryArg: "q", ProfileID: "P-DOES-NOT-EXIST"},
				})
				require.ErrorIs(t, err, cloudfront.ErrFLEProfileNotFound)
			},
		},
		{
			name: "profile_in_use_by_config_blocks_delete",
			run: func(t *testing.T, b *cloudfront.InMemoryBackend) {
				t.Helper()
				prof, err := b.CreateFieldLevelEncryptionProfile("prof", "", nil)
				require.NoError(t, err)
				_, err = b.CreateFieldLevelEncryption("cfg", "", []cloudfront.FLEQueryArgProfile{
					{QueryArg: "q", ProfileID: prof.ID},
				})
				require.NoError(t, err)

				err = b.DeleteFieldLevelEncryptionProfile(prof.ID)
				require.ErrorIs(t, err, cloudfront.ErrFLEProfileInUse)
			},
		},
		{
			name: "profile_delete_ok_after_config_drops_ref",
			run: func(t *testing.T, b *cloudfront.InMemoryBackend) {
				t.Helper()
				prof, err := b.CreateFieldLevelEncryptionProfile("prof", "", nil)
				require.NoError(t, err)
				cfg, err := b.CreateFieldLevelEncryption("cfg", "", []cloudfront.FLEQueryArgProfile{
					{QueryArg: "q", ProfileID: prof.ID},
				})
				require.NoError(t, err)

				require.ErrorIs(t, b.DeleteFieldLevelEncryptionProfile(prof.ID), cloudfront.ErrFLEProfileInUse)

				_, err = b.UpdateFieldLevelEncryption(cfg.ID, cfg.Name, "", nil)
				require.NoError(t, err)
				require.NoError(t, b.DeleteFieldLevelEncryptionProfile(prof.ID))
			},
		},
		{
			name: "profile_roundtrips_entities",
			run: func(t *testing.T, b *cloudfront.InMemoryBackend) {
				t.Helper()
				pk, err := b.CreatePublicKey("cr", "pk", "", testRSA2048PublicKeyPEM)
				require.NoError(t, err)
				prof, err := b.CreateFieldLevelEncryptionProfile("prof", "", []cloudfront.EncryptionEntity{
					{PublicKeyID: pk.ID, ProviderID: "prov", FieldPatterns: []string{"a", "b"}},
				})
				require.NoError(t, err)

				got, err := b.GetFieldLevelEncryptionProfile(prof.ID)
				require.NoError(t, err)
				require.Len(t, got.EncryptionEntities, 1)
				assert.Equal(t, pk.ID, got.EncryptionEntities[0].PublicKeyID)
				assert.Equal(t, []string{"a", "b"}, got.EncryptionEntities[0].FieldPatterns)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.run(t, newB(t))
		})
	}
}

func TestDeleteFLERequiresIfMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		sendIfMatch bool
		wantStatus  int
	}{
		{name: "no_if_match_rejected", sendIfMatch: false, wantStatus: http.StatusPreconditionFailed},
		{name: "correct_if_match_accepted", sendIfMatch: true, wantStatus: http.StatusNoContent},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			createRec := doXML(t, h, http.MethodPost, "/2020-05-31/field-level-encryption",
				[]byte(`<FieldLevelEncryptionConfig><Comment>test</Comment></FieldLevelEncryptionConfig>`))
			require.Equal(t, http.StatusCreated, createRec.Code)

			fleID := extractXMLTag(t, createRec.Body.String())
			require.NotEmpty(t, fleID)

			etag := createRec.Header().Get("ETag")

			headers := map[string]string{}
			if tc.sendIfMatch {
				headers["If-Match"] = etag
			}

			rec := doXMLWithHeaders(t, h, http.MethodDelete,
				"/2020-05-31/field-level-encryption/"+fleID, nil, headers)
			assert.Equal(t, tc.wantStatus, rec.Code, tc.name)
		})
	}
}

// TestFieldLevelEncryptionCRUD covers the full FLE lifecycle via the HTTP handler.
func TestFieldLevelEncryptionCRUD(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup       func(*testing.T, *cloudfront.Handler) string
		check       func(*testing.T, *httptest.ResponseRecorder, string)
		headersFunc func(*testing.T, *cloudfront.Handler, string) map[string]string
		name        string
		method      string
		path        string
		body        []byte
		wantStatus  int
	}{
		{
			name:   "create_fle",
			method: http.MethodPost,
			path:   "/2020-05-31/field-level-encryption",
			body: []byte(
				`<FieldLevelEncryptionConfig>` +
					`<Comment>test fle</Comment>` +
					`<QueryArgProfileConfig><QueryArgProfiles><Items>` +
					`<QueryArgProfile><Profile>my-fle-config</Profile></QueryArgProfile>` +
					`</Items></QueryArgProfiles></QueryArgProfileConfig>` +
					`</FieldLevelEncryptionConfig>`,
			),
			setup: func(t *testing.T, _ *cloudfront.Handler) string {
				t.Helper()

				return ""
			},
			wantStatus: http.StatusCreated,
			check: func(t *testing.T, rec *httptest.ResponseRecorder, _ string) {
				t.Helper()
				assert.Contains(t, rec.Body.String(), "<FieldLevelEncryption")
				assert.NotEmpty(t, rec.Header().Get("Location"))
				assert.NotEmpty(t, rec.Header().Get("ETag"))
			},
		},
		{
			name:   "list_fle",
			method: http.MethodGet,
			path:   "/2020-05-31/field-level-encryption",
			body:   nil,
			setup: func(t *testing.T, h *cloudfront.Handler) string {
				t.Helper()
				_, err := h.Backend.CreateFieldLevelEncryption("list-fle-cfg", "comment", nil)
				require.NoError(t, err)

				return ""
			},
			wantStatus: http.StatusOK,
			check: func(t *testing.T, rec *httptest.ResponseRecorder, _ string) {
				t.Helper()
				assert.Contains(t, rec.Body.String(), "<FieldLevelEncryptionList")
				assert.Contains(t, rec.Body.String(), "<Quantity>1</Quantity>")
			},
		},
		{
			name:   "get_fle",
			method: http.MethodGet,
			path:   "",
			body:   nil,
			setup: func(t *testing.T, h *cloudfront.Handler) string {
				t.Helper()
				fle, err := h.Backend.CreateFieldLevelEncryption("get-fle-cfg", "a comment", nil)
				require.NoError(t, err)

				return "/2020-05-31/field-level-encryption/" + fle.ID
			},
			wantStatus: http.StatusOK,
			check: func(t *testing.T, rec *httptest.ResponseRecorder, _ string) {
				t.Helper()
				assert.Contains(t, rec.Body.String(), "<FieldLevelEncryption")
				assert.NotEmpty(t, rec.Header().Get("ETag"))
			},
		},
		{
			name:   "get_fle_config",
			method: http.MethodGet,
			path:   "",
			body:   nil,
			setup: func(t *testing.T, h *cloudfront.Handler) string {
				t.Helper()
				fle, err := h.Backend.CreateFieldLevelEncryption("get-fle-cfg2", "comment2", nil)
				require.NoError(t, err)

				return "/2020-05-31/field-level-encryption/" + fle.ID + "/config"
			},
			wantStatus: http.StatusOK,
			check: func(t *testing.T, rec *httptest.ResponseRecorder, _ string) {
				t.Helper()
				assert.Contains(t, rec.Body.String(), "<FieldLevelEncryption")
			},
		},
		{
			name:   "update_fle",
			method: http.MethodPut,
			path:   "",
			body:   []byte(`<FieldLevelEncryptionConfig><Comment>updated</Comment></FieldLevelEncryptionConfig>`),
			setup: func(t *testing.T, h *cloudfront.Handler) string {
				t.Helper()
				fle, err := h.Backend.CreateFieldLevelEncryption("upd-fle-cfg", "original", nil)
				require.NoError(t, err)

				return "/2020-05-31/field-level-encryption/" + fle.ID + "/config"
			},
			wantStatus: http.StatusOK,
			check: func(t *testing.T, rec *httptest.ResponseRecorder, _ string) {
				t.Helper()
				assert.Contains(t, rec.Body.String(), "<FieldLevelEncryption")
				assert.NotEmpty(t, rec.Header().Get("ETag"))
			},
		},
		{
			name:   "get_fle_not_found",
			method: http.MethodGet,
			path:   "/2020-05-31/field-level-encryption/doesnotexist",
			body:   nil,
			setup: func(t *testing.T, _ *cloudfront.Handler) string {
				t.Helper()

				return ""
			},
			wantStatus: http.StatusNotFound,
			check: func(t *testing.T, rec *httptest.ResponseRecorder, _ string) {
				t.Helper()
				assert.Contains(t, rec.Body.String(), "<Error>")
			},
		},
		{
			name:   "delete_fle",
			method: http.MethodDelete,
			path:   "",
			body:   nil,
			setup: func(t *testing.T, h *cloudfront.Handler) string {
				t.Helper()
				fle, err := h.Backend.CreateFieldLevelEncryption("del-fle-cfg", "delete me", nil)
				require.NoError(t, err)

				return "/2020-05-31/field-level-encryption/" + fle.ID
			},
			headersFunc: func(t *testing.T, h *cloudfront.Handler, path string) map[string]string {
				t.Helper()
				parts := strings.Split(strings.TrimRight(path, "/"), "/")
				id := parts[len(parts)-1]
				fle, err := h.Backend.GetFieldLevelEncryption(id)
				require.NoError(t, err)

				return map[string]string{"If-Match": fle.ETag}
			},
			wantStatus: http.StatusNoContent,
			check: func(t *testing.T, rec *httptest.ResponseRecorder, _ string) {
				t.Helper()
				assert.Empty(t, rec.Body.String())
			},
		},
		{
			name:   "delete_fle_not_found",
			method: http.MethodDelete,
			path:   "/2020-05-31/field-level-encryption/notexist",
			body:   nil,
			setup: func(t *testing.T, _ *cloudfront.Handler) string {
				t.Helper()

				return ""
			},
			wantStatus: http.StatusNotFound,
			check: func(t *testing.T, rec *httptest.ResponseRecorder, _ string) {
				t.Helper()
				assert.Contains(t, rec.Body.String(), "<Error>")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			path := tt.path
			if tt.setup != nil {
				if p := tt.setup(t, h); p != "" {
					path = p
				}
			}

			var hdrs map[string]string
			if tt.headersFunc != nil {
				hdrs = tt.headersFunc(t, h, path)
			}
			rec := doXMLWithHeaders(t, h, tt.method, path, tt.body, hdrs)

			assert.Equal(t, tt.wantStatus, rec.Code)
			if tt.check != nil {
				tt.check(t, rec, path)
			}
		})
	}
}

// TestUpdateFieldLevelEncryptionConfig_RealClient drives the real
// aws-sdk-go-v2 client to prove UpdateFieldLevelEncryptionConfig is
// reachable. Real UpdateFieldLevelEncryptionConfig PUTs to
// /field-level-encryption/{Id}/config (cloudfront@v1.67.4 serializers.go:
// awsRestxml_serializeOpUpdateFieldLevelEncryptionConfig's SplitURI);
// gopherstack previously bound it to the bare /field-level-encryption/{Id}
// path instead, so every real client call 404'd (gopherstack-o31x).
func TestUpdateFieldLevelEncryptionConfig_RealClient(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	client := newTestCloudFrontClient(t, h)

	fle, err := h.Backend.CreateFieldLevelEncryption("real-client-fle", "original", nil)
	require.NoError(t, err)

	updated, err := client.UpdateFieldLevelEncryptionConfig(t.Context(), &cfsdk.UpdateFieldLevelEncryptionConfigInput{
		Id: aws.String(fle.ID),
		FieldLevelEncryptionConfig: &types.FieldLevelEncryptionConfig{
			CallerReference: aws.String("real-client-fle"),
			Comment:         aws.String("updated"),
		},
	})
	require.NoError(t, err)
	require.NotNil(t, updated.FieldLevelEncryption)
	assert.Equal(t, "updated", aws.ToString(updated.FieldLevelEncryption.FieldLevelEncryptionConfig.Comment))
}

// TestUpdateFieldLevelEncryptionConfig_CallerReferenceCollisionAllowed is a regression test
// for gopherstack-kpk5. UpdateFieldLevelEncryptionConfig's declared error set
// (cloudfront@v1.67.4 deserializers.go:24068
// awsRestxml_deserializeOpErrorUpdateFieldLevelEncryptionConfig) has no
// FieldLevelEncryptionConfigAlreadyExists -- unlike CreateFieldLevelEncryptionConfig, which
// does declare it -- so real AWS does not re-validate CallerReference uniqueness on Update.
// Same Create-only/Update-silent split holds for DistributionAlreadyExists
// (CreateDistribution/UpdateDistribution) and StreamingDistributionAlreadyExists
// (CreateStreamingDistribution/UpdateStreamingDistribution). Renaming a config's
// CallerReference onto one already used by a different config must therefore succeed, not
// return IllegalUpdate or FieldLevelEncryptionConfigAlreadyExists.
func TestUpdateFieldLevelEncryptionConfig_CallerReferenceCollisionAllowed(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	first, err := h.Backend.CreateFieldLevelEncryption("fle-collision-target", "first", nil)
	require.NoError(t, err)
	second, err := h.Backend.CreateFieldLevelEncryption("fle-collision-source", "second", nil)
	require.NoError(t, err)

	body := []byte(`<FieldLevelEncryptionConfig><CallerReference>fle-collision-target</CallerReference>` +
		`<Comment>renamed onto first's CallerReference</Comment></FieldLevelEncryptionConfig>`)
	rec := doXML(t, h, http.MethodPut, "/2020-05-31/field-level-encryption/"+second.ID+"/config", body)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	assert.Contains(t, rec.Body.String(), "<CallerReference>fle-collision-target</CallerReference>")
	assert.NotContains(t, rec.Body.String(), "FieldLevelEncryptionConfigAlreadyExists")
	assert.NotContains(t, rec.Body.String(), "IllegalUpdate")

	got, err := h.Backend.GetFieldLevelEncryption(second.ID)
	require.NoError(t, err)
	assert.Equal(t, "fle-collision-target", got.Name)

	stillThere, err := h.Backend.GetFieldLevelEncryption(first.ID)
	require.NoError(t, err)
	assert.Equal(t, "fle-collision-target", stillThere.Name)
}

// TestUpdateFieldLevelEncryptionProfile_RealClient drives the real
// aws-sdk-go-v2 client to prove UpdateFieldLevelEncryptionProfile is
// reachable. Real UpdateFieldLevelEncryptionProfile PUTs to
// /field-level-encryption-profile/{Id}/config (cloudfront@v1.67.4
// serializers.go: awsRestxml_serializeOpUpdateFieldLevelEncryptionProfile's
// SplitURI); gopherstack previously bound it to the bare
// /field-level-encryption-profile/{Id} path instead, so every real client
// call 404'd (gopherstack-o31x).
func TestUpdateFieldLevelEncryptionProfile_RealClient(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	client := newTestCloudFrontClient(t, h)

	pk, err := h.Backend.CreatePublicKey(
		"real-client-fle-profile-pk",
		"real-client-fle-profile-pk",
		"",
		testRSA2048PublicKeyPEM,
	)
	require.NoError(t, err)

	profile, err := h.Backend.CreateFieldLevelEncryptionProfile(
		"real-client-fle-profile", "original", []cloudfront.EncryptionEntity{
			{PublicKeyID: pk.ID, ProviderID: "prov", FieldPatterns: []string{"field1"}},
		},
	)
	require.NoError(t, err)

	updated, err := client.UpdateFieldLevelEncryptionProfile(
		t.Context(),
		&cfsdk.UpdateFieldLevelEncryptionProfileInput{
			Id: aws.String(profile.ID),
			FieldLevelEncryptionProfileConfig: &types.FieldLevelEncryptionProfileConfig{
				CallerReference: aws.String("real-client-fle-profile"),
				Name:            aws.String("real-client-fle-profile"),
				Comment:         aws.String("updated"),
				EncryptionEntities: &types.EncryptionEntities{
					Quantity: aws.Int32(1),
					Items: []types.EncryptionEntity{
						{
							PublicKeyId: aws.String(pk.ID),
							ProviderId:  aws.String("prov"),
							FieldPatterns: &types.FieldPatterns{
								Quantity: aws.Int32(1),
								Items:    []string{"field1"},
							},
						},
					},
				},
			},
		},
	)
	require.NoError(t, err)
	require.NotNil(t, updated.FieldLevelEncryptionProfile)
	assert.Equal(
		t,
		"updated",
		aws.ToString(updated.FieldLevelEncryptionProfile.FieldLevelEncryptionProfileConfig.Comment),
	)
}

// TestFieldLevelEncryptionProfileCRUD covers the full FLE Profile lifecycle via the HTTP handler.
func TestFieldLevelEncryptionProfileCRUD(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup       func(*testing.T, *cloudfront.Handler) string
		check       func(*testing.T, *httptest.ResponseRecorder, string)
		headersFunc func(*testing.T, *cloudfront.Handler, string) map[string]string
		name        string
		method      string
		path        string
		body        []byte
		wantStatus  int
	}{
		{
			name:   "create_fle_profile",
			method: http.MethodPost,
			path:   "/2020-05-31/field-level-encryption-profile",
			body: []byte(
				`<FieldLevelEncryptionProfileConfig>` +
					`<Name>my-profile</Name>` +
					`<Comment>test</Comment>` +
					`</FieldLevelEncryptionProfileConfig>`,
			),
			setup: func(t *testing.T, _ *cloudfront.Handler) string {
				t.Helper()

				return ""
			},
			wantStatus: http.StatusCreated,
			check: func(t *testing.T, rec *httptest.ResponseRecorder, _ string) {
				t.Helper()
				assert.Contains(t, rec.Body.String(), "<FieldLevelEncryptionProfile")
				assert.NotEmpty(t, rec.Header().Get("Location"))
				assert.NotEmpty(t, rec.Header().Get("ETag"))
			},
		},
		{
			name:   "list_fle_profiles",
			method: http.MethodGet,
			path:   "/2020-05-31/field-level-encryption-profile",
			body:   nil,
			setup: func(t *testing.T, h *cloudfront.Handler) string {
				t.Helper()
				_, err := h.Backend.CreateFieldLevelEncryptionProfile("list-fle-profile", "comment", nil)
				require.NoError(t, err)

				return ""
			},
			wantStatus: http.StatusOK,
			check: func(t *testing.T, rec *httptest.ResponseRecorder, _ string) {
				t.Helper()
				assert.Contains(t, rec.Body.String(), "<FieldLevelEncryptionProfileList")
				assert.Contains(t, rec.Body.String(), "<Quantity>1</Quantity>")
			},
		},
		{
			name:   "get_fle_profile",
			method: http.MethodGet,
			path:   "",
			body:   nil,
			setup: func(t *testing.T, h *cloudfront.Handler) string {
				t.Helper()
				p, err := h.Backend.CreateFieldLevelEncryptionProfile("get-fle-profile", "comment", nil)
				require.NoError(t, err)

				return "/2020-05-31/field-level-encryption-profile/" + p.ID
			},
			wantStatus: http.StatusOK,
			check: func(t *testing.T, rec *httptest.ResponseRecorder, _ string) {
				t.Helper()
				assert.Contains(t, rec.Body.String(), "<FieldLevelEncryptionProfile")
				assert.NotEmpty(t, rec.Header().Get("ETag"))
			},
		},
		{
			name:   "get_fle_profile_config",
			method: http.MethodGet,
			path:   "",
			body:   nil,
			setup: func(t *testing.T, h *cloudfront.Handler) string {
				t.Helper()
				p, err := h.Backend.CreateFieldLevelEncryptionProfile("get-fle-profile2", "comment2", nil)
				require.NoError(t, err)

				return "/2020-05-31/field-level-encryption-profile/" + p.ID + "/config"
			},
			wantStatus: http.StatusOK,
			check: func(t *testing.T, rec *httptest.ResponseRecorder, _ string) {
				t.Helper()
				assert.Contains(t, rec.Body.String(), "<FieldLevelEncryptionProfile")
			},
		},
		{
			name:   "update_fle_profile",
			method: http.MethodPut,
			path:   "",
			body: []byte(
				`<FieldLevelEncryptionProfileConfig>` +
					`<Name>upd-profile</Name>` +
					`<Comment>updated</Comment>` +
					`</FieldLevelEncryptionProfileConfig>`,
			),
			setup: func(t *testing.T, h *cloudfront.Handler) string {
				t.Helper()
				p, err := h.Backend.CreateFieldLevelEncryptionProfile("old-fle-profile", "original", nil)
				require.NoError(t, err)

				return "/2020-05-31/field-level-encryption-profile/" + p.ID + "/config"
			},
			wantStatus: http.StatusOK,
			check: func(t *testing.T, rec *httptest.ResponseRecorder, _ string) {
				t.Helper()
				assert.Contains(t, rec.Body.String(), "<FieldLevelEncryptionProfile")
				assert.NotEmpty(t, rec.Header().Get("ETag"))
			},
		},
		{
			name:   "delete_fle_profile",
			method: http.MethodDelete,
			path:   "",
			body:   nil,
			setup: func(t *testing.T, h *cloudfront.Handler) string {
				t.Helper()
				p, err := h.Backend.CreateFieldLevelEncryptionProfile("del-fle-profile", "delete me", nil)
				require.NoError(t, err)

				return "/2020-05-31/field-level-encryption-profile/" + p.ID
			},
			headersFunc: func(t *testing.T, h *cloudfront.Handler, path string) map[string]string {
				t.Helper()
				parts := strings.Split(strings.TrimRight(path, "/"), "/")
				id := parts[len(parts)-1]
				p, err := h.Backend.GetFieldLevelEncryptionProfile(id)
				require.NoError(t, err)

				return map[string]string{"If-Match": p.ETag}
			},
			wantStatus: http.StatusNoContent,
			check:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			path := tt.path
			if tt.setup != nil {
				if p := tt.setup(t, h); p != "" {
					path = p
				}
			}

			var hdrs map[string]string
			if tt.headersFunc != nil {
				hdrs = tt.headersFunc(t, h, path)
			}
			rec := doXMLWithHeaders(t, h, tt.method, path, tt.body, hdrs)

			assert.Equal(t, tt.wantStatus, rec.Code)
			if tt.check != nil {
				tt.check(t, rec, path)
			}
		})
	}
}

// TestInMemoryBackend_FieldLevelEncryption tests FLE backend operations directly.
func TestInMemoryBackend_FieldLevelEncryption(t *testing.T) {
	t.Parallel()

	tests := []struct {
		run  func(*testing.T, *cloudfront.InMemoryBackend)
		name string
	}{
		{
			name: "create_get_list_update_delete",
			run: func(t *testing.T, b *cloudfront.InMemoryBackend) {
				t.Helper()
				fle, err := b.CreateFieldLevelEncryption("fle-backend-test", "comment", nil)
				require.NoError(t, err)
				assert.NotEmpty(t, fle.ID)

				got, err := b.GetFieldLevelEncryption(fle.ID)
				require.NoError(t, err)
				assert.Equal(t, "comment", got.Comment)

				list := b.ListFieldLevelEncryptions()
				assert.Len(t, list, 1)

				updated, err := b.UpdateFieldLevelEncryption(fle.ID, "fle-backend-test-new", "updated", nil)
				require.NoError(t, err)
				assert.Equal(t, "updated", updated.Comment)

				require.NoError(t, b.DeleteFieldLevelEncryption(fle.ID))
				_, err = b.GetFieldLevelEncryption(fle.ID)
				require.Error(t, err)
			},
		},
		{
			name: "create_duplicate_name_fails",
			run: func(t *testing.T, b *cloudfront.InMemoryBackend) {
				t.Helper()
				_, err := b.CreateFieldLevelEncryption("dup-fle", "comment", nil)
				require.NoError(t, err)
				_, err = b.CreateFieldLevelEncryption("dup-fle", "comment", nil)
				require.Error(t, err)
			},
		},
		{
			name: "get_fle_profile_list_update_delete",
			run: func(t *testing.T, b *cloudfront.InMemoryBackend) {
				t.Helper()
				p, err := b.CreateFieldLevelEncryptionProfile("profile-test", "comment", nil)
				require.NoError(t, err)

				got, err := b.GetFieldLevelEncryptionProfile(p.ID)
				require.NoError(t, err)
				assert.Equal(t, "profile-test", got.Name)

				list := b.ListFieldLevelEncryptionProfiles()
				assert.Len(t, list, 1)

				updated, err := b.UpdateFieldLevelEncryptionProfile(p.ID, "profile-test-new", "updated", nil)
				require.NoError(t, err)
				assert.Equal(t, "updated", updated.Comment)

				require.NoError(t, b.DeleteFieldLevelEncryptionProfile(p.ID))
				_, err = b.GetFieldLevelEncryptionProfile(p.ID)
				require.Error(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := cloudfront.NewInMemoryBackend(t.Context(), "123456789012", "us-east-1")
			tt.run(t, b)
		})
	}
}

// TestListFieldLevelEncryptionConfigs_ItemShape_RealClient is a regression test for
// gopherstack-21my: ListFieldLevelEncryptionConfigs' item struct (fleSummaryXML,
// handler_field_level_encryption.go) omitted QueryArgProfileConfig entirely, even though
// the real FieldLevelEncryptionSummary deserializer
// (awsRestxml_deserializeDocumentFieldLevelEncryptionSummary) reads it and the sibling
// GetFieldLevelEncryptionConfig (fleConfigInnerXML) already emits it correctly from the
// same backing FieldLevelEncryption.QueryArgProfiles/.ForwardWhenQueryArgProfileIsUnknown
// fields -- the "Get right, List wrong" trap. Seeds two configs with distinguishable
// query-arg profiles and asserts both round-trip.
func TestListFieldLevelEncryptionConfigs_ItemShape_RealClient(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	profile, err := h.Backend.CreateFieldLevelEncryptionProfile("list-shape-profile", "", nil)
	require.NoError(t, err)

	first, err := h.Backend.CreateFieldLevelEncryption("list-shape-fle-1", "first", []cloudfront.FLEQueryArgProfile{
		{QueryArg: "first-arg", ProfileID: profile.ID},
	})
	require.NoError(t, err)

	second, err := h.Backend.CreateFieldLevelEncryption("list-shape-fle-2", "second", []cloudfront.FLEQueryArgProfile{
		{QueryArg: "second-arg", ProfileID: profile.ID},
	})
	require.NoError(t, err)

	client := newTestCloudFrontClient(t, h)

	listed, err := client.ListFieldLevelEncryptionConfigs(t.Context(), &cfsdk.ListFieldLevelEncryptionConfigsInput{})
	require.NoError(t, err)
	require.NotNil(t, listed.FieldLevelEncryptionList)
	require.Len(t, listed.FieldLevelEncryptionList.Items, 2)

	byID := make(map[string]types.FieldLevelEncryptionSummary, 2)
	for _, item := range listed.FieldLevelEncryptionList.Items {
		require.NotNil(t, item.Id)
		byID[*item.Id] = item
	}

	item1, ok := byID[first.ID]
	require.True(t, ok)
	require.NotNil(t, item1.QueryArgProfileConfig)
	require.NotNil(t, item1.QueryArgProfileConfig.QueryArgProfiles)
	require.Len(t, item1.QueryArgProfileConfig.QueryArgProfiles.Items, 1)
	assert.Equal(t, "first-arg", aws.ToString(item1.QueryArgProfileConfig.QueryArgProfiles.Items[0].QueryArg))
	assert.Equal(t, profile.ID, aws.ToString(item1.QueryArgProfileConfig.QueryArgProfiles.Items[0].ProfileId))

	item2, ok := byID[second.ID]
	require.True(t, ok)
	require.NotNil(t, item2.QueryArgProfileConfig)
	require.NotNil(t, item2.QueryArgProfileConfig.QueryArgProfiles)
	require.Len(t, item2.QueryArgProfileConfig.QueryArgProfiles.Items, 1)
	assert.Equal(t, "second-arg", aws.ToString(item2.QueryArgProfileConfig.QueryArgProfiles.Items[0].QueryArg))
}

// TestListFieldLevelEncryptionProfiles_ItemShape_RealClient is a regression test for
// gopherstack-21my: ListFieldLevelEncryptionProfiles' item struct (flePSummaryXML,
// handler_field_level_encryption.go) omitted EncryptionEntities entirely, even though the
// real FieldLevelEncryptionProfileSummary deserializer
// (awsRestxml_deserializeDocumentFieldLevelEncryptionProfileSummary) reads it and the
// sibling GetFieldLevelEncryptionProfile (fleProfileConfigInnerXML) already emits it
// correctly from the same backing FieldLevelEncryptionProfile.EncryptionEntities field --
// the "Get right, List wrong" trap. Seeds two profiles with distinguishable encryption
// entities and asserts both round-trip.
func TestListFieldLevelEncryptionProfiles_ItemShape_RealClient(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	first, err := h.Backend.CreateFieldLevelEncryptionProfile(
		"list-shape-profile-1", "first profile", []cloudfront.EncryptionEntity{
			{ProviderID: "provider-one", FieldPatterns: []string{"field-one"}},
		},
	)
	require.NoError(t, err)

	second, err := h.Backend.CreateFieldLevelEncryptionProfile(
		"list-shape-profile-2", "second profile", []cloudfront.EncryptionEntity{
			{ProviderID: "provider-two", FieldPatterns: []string{"field-two"}},
		},
	)
	require.NoError(t, err)

	client := newTestCloudFrontClient(t, h)

	listed, err := client.ListFieldLevelEncryptionProfiles(
		t.Context(), &cfsdk.ListFieldLevelEncryptionProfilesInput{},
	)
	require.NoError(t, err)
	require.NotNil(t, listed.FieldLevelEncryptionProfileList)
	require.Len(t, listed.FieldLevelEncryptionProfileList.Items, 2)

	byID := make(map[string]types.FieldLevelEncryptionProfileSummary, 2)
	for _, item := range listed.FieldLevelEncryptionProfileList.Items {
		require.NotNil(t, item.Id)
		byID[*item.Id] = item
	}

	item1, ok := byID[first.ID]
	require.True(t, ok)
	require.Len(t, item1.EncryptionEntities.Items, 1)
	assert.Equal(t, "provider-one", aws.ToString(item1.EncryptionEntities.Items[0].ProviderId))
	require.Len(t, item1.EncryptionEntities.Items[0].FieldPatterns.Items, 1)
	assert.Equal(t, "field-one", item1.EncryptionEntities.Items[0].FieldPatterns.Items[0])

	item2, ok := byID[second.ID]
	require.True(t, ok)
	require.Len(t, item2.EncryptionEntities.Items, 1)
	assert.Equal(t, "provider-two", aws.ToString(item2.EncryptionEntities.Items[0].ProviderId))
}

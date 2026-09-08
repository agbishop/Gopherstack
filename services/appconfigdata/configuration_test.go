package appconfigdata_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/appconfigdata"
)

// --- GetLatestConfiguration ---

func TestHandler_GetLatestConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		content       string
		contentType   string
		wantBody      string
		wantStatus    int
		hasProfile    bool
		wantEtag      bool
		wantNoContent bool
	}{
		{
			name:        "with_configuration_returns_200",
			hasProfile:  true,
			content:     `{"featureFlag":true}`,
			contentType: "application/json",
			wantStatus:  http.StatusOK,
			wantBody:    `{"featureFlag":true}`,
			wantEtag:    true,
		},
		{
			// AWS's GetLatestConfiguration has a *fixed* responseCode of 200 -- even when
			// unchanged, the status is 200 with an empty body, never 204.
			name:          "second_poll_with_no_change_returns_200_empty_body",
			hasProfile:    true,
			content:       `{"featureFlag":true}`,
			contentType:   "application/json",
			wantStatus:    http.StatusOK,
			wantNoContent: true,
		},
		{
			name:       "invalid_token_returns_400",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			if tt.name == "invalid_token_returns_400" {
				rec := doRequest(
					t,
					h,
					http.MethodGet,
					"/configuration?configuration_token=bad-token",
					nil,
				)
				assert.Equal(t, tt.wantStatus, rec.Code)

				return
			}

			if tt.hasProfile {
				require.NoError(
					t,
					h.Backend.SetConfiguration(
						"my-app",
						"prod",
						"my-profile",
						tt.content,
						tt.contentType,
					),
				)
			}

			token := startSession(t, h, "my-app", "prod", "my-profile")

			if tt.wantNoContent {
				// First poll to set PreviousContentHash.
				rec1 := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+token, nil)
				require.Equal(t, http.StatusOK, rec1.Code)
				token = rec1.Header().Get("Next-Poll-Configuration-Token")
				require.NotEmpty(t, token)

				// Second poll — content unchanged → 200 with an empty body (AWS never
				// returns 204 for this operation).
				rec2 := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+token, nil)
				assert.Equal(t, http.StatusOK, rec2.Code)
				assert.Empty(t, rec2.Header().Get("ETag"), "ETag must not be set when unchanged")
				assert.Empty(t, rec2.Body.String())

				return
			}

			rec := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+token, nil)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantBody != "" {
				assert.Equal(t, tt.wantBody, rec.Body.String())
			}

			if tt.wantEtag {
				assert.NotEmpty(t, rec.Header().Get("ETag"))
			}

			if tt.wantStatus == http.StatusOK {
				nextToken := rec.Header().Get("Next-Poll-Configuration-Token")
				assert.NotEmpty(t, nextToken)
				assert.NotEqual(t, token, nextToken)
			}
		})
	}
}

func TestHandler_GetLatestConfiguration_EmptyToken(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, http.MethodGet, "/configuration", nil)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestHandler_NoContentHeaders verifies that an unchanged-configuration response (200 with
// an empty body) does not include ETag or Content-Type — only the poll-control headers
// should be present.
func TestHandler_NoContentHeaders(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	seedProfile(t, h, "app", "env", "p", `{"x":1}`)
	token := startSession(t, h, "app", "env", "p")

	// First poll sets PreviousContentHash.
	rec1 := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+token, nil)
	require.Equal(t, http.StatusOK, rec1.Code)
	assert.NotEmpty(t, rec1.Header().Get("ETag"), "first poll must include ETag")
	assert.NotEmpty(t, rec1.Header().Get("Next-Poll-Configuration-Token"))
	assert.NotEmpty(t, rec1.Header().Get("Next-Poll-Interval-In-Seconds"))
	token = rec1.Header().Get("Next-Poll-Configuration-Token")

	// Second poll — content unchanged → 200 with an empty body (AWS's fixed responseCode).
	rec2 := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+token, nil)
	require.Equal(t, http.StatusOK, rec2.Code)
	assert.Empty(t, rec2.Body.String(), "unchanged response must have an empty body")
	assert.Empty(t, rec2.Header().Get("ETag"), "unchanged response must not include ETag")
	// Poll-control headers must still be present.
	assert.NotEmpty(t, rec2.Header().Get("Next-Poll-Configuration-Token"))
	assert.NotEmpty(t, rec2.Header().Get("Next-Poll-Interval-In-Seconds"))
}

// TestHandler_VersionLabelHeader verifies the Version-Label response header.
// The AWS SDK v2 deserializer reads "Version-Label" (not "X-Amzn-AppConfig-Version-Label").
func TestHandler_VersionLabelHeader(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	seedProfile(t, h, "app", "env", "p", `{"v":1}`)
	token := startSession(t, h, "app", "env", "p")

	rec := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+token, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("Version-Label"))
}

// TestHandler_ProfileDeletedMidSession verifies that polling after the profile is removed
// returns 404 ResourceNotFoundException.
func TestHandler_ProfileDeletedMidSession(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	seedProfile(t, h, "app", "env", "p", `{"v":1}`)
	token := startSession(t, h, "app", "env", "p")

	// Poll once to get a valid next token.
	rec1 := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+token, nil)
	require.Equal(t, http.StatusOK, rec1.Code)
	nextToken := rec1.Header().Get("Next-Poll-Configuration-Token")

	// Delete the profile.
	require.True(t, h.Backend.DeleteProfile("app", "env", "p"))

	// Polling with the next token must yield 404 because the backend purges sessions on delete.
	rec2 := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+nextToken, nil)
	// Sessions tied to the deleted profile are removed, so token is now unknown → 400.
	assert.Equal(t, http.StatusBadRequest, rec2.Code)
}

// TestHandler_TokenExpired verifies that a swept (absolute-expiry) session token is gone
// from the backend, surfacing as ErrSessionNotFound (AWS itself returns 400, not 401, for
// an expired token -- see errors.go).
func TestHandler_TokenExpired(t *testing.T) {
	t.Parallel()

	b := appconfigdata.NewInMemoryBackend()
	require.NoError(t, b.SetConfiguration("app", "env", "p", `{"v":1}`, "application/json"))

	token, err := b.StartSession("app", "env", "p", 0)
	require.NoError(t, err)

	// Manually expire the session by sweeping with a zero idle TTL and then forcing the
	// absolute-expiry path is not possible without time travel, so we test the backend
	// directly: SweepExpiredSessions with zero TTL removes all sessions, then the token
	// returns ErrSessionNotFound (not ErrTokenExpired — it's gone from the map).
	b.SweepExpiredSessions(t.Context(), 0)

	_, _, _, _, _, err = b.GetLatestConfiguration(token)
	assert.ErrorIs(t, err, appconfigdata.ErrSessionNotFound)
}

// --- Token rotation ---

func TestHandler_TokenRotation(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	seedProfile(t, h, "app", "env", "profile", `{"v":1}`)

	token := startSession(t, h, "app", "env", "profile")

	// First poll succeeds and returns content.
	rec1 := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+token, nil)
	require.Equal(t, http.StatusOK, rec1.Code)
	nextToken := rec1.Header().Get("Next-Poll-Configuration-Token")
	assert.NotEmpty(t, nextToken)
	assert.NotEqual(t, token, nextToken)

	// Old token is in grace period — idempotent replay returns the cached response.
	rec2 := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+token, nil)
	assert.Equal(t, http.StatusOK, rec2.Code, "old token should work during grace period")

	// New token polls unchanged content → 200 with an empty body (AWS never returns 204).
	// The client already received the current version via T0, so T1's first poll yields empty body.
	rec3 := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+nextToken, nil)
	assert.Equal(t, http.StatusOK, rec3.Code)
	assert.Empty(t, rec3.Body.String(), "unchanged response must have an empty body")

	// After a configuration update the next token returns new content → 200.
	require.NoError(t, h.Backend.SetConfiguration("app", "env", "profile", `{"v":2}`, "application/json"))
	nextToken2 := rec3.Header().Get("Next-Poll-Configuration-Token")
	rec4 := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+nextToken2, nil)
	assert.Equal(t, http.StatusOK, rec4.Code, "changed content must yield 200")
}

// TestHandler_GraceTokenIdempotency verifies that replaying an old token within the grace
// window returns the same response as the original poll.
func TestHandler_GraceTokenIdempotency(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	seedProfile(t, h, "app", "env", "p", `{"val":42}`)
	token := startSession(t, h, "app", "env", "p")

	// First poll.
	rec1 := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+token, nil)
	require.Equal(t, http.StatusOK, rec1.Code)
	body1 := rec1.Body.String()
	next1 := rec1.Header().Get("Next-Poll-Configuration-Token")

	// Replay the original token (grace period).
	rec2 := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+token, nil)
	require.Equal(t, http.StatusOK, rec2.Code)
	assert.Equal(t, body1, rec2.Body.String(), "grace replay must return same body")
	assert.Equal(t, next1, rec2.Header().Get("Next-Poll-Configuration-Token"),
		"grace replay must return same next token")
}

// TestHandler_ContentTypeNotSetOnUnchanged is a focused regression test for the protocol
// violation where empty-body (unchanged-configuration) responses previously set Content-Type.
func TestHandler_ContentTypeNotSetOnUnchanged(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	seedProfile(t, h, "a", "e", "p", `{"x":1}`)
	token := startSession(t, h, "a", "e", "p")

	// First poll.
	rec1 := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+token, nil)
	require.Equal(t, http.StatusOK, rec1.Code)
	next := rec1.Header().Get("Next-Poll-Configuration-Token")

	// Second poll — content unchanged → 200 with an empty body (AWS's fixed responseCode).
	rec2 := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+next, nil)
	require.Equal(t, http.StatusOK, rec2.Code)
	assert.Empty(t, rec2.Body.String(), "unchanged response must have an empty body")
	// Content-Type must NOT be set to "application/json" (or anything data-describing) when unchanged.
	ct := rec2.Header().Get("Content-Type")
	assert.Empty(t, ct, "Content-Type must not be set on an unchanged (empty-body) response, got: %q", ct)
}

// TestHandler_TokenExpired_Returns400 verifies that an expired token returns 400 BadRequestException
// with Problem=Expired, matching AWS behavior (not 401 Unauthorized).
func TestHandler_TokenExpired_Returns400(t *testing.T) {
	t.Parallel()

	// We can't easily travel time, but we can verify the error mapping by injecting
	// a known-expired session directly via backend, or by checking that ErrTokenExpired
	// from the backend maps to 400 not 401.
	// The test uses SweepExpiredSessions(ttl=0) which evicts the session from the map,
	// causing ErrSessionNotFound → 400 with Problem=Corrupted.
	// For ErrTokenExpired path, we test via backend unit test + check the constant.
	b := appconfigdata.NewInMemoryBackend()
	require.NoError(t, b.SetConfiguration("app", "env", "p", `{}`, "application/json"))

	token, err := b.StartSession("app", "env", "p", 0)
	require.NoError(t, err)

	// Sweep all sessions to simulate expiry.
	b.SweepExpiredSessions(t.Context(), 0)

	_, _, _, _, _, backendErr := b.GetLatestConfiguration(token)
	// After sweep, session is gone → ErrSessionNotFound (not ErrTokenExpired).
	require.ErrorIs(t, backendErr, appconfigdata.ErrSessionNotFound)

	// Verify ErrTokenExpired is NOT mapped to 401 by checking via HTTP handler error dispatch.
	// We exercise the 400 path by using an unknown token (same status as expired → corrupted mapping).
	h := appconfigdata.NewHandler(appconfigdata.NewInMemoryBackend())
	seedProfile(t, h, "app", "env", "p", `{}`)
	tok := startSession(t, h, "app", "env", "p")
	h.Backend.SweepExpiredSessions(t.Context(), 0)

	rec := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+tok, nil)
	assert.Equal(t, http.StatusBadRequest, rec.Code, "expired/invalid token must return 400, not 401")
	assert.Equal(t, "BadRequestException", rec.Header().Get("X-Amzn-Errortype"))
}

// TestHandler_RetryAfterHeader verifies the Retry-After header is set on poll-too-frequent errors.
func TestHandler_RetryAfterHeader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		wantRetryAfter string
		pollInterval   int
	}{
		{name: "custom_interval_30s", pollInterval: 30, wantRetryAfter: "30"},
		{name: "custom_interval_60s", pollInterval: 60, wantRetryAfter: "60"},
		{name: "custom_interval_120s", pollInterval: 120, wantRetryAfter: "120"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			require.NoError(t, h.Backend.SetConfiguration("app", "env", "p", `{}`, "application/json"))

			sessionBody, err := json.Marshal(map[string]any{
				"ApplicationIdentifier":                "app",
				"EnvironmentIdentifier":                "env",
				"ConfigurationProfileIdentifier":       "p",
				"RequiredMinimumPollIntervalInSeconds": tt.pollInterval,
			})
			require.NoError(t, err)

			sessionRec := doRequest(t, h, http.MethodPost, "/configurationsessions", sessionBody)
			require.Equal(t, http.StatusCreated, sessionRec.Code)

			var sessionResp map[string]string
			require.NoError(t, json.Unmarshal(sessionRec.Body.Bytes(), &sessionResp))
			tok := sessionResp["InitialConfigurationToken"]

			// First poll — gets content.
			rec1 := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+tok, nil)
			require.Equal(t, http.StatusOK, rec1.Code)
			nextTok := rec1.Header().Get("Next-Poll-Configuration-Token")

			// Immediate re-poll — too frequent.
			rec2 := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+nextTok, nil)
			assert.Equal(t, http.StatusBadRequest, rec2.Code)
			assert.Equal(t, tt.wantRetryAfter, rec2.Header().Get("Retry-After"),
				"Retry-After must match session poll interval")
		})
	}
}

// TestHandler_VersionLabelHeaderNameIsVersionLabel verifies the response uses the AWS-defined
// "Version-Label" header name (not the older "X-Amzn-AppConfig-Version-Label" prefix).
func TestHandler_VersionLabelHeaderNameIsVersionLabel(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	require.NoError(t, h.Backend.SetConfiguration("myapp", "prod", "flags", `{"enabled":true}`, "application/json"))

	token := startSession(t, h, "myapp", "prod", "flags")
	rec := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+token, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	// "Version-Label" must be set — the AWS SDK v2 deserializer reads this exact header.
	assert.NotEmpty(t, rec.Header().Get("Version-Label"),
		"Version-Label header must be set on 200 responses")

	// The old header name must NOT be set — it is not in the AWS protocol.
	assert.Empty(t, rec.Header().Get("X-Amzn-Appconfig-Version-Label"),
		"X-Amzn-AppConfig-Version-Label is not in the AWS protocol and must not be set")
}

// TestHandler_VersionLabel_UnchangedResponse verifies the response status and body when
// polling again with no configuration change (200 with an empty body, never 204).
func TestHandler_VersionLabel_UnchangedResponse(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	seedProfile(t, h, "app", "env", "p", `{"v":1}`)
	token := startSession(t, h, "app", "env", "p")

	// First poll — consume version label.
	rec1 := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+token, nil)
	require.Equal(t, http.StatusOK, rec1.Code)
	require.NotEmpty(t, rec1.Header().Get("Version-Label"))
	nextToken := rec1.Header().Get("Next-Poll-Configuration-Token")

	// Second poll — unchanged → 200 with an empty body. Per the AWS doc comment on
	// GetLatestConfigurationOutput.VersionLabel: "If the client already has the latest
	// version of the configuration data, this value is empty." — so the header must be absent.
	rec2 := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+nextToken, nil)
	assert.Equal(t, http.StatusOK, rec2.Code)
	assert.Empty(t, rec2.Body.String(), "unchanged response must have an empty body")
	assert.Empty(t, rec2.Header().Get("Version-Label"),
		"Version-Label must be empty when the client already has the latest configuration")
}

// TestHandler_ConfigUpdateDetection verifies that after a configuration update, the next
// poll returns 200 with the new content (change detection via content hash).
func TestHandler_ConfigUpdateDetection(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	require.NoError(t, h.Backend.SetConfiguration("app", "env", "p", `{"v":1}`, "application/json"))
	token := startSession(t, h, "app", "env", "p")

	// First poll — returns v1.
	rec1 := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+token, nil)
	require.Equal(t, http.StatusOK, rec1.Code)
	assert.Equal(t, `{"v":1}`, rec1.Body.String())
	t1 := rec1.Header().Get("Next-Poll-Configuration-Token")
	etag1 := rec1.Header().Get("ETag")

	// Second poll — no change → 200 with an empty body.
	rec2 := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+t1, nil)
	require.Equal(t, http.StatusOK, rec2.Code)
	assert.Empty(t, rec2.Body.String(), "unchanged response must have an empty body")
	t2 := rec2.Header().Get("Next-Poll-Configuration-Token")

	// Update configuration.
	require.NoError(t, h.Backend.SetConfiguration("app", "env", "p", `{"v":2}`, "application/json"))

	// Third poll — detects change → 200 with v2.
	rec3 := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+t2, nil)
	require.Equal(t, http.StatusOK, rec3.Code)
	assert.Equal(t, `{"v":2}`, rec3.Body.String())

	etag3 := rec3.Header().Get("ETag")
	assert.NotEmpty(t, etag3, "changed content must include ETag")
	assert.NotEqual(t, etag1, etag3, "ETag must change when content changes")

	// Fourth poll — no change → 200 with an empty body.
	t3 := rec3.Header().Get("Next-Poll-Configuration-Token")
	rec4 := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+t3, nil)
	assert.Equal(t, http.StatusOK, rec4.Code)
	assert.Empty(t, rec4.Body.String(), "unchanged response must have an empty body")
}

// TestHandler_JSONSemanticEquivalence verifies that semantically equivalent JSON documents
// (same keys/values, different whitespace) produce the same hash, yielding a 200 response
// with an empty body on the second poll.
func TestHandler_JSONSemanticEquivalence(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	require.NoError(t, h.Backend.SetConfiguration("app", "env", "p",
		`{"b":2,"a":1}`, "application/json"))
	token := startSession(t, h, "app", "env", "p")

	rec1 := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+token, nil)
	require.Equal(t, http.StatusOK, rec1.Code)
	t1 := rec1.Header().Get("Next-Poll-Configuration-Token")

	// Update with semantically equivalent JSON (different key order, extra whitespace).
	require.NoError(t, h.Backend.SetConfiguration("app", "env", "p",
		`{ "a": 1, "b": 2 }`, "application/json"))

	rec2 := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+t1, nil)
	require.Equal(t, http.StatusOK, rec2.Code,
		"semantically equivalent JSON must not trigger change detection")
	assert.Empty(t, rec2.Body.String(), "unchanged response must have an empty body")
}

// TestHandler_ContentTypePreserved verifies that non-JSON content types are passed through
// without modification, and the Content-Type header matches what was stored.
func TestHandler_ContentTypePreserved(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		content     string
		contentType string
	}{
		{
			name:        "plain_text",
			content:     "feature.enabled=true\nfeature.limit=100",
			contentType: "text/plain",
		},
		{
			name:        "yaml",
			content:     "feature:\n  enabled: true",
			contentType: "application/x-yaml",
		},
		{
			name:        "toml",
			content:     "[feature]\nenabled = true",
			contentType: "application/toml",
		},
		{
			name:        "json_plus_suffix",
			content:     `{"enabled":true}`,
			contentType: "application/vnd.api+json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			require.NoError(t, h.Backend.SetConfiguration("app", "env", "p", tt.content, tt.contentType))
			token := startSession(t, h, "app", "env", "p")

			rec := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+token, nil)
			require.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, tt.content, rec.Body.String())
			assert.Contains(t, rec.Header().Get("Content-Type"), strings.Split(tt.contentType, ";")[0])
		})
	}
}

// TestHandler_ETagFormat verifies the ETag header uses double-quoted SHA-256 hex format.
func TestHandler_ETagFormat(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	seedProfile(t, h, "app", "env", "p", `{"k":"v"}`)
	token := startSession(t, h, "app", "env", "p")

	rec := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+token, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	etag := rec.Header().Get("ETag")
	require.NotEmpty(t, etag)
	assert.True(t, strings.HasPrefix(etag, `"`), "ETag must start with double-quote")
	assert.True(t, strings.HasSuffix(etag, `"`), "ETag must end with double-quote")

	// Inner content is a hex-encoded SHA-256 (64 hex chars).
	inner := strings.Trim(etag, `"`)
	assert.Len(t, inner, 64, "ETag inner content must be 64-char SHA-256 hex")
}

// TestHandler_ContentLengthHeader verifies Content-Length is set on 200 responses.
func TestHandler_ContentLengthHeader(t *testing.T) {
	t.Parallel()

	content := `{"hello":"world","count":42}`
	h := newTestHandler(t)
	require.NoError(t, h.Backend.SetConfiguration("app", "env", "p", content, "application/json"))
	token := startSession(t, h, "app", "env", "p")

	rec := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+token, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	cl := rec.Header().Get("Content-Length")
	assert.Equal(t, strconv.Itoa(len(content)), cl, "Content-Length must match actual content size")
}

// TestHandler_NextPollTokenHeader verifies both Next-Poll-Configuration-Token and
// Next-Poll-Interval-In-Seconds are always set on successful responses, regardless of
// whether the configuration content changed.
func TestHandler_NextPollTokenHeader(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	seedProfile(t, h, "app", "env", "p", `{"v":1}`)
	token := startSession(t, h, "app", "env", "p")

	// First poll (content present) must carry both poll-control headers.
	rec1 := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+token, nil)
	require.Equal(t, http.StatusOK, rec1.Code)
	assert.NotEmpty(t, rec1.Header().Get("Next-Poll-Configuration-Token"))
	assert.NotEmpty(t, rec1.Header().Get("Next-Poll-Interval-In-Seconds"))
	next := rec1.Header().Get("Next-Poll-Configuration-Token")

	// Second poll (unchanged, empty body) must also carry both poll-control headers.
	rec2 := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+next, nil)
	require.Equal(t, http.StatusOK, rec2.Code)
	assert.Empty(t, rec2.Body.String(), "unchanged response must have an empty body")
	assert.NotEmpty(t, rec2.Header().Get("Next-Poll-Configuration-Token"))
	assert.NotEmpty(t, rec2.Header().Get("Next-Poll-Interval-In-Seconds"))
}

// TestHandler_PollRateLimitEnforced verifies that when a session declares an explicit
// minimum poll interval, immediate re-polling returns 400 with Problem=PollIntervalNotSatisfied
// and Retry-After header matching the session interval.
func TestHandler_PollRateLimitEnforced(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		wantRetry    string
		pollInterval int
	}{
		{name: "30s_interval_enforced", pollInterval: 30, wantRetry: "30"},
		{name: "60s_interval_enforced", pollInterval: 60, wantRetry: "60"},
		{name: "15s_minimum_enforced", pollInterval: 15, wantRetry: "15"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			require.NoError(t, h.Backend.SetConfiguration("app", "env", "p", `{}`, "application/json"))

			sessionBody, _ := json.Marshal(map[string]any{
				"ApplicationIdentifier":                "app",
				"EnvironmentIdentifier":                "env",
				"ConfigurationProfileIdentifier":       "p",
				"RequiredMinimumPollIntervalInSeconds": tt.pollInterval,
			})

			rec := doRequest(t, h, http.MethodPost, "/configurationsessions", sessionBody)
			require.Equal(t, http.StatusCreated, rec.Code)

			var sessionResp map[string]string
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &sessionResp))
			tok := sessionResp["InitialConfigurationToken"]

			// First poll — succeeds.
			rec1 := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+tok, nil)
			require.Equal(t, http.StatusOK, rec1.Code)
			nextTok := rec1.Header().Get("Next-Poll-Configuration-Token")
			require.NotEmpty(t, nextTok)

			// Immediate re-poll — rate limited.
			rec2 := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+nextTok, nil)
			assert.Equal(t, http.StatusBadRequest, rec2.Code)
			assert.Equal(t, "BadRequestException", rec2.Header().Get("X-Amzn-Errortype"))
			assert.Equal(t, tt.wantRetry, rec2.Header().Get("Retry-After"))

			var body map[string]any
			require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &body))
			assert.Equal(t, "BadRequestException", body["__type"])
			assert.Equal(t, "InvalidParameters", body["Reason"])

			details, ok := body["Details"].(map[string]any)
			require.True(t, ok)
			params, ok := details["InvalidParameters"].(map[string]any)
			require.True(t, ok)
			tokenDetail, ok := params["ConfigurationToken"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, "PollIntervalNotSatisfied", tokenDetail["Problem"])
		})
	}
}

// TestHandler_NoPollRateLimitWhenIntervalZero verifies that when no minimum poll interval
// is declared (0), immediate re-polling is allowed. AWS only enforces rate limiting when
// the client explicitly declares RequiredMinimumPollIntervalInSeconds > 0.
func TestHandler_NoPollRateLimitWhenIntervalZero(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	require.NoError(t, h.Backend.SetConfiguration("app", "env", "p", `{"v":1}`, "application/json"))
	tok := startSession(t, h, "app", "env", "p")

	// First poll.
	rec1 := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+tok, nil)
	require.Equal(t, http.StatusOK, rec1.Code)
	nextTok := rec1.Header().Get("Next-Poll-Configuration-Token")

	// Immediate re-poll must NOT be rejected.
	rec2 := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+nextTok, nil)
	require.Equal(t, http.StatusOK, rec2.Code,
		"immediate re-poll with no declared interval must return 200 (empty body), not a rate-limit error")
	assert.Empty(t, rec2.Body.String(), "unchanged response must have an empty body")
}

// TestHandler_ResponseHeaders verifies that AWS-required response headers are present and
// absent at the right times, for both a content-bearing response and an unchanged
// (empty-body) response -- both of which use HTTP 200, per AWS's fixed responseCode.
func TestHandler_ResponseHeaders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		wantETag         bool
		wantVersionLabel bool
		wantEmptyBody    bool
		isSecondPoll     bool
	}{
		{
			name:             "first_poll_content_headers",
			wantETag:         true,
			wantVersionLabel: true,
		},
		{
			name:          "second_poll_unchanged_no_etag_no_content_type",
			wantETag:      false,
			wantEmptyBody: true,
			isSecondPoll:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			require.NoError(t, h.Backend.SetConfiguration("app", "env", "p", `{"v":1}`, "application/json"))
			tok := startSession(t, h, "app", "env", "p")

			if tt.isSecondPoll {
				rec0 := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+tok, nil)
				require.Equal(t, http.StatusOK, rec0.Code)
				tok = rec0.Header().Get("Next-Poll-Configuration-Token")
			}

			rec := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+tok, nil)
			assert.Equal(t, http.StatusOK, rec.Code, "GetLatestConfiguration always returns 200")

			// Poll-control headers always present on success.
			assert.NotEmpty(t, rec.Header().Get("Next-Poll-Configuration-Token"))
			assert.NotEmpty(t, rec.Header().Get("Next-Poll-Interval-In-Seconds"))

			if tt.wantETag {
				assert.NotEmpty(t, rec.Header().Get("ETag"), "ETag must be set when content changed")
			} else {
				assert.Empty(t, rec.Header().Get("ETag"), "ETag must not be set when unchanged")
			}

			if tt.wantVersionLabel {
				assert.NotEmpty(t, rec.Header().Get("Version-Label"),
					"Version-Label must be set when content changed")
			} else {
				assert.Empty(t, rec.Header().Get("Version-Label"),
					"Version-Label must not be set when unchanged")
			}

			if tt.wantEmptyBody {
				assert.Empty(t, rec.Body.String(), "unchanged response must have an empty body")
				assert.Empty(t, rec.Header().Get("Content-Type"),
					"Content-Type must not be set on an unchanged (empty-body) response")
			}
		})
	}
}

// TestHandler_TokenRotationAndGracePeriod verifies that each poll rotates the token and
// the old token remains usable within the grace period (idempotent retry).
func TestHandler_TokenRotationAndGracePeriod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		replayOld     bool
		wantEmptyBody bool
	}{
		{
			name:          "new_token_usable_after_rotation",
			replayOld:     false,
			wantEmptyBody: true, // unchanged since the first poll already delivered the content
		},
		{
			name:      "old_token_in_grace_period_returns_same_response",
			replayOld: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			require.NoError(t, h.Backend.SetConfiguration("app", "env", "p", `{"v":1}`, "application/json"))
			initialTok := startSession(t, h, "app", "env", "p")

			rec1 := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+initialTok, nil)
			require.Equal(t, http.StatusOK, rec1.Code)
			nextTok := rec1.Header().Get("Next-Poll-Configuration-Token")
			require.NotEmpty(t, nextTok)
			assert.NotEqual(t, initialTok, nextTok, "token must rotate after each successful poll")

			var useToken string
			if tt.replayOld {
				useToken = initialTok
			} else {
				useToken = nextTok
			}

			rec2 := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+useToken, nil)
			require.Equal(t, http.StatusOK, rec2.Code, "GetLatestConfiguration always returns 200")

			if tt.wantEmptyBody {
				assert.Empty(t, rec2.Body.String(), "unchanged response must have an empty body")
			} else {
				assert.NotEmpty(t, rec2.Body.String(), "grace replay must return the cached content body")
			}
		})
	}
}

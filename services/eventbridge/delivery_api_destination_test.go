package eventbridge_test

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/eventbridge"
)

// parseBasicAuth decodes an "Authorization: Basic <base64>" header value into
// its username and password.
func parseBasicAuth(authHeader string) (string, string, bool) {
	const prefix = "Basic "
	if !strings.HasPrefix(authHeader, prefix) {
		return "", "", false
	}
	decoded, err := base64.StdEncoding.DecodeString(authHeader[len(prefix):])
	if err != nil {
		return "", "", false
	}
	user, pass, ok := strings.Cut(string(decoded), ":")
	if !ok {
		return "", "", false
	}

	return user, pass, true
}

// capturedRequest records the salient parts of an inbound HTTP request so tests
// can assert what EventBridge delivered to an API destination.
type capturedRequest struct {
	header http.Header
	method string
	path   string
	query  string
	body   string
}

// recordingServer is an httptest server that captures every request it serves.
type recordingServer struct {
	server   *httptest.Server
	requests []capturedRequest
	mu       sync.Mutex
	status   int
}

func newRecordingServer(t *testing.T) *recordingServer {
	t.Helper()
	rs := &recordingServer{status: http.StatusOK}
	rs.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		rs.mu.Lock()
		rs.requests = append(rs.requests, capturedRequest{
			method: r.Method,
			path:   r.URL.Path,
			query:  r.URL.RawQuery,
			header: r.Header.Clone(),
			body:   string(body),
		})
		rs.mu.Unlock()
		w.WriteHeader(rs.status)
	}))
	t.Cleanup(rs.server.Close)

	return rs
}

func (rs *recordingServer) Requests() []capturedRequest {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	return append([]capturedRequest{}, rs.requests...)
}

func (rs *recordingServer) Count() int {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	return len(rs.requests)
}

// setupAPIDestination wires a connection + API destination pointing at endpoint,
// plus a rule+target so that PutEvents for source "api.svc" delivers to it.
func setupAPIDestination(
	t *testing.T,
	backend *eventbridge.InMemoryBackend,
	connInput eventbridge.CreateConnectionInput,
	endpoint, method string,
	rateLimit int,
) {
	t.Helper()

	conn, err := backend.CreateConnection(t.Context(), connInput)
	require.NoError(t, err)

	dest, err := backend.CreateAPIDestination(t.Context(), eventbridge.CreateAPIDestinationInput{
		Name:                         "dest-" + connInput.Name,
		ConnectionArn:                conn.ConnectionArn,
		InvocationEndpoint:           endpoint,
		HTTPMethod:                   method,
		InvocationRateLimitPerSecond: rateLimit,
	})
	require.NoError(t, err)

	_, err = backend.PutRule(t.Context(), eventbridge.PutRuleInput{
		Name:         "rule-" + connInput.Name,
		EventPattern: `{"source":["api.svc"]}`,
		State:        "ENABLED",
	})
	require.NoError(t, err)

	_, err = backend.PutTargets(t.Context(), "rule-"+connInput.Name, "default", []eventbridge.Target{
		{ID: "t1", Arn: dest.APIDestinationArn, Input: `{"hello":"world"}`},
	})
	require.NoError(t, err)
}

// TestAPIDestinationDelivery_Auth verifies real HTTP invocation of API
// destinations honouring each connection authorization type, the destination's
// HTTP method and endpoint, and the delivered body.
func TestAPIDestinationDelivery_Auth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		connInput  eventbridge.CreateConnectionInput
		assertReq  func(t *testing.T, req capturedRequest)
		name       string
		httpMethod string
	}{
		{
			name:       "api_key",
			httpMethod: http.MethodPost,
			connInput: eventbridge.CreateConnectionInput{
				Name:              "conn-apikey",
				AuthorizationType: "API_KEY",
				AuthParameters: &eventbridge.ConnectionAuthParameters{
					APIKeyAuthParameters: &eventbridge.ConnectionAPIKeyAuthParameters{
						APIKeyName:  "x-api-key",
						APIKeyValue: "supersecret",
					},
				},
			},
			assertReq: func(t *testing.T, req capturedRequest) {
				t.Helper()
				assert.Equal(t, http.MethodPost, req.method)
				assert.Equal(t, "supersecret", req.header.Get("X-Api-Key"))
				assert.JSONEq(t, `{"hello":"world"}`, req.body)
			},
		},
		{
			name:       "basic",
			httpMethod: http.MethodPut,
			connInput: eventbridge.CreateConnectionInput{
				Name:              "conn-basic",
				AuthorizationType: "BASIC",
				AuthParameters: &eventbridge.ConnectionAuthParameters{
					BasicAuthParameters: &eventbridge.ConnectionBasicAuthParameters{
						Username: "alice",
						Password: "pa55",
					},
				},
			},
			assertReq: func(t *testing.T, req capturedRequest) {
				t.Helper()
				assert.Equal(t, http.MethodPut, req.method)
				user, pass, ok := parseBasicAuth(req.header.Get("Authorization"))
				require.True(t, ok, "expected basic auth header")
				assert.Equal(t, "alice", user)
				assert.Equal(t, "pa55", pass)
			},
		},
		{
			name:       "custom_headers_and_query",
			httpMethod: http.MethodPost,
			connInput: eventbridge.CreateConnectionInput{
				Name:              "conn-http-params",
				AuthorizationType: "API_KEY",
				AuthParameters: &eventbridge.ConnectionAuthParameters{
					APIKeyAuthParameters: &eventbridge.ConnectionAPIKeyAuthParameters{
						APIKeyName:  "x-api-key",
						APIKeyValue: "k",
					},
					InvocationHTTPParameters: &eventbridge.ConnectionHTTPParameters{
						HeaderParameters: []eventbridge.ConnectionHeaderParameter{
							{Key: "X-Custom", Value: "custom-val"},
						},
						QueryStringParameters: []eventbridge.ConnectionQueryStringParameter{
							{Key: "trace", Value: "abc"},
						},
					},
				},
			},
			assertReq: func(t *testing.T, req capturedRequest) {
				t.Helper()
				assert.Equal(t, "custom-val", req.header.Get("X-Custom"))
				assert.Contains(t, req.query, "trace=abc")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := newRecordingServer(t)
			backend := newBackend()
			backend.SetDeliveryTargets(&eventbridge.DeliveryTargets{})

			setupAPIDestination(t, backend, tt.connInput, server.server.URL+"/webhook", tt.httpMethod, 0)

			backend.PutEvents(t.Context(), []eventbridge.EventEntry{
				{Source: "api.svc", DetailType: "Evt", Detail: `{}`},
			})

			require.Eventually(t, func() bool {
				return server.Count() > 0
			}, 3*time.Second, 10*time.Millisecond)

			reqs := server.Requests()
			require.Len(t, reqs, 1)
			assert.Equal(t, "/webhook", reqs[0].path)
			tt.assertReq(t, reqs[0])
		})
	}
}

// TestAPIDestinationDelivery_OAuth verifies the OAuth client-credentials flow:
// EventBridge fetches a bearer token from the connection's authorization
// endpoint and presents it to the destination.
func TestAPIDestinationDelivery_OAuth(t *testing.T) {
	t.Parallel()

	var (
		tokenHits int
		tokenMu   sync.Mutex
	)
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenMu.Lock()
		tokenHits++
		tokenMu.Unlock()
		// Client credentials must arrive via Basic auth.
		user, pass, ok := parseBasicAuth(r.Header.Get("Authorization"))
		if !ok || user != "client-id" || pass != "client-secret" {
			w.WriteHeader(http.StatusUnauthorized)

			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"tok-xyz","token_type":"bearer"}`)
	}))
	t.Cleanup(tokenServer.Close)

	destServer := newRecordingServer(t)
	backend := newBackend()
	backend.SetDeliveryTargets(&eventbridge.DeliveryTargets{})

	setupAPIDestination(t, backend, eventbridge.CreateConnectionInput{
		Name:              "conn-oauth",
		AuthorizationType: "OAUTH_CLIENT_CREDENTIALS",
		AuthParameters: &eventbridge.ConnectionAuthParameters{
			OAuthParameters: &eventbridge.ConnectionOAuthParameters{
				AuthorizationEndpoint: tokenServer.URL + "/token",
				HTTPMethod:            http.MethodPost,
				ClientParameters: &eventbridge.ConnectionOAuthClientParameters{
					ClientID:     "client-id",
					ClientSecret: "client-secret",
				},
			},
		},
	}, destServer.server.URL+"/hook", http.MethodPost, 0)

	backend.PutEvents(t.Context(), []eventbridge.EventEntry{
		{Source: "api.svc", DetailType: "Evt", Detail: `{}`},
	})

	require.Eventually(t, func() bool {
		return destServer.Count() > 0
	}, 3*time.Second, 10*time.Millisecond)

	reqs := destServer.Requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, "Bearer tok-xyz", reqs[0].header.Get("Authorization"))

	tokenMu.Lock()
	defer tokenMu.Unlock()
	assert.Positive(t, tokenHits, "token endpoint should have been called")
}

// TestAPIDestinationDelivery_BodyMerge verifies connection invocation body
// parameters are merged into the delivered JSON payload.
func TestAPIDestinationDelivery_BodyMerge(t *testing.T) {
	t.Parallel()

	server := newRecordingServer(t)
	backend := newBackend()
	backend.SetDeliveryTargets(&eventbridge.DeliveryTargets{})

	setupAPIDestination(t, backend, eventbridge.CreateConnectionInput{
		Name:              "conn-bodymerge",
		AuthorizationType: "API_KEY",
		AuthParameters: &eventbridge.ConnectionAuthParameters{
			APIKeyAuthParameters: &eventbridge.ConnectionAPIKeyAuthParameters{
				APIKeyName:  "x-api-key",
				APIKeyValue: "k",
			},
			InvocationHTTPParameters: &eventbridge.ConnectionHTTPParameters{
				BodyParameters: []eventbridge.ConnectionBodyParameter{
					{Key: "injected", Value: "yes"},
				},
			},
		},
	}, server.server.URL+"/hook", http.MethodPost, 0)

	backend.PutEvents(t.Context(), []eventbridge.EventEntry{
		{Source: "api.svc", DetailType: "Evt", Detail: `{}`},
	})

	require.Eventually(t, func() bool {
		return server.Count() > 0
	}, 3*time.Second, 10*time.Millisecond)

	reqs := server.Requests()
	require.Len(t, reqs, 1)

	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(reqs[0].body), &got))
	assert.Equal(t, "world", got["hello"])
	assert.Equal(t, "yes", got["injected"])
}

// TestAPIDestinationDelivery_TargetHTTPParameters verifies that a target's own
// HttpParameters (PutTargets Target.HttpParameters, not the connection's
// InvocationHttpParameters) are applied to the outbound request, and that on
// a conflicting key the connection's value wins -- matching
// aws-sdk-go-v2/service/eventbridge@v1.48.4 types.go's documented precedence
// for Target.HttpParameters ("In case of any conflicting keys, values from
// the Connection take precedence"). Previously deliverToAPIDestination never
// read target.HTTPParameters at all, so it was accepted and stored by
// PutTargets but silently dropped at delivery time.
func TestAPIDestinationDelivery_TargetHTTPParameters(t *testing.T) {
	t.Parallel()

	server := newRecordingServer(t)
	backend := newBackend()
	backend.SetDeliveryTargets(&eventbridge.DeliveryTargets{})

	conn, err := backend.CreateConnection(t.Context(), eventbridge.CreateConnectionInput{
		Name:              "conn-target-http-params",
		AuthorizationType: "API_KEY",
		AuthParameters: &eventbridge.ConnectionAuthParameters{
			APIKeyAuthParameters: &eventbridge.ConnectionAPIKeyAuthParameters{
				APIKeyName:  "x-api-key",
				APIKeyValue: "k",
			},
			InvocationHTTPParameters: &eventbridge.ConnectionHTTPParameters{
				HeaderParameters: []eventbridge.ConnectionHeaderParameter{
					{Key: "X-Conflict", Value: "from-connection"},
				},
			},
		},
	})
	require.NoError(t, err)

	dest, err := backend.CreateAPIDestination(t.Context(), eventbridge.CreateAPIDestinationInput{
		Name:               "dest-target-http-params",
		ConnectionArn:      conn.ConnectionArn,
		InvocationEndpoint: server.server.URL + "/webhook",
		HTTPMethod:         http.MethodPost,
	})
	require.NoError(t, err)

	_, err = backend.PutRule(t.Context(), eventbridge.PutRuleInput{
		Name:         "rule-target-http-params",
		EventPattern: `{"source":["api.svc"]}`,
		State:        "ENABLED",
	})
	require.NoError(t, err)

	_, err = backend.PutTargets(t.Context(), "rule-target-http-params", "default", []eventbridge.Target{
		{
			ID:  "t1",
			Arn: dest.APIDestinationArn,
			HTTPParameters: &eventbridge.HTTPParameters{
				HeaderParameters: map[string]string{
					"X-From-Target": "target-value",
					"X-Conflict":    "from-target",
				},
				QueryStringParameters: map[string]string{"trace": "target-trace"},
			},
		},
	})
	require.NoError(t, err)

	backend.PutEvents(t.Context(), []eventbridge.EventEntry{
		{Source: "api.svc", DetailType: "Evt", Detail: `{}`},
	})

	require.Eventually(t, func() bool {
		return server.Count() > 0
	}, 3*time.Second, 10*time.Millisecond)

	reqs := server.Requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, "target-value", reqs[0].header.Get("X-From-Target"), "target-only header must be forwarded")
	assert.Contains(t, reqs[0].query, "trace=target-trace", "target-only query param must be forwarded")
	assert.Equal(t, "from-connection", reqs[0].header.Get("X-Conflict"),
		"on a conflicting key the connection's value must win over the target's")
}

// TestAPIDestinationDelivery_MissingDestinationGoesToDLQ verifies that an
// unknown API-destination ARN is treated as a delivery failure and routed to
// the DLQ rather than silently dropped.
func TestAPIDestinationDelivery_MissingDestinationGoesToDLQ(t *testing.T) {
	t.Parallel()

	dlq := newMockSQSSender()
	backend := newBackend()
	backend.SetDeliveryTargets(&eventbridge.DeliveryTargets{SQS: dlq})

	_, err := backend.PutRule(t.Context(), eventbridge.PutRuleInput{
		Name:         "missing-dest-rule",
		EventPattern: `{"source":["api.svc"]}`,
		State:        "ENABLED",
	})
	require.NoError(t, err)

	dlqARN := "arn:aws:sqs:us-east-1:123456789012:apidest-dlq"
	_, err = backend.PutTargets(t.Context(), "missing-dest-rule", "default", []eventbridge.Target{
		{
			ID:               "t1",
			Arn:              "arn:aws:events:us-east-1:123456789012:api-destination/does-not-exist/abc",
			Input:            `{"x":1}`,
			DeadLetterConfig: &eventbridge.DeadLetterConfig{Arn: dlqARN},
			RetryPolicy:      &eventbridge.RetryPolicy{MaximumRetryAttempts: 0},
		},
	})
	require.NoError(t, err)

	backend.PutEvents(t.Context(), []eventbridge.EventEntry{
		{Source: "api.svc", DetailType: "Evt", Detail: `{}`},
	})

	require.Eventually(t, func() bool {
		return len(dlq.MessagesFor(dlqARN)) > 0
	}, 3*time.Second, 10*time.Millisecond)
}

// TestAPIDestinationRateLimit asserts that WaitAPIDestinationRateLimit spaces
// requests to a destination according to its configured rate.
func TestAPIDestinationRateLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		arn         string
		rate        int
		calls       int
		wantMinWait time.Duration
	}{
		{
			name:        "rate_20_four_calls_spaced",
			arn:         "arn:aws:events:us-east-1:123456789012:api-destination/rl/abc",
			rate:        20, // 50ms interval
			calls:       4,  // 3 intervals ≈ 150ms
			wantMinWait: 100 * time.Millisecond,
		},
		{
			name:        "zero_rate_no_wait",
			arn:         "arn:aws:events:us-east-1:123456789012:api-destination/norl/abc",
			rate:        0,
			calls:       5,
			wantMinWait: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := newBackend()
			start := time.Now()
			for range tt.calls {
				backend.WaitAPIDestinationRateLimit(t.Context(), tt.arn, tt.rate)
			}
			elapsed := time.Since(start)

			assert.GreaterOrEqual(t, elapsed, tt.wantMinWait,
				"expected at least %s across %d calls at rate %d, got %s",
				tt.wantMinWait, tt.calls, tt.rate, elapsed)
		})
	}
}

// TestConnection_SecretsMaskedButUsable confirms that connection secrets are
// masked in Describe output yet remain usable for outbound delivery.
func TestConnection_SecretsMaskedInDescribe(t *testing.T) {
	t.Parallel()

	backend := newBackend()
	_, err := backend.CreateConnection(t.Context(), eventbridge.CreateConnectionInput{
		Name:              "masked-conn",
		AuthorizationType: "API_KEY",
		AuthParameters: &eventbridge.ConnectionAuthParameters{
			APIKeyAuthParameters: &eventbridge.ConnectionAPIKeyAuthParameters{
				APIKeyName:  "x-key",
				APIKeyValue: "should-not-leak",
			},
		},
	})
	require.NoError(t, err)

	desc, err := backend.DescribeConnection(t.Context(), "masked-conn")
	require.NoError(t, err)
	require.NotNil(t, desc.AuthParameters)
	require.NotNil(t, desc.AuthParameters.APIKeyAuthParameters)
	assert.Equal(t, "x-key", desc.AuthParameters.APIKeyAuthParameters.APIKeyName)
	assert.Empty(t, desc.AuthParameters.APIKeyAuthParameters.APIKeyValue,
		"secret value must be masked in Describe output")
}

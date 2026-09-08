package appsync_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/appsync"
	"github.com/blackbirdworks/gopherstack/services/cognitoidp"
)

// setupExecutableAPI creates a GraphQL API with the given auth configuration
// and a trivial "hello" query resolved by a NONE data source, so the caller
// only needs to worry about the auth outcome, not query execution.
func setupExecutableAPI(
	t *testing.T,
	b *appsync.InMemoryBackend,
	authType appsync.AuthenticationType,
	additional []appsync.AdditionalAuthenticationProvider,
	cfg *appsync.GraphqlAPIConfig,
) *appsync.GraphqlAPI {
	t.Helper()

	api, err := b.CreateGraphqlAPI("TestAPI", authType, false, "", "", additional, nil, cfg)
	require.NoError(t, err)
	_, err = b.StartSchemaCreation(api.APIID, `type Query { hello: String }`)
	require.NoError(t, err)
	_, err = b.CreateDataSource(api.APIID, &appsync.DataSource{Name: "NoneDS", Type: appsync.DataSourceTypeNone})
	require.NoError(t, err)
	_, err = b.CreateResolver(api.APIID, "Query", &appsync.Resolver{FieldName: "hello", DataSourceName: "NoneDS"})
	require.NoError(t, err)

	return api
}

func TestExecuteGraphQL_APIKeyAuth(t *testing.T) {
	t.Parallel()

	t.Run("valid_key_authorizes", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend()
		api := setupExecutableAPI(t, b, appsync.AuthTypeAPIKey, nil, nil)
		key, err := b.CreateAPIKey(api.APIID, "", 0)
		require.NoError(t, err)

		_, err = b.ExecuteGraphQL(
			t.Context(), api.APIID, `query { hello }`, "", nil, appsync.GraphQLAuth{APIKey: key.ID},
		)
		require.NoError(t, err)
	})

	t.Run("missing_key_rejected", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend()
		api := setupExecutableAPI(t, b, appsync.AuthTypeAPIKey, nil, nil)

		_, err := b.ExecuteGraphQL(t.Context(), api.APIID, `query { hello }`, "", nil, appsync.GraphQLAuth{})
		require.ErrorIs(t, err, appsync.ErrUnauthorized)
	})

	t.Run("wrong_key_rejected", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend()
		api := setupExecutableAPI(t, b, appsync.AuthTypeAPIKey, nil, nil)
		_, err := b.CreateAPIKey(api.APIID, "", 0)
		require.NoError(t, err)

		_, err = b.ExecuteGraphQL(
			t.Context(), api.APIID, `query { hello }`, "", nil, appsync.GraphQLAuth{APIKey: "da2-not-a-real-key"},
		)
		require.ErrorIs(t, err, appsync.ErrUnauthorized)
	})

	t.Run("expired_key_rejected", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			b := newTestBackend()
			api := setupExecutableAPI(t, b, appsync.AuthTypeAPIKey, nil, nil)

			expires := time.Now().Add(2 * 24 * time.Hour).Unix()
			key, err := b.CreateAPIKey(api.APIID, "", expires)
			require.NoError(t, err)

			auth := appsync.GraphQLAuth{APIKey: key.ID}

			_, err = b.ExecuteGraphQL(t.Context(), api.APIID, `query { hello }`, "", nil, auth)
			require.NoError(t, err, "key must still be live before its expiry")

			time.Sleep(3 * 24 * time.Hour)
			synctest.Wait()

			_, err = b.ExecuteGraphQL(t.Context(), api.APIID, `query { hello }`, "", nil, auth)
			require.ErrorIs(t, err, appsync.ErrUnauthorized, "key must be rejected once past its Expires")
		})
	})
}

func TestHandleGraphQL_APIKeyAuth_HTTPShape(t *testing.T) {
	t.Parallel()

	h, b := newTestHandler()
	api := setupExecutableAPI(t, b, appsync.AuthTypeAPIKey, nil, nil)
	key, err := b.CreateAPIKey(api.APIID, "", 0)
	require.NoError(t, err)

	body := map[string]any{"query": `query { hello }`}
	path := "/v1/apis/" + api.APIID + "/graphql"

	t.Run("valid_key_returns_200", func(t *testing.T) {
		t.Parallel()

		rec := doRequestWithHeaders(t, h, path, body, map[string]string{"x-api-key": key.ID})
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("missing_key_returns_401_unauthorized_body", func(t *testing.T) {
		t.Parallel()

		rec := doRequest(t, h, http.MethodPost, path, body)
		require.Equal(t, http.StatusUnauthorized, rec.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		// Real AppSync's transport-level auth failure body is exactly
		// {"message":"Unauthorized"} -- no "code"/"__type" field, unlike this
		// package's control-plane errorResponse() convention.
		assert.Equal(t, map[string]any{"message": "Unauthorized"}, resp)
	})
}

func TestExecuteGraphQL_LambdaAuth(t *testing.T) {
	t.Parallel()

	const authorizerARN = "arn:aws:lambda:us-east-1:000000000000:function:authorizer"

	setup := func(t *testing.T, mock *mockLambdaInvoker) (*appsync.InMemoryBackend, *appsync.GraphqlAPI) {
		t.Helper()

		b := newTestBackend()
		b.SetLambdaInvoker(mock)
		api := setupExecutableAPI(t, b, appsync.AuthTypeLambda, nil, &appsync.GraphqlAPIConfig{
			LambdaAuthorizerConfig: &appsync.LambdaAuthorizerConfig{AuthorizerURI: authorizerARN},
		})

		return b, api
	}

	t.Run("authorized_grants_access", func(t *testing.T) {
		t.Parallel()

		mock := &mockLambdaInvoker{payload: []byte(`{"isAuthorized":true}`)}
		b, api := setup(t, mock)

		_, err := b.ExecuteGraphQL(
			t.Context(), api.APIID, `query { hello }`, "", nil, appsync.GraphQLAuth{AuthToken: "secret-tok"},
		)
		require.NoError(t, err)
		require.Len(t, mock.calls, 1)
		assert.Equal(t, authorizerARN, mock.calls[0].name)
		assert.Equal(t, "RequestResponse", mock.calls[0].invType)

		var event struct {
			RequestContext struct {
				APIID       string `json:"apiId"`
				QueryString string `json:"queryString"`
			} `json:"requestContext"`
			AuthorizationToken string `json:"authorizationToken"`
		}
		require.NoError(t, json.Unmarshal(mock.calls[0].payload, &event))
		assert.Equal(t, "secret-tok", event.AuthorizationToken)
		assert.Equal(t, api.APIID, event.RequestContext.APIID)
		assert.Contains(t, event.RequestContext.QueryString, "hello")
	})

	t.Run("denied_rejects", func(t *testing.T) {
		t.Parallel()

		mock := &mockLambdaInvoker{payload: []byte(`{"isAuthorized":false}`)}
		b, api := setup(t, mock)

		_, err := b.ExecuteGraphQL(
			t.Context(), api.APIID, `query { hello }`, "", nil, appsync.GraphQLAuth{AuthToken: "secret-tok"},
		)
		require.ErrorIs(t, err, appsync.ErrUnauthorized)
	})

	t.Run("missing_token_rejects_without_invoking", func(t *testing.T) {
		t.Parallel()

		mock := &mockLambdaInvoker{payload: []byte(`{"isAuthorized":true}`)}
		b, api := setup(t, mock)

		_, err := b.ExecuteGraphQL(t.Context(), api.APIID, `query { hello }`, "", nil, appsync.GraphQLAuth{})
		require.ErrorIs(t, err, appsync.ErrUnauthorized)
		assert.Empty(t, mock.calls, "must not invoke the authorizer without a token to check")
	})
}

// signedIAMRequest builds a POST /graphql request signed with the AWS SDK v4
// signer, matching the shape services/apigatewayv2 and pkgs/httputils tests
// use to exercise SigV4 verification.
func signedIAMRequest(t *testing.T, secret string) *http.Request {
	t.Helper()

	body := `{"query":"query { hello }"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/apis/x/graphql", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	sum := sha256.Sum256([]byte(body))
	payloadHash := hex.EncodeToString(sum[:])

	signer := v4.NewSigner()
	creds := aws.Credentials{AccessKeyID: "AKIDEXAMPLE", SecretAccessKey: secret}
	signErr := signer.SignHTTP(context.Background(), creds, req, payloadHash, "appsync", "us-east-1", time.Now())
	require.NoError(t, signErr)

	return req
}

func TestExecuteGraphQL_IAMAuth(t *testing.T) {
	t.Parallel()

	setup := func(t *testing.T) (*appsync.InMemoryBackend, *appsync.GraphqlAPI) {
		t.Helper()

		b := newTestBackend()
		api := setupExecutableAPI(t, b, appsync.AuthTypeIAM, nil, nil)

		return b, api
	}

	t.Run("valid_signature_authorizes", func(t *testing.T) {
		t.Parallel()

		b, api := setup(t)
		req := signedIAMRequest(t, "test") // httputils.SigV4Validator's default secret

		_, err := b.ExecuteGraphQL(
			t.Context(), api.APIID, `query { hello }`, "", nil, appsync.GraphQLAuth{Request: req},
		)
		require.NoError(t, err)
	})

	t.Run("wrong_secret_rejected", func(t *testing.T) {
		t.Parallel()

		b, api := setup(t)
		req := signedIAMRequest(t, "wrong-secret")

		_, err := b.ExecuteGraphQL(
			t.Context(), api.APIID, `query { hello }`, "", nil, appsync.GraphQLAuth{Request: req},
		)
		require.ErrorIs(t, err, appsync.ErrUnauthorized)
	})

	t.Run("unsigned_request_rejected", func(t *testing.T) {
		t.Parallel()

		b, api := setup(t)
		req := httptest.NewRequest(http.MethodPost, "/v1/apis/x/graphql", nil)

		_, err := b.ExecuteGraphQL(
			t.Context(), api.APIID, `query { hello }`, "", nil, appsync.GraphQLAuth{Request: req},
		)
		require.ErrorIs(t, err, appsync.ErrUnauthorized)
	})

	t.Run("nil_request_rejected", func(t *testing.T) {
		t.Parallel()

		b, api := setup(t)

		_, err := b.ExecuteGraphQL(t.Context(), api.APIID, `query { hello }`, "", nil, appsync.GraphQLAuth{})
		require.ErrorIs(t, err, appsync.ErrUnauthorized)
	})
}

func TestExecuteGraphQL_AdditionalAuthenticationProviders(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	api := setupExecutableAPI(t, b, appsync.AuthTypeIAM, []appsync.AdditionalAuthenticationProvider{
		{AuthenticationType: appsync.AuthTypeAPIKey},
	}, nil)
	key, err := b.CreateAPIKey(api.APIID, "", 0)
	require.NoError(t, err)

	// Primary auth is AWS_IAM; the caller presents only the API key that's
	// configured as an additional provider and no SigV4 signature at all --
	// real AppSync authorizes if ANY configured provider accepts the request.
	_, err = b.ExecuteGraphQL(t.Context(), api.APIID, `query { hello }`, "", nil, appsync.GraphQLAuth{APIKey: key.ID})
	require.NoError(t, err)

	// Neither the primary nor the additional provider's credential is
	// presented -- must still be rejected.
	_, err = b.ExecuteGraphQL(t.Context(), api.APIID, `query { hello }`, "", nil, appsync.GraphQLAuth{})
	require.ErrorIs(t, err, appsync.ErrUnauthorized)
}

// cognitoTestEndpoint is shared by every Cognito/OIDC test in this file: the
// appsync backend under test and the cognitoidp backend standing in for the
// wired JWKS provider must agree on it, since cognitoidp derives a pool's
// issuer as endpoint+"/"+poolID (services/cognitoidp/user_pools.go) and
// checkCognitoAuth reconstructs the same string to look the key up.
const cognitoTestEndpoint = "http://localhost:8000"

// cognitoFixture is what cognitoTestFixture builds: a real Cognito user pool
// with a signed-in user, and everything appsync needs to trust it.
type cognitoFixture struct {
	Backend *cognitoidp.InMemoryBackend
	PoolID  string
	// ClientID is the app client the signed-in user authenticated with.
	ClientID string
	// IssuerURL is the issuer appsync's UserPoolConfig/OpenIDConnectConfig
	// would need to name to match this pool.
	IssuerURL string
	// IDToken is a live RSA-signed ID token for the signed-in user.
	IDToken string
}

// cognitoTestFixture creates a real Cognito user pool, app client, and
// confirmed signed-in user. Reused by both the Cognito and OIDC accept-path
// tests: OPENID_CONNECT verification is reachable through the exact same
// JWKS mechanism whenever its Issuer names a pool like this one, which is
// the realistic local-dev OIDC setup (Cognito user pools are themselves
// OIDC-compliant issuers).
func cognitoTestFixture(t *testing.T) cognitoFixture {
	t.Helper()

	cognitoBk := cognitoidp.NewInMemoryBackend("000000000000", "us-east-1", cognitoTestEndpoint)

	pool, err := cognitoBk.CreateUserPool("test-pool")
	require.NoError(t, err)

	client, err := cognitoBk.CreateUserPoolClient(pool.ID, "test-client")
	require.NoError(t, err)

	_, err = cognitoBk.SignUp(client.ClientID, "testuser", "Password1!", nil)
	require.NoError(t, err)
	require.NoError(t, cognitoBk.AdminConfirmSignUp(pool.ID, "testuser"))

	result, err := cognitoBk.InitiateAuth(client.ClientID, "USER_PASSWORD_AUTH", "testuser", "Password1!")
	require.NoError(t, err)

	return cognitoFixture{
		Backend:   cognitoBk,
		PoolID:    pool.ID,
		ClientID:  client.ClientID,
		IssuerURL: cognitoTestEndpoint + "/" + pool.ID,
		IDToken:   result.Tokens.IDToken,
	}
}

func TestExecuteGraphQL_CognitoAuth(t *testing.T) {
	t.Parallel()

	t.Run("valid_token_authorizes", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend()
		fx := cognitoTestFixture(t)
		b.SetJWKSProvider(fx.Backend)

		api := setupExecutableAPI(t, b, appsync.AuthTypeCognito, nil, &appsync.GraphqlAPIConfig{
			UserPoolConfig: &appsync.UserPoolConfig{
				UserPoolID: fx.PoolID, AWSRegion: "us-east-1", DefaultAction: "ALLOW", AppIDClientRegex: fx.ClientID,
			},
		})

		_, err := b.ExecuteGraphQL(t.Context(), api.APIID, `query { hello }`, "", nil,
			appsync.GraphQLAuth{AuthToken: "Bearer " + fx.IDToken})
		require.NoError(t, err)
	})

	t.Run("unwired_jwks_provider_is_permissive", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend()
		// SetJWKSProvider deliberately not called.
		api := setupExecutableAPI(t, b, appsync.AuthTypeCognito, nil, &appsync.GraphqlAPIConfig{
			UserPoolConfig: &appsync.UserPoolConfig{
				UserPoolID: "some-pool", AWSRegion: "us-east-1", DefaultAction: "ALLOW",
			},
		})

		_, err := b.ExecuteGraphQL(t.Context(), api.APIID, `query { hello }`, "", nil, appsync.GraphQLAuth{})
		require.NoError(t, err, "an unwired JWKS provider must not reject valid traffic")
	})

	t.Run("garbage_token_rejected", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend()
		fx := cognitoTestFixture(t)
		b.SetJWKSProvider(fx.Backend)

		api := setupExecutableAPI(t, b, appsync.AuthTypeCognito, nil, &appsync.GraphqlAPIConfig{
			UserPoolConfig: &appsync.UserPoolConfig{
				UserPoolID: fx.PoolID, AWSRegion: "us-east-1", DefaultAction: "ALLOW",
			},
		})

		_, err := b.ExecuteGraphQL(t.Context(), api.APIID, `query { hello }`, "", nil,
			appsync.GraphQLAuth{AuthToken: "Bearer not-a-jwt"})
		require.ErrorIs(t, err, appsync.ErrUnauthorized)
	})

	t.Run("wrong_pool_configured_rejected", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend()
		fx := cognitoTestFixture(t)
		b.SetJWKSProvider(fx.Backend)

		api := setupExecutableAPI(t, b, appsync.AuthTypeCognito, nil, &appsync.GraphqlAPIConfig{
			UserPoolConfig: &appsync.UserPoolConfig{
				UserPoolID: "different-pool-id", AWSRegion: "us-east-1", DefaultAction: "ALLOW",
			},
		})

		_, err := b.ExecuteGraphQL(t.Context(), api.APIID, `query { hello }`, "", nil,
			appsync.GraphQLAuth{AuthToken: "Bearer " + fx.IDToken})
		require.ErrorIs(t, err, appsync.ErrUnauthorized)
	})

	t.Run("client_id_regex_mismatch_rejected", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend()
		fx := cognitoTestFixture(t)
		b.SetJWKSProvider(fx.Backend)

		api := setupExecutableAPI(t, b, appsync.AuthTypeCognito, nil, &appsync.GraphqlAPIConfig{
			UserPoolConfig: &appsync.UserPoolConfig{
				UserPoolID: fx.PoolID, AWSRegion: "us-east-1", DefaultAction: "ALLOW",
				AppIDClientRegex: "^nonexistent-client$",
			},
		})

		_, err := b.ExecuteGraphQL(t.Context(), api.APIID, `query { hello }`, "", nil,
			appsync.GraphQLAuth{AuthToken: "Bearer " + fx.IDToken})
		require.ErrorIs(t, err, appsync.ErrUnauthorized)
	})

	t.Run("expired_token_rejected", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			b := newTestBackend()
			fx := cognitoTestFixture(t)
			b.SetJWKSProvider(fx.Backend)

			api := setupExecutableAPI(t, b, appsync.AuthTypeCognito, nil, &appsync.GraphqlAPIConfig{
				UserPoolConfig: &appsync.UserPoolConfig{
					UserPoolID: fx.PoolID, AWSRegion: "us-east-1", DefaultAction: "ALLOW",
				},
			})

			auth := appsync.GraphQLAuth{AuthToken: "Bearer " + fx.IDToken}

			_, err := b.ExecuteGraphQL(t.Context(), api.APIID, `query { hello }`, "", nil, auth)
			require.NoError(t, err, "token must be live immediately after issuance")

			time.Sleep(2 * time.Hour) // well past Cognito's 1-hour ID token lifetime
			synctest.Wait()

			_, err = b.ExecuteGraphQL(t.Context(), api.APIID, `query { hello }`, "", nil, auth)
			require.ErrorIs(t, err, appsync.ErrUnauthorized, "token must be rejected once past its exp claim")
		})
	})
}

func TestExecuteGraphQL_OIDCAuth(t *testing.T) {
	t.Parallel()

	t.Run("valid_token_authorizes", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend()
		fx := cognitoTestFixture(t)
		b.SetJWKSProvider(fx.Backend)

		api := setupExecutableAPI(t, b, appsync.AuthTypeOIDC, nil, &appsync.GraphqlAPIConfig{
			OpenIDConnectConfig: &appsync.OpenIDConnectConfig{Issuer: fx.IssuerURL, ClientID: fx.ClientID},
		})

		_, err := b.ExecuteGraphQL(t.Context(), api.APIID, `query { hello }`, "", nil,
			appsync.GraphQLAuth{AuthToken: "Bearer " + fx.IDToken})
		require.NoError(t, err)
	})

	t.Run("unwired_jwks_provider_is_permissive", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend()
		api := setupExecutableAPI(t, b, appsync.AuthTypeOIDC, nil, &appsync.GraphqlAPIConfig{
			OpenIDConnectConfig: &appsync.OpenIDConnectConfig{Issuer: "https://accounts.example.com"},
		})

		_, err := b.ExecuteGraphQL(t.Context(), api.APIID, `query { hello }`, "", nil, appsync.GraphQLAuth{})
		require.NoError(t, err, "an unwired JWKS provider must not reject valid traffic")
	})

	// unknown_external_issuer_rejected proves the "not reachable" case for a
	// genuine external OIDC issuer (e.g. Auth0/Okta/real AWS) that this
	// gopherstack instance has no signing key for: even a validly-signed
	// token is rejected rather than trusted, because it can't be verified --
	// gopherstack does not fetch a real IdP's JWKS over the network. This is
	// deliberately NOT permissive; only an unwired provider is.
	t.Run("unknown_external_issuer_rejected", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend()
		fx := cognitoTestFixture(t)
		b.SetJWKSProvider(fx.Backend)

		api := setupExecutableAPI(t, b, appsync.AuthTypeOIDC, nil, &appsync.GraphqlAPIConfig{
			OpenIDConnectConfig: &appsync.OpenIDConnectConfig{Issuer: "https://accounts.google.com"},
		})

		_, err := b.ExecuteGraphQL(t.Context(), api.APIID, `query { hello }`, "", nil,
			appsync.GraphQLAuth{AuthToken: "Bearer " + fx.IDToken})
		require.ErrorIs(t, err, appsync.ErrUnauthorized)
	})

	t.Run("client_id_mismatch_rejected", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend()
		fx := cognitoTestFixture(t)
		b.SetJWKSProvider(fx.Backend)

		api := setupExecutableAPI(t, b, appsync.AuthTypeOIDC, nil, &appsync.GraphqlAPIConfig{
			OpenIDConnectConfig: &appsync.OpenIDConnectConfig{Issuer: fx.IssuerURL, ClientID: "some-other-client-id"},
		})

		_, err := b.ExecuteGraphQL(t.Context(), api.APIID, `query { hello }`, "", nil,
			appsync.GraphQLAuth{AuthToken: "Bearer " + fx.IDToken})
		require.ErrorIs(t, err, appsync.ErrUnauthorized)
	})
}

func TestExecuteGraphQL_CognitoOIDC_AdditionalAuthenticationProvider(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	fx := cognitoTestFixture(t)
	b.SetJWKSProvider(fx.Backend)

	// Primary is API_KEY; Cognito is only an additional provider. A caller
	// presenting nothing but a valid Cognito token, and no API key, must
	// still authorize.
	api := setupExecutableAPI(t, b, appsync.AuthTypeAPIKey,
		[]appsync.AdditionalAuthenticationProvider{
			{
				AuthenticationType: appsync.AuthTypeCognito,
				UserPoolConfig: &appsync.CognitoUserPoolConfig{
					UserPoolID: fx.PoolID, AWSRegion: "us-east-1", AppIDClient: fx.ClientID,
				},
			},
		},
		nil,
	)

	_, err := b.ExecuteGraphQL(t.Context(), api.APIID, `query { hello }`, "", nil,
		appsync.GraphQLAuth{AuthToken: "Bearer " + fx.IDToken})
	require.NoError(t, err)
}

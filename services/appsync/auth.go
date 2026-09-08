package appsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
)

// errUnexpectedJWTSigningMethod mirrors the sentinel
// services/apigatewayv2/http_proxy.go's enforceJWTAuthorizer uses for the
// same check -- reimplemented here because it's unexported in that package
// (no shared pkgs/ verifier exists to import instead).
var errUnexpectedJWTSigningMethod = errors.New("unexpected JWT signing method")

// ErrUnauthorized is returned when a GraphQL request's credentials don't
// satisfy the API's AuthenticationType (or any AdditionalAuthenticationProviders).
// The message matches the exact body real AppSync returns for a transport-level
// auth failure -- HTTP 401 with {"message":"Unauthorized"} (AWS re:Post
// "Resolve unauth errors for GraphQL requests in AWS AppSync": a 401 means
// credentials are missing/invalid for the configured auth mode, as opposed to
// a 200 with a GraphQL errors[].errorType "Unauthorized" for resolver-level
// field auth).
var ErrUnauthorized = errors.New("Unauthorized")

// GraphQLAuth carries the caller-presented credentials for a GraphQL request,
// extracted from HTTP headers by the caller (handleGraphQL) before
// ExecuteGraphQL runs the query. Only the field(s) relevant to the API's
// configured auth type need be set.
type GraphQLAuth struct {
	// Request is the full incoming HTTP request. Required only for AWS_IAM
	// (SigV4 signature verification needs the whole request to recompute the
	// canonical request); nil is fine for every other auth type.
	Request   *http.Request
	APIKey    string // x-api-key header
	AuthToken string // Authorization header, verbatim
}

// lambdaAuthorizerEvent is the event AppSync sends to a Lambda authorizer.
// Field shape verified against AWS docs ("Using Lambda authorization with
// AWS AppSync"): queryString/operationName/variables nest under requestContext.
type lambdaAuthorizerEvent struct {
	RequestContext     lambdaAuthorizerRequestContext `json:"requestContext"`
	AuthorizationToken string                         `json:"authorizationToken"`
}

type lambdaAuthorizerRequestContext struct {
	Variables     map[string]any `json:"variables"`
	APIID         string         `json:"apiId"`
	AccountID     string         `json:"accountId"`
	RequestID     string         `json:"requestId"`
	QueryString   string         `json:"queryString"`
	OperationName string         `json:"operationName"`
}

// lambdaAuthorizerResponse is the response contract for an AppSync Lambda
// authorizer. Only IsAuthorized is consulted here; DeniedFields/
// ResolverContext (field-level authorization and $ctx.identity.resolverContext
// propagation) are a separate, larger feature and are not implemented.
type lambdaAuthorizerResponse struct {
	IsAuthorized bool `json:"isAuthorized"`
}

// graphQLAuthConfig bundles the auth-relevant fields of one configured
// provider -- the API's primary AuthenticationType, or one entry of
// AdditionalAuthenticationProviders -- so authorizeGraphQL/checkAuthProvider
// take one parameter instead of accumulating one per auth type. Built by
// graphQLAuthConfigFromAPI / graphQLAuthConfigFromAdditional.
type graphQLAuthConfig struct {
	AuthType  AuthenticationType
	LambdaCfg *LambdaAuthorizerConfig
	// UserPoolID/ClientIDRegex are AMAZON_COGNITO_USER_POOLS's UserPoolConfig
	// (primary) / CognitoUserPoolConfig (additional) fields.
	UserPoolID    string
	ClientIDRegex string
	// OIDCIssuer/OIDCClientID are OPENID_CONNECT's OpenIDConnectConfig fields
	// (same struct for both primary and additional).
	OIDCIssuer   string
	OIDCClientID string
}

// graphQLAuthConfigFromAPI extracts the primary-auth-relevant fields from a
// GraphqlAPI snapshot. Must be called while still holding InMemoryBackend.mu
// (UpdateGraphqlAPI mutates *GraphqlAPI's fields in place under that lock),
// producing a value safe to read after the lock is released.
func graphQLAuthConfigFromAPI(api *GraphqlAPI) graphQLAuthConfig {
	cfg := graphQLAuthConfig{AuthType: api.AuthenticationType, LambdaCfg: api.LambdaAuthorizerConfig}

	if api.UserPoolConfig != nil {
		cfg.UserPoolID = api.UserPoolConfig.UserPoolID
		cfg.ClientIDRegex = api.UserPoolConfig.AppIDClientRegex
	}

	if api.OpenIDConnectConfig != nil {
		cfg.OIDCIssuer = api.OpenIDConnectConfig.Issuer
		cfg.OIDCClientID = api.OpenIDConnectConfig.ClientID
	}

	return cfg
}

// graphQLAuthConfigFromAdditional is graphQLAuthConfigFromAPI's counterpart
// for one AdditionalAuthenticationProviders entry.
func graphQLAuthConfigFromAdditional(p AdditionalAuthenticationProvider) graphQLAuthConfig {
	cfg := graphQLAuthConfig{AuthType: p.AuthenticationType, LambdaCfg: p.LambdaAuthorizerConfig}

	if p.UserPoolConfig != nil {
		cfg.UserPoolID = p.UserPoolConfig.UserPoolID
		cfg.ClientIDRegex = p.UserPoolConfig.AppIDClient
	}

	if p.OpenIDConnectConfig != nil {
		cfg.OIDCIssuer = p.OpenIDConnectConfig.Issuer
		cfg.OIDCClientID = p.OpenIDConnectConfig.ClientID
	}

	return cfg
}

// authorizeGraphQL checks the request's credentials against the API's primary
// AuthenticationType and, failing that, each AdditionalAuthenticationProviders
// entry -- a request authenticates if ANY configured provider accepts it,
// matching real AppSync's semantics for additional auth providers.
func (b *InMemoryBackend) authorizeGraphQL(
	ctx context.Context,
	apiID string,
	primary graphQLAuthConfig,
	additional []AdditionalAuthenticationProvider,
	query, operationName string,
	variables map[string]any,
	auth GraphQLAuth,
) error {
	if b.checkAuthProvider(ctx, apiID, primary, query, operationName, variables, auth) {
		return nil
	}

	for _, p := range additional {
		if b.checkAuthProvider(
			ctx, apiID, graphQLAuthConfigFromAdditional(p), query, operationName, variables, auth,
		) {
			return nil
		}
	}

	return ErrUnauthorized
}

// checkAuthProvider evaluates a single auth provider (primary or additional).
func (b *InMemoryBackend) checkAuthProvider(
	ctx context.Context,
	apiID string,
	cfg graphQLAuthConfig,
	query, operationName string,
	variables map[string]any,
	auth GraphQLAuth,
) bool {
	switch cfg.AuthType {
	case AuthTypeAPIKey:
		return b.checkAPIKeyAuth(apiID, auth.APIKey)
	case AuthTypeLambda:
		return b.checkLambdaAuth(ctx, apiID, cfg.LambdaCfg, query, operationName, variables, auth.AuthToken)
	case AuthTypeIAM:
		return b.checkIAMAuth(auth.Request)
	case AuthTypeCognito:
		return b.checkCognitoAuth(auth.AuthToken, cfg.UserPoolID, cfg.ClientIDRegex)
	case AuthTypeOIDC:
		return b.checkOIDCAuth(auth.AuthToken, cfg.OIDCIssuer, cfg.OIDCClientID)
	default:
		return false
	}
}

// checkAPIKeyAuth reports whether key is a live (non-expired) API key on apiID.
func (b *InMemoryBackend) checkAPIKeyAuth(apiID, key string) bool {
	if key == "" {
		return false
	}

	b.mu.RLock("checkAPIKeyAuth")
	defer b.mu.RUnlock()

	k, ok := b.apiKeys[apiID][key]
	if !ok {
		return false
	}

	return k.Expires <= 0 || k.Expires > time.Now().Unix()
}

// checkIAMAuth verifies the request's SigV4 Authorization header. gopherstack
// is a single-tenant simulator (see pkgs/httputils.SigV4Validator's doc
// comment), so verification is against a single configured secret rather than
// a per-caller IAM principal/policy.
func (b *InMemoryBackend) checkIAMAuth(r *http.Request) bool {
	if r == nil {
		return false
	}

	return httputils.NewSigV4Validator(b.sigv4Secret).Verify(r) == nil
}

// checkLambdaAuth invokes the configured Lambda authorizer with the AppSync
// authorizer event and reports whether it granted access.
func (b *InMemoryBackend) checkLambdaAuth(
	ctx context.Context,
	apiID string,
	cfg *LambdaAuthorizerConfig,
	query, operationName string,
	variables map[string]any,
	authToken string,
) bool {
	if cfg == nil || b.lambdaFn == nil || authToken == "" {
		return false
	}

	event := lambdaAuthorizerEvent{
		AuthorizationToken: authToken,
		RequestContext: lambdaAuthorizerRequestContext{
			APIID:         apiID,
			AccountID:     b.accountID,
			QueryString:   query,
			OperationName: operationName,
			Variables:     variables,
		},
	}

	payload, marshalErr := json.Marshal(event)
	if marshalErr != nil {
		return false
	}

	result, _, invokeErr := b.lambdaFn.InvokeFunction(ctx, cfg.AuthorizerURI, "RequestResponse", payload)
	if invokeErr != nil {
		return false
	}

	var resp lambdaAuthorizerResponse
	if jsonErr := json.Unmarshal(result, &resp); jsonErr != nil {
		return false
	}

	return resp.IsAuthorized
}

// checkCognitoAuth verifies authToken as an AMAZON_COGNITO_USER_POOLS bearer
// JWT. userPoolID identifies which user pool this API trusts; the expected
// issuer is reconstructed exactly as services/cognitoidp's own pools compute
// theirs (endpoint + "/" + poolID -- see user_pools.go), since gopherstack's
// local Cognito never issues the real "cognito-idp.<region>.amazonaws.com"
// issuer AWSRegion would imply. AWSRegion itself is therefore not consulted.
//
// If b.jwksProvider is unset (SetJWKSProvider never called -- see its doc
// comment), this passes the request through instead of rejecting it.
func (b *InMemoryBackend) checkCognitoAuth(authToken, userPoolID, clientIDRegex string) bool {
	if b.jwksProvider == nil {
		return true
	}

	if userPoolID == "" {
		return false
	}

	claims, ok := b.verifyJWTBearer(authToken, fmt.Sprintf("%s/%s", b.endpoint, userPoolID))
	if !ok {
		return false
	}

	return cognitoClientIDMatches(claims, clientIDRegex)
}

// checkOIDCAuth verifies authToken as an OPENID_CONNECT bearer JWT against
// the API's configured issuer. Real verification is reachable whenever the
// configured issuer is one this gopherstack instance actually knows the
// signing key for -- in practice, an OpenIDConnectConfig.Issuer pointed at
// one of services/cognitoidp's user pools (Cognito user pools are
// OIDC-compliant issuers, so this is the realistic local-dev OIDC setup).
// An issuer the JWKSProvider has never heard of has no key material to
// verify against and is rejected, same as a bad signature -- gopherstack
// does not fetch a real external IdP's JWKS document over the network.
//
// If b.jwksProvider is unset (SetJWKSProvider never called -- see its doc
// comment), this passes the request through instead of rejecting it.
func (b *InMemoryBackend) checkOIDCAuth(authToken, issuer, clientID string) bool {
	if b.jwksProvider == nil {
		return true
	}

	if issuer == "" {
		return false
	}

	claims, ok := b.verifyJWTBearer(authToken, issuer)
	if !ok {
		return false
	}

	if clientID == "" {
		return true
	}

	return slices.Contains(jwtClaimStrings(claims, "aud"), clientID)
}

// verifyJWTBearer parses and verifies authToken as a JWT bearer credential:
// RSA signature (via b.jwksProvider, keyed by wantIssuer+kid), a required
// non-expired exp claim, and iss == wantIssuer. On success it returns the
// token's claims. Mirrors services/apigatewayv2/http_proxy.go's
// enforceJWTAuthorizer (config-driven issuer used for both the key lookup
// and the iss check, not the token's own unauthenticated iss claim) --
// reimplemented here rather than imported because that package's helpers
// are unexported and there is no shared pkgs/ verifier.
func (b *InMemoryBackend) verifyJWTBearer(authToken, wantIssuer string) (jwt.MapClaims, bool) {
	if authToken == "" || wantIssuer == "" {
		return nil, false
	}

	tokenStr := strings.TrimPrefix(authToken, "Bearer ")
	tokenStr = strings.TrimPrefix(tokenStr, "bearer ")

	keyfunc := func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, errUnexpectedJWTSigningMethod
		}

		kid, _ := t.Header["kid"].(string)

		return b.jwksProvider.GetJWTPublicKey(wantIssuer, kid)
	}

	token, parseErr := jwt.Parse(tokenStr, keyfunc, jwt.WithExpirationRequired())
	if parseErr != nil {
		return nil, false
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, false
	}

	// Defense in depth: with the one JWKSProvider this codebase has
	// (cognitoidp), a successful key lookup already guarantees iss ==
	// wantIssuer (GetJWTPublicKey only returns a key scoped to the pool whose
	// issuerURL was requested), so this never actually fires today. Kept for
	// a hypothetically looser future JWKSProvider, matching enforceJWTAuthorizer.
	if iss, _ := claims["iss"].(string); iss != wantIssuer {
		return nil, false
	}

	return claims, true
}

// jwtClaimStrings normalizes a claim that may be a single string or a JSON
// array of strings (the "aud"/"client_id" shape) into a string slice.
func jwtClaimStrings(claims jwt.MapClaims, key string) []string {
	switch v := claims[key].(type) {
	case string:
		return []string{v}
	case []any:
		out := make([]string, 0, len(v))

		for _, e := range v {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}

		return out
	default:
		return nil
	}
}

// cognitoClientIDMatches reports whether pattern (UserPoolConfig's
// AppIdClientRegex, a regular expression per the real AppSync field) matches
// any candidate app client ID on claims -- checking both "client_id"
// (Cognito access tokens) and "aud" (Cognito ID tokens), since either token
// type may be presented. An empty pattern matches unconditionally (no
// AppIdClientRegex was configured); an invalid regex never matches.
func cognitoClientIDMatches(claims jwt.MapClaims, pattern string) bool {
	if pattern == "" {
		return true
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}

	candidates := append(jwtClaimStrings(claims, "client_id"), jwtClaimStrings(claims, "aud")...)

	return slices.ContainsFunc(candidates, re.MatchString)
}

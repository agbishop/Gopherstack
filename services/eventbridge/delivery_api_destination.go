package eventbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
)

const apiDestTimeout = 5 * time.Second

// deliverToAPIDestination performs a real HTTP invocation of an API destination:
// it resolves the destination's endpoint/method/rate-limit and the connection's
// auth, applies the connection's invocation HTTP parameters (headers, query
// string, and body merges), signs the request per the connection's
// authorization type, honours the destination's rate limit, and reports whether
// delivery failed (a transport error or a >=400 response), which drives
// retry/DLQ handling upstream.
func deliverToAPIDestination(
	ctx context.Context,
	resolver APIDestinationResolver,
	target *Target,
	payload string,
) bool {
	log := logger.Load(ctx)
	if resolver == nil {
		log.WarnContext(ctx, "EventBridge: no API-destination resolver configured", "arn", target.Arn)

		return true
	}

	dest, ok := resolver.ResolveAPIDestination(target.Arn)
	if !ok {
		log.WarnContext(ctx, "EventBridge: API destination not found", "arn", target.Arn)

		return true
	}

	// Throttle to the destination's configured invocation rate before sending.
	resolver.WaitAPIDestinationRateLimit(ctx, target.Arn, dest.RateLimitPerSecond)

	body := mergeBodyParameters(payload, dest.BodyParameters)

	method := dest.HTTPMethod
	if method == "" {
		method = http.MethodPost
	}

	req, err := http.NewRequestWithContext(ctx, method, dest.Endpoint, strings.NewReader(body))
	if err != nil {
		log.WarnContext(ctx, "EventBridge: failed to build API-destination request", "arn", target.Arn, "error", err)

		return true
	}
	req.Header.Set("Content-Type", "application/json")

	applyTargetHTTPParameters(req, target.HTTPParameters)
	applyConnectionHTTPParameters(req, dest.HeaderParameters, dest.QueryStringParameters)

	if authErr := applyAPIDestinationAuth(ctx, req, dest); authErr != nil {
		log.WarnContext(ctx, "EventBridge: failed to apply API-destination auth", "arn", target.Arn, "error", authErr)

		return true
	}

	client := &http.Client{Timeout: apiDestTimeout}
	resp, err := client.Do(req)
	if err != nil {
		log.WarnContext(ctx, "EventBridge: API-destination request failed", "arn", target.Arn, "error", err)

		return true
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	return resp.StatusCode >= http.StatusBadRequest
}

// mergeBodyParameters merges a connection's invocation body parameters into the
// event payload. When the payload is a JSON object the parameters are added as
// top-level keys (matching AWS, which merges connection body parameters into the
// request body); otherwise the payload is returned unchanged.
func mergeBodyParameters(payload string, params []ConnectionBodyParameter) string {
	if len(params) == 0 {
		return payload
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(payload), &obj); err != nil || obj == nil {
		return payload
	}

	for _, p := range params {
		obj[p.Key] = p.Value
	}

	merged, err := json.Marshal(obj)
	if err != nil {
		return payload
	}

	return string(merged)
}

// applyTargetHTTPParameters adds a PutTargets Target's own HttpParameters
// (as opposed to the connection's InvocationHttpParameters) to the outbound
// request. Applied before the connection's parameters so a conflicting key
// is overwritten by the connection's value, matching AWS's documented
// precedence for Target.HttpParameters.
func applyTargetHTTPParameters(req *http.Request, hp *HTTPParameters) {
	if hp == nil {
		return
	}

	for k, v := range hp.HeaderParameters {
		req.Header.Set(k, v)
	}

	if len(hp.QueryStringParameters) > 0 {
		q := req.URL.Query()
		for k, v := range hp.QueryStringParameters {
			q.Set(k, v)
		}
		req.URL.RawQuery = q.Encode()
	}
}

// applyConnectionHTTPParameters adds a connection's invocation header and
// query-string parameters to the outbound request.
func applyConnectionHTTPParameters(
	req *http.Request,
	headers []ConnectionHeaderParameter,
	queries []ConnectionQueryStringParameter,
) {
	for _, h := range headers {
		req.Header.Set(h.Key, h.Value)
	}

	if len(queries) > 0 {
		q := req.URL.Query()
		for _, qp := range queries {
			q.Set(qp.Key, qp.Value)
		}
		req.URL.RawQuery = q.Encode()
	}
}

// applyAPIDestinationAuth signs the outbound request according to the resolved
// connection's authorization type.
func applyAPIDestinationAuth(ctx context.Context, req *http.Request, dest *ResolvedAPIDestination) error {
	switch dest.AuthType {
	case connectionAuthAPIKey:
		if dest.APIKeyName != "" {
			req.Header.Set(dest.APIKeyName, dest.APIKeyValue)
		}
	case connectionAuthBasic:
		req.SetBasicAuth(dest.BasicUsername, dest.BasicPassword)
	case connectionAuthOAuth:
		if dest.OAuth == nil {
			return nil
		}
		token, err := fetchOAuthToken(ctx, dest.OAuth)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}

	return nil
}

// fetchOAuthToken performs an OAuth 2.0 client-credentials grant against the
// connection's authorization endpoint and returns the access token. Client
// credentials are sent via HTTP Basic auth and the grant_type in the form body,
// mirroring the common client-credentials flow AWS uses for OAuth connections.
func fetchOAuthToken(ctx context.Context, oauth *ResolvedOAuth) (string, error) {
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	for _, bp := range oauth.BodyParameters {
		form.Set(bp.Key, bp.Value)
	}

	method := oauth.HTTPMethod
	if method == "" {
		method = http.MethodPost
	}

	req, err := http.NewRequestWithContext(
		ctx, method, oauth.AuthorizationEndpoint, strings.NewReader(form.Encode()),
	)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(oauth.ClientID, oauth.ClientSecret)

	applyConnectionHTTPParameters(req, oauth.HeaderParameters, oauth.QueryStringParameters)

	client := &http.Client{Timeout: apiDestTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxOAuthResponseBytes))
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return "", fmt.Errorf("%w: OAuth token endpoint returned %d", ErrInvalidParameter, resp.StatusCode)
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err = json.Unmarshal(data, &tokenResp); err != nil {
		return "", err
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("%w: OAuth token endpoint returned no access_token", ErrInvalidParameter)
	}

	return tokenResp.AccessToken, nil
}

const maxOAuthResponseBytes = 1 << 20

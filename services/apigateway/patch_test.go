package apigateway_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/apigateway"
)

// setupStage creates a REST API, a deployment, and a stage pointing at it,
// returning the API ID alongside the ready-to-patch stage. variables/canary
// may be nil.
func setupStage(
	t *testing.T, h *apigateway.Handler, variables map[string]string, canary *apigateway.CanarySettings,
) (string, string) {
	t.Helper()

	api, err := h.Backend.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "patch-test-api"})
	require.NoError(t, err)

	depl, err := h.Backend.CreateDeployment(api.ID, "", "v1")
	require.NoError(t, err)

	_, err = h.Backend.CreateStage(apigateway.CreateStageInput{
		RestAPIID:      api.ID,
		StageName:      "prod",
		DeploymentID:   depl.ID,
		Variables:      variables,
		CanarySettings: canary,
	})
	require.NoError(t, err)

	return api.ID, "prod"
}

// Test_ApplyStructuredPatch_StageVariables exercises per-variable "/variables/{name}"
// PATCH ops (add/replace/remove, including a JSON-Pointer-escaped name), which
// the old flatten silently dropped (it took the whole remaining path
// "variables/foo" as one bogus flat field, matching no Update*Input json tag).
func Test_ApplyStructuredPatch_StageVariables(t *testing.T) {
	t.Parallel()

	cases := []struct {
		want map[string]string
		name string
		ops  string
	}{
		{
			name: "add a new variable merges with existing ones",
			ops:  `[{"op":"add","path":"/variables/c","value":"3"}]`,
			want: map[string]string{"a": "1", "b": "2", "c": "3"},
		},
		{
			name: "replace an existing variable leaves siblings untouched",
			ops:  `[{"op":"replace","path":"/variables/a","value":"9"}]`,
			want: map[string]string{"a": "9", "b": "2"},
		},
		{
			name: "remove deletes only the named variable",
			ops:  `[{"op":"remove","path":"/variables/b"}]`,
			want: map[string]string{"a": "1"},
		},
		{
			name: "JSON-Pointer-escaped variable name is unescaped",
			ops:  `[{"op":"add","path":"/variables/a~1b","value":"x"}]`,
			want: map[string]string{"a": "1", "b": "2", "a/b": "x"},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newAPIGWHandler()
			apiID, stageName := setupStage(t, h, map[string]string{"a": "1", "b": "2"}, nil)

			rec := restRequest(t, h, http.MethodPatch,
				fmt.Sprintf("/restapis/%s/stages/%s", apiID, stageName), tt.ops)
			require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

			stage, err := h.Backend.GetStage(apiID, stageName)
			require.NoError(t, err)
			assert.Equal(t, tt.want, stage.Variables)
		})
	}
}

// Test_ApplyStructuredPatch_StageCanaryPromotion exercises the AWS-documented
// canary-promotion pattern: a "copy" op from "/canarySettings/deploymentId" to
// "/deploymentId". The old flatten never implemented "copy" at all (only
// "add"/"replace" were honored), so promotion was a total no-op.
func Test_ApplyStructuredPatch_StageCanaryPromotion(t *testing.T) {
	t.Parallel()

	h := newAPIGWHandler()
	api, err := h.Backend.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "canary-promo-api"})
	require.NoError(t, err)

	mainDepl, err := h.Backend.CreateDeployment(api.ID, "", "v1")
	require.NoError(t, err)

	// The canary deployment must be a real Deployment on the same REST API --
	// UpdateStage now validates deploymentId (this sweep), matching
	// CreateStage's existing guard, so a fabricated id would be rejected here.
	canaryDepl, err := h.Backend.CreateDeployment(api.ID, "", "canary")
	require.NoError(t, err)

	_, err = h.Backend.CreateStage(apigateway.CreateStageInput{
		RestAPIID:    api.ID,
		StageName:    "prod",
		DeploymentID: mainDepl.ID,
		CanarySettings: &apigateway.CanarySettings{
			DeploymentID:   canaryDepl.ID,
			PercentTraffic: 50,
		},
	})
	require.NoError(t, err)

	apiID, stageName := api.ID, "prod"

	before, err := h.Backend.GetStage(apiID, stageName)
	require.NoError(t, err)
	require.NotEqual(t, canaryDepl.ID, before.DeploymentID, "precondition: main deployment differs from canary")

	body := `{"patchOperations":[{"op":"copy","from":"/canarySettings/deploymentId","path":"/deploymentId"}]}`
	rec := restRequest(t, h, http.MethodPatch, fmt.Sprintf("/restapis/%s/stages/%s", apiID, stageName), body)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	after, err := h.Backend.GetStage(apiID, stageName)
	require.NoError(t, err)
	assert.Equal(t, canaryDepl.ID, after.DeploymentID,
		"promoted deployment id should replace the stage's deploymentId")
}

// Test_ApplyStructuredPatch_StageMethodSettings exercises per-route method
// settings, addressed by AWS with NO "methodSettings" path prefix
// ("/{resourcePath}/{httpMethod}/{category}/{property}", e.g. "/*/*/logging/loglevel"
// or "/~1pets/GET/caching/enabled"). The old flatten had no notion of this
// path shape at all.
func Test_ApplyStructuredPatch_StageMethodSettings(t *testing.T) {
	t.Parallel()

	cases := []struct {
		check    func(t *testing.T, ms apigateway.MethodSetting)
		name     string
		path     string
		value    string
		routeKey string
	}{
		{
			name:     "wildcard throttling burst limit (string value coerced to int)",
			path:     "/*/*/throttling/burstLimit",
			value:    "500",
			routeKey: "*/*",
			check: func(t *testing.T, ms apigateway.MethodSetting) {
				t.Helper()
				assert.Equal(t, 500, ms.ThrottlingBurstLimit)
			},
		},
		{
			name:     "wildcard throttling rate limit (string value coerced to float)",
			path:     "/*/*/throttling/rateLimit",
			value:    "12.5",
			routeKey: "*/*",
			check: func(t *testing.T, ms apigateway.MethodSetting) {
				t.Helper()
				assert.InDelta(t, 12.5, ms.ThrottlingRateLimit, 0.001)
			},
		},
		{
			name:     "wildcard logging level",
			path:     "/*/*/logging/loglevel",
			value:    "INFO",
			routeKey: "*/*",
			check: func(t *testing.T, ms apigateway.MethodSetting) {
				t.Helper()
				assert.Equal(t, "INFO", ms.LoggingLevel)
			},
		},
		{
			name:     "wildcard metrics enabled (string value coerced to bool)",
			path:     "/*/*/metrics/enabled",
			value:    "true",
			routeKey: "*/*",
			check: func(t *testing.T, ms apigateway.MethodSetting) {
				t.Helper()
				assert.True(t, ms.MetricsEnabled)
			},
		},
		{
			name:     "escaped resource path and specific method",
			path:     "/~1pets/GET/caching/enabled",
			value:    "true",
			routeKey: "/pets/GET",
			check: func(t *testing.T, ms apigateway.MethodSetting) {
				t.Helper()
				assert.True(t, ms.CachingEnabled)
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newAPIGWHandler()
			apiID, stageName := setupStage(t, h, nil, nil)

			body := fmt.Sprintf(`[{"op":"replace","path":%q,"value":%q}]`, tt.path, tt.value)
			rec := restRequest(t, h, http.MethodPatch,
				fmt.Sprintf("/restapis/%s/stages/%s", apiID, stageName), body)
			require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

			stage, err := h.Backend.GetStage(apiID, stageName)
			require.NoError(t, err)
			require.Contains(t, stage.MethodSettings, tt.routeKey)
			tt.check(t, stage.MethodSettings[tt.routeKey])
		})
	}
}

// Test_ApplyStructuredPatch_StageMethodSettingsRemove verifies that "remove"
// resets a per-route property back to its zero value, merging with (not
// wiping) any other properties already set on that same route.
func Test_ApplyStructuredPatch_StageMethodSettingsRemove(t *testing.T) {
	t.Parallel()

	h := newAPIGWHandler()
	apiID, stageName := setupStage(t, h, nil, nil)

	setBody := `[
		{"op":"replace","path":"/*/*/throttling/burstLimit","value":"500"},
		{"op":"replace","path":"/*/*/logging/loglevel","value":"INFO"}
	]`
	rec := restRequest(t, h, http.MethodPatch, fmt.Sprintf("/restapis/%s/stages/%s", apiID, stageName), setBody)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	removeBody := `[{"op":"remove","path":"/*/*/throttling/burstLimit"}]`
	rec = restRequest(t, h, http.MethodPatch, fmt.Sprintf("/restapis/%s/stages/%s", apiID, stageName), removeBody)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	stage, err := h.Backend.GetStage(apiID, stageName)
	require.NoError(t, err)
	require.Contains(t, stage.MethodSettings, "*/*")
	assert.Equal(t, 0, stage.MethodSettings["*/*"].ThrottlingBurstLimit, "removed property resets to zero value")
	assert.Equal(t, "INFO", stage.MethodSettings["*/*"].LoggingLevel, "sibling property on the same route is untouched")
}

// Test_ApplyStructuredPatch_StageTopLevelCoercion exercises top-level
// boolean/string fields whose PATCH value is transmitted as a JSON string on
// the wire (per aws-sdk-go-v2's PatchOperation serializer). Before the fix,
// copying that raw JSON string into a *bool field made json.Unmarshal fail
// with a type mismatch, so these patches errored instead of applying.
func Test_ApplyStructuredPatch_StageTopLevelCoercion(t *testing.T) {
	t.Parallel()

	cases := []struct {
		check func(t *testing.T, stage *apigateway.Stage)
		name  string
		ops   string
	}{
		{
			name: "tracingEnabled string value coerces to bool",
			ops:  `[{"op":"replace","path":"/tracingEnabled","value":"true"}]`,
			check: func(t *testing.T, stage *apigateway.Stage) {
				t.Helper()
				assert.True(t, stage.TracingEnabled)
			},
		},
		{
			name: "cacheClusterEnabled string value coerces to bool and derives status",
			ops:  `[{"op":"replace","path":"/cacheClusterEnabled","value":"true"}]`,
			check: func(t *testing.T, stage *apigateway.Stage) {
				t.Helper()
				assert.True(t, stage.CacheClusterEnabled)
				assert.Equal(t, "AVAILABLE", stage.CacheClusterStatus)
			},
		},
		{
			name: "cacheClusterSize is a plain string field",
			ops:  `[{"op":"replace","path":"/cacheClusterSize","value":"0.5"}]`,
			check: func(t *testing.T, stage *apigateway.Stage) {
				t.Helper()
				assert.Equal(t, "0.5", stage.CacheClusterSize)
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newAPIGWHandler()
			apiID, stageName := setupStage(t, h, nil, nil)

			rec := restRequest(t, h, http.MethodPatch,
				fmt.Sprintf("/restapis/%s/stages/%s", apiID, stageName), tt.ops)
			require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

			stage, err := h.Backend.GetStage(apiID, stageName)
			require.NoError(t, err)
			tt.check(t, stage)
		})
	}
}

// Test_ApplyStructuredPatch_RestAPIBinaryMediaTypes exercises per-entry
// "/binaryMediaTypes/{escaped-media-type}" add/remove, which must merge with
// (not replace) the API's existing binary media types.
func Test_ApplyStructuredPatch_RestAPIBinaryMediaTypes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		ops  string
		want []string
	}{
		{
			name: "add merges with the existing type",
			ops:  `[{"op":"add","path":"/binaryMediaTypes/application~1octet-stream"}]`,
			want: []string{"image/png", "application/octet-stream"},
		},
		{
			name: "add is idempotent",
			ops:  `[{"op":"add","path":"/binaryMediaTypes/image~1png"}]`,
			want: []string{"image/png"},
		},
		{
			name: "remove drops only the named type",
			ops:  `[{"op":"remove","path":"/binaryMediaTypes/image~1png"}]`,
			want: []string{},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newAPIGWHandler()
			api, err := h.Backend.CreateRestAPI(apigateway.CreateRestAPIInput{
				Name:             "media-test-api",
				BinaryMediaTypes: []string{"image/png"},
			})
			require.NoError(t, err)

			rec := restRequest(t, h, http.MethodPatch, fmt.Sprintf("/restapis/%s", api.ID), tt.ops)
			require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

			got, err := h.Backend.GetRestAPI(api.ID)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.want, got.BinaryMediaTypes)
		})
	}
}

// Test_ApplyStructuredPatch_RestAPIMinimumCompressionSizeCoercion verifies the
// numeric top-level field /minimumCompressionSize (an *int) correctly parses
// its string-typed wire value instead of failing to unmarshal.
func Test_ApplyStructuredPatch_RestAPIMinimumCompressionSizeCoercion(t *testing.T) {
	t.Parallel()

	h := newAPIGWHandler()
	api, err := h.Backend.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "compression-test-api"})
	require.NoError(t, err)

	ops := `[{"op":"replace","path":"/minimumCompressionSize","value":"1024"}]`
	rec := restRequest(t, h, http.MethodPatch, fmt.Sprintf("/restapis/%s", api.ID), ops)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	got, err := h.Backend.GetRestAPI(api.ID)
	require.NoError(t, err)
	assert.Equal(t, 1024, got.MinimumCompressionSize)
}

// Test_ApplyStructuredPatch_Account exercises UpdateAccount's nested
// "/throttle/{rateLimit,burstLimit}" edits (merged across separate PATCH
// calls, since each only touches one sub-field) and the top-level
// "/cloudwatchRoleArn" field, which UpdateAccountInput previously lacked
// entirely (so it could never be set via UpdateAccount at all).
func Test_ApplyStructuredPatch_Account(t *testing.T) {
	t.Parallel()

	h := newAPIGWHandler()

	initial, err := h.Backend.GetAccount()
	require.NoError(t, err)
	require.NotNil(t, initial.ThrottleSettings, "the backend seeds default account throttle settings")
	initialBurstLimit := initial.ThrottleSettings.BurstLimit

	rec := restRequest(t, h, http.MethodPatch, "/account",
		`[{"op":"replace","path":"/cloudwatchRoleArn","value":"arn:aws:iam::123456789012:role/apigw"}]`)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	acct, err := h.Backend.GetAccount()
	require.NoError(t, err)
	assert.Equal(t, "arn:aws:iam::123456789012:role/apigw", acct.CloudwatchRoleARN)

	rec = restRequest(t, h, http.MethodPatch, "/account",
		`[{"op":"replace","path":"/throttle/rateLimit","value":"100.5"}]`)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	acct, err = h.Backend.GetAccount()
	require.NoError(t, err)
	require.NotNil(t, acct.ThrottleSettings)
	assert.InDelta(t, 100.5, acct.ThrottleSettings.RateLimit, 0.001)
	assert.Equal(t, initialBurstLimit, acct.ThrottleSettings.BurstLimit, "untouched sub-field keeps its prior value")

	rec = restRequest(t, h, http.MethodPatch, "/account",
		`[{"op":"replace","path":"/throttle/burstLimit","value":"50"}]`)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	acct, err = h.Backend.GetAccount()
	require.NoError(t, err)
	require.NotNil(t, acct.ThrottleSettings)
	assert.InDelta(t, 100.5, acct.ThrottleSettings.RateLimit, 0.001, "earlier rateLimit patch must not be clobbered")
	assert.Equal(t, 50, acct.ThrottleSettings.BurstLimit)
	assert.Equal(t, "arn:aws:iam::123456789012:role/apigw", acct.CloudwatchRoleARN, "unrelated field untouched")
}

// Test_ApplyStructuredPatch_UsagePlanAPIStages exercises the single-segment
// "/apiStages" add/remove membership edit, where AWS's Value is the string
// "{restApiId}:{stage}" (not a nested path). It also proves the companion fix
// to InMemoryBackend.UpdateUsagePlan, which previously only applied APIStages
// when non-empty (`len(...) > 0`), silently ignoring a patch that removes the
// last remaining API stage.
func Test_ApplyStructuredPatch_UsagePlanAPIStages(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		op   string
		want []apigateway.APIStageAssociation
	}{
		{
			name: "add merges with the existing stage",
			op:   `{"op":"add","path":"/apiStages","value":"api2:prod"}`,
			want: []apigateway.APIStageAssociation{
				{RestAPIID: "api1", Stage: "dev"},
				{RestAPIID: "api2", Stage: "prod"},
			},
		},
		{
			name: "add is idempotent",
			op:   `{"op":"add","path":"/apiStages","value":"api1:dev"}`,
			want: []apigateway.APIStageAssociation{{RestAPIID: "api1", Stage: "dev"}},
		},
		{
			name: "remove the only stage empties the list",
			op:   `{"op":"remove","path":"/apiStages","value":"api1:dev"}`,
			want: []apigateway.APIStageAssociation{},
		},
		{
			name: "remove a non-member stage is a no-op",
			op:   `{"op":"remove","path":"/apiStages","value":"api9:staging"}`,
			want: []apigateway.APIStageAssociation{{RestAPIID: "api1", Stage: "dev"}},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newAPIGWHandler()
			plan, err := h.Backend.CreateUsagePlan(apigateway.CreateUsagePlanInput{
				Name:      "plan",
				APIStages: []apigateway.APIStageAssociation{{RestAPIID: "api1", Stage: "dev"}},
			})
			require.NoError(t, err)

			body := fmt.Sprintf(`{"patchOperations":[%s]}`, tt.op)
			rec := restRequest(t, h, http.MethodPatch, fmt.Sprintf("/usageplans/%s", plan.ID), body)
			require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

			got, err := h.Backend.GetUsagePlan(plan.ID)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.want, got.APIStages)
		})
	}
}

// Test_ApplyStructuredPatch_GatewayResponseMerge proves UpdateGatewayResponse
// merges a per-key PATCH ("/responseParameters/{key}") with the response's
// existing StatusCode/ResponseTemplates/other ResponseParameters entries,
// instead of wholesale-replacing the resource (which previously reused
// PutGatewayResponse's full-replace semantics and silently reverted
// StatusCode to its default and wiped ResponseTemplates on every PATCH).
func Test_ApplyStructuredPatch_GatewayResponseMerge(t *testing.T) {
	t.Parallel()

	h := newAPIGWHandler()
	api, err := h.Backend.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "gwresp-test-api"})
	require.NoError(t, err)

	putBody := `{
		"statusCode":"404",
		"responseParameters":{"gatewayresponse.header.A":"'a-value'"},
		"responseTemplates":{"application/json":"{\"message\":$context.error.messageString}"}
	}`
	rec := restRequest(t, h, http.MethodPut,
		fmt.Sprintf("/restapis/%s/gatewayresponses/ACCESS_DENIED", api.ID), putBody)
	require.Equal(t, http.StatusCreated, rec.Code, "put body: %s", rec.Body.String())

	patchBody := `[{"op":"add","path":"/responseParameters/gatewayresponse.header.B","value":"'b-value'"}]`
	rec = restRequest(t, h, http.MethodPatch,
		fmt.Sprintf("/restapis/%s/gatewayresponses/ACCESS_DENIED", api.ID), patchBody)
	require.Equal(t, http.StatusOK, rec.Code, "patch body: %s", rec.Body.String())

	gr, err := h.Backend.GetGatewayResponse(api.ID, "ACCESS_DENIED")
	require.NoError(t, err)
	assert.Equal(t, "404", gr.StatusCode, "unrelated statusCode must survive the patch")
	assert.Equal(t,
		map[string]string{"application/json": "{\"message\":$context.error.messageString}"},
		gr.ResponseTemplates,
		"unrelated responseTemplates must survive the patch",
	)
	assert.Equal(t, map[string]string{
		"gatewayresponse.header.A": "'a-value'",
		"gatewayresponse.header.B": "'b-value'",
	}, gr.ResponseParameters, "the pre-existing parameter must survive alongside the newly patched one")
}

// Test_ApplyStructuredPatch_TopLevelReplaceStillWorks is a regression check:
// the common case (a single plain top-level field replace) must keep working
// exactly as before, in both wire forms (bare array and the
// {"patchOperations":[...]} wrapper with sibling fields).
func Test_ApplyStructuredPatch_TopLevelReplaceStillWorks(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
	}{
		{name: "bare patch array", body: `[{"op":"replace","path":"/description","value":"updated"}]`},
		{
			name: "patchOperations wrapper",
			body: `{"patchOperations":[{"op":"replace","path":"/description","value":"updated"}]}`,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newAPIGWHandler()
			api, err := h.Backend.CreateRestAPI(apigateway.CreateRestAPIInput{
				Name:        "toplevel-test-api",
				Description: "original",
			})
			require.NoError(t, err)

			rec := restRequest(t, h, http.MethodPatch, fmt.Sprintf("/restapis/%s", api.ID), tt.body)
			require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

			got, err := h.Backend.GetRestAPI(api.ID)
			require.NoError(t, err)
			assert.Equal(t, "updated", got.Description)
			assert.Equal(t, "toplevel-test-api", got.Name, "unrelated field untouched")
		})
	}
}

// Test_ApplyStructuredPatch_RestAPIDescriptionRemove proves that PATCH
// "remove" on the top-level scalar "/description" actually clears the field,
// which AWS documents as supported (patch-operations.html: UpdateRestApi
// "/description" row lists op:remove). Before UpdateRestAPIInput.Description
// became a *string, "remove" was indistinguishable from "not provided in
// this PATCH at all" and silently no-op'd.
func Test_ApplyStructuredPatch_RestAPIDescriptionRemove(t *testing.T) {
	t.Parallel()

	h := newAPIGWHandler()
	api, err := h.Backend.CreateRestAPI(apigateway.CreateRestAPIInput{
		Name:        "desc-remove-api",
		Description: "original description",
	})
	require.NoError(t, err)

	rec := restRequest(t, h, http.MethodPatch, fmt.Sprintf("/restapis/%s", api.ID),
		`[{"op":"remove","path":"/description"}]`)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	got, err := h.Backend.GetRestAPI(api.ID)
	require.NoError(t, err)
	assert.Empty(t, got.Description, "explicit remove must clear the field")
	assert.Equal(t, "desc-remove-api", got.Name, "unrelated field untouched")
}

// Test_ApplyStructuredPatch_RestAPIEndpointFields exercises the two RestApi
// fields absent from gopherstack's RestAPI struct until this sweep:
// "/disableExecuteApiEndpoint" (a bool needing wire-string coercion, like
// tracingEnabled) and "/endpointAccessMode" (a plain string enum). Both are
// documented as replace-only PATCH paths (patch-operations.html).
func Test_ApplyStructuredPatch_RestAPIEndpointFields(t *testing.T) {
	t.Parallel()

	h := newAPIGWHandler()
	api, err := h.Backend.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "endpoint-fields-api"})
	require.NoError(t, err)
	require.False(t, api.DisableExecuteAPIEndpoint, "precondition: defaults to false")
	require.Equal(t, "AVAILABLE", api.APIStatus, "gopherstack creates RestApis synchronously")

	rec := restRequest(t, h, http.MethodPatch, fmt.Sprintf("/restapis/%s", api.ID),
		`[{"op":"replace","path":"/disableExecuteApiEndpoint","value":"true"},`+
			`{"op":"replace","path":"/endpointAccessMode","value":"STRICT"}]`)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	got, err := h.Backend.GetRestAPI(api.ID)
	require.NoError(t, err)
	assert.True(t, got.DisableExecuteAPIEndpoint, "string wire value must coerce to bool")
	assert.Equal(t, "STRICT", got.EndpointAccessMode)
}

// Test_ApplyStructuredPatch_AuthorizerIdentitySourceRemove is the second
// concrete instance (alongside RestApi's "/description") of a top-level
// scalar PATCH "remove" that AWS documents as supported
// (patch-operations.html: UpdateAuthorizer "/identitySource" row).
func Test_ApplyStructuredPatch_AuthorizerIdentitySourceRemove(t *testing.T) {
	t.Parallel()

	h := newAPIGWHandler()
	api, err := h.Backend.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "authz-remove-api"})
	require.NoError(t, err)

	auth, err := h.Backend.CreateAuthorizer(api.ID, apigateway.CreateAuthorizerInput{
		Name:           "my-auth",
		Type:           "TOKEN",
		AuthorizerURI:  "arn:aws:apigateway:us-east-1:lambda:path/functions/f/invocations",
		IdentitySource: "method.request.header.Authorization",
	})
	require.NoError(t, err)
	require.NotEmpty(t, auth.IdentitySource, "precondition")

	rec := restRequest(t, h, http.MethodPatch, fmt.Sprintf("/restapis/%s/authorizers/%s", api.ID, auth.ID),
		`[{"op":"remove","path":"/identitySource"}]`)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	got, err := h.Backend.GetAuthorizer(api.ID, auth.ID)
	require.NoError(t, err)
	assert.Empty(t, got.IdentitySource, "explicit remove must clear the field")
	assert.Equal(t, "my-auth", got.Name, "unrelated field untouched")
}

// Test_ApplyStructuredPatch_StageDocumentationVersion exercises the
// top-level "/documentationVersion" field (types.Stage.DocumentationVersion
// in the SDK), absent from gopherstack's Stage struct until this sweep.
func Test_ApplyStructuredPatch_StageDocumentationVersion(t *testing.T) {
	t.Parallel()

	h := newAPIGWHandler()
	apiID, stageName := setupStage(t, h, nil, nil)

	rec := restRequest(t, h, http.MethodPatch, fmt.Sprintf("/restapis/%s/stages/%s", apiID, stageName),
		`[{"op":"replace","path":"/documentationVersion","value":"1.0.0"}]`)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	stage, err := h.Backend.GetStage(apiID, stageName)
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", stage.DocumentationVersion)
}

// Test_ApplyStructuredPatch_StageCanaryStageVariableOverrides exercises
// "/canarySettings/stageVariableOverrides", AWS's one documented
// canarySettings path with no per-key wildcard row (unlike "/variables",
// which supports "/variables/{name}") -- so, per the AWS "UpdateStage"
// patch-operations reference, its Value is a JSON string whose CONTENTS are
// themselves a JSON-encoded {name: value} object, replacing the whole map at
// once rather than merging one key.
func Test_ApplyStructuredPatch_StageCanaryStageVariableOverrides(t *testing.T) {
	t.Parallel()

	h := newAPIGWHandler()
	apiID, stageName := setupStage(t, h, nil, &apigateway.CanarySettings{
		DeploymentID:   "canary-depl",
		PercentTraffic: 10,
	})

	// PatchOperation.Value is always a JSON string on the wire; here that
	// string's own contents are a JSON object, so the value is
	// double-encoded: `"{\"apiKey\":\"canary-value\"}"`.
	body := `[{"op":"replace","path":"/canarySettings/stageVariableOverrides",` +
		`"value":"{\"apiKey\":\"canary-value\",\"other\":\"x\"}"}]`
	rec := restRequest(t, h, http.MethodPatch, fmt.Sprintf("/restapis/%s/stages/%s", apiID, stageName), body)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	stage, err := h.Backend.GetStage(apiID, stageName)
	require.NoError(t, err)
	require.NotNil(t, stage.CanarySettings)
	wantOverrides := map[string]string{"apiKey": "canary-value", "other": "x"}
	assert.Equal(t, wantOverrides, stage.CanarySettings.StageVariableOverrides)
	assert.Equal(t, "canary-depl", stage.CanarySettings.DeploymentID, "sibling canary field untouched")
	assert.InDelta(t, 10.0, stage.CanarySettings.PercentTraffic, 0.001, "sibling canary field untouched")
}

// Test_ApplyStructuredPatch_StageMethodSettingsCachingFields exercises the
// two MethodSetting fields entirely absent from gopherstack's struct until
// this sweep: "caching/dataEncrypted" (bool) and
// "caching/unauthorizedCacheControlHeaderStrategy" (string enum), both
// addressed the same per-route way as every other method-settings property.
func Test_ApplyStructuredPatch_StageMethodSettingsCachingFields(t *testing.T) {
	t.Parallel()

	h := newAPIGWHandler()
	apiID, stageName := setupStage(t, h, nil, nil)

	body := `[
		{"op":"replace","path":"/~1pets/GET/caching/dataEncrypted","value":"true"},
		{"op":"replace","path":"/~1pets/GET/caching/unauthorizedCacheControlHeaderStrategy","value":"FAIL_WITH_403"}
	]`
	rec := restRequest(t, h, http.MethodPatch, fmt.Sprintf("/restapis/%s/stages/%s", apiID, stageName), body)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	stage, err := h.Backend.GetStage(apiID, stageName)
	require.NoError(t, err)
	require.Contains(t, stage.MethodSettings, "/pets/GET")
	ms := stage.MethodSettings["/pets/GET"]
	assert.True(t, ms.CacheDataEncrypted)
	assert.Equal(t, "FAIL_WITH_403", ms.UnauthorizedCacheControlHeaderStrategy)
}

// Test_ApplyStructuredPatch_UsagePlanPerRouteThrottle exercises
// "/apiStages/{restApiId}:{stage}/throttle/{resourcePath}/{httpMethod}[/rateLimit|burstLimit]",
// the per-route throttle override path within one usage-plan API stage
// (distinct from the whole-apiStage "/apiStages" membership path already
// covered by Test_ApplyStructuredPatch_UsagePlanAPIStages).
func Test_ApplyStructuredPatch_UsagePlanPerRouteThrottle(t *testing.T) {
	t.Parallel()

	h := newAPIGWHandler()
	plan, err := h.Backend.CreateUsagePlan(apigateway.CreateUsagePlanInput{
		Name:      "route-throttle-plan",
		APIStages: []apigateway.APIStageAssociation{{RestAPIID: "api1", Stage: "prod"}},
	})
	require.NoError(t, err)

	setBody := `[
		{"op":"replace","path":"/apiStages/api1:prod/throttle/~1items/GET/rateLimit","value":"10.5"},
		{"op":"replace","path":"/apiStages/api1:prod/throttle/~1items/GET/burstLimit","value":"20"}
	]`
	rec := restRequest(t, h, http.MethodPatch, fmt.Sprintf("/usageplans/%s", plan.ID), setBody)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	got, err := h.Backend.GetUsagePlan(plan.ID)
	require.NoError(t, err)
	require.Len(t, got.APIStages, 1)
	require.NotNil(t, got.APIStages[0].Throttle)
	require.Contains(t, got.APIStages[0].Throttle, "GET /items")
	assert.InDelta(t, 10.5, got.APIStages[0].Throttle["GET /items"].RateLimit, 0.001)
	assert.Equal(t, 20, got.APIStages[0].Throttle["GET /items"].BurstLimit)

	removeBody := `[{"op":"remove","path":"/apiStages/api1:prod/throttle/~1items/GET"}]`
	rec = restRequest(t, h, http.MethodPatch, fmt.Sprintf("/usageplans/%s", plan.ID), removeBody)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	got, err = h.Backend.GetUsagePlan(plan.ID)
	require.NoError(t, err)
	require.Len(t, got.APIStages, 1)
	assert.NotContains(t, got.APIStages[0].Throttle, "GET /items", "remove drops the whole per-route entry")
}

// Test_ApplyStructuredPatch_APIKeyStages exercises UpdateApiKey's "/stages"
// add/remove (AWS "DEPRECATED FOR USAGE PLANS" but still real and
// wire-modeled), where Value is "{restApiId}/{stageName}" -- distinct from
// UsagePlan's "{restApiId}:{stage}" apiStages membership value format (":"
// not "/").
func Test_ApplyStructuredPatch_APIKeyStages(t *testing.T) {
	t.Parallel()

	h := newAPIGWHandler()
	apiID, stageName := setupStage(t, h, nil, nil)
	key, err := h.Backend.CreateAPIKey(apigateway.CreateAPIKeyInput{Name: "stage-patch-key", Enabled: true})
	require.NoError(t, err)

	stageValue := apiID + "/" + stageName
	addBody := fmt.Sprintf(`[{"op":"add","path":"/stages","value":%q}]`, stageValue)
	rec := restRequest(t, h, http.MethodPatch, "/apikeys/"+key.ID, addBody)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	got, err := h.Backend.GetAPIKey(key.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{stageValue}, got.StageKeys)

	removeBody := fmt.Sprintf(`[{"op":"remove","path":"/stages","value":%q}]`, stageValue)
	rec = restRequest(t, h, http.MethodPatch, "/apikeys/"+key.ID, removeBody)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	got, err = h.Backend.GetAPIKey(key.ID)
	require.NoError(t, err)
	assert.Empty(t, got.StageKeys)
}

// Test_ApplyStructuredPatch_MultiOpSameRequest exercises the six resolvers
// that, before this fix, each independently re-derived their starting
// map/struct from CURRENT BACKEND STATE instead of checking whether an
// earlier op in the SAME PATCH request's patchOperations array already
// staged that field — so the last op's write silently discarded every
// earlier op touching the same merged field within one request (see
// stagedValue's doc comment). Each case sends two ops in a single request
// and asserts both landed.
func Test_ApplyStructuredPatch_MultiOpSameRequest(t *testing.T) {
	t.Parallel()

	cases := []struct {
		verify func(t *testing.T, h *apigateway.Handler, apiID, stageName string)
		name   string
		path   string
		body   string
	}{
		{
			name: "stage variables two adds",
			body: `[{"op":"add","path":"/variables/b","value":"2"},` +
				`{"op":"add","path":"/variables/c","value":"3"}]`,
			verify: func(t *testing.T, h *apigateway.Handler, apiID, stageName string) {
				t.Helper()
				stage, err := h.Backend.GetStage(apiID, stageName)
				require.NoError(t, err)
				assert.Equal(t, map[string]string{"a": "1", "b": "2", "c": "3"}, stage.Variables)
			},
		},
		{
			name: "stage canary two sub-fields",
			body: `[{"op":"replace","path":"/canarySettings/deploymentId","value":"canary-2"},` +
				`{"op":"replace","path":"/canarySettings/percentTraffic","value":"25"}]`,
			verify: func(t *testing.T, h *apigateway.Handler, apiID, stageName string) {
				t.Helper()
				stage, err := h.Backend.GetStage(apiID, stageName)
				require.NoError(t, err)
				require.NotNil(t, stage.CanarySettings)
				assert.Equal(t, "canary-2", stage.CanarySettings.DeploymentID)
				assert.InDelta(t, 25.0, stage.CanarySettings.PercentTraffic, 0.001)
			},
		},
		{
			name: "stage access log two sub-fields",
			body: `[{"op":"replace","path":"/accessLogSettings/destinationArn","value":"arn:aws:logs:x"},` +
				`{"op":"replace","path":"/accessLogSettings/format","value":"$context.requestId"}]`,
			verify: func(t *testing.T, h *apigateway.Handler, apiID, stageName string) {
				t.Helper()
				stage, err := h.Backend.GetStage(apiID, stageName)
				require.NoError(t, err)
				require.NotNil(t, stage.AccessLogSettings)
				assert.Equal(t, "arn:aws:logs:x", stage.AccessLogSettings.DestinationARN)
				assert.Equal(t, "$context.requestId", stage.AccessLogSettings.Format)
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newAPIGWHandler()
			apiID, stageName := setupStage(t, h, map[string]string{"a": "1"}, &apigateway.CanarySettings{
				DeploymentID:   "orig",
				PercentTraffic: 5,
			})

			rec := restRequest(t, h, http.MethodPatch,
				fmt.Sprintf("/restapis/%s/stages/%s", apiID, stageName), tt.body)
			require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

			tt.verify(t, h, apiID, stageName)
		})
	}
}

// Test_ApplyStructuredPatch_RestAPIBinaryMediaTypesMultiOp exercises two
// binaryMediaTypes adds in a single request (see
// Test_ApplyStructuredPatch_MultiOpSameRequest's doc comment for the bug
// class this guards against).
func Test_ApplyStructuredPatch_RestAPIBinaryMediaTypesMultiOp(t *testing.T) {
	t.Parallel()

	h := newAPIGWHandler()
	api, err := h.Backend.CreateRestAPI(apigateway.CreateRestAPIInput{
		Name:             "media-multiop-api",
		BinaryMediaTypes: []string{"image/png"},
	})
	require.NoError(t, err)

	body := `[{"op":"add","path":"/binaryMediaTypes/application~1octet-stream"},` +
		`{"op":"add","path":"/binaryMediaTypes/application~1pdf"}]`
	rec := restRequest(t, h, http.MethodPatch, fmt.Sprintf("/restapis/%s", api.ID), body)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	got, err := h.Backend.GetRestAPI(api.ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"image/png", "application/octet-stream", "application/pdf"}, got.BinaryMediaTypes)
}

// Test_ApplyStructuredPatch_AccountThrottleMultiOp exercises rateLimit and
// burstLimit set in a SINGLE request (Test_ApplyStructuredPatch_Account only
// ever sends one sub-field per request, so it never exercised this bug).
func Test_ApplyStructuredPatch_AccountThrottleMultiOp(t *testing.T) {
	t.Parallel()

	h := newAPIGWHandler()

	rec := restRequest(t, h, http.MethodPatch, "/account",
		`[{"op":"replace","path":"/throttle/rateLimit","value":"200.5"},`+
			`{"op":"replace","path":"/throttle/burstLimit","value":"75"}]`)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	acct, err := h.Backend.GetAccount()
	require.NoError(t, err)
	require.NotNil(t, acct.ThrottleSettings)
	assert.InDelta(t, 200.5, acct.ThrottleSettings.RateLimit, 0.001)
	assert.Equal(t, 75, acct.ThrottleSettings.BurstLimit)
}

// Test_ApplyStructuredPatch_GatewayResponseMultiOp exercises two
// responseParameters keys added in a single request (the existing
// GatewayResponseMerge test only ever sends one key per request).
func Test_ApplyStructuredPatch_GatewayResponseMultiOp(t *testing.T) {
	t.Parallel()

	h := newAPIGWHandler()
	api, err := h.Backend.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "gwresp-multiop-api"})
	require.NoError(t, err)

	putBody := `{"statusCode":"404","responseParameters":{"gatewayresponse.header.A":"'a-value'"}}`
	rec := restRequest(t, h, http.MethodPut,
		fmt.Sprintf("/restapis/%s/gatewayresponses/ACCESS_DENIED", api.ID), putBody)
	require.Equal(t, http.StatusCreated, rec.Code, "put body: %s", rec.Body.String())

	patchBody := `[{"op":"add","path":"/responseParameters/gatewayresponse.header.B","value":"'b-value'"},` +
		`{"op":"add","path":"/responseParameters/gatewayresponse.header.C","value":"'c-value'"}]`
	rec = restRequest(t, h, http.MethodPatch,
		fmt.Sprintf("/restapis/%s/gatewayresponses/ACCESS_DENIED", api.ID), patchBody)
	require.Equal(t, http.StatusOK, rec.Code, "patch body: %s", rec.Body.String())

	gr, err := h.Backend.GetGatewayResponse(api.ID, "ACCESS_DENIED")
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"gatewayresponse.header.A": "'a-value'",
		"gatewayresponse.header.B": "'b-value'",
		"gatewayresponse.header.C": "'c-value'",
	}, gr.ResponseParameters, "both ops in the same request must land, not just the last")
}

// Test_ApplyStructuredPatch_DomainNameNestedPaths exercises UpdateDomainName
// PATCH paths under "/endpointConfiguration/*" and "/mutualTlsAuthentication/*",
// which applyResourcePatchOp previously had no dispatch case for at all
// (opUpdateDomainName was absent from its switch), so every nested/multi-segment
// DomainName PATCH silently no-opped via applyTopLevelPatchOp's
// path-contains-"/" guard. Four ops across two different nested fields land
// in a single request, also exercising the staged-merge pattern for brand new
// resolver code.
func Test_ApplyStructuredPatch_DomainNameNestedPaths(t *testing.T) {
	t.Parallel()

	h := newAPIGWHandler()
	_, err := h.Backend.CreateDomainName(apigateway.CreateDomainNameInput{
		DomainName:            "nested.example.com",
		EndpointConfiguration: &apigateway.EndpointConfiguration{Types: []string{"REGIONAL"}},
	})
	require.NoError(t, err)

	body := `[
		{"op":"add","path":"/endpointConfiguration/types","value":"EDGE"},
		{"op":"replace","path":"/endpointConfiguration/ipAddressType","value":"dualstack"},
		{"op":"add","path":"/mutualTlsAuthentication/truststoreUri","value":"s3://bucket/truststore.pem"},
		{"op":"replace","path":"/mutualTlsAuthentication/truststoreVersion","value":"v2"}
	]`
	rec := restRequest(t, h, http.MethodPatch, "/domainnames/nested.example.com", body)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	got, err := h.Backend.GetDomainName("nested.example.com")
	require.NoError(t, err)
	require.NotNil(t, got.EndpointConfiguration)
	assert.ElementsMatch(t, []string{"REGIONAL", "EDGE"}, got.EndpointConfiguration.Types,
		"add must merge with the existing type, not replace it")
	assert.Equal(t, "dualstack", got.EndpointConfiguration.IPAddressType)
	require.NotNil(t, got.MutualTLSAuthentication)
	assert.Equal(t, "s3://bucket/truststore.pem", got.MutualTLSAuthentication.TruststoreURI)
	assert.Equal(t, "v2", got.MutualTLSAuthentication.TruststoreVersion)

	removeBody := `[{"op":"remove","path":"/endpointConfiguration/types","value":"REGIONAL"},` +
		`{"op":"remove","path":"/mutualTlsAuthentication/truststoreUri"}]`
	rec = restRequest(t, h, http.MethodPatch, "/domainnames/nested.example.com", removeBody)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	got, err = h.Backend.GetDomainName("nested.example.com")
	require.NoError(t, err)
	assert.Equal(t, []string{"EDGE"}, got.EndpointConfiguration.Types)
	assert.Empty(t, got.MutualTLSAuthentication.TruststoreURI)
	assert.Equal(t, "v2", got.MutualTLSAuthentication.TruststoreVersion, "unrelated sub-field survives the remove")
}

// Test_ApplyStructuredPatch_DomainNameCertificateArnRemove is the third
// concrete instance (alongside RestApi's "/description" and Authorizer's
// "/identitySource") of a top-level scalar PATCH "remove" that AWS documents
// as supported (patch-operations.html: UpdateDomainName "/certificateArn"
// row). It contrasts an explicit remove against a same-shaped PATCH that
// doesn't mention the field at all, proving the two are distinguishable.
func Test_ApplyStructuredPatch_DomainNameCertificateArnRemove(t *testing.T) {
	t.Parallel()

	const origCertARN = "arn:aws:acm:us-east-1:123456789012:certificate/edge"

	cases := []struct {
		name        string
		ops         string
		wantCertARN string
	}{
		{
			name:        "explicit remove clears certificateArn",
			ops:         `[{"op":"remove","path":"/certificateArn"}]`,
			wantCertARN: "",
		},
		{
			name:        "field absent from the patch leaves certificateArn untouched",
			ops:         `[{"op":"replace","path":"/securityPolicy","value":"TLS_1_2"}]`,
			wantCertARN: origCertARN,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newAPIGWHandler()
			_, err := h.Backend.CreateDomainName(apigateway.CreateDomainNameInput{
				DomainName:     "cert-remove.example.com",
				CertificateARN: origCertARN,
			})
			require.NoError(t, err)

			rec := restRequest(t, h, http.MethodPatch, "/domainnames/cert-remove.example.com", tt.ops)
			require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

			got, err := h.Backend.GetDomainName("cert-remove.example.com")
			require.NoError(t, err)
			assert.Equal(t, tt.wantCertARN, got.CertificateARN)
		})
	}
}

// Test_ApplyStructuredPatch_DomainNameReplaceOnlyFields exercises the four
// DomainName scalar fields patch-operations.html documents as replace-only
// (gopherstack-npq5): "/managementPolicy", "/policy", "/routingMode",
// "/endpointAccessMode". Before this fix none of these had a matching field
// on UpdateDomainNameInput, so encoding/json silently dropped the value and
// the PATCH was accepted and did nothing.
func Test_ApplyStructuredPatch_DomainNameReplaceOnlyFields(t *testing.T) {
	t.Parallel()

	cases := []struct {
		want func(*apigateway.DomainName) string
		name string
		path string
	}{
		{
			name: "managementPolicy",
			path: "/managementPolicy",
			want: func(d *apigateway.DomainName) string { return d.ManagementPolicy },
		},
		{
			name: "policy",
			path: "/policy",
			want: func(d *apigateway.DomainName) string { return d.Policy },
		},
		{
			name: "routingMode",
			path: "/routingMode",
			want: func(d *apigateway.DomainName) string { return d.RoutingMode },
		},
		{
			name: "endpointAccessMode",
			path: "/endpointAccessMode",
			want: func(d *apigateway.DomainName) string { return d.EndpointAccessMode },
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newAPIGWHandler()
			domain := "replace-only-" + tt.name + ".example.com"
			_, err := h.Backend.CreateDomainName(apigateway.CreateDomainNameInput{DomainName: domain})
			require.NoError(t, err)

			body := fmt.Sprintf(`[{"op":"replace","path":%q,"value":"new-value"}]`, tt.path)
			rec := restRequest(t, h, http.MethodPatch, "/domainnames/"+domain, body)
			require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

			got, err := h.Backend.GetDomainName(domain)
			require.NoError(t, err)
			assert.Equal(t, "new-value", tt.want(got))
		})
	}
}

// Test_ApplyStructuredPatch_DomainNameRemovableCertificateFields exercises
// the three DomainName certificate-identity fields patch-operations.html
// documents as supporting add/replace/remove (gopherstack-npq5):
// "/certificateName", "/regionalCertificateName",
// "/ownershipVerificationCertificateArn". Mirrors
// Test_ApplyStructuredPatch_DomainNameCertificateArnRemove's shape for the
// three fields that same commit's fix didn't cover.
func Test_ApplyStructuredPatch_DomainNameRemovableCertificateFields(t *testing.T) {
	t.Parallel()

	cases := []struct {
		want func(*apigateway.DomainName) string
		name string
		path string
	}{
		{
			name: "certificateName",
			path: "/certificateName",
			want: func(d *apigateway.DomainName) string { return d.CertificateName },
		},
		{
			name: "regionalCertificateName",
			path: "/regionalCertificateName",
			want: func(d *apigateway.DomainName) string { return d.RegionalCertificateName },
		},
		{
			name: "ownershipVerificationCertificateArn",
			path: "/ownershipVerificationCertificateArn",
			want: func(d *apigateway.DomainName) string { return d.OwnershipVerificationCertificateARN },
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newAPIGWHandler()
			domain := "removable-" + tt.name + ".example.com"
			_, err := h.Backend.CreateDomainName(apigateway.CreateDomainNameInput{DomainName: domain})
			require.NoError(t, err)

			addBody := fmt.Sprintf(`[{"op":"add","path":%q,"value":"cert-value"}]`, tt.path)
			rec := restRequest(t, h, http.MethodPatch, "/domainnames/"+domain, addBody)
			require.Equal(t, http.StatusOK, rec.Code, "add body: %s", rec.Body.String())

			got, err := h.Backend.GetDomainName(domain)
			require.NoError(t, err)
			assert.Equal(t, "cert-value", tt.want(got), "add must set the field")

			removeBody := fmt.Sprintf(`[{"op":"remove","path":%q}]`, tt.path)
			rec = restRequest(t, h, http.MethodPatch, "/domainnames/"+domain, removeBody)
			require.Equal(t, http.StatusOK, rec.Code, "remove body: %s", rec.Body.String())

			got, err = h.Backend.GetDomainName(domain)
			require.NoError(t, err)
			assert.Empty(t, tt.want(got), "explicit remove must clear the field")
		})
	}
}

// Test_ApplyStructuredPatch_UsagePlanProductCode exercises UpdateUsagePlan's
// "/productCode" field, which patch-operations.html documents as supporting
// add/replace/remove (gopherstack-npq5). UsagePlan had no ProductCode field
// at all before this fix, so a PATCH naming it was accepted and did nothing.
func Test_ApplyStructuredPatch_UsagePlanProductCode(t *testing.T) {
	t.Parallel()

	h := newAPIGWHandler()
	plan, err := h.Backend.CreateUsagePlan(apigateway.CreateUsagePlanInput{Name: "product-code-plan"})
	require.NoError(t, err)

	addBody := `[{"op":"add","path":"/productCode","value":"prod-abc123"}]`
	rec := restRequest(t, h, http.MethodPatch, "/usageplans/"+plan.ID, addBody)
	require.Equal(t, http.StatusOK, rec.Code, "add body: %s", rec.Body.String())

	got, err := h.Backend.GetUsagePlan(plan.ID)
	require.NoError(t, err)
	assert.Equal(t, "prod-abc123", got.ProductCode, "add must set productCode")

	replaceBody := `[{"op":"replace","path":"/productCode","value":"prod-xyz789"}]`
	rec = restRequest(t, h, http.MethodPatch, "/usageplans/"+plan.ID, replaceBody)
	require.Equal(t, http.StatusOK, rec.Code, "replace body: %s", rec.Body.String())

	got, err = h.Backend.GetUsagePlan(plan.ID)
	require.NoError(t, err)
	assert.Equal(t, "prod-xyz789", got.ProductCode, "replace must update productCode")

	removeBody := `[{"op":"remove","path":"/productCode"}]`
	rec = restRequest(t, h, http.MethodPatch, "/usageplans/"+plan.ID, removeBody)
	require.Equal(t, http.StatusOK, rec.Code, "remove body: %s", rec.Body.String())

	got, err = h.Backend.GetUsagePlan(plan.ID)
	require.NoError(t, err)
	assert.Empty(t, got.ProductCode, "explicit remove must clear productCode")
}

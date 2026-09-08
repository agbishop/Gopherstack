package iot_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	iotsdk "github.com/aws/aws-sdk-go-v2/service/iot"
	iottypes "github.com/aws/aws-sdk-go-v2/service/iot/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/iot"
)

// TestBatch2_PolicyPrincipalOps tests policy/principal listing.
func TestPolicyPrincipalOps(t *testing.T) {
	t.Parallel()
	b := iot.NewInMemoryBackend()
	h := iot.NewHandler(b, nil)

	// Seed: policy + attachment
	b.AddPolicyInternal(iot.Policy{
		PolicyName:     "my-policy",
		PolicyDocument: `{"Version":"2012-10-17"}`,
		ARN:            "arn:aws:iot:us-east-1:000000000000:policy/my-policy",
	})
	_ = b.AttachPolicy(&iot.AttachPolicyInput{
		PolicyName: "my-policy",
		Target:     "arn:aws:iot:us-east-1:000000000000:cert/abc123",
	})

	// ListPolicyPrincipals (via header)
	out := iotOK(t, h, http.MethodGet, "/policy-principals", nil)
	// No header set so principals is empty — just verify response structure
	if out["principals"] == nil {
		t.Error("expected principals key in response")
	}

	// ListTargetsForPolicy
	out2 := iotOK(t, h, http.MethodGet, "/policy-targets/my-policy", nil)
	targets, _ := out2["targets"].([]any)
	if len(targets) != 1 {
		t.Errorf("expected 1 target, got %d", len(targets))
	}
}

// TestGetEffectivePolicies_ThingNameIsQueryParam guards
// GetEffectivePoliciesInput's real thingName member: a query parameter, not
// a JSON body field (iot@v1.77.4 serializers.go
// awsRestjson1_serializeOpHttpBindingsGetEffectivePoliciesInput) --
// previously read from the body, where a real client never puts it, so
// thing-scoped effective-policy resolution always saw an empty thingName.
func TestGetEffectivePolicies_ThingNameIsQueryParam(t *testing.T) {
	t.Parallel()
	b := iot.NewInMemoryBackend()
	h := iot.NewHandler(b, nil)

	b.AddThingInternal(iot.Thing{ThingName: "effective-policy-thing"})
	b.AddPolicyInternal(iot.Policy{
		PolicyName:     "thing-scoped-policy",
		PolicyDocument: `{"Version":"2012-10-17"}`,
		ARN:            "arn:aws:iot:us-east-1:000000000000:policy/thing-scoped-policy",
	})
	require.NoError(t, b.AttachThingPrincipal(&iot.AttachThingPrincipalInput{
		ThingName: "effective-policy-thing",
		Principal: "arn:aws:iot:us-east-1:000000000000:cert/eff1",
	}))
	require.NoError(t, b.AttachPrincipalPolicy(&iot.AttachPrincipalPolicyInput{
		PolicyName: "thing-scoped-policy",
		Principal:  "arn:aws:iot:us-east-1:000000000000:cert/eff1",
	}))

	out := iotOK(t, h, http.MethodPost, "/effective-policies?thingName=effective-policy-thing", map[string]any{})
	policies, _ := out["effectivePolicies"].([]any)
	require.Len(t, policies, 1)
	entry, _ := policies[0].(map[string]any)
	assert.Equal(t, "thing-scoped-policy", entry["policyName"])
}

// TestListPrincipalThingsV2_WireShapeAndFilter guards
// ListPrincipalThingsV2Output's real principalThingObjects member -- each
// entry is {thingName, thingPrincipalType} (types.PrincipalThingObject,
// iot@v1.77.4), not a bare thingName -- backed by ListPrincipalThingsV2's
// own backend method (previously delegated to V1 ListPrincipalThings and
// hardcoded every entry's type to the default). Attaches with a non-default
// EXCLUSIVE_THING type -- provable only now that AttachThingPrincipal
// persists the type it's given, rather than always defaulting.
func TestListPrincipalThingsV2_WireShapeAndFilter(t *testing.T) {
	t.Parallel()
	b := iot.NewInMemoryBackend()
	h := iot.NewHandler(b, nil)

	b.AddThingInternal(iot.Thing{ThingName: "v2-principal-thing"})
	require.NoError(t, b.AttachThingPrincipal(&iot.AttachThingPrincipalInput{
		ThingName:          "v2-principal-thing",
		Principal:          "arn:aws:iot:us-east-1:000000000000:cert/pt1",
		ThingPrincipalType: "EXCLUSIVE_THING",
	}))

	rec := doRefRequest(t, h, http.MethodGet, "/principal-things-v2", nil,
		map[string]string{"X-Amzn-Principal": "arn:aws:iot:us-east-1:000000000000:cert/pt1"})
	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	objs, _ := out["principalThingObjects"].([]any)
	require.Len(t, objs, 1)
	entry, ok := objs[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "v2-principal-thing", entry["thingName"])
	assert.Equal(t, "EXCLUSIVE_THING", entry["thingPrincipalType"])
}

// TestAttachThingPrincipal_ThingPrincipalTypeSurvives_SDKRoundTrip guards
// AttachThingPrincipalInput's real thingPrincipalType query parameter
// (iot@v1.77.4 serializers.go
// awsRestjson1_serializeOpHttpBindingsAttachThingPrincipalInput), previously
// entirely dropped -- every attachment was silently forced to the default
// NON_EXCLUSIVE_THING regardless of what a real client requested. Driven
// through a real generated AWS SDK v2 client and read back via both
// ListThingPrincipalsV2 and ListPrincipalThingsV2.
func TestAttachThingPrincipal_ThingPrincipalTypeSurvives_SDKRoundTrip(t *testing.T) {
	t.Parallel()

	client, b := newIoTSDKClient(t)
	ctx := t.Context()

	b.AddThingInternal(iot.Thing{ThingName: "sdk-attach-thing"})

	_, err := client.AttachThingPrincipal(ctx, &iotsdk.AttachThingPrincipalInput{
		ThingName:          aws.String("sdk-attach-thing"),
		Principal:          aws.String("arn:aws:iot:us-east-1:000000000000:cert/sdk1"),
		ThingPrincipalType: iottypes.ThingPrincipalTypeExclusiveThing,
	})
	require.NoError(t, err)

	tp, err := client.ListThingPrincipalsV2(ctx, &iotsdk.ListThingPrincipalsV2Input{
		ThingName: aws.String("sdk-attach-thing"),
	})
	require.NoError(t, err)
	require.Len(t, tp.ThingPrincipalObjects, 1)
	assert.Equal(t, "arn:aws:iot:us-east-1:000000000000:cert/sdk1", aws.ToString(tp.ThingPrincipalObjects[0].Principal))
	assert.Equal(t, iottypes.ThingPrincipalTypeExclusiveThing, tp.ThingPrincipalObjects[0].ThingPrincipalType)

	pt, err := client.ListPrincipalThingsV2(ctx, &iotsdk.ListPrincipalThingsV2Input{
		Principal: aws.String("arn:aws:iot:us-east-1:000000000000:cert/sdk1"),
	})
	require.NoError(t, err)
	require.Len(t, pt.PrincipalThingObjects, 1)
	assert.Equal(t, "sdk-attach-thing", aws.ToString(pt.PrincipalThingObjects[0].ThingName))
	assert.Equal(t, iottypes.ThingPrincipalTypeExclusiveThing, pt.PrincipalThingObjects[0].ThingPrincipalType)
}

// TestPolicyPrincipalListing_Pagination guards the real marker/pageSize
// (ListPrincipalPolicies/ListPolicyPrincipals/ListTargetsForPolicy,
// iot@v1.77.4) and maxResults/nextToken (ListPrincipalThings) pagination
// params, previously entirely ignored across this whole family -- every
// list op always returned everything in one page.
func TestPolicyPrincipalListing_Pagination(t *testing.T) {
	t.Parallel()
	b := iot.NewInMemoryBackend()
	h := iot.NewHandler(b, nil)

	const principal = "arn:aws:iot:us-east-1:000000000000:cert/pag1"

	for i := range 3 {
		name := "pag-policy-" + string(rune('a'+i))
		b.AddPolicyInternal(iot.Policy{
			PolicyName: name,
			ARN:        "arn:aws:iot:us-east-1:000000000000:policy/" + name,
		})
		require.NoError(t, b.AttachPrincipalPolicy(&iot.AttachPrincipalPolicyInput{
			PolicyName: name,
			Principal:  principal,
		}))
	}

	t.Run("list_principal_policies", func(t *testing.T) {
		t.Parallel()

		rec := doRefRequest(t, h, http.MethodGet, "/principal-policies?pageSize=1", nil,
			map[string]string{"X-Amzn-Iot-Principal": principal})
		require.Equal(t, http.StatusOK, rec.Code)

		var out map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
		policies, _ := out["policies"].([]any)
		assert.LessOrEqual(t, len(policies), 1)
		assert.NotEmpty(t, out["nextMarker"])
	})

	t.Run("list_policy_principals", func(t *testing.T) {
		t.Parallel()

		for i := range 3 {
			require.NoError(t, b.AttachPrincipalPolicy(&iot.AttachPrincipalPolicyInput{
				PolicyName: "pag-policy-a",
				Principal:  "arn:aws:iot:us-east-1:000000000000:cert/pag-multi-" + string(rune('a'+i)),
			}))
		}

		rec := doRefRequest(t, h, http.MethodGet, "/policy-principals?pageSize=1", nil,
			map[string]string{"X-Amzn-Policy-Name": "pag-policy-a"})
		require.Equal(t, http.StatusOK, rec.Code)

		var out map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
		princs, _ := out["principals"].([]any)
		assert.LessOrEqual(t, len(princs), 1)
		assert.NotEmpty(t, out["nextMarker"])
	})

	t.Run("list_targets_for_policy", func(t *testing.T) {
		t.Parallel()

		for i := range 3 {
			require.NoError(t, b.AttachPolicy(&iot.AttachPolicyInput{
				PolicyName: "pag-policy-b",
				Target:     "arn:aws:iot:us-east-1:000000000000:cert/pag-target-" + string(rune('a'+i)),
			}))
		}

		rec := doRefRequest(t, h, http.MethodGet, "/policy-targets/pag-policy-b?pageSize=1", nil, nil)
		require.Equal(t, http.StatusOK, rec.Code)

		var out map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
		targets, _ := out["targets"].([]any)
		assert.LessOrEqual(t, len(targets), 1)
		assert.NotEmpty(t, out["nextMarker"])
	})

	t.Run("list_principal_things", func(t *testing.T) {
		t.Parallel()

		for i := range 3 {
			name := "pag-thing-" + string(rune('a'+i))
			b.AddThingInternal(iot.Thing{ThingName: name})
			require.NoError(t, b.AttachThingPrincipal(&iot.AttachThingPrincipalInput{
				ThingName: name,
				Principal: "arn:aws:iot:us-east-1:000000000000:cert/pag-thing-principal",
			}))
		}

		rec := doRefRequest(t, h, http.MethodGet, "/principal-things?maxResults=1", nil,
			map[string]string{"X-Amzn-Principal": "arn:aws:iot:us-east-1:000000000000:cert/pag-thing-principal"})
		require.Equal(t, http.StatusOK, rec.Code)

		var out map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
		things, _ := out["things"].([]any)
		assert.LessOrEqual(t, len(things), 1)
		assert.NotEmpty(t, out["nextToken"])
	})
}

// TestRefinement1_AttachPrincipalPolicy_Handler verifies AttachPrincipalPolicy via HTTP.
func TestAttachPrincipalPolicy_Handler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		wantCode int
	}{
		{name: "attach_policy_to_principal", wantCode: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, _ := newRefHandler()
			rec := doRefRequest(t, h, http.MethodPut, "/principal-policies/my-policy", nil,
				map[string]string{"x-amzn-iot-principal": "arn:aws:iot:us-east-1:123456789012:cert/abc123"})
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

func TestGetPolicy(t *testing.T) {
	t.Parallel()

	backend := iot.NewInMemoryBackend()
	backend.AddPolicyInternal(iot.Policy{PolicyName: "my-policy", PolicyDocument: `{"Version":"2012-10-17"}`})

	out, err := backend.GetPolicy("my-policy")
	require.NoError(t, err)
	assert.Equal(t, "my-policy", out.PolicyName)
	assert.NotEmpty(t, out.PolicyARN)
	assert.JSONEq(t, `{"Version":"2012-10-17"}`, out.PolicyDocument)
}

func TestGetPolicy_NotFound(t *testing.T) {
	t.Parallel()

	backend := iot.NewInMemoryBackend()
	_, err := backend.GetPolicy("nonexistent")
	require.Error(t, err)
	assert.ErrorIs(t, err, iot.ErrPolicyNotFound)
}

func TestDeletePolicy(t *testing.T) {
	t.Parallel()

	backend := iot.NewInMemoryBackend()
	backend.AddPolicyInternal(iot.Policy{PolicyName: "del-policy"})
	require.Equal(t, 1, backend.PolicyCount())

	require.NoError(t, backend.DeletePolicy("del-policy"))
	assert.Equal(t, 0, backend.PolicyCount())
}

func TestDeletePolicy_NotFound(t *testing.T) {
	t.Parallel()

	backend := iot.NewInMemoryBackend()
	err := backend.DeletePolicy("nonexistent")
	require.Error(t, err)
	assert.ErrorIs(t, err, iot.ErrPolicyNotFound)
}

func TestDeletePolicy_ClearsResourceTagsOnRecreate(t *testing.T) {
	t.Parallel()

	backend := iot.NewInMemoryBackend()

	created, err := backend.CreatePolicy(&iot.CreatePolicyInput{
		PolicyName:     "reused-policy",
		PolicyDocument: "{}",
	})
	require.NoError(t, err)
	require.NoError(t, backend.TagResourceGeneric(created.PolicyARN, map[string]string{"env": "prod"}))
	require.Equal(t, map[string]string{"env": "prod"}, backend.ListTagsForResource(created.PolicyARN))

	require.NoError(t, backend.DeletePolicy("reused-policy"))

	recreated, err := backend.CreatePolicy(&iot.CreatePolicyInput{
		PolicyName:     "reused-policy",
		PolicyDocument: "{}",
	})
	require.NoError(t, err)
	require.Equal(t, created.PolicyARN, recreated.PolicyARN)
	assert.Empty(t, backend.ListTagsForResource(recreated.PolicyARN))
}

func TestDeletePolicy_LeavesOtherPolicyTagsIntact(t *testing.T) {
	t.Parallel()

	backend := iot.NewInMemoryBackend()

	gone, err := backend.CreatePolicy(&iot.CreatePolicyInput{PolicyName: "gone-policy-3", PolicyDocument: "{}"})
	require.NoError(t, err)
	kept, err := backend.CreatePolicy(&iot.CreatePolicyInput{PolicyName: "kept-policy-2", PolicyDocument: "{}"})
	require.NoError(t, err)

	require.NoError(t, backend.TagResourceGeneric(gone.PolicyARN, map[string]string{"env": "prod"}))
	require.NoError(t, backend.TagResourceGeneric(kept.PolicyARN, map[string]string{"env": "dev"}))

	require.NoError(t, backend.DeletePolicy("gone-policy-3"))

	assert.Empty(t, backend.ListTagsForResource(gone.PolicyARN))
	assert.Equal(t, map[string]string{"env": "dev"}, backend.ListTagsForResource(kept.PolicyARN))
}

func TestDeletePolicy_ClearsPolicyVersions(t *testing.T) {
	t.Parallel()

	backend := iot.NewInMemoryBackend()

	_, err := backend.CreatePolicy(&iot.CreatePolicyInput{
		PolicyName:     "gone-policy",
		PolicyDocument: "{}",
	})
	require.NoError(t, err)

	require.NoError(t, backend.DeletePolicy("gone-policy"))

	_, err = backend.GetPolicyVersion("gone-policy", "1")
	require.Error(t, err, "GetPolicyVersion must not see a deleted policy's stale version")
	assert.ErrorIs(t, err, iot.ErrPolicyVersionNotFound)
}

func TestDeletePolicy_LeavesOtherPolicyVersionsIntact(t *testing.T) {
	t.Parallel()

	backend := iot.NewInMemoryBackend()

	_, err := backend.CreatePolicy(&iot.CreatePolicyInput{PolicyName: "keep-policy", PolicyDocument: "{}"})
	require.NoError(t, err)
	_, err = backend.CreatePolicy(&iot.CreatePolicyInput{PolicyName: "gone-policy-2", PolicyDocument: "{}"})
	require.NoError(t, err)

	require.NoError(t, backend.DeletePolicy("gone-policy-2"))

	v, err := backend.GetPolicyVersion("keep-policy", "1")
	require.NoError(t, err)
	assert.Equal(t, "1", v.VersionID)
}

func TestListPolicies_Sorted(t *testing.T) {
	t.Parallel()

	backend := iot.NewInMemoryBackend()
	backend.AddPolicyInternal(iot.Policy{PolicyName: "z-policy"})
	backend.AddPolicyInternal(iot.Policy{PolicyName: "a-policy"})
	backend.AddPolicyInternal(iot.Policy{PolicyName: "m-policy"})

	policies := backend.ListPolicies()
	require.Len(t, policies, 3)
	assert.Equal(t, "a-policy", policies[0].PolicyName)
	assert.Equal(t, "m-policy", policies[1].PolicyName)
	assert.Equal(t, "z-policy", policies[2].PolicyName)
}

func TestHandler_ListPolicies_Response(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		setupPolicies []string
		wantCount     int
	}{
		{
			name:          "empty_list",
			setupPolicies: nil,
			wantCount:     0,
		},
		{
			name:          "two_policies",
			setupPolicies: []string{"pol-a", "pol-b"},
			wantCount:     2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := iot.NewInMemoryBackend()
			h := iot.NewHandler(backend, nil)

			for _, name := range tt.setupPolicies {
				backend.AddPolicyInternal(iot.Policy{PolicyName: name})
			}

			resp := doRequest(t, h, http.MethodGet, "/policies", nil)
			require.Equal(t, http.StatusOK, resp.Code)

			var out map[string]any
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
			policies, ok := out["policies"]
			require.True(t, ok, "response must have 'policies' key")

			policiesSlice, ok := policies.([]any)
			require.True(t, ok)
			assert.Len(t, policiesSlice, tt.wantCount)
		})
	}
}

func TestHandler_GetPolicy_HTTP(t *testing.T) {
	t.Parallel()

	backend := iot.NewInMemoryBackend()
	h := iot.NewHandler(backend, nil)
	backend.AddPolicyInternal(iot.Policy{PolicyName: "http-policy", PolicyDocument: `{}`})

	resp := doRequest(t, h, http.MethodGet, "/policies/http-policy", nil)
	require.Equal(t, http.StatusOK, resp.Code)

	// creationDate/lastModifiedDate are JSON numbers (epoch seconds), not
	// RFC3339 strings, so this must decode into map[string]any.
	var out map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	assert.Equal(t, "http-policy", out["policyName"])
	assert.NotEmpty(t, out["policyArn"])

	for _, field := range []string{"creationDate", "lastModifiedDate"} {
		_, isNumber := out[field].(float64)
		assert.True(t, isNumber, "%s must be a JSON number (epoch seconds), got %T: %v", field, out[field], out[field])
	}
}

func TestHandler_DeletePolicy_HTTP(t *testing.T) {
	t.Parallel()

	backend := iot.NewInMemoryBackend()
	h := iot.NewHandler(backend, nil)
	backend.AddPolicyInternal(iot.Policy{PolicyName: "delete-http-policy"})

	resp := doRequest(t, h, http.MethodDelete, "/policies/delete-http-policy", nil)
	require.Equal(t, http.StatusNoContent, resp.Code)

	assert.Equal(t, 0, backend.PolicyCount())
}

func TestHandler_GetPolicy_NotFound_Returns404(t *testing.T) {
	t.Parallel()

	backend := iot.NewInMemoryBackend()
	h := iot.NewHandler(backend, nil)

	resp := doRequest(t, h, http.MethodGet, "/policies/nonexistent", nil)
	assert.Equal(t, http.StatusNotFound, resp.Code)
}

func TestPolicyTargetCount_Helper(t *testing.T) {
	t.Parallel()

	backend := iot.NewInMemoryBackend()
	backend.AddPolicyInternal(iot.Policy{PolicyName: "attach-test-policy"})
	assert.Equal(t, 0, backend.PolicyTargetCount("attach-test-policy"))

	require.NoError(t, backend.AttachPolicy(&iot.AttachPolicyInput{
		PolicyName: "attach-test-policy",
		Target:     "arn:aws:iot:us-east-1:000000000000:cert/abc",
	}))
	assert.Equal(t, 1, backend.PolicyTargetCount("attach-test-policy"))
}

func TestCreatePolicy_ReturnsPolicyVersionID(t *testing.T) {
	t.Parallel()

	h, _ := newR3Handler()
	var out map[string]string
	code := r3JSON(t, h, http.MethodPost, "/policies/test-policy", map[string]any{
		"policyDocument": `{"Version":"2012-10-17"}`,
	}, &out)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "1", out["policyVersionId"])
}

func TestCreatePolicy_AutoCreatesVersion1(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	_, err := b.CreatePolicy(&iot.CreatePolicyInput{
		PolicyName:     "auto-version-policy",
		PolicyDocument: `{"Version":"2012-10-17"}`,
	})
	require.NoError(t, err)

	versions, err := b.ListPolicyVersions("auto-version-policy")
	require.NoError(t, err)
	require.Len(t, versions, 1)
	assert.Equal(t, "1", versions[0].VersionID)
	assert.True(t, versions[0].IsDefaultVersion)
}

func TestCreatePolicy_Version1IsDefault(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	_, err := b.CreatePolicy(&iot.CreatePolicyInput{
		PolicyName:     "default-v1-policy",
		PolicyDocument: `{"Version":"2012-10-17","Statement":[]}`,
	})
	require.NoError(t, err)

	pv, err := b.GetPolicyVersion("default-v1-policy", "1")
	require.NoError(t, err)
	assert.True(t, pv.IsDefaultVersion)
}

func TestGetPolicy_ReturnsDefaultVersionID(t *testing.T) {
	t.Parallel()

	h, _ := newR3Handler()
	r3Req(t, h, http.MethodPost, "/policies/versioned-policy", map[string]any{
		"policyDocument": `{"Version":"2012-10-17"}`,
	})

	var out map[string]any
	code := r3JSON(t, h, http.MethodGet, "/policies/versioned-policy", nil, &out)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "1", out["defaultVersionId"])
}

func TestGetPolicy_ReturnsCreationDate(t *testing.T) {
	t.Parallel()

	h, _ := newR3Handler()
	r3Req(t, h, http.MethodPost, "/policies/dated-policy", map[string]any{
		"policyDocument": `{"Version":"2012-10-17"}`,
	})

	var out map[string]any
	code := r3JSON(t, h, http.MethodGet, "/policies/dated-policy", nil, &out)
	require.Equal(t, http.StatusOK, code)
	assert.NotEmpty(t, out["creationDate"])
	assert.NotEmpty(t, out["lastModifiedDate"])
}

func TestGetPolicy_DefaultVersionUpdatesAfterSetDefault(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	_, err := b.CreatePolicy(&iot.CreatePolicyInput{
		PolicyName:     "multi-ver-policy",
		PolicyDocument: `{"Version":"2012-10-17"}`,
	})
	require.NoError(t, err)

	_, err = b.CreatePolicyVersion(&iot.CreatePolicyVersionInput{
		PolicyName:     "multi-ver-policy",
		PolicyDocument: `{"Version":"2012-10-17","Statement":[]}`,
		SetAsDefault:   true,
	})
	require.NoError(t, err)

	out, err := b.GetPolicy("multi-ver-policy")
	require.NoError(t, err)
	assert.Equal(t, "2", out.DefaultVersionID)
}

func TestPolicy_CreationDateNotZero(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	_, err := b.CreatePolicy(&iot.CreatePolicyInput{
		PolicyName:     "ts-policy",
		PolicyDocument: `{}`,
	})
	require.NoError(t, err)

	out, err := b.GetPolicy("ts-policy")
	require.NoError(t, err)
	assert.False(t, out.CreatedAt.IsZero(), "CreatedAt should not be zero")
	assert.False(t, out.LastModifiedAt.IsZero(), "LastModifiedAt should not be zero")
}

func TestPolicy_LastModifiedUpdatesOnNewVersion(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	_, err := b.CreatePolicy(&iot.CreatePolicyInput{
		PolicyName:     "lm-policy",
		PolicyDocument: `{}`,
	})
	require.NoError(t, err)

	before, err := b.GetPolicy("lm-policy")
	require.NoError(t, err)

	_, err = b.CreatePolicyVersion(&iot.CreatePolicyVersionInput{
		PolicyName:     "lm-policy",
		PolicyDocument: `{"updated": true}`,
		SetAsDefault:   false,
	})
	require.NoError(t, err)

	after, err := b.GetPolicy("lm-policy")
	require.NoError(t, err)

	assert.False(t, after.LastModifiedAt.Before(before.LastModifiedAt),
		"LastModifiedAt should be >= initial value after CreatePolicyVersion")
}

func TestDeletePolicyVersion_DefaultVersionBlocked(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	_, err := b.CreatePolicy(&iot.CreatePolicyInput{
		PolicyName:     "del-default-policy",
		PolicyDocument: `{}`,
	})
	require.NoError(t, err)

	err = b.DeletePolicyVersion("del-default-policy", "1")
	require.ErrorIs(t, err, iot.ErrDeleteConflict)
}

func TestDeletePolicyVersion_NonDefaultCanBeDeleted(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	_, err := b.CreatePolicy(&iot.CreatePolicyInput{
		PolicyName:     "del-v2-policy",
		PolicyDocument: `{}`,
	})
	require.NoError(t, err)

	_, err = b.CreatePolicyVersion(&iot.CreatePolicyVersionInput{
		PolicyName:     "del-v2-policy",
		PolicyDocument: `{"updated": true}`,
		SetAsDefault:   false,
	})
	require.NoError(t, err)

	err = b.DeletePolicyVersion("del-v2-policy", "2")
	require.NoError(t, err)
}

func TestDeletePolicyVersion_AfterSetDefault_OldDefaultCanBeDeleted(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	_, err := b.CreatePolicy(&iot.CreatePolicyInput{
		PolicyName:     "swap-default-policy",
		PolicyDocument: `{}`,
	})
	require.NoError(t, err)

	_, err = b.CreatePolicyVersion(&iot.CreatePolicyVersionInput{
		PolicyName:     "swap-default-policy",
		PolicyDocument: `{"v2": true}`,
		SetAsDefault:   true,
	})
	require.NoError(t, err)

	err = b.DeletePolicyVersion("swap-default-policy", "1")
	require.NoError(t, err)
}

func TestDeletePolicyVersion_DefaultVersion_DeleteConflict(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	_, err := b.CreatePolicy(&iot.CreatePolicyInput{
		PolicyName:     "del-def-conflict",
		PolicyDocument: `{}`,
	})
	require.NoError(t, err)

	// Delete version 1 (the default) — should fail with ErrDeleteConflict
	err = b.DeletePolicyVersion("del-def-conflict", "1")
	require.ErrorIs(t, err, iot.ErrDeleteConflict)
}

func TestErrorFormat_PolicyNotFound_UsesAWSFormat(t *testing.T) {
	t.Parallel()

	h, _ := newR3Handler()
	var out map[string]string
	code := r3JSON(t, h, http.MethodGet, "/policies/missing-policy", nil, &out)
	assert.Equal(t, http.StatusNotFound, code)
	assert.Equal(t, "ResourceNotFoundException", out["__type"])
}

func TestPolicyVersion_CreateAndList(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	_, err := b.CreatePolicy(&iot.CreatePolicyInput{
		PolicyName:     "vlist-policy",
		PolicyDocument: `{}`,
	})
	require.NoError(t, err)

	_, err = b.CreatePolicyVersion(&iot.CreatePolicyVersionInput{
		PolicyName:     "vlist-policy",
		PolicyDocument: `{"v2": true}`,
		SetAsDefault:   false,
	})
	require.NoError(t, err)

	versions, err := b.ListPolicyVersions("vlist-policy")
	require.NoError(t, err)
	assert.Len(t, versions, 2, "should have version 1 (auto-created) and version 2")
}

func TestPolicyVersion_SetDefault_ClearsOldDefault(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	_, err := b.CreatePolicy(&iot.CreatePolicyInput{
		PolicyName:     "setdef-policy",
		PolicyDocument: `{}`,
	})
	require.NoError(t, err)

	_, err = b.CreatePolicyVersion(&iot.CreatePolicyVersionInput{
		PolicyName:     "setdef-policy",
		PolicyDocument: `{"v2": true}`,
		SetAsDefault:   false,
	})
	require.NoError(t, err)

	err = b.SetDefaultPolicyVersion("setdef-policy", "2")
	require.NoError(t, err)

	v1, err := b.GetPolicyVersion("setdef-policy", "1")
	require.NoError(t, err)
	assert.False(t, v1.IsDefaultVersion, "version 1 should no longer be default")

	v2, err := b.GetPolicyVersion("setdef-policy", "2")
	require.NoError(t, err)
	assert.True(t, v2.IsDefaultVersion, "version 2 should now be default")
}

func TestCreatePolicy_DocumentMatchesVersion1(t *testing.T) {
	t.Parallel()

	const doc = `{"Version":"2012-10-17","Statement":[]}`
	_, b := newR3Handler()
	_, err := b.CreatePolicy(&iot.CreatePolicyInput{
		PolicyName:     "docmatch-policy",
		PolicyDocument: doc,
	})
	require.NoError(t, err)

	pv, err := b.GetPolicyVersion("docmatch-policy", "1")
	require.NoError(t, err)
	assert.JSONEq(t, doc, pv.PolicyDocument)
}

func TestGetPolicyVersion_NotFound_AfterDelete(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	_, err := b.CreatePolicy(&iot.CreatePolicyInput{
		PolicyName:     "del-get-policy",
		PolicyDocument: `{}`,
	})
	require.NoError(t, err)

	_, err = b.CreatePolicyVersion(&iot.CreatePolicyVersionInput{
		PolicyName:     "del-get-policy",
		PolicyDocument: `{"v2": true}`,
		SetAsDefault:   true,
	})
	require.NoError(t, err)

	err = b.DeletePolicyVersion("del-get-policy", "1")
	require.NoError(t, err)

	_, err = b.GetPolicyVersion("del-get-policy", "1")
	require.ErrorIs(t, err, iot.ErrPolicyVersionNotFound)
}

func TestReset_ClearsPoliciesAndVersions(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	_, err := b.CreatePolicy(&iot.CreatePolicyInput{
		PolicyName:     "reset-ver-policy",
		PolicyDocument: `{}`,
	})
	require.NoError(t, err)

	b.Reset()

	_, err = b.ListPolicyVersions("reset-ver-policy")
	require.ErrorIs(t, err, iot.ErrPolicyNotFound)
}

func TestGetPolicy_ReturnsSameDocument(t *testing.T) {
	t.Parallel()

	const doc = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow"}]}`
	h, _ := newR3Handler()
	r3Req(t, h, http.MethodPost, "/policies/doc-policy", map[string]any{
		"policyDocument": doc,
	})

	var out map[string]any
	code := r3JSON(t, h, http.MethodGet, "/policies/doc-policy", nil, &out)
	require.Equal(t, http.StatusOK, code)
	assert.JSONEq(t, doc, out["policyDocument"].(string))
}

func TestListPolicies_SortedByName(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	names := []string{"zoo-policy", "ant-policy", "mid-policy"}
	for _, n := range names {
		_, err := b.CreatePolicy(&iot.CreatePolicyInput{
			PolicyName:     n,
			PolicyDocument: `{}`,
		})
		require.NoError(t, err)
	}

	policies := b.ListPolicies()
	require.Len(t, policies, 3)
	assert.Equal(t, "ant-policy", policies[0].PolicyName)
	assert.Equal(t, "mid-policy", policies[1].PolicyName)
	assert.Equal(t, "zoo-policy", policies[2].PolicyName)
}

func TestDeletePolicy_RemovesFromList(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	_, err := b.CreatePolicy(&iot.CreatePolicyInput{
		PolicyName:     "to-delete-policy",
		PolicyDocument: `{}`,
	})
	require.NoError(t, err)

	err = b.DeletePolicy("to-delete-policy")
	require.NoError(t, err)

	_, err = b.GetPolicy("to-delete-policy")
	require.ErrorIs(t, err, iot.ErrPolicyNotFound)
}

func TestCreatePolicy_ARNFormat(t *testing.T) {
	t.Parallel()

	b := iot.NewInMemoryBackendWithConfig("555566667777", "ca-central-1")
	out, err := b.CreatePolicy(&iot.CreatePolicyInput{
		PolicyName:     "arn-test-policy",
		PolicyDocument: `{}`,
	})
	require.NoError(t, err)
	assert.Equal(t, "arn:aws:iot:ca-central-1:555566667777:policy/arn-test-policy", out.PolicyARN)
}

func TestGetPolicy_DefaultVersionID_AfterMultipleVersions(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	_, err := b.CreatePolicy(&iot.CreatePolicyInput{
		PolicyName:     "multi-default-policy",
		PolicyDocument: `{"v": 1}`,
	})
	require.NoError(t, err)

	// Add versions 2 and 3 without setting as default
	for range 2 {
		_, err = b.CreatePolicyVersion(&iot.CreatePolicyVersionInput{
			PolicyName:     "multi-default-policy",
			PolicyDocument: `{}`,
			SetAsDefault:   false,
		})
		require.NoError(t, err)
	}

	// Version 1 should still be default
	out, err := b.GetPolicy("multi-default-policy")
	require.NoError(t, err)
	assert.Equal(t, "1", out.DefaultVersionID)

	// Add version 4 and set as default
	_, err = b.CreatePolicyVersion(&iot.CreatePolicyVersionInput{
		PolicyName:     "multi-default-policy",
		PolicyDocument: `{"v": 4}`,
		SetAsDefault:   true,
	})
	require.NoError(t, err)

	out, err = b.GetPolicy("multi-default-policy")
	require.NoError(t, err)
	assert.Equal(t, "4", out.DefaultVersionID)
}

// TestParityB_PolicyVersionsPath verifies CreatePolicyVersion uses /versions (plural).
func TestPolicyVersionsPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{
			name:       "create_version_plural_path",
			path:       "/policies/parb-pol/versions",
			wantStatus: http.StatusOK,
		},
		{
			name:       "list_versions_plural_path",
			path:       "/policies/parb-pol/versions",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			doRequest(t, h, http.MethodPost, "/policies/parb-pol",
				map[string]string{"policyDocument": `{"Version":"2012-10-17"}`})

			var rec *httptest.ResponseRecorder
			if tt.name == "create_version_plural_path" {
				rec = doRequest(t, h, http.MethodPost, tt.path,
					map[string]string{"policyDocument": `{"Version":"2012-10-17","v2":true}`})
			} else {
				rec = doRequest(t, h, http.MethodGet, tt.path, nil)
			}

			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestParityB_GetPolicyVersion_ReturnsPolicyArnAndName verifies GetPolicyVersion returns policyArn and policyName.
func TestGetPolicyVersion_ReturnsPolicyArnAndName(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	doRequest(t, h, http.MethodPost, "/policies/parb-pol2",
		map[string]string{"policyDocument": `{"Version":"2012-10-17"}`})
	doRequest(t, h, http.MethodPost, "/policies/parb-pol2/versions?setAsDefault=true",
		map[string]string{"policyDocument": `{"Version":"2012-10-17","v2":true}`})

	rec := doRequest(t, h, http.MethodGet, "/policies/parb-pol2/versions/2", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "parb-pol2", resp["policyName"], "policyName must be in GetPolicyVersion response")
	policyARN, _ := resp["policyArn"].(string)
	assert.NotEmpty(t, policyARN, "policyArn must be in GetPolicyVersion response")
}

// TestParityB_DeletePolicy_WithTargets_Blocked verifies policies with attached targets
// cannot be deleted.
func TestDeletePolicy_WithTargets_Blocked(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantDeleteErr error
		name          string
		attachTarget  bool
	}{
		{
			name:          "with_target_blocked",
			attachTarget:  true,
			wantDeleteErr: iot.ErrDeleteConflict,
		},
		{
			name:          "no_target_allowed",
			attachTarget:  false,
			wantDeleteErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := iot.NewInMemoryBackend()
			b.AddPolicyInternal(iot.Policy{PolicyName: "del-pol-" + tt.name, PolicyDocument: `{}`})

			if tt.attachTarget {
				err := b.AttachPolicy(&iot.AttachPolicyInput{
					PolicyName: "del-pol-" + tt.name,
					Target:     "arn:aws:iot:us-east-1:000000000000:cert/abc",
				})
				require.NoError(t, err)
			}

			err := b.DeletePolicy("del-pol-" + tt.name)
			if tt.wantDeleteErr != nil {
				require.ErrorIs(t, err, tt.wantDeleteErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestParityB_PolicyVersionLimit verifies a policy cannot have more than 5 versions.
func TestPolicyVersionLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		extraCounts int
		wantErr     bool
	}{
		{name: "four_versions_ok", extraCounts: 4, wantErr: false},
		{name: "five_versions_at_limit", extraCounts: 5, wantErr: false},
		{name: "six_versions_exceeds_limit", extraCounts: 6, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := iot.NewInMemoryBackend()
			b.AddPolicyInternal(iot.Policy{PolicyName: "limit-pol", PolicyDocument: `{}`})

			var lastErr error
			for range tt.extraCounts {
				_, lastErr = b.CreatePolicyVersion(&iot.CreatePolicyVersionInput{
					PolicyName:     "limit-pol",
					PolicyDocument: `{}`,
				})
			}

			if tt.wantErr {
				require.ErrorIs(t, lastErr, iot.ErrVersionsLimitExceeded)
			} else {
				require.NoError(t, lastErr)
			}
		})
	}
}

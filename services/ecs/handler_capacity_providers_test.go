package ecs_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/ecs"
)

func TestCapacityProvider_ASGBacked_Roundtrip(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	resp := doECSRequest(t, h, "CreateCapacityProvider", map[string]any{
		"name": "asg-cp",
		"autoScalingGroupProvider": map[string]any{
			"autoScalingGroupArn":          "arn:aws:autoscaling:us-east-1:000000000000:autoScalingGroup:asg-1",
			"managedTerminationProtection": "ENABLED",
			"managedDraining":              "ENABLED",
			"managedScaling": map[string]any{
				"status":                 "ENABLED",
				"targetCapacity":         100,
				"minimumScalingStepSize": 1,
				"maximumScalingStepSize": 10,
				"instanceWarmupPeriod":   300,
			},
		},
	})
	require.Equal(t, http.StatusOK, resp.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
	cp := out["capacityProvider"].(map[string]any)
	require.Equal(t, "asg-cp", cp["name"])

	asg := cp["autoScalingGroupProvider"].(map[string]any)
	assert.Equal(
		t,
		"arn:aws:autoscaling:us-east-1:000000000000:autoScalingGroup:asg-1",
		asg["autoScalingGroupArn"],
	)
	assert.Equal(t, "ENABLED", asg["managedTerminationProtection"])
	assert.Equal(t, "ENABLED", asg["managedDraining"])

	ms := asg["managedScaling"].(map[string]any)
	assert.Equal(t, "ENABLED", ms["status"])
	assert.InDelta(t, float64(100), ms["targetCapacity"], 0.001)
	assert.InDelta(t, float64(1), ms["minimumScalingStepSize"], 0.001)
	assert.InDelta(t, float64(10), ms["maximumScalingStepSize"], 0.001)
	assert.InDelta(t, float64(300), ms["instanceWarmupPeriod"], 0.001)
}

func TestCapacityProvider_ASGBacked_NoManagedScaling(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	resp := doECSRequest(t, h, "CreateCapacityProvider", map[string]any{
		"name": "asg-cp-minimal",
		"autoScalingGroupProvider": map[string]any{
			"autoScalingGroupArn": "arn:aws:autoscaling:us-east-1:000000000000:autoScalingGroup:asg-2",
		},
	})
	require.Equal(t, http.StatusOK, resp.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
	cp := out["capacityProvider"].(map[string]any)
	asg := cp["autoScalingGroupProvider"].(map[string]any)
	assert.Nil(t, asg["managedScaling"])
}

func TestCapacityProvider_FARGATE_Types(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Create FARGATE-named and FARGATE_SPOT-named capacity providers
	for _, name := range []string{"FARGATE", "FARGATE_SPOT"} {
		resp := doECSRequest(t, h, "CreateCapacityProvider", map[string]any{"name": name})
		require.Equal(t, http.StatusOK, resp.Code)
	}

	// Describe them by explicit name
	resp := doECSRequest(t, h, "DescribeCapacityProviders", map[string]any{
		"capacityProviders": []string{"FARGATE", "FARGATE_SPOT"},
	})
	require.Equal(t, http.StatusOK, resp.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
	providers := out["capacityProviders"].([]any)
	require.Len(t, providers, 2)

	names := make([]string, 0, 2)
	for _, p := range providers {
		names = append(names, p.(map[string]any)["name"].(string))
	}
	assert.Contains(t, names, "FARGATE")
	assert.Contains(t, names, "FARGATE_SPOT")
}

// TestCapacityProvider_Update_ManagedScaling proves UpdateCapacityProvider's
// autoScalingGroupProvider wire shape matches the real
// AutoScalingGroupProviderUpdate: no autoScalingGroupArn field (a real typed
// client cannot send one -- the ASG a capacity provider wraps cannot be
// swapped after creation), yet the ARN set at creation time must survive an
// update that only touches managedScaling.
func TestCapacityProvider_Update_ManagedScaling(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	const wantArn = "arn:aws:autoscaling:us-east-1:000000000000:autoScalingGroup:asg-3"

	doECSRequest(t, h, "CreateCapacityProvider", map[string]any{
		"name": "asg-update-cp",
		"autoScalingGroupProvider": map[string]any{
			"autoScalingGroupArn": wantArn,
			"managedScaling": map[string]any{
				"status":         "ENABLED",
				"targetCapacity": 75,
			},
		},
	})

	resp := doECSRequest(t, h, "UpdateCapacityProvider", map[string]any{
		"name": "asg-update-cp",
		"autoScalingGroupProvider": map[string]any{
			"managedScaling": map[string]any{
				"status":         "ENABLED",
				"targetCapacity": 90,
			},
		},
	})
	require.Equal(t, http.StatusOK, resp.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
	cp := out["capacityProvider"].(map[string]any)
	asg := cp["autoScalingGroupProvider"].(map[string]any)
	ms := asg["managedScaling"].(map[string]any)
	assert.InDelta(t, float64(90), ms["targetCapacity"], 0.001)
	assert.Equal(t, wantArn, asg["autoScalingGroupArn"],
		"the ASG's ARN must survive an update since it cannot be changed")
}

func TestCapacityProvider_DeleteByARN(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	createResp := doECSRequest(t, h, "CreateCapacityProvider", map[string]any{
		"name": "del-by-arn-cp",
	})
	require.Equal(t, http.StatusOK, createResp.Code)
	var createOut map[string]any
	require.NoError(t, json.Unmarshal(createResp.Body.Bytes(), &createOut))
	cpArn := createOut["capacityProvider"].(map[string]any)["capacityProviderArn"].(string)
	require.NotEmpty(t, cpArn)

	// Delete by ARN
	deleteResp := doECSRequest(t, h, "DeleteCapacityProvider", map[string]any{
		"capacityProvider": cpArn,
	})
	require.Equal(t, http.StatusOK, deleteResp.Code)
}

func TestCapacityProvider_AlreadyExists(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doECSRequest(t, h, "CreateCapacityProvider", map[string]any{"name": "dup-cp"})
	dupResp := doECSRequest(t, h, "CreateCapacityProvider", map[string]any{"name": "dup-cp"})
	require.Equal(t, http.StatusBadRequest, dupResp.Code)
}

func TestCapacityProvider_FARGATE_BuiltIn_DescribeWithoutCreate(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	resp := doECSRequest(t, h, "DescribeCapacityProviders", map[string]any{
		"capacityProviders": []string{"FARGATE"},
	})
	require.Equal(t, http.StatusOK, resp.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
	providers := out["capacityProviders"].([]any)
	require.Len(t, providers, 1)
	cp := providers[0].(map[string]any)
	assert.Equal(t, "FARGATE", cp["name"])
	assert.Equal(t, "ACTIVE", cp["status"])
}

func TestCapacityProvider_FARGATE_SPOT_BuiltIn(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	resp := doECSRequest(t, h, "DescribeCapacityProviders", map[string]any{
		"capacityProviders": []string{"FARGATE", "FARGATE_SPOT"},
	})
	require.Equal(t, http.StatusOK, resp.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
	providers := out["capacityProviders"].([]any)
	require.Len(t, providers, 2)

	names := make([]string, 0, 2)
	for _, p := range providers {
		names = append(names, p.(map[string]any)["name"].(string))
	}
	assert.Contains(t, names, "FARGATE")
	assert.Contains(t, names, "FARGATE_SPOT")
}

func TestCapacityProvider_FARGATE_BuiltIn_WithCreated(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doECSRequest(t, h, "CreateCapacityProvider", map[string]any{
		"name": "my-asg-cp",
		"autoScalingGroupProvider": map[string]any{
			"autoScalingGroupArn": "arn:aws:autoscaling:us-east-1:000000000000:autoScalingGroup:asg-1",
		},
	})

	// Describe FARGATE (built-in) and custom together
	resp := doECSRequest(t, h, "DescribeCapacityProviders", map[string]any{
		"capacityProviders": []string{"FARGATE", "my-asg-cp"},
	})
	require.Equal(t, http.StatusOK, resp.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
	providers := out["capacityProviders"].([]any)
	require.Len(t, providers, 2)

	names := make([]string, 0, 2)
	for _, p := range providers {
		names = append(names, p.(map[string]any)["name"].(string))
	}
	assert.Contains(t, names, "FARGATE")
	assert.Contains(t, names, "my-asg-cp")
}

func TestCapacityProvider_Unknown_ReturnsFailure(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// AWS reports unknown capacity providers via the Failures list (reason
	// MISSING) with a 200 response, not a whole-request error.
	resp := doECSRequest(t, h, "DescribeCapacityProviders", map[string]any{
		"capacityProviders": []string{"nonexistent-cp"},
	})
	require.Equal(t, http.StatusOK, resp.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
	assert.Empty(t, out["capacityProviders"])

	failures := out["failures"].([]any)
	require.Len(t, failures, 1)
	f := failures[0].(map[string]any)
	assert.Equal(t, "nonexistent-cp", f["arn"])
	assert.Equal(t, "MISSING", f["reason"])
}

func TestECS_CreateCapacityProvider(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    map[string]any
		name     string
		wantName string
		wantCode int
	}{
		{
			name:     "creates capacity provider",
			input:    map[string]any{"name": "my-cp"},
			wantCode: http.StatusOK,
			wantName: "my-cp",
		},
		{
			name:     "missing name returns 400",
			input:    map[string]any{},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "with tags",
			input: map[string]any{
				"name": "tagged-cp",
				"tags": []map[string]any{{"key": "env", "value": "test"}},
			},
			wantCode: http.StatusOK,
			wantName: "tagged-cp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doECSRequest(t, h, "CreateCapacityProvider", tt.input)

			require.Equal(t, tt.wantCode, rec.Code)

			if tt.wantCode != http.StatusOK {
				return
			}

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

			cp, ok := resp["capacityProvider"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, tt.wantName, cp["name"])
			assert.Equal(t, "ACTIVE", cp["status"])
			assert.NotEmpty(t, cp["capacityProviderArn"])
		})
	}
}

// TestECS_CreateCapacityProvider_AlreadyExists asserts the real code:
// CreateCapacityProvider's own deserializer models no "already exists"
// exception (no such shape exists anywhere in ecs@v1.90.0), only
// InvalidParameterException for a duplicate name.
func TestECS_CreateCapacityProvider_AlreadyExists(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doECSRequest(t, h, "CreateCapacityProvider", map[string]any{"name": "my-cp"})
	rec := doECSRequest(t, h, "CreateCapacityProvider", map[string]any{"name": "my-cp"})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "InvalidParameterException")
}

func TestECS_DeleteCapacityProvider(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cpName   string
		deleteBy string
		wantCode int
	}{
		{
			name:     "delete by name",
			cpName:   "my-cp",
			deleteBy: "my-cp",
			wantCode: http.StatusOK,
		},
		{
			name:     "delete non-existent returns 400",
			cpName:   "",
			deleteBy: "non-existent",
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			if tt.cpName != "" {
				doECSRequest(t, h, "CreateCapacityProvider", map[string]any{"name": tt.cpName})
			}

			rec := doECSRequest(
				t,
				h,
				"DeleteCapacityProvider",
				map[string]any{"capacityProvider": tt.deleteBy},
			)

			require.Equal(t, tt.wantCode, rec.Code)

			if tt.wantCode != http.StatusOK {
				return
			}

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

			cp, ok := resp["capacityProvider"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, tt.cpName, cp["name"])
		})
	}
}

// TestECS_DeleteCapacityProvider_InUse verifies AWS's DeleteCapacityProvider
// guard: a capacity provider associated with a cluster cannot be deleted.
func TestECS_DeleteCapacityProvider_InUse(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doECSRequest(t, h, "CreateCapacityProvider", map[string]any{
		"name": "in-use-cp",
		"autoScalingGroupProvider": map[string]any{
			"autoScalingGroupArn": "arn:aws:autoscaling:us-east-1:123456789012:autoScalingGroup:x:autoScalingGroupName/asg",
		},
	})
	doECSRequest(t, h, "CreateCluster", map[string]any{"clusterName": "cp-in-use-cluster"})

	putResp := doECSRequest(t, h, "PutClusterCapacityProviders", map[string]any{
		"cluster":           "cp-in-use-cluster",
		"capacityProviders": []string{"in-use-cp"},
	})
	require.Equal(t, http.StatusOK, putResp.Code)

	deleteResp := doECSRequest(t, h, "DeleteCapacityProvider", map[string]any{
		"capacityProvider": "in-use-cp",
	})
	require.Equal(t, http.StatusBadRequest, deleteResp.Code)

	// Detach it from the cluster; deletion then succeeds.
	putResp = doECSRequest(t, h, "PutClusterCapacityProviders", map[string]any{
		"cluster":           "cp-in-use-cluster",
		"capacityProviders": []string{},
	})
	require.Equal(t, http.StatusOK, putResp.Code)

	deleteResp = doECSRequest(t, h, "DeleteCapacityProvider", map[string]any{
		"capacityProvider": "in-use-cp",
	})
	require.Equal(t, http.StatusOK, deleteResp.Code)
}

func TestECS_DescribeCapacityProviders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input        map[string]any
		name         string
		seedNames    []string
		wantCount    int
		wantFailures int
		wantCode     int
	}{
		{
			name:      "describe all when no filter",
			seedNames: []string{"cp-a", "cp-b"},
			input:     map[string]any{},
			wantCode:  http.StatusOK,
			wantCount: 2,
		},
		{
			name:      "describe specific by name",
			seedNames: []string{"cp-a", "cp-b"},
			input:     map[string]any{"capacityProviders": []string{"cp-a"}},
			wantCode:  http.StatusOK,
			wantCount: 1,
		},
		{
			name:      "empty backend returns empty list",
			seedNames: nil,
			input:     map[string]any{},
			wantCode:  http.StatusOK,
			wantCount: 0,
		},
		{
			// AWS reports unknown capacity providers as a Failure entry (reason
			// MISSING), not as a whole-request error — matches DescribeClusters,
			// DescribeContainerInstances, etc.
			name:         "unknown capacity provider returns failure not error",
			input:        map[string]any{"capacityProviders": []string{"unknown-cp"}},
			wantCode:     http.StatusOK,
			wantCount:    0,
			wantFailures: 1,
		},
		{
			name:         "mix of known and unknown returns partial success",
			seedNames:    []string{"cp-a"},
			input:        map[string]any{"capacityProviders": []string{"cp-a", "unknown-cp"}},
			wantCode:     http.StatusOK,
			wantCount:    1,
			wantFailures: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			for _, n := range tt.seedNames {
				doECSRequest(t, h, "CreateCapacityProvider", map[string]any{"name": n})
			}

			rec := doECSRequest(t, h, "DescribeCapacityProviders", tt.input)

			require.Equal(t, tt.wantCode, rec.Code)

			if tt.wantCode != http.StatusOK {
				return
			}

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

			providers, ok := resp["capacityProviders"].([]any)
			require.True(t, ok)
			assert.Len(t, providers, tt.wantCount)

			failures, ok := resp["failures"].([]any)
			require.True(t, ok)
			assert.Len(t, failures, tt.wantFailures)
		})
	}
}

func TestECS_DescribeCapacityProviders_SortedByName(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	for _, n := range []string{"cp-z", "cp-a", "cp-m"} {
		doECSRequest(t, h, "CreateCapacityProvider", map[string]any{"name": n})
	}

	rec := doECSRequest(t, h, "DescribeCapacityProviders", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	providers, ok := resp["capacityProviders"].([]any)
	require.True(t, ok)
	require.Len(t, providers, 3)

	assert.Equal(t, "cp-a", providers[0].(map[string]any)["name"])
	assert.Equal(t, "cp-m", providers[1].(map[string]any)["name"])
	assert.Equal(t, "cp-z", providers[2].(map[string]any)["name"])
}

func TestECS_DescribeCapacityProviders_ByARN(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doECSRequest(t, h, "CreateCapacityProvider", map[string]any{"name": "my-cp"})
	require.Equal(t, http.StatusOK, rec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))

	arn := createResp["capacityProvider"].(map[string]any)["capacityProviderArn"].(string)

	rec = doECSRequest(
		t,
		h,
		"DescribeCapacityProviders",
		map[string]any{"capacityProviders": []string{arn}},
	)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	providers, ok := resp["capacityProviders"].([]any)
	require.True(t, ok)
	require.Len(t, providers, 1)
	assert.Equal(t, "my-cp", providers[0].(map[string]any)["name"])
}

// TestECS_UpdateCapacityProvider verifies UpdateCapacityProvider.
func TestECS_UpdateCapacityProvider(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    map[string]any
		name     string
		wantCode int
	}{
		{
			// No status field: the real UpdateCapacityProviderRequest has
			// no such field (status only ever transitions via
			// CreateCapacityProvider/DeleteCapacityProvider).
			name:     "update existing",
			input:    map[string]any{"name": "my-cp"},
			wantCode: http.StatusOK,
		},
		{
			name:     "not found",
			input:    map[string]any{"name": "missing"},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			if tt.wantCode == http.StatusOK {
				doECSRequest(t, h, "CreateCapacityProvider", map[string]any{"name": "my-cp"})
			}

			rec := doECSRequest(t, h, "UpdateCapacityProvider", tt.input)
			require.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

func TestCapacityProvider_DeepCopy_Tags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		tagKey  string
		tagVal  string
		wantTag string
		mutate  bool
	}{
		{
			name:    "tags are independent after create",
			tagKey:  "env",
			tagVal:  "prod",
			mutate:  true,
			wantTag: "prod",
		},
		{
			name:    "no tags returns nil slice",
			tagKey:  "",
			tagVal:  "",
			mutate:  false,
			wantTag: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := ecs.NewInMemoryBackend(testAccountID, testRegion, ecs.NewNoopRunner())

			tags := []ecs.Tag{}
			if tt.tagKey != "" {
				tags = append(tags, ecs.Tag{Key: tt.tagKey, Value: tt.tagVal})
			}

			cp, err := b.CreateCapacityProvider(ecs.CreateCapacityProviderInput{
				Name: "deep-copy-test",
				Tags: tags,
			})
			require.NoError(t, err)

			if tt.mutate && len(cp.Tags) > 0 {
				cp.Tags[0].Value = "mutated"
			}

			cps, _, err := b.DescribeCapacityProviders([]string{"deep-copy-test"}, "")
			require.NoError(t, err)
			require.Len(t, cps, 1)

			if tt.tagKey != "" {
				require.Len(t, cps[0].Tags, 1)
				assert.Equal(t, tt.wantTag, cps[0].Tags[0].Value)
			} else {
				assert.Empty(t, cps[0].Tags)
			}
		})
	}
}

func TestDescribeCapacityProviders_Sorted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		cpNames   []string
		wantOrder []string
	}{
		{
			name:      "multiple providers returned",
			cpNames:   []string{"alpha", "beta", "gamma"},
			wantOrder: []string{"alpha", "beta", "gamma"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			for _, name := range tt.cpNames {
				rec := doECSRequest(t, h, "CreateCapacityProvider", map[string]any{"name": name})
				require.Equal(t, http.StatusOK, rec.Code)
			}

			rec := doECSRequest(t, h, "DescribeCapacityProviders", map[string]any{
				"capacityProviders": tt.cpNames,
			})
			require.Equal(t, http.StatusOK, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

			cps, ok := resp["capacityProviders"].([]any)
			require.True(t, ok)
			require.Len(t, cps, len(tt.wantOrder))

			for i, wantName := range tt.wantOrder {
				cp := cps[i].(map[string]any)
				assert.Equal(t, wantName, cp["name"])
			}
		})
	}
}

// TestDescribeCapacityProviders_TagsRequireInclude proves that
// DescribeCapacityProviders only returns tags when Include=["TAGS"] is
// specified, matching real AWS's DescribeCapacityProvidersInput.Include
// gating. Previously tags were always returned regardless of Include.
func TestDescribeCapacityProviders_TagsRequireInclude(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doECSRequest(t, h, "CreateCapacityProvider", map[string]any{
		"name": "tagged-cp",
		"tags": []map[string]any{{"key": "env", "value": "prod"}},
	})

	t.Run("without include", func(t *testing.T) {
		t.Parallel()

		rec := doECSRequest(t, h, "DescribeCapacityProviders", map[string]any{
			"capacityProviders": []string{"tagged-cp"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		cp := resp["capacityProviders"].([]any)[0].(map[string]any)
		assert.Nil(t, cp["tags"])
	})

	t.Run("with include TAGS", func(t *testing.T) {
		t.Parallel()

		rec := doECSRequest(t, h, "DescribeCapacityProviders", map[string]any{
			"capacityProviders": []string{"tagged-cp"},
			"include":           []string{"TAGS"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		cp := resp["capacityProviders"].([]any)[0].(map[string]any)
		tags := cp["tags"].([]any)
		require.Len(t, tags, 1)
		assert.Equal(t, "env", tags[0].(map[string]any)["key"])
	})
}

// TestDescribeCapacityProviders_ClusterFilter proves that the Cluster
// filter parameter restricts results to capacity providers associated with
// that cluster (via CreateCluster/PutClusterCapacityProviders), matching
// real AWS's documented DescribeCapacityProvidersInput.Cluster behavior.
// Previously this parameter was entirely unsupported.
func TestDescribeCapacityProviders_ClusterFilter(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doECSRequest(t, h, "CreateCapacityProvider", map[string]any{"name": "cp-in-cluster"})
	doECSRequest(t, h, "CreateCapacityProvider", map[string]any{"name": "cp-not-in-cluster"})

	doECSRequest(t, h, "CreateCluster", map[string]any{
		"clusterName":       "cp-filter-cluster",
		"capacityProviders": []string{"cp-in-cluster", "FARGATE"},
	})

	rec := doECSRequest(t, h, "DescribeCapacityProviders", map[string]any{
		"cluster": "cp-filter-cluster",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	cps := resp["capacityProviders"].([]any)
	require.Len(t, cps, 2)

	names := make(map[string]bool)
	for _, cp := range cps {
		names[cp.(map[string]any)["name"].(string)] = true
	}
	assert.True(t, names["cp-in-cluster"])
	assert.True(t, names["FARGATE"])
	assert.False(t, names["cp-not-in-cluster"])
}

// TestDescribeCapacityProviders_ClusterFilter_UnknownCluster proves that an
// unknown cluster name yields an empty result rather than an error, matching
// AWS's filter-parameter semantics (as opposed to the hard-404 semantics of
// naming an unknown cluster directly in DescribeClusters).
func TestDescribeCapacityProviders_ClusterFilter_UnknownCluster(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doECSRequest(t, h, "DescribeCapacityProviders", map[string]any{
		"cluster": "no-such-cluster",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	cps := resp["capacityProviders"].([]any)
	assert.Empty(t, cps)
}

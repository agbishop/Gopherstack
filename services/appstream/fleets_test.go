package appstream_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/appstream"
)

func TestAppStream_Fleets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(h *appstream.Handler)
		check    func(t *testing.T, body []byte)
		body     any
		name     string
		action   string
		wantCode int
	}{
		{
			name:   "CreateFleet returns fleet with ARN and STOPPED state",
			action: "CreateFleet",
			body: map[string]any{
				"Name":         "my-fleet",
				"InstanceType": "stream.standard.medium",
				"FleetType":    "ON_DEMAND",
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				fleet := resp["Fleet"].(map[string]any)
				assert.Equal(t, "my-fleet", fleet["Name"])
				assert.Equal(t, "STOPPED", fleet["State"])
				assert.Contains(t, fleet["Arn"], ":fleet/my-fleet")
			},
		},
		{
			name:   "CreateFleet duplicate returns error",
			action: "CreateFleet",
			setup: func(h *appstream.Handler) {
				createFleet(t, h, "dup-fleet")
			},
			body: map[string]any{
				"Name":         "dup-fleet",
				"InstanceType": "stream.standard.medium",
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name:   "DescribeFleets returns created fleet",
			action: "DescribeFleets",
			setup: func(h *appstream.Handler) {
				createFleet(t, h, "list-fleet")
			},
			body:     map[string]any{},
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				fleets := resp["Fleets"].([]any)
				assert.Len(t, fleets, 1)
			},
		},
		{
			name:   "StartFleet transitions to RUNNING",
			action: "StartFleet",
			setup: func(h *appstream.Handler) {
				createFleet(t, h, "start-fleet")
			},
			body:     map[string]any{"Name": "start-fleet"},
			wantCode: http.StatusOK,
		},
		{
			name:   "StopFleet transitions to STOPPED",
			action: "StopFleet",
			setup: func(h *appstream.Handler) {
				createFleet(t, h, "stop-fleet")
				doRequest(t, h, "StartFleet", map[string]any{"Name": "stop-fleet"})
			},
			body:     map[string]any{"Name": "stop-fleet"},
			wantCode: http.StatusOK,
		},
		{
			name:   "DeleteFleet while RUNNING returns error",
			action: "DeleteFleet",
			setup: func(h *appstream.Handler) {
				createFleet(t, h, "running-fleet")
				doRequest(t, h, "StartFleet", map[string]any{"Name": "running-fleet"})
			},
			body:     map[string]any{"Name": "running-fleet"},
			wantCode: http.StatusBadRequest,
		},
		{
			name:   "DeleteFleet while STOPPED succeeds",
			action: "DeleteFleet",
			setup: func(h *appstream.Handler) {
				createFleet(t, h, "del-fleet")
			},
			body:     map[string]any{"Name": "del-fleet"},
			wantCode: http.StatusOK,
		},
		{
			name:     "DeleteFleet unknown name returns error",
			action:   "DeleteFleet",
			body:     map[string]any{"Name": "no-such-fleet"},
			wantCode: http.StatusBadRequest,
		},
		{
			name:   "UpdateFleet changes instance type",
			action: "UpdateFleet",
			setup: func(h *appstream.Handler) {
				createFleet(t, h, "upd-fleet")
			},
			body:     map[string]any{"Name": "upd-fleet", "InstanceType": "stream.compute.large"},
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				fleet := resp["Fleet"].(map[string]any)
				assert.Equal(t, "stream.compute.large", fleet["InstanceType"])
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			if tc.setup != nil {
				tc.setup(h)
			}
			rec := doRequest(t, h, tc.action, tc.body)
			assert.Equal(t, tc.wantCode, rec.Code)
			if tc.check != nil {
				tc.check(t, rec.Body.Bytes())
			}
		})
	}
}

func TestAppStream_Associations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(h *appstream.Handler)
		check    func(t *testing.T, body []byte)
		body     any
		name     string
		action   string
		wantCode int
	}{
		{
			name:   "AssociateFleet links fleet and stack",
			action: "AssociateFleet",
			setup: func(h *appstream.Handler) {
				createFleet(t, h, "fleet-1")
				createStack(t, h, "stack-1")
			},
			body:     map[string]any{"FleetName": "fleet-1", "StackName": "stack-1"},
			wantCode: http.StatusOK,
		},
		{
			name:   "AssociateFleet unknown fleet returns error",
			action: "AssociateFleet",
			setup: func(h *appstream.Handler) {
				createStack(t, h, "stack-x")
			},
			body:     map[string]any{"FleetName": "no-fleet", "StackName": "stack-x"},
			wantCode: http.StatusBadRequest,
		},
		{
			name:   "ListAssociatedFleets returns associated fleet",
			action: "ListAssociatedFleets",
			setup: func(h *appstream.Handler) {
				createFleet(t, h, "fleet-2")
				createStack(t, h, "stack-2")
				doRequest(t, h, "AssociateFleet", map[string]any{"FleetName": "fleet-2", "StackName": "stack-2"})
			},
			body:     map[string]any{"StackName": "stack-2"},
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				names := resp["Names"].([]any)
				assert.Len(t, names, 1)
				assert.Equal(t, "fleet-2", names[0])
			},
		},
		{
			name:   "ListAssociatedStacks returns associated stack",
			action: "ListAssociatedStacks",
			setup: func(h *appstream.Handler) {
				createFleet(t, h, "fleet-3")
				createStack(t, h, "stack-3")
				doRequest(t, h, "AssociateFleet", map[string]any{"FleetName": "fleet-3", "StackName": "stack-3"})
			},
			body:     map[string]any{"FleetName": "fleet-3"},
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				names := resp["Names"].([]any)
				assert.Len(t, names, 1)
				assert.Equal(t, "stack-3", names[0])
			},
		},
		{
			name:   "DisassociateFleet removes link",
			action: "DisassociateFleet",
			setup: func(h *appstream.Handler) {
				createFleet(t, h, "fleet-4")
				createStack(t, h, "stack-4")
				doRequest(t, h, "AssociateFleet", map[string]any{"FleetName": "fleet-4", "StackName": "stack-4"})
			},
			body:     map[string]any{"FleetName": "fleet-4", "StackName": "stack-4"},
			wantCode: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			if tc.setup != nil {
				tc.setup(h)
			}
			rec := doRequest(t, h, tc.action, tc.body)
			assert.Equal(t, tc.wantCode, rec.Code)
			if tc.check != nil {
				tc.check(t, rec.Body.Bytes())
			}
		})
	}
}

// TestAppStream_FleetErrorCodes covers AWS-accuracy gaps in Fleet error
// __type values and state-transition semantics (as corrected by an audit
// against the real aws-sdk-go-v2 operation-scoped error deserializers, which
// only recognize a subset of exception shapes per operation):
//  1. ErrAlreadyExists __type: ResourceAlreadyExistsException (not
//     InvalidParameterCombinationException)
//  2. DeleteFleet on running fleet: ResourceInUseException (not
//     InvalidAccountStatusException, which DeleteFleet's deserializer does
//     not recognize)
//  3. StartFleet on already-RUNNING fleet → InvalidAccountStatusException
//     (StartFleet's deserializer does recognize this exception)
//  4. StopFleet on already-STOPPED fleet succeeds (idempotent; StopFleet's
//     deserializer has no state-conflict exception at all)
//  5. CreateFleet with invalid FleetType → InvalidParameterCombinationException
//  6. CreateFleet missing InstanceType → InvalidParameterCombinationException
func TestAppStream_FleetErrorCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(h *appstream.Handler)
		body     any
		name     string
		action   string
		wantType string
		wantCode int
	}{
		{
			name:   "CreateFleet duplicate returns ResourceAlreadyExistsException",
			action: "CreateFleet",
			setup: func(h *appstream.Handler) {
				createFleet(t, h, "dup-fleet")
			},
			body: map[string]any{
				"Name":         "dup-fleet",
				"InstanceType": "stream.standard.medium",
			},
			wantCode: http.StatusBadRequest,
			wantType: "ResourceAlreadyExistsException",
		},
		// The real DeleteFleet deserializer only recognizes
		// ConcurrentModificationException, ResourceInUseException, and
		// ResourceNotFoundException -- confirmed against
		// aws-sdk-go-v2/service/appstream deserializers.go.
		{
			name:   "DeleteFleet on running fleet returns ResourceInUseException",
			action: "DeleteFleet",
			setup: func(h *appstream.Handler) {
				createFleet(t, h, "running-fleet")
				rec := doRequest(t, h, "StartFleet", map[string]any{"Name": "running-fleet"})
				require.Equal(t, http.StatusOK, rec.Code)
			},
			body:     map[string]any{"Name": "running-fleet"},
			wantCode: http.StatusBadRequest,
			wantType: "ResourceInUseException",
		},
		{
			name:   "StartFleet on running fleet returns InvalidAccountStatusException",
			action: "StartFleet",
			setup: func(h *appstream.Handler) {
				createFleet(t, h, "running-fleet2")
				rec := doRequest(t, h, "StartFleet", map[string]any{"Name": "running-fleet2"})
				require.Equal(t, http.StatusOK, rec.Code)
			},
			body:     map[string]any{"Name": "running-fleet2"},
			wantCode: http.StatusBadRequest,
			wantType: "InvalidAccountStatusException",
		},
		// Real AWS's StopFleet deserializer only recognizes
		// ConcurrentModificationException and ResourceNotFoundException --
		// there is no state-conflict exception.
		{
			name:     "StopFleet on stopped fleet succeeds",
			action:   "StopFleet",
			setup:    func(h *appstream.Handler) { createFleet(t, h, "stopped-fleet") },
			body:     map[string]any{"Name": "stopped-fleet"},
			wantCode: http.StatusOK,
		},
		{
			name:   "CreateFleet with invalid FleetType returns InvalidParameterCombinationException",
			action: "CreateFleet",
			body: map[string]any{
				"Name":         "bad-type-fleet",
				"InstanceType": "stream.standard.medium",
				"FleetType":    "INVALID_TYPE",
			},
			wantCode: http.StatusBadRequest,
			wantType: "InvalidParameterCombinationException",
		},
		{
			name:   "CreateFleet with ALWAYS_ON FleetType succeeds",
			action: "CreateFleet",
			body: map[string]any{
				"Name":         "always-on-fleet",
				"InstanceType": "stream.standard.medium",
				"FleetType":    "ALWAYS_ON",
			},
			wantCode: http.StatusOK,
		},
		{
			name:   "CreateFleet with ELASTIC FleetType succeeds",
			action: "CreateFleet",
			body: map[string]any{
				"Name":         "elastic-fleet",
				"InstanceType": "stream.standard.medium",
				"FleetType":    "ELASTIC",
			},
			wantCode: http.StatusOK,
		},
		{
			name:   "CreateFleet missing InstanceType returns InvalidParameterCombinationException",
			action: "CreateFleet",
			body: map[string]any{
				"Name": "no-instance-fleet",
			},
			wantCode: http.StatusBadRequest,
			wantType: "InvalidParameterCombinationException",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			if tc.setup != nil {
				tc.setup(h)
			}

			rec := doRequest(t, h, tc.action, tc.body)
			assert.Equal(t, tc.wantCode, rec.Code)

			if tc.wantType != "" {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Equal(t, tc.wantType, resp["__type"], "wrong __type in error response")
			}
		})
	}
}

// TestAppStream_FleetComputeCapacityStatus verifies that Fleet responses include
// ComputeCapacityStatus with a Desired field — required by the real AWS API.
func TestAppStream_FleetComputeCapacityStatus(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "CreateFleet", map[string]any{
		"Name":         "cap-fleet",
		"InstanceType": "stream.standard.medium",
		"ComputeCapacity": map[string]any{
			"DesiredInstances": 2,
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	fleet := resp["Fleet"].(map[string]any)

	ccs, ok := fleet["ComputeCapacityStatus"].(map[string]any)
	require.True(t, ok, "ComputeCapacityStatus must be present in CreateFleet response")
	desired, ok := ccs["Desired"]
	require.True(t, ok, "ComputeCapacityStatus.Desired must be present")
	assert.EqualValues(t, 2, desired, "Desired should match input")

	// Also verify DescribeFleets returns it
	t.Run("DescribeFleets includes ComputeCapacityStatus", func(t *testing.T) {
		t.Parallel()

		h2 := newTestHandler(t)
		doRequest(t, h2, "CreateFleet", map[string]any{
			"Name":         "cap-fleet2",
			"InstanceType": "stream.standard.medium",
			"ComputeCapacity": map[string]any{
				"DesiredInstances": 3,
			},
		})

		recD := doRequest(t, h2, "DescribeFleets", map[string]any{"Names": []string{"cap-fleet2"}})
		require.Equal(t, http.StatusOK, recD.Code)

		var dr map[string]any
		require.NoError(t, json.Unmarshal(recD.Body.Bytes(), &dr))
		fleets := dr["Fleets"].([]any)
		require.Len(t, fleets, 1)
		f := fleets[0].(map[string]any)
		ccs2, ccOK := f["ComputeCapacityStatus"].(map[string]any)
		require.True(t, ccOK, "ComputeCapacityStatus must be present in DescribeFleets response")
		assert.EqualValues(t, 3, ccs2["Desired"])
	})
}

// TestAppStream_FleetComputeCapacityStatusDefault verifies that a fleet created
// without explicit ComputeCapacity still returns ComputeCapacityStatus with Desired=1.
func TestAppStream_FleetComputeCapacityStatusDefault(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "CreateFleet", map[string]any{
		"Name":         "default-cap-fleet",
		"InstanceType": "stream.standard.medium",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	fleet := resp["Fleet"].(map[string]any)

	ccs, ok := fleet["ComputeCapacityStatus"].(map[string]any)
	require.True(t, ok, "ComputeCapacityStatus must be present even without explicit ComputeCapacity input")
	desired, ok := ccs["Desired"]
	require.True(t, ok, "ComputeCapacityStatus.Desired must be present")
	assert.EqualValues(t, 1, desired, "default Desired should be 1")
}

// TestAppStream_FleetImageNameRoundtrip verifies that ImageName set on CreateFleet
// is returned in the Fleet response.
func TestAppStream_FleetImageNameRoundtrip(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "CreateFleet", map[string]any{
		"Name":         "img-fleet",
		"InstanceType": "stream.standard.medium",
		"ImageName":    "AppStream-WinServer2019-05-01-2023",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	fleet := resp["Fleet"].(map[string]any)
	assert.Equal(t, "AppStream-WinServer2019-05-01-2023", fleet["ImageName"])
}

// TestAppStream_FleetImageArnRoundtrip verifies that ImageArn set on CreateFleet
// is returned in the Fleet response.
func TestAppStream_FleetImageArnRoundtrip(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	imageArn := "arn:aws:appstream:us-east-1:123456789012:image/AppStream-WinServer2019"
	rec := doRequest(t, h, "CreateFleet", map[string]any{
		"Name":         "imgarn-fleet",
		"InstanceType": "stream.standard.medium",
		"ImageArn":     imageArn,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	fleet := resp["Fleet"].(map[string]any)
	assert.Equal(t, imageArn, fleet["ImageArn"])
}

// TestAppStream_FleetIdleDisconnectTimeout verifies that IdleDisconnectTimeoutInSeconds
// is accepted as input and returned in Fleet responses.
func TestAppStream_FleetIdleDisconnectTimeout(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "CreateFleet", map[string]any{
		"Name":                           "idle-fleet",
		"InstanceType":                   "stream.standard.medium",
		"IdleDisconnectTimeoutInSeconds": 600,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	fleet := resp["Fleet"].(map[string]any)
	assert.EqualValues(t, 600, fleet["IdleDisconnectTimeoutInSeconds"])
}

// TestAppStream_FleetEnableDefaultInternetAccess verifies that
// EnableDefaultInternetAccess is accepted and returned.
func TestAppStream_FleetEnableDefaultInternetAccess(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "CreateFleet", map[string]any{
		"Name":                        "inet-fleet",
		"InstanceType":                "stream.standard.medium",
		"EnableDefaultInternetAccess": true,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	fleet := resp["Fleet"].(map[string]any)
	assert.Equal(t, true, fleet["EnableDefaultInternetAccess"])
}

// TestAppStream_FleetUpdateImageName verifies that UpdateFleet can change ImageName.
func TestAppStream_FleetUpdateImageName(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateFleet", map[string]any{
		"Name":         "upd-img-fleet",
		"InstanceType": "stream.standard.medium",
		"ImageName":    "old-image",
	})

	rec := doRequest(t, h, "UpdateFleet", map[string]any{
		"Name":      "upd-img-fleet",
		"ImageName": "new-image",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	fleet := resp["Fleet"].(map[string]any)
	assert.Equal(t, "new-image", fleet["ImageName"])
}

// TestAppStream_FleetUpdateComputeCapacity verifies that UpdateFleet can change
// DesiredInstances via ComputeCapacity.
func TestAppStream_FleetUpdateComputeCapacity(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateFleet", map[string]any{
		"Name":            "upd-cap-fleet",
		"InstanceType":    "stream.standard.medium",
		"ComputeCapacity": map[string]any{"DesiredInstances": 1},
	})

	rec := doRequest(t, h, "UpdateFleet", map[string]any{
		"Name":            "upd-cap-fleet",
		"ComputeCapacity": map[string]any{"DesiredInstances": 5},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	fleet := resp["Fleet"].(map[string]any)
	ccs := fleet["ComputeCapacityStatus"].(map[string]any)
	assert.EqualValues(t, 5, ccs["Desired"])
}

// TestAppStream_FleetARNFormat verifies that fleet ARNs match the AWS format
// arn:aws:appstream:<region>:<account>:fleet/<name>.
func TestAppStream_FleetARNFormat(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "CreateFleet", map[string]any{
		"Name":         "arn-fleet",
		"InstanceType": "stream.standard.medium",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	fleet := resp["Fleet"].(map[string]any)
	assert.Regexp(t, `^arn:aws:appstream:[a-z0-9-]+:\d+:fleet/arn-fleet$`, fleet["Arn"])
}

// TestAppStream_FleetDefaultsMatchAWS verifies that fleet defaults match AWS:
// - FleetType defaults to ON_DEMAND
// - MaxUserDurationInSeconds defaults to 57600 (16h)
// - DisconnectTimeoutInSeconds defaults to 300 (5min).
func TestAppStream_FleetDefaultsMatchAWS(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "CreateFleet", map[string]any{
		"Name":         "defaults-fleet",
		"InstanceType": "stream.standard.medium",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	fleet := resp["Fleet"].(map[string]any)
	assert.Equal(t, "ON_DEMAND", fleet["FleetType"])
	assert.EqualValues(t, 57600, fleet["MaxUserDurationInSeconds"])
	assert.EqualValues(t, 300, fleet["DisconnectTimeoutInSeconds"])
}

// TestAppStream_FleetStartedStateRunning verifies that StartFleet changes state to RUNNING.
func TestAppStream_FleetStartedStateRunning(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateFleet", map[string]any{
		"Name":         "state-fleet",
		"InstanceType": "stream.standard.medium",
	})
	doRequest(t, h, "StartFleet", map[string]any{"Name": "state-fleet"})

	rec := doRequest(t, h, "DescribeFleets", map[string]any{"Names": []string{"state-fleet"}})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	fleet := resp["Fleets"].([]any)[0].(map[string]any)
	assert.Equal(t, "RUNNING", fleet["State"])
}

// TestAppStream_DeleteFleet_ClearsAssociations proves DeleteFleet drops the
// deleted fleet's own association-map entry rather than leaving it behind
// keyed by the (reusable) fleet name. Regression for gopherstack-65w:
// DeleteFleet previously iterated every fleet's stack-association map trying
// to delete an entry keyed by the deleted *fleet's* name -- a no-op, since
// those inner maps are keyed by stack name -- so b.associations[name] itself
// was never removed. A fleet re-created with the same name then inherited
// the stale associations of the fleet it replaced.
func TestAppStream_DeleteFleet_ClearsAssociations(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createFleet(t, h, "reused-fleet")
	createStack(t, h, "ghost-stack")
	doRequest(t, h, "AssociateFleet", map[string]any{"FleetName": "reused-fleet", "StackName": "ghost-stack"})

	rec := doRequest(t, h, "DeleteFleet", map[string]any{"Name": "reused-fleet"})
	require.Equal(t, http.StatusOK, rec.Code)

	createFleet(t, h, "reused-fleet")

	rec = doRequest(t, h, "ListAssociatedStacks", map[string]any{"FleetName": "reused-fleet"})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Empty(t, resp["Names"], "re-created fleet must not inherit the deleted fleet's stack associations")
}

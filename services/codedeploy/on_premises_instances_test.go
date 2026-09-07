package codedeploy_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/codedeploy"
)

func TestHandler_AddTagsToOnPremisesInstances(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input      map[string]any
		name       string
		wantStatus int
	}{
		{
			name: "success",
			input: map[string]any{
				"instanceNames": []string{"instance-1", "instance-2"},
				"tags": []map[string]string{
					{"Key": "env", "Value": "prod"},
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing_instance_names",
			input:      map[string]any{},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, "AddTagsToOnPremisesInstances", tt.input)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestHandler_BatchGetOnPremisesInstances(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(h *codedeploy.Handler)
		input      map[string]any
		name       string
		wantStatus int
		wantCount  int
	}{
		{
			name: "success",
			setup: func(h *codedeploy.Handler) {
				kv := map[string]string{"env": "test"}
				err := h.Backend.AddTagsToOnPremisesInstances([]string{"inst-1", "inst-2"}, kv)
				require.NoError(t, err)
			},
			input:      map[string]any{"instanceNames": []string{"inst-1", "inst-2", "missing"}},
			wantStatus: http.StatusOK,
			wantCount:  2,
		},
		{
			name:       "missing_names",
			input:      map[string]any{},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			if tt.setup != nil {
				tt.setup(h)
			}

			rec := doRequest(t, h, "BatchGetOnPremisesInstances", tt.input)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				infos, ok := resp["instanceInfos"].([]any)
				require.True(t, ok)
				assert.Len(t, infos, tt.wantCount)
			}
		})
	}
}

func TestOnPremisesInstances_TagsAlwaysSlice(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Add instance with no tags
	h.Backend.AddOnPremisesInstanceInternal(&codedeploy.OnPremisesInstance{
		InstanceName: "notag-server",
	})

	rec := doRequest(t, h, "BatchGetOnPremisesInstances", map[string]any{
		"instanceNames": []string{"notag-server"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		InstanceInfos []struct {
			Tags []map[string]string `json:"tags"`
		} `json:"instanceInfos"`
	}

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.InstanceInfos, 1)
	// Tags should be an empty slice, not nil/missing
	assert.NotNil(t, resp.InstanceInfos[0].Tags)
	assert.Empty(t, resp.InstanceInfos[0].Tags)
}

func TestHandler_OnPremisesInstances(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// RegisterOnPremisesInstance
	rec := doRequest(t, h, "RegisterOnPremisesInstance", map[string]any{
		"instanceName": "my-instance",
		"iamUserArn":   "arn:aws:iam::123456789012:user/deploy-user",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// GetOnPremisesInstance
	rec = doRequest(t, h, "GetOnPremisesInstance", map[string]any{
		"instanceName": "my-instance",
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	// ListOnPremisesInstances
	rec = doRequest(t, h, "ListOnPremisesInstances", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	// RemoveTagsFromOnPremisesInstances
	rec = doRequest(t, h, "RemoveTagsFromOnPremisesInstances", map[string]any{
		"instanceNames": []string{"my-instance"},
		"tags":          []map[string]any{{"Key": "env", "Value": "test"}},
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	// DeregisterOnPremisesInstance
	rec = doRequest(t, h, "DeregisterOnPremisesInstance", map[string]any{
		"instanceName": "my-instance",
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestOnPremisesInstances_IamValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		instanceName  string
		iamSessionArn string
		iamUserArn    string
		wantErrType   string
		wantStatus    int
	}{
		{
			name:          "session_arn_only",
			instanceName:  "server-01",
			iamSessionArn: "arn:aws:sts::123:assumed-role/role/session",
			wantStatus:    http.StatusOK,
		},
		{
			name:         "user_arn_only",
			instanceName: "server-02",
			iamUserArn:   "arn:aws:iam::123:user/user1",
			wantStatus:   http.StatusOK,
		},
		{
			name:          "both_arns_set",
			instanceName:  "server-03",
			iamSessionArn: "arn:aws:sts::123:assumed-role/role/session",
			iamUserArn:    "arn:aws:iam::123:user/user1",
			wantStatus:    http.StatusBadRequest,
			wantErrType:   "MultipleIamArnsProvidedException",
		},
		{
			name:         "no_arns_set",
			instanceName: "server-04",
			wantStatus:   http.StatusBadRequest,
			wantErrType:  "IamArnRequiredException",
		},
		{
			name:         "invalid_name_has_spaces",
			instanceName: "server 05",
			iamUserArn:   "arn:aws:iam::123:user/user1",
			wantStatus:   http.StatusBadRequest,
			wantErrType:  "InvalidInstanceNameException",
		},
		{
			name:         "name_too_long",
			instanceName: strings.Repeat("x", 101),
			iamUserArn:   "arn:aws:iam::123:user/user1",
			wantStatus:   http.StatusBadRequest,
			wantErrType:  "InvalidInstanceNameException",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, "RegisterOnPremisesInstance", map[string]any{
				"instanceName":  tt.instanceName,
				"iamSessionArn": tt.iamSessionArn,
				"iamUserArn":    tt.iamUserArn,
			})
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantErrType != "" {
				var resp map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Equal(t, tt.wantErrType, resp["__type"])
			}
		})
	}
}

func TestOnPremisesInstances_ListTagFilter(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Register instances.
	require.NoError(t, h.Backend.RegisterOnPremisesInstance("server-a", "arn:aws:sts::123:assumed-role/r/s", ""))
	require.NoError(t, h.Backend.RegisterOnPremisesInstance("server-b", "arn:aws:sts::123:assumed-role/r/s2", ""))
	require.NoError(t, h.Backend.AddTagsToOnPremisesInstances([]string{"server-a"}, map[string]string{"tier": "web"}))
	require.NoError(t, h.Backend.AddTagsToOnPremisesInstances([]string{"server-b"}, map[string]string{"tier": "db"}))

	tests := []struct {
		name       string
		tagFilters []map[string]string
		wantNames  []string
	}{
		{
			name:       "no_filter",
			tagFilters: nil,
			wantNames:  []string{"server-a", "server-b"},
		},
		{
			name:       "equals_web",
			tagFilters: []map[string]string{{"Key": "tier", "Value": "web", "Type": "EQUALS"}},
			wantNames:  []string{"server-a"},
		},
		{
			name:       "key_only",
			tagFilters: []map[string]string{{"Key": "tier", "Type": "KEY_ONLY"}},
			wantNames:  []string{"server-a", "server-b"},
		},
		{
			name:       "value_only_db",
			tagFilters: []map[string]string{{"Value": "db", "Type": "VALUE_ONLY"}},
			wantNames:  []string{"server-b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			input := map[string]any{}
			if tt.tagFilters != nil {
				input["tagFilters"] = tt.tagFilters
			}

			rec := doRequest(t, h, "ListOnPremisesInstances", input)
			require.Equal(t, http.StatusOK, rec.Code)

			var resp struct {
				InstanceNames []string `json:"instanceNames"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.ElementsMatch(t, tt.wantNames, resp.InstanceNames)
		})
	}
}

// TestOnPremisesInstances_NotFoundErrorMapping verifies that a not-found
// on-premises instance lookup surfaces as a 404, not a fallback 500
// ServiceException, and that each op maps to its own SDK-declared code:
// GetOnPremisesInstance's deserializer models InstanceNotRegisteredException,
// not InstanceDoesNotExistException (aws-sdk-go-v2/service/codedeploy
// deserializers.go, gopherstack-3pz8). DeregisterOnPremisesInstance's
// deserializer models neither code -- InstanceDoesNotExistException is a
// known-wrong landmine there (see on_premises_instances.go), kept only
// because no confirmed remedy exists yet.
func TestOnPremisesInstances_NotFoundErrorMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    map[string]any
		name     string
		action   string
		wantType string
	}{
		{
			name:     "GetOnPremisesInstance",
			action:   "GetOnPremisesInstance",
			input:    map[string]any{"instanceName": "no-such-instance"},
			wantType: "InstanceNotRegisteredException",
		},
		{
			name:     "DeregisterOnPremisesInstance",
			action:   "DeregisterOnPremisesInstance",
			input:    map[string]any{"instanceName": "no-such-instance"},
			wantType: "InstanceDoesNotExistException",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, tt.action, tt.input)
			require.Equal(t, http.StatusNotFound, rec.Code)

			var resp map[string]string
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, tt.wantType, resp["__type"])
		})
	}
}

// TestGetOnPremisesInstance_NotRegisteredNotDoesNotExist locks in
// GetOnPremisesInstance's fix: the wrong pre-gopherstack-3pz8 code must not
// come back, and no instance is created as a side effect of a failed lookup.
func TestGetOnPremisesInstance_NotRegisteredNotDoesNotExist(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "GetOnPremisesInstance", map[string]any{"instanceName": "no-such-instance"})
	require.Equal(t, http.StatusNotFound, rec.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "InstanceNotRegisteredException", resp["__type"])
	assert.NotEqual(t, "InstanceDoesNotExistException", resp["__type"])

	listRec := doRequest(t, h, "ListOnPremisesInstances", map[string]any{})
	require.Equal(t, http.StatusOK, listRec.Code)

	var listResp struct {
		InstanceNames []string `json:"instanceNames"`
	}
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listResp))
	assert.Empty(t, listResp.InstanceNames)
}

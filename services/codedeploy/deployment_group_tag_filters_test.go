package codedeploy_test

import (
	"encoding/json"
	"maps"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ec2TagSetPayload() map[string]any {
	return map[string]any{
		"ec2TagSetList": []any{
			[]map[string]string{{"Key": "env", "Value": "prod"}},
		},
	}
}

func onPremTagSetPayload() map[string]any {
	return map[string]any{
		"onPremisesTagSetList": []any{
			[]map[string]string{{"Key": "env", "Value": "prod"}},
		},
	}
}

func TestDeploymentGroups_TagFilterValidation_Create(t *testing.T) {
	t.Parallel()

	tests := []struct {
		extra      map[string]any
		name       string
		wantType   string
		wantStatus int
	}{
		{
			name: "ec2_filters_and_set_rejected",
			extra: map[string]any{
				"ec2TagFilters": []map[string]string{{"Key": "env", "Value": "prod", "Type": "KEY_AND_VALUE"}},
				"ec2TagSet":     ec2TagSetPayload(),
			},
			wantStatus: http.StatusBadRequest,
			wantType:   "InvalidEC2TagCombinationException",
		},
		{
			name: "onprem_filters_and_set_rejected",
			extra: map[string]any{
				"onPremisesInstanceTagFilters": []map[string]string{
					{"Key": "env", "Value": "prod", "Type": "KEY_AND_VALUE"},
				},
				"onPremisesTagSet": onPremTagSetPayload(),
			},
			wantStatus: http.StatusBadRequest,
			wantType:   "InvalidOnPremisesTagCombinationException",
		},
		{
			name: "invalid_ec2_tag_type_rejected",
			extra: map[string]any{
				"ec2TagFilters": []map[string]string{{"Key": "env", "Value": "prod", "Type": "EQUALS"}},
			},
			wantStatus: http.StatusBadRequest,
			wantType:   "InvalidEC2TagException",
		},
		{
			name: "ec2_filters_only_legal",
			extra: map[string]any{
				"ec2TagFilters": []map[string]string{{"Key": "env", "Value": "prod", "Type": "KEY_AND_VALUE"}},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "ec2_set_only_legal",
			extra: map[string]any{
				"ec2TagSet": ec2TagSetPayload(),
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "onprem_filters_only_legal",
			extra: map[string]any{
				"onPremisesInstanceTagFilters": []map[string]string{{"Key": "env", "Value": "prod"}},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "onprem_set_only_legal",
			extra: map[string]any{
				"onPremisesTagSet": onPremTagSetPayload(),
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "neither_set_legal",
			extra:      map[string]any{},
			wantStatus: http.StatusOK,
		},
		{
			name: "empty_ec2_tag_type_legal",
			extra: map[string]any{
				"ec2TagFilters": []map[string]string{{"Key": "env", "Value": "prod"}},
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			createCDApp(t, h, "app")

			input := map[string]any{
				"applicationName":     "app",
				"deploymentGroupName": "dg",
				"serviceRoleArn":      "arn:aws:iam::123:role/role",
			}
			maps.Copy(input, tt.extra)

			rec := doRequest(t, h, "CreateDeploymentGroup", input)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantType != "" {
				var resp map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Equal(t, tt.wantType, resp["__type"])
			}
		})
	}
}

func TestDeploymentGroups_TagFilterValidation_Update(t *testing.T) {
	t.Parallel()

	tests := []struct {
		extra      map[string]any
		name       string
		wantType   string
		wantStatus int
	}{
		{
			name: "ec2_filters_and_set_rejected",
			extra: map[string]any{
				"ec2TagFilters": []map[string]string{{"Key": "env", "Value": "prod", "Type": "KEY_AND_VALUE"}},
				"ec2TagSet":     ec2TagSetPayload(),
			},
			wantStatus: http.StatusBadRequest,
			wantType:   "InvalidEC2TagCombinationException",
		},
		{
			name: "onprem_filters_and_set_rejected",
			extra: map[string]any{
				"onPremisesInstanceTagFilters": []map[string]string{
					{"Key": "env", "Value": "prod", "Type": "KEY_AND_VALUE"},
				},
				"onPremisesTagSet": onPremTagSetPayload(),
			},
			wantStatus: http.StatusBadRequest,
			wantType:   "InvalidOnPremisesTagCombinationException",
		},
		{
			name: "invalid_ec2_tag_type_rejected",
			extra: map[string]any{
				"ec2TagFilters": []map[string]string{{"Key": "env", "Value": "prod", "Type": "EQUALS"}},
			},
			wantStatus: http.StatusBadRequest,
			wantType:   "InvalidEC2TagException",
		},
		{
			name: "ec2_filters_only_legal",
			extra: map[string]any{
				"ec2TagFilters": []map[string]string{{"Key": "env", "Value": "prod", "Type": "KEY_AND_VALUE"}},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "ec2_set_only_legal",
			extra: map[string]any{
				"ec2TagSet": ec2TagSetPayload(),
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "neither_set_legal",
			extra:      map[string]any{},
			wantStatus: http.StatusOK,
		},
		{
			name: "empty_ec2_tag_type_legal",
			extra: map[string]any{
				"ec2TagFilters": []map[string]string{{"Key": "env", "Value": "prod"}},
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			createCDApp(t, h, "app")
			doRequest(t, h, "CreateDeploymentGroup", map[string]any{
				"applicationName":     "app",
				"deploymentGroupName": "dg",
				"serviceRoleArn":      "arn:aws:iam::123:role/role",
			})

			input := map[string]any{
				"applicationName":            "app",
				"currentDeploymentGroupName": "dg",
			}
			maps.Copy(input, tt.extra)

			rec := doRequest(t, h, "UpdateDeploymentGroup", input)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantType != "" {
				var resp map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Equal(t, tt.wantType, resp["__type"])
			}
		})
	}
}

package resourcegroups_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/resourcegroups"
)

func TestResourceGroupsHandler_GroupResources(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(t *testing.T, h *resourcegroups.Handler)
		name         string
		group        string
		resourceARNs []string
		wantContains []string
		wantCode     int
	}{
		{
			name:  "success",
			group: "my-group",
			setup: func(t *testing.T, h *resourcegroups.Handler) {
				t.Helper()
				doResourceGroupsRequest(t, h, "CreateGroup", map[string]any{"Name": "my-group"})
			},
			resourceARNs: []string{"arn:aws:ec2:us-east-1:000000000000:instance/i-12345"},
			wantCode:     http.StatusOK,
			wantContains: []string{"Succeeded"},
		},
		{
			name:     "missing_group",
			group:    "",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "not_found",
			group:    "nonexistent",
			wantCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestResourceGroupsHandler(t)
			if tt.setup != nil {
				tt.setup(t, h)
			}

			body := map[string]any{
				"Group":        tt.group,
				"ResourceArns": tt.resourceARNs,
			}
			rec := doResourceGroupsRequest(t, h, "GroupResources", body)
			assert.Equal(t, tt.wantCode, rec.Code)
			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

// TestGroupResources_NotFound verifies 404 for unknown group.
func TestGroupResources_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestResourceGroupsHandler(t)
	rec := doResourceGroupsRequest(t, h, "GroupResources", map[string]any{
		"Group":        "nonexistent",
		"ResourceArns": []string{"arn:aws:s3:::b1"},
	})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestGroupResources_InvalidARNReturnsFailed verifies that malformed ARNs are
// returned in the Failed list with INVALID_ARN error code, not added to the
// group. Real AWS Resource Groups rejects non-ARN strings with INVALID_ARN.
func TestGroupResources_InvalidARNReturnsFailed(t *testing.T) {
	t.Parallel()

	tests := []struct { //nolint:govet // fieldalignment: readability over micro-optimization
		name          string
		arns          []string
		wantSucceeded int
		wantFailed    int
		wantErrCode   string
	}{
		{
			name:          "valid_arns_only",
			arns:          []string{"arn:aws:s3:::my-bucket", "arn:aws:ec2:us-east-1:123456789012:instance/i-1"},
			wantSucceeded: 2,
			wantFailed:    0,
		},
		{
			name:          "not_an_arn",
			arns:          []string{"just-a-string"},
			wantSucceeded: 0,
			wantFailed:    1,
			wantErrCode:   "INVALID_ARN",
		},
		{
			name:          "arn_too_few_segments",
			arns:          []string{"arn:aws:s3"},
			wantSucceeded: 0,
			wantFailed:    1,
			wantErrCode:   "INVALID_ARN",
		},
		{
			name: "mixed_valid_and_invalid",
			arns: []string{
				"arn:aws:s3:::bucket1",
				"not-an-arn",
				"arn:aws:lambda:us-east-1:123456789012:function/fn",
				"",
			},
			wantSucceeded: 2,
			wantFailed:    2,
			wantErrCode:   "INVALID_ARN",
		},
		{
			name:          "empty_string_arn",
			arns:          []string{""},
			wantSucceeded: 0,
			wantFailed:    1,
			wantErrCode:   "INVALID_ARN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestResourceGroupsHandler(t)
			doResourceGroupsRequest(t, h, "CreateGroup", map[string]any{"Name": "parity-grp"})

			rec := doResourceGroupsRequest(t, h, "GroupResources", map[string]any{
				"Group":        "parity-grp",
				"ResourceArns": tt.arns,
			})
			require.Equal(t, http.StatusOK, rec.Code)

			var out struct {
				Succeeded []string `json:"Succeeded"`
				Failed    []struct {
					ResourceArn  string `json:"ResourceArn"`
					ErrorCode    string `json:"ErrorCode"`
					ErrorMessage string `json:"ErrorMessage"`
				} `json:"Failed"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

			assert.Len(
				t,
				out.Succeeded,
				tt.wantSucceeded,
				"succeeded count",
			)
			assert.Len(t, out.Failed, tt.wantFailed, "failed count: %s", rec.Body.String())

			if tt.wantErrCode != "" && len(out.Failed) > 0 {
				assert.Equal(t, tt.wantErrCode, out.Failed[0].ErrorCode)
				assert.NotEmpty(t, out.Failed[0].ErrorMessage)
			}
		})
	}
}

// TestGroupResources_InvalidARNNotAddedToGroup verifies that invalid ARNs
// rejected with INVALID_ARN are not persisted in the group resource list.
func TestGroupResources_InvalidARNNotAddedToGroup(t *testing.T) {
	t.Parallel()

	h := newTestResourceGroupsHandler(t)
	doResourceGroupsRequest(t, h, "CreateGroup", map[string]any{"Name": "isolation-grp"})

	doResourceGroupsRequest(t, h, "GroupResources", map[string]any{
		"Group":        "isolation-grp",
		"ResourceArns": []string{"not-an-arn", "arn:aws:s3:::real-bucket"},
	})

	rec := doResourceGroupsRequest(t, h, "ListGroupResources", map[string]any{"Group": "isolation-grp"})
	require.Equal(t, http.StatusOK, rec.Code)

	var out struct {
		Resources []struct {
			Identifier struct {
				ResourceArn string `json:"ResourceArn"`
			} `json:"Identifier"`
		} `json:"Resources"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	require.Len(t, out.Resources, 1, "only the valid ARN should be in group")
	assert.Equal(t, "arn:aws:s3:::real-bucket", out.Resources[0].Identifier.ResourceArn)
}

// TestGroupResources_OutputShape verifies the GroupResources response
// structure matches AWS: Failed entries have ResourceArn, ErrorCode,
// ErrorMessage fields.
func TestGroupResources_OutputShape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		wantInBody  string
		wantOutBody string
	}{
		{
			name:       "failed_has_error_code_field",
			wantInBody: "ErrorCode",
		},
		{
			name:       "failed_has_resource_arn_field",
			wantInBody: "ResourceArn",
		},
		{
			name:       "failed_has_error_message_field",
			wantInBody: "ErrorMessage",
		},
		{
			name:        "succeeded_is_list",
			wantOutBody: "Pending",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestResourceGroupsHandler(t)
			doResourceGroupsRequest(t, h, "CreateGroup", map[string]any{"Name": "shape-grp"})

			rec := doResourceGroupsRequest(t, h, "GroupResources", map[string]any{
				"Group":        "shape-grp",
				"ResourceArns": []string{"bad-arn"},
			})
			require.Equal(t, http.StatusOK, rec.Code)
			body := rec.Body.String()

			if tt.wantInBody != "" {
				assert.Contains(t, body, tt.wantInBody)
			}

			if tt.wantOutBody != "" {
				assert.NotContains(t, body, tt.wantOutBody+"\":null")
			}
		})
	}
}

// TestUngroupResourcesFailures covers that UngroupResources must return
// failures for ARNs that are not currently in the group.
func TestUngroupResourcesFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		grouped       []string
		ungroup       []string
		wantSucceeded []string
		wantFailCount int
	}{
		{
			name:          "all_members",
			grouped:       []string{"arn:aws:s3:::b1", "arn:aws:s3:::b2"},
			ungroup:       []string{"arn:aws:s3:::b1", "arn:aws:s3:::b2"},
			wantSucceeded: []string{"arn:aws:s3:::b1", "arn:aws:s3:::b2"},
			wantFailCount: 0,
		},
		{
			name:          "one_non_member",
			grouped:       []string{"arn:aws:s3:::b1"},
			ungroup:       []string{"arn:aws:s3:::b1", "arn:aws:s3:::nonexistent"},
			wantSucceeded: []string{"arn:aws:s3:::b1"},
			wantFailCount: 1,
		},
		{
			name:          "all_non_members",
			grouped:       []string{},
			ungroup:       []string{"arn:aws:s3:::b1", "arn:aws:s3:::b2"},
			wantSucceeded: []string{},
			wantFailCount: 2,
		},
		{
			name:          "empty_input",
			grouped:       []string{"arn:aws:s3:::b1"},
			ungroup:       []string{},
			wantSucceeded: []string{},
			wantFailCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestResourceGroupsHandler(t)
			doResourceGroupsRequest(t, h, "CreateGroup", map[string]any{"Name": "ug-group"})

			if len(tt.grouped) > 0 {
				doResourceGroupsRequest(t, h, "GroupResources", map[string]any{
					"Group":        "ug-group",
					"ResourceArns": tt.grouped,
				})
			}

			rec := doResourceGroupsRequest(t, h, "UngroupResources", map[string]any{
				"Group":        "ug-group",
				"ResourceArns": tt.ungroup,
			})
			require.Equal(t, http.StatusOK, rec.Code)

			var out map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

			succeeded, _ := out["Succeeded"].([]any)
			failed, _ := out["Failed"].([]any)

			assert.Len(
				t,
				succeeded,
				len(tt.wantSucceeded),
				"succeeded count mismatch: %s",
				rec.Body.String(),
			)
			assert.Len(t, failed, tt.wantFailCount, "failed count mismatch: %s", rec.Body.String())

			for _, s := range tt.wantSucceeded {
				found := false
				for _, item := range succeeded {
					if item.(string) == s {
						found = true
					}
				}
				assert.True(t, found, "expected %s in Succeeded", s)
			}
		})
	}
}

func TestResourceGroupsHandler_ListGroupResources(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(t *testing.T, h *resourcegroups.Handler)
		name         string
		group        string
		wantContains []string
		wantCode     int
	}{
		{
			name:  "success_empty",
			group: "my-group",
			setup: func(t *testing.T, h *resourcegroups.Handler) {
				t.Helper()
				doResourceGroupsRequest(t, h, "CreateGroup", map[string]any{"Name": "my-group"})
			},
			wantCode:     http.StatusOK,
			wantContains: []string{"Resources"},
		},
		{
			name:  "success_with_resources",
			group: "my-group",
			setup: func(t *testing.T, h *resourcegroups.Handler) {
				t.Helper()
				doResourceGroupsRequest(t, h, "CreateGroup", map[string]any{"Name": "my-group"})
				doResourceGroupsRequest(t, h, "GroupResources", map[string]any{
					"Group":        "my-group",
					"ResourceArns": []string{"arn:aws:ec2:us-east-1:000000000000:instance/i-123"},
				})
			},
			wantCode:     http.StatusOK,
			wantContains: []string{"Resources", "i-123"},
		},
		{
			name:     "not_found",
			group:    "nonexistent",
			wantCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestResourceGroupsHandler(t)
			if tt.setup != nil {
				tt.setup(t, h)
			}

			rec := doResourceGroupsRequest(t, h, "ListGroupResources", map[string]any{"Group": tt.group})
			assert.Equal(t, tt.wantCode, rec.Code)
			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

// TestListGroupResources_Empty verifies empty list for group with no resources.
func TestListGroupResources_Empty(t *testing.T) {
	t.Parallel()

	h := newTestResourceGroupsHandler(t)
	doResourceGroupsRequest(t, h, "CreateGroup", map[string]any{"Name": "empty-group"})

	rec := doResourceGroupsRequest(t, h, "ListGroupResources", map[string]any{
		"Group": "empty-group",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Resources")
}

// TestListGroupResources_NotFound verifies 404 for unknown group.
func TestListGroupResources_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestResourceGroupsHandler(t)
	rec := doResourceGroupsRequest(t, h, "ListGroupResources", map[string]any{
		"Group": "ghost",
	})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestListGroupResources_ResourceTypeInResponse verifies ResourceType field in HTTP response.
func TestListGroupResources_ResourceTypeInResponse(t *testing.T) {
	t.Parallel()

	h := newTestResourceGroupsHandler(t)
	doResourceGroupsRequest(t, h, "CreateGroup", map[string]any{"Name": "typed-group"})
	doResourceGroupsRequest(t, h, "GroupResources", map[string]any{
		"Group":        "typed-group",
		"ResourceArns": []string{"arn:aws:ec2:us-east-1:000000000000:instance/i-abc"},
	})

	rec := doResourceGroupsRequest(t, h, "ListGroupResources", map[string]any{
		"Group": "typed-group",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "AWS::EC2::Instance")
	assert.Contains(t, rec.Body.String(), "ResourceType")
}

// TestListGroupResources_ResourceIdentifiersDeprecatedField verifies that
// the deprecated ResourceIdentifiers field ("don't use this parameter, use
// the Resources response field instead" per the real API docs) is populated
// identically to Resources for backward-compatible clients, and that
// QueryErrors (only ever non-empty for CLOUDFORMATION_STACK_1_0-based
// groups, which this emulator does not model) is omitted when empty.
func TestListGroupResources_ResourceIdentifiersDeprecatedField(t *testing.T) {
	t.Parallel()

	h := newTestResourceGroupsHandler(t)
	doResourceGroupsRequest(t, h, "CreateGroup", map[string]any{"Name": "dep-group"})
	doResourceGroupsRequest(t, h, "GroupResources", map[string]any{
		"Group":        "dep-group",
		"ResourceArns": []string{"arn:aws:ec2:us-east-1:000000000000:instance/i-abc"},
	})

	rec := doResourceGroupsRequest(t, h, "ListGroupResources", map[string]any{
		"Group": "dep-group",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var out struct {
		Resources []struct {
			Identifier struct {
				ResourceArn  string `json:"ResourceArn"`
				ResourceType string `json:"ResourceType"`
			} `json:"Identifier"`
		} `json:"Resources"`
		ResourceIdentifiers []struct {
			ResourceArn  string `json:"ResourceArn"`
			ResourceType string `json:"ResourceType"`
		} `json:"ResourceIdentifiers"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	require.Len(t, out.ResourceIdentifiers, 1)
	assert.Equal(t, out.Resources[0].Identifier.ResourceArn, out.ResourceIdentifiers[0].ResourceArn)
	assert.Equal(t, out.Resources[0].Identifier.ResourceType, out.ResourceIdentifiers[0].ResourceType)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	assert.NotContains(t, raw, "QueryErrors", "QueryErrors must be omitted when empty")
}

// TestListGroupResources_FilterViaHandler verifies resource-type filter through HTTP.
func TestListGroupResources_FilterViaHandler(t *testing.T) {
	t.Parallel()

	h := newTestResourceGroupsHandler(t)
	doResourceGroupsRequest(t, h, "CreateGroup", map[string]any{"Name": "filtered-group"})
	doResourceGroupsRequest(t, h, "GroupResources", map[string]any{
		"Group": "filtered-group",
		"ResourceArns": []string{
			"arn:aws:s3:::my-bucket",
			"arn:aws:ec2:us-east-1:000000000000:instance/i-abc",
			"arn:aws:lambda:us-east-1:000000000000:function:my-fn",
		},
	})

	rec := doResourceGroupsRequest(t, h, "ListGroupResources", map[string]any{
		"Group": "filtered-group",
		"Filters": []map[string]any{
			{"Name": "resource-type", "Values": []string{"AWS::EC2::Instance"}},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, "AWS::EC2::Instance")
	assert.NotContains(t, body, "AWS::S3::Bucket")
	assert.NotContains(t, body, "AWS::Lambda::Function")
}

// TestListGroupResources_PaginationViaHandler verifies handler NextToken.
func TestListGroupResources_PaginationViaHandler(t *testing.T) {
	t.Parallel()

	h := newTestResourceGroupsHandler(t)
	doResourceGroupsRequest(t, h, "CreateGroup", map[string]any{"Name": "paged-group"})

	arns := make([]string, 5)
	for i := range arns {
		arns[i] = fmt.Sprintf("arn:aws:s3:::bucket-%d", i)
	}
	doResourceGroupsRequest(t, h, "GroupResources", map[string]any{
		"Group":        "paged-group",
		"ResourceArns": arns,
	})

	rec1 := doResourceGroupsRequest(t, h, "ListGroupResources", map[string]any{
		"Group":      "paged-group",
		"MaxResults": 2,
	})
	require.Equal(t, http.StatusOK, rec1.Code)

	var out1 map[string]any
	require.NoError(t, json.Unmarshal(rec1.Body.Bytes(), &out1))
	resources1 := out1["Resources"].([]any)
	assert.Len(t, resources1, 2)
	token1, _ := out1["NextToken"].(string)
	require.NotEmpty(t, token1)

	rec2 := doResourceGroupsRequest(t, h, "ListGroupResources", map[string]any{
		"Group":      "paged-group",
		"MaxResults": 10,
		"NextToken":  token1,
	})
	require.Equal(t, http.StatusOK, rec2.Code)

	var out2 map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &out2))
	resources2 := out2["Resources"].([]any)
	assert.Len(t, resources2, 3)
	assert.Empty(t, out2["NextToken"])
}

func TestResourceGroupsHandler_ListGroupingStatuses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(t *testing.T, h *resourcegroups.Handler)
		name         string
		group        string
		wantContains []string
		wantCode     int
	}{
		{
			name:  "success",
			group: "my-group",
			setup: func(t *testing.T, h *resourcegroups.Handler) {
				t.Helper()
				doResourceGroupsRequest(t, h, "CreateGroup", map[string]any{"Name": "my-group"})
				doResourceGroupsRequest(t, h, "GroupResources", map[string]any{
					"Group":        "my-group",
					"ResourceArns": []string{"arn:aws:ec2:us-east-1:000000000000:instance/i-abc"},
				})
			},
			wantCode:     http.StatusOK,
			wantContains: []string{"GroupingStatuses", "i-abc"},
		},
		{
			name:     "missing_group",
			group:    "",
			wantCode: http.StatusBadRequest,
		},
		{
			// ListGroupingStatusesInput's declared error set (deserializers.go)
			// has no NotFoundException, unlike sibling ListGroupResources -- a
			// nonexistent group must return an empty result, not 404
			// (gopherstack-m4k0).
			name:         "nonexistent_group_returns_empty",
			group:        "nonexistent",
			wantCode:     http.StatusOK,
			wantContains: []string{`"GroupingStatuses":[]`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestResourceGroupsHandler(t)
			if tt.setup != nil {
				tt.setup(t, h)
			}

			rec := doResourceGroupsRequest(t, h, "ListGroupingStatuses", map[string]any{"Group": tt.group})
			assert.Equal(t, tt.wantCode, rec.Code)
			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

// TestListGroupingStatuses_AfterGroupResources verifies status records are
// created when resources are grouped.
func TestListGroupingStatuses_AfterGroupResources(t *testing.T) {
	t.Parallel()

	h := newTestResourceGroupsHandler(t)
	doResourceGroupsRequest(t, h, "CreateGroup", map[string]any{"Name": "g1"})
	doResourceGroupsRequest(t, h, "GroupResources", map[string]any{
		"Group":        "g1",
		"ResourceArns": []string{"arn:aws:s3:::b1"},
	})

	rec := doResourceGroupsRequest(t, h, "ListGroupingStatuses", map[string]any{
		"Group": "g1",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "SUCCESS")
}

// TestListGroupingStatuses_IncludesUngroup verifies UNGROUP statuses appear.
func TestListGroupingStatuses_IncludesUngroup(t *testing.T) {
	t.Parallel()

	h := newTestResourceGroupsHandler(t)
	doResourceGroupsRequest(t, h, "CreateGroup", map[string]any{"Name": "lifecycle-group"})
	doResourceGroupsRequest(t, h, "GroupResources", map[string]any{
		"Group":        "lifecycle-group",
		"ResourceArns": []string{"arn:aws:s3:::b1", "arn:aws:s3:::b2"},
	})
	doResourceGroupsRequest(t, h, "UngroupResources", map[string]any{
		"Group":        "lifecycle-group",
		"ResourceArns": []string{"arn:aws:s3:::b1"},
	})

	rec := doResourceGroupsRequest(t, h, "ListGroupingStatuses", map[string]any{
		"Group": "lifecycle-group",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, "GROUP")
	assert.Contains(t, body, "UNGROUP")
	assert.Contains(t, body, "SUCCESS")
}

// TestResourceGroupsHandler_GroupingStatuses_UpdatedAtIsEpochSeconds verifies
// UpdatedAt is serialized as a JSON number of epoch seconds, not a string.
func TestResourceGroupsHandler_GroupingStatuses_UpdatedAtIsEpochSeconds(t *testing.T) {
	t.Parallel()

	h := newTestResourceGroupsHandler(t)
	doResourceGroupsRequest(t, h, "CreateGroup", map[string]any{"Name": "status-group"})
	doResourceGroupsRequest(t, h, "GroupResources", map[string]any{
		"Group":        "status-group",
		"ResourceArns": []string{"arn:aws:s3:::my-bucket"},
	})

	rec := doResourceGroupsRequest(t, h, "ListGroupingStatuses", map[string]any{"Group": "status-group"})
	require.Equal(t, http.StatusOK, rec.Code)

	var out struct {
		GroupingStatuses []map[string]any `json:"GroupingStatuses"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.Len(t, out.GroupingStatuses, 1)
	_, isNumber := out.GroupingStatuses[0]["UpdatedAt"].(float64)
	assert.True(t, isNumber, "ListGroupingStatuses UpdatedAt must be a JSON number: %s", rec.Body.String())
}

// TestListGroupingStatuses_PaginationViaHandler verifies handler NextToken.
func TestListGroupingStatuses_PaginationViaHandler(t *testing.T) {
	t.Parallel()

	h := newTestResourceGroupsHandler(t)
	doResourceGroupsRequest(t, h, "CreateGroup", map[string]any{"Name": "status-group"})

	arns := make([]string, 5)
	for i := range arns {
		arns[i] = fmt.Sprintf("arn:aws:s3:::b-%d", i)
	}
	doResourceGroupsRequest(t, h, "GroupResources", map[string]any{
		"Group":        "status-group",
		"ResourceArns": arns,
	})

	rec1 := doResourceGroupsRequest(t, h, "ListGroupingStatuses", map[string]any{
		"Group":      "status-group",
		"MaxResults": 3,
	})
	require.Equal(t, http.StatusOK, rec1.Code)

	var out1 map[string]any
	require.NoError(t, json.Unmarshal(rec1.Body.Bytes(), &out1))
	statuses1 := out1["GroupingStatuses"].([]any)
	assert.Len(t, statuses1, 3)
	tok1, _ := out1["NextToken"].(string)
	require.NotEmpty(t, tok1)

	rec2 := doResourceGroupsRequest(t, h, "ListGroupingStatuses", map[string]any{
		"Group":      "status-group",
		"MaxResults": 10,
		"NextToken":  tok1,
	})
	require.Equal(t, http.StatusOK, rec2.Code)

	var out2 map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &out2))
	statuses2 := out2["GroupingStatuses"].([]any)
	assert.Len(t, statuses2, 2)
	assert.Empty(t, out2["NextToken"])
}

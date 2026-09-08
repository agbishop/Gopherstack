package cloudformation_test

import (
	"encoding/xml"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudformation"
)

// ---- Handler: DetectStackDrift ---------------------------------------------

func TestHandler_DetectStackDrift(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		body     string
		wantCode int
	}{
		{
			name: "success",
			body: "Action=CreateStack&StackName=drift-stack&TemplateBody=" + simpleTemplate,
		},
		{
			name:     "missing_stack_name",
			body:     "Action=DetectStackDrift",
			wantCode: 400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()
			if tt.wantCode == 0 {
				// Create stack first
				postForm(t, h, tt.body)
				rec := postForm(t, h, "Action=DetectStackDrift&StackName=drift-stack")
				assert.Equal(t, 200, rec.Code)

				var resp struct {
					XMLName xml.Name `xml:"DetectStackDriftResponse"`
					Result  struct {
						StackDriftDetectionID string `xml:"StackDriftDetectionId"`
					} `xml:"DetectStackDriftResult"`
				}
				require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
				assert.NotEmpty(t, resp.Result.StackDriftDetectionID)
			} else {
				rec := postForm(t, h, tt.body)
				assert.Equal(t, tt.wantCode, rec.Code)
			}
		})
	}
}

// ---- Handler: DescribeStackDriftDetectionStatus ----------------------------

func TestHandler_DescribeStackDriftDetectionStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		wantCode int
	}{
		{name: "success"},
		{name: "missing_detection_id", wantCode: 400},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()

			if tt.wantCode != 0 {
				rec := postForm(t, h, "Action=DescribeStackDriftDetectionStatus")
				assert.Equal(t, tt.wantCode, rec.Code)

				return
			}

			postForm(t, h, "Action=CreateStack&StackName=test-stack&TemplateBody="+simpleTemplate)
			rec := postForm(t, h, "Action=DetectStackDrift&StackName=test-stack")
			assert.Equal(t, 200, rec.Code)

			var detectResp struct {
				Result struct {
					ID string `xml:"StackDriftDetectionId"`
				} `xml:"DetectStackDriftResult"`
			}
			require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &detectResp))

			rec2 := postForm(
				t,
				h,
				"Action=DescribeStackDriftDetectionStatus&StackDriftDetectionId="+detectResp.Result.ID,
			)
			assert.Equal(t, 200, rec2.Code)

			var statusResp struct {
				Result struct {
					DetectionStatus string `xml:"DetectionStatus"`
					DriftStatus     string `xml:"StackDriftStatus"`
				} `xml:"DescribeStackDriftDetectionStatusResult"`
			}
			require.NoError(t, xml.Unmarshal(rec2.Body.Bytes(), &statusResp))
			assert.Equal(t, "DETECTION_COMPLETE", statusResp.Result.DetectionStatus)
			assert.Equal(t, "IN_SYNC", statusResp.Result.DriftStatus)
		})
	}
}

// ---- Handler: DetectStackResourceDrift -------------------------------------

func TestHandler_DetectStackResourceDrift(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		body     string
		wantCode int
	}{
		{
			name:     "missing_stack_name",
			body:     "Action=DetectStackResourceDrift&LogicalResourceId=MyBucket",
			wantCode: 400,
		},
		{
			name:     "missing_logical_id",
			body:     "Action=DetectStackResourceDrift&StackName=my-stack",
			wantCode: 400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()
			rec := postForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		h := newHandler()
		postForm(t, h, "Action=CreateStack&StackName=my-stack&TemplateBody="+simpleTemplate)
		rec := postForm(t, h, "Action=DetectStackResourceDrift&StackName=my-stack&LogicalResourceId=MyBucket")
		assert.Equal(t, 200, rec.Code)

		var resp struct {
			Result struct {
				Drift struct {
					LogicalResourceID string `xml:"LogicalResourceId"`
					ResourceType      string `xml:"ResourceType"`
					StackID           string `xml:"StackId"`
					DriftStatus       string `xml:"StackResourceDriftStatus"`
					Timestamp         string `xml:"Timestamp"`
				} `xml:"StackResourceDrift"`
			} `xml:"DetectStackResourceDriftResult"`
		}
		require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, "MyBucket", resp.Result.Drift.LogicalResourceID)
		assert.NotEmpty(t, resp.Result.Drift.ResourceType)
		assert.NotEmpty(t, resp.Result.Drift.StackID)
		assert.Equal(t, "IN_SYNC", resp.Result.Drift.DriftStatus)
		assert.NotEmpty(t, resp.Result.Drift.Timestamp)
	})
}

// ---- Handler: DescribeStackResourceDrifts ----------------------------------

func TestHandler_DescribeStackResourceDrifts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		body     string
		wantCode int
	}{
		{name: "success"},
		{
			name:     "missing_stack_name",
			body:     "Action=DescribeStackResourceDrifts",
			wantCode: 400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()

			if tt.wantCode != 0 {
				rec := postForm(t, h, tt.body)
				assert.Equal(t, tt.wantCode, rec.Code)

				return
			}

			postForm(t, h, "Action=CreateStack&StackName=my-stack&TemplateBody="+simpleTemplate)
			rec := postForm(t, h, "Action=DescribeStackResourceDrifts&StackName=my-stack")
			assert.Equal(t, 200, rec.Code)

			var resp struct {
				Result struct {
					Drifts []struct {
						DriftStatus string `xml:"StackResourceDriftStatus"`
					} `xml:"StackResourceDrifts>member"`
				} `xml:"DescribeStackResourceDriftsResult"`
			}
			require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))

			for _, d := range resp.Result.Drifts {
				assert.Equal(t, "IN_SYNC", d.DriftStatus)
			}
		})
	}
}

// ---- Handler: SetStackPolicy / GetStackPolicy ------------------------------

func TestHandler_StackPolicy(t *testing.T) {
	t.Parallel()

	policy := `{"Statement":[{"Effect":"Allow","Action":"Update:*","Principal":"*","Resource":"*"}]}`

	tests := []struct {
		name     string
		body     string
		wantCode int
	}{
		{name: "success"},
		{
			name:     "set_missing_stack_name",
			body:     "Action=SetStackPolicy&StackPolicyBody=" + policy,
			wantCode: 400,
		},
		{
			name:     "set_empty_policy_body",
			body:     "Action=SetStackPolicy&StackName=any-stack",
			wantCode: 400,
		},
		{
			name:     "get_missing_stack_name",
			body:     "Action=GetStackPolicy",
			wantCode: 400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()

			if tt.wantCode != 0 {
				rec := postForm(t, h, tt.body)
				assert.Equal(t, tt.wantCode, rec.Code)

				return
			}

			postForm(t, h, "Action=CreateStack&StackName=my-stack&TemplateBody="+simpleTemplate)

			rec := postForm(t, h, "Action=SetStackPolicy&StackName=my-stack&StackPolicyBody="+policy)
			assert.Equal(t, 200, rec.Code)

			rec2 := postForm(t, h, "Action=GetStackPolicy&StackName=my-stack")
			assert.Equal(t, 200, rec2.Code)

			var resp struct {
				Result struct {
					Body string `xml:"StackPolicyBody"`
				} `xml:"GetStackPolicyResult"`
			}
			require.NoError(t, xml.Unmarshal(rec2.Body.Bytes(), &resp))
			assert.Equal(t, policy, resp.Result.Body)
		})
	}
}

// ---- Handler: GetTemplateSummary -------------------------------------------

func TestHandler_GetTemplateSummary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		body     string
		wantCode int
	}{
		{
			name:     "from_template_body",
			body:     "Action=GetTemplateSummary&TemplateBody=" + simpleTemplate,
			wantCode: 200,
		},
		{
			name:     "empty_body_empty_stack",
			body:     "Action=GetTemplateSummary",
			wantCode: 200,
		},
		{
			name:     "stack_not_found",
			body:     "Action=GetTemplateSummary&StackName=missing-stack",
			wantCode: 400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()
			rec := postForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

// ---- Handler: EstimateTemplateCost -----------------------------------------

func TestHandler_EstimateTemplateCost(t *testing.T) {
	t.Parallel()

	h := newHandler()
	rec := postForm(t, h, "Action=EstimateTemplateCost&TemplateBody="+simpleTemplate)
	assert.Equal(t, 200, rec.Code)

	var resp struct {
		Result struct {
			URL string `xml:"Url"`
		} `xml:"EstimateTemplateCostResult"`
	}
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.Result.URL)
}

// ---- Handler: ContinueUpdateRollback ---------------------------------------

func TestHandler_ContinueUpdateRollback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		body     string
		wantCode int
	}{
		{name: "success"},
		{
			name:     "missing_stack_name",
			body:     "Action=ContinueUpdateRollback",
			wantCode: 400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()

			if tt.wantCode != 0 {
				rec := postForm(t, h, tt.body)
				assert.Equal(t, tt.wantCode, rec.Code)

				return
			}

			postForm(t, h, "Action=CreateStack&StackName=my-stack&TemplateBody="+simpleTemplate)
			rec := postForm(t, h, "Action=ContinueUpdateRollback&StackName=my-stack")
			assert.Equal(t, 200, rec.Code)
		})
	}
}

// ---- Handler: CancelUpdateStack --------------------------------------------

func TestHandler_CancelUpdateStack(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		body     string
		wantCode int
	}{
		{name: "success"},
		{
			// Real AWS: "You can cancel only stacks that are in the
			// UPDATE_IN_PROGRESS state."
			name:     "rejected_when_not_update_in_progress",
			wantCode: 400,
		},
		{
			name:     "missing_stack_name",
			body:     "Action=CancelUpdateStack",
			wantCode: 400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend()
			h := cloudformation.NewHandler(b)

			if tt.name == "missing_stack_name" {
				rec := postForm(t, h, tt.body)
				assert.Equal(t, tt.wantCode, rec.Code)

				return
			}

			postForm(t, h, "Action=CreateStack&StackName=my-stack&TemplateBody="+simpleTemplate)

			if tt.name == "success" {
				b.ForceStackStatus("my-stack", "UPDATE_IN_PROGRESS")
			}

			rec := postForm(t, h, "Action=CancelUpdateStack&StackName=my-stack")

			if tt.wantCode != 0 {
				assert.Equal(t, tt.wantCode, rec.Code)

				return
			}

			assert.Equal(t, 200, rec.Code)
		})
	}
}

// ---- Handler: DescribeAccountLimits ----------------------------------------

func TestHandler_DescribeAccountLimits(t *testing.T) {
	t.Parallel()

	h := newHandler()
	rec := postForm(t, h, "Action=DescribeAccountLimits")
	assert.Equal(t, 200, rec.Code)

	var resp struct {
		Result struct {
			Limits []struct {
				Name  string `xml:"Name"`
				Value int    `xml:"Value"`
			} `xml:"AccountLimits>member"`
		} `xml:"DescribeAccountLimitsResult"`
	}
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.Result.Limits)
}

// ---- Handler: Chaos and metadata methods -----------------------------------

func TestHandler_ChaosAndMetadata(t *testing.T) {
	t.Parallel()

	h := newHandler()

	t.Run("chaos_service_name", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "cloudformation", h.ChaosServiceName())
	})

	t.Run("chaos_operations", func(t *testing.T) {
		t.Parallel()
		ops := h.ChaosOperations()
		assert.NotEmpty(t, ops)
		assert.Contains(t, ops, "CreateStack")
		assert.Contains(t, ops, "DetectStackDrift")
	})

	t.Run("chaos_regions", func(t *testing.T) {
		t.Parallel()
		regions := h.ChaosRegions()
		assert.NotEmpty(t, regions)
	})
}

// ---- Handler: GetSupportedOperations includes new ops ----------------------

func TestHandler_GetSupportedOperationsExt(t *testing.T) {
	t.Parallel()

	h := newHandler()
	ops := h.GetSupportedOperations()

	newOps := []string{
		"DetectStackDrift",
		"DetectStackResourceDrift",
		"DescribeStackDriftDetectionStatus",
		"DescribeStackResourceDrifts",
		"SetStackPolicy",
		"GetStackPolicy",
		"GetTemplateSummary",
		"EstimateTemplateCost",
		"ContinueUpdateRollback",
		"CancelUpdateStack",
		"DescribeAccountLimits",
	}

	for _, op := range newOps {
		assert.Contains(t, ops, op)
	}
}

// ---- Handler: ChangeSet error paths ----------------------------------------

func TestHandler_ChangeSetErrorPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{
			name: "execute_not_found",
			body: "Action=ExecuteChangeSet&StackName=no-stack&ChangeSetName=no-cs",
		},
		{
			name: "delete_not_found",
			body: "Action=DeleteChangeSet&StackName=no-stack&ChangeSetName=no-cs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()
			rec := postForm(t, h, tt.body)
			assert.Equal(t, 400, rec.Code)
		})
	}
}

// ---- Handler: new op error paths -------------------------------------------

func TestHandler_ExtOpsErrorPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{
			name: "detect_drift_stack_not_found",
			body: "Action=DetectStackDrift&StackName=no-such-stack",
		},
		{
			name: "detect_resource_drift_stack_not_found",
			body: "Action=DetectStackResourceDrift&StackName=no-such-stack&LogicalResourceId=MyBucket",
		},
		{
			name: "describe_drift_status_not_found",
			body: "Action=DescribeStackDriftDetectionStatus&StackDriftDetectionId=no-such-id",
		},
		{
			name: "describe_resource_drifts_stack_not_found",
			body: "Action=DescribeStackResourceDrifts&StackName=no-such-stack",
		},
		{
			name: "set_policy_stack_not_found",
			body: "Action=SetStackPolicy&StackName=no-such-stack&StackPolicyBody={}",
		},
		{
			name: "get_policy_stack_not_found",
			body: "Action=GetStackPolicy&StackName=no-such-stack",
		},
		{
			name: "continue_rollback_stack_not_found",
			body: "Action=ContinueUpdateRollback&StackName=no-such-stack",
		},
		{
			name: "cancel_update_stack_not_found",
			body: "Action=CancelUpdateStack&StackName=no-such-stack",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()
			rec := postForm(t, h, tt.body)
			assert.Equal(t, 400, rec.Code)
		})
	}
}

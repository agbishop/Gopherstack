package autoscaling_test

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/autoscaling"
)

// TestAutoscalingHandler_LaunchConfigurationUserDataRoundTrip verifies that UserData, KernelId, RamdiskId
// are stored and returned by DescribeLaunchConfigurations.
func TestAutoscalingHandler_LaunchConfigurationUserDataRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		lcName    string
		userData  string
		kernelID  string
		ramdiskID string
	}{
		{
			name:      "with all three fields",
			lcName:    "lc-userdata",
			userData:  "IyEvYmluL2Jhc2gKZWNobyBoZWxsbw==",
			kernelID:  "aki-12345678",
			ramdiskID: "ari-12345678",
		},
		{
			name:     "userData only",
			lcName:   "lc-userdata-only",
			userData: "dXNlcmRhdGE=",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newAutoscalingHandler()

			createVals := url.Values{
				"LaunchConfigurationName": {tc.lcName},
				"ImageId":                 {"ami-12345678"},
				"InstanceType":            {"t3.micro"},
				"UserData":                {tc.userData},
			}
			if tc.kernelID != "" {
				createVals.Set("KernelId", tc.kernelID)
			}
			if tc.ramdiskID != "" {
				createVals.Set("RamdiskId", tc.ramdiskID)
			}

			code, body := doAS(t, h, "CreateLaunchConfiguration", createVals)
			require.Equal(t, 200, code, body)

			code, body = doAS(t, h, "DescribeLaunchConfigurations", url.Values{
				"LaunchConfigurationNames.member.1": {tc.lcName},
			})
			require.Equal(t, 200, code)

			if tc.userData != "" {
				assert.Contains(t, body, fmt.Sprintf("<UserData>%s</UserData>", tc.userData))
			}
			if tc.kernelID != "" {
				assert.Contains(t, body, fmt.Sprintf("<KernelId>%s</KernelId>", tc.kernelID))
			}
			if tc.ramdiskID != "" {
				assert.Contains(t, body, fmt.Sprintf("<RamdiskId>%s</RamdiskId>", tc.ramdiskID))
			}
		})
	}
}

func TestAutoscalingHandler_LaunchConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{
			name: "create_success",
			body: "Action=CreateLaunchConfiguration&Version=2011-01-01" +
				"&LaunchConfigurationName=my-lc&ImageId=ami-12345678&InstanceType=t2.micro",
			wantStatus: http.StatusOK,
		},
		{
			name:       "describe",
			body:       "Action=DescribeLaunchConfigurations&Version=2011-01-01",
			wantStatus: http.StatusOK,
		},
		{
			name:       "unknown_action",
			body:       "Action=UnknownAction&Version=2011-01-01",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newAutoscalingHandler()
			rec := postAutoscalingForm(t, h, tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestAutoscalingHandler_DescribeLaunchConfigurationsWithData(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setup      func(t *testing.T, h *autoscaling.Handler)
		body       string
		wantStatus int
		wantCount  int
	}{
		{
			name: "with_lcs",
			setup: func(t *testing.T, h *autoscaling.Handler) {
				t.Helper()
				postAutoscalingForm(
					t,
					h,
					"Action=CreateLaunchConfiguration&Version=2011-01-01"+
						"&LaunchConfigurationName=lc-a&ImageId=ami-111&InstanceType=t2.micro",
				)
				postAutoscalingForm(
					t,
					h,
					"Action=CreateLaunchConfiguration&Version=2011-01-01"+
						"&LaunchConfigurationName=lc-b&ImageId=ami-222&InstanceType=t3.small",
				)
			},
			body:       "Action=DescribeLaunchConfigurations&Version=2011-01-01",
			wantStatus: http.StatusOK,
			wantCount:  2,
		},
		{
			name: "filter_by_name",
			setup: func(t *testing.T, h *autoscaling.Handler) {
				t.Helper()
				postAutoscalingForm(
					t,
					h,
					"Action=CreateLaunchConfiguration&Version=2011-01-01"+
						"&LaunchConfigurationName=lc-filter&ImageId=ami-333&InstanceType=t2.micro",
				)
			},
			body:       "Action=DescribeLaunchConfigurations&Version=2011-01-01&LaunchConfigurationNames.member.1=lc-filter",
			wantStatus: http.StatusOK,
			wantCount:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newAutoscalingHandler()
			if tt.setup != nil {
				tt.setup(t, h)
			}

			rec := postAutoscalingForm(t, h, tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantCount > 0 {
				var resp struct {
					XMLName xml.Name `xml:"DescribeLaunchConfigurationsResponse"`
					Result  struct {
						LaunchConfigurations struct {
							Members []struct {
								Name string `xml:"LaunchConfigurationName"`
							} `xml:"member"`
						} `xml:"LaunchConfigurations"`
					} `xml:"DescribeLaunchConfigurationsResult"`
				}
				require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Len(t, resp.Result.LaunchConfigurations.Members, tt.wantCount)
			}
		})
	}
}

func TestAutoscalingHandler_DeleteLaunchConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setup      func(t *testing.T, h *autoscaling.Handler)
		body       string
		wantStatus int
	}{
		{
			name: "delete_existing_lc",
			setup: func(t *testing.T, h *autoscaling.Handler) {
				t.Helper()
				postAutoscalingForm(
					t,
					h,
					"Action=CreateLaunchConfiguration&Version=2011-01-01"+
						"&LaunchConfigurationName=del-lc&ImageId=ami-abc&InstanceType=t2.micro",
				)
			},
			body:       "Action=DeleteLaunchConfiguration&Version=2011-01-01&LaunchConfigurationName=del-lc",
			wantStatus: http.StatusOK,
		},
		{
			name:       "delete_nonexistent_lc",
			body:       "Action=DeleteLaunchConfiguration&Version=2011-01-01&LaunchConfigurationName=no-such-lc",
			wantStatus: http.StatusBadRequest,
		},
		{
			// api_op_DeleteLaunchConfiguration.go: "The launch configuration
			// must not be attached to an Auto Scaling group."
			name: "delete_lc_attached_to_group_rejected",
			setup: func(t *testing.T, h *autoscaling.Handler) {
				t.Helper()
				postAutoscalingForm(
					t,
					h,
					"Action=CreateLaunchConfiguration&Version=2011-01-01"+
						"&LaunchConfigurationName=attached-lc&ImageId=ami-abc&InstanceType=t2.micro",
				)
				postAutoscalingForm(
					t,
					h,
					"Action=CreateAutoScalingGroup&Version=2011-01-01"+
						"&AutoScalingGroupName=lc-owner&MinSize=0&MaxSize=1"+
						"&LaunchConfigurationName=attached-lc",
				)
			},
			body:       "Action=DeleteLaunchConfiguration&Version=2011-01-01&LaunchConfigurationName=attached-lc",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newAutoscalingHandler()
			if tt.setup != nil {
				tt.setup(t, h)
			}

			rec := postAutoscalingForm(t, h, tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.name == "delete_lc_attached_to_group_rejected" {
				assert.Contains(t, rec.Body.String(), "ResourceInUse")
			}
		})
	}
}

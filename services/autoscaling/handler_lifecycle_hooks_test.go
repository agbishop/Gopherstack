package autoscaling_test

import (
	"context"
	"encoding/xml"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/autoscaling"
)

// TestAutoscalingHandler_LifecycleHookGlobalTimeout verifies that DescribeLifecycleHooks returns
// GlobalTimeout equal to HeartbeatTimeout (numberOfRetries=1 is the default).
func TestAutoscalingHandler_LifecycleHookGlobalTimeout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		heartbeatTimeout string
		wantGlobal       string
	}{
		{"default timeout", "", "<GlobalTimeout>3600</GlobalTimeout>"},
		{"custom 600", "600", "<GlobalTimeout>600</GlobalTimeout>"},
		{"custom 7200", "7200", "<GlobalTimeout>7200</GlobalTimeout>"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newAutoscalingHandler()
			asgName := "hook-asg-" + strings.ReplaceAll(tc.name, " ", "-")

			code, body := doAS(t, h, "CreateAutoScalingGroup", url.Values{
				"AutoScalingGroupName":       {asgName},
				"MinSize":                    {"0"},
				"MaxSize":                    {"5"},
				"AvailabilityZones.member.1": {"us-east-1a"},
			})
			require.Equal(t, 200, code, body)

			hookVals := url.Values{
				"AutoScalingGroupName": {asgName},
				"LifecycleHookName":    {"my-hook"},
				"LifecycleTransition":  {"autoscaling:EC2_INSTANCE_LAUNCHING"},
			}
			if tc.heartbeatTimeout != "" {
				hookVals.Set("HeartbeatTimeout", tc.heartbeatTimeout)
			}

			code, body = doAS(t, h, "PutLifecycleHook", hookVals)
			require.Equal(t, 200, code, body)

			code, body = doAS(t, h, "DescribeLifecycleHooks", url.Values{
				"AutoScalingGroupName": {asgName},
			})
			require.Equal(t, 200, code)

			assert.Contains(t, body, tc.wantGlobal,
				"GlobalTimeout must equal HeartbeatTimeout; got: %s", body)
		})
	}
}

// xmlASGInstancesResponse is a minimal struct for parsing DescribeAutoScalingGroups
// focused on instance lifecycle state and capacity fields.
type xmlASGInstancesResponse struct {
	XMLName xml.Name `xml:"DescribeAutoScalingGroupsResponse"`
	Result  struct {
		AutoScalingGroups struct {
			Members []struct {
				AutoScalingGroupName string `xml:"AutoScalingGroupName"`
				MixedInstancesPolicy struct {
					LaunchTemplate struct {
						LaunchTemplateSpecification struct {
							LaunchTemplateID string `xml:"LaunchTemplateId"`
						} `xml:"LaunchTemplateSpecification"`
						Overrides struct {
							Members []struct {
								InstanceType     string `xml:"InstanceType"`
								WeightedCapacity string `xml:"WeightedCapacity"`
							} `xml:"member"`
						} `xml:"Overrides"`
					} `xml:"LaunchTemplate"`
					InstancesDistribution struct {
						SpotAllocationStrategy string `xml:"SpotAllocationStrategy"`
						OnDemandBaseCapacity   int32  `xml:"OnDemandBaseCapacity"`
					} `xml:"InstancesDistribution"`
				} `xml:"MixedInstancesPolicy"`
				TrafficSources struct {
					Members []struct {
						Identifier string `xml:"Identifier"`
						Type       string `xml:"Type"`
					} `xml:"member"`
				} `xml:"TrafficSources"`
				Instances struct {
					Members []struct {
						InstanceID     string `xml:"InstanceId"`
						LifecycleState string `xml:"LifecycleState"`
					} `xml:"member"`
				} `xml:"Instances"`
				DesiredCapacity int32 `xml:"DesiredCapacity"`
				MinSize         int32 `xml:"MinSize"`
			} `xml:"member"`
		} `xml:"AutoScalingGroups"`
	} `xml:"DescribeAutoScalingGroupsResult"`
}

func describeASGInstances(t *testing.T, body string) xmlASGInstancesResponse {
	t.Helper()

	var parsed xmlASGInstancesResponse
	require.NoError(t, xml.Unmarshal([]byte(body), &parsed))
	require.Len(t, parsed.Result.AutoScalingGroups.Members, 1)

	return parsed
}

// TestAutoscalingHandler_LifecycleHookGatesLaunch verifies that an EC2_INSTANCE_LAUNCHING lifecycle
// hook pauses newly-launched instances in Pending:Wait, and that
// CompleteLifecycleAction resolves the wait (CONTINUE -> InService, ABANDON ->
// instance removed). Before this fix, PutLifecycleHook/CompleteLifecycleAction never
// touched real instance state -- hooks were pure bookkeeping with no effect.
func TestAutoscalingHandler_LifecycleHookGatesLaunch(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name            string
		result          string
		wantState       string
		wantReplacement bool
	}{
		{name: "continue resolves to InService", result: "CONTINUE", wantState: "InService"},
		{
			name:            "abandon terminates and relaunches a replacement",
			result:          "ABANDON",
			wantState:       "Pending:Wait",
			wantReplacement: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newAutoscalingHandler()
			asgName := "launch-hook-asg-" + tc.result

			code, body := doAS(t, h, "CreateAutoScalingGroup", url.Values{
				"AutoScalingGroupName":       {asgName},
				"MinSize":                    {"0"},
				"MaxSize":                    {"5"},
				"AvailabilityZones.member.1": {"us-east-1a"},
			})
			require.Equal(t, 200, code, body)

			code, body = doAS(t, h, "PutLifecycleHook", url.Values{
				"AutoScalingGroupName": {asgName},
				"LifecycleHookName":    {"launch-hook"},
				"LifecycleTransition":  {"autoscaling:EC2_INSTANCE_LAUNCHING"},
				"HeartbeatTimeout":     {"60"},
			})
			require.Equal(t, 200, code, body)

			code, body = doAS(t, h, "SetDesiredCapacity", url.Values{
				"AutoScalingGroupName": {asgName},
				"DesiredCapacity":      {"1"},
			})
			require.Equal(t, 200, code, body)

			code, body = doAS(t, h, "DescribeAutoScalingGroups", url.Values{
				"AutoScalingGroupNames.member.1": {asgName},
			})
			require.Equal(t, 200, code, body)

			parsed := describeASGInstances(t, body)
			require.Len(t, parsed.Result.AutoScalingGroups.Members[0].Instances.Members, 1,
				"gated instance must still appear in the group while Pending:Wait")

			inst := parsed.Result.AutoScalingGroups.Members[0].Instances.Members[0]
			assert.Equal(t, "Pending:Wait", inst.LifecycleState,
				"new instance behind an active launch hook must start Pending:Wait, not InService")

			code, body = doAS(t, h, "CompleteLifecycleAction", url.Values{
				"AutoScalingGroupName":  {asgName},
				"LifecycleHookName":     {"launch-hook"},
				"InstanceId":            {inst.InstanceID},
				"LifecycleActionResult": {tc.result},
			})
			require.Equal(t, 200, code, body)

			code, body = doAS(t, h, "DescribeAutoScalingGroups", url.Values{
				"AutoScalingGroupNames.member.1": {asgName},
			})
			require.Equal(t, 200, code, body)

			parsed = describeASGInstances(t, body)
			gotInstances := parsed.Result.AutoScalingGroups.Members[0].Instances.Members

			// AWS: ABANDON on a launch hook means "terminate and replace the instance"
			// (lifecycle-hooks.html), so DesiredCapacity=1 is maintained by a
			// replacement -- not left at zero.
			require.Len(t, gotInstances, 1)
			assert.Equal(t, tc.wantState, gotInstances[0].LifecycleState)

			if tc.wantReplacement {
				assert.NotEqual(t, inst.InstanceID, gotInstances[0].InstanceID,
					"the replacement must be a new instance, not the abandoned one")
			}
		})
	}
}

// TestAutoscalingHandler_LifecycleHookGatesTermination verifies that an EC2_INSTANCE_TERMINATING
// lifecycle hook defers instance removal: TerminateInstanceInAutoScalingGroup pauses
// the instance in Terminating:Wait (with an InProgress activity) instead of removing
// it immediately, and CompleteLifecycleAction finishes the termination and applies
// the originally-requested desired-capacity decrement.
func TestAutoscalingHandler_LifecycleHookGatesTermination(t *testing.T) {
	t.Parallel()

	h := newAutoscalingHandler()
	asgName := "terminate-hook-asg"

	code, body := doAS(t, h, "CreateAutoScalingGroup", url.Values{
		"AutoScalingGroupName":       {asgName},
		"MinSize":                    {"0"},
		"MaxSize":                    {"5"},
		"DesiredCapacity":            {"1"},
		"AvailabilityZones.member.1": {"us-east-1a"},
	})
	require.Equal(t, 200, code, body)

	code, body = doAS(t, h, "DescribeAutoScalingGroups", url.Values{
		"AutoScalingGroupNames.member.1": {asgName},
	})
	require.Equal(t, 200, code, body)

	parsed := describeASGInstances(t, body)
	require.Len(t, parsed.Result.AutoScalingGroups.Members[0].Instances.Members, 1)
	instanceID := parsed.Result.AutoScalingGroups.Members[0].Instances.Members[0].InstanceID

	code, body = doAS(t, h, "PutLifecycleHook", url.Values{
		"AutoScalingGroupName": {asgName},
		"LifecycleHookName":    {"terminate-hook"},
		"LifecycleTransition":  {"autoscaling:EC2_INSTANCE_TERMINATING"},
		"HeartbeatTimeout":     {"60"},
	})
	require.Equal(t, 200, code, body)

	code, body = doAS(t, h, "TerminateInstanceInAutoScalingGroup", url.Values{
		"InstanceId":                     {instanceID},
		"ShouldDecrementDesiredCapacity": {"true"},
	})
	require.Equal(t, 200, code, body)
	assert.Contains(t, body, "<StatusCode>InProgress</StatusCode>",
		"a deferred termination must report InProgress while waiting on the hook; got: %s", body)

	code, body = doAS(t, h, "DescribeAutoScalingGroups", url.Values{
		"AutoScalingGroupNames.member.1": {asgName},
	})
	require.Equal(t, 200, code, body)

	parsed = describeASGInstances(t, body)
	group := parsed.Result.AutoScalingGroups.Members[0]
	require.Len(t, group.Instances.Members, 1, "instance must remain present during Terminating:Wait")
	assert.Equal(t, "Terminating:Wait", group.Instances.Members[0].LifecycleState)
	assert.Equal(t, int32(1), group.DesiredCapacity, "capacity decrement must be deferred until the hook resolves")

	code, body = doAS(t, h, "CompleteLifecycleAction", url.Values{
		"AutoScalingGroupName":  {asgName},
		"LifecycleHookName":     {"terminate-hook"},
		"InstanceId":            {instanceID},
		"LifecycleActionResult": {"CONTINUE"},
	})
	require.Equal(t, 200, code, body)

	code, body = doAS(t, h, "DescribeAutoScalingGroups", url.Values{
		"AutoScalingGroupNames.member.1": {asgName},
	})
	require.Equal(t, 200, code, body)

	parsed = describeASGInstances(t, body)
	group = parsed.Result.AutoScalingGroups.Members[0]
	assert.Empty(t, group.Instances.Members, "instance must be removed once the terminating hook resolves")
	assert.Equal(t, int32(0), group.DesiredCapacity, "ShouldDecrementDesiredCapacity must apply once resolved")
}

// TestAutoscalingHandler_LifecycleHookGatesDesiredCapacityScaleIn verifies that an
// EC2_INSTANCE_TERMINATING lifecycle hook also gates the desired-capacity-driven
// scale-in path (SetDesiredCapacity), not just TerminateInstanceInAutoScalingGroup.
// Before this fix, applyDesiredCapacityChange's scale-in branch removed instances
// immediately regardless of any registered terminating hook.
func TestAutoscalingHandler_LifecycleHookGatesDesiredCapacityScaleIn(t *testing.T) {
	t.Parallel()

	h := newAutoscalingHandler()
	asgName := "scale-in-hook-asg"

	code, body := doAS(t, h, "CreateAutoScalingGroup", url.Values{
		"AutoScalingGroupName":       {asgName},
		"MinSize":                    {"0"},
		"MaxSize":                    {"5"},
		"DesiredCapacity":            {"1"},
		"AvailabilityZones.member.1": {"us-east-1a"},
	})
	require.Equal(t, 200, code, body)

	code, body = doAS(t, h, "DescribeAutoScalingGroups", url.Values{
		"AutoScalingGroupNames.member.1": {asgName},
	})
	require.Equal(t, 200, code, body)

	parsed := describeASGInstances(t, body)
	require.Len(t, parsed.Result.AutoScalingGroups.Members[0].Instances.Members, 1)
	instanceID := parsed.Result.AutoScalingGroups.Members[0].Instances.Members[0].InstanceID

	code, body = doAS(t, h, "PutLifecycleHook", url.Values{
		"AutoScalingGroupName": {asgName},
		"LifecycleHookName":    {"scale-in-terminate-hook"},
		"LifecycleTransition":  {"autoscaling:EC2_INSTANCE_TERMINATING"},
		"HeartbeatTimeout":     {"60"},
	})
	require.Equal(t, 200, code, body)

	code, body = doAS(t, h, "SetDesiredCapacity", url.Values{
		"AutoScalingGroupName": {asgName},
		"DesiredCapacity":      {"0"},
	})
	require.Equal(t, 200, code, body)

	code, body = doAS(t, h, "DescribeAutoScalingGroups", url.Values{
		"AutoScalingGroupNames.member.1": {asgName},
	})
	require.Equal(t, 200, code, body)

	parsed = describeASGInstances(t, body)
	group := parsed.Result.AutoScalingGroups.Members[0]
	require.Len(t, group.Instances.Members, 1,
		"instance behind an active terminating hook must remain present during scale-in, not be removed instantly")
	assert.Equal(t, "Terminating:Wait", group.Instances.Members[0].LifecycleState)
	assert.Equal(t, instanceID, group.Instances.Members[0].InstanceID)
	assert.Equal(t, int32(0), group.DesiredCapacity,
		"DesiredCapacity reflects the new target immediately, even while the instance removal itself is deferred")

	code, body = doAS(t, h, "CompleteLifecycleAction", url.Values{
		"AutoScalingGroupName":  {asgName},
		"LifecycleHookName":     {"scale-in-terminate-hook"},
		"InstanceId":            {instanceID},
		"LifecycleActionResult": {"CONTINUE"},
	})
	require.Equal(t, 200, code, body)

	code, body = doAS(t, h, "DescribeAutoScalingGroups", url.Values{
		"AutoScalingGroupNames.member.1": {asgName},
	})
	require.Equal(t, 200, code, body)

	parsed = describeASGInstances(t, body)
	group = parsed.Result.AutoScalingGroups.Members[0]
	assert.Empty(t, group.Instances.Members, "instance must be removed once the terminating hook resolves")
	assert.Equal(t, int32(0), group.DesiredCapacity, "DesiredCapacity must not be double-decremented on resolution")
}

// TestAutoscalingHandler_RecordLifecycleActionHeartbeatByInstanceID verifies that
// RecordLifecycleActionHeartbeat can locate a pending action by (group, hook,
// instance) when no LifecycleActionToken is supplied -- AWS accepts either.
func TestAutoscalingHandler_RecordLifecycleActionHeartbeatByInstanceID(t *testing.T) {
	t.Parallel()

	h := newAutoscalingHandler()
	asgName := "heartbeat-asg"

	code, body := doAS(t, h, "CreateAutoScalingGroup", url.Values{
		"AutoScalingGroupName":       {asgName},
		"MinSize":                    {"0"},
		"MaxSize":                    {"5"},
		"AvailabilityZones.member.1": {"us-east-1a"},
	})
	require.Equal(t, 200, code, body)

	code, body = doAS(t, h, "PutLifecycleHook", url.Values{
		"AutoScalingGroupName": {asgName},
		"LifecycleHookName":    {"launch-hook"},
		"LifecycleTransition":  {"autoscaling:EC2_INSTANCE_LAUNCHING"},
		"HeartbeatTimeout":     {"60"},
	})
	require.Equal(t, 200, code, body)

	code, body = doAS(t, h, "SetDesiredCapacity", url.Values{
		"AutoScalingGroupName": {asgName},
		"DesiredCapacity":      {"1"},
	})
	require.Equal(t, 200, code, body)

	code, body = doAS(t, h, "DescribeAutoScalingGroups", url.Values{
		"AutoScalingGroupNames.member.1": {asgName},
	})
	require.Equal(t, 200, code, body)

	parsed := describeASGInstances(t, body)
	instanceID := parsed.Result.AutoScalingGroups.Members[0].Instances.Members[0].InstanceID

	code, body = doAS(t, h, "RecordLifecycleActionHeartbeat", url.Values{
		"AutoScalingGroupName": {asgName},
		"LifecycleHookName":    {"launch-hook"},
		"InstanceId":           {instanceID},
	})
	require.Equal(t, 200, code, "heartbeat by instance ID (no token) must find the pending action; body: %s", body)

	code, body = doAS(t, h, "RecordLifecycleActionHeartbeat", url.Values{
		"AutoScalingGroupName": {asgName},
		"LifecycleHookName":    {"no-such-hook"},
		"InstanceId":           {instanceID},
	})
	assert.Equal(t, 400, code, "heartbeat for an unknown hook with no pending action must fail; body: %s", body)
}

// TestAutoscalingHandler_CreateASGWithInitialLifecycleHooks verifies that CreateAutoScalingGroup
// registers LifecycleHookSpecificationList atomically with group creation, and that
// the group's own initial instances are gated by a matching launch hook -- this is
// the wire shape Terraform's aws_autoscaling_group initial_lifecycle_hook block uses,
// and it was previously accepted and silently discarded.
func TestAutoscalingHandler_CreateASGWithInitialLifecycleHooks(t *testing.T) {
	t.Parallel()

	h := newAutoscalingHandler()
	asgName := "initial-hook-asg"

	code, body := doAS(t, h, "CreateAutoScalingGroup", url.Values{
		"AutoScalingGroupName":       {asgName},
		"MinSize":                    {"1"},
		"MaxSize":                    {"5"},
		"DesiredCapacity":            {"1"},
		"AvailabilityZones.member.1": {"us-east-1a"},
		"LifecycleHookSpecificationList.member.1.LifecycleHookName":    {"init-hook"},
		"LifecycleHookSpecificationList.member.1.LifecycleTransition":  {"autoscaling:EC2_INSTANCE_LAUNCHING"},
		"LifecycleHookSpecificationList.member.1.HeartbeatTimeout":     {"120"},
		"LifecycleHookSpecificationList.member.1.DefaultResult":        {"CONTINUE"},
		"LifecycleHookSpecificationList.member.1.NotificationMetadata": {"boot-meta"},
	})
	require.Equal(t, 200, code, body)

	code, body = doAS(t, h, "DescribeLifecycleHooks", url.Values{
		"AutoScalingGroupName": {asgName},
	})
	require.Equal(t, 200, code, body)
	assert.Contains(t, body, "<LifecycleHookName>init-hook</LifecycleHookName>")
	assert.Contains(t, body, "<NotificationMetadata>boot-meta</NotificationMetadata>",
		"NotificationMetadata must round-trip; got: %s", body)
	assert.Contains(t, body, "<GlobalTimeout>120</GlobalTimeout>")

	code, body = doAS(t, h, "DescribeAutoScalingGroups", url.Values{
		"AutoScalingGroupNames.member.1": {asgName},
	})
	require.Equal(t, 200, code, body)

	parsed := describeASGInstances(t, body)
	require.Len(t, parsed.Result.AutoScalingGroups.Members[0].Instances.Members, 1)
	assert.Equal(t, "Pending:Wait", parsed.Result.AutoScalingGroups.Members[0].Instances.Members[0].LifecycleState,
		"the group's own initial instance must be gated by a hook registered at creation time")
}

func TestAutoscalingHandler_CompleteLifecycleAction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setup      func(t *testing.T, h *autoscaling.Handler)
		body       string
		wantStatus int
	}{
		{
			name: "complete_lifecycle_action_success",
			setup: func(t *testing.T, h *autoscaling.Handler) {
				t.Helper()
				postAutoscalingForm(t, h,
					"Action=CreateAutoScalingGroup&Version=2011-01-01"+
						"&AutoScalingGroupName=lc-action-asg&MinSize=0&MaxSize=5")
			},
			body: "Action=CompleteLifecycleAction&Version=2011-01-01" +
				"&AutoScalingGroupName=lc-action-asg" +
				"&LifecycleHookName=my-hook" +
				"&LifecycleActionToken=abc123" +
				"&LifecycleActionResult=CONTINUE",
			wantStatus: http.StatusOK,
		},
		{
			name: "complete_lifecycle_group_not_found",
			body: "Action=CompleteLifecycleAction&Version=2011-01-01" +
				"&AutoScalingGroupName=no-such&LifecycleHookName=h&LifecycleActionResult=CONTINUE",
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "complete_lifecycle_missing_hook_name",
			setup: func(t *testing.T, h *autoscaling.Handler) {
				t.Helper()
				postAutoscalingForm(t, h,
					"Action=CreateAutoScalingGroup&Version=2011-01-01"+
						"&AutoScalingGroupName=miss-hook-asg&MinSize=0&MaxSize=5")
			},
			body: "Action=CompleteLifecycleAction&Version=2011-01-01" +
				"&AutoScalingGroupName=miss-hook-asg&LifecycleActionResult=CONTINUE",
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "complete_lifecycle_missing_result",
			setup: func(t *testing.T, h *autoscaling.Handler) {
				t.Helper()
				postAutoscalingForm(t, h,
					"Action=CreateAutoScalingGroup&Version=2011-01-01"+
						"&AutoScalingGroupName=miss-result-asg&MinSize=0&MaxSize=5")
			},
			body: "Action=CompleteLifecycleAction&Version=2011-01-01" +
				"&AutoScalingGroupName=miss-result-asg&LifecycleHookName=my-hook",
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
		})
	}
}

func TestAutoscalingHandler_DeleteLifecycleHook(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setup      func(t *testing.T, h *autoscaling.Handler, b *autoscaling.InMemoryBackend)
		body       string
		wantStatus int
	}{
		{
			name: "delete_existing_hook",
			setup: func(t *testing.T, _ *autoscaling.Handler, b *autoscaling.InMemoryBackend) {
				t.Helper()
				_, err := b.CreateAutoScalingGroup(autoscaling.CreateAutoScalingGroupInput{
					AutoScalingGroupName: "hook-asg",
					MinSize:              0,
					MaxSize:              5,
				})
				require.NoError(t, err)

				err = b.AddLifecycleHook(autoscaling.LifecycleHook{
					LifecycleHookName:    "my-hook",
					AutoScalingGroupName: "hook-asg",
					LifecycleTransition:  "autoscaling:EC2_INSTANCE_LAUNCHING",
					DefaultResult:        "CONTINUE",
				})
				require.NoError(t, err)
			},
			body:       "Action=DeleteLifecycleHook&Version=2011-01-01&AutoScalingGroupName=hook-asg&LifecycleHookName=my-hook",
			wantStatus: http.StatusOK,
		},
		{
			name: "delete_nonexistent_hook",
			setup: func(t *testing.T, h *autoscaling.Handler, _ *autoscaling.InMemoryBackend) {
				t.Helper()
				postAutoscalingForm(t, h,
					"Action=CreateAutoScalingGroup&Version=2011-01-01"+
						"&AutoScalingGroupName=no-hook-asg&MinSize=0&MaxSize=5")
			},
			body: "Action=DeleteLifecycleHook&Version=2011-01-01" +
				"&AutoScalingGroupName=no-hook-asg&LifecycleHookName=ghost-hook",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "delete_hook_group_not_found",
			body:       "Action=DeleteLifecycleHook&Version=2011-01-01&AutoScalingGroupName=no-such&LifecycleHookName=my-hook",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := autoscaling.NewInMemoryBackend()
			h := autoscaling.NewHandler(b)

			if tt.setup != nil {
				tt.setup(t, h, b)
			}

			rec := postAutoscalingForm(t, h, tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestAutoscalingHandler_PutAndDescribeLifecycleHooks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(t *testing.T, h *autoscaling.Handler)
		checkBody  func(t *testing.T, body string)
		name       string
		body       string
		wantStatus int
	}{
		{
			name: "put_lifecycle_hook_success",
			setup: func(t *testing.T, h *autoscaling.Handler) {
				t.Helper()
				postAutoscalingForm(
					t,
					h,
					"Action=CreateAutoScalingGroup&Version=2011-01-01&AutoScalingGroupName=hook-asg&MinSize=0&MaxSize=5",
				)
			},
			body: "Action=PutLifecycleHook&Version=2011-01-01&AutoScalingGroupName=hook-asg" +
				"&LifecycleHookName=my-hook&LifecycleTransition=autoscaling:EC2_INSTANCE_LAUNCHING",
			wantStatus: http.StatusOK,
		},
		{
			name:       "put_lifecycle_hook_group_not_found",
			body:       "Action=PutLifecycleHook&Version=2011-01-01&AutoScalingGroupName=no-such&LifecycleHookName=h",
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "describe_lifecycle_hooks_success",
			setup: func(t *testing.T, h *autoscaling.Handler) {
				t.Helper()
				postAutoscalingForm(
					t, h,
					"Action=CreateAutoScalingGroup&Version=2011-01-01&AutoScalingGroupName=dlh-asg&MinSize=0&MaxSize=5",
				)
				postAutoscalingForm(
					t, h,
					"Action=PutLifecycleHook&Version=2011-01-01&AutoScalingGroupName=dlh-asg&LifecycleHookName=h1",
				)
			},
			body:       "Action=DescribeLifecycleHooks&Version=2011-01-01&AutoScalingGroupName=dlh-asg",
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, "h1")
			},
		},
		{
			name:       "describe_lifecycle_hooks_group_not_found",
			body:       "Action=DescribeLifecycleHooks&Version=2011-01-01&AutoScalingGroupName=no-such",
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

			if tt.checkBody != nil {
				tt.checkBody(t, rec.Body.String())
			}
		})
	}
}

// completeLifecycleAction posts CompleteLifecycleAction for (asgName, hookName,
// instanceID) and requires it to succeed. AWS's CompleteLifecycleAction is a
// no-op (still 200) when no pending action matches, which the chain-ordering
// tests below rely on to prove a hook is NOT yet armed.
func completeLifecycleAction(t *testing.T, h *autoscaling.Handler, asgName, hookName, instanceID, result string) {
	t.Helper()

	code, body := doAS(t, h, "CompleteLifecycleAction", url.Values{
		"AutoScalingGroupName":  {asgName},
		"LifecycleHookName":     {hookName},
		"InstanceId":            {instanceID},
		"LifecycleActionResult": {result},
	})
	require.Equal(t, 200, code, body)
}

// soleInstanceState returns the LifecycleState and InstanceID of asgName's only
// instance, or ("", "") if the group has none.
func soleInstanceState(t *testing.T, h *autoscaling.Handler, asgName string) (string, string) {
	t.Helper()

	code, body := doAS(t, h, "DescribeAutoScalingGroups", url.Values{
		"AutoScalingGroupNames.member.1": {asgName},
	})
	require.Equal(t, 200, code, body)

	members := describeASGInstances(t, body).Result.AutoScalingGroups.Members[0].Instances.Members
	if len(members) == 0 {
		return "", ""
	}

	require.Len(t, members, 1)

	return members[0].LifecycleState, members[0].InstanceID
}

func putLaunchOrTerminateHook(t *testing.T, h *autoscaling.Handler, asgName, hookName, transition string) {
	t.Helper()

	code, body := doAS(t, h, "PutLifecycleHook", url.Values{
		"AutoScalingGroupName": {asgName},
		"LifecycleHookName":    {hookName},
		"LifecycleTransition":  {transition},
		"HeartbeatTimeout":     {"60"},
	})
	require.Equal(t, 200, code, body)
}

// TestAutoscalingHandler_LifecycleHookChainOrdering verifies that two hooks on
// the same transition form an ordered chain: the second is only armed once the
// first resolves with CONTINUE, and the transition only completes once both
// have. Before the chain fix, PutLifecycleHook's second registration on a
// transition was armed by nothing -- CompleteLifecycleAction/heartbeat timeout
// for it would never find a pending action, so a caller relying on it for a
// signal got silence forever.
func TestAutoscalingHandler_LifecycleHookChainOrdering(t *testing.T) {
	t.Parallel()

	cases := []struct {
		arm         func(t *testing.T, h *autoscaling.Handler) (asgName, instanceID string)
		assertFinal func(t *testing.T, h *autoscaling.Handler, asgName string)
		name        string
		waitState   string
	}{
		{
			name:      "launching",
			waitState: "Pending:Wait",
			arm: func(t *testing.T, h *autoscaling.Handler) (string, string) {
				t.Helper()

				asgName := "chain-order-launch"
				code, body := doAS(t, h, "CreateAutoScalingGroup", url.Values{
					"AutoScalingGroupName":       {asgName},
					"MinSize":                    {"0"},
					"MaxSize":                    {"5"},
					"AvailabilityZones.member.1": {"us-east-1a"},
				})
				require.Equal(t, 200, code, body)

				putLaunchOrTerminateHook(t, h, asgName, "hook-1", "autoscaling:EC2_INSTANCE_LAUNCHING")
				putLaunchOrTerminateHook(t, h, asgName, "hook-2", "autoscaling:EC2_INSTANCE_LAUNCHING")

				code, body = doAS(t, h, "SetDesiredCapacity", url.Values{
					"AutoScalingGroupName": {asgName},
					"DesiredCapacity":      {"1"},
				})
				require.Equal(t, 200, code, body)

				state, instanceID := soleInstanceState(t, h, asgName)
				require.Equal(t, "Pending:Wait", state)

				return asgName, instanceID
			},
			assertFinal: func(t *testing.T, h *autoscaling.Handler, asgName string) {
				t.Helper()

				state, _ := soleInstanceState(t, h, asgName)
				assert.Equal(t, "InService", state)
			},
		},
		{
			name:      "terminating",
			waitState: "Terminating:Wait",
			arm: func(t *testing.T, h *autoscaling.Handler) (string, string) {
				t.Helper()

				asgName := "chain-order-terminate"
				code, body := doAS(t, h, "CreateAutoScalingGroup", url.Values{
					"AutoScalingGroupName":       {asgName},
					"MinSize":                    {"0"},
					"MaxSize":                    {"5"},
					"DesiredCapacity":            {"1"},
					"AvailabilityZones.member.1": {"us-east-1a"},
				})
				require.Equal(t, 200, code, body)

				_, instanceID := soleInstanceState(t, h, asgName)

				putLaunchOrTerminateHook(t, h, asgName, "hook-1", "autoscaling:EC2_INSTANCE_TERMINATING")
				putLaunchOrTerminateHook(t, h, asgName, "hook-2", "autoscaling:EC2_INSTANCE_TERMINATING")

				code, body = doAS(t, h, "TerminateInstanceInAutoScalingGroup", url.Values{
					"InstanceId":                     {instanceID},
					"ShouldDecrementDesiredCapacity": {"true"},
				})
				require.Equal(t, 200, code, body)

				state, _ := soleInstanceState(t, h, asgName)
				require.Equal(t, "Terminating:Wait", state)

				return asgName, instanceID
			},
			assertFinal: func(t *testing.T, h *autoscaling.Handler, asgName string) {
				t.Helper()

				state, _ := soleInstanceState(t, h, asgName)
				assert.Empty(t, state, "instance must be removed once the terminating chain is exhausted")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newAutoscalingHandler()
			asgName, instanceID := tc.arm(t, h)

			// hook-2 is not armed yet -- completing it now must be a no-op.
			completeLifecycleAction(t, h, asgName, "hook-2", instanceID, "CONTINUE")
			state, _ := soleInstanceState(t, h, asgName)
			assert.Equal(t, tc.waitState, state, "hook-2 completing early must not advance the chain")

			// hook-1 resolves and re-arms hook-2 -- the chain is not finished yet.
			completeLifecycleAction(t, h, asgName, "hook-1", instanceID, "CONTINUE")
			state, _ = soleInstanceState(t, h, asgName)
			assert.Equal(t, tc.waitState, state, "hook-1 CONTINUE must re-arm hook-2, not finish the transition")

			// hook-1 already resolved -- completing it again must be a no-op.
			completeLifecycleAction(t, h, asgName, "hook-1", instanceID, "CONTINUE")
			state, _ = soleInstanceState(t, h, asgName)
			assert.Equal(t, tc.waitState, state, "re-completing hook-1 must not skip ahead")

			// hook-2 resolves -- the chain is exhausted, so the transition completes.
			completeLifecycleAction(t, h, asgName, "hook-2", instanceID, "CONTINUE")
			tc.assertFinal(t, h, asgName)
		})
	}
}

// launchChainAbandonShortCircuits uses a three-hook launching chain: hook-1
// CONTINUE must first advance the chain to hook-2 (the same chain-advance
// TestAutoscalingHandler_LifecycleHookChainOrdering covers -- asserted again
// here so this test cannot pass merely because the pre-fix code arms only
// hook-1), and then ABANDON on hook-2 must terminate-and-replace the instance
// (b7d3a8485's terminate-and-replace disposition) without ever arming hook-3.
// The replacement -- a new instance -- restarts the chain at hook-1.
func launchChainAbandonShortCircuits(t *testing.T) {
	t.Helper()

	h := newAutoscalingHandler()
	asgName := "chain-abandon-launch"

	code, body := doAS(t, h, "CreateAutoScalingGroup", url.Values{
		"AutoScalingGroupName":       {asgName},
		"MinSize":                    {"0"},
		"MaxSize":                    {"5"},
		"AvailabilityZones.member.1": {"us-east-1a"},
	})
	require.Equal(t, 200, code, body)

	for _, hookName := range []string{"hook-1", "hook-2", "hook-3"} {
		putLaunchOrTerminateHook(t, h, asgName, hookName, "autoscaling:EC2_INSTANCE_LAUNCHING")
	}

	code, body = doAS(t, h, "SetDesiredCapacity", url.Values{
		"AutoScalingGroupName": {asgName},
		"DesiredCapacity":      {"1"},
	})
	require.Equal(t, 200, code, body)

	state, originalID := soleInstanceState(t, h, asgName)
	require.Equal(t, "Pending:Wait", state)

	completeLifecycleAction(t, h, asgName, "hook-1", originalID, "CONTINUE")
	state, _ = soleInstanceState(t, h, asgName)
	require.Equal(t, "Pending:Wait", state, "hook-1 CONTINUE must re-arm hook-2 before ABANDON is even relevant")

	completeLifecycleAction(t, h, asgName, "hook-2", originalID, "ABANDON")

	state, replacementID := soleInstanceState(t, h, asgName)
	require.NotEmpty(t, replacementID, "ABANDON on hook-2 must terminate-and-replace, not leave the group short")
	assert.NotEqual(t, originalID, replacementID, "the replacement must be a new instance")
	assert.Equal(t, "Pending:Wait", state, "the replacement is itself gated by the launch chain")

	// hook-3 must never have been armed for the abandoned instance: ABANDON on
	// hook-2 short-circuits it. The replacement restarts the chain at hook-1, so
	// completing hook-3 now is still a no-op.
	completeLifecycleAction(t, h, asgName, "hook-3", replacementID, "CONTINUE")
	state, _ = soleInstanceState(t, h, asgName)
	assert.Equal(t, "Pending:Wait", state, "the replacement's chain starts at hook-1, not hook-3")
}

// terminateChainAbandonShortCircuits uses a three-hook terminating chain:
// hook-1 CONTINUE must first advance the chain to hook-2 (asserted here too,
// so this test cannot pass merely because the pre-fix code arms only hook-1),
// and then ABANDON on hook-2 must finish the termination immediately, without
// ever arming hook-3.
func terminateChainAbandonShortCircuits(t *testing.T) {
	t.Helper()

	h := newAutoscalingHandler()
	asgName := "chain-abandon-terminate"

	code, body := doAS(t, h, "CreateAutoScalingGroup", url.Values{
		"AutoScalingGroupName":       {asgName},
		"MinSize":                    {"0"},
		"MaxSize":                    {"5"},
		"DesiredCapacity":            {"1"},
		"AvailabilityZones.member.1": {"us-east-1a"},
	})
	require.Equal(t, 200, code, body)

	_, instanceID := soleInstanceState(t, h, asgName)

	for _, hookName := range []string{"hook-1", "hook-2", "hook-3"} {
		putLaunchOrTerminateHook(t, h, asgName, hookName, "autoscaling:EC2_INSTANCE_TERMINATING")
	}

	code, body = doAS(t, h, "TerminateInstanceInAutoScalingGroup", url.Values{
		"InstanceId":                     {instanceID},
		"ShouldDecrementDesiredCapacity": {"true"},
	})
	require.Equal(t, 200, code, body)

	completeLifecycleAction(t, h, asgName, "hook-1", instanceID, "CONTINUE")
	state, _ := soleInstanceState(t, h, asgName)
	require.Equal(t, "Terminating:Wait", state, "hook-1 CONTINUE must re-arm hook-2 before ABANDON is even relevant")

	completeLifecycleAction(t, h, asgName, "hook-2", instanceID, "ABANDON")

	state, _ = soleInstanceState(t, h, asgName)
	assert.Empty(t, state, "ABANDON on hook-2 must finish termination immediately, without waiting on hook-3")
}

func TestAutoscalingHandler_LifecycleHookChainAbandonShortCircuits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		run  func(t *testing.T)
		name string
	}{
		{name: "launching", run: launchChainAbandonShortCircuits},
		{name: "terminating", run: terminateChainAbandonShortCircuits},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.run(t)
		})
	}
}

// TestAutoscalingHandler_LifecycleHookChainResumesAfterRestore verifies that a
// group snapshotted mid-chain (past hook-1, waiting on hook-2) resumes at
// hook-2 after Restore, instead of restarting the chain at hook-1 or leaving
// the instance stuck forever (in-flight timers are never persisted -- see
// rearmPendingWaits).
func TestAutoscalingHandler_LifecycleHookChainResumesAfterRestore(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	b := autoscaling.NewInMemoryBackend()
	h := autoscaling.NewHandler(b)
	asgName := "chain-restore-asg"

	code, body := doAS(t, h, "CreateAutoScalingGroup", url.Values{
		"AutoScalingGroupName":       {asgName},
		"MinSize":                    {"0"},
		"MaxSize":                    {"5"},
		"AvailabilityZones.member.1": {"us-east-1a"},
	})
	require.Equal(t, 200, code, body)

	putLaunchOrTerminateHook(t, h, asgName, "hook-1", "autoscaling:EC2_INSTANCE_LAUNCHING")
	putLaunchOrTerminateHook(t, h, asgName, "hook-2", "autoscaling:EC2_INSTANCE_LAUNCHING")

	code, body = doAS(t, h, "SetDesiredCapacity", url.Values{
		"AutoScalingGroupName": {asgName},
		"DesiredCapacity":      {"1"},
	})
	require.Equal(t, 200, code, body)

	state, instanceID := soleInstanceState(t, h, asgName)
	require.Equal(t, "Pending:Wait", state)

	// Advance past hook-1 so the group is paused mid-chain, on hook-2, before
	// snapshotting.
	completeLifecycleAction(t, h, asgName, "hook-1", instanceID, "CONTINUE")
	state, _ = soleInstanceState(t, h, asgName)
	require.Equal(t, "Pending:Wait", state, "must still be mid-chain, waiting on hook-2, before snapshotting")

	snap := b.Snapshot(ctx)
	require.NotEmpty(t, snap)

	restored := autoscaling.NewInMemoryBackend()
	require.NoError(t, restored.Restore(ctx, snap))
	rh := autoscaling.NewHandler(restored)

	// hook-1 already resolved before the snapshot: restarting the chain at
	// hook-1 would be wrong, so completing it again post-restore must be a no-op.
	completeLifecycleAction(t, rh, asgName, "hook-1", instanceID, "CONTINUE")
	state, _ = soleInstanceState(t, rh, asgName)
	assert.Equal(t, "Pending:Wait", state, "restore must resume at hook-2, not restart the chain at hook-1")

	// hook-2 is the actually-pending hook: completing it resolves the wait.
	completeLifecycleAction(t, rh, asgName, "hook-2", instanceID, "CONTINUE")
	state, _ = soleInstanceState(t, rh, asgName)
	assert.Equal(t, "InService", state, "restore must not drop the pending wait")
}

// TestAutoscalingHandler_DeleteLifecycleHookResolvesOutstandingAction verifies
// that deleting a hook with an instance still waiting on it completes that
// action instead of stranding the instance forever. api_op_DeleteLifecycleHook.go:
// "If there are any outstanding lifecycle actions, they are completed first
// (ABANDON for launching instances, CONTINUE for terminating instances)."
// Before this fix, DeleteLifecycleHook's cleanupHookTimers only stopped the
// timer and dropped the pending action -- no hook left to complete it, no
// timer left to time it out, leaving the instance in Pending:Wait/
// Terminating:Wait permanently.
func TestAutoscalingHandler_DeleteLifecycleHookResolvesOutstandingAction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		transition string
	}{
		{
			name:       "launching hook abandon terminates and replaces",
			transition: "autoscaling:EC2_INSTANCE_LAUNCHING",
		},
		{
			name:       "terminating hook continue finishes removal",
			transition: "autoscaling:EC2_INSTANCE_TERMINATING",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newAutoscalingHandler()
			asgName := "delete-hook-outstanding-" + tc.transition

			code, body := doAS(t, h, "CreateAutoScalingGroup", url.Values{
				"AutoScalingGroupName":       {asgName},
				"MinSize":                    {"0"},
				"MaxSize":                    {"5"},
				"AvailabilityZones.member.1": {"us-east-1a"},
			})
			require.Equal(t, 200, code, body)

			putLaunchOrTerminateHook(t, h, asgName, "hook", tc.transition)

			var waitingInstanceID string

			if tc.transition == "autoscaling:EC2_INSTANCE_LAUNCHING" {
				code, body = doAS(t, h, "SetDesiredCapacity", url.Values{
					"AutoScalingGroupName": {asgName},
					"DesiredCapacity":      {"1"},
				})
				require.Equal(t, 200, code, body)

				state, id := soleInstanceState(t, h, asgName)
				require.Equal(t, "Pending:Wait", state)
				waitingInstanceID = id
			} else {
				code, body = doAS(t, h, "SetDesiredCapacity", url.Values{
					"AutoScalingGroupName": {asgName},
					"DesiredCapacity":      {"1"},
				})
				require.Equal(t, 200, code, body)

				_, waitingInstanceID = soleInstanceState(t, h, asgName)

				code, body = doAS(t, h, "TerminateInstanceInAutoScalingGroup", url.Values{
					"InstanceId":                     {waitingInstanceID},
					"ShouldDecrementDesiredCapacity": {"true"},
				})
				require.Equal(t, 200, code, body)

				state, _ := soleInstanceState(t, h, asgName)
				require.Equal(t, "Terminating:Wait", state)
			}

			code, body = doAS(t, h, "DeleteLifecycleHook", url.Values{
				"AutoScalingGroupName": {asgName},
				"LifecycleHookName":    {"hook"},
			})
			require.Equal(t, 200, code, body)

			state, id := soleInstanceState(t, h, asgName)

			if tc.transition == "autoscaling:EC2_INSTANCE_LAUNCHING" {
				assert.Equal(t, "InService", state,
					"abandoning the launch on hook deletion must replace the instance, not leave it Pending:Wait")
				assert.NotEqual(t, waitingInstanceID, id, "the replacement must be a new instance")
			} else {
				assert.Empty(t, state,
					"continuing the termination on hook deletion must actually remove the instance, "+
						"not leave it Terminating:Wait")
			}
		})
	}
}

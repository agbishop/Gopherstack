package iot_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	iotsdk "github.com/aws/aws-sdk-go-v2/service/iot"
	iottypes "github.com/aws/aws-sdk-go-v2/service/iot/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/iot"
)

// TestBatch3_SecurityProfileTargets tests DetachSecurityProfile,
// ListTargetsForSecurityProfile, ListSecurityProfilesForTarget.
func TestSecurityProfileTargets(t *testing.T) {
	t.Parallel()
	h, b := newHandlerForBatch3Test(t)

	profileName := "test-profile"
	targetARN := "arn:aws:iot:us-east-1:000000000000:thinggroup/my-group"

	// AttachSecurityProfile requires the security profile to already exist
	// (real AWS IoT returns ResourceNotFoundException otherwise).
	if _, err := b.CreateSecurityProfile(&iot.CreateSecurityProfileInput{
		SecurityProfileName: profileName,
	}); err != nil {
		t.Fatal(err)
	}

	// Attach via backend directly (AttachSecurityProfile already implemented)
	if err := b.AttachSecurityProfile(&iot.AttachSecurityProfileInput{
		SecurityProfileName:      profileName,
		SecurityProfileTargetArn: targetARN,
	}); err != nil {
		t.Fatal(err)
	}

	// List targets for profile
	out := iotOK(t, h, http.MethodGet, "/security-profiles/"+profileName+"/targets", nil)
	targets, _ := out["securityProfileTargets"].([]any)
	if len(targets) != 1 {
		t.Errorf("expected 1 target, got %d", len(targets))
	}

	// List profiles for target
	out2 := iotOK(t, h, http.MethodGet, "/security-profiles-for-target?securityProfileTargetArn="+targetARN, nil)
	mappings, _ := out2["securityProfileTargetMappings"].([]any)
	if len(mappings) != 1 {
		t.Errorf("expected 1 mapping, got %d", len(mappings))
	}

	// Detach
	iotOK(
		t,
		h,
		http.MethodDelete,
		"/security-profiles/"+profileName+"/targets?securityProfileTargetArn="+targetARN,
		nil,
	)

	// Verify detached
	out3 := iotOK(t, h, http.MethodGet, "/security-profiles/"+profileName+"/targets", nil)
	targets2, _ := out3["securityProfileTargets"].([]any)
	if len(targets2) != 0 {
		t.Errorf("expected 0 targets after detach, got %d", len(targets2))
	}
}

// TestNewOps_SecurityProfile tests SecurityProfile CRUD.
func TestSecurityProfile(t *testing.T) {
	t.Parallel()
	h := newIoTHandlerBatch1(t)

	// CreateSecurityProfile
	out := iotOK(t, h, http.MethodPost, "/security-profiles/my-profile", map[string]any{
		"securityProfileDescription": "test profile",
	})
	if out["securityProfileName"] != "my-profile" {
		t.Errorf("name mismatch: %v", out)
	}

	// DescribeSecurityProfile
	out2 := iotOK(t, h, http.MethodGet, "/security-profiles/my-profile", nil)
	if out2["securityProfileName"] != "my-profile" {
		t.Errorf("describe mismatch: %v", out2)
	}

	// ListSecurityProfiles
	out3 := iotOK(t, h, http.MethodGet, "/security-profiles", nil)
	profiles, _ := out3["securityProfileIdentifiers"].([]any)
	if len(profiles) != 1 {
		t.Errorf("expected 1 profile, got %d", len(profiles))
	}

	// UpdateSecurityProfile
	out4 := iotOK(t, h, http.MethodPatch, "/security-profiles/my-profile", map[string]any{
		"securityProfileDescription": "updated",
	})
	if out4["version"] == nil {
		t.Error("expected version in update response")
	}

	// DeleteSecurityProfile
	iotOK(t, h, http.MethodDelete, "/security-profiles/my-profile", nil)

	iotExpectError(t, h, "/security-profiles/my-profile")
}

// TestSecurityProfile_FullFieldsAndUpdateSemantics covers this pass's
// security_profiles gap closure: CreateSecurityProfile previously silently
// dropped Behaviors/AlertTargets/AdditionalMetricsToRetain(V2)/
// MetricsExportConfig entirely (types.CreateSecurityProfileInput field-diff,
// v1.77.4). Also covers UpdateSecurityProfile's ExpectedVersion optimistic
// lock and DeleteX-flag-vs-field mutual exclusion, both previously
// unmodeled (UpdateSecurityProfile only ever accepted a description).
func TestSecurityProfile_FullFieldsAndUpdateSemantics(t *testing.T) {
	t.Parallel()
	h, b := newHandlerForBatch3Test(t)

	name := "profile-full"
	create := iotOK(t, h, http.MethodPost, "/security-profiles/"+name, map[string]any{
		"securityProfileDescription": "full profile",
		"behaviors": []map[string]any{
			{
				"name":   "excessive-connects",
				"metric": "aws:num-connections",
				"criteria": map[string]any{
					"comparisonOperator": "greater-than",
					"durationSeconds":    300,
					"value":              map[string]any{"count": 10},
				},
			},
		},
		"alertTargets": map[string]any{
			"SNS": map[string]any{
				"alertTargetArn": "arn:aws:sns:us-east-1:000000000000:alerts",
				"roleArn":        "arn:aws:iam::000000000000:role/AlertRole",
			},
		},
		"additionalMetricsToRetainV2": []map[string]any{
			{"metric": "aws:num-messages-sent"},
		},
		"metricsExportConfig": map[string]any{
			"mqttTopic": "$aws/things/foo/metrics",
			"roleArn":   "arn:aws:iam::000000000000:role/ExportRole",
		},
	})
	if create["securityProfileName"] != name {
		t.Fatalf("create mismatch: %v", create)
	}

	describe := iotOK(t, h, http.MethodGet, "/security-profiles/"+name, nil)
	behaviors, _ := describe["behaviors"].([]any)
	if len(behaviors) != 1 {
		t.Fatalf("expected 1 behavior on describe, got %d: %v", len(behaviors), describe)
	}
	beh, _ := behaviors[0].(map[string]any)
	if beh["name"] != "excessive-connects" {
		t.Errorf("unexpected behavior: %v", beh)
	}
	alertTargets, _ := describe["alertTargets"].(map[string]any)
	if _, ok := alertTargets["SNS"]; !ok {
		t.Errorf("expected SNS alertTarget, got %v", alertTargets)
	}
	metricsToRetain, _ := describe["additionalMetricsToRetainV2"].([]any)
	if len(metricsToRetain) != 1 {
		t.Errorf("expected 1 additionalMetricsToRetainV2 entry, got %v", describe)
	}
	metricsExport, _ := describe["metricsExportConfig"].(map[string]any)
	if metricsExport["mqttTopic"] != "$aws/things/foo/metrics" {
		t.Errorf("unexpected metricsExportConfig: %v", metricsExport)
	}
	// Real DescribeSecurityProfileOutput has no "tags" field at all.
	if _, hasTags := describe["tags"]; hasTags {
		t.Errorf("tags must not leak on DescribeSecurityProfile output, got %v", describe)
	}

	// Setting a Delete* flag alongside the corresponding field in the same
	// UpdateSecurityProfile call is InvalidRequestException.
	conflictRec := iotRequest(t, h, http.MethodPatch, "/security-profiles/"+name, map[string]any{
		"deleteBehaviors": true,
		"behaviors":       []map[string]any{{"name": "x", "criteria": map[string]any{}}},
	})
	if conflictRec.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected 400 for deleteBehaviors+behaviors conflict, got %d: %s",
			conflictRec.Code, conflictRec.Body.String(),
		)
	}

	// ExpectedVersion mismatch -> VersionConflictException.
	mismatchRec := iotRequest(t, h, http.MethodPatch, "/security-profiles/"+name+"?expectedVersion=999", map[string]any{
		"securityProfileDescription": "won't apply",
	})
	if mismatchRec.Code != http.StatusConflict {
		t.Fatalf(
			"expected 409 for expectedVersion mismatch, got %d: %s", mismatchRec.Code, mismatchRec.Body.String(),
		)
	}

	// Correct ExpectedVersion + deleteBehaviors actually clears Behaviors.
	updateOut := iotOK(t, h, http.MethodPatch, "/security-profiles/"+name+"?expectedVersion=1", map[string]any{
		"deleteBehaviors": true,
	})
	if _, hasBehaviors := updateOut["behaviors"]; hasBehaviors {
		t.Errorf("expected behaviors cleared after deleteBehaviors, got %v", updateOut)
	}
	if v, _ := updateOut["version"].(float64); v != 2 {
		t.Errorf("expected version=2 after update, got %v", updateOut["version"])
	}

	got, err := b.DescribeSecurityProfile(name)
	if err != nil {
		t.Fatal(err)
	}
	if got.Behaviors != nil {
		t.Errorf("expected Behaviors nil after deleteBehaviors, got %v", got.Behaviors)
	}
	if got.AlertTargets == nil {
		t.Errorf("expected AlertTargets untouched by deleteBehaviors, got nil")
	}
}

func TestValidateSecurityProfileBehaviors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		behaviors  []map[string]any
		wantValid  bool
		wantStatus int
	}{
		{
			name: "valid_behavior",
			behaviors: []map[string]any{
				{
					"name":   "excessive-connects",
					"metric": "aws:num-connections",
					"criteria": map[string]any{
						"comparisonOperator": "greater-than",
						"durationSeconds":    300,
					},
				},
			},
			wantValid:  true,
			wantStatus: http.StatusOK,
		},
		{
			name: "missing_name",
			behaviors: []map[string]any{
				{
					"criteria": map[string]any{"comparisonOperator": "greater-than"},
				},
			},
			wantValid:  false,
			wantStatus: http.StatusOK,
		},
		{
			name: "invalid_comparison_operator",
			behaviors: []map[string]any{
				{
					"name":     "bad-behavior",
					"criteria": map[string]any{"comparisonOperator": "not-a-real-operator"},
				},
			},
			wantValid:  false,
			wantStatus: http.StatusOK,
		},
		{
			name: "missing_criteria",
			behaviors: []map[string]any{
				{"name": "no-criteria"},
			},
			wantValid:  false,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, _ := newRefHandler()

			rec := doRefRequest(t, h, http.MethodPost, "/security-profile-behaviors/validate", map[string]any{
				"behaviors": tt.behaviors,
			}, nil)

			require.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantValid {
				assert.Contains(t, rec.Body.String(), `"valid":true`)
			} else {
				assert.Contains(t, rec.Body.String(), `"valid":false`)
			}
		})
	}
}

// TestSecurityProfile_RoutingWireShapesAndBehaviorCriteriaType_SDKRoundTrip
// drives a real generated AWS SDK v2 IoT client through the actual
// service.Router path (newIoTSDKClient), not h.Handler() directly — the
// only way to prove reachability through RouteMatcher.
//
// One case per real types.BehaviorCriteriaType value (STATIC/STATISTICAL/
// MACHINE_LEARNING). Each case proves, in one round trip: (1) ListSecurityProfiles
// is reachable and uses the real "name"/"arn" SecurityProfileIdentifier keys;
// (2) ListSecurityProfilesForTarget is reachable and nests
// securityProfileIdentifier{name,arn}+target{arn}; (3) ListTargetsForSecurityProfile
// uses the real "arn" key; (4) the behaviorCriteriaType filter on
// ListActiveViolations resolves each violation's BehaviorCriteriaType from
// the owning security profile's stored Behavior.
func TestSecurityProfile_RoutingWireShapesAndBehaviorCriteriaType_SDKRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		criteria *iottypes.BehaviorCriteria
		name     string
		wantType iottypes.BehaviorCriteriaType
	}{
		{
			name: "static",
			criteria: &iottypes.BehaviorCriteria{
				ComparisonOperator: iottypes.ComparisonOperatorGreaterThan,
				Value:              &iottypes.MetricValue{Count: aws.Int64(10)},
			},
			wantType: iottypes.BehaviorCriteriaTypeStatic,
		},
		{
			name: "statistical",
			criteria: &iottypes.BehaviorCriteria{
				StatisticalThreshold: &iottypes.StatisticalThreshold{Statistic: aws.String("p90")},
			},
			wantType: iottypes.BehaviorCriteriaTypeStatistical,
		},
		{
			name: "machine_learning",
			criteria: &iottypes.BehaviorCriteria{
				MlDetectionConfig: &iottypes.MachineLearningDetectionConfig{
					ConfidenceLevel: iottypes.ConfidenceLevelHigh,
				},
			},
			wantType: iottypes.BehaviorCriteriaTypeMachineLearning,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client, b := newIoTSDKClient(t)
			ctx := t.Context()

			profileName := "profile-" + tt.name
			behaviorName := "behavior-" + tt.name
			targetARN := "arn:aws:iot:us-east-1:000000000000:thinggroup/group-" + tt.name

			_, err := client.CreateSecurityProfile(ctx, &iotsdk.CreateSecurityProfileInput{
				SecurityProfileName: aws.String(profileName),
				Behaviors: []iottypes.Behavior{{
					Name:     aws.String(behaviorName),
					Metric:   aws.String("aws:num-connections"),
					Criteria: tt.criteria,
				}},
			})
			require.NoError(t, err)

			_, err = client.AttachSecurityProfile(ctx, &iotsdk.AttachSecurityProfileInput{
				SecurityProfileName:      aws.String(profileName),
				SecurityProfileTargetArn: aws.String(targetARN),
			})
			require.NoError(t, err)

			// (1) ListSecurityProfiles: reachable + real name/arn keys.
			listOut, err := client.ListSecurityProfiles(ctx, &iotsdk.ListSecurityProfilesInput{})
			require.NoError(t, err)
			require.Len(t, listOut.SecurityProfileIdentifiers, 1)
			assert.Equal(t, profileName, aws.ToString(listOut.SecurityProfileIdentifiers[0].Name))
			assert.NotEmpty(t, aws.ToString(listOut.SecurityProfileIdentifiers[0].Arn))

			// (2) ListSecurityProfilesForTarget: reachable + real nested shape.
			forTargetOut, err := client.ListSecurityProfilesForTarget(ctx, &iotsdk.ListSecurityProfilesForTargetInput{
				SecurityProfileTargetArn: aws.String(targetARN),
			})
			require.NoError(t, err)
			require.Len(t, forTargetOut.SecurityProfileTargetMappings, 1)
			mapping := forTargetOut.SecurityProfileTargetMappings[0]
			require.NotNil(t, mapping.SecurityProfileIdentifier)
			assert.Equal(t, profileName, aws.ToString(mapping.SecurityProfileIdentifier.Name))
			assert.NotEmpty(t, aws.ToString(mapping.SecurityProfileIdentifier.Arn))
			require.NotNil(t, mapping.Target)
			assert.Equal(t, targetARN, aws.ToString(mapping.Target.Arn))

			// (3) ListTargetsForSecurityProfile: real "arn" key.
			targetsOut, err := client.ListTargetsForSecurityProfile(ctx, &iotsdk.ListTargetsForSecurityProfileInput{
				SecurityProfileName: aws.String(profileName),
			})
			require.NoError(t, err)
			require.Len(t, targetsOut.SecurityProfileTargets, 1)
			assert.Equal(t, targetARN, aws.ToString(targetsOut.SecurityProfileTargets[0].Arn))

			// (4) behaviorCriteriaType filter: seed a matching violation and
			// a "control" violation for an unrelated, never-stored behavior
			// (which must never match a non-empty filter value).
			_, err = b.SeedActiveViolation(&iot.SeedActiveViolationInput{
				ViolationID:         "viol-" + tt.name,
				ThingName:           "thing-" + tt.name,
				SecurityProfileName: profileName,
				Behavior:            &iot.ViolationBehavior{Name: behaviorName, Metric: "aws:num-connections"},
			})
			require.NoError(t, err)
			_, err = b.SeedActiveViolation(&iot.SeedActiveViolationInput{
				ViolationID:         "viol-control-" + tt.name,
				ThingName:           "thing-" + tt.name,
				SecurityProfileName: profileName,
				Behavior:            &iot.ViolationBehavior{Name: "unrelated-behavior", Metric: "aws:num-connections"},
			})
			require.NoError(t, err)

			activeOut, err := client.ListActiveViolations(ctx, &iotsdk.ListActiveViolationsInput{
				BehaviorCriteriaType: tt.wantType,
			})
			require.NoError(t, err)

			gotIDs := make([]string, 0, len(activeOut.ActiveViolations))
			for _, v := range activeOut.ActiveViolations {
				gotIDs = append(gotIDs, aws.ToString(v.ViolationId))
			}
			assert.Equal(t, []string{"viol-" + tt.name}, gotIDs)

			// ListViolationEvents shares the same filter implementation and
			// underlying securityProfileBehaviorCriteriaTypeLocked lookup;
			// SeedActiveViolation also records a matching ViolationEvent for
			// each seeded violation (see device_defender.go).
			eventsOut, err := client.ListViolationEvents(ctx, &iotsdk.ListViolationEventsInput{
				StartTime:            aws.Time(time.Unix(0, 0)),
				EndTime:              aws.Time(time.Now().Add(time.Hour)),
				BehaviorCriteriaType: tt.wantType,
			})
			require.NoError(t, err)

			gotEventIDs := make([]string, 0, len(eventsOut.ViolationEvents))
			for _, e := range eventsOut.ViolationEvents {
				gotEventIDs = append(gotEventIDs, aws.ToString(e.ViolationId))
			}
			assert.Equal(t, []string{"viol-" + tt.name}, gotEventIDs)
		})
	}
}

// TestSecurityProfile_DetachNotFoundAndDeleteCascade covers: DetachSecurityProfile
// returns ResourceNotFoundException for an unknown profile name instead of
// silently no-op'ing; DeleteSecurityProfile cleans up the profile's target
// attachments so a re-created profile with the same name doesn't inherit
// the old ghost row in securityProfileTargets.
// DeleteSecurityProfileInput.ExpectedVersion (iot@v1.77.4/api_op_DeleteSecurityProfile.go:38-41):
// "If you specify a value that is different from the actual version, a
// VersionConflictException is thrown." expectedVersion is a QUERY parameter
// (awsRestjson1_serializeOpHttpBindingsDeleteSecurityProfileInput).
func TestDeleteSecurityProfileExpectedVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		expectedVer int
		wantStatus  int
	}{
		{"matching_version_succeeds", 1, http.StatusOK},
		{"stale_version_conflicts", 99, http.StatusConflict},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newIoTHandlerBatch1(t)
			iotOK(t, h, http.MethodPost, "/security-profiles/my-profile", map[string]any{})

			path := fmt.Sprintf("/security-profiles/my-profile?expectedVersion=%d", tt.expectedVer)
			rec := iotRequest(t, h, http.MethodDelete, path, nil)

			require.Equal(t, tt.wantStatus, rec.Code, "body: %s", rec.Body.String())

			if tt.wantStatus == http.StatusOK {
				iotExpectError(t, h, "/security-profiles/my-profile")
			} else {
				iotOK(t, h, http.MethodGet, "/security-profiles/my-profile", nil)
			}
		})
	}
}

func TestSecurityProfile_DetachNotFoundAndDeleteCascade(t *testing.T) {
	t.Parallel()

	t.Run("detach_unknown_profile_returns_not_found", func(t *testing.T) {
		t.Parallel()

		client, _ := newIoTSDKClient(t)
		ctx := t.Context()

		_, err := client.DetachSecurityProfile(ctx, &iotsdk.DetachSecurityProfileInput{
			SecurityProfileName:      aws.String("no-such-profile"),
			SecurityProfileTargetArn: aws.String("arn:aws:iot:us-east-1:000000000000:thinggroup/g"),
		})
		var nfe *iottypes.ResourceNotFoundException
		require.ErrorAs(t, err, &nfe)
	})

	t.Run("delete_cascades_target_attachments", func(t *testing.T) {
		t.Parallel()

		client, _ := newIoTSDKClient(t)
		ctx := t.Context()

		name := "recreated-profile"
		targetARN := "arn:aws:iot:us-east-1:000000000000:thinggroup/g"

		_, err := client.CreateSecurityProfile(
			ctx,
			&iotsdk.CreateSecurityProfileInput{SecurityProfileName: aws.String(name)},
		)
		require.NoError(t, err)
		_, err = client.AttachSecurityProfile(ctx, &iotsdk.AttachSecurityProfileInput{
			SecurityProfileName:      aws.String(name),
			SecurityProfileTargetArn: aws.String(targetARN),
		})
		require.NoError(t, err)

		_, err = client.DeleteSecurityProfile(
			ctx,
			&iotsdk.DeleteSecurityProfileInput{SecurityProfileName: aws.String(name)},
		)
		require.NoError(t, err)

		// Re-create a profile with the same name and verify it starts with
		// NO target attachments -- before the cascade-delete fix, the old
		// attachment would still be present here as a ghost row.
		_, err = client.CreateSecurityProfile(
			ctx,
			&iotsdk.CreateSecurityProfileInput{SecurityProfileName: aws.String(name)},
		)
		require.NoError(t, err)

		targetsOut, err := client.ListTargetsForSecurityProfile(ctx, &iotsdk.ListTargetsForSecurityProfileInput{
			SecurityProfileName: aws.String(name),
		})
		require.NoError(t, err)
		assert.Empty(t, targetsOut.SecurityProfileTargets)
	})
}

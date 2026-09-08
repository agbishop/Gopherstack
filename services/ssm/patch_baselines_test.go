package ssm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/ssm"
)

// TestDescribeAvailablePatches verifies that the available patches catalog is returned.
func TestDescribeAvailablePatches(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		wantName  string
		seed      []ssm.Patch
		wantCount int
	}{
		{
			name:      "empty catalog returns empty list",
			seed:      nil,
			wantCount: 0,
		},
		{
			name: "returns seeded patches",
			seed: []ssm.Patch{
				{Name: "CVE-2024-001", Product: "Windows", Classification: "SecurityUpdates", Severity: "Critical"},
				{Name: "CVE-2024-002", Product: "Windows", Classification: "BugFix", Severity: "Medium"},
			},
			wantCount: 2,
			wantName:  "CVE-2024-001",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h, b := newTestHandler(t)
			for _, p := range tc.seed {
				b.AddAvailablePatchInternal(p)
			}

			rec := doRequest(t, h, "DescribeAvailablePatches", `{}`)
			require.Equal(t, http.StatusOK, rec.Code)
			assertBodyContains(t, rec, "Patches")

			if tc.wantName != "" {
				assertBodyContains(t, rec, tc.wantName)
			}
		})
	}
}

// TestDescribePatchGroupState verifies that patch group state is aggregated from instance patch states.
func TestDescribePatchGroupState(t *testing.T) {
	t.Parallel()

	now := ssm.UnixTimeFloat(time.Now())

	cases := []struct {
		name             string
		patchGroup       string
		wantBodyContains string
		seed             []ssm.InstancePatchState
		wantInstances    bool // true if Instances > 0 expected
	}{
		{
			name:             "empty store returns zero counts",
			seed:             nil,
			patchGroup:       "grp1",
			wantInstances:    false,
			wantBodyContains: "Instances",
		},
		{
			name: "aggregates counts for matching patch group",
			seed: []ssm.InstancePatchState{
				{InstanceID: "i-aaa", PatchGroup: "grp1", InstalledCount: 5, FailedCount: 1, OperationStartTime: now},
				{InstanceID: "i-bbb", PatchGroup: "grp1", InstalledCount: 3, MissingCount: 2, OperationStartTime: now},
				{InstanceID: "i-ccc", PatchGroup: "grp2", InstalledCount: 10, OperationStartTime: now},
			},
			patchGroup:       "grp1",
			wantInstances:    true,
			wantBodyContains: "grp1",
		},
		{
			name: "ignores instances from other patch groups",
			seed: []ssm.InstancePatchState{
				{InstanceID: "i-aaa", PatchGroup: "grp2", InstalledCount: 5, OperationStartTime: now},
			},
			patchGroup:       "grp1",
			wantInstances:    false,
			wantBodyContains: "Instances",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h, b := newTestHandler(t)
			for _, s := range tc.seed {
				b.AddInstancePatchStateInternal(s)
			}

			body := `{"PatchGroup":"` + tc.patchGroup + `"}`
			rec := doRequest(t, h, "DescribePatchGroupState", body)
			require.Equal(t, http.StatusOK, rec.Code)
			assertBodyContains(t, rec, "Instances")

			if tc.wantInstances {
				// The response should contain a non-zero Instances count
				assert.NotContains(t, rec.Body.String(), `"Instances":0`)
			}
		})
	}
}

func TestGetDefaultPatchBaseline_StableRealID(t *testing.T) {
	t.Parallel()

	b := ssm.NewInMemoryBackend()
	ctx := context.TODO()

	first, err := b.GetDefaultPatchBaseline(ctx, &ssm.GetDefaultPatchBaselineInput{
		OperatingSystem: "AMAZON_LINUX_2",
	})
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(first.BaselineID, "pb-"),
		"default baseline ID must be well-shaped")
	assert.NotEqual(t, "pb-00000000000000000", first.BaselineID,
		"must not return the fabricated all-zeros ID")

	second, err := b.GetDefaultPatchBaseline(ctx, &ssm.GetDefaultPatchBaselineInput{
		OperatingSystem: "AMAZON_LINUX_2",
	})
	require.NoError(t, err)
	assert.Equal(t, first.BaselineID, second.BaselineID,
		"default baseline ID must be stable for a given OS")

	// A registered default overrides the synthetic one.
	blID := createTestBaseline(t, b, "custom-baseline")
	_, err = b.RegisterDefaultPatchBaseline(ctx, &ssm.RegisterDefaultPatchBaselineInput{
		BaselineID: blID,
	})
	require.NoError(t, err)

	got, err := b.GetDefaultPatchBaseline(ctx, &ssm.GetDefaultPatchBaselineInput{
		OperatingSystem: "AMAZON_LINUX_2",
	})
	require.NoError(t, err)
	assert.Equal(t, blID, got.BaselineID,
		"a registered default must be returned over the synthetic one")
}

// TestDescribeEffectivePatches_FromApprovedAndCatalog previously asserted
// exactly 2 EffectivePatches (the baseline's own ApprovedPatches) with no
// catalogue entries at all -- that passed only because this test's fresh
// ssm.NewInMemoryBackend() never called DescribeAvailablePatches first, and
// effectivePatchesForBaseline used to read b.availablePatches[region]
// directly instead of the lazy-seeding availablePatchesFor helper
// DescribeAvailablePatches itself uses. A real client's DescribeEffective
// PatchesForPatchBaseline reflects the built-in catalogue regardless of call
// order; this test ratified that order-dependent gap and is corrected here.
func TestDescribeEffectivePatches_FromApprovedAndCatalog(t *testing.T) {
	t.Parallel()

	b := ssm.NewInMemoryBackend()
	ctx := context.TODO()

	create, err := b.CreatePatchBaseline(ctx, &ssm.CreatePatchBaselineInput{
		Name:                           "eff-baseline",
		OperatingSystem:                "AMAZON_LINUX_2",
		ApprovedPatches:                []string{"CVE-2024-0001", "CVE-2024-0002"},
		RejectedPatches:                []string{"ALAS2-2024-2460"},
		ApprovedPatchesComplianceLevel: "CRITICAL",
	})
	require.NoError(t, err)

	out, err := b.DescribeEffectivePatchesForPatchBaseline(
		ctx,
		&ssm.DescribeEffectivePatchesForPatchBaselineInput{BaselineID: create.BaselineID},
	)
	require.NoError(t, err)

	byName := make(map[string]ssm.EffectivePatch, len(out.EffectivePatches))
	for _, ep := range out.EffectivePatches {
		require.NotNil(t, ep.Patch)
		require.NotNil(t, ep.PatchStatus)
		byName[ep.Patch.Name] = ep
	}

	for _, name := range []string{"CVE-2024-0001", "CVE-2024-0002"} {
		ep, ok := byName[name]
		require.True(t, ok, "approved patch %s must be present", name)
		assert.Equal(t, "EXPLICIT_APPROVED", ep.PatchStatus.DeploymentStatus)
		assert.Equal(t, "CRITICAL", ep.PatchStatus.ComplianceLevel)
		assert.Greater(t, ep.PatchStatus.ApprovalDate, float64(0),
			"ApprovalDate must be a populated epoch-seconds value for an explicitly approved patch")
	}

	rejected, ok := byName["ALAS2-2024-2460"]
	require.True(t, ok, "explicitly rejected patch must still appear in the effective set")
	assert.Equal(t, "EXPLICIT_REJECTED", rejected.PatchStatus.DeploymentStatus)

	// The built-in catalogue's other patches show up pending a decision,
	// regardless of whether DescribeAvailablePatches ran first.
	pending, ok := byName["ALAS2-2024-2451"]
	require.True(t, ok, "un-decided catalogue patches must be included")
	assert.Equal(t, "PENDING_APPROVAL", pending.PatchStatus.DeploymentStatus)

	require.Len(t, out.EffectivePatches, 7,
		"2 approved + 1 rejected + 4 remaining built-in catalogue patches")

	// Unknown baseline still errors distinctly.
	_, err = b.DescribeEffectivePatchesForPatchBaseline(
		ctx,
		&ssm.DescribeEffectivePatchesForPatchBaselineInput{BaselineID: "pb-unknown"},
	)
	require.ErrorIs(t, err, ssm.ErrPatchBaselineNotFound)
}

// TestStubOps_DeletePatchBaseline exercises the patch-baseline delete stub.
func TestStubOps_DeletePatchBaseline(t *testing.T) {
	t.Parallel()

	h, b := newTestHandler(t)

	pb, err := b.CreatePatchBaseline(context.TODO(), &ssm.CreatePatchBaselineInput{
		Name:            "test-baseline",
		OperatingSystem: "AMAZON_LINUX_2",
	})
	require.NoError(t, err)

	body, _ := json.Marshal(map[string]any{"BaselineId": pb.BaselineID})
	rec := doRequest(t, h, "DeletePatchBaseline", string(body))
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestStubOps_DeregisterPatchBaselineForPatchGroup exercises that stub.
func TestStubOps_DeregisterPatchBaselineForPatchGroup(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t)
	rec := doRequest(
		t,
		h,
		"DeregisterPatchBaselineForPatchGroup",
		`{"BaselineId":"pb-1234","PatchGroup":"test-group"}`,
	)
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestStubOps_DescribePatchBaselines exercises that stub.
func TestStubOps_DescribePatchBaselines(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t)
	rec := doRequest(t, h, "DescribePatchBaselines", `{}`)
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestStubOps_GetDefaultPatchBaseline exercises that stub.
func TestStubOps_GetDefaultPatchBaseline(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t)
	rec := doRequest(t, h, "GetDefaultPatchBaseline", `{}`)
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestStubOps_GetPatchBaseline exercises that stub.
func TestStubOps_GetPatchBaseline(t *testing.T) {
	t.Parallel()

	h, b := newTestHandler(t)

	pb, err := b.CreatePatchBaseline(context.TODO(), &ssm.CreatePatchBaselineInput{
		Name:            "test-baseline-2",
		OperatingSystem: "AMAZON_LINUX_2",
	})
	require.NoError(t, err)

	body, _ := json.Marshal(map[string]any{"BaselineId": pb.BaselineID})
	rec := doRequest(t, h, "GetPatchBaseline", string(body))
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestStubOps_UpdatePatchBaseline exercises that stub.
func TestStubOps_UpdatePatchBaseline(t *testing.T) {
	t.Parallel()

	h, b := newTestHandler(t)

	pb, err := b.CreatePatchBaseline(context.TODO(), &ssm.CreatePatchBaselineInput{
		Name:            "test-baseline-3",
		OperatingSystem: "AMAZON_LINUX_2",
	})
	require.NoError(t, err)

	body, _ := json.Marshal(map[string]any{"BaselineId": pb.BaselineID, "Name": "updated-baseline"})
	rec := doRequest(t, h, "UpdatePatchBaseline", string(body))
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestStubOps_RegisterPatchBaselineForPatchGroup requires valid baseline.
func TestStubOps_RegisterPatchBaselineForPatchGroup(t *testing.T) {
	t.Parallel()

	h, b := newTestHandler(t)

	pb, err := b.CreatePatchBaseline(context.TODO(), &ssm.CreatePatchBaselineInput{
		Name:            "test-baseline-4",
		OperatingSystem: "AMAZON_LINUX_2",
	})
	require.NoError(t, err)

	body, _ := json.Marshal(map[string]any{"BaselineId": pb.BaselineID, "PatchGroup": "test-group"})
	rec := doRequest(t, h, "RegisterPatchBaselineForPatchGroup", string(body))
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestPatchBaselineOps_RequireRequiredFields covers gopherstack-enpq's
// stub-list findings for this family: DeletePatchBaseline, GetPatchBaseline,
// UpdatePatchBaseline and RegisterPatchBaselineForPatchGroup previously
// returned DoesNotExistException for a missing required BaselineId/PatchGroup
// (the wrong error class -- a required-field violation, not a not-found);
// DescribeEffectivePatchesForPatchBaseline, DescribePatchGroupState,
// DescribePatchProperties, GetDeployablePatchSnapshotForInstance,
// GetPatchBaselineForPatchGroup and RegisterDefaultPatchBaseline previously
// fabricated a synthetic success for an empty body. All required fields
// confirmed against aws-sdk-go-v2/service/ssm@v1.73.4's validators.go.
func TestPatchBaselineOps_RequireRequiredFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		op   string
		body string
	}{
		{name: "delete_missing_baseline_id", op: "DeletePatchBaseline", body: `{}`},
		{name: "get_missing_baseline_id", op: "GetPatchBaseline", body: `{}`},
		{name: "update_missing_baseline_id", op: "UpdatePatchBaseline", body: `{}`},
		{
			name: "register_for_group_missing_both",
			op:   "RegisterPatchBaselineForPatchGroup",
			body: `{}`,
		},
		{
			name: "register_for_group_missing_patch_group",
			op:   "RegisterPatchBaselineForPatchGroup",
			body: `{"BaselineId":"pb-1234"}`,
		},
		{
			name: "deregister_for_group_missing_both",
			op:   "DeregisterPatchBaselineForPatchGroup",
			body: `{}`,
		},
		{
			name: "deregister_for_group_missing_patch_group",
			op:   "DeregisterPatchBaselineForPatchGroup",
			body: `{"BaselineId":"pb-1234"}`,
		},
		{
			name: "describe_effective_patches_missing_baseline_id",
			op:   "DescribeEffectivePatchesForPatchBaseline",
			body: `{}`,
		},
		{name: "describe_group_state_missing_patch_group", op: "DescribePatchGroupState", body: `{}`},
		{name: "describe_properties_missing_both", op: "DescribePatchProperties", body: `{}`},
		{
			name: "describe_properties_missing_property",
			op:   "DescribePatchProperties",
			body: `{"OperatingSystem":"WINDOWS"}`,
		},
		{
			name: "get_deployable_snapshot_missing_both",
			op:   "GetDeployablePatchSnapshotForInstance",
			body: `{}`,
		},
		{
			name: "get_deployable_snapshot_missing_snapshot_id",
			op:   "GetDeployablePatchSnapshotForInstance",
			body: `{"InstanceId":"i-1234"}`,
		},
		{
			name: "get_baseline_for_group_missing_patch_group",
			op:   "GetPatchBaselineForPatchGroup",
			body: `{}`,
		},
		{name: "register_default_missing_baseline_id", op: "RegisterDefaultPatchBaseline", body: `{}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, _ := newTestHandler(t)
			rec := doRequest(t, h, tt.op, tt.body)
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			assert.Contains(t, rec.Body.String(), "ValidationException")
		})
	}
}

func TestCreatePatchBaseline_Success(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{
			name:       "minimal_baseline",
			body:       `{"Name":"MyBaseline"}`,
			wantStatus: http.StatusOK,
		},
		{
			name: "full_baseline",
			body: `{"Name":"FullBaseline","Description":"desc","OperatingSystem":"WINDOWS",` +
				`"ApprovedPatches":["KB123"],"RejectedPatches":["KB999"],` +
				`"ApprovedPatchesComplianceLevel":"HIGH"}`,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, backend := newTestHandler(t)
			rec := doRequest(t, h, "CreatePatchBaseline", tt.body)

			require.Equal(t, tt.wantStatus, rec.Code)

			var resp ssm.CreatePatchBaselineOutput
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.NotEmpty(t, resp.BaselineID)
			assert.True(t, strings.HasPrefix(resp.BaselineID, "pb-"))
			assert.Equal(t, 1, backend.PatchBaselineCount())
		})
	}
}

// TestPatchBaseline_ApprovalRulesGlobalFiltersSourcesRoundTrip locks in
// wire-shape coverage for the previously entirely-missing
// ApprovalRules/GlobalFilters/Sources/RejectedPatchesAction/
// AvailableSecurityUpdatesComplianceStatus/ApprovedPatchesEnableNonSecurity
// fields on CreatePatchBaselineInput/PatchBaseline, confirmed against
// aws-sdk-go-v2/service/ssm@v1.73.4's api_op_CreatePatchBaseline.go.
func TestPatchBaseline_ApprovalRulesGlobalFiltersSourcesRoundTrip(t *testing.T) {
	t.Parallel()

	h, b := newTestHandler(t)

	body := `{
		"Name": "FullFeatured",
		"OperatingSystem": "AMAZON_LINUX_2",
		"RejectedPatchesAction": "BLOCK",
		"AvailableSecurityUpdatesComplianceStatus": "NON_COMPLIANT",
		"ApprovedPatchesEnableNonSecurity": true,
		"ApprovalRules": {
			"PatchRules": [{
				"PatchFilterGroup": {"PatchFilters": [{"Key": "PRODUCT", "Values": ["AmazonLinux2"]}]},
				"ApproveAfterDays": 7,
				"ComplianceLevel": "CRITICAL"
			}]
		},
		"GlobalFilters": {
			"PatchFilters": [{"Key": "CLASSIFICATION", "Values": ["Security"]}]
		},
		"Sources": [{"Name": "custom-repo", "Products": ["AmazonLinux2"], "Configuration": "[main]\nenabled=1"}]
	}`

	rec := doRequest(t, h, "CreatePatchBaseline", body)
	require.Equal(t, http.StatusOK, rec.Code)

	var created ssm.CreatePatchBaselineOutput
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))

	out, err := b.GetPatchBaseline(context.Background(), &ssm.GetPatchBaselineInput{BaselineID: created.BaselineID})
	require.NoError(t, err)

	assert.Equal(t, "BLOCK", out.RejectedPatchesAction)
	assert.Equal(t, "NON_COMPLIANT", out.AvailableSecurityUpdatesComplianceStatus)
	require.NotNil(t, out.ApprovedPatchesEnableNonSecurity)
	assert.True(t, *out.ApprovedPatchesEnableNonSecurity)
	require.NotNil(t, out.ApprovalRules)
	require.Len(t, out.ApprovalRules.PatchRules, 1)
	assert.Equal(t, "CRITICAL", out.ApprovalRules.PatchRules[0].ComplianceLevel)
	require.NotNil(t, out.ApprovalRules.PatchRules[0].ApproveAfterDays)
	assert.EqualValues(t, 7, *out.ApprovalRules.PatchRules[0].ApproveAfterDays)
	require.NotNil(t, out.GlobalFilters)
	require.Len(t, out.GlobalFilters.PatchFilters, 1)
	assert.Equal(t, "CLASSIFICATION", out.GlobalFilters.PatchFilters[0].Key)
	require.Len(t, out.Sources, 1)
	assert.Equal(t, "custom-repo", out.Sources[0].Name)
}

// TestUpdatePatchBaseline_ApprovedPatchesEnableNonSecurityPointerSemantics locks
// in that ApprovedPatchesEnableNonSecurity is a *bool (matching
// aws-sdk-go-v2/service/ssm@v1.73.4's CreatePatchBaselineInput/
// UpdatePatchBaselineInput/PatchBaseline, confirmed via `go doc`), not a plain
// bool -- a plain bool can't distinguish an explicit `false` from "field
// omitted" on UpdatePatchBaselineInput, so Update could previously only ever
// turn the flag on, never back off.
func TestUpdatePatchBaseline_ApprovedPatchesEnableNonSecurityPointerSemantics(t *testing.T) {
	t.Parallel()

	cases := []struct {
		initial *bool
		update  *bool
		want    *bool
		name    string
	}{
		{
			name:    "omitted update leaves existing true unchanged",
			initial: new(true),
			update:  nil,
			want:    new(true),
		},
		{
			name:    "explicit false flips existing true off",
			initial: new(true),
			update:  new(false),
			want:    new(false),
		},
		{
			name:    "explicit true flips existing false on",
			initial: new(false),
			update:  new(true),
			want:    new(true),
		},
		{
			name:    "omitted update leaves existing false unchanged",
			initial: new(false),
			update:  nil,
			want:    new(false),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend(t)

			created, err := b.CreatePatchBaseline(context.Background(), &ssm.CreatePatchBaselineInput{
				Name:                             "pb-" + tc.name,
				OperatingSystem:                  "AMAZON_LINUX_2",
				ApprovedPatchesEnableNonSecurity: tc.initial,
			})
			require.NoError(t, err)

			updated, err := b.UpdatePatchBaseline(context.Background(), &ssm.UpdatePatchBaselineInput{
				BaselineID:                       created.BaselineID,
				ApprovedPatchesEnableNonSecurity: tc.update,
			})
			require.NoError(t, err)

			require.NotNil(t, updated.ApprovedPatchesEnableNonSecurity)
			assert.Equal(t, *tc.want, *updated.ApprovedPatchesEnableNonSecurity)
		})
	}
}

// TestGetPatchBaseline_PatchGroupsReflectsRegistrations locks in
// GetPatchBaselineOutput.PatchGroups (previously unpopulated): the patch
// groups currently registered with a baseline via
// RegisterPatchBaselineForPatchGroup must be listed, and deregistering must
// remove them again. The synthetic "default"/"default-<OS>" bookkeeping keys
// RegisterDefaultPatchBaseline writes into the same map must never leak into
// this list -- they are not real patch groups.
func TestGetPatchBaseline_PatchGroupsReflectsRegistrations(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	ctx := context.Background()

	created, err := b.CreatePatchBaseline(ctx, &ssm.CreatePatchBaselineInput{Name: "GroupedBaseline"})
	require.NoError(t, err)

	_, err = b.RegisterPatchBaselineForPatchGroup(ctx, &ssm.RegisterPatchBaselineForPatchGroupInput{
		BaselineID: created.BaselineID, PatchGroup: "prod-web",
	})
	require.NoError(t, err)
	_, err = b.RegisterPatchBaselineForPatchGroup(ctx, &ssm.RegisterPatchBaselineForPatchGroupInput{
		BaselineID: created.BaselineID, PatchGroup: "prod-db",
	})
	require.NoError(t, err)
	_, err = b.RegisterDefaultPatchBaseline(ctx, &ssm.RegisterDefaultPatchBaselineInput{
		BaselineID: created.BaselineID,
	})
	require.NoError(t, err)

	out, err := b.GetPatchBaseline(ctx, &ssm.GetPatchBaselineInput{BaselineID: created.BaselineID})
	require.NoError(t, err)
	assert.Equal(t, []string{"prod-db", "prod-web"}, out.PatchGroups,
		"must list real patch groups, sorted, excluding the default-baseline bookkeeping keys")

	_, err = b.DeregisterPatchBaselineForPatchGroup(ctx, &ssm.DeregisterPatchBaselineForPatchGroupInput{
		BaselineID: created.BaselineID, PatchGroup: "prod-web",
	})
	require.NoError(t, err)

	out, err = b.GetPatchBaseline(ctx, &ssm.GetPatchBaselineInput{BaselineID: created.BaselineID})
	require.NoError(t, err)
	assert.Equal(t, []string{"prod-db"}, out.PatchGroups)
}

func TestSSMBounds_DescribePatchGroups(t *testing.T) {
	t.Parallel()

	_, b := newTestHandler(t)
	ctx := context.Background()

	tests := []struct {
		maxResults *int64
		name       string
		wantError  bool
	}{
		{nil, "nil uses default", false},
		{ptr64(1), "1 is valid", false},
		{ptr64(100), "100 is valid cap", false},
		{ptr64(101), "101 exceeds cap", true},
		{ptr64(0), "0 is invalid", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out, err := b.DescribePatchGroups(ctx, &ssm.DescribePatchGroupsInput{
				MaxResults: tc.maxResults,
			})

			if tc.wantError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, out)
			}
		})
	}
}

func TestBackendOps_GetPatchBaseline(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	id := createTestBaseline(t, b, "get-baseline-test")

	out, err := b.GetPatchBaseline(context.TODO(), &ssm.GetPatchBaselineInput{BaselineID: id})
	require.NoError(t, err)
	assert.Equal(t, id, out.BaselineID)
}

func TestBackendOps_GetDefaultPatchBaseline_NoDefault(t *testing.T) {
	t.Parallel()

	b := newBackend(t)

	// No default registered; should return a synthetic ID.
	out, err := b.GetDefaultPatchBaseline(context.TODO(), &ssm.GetDefaultPatchBaselineInput{})
	require.NoError(t, err)
	assert.NotEmpty(t, out.BaselineID)
}

func TestBackendOps_GetDefaultPatchBaseline_WithDefault(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	id := createTestBaseline(t, b, "default-baseline")

	_, err := b.RegisterDefaultPatchBaseline(context.TODO(), &ssm.RegisterDefaultPatchBaselineInput{
		BaselineID: id,
	})
	require.NoError(t, err)

	out, err := b.GetDefaultPatchBaseline(context.TODO(), &ssm.GetDefaultPatchBaselineInput{})
	require.NoError(t, err)
	assert.Equal(t, id, out.BaselineID)
}

func TestBackendOps_GetPatchBaselineForPatchGroup(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	id := createTestBaseline(t, b, "group-baseline")

	_, err := b.RegisterPatchBaselineForPatchGroup(context.TODO(), &ssm.RegisterPatchBaselineForPatchGroupInput{
		BaselineID: id,
		PatchGroup: "my-group",
	})
	require.NoError(t, err)

	out, err := b.GetPatchBaselineForPatchGroup(context.TODO(), &ssm.GetPatchBaselineForPatchGroupInput{
		PatchGroup: "my-group",
	})
	require.NoError(t, err)
	assert.Equal(t, id, out.BaselineID)
	assert.Equal(t, "my-group", out.PatchGroup)
}

func TestBackendOps_RegisterDefaultPatchBaseline(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	id := createTestBaseline(t, b, "reg-default-baseline")

	out, err := b.RegisterDefaultPatchBaseline(context.TODO(), &ssm.RegisterDefaultPatchBaselineInput{
		BaselineID: id,
	})
	require.NoError(t, err)
	assert.Equal(t, id, out.BaselineID)
}

func TestBackendOps_DeregisterPatchBaselineForPatchGroup(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	id := createTestBaseline(t, b, "dereg-baseline")

	_, err := b.RegisterPatchBaselineForPatchGroup(context.TODO(), &ssm.RegisterPatchBaselineForPatchGroupInput{
		BaselineID: id,
		PatchGroup: "dereg-group",
	})
	require.NoError(t, err)

	out, err := b.DeregisterPatchBaselineForPatchGroup(context.TODO(), &ssm.DeregisterPatchBaselineForPatchGroupInput{
		BaselineID: id,
		PatchGroup: "dereg-group",
	})
	require.NoError(t, err)
	assert.NotNil(t, out)
}

func TestBackendOps_DeletePatchBaseline(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	id := createTestBaseline(t, b, "delete-baseline")

	out, err := b.DeletePatchBaseline(context.TODO(), &ssm.DeletePatchBaselineInput{BaselineID: id})
	require.NoError(t, err)
	assert.Equal(t, id, out.BaselineID)

	// Should be gone.
	_, err = b.GetPatchBaseline(context.TODO(), &ssm.GetPatchBaselineInput{BaselineID: id})
	require.Error(t, err)
}

// TestBackendOps_DeletePatchBaseline_RefusesWhileRegisteredToPatchGroup
// verifies real AWS's documented DeletePatchBaseline behavior: it returns
// ResourceInUseException rather than deleting a baseline that is still
// registered to a patch group, so no patch group is ever left pointing at a
// deleted baseline.
func TestBackendOps_DeletePatchBaseline_RefusesWhileRegisteredToPatchGroup(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	id := createTestBaseline(t, b, "in-use-baseline")
	otherID := createTestBaseline(t, b, "in-use-baseline-sibling")

	_, err := b.RegisterPatchBaselineForPatchGroup(context.TODO(), &ssm.RegisterPatchBaselineForPatchGroupInput{
		BaselineID: id,
		PatchGroup: "in-use-group",
	})
	require.NoError(t, err)

	_, err = b.DeletePatchBaseline(context.TODO(), &ssm.DeletePatchBaselineInput{BaselineID: id})
	require.ErrorIs(t, err, ssm.ErrPatchBaselineInUse)

	// The baseline must still exist -- the delete was refused, not silently
	// applied with the registration left dangling.
	_, err = b.GetPatchBaseline(context.TODO(), &ssm.GetPatchBaselineInput{BaselineID: id})
	require.NoError(t, err)

	// An unrelated baseline's delete must not be disturbed.
	_, err = b.DeletePatchBaseline(context.TODO(), &ssm.DeletePatchBaselineInput{BaselineID: otherID})
	require.NoError(t, err)

	// Deregistering the patch group clears the way to delete.
	_, err = b.DeregisterPatchBaselineForPatchGroup(context.TODO(), &ssm.DeregisterPatchBaselineForPatchGroupInput{
		BaselineID: id,
		PatchGroup: "in-use-group",
	})
	require.NoError(t, err)

	_, err = b.DeletePatchBaseline(context.TODO(), &ssm.DeletePatchBaselineInput{BaselineID: id})
	require.NoError(t, err)
}

func TestBackendOps_DescribePatchBaselines(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	createTestBaseline(t, b, "describe-baseline-1")
	createTestBaseline(t, b, "describe-baseline-2")

	out, err := b.DescribePatchBaselines(context.TODO(), &ssm.DescribePatchBaselinesInput{})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(out.BaselineIdentities), 2)
}

func TestBackendOps_DescribePatchGroups(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	id := createTestBaseline(t, b, "pg-baseline")

	_, err := b.RegisterPatchBaselineForPatchGroup(context.TODO(), &ssm.RegisterPatchBaselineForPatchGroupInput{
		BaselineID: id,
		PatchGroup: "pg-test-group",
	})
	require.NoError(t, err)

	out, err := b.DescribePatchGroups(context.TODO(), &ssm.DescribePatchGroupsInput{})
	require.NoError(t, err)
	assert.NotEmpty(t, out.Mappings)
}

func TestBackendOps_DescribePatchGroupState(t *testing.T) {
	t.Parallel()

	b := newBackend(t)

	out, err := b.DescribePatchGroupState(
		context.TODO(),
		&ssm.DescribePatchGroupStateInput{PatchGroup: "grp1"},
	)
	require.NoError(t, err)
	assert.NotNil(t, out)
	assert.Equal(t, int32(0), out.Instances)

	_, err = b.DescribePatchGroupState(context.TODO(), &ssm.DescribePatchGroupStateInput{})
	require.ErrorIs(t, err, ssm.ErrValidationException, "PatchGroup is required")
}

func TestBackendOps_DescribePatchProperties(t *testing.T) {
	t.Parallel()

	b := newBackend(t)

	out, err := b.DescribePatchProperties(context.TODO(), &ssm.DescribePatchPropertiesInput{
		OperatingSystem: "WINDOWS",
		Property:        "CLASSIFICATION",
	})
	require.NoError(t, err)
	assert.NotNil(t, out.Properties)

	_, err = b.DescribePatchProperties(context.TODO(), &ssm.DescribePatchPropertiesInput{})
	require.ErrorIs(t, err, ssm.ErrValidationException, "OperatingSystem and Property are required")
}

func TestBackendOps_DescribeEffectivePatchesForPatchBaseline(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	id := createTestBaseline(t, b, "effective-patches-baseline")

	out, err := b.DescribeEffectivePatchesForPatchBaseline(
		context.TODO(),
		&ssm.DescribeEffectivePatchesForPatchBaselineInput{BaselineID: id},
	)
	require.NoError(t, err)
	assert.NotNil(t, out.EffectivePatches)
}

func TestBackendOps_GetDeployablePatchSnapshotForInstance(t *testing.T) {
	t.Parallel()

	b := newBackend(t)

	out, err := b.GetDeployablePatchSnapshotForInstance(
		context.TODO(),
		&ssm.GetDeployablePatchSnapshotForInstanceInput{
			InstanceID: "i-snapshot-test",
			SnapshotID: "snap-1234",
		},
	)
	require.NoError(t, err)
	assert.Equal(t, "i-snapshot-test", out.InstanceID)
	assert.Equal(t, "snap-1234", out.SnapshotID)
	assert.NotEmpty(t, out.SnapshotDownloadURL)
}

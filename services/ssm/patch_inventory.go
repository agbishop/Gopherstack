package ssm

import (
	"fmt"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
)

// --- Inventory deletion job records ---

// InventoryDeletionSummary summarises the outcome of a DeleteInventory job.
type InventoryDeletionSummary struct {
	SummaryItems   []any `json:"SummaryItems,omitempty"`
	RemainingCount int   `json:"RemainingCount"`
	TotalCount     int   `json:"TotalCount"`
}

// InventoryDeletion is a record of a DeleteInventory job, returned by
// DescribeInventoryDeletions.
type InventoryDeletion struct {
	DeletionSummary   *InventoryDeletionSummary `json:"DeletionSummary,omitempty"`
	DeletionID        string                    `json:"DeletionId"`
	TypeName          string                    `json:"TypeName"`
	LastStatus        string                    `json:"LastStatus"`
	LastStatusMessage string                    `json:"LastStatusMessage,omitempty"`
	DeletionStartTime float64                   `json:"DeletionStartTime"`
}

// --- Default (AWS-managed) patch baselines ---

// defaultBaselineID derives a stable, realistically-shaped patch baseline ID for
// the AWS-managed default baseline of the given OS. AWS provides a managed
// default baseline per operating system; returning a deterministic, well-shaped
// ID (rather than a fabricated all-zeros ID) keeps GetDefaultPatchBaseline
// stable across calls and consistent for a given OS.
func defaultBaselineID(os string) string {
	h := httputils.FNV64a("AWS-DefaultPatchBaseline:" + os)

	return fmt.Sprintf("pb-%017x", h)
}

// defaultPatchScanOS is the operating system assumed for a patch operation
// whose instance has no registered patch group / baseline, matching the
// Windows fallback GetDefaultPatchBaseline already uses.
const defaultPatchScanOS = "WINDOWS"

// patchProductAmazonLinux2 identifies the Amazon Linux 2 platform in patch
// catalogue/baseline data (mirrored across the built-in catalogue and the
// deployable-snapshot fallback).
const patchProductAmazonLinux2 = "AmazonLinux2"

// patchClassificationSecurityUpdates is the classification used for
// AWS-managed security patches, both in the built-in catalogue and for
// explicitly-approved patches with no catalogue entry.
const patchClassificationSecurityUpdates = "SecurityUpdates"

// patchComplianceStateMissing/Installed are the PatchComplianceData.State
// values this emulator produces for a patch that has not yet been installed
// versus one an Install operation has applied.
const (
	patchComplianceStateMissing   = "Missing"
	patchComplianceStateInstalled = "Installed"
)

// defaultAgentVersionSSM is the SSM Agent version reported for managed
// instances derived from activations, matching across
// DescribeInstanceInformation and DescribeInstanceProperties.
const defaultAgentVersionSSM = "3.0.0"

// defaultPatchCatalog returns the built-in catalogue of available patches AWS
// Patch Manager ships knowledge of out of the box, spanning the platforms this
// emulator's patch baselines commonly target. It seeds a region's
// available-patches store on first access (see availablePatchesFor) instead of
// leaving DescribeAvailablePatches permanently empty. ReleaseDate values are
// the real-world release dates of these KB/advisory IDs, fixed at compile
// time (never time.Now()) so ApprovalRules.ApproveAfterDays evaluation stays
// reproducible.
func defaultPatchCatalog() []Patch {
	return []Patch{
		{
			Name: "KB5034441", Product: "WindowsServer2022",
			Classification: patchClassificationSecurityUpdates, Severity: "Critical",
			ReleaseDate: UnixTimeFloat(time.Date(2024, time.January, 9, 0, 0, 0, 0, time.UTC)),
		},
		{
			Name: "KB5034129", Product: "WindowsServer2019",
			Classification: patchClassificationSecurityUpdates, Severity: "Important",
			ReleaseDate: UnixTimeFloat(time.Date(2023, time.December, 12, 0, 0, 0, 0, time.UTC)),
		},
		{
			Name: "ALAS2-2024-2451", Product: patchProductAmazonLinux2,
			Classification: "Security", Severity: "Critical",
			ReleaseDate: UnixTimeFloat(time.Date(2024, time.March, 1, 0, 0, 0, 0, time.UTC)),
		},
		{
			Name: "ALAS2-2024-2460", Product: patchProductAmazonLinux2,
			Classification: "Bugfix", Severity: "Medium",
			ReleaseDate: UnixTimeFloat(time.Date(2024, time.March, 15, 0, 0, 0, 0, time.UTC)),
		},
		{
			Name: "USN-6567-1", Product: "Ubuntu2204",
			Classification: "Security", Severity: "High",
			ReleaseDate: UnixTimeFloat(time.Date(2024, time.January, 22, 0, 0, 0, 0, time.UTC)),
		},
	}
}

// availablePatchesFor returns the region's available-patches catalogue,
// lazily seeding it with defaultPatchCatalog on first access so
// DescribeAvailablePatches and effective-patch computation are backed by real
// (if synthetic) catalogue data instead of a permanently empty map.
// Must be called with b.mu held for writing.
func (b *InMemoryBackend) availablePatchesFor(region string) []Patch {
	if b.availablePatches[region] == nil {
		b.availablePatches[region] = defaultPatchCatalog()
	}

	return b.availablePatches[region]
}

// paramValue returns the first value of a SendCommand parameter, or "" if the
// parameter is absent or empty.
func paramValue(params map[string][]string, key string) string {
	if v, ok := params[key]; ok && len(v) > 0 {
		return v[0]
	}

	return ""
}

// resolvePatchGroupBaseline looks up the patch baseline registered for a patch
// group. Must be called with b.mu held.
func (b *InMemoryBackend) resolvePatchGroupBaseline(region, patchGroup string) (PatchBaseline, bool) {
	id, ok := b.patchGroupToBaselineStore(region)[patchGroup]
	if !ok {
		return PatchBaseline{}, false
	}

	blPtr, found := b.patchBaselinesStore(region).Get(id)
	if !found {
		return PatchBaseline{}, false
	}

	return *blPtr, true
}

// applyPatchBaselineOperation simulates a Patch Manager scan/install run
// (the AWS-RunPatchBaseline / AWS-ApplyPatchBaseline documents) against a
// managed instance: it resolves the instance's patch baseline (via its patch
// group, falling back to the AWS-managed default baseline for its OS),
// computes the effective patch set against the region's available-patches
// catalogue, and records the resulting InstancePatchState and
// PatchComplianceData — the same state DescribeInstancePatchStates,
// DescribeInstancePatchStatesForPatchGroup and DescribeInstancePatches read.
// Must be called with b.mu held for writing.
func (b *InMemoryBackend) applyPatchBaselineOperation(
	region, instanceID string,
	params map[string][]string,
) {
	patchGroup := paramValue(params, "PatchGroup")

	operation := paramValue(params, "Operation")
	if operation == "" {
		operation = "Scan"
	}

	os := defaultPatchScanOS

	baseline, ok := b.resolvePatchGroupBaseline(region, patchGroup)
	if ok && baseline.OperatingSystem != "" {
		os = baseline.OperatingSystem
	}

	if !ok {
		baseline = PatchBaseline{BaselineID: defaultBaselineID(os), OperatingSystem: os}
	}

	// Ensure the region's available-patches catalogue is seeded before
	// deriving the effective patch set from it.
	b.availablePatchesFor(region)

	patches, installedCount, missingCount := patchComplianceFromEffective(
		b.effectivePatchesForBaseline(region, baseline),
		operation,
	)

	operationTime := UnixTimeFloat(time.Now())

	state := &InstancePatchState{
		InstanceID:         instanceID,
		PatchGroup:         patchGroup,
		BaselineID:         baseline.BaselineID,
		Operation:          operation,
		OperationStartTime: operationTime,
		OperationEndTime:   operationTime,
		InstalledCount:     installedCount,
		MissingCount:       missingCount,
	}

	b.instancePatchStatesStore(region).Put(state)

	if b.instancePatches[region] == nil {
		b.instancePatches[region] = make(map[string][]PatchComplianceData)
	}
	b.instancePatchesStore(region)[instanceID] = patches
}

// patchComplianceFromEffective converts a baseline's effective patch set into
// per-patch compliance records for an instance. An Install operation marks
// explicitly- or rule-approved patches (EXPLICIT_APPROVED, APPROVED)
// Installed; a Scan (or anything else) reports the baseline's real compliance
// state (Missing for anything not yet installed).
func patchComplianceFromEffective(
	effective []EffectivePatch,
	operation string,
) ([]PatchComplianceData, int, int) {
	now := UnixTimeFloat(time.Now())
	patches := make([]PatchComplianceData, 0, len(effective))

	var installedCount, missingCount int

	for _, ep := range effective {
		if ep.Patch == nil {
			continue
		}

		// Rejected patches aren't part of an instance's compliance surface --
		// they were intentionally excluded, not missed. Matches this
		// function's behavior before effectivePatchesForBaseline started
		// emitting EXPLICIT_REJECTED entries.
		if ep.PatchStatus != nil && ep.PatchStatus.DeploymentStatus == patchDeploymentStatusExplicitRejected {
			continue
		}

		approved := ep.PatchStatus != nil &&
			(ep.PatchStatus.DeploymentStatus == patchDeploymentStatusExplicitApproved ||
				ep.PatchStatus.DeploymentStatus == patchDeploymentStatusApproved)

		complianceState := patchComplianceStateMissing

		var installedTime float64

		if operation == "Install" && approved {
			complianceState = patchComplianceStateInstalled
			installedTime = now
			installedCount++
		} else {
			missingCount++
		}

		patches = append(patches, PatchComplianceData{
			Title:          ep.Patch.Name,
			KBId:           ep.Patch.Name,
			Classification: ep.Patch.Classification,
			Severity:       ep.Patch.Severity,
			State:          complianceState,
			InstalledTime:  installedTime,
		})
	}

	return patches, installedCount, missingCount
}

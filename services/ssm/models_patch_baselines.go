package ssm

// DeregisterPatchBaselineForPatchGroupOutput is the response for DeregisterPatchBaselineForPatchGroup.
type DeregisterPatchBaselineForPatchGroupOutput struct {
	BaselineID string `json:"BaselineId"`
	PatchGroup string `json:"PatchGroup"`
}

// DeregisterPatchBaselineForPatchGroupInput is the request for DeregisterPatchBaselineForPatchGroup.
type DeregisterPatchBaselineForPatchGroupInput struct {
	BaselineID string `json:"BaselineId"`
	PatchGroup string `json:"PatchGroup"`
}

// PatchFilter is a filter for patch operations.
type PatchFilter struct {
	Key    string   `json:"Key"`
	Values []string `json:"Values"`
}

// Patch represents a patch in the available patches catalog. ReleaseDate is
// epoch seconds, not an RFC3339 string -- confirmed against
// aws-sdk-go-v2/service/ssm@v1.73.4's deserializers.go
// (awsAwsjson11_deserializeDocumentPatch, case "ReleaseDate":
// smithytime.ParseEpochSeconds(f64)), matching every other timestamp this
// awsjson1.1 service emits.
type Patch struct {
	Name           string  `json:"Name"`
	Product        string  `json:"Product"`
	Classification string  `json:"Classification"`
	Severity       string  `json:"Severity"`
	ReleaseDate    float64 `json:"ReleaseDate,omitempty"`
}

// DescribeAvailablePatchesInput is the request for DescribeAvailablePatches.
type DescribeAvailablePatchesInput struct {
	MaxResults *int64        `json:"MaxResults,omitempty"`
	NextToken  string        `json:"NextToken,omitempty"`
	Filters    []PatchFilter `json:"Filters,omitempty"`
}

// DescribeAvailablePatchesOutput is the response for DescribeAvailablePatches.
type DescribeAvailablePatchesOutput struct {
	NextToken string  `json:"NextToken,omitempty"`
	Patches   []Patch `json:"Patches"`
}

// PatchOrchestratorFilter filters patches by field.
type PatchOrchestratorFilter struct {
	Key    string   `json:"Key"`
	Values []string `json:"Values"`
}

// PatchBaselineFilter is a key-value filter for DescribePatchBaselines.
type PatchBaselineFilter struct {
	Key    string   `json:"Key"`
	Values []string `json:"Values"`
}

// DescribePatchBaselinesInput is the request payload for DescribePatchBaselines.
type DescribePatchBaselinesInput struct {
	MaxResults *int64                `json:"MaxResults,omitempty"`
	NextToken  string                `json:"NextToken,omitempty"`
	Filters    []PatchBaselineFilter `json:"Filters,omitempty"`
}

// DescribePatchBaselinesOutput is the response payload for DescribePatchBaselines.
type DescribePatchBaselinesOutput struct {
	NextToken          string                  `json:"NextToken,omitempty"`
	BaselineIdentities []PatchBaselineIdentity `json:"BaselineIdentities"`
}

// PatchBaselineIdentity is a lightweight patch baseline listing entry.
type PatchBaselineIdentity struct {
	BaselineID      string `json:"BaselineId"`
	BaselineName    string `json:"BaselineName"`
	OperatingSystem string `json:"OperatingSystem,omitempty"`
	Description     string `json:"Description,omitempty"`
}

// GetPatchBaselineInput is the request payload for GetPatchBaseline.
type GetPatchBaselineInput struct {
	BaselineID string `json:"BaselineId"`
}

// GetPatchBaselineOutput is the response payload for GetPatchBaseline.
// PatchGroups (the patch groups currently registered with this baseline) is
// unique to GetPatchBaselineOutput -- confirmed absent from
// UpdatePatchBaselineOutput/CreatePatchBaselineOutput in
// aws-sdk-go-v2/service/ssm@v1.73.4's api_op_UpdatePatchBaseline.go -- so it
// lives here rather than on the shared embedded PatchBaseline struct.
type GetPatchBaselineOutput struct {
	PatchGroups []string `json:"PatchGroups,omitempty"`
	PatchBaseline
}

// RegisterPatchBaselineForPatchGroupInput is the request payload.
type RegisterPatchBaselineForPatchGroupInput struct {
	BaselineID string `json:"BaselineId"`
	PatchGroup string `json:"PatchGroup"`
}

// RegisterPatchBaselineForPatchGroupOutput is the response payload.
type RegisterPatchBaselineForPatchGroupOutput struct {
	BaselineID string `json:"BaselineId"`
	PatchGroup string `json:"PatchGroup"`
}

// UpdatePatchBaselineInput is the request payload for UpdatePatchBaseline.
type UpdatePatchBaselineInput struct {
	ApprovalRules                            *PatchRuleGroup   `json:"ApprovalRules,omitempty"`
	GlobalFilters                            *PatchFilterGroup `json:"GlobalFilters,omitempty"`
	ApprovedPatchesEnableNonSecurity         *bool             `json:"ApprovedPatchesEnableNonSecurity,omitempty"`
	BaselineID                               string            `json:"BaselineId"`
	Name                                     string            `json:"Name,omitempty"`
	Description                              string            `json:"Description,omitempty"`
	ApprovedPatchesComplianceLevel           string            `json:"ApprovedPatchesComplianceLevel,omitempty"`
	AvailableSecurityUpdatesComplianceStatus string            `json:"AvailableSecurityUpdatesComplianceStatus,omitempty"`
	RejectedPatchesAction                    string            `json:"RejectedPatchesAction,omitempty"`
	ApprovedPatches                          []string          `json:"ApprovedPatches,omitempty"`
	RejectedPatches                          []string          `json:"RejectedPatches,omitempty"`
	Sources                                  []PatchSource     `json:"Sources,omitempty"`
}

// UpdatePatchBaselineOutput is the response payload for UpdatePatchBaseline.
type UpdatePatchBaselineOutput struct {
	PatchBaseline
}

// PatchFilterGroup groups the PatchFilters that make up a PatchRule's
// matching criteria, or a baseline's top-level GlobalFilters.
type PatchFilterGroup struct {
	PatchFilters []PatchFilter `json:"PatchFilters"`
}

// PatchRule is one auto-approval rule within a PatchRuleGroup.
type PatchRule struct {
	PatchFilterGroup  *PatchFilterGroup `json:"PatchFilterGroup,omitempty"`
	ApproveAfterDays  *int32            `json:"ApproveAfterDays,omitempty"`
	ApproveUntilDate  string            `json:"ApproveUntilDate,omitempty"`
	ComplianceLevel   string            `json:"ComplianceLevel,omitempty"`
	EnableNonSecurity bool              `json:"EnableNonSecurity,omitempty"`
}

// PatchRuleGroup is the set of auto-approval rules for a patch baseline
// (CreatePatchBaselineInput/UpdatePatchBaselineInput's ApprovalRules).
type PatchRuleGroup struct {
	PatchRules []PatchRule `json:"PatchRules"`
}

// PatchSource describes a custom patch repository. Linux managed nodes only.
type PatchSource struct {
	Name          string   `json:"Name"`
	Configuration string   `json:"Configuration"`
	Products      []string `json:"Products"`
}

// PatchBaseline represents an SSM patch baseline.
type PatchBaseline struct {
	ApprovalRules                            *PatchRuleGroup   `json:"ApprovalRules,omitempty"`
	ApprovedPatchesEnableNonSecurity         *bool             `json:"ApprovedPatchesEnableNonSecurity,omitempty"`
	GlobalFilters                            *PatchFilterGroup `json:"GlobalFilters,omitempty"`
	RejectedPatchesAction                    string            `json:"RejectedPatchesAction,omitempty"`
	ApprovedPatchesComplianceLevel           string            `json:"ApprovedPatchesComplianceLevel,omitempty"`
	AvailableSecurityUpdatesComplianceStatus string            `json:"AvailableSecurityUpdatesComplianceStatus,omitempty"`
	BaselineID                               string            `json:"BaselineId"`
	OperatingSystem                          string            `json:"OperatingSystem,omitempty"`
	Description                              string            `json:"Description,omitempty"`
	Name                                     string            `json:"Name"`
	ApprovedPatches                          []string          `json:"ApprovedPatches,omitempty"`
	RejectedPatches                          []string          `json:"RejectedPatches,omitempty"`
	Sources                                  []PatchSource     `json:"Sources,omitempty"`
	CreatedDate                              float64           `json:"CreatedDate"`
	ModifiedDate                             float64           `json:"ModifiedDate"`
}

// CreatePatchBaselineInput is the request payload for CreatePatchBaseline.
type CreatePatchBaselineInput struct {
	ApprovalRules                            *PatchRuleGroup   `json:"ApprovalRules,omitempty"`
	ApprovedPatchesEnableNonSecurity         *bool             `json:"ApprovedPatchesEnableNonSecurity,omitempty"`
	GlobalFilters                            *PatchFilterGroup `json:"GlobalFilters,omitempty"`
	ApprovedPatchesComplianceLevel           string            `json:"ApprovedPatchesComplianceLevel,omitempty"`
	AvailableSecurityUpdatesComplianceStatus string            `json:"AvailableSecurityUpdatesComplianceStatus,omitempty"`
	RejectedPatchesAction                    string            `json:"RejectedPatchesAction,omitempty"`
	Name                                     string            `json:"Name"`
	OperatingSystem                          string            `json:"OperatingSystem,omitempty"`
	Description                              string            `json:"Description,omitempty"`
	ApprovedPatches                          []string          `json:"ApprovedPatches,omitempty"`
	RejectedPatches                          []string          `json:"RejectedPatches,omitempty"`
	Sources                                  []PatchSource     `json:"Sources,omitempty"`
	Tags                                     []Tag             `json:"Tags,omitempty"`
}

// CreatePatchBaselineOutput is the response payload for CreatePatchBaseline.
type CreatePatchBaselineOutput struct {
	BaselineID string `json:"BaselineId"`
}

// GetDefaultPatchBaselineInput is the request payload for GetDefaultPatchBaseline.
type GetDefaultPatchBaselineInput struct {
	OperatingSystem string `json:"OperatingSystem,omitempty"`
}

// GetDefaultPatchBaselineOutput is the response payload for GetDefaultPatchBaseline.
type GetDefaultPatchBaselineOutput struct {
	BaselineID      string `json:"BaselineId"`
	OperatingSystem string `json:"OperatingSystem,omitempty"`
}

// GetPatchBaselineForPatchGroupInput is the request payload for GetPatchBaselineForPatchGroup.
type GetPatchBaselineForPatchGroupInput struct {
	PatchGroup      string `json:"PatchGroup"`
	OperatingSystem string `json:"OperatingSystem,omitempty"`
}

// GetPatchBaselineForPatchBaselineOutput is the response for GetPatchBaselineForPatchGroup.
type GetPatchBaselineForPatchBaselineOutput struct {
	BaselineID      string `json:"BaselineId"`
	PatchGroup      string `json:"PatchGroup,omitempty"`
	OperatingSystem string `json:"OperatingSystem,omitempty"`
}

// RegisterDefaultPatchBaselineInput is the request payload for RegisterDefaultPatchBaseline.
type RegisterDefaultPatchBaselineInput struct {
	BaselineID string `json:"BaselineId"`
}

// RegisterDefaultPatchBaselineOutput is the response payload for RegisterDefaultPatchBaseline.
type RegisterDefaultPatchBaselineOutput struct {
	BaselineID string `json:"BaselineId"`
}

// DeletePatchBaselineInput is the request payload for DeletePatchBaseline.
type DeletePatchBaselineInput struct {
	BaselineID string `json:"BaselineId"`
}

// DeletePatchBaselineOutput is the response payload for DeletePatchBaseline.
type DeletePatchBaselineOutput struct {
	BaselineID string `json:"BaselineId"`
}

// DescribePatchGroupStateInput is the request payload for DescribePatchGroupState.
type DescribePatchGroupStateInput struct {
	PatchGroup string `json:"PatchGroup"`
}

// DescribePatchGroupStateOutput is the response payload for DescribePatchGroupState.
type DescribePatchGroupStateOutput struct {
	Instances                                int32 `json:"Instances"`
	InstancesWithCriticalNonCompliantPatches int32 `json:"InstancesWithCriticalNonCompliantPatches"`
	InstancesWithFailedPatches               int32 `json:"InstancesWithFailedPatches"`
	InstancesWithInstalledPatches            int32 `json:"InstancesWithInstalledPatches"`
	InstancesWithInstalledOtherPatches       int32 `json:"InstancesWithInstalledOtherPatches"`
	InstancesWithMissingPatches              int32 `json:"InstancesWithMissingPatches"`
	InstancesWithNotApplicablePatches        int32 `json:"InstancesWithNotApplicablePatches"`
}

// DescribePatchGroupsInput is the request payload for DescribePatchGroups.
type DescribePatchGroupsInput struct {
	MaxResults *int64        `json:"MaxResults,omitempty"`
	NextToken  string        `json:"NextToken,omitempty"`
	Filters    []PatchFilter `json:"Filters,omitempty"`
}

// PatchGroupPatchBaselineMapping maps a patch group to a baseline identity.
type PatchGroupPatchBaselineMapping struct {
	BaselineIdentity PatchBaselineIdentity `json:"BaselineIdentity"`
	PatchGroup       string                `json:"PatchGroup"`
}

// DescribePatchGroupsOutput is the response payload for DescribePatchGroups.
type DescribePatchGroupsOutput struct {
	NextToken string                           `json:"NextToken,omitempty"`
	Mappings  []PatchGroupPatchBaselineMapping `json:"Mappings"`
}

// DescribePatchPropertiesInput is the request payload for DescribePatchProperties.
type DescribePatchPropertiesInput struct {
	MaxResults      *int32 `json:"MaxResults,omitempty"`
	OperatingSystem string `json:"OperatingSystem,omitempty"`
	Property        string `json:"Property,omitempty"`
	PatchSet        string `json:"PatchSet,omitempty"`
	NextToken       string `json:"NextToken,omitempty"`
}

// DescribePatchPropertiesOutput is the response payload for DescribePatchProperties.
type DescribePatchPropertiesOutput struct {
	NextToken  string              `json:"NextToken,omitempty"`
	Properties []map[string]string `json:"Properties"`
}

// DescribeEffectivePatchesForPatchBaselineInput is the request payload.
type DescribeEffectivePatchesForPatchBaselineInput struct {
	MaxResults *int64 `json:"MaxResults,omitempty"`
	BaselineID string `json:"BaselineId"`
	NextToken  string `json:"NextToken,omitempty"`
}

// EffectivePatch represents a patch matched by a baseline rule.
type EffectivePatch struct {
	Patch       *Patch       `json:"Patch,omitempty"`
	PatchStatus *PatchStatus `json:"PatchStatus,omitempty"`
}

// PatchStatus holds the deployment/compliance status of a patch. ApprovalDate
// is epoch seconds, not an RFC3339 string -- confirmed against
// aws-sdk-go-v2/service/ssm@v1.73.4's deserializers.go
// (awsAwsjson11_deserializeDocumentPatchStatus, case "ApprovalDate":
// ParseEpochSeconds(f64)); the real member is a JSON number like every other
// timestamp this awsjson1.1 service emits.
type PatchStatus struct {
	ComplianceLevel  string  `json:"ComplianceLevel,omitempty"`
	DeploymentStatus string  `json:"DeploymentStatus,omitempty"`
	ApprovalDate     float64 `json:"ApprovalDate,omitempty"`
}

// DescribeEffectivePatchesForPatchBaselineOutput is the response payload.
type DescribeEffectivePatchesForPatchBaselineOutput struct {
	NextToken        string           `json:"NextToken,omitempty"`
	EffectivePatches []EffectivePatch `json:"EffectivePatches"`
}

// GetDeployablePatchSnapshotForInstanceInput is the request payload.
type GetDeployablePatchSnapshotForInstanceInput struct {
	InstanceID string `json:"InstanceId"`
	SnapshotID string `json:"SnapshotId"`
}

// GetDeployablePatchSnapshotForInstanceOutput is the response payload.
type GetDeployablePatchSnapshotForInstanceOutput struct {
	InstanceID          string `json:"InstanceId,omitempty"`
	SnapshotID          string `json:"SnapshotId,omitempty"`
	SnapshotDownloadURL string `json:"SnapshotDownloadUrl,omitempty"`
	Product             string `json:"Product,omitempty"`
}

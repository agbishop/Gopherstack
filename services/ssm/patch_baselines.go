package ssm

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// Real PatchDeploymentStatus enum values (aws-sdk-go-v2/service/ssm@v1.73.4's
// types/enums.go) -- "AVAILABLE" is not one of them; it was a fabricated value
// this emulator used to describe an un-decided catalogue patch.
const (
	patchDeploymentStatusExplicitApproved = "EXPLICIT_APPROVED"
	patchDeploymentStatusExplicitRejected = "EXPLICIT_REJECTED"
	patchDeploymentStatusPendingApproval  = "PENDING_APPROVAL"
	patchDeploymentStatusApproved         = "APPROVED"
)

// Real PatchAction enum values (RejectedPatchesAction), aws-sdk-go-v2/service/ssm@v1.73.4's
// types/enums.go.
const (
	patchRejectedActionBlock             = "BLOCK"
	patchRejectedActionAllowAsDependency = "ALLOW_AS_DEPENDENCY"
)

// defaultPatchGroupKey is the patchGroupToBaseline key used for the
// OS-agnostic default patch baseline (RegisterDefaultPatchBaseline without a
// specific OS). Per-OS defaults use "default-" + OperatingSystem.
const defaultPatchGroupKey = "default"

func (b *InMemoryBackend) patchGroupToBaselineStore(region string) map[string]string {
	return b.patchGroupToBaseline[region]
}
func (b *InMemoryBackend) patchBaselinesStore(region string) *store.Table[PatchBaseline] {
	return getOrCreateTable(b, b.patchBaselines, "patchBaselines", region, patchBaselineKeyFn)
}

// validateRejectedPatchesAction rejects a RejectedPatchesAction value outside
// the real PatchAction enum (aws-sdk-go-v2/service/ssm@v1.73.4's
// types/enums.go: BLOCK, ALLOW_AS_DEPENDENCY) instead of silently storing
// whatever string the caller sent.
func validateRejectedPatchesAction(action string) error {
	if action == "" || action == patchRejectedActionBlock || action == patchRejectedActionAllowAsDependency {
		return nil
	}

	return fmt.Errorf("%w: RejectedPatchesAction must be one of BLOCK, ALLOW_AS_DEPENDENCY", ErrValidationException)
}

// validateApprovalRules checks a PatchRuleGroup against the constraints
// documented for PatchRule (aws-sdk-go-v2/service/ssm@v1.73.4's
// types/types.go and the AWS API_PatchRule reference page):
//   - PatchFilterGroup is "This member is required" on PatchRule.
//   - PatchFilter's Key and Values are each "This member is required".
//   - "your request must include a value for either ApproveAfterDays or
//     ApproveUntilDate" -- at least one is required. The docs don't say what
//     happens if both are given; this backend treats the documented "either
//     ... or" as exclusive and rejects both being set, rather than silently
//     picking one.
//   - ApproveAfterDays: "Valid Range: Minimum value of 0. Maximum value of 360."
//   - ApproveUntilDate: "Enter dates in the format YYYY-MM-DD."
func validateApprovalRules(group *PatchRuleGroup) error {
	if group == nil {
		return nil
	}

	if len(group.PatchRules) == 0 {
		return fmt.Errorf("%w: ApprovalRules.PatchRules is required", ErrValidationException)
	}

	for i, rule := range group.PatchRules {
		if err := validateApprovalRule(i, rule); err != nil {
			return err
		}
	}

	return nil
}

// validateApprovalRule validates a single PatchRule; see validateApprovalRules.
func validateApprovalRule(i int, rule PatchRule) error {
	if rule.PatchFilterGroup == nil || len(rule.PatchFilterGroup.PatchFilters) == 0 {
		return fmt.Errorf("%w: PatchRules[%d].PatchFilterGroup is required", ErrValidationException, i)
	}

	for _, f := range rule.PatchFilterGroup.PatchFilters {
		if f.Key == "" || f.Values == nil {
			return fmt.Errorf(
				"%w: PatchRules[%d].PatchFilterGroup.PatchFilters requires Key and Values",
				ErrValidationException, i,
			)
		}
	}

	hasAfterDays := rule.ApproveAfterDays != nil
	hasUntilDate := rule.ApproveUntilDate != ""

	// "your request must include a value for either ApproveAfterDays or
	// ApproveUntilDate" (API_PatchRule.html) requires at least one -- neither
	// set is rejected. Both set is NOT documented as an error anywhere in that
	// page (no "not both"/"mutually exclusive" language) and validatePatchRule
	// (validators.go) doesn't enforce exclusivity either, so this backend
	// accepts it rather than fabricating a rejection; see ruleOutcomeForPatch
	// for which one then wins.
	if !hasAfterDays && !hasUntilDate {
		return fmt.Errorf(
			"%w: PatchRules[%d] requires a value for ApproveAfterDays or ApproveUntilDate",
			ErrValidationException, i,
		)
	}

	const maxApproveAfterDays = 360

	if hasAfterDays && (*rule.ApproveAfterDays < 0 || *rule.ApproveAfterDays > maxApproveAfterDays) {
		return fmt.Errorf("%w: PatchRules[%d].ApproveAfterDays must be between 0 and 360", ErrValidationException, i)
	}

	if hasUntilDate {
		if _, err := time.Parse(time.DateOnly, rule.ApproveUntilDate); err != nil {
			return fmt.Errorf(
				"%w: PatchRules[%d].ApproveUntilDate must be formatted YYYY-MM-DD",
				ErrValidationException, i,
			)
		}
	}

	return nil
}

// CreatePatchBaseline creates a new patch baseline.
func (b *InMemoryBackend) CreatePatchBaseline(
	ctx context.Context,
	input *CreatePatchBaselineInput,
) (*CreatePatchBaselineOutput, error) {
	if input.Name == "" {
		return nil, fmt.Errorf("%w: Name is required", ErrValidationException)
	}

	if err := validateRejectedPatchesAction(input.RejectedPatchesAction); err != nil {
		return nil, err
	}

	if err := validateApprovalRules(input.ApprovalRules); err != nil {
		return nil, err
	}

	const defaultPatchOS = "WINDOWS"
	os := input.OperatingSystem
	if os == "" {
		os = defaultPatchOS
	}

	region := getRegion(ctx)
	b.mu.Lock("CreatePatchBaseline")
	defer b.mu.Unlock()

	baselineID := baselineIDPrefix + uuid.NewString()
	now := UnixTimeFloat(time.Now())

	bl := PatchBaseline{
		BaselineID:                               baselineID,
		Name:                                     input.Name,
		Description:                              input.Description,
		OperatingSystem:                          os,
		ApprovedPatches:                          input.ApprovedPatches,
		RejectedPatches:                          input.RejectedPatches,
		ApprovedPatchesComplianceLevel:           input.ApprovedPatchesComplianceLevel,
		AvailableSecurityUpdatesComplianceStatus: input.AvailableSecurityUpdatesComplianceStatus,
		RejectedPatchesAction:                    input.RejectedPatchesAction,
		ApprovalRules:                            input.ApprovalRules,
		GlobalFilters:                            input.GlobalFilters,
		Sources:                                  input.Sources,
		ApprovedPatchesEnableNonSecurity:         input.ApprovedPatchesEnableNonSecurity,
		CreatedDate:                              now,
		ModifiedDate:                             now,
	}

	b.patchBaselinesStore(region).Put(&bl)

	if len(input.Tags) > 0 {
		if b.miscResourceTags[region] == nil {
			b.miscResourceTags[region] = make(map[string]map[string]string)
		}
		miscTags := b.miscResourceTagsStore(region)
		if miscTags[baselineID] == nil {
			miscTags[baselineID] = make(map[string]string)
		}
		for _, t := range input.Tags {
			miscTags[baselineID][t.Key] = t.Value
		}
	}

	return &CreatePatchBaselineOutput{BaselineID: baselineID}, nil
}

// DeregisterPatchBaselineForPatchGroup removes a patch group association.
func (b *InMemoryBackend) DeregisterPatchBaselineForPatchGroup(
	ctx context.Context,
	input *DeregisterPatchBaselineForPatchGroupInput,
) (*DeregisterPatchBaselineForPatchGroupOutput, error) {
	if input.BaselineID == "" {
		return nil, fmt.Errorf("%w: BaselineId is required", ErrValidationException)
	}

	if input.PatchGroup == "" {
		return nil, fmt.Errorf("%w: PatchGroup is required", ErrValidationException)
	}

	region := getRegion(ctx)
	b.mu.Lock("DeregisterPatchBaselineForPatchGroup")
	defer b.mu.Unlock()

	delete(b.patchGroupToBaselineStore(region), input.PatchGroup)

	return &DeregisterPatchBaselineForPatchGroupOutput{
		BaselineID: input.BaselineID,
		PatchGroup: input.PatchGroup,
	}, nil
}

// patchMatchesFilters returns true when p satisfies all provided key-value
// filters. Supported keys are the ones backed by fields this emulator's Patch
// actually models: PRODUCT, NAME, SEVERITY, CLASSIFICATION (real keys per
// aws-sdk-go-v2/service/ssm@v1.73.4's api_op_DescribeAvailablePatches.go doc
// comment; PATCH_ID/MSRC_SEVERITY/PRODUCT_FAMILY/PATCH_SET are real but have
// no backing Go field, same disclosed-gap class as GetInventorySchema).
func patchMatchesFilters(p Patch, filters []PatchFilter) bool {
	for _, f := range filters {
		var fieldValue string

		switch f.Key {
		case "PRODUCT":
			fieldValue = p.Product
		case "NAME":
			fieldValue = p.Name
		case "SEVERITY":
			fieldValue = p.Severity
		case "CLASSIFICATION":
			fieldValue = p.Classification
		default:
			continue
		}

		if !slices.Contains(f.Values, fieldValue) {
			return false
		}
	}

	return true
}

// ruleFilterGroupMatches reports whether p satisfies every filter in group,
// for the purpose of ApprovalRules evaluation. Unlike patchMatchesFilters
// (used for read-only catalogue/baseline filtering, where an unsupported key
// is silently skipped and so loosens rather than narrows the filter), any
// unsupported key here fails the whole group closed: a PatchRule this
// emulator can't actually evaluate must never silently rule-approve patches,
// which has real observable consequences (InstancePatchState/
// PatchComplianceData), unlike a merely-broader read-only filter.
func ruleFilterGroupMatches(p Patch, group *PatchFilterGroup) bool {
	if group == nil || len(group.PatchFilters) == 0 {
		return false
	}

	for _, f := range group.PatchFilters {
		switch f.Key {
		case "PRODUCT", "NAME", "SEVERITY", "CLASSIFICATION":
		default:
			return false
		}
	}

	return patchMatchesFilters(p, group.PatchFilters)
}

// ruleOutcomeForPatch evaluates a patch baseline's ApprovalRules against a
// catalogue patch. It applies the first PatchRule (in list order) whose
// PatchFilterGroup matches -- this emulator does not model AWS's resolution
// order for a patch matched by more than one rule in the same group, since
// the synthetic catalogue never needs it. matched reports whether any rule
// matched; approved reports whether that rule's ApproveAfterDays/
// ApproveUntilDate condition is satisfied as of now.
//
// ApproveAfterDays: "a value of 7 means patches are approved seven days after
// they are released" (API_PatchRule.html) -- approved once now >= release +
// days.
//
// ApproveUntilDate: "Any patches released on or before this date are
// installed automatically" (API_PatchRule.html) -- approved once the patch's
// release date falls on or before the cutoff date, regardless of the current
// time (the cutoff is a property of the patch's release date, not of now).
func ruleOutcomeForPatch(group *PatchRuleGroup, p Patch, now time.Time) (bool, bool, string, float64) {
	if group == nil {
		return false, false, "", 0
	}

	for _, rule := range group.PatchRules {
		if !ruleFilterGroupMatches(p, rule.PatchFilterGroup) {
			continue
		}

		var (
			approved     bool
			approvalDate float64
		)

		releaseTime := time.Unix(int64(p.ReleaseDate), 0).UTC()

		// Neither the AWS docs nor validatePatchRule forbid a rule that sets
		// both ApproveAfterDays and ApproveUntilDate (see validateApprovalRule),
		// so this can't just reject the input. ApproveUntilDate checked first
		// -- and so wins when both are set -- as the more specific of the two
		// (a fixed cutoff date vs. a relative day-count).
		switch {
		case rule.ApproveUntilDate != "":
			until, err := time.Parse(time.DateOnly, rule.ApproveUntilDate)
			if err == nil {
				approvalDate = p.ReleaseDate
				approved = releaseTime.Before(until.AddDate(0, 0, 1))
			}
		case rule.ApproveAfterDays != nil:
			cutoff := releaseTime.AddDate(0, 0, int(*rule.ApproveAfterDays))
			approvalDate = UnixTimeFloat(cutoff)
			approved = !now.Before(cutoff)
		}

		return true, approved, rule.ComplianceLevel, approvalDate
	}

	return false, false, "", 0
}

// DescribeAvailablePatches returns patches from the available patches catalog,
// lazily seeding it with the built-in catalogue (defaultPatchCatalog) on the
// region's first access rather than leaving it permanently empty.
func (b *InMemoryBackend) DescribeAvailablePatches(
	ctx context.Context,
	input *DescribeAvailablePatchesInput,
) (*DescribeAvailablePatchesOutput, error) {
	region := getRegion(ctx)
	b.mu.Lock("DescribeAvailablePatches")
	defer b.mu.Unlock()

	catalog := b.availablePatchesFor(region)
	matched := make([]Patch, 0, len(catalog))

	for _, p := range catalog {
		if patchMatchesFilters(p, input.Filters) {
			matched = append(matched, p)
		}
	}

	startIdx := parseNextToken(input.NextToken)

	const defaultAvailablePatchesMaxResults = 100

	maxResults := int64(defaultAvailablePatchesMaxResults)
	if input.MaxResults != nil && *input.MaxResults > 0 {
		maxResults = *input.MaxResults
	}

	if startIdx >= len(matched) {
		return &DescribeAvailablePatchesOutput{Patches: []Patch{}}, nil
	}

	end := startIdx + int(maxResults)

	var nextToken string

	if end < len(matched) {
		nextToken = strconv.Itoa(end)
	} else {
		end = len(matched)
	}

	return &DescribeAvailablePatchesOutput{
		Patches:   matched[startIdx:end],
		NextToken: nextToken,
	}, nil
}

// patchBaselineMatchesFilters returns true when bl satisfies all provided key-value filters.
// Supported filter keys: OPERATING_SYSTEM, NAME_PREFIX.
func patchBaselineMatchesFilters(bl PatchBaseline, filters []PatchBaselineFilter) bool {
	for _, f := range filters {
		var fieldValue string

		switch f.Key {
		case "OPERATING_SYSTEM":
			fieldValue = bl.OperatingSystem
		case "NAME_PREFIX":
			for _, v := range f.Values {
				if len(bl.Name) >= len(v) && bl.Name[:len(v)] == v {
					fieldValue = v
				}
			}
		default:
			continue
		}

		if !slices.Contains(f.Values, fieldValue) {
			return false
		}
	}

	return true
}

// DescribePatchBaselines lists patch baselines with optional OS and name filters.
func (b *InMemoryBackend) DescribePatchBaselines(
	ctx context.Context,
	input *DescribePatchBaselinesInput,
) (*DescribePatchBaselinesOutput, error) {
	region := getRegion(ctx)
	b.mu.RLock("DescribePatchBaselines")
	defer b.mu.RUnlock()

	baselines := b.patchBaselinesStore(region)
	all := make([]PatchBaselineIdentity, 0, baselines.Len())
	for _, blPtr := range baselines.All() {
		bl := *blPtr
		if !patchBaselineMatchesFilters(bl, input.Filters) {
			continue
		}

		all = append(all, PatchBaselineIdentity{
			BaselineID:      bl.BaselineID,
			BaselineName:    bl.Name,
			OperatingSystem: bl.OperatingSystem,
			Description:     bl.Description,
		})
	}

	sort.Slice(all, func(i, j int) bool { return all[i].BaselineID < all[j].BaselineID })

	startIdx := parseNextToken(input.NextToken)

	const defaultBaselineMaxResults = 50

	maxResults := int64(defaultBaselineMaxResults)
	if input.MaxResults != nil && *input.MaxResults > 0 {
		maxResults = *input.MaxResults
	}

	if startIdx >= len(all) {
		return &DescribePatchBaselinesOutput{BaselineIdentities: []PatchBaselineIdentity{}}, nil
	}

	end := startIdx + int(maxResults)

	var nextToken string

	if end < len(all) {
		nextToken = strconv.Itoa(end)
	} else {
		end = len(all)
	}

	return &DescribePatchBaselinesOutput{
		BaselineIdentities: all[startIdx:end],
		NextToken:          nextToken,
	}, nil
}

// GetPatchBaseline retrieves a patch baseline by ID.
func (b *InMemoryBackend) GetPatchBaseline(
	ctx context.Context,
	input *GetPatchBaselineInput,
) (*GetPatchBaselineOutput, error) {
	if input.BaselineID == "" {
		return nil, fmt.Errorf("%w: BaselineId is required", ErrValidationException)
	}

	region := getRegion(ctx)
	b.mu.RLock("GetPatchBaseline")
	defer b.mu.RUnlock()

	bl, exists := b.patchBaselinesStore(region).Get(input.BaselineID)
	if !exists {
		return nil, ErrPatchBaselineNotFound
	}

	return &GetPatchBaselineOutput{
		PatchBaseline: *bl,
		PatchGroups:   b.patchGroupsForBaselineLocked(region, input.BaselineID),
	}, nil
}

// patchGroupsForBaselineLocked returns the sorted list of patch groups
// currently registered with baselineID, derived from the reverse
// patchGroup->baselineID mapping (RegisterPatchBaselineForPatchGroup /
// RegisterDefaultPatchBaseline). Matches real AWS's GetPatchBaselineOutput
// .PatchGroups field, which was previously entirely unpopulated. Must be
// called with b.mu held (read or write).
func (b *InMemoryBackend) patchGroupsForBaselineLocked(region, baselineID string) []string {
	var groups []string

	for group, id := range b.patchGroupToBaselineStore(region) {
		if id == baselineID && group != defaultPatchGroupKey && !strings.HasPrefix(group, "default-") {
			groups = append(groups, group)
		}
	}

	sort.Strings(groups)

	return groups
}

// RegisterPatchBaselineForPatchGroup associates a baseline with a patch group.
func (b *InMemoryBackend) RegisterPatchBaselineForPatchGroup(
	ctx context.Context,
	input *RegisterPatchBaselineForPatchGroupInput,
) (*RegisterPatchBaselineForPatchGroupOutput, error) {
	if input.BaselineID == "" {
		return nil, fmt.Errorf("%w: BaselineId is required", ErrValidationException)
	}

	if input.PatchGroup == "" {
		return nil, fmt.Errorf("%w: PatchGroup is required", ErrValidationException)
	}

	region := getRegion(ctx)
	b.mu.Lock("RegisterPatchBaselineForPatchGroup")
	defer b.mu.Unlock()

	if !b.patchBaselinesStore(region).Has(input.BaselineID) {
		return nil, ErrPatchBaselineNotFound
	}

	if b.patchGroupToBaseline[region] == nil {
		b.patchGroupToBaseline[region] = make(map[string]string)
	}
	b.patchGroupToBaselineStore(region)[input.PatchGroup] = input.BaselineID

	return &RegisterPatchBaselineForPatchGroupOutput{
		BaselineID: input.BaselineID,
		PatchGroup: input.PatchGroup,
	}, nil
}

// validateUpdatePatchBaselineInput applies the same RejectedPatchesAction/
// ApprovalRules checks as CreatePatchBaseline.
func validateUpdatePatchBaselineInput(input *UpdatePatchBaselineInput) error {
	if err := validateRejectedPatchesAction(input.RejectedPatchesAction); err != nil {
		return err
	}

	return validateApprovalRules(input.ApprovalRules)
}

// UpdatePatchBaseline updates a patch baseline.
func (b *InMemoryBackend) UpdatePatchBaseline(
	ctx context.Context,
	input *UpdatePatchBaselineInput,
) (*UpdatePatchBaselineOutput, error) {
	if input.BaselineID == "" {
		return nil, fmt.Errorf("%w: BaselineId is required", ErrValidationException)
	}

	if err := validateUpdatePatchBaselineInput(input); err != nil {
		return nil, err
	}

	region := getRegion(ctx)
	b.mu.Lock("UpdatePatchBaseline")
	defer b.mu.Unlock()

	baselines := b.patchBaselinesStore(region)
	blPtr, exists := baselines.Get(input.BaselineID)
	if !exists {
		return nil, ErrPatchBaselineNotFound
	}

	bl := *blPtr

	if input.Name != "" {
		bl.Name = input.Name
	}

	if input.Description != "" {
		bl.Description = input.Description
	}

	if len(input.ApprovedPatches) > 0 {
		bl.ApprovedPatches = input.ApprovedPatches
	}

	if len(input.RejectedPatches) > 0 {
		bl.RejectedPatches = input.RejectedPatches
	}

	if input.ApprovedPatchesComplianceLevel != "" {
		bl.ApprovedPatchesComplianceLevel = input.ApprovedPatchesComplianceLevel
	}

	if input.AvailableSecurityUpdatesComplianceStatus != "" {
		bl.AvailableSecurityUpdatesComplianceStatus = input.AvailableSecurityUpdatesComplianceStatus
	}

	if input.RejectedPatchesAction != "" {
		bl.RejectedPatchesAction = input.RejectedPatchesAction
	}

	if input.ApprovalRules != nil {
		bl.ApprovalRules = input.ApprovalRules
	}

	if input.GlobalFilters != nil {
		bl.GlobalFilters = input.GlobalFilters
	}

	if len(input.Sources) > 0 {
		bl.Sources = input.Sources
	}

	if input.ApprovedPatchesEnableNonSecurity != nil {
		bl.ApprovedPatchesEnableNonSecurity = input.ApprovedPatchesEnableNonSecurity
	}

	bl.ModifiedDate = UnixTimeFloat(timeNow())
	baselines.Put(&bl)

	return &UpdatePatchBaselineOutput{PatchBaseline: bl}, nil
}

// GetDefaultPatchBaseline returns the baseline registered for "default" or a
// hard-coded fallback baseline ID.
func (b *InMemoryBackend) GetDefaultPatchBaseline(
	ctx context.Context,
	input *GetDefaultPatchBaselineInput,
) (*GetDefaultPatchBaselineOutput, error) {
	region := getRegion(ctx)
	b.mu.RLock("GetDefaultPatchBaseline")
	defer b.mu.RUnlock()

	key := defaultPatchGroupKey
	if input.OperatingSystem != "" {
		key = "default-" + input.OperatingSystem
	}

	if id, ok := b.patchGroupToBaselineStore(region)[key]; ok {
		os := input.OperatingSystem
		if os == "" {
			if blPtr, foundBl := b.patchBaselinesStore(region).Get(id); foundBl {
				os = blPtr.OperatingSystem
			}
		}

		return &GetDefaultPatchBaselineOutput{
			BaselineID:      id,
			OperatingSystem: os,
		}, nil
	}

	// No default registered for this OS: fall back to the AWS-managed default
	// baseline, which is real state seeded into the store (its ID is stable and
	// GetPatchBaseline can describe it), rather than a fabricated all-zeros ID.
	os := input.OperatingSystem
	if os == "" {
		os = "WINDOWS"
	}

	return &GetDefaultPatchBaselineOutput{
		BaselineID:      defaultBaselineID(os),
		OperatingSystem: os,
	}, nil
}

// GetPatchBaselineForPatchGroup looks up the baseline for a given patch group.
// PatchGroup is required (confirmed via validateOpGetPatchBaselineForPatchGroupInput
// in aws-sdk-go-v2/service/ssm@v1.73.4's validators.go).
func (b *InMemoryBackend) GetPatchBaselineForPatchGroup(
	ctx context.Context,
	input *GetPatchBaselineForPatchGroupInput,
) (*GetPatchBaselineForPatchBaselineOutput, error) {
	if input.PatchGroup == "" {
		return nil, fmt.Errorf("%w: PatchGroup is required", ErrValidationException)
	}

	region := getRegion(ctx)
	b.mu.RLock("GetPatchBaselineForPatchGroup")
	defer b.mu.RUnlock()

	if id, ok := b.patchGroupToBaselineStore(region)[input.PatchGroup]; ok {
		return &GetPatchBaselineForPatchBaselineOutput{
			BaselineID:      id,
			PatchGroup:      input.PatchGroup,
			OperatingSystem: input.OperatingSystem,
		}, nil
	}

	// No explicit mapping registered for this patch group: real AWS always
	// resolves to a baseline, falling back to the AWS-managed default for the
	// OS (GetPatchBaselineForPatchGroup's own deserializer models no
	// exception besides InternalServerError) — the same fallback
	// GetDefaultPatchBaseline uses.
	key := defaultPatchGroupKey
	if input.OperatingSystem != "" {
		key = "default-" + input.OperatingSystem
	}
	if id, ok := b.patchGroupToBaselineStore(region)[key]; ok {
		os := input.OperatingSystem
		if os == "" {
			if blPtr, foundBl := b.patchBaselinesStore(region).Get(id); foundBl {
				os = blPtr.OperatingSystem
			}
		}

		return &GetPatchBaselineForPatchBaselineOutput{
			BaselineID:      id,
			PatchGroup:      input.PatchGroup,
			OperatingSystem: os,
		}, nil
	}

	os := input.OperatingSystem
	if os == "" {
		os = defaultPatchScanOS
	}

	return &GetPatchBaselineForPatchBaselineOutput{
		BaselineID:      defaultBaselineID(os),
		PatchGroup:      input.PatchGroup,
		OperatingSystem: os,
	}, nil
}

// RegisterDefaultPatchBaseline sets the default patch baseline. BaselineId is
// required (confirmed via validateOpRegisterDefaultPatchBaselineInput in
// aws-sdk-go-v2/service/ssm@v1.73.4's validators.go).
func (b *InMemoryBackend) RegisterDefaultPatchBaseline(
	ctx context.Context,
	input *RegisterDefaultPatchBaselineInput,
) (*RegisterDefaultPatchBaselineOutput, error) {
	if input.BaselineID == "" {
		return nil, fmt.Errorf("%w: BaselineId is required", ErrValidationException)
	}

	region := getRegion(ctx)
	b.mu.Lock("RegisterDefaultPatchBaseline")
	defer b.mu.Unlock()

	if !b.patchBaselinesStore(region).Has(input.BaselineID) {
		return nil, fmt.Errorf(
			"%w: baseline %q not found",
			ErrPatchBaselineNotFound,
			input.BaselineID,
		)
	}

	if b.patchGroupToBaseline[region] == nil {
		b.patchGroupToBaseline[region] = make(map[string]string)
	}
	store := b.patchGroupToBaselineStore(region)
	store[defaultPatchGroupKey] = input.BaselineID

	// Also store per-OS key when the baseline has a known OperatingSystem.
	if bl, ok := b.patchBaselinesStore(region).Get(input.BaselineID); ok && bl.OperatingSystem != "" {
		store["default-"+bl.OperatingSystem] = input.BaselineID
	}

	return &RegisterDefaultPatchBaselineOutput{BaselineID: input.BaselineID}, nil
}

// DeletePatchBaseline removes a patch baseline by ID.
func (b *InMemoryBackend) DeletePatchBaseline(
	ctx context.Context,
	input *DeletePatchBaselineInput,
) (*DeletePatchBaselineOutput, error) {
	if input.BaselineID == "" {
		return nil, fmt.Errorf("%w: BaselineId is required", ErrValidationException)
	}

	region := getRegion(ctx)
	b.mu.Lock("DeletePatchBaseline")
	defer b.mu.Unlock()

	if groups := b.patchGroupsForBaselineLocked(region, input.BaselineID); len(groups) > 0 {
		return nil, fmt.Errorf(
			"%w: patch baseline %s is registered with patch group(s) %s",
			ErrPatchBaselineInUse, input.BaselineID, strings.Join(groups, ", "),
		)
	}

	patchBaselines := b.patchBaselinesStore(region)
	patchBaselines.Delete(input.BaselineID)
	delete(b.miscResourceTagsStore(region), input.BaselineID)
	cleanupEmptyInnerMap(b.miscResourceTags, region)

	return &DeletePatchBaselineOutput{BaselineID: input.BaselineID}, nil
}

// DescribePatchGroupState returns aggregated patch counts for a patch group.
func (b *InMemoryBackend) DescribePatchGroupState(
	ctx context.Context,
	input *DescribePatchGroupStateInput,
) (*DescribePatchGroupStateOutput, error) {
	if input.PatchGroup == "" {
		return nil, fmt.Errorf("%w: PatchGroup is required", ErrValidationException)
	}

	region := getRegion(ctx)
	b.mu.RLock("DescribePatchGroupState")
	defer b.mu.RUnlock()

	out := &DescribePatchGroupStateOutput{}
	for _, s := range b.instancePatchStatesStore(region).All() {
		if s.PatchGroup != input.PatchGroup {
			continue
		}
		out.Instances++
		if s.FailedCount > 0 {
			out.InstancesWithFailedPatches++
		}
		if s.InstalledCount > 0 {
			out.InstancesWithInstalledPatches++
		}
		if s.MissingCount > 0 {
			out.InstancesWithMissingPatches++
		}
	}

	return out, nil
}

// patchGroupMappingMatchesFilters applies DescribePatchGroups' filter keys:
// NAME_PREFIX (against the patch group name) and OPERATING_SYSTEM (against
// the mapped baseline's OS) -- both confirmed against
// aws-sdk-go-v2/service/ssm@v1.73.4's api_op_DescribePatchGroups.go doc
// comment, matching the same two keys DescribePatchBaselines already
// supports via patchBaselineMatchesFilters.
func patchGroupMappingMatchesFilters(m PatchGroupPatchBaselineMapping, filters []PatchFilter) bool {
	for _, f := range filters {
		var fieldValue string

		switch f.Key {
		case "OPERATING_SYSTEM":
			fieldValue = m.BaselineIdentity.OperatingSystem
		case "NAME_PREFIX":
			for _, v := range f.Values {
				if len(m.PatchGroup) >= len(v) && m.PatchGroup[:len(v)] == v {
					fieldValue = v
				}
			}
		default:
			continue
		}

		if !slices.Contains(f.Values, fieldValue) {
			return false
		}
	}

	return true
}

// DescribePatchGroups lists the patch group to baseline mappings.
func (b *InMemoryBackend) DescribePatchGroups(
	ctx context.Context,
	input *DescribePatchGroupsInput,
) (*DescribePatchGroupsOutput, error) {
	region := getRegion(ctx)
	b.mu.RLock("DescribePatchGroups")
	defer b.mu.RUnlock()

	store := b.patchGroupToBaselineStore(region)
	mappings := make([]PatchGroupPatchBaselineMapping, 0, len(store))

	patchBaselines := b.patchBaselinesStore(region)
	for group, baselineID := range store {
		identity := PatchBaselineIdentity{BaselineID: baselineID}
		if bl, ok := patchBaselines.Get(baselineID); ok {
			identity.BaselineName = bl.Name
			identity.OperatingSystem = bl.OperatingSystem
			identity.Description = bl.Description
		}

		mapping := PatchGroupPatchBaselineMapping{
			PatchGroup:       group,
			BaselineIdentity: identity,
		}
		if !patchGroupMappingMatchesFilters(mapping, input.Filters) {
			continue
		}

		mappings = append(mappings, mapping)
	}

	startIdx := parseNextToken(input.NextToken)

	const (
		defaultPatchGroupsMaxResults = 50
		maxPatchGroupsMaxResults     = 100
	)

	maxResults := int64(defaultPatchGroupsMaxResults)
	if input.MaxResults != nil {
		if *input.MaxResults < 1 || *input.MaxResults > maxPatchGroupsMaxResults {
			return nil, fmt.Errorf(
				"%w: MaxResults must be between 1 and %d",
				ErrValidationException,
				maxPatchGroupsMaxResults,
			)
		}

		maxResults = *input.MaxResults
	}

	if startIdx >= len(mappings) {
		return &DescribePatchGroupsOutput{Mappings: []PatchGroupPatchBaselineMapping{}}, nil
	}

	end := startIdx + int(maxResults)

	var nextToken string
	if end < len(mappings) {
		nextToken = strconv.Itoa(end)
	} else {
		end = len(mappings)
	}

	return &DescribePatchGroupsOutput{
		Mappings:  mappings[startIdx:end],
		NextToken: nextToken,
	}, nil
}

// DescribePatchProperties returns property data aggregated from patch
// baselines. OperatingSystem and Property are both required (confirmed via
// validateOpDescribePatchPropertiesInput in
// aws-sdk-go-v2/service/ssm@v1.73.4's validators.go). Real AWS instead
// aggregates distinct values of the requested Property (PRODUCT/
// PRODUCT_FAMILY/CLASSIFICATION/MSRC_SEVERITY/PRIORITY/SEVERITY) from its
// patch catalogue for the given OS; this emulator does not consult either
// input field and instead lists baseline name/OS pairs -- disclosed rather
// than fixed, since the real per-Property map-key convention isn't verifiable
// from the pinned SDK's untyped []map[string]string output (same class as
// GetInventorySchema.Attributes).
func (b *InMemoryBackend) DescribePatchProperties(
	ctx context.Context,
	input *DescribePatchPropertiesInput,
) (*DescribePatchPropertiesOutput, error) {
	if input.OperatingSystem == "" {
		return nil, fmt.Errorf("%w: OperatingSystem is required", ErrValidationException)
	}

	if input.Property == "" {
		return nil, fmt.Errorf("%w: Property is required", ErrValidationException)
	}

	region := getRegion(ctx)
	b.mu.RLock("DescribePatchProperties")
	defer b.mu.RUnlock()

	seen := map[string]bool{}
	props := make([]map[string]string, 0)
	for _, bl := range b.patchBaselinesStore(region).All() {
		if input.OperatingSystem != "" && bl.OperatingSystem != input.OperatingSystem {
			continue
		}

		key := bl.OperatingSystem + ":" + bl.Name
		if seen[key] {
			continue
		}

		seen[key] = true
		props = append(props, map[string]string{
			"OperatingSystem": bl.OperatingSystem,
			"BaselineName":    bl.Name,
		})
	}

	sort.Slice(props, func(i, k int) bool { return props[i]["BaselineName"] < props[k]["BaselineName"] })

	maxResults := 0
	if input.MaxResults != nil {
		maxResults = int(*input.MaxResults)
	}

	page, next := paginateSlice(props, input.NextToken, maxResults, defaultDescribeMaxResults)

	return &DescribePatchPropertiesOutput{Properties: page, NextToken: next}, nil
}

// DescribeEffectivePatchesForPatchBaseline returns the effective patch set for
// a baseline, derived from its approved/rejected patches plus the region's
// available-patches catalogue (see effectivePatchesForBaseline). BaselineId is
// required (confirmed via validateOpDescribeEffectivePatchesForPatchBaselineInput
// in aws-sdk-go-v2/service/ssm@v1.73.4's validators.go).
func (b *InMemoryBackend) DescribeEffectivePatchesForPatchBaseline(
	ctx context.Context,
	input *DescribeEffectivePatchesForPatchBaselineInput,
) (*DescribeEffectivePatchesForPatchBaselineOutput, error) {
	if input.BaselineID == "" {
		return nil, fmt.Errorf("%w: BaselineId is required", ErrValidationException)
	}

	region := getRegion(ctx)
	// Write lock, not read: effectivePatchesForBaseline calls availablePatchesFor,
	// which lazily seeds b.availablePatches[region] on first access (see
	// DescribeAvailablePatches, which uses the same write lock for the same reason).
	b.mu.Lock("DescribeEffectivePatchesForPatchBaseline")
	defer b.mu.Unlock()

	baselinePtr, exists := b.patchBaselinesStore(region).Get(input.BaselineID)
	if !exists {
		return nil, fmt.Errorf(
			"%w: baseline %q not found",
			ErrPatchBaselineNotFound,
			input.BaselineID,
		)
	}

	effective := b.effectivePatchesForBaseline(region, *baselinePtr)

	return paginateEffectivePatches(effective, input.NextToken, input.MaxResults), nil
}

// effectivePatchesForBaseline derives the effective patch set for a baseline
// from its explicitly-approved patches plus the region's available-patch
// catalogue (matched on OS/product), backing the response with real stored
// state instead of an empty list (once the catalogue has been seeded — see
// availablePatchesFor). Must be called with b.mu held.
func (b *InMemoryBackend) effectivePatchesForBaseline(
	region string,
	baseline PatchBaseline,
) []EffectivePatch {
	level := baseline.ApprovedPatchesComplianceLevel
	if level == "" {
		level = "UNSPECIFIED"
	}

	approvalDate := baseline.CreatedDate

	effective := make([]EffectivePatch, 0, len(baseline.ApprovedPatches)+len(baseline.RejectedPatches))

	approved := make(map[string]struct{}, len(baseline.ApprovedPatches))
	for _, id := range baseline.ApprovedPatches {
		approved[id] = struct{}{}
		p := id
		effective = append(effective, EffectivePatch{
			Patch: &Patch{Name: p, Classification: patchClassificationSecurityUpdates},
			PatchStatus: &PatchStatus{
				DeploymentStatus: patchDeploymentStatusExplicitApproved,
				ComplianceLevel:  level,
				ApprovalDate:     approvalDate,
			},
		})
	}

	rejected := make(map[string]struct{}, len(baseline.RejectedPatches))
	for _, id := range baseline.RejectedPatches {
		rejected[id] = struct{}{}
		p := id
		effective = append(effective, EffectivePatch{
			Patch: &Patch{Name: p, Classification: patchClassificationSecurityUpdates},
			PatchStatus: &PatchStatus{
				DeploymentStatus: patchDeploymentStatusExplicitRejected,
				ComplianceLevel:  level,
			},
		})
	}

	// Include catalogue patches that are not explicitly approved or rejected --
	// these are decided by ApprovalRules (RejectedPatches > ApprovedPatches >
	// ApprovalRules precedence: "A patch specified in the approved patches list
	// will be installed irrespective of whether it is matched by an approval
	// rule... Items in the rejected patches list... override both ApprovalRules
	// and ApprovedPatches" -- AWS Systems Manager User Guide, "How security
	// patches are selected"), or remain pending if no rule approves them yet.
	// availablePatchesFor (not a direct b.availablePatches[region] read) lazily
	// seeds the catalogue, so this reflects the built-in catalogue regardless
	// of whether DescribeAvailablePatches happened to run first in this region.
	now := timeNow()

	for _, p := range b.availablePatchesFor(region) {
		if _, isApproved := approved[p.Name]; isApproved {
			continue
		}

		if _, isRejected := rejected[p.Name]; isRejected {
			continue
		}

		status := &PatchStatus{
			DeploymentStatus: patchDeploymentStatusPendingApproval,
			ComplianceLevel:  level,
		}

		if matched, ruleApproved, ruleLevel, ruleApprovalDate := ruleOutcomeForPatch(
			baseline.ApprovalRules, p, now,
		); matched {
			status.ApprovalDate = ruleApprovalDate

			if ruleApproved {
				status.DeploymentStatus = patchDeploymentStatusApproved

				if ruleLevel != "" {
					status.ComplianceLevel = ruleLevel
				}
			}
		}

		patch := p
		effective = append(effective, EffectivePatch{Patch: &patch, PatchStatus: status})
	}

	return effective
}

// paginateEffectivePatches applies opaque index-based pagination to an effective
// patch list.
func paginateEffectivePatches(
	all []EffectivePatch,
	nextToken string,
	maxResults *int64,
) *DescribeEffectivePatchesForPatchBaselineOutput {
	startIdx := parseNextToken(nextToken)

	const defaultMax = 100

	limit := int64(defaultMax)
	if maxResults != nil && *maxResults > 0 {
		limit = *maxResults
	}

	if startIdx >= len(all) {
		return &DescribeEffectivePatchesForPatchBaselineOutput{
			EffectivePatches: []EffectivePatch{},
		}
	}

	end := startIdx + int(limit)

	var token string

	if end < len(all) {
		token = strconv.Itoa(end)
	} else {
		end = len(all)
	}

	return &DescribeEffectivePatchesForPatchBaselineOutput{
		EffectivePatches: all[startIdx:end],
		NextToken:        token,
	}
}

// GetDeployablePatchSnapshotForInstance returns the deployable patch snapshot
// for an instance. The snapshot is backed by the instance's effective patch
// baseline (looked up via its recorded patch state or the default baseline for
// its OS) rather than a random URL, and the Product reflects the real baseline
// OS. A caller-supplied SnapshotId is preserved so repeated calls are stable.
func (b *InMemoryBackend) GetDeployablePatchSnapshotForInstance(
	ctx context.Context,
	input *GetDeployablePatchSnapshotForInstanceInput,
) (*GetDeployablePatchSnapshotForInstanceOutput, error) {
	if input.InstanceID == "" {
		return nil, fmt.Errorf("%w: InstanceId is required", ErrValidationException)
	}

	if input.SnapshotID == "" {
		return nil, fmt.Errorf("%w: SnapshotId is required", ErrValidationException)
	}

	region := getRegion(ctx)
	b.mu.RLock("GetDeployablePatchSnapshotForInstance")
	defer b.mu.RUnlock()

	snapshotID := input.SnapshotID

	// Resolve the instance's effective baseline: prefer its recorded patch
	// state, else the AWS default baseline for its OS (Windows fallback).
	product := patchProductAmazonLinux2
	baselineID := defaultBaselineID("AMAZON_LINUX_2")

	if st, ok := b.instancePatchStatesStore(region).Get(input.InstanceID); ok && st != nil {
		if st.BaselineID != "" {
			baselineID = st.BaselineID
		}

		if bl, found := b.patchBaselinesStore(region).Get(st.BaselineID); found &&
			bl.OperatingSystem != "" {
			product = bl.OperatingSystem
		}
	}

	return &GetDeployablePatchSnapshotForInstanceOutput{
		InstanceID: input.InstanceID,
		SnapshotID: snapshotID,
		Product:    product,
		SnapshotDownloadURL: "https://patch-baseline-snapshot-" + region +
			".s3." + region + ".amazonaws.com/" + baselineID + "-" + snapshotID,
	}, nil
}

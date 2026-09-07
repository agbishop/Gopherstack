package ssm

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/awsmeta"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// validParamNameRegex matches only alphanumeric, ., -, _, and / characters.
var validParamNameRegex = regexp.MustCompile(`^[a-zA-Z0-9._\-/]+$`)

const maxParamNameLength = 2048

// maxParamHierarchyLevels is the maximum number of "/"-delimited path segments
// allowed in a parameter name. AWS: "A parameter name hierarchy can have a
// maximum of 15 levels" — e.g. /L1/L2/.../L15/name is valid, one more is not.
const maxParamHierarchyLevels = 15

// validateParameterName returns a ParameterPatternMismatchException error
// when the name is invalid -- PutParameter's own declared exception for a
// malformed Name (ssm@v1.73.4 deserializers.go:13901), not the generic
// ValidationException.
func validateParameterName(name string) error {
	if len(name) > maxParamNameLength {
		return fmt.Errorf(
			"%w: parameter name exceeds maximum length of %d",
			ErrParameterNamePattern,
			maxParamNameLength,
		)
	}

	if strings.Contains(name, "//") {
		return fmt.Errorf(
			"%w: parameter name must not contain double slashes",
			ErrParameterNamePattern,
		)
	}

	lower := strings.ToLower(strings.TrimPrefix(name, "/"))
	reservedPrefixes := []string{"ssm", "aws", "amazon"}
	for _, prefix := range reservedPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return fmt.Errorf(
				"%w: parameter name must not start with reserved namespace %q",
				ErrParameterNamePattern,
				prefix,
			)
		}
	}

	if !validParamNameRegex.MatchString(name) {
		return fmt.Errorf("%w: parameter name contains invalid characters", ErrParameterNamePattern)
	}

	if levels := parameterHierarchyLevels(name); levels > maxParamHierarchyLevels {
		return fmt.Errorf(
			"%w: parameter name hierarchy has %d levels, maximum is %d",
			ErrHierarchyLevelLimitExceeded, levels, maxParamHierarchyLevels,
		)
	}

	return nil
}

// parameterHierarchyLevels counts the "/"-delimited, non-empty segments of a
// parameter name, including the final name segment itself. AWS counts a
// leading slash as the hierarchy root (not a level) — e.g. "/a/b/c" has 3
// levels, matching how AWS reports HierarchyLevelLimitExceededException.
func parameterHierarchyLevels(name string) int {
	segments := strings.Split(name, "/")

	levels := 0
	for _, s := range segments {
		if s != "" {
			levels++
		}
	}

	return levels
}
func (b *InMemoryBackend) parametersStore(region string) *store.Table[Parameter] {
	return getOrCreateTable(b, b.parameters, "parameters", region, parameterKeyFn)
}
func (b *InMemoryBackend) historyStore(region string) map[string][]ParameterHistory {
	return b.history[region]
}
func (b *InMemoryBackend) parameterLabelsStore(region string) map[string]map[int64][]string {
	return b.parameterLabels[region]
}

// isValidParameterType returns true when t is one of the three supported SSM
// parameter types. Real AWS rejects an unrecognised type with
// UnsupportedParameterType, PutParameter's own declared exception for this
// (ssm@v1.73.4 deserializers.go:13910).
func isValidParameterType(t string) bool {
	switch t {
	case StringType, StringListType, SecureStringType:
		return true
	}

	return false
}

// isValidDataType returns true when dt is a supported SSM DataType value.
func isValidDataType(dt string) bool {
	switch dt {
	case "text", "aws:ec2:image", "aws:ssm:integration-default-configuration-directory":
		return true
	}

	return false
}

// validateAllowedPattern compiles the pattern and checks the value against
// it, returning InvalidAllowedPatternException -- PutParameter's own
// declared exception for this (ssm@v1.73.4 deserializers.go:13880) -- not
// the generic ValidationException.
func validateAllowedPattern(pattern, value string) error {
	if pattern == "" {
		return nil
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("%w: invalid AllowedPattern: %w", ErrInvalidAllowedPattern, err)
	}

	if !re.MatchString(value) {
		return fmt.Errorf(
			"%w: parameter value does not match AllowedPattern %q",
			ErrInvalidAllowedPattern, pattern,
		)
	}

	return nil
}

// resolveTier canonicalises the tier string and enforces per-tier value size
// limits, returning the resolved tier name or an error.
//
// AWS auto-upgrades Intelligent-Tiering parameters to Advanced whenever the
// request needs a capability Standard doesn't support — a value over 4 KiB or
// parameter policies attached — rather than rejecting the request. An
// explicit Standard tier still hard-fails on those same conditions, since the
// caller opted out of auto-selection.
func resolveTier(tier, value, policies string) (string, error) {
	if tier == "" {
		tier = tierStandard
	}

	needsAdvanced := len(value) > maxStandardValueBytes || policies != ""

	switch tier {
	case tierIntelligentTiering:
		if needsAdvanced {
			tier = tierAdvanced
		}
	case tierStandard:
		if len(value) > maxStandardValueBytes {
			return "", fmt.Errorf(
				"%w: parameter value exceeds %d bytes for Standard tier",
				ErrValidationException, maxStandardValueBytes,
			)
		}

		if policies != "" {
			return "", fmt.Errorf(
				"%w: parameter policies are only supported for Advanced tier parameters",
				ErrValidationException,
			)
		}
	case tierAdvanced:
		// already Advanced; size checked below.
	default:
		return "", fmt.Errorf("%w: invalid Tier %q", ErrValidationException, tier)
	}

	if tier == tierAdvanced && len(value) > maxAdvancedValueBytes {
		return "", fmt.Errorf(
			"%w: parameter value exceeds %d bytes for Advanced tier",
			ErrValidationException, maxAdvancedValueBytes,
		)
	}

	return tier, nil
}

// parameterARN builds the ARN for a parameter. AWS omits the leading slash
// between "parameter" and the name (so /a/b → parameter/a/b, and a relative
// name "foo" → parameter/foo).
func parameterARN(region, account, name string) string {
	trimmed := strings.TrimPrefix(name, "/")

	return arn.Build("ssm", region, account, fmt.Sprintf("parameter/%s", trimmed))
}

// splitParameterSelector splits a parameter name into its base name and the
// selector suffix (version or label). A selector is introduced by the last ":"
// in the name. AWS parameter names may legitimately contain "/" but never ":",
// so any ":" delimits a selector. Returns (baseName, selector) where selector
// is the part after the colon ("" when no selector is present).
func splitParameterSelector(name string) (string, string) {
	idx := strings.LastIndex(name, ":")
	if idx < 0 {
		return name, ""
	}

	return name[:idx], name[idx+1:]
}

// resolveParameterSelector returns the Parameter for the given base name and
// selector. The selector may be empty (latest version), a numeric version, or a
// label. It mirrors AWS error semantics:
//   - unknown parameter             → ParameterNotFound
//   - numeric selector, no version  → ParameterVersionNotFound
//   - label selector, no match      → ParameterNotFound
//
// Caller must hold at least the read lock.
func (b *InMemoryBackend) resolveParameterSelector(
	region, baseName, selector string,
) (Parameter, error) {
	currentPtr, exists := b.parametersStore(region).Get(baseName)
	if !exists {
		return Parameter{}, ErrParameterNotFound
	}

	current := *currentPtr

	if selector == "" {
		return current, nil
	}

	history := b.historyStore(region)[baseName]

	// Numeric selector → specific version.
	if version, err := strconv.ParseInt(selector, 10, 64); err == nil {
		return b.parameterAtVersion(current, history, version)
	}

	// Label selector → resolve label to a version via the labels store.
	version, ok := b.versionForLabel(region, baseName, selector)
	if !ok {
		return Parameter{}, ErrParameterNotFound
	}

	param, err := b.parameterAtVersion(current, history, version)
	if err != nil {
		// A label pointing at a missing version behaves like a missing parameter.
		return Parameter{}, ErrParameterNotFound
	}

	return param, nil
}

// parameterAtVersion materializes a Parameter for a specific version from the
// history list, falling back to the current record when the requested version
// is the current one. Returns ParameterVersionNotFound when no such version
// exists.
func (b *InMemoryBackend) parameterAtVersion(
	current Parameter, history []ParameterHistory, version int64,
) (Parameter, error) {
	if version == current.Version {
		return current, nil
	}

	for _, h := range history {
		if h.Version != version {
			continue
		}

		return Parameter{
			Name:             h.Name,
			Type:             h.Type,
			Value:            h.Value,
			Description:      h.Description,
			KeyID:            h.KeyID,
			Tier:             h.Tier,
			AllowedPattern:   h.AllowedPattern,
			DataType:         h.DataType,
			Version:          h.Version,
			LastModifiedDate: h.LastModifiedDate,
		}, nil
	}

	return Parameter{}, ErrParameterVersionNotFound
}

// versionForLabel returns the version a label currently points at. Caller must
// hold at least the read lock.
func (b *InMemoryBackend) versionForLabel(region, name, label string) (int64, bool) {
	versionLabels, ok := b.parameterLabelsStore(region)[name]
	if !ok {
		return 0, false
	}

	for version, labels := range versionLabels {
		if slices.Contains(labels, label) {
			return version, true
		}
	}

	return 0, false
}

// validatePutParameterInput validates the pre-lock fields of a PutParameter
// request and returns the resolved dataType and tier.
func validatePutParameterInput(input *PutParameterInput) (putParameterValidated, error) {
	if err := validateParameterName(input.Name); err != nil {
		return putParameterValidated{}, err
	}

	if !isValidParameterType(input.Type) {
		return putParameterValidated{}, fmt.Errorf(
			"%w: invalid Type %q, must be String, StringList, or SecureString",
			ErrUnsupportedParameterType, input.Type,
		)
	}

	dataType := input.DataType
	if dataType == "" {
		dataType = "text"
	}

	if !isValidDataType(dataType) {
		// No declared PutParameter exception fits an invalid DataType
		// (checked ssm@v1.73.4 deserializers.go); ValidationException stays.
		return putParameterValidated{}, fmt.Errorf(
			"%w: invalid DataType %q", ErrValidationException, dataType,
		)
	}

	if err := validateAllowedPattern(input.AllowedPattern, input.Value); err != nil {
		return putParameterValidated{}, err
	}

	tier, err := resolveTier(input.Tier, input.Value, input.Policies)
	if err != nil {
		return putParameterValidated{}, err
	}

	return putParameterValidated{dataType: dataType, tier: tier}, nil
}

// applyPutParameterTagsLocked applies PutParameterInput.Tags -- only
// meaningful when creating a brand-new parameter (api_op_PutParameter.go:203:
// "To add tags to an existing Systems Manager parameter, use the
// AddTagsToResource operation"). Must be called with b.mu held for writing.
func (b *InMemoryBackend) applyPutParameterTagsLocked(region, name string, tagList []Tag) {
	if len(tagList) == 0 {
		return
	}

	if b.tags[region] == nil {
		b.tags[region] = make(map[string]*tags.Tags)
	}

	tagsStore := b.tagsStore(region)
	if tagsStore[name] == nil {
		tagsStore[name] = tags.New("ssm." + name + ".tags")
	}

	for _, t := range tagList {
		tagsStore[name].Set(t.Key, t.Value)
	}
}

func (b *InMemoryBackend) PutParameter(
	ctx context.Context,
	input *PutParameterInput,
) (*PutParameterOutput, error) {
	validated, err := validatePutParameterInput(input)
	if err != nil {
		return nil, err
	}

	dataType := validated.dataType
	tier := validated.tier
	region := getRegion(ctx)

	b.mu.Lock("PutParameter")
	defer b.mu.Unlock()

	params := b.parametersStore(region)
	existingPtr, exists := params.Get(input.Name)
	if exists && !input.Overwrite {
		return nil, ErrParameterAlreadyExists
	}

	version := int64(1)
	if exists {
		version = existingPtr.Version + 1
	}

	// Encrypt if SecureString type; use KMS when a KeyID is specified.
	value := input.Value
	if input.Type == SecureStringType {
		var encErr error
		value, encErr = b.encryptSSMValue(input.KeyID, input.Value)
		if encErr != nil {
			return nil, encErr
		}
	}

	param := Parameter{
		Name:             input.Name,
		Type:             input.Type,
		Value:            value,
		Description:      input.Description,
		Version:          version,
		LastModifiedDate: UnixTimeFloat(time.Now()),
		KeyID:            input.KeyID,
		Tier:             tier,
		AllowedPattern:   input.AllowedPattern,
		DataType:         dataType,
		Policies:         input.Policies,
	}

	// AWS retains only the most recent maxHistoryCap versions of a parameter,
	// automatically deleting the oldest version when a new one is created. If
	// the oldest version has a label attached, AWS refuses to delete it (and
	// therefore refuses to create the new version) with
	// ParameterMaxVersionLimitExceeded, since that would silently orphan the
	// label. Check this before mutating any state.
	if existingHist := b.historyStore(region)[input.Name]; len(existingHist) >= maxHistoryCap {
		oldest := existingHist[0]
		if labels := b.parameterLabelsStore(region)[input.Name][oldest.Version]; len(labels) > 0 {
			return nil, fmt.Errorf(
				"%w: version %d, the oldest version, can't be deleted because it has a label"+
					" associated with it. Move the label to another version of the parameter,"+
					" and try again",
				ErrParameterMaxVersionLimitExceeded, oldest.Version,
			)
		}
	}

	params.Put(&param)

	if !exists {
		b.applyPutParameterTagsLocked(region, input.Name, input.Tags)
	}

	// A write always resets LastModifiedDate and wholesale-replaces Policies,
	// invalidating any previously-recorded policy-notification dedupe state
	// for this parameter (see clearParameterPolicyNotificationStateLocked).
	b.clearParameterPolicyNotificationStateLocked(region, input.Name)

	b.recordParameterHistoryLocked(region, input, value, dataType, tier, param.LastModifiedDate, version)

	return &PutParameterOutput{Version: version, Tier: tier}, nil
}

// recordParameterHistoryLocked appends a ParameterHistory entry for a
// PutParameter write (store encrypted value for SecureString) and evicts the
// oldest entry once history exceeds maxHistoryCap. Must be called with b.mu
// held for writing.
func (b *InMemoryBackend) recordParameterHistoryLocked(
	region string, input *PutParameterInput, value, dataType, tier string, lastModifiedDate float64, version int64,
) {
	paramHistory := ParameterHistory{
		Name:             input.Name,
		Type:             input.Type,
		Value:            value,
		Version:          version,
		LastModifiedDate: lastModifiedDate,
		Labels:           []string{},
		KeyID:            input.KeyID,
		Tier:             tier,
		AllowedPattern:   input.AllowedPattern,
		DataType:         dataType,
		Description:      input.Description,
		Policies:         input.Policies,
	}
	if b.history[region] == nil {
		b.history[region] = make(map[string][]ParameterHistory)
	}
	history := b.historyStore(region)
	history[input.Name] = append(history[input.Name], paramHistory)

	// Cap history to the most recent maxHistoryCap entries to prevent unbounded growth.
	if len(history[input.Name]) > maxHistoryCap {
		evicted := history[input.Name][:len(history[input.Name])-maxHistoryCap]
		history[input.Name] = history[input.Name][len(history[input.Name])-maxHistoryCap:]

		// Evicted versions can never be labeled here (PutParameter's pre-check
		// bars evicting a labeled oldest version) but their version-label map
		// entries — created lazily on first label — may still exist as empty
		// slices; drop them so parameterLabels doesn't grow unboundedly.
		if versionLabels := b.parameterLabelsStore(region)[input.Name]; versionLabels != nil {
			for _, ev := range evicted {
				delete(versionLabels, ev.Version)
			}
		}
	}
}

// GetParameter retrieves a single parameter. The name may carry a version or
// label selector suffix (e.g. "/a/b:3" or "/a/b:prod"), in which case the
// matching version is returned and echoed back via Parameter.Selector. The
// response always includes the parameter ARN.
func (b *InMemoryBackend) GetParameter(
	ctx context.Context,
	input *GetParameterInput,
) (*GetParameterOutput, error) {
	region := getRegion(ctx)
	account := awsmeta.Account(ctx)

	baseName, selector := splitParameterSelector(input.Name)

	b.mu.RLock("GetParameter")
	defer b.mu.RUnlock()

	param, err := b.resolveParameterSelector(region, baseName, selector)
	if err != nil {
		return nil, err
	}

	// Decrypt SecureString if WithDecryption is true; propagate errors.
	if input.WithDecryption && param.Type == SecureStringType {
		decrypted, derr := b.decryptSSMValue(param.KeyID, param.Value)
		if derr != nil {
			return nil, fmt.Errorf("%w: %w", ErrValidationException, derr)
		}

		param.Value = decrypted
	}

	param.ARN = parameterARN(region, account, baseName)
	if selector != "" {
		param.Selector = ":" + selector
	}

	return &GetParameterOutput{Parameter: param.toParameterOutput()}, nil
}

// GetParameters retrieves multiple parameters. Missing names are returned as InvalidParameters.
func (b *InMemoryBackend) GetParameters(
	ctx context.Context,
	input *GetParametersInput,
) (*GetParametersOutput, error) {
	region := getRegion(ctx)
	account := awsmeta.Account(ctx)

	b.mu.RLock("GetParameters")
	defer b.mu.RUnlock()

	output := &GetParametersOutput{
		Parameters:        make([]ParameterOutput, 0, len(input.Names)),
		InvalidParameters: make([]string, 0, len(input.Names)),
	}

	for _, name := range input.Names {
		baseName, selector := splitParameterSelector(name)

		param, err := b.resolveParameterSelector(region, baseName, selector)
		if err != nil {
			// Unknown name, missing version, or unresolvable label all become
			// invalid parameters in GetParameters (AWS does not fail the call).
			output.InvalidParameters = append(output.InvalidParameters, name)

			continue
		}

		// Decrypt SecureString if WithDecryption is true
		if input.WithDecryption && param.Type == SecureStringType {
			decrypted, derr := b.decryptSSMValue(param.KeyID, param.Value)
			if derr != nil {
				// If decryption fails, add to invalid parameters
				output.InvalidParameters = append(output.InvalidParameters, name)

				continue
			}
			param.Value = decrypted
		}

		param.ARN = parameterARN(region, account, baseName)
		if selector != "" {
			param.Selector = ":" + selector
		}
		output.Parameters = append(output.Parameters, param.toParameterOutput())
	}

	return output, nil
}

// DeleteParameter deletes a single parameter.
func (b *InMemoryBackend) DeleteParameter(
	ctx context.Context,
	input *DeleteParameterInput,
) (*DeleteParameterOutput, error) {
	region := getRegion(ctx)

	b.mu.Lock("DeleteParameter")
	defer b.mu.Unlock()

	params := b.parametersStore(region)
	if !params.Has(input.Name) {
		return nil, ErrParameterNotFound
	}

	params.Delete(input.Name)
	delete(b.historyStore(region), input.Name)
	delete(b.parameterLabelsStore(region), input.Name)
	b.clearParameterPolicyNotificationStateLocked(region, input.Name)

	tags := b.tagsStore(region)
	if t, ok := tags[input.Name]; ok {
		t.Close()
		delete(tags, input.Name)
	}

	b.cleanupEmptyParamRegion(region)

	return &DeleteParameterOutput{}, nil
}

// DeleteParameters deletes multiple parameters.
func (b *InMemoryBackend) DeleteParameters(
	ctx context.Context,
	input *DeleteParametersInput,
) (*DeleteParametersOutput, error) {
	region := getRegion(ctx)

	b.mu.Lock("DeleteParameters")
	defer b.mu.Unlock()

	params := b.parametersStore(region)
	history := b.historyStore(region)
	tags := b.tagsStore(region)

	output := &DeleteParametersOutput{
		DeletedParameters: make([]string, 0, len(input.Names)),
		InvalidParameters: make([]string, 0, len(input.Names)),
	}

	for _, name := range input.Names {
		if params.Has(name) {
			params.Delete(name)
			delete(history, name)
			delete(b.parameterLabelsStore(region), name)
			b.clearParameterPolicyNotificationStateLocked(region, name)
			if t, ok := tags[name]; ok {
				t.Close()
				delete(tags, name)
			}
			output.DeletedParameters = append(output.DeletedParameters, name)
		} else {
			output.InvalidParameters = append(output.InvalidParameters, name)
		}
	}

	b.cleanupEmptyParamRegion(region)

	return output, nil
}

// GetParameterHistory retrieves all versions of a parameter.
// buildReversedHistory returns the history list in reverse order (newest first),
// populating labels from parameterLabels and optionally decrypting SecureString values.
func (b *InMemoryBackend) buildReversedHistory(
	historyList []ParameterHistory,
	parameterLabels map[string]map[int64][]string,
	name string,
	withDecryption bool,
) []ParameterHistory {
	n := len(historyList)
	reversed := make([]ParameterHistory, n)
	for i, h := range historyList {
		entry := h
		if versionLabels, ok := parameterLabels[name]; ok {
			if labels, ok2 := versionLabels[entry.Version]; ok2 && len(labels) > 0 {
				entry.Labels = labels
			}
		}
		if withDecryption && entry.Type == SecureStringType {
			if decrypted, err := b.decryptSSMValue(entry.KeyID, entry.Value); err == nil {
				entry.Value = decrypted
			}
		}
		reversed[n-1-i] = entry
	}

	return reversed
}
func (b *InMemoryBackend) GetParameterHistory(
	ctx context.Context,
	input *GetParameterHistoryInput,
) (*GetParameterHistoryOutput, error) {
	region := getRegion(ctx)

	b.mu.RLock("GetParameterHistory")
	defer b.mu.RUnlock()

	historyList, exists := b.historyStore(region)[input.Name]
	if !exists {
		return nil, ErrParameterNotFound
	}

	maxResults := int64(maxHistoryResults)
	if input.MaxResults != nil {
		if *input.MaxResults < 1 || *input.MaxResults > maxHistoryResults {
			return nil, fmt.Errorf(
				"%w: MaxResults must be between 1 and %d",
				ErrValidationException,
				maxHistoryResults,
			)
		}

		maxResults = *input.MaxResults
	}

	reversed := b.buildReversedHistory(
		historyList,
		b.parameterLabelsStore(region),
		input.Name,
		input.WithDecryption,
	)
	n := len(reversed)

	startIdx := parseNextToken(input.NextToken)

	if startIdx >= n {
		return &GetParameterHistoryOutput{Parameters: []ParameterHistoryOutput{}}, nil
	}

	end := startIdx + int(maxResults)

	var nextToken string

	if end < n {
		nextToken = strconv.Itoa(end)
	} else {
		end = n
	}

	page := make([]ParameterHistoryOutput, 0, end-startIdx)
	for _, h := range reversed[startIdx:end] {
		page = append(page, h.toParameterHistoryOutput())
	}

	return &GetParameterHistoryOutput{
		Parameters: page,
		NextToken:  nextToken,
	}, nil
}

// ListAll returns all parameters sorted by name (useful for Dashboard UI).
func (b *InMemoryBackend) ListAll(ctx context.Context) []Parameter {
	region := getRegion(ctx)

	b.mu.RLock("ListAll")
	defer b.mu.RUnlock()

	paramsTable := b.parametersStore(region)
	params := make([]Parameter, 0, paramsTable.Len())
	for _, p := range paramsTable.All() {
		params = append(params, *p)
	}

	sort.Slice(params, func(i, j int) bool {
		return strings.Compare(params[i].Name, params[j].Name) < 0
	})

	return params
}

// paramByPathMatchesFilters converts a Parameter to ParameterMetadata and
// delegates to paramMatchesFilters, keeping GetParametersByPath's complexity low.
func paramByPathMatchesFilters(param Parameter, filters []ParameterFilter) bool {
	meta := ParameterMetadata{
		Name:     param.Name,
		Type:     param.Type,
		KeyID:    param.KeyID,
		Tier:     param.Tier,
		DataType: param.DataType,
	}

	return paramMatchesFilters(meta, filters)
}

// collectPathParams returns all parameters whose names begin with path, applying
// the recursive and filter constraints. It performs a linear scan over store
// (O(n)) and sorts the result by name. This replaces the previous binary-search
// approach that required maintaining a sorted slice on every PutParameter write
// (O(n) insert); the emulator write path is now O(1) and reads are O(n log n).
func collectPathParams(
	paramsTable *store.Table[Parameter],
	path string,
	recursive bool,
	filters []ParameterFilter,
) []Parameter {
	var matched []Parameter
	for _, p := range paramsTable.All() {
		name, param := p.Name, *p
		if !strings.HasPrefix(name, path) {
			continue
		}
		if !recursive {
			suffix := name[len(path):]
			if strings.Contains(suffix, "/") {
				continue
			}
		}
		if len(filters) > 0 && !paramByPathMatchesFilters(param, filters) {
			continue
		}
		matched = append(matched, param)
	}

	sort.Slice(matched, func(i, j int) bool { return matched[i].Name < matched[j].Name })

	return matched
}

// cleanupEmptyParamRegion removes the per-region inner maps for history and
// tags when the last parameter in a region is deleted. b.parameters is
// intentionally NOT pruned here: it is now a *store.Table[Parameter] per
// region, registered once by name ("parameters/"+region) on b.registry (see
// store_setup.go's getOrCreateTable); store.Registry has no unregister, so
// deleting the region's entry from b.parameters would make a later write to
// the same region re-register the same name and panic. An empty Table left
// in place is observably identical to an absent one (Len() == 0, no
// entries) — this only affects internal bookkeeping, never a caller-visible
// response.
// Caller must hold the write lock.
func (b *InMemoryBackend) cleanupEmptyParamRegion(region string) {
	cleanupEmptyInnerMap(b.history, region)
	cleanupEmptyInnerMap(b.tags, region)
}

// decryptParamsSlice returns a copy of params with SecureString values decrypted
// when requested, and the ARN populated on each parameter.
func (b *InMemoryBackend) decryptParamsSlice(
	params []Parameter, withDecryption bool, region, account string,
) []Parameter {
	// No capacity hint — user-derived values in the capacity slot trigger CodeQL.
	// nolint:prealloc,nolintlint // satisfies CodeQL by removing tainted capacity hint
	result := make([]Parameter, 0)
	for _, p := range params {
		if withDecryption && p.Type == SecureStringType {
			if decrypted, err := b.decryptSSMValue(p.KeyID, p.Value); err == nil {
				p.Value = decrypted
			}
		}
		p.ARN = parameterARN(region, account, p.Name)
		result = append(result, p)
	}

	return result
}

// GetParametersByPath returns parameters whose names begin with the given path.
func (b *InMemoryBackend) GetParametersByPath(
	ctx context.Context,
	input *GetParametersByPathInput,
) (*GetParametersByPathOutput, error) {
	region := getRegion(ctx)
	account := awsmeta.Account(ctx)

	b.mu.RLock("GetParametersByPath")
	defer b.mu.RUnlock()

	path := input.Path
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}

	matched := collectPathParams(
		b.parametersStore(region),
		path,
		input.Recursive,
		input.ParameterFilters,
	)

	startIdx := parseNextToken(input.NextToken)

	maxResults := int64(defaultPathMaxResults)
	if input.MaxResults != nil {
		if *input.MaxResults < 1 || *input.MaxResults > defaultPathMaxResults {
			return nil, fmt.Errorf(
				"%w: MaxResults must be between 1 and %d",
				ErrValidationException,
				defaultPathMaxResults,
			)
		}

		maxResults = *input.MaxResults
	}

	if startIdx >= len(matched) {
		return &GetParametersByPathOutput{Parameters: []ParameterOutput{}}, nil
	}

	end := startIdx + int(maxResults)

	var nextToken string

	if end < len(matched) {
		nextToken = strconv.Itoa(end)
	} else {
		end = len(matched)
	}

	return &GetParametersByPathOutput{
		Parameters: toParameterOutputs(b.decryptParamsSlice(
			matched[startIdx:end],
			input.WithDecryption,
			region,
			account,
		)),
		NextToken: nextToken,
	}, nil
}

// DescribeParameters returns metadata for all parameters (no values).
func (b *InMemoryBackend) DescribeParameters(
	ctx context.Context,
	input *DescribeParametersInput,
) (*DescribeParametersOutput, error) {
	region := getRegion(ctx)
	account := awsmeta.Account(ctx)

	b.mu.RLock("DescribeParameters")
	defer b.mu.RUnlock()

	paramsTable := b.parametersStore(region)
	all := make([]ParameterMetadata, 0, paramsTable.Len())

	for _, p := range paramsTable.All() {
		all = append(all, ParameterMetadata{
			Name:             p.Name,
			Type:             p.Type,
			Version:          p.Version,
			LastModifiedDate: p.LastModifiedDate,
			Description:      p.Description,
			KeyID:            p.KeyID,
			Tier:             p.Tier,
			AllowedPattern:   p.AllowedPattern,
			DataType:         p.DataType,
			Policies:         parameterPoliciesToWire(p.Policies),
			ARN:              parameterARN(region, account, p.Name),
		})
	}

	// Apply filters
	if len(input.ParameterFilters) > 0 {
		var filtered []ParameterMetadata

		for _, meta := range all {
			if paramMatchesFilters(meta, input.ParameterFilters) {
				filtered = append(filtered, meta)
			}
		}

		all = filtered
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].Name < all[j].Name
	})

	startIdx := parseNextToken(input.NextToken)

	maxResults := int64(defaultDescribeMaxResults)
	if input.MaxResults != nil {
		if *input.MaxResults < 1 || *input.MaxResults > defaultDescribeMaxResults {
			return nil, fmt.Errorf(
				"%w: MaxResults must be between 1 and %d",
				ErrValidationException,
				defaultDescribeMaxResults,
			)
		}

		maxResults = *input.MaxResults
	}

	if startIdx >= len(all) {
		return &DescribeParametersOutput{Parameters: []ParameterMetadata{}}, nil
	}

	end := startIdx + int(maxResults)

	var nextToken string

	if end < len(all) {
		nextToken = strconv.Itoa(end)
	} else {
		end = len(all)
	}

	return &DescribeParametersOutput{
		Parameters: all[startIdx:end],
		NextToken:  nextToken,
	}, nil
}

// paramMatchesFilters returns true when the metadata satisfies ALL filters.
func paramMatchesFilters(meta ParameterMetadata, filters []ParameterFilter) bool {
	for _, f := range filters {
		if !paramMatchesFilter(meta, f) {
			return false
		}
	}

	return true
}

// paramMatchesFilter returns true when the metadata satisfies a single filter.
// Within one filter, multiple Values are OR-combined. Unrecognised keys match
// everything (gopherstack has no schema-validation layer for filter keys).
func paramMatchesFilter(meta ParameterMetadata, f ParameterFilter) bool {
	if f.Key == "Path" {
		return paramMatchesPathFilter(meta.Name, f)
	}

	var fieldValue string

	switch f.Key {
	case filterKeyName:
		fieldValue = meta.Name
	case "Type":
		fieldValue = meta.Type
	case "KeyId":
		fieldValue = meta.KeyID
	case "Tier":
		fieldValue = meta.Tier
	case "DataType":
		fieldValue = meta.DataType
	default:
		return true // unknown keys are silently ignored (backwards compat)
	}

	return fieldMatchesFilterOption(fieldValue, f.Option, f.Values)
}

// fieldMatchesFilterOption compares fieldValue against each value under the
// given option (defaulting to Equals), OR-combining the values.
func fieldMatchesFilterOption(fieldValue, option string, values []string) bool {
	if option == "" {
		option = "Equals"
	}

	for _, v := range values {
		switch option {
		case "Equals":
			if fieldValue == v {
				return true
			}
		case "BeginsWith":
			if strings.HasPrefix(fieldValue, v) {
				return true
			}
		case "Contains":
			if strings.Contains(fieldValue, v) {
				return true
			}
		}
	}

	return false
}

// paramMatchesPathFilter applies a Key=Path ParameterFilter (DescribeParameters
// only, per types.ParameterStringFilter's own doc comment) to a parameter name.
// Option Recursive matches any descendant of the value; OneLevel matches only
// a direct child.
func paramMatchesPathFilter(name string, f ParameterFilter) bool {
	for _, v := range f.Values {
		prefix := v
		if !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}

		if !strings.HasPrefix(name, prefix) {
			continue
		}

		if f.Option == "OneLevel" && strings.Contains(name[len(prefix):], "/") {
			continue
		}

		return true
	}

	return false
}

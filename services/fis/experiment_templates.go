package fis

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// ----------------------------------------
// Template validation
// ----------------------------------------

// selectionModeRe matches valid FIS selectionMode values: ALL, COUNT(N), PERCENT(N).
var selectionModeRe = regexp.MustCompile(`^(ALL|COUNT\(\d+\)|PERCENT\(\d{1,3}(\.\d+)?\))$`)

// maxDescriptionLen is the maximum allowed length for template/experiment descriptions.
const maxDescriptionLen = 512

// maxClientTokenLen is the maximum allowed length for idempotency client tokens.
const maxClientTokenLen = 64

// validateTemplate checks that a create request meets AWS FIS requirements.
func validateTemplate(input *createExperimentTemplateRequest) error {
	if strings.TrimSpace(input.RoleArn) == "" {
		return fmt.Errorf("%w: roleArn is required", ErrValidation)
	}

	if !isValidRoleArn(input.RoleArn) {
		return fmt.Errorf("%w: roleArn must be a valid IAM role ARN (arn:aws:iam::{account}:role/...)", ErrValidation)
	}

	if len(input.Description) > maxDescriptionLen {
		return fmt.Errorf(
			"%w: description must be at most %d characters; got %d",
			ErrValidation, maxDescriptionLen, len(input.Description),
		)
	}

	if len(input.ClientToken) > maxClientTokenLen {
		return fmt.Errorf(
			"%w: clientToken must be at most %d characters",
			ErrValidation, maxClientTokenLen,
		)
	}

	if len(input.StopConditions) == 0 {
		return fmt.Errorf("%w: stopConditions is required", ErrValidation)
	}

	if err := validateStopConditions(input.StopConditions); err != nil {
		return err
	}

	if err := validateTargets(input.Targets); err != nil {
		return err
	}

	if err := validateActions(input.Actions, input.Targets); err != nil {
		return err
	}

	if err := validateReportConfiguration(input.ExperimentReportConfiguration); err != nil {
		return err
	}

	if len(input.Tags) > maxTagsPerResource {
		return fmt.Errorf(
			"%w: tags must have at most %d entries; got %d",
			ErrValidation, maxTagsPerResource, len(input.Tags),
		)
	}

	return nil
}

// validateUpdateTemplate checks that an update request meets AWS FIS requirements.
func validateUpdateTemplate(input *updateExperimentTemplateRequest) error {
	if input.RoleArn != "" && !isValidRoleArn(input.RoleArn) {
		return fmt.Errorf("%w: roleArn must be a valid IAM role ARN (arn:aws:iam::{account}:role/...)", ErrValidation)
	}

	if len(input.Description) > maxDescriptionLen {
		return fmt.Errorf(
			"%w: description must be at most %d characters; got %d",
			ErrValidation, maxDescriptionLen, len(input.Description),
		)
	}

	if input.StopConditions != nil {
		if len(input.StopConditions) == 0 {
			return fmt.Errorf("%w: stopConditions must not be empty when provided", ErrValidation)
		}

		if err := validateStopConditions(input.StopConditions); err != nil {
			return err
		}
	}

	if input.Targets != nil {
		if err := validateTargets(input.Targets); err != nil {
			return err
		}
	}

	if input.Actions != nil {
		if err := validateActions(input.Actions, input.Targets); err != nil {
			return err
		}
	}

	if err := validateReportConfiguration(input.ExperimentReportConfiguration); err != nil {
		return err
	}

	return nil
}

// validateReportConfiguration checks that an experiment report configuration's
// duration fields, when present, are syntactically valid ISO 8601 durations.
func validateReportConfiguration(cfg *experimentTemplateReportConfigDTO) error {
	if cfg == nil {
		return nil
	}

	if cfg.PreExperimentDuration != "" && !isValidISODuration(cfg.PreExperimentDuration) {
		return fmt.Errorf(
			"%w: experimentReportConfiguration.preExperimentDuration %q is not a valid ISO 8601 duration",
			ErrValidation, cfg.PreExperimentDuration,
		)
	}

	if cfg.PostExperimentDuration != "" && !isValidISODuration(cfg.PostExperimentDuration) {
		return fmt.Errorf(
			"%w: experimentReportConfiguration.postExperimentDuration %q is not a valid ISO 8601 duration",
			ErrValidation, cfg.PostExperimentDuration,
		)
	}

	return nil
}

// validateStopConditions validates the stop conditions slice. Source must be
// exactly "none" or "aws:cloudwatch:alarm" (CreateExperimentTemplateStopConditionInput
// doc comment); value is required (the alarm ARN) when source is an alarm and
// must be absent when source is "none".
func validateStopConditions(conditions []experimentTemplateStopConditionDTO) error {
	for i, sc := range conditions {
		src := strings.TrimSpace(sc.Source)
		if src == "" {
			return fmt.Errorf("%w: stopConditions[%d].source is required", ErrValidation, i)
		}

		switch src {
		case "none":
			if strings.TrimSpace(sc.Value) != "" {
				return fmt.Errorf(
					"%w: stopConditions[%d]: value must be empty when source is \"none\"",
					ErrValidation, i,
				)
			}
		case stopConditionSourceAlarm:
			if strings.TrimSpace(sc.Value) == "" {
				return fmt.Errorf(
					"%w: stopConditions[%d].value is required when source is \"aws:cloudwatch:alarm\"",
					ErrValidation, i,
				)
			}
		default:
			return fmt.Errorf(
				"%w: stopConditions[%d].source must be \"none\" or \"aws:cloudwatch:alarm\"; got %q",
				ErrValidation, i, src,
			)
		}
	}

	return nil
}

// validateTargets validates the target map.
func validateTargets(targets map[string]experimentTemplateTargetDTO) error {
	for name, tgt := range targets {
		if strings.TrimSpace(tgt.ResourceType) == "" {
			return fmt.Errorf("%w: target %q: resourceType is required", ErrValidation, name)
		}

		if strings.TrimSpace(tgt.SelectionMode) == "" {
			return fmt.Errorf("%w: target %q: selectionMode is required", ErrValidation, name)
		}

		if !selectionModeRe.MatchString(tgt.SelectionMode) {
			return fmt.Errorf(
				"%w: target %q: selectionMode must be ALL, COUNT(N), or PERCENT(N); got %q",
				ErrValidation, name, tgt.SelectionMode,
			)
		}

		// Validate filters.
		for j, f := range tgt.Filters {
			if strings.TrimSpace(f.Path) == "" {
				return fmt.Errorf("%w: target %q: filter[%d].path is required", ErrValidation, name, j)
			}

			if len(f.Values) == 0 {
				return fmt.Errorf("%w: target %q: filter[%d].values must not be empty", ErrValidation, name, j)
			}
		}
	}

	return nil
}

// validateActions validates the action map.
func validateActions(
	actions map[string]experimentTemplateActionDTO,
	targets map[string]experimentTemplateTargetDTO,
) error {
	for name, action := range actions {
		if err := validateAction(name, action, actions, targets); err != nil {
			return err
		}
	}

	// Detect cycles in startAfter dependencies.
	if err := detectActionCycles(actions); err != nil {
		return err
	}

	return nil
}

// validateAction validates a single action's identifier, references, and parameters.
func validateAction(
	name string,
	action experimentTemplateActionDTO,
	actions map[string]experimentTemplateActionDTO,
	targets map[string]experimentTemplateTargetDTO,
) error {
	if strings.TrimSpace(action.ActionID) == "" {
		return fmt.Errorf("%w: action %q: actionId is required", ErrValidation, name)
	}

	if err := validateActionReferences(name, action, actions, targets); err != nil {
		return err
	}

	return validateActionDuration(name, action)
}

// validateActionReferences validates an action's startAfter and target references.
func validateActionReferences(
	name string,
	action experimentTemplateActionDTO,
	actions map[string]experimentTemplateActionDTO,
	targets map[string]experimentTemplateTargetDTO,
) error {
	for _, depName := range action.StartAfter {
		if _, ok := actions[depName]; !ok {
			return fmt.Errorf(
				"%w: action %q: startAfter references undefined action %q",
				ErrValidation, name, depName,
			)
		}
	}

	for _, tgtName := range action.Targets {
		if _, ok := targets[tgtName]; !ok {
			return fmt.Errorf(
				"%w: action %q references undefined target %q",
				ErrValidation, name, tgtName,
			)
		}
	}

	return nil
}

// validateActionDuration validates the duration parameter requirements for an action.
func validateActionDuration(name string, action experimentTemplateActionDTO) error {
	// aws:fis:wait requires duration.
	if action.ActionID == actionIDWait && strings.TrimSpace(action.Parameters["duration"]) == "" {
		return fmt.Errorf(
			"%w: action %q: %s requires the duration parameter",
			ErrValidation, name, actionIDWait,
		)
	}

	// Validate duration parameter format when present.
	if dur, ok := action.Parameters["duration"]; ok && dur != "" {
		if !isValidISODuration(dur) {
			return fmt.Errorf(
				"%w: action %q: duration parameter %q is not a valid ISO 8601 duration",
				ErrValidation, name, dur,
			)
		}
	}

	return nil
}

// detectActionCycles returns an error if the startAfter dependency graph has a cycle.
func detectActionCycles(actions map[string]experimentTemplateActionDTO) error {
	const (
		unvisited = 0
		inStack   = 1
		done      = 2
	)

	state := make(map[string]int, len(actions))

	var visit func(name string) error
	visit = func(name string) error {
		switch state[name] {
		case inStack:
			return fmt.Errorf("%w: action dependency cycle detected involving action %q", ErrValidation, name)
		case done:
			return nil
		}

		state[name] = inStack

		for _, dep := range actions[name].StartAfter {
			if err := visit(dep); err != nil {
				return err
			}
		}

		state[name] = done

		return nil
	}

	for name := range actions {
		if err := visit(name); err != nil {
			return err
		}
	}

	return nil
}

// iamAccountIDLen is the fixed length of an AWS account ID (12 decimal digits).
const iamAccountIDLen = 12

// isValidRoleArn checks that the string looks like an IAM role ARN with a valid 12-digit account ID.
func isValidRoleArn(s string) bool {
	const prefix = "arn:aws:iam::"
	if !strings.HasPrefix(s, prefix) {
		return false
	}

	rest := s[len(prefix):]
	colon := strings.Index(rest, ":")

	if colon != iamAccountIDLen {
		return false
	}

	for _, c := range rest[:12] {
		if c < '0' || c > '9' {
			return false
		}
	}

	return strings.HasPrefix(rest[colon:], ":role/")
}

// ----------------------------------------
// ExperimentTemplate CRUD
// ----------------------------------------

// CreateExperimentTemplate creates a new experiment template.
func (b *InMemoryBackend) CreateExperimentTemplate(
	input *createExperimentTemplateRequest,
	accountID, region string,
) (*ExperimentTemplate, error) {
	if err := validateTemplate(input); err != nil {
		return nil, err
	}

	// Check clientToken idempotency before generating a new ID.
	if input.ClientToken != "" {
		b.mu.RLock("CreateExperimentTemplate-idempotency")
		existingID, ok := b.tplClientTokens[input.ClientToken]
		b.mu.RUnlock()

		if ok {
			return b.GetExperimentTemplate(existingID)
		}
	}

	id := generateID("EXT")
	arnStr := arn.Build("fis", region, accountID, "experiment-template/"+id)

	now := time.Now()
	tpl := &ExperimentTemplate{
		ID:             id,
		Arn:            arnStr,
		Description:    input.Description,
		RoleArn:        input.RoleArn,
		Tags:           copyStringMap(input.Tags),
		Targets:        convertTargetDTOs(input.Targets),
		Actions:        convertActionDTOs(input.Actions),
		StopConditions: convertStopConditionDTOs(input.StopConditions),
		CreationTime:   now,
		LastUpdateTime: now,
	}

	if input.LogConfiguration != nil {
		tpl.LogConfiguration = convertTemplateLogConfigDTO(input.LogConfiguration)
	}

	if input.ExperimentOptions != nil {
		tpl.ExperimentOptions = &ExperimentTemplateExperimentOptions{
			AccountTargeting:          input.ExperimentOptions.AccountTargeting,
			EmptyTargetResolutionMode: input.ExperimentOptions.EmptyTargetResolutionMode,
		}
	}

	if input.ExperimentReportConfiguration != nil {
		tpl.ExperimentReportConfiguration = convertReportConfigDTO(input.ExperimentReportConfiguration)
	}

	b.mu.Lock("CreateExperimentTemplate")
	defer b.mu.Unlock()

	b.templates.Put(tpl)

	if input.ClientToken != "" {
		b.tplClientTokens[input.ClientToken] = id
	}

	return tpl, nil
}

// GetExperimentTemplate retrieves an experiment template by ID.
func (b *InMemoryBackend) GetExperimentTemplate(id string) (*ExperimentTemplate, error) {
	b.mu.RLock("GetExperimentTemplate")
	defer b.mu.RUnlock()

	tpl, ok := b.templates.Get(id)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrTemplateNotFound, id)
	}

	cp := cloneTemplate(tpl)

	return cp, nil
}

// UpdateExperimentTemplate updates an existing experiment template.
func (b *InMemoryBackend) UpdateExperimentTemplate(
	id string,
	input *updateExperimentTemplateRequest,
) (*ExperimentTemplate, error) {
	if err := validateUpdateTemplate(input); err != nil {
		return nil, err
	}

	b.mu.Lock("UpdateExperimentTemplate")
	defer b.mu.Unlock()

	tpl, ok := b.templates.Get(id)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrTemplateNotFound, id)
	}

	if input.Description != "" {
		tpl.Description = input.Description
	}

	if input.RoleArn != "" {
		tpl.RoleArn = input.RoleArn
	}

	if input.Targets != nil {
		tpl.Targets = convertTargetDTOs(input.Targets)
	}

	if input.Actions != nil {
		tpl.Actions = convertActionDTOs(input.Actions)
	}

	if input.StopConditions != nil {
		tpl.StopConditions = convertStopConditionDTOs(input.StopConditions)
	}

	if input.LogConfiguration != nil {
		tpl.LogConfiguration = convertTemplateLogConfigDTO(input.LogConfiguration)
	}

	if input.ExperimentOptions != nil {
		tpl.ExperimentOptions = &ExperimentTemplateExperimentOptions{
			AccountTargeting:          input.ExperimentOptions.AccountTargeting,
			EmptyTargetResolutionMode: input.ExperimentOptions.EmptyTargetResolutionMode,
		}
	}

	if input.ExperimentReportConfiguration != nil {
		tpl.ExperimentReportConfiguration = convertReportConfigDTO(input.ExperimentReportConfiguration)
	}

	tpl.LastUpdateTime = time.Now()

	return cloneTemplate(tpl), nil
}

// DeleteExperimentTemplate deletes an experiment template by ID, including its target account configurations.
func (b *InMemoryBackend) DeleteExperimentTemplate(id string) error {
	b.mu.Lock("DeleteExperimentTemplate")
	defer b.mu.Unlock()

	if !b.templates.Has(id) {
		return fmt.Errorf("%w: %s", ErrTemplateNotFound, id)
	}

	b.templates.Delete(id)

	// Clone the index group before deleting in the loop: Delete mutates the
	// index's backing slice for this group, which would otherwise invalidate
	// iteration over the very slice being ranged.
	cfgs := slices.Clone(b.targetAccountConfigsByTemplate.Get(id))
	for _, cfg := range cfgs {
		b.targetAccountConfigs.Delete(targetAccountConfigKey(cfg.ExperimentTemplateID, cfg.AccountID))
	}

	// Drop idempotency-token entries pointing at this template so the map
	// cannot grow unbounded across long-lived backends.
	for tok, tid := range b.tplClientTokens {
		if tid == id {
			delete(b.tplClientTokens, tok)
		}
	}

	return nil
}

// ListExperimentTemplates returns all experiment templates sorted by ID.
func (b *InMemoryBackend) ListExperimentTemplates() ([]*ExperimentTemplate, error) {
	b.mu.RLock("ListExperimentTemplates")
	defer b.mu.RUnlock()

	all := b.templates.All()
	result := make([]*ExperimentTemplate, 0, len(all))

	for _, tpl := range all {
		result = append(result, cloneTemplate(tpl))
	}

	slices.SortFunc(result, func(a, b *ExperimentTemplate) int { return strings.Compare(a.ID, b.ID) })

	return result, nil
}

// ----------------------------------------
// Deep copy / conversion helpers
// ----------------------------------------

// cloneTemplate returns a deep copy of an ExperimentTemplate.
func cloneTemplate(tpl *ExperimentTemplate) *ExperimentTemplate {
	cp := *tpl

	cp.Tags = copyStringMap(tpl.Tags)

	if tpl.Targets != nil {
		cp.Targets = make(map[string]ExperimentTemplateTarget, len(tpl.Targets))

		for k, v := range tpl.Targets {
			t := v
			t.ResourceArns = append([]string(nil), v.ResourceArns...)
			t.ResourceTags = copyStringMap(v.ResourceTags)
			t.Parameters = copyStringMap(v.Parameters)

			filters := make([]ExperimentTemplateTargetFilter, len(v.Filters))
			for i, f := range v.Filters {
				filters[i] = ExperimentTemplateTargetFilter{
					Path:   f.Path,
					Values: append([]string(nil), f.Values...),
				}
			}

			t.Filters = filters
			cp.Targets[k] = t
		}
	}

	if tpl.Actions != nil {
		cp.Actions = make(map[string]ExperimentTemplateAction, len(tpl.Actions))

		for k, v := range tpl.Actions {
			a := v
			a.Parameters = copyStringMap(v.Parameters)
			a.Targets = copyStringMap(v.Targets)
			a.StartAfter = append([]string(nil), v.StartAfter...)
			cp.Actions[k] = a
		}
	}

	if tpl.StopConditions != nil {
		cp.StopConditions = append([]ExperimentTemplateStopCondition(nil), tpl.StopConditions...)
	}

	if tpl.LogConfiguration != nil {
		lc := *tpl.LogConfiguration
		if tpl.LogConfiguration.CloudWatchLogsConfiguration != nil {
			cwl := *tpl.LogConfiguration.CloudWatchLogsConfiguration
			lc.CloudWatchLogsConfiguration = &cwl
		}

		if tpl.LogConfiguration.S3Configuration != nil {
			s3 := *tpl.LogConfiguration.S3Configuration
			lc.S3Configuration = &s3
		}

		cp.LogConfiguration = &lc
	}

	if tpl.ExperimentOptions != nil {
		opt := *tpl.ExperimentOptions
		cp.ExperimentOptions = &opt
	}

	cp.ExperimentReportConfiguration = cloneExperimentTemplateReportConfig(tpl.ExperimentReportConfiguration)

	return &cp
}

// cloneExperimentTemplateReportConfig deep-copies a template's report configuration.
func cloneExperimentTemplateReportConfig(
	cfg *ExperimentTemplateReportConfiguration,
) *ExperimentTemplateReportConfiguration {
	if cfg == nil {
		return nil
	}

	cp := *cfg

	if cfg.DataSources != nil {
		ds := *cfg.DataSources
		ds.CloudWatchDashboards = append(
			[]ExperimentTemplateReportConfigurationCloudWatchDashboard(nil),
			cfg.DataSources.CloudWatchDashboards...,
		)
		cp.DataSources = &ds
	}

	if cfg.Outputs != nil {
		out := *cfg.Outputs
		if cfg.Outputs.S3Configuration != nil {
			s3 := *cfg.Outputs.S3Configuration
			out.S3Configuration = &s3
		}
		cp.Outputs = &out
	}

	return &cp
}

// convertReportConfigDTO converts a request DTO's report configuration into the
// domain type stored on an ExperimentTemplate.
func convertReportConfigDTO(dto *experimentTemplateReportConfigDTO) *ExperimentTemplateReportConfiguration {
	if dto == nil {
		return nil
	}

	cfg := &ExperimentTemplateReportConfiguration{
		PreExperimentDuration:  dto.PreExperimentDuration,
		PostExperimentDuration: dto.PostExperimentDuration,
	}

	if dto.DataSources != nil {
		dashboards := make(
			[]ExperimentTemplateReportConfigurationCloudWatchDashboard,
			len(dto.DataSources.CloudWatchDashboards),
		)
		for i, d := range dto.DataSources.CloudWatchDashboards {
			dashboards[i] = ExperimentTemplateReportConfigurationCloudWatchDashboard(d)
		}

		cfg.DataSources = &ExperimentTemplateReportConfigurationDataSources{CloudWatchDashboards: dashboards}
	}

	if dto.Outputs != nil && dto.Outputs.S3Configuration != nil {
		cfg.Outputs = &ExperimentTemplateReportConfigurationOutputs{
			S3Configuration: &ExperimentTemplateReportConfigurationOutputsS3Configuration{
				BucketName: dto.Outputs.S3Configuration.BucketName,
				Prefix:     dto.Outputs.S3Configuration.Prefix,
			},
		}
	}

	return cfg
}

func convertTargetDTOs(in map[string]experimentTemplateTargetDTO) map[string]ExperimentTemplateTarget {
	if in == nil {
		return map[string]ExperimentTemplateTarget{}
	}

	out := make(map[string]ExperimentTemplateTarget, len(in))

	for name, dto := range in {
		filters := make([]ExperimentTemplateTargetFilter, len(dto.Filters))
		for i, f := range dto.Filters {
			filters[i] = ExperimentTemplateTargetFilter(f)
		}

		out[name] = ExperimentTemplateTarget{
			ResourceType:  dto.ResourceType,
			SelectionMode: dto.SelectionMode,
			ResourceArns:  append([]string(nil), dto.ResourceArns...),
			ResourceTags:  copyStringMap(dto.ResourceTags),
			Filters:       filters,
			Parameters:    copyStringMap(dto.Parameters),
		}
	}

	return out
}

func convertActionDTOs(in map[string]experimentTemplateActionDTO) map[string]ExperimentTemplateAction {
	if in == nil {
		return map[string]ExperimentTemplateAction{}
	}

	out := make(map[string]ExperimentTemplateAction, len(in))

	for name, dto := range in {
		out[name] = ExperimentTemplateAction{
			ActionID:    dto.ActionID,
			Description: dto.Description,
			Parameters:  copyStringMap(dto.Parameters),
			StartAfter:  append([]string(nil), dto.StartAfter...),
			Targets:     copyStringMap(dto.Targets),
		}
	}

	return out
}

func convertStopConditionDTOs(in []experimentTemplateStopConditionDTO) []ExperimentTemplateStopCondition {
	if in == nil {
		return []ExperimentTemplateStopCondition{}
	}

	out := make([]ExperimentTemplateStopCondition, len(in))
	for i, dto := range in {
		out[i] = ExperimentTemplateStopCondition(dto)
	}

	return out
}

func convertTemplateLogConfigDTO(dto *experimentTemplateLogConfigurationDTO) *ExperimentTemplateLogConfiguration {
	if dto == nil {
		return nil
	}

	lc := &ExperimentTemplateLogConfiguration{
		LogSchemaVersion: dto.LogSchemaVersion,
	}

	if dto.CloudWatchLogsConfiguration != nil {
		lc.CloudWatchLogsConfiguration = &ExperimentTemplateCloudWatchLogsConfiguration{
			LogGroupArn: dto.CloudWatchLogsConfiguration.LogGroupArn,
		}
	}

	if dto.S3Configuration != nil {
		lc.S3Configuration = &ExperimentTemplateS3Configuration{
			BucketName: dto.S3Configuration.BucketName,
			Prefix:     dto.S3Configuration.Prefix,
		}
	}

	return lc
}

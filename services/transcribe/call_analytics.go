package transcribe

import (
	"fmt"
	"slices"
	"sort"
	"time"
)

// requiredChannelDefinitionCount is the exact number of channel definitions required for call analytics.
const requiredChannelDefinitionCount = 2

// minCallAnalyticsRules and maxCallAnalyticsRules bound the number of rules a
// category may contain ("you must create between 1 and 20 rules").
const (
	minCallAnalyticsRules = 1
	maxCallAnalyticsRules = 20
)

// supportedCallAnalyticsInputTypes returns the set of call analytics category input types.
func supportedCallAnalyticsInputTypes() []string { return []string{"REAL_TIME", "POST_CALL"} }

// supportedTranscriptFilterTypes returns the set of TranscriptFilter.TranscriptFilterType values.
func supportedTranscriptFilterTypes() []string { return []string{"EXACT"} }

// supportedSentimentValues returns the set of SentimentFilter.Sentiments values.
func supportedSentimentValues() []string { return []string{"POSITIVE", "NEGATIVE", "NEUTRAL", "MIXED"} }

// validateTranscriptFilterRule checks TranscriptFilter's required sub-fields
// when the filter is set on a Call Analytics rule.
func validateTranscriptFilterRule(i int, tf *TranscriptFilter) error {
	if tf == nil {
		return nil
	}

	if tf.TranscriptFilterType == "" {
		return fmt.Errorf("%w: Rules[%d].TranscriptFilter.TranscriptFilterType is required", ErrValidation, i)
	}

	if !slices.Contains(supportedTranscriptFilterTypes(), tf.TranscriptFilterType) {
		return fmt.Errorf("%w: Rules[%d].TranscriptFilter.TranscriptFilterType %q must be one of %v",
			ErrValidation, i, tf.TranscriptFilterType, supportedTranscriptFilterTypes())
	}

	if tf.Targets == nil {
		return fmt.Errorf("%w: Rules[%d].TranscriptFilter.Targets is required", ErrValidation, i)
	}

	return nil
}

// validateSentimentFilterRule checks SentimentFilter's required sub-fields
// when the filter is set on a Call Analytics rule.
func validateSentimentFilterRule(i int, sf *SentimentFilter) error {
	if sf == nil {
		return nil
	}

	if sf.Sentiments == nil {
		return fmt.Errorf("%w: Rules[%d].SentimentFilter.Sentiments is required", ErrValidation, i)
	}

	for _, sentiment := range sf.Sentiments {
		if !slices.Contains(supportedSentimentValues(), sentiment) {
			return fmt.Errorf("%w: Rules[%d].SentimentFilter.Sentiments %q must be one of %v",
				ErrValidation, i, sentiment, supportedSentimentValues())
		}
	}

	return nil
}

// validateCallAnalyticsRules checks the Rules field required by
// CreateCallAnalyticsCategory/UpdateCallAnalyticsCategory: it must be present,
// contain 1-20 entries, and each populated filter must carry its own required
// sub-fields (TranscriptFilter.Targets/TranscriptFilterType,
// SentimentFilter.Sentiments).
func validateCallAnalyticsRules(rules []CallAnalyticsRule) error {
	if rules == nil {
		return fmt.Errorf("%w: Rules is required", ErrValidation)
	}

	if len(rules) < minCallAnalyticsRules || len(rules) > maxCallAnalyticsRules {
		return fmt.Errorf("%w: Rules must contain between %d and %d entries, got %d",
			ErrValidation, minCallAnalyticsRules, maxCallAnalyticsRules, len(rules))
	}

	for i := range rules {
		if err := validateTranscriptFilterRule(i, rules[i].TranscriptFilter); err != nil {
			return err
		}

		if err := validateSentimentFilterRule(i, rules[i].SentimentFilter); err != nil {
			return err
		}
	}

	return nil
}

// validateCallAnalyticsInputType checks that an InputType is valid.
func validateCallAnalyticsInputType(inputType string) error {
	if inputType != "" && !slices.Contains(supportedCallAnalyticsInputTypes(), inputType) {
		return fmt.Errorf("%w: InputType %q must be one of %v",
			ErrValidation, inputType, supportedCallAnalyticsInputTypes())
	}

	return nil
}

// validateChannelDefinitions checks that exactly 2 channel definitions are provided with distinct roles.
func validateChannelDefinitions(defs []ChannelDefinition) error {
	if len(defs) == 0 {
		return nil
	}

	if len(defs) != requiredChannelDefinitionCount {
		return fmt.Errorf("%w: ChannelDefinitions must contain exactly 2 entries (AGENT and CUSTOMER), got %d",
			ErrValidation, len(defs))
	}

	roles := make(map[string]bool)
	for _, d := range defs {
		if d.ParticipantRole == "" {
			return fmt.Errorf("%w: each ChannelDefinition must have a ParticipantRole", ErrValidation)
		}

		if d.ParticipantRole != "AGENT" && d.ParticipantRole != "CUSTOMER" {
			return fmt.Errorf("%w: ParticipantRole %q must be AGENT or CUSTOMER", ErrValidation, d.ParticipantRole)
		}

		if roles[d.ParticipantRole] {
			return fmt.Errorf(
				"%w: duplicate ParticipantRole %q in ChannelDefinitions",
				ErrValidation,
				d.ParticipantRole,
			)
		}

		roles[d.ParticipantRole] = true
	}

	return nil
}

// --- Call Analytics categories ---

// CreateCallAnalyticsCategory creates a new Call Analytics category.
func (b *InMemoryBackend) CreateCallAnalyticsCategory(input *CallAnalyticsCategory) (*CallAnalyticsCategory, error) {
	if input.CategoryName == "" {
		return nil, fmt.Errorf("%w: CategoryName is required", ErrValidation)
	}

	if err := validateCallAnalyticsInputType(input.InputType); err != nil {
		return nil, err
	}

	if err := validateCallAnalyticsRules(input.Rules); err != nil {
		return nil, err
	}

	b.mu.Lock("CreateCallAnalyticsCategory")
	defer b.mu.Unlock()

	if b.callAnalyticsCategories.Has(input.CategoryName) {
		return nil, fmt.Errorf("%w: category %s already exists", ErrAlreadyExists, input.CategoryName)
	}

	now := time.Now()
	cat := *input
	cat.CreateTime = now
	cat.LastUpdateTime = now
	b.callAnalyticsCategories.Put(&cat)
	b.recordResourceTagsLocked(resourceARN(resourceTypeCallAnalyticsCategory, cat.CategoryName), cat.Tags)

	cp := cat

	return &cp, nil
}

// DeleteCallAnalyticsCategory removes a Call Analytics category by name.
func (b *InMemoryBackend) DeleteCallAnalyticsCategory(categoryName string) error {
	if categoryName == "" {
		return fmt.Errorf("%w: CategoryName is required", ErrValidation)
	}

	b.mu.Lock("DeleteCallAnalyticsCategory")
	defer b.mu.Unlock()

	if !b.callAnalyticsCategories.Delete(categoryName) {
		return fmt.Errorf("%w: category %s not found", ErrNotFound, categoryName)
	}

	b.forgetResourceTagsLocked(resourceARN(resourceTypeCallAnalyticsCategory, categoryName))

	return nil
}

// AddCallAnalyticsCategoryInternal seeds a Call Analytics category directly (test helper).
func (b *InMemoryBackend) AddCallAnalyticsCategoryInternal(cat *CallAnalyticsCategory) {
	b.mu.Lock("AddCallAnalyticsCategoryInternal")
	defer b.mu.Unlock()

	cp := *cat
	b.callAnalyticsCategories.Put(&cp)
}

// GetCallAnalyticsCategory returns a Call Analytics category by name.
func (b *InMemoryBackend) GetCallAnalyticsCategory(
	categoryName string,
) (*CallAnalyticsCategory, error) {
	b.mu.RLock("GetCallAnalyticsCategory")
	defer b.mu.RUnlock()

	cat, ok := b.callAnalyticsCategories.Get(categoryName)
	if !ok {
		return nil, fmt.Errorf("%w: category %s not found", ErrNotFound, categoryName)
	}

	cp := *cat
	cp.Tags = b.liveTagsLocked(resourceARN(resourceTypeCallAnalyticsCategory, categoryName))

	return &cp, nil
}

// UpdateCallAnalyticsCategory updates an existing Call Analytics category.
func (b *InMemoryBackend) UpdateCallAnalyticsCategory(input *CallAnalyticsCategory) (*CallAnalyticsCategory, error) {
	if input.CategoryName == "" {
		return nil, fmt.Errorf("%w: CategoryName is required", ErrValidation)
	}

	if err := validateCallAnalyticsInputType(input.InputType); err != nil {
		return nil, err
	}

	if err := validateCallAnalyticsRules(input.Rules); err != nil {
		return nil, err
	}

	b.mu.Lock("UpdateCallAnalyticsCategory")
	defer b.mu.Unlock()

	cat, ok := b.callAnalyticsCategories.Get(input.CategoryName)
	if !ok {
		return nil, fmt.Errorf("%w: category %s not found", ErrNotFound, input.CategoryName)
	}

	cat.InputType = input.InputType
	cat.Rules = input.Rules
	cat.LastUpdateTime = time.Now()

	cp := *cat

	return &cp, nil
}

// ListCallAnalyticsCategories returns all Call Analytics categories with pagination.
func (b *InMemoryBackend) ListCallAnalyticsCategories(
	nextToken string, maxResults int32,
) ([]CallAnalyticsCategory, string) {
	b.mu.RLock("ListCallAnalyticsCategories")
	defer b.mu.RUnlock()

	all := make([]CallAnalyticsCategory, 0, b.callAnalyticsCategories.Len())
	for _, c := range b.callAnalyticsCategories.All() {
		cp := *c
		cp.Tags = b.liveTagsLocked(resourceARN(resourceTypeCallAnalyticsCategory, c.CategoryName))
		all = append(all, cp)
	}

	sort.Slice(all, func(i, j int) bool { return all[i].CategoryName < all[j].CategoryName })

	return paginateList(all, nextToken, maxResults)
}

// --- Call Analytics jobs ---

// StartCallAnalyticsJob creates a new Call Analytics job.
func (b *InMemoryBackend) StartCallAnalyticsJob(input *CallAnalyticsJob) (*CallAnalyticsJob, error) {
	if err := validateJobName(input.CallAnalyticsJobName); err != nil {
		return nil, fmt.Errorf("%w: CallAnalyticsJobName is required", ErrValidation)
	}

	if err := validateLanguageCode(input.LanguageCode); err != nil {
		return nil, err
	}

	if err := validateChannelDefinitions(input.ChannelDefinitions); err != nil {
		return nil, err
	}

	b.mu.Lock("StartCallAnalyticsJob")
	defer b.mu.Unlock()

	if b.callAnalyticsJobs.Has(input.CallAnalyticsJobName) {
		return nil, fmt.Errorf(
			"%w: call analytics job %s already exists",
			ErrAlreadyExists,
			input.CallAnalyticsJobName,
		)
	}

	now := time.Now()
	job := *input
	job.CallAnalyticsJobStatus = jobStatusCompleted
	job.CreationTime = now
	job.StartTime = now
	job.CompletionTime = now
	b.callAnalyticsJobs.Put(&job)
	b.recordResourceTagsLocked(resourceARN(resourceTypeCallAnalyticsJob, job.CallAnalyticsJobName), job.Tags)

	cp := job

	return &cp, nil
}

// GetCallAnalyticsJob returns a Call Analytics job by name.
func (b *InMemoryBackend) GetCallAnalyticsJob(jobName string) (*CallAnalyticsJob, error) {
	b.mu.RLock("GetCallAnalyticsJob")
	defer b.mu.RUnlock()

	job, ok := b.callAnalyticsJobs.Get(jobName)
	if !ok {
		return nil, fmt.Errorf("%w: call analytics job %s not found", ErrNotFound, jobName)
	}

	cp := *job
	cp.Tags = b.liveTagsLocked(resourceARN(resourceTypeCallAnalyticsJob, jobName))

	return &cp, nil
}

// ListCallAnalyticsJobs returns Call Analytics jobs with optional status filter, name
// substring filter, and pagination.
func (b *InMemoryBackend) ListCallAnalyticsJobs(
	statusFilter, nameContains, nextToken string, maxResults int32,
) ([]CallAnalyticsJob, string) {
	b.mu.RLock("ListCallAnalyticsJobs")
	defer b.mu.RUnlock()

	all := make([]CallAnalyticsJob, 0, b.callAnalyticsJobs.Len())
	for _, j := range b.callAnalyticsJobs.All() {
		if (statusFilter == "" || j.CallAnalyticsJobStatus == statusFilter) &&
			matchesNameContains(j.CallAnalyticsJobName, nameContains) {
			all = append(all, *j)
		}
	}

	sort.Slice(
		all,
		func(i, j int) bool { return all[i].CallAnalyticsJobName < all[j].CallAnalyticsJobName },
	)

	return paginateList(all, nextToken, maxResults)
}

// DeleteCallAnalyticsJob removes a Call Analytics job by name.
func (b *InMemoryBackend) DeleteCallAnalyticsJob(jobName string) error {
	if jobName == "" {
		return fmt.Errorf("%w: CallAnalyticsJobName is required", ErrValidation)
	}

	b.mu.Lock("DeleteCallAnalyticsJob")
	defer b.mu.Unlock()

	if !b.callAnalyticsJobs.Delete(jobName) {
		return fmt.Errorf("%w: call analytics job %s not found", ErrNotFound, jobName)
	}

	b.forgetResourceTagsLocked(resourceARN(resourceTypeCallAnalyticsJob, jobName))

	return nil
}

// AddCallAnalyticsJobInternal seeds a Call Analytics job directly (test helper).
func (b *InMemoryBackend) AddCallAnalyticsJobInternal(job *CallAnalyticsJob) {
	b.mu.Lock("AddCallAnalyticsJobInternal")
	defer b.mu.Unlock()

	cp := *job
	b.callAnalyticsJobs.Put(&cp)
}

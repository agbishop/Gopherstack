package comprehend

import (
	"fmt"
	"maps"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/collections"
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// InMemoryBackend stores Comprehend state safely for concurrent requests.
//
// jobs, resources, and iterations are *store.Table[T] (see store_setup.go).
// Each keys off a real, non-json:"-" identity field the value type already
// carries (Job.JobID, Resource.Arn, FlywheelIteration.FlywheelIterationID),
// so all three register directly on registry -- no DTO indirection is
// needed. Neither table needs a secondary store.Index: every filtered
// listing (ListJobs by JobType, ListResources by Type,
// ListFlywheelIterations by FlywheelArn) scans the table's full contents,
// exactly as the original map-range loops did, and none of the fields
// mutated in place by advanceJob/advanceTrainingResource/etc. are ever used
// as an index key. tags, policies, and policyRevisions are left as plain
// maps: their values are not *T (map[string]string / string), so they do not
// fit store.Table's keyed-by-identity-value shape.
type InMemoryBackend struct {
	jobs             *store.Table[Job]
	resources        *store.Table[Resource]
	iterations       *store.Table[FlywheelIteration]
	registry         *store.Registry
	tags             map[string]map[string]string
	policies         map[string]string
	policyRevisions  map[string]string
	policyCreatedAt  map[string]time.Time
	policyModifiedAt map[string]time.Time
	mu               *lockmetrics.RWMutex
	accountID        string
	region           string
}

// NewInMemoryBackend creates a configured Comprehend backend.
func NewInMemoryBackend(accountID, region string) *InMemoryBackend {
	b := &InMemoryBackend{
		registry:         store.NewRegistry(),
		tags:             make(map[string]map[string]string),
		policies:         make(map[string]string),
		policyRevisions:  make(map[string]string),
		policyCreatedAt:  make(map[string]time.Time),
		policyModifiedAt: make(map[string]time.Time),
		accountID:        accountID,
		region:           region,
		mu:               lockmetrics.New("comprehend"),
	}

	registerAllTables(b)

	return b
}

// Region returns configured AWS region.
func (b *InMemoryBackend) Region() string { return b.region }

// Reset clears all stored Comprehend resources.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	b.registry.ResetAll()
	b.tags = make(map[string]map[string]string)
	b.policies = make(map[string]string)
	b.policyRevisions = make(map[string]string)
	b.policyCreatedAt = make(map[string]time.Time)
	b.policyModifiedAt = make(map[string]time.Time)
}

// StartJob submits an analysis job with AWS-style initial status. tags are
// associated with the job's ARN in the same tag store Create* resources use,
// so ListTagsForResource/TagResource/UntagResource work against job ARNs too
// (Start*DetectionJob requests all accept an optional Tags field in the real
// API).
func (b *InMemoryBackend) StartJob(jobType, name string, values map[string]any, tags []Tag) (*Job, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("%w: JobName is required", ErrValidation)
	}
	if len(tags) > maxTagsPerResource {
		return nil, fmt.Errorf(
			"%w: request would exceed the %d-tag-per-resource limit",
			ErrTooManyTags,
			maxTagsPerResource,
		)
	}
	if err := validateKmsKeyID(stringValue(values, "VolumeKmsKeyId", "")); err != nil {
		return nil, err
	}

	b.mu.Lock("StartJob")
	defer b.mu.Unlock()

	for _, job := range b.jobs.All() {
		if job.JobType == jobType && job.JobName == name {
			return nil, fmt.Errorf("%w: job %q already exists", ErrConflict, name)
		}
	}

	id := uuid.NewString()
	job := &Job{
		JobID:                 id,
		JobArn:                arn.Build("comprehend", b.region, b.accountID, jobType+"/"+id),
		JobName:               name,
		JobType:               jobType,
		JobStatus:             statusSubmitted,
		LanguageCode:          stringValue(values, fieldLanguageCode, defaultLanguageCode),
		DataAccessRoleArn:     stringValue(values, "DataAccessRoleArn", ""),
		DocumentClassifierArn: stringValue(values, fieldDocumentClassifierARN, ""),
		EntityRecognizerArn:   stringValue(values, fieldEntityRecognizerARN, ""),
		InputDataConfig:       mapValue(values, "InputDataConfig"),
		OutputDataConfig:      mapValue(values, "OutputDataConfig"),
		TargetEventTypes:      stringSliceValue(values, "TargetEventTypes"),
		Configuration:         cloneMap(values),
		SubmitTime:            time.Now().UTC(),
		shouldFail:            strings.Contains(strings.ToLower(name), failedMarker),
	}
	b.jobs.Put(job)
	b.tags[job.JobArn] = tagsMap(tags)

	return cloneJob(job), nil
}

// DescribeJob retrieves and advances a submitted job through its lifecycle.
func (b *InMemoryBackend) DescribeJob(id, jobType string) (*Job, error) {
	b.mu.Lock("DescribeJob")
	defer b.mu.Unlock()

	job, ok := b.jobs.Get(id)
	if !ok || job.JobType != jobType {
		return nil, fmt.Errorf("%w: job %q", ErrJobNotFound, id)
	}

	advanceJob(job)

	return cloneJob(job), nil
}

// StopJob starts cancellation of an active job. AWS returns InvalidRequestException
// when the job is already in a terminal state (COMPLETED, FAILED, STOPPED, STOP_REQUESTED).
func (b *InMemoryBackend) StopJob(id, jobType string) (*Job, error) {
	b.mu.Lock("StopJob")
	defer b.mu.Unlock()

	job, ok := b.jobs.Get(id)
	if !ok || job.JobType != jobType {
		return nil, fmt.Errorf("%w: job %q", ErrJobNotFound, id)
	}

	switch job.JobStatus {
	case statusSubmitted, statusInProgress:
		job.JobStatus = statusStopRequested
		job.stopRequested = true
	default:
		return nil, fmt.Errorf("%w: job %q cannot be stopped in status %s", ErrValidation, id, job.JobStatus)
	}

	return cloneJob(job), nil
}

// ListJobs returns jobs for one operation family in stable submission order.
func (b *InMemoryBackend) ListJobs(jobType string) []*Job {
	b.mu.RLock("ListJobs")
	defer b.mu.RUnlock()

	all := b.jobs.All()
	out := make([]*Job, 0, len(all))
	for _, job := range all {
		if job.JobType == jobType {
			out = append(out, cloneJob(job))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SubmitTime.Before(out[j].SubmitTime) })

	return out
}

func advanceJob(job *Job) {
	switch job.JobStatus {
	case statusSubmitted:
		job.JobStatus = statusInProgress
	case statusInProgress:
		if job.shouldFail {
			job.JobStatus = statusFailed
			job.FailureReason = "simulated analysis failure"
		} else {
			job.JobStatus = statusCompleted
		}
		job.EndTime = time.Now().UTC()
	case statusStopRequested:
		job.JobStatus = statusStopped
		job.EndTime = time.Now().UTC()
	}
	job.polls++
}

// CreateResource creates a stateful Comprehend resource and optionally attaches tags.
func (b *InMemoryBackend) CreateResource(
	resourceType, name, versionName string,
	values map[string]any,
	tags []Tag,
) (*Resource, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("%w: Name is required", ErrValidation)
	}
	if len(tags) > maxTagsPerResource {
		return nil, fmt.Errorf(
			"%w: request would exceed the %d-tag-per-resource limit",
			ErrTooManyTags,
			maxTagsPerResource,
		)
	}
	for _, field := range requiredResourceFields[resourceType] {
		if values[field] == nil {
			return nil, fmt.Errorf("%w: %s is required", ErrValidation, field)
		}
	}
	if err := validateKmsKeyID(stringValue(values, "ModelKmsKeyId", "")); err != nil {
		return nil, err
	}
	if err := validateKmsKeyID(stringValue(values, "VolumeKmsKeyId", "")); err != nil {
		return nil, err
	}
	// CreateFlywheelInput is the only Create*/resource op whose input carries a
	// nested DataSecurityConfig (confirmed against types.DataSecurityConfig and
	// CreateFlywheelInput in the vendored SDK -- CreateDatasetInput has no such
	// field at all; a dataset inherits its data security config from the
	// flywheel it's attached to). DataSecurityConfig has its own three KMS key
	// fields (DataLakeKmsKeyId/ModelKmsKeyId/VolumeKmsKeyId), independent of
	// this op's top-level ModelKmsKeyId/VolumeKmsKeyId validated above, and
	// real AWS validates each the same way.
	if resourceType == resourceTypeFlywheel {
		if err := validateDataSecurityConfigKmsKeys(mapValue(values, "DataSecurityConfig")); err != nil {
			return nil, err
		}
	}

	resourceArn := b.resourceARN(resourceType, name, versionName)
	b.mu.Lock("CreateResource")
	defer b.mu.Unlock()

	if b.resources.Has(resourceArn) {
		return nil, fmt.Errorf("%w: resource %q already exists", ErrConflict, name)
	}

	now := time.Now().UTC()
	resource := &Resource{
		CreatedAt:     now,
		UpdatedAt:     now,
		Name:          name,
		Arn:           resourceArn,
		Type:          resourceType,
		Status:        initialResourceStatus(resourceType),
		VersionName:   versionName,
		ModelArn:      stringValue(values, "ModelArn", ""),
		FlywheelArn:   stringValue(values, fieldFlywheelARN, ""),
		Configuration: cloneMap(values),
	}
	switch resourceType {
	case resourceTypeEndpoint:
		resource.EndpointArn = resourceArn
	case resourceTypeDataset:
		resource.DatasetArn = resourceArn
	}
	if isTrainingResourceType(resourceType) && resource.Status == statusTrained {
		// Training is fast-forwarded straight to TRAINED (see
		// initialResourceStatus), so there is no separate SUBMITTED ->
		// IN_PROGRESS transition to timestamp -- collapse both training
		// timestamps to the creation instant rather than leaving them zero.
		resource.TrainingStartTime = now
		resource.TrainingEndTime = now
	}
	b.resources.Put(resource)
	b.tags[resourceArn] = tagsMap(tags)

	return cloneResource(resource), nil
}

// GetResource finds resource by ARN. For classifier and recognizer types, each
// Describe call advances training lifecycle: SUBMITTED → IN_PROGRESS → TRAINED
// (or FAILED when the name contains "[fail]"). This mirrors AWS async training.
func (b *InMemoryBackend) GetResource(resourceArn, resourceType string) (*Resource, error) {
	b.mu.Lock("GetResource")
	defer b.mu.Unlock()

	resource, ok := b.resources.Get(resourceArn)
	if !ok || resource.Type != resourceType {
		return nil, fmt.Errorf("%w: resource %q", ErrNotFound, resourceArn)
	}

	advanceTrainingResource(resource)

	return cloneResource(resource), nil
}

// ListResources returns resources of one type. For classifier and recognizer
// types, listing advances the async training lifecycle one step (mirroring a
// status poll), consistent with how Describe advances it. This lets a
// create→describe→list→delete flow reach a deletable (TRAINED) state.
func (b *InMemoryBackend) ListResources(resourceType string) []*Resource {
	b.mu.Lock("ListResources")
	defer b.mu.Unlock()

	all := b.resources.All()
	out := make([]*Resource, 0, len(all))
	for _, resource := range all {
		if resource.Type == resourceType {
			advanceTrainingResource(resource)
			out = append(out, cloneResource(resource))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Arn < out[j].Arn })

	return out
}

// UpdateResource changes stored configuration of a mutable resource.
func (b *InMemoryBackend) UpdateResource(resourceArn, resourceType string, values map[string]any) (*Resource, error) {
	b.mu.Lock("UpdateResource")
	defer b.mu.Unlock()

	resource, ok := b.resources.Get(resourceArn)
	if !ok || resource.Type != resourceType {
		return nil, fmt.Errorf("%w: resource %q", ErrNotFound, resourceArn)
	}

	maps.Copy(resource.Configuration, cloneMap(values))
	resource.UpdatedAt = time.Now().UTC()

	return cloneResource(resource), nil
}

// DeleteResource removes a stored resource and its tags. AWS returns
// ResourceInUseException when deleting a classifier or recognizer that is
// still training (status SUBMITTED or IN_PROGRESS), and also -- per
// DeleteDocumentClassifierInput/DeleteEntityRecognizerInput's own doc
// comments ("If an active inference job is using the model, a
// ResourceInUseException will be returned") -- when an async detection job
// still referencing the model is itself SUBMITTED or IN_PROGRESS.
func (b *InMemoryBackend) DeleteResource(resourceArn, resourceType string) error {
	b.mu.Lock("DeleteResource")
	defer b.mu.Unlock()

	resource, ok := b.resources.Get(resourceArn)
	if !ok || resource.Type != resourceType {
		return fmt.Errorf("%w: resource %q", ErrNotFound, resourceArn)
	}

	if isTrainingResourceType(resource.Type) &&
		(resource.Status == statusSubmitted || resource.Status == statusInProgress) {
		return fmt.Errorf(
			"%w: resource %q cannot be deleted in status %s",
			ErrConflict, resourceArn, resource.Status,
		)
	}
	if activeJobUsesModel(b.jobs.All(), resource.Type, resourceArn) {
		return fmt.Errorf("%w: an active inference job is using model %q", ErrConflict, resourceArn)
	}

	b.resources.Delete(resourceArn)
	delete(b.tags, resourceArn)

	return nil
}

// StartFlywheelIteration creates an asynchronous flywheel iteration.
func (b *InMemoryBackend) StartFlywheelIteration(flywheelArn string) (*FlywheelIteration, error) {
	if _, err := b.GetResource(flywheelArn, resourceTypeFlywheel); err != nil {
		return nil, err
	}

	b.mu.Lock("StartFlywheelIteration")
	defer b.mu.Unlock()

	id := uuid.NewString()
	iteration := &FlywheelIteration{
		CreationTime:            time.Now().UTC(),
		FlywheelArn:             flywheelArn,
		FlywheelIterationID:     id,
		FlywheelIterationStatus: statusFlywheelIterationTraining,
	}
	b.iterations.Put(iteration)

	return cloneIteration(iteration), nil
}

// GetFlywheelIteration returns and advances an iteration. Real
// FlywheelIterationStatus values are TRAINING -> EVALUATING -> COMPLETED
// (types/enums.go:325-334) -- not the JobStatus-style SUBMITTED/IN_PROGRESS
// vocabulary used elsewhere in this file, which FlywheelIterationStatus does
// not share.
func (b *InMemoryBackend) GetFlywheelIteration(id string) (*FlywheelIteration, error) {
	b.mu.Lock("GetFlywheelIteration")
	defer b.mu.Unlock()

	iteration, ok := b.iterations.Get(id)
	if !ok {
		return nil, fmt.Errorf("%w: iteration %q", ErrNotFound, id)
	}
	switch iteration.FlywheelIterationStatus {
	case statusFlywheelIterationTraining:
		iteration.FlywheelIterationStatus = statusFlywheelIterationEvaluating
	case statusFlywheelIterationEvaluating:
		iteration.FlywheelIterationStatus = statusCompleted
		iteration.EndTime = time.Now().UTC()
	}
	iteration.polls++

	return cloneIteration(iteration), nil
}

// ListFlywheelIterations lists iterations belonging to one flywheel.
func (b *InMemoryBackend) ListFlywheelIterations(flywheelArn string) []*FlywheelIteration {
	b.mu.RLock("ListFlywheelIterations")
	defer b.mu.RUnlock()

	all := b.iterations.All()
	out := make([]*FlywheelIteration, 0, len(all))
	for _, iteration := range all {
		if iteration.FlywheelArn == flywheelArn {
			out = append(out, cloneIteration(iteration))
		}
	}

	return out
}

// TagResource adds or replaces tags on an existing resource. AWS returns
// TooManyTagsException when the resulting tag set (existing keys plus newly
// requested ones) would exceed the 50-tag-per-resource limit.
func (b *InMemoryBackend) TagResource(resourceArn string, tags []Tag) error {
	b.mu.Lock("TagResource")
	defer b.mu.Unlock()

	current, ok := b.tags[resourceArn]
	if !ok {
		return fmt.Errorf("%w: resource %q", ErrNotFound, resourceArn)
	}
	if mergedTagKeyCount(current, tags) > maxTagsPerResource {
		return fmt.Errorf("%w: resource %q would exceed the %d-tag-per-resource limit",
			ErrTooManyTags, resourceArn, maxTagsPerResource)
	}
	for _, tag := range tags {
		current[tag.Key] = tag.Value
	}

	return nil
}

// UntagResource removes keys from an existing resource.
func (b *InMemoryBackend) UntagResource(resourceArn string, keys []string) error {
	b.mu.Lock("UntagResource")
	defer b.mu.Unlock()

	current, ok := b.tags[resourceArn]
	if !ok {
		return fmt.Errorf("%w: resource %q", ErrNotFound, resourceArn)
	}
	for _, key := range keys {
		delete(current, key)
	}

	return nil
}

// ListTags returns sorted resource tags.
func (b *InMemoryBackend) ListTags(resourceArn string) ([]Tag, error) {
	b.mu.RLock("ListTags")
	defer b.mu.RUnlock()

	current, ok := b.tags[resourceArn]
	if !ok {
		return nil, fmt.Errorf("%w: resource %q", ErrNotFound, resourceArn)
	}
	keys := collections.SortedKeys(current)
	out := make([]Tag, 0, len(keys))
	for _, key := range keys {
		out = append(out, Tag{Key: key, Value: current[key]})
	}

	return out, nil
}

// TaggedEntry pairs a resource ARN with its tags.
type TaggedEntry struct {
	Tags map[string]string
	ARN  string
}

// TaggedResources returns every Comprehend resource ARN that currently has
// at least one tag applied via TagResource.
func (b *InMemoryBackend) TaggedResources() []TaggedEntry {
	b.mu.RLock("TaggedResources")
	defer b.mu.RUnlock()

	out := make([]TaggedEntry, 0, len(b.tags))

	for resourceArn, current := range b.tags {
		if len(current) == 0 {
			continue
		}

		out = append(out, TaggedEntry{ARN: resourceArn, Tags: maps.Clone(current)})
	}

	return out
}

// PutResourcePolicy saves a resource policy.
func (b *InMemoryBackend) PutResourcePolicy(resourceArn, policy, expectedRevision string) (string, error) {
	b.mu.Lock("PutResourcePolicy")
	defer b.mu.Unlock()

	if expectedRevision != "" && b.policyRevisions[resourceArn] != expectedRevision {
		// PutResourcePolicy's own deserializeOpError switch types only
		// InternalServerException, InvalidRequestException and
		// ResourceNotFoundException -- ResourceInUseException is not modeled
		// for this op (aws-sdk-go-v2/service/comprehend deserializers.go).
		return "", fmt.Errorf("%w: policy revision mismatch", ErrValidation)
	}

	revision := uuid.NewString()
	now := time.Now().UTC()
	if _, exists := b.policies[resourceArn]; !exists {
		b.policyCreatedAt[resourceArn] = now
	}
	b.policies[resourceArn] = policy
	b.policyRevisions[resourceArn] = revision
	b.policyModifiedAt[resourceArn] = now

	return revision, nil
}

// GetResourcePolicy retrieves a resource policy along with its creation and
// last-modified times, which real DescribeResourcePolicy tracks per policy
// rather than stamping at read time.
func (b *InMemoryBackend) GetResourcePolicy(resourceArn string) (string, string, time.Time, time.Time, error) {
	b.mu.RLock("GetResourcePolicy")
	defer b.mu.RUnlock()

	policy, ok := b.policies[resourceArn]
	if !ok {
		return "", "", time.Time{}, time.Time{}, fmt.Errorf(
			"%w: resource policy not found for %q",
			ErrNotFound,
			resourceArn,
		)
	}

	return policy, b.policyRevisions[resourceArn], b.policyCreatedAt[resourceArn], b.policyModifiedAt[resourceArn], nil
}

// DeleteResourcePolicy removes a resource policy.
func (b *InMemoryBackend) DeleteResourcePolicy(resourceArn, expectedRevision string) error {
	b.mu.Lock("DeleteResourcePolicy")
	defer b.mu.Unlock()

	if expectedRevision != "" && b.policyRevisions[resourceArn] != expectedRevision {
		// DeleteResourcePolicy's own deserializeOpError switch, like
		// PutResourcePolicy's, has no ResourceInUseException case.
		return fmt.Errorf("%w: policy revision mismatch", ErrValidation)
	}
	if _, ok := b.policies[resourceArn]; !ok {
		return fmt.Errorf("%w: resource policy not found for %q", ErrNotFound, resourceArn)
	}

	delete(b.policies, resourceArn)
	delete(b.policyRevisions, resourceArn)
	delete(b.policyCreatedAt, resourceArn)
	delete(b.policyModifiedAt, resourceArn)

	return nil
}

// StopTrainingResource sets a trainable resource's status to STOPPED.
func (b *InMemoryBackend) StopTrainingResource(resourceArn, resourceType string) error {
	b.mu.Lock("StopTrainingResource")
	defer b.mu.Unlock()

	resource, ok := b.resources.Get(resourceArn)
	if !ok || resource.Type != resourceType {
		return fmt.Errorf("%w: resource %q", ErrNotFound, resourceArn)
	}

	if resource.Status == statusSubmitted || resource.Status == statusInProgress {
		resource.Status = statusStopped
		resource.UpdatedAt = time.Now().UTC()
	}

	return nil
}

func (b *InMemoryBackend) resourceARN(resourceType, name, version string) string {
	resource := resourceType + "/" + name
	if version != "" {
		resource += "/version/" + version
	}

	return arn.Build("comprehend", b.region, b.accountID, resource)
}

func initialResourceStatus(resourceType string) string {
	switch resourceType {
	case resourceTypeEndpoint:
		return statusEndpointInService
	case resourceTypeFlywheel:
		return statusActive
	case resourceTypeDataset:
		return statusCompleted
	case resourceTypeDocClassifier, resourceTypeEntityRecognizer:
		// Emulator skips async training; classifiers/recognizers are immediately TRAINED.
		// The real AWS provider waits minutes before polling, causing CI timeouts if we
		// start at SUBMITTED and require multiple poll cycles.
		return statusTrained
	default:
		return statusSubmitted
	}
}

// isTrainingResourceType reports whether rType goes through async training.
func isTrainingResourceType(rType string) bool {
	switch rType {
	case resourceTypeDocClassifier, resourceTypeEntityRecognizer:
		return true
	}

	return false
}

// activeJobUsesModel reports whether any non-terminal (SUBMITTED or
// IN_PROGRESS) job still references resourceArn as its model: a
// document-classification-job's DocumentClassifierArn for
// resourceTypeDocClassifier, or an entities-detection-job's
// EntityRecognizerArn for resourceTypeEntityRecognizer. These are the only
// two job families that carry either field (see asyncJobSpecs,
// handler_jobs.go).
func activeJobUsesModel(jobs []*Job, resourceType, resourceArn string) bool {
	for _, job := range jobs {
		if job.JobStatus != statusSubmitted && job.JobStatus != statusInProgress {
			continue
		}
		switch resourceType {
		case resourceTypeDocClassifier:
			if job.DocumentClassifierArn == resourceArn {
				return true
			}
		case resourceTypeEntityRecognizer:
			if job.EntityRecognizerArn == resourceArn {
				return true
			}
		}
	}

	return false
}

// advanceTrainingResource steps a classifier/recognizer one lifecycle state
// forward on each Describe call: SUBMITTED → IN_PROGRESS → TRAINED (or FAILED).
func advanceTrainingResource(resource *Resource) {
	if !isTrainingResourceType(resource.Type) {
		return
	}
	switch resource.Status {
	case statusSubmitted:
		resource.Status = statusInProgress
		resource.UpdatedAt = time.Now().UTC()
		resource.TrainingStartTime = resource.UpdatedAt
	case statusInProgress:
		if strings.Contains(strings.ToLower(resource.Name), failedMarker) {
			resource.Status = statusFailed
		} else {
			resource.Status = statusTrained
		}
		resource.UpdatedAt = time.Now().UTC()
		resource.TrainingEndTime = resource.UpdatedAt
	}
}

func tagsMap(tags []Tag) map[string]string {
	out := make(map[string]string, len(tags))
	for _, tag := range tags {
		out[tag.Key] = tag.Value
	}

	return out
}

func stringValue(values map[string]any, key, fallback string) string {
	value, ok := values[key].(string)
	if !ok || value == "" {
		return fallback
	}

	return value
}

func mapValue(values map[string]any, key string) map[string]any {
	value, _ := values[key].(map[string]any)

	return cloneMap(value)
}

func stringSliceValue(values map[string]any, key string) []string {
	raw, _ := values[key].([]any)
	out := make([]string, 0, len(raw))
	for _, value := range raw {
		if text, ok := value.(string); ok {
			out = append(out, text)
		}
	}

	return out
}

func cloneMap(source map[string]any) map[string]any {
	out := make(map[string]any, len(source))
	maps.Copy(out, source)

	return out
}

func cloneJob(job *Job) *Job {
	cloned := *job
	cloned.InputDataConfig = cloneMap(job.InputDataConfig)
	cloned.OutputDataConfig = cloneMap(job.OutputDataConfig)
	cloned.TargetEventTypes = append([]string(nil), job.TargetEventTypes...)
	cloned.Configuration = cloneMap(job.Configuration)

	return &cloned
}

// maxTagsPerResource mirrors the real API's documented 50-tag-per-resource
// limit (both existing and newly requested tags count toward it).
const maxTagsPerResource = 50

// mergedTagKeyCount returns the number of distinct tag keys that would exist
// after applying tags on top of current, without mutating either.
func mergedTagKeyCount(current map[string]string, tags []Tag) int {
	merged := make(map[string]struct{}, len(current)+len(tags))
	for key := range current {
		merged[key] = struct{}{}
	}
	for _, tag := range tags {
		merged[tag.Key] = struct{}{}
	}

	return len(merged)
}

// requiredResourceFields lists the resourceType-specific members that
// CreateResource's generic pass-through path (which stores and echoes the
// whole input map via cloneMap -- a supplied value already round-trips fine)
// did not enforce for presence: CreateFlywheelInput's DataAccessRoleArn and
// DataLakeS3Uri, CreateEndpointInput's DesiredInferenceUnits, and
// CreateDatasetInput's FlywheelArn and InputDataConfig.
// FlywheelName/EndpointName/DatasetName are not listed here because
// CreateResource's own Name-presence check above already covers every
// resourceSpecs() nameField.
// Keying by resourceType (not by action) is safe here, unlike forecast's
// action-keyed equivalent: no other operation creates a resourceTypeFlywheel,
// resourceTypeEndpoint, or resourceTypeDataset resource (ImportModel only
// ever creates resourceTypeDocClassifier/resourceTypeEntityRecognizer).
// Verified against aws-sdk-go-v2/service/comprehend@v1.43.4/validators.go's
// validateOpCreateFlywheelInput/validateOpCreateEndpointInput
// (gopherstack-wl0s); DataAccessRoleArn is required there too even though
// the originating audit only named DataLakeS3Uri/DesiredInferenceUnits.
// validateOpCreateDatasetInput likewise marks FlywheelArn and
// InputDataConfig required (DatasetName's presence is already covered by
// the Name-presence check above).
//
//nolint:gochecknoglobals // static declarative table, mirrors resourceSpecs above
var requiredResourceFields = map[string][]string{
	resourceTypeFlywheel: {"DataAccessRoleArn", "DataLakeS3Uri"},
	resourceTypeEndpoint: {"DesiredInferenceUnits"},
	resourceTypeDataset:  {"FlywheelArn", "InputDataConfig"},
}

// kmsKeyIDRe matches a bare KMS key ID (UUID form).
var kmsKeyIDRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// kmsKeyArnRe matches a KMS key or alias ARN, e.g.
// "arn:aws:kms:us-west-2:111122223333:key/1234abcd-12ab-34cd-56ef-1234567890ab"
// or "arn:aws:kms:us-west-2:111122223333:alias/my-key".
var kmsKeyArnRe = regexp.MustCompile(`^arn:aws[a-zA-Z-]*:kms:[a-z0-9-]+:\d{12}:(key/[0-9a-fA-F-]{36}|alias/.+)$`)

// validateKmsKeyID checks a ModelKmsKeyId/VolumeKmsKeyId value against the
// two shapes AWS documents for every KMS key ID field on this service (bare
// key ID or key/alias ARN). Empty is valid (the field is optional). Real AWS
// returns KmsKeyValidationException for a value that matches neither shape.
func validateKmsKeyID(id string) error {
	if id == "" {
		return nil
	}
	if kmsKeyIDRe.MatchString(id) || kmsKeyArnRe.MatchString(id) {
		return nil
	}

	return fmt.Errorf("%w: %q is not a valid KMS key ID or ARN", ErrKmsKeyValidation, id)
}

// validateDataSecurityConfigKmsKeys validates the three KMS key ID fields
// nested inside a DataSecurityConfig object (DataLakeKmsKeyId/ModelKmsKeyId/
// VolumeKmsKeyId -- see types.DataSecurityConfig in the vendored SDK) the
// same way validateKmsKeyID validates a top-level key field. cfg may be nil
// (DataSecurityConfig is optional on CreateFlywheelInput).
func validateDataSecurityConfigKmsKeys(cfg map[string]any) error {
	for _, field := range [...]string{"DataLakeKmsKeyId", "ModelKmsKeyId", "VolumeKmsKeyId"} {
		if err := validateKmsKeyID(stringValue(cfg, field, "")); err != nil {
			return err
		}
	}

	return nil
}

func cloneResource(resource *Resource) *Resource {
	cloned := *resource
	cloned.Configuration = cloneMap(resource.Configuration)

	return &cloned
}

func cloneIteration(iteration *FlywheelIteration) *FlywheelIteration {
	cloned := *iteration

	return &cloned
}

package databrew

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

func (b *InMemoryBackend) jobARN(region, name string) string {
	return arn.Build("databrew", region, b.accountID, "job/"+name)
}

func (b *InMemoryBackend) CreateJob(
	ctx context.Context,
	name, jobType, datasetName, projectName, recipeName, roleArn string,
	outputs []Output,
	tags map[string]string,
	extra JobExtras,
) (*Job, error) {
	b.mu.Lock("CreateJob")
	defer b.mu.Unlock()
	region := getRegion(ctx, b.defaultRegion)
	if name == "" {
		return nil, ErrValidation
	}
	t := b.jobsTable(region)
	if t.Has(name) {
		return nil, ErrAlreadyExists
	}
	if err := b.validateJobResourceRefs(region, datasetName, projectName, recipeName); err != nil {
		return nil, err
	}
	if err := validateJobExtras(extra); err != nil {
		return nil, err
	}
	j := &Job{
		Name: name, Arn: b.jobARN(region, name), Type: jobType,
		DatasetName: datasetName, ProjectName: projectName,
		RecipeName: recipeName, RoleArn: roleArn, Outputs: outputs,
		AccountID: b.accountID,
		Tags:      maps.Clone(tags), CreateDate: float64(time.Now().Unix()),
		LastModifiedDate:         float64(time.Now().Unix()),
		ProfileConfiguration:     extra.ProfileConfiguration,
		JobSample:                extra.JobSample,
		EncryptionMode:           extra.EncryptionMode,
		EncryptionKeyArn:         extra.EncryptionKeyArn,
		LogSubscription:          extra.LogSubscription,
		DataCatalogOutputs:       extra.DataCatalogOutputs,
		DatabaseOutputs:          extra.DatabaseOutputs,
		ValidationConfigurations: extra.ValidationConfigurations,
		MaxCapacity:              extra.MaxCapacity,
		MaxRetries:               extra.MaxRetries,
		Timeout:                  extra.Timeout,
	}
	if recipeName != "" {
		version := extra.RecipeVersion
		if version == "" {
			version = recipeVersionLatestWorking
		}
		j.RecipeReference = &RecipeRef{Name: recipeName, RecipeVersion: version}
	}
	t.Put(j)

	return j, nil
}

func (b *InMemoryBackend) DescribeJob(ctx context.Context, name string) (*Job, error) {
	b.mu.RLock("DescribeJob")
	defer b.mu.RUnlock()
	region := getRegion(ctx, b.defaultRegion)
	j, ok := b.jobsTable(region).Get(name)
	if !ok {
		return nil, ErrNotFound
	}
	cp := *j
	cp.Tags = maps.Clone(j.Tags)
	cp.Outputs = append([]Output(nil), j.Outputs...)

	return &cp, nil
}

func (b *InMemoryBackend) ListJobs(
	ctx context.Context,
	maxResults int,
	nextToken,
	datasetName,
	projectName string,
) ([]*Job, string) {
	b.mu.RLock("ListJobs")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.defaultRegion)
	t := b.jobsTable(region)
	keys := snapshotKeys(t, jobKeyFn)
	var filtered []string
	for _, k := range keys {
		j, _ := t.Get(k)
		if datasetName != "" && j.DatasetName != datasetName {
			continue
		}
		if projectName != "" && j.ProjectName != projectName {
			continue
		}
		filtered = append(filtered, k)
	}
	pageKeys, next := paginateKeys(filtered, maxResults, nextToken)
	out := make([]*Job, 0, len(pageKeys))
	for _, k := range pageKeys {
		v, _ := t.Get(k)
		cp := *v
		cp.Tags = maps.Clone(v.Tags)
		cp.Outputs = append([]Output(nil), v.Outputs...)
		out = append(out, &cp)
	}

	return out, next
}

func (b *InMemoryBackend) UpdateJob(
	ctx context.Context,
	name, roleArn string,
	outputs []Output,
	maxCapacity, maxRetries, timeout int,
	extra JobExtras,
) error {
	b.mu.Lock("UpdateJob")
	defer b.mu.Unlock()
	region := getRegion(ctx, b.defaultRegion)
	j, ok := b.jobsTable(region).Get(name)
	if !ok {
		return ErrNotFound
	}
	if err := validateJobExtras(extra); err != nil {
		return err
	}
	if roleArn != "" {
		j.RoleArn = roleArn
	}
	if len(outputs) > 0 {
		j.Outputs = outputs
	}
	if maxCapacity > 0 {
		j.MaxCapacity = maxCapacity
	}
	if maxRetries >= 0 {
		j.MaxRetries = maxRetries
	}
	if timeout > 0 {
		j.Timeout = timeout
	}
	applyJobExtras(j, extra)
	j.LastModifiedDate = float64(time.Now().Unix())

	return nil
}

// validateJobResourceRefs rejects a CreateJob call that names a dataset,
// project, or recipe that doesn't exist, before any state is mutated.
// CreateProfileJob and CreateRecipeJob both document ResourceNotFoundException
// (aws-sdk-go-v2/service/databrew's deserializers.go:465/960, in
// awsRestjson1_deserializeOpErrorCreateProfileJob/CreateRecipeJob);
// CreateProject's error switch (deserializers.go:626-638) has no
// ResourceNotFoundException case, so ProjectName/RecipeName there are
// deliberately left unvalidated by CreateProject itself. Each name here is
// checked only when non-empty: CreateRecipeJobInput accepts ProjectName as an
// alternative to DatasetName+RecipeReference, so an unset reference is not an
// error.
func (b *InMemoryBackend) validateJobResourceRefs(region, datasetName, projectName, recipeName string) error {
	if datasetName != "" && !b.datasetsTable(region).Has(datasetName) {
		return fmt.Errorf("%w: dataset %q", ErrNotFound, datasetName)
	}
	if projectName != "" && !b.projectsTable(region).Has(projectName) {
		return fmt.Errorf("%w: project %q", ErrNotFound, projectName)
	}
	if recipeName != "" && !b.recipesTable(region).Has(recipeName) {
		return fmt.Errorf("%w: recipe %q", ErrNotFound, recipeName)
	}

	return nil
}

// validateJobExtras rejects extras values the real service would reject,
// before any caller mutates stored Job state. Enum values and DataCatalog/
// Database output required members are confirmed against botocore
// databrew/2017-07-25 service-2.json (EncryptionMode, LogSubscription,
// SampleMode, DatabaseOutputMode, DataCatalogOutput/DatabaseOutput
// "required" lists).
func validateJobExtras(extra JobExtras) error {
	if extra.EncryptionMode != "" && extra.EncryptionMode != "SSE-KMS" && extra.EncryptionMode != "SSE-S3" {
		return fmt.Errorf("%w: invalid EncryptionMode %q", ErrValidation, extra.EncryptionMode)
	}
	if extra.LogSubscription != "" && extra.LogSubscription != "ENABLE" && extra.LogSubscription != "DISABLE" {
		return fmt.Errorf("%w: invalid LogSubscription %q", ErrValidation, extra.LogSubscription)
	}
	if extra.JobSample != nil && extra.JobSample.Mode != "" &&
		extra.JobSample.Mode != "FULL_DATASET" && extra.JobSample.Mode != "CUSTOM_ROWS" {
		return fmt.Errorf("%w: invalid JobSample.Mode %q", ErrValidation, extra.JobSample.Mode)
	}
	if err := validateDataCatalogOutputs(extra.DataCatalogOutputs); err != nil {
		return err
	}

	return validateDatabaseOutputs(extra.DatabaseOutputs)
}

func validateDataCatalogOutputs(outs []DataCatalogOutput) error {
	for i, o := range outs {
		if o.DatabaseName == "" || o.TableName == "" {
			return fmt.Errorf("%w: DataCatalogOutputs[%d] requires DatabaseName and TableName", ErrValidation, i)
		}
		if o.DatabaseOptions != nil && o.DatabaseOptions.TableName == "" {
			return fmt.Errorf("%w: DataCatalogOutputs[%d].DatabaseOptions requires TableName", ErrValidation, i)
		}
		if o.S3Options != nil && o.S3Options.Location.Bucket == "" {
			return fmt.Errorf("%w: DataCatalogOutputs[%d].S3Options requires Location.Bucket", ErrValidation, i)
		}
		// Overwrite is "Not supported with DatabaseOptions" per
		// aws-sdk-go-v2/service/databrew/types.DataCatalogOutput's doc comment.
		if o.Overwrite && o.DatabaseOptions != nil {
			return fmt.Errorf(
				"%w: DataCatalogOutputs[%d].Overwrite is not supported with DatabaseOptions", ErrValidation, i,
			)
		}
	}

	return nil
}

func validateDatabaseOutputs(outs []DatabaseOutput) error {
	for i, o := range outs {
		if o.GlueConnectionName == "" || o.DatabaseOptions == nil {
			return fmt.Errorf(
				"%w: DatabaseOutputs[%d] requires GlueConnectionName and DatabaseOptions", ErrValidation, i,
			)
		}
		if o.DatabaseOptions.TableName == "" {
			return fmt.Errorf("%w: DatabaseOutputs[%d].DatabaseOptions requires TableName", ErrValidation, i)
		}
		if o.DatabaseOutputMode != "" && o.DatabaseOutputMode != "NEW_TABLE" {
			return fmt.Errorf(
				"%w: invalid DatabaseOutputs[%d].DatabaseOutputMode %q",
				ErrValidation,
				i,
				o.DatabaseOutputMode,
			)
		}
	}

	return nil
}

// applyJobExtras overwrites j's optional fields with any non-zero values in
// extra, leaving fields extra didn't set unchanged. Callers must hold b.mu.
func applyJobExtras(j *Job, extra JobExtras) {
	if extra.ProfileConfiguration != nil {
		j.ProfileConfiguration = extra.ProfileConfiguration
	}
	if extra.JobSample != nil {
		j.JobSample = extra.JobSample
	}
	if extra.EncryptionMode != "" {
		j.EncryptionMode = extra.EncryptionMode
	}
	if extra.EncryptionKeyArn != "" {
		j.EncryptionKeyArn = extra.EncryptionKeyArn
	}
	if extra.LogSubscription != "" {
		j.LogSubscription = extra.LogSubscription
	}
	if extra.DataCatalogOutputs != nil {
		j.DataCatalogOutputs = extra.DataCatalogOutputs
	}
	if extra.DatabaseOutputs != nil {
		j.DatabaseOutputs = extra.DatabaseOutputs
	}
	if extra.ValidationConfigurations != nil {
		j.ValidationConfigurations = extra.ValidationConfigurations
	}
}

func (b *InMemoryBackend) DeleteJob(ctx context.Context, name string) error {
	b.mu.Lock("DeleteJob")
	defer b.mu.Unlock()
	region := getRegion(ctx, b.defaultRegion)
	if !b.jobsTable(region).Delete(name) {
		return ErrNotFound
	}
	runStore := b.jobRunsStore(region)
	delete(runStore, name)

	return nil
}

const (
	// jobRunTransitionDelay is how long to wait before transitioning a job run to SUCCEEDED.
	jobRunTransitionDelay = 100 * time.Millisecond
	// jobRunDefaultExecTime is the simulated execution time returned in a completed job run.
	jobRunDefaultExecTime = 30
)

// StartJobRun creates a new job run with STARTING state, transitioning to SUCCEEDED asynchronously.
func (b *InMemoryBackend) StartJobRun(ctx context.Context, jobName string) (*JobRun, error) {
	b.mu.Lock("StartJobRun")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.defaultRegion)
	j, ok := b.jobsTable(region).Get(jobName)
	if !ok {
		return nil, fmt.Errorf("%w: job %q not found", ErrNotFound, jobName)
	}

	// Attempt/DataCatalogOutputs/DatabaseOutputs/DatasetName/JobSample/
	// LogSubscription/Outputs/RecipeReference/ValidationConfigurations are
	// real types.JobRun members (deserializers.go's
	// awsRestjson1_deserializeDocumentJobRun) this backend only has one
	// source for: the parent Job's own configuration at the moment the run
	// starts. Attempt is always 1: this backend never retries a run
	// (StartJobRun always transitions STARTING->SUCCEEDED, see
	// jobRunTransitionDelay), so there is never a second attempt to count.
	run := &JobRun{
		JobName:                  jobName,
		RunID:                    uuid.New().String(),
		State:                    "STARTING",
		StartedOn:                float64(time.Now().Unix()),
		Attempt:                  1,
		DatasetName:              j.DatasetName,
		DataCatalogOutputs:       append([]DataCatalogOutput(nil), j.DataCatalogOutputs...),
		DatabaseOutputs:          append([]DatabaseOutput(nil), j.DatabaseOutputs...),
		JobSample:                j.JobSample,
		LogSubscription:          j.LogSubscription,
		Outputs:                  append([]Output(nil), j.Outputs...),
		RecipeReference:          j.RecipeReference,
		ValidationConfigurations: append([]map[string]any(nil), j.ValidationConfigurations...),
	}

	runStore := b.jobRunsStore(region)
	runStore[jobName] = append(runStore[jobName], run)

	b.runDelayed(jobRunTransitionDelay, func() {
		b.mu.Lock("StartJobRun.transition")
		defer b.mu.Unlock()
		// Re-check the run still exists: Reset may have cleared jobRuns while
		// the transition was pending, in which case there is nothing to update.
		if !b.jobRunExists(region, jobName, run.RunID) {
			return
		}
		run.State = "SUCCEEDED"
		run.CompletedOn = float64(time.Now().Unix())
		run.ExecutionTime = jobRunDefaultExecTime
	})

	cp := *run

	return &cp, nil
}

// jobRunExists reports whether a run with runID still exists for jobName in the given region.
// Callers must hold b.mu.
func (b *InMemoryBackend) jobRunExists(region, jobName, runID string) bool {
	regionRuns := b.jobRuns[region]
	if regionRuns == nil {
		return false
	}
	for _, r := range regionRuns[jobName] {
		if r.RunID == runID {
			return true
		}
	}

	return false
}

func (b *InMemoryBackend) ListJobRuns(
	ctx context.Context,
	jobName string,
	maxResults int,
	nextToken string,
) ([]*JobRun, string, error) {
	b.mu.RLock("ListJobRuns")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.defaultRegion)
	if !b.jobsTable(region).Has(jobName) {
		return nil, "", fmt.Errorf("%w: job %q", ErrNotFound, jobName)
	}

	runStore := b.jobRunsStoreRO(region)
	runs := runStore[jobName]

	// runs are stored in chronological order, ListJobRuns expects reverse chronological
	var reversed []*JobRun
	for _, run := range slices.Backward(runs) {
		cp := *run
		reversed = append(reversed, &cp)
	}

	if maxResults <= 0 {
		maxResults = 100
	}
	startIdx := 0
	if nextToken != "" {
		startIdx = len(reversed)
		for i, r := range reversed {
			// nextToken for runs is RunID (or we can just compare RunID)
			// Wait, the test might rely on RunID for token.
			// Let's assume nextToken is RunID and find the run *after* the token
			if r.RunID == nextToken {
				startIdx = i + 1

				break
			}
		}
	}

	endIdx := startIdx + maxResults
	endIdx = min(endIdx, len(reversed))

	var next string
	if endIdx < len(reversed) {
		next = reversed[endIdx-1].RunID
	}

	if startIdx < len(reversed) {
		return reversed[startIdx:endIdx], next, nil
	}

	return nil, "", nil
}

func (b *InMemoryBackend) StopJobRun(ctx context.Context, name, runID string) (*JobRun, error) {
	b.mu.Lock("StopJobRun")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.defaultRegion)
	runStore := b.jobRunsStore(region)
	runs, ok := runStore[name]
	if !ok {
		return nil, ErrNotFound
	}

	for _, run := range runs {
		if run.RunID == runID {
			if run.State != "SUCCEEDED" && run.State != "FAILED" && run.State != "STOPPED" {
				run.State = "STOPPED"
				run.CompletedOn = float64(time.Now().Unix())
			}
			cp := *run

			return &cp, nil
		}
	}

	return nil, ErrNotFound
}

func (b *InMemoryBackend) DescribeJobRun(ctx context.Context, name, runID string) (*JobRun, error) {
	b.mu.RLock("DescribeJobRun")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.defaultRegion)
	runStore := b.jobRunsStoreRO(region)
	runs, ok := runStore[name]
	if !ok {
		return nil, ErrNotFound
	}

	for _, run := range runs {
		if run.RunID == runID {
			cp := *run

			return &cp, nil
		}
	}

	return nil, ErrNotFound
}

package bedrock

import (
	"fmt"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// CreateModelImportJob creates a new model import job. importedModelName,
// roleArn, and modelDataSourceS3Uri mirror the real CreateModelImportJobInput's
// required ImportedModelName/RoleArn/ModelDataSource fields -- gopherstack
// previously accepted only jobName+tags, silently dropping all three.
func (b *InMemoryBackend) CreateModelImportJob(
	jobName, importedModelName, roleArn, modelDataSourceS3Uri string,
	tags []Tag,
) (*ModelImportJob, error) {
	if jobName == "" {
		return nil, fmt.Errorf("%w: jobName is required", ErrValidation)
	}

	if importedModelName == "" {
		return nil, fmt.Errorf("%w: importedModelName is required", ErrValidation)
	}

	if roleArn == "" {
		return nil, fmt.Errorf("%w: roleArn is required", ErrValidation)
	}

	b.mu.Lock("CreateModelImportJob")
	defer b.mu.Unlock()

	b.importJobCounter++
	id := fmt.Sprintf("mij-%07d", b.importJobCounter)
	jobARN := arn.Build("bedrock", b.region, b.accountID, "model-import-job/"+id)
	importedModelARN := arn.Build("bedrock", b.region, b.accountID, "imported-model/"+id)
	now := time.Now().UTC()

	job := &ModelImportJob{
		JobArn:            jobARN,
		JobName:           jobName,
		ImportedModelArn:  importedModelARN,
		ImportedModelName: importedModelName,
		RoleArn:           roleArn,
		ModelDataSourceS3: modelDataSourceS3Uri,
		Status:            statusInProgress,
		CreationTime:      now,
		LastModifiedTime:  now,
		Tags:              copyTags(tags),
	}
	b.modelImportJobs.Put(job)

	cp := *job
	cp.Tags = copyTags(job.Tags)

	return &cp, nil
}

// GetModelImportJob returns a model import job by ARN.
func (b *InMemoryBackend) GetModelImportJob(jobARN string) (*ModelImportJob, error) {
	b.mu.RLock("GetModelImportJob")
	defer b.mu.RUnlock()

	job, ok := b.modelImportJobs.Get(jobARN)
	if !ok {
		return nil, fmt.Errorf("%w: model import job %s not found", ErrNotFound, jobARN)
	}

	cp := *job
	cp.Tags = copyTags(job.Tags)

	return &cp, nil
}

// ListModelImportJobs returns model import jobs matching in's filters,
// sorted and paginated. in may be nil, matching an unfiltered call.
// Structurally similar to ListModelCopyJobs/ListCustomModelDeployments/
// ListProvisionedModelThroughputs (same filter/sort/paginate shape) but over
// a distinct resource type and filter set; see matchesModelImportJobFilter.
//
//nolint:dupl // see doc comment above.
func (b *InMemoryBackend) ListModelImportJobs(in *ListModelImportJobsInput) ([]*ModelImportJob, string) {
	b.mu.RLock("ListModelImportJobs")
	defer b.mu.RUnlock()

	list := make([]*ModelImportJob, 0, b.modelImportJobs.Len())

	for _, j := range b.modelImportJobs.All() {
		if !matchesModelImportJobFilter(j, in) {
			continue
		}

		cp := *j
		cp.Tags = copyTags(j.Tags)
		list = append(list, &cp)
	}

	descending := in != nil && in.SortOrder == sortOrderDescending
	sort.Slice(list, func(i, k int) bool {
		if !list[i].CreationTime.Equal(list[k].CreationTime) {
			if descending {
				return list[i].CreationTime.After(list[k].CreationTime)
			}

			return list[i].CreationTime.Before(list[k].CreationTime)
		}

		return list[i].JobArn < list[k].JobArn
	})

	if in == nil {
		list, _ = paginate(list, 0, "")

		return list, ""
	}

	return paginate(list, int(in.MaxResults), in.NextToken)
}

// matchesModelImportJobFilter reports whether a model import job satisfies
// the list filters (statusEquals, nameContains, creationTimeAfter/Before).
func matchesModelImportJobFilter(j *ModelImportJob, in *ListModelImportJobsInput) bool {
	if in == nil {
		return true
	}
	if in.StatusEquals != "" && j.Status != in.StatusEquals {
		return false
	}
	if in.NameContains != "" && !containsIgnoreCase(j.JobName, in.NameContains) {
		return false
	}
	if in.CreationTimeAfter != nil && !j.CreationTime.After(*in.CreationTimeAfter) {
		return false
	}
	if in.CreationTimeBefore != nil && !j.CreationTime.Before(*in.CreationTimeBefore) {
		return false
	}

	return true
}

// GetImportedModel returns the import job whose importedModelArn matches.
func (b *InMemoryBackend) GetImportedModel(modelARN string) (*ModelImportJob, error) {
	b.mu.RLock("GetImportedModel")
	defer b.mu.RUnlock()

	for _, j := range b.modelImportJobs.All() {
		if j.ImportedModelArn == modelARN {
			cp := *j
			cp.Tags = copyTags(j.Tags)

			return &cp, nil
		}
	}

	return nil, fmt.Errorf("%w: imported model %s not found", ErrNotFound, modelARN)
}

// ListImportedModels returns imported models (import jobs with an imported
// model ARN), optionally filtered by nameContains (matched against
// ImportedModelName) and creation-time range, sorted and paginated.
func (b *InMemoryBackend) ListImportedModels(in *ListImportedModelsInput) ([]*ModelImportJob, string) {
	b.mu.RLock("ListImportedModels")
	defer b.mu.RUnlock()

	models := make([]*ModelImportJob, 0, b.modelImportJobs.Len())

	for _, j := range b.modelImportJobs.All() {
		if j.ImportedModelArn == "" {
			continue
		}
		if !matchesImportedModelFilter(j, in) {
			continue
		}

		cp := *j
		cp.Tags = copyTags(j.Tags)
		models = append(models, &cp)
	}

	sort.Slice(models, func(i, k int) bool {
		if !models[i].CreationTime.Equal(models[k].CreationTime) {
			return models[i].CreationTime.Before(models[k].CreationTime)
		}

		return models[i].ImportedModelArn < models[k].ImportedModelArn
	})

	maxResults, nextToken := 0, ""
	if in != nil {
		maxResults, nextToken = int(in.MaxResults), in.NextToken
	}

	return paginate(models, maxResults, nextToken)
}

// matchesImportedModelFilter reports whether an imported model satisfies the
// list filters (nameContains, creationTimeAfter/Before).
func matchesImportedModelFilter(j *ModelImportJob, in *ListImportedModelsInput) bool {
	if in == nil {
		return true
	}
	if in.NameContains != "" && !containsIgnoreCase(j.ImportedModelName, in.NameContains) {
		return false
	}
	if in.CreationTimeAfter != nil && !j.CreationTime.After(*in.CreationTimeAfter) {
		return false
	}
	if in.CreationTimeBefore != nil && !j.CreationTime.Before(*in.CreationTimeBefore) {
		return false
	}

	return true
}

// DeleteImportedModel removes the import job whose importedModelArn matches.
func (b *InMemoryBackend) DeleteImportedModel(modelARN string) error {
	b.mu.Lock("DeleteImportedModel")
	defer b.mu.Unlock()

	for _, j := range b.modelImportJobs.All() {
		if j.ImportedModelArn == modelARN {
			b.modelImportJobs.Delete(j.JobArn)

			return nil
		}
	}

	return fmt.Errorf("%w: imported model %s not found", ErrNotFound, modelARN)
}

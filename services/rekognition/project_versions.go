package rekognition

import (
	"fmt"
	"maps"
	"sort"
	"time"
)

func (b *InMemoryBackend) projectVersionARN(projectARN, versionName string) string {
	return fmt.Sprintf("%s/version/%s", projectARN, versionName)
}

// CreateProjectVersion creates a new model version within a project. params
// carries CreateProjectVersionInput's fields beyond ProjectArn/VersionName/
// Tags (OutputConfig/KmsKeyId/VersionDescription) -- all stored verbatim and
// returned unchanged by DescribeProjectVersions. tags are applied the same
// way CreateStreamProcessor applies its initial tags (b.tags keyed by ARN);
// ProjectVersion ARNs are taggable per TagResource's doc (see resourceExists
// in tags.go).
func (b *InMemoryBackend) CreateProjectVersion(
	projectARN, versionName string,
	params CreateProjectVersionParams,
	tags map[string]string,
) (*ProjectVersion, error) {
	if err := validateTags(tags); err != nil {
		return nil, err
	}

	b.mu.Lock("CreateProjectVersion")
	defer b.mu.Unlock()

	if !b.projects.Has(projectARN) {
		return nil, ErrProjectNotFound
	}

	arn := b.projectVersionARN(projectARN, versionName)

	if b.projectVersions.Has(arn) {
		return nil, ErrProjectVersionAlreadyExists
	}

	tagsCopy := make(map[string]string, len(tags))
	maps.Copy(tagsCopy, tags)

	v := &storedProjectVersion{
		CreationTimestamp:                       time.Now(),
		ProjectVersionARN:                       arn,
		ProjectARN:                              projectARN,
		VersionName:                             versionName,
		Status:                                  "TRAINING_IN_PROGRESS",
		Tags:                                    tagsCopy,
		OutputConfigS3Bucket:                    params.OutputConfigS3Bucket,
		OutputConfigS3KeyPrefix:                 params.OutputConfigS3KeyPrefix,
		KmsKeyID:                                params.KmsKeyID,
		VersionDescription:                      params.VersionDescription,
		FeatureConfigContentModConfidenceThresh: params.FeatureConfigContentModConfidenceThresh,
	}
	b.projectVersions.Put(v)

	if len(tagsCopy) > 0 {
		b.tags[arn] = tagsCopy
	}

	return v.toProjectVersion(), nil
}

// DeleteProjectVersion deletes a project version. DeleteProjectVersionInput's
// own doc comment (api_op_DeleteProjectVersion.go): "You can't delete a
// project version if it is running or if it is training.".
func (b *InMemoryBackend) DeleteProjectVersion(projectVersionARN string) error {
	b.mu.Lock("DeleteProjectVersion")
	defer b.mu.Unlock()

	v, exists := b.projectVersions.Get(projectVersionARN)
	if !exists {
		return ErrProjectVersionNotFound
	}

	if v.Status == "TRAINING_IN_PROGRESS" || v.Status == processorRunning {
		return ErrProjectVersionInUse
	}

	b.projectVersions.Delete(projectVersionARN)

	return nil
}

// DescribeProjectVersions lists versions for a project, optionally filtered by version names.
func (b *InMemoryBackend) DescribeProjectVersions(
	projectARN string, versionNames []string, maxResults int32, nextToken string,
) ([]*ProjectVersion, string, error) {
	b.mu.RLock("DescribeProjectVersions")
	defer b.mu.RUnlock()

	// Collect and sort version ARN keys that belong to this project.
	keys := make([]string, 0)
	for _, v := range b.projectVersions.All() {
		if v.ProjectARN == projectARN {
			keys = append(keys, v.ProjectVersionARN)
		}
	}
	sort.Strings(keys)

	// Build a filter set if requested.
	filter := make(map[string]bool, len(versionNames))
	for _, name := range versionNames {
		filter[name] = true
	}

	start := 0
	if nextToken != "" {
		for i, k := range keys {
			if k == nextToken {
				start = i

				break
			}
		}
	}

	const maxPerPage = 100
	limit := int32(maxPerPage)
	if maxResults > 0 && maxResults < limit {
		limit = maxResults
	}

	var result []*ProjectVersion
	var outToken string
	count := int32(0)

	for i := start; i < len(keys); i++ {
		k := keys[i]
		v, _ := b.projectVersions.Get(k)

		if len(filter) > 0 && !filter[v.VersionName] {
			continue
		}

		if count >= limit {
			outToken = k

			break
		}

		result = append(result, v.toProjectVersion())
		count++
	}

	return result, outToken, nil
}

// CopyProjectVersion copies a project version to another project. The source
// version must belong to params.SourceProjectARN -- AWS reports a mismatch
// the same way it reports any other missing source, ResourceNotFoundException
// (verified against CopyProjectVersion's deserializeOpError switch, which
// declares ResourceNotFoundException but no ValidationException).
func (b *InMemoryBackend) CopyProjectVersion(
	sourceProjectVersionARN, destinationProjectARN, versionName string,
	params CopyProjectVersionParams,
) (*ProjectVersion, error) {
	b.mu.Lock("CopyProjectVersion")
	defer b.mu.Unlock()

	src, exists := b.projectVersions.Get(sourceProjectVersionARN)
	if !exists || src.ProjectARN != params.SourceProjectARN {
		return nil, ErrProjectVersionNotFound
	}

	if !b.projects.Has(destinationProjectARN) {
		return nil, ErrProjectNotFound
	}

	name := versionName
	if name == "" {
		name = src.VersionName
	}

	newARN := b.projectVersionARN(destinationProjectARN, name)

	v := &storedProjectVersion{
		CreationTimestamp:       time.Now(),
		ProjectVersionARN:       newARN,
		ProjectARN:              destinationProjectARN,
		VersionName:             name,
		Status:                  "COPYING_IN_PROGRESS",
		SourceProjectVersionARN: sourceProjectVersionARN,
		OutputConfigS3Bucket:    params.OutputConfigS3Bucket,
		OutputConfigS3KeyPrefix: params.OutputConfigS3KeyPrefix,
	}
	b.projectVersions.Put(v)

	return v.toProjectVersion(), nil
}

// StartProjectVersion sets a project version status to RUNNING.
func (b *InMemoryBackend) StartProjectVersion(
	projectVersionARN string, minInferenceUnits, maxInferenceUnits int32,
) error {
	b.mu.Lock("StartProjectVersion")
	defer b.mu.Unlock()

	v, exists := b.projectVersions.Get(projectVersionARN)
	if !exists {
		return ErrProjectVersionNotFound
	}

	v.Status = processorRunning
	v.MinInferenceUnits = minInferenceUnits
	v.MaxInferenceUnits = maxInferenceUnits

	return nil
}

// StopProjectVersion sets a project version status to STOPPED.
func (b *InMemoryBackend) StopProjectVersion(projectVersionARN string) error {
	b.mu.Lock("StopProjectVersion")
	defer b.mu.Unlock()

	v, exists := b.projectVersions.Get(projectVersionARN)
	if !exists {
		return ErrProjectVersionNotFound
	}

	v.Status = processorStopped

	return nil
}

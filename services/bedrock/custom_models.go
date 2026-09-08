package bedrock

import (
	"fmt"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// newCustomModelID generates a unique custom model ID.
func (b *InMemoryBackend) newCustomModelID() string {
	b.customModelCounter++

	return fmt.Sprintf("cm-%07d", b.customModelCounter)
}

// CreateCustomModel creates a new custom model with no base model: the wire
// input carries only a data source (S3/SageMaker model package), never a
// base model reference, and gopherstack doesn't process model artifacts to
// derive one. BaseModelArn/BaseModelName stay empty, so baseModelArnEquals/
// foundationModelArnEquals never match imported models -- matches real AWS
// (bedrock@v1.66.4 api_op_CreateCustomModel.go: "The model appears in
// ListCustomModels with a customizationType of imported").
func (b *InMemoryBackend) CreateCustomModel(modelName string, tags []Tag) (*CustomModel, error) {
	b.mu.Lock("CreateCustomModel")
	defer b.mu.Unlock()

	if modelName == "" {
		return nil, fmt.Errorf("%w: modelName is required", ErrValidation)
	}

	if _, exists := b.customModelsByName[modelName]; exists {
		return nil, fmt.Errorf("%w: custom model %s already exists", ErrAlreadyExists, modelName)
	}

	id := b.newCustomModelID()
	modelARN := arn.Build("bedrock", b.region, b.accountID, "custom-model/"+id)

	model := &CustomModel{
		ModelArn:          modelARN,
		ModelName:         modelName,
		ModelStatus:       statusActive,
		CustomizationType: customizationTypeImported,
		CreationTime:      time.Now().UTC(),
		Tags:              copyTags(tags),
	}
	b.customModels.Put(model)
	b.customModelsByName[modelName] = modelARN
	cp := *model
	cp.Tags = copyTags(model.Tags)

	return &cp, nil
}

// findCustomModelARN resolves a custom model ID or name to its ARN.
// Caller must hold at least a read lock.
func (b *InMemoryBackend) findCustomModelARN(idOrARN string) (string, bool) {
	if _, ok := b.customModels.Get(idOrARN); ok {
		return idOrARN, true
	}

	if a := b.customModelsByName[idOrARN]; a != "" {
		return a, true
	}

	return "", false
}

// resolveCustomModelARN resolves to ARN or returns ErrNotFound.
func (b *InMemoryBackend) resolveCustomModelARN(idOrARN string) (string, error) {
	if a, ok := b.findCustomModelARN(idOrARN); ok {
		return a, nil
	}

	return "", fmt.Errorf("%w: custom model %s not found", ErrNotFound, idOrARN)
}

// GetCustomModel returns a custom model by ARN or name.
func (b *InMemoryBackend) GetCustomModel(idOrARN string) (*CustomModel, error) {
	b.mu.RLock("GetCustomModel")
	defer b.mu.RUnlock()

	modelARN, err := b.resolveCustomModelARN(idOrARN)
	if err != nil {
		return nil, err
	}

	m, _ := b.customModels.Get(modelARN)
	cp := *m
	cp.Tags = copyTags(m.Tags)

	return &cp, nil
}

// ListCustomModels returns custom models matching in's filters, sorted and
// paginated. in may be nil, matching an unfiltered call. Structurally similar
// to ListEvaluationJobs/ListModelInvocationJobs (same filter/sort/paginate
// shape) but over a distinct resource type and filter set; see
// matchesCustomModelFilter. baseModelArnEquals/foundationModelArnEquals only
// match models produced by a completed CreateModelCustomizationJob -- those
// carry a real BaseModelArn from the job's baseModelIdentifier. Imported
// models (CreateCustomModel) have no base model and match neither filter.
//
//nolint:dupl // see doc comment above.
func (b *InMemoryBackend) ListCustomModels(in *ListCustomModelsInput) ([]*CustomModel, string) {
	b.mu.RLock("ListCustomModels")
	defer b.mu.RUnlock()

	list := make([]*CustomModel, 0, b.customModels.Len())

	for _, m := range b.customModels.All() {
		if !matchesCustomModelFilter(m, in) {
			continue
		}

		cp := *m
		cp.Tags = copyTags(m.Tags)
		list = append(list, &cp)
	}

	descending := in != nil && in.SortOrder == sortOrderDescending
	sort.Slice(list, func(i, j int) bool {
		if !list[i].CreationTime.Equal(list[j].CreationTime) {
			if descending {
				return list[i].CreationTime.After(list[j].CreationTime)
			}

			return list[i].CreationTime.Before(list[j].CreationTime)
		}

		return list[i].ModelArn < list[j].ModelArn
	})

	maxResults, nextToken := 0, ""
	if in != nil {
		maxResults, nextToken = int(in.MaxResults), in.NextToken
	}

	return paginate(list, maxResults, nextToken)
}

// matchesCustomModelFilter reports whether a custom model satisfies the list
// filters (modelStatus, nameContains, baseModelArnEquals/
// foundationModelArnEquals, isOwned, creationTimeAfter/Before).
func matchesCustomModelFilter(m *CustomModel, in *ListCustomModelsInput) bool {
	if in == nil {
		return true
	}
	if in.ModelStatus != "" && m.ModelStatus != in.ModelStatus {
		return false
	}
	if in.NameContains != "" && !containsIgnoreCase(m.ModelName, in.NameContains) {
		return false
	}
	if !matchesBaseModelFilter(m, in) {
		return false
	}
	// Single-account emulator: every custom model is owned by this account.
	if in.IsOwned != nil && !*in.IsOwned {
		return false
	}
	if in.CreationTimeAfter != nil && !m.CreationTime.After(*in.CreationTimeAfter) {
		return false
	}
	if in.CreationTimeBefore != nil && !m.CreationTime.Before(*in.CreationTimeBefore) {
		return false
	}

	return true
}

// matchesBaseModelFilter checks baseModelArnEquals/foundationModelArnEquals.
// gopherstack only ever derives BaseModelArn from a foundation model (see
// CreateModelCustomizationJob), so the two filters coincide here; real AWS's
// baseModelArnEquals can also match a base that is itself a custom model
// (chained fine-tuning).
func matchesBaseModelFilter(m *CustomModel, in *ListCustomModelsInput) bool {
	if in.BaseModelArnEquals != "" && m.BaseModelArn != in.BaseModelArnEquals {
		return false
	}

	return in.FoundationModelArnEquals == "" || m.BaseModelArn == in.FoundationModelArnEquals
}

// DeleteCustomModel removes a custom model by ARN or name.
func (b *InMemoryBackend) DeleteCustomModel(idOrARN string) error {
	b.mu.Lock("DeleteCustomModel")
	defer b.mu.Unlock()

	modelARN, err := b.resolveCustomModelARN(idOrARN)
	if err != nil {
		return err
	}

	m, _ := b.customModels.Get(modelARN)
	delete(b.customModelsByName, m.ModelName)
	b.customModels.Delete(modelARN)

	return nil
}

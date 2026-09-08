package transcribe

import (
	"fmt"
	"slices"
	"sort"
	"time"
)

// supportedBaseModelNames returns the set of base model names for custom language models.
func supportedBaseModelNames() []string { return []string{"NarrowBand", "WideBand"} }

// validateBaseModelName checks that a CLM base model name is valid.
func validateBaseModelName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: BaseModelName is required", ErrValidation)
	}

	if !slices.Contains(supportedBaseModelNames(), name) {
		return fmt.Errorf("%w: BaseModelName %q must be one of %v",
			ErrValidation, name, supportedBaseModelNames())
	}

	return nil
}

// CreateLanguageModel creates a new custom language model.
func (b *InMemoryBackend) CreateLanguageModel(input *LanguageModel) (*LanguageModel, error) {
	if input.ModelName == "" {
		return nil, fmt.Errorf("%w: ModelName is required", ErrValidation)
	}

	if err := validateBaseModelName(input.BaseModelName); err != nil {
		return nil, err
	}

	if err := validateLanguageCode(input.LanguageCode); err != nil {
		return nil, err
	}

	if input.LanguageCode == "" {
		return nil, fmt.Errorf("%w: LanguageCode is required", ErrValidation)
	}

	if input.InputDataConfig == nil {
		return nil, fmt.Errorf("%w: InputDataConfig is required", ErrValidation)
	}

	if input.InputDataConfig.S3Uri == "" {
		return nil, fmt.Errorf("%w: InputDataConfig.S3Uri is required", ErrValidation)
	}

	if input.InputDataConfig.DataAccessRoleArn == "" {
		return nil, fmt.Errorf("%w: InputDataConfig.DataAccessRoleArn is required", ErrValidation)
	}

	b.mu.Lock("CreateLanguageModel")
	defer b.mu.Unlock()

	if b.languageModels.Has(input.ModelName) {
		return nil, fmt.Errorf("%w: language model %s already exists", ErrAlreadyExists, input.ModelName)
	}

	now := time.Now()
	m := *input
	m.ModelStatus = modelStatusCompleted
	m.CreateTime = now
	m.LastModifiedTime = now
	b.languageModels.Put(&m)
	b.recordResourceTagsLocked(resourceARN(resourceTypeLanguageModel, m.ModelName), m.Tags)

	cp := m

	return &cp, nil
}

// DeleteLanguageModel removes a custom language model by name.
func (b *InMemoryBackend) DeleteLanguageModel(modelName string) error {
	if modelName == "" {
		return fmt.Errorf("%w: ModelName is required", ErrValidation)
	}

	b.mu.Lock("DeleteLanguageModel")
	defer b.mu.Unlock()

	if !b.languageModels.Delete(modelName) {
		return fmt.Errorf("%w: language model %s not found", ErrNotFound, modelName)
	}

	b.forgetResourceTagsLocked(resourceARN(resourceTypeLanguageModel, modelName))

	return nil
}

// AddLanguageModelInternal seeds a language model directly (test helper).
func (b *InMemoryBackend) AddLanguageModelInternal(m *LanguageModel) {
	b.mu.Lock("AddLanguageModelInternal")
	defer b.mu.Unlock()

	cp := *m
	b.languageModels.Put(&cp)
}

// DescribeLanguageModel returns a language model by name.
func (b *InMemoryBackend) DescribeLanguageModel(modelName string) (*LanguageModel, error) {
	b.mu.RLock("DescribeLanguageModel")
	defer b.mu.RUnlock()

	m, ok := b.languageModels.Get(modelName)
	if !ok {
		return nil, fmt.Errorf("%w: language model %s not found", ErrNotFound, modelName)
	}

	cp := *m

	return &cp, nil
}

// ListLanguageModels returns language models with optional status filter, name
// substring filter, and pagination.
func (b *InMemoryBackend) ListLanguageModels(
	statusFilter, nameContains, nextToken string, maxResults int32,
) ([]LanguageModel, string) {
	b.mu.RLock("ListLanguageModels")
	defer b.mu.RUnlock()

	all := make([]LanguageModel, 0, b.languageModels.Len())
	for _, m := range b.languageModels.All() {
		if (statusFilter == "" || m.ModelStatus == statusFilter) && matchesNameContains(m.ModelName, nameContains) {
			all = append(all, *m)
		}
	}

	sort.Slice(all, func(i, j int) bool { return all[i].ModelName < all[j].ModelName })

	return paginateList(all, nextToken, maxResults)
}

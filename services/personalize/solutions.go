package personalize

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/awstime"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
)

// --- Solution ---

// CreateSolution creates a new solution. performAutoTraining defaults to true
// in the real API when the caller omits it, so it is passed as an already
// caller-resolved bool rather than re-defaulted here.
func (b *InMemoryBackend) CreateSolution(
	name, datasetGroupArn, recipeArn, eventType string,
	performAutoML, performHPO, performAutoTraining, performIncrementalUpdate bool,
	solutionConfig *SolutionConfig,
	tags map[string]string,
) (*Solution, error) {
	b.mu.Lock("CreateSolution")
	defer b.mu.Unlock()

	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrValidation)
	}
	if b.solutions.Has(name) {
		return nil, fmt.Errorf("%w: solution %q already exists", ErrAlreadyExists, name)
	}
	if b.findDatasetGroup(datasetGroupArn) == nil {
		return nil, fmt.Errorf("%w: dataset group %q not found", ErrNotFound, datasetGroupArn)
	}
	// recipeArn is required only when performAutoML is false; when omitted
	// (the AutoML path) there is nothing to validate.
	if recipeArn != "" && !recipeExists(recipeArn) {
		return nil, fmt.Errorf("%w: recipe %q not found", ErrNotFound, recipeArn)
	}

	now := time.Now().UTC()
	sol := &Solution{
		SolutionArn:              b.personalizeARN("solution", name),
		Name:                     name,
		DatasetGroupArn:          datasetGroupArn,
		RecipeArn:                recipeArn,
		EventType:                eventType,
		PerformAutoML:            performAutoML,
		PerformHPO:               performHPO,
		PerformAutoTraining:      performAutoTraining,
		PerformIncrementalUpdate: performIncrementalUpdate,
		SolutionConfig:           solutionConfig,
		Status:                   statusActive,
		CreationDateTime:         now,
		LastUpdatedDateTime:      now,
	}
	if performAutoML {
		// AutoMLResult.BestRecipeArn is only meaningful when AutoML actually
		// ran; this deterministic mock always "selects" USER_PERSONALIZATION
		// (matches the recipe getBuiltinRecipes always returns as
		// candidate #1), since there is no real training loop to search.
		sol.AutoMLResult = map[string]any{
			"bestRecipeArn": "arn:aws:personalize:::recipe/aws-user-personalization",
		}
	}
	b.solutions.Put(sol)
	if len(tags) > 0 {
		b.tags[sol.SolutionArn] = copyStringMap(tags)
	}

	return sol, nil
}

// DescribeSolution returns a solution by name or ARN.
func (b *InMemoryBackend) DescribeSolution(nameOrArn string) (*Solution, error) {
	b.mu.RLock("DescribeSolution")
	defer b.mu.RUnlock()

	if sol := b.findSolution(nameOrArn); sol != nil {
		return sol, nil
	}

	return nil, fmt.Errorf("%w: solution %q not found", ErrNotFound, nameOrArn)
}

// UpdateSolution updates a solution's automatic-training configuration and,
// via solutionUpdateConfig, the AutoTrainingConfig/EventsConfig subset of
// its SolutionConfig (types.UpdateSolutionInput.SolutionUpdateConfig,
// api_op_UpdateSolution.go) -- performAutoML/performHPO and every other
// SolutionConfig member remain immutable, creation-only fields. nil means
// "not specified in the request", leaving the current value untouched,
// matching the optional *bool/*SolutionUpdateConfig request members.
func (b *InMemoryBackend) UpdateSolution(
	nameOrArn string,
	performAutoTraining, performIncrementalUpdate *bool,
	solutionUpdateConfig *SolutionUpdateConfig,
) (*Solution, error) {
	b.mu.Lock("UpdateSolution")
	defer b.mu.Unlock()

	sol := b.findSolution(nameOrArn)
	if sol == nil {
		return nil, fmt.Errorf("%w: solution %q not found", ErrNotFound, nameOrArn)
	}
	if performAutoTraining != nil {
		sol.PerformAutoTraining = *performAutoTraining
	}
	if performIncrementalUpdate != nil {
		sol.PerformIncrementalUpdate = *performIncrementalUpdate
	}
	if solutionUpdateConfig != nil {
		if sol.SolutionConfig == nil {
			sol.SolutionConfig = &SolutionConfig{}
		}
		if solutionUpdateConfig.AutoTrainingConfig != nil {
			sol.SolutionConfig.AutoTrainingConfig = solutionUpdateConfig.AutoTrainingConfig
		}
		if solutionUpdateConfig.EventsConfig != nil {
			sol.SolutionConfig.EventsConfig = solutionUpdateConfig.EventsConfig
		}
	}
	sol.LastUpdatedDateTime = time.Now().UTC()
	sol.LatestSolutionUpdate = map[string]any{
		keyCreationDateTime:         awstime.Epoch(sol.LastUpdatedDateTime),
		keyLastUpdatedDateTime:      awstime.Epoch(sol.LastUpdatedDateTime),
		"performAutoTraining":       sol.PerformAutoTraining,
		keyPerformIncrementalUpdate: sol.PerformIncrementalUpdate,
		"solutionUpdateConfig":      solutionUpdateConfig,
		keyStatus:                   sol.Status,
	}

	return sol, nil
}

// DeleteSolution removes a solution and all its versions. Per
// api_op_DeleteSolution.go's doc comment, the caller must first delete all
// campaigns based on the solution.
func (b *InMemoryBackend) DeleteSolution(nameOrArn string) error {
	b.mu.Lock("DeleteSolution")
	defer b.mu.Unlock()

	sol := b.findSolution(nameOrArn)
	if sol == nil {
		return fmt.Errorf("%w: solution %q not found", ErrNotFound, nameOrArn)
	}
	for _, c := range b.campaigns.All() {
		if strings.HasPrefix(c.SolutionVersionArn, sol.SolutionArn+"/") {
			return fmt.Errorf("%w: solution %q still has campaigns", ErrInUse, nameOrArn)
		}
	}
	for _, sv := range b.solutionVersions.All() {
		if sv.SolutionArn == sol.SolutionArn {
			b.solutionVersions.Delete(sv.SolutionVersionArn)
			delete(b.tags, sv.SolutionVersionArn)
		}
	}
	b.solutions.Delete(sol.Name)
	delete(b.tags, sol.SolutionArn)

	return nil
}

// ListSolutions returns solutions, optionally filtered by dataset group ARN.
func (b *InMemoryBackend) ListSolutions(
	datasetGroupArn string,
	maxResults int,
	nextToken string,
) ([]*Solution, string) {
	b.mu.RLock("ListSolutions")
	defer b.mu.RUnlock()

	all := b.solutions.Snapshot()
	filtered := make([]*Solution, 0, len(all))
	for _, sol := range all {
		if datasetGroupArn == "" || sol.DatasetGroupArn == datasetGroupArn {
			filtered = append(filtered, sol)
		}
	}

	return paginateItems(filtered, solutionKeyFn, maxResults, nextToken)
}

func (b *InMemoryBackend) findSolution(nameOrArn string) *Solution {
	if sol, ok := b.solutions.Get(nameOrArn); ok {
		return sol
	}
	for _, sol := range b.solutions.All() {
		if sol.SolutionArn == nameOrArn {
			return sol
		}
	}

	return nil
}

// --- SolutionVersion ---

// CreateSolutionVersion creates a new solution version. name is the real,
// optional CreateSolutionVersionInput.Name member (api_op_CreateSolutionVersion.go)
// -- present only on the full SolutionVersion shape, not
// SolutionVersionSummary (types.go:2164 declares no Name member).
func (b *InMemoryBackend) CreateSolutionVersion(
	solutionArn, trainingMode, name string,
	tags map[string]string,
) (*SolutionVersion, error) {
	b.mu.Lock("CreateSolutionVersion")
	defer b.mu.Unlock()

	if solutionArn == "" {
		return nil, fmt.Errorf("%w: solutionArn is required", ErrValidation)
	}
	sol := b.findSolution(solutionArn)
	if sol == nil {
		return nil, fmt.Errorf("%w: solution %q not found", ErrNotFound, solutionArn)
	}

	versionID := uuid.New().String()
	now := time.Now().UTC()
	sv := &SolutionVersion{
		SolutionVersionArn: sol.SolutionArn + "/" + versionID,
		SolutionArn:        sol.SolutionArn,
		Status:             statusActive,
		TrainingMode:       trainingMode,
		Name:               name,
		TrainingHours:      mockMetricValue,
		// SolutionConfig and the fields below reflect the parent solution's
		// state at training time (the real API has no per-version override on
		// CreateSolutionVersion) -- these are value copies, so a later
		// UpdateSolution call cannot retroactively change them.
		SolutionConfig:           sol.SolutionConfig,
		DatasetGroupArn:          sol.DatasetGroupArn,
		EventType:                sol.EventType,
		RecipeArn:                sol.RecipeArn,
		PerformAutoML:            sol.PerformAutoML,
		PerformHPO:               sol.PerformHPO,
		PerformIncrementalUpdate: sol.PerformIncrementalUpdate,
		CreationDateTime:         now,
		LastUpdatedDateTime:      now,
	}
	b.solutionVersions.Put(sv)
	if len(tags) > 0 {
		b.tags[sv.SolutionVersionArn] = copyStringMap(tags)
	}

	return sv, nil
}

// DescribeSolutionVersion returns a solution version by ARN.
func (b *InMemoryBackend) DescribeSolutionVersion(svArn string) (*SolutionVersion, error) {
	b.mu.RLock("DescribeSolutionVersion")
	defer b.mu.RUnlock()

	sv, ok := b.solutionVersions.Get(svArn)
	if !ok {
		return nil, fmt.Errorf("%w: solution version %q not found", ErrNotFound, svArn)
	}

	return sv, nil
}

// ListSolutionVersions returns solution versions for a given solution ARN.
func (b *InMemoryBackend) ListSolutionVersions(
	solutionArn string,
	maxResults int,
	nextToken string,
) ([]*SolutionVersion, string) {
	b.mu.RLock("ListSolutionVersions")
	defer b.mu.RUnlock()

	all := b.solutionVersions.Snapshot()
	filtered := make([]*SolutionVersion, 0, len(all))
	for _, sv := range all {
		if solutionArn == "" || sv.SolutionArn == solutionArn {
			filtered = append(filtered, sv)
		}
	}

	return paginateItems(filtered, solutionVersionKeyFn, maxResults, nextToken)
}

// LatestSolutionVersion returns the most recently created solution version
// for solutionArn, or nil if none exist yet. Populates
// Solution.latestSolutionVersion (types.SolutionVersionSummary,
// types.go:2164) on DescribeSolution -- confirmed against
// deserializers.go:15334, which maps the "latestSolutionVersion" key to that
// summary type, not the full SolutionVersion.
func (b *InMemoryBackend) LatestSolutionVersion(solutionArn string) *SolutionVersion {
	b.mu.RLock("LatestSolutionVersion")
	defer b.mu.RUnlock()

	return b.latestSolutionVersionLocked(solutionArn)
}

// latestSolutionVersionLocked is LatestSolutionVersion without its own
// locking, for callers that already hold b.mu (read or write).
func (b *InMemoryBackend) latestSolutionVersionLocked(solutionArn string) *SolutionVersion {
	var latest *SolutionVersion
	for _, sv := range b.solutionVersions.All() {
		if sv.SolutionArn != solutionArn {
			continue
		}
		if latest == nil || sv.CreationDateTime.After(latest.CreationDateTime) {
			latest = sv
		}
	}

	return latest
}

// StopSolutionVersionCreation transitions a solution version to CREATE STOPPED.
func (b *InMemoryBackend) StopSolutionVersionCreation(svArn string) error {
	b.mu.Lock("StopSolutionVersionCreation")
	defer b.mu.Unlock()

	sv, ok := b.solutionVersions.Get(svArn)
	if !ok {
		return fmt.Errorf("%w: solution version %q not found", ErrNotFound, svArn)
	}
	sv.Status = statusSolutionVersionStopped
	sv.LastUpdatedDateTime = time.Now().UTC()

	return nil
}

// GetSolutionMetrics returns deterministic accuracy metrics for a solution version.
// Values are derived from the ARN hash so each solution version gets distinct (but stable) metrics.
func (b *InMemoryBackend) GetSolutionMetrics(svArn string) (map[string]any, error) {
	b.mu.RLock("GetSolutionMetrics")
	defer b.mu.RUnlock()

	if !b.solutionVersions.Has(svArn) {
		return nil, fmt.Errorf("%w: solution version %q not found", ErrNotFound, svArn)
	}

	return map[string]any{
		keySolutionVersionArn: svArn,
		"metrics": map[string]any{
			"coverage":                   svMetric(svArn, "coverage"),
			"mean_reciprocal_rank_at_25": svMetric(svArn, "mrr@25"),
			"normalized_discounted_cumulative_gain_at_5":  svMetric(svArn, "ndcg@5"),
			"normalized_discounted_cumulative_gain_at_10": svMetric(svArn, "ndcg@10"),
			"normalized_discounted_cumulative_gain_at_25": svMetric(svArn, "ndcg@25"),
			"precision_at_5":  svMetric(svArn, "p@5"),
			"precision_at_10": svMetric(svArn, "p@10"),
			"precision_at_25": svMetric(svArn, "p@25"),
		},
	}, nil
}

// svMetric returns a stable [0.01, 0.99] metric value derived from the solution version ARN and metric name.
func svMetric(svArn, metricName string) float64 {
	const buckets = 98 // maps hash into [0.01, 0.99]
	h := httputils.FNV32a(svArn + "|" + metricName)

	return float64(h%buckets+1) / 100.0 //nolint:mnd // 100.0 converts integer percent to float ratio
}

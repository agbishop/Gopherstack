package databrew

import (
	"context"
	"fmt"
	"maps"
	"regexp"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

const (
	// recipeVersionLatestWorking is the literal RecipeVersion identifier AWS
	// uses for a recipe's current (unpublished) draft -- it is never a
	// numeric value. See aws-sdk-go-v2/service/databrew/types.Recipe's
	// RecipeVersion doc comment.
	recipeVersionLatestWorking = "LATEST_WORKING"
	// recipeVersionLatestPublished is the RecipeVersion identifier meaning
	// "the most recently published numeric version". Unlike LATEST_WORKING
	// it is never stored on a Recipe value; it only appears as a caller-
	// supplied filter/lookup key.
	recipeVersionLatestPublished = "LATEST_PUBLISHED"

	// maxBatchDeleteRecipeVersions is the documented limit on
	// BatchDeleteRecipeVersionInput.RecipeVersions
	// ("The version list size exceeds 50" is a whole-request rejection).
	maxBatchDeleteRecipeVersions = 50
)

// recipeNumericVersionRegex matches a published recipe's numeric "X.Y"
// version identifier (this backend only ever mints "N.0" values via
// PublishRecipe, but real AWS accepts any non-negative X.Y, so validation
// accepts the full documented shape).
var recipeNumericVersionRegex = regexp.MustCompile(`^[0-9]+\.[0-9]+$`)

// isValidRecipeVersionID reports whether v is a syntactically valid recipe
// version identifier for BatchDeleteRecipeVersion/DeleteRecipeVersion: a
// numeric "X.Y" version or the literal "LATEST_WORKING". LATEST_PUBLISHED is
// deliberately excluded -- both operations' doc comments in
// aws-sdk-go-v2/service/databrew state it "is not supported" as a deletion
// target.
func isValidRecipeVersionID(v string) bool {
	return v == recipeVersionLatestWorking || recipeNumericVersionRegex.MatchString(v)
}

func (b *InMemoryBackend) recipeARN(region, name string) string {
	return arn.Build("databrew", region, b.accountID, "recipe/"+name)
}

// copyRecipe returns a defensive copy of r, deep-copying its Tags/Steps so
// callers can't mutate backend state through the returned pointer.
func copyRecipe(r *Recipe) *Recipe {
	cp := *r
	cp.Tags = maps.Clone(r.Tags)
	cp.Steps = append([]RecipeStep(nil), r.Steps...)

	return &cp
}

// recipeProjectName returns the name of the project (if any) that
// currently uses name as its RecipeName. aws-sdk-go-v2/service/databrew's
// types.Recipe.ProjectName documents "The name of the project that the
// recipe is associated with," but this backend does not store that
// association on the recipe itself -- CreateProject already stores the
// reverse link (Project.RecipeName), so it is derived here by scanning
// projects for a match. If more than one project references the same
// recipe name, the first match in key order is returned (this backend does
// not enforce recipe-to-project uniqueness, and neither does the real
// service). Callers must hold at least b.mu.RLock.
func (b *InMemoryBackend) recipeProjectName(region, name string) string {
	t := b.projectsTable(region)
	for _, k := range snapshotKeys(t, projectKeyFn) {
		p, ok := t.Get(k)
		if ok && p.RecipeName == name {
			return p.Name
		}
	}

	return ""
}

// copyRecipeWithProject is copyRecipe plus ProjectName derivation. Callers
// must hold at least b.mu.RLock.
func (b *InMemoryBackend) copyRecipeWithProject(region string, r *Recipe) *Recipe {
	cp := copyRecipe(r)
	cp.ProjectName = b.recipeProjectName(region, r.Name)

	return cp
}

func (b *InMemoryBackend) CreateRecipe(
	ctx context.Context,
	name, description string,
	steps []RecipeStep,
	tags map[string]string,
) (*Recipe, error) {
	b.mu.Lock("CreateRecipe")
	defer b.mu.Unlock()
	region := getRegion(ctx, b.defaultRegion)
	if name == "" {
		return nil, ErrValidation
	}
	t := b.recipesTable(region)
	if t.Has(name) {
		return nil, ErrAlreadyExists
	}
	now := float64(time.Now().Unix())
	r := &Recipe{
		Name: name, Arn: b.recipeARN(region, name), Description: description,
		Steps: steps, Tags: maps.Clone(tags), RecipeVersion: recipeVersionLatestWorking,
		CreateDate: now, LastModifiedDate: now,
	}
	t.Put(r)

	return copyRecipe(r), nil
}

// DescribeRecipe returns the recipe version identified by version:
//   - "" or "LATEST_PUBLISHED": the most recently published numeric version.
//     If version is "" and the recipe has never been published (no numeric
//     version exists yet), the working draft is returned instead -- the real
//     API's documented default is "the latest published version", but
//     DescribeRecipe on a freshly created, unpublished recipe (no version
//     filter) is a common, supported call that must not 404. Passing
//     "LATEST_PUBLISHED" explicitly does 404 in that case, matching the
//     literal filter semantics.
//   - "LATEST_WORKING": the current (possibly unpublished) draft.
//   - a numeric "X.Y" version: that specific published snapshot.
func (b *InMemoryBackend) DescribeRecipe(ctx context.Context, name, version string) (*Recipe, error) {
	b.mu.RLock("DescribeRecipe")
	defer b.mu.RUnlock()
	region := getRegion(ctx, b.defaultRegion)
	working, ok := b.recipesTable(region).Get(name)
	if !ok {
		return nil, ErrNotFound
	}
	versions := b.recipeVersionsStoreRO(region)[name]

	switch version {
	case "", recipeVersionLatestPublished:
		if len(versions) > 0 {
			return b.copyRecipeWithProject(region, versions[len(versions)-1]), nil
		}
		if version == recipeVersionLatestPublished {
			return nil, ErrNotFound
		}

		return b.copyRecipeWithProject(region, working), nil
	case recipeVersionLatestWorking:
		return b.copyRecipeWithProject(region, working), nil
	default:
		for _, v := range versions {
			if v.RecipeVersion == version {
				return b.copyRecipeWithProject(region, v), nil
			}
		}

		return nil, ErrNotFound
	}
}

// ListRecipes lists recipes. versionFilter mirrors ListRecipesInput's
// RecipeVersion field: "LATEST_WORKING" returns every recipe's working
// draft; "" or "LATEST_PUBLISHED" (the real API's documented default)
// returns only recipes that have at least one published version, each
// represented by its most recent published snapshot -- a never-published
// recipe does not appear in the default listing (confirmed against
// aws-sdk-go-v2/service/databrew/api_op_ListRecipes.go's ListRecipesInput
// doc comment: "If RecipeVersion is omitted, ListRecipes returns all of the
// LATEST_PUBLISHED recipe versions.").
func (b *InMemoryBackend) ListRecipes(
	ctx context.Context,
	maxResults int,
	nextToken, versionFilter string,
) ([]*Recipe, string) {
	b.mu.RLock("ListRecipes")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.defaultRegion)
	t := b.recipesTable(region)
	versions := b.recipeVersionsStoreRO(region)
	allNames := snapshotKeys(t, recipeKeyFn)

	showWorking := versionFilter == recipeVersionLatestWorking
	keys := make([]string, 0, len(allNames))
	for _, name := range allNames {
		if showWorking || len(versions[name]) > 0 {
			keys = append(keys, name)
		}
	}

	pageKeys, next := paginateKeys(keys, maxResults, nextToken)
	out := make([]*Recipe, 0, len(pageKeys))
	for _, name := range pageKeys {
		if showWorking {
			v, _ := t.Get(name)
			out = append(out, b.copyRecipeWithProject(region, v))

			continue
		}
		vs := versions[name]
		out = append(out, b.copyRecipeWithProject(region, vs[len(vs)-1]))
	}

	return out, next
}

// ListRecipeVersions returns the published version history for name (never
// including LATEST_WORKING, matching the real op's doc comment: "Lists the
// versions of a particular DataBrew recipe, except for LATEST_WORKING").
// The returned slice is never nil (an unpublished recipe returns an empty,
// non-nil slice) so callers marshal "Recipes":[] rather than "Recipes":null.
//
// Deliberately does NOT reject an unknown name: ListRecipeVersions's own
// awsRestjson1_deserializeOpErrorListRecipeVersions switch (aws-sdk-go-v2/
// service/databrew@v1.42.4 deserializers.go) types only ValidationException
// -- ResourceNotFoundException is not modeled for this op at all, unlike
// DescribeRecipe/UpdateRecipe/etc. An unknown name falls through to the same
// empty-slice result as a never-published recipe.
func (b *InMemoryBackend) ListRecipeVersions(
	ctx context.Context,
	name string,
	maxResults int,
	nextToken string,
) ([]*Recipe, string, error) {
	b.mu.RLock("ListRecipeVersions")
	defer b.mu.RUnlock()
	region := getRegion(ctx, b.defaultRegion)

	// versions is stored in publish order (oldest first), which is also
	// numerically increasing (PublishRecipe always appends "len+1.0"), so no
	// separate sort is needed -- same pattern as jobRuns.
	versions := b.recipeVersionsStoreRO(region)[name]
	if maxResults <= 0 {
		maxResults = 100
	}
	startIdx := 0
	if nextToken != "" {
		startIdx = len(versions)
		for i, v := range versions {
			if v.RecipeVersion == nextToken {
				startIdx = i + 1

				break
			}
		}
	}
	endIdx := min(startIdx+maxResults, len(versions))

	var next string
	if endIdx < len(versions) {
		next = versions[endIdx-1].RecipeVersion
	}

	out := make([]*Recipe, 0, max(endIdx-startIdx, 0))
	if startIdx < len(versions) {
		for _, v := range versions[startIdx:endIdx] {
			out = append(out, b.copyRecipeWithProject(region, v))
		}
	}

	return out, next, nil
}

// PublishRecipe snapshots the current working draft as a new numbered
// published version ("N.0", where N is the count of prior published
// versions plus one -- this backend does not model minor-version publishes).
func (b *InMemoryBackend) PublishRecipe(ctx context.Context, name, description string) error {
	b.mu.Lock("PublishRecipe")
	defer b.mu.Unlock()
	region := getRegion(ctx, b.defaultRegion)
	working, ok := b.recipesTable(region).Get(name)
	if !ok {
		return ErrNotFound
	}
	if description != "" {
		working.Description = description
	}
	now := float64(time.Now().Unix())
	working.PublishedDate = now
	working.PublishedBy = "admin"
	working.LastModifiedDate = now

	versions := b.recipeVersionsStore(region)
	nextVersion := fmt.Sprintf("%d.0", len(versions[name])+1)
	snapshot := &Recipe{
		Name: name, Arn: working.Arn, Description: working.Description,
		Steps: append([]RecipeStep(nil), working.Steps...), Tags: maps.Clone(working.Tags),
		RecipeVersion:    nextVersion,
		CreatedBy:        working.CreatedBy,
		CreateDate:       working.CreateDate,
		LastModifiedBy:   working.LastModifiedBy,
		LastModifiedDate: now,
		PublishedBy:      "admin",
		PublishedDate:    now,
	}
	versions[name] = append(versions[name], snapshot)

	return nil
}

// UpdateRecipe modifies the definition of the LATEST_WORKING version only,
// matching aws-sdk-go-v2/service/databrew's UpdateRecipe doc comment;
// published version snapshots are immutable once created by PublishRecipe.
// Steps is only overwritten when non-empty: UpdateRecipeInput's Steps member
// has no "This member is required" marker (only Name does), so a caller
// updating just Description must not have their existing Steps clobbered.
func (b *InMemoryBackend) UpdateRecipe(
	ctx context.Context,
	name, description string,
	steps []RecipeStep,
) error {
	b.mu.Lock("UpdateRecipe")
	defer b.mu.Unlock()
	region := getRegion(ctx, b.defaultRegion)
	r, ok := b.recipesTable(region).Get(name)
	if !ok {
		return ErrNotFound
	}
	if description != "" {
		r.Description = description
	}
	if len(steps) > 0 {
		r.Steps = steps
	}
	r.LastModifiedDate = float64(time.Now().Unix())

	return nil
}

// DeleteRecipe deletes a recipe's working draft and cascades to its entire
// published version history, so no orphaned recipeVersions rows survive the
// recipe itself.
func (b *InMemoryBackend) DeleteRecipe(ctx context.Context, name string) error {
	b.mu.Lock("DeleteRecipe")
	defer b.mu.Unlock()
	region := getRegion(ctx, b.defaultRegion)
	if !b.recipesTable(region).Delete(name) {
		return ErrNotFound
	}
	delete(b.recipeVersionsStore(region), name)

	return nil
}

// DeleteRecipeVersion deletes a single recipe version. version must be a
// numeric "X.Y" identifier or "LATEST_WORKING" (LATEST_PUBLISHED is
// rejected -- see isValidRecipeVersionID). Deleting LATEST_WORKING only
// succeeds if the recipe has no published versions (in which case the whole
// recipe, having no remaining version, is removed); this mirrors
// BatchDeleteRecipeVersion's documented constraint that LATEST_WORKING is
// only deleted if the recipe has no other versions.
func (b *InMemoryBackend) DeleteRecipeVersion(ctx context.Context, name, version string) error {
	b.mu.Lock("DeleteRecipeVersion")
	defer b.mu.Unlock()
	region := getRegion(ctx, b.defaultRegion)
	if !b.recipesTable(region).Has(name) {
		return ErrNotFound
	}
	if version == recipeVersionLatestPublished || !isValidRecipeVersionID(version) {
		return fmt.Errorf("%w: invalid recipe version %q", ErrValidation, version)
	}

	versions := b.recipeVersionsStore(region)
	if version == recipeVersionLatestWorking {
		if len(versions[name]) > 0 {
			return fmt.Errorf(
				"%w: LATEST_WORKING cannot be deleted while %d published version(s) exist",
				ErrValidation, len(versions[name]),
			)
		}
		if p := b.recipeProjectName(region, name); p != "" {
			return fmt.Errorf("%w: LATEST_WORKING of recipe %q is used by project %q", ErrConflict, name, p)
		}
		if j, used := b.recipeVersionUsedByJob(region, name, version); used {
			return fmt.Errorf("%w: recipe %q version %q is used by job %q", ErrConflict, name, version, j)
		}
		b.recipesTable(region).Delete(name)
		delete(versions, name)

		return nil
	}

	idx := recipeVersionIndex(versions[name], version)
	if idx == -1 {
		return ErrNotFound
	}
	if j, used := b.recipeVersionUsedByJob(region, name, version); used {
		return fmt.Errorf("%w: recipe %q version %q is used by job %q", ErrConflict, name, version, j)
	}
	versions[name] = append(versions[name][:idx], versions[name][idx+1:]...)

	return nil
}

// recipeVersionUsedByJob reports whether a job currently references name at
// exactly version via its RecipeReference, and if so, that job's name.
// aws-sdk-go-v2/service/databrew's BatchDeleteRecipeVersion doc comment lists
// "A version is being used by a job" as a documented delete-blocking
// condition. Callers must hold at least b.mu.RLock.
func (b *InMemoryBackend) recipeVersionUsedByJob(region, name, version string) (string, bool) {
	t := b.jobsTable(region)
	for _, k := range snapshotKeys(t, jobKeyFn) {
		j, ok := t.Get(k)
		if ok && j.RecipeReference != nil && j.RecipeReference.Name == name &&
			j.RecipeReference.RecipeVersion == version {
			return j.Name, true
		}
	}

	return "", false
}

// recipeVersionIndex returns the index of version within versions, or -1.
func recipeVersionIndex(versions []*Recipe, version string) int {
	for i, v := range versions {
		if v.RecipeVersion == version {
			return i
		}
	}

	return -1
}

// BatchDeleteRecipeVersion deletes multiple recipe versions at once. Unlike
// DeleteRecipeVersion, an individual version that doesn't exist (or is a
// LATEST_WORKING that can't yet be deleted) is a documented PARTIAL failure
// -- the overall call still succeeds and that version is reported in the
// returned []RecipeVersionErrorDetail -- whereas the recipe not existing at
// all, an empty/oversized/duplicate-containing list, or a syntactically
// invalid version identifier reject the WHOLE request (returned as an
// error), per aws-sdk-go-v2/service/databrew's BatchDeleteRecipeVersion doc
// comment.
func (b *InMemoryBackend) BatchDeleteRecipeVersion(
	ctx context.Context,
	name string,
	toDelete []string,
) ([]RecipeVersionErrorDetail, error) {
	b.mu.Lock("BatchDeleteRecipeVersion")
	defer b.mu.Unlock()
	region := getRegion(ctx, b.defaultRegion)
	if !b.recipesTable(region).Has(name) {
		return nil, ErrNotFound
	}
	if err := validateBatchDeleteRecipeVersions(toDelete); err != nil {
		return nil, err
	}

	versions := b.recipeVersionsStore(region)
	errs := make([]RecipeVersionErrorDetail, 0, len(toDelete))
	for _, v := range toDelete {
		if errDetail, failed := b.batchDeleteOneRecipeVersion(region, name, v, versions); failed {
			errs = append(errs, errDetail)
		}
	}

	return errs, nil
}

// validateBatchDeleteRecipeVersions applies BatchDeleteRecipeVersion's
// whole-request validation rules (empty list, oversized list, duplicate
// entries, syntactically invalid identifiers). Callers must hold b.mu.
func validateBatchDeleteRecipeVersions(toDelete []string) error {
	if len(toDelete) == 0 {
		return fmt.Errorf("%w: RecipeVersions must not be empty", ErrValidation)
	}
	if len(toDelete) > maxBatchDeleteRecipeVersions {
		return fmt.Errorf(
			"%w: RecipeVersions exceeds the maximum of %d", ErrValidation, maxBatchDeleteRecipeVersions,
		)
	}
	seen := make(map[string]bool, len(toDelete))
	for _, v := range toDelete {
		if seen[v] {
			return fmt.Errorf("%w: duplicate recipe version %q", ErrValidation, v)
		}
		seen[v] = true
		if !isValidRecipeVersionID(v) {
			return fmt.Errorf("%w: invalid recipe version %q", ErrValidation, v)
		}
	}

	return nil
}

// batchDeleteOneRecipeVersion deletes a single version as part of a batch,
// reporting a partial failure (ok=true) instead of returning an error.
// Callers must hold b.mu.
func (b *InMemoryBackend) batchDeleteOneRecipeVersion(
	region, name, version string,
	versions map[string][]*Recipe,
) (RecipeVersionErrorDetail, bool) {
	if version == recipeVersionLatestWorking {
		if len(versions[name]) > 0 {
			return RecipeVersionErrorDetail{
				RecipeVersion: version, ErrorCode: errCodeValidation,
				ErrorMessage: "LATEST_WORKING cannot be deleted while other versions exist",
			}, true
		}
		if p := b.recipeProjectName(region, name); p != "" {
			return RecipeVersionErrorDetail{
				RecipeVersion: version, ErrorCode: errCodeConflict,
				ErrorMessage: fmt.Sprintf("LATEST_WORKING is being used by project %q", p),
			}, true
		}
		if j, used := b.recipeVersionUsedByJob(region, name, version); used {
			return RecipeVersionErrorDetail{
				RecipeVersion: version, ErrorCode: errCodeConflict,
				ErrorMessage: fmt.Sprintf("version is being used by job %q", j),
			}, true
		}
		b.recipesTable(region).Delete(name)
		delete(versions, name)

		return RecipeVersionErrorDetail{}, false
	}

	idx := recipeVersionIndex(versions[name], version)
	if idx == -1 {
		return RecipeVersionErrorDetail{
			RecipeVersion: version, ErrorCode: errCodeResourceNotFound,
			ErrorMessage: fmt.Sprintf("Recipe version %s not found", version),
		}, true
	}
	if j, used := b.recipeVersionUsedByJob(region, name, version); used {
		return RecipeVersionErrorDetail{
			RecipeVersion: version, ErrorCode: errCodeConflict,
			ErrorMessage: fmt.Sprintf("version is being used by job %q", j),
		}, true
	}
	versions[name] = append(versions[name][:idx], versions[name][idx+1:]...)

	return RecipeVersionErrorDetail{}, false
}

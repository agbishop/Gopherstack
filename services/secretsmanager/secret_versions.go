package secretsmanager

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"
)

const (
	// maxVersionsPerSecret is the maximum number of versions retained per secret.
	// Matches the AWS Secrets Manager limit of 100 versions.
	maxVersionsPerSecret = 100
	// maxSecretValueBytes is the maximum allowed size of a secret value in bytes (64 KB).
	maxSecretValueBytes = 65536
	// maxResultsBatchGet is the maximum allowed MaxResults for BatchGetSecretValue.
	maxResultsBatchGet = 20
	// maxSecretIDListSize is the maximum number of IDs allowed in BatchGetSecretValue.SecretIdList.
	maxSecretIDListSize = 20
)

// validateSecretSize returns ErrSecretValueTooLarge when the secret value exceeds 64 KB.
func validateSecretSize(secretString string, secretBinary []byte) error {
	if len(secretString) > maxSecretValueBytes {
		return fmt.Errorf(
			"%w: secret string value exceeds maximum size of %d bytes",
			ErrSecretValueTooLarge,
			maxSecretValueBytes,
		)
	}

	if len(secretBinary) > maxSecretValueBytes {
		return fmt.Errorf(
			"%w: secret binary value exceeds maximum size of %d bytes",
			ErrSecretValueTooLarge,
			maxSecretValueBytes,
		)
	}

	return nil
}

// GetSecretValue retrieves the value of a secret version.
func (b *InMemoryBackend) GetSecretValue(
	ctx context.Context, input *GetSecretValueInput,
) (*GetSecretValueOutput, error) {
	if input.SecretID == "" {
		return nil, fmt.Errorf("%w: SecretId is required", ErrInvalidParameter)
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("GetSecretValue")
	defer b.mu.Unlock()

	name := resolveSecretID(input.SecretID)

	secret, exists := b.secretGet(region, name)
	if !exists {
		return nil, ErrSecretNotFound
	}

	if secret.DeletedDate != nil {
		return nil, fmt.Errorf("%w: secret %s is deleted", ErrSecretDeleted, input.SecretID)
	}

	version := b.findVersion(secret, input.VersionID, input.VersionStage)
	if version == nil {
		return nil, ErrVersionNotFound
	}

	// When both VersionId and VersionStage are supplied, they must agree.
	// AWS returns ResourceNotFoundException when the ID does not carry the requested stage.
	if input.VersionID != "" && input.VersionStage != "" {
		if !slices.Contains(version.StagingLabels, input.VersionStage) {
			return nil, fmt.Errorf(
				"%w: staging label %s not found on version %s",
				ErrVersionNotFound,
				input.VersionStage,
				input.VersionID,
			)
		}
	}

	plainString, plainBinary, err := b.openVersion(ctx, version)
	if err != nil {
		return nil, err
	}

	// Track access date (truncated to day granularity as AWS does).
	accessDay := UnixTimeFloat(b.now().UTC().Truncate(hoursPerDay * time.Hour))
	secret.LastAccessedDate = &accessDay
	version.LastAccessedDate = &accessDay

	return &GetSecretValueOutput{
		ARN:              secret.ARN,
		Name:             secret.Name,
		VersionID:        version.VersionID,
		SecretString:     plainString,
		SecretBinary:     plainBinary,
		VersionStages:    version.StagingLabels,
		CreatedDate:      version.CreatedDate,
		LastAccessedDate: version.LastAccessedDate,
	}, nil
}

// findVersion locates the appropriate version by ID or staging label.
// Must be called with at least a read lock held.
func (b *InMemoryBackend) findVersion(secret *Secret, versionID, versionStage string) *SecretVersion {
	if versionID != "" {
		return secret.Versions[versionID]
	}

	label := versionStage
	if label == "" {
		label = StagingLabelCurrent
	}

	for _, v := range secret.Versions {
		if slices.Contains(v.StagingLabels, label) {
			return v
		}
	}

	return nil
}

// PutSecretValue adds a new version to an existing secret.
func (b *InMemoryBackend) PutSecretValue(
	ctx context.Context, input *PutSecretValueInput,
) (*PutSecretValueOutput, error) {
	if input.SecretID == "" {
		return nil, fmt.Errorf("%w: SecretId is required", ErrInvalidParameter)
	}

	if input.SecretString == "" && len(input.SecretBinary) == 0 {
		return nil, fmt.Errorf(
			"%w: you must provide either SecretString or SecretBinary",
			ErrInvalidParameter,
		)
	}

	if input.SecretString != "" && len(input.SecretBinary) > 0 {
		return nil, fmt.Errorf(
			"%w: you must provide either SecretString or SecretBinary, but not both",
			ErrInvalidParameter,
		)
	}

	if err := validateSecretSize(input.SecretString, input.SecretBinary); err != nil {
		return nil, err
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("PutSecretValue")
	defer b.mu.Unlock()

	name := resolveSecretID(input.SecretID)

	secret, lookupErr := b.resolvePrimaryOnlySecretLocked(region, input.SecretID, name)
	if lookupErr != nil {
		return nil, lookupErr
	}

	versionID := input.ClientRequestToken
	if versionID == "" {
		versionID = uuid.New().String()
	}

	// Idempotency: if the version ID already exists with identical content, return it.
	if existing, ok := secret.Versions[versionID]; ok {
		matched, err := b.matchesExistingVersion(ctx, existing, input.SecretString, input.SecretBinary)
		if err != nil {
			return nil, err
		}

		if matched {
			return &PutSecretValueOutput{
				ARN:           secret.ARN,
				Name:          secret.Name,
				VersionID:     existing.VersionID,
				VersionStages: existing.StagingLabels,
			}, nil
		}
	}

	callerWantsCurrentLabel, stagingLabels := b.resolveStagingLabels(secret, input.VersionStages)

	now := UnixTimeFloat(b.now())

	version, err := b.sealVersion(ctx, secret, versionID, input.SecretString, input.SecretBinary, stagingLabels, now)
	if err != nil {
		return nil, err
	}

	secret.Versions[versionID] = version

	if callerWantsCurrentLabel {
		secret.CurrentVersionID = versionID
	}

	secret.LastChangedDate = &now
	b.syncReplicationStatusLocked(region, secret)

	pruneVersions(secret)

	return &PutSecretValueOutput{
		ARN:           secret.ARN,
		Name:          secret.Name,
		VersionID:     versionID,
		VersionStages: stagingLabels,
	}, nil
}

// rotateStagingLabels moves AWSCURRENT to AWSPREVIOUS and removes old AWSPREVIOUS.
// Must be called with a write lock held.
func (b *InMemoryBackend) rotateStagingLabels(secret *Secret) {
	for _, v := range secret.Versions {
		newLabels := make([]string, 0, len(v.StagingLabels))

		for _, sl := range v.StagingLabels {
			switch sl {
			case StagingLabelCurrent:
				// Promote current to previous only if there isn't already a previous
				newLabels = append(newLabels, StagingLabelPrevious)
			case StagingLabelPrevious:
				// Drop old previous label (will be replaced)
			default:
				newLabels = append(newLabels, sl)
			}
		}

		v.StagingLabels = newLabels
	}
}

// resolveStagingLabels determines the staging labels for a new version and — when
// AWSCURRENT is requested — rotates the existing AWSCURRENT label to AWSPREVIOUS.
// Returns (wantsCurrentLabel, labels). Must be called with a write lock held.
func (b *InMemoryBackend) resolveStagingLabels(secret *Secret, requested []string) (bool, []string) {
	wantsCurrent := len(requested) == 0 || slices.Contains(requested, StagingLabelCurrent)

	if !wantsCurrent {
		out := make([]string, len(requested))
		copy(out, requested)

		return false, out
	}

	b.rotateStagingLabels(secret)

	out := []string{StagingLabelCurrent}

	for _, label := range requested {
		if label != StagingLabelCurrent {
			out = append(out, label)
		}
	}

	return true, out
}

// pruneVersions removes the oldest unlabeled versions when the total version count
// exceeds maxVersionsPerSecret. Versions with any staging labels are never pruned.
// Must be called with a write lock held.
func pruneVersions(secret *Secret) {
	if len(secret.Versions) <= maxVersionsPerSecret {
		return
	}

	type versionEntry struct {
		id          string
		createdDate float64
	}

	unlabeled := make([]versionEntry, 0, len(secret.Versions))

	for id, v := range secret.Versions {
		if len(v.StagingLabels) == 0 {
			unlabeled = append(unlabeled, versionEntry{id: id, createdDate: v.CreatedDate})
		}
	}

	// Sort oldest first; break ties by ID for deterministic eviction order.
	sort.Slice(unlabeled, func(i, j int) bool {
		if unlabeled[i].createdDate != unlabeled[j].createdDate {
			return unlabeled[i].createdDate < unlabeled[j].createdDate
		}

		return unlabeled[i].id < unlabeled[j].id
	})

	toRemove := min(len(secret.Versions)-maxVersionsPerSecret, len(unlabeled))

	for i := range toRemove {
		delete(secret.Versions, unlabeled[i].id)
	}
}

// ListSecretVersionIDs returns the list of versions for a secret with optional pagination.
func (b *InMemoryBackend) ListSecretVersionIDs(
	ctx context.Context, input *ListSecretVersionIDsInput,
) (*ListSecretVersionIDsOutput, error) {
	if err := validateMaxResults(input.MaxResults, maxResultsListSecrets); err != nil {
		return nil, err
	}

	region := getRegion(ctx, b.region)

	b.mu.RLock("ListSecretVersionIDs")
	defer b.mu.RUnlock()

	name := resolveSecretID(input.SecretID)

	secret, exists := b.secretGet(region, name)
	if !exists {
		return nil, ErrSecretNotFound
	}

	entries := make([]SecretVersionEntry, 0, len(secret.Versions))

	for _, v := range secret.Versions {
		if !input.IncludeDeprecated && len(v.StagingLabels) == 0 {
			continue
		}

		entries = append(entries, SecretVersionEntry{
			VersionID:        v.VersionID,
			StagingLabels:    v.StagingLabels,
			CreatedDate:      v.CreatedDate,
			LastAccessedDate: v.LastAccessedDate,
			KmsKeyIDs:        append([]string{}, v.KmsKeyIDs...),
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].CreatedDate != entries[j].CreatedDate {
			return entries[i].CreatedDate > entries[j].CreatedDate
		}

		return entries[i].VersionID > entries[j].VersionID
	})

	startIdx := parseToken(input.NextToken)
	maxResults := int64(defaultMaxResults)

	if input.MaxResults != nil && *input.MaxResults > 0 {
		maxResults = *input.MaxResults
	}

	if startIdx >= len(entries) {
		return &ListSecretVersionIDsOutput{
			ARN:      secret.ARN,
			Name:     secret.Name,
			Versions: []SecretVersionEntry{},
		}, nil
	}

	end := startIdx + int(maxResults)

	var nextToken string

	if end < len(entries) {
		nextToken = strconv.Itoa(end)
	} else {
		end = len(entries)
	}

	return &ListSecretVersionIDsOutput{
		ARN:       secret.ARN,
		Name:      secret.Name,
		Versions:  entries[startIdx:end],
		NextToken: nextToken,
	}, nil
}

// generateVersionID generates a random version ID for secret rotation.
func generateVersionID() string {
	return uuid.New().String()
}

// BatchGetSecretValue retrieves the values of multiple secrets in a single call.
func (b *InMemoryBackend) BatchGetSecretValue(
	ctx context.Context, input *BatchGetSecretValueInput,
) (*BatchGetSecretValueOutput, error) {
	if input.MaxResults != nil {
		mr := int64(*input.MaxResults)
		if err := validateMaxResults(&mr, maxResultsBatchGet); err != nil {
			return nil, err
		}
	}

	if len(input.SecretIDList) > maxSecretIDListSize {
		return nil, fmt.Errorf(
			"%w: SecretIdList must not contain more than %d entries",
			ErrInvalidParameter,
			maxSecretIDListSize,
		)
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("BatchGetSecretValue")
	defer b.mu.Unlock()

	out := &BatchGetSecretValueOutput{
		SecretValues: []SecretValueEntry{},
		Errors:       []APIErrorType{},
	}

	if len(input.SecretIDList) > 0 && len(input.Filters) > 0 {
		return nil, fmt.Errorf(
			"%w: you cannot specify both SecretIdList and Filters in the same request",
			ErrInvalidParameter,
		)
	}

	if len(input.SecretIDList) > 0 {
		b.batchGetByIDList(ctx, region, input.SecretIDList, out)

		return out, nil
	}

	return b.batchGetByFilter(ctx, region, input, out), nil
}

// batchGetByIDList populates out with values and errors for each explicit secret ID.
// Must be called with write lock held.
func (b *InMemoryBackend) batchGetByIDList(
	ctx context.Context, region string, ids []string, out *BatchGetSecretValueOutput,
) {
	accessDay := UnixTimeFloat(b.now().UTC().Truncate(hoursPerDay * time.Hour))

	for _, id := range ids {
		name := resolveSecretID(id)

		secret, ok := b.secretGet(region, name)
		if !ok {
			out.Errors = append(out.Errors, APIErrorType{
				ErrorCode: errResourceNotFoundException,
				Message:   "Secrets Manager can't find the specified secret.",
				SecretID:  id,
			})

			continue
		}

		if secret.DeletedDate != nil {
			out.Errors = append(out.Errors, APIErrorType{
				ErrorCode: "InvalidRequestException",
				Message:   "You can't perform this operation on the secret because it was deleted.",
				SecretID:  id,
			})

			continue
		}

		ver := b.findVersion(secret, "", StagingLabelCurrent)
		if ver == nil {
			out.Errors = append(out.Errors, APIErrorType{
				ErrorCode: errResourceNotFoundException,
				Message:   "Secrets Manager can't find the specified secret version.",
				SecretID:  id,
			})

			continue
		}

		entry, err := b.secretVersionEntry(ctx, secret, ver)
		if err != nil {
			out.Errors = append(out.Errors, APIErrorType{
				ErrorCode: "DecryptionFailure",
				Message:   err.Error(),
				SecretID:  id,
			})

			continue
		}

		secret.LastAccessedDate = &accessDay
		ver.LastAccessedDate = &accessDay
		out.SecretValues = append(out.SecretValues, entry)
	}
}

// batchGetByFilter collects and paginates secrets matching filters.
// Must be called with write lock held.
func (b *InMemoryBackend) batchGetByFilter(
	ctx context.Context,
	region string,
	input *BatchGetSecretValueInput,
	out *BatchGetSecretValueOutput,
) *BatchGetSecretValueOutput {
	secretsInRegion := b.secretsInRegion(region)
	allValues := make([]SecretValueEntry, 0, len(secretsInRegion))
	accessDay := UnixTimeFloat(b.now().UTC().Truncate(hoursPerDay * time.Hour))

	for _, secret := range secretsInRegion {
		if secret.DeletedDate != nil || !batchMatchesFilters(secret, input.Filters) {
			continue
		}

		ver := b.findVersion(secret, "", StagingLabelCurrent)
		if ver == nil {
			continue
		}

		entry, err := b.secretVersionEntry(ctx, secret, ver)
		if err != nil {
			out.Errors = append(out.Errors, APIErrorType{
				ErrorCode: "DecryptionFailure",
				Message:   err.Error(),
				SecretID:  secret.Name,
			})

			continue
		}

		secret.LastAccessedDate = &accessDay
		ver.LastAccessedDate = &accessDay
		allValues = append(allValues, entry)
	}

	// Sort by name for deterministic pagination.
	sort.Slice(allValues, func(i, j int) bool {
		return allValues[i].Name < allValues[j].Name
	})

	maxResults := int64(defaultMaxResults)
	if input.MaxResults != nil && *input.MaxResults > 0 {
		maxResults = int64(*input.MaxResults)
	}

	startIdx := parseToken(input.NextToken)
	if startIdx >= len(allValues) {
		return out
	}

	end := startIdx + int(maxResults)

	if end < len(allValues) {
		out.NextToken = strconv.Itoa(end)
	} else {
		end = len(allValues)
	}

	out.SecretValues = allValues[startIdx:end]

	return out
}

// secretVersionEntry builds a SecretValueEntry from a secret and version,
// decrypting the value via b.openVersion when it was sealed with KMS.
func (b *InMemoryBackend) secretVersionEntry(
	ctx context.Context, secret *Secret, ver *SecretVersion,
) (SecretValueEntry, error) {
	plainString, plainBinary, err := b.openVersion(ctx, ver)
	if err != nil {
		return SecretValueEntry{}, err
	}

	return SecretValueEntry{
		ARN:           secret.ARN,
		Name:          secret.Name,
		VersionID:     ver.VersionID,
		SecretString:  plainString,
		SecretBinary:  plainBinary,
		VersionStages: ver.StagingLabels,
		CreatedDate:   ver.CreatedDate,
	}, nil
}

// batchMatchesFilters returns true if the secret matches all provided filters.
// BatchGetSecretValueInput.Filters is []types.Filter (api_op_BatchGetSecretValue.go),
// the identical shared type ListSecretsInput.Filters uses, so all 7 documented keys
// apply here too (name/description/tag-key/tag-value/primary-region/owning-service/
// all) -- delegates to secretMatchesFilter rather than re-implementing a narrower
// switch, so the two operations can't drift.
func batchMatchesFilters(secret *Secret, filters []BatchGetSecretValueFilter) bool {
	for _, f := range filters {
		if !secretMatchesFilter(secret, SecretFilter(f)) {
			return false
		}
	}

	return true
}

// UpdateSecretVersionStage moves or adds a staging label to a specific secret version.
func (b *InMemoryBackend) UpdateSecretVersionStage(
	ctx context.Context,
	input *UpdateSecretVersionStageInput,
) (*UpdateSecretVersionStageOutput, error) {
	region := getRegion(ctx, b.region)

	b.mu.Lock("UpdateSecretVersionStage")
	defer b.mu.Unlock()

	name := resolveSecretID(input.SecretID)

	secret, ok := b.secretGet(region, name)
	if !ok {
		return nil, ErrSecretNotFound
	}

	if secret.DeletedDate != nil {
		return nil, fmt.Errorf("%w: secret %s is deleted", ErrSecretDeleted, input.SecretID)
	}

	// AWS does not allow removing AWSCURRENT without moving it to another version.
	if input.VersionStage == StagingLabelCurrent && input.MoveToVersionID == "" {
		return nil, fmt.Errorf(
			"%w: AWSCURRENT staging label can only be moved to another version, not removed",
			ErrInvalidParameter,
		)
	}

	if input.MoveToVersionID != "" {
		if err := b.moveStagingLabel(secret, input); err != nil {
			return nil, err
		}
	} else if input.RemoveFromVersionID != "" {
		if err := removeLabelFromVersion(secret, input.RemoveFromVersionID, input.VersionStage); err != nil {
			return nil, err
		}
	}

	return &UpdateSecretVersionStageOutput{
		ARN:  secret.ARN,
		Name: secret.Name,
	}, nil
}

// moveStagingLabel strips a staging label from all versions and applies it to the target version.
// Must be called with write lock held.
func (b *InMemoryBackend) moveStagingLabel(secret *Secret, input *UpdateSecretVersionStageInput) error {
	if input.RemoveFromVersionID != "" {
		if _, exists := secret.Versions[input.RemoveFromVersionID]; !exists {
			return ErrVersionNotFound
		}
	}

	targetVer, exists := secret.Versions[input.MoveToVersionID]
	if !exists {
		return ErrVersionNotFound
	}

	// Per the real API (UpdateSecretVersionStageInput.RemoveFromVersionId): "If the
	// staging label is already attached to a different version of the secret, then you
	// must also specify RemoveFromVersionId. ... If the label is attached and you either
	// do not specify this parameter, or the version ID does not match, then the
	// operation fails." Find who currently holds the label (if anyone) and enforce that.
	currentHolderID := versionIDWithLabel(secret, input.VersionStage)
	if currentHolderID != "" && currentHolderID != input.MoveToVersionID &&
		currentHolderID != input.RemoveFromVersionID {
		return fmt.Errorf(
			"%w: staging label %s is currently attached to version %s;"+
				" RemoveFromVersionId must be set to %s to move it",
			ErrInvalidParameter, input.VersionStage, currentHolderID, currentHolderID,
		)
	}

	if input.VersionStage == StagingLabelCurrent {
		// Moving AWSCURRENT demotes whoever held it to AWSPREVIOUS instead of
		// just dropping the label (aws-sdk-go-v2/service/secretsmanager@v1.44.4
		// api_op_UpdateSecretVersionStage.go doc: "Whenever you move AWSCURRENT,
		// Secrets Manager automatically moves the label AWSPREVIOUS to the
		// version that AWSCURRENT was removed from"). Reuses the same rotation
		// PutSecretValue performs so both call sites agree.
		b.rotateStagingLabels(secret)
		targetVer.StagingLabels = append(removeLabel(targetVer.StagingLabels, StagingLabelCurrent), StagingLabelCurrent)
		secret.CurrentVersionID = input.MoveToVersionID

		return nil
	}

	// Strip the label from ALL versions — a staging label belongs to exactly one version.
	for _, ver := range secret.Versions {
		ver.StagingLabels = removeLabel(ver.StagingLabels, input.VersionStage)
	}

	targetVer.StagingLabels = append(targetVer.StagingLabels, input.VersionStage)

	return nil
}

// versionIDWithLabel returns the ID of the version currently carrying the given staging
// label, or "" if no version has it. A staging label is attached to at most one version.
func versionIDWithLabel(secret *Secret, label string) string {
	for id, ver := range secret.Versions {
		if slices.Contains(ver.StagingLabels, label) {
			return id
		}
	}

	return ""
}

// removeLabelFromVersion removes a label from a specific version.
func removeLabelFromVersion(secret *Secret, versionID, label string) error {
	ver, exists := secret.Versions[versionID]
	if !exists {
		return ErrVersionNotFound
	}

	ver.StagingLabels = removeLabel(ver.StagingLabels, label)

	return nil
}

// removeLabel returns a copy of labels with the given label removed.
func removeLabel(labels []string, label string) []string {
	newLabels := make([]string, 0, len(labels))

	for _, lbl := range labels {
		if lbl != label {
			newLabels = append(newLabels, lbl)
		}
	}

	return newLabels
}

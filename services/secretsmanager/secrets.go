package secretsmanager

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// secretNamePattern is the set of characters allowed in secret names by AWS.
// AWS allows letters, digits, and the following special characters: /_+=.@-.
var secretNamePattern = regexp.MustCompile(`^[a-zA-Z0-9/_+=.@-]+$`)

// maxSecretNameLength is the maximum allowed length of a secret name.
const maxSecretNameLength = 512

// defaultRecoveryWindowDays is the default recovery window in days for soft-deleted secrets.
const defaultRecoveryWindowDays = 30

// minRecoveryWindowDays is the minimum recovery window in days.
const minRecoveryWindowDays = 7

const (
	// randomSuffixBytes is the number of bytes to use for the ARN random suffix.
	randomSuffixBytes = 3
	// arnMinParts is the minimum number of colon-separated parts in a Secrets Manager ARN.
	arnMinParts = 7
	// arnNameIndex is the index of the name-with-suffix part in a Secrets Manager ARN.
	arnNameIndex = 6
	// arnSuffixLen is the length of the random ARN suffix: dash + 6 hex characters.
	arnSuffixLen = 7
	// maxResultsListSecrets is the maximum allowed MaxResults for ListSecrets/ListSecretVersionIds.
	maxResultsListSecrets = 100
)

// generateRandomSuffix generates a 6-character hex random suffix for ARNs.
func generateRandomSuffix() (string, error) {
	b := make([]byte, randomSuffixBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate ARN suffix: %w", err)
	}

	return hex.EncodeToString(b), nil
}

// buildARNWithRegion constructs a Secrets Manager ARN using the given region.
func (b *InMemoryBackend) buildARNWithRegion(region, name, suffix string) string {
	return arn.Build("secretsmanager", region, b.accountID, "secret:"+name+"-"+suffix)
}

// primaryRegionOrSelf returns s's origin region: its own region for a
// primary/standalone secret, or the linked primary's region for a replica.
func (s *Secret) primaryRegionOrSelf() string {
	if s.PrimaryRegion != "" {
		return s.PrimaryRegion
	}

	return s.region
}

// isReplica reports whether s is a replica secret rather than a primary or
// standalone one -- see PrimaryRegion's doc comment.
func (s *Secret) isReplica() bool {
	return s.PrimaryRegion != ""
}

// resolvePrimaryOnlySecretLocked looks up name in region for an operation
// that real AWS only permits against a primary/standalone secret --
// PutSecretValue and RotateSecret. It reports ErrSecretNotFound,
// ErrSecretDeleted, or ErrReplicaNotWritable as appropriate, folding those
// three checks (shared verbatim by both callers) into one branch at the call
// site instead of three. Must be called with b.mu held.
func (b *InMemoryBackend) resolvePrimaryOnlySecretLocked(region, secretID, name string) (*Secret, error) {
	secret, exists := b.secretGet(region, name)
	if !exists {
		return nil, ErrSecretNotFound
	}

	if secret.DeletedDate != nil {
		return nil, fmt.Errorf("%w: secret %s is deleted", ErrSecretDeleted, secretID)
	}

	if secret.isReplica() {
		return nil, fmt.Errorf(
			"%w: %s is a replica secret; this operation must be called against the primary secret in %s",
			ErrReplicaNotWritable, secretID, secret.PrimaryRegion,
		)
	}

	return secret, nil
}

// validateSecretName returns an error when the name is empty, too long, contains invalid chars,
// or starts with the "aws/" prefix reserved for AWS managed secrets.
func validateSecretName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: secret name must not be empty", ErrInvalidSecretName)
	}

	if len(name) > maxSecretNameLength {
		return fmt.Errorf(
			"%w: secret name must be %d characters or fewer",
			ErrInvalidSecretName,
			maxSecretNameLength,
		)
	}

	if !secretNamePattern.MatchString(name) {
		return fmt.Errorf(
			"%w: secret name must match pattern [a-zA-Z0-9/_+=.@-]+",
			ErrInvalidSecretName,
		)
	}

	if strings.HasPrefix(name, "aws/") {
		return fmt.Errorf(
			"%w: secret name must not start with \"aws/\" (reserved for AWS managed secrets)",
			ErrInvalidSecretName,
		)
	}

	return nil
}

// CreateSecret creates a new secret with an optional initial value.
func (b *InMemoryBackend) CreateSecret(ctx context.Context, input *CreateSecretInput) (*CreateSecretOutput, error) {
	if err := validateSecretName(input.Name); err != nil {
		return nil, err
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

	if err := validateTagCount(0, len(input.Tags)); err != nil {
		return nil, err
	}

	region := getRegion(ctx, b.region)
	if input.Region != "" {
		region = input.Region
	}

	b.mu.Lock("CreateSecret")
	defer b.mu.Unlock()

	if existing, exists := b.secretGet(region, input.Name); exists {
		return b.createSecretNameCollision(ctx, region, existing, input)
	}

	suffix, err := generateRandomSuffix()
	if err != nil {
		return nil, err
	}

	arn := b.buildARNWithRegion(region, input.Name, suffix)

	secret := &Secret{
		region:      region,
		ARN:         arn,
		Name:        input.Name,
		Description: input.Description,
		KmsKeyID:    input.KmsKeyID,
		Type:        input.Type,
		Versions:    make(map[string]*SecretVersion),
	}

	createdNow := UnixTimeFloat(b.now())
	secret.CreatedDate = &createdNow

	if len(input.Tags) > 0 {
		secret.Tags = tags.New(secret.Name + ".tags")

		for _, t := range input.Tags {
			secret.Tags.Set(t.Key, t.Value)
		}
	}

	versionID, err := b.seedInitialVersion(ctx, secret, input, b.now())
	if err != nil {
		return nil, err
	}

	b.secretPut(secret)

	if len(input.AddReplicaRegions) > 0 {
		replicas := make([]ReplicationStatusType, 0, len(input.AddReplicaRegions))
		for _, r := range input.AddReplicaRegions {
			replicas = append(replicas, ReplicationStatusType{
				Region:        r.Region,
				KmsKeyID:      r.KmsKeyID,
				Status:        replicationStatusInProgress,
				StatusMessage: "replication queued",
			})
		}
		b.replicationConfigsStore(region)[input.Name] = replicas
	}

	b.syncReplicationStatusLocked(region, secret)

	return &CreateSecretOutput{
		ARN:               arn,
		Name:              input.Name,
		VersionID:         versionID,
		ReplicationStatus: b.replicationConfigsStore(region)[input.Name],
	}, nil
}

// createSecretNameCollision handles CreateSecret when a secret with input.Name already
// exists: it applies the ClientRequestToken idempotency contract (see
// CreateSecretInput.ClientRequestToken in the real API) — a retried CreateSecret with a
// ClientRequestToken matching an already-created version is ignored (success, no new
// version) when the content matches, and fails when the content differs, since
// CreateSecret cannot modify an existing version. Must be called with b.mu held.
func (b *InMemoryBackend) createSecretNameCollision(
	ctx context.Context, region string, existing *Secret, input *CreateSecretInput,
) (*CreateSecretOutput, error) {
	if existing.DeletedDate != nil {
		return nil, fmt.Errorf(
			"%w: a secret with this name is already scheduled for deletion; restore or force-delete it first",
			ErrSecretDeleted,
		)
	}

	if input.ClientRequestToken == "" {
		return nil, ErrSecretAlreadyExists
	}

	v, ok := existing.Versions[input.ClientRequestToken]
	if !ok {
		return nil, ErrSecretAlreadyExists
	}

	matched, err := b.matchesExistingVersion(ctx, v, input.SecretString, input.SecretBinary)
	if err != nil {
		return nil, err
	}

	if matched {
		return &CreateSecretOutput{
			ARN:               existing.ARN,
			Name:              existing.Name,
			VersionID:         v.VersionID,
			ReplicationStatus: b.replicationConfigsStore(region)[input.Name],
		}, nil
	}

	return nil, fmt.Errorf(
		"%w: a version with ClientRequestToken %s already exists with different content;"+
			" use PutSecretValue to create a new version",
		ErrInvalidParameter, input.ClientRequestToken,
	)
}

// seedInitialVersion creates the initial AWSCURRENT version on a freshly created secret
// when the create request carries a value, sealing it via KMS when an encryptor is wired
// (see sealVersion), and returns the version ID (empty if none). nowTime is the backend's
// (possibly test-injected) clock value, so CreatedDate stays consistent with the rest of
// the secret's timestamps. Must be called with b.mu held (write lock).
func (b *InMemoryBackend) seedInitialVersion(
	ctx context.Context, secret *Secret, input *CreateSecretInput, nowTime time.Time,
) (string, error) {
	if input.SecretString == "" && len(input.SecretBinary) == 0 {
		return "", nil
	}

	// Use ClientRequestToken as initial version ID for idempotency.
	versionID := input.ClientRequestToken
	if versionID == "" {
		versionID = uuid.New().String()
	}

	now := UnixTimeFloat(nowTime)

	version, err := b.sealVersion(
		ctx, secret, versionID, input.SecretString, input.SecretBinary, []string{StagingLabelCurrent}, now,
	)
	if err != nil {
		return "", err
	}

	secret.Versions[versionID] = version
	secret.CurrentVersionID = versionID
	secret.LastChangedDate = &now

	return versionID, nil
}

// DeleteSecret marks a secret as deleted, or permanently removes it when ForceDeleteWithoutRecovery is set.
func (b *InMemoryBackend) DeleteSecret(ctx context.Context, input *DeleteSecretInput) (*DeleteSecretOutput, error) {
	region := getRegion(ctx, b.region)

	b.mu.Lock("DeleteSecret")
	defer b.mu.Unlock()

	name := resolveSecretID(input.SecretID)

	secret, exists := b.secretGet(region, name)
	if !exists {
		return nil, ErrSecretNotFound
	}

	now := UnixTimeFloat(b.now())

	// AWS rejects combining ForceDeleteWithoutRecovery with RecoveryWindowInDays:
	// the two parameters are mutually exclusive. Real AWS returns
	// InvalidParameterException for this combination.
	if input.ForceDeleteWithoutRecovery && input.RecoveryWindowInDays != nil {
		return nil, fmt.Errorf(
			"%w: you can't use ForceDeleteWithoutRecovery in conjunction with RecoveryWindowInDays",
			ErrInvalidParameter,
		)
	}

	// Real AWS: "You can't delete a primary secret that is replicated to
	// other Regions. You must first delete the replicas using
	// RemoveRegionsFromReplication, and then delete the primary secret."
	// (api_op_DeleteSecret.go doc comment, aws-sdk-go-v2/service/secretsmanager@v1.44.4).
	if secret.PrimaryRegion == "" && len(b.replicationConfigsStore(region)[name]) > 0 {
		return nil, fmt.Errorf(
			"%w: you can't delete a primary secret that is replicated to other Regions;"+
				" remove the replicas with RemoveRegionsFromReplication first",
			ErrInvalidParameter,
		)
	}

	// "When you delete a replica, it is deleted immediately" -- a replica
	// carries no recovery window of its own, regardless of the request's
	// ForceDeleteWithoutRecovery/RecoveryWindowInDays fields.
	if secret.PrimaryRegion != "" {
		b.deleteReplicaImmediatelyLocked(region, secret)

		return &DeleteSecretOutput{ARN: secret.ARN, Name: secret.Name, DeletionDate: now}, nil
	}

	if input.ForceDeleteWithoutRecovery {
		if secret.Tags != nil {
			secret.Tags.Close()
		}

		b.secretDelete(region, name)
		delete(b.resourcePoliciesStore(region), name)
		delete(b.replicationConfigsStore(region), name)

		return &DeleteSecretOutput{
			ARN:          secret.ARN,
			Name:         secret.Name,
			DeletionDate: now,
		}, nil
	}

	// Reject deletion of an already soft-deleted secret.
	if secret.DeletedDate != nil {
		return nil, fmt.Errorf(
			"%w: you can't delete a secret that's already scheduled for deletion",
			ErrInvalidParameter,
		)
	}

	// Validate recovery window if provided.
	recoveryDays := int64(defaultRecoveryWindowDays)
	if input.RecoveryWindowInDays != nil {
		recoveryDays = *input.RecoveryWindowInDays
		if recoveryDays < minRecoveryWindowDays || recoveryDays > defaultRecoveryWindowDays {
			return nil, fmt.Errorf(
				"%w: RecoveryWindowInDays must be between %d and %d",
				ErrInvalidParameter,
				minRecoveryWindowDays,
				defaultRecoveryWindowDays,
			)
		}
	}

	secret.DeletedDate = &now
	deletionDate := UnixTimeFloat(b.now().Add(time.Duration(recoveryDays) * hoursPerDay * time.Hour))
	secret.ScheduledDeletionDate = &deletionDate

	return &DeleteSecretOutput{
		ARN:          secret.ARN,
		Name:         secret.Name,
		DeletionDate: deletionDate,
	}, nil
}

// deleteReplicaImmediatelyLocked removes a replica secret (found under
// replicaRegion) and drops its status entry from the primary's outgoing
// replication config, so the primary stops re-mirroring into a region that
// no longer has a replica. Must be called with b.mu held.
func (b *InMemoryBackend) deleteReplicaImmediatelyLocked(replicaRegion string, secret *Secret) {
	if secret.Tags != nil {
		secret.Tags.Close()
	}

	b.secretDelete(replicaRegion, secret.Name)
	delete(b.resourcePoliciesStore(replicaRegion), secret.Name)

	cfgs, ok := b.replicationConfigs[secret.PrimaryRegion]
	if !ok {
		return
	}

	statuses := cfgs[secret.Name]
	remaining := make([]ReplicationStatusType, 0, len(statuses))

	for _, st := range statuses {
		if st.Region != replicaRegion {
			remaining = append(remaining, st)
		}
	}

	setReplicationStatuses(cfgs, secret.Name, remaining)
}

// ListSecrets returns a paginated list of secrets.
func (b *InMemoryBackend) ListSecrets(ctx context.Context, input *ListSecretsInput) (*ListSecretsOutput, error) {
	if err := validateMaxResults(input.MaxResults, maxResultsListSecrets); err != nil {
		return nil, err
	}

	region := getRegion(ctx, b.region)

	b.mu.RLock(opListSecrets)
	defer b.mu.RUnlock()

	secretsInRegion := b.secretsInRegion(region)
	entries := make([]SecretListEntry, 0, len(secretsInRegion))

	for _, s := range secretsInRegion {
		if s.DeletedDate != nil && !input.IncludePlannedDeletion {
			continue
		}

		if !secretMatchesFilters(s, input.Filters) {
			continue
		}

		entries = append(entries, secretToListEntry(s))
	}

	sortSecretListEntries(entries, input.SortBy, input.SortOrder)

	startIdx := parseToken(input.NextToken)
	maxResults := int64(defaultMaxResults)

	if input.MaxResults != nil && *input.MaxResults > 0 {
		maxResults = *input.MaxResults
	}

	if startIdx >= len(entries) {
		return &ListSecretsOutput{SecretList: []SecretListEntry{}}, nil
	}

	end := startIdx + int(maxResults)

	var nextToken string

	if end < len(entries) {
		nextToken = strconv.Itoa(end)
	} else {
		end = len(entries)
	}

	return &ListSecretsOutput{
		SecretList: entries[startIdx:end],
		NextToken:  nextToken,
	}, nil
}

// sortSecretListEntries orders entries by the requested SortBy key ("name" (default),
// "created-date", "last-changed-date", "last-accessed-date"), honouring SortOrder
// ("asc" default, or "desc"). Unset date fields sort as the earliest possible value.
// Matches the AWS SortByType enum (ListSecrets request field "SortBy").
func sortSecretListEntries(entries []SecretListEntry, sortBy, sortOrder string) {
	desc := strings.EqualFold(sortOrder, "desc")

	var less func(i, j int) bool

	switch strings.ToLower(strings.TrimSpace(sortBy)) {
	case "created-date":
		less = func(i, j int) bool {
			return float64PtrLess(entries[i].CreatedDate, entries[j].CreatedDate, entries[i].Name, entries[j].Name)
		}
	case "last-changed-date":
		less = func(i, j int) bool {
			return float64PtrLess(
				entries[i].LastChangedDate, entries[j].LastChangedDate, entries[i].Name, entries[j].Name,
			)
		}
	case "last-accessed-date":
		less = func(i, j int) bool {
			return float64PtrLess(
				entries[i].LastAccessedDate, entries[j].LastAccessedDate, entries[i].Name, entries[j].Name,
			)
		}
	default:
		less = func(i, j int) bool { return entries[i].Name < entries[j].Name }
	}

	sort.Slice(entries, func(i, j int) bool {
		if desc {
			return less(j, i)
		}

		return less(i, j)
	})
}

// float64PtrLess compares two optional float64 fields, treating a nil pointer as the
// earliest possible value (AWS omits date fields that have never been set, e.g. a
// secret that was never rotated has no LastRotatedDate). Ties are broken by name for
// deterministic, stable ordering.
func float64PtrLess(a, b *float64, nameA, nameB string) bool {
	av, bv := ptrFloatOrMin(a), ptrFloatOrMin(b)
	if av != bv {
		return av < bv
	}

	return nameA < nameB
}

// ptrFloatOrMin dereferences a *float64, returning -math.MaxFloat64 for nil.
func ptrFloatOrMin(f *float64) float64 {
	if f == nil {
		return -math.MaxFloat64
	}

	return *f
}

// secretMatchesFilters returns true if the secret matches all provided filters.
func secretMatchesFilters(s *Secret, filters []SecretFilter) bool {
	for _, f := range filters {
		if !secretMatchesFilter(s, f) {
			return false
		}
	}

	return true
}

// secretMatchesFilter returns true if the secret matches a single filter.
func secretMatchesFilter(s *Secret, f SecretFilter) bool {
	switch f.Key {
	case "name":
		return anyMatchPrefix(f.Values, s.Name)
	case "description":
		return anyMatchPrefix(f.Values, s.Description)
	case "tag-key":
		return secretHasTagKey(s, f.Values)
	case "tag-value":
		return secretHasTagValue(s, f.Values)
	case "all":
		// "all" matches any of the filterable string fields.
		return anyMatchPrefix(f.Values, s.Name) ||
			anyMatchPrefix(f.Values, s.Description) ||
			secretHasTagKey(s, f.Values) ||
			secretHasTagValue(s, f.Values)
	case "primary-region":
		// In a single-region mock every secret belongs to the single region;
		// the filter always passes (no cross-region replication routing needed).
		return true
	case "owning-service":
		// No secret in this mock ever has an owning service (no CreateSecret/
		// UpdateSecret input field sets DescribeSecretOutput.OwningService — it
		// is only ever set by AWS itself for service-linked secrets, e.g.
		// RDS-managed rotation, which this mock does not model). A real
		// "owning-service" prefix filter therefore matches nothing here, same
		// as it would against any AWS secret with no owning service.
		return anyMatchPrefix(f.Values, "")
	default:
		return true
	}
}

// anyMatchPrefix returns true if target matches values under prefix semantics,
// honouring AWS's documented negation prefix: "You can prefix your search value with
// an exclamation mark ( ! ) in order to perform negation filters" (types.Filter.Values
// doc comment, aws-sdk-go-v2/service/secretsmanager@v1.44.4 types/types.go -- Filter is
// the shared type both ListSecretsInput and BatchGetSecretValueInput carry as Filters).
// A negated value excludes any target with that prefix; if any positive (non-negated)
// values are present, at least one must also match.
func anyMatchPrefix(values []string, target string) bool {
	hasPositive, positiveMatch := false, false

	for _, v := range values {
		if negated, ok := strings.CutPrefix(v, "!"); ok {
			if strings.HasPrefix(target, negated) {
				return false
			}

			continue
		}

		hasPositive = true
		if strings.HasPrefix(target, v) {
			positiveMatch = true
		}
	}

	return !hasPositive || positiveMatch
}

// secretHasTagKey returns true if the secret has at least one of the given tag keys.
func secretHasTagKey(s *Secret, keys []string) bool {
	if s.Tags == nil {
		return false
	}

	tagMap := s.Tags.Clone()
	for _, k := range keys {
		if _, ok := tagMap[k]; ok {
			return true
		}
	}

	return false
}

// secretHasTagValue returns true if the secret has at least one tag with any of the given values.
func secretHasTagValue(s *Secret, values []string) bool {
	if s.Tags == nil {
		return false
	}

	tagMap := s.Tags.Clone()
	for _, v := range tagMap {
		if slices.Contains(values, v) {
			return true
		}
	}

	return false
}

// DescribeSecret returns metadata about a secret.
func (b *InMemoryBackend) DescribeSecret(
	ctx context.Context,
	input *DescribeSecretInput,
) (*DescribeSecretOutput, error) {
	region := getRegion(ctx, b.region)

	b.mu.RLock(opDescribeSecret)
	defer b.mu.RUnlock()

	name := resolveSecretID(input.SecretID)

	secret, exists := b.secretGet(region, name)
	if !exists {
		return nil, ErrSecretNotFound
	}

	versionIDsToStages := make(map[string][]string, len(secret.Versions))

	for vID, v := range secret.Versions {
		if len(v.StagingLabels) > 0 {
			versionIDsToStages[vID] = append([]string(nil), v.StagingLabels...)
		}
	}

	out := &DescribeSecretOutput{
		ARN:                            secret.ARN,
		Name:                           secret.Name,
		Description:                    secret.Description,
		KmsKeyID:                       secret.KmsKeyID,
		RotationLambdaARN:              secret.RotationLambdaARN,
		RotationRules:                  cloneRotationRules(secret.RotationRules),
		Tags:                           tagsToSlice(secret.Tags),
		CreatedDate:                    secret.CreatedDate,
		DeletedDate:                    secret.DeletedDate,
		LastChangedDate:                secret.LastChangedDate,
		LastRotatedDate:                secret.LastRotatedDate,
		LastAccessedDate:               secret.LastAccessedDate,
		VersionIDsToStages:             versionIDsToStages,
		RotationEnabled:                secret.RotationEnabled,
		ReplicationStatus:              b.replicationConfigsStoreRO(region)[name],
		PrimaryRegion:                  secret.primaryRegionOrSelf(),
		Type:                           secret.Type,
		ExternalSecretRotationRoleArn:  secret.ExternalSecretRotationRoleArn,
		ExternalSecretRotationMetadata: cloneExternalSecretRotationMetadata(secret.ExternalSecretRotationMetadata),
	}

	// Compute NextRotationDate from the last rotation base + interval.
	out.NextRotationDate = computeNextRotationDate(secret)

	return out, nil
}

// UpdateSecret updates the description of a secret and optionally creates a new version.
func (b *InMemoryBackend) UpdateSecret(ctx context.Context, input *UpdateSecretInput) (*UpdateSecretOutput, error) {
	if err := validateSecretSize(input.SecretString, input.SecretBinary); err != nil {
		return nil, err
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("UpdateSecret")
	defer b.mu.Unlock()

	name := resolveSecretID(input.SecretID)

	secret, exists := b.secretGet(region, name)
	if !exists {
		return nil, ErrSecretNotFound
	}

	if secret.DeletedDate != nil {
		return nil, fmt.Errorf("%w: secret %s is deleted", ErrSecretDeleted, input.SecretID)
	}

	// A replica secret "can't be updated independently from its primary
	// secret, except for its encryption key" -- so only a KmsKeyID-only
	// UpdateSecret call is allowed through here; any of the other fields
	// makes this a rejected independent update.
	if secret.isReplica() && (input.SecretString != "" || len(input.SecretBinary) > 0 ||
		input.Description != "" || input.Type != "") {
		return nil, fmt.Errorf(
			"%w: %s is a replica secret; only its encryption key can be updated"+
				" independently of the primary secret in %s",
			ErrReplicaNotWritable, input.SecretID, secret.PrimaryRegion,
		)
	}

	// KmsKeyID is applied before updateSecretVersion because sealVersion reads
	// secret.KmsKeyID to pick the encryption key for a value change in the same
	// call, but rolled back on failure so a rejected request leaves it
	// untouched (matches the parity-principles "state mutated before
	// validation" bug class).
	oldKmsKeyID := secret.KmsKeyID
	if input.KmsKeyID != nil {
		secret.KmsKeyID = *input.KmsKeyID
	}

	var versionID string

	if input.SecretString != "" || len(input.SecretBinary) > 0 {
		var err error

		versionID, err = b.updateSecretVersion(ctx, region, secret, input)
		if err != nil {
			secret.KmsKeyID = oldKmsKeyID

			return nil, err
		}
	}

	if input.Description != "" {
		secret.Description = input.Description
	}

	if input.Type != "" {
		secret.Type = input.Type
	}

	return &UpdateSecretOutput{
		ARN:       secret.ARN,
		Name:      secret.Name,
		VersionID: versionID,
	}, nil
}

// updateSecretVersion applies an UpdateSecret request's new value to secret:
// it resolves the version ID (from ClientRequestToken or a fresh UUID),
// treats a retried ClientRequestToken carrying identical content (decrypted
// first when sealed via KMS) as an idempotent no-op, and otherwise seals and
// stores a new AWSCURRENT version. Returns the resulting version ID. Must be
// called with b.mu held and only when input carries a SecretString or
// SecretBinary.
func (b *InMemoryBackend) updateSecretVersion(
	ctx context.Context, region string, secret *Secret, input *UpdateSecretInput,
) (string, error) {
	versionID := input.ClientRequestToken
	if versionID == "" {
		versionID = uuid.New().String()
	}

	if existing, ok := secret.Versions[versionID]; ok {
		matched, err := b.matchesExistingVersion(ctx, existing, input.SecretString, input.SecretBinary)
		if err != nil {
			return "", err
		}

		if matched {
			return versionID, nil
		}
	}

	b.rotateStagingLabels(secret)

	now := UnixTimeFloat(b.now())

	version, err := b.sealVersion(
		ctx, secret, versionID, input.SecretString, input.SecretBinary, []string{StagingLabelCurrent}, now,
	)
	if err != nil {
		return "", err
	}

	secret.Versions[versionID] = version
	secret.CurrentVersionID = versionID
	secret.LastChangedDate = &now
	b.syncReplicationStatusLocked(region, secret)

	pruneVersions(secret)

	return versionID, nil
}

// RestoreSecret clears the deletion mark from a secret.
func (b *InMemoryBackend) RestoreSecret(ctx context.Context, input *RestoreSecretInput) (*RestoreSecretOutput, error) {
	region := getRegion(ctx, b.region)

	b.mu.Lock("RestoreSecret")
	defer b.mu.Unlock()

	name := resolveSecretID(input.SecretID)

	secret, exists := b.secretGet(region, name)
	if !exists {
		return nil, ErrSecretNotFound
	}

	// AWS returns InvalidRequestException when trying to restore a secret that
	// is not in a deleted (pending deletion) state.
	if secret.DeletedDate == nil {
		return nil, fmt.Errorf(
			"%w: secret %s is not in a deleted state",
			ErrInvalidParameter,
			input.SecretID,
		)
	}

	secret.DeletedDate = nil

	return &RestoreSecretOutput{
		ARN:  secret.ARN,
		Name: secret.Name,
	}, nil
}

// ListAll returns all secrets across all regions as list entries, sorted by name
// (for dashboard use).
func (b *InMemoryBackend) ListAll() []SecretListEntry {
	b.mu.RLock("ListAll")
	defer b.mu.RUnlock()

	all := b.secrets.All()
	entries := make([]SecretListEntry, 0, len(all))

	for _, s := range all {
		entries = append(entries, secretToListEntry(s))
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	return entries
}

// secretToListEntry converts a Secret to a SecretListEntry.
func secretToListEntry(s *Secret) SecretListEntry {
	var versionStages map[string][]string
	if len(s.Versions) > 0 {
		versionStages = make(map[string][]string, len(s.Versions))
		for id, v := range s.Versions {
			if len(v.StagingLabels) > 0 {
				versionStages[id] = append([]string(nil), v.StagingLabels...)
			}
		}
		if len(versionStages) == 0 {
			versionStages = nil
		}
	}

	return SecretListEntry{
		ARN:                            s.ARN,
		Name:                           s.Name,
		Description:                    s.Description,
		KmsKeyID:                       s.KmsKeyID,
		RotationLambdaARN:              s.RotationLambdaARN,
		PrimaryRegion:                  s.primaryRegionOrSelf(),
		RotationRules:                  cloneRotationRules(s.RotationRules),
		RotationEnabled:                s.RotationEnabled,
		DeletedDate:                    s.DeletedDate,
		LastChangedDate:                s.LastChangedDate,
		LastAccessedDate:               s.LastAccessedDate,
		LastRotatedDate:                s.LastRotatedDate,
		CreatedDate:                    s.CreatedDate,
		NextRotationDate:               computeNextRotationDate(s),
		Tags:                           tagsToSlice(s.Tags),
		SecretVersionsToStages:         versionStages,
		Type:                           s.Type,
		ExternalSecretRotationRoleArn:  s.ExternalSecretRotationRoleArn,
		ExternalSecretRotationMetadata: cloneExternalSecretRotationMetadata(s.ExternalSecretRotationMetadata),
	}
}

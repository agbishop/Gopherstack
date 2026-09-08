package backup

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	sdk_s3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
)

// completedJobBytes is the simulated transfer / backup size for completed jobs.
const completedJobBytes = 1024

// StartBackupJob starts a new backup job.
func (b *InMemoryBackend) StartBackupJob(
	vaultName, resourceArn, iamRoleArn, resourceType string,
) (*Job, error) {
	b.mu.Lock("StartBackupJob")
	defer b.mu.Unlock()

	if resourceArn == "" {
		return nil, fmt.Errorf("%w: ResourceArn is required", ErrValidation)
	}

	if iamRoleArn == "" {
		return nil, fmt.Errorf("%w: IamRoleArn is required", ErrValidation)
	}

	if err := b.resourceExistsLocked(resourceArn); err != nil {
		return nil, err
	}

	vault, ok := b.vaults.Get(vaultName)
	if !ok {
		return nil, fmt.Errorf("%w: vault %s not found", ErrNotFound, vaultName)
	}

	jobID := uuid.NewString()
	j := &Job{
		BackupJobID:     jobID,
		BackupVaultName: vaultName,
		BackupVaultArn:  vault.BackupVaultArn,
		ResourceArn:     resourceArn,
		IAMRoleArn:      iamRoleArn,
		ResourceType:    resourceType,
		State:           statusCreated,
		AccountID:       b.accountID,
		Region:          b.region,
		CreationTime:    time.Now().UTC(),
	}
	b.jobs.Put(j)
	cp := *j

	return &cp, nil
}

// resourceExistsLocked checks a StartBackupJob ResourceArn against the
// resource-owning backend, for the handful of resource types this emulator
// can classify and has a wired hook for. An ARN of an unclassifiable or
// unwired type is accepted unchecked -- real AWS Backup supports many
// resource types this emulator does not model, and rejecting them would be
// a fabricated rejection; an unwired hook must behave as if this check
// never ran. Callers must hold b.mu.
func (b *InMemoryBackend) resourceExistsLocked(resourceArn string) error {
	bucket, ok := s3BucketFromARN(resourceArn)
	if !ok || b.s3 == nil {
		return nil
	}

	if _, err := b.s3.HeadBucket(context.Background(), &sdk_s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	}); err != nil {
		return fmt.Errorf("%w: S3 bucket %s does not exist", ErrNotFound, bucket)
	}

	return nil
}

// arnFieldCount is the number of colon-separated fields in an ARN:
// arn:partition:service:region:account-id:resource.
const arnFieldCount = 6

// s3BucketFromARN reports whether resourceArn is a well-formed S3 bucket ARN
// (arn:{partition}:s3:::{bucket}, no object key) and, if so, the bucket
// name. AWS Backup's S3 resource type ARN is exactly the bucket ARN --
// object-key ARNs (bucket/key) are left unclassified and stay permissive.
func s3BucketFromARN(resourceArn string) (string, bool) {
	parts := strings.SplitN(resourceArn, ":", arnFieldCount)
	if len(parts) != arnFieldCount || parts[0] != "arn" || parts[2] != "s3" ||
		parts[5] == "" || strings.Contains(parts[5], "/") {
		return "", false
	}

	return parts[5], true
}

// DescribeBackupJob returns a backup job by ID.
func (b *InMemoryBackend) DescribeBackupJob(jobID string) (*Job, error) {
	b.mu.RLock("DescribeBackupJob")
	defer b.mu.RUnlock()

	j, ok := b.jobs.Get(jobID)
	if !ok {
		return nil, fmt.Errorf("%w: backup job %s not found", ErrNotFound, jobID)
	}
	cp := *j

	return &cp, nil
}

// ListBackupJobs returns all backup jobs, optionally filtered by vault name.
// Results are sorted by creation time (newest first).
func (b *InMemoryBackend) ListBackupJobs(vaultName string) []*Job {
	b.mu.RLock("ListBackupJobs")
	defer b.mu.RUnlock()

	all := b.jobs.All()
	list := make([]*Job, 0, len(all))
	for _, j := range all {
		if vaultName != "" && j.BackupVaultName != vaultName {
			continue
		}
		cp := *j
		list = append(list, &cp)
	}

	slices.SortFunc(list, func(a, b *Job) int {
		// Newest first.
		if a.CreationTime.After(b.CreationTime) {
			return -1
		}
		if a.CreationTime.Before(b.CreationTime) {
			return 1
		}

		return 0
	})

	return list
}

// StopBackupJob cancels a running backup job.
func (b *InMemoryBackend) StopBackupJob(jobID string) error {
	b.mu.Lock("StopBackupJob")
	defer b.mu.Unlock()

	job, ok := b.jobs.Get(jobID)
	if !ok {
		return fmt.Errorf("%w: %s", errBackupJobNotFound, jobID)
	}
	job.State = "ABORTED"

	return nil
}

// ListBackupJobSummaries returns a summary of backup jobs by resource type and status.
func (b *InMemoryBackend) ListBackupJobSummaries() []map[string]any {
	b.mu.RLock("ListBackupJobSummaries")
	defer b.mu.RUnlock()

	// Group by state.
	counts := make(map[string]int)
	for _, j := range b.jobs.All() {
		counts[j.State]++
	}

	summaries := make([]map[string]any, 0, len(counts))
	for status, count := range counts {
		summaries = append(summaries, map[string]any{
			keyState:         status,
			keySummaryCount:  count,
			keySummaryRegion: b.region,
			keyAccountID:     b.accountID,
		})
	}

	return summaries
}

// ListBackupJobsFilter contains optional filter parameters for listing backup jobs.
type ListBackupJobsFilter struct {
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
	VaultName     string
	State         string
	ResourceArn   string
	ResourceType  string
	AccountID     string
	ParentJobID   string
	NextToken     string
	MaxResults    int
}

// jobMatchesFilter reports whether j satisfies all active fields in f.
func jobMatchesFilter(j *Job, f ListBackupJobsFilter) bool {
	switch {
	case f.VaultName != "" && j.BackupVaultName != f.VaultName:
		return false
	case f.State != "" && j.State != f.State:
		return false
	case f.ResourceArn != "" && j.ResourceArn != f.ResourceArn:
		return false
	case f.ResourceType != "" && j.ResourceType != f.ResourceType:
		return false
	// ByAccountId doc (api_op_ListBackupJobs.go): "If used from an
	// Organizations management account, passing * returns all jobs across
	// the organization" -- "*" is a wildcard, not a literal account ID.
	case f.AccountID != "" && f.AccountID != wildcardAccountID && j.AccountID != f.AccountID:
		return false
	case f.ParentJobID != "" && j.ParentJobID != f.ParentJobID:
		return false
	}

	return inTimeRange(j.CreationTime, f.CreatedAfter, f.CreatedBefore)
}

// ListBackupJobsFiltered returns backup jobs matching the filter, with pagination.
// Returns (jobs, nextToken).
//
//nolint:dupl // structurally identical to ListCopyJobsFiltered but operates on a different type
func (b *InMemoryBackend) ListBackupJobsFiltered(f ListBackupJobsFilter) ([]*Job, string) {
	b.mu.RLock("ListBackupJobsFiltered")
	defer b.mu.RUnlock()

	all := b.jobs.All()
	list := make([]*Job, 0, len(all))
	for _, j := range all {
		if !jobMatchesFilter(j, f) {
			continue
		}
		cp := *j
		list = append(list, &cp)
	}

	slices.SortFunc(list, func(a, b *Job) int {
		if d := b.CreationTime.Compare(a.CreationTime); d != 0 {
			return d
		}

		return strings.Compare(a.BackupJobID, b.BackupJobID)
	})

	return paginateByID(
		list,
		func(j *Job) string { return j.BackupJobID },
		f.MaxResults,
		f.NextToken,
	)
}

// ---- ListRecoveryPoints filtering + pagination ----

// CompleteBackupJob transitions a job from CREATED to COMPLETED and creates a recovery point.
// This models AWS's asynchronous job completion in a synchronous way for the emulator.
func (b *InMemoryBackend) CompleteBackupJob(jobID string) error {
	b.mu.Lock("CompleteBackupJob")
	defer b.mu.Unlock()

	job, ok := b.jobs.Get(jobID)
	if !ok {
		return fmt.Errorf("%w: backup job %s not found", ErrNotFound, jobID)
	}
	if job.State != statusCreated {
		return nil // already done
	}

	now := time.Now().UTC()
	job.State = statusCompleted
	job.CompletionTime = &now
	job.PercentDone = "100.0"
	job.MessageCategory = "SUCCESS"
	job.BytesTransferred = completedJobBytes
	job.BackupSizeInBytes = completedJobBytes

	// Build a recovery point ARN.
	rpID := job.BackupJobID
	rpArn := "arn:aws:backup:" + b.region + ":" + b.accountID + ":recovery-point:" + rpID
	job.RecoveryPointArn = rpArn

	vault, ok2 := b.vaults.Get(job.BackupVaultName)
	if !ok2 {
		return nil // vault deleted between job start and completion
	}

	rp := &RecoveryPoint{
		RecoveryPointArn:  rpArn,
		BackupVaultName:   job.BackupVaultName,
		BackupVaultArn:    vault.BackupVaultArn,
		ResourceArn:       job.ResourceArn,
		ResourceType:      job.ResourceType,
		IAMRoleArn:        job.IAMRoleArn,
		Status:            statusCompleted,
		CreationDate:      now,
		CompletionDate:    &now,
		BackupSizeInBytes: completedJobBytes,
		StorageClass:      "WARM",
		IsEncrypted:       vault.EncryptionKeyArn != "",
		EncryptionKeyArn:  vault.EncryptionKeyArn,
	}
	b.recoveryPoints.Put(rp)
	vault.NumberOfRecoveryPoints++

	// Update protected resource record.
	b.protectedResources.Put(&ProtectedResource{
		ResourceArn:     job.ResourceArn,
		ResourceType:    job.ResourceType,
		BackupVaultName: job.BackupVaultName,
		LastBackupTime:  now,
	})

	return nil
}

// ---- CreateBackupPlan / UpdateBackupPlan with rule validation ----

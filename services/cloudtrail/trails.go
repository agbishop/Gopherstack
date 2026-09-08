package cloudtrail

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	sdk_s3 "github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// checkBucketLocked verifies bucket exists via the wired S3 backend. A no-op
// when S3 is unwired (b.s3 == nil), matching this repo's
// unwired-hook-stays-permissive convention. Callers must hold b.mu.
func (b *InMemoryBackend) checkBucketLocked(bucket string) error {
	if b.s3 == nil {
		return nil
	}

	_, err := b.s3.HeadBucket(context.Background(), &sdk_s3.HeadBucketInput{Bucket: aws.String(bucket)})
	if err != nil {
		return fmt.Errorf("%w: bucket %s does not exist", ErrS3BucketNotFound, bucket)
	}

	return nil
}

// CreateTrail creates a new CloudTrail trail.
func (b *InMemoryBackend) CreateTrail(
	name, s3BucketName, s3KeyPrefix, snsTopicName,
	cloudWatchLogsLogGroupARN, cloudWatchLogsRoleARN, kmsKeyID string,
	includeGlobalServiceEvents, isMultiRegionTrail, enableLogFileValidation bool,
	kv map[string]string,
) (*Trail, error) {
	b.mu.Lock("CreateTrail")
	defer b.mu.Unlock()

	if b.trails.Has(name) {
		return nil, fmt.Errorf("%w: trail %s already exists", ErrAlreadyExists, name)
	}

	if err := b.checkBucketLocked(s3BucketName); err != nil {
		return nil, err
	}

	trailARN := arn.Build("cloudtrail", b.region, b.accountID, "trail/"+name)
	t := tags.New("cloudtrail.trail." + name + ".tags")
	if len(kv) > 0 {
		t.Merge(kv)
	}
	var snsTopicARN string
	if snsTopicName != "" {
		snsTopicARN = arn.Build("sns", b.region, b.accountID, snsTopicName)
	}
	trail := &Trail{
		Name:                       name,
		S3BucketName:               s3BucketName,
		S3KeyPrefix:                s3KeyPrefix,
		SnsTopicName:               snsTopicName,
		SnsTopicARN:                snsTopicARN,
		CloudWatchLogsLogGroupARN:  cloudWatchLogsLogGroupARN,
		CloudWatchLogsRoleARN:      cloudWatchLogsRoleARN,
		KMSKeyID:                   kmsKeyID,
		TrailARN:                   trailARN,
		HomeRegion:                 b.region,
		AccountID:                  b.accountID,
		Region:                     b.region,
		IncludeGlobalServiceEvents: includeGlobalServiceEvents,
		IsMultiRegionTrail:         isMultiRegionTrail,
		LogFileValidationEnabled:   enableLogFileValidation,
		IsLogging:                  false,
		CreationTime:               time.Now().UTC(),
		Tags:                       t,
	}
	b.trails.Put(trail)
	cp := *trail

	return &cp, nil
}

// GetTrail returns a trail by name or ARN.
func (b *InMemoryBackend) GetTrail(nameOrARN string) (*Trail, error) {
	b.mu.RLock("GetTrail")
	defer b.mu.RUnlock()

	return b.findTrailLocked(nameOrARN)
}

// findTrailLocked looks up a trail by name or ARN (must hold at least a read lock).
func (b *InMemoryBackend) findTrailLocked(nameOrARN string) (*Trail, error) {
	t := b.findByNameOrARNLocked(nameOrARN)
	if t == nil {
		return nil, fmt.Errorf("%w: trail %s not found", ErrNotFound, nameOrARN)
	}

	cp := *t
	cp.EventSelectors = copyEventSelectors(t.EventSelectors)

	return &cp, nil
}

// DescribeTrails returns trails matching the given name list.
// If nameList is empty, all trails are returned.
func (b *InMemoryBackend) DescribeTrails(nameList []string) []*Trail {
	b.mu.RLock("DescribeTrails")
	defer b.mu.RUnlock()

	if len(nameList) == 0 {
		all := b.trails.All()
		list := make([]*Trail, 0, len(all))
		for _, t := range all {
			cp := *t
			cp.EventSelectors = copyEventSelectors(t.EventSelectors)
			list = append(list, &cp)
		}
		sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })

		return list
	}

	list := make([]*Trail, 0, len(nameList))
	for _, name := range nameList {
		t, err := b.findTrailLocked(name)
		if err == nil {
			list = append(list, t)
		}
	}

	return list
}

// UpdateTrail updates an existing trail's configuration.
func (b *InMemoryBackend) UpdateTrail(
	name, s3BucketName, s3KeyPrefix, snsTopicName,
	cloudWatchLogsLogGroupARN, cloudWatchLogsRoleARN, kmsKeyID string,
	includeGlobalServiceEvents, isMultiRegionTrail, enableLogFileValidation *bool,
) (*Trail, error) {
	b.mu.Lock("UpdateTrail")
	defer b.mu.Unlock()

	t := b.findByNameOrARNLocked(name)
	if t == nil {
		return nil, fmt.Errorf("%w: trail %s not found", ErrNotFound, name)
	}

	if s3BucketName != "" {
		if err := b.checkBucketLocked(s3BucketName); err != nil {
			return nil, err
		}

		t.S3BucketName = s3BucketName
	}
	if s3KeyPrefix != "" {
		t.S3KeyPrefix = s3KeyPrefix
	}
	if snsTopicName != "" {
		t.SnsTopicName = snsTopicName
		t.SnsTopicARN = arn.Build("sns", b.region, b.accountID, snsTopicName)
	}
	if cloudWatchLogsLogGroupARN != "" {
		t.CloudWatchLogsLogGroupARN = cloudWatchLogsLogGroupARN
	}
	if cloudWatchLogsRoleARN != "" {
		t.CloudWatchLogsRoleARN = cloudWatchLogsRoleARN
	}
	if kmsKeyID != "" {
		t.KMSKeyID = kmsKeyID
	}
	if includeGlobalServiceEvents != nil {
		t.IncludeGlobalServiceEvents = *includeGlobalServiceEvents
	}
	if isMultiRegionTrail != nil {
		t.IsMultiRegionTrail = *isMultiRegionTrail
	}
	if enableLogFileValidation != nil {
		t.LogFileValidationEnabled = *enableLogFileValidation
	}

	cp := *t
	cp.EventSelectors = copyEventSelectors(t.EventSelectors)

	return &cp, nil
}

// DeleteTrail deletes a trail by name or ARN.
func (b *InMemoryBackend) DeleteTrail(nameOrARN string) error {
	b.mu.Lock("DeleteTrail")
	defer b.mu.Unlock()

	t := b.findByNameOrARNLocked(nameOrARN)
	if t == nil {
		return fmt.Errorf("%w: trail %s not found", ErrNotFound, nameOrARN)
	}

	t.Tags.Close()
	b.trails.Delete(t.Name)
	delete(b.eventConfigs, t.TrailARN)
	b.resourcePolicies.Delete(t.TrailARN)

	return nil
}

// StartLogging sets the isLogging flag for a trail to true and records the start time.
func (b *InMemoryBackend) StartLogging(nameOrARN string) error {
	b.mu.Lock("StartLogging")
	defer b.mu.Unlock()

	t := b.findByNameOrARNLocked(nameOrARN)
	if t == nil {
		return fmt.Errorf("%w: trail %s not found", ErrNotFound, nameOrARN)
	}
	now := time.Now().UTC()
	t.IsLogging = true
	t.StartLoggingTime = &now
	t.LatestDeliveryTime = &now

	return nil
}

// StopLogging sets the isLogging flag for a trail to false and records the stop time.
func (b *InMemoryBackend) StopLogging(nameOrARN string) error {
	b.mu.Lock("StopLogging")
	defer b.mu.Unlock()

	t := b.findByNameOrARNLocked(nameOrARN)
	if t == nil {
		return fmt.Errorf("%w: trail %s not found", ErrNotFound, nameOrARN)
	}
	now := time.Now().UTC()
	t.IsLogging = false
	t.StopLoggingTime = &now

	return nil
}

// GetTrailStatus returns the full logging status of a trail.
func (b *InMemoryBackend) GetTrailStatus(nameOrARN string) (*Trail, error) {
	b.mu.RLock("GetTrailStatus")
	defer b.mu.RUnlock()

	t := b.findByNameOrARNLocked(nameOrARN)
	if t == nil {
		return nil, fmt.Errorf("%w: trail %s not found", ErrNotFound, nameOrARN)
	}
	cp := *t

	return &cp, nil
}

// ListTrails returns all trails.
func (b *InMemoryBackend) ListTrails() []*Trail {
	b.mu.RLock("ListTrails")
	defer b.mu.RUnlock()

	all := b.trails.All()
	list := make([]*Trail, 0, len(all))
	for _, t := range all {
		cp := *t
		cp.EventSelectors = copyEventSelectors(t.EventSelectors)
		list = append(list, &cp)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].TrailARN < list[j].TrailARN })

	return list
}

// DeregisterOrganizationDelegatedAdmin deregisters an organization delegated admin account.
// This is a no-op in the in-memory backend (returns success).
func (b *InMemoryBackend) DeregisterOrganizationDelegatedAdmin(delegatedAdminAccountID string) error {
	if delegatedAdminAccountID == "" {
		return fmt.Errorf("%w: DelegatedAdminAccountId is required", ErrValidation)
	}

	return nil
}

// RegisterOrganizationDelegatedAdmin is a no-op that registers an org delegated admin.
func (b *InMemoryBackend) RegisterOrganizationDelegatedAdmin(accountID string) error {
	if accountID == "" {
		return fmt.Errorf("%w: MemberAccountId is required", ErrValidation)
	}

	return nil
}

// ListPublicKeys returns empty public keys (stub).
func (b *InMemoryBackend) ListPublicKeys() []map[string]any {
	return []map[string]any{}
}

package datasync

import (
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// CreateLocationS3 creates a new S3 location.
func (b *InMemoryBackend) CreateLocationS3(
	subdirectory, s3BucketArn, s3StorageClass string,
	s3Config S3Config,
	agentArns []string,
	tags map[string]string,
) (*Location, error) {
	b.mu.Lock("CreateLocationS3")
	defer b.mu.Unlock()

	if err := b.validateAgentArns(agentArns); err != nil {
		return nil, err
	}

	id := newID()
	locationArn := b.locationARN(id)
	now := time.Now().UTC()

	// Build S3 URI: s3://<bucket-name>/<subdirectory>
	bucketName := extractBucketName(s3BucketArn)
	sub := strings.TrimPrefix(subdirectory, "/")
	locationURI := fmt.Sprintf("s3://%s/%s", bucketName, sub)

	locationTags := make(map[string]string)
	maps.Copy(locationTags, tags)

	storedCfg := &storedS3Config{BucketAccessRoleArn: s3Config.BucketAccessRoleArn, AgentArns: agentArns}

	l := &storedLocation{
		LocationArn:    locationArn,
		LocationURI:    locationURI,
		S3BucketArn:    s3BucketArn,
		Subdirectory:   subdirectory,
		S3StorageClass: s3StorageClass,
		S3Config:       storedCfg,
		LocationType:   "S3",
		CreationTime:   now,
		Tags:           locationTags,
	}
	b.locations.Put(l)

	if len(locationTags) > 0 {
		b.tags[locationArn] = make(map[string]string)
		maps.Copy(b.tags[locationArn], locationTags)
	}

	cp := l.toLocation()

	return &cp, nil
}

// DescribeLocationS3 returns S3 location details.
func (b *InMemoryBackend) DescribeLocationS3(locationArn string) (*LocationS3, error) {
	b.mu.RLock("DescribeLocationS3")
	defer b.mu.RUnlock()

	l, ok := b.locations.Get(locationArn)
	if !ok {
		return nil, ErrNotFound
	}

	if l.LocationType != "S3" {
		return nil, ErrNotFound
	}

	cp := l.toLocationS3()

	return &cp, nil
}

// DeleteLocation deletes a location.
func (b *InMemoryBackend) DeleteLocation(locationArn string) error {
	b.mu.Lock("DeleteLocation")
	defer b.mu.Unlock()

	if !b.locations.Has(locationArn) {
		return ErrNotFound
	}

	b.locations.Delete(locationArn)
	delete(b.tags, locationArn)

	return nil
}

// ListLocations returns locations, sorted by ARN.
func (b *InMemoryBackend) ListLocations(
	filters []LocationFilter,
	maxResults int32,
	nextToken string,
) ([]*LocationListEntry, string, error) {
	b.mu.RLock("ListLocations")
	defer b.mu.RUnlock()

	sorted := b.locations.Snapshot()

	all := make([]*LocationListEntry, 0, len(sorted))
	for _, l := range sorted {
		matched, err := matchLocationFilters(l, filters)
		if err != nil {
			return nil, "", err
		}

		if !matched {
			continue
		}

		all = append(all, &LocationListEntry{
			LocationArn:  l.LocationArn,
			LocationURI:  l.LocationURI,
			CreationTime: l.CreationTime,
		})
	}

	limit := int(maxResults)
	pg := page.New(all, nextToken, limit, defaultMaxResults)

	return pg.Data, pg.Next, nil
}

// matchLocationFilters reports whether l satisfies every filter (AND across
// filters, per the shared AWS list-filter convention).
func matchLocationFilters(l *storedLocation, filters []LocationFilter) (bool, error) {
	for _, f := range filters {
		var actual string

		switch f.Name {
		case "LocationUri":
			actual = l.LocationURI
		case "LocationType":
			actual = l.LocationType
		case "CreationTime":
			actual = l.CreationTime.UTC().Format(time.RFC3339)
		default:
			return false, fmt.Errorf("%w: unrecognized filter Name %q", ErrInvalidParameter, f.Name)
		}

		matched, err := matchFilterOperator(f.Operator, actual, f.Values)
		if err != nil {
			return false, err
		}

		if !matched {
			return false, nil
		}
	}

	return true, nil
}

// UpdateLocationS3 updates an S3 location's subdirectory, storage class, and S3 config.
func (b *InMemoryBackend) UpdateLocationS3(locationArn, subdirectory, s3StorageClass string, s3Config S3Config) error {
	b.mu.Lock("UpdateLocationS3")
	defer b.mu.Unlock()

	l, ok := b.locations.Get(locationArn)
	if !ok || l.LocationType != "S3" {
		return ErrNotFound
	}

	if subdirectory != "" {
		l.Subdirectory = subdirectory
		bucketName := extractBucketName(l.S3BucketArn)
		sub := strings.TrimPrefix(subdirectory, "/")
		l.LocationURI = fmt.Sprintf("s3://%s/%s", bucketName, sub)
	}

	l.S3StorageClass = s3StorageClass
	l.S3Config = &storedS3Config{BucketAccessRoleArn: s3Config.BucketAccessRoleArn}

	return nil
}

// extractBucketName extracts the bucket name from an S3 ARN.
// Format: arn:aws:s3:::bucket-name or arn:aws:s3:::bucket-name/prefix.
func extractBucketName(s3BucketArn string) string {
	// S3 ARNs: arn:aws:s3:::bucket-name
	parts := strings.SplitN(s3BucketArn, ":::", arnSplitParts)
	if len(parts) == arnSplitParts {
		name, _, _ := strings.Cut(parts[1], "/")
		if name != "" {
			return name
		}
	}

	return "unknown-bucket"
}

package lightsail

// This file backs family S (10 ops: CreateBucket, DeleteBucket, UpdateBucket,
// UpdateBucketBundle, GetBuckets, SetResourceAccessForBucket,
// GetBucketMetricData, CreateBucketAccessKey, DeleteBucketAccessKey,
// GetBucketAccessKeys) -- Lightsail's own object storage, modeled
// independently of this repo's real services/s3 (PARITY.md 4.6/S).

import (
	"fmt"
	"slices"
	"sort"

	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

const (
	opTypeCreateBucket               = "CreateBucket"
	opTypeDeleteBucket               = "DeleteBucket"
	opTypeUpdateBucketBundle         = "UpdateBucketBundle"
	opTypeSetResourceAccessForBucket = "SetResourceAccessForBucket"
	opTypeCreateBucketAccessKey      = "CreateBucketAccessKey"
	opTypeDeleteBucketAccessKey      = "DeleteBucketAccessKey"
)

// CreateBucket creates a new Lightsail-native storage bucket.
func (b *InMemoryBackend) CreateBucket(
	name, bundleID string,
	enableObjectVersioning bool,
	userTags map[string]string,
) ([]Operation, error) {
	rdsBd, ok := findBucketBundle(bundleID)
	if !ok {
		return nil, validationError("unknown BundleId: " + bundleID)
	}

	b.mu.Lock("CreateBucket")
	defer b.mu.Unlock()

	if err := b.registerNameLocked(ResourceTypeBucket, name); err != nil {
		return nil, err
	}

	versioning := ObjectVersioningNeverEnabled
	if enableObjectVersioning {
		versioning = ObjectVersioningEnabled
	}

	bk := &Bucket{
		Name:               name,
		Arn:                b.regionalARN(ResourceTypeBucket, newUUID()),
		SupportCode:        newSupportCode(),
		BundleID:           rdsBd.BundleID,
		State:              BucketStateOK,
		ObjectVersioning:   versioning,
		URL:                name + ".s3-website." + b.region + ".amazonaws.com",
		CreatedAt:          nowUTC(),
		Location:           ResourceLocation{RegionName: b.region, AvailabilityZone: availabilityZoneA(b.region)},
		AbleToUpdateBundle: true,
		Tags:               tags.New("lightsail.bucket." + name + ".tags"),
	}
	bk.Tags.Merge(userTags)
	b.buckets.Put(bk)

	return b.newOperationsLocked(opTypeCreateBucket, ResourceTypeBucket, []string{name}), nil
}

func findBucketBundle(id string) (*BucketBundle, bool) {
	for _, bd := range seedBucketBundles {
		if bd.BundleID == id {
			return &bd, true
		}
	}

	return nil, false
}

// DeleteBucket deletes the named bucket. ForceDelete's own doc comment
// (api_op_DeleteBucket.go) lists four conditions requiring it: the bucket
// is a distribution's origin, an instance/container service was granted
// access to it, it has objects, or it has access keys -- this backend
// checks the first, second, and fourth (it does not model bucket object
// contents at all, PARITY.md family S's 10-op list has no object
// operations, so "has objects" cannot be evaluated).
func (b *InMemoryBackend) DeleteBucket(name string, forceDelete bool) ([]Operation, error) {
	b.mu.Lock("DeleteBucket")
	defer b.mu.Unlock()

	bk, ok := b.buckets.Get(name)
	if !ok {
		return nil, notFoundError("Bucket", name)
	}

	if !forceDelete {
		if len(bk.ReadonlyAccessAccounts) > 0 {
			return nil, validationError("bucket has readonly access accounts; set ForceDelete to delete anyway")
		}

		if len(bk.AccessKeys) > 0 {
			return nil, validationError("bucket has access keys; set ForceDelete to delete anyway")
		}

		for _, d := range b.distributions.All() {
			if d.Origin.Name == name {
				return nil, validationError(
					fmt.Sprintf("bucket is the origin of distribution %s; set ForceDelete to delete anyway", d.Name),
				)
			}
		}
	}

	if bk.Tags != nil {
		bk.Tags.Close()
	}

	b.buckets.Delete(name)
	b.unregisterNameLocked(name)

	return b.newOperationsLocked(opTypeDeleteBucket, ResourceTypeBucket, []string{name}), nil
}

// UpdateBucket updates the named bucket's versioning/readonly-access-accounts.
func (b *InMemoryBackend) UpdateBucket(
	name, versioning string,
	readonlyAccessAccounts []string,
) (*Bucket, []Operation, error) {
	b.mu.Lock("UpdateBucket")
	defer b.mu.Unlock()

	bk, ok := b.buckets.Get(name)
	if !ok {
		return nil, nil, notFoundError("Bucket", name)
	}

	if versioning != "" {
		bk.ObjectVersioning = versioning
	}

	if readonlyAccessAccounts != nil {
		bk.ReadonlyAccessAccounts = readonlyAccessAccounts
	}

	return bk.clone(), b.newOperationsLocked("UpdateBucket", ResourceTypeBucket, []string{name}), nil
}

// UpdateBucketBundle changes the named bucket's bundle tier.
func (b *InMemoryBackend) UpdateBucketBundle(name, bundleID string) ([]Operation, error) {
	b.mu.Lock("UpdateBucketBundle")
	defer b.mu.Unlock()

	bk, ok := b.buckets.Get(name)
	if !ok {
		return nil, notFoundError("Bucket", name)
	}

	if _, bundleOK := findBucketBundle(bundleID); !bundleOK {
		return nil, validationError("unknown BundleId: " + bundleID)
	}

	bk.BundleID = bundleID

	return b.newOperationsLocked(opTypeUpdateBucketBundle, ResourceTypeBucket, []string{name}), nil
}

// GetBuckets returns the named bucket, or every bucket if name is empty.
func (b *InMemoryBackend) GetBuckets(name string) ([]*Bucket, error) {
	b.mu.RLock("GetBuckets")
	defer b.mu.RUnlock()

	if name != "" {
		bk, ok := b.buckets.Get(name)
		if !ok {
			return nil, notFoundError("Bucket", name)
		}

		return []*Bucket{bk.clone()}, nil
	}

	all := b.buckets.All()
	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })

	out := make([]*Bucket, len(all))
	for i, v := range all {
		out[i] = v.clone()
	}

	return out, nil
}

// SetResourceAccessForBucket grants or revokes resourceName's (an Instance
// or ContainerService) access to the named bucket.
func (b *InMemoryBackend) SetResourceAccessForBucket(resourceName, bucketName, access string) ([]Operation, error) {
	b.mu.Lock("SetResourceAccessForBucket")
	defer b.mu.Unlock()

	bk, ok := b.buckets.Get(bucketName)
	if !ok {
		return nil, notFoundError("Bucket", bucketName)
	}

	kind, known := b.activeNames[resourceName]
	if !known || (kind != ResourceTypeInstance && kind != ResourceTypeContainerService) {
		return nil, notFoundError("Instance or ContainerService", resourceName)
	}

	if access == "allow" {
		bk.ReadonlyAccessAccounts = appendUnique(bk.ReadonlyAccessAccounts, resourceName)
	} else {
		bk.ReadonlyAccessAccounts = removeString(bk.ReadonlyAccessAccounts, resourceName)
	}

	return b.newOperationsLocked(opTypeSetResourceAccessForBucket, ResourceTypeBucket, []string{bucketName}), nil
}

func appendUnique(in []string, s string) []string {
	if slices.Contains(in, s) {
		return in
	}

	return append(in, s)
}

// GetBucketMetricData returns a real, well-formed, EMPTY MetricData
// response -- one of the six honestly-unfakeable telemetry ops
// (PARITY.md 4.10).
func (b *InMemoryBackend) GetBucketMetricData(name string) error {
	b.mu.RLock("GetBucketMetricData")
	defer b.mu.RUnlock()

	if _, ok := b.buckets.Get(name); !ok {
		return notFoundError("Bucket", name)
	}

	return nil
}

// CreateBucketAccessKey creates a new access key for the named bucket,
// returning the secret in full exactly once (never retrievable again,
// PARITY.md 4.6 -- same write-once pattern as CreateKeyPair).
func (b *InMemoryBackend) CreateBucketAccessKey(bucketName string) (*AccessKey, []Operation, error) {
	b.mu.Lock("CreateBucketAccessKey")
	defer b.mu.Unlock()

	bk, ok := b.buckets.Get(bucketName)
	if !ok {
		return nil, nil, notFoundError("Bucket", bucketName)
	}

	b.nextAccessKeySeq++
	key := AccessKey{
		AccessKeyID: "LSAK" + randomHex() + randomHex(), SecretAccessKey: newSupportCode() + randomHex(),
		Status: AccessKeyStatusActive, CreatedAt: nowUTC(),
	}
	bk.AccessKeys = append(bk.AccessKeys, key)

	return &key, b.newOperationsLocked(opTypeCreateBucketAccessKey, ResourceTypeBucket, []string{bucketName}), nil
}

// DeleteBucketAccessKey deletes the named bucket's access key.
func (b *InMemoryBackend) DeleteBucketAccessKey(bucketName, accessKeyID string) ([]Operation, error) {
	b.mu.Lock("DeleteBucketAccessKey")
	defer b.mu.Unlock()

	bk, ok := b.buckets.Get(bucketName)
	if !ok {
		return nil, notFoundError("Bucket", bucketName)
	}

	out := make([]AccessKey, 0, len(bk.AccessKeys))

	for _, k := range bk.AccessKeys {
		if k.AccessKeyID != accessKeyID {
			out = append(out, k)
		}
	}

	bk.AccessKeys = out

	return b.newOperationsLocked(opTypeDeleteBucketAccessKey, ResourceTypeBucket, []string{bucketName}), nil
}

// GetBucketAccessKeys returns the named bucket's access keys (metadata
// only -- SecretAccessKey is never included here, matching real AWS's
// write-once-readable secret, PARITY.md 4.6).
func (b *InMemoryBackend) GetBucketAccessKeys(bucketName string) ([]AccessKey, error) {
	b.mu.RLock("GetBucketAccessKeys")
	defer b.mu.RUnlock()

	bk, ok := b.buckets.Get(bucketName)
	if !ok {
		return nil, notFoundError("Bucket", bucketName)
	}

	out := make([]AccessKey, len(bk.AccessKeys))
	for i, k := range bk.AccessKeys {
		k.SecretAccessKey = ""
		out[i] = k
	}

	return out, nil
}

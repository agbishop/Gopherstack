package dms

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

func isValidNetworkType(s string) bool {
	return s == "" || s == "IPV4" || s == "IPV6" || s == "DUAL"
}

// CreateInstanceProfile creates a new instance profile.
func (b *InMemoryBackend) CreateInstanceProfile(
	ctx context.Context,
	instanceProfileName, availabilityZone, kmsKeyArn, networkType, description, subnetGroupIdentifier string,
	publiclyAccessible bool,
	kv map[string]string,
) (*InstanceProfile, error) {
	b.mu.Lock("CreateInstanceProfile")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	key := instanceProfileName
	if key == "" {
		key = uuid.NewString()
	}

	if b.instanceProfiles.Has(regionKey(region, key)) {
		return nil, fmt.Errorf("%w: instance profile %s already exists", ErrAlreadyExists, key)
	}

	if !isValidNetworkType(networkType) {
		return nil, fmt.Errorf(
			"%w: invalid NetworkType %q; valid: IPV4, IPV6, DUAL",
			ErrValidation,
			networkType,
		)
	}

	profileARN := arn.Build("dms", region, b.accountID, "instance-profile:"+uuid.NewString())
	t := tags.New("dms.instance-profile." + key + ".tags")
	if len(kv) > 0 {
		t.Merge(kv)
	}

	if instanceProfileName == "" {
		instanceProfileName = key
	}

	ip := &InstanceProfile{
		InstanceProfileName:   instanceProfileName,
		InstanceProfileArn:    profileARN,
		AvailabilityZone:      availabilityZone,
		KmsKeyArn:             kmsKeyArn,
		NetworkType:           networkType,
		Description:           description,
		SubnetGroupIdentifier: subnetGroupIdentifier,
		PubliclyAccessible:    publiclyAccessible,
		AccountID:             b.accountID,
		Region:                region,
		CreationTime:          time.Now().UTC(),
		Tags:                  t,
	}
	b.instanceProfiles.Put(ip)
	cp := *ip

	return &cp, nil
}

// AddInstanceProfileInternal seeds an instance profile directly without HTTP.
func (b *InMemoryBackend) AddInstanceProfileInternal(name string) {
	b.mu.Lock("AddInstanceProfileInternal")
	defer b.mu.Unlock()
	if name == "" {
		name = uuid.NewString()
	}
	profileARN := arn.Build("dms", b.region, b.accountID, "instance-profile:"+uuid.NewString())
	t := tags.New("dms.instance-profile." + name + ".tags")
	ip := &InstanceProfile{
		InstanceProfileName: name,
		InstanceProfileArn:  profileARN,
		AccountID:           b.accountID,
		Region:              b.region,
		CreationTime:        time.Now().UTC(),
		Tags:                t,
	}
	b.instanceProfiles.Put(ip)
}

// DeleteInstanceProfile deletes an instance profile by name or ARN. Real
// AWS: "All migration projects associated with the instance profile must be
// deleted or modified before you can delete the instance profile".
func (b *InMemoryBackend) DeleteInstanceProfile(ctx context.Context, nameOrArn string) error {
	b.mu.Lock("DeleteInstanceProfile")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	if ip, ok := b.instanceProfiles.Get(regionKey(region, nameOrArn)); ok {
		if b.migrationProjectUsesInstanceProfileLocked(region, ip.InstanceProfileArn) {
			return fmt.Errorf("%w: instance profile %s has associated migration projects", ErrInvalidState, nameOrArn)
		}

		ip.Tags.Close()
		b.instanceProfiles.Delete(regionKey(region, nameOrArn))

		return nil
	}

	if ip, ok := lookupUnique(b.instanceProfilesByARN, regionKey(region, nameOrArn)); ok {
		if b.migrationProjectUsesInstanceProfileLocked(region, ip.InstanceProfileArn) {
			return fmt.Errorf("%w: instance profile %s has associated migration projects", ErrInvalidState, nameOrArn)
		}

		ip.Tags.Close()
		b.instanceProfiles.Delete(regionKey(region, ip.InstanceProfileName))

		return nil
	}

	return fmt.Errorf("%w: instance profile %s not found", ErrNotFound, nameOrArn)
}

// migrationProjectUsesInstanceProfileLocked reports whether any migration
// project in region references instanceProfileArn. Caller must hold b.mu.
func (b *InMemoryBackend) migrationProjectUsesInstanceProfileLocked(region, instanceProfileArn string) bool {
	for _, mp := range b.migrationProjectsByRegion.Get(region) {
		if mp.InstanceProfileArn == instanceProfileArn {
			return true
		}
	}

	return false
}

// ModifyInstanceProfile updates an instance profile. Real AWS: "All migration
// projects associated with the instance profile must be deleted or modified
// before you can modify the instance profile" (databasemigrationservice@v1.66.4
// api_op_ModifyInstanceProfile.go:16-17).
func (b *InMemoryBackend) ModifyInstanceProfile(
	ctx context.Context,
	nameOrArn, availabilityZone, description, networkType string,
) (*InstanceProfile, error) {
	b.mu.Lock("ModifyInstanceProfile")
	defer b.mu.Unlock()

	ip := b.findInstanceProfile(ctx, nameOrArn)
	if ip == nil {
		return nil, fmt.Errorf("%w: instance profile %s not found", ErrNotFound, nameOrArn)
	}

	region := getRegion(ctx, b.region)
	if b.migrationProjectUsesInstanceProfileLocked(region, ip.InstanceProfileArn) {
		return nil, fmt.Errorf(
			"%w: instance profile %s has associated migration projects",
			ErrInvalidState,
			nameOrArn,
		)
	}

	if availabilityZone != "" {
		ip.AvailabilityZone = availabilityZone
	}

	if description != "" {
		ip.Description = description
	}

	if networkType != "" {
		ip.NetworkType = networkType
	}

	cp := *ip

	return &cp, nil
}

// findInstanceProfile locates an instance profile by name or ARN within the
// request region (must hold a lock).
func (b *InMemoryBackend) findInstanceProfile(ctx context.Context, nameOrArn string) *InstanceProfile {
	region := getRegion(ctx, b.region)
	if ip, ok := b.instanceProfiles.Get(regionKey(region, nameOrArn)); ok {
		return ip
	}

	if ip, ok := lookupUnique(b.instanceProfilesByARN, regionKey(region, nameOrArn)); ok {
		return ip
	}

	return nil
}

// DescribeInstanceProfiles returns all instance profiles.
func (b *InMemoryBackend) DescribeInstanceProfiles(ctx context.Context) ([]*InstanceProfile, error) {
	b.mu.RLock("DescribeInstanceProfiles")
	defer b.mu.RUnlock()

	items := b.instanceProfilesByRegion.Get(getRegion(ctx, b.region))
	list := make([]*InstanceProfile, 0, len(items))
	for _, ip := range items {
		cp := *ip
		list = append(list, &cp)
	}

	return list, nil
}

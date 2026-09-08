package secretsmanager

import (
	"context"
	"fmt"
	"strings"
)

// replicationStatusInSync is the status used for in-sync replicas.
const (
	replicationStatusFailed     = "Failed"
	replicationStatusInProgress = "InProgress"
	replicationStatusInSync     = "InSync"
)

// ReplicateSecretToRegions adds replication configuration for the specified regions.
func (b *InMemoryBackend) ReplicateSecretToRegions(
	ctx context.Context,
	input *ReplicateSecretToRegionsInput,
) (*ReplicateSecretToRegionsOutput, error) {
	region := getRegion(ctx, b.region)

	b.mu.Lock("ReplicateSecretToRegions")
	defer b.mu.Unlock()

	name := resolveSecretID(input.SecretID)

	secret, ok := b.secretGet(region, name)
	if !ok {
		return nil, ErrSecretNotFound
	}

	if secret.DeletedDate != nil {
		return nil, fmt.Errorf("%w: secret %s is deleted", ErrSecretDeleted, input.SecretID)
	}

	configs := b.replicationConfigsStore(region)
	existing := configs[name]
	existingByRegion := make(map[string]int, len(existing))

	for i, r := range existing {
		existingByRegion[r.Region] = i
	}

	for _, replica := range input.AddReplicaRegions {
		if _, found := existingByRegion[replica.Region]; found && !input.ForceOverwriteReplicaSecret {
			return nil, fmt.Errorf(
				"%w: a replica already exists in region %s; use ForceOverwriteReplicaSecret to overwrite",
				ErrReplicaAlreadyExists, replica.Region,
			)
		}

		status := ReplicationStatusType{
			Region:        replica.Region,
			KmsKeyID:      replica.KmsKeyID,
			Status:        replicationStatusInProgress,
			StatusMessage: "replication queued",
		}

		if idx, found := existingByRegion[replica.Region]; found {
			existing[idx] = status
		} else {
			existing = append(existing, status)
		}
	}

	configs[name] = existing
	b.syncReplicationStatusLocked(region, secret)

	return &ReplicateSecretToRegionsOutput{
		ARN:               secret.ARN,
		ReplicationStatus: configs[name],
	}, nil
}

// RemoveRegionsFromReplication removes replication configuration for the specified regions.
func (b *InMemoryBackend) RemoveRegionsFromReplication(
	ctx context.Context,
	input *RemoveRegionsFromReplicationInput,
) (*RemoveRegionsFromReplicationOutput, error) {
	region := getRegion(ctx, b.region)

	b.mu.Lock("RemoveRegionsFromReplication")
	defer b.mu.Unlock()

	name := resolveSecretID(input.SecretID)

	secret, ok := b.secretGet(region, name)
	if !ok {
		return nil, ErrSecretNotFound
	}

	if secret.DeletedDate != nil {
		return nil, fmt.Errorf("%w: secret %s is deleted", ErrSecretDeleted, input.SecretID)
	}

	toRemove := make(map[string]struct{}, len(input.RemoveReplicaRegions))

	for _, r := range input.RemoveReplicaRegions {
		toRemove[r] = struct{}{}
	}

	configs := b.replicationConfigsStore(region)
	existing := configs[name]
	remaining := make([]ReplicationStatusType, 0, len(existing))

	for _, r := range existing {
		if _, remove := toRemove[r.Region]; !remove {
			remaining = append(remaining, r)

			continue
		}

		// The replica is no longer configured: it must stop being
		// independently readable, matching real AWS -- RemoveRegionsFromReplication
		// removes the replica secret from the target Region.
		b.secretDelete(r.Region, name)
	}

	setReplicationStatuses(configs, name, remaining)

	return &RemoveRegionsFromReplicationOutput{
		ARN:               secret.ARN,
		ReplicationStatus: remaining,
	}, nil
}

// StopReplicationToReplica promotes a replica secret to a standalone secret.
func (b *InMemoryBackend) StopReplicationToReplica(
	ctx context.Context,
	input *StopReplicationToReplicaInput,
) (*StopReplicationToReplicaOutput, error) {
	region := getRegion(ctx, b.region)

	b.mu.Lock("StopReplicationToReplica")
	defer b.mu.Unlock()

	name := resolveSecretID(input.SecretID)

	secret, ok := b.secretGet(region, name)
	if !ok {
		return nil, ErrSecretNotFound
	}

	if secret.DeletedDate != nil {
		return nil, fmt.Errorf("%w: secret %s is deleted", ErrSecretDeleted, input.SecretID)
	}

	if secret.PrimaryRegion != "" {
		// Real semantics: "Removes the link between the replica secret and
		// the primary secret and promotes the replica to a primary secret in
		// the replica Region. You must call this operation from the Region
		// in which you want to promote the replica" (StopReplicationToReplica
		// doc comment). secret here IS the replica (found under region, the
		// calling context's region) -- detach it and drop it from the
		// primary's outgoing replication config.
		primaryRegion := secret.PrimaryRegion
		secret.PrimaryRegion = ""

		if cfgs, cfgsOK := b.replicationConfigs[primaryRegion]; cfgsOK {
			statuses := cfgs[secret.Name]
			remaining := make([]ReplicationStatusType, 0, len(statuses))

			for _, st := range statuses {
				if st.Region != region {
					remaining = append(remaining, st)
				}
			}

			setReplicationStatuses(cfgs, secret.Name, remaining)
		}
	} else {
		// Called against the primary itself: drop all of its outgoing
		// replication configuration (and the replica secrets it mirrored).
		for _, st := range b.replicationConfigsStore(region)[name] {
			b.secretDelete(st.Region, name)
		}

		delete(b.replicationConfigsStore(region), name)
	}

	return &StopReplicationToReplicaOutput{
		ARN: secret.ARN,
	}, nil
}

// setReplicationStatuses stores remaining under name in configs, or deletes
// the entry entirely when remaining is empty -- otherwise a replication
// config key that has had all its regions removed lingers as an
// always-present-but-empty slice, which is both a minor unbounded-growth
// leak and makes ReplicationConfigCount/[InMemoryBackend.replicationConfigs]
// report a config existing when the secret is not actually replicated
// anywhere.
func setReplicationStatuses(
	configs map[string][]ReplicationStatusType, name string, remaining []ReplicationStatusType,
) {
	if len(remaining) == 0 {
		delete(configs, name)

		return
	}

	configs[name] = remaining
}

func (b *InMemoryBackend) syncReplicationStatusLocked(region string, secret *Secret) {
	configs := b.replicationConfigsStore(region)
	statuses, exists := configs[secret.Name]
	if !exists || len(statuses) == 0 {
		return
	}

	currentVer := b.findVersion(secret, "", StagingLabelCurrent)
	if currentVer == nil {
		for i := range statuses {
			statuses[i].Status = replicationStatusFailed
			statuses[i].StatusMessage = "no current secret version to replicate"
		}
		configs[secret.Name] = statuses

		return
	}

	for i := range statuses {
		statuses[i].Status = replicationStatusInSync
		statuses[i].StatusMessage = "replicated version " + currentVer.VersionID
		b.upsertReplicaSecretLocked(secret, statuses[i].Region, statuses[i].KmsKeyID)
	}

	configs[secret.Name] = statuses
}

// upsertReplicaSecretLocked mirrors primary's current version set into a
// real, independently GetSecretValue/DescribeSecret-able Secret stored under
// replicaRegion. Without this, a configured replica region was only ever
// tracked as ReplicationStatusType bookkeeping -- ReplicateSecretToRegions
// looked and echoed real, but a client switching to the replica region got
// ResourceNotFoundException instead of the replicated value. Must be called
// with b.mu held.
func (b *InMemoryBackend) upsertReplicaSecretLocked(primary *Secret, replicaRegion, kmsKeyID string) {
	if kmsKeyID == "" {
		kmsKeyID = primary.KmsKeyID
	}

	b.secretPut(&Secret{
		region:           replicaRegion,
		ARN:              replicaARN(primary.ARN, replicaRegion),
		Name:             primary.Name,
		Description:      primary.Description,
		KmsKeyID:         kmsKeyID,
		Type:             primary.Type,
		PrimaryRegion:    primary.primaryRegionOrSelf(),
		CreatedDate:      primary.CreatedDate,
		CurrentVersionID: primary.CurrentVersionID,
		Versions:         cloneSecretVersionsForReplica(primary.Versions),
	})
}

// replicaARN returns primaryARN with its region segment swapped to
// replicaRegion. Real AWS keeps everything else identical: "The replica ARN
// is the same as the original primary secret ARN expect the Region is
// changed to the replica Region" (api_op_StopReplicationToReplica.go's
// StopReplicationToReplicaInput.SecretId doc comment,
// aws-sdk-go-v2/service/secretsmanager@v1.44.4). Secret names cannot contain
// ":" (see secretNamePattern), so a plain colon split is safe.
func replicaARN(primaryARN, replicaRegion string) string {
	const arnRegionIndex = 3

	parts := strings.Split(primaryARN, ":")
	if len(parts) < arnMinParts {
		return primaryARN
	}

	parts[arnRegionIndex] = replicaRegion

	return strings.Join(parts, ":")
}

// cloneSecretVersionsForReplica deep-copies a primary secret's version set
// for storage under an independent replica Secret, so later mutation of the
// primary's versions (new SecretVersion structs are always allocated fresh,
// never mutated in place -- see rotateSecretLocked/putSecretValueLocked)
// cannot alias into the replica's copy.
func cloneSecretVersionsForReplica(versions map[string]*SecretVersion) map[string]*SecretVersion {
	cloned := make(map[string]*SecretVersion, len(versions))

	for id, v := range versions {
		cp := *v
		cp.StagingLabels = append([]string(nil), v.StagingLabels...)
		cp.KmsKeyIDs = append([]string(nil), v.KmsKeyIDs...)
		cp.SecretBinary = append([]byte(nil), v.SecretBinary...)
		cp.Ciphertext = append([]byte(nil), v.Ciphertext...)
		cloned[id] = &cp
	}

	return cloned
}

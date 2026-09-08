package dms

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// CreateReplicationConfig creates a replication config.
func (b *InMemoryBackend) CreateReplicationConfig(
	ctx context.Context,
	params CreateReplicationConfigParams,
	kv map[string]string,
) (*ReplicationConfig, error) {
	b.mu.Lock("CreateReplicationConfig")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	if b.replicationConfigs.Has(regionKey(region, params.Identifier)) {
		return nil, fmt.Errorf(
			"%w: replication config %s already exists",
			ErrAlreadyExists,
			params.Identifier,
		)
	}

	configARN := arn.Build("dms", region, b.accountID, "replication-config:"+uuid.NewString())
	t := tags.New("dms.replication-config." + params.Identifier + ".tags")
	if len(kv) > 0 {
		t.Merge(kv)
	}
	rc := &ReplicationConfig{
		ReplicationConfigIdentifier: params.Identifier,
		ReplicationConfigArn:        configARN,
		ReplicationType:             params.ReplicationType,
		SourceEndpointArn:           params.SourceEndpointArn,
		TargetEndpointArn:           params.TargetEndpointArn,
		TableMappings:               params.TableMappings,
		ComputeConfig:               params.ComputeConfig,
		AccountID:                   b.accountID,
		Region:                      region,
		Status:                      statusCreated,
		Tags:                        t,
	}
	b.replicationConfigs.Put(rc)
	cp := *rc

	return &cp, nil
}

// DeleteReplicationConfig deletes a replication config by identifier or ARN.
// Real AWS: "You can't delete the configuration for an DMS Serverless
// replication that is ongoing. You can delete the configuration when the
// replication is in a non-RUNNING and non-STARTING state".
func (b *InMemoryBackend) DeleteReplicationConfig(ctx context.Context, identifierOrArn string) error {
	b.mu.Lock("DeleteReplicationConfig")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	if rc, ok := b.replicationConfigs.Get(regionKey(region, identifierOrArn)); ok {
		if rc.Status == statusRunning {
			return fmt.Errorf("%w: replication config %s is running", ErrInvalidState, identifierOrArn)
		}

		rc.Tags.Close()
		b.replicationConfigs.Delete(regionKey(region, identifierOrArn))

		return nil
	}

	if rc, ok := lookupUnique(b.replicationConfigsByARN, regionKey(region, identifierOrArn)); ok {
		if rc.Status == statusRunning {
			return fmt.Errorf("%w: replication config %s is running", ErrInvalidState, identifierOrArn)
		}

		rc.Tags.Close()
		b.replicationConfigs.Delete(regionKey(region, rc.ReplicationConfigIdentifier))

		return nil
	}

	return fmt.Errorf("%w: replication config %s not found", ErrNotFound, identifierOrArn)
}

// DescribeReplicationConfigs returns all replication configs.
func (b *InMemoryBackend) DescribeReplicationConfigs(ctx context.Context) ([]*ReplicationConfig, error) {
	b.mu.RLock("DescribeReplicationConfigs")
	defer b.mu.RUnlock()

	items := b.replicationConfigsByRegion.Get(getRegion(ctx, b.region))
	list := make([]*ReplicationConfig, 0, len(items))
	for _, rc := range items {
		cp := *rc
		list = append(list, &cp)
	}

	return list, nil
}

// ModifyReplicationConfig updates the replication type of an existing replication config.
func (b *InMemoryBackend) ModifyReplicationConfig(
	ctx context.Context,
	identifierOrArn, replicationType string,
) (*ReplicationConfig, error) {
	b.mu.Lock("ModifyReplicationConfig")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	if rc, ok := b.replicationConfigs.Get(regionKey(region, identifierOrArn)); ok {
		if replicationType != "" {
			rc.ReplicationType = replicationType
		}
		cp := *rc

		return &cp, nil
	}

	if rc, ok := lookupUnique(b.replicationConfigsByARN, regionKey(region, identifierOrArn)); ok {
		if replicationType != "" {
			rc.ReplicationType = replicationType
		}
		cp := *rc

		return &cp, nil
	}

	return nil, fmt.Errorf("%w: replication config %s not found", ErrNotFound, identifierOrArn)
}

// findReplicationConfig locates a replication config by identifier or ARN
// within the request region (must hold a lock).
func (b *InMemoryBackend) findReplicationConfig(ctx context.Context, arnOrID string) *ReplicationConfig {
	region := getRegion(ctx, b.region)
	if rc, ok := b.replicationConfigs.Get(regionKey(region, arnOrID)); ok {
		return rc
	}

	if rc, ok := lookupUnique(b.replicationConfigsByARN, regionKey(region, arnOrID)); ok {
		return rc
	}

	return nil
}

// StartReplication starts (or resumes) the DMS Serverless replication
// associated with a replication config. Real AWS rejects starting a
// replication that is already running.
func (b *InMemoryBackend) StartReplication(
	ctx context.Context,
	replicationConfigArn, startReplicationType string,
) (*ReplicationConfig, error) {
	b.mu.Lock("StartReplication")
	defer b.mu.Unlock()

	rc := b.findReplicationConfig(ctx, replicationConfigArn)
	if rc == nil {
		return nil, fmt.Errorf("%w: replication config %s not found", ErrNotFound, replicationConfigArn)
	}

	if rc.Status == statusRunning {
		return nil, fmt.Errorf(
			"%w: replication for %s is already running",
			ErrInvalidState,
			replicationConfigArn,
		)
	}

	rc.Status = statusRunning
	rc.StartReplicationType = startReplicationType
	cp := *rc

	return &cp, nil
}

// StopReplication stops the DMS Serverless replication associated with a
// replication config. Real AWS rejects stopping a replication that is not
// currently running.
func (b *InMemoryBackend) StopReplication(
	ctx context.Context,
	replicationConfigArn string,
) (*ReplicationConfig, error) {
	b.mu.Lock("StopReplication")
	defer b.mu.Unlock()

	rc := b.findReplicationConfig(ctx, replicationConfigArn)
	if rc == nil {
		return nil, fmt.Errorf("%w: replication config %s not found", ErrNotFound, replicationConfigArn)
	}

	if rc.Status != statusRunning {
		return nil, fmt.Errorf(
			"%w: replication for %s is not running",
			ErrInvalidState,
			replicationConfigArn,
		)
	}

	rc.Status = statusStopped
	cp := *rc

	return &cp, nil
}

// ReloadReplicationTables reloads the target tables of a running DMS
// Serverless replication with source data. Real AWS only permits this while
// the replication is RUNNING, otherwise it throws InvalidResourceStateFault.
func (b *InMemoryBackend) ReloadReplicationTables(ctx context.Context, arnOrID string) (*ReplicationConfig, error) {
	b.mu.Lock("ReloadReplicationTables")
	defer b.mu.Unlock()

	rc := b.findReplicationConfig(ctx, arnOrID)
	if rc == nil {
		return nil, fmt.Errorf("%w: replication config %s not found", ErrNotFound, arnOrID)
	}

	if rc.Status != statusRunning {
		return nil, fmt.Errorf(
			"%w: replication for %s must be running to reload tables; current status is %s",
			ErrInvalidState,
			arnOrID,
			rc.Status,
		)
	}

	cp := *rc

	return &cp, nil
}

// DescribeReplications returns DMS Serverless replication runtime state,
// backed by the replication configs that have had StartReplication called
// against them at least once. A config that has never been started still
// carries a valid Status ("created") from CreateReplicationConfig, matching
// AWS's behavior of DescribeReplications listing every config regardless of
// whether it has ever run. Optionally filtered by config identifier or ARN.
func (b *InMemoryBackend) DescribeReplications(
	ctx context.Context,
	replicationConfigIDOrArn string,
) ([]*ReplicationConfig, error) {
	b.mu.RLock("DescribeReplications")
	defer b.mu.RUnlock()

	if replicationConfigIDOrArn != "" {
		rc := b.findReplicationConfig(ctx, replicationConfigIDOrArn)
		if rc == nil {
			return []*ReplicationConfig{}, nil
		}

		cp := *rc

		return []*ReplicationConfig{&cp}, nil
	}

	items := b.replicationConfigsByRegion.Get(getRegion(ctx, b.region))
	list := make([]*ReplicationConfig, 0, len(items))

	for _, rc := range items {
		cp := *rc
		list = append(list, &cp)
	}

	return list, nil
}

package eks

import (
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/collections"
)

// updateTransitionDelay is the async delay before an InProgress Update reaches
// Successful, matching the 100ms scale used by clusterTransitionDelay/
// addonTransitionDelay/nodegroupTransitionDelay/fargateProfileTransitionDelay.
const updateTransitionDelay = 100 * time.Millisecond

// scheduleUpdateTransition schedules the async InProgress -> Successful
// transition for the given update, mirroring scheduleClusterActivation.
// UpdateClusterConfig/UpdateClusterVersion/UpdateNodegroupVersion/
// UpdateNodegroupConfig previously stamped Status: statusInProgress and left it
// there forever -- no ticker, no later call, nothing else ever wrote to an
// Update's Status except CancelUpdate (VersionRollback-only). A client polling
// DescribeUpdate never saw a terminal status.
func (b *InMemoryBackend) scheduleUpdateTransition(clusterName, updateID string) {
	b.work.After("UpdateTransition", updateTransitionDelay, func() {
		b.mu.Lock("UpdateTransition-async")
		defer b.mu.Unlock()

		if u, ok := b.updates.Get(updateKey(clusterName, updateID)); ok && u.Status == statusInProgress {
			u.Status = statusSuccessful
		}
	})
}

// AssociateEncryptionConfig associates encryption configuration with a cluster.
// Each call replaces the stored configuration rather than appending.
func (b *InMemoryBackend) AssociateEncryptionConfig(
	clusterName string,
	configs []EncryptionConfig,
) ([]EncryptionConfig, error) {
	b.mu.Lock("AssociateEncryptionConfig")
	defer b.mu.Unlock()

	c, ok := b.clusters.Get(clusterName)
	if !ok {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrNotFound, clusterName)
	}

	for _, cfg := range configs {
		if keyARN, hasKey := cfg.Provider["keyArn"]; hasKey && keyARN != "" {
			if !isKMSARN(keyARN) {
				return nil, fmt.Errorf("%w: provider.keyArn %q is not a valid KMS key ARN", ErrValidation, keyARN)
			}
		}
	}

	stored := make([]EncryptionConfig, len(configs))
	copy(stored, configs)
	b.encryptionConfigs[clusterName] = stored
	c.EncryptionConfig = stored

	result := make([]EncryptionConfig, len(stored))
	copy(result, stored)

	return result, nil
}

// isKMSARN returns true when s begins with the KMS ARN prefix.
func isKMSARN(s string) bool {
	return len(s) > 12 && s[:12] == "arn:aws:kms:"
}

// ClusterConfigUpdate holds mutable cluster config fields for UpdateClusterConfig.
type ClusterConfigUpdate struct {
	AccessConfig  *AccessConfig
	ComputeConfig *ComputeConfig
	StorageConfig *StorageConfig
	LogEntries    []ClusterLogEntry
	SubnetIDs     []string
}

// UpdateClusterConfig updates the cluster configuration including logging, subnets, and access settings.
func (b *InMemoryBackend) UpdateClusterConfig(clusterName string, upd ClusterConfigUpdate) (*Update, error) {
	b.mu.Lock("UpdateClusterConfig")
	defer b.mu.Unlock()

	c, ok := b.clusters.Get(clusterName)
	if !ok {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrNotFound, clusterName)
	}

	if upd.LogEntries != nil {
		c.ClusterLogging = mergeClusterLogEntries(c.ClusterLogging, upd.LogEntries)
	}

	if len(upd.SubnetIDs) > 0 {
		if c.VpcConfig == nil {
			c.VpcConfig = &VpcConfig{}
		}

		c.VpcConfig.SubnetIDs = cloneStrings(upd.SubnetIDs)
	}

	// types.UpdateAccessConfigRequest (eks@v1.90.4 types/types.go:2727) has
	// only AuthenticationMode -- no BootstrapClusterCreatorAdminPermissions
	// member exists on this op's wire shape, so it must never be written
	// here (a real client cannot even send it, and unconditionally copying
	// upd.AccessConfig's zero value clobbered a true bootstrap flag set at
	// CreateCluster back to false on every AuthenticationMode-only update).
	if upd.AccessConfig != nil {
		if c.AccessConfig == nil {
			c.AccessConfig = &AccessConfig{}
		}

		if upd.AccessConfig.AuthenticationMode != "" {
			c.AccessConfig.AuthenticationMode = upd.AccessConfig.AuthenticationMode
		}
	}

	if upd.ComputeConfig != nil {
		c.ComputeConfig = upd.ComputeConfig
	}

	if upd.StorageConfig != nil {
		c.StorageConfig = upd.StorageConfig
	}

	u := &Update{
		ID:          stableID(clusterName + "/config-update/" + time.Now().String()),
		ClusterName: clusterName,
		Status:      statusInProgress,
		Type:        "ConfigUpdate",
		CreatedAt:   time.Now().UTC(),
	}
	b.storeUpdateLocked(u)
	b.scheduleUpdateTransition(clusterName, u.ID)

	return u.clone(), nil
}

// VpcEndpointUpdate carries optional VPC endpoint access changes for UpdateClusterVpcEndpoint.
type VpcEndpointUpdate struct {
	EndpointPublicAccess  *bool
	EndpointPrivateAccess *bool
	PublicAccessCIDRs     []string
}

// UpdateClusterVpcEndpoint updates the VPC endpoint access settings on a cluster.
// Fields with nil pointer values are left unchanged.
func (b *InMemoryBackend) UpdateClusterVpcEndpoint(clusterName string, upd VpcEndpointUpdate) (*Update, error) {
	b.mu.Lock("UpdateClusterVpcEndpoint")
	defer b.mu.Unlock()

	c, ok := b.clusters.Get(clusterName)
	if !ok {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrNotFound, clusterName)
	}

	if c.VpcConfig == nil {
		c.VpcConfig = &VpcConfig{
			EndpointPublicAccess: true,
		}
	}

	var params []UpdateParam
	if upd.EndpointPublicAccess != nil {
		c.VpcConfig.EndpointPublicAccess = *upd.EndpointPublicAccess
		params = append(
			params,
			UpdateParam{Type: "EndpointPublicAccess", Value: strconv.FormatBool(*upd.EndpointPublicAccess)},
		)
	}
	if upd.EndpointPrivateAccess != nil {
		c.VpcConfig.EndpointPrivateAccess = *upd.EndpointPrivateAccess
		params = append(
			params,
			UpdateParam{Type: "EndpointPrivateAccess", Value: strconv.FormatBool(*upd.EndpointPrivateAccess)},
		)
	}
	if upd.PublicAccessCIDRs != nil {
		c.VpcConfig.PublicAccessCIDRs = cloneStrings(upd.PublicAccessCIDRs)
		params = append(params, UpdateParam{Type: "PublicAccessCidrs", Value: fmt.Sprintf("%v", upd.PublicAccessCIDRs)})
	}

	u := &Update{
		ID:          stableID(clusterName + "/vpc-update/" + time.Now().String()),
		ClusterName: clusterName,
		Status:      statusSuccessful,
		Type:        "EndpointAccessUpdate",
		Params:      params,
		CreatedAt:   time.Now().UTC(),
	}
	b.storeUpdateLocked(u)

	return u.clone(), nil
}

// mergeClusterLogEntries applies logEntries on top of existing, enabling or disabling
// individual log types, and returns the merged structured result.
func mergeClusterLogEntries(existing []ClusterLogEntry, updates []ClusterLogEntry) []ClusterLogEntry {
	merged := make(map[string]bool)

	for _, e := range existing {
		if e.Enabled {
			for _, t := range e.Types {
				merged[t] = true
			}
		}
	}

	for _, entry := range updates {
		for _, t := range entry.Types {
			if entry.Enabled {
				merged[t] = true
			} else {
				delete(merged, t)
			}
		}
	}

	if len(merged) == 0 {
		return nil
	}

	enabled := collections.SortedKeys(merged)

	return []ClusterLogEntry{{Types: enabled, Enabled: true}}
}

// UpdateClusterVersion updates the cluster Kubernetes version.
func (b *InMemoryBackend) UpdateClusterVersion(clusterName, version string) (*Update, error) {
	b.mu.Lock("UpdateClusterVersion")
	defer b.mu.Unlock()

	c, ok := b.clusters.Get(clusterName)
	if !ok {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrNotFound, clusterName)
	}

	if version != "" {
		c.Version = version
	}

	u := &Update{
		ID:          stableID(clusterName + "/version-update/" + time.Now().String()),
		ClusterName: clusterName,
		Status:      statusInProgress,
		Type:        typeVersionUpdate,
		Params:      []UpdateParam{{Type: "Version", Value: version}},
		CreatedAt:   time.Now().UTC(),
	}
	b.storeUpdateLocked(u)
	b.scheduleUpdateTransition(clusterName, u.ID)

	return u.clone(), nil
}

// storeUpdateLocked stores an update record. Must be called with b.mu held.
func (b *InMemoryBackend) storeUpdateLocked(u *Update) {
	b.updates.Put(u)
}

// clone deep-copies u's reference fields. A shallow "cp := *u" is not enough:
// scheduleUpdateTransition and CancelUpdate mutate a live, stored *Update in
// place under lock, so any copy that still aliases Params/Errors/Cancellation
// stays exposed to that mutation after it crosses the lock boundary -- see
// TestUpdateClusterVersion_ReturnedUpdateIsNotLiveAliased.
func (u *Update) clone() *Update {
	cp := *u
	cp.Params = slices.Clone(u.Params)
	cp.Errors = slices.Clone(u.Errors)

	if u.Cancellation != nil {
		c := *u.Cancellation
		cp.Cancellation = &c
	}

	return &cp
}

// StoreUpdate stores an update record created outside the backend (e.g. by a handler).
func (b *InMemoryBackend) StoreUpdate(u *Update) {
	b.mu.Lock("StoreUpdate")
	defer b.mu.Unlock()

	b.storeUpdateLocked(u)
}

// CancelUpdate cancels an in-progress update on a best-effort basis. Real EKS
// currently only supports cancellation for VersionRollback updates that are
// still InProgress (Kubernetes version rollback on EKS Auto Mode clusters) --
// verified against aws-sdk-go-v2/service/eks's CancelUpdate doc comment.
// Any other update type or status returns ErrInvalidRequest, matching the
// real API's "cancellation is only performed if the update can be
// cancelled" behavior.
func (b *InMemoryBackend) CancelUpdate(clusterName, updateID string) (*Update, error) {
	b.mu.Lock("CancelUpdate")
	defer b.mu.Unlock()

	if _, ok := b.clusters.Get(clusterName); !ok {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrNotFound, clusterName)
	}

	u, ok := b.updates.Get(updateKey(clusterName, updateID))
	if !ok {
		return nil, fmt.Errorf("%w: update %s not found in cluster %s", ErrNotFound, updateID, clusterName)
	}

	if u.Type != typeVersionRollback {
		return nil, fmt.Errorf(
			"%w: update %s of type %s does not support cancellation", ErrInvalidRequest, updateID, u.Type,
		)
	}

	if u.Status != statusInProgress {
		return nil, fmt.Errorf(
			"%w: update %s is not in a cancellable state (status %s)", ErrInvalidRequest, updateID, u.Status,
		)
	}

	u.Status = statusCancelled
	u.Cancellation = &Cancellation{
		Status: cancellationStatusSuccessful,
		Reason: "Cancelled by CancelUpdate request",
	}
	b.storeUpdateLocked(u)

	return u.clone(), nil
}

// DescribeUpdate returns an update record by cluster and update ID.
func (b *InMemoryBackend) DescribeUpdate(clusterName, updateID string) (*Update, error) {
	b.mu.RLock("DescribeUpdate")
	defer b.mu.RUnlock()

	if _, ok := b.clusters.Get(clusterName); !ok {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrNotFound, clusterName)
	}

	u, ok := b.updates.Get(updateKey(clusterName, updateID))
	if !ok {
		return nil, fmt.Errorf("%w: update %s not found in cluster %s", ErrNotFound, updateID, clusterName)
	}

	return u.clone(), nil
}

// ListUpdates returns all update IDs for a cluster sorted alphabetically.
func (b *InMemoryBackend) ListUpdates(clusterName string) ([]string, error) {
	b.mu.RLock("ListUpdates")
	defer b.mu.RUnlock()

	if _, ok := b.clusters.Get(clusterName); !ok {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrNotFound, clusterName)
	}

	items := b.updatesByCluster.Get(clusterName)
	ids := make([]string, len(items))

	for i, u := range items {
		ids[i] = u.ID
	}

	slices.Sort(ids)

	return ids, nil
}

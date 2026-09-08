package redshift

import (
	"fmt"
	"sort"
)

// ModifyClusterOptions groups ModifyCluster's optional fields. Real
// ModifyClusterInput (redshift@v1.65.4 api_op_ModifyCluster.go) has ~30
// fields; this backend models the subset gopherstack-igsa scoped in, plus
// the pre-existing NodeType/NumberOfNodes/Encrypted/EnhancedVpcRouting.
//
// Encrypted, EnhancedVpcRouting and PubliclyAccessible are tri-state (nil
// means "not specified, leave unchanged"): their real ModifyClusterInput
// counterparts are *bool, and the SDK can explicitly send "false" to disable
// a setting (e.g. to decrypt a cluster). A plain bool cannot distinguish
// "not sent" from "explicitly false".
type ModifyClusterOptions struct {
	Encrypted           *bool
	EnhancedVpcRouting  *bool
	PubliclyAccessible  *bool
	NodeType            string
	MasterUserPassword  string
	ClusterVersion      string
	VpcSecurityGroupIDs []string
	NumberOfNodes       int
	Port                int
	ApplyImmediately    bool
}

// ModifyCluster modifies a cluster's attributes.
// When ApplyImmediately is false, NodeType/NumberOfNodes/Encrypted/
// ClusterVersion/PubliclyAccessible are stored in PendingModifiedValues and
// returned without being applied to the live cluster -- these are exactly
// the fields the real types.PendingModifiedValues (types/types.go:1491)
// tracks. VpcSecurityGroupIds and Port are applied unconditionally: real
// ModifyClusterInput documents VpcSecurityGroupIds as "asynchronously
// applied as soon as possible" (not gated by ApplyImmediately), and Port has
// no entry in PendingModifiedValues at all.
func (b *InMemoryBackend) ModifyCluster(id string, opts ModifyClusterOptions) (*Cluster, error) {
	if id == "" {
		return nil, fmt.Errorf("%w: ClusterIdentifier is required", ErrInvalidParameter)
	}

	b.mu.Lock("ModifyCluster")
	defer b.mu.Unlock()

	cluster, exists := b.clusters.Get(id)
	if !exists {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrClusterNotFound, id)
	}

	if cluster.Status != clusterStatusAvailable {
		return nil, fmt.Errorf(
			"%w: cluster %s is not in the available state (status %q)",
			ErrClusterInvalidState, id, cluster.Status,
		)
	}

	if opts.VpcSecurityGroupIDs != nil {
		cluster.VpcSecurityGroupIDs = opts.VpcSecurityGroupIDs
	}

	if opts.Port > 0 {
		cluster.Port = opts.Port
	}

	if !opts.ApplyImmediately {
		cluster.PendingModifiedValues = pendingModifiedValuesFrom(opts)
		cp := cloneCluster(cluster)

		return &cp, nil
	}

	applyModifyClusterImmediate(cluster, opts)
	cluster.PendingModifiedValues = nil
	cp := cloneCluster(cluster)

	return &cp, nil
}

// pendingModifiedValuesFrom builds the PendingModifiedValues ModifyCluster
// queues when ApplyImmediately is false -- exactly the fields real
// types.PendingModifiedValues (types/types.go:1491) tracks.
func pendingModifiedValuesFrom(opts ModifyClusterOptions) *ClusterPendingModifiedValues {
	pending := &ClusterPendingModifiedValues{}
	if opts.NodeType != "" {
		pending.NodeType = opts.NodeType
	}

	if opts.NumberOfNodes > 0 {
		pending.NumberOfNodes = opts.NumberOfNodes
	}

	if opts.Encrypted != nil {
		pending.Encrypted = *opts.Encrypted
	}

	if opts.ClusterVersion != "" {
		pending.ClusterVersion = opts.ClusterVersion
	}

	if opts.PubliclyAccessible != nil {
		pending.PubliclyAccessible = *opts.PubliclyAccessible
	}

	return pending
}

// applyModifyClusterImmediate applies opts directly to cluster, used when
// ApplyImmediately is true (or for the fields never gated by it).
func applyModifyClusterImmediate(cluster *Cluster, opts ModifyClusterOptions) {
	if opts.NodeType != "" {
		cluster.NodeType = opts.NodeType
	}

	if opts.NumberOfNodes > 0 {
		cluster.NumberOfNodes = opts.NumberOfNodes
	}

	if opts.Encrypted != nil {
		cluster.Encrypted = *opts.Encrypted
	}

	if opts.EnhancedVpcRouting != nil {
		cluster.EnhancedVpcRouting = *opts.EnhancedVpcRouting
	}

	if opts.ClusterVersion != "" {
		cluster.ClusterVersion = opts.ClusterVersion
	}

	if opts.PubliclyAccessible != nil {
		cluster.PubliclyAccessible = *opts.PubliclyAccessible
	}
}

// RebootCluster initiates a reboot of the specified cluster.
func (b *InMemoryBackend) RebootCluster(id string) (*Cluster, error) {
	if id == "" {
		return nil, fmt.Errorf("%w: ClusterIdentifier is required", ErrInvalidParameter)
	}

	b.mu.Lock("RebootCluster")
	defer b.mu.Unlock()

	cluster, exists := b.clusters.Get(id)
	if !exists {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrClusterNotFound, id)
	}

	cluster.Status = "rebooting"
	cp := cloneCluster(cluster)
	// Immediately set back to available for in-memory simplicity.
	cluster.Status = clusterStatusAvailable

	return &cp, nil
}

// ModifyAquaConfiguration validates that id refers to a real cluster and
// otherwise does nothing: the real operation is retired
// (api_op_ModifyAquaConfiguration.go: "Calling this operation does not
// change AQUA configuration. Amazon Redshift automatically determines
// whether to use AQUA") but still requires and existence-checks
// ClusterIdentifier (ClusterNotFoundFault is declared in its error switch).
func (b *InMemoryBackend) ModifyAquaConfiguration(id string) (*Cluster, error) {
	if id == "" {
		return nil, fmt.Errorf("%w: ClusterIdentifier is required", ErrInvalidParameter)
	}

	b.mu.Lock("ModifyAquaConfiguration")
	defer b.mu.Unlock()

	cluster, exists := b.clusters.Get(id)
	if !exists {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrClusterNotFound, id)
	}

	cp := cloneCluster(cluster)

	return &cp, nil
}

// PauseCluster pauses the specified cluster.
func (b *InMemoryBackend) PauseCluster(id string) (*Cluster, error) {
	if id == "" {
		return nil, fmt.Errorf("%w: ClusterIdentifier is required", ErrInvalidParameter)
	}

	b.mu.Lock("PauseCluster")
	defer b.mu.Unlock()

	cluster, exists := b.clusters.Get(id)
	if !exists {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrClusterNotFound, id)
	}

	cluster.Status = "paused"
	cp := cloneCluster(cluster)

	return &cp, nil
}

// ResumeCluster resumes a paused cluster.
func (b *InMemoryBackend) ResumeCluster(id string) (*Cluster, error) {
	if id == "" {
		return nil, fmt.Errorf("%w: ClusterIdentifier is required", ErrInvalidParameter)
	}

	b.mu.Lock("ResumeCluster")
	defer b.mu.Unlock()

	cluster, exists := b.clusters.Get(id)
	if !exists {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrClusterNotFound, id)
	}

	cluster.Status = clusterStatusAvailable
	cp := cloneCluster(cluster)

	return &cp, nil
}

// ResizeCluster initiates a resize of the specified cluster.
func (b *InMemoryBackend) ResizeCluster(
	id, nodeType, clusterType string,
	numberOfNodes int,
	classic bool,
) (*Cluster, error) {
	if id == "" {
		return nil, fmt.Errorf("%w: ClusterIdentifier is required", ErrInvalidParameter)
	}

	b.mu.Lock("ResizeCluster")
	defer b.mu.Unlock()

	cluster, exists := b.clusters.Get(id)
	if !exists {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrClusterNotFound, id)
	}

	if nodeType != "" {
		cluster.NodeType = nodeType
	}

	if clusterType != "" {
		cluster.ClusterType = clusterType
	}

	if numberOfNodes > 0 {
		cluster.NumberOfNodes = numberOfNodes
	}

	// Record the resize as an already-completed activeResizes entry so
	// DescribeResize can observe it immediately after ResizeCluster returns (real
	// AWS resize is asynchronous and pollable; this backend applies it
	// instantly). AllowCancelResize is false since the resize is already
	// finished by the time this call returns, matching CancelResize's existing
	// "cannot be cancelled at this stage" rejection for a resize that has
	// already completed. See PARITY.md gaps history for why this was previously
	// missing entirely.
	resizeType := "elastic"
	if classic {
		resizeType = "classic"
	}

	b.activeResizes[id] = &ResizeProgress{
		TargetNodeType:      cluster.NodeType,
		TargetClusterType:   cluster.ClusterType,
		TargetNumberOfNodes: cluster.NumberOfNodes,
		Status:              resizeStatusSucceeded,
		ResizeType:          resizeType,
		AllowCancelResize:   false,
	}

	cp := cloneCluster(cluster)

	return &cp, nil
}

// RotateEncryptionKey rotates the encryption key for the specified cluster.
func (b *InMemoryBackend) RotateEncryptionKey(id string) (*Cluster, error) {
	if id == "" {
		return nil, fmt.Errorf("%w: ClusterIdentifier is required", ErrInvalidParameter)
	}

	b.mu.Lock("RotateEncryptionKey")
	defer b.mu.Unlock()

	cluster, exists := b.clusters.Get(id)
	if !exists {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrClusterNotFound, id)
	}

	cluster.Encrypted = true
	cp := cloneCluster(cluster)

	return &cp, nil
}

// ModifyClusterIamRoles adds and removes IAM roles on a cluster.
func (b *InMemoryBackend) ModifyClusterIamRoles(id string, addRoles, removeRoles []string) (*Cluster, error) {
	if id == "" {
		return nil, fmt.Errorf("%w: ClusterIdentifier is required", ErrInvalidParameter)
	}

	b.mu.Lock("ModifyClusterIamRoles")
	defer b.mu.Unlock()

	cluster, exists := b.clusters.Get(id)
	if !exists {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrClusterNotFound, id)
	}

	if cluster.Status != clusterStatusAvailable {
		return nil, fmt.Errorf(
			"%w: cluster %s is not in the available state (status %q)",
			ErrClusterInvalidState, id, cluster.Status,
		)
	}

	// Build a set of current roles for O(1) lookup.
	roleSet := make(map[string]struct{}, len(cluster.IamRoles))
	for _, r := range cluster.IamRoles {
		roleSet[r] = struct{}{}
	}

	for _, r := range addRoles {
		roleSet[r] = struct{}{}
	}

	for _, r := range removeRoles {
		delete(roleSet, r)
	}

	roles := make([]string, 0, len(roleSet))
	for r := range roleSet {
		roles = append(roles, r)
	}

	sort.Strings(roles)
	cluster.IamRoles = roles

	cp := cloneCluster(cluster)

	return &cp, nil
}

// ModifyClusterMaintenance modifies the maintenance settings of a cluster.
func (b *InMemoryBackend) ModifyClusterMaintenance(
	id, maintenanceTrack string,
	_ bool,
) (*Cluster, error) {
	if id == "" {
		return nil, fmt.Errorf("%w: ClusterIdentifier is required", ErrInvalidParameter)
	}

	b.mu.Lock("ModifyClusterMaintenance")
	defer b.mu.Unlock()

	cluster, exists := b.clusters.Get(id)
	if !exists {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrClusterNotFound, id)
	}

	if maintenanceTrack != "" {
		cluster.PreferredMaintenanceWindow = maintenanceTrack
	}

	cp := cloneCluster(cluster)

	return &cp, nil
}

// FailoverPrimaryCompute simulates a primary compute failover by updating the cluster status.
func (b *InMemoryBackend) FailoverPrimaryCompute(clusterID string) (*Cluster, error) {
	if clusterID == "" {
		return nil, fmt.Errorf("%w: ClusterIdentifier is required", ErrInvalidParameter)
	}

	b.mu.Lock("FailoverPrimaryCompute")
	defer b.mu.Unlock()

	cluster, exists := b.clusters.Get(clusterID)
	if !exists {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrClusterNotFound, clusterID)
	}

	cp := cloneCluster(cluster)

	return &cp, nil
}

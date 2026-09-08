package eks

import (
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// nodegroupTransitionDelay is the async delay before a CREATING nodegroup reaches ACTIVE.
const nodegroupTransitionDelay = 100 * time.Millisecond

// NodegroupInput holds optional fields for CreateNodegroup beyond positional params.
type NodegroupInput struct {
	Labels         map[string]string
	RemoteAccess   *RemoteAccess
	LaunchTemplate *LaunchTemplate
	UpdateConfig   *NodegroupUpdateConfig
	Subnets        []string
	Taints         []NodegroupTaint
	DiskSize       int32
}

const (
	nodegroupDiskSizeMin = 20
	nodegroupDiskSizeMax = 16384
)

// resolveNodegroupVersion implements api_op_CreateNodegroup.go's Version
// field doc: "By default, the Kubernetes version of the cluster is used,
// and this is the only accepted specified value." An empty version defaults
// to the cluster's; any other value must match exactly.
func resolveNodegroupVersion(clusterName, version, clusterVersion string) (string, error) {
	if version == "" {
		return clusterVersion, nil
	}

	if version != clusterVersion {
		return "", fmt.Errorf(
			"%w: nodegroup version %s must match cluster %s's Kubernetes version %s",
			ErrValidation, version, clusterName, clusterVersion,
		)
	}

	return version, nil
}

// newNodegroupLocked builds a new Nodegroup value for CreateNodegroup. Must
// be called with b.mu held.
func (b *InMemoryBackend) newNodegroupLocked(
	clusterName, nodegroupName, nodeRole, amiType, capacityType, version, releaseVersion string,
	instanceTypes []string,
	desiredSize, minSize, maxSize int32,
	input NodegroupInput,
	kv map[string]string,
) *Nodegroup {
	ngARN := arn.Build(
		"eks",
		b.region,
		b.accountID,
		"nodegroup/"+clusterName+"/"+nodegroupName+"/"+stableID(clusterName+"/"+nodegroupName),
	)
	t := tags.New("eks.nodegroup." + clusterName + "." + nodegroupName + ".tags")
	if len(kv) > 0 {
		t.Merge(kv)
	}

	if amiType == "" {
		amiType = "AL2_x86_64"
	}
	if capacityType == "" {
		capacityType = "ON_DEMAND"
	}

	asgName := "eks-" + nodegroupName + "-" + stableID(clusterName+"/"+nodegroupName)

	var updateCfg *NodegroupUpdateConfig
	if input.UpdateConfig != nil {
		uc := *input.UpdateConfig
		updateCfg = &uc
	}

	return &Nodegroup{
		NodegroupName:  nodegroupName,
		ClusterName:    clusterName,
		ARN:            ngARN,
		NodeRole:       nodeRole,
		Status:         statusCreating,
		AMIType:        amiType,
		CapacityType:   capacityType,
		InstanceTypes:  cloneStrings(instanceTypes),
		Version:        version,
		ReleaseVersion: releaseVersion,
		DesiredSize:    desiredSize,
		MinSize:        minSize,
		MaxSize:        maxSize,
		DiskSize:       input.DiskSize,
		Subnets:        cloneStrings(input.Subnets),
		Labels:         cloneStringMap(input.Labels),
		Taints:         cloneTaints(input.Taints),
		RemoteAccess:   cloneRemoteAccess(input.RemoteAccess),
		LaunchTemplate: cloneLaunchTemplate(input.LaunchTemplate),
		UpdateConfig:   updateCfg,
		Resources: &NodegroupResources{
			AutoScalingGroups: []AutoScalingGroup{{Name: asgName}},
		},
		AccountID: b.accountID,
		Region:    b.region,
		CreatedAt: time.Now().UTC(),
		Tags:      t,
	}
}

// CreateNodegroup creates a new node group in a cluster.
func (b *InMemoryBackend) CreateNodegroup(
	clusterName, nodegroupName, nodeRole, amiType, capacityType, version, releaseVersion string,
	instanceTypes []string,
	desiredSize, minSize, maxSize int32,
	input NodegroupInput,
	kv map[string]string,
) (*Nodegroup, error) {
	b.mu.Lock("CreateNodegroup")
	defer b.mu.Unlock()

	// CreateNodegroup's own deserializer (eks@v1.90.4 deserializers.go) has
	// no ResourceNotFoundException case -- an unknown cluster here is
	// ErrValidation (InvalidParameterException), not ErrNotFound.
	cluster, ok := b.clusters.Get(clusterName)
	if !ok {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrValidation, clusterName)
	}

	version, err := resolveNodegroupVersion(clusterName, version, cluster.Version)
	if err != nil {
		return nil, err
	}

	if _, exists := b.nodegroups.Get(nodegroupKey(clusterName, nodegroupName)); exists {
		return nil, fmt.Errorf(
			"%w: nodegroup %s already exists in cluster %s",
			ErrAlreadyExists,
			nodegroupName,
			clusterName,
		)
	}

	if input.DiskSize != 0 && (input.DiskSize < nodegroupDiskSizeMin || input.DiskSize > nodegroupDiskSizeMax) {
		return nil, fmt.Errorf(
			"%w: diskSize %d is out of range [%d, %d]",
			ErrValidation, input.DiskSize, nodegroupDiskSizeMin, nodegroupDiskSizeMax,
		)
	}

	ng := b.newNodegroupLocked(
		clusterName, nodegroupName, nodeRole, amiType, capacityType, version, releaseVersion,
		instanceTypes, desiredSize, minSize, maxSize, input, kv,
	)
	b.nodegroups.Put(ng)

	// Schedule async transition CREATING -> ACTIVE.
	b.work.After("NodegroupTransition", nodegroupTransitionDelay, func() {
		b.mu.Lock("CreateNodegroup-async")
		defer b.mu.Unlock()

		if n, found := b.nodegroups.Get(nodegroupKey(clusterName, nodegroupName)); found && n.Status == statusCreating {
			n.Status = statusActive
		}
	})

	cp := deepCopyNodegroup(ng)

	return cp, nil
}

// DescribeNodegroup returns a node group by cluster and nodegroup name.
func (b *InMemoryBackend) DescribeNodegroup(clusterName, nodegroupName string) (*Nodegroup, error) {
	b.mu.RLock("DescribeNodegroup")
	defer b.mu.RUnlock()

	if _, ok := b.clusters.Get(clusterName); !ok {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrNotFound, clusterName)
	}

	ng, ok := b.nodegroups.Get(nodegroupKey(clusterName, nodegroupName))
	if !ok {
		return nil, fmt.Errorf("%w: nodegroup %s not found in cluster %s", ErrNotFound, nodegroupName, clusterName)
	}

	return deepCopyNodegroup(ng), nil
}

// ListNodegroups returns all node group names in a cluster sorted alphabetically.
func (b *InMemoryBackend) ListNodegroups(clusterName string) ([]string, error) {
	b.mu.RLock("ListNodegroups")
	defer b.mu.RUnlock()

	if _, ok := b.clusters.Get(clusterName); !ok {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrNotFound, clusterName)
	}

	items := b.nodegroupsByCluster.Get(clusterName)
	names := make([]string, len(items))

	for i, ng := range items {
		names[i] = ng.NodegroupName
	}

	slices.Sort(names)

	return names, nil
}

// DeleteNodegroup deletes a node group from a cluster.
func (b *InMemoryBackend) DeleteNodegroup(clusterName, nodegroupName string) (*Nodegroup, error) {
	b.mu.Lock("DeleteNodegroup")
	defer b.mu.Unlock()

	if _, ok := b.clusters.Get(clusterName); !ok {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrNotFound, clusterName)
	}

	ng, ok := b.nodegroups.Get(nodegroupKey(clusterName, nodegroupName))
	if !ok {
		return nil, fmt.Errorf("%w: nodegroup %s not found in cluster %s", ErrNotFound, nodegroupName, clusterName)
	}
	cp := deepCopyNodegroup(ng)
	cp.Status = statusDeleting
	b.nodegroups.Delete(nodegroupKey(clusterName, nodegroupName))

	if ng.Tags != nil {
		ng.Tags.Close()
	}

	return cp, nil
}

// NodegroupConfigUpdate holds the mutable fields for UpdateNodegroupConfig.
type NodegroupConfigUpdate struct {
	AddOrUpdateLabels map[string]string
	UpdateConfig      *NodegroupUpdateConfig
	DesiredSize       *int32
	MinSize           *int32
	MaxSize           *int32
	RemoveLabels      []string
	AddOrUpdateTaints []NodegroupTaint
	RemoveTaints      []NodegroupTaint
}

// UpdateNodegroupConfig updates the configuration of a node group including scaling,
// labels, taints, and update strategy.
func (b *InMemoryBackend) UpdateNodegroupConfig(
	clusterName, nodegroupName string,
	upd NodegroupConfigUpdate,
) (*Nodegroup, error) {
	b.mu.Lock("UpdateNodegroupConfig")
	defer b.mu.Unlock()

	if _, ok := b.clusters.Get(clusterName); !ok {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrNotFound, clusterName)
	}

	ng, ok := b.nodegroups.Get(nodegroupKey(clusterName, nodegroupName))
	if !ok {
		return nil, fmt.Errorf("%w: nodegroup %s not found in cluster %s", ErrNotFound, nodegroupName, clusterName)
	}

	if upd.DesiredSize != nil {
		ng.DesiredSize = *upd.DesiredSize
	}

	if upd.MinSize != nil {
		ng.MinSize = *upd.MinSize
	}

	if upd.MaxSize != nil {
		ng.MaxSize = *upd.MaxSize
	}

	if len(upd.AddOrUpdateLabels) > 0 {
		if ng.Labels == nil {
			ng.Labels = make(map[string]string)
		}

		maps.Copy(ng.Labels, upd.AddOrUpdateLabels)
	}

	for _, k := range upd.RemoveLabels {
		delete(ng.Labels, k)
	}

	if len(upd.AddOrUpdateTaints) > 0 {
		ng.Taints = mergeTaints(ng.Taints, upd.AddOrUpdateTaints)
	}

	if len(upd.RemoveTaints) > 0 {
		ng.Taints = removeTaints(ng.Taints, upd.RemoveTaints)
	}

	if upd.UpdateConfig != nil {
		uc := *upd.UpdateConfig
		ng.UpdateConfig = &uc
	}

	return deepCopyNodegroup(ng), nil
}

// mergeTaints adds or updates taints in the existing slice.
func mergeTaints(existing []NodegroupTaint, updates []NodegroupTaint) []NodegroupTaint {
	result := make([]NodegroupTaint, 0, len(existing)+len(updates))
	result = append(result, existing...)

	for _, upd := range updates {
		found := false

		for i, t := range result {
			if t.Key == upd.Key {
				result[i] = upd
				found = true

				break
			}
		}

		if !found {
			result = append(result, upd)
		}
	}

	return result
}

// removeTaints removes taints matching by key+effect from the slice.
func removeTaints(existing []NodegroupTaint, toRemove []NodegroupTaint) []NodegroupTaint {
	result := existing[:0:len(existing)]

	for _, t := range existing {
		removed := false

		for _, r := range toRemove {
			if t.Key == r.Key && t.Effect == r.Effect {
				removed = true

				break
			}
		}

		if !removed {
			result = append(result, t)
		}
	}

	return result
}

// UpdateNodegroupVersion updates the node group Kubernetes version.
func (b *InMemoryBackend) UpdateNodegroupVersion(
	clusterName, nodegroupName, version string,
) (*Update, error) {
	b.mu.Lock("UpdateNodegroupVersion")
	defer b.mu.Unlock()

	if _, ok := b.clusters.Get(clusterName); !ok {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrNotFound, clusterName)
	}

	ng, ok := b.nodegroups.Get(nodegroupKey(clusterName, nodegroupName))
	if !ok {
		return nil, fmt.Errorf("%w: nodegroup %s not found in cluster %s", ErrNotFound, nodegroupName, clusterName)
	}

	if version != "" {
		ng.Version = version
	}

	u := &Update{
		ID:            stableID(clusterName + "/" + nodegroupName + "/version-update/" + time.Now().String()),
		ClusterName:   clusterName,
		NodegroupName: nodegroupName,
		Status:        statusInProgress,
		Type:          typeVersionUpdate,
		Params:        []UpdateParam{{Type: "Version", Value: version}},
		CreatedAt:     time.Now().UTC(),
	}
	b.storeUpdateLocked(u)
	b.scheduleUpdateTransition(clusterName, u.ID)

	return u.clone(), nil
}

// AddNodegroupInternal inserts a pre-built node group into the backend.
// Intended only for test seeding.
func (b *InMemoryBackend) AddNodegroupInternal(ng *Nodegroup) {
	b.mu.Lock("AddNodegroupInternal")
	defer b.mu.Unlock()

	if ng.Tags == nil {
		ng.Tags = tags.New("eks.nodegroup." + ng.ClusterName + "." + ng.NodegroupName + ".tags")
	}

	b.nodegroups.Put(ng)
}

// cloneRemoteAccess returns a deep copy of a RemoteAccess struct (nil-safe).
func cloneRemoteAccess(ra *RemoteAccess) *RemoteAccess {
	if ra == nil {
		return nil
	}

	cp := *ra
	cp.SourceSecurityGroups = cloneStrings(ra.SourceSecurityGroups)

	return &cp
}

// cloneLaunchTemplate returns a deep copy of a LaunchTemplate struct (nil-safe).
func cloneLaunchTemplate(lt *LaunchTemplate) *LaunchTemplate {
	if lt == nil {
		return nil
	}

	cp := *lt

	return &cp
}

// deepCopyNodegroup returns a deep copy of a Nodegroup with all slice/map fields duplicated.
func deepCopyNodegroup(ng *Nodegroup) *Nodegroup {
	cp := *ng
	cp.InstanceTypes = cloneStrings(ng.InstanceTypes)
	cp.Subnets = cloneStrings(ng.Subnets)
	cp.Labels = cloneStringMap(ng.Labels)
	cp.Taints = cloneTaints(ng.Taints)
	cp.RemoteAccess = cloneRemoteAccess(ng.RemoteAccess)
	cp.LaunchTemplate = cloneLaunchTemplate(ng.LaunchTemplate)

	if ng.Resources != nil {
		resCp := *ng.Resources
		resCp.AutoScalingGroups = make([]AutoScalingGroup, len(ng.Resources.AutoScalingGroups))
		copy(resCp.AutoScalingGroups, ng.Resources.AutoScalingGroups)
		cp.Resources = &resCp
	}

	if ng.UpdateConfig != nil {
		uc := *ng.UpdateConfig
		cp.UpdateConfig = &uc
	}

	return &cp
}

package dax

import (
	"fmt"
	"maps"
	"math/rand/v2"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

const (
	// daxPort is the standard DAX cluster port.
	daxPort = 8111

	// daxClusterURLPort is the URL port for DAX.
	daxClusterURLPort = 8111

	// paramApplyStatusInSync is the value reported for parameter group status when in sync.
	paramApplyStatusInSync = "in-sync"

	// sseStatusDisabled is the SSE status when not enabled.
	sseStatusDisabled = "DISABLED"

	// sseStatusEnabled is the SSE status when enabled.
	sseStatusEnabled = "ENABLED"

	// notificationTopicStatusActive is the active SNS topic status.
	notificationTopicStatusActive = "active"

	// maintenanceWindowMinutes is the fixed width of the simulated maintenance window.
	maintenanceWindowMinutes = 60

	// hoursPerDay is the number of hours in a day.
	hoursPerDay = 24

	// minutesPerHour is the number of minutes in an hour.
	minutesPerHour = 60

	// maxClusterNameLength is the maximum allowed length for a DAX cluster name.
	maxClusterNameLength = 20
)

// maintenanceWindowDays maps random seeds to day abbreviations for the maintenance window.
//
//nolint:gochecknoglobals // package-level lookup table
var maintenanceWindowDays = []string{
	"sun",
	"mon",
	"tue",
	"wed",
	"thu",
	"fri",
	"sat",
}

// clusterARN builds a DAX cluster ARN.
func (b *InMemoryBackend) clusterARN(name string) string {
	return arn.Build("dax", b.Region, b.AccountID, fmt.Sprintf("cache/%s", name))
}

// daxURL builds a dax:// URL from a host address and port number.
func daxURL(addr string, port int) string {
	return "dax://" + net.JoinHostPort(addr, strconv.Itoa(port))
}

// clusterEndpointAddress generates a realistic DAX endpoint address.
func clusterEndpointAddress(name, region string) string {
	const maxSuffix uint32 = 0xFFFFFF
	suffix := fmt.Sprintf("%06x", rand.Uint32N(maxSuffix)) //nolint:gosec // not security sensitive

	return fmt.Sprintf("%s.%s.dax-clusters.%s.amazonaws.com", name, suffix, region)
}

// nodeEndpointAddress generates a node-level endpoint address.
func nodeEndpointAddress(clusterName, nodeID, region string) string {
	return fmt.Sprintf("%s-%s.nodes.dax-clusters.%s.amazonaws.com", clusterName, nodeID, region)
}

// randomMaintenanceWindow returns a random 60-minute maintenance window slot.
func randomMaintenanceWindow() string {
	//nolint:gosec // not security sensitive
	day := maintenanceWindowDays[rand.Uint32N(uint32(len(maintenanceWindowDays)))]
	hour := rand.Uint32N(hoursPerDay)      //nolint:gosec // not security sensitive
	minute := rand.Uint32N(minutesPerHour) //nolint:gosec // not security sensitive

	totalMinutes := hour*minutesPerHour + minute + uint32(maintenanceWindowMinutes)
	endHour := totalMinutes / minutesPerHour % hoursPerDay
	endMinute := totalMinutes % minutesPerHour

	return fmt.Sprintf("%s:%02d:%02d-%s:%02d:%02d", day, hour, minute, day, endHour, endMinute)
}

// validateClusterName validates the DAX cluster name format per AWS constraints.
func validateClusterName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: ClusterName is required", ErrInvalidParameterValue)
	}

	if len(name) > maxClusterNameLength {
		return fmt.Errorf(
			"%w: ClusterName %q exceeds maximum length of %d characters",
			ErrInvalidParameterValue, name, maxClusterNameLength,
		)
	}

	if !nameRegexp.MatchString(name) {
		return fmt.Errorf(
			"%w: ClusterName %q is invalid: must start with a letter, "+
				"contain only letters, numbers, and hyphens, and not end with a hyphen",
			ErrInvalidParameterValue, name,
		)
	}

	if strings.Contains(name, "--") {
		return fmt.Errorf(
			"%w: ClusterName %q is invalid: must not contain consecutive hyphens",
			ErrInvalidParameterValue, name,
		)
	}

	return nil
}

// validateAvailabilityZonesLength enforces api_op_CreateCluster.go's ReplicationFactor
// doc: "If the AvailabilityZones parameter is provided, its length must equal the
// ReplicationFactor parameter.".
func validateAvailabilityZonesLength(azs []string, replicationFactor int) error {
	if len(azs) > 0 && len(azs) != replicationFactor {
		return fmt.Errorf(
			"%w: AvailabilityZones has %d entries but ReplicationFactor is %d",
			ErrInvalidParameterCombination,
			len(azs),
			replicationFactor,
		)
	}

	return nil
}

// validateCreateCluster validates the CreateCluster input before acquiring the lock.
func validateCreateCluster(input *CreateClusterInput) error {
	if err := validateClusterName(input.ClusterName); err != nil {
		return err
	}

	if input.NodeType == "" {
		return fmt.Errorf("%w: NodeType is required", ErrInvalidParameterValue)
	}

	if !validNodeTypes[input.NodeType] {
		return fmt.Errorf("%w: unsupported node type %q", ErrInvalidParameterValue, input.NodeType)
	}

	if input.IamRoleArn == "" {
		return fmt.Errorf("%w: IamRoleArn is required", ErrInvalidParameterValue)
	}

	if input.ReplicationFactor < minReplicationFactor {
		return fmt.Errorf(
			"%w: ReplicationFactor %d is below minimum of %d",
			ErrInvalidParameterCombination,
			input.ReplicationFactor,
			minReplicationFactor,
		)
	}

	if input.ReplicationFactor > maxReplicationFactor {
		return fmt.Errorf(
			"%w: ReplicationFactor %d exceeds maximum of %d",
			ErrInvalidParameterCombination,
			input.ReplicationFactor,
			maxReplicationFactor,
		)
	}

	if err := validateAvailabilityZonesLength(input.AvailabilityZones, input.ReplicationFactor); err != nil {
		return err
	}

	if input.ClusterEndpointEncryptionType != "" &&
		input.ClusterEndpointEncryptionType != EncryptionTypeNone &&
		input.ClusterEndpointEncryptionType != EncryptionTypeTLS {
		return fmt.Errorf(
			"%w: ClusterEndpointEncryptionType must be %q or %q",
			ErrInvalidParameterValue,
			EncryptionTypeNone,
			EncryptionTypeTLS,
		)
	}

	if input.NetworkType != "" &&
		input.NetworkType != NetworkTypeIPv4 &&
		input.NetworkType != NetworkTypeIPv6 &&
		input.NetworkType != NetworkTypeDualStack {
		return fmt.Errorf(
			"%w: NetworkType must be %q, %q, or %q",
			ErrInvalidParameterValue,
			NetworkTypeIPv4,
			NetworkTypeIPv6,
			NetworkTypeDualStack,
		)
	}

	return nil
}

// applyCreateClusterDefaults fills in default values for optional fields.
func applyCreateClusterDefaults(input *CreateClusterInput) {
	if input.SubnetGroupName == "" {
		input.SubnetGroupName = DefaultSubnetGroupName
	}

	if input.ParameterGroupName == "" {
		input.ParameterGroupName = DefaultParameterGroupName
	}

	if input.ClusterEndpointEncryptionType == "" {
		input.ClusterEndpointEncryptionType = EncryptionTypeNone
	}

	// If no explicit NetworkType is provided, the real API derives it from the
	// subnet group's configuration; gopherstack subnet groups are always
	// IPv4-only (see SubnetGroup.SupportedNetworkTypes), so the derived default
	// is always "ipv4".
	if input.NetworkType == "" {
		input.NetworkType = NetworkTypeIPv4
	}
}

// buildClusterNodes builds the node list for a new cluster.
func (b *InMemoryBackend) buildClusterNodes(input CreateClusterInput, now time.Time, nextNodeIndex *int) []Node {
	capacity := input.ReplicationFactor
	const maxCapacity = 100
	if capacity > maxCapacity {
		capacity = maxCapacity
	} else if capacity < 0 {
		capacity = 0
	}
	nodes := make([]Node, 0, capacity)

	for i := range capacity {
		nodeIdx := *nextNodeIndex
		*nextNodeIndex++
		nodeID := fmt.Sprintf("%s-%04d", input.ClusterName, nodeIdx)
		az := b.Region + "a"

		if i < len(input.AvailabilityZones) {
			az = input.AvailabilityZones[i]
		}

		addr := nodeEndpointAddress(input.ClusterName, fmt.Sprintf("%04d", nodeIdx), b.Region)
		nodes = append(nodes, Node{
			NodeID:               nodeID,
			NodeStatus:           StatusAvailable,
			AvailabilityZone:     az,
			CreateTime:           now,
			ParameterGroupStatus: paramApplyStatusInSync,
			Endpoint: &Endpoint{
				Address: addr,
				Port:    daxPort,
				URL:     daxURL(addr, daxClusterURLPort),
			},
		})
	}

	return nodes
}

// CreateCluster creates a new DAX cluster.
func (b *InMemoryBackend) CreateCluster(input CreateClusterInput) (*Cluster, error) {
	if err := validateCreateCluster(&input); err != nil {
		return nil, err
	}

	applyCreateClusterDefaults(&input)

	b.mu.Lock("CreateCluster")
	defer b.mu.Unlock()

	if b.clusters.Has(input.ClusterName) {
		return nil, fmt.Errorf("%w: %s", ErrClusterAlreadyExists, input.ClusterName)
	}

	if !b.subnetGroups.Has(input.SubnetGroupName) {
		return nil, fmt.Errorf("%w: %s", ErrSubnetGroupNotFound, input.SubnetGroupName)
	}

	if !b.paramGroups.Has(input.ParameterGroupName) {
		return nil, fmt.Errorf("%w: %s", ErrParameterGroupNotFound, input.ParameterGroupName)
	}

	now := time.Now().UTC()
	clusterARN := b.clusterARN(input.ClusterName)
	var nextIndex int
	nodes := b.buildClusterNodes(input, now, &nextIndex)

	sseStatus := sseStatusDisabled
	if input.SSESpecificationEnabled {
		sseStatus = sseStatusEnabled
	}

	maintenanceWindow := input.PreferredMaintenanceWindow
	if maintenanceWindow == "" {
		maintenanceWindow = randomMaintenanceWindow()
	}

	clusterEndpoint := clusterEndpointAddress(input.ClusterName, b.Region)

	cluster := b.initCluster(input, nextIndex, nodes, clusterARN, clusterEndpoint, now, maintenanceWindow, sseStatus)

	if input.NotificationTopicArn != "" {
		cluster.NotificationConfiguration = &NotificationConfiguration{
			TopicArn:    input.NotificationTopicArn,
			TopicStatus: notificationTopicStatusActive,
		}
	}

	maps.Copy(cluster.Tags, input.Tags)

	b.clusters.Put(cluster)

	if len(input.Tags) > 0 {
		b.tags[clusterARN] = make(map[string]string)
		maps.Copy(b.tags[clusterARN], input.Tags)
	}

	b.emitEventLocked(input.ClusterName, EventSourceTypeCluster,
		fmt.Sprintf("Cluster %s has been created.", input.ClusterName))

	if os.Getenv("DAX_TEST_SYNC") == "1" {
		cluster.Status = StatusAvailable
	} else {
		go func(cName string) {
			time.Sleep(time.Second)
			b.mu.Lock("CreateCluster:async")
			defer b.mu.Unlock()
			if c, ok := b.clusters.Get(cName); ok && c.Status == StatusCreating {
				c.Status = StatusAvailable
			}
		}(input.ClusterName)
	}

	cp := b.clusterCopy(cluster)

	return cp, nil
}

// initCluster is a helper to build a Cluster object.
func (b *InMemoryBackend) initCluster(
	input CreateClusterInput,
	nextIndex int,
	nodes []Node,
	clusterARN, clusterEndpoint string,
	now time.Time,
	maintenanceWindow string,
	sseStatus string,
) *Cluster {
	return &Cluster{
		ClusterName:                   input.ClusterName,
		ClusterArn:                    clusterARN,
		Description:                   input.Description,
		NodeType:                      input.NodeType,
		Status:                        StatusCreating,
		IamRoleArn:                    input.IamRoleArn,
		SubnetGroupName:               input.SubnetGroupName,
		SecurityGroupIDs:              input.SecurityGroupIDs,
		PreferredMaintenanceWindow:    maintenanceWindow,
		ClusterEndpointEncryptionType: input.ClusterEndpointEncryptionType,
		NetworkType:                   input.NetworkType,
		CreateTime:                    now,
		TotalNodes:                    input.ReplicationFactor,
		ActiveNodes:                   input.ReplicationFactor,
		NextNodeIndex:                 nextIndex,
		Nodes:                         nodes,
		Endpoint: &Endpoint{
			Address: clusterEndpoint,
			Port:    daxPort,
			URL:     daxURL(clusterEndpoint, daxClusterURLPort),
		},
		ParameterGroup: ParameterGroupStatus{
			ParameterGroupName:   input.ParameterGroupName,
			ParameterApplyStatus: paramApplyStatusInSync,
		},
		SSEDescription: SSEDescription{
			Status: sseStatus,
		},
		Tags: make(map[string]string),
	}
}

// collectClustersLocked collects clusters, filtering by name if provided.
// Must be called with b.mu held for reading.
func (b *InMemoryBackend) collectClustersLocked(clusterNames []string) ([]*Cluster, error) {
	if len(clusterNames) == 0 {
		return b.clusters.All(), nil
	}

	for _, name := range clusterNames {
		if !b.clusters.Has(name) {
			return nil, fmt.Errorf("%w: %s", ErrClusterNotFound, name)
		}
	}

	all := make([]*Cluster, 0, len(clusterNames))
	for _, name := range clusterNames {
		if c, ok := b.clusters.Get(name); ok {
			all = append(all, c)
		}
	}

	return all, nil
}

// paginateClusters applies pagination to a sorted cluster slice and returns copies.
func (b *InMemoryBackend) paginateClusters(
	all []*Cluster,
	maxResults int,
	nextToken string,
) ([]*Cluster, string) {
	start := 0

	if nextToken != "" {
		start = len(all)

		for i, c := range all {
			if c.ClusterName >= nextToken {
				start = i

				break
			}
		}
	}

	if start >= len(all) {
		return []*Cluster{}, ""
	}

	end := start + maxResults
	newNextToken := ""

	if end < len(all) {
		newNextToken = all[end].ClusterName
	} else {
		end = len(all)
	}

	page := all[start:end]
	result := make([]*Cluster, 0, len(page))

	for _, c := range page {
		result = append(result, b.clusterCopy(c))
	}

	return result, newNextToken
}

// DescribeClusters returns DAX clusters, optionally filtered by name.
func (b *InMemoryBackend) DescribeClusters(
	clusterNames []string,
	maxResults int,
	nextToken string,
) ([]*Cluster, string, error) {
	b.mu.RLock("DescribeClusters")
	defer b.mu.RUnlock()

	if maxResults <= 0 {
		maxResults = maxClustersDefault
	}

	all, err := b.collectClustersLocked(clusterNames)
	if err != nil {
		return nil, "", err
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].ClusterName < all[j].ClusterName
	})

	result, token := b.paginateClusters(all, maxResults, nextToken)

	return result, token, nil
}

// UpdateCluster updates a DAX cluster's configuration.
func (b *InMemoryBackend) UpdateCluster(input UpdateClusterInput) (*Cluster, error) {
	if input.ClusterName == "" {
		return nil, fmt.Errorf("%w: ClusterName is required", ErrInvalidParameterValue)
	}

	b.mu.Lock("UpdateCluster")
	defer b.mu.Unlock()

	cluster, ok := b.clusters.Get(input.ClusterName)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrClusterNotFound, input.ClusterName)
	}

	if cluster.Status == StatusDeleting {
		return nil, fmt.Errorf(
			"%w: cluster %s is being deleted",
			ErrInvalidClusterState,
			input.ClusterName,
		)
	}

	if input.ParameterGroupName != "" && !b.paramGroups.Has(input.ParameterGroupName) {
		return nil, fmt.Errorf("%w: %s", ErrParameterGroupNotFound, input.ParameterGroupName)
	}

	if input.Description != nil {
		cluster.Description = *input.Description
	}

	if input.PreferredMaintenanceWindow != "" {
		cluster.PreferredMaintenanceWindow = input.PreferredMaintenanceWindow
	}

	if len(input.SecurityGroupIDs) > 0 {
		cluster.SecurityGroupIDs = append([]string(nil), input.SecurityGroupIDs...)
	}

	if input.ParameterGroupName != "" {
		cluster.ParameterGroup.ParameterGroupName = input.ParameterGroupName
	}

	if input.NotificationTopicArn != "" {
		status := notificationTopicStatusActive
		if input.NotificationTopicStatus != "" {
			status = input.NotificationTopicStatus
		}

		cluster.NotificationConfiguration = &NotificationConfiguration{
			TopicArn:    input.NotificationTopicArn,
			TopicStatus: status,
		}
	} else if input.NotificationTopicStatus != "" && cluster.NotificationConfiguration != nil {
		cluster.NotificationConfiguration.TopicStatus = input.NotificationTopicStatus
	}

	cp := b.clusterCopy(cluster)

	return cp, nil
}

// DeleteCluster marks a DAX cluster as deleting and removes it from the store.
func (b *InMemoryBackend) DeleteCluster(clusterName string) (*Cluster, error) {
	if clusterName == "" {
		return nil, fmt.Errorf("%w: ClusterName is required", ErrInvalidParameterValue)
	}

	b.mu.Lock("DeleteCluster")
	defer b.mu.Unlock()

	cluster, ok := b.clusters.Get(clusterName)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrClusterNotFound, clusterName)
	}

	if cluster.Status == StatusDeleting {
		return nil, fmt.Errorf("%w: cluster %s is already being deleted", ErrInvalidClusterState, clusterName)
	}

	cp := b.clusterCopy(cluster)
	cluster.Status = StatusDeleting
	cp.Status = StatusDeleting

	b.emitEventLocked(clusterName, EventSourceTypeCluster,
		fmt.Sprintf("Cluster %s is being deleted.", clusterName))

	if os.Getenv("DAX_TEST_SYNC") == "1" {
		b.clusters.Delete(clusterName)
		delete(b.tags, cluster.ClusterArn)
	} else {
		go func(cName string, cArn string) {
			time.Sleep(time.Second)
			b.mu.Lock("DeleteCluster:async")
			defer b.mu.Unlock()
			if c, exists := b.clusters.Get(cName); exists && c.Status == StatusDeleting {
				b.clusters.Delete(cName)
				delete(b.tags, cArn)
				b.emitEventLocked(cName, EventSourceTypeCluster,
					fmt.Sprintf("Cluster %s has been deleted.", cName))
			}
		}(clusterName, cluster.ClusterArn)
	}

	return cp, nil
}

// IncreaseReplicationFactor adds nodes to a cluster.
func (b *InMemoryBackend) IncreaseReplicationFactor(input IncreaseReplicationFactorInput) (*Cluster, error) {
	if input.ClusterName == "" {
		return nil, fmt.Errorf("%w: ClusterName is required", ErrInvalidParameterValue)
	}

	if input.NewReplicationFactor < minReplicationFactor || input.NewReplicationFactor > maxReplicationFactor {
		return nil, fmt.Errorf(
			"%w: NewReplicationFactor must be between %d and %d",
			ErrInvalidParameterCombination,
			minReplicationFactor,
			maxReplicationFactor,
		)
	}

	b.mu.Lock("IncreaseReplicationFactor")
	defer b.mu.Unlock()

	cluster, ok := b.clusters.Get(input.ClusterName)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrClusterNotFound, input.ClusterName)
	}

	if cluster.Status != StatusAvailable {
		return nil, fmt.Errorf(
			"%w: cluster %s must be %s to resize (currently %s)",
			ErrInvalidClusterState,
			input.ClusterName,
			StatusAvailable,
			cluster.Status,
		)
	}

	if input.NewReplicationFactor <= len(cluster.Nodes) {
		return nil, fmt.Errorf(
			"%w: NewReplicationFactor %d must be greater than current %d",
			ErrInvalidParameterCombination,
			input.NewReplicationFactor,
			len(cluster.Nodes),
		)
	}

	now := time.Now().UTC()
	existingCount := len(cluster.Nodes)

	for i := existingCount; i < input.NewReplicationFactor; i++ {
		az := b.Region + "a"
		if j := i - existingCount; j < len(input.AvailabilityZones) {
			az = input.AvailabilityZones[j]
		}

		nodeIdx := cluster.NextNodeIndex
		cluster.NextNodeIndex++
		nodeID := fmt.Sprintf("%s-%04d", input.ClusterName, nodeIdx)
		addr := nodeEndpointAddress(input.ClusterName, fmt.Sprintf("%04d", nodeIdx), b.Region)

		cluster.Nodes = append(cluster.Nodes, Node{
			NodeID:               nodeID,
			NodeStatus:           StatusAvailable,
			AvailabilityZone:     az,
			CreateTime:           now,
			ParameterGroupStatus: paramApplyStatusInSync,
			Endpoint: &Endpoint{
				Address: addr,
				Port:    daxPort,
				URL:     daxURL(addr, daxClusterURLPort),
			},
		})
	}

	cluster.TotalNodes = input.NewReplicationFactor
	cluster.ActiveNodes = input.NewReplicationFactor
	cluster.Status = StatusModifying

	b.emitEventLocked(input.ClusterName, EventSourceTypeCluster,
		fmt.Sprintf("Replication factor increased to %d.", input.NewReplicationFactor))

	go func(cName string) {
		time.Sleep(time.Second)
		b.mu.Lock("IncreaseReplicationFactor:async")
		defer b.mu.Unlock()
		if c, exists := b.clusters.Get(cName); exists && c.Status == StatusModifying {
			c.Status = StatusAvailable
		}
	}(input.ClusterName)

	return b.clusterCopy(cluster), nil
}

// DecreaseReplicationFactor removes nodes from a cluster.
func (b *InMemoryBackend) DecreaseReplicationFactor(input DecreaseReplicationFactorInput) (*Cluster, error) {
	if input.ClusterName == "" {
		return nil, fmt.Errorf("%w: ClusterName is required", ErrInvalidParameterValue)
	}

	if input.NewReplicationFactor < minReplicationFactor || input.NewReplicationFactor > maxReplicationFactor {
		return nil, fmt.Errorf(
			"%w: NewReplicationFactor must be between %d and %d",
			ErrInvalidParameterCombination,
			minReplicationFactor,
			maxReplicationFactor,
		)
	}

	b.mu.Lock("DecreaseReplicationFactor")
	defer b.mu.Unlock()

	cluster, ok := b.clusters.Get(input.ClusterName)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrClusterNotFound, input.ClusterName)
	}

	if cluster.Status != StatusAvailable {
		return nil, fmt.Errorf(
			"%w: cluster %s must be %s to resize (currently %s)",
			ErrInvalidClusterState,
			input.ClusterName,
			StatusAvailable,
			cluster.Status,
		)
	}

	if input.NewReplicationFactor >= len(cluster.Nodes) {
		return nil, fmt.Errorf(
			"%w: NewReplicationFactor %d must be less than current %d",
			ErrInvalidParameterCombination,
			input.NewReplicationFactor,
			len(cluster.Nodes),
		)
	}

	var removedIDs []string

	switch {
	case len(input.NodeIDsToRemove) > 0:
		kept, err := removeSpecificNodes(
			cluster.Nodes, input.NodeIDsToRemove, input.ClusterName, input.NewReplicationFactor,
		)
		if err != nil {
			return nil, err
		}

		removedIDs = input.NodeIDsToRemove
		cluster.Nodes = kept
	case len(input.AvailabilityZones) > 0:
		kept, removed := removeNodesByAvailabilityZones(
			cluster.Nodes, input.AvailabilityZones, input.NewReplicationFactor,
		)
		removedIDs = removed
		cluster.Nodes = kept
	default:
		for _, n := range cluster.Nodes[input.NewReplicationFactor:] {
			removedIDs = append(removedIDs, n.NodeID)
		}

		cluster.Nodes = cluster.Nodes[:input.NewReplicationFactor]
	}

	cluster.TotalNodes = input.NewReplicationFactor
	cluster.ActiveNodes = input.NewReplicationFactor
	cluster.Status = StatusModifying
	// NodeIDsToRemove (types.Cluster.NodeIdsToRemove) surfaces on the wire only
	// while the decrease is in flight; cleared once the async transition below
	// brings the cluster back to "available".
	cluster.NodeIDsToRemove = removedIDs

	b.emitEventLocked(input.ClusterName, EventSourceTypeCluster,
		fmt.Sprintf("Replication factor decreased to %d.", input.NewReplicationFactor))

	go func(cName string) {
		time.Sleep(time.Second)
		b.mu.Lock("DecreaseReplicationFactor:async")
		defer b.mu.Unlock()
		if c, exists := b.clusters.Get(cName); exists && c.Status == StatusModifying {
			c.Status = StatusAvailable
			c.NodeIDsToRemove = nil
		}
	}(input.ClusterName)

	return b.clusterCopy(cluster), nil
}

// RebootNode initiates a reboot of a specific node in a cluster.
func (b *InMemoryBackend) RebootNode(clusterName, nodeID string) (*Cluster, error) {
	if clusterName == "" {
		return nil, fmt.Errorf("%w: ClusterName is required", ErrInvalidParameterValue)
	}

	if nodeID == "" {
		return nil, fmt.Errorf("%w: NodeId is required", ErrInvalidParameterValue)
	}

	b.mu.Lock("RebootNode")
	defer b.mu.Unlock()

	cluster, ok := b.clusters.Get(clusterName)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrClusterNotFound, clusterName)
	}

	if cluster.Status != StatusAvailable {
		return nil, fmt.Errorf(
			"%w: cluster %s must be %s to reboot a node",
			ErrInvalidClusterState,
			clusterName,
			StatusAvailable,
		)
	}

	found := false

	for i := range cluster.Nodes {
		if cluster.Nodes[i].NodeID == nodeID {
			cluster.Nodes[i].NodeStatus = StatusRebooting
			found = true

			break
		}
	}

	if !found {
		return nil, fmt.Errorf("%w: node %s not found in cluster %s", ErrNodeNotFound, nodeID, clusterName)
	}

	b.emitEventLocked(clusterName, EventSourceTypeCluster,
		fmt.Sprintf("Node %s reboot initiated.", nodeID))

	go func() {
		time.Sleep(time.Second)
		b.mu.Lock("RebootNode:recovery")
		defer b.mu.Unlock()
		c, exists := b.clusters.Get(clusterName)
		if !exists {
			return
		}
		for i := range c.Nodes {
			if c.Nodes[i].NodeID == nodeID {
				c.Nodes[i].NodeStatus = StatusAvailable

				break
			}
		}
		b.emitEventLocked(clusterName, EventSourceTypeCluster,
			fmt.Sprintf("Node %s reboot complete.", nodeID))
	}()

	return b.clusterCopy(cluster), nil
}

// clusterCopy returns a deep copy of a Cluster.
func (b *InMemoryBackend) clusterCopy(c *Cluster) *Cluster {
	cp := *c
	cp.Tags = make(map[string]string)
	maps.Copy(cp.Tags, c.Tags)

	cp.SecurityGroupIDs = append([]string(nil), c.SecurityGroupIDs...)
	cp.NodeIDsToRemove = append([]string(nil), c.NodeIDsToRemove...)

	cp.Nodes = make([]Node, len(c.Nodes))
	for i, n := range c.Nodes {
		nodeCp := n

		if n.Endpoint != nil {
			endpointCp := *n.Endpoint
			nodeCp.Endpoint = &endpointCp
		}

		cp.Nodes[i] = nodeCp
	}

	if c.Endpoint != nil {
		endpointCp := *c.Endpoint
		cp.Endpoint = &endpointCp
	}

	if c.NotificationConfiguration != nil {
		ncCp := *c.NotificationConfiguration
		cp.NotificationConfiguration = &ncCp
	}

	return &cp
}

// removeNodesByAvailabilityZones removes nodes down to newFactor, preferring
// nodes whose AvailabilityZone is in azs (in cluster.Nodes order) and falling
// back to trailing nodes if too few nodes sit in the requested AZs. Mirrors
// AWS's DecreaseReplicationFactorInput.AvailabilityZones ("The Availability
// Zone(s) from which to remove nodes"), an alternative to NodeIdsToRemove.
func removeNodesByAvailabilityZones(nodes []Node, azs []string, newFactor int) ([]Node, []string) {
	azSet := make(map[string]bool, len(azs))
	for _, az := range azs {
		azSet[az] = true
	}

	removeCount := len(nodes) - newFactor
	removeSet := make(map[string]bool, removeCount)

	var removedIDs []string
	for _, n := range nodes {
		if len(removedIDs) >= removeCount {
			break
		}
		if azSet[n.AvailabilityZone] {
			removeSet[n.NodeID] = true
			removedIDs = append(removedIDs, n.NodeID)
		}
	}

	for i := len(nodes) - 1; i >= 0 && len(removedIDs) < removeCount; i-- {
		n := nodes[i]
		if !removeSet[n.NodeID] {
			removeSet[n.NodeID] = true
			removedIDs = append(removedIDs, n.NodeID)
		}
	}

	kept := make([]Node, 0, newFactor)
	for _, n := range nodes {
		if !removeSet[n.NodeID] {
			kept = append(kept, n)
		}
	}

	return kept, removedIDs
}

// removeSpecificNodes validates NodeIDsToRemove count and existence, then returns the kept nodes.
func removeSpecificNodes(nodes []Node, nodeIDsToRemove []string, clusterName string, newFactor int) ([]Node, error) {
	expectedRemoveCount := len(nodes) - newFactor
	if len(nodeIDsToRemove) != expectedRemoveCount {
		return nil, fmt.Errorf(
			"%w: NodeIDsToRemove has %d entries but %d nodes must be removed to reach factor %d",
			ErrInvalidParameterCombination,
			len(nodeIDsToRemove),
			expectedRemoveCount,
			newFactor,
		)
	}

	existingIDs := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		existingIDs[n.NodeID] = true
	}

	for _, id := range nodeIDsToRemove {
		if !existingIDs[id] {
			return nil, fmt.Errorf(
				"%w: node %s does not exist in cluster %s",
				ErrNodeNotFound, id, clusterName,
			)
		}
	}

	removeSet := make(map[string]bool, len(nodeIDsToRemove))
	for _, id := range nodeIDsToRemove {
		removeSet[id] = true
	}

	kept := make([]Node, 0, newFactor)
	for _, n := range nodes {
		if !removeSet[n.NodeID] {
			kept = append(kept, n)
		}
	}

	return kept, nil
}

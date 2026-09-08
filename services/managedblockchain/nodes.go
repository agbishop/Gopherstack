package managedblockchain

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// nodeStatusAvailable is the status for a ready node.
const nodeStatusAvailable = "AVAILABLE"

// defaultStateDB is the real API's documented default for
// NodeConfiguration.StateDB "When using an Amazon Managed Blockchain network
// with Hyperledger Fabric version 1.4 or later, the default is CouchDB."
// gopherstack only emulates Fabric 1.4 (see defaultFrameworkVersion), so
// this default always applies.
const defaultStateDB = "CouchDB"

// nodeARN builds the ARN for a Managed Blockchain node.
func nodeARN(region, accountID, nodeID string) string {
	return arn.Build("managedblockchain", region, accountID, "nodes/"+nodeID)
}

// nodePeerEndpoint synthesizes the endpoint exposed on
// Node.FrameworkAttributes.Fabric.PeerEndpoint. gopherstack has no real
// Fabric peer to connect to; this deterministically derives a plausible
// endpoint from the node's own identity, matching real AWS's
// "<node-id>.<member-id>.<network-id>...:30003" shape.
func nodePeerEndpoint(nodeID, memberID, networkID, region string) string {
	return fmt.Sprintf("%s.%s.%s.managedblockchain.%s.amazonaws.com:30003", nodeID, memberID, networkID, region)
}

// nodePeerEventEndpoint synthesizes the endpoint exposed on
// Node.FrameworkAttributes.Fabric.PeerEventEndpoint, matching real AWS's
// "<node-id>.<member-id>.<network-id>...:30004" shape.
func nodePeerEventEndpoint(nodeID, memberID, networkID, region string) string {
	return fmt.Sprintf("%s.%s.%s.managedblockchain.%s.amazonaws.com:30004", nodeID, memberID, networkID, region)
}

// resolveStateDB returns stateDB verbatim if the caller supplied one, or the
// real API's documented default otherwise.
func resolveStateDB(stateDB string) string {
	if stateDB == "" {
		return defaultStateDB
	}

	return stateDB
}

// cloneNode returns a deep copy of n with the Tags map cloned.
func cloneNode(n *Node) *Node {
	cp := *n
	cp.Tags = maps.Clone(n.Tags)
	cp.FrameworkAttributes = cloneNodeFrameworkAttributes(n.FrameworkAttributes)

	return &cp
}

// cloneNodeFrameworkAttributes returns a deep copy of a NodeFrameworkAttributesState.
func cloneNodeFrameworkAttributes(fa *NodeFrameworkAttributesState) *NodeFrameworkAttributesState {
	if fa == nil {
		return nil
	}

	cp := &NodeFrameworkAttributesState{}

	if fa.Fabric != nil {
		fabric := *fa.Fabric
		cp.Fabric = &fabric
	}

	return cp
}

// CreateNode creates a new peer node within a member. The node's KmsKeyArn
// is inherited from its owning member, matching real AWS ("The node
// inherits this parameter from the member that it belongs to.").
func (b *InMemoryBackend) CreateNode(
	region, accountID, networkID, memberID, instanceType, availabilityZone, stateDB string,
	tags map[string]string,
) (*Node, error) {
	b.mu.Lock("CreateNode")
	defer b.mu.Unlock()

	if err := checkTagLimit(nil, tags); err != nil {
		return nil, err
	}

	if _, exists := b.networks.Get(networkID); !exists {
		return nil, ErrNetworkNotFound
	}

	owner, exists := b.members.Get(memberKey(networkID, memberID))
	if !exists {
		return nil, ErrMemberNotFound
	}

	now := time.Now().UTC()
	nodeID := uuid.NewString()

	t := make(map[string]string)
	maps.Copy(t, tags)

	node := &Node{
		ID:               nodeID,
		Arn:              nodeARN(region, accountID, nodeID),
		NetworkID:        networkID,
		MemberID:         memberID,
		InstanceType:     instanceType,
		AvailabilityZone: availabilityZone,
		Status:           nodeStatusAvailable,
		CreationDate:     &now,
		Tags:             t,
		StateDB:          resolveStateDB(stateDB),
		KmsKeyArn:        owner.KmsKeyArn,
		FrameworkAttributes: &NodeFrameworkAttributesState{
			Fabric: &NodeFabricAttributesState{
				PeerEndpoint:      nodePeerEndpoint(nodeID, memberID, networkID, region),
				PeerEventEndpoint: nodePeerEventEndpoint(nodeID, memberID, networkID, region),
			},
		},
	}

	b.nodes.Put(node)
	b.arnToResource[node.Arn] = node

	return cloneNode(node), nil
}

// GetNode returns a node by network ID, member ID, and node ID.
func (b *InMemoryBackend) GetNode(networkID, memberID, nodeID string) (*Node, error) {
	b.mu.RLock("GetNode")
	defer b.mu.RUnlock()

	if _, exists := b.networks.Get(networkID); !exists {
		return nil, ErrNetworkNotFound
	}

	node, exists := b.nodes.Get(nodeKey(networkID, memberID, nodeID))
	if !exists {
		return nil, ErrNodeNotFound
	}

	return cloneNode(node), nil
}

// ListNodes returns all nodes for a member sorted by ID, optionally filtered.
func (b *InMemoryBackend) ListNodes(networkID, memberID string, filter ListNodesFilter) ([]*Node, error) {
	b.mu.RLock("ListNodes")
	defer b.mu.RUnlock()

	if _, exists := b.networks.Get(networkID); !exists {
		return nil, ErrNetworkNotFound
	}

	nodes := b.nodesByMember.Get(nodeMemberKey(networkID, memberID))
	all := make([]*Node, 0, len(nodes))

	for _, n := range nodes {
		if filter.Status != "" && n.Status != filter.Status {
			continue
		}

		all = append(all, cloneNode(n))
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].ID < all[j].ID
	})

	return all, nil
}

// DeleteNode removes a node from a member.
func (b *InMemoryBackend) DeleteNode(networkID, memberID, nodeID string) error {
	b.mu.Lock("DeleteNode")
	defer b.mu.Unlock()

	if _, exists := b.networks.Get(networkID); !exists {
		return ErrNetworkNotFound
	}

	node, exists := b.nodes.Get(nodeKey(networkID, memberID, nodeID))
	if !exists {
		return ErrNodeNotFound
	}

	delete(b.arnToResource, node.Arn)
	b.nodes.Delete(nodeKey(networkID, memberID, nodeID))

	return nil
}

// deleteNodesForMemberLocked cascade-deletes every node belonging to
// (networkID, memberID) from both the nodes table and the ARN index. The
// index's result slice is cloned before the delete loop since Table.Delete
// mutates the very index group being ranged over. Must be called with mu
// held. Called from members.go's DeleteMember and proposals.go's
// executeProposalActionsLocked removal-action cascade.
func (b *InMemoryBackend) deleteNodesForMemberLocked(networkID, memberID string) {
	nodes := slices.Clone(b.nodesByMember.Get(nodeMemberKey(networkID, memberID)))

	for _, node := range nodes {
		delete(b.arnToResource, node.Arn)
		b.nodes.Delete(nodeKey(node.NetworkID, node.MemberID, node.ID))
	}
}

// AddNodeInternal adds a node directly to the backend (for testing and seeding).
// The network and member must already exist.
func (b *InMemoryBackend) AddNodeInternal(region, accountID, networkID, memberID, instanceType string) *Node {
	b.mu.Lock("AddNodeInternal")
	defer b.mu.Unlock()

	now := time.Now().UTC()
	nodeID := uuid.NewString()

	node := &Node{
		ID:           nodeID,
		Arn:          nodeARN(region, accountID, nodeID),
		NetworkID:    networkID,
		MemberID:     memberID,
		InstanceType: instanceType,
		Status:       nodeStatusAvailable,
		CreationDate: &now,
		Tags:         make(map[string]string),
	}

	b.nodes.Put(node)
	b.arnToResource[node.Arn] = node

	return cloneNode(node)
}

// UpdateNode updates a node's log publishing configuration.
func (b *InMemoryBackend) UpdateNode(
	networkID, memberID, nodeID string,
	logConfig *NodeLogPublishingConfigState,
) (*Node, error) {
	b.mu.Lock("UpdateNode")
	defer b.mu.Unlock()

	if _, exists := b.networks.Get(networkID); !exists {
		return nil, ErrNetworkNotFound
	}

	node, exists := b.nodes.Get(nodeKey(networkID, memberID, nodeID))
	if !exists {
		return nil, ErrNodeNotFound
	}

	if logConfig != nil {
		node.LogPublishingConfiguration = cloneNodeLogConfig(logConfig)
	}

	return cloneNode(node), nil
}

// cloneNodeLogConfig returns a deep copy of NodeLogPublishingConfigState.
func cloneNodeLogConfig(c *NodeLogPublishingConfigState) *NodeLogPublishingConfigState {
	if c == nil {
		return nil
	}

	cp := &NodeLogPublishingConfigState{}

	if c.Fabric != nil {
		fabric := &NodeFabricLogState{}

		if c.Fabric.ChaincodeLogs != nil {
			fabric.ChaincodeLogs = cloneLogConfig(c.Fabric.ChaincodeLogs)
		}

		if c.Fabric.PeerLogs != nil {
			fabric.PeerLogs = cloneLogConfig(c.Fabric.PeerLogs)
		}

		cp.Fabric = fabric
	}

	return cp
}

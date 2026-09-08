package managedblockchain

import (
	"fmt"
	"maps"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

const (
	// networkStatusAvailable is the status for a ready network.
	networkStatusAvailable = "AVAILABLE"
	// defaultFramework is the default framework for new networks.
	defaultFramework = "HYPERLEDGER_FABRIC"
	// defaultFrameworkVersion is the default framework version.
	defaultFrameworkVersion = "1.4"
)

// networkARN builds the ARN for a Managed Blockchain network.
func networkARN(region, accountID, networkID string) string {
	return arn.Build("managedblockchain", region, accountID, "networks/"+networkID)
}

// fabricOrderingServiceEndpoint synthesizes the ordering-service endpoint
// exposed on Network.FrameworkAttributes.Fabric.OrderingServiceEndpoint.
// gopherstack has no real Fabric ordering service to connect to; this
// deterministically derives a plausible endpoint from the network's own
// identity, matching real AWS's "orderer.<network-id>...:30001" shape.
func fabricOrderingServiceEndpoint(networkID, region string) string {
	return fmt.Sprintf("orderer.%s.managedblockchain.%s.amazonaws.com:30001", networkID, region)
}

// networkVPCEndpointServiceName synthesizes the value exposed on
// Network.VpcEndpointServiceName. Real AWS assigns every AVAILABLE network a
// VPC PrivateLink endpoint service name regardless of framework
// configuration; gopherstack derives a deterministic placeholder from the
// network's own identity.
func networkVPCEndpointServiceName(networkID, region string) string {
	return fmt.Sprintf("com.amazonaws.managedblockchain.%s.%s", region, networkID)
}

// buildNetworkFrameworkAttributes builds Network.FrameworkAttributes from a
// CreateNetwork caller's requested Fabric edition. If edition is empty (the
// caller omitted FrameworkConfiguration entirely, which the real API's
// client-side validator permits), no Fabric attributes are synthesized --
// gopherstack does not invent an edition the caller never asked for.
func buildNetworkFrameworkAttributes(networkID, region, edition string) *NetworkFrameworkAttributesState {
	if edition == "" {
		return nil
	}

	return &NetworkFrameworkAttributesState{
		Fabric: &NetworkFabricAttributesState{
			Edition:                 edition,
			OrderingServiceEndpoint: fabricOrderingServiceEndpoint(networkID, region),
		},
	}
}

// CreateNetwork creates a new Managed Blockchain network and its first member.
func (b *InMemoryBackend) CreateNetwork(
	region, accountID, name, description, framework, frameworkVersion, memberName, memberDescription string,
	tags, memberTags map[string]string,
	votingPolicy *VotingPolicy,
	fabricEdition, memberAdminUsername, memberKmsKeyArn string,
) (*Network, *Member, error) {
	b.mu.Lock("CreateNetwork")
	defer b.mu.Unlock()

	if err := checkTagLimit(nil, tags); err != nil {
		return nil, nil, err
	}

	if err := checkTagLimit(nil, memberTags); err != nil {
		return nil, nil, err
	}

	for _, n := range b.networks.All() {
		if n.Name == name {
			return nil, nil, ErrNetworkAlreadyExists
		}
	}

	now := time.Now().UTC()
	networkID := uuid.NewString()
	memberID := uuid.NewString()

	fw := framework
	if fw == "" {
		fw = defaultFramework
	}

	fwv := frameworkVersion
	if fwv == "" {
		fwv = defaultFrameworkVersion
	}

	t := make(map[string]string)
	maps.Copy(t, tags)

	network := &Network{
		ID:                     networkID,
		Arn:                    networkARN(region, accountID, networkID),
		Name:                   name,
		Description:            description,
		Framework:              fw,
		FrameworkVersion:       fwv,
		Status:                 networkStatusAvailable,
		CreationDate:           &now,
		Tags:                   t,
		VotingPolicy:           cloneVotingPolicy(votingPolicy),
		FrameworkAttributes:    buildNetworkFrameworkAttributes(networkID, region, fabricEdition),
		VpcEndpointServiceName: networkVPCEndpointServiceName(networkID, region),
	}

	b.networks.Put(network)
	b.arnToResource[network.Arn] = network

	mt := make(map[string]string)
	maps.Copy(mt, memberTags)

	member := &Member{
		ID:                  memberID,
		Arn:                 memberARN(region, accountID, memberID),
		Name:                memberName,
		Description:         memberDescription,
		NetworkID:           networkID,
		Status:              memberStatusAvailable,
		CreationDate:        &now,
		Tags:                mt,
		IsOwned:             true,
		KmsKeyArn:           resolveMemberKmsKeyArn(memberKmsKeyArn),
		FrameworkAttributes: buildMemberFrameworkAttributes(memberID, networkID, region, memberAdminUsername),
	}

	b.members.Put(member)
	b.arnToResource[member.Arn] = member

	return cloneNetwork(network), cloneMember(member), nil
}

// cloneVotingPolicy returns a deep copy of a VotingPolicy.
func cloneVotingPolicy(vp *VotingPolicy) *VotingPolicy {
	if vp == nil {
		return nil
	}

	cp := *vp

	if vp.ApprovalThresholdPolicy != nil {
		atp := *vp.ApprovalThresholdPolicy
		cp.ApprovalThresholdPolicy = &atp
	}

	return &cp
}

// cloneNetwork returns a deep copy of n with the Tags map cloned.
func cloneNetwork(n *Network) *Network {
	cp := *n
	cp.Tags = maps.Clone(n.Tags)
	cp.VotingPolicy = cloneVotingPolicy(n.VotingPolicy)
	cp.FrameworkAttributes = cloneNetworkFrameworkAttributes(n.FrameworkAttributes)

	return &cp
}

// cloneNetworkFrameworkAttributes returns a deep copy of a NetworkFrameworkAttributesState.
func cloneNetworkFrameworkAttributes(fa *NetworkFrameworkAttributesState) *NetworkFrameworkAttributesState {
	if fa == nil {
		return nil
	}

	cp := &NetworkFrameworkAttributesState{}

	if fa.Fabric != nil {
		fabric := *fa.Fabric
		cp.Fabric = &fabric
	}

	return cp
}

// deleteNetworkIfEmptyLocked deletes network once it has no members left: "If
// MemberId is the last member in a network specified by the last Amazon Web
// Services account, the network is deleted also." (aws-sdk-go-v2 managedblockchain
// api_op_DeleteMember.go doc comment, v1.34.4). Must be called with mu held.
func deleteNetworkIfEmptyLocked(b *InMemoryBackend, network *Network) {
	if len(b.membersByNetwork.Get(network.ID)) > 0 {
		return
	}

	delete(b.arnToResource, network.Arn)
	b.networks.Delete(network.ID)
}

// GetNetwork returns the details of a network by ID.
func (b *InMemoryBackend) GetNetwork(networkID string) (*Network, error) {
	b.mu.RLock("GetNetwork")
	defer b.mu.RUnlock()

	network, exists := b.networks.Get(networkID)
	if !exists {
		return nil, ErrNetworkNotFound
	}

	return cloneNetwork(network), nil
}

// ListNetworks returns all networks, optionally filtered.
func (b *InMemoryBackend) ListNetworks(filter ListNetworksFilter) ([]*Network, error) {
	b.mu.RLock("ListNetworks")
	defer b.mu.RUnlock()

	all := make([]*Network, 0, b.networks.Len())

	for _, n := range b.networks.All() {
		if filter.Name != "" && n.Name != filter.Name {
			continue
		}

		if filter.Framework != "" && n.Framework != filter.Framework {
			continue
		}

		if filter.Status != "" && n.Status != filter.Status {
			continue
		}

		all = append(all, cloneNetwork(n))
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].Name < all[j].Name
	})

	return all, nil
}

// AddNetworkInternal adds a network directly to the backend (for testing and seeding).
func (b *InMemoryBackend) AddNetworkInternal(region, accountID, name string) *Network {
	b.mu.Lock("AddNetworkInternal")
	defer b.mu.Unlock()

	now := time.Now().UTC()
	networkID := uuid.NewString()

	network := &Network{
		ID:                     networkID,
		Arn:                    networkARN(region, accountID, networkID),
		Name:                   name,
		Framework:              defaultFramework,
		FrameworkVersion:       defaultFrameworkVersion,
		Status:                 networkStatusAvailable,
		CreationDate:           &now,
		Tags:                   make(map[string]string),
		VpcEndpointServiceName: networkVPCEndpointServiceName(networkID, region),
	}

	b.networks.Put(network)
	b.arnToResource[network.Arn] = network

	return cloneNetwork(network)
}

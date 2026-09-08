package ec2

// Code in this file supports Phase 3.3 of the datalayer refactor: every
// map[string]*T resource field on InMemoryBackend is registered exactly once,
// here, as a *store.Table[T] on b.registry. See pkgs/store's package doc and
// the services/sqs pilot (commit 0f09d77c) for the pattern this follows.
//
// A handful of fields are deliberately NOT registered here and remain plain
// maps -- see the comment block above registerAllTables for the list and why.
import (
	"strconv"

	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

func addressAttributesKeyFn(v *AddressAttribute) string { return v.AllocationID }
func addressesKeyFn(v *Address) string                  { return v.AllocationID }
func applicationStatusChecksKeyFn(v *ApplicationStatusCheck) string {
	return v.ApplicationStatusCheckID
}
func applicationStatusCheckAssociationsKeyFn(v *ApplicationStatusCheckAssociation) string {
	return appStatusCheckAssociationKeyFn(v)
}
func applicationStatusSuppressionsKeyFn(v *ApplicationStatusSuppression) string { return v.InstanceID }
func bundleTasksKeyFn(v *BundleTask) string                                     { return v.BundleID }
func byoipCidrsKeyFn(v *ByoipCidr) string                                       { return v.Cidr }
func capacityBlockExtensionOfferingsKeyFn(v *CapacityBlockExtensionOffering) string {
	return v.CapacityBlockExtensionOfferingID
}
func capacityBlockExtensionsKeyFn(v *CapacityBlockExtension) string {
	return v.CapacityBlockExtensionOfferingID
}
func capacityBlockOfferingsKeyFn(v *CapacityBlockOffering) string { return v.CapacityBlockOfferingID }
func capacityBlocksKeyFn(v *CapacityBlock) string                 { return v.CapacityBlockID }
func capacityManagerDataExportsKeyFn(v *CapacityManagerDataExport) string {
	return v.CapacityManagerDataExportID
}
func capacityReservationBillingRequestsKeyFn(v *CapacityReservationBillingRequest) string {
	return v.CapacityReservationID
}
func capacityReservationFleetsKeyFn(v *CapacityReservationFleet) string {
	return v.CapacityReservationFleetID
}
func capacityReservationCancellationQuotesKeyFn(v *CapacityReservationCancellationQuote) string {
	return v.CapacityReservationCancellationQuoteID
}
func capacityReservationsKeyFn(v *CapacityReservation) string             { return v.CapacityReservationID }
func carrierGatewaysKeyFn(v *CarrierGateway) string                       { return v.CarrierGatewayID }
func classicLinkInstancesKeyFn(v *ClassicLinkInstance) string             { return v.InstanceID }
func clientVpnEndpointsKeyFn(v *ClientVpnEndpoint) string                 { return v.ClientVpnEndpointID }
func coipCidrsKeyFn(v *CoipCidr) string                                   { return coipCidrKey(v.CoipPoolID, v.Cidr) }
func coipPoolsKeyFn(v *CoipPool) string                                   { return v.PoolID }
func conversionTasksKeyFn(v *ConversionTask) string                       { return v.ConversionTaskID }
func customerGatewaysKeyFn(v *CustomerGateway) string                     { return v.CustomerGatewayID }
func declarativePoliciesReportsKeyFn(v *DeclarativePoliciesReport) string { return v.ReportID }
func dedicatedHostsKeyFn(v *Host) string                                  { return v.HostID }
func dhcpOptionSetsKeyFn(v *DhcpOptions) string                           { return v.DhcpOptionsID }
func egressOnlyIGWsKeyFn(v *EgressOnlyInternetGateway) string             { return v.ID }
func endpointConnectionNotifsKeyFn(v *VpcEndpointConnectionNotification) string {
	return v.ConnectionNotificationID
}
func exportImageTasksKeyFn(v *ExportImageTaskRec) string           { return v.ExportImageTaskID }
func exportTasksKeyFn(v *ExportTask) string                        { return v.ExportTaskID }
func fleetsKeyFn(v *Fleet) string                                  { return v.FleetID }
func flowLogsKeyFn(v *FlowLog) string                              { return v.FlowLogID }
func fpgaImagesKeyFn(v *FpgaImage) string                          { return v.FpgaImageID }
func hostReservationsKeyFn(v *HostReservation) string              { return v.HostReservationID }
func iamAssociationsKeyFn(v *IamInstanceProfileAssociation) string { return v.AssociationID }
func imageImportTasksKeyFn(v *ImageImportTask) string              { return v.ImportTaskID }
func imagesKeyFn(v *AMIStub) string                                { return v.ImageID }
func instanceConnectEndpointsKeyFn(v *InstanceConnectEndpoint) string {
	return v.InstanceConnectEndpointID
}
func instanceEventWindowsKeyFn(v *InstanceEventWindow) string { return v.InstanceEventWindowID }
func instancesKeyFn(v *Instance) string                       { return v.ID }
func internetGatewaysKeyFn(v *InternetGateway) string         { return v.ID }
func interruptibleCRAllocationsKeyFn(v *InterruptibleCapacityReservationAllocation) string {
	return v.SourceCapacityReservationID
}
func ipamAsnAssociationsKeyFn(v *IpamAsnAssociation) string { return v.Asn + "|" + v.Cidr }
func ipamByoasnsKeyFn(v *IpamByoasn) string                 { return v.Asn }
func ipamPoliciesKeyFn(v *IpamPolicy) string                { return v.IpamPolicyID }
func ipamPoolAllocationsKeyFn(v *IpamPoolAllocation) string { return v.IpamPoolAllocationID }
func ipamPoolsKeyFn(v *IpamPool) string                     { return v.IpamPoolID }
func ipamPrefixListResolverTargetsKeyFn(v *IpamPrefixListResolverTarget) string {
	return v.IpamPrefixListResolverTargetID
}
func ipamPrefixListResolversKeyFn(v *IpamPrefixListResolver) string {
	return v.IpamPrefixListResolverID
}
func ipamResourceCidrsKeyFn(v *IpamResourceCidr) string {
	return ipamResourceCidrKey(v.ResourceID, v.ResourceCidr)
}
func ipamResourceDiscoveriesKeyFn(v *IpamResourceDiscovery) string { return v.IpamResourceDiscoveryID }
func ipamResourceDiscoveryAssocsKeyFn(v *IpamResourceDiscoveryAssociation) string {
	return v.IpamResourceDiscoveryAssociationID
}
func ipamScopesKeyFn(v *IpamScope) string { return v.IpamScopeID }
func ipamVerificationTokensKeyFn(v *IpamExternalResourceVerificationToken) string {
	return v.IpamExternalResourceVerificationTokenID
}
func ipamsKeyFn(v *Ipam) string                     { return v.IpamID }
func ipv4PoolsKeyFn(v *Ipv4Pool) string             { return v.PoolID }
func ipv6PoolsKeyFn(v *Ipv6Pool) string             { return v.PoolID }
func keyPairsKeyFn(v *KeyPair) string               { return v.Name }
func launchTemplatesKeyFn(v *LaunchTemplate) string { return v.ID }
func localGatewayRouteTableVifGroupAssociationsKeyFn(v *LocalGatewayRouteTableVirtualInterfaceGroupAssociation) string {
	return v.LocalGatewayRouteTableVirtualInterfaceGroupAssociationID
}
func localGatewayRouteTableVpcAssociationsKeyFn(v *LocalGatewayRouteTableVpcAssociation) string {
	return v.LocalGatewayRouteTableVpcAssociationID
}
func localGatewayRouteTablesKeyFn(v *LocalGatewayRouteTable) string {
	return v.LocalGatewayRouteTableID
}
func localGatewayRoutesKeyFn(v *LocalGatewayRoute) string {
	return localGatewayRouteKey(v.LocalGatewayRouteTableID, v.DestinationCidrBlock, v.DestinationPrefixListID)
}
func localGatewayVirtualInterfaceGroupsKeyFn(v *LocalGatewayVirtualInterfaceGroup) string {
	return v.LocalGatewayVirtualInterfaceGroupID
}
func localGatewayVirtualInterfacesKeyFn(v *LocalGatewayVirtualInterface) string {
	return v.LocalGatewayVirtualInterfaceID
}
func localGatewaysKeyFn(v *LocalGateway) string               { return v.LocalGatewayID }
func macModificationTasksKeyFn(v *MacModificationTask) string { return v.MacModificationTaskID }
func managedPrefixListsKeyFn(v *ManagedPrefixList) string     { return v.PrefixListID }
func movingAddressesKeyFn(v *MovingAddressStatus) string      { return v.PublicIP }
func natGatewaysKeyFn(v *NatGateway) string                   { return v.ID }
func networkACLsKeyFn(v *StoredNetworkACL) string             { return v.ID }
func networkInsightsAccessScopeAnalysesKeyFn(v *NetworkInsightsAccessScopeAnalysis) string {
	return v.NetworkInsightsAccessScopeAnalysisID
}
func networkInsightsAccessScopesKeyFn(v *NetworkInsightsAccessScope) string {
	return v.NetworkInsightsAccessScopeID
}
func networkInsightsAnalysesKeyFn(v *NetworkInsightsAnalysis) string {
	return v.NetworkInsightsAnalysisID
}
func networkInsightsPathsKeyFn(v *NetworkInsightsPath) string { return v.NetworkInsightsPathID }
func networkInterfacesKeyFn(v *NetworkInterface) string       { return v.ID }
func networkPerformanceSubscriptionsKeyFn(v *NetworkPerformanceSubscription) string {
	return networkPerformanceSubscriptionKey(v.Source, v.Destination, v.Metric, v.Statistic)
}
func niPermissionsKeyFn(v *NetworkInterfacePermission) string             { return v.PermissionID }
func outpostLagsKeyFn(v *OutpostLag) string                               { return v.OutpostLagID }
func placementGroupsKeyFn(v *PlacementGroup) string                       { return v.Name }
func recycleBinImagesKeyFn(v *RecycleBinImage) string                     { return v.ImageID }
func recycleBinSnapshotsKeyFn(v *Snapshot) string                         { return v.SnapshotID }
func recycleBinVolumesKeyFn(v *RecycleBinVolume) string                   { return v.VolumeID }
func replaceRootVolumeTasksKeyFn(v *ReplaceRootVolumeTask) string         { return v.ReplaceRootVolumeTaskID }
func reservedInstancesKeyFn(v *ReservedInstance) string                   { return v.ReservedInstancesID }
func reservedInstancesExchangesKeyFn(v *ReservedInstancesExchange) string { return v.ExchangeID }
func reservedInstancesListingsKeyFn(v *ReservedInstancesListing) string {
	return v.ReservedInstancesListingID
}
func reservedInstancesModificationsKeyFn(v *ReservedInstancesModification) string {
	return v.ReservedInstancesModificationID
}
func reservedInstancesOfferingsKeyFn(v *ReservedInstancesOffering) string {
	return v.ReservedInstancesOfferingID
}
func routeServerAssociationsKeyFn(v *RouteServerAssociation) string {
	return v.RouteServerID + "/" + v.VpcID
}
func routeServerEndpointsKeyFn(v *RouteServerEndpoint) string { return v.RouteServerEndpointID }
func routeServerPeersKeyFn(v *RouteServerPeer) string         { return v.RouteServerPeerID }
func routeServerPropagationsKeyFn(v *RouteServerPropagation) string {
	return v.RouteServerID + "/" + v.RouteTableID
}
func routeServersKeyFn(v *RouteServer) string               { return v.RouteServerID }
func routeTablesKeyFn(v *RouteTable) string                 { return v.ID }
func scheduledInstancesKeyFn(v *ScheduledInstance) string   { return v.ScheduledInstanceID }
func secondaryInterfacesKeyFn(v *SecondaryInterface) string { return v.SecondaryInterfaceID }
func secondaryNetworksKeyFn(v *SecondaryNetwork) string     { return v.SecondaryNetworkID }
func secondarySubnetsKeyFn(v *SecondarySubnet) string       { return v.SecondarySubnetID }
func securityGroupsKeyFn(v *SecurityGroup) string           { return v.ID }
func serviceLinkVirtualInterfacesKeyFn(v *ServiceLinkVirtualInterface) string {
	return v.ServiceLinkVirtualInterfaceID
}
func snapshotImportTasksKeyFn(v *SnapshotImportTask) string     { return v.ImportTaskID }
func snapshotLocksKeyFn(v *SnapshotLock) string                 { return v.SnapshotID }
func snapshotsKeyFn(v *Snapshot) string                         { return v.SnapshotID }
func spotFleetsKeyFn(v *SpotFleetRequest) string                { return v.SpotFleetRequestID }
func spotRequestsKeyFn(v *SpotInstanceRequest) string           { return v.ID }
func sqlHaRegistrationsKeyFn(v *RegisteredSQLHaInstance) string { return v.InstanceID }
func storeImageTasksKeyFn(v *StoreImageTask) string             { return v.AmiID }
func subnetsKeyFn(v *Subnet) string                             { return v.ID }
func tgwClientVpnAttachmentsKeyFn(v *TransitGatewayClientVpnAttachment) string {
	return v.TransitGatewayAttachmentID
}
func tgwConnectPeersKeyFn(v *TransitGatewayConnectPeer) string        { return v.TransitGatewayConnectPeerID }
func tgwConnectsKeyFn(v *TransitGatewayConnect) string                { return v.TransitGatewayAttachmentID }
func tgwMeteringPoliciesKeyFn(v *TransitGatewayMeteringPolicy) string { return v.ID }
func tgwMeteringPolicyEntriesKeyFn(v *TransitGatewayMeteringPolicyEntry) string {
	return v.TransitGatewayMeteringPolicyID + ":" + strconv.Itoa(v.PolicyRuleNumber)
}
func tgwMulticastDomainAssociationsKeyFn(v *TransitGatewayMulticastDomainAssociation) string {
	return v.TransitGatewayMulticastDomainID + ":" + v.SubnetID
}
func tgwMulticastDomainsKeyFn(v *TransitGatewayMulticastDomain) string { return v.ID }
func tgwMulticastGroupEntriesKeyFn(v *TransitGatewayMulticastGroupEntry) string {
	return v.TransitGatewayMulticastDomainID + ":" + v.GroupIPAddress + ":" + v.NetworkInterfaceID
}
func tgwPeeringAttachmentsKeyFn(v *TransitGatewayPeeringAttachment) string {
	return v.TransitGatewayAttachmentID
}
func tgwPolicyTableAssociationsKeyFn(v *TransitGatewayPolicyTableAssociation) string {
	return v.TransitGatewayPolicyTableID + ":" + v.TransitGatewayAttachmentID
}
func tgwPolicyTablesKeyFn(v *TransitGatewayPolicyTable) string { return v.TransitGatewayPolicyTableID }
func tgwPolicyTableEntriesKeyFn(v *TransitGatewayPolicyTableEntry) string {
	return v.TransitGatewayPolicyTableID + ":" + strconv.Itoa(v.PolicyRuleNumber)
}
func tgwPrefixListRefsKeyFn(v *TransitGatewayPrefixListReference) string {
	return v.TransitGatewayRouteTableID + "/" + v.PrefixListID
}
func tgwRTAssociationsKeyFn(v *TransitGatewayRouteTableAssociation) string {
	return v.TransitGatewayRouteTableID + ":" + v.TransitGatewayAttachmentID
}
func tgwRouteTableAnnouncementsKeyFn(v *TransitGatewayRouteTableAnnouncement) string {
	return v.TransitGatewayRouteTableAnnouncementID
}
func tgwRouteTablesKeyFn(v *TransitGatewayRouteTable) string { return v.RouteTableID }
func tgwRoutesKeyFn(v *TransitGatewayRoute) string {
	return v.TransitGatewayRouteTableID + ":" + v.DestinationCidrBlock
}
func tgwVpcAttachmentsKeyFn(v *TransitGatewayVpcAttachment) string {
	return v.TransitGatewayAttachmentID
}
func trafficMirrorFilterRulesKeyFn(v *TrafficMirrorFilterRule) string {
	return v.TrafficMirrorFilterRuleID
}
func trafficMirrorFiltersKeyFn(v *TrafficMirrorFilter) string             { return v.TrafficMirrorFilterID }
func trafficMirrorSessionsKeyFn(v *TrafficMirrorSession) string           { return v.TrafficMirrorSessionID }
func trafficMirrorTargetsKeyFn(v *TrafficMirrorTarget) string             { return v.TrafficMirrorTargetID }
func transitGatewaysKeyFn(v *TransitGateway) string                       { return v.ID }
func trunkInterfaceAssociationsKeyFn(v *TrunkInterfaceAssociation) string { return v.AssociationID }
func usageReportsKeyFn(v *UsageReport) string                             { return v.ReportID }
func verifiedAccessEndpointsKeyFn(v *VerifiedAccessEndpoint) string {
	return v.VerifiedAccessEndpointID
}
func verifiedAccessGroupsKeyFn(v *VerifiedAccessGroup) string { return v.VerifiedAccessGroupID }
func verifiedAccessInstanceLoggingConfigsKeyFn(v *VerifiedAccessInstanceLoggingConfig) string {
	return v.VerifiedAccessInstanceID
}
func verifiedAccessInstancesKeyFn(v *VerifiedAccessInstance) string {
	return v.VerifiedAccessInstanceID
}
func verifiedAccessTrustProvidersKeyFn(v *VerifiedAccessTrustProvider) string {
	return v.VerifiedAccessTrustProviderID
}
func volumeModificationsKeyFn(v *VolumeModification) string { return v.VolumeID }
func volumesKeyFn(v *Volume) string                         { return v.ID }
func vpcBlockPublicAccessExclusionsKeyFn(v *VpcBlockPublicAccessExclusion) string {
	return v.ExclusionID
}
func vpcEncryptionControlsKeyFn(v *VpcEncryptionControl) string { return v.VpcEncryptionControlID }
func vpcEndpointConnectionsKeyFn(v *VpcEndpointConnection) string {
	return v.ServiceID + ":" + v.VpcEndpointID
}
func vpcEndpointServiceConfigsKeyFn(v *VpcEndpointServiceConfig) string { return v.ServiceID }
func vpcEndpointsKeyFn(v *VpcEndpoint) string                           { return v.ID }
func vpcPeeringConnectionsKeyFn(v *VpcPeeringConnection) string         { return v.VpcPeeringConnectionID }
func vpcsKeyFn(v *VPC) string                                           { return v.ID }
func vpnConcentratorsKeyFn(v *VpnConcentrator) string                   { return v.VpnConcentratorID }
func vpnConnectionRoutesKeyFn(v *VpnConnectionRoute) string {
	return v.VpnConnectionID + ":" + v.DestinationCIDR
}
func vpnConnectionsKeyFn(v *VpnConnection) string { return v.VpnConnectionID }
func vpnGatewaysKeyFn(v *VpnGateway) string       { return v.VpnGatewayID }

// registerAllTables registers every converted resource map on b.registry exactly
// once, during construction only -- store.Register panics on a duplicate name, so
// runtime resets go through registry.ResetAll() instead (see InMemoryBackend.Reset
// in accept_ops.go).
//
// addressTransfers, verifiedAccessEndpointPolicies, verifiedAccessGroupPolicies,
// vpcCidrAssociations, and vpcPeeringOptions stay plain maps here: their key
// isn't a pure function of the stored value's own fields, which store.Table
// requires.
func registerAllTables(b *InMemoryBackend) {
	for _, register := range tableRegistrations {
		register(b)
	}
}

// tableRegistrations is the data-driven list registerAllTables walks: one
// closure per resource table, each binding its own store.New/store.Register
// call to the concrete field and value type. A closure list (rather than one
// statement per field in a single function body) keeps registerAllTables small
// regardless of how many resource tables the backend grows to.
//
//nolint:gochecknoglobals // registration table, analogous to errCodeLookup-style lookup tables elsewhere
var tableRegistrations = []func(*InMemoryBackend){
	func(b *InMemoryBackend) {
		b.addressAttributes = store.Register(b.registry, "addressAttributes", store.New(addressAttributesKeyFn))
	},
	func(b *InMemoryBackend) {
		b.addresses = store.Register(b.registry, "addresses", store.New(addressesKeyFn))
	},
	func(b *InMemoryBackend) {
		b.bundleTasks = store.Register(b.registry, "bundleTasks", store.New(bundleTasksKeyFn))
	},
	func(b *InMemoryBackend) {
		b.byoipCidrs = store.Register(b.registry, "byoipCidrs", store.New(byoipCidrsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.capacityBlockExtensionOfferings = store.Register(
			b.registry,
			"capacityBlockExtensionOfferings",
			store.New(capacityBlockExtensionOfferingsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.capacityBlockExtensions = store.Register(
			b.registry,
			"capacityBlockExtensions",
			store.New(capacityBlockExtensionsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.capacityBlockOfferings = store.Register(
			b.registry,
			"capacityBlockOfferings",
			store.New(capacityBlockOfferingsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.capacityBlocks = store.Register(b.registry, "capacityBlocks", store.New(capacityBlocksKeyFn))
	},
	func(b *InMemoryBackend) {
		b.capacityManagerDataExports = store.Register(
			b.registry,
			"capacityManagerDataExports",
			store.New(capacityManagerDataExportsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.capacityReservationCancellationQuotes = store.Register(
			b.registry,
			"capacityReservationCancellationQuotes",
			store.New(capacityReservationCancellationQuotesKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.capacityReservationBillingRequests = store.Register(
			b.registry,
			"capacityReservationBillingRequests",
			store.New(capacityReservationBillingRequestsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.capacityReservationFleets = store.Register(
			b.registry,
			"capacityReservationFleets",
			store.New(capacityReservationFleetsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.capacityReservations = store.Register(
			b.registry,
			"capacityReservations",
			store.New(capacityReservationsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.carrierGateways = store.Register(b.registry, "carrierGateways", store.New(carrierGatewaysKeyFn))
	},
	func(b *InMemoryBackend) {
		b.classicLinkInstances = store.Register(
			b.registry,
			"classicLinkInstances",
			store.New(classicLinkInstancesKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.clientVpnEndpoints = store.Register(b.registry, "clientVpnEndpoints", store.New(clientVpnEndpointsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.coipCidrs = store.Register(b.registry, "coipCidrs", store.New(coipCidrsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.coipPools = store.Register(b.registry, "coipPools", store.New(coipPoolsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.conversionTasks = store.Register(b.registry, "conversionTasks", store.New(conversionTasksKeyFn))
	},
	func(b *InMemoryBackend) {
		b.customerGateways = store.Register(b.registry, "customerGateways", store.New(customerGatewaysKeyFn))
	},
	func(b *InMemoryBackend) {
		b.declarativePoliciesReports = store.Register(
			b.registry,
			"declarativePoliciesReports",
			store.New(declarativePoliciesReportsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.dedicatedHosts = store.Register(b.registry, "dedicatedHosts", store.New(dedicatedHostsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.dhcpOptionSets = store.Register(b.registry, "dhcpOptionSets", store.New(dhcpOptionSetsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.egressOnlyIGWs = store.Register(b.registry, "egressOnlyIGWs", store.New(egressOnlyIGWsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.endpointConnectionNotifs = store.Register(
			b.registry,
			"endpointConnectionNotifs",
			store.New(endpointConnectionNotifsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.exportImageTasks = store.Register(b.registry, "exportImageTasks", store.New(exportImageTasksKeyFn))
	},
	func(b *InMemoryBackend) {
		b.exportTasks = store.Register(b.registry, "exportTasks", store.New(exportTasksKeyFn))
	},
	func(b *InMemoryBackend) {
		b.fleets = store.Register(b.registry, "fleets", store.New(fleetsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.flowLogs = store.Register(b.registry, "flowLogs", store.New(flowLogsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.fpgaImages = store.Register(b.registry, "fpgaImages", store.New(fpgaImagesKeyFn))
	},
	func(b *InMemoryBackend) {
		b.hostReservations = store.Register(b.registry, "hostReservations", store.New(hostReservationsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.iamAssociations = store.Register(b.registry, "iamAssociations", store.New(iamAssociationsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.imageImportTasks = store.Register(b.registry, "imageImportTasks", store.New(imageImportTasksKeyFn))
	},
	func(b *InMemoryBackend) {
		b.images = store.Register(b.registry, "images", store.New(imagesKeyFn))
	},
	func(b *InMemoryBackend) {
		b.instanceConnectEndpoints = store.Register(
			b.registry,
			"instanceConnectEndpoints",
			store.New(instanceConnectEndpointsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.instanceEventWindows = store.Register(
			b.registry,
			"instanceEventWindows",
			store.New(instanceEventWindowsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.instances = store.Register(b.registry, "instances", store.New(instancesKeyFn))
	},
	func(b *InMemoryBackend) {
		b.internetGateways = store.Register(b.registry, "internetGateways", store.New(internetGatewaysKeyFn))
	},
	func(b *InMemoryBackend) {
		b.interruptibleCRAllocations = store.Register(
			b.registry,
			"interruptibleCRAllocations",
			store.New(interruptibleCRAllocationsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.ipamAsnAssociations = store.Register(b.registry, "ipamAsnAssociations", store.New(ipamAsnAssociationsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.ipamByoasns = store.Register(b.registry, "ipamByoasns", store.New(ipamByoasnsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.ipamPolicies = store.Register(b.registry, "ipamPolicies", store.New(ipamPoliciesKeyFn))
	},
	func(b *InMemoryBackend) {
		b.ipamPoolAllocations = store.Register(b.registry, "ipamPoolAllocations", store.New(ipamPoolAllocationsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.ipamPools = store.Register(b.registry, "ipamPools", store.New(ipamPoolsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.ipamPrefixListResolverTargets = store.Register(
			b.registry,
			"ipamPrefixListResolverTargets",
			store.New(ipamPrefixListResolverTargetsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.ipamPrefixListResolvers = store.Register(
			b.registry,
			"ipamPrefixListResolvers",
			store.New(ipamPrefixListResolversKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.ipamResourceCidrs = store.Register(b.registry, "ipamResourceCidrs", store.New(ipamResourceCidrsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.ipamResourceDiscoveries = store.Register(
			b.registry,
			"ipamResourceDiscoveries",
			store.New(ipamResourceDiscoveriesKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.ipamResourceDiscoveryAssocs = store.Register(
			b.registry,
			"ipamResourceDiscoveryAssocs",
			store.New(ipamResourceDiscoveryAssocsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.ipamScopes = store.Register(b.registry, "ipamScopes", store.New(ipamScopesKeyFn))
	},
	func(b *InMemoryBackend) {
		b.ipamVerificationTokens = store.Register(
			b.registry,
			"ipamVerificationTokens",
			store.New(ipamVerificationTokensKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.ipams = store.Register(b.registry, "ipams", store.New(ipamsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.ipv4Pools = store.Register(b.registry, "ipv4Pools", store.New(ipv4PoolsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.ipv6Pools = store.Register(b.registry, "ipv6Pools", store.New(ipv6PoolsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.keyPairs = store.Register(b.registry, "keyPairs", store.New(keyPairsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.launchTemplates = store.Register(b.registry, "launchTemplates", store.New(launchTemplatesKeyFn))
	},
	func(b *InMemoryBackend) {
		b.localGatewayRouteTableVifGroupAssociations = store.Register(
			b.registry,
			"localGatewayRouteTableVifGroupAssociations",
			store.New(localGatewayRouteTableVifGroupAssociationsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.localGatewayRouteTableVpcAssociations = store.Register(
			b.registry,
			"localGatewayRouteTableVpcAssociations",
			store.New(localGatewayRouteTableVpcAssociationsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.localGatewayRouteTables = store.Register(
			b.registry,
			"localGatewayRouteTables",
			store.New(localGatewayRouteTablesKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.localGatewayRoutes = store.Register(b.registry, "localGatewayRoutes", store.New(localGatewayRoutesKeyFn))
	},
	func(b *InMemoryBackend) {
		b.localGatewayVirtualInterfaceGroups = store.Register(
			b.registry,
			"localGatewayVirtualInterfaceGroups",
			store.New(localGatewayVirtualInterfaceGroupsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.localGatewayVirtualInterfaces = store.Register(
			b.registry,
			"localGatewayVirtualInterfaces",
			store.New(localGatewayVirtualInterfacesKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.localGateways = store.Register(b.registry, "localGateways", store.New(localGatewaysKeyFn))
	},
	func(b *InMemoryBackend) {
		b.macModificationTasks = store.Register(
			b.registry,
			"macModificationTasks",
			store.New(macModificationTasksKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.managedPrefixLists = store.Register(b.registry, "managedPrefixLists", store.New(managedPrefixListsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.movingAddresses = store.Register(b.registry, "movingAddresses", store.New(movingAddressesKeyFn))
	},
	func(b *InMemoryBackend) {
		b.natGateways = store.Register(b.registry, "natGateways", store.New(natGatewaysKeyFn))
	},
	func(b *InMemoryBackend) {
		b.networkACLs = store.Register(b.registry, "networkACLs", store.New(networkACLsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.networkInsightsAccessScopeAnalyses = store.Register(
			b.registry,
			"networkInsightsAccessScopeAnalyses",
			store.New(networkInsightsAccessScopeAnalysesKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.networkInsightsAccessScopes = store.Register(
			b.registry,
			"networkInsightsAccessScopes",
			store.New(networkInsightsAccessScopesKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.networkInsightsAnalyses = store.Register(
			b.registry,
			"networkInsightsAnalyses",
			store.New(networkInsightsAnalysesKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.networkInsightsPaths = store.Register(
			b.registry,
			"networkInsightsPaths",
			store.New(networkInsightsPathsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.networkInterfaces = store.Register(b.registry, "networkInterfaces", store.New(networkInterfacesKeyFn))
	},
	func(b *InMemoryBackend) {
		b.networkPerformanceSubscriptions = store.Register(
			b.registry,
			"networkPerformanceSubscriptions",
			store.New(networkPerformanceSubscriptionsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.niPermissions = store.Register(b.registry, "niPermissions", store.New(niPermissionsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.outpostLags = store.Register(b.registry, "outpostLags", store.New(outpostLagsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.placementGroups = store.Register(b.registry, "placementGroups", store.New(placementGroupsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.recycleBinImages = store.Register(b.registry, "recycleBinImages", store.New(recycleBinImagesKeyFn))
	},
	func(b *InMemoryBackend) {
		b.recycleBinSnapshots = store.Register(b.registry, "recycleBinSnapshots", store.New(recycleBinSnapshotsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.recycleBinVolumes = store.Register(b.registry, "recycleBinVolumes", store.New(recycleBinVolumesKeyFn))
	},
	func(b *InMemoryBackend) {
		b.replaceRootVolumeTasks = store.Register(
			b.registry,
			"replaceRootVolumeTasks",
			store.New(replaceRootVolumeTasksKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.reservedInstances = store.Register(b.registry, "reservedInstances", store.New(reservedInstancesKeyFn))
	},
	func(b *InMemoryBackend) {
		b.reservedInstancesExchanges = store.Register(
			b.registry,
			"reservedInstancesExchanges",
			store.New(reservedInstancesExchangesKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.reservedInstancesListings = store.Register(
			b.registry,
			"reservedInstancesListings",
			store.New(reservedInstancesListingsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.reservedInstancesModifications = store.Register(
			b.registry,
			"reservedInstancesModifications",
			store.New(reservedInstancesModificationsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.reservedInstancesOfferings = store.Register(
			b.registry,
			"reservedInstancesOfferings",
			store.New(reservedInstancesOfferingsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.routeServerAssociations = store.Register(
			b.registry,
			"routeServerAssociations",
			store.New(routeServerAssociationsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.routeServerEndpoints = store.Register(
			b.registry,
			"routeServerEndpoints",
			store.New(routeServerEndpointsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.routeServerPeers = store.Register(b.registry, "routeServerPeers", store.New(routeServerPeersKeyFn))
	},
	func(b *InMemoryBackend) {
		b.routeServerPropagations = store.Register(
			b.registry,
			"routeServerPropagations",
			store.New(routeServerPropagationsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.routeServers = store.Register(b.registry, "routeServers", store.New(routeServersKeyFn))
	},
	func(b *InMemoryBackend) {
		b.routeTables = store.Register(b.registry, "routeTables", store.New(routeTablesKeyFn))
	},
	func(b *InMemoryBackend) {
		b.scheduledInstances = store.Register(b.registry, "scheduledInstances", store.New(scheduledInstancesKeyFn))
	},
	func(b *InMemoryBackend) {
		b.secondaryInterfaces = store.Register(b.registry, "secondaryInterfaces", store.New(secondaryInterfacesKeyFn))
	},
	func(b *InMemoryBackend) {
		b.secondaryNetworks = store.Register(b.registry, "secondaryNetworks", store.New(secondaryNetworksKeyFn))
	},
	func(b *InMemoryBackend) {
		b.secondarySubnets = store.Register(b.registry, "secondarySubnets", store.New(secondarySubnetsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.securityGroups = store.Register(b.registry, "securityGroups", store.New(securityGroupsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.serviceLinkVirtualInterfaces = store.Register(
			b.registry,
			"serviceLinkVirtualInterfaces",
			store.New(serviceLinkVirtualInterfacesKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.snapshotImportTasks = store.Register(b.registry, "snapshotImportTasks", store.New(snapshotImportTasksKeyFn))
	},
	func(b *InMemoryBackend) {
		b.snapshotLocks = store.Register(b.registry, "snapshotLocks", store.New(snapshotLocksKeyFn))
	},
	func(b *InMemoryBackend) {
		b.snapshots = store.Register(b.registry, "snapshots", store.New(snapshotsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.spotFleets = store.Register(b.registry, "spotFleets", store.New(spotFleetsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.spotRequests = store.Register(b.registry, "spotRequests", store.New(spotRequestsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.sqlHaRegistrations = store.Register(b.registry, "sqlHaRegistrations", store.New(sqlHaRegistrationsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.storeImageTasks = store.Register(b.registry, "storeImageTasks", store.New(storeImageTasksKeyFn))
	},
	func(b *InMemoryBackend) {
		b.subnets = store.Register(b.registry, "subnets", store.New(subnetsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.tgwClientVpnAttachments = store.Register(
			b.registry,
			"tgwClientVpnAttachments",
			store.New(tgwClientVpnAttachmentsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.tgwConnectPeers = store.Register(b.registry, "tgwConnectPeers", store.New(tgwConnectPeersKeyFn))
	},
	func(b *InMemoryBackend) {
		b.tgwConnects = store.Register(b.registry, "tgwConnects", store.New(tgwConnectsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.tgwMeteringPolicies = store.Register(b.registry, "tgwMeteringPolicies", store.New(tgwMeteringPoliciesKeyFn))
	},
	func(b *InMemoryBackend) {
		b.tgwMeteringPolicyEntries = store.Register(
			b.registry,
			"tgwMeteringPolicyEntries",
			store.New(tgwMeteringPolicyEntriesKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.tgwMulticastDomainAssociations = store.Register(
			b.registry,
			"tgwMulticastDomainAssociations",
			store.New(tgwMulticastDomainAssociationsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.tgwMulticastDomains = store.Register(b.registry, "tgwMulticastDomains", store.New(tgwMulticastDomainsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.tgwMulticastGroupEntries = store.Register(
			b.registry,
			"tgwMulticastGroupEntries",
			store.New(tgwMulticastGroupEntriesKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.tgwPeeringAttachments = store.Register(
			b.registry,
			"tgwPeeringAttachments",
			store.New(tgwPeeringAttachmentsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.tgwPolicyTableAssociations = store.Register(
			b.registry,
			"tgwPolicyTableAssociations",
			store.New(tgwPolicyTableAssociationsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.tgwPolicyTables = store.Register(b.registry, "tgwPolicyTables", store.New(tgwPolicyTablesKeyFn))
	},
	func(b *InMemoryBackend) {
		b.tgwPolicyTableEntries = store.Register(
			b.registry,
			"tgwPolicyTableEntries",
			store.New(tgwPolicyTableEntriesKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.tgwPrefixListRefs = store.Register(b.registry, "tgwPrefixListRefs", store.New(tgwPrefixListRefsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.tgwRTAssociations = store.Register(b.registry, "tgwRTAssociations", store.New(tgwRTAssociationsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.tgwRouteTableAnnouncements = store.Register(
			b.registry,
			"tgwRouteTableAnnouncements",
			store.New(tgwRouteTableAnnouncementsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.tgwRouteTables = store.Register(b.registry, "tgwRouteTables", store.New(tgwRouteTablesKeyFn))
	},
	func(b *InMemoryBackend) {
		b.tgwRoutes = store.Register(b.registry, "tgwRoutes", store.New(tgwRoutesKeyFn))
	},
	func(b *InMemoryBackend) {
		b.tgwVpcAttachments = store.Register(b.registry, "tgwVpcAttachments", store.New(tgwVpcAttachmentsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.trafficMirrorFilterRules = store.Register(
			b.registry,
			"trafficMirrorFilterRules",
			store.New(trafficMirrorFilterRulesKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.trafficMirrorFilters = store.Register(
			b.registry,
			"trafficMirrorFilters",
			store.New(trafficMirrorFiltersKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.trafficMirrorSessions = store.Register(
			b.registry,
			"trafficMirrorSessions",
			store.New(trafficMirrorSessionsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.trafficMirrorTargets = store.Register(
			b.registry,
			"trafficMirrorTargets",
			store.New(trafficMirrorTargetsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.transitGateways = store.Register(b.registry, "transitGateways", store.New(transitGatewaysKeyFn))
	},
	func(b *InMemoryBackend) {
		b.trunkInterfaceAssociations = store.Register(
			b.registry,
			"trunkInterfaceAssociations",
			store.New(trunkInterfaceAssociationsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.usageReports = store.Register(b.registry, "usageReports", store.New(usageReportsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.verifiedAccessEndpoints = store.Register(
			b.registry,
			"verifiedAccessEndpoints",
			store.New(verifiedAccessEndpointsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.verifiedAccessGroups = store.Register(
			b.registry,
			"verifiedAccessGroups",
			store.New(verifiedAccessGroupsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.verifiedAccessInstanceLoggingConfigs = store.Register(
			b.registry,
			"verifiedAccessInstanceLoggingConfigs",
			store.New(verifiedAccessInstanceLoggingConfigsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.verifiedAccessInstances = store.Register(
			b.registry,
			"verifiedAccessInstances",
			store.New(verifiedAccessInstancesKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.verifiedAccessTrustProviders = store.Register(
			b.registry,
			"verifiedAccessTrustProviders",
			store.New(verifiedAccessTrustProvidersKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.volumeModifications = store.Register(b.registry, "volumeModifications", store.New(volumeModificationsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.volumes = store.Register(b.registry, "volumes", store.New(volumesKeyFn))
	},
	func(b *InMemoryBackend) {
		b.vpcBlockPublicAccessExclusions = store.Register(
			b.registry,
			"vpcBlockPublicAccessExclusions",
			store.New(vpcBlockPublicAccessExclusionsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.vpcEncryptionControls = store.Register(
			b.registry,
			"vpcEncryptionControls",
			store.New(vpcEncryptionControlsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.vpcEndpointConnections = store.Register(
			b.registry,
			"vpcEndpointConnections",
			store.New(vpcEndpointConnectionsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.vpcEndpointServiceConfigs = store.Register(
			b.registry,
			"vpcEndpointServiceConfigs",
			store.New(vpcEndpointServiceConfigsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.vpcEndpoints = store.Register(b.registry, "vpcEndpoints", store.New(vpcEndpointsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.vpcPeeringConnections = store.Register(
			b.registry,
			"vpcPeeringConnections",
			store.New(vpcPeeringConnectionsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.vpcs = store.Register(b.registry, "vpcs", store.New(vpcsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.vpnConcentrators = store.Register(b.registry, "vpnConcentrators", store.New(vpnConcentratorsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.vpnConnectionRoutes = store.Register(b.registry, "vpnConnectionRoutes", store.New(vpnConnectionRoutesKeyFn))
	},
	func(b *InMemoryBackend) {
		b.vpnConnections = store.Register(b.registry, "vpnConnections", store.New(vpnConnectionsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.vpnGateways = store.Register(b.registry, "vpnGateways", store.New(vpnGatewaysKeyFn))
	},
	func(b *InMemoryBackend) {
		b.applicationStatusChecks = store.Register(
			b.registry,
			"applicationStatusChecks",
			store.New(applicationStatusChecksKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.applicationStatusCheckAssociations = store.Register(
			b.registry,
			"applicationStatusCheckAssociations",
			store.New(applicationStatusCheckAssociationsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.applicationStatusSuppressions = store.Register(
			b.registry,
			"applicationStatusSuppressions",
			store.New(applicationStatusSuppressionsKeyFn),
		)
	},
}

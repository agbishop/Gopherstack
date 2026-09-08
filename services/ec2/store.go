package ec2

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// Errors returned by the EC2 backend.
var (
	ErrInstanceNotFound      = errors.New("InvalidInstanceID.NotFound")
	ErrSecurityGroupNotFound = errors.New("InvalidGroup.NotFound")
	ErrVPCNotFound           = errors.New("InvalidVpcID.NotFound")
	ErrSubnetNotFound        = errors.New("InvalidSubnetID.NotFound")
	ErrInvalidParameter      = errors.New("InvalidParameterValue")
	ErrDuplicateSGName       = errors.New("InvalidGroup.Duplicate")
	ErrInvalidInstanceState  = errors.New("IncorrectInstanceState")
	ErrSpotFleetNotFound     = errors.New("InvalidSpotFleetRequestId.NotFound")
	ErrCIDRConflict          = errors.New("InvalidVpc.Conflict")
	ErrDryRunOperation       = errors.New("request would have succeeded, but DryRun flag is set")
	ErrDuplicatePermission   = errors.New("InvalidPermission.Duplicate")

	// ErrDependencyViolation is returned when an operation cannot complete
	// because another resource still depends on the target resource.
	ErrDependencyViolation = errors.New("DependencyViolation")

	// ErrResourceAlreadyAssociated is returned when attaching a resource that
	// is already attached elsewhere (e.g. an Internet Gateway that is already
	// attached to a VPC, or a VPC that already has an Internet Gateway).
	ErrResourceAlreadyAssociated = errors.New("Resource.AlreadyAssociated")

	// ErrVpcClassicLinkDisabled is returned by AttachClassicLinkVpc when the
	// target VPC has not been enabled for ClassicLink.
	ErrVpcClassicLinkDisabled = errors.New("VpcClassicLinkDisabled")

	// ErrClassicLinkInstanceNotFound is returned when a ClassicLink
	// instance/VPC linkage cannot be found (e.g. DetachClassicLinkVpc for an
	// instance that is not currently linked).
	ErrClassicLinkInstanceNotFound = errors.New("InvalidInstanceID.NotFound")

	// ErrVpcBlockPublicAccessExclusionNotFound is returned when a VPC Block
	// Public Access exclusion ID cannot be found.
	ErrVpcBlockPublicAccessExclusionNotFound = errors.New("InvalidVpcBlockPublicAccessExclusionId.NotFound")

	// ErrInvalidUserData is returned when RunInstances / ModifyInstanceAttribute
	// user data is not valid base64 or exceeds the 16 KiB decoded-size limit.
	ErrInvalidUserData = errors.New("InvalidUserData.Malformed")
	// ErrMissingParameter is returned when a required parameter (e.g. the
	// ModifyInstanceAttribute attribute selector) is absent.
	ErrMissingParameter = errors.New("MissingParameter")
	// ErrInvalidPaginationToken is returned when a NextToken is forged, tampered
	// with, or otherwise fails HMAC verification.
	ErrInvalidPaginationToken = errors.New("InvalidPaginationToken")
	// ErrOperationNotPermitted is returned when an operation is blocked by an
	// instance-level API protection attribute (disableApiTermination /
	// disableApiStop), mirroring AWS's OperationNotPermitted error code.
	ErrOperationNotPermitted = errors.New("OperationNotPermitted")
)

// EC2 instance state codes as defined by the AWS EC2 API.
const (
	stateCodeRunning      = 16
	stateCodeTerminated   = 48
	stateCodeStopped      = 80
	stateCodePending      = 0
	stateCodeShuttingDown = 32
	stateCodeStopping     = 64

	// stateAvailable is the "available" state string shared by volumes, ENIs,
	// and other resources that are not currently in use.
	stateAvailable = "available"

	stateInUse                         = "in-use"
	stateCancelled                     = "cancelled"
	vpcEndpointTypeInterface           = "Interface"
	vpcEndpointTypeGatewayLoadBalancer = "GatewayLoadBalancer"
	resourceTypeVPC                    = "vpc"
	resourceTypeSnapshot               = "snapshot"
	resourceTypeENI                    = "network-interface"
	vpcDefaultName                     = "vpc-default"
	archX8664                          = "x86_64"
	resourceTypeFISInstance            = "aws:ec2:instance"
	ec2BooleanFalse                    = "false"

	// stateActive is the "active" state string used by peering connections,
	// capacity reservations, and spot instance requests.
	stateActive = "active"

	// lifecycleReconcileInterval is how often the reconciler advances transitional instance states.
	lifecycleReconcileInterval = 50 * time.Millisecond

	// cidrAllIPv4 is the IPv4 catch-all CIDR used in default security group egress rules.
	cidrAllIPv4 = "0.0.0.0/0"

	// defaultSecurityGroupName is the name AWS gives the auto-created default
	// security group of every VPC. It never blocks DeleteVpc (see
	// vpcDependencyViolationLocked) and is deleted automatically with the VPC.
	defaultSecurityGroupName = "default"
	// defaultSecurityGroupDescription is the fixed description AWS assigns to
	// every VPC's auto-created default security group.
	defaultSecurityGroupDescription = "default VPC security group"
)

// InstanceState represents the state of an EC2 instance.
type InstanceState struct {
	Name string `json:"name,omitempty"`
	Code int    `json:"code,omitempty"`
}

// Well-known instance states.
//
//nolint:gochecknoglobals // package-level sentinel values, analogous to exported errors
var (
	StateRunning      = InstanceState{Code: stateCodeRunning, Name: "running"}
	StateTerminated   = InstanceState{Code: stateCodeTerminated, Name: "terminated"}
	StateStopped      = InstanceState{Code: stateCodeStopped, Name: "stopped"}
	StatePending      = InstanceState{Code: stateCodePending, Name: "pending"}
	StateShuttingDown = InstanceState{Code: stateCodeShuttingDown, Name: "shutting-down"}
	StateStopping     = InstanceState{Code: stateCodeStopping, Name: "stopping"}
)

// Instance represents an EC2 instance (metadata only, no actual compute).
type Instance struct {
	TerminatedAt            time.Time                  `json:"terminatedAt"`
	LaunchTime              time.Time                  `json:"launchTime"`
	EventStartTimeOverrides map[string]time.Time       `json:"eventStartTimeOverrides,omitempty"`
	CapacityReservationSpec CapacityReservationSpec    `json:"capacityReservationSpecification"`
	MaintenanceOptions      InstanceMaintenanceOptions `json:"maintenanceOptions"`
	PublicDNSName           string                     `json:"publicDNSName,omitempty"`
	MetadataOptionsTokens   string                     `json:"metadataOptionsTokens,omitempty"`
	MetadataOptionsState    string                     `json:"metadataOptionsState,omitempty"`
	MonitoringState         string                     `json:"monitoringState,omitempty"`
	VPCID                   string                     `json:"vpcID,omitempty"`
	ID                      string                     `json:"id,omitempty"`
	PrivateIP               string                     `json:"privateIP,omitempty"`
	PublicIPAddress         string                     `json:"publicIPAddress,omitempty"`
	SubnetID                string                     `json:"subnetID,omitempty"`
	UserData                string                     `json:"userData,omitempty"`
	SriovNetSupport         string                     `json:"sriovNetSupport,omitempty"`
	ProviderID              string                     `json:"providerID,omitempty"`
	// InstanceInitiatedShutdownBehavior is "stop" (default) or "terminate".
	InstanceInitiatedShutdownBehavior string `json:"instanceInitiatedShutdownBehavior,omitempty"`
	// NetworkPerformanceOptions carries the bandwidth-weighting mode.
	NetworkPerformanceOptions InstanceNetworkPerformanceOptions `json:"networkPerformanceOptions"`
	KeyName                   string                            `json:"keyName,omitempty"`
	InstanceType              string                            `json:"instanceType,omitempty"`
	// StateTransitionReason mirrors AWS's legacy <reason> element (e.g.
	// "User initiated (2016-05-...)").
	StateTransitionReason string `json:"stateTransitionReason,omitempty"`
	ImageID               string `json:"imageID,omitempty"`
	// OutpostArn is the real SDK's types.Instance.OutpostArn -- a top-level
	// field, sibling to Placement (not nested under it; confirmed via
	// deserializers.go's awsEc2query_deserializeDocumentInstance, which reads
	// "outpostArn" and "placement" as separate XML elements). Set from the
	// launch subnet's Subnet.OutpostArn at RunInstances time.
	OutpostArn string `json:"outpostArn,omitempty"`
	// StateReasonCode/StateReasonMessage mirror AWS's <stateReason> element,
	// populated on user-initiated stop/terminate and cleared on start.
	StateReasonMessage    string                `json:"stateReasonMessage,omitempty"`
	StateReasonCode       string                `json:"stateReasonCode,omitempty"`
	Placement             InstancePlacement     `json:"placement"`
	SecurityGroups        []string              `json:"securityGroups,omitempty"`
	State                 InstanceState         `json:"state"`
	PrivateDNSNameOptions PrivateDNSNameOptions `json:"privateDnsNameOptions"`
	CPUOptions            CPUOptions            `json:"cpuOptions"`
	SSHPort               int                   `json:"sshPort,omitempty"`
	EnaSupport            bool                  `json:"enaSupport,omitempty"`
	// DisableAPITermination / DisableAPIStop gate TerminateInstances /
	// StopInstances respectively (ModifyInstanceAttribute-settable).
	DisableAPITermination bool `json:"disableApiTermination,omitempty"`
	DisableAPIStop        bool `json:"disableApiStop,omitempty"`
	EBSOptimized          bool `json:"ebsOptimized,omitempty"`
}

// LaunchTemplate represents an EC2 launch template.
type LaunchTemplate struct {
	CreateTime           time.Time `json:"createTime"`
	ID                   string    `json:"id,omitempty"`
	Name                 string    `json:"name,omitempty"`
	ImageID              string    `json:"imageID,omitempty"`
	InstanceType         string    `json:"instanceType,omitempty"`
	CreatedBy            string    `json:"createdBy,omitempty"`
	DefaultVersionNumber int64     `json:"defaultVersionNumber"`
	LatestVersionNumber  int64     `json:"latestVersionNumber"`
}

// VpcEndpoint represents an EC2 VPC endpoint.
type VpcEndpoint struct {
	CreateTime      time.Time `json:"createTime"`
	ID              string    `json:"id,omitempty"`
	VPCID           string    `json:"vpcID,omitempty"`
	ServiceName     string    `json:"serviceName,omitempty"`
	State           string    `json:"state,omitempty"`
	VpcEndpointType string    `json:"vpcEndpointType,omitempty"`
	OwnerID         string    `json:"ownerID,omitempty"`
	SubnetIDs       []string  `json:"subnetIDs,omitempty"`
	RouteTableIDs   []string  `json:"routeTableIDs,omitempty"`
	// PayerResponsibilities holds the payer-responsibility settings set via
	// ModifyVpcEndpointPayerResponsibility. Empty until first modified.
	PayerResponsibilities []PayerResponsibilityEntry `json:"payerResponsibilities,omitempty"`
}

// PayerResponsibilityEntry records who is billed for a VPC endpoint's usage,
// scoped to a particular charge category. Set via
// ModifyVpcEndpointPayerResponsibility.
type PayerResponsibilityEntry struct {
	PayerResponsibilityType string `json:"payerResponsibilityType,omitempty"`
	Scope                   string `json:"scope,omitempty"`
}

// NetworkACL represents an EC2 network ACL.
type NetworkACL struct {
	ID             string      `json:"id,omitempty"`
	VPCID          string      `json:"vpcID,omitempty"`
	AssociationIDs []string    `json:"associationIDs,omitempty"`
	Entries        []NACLEntry `json:"entries,omitempty"`
	IsDefault      bool        `json:"isDefault,omitempty"`
}

// InstanceStateChange records the state transition for a single instance.
// It is returned by StartInstances, StopInstances, and TerminateInstances so
// callers have accurate before/after information without hard-coding states.
type InstanceStateChange struct {
	InstanceID    string        `json:"instanceID,omitempty"`
	PreviousState InstanceState `json:"previousState"`
	CurrentState  InstanceState `json:"currentState"`
}

// SecurityGroupRule represents an inbound or outbound rule.
// Either IPRange or SourceGroupID is set; both can be empty for protocol-only rules.
//
// Description is metadata only: it does not participate in a rule's identity
// for authorize/revoke/duplicate-detection purposes (see ruleKey), matching
// AWS's UpdateSecurityGroupRuleDescriptions* semantics of changing a rule's
// description without it becoming a "different" rule.
type SecurityGroupRule struct {
	Protocol           string `json:"protocol,omitempty"`
	IPRange            string `json:"ipRange,omitempty"`
	SourceGroupID      string `json:"sourceGroupId,omitempty"`
	SourceGroupOwnerID string `json:"sourceGroupOwnerId,omitempty"`
	Description        string `json:"description,omitempty"`
	FromPort           int    `json:"fromPort,omitempty"`
	ToPort             int    `json:"toPort,omitempty"`
}

// SecurityGroup represents an EC2 security group.
type SecurityGroup struct {
	ID           string              `json:"id,omitempty"`
	Name         string              `json:"name,omitempty"`
	Description  string              `json:"description,omitempty"`
	VPCID        string              `json:"vpcID,omitempty"`
	ARN          string              `json:"arn,omitempty"`
	IngressRules []SecurityGroupRule `json:"ingressRules,omitempty"`
	EgressRules  []SecurityGroupRule `json:"egressRules,omitempty"`
}

// VPC represents an EC2 VPC.
type VPC struct {
	Attributes              map[string]bool `json:"attributes,omitempty"`
	ID                      string          `json:"id,omitempty"`
	CIDRBlock               string          `json:"cidrBlock,omitempty"`
	IsDefault               bool            `json:"isDefault,omitempty"`
	ClassicLinkEnabled      bool            `json:"classicLinkEnabled,omitempty"`
	ClassicLinkDNSSupported bool            `json:"classicLinkDnsSupported,omitempty"`
}

// Subnet represents an EC2 Subnet.
type Subnet struct {
	ID               string `json:"id,omitempty"`
	VPCID            string `json:"vpcID,omitempty"`
	CIDRBlock        string `json:"cidrBlock,omitempty"`
	AvailabilityZone string `json:"availabilityZone,omitempty"`
	// OutpostArn is the Outpost this subnet is hosted on, set via
	// CreateSubnetWithOutpost and cross-validated against the real
	// services/outposts backend (cross_service.go) when wired. Empty for a
	// normal (non-Outpost) subnet.
	OutpostArn          string `json:"outpostArn,omitempty"`
	IsDefault           bool   `json:"isDefault,omitempty"`
	MapPublicIPOnLaunch bool   `json:"mapPublicIpOnLaunch,omitempty"`
}

// InMemoryBackend is the in-memory store for EC2 resources.
type InMemoryBackend struct {
	compute      Compute
	dnsRegistrar DNSRegistrar
	// appConfig is the service.AppContext.Config value Provider.Init
	// received, recorded so RunInstances/TerminateInstances can resolve the
	// Outposts backend on demand -- see cross_service.go's SetAppConfig doc
	// comment for why this must be lazy rather than resolved at
	// construction time.
	appConfig                      any
	addressTransfers               map[string]*AddressTransfer
	capacityReservations           *store.Table[CapacityReservation]
	vpcs                           *store.Table[VPC]
	subnets                        *store.Table[Subnet]
	keyPairs                       *store.Table[KeyPair]
	reservedInstancesExchanges     *store.Table[ReservedInstancesExchange]
	addresses                      *store.Table[Address]
	internetGateways               *store.Table[InternetGateway]
	natGateways                    *store.Table[NatGateway]
	routeTables                    *store.Table[RouteTable]
	placementGroups                *store.Table[PlacementGroup]
	spotRequests                   *store.Table[SpotInstanceRequest]
	instances                      *store.Table[Instance]
	images                         *store.Table[AMIStub]
	launchTemplates                *store.Table[LaunchTemplate]
	vpcEndpoints                   *store.Table[VpcEndpoint]
	tags                           map[string]map[string]string
	securityGroups                 *store.Table[SecurityGroup]
	networkInterfaces              *store.Table[NetworkInterface]
	volumes                        *store.Table[Volume]
	tgwMulticastDomainAssociations *store.Table[TransitGatewayMulticastDomainAssociation]
	tgwPeeringAttachments          *store.Table[TransitGatewayPeeringAttachment]
	tgwVpcAttachments              *store.Table[TransitGatewayVpcAttachment]
	vpcEndpointConnections         *store.Table[VpcEndpointConnection]
	vpcPeeringConnections          *store.Table[VpcPeeringConnection]
	byoipCidrs                     *store.Table[ByoipCidr]
	dedicatedHosts                 *store.Table[Host]
	snapshots                      *store.Table[Snapshot]
	networkACLs                    *store.Table[StoredNetworkACL]
	transitGateways                *store.Table[TransitGateway]
	flowLogs                       *store.Table[FlowLog]
	dhcpOptionSets                 *store.Table[DhcpOptions]
	egressOnlyIGWs                 *store.Table[EgressOnlyInternetGateway]
	iamAssociations                *store.Table[IamInstanceProfileAssociation]
	tgwRouteTables                 *store.Table[TransitGatewayRouteTable]
	tgwRoutes                      *store.Table[TransitGatewayRoute]
	tgwRTAssociations              *store.Table[TransitGatewayRouteTableAssociation]
	tgwPolicyTables                *store.Table[TransitGatewayPolicyTable]
	tgwPolicyTableAssociations     *store.Table[TransitGatewayPolicyTableAssociation]
	tgwPolicyTableEntries          *store.Table[TransitGatewayPolicyTableEntry]
	tgwRouteTableAnnouncements     *store.Table[TransitGatewayRouteTableAnnouncement]
	vpcCidrAssociations            map[string]*VpcCidrBlockAssociation
	vpnGateways                    *store.Table[VpnGateway]
	customerGateways               *store.Table[CustomerGateway]
	vpnConnections                 *store.Table[VpnConnection]
	vpcEndpointServiceConfigs      *store.Table[VpcEndpointServiceConfig]
	ipams                          *store.Table[Ipam]
	ipamScopes                     *store.Table[IpamScope]
	ipamPools                      *store.Table[IpamPool]
	ipamPoolCidrs                  map[string][]*IpamPoolCidr
	ipamPoolAllocations            *store.Table[IpamPoolAllocation]
	ipamResourceDiscoveries        *store.Table[IpamResourceDiscovery]
	ipamResourceDiscoveryAssocs    *store.Table[IpamResourceDiscoveryAssociation]
	ipamByoasns                    *store.Table[IpamByoasn]
	ipamAsnAssociations            *store.Table[IpamAsnAssociation]
	ipamVerificationTokens         *store.Table[IpamExternalResourceVerificationToken]
	ipamResourceCidrs              *store.Table[IpamResourceCidr]
	ipamPrefixListResolvers        *store.Table[IpamPrefixListResolver]
	ipamPrefixListResolverVersions map[string][]int64
	ipamPrefixListResolverTargets  *store.Table[IpamPrefixListResolverTarget]
	ipamPolicies                   *store.Table[IpamPolicy]
	ipamPolicyEnabledTargets       map[string]string
	ipamOrgAdminAccountID          string
	spotFleets                     *store.Table[SpotFleetRequest]
	spotFleetHistory               map[string][]SpotFleetHistoryRecord
	// batch1 additions
	volumeModifications      *store.Table[VolumeModification]
	snapshotTiers            map[string]string
	snapshotAttributes       map[string]map[string]string
	sgVpcAssociations        map[string]map[string]string
	vpcTenancy               map[string]string
	vpcPeeringOptions        map[string]*PeeringConnectionOptions
	subnetCIDRAssociations   map[string][]*SubnetCIDRAssociation
	addressAttributes        *store.Table[AddressAttribute]
	instanceCreditSpecs      map[string]string
	instanceMetadataDefaults *InstanceMetadataDefaults
	instanceEventNotifAttrs  *InstanceEventNotificationAttributes
	niPermissions            *store.Table[NetworkInterfacePermission]
	niIPv6Addresses          map[string][]string
	idFormatSettings         map[string]bool
	// batch2 additions
	endpointConnectionNotifs      *store.Table[VpcEndpointConnectionNotification]
	vpcEndpointServicePermissions map[string][]string
	snapshotLocks                 *store.Table[SnapshotLock]
	replaceRootVolumeTasks        *store.Table[ReplaceRootVolumeTask]
	subnetCIDRReservations        map[string][]*SubnetCIDRReservation
	imageDisabled                 map[string]bool
	imageDeprecated               map[string]string
	imageDeregistrationProtection map[string]bool
	imageAttributes               map[string]map[string]string
	vgwRoutePropagation           map[string]bool
	// batch4 additions
	managedPrefixLists           *store.Table[ManagedPrefixList]
	clientVpnEndpoints           *store.Table[ClientVpnEndpoint]
	tgwConnects                  *store.Table[TransitGatewayConnect]
	tgwConnectPeers              *store.Table[TransitGatewayConnectPeer]
	tgwPrefixListRefs            *store.Table[TransitGatewayPrefixListReference]
	verifiedAccessEndpoints      *store.Table[VerifiedAccessEndpoint]
	verifiedAccessGroups         *store.Table[VerifiedAccessGroup]
	verifiedAccessInstances      *store.Table[VerifiedAccessInstance]
	verifiedAccessTrustProviders *store.Table[VerifiedAccessTrustProvider]
	// batch3 additions
	instanceConnectEndpoints *store.Table[InstanceConnectEndpoint]
	instanceEventWindows     *store.Table[InstanceEventWindow]
	imageImportTasks         *store.Table[ImageImportTask]
	snapshotImportTasks      *store.Table[SnapshotImportTask]
	recycleBinImages         *store.Table[RecycleBinImage]
	recycleBinSnapshots      *store.Table[Snapshot]
	recycleBinVolumes        *store.Table[RecycleBinVolume]
	fastLaunchImages         map[string]*FastLaunchImageItem
	fastSnapshotRestores     map[string]bool
	vpnConnectionRoutes      *store.Table[VpnConnectionRoute]
	spotDatafeed             *SpotDatafeed
	// batch5 additions
	trafficMirrorFilters               *store.Table[TrafficMirrorFilter]
	trafficMirrorFilterRules           *store.Table[TrafficMirrorFilterRule]
	trafficMirrorSessions              *store.Table[TrafficMirrorSession]
	trafficMirrorTargets               *store.Table[TrafficMirrorTarget]
	fleets                             *store.Table[Fleet]
	fleetHistory                       map[string][]FleetHistoryRecord
	networkInsightsPaths               *store.Table[NetworkInsightsPath]
	networkInsightsAnalyses            *store.Table[NetworkInsightsAnalysis]
	networkInsightsAccessScopes        *store.Table[NetworkInsightsAccessScope]
	networkInsightsAccessScopeAnalyses *store.Table[NetworkInsightsAccessScopeAnalysis]
	carrierGateways                    *store.Table[CarrierGateway]
	reservedInstances                  *store.Table[ReservedInstance]
	reservedInstancesOfferings         *store.Table[ReservedInstancesOffering]
	reservedInstancesListings          *store.Table[ReservedInstancesListing]
	reservedInstancesModifications     *store.Table[ReservedInstancesModification]
	// route server additions
	routeServers            *store.Table[RouteServer]
	routeServerEndpoints    *store.Table[RouteServerEndpoint]
	routeServerPeers        *store.Table[RouteServerPeer]
	routeServerAssociations *store.Table[RouteServerAssociation]
	routeServerPropagations *store.Table[RouteServerPropagation]
	// local gateway additions
	localGateways                              *store.Table[LocalGateway]
	localGatewayVirtualInterfaces              *store.Table[LocalGatewayVirtualInterface]
	localGatewayVirtualInterfaceGroups         *store.Table[LocalGatewayVirtualInterfaceGroup]
	localGatewayRouteTables                    *store.Table[LocalGatewayRouteTable]
	localGatewayRoutes                         *store.Table[LocalGatewayRoute]
	localGatewayRouteTableVpcAssociations      *store.Table[LocalGatewayRouteTableVpcAssociation]
	localGatewayRouteTableVifGroupAssociations *store.Table[LocalGatewayRouteTableVirtualInterfaceGroupAssociation]
	// transit gateway multicast domain / metering policy additions
	tgwMulticastDomains      *store.Table[TransitGatewayMulticastDomain]
	tgwMulticastGroupEntries *store.Table[TransitGatewayMulticastGroupEntry]
	tgwMeteringPolicies      *store.Table[TransitGatewayMeteringPolicy]
	tgwMeteringPolicyEntries *store.Table[TransitGatewayMeteringPolicyEntry]
	// VPC ClassicLink / Block Public Access additions
	classicLinkInstances           *store.Table[ClassicLinkInstance]
	vpcBlockPublicAccessOptions    *VpcBlockPublicAccessOptions
	vpcBlockPublicAccessExclusions *store.Table[VpcBlockPublicAccessExclusion]
	// Capacity Reservation Fleet / Capacity Block / Capacity Manager additions
	capacityReservationFleets             *store.Table[CapacityReservationFleet]
	capacityBlockOfferings                *store.Table[CapacityBlockOffering]
	capacityBlockExtensionOfferings       *store.Table[CapacityBlockExtensionOffering]
	capacityBlocks                        *store.Table[CapacityBlock]
	capacityBlockExtensions               *store.Table[CapacityBlockExtension]
	capacityReservationBillingRequests    *store.Table[CapacityReservationBillingRequest]
	capacityManagerDataExports            *store.Table[CapacityManagerDataExport]
	capacityManagerState                  *CapacityManagerState
	capacityReservationCancellationQuotes *store.Table[CapacityReservationCancellationQuote]
	// VerifiedAccess policy / logging additions
	verifiedAccessEndpointPolicies       map[string]*VerifiedAccessPolicy
	verifiedAccessGroupPolicies          map[string]*VerifiedAccessPolicy
	verifiedAccessInstanceLoggingConfigs *store.Table[VerifiedAccessInstanceLoggingConfig]
	// FPGA image additions
	fpgaImages *store.Table[FpgaImage]
	// Scheduled Instances additions
	scheduledInstances        *store.Table[ScheduledInstance]
	scheduledInstanceLaunched map[string]int32
	// COIP / Public IPv4 / IPv6 pool additions
	coipPools *store.Table[CoipPool]
	coipCidrs *store.Table[CoipCidr]
	ipv4Pools *store.Table[Ipv4Pool]
	ipv6Pools *store.Table[Ipv6Pool]
	// VM Import/Export, Bundle, and Conversion Task additions
	bundleTasks      *store.Table[BundleTask]
	conversionTasks  *store.Table[ConversionTask]
	exportTasks      *store.Table[ExportTask]
	exportImageTasks *store.Table[ExportImageTaskRec]
	// Trunk Interface / Enclave Certificate IAM Role additions
	trunkInterfaceAssociations *store.Table[TrunkInterfaceAssociation]
	enclaveCertIamRoles        map[string][]*EnclaveCertIamRoleAssociation
	// Allowed Images Settings / Store-Restore Image Task / Image Usage Report additions
	allowedImagesSettings *AllowedImagesSettings
	storeImageTasks       *store.Table[StoreImageTask]
	usageReports          *store.Table[UsageReport]
	usageReportEntries    map[string][]*UsageReportEntry
	instanceProductCodes  map[string][]string
	// Mac Host / Mac modification task additions
	macModificationTasks *store.Table[MacModificationTask]
	// Secondary Network / Secondary Subnet / Secondary Interface / Outpost LAG /
	// Service Link Virtual Interface additions
	secondaryNetworks            *store.Table[SecondaryNetwork]
	secondarySubnets             *store.Table[SecondarySubnet]
	secondaryInterfaces          *store.Table[SecondaryInterface]
	serviceLinkVirtualInterfaces *store.Table[ServiceLinkVirtualInterface]
	outpostLags                  *store.Table[OutpostLag]
	// Instance-attribute misc cluster additions (AZ group opt-in, SQL HA)
	availabilityZoneGroupOptIns map[string]string
	sqlHaRegistrations          *store.Table[RegisteredSQLHaInstance]
	sqlHaHistory                map[string][]*RegisteredSQLHaInstance
	// parity-sweep-2 additions: VPC Encryption Control, VPN Concentrator, Host
	// Reservations, Declarative Policies, AWS Network Performance
	vpcEncryptionControls           *store.Table[VpcEncryptionControl]
	vpnConcentrators                *store.Table[VpnConcentrator]
	hostReservations                *store.Table[HostReservation]
	declarativePoliciesReports      *store.Table[DeclarativePoliciesReport]
	networkPerformanceSubscriptions *store.Table[NetworkPerformanceSubscription]
	// gopherstack-5o9 final EC2 parity sweep additions
	tgwRTPropagations          map[string]map[string]*TransitGatewayRouteTablePropagation
	interruptibleCRAllocations *store.Table[InterruptibleCapacityReservationAllocation]
	movingAddresses            *store.Table[MovingAddressStatus]
	// parity-4 SDK-bump additions (gopherstack, eb437919a): TGW Client VPN
	// attachments, image watermarks, account VPC Encryption Control, Capacity
	// Manager monitored tag keys (nested in capacityManagerState), and
	// account-level managed resource visibility.
	tgwClientVpnAttachments            *store.Table[TransitGatewayClientVpnAttachment]
	imageWatermarks                    map[string][]string
	accountVpcEncryptionControl        *AccountVpcEncryptionControl
	applicationStatusChecks            *store.Table[ApplicationStatusCheck]
	applicationStatusCheckAssociations *store.Table[ApplicationStatusCheckAssociation]
	applicationStatusSuppressions      *store.Table[ApplicationStatusSuppression]
	managedResourceDefaultVisibility   string
	// registry lets Reset collapse the ~150 converted resource maps' lifecycle
	// to one call (registry.ResetAll()) instead of hand-rolled re-initialization
	// of each map. See store_setup.go for every Table registration.
	registry                       *store.Registry
	mu                             *lockmetrics.RWMutex
	lifecycleStop                  chan struct{}
	eniIDByAttachment              map[string]string
	eniIDsByInstance               map[string]map[string]struct{}
	instanceIDsByVPC               map[string]map[string]struct{}
	subnetIDsByVPC                 map[string]map[string]struct{}
	routeTableIDsByVPC             map[string]map[string]struct{}
	sgIDsByVPC                     map[string]map[string]struct{}
	natGatewayIDsByVPC             map[string]map[string]struct{}
	eniIDsByVPC                    map[string]map[string]struct{}
	snapshotBlockPublicAccess      string
	ebsDefaultKmsKeyID             string
	imageBlockPublicAccess         string
	defaultCreditSpec              string
	Region                         string `json:"region,omitempty"`
	AccountID                      string `json:"accountID,omitempty"`
	freePrivateIPs                 []string
	nextPrivateIPIndex             int
	nextElasticIPIndex             int
	ebsEncryptionByDefault         bool
	serialConsoleAccess            bool
	reachabilityAnalyzerOrgSharing bool
	lifecycleOnce                  sync.Once
	lifecycleStopOnce              sync.Once
}

func newInMemoryBackendMaps() *InMemoryBackend {
	b := &InMemoryBackend{
		registry:            store.NewRegistry(),
		tags:                make(map[string]map[string]string),
		addressTransfers:    make(map[string]*AddressTransfer),
		vpcCidrAssociations: make(map[string]*VpcCidrBlockAssociation),
		ipamPoolCidrs:       make(map[string][]*IpamPoolCidr),
		instanceIDsByVPC:    make(map[string]map[string]struct{}),
		eniIDsByInstance:    make(map[string]map[string]struct{}),
		eniIDByAttachment:   make(map[string]string),
	}
	registerAllTables(b)
	initCoreExtraMaps(b)
	initBatch6Maps(b)
	initVpcConfigMaps(b)
	initCapacityFamilyMaps(b)
	initVerifiedAccessExtMaps(b)
	initParityFinalMaps(b)
	initParity4Maps(b)
	initSecondaryIndexMaps(b)
	b.resetIpamDiscoveryMapsLocked()
	b.resetIpamPolicyMapsLocked()
	b.resetScheduledInstanceMapsLocked()
	b.resetIPPoolMapsLocked()
	b.resetAllowedImagesSettingsLocked()
	b.resetImageTasksLocked()
	b.resetUsageReportMapsLocked()
	b.resetVMImportExportMapsLocked()
	b.resetTrunkEnclaveMapsLocked()
	b.instanceProductCodes = make(map[string][]string)
	b.resetMacHostMapsLocked()
	b.resetSecondaryNetworkMapsLocked()
	b.resetInstanceAttrMapsLocked()
	b.resetSQLHaMapsLocked()

	return b
}

// initVerifiedAccessExtMaps initialises the VerifiedAccess policy/logging
// state maps (split out to keep newInMemoryBackendMaps under the funlen
// limit).
func initVerifiedAccessExtMaps(b *InMemoryBackend) {
	b.verifiedAccessEndpointPolicies = make(map[string]*VerifiedAccessPolicy)
	b.verifiedAccessGroupPolicies = make(map[string]*VerifiedAccessPolicy)
}

// initCoreExtraMaps initialises spot fleet, snapshot, IMDS, and batch4 state
// maps (split out to keep newInMemoryBackendMaps under the funlen limit).
func initCoreExtraMaps(b *InMemoryBackend) {
	b.spotFleetHistory = make(map[string][]SpotFleetHistoryRecord)
	b.fleetHistory = make(map[string][]FleetHistoryRecord)
	b.snapshotTiers = make(map[string]string)
	b.snapshotAttributes = make(map[string]map[string]string)
	b.sgVpcAssociations = make(map[string]map[string]string)
	b.vpcTenancy = make(map[string]string)
	b.vpcPeeringOptions = make(map[string]*PeeringConnectionOptions)
	b.subnetCIDRAssociations = make(map[string][]*SubnetCIDRAssociation)
	b.instanceCreditSpecs = make(map[string]string)
	b.niIPv6Addresses = make(map[string][]string)
	b.idFormatSettings = make(map[string]bool)
	b.vpcEndpointServicePermissions = make(map[string][]string)
	b.subnetCIDRReservations = make(map[string][]*SubnetCIDRReservation)
	b.imageDisabled = make(map[string]bool)
	b.imageDeprecated = make(map[string]string)
	b.imageDeregistrationProtection = make(map[string]bool)
	b.imageAttributes = make(map[string]map[string]string)
	b.vgwRoutePropagation = make(map[string]bool)
}

// initCapacityFamilyMaps initialises the Capacity Reservation Fleet, Capacity
// Block, and Capacity Manager state maps (split out to keep
// newInMemoryBackendMaps under the funlen limit).
func initCapacityFamilyMaps(b *InMemoryBackend) {
	b.capacityManagerState = &CapacityManagerState{
		Status:           capacityManagerStatusDisabled,
		MonitoredTagKeys: make(map[string]*CapacityManagerMonitoredTagKey),
	}
}

// initVpcConfigMaps initialises the VPC ClassicLink and Block Public Access
// state maps (split out to keep newInMemoryBackendMaps under the funlen limit).
func initVpcConfigMaps(b *InMemoryBackend) {
	b.vpcBlockPublicAccessOptions = &VpcBlockPublicAccessOptions{
		InternetGatewayBlockMode: vpcBPABlockModeOff,
		State:                    vpcBPAStateDefault,
		ExclusionsAllowed:        vpcBPAExclusionsAllowed,
		ManagedBy:                vpcBPAManagedByAccount,
	}
}

// initBatch6Maps initialises the verified-access, import-task, recycle-bin,
// fast-launch and VPN-route maps (split out to keep newInMemoryBackendMaps
// under the funlen limit).
func initBatch6Maps(b *InMemoryBackend) {
	b.fastLaunchImages = make(map[string]*FastLaunchImageItem)
	b.fastSnapshotRestores = make(map[string]bool)
}

// initParity4Maps initialises the state added for the parity-4 SDK-bump pass
// (16 new operations discovered after bumping aws-sdk-go-v2/service/ec2):
// image watermarks and the account-level VPC Encryption Control / managed
// resource visibility singletons. Split out to keep newInMemoryBackendMaps
// under the funlen limit, matching initVpcConfigMaps/initCapacityFamilyMaps.
func initParity4Maps(b *InMemoryBackend) {
	b.imageWatermarks = make(map[string][]string)
	b.accountVpcEncryptionControl = &AccountVpcEncryptionControl{
		Mode:      accountVpcEncryptionControlModeUnmanaged,
		State:     accountVpcEncryptionControlStateDefault,
		ManagedBy: accountVpcEncryptionControlManagedByAccount,
	}
	b.managedResourceDefaultVisibility = managedResourceVisibilityHidden
}

// NewInMemoryBackend creates a new InMemoryBackend with a default VPC and subnet.
func NewInMemoryBackend(accountID, region string) *InMemoryBackend {
	b := newInMemoryBackendMaps()
	b.AccountID = accountID
	b.Region = region
	b.mu = lockmetrics.New("ec2")
	b.lifecycleStop = make(chan struct{})
	b.initDefaults()

	return b
}

// ---- Reset ----

// Reset clears all resource state in the backend, returning it to its initial state.
// All resource maps for original and new operations are re-created, and defaults
// (default VPC, subnet, security group) are re-populated.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	// Reset every store.Table-backed resource map in one call instead of the
	// per-map make() calls this used to be (Phase 3.3 pkgs/store conversion).
	// See registerAllTables in store_setup.go for the full list of tables this
	// covers, and the comment there for the handful of fields that are NOT
	// covered because they remain plain maps.
	b.registry.ResetAll()

	// Reset original resource maps.
	b.tags = make(map[string]map[string]string)
	b.addressTransfers = make(map[string]*AddressTransfer)
	b.vpcCidrAssociations = make(map[string]*VpcCidrBlockAssociation)
	initSecondaryIndexMaps(b)
	b.freePrivateIPs = nil
	b.nextPrivateIPIndex = 0
	b.nextElasticIPIndex = 0

	// Reset account-level default settings set via Enable/Disable/Modify*
	// operations. These aren't store.Table-backed and, unlike the maps below,
	// were never re-initialised by any init*Maps helper -- only their Go zero
	// value at construction made them look reset.
	b.instanceMetadataDefaults = nil
	b.instanceEventNotifAttrs = nil
	b.spotDatafeed = nil
	b.snapshotBlockPublicAccess = ""
	b.ebsDefaultKmsKeyID = ""
	b.imageBlockPublicAccess = ""
	b.defaultCreditSpec = ""
	b.ebsEncryptionByDefault = false
	b.serialConsoleAccess = false

	b.resetNewOpsMapsLocked()

	// Re-populate defaults (must be called without the lock held since it acquires its own).
	// Since we already hold the lock, populate inline.
	b.vpcs.Put(&VPC{
		ID:        vpcDefaultName,
		CIDRBlock: "172.31.0.0/16",
		IsDefault: true,
	})
	b.subnets.Put(&Subnet{
		ID:               "subnet-default",
		VPCID:            vpcDefaultName,
		CIDRBlock:        "172.31.0.0/20",
		AvailabilityZone: b.Region + "a",
		IsDefault:        true,
	})
	b.indexSubnetLocked("subnet-default", vpcDefaultName)
	b.securityGroups.Put(&SecurityGroup{
		ID:          "sg-default",
		Name:        defaultSecurityGroupName,
		Description: defaultSecurityGroupDescription,
		VPCID:       vpcDefaultName,
	})
	b.indexSGLocked("sg-default", vpcDefaultName)
}

// resetNewOpsMapsLocked re-initialises all "new operations" resource maps introduced
// after the original core set. Must be called with b.mu held.
func (b *InMemoryBackend) resetNewOpsMapsLocked() {
	b.addressTransfers = make(map[string]*AddressTransfer)
	b.vpcCidrAssociations = make(map[string]*VpcCidrBlockAssociation)
	initCoreExtraMaps(b)
	initBatch6Maps(b)
	b.resetAdvancedNetworkingMapsLocked()
	b.resetIpamDiscoveryMapsLocked()
	b.resetIpamPolicyMapsLocked()
	b.resetBatch4MapsLocked()
	initVpcConfigMaps(b)
	initCapacityFamilyMaps(b)
	initVerifiedAccessExtMaps(b)
	b.resetScheduledInstanceMapsLocked()
	b.resetIPPoolMapsLocked()
	b.resetAllowedImagesSettingsLocked()
	b.resetImageTasksLocked()
	b.resetUsageReportMapsLocked()
	b.resetVMImportExportMapsLocked()
	b.resetTrunkEnclaveMapsLocked()
	b.instanceProductCodes = make(map[string][]string)
	b.resetMacHostMapsLocked()
	b.resetSecondaryNetworkMapsLocked()
	b.resetInstanceAttrMapsLocked()
	b.resetSQLHaMapsLocked()
	initParityFinalMaps(b)
	initParity4Maps(b)
}

// resetBatch4MapsLocked re-initialises all batch4 resource maps.
// Must be called with b.mu held.
func (b *InMemoryBackend) resetBatch4MapsLocked() {
}

// StartLifecycleReconciler starts the background goroutine that advances
// instances through their transitional states (pending→running, stopping→stopped,
// shutting-down→terminated), until ctx is cancelled or StopLifecycleReconciler is
// called. Idempotent. Started by the production provider only; tests drive
// transitions via TickLifecycleForTest and must not start the ticker, or it
// races with their direct ticks and state assertions.
func (b *InMemoryBackend) StartLifecycleReconciler(ctx context.Context) {
	b.lifecycleOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(lifecycleReconcileInterval)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-b.lifecycleStop:
					return
				case <-ticker.C:
					b.reconcileInstanceLifecycle()
				}
			}
		}()
	})
}

// StopLifecycleReconciler signals the background lifecycle goroutine (if any) to
// exit. Idempotent and safe to call even if the reconciler was never started.
func (b *InMemoryBackend) StopLifecycleReconciler() {
	b.lifecycleStopOnce.Do(func() {
		close(b.lifecycleStop)
	})
}

// reconcileInstanceLifecycle advances all instances in transitional states to their
// next stable state. It is also called directly by tests via TickLifecycleForTest.
// Performance: takes a cheap read-lock pass first to bail early when nothing is
// transitional, avoiding a write-lock acquisition on every 50ms tick.
func (b *InMemoryBackend) reconcileInstanceLifecycle() {
	// Fast path: read-lock to detect any transitional instance.
	b.mu.RLock("reconcileInstanceLifecycle-check")
	hasTransitional := false
	for _, inst := range b.instances.All() {
		switch inst.State {
		case StatePending, StateStopping, StateShuttingDown:
			hasTransitional = true
		}
		if hasTransitional {
			break
		}
	}
	b.mu.RUnlock()

	if !hasTransitional {
		return
	}

	// Slow path: write-lock to advance transitional instances.
	b.mu.Lock("reconcileInstanceLifecycle")
	defer b.mu.Unlock()

	for _, inst := range b.instances.All() {
		switch inst.State {
		case StatePending:
			inst.State = StateRunning
		case StateStopping:
			inst.State = StateStopped
		case StateShuttingDown:
			inst.State = StateTerminated
		}
	}
}

// initDefaults pre-populates a default VPC, subnet, and security group.
func (b *InMemoryBackend) initDefaults() {
	defaultVPCID := vpcDefaultName
	b.vpcs.Put(&VPC{
		ID:        defaultVPCID,
		CIDRBlock: "172.31.0.0/16",
		IsDefault: true,
	})

	defaultSubnetID := "subnet-default"
	b.subnets.Put(&Subnet{
		ID:               defaultSubnetID,
		VPCID:            defaultVPCID,
		CIDRBlock:        "172.31.0.0/20",
		AvailabilityZone: b.Region + "a",
		IsDefault:        true,
	})
	b.indexSubnetLocked(defaultSubnetID, defaultVPCID)

	defaultSGID := "sg-default"
	b.securityGroups.Put(&SecurityGroup{
		ID:          defaultSGID,
		Name:        defaultSecurityGroupName,
		Description: defaultSecurityGroupDescription,
		VPCID:       defaultVPCID,
	})
	b.indexSGLocked(defaultSGID, defaultVPCID)
}

// resolveRunInstancesCount defaults count < 1 to 1 and rejects a count above
// maxInstancesPerRunInstancesRequest, matching handler_filters.go's
// parseRunInstancesCounts so direct backend callers (cloudformation, tests)
// get the same error HTTP callers get, instead of a silently shortened batch.
func resolveRunInstancesCount(count int) (int, error) {
	if count < 1 {
		return 1, nil
	}

	if count > maxInstancesPerRunInstancesRequest {
		return 0, fmt.Errorf(
			"%w: cannot launch %d instances in a single request; the limit is %d",
			ErrResourceCountExceeded, count, maxInstancesPerRunInstancesRequest,
		)
	}

	return count, nil
}

// newOutpostReservedInstanceIDs pre-mints count instance IDs for an
// Outpost capacity reservation. No capacity hint, matching the
// non-outpost `instances` make in RunInstances below: a fixed
// maxInstancesPerRunInstancesRequest reservation overshoots for small
// counts, and a count-derived hint (even clamped) trips CodeQL
// go/uncontrolled-allocation-size (alert #253; gopherstack-17sl found a
// guard-then-use of count isn't recognized here either). count is only
// used for the loop count, never the make() size (safe).
//
//nolint:prealloc,nolintlint // satisfies CodeQL by removing tainted capacity hint
func newOutpostReservedInstanceIDs(count int) []string {
	ids := make([]string, 0)
	for range count {
		ids = append(ids, newInstanceID())
	}

	return ids
}

// RunInstances creates one or more EC2 instance stubs.
func (b *InMemoryBackend) RunInstances(
	imageID, instanceType, subnetID string,
	count int,
) ([]*Instance, error) {
	if imageID == "" {
		return nil, fmt.Errorf("%w: ImageId is required", ErrInvalidParameter)
	}

	count, err := resolveRunInstancesCount(count)
	if err != nil {
		return nil, err
	}

	b.mu.Lock("RunInstances")
	defer b.mu.Unlock()

	if subnetID == "" {
		subnetID = b.findDefaultSubnetID()
	} else if _, ok := b.subnets.Get(subnetID); !ok {
		return nil, fmt.Errorf("%w: %s", ErrSubnetNotFound, subnetID)
	}

	vpcID := ""
	mapPublicIP := false
	availabilityZone := ""
	outpostArn := ""

	if sub, ok := b.subnets.Get(subnetID); ok {
		vpcID = sub.VPCID
		mapPublicIP = sub.MapPublicIPOnLaunch
		availabilityZone = sub.AvailabilityZone
		outpostArn = sub.OutpostArn
	}

	// Real AWS depletes an Outpost's configured instance-type capacity as
	// instances launch onto it (see services/outposts/capacity_ledger.go's
	// ConsumeCapacity doc comment). Reserve capacity for the WHOLE batch
	// before creating any instance, using pre-minted IDs, so a rejected
	// batch (Outpost gone, insufficient capacity) never creates an
	// ec2.Instance at all -- matches real RunInstances failing atomically.
	var instanceIDs []string
	if outpostArn != "" {
		instanceIDs = newOutpostReservedInstanceIDs(count)

		if outpostsBk, ok := b.outpostsBackend(); ok {
			if capErr := outpostsBk.ConsumeCapacity(outpostArn, instanceType, b.AccountID, instanceIDs); capErr != nil {
				return nil, translateOutpostsCapacityErr(capErr)
			}
		}
	}

	// No capacity hint — user-derived values in the make capacity position
	// trigger CodeQL go/slice-memory-allocation-excessive-size even after
	// clamping. count is only used for the loop count below (safe).
	//nolint:prealloc,nolintlint // satisfies CodeQL by removing tainted capacity hint
	instances := make([]*Instance, 0)

	for i := range count {
		id := newInstanceID()
		if len(instanceIDs) > 0 {
			id = instanceIDs[i]
		}

		inst := &Instance{
			ID:           id,
			ImageID:      imageID,
			InstanceType: instanceType,
			// AWS state machine: pending → running via reconciler goroutine.
			State:      StatePending,
			VPCID:      vpcID,
			SubnetID:   subnetID,
			OutpostArn: outpostArn,
			LaunchTime: time.Now(),
			EnaSupport: true,
		}
		inst.Placement.AvailabilityZone = availabilityZone
		inst.PrivateIP = b.allocPrivateIP()
		if mapPublicIP {
			inst.PublicIPAddress = b.allocElasticIP()
			inst.PublicDNSName = fmt.Sprintf("ec2-%s.compute-1.amazonaws.com",
				strings.ReplaceAll(inst.PublicIPAddress, ".", "-"))
		}
		eniID := newENIID()
		attachID := "eni-attach-" + uuid.New().String()[:8]
		b.networkInterfaces.Put(&NetworkInterface{
			ID:                  eniID,
			SubnetID:            subnetID,
			VPCID:               vpcID,
			PrivateIP:           inst.PrivateIP,
			InstanceID:          id,
			AttachmentID:        attachID,
			DeviceIndex:         0,
			Status:              stateInUse,
			OwnerID:             b.AccountID,
			SourceDestCheck:     true,
			DeleteOnTermination: true,
		})
		b.instances.Put(inst)
		b.indexInstanceLocked(inst)
		eni, _ := b.networkInterfaces.Get(eniID)
		b.indexENILocked(eniID, eni)
		b.indexENIByVPCLocked(eniID, eni)
		instances = append(instances, inst)
	}

	return instances, nil
}

// findDefaultSubnetID returns the ID of the default subnet, or empty string if none.
// Must be called with b.mu held.
func (b *InMemoryBackend) findDefaultSubnetID() string {
	for _, s := range b.subnets.All() {
		id := subnetsKeyFn(s)
		if s.IsDefault {
			return id
		}
	}

	return ""
}

// cidrsOverlap reports whether two CIDR blocks overlap.
// Malformed CIDRs are treated as non-overlapping to avoid panics on bad input.
func cidrsOverlap(a, b string) bool {
	_, netA, err1 := net.ParseCIDR(a)
	_, netB, err2 := net.ParseCIDR(b)

	if err1 != nil || err2 != nil {
		return false
	}

	return netA.Contains(netB.IP) || netB.Contains(netA.IP)
}

// cidrContains reports whether outer fully contains inner.
func cidrContains(outer, inner string) bool {
	_, outerNet, err1 := net.ParseCIDR(outer)
	_, innerNet, err2 := net.ParseCIDR(inner)

	if err1 != nil || err2 != nil {
		return false
	}

	// outer must contain inner's base address and inner's broadcast address
	ones1, _ := outerNet.Mask.Size()
	ones2, _ := innerNet.Mask.Size()

	return outerNet.Contains(innerNet.IP) && ones1 <= ones2
}

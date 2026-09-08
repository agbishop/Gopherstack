package vpclattice

import (
	"context"
	"time"
)

// StorageBackend is the interface for VPC Lattice storage operations.
type StorageBackend interface {
	CreateService(
		ctx context.Context,
		name, authType, certificateArn, customDomainName string,
		tags map[string]string,
	) (*Service, error)
	GetService(serviceID string) (*Service, error)
	UpdateService(serviceID, authType, certificateArn string) (*Service, error)
	DeleteService(serviceID string) (*Service, error)
	ListServices(ctx context.Context, maxResults int32, nextToken string) ([]*ServiceSummary, string, error)

	CreateServiceNetwork(ctx context.Context, name, authType string, tags map[string]string) (*ServiceNetwork, error)
	GetServiceNetwork(snID string) (*ServiceNetwork, error)
	UpdateServiceNetwork(snID, authType string) (*ServiceNetwork, error)
	DeleteServiceNetwork(snID string) error
	ListServiceNetworks(
		ctx context.Context,
		maxResults int32,
		nextToken string,
	) ([]*ServiceNetworkSummary, string, error)

	CreateServiceNetworkServiceAssociation(
		ctx context.Context,
		serviceNetworkID, serviceID string,
		tags map[string]string,
	) (*ServiceNetworkServiceAssociation, error)
	GetServiceNetworkServiceAssociation(snsaID string) (*ServiceNetworkServiceAssociation, error)
	DeleteServiceNetworkServiceAssociation(snsaID string) error
	ListServiceNetworkServiceAssociations(
		ctx context.Context,
		serviceNetworkID, serviceID string,
		maxResults int32,
		nextToken string,
	) ([]*ServiceNetworkServiceAssociationSummary, string, error)

	CreateServiceNetworkVpcAssociation(
		ctx context.Context,
		serviceNetworkID, vpcID string,
		securityGroupIDs []string,
		privateDNSEnabled bool,
		dnsOptions *DNSOptions,
		tags map[string]string,
	) (*ServiceNetworkVpcAssociation, error)
	GetServiceNetworkVpcAssociation(snvaID string) (*ServiceNetworkVpcAssociation, error)
	UpdateServiceNetworkVpcAssociation(
		snvaID string,
		securityGroupIDs []string,
	) (*ServiceNetworkVpcAssociation, error)
	DeleteServiceNetworkVpcAssociation(snvaID string) error
	ListServiceNetworkVpcAssociations(
		ctx context.Context,
		serviceNetworkID, vpcID string,
		maxResults int32,
		nextToken string,
	) ([]*ServiceNetworkVpcAssociationSummary, string, error)

	CreateListener(
		serviceID, name, protocol string,
		port int32,
		defaultAction *RuleAction,
		tags map[string]string,
	) (*Listener, error)
	GetListener(serviceID, listenerID string) (*Listener, error)
	UpdateListener(serviceID, listenerID string, defaultAction *RuleAction) (*Listener, error)
	DeleteListener(serviceID, listenerID string) error
	ListListeners(
		serviceID string,
		maxResults int32,
		nextToken string,
	) ([]*ListenerSummary, string, error)

	CreateRule(
		serviceID, listenerID, name string,
		priority int32,
		action *RuleAction,
		match *RuleMatch,
		tags map[string]string,
	) (*Rule, error)
	GetRule(serviceID, listenerID, ruleID string) (*Rule, error)
	UpdateRule(
		serviceID, listenerID, ruleID string,
		priority int32,
		action *RuleAction,
		match *RuleMatch,
	) (*Rule, error)
	DeleteRule(serviceID, listenerID, ruleID string) error
	ListRules(
		serviceID, listenerID string,
		maxResults int32,
		nextToken string,
	) ([]*RuleSummary, string, error)
	BatchUpdateRule(
		serviceID, listenerID string,
		updates []*RuleUpdate,
	) ([]*RuleUpdateSuccess, []*RuleUpdateFailure, error)

	CreateTargetGroup(
		ctx context.Context,
		name, tgType string,
		config *TargetGroupConfig,
		tags map[string]string,
	) (*TargetGroup, error)
	GetTargetGroup(tgID string) (*TargetGroup, error)
	UpdateTargetGroup(tgID string, healthCheck *HealthCheckConfig) (*TargetGroup, error)
	DeleteTargetGroup(tgID string) error
	ListTargetGroups(
		ctx context.Context,
		tgType, vpcID string,
		maxResults int32,
		nextToken string,
	) ([]*TargetGroupSummary, string, error)
	RegisterTargets(tgID string, targets []*Target) ([]*TargetFailure, error)
	DeregisterTargets(tgID string, targets []*Target) ([]*TargetFailure, error)
	ListTargets(
		ctx context.Context,
		tgID string,
		filters []Target,
		maxResults int32,
		nextToken string,
	) ([]*TargetSummary, string, error)

	CreateAccessLogSubscription(
		ctx context.Context,
		resourceID, destinationArn, logType string,
		tags map[string]string,
	) (*AccessLogSubscription, error)
	GetAccessLogSubscription(alsID string) (*AccessLogSubscription, error)
	UpdateAccessLogSubscription(alsID, destinationArn string) (*AccessLogSubscription, error)
	DeleteAccessLogSubscription(alsID string) error
	ListAccessLogSubscriptions(
		ctx context.Context,
		resourceID string,
		maxResults int32,
		nextToken string,
	) ([]*AccessLogSubscriptionSummary, string, error)

	CreateResourceGateway(
		ctx context.Context,
		name, vpcID, ipAddressType, resourceConfigDNSResolution string,
		ipv4AddressesPerENI int32,
		securityGroupIDs, subnetIDs []string,
		tags map[string]string,
	) (*ResourceGateway, error)
	GetResourceGateway(id string) (*ResourceGateway, error)
	UpdateResourceGateway(id string, securityGroupIDs []string) (*ResourceGateway, error)
	DeleteResourceGateway(id string) (*ResourceGateway, error)
	ListResourceGateways(
		ctx context.Context,
		maxResults int32,
		nextToken string,
	) ([]*ResourceGatewaySummary, string, error)

	CreateResourceConfiguration(
		ctx context.Context,
		name, resourceType, protocol, resourceGatewayIdentifier, resourceConfigurationGroupIdentifier string,
		allowAssociationToShareableServiceNetwork bool,
		portRanges []string,
		definition *ResourceConfigurationDefinition,
		customDomainName, domainVerificationID, groupDomain string,
		tags map[string]string,
	) (*ResourceConfiguration, error)
	GetResourceConfiguration(id string) (*ResourceConfiguration, error)
	UpdateResourceConfiguration(
		id string,
		allowAssociationToShareableServiceNetwork *bool,
		portRanges []string,
		definition *ResourceConfigurationDefinition,
	) (*ResourceConfiguration, error)
	DeleteResourceConfiguration(id string) error
	ListResourceConfigurations(
		ctx context.Context,
		resourceGatewayIdentifier, resourceConfigurationGroupIdentifier string,
		maxResults int32,
		nextToken string,
	) ([]*ResourceConfigurationSummary, string, error)

	CreateServiceNetworkResourceAssociation(
		ctx context.Context,
		serviceNetworkIdentifier, resourceConfigurationIdentifier string,
		privateDNSEnabled bool,
		tags map[string]string,
	) (*ServiceNetworkResourceAssociation, error)
	GetServiceNetworkResourceAssociation(id string) (*ServiceNetworkResourceAssociation, error)
	DeleteServiceNetworkResourceAssociation(id string) (*ServiceNetworkResourceAssociation, error)
	ListServiceNetworkResourceAssociations(
		ctx context.Context,
		serviceNetworkIdentifier, resourceConfigurationIdentifier string,
		maxResults int32,
		nextToken string,
	) ([]*ServiceNetworkResourceAssociationSummary, string, error)

	StartDomainVerification(
		ctx context.Context,
		domainName string,
		tags map[string]string,
	) (*DomainVerification, error)
	GetDomainVerification(id string) (*DomainVerification, error)
	DeleteDomainVerification(id string) error
	ListDomainVerifications(
		ctx context.Context,
		maxResults int32,
		nextToken string,
	) ([]*DomainVerificationSummary, string, error)

	ListResourceEndpointAssociations(
		ctx context.Context,
		maxResults int32,
		nextToken string,
	) ([]*ResourceEndpointAssociationSummary, string, error)
	DeleteResourceEndpointAssociation(id string) error

	ListServiceNetworkVpcEndpointAssociations(
		ctx context.Context,
		serviceNetworkIdentifier string,
		maxResults int32,
		nextToken string,
	) ([]*ServiceNetworkVpcEndpointAssociationSummary, string, error)

	PutAuthPolicy(resourceID, policy string) (*AuthPolicy, error)
	GetAuthPolicy(resourceID string) (*AuthPolicy, error)
	DeleteAuthPolicy(resourceID string) error

	PutResourcePolicy(resourceArn, policy string) error
	GetResourcePolicy(resourceArn string) (string, error)
	DeleteResourcePolicy(resourceArn string) error

	TagResource(resourceArn string, tags map[string]string) error
	UntagResource(resourceArn string, keys []string) error
	ListTagsForResource(resourceArn string) (map[string]string, error)

	AccountID() string
	Region() string
	Reset()
	Snapshot(ctx context.Context) []byte
	Restore(ctx context.Context, data []byte) error
}

// Service represents a VPC Lattice service.
type Service struct {
	CreatedAt        time.Time
	LastUpdatedAt    time.Time
	ARN              string
	ID               string
	Name             string
	AuthType         string
	CertificateArn   string
	CustomDomainName string
	DNSName          string
	HostedZoneID     string
	Status           string
}

// ServiceSummary is a service entry for list responses.
type ServiceSummary struct {
	CreatedAt        time.Time
	LastUpdatedAt    time.Time
	ARN              string
	ID               string
	Name             string
	CustomDomainName string
	DNSName          string
	HostedZoneID     string
	Status           string
}

// ServiceNetwork represents a VPC Lattice service network.
type ServiceNetwork struct {
	CreatedAt                  time.Time
	LastUpdatedAt              time.Time
	ARN                        string
	ID                         string
	Name                       string
	AuthType                   string
	NumberOfAssociatedServices int64
	NumberOfAssociatedVPCs     int64
}

// ServiceNetworkSummary is a service network entry for list responses.
type ServiceNetworkSummary struct {
	CreatedAt                                time.Time
	ARN                                      string
	ID                                       string
	Name                                     string
	NumberOfAssociatedServices               int64
	NumberOfAssociatedVPCs                   int64
	NumberOfAssociatedResourceConfigurations int64
}

// ServiceNetworkServiceAssociation is a service-to-service-network association.
type ServiceNetworkServiceAssociation struct {
	CreatedAt          time.Time
	ARN                string
	ID                 string
	ServiceARN         string
	ServiceID          string
	ServiceName        string
	ServiceNetworkARN  string
	ServiceNetworkID   string
	ServiceNetworkName string
	Status             string
	CreatedBy          string
	CustomDomainName   string
	DNSName            string
	HostedZoneID       string
}

// ServiceNetworkServiceAssociationSummary is a summary for list responses.
type ServiceNetworkServiceAssociationSummary struct {
	CreatedAt          time.Time
	ARN                string
	ID                 string
	ServiceARN         string
	ServiceID          string
	ServiceName        string
	ServiceNetworkARN  string
	ServiceNetworkID   string
	ServiceNetworkName string
	Status             string
	CustomDomainName   string
	DNSName            string
	HostedZoneID       string
}

// ServiceNetworkVpcAssociation is a VPC-to-service-network association.
type ServiceNetworkVpcAssociation struct {
	DNSOptions         *DNSOptions
	CreatedAt          time.Time
	LastUpdatedAt      time.Time
	ARN                string
	ID                 string
	VpcID              string
	ServiceNetworkARN  string
	ServiceNetworkID   string
	ServiceNetworkName string
	Status             string
	CreatedBy          string
	SecurityGroupIDs   []string
	PrivateDNSEnabled  bool
}

// DNSOptions carries the DNS configuration options accepted by
// CreateServiceNetworkVpcAssociation (the real SDK's types.DnsOptions) --
// set only at creation time, real AWS has no update path for it.
type DNSOptions struct {
	PrivateDNSPreference       string
	PrivateDNSSpecifiedDomains []string
}

// ServiceNetworkVpcAssociationSummary is a summary for list responses.
type ServiceNetworkVpcAssociationSummary struct {
	DNSOptions         *DNSOptions
	CreatedAt          time.Time
	ARN                string
	ID                 string
	VpcID              string
	ServiceNetworkARN  string
	ServiceNetworkID   string
	ServiceNetworkName string
	Status             string
	PrivateDNSEnabled  bool
}

// Listener represents a VPC Lattice listener.
type Listener struct {
	DefaultAction *RuleAction
	CreatedAt     time.Time
	LastUpdatedAt time.Time
	ARN           string
	ID            string
	ServiceARN    string
	ServiceID     string
	Name          string
	Protocol      string
	Port          int32
}

// ListenerSummary is a listener entry for list responses.
type ListenerSummary struct {
	CreatedAt     time.Time
	LastUpdatedAt time.Time
	ARN           string
	ID            string
	Name          string
	Protocol      string
	Port          int32
}

// Rule represents a VPC Lattice listener rule.
type Rule struct {
	Action        *RuleAction
	Match         *RuleMatch
	CreatedAt     time.Time
	LastUpdatedAt time.Time
	ARN           string
	ID            string
	Name          string
	Priority      int32
	IsDefault     bool
}

// RuleSummary is a rule entry for list responses.
type RuleSummary struct {
	CreatedAt     time.Time
	LastUpdatedAt time.Time
	ARN           string
	ID            string
	Name          string
	Priority      int32
	IsDefault     bool
}

// RuleUpdate is an update spec for BatchUpdateRule.
type RuleUpdate struct {
	Action         *RuleAction
	Match          *RuleMatch
	RuleIdentifier string
	Priority       int32
}

// RuleUpdateSuccess is a successful rule update result.
type RuleUpdateSuccess struct {
	Action    *RuleAction
	Match     *RuleMatch
	ARN       string
	ID        string
	Name      string
	Priority  int32
	IsDefault bool
}

// RuleUpdateFailure is a failed rule update result.
type RuleUpdateFailure struct {
	RuleIdentifier string
	Message        string
	Code           string
}

// RuleAction is the action for a listener rule.
type RuleAction struct {
	ForwardTargetGroups     []*WeightedTargetGroup `json:"forwardTargetGroups,omitempty"`
	FixedResponseStatusCode int32                  `json:"fixedResponseStatusCode,omitempty"`
	IsFixedResponse         bool                   `json:"isFixedResponse,omitempty"`
}

// WeightedTargetGroup is a weighted target group for forward actions.
type WeightedTargetGroup struct {
	TargetGroupID string `json:"targetGroupId"`
	Weight        int32  `json:"weight"`
}

// RuleMatch is the match conditions for a listener rule.
type RuleMatch struct {
	HTTPMethod        string         `json:"httpMethod,omitempty"`
	PathMatchType     string         `json:"pathMatchType,omitempty"`
	PathMatchValue    string         `json:"pathMatchValue,omitempty"`
	HeaderMatches     []*HeaderMatch `json:"headerMatches,omitempty"`
	PathCaseSensitive bool           `json:"pathCaseSensitive,omitempty"`
}

// HeaderMatch is an HTTP header match condition.
type HeaderMatch struct {
	Name          string `json:"name"`
	MatchType     string `json:"matchType"`
	MatchValue    string `json:"matchValue"`
	CaseSensitive bool   `json:"caseSensitive"`
}

// TargetGroup represents a VPC Lattice target group.
type TargetGroup struct {
	CreatedAt     time.Time
	LastUpdatedAt time.Time
	ARN           string
	ID            string
	Name          string
	Type          string
	Status        string
	Config        *TargetGroupConfig
	ServiceARNs   []string
}

// TargetGroupSummary is a target group entry for list responses.
type TargetGroupSummary struct {
	CreatedAt                   time.Time
	LastUpdatedAt               time.Time
	ARN                         string
	ID                          string
	Name                        string
	Type                        string
	Status                      string
	Protocol                    string
	VpcID                       string
	IPAddressType               string
	LambdaEventStructureVersion string
	ServiceARNs                 []string
	Port                        int32
}

// TargetGroupConfig is the configuration for a target group.
type TargetGroupConfig struct {
	HealthCheck                 *HealthCheckConfig `json:"healthCheck,omitempty"`
	Protocol                    string             `json:"protocol,omitempty"`
	ProtocolVersion             string             `json:"protocolVersion,omitempty"`
	VpcID                       string             `json:"vpcId,omitempty"`
	IPAddressType               string             `json:"ipAddressType,omitempty"`
	LambdaEventStructureVersion string             `json:"lambdaEventStructureVersion,omitempty"`
	Port                        int32              `json:"port,omitempty"`
}

// HealthCheckConfig is the health check configuration for a target group.
type HealthCheckConfig struct {
	Protocol                   string `json:"protocol,omitempty"`
	ProtocolVersion            string `json:"protocolVersion,omitempty"`
	Path                       string `json:"path,omitempty"`
	MatcherHTTPCode            string `json:"matcherHttpCode,omitempty"`
	Port                       int32  `json:"port,omitempty"`
	HealthyThresholdCount      int32  `json:"healthyThresholdCount,omitempty"`
	UnhealthyThresholdCount    int32  `json:"unhealthyThresholdCount,omitempty"`
	HealthCheckIntervalSeconds int32  `json:"healthCheckIntervalSeconds,omitempty"`
	HealthCheckTimeoutSeconds  int32  `json:"healthCheckTimeoutSeconds,omitempty"`
	Enabled                    bool   `json:"enabled,omitempty"`
}

// Target is a target registered to a target group.
type Target struct {
	ID   string
	Port int32
}

// TargetSummary is a target entry for list responses.
type TargetSummary struct {
	ID         string
	Status     string
	ReasonCode string
	Port       int32
}

// TargetFailure is a target registration/deregistration failure.
type TargetFailure struct {
	ID      string
	Code    string
	Message string
	Port    int32
}

// AccessLogSubscription represents a VPC Lattice access log subscription.
type AccessLogSubscription struct {
	CreatedAt             time.Time
	LastUpdatedAt         time.Time
	ARN                   string
	ID                    string
	ResourceARN           string
	ResourceID            string
	DestinationARN        string
	ServiceNetworkLogType string
}

// AccessLogSubscriptionSummary is a summary for list responses.
type AccessLogSubscriptionSummary struct {
	CreatedAt             time.Time
	LastUpdatedAt         time.Time
	ARN                   string
	ID                    string
	ResourceARN           string
	ResourceID            string
	DestinationARN        string
	ServiceNetworkLogType string
}

// AuthPolicy represents an auth policy on a VPC Lattice resource.
type AuthPolicy struct {
	Policy string
	State  string
}

// ResourceGateway represents a VPC Lattice resource gateway -- a point of
// ingress into a VPC where a resource (behind a ResourceConfiguration)
// resides.
type ResourceGateway struct {
	CreatedAt                   time.Time
	LastUpdatedAt               time.Time
	ARN                         string
	ID                          string
	Name                        string
	VpcID                       string
	IPAddressType               string
	ResourceConfigDNSResolution string
	Status                      string
	SecurityGroupIDs            []string
	SubnetIDs                   []string
	Ipv4AddressesPerEni         int32
	ServiceManaged              bool
}

// ResourceGatewaySummary is a resource gateway entry for list responses.
type ResourceGatewaySummary struct {
	CreatedAt                   time.Time
	LastUpdatedAt               time.Time
	ARN                         string
	ID                          string
	Name                        string
	VpcID                       string
	IPAddressType               string
	ResourceConfigDNSResolution string
	Status                      string
	SecurityGroupIDs            []string
	SubnetIDs                   []string
	Ipv4AddressesPerEni         int32
}

// ResourceConfigurationDefinition identifies the underlying resource a
// ResourceConfiguration points at (types.ResourceConfigurationDefinition, a
// wire union of arnResource/dnsResource/ipResource). Exactly one of
// ArnValue/DomainName+IPAddressType/IPAddress is populated, selected by Kind.
type ResourceConfigurationDefinition struct {
	Kind          string // "arnResource" | "dnsResource" | "ipResource"
	ArnValue      string
	DomainName    string
	IPAddressType string
	IPAddress     string
}

// ResourceConfiguration represents a VPC Lattice resource configuration.
type ResourceConfiguration struct {
	CreatedAt                    time.Time
	LastUpdatedAt                time.Time
	Definition                   *ResourceConfigurationDefinition
	ARN                          string
	ID                           string
	Name                         string
	Type                         string
	Status                       string
	Protocol                     string
	ResourceGatewayID            string
	ResourceConfigurationGroupID string
	CustomDomainName             string
	GroupDomain                  string
	DomainVerificationID         string
	DomainVerificationARN        string
	DomainVerificationStatus     string
	FailureReason                string
	PortRanges                   []string
	AllowShareableAssoc          bool
	AmazonManaged                bool
}

// ResourceConfigurationSummary is a resource configuration entry for list
// responses.
type ResourceConfigurationSummary struct {
	CreatedAt                    time.Time
	LastUpdatedAt                time.Time
	ARN                          string
	ID                           string
	Name                         string
	Type                         string
	Status                       string
	ResourceGatewayID            string
	ResourceConfigurationGroupID string
	CustomDomainName             string
	GroupDomain                  string
	DomainVerificationID         string
	AmazonManaged                bool
}

// ServiceNetworkResourceAssociation associates a resource configuration with
// a service network.
type ServiceNetworkResourceAssociation struct {
	CreatedAt                 time.Time
	LastUpdatedAt             time.Time
	ARN                       string
	ID                        string
	ResourceConfigurationARN  string
	ResourceConfigurationID   string
	ResourceConfigurationName string
	ServiceNetworkARN         string
	ServiceNetworkID          string
	ServiceNetworkName        string
	Status                    string
	CreatedBy                 string
	PrivateDNSEnabled         bool
}

// ServiceNetworkResourceAssociationSummary is a summary for list responses.
type ServiceNetworkResourceAssociationSummary struct {
	CreatedAt                 time.Time
	ARN                       string
	ID                        string
	ResourceConfigurationARN  string
	ResourceConfigurationID   string
	ResourceConfigurationName string
	ServiceNetworkARN         string
	ServiceNetworkID          string
	ServiceNetworkName        string
	Status                    string
}

// DomainVerification represents a custom domain ownership verification.
type DomainVerification struct {
	CreatedAt        time.Time
	LastVerifiedTime *time.Time
	ARN              string
	ID               string
	DomainName       string
	Status           string
}

// DomainVerificationSummary is a summary for list responses.
type DomainVerificationSummary struct {
	CreatedAt        time.Time
	LastVerifiedTime *time.Time
	ARN              string
	ID               string
	DomainName       string
	Status           string
}

// ResourceEndpointAssociationSummary is a summary for list responses.
// Real AWS populates this from EC2 VPC endpoints of type Resource pointed at
// a ResourceConfiguration -- vpc-lattice itself has no Create operation for
// this resource. This backend has no EC2 VPC-endpoint cross-service
// modeling, so ListResourceEndpointAssociations always returns empty (an
// honest reflection of "never created", not a fabricated entry) -- see
// service_network_resource_associations.go.
type ResourceEndpointAssociationSummary struct {
	CreatedAt                time.Time
	ARN                      string
	ID                       string
	ResourceConfigurationARN string
	ResourceConfigurationID  string
	VpcEndpointID            string
	VpcEndpointOwner         string
}

// ServiceNetworkVpcEndpointAssociationSummary is a summary for list
// responses. Same structural note as ResourceEndpointAssociationSummary:
// populated from EC2 VPC endpoints of type ServiceNetwork, which this
// backend doesn't model.
type ServiceNetworkVpcEndpointAssociationSummary struct {
	CreatedAt         time.Time
	ID                string
	ServiceNetworkARN string
	ServiceNetworkID  string
	VpcEndpointID     string
	VpcID             string
	VpcEndpointOwner  string
}

var _ StorageBackend = (*InMemoryBackend)(nil)

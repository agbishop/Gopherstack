package eks

import (
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

const (
	statusActive      = "ACTIVE"
	statusInProgress  = "InProgress"
	statusCancelled   = "Cancelled"
	defaultK8sVersion = "1.32"
	priorK8sVersion   = "1.31"
)

const (
	// typeVersionRollback is the only UpdateType real EKS currently allows
	// CancelUpdate to act on (Kubernetes version rollback on EKS Auto Mode
	// clusters) -- verified against aws-sdk-go-v2/service/eks's CancelUpdate
	// doc comment and types.UpdateTypeVersionRollback.
	typeVersionRollback = "VersionRollback"

	cancellationStatusSuccessful = "Successful"
)

const (
	keyNetworking               = "networking"
	keyCompatibilities          = "compatibilities"
	keyClusterVersion           = "clusterVersion"
	keyDefaultVersion           = "defaultVersion"
	keyEndOfStandardSupportDate = "endOfStandardSupportDate"
	keyEndOfExtendedSupportDate = "endOfExtendedSupportDate"
)

const (
	keyAddonName              = "addonName"
	keyAddonVersions          = "addonVersions"
	keyAddonVersion           = "addonVersion"
	typeUpgradeReadiness      = "UPGRADE_READINESS"
	typeVersionUpdate         = "VersionUpdate"
	connectorActivationWindow = 24 * time.Hour
)

const (
	statusPassing    = "PASSING"
	statusCreating   = "CREATING"
	statusFailed     = "FAILED"
	statusDegraded   = "DEGRADED"
	statusDeleting   = "DELETING"
	statusSuccessful = "Successful"
)

// VpcConfig captures the cluster VPC configuration returned by AWS.
type VpcConfig struct {
	ClusterSecurityGroupID string   `json:"clusterSecurityGroupId,omitempty"`
	VpcID                  string   `json:"vpcId,omitempty"`
	SubnetIDs              []string `json:"subnetIds,omitempty"`
	SecurityGroupIDs       []string `json:"securityGroupIds,omitempty"`
	PublicAccessCIDRs      []string `json:"publicAccessCidrs,omitempty"`
	EndpointPrivateAccess  bool     `json:"endpointPrivateAccess"`
	EndpointPublicAccess   bool     `json:"endpointPublicAccess"`
}

// AccessConfig holds the cluster authentication mode configuration.
type AccessConfig struct {
	AuthenticationMode                      string `json:"authenticationMode,omitempty"`
	BootstrapClusterCreatorAdminPermissions bool   `json:"bootstrapClusterCreatorAdminPermissions"`
}

// ComputeConfig holds the EKS Auto Mode compute configuration.
type ComputeConfig struct {
	NodeRoleARN string   `json:"nodeRoleArn,omitempty"`
	NodePools   []string `json:"nodePools,omitempty"`
	Enabled     bool     `json:"enabled"`
}

// BlockStorageConfig holds EKS Auto Mode block storage settings.
type BlockStorageConfig struct {
	Enabled bool `json:"enabled"`
}

// StorageConfig holds the EKS Auto Mode storage configuration.
type StorageConfig struct {
	BlockStorage *BlockStorageConfig `json:"blockStorage,omitempty"`
}

// ElasticLoadBalancingConfig holds EKS Auto Mode load balancer settings.
type ElasticLoadBalancingConfig struct {
	Enabled bool `json:"enabled"`
}

// KubernetesNetworkConfig captures cluster networking parameters. The real
// SDK's KubernetesNetworkConfigRequest/KubernetesNetworkConfigResponse
// (eks@v1.90.4 types/types.go:1597,1645) both declare ElasticLoadBalancing as
// a sibling of IpFamily/ServiceIpv4Cidr/ServiceIpv6Cidr under ONE
// "kubernetesNetworkConfig" wire key -- there is no separate top-level
// "networkingConfig" object in real AWS. gopherstack-tp8x: a prior version of
// this type split ElasticLoadBalancing into a second, separately-named
// top-level Cluster.NetworkingConfig field/JSON key that a real client never
// reads or sends.
type KubernetesNetworkConfig struct {
	ElasticLoadBalancing *ElasticLoadBalancingConfig `json:"elasticLoadBalancing,omitempty"`
	IPFamily             string                      `json:"ipFamily,omitempty"`
	ServiceIPv4CIDR      string                      `json:"serviceIpv4Cidr,omitempty"`
	ServiceIPv6CIDR      string                      `json:"serviceIpv6Cidr,omitempty"`
}

// ClusterLogEntry represents one log-type group in the structured logging config.
type ClusterLogEntry struct {
	Types   []string `json:"types"`
	Enabled bool     `json:"enabled"`
}

// ConnectorConfig holds metadata for externally-registered clusters.
type ConnectorConfig struct {
	ActivationCode   string `json:"activationCode,omitempty"`
	ActivationExpiry string `json:"activationExpiry,omitempty"`
	ActivationID     string `json:"activationId,omitempty"`
	Provider         string `json:"provider,omitempty"`
	RoleARN          string `json:"roleArn,omitempty"`
}

// Cluster represents an EKS cluster.
//
// The Tags field is backend-owned. Callers must treat the returned pointer as
// read-only; mutate tags only via TagResource / CreateCluster.
type Cluster struct {
	CreatedAt               time.Time                `json:"createdAt"`
	Tags                    *tags.Tags               `json:"tags,omitempty"`
	VpcConfig               *VpcConfig               `json:"resourcesVpcConfig,omitempty"`
	KubernetesNetworkConfig *KubernetesNetworkConfig `json:"kubernetesNetworkConfig,omitempty"`
	AccessConfig            *AccessConfig            `json:"accessConfig,omitempty"`
	ComputeConfig           *ComputeConfig           `json:"computeConfig,omitempty"`
	StorageConfig           *StorageConfig           `json:"storageConfig,omitempty"`
	ConnectorConfig         *ConnectorConfig         `json:"connectorConfig,omitempty"`
	ARN                     string                   `json:"arn"`
	Name                    string                   `json:"name"`
	Endpoint                string                   `json:"endpoint,omitempty"`
	OIDCIssuer              string                   `json:"oidcIssuer,omitempty"`
	Version                 string                   `json:"version"`
	Status                  string                   `json:"status"`
	RoleARN                 string                   `json:"roleArn,omitempty"`
	AccountID               string                   `json:"accountId"`
	Region                  string                   `json:"region"`
	PlatformVersion         string                   `json:"platformVersion,omitempty"`
	CertificateAuthority    string                   `json:"certificateAuthority,omitempty"`
	ClusterLogging          []ClusterLogEntry        `json:"clusterLogging,omitempty"`
	EncryptionConfig        []EncryptionConfig       `json:"encryptionConfig,omitempty"`
}

// NodegroupTaint represents a Kubernetes taint applied to managed nodes.
type NodegroupTaint struct {
	Key    string `json:"key"`
	Value  string `json:"value,omitempty"`
	Effect string `json:"effect"`
}

// RemoteAccess captures SSH remote-access configuration for a node group.
type RemoteAccess struct {
	EC2SSHKey            string   `json:"ec2SshKey,omitempty"`
	SourceSecurityGroups []string `json:"sourceSecurityGroups,omitempty"`
}

// LaunchTemplate captures the launch-template reference for a node group.
type LaunchTemplate struct {
	ID      string `json:"id,omitempty"`
	Name    string `json:"name,omitempty"`
	Version string `json:"version,omitempty"`
}

// AutoScalingGroup holds the name of an ASG backing the node group.
type AutoScalingGroup struct {
	Name string `json:"name"`
}

// NodegroupResources captures AWS resources backing the node group.
type NodegroupResources struct {
	AutoScalingGroups []AutoScalingGroup `json:"autoScalingGroups,omitempty"`
}

// NodegroupUpdateConfig holds the nodegroup update strategy settings.
type NodegroupUpdateConfig struct {
	MaxUnavailable           *int32 `json:"maxUnavailable,omitempty"`
	MaxUnavailablePercentage *int32 `json:"maxUnavailablePercentage,omitempty"`
}

// Nodegroup represents an EKS managed node group.
//
// The Tags field is backend-owned. Callers must treat the returned pointer as
// read-only; mutate tags only via TagResource / CreateNodegroup.
type Nodegroup struct {
	CreatedAt      time.Time              `json:"createdAt"`
	Tags           *tags.Tags             `json:"tags,omitempty"`
	Labels         map[string]string      `json:"labels,omitempty"`
	RemoteAccess   *RemoteAccess          `json:"remoteAccess,omitempty"`
	LaunchTemplate *LaunchTemplate        `json:"launchTemplate,omitempty"`
	Resources      *NodegroupResources    `json:"resources,omitempty"`
	UpdateConfig   *NodegroupUpdateConfig `json:"updateConfig,omitempty"`
	CapacityType   string                 `json:"capacityType,omitempty"`
	Region         string                 `json:"region"`
	ARN            string                 `json:"nodegroupArn"`
	NodeRole       string                 `json:"nodeRole,omitempty"`
	Status         string                 `json:"status"`
	AMIType        string                 `json:"amiType,omitempty"`
	NodegroupName  string                 `json:"nodegroupName"`
	ClusterName    string                 `json:"clusterName"`
	Version        string                 `json:"version,omitempty"`
	ReleaseVersion string                 `json:"releaseVersion,omitempty"`
	AccountID      string                 `json:"accountId"`
	Taints         []NodegroupTaint       `json:"taints,omitempty"`
	InstanceTypes  []string               `json:"instanceTypes,omitempty"`
	Subnets        []string               `json:"subnets,omitempty"`
	DesiredSize    int32                  `json:"desiredSize"`
	MinSize        int32                  `json:"minSize"`
	MaxSize        int32                  `json:"maxSize"`
	DiskSize       int32                  `json:"diskSize,omitempty"`
}

// AccessEntry represents an EKS access entry that grants a principal access to a cluster.
type AccessEntry struct {
	CreatedAt        time.Time  `json:"createdAt"`
	ModifiedAt       time.Time  `json:"modifiedAt"`
	Tags             *tags.Tags `json:"tags,omitempty"`
	PrincipalARN     string     `json:"principalArn"`
	ClusterName      string     `json:"clusterName"`
	ARN              string     `json:"accessEntryArn"`
	Type             string     `json:"type"`
	Username         string     `json:"username,omitempty"`
	KubernetesGroups []string   `json:"kubernetesGroups,omitempty"`
}

// AccessPolicyAssociation represents an access policy associated with an access entry.
type AccessPolicyAssociation struct {
	AssociatedAt time.Time      `json:"associatedAt"`
	AccessScope  map[string]any `json:"accessScope,omitempty"`
	PolicyARN    string         `json:"policyArn"`
	ClusterName  string         `json:"clusterName"`
	PrincipalARN string         `json:"principalArn"`
}

// EncryptionConfig represents a cluster encryption configuration.
type EncryptionConfig struct {
	Provider  map[string]string `json:"provider,omitempty"`
	Resources []string          `json:"resources,omitempty"`
}

// IdentityProviderConfig represents an identity provider configuration for a cluster.
//
// OIDC holds the flat string-valued OIDC fields (issuerUrl, clientId,
// usernameClaim, usernamePrefix, groupsClaim, groupsPrefix); RequiredClaims
// is kept separate since the real
// aws-sdk-go-v2/service/eks.OidcIdentityProviderConfig.RequiredClaims is a
// nested map, not a flat string like the other OIDC fields.
type IdentityProviderConfig struct {
	CreatedAt      time.Time         `json:"createdAt"`
	Tags           *tags.Tags        `json:"tags,omitempty"`
	OIDC           map[string]string `json:"oidc,omitempty"`
	RequiredClaims map[string]string `json:"requiredClaims,omitempty"`
	ClusterName    string            `json:"clusterName"`
	Name           string            `json:"name"`
	ARN            string            `json:"arn"`
	Type           string            `json:"type"`
	Status         string            `json:"status"`
}

// AddonHealth represents the health status of an EKS managed add-on.
type AddonHealth struct {
	Issues []map[string]string `json:"issues,omitempty"`
}

// Addon represents an EKS managed add-on.
type Addon struct {
	CreatedAt               time.Time    `json:"createdAt"`
	Health                  *AddonHealth `json:"health,omitempty"`
	Tags                    *tags.Tags   `json:"tags,omitempty"`
	ARN                     string       `json:"addonArn"`
	ClusterName             string       `json:"clusterName"`
	AddonName               string       `json:"addonName"`
	AddonVersion            string       `json:"addonVersion,omitempty"`
	MarketplaceVersion      string       `json:"marketplaceVersion,omitempty"`
	Status                  string       `json:"status"`
	ServiceAccountRoleARN   string       `json:"serviceAccountRoleArn,omitempty"`
	Configuration           string       `json:"configurationValues,omitempty"`
	ResolveConflicts        string       `json:"resolveConflicts,omitempty"`
	PodIdentityAssociations []string     `json:"podIdentityAssociations,omitempty"`
}

// CapabilityIssue represents a single health issue affecting a Capability.
type CapabilityIssue struct {
	Code        string   `json:"code,omitempty"`
	Message     string   `json:"message,omitempty"`
	ResourceIDs []string `json:"resourceIds,omitempty"`
}

// CapabilityHealth mirrors aws-sdk-go-v2/service/eks/types.CapabilityHealth.
type CapabilityHealth struct {
	Issues []CapabilityIssue `json:"issues"`
}

// Capability represents an EKS capability. Capabilities are cluster-scoped:
// CapabilityName is unique per cluster, not globally (verified against
// aws-sdk-go-v2/service/eks -- CreateCapabilityInput requires ClusterName,
// CapabilityName, Type, RoleArn, and DeletePropagationPolicy; the route is
// /clusters/{clusterName}/capabilities[/{capabilityName}]).
type Capability struct {
	CreatedAt               time.Time         `json:"createdAt"`
	ModifiedAt              time.Time         `json:"modifiedAt"`
	Tags                    *tags.Tags        `json:"tags,omitempty"`
	Configuration           map[string]any    `json:"configuration,omitempty"`
	Health                  *CapabilityHealth `json:"health,omitempty"`
	ClusterName             string            `json:"clusterName"`
	CapabilityName          string            `json:"capabilityName"`
	ARN                     string            `json:"arn"`
	Type                    string            `json:"type,omitempty"`
	RoleARN                 string            `json:"roleArn,omitempty"`
	DeletePropagationPolicy string            `json:"deletePropagationPolicy,omitempty"`
	Version                 string            `json:"version,omitempty"`
	Status                  string            `json:"status"`
}

// SubscriptionTerm holds the term duration/unit for an EKS Anywhere
// subscription (required on create -- verified against
// aws-sdk-go-v2/service/eks's CreateEksAnywhereSubscriptionInput.Term).
type SubscriptionTerm struct {
	Unit     string `json:"unit,omitempty"`
	Duration int32  `json:"duration,omitempty"`
}

// AnywhereSubscription represents an EKS Anywhere subscription.
type AnywhereSubscription struct {
	CreatedAt       time.Time         `json:"createdAt"`
	EffectiveDate   time.Time         `json:"effectiveDate"`
	ExpirationDate  time.Time         `json:"expirationDate"`
	Tags            *tags.Tags        `json:"tags,omitempty"`
	Term            *SubscriptionTerm `json:"term,omitempty"`
	ID              string            `json:"id"`
	ARN             string            `json:"arn"`
	Name            string            `json:"name"`
	Status          string            `json:"status"`
	LicenseType     string            `json:"licenseType,omitempty"`
	LicenseQuantity int32             `json:"licenseQuantity,omitempty"`
	AutoRenew       bool              `json:"autoRenew"`
}

// FargateProfileSelector is a namespace/labels selector for a Fargate profile.
type FargateProfileSelector struct {
	Labels    map[string]string `json:"labels,omitempty"`
	Namespace string            `json:"namespace"`
}

// FargateProfileIssue represents a single health issue reported for a Fargate profile.
type FargateProfileIssue struct {
	Code        string   `json:"code,omitempty"`
	Message     string   `json:"message,omitempty"`
	ResourceIDs []string `json:"resourceIds,omitempty"`
}

// FargateProfileHealth mirrors aws-sdk-go-v2/service/eks/types.FargateProfileHealth.
type FargateProfileHealth struct {
	Issues []FargateProfileIssue `json:"issues"`
}

// FargateProfile represents an EKS Fargate profile.
type FargateProfile struct {
	CreatedAt           time.Time                `json:"createdAt"`
	Tags                *tags.Tags               `json:"tags,omitempty"`
	Health              *FargateProfileHealth    `json:"health,omitempty"`
	Subnets             []string                 `json:"subnets,omitempty"`
	ClusterName         string                   `json:"clusterName"`
	FargateProfileName  string                   `json:"fargateProfileName"`
	ARN                 string                   `json:"fargateProfileArn"`
	PodExecutionRoleARN string                   `json:"podExecutionRoleArn,omitempty"`
	Status              string                   `json:"status"`
	Selectors           []FargateProfileSelector `json:"selectors,omitempty"`
}

// PodIdentityAssociation represents an EKS pod identity association.
type PodIdentityAssociation struct {
	CreatedAt          time.Time  `json:"createdAt"`
	ModifiedAt         time.Time  `json:"modifiedAt"`
	Tags               *tags.Tags `json:"tags,omitempty"`
	ClusterName        string     `json:"clusterName"`
	AssociationID      string     `json:"associationId"`
	ARN                string     `json:"associationArn"`
	Namespace          string     `json:"namespace"`
	ServiceAccount     string     `json:"serviceAccount"`
	RoleARN            string     `json:"roleArn,omitempty"`
	OwnerARN           string     `json:"ownerArn,omitempty"`
	ExternalID         string     `json:"externalId,omitempty"`
	Policy             string     `json:"policy,omitempty"`
	DisableSessionTags bool       `json:"disableSessionTags"`
}

// PodIdentityAssociationSpec is one entry of UpdateAddonInput's
// PodIdentityAssociations, matching types.AddonPodIdentityAssociations
// (RoleArn + ServiceAccount only -- no namespace).
type PodIdentityAssociationSpec struct {
	RoleARN        string
	ServiceAccount string
}

// Insight represents an EKS cluster insight.
type Insight struct {
	LastRefreshTime time.Time         `json:"lastRefreshTime"`
	LastTransition  time.Time         `json:"lastTransitionTime"`
	AdditionalInfo  map[string]string `json:"additionalInfo,omitempty"`
	ID              string            `json:"id"`
	ClusterName     string            `json:"clusterName"`
	Category        string            `json:"category"`
	Status          string            `json:"status"`
	Description     string            `json:"description,omitempty"`
	Recommendation  string            `json:"recommendation,omitempty"`
}

// InsightsRefresh represents the cluster-level (singleton -- there is no
// per-refresh id in the real API) EKS insights refresh operation state.
type InsightsRefresh struct {
	StartedAt   time.Time `json:"startedAt"`
	EndedAt     time.Time `json:"endedAt,omitzero"`
	ClusterName string    `json:"clusterName"`
	Status      string    `json:"status"`
	Message     string    `json:"message,omitempty"`
}

// UpdateParam represents a single parameter changed by an EKS update operation.
type UpdateParam struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// UpdateError represents an error encountered during an EKS update.
type UpdateError struct {
	ErrorCode    string   `json:"errorCode"`
	ErrorMessage string   `json:"errorMessage"`
	ResourceIDs  []string `json:"resourceIds,omitempty"`
}

// Cancellation represents the latest cancellation state of an Update, present
// only when a cancellation was attempted (e.g. via CancelUpdate).
type Cancellation struct {
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

// Update represents an EKS update record. NodegroupName is backend-internal
// (not part of the real Update wire shape) -- it exists only so ListUpdates
// can honor ListUpdatesInput.NodegroupName.
type Update struct {
	CreatedAt     time.Time     `json:"createdAt"`
	Cancellation  *Cancellation `json:"cancellation,omitempty"`
	ID            string        `json:"id"`
	ClusterName   string        `json:"clusterName"`
	NodegroupName string        `json:"-"`
	Status        string        `json:"status"`
	Type          string        `json:"type"`
	Params        []UpdateParam `json:"params,omitempty"`
	Errors        []UpdateError `json:"errors,omitempty"`
}

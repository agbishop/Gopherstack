package medialive

import (
	"context"
	"time"
)

// StorageBackend is the interface for MediaLive storage operations.
type StorageBackend interface {
	// Channels
	CreateChannel(
		name, channelClass, roleArn string,
		anywhereSettings ChannelAnywhereSettings,
		extras ChannelCreateExtras,
		tags map[string]string,
	) (*Channel, error)
	DescribeChannel(channelID string) (*Channel, error)
	UpdateChannel(
		channelID, name, roleArn string,
		anywhereSettings ChannelAnywhereSettings,
		hasAnywhereSettings bool,
		extras ChannelUpdateExtras,
	) (*Channel, error)
	DeleteChannel(channelID string) (*Channel, error)
	ListChannels(maxResults int, nextToken string) ([]*ChannelSummary, string, error)
	StartChannel(channelID string) (*Channel, error)
	StopChannel(channelID string) (*Channel, error)

	// Inputs
	CreateInput(name, inputType, roleArn string, sdiSources []string, tags map[string]string) (*Input, error)
	DescribeInput(inputID string) (*Input, error)
	UpdateInput(inputID, name, roleArn string, sdiSources []string, sdiSourcesSet bool) (*Input, error)
	DeleteInput(inputID string) error
	ListInputs(maxResults int, nextToken string) ([]*InputSummary, string, error)

	// InputSecurityGroups
	CreateInputSecurityGroup(
		whitelistRules []WhitelistRule,
		tags map[string]string,
	) (*InputSecurityGroup, error)
	DescribeInputSecurityGroup(groupID string) (*InputSecurityGroup, error)
	UpdateInputSecurityGroup(
		groupID string,
		whitelistRules []WhitelistRule,
	) (*InputSecurityGroup, error)
	DeleteInputSecurityGroup(groupID string) error
	ListInputSecurityGroups(
		maxResults int,
		nextToken string,
	) ([]*InputSecurityGroupSummary, string, error)

	// Multiplexes
	CreateMultiplex(
		name string,
		availabilityZones []string,
		settings MultiplexSettings,
		tags map[string]string,
	) (*Multiplex, error)
	DescribeMultiplex(multiplexID string) (*Multiplex, error)
	UpdateMultiplex(multiplexID, name string, settings MultiplexSettings) (*Multiplex, error)
	DeleteMultiplex(multiplexID string) (*Multiplex, error)
	ListMultiplexes(maxResults int, nextToken string) ([]*MultiplexSummary, string, error)
	StartMultiplex(multiplexID string) (*Multiplex, error)
	StopMultiplex(multiplexID string) (*Multiplex, error)

	// MultiplexPrograms
	CreateMultiplexProgram(
		multiplexID string,
		prog MultiplexProgramSettings,
	) (*MultiplexProgram, error)
	DescribeMultiplexProgram(multiplexID, programName string) (*MultiplexProgram, error)
	UpdateMultiplexProgram(
		multiplexID string,
		prog MultiplexProgramSettings,
	) (*MultiplexProgram, error)
	DeleteMultiplexProgram(multiplexID, programName string) (*MultiplexProgram, error)
	ListMultiplexPrograms(
		multiplexID string,
		maxResults int,
		nextToken string,
	) ([]*MultiplexProgramSummary, string, error)

	// Tags
	CreateTags(resourceARN string, tags map[string]string) error
	DeleteTags(resourceARN string, tagKeys []string) error
	ListTagsForResource(resourceARN string) (map[string]string, error)

	// InputDevices
	ClaimDevice(id string) (*InputDevice, error)
	ListInputDevices(maxResults int, nextToken string) ([]*InputDevice, string, error)
	DescribeInputDevice(deviceID string) (*InputDevice, error)
	UpdateInputDevice(deviceID, name string) (*InputDevice, error)
	RebootInputDevice(deviceID string) error
	TransferInputDevice(deviceID, targetCustomerID, targetRegion, message string) error
	AcceptInputDeviceTransfer(deviceID string) error
	CancelInputDeviceTransfer(deviceID string) error
	RejectInputDeviceTransfer(deviceID string) error
	ListInputDeviceTransfers(
		transferType string,
		maxResults int,
		nextToken string,
	) ([]*InputDeviceTransfer, string, error)

	// Clusters
	CreateCluster(
		name, clusterType, instanceRoleArn string,
		networkSettings ClusterNetworkSettings,
		tags map[string]string,
	) (*Cluster, error)
	DescribeCluster(clusterID string) (*Cluster, error)
	UpdateCluster(
		clusterID, name string,
		networkSettings ClusterNetworkSettings,
		hasNetworkSettings bool,
	) (*Cluster, error)
	DeleteCluster(clusterID string) (*Cluster, error)
	ListClusters(maxResults int, nextToken string) ([]*ClusterSummary, string, error)

	// Nodes
	CreateNode(clusterID, name, role string, tags map[string]string) (*Node, error)
	DescribeNode(clusterID, nodeID string) (*Node, error)
	UpdateNode(clusterID, nodeID, name, role string) (*Node, error)
	UpdateNodeState(clusterID, nodeID, state string) (*Node, error)
	DeleteNode(clusterID, nodeID string) (*Node, error)
	ListNodes(clusterID string, maxResults int, nextToken string) ([]*NodeSummary, string, error)
	CreateNodeRegistrationScript(clusterID string) (string, error)
	ListClusterAlerts(
		clusterID string,
		maxResults int,
		nextToken string,
		stateFilter string,
	) ([]map[string]any, string, error)

	// SignalMaps
	CreateSignalMap(
		name, description, discoveryEntryPointArn string,
		cwGroupIDs, ebGroupIDs []string,
		tags map[string]string,
	) (*SignalMap, error)
	GetSignalMap(identifier string) (*SignalMap, error)
	ListSignalMaps(
		maxResults int, nextToken string, cwGroupIdentifier, ebGroupIdentifier string,
	) ([]*SignalMap, string, error)
	DeleteSignalMap(identifier string) error
	StartUpdateSignalMap(
		identifier, name, description string,
		cwGroupIDs, ebGroupIDs []string,
	) (*SignalMap, error)
	StartMonitorDeployment(identifier string) (*SignalMap, error)

	// CloudWatch Alarm Template Groups
	CreateCloudWatchAlarmTemplateGroup(
		name, description string,
		tags map[string]string,
	) (*CloudWatchAlarmTemplateGroup, error)
	GetCloudWatchAlarmTemplateGroup(identifier string) (*CloudWatchAlarmTemplateGroup, error)
	ListCloudWatchAlarmTemplateGroups(
		maxResults int,
		nextToken string,
		signalMapIdentifier string,
	) ([]*CloudWatchAlarmTemplateGroupSummary, string, error)
	UpdateCloudWatchAlarmTemplateGroup(
		identifier, name, description string,
	) (*CloudWatchAlarmTemplateGroup, error)
	DeleteCloudWatchAlarmTemplateGroup(identifier string) error

	// CloudWatch Alarm Templates
	CreateCloudWatchAlarmTemplate(
		name string,
		description string,
		groupIdentifier string,
		metricName string,
		namespace string,
		statistic string,
		comparisonOperator string,
		targetResourceType string,
		treatMissingData string,
		threshold float64,
		evaluationPeriods, datapointsToAlarm, period int32,
		tags map[string]string,
	) (*CloudWatchAlarmTemplate, error)
	GetCloudWatchAlarmTemplate(identifier string) (*CloudWatchAlarmTemplate, error)
	ListCloudWatchAlarmTemplates(
		maxResults int,
		nextToken string,
		groupIdentifier, signalMapIdentifier string,
	) ([]*CloudWatchAlarmTemplate, string, error)
	UpdateCloudWatchAlarmTemplate(
		identifier string,
		name string,
		description string,
		groupIdentifier string,
		metricName string,
		namespace string,
		statistic string,
		comparisonOperator string,
		targetResourceType string,
		treatMissingData string,
		threshold float64,
		evaluationPeriods, datapointsToAlarm, period int32,
	) (*CloudWatchAlarmTemplate, error)
	DeleteCloudWatchAlarmTemplate(identifier string) error

	// EventBridge Rule Template Groups
	CreateEventBridgeRuleTemplateGroup(
		name, description string,
		tags map[string]string,
	) (*EventBridgeRuleTemplateGroup, error)
	GetEventBridgeRuleTemplateGroup(identifier string) (*EventBridgeRuleTemplateGroup, error)
	ListEventBridgeRuleTemplateGroups(
		maxResults int,
		nextToken string,
		signalMapIdentifier string,
	) ([]*EventBridgeRuleTemplateGroupSummary, string, error)
	UpdateEventBridgeRuleTemplateGroup(
		identifier, name, description string,
	) (*EventBridgeRuleTemplateGroup, error)
	DeleteEventBridgeRuleTemplateGroup(identifier string) error

	// EventBridge Rule Templates
	CreateEventBridgeRuleTemplate(
		name, description, groupIdentifier, eventType string,
		eventTargets []EventBridgeRuleTemplateTarget,
		tags map[string]string,
	) (*EventBridgeRuleTemplate, error)
	GetEventBridgeRuleTemplate(identifier string) (*EventBridgeRuleTemplate, error)
	ListEventBridgeRuleTemplates(
		maxResults int,
		nextToken string,
		groupIdentifier, signalMapIdentifier string,
	) ([]*EventBridgeRuleTemplateSummary, string, error)
	UpdateEventBridgeRuleTemplate(
		identifier, name, description, groupIdentifier, eventType string,
		eventTargets []EventBridgeRuleTemplateTarget,
	) (*EventBridgeRuleTemplate, error)
	DeleteEventBridgeRuleTemplate(identifier string) error

	// Offerings (read-only catalog)
	ListOfferings(maxResults int, nextToken string) ([]*Offering, string, error)
	DescribeOffering(offeringID string) (*Offering, error)

	// Reservations
	PurchaseOffering(
		offeringID, name, start string,
		count int32,
		renewalSettings RenewalSettings,
		tags map[string]string,
	) (*Reservation, error)
	ListReservations(
		maxResults int, nextToken string, filter ReservationFilter,
	) ([]*Reservation, string, error)
	DescribeReservation(reservationID string) (*Reservation, error)
	DeleteReservation(reservationID string) (*Reservation, error)
	UpdateReservation(
		reservationID, name string,
		renewalSettings RenewalSettings,
		hasRenewalSettings bool,
	) (*Reservation, error)

	// Batch ops
	// BatchStart/BatchStop take only channel and multiplex IDs -- the real
	// BatchStartInput/BatchStopInput shapes have NO inputIds field (verified
	// against aws-sdk-go-v2/service/medialive's api_op_BatchStart.go /
	// api_op_BatchStop.go; only ChannelIds+MultiplexIds).
	BatchStart(channelIDs, multiplexIDs []string) (*BatchResult, error)
	BatchStop(channelIDs, multiplexIDs []string) (*BatchResult, error)
	// BatchDelete takes channel, input, multiplex, AND input-security-group
	// IDs -- BatchDeleteInput is the one Batch* shape with all four fields.
	BatchDelete(channelIDs, inputIDs, multiplexIDs, inputSecurityGroupIDs []string) (*BatchResult, error)
	BatchUpdateSchedule(
		channelID string,
		creates []ScheduleAction,
		deleteActionNames []string,
	) (*BatchUpdateScheduleResult, error)

	// Networks
	CreateNetwork(
		name string,
		ipPools []IPPool,
		routes []Route,
		tags map[string]string,
	) (*Network, error)
	DescribeNetwork(networkID string) (*Network, error)
	UpdateNetwork(networkID, name string, ipPools []IPPool, routes []Route) (*Network, error)
	DeleteNetwork(networkID string) (*Network, error)
	ListNetworks(maxResults int, nextToken string) ([]*Network, string, error)

	// SdiSources
	CreateSdiSource(name, sdiType, mode string, tags map[string]string) (*SdiSource, error)
	DescribeSdiSource(sdiSourceID string) (*SdiSource, error)
	UpdateSdiSource(sdiSourceID, name, sdiType, mode string) (*SdiSource, error)
	DeleteSdiSource(sdiSourceID string) (*SdiSource, error)
	ListSdiSources(maxResults int, nextToken string) ([]*SdiSource, string, error)

	// ChannelPlacementGroups (nested under a cluster)
	CreateChannelPlacementGroup(
		clusterID, name string,
		nodes []string,
		tags map[string]string,
	) (*ChannelPlacementGroup, error)
	DescribeChannelPlacementGroup(clusterID, groupID string) (*ChannelPlacementGroup, error)
	UpdateChannelPlacementGroup(
		clusterID, groupID, name string,
		nodes []string,
	) (*ChannelPlacementGroup, error)
	DeleteChannelPlacementGroup(clusterID, groupID string) (*ChannelPlacementGroup, error)
	ListChannelPlacementGroups(
		clusterID string,
		maxResults int,
		nextToken string,
	) ([]*ChannelPlacementGroup, string, error)

	// Account configuration
	DescribeAccountConfiguration() (*AccountConfiguration, error)
	UpdateAccountConfiguration(kmsKeyID string) (*AccountConfiguration, error)

	// Schedule
	DescribeSchedule(channelID string) ([]ScheduleAction, error)
	DeleteSchedule(channelID string) error

	// Alerts and versions
	ListAlerts(channelID string) ([]map[string]any, error)
	ListMultiplexAlerts(multiplexID string) ([]map[string]any, error)
	ListVersions() []ChannelEngineVersion

	// Channel lifecycle extras
	UpdateChannelClass(channelID, channelClass string) (*Channel, error)
	RestartChannelPipelines(channelID string, pipelineIDs []string) (*Channel, error)
	DescribeThumbnails(channelID string) (*Channel, error)

	// InputDevice lifecycle extras
	StartInputDevice(deviceID string) error
	StopInputDevice(deviceID string) error
	StartInputDeviceMaintenanceWindow(deviceID string) error
	DescribeInputDeviceThumbnail(deviceID string) (*InputDevice, error)

	// SignalMap monitor deployment teardown
	StartDeleteMonitorDeployment(identifier string) (*SignalMap, error)

	// Partner inputs
	CreatePartnerInput(inputID string, tags map[string]string) (*Input, error)

	AccountID() string
	Region() string
	Reset()
	Snapshot(ctx context.Context) []byte
	Restore(ctx context.Context, data []byte) error
}

// IPPool is a CIDR pool for a Network.
type IPPool struct {
	Cidr string `json:"cidr"`
}

// Route is a static route for a Network.
type Route struct {
	Cidr    string `json:"cidr"`
	Gateway string `json:"gateway"`
}

// Network represents a MediaLive Anywhere network resource.
type Network struct {
	Tags                 map[string]string
	ARN                  string
	ID                   string
	Name                 string
	State                string
	AssociatedClusterIDs []string
	IPPools              []IPPool
	Routes               []Route
}

// SdiSource represents a MediaLive SDI source resource.
type SdiSource struct {
	ARN    string
	ID     string
	Name   string
	Type   string
	Mode   string
	State  string
	Inputs []string
}

// ChannelPlacementGroup represents a placement group within a cluster.
type ChannelPlacementGroup struct {
	Tags      map[string]string
	ARN       string
	ID        string
	Name      string
	ClusterID string
	State     string
	Channels  []string
	Nodes     []string
}

// AccountConfiguration holds account-wide MediaLive settings.
type AccountConfiguration struct {
	KmsKeyID string
}

// ChannelEngineVersion is an available channel engine version.
type ChannelEngineVersion struct {
	ExpirationDate string
	Version        string
}

// ChannelAnywhereSettings holds the MediaLive Anywhere Cluster/
// ChannelPlacementGroup association for a Channel. Wire keys
// (anywhereSettings.clusterId/channelPlacementGroupId) verified against
// aws-sdk-go-v2/service/medialive's types.AnywhereSettings. Before this fix,
// gopherstack didn't track this at all: CreateChannel/UpdateChannel
// silently dropped a caller's anywhereSettings, and Cluster.ChannelIds/
// ChannelPlacementGroup.Channels/Node.ChannelPlacementGroups (all real wire
// fields) had nothing to derive from and were hardcoded to empty lists.
type ChannelAnywhereSettings struct {
	ClusterID               string
	ChannelPlacementGroupID string
}

// hasAnywhereSettings reports whether s has any real content, so callers can
// omit an empty "anywhereSettings" key the same way a real, non-Anywhere
// Channel omits it entirely.
func (s ChannelAnywhereSettings) hasAnywhereSettings() bool {
	return s.ClusterID != "" || s.ChannelPlacementGroupID != ""
}

// --- Channel extended settings (gopherstack-jb9i) ---
//
// CreateChannelInput/UpdateChannelInput have 17 top-level members (verified
// against aws-sdk-go-v2/service/medialive@v1.101.4's api_op_CreateChannel.go/
// api_op_UpdateChannel.go); before this fix gopherstack modeled 5
// (name/channelClass/roleArn/tags/anywhereSettings). The 12 added here are
// CdiInputSpecification/ChannelEngineVersion/ChannelSecurityGroups/
// Destinations/EncoderSettings/InferenceSettings/InputAttachments/
// InputSpecification/LinkedChannelSettings/LogLevel/Maintenance/Vpc.
// EncoderSettings is the deepest: it fans out into per-codec union types
// (AudioCodecSettings/VideoCodecSettings, each ~20 variants) and
// per-output-technology union types (OutputGroupSettings/OutputSettings,
// each ~10-13 variants) that are genuinely impractical to hand-model in
// full -- every EncoderSettings sub-type below documents exactly what is
// and is not modeled; see also PARITY.md's gaps entry.

// CdiInputSpecification specifies the maximum CDI input resolution for a
// channel. Wire key "cdiInputSpecification.resolution" -- verified against
// types.CdiInputSpecification.
type CdiInputSpecification struct {
	Resolution string
}

func (s CdiInputSpecification) hasCdiInputSpecification() bool { return s.Resolution != "" }

// InputSpecification describes the class of network/file inputs a channel
// expects. Wire keys "inputSpecification.codec/maximumBitrate/resolution" --
// verified against types.InputSpecification.
type InputSpecification struct {
	Codec          string
	MaximumBitrate string
	Resolution     string
}

func (s InputSpecification) hasInputSpecification() bool {
	return s.Codec != "" || s.MaximumBitrate != "" || s.Resolution != ""
}

// ChannelVpcSettings holds the caller-supplied VPC output configuration
// (request shape types.VpcOutputSettings: subnetIds/publicAddressAllocationIds/
// securityGroupIds). The response shape (types.VpcOutputSettingsDescription)
// additionally reports availabilityZones/networkInterfaceIds -- values
// MediaLive computes from a real VPC/ENI integration that gopherstack does
// not have; left omitted on output rather than fabricated (same convention
// as ChannelEngineVersion's ExpirationDate below).
type ChannelVpcSettings struct {
	SubnetIDs                  []string
	PublicAddressAllocationIDs []string
	SecurityGroupIDs           []string
}

func (v ChannelVpcSettings) hasVpc() bool { return len(v.SubnetIDs) > 0 }

// ChannelMaintenance holds a channel's maintenance window configuration.
// Create accepts day/startTime (types.MaintenanceCreateSettings); Update
// additionally accepts scheduledDate (types.MaintenanceUpdateSettings). The
// response shape (types.MaintenanceStatus) also reports a computed
// maintenanceDeadline that gopherstack has no scheduler to derive -- always
// omitted, never fabricated.
type ChannelMaintenance struct {
	Day           string
	StartTime     string
	ScheduledDate string
}

func (m ChannelMaintenance) hasMaintenance() bool {
	return m.Day != "" || m.StartTime != "" || m.ScheduledDate != ""
}

// AudioFeedInputMapping maps an audio selector in the channel to a feed
// input on an associated Elemental Inference feed. Wire keys
// "audioSelectorName"/"feedInput" -- verified against types.AudioFeedInput.
type AudioFeedInputMapping struct {
	AudioSelectorName string
	FeedInput         string
}

// ChannelInferenceSettings configures Elemental Inference features for a
// channel. Wire keys "audioFeedInputs"/"feedArn" -- verified against
// types.InferenceSettings/types.DescribeInferenceSettings (identical shape
// on request and response).
type ChannelInferenceSettings struct {
	FeedArn         string
	AudioFeedInputs []AudioFeedInputMapping
}

func (s ChannelInferenceSettings) hasInferenceSettings() bool {
	return len(s.AudioFeedInputs) > 0 || s.FeedArn != ""
}

// ChannelFollowerSettings holds a follower channel's linked-channel
// configuration. Wire keys "linkedChannelType"/"primaryChannelArn" --
// verified against types.FollowerChannelSettings (request). The response
// shape's primary side (types.DescribePrimaryChannelSettings) additionally
// reports "followingChannelArns", a value MediaLive derives from every
// OTHER channel's follower settings; gopherstack derives it the same way at
// read time (see followingChannelArns in channels.go) rather than storing
// it -- see ChannelPrimarySettings below.
type ChannelFollowerSettings struct {
	LinkedChannelType string
	PrimaryChannelArn string
}

// ChannelPrimarySettings is a primary channel's linked-channel settings.
// FollowingChannelArns is derived (never accepted on a request -- extraction
// never populates it), same pattern as Cluster.ChannelIDs/
// ChannelPlacementGroup.Channels: computed at read time by
// followingChannelArns (channels.go) and stamped onto the returned domain
// object right before Describe/Create/Update/List/Start/Stop hand it back.
type ChannelPrimarySettings struct {
	LinkedChannelType    string
	FollowingChannelArns []string
}

// ChannelLinkedChannelSettings is a channel's linked-channel configuration
// (types.LinkedChannelSettings). Only one of Follower/Primary is normally
// set, matching AWS: a channel is either a follower or a primary, never
// both.
type ChannelLinkedChannelSettings struct {
	Follower ChannelFollowerSettings
	Primary  ChannelPrimarySettings
}

func (s ChannelLinkedChannelSettings) hasLinkedChannelSettings() bool {
	return s.Follower.LinkedChannelType != "" || s.Follower.PrimaryChannelArn != "" ||
		s.Primary.LinkedChannelType != ""
}

// OutputDestinationSetting is one standard-output destination endpoint
// (RTMP/etc). Wire keys "passwordParam"/"streamName"/"url"/"username" --
// verified against types.OutputDestinationSettings.
type OutputDestinationSetting struct {
	PasswordParam string
	StreamName    string
	URL           string
	Username      string
}

// MediaPackageDestinationSettings targets a MediaPackage channel. Wire keys
// "channelEndpointId"/"channelGroup"/"channelId"/"channelName"/
// "mediaPackageRegionName" -- verified against
// types.MediaPackageOutputDestinationSettings.
type MediaPackageDestinationSettings struct {
	ChannelEndpointID      string
	ChannelGroup           string
	ChannelID              string
	ChannelName            string
	MediaPackageRegionName string
}

// MultiplexDestinationSettings targets a Multiplex program. Wire keys
// "multiplexId"/"programName" -- verified against
// types.MultiplexProgramChannelDestinationSettings.
type MultiplexDestinationSettings struct {
	MultiplexID string
	ProgramName string
}

// MediaConnectRouterDestinationSettings targets a MediaConnect Router
// output. Wire keys "encryptionType"/"secretArn" -- verified against
// types.MediaConnectRouterOutputDestinationSettings.
type MediaConnectRouterDestinationSettings struct {
	EncryptionType string
	SecretArn      string
}

// SrtDestinationSettings targets an SRT output. Wire keys
// "connectionMode"/"encryptionPassphraseSecretArn"/"listenerPort"/
// "streamId"/"url" -- verified against types.SrtOutputDestinationSettings.
type SrtDestinationSettings struct {
	ConnectionMode                string
	EncryptionPassphraseSecretArn string
	StreamID                      string
	URL                           string
	ListenerPort                  int32
}

// ChannelOutputDestination is one entry of EncoderSettings' output routing
// table (types.OutputDestination). Modeled in full -- unlike EncoderSettings'
// per-output-group/per-output settings (see OutputGroup below), every
// OutputDestination member is itself a flat, small, non-recursive struct, so
// there is no "genuinely impractical union depth" carve-out needed here.
type ChannelOutputDestination struct {
	ID                         string
	LogicalInterfaceNames      []string
	MediaConnectRouterSettings []MediaConnectRouterDestinationSettings
	MediaPackageSettings       []MediaPackageDestinationSettings
	MultiplexSettings          *MultiplexDestinationSettings
	Settings                   []OutputDestinationSetting
	SrtSettings                []SrtDestinationSettings
}

// AudioSilenceFailoverSettings / InputLossFailoverSettings /
// VideoBlackFailoverSettings are the three named failover-condition variants
// (types.AudioSilenceFailoverSettings/types.InputLossFailoverSettings/
// types.VideoBlackFailoverSettings). Unlike the codec-settings unions
// (dozens of variants, each itself deep), this union has exactly 3 small
// flat variants, so it's modeled in full rather than treated as a gap.
type AudioSilenceFailoverSettings struct {
	AudioSelectorName         string
	AudioSilenceThresholdMsec int32
}

// InputLossFailoverSettings triggers failover after a period with no input
// detected.
type InputLossFailoverSettings struct {
	InputLossThresholdMsec int32
}

// VideoBlackFailoverSettings triggers failover after a period of black
// video.
type VideoBlackFailoverSettings struct {
	BlackDetectThreshold    float64
	VideoBlackThresholdMsec int32
}

// ChannelFailoverConditionSettings is the tagged union of failover-detection
// methods (types.FailoverConditionSettings); at most one variant is set.
type ChannelFailoverConditionSettings struct {
	AudioSilenceSettings *AudioSilenceFailoverSettings
	InputLossSettings    *InputLossFailoverSettings
	VideoBlackSettings   *VideoBlackFailoverSettings
}

// ChannelFailoverCondition wraps one FailoverConditionSettings entry
// (types.FailoverCondition -- a single-field wrapper struct on the real SDK
// too, kept here for wire-shape fidelity rather than flattened away).
type ChannelFailoverCondition struct {
	Settings ChannelFailoverConditionSettings
}

// ChannelAutomaticInputFailoverSettings configures automatic input failover
// for an InputAttachment. Wire keys "secondaryInputId"/"errorClearTimeMsec"/
// "failoverConditions"/"inputPreference" -- verified against
// types.AutomaticInputFailoverSettings.
type ChannelAutomaticInputFailoverSettings struct {
	SecondaryInputID   string
	InputPreference    string
	FailoverConditions []ChannelFailoverCondition
	ErrorClearTimeMsec int32
}

func (s ChannelAutomaticInputFailoverSettings) hasFailover() bool {
	return s.SecondaryInputID != ""
}

// ChannelInputAttachment attaches an Input to a Channel
// (types.InputAttachment). InputSettings (per-attachment audio/caption/video
// selector configuration) is modeled in full below (gopherstack-sthr).
type ChannelInputAttachment struct {
	InputSettings                  InputSettings
	InputAttachmentName            string
	InputID                        string
	LogicalInterfaceNames          []string
	AutomaticInputFailoverSettings ChannelAutomaticInputFailoverSettings
}

// AudioHlsRenditionSelection picks an audio rendition out of an HLS input's
// #EXT-X-MEDIA tags. Wire keys "groupId"/"name" -- verified against
// types.AudioHlsRenditionSelection.
type AudioHlsRenditionSelection struct {
	GroupID string
	Name    string
}

// AudioLanguageSelection picks an audio stream by its 3-letter language
// code. Wire keys "languageCode"/"languageSelectionPolicy" -- verified
// against types.AudioLanguageSelection.
type AudioLanguageSelection struct {
	LanguageCode            string
	LanguageSelectionPolicy string
}

// AudioDolbyEDecode configures Dolby E program extraction. Wire key
// "programSelection" -- verified against types.AudioDolbyEDecode.
type AudioDolbyEDecode struct {
	ProgramSelection string
}

// InputChannelLevel maps one input channel into an AudioChannelMapping's
// output channel with a gain adjustment. Wire keys "gain"/"inputChannel" --
// verified against types.InputChannelLevel.
type InputChannelLevel struct {
	Gain         int32
	InputChannel int32
}

// AudioChannelMapping is one output-channel entry of RemixSettings. Wire
// keys "inputChannelLevels"/"outputChannel" -- verified against
// types.AudioChannelMapping.
type AudioChannelMapping struct {
	InputChannelLevels []InputChannelLevel
	OutputChannel      int32
}

// RemixSettings controls fine-grained input-to-output audio channel
// remixing. Wire keys "channelMappings"/"channelsIn"/"channelsOut" --
// verified against types.RemixSettings.
type RemixSettings struct {
	ChannelMappings []AudioChannelMapping
	ChannelsIn      int32
	ChannelsOut     int32
}

// AudioNormalizationSettings configures loudness normalization. Wire keys
// "algorithm"/"algorithmControl"/"peakCalculation"/"peakLimiterThreshold"/
// "targetLkfs" -- verified against types.AudioNormalizationSettings.
type AudioNormalizationSettings struct {
	Algorithm            string
	AlgorithmControl     string
	PeakCalculation      string
	PeakLimiterThreshold float64
	TargetLkfs           float64
}

// AudioPreMixerSettings configures per-PID/per-track audio remixing before
// interleaving (types.AudioPreMixerSettings). Wire keys
// "audioNormalizationSettings"/"channels"/"gainDb"/"remixSettings".
type AudioPreMixerSettings struct {
	AudioNormalizationSettings *AudioNormalizationSettings
	RemixSettings              *RemixSettings
	GainDB                     float64
	Channels                   int32
}

// AudioPid is one PID entry of an AudioPidSelection. Wire keys
// "dolbyEDecode"/"pid"/"premixSettings" -- verified against types.AudioPid.
type AudioPid struct {
	DolbyEDecode   *AudioDolbyEDecode
	PremixSettings *AudioPreMixerSettings
	Pid            int32
}

// AudioPidSelection selects one or more audio PIDs from a source. Wire keys
// "pid"/"pids" -- verified against types.AudioPidSelection.
type AudioPidSelection struct {
	Pids []AudioPid
	Pid  int32
}

// AudioTrack is one track entry of an AudioTrackSelection. Wire keys
// "premixSettings"/"track" -- verified against types.AudioTrack.
type AudioTrack struct {
	PremixSettings *AudioPreMixerSettings
	Track          int32
}

// AudioTrackSelection selects one or more audio tracks from a source. Wire
// keys "dolbyEDecode"/"tracks" -- verified against types.AudioTrackSelection.
type AudioTrackSelection struct {
	DolbyEDecode *AudioDolbyEDecode
	Tracks       []AudioTrack
}

// AudioSelectorSettings is the tagged union of audio-selection methods
// (types.AudioSelectorSettings); at most one variant is set.
type AudioSelectorSettings struct {
	AudioHlsRenditionSelection *AudioHlsRenditionSelection
	AudioLanguageSelection     *AudioLanguageSelection
	AudioPidSelection          *AudioPidSelection
	AudioTrackSelection        *AudioTrackSelection
}

// AudioSelector names one audio selector an input exposes for
// AudioDescriptions to reference. Wire keys "name"/"selectorSettings" --
// verified against types.AudioSelector.
type AudioSelector struct {
	SelectorSettings AudioSelectorSettings
	Name             string
}

// AncillarySourceSettings extracts captions from an ancillary data channel.
// Wire key "sourceAncillaryChannelNumber" -- verified against
// types.AncillarySourceSettings.
type AncillarySourceSettings struct {
	SourceAncillaryChannelNumber int32
}

// DvbSubSourceSettings extracts DVB-Sub captions. Wire keys
// "ocrLanguage"/"pid" -- verified against types.DvbSubSourceSettings.
type DvbSubSourceSettings struct {
	OcrLanguage string
	Pid         int32
}

// EmbeddedSourceSettings extracts embedded 608/708 captions. Wire keys
// "convert608To708"/"scte20Detection"/"source608ChannelNumber"/
// "source608TrackNumber" -- verified against types.EmbeddedSourceSettings.
type EmbeddedSourceSettings struct {
	Convert608To708        string
	Scte20Detection        string
	Source608ChannelNumber int32
	Source608TrackNumber   int32
}

// Scte20SourceSettings extracts SCTE-20 captions. Wire keys
// "convert608To708"/"source608ChannelNumber" -- verified against
// types.Scte20SourceSettings.
type Scte20SourceSettings struct {
	Convert608To708        string
	Source608ChannelNumber int32
}

// Scte27SourceSettings extracts SCTE-27 captions. Wire keys
// "ocrLanguage"/"pid" -- verified against types.Scte27SourceSettings.
type Scte27SourceSettings struct {
	OcrLanguage string
	Pid         int32
}

// SmartSubtitleSourceSettings extracts Elemental-Inference-generated
// subtitles. Wire keys "captionSynchronizationMode"/"inferenceFeedOutput" --
// verified against types.SmartSubtitleSourceSettings.
type SmartSubtitleSourceSettings struct {
	CaptionSynchronizationMode string
	InferenceFeedOutput        string
}

// CaptionRectangle positions a TTML/EBU-TT-D caption display region as
// frame-relative percentages. Wire keys "height"/"leftOffset"/"topOffset"/
// "width" -- verified against types.CaptionRectangle.
type CaptionRectangle struct {
	Height     float64
	LeftOffset float64
	TopOffset  float64
	Width      float64
}

// TeletextSourceSettings extracts Teletext captions. Wire keys
// "outputRectangle"/"pageNumber" -- verified against
// types.TeletextSourceSettings.
type TeletextSourceSettings struct {
	OutputRectangle *CaptionRectangle
	PageNumber      string
}

// CaptionSelectorSettings is the tagged union of caption-source formats
// (types.CaptionSelectorSettings); at most one variant is set.
// AribSourceSettings (types.AribSourceSettings) has no fields on the real
// wire, so a bool records "this variant is set" -- same convention as
// ChannelMotionGraphicsSettings.HTMLMotionGraphicsSettings.
type CaptionSelectorSettings struct {
	AncillarySourceSettings     *AncillarySourceSettings
	DvbSubSourceSettings        *DvbSubSourceSettings
	EmbeddedSourceSettings      *EmbeddedSourceSettings
	Scte20SourceSettings        *Scte20SourceSettings
	Scte27SourceSettings        *Scte27SourceSettings
	SmartSubtitleSourceSettings *SmartSubtitleSourceSettings
	TeletextSourceSettings      *TeletextSourceSettings
	AribSourceSettings          bool
}

// CaptionSelector names one caption selector an input exposes for
// CaptionDescriptions to reference. Wire keys "languageCode"/"name"/
// "selectorSettings" -- verified against types.CaptionSelector.
type CaptionSelector struct {
	Name             string
	LanguageCode     string
	SelectorSettings CaptionSelectorSettings
}

// Hdr10Settings supplies HDR10 color-space metadata missing from an AWS
// Elemental Link source. Wire keys "maxCll"/"maxFall" -- verified against
// types.Hdr10Settings.
type Hdr10Settings struct {
	MaxCll  int32
	MaxFall int32
}

// VideoSelectorColorSpaceSettings is the tagged union of color-space
// metadata sources (types.VideoSelectorColorSpaceSettings). The real SDK
// currently defines exactly one variant, Hdr10Settings.
type VideoSelectorColorSpaceSettings struct {
	Hdr10Settings *Hdr10Settings
}

// VideoSelectorPid selects a video PID from a source. Wire key "pid" --
// verified against types.VideoSelectorPid.
type VideoSelectorPid struct {
	Pid int32
}

// VideoSelectorProgramID selects a program from a multi-program transport
// stream. Wire key "programId" -- verified against
// types.VideoSelectorProgramId.
type VideoSelectorProgramID struct {
	ProgramID int32
}

// VideoSelectorSettings is the tagged union of video-selection methods
// (types.VideoSelectorSettings); at most one variant is set.
type VideoSelectorSettings struct {
	VideoSelectorPid       *VideoSelectorPid
	VideoSelectorProgramID *VideoSelectorProgramID
}

// VideoSelector configures which video elementary stream an input decodes
// and how its color-space metadata is handled. Wire keys "colorSpace"/
// "colorSpaceSettings"/"colorSpaceUsage"/"selectorSettings" -- verified
// against types.VideoSelector.
type VideoSelector struct {
	SelectorSettings   VideoSelectorSettings
	ColorSpaceSettings VideoSelectorColorSpaceSettings
	ColorSpace         string
	ColorSpaceUsage    string
}

// HlsInputSettings configures HLS-manifest-specific input behavior. Wire
// keys "bandwidth"/"bufferSegments"/"retries"/"retryInterval"/
// "scte35Source" -- verified against types.HlsInputSettings.
type HlsInputSettings struct {
	Scte35Source   string
	Bandwidth      int32
	BufferSegments int32
	Retries        int32
	RetryInterval  int32
}

// MulticastInputSettings restricts a multicast input to a specific source
// IP (Source-Specific Multicast). Wire key "sourceIpAddress" -- verified
// against types.MulticastInputSettings.
type MulticastInputSettings struct {
	SourceIPAddress string
}

// NetworkInputSettings configures HLS/multicast-specific input behavior and
// TLS certificate validation (types.NetworkInputSettings). Wire keys
// "hlsInputSettings"/"multicastInputSettings"/"serverValidation".
type NetworkInputSettings struct {
	HlsInputSettings       *HlsInputSettings
	MulticastInputSettings *MulticastInputSettings
	ServerValidation       string
}

// InputSettings configures per-attachment audio/caption/video selectors and
// filtering for a ChannelInputAttachment (types.InputSettings). Wire keys
// "audioSelectors"/"captionSelectors"/"deblockFilter"/"denoiseFilter"/
// "filterStrength"/"inputFilter"/"networkInputSettings"/"scte35Pid"/
// "smpte2038DataPreference"/"sourceEndBehavior"/"videoSelector" -- verified
// against types.InputSettings.
type InputSettings struct {
	VideoSelector           VideoSelector
	NetworkInputSettings    NetworkInputSettings
	DeblockFilter           string
	DenoiseFilter           string
	InputFilter             string
	Smpte2038DataPreference string
	SourceEndBehavior       string
	AudioSelectors          []AudioSelector
	CaptionSelectors        []CaptionSelector
	FilterStrength          int32
	Scte35Pid               int32
}

// InputLocation is a URI plus optional Parameter-Store-backed credentials,
// reused by several EncoderSettings sub-shapes (avail/blackout slate
// images, input-loss slate). Wire keys "uri"/"passwordParam"/"username" --
// verified against types.InputLocation.
type InputLocation struct {
	URI           string
	PasswordParam string
	Username      string
}

// TimecodeConfig configures how EncoderSettings acquires/adjusts source
// timecodes. Wire keys "source"/"syncThreshold" -- verified against
// types.TimecodeConfig.
type TimecodeConfig struct {
	Source        string
	SyncThreshold int32
}

// AvailBlanking configures ad-avail blanking. Wire keys
// "availBlankingImage"/"state" -- verified against types.AvailBlanking.
type AvailBlanking struct {
	AvailBlankingImage InputLocation
	State              string
}

// BlackoutSlate configures SCTE-104/35 network-blackout behavior. Wire keys
// "blackoutSlateImage"/"networkEndBlackout"/"networkEndBlackoutImage"/
// "networkId"/"state" -- verified against types.BlackoutSlate.
type BlackoutSlate struct {
	BlackoutSlateImage      InputLocation
	NetworkEndBlackoutImage InputLocation
	NetworkEndBlackout      string
	NetworkID               string
	State                   string
}

// FeatureActivations toggles opt-in encoder features. Wire keys
// "inputPrepareScheduleActions"/"outputStaticImageOverlayScheduleActions" --
// verified against types.FeatureActivations.
type FeatureActivations struct {
	InputPrepareScheduleActions             string
	OutputStaticImageOverlayScheduleActions string
}

// InputLossBehavior configures encoder behavior when the input signal is
// lost. Wire keys "blackFrameMsec"/"inputLossImageColor"/
// "inputLossImageSlate"/"inputLossImageType"/"repeatFrameMsec" -- verified
// against types.InputLossBehavior.
type InputLossBehavior struct {
	InputLossImageSlate InputLocation
	InputLossImageColor string
	InputLossImageType  string
	BlackFrameMsec      int32
	RepeatFrameMsec     int32
}

// DisabledLockingSettings / EpochLockingSettings / PipelineLockingSettings
// are the three OutputLockingSettings variants (types.DisabledLockingSettings/
// types.EpochLockingSettings/types.PipelineLockingSettings) -- a 3-way union
// of small flat structs, modeled in full like the failover-condition union
// above.
type DisabledLockingSettings struct {
	CustomEpoch string
}

// EpochLockingSettings locks pipeline output to the Unix epoch (optionally a
// custom one).
type EpochLockingSettings struct {
	CustomEpoch string
	JamSyncTime string
}

// PipelineLockingSettings locks pipeline output to each other.
type PipelineLockingSettings struct {
	CustomEpoch           string
	PipelineLockingMethod string
}

// OutputLockingSettings is the tagged union of pipeline-locking strategies
// (types.OutputLockingSettings); at most one variant is set.
type OutputLockingSettings struct {
	Disabled *DisabledLockingSettings
	Epoch    *EpochLockingSettings
	Pipeline *PipelineLockingSettings
}

// GlobalConfiguration holds event-wide encoder settings. Wire keys
// "initialAudioGain"/"inputEndAction"/"inputLossBehavior"/
// "outputLockingMode"/"outputLockingSettings"/"outputTimingSource"/
// "supportLowFramerateInputs" -- verified against types.GlobalConfiguration.
type GlobalConfiguration struct {
	OutputLockingSettings     OutputLockingSettings
	InputEndAction            string
	OutputLockingMode         string
	OutputTimingSource        string
	SupportLowFramerateInputs string
	InputLossBehavior         InputLossBehavior
	InitialAudioGain          int32
}

// ThumbnailConfiguration enables/disables per-pipeline thumbnail generation.
// Wire key "state" -- verified against types.ThumbnailConfiguration.
type ThumbnailConfiguration struct {
	State string
}

// AacSettings configures AAC audio encoding. Wire keys "bitrate"/
// "codingMode"/"inputType"/"profile"/"rateControlMode"/"rawFormat"/
// "sampleRate"/"spec"/"vbrQuality" -- verified against types.AacSettings.
type AacSettings struct {
	CodingMode      string
	InputType       string
	Profile         string
	RateControlMode string
	RawFormat       string
	Spec            string
	VbrQuality      string
	Bitrate         float64
	SampleRate      float64
}

// Ac3Settings configures AC3 (Dolby Digital) audio encoding. Wire keys
// "attenuationControl"/"bitrate"/"bitstreamMode"/"codingMode"/"dialnorm"/
// "drcProfile"/"lfeFilter"/"metadataControl" -- verified against
// types.Ac3Settings.
type Ac3Settings struct {
	AttenuationControl string
	BitstreamMode      string
	CodingMode         string
	DrcProfile         string
	LfeFilter          string
	MetadataControl    string
	Bitrate            float64
	Dialnorm           int32
}

// Eac3AtmosSettings configures Dolby Digital Plus with Dolby Atmos audio
// encoding. Wire keys "bitrate"/"codingMode"/"dialnorm"/"drcLine"/"drcRf"/
// "heightTrim"/"surroundTrim" -- verified against types.Eac3AtmosSettings.
type Eac3AtmosSettings struct {
	CodingMode   string
	DrcLine      string
	DrcRf        string
	Bitrate      float64
	HeightTrim   float64
	SurroundTrim float64
	Dialnorm     int32
}

// Eac3Settings configures Dolby Digital Plus audio encoding
// (types.Eac3Settings). Wire keys "attenuationControl"/"bitrate"/
// "bitstreamMode"/"codingMode"/"dcFilter"/"dialnorm"/"drcLine"/"drcRf"/
// "lfeControl"/"lfeFilter"/"loRoCenterMixLevel"/"loRoSurroundMixLevel"/
// "ltRtCenterMixLevel"/"ltRtSurroundMixLevel"/"metadataControl"/
// "passthroughControl"/"phaseControl"/"stereoDownmix"/"surroundExMode"/
// "surroundMode".
type Eac3Settings struct {
	AttenuationControl   string
	BitstreamMode        string
	CodingMode           string
	DcFilter             string
	DrcLine              string
	DrcRf                string
	LfeControl           string
	LfeFilter            string
	MetadataControl      string
	PassthroughControl   string
	PhaseControl         string
	StereoDownmix        string
	SurroundExMode       string
	SurroundMode         string
	Bitrate              float64
	LoRoCenterMixLevel   float64
	LoRoSurroundMixLevel float64
	LtRtCenterMixLevel   float64
	LtRtSurroundMixLevel float64
	Dialnorm             int32
}

// Mp2Settings configures MPEG-1 Layer 2 audio encoding. Wire keys
// "bitrate"/"codingMode"/"sampleRate" -- verified against types.Mp2Settings.
type Mp2Settings struct {
	CodingMode string
	Bitrate    float64
	SampleRate float64
}

// WavSettings configures WAV audio encoding. Wire keys "bitDepth"/
// "codingMode"/"sampleRate" -- verified against types.WavSettings.
type WavSettings struct {
	CodingMode string
	BitDepth   float64
	SampleRate float64
}

// AudioCodecSettings is the tagged union of audio codecs
// (types.AudioCodecSettings); at most one variant is set.
// PassThroughSettings (types.PassThroughSettings) has no fields on the real
// wire, so a bool records "this variant is set" -- same convention as
// ChannelMotionGraphicsSettings.HTMLMotionGraphicsSettings.
type AudioCodecSettings struct {
	AacSettings         *AacSettings
	Ac3Settings         *Ac3Settings
	Eac3AtmosSettings   *Eac3AtmosSettings
	Eac3Settings        *Eac3Settings
	Mp2Settings         *Mp2Settings
	WavSettings         *WavSettings
	PassThroughSettings bool
}

// NielsenCBET configures Nielsen CBET watermarking. Wire keys
// "cbetCheckDigitString"/"cbetStepaside"/"csid" -- verified against
// types.NielsenCBET.
type NielsenCBET struct {
	CbetCheckDigitString string
	CbetStepaside        string
	Csid                 string
}

// NielsenNaesIiNw configures Nielsen NAES II / NW watermarking. Wire keys
// "checkDigitString"/"sid"/"timezone" -- verified against
// types.NielsenNaesIiNw.
type NielsenNaesIiNw struct {
	CheckDigitString string
	Timezone         string
	Sid              float64
}

// NielsenWatermarksSettings configures which Nielsen watermark(s) to embed
// (types.NielsenWatermarksSettings). Wire keys "nielsenCbetSettings"/
// "nielsenDistributionType"/"nielsenNaesIiNwSettings".
type NielsenWatermarksSettings struct {
	NielsenCbetSettings     *NielsenCBET
	NielsenNaesIiNwSettings *NielsenNaesIiNw
	NielsenDistributionType string
}

// AudioWatermarkSettings configures audio watermarking solutions
// (types.AudioWatermarkSettings). Wire key "nielsenWatermarksSettings".
type AudioWatermarkSettings struct {
	NielsenWatermarksSettings *NielsenWatermarksSettings
}

// AudioDescription names one audio encode derived from an input audio
// selector. Wire keys "audioDashRoles"/"audioNormalizationSettings"/
// "audioSelectorName"/"audioType"/"audioTypeControl"/
// "audioWatermarkingSettings"/"codecSettings"/"dvbDashAccessibility"/
// "languageCode"/"languageCodeControl"/"name"/"remixSettings"/
// "streamName" -- verified against types.AudioDescription.
// AudioNormalizationSettings/RemixSettings reuse the same domain types
// InputSettings' AudioPreMixerSettings uses (types.AudioNormalizationSettings/
// types.RemixSettings are the same SDK shapes in both places).
type AudioDescription struct {
	AudioNormalizationSettings *AudioNormalizationSettings
	CodecSettings              *AudioCodecSettings
	AudioWatermarkingSettings  *AudioWatermarkSettings
	RemixSettings              *RemixSettings
	LanguageCodeControl        string
	AudioTypeControl           string
	StreamName                 string
	DvbDashAccessibility       string
	AudioType                  string
	Name                       string
	LanguageCode               string
	AudioSelectorName          string
	AudioDashRoles             []string
}

// TimecodeBurninSettings configures timecode burn-in
// (types.TimecodeBurninSettings, types.go:8411). Wire keys "fontSize"/
// "position"/"prefix". Shared by all five VideoCodecSettings variants.
type TimecodeBurninSettings struct {
	FontSize string
	Position string
	Prefix   string
}

// Av1ColorSpaceSettings is the tagged union of AV1 color space conversions
// (types.Av1ColorSpaceSettings, types.go:656); at most one variant is set.
// ColorSpacePassthroughSettings/Hlg2020Settings/Rec601Settings/
// Rec709Settings have no fields on the wire -- bools record "this variant is
// set", same convention as AudioCodecSettings.PassThroughSettings.
type Av1ColorSpaceSettings struct {
	Hdr10Settings                 *Hdr10Settings
	ColorSpacePassthroughSettings bool
	Hlg2020Settings               bool
	Rec601Settings                bool
	Rec709Settings                bool
}

// H264ColorSpaceSettings is the tagged union of H264 color space conversions
// (types.H264ColorSpaceSettings, types.go:3005); at most one variant is set.
type H264ColorSpaceSettings struct {
	ColorSpacePassthroughSettings bool
	Rec601Settings                bool
	Rec709Settings                bool
}

// H265ColorSpaceSettings is the tagged union of H265 color space conversions
// (types.H265ColorSpaceSettings, types.go:3282); at most one variant is set.
type H265ColorSpaceSettings struct {
	Hdr10Settings                 *Hdr10Settings
	ColorSpacePassthroughSettings bool
	DolbyVision81Settings         bool
	Hlg2020Settings               bool
	Rec601Settings                bool
	Rec709Settings                bool
}

// BandwidthReductionFilterSettings configures the bandwidth reduction video
// pre-filter (types.BandwidthReductionFilterSettings, types.go:871). Wire
// keys "postFilterSharpening"/"strength".
type BandwidthReductionFilterSettings struct {
	PostFilterSharpening string
	Strength             string
}

// TemporalFilterSettings configures the temporal video pre-filter
// (types.TemporalFilterSettings, types.go:8350). Wire keys
// "postFilterSharpening"/"strength".
type TemporalFilterSettings struct {
	PostFilterSharpening string
	Strength             string
}

// H264FilterSettings is the tagged union of H264 pre-encode video filters
// (types.H264FilterSettings, types.go:3020); at most one variant is set.
// H265FilterSettings (types.go:3306) has the identical field set, so both
// convert through one shared wire struct -- see toVideoFilterSettingsOutput.
type H264FilterSettings struct {
	BandwidthReductionFilterSettings *BandwidthReductionFilterSettings
	TemporalFilterSettings           *TemporalFilterSettings
}

// H265FilterSettings mirrors H264FilterSettings -- see its doc comment.
type H265FilterSettings struct {
	BandwidthReductionFilterSettings *BandwidthReductionFilterSettings
	TemporalFilterSettings           *TemporalFilterSettings
}

// Mpeg2FilterSettings wraps MPEG2's single pre-encode video filter
// (types.Mpeg2FilterSettings, types.go:5761). Wire key
// "temporalFilterSettings".
type Mpeg2FilterSettings struct {
	TemporalFilterSettings *TemporalFilterSettings
}

// Av1Settings configures the AV1 video codec (types.Av1Settings,
// types.go:677); 24 fields, verified field-by-field against the
// serializer. FramerateDenominator/FramerateNumerator are AWS-required but
// modeled the same as every other field, matching this file's convention.
type Av1Settings struct {
	ColorSpaceSettings     *Av1ColorSpaceSettings
	TimecodeBurninSettings *TimecodeBurninSettings
	AfdSignaling           string
	BitDepth               string
	FixedAfd               string
	GopSizeUnits           string
	Level                  string
	LookAheadRateControl   string
	RateControlMode        string
	SceneChangeDetect      string
	SpatialAq              string
	TemporalAq             string
	TimecodeInsertion      string
	GopSize                float64
	Bitrate                int32
	BufSize                int32
	FramerateDenominator   int32
	FramerateNumerator     int32
	MaxBitrate             int32
	MinBitrate             int32
	MinIInterval           int32
	ParDenominator         int32
	ParNumerator           int32
	QvbrQualityLevel       int32
}

// H264Settings configures the H264 video codec (types.H264Settings,
// types.go:3032); 44 fields, verified field-by-field against the
// serializer -- the largest single variant in the union.
type H264Settings struct {
	ColorSpaceSettings     *H264ColorSpaceSettings
	FilterSettings         *H264FilterSettings
	TimecodeBurninSettings *TimecodeBurninSettings
	AdaptiveQuantization   string
	AfdSignaling           string
	ColorMetadata          string
	EntropyEncoding        string
	FixedAfd               string
	FlickerAq              string
	ForceFieldPictures     string
	FramerateControl       string
	GopBReference          string
	GopSizeUnits           string
	Level                  string
	LookAheadRateControl   string
	ParControl             string
	Profile                string
	QualityLevel           string
	RateControlMode        string
	ScanType               string
	SceneChangeDetect      string
	SpatialAq              string
	SubgopLength           string
	Syntax                 string
	TemporalAq             string
	TimecodeInsertion      string
	GopSize                float64
	Bitrate                int32
	BufFillPct             int32
	BufSize                int32
	FramerateDenominator   int32
	FramerateNumerator     int32
	GopClosedCadence       int32
	GopNumBFrames          int32
	MaxBitrate             int32
	MinBitrate             int32
	MinIInterval           int32
	MinQp                  int32
	NumRefFrames           int32
	ParDenominator         int32
	ParNumerator           int32
	QvbrQualityLevel       int32
	Slices                 int32
	Softness               int32
}

// H265Settings configures the H265 video codec (types.H265Settings,
// types.go:3318); 42 fields, verified field-by-field against the
// serializer.
type H265Settings struct {
	ColorSpaceSettings          *H265ColorSpaceSettings
	FilterSettings              *H265FilterSettings
	TimecodeBurninSettings      *TimecodeBurninSettings
	AdaptiveQuantization        string
	AfdSignaling                string
	AlternativeTransferFunction string
	ColorMetadata               string
	Deblocking                  string
	FixedAfd                    string
	FlickerAq                   string
	GopBReference               string
	GopSizeUnits                string
	Level                       string
	LookAheadRateControl        string
	MvOverPictureBoundaries     string
	MvTemporalPredictor         string
	Profile                     string
	RateControlMode             string
	ScanType                    string
	SceneChangeDetect           string
	SubgopLength                string
	Tier                        string
	TilePadding                 string
	TimecodeInsertion           string
	TreeblockSize               string
	GopSize                     float64
	Bitrate                     int32
	BufSize                     int32
	FramerateDenominator        int32
	FramerateNumerator          int32
	GopClosedCadence            int32
	GopNumBFrames               int32
	MaxBitrate                  int32
	MinBitrate                  int32
	MinIInterval                int32
	MinQp                       int32
	ParDenominator              int32
	ParNumerator                int32
	QvbrQualityLevel            int32
	Slices                      int32
	TileHeight                  int32
	TileWidth                   int32
}

// Mpeg2Settings configures the MPEG2 video codec (types.Mpeg2Settings,
// types.go:5770); 17 fields, verified field-by-field against the
// serializer. Unlike the other four variants, MPEG2 has no ColorSpaceSettings
// sub-union -- ColorSpace is a plain enum here.
type Mpeg2Settings struct {
	FilterSettings         *Mpeg2FilterSettings
	TimecodeBurninSettings *TimecodeBurninSettings
	AdaptiveQuantization   string
	AfdSignaling           string
	ColorMetadata          string
	ColorSpace             string
	DisplayAspectRatio     string
	FixedAfd               string
	GopSizeUnits           string
	ScanType               string
	SubgopLength           string
	TimecodeInsertion      string
	GopSize                float64
	FramerateDenominator   int32
	FramerateNumerator     int32
	GopClosedCadence       int32
	GopNumBFrames          int32
}

// FrameCaptureSettings configures the frame-capture pseudo-codec
// (types.FrameCaptureSettings, types.go:2943); 3 fields, verified against
// the serializer.
type FrameCaptureSettings struct {
	TimecodeBurninSettings *TimecodeBurninSettings
	CaptureIntervalUnits   string
	CaptureInterval        int32
}

// VideoCodecSettings is the tagged union of video codecs
// (types.VideoCodecSettings, types.go:8582); at most one variant is set.
type VideoCodecSettings struct {
	Av1Settings          *Av1Settings
	FrameCaptureSettings *FrameCaptureSettings
	H264Settings         *H264Settings
	H265Settings         *H265Settings
	Mpeg2Settings        *Mpeg2Settings
}

// VideoDescription names one video encode. Wire keys "name"/"codecSettings"/
// "height"/"respondToAfd"/"scalingBehavior"/"sharpness"/"width" -- verified
// against types.VideoDescription. VideoPreprocessors is deliberately NOT
// modeled; see PARITY.md's gaps entry.
type VideoDescription struct {
	CodecSettings   *VideoCodecSettings
	Name            string
	RespondToAfd    string
	ScalingBehavior string
	Height          int32
	Width           int32
	Sharpness       int32
}

// BurnInDestinationSettings configures burned-in captions
// (types.BurnInDestinationSettings, types.go:998). Wire keys "alignment"/
// "backgroundColor"/"backgroundOpacity"/"font"/"fontColor"/"fontOpacity"/
// "fontResolution"/"fontSize"/"outlineColor"/"outlineSize"/"shadowColor"/
// "shadowOpacity"/"shadowXOffset"/"shadowYOffset"/"subtitleRows"/
// "teletextGridControl"/"xPosition"/"yPosition". Font reuses InputLocation.
type BurnInDestinationSettings struct {
	Font                InputLocation
	Alignment           string
	BackgroundColor     string
	FontColor           string
	FontSize            string
	OutlineColor        string
	ShadowColor         string
	SubtitleRows        string
	TeletextGridControl string
	BackgroundOpacity   int32
	FontOpacity         int32
	FontResolution      int32
	OutlineSize         int32
	ShadowOpacity       int32
	ShadowXOffset       int32
	ShadowYOffset       int32
	XPosition           int32
	YPosition           int32
}

// DvbSubDestinationSettings configures DVB-Sub captions
// (types.DvbSubDestinationSettings, types.go:2216) -- the same field set as
// BurnInDestinationSettings, under DVB-Sub-scoped enum types on the SDK
// side (both are plain strings here).
type DvbSubDestinationSettings struct {
	Font                InputLocation
	Alignment           string
	BackgroundColor     string
	FontColor           string
	FontSize            string
	OutlineColor        string
	ShadowColor         string
	SubtitleRows        string
	TeletextGridControl string
	BackgroundOpacity   int32
	FontOpacity         int32
	FontResolution      int32
	OutlineSize         int32
	ShadowOpacity       int32
	ShadowXOffset       int32
	ShadowYOffset       int32
	XPosition           int32
	YPosition           int32
}

// EbuTtDDestinationSettings configures EBU-TT-D captions
// (types.EbuTtDDestinationSettings, types.go:2469). Wire keys
// "copyrightHolder"/"defaultFontSize"/"defaultLineHeight"/"fillLineGap"/
// "fontFamily"/"styleControl".
type EbuTtDDestinationSettings struct {
	CopyrightHolder   string
	FontFamily        string
	FillLineGap       string
	StyleControl      string
	DefaultFontSize   int32
	DefaultLineHeight int32
}

// TtmlDestinationSettings configures TTML captions
// (types.TtmlDestinationSettings, types.go:8483). Wire key "styleControl".
type TtmlDestinationSettings struct {
	StyleControl string
}

// WebvttDestinationSettings configures WebVTT captions
// (types.WebvttDestinationSettings, types.go:8792). Wire key "styleControl".
type WebvttDestinationSettings struct {
	StyleControl string
}

// CaptionDestinationSettings is the tagged union of caption output formats
// (types.CaptionDestinationSettings, types.go:1151); at most one variant is
// set. Arib/Embedded/EmbeddedPlusScte20/RtmpCaptionInfo/
// Scte20PlusEmbedded/Scte27/SmpteTt/TeletextDestinationSettings have no
// fields on the real wire, so a bool records "this variant is set" -- same
// convention as AudioCodecSettings.PassThroughSettings.
type CaptionDestinationSettings struct {
	BurnInDestinationSettings             *BurnInDestinationSettings
	DvbSubDestinationSettings             *DvbSubDestinationSettings
	EbuTtDDestinationSettings             *EbuTtDDestinationSettings
	TtmlDestinationSettings               *TtmlDestinationSettings
	WebvttDestinationSettings             *WebvttDestinationSettings
	AribDestinationSettings               bool
	EmbeddedDestinationSettings           bool
	EmbeddedPlusScte20DestinationSettings bool
	RtmpCaptionInfoDestinationSettings    bool
	Scte20PlusEmbeddedDestinationSettings bool
	Scte27DestinationSettings             bool
	SmpteTtDestinationSettings            bool
	TeletextDestinationSettings           bool
}

// CaptionDescription names one caption output derived from an input caption
// selector. Wire keys "captionDashRoles"/"captionSelectorName"/"name"/
// "accessibility"/"destinationSettings"/"dvbDashAccessibility"/
// "languageCode"/"languageDescription" -- verified against
// types.CaptionDescription, types.go:1107.
type CaptionDescription struct {
	DestinationSettings  *CaptionDestinationSettings
	CaptionSelectorName  string
	Name                 string
	Accessibility        string
	DvbDashAccessibility string
	LanguageCode         string
	LanguageDescription  string
	CaptionDashRoles     []string
}

// OutputLocationRef references an OutputDestination by ID (types.
// OutputLocationRef, types.go:6803). Wire key "destinationRefId".
type OutputLocationRef struct {
	DestinationRefID string
}

// OutputAdditionalDestination is an extra HTTP destination for a CMAF
// Ingest/MediaPackage V2 output group (types.AdditionalDestinations,
// types.go:104, and the identically-shaped types.
// MediaPackageAdditionalDestinations, types.go:5491 -- reused for both).
// Wire key "destination".
type OutputAdditionalDestination struct {
	Destination *OutputLocationRef
}

// CaptionLanguageMapping maps one embedded-captions channel to a language
// (types.CaptionLanguageMapping, types.go:1197; shared by HlsGroupSettings
// and MediaPackageV2GroupSettings). Wire keys "captionChannel"/
// "languageCode"/"languageDescription".
type CaptionLanguageMapping struct {
	LanguageCode        string
	LanguageDescription string
	CaptionChannel      int32
}

// CmafIngestCaptionLanguageMapping maps one embedded-captions channel to a
// language for CMAF Ingest output groups (types.
// CmafIngestCaptionLanguageMapping, types.go:1778). Wire keys
// "captionChannel"/"languageCode".
type CmafIngestCaptionLanguageMapping struct {
	LanguageCode   string
	CaptionChannel int32
}

// DvbNitSettings configures the DVB Network Information Table (types.
// DvbNitSettings, types.go:2168). Wire keys "networkId"/"networkName"/
// "repInterval".
type DvbNitSettings struct {
	NetworkName string
	NetworkID   int32
	RepInterval int32
}

// DvbSdtSettings configures the DVB Service Description Table (types.
// DvbSdtSettings, types.go:2189). Wire keys "outputSdt"/"repInterval"/
// "serviceName"/"serviceProviderName".
type DvbSdtSettings struct {
	OutputSdt           string
	ServiceName         string
	ServiceProviderName string
	RepInterval         int32
}

// DvbTdtSettings configures the DVB Time and Date Table (types.
// DvbTdtSettings, types.go:2348). Wire key "repInterval".
type DvbTdtSettings struct {
	RepInterval int32
}

// M2tsSettings configures MPEG-2 Transport Stream container packaging,
// shared by ArchiveContainerSettings/MediaConnectRouterContainerSettings/
// UDPContainerSettings (types.M2tsSettings, types.go:5016).
//
//nolint:dupl // mirrors m2tsSettingsOutput field-for-field by design; the pointer fields' pointee types differ
type M2tsSettings struct {
	DvbNitSettings                  *DvbNitSettings
	DvbSdtSettings                  *DvbSdtSettings
	DvbTdtSettings                  *DvbTdtSettings
	AbsentInputAudioBehavior        string
	Arib                            string
	AribCaptionsPid                 string
	AribCaptionsPidControl          string
	AudioBufferModel                string
	AudioPids                       string
	AudioStreamType                 string
	BufferModel                     string
	CcDescriptor                    string
	DvbSubPids                      string
	DvbTeletextPid                  string
	Ebif                            string
	EbpAudioInterval                string
	EbpPlacement                    string
	EcmPid                          string
	EsRateInPes                     string
	EtvPlatformPid                  string
	EtvSignalPid                    string
	Klv                             string
	KlvDataPids                     string
	NielsenID3Behavior              string
	PcrControl                      string
	PcrPid                          string
	PmtPid                          string
	RateMode                        string
	Scte27Pids                      string
	Scte35Control                   string
	Scte35Pid                       string
	SegmentationMarkers             string
	SegmentationStyle               string
	TimedMetadataBehavior           string
	TimedMetadataPid                string
	VideoPid                        string
	FragmentTime                    float64
	NullPacketBitrate               float64
	Scte35PrerollPullupMilliseconds float64
	SegmentationTime                float64
	AudioFramesPerPes               int32
	Bitrate                         int32
	EbpLookaheadMs                  int32
	PatInterval                     int32
	PcrPeriod                       int32
	PmtInterval                     int32
	ProgramNum                      int32
	TransportStreamID               int32
}

// ArchiveContainerSettings is the tagged union of Archive output container
// formats (types.ArchiveContainerSettings, types.go:149). The Raw variant
// (types.RawSettings, types.go:6950) has no fields on the wire, so a bool
// records "this variant is set" -- same convention as PassThroughSettings.
type ArchiveContainerSettings struct {
	M2tsSettings *M2tsSettings
	RawSettings  bool
}

// ArchiveS3Settings configures S3 delivery for Archive outputs (types.
// ArchiveS3Settings, types.go:198). Wire key "cannedAcl".
type ArchiveS3Settings struct {
	CannedACL string
}

// ArchiveCdnSettings configures CDN interaction for an Archive output group
// (types.ArchiveCdnSettings, types.go:140). Wire key "archiveS3Settings".
type ArchiveCdnSettings struct {
	ArchiveS3Settings *ArchiveS3Settings
}

// ArchiveOutputSettings configures one Archive output (types.
// ArchiveOutputSettings, types.go:179). Wire keys "containerSettings"/
// "extension"/"nameModifier".
type ArchiveOutputSettings struct {
	ContainerSettings *ArchiveContainerSettings
	Extension         string
	NameModifier      string
}

// FrameCaptureS3Settings configures S3 delivery for Frame Capture outputs
// (types.FrameCaptureS3Settings, types.go:2934). Wire key "cannedAcl".
type FrameCaptureS3Settings struct {
	CannedACL string
}

// FrameCaptureCdnSettings configures CDN interaction for a Frame Capture
// output group (types.FrameCaptureCdnSettings, types.go:2889). Wire key
// "frameCaptureS3Settings".
type FrameCaptureCdnSettings struct {
	FrameCaptureS3Settings *FrameCaptureS3Settings
}

// FrameCaptureOutputSettings configures one Frame Capture output (types.
// FrameCaptureOutputSettings, types.go:2924). Wire key "nameModifier".
type FrameCaptureOutputSettings struct {
	NameModifier string
}

// CmafIngestOutputSettings configures one CMAF Ingest output (types.
// CmafIngestOutputSettings, types.go:1882). Wire key "nameModifier".
type CmafIngestOutputSettings struct {
	NameModifier string
}

// M3u8Settings configures the .m3u8 container for a Standard HLS output
// (types.M3u8Settings, types.go:5261).
type M3u8Settings struct {
	AudioPids             string
	EcmPid                string
	KlvBehavior           string
	KlvDataPids           string
	NielsenID3Behavior    string
	PcrControl            string
	PcrPid                string
	PmtPid                string
	Scte35Behavior        string
	Scte35Pid             string
	TimedMetadataBehavior string
	TimedMetadataPid      string
	AudioFramesPerPes     int32
	PatInterval           int32
	PcrPeriod             int32
	PmtInterval           int32
	ProgramNum            int32
	TransportStreamID     int32
}

// AudioOnlyHlsSettings configures an audio-only HLS output (types.
// AudioOnlyHlsSettings, types.go:438). Wire keys "audioGroupId"/
// "audioOnlyImage"/"audioTrackType"/"segmentType".
type AudioOnlyHlsSettings struct {
	AudioOnlyImage InputLocation
	AudioGroupID   string
	AudioTrackType string
	SegmentType    string
}

// Fmp4HlsSettings configures a fragmented-MP4 HLS output (types.
// Fmp4HlsSettings, types.go:2840). Wire keys "audioRenditionSets"/
// "nielsenId3Behavior"/"timedMetadataBehavior".
type Fmp4HlsSettings struct {
	AudioRenditionSets    string
	NielsenID3Behavior    string
	TimedMetadataBehavior string
}

// StandardHlsSettings configures a standard (MPEG-TS) HLS output (types.
// StandardHlsSettings, types.go:8109). Wire keys "audioRenditionSets"/
// "m3u8Settings".
type StandardHlsSettings struct {
	M3u8Settings       *M3u8Settings
	AudioRenditionSets string
}

// HlsSettings is the tagged union of per-output-type HLS settings (types.
// HlsSettings, types.go:4007). FrameCaptureHlsSettings (types.
// FrameCaptureHlsSettings, types.go:2919) has no fields on the wire, so a
// bool records "this variant is set".
type HlsSettings struct {
	AudioOnlyHlsSettings    *AudioOnlyHlsSettings
	Fmp4HlsSettings         *Fmp4HlsSettings
	StandardHlsSettings     *StandardHlsSettings
	FrameCaptureHlsSettings bool
}

// HlsOutputSettings configures one HLS output (types.HlsOutputSettings,
// types.go:3975). Wire keys "h265PackagingType"/"hlsSettings"/
// "nameModifier"/"segmentModifier".
type HlsOutputSettings struct {
	HlsSettings       *HlsSettings
	H265PackagingType string
	NameModifier      string
	SegmentModifier   string
}

// MediaConnectRouterContainerSettings wraps M2tsSettings for MediaConnect
// Router outputs (types.MediaConnectRouterContainerSettings, types.go:5414).
// Wire key "m2tsSettings".
type MediaConnectRouterContainerSettings struct {
	M2tsSettings *M2tsSettings
}

// MediaConnectRouterOutputSettings configures one MediaConnect Router
// output (types.MediaConnectRouterOutputSettings, types.go:5471). Wire keys
// "containerSettings"/"destination". ConnectedRouterInputs is deliberately
// excluded -- the SDK documents it as "deprecated and unused".
type MediaConnectRouterOutputSettings struct {
	ContainerSettings *MediaConnectRouterContainerSettings
	Destination       *OutputLocationRef
}

// MediaPackageV2DestinationSettings configures MediaPackage V2 (CMAF
// Ingest) delivery for a MediaPackage output (types.
// MediaPackageV2DestinationSettings, types.go:5560). Wire keys
// "audioGroupId"/"audioRenditionSets"/"hlsAutoSelect"/"hlsDefault".
type MediaPackageV2DestinationSettings struct {
	AudioGroupID       string
	AudioRenditionSets string
	HlsAutoSelect      string
	HlsDefault         string
}

// MediaPackageOutputSettings configures one MediaPackage output (types.
// MediaPackageOutputSettings, types.go:5551). Wire key
// "mediaPackageV2DestinationSettings".
type MediaPackageOutputSettings struct {
	MediaPackageV2DestinationSettings *MediaPackageV2DestinationSettings
}

// MsSmoothOutputSettings configures one MS Smooth output (types.
// MsSmoothOutputSettings, types.go:5970). Wire keys "h265PackagingType"/
// "nameModifier".
type MsSmoothOutputSettings struct {
	H265PackagingType string
	NameModifier      string
}

// MultiplexM2tsSettings configures MPEG-2 TS packaging for a Multiplex
// output (types.MultiplexM2tsSettings, types.go:6145).
type MultiplexM2tsSettings struct {
	AbsentInputAudioBehavior        string
	Arib                            string
	AudioBufferModel                string
	AudioStreamType                 string
	CcDescriptor                    string
	Ebif                            string
	EsRateInPes                     string
	Klv                             string
	NielsenID3Behavior              string
	PcrControl                      string
	Scte35Control                   string
	AudioFramesPerPes               int32
	PcrPeriod                       int32
	Scte35PrerollPullupMilliseconds float64
}

// MultiplexContainerSettings wraps MultiplexM2tsSettings for Multiplex
// outputs (types.MultiplexContainerSettings, types.go:6131). Wire key
// "multiplexM2tsSettings".
type MultiplexContainerSettings struct {
	MultiplexM2tsSettings *MultiplexM2tsSettings
}

// MultiplexOutputSettings configures one Multiplex output (types.
// MultiplexOutputSettings, types.go:6229). Wire keys "containerSettings"/
// "destination".
type MultiplexOutputSettings struct {
	Destination       *OutputLocationRef
	ContainerSettings *MultiplexContainerSettings
}

// RtmpOutputSettings configures one RTMP output (types.RtmpOutputSettings,
// types.go:7243). Wire keys "certificateMode"/"connectionRetryInterval"/
// "destination"/"numRetries".
type RtmpOutputSettings struct {
	Destination             *OutputLocationRef
	CertificateMode         string
	ConnectionRetryInterval int32
	NumRetries              int32
}

// UDPContainerSettings wraps M2tsSettings for SRT/UDP outputs (types.
// UDPContainerSettings, types.go:8493). Wire key "m2tsSettings".
type UDPContainerSettings struct {
	M2tsSettings *M2tsSettings
}

// FecOutputSettings configures Forward Error Correction for a UDP output
// (types.FecOutputSettings, types.go:2803). Wire keys "columnDepth"/
// "includeFec"/"rowLength".
type FecOutputSettings struct {
	IncludeFec  string
	ColumnDepth int32
	RowLength   int32
}

// SrtOutputSettings configures one SRT output (types.SrtOutputSettings,
// types.go:8046). Wire keys "bufferMsec"/"containerSettings"/"destination"/
// "encryptionType"/"latency".
type SrtOutputSettings struct {
	ContainerSettings *UDPContainerSettings
	Destination       *OutputLocationRef
	EncryptionType    string
	BufferMsec        int32
	Latency           int32
}

// UDPOutputSettings configures one UDP output (types.UDPOutputSettings,
// types.go:8523). Wire keys "bufferMsec"/"containerSettings"/"destination"/
// "fecOutputSettings".
type UDPOutputSettings struct {
	ContainerSettings *UDPContainerSettings
	Destination       *OutputLocationRef
	FecOutputSettings *FecOutputSettings
	BufferMsec        int32
}

// OutputSettings is the tagged union of per-output-technology settings
// (types.OutputSettings, types.go:6827); at most one variant is set. All 11
// variants are modeled in full, along with the container/CDN/HLS-stream
// sub-unions they reference (M2tsSettings, HlsSettings, HlsCdnSettings,
// KeyProviderSettings, ArchiveCdnSettings, FrameCaptureCdnSettings).
type OutputSettings struct {
	ArchiveOutputSettings            *ArchiveOutputSettings
	CmafIngestOutputSettings         *CmafIngestOutputSettings
	FrameCaptureOutputSettings       *FrameCaptureOutputSettings
	HlsOutputSettings                *HlsOutputSettings
	MediaConnectRouterOutputSettings *MediaConnectRouterOutputSettings
	MediaPackageOutputSettings       *MediaPackageOutputSettings
	MsSmoothOutputSettings           *MsSmoothOutputSettings
	MultiplexOutputSettings          *MultiplexOutputSettings
	RtmpOutputSettings               *RtmpOutputSettings
	SrtOutputSettings                *SrtOutputSettings
	UDPOutputSettings                *UDPOutputSettings
}

func (s *OutputSettings) hasOutputSettings() bool {
	return s != nil && (s.ArchiveOutputSettings != nil || s.CmafIngestOutputSettings != nil ||
		s.FrameCaptureOutputSettings != nil || s.HlsOutputSettings != nil ||
		s.MediaConnectRouterOutputSettings != nil || s.MediaPackageOutputSettings != nil ||
		s.MsSmoothOutputSettings != nil || s.MultiplexOutputSettings != nil ||
		s.RtmpOutputSettings != nil || s.SrtOutputSettings != nil || s.UDPOutputSettings != nil)
}

// EncoderOutput names one encoder output and the AudioDescription/
// CaptionDescription/VideoDescription names it draws from (types.Output --
// named EncoderOutput here, not Output, to avoid colliding with this
// package's unrelated Output* helper types). Wire keys
// "audioDescriptionNames"/"captionDescriptionNames"/"outputName"/
// "outputSettings"/"videoDescriptionName" -- verified against types.Output,
// types.go:6827.
type EncoderOutput struct {
	OutputSettings          *OutputSettings
	OutputName              string
	VideoDescriptionName    string
	AudioDescriptionNames   []string
	CaptionDescriptionNames []string
}

// ArchiveGroupSettings configures an Archive output group (types.
// ArchiveGroupSettings, types.go:161). Wire keys "archiveCdnSettings"/
// "destination"/"rolloverInterval".
type ArchiveGroupSettings struct {
	Destination        *OutputLocationRef
	ArchiveCdnSettings *ArchiveCdnSettings
	RolloverInterval   int32
}

// CmafIngestGroupSettings configures a CMAF Ingest output group (types.
// CmafIngestGroupSettings, types.go:1795).
type CmafIngestGroupSettings struct {
	Destination              *OutputLocationRef
	NielsenID3NameModifier   string
	Scte35Type               string
	ID3Behavior              string
	ID3NameModifier          string
	KlvBehavior              string
	KlvNameModifier          string
	TimedMetadataPassthrough string
	NielsenID3Behavior       string
	Scte35NameModifier       string
	TimedMetadataID3Frame    string
	SegmentLengthUnits       string
	AdditionalDestinations   []OutputAdditionalDestination
	CaptionLanguageMappings  []CmafIngestCaptionLanguageMapping
	SegmentLength            int32
	SendDelayMs              int32
	TimedMetadataID3Period   int32
}

// FrameCaptureGroupSettings configures a Frame Capture output group (types.
// FrameCaptureGroupSettings, types.go:2898). Wire keys "destination"/
// "frameCaptureCdnSettings".
type FrameCaptureGroupSettings struct {
	Destination             *OutputLocationRef
	FrameCaptureCdnSettings *FrameCaptureCdnSettings
}

// HlsAkamaiSettings configures Akamai CDN delivery for an HLS output group
// (types.HlsAkamaiSettings, types.go:3568).
type HlsAkamaiSettings struct {
	HTTPTransferMode        string
	Salt                    string
	Token                   string
	ConnectionRetryInterval int32
	FilecacheDuration       int32
	NumRetries              int32
	RestartDelay            int32
}

// HlsBasicPutSettings configures basic-PUT CDN delivery for an HLS output
// group (types.HlsBasicPutSettings, types.go:3600).
type HlsBasicPutSettings struct {
	ConnectionRetryInterval int32
	FilecacheDuration       int32
	NumRetries              int32
	RestartDelay            int32
}

// HlsMediaStoreSettings configures MediaStore CDN delivery for an HLS
// output group (types.HlsMediaStoreSettings, types.go:3949).
type HlsMediaStoreSettings struct {
	MediaStoreStorageClass  string
	ConnectionRetryInterval int32
	FilecacheDuration       int32
	NumRetries              int32
	RestartDelay            int32
}

// HlsS3Settings configures S3 CDN delivery for an HLS output group (types.
// HlsS3Settings, types.go:3998). Wire key "cannedAcl".
type HlsS3Settings struct {
	CannedACL string
}

// HlsWebdavSettings configures WebDAV CDN delivery for an HLS output group
// (types.HlsWebdavSettings, types.go:4038).
type HlsWebdavSettings struct {
	HTTPTransferMode        string
	ConnectionRetryInterval int32
	FilecacheDuration       int32
	NumRetries              int32
	RestartDelay            int32
}

// HlsCdnSettings is the tagged union of CDN delivery mechanisms for an HLS
// output group (types.HlsCdnSettings, types.go:3622).
type HlsCdnSettings struct {
	HlsAkamaiSettings     *HlsAkamaiSettings
	HlsBasicPutSettings   *HlsBasicPutSettings
	HlsMediaStoreSettings *HlsMediaStoreSettings
	HlsS3Settings         *HlsS3Settings
	HlsWebdavSettings     *HlsWebdavSettings
}

// StaticKeySettings configures static-key HLS encryption (types.
// StaticKeySettings, types.go:8285). Wire keys "keyProviderServer"/
// "staticKeyValue".
type StaticKeySettings struct {
	KeyProviderServer InputLocation
	StaticKeyValue    string
}

// KeyProviderSettings is the tagged union of HLS encryption key providers
// (types.KeyProviderSettings, types.go:4995).
type KeyProviderSettings struct {
	StaticKeySettings *StaticKeySettings
}

// HlsGroupSettings configures an Apple HLS output group (types.
// HlsGroupSettings, types.go:3643). The largest OutputGroupSettings
// variant.
type HlsGroupSettings struct {
	Destination                *OutputLocationRef
	HlsCdnSettings             *HlsCdnSettings
	KeyProviderSettings        *KeyProviderSettings
	IvInManifest               string
	IFrameOnlyPlaylists        string
	BaseURLContent             string
	BaseURLContent1            string
	BaseURLManifest            string
	BaseURLManifest1           string
	CaptionLanguageSetting     string
	ClientCache                string
	CodecSpecification         string
	ConstantIv                 string
	DirectoryStructure         string
	DiscontinuityTags          string
	EncryptionType             string
	HlsID3SegmentTagging       string
	KeyFormat                  string
	IncompleteSegmentBehavior  string
	InputLossAction            string
	TSFileMode                 string
	TimedMetadataID3Frame      string
	IvSource                   string
	ProgramDateTime            string
	ManifestCompression        string
	ManifestDurationFormat     string
	Mode                       string
	OutputSelection            string
	KeyFormatVersions          string
	ProgramDateTimeClock       string
	RedundantManifest          string
	SegmentationMode           string
	StreamInfResolution        string
	CaptionLanguageMappings    []CaptionLanguageMapping
	AdMarkers                  []string
	IndexNSegments             int32
	KeepSegments               int32
	MinSegmentLength           int32
	ProgramDateTimePeriod      int32
	SegmentLength              int32
	SegmentsPerSubdirectory    int32
	TimedMetadataID3Period     int32
	TimestampDeltaMilliseconds int32
}

// MediaConnectRouterGroupSettings configures a MediaConnect Router output
// group (types.MediaConnectRouterGroupSettings, types.go:5423). Wire key
// "availabilityZones".
type MediaConnectRouterGroupSettings struct {
	AvailabilityZones []string
}

// MediaPackageV2GroupSettings configures MediaPackage V2 (CMAF Ingest)
// output when the group destination specifies a channelGroup/channelName
// (types.MediaPackageV2GroupSettings, types.go:5601).
type MediaPackageV2GroupSettings struct {
	ID3Behavior              string
	KlvBehavior              string
	NielsenID3Behavior       string
	Scte35Type               string
	SegmentLengthUnits       string
	TimedMetadataID3Frame    string
	TimedMetadataPassthrough string
	AdditionalDestinations   []OutputAdditionalDestination
	CaptionLanguageMappings  []CaptionLanguageMapping
	SegmentLength            int32
	TimedMetadataID3Period   int32
}

// MediaPackageGroupSettings configures a MediaPackage output group (types.
// MediaPackageGroupSettings, types.go:5502). Wire keys "destination"/
// "mediapackageV2GroupSettings".
type MediaPackageGroupSettings struct {
	Destination                 *OutputLocationRef
	MediaPackageV2GroupSettings *MediaPackageV2GroupSettings
}

// MsSmoothGroupSettings configures an MS Smooth output group (types.
// MsSmoothGroupSettings, types.go:5872).
type MsSmoothGroupSettings struct {
	Destination              *OutputLocationRef
	AcquisitionPointID       string
	AudioOnlyTimecodeControl string
	CertificateMode          string
	EventID                  string
	EventIDMode              string
	EventStopBehavior        string
	InputLossAction          string
	SegmentationMode         string
	SparseTrackType          string
	StreamManifestBehavior   string
	TimestampOffset          string
	TimestampOffsetMode      string
	ConnectionRetryInterval  int32
	FilecacheDuration        int32
	FragmentLength           int32
	NumRetries               int32
	RestartDelay             int32
	SendDelayMs              int32
}

// RtmpGroupSettings configures an RTMP output group (types.
// RtmpGroupSettings, types.go:7192).
type RtmpGroupSettings struct {
	AuthenticationScheme  string
	CacheFullBehavior     string
	CaptionData           string
	IncludeFillerNalUnits string
	InputLossAction       string
	AdMarkers             []string
	CacheLength           int32
	RestartDelay          int32
}

// SrtGroupSettings configures an SRT output group (types.SrtGroupSettings,
// types.go:7933). Wire key "inputLossAction".
type SrtGroupSettings struct {
	InputLossAction string
}

// UDPGroupSettings configures a UDP output group (types.UDPGroupSettings,
// types.go:8502). Wire keys "inputLossAction"/"timedMetadataId3Frame"/
// "timedMetadataId3Period".
type UDPGroupSettings struct {
	InputLossAction        string
	TimedMetadataID3Frame  string
	TimedMetadataID3Period int32
}

// OutputGroupSettings is the tagged union of per-output-technology group
// settings (types.OutputGroupSettings, types.go:6764); at most one variant
// is set. All 11 variants are modeled in full. MultiplexGroupSettings
// (types.MultiplexGroupSettings, types.go:6140) has no fields on the wire,
// so a bool records "this variant is set" -- same convention used
// throughout this file for empty-marker unions.
type OutputGroupSettings struct {
	ArchiveGroupSettings            *ArchiveGroupSettings
	CmafIngestGroupSettings         *CmafIngestGroupSettings
	FrameCaptureGroupSettings       *FrameCaptureGroupSettings
	HlsGroupSettings                *HlsGroupSettings
	MediaConnectRouterGroupSettings *MediaConnectRouterGroupSettings
	MediaPackageGroupSettings       *MediaPackageGroupSettings
	MsSmoothGroupSettings           *MsSmoothGroupSettings
	RtmpGroupSettings               *RtmpGroupSettings
	SrtGroupSettings                *SrtGroupSettings
	UDPGroupSettings                *UDPGroupSettings
	MultiplexGroupSettings          bool
}

func (s *OutputGroupSettings) hasOutputGroupSettings() bool {
	return s != nil && (s.ArchiveGroupSettings != nil || s.CmafIngestGroupSettings != nil ||
		s.FrameCaptureGroupSettings != nil || s.HlsGroupSettings != nil ||
		s.MediaConnectRouterGroupSettings != nil || s.MediaPackageGroupSettings != nil ||
		s.MsSmoothGroupSettings != nil || s.RtmpGroupSettings != nil ||
		s.SrtGroupSettings != nil || s.UDPGroupSettings != nil || s.MultiplexGroupSettings)
}

// OutputGroup names one output group and its member Outputs. Wire keys
// "name"/"outputGroupSettings"/"outputs" -- verified against types.
// OutputGroup, types.go:6764.
type OutputGroup struct {
	OutputGroupSettings *OutputGroupSettings
	Name                string
	Outputs             []EncoderOutput
}

// EsamSettings configures ESAM ad-avail signaling to a POIS server. Wire
// keys "acquisitionPointId"/"adAvailOffset"/"passwordParam"/"poisEndpoint"/
// "username"/"zoneIdentity" -- verified against types.Esam.
type EsamSettings struct {
	AcquisitionPointID string
	PoisEndpoint       string
	PasswordParam      string
	Username           string
	ZoneIdentity       string
	AdAvailOffset      int32
}

// Scte35SpliceInsertSettings is the "typical" SCTE-35 avail-insertion mode:
// all segmentation signals create breaks (types.Scte35SpliceInsert). Wire
// keys "adAvailOffset"/"noRegionalBlackoutFlag"/"webDeliveryAllowedFlag".
type Scte35SpliceInsertSettings struct {
	NoRegionalBlackoutFlag string
	WebDeliveryAllowedFlag string
	AdAvailOffset          int32
}

// Scte35TimeSignalAposSettings is the "atypical" SCTE-35 avail-insertion
// mode: only Time Signal Placement Opportunity/Break messages create breaks
// (types.Scte35TimeSignalApos). Same field shape as
// Scte35SpliceInsertSettings but a distinct wire object.
type Scte35TimeSignalAposSettings struct {
	NoRegionalBlackoutFlag string
	WebDeliveryAllowedFlag string
	AdAvailOffset          int32
}

// ChannelAvailSettings is the tagged union of ad-avail signaling methods
// (types.AvailSettings); at most one variant is set.
type ChannelAvailSettings struct {
	Esam                 *EsamSettings
	Scte35SpliceInsert   *Scte35SpliceInsertSettings
	Scte35TimeSignalApos *Scte35TimeSignalAposSettings
}

// ChannelAvailConfiguration configures how EncoderSettings creates SCTE-35
// ad-avail cues (types.AvailConfiguration). Wire keys "availSettings"/
// "scte35SegmentationScope".
type ChannelAvailConfiguration struct {
	AvailSettings           ChannelAvailSettings
	Scte35SegmentationScope string
}

func (a ChannelAvailConfiguration) hasAvailConfiguration() bool {
	return a.Scte35SegmentationScope != "" || a.AvailSettings.Esam != nil ||
		a.AvailSettings.Scte35SpliceInsert != nil || a.AvailSettings.Scte35TimeSignalApos != nil
}

// ChannelColorCorrection is one 3D-LUT color-space conversion entry (types.
// ColorCorrection). Wire keys "inputColorSpace"/"outputColorSpace"/"uri".
type ChannelColorCorrection struct {
	InputColorSpace  string
	OutputColorSpace string
	URI              string
}

// ChannelColorCorrectionSettings configures 3D-LUT-based color conversion
// (types.ColorCorrectionSettings). Wire key "globalColorCorrections".
type ChannelColorCorrectionSettings struct {
	GlobalColorCorrections []ChannelColorCorrection
}

func (s ChannelColorCorrectionSettings) hasColorCorrectionSettings() bool {
	return len(s.GlobalColorCorrections) > 0
}

// ChannelMotionGraphicsSettings is the tagged union of motion-graphics
// sources (types.MotionGraphicsSettings). The real SDK currently defines
// exactly one variant, HtmlMotionGraphicsSettings -- itself an empty marker
// struct on the wire (types.HtmlMotionGraphicsSettings has no fields) -- so
// a bool records "this variant is set" instead of an empty pointer-to-empty
// struct.
type ChannelMotionGraphicsSettings struct {
	HTMLMotionGraphicsSettings bool
}

// ChannelMotionGraphicsConfiguration configures motion-graphics overlay
// insertion (types.MotionGraphicsConfiguration). Wire keys
// "motionGraphicsInsertion"/"motionGraphicsSettings".
type ChannelMotionGraphicsConfiguration struct {
	MotionGraphicsInsertion string
	MotionGraphicsSettings  ChannelMotionGraphicsSettings
}

func (m ChannelMotionGraphicsConfiguration) hasMotionGraphicsConfiguration() bool {
	return m.MotionGraphicsInsertion != "" || m.MotionGraphicsSettings.HTMLMotionGraphicsSettings
}

// ChannelNielsenConfiguration configures Nielsen watermark-to-ID3 tagging
// (types.NielsenConfiguration). Wire keys "distributorId"/
// "nielsenPcmToId3Tagging".
type ChannelNielsenConfiguration struct {
	DistributorID          string
	NielsenPcmToID3Tagging string
}

func (n ChannelNielsenConfiguration) hasNielsenConfiguration() bool {
	return n.DistributorID != "" || n.NielsenPcmToID3Tagging != ""
}

// EncoderSettings is EncoderSettings' modeled subset -- see the per-type doc
// comments above and PARITY.md's gaps entry for exactly what's excluded (the
// per-codec AudioCodecSettings/VideoCodecSettings unions and the
// per-output-technology OutputGroupSettings/OutputSettings unions).
// CaptionDestinationSettings (gopherstack-1szb) IS modeled in full, alongside
// its BurnIn/DvbSub/EbuTtD/Ttml/Webvtt sub-shapes above. AvailConfiguration/
// ColorCorrectionSettings/MotionGraphicsConfiguration/NielsenConfiguration
// (gopherstack-sthr) ARE modeled in full below -- unlike the codec/
// output-technology unions, none of these four is itself a large per-format
// union (AvailConfiguration's AvailSettings is only a 3-way union of small
// flat structs, comparable to the failover-condition/output-locking unions
// already modeled above). AudioDescriptions/VideoDescriptions/OutputGroups/
// TimecodeConfig are required on a real CreateChannelInput's EncoderSettings;
// gopherstack accepts a partial value (matching every other
// optional-nested-object family in this service) since it does not perform
// AWS's own request validation.
type EncoderSettings struct {
	BlackoutSlate               BlackoutSlate
	AvailBlanking               AvailBlanking
	AvailConfiguration          ChannelAvailConfiguration
	FeatureActivations          FeatureActivations
	NielsenConfiguration        ChannelNielsenConfiguration
	MotionGraphicsConfiguration ChannelMotionGraphicsConfiguration
	ThumbnailConfiguration      ThumbnailConfiguration
	CaptionDescriptions         []CaptionDescription
	TimecodeConfig              TimecodeConfig
	OutputGroups                []OutputGroup
	ColorCorrectionSettings     ChannelColorCorrectionSettings
	VideoDescriptions           []VideoDescription
	AudioDescriptions           []AudioDescription
	GlobalConfiguration         GlobalConfiguration
}

// hasLegacyEncoderFields covers the EncoderSettings sub-fields modeled
// before gopherstack-sthr (see hasEncoderSettings, split out to stay under
// this repo's cyclomatic-complexity budget).
func (s EncoderSettings) hasLegacyEncoderFields() bool {
	return len(s.AudioDescriptions) > 0 || len(s.VideoDescriptions) > 0 ||
		len(s.CaptionDescriptions) > 0 || len(s.OutputGroups) > 0 ||
		s.TimecodeConfig.Source != "" || s.AvailBlanking.State != "" ||
		s.BlackoutSlate.State != "" ||
		s.FeatureActivations.InputPrepareScheduleActions != "" ||
		s.FeatureActivations.OutputStaticImageOverlayScheduleActions != "" ||
		s.ThumbnailConfiguration.State != "" ||
		s.GlobalConfiguration.InputEndAction != "" || s.GlobalConfiguration.InitialAudioGain != 0 ||
		s.GlobalConfiguration.OutputLockingMode != "" || s.GlobalConfiguration.OutputTimingSource != "" ||
		s.GlobalConfiguration.SupportLowFramerateInputs != ""
}

// hasSthrEncoderFields covers the four EncoderSettings sub-fields added by
// gopherstack-sthr (AvailConfiguration/ColorCorrectionSettings/
// MotionGraphicsConfiguration/NielsenConfiguration).
func (s EncoderSettings) hasSthrEncoderFields() bool {
	return s.AvailConfiguration.hasAvailConfiguration() ||
		s.ColorCorrectionSettings.hasColorCorrectionSettings() ||
		s.MotionGraphicsConfiguration.hasMotionGraphicsConfiguration() ||
		s.NielsenConfiguration.hasNielsenConfiguration()
}

func (s EncoderSettings) hasEncoderSettings() bool {
	return s.hasLegacyEncoderFields() || s.hasSthrEncoderFields()
}

// ChannelCreateExtras bundles the 11 CreateChannelInput members added by
// gopherstack-jb9i beyond name/channelClass/roleArn/tags/anywhereSettings
// (which remain direct CreateChannel parameters, matching the pre-existing
// convention) so CreateChannel's signature doesn't balloon to 15+ positional
// parameters. A zero-valued field means "not configured", matching a real
// CreateChannelInput that omits the corresponding member -- no presence
// tracking is needed for Create, unlike Update (see ChannelUpdateExtras).
type ChannelCreateExtras struct {
	InputSpecification    InputSpecification
	Maintenance           ChannelMaintenance
	ChannelEngineVersion  ChannelEngineVersion
	CdiInputSpecification CdiInputSpecification
	LogLevel              string
	LinkedChannelSettings ChannelLinkedChannelSettings
	Vpc                   ChannelVpcSettings
	InferenceSettings     ChannelInferenceSettings
	ChannelSecurityGroups []string
	Destinations          []ChannelOutputDestination
	InputAttachments      []ChannelInputAttachment
	EncoderSettings       EncoderSettings
}

// ChannelUpdateExtras is ChannelCreateExtras' Update-side counterpart. Each
// field is paired with a HasX flag so UpdateChannel can distinguish "the
// caller omitted this member" (leave unchanged) from "the caller sent an
// explicit, possibly zero, value" (overwrite) -- the same "include this
// parameter only if you want to change it" convention already used by
// UpdateChannel's anywhereSettings and UpdateCluster's networkSettings.
type ChannelUpdateExtras struct {
	Maintenance              ChannelMaintenance
	InputSpecification       InputSpecification
	ChannelEngineVersion     ChannelEngineVersion
	LogLevel                 string
	CdiInputSpecification    CdiInputSpecification
	Vpc                      ChannelVpcSettings
	LinkedChannelSettings    ChannelLinkedChannelSettings
	InferenceSettings        ChannelInferenceSettings
	Destinations             []ChannelOutputDestination
	InputAttachments         []ChannelInputAttachment
	ChannelSecurityGroups    []string
	EncoderSettings          EncoderSettings
	HasChannelSecurityGroups bool
	HasInputAttachments      bool
	HasDestinations          bool
	HasInputSpecification    bool
	HasEncoderSettings       bool
	HasLinkedChannelSettings bool
	HasInferenceSettings     bool
	HasLogLevel              bool
	HasChannelEngineVersion  bool
	HasMaintenance           bool
	HasCdiInputSpecification bool
	HasVpc                   bool
}

// Channel represents a MediaLive channel.
// Tags first: reduces GC pointer scan from 104 to 96 bytes.
type Channel struct {
	Tags                  map[string]string
	Maintenance           ChannelMaintenance
	InputSpecification    InputSpecification
	AnywhereSettings      ChannelAnywhereSettings
	ChannelEngineVersion  ChannelEngineVersion
	CdiInputSpecification CdiInputSpecification
	Name                  string
	LogLevel              string
	RoleARN               string
	State                 string
	ChannelClass          string
	ARN                   string
	ID                    string
	LinkedChannelSettings ChannelLinkedChannelSettings
	Vpc                   ChannelVpcSettings
	InferenceSettings     ChannelInferenceSettings
	InputAttachments      []ChannelInputAttachment
	Destinations          []ChannelOutputDestination
	ChannelSecurityGroups []string
	EncoderSettings       EncoderSettings
}

// ChannelSummary is a channel in a list response. Same shape as Channel
// minus EncoderSettings -- a real ListChannelsOutput/ChannelSummary never
// returns the (potentially huge) encoder configuration, verified against
// types.ChannelSummary.
type ChannelSummary struct {
	ARN                   string
	ID                    string
	Name                  string
	ChannelClass          string
	State                 string
	LogLevel              string
	ChannelSecurityGroups []string
	Destinations          []ChannelOutputDestination
	InputAttachments      []ChannelInputAttachment
	AnywhereSettings      ChannelAnywhereSettings
	CdiInputSpecification CdiInputSpecification
	ChannelEngineVersion  ChannelEngineVersion
	InferenceSettings     ChannelInferenceSettings
	InputSpecification    InputSpecification
	LinkedChannelSettings ChannelLinkedChannelSettings
	Maintenance           ChannelMaintenance
	Vpc                   ChannelVpcSettings
}

// Input represents a MediaLive input.
// Tags first, SdiSources last: reduces GC pointer scan from 120 to 112 bytes.
type Input struct {
	Tags       map[string]string
	ARN        string
	ID         string
	Name       string
	InputType  string
	RoleARN    string
	State      string
	SdiSources []string
}

// InputSummary is an input in a list response.
type InputSummary struct {
	ARN       string
	ID        string
	Name      string
	InputType string
	State     string
}

// InputSecurityGroup represents a MediaLive input security group.
// Tags first, then strings, then slice: reduces GC pointer scan from 80 to 64 bytes.
type InputSecurityGroup struct {
	Tags           map[string]string
	ARN            string
	ID             string
	State          string
	WhitelistRules []WhitelistRule
}

// InputSecurityGroupSummary is a security group in a list response.
type InputSecurityGroupSummary struct {
	Tags           map[string]string
	ARN            string
	ID             string
	State          string
	WhitelistRules []WhitelistRule
}

// WhitelistRule is a CIDR-based whitelist entry.
type WhitelistRule struct {
	Cidr string `json:"cidr"`
}

// InputDevice represents a MediaLive input device.
type InputDevice struct {
	Tags                    map[string]string
	ARN                     string
	ID                      string
	Name                    string
	SerialNumber            string
	MacAddress              string
	DeviceType              string
	ConnectionState         string
	DeviceSettingsSyncState string
	DeviceUpdateStatus      string
}

// InputDeviceTransfer represents a pending input device transfer.
type InputDeviceTransfer struct {
	DeviceID         string
	TargetCustomerID string
	TransferType     string
	Message          string
}

// MultiplexSettings holds transport-stream parameters for a Multiplex.
type MultiplexSettings struct {
	TransportStreamBitrate              int
	TransportStreamID                   int
	TransportStreamReservedBitrate      int
	MaximumVideoBufferDelayMilliseconds int
}

// Multiplex represents a MediaLive Multiplex resource.
// Tags first, value struct last: reduces GC pointer scan.
type Multiplex struct {
	Tags              map[string]string
	ARN               string
	ID                string
	Name              string
	State             string
	AvailabilityZones []string
	Settings          MultiplexSettings
	ProgramCount      int
}

// MultiplexSummary is a Multiplex in a list response.
type MultiplexSummary struct {
	Tags                   map[string]string
	ARN                    string
	ID                     string
	Name                   string
	State                  string
	AvailabilityZones      []string
	TransportStreamBitrate int
	ProgramCount           int
}

// ServiceDescriptor holds provider/service name for a program.
type ServiceDescriptor struct {
	ProviderName string
	ServiceName  string
}

// MultiplexProgramSettings holds the settings for a MultiplexProgram.
type MultiplexProgramSettings struct {
	ServiceDescriptor        ServiceDescriptor
	ProgramName              string
	PreferredChannelPipeline string
	ProgramNumber            int
}

// MultiplexProgram represents a program within a Multiplex.
// Strings first, value struct last: reduces GC pointer scan.
type MultiplexProgram struct {
	ChannelID   string
	ProgramName string
	Settings    MultiplexProgramSettings
}

// MultiplexProgramSummary is a program in a list response.
type MultiplexProgramSummary struct {
	ProgramName string
	ChannelID   string
}

// InterfaceMapping logically connects one interface on every Node in a
// Cluster with one Network. Wire keys (networkSettings.interfaceMappings[])
// are lowerCamel "logicalInterfaceName"/"networkId" -- verified against
// aws-sdk-go-v2/service/medialive's types.InterfaceMapping.
type InterfaceMapping struct {
	LogicalInterfaceName string
	NetworkID            string
}

// ClusterNetworkSettings connects the Nodes in a Cluster to one or more of
// the Networks the Cluster is associated with. A real DescribeCluster/
// CreateCluster/UpdateCluster/ListClusters response's "networkSettings" is
// nil/absent until the caller configures it (verified against
// aws-sdk-go-v2/service/medialive's ClusterNetworkSettings type and
// DescribeClusterOutput's deserializer) -- gopherstack tracked NO fields for
// this at all before this fix, silently dropping every caller's
// networkSettings on Create/UpdateCluster.
type ClusterNetworkSettings struct {
	DefaultRoute      string
	InterfaceMappings []InterfaceMapping
}

// hasNetworkSettings reports whether ns has any real content, so callers can
// omit an empty "networkSettings" key the same way a real, never-configured
// Cluster omits it entirely.
func (ns ClusterNetworkSettings) hasNetworkSettings() bool {
	return ns.DefaultRoute != "" || len(ns.InterfaceMappings) > 0
}

// Cluster represents a MediaLive Anywhere Cluster resource.
// Tags first: reduces GC pointer scan.
type Cluster struct {
	Tags            map[string]string
	NetworkSettings ClusterNetworkSettings
	ARN             string
	ID              string
	Name            string
	ClusterType     string
	InstanceRoleArn string
	State           string
	ChannelIDs      []string
}

// ClusterSummary is a Cluster in a list response.
type ClusterSummary struct {
	NetworkSettings ClusterNetworkSettings
	ARN             string
	ID              string
	Name            string
	ClusterType     string
	InstanceRoleArn string
	State           string
	ChannelIDs      []string
}

// Node represents a MediaLive Anywhere Node within a Cluster.
// Tags first: reduces GC pointer scan.
type Node struct {
	Tags                   map[string]string
	ARN                    string
	ID                     string
	Name                   string
	ClusterID              string
	Role                   string
	State                  string
	ConnectionState        string
	ChannelPlacementGroups []string
}

// NodeSummary is a Node in a list response.
type NodeSummary struct {
	ARN                    string
	ID                     string
	Name                   string
	ClusterID              string
	Role                   string
	State                  string
	ConnectionState        string
	ChannelPlacementGroups []string
}

// SignalMap represents a MediaLive signal map resource.
type SignalMap struct {
	CreatedAt                       time.Time
	ModifiedAt                      time.Time
	Tags                            map[string]string
	Arn                             string
	ID                              string
	Name                            string
	Description                     string
	DiscoveryEntryPointArn          string
	Status                          string
	MonitorDeploymentStatus         string
	CloudWatchAlarmTemplateGroupIDs []string
	EventBridgeRuleTemplateGroupIDs []string
}

// CloudWatchAlarmTemplateGroup is a named group for CloudWatch alarm templates.
type CloudWatchAlarmTemplateGroup struct {
	CreatedAt   time.Time
	ModifiedAt  time.Time
	Tags        map[string]string
	Arn         string
	ID          string
	Name        string
	Description string
}

// CloudWatchAlarmTemplateGroupSummary is a CloudWatchAlarmTemplateGroup in a
// list response. The real ListCloudWatchAlarmTemplateGroupsOutput items use
// the CloudWatchAlarmTemplateGroupSummary shape, which has "templateCount"
// -- a field that does NOT exist on Get/Create/Update's response shape
// (verified against aws-sdk-go-v2/service/medialive's
// CloudWatchAlarmTemplateGroupSummary vs CloudWatchAlarmTemplateGroup
// types).
type CloudWatchAlarmTemplateGroupSummary struct {
	CloudWatchAlarmTemplateGroup
	TemplateCount int32
}

// CloudWatchAlarmTemplate is a template for generating CloudWatch alarms.
type CloudWatchAlarmTemplate struct {
	CreatedAt          time.Time
	ModifiedAt         time.Time
	Tags               map[string]string
	Arn                string
	ID                 string
	Name               string
	Description        string
	GroupID            string
	GroupIdentifier    string
	MetricName         string
	Namespace          string
	Statistic          string
	ComparisonOperator string
	TargetResourceType string
	TreatMissingData   string
	Threshold          float64
	EvaluationPeriods  int32
	DatapointsToAlarm  int32
	Period             int32
}

// EventBridgeRuleTemplateGroup is a named group for EventBridge rule templates.
type EventBridgeRuleTemplateGroup struct {
	CreatedAt   time.Time
	ModifiedAt  time.Time
	Tags        map[string]string
	Arn         string
	ID          string
	Name        string
	Description string
}

// EventBridgeRuleTemplateGroupSummary is an EventBridgeRuleTemplateGroup in
// a list response -- same "templateCount only on the List Summary shape"
// nuance as CloudWatchAlarmTemplateGroupSummary (see its doc comment).
type EventBridgeRuleTemplateGroupSummary struct {
	EventBridgeRuleTemplateGroup
	TemplateCount int32
}

// EventBridgeRuleTemplateTarget is a target ARN for an EventBridge rule.
type EventBridgeRuleTemplateTarget struct {
	Arn string `json:"arn"`
}

// EventBridgeRuleTemplate is a template for EventBridge rules.
type EventBridgeRuleTemplate struct {
	CreatedAt       time.Time
	ModifiedAt      time.Time
	Tags            map[string]string
	Arn             string
	ID              string
	Name            string
	Description     string
	GroupID         string
	GroupIdentifier string
	EventType       string
	EventTargets    []EventBridgeRuleTemplateTarget
}

// EventBridgeRuleTemplateSummary is an EventBridgeRuleTemplate in a list
// response. The real ListEventBridgeRuleTemplatesOutput items use the
// EventBridgeRuleTemplateSummary shape, which has "eventTargetCount"
// (an integer) instead of the full "eventTargets" array (verified against
// aws-sdk-go-v2/service/medialive's EventBridgeRuleTemplateSummary vs
// EventBridgeRuleTemplate types).
type EventBridgeRuleTemplateSummary struct {
	EventBridgeRuleTemplate
	EventTargetCount int32
}

// OfferingResourceSpecification describes the resource type for an offering.
type OfferingResourceSpecification struct {
	ResourceType     string `json:"resourceType"`
	VideoQuality     string `json:"videoQuality"`
	Resolution       string `json:"resolution"`
	SpecialFeature   string `json:"specialFeature"`
	MaximumBitrate   string `json:"maximumBitrate"`
	MaximumFramerate string `json:"maximumFramerate"`
	Codec            string `json:"codec"`
}

// Offering is a pre-defined reserved resource listing from the MediaLive catalog.
type Offering struct {
	ResourceSpecification OfferingResourceSpecification
	Arn                   string
	OfferingID            string
	OfferingDescription   string
	OfferingType          string
	CurrencyCode          string
	DurationUnits         string
	Region                string
	FixedPrice            float64
	UsagePrice            float64
	Duration              int32
}

// RenewalSettings holds a Reservation's renewal configuration. Wire keys
// (renewalSettings.automaticRenewal/renewalCount) verified against
// aws-sdk-go-v2/service/medialive's
// awsRestjson1_serializeDocumentRenewalSettings.
type RenewalSettings struct {
	AutomaticRenewal string
	RenewalCount     int32
}

// hasRenewalSettings reports whether rs has any real content, so callers can
// omit an empty "renewalSettings" key.
func (rs RenewalSettings) hasRenewalSettings() bool {
	return rs.AutomaticRenewal != "" || rs.RenewalCount != 0
}

// Reservation is a purchased Offering.
type Reservation struct {
	Tags                  map[string]string
	ResourceSpecification OfferingResourceSpecification
	OfferingType          string
	End                   string
	Start                 string
	Name                  string
	OfferingID            string
	OfferingDescription   string
	DurationUnits         string
	Arn                   string
	ReservationID         string
	CurrencyCode          string
	Region                string
	State                 string
	RenewalSettings       RenewalSettings
	UsagePrice            float64
	FixedPrice            float64
	Duration              int32
	Count                 int32
}

// BatchSuccessfulResult is a successful result in a batch operation.
type BatchSuccessfulResult struct {
	Arn   string
	ID    string
	State string
}

// BatchFailedResult is a failed result in a batch operation.
type BatchFailedResult struct {
	Arn     string
	ID      string
	Code    string
	Message string
}

// BatchResult holds results of a batch start/stop/delete.
type BatchResult struct {
	Successful []BatchSuccessfulResult
	Failed     []BatchFailedResult
}

// ScheduleAction represents a single schedule action for BatchUpdateSchedule.
type ScheduleAction struct {
	ActionName string
	ActionType string
}

// BatchUpdateScheduleResult holds the result of BatchUpdateSchedule.
type BatchUpdateScheduleResult struct {
	Creates []ScheduleAction
	Deletes []ScheduleAction
}

var _ StorageBackend = (*InMemoryBackend)(nil)

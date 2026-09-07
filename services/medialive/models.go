package medialive

import (
	"maps"
	"time"
)

// storedChannel's gopherstack-jb9i additions (CdiInputSpecification through
// Vpc) mirror ChannelCreateExtras/ChannelUpdateExtras field-for-field -- see
// their doc comments in interfaces.go for what each wire member is and,
// where relevant, what sub-shape is deliberately not modeled.
type storedChannel struct {
	Tags                  map[string]string            `json:"tags"`
	Maintenance           ChannelMaintenance           `json:"maintenance"`
	InputSpecification    InputSpecification           `json:"inputSpecification"`
	AnywhereSettings      ChannelAnywhereSettings      `json:"anywhereSettings"`
	ChannelEngineVersion  ChannelEngineVersion         `json:"channelEngineVersion"`
	CdiInputSpecification CdiInputSpecification        `json:"cdiInputSpecification"`
	Name                  string                       `json:"name"`
	LogLevel              string                       `json:"logLevel"`
	RoleARN               string                       `json:"roleArn"`
	State                 string                       `json:"state"`
	ChannelClass          string                       `json:"channelClass"`
	ARN                   string                       `json:"arn"`
	ID                    string                       `json:"id"`
	LinkedChannelSettings ChannelLinkedChannelSettings `json:"linkedChannelSettings"`
	Vpc                   ChannelVpcSettings           `json:"vpc"`
	InferenceSettings     ChannelInferenceSettings     `json:"inferenceSettings"`
	InputAttachments      []ChannelInputAttachment     `json:"inputAttachments"`
	Destinations          []ChannelOutputDestination   `json:"destinations"`
	ChannelSecurityGroups []string                     `json:"channelSecurityGroups"`
	EncoderSettings       EncoderSettings              `json:"encoderSettings"`
}

// toChannel converts to the domain Channel shape. LinkedChannelSettings.
// Primary.FollowingChannelArns is NOT populated here (it's derived from
// every other channel's settings, not stored on this one) -- callers that
// need it use the backend's toChannelWithDerived, which stamps it in after
// calling this method. See ChannelPrimarySettings' doc comment.
func (c *storedChannel) toChannel() *Channel {
	tags := make(map[string]string, len(c.Tags))
	maps.Copy(tags, c.Tags)

	return &Channel{
		ARN:                   c.ARN,
		ID:                    c.ID,
		Name:                  c.Name,
		ChannelClass:          c.ChannelClass,
		RoleARN:               c.RoleARN,
		State:                 c.State,
		LogLevel:              c.LogLevel,
		Tags:                  tags,
		AnywhereSettings:      c.AnywhereSettings,
		CdiInputSpecification: c.CdiInputSpecification,
		ChannelEngineVersion:  c.ChannelEngineVersion,
		ChannelSecurityGroups: c.ChannelSecurityGroups,
		Destinations:          c.Destinations,
		EncoderSettings:       c.EncoderSettings,
		InferenceSettings:     c.InferenceSettings,
		InputAttachments:      c.InputAttachments,
		InputSpecification:    c.InputSpecification,
		LinkedChannelSettings: c.LinkedChannelSettings,
		Maintenance:           c.Maintenance,
		Vpc:                   c.Vpc,
	}
}

// toSummary converts to the domain ChannelSummary shape (same
// caveat as toChannel regarding FollowingChannelArns -- see
// InMemoryBackend.ListChannels, which stamps it in after calling this
// method). EncoderSettings is intentionally excluded -- see ChannelSummary's
// doc comment.
func (c *storedChannel) toSummary() *ChannelSummary {
	return &ChannelSummary{
		ARN:                   c.ARN,
		ID:                    c.ID,
		Name:                  c.Name,
		ChannelClass:          c.ChannelClass,
		State:                 c.State,
		LogLevel:              c.LogLevel,
		AnywhereSettings:      c.AnywhereSettings,
		CdiInputSpecification: c.CdiInputSpecification,
		ChannelEngineVersion:  c.ChannelEngineVersion,
		ChannelSecurityGroups: c.ChannelSecurityGroups,
		Destinations:          c.Destinations,
		InferenceSettings:     c.InferenceSettings,
		InputAttachments:      c.InputAttachments,
		InputSpecification:    c.InputSpecification,
		LinkedChannelSettings: c.LinkedChannelSettings,
		Maintenance:           c.Maintenance,
		Vpc:                   c.Vpc,
	}
}

type storedInput struct {
	Tags       map[string]string `json:"tags"`
	ARN        string            `json:"arn"`
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	InputType  string            `json:"inputType"`
	RoleARN    string            `json:"roleArn"`
	State      string            `json:"state"`
	SdiSources []string          `json:"sdiSources"`
}

func (i *storedInput) toInput() *Input {
	tags := make(map[string]string, len(i.Tags))
	maps.Copy(tags, i.Tags)

	sdiSources := make([]string, len(i.SdiSources))
	copy(sdiSources, i.SdiSources)

	return &Input{
		ARN:        i.ARN,
		ID:         i.ID,
		Name:       i.Name,
		InputType:  i.InputType,
		RoleARN:    i.RoleARN,
		State:      i.State,
		Tags:       tags,
		SdiSources: sdiSources,
	}
}

func (i *storedInput) toSummary() *InputSummary {
	return &InputSummary{
		ARN:       i.ARN,
		ID:        i.ID,
		Name:      i.Name,
		InputType: i.InputType,
		State:     i.State,
	}
}

type storedInputSecurityGroup struct {
	Tags           map[string]string `json:"tags"`
	ARN            string            `json:"arn"`
	ID             string            `json:"id"`
	State          string            `json:"state"`
	WhitelistRules []WhitelistRule   `json:"whitelistRules"`
}

func (g *storedInputSecurityGroup) toGroup() *InputSecurityGroup {
	tags := make(map[string]string, len(g.Tags))
	maps.Copy(tags, g.Tags)

	rules := make([]WhitelistRule, len(g.WhitelistRules))
	copy(rules, g.WhitelistRules)

	return &InputSecurityGroup{
		ARN:            g.ARN,
		ID:             g.ID,
		State:          g.State,
		WhitelistRules: rules,
		Tags:           tags,
	}
}

func (g *storedInputSecurityGroup) toSummary() *InputSecurityGroupSummary {
	rules := make([]WhitelistRule, len(g.WhitelistRules))
	copy(rules, g.WhitelistRules)

	tags := make(map[string]string, len(g.Tags))
	maps.Copy(tags, g.Tags)

	return &InputSecurityGroupSummary{
		ARN:            g.ARN,
		ID:             g.ID,
		State:          g.State,
		WhitelistRules: rules,
		Tags:           tags,
	}
}

// Tags and pointer fields first for optimal field alignment.
type storedInputDevice struct {
	Tags            map[string]string          `json:"tags"`
	PendingTransfer *storedInputDeviceTransfer `json:"pendingTransfer,omitempty"`
	ARN             string                     `json:"arn"`
	ID              string                     `json:"id"`
	Name            string                     `json:"name"`
	SerialNumber    string                     `json:"serialNumber"`
	MacAddress      string                     `json:"macAddress"`
	DeviceType      string                     `json:"deviceType"`
	ConnectionState string                     `json:"connectionState"`
	// DeviceSettingsSyncState and DeviceUpdateStatus: SYNCED/SYNCING, UP_TO_DATE/etc.
	DeviceSettingsSyncState string `json:"deviceSettingsSyncState"`
	DeviceUpdateStatus      string `json:"deviceUpdateStatus"`
}

func (d *storedInputDevice) toDevice() *InputDevice {
	tags := make(map[string]string, len(d.Tags))
	maps.Copy(tags, d.Tags)

	return &InputDevice{
		Tags:                    tags,
		ARN:                     d.ARN,
		ID:                      d.ID,
		Name:                    d.Name,
		SerialNumber:            d.SerialNumber,
		MacAddress:              d.MacAddress,
		DeviceType:              d.DeviceType,
		ConnectionState:         d.ConnectionState,
		DeviceSettingsSyncState: d.DeviceSettingsSyncState,
		DeviceUpdateStatus:      d.DeviceUpdateStatus,
	}
}

func (d *storedInputDevice) toPendingTransfer(transferType string) *InputDeviceTransfer {
	if d.PendingTransfer == nil {
		return nil
	}

	return &InputDeviceTransfer{
		DeviceID:         d.ID,
		TargetCustomerID: d.PendingTransfer.TargetCustomerID,
		TransferType:     transferType,
		Message:          d.PendingTransfer.Message,
	}
}

type storedInputDeviceTransfer struct {
	TargetCustomerID string `json:"targetCustomerId"`
	TargetRegion     string `json:"targetRegion"`
	Message          string `json:"message"`
}

type storedMultiplexSettings struct {
	TransportStreamBitrate              int `json:"transportStreamBitrate"`
	TransportStreamID                   int `json:"transportStreamId"`
	TransportStreamReservedBitrate      int `json:"transportStreamReservedBitrate"`
	MaximumVideoBufferDelayMilliseconds int `json:"maximumVideoBufferDelayMilliseconds"`
}

// Tags and Programs (maps) first, then slice, then strings, then value struct: reduces GC pointer scan.
type storedMultiplex struct {
	Tags              map[string]string                  `json:"tags"`
	Programs          map[string]*storedMultiplexProgram `json:"programs"`
	ARN               string                             `json:"arn"`
	ID                string                             `json:"id"`
	Name              string                             `json:"name"`
	State             string                             `json:"state"`
	AvailabilityZones []string                           `json:"availabilityZones"`
	Settings          storedMultiplexSettings            `json:"settings"`
}

func (m *storedMultiplex) toMultiplex() *Multiplex {
	tags := make(map[string]string, len(m.Tags))
	maps.Copy(tags, m.Tags)

	zones := make([]string, len(m.AvailabilityZones))
	copy(zones, m.AvailabilityZones)

	return &Multiplex{
		Tags:              tags,
		AvailabilityZones: zones,
		ARN:               m.ARN,
		ID:                m.ID,
		Name:              m.Name,
		State:             m.State,
		Settings: MultiplexSettings{
			TransportStreamBitrate:              m.Settings.TransportStreamBitrate,
			TransportStreamID:                   m.Settings.TransportStreamID,
			TransportStreamReservedBitrate:      m.Settings.TransportStreamReservedBitrate,
			MaximumVideoBufferDelayMilliseconds: m.Settings.MaximumVideoBufferDelayMilliseconds,
		},
		ProgramCount: len(m.Programs),
	}
}

func (m *storedMultiplex) toSummary() *MultiplexSummary {
	zones := make([]string, len(m.AvailabilityZones))
	copy(zones, m.AvailabilityZones)

	tags := make(map[string]string, len(m.Tags))
	maps.Copy(tags, m.Tags)

	return &MultiplexSummary{
		ARN:                    m.ARN,
		ID:                     m.ID,
		Name:                   m.Name,
		State:                  m.State,
		AvailabilityZones:      zones,
		ProgramCount:           len(m.Programs),
		TransportStreamBitrate: m.Settings.TransportStreamBitrate,
		Tags:                   tags,
	}
}

type storedServiceDescriptor struct {
	ProviderName string `json:"providerName"`
	ServiceName  string `json:"serviceName"`
}

type storedMultiplexProgramSettings struct {
	ServiceDescriptor        storedServiceDescriptor `json:"serviceDescriptor"`
	PreferredChannelPipeline string                  `json:"preferredChannelPipeline"`
	ProgramNumber            int                     `json:"programNumber"`
}

// Strings first, value struct last: reduces GC pointer scan.
type storedMultiplexProgram struct {
	ProgramName string                         `json:"programName"`
	ChannelID   string                         `json:"channelId"`
	Settings    storedMultiplexProgramSettings `json:"settings"`
}

func (p *storedMultiplexProgram) toProgram() *MultiplexProgram {
	return &MultiplexProgram{
		ChannelID:   p.ChannelID,
		ProgramName: p.ProgramName,
		Settings: MultiplexProgramSettings{
			ProgramName:              p.ProgramName,
			ProgramNumber:            p.Settings.ProgramNumber,
			PreferredChannelPipeline: p.Settings.PreferredChannelPipeline,
			ServiceDescriptor: ServiceDescriptor{
				ProviderName: p.Settings.ServiceDescriptor.ProviderName,
				ServiceName:  p.Settings.ServiceDescriptor.ServiceName,
			},
		},
	}
}

func (p *storedMultiplexProgram) toSummary() *MultiplexProgramSummary {
	return &MultiplexProgramSummary{
		ProgramName: p.ProgramName,
		ChannelID:   p.ChannelID,
	}
}

// Tags and Nodes (maps) first, then strings: reduces GC pointer scan.
type storedCluster struct {
	Tags            map[string]string      `json:"tags"`
	Nodes           map[string]*storedNode `json:"nodes"`
	ARN             string                 `json:"arn"`
	ID              string                 `json:"id"`
	Name            string                 `json:"name"`
	ClusterType     string                 `json:"clusterType"`
	InstanceRoleArn string                 `json:"instanceRoleArn"`
	State           string                 `json:"state"`
	NetworkSettings ClusterNetworkSettings `json:"networkSettings"`
}

// toCluster converts to the domain Cluster shape. channelIDs is the live
// set of Channel IDs whose AnywhereSettings.ClusterID matches this cluster
// (derived, not persisted -- see Channel.AnywhereSettings' doc comment for
// why gopherstack can now compute this instead of hardcoding an empty
// list).
func (c *storedCluster) toCluster(channelIDs []string) *Cluster {
	tags := make(map[string]string, len(c.Tags))
	maps.Copy(tags, c.Tags)

	return &Cluster{
		Tags:            tags,
		ARN:             c.ARN,
		ID:              c.ID,
		Name:            c.Name,
		ClusterType:     c.ClusterType,
		InstanceRoleArn: c.InstanceRoleArn,
		State:           c.State,
		NetworkSettings: c.NetworkSettings,
		ChannelIDs:      channelIDs,
	}
}

func (c *storedCluster) toSummary(channelIDs []string) *ClusterSummary {
	return &ClusterSummary{
		ARN:             c.ARN,
		ID:              c.ID,
		Name:            c.Name,
		ClusterType:     c.ClusterType,
		InstanceRoleArn: c.InstanceRoleArn,
		State:           c.State,
		NetworkSettings: c.NetworkSettings,
		ChannelIDs:      channelIDs,
	}
}

// Tags first, then strings: reduces GC pointer scan.
type storedNode struct {
	Tags            map[string]string `json:"tags"`
	ARN             string            `json:"arn"`
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	ClusterID       string            `json:"clusterId"`
	Role            string            `json:"role"`
	State           string            `json:"state"`
	ConnectionState string            `json:"connectionState"`
}

// toNode converts to the domain Node shape. cpgIDs is the live set of
// ChannelPlacementGroup IDs whose Nodes list contains this node's ID
// (derived, not persisted).
func (n *storedNode) toNode(cpgIDs []string) *Node {
	tags := make(map[string]string, len(n.Tags))
	maps.Copy(tags, n.Tags)

	return &Node{
		Tags:                   tags,
		ARN:                    n.ARN,
		ID:                     n.ID,
		Name:                   n.Name,
		ClusterID:              n.ClusterID,
		Role:                   n.Role,
		State:                  n.State,
		ConnectionState:        n.ConnectionState,
		ChannelPlacementGroups: cpgIDs,
	}
}

func (n *storedNode) toSummary(cpgIDs []string) *NodeSummary {
	return &NodeSummary{
		ARN:                    n.ARN,
		ID:                     n.ID,
		Name:                   n.Name,
		ClusterID:              n.ClusterID,
		Role:                   n.Role,
		State:                  n.State,
		ConnectionState:        n.ConnectionState,
		ChannelPlacementGroups: cpgIDs,
	}
}

type storedSignalMap struct {
	CreatedAt                       time.Time         `json:"createdAt"`
	ModifiedAt                      time.Time         `json:"modifiedAt"`
	Tags                            map[string]string `json:"tags"`
	Arn                             string            `json:"arn"`
	ID                              string            `json:"id"`
	Name                            string            `json:"name"`
	Description                     string            `json:"description"`
	DiscoveryEntryPointArn          string            `json:"discoveryEntryPointArn"`
	Status                          string            `json:"status"`
	MonitorDeploymentStatus         string            `json:"monitorDeploymentStatus"`
	CloudWatchAlarmTemplateGroupIDs []string          `json:"cloudWatchAlarmTemplateGroupIds"`
	EventBridgeRuleTemplateGroupIDs []string          `json:"eventBridgeRuleTemplateGroupIds"`
}

func (s *storedSignalMap) toSignalMap() *SignalMap {
	tags := make(map[string]string, len(s.Tags))
	maps.Copy(tags, s.Tags)
	cwIDs := make([]string, len(s.CloudWatchAlarmTemplateGroupIDs))
	copy(cwIDs, s.CloudWatchAlarmTemplateGroupIDs)
	ebIDs := make([]string, len(s.EventBridgeRuleTemplateGroupIDs))
	copy(ebIDs, s.EventBridgeRuleTemplateGroupIDs)

	return &SignalMap{
		Tags:                            tags,
		CloudWatchAlarmTemplateGroupIDs: cwIDs,
		EventBridgeRuleTemplateGroupIDs: ebIDs,
		Arn:                             s.Arn,
		ID:                              s.ID,
		Name:                            s.Name,
		Description:                     s.Description,
		DiscoveryEntryPointArn:          s.DiscoveryEntryPointArn,
		Status:                          s.Status,
		MonitorDeploymentStatus:         s.MonitorDeploymentStatus,
		CreatedAt:                       s.CreatedAt,
		ModifiedAt:                      s.ModifiedAt,
	}
}

type storedCloudWatchAlarmTemplateGroup struct {
	CreatedAt   time.Time         `json:"createdAt"`
	ModifiedAt  time.Time         `json:"modifiedAt"`
	Tags        map[string]string `json:"tags"`
	Arn         string            `json:"arn"`
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
}

func (g *storedCloudWatchAlarmTemplateGroup) toGroup() *CloudWatchAlarmTemplateGroup {
	tags := make(map[string]string, len(g.Tags))
	maps.Copy(tags, g.Tags)

	return &CloudWatchAlarmTemplateGroup{
		Tags:        tags,
		Arn:         g.Arn,
		ID:          g.ID,
		Name:        g.Name,
		Description: g.Description,
		CreatedAt:   g.CreatedAt,
		ModifiedAt:  g.ModifiedAt,
	}
}

type storedCloudWatchAlarmTemplate struct {
	CreatedAt          time.Time         `json:"createdAt"`
	ModifiedAt         time.Time         `json:"modifiedAt"`
	Tags               map[string]string `json:"tags"`
	Arn                string            `json:"arn"`
	ID                 string            `json:"id"`
	Name               string            `json:"name"`
	Description        string            `json:"description"`
	GroupID            string            `json:"groupId"`
	GroupIdentifier    string            `json:"groupIdentifier"`
	MetricName         string            `json:"metricName"`
	Namespace          string            `json:"namespace"`
	Statistic          string            `json:"statistic"`
	ComparisonOperator string            `json:"comparisonOperator"`
	TargetResourceType string            `json:"targetResourceType"`
	TreatMissingData   string            `json:"treatMissingData"`
	Threshold          float64           `json:"threshold"`
	EvaluationPeriods  int32             `json:"evaluationPeriods"`
	DatapointsToAlarm  int32             `json:"datapointsToAlarm"`
	Period             int32             `json:"period"`
}

func (t *storedCloudWatchAlarmTemplate) toTemplate() *CloudWatchAlarmTemplate {
	tags := make(map[string]string, len(t.Tags))
	maps.Copy(tags, t.Tags)

	return &CloudWatchAlarmTemplate{
		Tags: tags, Arn: t.Arn, ID: t.ID, Name: t.Name, Description: t.Description,
		GroupID: t.GroupID, GroupIdentifier: t.GroupIdentifier,
		MetricName: t.MetricName, Namespace: t.Namespace, Statistic: t.Statistic,
		ComparisonOperator: t.ComparisonOperator, TargetResourceType: t.TargetResourceType,
		TreatMissingData: t.TreatMissingData, Threshold: t.Threshold,
		EvaluationPeriods: t.EvaluationPeriods, DatapointsToAlarm: t.DatapointsToAlarm, Period: t.Period,
		CreatedAt: t.CreatedAt, ModifiedAt: t.ModifiedAt,
	}
}

type storedEventBridgeRuleTemplateGroup struct {
	CreatedAt   time.Time         `json:"createdAt"`
	ModifiedAt  time.Time         `json:"modifiedAt"`
	Tags        map[string]string `json:"tags"`
	Arn         string            `json:"arn"`
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
}

func (g *storedEventBridgeRuleTemplateGroup) toGroup() *EventBridgeRuleTemplateGroup {
	tags := make(map[string]string, len(g.Tags))
	maps.Copy(tags, g.Tags)

	return &EventBridgeRuleTemplateGroup{
		Tags:        tags,
		Arn:         g.Arn,
		ID:          g.ID,
		Name:        g.Name,
		Description: g.Description,
		CreatedAt:   g.CreatedAt,
		ModifiedAt:  g.ModifiedAt,
	}
}

type storedEventBridgeRuleTemplate struct {
	CreatedAt       time.Time                       `json:"createdAt"`
	ModifiedAt      time.Time                       `json:"modifiedAt"`
	Tags            map[string]string               `json:"tags"`
	Arn             string                          `json:"arn"`
	ID              string                          `json:"id"`
	Name            string                          `json:"name"`
	Description     string                          `json:"description"`
	GroupID         string                          `json:"groupId"`
	GroupIdentifier string                          `json:"groupIdentifier"`
	EventType       string                          `json:"eventType"`
	EventTargets    []EventBridgeRuleTemplateTarget `json:"eventTargets"`
}

func (t *storedEventBridgeRuleTemplate) toTemplate() *EventBridgeRuleTemplate {
	tags := make(map[string]string, len(t.Tags))
	maps.Copy(tags, t.Tags)
	targets := make([]EventBridgeRuleTemplateTarget, len(t.EventTargets))
	copy(targets, t.EventTargets)

	return &EventBridgeRuleTemplate{
		Tags: tags, EventTargets: targets, Arn: t.Arn, ID: t.ID, Name: t.Name,
		Description: t.Description, GroupID: t.GroupID, GroupIdentifier: t.GroupIdentifier,
		EventType: t.EventType, CreatedAt: t.CreatedAt, ModifiedAt: t.ModifiedAt,
	}
}

type storedReservation struct {
	Tags                  map[string]string             `json:"tags"`
	ResourceSpecification OfferingResourceSpecification `json:"resourceSpecification"`
	OfferingType          string                        `json:"offeringType"`
	End                   string                        `json:"end"`
	ReservationID         string                        `json:"reservationId"`
	Name                  string                        `json:"name"`
	OfferingID            string                        `json:"offeringId"`
	OfferingDescription   string                        `json:"offeringDescription"`
	DurationUnits         string                        `json:"durationUnits"`
	CurrencyCode          string                        `json:"currencyCode"`
	Start                 string                        `json:"start"`
	Arn                   string                        `json:"arn"`
	Region                string                        `json:"region"`
	State                 string                        `json:"state"`
	RenewalSettings       RenewalSettings               `json:"renewalSettings"`
	FixedPrice            float64                       `json:"fixedPrice"`
	UsagePrice            float64                       `json:"usagePrice"`
	Duration              int32                         `json:"duration"`
	Count                 int32                         `json:"count"`
}

func (r *storedReservation) toReservation() *Reservation {
	tags := make(map[string]string, len(r.Tags))
	maps.Copy(tags, r.Tags)

	return &Reservation{
		Tags: tags, ResourceSpecification: r.ResourceSpecification,
		RenewalSettings: r.RenewalSettings,
		Arn:             r.Arn, ReservationID: r.ReservationID, Name: r.Name,
		OfferingID: r.OfferingID, OfferingDescription: r.OfferingDescription,
		OfferingType: r.OfferingType, CurrencyCode: r.CurrencyCode,
		Start: r.Start, End: r.End, Region: r.Region, State: r.effectiveState(),
		FixedPrice: r.FixedPrice, UsagePrice: r.UsagePrice,
		Duration: r.Duration, DurationUnits: r.DurationUnits, Count: r.Count,
	}
}

// storedScheduleAction persists one schedule action for a channel.
type storedScheduleAction struct {
	ActionName string `json:"actionName"`
	ActionType string `json:"actionType"`
}

type storedNetwork struct {
	Tags    map[string]string `json:"tags"`
	ARN     string            `json:"arn"`
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	State   string            `json:"state"`
	IPPools []IPPool          `json:"ipPools"`
	Routes  []Route           `json:"routes"`
}

// toNetwork converts to the domain Network shape. clusterIDs is the live set
// of Cluster IDs whose NetworkSettings.InterfaceMappings reference this
// network (derived, not persisted -- see clusterIDsForNetwork; gopherstack
// previously hardcoded this to an always-empty slice, gopherstack-1um).
func (n *storedNetwork) toNetwork(clusterIDs []string) *Network {
	tags := make(map[string]string, len(n.Tags))
	maps.Copy(tags, n.Tags)
	clusters := make([]string, len(clusterIDs))
	copy(clusters, clusterIDs)
	pools := make([]IPPool, len(n.IPPools))
	copy(pools, n.IPPools)
	routes := make([]Route, len(n.Routes))
	copy(routes, n.Routes)

	return &Network{
		Tags:                 tags,
		ARN:                  n.ARN,
		ID:                   n.ID,
		Name:                 n.Name,
		State:                n.State,
		AssociatedClusterIDs: clusters,
		IPPools:              pools,
		Routes:               routes,
	}
}

type storedSdiSource struct {
	ARN    string   `json:"arn"`
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Type   string   `json:"type"`
	Mode   string   `json:"mode"`
	State  string   `json:"state"`
	Inputs []string `json:"inputs"`
}

func (s *storedSdiSource) toSdiSource() *SdiSource {
	inputs := make([]string, len(s.Inputs))
	copy(inputs, s.Inputs)

	return &SdiSource{
		ARN:    s.ARN,
		ID:     s.ID,
		Name:   s.Name,
		Type:   s.Type,
		Mode:   s.Mode,
		State:  s.State,
		Inputs: inputs,
	}
}

type storedChannelPlacementGroup struct {
	Tags      map[string]string `json:"tags"`
	ARN       string            `json:"arn"`
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	ClusterID string            `json:"clusterId"`
	State     string            `json:"state"`
	Nodes     []string          `json:"nodes"`
}

// toGroup converts to the domain ChannelPlacementGroup shape. channelIDs is
// the live set of Channel IDs whose AnywhereSettings.ChannelPlacementGroupID
// matches this group (derived, not persisted -- the group previously had a
// "Channels" field that was always initialized to an empty slice and never
// updated on channel attach; replaced with a live derivation now that
// Channel.AnywhereSettings tracks the association).
func (g *storedChannelPlacementGroup) toGroup(channelIDs []string) *ChannelPlacementGroup {
	tags := make(map[string]string, len(g.Tags))
	maps.Copy(tags, g.Tags)
	nodes := make([]string, len(g.Nodes))
	copy(nodes, g.Nodes)

	return &ChannelPlacementGroup{
		Tags:      tags,
		ARN:       g.ARN,
		ID:        g.ID,
		Name:      g.Name,
		ClusterID: g.ClusterID,
		State:     g.State,
		Channels:  channelIDs,
		Nodes:     nodes,
	}
}

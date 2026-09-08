package medialive

import (
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

const (
	defaultMaxResults = 20

	stateIdle     = "IDLE"
	stateStarting = "STARTING"
	stateRunning  = "RUNNING"
	stateStopping = "STOPPING"
	stateDeleted  = "DELETED"
	stateDeleting = "DELETING"

	stateDetached = "DETACHED"

	channelClassStandard       = "STANDARD"
	channelClassSinglePipeline = "SINGLE_PIPELINE"
	inputTypeUDPPush           = "UDP_PUSH"
	inputSecurityGroupActive   = "IDLE"

	pipelinesRunningCountStandard       = 2
	pipelinesRunningCountSinglePipeline = 1

	offeringTypeNoUpfront        = "NO_UPFRONT"
	offeringCurrencyUSD          = "USD"
	offeringDurationMonths       = "MONTHS"
	offeringVideoQualityStandard = "STANDARD"
	offeringUsagePrice           = 0.5
	offeringUsagePrice2          = 1.5
	offeringUsagePrice3          = 0.2
	offeringDuration             = 12
	batchErrNotFound             = "NOT_FOUND"

	resourceTypeChannel            = "channel"
	resourceTypeInput              = "input"
	resourceTypeInputSecurityGroup = "inputSecurityGroup"
	resourceTypeInputDevice        = "inputDevice"
	resourceTypeMultiplex          = "multiplex"
	resourceTypeCluster            = "cluster"
	resourceTypeNode               = "node"

	clusterStateActive   = "ACTIVE"
	clusterStateDeleting = "DELETING"
	clusterStateDeleted  = "DELETED"

	nodeStateActive    = "ACTIVE"
	nodeStateDeleted   = "DELETED"
	nodeRoleActive     = "ACTIVE"
	nodeConnectionConn = "CONNECTED"

	deviceConnectionConnected = "CONNECTED"
	deviceSettingsSynced      = "SYNCED"
	deviceUpdateUpToDate      = "UP_TO_DATE"
	deviceTypeHD              = "HD"
	transferTypeOutgoing      = "OUTGOING"
	transferTypeIncoming      = "INCOMING"

	networkStateActive   = "ACTIVE"
	networkStateDeleting = "DELETING"

	sdiSourceStateIdle    = "IDLE"
	sdiSourceStateDeleted = "DELETED"
	sdiSourceTypeSingle   = "SINGLE"
	sdiSourceModeQuadrant = "QUADRANT"

	channelPlacementGroupStateUnassigned = "UNASSIGNED"
	channelPlacementGroupStateDeleting   = "DELETING"

	channelEngineVersion = "AVCHD-1.0.0"
)

// InMemoryBackend is an in-memory implementation of StorageBackend.
type InMemoryBackend struct {
	cwAlarmTemplates         *store.Table[storedCloudWatchAlarmTemplate]
	ebRuleTemplateGroups     *store.Table[storedEventBridgeRuleTemplateGroup]
	inputs                   *store.Table[storedInput]
	inputSecurityGroups      *store.Table[storedInputSecurityGroup]
	inputDevices             *store.Table[storedInputDevice]
	pendingTransferDeviceIDs map[string]struct{}
	multiplexes              *store.Table[storedMultiplex]
	clusters                 *store.Table[storedCluster]
	tags                     map[string]map[string]string
	signalMaps               *store.Table[storedSignalMap]
	ebRuleTemplates          *store.Table[storedEventBridgeRuleTemplate]
	channels                 *store.Table[storedChannel]
	mu                       *lockmetrics.RWMutex
	registry                 *store.Registry
	cwAlarmTemplateGroups    *store.Table[storedCloudWatchAlarmTemplateGroup]
	reservations             *store.Table[storedReservation]
	scheduleActions          map[string][]*storedScheduleAction
	networks                 *store.Table[storedNetwork]
	sdiSources               *store.Table[storedSdiSource]
	channelPlacementGroups   *store.Table[storedChannelPlacementGroup]
	// nowFunc is the backend's time source, overridable in tests so
	// PurchaseOffering's derived Start/End can be exercised deterministically.
	nowFunc         func() time.Time
	accountKmsKeyID string
	accountID       string
	region          string
	offerings       []*Offering
}

// NewInMemoryBackend creates a new InMemoryBackend.
func NewInMemoryBackend(accountID, region string) *InMemoryBackend {
	b := &InMemoryBackend{
		mu:                       lockmetrics.New("medialive"),
		registry:                 store.NewRegistry(),
		pendingTransferDeviceIDs: make(map[string]struct{}),
		tags:                     make(map[string]map[string]string),
		scheduleActions:          make(map[string][]*storedScheduleAction),
		offerings:                seedOfferings(region),
		accountID:                accountID,
		region:                   region,
		nowFunc:                  time.Now,
	}
	registerAllTables(b)

	return b
}

func (b *InMemoryBackend) now() time.Time { return b.nowFunc().UTC() }

// seedOfferings returns a small catalog of standard offerings.
func seedOfferings(region string) []*Offering {
	hd := OfferingResourceSpecification{
		ResourceType: "OUTPUT", VideoQuality: offeringVideoQualityStandard, Resolution: "HD",
		MaximumBitrate: "MAX_20_MBPS", MaximumFramerate: "MAX_30_FPS", Codec: "AVC",
		SpecialFeature: "AUDIO_NORMALIZATION",
	}
	uhd := OfferingResourceSpecification{
		ResourceType: "OUTPUT", VideoQuality: offeringVideoQualityStandard, Resolution: "UHD",
		MaximumBitrate: "MAX_50_MBPS", MaximumFramerate: "MAX_60_FPS", Codec: "HEVC",
	}
	input := OfferingResourceSpecification{
		ResourceType: "INPUT", VideoQuality: offeringVideoQualityStandard, Resolution: "HD",
		MaximumBitrate: "MAX_20_MBPS", MaximumFramerate: "MAX_30_FPS", Codec: "AVC",
	}

	return []*Offering{
		{
			OfferingID:            "87654321",
			Arn:                   "arn:aws:medialive:" + region + "::offering:87654321",
			OfferingDescription:   "HD AVC output at 10-20 Mbps, 30 fps, standard VQ in " + region,
			OfferingType:          offeringTypeNoUpfront,
			CurrencyCode:          offeringCurrencyUSD,
			FixedPrice:            0.0,
			UsagePrice:            offeringUsagePrice,
			Duration:              offeringDuration,
			DurationUnits:         offeringDurationMonths,
			ResourceSpecification: hd,
			Region:                region,
		},
		{
			OfferingID:            "12345678",
			Arn:                   "arn:aws:medialive:" + region + "::offering:12345678",
			OfferingDescription:   "UHD HEVC output at 20-50 Mbps, 60 fps, standard VQ in " + region,
			OfferingType:          offeringTypeNoUpfront,
			CurrencyCode:          offeringCurrencyUSD,
			FixedPrice:            0.0,
			UsagePrice:            offeringUsagePrice2,
			Duration:              offeringDuration,
			DurationUnits:         offeringDurationMonths,
			ResourceSpecification: uhd,
			Region:                region,
		},
		{
			OfferingID:            "11223344",
			Arn:                   "arn:aws:medialive:" + region + "::offering:11223344",
			OfferingDescription:   "HD AVC input at 10-20 Mbps, 30 fps, standard VQ in " + region,
			OfferingType:          offeringTypeNoUpfront,
			CurrencyCode:          offeringCurrencyUSD,
			FixedPrice:            0.0,
			UsagePrice:            offeringUsagePrice3,
			Duration:              offeringDuration,
			DurationUnits:         offeringDurationMonths,
			ResourceSpecification: input,
			Region:                region,
		},
	}
}

// AccountID returns the configured account ID.
func (b *InMemoryBackend) AccountID() string { return b.accountID }

// Region returns the configured region.
func (b *InMemoryBackend) Region() string { return b.region }

// Reset clears all stored data.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	b.registry.ResetAll()
	b.pendingTransferDeviceIDs = make(map[string]struct{})
	b.tags = make(map[string]map[string]string)
	b.scheduleActions = make(map[string][]*storedScheduleAction)
	b.accountKmsKeyID = ""
}

func (b *InMemoryBackend) channelARN(id string) string {
	return arn.Build("medialive", b.region, b.accountID, resourceTypeChannel+":"+id)
}

func (b *InMemoryBackend) inputARN(id string) string {
	return arn.Build("medialive", b.region, b.accountID, resourceTypeInput+":"+id)
}

func (b *InMemoryBackend) inputSecurityGroupARN(id string) string {
	return arn.Build("medialive", b.region, b.accountID, resourceTypeInputSecurityGroup+":"+id)
}

func (b *InMemoryBackend) inputDeviceARN(id string) string {
	return arn.Build("medialive", b.region, b.accountID, resourceTypeInputDevice+":"+id)
}

func (b *InMemoryBackend) signalMapARN(id string) string {
	return arn.Build("medialive", b.region, b.accountID, "signal-map:"+id)
}

func (b *InMemoryBackend) cwAlarmTemplateGroupARN(id string) string {
	return arn.Build("medialive", b.region, b.accountID, "cloudwatch-alarm-template-group:"+id)
}

func (b *InMemoryBackend) cwAlarmTemplateARN(id string) string {
	return arn.Build("medialive", b.region, b.accountID, "cloudwatch-alarm-template:"+id)
}

func (b *InMemoryBackend) ebRuleTemplateGroupARN(id string) string {
	return arn.Build("medialive", b.region, b.accountID, "eventbridge-rule-template-group:"+id)
}

func (b *InMemoryBackend) ebRuleTemplateARN(id string) string {
	return arn.Build("medialive", b.region, b.accountID, "eventbridge-rule-template:"+id)
}

func (b *InMemoryBackend) reservationARN(id string) string {
	return arn.Build("medialive", b.region, b.accountID, "reservation:"+id)
}

func (b *InMemoryBackend) networkARN(id string) string {
	return arn.Build("medialive", b.region, b.accountID, "network:"+id)
}

func (b *InMemoryBackend) sdiSourceARN(id string) string {
	return arn.Build("medialive", b.region, b.accountID, "sdiSource:"+id)
}

func (b *InMemoryBackend) channelPlacementGroupARN(id string) string {
	return arn.Build("medialive", b.region, b.accountID, "channelPlacementGroup:"+id)
}

func newID() string {
	return uuid.New().String()[:8]
}

func (b *InMemoryBackend) multiplexARN(id string) string {
	return arn.Build("medialive", b.region, b.accountID, resourceTypeMultiplex+":"+id)
}

func (b *InMemoryBackend) clusterARN(id string) string {
	return arn.Build("medialive", b.region, b.accountID, resourceTypeCluster+":"+id)
}

func (b *InMemoryBackend) nodeARN(id string) string {
	return arn.Build("medialive", b.region, b.accountID, resourceTypeNode+":"+id)
}

// --- Account configuration operations ---

// DescribeAccountConfiguration returns the account-wide configuration.
func (b *InMemoryBackend) DescribeAccountConfiguration() (*AccountConfiguration, error) {
	b.mu.RLock("DescribeAccountConfiguration")
	defer b.mu.RUnlock()

	return &AccountConfiguration{KmsKeyID: b.accountKmsKeyID}, nil
}

// UpdateAccountConfiguration updates the account-wide configuration.
func (b *InMemoryBackend) UpdateAccountConfiguration(
	kmsKeyID string,
) (*AccountConfiguration, error) {
	b.mu.Lock("UpdateAccountConfiguration")
	defer b.mu.Unlock()

	b.accountKmsKeyID = kmsKeyID

	return &AccountConfiguration{KmsKeyID: b.accountKmsKeyID}, nil
}

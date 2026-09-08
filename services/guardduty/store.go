package guardduty

import (
	"fmt"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

const (
	defaultFindingSeverity = 5.0
	severityLowThreshold   = 4.0
	severityHighThreshold  = 7.0

	statusEnabled  = "ENABLED"
	statusDisabled = "DISABLED"
	statusActive   = "ACTIVE"
	statusInactive = "INACTIVE"
	freqSixHours   = "SIX_HOURS"

	errResourceNotFound  = "ResourceNotFoundException"
	errConflictException = "ConflictException"

	// arnPartCount is the number of colon-separated parts in a well-formed
	// ARN (arn:partition:service:region:account:resource) -- the resource
	// part itself may contain further "/"-separated segments, so it is not
	// split further by SplitN.
	arnPartCount = 6
)

// InMemoryBackend implements StorageBackend using pkgs/store tables.
//
// See store_setup.go for how the tables/indexes below are registered and
// persistence.go for how they are snapshotted/restored. tags (a
// map[string]string value map, not *T) and memberSeq (a scalar counter) are
// the only state left as plain fields -- see persistence.go's file doc
// comment for the per-field persistence audit.
type InMemoryBackend struct {
	mu       *lockmetrics.RWMutex
	registry *store.Registry

	// detectors is a "clean" table: DetectorID is a real, wire-visible
	// identity field, so it is registered directly on registry.
	detectors *store.Table[Detector]

	// filters, ipSets, and threatIntelSets were detector-nested maps
	// (map[string]map[string]*T). Each is now a single flat table keyed by
	// the composite "detectorID|id" string (see detectorKey), with a
	// companion byDetector index replacing the per-detector scans the
	// nested maps used to answer directly. DetectorID is hidden from the
	// wire shape (json:"-"), so these are "dirty" tables -- not registered
	// on registry directly (see persistence.go's DTO wrapping).
	filters           *store.Table[Filter]
	filtersByDetector *store.Index[Filter]

	ipSets           *store.Table[IPSet]
	ipSetsByDetector *store.Index[IPSet]

	threatIntelSets           *store.Table[ThreatIntelSet]
	threatIntelSetsByDetector *store.Index[ThreatIntelSet]

	// findings and members were also detector-nested, but Finding.DetectorID
	// and Member.DetectorID are real (non-hidden) wire fields, so both are
	// "clean" composite-keyed tables registered directly on registry.
	findings           *store.Table[Finding]
	findingsByDetector *store.Index[Finding]

	members           *store.Table[Member]
	membersByDetector *store.Index[Member]

	// tags is a non-*T value map (map[string]string), so it does not fit
	// store.Table's keyed-by-identity-value shape; it remains a plain map,
	// persisted directly (see persistence.go).
	tags map[string]map[string]string

	// invitations and orgAdminAccounts are flat (not detector-nested) maps
	// whose value types carry a real identity field (InvitationID,
	// AdminAccountID), so both are "clean" tables registered directly.
	invitations      *store.Table[Invitation]
	orgAdminAccounts *store.Table[OrgAdminAccount]

	// orgConfigs and adminAccounts were flat maps keyed by detectorID, but
	// OrgConfig and AdminAccount carry no identity field of their own
	// (identity-less). Each gained an unexported detectorID field purely
	// for the table's key; being unexported it never round-trips through a
	// direct json.Marshal of the value, so both are "dirty" tables (see
	// persistence.go's DTO wrapping).
	orgConfigs    *store.Table[OrgConfig]
	adminAccounts *store.Table[AdminAccount]

	// publishingDestinations, threatEntitySets, and trustedEntitySets are
	// detector-nested maps whose value types hide DetectorID (json:"-"),
	// same treatment as filters/ipSets/threatIntelSets above.
	publishingDestinations           *store.Table[PublishingDestination]
	publishingDestinationsByDetector *store.Index[PublishingDestination]

	threatEntitySets           *store.Table[ThreatEntitySet]
	threatEntitySetsByDetector *store.Index[ThreatEntitySet]

	trustedEntitySets           *store.Table[TrustedEntitySet]
	trustedEntitySetsByDetector *store.Index[TrustedEntitySet]

	// investigations is a detector-nested table with the same DetectorID
	// json:"-" treatment as filters/ipSets/threatIntelSets above.
	investigations           *store.Table[Investigation]
	investigationsByDetector *store.Index[Investigation]

	// malwareScans and malwareProtectionPlans are flat maps whose value
	// types carry a real identity field (ScanID, MalwareProtectionPlanID),
	// so both are "clean" tables registered directly.
	malwareScans           *store.Table[MalwareScan]
	malwareProtectionPlans *store.Table[MalwareProtectionPlan]

	// malwareScanSettings was a flat map keyed by detectorID; like
	// orgConfigs/adminAccounts, MalwareScanSettings is identity-less and
	// gained an unexported detectorID field, making it a "dirty" table.
	malwareScanSettings *store.Table[MalwareScanSettings]

	// appConfig is service.AppContext.Config, captured for lazy sibling-service
	// lookups -- see cross_service.go's SetAppConfig.
	appConfig any

	accountID string
	region    string
	memberSeq int64
}

// NewInMemoryBackend constructs a new InMemoryBackend.
func NewInMemoryBackend(accountID, region string) *InMemoryBackend {
	b := &InMemoryBackend{
		mu:        lockmetrics.New("guardduty"),
		accountID: accountID,
		region:    region,
		registry:  store.NewRegistry(),
		tags:      make(map[string]map[string]string),
	}
	registerAllTables(b)

	return b
}

// detectorKey builds the composite primary key used by every detector-nested
// table (filters, findings, ipSets, threatIntelSets, members,
// publishingDestinations, threatEntitySets, trustedEntitySets).
func detectorKey(detectorID, id string) string { return detectorID + "|" + id }

func (b *InMemoryBackend) detectorARN(id string) string {
	return arn.Build("guardduty", b.region, b.accountID, fmt.Sprintf("detector/%s", id))
}

func (b *InMemoryBackend) filterARN(detectorID, filterName string) string {
	return arn.Build("guardduty", b.region, b.accountID, fmt.Sprintf("detector/%s/filter/%s", detectorID, filterName))
}

func (b *InMemoryBackend) ipSetARN(detectorID, ipSetID string) string {
	return arn.Build("guardduty", b.region, b.accountID, fmt.Sprintf("detector/%s/ipset/%s", detectorID, ipSetID))
}

func (b *InMemoryBackend) threatIntelSetARN(detectorID, setID string) string {
	return arn.Build(
		"guardduty", b.region, b.accountID,
		fmt.Sprintf("detector/%s/threatintelset/%s", detectorID, setID),
	)
}

func (b *InMemoryBackend) findingARN(detectorID, findingID string) string {
	return arn.Build("guardduty", b.region, b.accountID, fmt.Sprintf("detector/%s/finding/%s", detectorID, findingID))
}

func (b *InMemoryBackend) threatEntitySetARN(detectorID, setID string) string {
	return arn.Build(
		"guardduty", b.region, b.accountID,
		fmt.Sprintf("detector/%s/threatentityset/%s", detectorID, setID),
	)
}

func (b *InMemoryBackend) trustedEntitySetARN(detectorID, setID string) string {
	return arn.Build(
		"guardduty", b.region, b.accountID,
		fmt.Sprintf("detector/%s/trustedentityset/%s", detectorID, setID),
	)
}

func (b *InMemoryBackend) publishingDestinationARN(detectorID, destID string) string {
	return arn.Build(
		"guardduty", b.region, b.accountID,
		fmt.Sprintf("detector/%s/publishingDestination/%s", detectorID, destID),
	)
}

// AccountID returns the configured account ID.
func (b *InMemoryBackend) AccountID() string { return b.accountID }

// Region returns the configured region.
func (b *InMemoryBackend) Region() string { return b.region }

// Reset clears all state.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	// registry.ResetAll handles every "clean" table (detectors, findings,
	// members, invitations, orgAdminAccounts, malwareScans,
	// malwareProtectionPlans). The "dirty" tables below were deliberately
	// NOT registered on registry (see store_setup.go), so each is reset
	// individually.
	b.registry.ResetAll()
	b.filters.Reset()
	b.ipSets.Reset()
	b.threatIntelSets.Reset()
	b.publishingDestinations.Reset()
	b.threatEntitySets.Reset()
	b.trustedEntitySets.Reset()
	b.investigations.Reset()
	b.orgConfigs.Reset()
	b.adminAccounts.Reset()
	b.malwareScanSettings.Reset()
	b.tags = make(map[string]map[string]string)
	b.memberSeq = 0
}

package networkmonitor

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// regionContextKey is the context key under which the per-request AWS region is stored.
type regionContextKey struct{}

// getRegion extracts the region from ctx, falling back to defaultRegion when unset.
func getRegion(ctx context.Context, defaultRegion string) string {
	if r, ok := ctx.Value(regionContextKey{}).(string); ok && r != "" {
		return r
	}

	return defaultRegion
}

var (
	// ErrNotFound is returned when a monitor or probe does not exist.
	ErrNotFound = awserr.New("ResourceNotFoundException", awserr.ErrNotFound)
	// ErrAlreadyExists is returned when a monitor already exists.
	ErrAlreadyExists = awserr.New("ConflictException", awserr.ErrAlreadyExists)
	// ErrValidation is returned for invalid input parameters.
	ErrValidation = awserr.New("ValidationException", awserr.ErrInvalidParameter)
	// ErrServiceQuotaExceeded is returned when a create operation would exceed
	// one of the documented Network Synthetic Monitor service quotas (see the
	// max*PerAccount/max*PerMonitor constants below).
	ErrServiceQuotaExceeded = errors.New("networkmonitor: service quota exceeded")
)

const (
	monitorStateActive = "ACTIVE"

	probeStateActive = "ACTIVE"

	protocolTCP  = "TCP"
	protocolICMP = "ICMP"

	defaultAggregationPeriod = int64(60)
	minAggregationPeriod     = int64(30)

	networkmonitorService = "networkmonitor"

	arnColonParts  = 6
	probePathParts = 2

	// minPacketSize/maxPacketSize bound the probe packetSize (bytes), matching
	// the real networkmonitor API's documented constraint ("must be a number
	// between 56 and 8500").
	minPacketSize = int32(56)
	maxPacketSize = int32(8500)

	// minDestinationPort/maxDestinationPort bound the probe destinationPort.
	// The real API's documented range for every op that carries a
	// destinationPort (CreateMonitorProbeInput, ProbeInput, Probe,
	// CreateProbeOutput/GetProbeOutput, UpdateProbeInput/Output -- see
	// aws-sdk-go-v2/service/networkmonitor/types/types.go doc comments) is
	// consistently "a number between 1 and 65536", so 65536 (not TCP's usual
	// 65535 ceiling) is the correct upper bound here.
	minDestinationPort = int32(1)
	maxDestinationPort = int32(65536)

	// Service quotas for Network Synthetic Monitor, confirmed against
	// https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/cloudwatch_limits.html#nw-monitor-quotas
	// (service code "networkmonitor"): "Number of monitors per account per AWS
	// region" (default 100), "Number of probes per monitor" (default 24), and
	// "Number of probes per subnet for each monitor" (default 4). All three
	// are adjustable in real AWS but gopherstack emulates the unmodified
	// defaults.
	maxMonitorsPerAccountRegion  = 100
	maxProbesPerMonitor          = 24
	maxProbesPerSubnetPerMonitor = 4
)

var monitorNameRE = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,200}$`)

// isValidProbeState reports whether state is one of the known ProbeState
// enum values from the real SDK (types.ProbeState.Values()): PENDING,
// ACTIVE, INACTIVE, ERROR, DELETING, DELETED.
func isValidProbeState(state string) bool {
	switch state {
	case "PENDING", "ACTIVE", "INACTIVE", "ERROR", "DELETING", "DELETED":
		return true
	default:
		return false
	}
}

// StorageBackend is the interface for the Network Monitor in-memory backend.
type StorageBackend interface {
	CreateMonitor(
		ctx context.Context,
		name string,
		aggregationPeriod *int64,
		probes []createMonitorProbeInput,
		tags map[string]string,
	) (*Monitor, error)
	DeleteMonitor(ctx context.Context, name string) error
	GetMonitor(ctx context.Context, name string) (*Monitor, error)
	UpdateMonitor(ctx context.Context, name string, aggregationPeriod int64) (*Monitor, error)
	ListMonitors(
		ctx context.Context,
		state, nextToken string,
		maxResults int,
	) ([]monitorSummary, string, error)
	CreateProbe(
		ctx context.Context,
		monitorName string,
		probe *probeInput,
		tags map[string]string,
	) (*Probe, error)
	DeleteProbe(ctx context.Context, monitorName, probeID string) error
	GetProbe(ctx context.Context, monitorName, probeID string) (*Probe, error)
	UpdateProbe(
		ctx context.Context,
		monitorName, probeID string,
		req *updateProbeRequest,
	) (*Probe, error)
	ListTagsForResource(ctx context.Context, resourceARN string) (map[string]string, error)
	TagResource(ctx context.Context, resourceARN string, tags map[string]string) error
	UntagResource(ctx context.Context, resourceARN string, tagKeys []string) error
	Snapshot(ctx context.Context) []byte
	Restore(ctx context.Context, data []byte) error
}

// InMemoryBackend is the in-memory implementation of StorageBackend.
// Resources are isolated by region via the "region|name" composite key
// [regionKey] builds (see store_setup.go for the Phase 3.3 datalayer
// conversion this struct went through).
type InMemoryBackend struct {
	registry         *store.Registry
	monitors         *store.Table[Monitor]
	monitorsByRegion *store.Index[Monitor]
	mu               *lockmetrics.RWMutex
	accountID        string
	defaultRegion    string
	nextProbeSeq     int64
}

var _ StorageBackend = (*InMemoryBackend)(nil)

// NewInMemoryBackend creates a new InMemoryBackend.
func NewInMemoryBackend(region, accountID string) *InMemoryBackend {
	b := &InMemoryBackend{
		registry:      store.NewRegistry(),
		accountID:     accountID,
		defaultRegion: region,
		mu:            lockmetrics.New("networkmonitor"),
	}

	registerAllTables(b)

	return b
}

// Reset clears all backend state.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	b.registry.ResetAll()
	b.nextProbeSeq = 0
}

// regionKey builds the composite "region|id" primary key store.Table uses
// for monitors, which were previously nested by region
// (map[string]map[string]*Monitor).
func regionKey(region, id string) string { return region + "|" + id }

func (b *InMemoryBackend) buildMonitorARN(region, monitorName string) string {
	return arn.Build(networkmonitorService, region, b.accountID, "monitor/"+monitorName)
}

func (b *InMemoryBackend) buildProbeARN(region, monitorName, probeID string) string {
	return arn.Build(
		networkmonitorService,
		region,
		b.accountID,
		fmt.Sprintf("probe/%s/%s", monitorName, probeID),
	)
}

func (b *InMemoryBackend) nextProbeID() string {
	b.nextProbeSeq++

	return fmt.Sprintf("probe-%08d", b.nextProbeSeq)
}

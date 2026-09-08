// Package ce provides an in-memory implementation of the AWS Cost Explorer (Ce) service.
package ce

import (
	"slices"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// DefaultAnomalyTTL is the default time-to-live for detected anomalies.
const DefaultAnomalyTTL = 30 * 24 * time.Hour

const (
	granularityDaily        = "DAILY"
	granularityMonthly      = "MONTHLY"
	granularityHourly       = "HOURLY"
	metricUnitUSD           = "USD"
	metricUnitNA            = "N/A"
	timePeriodKeyEnd        = "End"
	timePeriodKeyStart      = "Start"
	syntheticInstanceType   = "t3.medium"
	spUtilizationPct        = "85.0000"
	riCoveragePct           = "65.0000"
	riUtilizationPct        = "88.0000"
	zeroAmountStr           = "0.0000"
	defaultSavingsPlansType = "COMPUTE_SP"
	mapKeyRegion            = "Region"
	mapKeyCurrencyCode      = "CurrencyCode"
	dimKeyService           = "SERVICE"
	dimKeyRegion            = "REGION"
	dimKeyUsageType         = "USAGE_TYPE"
	dimKeyLinkedAccount     = "LINKED_ACCOUNT"
	statusProcessing        = "PROCESSING"
	accountScopePayer       = "PAYER"
	accountScopeLinked      = "LINKED"
)

// Synthetic data ratio constants used in cost simulation.
const (
	syntheticJitterRange     = 5    // half-range for YearDay jitter (±5 units)
	syntheticJitterScale     = 0.01 // 1% variation per YearDay unit
	unblendedCostFactor      = 0.98 // UnblendedCost = BlendedCost * 98%
	usageQuantityFactor      = 10   // UsageQuantity units per cost dollar
	amortizedCostFactor      = 0.99 // AmortizedCost = UnblendedCost * 99%
	netUnblendedCostFactor   = 0.97 // NetUnblendedCost = UnblendedCost * 97%
	normalizedUsageFactor    = 4    // normalized usage units per quantity unit
	stddevMinThreshold       = 0.01 // minimum stddev; values below use fallback
	stddevFallbackRatio      = 0.05 // fallback stddev = mean * 5%
	spCommitmentRatio        = 0.60 // SP commitment = total cost * 60%
	spUsedCommitmentRatio    = 0.85 // SP used commitment = total commitment * 85%
	spNetSavingsRatio        = 0.25 // SP net savings = total cost * 25%
	riPurchasedCostRatio     = 0.40 // RI purchased hours ratio relative to total cost
	riActualUsageRatio       = 0.88 // RI actual hours = purchased * 88%
	costToHoursMultiplier    = 10   // synthetic hours per cost dollar
	riNetSavingsRatio        = 0.30 // RI net savings = total * 30%
	riPotentialSavingsRatio  = 0.35 // RI potential savings = total * 35%
	riAmortizedFeeRatio      = 0.70 // RI amortized fee = purchased * 70%
	riRealizedSavingsRatio   = 0.28 // RI realized savings = total * 28%
	riUnrealizedSavingsRatio = 0.07 // RI unrealized savings = total * 7%
	riCoverageRatio          = 0.65 // RI covered hours fraction
	normalizedUnitsPerHour   = 4    // normalized units per running hour
	onDemandCostRate         = 0.05 // on-demand cost per synthetic hour
	daysPerMonth             = 30   // synthetic month length in days
	riMonthlyCostRatio       = 0.60 // RI monthly cost = on-demand cost * 60%
	riBreakEvenDivisor       = 2    // break-even months = term months / 2
	riUpfrontSplitRatio      = 0.50 // upfront and recurring each at 50% of RI cost
	rightsizingSavingsRatio  = 0.5  // rightsizing target saves 50% of current cost
	analysisETAMinutes       = 5    // estimated minutes until commitment analysis completes
	forecastMinLevel         = 51   // minimum valid prediction interval level
	forecastMaxLevel         = 99   // maximum valid prediction interval level
	forecastDefaultLevel     = 80   // default prediction interval level
	forecastBaseZ            = 1.28 // z-score at the default 80% prediction interval level
	forecastZScalePerPct     = 0.02 // z-score increment per percentage point above default
)

// InMemoryBackend is a thread-safe in-memory store for Cost Explorer resources.
type InMemoryBackend struct {
	// registry holds every store.Table-backed resource field so their
	// Reset/Snapshot/Restore collapse to one call each -- every table below
	// is "clean" (registered directly under its own real, non-json:"-"
	// identity field, no DTO-registry needed); see store_setup.go.
	registry                *store.Registry
	costCategories          *store.Table[CostCategory]
	anomalyMonitors         *store.Table[AnomalyMonitor]
	anomalySubscriptions    *store.Table[AnomalySubscription]
	anomalies               *store.Table[Anomaly]
	mu                      *lockmetrics.RWMutex
	costAllocationTags      *store.Table[CostAllocationTag]
	commitmentAnalyses      *store.Table[CommitmentAnalysis]
	savingsPlansGenerations *store.Table[SavingsPlansGeneration]
	accountID               string
	region                  string
	costLedger              []CostEntry
	backfillJobs            []*BackfillJob
	anomalyTTL              time.Duration
}

// NewInMemoryBackend creates a new backend for the given account and region.
func NewInMemoryBackend(accountID, region string) *InMemoryBackend {
	b := &InMemoryBackend{
		registry:   store.NewRegistry(),
		accountID:  accountID,
		region:     region,
		mu:         lockmetrics.New("ce"),
		anomalyTTL: DefaultAnomalyTTL,
	}
	registerAllTables(b)
	b.seedCostLedger()

	return b
}

// Region returns the region for this backend instance.
func (b *InMemoryBackend) Region() string { return b.region }

// Reset clears all in-memory state.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	b.registry.ResetAll()
	b.costLedger = nil
	b.backfillJobs = nil
	b.seedCostLedger()
}

// containsString reports whether s appears in slice.
func containsString(slice []string, s string) bool {
	return slices.Contains(slice, s)
}

// paginateList sorts list by keyFn, applies the opaque nextPageToken cursor, and returns
// at most maxResults items plus the token for the following page (empty on the last page).
func paginateList[T any](list []T, maxResults int, nextPageToken string, keyFn func(T) string) ([]T, string) {
	sort.Slice(list, func(i, j int) bool {
		return keyFn(list[i]) < keyFn(list[j])
	})

	start := 0
	if nextPageToken != "" {
		for i := range list {
			if keyFn(list[i]) >= nextPageToken {
				start = i

				break
			}

			start = i + 1
		}
	}

	const defaultPageSize = 100
	limit := maxResults
	if limit <= 0 || limit > defaultPageSize {
		limit = defaultPageSize
	}

	end := min(start+limit, len(list))
	page := list[start:end]

	next := ""
	if end < len(list) {
		next = keyFn(list[end])
	}

	return page, next
}

// paginateOrdered pages through list without re-sorting it, unlike
// [paginateList]. Use it when the caller has already established the display
// order (e.g. most-recently-started-first, or an independent SortBy) and
// pagination must preserve that order rather than re-sorting by keyFn.
// keyFn must still produce a value unique per item -- nextPageToken is the
// key of the first item of the next page (see the "next" assignment below),
// so the cursor resumes AT the item whose key matches nextPageToken, not
// after it: `start = i + 1` here would silently skip that item on every
// resumed page, dropping exactly one record per page boundary.
func paginateOrdered[T any](list []T, maxResults int, nextPageToken string, keyFn func(T) string) ([]T, string) {
	start := 0

	if nextPageToken != "" {
		for i := range list {
			if keyFn(list[i]) == nextPageToken {
				start = i

				break
			}
		}
	}

	const defaultPageSize = 100
	limit := maxResults
	if limit <= 0 || limit > defaultPageSize {
		limit = defaultPageSize
	}

	end := min(start+limit, len(list))
	page := list[start:end]

	next := ""
	if end < len(list) {
		next = keyFn(list[end])
	}

	return page, next
}

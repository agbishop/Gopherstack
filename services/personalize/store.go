package personalize

import (
	"maps"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// statusCreatePending/statusCreateProgress/statusDeletePending/
// statusCreateFailed/statusStopPending are real Personalize wire values
// (confirmed against every Status-bearing type's doc comment in
// aws-sdk-go-v2/service/personalize@v1.50.4/types/types.go) that this
// backend never assigns: every op here is synchronous, so there is no async
// provisioning/deletion phase for a resource to be PENDING or IN_PROGRESS
// in, and no partial-failure window for CREATE FAILED to represent honestly
// (gopherstack-h3th). This is the same deliberate skip-to-terminal-state
// pattern StopSolutionVersionCreation and StopRecommender/StartRecommender
// already use (jump straight to CREATE STOPPED / INACTIVE / ACTIVE, never
// through *_PENDING or *_IN_PROGRESS) -- see PARITY.md for the full
// per-constant reachability table. statusStopPending is Recommender-only
// ("STOP PENDING > STOP IN_PROGRESS > INACTIVE > START PENDING > START
// IN_PROGRESS > ACTIVE" per types.Recommender's doc comment);
// StopRecommender/StartRecommender (recommenders.go) already jump directly
// between ACTIVE and INACTIVE. Kept declared (not fabricated into use) for
// any op that gains a genuine async-equivalent failure path later.
const (
	statusActive         = "ACTIVE"
	statusCreatePending  = "CREATE PENDING"
	statusCreateProgress = "CREATE IN_PROGRESS"
	statusDeletePending  = "DELETE PENDING"
	statusCreateFailed   = "CREATE FAILED"
	// statusSolutionVersionStopped is the terminal status for a solution
	// version whose training was stopped via StopSolutionVersionCreation.
	// The SolutionVersion.Status wire enum only has "CREATE STOPPED" (no
	// bare "STOPPED") -- see the [SolutionVersion] type doc in
	// aws-sdk-go-v2/service/personalize/types.
	//
	// [SolutionVersion]: https://docs.aws.amazon.com/personalize/latest/dg/API_SolutionVersion.html
	statusSolutionVersionStopped = "CREATE STOPPED"
	statusStopPending            = "STOP PENDING"

	defaultAccountID = "000000000000"
	defaultRegion    = "us-east-1"

	mockMetricValue = 0.5
)

// InMemoryBackend stores Amazon Personalize state.
type InMemoryBackend struct {
	mu       *lockmetrics.RWMutex
	registry *store.Registry

	datasetGroups          *store.Table[DatasetGroup]
	datasets               *store.Table[Dataset]
	schemas                *store.Table[Schema]
	solutions              *store.Table[Solution]
	solutionVersions       *store.Table[SolutionVersion]
	campaigns              *store.Table[Campaign]
	datasetImportJobs      *store.Table[DatasetImportJob]
	datasetExportJobs      *store.Table[DatasetExportJob]
	batchInferenceJobs     *store.Table[BatchInferenceJob]
	batchSegmentJobs       *store.Table[BatchSegmentJob]
	eventTrackers          *store.Table[EventTracker]
	filters                *store.Table[Filter]
	recommenders           *store.Table[Recommender]
	metricAttributions     *store.Table[MetricAttribution]
	dataDeletionJobs       *store.Table[DataDeletionJob]
	featureTransformations *store.Table[storedFeatureTransformation]

	// tags is left as a plain map: its value type is map[string]string, not
	// *T, so it does not fit store.Table's keyed-by-identity-value shape.
	// See persistence.go.
	tags map[string]map[string]string

	accountID string
	region    string
}

// NewInMemoryBackend returns a stateful Amazon Personalize backend.
func NewInMemoryBackend(accountID, region string) *InMemoryBackend {
	if accountID == "" {
		accountID = defaultAccountID
	}
	if region == "" {
		region = defaultRegion
	}

	b := &InMemoryBackend{
		tags:      make(map[string]map[string]string),
		accountID: accountID,
		region:    region,
		mu:        lockmetrics.New("personalize"),
		registry:  store.NewRegistry(),
	}
	registerAllTables(b)
	b.seedBuiltinFeatureTransformations()

	return b
}

// Reset clears all in-memory Personalize state for the /_gopherstack/reset
// test hook so suites start from a clean slate.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	b.registry.ResetAll()
	b.tags = make(map[string]map[string]string)
	// featureTransformations are read-only builtins; re-seed to restore after
	// registry.ResetAll() cleared the table along with every other one.
	b.seedBuiltinFeatureTransformations()
}

// Region returns the configured region.
func (b *InMemoryBackend) Region() string { return b.region }

// AccountID returns the configured account ID.
func (b *InMemoryBackend) AccountID() string { return b.accountID }

func (b *InMemoryBackend) personalizeARN(resource, name string) string {
	return arn.Build("personalize", b.region, b.accountID, resource+"/"+name)
}

// --- Helpers ---

func copyStringMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	maps.Copy(out, m)

	return out
}

// paginateItems is the store.Table-backed counterpart of paginate: items must
// already be in the table's key-sorted order (as returned by
// [store.Table.Snapshot]), which paginateItems relies on for both the page
// slice and the nextToken continuation semantics. Every caller's keyOf is
// exactly the table's own (unique) primary-key function, so items is totally
// ordered by keyOf -- this is a threshold search: resume at the first item
// whose key is strictly greater than nextToken. A deleted or forged token
// then resumes past everything already served instead of restarting at page
// one, and a deleted item simply resumes at the next one.
func paginateItems[T any](items []*T, keyOf func(*T) string, maxResults int, nextToken string) ([]*T, string) {
	const defaultPageSize = 100

	if maxResults <= 0 {
		maxResults = defaultPageSize
	}

	start := 0
	if nextToken != "" {
		start = len(items)

		for i, v := range items {
			if keyOf(v) > nextToken {
				start = i

				break
			}
		}
	}

	end := start + maxResults
	var outToken string
	if end < len(items) {
		outToken = keyOf(items[end])
	} else {
		end = len(items)
	}

	return items[start:end], outToken
}

// paginate is used only by ListMetricAttributionMetrics, which pages over a
// synthetic, non-map-backed []map[string]any and so is out of scope for the
// store.Table conversion above. Its sole caller sorts keys ascending and
// unique before calling in, so this is a threshold search: resume at the
// first key strictly greater than nextToken. A deleted or forged token then
// resumes past everything already served instead of restarting at page one.
func paginate[T any](keys []string, get func(string) T, maxResults int, nextToken string) ([]T, string) {
	const defaultPageSize = 100

	if maxResults <= 0 {
		maxResults = defaultPageSize
	}

	start := 0
	if nextToken != "" {
		start = len(keys)

		for i, k := range keys {
			if k > nextToken {
				start = i

				break
			}
		}
	}

	end := start + maxResults
	var outToken string
	if end < len(keys) {
		outToken = keys[end]
	} else {
		end = len(keys)
	}

	items := make([]T, 0, end-start)
	for _, k := range keys[start:end] {
		items = append(items, get(k))
	}

	return items, outToken
}

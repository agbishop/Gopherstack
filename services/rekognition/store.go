package rekognition

import (
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// InMemoryBackend is an in-memory implementation of StorageBackend.
type InMemoryBackend struct {
	mu       *lockmetrics.RWMutex
	registry *store.Registry
	s3       S3Backend

	collections       *store.Table[storedCollection]
	faces             *store.Table[storedFace]
	facesByCollection *store.Index[storedFace]
	streamProcessors  *store.Table[storedStreamProcessor]

	// tags is left as a plain map: values are plain string maps, not *T, so
	// they do not fit store.Table's keyed-by-identity-value shape. See
	// persistence.go for how it is carried through Snapshot/Restore.
	tags map[string]map[string]string // resourceARN -> tags

	projects                 *store.Table[storedProject]
	projectVersions          *store.Table[storedProjectVersion]
	projectPolicies          *store.Table[storedProjectPolicy]
	projectPoliciesByProject *store.Index[storedProjectPolicy]
	datasets                 *store.Table[storedDataset]

	// datasetEntries is left as a plain map: values are []string, not *T, so
	// they do not fit store.Table's keyed-by-identity-value shape. See
	// persistence.go.
	datasetEntries map[string][]string // datasetArn -> jsonLines

	users             *store.Table[storedUser]
	usersByCollection *store.Index[storedUser]
	livenessSessions  *store.Table[storedLivenessSession]
	asyncJobs         *store.Table[storedAsyncJob]
	mediaAnalysisJobs *store.Table[storedMediaAnalysisJob]

	accountID string
	region    string
}

// NewInMemoryBackend creates a new in-memory backend.
func NewInMemoryBackend(accountID, region string) *InMemoryBackend {
	b := &InMemoryBackend{
		mu:             lockmetrics.New("rekognition"),
		registry:       store.NewRegistry(),
		accountID:      accountID,
		region:         region,
		tags:           make(map[string]map[string]string),
		datasetEntries: make(map[string][]string),
	}

	registerAllTables(b)

	return b
}

// SetS3Backend wires S3 so an Image/Video/Input S3Object is validated
// against real S3 state instead of only being stored/echoed.
func (b *InMemoryBackend) SetS3Backend(s3 S3Backend) {
	b.s3 = s3
}

// AccountID returns the account ID.
func (b *InMemoryBackend) AccountID() string { return b.accountID }

// Region returns the region.
func (b *InMemoryBackend) Region() string { return b.region }

// paginateTable pages over a store.Table[V]'s contents in ascending key order
// (see store.Table.Snapshot), converting each value via convert.
func paginateTable[V any, R any](
	t *store.Table[V],
	maxResults, maxPerPage int32,
	nextToken string,
	keyFn func(*V) string,
	convert func(*V) R,
) ([]R, string) {
	items := t.Snapshot()

	start := 0
	if nextToken != "" {
		for i, v := range items {
			if keyFn(v) == nextToken {
				start = i

				break
			}
		}
	}

	limit := maxPerPage
	if maxResults > 0 && maxResults < limit {
		limit = maxResults
	}

	end := min(start+int(limit), len(items))

	result := make([]R, 0, end-start)
	for _, v := range items[start:end] {
		result = append(result, convert(v))
	}

	var outToken string
	if end < len(items) {
		outToken = keyFn(items[end])
	}

	return result, outToken
}

// Reset clears all stored state.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	b.registry.ResetAll()
	b.tags = make(map[string]map[string]string)
	b.datasetEntries = make(map[string][]string)
}

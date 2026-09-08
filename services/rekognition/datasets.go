package rekognition

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (b *InMemoryBackend) datasetARN(projectARN, datasetType string) string {
	return fmt.Sprintf("%s/dataset/%s/%s", projectARN, datasetType, uuid.NewString())
}

// =============================================================================
// Datasets
// =============================================================================

// CreateDataset creates a new dataset. Real AWS rejects a second dataset of
// the same DatasetType for a project with ResourceAlreadyExistsException;
// datasetARN is always uuid-suffixed (so two datasets of the same type never
// collide on table key), so that check must be done explicitly here via a
// scan for an existing (ProjectARN, DatasetType) pair.
func (b *InMemoryBackend) CreateDataset(projectARN, datasetType string) (*Dataset, error) {
	b.mu.Lock("CreateDataset")
	defer b.mu.Unlock()

	if !b.projects.Has(projectARN) {
		return nil, ErrProjectNotFound
	}

	var duplicate bool

	b.datasets.Range(func(d *storedDataset) bool {
		if d.ProjectARN == projectARN && d.DatasetType == datasetType {
			duplicate = true

			return false
		}

		return true
	})

	if duplicate {
		return nil, ErrDatasetAlreadyExists
	}

	arn := b.datasetARN(projectARN, datasetType)
	now := time.Now()

	ds := &storedDataset{
		CreationTimestamp:    now,
		LastUpdatedTimestamp: now,
		DatasetARN:           arn,
		ProjectARN:           projectARN,
		DatasetType:          datasetType,
		Status:               "CREATE_COMPLETE",
	}
	b.datasets.Put(ds)

	return ds.toDataset(), nil
}

// DeleteDataset deletes a dataset. DeleteDatasetInput's own doc comment
// (api_op_DeleteDataset.go): "You can't delete a dataset while it is
// creating (Status = CREATE_IN_PROGRESS) or if the dataset is updating
// (Status = UPDATE_IN_PROGRESS).".
func (b *InMemoryBackend) DeleteDataset(datasetARN string) error {
	b.mu.Lock("DeleteDataset")
	defer b.mu.Unlock()

	ds, exists := b.datasets.Get(datasetARN)
	if !exists {
		return ErrDatasetNotFound
	}

	if ds.Status == "CREATE_IN_PROGRESS" || ds.Status == "UPDATE_IN_PROGRESS" {
		return ErrDatasetInUse
	}

	b.datasets.Delete(datasetARN)
	delete(b.datasetEntries, datasetARN)

	return nil
}

// DescribeDataset returns details about a dataset.
func (b *InMemoryBackend) DescribeDataset(datasetARN string) (*Dataset, error) {
	b.mu.RLock("DescribeDataset")
	defer b.mu.RUnlock()

	ds, exists := b.datasets.Get(datasetARN)
	if !exists {
		return nil, ErrDatasetNotFound
	}

	result := ds.toDataset()
	result.Stats = computeDatasetStats(b.datasetEntries[datasetARN])

	return result, nil
}

// ListDatasetEntriesFilter groups ListDatasetEntriesInput's optional filter
// members (own doc comments, api_op_ListDatasetEntries.go). HasErrors has no
// effect beyond excluding everything when true: this backend has no
// entry-level error concept (see computeDatasetStats' ErrorEntries note), so
// no entry can ever satisfy it.
type ListDatasetEntriesFilter struct {
	HasErrors         *bool
	Labeled           *bool
	SourceRefContains string
	ContainsLabels    []string
}

func matchesDatasetEntryFilter(entry string, filter ListDatasetEntriesFilter) bool {
	if filter.HasErrors != nil && *filter.HasErrors {
		return false
	}

	names, labeled := entryLabels(entry)

	if filter.Labeled != nil && *filter.Labeled != labeled {
		return false
	}

	if len(filter.ContainsLabels) > 0 && !slices.ContainsFunc(filter.ContainsLabels, func(want string) bool {
		return slices.Contains(names, want)
	}) {
		return false
	}

	if filter.SourceRefContains != "" && !strings.Contains(entrySourceRef(entry), filter.SourceRefContains) {
		return false
	}

	return true
}

// entryLabels parses one JSON-lines dataset entry and returns every label
// name found across its "-metadata" blocks, plus whether it carried at
// least one such block (i.e. is "labeled").
func entryLabels(entry string) ([]string, bool) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(entry), &obj); err != nil {
		return nil, false
	}

	var names []string

	labeled := false

	for key, val := range obj {
		const metaSuffix = "-metadata"
		if len(key) < len(metaSuffix) || key[len(key)-len(metaSuffix):] != metaSuffix {
			continue
		}

		labeled = true
		names = append(names, labelNamesFromMeta(val)...)
	}

	return names, labeled
}

// entrySourceRef parses one JSON-lines dataset entry and returns its
// "source-ref" field (the image's S3 location), or "" if absent/malformed.
func entrySourceRef(entry string) string {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(entry), &obj); err != nil {
		return ""
	}

	raw, ok := obj["source-ref"]
	if !ok {
		return ""
	}

	var ref string
	_ = json.Unmarshal(raw, &ref)

	return ref
}

// ListDatasetEntries returns a paginated list of dataset entries, optionally
// constrained by filter.
func (b *InMemoryBackend) ListDatasetEntries(
	datasetARN string, filter ListDatasetEntriesFilter, maxResults int32, nextToken string,
) ([]string, string, error) {
	b.mu.RLock("ListDatasetEntries")
	defer b.mu.RUnlock()

	if !b.datasets.Has(datasetARN) {
		return nil, "", ErrDatasetNotFound
	}

	var entries []string

	for _, e := range b.datasetEntries[datasetARN] {
		if matchesDatasetEntryFilter(e, filter) {
			entries = append(entries, e)
		}
	}

	start := 0
	if nextToken != "" {
		for i, e := range entries {
			if e == nextToken {
				start = i

				break
			}
		}
	}

	const maxPerPage = 100
	limit := int32(maxPerPage)
	if maxResults > 0 && maxResults < limit {
		limit = maxResults
	}

	end := min(start+int(limit), len(entries))

	result := make([]string, end-start)
	copy(result, entries[start:end])

	var outToken string
	if end < len(entries) {
		outToken = entries[end]
	}

	return result, outToken, nil
}

// datasetPaginationToken is the opaque pagination cursor for dataset label listing.
type datasetPaginationToken struct {
	Offset int `json:"o"`
}

// countLabelsFromEntry parses one JSON-lines entry and accumulates label
// counts, returning whether the entry carried at least one -metadata block
// (i.e. is "labeled" for DatasetStats.LabeledEntries purposes).
func countLabelsFromEntry(entry string, counts map[string]int64) bool {
	names, labeled := entryLabels(entry)
	for _, n := range names {
		counts[n]++
	}

	return labeled
}

// computeDatasetStats mirrors types.DatasetStats: TotalEntries/
// LabeledEntries/TotalLabels are derived from the dataset's stored
// manifest entries; ErrorEntries is always 0 (this backend has no
// entry-level error concept, so 0 is accurate, not fabricated).
func computeDatasetStats(entries []string) DatasetStats {
	counts := make(map[string]int64)

	var labeled int64

	for _, entry := range entries {
		if countLabelsFromEntry(entry, counts) {
			labeled++
		}
	}

	return DatasetStats{
		TotalEntries:   int64(len(entries)),
		LabeledEntries: labeled,
		TotalLabels:    int64(len(counts)),
	}
}

// labelNamesFromMeta parses a -metadata block and returns its label names.
func labelNamesFromMeta(raw json.RawMessage) []string {
	var meta map[string]json.RawMessage
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil
	}

	var names []string

	// Single-label: "class-name"
	if cn, ok := meta["class-name"]; ok {
		var name string
		if err := json.Unmarshal(cn, &name); err == nil && name != "" {
			names = append(names, name)
		}
	}

	// Multi-label: "class-map": {"LabelA": ..., "LabelB": ...}
	if cm, ok := meta["class-map"]; ok {
		var classMap map[string]json.RawMessage
		if err := json.Unmarshal(cm, &classMap); err == nil {
			for name := range classMap {
				names = append(names, name)
			}
		}
	}

	return names
}

// decodeDatasetPageToken decodes an opaque pagination token into an offset.
func decodeDatasetPageToken(token string) int {
	if token == "" {
		return 0
	}

	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return 0
	}

	var tok datasetPaginationToken
	if err = json.Unmarshal(decoded, &tok); err != nil || tok.Offset <= 0 {
		return 0
	}

	return tok.Offset
}

// ListDatasetLabels parses stored dataset entries and returns labels with occurrence counts.
func (b *InMemoryBackend) ListDatasetLabels(
	datasetARN string, maxResults int32, nextToken string,
) ([]*DatasetLabel, string, error) {
	b.mu.RLock("ListDatasetLabels")

	if !b.datasets.Has(datasetARN) {
		b.mu.RUnlock()

		return nil, "", ErrDatasetNotFound
	}

	// Clone entries under lock.
	src := b.datasetEntries[datasetARN]
	entries := make([]string, len(src))
	copy(entries, src)

	b.mu.RUnlock()

	// Parse entries and count label occurrences (best-effort).
	counts := make(map[string]int64)
	for _, entry := range entries {
		countLabelsFromEntry(entry, counts)
	}

	// Sort by label name.
	names := make([]string, 0, len(counts))
	for n := range counts {
		names = append(names, n)
	}

	sort.Strings(names)

	start := decodeDatasetPageToken(nextToken)

	const maxPerPage = 100
	limit := int(maxPerPage)

	if maxResults > 0 && int(maxResults) < limit {
		limit = int(maxResults)
	}

	end := min(start+limit, len(names))

	result := make([]*DatasetLabel, 0, end-start)
	for _, n := range names[start:end] {
		result = append(result, &DatasetLabel{
			LabelName:  n,
			EntryCount: counts[n],
		})
	}

	var outToken string

	if end < len(names) {
		tok, _ := json.Marshal(datasetPaginationToken{Offset: end})
		outToken = base64.RawURLEncoding.EncodeToString(tok)
	}

	return result, outToken, nil
}

// UpdateDatasetEntries appends changes to dataset entries.
func (b *InMemoryBackend) UpdateDatasetEntries(datasetARN string, changes []byte) error {
	b.mu.Lock("UpdateDatasetEntries")
	defer b.mu.Unlock()

	if !b.datasets.Has(datasetARN) {
		return ErrDatasetNotFound
	}

	b.datasetEntries[datasetARN] = append(b.datasetEntries[datasetARN], string(changes))

	return nil
}

// DistributeDatasetEntries validates datasets and marks them as UPDATE_IN_PROGRESS.
func (b *InMemoryBackend) DistributeDatasetEntries(datasets []DatasetDistribution) error {
	b.mu.Lock("DistributeDatasetEntries")
	defer b.mu.Unlock()

	for _, d := range datasets {
		ds, ok := b.datasets.Get(d.DatasetARN)
		if !ok {
			return ErrDatasetNotFound
		}

		ds.Status = "UPDATE_IN_PROGRESS"
		ds.LastUpdatedTimestamp = time.Now()
	}

	return nil
}

package opensearch

import (
	"fmt"
	"maps"
	"sort"
	"strconv"
	"strings"
)

// Document search bounds. The engine is deliberately small and honest: it stores
// real documents and answers match_all / term / match queries over them, but it
// is not a full Lucene engine.
const (
	// defaultSearchSize is the number of hits returned when a search omits size.
	defaultSearchSize = 10
	// maxSearchSize caps the number of hits a single search may return.
	maxSearchSize = 1000
)

// SearchHit is a single document returned by a search.
type SearchHit struct {
	Source map[string]any `json:"_source"`
	ID     string         `json:"_id"`
	Index  string         `json:"_index"`
}

// SearchResult is the outcome of a bounded search over an index.
type SearchResult struct {
	Hits  []SearchHit
	Total int
}

// findIndexLocked returns the stored index for a domain, or an error. The caller
// must hold at least a read lock. action is the IAM-style action being
// performed against the index (e.g. "es:ESHttpGet"), checked against the
// domain's own resource-based AccessPolicies -- see accessPolicyDenies.
func (b *InMemoryBackend) findIndexLocked(domainName, indexName, action string) (*DomainIndex, error) {
	d, ok := b.domains.Get(domainName)
	if !ok || deleteWindowElapsed(d, b.clock()) {
		return nil, fmt.Errorf("%w: domain %s not found", ErrDomainNotFound, domainName)
	}

	if accessPolicyDenies(d.AccessPolicies, action, d.ARN) {
		return nil, fmt.Errorf(
			"%w: user is not authorized to perform %s on domain %s",
			ErrAccessDenied,
			action,
			domainName,
		)
	}

	idx, ok := b.domainIndexes.Get(domainIndexKey(domainName, indexName))
	if !ok {
		return nil, fmt.Errorf(
			"%w: index %s not found on domain %s",
			ErrConnectionNotFound,
			indexName,
			domainName,
		)
	}

	return idx, nil
}

// IndexDocument stores (or replaces) a document in an index and keeps the index
// document count accurate. When docID is empty a fresh ID is generated. It
// returns the effective document ID and whether a new document was created.
func (b *InMemoryBackend) IndexDocument(
	domainName, indexName, docID string,
	doc map[string]any,
) (string, bool, error) {
	b.mu.Lock("IndexDocument")
	defer b.mu.Unlock()

	idx, err := b.findIndexLocked(domainName, indexName, actionESHttpPut)
	if err != nil {
		return "", false, err
	}

	if idx.Documents == nil {
		idx.Documents = make(map[string]map[string]any)
	}

	if docID == "" {
		b.docCounter++
		docID = fmt.Sprintf("doc-%d", b.docCounter)
	}

	_, existed := idx.Documents[docID]
	idx.Documents[docID] = cloneDoc(doc)
	idx.DocumentCount = len(idx.Documents)

	return docID, !existed, nil
}

// GetDocument returns a stored document by ID.
func (b *InMemoryBackend) GetDocument(
	domainName, indexName, docID string,
) (map[string]any, error) {
	b.mu.RLock("GetDocument")
	defer b.mu.RUnlock()

	idx, err := b.findIndexLocked(domainName, indexName, actionESHttpGet)
	if err != nil {
		return nil, err
	}

	doc, ok := idx.Documents[docID]
	if !ok {
		return nil, fmt.Errorf(
			"%w: document %s not found in index %s",
			ErrConnectionNotFound,
			docID,
			indexName,
		)
	}

	return cloneDoc(doc), nil
}

// DeleteDocument removes a document by ID and updates the document count.
func (b *InMemoryBackend) DeleteDocument(domainName, indexName, docID string) error {
	b.mu.Lock("DeleteDocument")
	defer b.mu.Unlock()

	idx, err := b.findIndexLocked(domainName, indexName, actionESHttpDelete)
	if err != nil {
		return err
	}

	if _, ok := idx.Documents[docID]; !ok {
		return fmt.Errorf(
			"%w: document %s not found in index %s",
			ErrConnectionNotFound,
			docID,
			indexName,
		)
	}

	delete(idx.Documents, docID)
	idx.DocumentCount = len(idx.Documents)

	return nil
}

// CountDocuments returns the number of documents stored in an index.
func (b *InMemoryBackend) CountDocuments(domainName, indexName string) (int, error) {
	b.mu.RLock("CountDocuments")
	defer b.mu.RUnlock()

	idx, err := b.findIndexLocked(domainName, indexName, actionESHttpGet)
	if err != nil {
		return 0, err
	}

	return idx.DocumentCount, nil
}

// DomainDocumentCount returns the total number of documents stored across all
// indexes of a domain.
func (b *InMemoryBackend) DomainDocumentCount(domainName string) int {
	b.mu.RLock("DomainDocumentCount")
	defer b.mu.RUnlock()

	total := 0
	for _, idx := range b.domainIndexesByDomain.Get(domainName) {
		total += idx.DocumentCount
	}

	return total
}

// SearchIndex runs a bounded search over the stored documents of an index.
//
// Supported queries: an empty query or {"match_all": {}} returns every document;
// {"term": {field: value}} returns documents whose field equals value exactly;
// {"match": {field: text}} returns documents whose field contains text
// (case-insensitive substring). Hits are ordered by document ID for
// determinism. size defaults to 10 and is capped at maxSearchSize.
func (b *InMemoryBackend) SearchIndex(
	domainName, indexName string,
	query map[string]any,
	size int,
) (*SearchResult, error) {
	b.mu.RLock("SearchIndex")
	defer b.mu.RUnlock()

	idx, err := b.findIndexLocked(domainName, indexName, actionESHttpPost)
	if err != nil {
		return nil, err
	}

	pred, err := compileQuery(query)
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(idx.Documents))
	for id := range idx.Documents {
		ids = append(ids, id)
	}

	sort.Strings(ids)

	if size <= 0 {
		size = defaultSearchSize
	}

	if size > maxSearchSize {
		size = maxSearchSize
	}

	result := &SearchResult{Hits: []SearchHit{}}

	for _, id := range ids {
		doc := idx.Documents[id]
		if !pred(doc) {
			continue
		}

		result.Total++

		if len(result.Hits) < size {
			result.Hits = append(result.Hits, SearchHit{
				ID:     id,
				Index:  indexName,
				Source: cloneDoc(doc),
			})
		}
	}

	return result, nil
}

// docPredicate reports whether a document matches a query.
type docPredicate func(doc map[string]any) bool

// compileQuery translates a query map into a predicate. It returns a
// ValidationException for unsupported query clauses so callers surface the
// AWS-accurate error shape rather than silently matching everything.
func compileQuery(query map[string]any) (docPredicate, error) {
	if len(query) == 0 {
		return matchAll, nil
	}

	if _, ok := query["match_all"]; ok {
		return matchAll, nil
	}

	if raw, ok := query["term"]; ok {
		field, want, perr := singleFieldClause(raw)
		if perr != nil {
			return nil, perr
		}

		return func(doc map[string]any) bool {
			return termEqual(doc[field], want)
		}, nil
	}

	if raw, ok := query["match"]; ok {
		field, want, perr := singleFieldClause(raw)
		if perr != nil {
			return nil, perr
		}

		needle := strings.ToLower(stringify(want))

		return func(doc map[string]any) bool {
			return strings.Contains(strings.ToLower(stringify(doc[field])), needle)
		}, nil
	}

	return nil, fmt.Errorf("%w: unsupported query clause", ErrValidation)
}

// matchAll matches every document.
func matchAll(map[string]any) bool { return true }

// singleFieldClause extracts the single field/value pair from a term/match clause.
func singleFieldClause(raw any) (string, any, error) {
	clause, ok := raw.(map[string]any)
	if !ok || len(clause) != 1 {
		return "", nil, fmt.Errorf("%w: query clause must specify exactly one field", ErrValidation)
	}

	for field, want := range clause {
		return field, want, nil
	}

	return "", nil, fmt.Errorf("%w: query clause must specify exactly one field", ErrValidation)
}

// termEqual reports exact equality between a stored value and a query value,
// comparing by normalised string form so numeric JSON types compare cleanly.
func termEqual(have, want any) bool {
	return stringify(have) == stringify(want)
}

// stringify renders a JSON-decoded scalar to a stable string form.
func stringify(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	default:
		return fmt.Sprintf("%v", t)
	}
}

// cloneDoc returns a shallow copy of a document so stored state cannot be
// mutated through a returned reference.
func cloneDoc(doc map[string]any) map[string]any {
	if doc == nil {
		return map[string]any{}
	}

	out := make(map[string]any, len(doc))
	maps.Copy(out, doc)

	return out
}

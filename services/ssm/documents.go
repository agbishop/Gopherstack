package ssm

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// attachmentsInformation projects CreateDocument/UpdateDocument's Attachments
// source list down to the Name-only shape real AWS returns in
// DocumentDescription.AttachmentsInformation (types.AttachmentInformation).
func attachmentsInformation(sources []AttachmentsSource) []AttachmentInformation {
	if len(sources) == 0 {
		return nil
	}

	out := make([]AttachmentInformation, 0, len(sources))
	for _, s := range sources {
		out = append(out, AttachmentInformation{Name: s.Name})
	}

	return out
}

func (b *InMemoryBackend) documentsStore(region string) *store.Table[Document] {
	return getOrCreateTable(b, b.documents, "documents", region, documentKeyFn)
}
func (b *InMemoryBackend) documentVersionsStore(region string) map[string][]DocumentVersion {
	return b.documentVersions[region]
}
func (b *InMemoryBackend) documentPermissionsStore(region string) map[string][]string {
	return b.documentPermissions[region]
}

// documentSharedVersionsStore returns the per-document, per-account
// SharedDocumentVersion pins for region (document name -> account ID ->
// pinned version), a companion to documentPermissionsStore's plain account-ID
// list. Kept as a separate additive map rather than reshaping
// documentPermissions itself, so restoring an older snapshot (which has no
// such pins) stays a safe, purely additive zero-value default instead of
// requiring an incompatible ssmSnapshotVersion bump (gopherstack-5i6p).
func (b *InMemoryBackend) documentSharedVersionsStore(region string) map[string]map[string]string {
	return b.documentSharedVersions[region]
}

// registerDefaultDocuments pre-registers the built-in AWS documents.
func (b *InMemoryBackend) registerDefaultDocuments(region string) {
	now := UnixTimeFloat(time.Now())
	defaults := []struct {
		name     string
		docType  string
		content  string
		platform []string
	}{
		{
			name:    "AWS-RunShellScript",
			docType: DocumentTypeCommand,
			content: `{"schemaVersion":"2.2","description":"Run shell script",` +
				`"parameters":{"commands":{"type":"StringList"}},` +
				`"mainSteps":[{"action":"aws:runShellScript","name":"runShellScript",` +
				`"inputs":{"commands":["{{ commands }}"]}}]}`,
			platform: []string{"Linux"},
		},
		{
			name:    "AWS-RunPowerShellScript",
			docType: DocumentTypeCommand,
			content: `{"schemaVersion":"2.2","description":"Run PowerShell script",` +
				`"parameters":{"commands":{"type":"StringList"}},` +
				`"mainSteps":[{"action":"aws:runPowerShellScript","name":"runPowerShellScript",` +
				`"inputs":{"commands":["{{ commands }}"]}}]}`,
			platform: []string{"Windows"},
		},
	}

	if b.documentVersions[region] == nil {
		b.documentVersions[region] = make(map[string][]DocumentVersion)
	}
	documents := b.documentsStore(region)
	documentVersions := b.documentVersionsStore(region)

	for _, d := range defaults {
		doc := Document{
			Name:            d.name,
			Content:         d.content,
			DocumentType:    d.docType,
			DocumentFormat:  documentFormatJSON,
			Status:          statusActive,
			SchemaVersion:   "2.2",
			PlatformTypes:   d.platform,
			CreatedDate:     now,
			DocumentVersion: "1",
			LatestVersion:   "1",
			DefaultVersion:  "1",
		}
		documents.Put(&doc)
		documentVersions[d.name] = []DocumentVersion{
			{
				Name:             d.name,
				DocumentVersion:  "1",
				CreatedDate:      now,
				IsDefaultVersion: true,
				DocumentFormat:   documentFormatJSON,
				Status:           statusActive,
				Content:          d.content,
			},
		}
	}
}

const defaultListDocMaxResults = 50

// CreateDocument stores a new SSM document.
func (b *InMemoryBackend) CreateDocument(
	ctx context.Context,
	input *CreateDocumentInput,
) (*CreateDocumentOutput, error) {
	region := getRegion(ctx)
	b.mu.Lock("CreateDocument")
	defer b.mu.Unlock()

	documentsTable := b.documentsStore(region)
	if documentsTable.Has(input.Name) {
		return nil, ErrDocumentAlreadyExists
	}

	format := input.DocumentFormat
	if format == "" {
		format = documentFormatJSON
	}

	docType := input.DocumentType
	if docType == "" {
		docType = DocumentTypeCommand
	}

	now := UnixTimeFloat(time.Now())
	hash, sha1Hex := documentHashes(input.Content)
	doc := Document{
		Name:                   input.Name,
		DisplayName:            input.DisplayName,
		Content:                input.Content,
		DocumentType:           docType,
		DocumentFormat:         format,
		Status:                 statusActive,
		TargetType:             input.TargetType,
		Description:            input.Description,
		PlatformTypes:          input.PlatformTypes,
		SchemaVersion:          "2.2",
		CreatedDate:            now,
		DocumentVersion:        "1",
		LatestVersion:          "1",
		DefaultVersion:         "1",
		Requires:               input.Requires,
		Hash:                   hash,
		HashType:               documentHashTypeSha256,
		Sha1:                   sha1Hex,
		AttachmentsInformation: attachmentsInformation(input.Attachments),
	}

	documentsTable.Put(&doc)
	if b.documentVersions[region] == nil {
		b.documentVersions[region] = make(map[string][]DocumentVersion)
	}
	versionStore := b.documentVersionsStore(region)
	versionStore[input.Name] = []DocumentVersion{
		{
			Name:             input.Name,
			DisplayName:      input.DisplayName,
			DocumentVersion:  "1",
			CreatedDate:      now,
			IsDefaultVersion: true,
			DocumentFormat:   format,
			Status:           statusActive,
			Content:          input.Content,
		},
	}

	if len(input.Tags) > 0 {
		if b.miscResourceTags[region] == nil {
			b.miscResourceTags[region] = make(map[string]map[string]string)
		}
		miscTags := b.miscResourceTagsStore(region)
		if miscTags[input.Name] == nil {
			miscTags[input.Name] = make(map[string]string)
		}
		for _, t := range input.Tags {
			miscTags[input.Name][t.Key] = t.Value
		}
	}

	return &CreateDocumentOutput{
		DocumentDescription: doc.asDocumentDescription(b.miscResourceTagList(region, doc.Name)),
	}, nil
}

// asDocumentDescription converts an internal Document to the wire-accurate DocumentDescription shape
// returned by CreateDocument/UpdateDocument/DescribeDocument. Real AWS never
// includes Content in these metadata responses.
func (d Document) asDocumentDescription(docTags []Tag) DocumentDescription {
	return DocumentDescription{
		TargetType:             d.TargetType,
		LatestVersion:          d.LatestVersion,
		DocumentType:           d.DocumentType,
		DocumentFormat:         d.DocumentFormat,
		Status:                 d.Status,
		StatusInformation:      d.StatusInformation,
		DefaultVersion:         d.DefaultVersion,
		Name:                   d.Name,
		DisplayName:            d.DisplayName,
		SchemaVersion:          d.SchemaVersion,
		Description:            d.Description,
		DocumentVersion:        d.DocumentVersion,
		Hash:                   d.Hash,
		HashType:               d.HashType,
		Sha1:                   d.Sha1,
		PlatformTypes:          d.PlatformTypes,
		AttachmentsInformation: d.AttachmentsInformation,
		Requires:               d.Requires,
		Tags:                   docTags,
		CreatedDate:            d.CreatedDate,
	}
}

// resolveDocumentVersionSelector resolves the "$LATEST"/"$DEFAULT" selectors
// to a concrete version string. An explicit "$DEFAULT" always resolves to
// the document's DefaultVersion (set by UpdateDocumentDefaultVersion), which
// can genuinely differ from LatestVersion — this emulator previously
// conflated the two, always serving the latest content even when $DEFAULT
// was explicitly requested. An omitted DocumentVersion is treated the same
// as this emulator has always treated it (latest), since AWS's own docs
// don't state a default and existing callers depend on that behavior.
func resolveDocumentVersionSelector(doc Document, requested string) string {
	switch requested {
	case "":
		return doc.LatestVersion
	case "$DEFAULT":
		return doc.DefaultVersion
	case "$LATEST":
		return doc.LatestVersion
	default:
		return requested
	}
}

// evictOldestDocumentVersions trims vers (oldest-first, insertion order) down
// to at most maxCap entries, evicting the oldest first — except the version
// currently pinned as the document's DefaultVersion, which is never evicted.
//
// Without this guard, a long-lived document (1000+ UpdateDocument calls after
// UpdateDocumentDefaultVersion pinned an old version) could silently evict
// the very version $DEFAULT points at: GetDocument/DescribeDocument would
// then return ErrInvalidDocumentVersion for an explicit or omitted $DEFAULT
// selector instead of resolving it, orphaning the pointer. Fixes bd
// gopherstack-1hg. Mirrors PutParameter's labeled-parameter-version eviction
// guard (parameters.go) — same "never silently destroy a version a caller
// has pinned a reference to" principle, applied to documents. When the
// protected version happens to be among the oldest, the store may retain one
// extra entry beyond maxCap; that is the accepted tradeoff for never
// orphaning $DEFAULT.
func evictOldestDocumentVersions(vers []DocumentVersion, defaultVersion string, maxCap int) []DocumentVersion {
	if len(vers) <= maxCap {
		return vers
	}

	excess := len(vers) - maxCap
	kept := make([]DocumentVersion, 0, len(vers))
	evicted := 0

	for _, v := range vers {
		if evicted < excess && v.DocumentVersion != defaultVersion {
			evicted++

			continue
		}

		kept = append(kept, v)
	}

	return kept
}

// associationDocumentContent resolves an association document's body for
// DescribeEffectiveInstanceAssociations, matching real
// InstanceAssociation.Content (ssm@v1.73.4 types.go). Caller must already
// hold b.mu. Returns "" if the document (or the specific version) no longer
// exists -- an association can outlive the document it was created against.
func (b *InMemoryBackend) associationDocumentContent(region, docName, docVersion string) string {
	docPtr, ok := b.documentsStore(region).Get(docName)
	if !ok {
		return ""
	}

	target := resolveDocumentVersionSelector(*docPtr, docVersion)
	for _, v := range b.documentVersionsStore(region)[docName] {
		if v.DocumentVersion == target {
			return v.Content
		}
	}

	return ""
}

// GetDocument retrieves a document's content.
func (b *InMemoryBackend) GetDocument(
	ctx context.Context,
	input *GetDocumentInput,
) (*GetDocumentOutput, error) {
	region := getRegion(ctx)
	b.mu.RLock("GetDocument")
	defer b.mu.RUnlock()

	docPtr, exists := b.documentsStore(region).Get(input.Name)
	if !exists {
		return nil, ErrDocumentNotFound
	}

	doc := *docPtr

	target := resolveDocumentVersionSelector(doc, input.DocumentVersion)

	versions := b.documentVersionsStore(region)[input.Name]
	for _, v := range versions {
		if v.DocumentVersion != target {
			continue
		}

		return &GetDocumentOutput{
			Name:              doc.Name,
			Content:           v.Content,
			DisplayName:       doc.DisplayName,
			DocumentType:      doc.DocumentType,
			DocumentFormat:    v.DocumentFormat,
			DocumentVersion:   v.DocumentVersion,
			Status:            v.Status,
			StatusInformation: doc.StatusInformation,
			Requires:          doc.Requires,
			CreatedDate:       v.CreatedDate,
		}, nil
	}

	return nil, ErrInvalidDocumentVersion
}

// documentMatchesFilters returns true when doc satisfies all provided DocumentFilters
// (types.DocumentKeyValuesFilter, api_op_ListDocuments.go: "valid keys include Owner,
// Name, PlatformTypes, DocumentType, and TargetType"). Owner ("Self" vs. other
// accounts) and tag:tagName keys aren't modeled -- there's no document-ownership or
// tag-key data to filter on -- and fall through to unfiltered, matching this backend's
// established unknown-key convention (matchesActivationFilter).
func documentMatchesFilters(doc Document, filters []DocumentFilter) bool {
	for _, f := range filters {
		switch f.Key {
		case "DocumentType":
			if !slices.Contains(f.Values, doc.DocumentType) {
				return false
			}
		case filterKeyName:
			if !slices.Contains(f.Values, doc.Name) {
				return false
			}
		case "TargetType":
			if !slices.Contains(f.Values, doc.TargetType) {
				return false
			}
		case "PlatformTypes":
			if !slices.ContainsFunc(doc.PlatformTypes, func(p string) bool {
				return slices.Contains(f.Values, p)
			}) {
				return false
			}
		default:
			continue
		}
	}

	return true
}

// DescribeDocument returns document metadata.
func (b *InMemoryBackend) DescribeDocument(
	ctx context.Context,
	input *DescribeDocumentInput,
) (*DescribeDocumentOutput, error) {
	region := getRegion(ctx)
	b.mu.RLock("DescribeDocument")
	defer b.mu.RUnlock()

	docPtr, exists := b.documentsStore(region).Get(input.Name)
	if !exists {
		return nil, ErrDocumentNotFound
	}

	doc := *docPtr

	description := doc.asDocumentDescription(b.miscResourceTagList(region, doc.Name))

	// Honor a specific/$LATEST/$DEFAULT DocumentVersion selector: the
	// per-version fields (DocumentVersion, DocumentFormat, Status) must
	// reflect the resolved version, not always the latest.
	target := resolveDocumentVersionSelector(doc, input.DocumentVersion)
	if target != doc.DocumentVersion {
		found := false

		for _, v := range b.documentVersionsStore(region)[input.Name] {
			if v.DocumentVersion == target {
				description.DocumentVersion = v.DocumentVersion
				description.DocumentFormat = v.DocumentFormat
				description.Status = v.Status
				description.DisplayName = v.DisplayName
				description.Hash, description.Sha1 = documentHashes(v.Content)
				description.HashType = documentHashTypeSha256
				found = true

				break
			}
		}

		if !found {
			return nil, ErrInvalidDocumentVersion
		}
	}

	return &DescribeDocumentOutput{Document: description}, nil
}

// ListDocuments returns a list of document identifiers filtered by key-value criteria.
func (b *InMemoryBackend) ListDocuments(
	ctx context.Context,
	input *ListDocumentsInput,
) (*ListDocumentsOutput, error) {
	region := getRegion(ctx)
	b.mu.RLock("ListDocuments")
	defer b.mu.RUnlock()

	// Merge Filters and DocumentFilters (both carry the same shape).
	allFilters := make([]DocumentFilter, 0, len(input.Filters)+len(input.DocumentFilters))
	allFilters = append(allFilters, input.Filters...)
	allFilters = append(allFilters, input.DocumentFilters...)

	docsTable := b.documentsStore(region)
	all := make([]DocumentIdentifier, 0, docsTable.Len())
	for _, docPtr := range docsTable.All() {
		doc := *docPtr
		if !documentMatchesFilters(doc, allFilters) {
			continue
		}

		all = append(all, DocumentIdentifier{
			Name:            doc.Name,
			DisplayName:     doc.DisplayName,
			DocumentType:    doc.DocumentType,
			DocumentFormat:  doc.DocumentFormat,
			DocumentVersion: doc.DocumentVersion,
			SchemaVersion:   doc.SchemaVersion,
			PlatformTypes:   doc.PlatformTypes,
			Requires:        doc.Requires,
			Tags:            b.miscResourceTagList(region, doc.Name),
			TargetType:      doc.TargetType,
			CreatedDate:     doc.CreatedDate,
		})
	}

	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })

	startIdx := parseNextToken(input.NextToken)

	maxResults := int64(defaultListDocMaxResults)
	if input.MaxResults != nil && *input.MaxResults > 0 {
		maxResults = *input.MaxResults
	}

	if startIdx >= len(all) {
		return &ListDocumentsOutput{DocumentIdentifiers: []DocumentIdentifier{}}, nil
	}

	end := startIdx + int(maxResults)

	var nextToken string

	if end < len(all) {
		nextToken = strconv.Itoa(end)
	} else {
		end = len(all)
	}

	return &ListDocumentsOutput{
		DocumentIdentifiers: all[startIdx:end],
		NextToken:           nextToken,
	}, nil
}

// UpdateDocument increments the document version and updates content.
func (b *InMemoryBackend) UpdateDocument(
	ctx context.Context,
	input *UpdateDocumentInput,
) (*UpdateDocumentOutput, error) {
	region := getRegion(ctx)
	b.mu.Lock("UpdateDocument")
	defer b.mu.Unlock()

	docsTable := b.documentsStore(region)
	docPtr, exists := docsTable.Get(input.Name)
	if !exists {
		return nil, ErrDocumentNotFound
	}

	doc := *docPtr

	// Validate DocumentVersion if provided.
	if input.DocumentVersion != "" {
		switch input.DocumentVersion {
		case "$LATEST", "$DEFAULT", doc.LatestVersion:
			// accepted versions
		default:
			return nil, ErrInvalidDocumentVersion
		}
	}

	latestVer, _ := strconv.Atoi(doc.LatestVersion)
	newVer := strconv.Itoa(latestVer + 1)

	format := input.DocumentFormat
	if format == "" {
		format = doc.DocumentFormat
	}

	now := UnixTimeFloat(time.Now())
	hash, sha1Hex := documentHashes(input.Content)
	doc.Content = input.Content
	doc.DocumentVersion = newVer
	doc.LatestVersion = newVer
	doc.DocumentFormat = format
	doc.Hash = hash
	doc.HashType = documentHashTypeSha256
	doc.Sha1 = sha1Hex

	if input.DisplayName != "" {
		doc.DisplayName = input.DisplayName
	}

	if input.TargetType != "" {
		doc.TargetType = input.TargetType
	}

	if input.Attachments != nil {
		doc.AttachmentsInformation = attachmentsInformation(input.Attachments)
	}

	docsTable.Put(&doc)

	versionStore := b.documentVersionsStore(region)
	versionStore[input.Name] = append(versionStore[input.Name], DocumentVersion{
		Name:             input.Name,
		DisplayName:      doc.DisplayName,
		DocumentVersion:  newVer,
		CreatedDate:      now,
		IsDefaultVersion: false,
		DocumentFormat:   format,
		Status:           statusActive,
		Content:          input.Content,
	})

	if len(versionStore[input.Name]) > maxDocumentVersionCap {
		versionStore[input.Name] = evictOldestDocumentVersions(
			versionStore[input.Name], doc.DefaultVersion, maxDocumentVersionCap,
		)
	}

	return &UpdateDocumentOutput{
		DocumentDescription: doc.asDocumentDescription(b.miscResourceTagList(region, doc.Name)),
	}, nil
}

// deleteDocumentVersionScoped removes a single version from doc, leaving the
// document and its other versions intact. Called only when versions has more
// than one entry -- the caller falls back to a full delete when the targeted
// version is the last one remaining, matching "if not provided, all versions
// of the document are deleted" (api_op_DeleteDocument.go:34-49) applied to
// the degenerate one-version case.
func (b *InMemoryBackend) deleteDocumentVersionScoped(
	region string, doc Document, versions []DocumentVersion, idx int,
) *DeleteDocumentOutput {
	removed := versions[idx]
	versions = slices.Delete(versions, idx, idx+1)

	if removed.DocumentVersion == doc.LatestVersion {
		newLatest := versions[len(versions)-1]
		doc.LatestVersion = newLatest.DocumentVersion
		doc.DocumentVersion = newLatest.DocumentVersion
		doc.Content = newLatest.Content
		doc.DocumentFormat = newLatest.DocumentFormat
		doc.Status = newLatest.Status
		doc.Hash, doc.Sha1 = documentHashes(newLatest.Content)

		if newLatest.DisplayName != "" {
			doc.DisplayName = newLatest.DisplayName
		}
	}

	if removed.DocumentVersion == doc.DefaultVersion {
		newDefault := versions[len(versions)-1].DocumentVersion
		doc.DefaultVersion = newDefault

		for i := range versions {
			versions[i].IsDefaultVersion = versions[i].DocumentVersion == newDefault
		}
	}

	b.documentsStore(region).Put(&doc)
	b.documentVersionsStore(region)[doc.Name] = versions

	return &DeleteDocumentOutput{}
}

// resolveDeleteDocumentVersionIdx finds the index in versions matching
// input's DocumentVersion/VersionName selector, or -1 if unresolvable.
// VersionName never resolves: no Go field on DocumentVersion tracks it (see
// models_documents.go), a disclosed gap -- routing it through
// resolveDocumentVersionSelector would risk colliding with the numeric
// DocumentVersion namespace instead of honestly reporting "not found".
func resolveDeleteDocumentVersionIdx(doc Document, versions []DocumentVersion, input *DeleteDocumentInput) int {
	if input.DocumentVersion == "" {
		return -1
	}

	target := resolveDocumentVersionSelector(doc, input.DocumentVersion)

	return slices.IndexFunc(versions, func(v DocumentVersion) bool { return v.DocumentVersion == target })
}

// DeleteDocument removes a document, or one version of it when
// DocumentVersion/VersionName is given (see DeleteDocumentInput's doc
// comment). Real AWS also rejects deleting a still-shared document with
// InvalidDocumentOperation (ErrDocumentStillShared) -- deserializers.go:2225-2226.
func (b *InMemoryBackend) DeleteDocument(
	ctx context.Context,
	input *DeleteDocumentInput,
) (*DeleteDocumentOutput, error) {
	region := getRegion(ctx)
	b.mu.Lock("DeleteDocument")
	defer b.mu.Unlock()

	docPtr, exists := b.documentsStore(region).Get(input.Name)
	if !exists {
		return nil, ErrDocumentNotFound
	}

	doc := *docPtr

	if input.DocumentVersion != "" || input.VersionName != "" {
		versions := b.documentVersionsStore(region)[input.Name]

		idx := resolveDeleteDocumentVersionIdx(doc, versions, input)
		if idx == -1 {
			return nil, ErrDocumentNotFound
		}

		if len(versions) > 1 {
			return b.deleteDocumentVersionScoped(region, doc, versions, idx), nil
		}
		// Exactly one version remains: deleting it deletes the document,
		// falling through to the full delete below.
	}

	if len(b.documentPermissionsStore(region)[input.Name]) > 0 {
		return nil, ErrDocumentStillShared
	}

	b.documentsStore(region).Delete(input.Name)
	delete(b.documentVersionsStore(region), input.Name)
	delete(b.documentPermissionsStore(region), input.Name)
	delete(b.documentSharedVersionsStore(region), input.Name)
	delete(b.miscResourceTagsStore(region), input.Name)

	// b.documents itself is not pruned here — see the comment on
	// cleanupEmptyParamRegion above for why a Table-backed region entry must
	// never be removed from its outer map once registered.
	cleanupEmptyInnerMap(b.documentVersions, region)
	cleanupEmptyInnerMap(b.documentPermissions, region)
	cleanupEmptyInnerMap(b.documentSharedVersions, region)
	cleanupEmptyInnerMap(b.miscResourceTags, region)

	return &DeleteDocumentOutput{}, nil
}

// DescribeDocumentPermission returns the sharing permissions for a document,
// paginated by MaxResults/NextToken (previously unimplemented: every call
// returned the full unpaginated list regardless of MaxResults).
func (b *InMemoryBackend) DescribeDocumentPermission(
	ctx context.Context,
	input *DescribeDocumentPermissionInput,
) (*DescribeDocumentPermissionOutput, error) {
	region := getRegion(ctx)
	b.mu.RLock("DescribeDocumentPermission")
	defer b.mu.RUnlock()

	if !b.documentsStore(region).Has(input.Name) {
		return nil, ErrDocumentNotFound
	}

	accountIDs := b.documentPermissionsStore(region)[input.Name]
	sharedVersions := b.documentSharedVersionsStore(region)[input.Name]

	startIdx := parseNextToken(input.NextToken)

	maxResults := int64(defaultListDocMaxResults)
	if input.MaxResults != nil && *input.MaxResults > 0 {
		maxResults = *input.MaxResults
	}

	if startIdx >= len(accountIDs) {
		return &DescribeDocumentPermissionOutput{
			AccountIDs:             []string{},
			AccountSharingInfoList: []AccountSharingInfo{},
		}, nil
	}

	end := startIdx + int(maxResults)

	var nextToken string

	if end < len(accountIDs) {
		nextToken = strconv.Itoa(end)
	} else {
		end = len(accountIDs)
	}

	page := accountIDs[startIdx:end]
	infoList := make([]AccountSharingInfo, 0, len(page))

	for _, id := range page {
		infoList = append(infoList, AccountSharingInfo{
			AccountID:             id,
			SharedDocumentVersion: sharedVersions[id],
		})
	}

	return &DescribeDocumentPermissionOutput{
		AccountIDs:             page,
		AccountSharingInfoList: infoList,
		NextToken:              nextToken,
	}, nil
}

// ModifyDocumentPermission updates the sharing permissions for a document.
// SharedDocumentVersion is pinned per account in documentSharedVersionsStore;
// an omitted SharedDocumentVersion shares the document's current
// DefaultVersion instead, matching api_op_ModifyDocumentPermission.go:51-53.
func (b *InMemoryBackend) ModifyDocumentPermission(
	ctx context.Context,
	input *ModifyDocumentPermissionInput,
) (*ModifyDocumentPermissionOutput, error) {
	region := getRegion(ctx)
	b.mu.Lock("ModifyDocumentPermission")
	defer b.mu.Unlock()

	docPtr, exists := b.documentsStore(region).Get(input.Name)
	if !exists {
		return nil, ErrDocumentNotFound
	}

	if b.documentPermissions[region] == nil {
		b.documentPermissions[region] = make(map[string][]string)
	}
	permStore := b.documentPermissionsStore(region)
	current := permStore[input.Name]

	if b.documentSharedVersions[region] == nil {
		b.documentSharedVersions[region] = make(map[string]map[string]string)
	}
	sharedVersions := b.documentSharedVersionsStore(region)
	if sharedVersions[input.Name] == nil {
		sharedVersions[input.Name] = make(map[string]string)
	}

	pinned := input.SharedDocumentVersion
	if pinned == "" {
		pinned = docPtr.DefaultVersion
	}

	for _, id := range input.AccountIDsToAdd {
		if !slices.Contains(current, id) {
			current = append(current, id)
		}

		sharedVersions[input.Name][id] = pinned
	}

	for _, id := range input.AccountIDsToRemove {
		current = slices.DeleteFunc(current, func(v string) bool { return v == id })
		delete(sharedVersions[input.Name], id)
	}

	permStore[input.Name] = current

	return &ModifyDocumentPermissionOutput{}, nil
}

// ListDocumentVersions returns all versions of a document.
func (b *InMemoryBackend) ListDocumentVersions(
	ctx context.Context,
	input *ListDocumentVersionsInput,
) (*ListDocumentVersionsOutput, error) {
	region := getRegion(ctx)
	b.mu.RLock("ListDocumentVersions")
	defer b.mu.RUnlock()

	if !b.documentsStore(region).Has(input.Name) {
		return nil, ErrDocumentNotFound
	}

	versions := b.documentVersionsStore(region)[input.Name]

	startIdx := parseNextToken(input.NextToken)

	maxResults := int64(defaultListDocMaxResults)
	if input.MaxResults != nil && *input.MaxResults > 0 {
		maxResults = *input.MaxResults
	}

	if startIdx >= len(versions) {
		return &ListDocumentVersionsOutput{DocumentVersions: []DocumentVersionInfo{}}, nil
	}

	end := startIdx + int(maxResults)

	var nextToken string

	if end < len(versions) {
		nextToken = strconv.Itoa(end)
	} else {
		end = len(versions)
	}

	page := make([]DocumentVersionInfo, 0, end-startIdx)
	for _, v := range versions[startIdx:end] {
		page = append(page, DocumentVersionInfo{
			Name:             v.Name,
			DisplayName:      v.DisplayName,
			DocumentVersion:  v.DocumentVersion,
			DocumentFormat:   v.DocumentFormat,
			Status:           v.Status,
			CreatedDate:      v.CreatedDate,
			IsDefaultVersion: v.IsDefaultVersion,
		})
	}

	return &ListDocumentVersionsOutput{
		DocumentVersions: page,
		NextToken:        nextToken,
	}, nil
}

// UpdateDocumentDefaultVersion sets the DefaultVersion field on an existing document.
// It fails if the document or the requested version does not exist.
// Returns a no-op success when Name or DocumentVersion is empty (legacy stub compat).
func (b *InMemoryBackend) UpdateDocumentDefaultVersion(
	ctx context.Context,
	input *UpdateDocumentDefaultVersionInput,
) (*UpdateDocumentDefaultVersionOutput, error) {
	if input.Name == "" {
		return nil, fmt.Errorf("%w: Name is required", ErrValidationException)
	}

	if input.DocumentVersion == "" {
		return nil, fmt.Errorf("%w: DocumentVersion is required", ErrValidationException)
	}

	region := getRegion(ctx)
	b.mu.Lock("UpdateDocumentDefaultVersion")
	defer b.mu.Unlock()

	docs := b.documentsStore(region)
	docPtr, exists := docs.Get(input.Name)
	if !exists {
		return nil, fmt.Errorf("%w: document %q not found", ErrDocumentNotFound, input.Name)
	}

	doc := *docPtr

	// Verify the requested version exists in documentVersions.
	found := false

	docVersions := b.documentVersionsStore(region)
	for _, dv := range docVersions[input.Name] {
		if dv.DocumentVersion == input.DocumentVersion {
			found = true

			break
		}
	}

	if !found {
		return nil, fmt.Errorf("%w: version %q not found for document %q",
			ErrInvalidDocumentVersion, input.DocumentVersion, input.Name)
	}

	doc.DefaultVersion = input.DocumentVersion
	docs.Put(&doc)

	// Also mark the version entry.
	versions := docVersions[input.Name]
	for i := range versions {
		versions[i].IsDefaultVersion = versions[i].DocumentVersion == input.DocumentVersion
	}

	docVersions[input.Name] = versions

	return &UpdateDocumentDefaultVersionOutput{
		Description: &DocumentDefaultVersionDescription{
			Name:           input.Name,
			DefaultVersion: input.DocumentVersion,
		},
	}, nil
}

// UpdateDocumentMetadata updates document reviews metadata.
// This is a lightweight implementation that acknowledges the request and
// returns success without modifying stored state (the AWS API is complex
// and review state is not tracked in this in-memory implementation).
func (b *InMemoryBackend) UpdateDocumentMetadata(
	ctx context.Context,
	input *UpdateDocumentMetadataInput,
) (*UpdateDocumentMetadataOutput, error) {
	if err := validateUpdateDocumentMetadataInput(input); err != nil {
		return nil, err
	}

	region := getRegion(ctx)
	b.mu.RLock("UpdateDocumentMetadata")
	defer b.mu.RUnlock()

	if !b.documentsStore(region).Has(input.Name) {
		return nil, fmt.Errorf("%w: document %q not found", ErrDocumentNotFound, input.Name)
	}

	return &UpdateDocumentMetadataOutput{}, nil
}

// validateUpdateDocumentMetadataInput enforces UpdateDocumentMetadata's
// required fields: Name and DocumentReviews (with its own required Action)
// are both required on the real op, but were previously entirely
// unvalidated -- an empty body silently succeeded instead of rejecting with
// ValidationException.
func validateUpdateDocumentMetadataInput(input *UpdateDocumentMetadataInput) error {
	if input.Name == "" {
		return fmt.Errorf("%w: Name is required", ErrValidationException)
	}

	if input.DocumentReviews == nil {
		return fmt.Errorf("%w: DocumentReviews is required", ErrValidationException)
	}

	// Real DocumentReviewAction enum values (aws-sdk-go-v2/service/ssm@v1.73.4 types/enums.go:780-798).
	validActions := []string{"SendForReview", "UpdateReview", "Approve", "Reject"}
	if !slices.Contains(validActions, input.DocumentReviews.Action) {
		return fmt.Errorf("%w: DocumentReviews.Action must be one of %v", ErrValidationException, validActions)
	}

	return nil
}

// ListDocumentMetadataHistory returns an empty approval history.
// The in-memory backend does not track document review history; this returns
// a well-formed empty response consistent with the stateless stub approach.
func (b *InMemoryBackend) ListDocumentMetadataHistory(
	ctx context.Context,
	input *ListDocumentMetadataHistoryInput,
) (*ListDocumentMetadataHistoryOutput, error) {
	if input.Name == "" {
		return nil, fmt.Errorf("%w: Name is required", ErrValidationException)
	}

	// DocumentMetadataEnum has exactly one real value (types/enums.go:727-745).
	if input.Metadata != "DocumentReviews" {
		return nil, fmt.Errorf(`%w: Metadata must be "DocumentReviews"`, ErrValidationException)
	}

	region := getRegion(ctx)
	b.mu.RLock("ListDocumentMetadataHistory")
	defer b.mu.RUnlock()

	doc, exists := b.documentsStore(region).Get(input.Name)
	if !exists {
		return nil, fmt.Errorf("%w: document %q not found", ErrDocumentNotFound, input.Name)
	}

	return &ListDocumentMetadataHistoryOutput{
		Name:            doc.Name,
		DocumentVersion: doc.DocumentVersion,
		Metadata: &DocumentMetadataResponseInfo{
			ReviewerResponse: []DocumentReviewerResponseSource{},
		},
	}, nil
}

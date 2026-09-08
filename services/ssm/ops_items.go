package ssm

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

func (b *InMemoryBackend) opsItemsStore(region string) *store.Table[OpsItem] {
	return getOrCreateTable(b, b.opsItems, "opsItems", region, opsItemKeyFn)
}
func (b *InMemoryBackend) opsItemRelatedItemsStore(region string) map[string][]OpsItemRelatedItem {
	return b.opsItemRelatedItems[region]
}
func (b *InMemoryBackend) opsMetadataStore(region string) *store.Table[OpsMetadata] {
	return getOrCreateTable(b, b.opsMetadata, "opsMetadata", region, opsMetadataKeyFn)
}
func (b *InMemoryBackend) resourceIDToOpsMetadataArnStore(region string) map[string]string {
	return b.resourceIDToOpsMetadataArn[region]
}
func (b *InMemoryBackend) opsItemEventsStore(region string) []OpsItemEventSummary {
	return b.opsItemEvents[region]
}

// CreateOpsItem creates a new OpsItem.
func (b *InMemoryBackend) CreateOpsItem(
	ctx context.Context,
	input *CreateOpsItemInput,
) (*CreateOpsItemOutput, error) {
	if input.Title == "" {
		return nil, fmt.Errorf("%w: Title is required", ErrValidationException)
	}

	if input.Source == "" {
		return nil, fmt.Errorf("%w: Source is required", ErrValidationException)
	}

	if input.Description == "" {
		return nil, fmt.Errorf("%w: Description is required", ErrValidationException)
	}

	region := getRegion(ctx)
	b.mu.Lock("CreateOpsItem")
	defer b.mu.Unlock()

	opsItemID := opsItemIDPrefix + uuid.NewString()
	opsItemArn := arn.Build("ssm", region, defaultAccountID, fmt.Sprintf("opsitem/%s", opsItemID))
	now := UnixTimeFloat(time.Now())

	item := OpsItem{
		OpsItemID:        opsItemID,
		OpsItemArn:       opsItemArn,
		OpsItemType:      input.OpsItemType,
		Title:            input.Title,
		Source:           input.Source,
		Description:      input.Description,
		Status:           opsItemStatusOpen,
		Severity:         input.Severity,
		Category:         input.Category,
		OperationalData:  input.OperationalData,
		CreatedTime:      now,
		LastModifiedTime: now,
		Priority:         input.Priority,
		AccountID:        input.AccountID,
		ActualStartTime:  input.ActualStartTime,
		ActualEndTime:    input.ActualEndTime,
		Notifications:    append([]OpsItemNotification(nil), input.Notifications...),
		PlannedStartTime: input.PlannedStartTime,
		PlannedEndTime:   input.PlannedEndTime,
		RelatedOpsItems:  append([]RelatedOpsItemRef(nil), input.RelatedOpsItems...),
		Version:          "1",
	}

	b.opsItemsStore(region).Put(&item)

	b.opsItemEvents[region] = append(b.opsItemEvents[region], OpsItemEventSummary{
		OpsItemID: opsItemID,
		EventID:   "event-create-" + opsItemID,
	})

	if len(input.Tags) > 0 {
		if b.miscResourceTags[region] == nil {
			b.miscResourceTags[region] = make(map[string]map[string]string)
		}
		miscTags := b.miscResourceTagsStore(region)
		if miscTags[opsItemID] == nil {
			miscTags[opsItemID] = make(map[string]string)
		}
		for _, t := range input.Tags {
			miscTags[opsItemID][t.Key] = t.Value
		}
	}

	return &CreateOpsItemOutput{
		OpsItemID:  opsItemID,
		OpsItemArn: opsItemArn,
	}, nil
}

// AssociateOpsItemRelatedItem associates a related item to an OpsItem.
func (b *InMemoryBackend) AssociateOpsItemRelatedItem(
	ctx context.Context,
	input *AssociateOpsItemRelatedItemInput,
) (*AssociateOpsItemRelatedItemOutput, error) {
	if input.AssociationType == "" || input.ResourceType == "" || input.ResourceURI == "" {
		return nil, fmt.Errorf(
			"%w: AssociationType, ResourceType and ResourceUri are required",
			ErrValidationException,
		)
	}

	region := getRegion(ctx)
	b.mu.Lock("AssociateOpsItemRelatedItem")
	defer b.mu.Unlock()

	if !b.opsItemsStore(region).Has(input.OpsItemID) {
		return nil, ErrOpsItemNotFound
	}

	assocID := uuid.NewString()
	related := OpsItemRelatedItem{
		AssociationID:   assocID,
		AssociationType: input.AssociationType,
		ResourceType:    input.ResourceType,
		ResourceURI:     input.ResourceURI,
	}

	if b.opsItemRelatedItems[region] == nil {
		b.opsItemRelatedItems[region] = make(map[string][]OpsItemRelatedItem)
	}
	store := b.opsItemRelatedItemsStore(region)
	store[input.OpsItemID] = append(store[input.OpsItemID], related)

	return &AssociateOpsItemRelatedItemOutput{AssociationID: assocID}, nil
}

// CreateOpsMetadata creates OpsMetadata for a resource.
func (b *InMemoryBackend) CreateOpsMetadata(
	ctx context.Context,
	input *CreateOpsMetadataInput,
) (*CreateOpsMetadataOutput, error) {
	if input.ResourceID == "" {
		return nil, fmt.Errorf("%w: ResourceId is required", ErrValidationException)
	}

	region := getRegion(ctx)
	b.mu.Lock("CreateOpsMetadata")
	defer b.mu.Unlock()

	if b.resourceIDToOpsMetadataArn[region] == nil {
		b.resourceIDToOpsMetadataArn[region] = make(map[string]string)
	}
	resToArn := b.resourceIDToOpsMetadataArnStore(region)
	if _, exists := resToArn[input.ResourceID]; exists {
		return nil, fmt.Errorf(
			"%w: OpsMetadata already exists for resource %s",
			ErrOpsMetadataAlreadyExists,
			input.ResourceID,
		)
	}

	metaID := uuid.NewString()
	arn := fmt.Sprintf(opsMetadataArnTpl, region, defaultAccountID, metaID)
	now := UnixTimeFloat(time.Now())

	meta := OpsMetadata{
		OpsMetadataArn:   arn,
		ResourceID:       input.ResourceID,
		Metadata:         input.Metadata,
		CreationDate:     now,
		LastModifiedDate: now,
	}

	b.opsMetadataStore(region).Put(&meta)
	resToArn[input.ResourceID] = arn

	if len(input.Tags) > 0 {
		if b.miscResourceTags[region] == nil {
			b.miscResourceTags[region] = make(map[string]map[string]string)
		}
		miscTags := b.miscResourceTagsStore(region)
		if miscTags[arn] == nil {
			miscTags[arn] = make(map[string]string)
		}
		for _, t := range input.Tags {
			miscTags[arn][t.Key] = t.Value
		}
	}

	return &CreateOpsMetadataOutput{OpsMetadataArn: arn}, nil
}

// GetOpsSummary returns a summary count of ops items.
func (b *InMemoryBackend) GetOpsSummary(
	ctx context.Context,
	_ *GetOpsSummaryInput,
) (*GetOpsSummaryOutputFull, error) {
	region := getRegion(ctx)
	b.mu.RLock("GetOpsSummary")
	defer b.mu.RUnlock()

	return &GetOpsSummaryOutputFull{
		Entities: []OpsSummaryEntity{
			{
				ID: "AWS:OpsItem",
				Data: map[string]OpsSummaryValue{
					"Count": {Count: b.opsItemsStore(region).Len(), Unit: "Count"},
				},
			},
		},
	}, nil
}

// matchesOpsMetadataFilter reports whether an OpsMetadata entry satisfies a
// single key/values filter. Only ResourceId has backing state to filter on;
// every other key matches every entry (accept-and-echo, mirroring ListNodes'
// unknown-key handling, instances.go).
func matchesOpsMetadataFilter(m OpsMetadata, f OpsMetadataFilterEntry) bool {
	if f.Key != "ResourceId" {
		return true
	}

	return slices.Contains(f.Values, m.ResourceID)
}

// ListOpsMetadata returns ops metadata entries, filtered by input.Filters
// and paginated by input.MaxResults/NextToken -- real, optional
// ListOpsMetadataInput members (api_op_ListOpsMetadata.go) a literal
// struct{} input previously discarded from every request.
//
//nolint:dupl // mirrors ListAssociations' filter/sort/paginate shape inherently, not by copy-paste
func (b *InMemoryBackend) ListOpsMetadata(
	ctx context.Context,
	input *ListOpsMetadataInput,
) (*ListOpsMetadataOutputFull, error) {
	region := getRegion(ctx)
	b.mu.RLock("ListOpsMetadata")
	defer b.mu.RUnlock()

	opsMetadata := b.opsMetadataStore(region)
	list := make([]OpsMetadata, 0, opsMetadata.Len())

	for _, m := range opsMetadata.All() {
		matched := true

		for _, f := range input.Filters {
			if !matchesOpsMetadataFilter(*m, f) {
				matched = false

				break
			}
		}

		if matched {
			list = append(list, *m)
		}
	}

	sort.Slice(list, func(i, k int) bool {
		return list[i].OpsMetadataArn < list[k].OpsMetadataArn
	})

	var maxResults int
	if input.MaxResults != nil {
		maxResults = int(*input.MaxResults)
	}

	page, next := paginateSlice(list, input.NextToken, maxResults, defaultDescribeMaxResults)

	return &ListOpsMetadataOutputFull{OpsMetadataList: page, NextToken: next}, nil
}

// opsItemMatchesFilters returns true when the item satisfies all provided
// filters. Supported keys are the ones backed by fields this emulator's
// OpsItem models: Status, Title, Source (real keys per
// aws-sdk-go-v2/service/ssm@v1.73.4's api_op_DescribeOpsItems.go doc
// comment; the other ~25 documented keys, mostly AccessRequest/ChangeRequest
// sub-filters, have no backing field). That same doc comment documents each
// key's supported Operator(s): Status is Equals-only, but Title and Source
// both also support Contains (substring), honored below rather than always
// compared for exact equality.
func opsItemMatchesFilters(item OpsItem, filters []OpsItemFilter) bool {
	for _, f := range filters {
		var fieldValue string

		switch f.Key {
		case "Status":
			fieldValue = item.Status
		case "Title":
			fieldValue = item.Title
		case "Source":
			fieldValue = item.Source
		default:
			continue
		}

		if f.Operator == "Contains" {
			if !slices.ContainsFunc(f.Values, func(v string) bool { return strings.Contains(fieldValue, v) }) {
				return false
			}

			continue
		}

		if !slices.Contains(f.Values, fieldValue) {
			return false
		}
	}

	return true
}

// DescribeOpsItems lists OpsItems.
func (b *InMemoryBackend) DescribeOpsItems(
	ctx context.Context,
	input *DescribeOpsItemsInput,
) (*DescribeOpsItemsOutput, error) {
	region := getRegion(ctx)
	b.mu.RLock("DescribeOpsItems")
	defer b.mu.RUnlock()

	items := b.opsItemsStore(region)
	all := make([]OpsItemSummary, 0, items.Len())
	for _, itemPtr := range items.All() {
		item := *itemPtr
		if !opsItemMatchesFilters(item, input.OpsItemFilters) {
			continue
		}

		all = append(all, OpsItemSummary{
			OperationalData:  item.OperationalData,
			PlannedEndTime:   item.PlannedEndTime,
			PlannedStartTime: item.PlannedStartTime,
			ActualEndTime:    item.ActualEndTime,
			ActualStartTime:  item.ActualStartTime,
			OpsItemID:        item.OpsItemID,
			Title:            item.Title,
			Status:           item.Status,
			Source:           item.Source,
			OpsItemType:      item.OpsItemType,
			Category:         item.Category,
			Severity:         item.Severity,
			CreatedTime:      item.CreatedTime,
			LastModifiedTime: item.LastModifiedTime,
			Priority:         item.Priority,
		})
	}

	sort.Slice(all, func(i, j int) bool { return all[i].OpsItemID < all[j].OpsItemID })

	startIdx := parseNextToken(input.NextToken)

	const defaultOpsItemMaxResults = 50

	maxResults := int64(defaultOpsItemMaxResults)
	if input.MaxResults != nil && *input.MaxResults > 0 {
		maxResults = *input.MaxResults
	}

	if startIdx >= len(all) {
		return &DescribeOpsItemsOutput{OpsItemSummaries: []OpsItemSummary{}}, nil
	}

	end := startIdx + int(maxResults)

	var nextToken string

	if end < len(all) {
		nextToken = strconv.Itoa(end)
	} else {
		end = len(all)
	}

	return &DescribeOpsItemsOutput{
		OpsItemSummaries: all[startIdx:end],
		NextToken:        nextToken,
	}, nil
}

// GetOpsItem retrieves an OpsItem by ID.
func (b *InMemoryBackend) GetOpsItem(
	ctx context.Context,
	input *GetOpsItemInput,
) (*GetOpsItemOutput, error) {
	region := getRegion(ctx)
	b.mu.RLock("GetOpsItem")
	defer b.mu.RUnlock()

	item, exists := b.opsItemsStore(region).Get(input.OpsItemID)
	if !exists {
		return nil, ErrOpsItemNotFound
	}

	return &GetOpsItemOutput{OpsItem: opsItemToOutput(*item)}, nil
}

// GetOpsMetadata retrieves OpsMetadata by ARN.
func (b *InMemoryBackend) GetOpsMetadata(
	ctx context.Context,
	input *GetOpsMetadataInput,
) (*GetOpsMetadataOutput, error) {
	region := getRegion(ctx)
	b.mu.RLock("GetOpsMetadata")
	defer b.mu.RUnlock()

	meta, exists := b.opsMetadataStore(region).Get(input.OpsMetadataArn)
	if !exists {
		return nil, ErrOpsMetadataNotFound
	}

	return &GetOpsMetadataOutput{Metadata: meta.Metadata, ResourceID: meta.ResourceID}, nil
}

// nextOpsItemVersion returns current+1, defaulting to 1 for an unparseable
// or empty current version (e.g. an item persisted before Version existed).
func nextOpsItemVersion(current string) int {
	n, err := strconv.Atoi(current)
	if err != nil {
		return 1
	}

	return n + 1
}

// applyOpsItemCoreUpdates applies UpdateOpsItemInput's original (pre-Change-
// Manager-fields) settable properties to item in place.
func applyOpsItemCoreUpdates(item *OpsItem, input *UpdateOpsItemInput) {
	if input.Title != "" {
		item.Title = input.Title
	}

	if input.Description != "" {
		item.Description = input.Description
	}

	if input.Status != "" {
		item.Status = input.Status
	}

	if input.Severity != "" {
		item.Severity = input.Severity
	}

	if input.Category != "" {
		item.Category = input.Category
	}

	if input.Priority != nil {
		item.Priority = *input.Priority
	}

	if len(input.OperationalData) > 0 {
		if item.OperationalData == nil {
			item.OperationalData = make(map[string]OpsItemDataValue)
		}

		maps.Copy(item.OperationalData, input.OperationalData)
	}
}

// applyOpsItemChangeManagerUpdates applies the Change-Manager-oriented fields
// on UpdateOpsItemInput (OpsItemArn/ActualStartTime/ActualEndTime/
// Notifications/PlannedStartTime/PlannedEndTime/RelatedOpsItems) to item in
// place. AccountId is deliberately NOT applied here -- unlike
// CreateOpsItemInput, the real UpdateOpsItemInput has no AccountId member at
// all (api_op_UpdateOpsItem.go); a caller cannot change an OpsItem's account
// attribution after creation. Split out of UpdateOpsItem to keep its
// cyclomatic complexity under the package limit.
func applyOpsItemChangeManagerUpdates(item *OpsItem, input *UpdateOpsItemInput) {
	if input.OpsItemArn != "" {
		item.OpsItemArn = input.OpsItemArn
	}

	if input.ActualStartTime != nil {
		item.ActualStartTime = input.ActualStartTime
	}

	if input.ActualEndTime != nil {
		item.ActualEndTime = input.ActualEndTime
	}

	if input.Notifications != nil {
		item.Notifications = append([]OpsItemNotification(nil), input.Notifications...)
	}

	if input.PlannedStartTime != nil {
		item.PlannedStartTime = input.PlannedStartTime
	}

	if input.PlannedEndTime != nil {
		item.PlannedEndTime = input.PlannedEndTime
	}

	if input.RelatedOpsItems != nil {
		item.RelatedOpsItems = append([]RelatedOpsItemRef(nil), input.RelatedOpsItems...)
	}
}

// UpdateOpsItem updates an OpsItem including OperationalData.
func (b *InMemoryBackend) UpdateOpsItem(
	ctx context.Context,
	input *UpdateOpsItemInput,
) (*UpdateOpsItemOutput, error) {
	region := getRegion(ctx)
	b.mu.Lock("UpdateOpsItem")
	defer b.mu.Unlock()

	items := b.opsItemsStore(region)
	itemPtr, exists := items.Get(input.OpsItemID)
	if !exists {
		return nil, ErrOpsItemNotFound
	}

	item := *itemPtr

	applyOpsItemCoreUpdates(&item, input)
	applyOpsItemChangeManagerUpdates(&item, input)

	item.LastModifiedTime = UnixTimeFloat(timeNow())
	item.Version = strconv.Itoa(nextOpsItemVersion(item.Version))
	items.Put(&item)

	// Record an event for the update.
	b.opsItemEvents[region] = append(b.opsItemEvents[region], OpsItemEventSummary{
		OpsItemID: input.OpsItemID,
		EventID:   "event-update-" + input.OpsItemID,
	})

	return &UpdateOpsItemOutput{}, nil
}

// UpdateOpsMetadata updates OpsMetadata.
func (b *InMemoryBackend) UpdateOpsMetadata(
	ctx context.Context,
	input *UpdateOpsMetadataInput,
) (*UpdateOpsMetadataOutput, error) {
	region := getRegion(ctx)
	b.mu.Lock("UpdateOpsMetadata")
	defer b.mu.Unlock()

	opsMetadata := b.opsMetadataStore(region)
	metaPtr, exists := opsMetadata.Get(input.OpsMetadataArn)
	if !exists {
		return nil, ErrOpsMetadataNotFound
	}

	meta := *metaPtr

	if input.Metadata != nil {
		if meta.Metadata == nil {
			meta.Metadata = make(map[string]MetadataValue)
		}

		maps.Copy(meta.Metadata, input.Metadata)
	}

	meta.LastModifiedDate = UnixTimeFloat(timeNow())
	opsMetadata.Put(&meta)

	return &UpdateOpsMetadataOutput{OpsMetadataArn: input.OpsMetadataArn}, nil
}

// DeleteOpsItem removes an OpsItem by ID.
func (b *InMemoryBackend) DeleteOpsItem(
	ctx context.Context,
	input *DeleteOpsItemInput,
) (*DeleteOpsItemOutput, error) {
	region := getRegion(ctx)
	b.mu.Lock("DeleteOpsItem")
	defer b.mu.Unlock()

	opsItems := b.opsItemsStore(region)
	opsItems.Delete(input.OpsItemID)
	delete(b.opsItemRelatedItemsStore(region), input.OpsItemID)
	delete(b.miscResourceTagsStore(region), input.OpsItemID)

	cleanupEmptyInnerMap(b.opsItemRelatedItems, region)
	cleanupEmptyInnerMap(b.miscResourceTags, region)

	return &DeleteOpsItemOutput{}, nil
}

// DisassociateOpsItemRelatedItem removes a related item from an OpsItem.
// Returns success if the OpsItem does not exist (stub compat for empty ID).
func (b *InMemoryBackend) DisassociateOpsItemRelatedItem(
	ctx context.Context,
	input *DisassociateOpsItemRelatedItemInput,
) (*DisassociateOpsItemRelatedItemOutput, error) {
	if input.OpsItemID == "" || input.AssociationID == "" {
		return nil, fmt.Errorf("%w: OpsItemId and AssociationId are required", ErrValidationException)
	}

	region := getRegion(ctx)
	b.mu.Lock("DisassociateOpsItemRelatedItem")
	defer b.mu.Unlock()

	store := b.opsItemRelatedItemsStore(region)
	items, exists := store[input.OpsItemID]
	if !exists {
		// No-op if OpsItem doesn't have any related items.
		return &DisassociateOpsItemRelatedItemOutput{}, nil
	}

	filtered := items[:0]
	for _, item := range items {
		if item.AssociationID != input.AssociationID {
			filtered = append(filtered, item)
		}
	}

	store[input.OpsItemID] = filtered

	return &DisassociateOpsItemRelatedItemOutput{}, nil
}

// ListOpsItemRelatedItems returns stored related items for an OpsItem.
func (b *InMemoryBackend) ListOpsItemRelatedItems(
	ctx context.Context,
	input *ListOpsItemRelatedItemsInput,
) (*ListOpsItemRelatedItemsOutput, error) {
	region := getRegion(ctx)
	b.mu.RLock("ListOpsItemRelatedItems")
	defer b.mu.RUnlock()

	var all []OpsItemRelatedItem

	store := b.opsItemRelatedItemsStore(region)
	if input.OpsItemID != "" {
		all = append(all, store[input.OpsItemID]...)
	} else {
		for _, items := range store {
			all = append(all, items...)
		}
	}

	if all == nil {
		all = []OpsItemRelatedItem{}
	}

	// AssociationID is assigned via uuid.NewString() (AssociateOpsItemRelatedItem)
	// and never reused, so sorting on it alone is sufficient even though the
	// OpsItemId=="" branch above walks opsItemRelatedItems in unspecified map order.
	sort.Slice(all, func(i, j int) bool { return all[i].AssociationID < all[j].AssociationID })

	const maxOpsItemRelatedItems = 50

	if input.MaxResults != nil {
		if *input.MaxResults < 1 || *input.MaxResults > maxOpsItemRelatedItems {
			return nil, fmt.Errorf(
				"%w: MaxResults must be between 1 and %d",
				ErrValidationException,
				maxOpsItemRelatedItems,
			)
		}
	}

	startIdx := parseNextToken(input.NextToken)
	limit := int64(maxOpsItemRelatedItems)

	if input.MaxResults != nil {
		limit = *input.MaxResults
	}

	if startIdx >= len(all) {
		return &ListOpsItemRelatedItemsOutput{Summaries: []OpsItemRelatedItem{}}, nil
	}

	end := startIdx + int(limit)

	var nextToken string

	if end < len(all) {
		nextToken = strconv.Itoa(end)
	} else {
		end = len(all)
	}

	return &ListOpsItemRelatedItemsOutput{NextToken: nextToken, Summaries: all[startIdx:end]}, nil
}

// ListOpsItemEvents returns tracked events for OpsItems, optionally filtered by OpsItemID.
func (b *InMemoryBackend) ListOpsItemEvents(
	ctx context.Context,
	input *ListOpsItemEventsInput,
) (*ListOpsItemEventsOutput, error) {
	region := getRegion(ctx)
	b.mu.RLock("ListOpsItemEvents")
	defer b.mu.RUnlock()

	var summaries []OpsItemEventSummary

	events := b.opsItemEventsStore(region)
	for _, event := range events {
		if input.OpsItemID != "" && event.OpsItemID != input.OpsItemID {
			continue
		}

		summaries = append(summaries, event)
	}

	if summaries == nil {
		summaries = []OpsItemEventSummary{}
	}

	const maxOpsItemEvents = 50

	if input.MaxResults != nil {
		if *input.MaxResults < 1 || *input.MaxResults > maxOpsItemEvents {
			return nil, fmt.Errorf(
				"%w: MaxResults must be between 1 and %d",
				ErrValidationException,
				maxOpsItemEvents,
			)
		}
	}

	startIdx := parseNextToken(input.NextToken)
	limit := int64(maxOpsItemEvents)

	if input.MaxResults != nil {
		limit = *input.MaxResults
	}

	if startIdx >= len(summaries) {
		return &ListOpsItemEventsOutput{Summaries: []OpsItemEventSummary{}}, nil
	}

	end := startIdx + int(limit)

	var nextToken string

	if end < len(summaries) {
		nextToken = strconv.Itoa(end)
	} else {
		end = len(summaries)
	}

	return &ListOpsItemEventsOutput{NextToken: nextToken, Summaries: summaries[startIdx:end]}, nil
}

// DeleteOpsMetadata removes OpsMetadata by ARN.
func (b *InMemoryBackend) DeleteOpsMetadata(
	ctx context.Context,
	input *DeleteOpsMetadataInput,
) (*DeleteOpsMetadataOutput, error) {
	region := getRegion(ctx)
	b.mu.Lock("DeleteOpsMetadata")
	defer b.mu.Unlock()

	opsMetadata := b.opsMetadataStore(region)
	meta, exists := opsMetadata.Get(input.OpsMetadataArn)
	if !exists {
		return nil, ErrOpsMetadataNotFound
	}

	delete(b.resourceIDToOpsMetadataArnStore(region), meta.ResourceID)
	opsMetadata.Delete(input.OpsMetadataArn)
	delete(b.miscResourceTagsStore(region), input.OpsMetadataArn)

	cleanupEmptyInnerMap(b.resourceIDToOpsMetadataArn, region)
	cleanupEmptyInnerMap(b.miscResourceTags, region)

	return &DeleteOpsMetadataOutput{}, nil
}

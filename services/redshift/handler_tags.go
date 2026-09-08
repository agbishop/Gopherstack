package redshift

import (
	"encoding/xml"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strings"

	"github.com/blackbirdworks/gopherstack/pkgs/store"
	svcTags "github.com/blackbirdworks/gopherstack/pkgs/tags"
)

type redshiftTaggedResource struct {
	Tag          svcTags.KV `xml:"Tag"`
	ResourceName string     `xml:"ResourceName"`
	ResourceType string     `xml:"ResourceType"`
}

// handleDescribeTags returns tagged resources, optionally filtered by ResourceName,
// ResourceType, TagKey, and TagValue. Real AWS DescribeTags supports these filters.
func (h *Handler) handleDescribeTags(vals url.Values) (any, error) {
	resourceName := vals.Get("ResourceName")
	resourceType := vals.Get("ResourceType")
	// Real DescribeTagsInput.TagKeys/TagValues are []string, wire-encoded as the
	// indexed lists "TagKeys.TagKey.N"/"TagValues.TagValue.N" (confirmed against
	// awsAwsquery_serializeDocumentTagKeyList/TagValueList, redshift@v1.65.4
	// serializers.go) -- a real client never sends the bare "TagKey"/"TagValue".
	tagKeys := parseRedshiftTagKeysAt(vals, "TagKeys.TagKey.")
	tagValues := parseRedshiftTagKeysAt(vals, "TagValues.TagValue.")

	allTags := h.Backend.DescribeTags()

	type describeTagsResult struct {
		XMLName         xml.Name                 `xml:"DescribeTagsResult"`
		Marker          string                   `xml:"Marker,omitempty"`
		TaggedResources []redshiftTaggedResource `xml:"TaggedResources>TaggedResource,omitempty"`
	}
	type response struct {
		XMLName            xml.Name           `xml:"DescribeTagsResponse"`
		Xmlns              string             `xml:"xmlns,attr"`
		DescribeTagsResult describeTagsResult `xml:"DescribeTagsResult"`
	}

	// ResourceType filter: only "cluster" resources are currently stored.
	if resourceType != "" && resourceType != keyResourceCluster {
		return &response{Xmlns: redshiftXMLNS}, nil
	}

	var resources []redshiftTaggedResource

	for clusterID, tags := range allTags {
		if resourceName != "" {
			// Accept exact cluster-ID match or ARN suffix match.
			if clusterID != resourceName && !strings.HasSuffix(resourceName, ":cluster:"+clusterID) {
				continue
			}
		}

		for k, v := range tags {
			if !tagMatchesFilter(k, v, tagKeys, tagValues) {
				continue
			}

			resources = append(resources, redshiftTaggedResource{
				Tag:          svcTags.KV{Key: k, Value: v},
				ResourceName: clusterID,
				ResourceType: keyResourceCluster,
			})
		}
	}

	return &response{
		Xmlns: redshiftXMLNS,
		DescribeTagsResult: describeTagsResult{
			TaggedResources: resources,
		},
	}, nil
}

func (h *Handler) handleCreateTags(vals url.Values) (any, error) {
	clusterID := vals.Get("ResourceName")
	tags := parseRedshiftTags(vals)

	if err := h.Backend.CreateTags(clusterID, tags); err != nil {
		return nil, err
	}

	type response struct {
		XMLName xml.Name `xml:"CreateTagsResponse"`
		Xmlns   string   `xml:"xmlns,attr"`
	}

	return &response{Xmlns: redshiftXMLNS}, nil
}

func (h *Handler) handleDeleteTags(vals url.Values) (any, error) {
	clusterID := vals.Get("ResourceName")
	keys := parseRedshiftTagKeys(vals)

	if err := h.Backend.DeleteTags(clusterID, keys); err != nil {
		return nil, err
	}

	type response struct {
		XMLName xml.Name `xml:"DeleteTagsResponse"`
		Xmlns   string   `xml:"xmlns,attr"`
	}

	return &response{Xmlns: redshiftXMLNS}, nil
}

// parseRedshiftTags extracts Tags.Tag.N.Key/Tags.Tag.N.Value from form values.
// At most maxListItems tags are returned to prevent resource exhaustion.
// Returns as soon as an empty key is found (tags are expected to be consecutive).
func parseRedshiftTags(vals url.Values) map[string]string {
	tags := make(map[string]string)

	for i := 1; i <= maxListItems; i++ {
		prefix := fmt.Sprintf("Tags.Tag.%d.", i)
		key := vals.Get(prefix + "Key")

		if key == "" {
			// Tags are 1-indexed and consecutive; first missing key ends iteration.
			return tags
		}

		tags[key] = vals.Get(prefix + "Value")
	}

	// maxListItems exhausted without finding a missing key.
	return tags
}

// parseRedshiftTagKeys extracts TagKeys.TagKey.N from form values.
func parseRedshiftTagKeys(vals url.Values) []string {
	return parseRedshiftTagKeysAt(vals, "TagKeys.TagKey.")
}

// parseRedshiftTagKeysAt extracts a "<prefix>N"-indexed string list (e.g.
// TagKeys.TagKey.N or TagValues.TagValue.N) from form values. At most
// maxListItems entries are returned to prevent resource exhaustion.
func parseRedshiftTagKeysAt(vals url.Values, prefix string) []string {
	var keys []string

	for i := 1; i <= maxListItems; i++ {
		key := vals.Get(fmt.Sprintf("%s%d", prefix, i))
		if key == "" {
			return keys
		}

		keys = append(keys, key)
	}

	return keys
}

const maxListItems = 1000

// tagMatchesFilter reports whether a single tag (k, v) satisfies a
// TagKeys/TagValues filter pair. Matches real AWS's documented OR semantics
// (e.g. DescribeTags/DescribeUsageLimits/DescribeHsmClientCertificates docs:
// "If you specify both tag keys and tag values ... returns ... resources that
// have either or both of these tag keys/values"). Empty filter lists impose
// no constraint; if only one list is non-empty, matching is on that list alone.
func tagMatchesFilter(k, v string, tagKeys, tagValues []string) bool {
	if len(tagKeys) == 0 && len(tagValues) == 0 {
		return true
	}

	return slices.Contains(tagKeys, k) || slices.Contains(tagValues, v)
}

// anyTagMatchesFilter reports whether any tag in tags satisfies the
// TagKeys/TagValues filter pair -- for resource-level (not per-tag-entry)
// Describe* responses where the whole resource is included or excluded.
func anyTagMatchesFilter(tags map[string]string, tagKeys, tagValues []string) bool {
	if len(tagKeys) == 0 && len(tagValues) == 0 {
		return true
	}

	for k, v := range tags {
		if tagMatchesFilter(k, v, tagKeys, tagValues) {
			return true
		}
	}

	return false
}

// describeTaggedGroup implements the shared shape of DescribeClusterSubnetGroups
// and DescribeClusterSecurityGroups: an exact-name lookup that bypasses
// marker/maxRecords/tag filters (matching DescribeClusters' id-lookup
// shortcut in store.go), otherwise the TagKeys/TagValues-then-Marker/
// MaxRecords convention via paginateTaggedByName.
func describeTaggedGroup[V any](
	table *store.Table[V], name, marker string, maxRecords int, tagKeys, tagValues []string,
	notFound func(name string) error, clone func(*V) V,
	tagsOf func(*V) map[string]string, nameOf func(*V) string,
) ([]V, string, error) {
	if name != "" {
		v, exists := table.Get(name)
		if !exists {
			return nil, "", notFound(name)
		}

		return []V{clone(v)}, "", nil
	}

	sorted, nextMarker := paginateTaggedByName(table.Snapshot(), marker, maxRecords, tagKeys, tagValues, tagsOf, nameOf)

	result := make([]V, 0, len(sorted))
	for _, v := range sorted {
		result = append(result, clone(v))
	}

	return result, nextMarker, nil
}

// paginateTaggedByName applies the TagKeys/TagValues-then-Marker/MaxRecords
// convention shared by DescribeClusterSubnetGroups and
// DescribeClusterSecurityGroups (see DescribeClusters in store.go for the
// canonical version of this convention, which this mirrors generically).
// tagsOf and nameOf extract each item's tag map and sort/marker key.
func paginateTaggedByName[V any](
	sorted []*V, marker string, maxRecords int, tagKeys, tagValues []string,
	tagsOf func(*V) map[string]string, nameOf func(*V) string,
) ([]*V, string) {
	if len(tagKeys) > 0 || len(tagValues) > 0 {
		filtered := make([]*V, 0, len(sorted))

		for _, v := range sorted {
			if anyTagMatchesFilter(tagsOf(v), tagKeys, tagValues) {
				filtered = append(filtered, v)
			}
		}

		sorted = filtered
	}

	if marker != "" {
		cut := 0
		for cut < len(sorted) && nameOf(sorted[cut]) <= marker {
			cut++
		}

		sorted = sorted[cut:]
	}

	nextMarker := ""
	if maxRecords > 0 && len(sorted) > maxRecords {
		sorted = sorted[:maxRecords]
		nextMarker = nameOf(sorted[len(sorted)-1])
	}

	return sorted, nextMarker
}

// tagMapToKVList converts a resource's stored tag map into the sorted []svcTags.KV
// shape used for the wire-level "Tags>Tag" list embedded directly on many Redshift
// resource responses (e.g. Integration.Tags, HsmClientCertificate.Tags -- see the
// real SDK's []types.Tag fields). Sorted by key for deterministic serialization.
func tagMapToKVList(tags map[string]string) []svcTags.KV {
	if len(tags) == 0 {
		return nil
	}

	kvs := make([]svcTags.KV, 0, len(tags))
	for k, v := range tags {
		kvs = append(kvs, svcTags.KV{Key: k, Value: v})
	}

	sort.Slice(kvs, func(i, j int) bool { return kvs[i].Key < kvs[j].Key })

	return kvs
}

// parseTagListPrefixed extracts a Prefix.Tag.N.Key/Prefix.Tag.N.Value tag list from
// form values, e.g. prefix="TagList" for CreateIntegration (whose real
// CreateIntegrationInput field is named TagList, not Tags -- confirmed against
// aws-sdk-go-v2/service/redshift@v1.62.3/serializers.go
// awsAwsquery_serializeOpDocumentCreateIntegrationInput). At most maxListItems tags
// are returned to prevent resource exhaustion.
func parseTagListPrefixed(vals url.Values, prefix string) map[string]string {
	tags := make(map[string]string)

	for i := 1; i <= maxListItems; i++ {
		p := fmt.Sprintf("%s.Tag.%d.", prefix, i)
		key := vals.Get(p + "Key")

		if key == "" {
			return tags
		}

		tags[key] = vals.Get(p + "Value")
	}

	return tags
}

// parseStringList extracts a numbered list from form values using the given prefix.
// e.g. prefix="SnapshotIdentifierList.SnapshotIdentifier." yields elements at indices 1, 2, ...
// At most maxListItems items are returned to prevent resource exhaustion.
func parseStringList(vals url.Values, prefix string) []string {
	var result []string

	for i := 1; i <= maxListItems; i++ {
		v := vals.Get(fmt.Sprintf("%s%d", prefix, i))
		if v == "" {
			return result
		}

		result = append(result, v)
	}

	return result
}

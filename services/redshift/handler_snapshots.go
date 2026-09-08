package redshift

import (
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"time"
)

// ---- CreateClusterSnapshot ----

type createClusterSnapshotResponse struct {
	XMLName  xml.Name    `xml:"CreateClusterSnapshotResponse"`
	Xmlns    string      `xml:"xmlns,attr"`
	Snapshot xmlSnapshot `xml:"CreateClusterSnapshotResult>Snapshot"`
}

func (h *Handler) handleCreateClusterSnapshot(vals url.Values) (any, error) {
	snapshotID := vals.Get("SnapshotIdentifier")
	clusterID := vals.Get("ClusterIdentifier")

	snap, err := h.Backend.CreateClusterSnapshot(snapshotID, clusterID)
	if err != nil {
		return nil, err
	}

	return &createClusterSnapshotResponse{
		Xmlns:    redshiftXMLNS,
		Snapshot: snapshotToXML(snap),
	}, nil
}

// ---- DeleteClusterSnapshot ----

type deleteClusterSnapshotResponse struct {
	XMLName  xml.Name    `xml:"DeleteClusterSnapshotResponse"`
	Xmlns    string      `xml:"xmlns,attr"`
	Snapshot xmlSnapshot `xml:"DeleteClusterSnapshotResult>Snapshot"`
}

func (h *Handler) handleDeleteClusterSnapshot(vals url.Values) (any, error) {
	snapshotID := vals.Get("SnapshotIdentifier")

	snap, err := h.Backend.DeleteClusterSnapshot(snapshotID)
	if err != nil {
		return nil, err
	}

	return &deleteClusterSnapshotResponse{
		Xmlns:    redshiftXMLNS,
		Snapshot: snapshotToXML(snap),
	}, nil
}

// ---- DescribeClusterSnapshots ----

type xmlSnapshotList struct {
	Members []xmlSnapshot `xml:"Snapshot"`
}

type describeClusterSnapshotsResponse struct {
	XMLName   xml.Name        `xml:"DescribeClusterSnapshotsResponse"`
	Xmlns     string          `xml:"xmlns,attr"`
	Marker    string          `xml:"DescribeClusterSnapshotsResult>Marker,omitempty"`
	Snapshots xmlSnapshotList `xml:"DescribeClusterSnapshotsResult>Snapshots"`
}

const (
	defaultSnapshotPageSize = 100
	maxSnapshotPageSize     = 100
)

func (h *Handler) handleDescribeClusterSnapshots(vals url.Values) (any, error) {
	snapshotID := vals.Get("SnapshotIdentifier")
	clusterID := vals.Get("ClusterIdentifier")
	snapshotType := vals.Get("SnapshotType")
	markerStr := vals.Get("Marker")
	maxRecordsStr := vals.Get("MaxRecords")

	snaps, err := h.Backend.DescribeClusterSnapshots(snapshotID, clusterID, snapshotType, parseClusterExists(vals))
	if err != nil {
		return nil, err
	}

	snaps, err = filterSnapshotsByTimeRange(snaps, vals.Get("StartTime"), vals.Get("EndTime"))
	if err != nil {
		return nil, err
	}

	sortSnapshots(snaps, parseSnapshotSortingEntities(vals))

	pageSize := defaultSnapshotPageSize
	if maxRecordsStr != "" {
		n, parseErr := strconv.Atoi(maxRecordsStr)
		if parseErr != nil || n < 20 || n > maxSnapshotPageSize {
			return nil, fmt.Errorf(
				"%w: MaxRecords must be between 20 and %d", ErrInvalidParameter, maxSnapshotPageSize,
			)
		}

		pageSize = n
	}

	startIdx := 0

	if markerStr != "" {
		decoded, decErr := base64.StdEncoding.DecodeString(markerStr)
		if decErr != nil {
			return nil, fmt.Errorf("%w: invalid Marker", ErrInvalidParameter)
		}

		afterID := string(decoded)

		for i, s := range snaps {
			if s.SnapshotIdentifier == afterID {
				startIdx = i + 1

				break
			}
		}
	}

	end := min(startIdx+pageSize, len(snaps))

	page := snaps[startIdx:end]

	var nextMarker string

	if end < len(snaps) {
		nextMarker = base64.StdEncoding.EncodeToString([]byte(snaps[end-1].SnapshotIdentifier))
	}

	members := make([]xmlSnapshot, 0, len(page))
	for _, s := range page {
		sp := s
		members = append(members, snapshotToXML(&sp))
	}

	return &describeClusterSnapshotsResponse{
		Xmlns:     redshiftXMLNS,
		Marker:    nextMarker,
		Snapshots: xmlSnapshotList{Members: members},
	}, nil
}

// parseClusterExists parses the optional ClusterExists request parameter into a
// tri-state *bool, matching DescribeClusterSnapshotsInput's *bool ClusterExists
// field (nil means "not specified", distinct from explicit false).
func parseClusterExists(vals url.Values) *bool {
	v := vals.Get("ClusterExists")
	if v == "" {
		return nil
	}

	b := v == paramValueTrue

	return &b
}

// snapshotSortEntity is a parsed SortingEntities.SnapshotSortingEntity entry
// (real DescribeClusterSnapshotsInput.SortingEntities, redshift@v1.65.4
// api_op_DescribeClusterSnapshots.go, wire-encoded as
// "SortingEntities.SnapshotSortingEntity.N.Attribute"/".SortOrder" per
// awsAwsquery_serializeDocumentSnapshotSortingEntityList, serializers.go).
type snapshotSortEntity struct {
	attribute  string
	descending bool
}

// parseSnapshotSortingEntities extracts SortingEntities.SnapshotSortingEntity.N
// entries. Attribute is required per-entry (types.SnapshotSortingEntity);
// SortOrder defaults to ascending (types.SortByOrderAscending) when omitted,
// matching every other ascending-by-default sort in this service.
func parseSnapshotSortingEntities(vals url.Values) []snapshotSortEntity {
	var entities []snapshotSortEntity

	for i := 1; i <= maxListItems; i++ {
		prefix := fmt.Sprintf("SortingEntities.SnapshotSortingEntity.%d.", i)

		attr := vals.Get(prefix + "Attribute")
		if attr == "" {
			return entities
		}

		entities = append(entities, snapshotSortEntity{
			attribute:  attr,
			descending: vals.Get(prefix+"SortOrder") == "DESC",
		})
	}

	return entities
}

// sortSnapshots stably sorts snaps in place by entities, applied in order so
// earlier entities take precedence (matching the field's plural name -- a
// multi-key sort). SOURCE_TYPE (types.SnapshotAttributeToSortBySourceType)
// and CREATE_TIME (...ByCreateTime) are backed by real Snapshot fields.
// TOTAL_SIZE (...ByTotalSize) is intentionally NOT applied: this backend
// does not model snapshot storage size anywhere (no field on Snapshot), and
// fabricating one just to satisfy this sort key would produce a plausible
// but meaningless order -- see .claude/memories/parity-principles.md's
// no-stub rule. An unrecognized or TOTAL_SIZE entity is skipped (its
// relative order is left as-is by the stable sort) rather than faked.
func sortSnapshots(snaps []Snapshot, entities []snapshotSortEntity) {
	for _, e := range slices.Backward(entities) {
		var less func(a, b *Snapshot) bool

		switch e.attribute {
		case "CREATE_TIME":
			less = func(a, b *Snapshot) bool { return a.SnapshotCreateTime.Before(b.SnapshotCreateTime) }
		case "SOURCE_TYPE":
			less = func(a, b *Snapshot) bool { return a.SnapshotType < b.SnapshotType }
		default:
			continue
		}

		sort.SliceStable(snaps, func(i, j int) bool {
			if e.descending {
				return less(&snaps[j], &snaps[i])
			}

			return less(&snaps[i], &snaps[j])
		})
	}
}

// filterSnapshotsByTimeRange applies DescribeClusterSnapshotsInput.StartTime/
// EndTime (inclusive bounds on Snapshot.SnapshotCreateTime, per
// api_op_DescribeClusterSnapshots.go) before pagination, matching this
// service's filter-before-paginate convention. Empty strings impose no bound.
func filterSnapshotsByTimeRange(snaps []Snapshot, startStr, endStr string) ([]Snapshot, error) {
	if startStr == "" && endStr == "" {
		return snaps, nil
	}

	var startTime, endTime time.Time

	if startStr != "" {
		t, err := time.Parse(time.RFC3339, startStr)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid StartTime", ErrInvalidParameter)
		}

		startTime = t
	}

	if endStr != "" {
		t, err := time.Parse(time.RFC3339, endStr)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid EndTime", ErrInvalidParameter)
		}

		endTime = t
	}

	filtered := make([]Snapshot, 0, len(snaps))

	for _, s := range snaps {
		if startStr != "" && s.SnapshotCreateTime.Before(startTime) {
			continue
		}

		if endStr != "" && s.SnapshotCreateTime.After(endTime) {
			continue
		}

		filtered = append(filtered, s)
	}

	return filtered, nil
}

// ---- CopyClusterSnapshot ----

type copyClusterSnapshotResponse struct {
	XMLName  xml.Name    `xml:"CopyClusterSnapshotResponse"`
	Xmlns    string      `xml:"xmlns,attr"`
	Snapshot xmlSnapshot `xml:"CopyClusterSnapshotResult>Snapshot"`
}

func (h *Handler) handleCopyClusterSnapshot(vals url.Values) (any, error) {
	sourceSnapshotID := vals.Get("SourceSnapshotIdentifier")
	destinationSnapshotID := vals.Get("TargetSnapshotIdentifier")

	snap, err := h.Backend.CopyClusterSnapshot(sourceSnapshotID, destinationSnapshotID)
	if err != nil {
		return nil, err
	}

	return &copyClusterSnapshotResponse{
		Xmlns:    redshiftXMLNS,
		Snapshot: snapshotToXML(snap),
	}, nil
}

// ---- RestoreFromClusterSnapshot ----

type restoreFromClusterSnapshotResponse struct {
	XMLName xml.Name   `xml:"RestoreFromClusterSnapshotResponse"`
	Xmlns   string     `xml:"xmlns,attr"`
	Cluster xmlCluster `xml:"RestoreFromClusterSnapshotResult>Cluster"`
}

func (h *Handler) handleRestoreFromClusterSnapshot(vals url.Values) (any, error) {
	clusterID := vals.Get("ClusterIdentifier")
	snapshotID := vals.Get("SnapshotIdentifier")

	cluster, err := h.Backend.RestoreFromClusterSnapshot(clusterID, snapshotID)
	if err != nil {
		return nil, err
	}

	return &restoreFromClusterSnapshotResponse{
		Xmlns:   redshiftXMLNS,
		Cluster: h.toXMLCluster(cluster),
	}, nil
}

// ---- AuthorizeSnapshotAccess ----

type xmlAccountWithRestoreAccess struct {
	AccountID    string `xml:"AccountId"`
	AccountAlias string `xml:"AccountAlias,omitempty"`
}

type xmlRestoreAccessList struct {
	Members []xmlAccountWithRestoreAccess `xml:"AccountWithRestoreAccess,omitempty"`
}

type xmlSnapshot struct {
	SnapshotIdentifier            string               `xml:"SnapshotIdentifier"`
	ClusterIdentifier             string               `xml:"ClusterIdentifier"`
	SnapshotType                  string               `xml:"SnapshotType,omitempty"`
	SnapshotCreateTime            string               `xml:"SnapshotCreateTime,omitempty"`
	Status                        string               `xml:"Status"`
	AccountsWithRestoreAccess     xmlRestoreAccessList `xml:"AccountsWithRestoreAccess"`
	ManualSnapshotRetentionPeriod int                  `xml:"ManualSnapshotRetentionPeriod"`
}

type authorizeSnapshotAccessResponse struct {
	XMLName  xml.Name    `xml:"AuthorizeSnapshotAccessResponse"`
	Xmlns    string      `xml:"xmlns,attr"`
	Snapshot xmlSnapshot `xml:"AuthorizeSnapshotAccessResult>Snapshot"`
}

func snapshotToXML(snap *Snapshot) xmlSnapshot {
	accounts := make([]xmlAccountWithRestoreAccess, 0, len(snap.AccountsWithRestoreAccess))
	for _, a := range snap.AccountsWithRestoreAccess {
		accounts = append(accounts, xmlAccountWithRestoreAccess(a))
	}

	var createTime string
	if !snap.SnapshotCreateTime.IsZero() {
		createTime = snap.SnapshotCreateTime.UTC().Format(time.RFC3339)
	}

	return xmlSnapshot{
		SnapshotIdentifier:            snap.SnapshotIdentifier,
		ClusterIdentifier:             snap.ClusterIdentifier,
		SnapshotType:                  snap.SnapshotType,
		SnapshotCreateTime:            createTime,
		Status:                        snap.Status,
		ManualSnapshotRetentionPeriod: snap.ManualSnapshotRetentionPeriod,
		AccountsWithRestoreAccess:     xmlRestoreAccessList{Members: accounts},
	}
}

func (h *Handler) handleAuthorizeSnapshotAccess(vals url.Values) (any, error) {
	snapshotID := vals.Get("SnapshotIdentifier")
	accountWithRestoreAccess := vals.Get("AccountWithRestoreAccess")

	snap, err := h.Backend.AuthorizeSnapshotAccess(snapshotID, accountWithRestoreAccess)
	if err != nil {
		return nil, err
	}

	return &authorizeSnapshotAccessResponse{
		Xmlns:    redshiftXMLNS,
		Snapshot: snapshotToXML(snap),
	}, nil
}

// ---- BatchDeleteClusterSnapshots ----

type xmlSnapshotErrorMessage struct {
	SnapshotIdentifier        string `xml:"SnapshotIdentifier"`
	SnapshotClusterIdentifier string `xml:"SnapshotClusterIdentifier,omitempty"`
	FailureCode               string `xml:"FailureCode"`
	FailureReason             string `xml:"FailureReason"`
}

type batchDeleteClusterSnapshotsResponse struct {
	XMLName   xml.Name                  `xml:"BatchDeleteClusterSnapshotsResponse"`
	Xmlns     string                    `xml:"xmlns,attr"`
	Errors    []xmlSnapshotErrorMessage `xml:"BatchDeleteClusterSnapshotsResult>Errors>SnapshotErrorMessage,omitempty"`
	Resources []string                  `xml:"BatchDeleteClusterSnapshotsResult>Resources>String,omitempty"`
}

// parseDeleteClusterSnapshotMessageIdentifiers reads Identifiers, a list of
// DeleteClusterSnapshotMessage structs (each with a required SnapshotIdentifier
// and optional SnapshotClusterIdentifier member), not a flat string list.
// Confirmed against aws-sdk-go-v2/service/redshift@v1.65.4/serializers.go:
// awsAwsquery_serializeDocumentDeleteClusterSnapshotMessageList wraps the array
// in "DeleteClusterSnapshotMessage" and awsAwsquery_serializeDocumentDeleteClusterSnapshotMessage
// serializes SnapshotIdentifier as a nested object field, so the real wire key
// is "Identifiers.DeleteClusterSnapshotMessage.N.SnapshotIdentifier" -- neither
// "Identifiers.DeleteClusterSnapshotMessage.N" nor "Identifiers.SnapshotIdentifier.N"
// (the two shapes this parser previously tried) ever matches a real request, so a
// real client's snapshot identifiers were always silently dropped.
func parseDeleteClusterSnapshotMessageIdentifiers(vals url.Values) []string {
	var identifiers []string

	for i := 1; i <= maxListItems; i++ {
		v := vals.Get(fmt.Sprintf("Identifiers.DeleteClusterSnapshotMessage.%d.SnapshotIdentifier", i))
		if v == "" {
			break
		}

		identifiers = append(identifiers, v)
	}

	return identifiers
}

func (h *Handler) handleBatchDeleteClusterSnapshots(vals url.Values) (any, error) {
	identifiers := parseDeleteClusterSnapshotMessageIdentifiers(vals)

	batchErrors, deleted := h.Backend.BatchDeleteClusterSnapshots(identifiers)

	xmlErrors := make([]xmlSnapshotErrorMessage, 0, len(batchErrors))
	for _, e := range batchErrors {
		xmlErrors = append(xmlErrors, xmlSnapshotErrorMessage(e))
	}

	resources := make([]string, len(deleted))
	copy(resources, deleted)

	return &batchDeleteClusterSnapshotsResponse{
		Xmlns:     redshiftXMLNS,
		Errors:    xmlErrors,
		Resources: resources,
	}, nil
}

// ---- BatchModifyClusterSnapshots ----

type batchModifyClusterSnapshotsResponse struct {
	XMLName   xml.Name                  `xml:"BatchModifyClusterSnapshotsResponse"`
	Xmlns     string                    `xml:"xmlns,attr"`
	Errors    []xmlSnapshotErrorMessage `xml:"BatchModifyClusterSnapshotsResult>Errors>SnapshotErrorMessage,omitempty"`
	Resources []string                  `xml:"BatchModifyClusterSnapshotsResult>Resources>String,omitempty"`
}

func (h *Handler) handleBatchModifyClusterSnapshots(vals url.Values) (any, error) {
	identifiers := parseStringList(vals, "SnapshotIdentifierList.String.")
	force := vals.Get("Force") == paramValueTrue

	var retentionPeriod *int

	if v := vals.Get("ManualSnapshotRetentionPeriod"); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
			return nil, fmt.Errorf("%w: ManualSnapshotRetentionPeriod must be an integer", ErrInvalidParameter)
		}

		retentionPeriod = &n
	}

	batchErrors, modified := h.Backend.BatchModifyClusterSnapshots(identifiers, retentionPeriod, force)

	xmlErrors := make([]xmlSnapshotErrorMessage, 0, len(batchErrors))
	for _, e := range batchErrors {
		xmlErrors = append(xmlErrors, xmlSnapshotErrorMessage(e))
	}

	resources := make([]string, len(modified))
	copy(resources, modified)

	return &batchModifyClusterSnapshotsResponse{
		Xmlns:     redshiftXMLNS,
		Errors:    xmlErrors,
		Resources: resources,
	}, nil
}

// ---- RevokeSnapshotAccess ----

type revokeSnapshotAccessResponse struct {
	XMLName  xml.Name    `xml:"RevokeSnapshotAccessResponse"`
	Xmlns    string      `xml:"xmlns,attr"`
	Snapshot xmlSnapshot `xml:"RevokeSnapshotAccessResult>Snapshot"`
}

func (h *Handler) handleRevokeSnapshotAccess(vals url.Values) (any, error) {
	snapshotID := vals.Get("SnapshotIdentifier")
	accountWithRestoreAccess := vals.Get("AccountWithRestoreAccess")

	snap, err := h.Backend.RevokeSnapshotAccess(snapshotID, accountWithRestoreAccess)
	if err != nil {
		return nil, err
	}

	return &revokeSnapshotAccessResponse{
		Xmlns:    redshiftXMLNS,
		Snapshot: snapshotToXML(snap),
	}, nil
}

// ---- ModifyClusterSnapshot ----

type modifyClusterSnapshotResponse struct {
	XMLName  xml.Name    `xml:"ModifyClusterSnapshotResponse"`
	Xmlns    string      `xml:"xmlns,attr"`
	Snapshot xmlSnapshot `xml:"ModifyClusterSnapshotResult>Snapshot"`
}

func (h *Handler) handleModifyClusterSnapshot(vals url.Values) (any, error) {
	snapshotID := vals.Get("SnapshotIdentifier")
	force := vals.Get("Force") == paramValueTrue

	var retentionPeriod *int

	if v := vals.Get("ManualSnapshotRetentionPeriod"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, ErrInvalidParameter
		}

		retentionPeriod = &n
	}

	snap, err := h.Backend.ModifyClusterSnapshot(snapshotID, retentionPeriod, force)
	if err != nil {
		return nil, err
	}

	return &modifyClusterSnapshotResponse{
		Xmlns:    redshiftXMLNS,
		Snapshot: snapshotToXML(snap),
	}, nil
}

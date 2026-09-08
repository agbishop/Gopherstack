package elasticache

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
)

type logDeliveryDestinationDetailsXML struct {
	LogGroup       string `xml:"CloudWatchLogsDetails>LogGroup,omitempty"`
	DeliveryStream string `xml:"KinesisFirehoseDetails>DeliveryStream,omitempty"`
}

type logDeliveryConfigXML struct {
	DestinationDetails logDeliveryDestinationDetailsXML `xml:"DestinationDetails"`
	LogType            string                           `xml:"LogType,omitempty"`
	DestinationType    string                           `xml:"DestinationType,omitempty"`
	LogFormat          string                           `xml:"LogFormat,omitempty"`
	Status             string                           `xml:"Status,omitempty"`
	Message            string                           `xml:"Message,omitempty"`
}

type logDeliveryConfigsXML struct {
	LogDeliveryConfiguration []logDeliveryConfigXML `xml:"LogDeliveryConfiguration"`
}

func logDeliveryConfigsToXML(configs []LogDeliveryConfig) *logDeliveryConfigsXML {
	if len(configs) == 0 {
		return nil
	}

	items := make([]logDeliveryConfigXML, 0, len(configs))

	for _, c := range configs {
		item := logDeliveryConfigXML{
			LogType:         c.LogType,
			DestinationType: c.DestinationType,
			LogFormat:       c.LogFormat,
			Status:          c.Status,
			Message:         c.Message,
		}

		switch c.DestinationType {
		case "kinesis-firehose":
			item.DestinationDetails = logDeliveryDestinationDetailsXML{
				DeliveryStream: c.DestinationDetails,
			}
		default:
			item.DestinationDetails = logDeliveryDestinationDetailsXML{
				LogGroup: c.DestinationDetails,
			}
		}

		items = append(items, item)
	}

	return &logDeliveryConfigsXML{LogDeliveryConfiguration: items}
}

func (h *Handler) createReplicationGroup(ctx context.Context, c *echo.Context, form url.Values) error {
	opts := parseCreateReplicationGroupOpts(form)

	rg, err := h.Backend.CreateReplicationGroupFull(ctx, opts)
	if err != nil {
		return mapReplicationGroupCreateErr(c, err)
	}

	type result struct {
		XMLName          xml.Name            `xml:"CreateReplicationGroupResponse"`
		Xmlns            string              `xml:"xmlns,attr"`
		ReplicationGroup replicationGroupXML `xml:"CreateReplicationGroupResult>ReplicationGroup"`
	}

	return xmlResp(c, http.StatusOK, result{
		Xmlns:            elasticacheNS,
		ReplicationGroup: rgToXML(*rg),
	})
}

// parseCreateReplicationGroupOpts extracts all create options from a form submission.
func parseCreateReplicationGroupOpts(form url.Values) ReplicationGroupCreateOpts {
	opts := ReplicationGroupCreateOpts{
		ID:                    form.Get("ReplicationGroupId"),
		Description:           form.Get("ReplicationGroupDescription"),
		ParameterGroupName:    form.Get("CacheParameterGroupName"),
		SnapshotName:          form.Get("SnapshotName"),
		MaintenanceWindow:     form.Get("PreferredMaintenanceWindow"),
		SnapshotWindow:        form.Get("SnapshotWindow"),
		AuthToken:             form.Get("AuthToken"),
		KmsKeyID:              form.Get("KmsKeyId"),
		NotificationTopicArn:  form.Get("NotificationTopicArn"),
		TransitEncryptionMode: form.Get("TransitEncryptionMode"),
		Engine:                form.Get("Engine"),
		EngineVersion:         form.Get("EngineVersion"),
		CacheNodeType:         form.Get("CacheNodeType"),
		Durability:            form.Get("Durability"),

		AuthTokenEnabled: !strings.EqualFold(form.Get("AuthToken"), "") ||
			strings.EqualFold(form.Get("AuthTokenEnabled"), "true"),
		AtRestEncryptionEnabled:  strings.EqualFold(form.Get("AtRestEncryptionEnabled"), "true"),
		TransitEncryptionEnabled: strings.EqualFold(form.Get("TransitEncryptionEnabled"), "true"),
		ClusterModeEnabled: strings.EqualFold(form.Get("ClusterModeEnabled"), "true") ||
			strings.EqualFold(form.Get("ClusterMode"), "enabled"),
		DataTieringEnabled:       strings.EqualFold(form.Get("DataTieringEnabled"), "true"),
		MultiAZEnabled:           strings.EqualFold(form.Get("MultiAZEnabled"), "true"),
		AutomaticFailoverEnabled: strings.EqualFold(form.Get("AutomaticFailoverEnabled"), "true")}

	if s := form.Get("SnapshotRetentionLimit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			opts.SnapshotRetentionLimit = n
		}
	}

	if s := form.Get("NumNodeGroups"); s != "" {
		if n, err := strconv.ParseInt(s, 10, 32); err == nil {
			opts.NumNodeGroups = int32(n)
		}
	}

	if s := form.Get("ReplicasPerNodeGroup"); s != "" {
		if n, err := strconv.ParseInt(s, 10, 32); err == nil {
			opts.ReplicasPerNodeGroup = int32(n)
		}
	}

	// Parse UserGroupIds.
	for i := 1; ; i++ {
		id := form.Get(fmt.Sprintf("UserGroupIds.member.%d", i))
		if id == "" {
			break
		}
		opts.UserGroupIDs = append(opts.UserGroupIDs, id)
	}

	// Parse LogDeliveryConfigurations.
	for i := 1; ; i++ {
		prefix := fmt.Sprintf("LogDeliveryConfigurations.member.%d.", i)
		logType := form.Get(prefix + "LogType")
		if logType == "" {
			break
		}

		destType := form.Get(prefix + "DestinationType")
		logFormat := form.Get(prefix + "LogFormat")
		destDetails := form.Get(prefix + "DestinationDetails.CloudWatchLogsDetails.LogGroup")
		if destDetails == "" {
			destDetails = form.Get(prefix + "DestinationDetails.KinesisFirehoseDetails.DeliveryStream")
		}

		opts.LogDeliveryConfigurations = append(opts.LogDeliveryConfigurations, LogDeliveryConfig{
			LogType:            logType,
			DestinationType:    destType,
			LogFormat:          logFormat,
			DestinationDetails: destDetails,
			Status:             statusEnabled,
		})
	}

	// Parse Tags.
	if t := parseFormTags(form); len(t) > 0 {
		opts.Tags = t
	}

	return opts
}

// mapReplicationGroupCreateErr maps backend errors to XML error responses.
func mapReplicationGroupCreateErr(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, ErrReplicationGroupAlreadyExists):
		return xmlError(c, http.StatusBadRequest, "ReplicationGroupAlreadyExists", "Replication group already exists")
	case errors.Is(err, ErrParameterGroupNotFound):
		return xmlError(c, http.StatusNotFound, "CacheParameterGroupNotFound", "Cache parameter group not found")
	case errors.Is(err, ErrSnapshotNotFound):
		// Same rationale as createCacheCluster: SnapshotNotFoundFault isn't in
		// CreateReplicationGroup's modeled error list either.
		return xmlError(c, http.StatusBadRequest, "InvalidParameterValue", "Cache cluster snapshot not found")
	case errors.Is(err, ErrDataTieringInvalid):
		return xmlError(c, http.StatusBadRequest, "InvalidParameterValue", err.Error())
	case errors.Is(err, ErrAuthTokenRequiredForMode):
		return xmlError(c, http.StatusBadRequest, "InvalidParameterCombination", err.Error())
	default:
		return xmlError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}
}

func (h *Handler) deleteReplicationGroup(ctx context.Context, c *echo.Context, form url.Values) error {
	id := form.Get("ReplicationGroupId")
	rgs, descErr := h.Backend.DescribeReplicationGroups(ctx, id, "", 0)
	if descErr != nil {
		if errors.Is(descErr, ErrReplicationGroupNotFound) {
			return xmlError(c, http.StatusNotFound, "ReplicationGroupNotFoundFault", "Replication group not found")
		}

		return xmlError(c, http.StatusInternalServerError, "InternalFailure", descErr.Error())
	}
	if len(rgs.Data) == 0 {
		return xmlError(c, http.StatusNotFound, "ReplicationGroupNotFoundFault", "Replication group not found")
	}

	rg := rgs.Data[0]
	if err := h.Backend.DeleteReplicationGroup(ctx, id); err != nil {
		if errors.Is(err, ErrReplicationGroupNotFound) {
			return xmlError(c, http.StatusNotFound, "ReplicationGroupNotFoundFault", "Replication group not found")
		}
		if errors.Is(err, ErrReplicationGroupNotAvailable) {
			return xmlError(c, http.StatusBadRequest, "InvalidReplicationGroupState", err.Error())
		}

		return xmlError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	type replicationGroup struct {
		ReplicationGroupID string `xml:"ReplicationGroupId"`
		Description        string `xml:"Description"`
		Status             string `xml:"Status"`
		ARN                string `xml:"ARN"`
	}
	type result struct {
		XMLName          xml.Name         `xml:"DeleteReplicationGroupResponse"`
		Xmlns            string           `xml:"xmlns,attr"`
		ReplicationGroup replicationGroup `xml:"DeleteReplicationGroupResult>ReplicationGroup"`
	}

	return xmlResp(c, http.StatusOK, result{
		Xmlns: elasticacheNS,
		ReplicationGroup: replicationGroup{
			ReplicationGroupID: rg.ReplicationGroupID,
			Description:        rg.Description,
			Status:             "deleting",
			ARN:                rg.ARN,
		},
	})
}

// nodeGroupNodeXML is the XML for a single node within a node group.
type nodeGroupNodeXML struct {
	CacheClusterID            string        `xml:"CacheClusterId,omitempty"`
	CacheNodeID               string        `xml:"CacheNodeId,omitempty"`
	CurrentRole               string        `xml:"CurrentRole,omitempty"`
	PreferredAvailabilityZone string        `xml:"PreferredAvailabilityZone,omitempty"`
	ReadEndpoint              cacheEndpoint `xml:"ReadEndpoint,omitempty"`
}

// nodeGroupXML is the XML representation of a shard / node group.
type nodeGroupXML struct {
	NodeGroupID      string              `xml:"NodeGroupId"`
	Status           string              `xml:"Status"`
	Slots            string              `xml:"Slots,omitempty"`
	NodeGroupMembers nodeGroupMembersXML `xml:"NodeGroupMembers"`
}

type nodeGroupMembersXML struct {
	NodeGroupMember []nodeGroupNodeXML `xml:"NodeGroupMember"`
}

type nodeGroupsListXML struct {
	NodeGroup []nodeGroupXML `xml:"NodeGroup"`
}

// rgPendingModifiedXML is the XML for pending replication group changes.
type rgPendingModifiedXML struct {
	NumCacheNodes           *int32 `xml:"NumCacheNodes,omitempty"`
	CacheNodeType           string `xml:"CacheNodeType,omitempty"`
	EngineVersion           string `xml:"EngineVersion,omitempty"`
	AuthTokenStatus         string `xml:"AuthTokenStatus,omitempty"`
	AutomaticFailoverStatus string `xml:"AutomaticFailoverStatus,omitempty"`
}

// rgUserGroupIDsXML holds UserGroupId list in the XML response.
type rgUserGroupIDsXML struct {
	UserGroupID []string `xml:"member"`
}

// replicationGroupXML is the XML representation of a single replication
// group. Durability/EffectiveDurability/StorageEncryptionType
// (deserializers.go:21351/21364/21564, awsAwsquery_deserializeDocumentReplicationGroup)
// were added by the SDK after this service's last field diff
// (gopherstack-31dm). Durability is echoed from
// CreateReplicationGroupInput.Durability (serializers.go:6506) /
// ModifyReplicationGroupInput.Durability (serializers.go:8171) -- both real
// input members. EffectiveDurability and StorageEncryptionType have no
// Create/Modify input member (EffectiveDurability is server-resolved from
// engine version/cluster mode, StorageEncryptionType from KMS-key state) --
// deliberately left always empty rather than guessed, per parity-principles.md's
// no-fabrication rule.
type replicationGroupXML struct {
	PendingModifiedValues      *rgPendingModifiedXML  `xml:"PendingModifiedValues,omitempty"`
	NodeGroups                 *nodeGroupsListXML     `xml:"NodeGroups,omitempty"`
	UserGroupIDs               *rgUserGroupIDsXML     `xml:"UserGroupIds,omitempty"`
	LogDeliveryConfigurations  *logDeliveryConfigsXML `xml:"LogDeliveryConfigurations,omitempty"`
	ReplicationGroupID         string                 `xml:"ReplicationGroupId"`
	Description                string                 `xml:"Description"`
	Status                     string                 `xml:"Status"`
	ARN                        string                 `xml:"ARN"`
	Engine                     string                 `xml:"Engine,omitempty"`
	CacheParameterGroupName    string                 `xml:"CacheParameterGroupName,omitempty"`
	AutomaticFailover          string                 `xml:"AutomaticFailover,omitempty"`
	MultiAZ                    string                 `xml:"MultiAZ,omitempty"`
	CacheNodeType              string                 `xml:"CacheNodeType,omitempty"`
	SnapshotWindow             string                 `xml:"SnapshotWindow,omitempty"`
	PreferredMaintenanceWindow string                 `xml:"PreferredMaintenanceWindow,omitempty"`
	EngineVersion              string                 `xml:"EngineVersion,omitempty"`
	CreatedAt                  string                 `xml:"ReplicationGroupCreateTime,omitempty"`
	KmsKeyID                   string                 `xml:"KmsKeyId,omitempty"`
	NotificationTopicArn       string                 `xml:"NotificationTopicArn,omitempty"`
	TransitEncryptionMode      string                 `xml:"TransitEncryptionMode,omitempty"`
	DataTiering                string                 `xml:"DataTiering,omitempty"`
	Durability                 string                 `xml:"Durability,omitempty"`
	EffectiveDurability        string                 `xml:"EffectiveDurability,omitempty"`
	StorageEncryptionType      string                 `xml:"StorageEncryptionType,omitempty"`
	SnapshotRetentionLimit     int                    `xml:"SnapshotRetentionLimit,omitempty"`
	NumCacheClusters           int                    `xml:"NumCacheClusters,omitempty"`
	ClusterEnabled             bool                   `xml:"ClusterEnabled,omitempty"`
	AuthTokenEnabled           bool                   `xml:"AuthTokenEnabled,omitempty"`
	AtRestEncryptionEnabled    bool                   `xml:"AtRestEncryptionEnabled,omitempty"`
	TransitEncryptionEnabled   bool                   `xml:"TransitEncryptionEnabled,omitempty"`
}

// dataTieringStatus converts a bool to the AWS DataTieringStatus string.
func dataTieringStatus(enabled bool) string {
	if enabled {
		return statusEnabled
	}

	return ""
}

// nodeGroupsToXML converts backend NodeGroups to XML.
func nodeGroupsToXML(ngs []NodeGroup) *nodeGroupsListXML {
	if len(ngs) == 0 {
		return nil
	}

	xmlNGs := make([]nodeGroupXML, 0, len(ngs))
	for _, ng := range ngs {
		members := make([]nodeGroupNodeXML, 0)
		if ng.PrimaryNode != nil {
			members = append(members, nodeGroupNodeXML{
				CacheClusterID:            ng.PrimaryNode.CacheClusterID,
				CacheNodeID:               ng.PrimaryNode.CacheNodeID,
				CurrentRole:               "primary",
				PreferredAvailabilityZone: ng.PrimaryNode.PreferredAvailabilityZone,
			})
		}
		for _, r := range ng.Replicas {
			members = append(members, nodeGroupNodeXML{
				CacheClusterID:            r.CacheClusterID,
				CacheNodeID:               r.CacheNodeID,
				CurrentRole:               "replica",
				PreferredAvailabilityZone: r.PreferredAvailabilityZone,
			})
		}
		xmlNGs = append(xmlNGs, nodeGroupXML{
			NodeGroupID:      ng.NodeGroupID,
			Status:           ng.Status,
			Slots:            ng.Slots,
			NodeGroupMembers: nodeGroupMembersXML{NodeGroupMember: members},
		})
	}

	return &nodeGroupsListXML{NodeGroup: xmlNGs}
}

// pendingToXML converts RGPendingModifiedValues to XML.
func pendingToXML(p *RGPendingModifiedValues) *rgPendingModifiedXML {
	if p == nil {
		return nil
	}

	x := &rgPendingModifiedXML{
		CacheNodeType:           p.CacheNodeType,
		EngineVersion:           p.EngineVersion,
		AuthTokenStatus:         p.AuthTokenStatus,
		AutomaticFailoverStatus: p.AutomaticFailoverStatus,
	}
	if p.ReplicaCount != nil {
		rc := *p.ReplicaCount
		x.NumCacheNodes = &rc
	}

	return x
}

// rgToXML converts a ReplicationGroup to its XML representation.
func rgToXML(rg ReplicationGroup) replicationGroupXML {
	multiAZ := statusDisabled
	if rg.MultiAZEnabled {
		multiAZ = statusEnabled
	}

	autoFailover := rg.AutomaticFailover
	if autoFailover == "" {
		autoFailover = statusDisabled
	}

	numCacheClusters := int(rg.ReplicaCount) + 1
	if numCacheClusters <= 1 && !rg.ClusterModeEnabled {
		numCacheClusters = 1
	}

	var userGroupIDs *rgUserGroupIDsXML
	if len(rg.UserGroupIDs) > 0 {
		userGroupIDs = &rgUserGroupIDsXML{UserGroupID: rg.UserGroupIDs}
	}

	return replicationGroupXML{
		ReplicationGroupID:         rg.ReplicationGroupID,
		Description:                rg.Description,
		Status:                     rg.Status,
		ARN:                        rg.ARN,
		Engine:                     rg.Engine,
		CacheParameterGroupName:    rg.CacheParameterGroupName,
		AutomaticFailover:          autoFailover,
		MultiAZ:                    multiAZ,
		CacheNodeType:              rg.CacheNodeType,
		SnapshotWindow:             rg.SnapshotWindow,
		PreferredMaintenanceWindow: rg.PreferredMaintenanceWindow,
		EngineVersion:              rg.EngineVersion,
		CreatedAt:                  rg.CreatedAt.UTC().Format(time.RFC3339),
		KmsKeyID:                   rg.KmsKeyID,
		NotificationTopicArn:       rg.NotificationTopicArn,
		TransitEncryptionMode:      rg.TransitEncryptionMode,
		Durability:                 rg.Durability,
		SnapshotRetentionLimit:     rg.SnapshotRetentionLimit,
		NumCacheClusters:           numCacheClusters,
		ClusterEnabled:             rg.ClusterModeEnabled,
		AuthTokenEnabled:           rg.AuthTokenEnabled,
		AtRestEncryptionEnabled:    rg.AtRestEncryptionEnabled,
		TransitEncryptionEnabled:   rg.TransitEncryptionEnabled,
		DataTiering:                dataTieringStatus(rg.DataTieringEnabled),
		NodeGroups:                 nodeGroupsToXML(rg.NodeGroups),
		PendingModifiedValues:      pendingToXML(rg.PendingModifiedValues),
		UserGroupIDs:               userGroupIDs,
		LogDeliveryConfigurations:  logDeliveryConfigsToXML(rg.LogDeliveryConfigurations),
	}
}

// describeReplicationGroupsResultXML is the XML result for DescribeReplicationGroups.
type describeReplicationGroupsResultXML struct {
	XMLName           xml.Name                 `xml:"DescribeReplicationGroupsResponse"`
	Xmlns             string                   `xml:"xmlns,attr"`
	Marker            string                   `xml:"DescribeReplicationGroupsResult>Marker,omitempty"`
	ReplicationGroups replicationGroupsListXML `xml:"DescribeReplicationGroupsResult>ReplicationGroups"`
}

// replicationGroupsListXML holds the list of replication groups.
type replicationGroupsListXML struct {
	ReplicationGroup []replicationGroupXML `xml:"ReplicationGroup"`
}

func (h *Handler) describeReplicationGroups(ctx context.Context, c *echo.Context, form url.Values) error {
	id := form.Get("ReplicationGroupId")
	marker, maxRecords, err := parsePaginationChecked(c, form)
	if err != nil {
		return err
	}

	p, err := h.Backend.DescribeReplicationGroups(ctx, id, marker, maxRecords)
	if err != nil {
		if errors.Is(err, ErrReplicationGroupNotFound) {
			return xmlError(c, http.StatusNotFound, "ReplicationGroupNotFoundFault", "Replication group not found")
		}

		return xmlError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	items := make([]replicationGroupXML, 0, len(p.Data))
	for _, rg := range p.Data {
		items = append(items, rgToXML(rg))
	}

	return xmlResp(c, http.StatusOK, describeReplicationGroupsResultXML{
		Xmlns:             elasticacheNS,
		Marker:            p.Next,
		ReplicationGroups: replicationGroupsListXML{ReplicationGroup: items},
	})
}

func (h *Handler) modifyReplicationGroup(ctx context.Context, c *echo.Context, form url.Values) error {
	id := form.Get("ReplicationGroupId")
	opts := parseModifyReplicationGroupOpts(form)

	rg, err := h.Backend.ModifyReplicationGroupFull(ctx, id, opts)
	if err != nil {
		return mapReplicationGroupModifyErr(c, err)
	}

	type result struct {
		XMLName          xml.Name            `xml:"ModifyReplicationGroupResponse"`
		Xmlns            string              `xml:"xmlns,attr"`
		ReplicationGroup replicationGroupXML `xml:"ModifyReplicationGroupResult>ReplicationGroup"`
	}

	return xmlResp(c, http.StatusOK, result{
		Xmlns:            elasticacheNS,
		ReplicationGroup: rgToXML(*rg),
	})
}

// parseModifyReplicationGroupOpts extracts all modify options from a form.
func parseModifyReplicationGroupOpts(form url.Values) ReplicationGroupModifyOpts {
	opts := ReplicationGroupModifyOpts{
		Description:             form.Get("ReplicationGroupDescription"),
		ParameterGroupName:      form.Get("CacheParameterGroupName"),
		EngineVersion:           form.Get("EngineVersion"),
		CacheNodeType:           form.Get("CacheNodeType"),
		MaintenanceWindow:       form.Get("PreferredMaintenanceWindow"),
		SnapshotWindow:          form.Get("SnapshotWindow"),
		AuthToken:               form.Get("AuthToken"),
		AuthTokenUpdateStrategy: form.Get("AuthTokenUpdateStrategy"),
		NotificationTopicArn:    form.Get("NotificationTopicArn"),
		TransitEncryptionMode:   form.Get("TransitEncryptionMode"),
		Durability:              form.Get("Durability"),
		ApplyImmediately:        strings.EqualFold(form.Get("ApplyImmediately"), "true"),
	}

	if s := form.Get("AutomaticFailoverEnabled"); s != "" {
		v := strings.EqualFold(s, "true")
		opts.AutomaticFailoverEnabled = &v
	}

	if s := form.Get("MultiAZEnabled"); s != "" {
		v := strings.EqualFold(s, "true")
		opts.MultiAZEnabled = &v
	}

	if s := form.Get("SnapshotRetentionLimit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			opts.SnapshotRetentionLimit = &n
		}
	}

	if s := form.Get("ReplicaCount"); s != "" {
		if n, err := strconv.ParseInt(s, 10, 32); err == nil {
			rc := int32(n)
			opts.ReplicaCount = &rc
		}
	}

	// Parse UserGroupIds to add/remove.
	for i := 1; ; i++ {
		id := form.Get(fmt.Sprintf("UserGroupIdsToAdd.member.%d", i))
		if id == "" {
			break
		}
		opts.UserGroupIDsToAdd = append(opts.UserGroupIDsToAdd, id)
	}
	for i := 1; ; i++ {
		id := form.Get(fmt.Sprintf("UserGroupIdsToRemove.member.%d", i))
		if id == "" {
			break
		}
		opts.UserGroupIDsToRemove = append(opts.UserGroupIDsToRemove, id)
	}

	return opts
}

// mapReplicationGroupModifyErr maps backend errors to XML error responses.
func mapReplicationGroupModifyErr(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, ErrReplicationGroupNotFound):
		return xmlError(c, http.StatusNotFound, "ReplicationGroupNotFoundFault", "Replication group not found")
	case errors.Is(err, ErrParameterGroupNotFound):
		return xmlError(c, http.StatusNotFound, "CacheParameterGroupNotFound", "Cache parameter group not found")
	case errors.Is(err, ErrTransitEncryptionModeInvalid):
		return xmlError(c, http.StatusBadRequest, "InvalidParameterCombination", err.Error())
	case errors.Is(err, ErrClusterModeRequired):
		return xmlError(c, http.StatusBadRequest, "InvalidParameterCombination", err.Error())
	case errors.Is(err, ErrApplyImmediatelyRequired):
		return xmlError(c, http.StatusBadRequest, "InvalidParameterValue", err.Error())
	case errors.Is(err, ErrReplicationGroupNotAvailable):
		return xmlError(c, http.StatusBadRequest, "InvalidReplicationGroupState", err.Error())
	default:
		return xmlError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}
}

func (h *Handler) testFailoverReplicationGroup(ctx context.Context, c *echo.Context, form url.Values) error {
	id := form.Get("ReplicationGroupId")
	nodeGroupID := form.Get("NodeGroupId")

	rg, err := h.Backend.FailoverReplicationGroup(ctx, id, nodeGroupID)
	if err != nil {
		if errors.Is(err, ErrReplicationGroupNotFound) {
			return xmlError(c, http.StatusNotFound, "ReplicationGroupNotFoundFault", "Replication group not found")
		}
		if errors.Is(err, ErrReplicationGroupNotAvailable) {
			return xmlError(c, http.StatusBadRequest, "InvalidReplicationGroupState", err.Error())
		}

		return xmlError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	type result struct {
		XMLName          xml.Name            `xml:"TestFailoverResponse"`
		Xmlns            string              `xml:"xmlns,attr"`
		ReplicationGroup replicationGroupXML `xml:"TestFailoverResult>ReplicationGroup"`
	}

	return xmlResp(c, http.StatusOK, result{
		Xmlns:            elasticacheNS,
		ReplicationGroup: rgToXML(*rg),
	})
}

func (h *Handler) completeMigration(ctx context.Context, c *echo.Context, form url.Values) error {
	replicationGroupID := form.Get("ReplicationGroupId")
	force := strings.EqualFold(form.Get("Force"), "true")

	rg, err := h.Backend.CompleteMigration(ctx, replicationGroupID, force)
	if err != nil {
		if errors.Is(err, ErrReplicationGroupNotFound) {
			return xmlError(c, http.StatusNotFound, "ReplicationGroupNotFoundFault", "Replication group not found")
		}

		return xmlError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	type result struct {
		XMLName          xml.Name            `xml:"CompleteMigrationResponse"`
		Xmlns            string              `xml:"xmlns,attr"`
		ReplicationGroup replicationGroupXML `xml:"CompleteMigrationResult>ReplicationGroup"`
	}

	return xmlResp(c, http.StatusOK, result{
		Xmlns:            elasticacheNS,
		ReplicationGroup: rgToXML(*rg),
	})
}

// ----------------------------------------
// Helpers
// ----------------------------------------

func (h *Handler) startMigration(ctx context.Context, c *echo.Context, form url.Values) error {
	replicationGroupID := form.Get("ReplicationGroupId")
	endpoints := parseCustomerNodeEndpoints(form, "CustomerNodeEndpointList")

	rg, err := h.Backend.StartMigration(ctx, replicationGroupID, endpoints)
	if err != nil {
		switch {
		case errors.Is(err, ErrReplicationGroupNotFound):
			return xmlError(c, http.StatusNotFound, "ReplicationGroupNotFoundFault", "Replication group not found")
		case errors.Is(err, ErrCustomerNodeEndpointsRequired):
			return xmlError(c, http.StatusBadRequest, "InvalidParameterValue", err.Error())
		default:
			return xmlError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
		}
	}

	type result struct {
		XMLName          xml.Name            `xml:"StartMigrationResponse"`
		Xmlns            string              `xml:"xmlns,attr"`
		ReplicationGroup replicationGroupXML `xml:"StartMigrationResult>ReplicationGroup"`
	}

	return xmlResp(c, http.StatusOK, result{
		Xmlns:            elasticacheNS,
		ReplicationGroup: rgToXML(*rg),
	})
}

func (h *Handler) testMigration(ctx context.Context, c *echo.Context, form url.Values) error {
	replicationGroupID := form.Get("ReplicationGroupId")
	endpoints := parseCustomerNodeEndpoints(form, "CustomerNodeEndpointList")

	rg, err := h.Backend.TestMigration(ctx, replicationGroupID, endpoints)
	if err != nil {
		switch {
		case errors.Is(err, ErrReplicationGroupNotFound):
			return xmlError(c, http.StatusNotFound, "ReplicationGroupNotFoundFault", "Replication group not found")
		case errors.Is(err, ErrCustomerNodeEndpointsRequired):
			return xmlError(c, http.StatusBadRequest, "InvalidParameterValue", err.Error())
		default:
			return xmlError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
		}
	}

	type result struct {
		XMLName          xml.Name            `xml:"TestMigrationResponse"`
		Xmlns            string              `xml:"xmlns,attr"`
		ReplicationGroup replicationGroupXML `xml:"TestMigrationResult>ReplicationGroup"`
	}

	return xmlResp(c, http.StatusOK, result{
		Xmlns:            elasticacheNS,
		ReplicationGroup: rgToXML(*rg),
	})
}

// parseCustomerNodeEndpoints parses "<prefix>.member.N.{Address,Port}" form
// values into a []CustomerNodeEndpoint (StartMigration/TestMigration).
func parseCustomerNodeEndpoints(form url.Values, prefix string) []CustomerNodeEndpoint {
	var endpoints []CustomerNodeEndpoint

	for i := 1; ; i++ {
		base := fmt.Sprintf("%s.member.%d.", prefix, i)
		address := form.Get(base + "Address")
		portStr := form.Get(base + "Port")

		if address == "" && portStr == "" {
			break
		}

		port, _ := strconv.ParseInt(portStr, 10, 32)
		endpoints = append(endpoints, CustomerNodeEndpoint{Address: address, Port: int32(port)})
	}

	return endpoints
}

func (h *Handler) increaseReplicaCount(ctx context.Context, c *echo.Context, form url.Values) error {
	replicationGroupID := form.Get("ReplicationGroupId")
	newReplicaCount, _ := strconv.ParseInt(form.Get("NewReplicaCount"), 10, 32)
	applyImmediately := strings.EqualFold(form.Get("ApplyImmediately"), "true")

	rg, err := h.Backend.IncreaseReplicaCount(ctx, replicationGroupID, int32(newReplicaCount), applyImmediately)
	if err != nil {
		return mapReplicationGroupModifyErr(c, err)
	}

	type result struct {
		XMLName          xml.Name            `xml:"IncreaseReplicaCountResponse"`
		Xmlns            string              `xml:"xmlns,attr"`
		ReplicationGroup replicationGroupXML `xml:"IncreaseReplicaCountResult>ReplicationGroup"`
	}

	return xmlResp(c, http.StatusOK, result{
		Xmlns:            elasticacheNS,
		ReplicationGroup: rgToXML(*rg),
	})
}

func (h *Handler) decreaseReplicaCount(ctx context.Context, c *echo.Context, form url.Values) error {
	replicationGroupID := form.Get("ReplicationGroupId")
	newReplicaCount, _ := strconv.ParseInt(form.Get("NewReplicaCount"), 10, 32)
	applyImmediately := strings.EqualFold(form.Get("ApplyImmediately"), "true")

	rg, err := h.Backend.DecreaseReplicaCount(ctx, replicationGroupID, int32(newReplicaCount), applyImmediately)
	if err != nil {
		return mapReplicationGroupModifyErr(c, err)
	}

	type result struct {
		XMLName          xml.Name            `xml:"DecreaseReplicaCountResponse"`
		Xmlns            string              `xml:"xmlns,attr"`
		ReplicationGroup replicationGroupXML `xml:"DecreaseReplicaCountResult>ReplicationGroup"`
	}

	return xmlResp(c, http.StatusOK, result{
		Xmlns:            elasticacheNS,
		ReplicationGroup: rgToXML(*rg),
	})
}

func (h *Handler) modifyReplicationGroupShardConfiguration(
	ctx context.Context,
	c *echo.Context,
	form url.Values,
) error {
	replicationGroupID := form.Get("ReplicationGroupId")
	nodeGroupCount, _ := strconv.ParseInt(form.Get("NodeGroupCount"), 10, 32)
	applyImmediately := strings.EqualFold(form.Get("ApplyImmediately"), "true")

	rg, err := h.Backend.ModifyReplicationGroupShardConfiguration(
		ctx, replicationGroupID, int32(nodeGroupCount), applyImmediately,
	)
	if err != nil {
		return mapReplicationGroupModifyErr(c, err)
	}

	type result struct {
		XMLName          xml.Name            `xml:"ModifyReplicationGroupShardConfigurationResponse"`
		Xmlns            string              `xml:"xmlns,attr"`
		ReplicationGroup replicationGroupXML `xml:"ModifyReplicationGroupShardConfigurationResult>ReplicationGroup"`
	}

	return xmlResp(c, http.StatusOK, result{
		Xmlns:            elasticacheNS,
		ReplicationGroup: rgToXML(*rg),
	})
}

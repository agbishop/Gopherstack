package rds

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

func (h *Handler) handleCreateDBCluster(vals url.Values) (any, error) {
	id := vals.Get("DBClusterIdentifier")
	engine := vals.Get("Engine")
	masterUser := vals.Get("MasterUsername")
	dbName := vals.Get("DatabaseName")
	paramGroupName := vals.Get("DBClusterParameterGroupName")
	rawPort := vals.Get("Port")
	port := 0
	if rawPort != "" {
		var err error
		port, err = strconv.Atoi(rawPort)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid Port %q", ErrInvalidParameter, rawPort)
		}
	}

	serverlessV2Cfg, parseErr := parseServerlessV2ScalingConfig(vals)
	if parseErr != nil && !errors.Is(parseErr, ErrNoServerlessV2Config) {
		return nil, parseErr
	}

	backtrackWindow := int64(0)
	if rawBW := vals.Get("BacktrackWindow"); rawBW != "" {
		if v, err := strconv.ParseInt(rawBW, 10, 64); err == nil {
			backtrackWindow = v
		}
	}

	monitoringInterval := 0
	if rawMI := vals.Get("MonitoringInterval"); rawMI != "" {
		if v, err := strconv.Atoi(rawMI); err == nil {
			monitoringInterval = v
		}
	}

	backupRetention := minClusterBackupRetention
	if rawBR := vals.Get("BackupRetentionPeriod"); rawBR != "" {
		v, err := strconv.Atoi(rawBR)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid BackupRetentionPeriod %q", ErrInvalidParameter, rawBR)
		}

		if v < minClusterBackupRetention || v > maxClusterBackupRetention {
			return nil, fmt.Errorf(
				"%w: BackupRetentionPeriod must be between %d and %d; got %d",
				ErrInvalidParameter, minClusterBackupRetention, maxClusterBackupRetention, v,
			)
		}

		backupRetention = v
	}

	clusterOpts := DBClusterOptions{
		EngineVersion:                vals.Get("EngineVersion"),
		KmsKeyID:                     vals.Get("KmsKeyId"),
		PreferredBackupWindow:        vals.Get("PreferredBackupWindow"),
		PreferredMaintenanceWindow:   vals.Get("PreferredMaintenanceWindow"),
		MonitoringRoleArn:            vals.Get("MonitoringRoleArn"),
		StorageType:                  vals.Get("StorageType"),
		NetworkType:                  vals.Get("NetworkType"),
		EngineLifecycleSupport:       vals.Get("EngineLifecycleSupport"),
		ReplicationSourceIdentifier:  vals.Get("ReplicationSourceIdentifier"),
		EnabledCloudwatchLogsExports: parseMultiValueParam(vals, "EnableCloudwatchLogsExports.member"),
		AvailabilityZones:            parseMultiValueParam(vals, "AvailabilityZones.AvailabilityZone"),
		BacktrackWindow:              backtrackWindow,
		BackupRetentionPeriod:        backupRetention,
		MonitoringInterval:           monitoringInterval,
		MultiAZ:                      vals.Get("MultiAZ") == formTrue,
		StorageEncrypted:             vals.Get("StorageEncrypted") == formTrue,
		CopyTagsToSnapshot:           vals.Get("CopyTagsToSnapshot") == formTrue,
		DeletionProtection:           vals.Get("DeletionProtection") == formTrue,
		OptimizedWrites:              vals.Get("EnableOptimizedWrites") == formTrue,
	}

	cluster, err := h.Backend.CreateDBCluster(
		id,
		engine,
		masterUser,
		dbName,
		paramGroupName,
		port,
		serverlessV2Cfg,
		clusterOpts,
	)
	if err != nil {
		return nil, err
	}

	h.applyCreateTags(vals, cluster.DBClusterArn)

	return &createDBClusterResponse{
		Xmlns:     rdsXMLNS,
		DBCluster: toXMLCluster(cluster, h.Backend.ClusterAssociatedRoles(cluster.DBClusterIdentifier)),
	}, nil
}

func (h *Handler) handleDescribeDBClusters(vals url.Values) (any, error) {
	id := vals.Get("DBClusterIdentifier")
	clusters, err := h.Backend.DescribeDBClusters(id)
	if err != nil {
		return nil, err
	}
	clusters, err = applyDBClusterFilters(vals, clusters)
	if err != nil {
		return nil, err
	}
	members, marker, err := paginateDescribe(vals, clusters, func(a, b DBCluster) bool {
		return a.DBClusterIdentifier < b.DBClusterIdentifier
	}, func(item DBCluster) xmlDBCluster {
		cp := item

		return toXMLCluster(&cp, h.Backend.ClusterAssociatedRoles(cp.DBClusterIdentifier))
	})
	if err != nil {
		return nil, err
	}

	return &describeDBClustersResponse{
		Xmlns:      rdsXMLNS,
		DBClusters: xmlDBClusterList{Members: members},
		Marker:     marker,
	}, nil
}

func (h *Handler) handleDeleteDBCluster(vals url.Values) (any, error) {
	id := vals.Get("DBClusterIdentifier")
	skipFinalSnapshot := vals.Get("SkipFinalSnapshot") == formTrue
	finalSnapshotID := vals.Get("FinalDBSnapshotIdentifier")

	cluster, err := h.Backend.DeleteDBClusterWithOptions(id, skipFinalSnapshot, finalSnapshotID)
	if err != nil {
		return nil, err
	}

	return &deleteDBClusterResponse{
		Xmlns:     rdsXMLNS,
		DBCluster: toXMLCluster(cluster, h.Backend.ClusterAssociatedRoles(cluster.DBClusterIdentifier)),
	}, nil
}

func (h *Handler) handleModifyDBCluster(vals url.Values) (any, error) {
	id := vals.Get("DBClusterIdentifier")
	paramGroupName := vals.Get("DBClusterParameterGroupName")

	backtrackWindow := int64(0)
	if rawBW := vals.Get("BacktrackWindow"); rawBW != "" {
		if v, err := strconv.ParseInt(rawBW, 10, 64); err == nil {
			backtrackWindow = v
		}
	}

	monitoringInterval := -1
	if rawMI := vals.Get("MonitoringInterval"); rawMI != "" {
		if v, err := strconv.Atoi(rawMI); err == nil {
			monitoringInterval = v
		}
	}

	storageEncryptedRaw := vals.Get("StorageEncrypted")
	opts := DBClusterOptions{
		EngineVersion:              vals.Get("EngineVersion"),
		KmsKeyID:                   vals.Get("KmsKeyId"),
		PreferredBackupWindow:      vals.Get("PreferredBackupWindow"),
		PreferredMaintenanceWindow: vals.Get("PreferredMaintenanceWindow"),
		MonitoringRoleArn:          vals.Get("MonitoringRoleArn"),
		StorageType:                vals.Get("StorageType"),
		NetworkType:                vals.Get("NetworkType"),
		EngineLifecycleSupport:     vals.Get("EngineLifecycleSupport"),
		EnabledCloudwatchLogsExports: parseMultiValueParam(
			vals,
			"CloudwatchLogsExportConfiguration.EnableLogTypes.member",
		),
		BacktrackWindow:         backtrackWindow,
		MonitoringInterval:      monitoringInterval,
		MultiAZ:                 vals.Get("MultiAZ") == formTrue,
		CopyTagsToSnapshot:      vals.Get("CopyTagsToSnapshot") == formTrue,
		DeletionProtection:      vals.Get("DeletionProtection") == formTrue,
		DeletionProtectionSet:   vals.Get("DeletionProtection") != "",
		StorageEncrypted:        storageEncryptedRaw == formTrue,
		StorageEncryptedChanged: storageEncryptedRaw != "",
		OptimizedWrites:         vals.Get("EnableOptimizedWrites") == formTrue,
	}

	cluster, err := h.Backend.ModifyDBCluster(id, paramGroupName, opts)
	if err != nil {
		return nil, err
	}

	return &modifyDBClusterResponse{
		Xmlns:     rdsXMLNS,
		DBCluster: toXMLCluster(cluster, h.Backend.ClusterAssociatedRoles(cluster.DBClusterIdentifier)),
	}, nil
}

func (h *Handler) handleStartDBCluster(vals url.Values) (any, error) {
	id := vals.Get("DBClusterIdentifier")
	cluster, err := h.Backend.StartDBCluster(id)
	if err != nil {
		return nil, err
	}

	return &startDBClusterResponse{
		Xmlns:     rdsXMLNS,
		DBCluster: toXMLCluster(cluster, h.Backend.ClusterAssociatedRoles(cluster.DBClusterIdentifier)),
	}, nil
}

func (h *Handler) handleStopDBCluster(vals url.Values) (any, error) {
	id := vals.Get("DBClusterIdentifier")
	cluster, err := h.Backend.StopDBCluster(id)
	if err != nil {
		return nil, err
	}

	return &stopDBClusterResponse{
		Xmlns:     rdsXMLNS,
		DBCluster: toXMLCluster(cluster, h.Backend.ClusterAssociatedRoles(cluster.DBClusterIdentifier)),
	}, nil
}

func (h *Handler) handleRestoreDBClusterFromSnapshot(vals url.Values) (any, error) {
	clusterID := vals.Get("DBClusterIdentifier")
	snapshotID := vals.Get("SnapshotIdentifier")
	engine := vals.Get("Engine")
	cluster, err := h.Backend.RestoreDBClusterFromSnapshot(clusterID, snapshotID, engine)
	if err != nil {
		return nil, err
	}

	return &restoreDBClusterFromSnapshotResponse{
		Xmlns:     rdsXMLNS,
		DBCluster: toXMLCluster(cluster, h.Backend.ClusterAssociatedRoles(cluster.DBClusterIdentifier)),
	}, nil
}

func (h *Handler) handleRestoreDBClusterToPointInTime(vals url.Values) (any, error) {
	clusterID := vals.Get("DBClusterIdentifier")
	sourceClusterID := vals.Get("SourceDBClusterIdentifier")
	cluster, err := h.Backend.RestoreDBClusterToPointInTime(clusterID, sourceClusterID)
	if err != nil {
		return nil, err
	}

	return &restoreDBClusterToPointInTimeResponse{
		Xmlns:     rdsXMLNS,
		DBCluster: toXMLCluster(cluster, h.Backend.ClusterAssociatedRoles(cluster.DBClusterIdentifier)),
	}, nil
}

func toXMLCluster(c *DBCluster, roles []DBClusterRole) xmlDBCluster {
	var clusterCreateTime string
	if !c.ClusterCreateTime.IsZero() {
		clusterCreateTime = c.ClusterCreateTime.UTC().Format(time.RFC3339)
	}
	x := xmlDBCluster{
		DBClusterIdentifier:             c.DBClusterIdentifier,
		DBClusterArn:                    c.DBClusterArn,
		DBClusterResourceID:             c.DBClusterResourceID,
		Engine:                          c.Engine,
		EngineVersion:                   c.EngineVersion,
		Status:                          c.Status,
		MasterUsername:                  c.MasterUsername,
		DatabaseName:                    c.DatabaseName,
		DBClusterParameterGroupName:     c.DBClusterParameterGroupName,
		Endpoint:                        c.Endpoint,
		ReaderEndpoint:                  c.ReaderEndpoint,
		NetworkType:                     c.NetworkType,
		StorageType:                     c.StorageType,
		EngineLifecycleSupport:          c.EngineLifecycleSupport,
		Port:                            c.Port,
		Capacity:                        c.ServerlessCapacity,
		ActivityStreamStatus:            c.ActivityStreamStatus,
		ActivityStreamMode:              c.ActivityStreamMode,
		ActivityStreamKMSKeyID:          c.ActivityStreamKMSKeyID,
		ActivityStreamKinesisStreamName: c.ActivityStreamKinesisStreamName,
		PreferredBackupWindow:           c.PreferredBackupWindow,
		PreferredMaintenanceWindow:      c.PreferredMaintenanceWindow,
		KmsKeyID:                        c.KmsKeyID,
		MonitoringRoleArn:               c.MonitoringRoleArn,
		ClusterCreateTime:               clusterCreateTime,
		BacktrackWindow:                 c.BacktrackWindow,
		BackupRetentionPeriod:           c.BackupRetentionPeriod,
		MonitoringInterval:              c.MonitoringInterval,
		MultiAZ:                         c.MultiAZ,
		StorageEncrypted:                c.StorageEncrypted,
		CopyTagsToSnapshot:              c.CopyTagsToSnapshot,
		DeletionProtection:              c.DeletionProtection,
		OptimizedWrites:                 c.OptimizedWrites,
		HTTPEndpointEnabled:             c.HTTPEndpointEnabled,
		ReplicationSourceIdentifier:     c.ReplicationSourceIdentifier,
	}

	if c.ServerlessV2ScalingConfig != nil {
		x.ServerlessV2ScalingConfiguration = &xmlServerlessV2ScalingConfiguration{
			MinCapacity: c.ServerlessV2ScalingConfig.MinCapacity,
			MaxCapacity: c.ServerlessV2ScalingConfig.MaxCapacity,
		}
	}

	if len(c.DBClusterMembers) > 0 {
		members := make([]xmlDBClusterMember, 0, len(c.DBClusterMembers))
		for _, m := range c.DBClusterMembers {
			members = append(members, xmlDBClusterMember{
				DBInstanceIdentifier:          m.DBInstanceIdentifier,
				DBClusterParameterGroupStatus: dbClusterMemberParamGroupStatusInSync,
				PromotionTier:                 m.PromotionTier,
				IsClusterWriter:               m.IsClusterWriter,
			})
		}

		x.DBClusterMembers = &xmlDBClusterMemberList{Members: members}
	}

	if len(c.EnabledCloudwatchLogsExports) > 0 {
		members := make([]xmlLogTypeMember, 0, len(c.EnabledCloudwatchLogsExports))
		for _, lt := range c.EnabledCloudwatchLogsExports {
			members = append(members, xmlLogTypeMember{Value: lt})
		}

		x.EnabledCloudwatchLogsExports = &xmlLogTypeList{Members: members}
	}

	if len(c.AvailabilityZones) > 0 {
		x.AvailabilityZones = &xmlAvailabilityZoneList{Members: c.AvailabilityZones}
	}

	if len(roles) > 0 {
		members := make([]xmlDBClusterRole, 0, len(roles))
		for _, r := range roles {
			members = append(members, xmlDBClusterRole{
				FeatureName: r.FeatureName,
				RoleArn:     r.RoleArn,
				Status:      r.Status,
			})
		}

		x.AssociatedRoles = &xmlDBClusterRoleList{Members: members}
	}

	if len(c.ReadReplicaIdentifiers) > 0 {
		members := make([]xmlClusterReplicaIdentifier, 0, len(c.ReadReplicaIdentifiers))
		for _, rid := range c.ReadReplicaIdentifiers {
			members = append(members, xmlClusterReplicaIdentifier{Value: rid})
		}

		x.ReadReplicaIdentifiers = &xmlClusterReplicaIdentifierList{Members: members}
	}

	return x
}

// parseServerlessV2ScalingConfig parses ServerlessV2ScalingConfiguration from request form values.
// Returns nil when neither field is present in the request.
func parseServerlessV2ScalingConfig(vals url.Values) (*ServerlessV2ScalingConfiguration, error) {
	rawMin := vals.Get("ServerlessV2ScalingConfiguration.MinCapacity")
	rawMax := vals.Get("ServerlessV2ScalingConfiguration.MaxCapacity")
	if rawMin == "" && rawMax == "" {
		return nil, ErrNoServerlessV2Config
	}
	cfg := &ServerlessV2ScalingConfiguration{}
	if rawMin != "" {
		v, err := strconv.ParseFloat(rawMin, 64)
		if err != nil {
			return nil, fmt.Errorf(
				"%w: invalid ServerlessV2ScalingConfiguration.MinCapacity %q",
				ErrInvalidParameter, rawMin,
			)
		}
		cfg.MinCapacity = v
	}
	if rawMax != "" {
		v, err := strconv.ParseFloat(rawMax, 64)
		if err != nil {
			return nil, fmt.Errorf(
				"%w: invalid ServerlessV2ScalingConfiguration.MaxCapacity %q",
				ErrInvalidParameter, rawMax,
			)
		}
		cfg.MaxCapacity = v
	}

	return cfg, nil
}

type xmlServerlessV2ScalingConfiguration struct {
	MinCapacity float64 `xml:"MinCapacity"`
	MaxCapacity float64 `xml:"MaxCapacity"`
}

// xmlServerlessV2Ref is a type alias to keep struct field definitions within line-length limits.
type xmlServerlessV2Ref = xmlServerlessV2ScalingConfiguration

type xmlDBClusterMember struct {
	DBInstanceIdentifier          string `xml:"DBInstanceIdentifier"`
	DBClusterParameterGroupStatus string `xml:"DBClusterParameterGroupStatus,omitempty"`
	PromotionTier                 int    `xml:"PromotionTier,omitempty"`
	IsClusterWriter               bool   `xml:"IsClusterWriter"`
}

// dbClusterMemberParamGroupStatusInSync is the status AWS reports for a
// cluster member's parameter group once applied; this emulator applies
// parameter groups synchronously, so members are always in-sync.
const dbClusterMemberParamGroupStatusInSync = "in-sync"

// xmlDBClusterRole is the wire shape of types.DBClusterRole (rds@v1.124.1
// types.go:1511); FeatureName/RoleArn/Status element names and the wrapping
// AssociatedRoles>DBClusterRole nesting are confirmed against
// deserializers.go's awsAwsquery_deserializeDocumentDBClusterRole(s).
type xmlDBClusterRole struct {
	FeatureName string `xml:"FeatureName,omitempty"`
	RoleArn     string `xml:"RoleArn"`
	Status      string `xml:"Status"`
}

type xmlDBClusterRoleList struct {
	Members []xmlDBClusterRole `xml:"DBClusterRole"`
}

type xmlDBClusterMemberList struct {
	Members []xmlDBClusterMember `xml:"DBClusterMember"`
}

type xmlAvailabilityZoneList struct {
	Members []string `xml:"AvailabilityZone"`
}

// xmlClusterReplicaIdentifierList wraps ReadReplicaIdentifiers>ReadReplicaIdentifier,
// confirmed against deserializers.go's awsAwsquery_deserializeDocumentReadReplicaIdentifierList
// (rds@v1.124.1) -- distinct from the instance-level
// ReadReplicaDBInstanceIdentifiers>ReadReplicaDBInstanceIdentifier wrapping.
type xmlClusterReplicaIdentifier struct {
	Value string `xml:",chardata"`
}

type xmlClusterReplicaIdentifierList struct {
	Members []xmlClusterReplicaIdentifier `xml:"ReadReplicaIdentifier"`
}

type xmlDBCluster struct {
	ServerlessV2ScalingConfiguration *xmlServerlessV2Ref              `xml:"ServerlessV2ScalingConfiguration,omitempty"`
	DBClusterMembers                 *xmlDBClusterMemberList          `xml:"DBClusterMembers,omitempty"`
	EnabledCloudwatchLogsExports     *xmlLogTypeList                  `xml:"EnabledCloudwatchLogsExports,omitempty"`
	AvailabilityZones                *xmlAvailabilityZoneList         `xml:"AvailabilityZones,omitempty"`
	AssociatedRoles                  *xmlDBClusterRoleList            `xml:"AssociatedRoles,omitempty"`
	ReadReplicaIdentifiers           *xmlClusterReplicaIdentifierList `xml:"ReadReplicaIdentifiers,omitempty"`
	DBClusterIdentifier              string                           `xml:"DBClusterIdentifier"`
	DBClusterArn                     string                           `xml:"DBClusterArn,omitempty"`
	DBClusterResourceID              string                           `xml:"DbClusterResourceId,omitempty"`
	Engine                           string                           `xml:"Engine"`
	EngineVersion                    string                           `xml:"EngineVersion,omitempty"`
	Status                           string                           `xml:"Status"`
	MasterUsername                   string                           `xml:"MasterUsername"`
	DatabaseName                     string                           `xml:"DatabaseName,omitempty"`
	DBClusterParameterGroupName      string                           `xml:"DBClusterParameterGroup"`
	Endpoint                         string                           `xml:"Endpoint,omitempty"`
	ReaderEndpoint                   string                           `xml:"ReaderEndpoint,omitempty"`
	ReplicationSourceIdentifier      string                           `xml:"ReplicationSourceIdentifier,omitempty"`
	NetworkType                      string                           `xml:"NetworkType,omitempty"`
	StorageType                      string                           `xml:"StorageType,omitempty"`
	EngineLifecycleSupport           string                           `xml:"EngineLifecycleSupport,omitempty"`
	ActivityStreamStatus             string                           `xml:"ActivityStreamStatus,omitempty"`
	ActivityStreamMode               string                           `xml:"ActivityStreamMode,omitempty"`
	ActivityStreamKMSKeyID           string                           `xml:"ActivityStreamKmsKeyId,omitempty"`
	ActivityStreamKinesisStreamName  string                           `xml:"ActivityStreamKinesisStreamName,omitempty"`
	PreferredBackupWindow            string                           `xml:"PreferredBackupWindow,omitempty"`
	PreferredMaintenanceWindow       string                           `xml:"PreferredMaintenanceWindow,omitempty"`
	KmsKeyID                         string                           `xml:"KmsKeyId,omitempty"`
	MonitoringRoleArn                string                           `xml:"MonitoringRoleArn,omitempty"`
	ClusterCreateTime                string                           `xml:"ClusterCreateTime,omitempty"`
	Port                             int                              `xml:"Port"`
	Capacity                         int                              `xml:"Capacity,omitempty"`
	BackupRetentionPeriod            int                              `xml:"BackupRetentionPeriod"`
	BacktrackWindow                  int64                            `xml:"BacktrackWindow,omitempty"`
	MonitoringInterval               int                              `xml:"MonitoringInterval,omitempty"`
	MultiAZ                          bool                             `xml:"MultiAZ,omitempty"`
	StorageEncrypted                 bool                             `xml:"StorageEncrypted,omitempty"`
	CopyTagsToSnapshot               bool                             `xml:"CopyTagsToSnapshot,omitempty"`
	DeletionProtection               bool                             `xml:"DeletionProtection,omitempty"`
	OptimizedWrites                  bool                             `xml:"OptimizedWritesEnabled,omitempty"`
	HTTPEndpointEnabled              bool                             `xml:"HttpEndpointEnabled,omitempty"`
}

type xmlDBClusterList struct {
	Members []xmlDBCluster `xml:"DBCluster"`
}

type createDBClusterResponse struct {
	XMLName   xml.Name     `xml:"CreateDBClusterResponse"`
	Xmlns     string       `xml:"xmlns,attr"`
	DBCluster xmlDBCluster `xml:"CreateDBClusterResult>DBCluster"`
}

type describeDBClustersResponse struct {
	XMLName    xml.Name         `xml:"DescribeDBClustersResponse"`
	Xmlns      string           `xml:"xmlns,attr"`
	Marker     string           `xml:"DescribeDBClustersResult>Marker,omitempty"`
	DBClusters xmlDBClusterList `xml:"DescribeDBClustersResult>DBClusters"`
}

type deleteDBClusterResponse struct {
	XMLName   xml.Name     `xml:"DeleteDBClusterResponse"`
	Xmlns     string       `xml:"xmlns,attr"`
	DBCluster xmlDBCluster `xml:"DeleteDBClusterResult>DBCluster"`
}

type modifyDBClusterResponse struct {
	XMLName   xml.Name     `xml:"ModifyDBClusterResponse"`
	Xmlns     string       `xml:"xmlns,attr"`
	DBCluster xmlDBCluster `xml:"ModifyDBClusterResult>DBCluster"`
}

type startDBClusterResponse struct {
	XMLName   xml.Name     `xml:"StartDBClusterResponse"`
	Xmlns     string       `xml:"xmlns,attr"`
	DBCluster xmlDBCluster `xml:"StartDBClusterResult>DBCluster"`
}

type stopDBClusterResponse struct {
	XMLName   xml.Name     `xml:"StopDBClusterResponse"`
	Xmlns     string       `xml:"xmlns,attr"`
	DBCluster xmlDBCluster `xml:"StopDBClusterResult>DBCluster"`
}

type restoreDBClusterFromSnapshotResponse struct {
	XMLName   xml.Name     `xml:"RestoreDBClusterFromSnapshotResponse"`
	Xmlns     string       `xml:"xmlns,attr"`
	DBCluster xmlDBCluster `xml:"RestoreDBClusterFromSnapshotResult>DBCluster"`
}

type restoreDBClusterToPointInTimeResponse struct {
	XMLName   xml.Name     `xml:"RestoreDBClusterToPointInTimeResponse"`
	Xmlns     string       `xml:"xmlns,attr"`
	DBCluster xmlDBCluster `xml:"RestoreDBClusterToPointInTimeResult>DBCluster"`
}

func (h *Handler) handleAddRoleToDBCluster(vals url.Values) (any, error) {
	clusterID := vals.Get("DBClusterIdentifier")
	roleARN := vals.Get("RoleArn")
	featureName := vals.Get("FeatureName")

	if err := h.Backend.AddRoleToDBCluster(clusterID, roleARN, featureName); err != nil {
		return nil, err
	}

	return &addRoleToDBClusterResponse{Xmlns: rdsXMLNS}, nil
}

func (h *Handler) handleBacktrackDBCluster(vals url.Values) (any, error) {
	clusterID := vals.Get("DBClusterIdentifier")
	backtrackTo := vals.Get("BacktrackTo")

	bt, err := h.Backend.BacktrackDBCluster(clusterID, backtrackTo)
	if err != nil {
		return nil, err
	}

	return &backtrackDBClusterResponse{
		Xmlns:              rdsXMLNS,
		DBClusterBacktrack: toXMLDBClusterBacktrack(bt),
	}, nil
}

func toXMLDBClusterBacktrack(bt *DBClusterBacktrack) xmlDBClusterBacktrack {
	return xmlDBClusterBacktrack{
		DBClusterIdentifier: bt.DBClusterIdentifier,
		BacktrackIdentifier: bt.BacktrackIdentifier,
		BacktrackTo:         bt.BacktrackTo,
		Status:              bt.Status,
	}
}

type addRoleToDBClusterResponse struct {
	XMLName xml.Name `xml:"AddRoleToDBClusterResponse"`
	Xmlns   string   `xml:"xmlns,attr"`
}

type xmlDBClusterBacktrack struct {
	DBClusterIdentifier string `xml:"DBClusterIdentifier"`
	BacktrackIdentifier string `xml:"BacktrackIdentifier"`
	BacktrackTo         string `xml:"BacktrackTo,omitempty"`
	Status              string `xml:"Status"`
}

type backtrackDBClusterResponse struct {
	XMLName            xml.Name              `xml:"BacktrackDBClusterResponse"`
	Xmlns              string                `xml:"xmlns,attr"`
	DBClusterBacktrack xmlDBClusterBacktrack `xml:"BacktrackDBClusterResult>DBClusterBacktrack"`
}

func (h *Handler) handleRemoveRoleFromDBCluster(vals url.Values) (any, error) {
	clusterID := vals.Get("DBClusterIdentifier")
	roleARN := vals.Get("RoleArn")
	featureName := vals.Get("FeatureName")

	if err := h.Backend.RemoveRoleFromDBCluster(clusterID, roleARN, featureName); err != nil {
		return nil, err
	}

	return &removeRoleFromDBClusterResponse{Xmlns: rdsXMLNS}, nil
}

type removeRoleFromDBClusterResponse struct {
	XMLName xml.Name `xml:"RemoveRoleFromDBClusterResponse"`
	Xmlns   string   `xml:"xmlns,attr"`
}

func (h *Handler) handleFailoverDBCluster(vals url.Values) (any, error) {
	clusterID := vals.Get("DBClusterIdentifier")
	targetDBInstanceIdentifier := vals.Get("TargetDBInstanceIdentifier")
	cluster, err := h.Backend.FailoverDBCluster(clusterID, targetDBInstanceIdentifier)
	if err != nil {
		return nil, err
	}

	return &failoverDBClusterResponse{
		Xmlns:     rdsXMLNS,
		DBCluster: toXMLCluster(cluster, h.Backend.ClusterAssociatedRoles(cluster.DBClusterIdentifier)),
	}, nil
}

func (h *Handler) handleRebootDBCluster(vals url.Values) (any, error) {
	clusterID := vals.Get("DBClusterIdentifier")
	cluster, err := h.Backend.RebootDBCluster(clusterID)
	if err != nil {
		return nil, err
	}

	return &rebootDBClusterResponse{
		Xmlns:     rdsXMLNS,
		DBCluster: toXMLCluster(cluster, h.Backend.ClusterAssociatedRoles(cluster.DBClusterIdentifier)),
	}, nil
}

type failoverDBClusterResponse struct {
	XMLName   xml.Name     `xml:"FailoverDBClusterResponse"`
	Xmlns     string       `xml:"xmlns,attr"`
	DBCluster xmlDBCluster `xml:"FailoverDBClusterResult>DBCluster"`
}

type rebootDBClusterResponse struct {
	XMLName   xml.Name     `xml:"RebootDBClusterResponse"`
	Xmlns     string       `xml:"xmlns,attr"`
	DBCluster xmlDBCluster `xml:"RebootDBClusterResult>DBCluster"`
}

type promoteReadReplicaDBClusterResponse struct {
	XMLName   xml.Name     `xml:"PromoteReadReplicaDBClusterResponse"`
	Xmlns     string       `xml:"xmlns,attr"`
	DBCluster xmlDBCluster `xml:"PromoteReadReplicaDBClusterResult>DBCluster"`
}

type xmlDBClusterBacktrackList struct {
	Members []xmlDBClusterBacktrack `xml:"DBClusterBacktrack"`
}

type describeDBClusterBacktracksResponse struct {
	XMLName             xml.Name                  `xml:"DescribeDBClusterBacktracksResponse"`
	Xmlns               string                    `xml:"xmlns,attr"`
	DBClusterBacktracks xmlDBClusterBacktrackList `xml:"DescribeDBClusterBacktracksResult>DBClusterBacktracks"`
}

type modifyCurrentDBClusterCapacityResponse struct {
	XMLName              xml.Name `xml:"ModifyCurrentDBClusterCapacityResponse"`
	Xmlns                string   `xml:"xmlns,attr"`
	DBClusterIdentifier  string   `xml:"ModifyCurrentDBClusterCapacityResult>DBClusterIdentifier"`
	TimeoutAction        string   `xml:"ModifyCurrentDBClusterCapacityResult>TimeoutAction"`
	CurrentCapacity      int      `xml:"ModifyCurrentDBClusterCapacityResult>CurrentCapacity"`
	PendingCapacity      int      `xml:"ModifyCurrentDBClusterCapacityResult>PendingCapacity"`
	SecondsBeforeTimeout int      `xml:"ModifyCurrentDBClusterCapacityResult>SecondsBeforeTimeout"`
}

type restoreDBClusterFromS3Response struct {
	XMLName   xml.Name     `xml:"RestoreDBClusterFromS3Response"`
	Xmlns     string       `xml:"xmlns,attr"`
	DBCluster xmlDBCluster `xml:"RestoreDBClusterFromS3Result>DBCluster"`
}

func (h *Handler) handlePromoteReadReplicaDBCluster(vals url.Values) (any, error) {
	clusterID := vals.Get("DBClusterIdentifier")
	cluster, err := h.Backend.PromoteReadReplicaDBCluster(clusterID)
	if err != nil {
		return nil, err
	}

	return &promoteReadReplicaDBClusterResponse{
		Xmlns:     rdsXMLNS,
		DBCluster: toXMLCluster(cluster, h.Backend.ClusterAssociatedRoles(cluster.DBClusterIdentifier)),
	}, nil
}

func (h *Handler) handleDescribeDBClusterBacktracks(vals url.Values) (any, error) {
	clusterID := vals.Get("DBClusterIdentifier")
	backtracks, err := h.Backend.DescribeDBClusterBacktracks(clusterID)
	if err != nil {
		return nil, err
	}
	members := make([]xmlDBClusterBacktrack, 0, len(backtracks))
	for _, bt := range backtracks {
		members = append(members, xmlDBClusterBacktrack(bt))
	}

	return &describeDBClusterBacktracksResponse{
		Xmlns:               rdsXMLNS,
		DBClusterBacktracks: xmlDBClusterBacktrackList{Members: members},
	}, nil
}

func (h *Handler) handleModifyCurrentDBClusterCapacity(vals url.Values) (any, error) {
	clusterID := vals.Get("DBClusterIdentifier")
	capacityStr := vals.Get("Capacity")
	capacity, _ := strconv.Atoi(capacityStr)
	cluster, err := h.Backend.ModifyCurrentDBClusterCapacity(clusterID, capacity)
	if err != nil {
		return nil, err
	}

	secondsBeforeTimeout := 300
	if v := vals.Get("SecondsBeforeTimeout"); v != "" {
		if parsed, perr := strconv.Atoi(v); perr == nil {
			secondsBeforeTimeout = parsed
		}
	}

	timeoutAction := vals.Get("TimeoutAction")
	if timeoutAction == "" {
		timeoutAction = "ForceApplyCapacityChange"
	}

	return &modifyCurrentDBClusterCapacityResponse{
		Xmlns:                rdsXMLNS,
		DBClusterIdentifier:  cluster.DBClusterIdentifier,
		CurrentCapacity:      cluster.ServerlessCapacity,
		PendingCapacity:      cluster.ServerlessCapacity,
		SecondsBeforeTimeout: secondsBeforeTimeout,
		TimeoutAction:        timeoutAction,
	}, nil
}

func (h *Handler) handleRestoreDBClusterFromS3(vals url.Values) (any, error) {
	id := vals.Get("DBClusterIdentifier")
	engine := vals.Get("Engine")
	masterUsername := vals.Get("MasterUsername")
	s3Bucket := vals.Get("S3BucketName")
	s3IngestionRoleArn := vals.Get("S3IngestionRoleArn")
	sourceEngine := vals.Get("SourceEngine")
	sourceEngineVersion := vals.Get("SourceEngineVersion")
	cluster, err := h.Backend.RestoreDBClusterFromS3(
		id, engine, masterUsername, s3Bucket, s3IngestionRoleArn, sourceEngine, sourceEngineVersion,
	)
	if err != nil {
		return nil, err
	}

	return &restoreDBClusterFromS3Response{
		Xmlns:     rdsXMLNS,
		DBCluster: toXMLCluster(cluster, h.Backend.ClusterAssociatedRoles(cluster.DBClusterIdentifier)),
	}, nil
}

func toXMLClusterBackup(b *DBClusterAutomatedBackup) xmlDBClusterAutomatedBackup {
	return xmlDBClusterAutomatedBackup{
		DBClusterIdentifier:   b.DBClusterIdentifier,
		DBClusterResourceID:   b.DBClusterResourceID,
		Engine:                b.Engine,
		EngineVersion:         b.EngineVersion,
		Region:                b.Region,
		Status:                b.Status,
		BackupRetentionPeriod: b.BackupRetentionPeriod,
		StorageEncrypted:      b.StorageEncrypted,
	}
}

// fisFailoverDBClusters simulates a failover for the given DB clusters.
// In the in-memory backend there is no real replication, so this records a
// timed failover event on the backend for observability and automatically
// clears it after the given duration (if non-zero) or on ctx cancellation.
func (h *Handler) fisFailoverDBClusters(ctx context.Context, targets []string, dur time.Duration) error {
	var expiry time.Time
	if dur > 0 {
		expiry = time.Now().Add(dur)
	}

	ids := make([]string, 0, len(targets))

	func() {
		h.Backend.mu.Lock("FISFailoverDBClusters")
		defer h.Backend.mu.Unlock()

		for _, t := range targets {
			id := rdsIDFromARN(t)
			ids = append(ids, id)
			h.Backend.fisFailoverFaults[id] = expiry
		}
	}()

	if dur > 0 {
		// Time-limited: clear after duration or on cancellation.
		go h.Backend.scheduleFailoverFaultCleanup(ctx, ids, dur)
	} else {
		// Indefinite fault (dur==0): the goroutine blocks on ctx.Done().
		// It terminates when StopExperiment cancels the experiment context,
		// or when the server shuts down (root context is cancelled).
		// This is not a goroutine leak — the goroutine is intentionally
		// bound to the experiment lifetime via ctx.
		go func() {
			<-ctx.Done()

			h.Backend.mu.Lock("FISFailoverDBClusters-ctxcancel")
			defer h.Backend.mu.Unlock()

			for _, id := range ids {
				delete(h.Backend.fisFailoverFaults, id)
			}
		}()
	}

	return nil
}

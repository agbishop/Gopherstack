package rds

import (
	"encoding/xml"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

// parseDBInstanceIntParams parses numeric form parameters for CreateDBInstance/ModifyDBInstance.
func parseDBInstanceIntParams(
	vals url.Values,
) (int, int, int, int, int, error) {
	var allocatedStorage, iops, storageThroughput, monitoringInterval int
	backupRetention := 1

	if raw := vals.Get("AllocatedStorage"); raw != "" {
		v, e := strconv.Atoi(raw)
		if e != nil {
			return 0, 0, 0, 0, 0, fmt.Errorf("%w: invalid AllocatedStorage %q", ErrInvalidParameter, raw)
		}
		allocatedStorage = v
	}

	if raw := vals.Get("BackupRetentionPeriod"); raw != "" {
		v, e := strconv.Atoi(raw)
		if e != nil {
			return 0, 0, 0, 0, 0, fmt.Errorf("%w: invalid BackupRetentionPeriod %q", ErrInvalidParameter, raw)
		}

		backupRetention = v
	}

	if raw := vals.Get("Iops"); raw != "" {
		if v, e := strconv.Atoi(raw); e == nil {
			iops = v
		}
	}

	if raw := vals.Get("StorageThroughput"); raw != "" {
		if v, e := strconv.Atoi(raw); e == nil {
			storageThroughput = v
		}
	}

	if raw := vals.Get("MonitoringInterval"); raw != "" {
		v, e := strconv.Atoi(raw)
		if e != nil {
			return 0, 0, 0, 0, 0, fmt.Errorf("%w: invalid MonitoringInterval %q", ErrInvalidParameter, raw)
		}

		if !validMonitoringInterval(v) {
			return 0, 0, 0, 0, 0, fmt.Errorf(
				"%w: MonitoringInterval must be one of 0, 1, 5, 10, 15, 30, 60; got %d",
				ErrInvalidParameter, v,
			)
		}

		monitoringInterval = v
	}

	return allocatedStorage, backupRetention, iops, storageThroughput, monitoringInterval, nil
}

func (h *Handler) handleCreateDBInstance(vals url.Values) (any, error) {
	id := vals.Get("DBInstanceIdentifier")
	engine := vals.Get("Engine")
	instanceClass := vals.Get("DBInstanceClass")
	dbName := vals.Get("DBName")
	masterUser := vals.Get("MasterUsername")
	paramGroupName := vals.Get("DBParameterGroupName")

	allocatedStorage, backupRetention, iops, storageThroughput, monitoringInterval, err := parseDBInstanceIntParams(
		vals,
	)
	if err != nil {
		return nil, err
	}

	if backupRetention < 0 || backupRetention > 35 {
		return nil, fmt.Errorf(
			"%w: BackupRetentionPeriod must be between 0 and 35; got %d",
			ErrInvalidParameter, backupRetention,
		)
	}

	// AWS bounds AllocatedStorage to 20–65536 GiB for general-purpose engines.
	// A zero value means the field was omitted (the engine default applies).
	if allocatedStorage != 0 && (allocatedStorage < minAllocatedStorage || allocatedStorage > maxAllocatedStorage) {
		return nil, fmt.Errorf(
			"%w: AllocatedStorage must be between %d and %d; got %d",
			ErrInvalidParameter, minAllocatedStorage, maxAllocatedStorage, allocatedStorage,
		)
	}

	vpcSGIds := parseMultiValueParam(vals, "VpcSecurityGroupIds.VpcSecurityGroupID")
	dbSGNames := parseMultiValueParam(vals, "DBSecurityGroups.DBSecurityGroupName")
	logExports := parseMultiValueParam(vals, "EnableCloudwatchLogsExports.member")

	opts := DBInstanceOptions{
		EngineVersion:                    vals.Get("EngineVersion"),
		StorageType:                      vals.Get("StorageType"),
		AvailabilityZone:                 vals.Get("AvailabilityZone"),
		OptionGroupName:                  vals.Get("OptionGroupName"),
		LicenseModel:                     vals.Get("LicenseModel"),
		MonitoringRoleArn:                vals.Get("MonitoringRoleArn"),
		PreferredMaintenanceWindow:       vals.Get("PreferredMaintenanceWindow"),
		PreferredBackupWindow:            vals.Get("PreferredBackupWindow"),
		KmsKeyID:                         vals.Get("KmsKeyId"),
		DBClusterIdentifier:              vals.Get("DBClusterIdentifier"),
		DBSubnetGroupName:                vals.Get("DBSubnetGroupName"),
		BackupRetentionPeriod:            backupRetention,
		Iops:                             iops,
		StorageThroughput:                storageThroughput,
		MonitoringInterval:               monitoringInterval,
		MultiAZ:                          vals.Get("MultiAZ") == formTrue,
		StorageEncrypted:                 vals.Get("StorageEncrypted") == formTrue,
		IAMDatabaseAuthenticationEnabled: vals.Get("EnableIAMDatabaseAuthentication") == formTrue,
		DeletionProtection:               vals.Get("DeletionProtection") == formTrue,
		CopyTagsToSnapshot:               vals.Get("CopyTagsToSnapshot") == formTrue,
		PubliclyAccessible:               vals.Get("PubliclyAccessible") == formTrue,
		PerformanceInsightsEnabled:       vals.Get("EnablePerformanceInsights") == formTrue,
		StorageOptimized:                 vals.Get("StorageOptimized") == formTrue,
		OptimizedWrites:                  vals.Get("EnableOptimizedWrites") == formTrue,
		EngineLifecycleSupport:           vals.Get("EngineLifecycleSupport"),
		VpcSecurityGroupIDs:              vpcSGIds,
		DBSecurityGroupNames:             dbSGNames,
		EnabledCloudwatchLogsExports:     logExports,
	}

	inst, err := h.Backend.CreateDBInstance(
		id,
		engine,
		instanceClass,
		dbName,
		masterUser,
		paramGroupName,
		allocatedStorage,
		opts,
	)
	if err != nil {
		return nil, err
	}

	h.applyCreateTags(vals, inst.DBInstanceArn)

	return &createDBInstanceResponse{
		Xmlns:      rdsXMLNS,
		DBInstance: toXMLInstance(inst),
	}, nil
}

func (h *Handler) handleDeleteDBInstance(vals url.Values) (any, error) {
	id := vals.Get("DBInstanceIdentifier")
	skipFinalSnapshot := vals.Get("SkipFinalSnapshot") == formTrue
	finalSnapshotID := vals.Get("FinalDBSnapshotIdentifier")
	deleteAutomatedBackups := vals.Get("DeleteAutomatedBackups") != "false"

	inst, err := h.Backend.DeleteDBInstanceWithOptions(id, skipFinalSnapshot, finalSnapshotID, deleteAutomatedBackups)
	if err != nil {
		return nil, err
	}

	return &deleteDBInstanceResponse{
		Xmlns:      rdsXMLNS,
		DBInstance: toXMLInstance(inst),
	}, nil
}

// parseDescribeFilters parses the AWS query-protocol "Filters.Filter.N.Name" /
// "Filters.Filter.N.Values.Value.M" parameters into a filter-name -> values
// map. Confirmed against rds@v1.124.1 serializers.go:11730,
// awsAwsquery_serializeDocumentFilterValueList's array element name "Value",
// not the generic "member" -- a real client's Filters never appear on the
// wire as "Filters.Filter.N.Values.member.M".
func parseDescribeFilters(vals url.Values) map[string][]string {
	filters := make(map[string][]string)
	for i := 1; ; i++ {
		name := vals.Get(fmt.Sprintf("Filters.Filter.%d.Name", i))
		if name == "" {
			return filters
		}
		var values []string
		for j := 1; ; j++ {
			v := vals.Get(fmt.Sprintf("Filters.Filter.%d.Values.Value.%d", i, j))
			if v == "" {
				break
			}
			values = append(values, v)
		}
		filters[name] = values
	}
}

func (h *Handler) handleDescribeDBInstances(vals url.Values) (any, error) {
	id := vals.Get("DBInstanceIdentifier")
	instances, err := h.Backend.DescribeDBInstances(id)
	if err != nil {
		return nil, err
	}
	instances, err = applyDBInstanceFilters(vals, instances)
	if err != nil {
		return nil, err
	}
	members, marker, err := paginateDescribe(vals, instances, func(a, b DBInstance) bool {
		return a.DBInstanceIdentifier < b.DBInstanceIdentifier
	}, func(item DBInstance) xmlDBInstance {
		cp := item

		return toXMLInstance(&cp)
	})
	if err != nil {
		return nil, err
	}

	return &describeDBInstancesResponse{
		Xmlns:       rdsXMLNS,
		DBInstances: xmlDBInstanceList{Members: members},
		Marker:      marker,
	}, nil
}

func (h *Handler) handleModifyDBInstance(vals url.Values) (any, error) {
	id := vals.Get("DBInstanceIdentifier")
	instanceClass := vals.Get("DBInstanceClass")

	allocatedStorage, backupRetention, iops, storageThroughput, monitoringInterval, err := parseDBInstanceIntParams(
		vals,
	)
	if err != nil {
		return nil, err
	}
	// Modify uses -1 as sentinel for "not provided"
	if vals.Get("BackupRetentionPeriod") == "" {
		backupRetention = -1
	}
	if vals.Get("MonitoringInterval") == "" {
		monitoringInterval = -1
	}

	if backupRetention >= 0 && backupRetention > 35 {
		return nil, fmt.Errorf(
			"%w: BackupRetentionPeriod must be between 0 and 35; got %d",
			ErrInvalidParameter, backupRetention,
		)
	}

	vpcSGIds := parseMultiValueParam(vals, "VpcSecurityGroupIds.VpcSecurityGroupID")
	logExports := parseMultiValueParam(vals, "CloudwatchLogsExportConfiguration.EnableLogTypes.member")

	opts := DBInstanceOptions{
		EngineVersion:                    vals.Get("EngineVersion"),
		StorageType:                      vals.Get("StorageType"),
		OptionGroupName:                  vals.Get("OptionGroupName"),
		LicenseModel:                     vals.Get("LicenseModel"),
		MonitoringRoleArn:                vals.Get("MonitoringRoleArn"),
		PreferredMaintenanceWindow:       vals.Get("PreferredMaintenanceWindow"),
		PreferredBackupWindow:            vals.Get("PreferredBackupWindow"),
		DBParameterGroupName:             vals.Get("DBParameterGroupName"),
		BackupRetentionPeriod:            backupRetention,
		Iops:                             iops,
		StorageThroughput:                storageThroughput,
		MonitoringInterval:               monitoringInterval,
		MultiAZ:                          vals.Get("MultiAZ") == formTrue,
		MultiAZSet:                       vals.Get("MultiAZ") != "",
		IAMDatabaseAuthenticationEnabled: vals.Get("EnableIAMDatabaseAuthentication") == formTrue,
		IAMDatabaseAuthSet:               vals.Get("EnableIAMDatabaseAuthentication") != "",
		DeletionProtection:               vals.Get("DeletionProtection") == formTrue,
		DeletionProtectionSet:            vals.Get("DeletionProtection") != "",
		CopyTagsToSnapshot:               vals.Get("CopyTagsToSnapshot") == formTrue,
		AllowMajorVersionUpgrade:         vals.Get("AllowMajorVersionUpgrade") == formTrue,
		ApplyImmediately:                 vals.Get("ApplyImmediately") == formTrue,
		PubliclyAccessible:               vals.Get("PubliclyAccessible") == formTrue,
		PerformanceInsightsEnabled:       vals.Get("EnablePerformanceInsights") == formTrue,
		StorageOptimized:                 vals.Get("StorageOptimized") == formTrue,
		OptimizedWrites:                  vals.Get("EnableOptimizedWrites") == formTrue,
		EngineLifecycleSupport:           vals.Get("EngineLifecycleSupport"),
		VpcSecurityGroupIDs:              vpcSGIds,
		EnabledCloudwatchLogsExports:     logExports,
	}

	inst, err := h.Backend.ModifyDBInstance(id, instanceClass, allocatedStorage, opts)
	if err != nil {
		return nil, err
	}

	return &modifyDBInstanceResponse{
		Xmlns:      rdsXMLNS,
		DBInstance: toXMLInstance(inst),
	}, nil
}

func toXMLInstance(inst *DBInstance) xmlDBInstance {
	var instanceCreateTime string
	if !inst.InstanceCreateTime.IsZero() {
		instanceCreateTime = inst.InstanceCreateTime.UTC().Format(time.RFC3339)
	}
	result := xmlDBInstance{
		DBInstanceIdentifier:              inst.DBInstanceIdentifier,
		DBInstanceArn:                     inst.DBInstanceArn,
		DbiResourceID:                     inst.DbiResourceID,
		DBInstanceClass:                   inst.DBInstanceClass,
		DBClusterIdentifier:               inst.DBClusterIdentifier,
		Engine:                            inst.Engine,
		EngineVersion:                     inst.EngineVersion,
		DBInstanceStatus:                  inst.DBInstanceStatus,
		MasterUsername:                    inst.MasterUsername,
		DBName:                            inst.DBName,
		Endpoint:                          inst.Endpoint,
		Port:                              inst.Port,
		AllocatedStorage:                  inst.AllocatedStorage,
		Iops:                              inst.Iops,
		StorageThroughput:                 inst.StorageThroughput,
		VpcID:                             inst.VpcID,
		DBSubnetGroupName:                 inst.DBSubnetGroupName,
		ReplicaSourceDBInstanceIdentifier: inst.ReplicaSourceDBInstanceIdentifier,
		StorageType:                       inst.StorageType,
		StorageEncrypted:                  inst.StorageEncrypted,
		MultiAZ:                           inst.MultiAZ,
		AvailabilityZone:                  inst.AvailabilityZone,
		BackupRetentionPeriod:             inst.BackupRetentionPeriod,
		IAMDatabaseAuthenticationEnabled:  inst.IAMDatabaseAuthenticationEnabled,
		DeletionProtection:                inst.DeletionProtection,
		LicenseModel:                      inst.LicenseModel,
		MonitoringInterval:                inst.MonitoringInterval,
		MonitoringRoleArn:                 inst.MonitoringRoleArn,
		EnhancedMonitoringResourceArn:     inst.EnhancedMonitoringResourceArn,
		PreferredMaintenanceWindow:        inst.PreferredMaintenanceWindow,
		PreferredBackupWindow:             inst.PreferredBackupWindow,
		KmsKeyID:                          inst.KmsKeyID,
		CopyTagsToSnapshot:                inst.CopyTagsToSnapshot,
		PubliclyAccessible:                inst.PubliclyAccessible,
		PerformanceInsightsEnabled:        inst.PerformanceInsightsEnabled,
		StorageOptimized:                  inst.StorageOptimized,
		OptimizedWrites:                   inst.OptimizedWrites,
		EngineLifecycleSupport:            inst.EngineLifecycleSupport,
		InstanceCreateTime:                instanceCreateTime,
	}

	applyXMLInstanceGroups(inst, &result)

	if len(inst.VpcSecurityGroups) > 0 {
		members := make([]xmlVpcSecurityGroupMembership, 0, len(inst.VpcSecurityGroups))
		for _, sg := range inst.VpcSecurityGroups {
			members = append(members, xmlVpcSecurityGroupMembership(sg))
		}

		result.VpcSecurityGroups = &xmlVpcSecurityGroupList{Members: members}
	}

	if len(inst.DBSecurityGroups) > 0 {
		members := make([]xmlDBSecGroupMembership, 0, len(inst.DBSecurityGroups))
		for _, sg := range inst.DBSecurityGroups {
			members = append(members, xmlDBSecGroupMembership(sg))
		}

		result.DBSecurityGroups = &xmlDBSecGroupList{Members: members}
	}

	if len(inst.ReadReplicaIdentifiers) > 0 {
		members := make([]xmlReadReplicaIdentifier, 0, len(inst.ReadReplicaIdentifiers))
		for _, rid := range inst.ReadReplicaIdentifiers {
			members = append(members, xmlReadReplicaIdentifier{Value: rid})
		}

		result.ReadReplicaDBInstanceIdentifiers = &xmlReadReplicaIdentifierList{Members: members}
	}

	if len(inst.EnabledCloudwatchLogsExports) > 0 {
		members := make([]xmlLogTypeMember, 0, len(inst.EnabledCloudwatchLogsExports))
		for _, lt := range inst.EnabledCloudwatchLogsExports {
			members = append(members, xmlLogTypeMember{Value: lt})
		}

		result.EnabledCloudwatchLogsExports = &xmlLogTypeList{Members: members}
	}

	return result
}

// applyXMLInstanceGroups fills the pending-modified-values, DB parameter
// group, and option group membership fields of result from inst. Split out
// of toXMLInstance to keep that function under the funlen limit.
func applyXMLInstanceGroups(inst *DBInstance, result *xmlDBInstance) {
	if inst.DBInstanceStatus == instanceStatusModifying {
		if pv := inst.PendingModifiedValues; pv != nil {
			xpv := &xmlPendingModifiedValues{
				DBInstanceClass:  pv.DBInstanceClass,
				EngineVersion:    pv.EngineVersion,
				AllocatedStorage: pv.AllocatedStorage,
				Iops:             pv.Iops,
			}
			if pv.MultiAZChange != nil {
				xpv.MultiAZ = *pv.MultiAZChange
			}
			result.PendingModifiedValues = xpv
		} else {
			result.PendingModifiedValues = &xmlPendingModifiedValues{}
		}
	}

	if inst.DBParameterGroupName != "" {
		result.DBParameterGroups = &xmlDBParamGroupsWrapper{
			Members: []xmlDBParamGroupStatus{
				{DBParameterGroupName: inst.DBParameterGroupName, ParameterApplyStatus: dbParameterApplyStatusInSync},
			},
		}
	}

	if inst.OptionGroupName != "" {
		result.OptionGroupMemberships = &xmlOptionGroupMembershipList{
			Members: []xmlOptionGroupMembership{
				{OptionGroupName: inst.OptionGroupName, Status: optionGroupMembershipStatusInSync},
			},
		}
	}
}

// xmlDBParamGroupStatus mirrors types.DBParameterGroupStatus
// (rds@v1.124.1 types/types.go:2703). Confirmed against
// awsAwsquery_deserializeDocumentDBParameterGroupStatusList
// (deserializers.go:37336): the list element name is "DBParameterGroup", not
// "DBParameterGroupStatus" -- a prior shape wrapped a single struct under the
// wrong element name, so a real client's DBParameterGroups was always empty.
type xmlDBParamGroupStatus struct {
	DBParameterGroupName string `xml:"DBParameterGroupName,omitempty"`
	ParameterApplyStatus string `xml:"ParameterApplyStatus,omitempty"`
}

// optionGroupMembershipStatusInSync is the status AWS reports for an option
// group membership applied statically (no pending change), matching this
// backend's always-apply-immediately option group model.
const optionGroupMembershipStatusInSync = "in-sync"

// dbParameterApplyStatusInSync is the status AWS reports for a DB parameter
// group applied statically (no pending change), matching this backend's
// always-apply-immediately parameter group model.
const dbParameterApplyStatusInSync = "in-sync"

type xmlOptionGroupMembership struct {
	OptionGroupName string `xml:"OptionGroupName"`
	Status          string `xml:"Status"`
}

type xmlOptionGroupMembershipList struct {
	Members []xmlOptionGroupMembership `xml:"OptionGroupMembership"`
}

type xmlDBParamGroupsWrapper struct {
	Members []xmlDBParamGroupStatus `xml:"DBParameterGroup"`
}

type xmlVpcSecurityGroupMembership struct {
	VpcSecurityGroupID string `xml:"VpcSecurityGroupId"`
	Status             string `xml:"Status"`
}

type xmlVpcSecurityGroupList struct {
	Members []xmlVpcSecurityGroupMembership `xml:"VpcSecurityGroupMembership"`
}

// xmlDBSecGroupMembership mirrors types.DBSecurityGroupMembership
// (rds@v1.124.1 types/types.go:3132); confirmed against
// awsAwsquery_deserializeDocumentDBSecurityGroupMembershipList
// (deserializers.go), the list element name is "DBSecurityGroup".
type xmlDBSecGroupMembership struct {
	DBSecurityGroupName string `xml:"DBSecurityGroupName"`
	Status              string `xml:"Status"`
}

type xmlDBSecGroupList struct {
	Members []xmlDBSecGroupMembership `xml:"DBSecurityGroup"`
}

type xmlReadReplicaIdentifier struct {
	Value string `xml:",chardata"`
}

type xmlReadReplicaIdentifierList struct {
	Members []xmlReadReplicaIdentifier `xml:"ReadReplicaDBInstanceIdentifier"`
}

type xmlLogTypeMember struct {
	Value string `xml:",chardata"`
}

type xmlLogTypeList struct {
	Members []xmlLogTypeMember `xml:"member"`
}

type xmlPendingModifiedValues struct {
	DBInstanceClass  string `xml:"DBInstanceClass,omitempty"`
	EngineVersion    string `xml:"EngineVersion,omitempty"`
	AllocatedStorage int    `xml:"AllocatedStorage,omitempty"`
	Iops             int    `xml:"Iops,omitempty"`
	MultiAZ          bool   `xml:"MultiAZ,omitempty"`
}

type xmlDBInstance struct {
	DBParameterGroups                 *xmlDBParamGroupsWrapper      `xml:"DBParameterGroups,omitempty"`
	VpcSecurityGroups                 *xmlVpcSecurityGroupList      `xml:"VpcSecurityGroups,omitempty"`
	DBSecurityGroups                  *xmlDBSecGroupList            `xml:"DBSecurityGroups,omitempty"`
	ReadReplicaDBInstanceIdentifiers  *xmlReadReplicaIdentifierList `xml:"ReadReplicaDBInstanceIdentifiers,omitempty"`
	EnabledCloudwatchLogsExports      *xmlLogTypeList               `xml:"EnabledCloudwatchLogsExports,omitempty"`
	PendingModifiedValues             *xmlPendingModifiedValues     `xml:"PendingModifiedValues,omitempty"`
	OptionGroupMemberships            *xmlOptionGroupMembershipList `xml:"OptionGroupMemberships,omitempty"`
	LicenseModel                      string                        `xml:"LicenseModel,omitempty"`
	PreferredBackupWindow             string                        `xml:"PreferredBackupWindow,omitempty"`
	DBInstanceClass                   string                        `xml:"DBInstanceClass"`
	DBClusterIdentifier               string                        `xml:"DBClusterIdentifier,omitempty"`
	Engine                            string                        `xml:"Engine"`
	EngineVersion                     string                        `xml:"EngineVersion,omitempty"`
	DBInstanceStatus                  string                        `xml:"DBInstanceStatus"`
	MasterUsername                    string                        `xml:"MasterUsername"`
	DBName                            string                        `xml:"DBName,omitempty"`
	Endpoint                          string                        `xml:"Endpoint>Address"`
	VpcID                             string                        `xml:"DBSubnetGroup>VpcId,omitempty"`
	DBSubnetGroupName                 string                        `xml:"DBSubnetGroup>DBSubnetGroupName,omitempty"`
	ReplicaSourceDBInstanceIdentifier string                        `xml:"ReadReplicaSourceDBInstanceIdentifier,omitempty"`
	StorageType                       string                        `xml:"StorageType,omitempty"`
	AvailabilityZone                  string                        `xml:"AvailabilityZone,omitempty"`
	DBInstanceIdentifier              string                        `xml:"DBInstanceIdentifier"`
	MonitoringRoleArn                 string                        `xml:"MonitoringRoleArn,omitempty"`
	EnhancedMonitoringResourceArn     string                        `xml:"EnhancedMonitoringResourceArn,omitempty"`
	PreferredMaintenanceWindow        string                        `xml:"PreferredMaintenanceWindow,omitempty"`
	DbiResourceID                     string                        `xml:"DbiResourceId,omitempty"`
	DBInstanceArn                     string                        `xml:"DBInstanceArn,omitempty"`
	KmsKeyID                          string                        `xml:"KmsKeyId,omitempty"`
	InstanceCreateTime                string                        `xml:"InstanceCreateTime,omitempty"`
	EngineLifecycleSupport            string                        `xml:"EngineLifecycleSupport,omitempty"`
	AllocatedStorage                  int                           `xml:"AllocatedStorage"`
	Iops                              int                           `xml:"Iops,omitempty"`
	StorageThroughput                 int                           `xml:"StorageThroughput,omitempty"`
	BackupRetentionPeriod             int                           `xml:"BackupRetentionPeriod"`
	MonitoringInterval                int                           `xml:"MonitoringInterval,omitempty"`
	Port                              int                           `xml:"Endpoint>Port"`
	StorageEncrypted                  bool                          `xml:"StorageEncrypted"`
	IAMDatabaseAuthenticationEnabled  bool                          `xml:"IAMDatabaseAuthenticationEnabled,omitempty"`
	DeletionProtection                bool                          `xml:"DeletionProtection,omitempty"`
	CopyTagsToSnapshot                bool                          `xml:"CopyTagsToSnapshot,omitempty"`
	PubliclyAccessible                bool                          `xml:"PubliclyAccessible,omitempty"`
	PerformanceInsightsEnabled        bool                          `xml:"PerformanceInsightsEnabled,omitempty"`
	StorageOptimized                  bool                          `xml:"StorageOptimized,omitempty"`
	OptimizedWrites                   bool                          `xml:"OptimizedWritesEnabled,omitempty"`
	MultiAZ                           bool                          `xml:"MultiAZ"`
}

type xmlDBInstanceList struct {
	Members []xmlDBInstance `xml:"DBInstance"`
}

type createDBInstanceResponse struct {
	XMLName    xml.Name      `xml:"CreateDBInstanceResponse"`
	Xmlns      string        `xml:"xmlns,attr"`
	DBInstance xmlDBInstance `xml:"CreateDBInstanceResult>DBInstance"`
}

type deleteDBInstanceResponse struct {
	XMLName    xml.Name      `xml:"DeleteDBInstanceResponse"`
	Xmlns      string        `xml:"xmlns,attr"`
	DBInstance xmlDBInstance `xml:"DeleteDBInstanceResult>DBInstance"`
}

type describeDBInstancesResponse struct {
	XMLName     xml.Name          `xml:"DescribeDBInstancesResponse"`
	Xmlns       string            `xml:"xmlns,attr"`
	Marker      string            `xml:"DescribeDBInstancesResult>Marker,omitempty"`
	DBInstances xmlDBInstanceList `xml:"DescribeDBInstancesResult>DBInstances"`
}

type modifyDBInstanceResponse struct {
	XMLName    xml.Name      `xml:"ModifyDBInstanceResponse"`
	Xmlns      string        `xml:"xmlns,attr"`
	DBInstance xmlDBInstance `xml:"ModifyDBInstanceResult>DBInstance"`
}

func (h *Handler) handleCreateDBInstanceReadReplica(vals url.Values) (any, error) {
	id := vals.Get("DBInstanceIdentifier")
	sourceID := vals.Get("SourceDBInstanceIdentifier")
	sourceRegion := vals.Get("SourceRegion")
	paramGroupName := vals.Get("DBParameterGroupName")
	optionGroupName := vals.Get("OptionGroupName")
	inst, err := h.Backend.CreateDBInstanceReadReplica(id, sourceID, sourceRegion, paramGroupName, optionGroupName)
	if err != nil {
		return nil, err
	}

	h.applyCreateTags(vals, inst.DBInstanceArn)

	return &createDBInstanceReadReplicaResponse{
		Xmlns:      rdsXMLNS,
		DBInstance: toXMLInstance(inst),
	}, nil
}

func (h *Handler) handlePromoteReadReplica(vals url.Values) (any, error) {
	id := vals.Get("DBInstanceIdentifier")
	inst, err := h.Backend.PromoteReadReplica(id)
	if err != nil {
		return nil, err
	}

	return &promoteReadReplicaResponse{
		Xmlns:      rdsXMLNS,
		DBInstance: toXMLInstance(inst),
	}, nil
}

func (h *Handler) handleRebootDBInstance(vals url.Values) (any, error) {
	id := vals.Get("DBInstanceIdentifier")
	inst, err := h.Backend.RebootDBInstance(id)
	if err != nil {
		return nil, err
	}

	return &rebootDBInstanceResponse{
		Xmlns:      rdsXMLNS,
		DBInstance: toXMLInstance(inst),
	}, nil
}

func (h *Handler) handleDescribeValidDBInstanceModifications(vals url.Values) (any, error) {
	id := vals.Get("DBInstanceIdentifier")
	if _, err := h.Backend.DescribeValidDBInstanceModifications(id); err != nil {
		return nil, err
	}

	features := []xmlAvailableProcessorFeature{
		{Name: "coreCount", DefaultValue: "2", AllowedValues: "1,2,4,8"},
		{Name: "threadsPerCore", DefaultValue: "2", AllowedValues: "1,2"},
	}

	return &describeValidDBInstanceModificationsResponse{
		Xmlns: rdsXMLNS,
		Result: xmlValidDBInstanceModificationsWrapper{
			Message: xmlValidDBInstanceModificationsMessage{
				ValidProcessorFeatures: xmlAvailableProcessorFeatureList{Members: features},
			},
		},
	}, nil
}

type createDBInstanceReadReplicaResponse struct {
	XMLName    xml.Name      `xml:"CreateDBInstanceReadReplicaResponse"`
	Xmlns      string        `xml:"xmlns,attr"`
	DBInstance xmlDBInstance `xml:"CreateDBInstanceReadReplicaResult>DBInstance"`
}

type promoteReadReplicaResponse struct {
	XMLName    xml.Name      `xml:"PromoteReadReplicaResponse"`
	Xmlns      string        `xml:"xmlns,attr"`
	DBInstance xmlDBInstance `xml:"PromoteReadReplicaResult>DBInstance"`
}

type rebootDBInstanceResponse struct {
	XMLName    xml.Name      `xml:"RebootDBInstanceResponse"`
	Xmlns      string        `xml:"xmlns,attr"`
	DBInstance xmlDBInstance `xml:"RebootDBInstanceResult>DBInstance"`
}

type xmlAvailableProcessorFeature struct {
	Name          string `xml:"Name"`
	DefaultValue  string `xml:"DefaultValue"`
	AllowedValues string `xml:"AllowedValues"`
}

type xmlAvailableProcessorFeatureList struct {
	Members []xmlAvailableProcessorFeature `xml:"AvailableProcessorFeature"`
}

type xmlValidDBInstanceModificationsMessage struct {
	ValidProcessorFeatures xmlAvailableProcessorFeatureList `xml:"ValidProcessorFeatures"`
}

type xmlValidDBInstanceModificationsWrapper struct {
	Message xmlValidDBInstanceModificationsMessage `xml:"ValidDBInstanceModificationsMessage"`
}

type describeValidDBInstanceModificationsResponse struct {
	XMLName xml.Name                               `xml:"DescribeValidDBInstanceModificationsResponse"`
	Xmlns   string                                 `xml:"xmlns,attr"`
	Result  xmlValidDBInstanceModificationsWrapper `xml:"DescribeValidDBInstanceModificationsResult"`
}

type restoreDBInstanceToPointInTimeResponse struct {
	XMLName    xml.Name      `xml:"RestoreDBInstanceToPointInTimeResponse"`
	Xmlns      string        `xml:"xmlns,attr"`
	DBInstance xmlDBInstance `xml:"RestoreDBInstanceToPointInTimeResult>DBInstance"`
}

type startDBInstanceResponse struct {
	XMLName    xml.Name      `xml:"StartDBInstanceResponse"`
	Xmlns      string        `xml:"xmlns,attr"`
	DBInstance xmlDBInstance `xml:"StartDBInstanceResult>DBInstance"`
}

type stopDBInstanceResponse struct {
	XMLName    xml.Name      `xml:"StopDBInstanceResponse"`
	Xmlns      string        `xml:"xmlns,attr"`
	DBInstance xmlDBInstance `xml:"StopDBInstanceResult>DBInstance"`
}

func (h *Handler) handleRestoreDBInstanceToPointInTime(vals url.Values) (any, error) {
	id := vals.Get("TargetDBInstanceIdentifier")
	sourceID := vals.Get("SourceDBInstanceIdentifier")
	opts := DBInstanceOptions{
		MultiAZ:              vals.Get("MultiAZ") == formTrue,
		DeletionProtection:   vals.Get("DeletionProtection") == formTrue,
		StorageType:          vals.Get("StorageType"),
		AvailabilityZone:     vals.Get("AvailabilityZone"),
		DBParameterGroupName: vals.Get("DBParameterGroupName"),
		OptionGroupName:      vals.Get("OptionGroupName"),
	}

	inst, err := h.Backend.RestoreDBInstanceToPointInTime(id, sourceID, opts)
	if err != nil {
		return nil, err
	}

	return &restoreDBInstanceToPointInTimeResponse{
		Xmlns:      rdsXMLNS,
		DBInstance: toXMLInstance(inst),
	}, nil
}

func (h *Handler) handleStartDBInstance(vals url.Values) (any, error) {
	id := vals.Get("DBInstanceIdentifier")

	inst, err := h.Backend.StartDBInstance(id)
	if err != nil {
		return nil, err
	}

	return &startDBInstanceResponse{
		Xmlns:      rdsXMLNS,
		DBInstance: toXMLInstance(inst),
	}, nil
}

func (h *Handler) handleStopDBInstance(vals url.Values) (any, error) {
	id := vals.Get("DBInstanceIdentifier")

	inst, err := h.Backend.StopDBInstance(id)
	if err != nil {
		return nil, err
	}

	return &stopDBInstanceResponse{
		Xmlns:      rdsXMLNS,
		DBInstance: toXMLInstance(inst),
	}, nil
}

type switchoverReadReplicaResponse struct {
	XMLName    xml.Name      `xml:"SwitchoverReadReplicaResponse"`
	Xmlns      string        `xml:"xmlns,attr"`
	DBInstance xmlDBInstance `xml:"SwitchoverReadReplicaResult>DBInstance"`
}

type restoreDBInstanceFromS3Response struct {
	XMLName    xml.Name      `xml:"RestoreDBInstanceFromS3Response"`
	Xmlns      string        `xml:"xmlns,attr"`
	DBInstance xmlDBInstance `xml:"RestoreDBInstanceFromS3Result>DBInstance"`
}

func (h *Handler) handleSwitchoverReadReplica(vals url.Values) (any, error) {
	instanceID := vals.Get("DBInstanceIdentifier")
	inst, err := h.Backend.SwitchoverReadReplica(instanceID)
	if err != nil {
		return nil, err
	}

	return &switchoverReadReplicaResponse{
		Xmlns:      rdsXMLNS,
		DBInstance: toXMLInstance(inst),
	}, nil
}

func (h *Handler) handleRestoreDBInstanceFromS3(vals url.Values) (any, error) {
	id := vals.Get("DBInstanceIdentifier")
	engine := vals.Get("Engine")
	dbInstanceClass := vals.Get("DBInstanceClass")
	s3Bucket := vals.Get("S3BucketName")
	s3IngestionRoleArn := vals.Get("S3IngestionRoleArn")
	sourceEngine := vals.Get("SourceEngine")
	sourceEngineVersion := vals.Get("SourceEngineVersion")
	paramGroupName := vals.Get("DBParameterGroupName")
	optionGroupName := vals.Get("OptionGroupName")
	inst, err := h.Backend.RestoreDBInstanceFromS3(
		id, engine, dbInstanceClass, s3Bucket, s3IngestionRoleArn, sourceEngine, sourceEngineVersion,
		paramGroupName, optionGroupName,
	)
	if err != nil {
		return nil, err
	}

	return &restoreDBInstanceFromS3Response{
		Xmlns:      rdsXMLNS,
		DBInstance: toXMLInstance(inst),
	}, nil
}

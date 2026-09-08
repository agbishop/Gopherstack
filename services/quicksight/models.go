package quicksight

import (
	"encoding/json"
	"time"
)

type storedNamespace struct {
	Name           string `json:"name"`
	Arn            string `json:"arn"`
	CapacityRegion string `json:"capacityRegion"`
	Status         string `json:"status"`
	IdentityStore  string `json:"identityStore"`
}

func (n *storedNamespace) toNamespace() *Namespace {
	return &Namespace{
		Name:           n.Name,
		Arn:            n.Arn,
		CapacityRegion: n.CapacityRegion,
		CreationStatus: n.Status,
		IdentityStore:  n.IdentityStore,
	}
}

type storedGroup struct {
	GroupName   string `json:"groupName"`
	Arn         string `json:"arn"`
	Description string `json:"description"`
	Namespace   string `json:"namespace"`
	PrincipalID string `json:"principalId"`
}

func (g *storedGroup) toGroup() *Group {
	return &Group{
		GroupName:   g.GroupName,
		Arn:         g.Arn,
		Description: g.Description,
		Namespace:   g.Namespace,
		PrincipalID: g.PrincipalID,
	}
}

type storedUser struct {
	UserName     string `json:"userName"`
	Arn          string `json:"arn"`
	Email        string `json:"email"`
	Role         string `json:"role"`
	IdentityType string `json:"identityType"`
	Namespace    string `json:"namespace"`
	PrincipalID  string `json:"principalId"`
	SessionName  string `json:"sessionName"`
	Active       bool   `json:"active"`
}

func (u *storedUser) toUser() *User {
	return &User{
		UserName:     u.UserName,
		Arn:          u.Arn,
		Email:        u.Email,
		Role:         u.Role,
		IdentityType: u.IdentityType,
		Namespace:    u.Namespace,
		PrincipalID:  u.PrincipalID,
		SessionName:  u.SessionName,
		Active:       u.Active,
	}
}

type storedDataSource struct {
	CreatedTime     time.Time            `json:"createdTime"`
	LastUpdatedTime time.Time            `json:"lastUpdatedTime"`
	DataSourceID    string               `json:"dataSourceId"`
	Arn             string               `json:"arn"`
	Name            string               `json:"name"`
	Type            string               `json:"type"`
	Status          string               `json:"status"`
	Permissions     []ResourcePermission `json:"permissions,omitempty"`
}

func (d *storedDataSource) toDataSource() *DataSource {
	return &DataSource{
		CreatedTime:     d.CreatedTime,
		LastUpdatedTime: d.LastUpdatedTime,
		DataSourceID:    d.DataSourceID,
		Arn:             d.Arn,
		Name:            d.Name,
		Type:            d.Type,
		Status:          d.Status,
		Permissions:     clonePermissions(d.Permissions),
	}
}

type storedDataSet struct {
	CreatedTime       time.Time                         `json:"createdTime"`
	LastUpdatedTime   time.Time                         `json:"lastUpdatedTime"`
	RefreshSchedules  map[string]*storedRefreshSchedule `json:"refreshSchedules,omitempty"`
	RefreshProperties *storedDataSetRefreshProperties   `json:"refreshProperties,omitempty"`
	PhysicalTableMap  map[string]PhysicalTable          `json:"physicalTableMap,omitempty"`
	LogicalTableMap   map[string]LogicalTable           `json:"logicalTableMap,omitempty"`
	DataSetID         string                            `json:"dataSetId"`
	Arn               string                            `json:"arn"`
	Name              string                            `json:"name"`
	ImportMode        string                            `json:"importMode"`
	Permissions       []ResourcePermission              `json:"permissions,omitempty"`
}

func (d *storedDataSet) toDataSet() *DataSet {
	return &DataSet{
		CreatedTime:      d.CreatedTime,
		LastUpdatedTime:  d.LastUpdatedTime,
		DataSetID:        d.DataSetID,
		Arn:              d.Arn,
		Name:             d.Name,
		ImportMode:       d.ImportMode,
		Permissions:      clonePermissions(d.Permissions),
		PhysicalTableMap: clonePhysicalTableMap(d.PhysicalTableMap),
		LogicalTableMap:  cloneLogicalTableMap(d.LogicalTableMap),
	}
}

type storedIngestion struct {
	CreatedTime     time.Time `json:"createdTime"`
	IngestionID     string    `json:"ingestionId"`
	Arn             string    `json:"arn"`
	DataSetID       string    `json:"dataSetId"`
	IngestionStatus string    `json:"ingestionStatus"`
}

func (i *storedIngestion) toIngestion() *Ingestion {
	return &Ingestion{
		CreatedTime:     i.CreatedTime,
		IngestionID:     i.IngestionID,
		Arn:             i.Arn,
		DataSetID:       i.DataSetID,
		IngestionStatus: i.IngestionStatus,
	}
}

type storedDashboard struct {
	CreatedTime time.Time `json:"createdTime"`
	// DeletedVersions tracks version numbers removed by a targeted
	// DeleteDashboard(VersionNumber) call. This backend has no real
	// per-version content storage (see DeleteDashboard's doc comment), but a
	// deleted version number must still stop being reported live by
	// ListDashboardVersions and must 404 rather than re-succeed on a repeat
	// delete -- both observable independent of full version history.
	DeletedVersions        map[int64]bool       `json:"deletedVersions,omitempty"`
	LastUpdatedTime        time.Time            `json:"lastUpdatedTime"`
	LastPublishedTime      time.Time            `json:"lastPublishedTime"`
	Definition             map[string]any       `json:"definition,omitempty"`
	DashboardID            string               `json:"dashboardId"`
	Arn                    string               `json:"arn"`
	Name                   string               `json:"name"`
	Status                 string               `json:"status"`
	ThemeArn               string               `json:"themeArn,omitempty"`
	VersionDescription     string               `json:"versionDescription,omitempty"`
	Permissions            []ResourcePermission `json:"permissions,omitempty"`
	LinkEntities           []string             `json:"linkEntities,omitempty"`
	VersionNumber          int64                `json:"versionNumber"`
	PublishedVersionNumber int64                `json:"publishedVersionNumber"`
}

func (d *storedDashboard) toDashboard() *Dashboard {
	return &Dashboard{
		CreatedTime:            d.CreatedTime,
		LastUpdatedTime:        d.LastUpdatedTime,
		LastPublishedTime:      d.LastPublishedTime,
		DashboardID:            d.DashboardID,
		Arn:                    d.Arn,
		Name:                   d.Name,
		Status:                 d.Status,
		ThemeArn:               d.ThemeArn,
		VersionDescription:     d.VersionDescription,
		VersionNumber:          d.VersionNumber,
		PublishedVersionNumber: d.PublishedVersionNumber,
		Definition:             d.Definition,
		Permissions:            clonePermissions(d.Permissions),
		LinkEntities:           append([]string(nil), d.LinkEntities...),
	}
}

// storedRefreshSchedule is the persisted representation of one dataset SPICE
// refresh schedule, keyed by ScheduleId.
type storedRefreshSchedule struct {
	StartAfterDateTime time.Time      `json:"startAfterDateTime,omitzero"`
	ScheduleFrequency  map[string]any `json:"scheduleFrequency,omitempty"`
	ScheduleID         string         `json:"scheduleId"`
	Arn                string         `json:"arn"`
	RefreshType        string         `json:"refreshType"`
}

func (s *storedRefreshSchedule) toRefreshSchedule() *RefreshSchedule {
	return &RefreshSchedule{
		ScheduleID:         s.ScheduleID,
		Arn:                s.Arn,
		RefreshType:        s.RefreshType,
		StartAfterDateTime: s.StartAfterDateTime,
		ScheduleFrequency:  s.ScheduleFrequency,
	}
}

// storedDataSetRefreshProperties is the persisted representation of a dataset's
// SPICE refresh configuration.
type storedDataSetRefreshProperties struct {
	RefreshConfiguration map[string]any `json:"refreshConfiguration,omitempty"`
	FailureConfiguration map[string]any `json:"failureConfiguration,omitempty"`
}

func (p *storedDataSetRefreshProperties) toDataSetRefreshProperties() *DataSetRefreshProperties {
	return &DataSetRefreshProperties{
		RefreshConfiguration: p.RefreshConfiguration,
		FailureConfiguration: p.FailureConfiguration,
	}
}

type storedAnalysis struct {
	CreatedTime     time.Time            `json:"createdTime"`
	LastUpdatedTime time.Time            `json:"lastUpdatedTime"`
	Definition      map[string]any       `json:"definition,omitempty"`
	AnalysisID      string               `json:"analysisId"`
	Arn             string               `json:"arn"`
	Name            string               `json:"name"`
	ThemeArn        string               `json:"themeArn,omitempty"`
	Status          string               `json:"status"`
	Permissions     []ResourcePermission `json:"permissions,omitempty"`
}

func (a *storedAnalysis) toAnalysis() *Analysis {
	return &Analysis{
		CreatedTime:     a.CreatedTime,
		LastUpdatedTime: a.LastUpdatedTime,
		AnalysisID:      a.AnalysisID,
		Arn:             a.Arn,
		Name:            a.Name,
		ThemeArn:        a.ThemeArn,
		Status:          a.Status,
		Definition:      a.Definition,
		Permissions:     clonePermissions(a.Permissions),
	}
}

// backendSnapshot is the top-level on-disk shape for the QuickSight backend.
//
// Tables holds one JSON-encoded array per registered store.Table (see
// store_setup.go's registerAllTables), produced by
// [store.Registry.SnapshotAll]. The remaining fields are the resource
// collections left as raw maps because their value type carries no identity
// field of its own (see store_setup.go's doc comment for the full list and
// rationale) -- these round-trip directly, same as before conversion.
type backendSnapshot struct {
	Tables map[string]json.RawMessage `json:"tables"`

	GroupMembers map[string]bool              `json:"groupMembers"`
	Tags         map[string]map[string]string `json:"tags"`

	AccountSettings          map[string]*storedAccountSettings             `json:"accountSettings"`
	AccountSubscriptions     map[string]*storedAccountSubscription         `json:"accountSubscriptions"`
	AccountCustomPermissions map[string]string                             `json:"accountCustomPermissions"`
	IPRestrictions           map[string]*storedIPRestriction               `json:"ipRestrictions"`
	PublicSharing            map[string]bool                               `json:"publicSharing"`
	KeyRegistrations         map[string][]storedRegisteredKey              `json:"keyRegistrations"`
	DefaultQBusinessApps     map[string]*storedDefaultQBusinessApplication `json:"defaultQBusinessApps"`
	QPersonalization         map[string]string                             `json:"qPersonalization"`
	QSearchConfig            map[string]string                             `json:"qSearchConfig"`
	DashboardsQAConfig       map[string]string                             `json:"dashboardsQAConfig"`

	BrandAssignments      map[string]string `json:"brandAssignments"`
	RoleCustomPermissions map[string]string `json:"roleCustomPermissions"`
	RoleMemberships       map[string]bool   `json:"roleMemberships"`
	UserCustomPermissions map[string]string `json:"userCustomPermissions"`

	SPICECapacity     map[string]string `json:"spiceCapacity"`
	SelfUpgradeConfig map[string]string `json:"selfUpgradeConfig"`

	Version int `json:"version"`
}

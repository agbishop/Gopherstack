package quicksight

import "time"

// Namespace represents a QuickSight namespace.
type Namespace struct {
	CreationStatus string
	Name           string
	Arn            string
	CapacityRegion string
	IdentityStore  string
}

// Group represents a QuickSight group.
type Group struct {
	GroupName   string
	Arn         string
	Description string
	Namespace   string
	PrincipalID string
}

// GroupMember represents a QuickSight group member.
type GroupMember struct {
	MemberName string
	Arn        string
}

// User represents a QuickSight user.
type User struct {
	UserName              string
	Arn                   string
	Email                 string
	Role                  string
	IdentityType          string
	Namespace             string
	PrincipalID           string
	SessionName           string
	CustomPermissionsName string
	Active                bool
}

// DataSource represents a QuickSight data source.
// CreatedTime first: non-pointer prefix reduces GC pointer bytes.
type DataSource struct {
	CreatedTime     time.Time
	LastUpdatedTime time.Time
	DataSourceID    string
	Arn             string
	Name            string
	Type            string
	Status          string
	Permissions     []ResourcePermission
}

// DataSet represents a QuickSight dataset.
// CreatedTime first: non-pointer prefix reduces GC pointer bytes.
type DataSet struct {
	CreatedTime      time.Time
	LastUpdatedTime  time.Time
	PhysicalTableMap map[string]PhysicalTable
	LogicalTableMap  map[string]LogicalTable
	DataSetID        string
	Arn              string
	Name             string
	ImportMode       string
	Permissions      []ResourcePermission
}

// InputColumn describes one column of a PhysicalTable's underlying schema
// (quicksight@v1.123.1 types.InputColumn).
type InputColumn struct {
	ID      string
	Name    string
	Type    string
	SubType string
}

// UploadSettings describes the file format of an S3Source/FileSource
// physical table (quicksight@v1.123.1 types.UploadSettings).
type UploadSettings struct {
	ContainsHeader         *bool
	StartFromRow           *int32
	CustomCellAddressRange string
	Delimiter              string
	Format                 string
	TextQualifier          string
}

// TablePathElement identifies one step in a SaaSTable's hierarchical path
// (quicksight@v1.123.1 types.TablePathElement).
type TablePathElement struct {
	ID   string
	Name string
}

// RelationalTable is a PhysicalTable variant sourced from a relational data
// source (quicksight@v1.123.1 types.RelationalTable).
type RelationalTable struct {
	DataSourceArn string
	Name          string
	Catalog       string
	Schema        string
	InputColumns  []InputColumn
}

// CustomSQL is a PhysicalTable variant built from the result set of a custom
// SQL query (quicksight@v1.123.1 types.CustomSql).
type CustomSQL struct {
	DataSourceArn string
	Name          string
	SQLQuery      string
	Columns       []InputColumn
}

// S3Source is a PhysicalTable variant sourced from an S3 file
// (quicksight@v1.123.1 types.S3Source).
type S3Source struct {
	UploadSettings *UploadSettings
	DataSourceArn  string
	InputColumns   []InputColumn
}

// FileSource is a PhysicalTable variant sourced from an uploaded file
// (quicksight@v1.123.1 types.FileSource).
type FileSource struct {
	UploadSettings *UploadSettings
	DataSourceArn  string
	InputColumns   []InputColumn
	SheetIndex     int32
}

// SaaSTable is a PhysicalTable variant sourced from a SaaS connector
// (quicksight@v1.123.1 types.SaaSTable).
type SaaSTable struct {
	DataSourceArn string
	InputColumns  []InputColumn
	TablePath     []TablePathElement
}

// PhysicalTable is the union of underlying-source shapes a dataset can
// declare (quicksight@v1.123.1 types.PhysicalTable, an interface with
// PhysicalTableMember{CustomSql,FileSource,RelationalTable,S3Source,SaaSTable}
// implementations). Modeled here as a struct with one populated pointer
// field per wire union member, keyed by the same member name AWS uses on
// the wire, rather than as a Go interface -- valid input has exactly one
// non-nil field.
type PhysicalTable struct {
	RelationalTable *RelationalTable
	CustomSQL       *CustomSQL
	S3Source        *S3Source
	FileSource      *FileSource
	SaaSTable       *SaaSTable
}

// JoinInstruction is the ON-clause of a two-logical-table join
// (quicksight@v1.123.1 types.JoinInstruction). LeftJoinKeyProperties/
// RightJoinKeyProperties are optional SDK fields not modeled here: this
// backend never applies a join, so a table alias that is a join carries no
// backing state either way.
type JoinInstruction struct {
	LeftOperand  string
	OnClause     string
	RightOperand string
	Type         string
}

// LogicalTableSource identifies where a LogicalTable's rows come from: a
// PhysicalTable by ID, a join of two other logical tables, or another
// dataset's ARN (quicksight@v1.123.1 types.LogicalTableSource -- a union by
// doc comment, not an SDK interface; at most one field is expected set).
type LogicalTableSource struct {
	DataSetArn      string
	JoinInstruction *JoinInstruction
	PhysicalTableID string
}

// LogicalTable configures the combination/transformation of PhysicalTableMap
// entries (quicksight@v1.123.1 types.LogicalTable). DataTransforms is stored
// and echoed back verbatim rather than modeled: TransformOperation is a
// 10+-variant union (CastColumnType, CreateColumns, Filter, RenameColumn,
// ...) this in-memory backend never evaluates, so preserving the caller's
// raw JSON is honest where re-deriving typed structs it never acts on would
// not be.
type LogicalTable struct {
	Alias          string
	Source         *LogicalTableSource
	DataTransforms []any
}

// Ingestion represents a QuickSight ingestion.
// CreatedTime first: non-pointer prefix reduces GC pointer bytes.
type Ingestion struct {
	CreatedTime     time.Time
	IngestionID     string
	Arn             string
	DataSetID       string
	IngestionStatus string
}

// Dashboard represents a QuickSight dashboard.
// CreatedTime first: non-pointer prefix reduces GC pointer bytes.
type Dashboard struct {
	CreatedTime            time.Time
	LastUpdatedTime        time.Time
	LastPublishedTime      time.Time
	Definition             map[string]any
	DashboardID            string
	Arn                    string
	Name                   string
	Status                 string
	ThemeArn               string
	VersionDescription     string
	Permissions            []ResourcePermission
	LinkEntities           []string
	VersionNumber          int64
	PublishedVersionNumber int64
}

// DashboardVersion represents a version of a QuickSight dashboard.
type DashboardVersion struct {
	CreatedTime   time.Time
	Arn           string
	Status        string
	ThemeArn      string
	Description   string
	VersionNumber int64
}

// Analysis represents a QuickSight analysis.
// CreatedTime first: non-pointer prefix reduces GC pointer bytes.
type Analysis struct {
	CreatedTime     time.Time
	LastUpdatedTime time.Time
	AnalysisID      string
	Arn             string
	Name            string
	ThemeArn        string
	Status          string
	Definition      map[string]any
	Permissions     []ResourcePermission
}

// ResourcePermission represents a QuickSight principal + allowed actions grant.
type ResourcePermission struct {
	Principal string   `json:"principal"`
	Actions   []string `json:"actions"`
}

// FolderMember represents a QuickSight folder membership (a dashboard, analysis,
// dataset, or other asset that belongs to a folder).
type FolderMember struct {
	MemberID   string
	MemberType string
}

// FolderSearchFilter represents a single SearchFolders filter criterion.
type FolderSearchFilter struct {
	Operator string
	Name     string
	Value    string
}

// SearchFilter is a generic Name/Operator/Value search filter, shared by the
// Search{Analyses,Dashboards,DataSets,DataSources} operations. It has the same
// shape as [FolderSearchFilter] (SearchFolders' filter type).
type SearchFilter = FolderSearchFilter

// Folder represents a QuickSight folder.
// CreatedTime first: non-pointer prefix reduces GC pointer bytes.
type Folder struct {
	CreatedTime     time.Time
	LastUpdatedTime time.Time
	FolderID        string
	Arn             string
	Name            string
	FolderType      string
	ParentFolderArn string
	SharingModel    string
	Permissions     []ResourcePermission
}

// TemplateVersion represents one version of a QuickSight template.
// CreatedTime first: non-pointer prefix reduces GC pointer bytes.
type TemplateVersion struct {
	CreatedTime     time.Time
	Definition      map[string]any
	Arn             string
	Status          string
	SourceEntityArn string
	Description     string
	VersionNumber   int64
}

// Template represents a QuickSight template (a versioned, reusable dashboard/analysis
// layout definition).
type Template struct {
	CreatedTime     time.Time
	LastUpdatedTime time.Time
	TemplateID      string
	Arn             string
	Name            string
	Version         *TemplateVersion
	Permissions     []ResourcePermission
}

// TemplateAlias represents a named pointer to a specific template version.
type TemplateAlias struct {
	AliasName             string
	Arn                   string
	TemplateVersionNumber int64
}

// ThemeVersion represents one version of a QuickSight theme.
// CreatedTime first: non-pointer prefix reduces GC pointer bytes.
type ThemeVersion struct {
	CreatedTime   time.Time
	Configuration map[string]any
	Arn           string
	Status        string
	BaseThemeID   string
	Description   string
	VersionNumber int64
}

// Theme represents a QuickSight theme (a versioned set of visual style settings).
type Theme struct {
	CreatedTime     time.Time
	LastUpdatedTime time.Time
	ThemeID         string
	Arn             string
	Name            string
	Type            string
	Version         *ThemeVersion
	Permissions     []ResourcePermission
}

// ThemeAlias represents a named pointer to a specific theme version.
type ThemeAlias struct {
	AliasName          string
	Arn                string
	ThemeVersionNumber int64
}

// Topic represents a QuickSight topic (a natural-language Q&A data source).
// DataSetsV2/DataSetRelations/CustomInstructions/PublishOption are V2-only;
// DataSets/UserExperienceVersion are V1-only -- see topics_v2.go.
type Topic struct {
	CreatedTime           time.Time
	LastUpdatedTime       time.Time
	TopicID               string
	Arn                   string
	Name                  string
	Description           string
	UserExperienceVersion string
	CustomInstructions    string
	PublishOption         string
	DataSets              []map[string]any
	DataSetsV2            []map[string]any
	DataSetRelations      []map[string]any
	Permissions           []ResourcePermission
}

// TopicRefreshSchedule represents one refresh schedule for a topic's dataset,
// keyed by DatasetId.
type TopicRefreshSchedule struct {
	ScheduleConfig map[string]any
	DatasetID      string
	DatasetArn     string
	RefreshType    string
	IsEnabled      bool
}

// TopicRefreshDetails represents the status of a single topic refresh execution.
type TopicRefreshDetails struct {
	RefreshID     string
	RefreshStatus string
}

// TopicReviewedAnswer represents one human-reviewed answer attached to a topic.
type TopicReviewedAnswer struct {
	PrimaryVisual map[string]any
	Template      map[string]any
	AnswerID      string
	DatasetArn    string
	Question      string
	Mode          string
}

// TopicAnswerError represents a single failed entry in a batch reviewed-answer
// create/delete operation.
type TopicAnswerError struct {
	AnswerID string
	Message  string
}

// VPCConnection represents a QuickSight VPC connection.
type VPCConnection struct {
	CreatedTime        time.Time
	LastUpdatedTime    time.Time
	VPCConnectionID    string
	Arn                string
	Name               string
	VPCID              string
	RoleArn            string
	Status             string
	AvailabilityStatus string
	SubnetIDs          []string
	SecurityGroupIDs   []string
	DNSResolvers       []string
}

// IAMPolicyAssignment represents a QuickSight IAM policy assignment, scoped by namespace.
type IAMPolicyAssignment struct {
	Identities       map[string][]string
	AssignmentID     string
	AssignmentName   string
	AssignmentStatus string
	PolicyArn        string
	Namespace        string
	AwsAccountID     string
}

// AccountSettings represents a QuickSight account's account-wide settings.
type AccountSettings struct {
	AccountName                  string
	Edition                      string
	DefaultNamespace             string
	NotificationEmail            string
	PublicSharingEnabled         bool
	TerminationProtectionEnabled bool
}

// AccountSubscription represents a QuickSight account subscription.
type AccountSubscription struct {
	AccountName               string
	Edition                   string
	NotificationEmail         string
	AuthenticationType        string
	AccountSubscriptionStatus string
}

// AccountCustomization represents a QuickSight account's (or namespace's) branding customization.
type AccountCustomization struct {
	Namespace                         string
	DefaultTheme                      string
	DefaultEmailCustomizationTemplate string
}

// IPRestriction represents a QuickSight account's IP/VPC access restriction rules.
type IPRestriction struct {
	RuleMap              map[string]string
	VPCIDRuleMap         map[string]string
	VPCEndpointIDRuleMap map[string]string
	Enabled              bool
}

// RegisteredCustomerManagedKey represents a customer-managed KMS key registered with QuickSight.
type RegisteredCustomerManagedKey struct {
	KeyArn     string
	DefaultKey bool
}

// DefaultQBusinessApplication represents the default Amazon Q Business application
// linked to a QuickSight account.
type DefaultQBusinessApplication struct {
	ApplicationID string
	Namespace     string
}

// Brand represents a QuickSight brand, a versioned set of visual identity
// customizations (logo, theme, name) applied to the console/embedded experiences.
type Brand struct {
	CreatedTime        time.Time
	LastUpdatedTime    time.Time
	Definition         map[string]any
	BrandID            string
	Arn                string
	Status             string
	CurrentVersionID   string
	CurrentVersionStat string
	PublishedVersionID string
}

// CustomPermissions represents a named, reusable set of QuickSight capability
// overrides that can be attached to roles or users.
type CustomPermissions struct {
	Capabilities map[string]any
	Name         string
	Arn          string
}

// OAuthClientApplication represents a QuickSight OAuth 2.0 client application used
// to connect to a data source's identity provider.
type OAuthClientApplication struct {
	CreatedTime     time.Time
	LastUpdatedTime time.Time
	Extra           map[string]any
	ClientID        string
	Arn             string
	Name            string
	Status          string
}

// IdentityPropagationConfig represents the authorized targets for one downstream
// Amazon Web Services service that QuickSight can propagate an end user's identity to.
type IdentityPropagationConfig struct {
	Service           string
	AuthorizedTargets []string
}

// AssetBundleExportJob represents an asynchronous asset-bundle export job.
type AssetBundleExportJob struct {
	CreatedTime              time.Time
	JobID                    string
	Arn                      string
	Status                   string
	ExportFormat             string
	DownloadURL              string
	IncludeFolderMembers     string
	ResourceArns             []string
	IncludeAllDependencies   bool
	IncludeFolderMemberships bool
	IncludePermissions       bool
	IncludeTags              bool
}

// AssetBundleImportJob represents an asynchronous asset-bundle import job.
type AssetBundleImportJob struct {
	CreatedTime   time.Time
	JobID         string
	Arn           string
	Status        string
	FailureAction string
}

// DashboardSnapshotJob represents an asynchronous dashboard snapshot ("export to
// PDF/CSV/Excel") job.
type DashboardSnapshotJob struct {
	CreatedTime     time.Time
	LastUpdatedTime time.Time
	SnapshotConfig  map[string]any
	JobID           string
	Arn             string
	DashboardID     string
	Status          string
	S3URI           string
}

// QAResult represents a single Predict QA result: either a generated
// natural-language answer referencing a real Topic in the account, or an
// explicit "no answer" result when no topic matches the query text.
type QAResult struct {
	AnswerID     string
	AnswerStatus string
	QuestionID   string
	QuestionText string
	TopicID      string
	TopicName    string
	ResultType   string
}

// RefreshSchedule represents a QuickSight dataset SPICE refresh schedule.
type RefreshSchedule struct {
	StartAfterDateTime time.Time
	ScheduleFrequency  map[string]any
	ScheduleID         string
	Arn                string
	RefreshType        string
}

// DataSetRefreshProperties represents a QuickSight dataset's SPICE refresh
// configuration (incremental refresh window, failure notifications, etc.).
type DataSetRefreshProperties struct {
	RefreshConfiguration map[string]any
	FailureConfiguration map[string]any
}

// ActionConnector represents a QuickSight action connector: a configured
// integration (Salesforce, Jira, generic HTTP, etc.) that QuickSight agents
// and automations can invoke to perform actions against an external service.
type ActionConnector struct {
	CreatedTime          time.Time
	LastUpdatedTime      time.Time
	AuthenticationConfig map[string]any
	ActionConnectorID    string
	Arn                  string
	Name                 string
	Type                 string
	Description          string
	VPCConnectionArn     string
	Status               string
	Permissions          []ResourcePermission
}

// AutomationJob represents one run of a QuickSight automation (a
// console-authored workflow scoped to an automation group/automation).
type AutomationJob struct {
	CreatedAt         time.Time
	StartedAt         time.Time
	EndedAt           time.Time
	AutomationGroupID string
	AutomationID      string
	JobID             string
	Arn               string
	Status            string
	InputPayload      string
	OutputPayload     string
}

// FlowStepAlias represents one step-alias mapping in a flow's definition.
type FlowStepAlias struct {
	StepID    string
	StepAlias string
}

// Flow represents a QuickSight flow. FlowDefinition and StepAliases are only
// populated for DescribeFlow (the real API's FlowDetail shape); the summary
// operations (ListFlows/SearchFlows/GetFlowMetadata) leave them nil, mirroring
// how real AWS's FlowSummary carries no definition.
type Flow struct {
	CreatedTime     time.Time
	LastUpdatedTime time.Time
	LastPublishedAt time.Time
	FlowDefinition  map[string]any
	Description     string
	Arn             string
	Name            string
	FlowID          string
	CreatedBy       string
	LastPublishedBy string
	LastUpdatedBy   string
	PublishState    string
	Permissions     []ResourcePermission
	StepAliases     []FlowStepAlias
	RunCount        int32
	UserCount       int32
}

// Agent represents a QuickSight agent: an AI-powered conversational
// assistant scoped to a set of action connectors and spaces.
type Agent struct {
	CustomPrompt     *CustomPromptProfile
	CreatedAt        time.Time
	UpdatedAt        time.Time
	AgentID          string
	Arn              string
	Name             string
	Description      string
	IconID           string
	WelcomeMessage   string
	Creator          string
	AgentLifecycle   string
	AgentStatus      string
	ErrorMessage     string
	ActionConnectors []string
	Spaces           []string
	StarterPrompts   []string
	Permissions      []ResourcePermission
}

// CustomPromptProfile mirrors the real SDK's types.CustomPromptProfile: a
// caller-supplied reference to an already-provisioned Amazon Q Business
// custom-prompt profile (CreateAgentInput/UpdateAgentInput's
// CustomPromptInput.ExistingPrompt union member). All three IDs come from
// the caller, not from this backend, so storing and echoing them back on
// Describe/Create/Update is a genuine round-trip, not a fabrication -- unlike
// CustomPromptInput.NewPrompt, which asks AWS to mint a brand-new profile
// (and fresh IDs) server-side via a live Amazon Q Business subscription this
// backend has no state for; see agents.go's customPromptFromBody for how the
// two variants are told apart.
type CustomPromptProfile struct {
	ModelProfileID  string
	QbsAwsAccountID string
	SubscriptionID  string
}

// AssociationFailure represents one ARN that could not be attached to or
// detached from a resource, mirroring the real API's
// FailedToUpdateAssociation (UpdateAgent) / FailedSpaceResourceOperation
// (UpdateSpaceResources) shapes. ResourceType is empty for Agent
// associations (the real shape carries none) and set for Space resource
// operations.
type AssociationFailure struct {
	Arn          string
	ResourceType string
	ErrorCode    string
	ErrorMessage string
}

// AgentAssociationUpdate holds the per-ARN failures from one UpdateAgent
// call's attach/detach of action connectors and spaces.
type AgentAssociationUpdate struct {
	FailedToAddActionConnectors    []AssociationFailure
	FailedToAddSpaces              []AssociationFailure
	FailedToRemoveActionConnectors []AssociationFailure
	FailedToRemoveSpaces           []AssociationFailure
}

// KnowledgeBase represents a QuickSight knowledge base: an indexed corpus of
// documents from a data source that agents/flows can query for
// retrieval-augmented answers. Configuration/AccessControlConfiguration/
// MediaExtractionConfiguration are stored as opaque pass-through documents
// (like Dashboard.Definition elsewhere in this backend) since their real
// shapes are deeply nested and this backend has no processing logic that
// needs to interpret them.
type KnowledgeBase struct {
	CreatedAt                    time.Time
	UpdatedAt                    time.Time
	Configuration                map[string]any
	AccessControlConfiguration   map[string]any
	MediaExtractionConfiguration map[string]any
	KnowledgeBaseID              string
	Arn                          string
	Name                         string
	Description                  string
	DataSourceArn                string
	Status                       string
	PrimaryOwnerArn              string
	Permissions                  []ResourcePermission
	DocumentCount                int64
	SizeBytes                    int64
	EmailNotificationOptedIn     bool
}

// KnowledgeBaseDeleteResult identifies one knowledge base successfully
// deleted by BatchDeleteKnowledgeBase.
type KnowledgeBaseDeleteResult struct {
	KnowledgeBaseID string
	Arn             string
}

// KnowledgeBaseDeleteError identifies one knowledge base BatchDeleteKnowledgeBase
// failed to delete.
type KnowledgeBaseDeleteError struct {
	KnowledgeBaseID string
	ErrorCode       string
	ErrorMessage    string
}

// SpaceResource represents one QuickSight asset (identified by ARN and
// type) attached to a space.
type SpaceResource struct {
	UpdatedAt    time.Time
	ResourceArn  string
	ResourceType string
	ResourceName string
}

// UserIndexCapacity represents one user's derived index-capacity
// consumption: the number and size of the knowledge bases and spaces they
// own, computed from this backend's actual KnowledgeBase/Space state (see
// InMemoryBackend.ListUsersIndexCapacity) rather than fabricated.
type UserIndexCapacity struct {
	UserArn                 string
	UserName                string
	Email                   string
	Role                    string
	KBCount                 int32
	SpaceCount              int32
	TotalCapacityBytes      int64
	TotalKBCapacityBytes    int64
	TotalSpaceCapacityBytes int64
}

// UserIndexCapacityQuery bundles ListUsersIndexCapacity's optional filter
// and sort parameters (quicksight@v1.123.1
// api_op_ListUsersIndexCapacity.go's Filters/SortBy/SortOrder). At most one
// of MinCapacityBytes/MaxCapacityBytes and Prefix is populated per the
// real API's "only one filter is supported per request" -- both are
// carried so a caller sending more than one (which real AWS would reject,
// a validation concern this backend leaves unenforced like its sibling
// filters) still gets every constraint applied rather than only the last
// one parsed.
type UserIndexCapacityQuery struct {
	MinCapacityBytes *int64
	MaxCapacityBytes *int64
	Prefix           *string
	SortByCapacity   bool
	SortDescending   bool
}

// Space represents a QuickSight space: a named collection of resources
// (topics, dashboards, knowledge bases, action connectors, datasets)
// grouped together to scope agent/flow context.
type Space struct {
	CreatedAt    time.Time
	UpdatedAt    time.Time
	SpaceID      string
	Arn          string
	Name         string
	Description  string
	CreatedBy    string
	CreatedByArn string
	Resources    []SpaceResource
	Permissions  []ResourcePermission
}

// SelfUpgradeRequestDetail represents one namespace self-upgrade request (a
// user's request to be upgraded to a different UserRole).
type SelfUpgradeRequestDetail struct {
	LastUpdateFailureReason string
	OriginalRole            string
	RequestNote             string
	RequestStatus           string
	RequestedRole           string
	UpgradeRequestID        string
	UserName                string
	CreationTime            int64
	LastUpdateAttemptTime   int64
}

var _ StorageBackend = (*InMemoryBackend)(nil)

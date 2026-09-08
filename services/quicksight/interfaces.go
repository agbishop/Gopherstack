package quicksight

import "time"

type StorageBackend interface {
	// Namespaces
	CreateNamespace(accountID, namespace, capacityRegion string, tags map[string]string) (*Namespace, error)
	DescribeNamespace(accountID, namespace string) (*Namespace, error)
	DeleteNamespace(accountID, namespace string) error
	ListNamespaces(accountID string, maxResults int32, nextToken string) ([]*Namespace, string, error)

	// Groups
	CreateGroup(accountID, namespace, groupName, description string) (*Group, error)
	DescribeGroup(accountID, namespace, groupName string) (*Group, error)
	UpdateGroup(accountID, namespace, groupName, description string) (*Group, error)
	DeleteGroup(accountID, namespace, groupName string) error
	ListGroups(accountID, namespace string, maxResults int32, nextToken string) ([]*Group, string, error)
	SearchGroups(
		accountID, namespace string,
		filters []SearchFilter,
		maxResults int32,
		nextToken string,
	) ([]*Group, string, error)

	// Group Memberships
	CreateGroupMembership(accountID, namespace, groupName, memberName string) (*GroupMember, error)
	DescribeGroupMembership(accountID, namespace, groupName, memberName string) (*GroupMember, error)
	DeleteGroupMembership(accountID, namespace, groupName, memberName string) error
	ListGroupMemberships(
		accountID, namespace, groupName string,
		maxResults int32,
		nextToken string,
	) ([]*GroupMember, string, error)

	// Users
	RegisterUser(
		accountID, namespace, userName, email, role, identityType, sessionName string,
		tags map[string]string,
	) (*User, error)
	DescribeUser(accountID, namespace, userName string) (*User, error)
	UpdateUser(accountID, namespace, userName, email, role string) (*User, error)
	DeleteUser(accountID, namespace, userName string) error
	DeleteUserByPrincipalID(accountID, namespace, principalID string) error
	ListUsers(accountID, namespace string, maxResults int32, nextToken string) ([]*User, string, error)
	ListUserGroups(accountID, namespace, userName string, maxResults int32, nextToken string) ([]*Group, string, error)

	// DataSources
	CreateDataSource(
		accountID, dataSourceID, name, dsType string,
		permissions []ResourcePermission,
		tags map[string]string,
	) (*DataSource, error)
	DescribeDataSource(accountID, dataSourceID string) (*DataSource, error)
	UpdateDataSource(accountID, dataSourceID, name string) (*DataSource, error)
	DeleteDataSource(accountID, dataSourceID string) error
	ListDataSources(accountID string, maxResults int32, nextToken string) ([]*DataSource, string, error)
	SearchDataSources(
		accountID string,
		filters []SearchFilter,
		maxResults int32,
		nextToken string,
	) ([]*DataSource, string, error)
	DescribeDataSourcePermissions(accountID, dataSourceID string) (*DataSource, []ResourcePermission, error)
	UpdateDataSourcePermissions(
		accountID, dataSourceID string,
		grant, revoke []ResourcePermission,
	) (*DataSource, []ResourcePermission, error)

	// DataSets
	// CreateDataSet returns the created dataset plus the *Ingestion triggered
	// as a side effect when importMode is SPICE (nil for DIRECT_QUERY, which
	// triggers no ingestion). physicalTableMap is required by AWS (rejects a
	// dataset with no physical tables); logicalTableMap is optional.
	CreateDataSet(
		accountID, dataSetID, name, importMode string,
		permissions []ResourcePermission,
		tags map[string]string,
		physicalTableMap map[string]PhysicalTable,
		logicalTableMap map[string]LogicalTable,
	) (*DataSet, *Ingestion, error)
	DescribeDataSet(accountID, dataSetID string) (*DataSet, error)
	// UpdateDataSet returns the updated dataset plus the *Ingestion triggered
	// as a side effect when the resulting importMode is SPICE (nil for
	// DIRECT_QUERY), mirroring CreateDataSet's conditional-ingestion contract.
	// physicalTableMap is required by AWS on every UpdateDataSet call (this
	// operation doesn't support partial updates); logicalTableMap is optional.
	UpdateDataSet(
		accountID, dataSetID, name, importMode string,
		physicalTableMap map[string]PhysicalTable,
		logicalTableMap map[string]LogicalTable,
	) (*DataSet, *Ingestion, error)
	DeleteDataSet(accountID, dataSetID string) error
	ListDataSets(accountID string, maxResults int32, nextToken string) ([]*DataSet, string, error)
	SearchDataSets(
		accountID string,
		filters []SearchFilter,
		maxResults int32,
		nextToken string,
	) ([]*DataSet, string, error)
	DescribeDataSetPermissions(accountID, dataSetID string) (*DataSet, []ResourcePermission, error)
	UpdateDataSetPermissions(
		accountID, dataSetID string,
		grant, revoke []ResourcePermission,
	) (*DataSet, []ResourcePermission, error)

	// Ingestions
	CreateIngestion(accountID, dataSetID, ingestionID string) (*Ingestion, error)
	DescribeIngestion(accountID, dataSetID, ingestionID string) (*Ingestion, error)
	CancelIngestion(accountID, dataSetID, ingestionID string) error
	ListIngestions(accountID, dataSetID string, maxResults int32, nextToken string) ([]*Ingestion, string, error)

	// Dashboards
	CreateDashboard(
		accountID, dashboardID, name, themeArn, versionDescription string,
		definition map[string]any,
		permissions []ResourcePermission,
		tags map[string]string,
	) (*Dashboard, error)
	DescribeDashboard(accountID, dashboardID string) (*Dashboard, error)
	UpdateDashboard(
		accountID, dashboardID, name, themeArn, versionDescription string,
		definition map[string]any,
	) (*Dashboard, error)
	DeleteDashboard(accountID, dashboardID string, versionNumber int64) error
	ListDashboards(accountID string, maxResults int32, nextToken string) ([]*Dashboard, string, error)
	ListDashboardVersions(
		accountID, dashboardID string,
		maxResults int32,
		nextToken string,
	) ([]*DashboardVersion, string, error)
	SearchDashboards(
		accountID string,
		filters []SearchFilter,
		maxResults int32,
		nextToken string,
	) ([]*Dashboard, string, error)
	UpdateDashboardPublishedVersion(accountID, dashboardID string, versionNumber int64) (*Dashboard, error)
	UpdateDashboardLinks(accountID, dashboardID string, linkEntities []string) (*Dashboard, error)
	DescribeDashboardPermissions(accountID, dashboardID string) (*Dashboard, []ResourcePermission, error)
	UpdateDashboardPermissions(
		accountID, dashboardID string,
		grant, revoke []ResourcePermission,
	) (*Dashboard, []ResourcePermission, error)

	// Analyses
	CreateAnalysis(
		accountID, analysisID, name, themeArn string,
		definition map[string]any,
		permissions []ResourcePermission,
		tags map[string]string,
	) (*Analysis, error)
	DescribeAnalysis(accountID, analysisID string) (*Analysis, error)
	UpdateAnalysis(accountID, analysisID, name, themeArn string, definition map[string]any) (*Analysis, error)
	DeleteAnalysis(accountID, analysisID string, forceDeleteWithoutRecovery bool) error
	ListAnalyses(accountID string, maxResults int32, nextToken string) ([]*Analysis, string, error)
	RestoreAnalysis(accountID, analysisID string) (*Analysis, error)
	SearchAnalyses(
		accountID string,
		filters []SearchFilter,
		maxResults int32,
		nextToken string,
	) ([]*Analysis, string, error)
	DescribeAnalysisPermissions(accountID, analysisID string) (*Analysis, []ResourcePermission, error)
	UpdateAnalysisPermissions(
		accountID, analysisID string,
		grant, revoke []ResourcePermission,
	) (*Analysis, []ResourcePermission, error)

	// Tags
	TagResource(resourceARN string, tags map[string]string) error
	UntagResource(resourceARN string, tagKeys []string) error
	ListTagsForResource(resourceARN string) (map[string]string, error)

	// Folders
	CreateFolder(
		accountID, folderID, name, folderType, parentFolderArn, sharingModel string,
		permissions []ResourcePermission,
		tags map[string]string,
	) (*Folder, error)
	DescribeFolder(accountID, folderID string) (*Folder, error)
	UpdateFolder(accountID, folderID, name string) (*Folder, error)
	DeleteFolder(accountID, folderID string) error
	ListFolders(accountID string, maxResults int32, nextToken string) ([]*Folder, string, error)
	SearchFolders(
		accountID string,
		filters []FolderSearchFilter,
		maxResults int32,
		nextToken string,
	) ([]*Folder, string, error)

	// Folder memberships
	CreateFolderMembership(accountID, folderID, memberID, memberType string) (*FolderMember, error)
	DeleteFolderMembership(accountID, folderID, memberID, memberType string) error
	ListFolderMembers(
		accountID, folderID string,
		maxResults int32,
		nextToken string,
	) ([]*FolderMember, string, error)

	// Folder permissions
	DescribeFolderPermissions(accountID, folderID string) ([]ResourcePermission, error)
	UpdateFolderPermissions(
		accountID, folderID string,
		grant, revoke []ResourcePermission,
	) ([]ResourcePermission, error)
	DescribeFolderResolvedPermissions(accountID, folderID string) ([]ResourcePermission, error)

	// Folders-for-resource
	ListFoldersForResource(
		accountID, resourceArn string,
		maxResults int32,
		nextToken string,
	) ([]string, string, error)

	// Templates
	CreateTemplate(
		accountID, templateID, name, sourceEntityArn, versionDescription string,
		definition map[string]any,
		permissions []ResourcePermission,
		tags map[string]string,
	) (*Template, error)
	DescribeTemplate(accountID, templateID string, versionNumber int64) (*Template, error)
	UpdateTemplate(
		accountID, templateID, name, sourceEntityArn, versionDescription string,
		definition map[string]any,
	) (*Template, error)
	DeleteTemplate(accountID, templateID string, versionNumber int64) error
	ListTemplates(accountID string, maxResults int32, nextToken string) ([]*Template, string, error)
	ListTemplateVersions(
		accountID, templateID string,
		maxResults int32,
		nextToken string,
	) ([]*TemplateVersion, string, error)
	DescribeTemplatePermissions(accountID, templateID string) (*Template, []ResourcePermission, error)
	UpdateTemplatePermissions(
		accountID, templateID string,
		grant, revoke []ResourcePermission,
	) (*Template, []ResourcePermission, error)

	// Template aliases
	CreateTemplateAlias(accountID, templateID, aliasName string, versionNumber int64) (*TemplateAlias, error)
	DescribeTemplateAlias(accountID, templateID, aliasName string) (*TemplateAlias, error)
	UpdateTemplateAlias(accountID, templateID, aliasName string, versionNumber int64) (*TemplateAlias, error)
	DeleteTemplateAlias(accountID, templateID, aliasName string) error
	ListTemplateAliases(
		accountID, templateID string,
		maxResults int32,
		nextToken string,
	) ([]*TemplateAlias, string, error)

	// Themes
	CreateTheme(
		accountID, themeID, name, baseThemeID, versionDescription string,
		configuration map[string]any,
		permissions []ResourcePermission,
		tags map[string]string,
	) (*Theme, error)
	DescribeTheme(accountID, themeID string, versionNumber int64) (*Theme, error)
	UpdateTheme(
		accountID, themeID, name, baseThemeID, versionDescription string,
		configuration map[string]any,
	) (*Theme, error)
	DeleteTheme(accountID, themeID string, versionNumber int64) error
	ListThemes(accountID, themeType string, maxResults int32, nextToken string) ([]*Theme, string, error)
	ListThemeVersions(
		accountID, themeID string,
		maxResults int32,
		nextToken string,
	) ([]*ThemeVersion, string, error)
	DescribeThemePermissions(accountID, themeID string) (*Theme, []ResourcePermission, error)
	UpdateThemePermissions(
		accountID, themeID string,
		grant, revoke []ResourcePermission,
	) (*Theme, []ResourcePermission, error)

	// Theme aliases
	CreateThemeAlias(accountID, themeID, aliasName string, versionNumber int64) (*ThemeAlias, error)
	DescribeThemeAlias(accountID, themeID, aliasName string) (*ThemeAlias, error)
	UpdateThemeAlias(accountID, themeID, aliasName string, versionNumber int64) (*ThemeAlias, error)
	DeleteThemeAlias(accountID, themeID, aliasName string) error
	ListThemeAliases(
		accountID, themeID string,
		maxResults int32,
		nextToken string,
	) ([]*ThemeAlias, string, error)

	// Topics
	CreateTopic(
		accountID, topicID, name, description, userExperienceVersion string,
		dataSets []map[string]any,
		permissions []ResourcePermission,
		tags map[string]string,
	) (*Topic, error)
	DescribeTopic(accountID, topicID string) (*Topic, error)
	UpdateTopic(
		accountID, topicID, name, description, userExperienceVersion string,
		dataSets []map[string]any,
	) (*Topic, error)
	DeleteTopic(accountID, topicID string) error
	ListTopics(accountID string, maxResults int32, nextToken string) ([]*Topic, string, error)
	SearchTopics(
		accountID string,
		filters []SearchFilter,
		maxResults int32,
		nextToken string,
	) ([]*Topic, string, error)

	// Topic permissions
	DescribeTopicPermissions(accountID, topicID string) (*Topic, []ResourcePermission, error)
	UpdateTopicPermissions(
		accountID, topicID string,
		grant, revoke []ResourcePermission,
	) (*Topic, []ResourcePermission, error)

	// Topic refresh
	DescribeTopicRefresh(accountID, topicID, refreshID string) (*TopicRefreshDetails, error)

	// Topic refresh schedules
	CreateTopicRefreshSchedule(
		accountID, topicID, datasetID, datasetArn, refreshType string,
		isEnabled bool,
		scheduleConfig map[string]any,
	) (*TopicRefreshSchedule, error)
	DescribeTopicRefreshSchedule(accountID, topicID, datasetID string) (*TopicRefreshSchedule, error)
	UpdateTopicRefreshSchedule(
		accountID, topicID, datasetID, refreshType string,
		isEnabled *bool,
		scheduleConfig map[string]any,
	) (*TopicRefreshSchedule, error)
	DeleteTopicRefreshSchedule(accountID, topicID, datasetID string) (*TopicRefreshSchedule, error)
	ListTopicRefreshSchedules(accountID, topicID string) ([]*TopicRefreshSchedule, error)

	// Topic reviewed answers
	BatchCreateTopicReviewedAnswer(
		accountID, topicID string,
		answers []map[string]any,
	) ([]*TopicReviewedAnswer, []TopicAnswerError, error)
	BatchDeleteTopicReviewedAnswer(
		accountID, topicID string,
		answerIDs []string,
	) ([]string, []TopicAnswerError, error)
	ListTopicReviewedAnswers(accountID, topicID string) ([]*TopicReviewedAnswer, error)

	// Topics V2 (Q topics); Describe/Delete/List/Search/permissions reuse the
	// V1 methods above directly (handler_topics_v2.go) -- see topics_v2.go.
	CreateTopicV2(
		accountID, topicID, name, description, customInstructions string,
		dataSets []map[string]any,
		dataSetRelations []map[string]any,
		tags map[string]string,
	) (*Topic, error)
	UpdateTopicV2(
		accountID, topicID, name, description, customInstructions, publishOption string,
		dataSets []map[string]any,
		dataSetRelations []map[string]any,
	) (*Topic, error)

	// VPC connections
	CreateVPCConnection(
		accountID, vpcConnectionID, name, vpcID string,
		subnetIDs, securityGroupIDs, dnsResolvers []string,
		roleArn string,
		tags map[string]string,
	) (*VPCConnection, error)
	DescribeVPCConnection(accountID, vpcConnectionID string) (*VPCConnection, error)
	UpdateVPCConnection(
		accountID, vpcConnectionID, name string,
		subnetIDs, securityGroupIDs, dnsResolvers []string,
		roleArn string,
	) (*VPCConnection, error)
	DeleteVPCConnection(accountID, vpcConnectionID string) (*VPCConnection, error)
	ListVPCConnections(accountID string, maxResults int32, nextToken string) ([]*VPCConnection, string, error)

	// IAM policy assignments
	CreateIAMPolicyAssignment(
		accountID, namespace, assignmentName, assignmentStatus, policyArn string,
		identities map[string][]string,
	) (*IAMPolicyAssignment, error)
	DescribeIAMPolicyAssignment(accountID, namespace, assignmentName string) (*IAMPolicyAssignment, error)
	UpdateIAMPolicyAssignment(
		accountID, namespace, assignmentName, assignmentStatus, policyArn string,
		identities map[string][]string,
	) (*IAMPolicyAssignment, error)
	DeleteIAMPolicyAssignment(accountID, namespace, assignmentName string) error
	ListIAMPolicyAssignments(
		accountID, namespace, statusFilter string,
		maxResults int32,
		nextToken string,
	) ([]*IAMPolicyAssignment, string, error)
	ListIAMPolicyAssignmentsForUser(
		accountID, namespace, userName string,
		maxResults int32,
		nextToken string,
	) ([]*IAMPolicyAssignment, string, error)

	// Account settings
	DescribeAccountSettings(accountID string) (*AccountSettings, error)
	UpdateAccountSettings(
		accountID, defaultNamespace, notificationEmail string,
		terminationProtectionEnabled *bool,
	) (*AccountSettings, error)

	// Account subscription
	CreateAccountSubscription(
		accountID, accountName, edition, authenticationMethod, notificationEmail string,
	) (*AccountSubscription, error)
	DescribeAccountSubscription(accountID string) (*AccountSubscription, error)
	DeleteAccountSubscription(accountID string) error

	// Account customization
	CreateAccountCustomization(
		accountID, namespace, defaultTheme, defaultEmailCustomizationTemplate string,
	) (*AccountCustomization, error)
	DescribeAccountCustomization(accountID, namespace string, resolved bool) (*AccountCustomization, error)
	UpdateAccountCustomization(
		accountID, namespace, defaultTheme, defaultEmailCustomizationTemplate string,
	) (*AccountCustomization, error)
	DeleteAccountCustomization(accountID, namespace string) error

	// Account custom permission
	DescribeAccountCustomPermission(accountID string) (string, error)
	UpdateAccountCustomPermission(accountID, customPermissionsName string) error
	DeleteAccountCustomPermission(accountID string) error

	// IP restriction
	DescribeIPRestriction(accountID string) (*IPRestriction, error)
	UpdateIPRestriction(
		accountID string,
		ruleMap, vpcIDRuleMap, vpcEndpointIDRuleMap map[string]string,
		enabled *bool,
	) (*IPRestriction, error)

	// Public sharing
	UpdatePublicSharingSettings(accountID string, enabled bool) error

	// Key registration
	DescribeKeyRegistration(accountID string, defaultKeyOnly bool) ([]RegisteredCustomerManagedKey, error)
	UpdateKeyRegistration(
		accountID string,
		keys []RegisteredCustomerManagedKey,
	) ([]RegisteredCustomerManagedKey, error)

	// Default Q Business Application
	DescribeDefaultQBusinessApplication(accountID, namespace string) (*DefaultQBusinessApplication, error)
	UpdateDefaultQBusinessApplication(
		accountID, applicationID, namespace string,
	) (*DefaultQBusinessApplication, error)
	DeleteDefaultQBusinessApplication(accountID, namespace string) error

	// Q personalization
	DescribeQPersonalizationConfiguration(accountID string) (string, error)
	UpdateQPersonalizationConfiguration(accountID, mode string) (string, error)

	// Q Search configuration
	DescribeQuickSightQSearchConfiguration(accountID string) (string, error)
	UpdateQuickSightQSearchConfiguration(accountID, status string) (string, error)

	// Dashboards Q&A configuration
	DescribeDashboardsQAConfiguration(accountID string) (string, error)
	UpdateDashboardsQAConfiguration(accountID, status string) (string, error)

	// Brands
	CreateBrand(accountID, brandID string, definition map[string]any, tags map[string]string) (*Brand, error)
	DescribeBrand(accountID, brandID, versionID string) (*Brand, error)
	UpdateBrand(accountID, brandID string, definition map[string]any) (*Brand, error)
	DeleteBrand(accountID, brandID string) error
	ListBrands(accountID string, maxResults int32, nextToken string) ([]*Brand, string, error)
	DescribeBrandPublishedVersion(accountID, brandID string) (*Brand, error)
	UpdateBrandPublishedVersion(accountID, brandID, versionID string) error
	DescribeBrandAssignment(accountID string) (string, error)
	UpdateBrandAssignment(accountID, brandArn string) (string, error)
	DeleteBrandAssignment(accountID string) error

	// Custom permissions
	CreateCustomPermissions(
		accountID, name string,
		capabilities map[string]any,
		tags map[string]string,
	) (*CustomPermissions, error)
	DescribeCustomPermissions(accountID, name string) (*CustomPermissions, error)
	UpdateCustomPermissions(accountID, name string, capabilities map[string]any) (*CustomPermissions, error)
	DeleteCustomPermissions(accountID, name string) (*CustomPermissions, error)
	ListCustomPermissions(accountID string, maxResults int32, nextToken string) ([]*CustomPermissions, string, error)

	// Role custom permission
	UpdateRoleCustomPermission(accountID, namespace, role, customPermissionsName string) error
	DescribeRoleCustomPermission(accountID, namespace, role string) (string, error)
	DeleteRoleCustomPermission(accountID, namespace, role string) error

	// Role memberships
	CreateRoleMembership(accountID, namespace, role, memberName string) error
	DeleteRoleMembership(accountID, namespace, role, memberName string) error
	ListRoleMemberships(
		accountID, namespace, role string,
		maxResults int32,
		nextToken string,
	) ([]string, string, error)

	// User custom permission
	UpdateUserCustomPermission(accountID, namespace, userName, customPermissionsName string) error
	DeleteUserCustomPermission(accountID, namespace, userName string) error

	// OAuth client applications
	CreateOAuthClientApplication(
		accountID, clientID, name string,
		fields map[string]any,
		tags map[string]string,
	) (*OAuthClientApplication, error)
	DescribeOAuthClientApplication(accountID, clientID string) (*OAuthClientApplication, error)
	UpdateOAuthClientApplication(
		accountID, clientID, name string,
		fields map[string]any,
	) (*OAuthClientApplication, error)
	DeleteOAuthClientApplication(accountID, clientID string) (*OAuthClientApplication, error)
	ListOAuthClientApplications(
		accountID string,
		maxResults int32,
		nextToken string,
	) ([]*OAuthClientApplication, string, error)

	// Identity propagation configuration
	UpdateIdentityPropagationConfig(accountID, service string, authorizedTargets []string) error
	DeleteIdentityPropagationConfig(accountID, service string) error
	ListIdentityPropagationConfigs(accountID string) ([]*IdentityPropagationConfig, error)

	// Asset bundle export jobs
	StartAssetBundleExportJob(
		accountID, jobID, exportFormat, includeFolderMembers string,
		resourceArns []string,
		includeAllDependencies, includeFolderMemberships, includePermissions, includeTags bool,
	) (*AssetBundleExportJob, error)
	DescribeAssetBundleExportJob(accountID, jobID string) (*AssetBundleExportJob, error)
	ListAssetBundleExportJobs(
		accountID string,
		maxResults int32,
		nextToken string,
	) ([]*AssetBundleExportJob, string, error)

	// Asset bundle import jobs
	StartAssetBundleImportJob(accountID, jobID, failureAction string) (*AssetBundleImportJob, error)
	DescribeAssetBundleImportJob(accountID, jobID string) (*AssetBundleImportJob, error)
	ListAssetBundleImportJobs(
		accountID string,
		maxResults int32,
		nextToken string,
	) ([]*AssetBundleImportJob, string, error)

	// Dashboard snapshot jobs
	StartDashboardSnapshotJob(
		accountID, dashboardID, jobID string,
		snapshotConfiguration map[string]any,
	) (*DashboardSnapshotJob, error)
	DescribeDashboardSnapshotJob(accountID, dashboardID, jobID string) (*DashboardSnapshotJob, error)
	DescribeDashboardSnapshotJobResult(accountID, dashboardID, jobID string) (*DashboardSnapshotJob, error)
	StartDashboardSnapshotJobSchedule(accountID, dashboardID, scheduleID string) error

	// Predict QA results
	PredictQAResults(accountID, queryText string) (*QAResult, error)

	// DataSet refresh schedules
	CreateRefreshSchedule(
		accountID, datasetID, scheduleID, refreshType string,
		startAfterDateTime time.Time,
		scheduleFrequency map[string]any,
	) (*RefreshSchedule, error)
	DescribeRefreshSchedule(accountID, datasetID, scheduleID string) (*RefreshSchedule, error)
	UpdateRefreshSchedule(
		accountID, datasetID, scheduleID, refreshType string,
		startAfterDateTime time.Time,
		scheduleFrequency map[string]any,
	) (*RefreshSchedule, error)
	DeleteRefreshSchedule(accountID, datasetID, scheduleID string) (*RefreshSchedule, error)
	ListRefreshSchedules(accountID, datasetID string) ([]*RefreshSchedule, error)

	// DataSet refresh properties
	PutDataSetRefreshProperties(
		accountID, datasetID string,
		refreshConfiguration, failureConfiguration map[string]any,
	) (*DataSetRefreshProperties, error)
	DescribeDataSetRefreshProperties(accountID, datasetID string) (*DataSetRefreshProperties, error)
	DeleteDataSetRefreshProperties(accountID, datasetID string) error

	// Embed URLs
	GenerateEmbedURLForAnonymousUser(
		accountID, namespace string,
		authorizedResourceArns []string,
		experienceConfiguration map[string]any,
	) (embedURL, anonymousUserArn string, err error)
	GenerateEmbedURLForRegisteredUser(
		accountID, userArn string,
		experienceConfiguration map[string]any,
	) (string, error)
	GenerateEmbedURLForRegisteredUserWithIdentity(
		accountID string,
		experienceConfiguration map[string]any,
	) (string, error)
	GetDashboardEmbedURL(accountID, dashboardID, identityType string) (string, error)
	GetSessionEmbedURL(accountID, entryPoint string) (string, error)

	// Identity context
	GenerateIdentityContext(
		accountID, namespace, userIdentifierKind, userIdentifierValue, contextRegion string,
	) (string, error)

	// Action connectors
	CreateActionConnector(
		accountID, actionConnectorID, name, connectorType, description, vpcConnectionArn string,
		authenticationConfig map[string]any,
		permissions []ResourcePermission,
		tags map[string]string,
	) (*ActionConnector, error)
	DescribeActionConnector(accountID, actionConnectorID string) (*ActionConnector, error)
	UpdateActionConnector(
		accountID, actionConnectorID, name, description, vpcConnectionArn string,
		authenticationConfig map[string]any,
	) (*ActionConnector, error)
	DeleteActionConnector(accountID, actionConnectorID string) (*ActionConnector, error)
	ListActionConnectors(accountID string, maxResults int32, nextToken string) ([]*ActionConnector, string, error)
	SearchActionConnectors(
		accountID string,
		filters []SearchFilter,
		maxResults int32,
		nextToken string,
	) ([]*ActionConnector, string, error)
	DescribeActionConnectorPermissions(
		accountID, actionConnectorID string,
	) (*ActionConnector, []ResourcePermission, error)
	UpdateActionConnectorPermissions(
		accountID, actionConnectorID string,
		grant, revoke []ResourcePermission,
	) (*ActionConnector, []ResourcePermission, error)

	// Automation jobs
	StartAutomationJob(accountID, automationGroupID, automationID, inputPayload string) (*AutomationJob, error)
	DescribeAutomationJob(accountID, automationGroupID, automationID, jobID string) (*AutomationJob, error)

	// SPICE capacity configuration
	UpdateSPICECapacityConfiguration(accountID, purchaseMode string) error

	// Flows
	ListFlows(accountID string, maxResults int32, nextToken string) ([]*Flow, string, error)
	SearchFlows(
		accountID string,
		filters []SearchFilter,
		maxResults int32,
		nextToken string,
	) ([]*Flow, string, error)
	GetFlowMetadata(accountID, flowID string) (*Flow, error)
	GetFlowPermissions(accountID, flowID string) (*Flow, []ResourcePermission, error)
	UpdateFlowPermissions(
		accountID, flowID string,
		grant, revoke []ResourcePermission,
	) (*Flow, []ResourcePermission, error)
	CreateFlow(
		accountID, name, description string,
		flowDefinition map[string]any,
		permissions []ResourcePermission,
	) (*Flow, error)
	DescribeFlow(accountID, flowID string) (*Flow, error)
	UpdateFlow(accountID, flowID, name, description string, flowDefinition map[string]any) (*Flow, error)
	DeleteFlow(accountID, flowID string) error

	// Agents
	CreateAgent(
		accountID, agentID, name, description, iconID, welcomeMessage, agentLifecycle string,
		actionConnectors, spaces, starterPrompts []string,
		permissions []ResourcePermission,
		tags map[string]string,
		customPrompt *CustomPromptProfile,
	) (*Agent, error)
	DescribeAgent(accountID, agentID string) (*Agent, error)
	UpdateAgent(
		accountID, agentID, name, description, iconID, welcomeMessage string,
		actionConnectorsToAdd, actionConnectorsToRemove, spacesToAdd, spacesToRemove, starterPrompts []string,
		customPrompt *CustomPromptProfile,
	) (*Agent, *AgentAssociationUpdate, error)
	DeleteAgent(accountID, agentID string) error
	ListAgents(accountID string, maxResults int32, nextToken string) ([]*Agent, string, error)
	SearchAgents(
		accountID string,
		filters []SearchFilter,
		maxResults int32,
		nextToken string,
	) ([]*Agent, string, error)
	DescribeAgentPermissions(accountID, agentID string) (*Agent, []ResourcePermission, error)
	UpdateAgentPermissions(
		accountID, agentID string,
		grant, revoke []ResourcePermission,
	) (*Agent, []ResourcePermission, error)

	// Knowledge bases
	CreateKnowledgeBase(
		accountID, knowledgeBaseID, name, description, dataSourceArn, primaryOwnerArn string,
		configuration, accessControlConfiguration, mediaExtractionConfiguration map[string]any,
		permissions []ResourcePermission,
		tags map[string]string,
	) (*KnowledgeBase, error)
	DescribeKnowledgeBase(accountID, knowledgeBaseID string) (*KnowledgeBase, error)
	UpdateKnowledgeBase(
		accountID, knowledgeBaseID, name, description string,
		emailNotificationOptedIn *bool,
		configuration, accessControlConfiguration, mediaExtractionConfiguration map[string]any,
	) (*KnowledgeBase, error)
	DeleteKnowledgeBase(accountID, knowledgeBaseID string) (*KnowledgeBase, error)
	BatchDeleteKnowledgeBase(
		accountID string,
		knowledgeBaseIDs []string,
	) ([]KnowledgeBaseDeleteResult, []KnowledgeBaseDeleteError)
	ListKnowledgeBases(accountID string, maxResults int32, nextToken string) ([]*KnowledgeBase, string, error)
	SearchKnowledgeBases(
		accountID string,
		filters []SearchFilter,
		maxResults int32,
		nextToken string,
	) ([]*KnowledgeBase, string, error)
	DescribeKnowledgeBasePermissions(accountID, knowledgeBaseID string) (*KnowledgeBase, []ResourcePermission, error)
	UpdateKnowledgeBasePermissions(
		accountID, knowledgeBaseID string,
		grant, revoke []ResourcePermission,
	) (*KnowledgeBase, []ResourcePermission, error)

	// Spaces
	CreateSpace(accountID, spaceID, name, description string) (*Space, error)
	DescribeSpace(accountID, spaceID string) (*Space, error)
	UpdateSpace(accountID, spaceID, name, description string) (*Space, error)
	DeleteSpace(accountID, spaceID string) (*Space, error)
	ListSpaces(accountID string, maxResults int32, nextToken string) ([]*Space, string, error)
	SearchSpaces(
		accountID string,
		filters []SearchFilter,
		maxResults int32,
		nextToken string,
	) ([]*Space, string, error)
	DescribeSpacePermissions(accountID, spaceID string) (*Space, []ResourcePermission, error)
	UpdateSpacePermissions(
		accountID, spaceID string,
		grant, revoke []ResourcePermission,
	) (*Space, []ResourcePermission, error)
	ListSpaceResources(accountID, spaceID string) ([]SpaceResource, error)
	UpdateSpaceResources(
		accountID, spaceID string,
		add, remove []SpaceResource,
	) (*Space, []AssociationFailure, error)

	// User index capacity
	ListUsersIndexCapacity(
		accountID, namespace string,
		query UserIndexCapacityQuery,
		maxResults int32,
		nextToken string,
	) ([]UserIndexCapacity, string, error)

	// Namespace self-upgrade
	DescribeSelfUpgradeConfiguration(accountID, namespace string) (string, error)
	UpdateSelfUpgradeConfiguration(accountID, namespace, status string) (string, error)
	ListSelfUpgrades(
		accountID, namespace string,
		maxResults int32,
		nextToken string,
	) ([]*SelfUpgradeRequestDetail, string, error)
	UpdateSelfUpgrade(accountID, namespace, action, upgradeRequestID string) (*SelfUpgradeRequestDetail, error)

	AccountID() string
	Region() string
	Reset()
}

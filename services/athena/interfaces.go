package athena

import (
	"context"

	sdk_s3 "github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Storer is the subset of S3 operations Athena needs to write a succeeded
// query execution's result object to ResultConfiguration.OutputLocation,
// wired via SetS3Backend. When unset, results are stored/echoed but no S3
// object is written.
type S3Storer interface {
	PutObject(ctx context.Context, input *sdk_s3.PutObjectInput) (*sdk_s3.PutObjectOutput, error)
}

// GlueDatabase is the subset of a Glue database's fields Athena needs to
// present a GLUE-type catalog's database, wired via SetGlueMetadataSource.
type GlueDatabase struct {
	Parameters  map[string]string
	Name        string
	Description string
}

// GlueTable is the subset of a Glue table's fields Athena needs to present a
// GLUE-type catalog's table, wired via SetGlueMetadataSource.
type GlueTable struct {
	Name          string
	Parameters    map[string]string
	Columns       []Column
	PartitionKeys []Column
	CreateTime    float64
}

// GlueMetadataSource is the subset of Glue operations that Athena needs to
// resolve a GLUE-type DataCatalog's databases and tables against real Glue
// state, wired via SetGlueMetadataSource. When unset, GLUE-type catalogs
// fall back to Athena's own internally simulated database/table store.
type GlueMetadataSource interface {
	GetDatabase(name string) (*GlueDatabase, error)
	GetDatabases() []*GlueDatabase
	GetTable(dbName, tableName string) (*GlueTable, error)
	GetTables(dbName string) ([]*GlueTable, error)
}

// StorageBackend is the interface for the Athena in-memory store.
type StorageBackend interface {
	// WorkGroups
	CreateWorkGroup(
		name, description, state string,
		cfg WorkGroupConfiguration,
		tags map[string]string,
	) error
	GetWorkGroup(name string) (*WorkGroup, error)
	ListWorkGroups(nextToken string, maxResults int) ([]*WorkGroupSummary, string, error)
	UpdateWorkGroup(name, description, state string, cfg *WorkGroupConfigurationUpdates) error
	DeleteWorkGroup(name string, recursiveDelete bool) error

	// Named Queries
	CreateNamedQuery(name, description, database, queryString, workGroup string) (string, error)
	GetNamedQuery(id string) (*NamedQuery, error)
	ListNamedQueries(workGroup, nextToken string, maxResults int) ([]string, string, error)
	BatchGetNamedQuery(ids []string) ([]NamedQuery, []UnprocessedNamedQueryID)
	DeleteNamedQuery(id string) error

	// Data Catalogs
	CreateDataCatalog(
		name, catalogType, description string,
		params, tags map[string]string,
	) (*DataCatalog, error)
	GetDataCatalog(name string) (*DataCatalog, error)
	ListDataCatalogs(nextToken string, maxResults int) ([]*DataCatalogSummary, string, error)
	UpdateDataCatalog(
		name, catalogType, description string,
		params map[string]string,
	) error
	DeleteDataCatalog(name string, deleteCatalogOnly bool) (*DataCatalog, error)

	// Query Executions
	StartQueryExecution(
		query, workGroup string,
		ctx QueryExecutionContext,
		rc ResultConfiguration,
		execParams []string,
		reuseCfg *ResultReuseConfiguration,
	) (string, error)
	GetQueryExecution(id string) (*QueryExecution, error)
	GetQueryResults(id, nextToken string, maxResults int) (*sqlResultPage, error)
	ListQueryExecutions(workGroup string) ([]string, error)
	StopQueryExecution(id string) error
	BatchGetQueryExecution(ids []string) ([]QueryExecution, []UnprocessedQueryExecutionID)

	// Tags
	TagResource(arn string, tags map[string]string) error
	UntagResource(arn string, keys []string) error
	ListTagsForResource(arn, nextToken string, maxResults int) ([]Tag, string, error)

	// Prepared Statements
	BatchGetPreparedStatement(
		workGroup string,
		names []string,
	) ([]PreparedStatement, []UnprocessedPreparedStatementName)
	CreatePreparedStatement(name, description, workGroup, queryStatement string) error
	DeletePreparedStatement(name, workGroup string) error
	GetPreparedStatement(name, workGroup string) (*PreparedStatement, error)
	ListPreparedStatements(
		workGroup, nextToken string,
		maxResults int,
	) ([]PreparedStatementSummary, string, error)

	// Capacity Reservations
	CancelCapacityReservation(name string) error
	CreateCapacityReservation(name string, targetDPUs int32, tags map[string]string) error
	DeleteCapacityReservation(name string) error

	// Notebooks
	CreateNotebook(workGroup, name string) (string, error)
	CreatePresignedNotebookURL(sessionID string) (url, authToken string, authTokenExpiration float64, err error)
	DeleteNotebook(notebookID string) error
	ExportNotebook(notebookID string) (NotebookMetadata, string, error)
	GetNotebookMetadata(notebookID string) (*NotebookMetadata, error)
	ListNotebookMetadata(workGroup, namePrefix string) ([]NotebookMetadata, error)
	ImportNotebook(workGroup, name, payload, notebookType string) (string, error)
	UpdateNotebook(notebookID, payload, notebookType, sessionID string) error
	UpdateNotebookMetadata(notebookID, newName string) error

	// Sessions
	StartSession(
		workGroup, description, notebookVersion string,
		engineCfg EngineConfiguration,
		sessionCfg SessionConfiguration,
		monitoringCfg MonitoringConfiguration,
		notebookID string,
	) (string, string, error)
	GetSession(id string) (*Session, error)
	GetSessionStatus(id string) (SessionStatus, error)
	GetSessionEndpoint(id string) (url, authToken string, authTokenExpiration float64, err error)
	TerminateSession(id string) (string, error)
	ListSessions(workGroup, stateFilter string) ([]SessionSummary, error)
	ListNotebookSessions(notebookID string) ([]SessionSummary, error)

	// Calculations
	StartCalculationExecution(sessionID, description, codeBlock string) (string, string, error)
	GetCalculationExecution(id string) (*CalculationExecution, error)
	GetCalculationExecutionStatus(id string) (CalculationStatus, CalculationStatistics, error)
	GetCalculationExecutionCode(id string) (string, error)
	StopCalculationExecution(id string) (string, error)
	ListCalculationExecutions(sessionID, stateFilter string) ([]CalculationSummary, error)

	// Capacity reservations (extended)
	GetCapacityReservation(name string) (*CapacityReservation, error)
	ListCapacityReservations() ([]CapacityReservation, error)
	UpdateCapacityReservation(name string, targetDPUs int32) error
	PutCapacityAssignmentConfiguration(name string, assignments []CapacityAssignment) error
	GetCapacityAssignmentConfiguration(name string) (*CapacityAssignmentConfiguration, error)

	// Database / table metadata
	GetDatabase(catalog, name string) (*Database, error)
	ListDatabases(catalog string) ([]Database, error)
	GetTableMetadata(catalog, database, table string) (*TableMetadata, error)
	ListTableMetadata(catalog, database, expr string) ([]TableMetadata, error)

	// Misc updates / runtime stats
	UpdateNamedQuery(id, name, description, queryString string) error
	UpdatePreparedStatement(name, workGroup, queryStatement, description string) error
	GetQueryRuntimeStatistics(id string) (*QueryRuntimeStatistics, error)
	GetResourceDashboard(resourceARN string) (string, error)

	// Engine version / executor / DPU listings
	ListEngineVersions() []EngineVersionDescriptor
	ListApplicationDPUSizes() []ApplicationDPUSizes
	ListExecutors(sessionID, stateFilter string) ([]Executor, error)
}

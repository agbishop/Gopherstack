package databrew

// DatasetFormatOptions holds format-specific options for a dataset.
//
// The JSON field's wire key is "Json" (mixed case), NOT "JSON" -- confirmed
// against aws-sdk-go-v2/service/databrew's deserializer
// (awsRestjson1_deserializeDocumentFormatOptions switches on the exact,
// case-sensitive key "Json"). A response emitting the Go-idiomatic "JSON"
// falls through that switch's default case and the client silently drops the
// field, so a dataset created with JSON format options would appear to have
// none on describe/list.
type DatasetFormatOptions struct {
	Csv   *CsvOptions   `json:"Csv,omitempty"`
	Excel *ExcelOptions `json:"Excel,omitempty"`
	JSON  *JSONOptions  `json:"Json,omitempty"`
}

// CsvOptions defines how DataBrew reads a CSV dataset input
// (aws-sdk-go-v2/service/databrew/types.CsvOptions).
type CsvOptions struct {
	HeaderRow *bool  `json:"HeaderRow,omitempty"`
	Delimiter string `json:"Delimiter,omitempty"`
}

// ExcelOptions defines how DataBrew reads an Excel dataset input
// (aws-sdk-go-v2/service/databrew/types.ExcelOptions).
type ExcelOptions struct {
	HeaderRow    *bool    `json:"HeaderRow,omitempty"`
	SheetIndexes []int32  `json:"SheetIndexes,omitempty"`
	SheetNames   []string `json:"SheetNames,omitempty"`
}

// JSONOptions defines how DataBrew reads a JSON dataset input
// (aws-sdk-go-v2/service/databrew/types.JSONOptions).
type JSONOptions struct {
	MultiLine bool `json:"MultiLine,omitempty"`
}

// DatasetInput holds the data source for a dataset.
type DatasetInput struct {
	S3InputDefinition          *S3Location       `json:"S3InputDefinition,omitempty"`
	DataCatalogInputDefinition *DataCatalogInput `json:"DataCatalogInputDefinition,omitempty"`
	DatabaseInputDefinition    *DatabaseInput    `json:"DatabaseInputDefinition,omitempty"`
}

// PathOptions defines how DataBrew selects files for a given Amazon S3 path
// in a dataset (aws-sdk-go-v2/service/databrew/types.PathOptions).
type PathOptions struct {
	FilesLimit                *FilesLimit                 `json:"FilesLimit,omitempty"`
	LastModifiedDateCondition *FilterExpression           `json:"LastModifiedDateCondition,omitempty"`
	Parameters                map[string]DatasetParameter `json:"Parameters,omitempty"`
}

// FilesLimit imposes a limit on the number of Amazon S3 files selected for a
// dataset from a connected Amazon S3 path.
type FilesLimit struct {
	Order     string `json:"Order,omitempty"`
	OrderedBy string `json:"OrderedBy,omitempty"`
	MaxFiles  int    `json:"MaxFiles"`
}

// FilterExpression defines parameter-matching conditions (e.g. for dynamic
// dataset paths or datetime parameter filters).
type FilterExpression struct {
	ValuesMap  map[string]string `json:"ValuesMap"`
	Expression string            `json:"Expression"`
}

// DatasetParameter maps a name used in a dataset's Amazon S3 path to its
// definition.
type DatasetParameter struct {
	DatetimeOptions *DatetimeOptions  `json:"DatetimeOptions,omitempty"`
	Filter          *FilterExpression `json:"Filter,omitempty"`
	Name            string            `json:"Name"`
	Type            string            `json:"Type"`
	CreateColumn    bool              `json:"CreateColumn,omitempty"`
}

// DatetimeOptions holds additional options for interpreting datetime
// parameters used in a dataset's Amazon S3 path.
type DatetimeOptions struct {
	Format         string `json:"Format"`
	LocaleCode     string `json:"LocaleCode,omitempty"`
	TimezoneOffset string `json:"TimezoneOffset,omitempty"`
}

// S3Location references an S3 path.
type S3Location struct {
	Bucket      string `json:"Bucket"`
	Key         string `json:"Key,omitempty"`
	BucketOwner string `json:"BucketOwner,omitempty"`
}

// DataCatalogInput references a Glue Data Catalog table.
type DataCatalogInput struct {
	TempDirectory *S3Location `json:"TempDirectory,omitempty"`
	DatabaseName  string      `json:"DatabaseName"`
	TableName     string      `json:"TableName"`
	CatalogID     string      `json:"CatalogId,omitempty"`
}

// DatabaseInput references a database table.
type DatabaseInput struct {
	TempDirectory      *S3Location `json:"TempDirectory,omitempty"`
	GlueConnectionName string      `json:"GlueConnectionName"`
	DatabaseTableName  string      `json:"DatabaseTableName"`
	QueryString        string      `json:"QueryString,omitempty"`
}

// Dataset represents a DataBrew dataset. AccountID mirrors
// aws-sdk-go-v2/service/databrew/types.Dataset's AccountId member -- present
// on ListDatasets items only. DescribeDatasetOutput
// (api_op_DescribeDataset.go:39-88) has no AccountId member at all, so
// handleDescribeDataset clears it before marshaling; a raw-body or non-SDK
// caller would otherwise see a field the real API never sends.
type Dataset struct {
	PathOptions      *PathOptions         `json:"PathOptions,omitempty"`
	FormatOptions    DatasetFormatOptions `json:"FormatOptions,omitzero"`
	Input            DatasetInput         `json:"Input"`
	Tags             map[string]string    `json:"Tags,omitempty"`
	Name             string               `json:"Name"`
	Arn              string               `json:"ResourceArn"`
	Format           string               `json:"Format,omitempty"`
	Source           string               `json:"Source,omitempty"`
	CreatedBy        string               `json:"CreatedBy,omitempty"`
	LastModifiedBy   string               `json:"LastModifiedBy,omitempty"`
	AccountID        string               `json:"AccountId,omitempty"`
	CreateDate       float64              `json:"CreateDate,omitempty"`
	LastModifiedDate float64              `json:"LastModifiedDate,omitempty"`
}

// RecipeStep is one transformation step in a recipe.
type RecipeStep struct {
	Action               map[string]any   `json:"Action,omitempty"`
	ConditionExpressions []map[string]any `json:"ConditionExpressions,omitempty"`
}

// Recipe represents a DataBrew recipe. ProjectName mirrors
// aws-sdk-go-v2/service/databrew/types.Recipe's ProjectName member
// (deserializers.go's awsRestjson1_deserializeDocumentRecipe, case
// "ProjectName") -- this backend does not store the association on the
// recipe itself, so it is derived at read time from the reverse link
// CreateProject already stores (Project.RecipeName); see
// InMemoryBackend.recipeProjectName.
type Recipe struct {
	Tags             map[string]string `json:"Tags,omitempty"`
	Name             string            `json:"Name"`
	Arn              string            `json:"ResourceArn"`
	Description      string            `json:"Description,omitempty"`
	ProjectName      string            `json:"ProjectName,omitempty"`
	PublishedBy      string            `json:"PublishedBy,omitempty"`
	RecipeVersion    string            `json:"RecipeVersion,omitempty"`
	CreatedBy        string            `json:"CreatedBy,omitempty"`
	LastModifiedBy   string            `json:"LastModifiedBy,omitempty"`
	Steps            []RecipeStep      `json:"Steps,omitempty"`
	PublishedDate    float64           `json:"PublishedDate,omitempty"`
	CreateDate       float64           `json:"CreateDate,omitempty"`
	LastModifiedDate float64           `json:"LastModifiedDate,omitempty"`
}

// RecipeVersionErrorDetail describes a single recipe version's failure
// within a BatchDeleteRecipeVersion partial-failure response.
type RecipeVersionErrorDetail struct {
	RecipeVersion string `json:"RecipeVersion,omitempty"`
	ErrorCode     string `json:"ErrorCode,omitempty"`
	ErrorMessage  string `json:"ErrorMessage,omitempty"`
}

// Sample describes a data sample for a project.
type Sample struct {
	Type string `json:"Type,omitempty"`
	Size int    `json:"Size,omitempty"`
}

// Project represents a DataBrew project. AccountID mirrors
// aws-sdk-go-v2/service/databrew/types.Project's AccountId member -- see
// Dataset's AccountID doc comment; DescribeProjectOutput
// (api_op_DescribeProject.go:39-97) has no AccountId member either.
//
// OpenDate/OpenedBy mirror types.Project's real members (deserializers.go's
// awsRestjson1_deserializeDocumentProject, cases "OpenDate"/"OpenedBy") --
// there is no "SessionStatus" member on the real type at all (confirmed
// against that same deserializer's full case list: no such key exists), so
// the field this struct previously carried under that name was fabricated,
// a real API caller never sends it, and it has been removed. OpenDate is
// set by StartProjectSession, the real trigger for it (see
// InMemoryBackend.OpenProjectSession). OpenedBy stays unpopulated -- like
// CreatedBy/LastModifiedBy above, this backend has no caller-identity
// infrastructure to derive it from.
type Project struct {
	Tags             map[string]string `json:"Tags,omitempty"`
	Name             string            `json:"Name"`
	Arn              string            `json:"ResourceArn"`
	DatasetName      string            `json:"DatasetName,omitempty"`
	RecipeName       string            `json:"RecipeName"`
	RoleArn          string            `json:"RoleArn,omitempty"`
	CreatedBy        string            `json:"CreatedBy,omitempty"`
	LastModifiedBy   string            `json:"LastModifiedBy,omitempty"`
	AccountID        string            `json:"AccountId,omitempty"`
	OpenedBy         string            `json:"OpenedBy,omitempty"`
	Sample           Sample            `json:"Sample,omitzero"`
	CreateDate       float64           `json:"CreateDate,omitempty"`
	LastModifiedDate float64           `json:"LastModifiedDate,omitempty"`
	OpenDate         float64           `json:"OpenDate,omitempty"`
}

// Output describes a DataBrew job output destination.
type Output struct {
	FormatOptions     map[string]any `json:"FormatOptions,omitempty"`
	Location          S3Location     `json:"Location,omitzero"`
	Format            string         `json:"Format,omitempty"`
	CompressionFormat string         `json:"CompressionFormat,omitempty"`
	PartitionColumns  []string       `json:"PartitionColumns,omitempty"`
	MaxOutputFiles    int            `json:"MaxOutputFiles,omitempty"`
	Overwrite         bool           `json:"Overwrite,omitempty"`
}

// RecipeRef holds a reference to a DataBrew recipe and optional version.
type RecipeRef struct {
	Name          string `json:"Name"`
	RecipeVersion string `json:"RecipeVersion,omitempty"`
}

// DatabaseTableOutputOptions specifies how and where a recipe job writes
// database output (aws-sdk-go-v2/service/databrew/types.DatabaseTableOutputOptions).
type DatabaseTableOutputOptions struct {
	TempDirectory *S3Location `json:"TempDirectory,omitempty"`
	TableName     string      `json:"TableName"`
}

// S3TableOutputOptions specifies the S3 location for a recipe job's Glue Data
// Catalog output (aws-sdk-go-v2/service/databrew/types.S3TableOutputOptions).
type S3TableOutputOptions struct {
	Location S3Location `json:"Location,omitzero"`
}

// DataCatalogOutput represents where a recipe job writes Glue Data Catalog
// output (aws-sdk-go-v2/service/databrew/types.DataCatalogOutput).
type DataCatalogOutput struct {
	DatabaseOptions *DatabaseTableOutputOptions `json:"DatabaseOptions,omitempty"`
	S3Options       *S3TableOutputOptions       `json:"S3Options,omitempty"`
	DatabaseName    string                      `json:"DatabaseName"`
	TableName       string                      `json:"TableName"`
	CatalogID       string                      `json:"CatalogId,omitempty"`
	Overwrite       bool                        `json:"Overwrite,omitempty"`
}

// DatabaseOutput represents a JDBC database output destination for a recipe
// job (aws-sdk-go-v2/service/databrew/types.DatabaseOutput).
type DatabaseOutput struct {
	DatabaseOptions    *DatabaseTableOutputOptions `json:"DatabaseOptions"`
	GlueConnectionName string                      `json:"GlueConnectionName"`
	DatabaseOutputMode string                      `json:"DatabaseOutputMode,omitempty"`
}

// JobSample determines how many rows a profile job processes
// (aws-sdk-go-v2/service/databrew/types.JobSample).
type JobSample struct {
	Mode string `json:"Mode,omitempty"`
	Size int64  `json:"Size,omitempty"`
}

// Job represents a DataBrew job. AccountID mirrors
// aws-sdk-go-v2/service/databrew/types.Job's AccountId member -- see
// Dataset's AccountID doc comment; DescribeJobOutput
// (api_op_DescribeJob.go:39+) has no AccountId member either.
type Job struct {
	// ProfileConfiguration is left untyped: it nests 4 levels deep
	// (ProfileConfiguration -> ColumnStatisticsConfigurations ->
	// Statistics(StatisticsConfiguration) -> Overrides([]StatisticOverride)),
	// spans 6 distinct struct shapes, and carries two independent
	// list-of-struct branches (column overrides and entity-detector
	// AllowedStatistics) -- deep enough that a partial model risks silently
	// dropping fields a client can't tell were never implemented.
	ProfileConfiguration     map[string]any      `json:"ProfileConfiguration,omitempty"`
	JobSample                *JobSample          `json:"JobSample,omitempty"`
	Tags                     map[string]string   `json:"Tags,omitempty"`
	RecipeReference          *RecipeRef          `json:"RecipeReference,omitempty"`
	EncryptionMode           string              `json:"EncryptionMode,omitempty"`
	EncryptionKeyArn         string              `json:"EncryptionKeyArn,omitempty"`
	DatasetName              string              `json:"DatasetName,omitempty"`
	ProjectName              string              `json:"ProjectName,omitempty"`
	Name                     string              `json:"Name"`
	CreatedBy                string              `json:"CreatedBy,omitempty"`
	AccountID                string              `json:"AccountId,omitempty"`
	RecipeName               string              `json:"-"`
	RoleArn                  string              `json:"RoleArn,omitempty"`
	LogSubscription          string              `json:"LogSubscription,omitempty"`
	Type                     string              `json:"Type,omitempty"`
	LastModifiedBy           string              `json:"LastModifiedBy,omitempty"`
	Arn                      string              `json:"ResourceArn"`
	ValidationConfigurations []map[string]any    `json:"ValidationConfigurations,omitempty"`
	DataCatalogOutputs       []DataCatalogOutput `json:"DataCatalogOutputs,omitempty"`
	DatabaseOutputs          []DatabaseOutput    `json:"DatabaseOutputs,omitempty"`
	Outputs                  []Output            `json:"Outputs,omitempty"`
	Timeout                  int                 `json:"Timeout,omitempty"`
	MaxRetries               int                 `json:"MaxRetries,omitempty"`
	MaxCapacity              int                 `json:"MaxCapacity,omitempty"`
	LastModifiedDate         float64             `json:"LastModifiedDate,omitempty"`
	CreateDate               float64             `json:"CreateDate,omitempty"`
}

// JobExtras bundles the optional job fields that are specific to one of the
// two job types (profile vs. recipe) but modeled on the shared Job entity,
// so CreateJob/UpdateJob's core positional signature doesn't grow further.
// ProfileConfiguration/JobSample/ValidationConfigurations are populated only
// by the profile-job handlers; DataCatalogOutputs/DatabaseOutputs only by
// the recipe-job handlers. EncryptionMode/EncryptionKeyArn/LogSubscription
// are accepted by both real job types. An unset (zero-value) field on
// UpdateJob leaves the corresponding Job field unchanged. MaxCapacity/
// MaxRetries/Timeout are here (rather than positional, unlike UpdateJob)
// purely because CreateJob has no existing positional slots for them --
// CreateProfileJobInput/CreateRecipeJobInput both accept all three but the
// pre-existing CreateJob signature silently dropped them.
type JobExtras struct {
	// ProfileConfiguration stays untyped -- see Job.ProfileConfiguration's
	// doc comment.
	ProfileConfiguration map[string]any
	JobSample            *JobSample
	// RecipeVersion is the caller-specified CreateRecipeJobInput.RecipeReference.RecipeVersion.
	// Empty means the recipe's LATEST_WORKING draft, matching real
	// aws-sdk-go-v2/service/databrew/types.RecipeReference.RecipeVersion
	// (optional, "The identifier for the version for the recipe").
	RecipeVersion            string
	EncryptionMode           string
	EncryptionKeyArn         string
	LogSubscription          string
	DataCatalogOutputs       []DataCatalogOutput
	DatabaseOutputs          []DatabaseOutput
	ValidationConfigurations []map[string]any
	MaxCapacity              int
	MaxRetries               int
	Timeout                  int
}

// JobRun represents a single execution of a DataBrew job. Attempt/
// DataCatalogOutputs/DatabaseOutputs/JobSample/LogSubscription/Outputs/
// RecipeReference mirror real types.JobRun members (deserializers.go's
// awsRestjson1_deserializeDocumentJobRun) that were previously never
// emitted at all; StartJobRun now snapshots them from the parent Job, the
// only backend state they could come from. ErrorMessage/StartedBy are also
// real members, left always-unpopulated and disclosed in PARITY.md: this
// backend's StartJobRun always transitions STARTING->SUCCEEDED (see
// jobRunTransitionDelay) with no FAILED path to source an error message
// from, and, like CreatedBy/LastModifiedBy elsewhere in this package, there
// is no caller-identity infrastructure to derive StartedBy from.
type JobRun struct {
	RecipeReference          *RecipeRef          `json:"RecipeReference,omitempty"`
	JobSample                *JobSample          `json:"JobSample,omitempty"`
	DatasetName              string              `json:"DatasetName,omitempty"`
	JobName                  string              `json:"JobName"`
	RunID                    string              `json:"RunId"`
	State                    string              `json:"State"`
	LogGroupName             string              `json:"LogGroupName,omitempty"`
	LogSubscription          string              `json:"LogSubscription,omitempty"`
	ErrorMessage             string              `json:"ErrorMessage,omitempty"`
	StartedBy                string              `json:"StartedBy,omitempty"`
	DataCatalogOutputs       []DataCatalogOutput `json:"DataCatalogOutputs,omitempty"`
	DatabaseOutputs          []DatabaseOutput    `json:"DatabaseOutputs,omitempty"`
	Outputs                  []Output            `json:"Outputs,omitempty"`
	ValidationConfigurations []map[string]any    `json:"ValidationConfigurations,omitempty"`
	StartedOn                float64             `json:"StartedOn,omitempty"`
	CompletedOn              float64             `json:"CompletedOn,omitempty"`
	ExecutionTime            int                 `json:"ExecutionTime,omitempty"`
	Attempt                  int                 `json:"Attempt,omitempty"`
}

// Rule represents a data quality rule.
type Rule struct {
	SubstitutionMap map[string]string `json:"SubstitutionMap,omitempty"`
	Threshold       map[string]any    `json:"Threshold,omitempty"`
	Name            string            `json:"Name"`
	CheckExpression string            `json:"CheckExpression"`
	ColumnSelectors []map[string]any  `json:"ColumnSelectors,omitempty"`
	Disabled        bool              `json:"Disabled,omitempty"`
}

// Ruleset is the internal storage representation of a DataBrew data quality
// ruleset. It is never marshaled directly.
//
// ListRulesets and DescribeRuleset use genuinely different wire shapes in
// the real SDK: DescribeRulesetOutput (api_op_DescribeRuleset.go:39-77)
// carries Rules (the full rule list), no AccountId/RuleCount at all, while
// ListRulesetsOutput.Rulesets is []types.RulesetItem (types.go:1020) --
// AccountId + RuleCount (an integer count), no Rules field at all (confirmed
// against awsRestjson1_deserializeDocumentRulesetItem, deserializers.go:11521,
// whose key switch has "RuleCount", not "Rules"). A real client silently
// ignores unrecognized keys, but a raw-body or non-SDK caller reading
// DescribeRuleset's response would see a fabricated AccountId/RuleCount, and
// one reading ListRulesets would see every ruleset's full rule text leaked
// even though real ListRulesets never sends it. newRulesetDescribeView and
// newRulesetListItem below project this type into each op's real shape.
type Ruleset struct {
	Tags             map[string]string `json:"Tags,omitempty"`
	Name             string            `json:"Name"`
	Arn              string            `json:"ResourceArn"`
	Description      string            `json:"Description,omitempty"`
	TargetArn        string            `json:"TargetArn"`
	CreatedBy        string            `json:"CreatedBy,omitempty"`
	LastModifiedBy   string            `json:"LastModifiedBy,omitempty"`
	AccountID        string            `json:"AccountId,omitempty"`
	Rules            []Rule            `json:"Rules"`
	RuleCount        int               `json:"RuleCount"`
	CreateDate       float64           `json:"CreateDate,omitempty"`
	LastModifiedDate float64           `json:"LastModifiedDate,omitempty"`
}

// RulesetDescribeView is the wire shape for DescribeRulesetOutput
// (api_op_DescribeRuleset.go:39-77): Rules, no AccountId/RuleCount.
type RulesetDescribeView struct {
	Tags             map[string]string `json:"Tags,omitempty"`
	Name             string            `json:"Name"`
	Arn              string            `json:"ResourceArn"`
	Description      string            `json:"Description,omitempty"`
	TargetArn        string            `json:"TargetArn"`
	CreatedBy        string            `json:"CreatedBy,omitempty"`
	LastModifiedBy   string            `json:"LastModifiedBy,omitempty"`
	Rules            []Rule            `json:"Rules"`
	CreateDate       float64           `json:"CreateDate,omitempty"`
	LastModifiedDate float64           `json:"LastModifiedDate,omitempty"`
}

// RulesetListItem is the wire shape for ListRulesetsOutput.Rulesets
// (types.RulesetItem, types/types.go:1020): AccountId + RuleCount, no Rules.
type RulesetListItem struct {
	Tags             map[string]string `json:"Tags,omitempty"`
	Name             string            `json:"Name"`
	Arn              string            `json:"ResourceArn"`
	Description      string            `json:"Description,omitempty"`
	TargetArn        string            `json:"TargetArn"`
	CreatedBy        string            `json:"CreatedBy,omitempty"`
	LastModifiedBy   string            `json:"LastModifiedBy,omitempty"`
	AccountID        string            `json:"AccountId,omitempty"`
	RuleCount        int               `json:"RuleCount"`
	CreateDate       float64           `json:"CreateDate,omitempty"`
	LastModifiedDate float64           `json:"LastModifiedDate,omitempty"`
}

// newRulesetDescribeView projects a Ruleset into DescribeRuleset's real
// output shape.
func newRulesetDescribeView(rs *Ruleset) RulesetDescribeView {
	return RulesetDescribeView{
		Tags: rs.Tags, Name: rs.Name, Arn: rs.Arn, Description: rs.Description,
		TargetArn: rs.TargetArn, CreatedBy: rs.CreatedBy, LastModifiedBy: rs.LastModifiedBy,
		Rules: rs.Rules, CreateDate: rs.CreateDate, LastModifiedDate: rs.LastModifiedDate,
	}
}

// newRulesetListItem projects a Ruleset into ListRulesets' real per-item
// shape (types.RulesetItem).
func newRulesetListItem(rs *Ruleset) RulesetListItem {
	return RulesetListItem{
		Tags: rs.Tags, Name: rs.Name, Arn: rs.Arn, Description: rs.Description,
		TargetArn: rs.TargetArn, CreatedBy: rs.CreatedBy, LastModifiedBy: rs.LastModifiedBy,
		AccountID: rs.AccountID, RuleCount: rs.RuleCount,
		CreateDate: rs.CreateDate, LastModifiedDate: rs.LastModifiedDate,
	}
}

// Schedule represents a DataBrew schedule. AccountID mirrors
// aws-sdk-go-v2/service/databrew/types.Schedule's AccountId member -- see
// Dataset's AccountID doc comment; DescribeScheduleOutput
// (api_op_DescribeSchedule.go:38-76) has no AccountId member either.
type Schedule struct {
	Tags             map[string]string `json:"Tags,omitempty"`
	Name             string            `json:"Name"`
	Arn              string            `json:"ResourceArn"`
	CronExpression   string            `json:"CronExpression"`
	CreatedBy        string            `json:"CreatedBy,omitempty"`
	LastModifiedBy   string            `json:"LastModifiedBy,omitempty"`
	AccountID        string            `json:"AccountId,omitempty"`
	JobNames         []string          `json:"JobNames,omitempty"`
	CreateDate       float64           `json:"CreateDate,omitempty"`
	LastModifiedDate float64           `json:"LastModifiedDate,omitempty"`
}

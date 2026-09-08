package athena

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"maps"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

const (
	idChars  = "abcdef0123456789"
	idLength = 10

	defaultWorkGroup = "primary"
	awsDataCatalog   = "AwsDataCatalog"
	millisToSeconds  = 1000.0

	dataCatalogTypeFederated = "FEDERATED"
	dataCatalogTypeGlue      = "GLUE"

	stateAuto       = "AUTO"
	stateSucceeded  = "SUCCEEDED"
	stateFailed     = "FAILED"
	stateCancelled  = "CANCELLED"
	stateCancelling = "CANCELLING"

	workGroupStateEnabled  = "ENABLED"
	workGroupStateDisabled = "DISABLED"

	columnTypeString = "string"

	tableTypeExternal = "EXTERNAL_TABLE"
)

// validDataCatalogTypes is the set of accepted DataCatalog type values.
func isValidDataCatalogType(t string) bool {
	switch t {
	case "LAMBDA", dataCatalogTypeGlue, "HIVE", dataCatalogTypeFederated:
		return true
	default:
		return false
	}
}

// InMemoryBackend implements StorageBackend using in-memory maps.
//
// Every AWS resource collection is a *store.Table[T] registered on registry
// (see store_setup.go for the full split between tables registered directly
// and the two "dirty" tables handled separately); queryResults, tableData,
// and resourceTags remain plain maps -- see the store_setup.go file doc for
// why each is left as-is.
type InMemoryBackend struct {
	glueSource                    GlueMetadataSource
	s3                            S3Storer
	queryExecutions               *store.Table[QueryExecution]
	mu                            *lockmetrics.RWMutex
	namedQueriesByWorkGroup       *store.Index[NamedQuery]
	dataCatalogs                  *store.Table[DataCatalog]
	notebooks                     *store.Table[Notebook]
	notebooksByName               *store.Index[Notebook]
	queryResults                  map[string]*sqlResult
	tableData                     map[string][]map[string]any
	resourceTags                  map[string]map[string]string
	preparedStatements            *store.Table[PreparedStatement]
	preparedStatementsByWorkGroup *store.Index[PreparedStatement]
	registry                      *store.Registry
	namedQueries                  *store.Table[NamedQuery]
	calculations                  *store.Table[CalculationExecution]
	queryExecutionsByWorkGroup    *store.Index[QueryExecution]
	capacityReservations          *store.Table[CapacityReservation]
	capacityAssignments           *store.Table[CapacityAssignmentConfiguration]
	databases                     *store.Table[Database]
	databasesByCatalog            *store.Index[Database]
	tables                        *store.Table[TableMetadata]
	tablesByDatabase              *store.Index[TableMetadata]
	sessions                      *store.Table[Session]
	workGroups                    *store.Table[WorkGroup]
	region                        string
	accountID                     string
}

// SetGlueMetadataSource wires a GLUE-type DataCatalog's database/table reads
// to real Glue state instead of Athena's internal simulation.
func (b *InMemoryBackend) SetGlueMetadataSource(src GlueMetadataSource) {
	b.glueSource = src
}

// SetS3Backend wires S3 so a succeeded query execution's result actually
// writes an object to ResultConfiguration.OutputLocation, instead of only
// storing/echoing the configuration.
func (b *InMemoryBackend) SetS3Backend(s3 S3Storer) {
	b.s3 = s3
}

// NewInMemoryBackend creates a new InMemoryBackend and seeds the default "primary" workgroup.
func NewInMemoryBackend(region, accountID string) *InMemoryBackend {
	if region == "" {
		region = config.DefaultRegion
	}

	if accountID == "" {
		accountID = config.DefaultAccountID
	}

	b := &InMemoryBackend{
		registry:     store.NewRegistry(),
		queryResults: make(map[string]*sqlResult),
		tableData:    make(map[string][]map[string]any),
		resourceTags: make(map[string]map[string]string),
		region:       region,
		accountID:    accountID,
		mu:           lockmetrics.New("athena"),
	}

	registerAllTables(b)

	b.workGroups.Put(&WorkGroup{
		Name:  defaultWorkGroup,
		State: workGroupStateEnabled,
	})

	b.seedDefaultMetadata()

	return b
}

// seedDefaultMetadata seeds AwsDataCatalog and example database/table metadata
// so that metadata operations return useful sample data without explicit setup.
func (b *InMemoryBackend) seedDefaultMetadata() {
	const database = "default"

	b.dataCatalogs.Put(&DataCatalog{
		Name:   awsDataCatalog,
		Type:   dataCatalogTypeGlue,
		Status: "CREATE_COMPLETE",
	})

	b.databases.Put(&Database{
		Catalog:     awsDataCatalog,
		Name:        database,
		Description: "Default Athena database",
	})

	b.tables.Put(&TableMetadata{
		Catalog:   awsDataCatalog,
		Database:  database,
		Name:      "sample_table",
		TableType: tableTypeExternal,
		Columns: []Column{
			{Name: "id", Type: "bigint"},
			{Name: "value", Type: columnTypeString},
		},
	})
}

// randomID generates a cryptographically random 10-character hex ID.
func randomID() string {
	b := make([]byte, idLength)
	charCount := uint64(len(idChars))

	for i := range b {
		var v [8]byte
		_, _ = rand.Read(v[:])
		b[i] = idChars[binary.BigEndian.Uint64(v[:])%charCount]
	}

	return string(b)
}

// sessionAuthTokenTTL is the validity window gopherstack assigns to the
// synthesized bearer tokens returned by GetSessionEndpoint and
// CreatePresignedNotebookUrl. AWS documents CreatePresignedNotebookUrl's token
// as needing a refresh roughly every 10 minutes; gopherstack reuses that
// window for both operations since neither backs a real authentication
// system.
const sessionAuthTokenTTL = 10 * time.Minute

// newSessionAuthToken synthesizes an opaque bearer token and its
// epoch-seconds expiration time for GetSessionEndpoint and
// CreatePresignedNotebookUrl, both of which require an AuthToken +
// AuthTokenExpirationTime pair on the wire despite gopherstack modeling no
// real authentication.
func newSessionAuthToken() (string, float64) {
	return randomID(), float64(time.Now().Add(sessionAuthTokenTTL).UnixMilli()) / millisToSeconds
}

// nowSeconds returns the current time as AWS-wire epoch-seconds.
func nowSeconds() float64 {
	return float64(time.Now().UnixMilli()) / millisToSeconds
}

func (b *InMemoryBackend) workGroupARN(name string) string {
	return arn.Build("athena", b.region, b.accountID, fmt.Sprintf("workgroup/%s", name))
}

func (b *InMemoryBackend) dataCatalogARN(name string) string {
	return arn.Build("athena", b.region, b.accountID, fmt.Sprintf("datacatalog/%s", name))
}

func (b *InMemoryBackend) capacityReservationARN(name string) string {
	return arn.Build("athena", b.region, b.accountID, fmt.Sprintf("capacity-reservation/%s", name))
}

// copyTags returns a shallow copy of the given tag map.
func copyTags(tags map[string]string) map[string]string {
	cp := make(map[string]string, len(tags))
	maps.Copy(cp, tags)

	return cp
}

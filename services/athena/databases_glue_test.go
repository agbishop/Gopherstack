package athena_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/athena"
)

var errFakeGlueNotFound = errors.New("fake glue: not found")

// fakeGlueSource is a minimal athena.GlueMetadataSource test double, standing
// in for a wired Glue backend without depending on services/glue.
type fakeGlueSource struct {
	databases map[string]*athena.GlueDatabase
	tables    map[string]*athena.GlueTable
}

func newFakeGlueSource() *fakeGlueSource {
	return &fakeGlueSource{
		databases: make(map[string]*athena.GlueDatabase),
		tables:    make(map[string]*athena.GlueTable),
	}
}

func (f *fakeGlueSource) GetDatabase(name string) (*athena.GlueDatabase, error) {
	d, ok := f.databases[name]
	if !ok {
		return nil, errFakeGlueNotFound
	}

	return d, nil
}

func (f *fakeGlueSource) GetDatabases() []*athena.GlueDatabase {
	out := make([]*athena.GlueDatabase, 0, len(f.databases))
	for _, d := range f.databases {
		out = append(out, d)
	}

	return out
}

func (f *fakeGlueSource) GetTable(dbName, tableName string) (*athena.GlueTable, error) {
	t, ok := f.tables[dbName+"/"+tableName]
	if !ok {
		return nil, errFakeGlueNotFound
	}

	return t, nil
}

func (f *fakeGlueSource) GetTables(dbName string) ([]*athena.GlueTable, error) {
	if _, ok := f.databases[dbName]; !ok {
		return nil, errFakeGlueNotFound
	}

	out := make([]*athena.GlueTable, 0, len(f.tables))

	for key, t := range f.tables {
		if key[:len(dbName)+1] == dbName+"/" {
			out = append(out, t)
		}
	}

	return out, nil
}

// TestGlueBackedCatalog_DelegatesToGlue proves that once a GLUE-type
// DataCatalog's SetGlueMetadataSource is wired, database/table reads come
// from the wired Glue source rather than Athena's internal simulation. This
// fails against the pre-fix code: databases.go never branched on the
// catalog's Type, so a database/table only present in the wired Glue source
// (and never created through Athena's own DDL) was reported not found.
func TestGlueBackedCatalog_DelegatesToGlue(t *testing.T) {
	t.Parallel()

	b := athena.NewInMemoryBackend("us-east-1", "123456789012")
	_, err := b.CreateDataCatalog("gluecat", "GLUE", "", nil, nil)
	require.NoError(t, err)

	src := newFakeGlueSource()
	src.databases["gluedb"] = &athena.GlueDatabase{Name: "gluedb", Description: "from glue"}
	src.tables["gluedb/gluetable"] = &athena.GlueTable{
		Name:    "gluetable",
		Columns: []athena.Column{{Name: "id", Type: "bigint"}},
	}

	b.SetGlueMetadataSource(src)

	db, err := b.GetDatabase("gluecat", "gluedb")
	require.NoError(t, err)
	assert.Equal(t, "gluedb", db.Name)
	assert.Equal(t, "from glue", db.Description)

	dbs, err := b.ListDatabases("gluecat")
	require.NoError(t, err)
	require.Len(t, dbs, 1)
	assert.Equal(t, "gluedb", dbs[0].Name)

	tbl, err := b.GetTableMetadata("gluecat", "gluedb", "gluetable")
	require.NoError(t, err)
	assert.Equal(t, "gluetable", tbl.Name)
	require.Len(t, tbl.Columns, 1)
	assert.Equal(t, "id", tbl.Columns[0].Name)

	tbls, err := b.ListTableMetadata("gluecat", "gluedb", "")
	require.NoError(t, err)
	require.Len(t, tbls, 1)
	assert.Equal(t, "gluetable", tbls[0].Name)

	// A database that only exists in Athena's internal simulation, never in
	// the wired Glue source, must NOT be visible through a GLUE-type
	// catalog once Glue is wired -- Glue is authoritative for GLUE catalogs.
	_, err = b.GetDatabase("gluecat", "not-in-glue")
	require.Error(t, err)
}

// TestGlueUnwiredCatalog_FallsBackToSimulation proves the unwired path stays
// permissive: AwsDataCatalog is seeded with Type "GLUE" (matching real AWS,
// where the built-in catalog is Glue-backed), and with no
// SetGlueMetadataSource call, GetDatabase/GetTableMetadata must still return
// the internally simulated seed data rather than erroring.
func TestGlueUnwiredCatalog_FallsBackToSimulation(t *testing.T) {
	t.Parallel()

	b := athena.NewInMemoryBackend("us-east-1", "123456789012")

	db, err := b.GetDatabase("AwsDataCatalog", "default")
	require.NoError(t, err)
	assert.Equal(t, "default", db.Name)

	tbl, err := b.GetTableMetadata("AwsDataCatalog", "default", "sample_table")
	require.NoError(t, err)
	assert.Equal(t, "sample_table", tbl.Name)
}

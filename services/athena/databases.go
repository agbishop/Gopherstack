package athena

import (
	"fmt"
	"maps"
	"regexp"
	"sort"
)

// isGlueBacked reports whether catalog is a GLUE-type DataCatalog with a
// Glue metadata source wired in. Callers must hold b.mu.
func (b *InMemoryBackend) isGlueBacked(catalog string) bool {
	if b.glueSource == nil {
		return false
	}

	dc, ok := b.dataCatalogs.Get(catalog)

	return ok && dc.Type == dataCatalogTypeGlue
}

// GetDatabase returns a database by catalog and name.
func (b *InMemoryBackend) GetDatabase(catalog, name string) (*Database, error) {
	if catalog == "" {
		return nil, fmt.Errorf("%w: CatalogName is required", ErrValidation)
	}

	if name == "" {
		return nil, fmt.Errorf("%w: DatabaseName is required", ErrValidation)
	}

	b.mu.RLock("GetDatabase")
	defer b.mu.RUnlock()

	if b.isGlueBacked(catalog) {
		gd, err := b.glueSource.GetDatabase(name)
		if err != nil {
			return nil, fmt.Errorf("%w: database %q not found in catalog %q", ErrMetadata, name, catalog)
		}

		return glueDatabaseToAthena(catalog, gd), nil
	}

	d, ok := b.databases.Get(databaseKey(catalog, name))
	if !ok {
		return nil, fmt.Errorf("%w: database %q not found in catalog %q", ErrMetadata, name, catalog)
	}

	cp := *d
	cp.Parameters = maps.Clone(d.Parameters)

	return &cp, nil
}

// ListDatabases returns all databases for a catalog.
func (b *InMemoryBackend) ListDatabases(catalog string) ([]Database, error) {
	if catalog == "" {
		return nil, fmt.Errorf("%w: CatalogName is required", ErrValidation)
	}

	b.mu.RLock("ListDatabases")
	defer b.mu.RUnlock()

	var out []Database

	if b.isGlueBacked(catalog) {
		for _, gd := range b.glueSource.GetDatabases() {
			out = append(out, *glueDatabaseToAthena(catalog, gd))
		}
	} else {
		dbs := b.databasesByCatalog.Get(catalog)
		out = make([]Database, 0, len(dbs))

		for _, d := range dbs {
			cp := *d
			cp.Parameters = maps.Clone(d.Parameters)
			out = append(out, cp)
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	return out, nil
}

// GetTableMetadata returns the metadata for a single table.
func (b *InMemoryBackend) GetTableMetadata(catalog, database, table string) (*TableMetadata, error) {
	if catalog == "" || database == "" || table == "" {
		return nil, fmt.Errorf("%w: CatalogName, DatabaseName, TableName are required", ErrValidation)
	}

	b.mu.RLock("GetTableMetadata")
	defer b.mu.RUnlock()

	if b.isGlueBacked(catalog) {
		gt, err := b.glueSource.GetTable(database, table)
		if err != nil {
			return nil, fmt.Errorf("%w: table %q not found in %s/%s", ErrMetadata, table, catalog, database)
		}

		return glueTableToAthena(catalog, database, gt), nil
	}

	t, ok := b.tables.Get(tableMetadataKey(catalog, database, table))
	if !ok {
		return nil, fmt.Errorf("%w: table %q not found in %s/%s", ErrMetadata, table, catalog, database)
	}

	cp := *t
	cp.Parameters = maps.Clone(t.Parameters)
	cp.Columns = append([]Column(nil), t.Columns...)
	cp.PartitionKeys = append([]Column(nil), t.PartitionKeys...)

	return &cp, nil
}

// ListTableMetadata returns all tables for a database, optionally filtered by
// a regex matched against table names (real Expression semantics — not a
// substring/prefix match).
func (b *InMemoryBackend) ListTableMetadata(catalog, database, expr string) ([]TableMetadata, error) {
	if catalog == "" || database == "" {
		return nil, fmt.Errorf("%w: CatalogName and DatabaseName are required", ErrValidation)
	}

	var re *regexp.Regexp

	if expr != "" {
		var err error

		re, err = regexp.Compile(expr)
		if err != nil {
			return nil, fmt.Errorf("%w: Expression %q is not a valid regex", ErrValidation, expr)
		}
	}

	b.mu.RLock("ListTableMetadata")
	defer b.mu.RUnlock()

	var out []TableMetadata

	if b.isGlueBacked(catalog) {
		gts, err := b.glueSource.GetTables(database)
		if err != nil {
			return nil, fmt.Errorf("%w: database %q not found in catalog %q", ErrMetadata, database, catalog)
		}

		for _, gt := range gts {
			if re != nil && !re.MatchString(gt.Name) {
				continue
			}

			out = append(out, *glueTableToAthena(catalog, database, gt))
		}
	} else {
		tables := b.tablesByDatabase.Get(databaseKey(catalog, database))
		out = make([]TableMetadata, 0, len(tables))

		for _, t := range tables {
			if re != nil && !re.MatchString(t.Name) {
				continue
			}

			cp := *t
			cp.Parameters = maps.Clone(t.Parameters)
			cp.Columns = append([]Column(nil), t.Columns...)
			cp.PartitionKeys = append([]Column(nil), t.PartitionKeys...)
			out = append(out, cp)
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	return out, nil
}

func glueDatabaseToAthena(catalog string, gd *GlueDatabase) *Database {
	return &Database{
		Catalog:     catalog,
		Name:        gd.Name,
		Description: gd.Description,
		Parameters:  maps.Clone(gd.Parameters),
	}
}

func glueTableToAthena(catalog, database string, gt *GlueTable) *TableMetadata {
	return &TableMetadata{
		Catalog:       catalog,
		Database:      database,
		Name:          gt.Name,
		TableType:     tableTypeExternal,
		Parameters:    maps.Clone(gt.Parameters),
		Columns:       append([]Column(nil), gt.Columns...),
		PartitionKeys: append([]Column(nil), gt.PartitionKeys...),
		CreateTime:    gt.CreateTime,
	}
}

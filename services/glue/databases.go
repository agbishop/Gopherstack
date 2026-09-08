package glue

import (
	"maps"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// cloneDatabase returns a deep copy of a Database.
func cloneDatabase(db *Database) *Database {
	cp := *db
	cp.Tags = maps.Clone(db.Tags)
	cp.Parameters = maps.Clone(db.Parameters)

	if len(db.CreateTableDefaultPermissions) > 0 {
		cp.CreateTableDefaultPermissions = make([]PrincipalPermissions, len(db.CreateTableDefaultPermissions))
		for i, p := range db.CreateTableDefaultPermissions {
			cp.CreateTableDefaultPermissions[i] = p
			cp.CreateTableDefaultPermissions[i].Permissions = append([]string(nil), p.Permissions...)

			if p.Principal != nil {
				principal := *p.Principal
				cp.CreateTableDefaultPermissions[i].Principal = &principal
			}
		}
	}

	if db.TargetDatabase != nil {
		target := *db.TargetDatabase
		cp.TargetDatabase = &target
	}

	return &cp
}

// databaseARN returns the ARN for a Glue database.
func (b *InMemoryBackend) databaseARN(name string) string {
	return arn.Build("glue", b.region, b.accountID, "database/"+name)
}

// CreateDatabase creates a new Glue database.
func (b *InMemoryBackend) CreateDatabase(
	input DatabaseInput,
	tags map[string]string,
) (*Database, error) {
	b.mu.Lock("CreateDatabase")
	defer b.mu.Unlock()

	if input.Name == "" || len(input.Name) > maxNameLen {
		return nil, ErrValidation
	}

	if err := validateTags(tags); err != nil {
		return nil, err
	}

	if b.databases.Has(input.Name) {
		return nil, ErrAlreadyExists
	}

	db := &Database{
		Name:                          input.Name,
		Description:                   input.Description,
		CatalogID:                     b.accountID,
		ARN:                           b.databaseARN(input.Name),
		Tags:                          maps.Clone(tags),
		CreateTime:                    float64(time.Now().Unix()),
		LocationURI:                   input.LocationURI,
		Parameters:                    maps.Clone(input.Parameters),
		CreateTableDefaultPermissions: append([]PrincipalPermissions(nil), input.CreateTableDefaultPermissions...),
		TargetDatabase:                input.TargetDatabase,
	}
	b.databases.Put(db)

	return cloneDatabase(db), nil
}

// GetDatabase retrieves a Glue database by name.
func (b *InMemoryBackend) GetDatabase(name string) (*Database, error) {
	b.mu.RLock("GetDatabase")
	defer b.mu.RUnlock()

	db, ok := b.databases.Get(name)
	if !ok {
		return nil, ErrNotFound
	}

	return cloneDatabase(db), nil
}

// GetDatabases returns all Glue databases sorted by name.
func (b *InMemoryBackend) GetDatabases() []*Database {
	b.mu.RLock("GetDatabases")
	defer b.mu.RUnlock()

	src := b.databases.Snapshot()
	out := make([]*Database, 0, len(src))
	for _, db := range src {
		out = append(out, cloneDatabase(db))
	}

	return out
}

// DeleteDatabase deletes a Glue database by name, also removing all its
// tables (and each table's partitions, versions, column statistics, and
// table optimizers -- see deleteTablePartitionsLocked) and user-defined
// functions. Per the real DeleteDatabase's documented contract
// (aws-sdk-go-v2/service/glue@v1.152.0 api_op_DeleteDatabase.go: "you no
// longer have access to the tables ... and the user-defined functions in the
// deleted database"), a client must never be able to reach either kind of
// resource once the database is gone.
func (b *InMemoryBackend) DeleteDatabase(name string) error {
	b.mu.Lock("DeleteDatabase")
	defer b.mu.Unlock()

	if !b.databases.Has(name) {
		return ErrNotFound
	}

	b.databases.Delete(name)

	prefix := name + "|"
	for _, t := range b.tables.Snapshot() {
		if k := tableKey(t.DatabaseName, t.Name); len(k) > len(prefix) && k[:len(prefix)] == prefix {
			b.tables.Delete(k)
			b.deleteTablePartitionsLocked(t.DatabaseName, t.Name)
		}
	}

	for _, u := range b.udfs.All() {
		if u.DatabaseName == name {
			b.udfs.Delete(b.udfKey(u.DatabaseName, u.FunctionName))
		}
	}

	return nil
}

// UpdateDatabase updates an existing Glue database.
func (b *InMemoryBackend) UpdateDatabase(name string, input DatabaseInput) error {
	b.mu.Lock("UpdateDatabase")
	defer b.mu.Unlock()

	db, ok := b.databases.Get(name)
	if !ok {
		return ErrNotFound
	}

	db.Description = input.Description
	db.LocationURI = input.LocationURI
	db.Parameters = maps.Clone(input.Parameters)
	db.CreateTableDefaultPermissions = append([]PrincipalPermissions(nil), input.CreateTableDefaultPermissions...)
	db.TargetDatabase = input.TargetDatabase

	return nil
}

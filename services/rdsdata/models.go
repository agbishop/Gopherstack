package rdsdata

import "time"

// Field represents a single field value in an RDS Data API record.
//
// ArrayValue is modeled for wire completeness (it is a real member of the
// SDK's Field union -- types.FieldMemberArrayValue) even though this mock
// can never populate it in a result: real AWS documents "Array parameters
// are not supported" for ExecuteStatementInput.Parameters (see
// validateNoArrayParameters in handler.go, which rejects it on the way in),
// and the pure-Go SQLite driver backing the mock engine never produces an
// array-typed result column. See PARITY.md.
type Field struct {
	IsNull       *bool       `json:"isNull,omitempty"`
	BooleanValue *bool       `json:"booleanValue,omitempty"`
	LongValue    *int64      `json:"longValue,omitempty"`
	DoubleValue  *float64    `json:"doubleValue,omitempty"`
	StringValue  *string     `json:"stringValue,omitempty"`
	ArrayValue   *ArrayValue `json:"arrayValue,omitempty"`
	BlobValue    []byte      `json:"blobValue,omitempty"`
}

// ArrayValue represents an array of values, mirroring the real API's
// ArrayValue union (types.ArrayValue in aws-sdk-go-v2/service/rdsdata).
// Exactly one member is meaningfully populated at a time, matching the
// real union's shape.
type ArrayValue struct {
	ArrayValues   []ArrayValue `json:"arrayValues,omitempty"`
	BooleanValues []bool       `json:"booleanValues,omitempty"`
	DoubleValues  []float64    `json:"doubleValues,omitempty"`
	LongValues    []int64      `json:"longValues,omitempty"`
	StringValues  []string     `json:"stringValues,omitempty"`
}

// ColumnMetadata describes a single column returned by a SQL statement.
// Field set mirrors the real RDS Data API shape (types.ColumnMetadata in
// aws-sdk-go-v2/service/rdsdata); see engine.go's columnMetadataFor for how
// each field is derived from the pure-Go SQLite driver's limited column
// introspection.
type ColumnMetadata struct {
	Name                string `json:"name"`
	Label               string `json:"label"`
	TypeName            string `json:"typeName"`
	SchemaName          string `json:"schemaName"`
	TableName           string `json:"tableName"`
	Type                int32  `json:"type"`
	ArrayBaseColumnType int32  `json:"arrayBaseColumnType"`
	Nullable            int32  `json:"nullable"`
	Precision           int32  `json:"precision"`
	Scale               int32  `json:"scale"`
	IsAutoIncrement     bool   `json:"isAutoIncrement"`
	IsCaseSensitive     bool   `json:"isCaseSensitive"`
	IsCurrency          bool   `json:"isCurrency"`
	IsSigned            bool   `json:"isSigned"`
}

// Value represents a single field value in the deprecated ExecuteSql
// result set -- the older Value union (types.Value in
// aws-sdk-go-v2/service/rdsdata, deserializers.go:3496), distinct from
// Field: bigIntValue/bitValue instead of longValue/booleanValue. No
// arrayValues/intValue/realValue/structValue members: the mock engine's row
// extraction (engine.go's fieldFromValue) never produces those.
type Value struct {
	IsNull      *bool    `json:"isNull,omitempty"`
	BitValue    *bool    `json:"bitValue,omitempty"`
	BigIntValue *int64   `json:"bigIntValue,omitempty"`
	DoubleValue *float64 `json:"doubleValue,omitempty"`
	StringValue *string  `json:"stringValue,omitempty"`
	BlobValue   []byte   `json:"blobValue,omitempty"`
}

// Record is a single row of a ResultFrame (types.Record in
// aws-sdk-go-v2/service/rdsdata, deserializers.go:2865).
type Record struct {
	Values []Value `json:"values"`
}

// ResultSetMetadata describes the columns of a ResultFrame (types.ResultSetMetadata,
// deserializers.go:2954).
type ResultSetMetadata struct {
	ColumnMetadata []ColumnMetadata `json:"columnMetadata"`
	ColumnCount    int64            `json:"columnCount"`
}

// ResultFrame is the result set of a single ExecuteSql statement
// (types.ResultFrame, deserializers.go:2913). It is left nil for statements
// that don't produce rows -- see SQLStatementResult.
type ResultFrame struct {
	ResultSetMetadata *ResultSetMetadata `json:"resultSetMetadata,omitempty"`
	Records           []Record           `json:"records"`
}

// Transaction represents an in-progress database transaction.
//
// CreatedAt and LastActivityAt back the Janitor's expiry rules (janitor.go),
// which mirror BeginTransaction's documented lifetime (rdsdata@v1.35.4
// api_op_BeginTransaction.go): a transaction is rolled back automatically
// after 24 hours, or after 3 minutes with no call using its transaction ID.
type Transaction struct {
	CreatedAt      time.Time `json:"createdAt"`
	LastActivityAt time.Time `json:"lastActivityAt"`
	TransactionID  string    `json:"transactionId"`
	Status         string    `json:"status"`
}

// ExecutedStatement represents a record of an executed SQL statement.
type ExecutedStatement struct {
	SQL           string `json:"sql"`
	ResourceARN   string `json:"resourceArn"`
	TransactionID string `json:"transactionId,omitempty"`
}

// SQLParameter represents a named parameter for a SQL statement.
// TypeHint mirrors the real API's DATE/DECIMAL/JSON/TIME/TIMESTAMP/UUID hint
// values; it is accepted on the wire but does not change bind behavior since
// the mock SQLite engine has no distinct DATE/TIMESTAMP/UUID column types to
// convert to (see PARITY.md).
type SQLParameter struct {
	Name     string `json:"name"`
	TypeHint string `json:"typeHint,omitempty"`
	Value    Field  `json:"value"`
}

// UpdateResult represents the result of a single update in a batch.
//
// GeneratedFields is populated with the rowid-alias INTEGER PRIMARY KEY
// value assigned by an INSERT, when the target table declares exactly one
// such column (see generatedFieldsFor in engine.go). It is left empty for
// every other statement shape, matching real AWS ("generatedFields ...
// isn't supported by Aurora PostgreSQL").
type UpdateResult struct {
	GeneratedFields []Field `json:"generatedFields"`
}

// SQLStatementResult represents the result of a single SQL statement in an ExecuteSql call.
type SQLStatementResult struct {
	ResultFrame            *ResultFrame `json:"resultFrame,omitempty"`
	NumberOfRecordsUpdated int64        `json:"numberOfRecordsUpdated"`
}

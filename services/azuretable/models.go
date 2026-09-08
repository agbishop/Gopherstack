package azuretable

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"
)

// EdmType identifies an entity property's OData EDM (Entity Data Model)
// type. See https://learn.microsoft.com/rest/api/storageservices/payload-format-for-table-service-operations
// for the wire-level annotation scheme this mirrors.
type EdmType string

// Supported EDM property types. EdmString is the default for a bare JSON
// string with no "@odata.type" annotation; EdmInt32 is the default for a
// bare, whole-number JSON number. See entity_ops.go's decodeProperty for the
// exact inference rules (mirroring azure-sdk-for-go/sdk/data/aztables's own
// EDMEntity.UnmarshalJSON, so unmodified SDK round trips match).
const (
	EdmString   EdmType = "Edm.String"
	EdmInt32    EdmType = "Edm.Int32"
	EdmInt64    EdmType = "Edm.Int64"
	EdmDouble   EdmType = "Edm.Double"
	EdmBoolean  EdmType = "Edm.Boolean"
	EdmDateTime EdmType = "Edm.DateTime"
	EdmGUID     EdmType = "Edm.Guid"
	EdmBinary   EdmType = "Edm.Binary"
)

// EntityProperty is a single typed entity property value. Value's concrete
// Go type is determined by Type:
//
//	EdmString    string
//	EdmInt32     int32
//	EdmInt64     int64
//	EdmDouble    float64
//	EdmBoolean   bool
//	EdmDateTime  time.Time
//	EdmGUID      string (canonical UUID string, not validated)
//	EdmBinary    []byte
//
// MarshalJSON below deliberately keeps a value receiver even though
// UnmarshalJSON must be a pointer receiver to mutate p: EntityProperty
// values live inside map[string]EntityProperty (Properties), and Go map
// values are not addressable, so encoding/json can only discover and call a
// value-receiver Marshaler when encoding a map's values directly --
// a pointer-receiver-only MarshalJSON would silently be skipped for every
// property in Properties, falling back to the wrong (default reflection)
// encoding with no error. This mixed-receiver shape is required, not an
// oversight.
type EntityProperty struct {
	Value any
	Type  EdmType
}

// entityPropertyWire is EntityProperty's on-disk (persistence snapshot)
// shape. A plain `any` Value field would lose type fidelity on JSON
// round-trip (encoding/json decodes every bare number into float64 and every
// []byte into a base64 string, regardless of the original Go type), so
// MarshalJSON/UnmarshalJSON re-encode Value per Type explicitly instead of
// relying on encoding/json's default interface{} behavior.
type entityPropertyWire struct {
	Value any     `json:"value"`
	Type  EdmType `json:"type"`
}

// MarshalJSON implements json.Marshaler for persistence snapshots (see
// persistence.go). This is independent of the OData wire encoding entity_ops.go
// uses for HTTP responses. Any Type/Value mismatch (e.g. an EdmBinary
// property whose Value isn't []byte) is a real error, not silently dropped:
// a construction bug elsewhere in the package should fail loudly here rather
// than writing a corrupt snapshot.
func (p EntityProperty) MarshalJSON() ([]byte, error) {
	v, err := p.marshalValue()
	if err != nil {
		return nil, err
	}

	return json.Marshal(entityPropertyWire{Type: p.Type, Value: v})
}

// marshalValue dispatches to marshalScalarValue or marshalStringEncodedValue
// per p.Type, mirroring unmarshalPropertyValue's own split.
func (p EntityProperty) marshalValue() (any, error) {
	switch p.Type {
	case EdmInt32, EdmDouble, EdmBoolean:
		return p.marshalScalarValue()
	case EdmInt64, EdmDateTime, EdmGUID, EdmBinary:
		return p.marshalStringEncodedValue()
	case EdmString:
		s, ok := p.Value.(string)
		if !ok {
			return nil, fmt.Errorf("%w: Edm.String property has non-string value %T", ErrInvalidEntityProperty, p.Value)
		}

		return s, nil
	default:
		return nil, fmt.Errorf("%w: unknown EdmType %q", ErrInvalidEntityProperty, p.Type)
	}
}

// marshalScalarValue encodes the three EDM types whose Go value is already
// the JSON-native type encoding/json's generic `any` decode produces (see
// unmarshalScalarValue's own doc comment for the matching decode side).
func (p EntityProperty) marshalScalarValue() (any, error) {
	switch p.Type {
	case EdmInt32:
		n, ok := p.Value.(int32)
		if !ok {
			return nil, fmt.Errorf("%w: Edm.Int32 property has non-int32 value %T", ErrInvalidEntityProperty, p.Value)
		}

		return n, nil
	case EdmDouble:
		f, ok := p.Value.(float64)
		if !ok {
			return nil, fmt.Errorf(
				"%w: Edm.Double property has non-float64 value %T", ErrInvalidEntityProperty, p.Value,
			)
		}

		return f, nil
	case EdmBoolean:
		fallthrough
	default:
		b, ok := p.Value.(bool)
		if !ok {
			return nil, fmt.Errorf("%w: Edm.Boolean property has non-bool value %T", ErrInvalidEntityProperty, p.Value)
		}

		return b, nil
	}
}

// marshalStringEncodedValue encodes the EDM types whose snapshot wire value
// is a string rather than encoding/json's native decode for that Go type
// (see unmarshalStringEncodedValue's own doc comment for the matching decode
// side).
func (p EntityProperty) marshalStringEncodedValue() (any, error) {
	switch p.Type {
	case EdmBinary:
		b, ok := p.Value.([]byte)
		if !ok {
			return nil, fmt.Errorf("%w: Edm.Binary property has non-[]byte value %T", ErrInvalidEntityProperty, p.Value)
		}

		return base64.StdEncoding.EncodeToString(b), nil
	case EdmDateTime:
		t, ok := p.Value.(time.Time)
		if !ok {
			return nil, fmt.Errorf(
				"%w: Edm.DateTime property has non-time.Time value %T", ErrInvalidEntityProperty, p.Value,
			)
		}

		return t.UTC().Format(time.RFC3339Nano), nil
	case EdmInt64:
		n, ok := p.Value.(int64)
		if !ok {
			return nil, fmt.Errorf("%w: Edm.Int64 property has non-int64 value %T", ErrInvalidEntityProperty, p.Value)
		}

		// Encoded as a decimal string, not a bare JSON number: float64 has
		// only a 53-bit mantissa, so round-tripping an Int64 through a JSON
		// number silently corrupts any value outside [-2^53, 2^53] (e.g.
		// 9007199254740993 becomes 9007199254740992). This mirrors the
		// OData wire format itself, which also encodes Edm.Int64 as a
		// string alongside its "@odata.type" annotation -- see
		// entity_ops.go's decodeStringEncodedProperty/encodePropertyInto --
		// so the snapshot and wire encodings are now consistent.
		return strconv.FormatInt(n, 10), nil
	case EdmGUID:
		fallthrough
	default:
		s, ok := p.Value.(string)
		if !ok {
			return nil, fmt.Errorf("%w: Edm.Guid property has non-string value %T", ErrInvalidEntityProperty, p.Value)
		}

		return s, nil
	}
}

// UnmarshalJSON implements json.Unmarshaler for persistence snapshots. See
// MarshalJSON's doc comment for why this can't just rely on encoding/json's
// default `any` decoding. A malformed or type-mismatched snapshot value is a
// real error, never silently zeroed -- a snapshot that can't be decoded
// exactly must not be decoded approximately.
func (p *EntityProperty) UnmarshalJSON(data []byte) error {
	var wire entityPropertyWire

	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}

	value, err := unmarshalPropertyValue(wire.Type, wire.Value)
	if err != nil {
		return err
	}

	p.Type, p.Value = wire.Type, value

	return nil
}

// unmarshalPropertyValue decodes wireValue (the generic `any` encoding/json
// produced for entityPropertyWire.Value) into its typed Go value per
// edmType.
func unmarshalPropertyValue(edmType EdmType, wireValue any) (any, error) {
	switch edmType {
	case EdmInt32, EdmDouble, EdmBoolean:
		return unmarshalScalarValue(edmType, wireValue)
	case EdmInt64, EdmDateTime, EdmGUID, EdmBinary:
		return unmarshalStringEncodedValue(edmType, wireValue)
	case EdmString:
		fallthrough
	default:
		s, ok := wireValue.(string)
		if !ok {
			return nil, fmt.Errorf(
				"%w: Edm.String snapshot value is not a string: %v",
				ErrInvalidEntityProperty,
				wireValue,
			)
		}

		return s, nil
	}
}

// unmarshalScalarValue decodes the three EDM types whose snapshot wire value
// is already the JSON-native type encoding/json's generic `any` decode
// produces (a bare JSON number or bool never needed re-encoding on the
// Marshal side -- see marshalValue).
func unmarshalScalarValue(edmType EdmType, wireValue any) (any, error) {
	switch edmType {
	case EdmInt32:
		f, ok := wireValue.(float64)
		if !ok {
			return nil, fmt.Errorf(
				"%w: Edm.Int32 snapshot value is not a number: %v",
				ErrInvalidEntityProperty,
				wireValue,
			)
		}

		if f != math.Trunc(f) || f < math.MinInt32 || f > math.MaxInt32 {
			return nil, fmt.Errorf(
				"%w: Edm.Int32 snapshot value out of range or fractional: %v",
				ErrInvalidEntityProperty,
				wireValue,
			)
		}

		return int32(f), nil
	case EdmDouble:
		f, ok := wireValue.(float64)
		if !ok {
			return nil, fmt.Errorf(
				"%w: Edm.Double snapshot value is not a number: %v",
				ErrInvalidEntityProperty,
				wireValue,
			)
		}

		return f, nil
	case EdmBoolean:
		b, ok := wireValue.(bool)
		if !ok {
			return nil, fmt.Errorf(
				"%w: Edm.Boolean snapshot value is not a bool: %v",
				ErrInvalidEntityProperty,
				wireValue,
			)
		}

		return b, nil
	default:
		return nil, fmt.Errorf("%w: unsupported scalar EdmType %q", ErrInvalidEntityProperty, edmType)
	}
}

// unmarshalStringEncodedValue decodes the four EDM types marshalValue always
// re-encodes as a JSON string (Int64 as decimal digits, DateTime as
// RFC3339Nano, Guid as its canonical string form, Binary as base64).
func unmarshalStringEncodedValue(edmType EdmType, wireValue any) (any, error) {
	s, ok := wireValue.(string)
	if !ok {
		return nil, fmt.Errorf(
			"%w: %s snapshot value is not a string: %v",
			ErrInvalidEntityProperty,
			edmType,
			wireValue,
		)
	}

	switch edmType {
	case EdmInt64:
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid Edm.Int64 snapshot value %q: %w", ErrInvalidEntityProperty, s, err)
		}

		return n, nil
	case EdmDateTime:
		t, err := time.Parse(time.RFC3339Nano, s)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid Edm.DateTime snapshot value %q: %w", ErrInvalidEntityProperty, s, err)
		}

		return t.UTC(), nil
	case EdmGUID:
		return s, nil
	case EdmBinary:
		b, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid Edm.Binary snapshot value: %w", ErrInvalidEntityProperty, err)
		}

		return b, nil
	default:
		return nil, fmt.Errorf("%w: unsupported string-encoded EdmType %q", ErrInvalidEntityProperty, edmType)
	}
}

// TableInfo is a read-only snapshot of a table's metadata, returned by
// StorageBackend.ListTables.
type TableInfo struct {
	Name string
}

// EntityInfo is a read-only snapshot of an entity, returned by the
// StorageBackend entity accessors. Properties excludes the system properties
// (PartitionKey, RowKey, Timestamp), which are surfaced via their own
// fields.
type EntityInfo struct {
	Timestamp    time.Time
	Properties   map[string]EntityProperty
	PartitionKey string
	RowKey       string
	ETag         string
}

// storedEntity is the backend's internal representation of an entity.
type storedEntity struct {
	Timestamp    time.Time
	Properties   map[string]EntityProperty
	PartitionKey string
	RowKey       string
}

// storedTable is the backend's internal representation of a table.
type storedTable struct {
	Entities map[entityCompositeKey]*storedEntity
	Name     string
}

// entityCompositeKey is an entity's map key within storedTable.Entities: a
// comparable struct of its two identifying fields, not a delimited string.
// A delimited string (e.g. partitionKey+"\x00"+rowKey, this package's
// original approach) is unsafe: JSON permits any byte value -- including
// NUL -- inside a string property, so two different (PartitionKey, RowKey)
// pairs could produce the same delimited string
// (partitionKey="a\x00b",rowKey="c" collides with partitionKey="a",
// rowKey="b\x00c"), silently rejecting a legitimate insert as a duplicate,
// or making Get/Replace/Delete operate on the wrong entity. A struct key
// compares its two fields independently, so no such collision is possible.
type entityCompositeKey struct {
	PartitionKey string
	RowKey       string
}

// MarshalText implements encoding.TextMarshaler so entityCompositeKey can be
// used as a map key in backendSnapshot's persisted storedTable.Entities
// (encoding/json only accepts string, integer, or TextMarshaler-implementing
// map key types). The two fields are marshaled as a JSON string array rather
// than delimited by a separator character: unlike a delimiter, nested JSON
// string encoding is unambiguous for every possible PartitionKey/RowKey
// value (including one containing a literal '"' or NUL byte), so this
// encoding can't reintroduce the same collision class the struct key itself
// was introduced to close.
func (k entityCompositeKey) MarshalText() ([]byte, error) {
	return json.Marshal([2]string{k.PartitionKey, k.RowKey})
}

// UnmarshalText implements encoding.TextUnmarshaler, the inverse of
// MarshalText.
func (k *entityCompositeKey) UnmarshalText(text []byte) error {
	var pair [2]string

	if err := json.Unmarshal(text, &pair); err != nil {
		return fmt.Errorf("azuretable: malformed entity composite key %q: %w", text, err)
	}

	k.PartitionKey, k.RowKey = pair[0], pair[1]

	return nil
}

// --- OData wire error envelope ---
//
// {"odata.error":{"code":"TableNotFound","message":{"lang":"en-US","value":"..."}}}

type odataErrorMessage struct {
	Lang  string `json:"lang"`
	Value string `json:"value"`
}

type odataErrorDetail struct {
	Message odataErrorMessage `json:"message"`
	Code    string            `json:"code"`
}

type odataErrorEnvelope struct {
	Error odataErrorDetail `json:"odata.error"`
}

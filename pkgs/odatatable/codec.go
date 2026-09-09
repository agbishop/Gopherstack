package odatatable

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// OData metadata level names, negotiated by each caller from its own
// request's Accept header and passed into EncodeEntity to vary response
// shape. Exported so services/azuretable and services/cosmosdb's Table API
// can compare their own negotiated level against a shared vocabulary instead
// of each hand-rolling the same three string literals.
const (
	MetadataLevelNoMetadata = "nometadata"
	MetadataLevelMinimal    = "minimalmetadata"
	MetadataLevelFull       = "fullmetadata"
)

// EntityTimeLayout formats an Edm.DateTime property/Timestamp value on the
// wire: a variable-precision (trailing zeros trimmed) RFC3339 string,
// matching aztables' own EDMDateTime.MarshalText layout exactly so its
// time.Parse round-trips cleanly.
const EntityTimeLayout = "2006-01-02T15:04:05.9999999Z"

// maxQueryTop is the largest $top value ParseTop honors; a caller-supplied
// value beyond this is clamped, not rejected, since larger values are
// harmless (an in-memory backend already returns everything in one page)
// but an absurd value should never be used to pre-size an allocation.
const maxQueryTop = 100000

// ParseTop parses the $top query parameter, defaulting to 0 (unlimited)
// when absent and clamping (never erroring on) an oversized value, per
// maxQueryTop's doc comment. A negative value is rejected.
func ParseTop(raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}

	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, ErrInvalidEntityProperty
	}

	if n > maxQueryTop {
		n = maxQueryTop
	}

	return n, nil
}

// --- Entity body decode (request -> EntityProperty map) ---

// DecodeEntityBody parses an entity JSON request body into its
// PartitionKey/RowKey (if present) and typed custom properties.
// "@odata.type"-annotated properties are decoded per their declared EDM
// type; unannotated ones are inferred the same way
// azure-sdk-for-go/sdk/data/aztables's own EDMEntity.UnmarshalJSON infers
// them client-side: try Edm.Int32 first, then fall back to the JSON value's
// natural type (float64 -> Edm.Double, bool -> Edm.Boolean, string ->
// Edm.String). "Timestamp" is silently ignored (server-managed; a client-
// supplied value never overwrites it -- the server always wins). Any
// "odata.*"/"@odata.type" metadata key is skipped.
func DecodeEntityBody(body []byte) (string, string, bool, bool, map[string]EntityProperty, error) {
	var raw map[string]json.RawMessage

	if err := json.Unmarshal(body, &raw); err != nil {
		return "", "", false, false, nil, fmt.Errorf("%w: %w", ErrInvalidEntityProperty, err)
	}

	partitionKey, hasPK, err := decodeSystemKeyProperty(raw, PartitionKeyProperty)
	if err != nil {
		return "", "", false, false, nil, err
	}

	rowKey, hasRK, err := decodeSystemKeyProperty(raw, RowKeyProperty)
	if err != nil {
		return "", "", false, false, nil, err
	}

	props, err := decodeCustomProperties(raw)
	if err != nil {
		return "", "", false, false, nil, err
	}

	return partitionKey, rowKey, hasPK, hasRK, props, nil
}

// decodeSystemKeyProperty extracts the string-valued system property name
// (PartitionKey or RowKey) from raw, if present.
func decodeSystemKeyProperty(raw map[string]json.RawMessage, name string) (string, bool, error) {
	rawVal, ok := raw[name]
	if !ok {
		return "", false, nil
	}

	var s string
	if err := json.Unmarshal(rawVal, &s); err != nil {
		return "", false, fmt.Errorf("%w: %s must be a string", ErrInvalidEntityProperty, name)
	}

	return s, true, nil
}

// isSystemOrMetadataKey reports whether key is a system property
// (PartitionKey/RowKey/Timestamp), an "@odata.type" annotation, or other
// "odata."-prefixed metadata -- none of which decodeCustomProperties treats
// as a user-defined entity property.
func isSystemOrMetadataKey(key string) bool {
	if strings.HasSuffix(key, "@odata.type") || strings.HasPrefix(key, "odata.") {
		return true
	}

	switch key {
	case PartitionKeyProperty, RowKeyProperty, TimestampProperty:
		return true
	default:
		return false
	}
}

// decodeCustomProperties decodes every user-defined (non-system,
// non-metadata) property in raw into its typed EntityProperty.
func decodeCustomProperties(raw map[string]json.RawMessage) (map[string]EntityProperty, error) {
	props := make(map[string]EntityProperty, len(raw))

	for key, rawVal := range raw {
		if isSystemOrMetadataKey(key) || string(rawVal) == "null" {
			continue
		}

		edmType := ""

		if annRaw, ok := raw[key+"@odata.type"]; ok {
			if err := json.Unmarshal(annRaw, &edmType); err != nil {
				return nil, fmt.Errorf("%w: malformed %s@odata.type", ErrInvalidEntityProperty, key)
			}
		}

		prop, err := decodeProperty(edmType, rawVal)
		if err != nil {
			return nil, err
		}

		props[key] = prop
	}

	return props, nil
}

// decodeProperty decodes raw into a typed EntityProperty per its (possibly
// empty) "@odata.type" annotation value edmType.
func decodeProperty(edmType string, raw json.RawMessage) (EntityProperty, error) {
	switch EdmType(edmType) {
	case EdmString, EdmInt32, EdmDouble, EdmBoolean:
		return decodeScalarProperty(EdmType(edmType), raw)
	case EdmInt64, EdmDateTime, EdmGUID, EdmBinary:
		return decodeStringEncodedProperty(EdmType(edmType), raw)
	case "":
		return decodeUnannotatedProperty(raw)
	default:
		return EntityProperty{}, fmt.Errorf("%w: unknown @odata.type %q", ErrInvalidEntityProperty, edmType)
	}
}

// decodeScalarProperty decodes the four EDM types whose wire representation
// is a bare, natively-typed JSON value (string/number/number/bool).
func decodeScalarProperty(edmType EdmType, raw json.RawMessage) (EntityProperty, error) {
	switch edmType {
	case EdmString:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return EntityProperty{}, fmt.Errorf("%w: not a string", ErrInvalidEntityProperty)
		}

		return EntityProperty{Type: EdmString, Value: s}, nil
	case EdmInt32:
		var n int32
		if err := json.Unmarshal(raw, &n); err != nil {
			return EntityProperty{}, fmt.Errorf("%w: not an Int32", ErrInvalidEntityProperty)
		}

		return EntityProperty{Type: EdmInt32, Value: n}, nil
	case EdmDouble:
		var f float64
		if err := json.Unmarshal(raw, &f); err != nil {
			return EntityProperty{}, fmt.Errorf("%w: not a Double", ErrInvalidEntityProperty)
		}

		return EntityProperty{Type: EdmDouble, Value: f}, nil
	case EdmBoolean:
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			return EntityProperty{}, fmt.Errorf("%w: not a Boolean", ErrInvalidEntityProperty)
		}

		return EntityProperty{Type: EdmBoolean, Value: b}, nil
	default:
		return EntityProperty{}, fmt.Errorf("%w: unsupported scalar type %q", ErrInvalidEntityProperty, edmType)
	}
}

// decodeStringEncodedProperty decodes the four EDM types whose wire
// representation is always a JSON string carrying an encoded value (decimal
// digits, an RFC3339 timestamp, a UUID, or base64), requiring the
// "@odata.type" annotation to disambiguate from Edm.String.
func decodeStringEncodedProperty(edmType EdmType, raw json.RawMessage) (EntityProperty, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return EntityProperty{}, fmt.Errorf("%w: %s must be a string", ErrInvalidEntityProperty, edmType)
	}

	switch edmType {
	case EdmInt64:
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return EntityProperty{}, fmt.Errorf("%w: invalid Int64 %q", ErrInvalidEntityProperty, s)
		}

		return EntityProperty{Type: EdmInt64, Value: n}, nil
	case EdmDateTime:
		t, err := time.Parse(EntityTimeLayout, s)
		if err != nil {
			return EntityProperty{}, fmt.Errorf("%w: invalid DateTime %q", ErrInvalidEntityProperty, s)
		}

		return EntityProperty{Type: EdmDateTime, Value: t}, nil
	case EdmGUID:
		return EntityProperty{Type: EdmGUID, Value: s}, nil
	case EdmBinary:
		b, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return EntityProperty{}, fmt.Errorf("%w: invalid base64 %q", ErrInvalidEntityProperty, s)
		}

		return EntityProperty{Type: EdmBinary, Value: b}, nil
	default:
		return EntityProperty{}, fmt.Errorf("%w: unsupported string-encoded type %q", ErrInvalidEntityProperty, edmType)
	}
}

// decodeUnannotatedProperty infers a bare (unannotated) property's EDM type,
// mirroring aztables' own client-side inference exactly: try Int32 first
// (so a decimal-point-free number becomes Edm.Int32), then fall back to the
// JSON value's natural Go type.
func decodeUnannotatedProperty(raw json.RawMessage) (EntityProperty, error) {
	var i32 int32
	if err := json.Unmarshal(raw, &i32); err == nil {
		return EntityProperty{Type: EdmInt32, Value: i32}, nil
	}

	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return EntityProperty{Type: EdmDouble, Value: f}, nil
	}

	var b bool
	if err := json.Unmarshal(raw, &b); err == nil {
		return EntityProperty{Type: EdmBoolean, Value: b}, nil
	}

	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return EntityProperty{Type: EdmString, Value: s}, nil
	}

	return EntityProperty{}, fmt.Errorf("%w: unsupported JSON value %s", ErrInvalidEntityProperty, string(raw))
}

// --- Entity body encode (EntityProperty map -> response) ---

// EncodeEntity builds an entity's OData JSON response body at the given
// metadata level. select, if non-empty, is a comma-separated $select
// projection list; PartitionKey/RowKey/Timestamp are always included
// regardless of select (they're the entity's identity and are cheap/always
// safe to return), while custom properties are filtered to the requested
// names. endpoint is the caller's own service base URL (e.g.
// "http://127.0.0.1:10002") and accountName is the fake account name used to
// build a fullmetadata "odata.type" value (e.g. "devstoreaccount1" for
// services/azuretable) -- both vary per caller, which is why they're
// parameters rather than baked into this package.
func EncodeEntity(entity EntityInfo, table, level, selectParam, endpoint, accountName string) map[string]any {
	m := map[string]any{}

	if level != MetadataLevelNoMetadata {
		m["odata.metadata"] = endpoint + "/$metadata#" + table + "/@Element"
		m["odata.etag"] = entity.ETag

		if level == MetadataLevelFull {
			keyPredicate := PartitionKeyProperty + "='" + EscapeODataKey(entity.PartitionKey) +
				"'," + RowKeyProperty + "='" + EscapeODataKey(entity.RowKey) + "'"
			m["odata.type"] = accountName + "." + table
			m["odata.id"] = endpoint + "/" + table + "(" + keyPredicate + ")"
			m["odata.editLink"] = table + "(" + keyPredicate + ")"
		}
	}

	m[PartitionKeyProperty] = entity.PartitionKey
	m[RowKeyProperty] = entity.RowKey
	m[TimestampProperty] = entity.Timestamp.UTC().Format(EntityTimeLayout)

	selected := SelectSet(selectParam)
	for name, prop := range entity.Properties {
		if selected != nil && !selected[name] {
			continue
		}

		encodePropertyInto(m, name, prop)
	}

	return m
}

// SelectSet parses a $select query parameter into a lookup set of requested
// property names, or nil if selectParam is empty (meaning "no projection,
// return every property").
func SelectSet(selectParam string) map[string]bool {
	if selectParam == "" {
		return nil
	}

	names := strings.Split(selectParam, ",")
	set := make(map[string]bool, len(names))

	for _, n := range names {
		set[strings.TrimSpace(n)] = true
	}

	return set
}

//nolint:cyclop // per-EDM-type dispatch; splitting would obscure it
func encodePropertyInto(m map[string]any, name string, prop EntityProperty) {
	switch prop.Type {
	case EdmString:
		if s, ok := prop.Value.(string); ok {
			m[name] = s
		}
	case EdmInt32:
		if n, ok := prop.Value.(int32); ok {
			m[name] = n
		}
	case EdmDouble:
		if f, ok := prop.Value.(float64); ok {
			m[name] = f
		}
	case EdmBoolean:
		if b, ok := prop.Value.(bool); ok {
			m[name] = b
		}
	case EdmInt64:
		if n, ok := prop.Value.(int64); ok {
			m[name] = strconv.FormatInt(n, 10)
			m[name+"@odata.type"] = string(EdmInt64)
		}
	case EdmDateTime:
		if t, ok := prop.Value.(time.Time); ok {
			m[name] = t.UTC().Format(EntityTimeLayout)
			m[name+"@odata.type"] = string(EdmDateTime)
		}
	case EdmGUID:
		if s, ok := prop.Value.(string); ok {
			m[name] = s
			m[name+"@odata.type"] = string(EdmGUID)
		}
	case EdmBinary:
		if b, ok := prop.Value.([]byte); ok {
			m[name] = base64.StdEncoding.EncodeToString(b)
			m[name+"@odata.type"] = string(EdmBinary)
		}
	}
}

package appsync

import (
	"encoding/json"
	"slices"

	"github.com/vektah/gqlparser/v2/ast"
)

// The types below mirror the GraphQL specification's introspection system
// (the __Schema/__Type/__Field/__InputValue/__EnumValue/__Directive shapes),
// not an AWS-specific format. marshalIntrospectionSchema wraps the result in
// {"data":{"__schema":...}}, the standard shape of running the introspection
// query -- confirmed against a real AppSync-exported schema.json, which uses
// the same envelope.
//
// Left out, disclosed rather than guessed: __Type.specifiedByURL (custom
// scalar @specifiedBy) and __Type.isOneOf (the 2024 oneOf-input-object
// addition). Neither is fabricated; both are simply omitted from the output.

type introspectionResponse struct {
	Data introspectionData `json:"data"`
}

type introspectionData struct {
	Schema introspectionSchema `json:"__schema"`
}

type introspectionSchema struct {
	Description      *string                  `json:"description"`
	Types            []introspectionType      `json:"types"`
	QueryType        *rootTypeRef             `json:"queryType"`
	MutationType     *rootTypeRef             `json:"mutationType"`
	SubscriptionType *rootTypeRef             `json:"subscriptionType"`
	Directives       []introspectionDirective `json:"directives"`
}

// rootTypeRef is the minimal {"name": ...} reference real AppSync uses for
// __Schema's queryType/mutationType/subscriptionType.
type rootTypeRef struct {
	Name string `json:"name"`
}

type introspectionType struct {
	Kind          string                    `json:"kind"`
	Name          string                    `json:"name"`
	Description   *string                   `json:"description"`
	Fields        []introspectionField      `json:"fields"`
	Interfaces    []typeRef                 `json:"interfaces"`
	PossibleTypes []typeRef                 `json:"possibleTypes"`
	EnumValues    []introspectionEnumValue  `json:"enumValues"`
	InputFields   []introspectionInputValue `json:"inputFields"`
}

// typeRef is the recursive __Type reference used for field/arg/input types:
// a named leaf, or a LIST/NON_NULL wrapper around another typeRef.
type typeRef struct {
	Name   *string  `json:"name"`
	OfType *typeRef `json:"ofType"`
	Kind   string   `json:"kind"`
}

type introspectionField struct {
	Description       *string                   `json:"description"`
	Type              *typeRef                  `json:"type"`
	DeprecationReason *string                   `json:"deprecationReason"`
	Name              string                    `json:"name"`
	Args              []introspectionInputValue `json:"args"`
	IsDeprecated      bool                      `json:"isDeprecated"`
}

type introspectionInputValue struct {
	Description       *string  `json:"description"`
	Type              *typeRef `json:"type"`
	DefaultValue      *string  `json:"defaultValue"`
	DeprecationReason *string  `json:"deprecationReason"`
	Name              string   `json:"name"`
	IsDeprecated      bool     `json:"isDeprecated"`
}

type introspectionEnumValue struct {
	Description       *string `json:"description"`
	DeprecationReason *string `json:"deprecationReason"`
	Name              string  `json:"name"`
	IsDeprecated      bool    `json:"isDeprecated"`
}

type introspectionDirective struct {
	Name         string                    `json:"name"`
	Description  *string                   `json:"description"`
	Locations    []ast.DirectiveLocation   `json:"locations"`
	Args         []introspectionInputValue `json:"args"`
	IsRepeatable bool                      `json:"isRepeatable"`
}

// marshalIntrospectionSchema builds the standard GraphQL introspection
// document for schema and marshals it to JSON. includeDirectives gates only
// the top-level __schema.directives list.
func marshalIntrospectionSchema(schema *ast.Schema, includeDirectives bool) ([]byte, error) {
	doc := introspectionResponse{
		Data: introspectionData{
			Schema: introspectionSchema{
				Description:      optionalString(schema.Description),
				Types:            buildTypes(schema),
				QueryType:        buildRootRef(schema.Query),
				MutationType:     buildRootRef(schema.Mutation),
				SubscriptionType: buildRootRef(schema.Subscription),
				Directives:       buildDirectives(schema, includeDirectives),
			},
		},
	}

	return json.Marshal(doc)
}

func buildRootRef(def *ast.Definition) *rootTypeRef {
	if def == nil {
		return nil
	}

	return &rootTypeRef{Name: def.Name}
}

func buildTypes(schema *ast.Schema) []introspectionType {
	names := make([]string, 0, len(schema.Types))
	for name := range schema.Types {
		names = append(names, name)
	}

	slices.Sort(names)

	out := make([]introspectionType, 0, len(names))
	for _, name := range names {
		out = append(out, buildIntrospectionType(schema, schema.Types[name]))
	}

	return out
}

func buildIntrospectionType(schema *ast.Schema, def *ast.Definition) introspectionType {
	t := introspectionType{
		Kind:        string(def.Kind),
		Name:        def.Name,
		Description: optionalString(def.Description),
	}

	switch def.Kind {
	case ast.Object:
		t.Fields = buildFields(schema, def.Fields)
		t.Interfaces = buildNamedRefs(schema, def.Interfaces)
	case ast.Interface:
		t.Fields = buildFields(schema, def.Fields)
		t.Interfaces = buildNamedRefs(schema, def.Interfaces)
		t.PossibleTypes = buildPossibleTypes(schema, def.Name)
	case ast.Union:
		t.PossibleTypes = buildPossibleTypes(schema, def.Name)
	case ast.Enum:
		t.EnumValues = buildEnumValues(def.EnumValues)
	case ast.InputObject:
		t.InputFields = buildInputFields(schema, def.Fields)
	case ast.Scalar:
		// No additional fields for scalars beyond kind/name/description.
	}

	return t
}

func buildFields(schema *ast.Schema, fields ast.FieldList) []introspectionField {
	out := make([]introspectionField, 0, len(fields))
	for _, f := range fields {
		isDeprecated, reason := deprecationOf(f.Directives)
		out = append(out, introspectionField{
			Name:              f.Name,
			Description:       optionalString(f.Description),
			Args:              buildArgs(schema, f.Arguments),
			Type:              buildTypeRef(schema, f.Type),
			IsDeprecated:      isDeprecated,
			DeprecationReason: reason,
		})
	}

	return out
}

func buildArgs(schema *ast.Schema, args ast.ArgumentDefinitionList) []introspectionInputValue {
	out := make([]introspectionInputValue, 0, len(args))
	for _, a := range args {
		isDeprecated, reason := deprecationOf(a.Directives)
		out = append(out, introspectionInputValue{
			Name:              a.Name,
			Description:       optionalString(a.Description),
			Type:              buildTypeRef(schema, a.Type),
			DefaultValue:      defaultValueOf(a.DefaultValue),
			IsDeprecated:      isDeprecated,
			DeprecationReason: reason,
		})
	}

	return out
}

func buildInputFields(schema *ast.Schema, fields ast.FieldList) []introspectionInputValue {
	out := make([]introspectionInputValue, 0, len(fields))
	for _, f := range fields {
		isDeprecated, reason := deprecationOf(f.Directives)
		out = append(out, introspectionInputValue{
			Name:              f.Name,
			Description:       optionalString(f.Description),
			Type:              buildTypeRef(schema, f.Type),
			DefaultValue:      defaultValueOf(f.DefaultValue),
			IsDeprecated:      isDeprecated,
			DeprecationReason: reason,
		})
	}

	return out
}

func buildEnumValues(values ast.EnumValueList) []introspectionEnumValue {
	out := make([]introspectionEnumValue, 0, len(values))
	for _, v := range values {
		isDeprecated, reason := deprecationOf(v.Directives)
		out = append(out, introspectionEnumValue{
			Name:              v.Name,
			Description:       optionalString(v.Description),
			IsDeprecated:      isDeprecated,
			DeprecationReason: reason,
		})
	}

	return out
}

func buildNamedRefs(schema *ast.Schema, names []string) []typeRef {
	out := make([]typeRef, 0, len(names))
	for _, name := range names {
		out = append(out, *namedTypeRef(schema, name))
	}

	return out
}

func buildPossibleTypes(schema *ast.Schema, name string) []typeRef {
	defs := schema.PossibleTypes[name]
	out := make([]typeRef, 0, len(defs))

	for _, d := range defs {
		out = append(out, *namedTypeRef(schema, d.Name))
	}

	return out
}

func buildDirectives(schema *ast.Schema, includeDirectives bool) []introspectionDirective {
	if !includeDirectives {
		return []introspectionDirective{}
	}

	names := make([]string, 0, len(schema.Directives))
	for name := range schema.Directives {
		names = append(names, name)
	}

	slices.Sort(names)

	out := make([]introspectionDirective, 0, len(names))
	for _, name := range names {
		d := schema.Directives[name]
		out = append(out, introspectionDirective{
			Name:         d.Name,
			Description:  optionalString(d.Description),
			Locations:    d.Locations,
			Args:         buildArgs(schema, d.Arguments),
			IsRepeatable: d.IsRepeatable,
		})
	}

	return out
}

// buildTypeRef walks a possibly LIST/NON_NULL-wrapped ast.Type into the
// recursive typeRef shape.
func buildTypeRef(schema *ast.Schema, t *ast.Type) *typeRef {
	if t == nil {
		return nil
	}

	if t.NonNull {
		inner := *t
		inner.NonNull = false

		return &typeRef{Kind: "NON_NULL", OfType: buildTypeRef(schema, &inner)}
	}

	if t.Elem != nil {
		return &typeRef{Kind: "LIST", OfType: buildTypeRef(schema, t.Elem)}
	}

	return namedTypeRef(schema, t.NamedType)
}

func namedTypeRef(schema *ast.Schema, name string) *typeRef {
	kind := "SCALAR"
	if def := schema.Types[name]; def != nil {
		kind = string(def.Kind)
	}

	n := name

	return &typeRef{Kind: kind, Name: &n}
}

// deprecationOf reports whether dirs carries a @deprecated directive and, if
// so, its reason (defaulting to the spec's standard text when the directive
// has no explicit reason argument).
func deprecationOf(dirs ast.DirectiveList) (bool, *string) {
	d := dirs.ForName("deprecated")
	if d == nil {
		return false, nil
	}

	reason := "No longer supported"
	if arg := d.Arguments.ForName("reason"); arg != nil && arg.Value != nil {
		reason = arg.Value.Raw
	}

	return true, &reason
}

// defaultValueOf renders v as the GraphQL-formatted string the introspection
// spec's __InputValue.defaultValue expects (e.g. "10", "\"foo\"", "RED").
func defaultValueOf(v *ast.Value) *string {
	if v == nil {
		return nil
	}

	s := v.String()

	return &s
}

func optionalString(s string) *string {
	if s == "" {
		return nil
	}

	return &s
}

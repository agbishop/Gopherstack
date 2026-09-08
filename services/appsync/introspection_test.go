package appsync_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/appsync"
)

const introspectionTestSDL = `
interface Node {
  id: ID!
}

enum Status {
  ACTIVE
  INACTIVE @deprecated(reason: "no longer used")
}

input WidgetFilter {
  name: String
  status: Status
}

type Widget implements Node {
  id: ID!
  name: String
  status: Status
  tags: [String!]
}

type Query {
  widget(id: ID!): Widget
  widgets(filter: WidgetFilter): [Widget!]!
}
`

type testTypeRef struct {
	Name   *string      `json:"name"`
	OfType *testTypeRef `json:"ofType"`
	Kind   string       `json:"kind"`
}

type testInputValue struct {
	Type *testTypeRef `json:"type"`
	Name string       `json:"name"`
}

type testField struct {
	Type *testTypeRef     `json:"type"`
	Name string           `json:"name"`
	Args []testInputValue `json:"args"`
}

type testEnumValue struct {
	DeprecationReason *string `json:"deprecationReason"`
	Name              string  `json:"name"`
	IsDeprecated      bool    `json:"isDeprecated"`
}

type testType struct {
	Kind          string           `json:"kind"`
	Name          string           `json:"name"`
	Fields        []testField      `json:"fields"`
	Interfaces    []testTypeRef    `json:"interfaces"`
	PossibleTypes []testTypeRef    `json:"possibleTypes"`
	EnumValues    []testEnumValue  `json:"enumValues"`
	InputFields   []testInputValue `json:"inputFields"`
}

type testDirective struct {
	Name string `json:"name"`
}

type testIntrospectionResponse struct {
	Data struct {
		Schema struct {
			QueryType *struct {
				Name string `json:"name"`
			} `json:"queryType"`
			Types      []testType      `json:"types"`
			Directives []testDirective `json:"directives"`
		} `json:"__schema"`
	} `json:"data"`
}

func typeByName(types []testType, name string) *testType {
	for i := range types {
		if types[i].Name == name {
			return &types[i]
		}
	}

	return nil
}

func fieldByName(fields []testField, name string) *testField {
	for i := range fields {
		if fields[i].Name == name {
			return &fields[i]
		}
	}

	return nil
}

func decodeIntrospection(t *testing.T, raw []byte) testIntrospectionResponse {
	t.Helper()
	require.True(t, json.Valid(raw))

	var doc testIntrospectionResponse
	require.NoError(t, json.Unmarshal(raw, &doc))

	return doc
}

func TestInMemoryBackend_GetIntrospectionSchema_JSON(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	_, err := b.StartSchemaCreation(api.APIID, introspectionTestSDL)
	require.NoError(t, err)

	raw, err := b.GetIntrospectionSchema(api.APIID, "JSON", true)
	require.NoError(t, err)

	doc := decodeIntrospection(t, raw)

	require.NotNil(t, doc.Data.Schema.QueryType)
	assert.Equal(t, "Query", doc.Data.Schema.QueryType.Name)

	widget := typeByName(doc.Data.Schema.Types, "Widget")
	require.NotNil(t, widget)
	assert.Equal(t, "OBJECT", widget.Kind)
	require.Len(t, widget.Interfaces, 1)
	assert.Equal(t, "INTERFACE", widget.Interfaces[0].Kind)
	require.NotNil(t, widget.Interfaces[0].Name)
	assert.Equal(t, "Node", *widget.Interfaces[0].Name)

	tags := fieldByName(widget.Fields, "tags")
	require.NotNil(t, tags)
	require.NotNil(t, tags.Type)
	assert.Equal(t, "LIST", tags.Type.Kind) // [String!]
	require.NotNil(t, tags.Type.OfType)
	assert.Equal(t, "NON_NULL", tags.Type.OfType.Kind)
	require.NotNil(t, tags.Type.OfType.OfType)
	assert.Equal(t, "SCALAR", tags.Type.OfType.OfType.Kind)
	require.NotNil(t, tags.Type.OfType.OfType.Name)
	assert.Equal(t, "String", *tags.Type.OfType.OfType.Name)

	node := typeByName(doc.Data.Schema.Types, "Node")
	require.NotNil(t, node)
	assert.Equal(t, "INTERFACE", node.Kind)
	require.Len(t, node.PossibleTypes, 1)
	require.NotNil(t, node.PossibleTypes[0].Name)
	assert.Equal(t, "Widget", *node.PossibleTypes[0].Name)

	status := typeByName(doc.Data.Schema.Types, "Status")
	require.NotNil(t, status)
	assert.Equal(t, "ENUM", status.Kind)
	require.Len(t, status.EnumValues, 2)

	var inactive *testEnumValue
	for i := range status.EnumValues {
		if status.EnumValues[i].Name == "INACTIVE" {
			inactive = &status.EnumValues[i]
		}
	}
	require.NotNil(t, inactive)
	assert.True(t, inactive.IsDeprecated)
	require.NotNil(t, inactive.DeprecationReason)
	assert.Equal(t, "no longer used", *inactive.DeprecationReason)

	filter := typeByName(doc.Data.Schema.Types, "WidgetFilter")
	require.NotNil(t, filter)
	assert.Equal(t, "INPUT_OBJECT", filter.Kind)
	assert.Len(t, filter.InputFields, 2)

	query := typeByName(doc.Data.Schema.Types, "Query")
	require.NotNil(t, query)

	widgetField := fieldByName(query.Fields, "widget")
	require.NotNil(t, widgetField)
	require.Len(t, widgetField.Args, 1)
	assert.Equal(t, "id", widgetField.Args[0].Name)
	require.NotNil(t, widgetField.Args[0].Type)
	assert.Equal(t, "NON_NULL", widgetField.Args[0].Type.Kind) // ID!
	require.NotNil(t, widgetField.Args[0].Type.OfType)
	assert.Equal(t, "SCALAR", widgetField.Args[0].Type.OfType.Kind)
	require.NotNil(t, widgetField.Args[0].Type.OfType.Name)
	assert.Equal(t, "ID", *widgetField.Args[0].Type.OfType.Name)

	widgetsField := fieldByName(query.Fields, "widgets")
	require.NotNil(t, widgetsField)
	require.NotNil(t, widgetsField.Type)
	assert.Equal(t, "NON_NULL", widgetsField.Type.Kind) // [Widget!]!
	require.NotNil(t, widgetsField.Type.OfType)
	assert.Equal(t, "LIST", widgetsField.Type.OfType.Kind)
	require.NotNil(t, widgetsField.Type.OfType.OfType)
	assert.Equal(t, "NON_NULL", widgetsField.Type.OfType.OfType.Kind)
	require.NotNil(t, widgetsField.Type.OfType.OfType.OfType)
	assert.Equal(t, "OBJECT", widgetsField.Type.OfType.OfType.OfType.Kind)
	require.NotNil(t, widgetsField.Type.OfType.OfType.OfType.Name)
	assert.Equal(t, "Widget", *widgetsField.Type.OfType.OfType.OfType.Name)

	var hasDeprecatedDirective bool
	for _, d := range doc.Data.Schema.Directives {
		if d.Name == "deprecated" {
			hasDeprecatedDirective = true
		}
	}
	assert.True(t, hasDeprecatedDirective, "includeDirectives=true should list the standard @deprecated directive")
}

func TestInMemoryBackend_GetIntrospectionSchema_JSON_IncludeDirectivesFalse(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	_, err := b.StartSchemaCreation(api.APIID, introspectionTestSDL)
	require.NoError(t, err)

	raw, err := b.GetIntrospectionSchema(api.APIID, "JSON", false)
	require.NoError(t, err)

	doc := decodeIntrospection(t, raw)

	assert.Empty(t, doc.Data.Schema.Directives)
}

func TestInMemoryBackend_GetIntrospectionSchema_SDLUnaffectedByIncludeDirectives(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	_, err := b.StartSchemaCreation(api.APIID, introspectionTestSDL)
	require.NoError(t, err)

	withDirectives, err := b.GetIntrospectionSchema(api.APIID, "SDL", true)
	require.NoError(t, err)

	withoutDirectives, err := b.GetIntrospectionSchema(api.APIID, "SDL", false)
	require.NoError(t, err)

	assert.Equal(t, introspectionTestSDL, string(withDirectives))
	assert.Equal(t, string(withDirectives), string(withoutDirectives))
}

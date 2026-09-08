package appsync_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/appsync"
)

// TestInMemoryBackend_ExecuteGraphQL_JSCodeResolver verifies that a UNIT
// resolver configured with APPSYNC_JS Code (instead of VTL mapping
// templates) actually runs its request/response handlers during query
// execution, rather than being silently treated as having no mapping at all
// (gopherstack-ivwh).
func TestInMemoryBackend_ExecuteGraphQL_JSCodeResolver(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantValue     any
		name          string
		dsType        appsync.DataSourceType
		code          string
		wantField     string
		lambdaPayload []byte
	}{
		{
			name:   "lambda_js_request_and_response",
			dsType: appsync.DataSourceTypeLambda,
			code: `export function request(ctx) {
  return { payload: ctx.arguments };
}
export function response(ctx) {
  return ctx.result;
}`,
			lambdaPayload: []byte(`"Hello, Alice"`),
			wantField:     "greet",
			wantValue:     "Hello, Alice",
		},
		{
			name:   "none_js_passthrough",
			dsType: appsync.DataSourceTypeNone,
			code: `export function request(ctx) {
  return ctx.arguments;
}
export function response(ctx) {
  return ctx.result;
}`,
			wantField: "greet",
			wantValue: map[string]any{"name": "Alice"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()

			dsCfg := &appsync.DataSource{Name: "DS", Type: tt.dsType}
			if tt.dsType == appsync.DataSourceTypeLambda {
				mock := &mockLambdaInvoker{payload: tt.lambdaPayload}
				b.SetLambdaInvoker(mock)
				dsCfg.LambdaConfig = &appsync.LambdaDataSourceConfig{
					LambdaFunctionARN: "arn:aws:lambda:us-east-1:000000000000:function:greet-fn",
				}
			}

			api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
			key, keyErr := b.CreateAPIKey(api.APIID, "", 0)
			require.NoError(t, keyErr)
			auth := appsync.GraphQLAuth{APIKey: key.ID}
			_, _ = b.StartSchemaCreation(api.APIID, `type Query { greet(name: String): String }`)
			_, _ = b.CreateDataSource(api.APIID, dsCfg)
			_, _ = b.CreateResolver(api.APIID, "Query", &appsync.Resolver{
				FieldName:      "greet",
				DataSourceName: "DS",
				Code:           tt.code,
			})

			result, err := b.ExecuteGraphQL(t.Context(), api.APIID, `query { greet(name: "Alice") }`, "", nil, auth)
			require.NoError(t, err)
			assert.Equal(t, tt.wantValue, result[tt.wantField])
		})
	}
}

// TestInMemoryBackend_ExecuteGraphQL_JSCodeResolver_UnsupportedConstruct
// verifies that JS code outside the documented evaluator subset (see
// jseval.go) fails the resolution rather than silently returning an empty
// or wrong result.
func TestInMemoryBackend_ExecuteGraphQL_JSCodeResolver_UnsupportedConstruct(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	key, keyErr := b.CreateAPIKey(api.APIID, "", 0)
	require.NoError(t, keyErr)
	auth := appsync.GraphQLAuth{APIKey: key.ID}
	_, _ = b.StartSchemaCreation(api.APIID, `type Query { greet: String }`)
	_, _ = b.CreateDataSource(api.APIID, &appsync.DataSource{Name: "DS", Type: appsync.DataSourceTypeNone})
	_, _ = b.CreateResolver(api.APIID, "Query", &appsync.Resolver{
		FieldName:      "greet",
		DataSourceName: "DS",
		Code: `export function request(ctx) {
  let x = 1 + 1;
  return x;
}`,
	})

	_, err := b.ExecuteGraphQL(t.Context(), api.APIID, `query { greet }`, "", nil, auth)
	require.Error(t, err)
}

// TestInMemoryBackend_ExecuteGraphQL_PipelineResolver verifies a PIPELINE
// resolver chains its Functions in PipelineConfig order, threading each
// function's result into the next function's ctx.prev.result, and applies
// the resolver's own after-mapping (ResponseMappingTemplate) over the last
// function's result (gopherstack-ivwh).
func TestInMemoryBackend_ExecuteGraphQL_PipelineResolver(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	ddb := &mockDynamoDB{}
	b.SetDynamoDBBackend(ddb)

	api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	key, keyErr := b.CreateAPIKey(api.APIID, "", 0)
	require.NoError(t, keyErr)
	auth := appsync.GraphQLAuth{APIKey: key.ID}
	_, _ = b.StartSchemaCreation(api.APIID, `type Query { process(id: String): String }`)

	_, _ = b.CreateDataSource(api.APIID, &appsync.DataSource{
		Name:           "DDBDataSource",
		Type:           appsync.DataSourceTypeDynamoDB,
		DynamoDBConfig: &appsync.DynamoDBDataSourceConfig{TableName: "items"},
	})
	_, _ = b.CreateDataSource(api.APIID, &appsync.DataSource{Name: "NoneDS", Type: appsync.DataSourceTypeNone})

	fn1, err := b.CreateFunction(api.APIID, &appsync.Function{
		Name:                    "PutFn",
		DataSourceName:          "DDBDataSource",
		RequestMappingTemplate:  `{"operation": "PutItem", "item": {"id": "$ctx.args.id", "step": "first"}}`,
		ResponseMappingTemplate: `$util.toJson($context.result)`,
	})
	require.NoError(t, err)

	fn2, err := b.CreateFunction(api.APIID, &appsync.Function{
		Name:                    "PassthroughFn",
		DataSourceName:          "NoneDS",
		RequestMappingTemplate:  `{"prevId": "$ctx.prev.result.id"}`,
		ResponseMappingTemplate: `$util.toJson($context.result)`,
	})
	require.NoError(t, err)

	_, err = b.CreateResolver(api.APIID, "Query", &appsync.Resolver{
		FieldName:               "process",
		Kind:                    "PIPELINE",
		PipelineConfig:          []string{fn1.FunctionID, fn2.FunctionID},
		ResponseMappingTemplate: `$util.toJson($context.result)`,
	})
	require.NoError(t, err)

	result, err := b.ExecuteGraphQL(t.Context(), api.APIID, `query { process(id: "abc") }`, "", nil, auth)
	require.NoError(t, err)

	process, ok := result["process"].(map[string]any)
	require.True(t, ok, "pipeline result must be the last function's (VTL-mapped) response")
	assert.Equal(t, "abc", process["prevId"],
		"second function's request template must see the first function's result via ctx.prev.result")
}

// TestInMemoryBackend_ExecuteGraphQL_PipelineResolver_MissingFunction
// verifies a PIPELINE resolver referencing a nonexistent function ID fails
// clearly rather than silently skipping the step.
func TestInMemoryBackend_ExecuteGraphQL_PipelineResolver_MissingFunction(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	key, keyErr := b.CreateAPIKey(api.APIID, "", 0)
	require.NoError(t, keyErr)
	auth := appsync.GraphQLAuth{APIKey: key.ID}
	_, _ = b.StartSchemaCreation(api.APIID, `type Query { process: String }`)
	_, err := b.CreateResolver(api.APIID, "Query", &appsync.Resolver{
		FieldName:      "process",
		Kind:           "PIPELINE",
		PipelineConfig: []string{"func-does-not-exist"},
	})
	require.NoError(t, err)

	_, err = b.ExecuteGraphQL(t.Context(), api.APIID, `query { process }`, "", nil, auth)
	require.Error(t, err)
}

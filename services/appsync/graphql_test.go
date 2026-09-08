package appsync_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/appsync"
)

func TestInMemoryBackend_ExecuteGraphQL_LambdaResolver(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantValue     any
		name          string
		schema        string
		query         string
		wantField     string
		lambdaPayload []byte
		wantCalls     int
	}{
		{
			name:          "executes_query_via_lambda_resolver",
			schema:        `type Query { hello: String }`,
			query:         `query { hello }`,
			lambdaPayload: []byte(`"world"`),
			wantField:     "hello",
			wantValue:     "world",
			wantCalls:     1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			mock := &mockLambdaInvoker{payload: tt.lambdaPayload}
			b.SetLambdaInvoker(mock)

			api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
			key, keyErr := b.CreateAPIKey(api.APIID, "", 0)
			require.NoError(t, keyErr)
			auth := appsync.GraphQLAuth{APIKey: key.ID}
			_, _ = b.StartSchemaCreation(api.APIID, tt.schema)
			_, _ = b.CreateDataSource(api.APIID, &appsync.DataSource{
				Name: "LambdaDS",
				Type: appsync.DataSourceTypeLambda,
				LambdaConfig: &appsync.LambdaDataSourceConfig{
					LambdaFunctionARN: "arn:aws:lambda:us-east-1:000000000000:function:hello-fn",
				},
			})
			_, _ = b.CreateResolver(api.APIID, "Query", &appsync.Resolver{
				FieldName:      "hello",
				DataSourceName: "LambdaDS",
			})

			result, err := b.ExecuteGraphQL(t.Context(), api.APIID, tt.query, "", nil, auth)
			require.NoError(t, err)
			assert.Len(t, mock.calls, tt.wantCalls)
			assert.Equal(t, tt.wantValue, result[tt.wantField])
		})
	}
}

func TestInMemoryBackend_ExecuteGraphQL_NoneResolver(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		schema    string
		query     string
		variables map[string]any
		wantField string
		wantErr   bool
	}{
		{
			name:      "none_resolver_returns_args",
			schema:    `type Query { echo(message: String): String }`,
			query:     `query { echo(message: "hi") }`,
			wantField: "echo",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
			key, keyErr := b.CreateAPIKey(api.APIID, "", 0)
			require.NoError(t, keyErr)
			auth := appsync.GraphQLAuth{APIKey: key.ID}
			_, _ = b.StartSchemaCreation(api.APIID, tt.schema)
			_, _ = b.CreateDataSource(api.APIID, &appsync.DataSource{
				Name: "NoneDS",
				Type: appsync.DataSourceTypeNone,
			})
			_, _ = b.CreateResolver(api.APIID, "Query", &appsync.Resolver{
				FieldName:      "echo",
				DataSourceName: "NoneDS",
			})

			result, err := b.ExecuteGraphQL(t.Context(), api.APIID, tt.query, "", tt.variables, auth)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Contains(t, result, tt.wantField)
		})
	}
}

func TestInMemoryBackend_ExecuteGraphQL_NoSchema(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	key, keyErr := b.CreateAPIKey(api.APIID, "", 0)
	require.NoError(t, keyErr)
	auth := appsync.GraphQLAuth{APIKey: key.ID}
	_, err := b.ExecuteGraphQL(t.Context(), api.APIID, `query { hello }`, "", nil, auth)
	require.Error(t, err)
}

func TestInMemoryBackend_ExecuteGraphQL_InvalidQuery(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	key, keyErr := b.CreateAPIKey(api.APIID, "", 0)
	require.NoError(t, keyErr)
	auth := appsync.GraphQLAuth{APIKey: key.ID}
	_, _ = b.StartSchemaCreation(api.APIID, `type Query { hello: String }`)

	_, err := b.ExecuteGraphQL(t.Context(), api.APIID, `{ not valid gql`, "", nil, auth)
	require.Error(t, err)
}

func TestInMemoryBackend_ExecuteGraphQL_MissingAPI(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	_, err := b.ExecuteGraphQL(t.Context(), "nonexistent", `query { hello }`, "", nil, appsync.GraphQLAuth{})
	require.Error(t, err)
}

func TestInMemoryBackend_ExecuteGraphQL_LambdaResolver_WithTemplates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		schema        string
		query         string
		reqTemplate   string
		respTemplate  string
		wantField     string
		lambdaPayload []byte
		wantCalls     int
	}{
		{
			name:          "uses_request_template",
			schema:        `type Query { greet(name: String): String }`,
			query:         `query { greet(name: "Alice") }`,
			reqTemplate:   `{"name": "$ctx.args.name"}`,
			respTemplate:  `$util.toJson($context.result)`,
			lambdaPayload: []byte(`"Hello, Alice"`),
			wantField:     "greet",
			wantCalls:     1,
		},
		{
			name:          "no_template_passes_args",
			schema:        `type Query { greet(name: String): String }`,
			query:         `query { greet(name: "Bob") }`,
			reqTemplate:   "",
			respTemplate:  "",
			lambdaPayload: []byte(`"Hi Bob"`),
			wantField:     "greet",
			wantCalls:     1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			mock := &mockLambdaInvoker{payload: tt.lambdaPayload}
			b.SetLambdaInvoker(mock)

			api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
			key, keyErr := b.CreateAPIKey(api.APIID, "", 0)
			require.NoError(t, keyErr)
			auth := appsync.GraphQLAuth{APIKey: key.ID}
			_, _ = b.StartSchemaCreation(api.APIID, tt.schema)
			_, _ = b.CreateDataSource(api.APIID, &appsync.DataSource{
				Name: "LambdaDS",
				Type: appsync.DataSourceTypeLambda,
				LambdaConfig: &appsync.LambdaDataSourceConfig{
					LambdaFunctionARN: "arn:aws:lambda:us-east-1:000:function:fn",
				},
			})
			_, _ = b.CreateResolver(api.APIID, "Query", &appsync.Resolver{
				FieldName:               "greet",
				DataSourceName:          "LambdaDS",
				RequestMappingTemplate:  tt.reqTemplate,
				ResponseMappingTemplate: tt.respTemplate,
			})

			result, err := b.ExecuteGraphQL(t.Context(), api.APIID, tt.query, "", nil, auth)
			require.NoError(t, err)
			assert.Len(t, mock.calls, tt.wantCalls)
			assert.NotNil(t, result[tt.wantField])
		})
	}
}

func TestInMemoryBackend_ExecuteGraphQL_NoneResolver_WithTemplates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		schema       string
		query        string
		reqTemplate  string
		respTemplate string
		wantField    string
	}{
		{
			name:         "none_resolver_with_response_template",
			schema:       `type Query { echo(msg: String): String }`,
			query:        `query { echo(msg: "hello") }`,
			reqTemplate:  `{"msg": "$ctx.args.msg"}`,
			respTemplate: `$util.toJson($context.result)`,
			wantField:    "echo",
		},
		{
			name:         "none_resolver_bare_args",
			schema:       `type Query { echo(msg: String): String }`,
			query:        `query { echo(msg: "hi") }`,
			reqTemplate:  "",
			respTemplate: "",
			wantField:    "echo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
			key, keyErr := b.CreateAPIKey(api.APIID, "", 0)
			require.NoError(t, keyErr)
			auth := appsync.GraphQLAuth{APIKey: key.ID}
			_, _ = b.StartSchemaCreation(api.APIID, tt.schema)
			_, _ = b.CreateDataSource(api.APIID, &appsync.DataSource{
				Name: "NoneDS",
				Type: appsync.DataSourceTypeNone,
			})
			_, _ = b.CreateResolver(api.APIID, "Query", &appsync.Resolver{
				FieldName:               "echo",
				DataSourceName:          "NoneDS",
				RequestMappingTemplate:  tt.reqTemplate,
				ResponseMappingTemplate: tt.respTemplate,
			})

			result, err := b.ExecuteGraphQL(t.Context(), api.APIID, tt.query, "", nil, auth)
			require.NoError(t, err)
			assert.Contains(t, result, tt.wantField)
		})
	}
}

func TestInMemoryBackend_ExecuteGraphQL_Mutation(t *testing.T) {
	t.Parallel()

	schema := `type Query { dummy: String }
type Mutation { createItem(name: String): String }`
	query := `mutation { createItem(name: "test") }`

	b := newTestBackend()
	api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	key, keyErr := b.CreateAPIKey(api.APIID, "", 0)
	require.NoError(t, keyErr)
	auth := appsync.GraphQLAuth{APIKey: key.ID}
	_, _ = b.StartSchemaCreation(api.APIID, schema)
	_, _ = b.CreateDataSource(api.APIID, &appsync.DataSource{
		Name: "NoneDS",
		Type: appsync.DataSourceTypeNone,
	})
	_, _ = b.CreateResolver(api.APIID, "Mutation", &appsync.Resolver{
		FieldName:      "createItem",
		DataSourceName: "NoneDS",
	})

	result, err := b.ExecuteGraphQL(t.Context(), api.APIID, query, "", nil, auth)
	require.NoError(t, err)
	assert.Contains(t, result, "createItem")
}

func TestInMemoryBackend_ExecuteGraphQL_UnsupportedDataSource(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	key, keyErr := b.CreateAPIKey(api.APIID, "", 0)
	require.NoError(t, keyErr)
	auth := appsync.GraphQLAuth{APIKey: key.ID}
	_, _ = b.StartSchemaCreation(api.APIID, `type Query { hello: String }`)
	_, _ = b.CreateDataSource(api.APIID, &appsync.DataSource{
		Name: "HTTPDS",
		Type: appsync.DataSourceTypeHTTP,
	})
	_, _ = b.CreateResolver(api.APIID, "Query", &appsync.Resolver{
		FieldName:      "hello",
		DataSourceName: "HTTPDS",
	})

	_, err := b.ExecuteGraphQL(t.Context(), api.APIID, `query { hello }`, "", nil, auth)
	require.Error(t, err)
}

func TestInMemoryBackend_ExecuteGraphQL_NamedOperation(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	key, keyErr := b.CreateAPIKey(api.APIID, "", 0)
	require.NoError(t, keyErr)
	auth := appsync.GraphQLAuth{APIKey: key.ID}
	_, _ = b.StartSchemaCreation(api.APIID, `type Query { hello: String }`)
	_, _ = b.CreateDataSource(api.APIID, &appsync.DataSource{
		Name: "NoneDS",
		Type: appsync.DataSourceTypeNone,
	})
	_, _ = b.CreateResolver(api.APIID, "Query", &appsync.Resolver{
		FieldName:      "hello",
		DataSourceName: "NoneDS",
	})

	result, err := b.ExecuteGraphQL(t.Context(), api.APIID,
		`query MyQuery { hello }`, "MyQuery", nil, auth)
	require.NoError(t, err)
	assert.Contains(t, result, "hello")
}

func TestInMemoryBackend_ExecuteGraphQL_OperationNotFound(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	key, keyErr := b.CreateAPIKey(api.APIID, "", 0)
	require.NoError(t, keyErr)
	auth := appsync.GraphQLAuth{APIKey: key.ID}
	_, _ = b.StartSchemaCreation(api.APIID, `type Query { hello: String }`)

	_, err := b.ExecuteGraphQL(t.Context(), api.APIID,
		`query MyQuery { hello }`, "NonExistentOp", nil, auth)
	require.Error(t, err)
}

func TestInMemoryBackend_ExecuteGraphQL_Subscription(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	key, keyErr := b.CreateAPIKey(api.APIID, "", 0)
	require.NoError(t, keyErr)
	auth := appsync.GraphQLAuth{APIKey: key.ID}
	_, _ = b.StartSchemaCreation(api.APIID, `
		type Query { dummy: String }
		type Subscription { onEvent: String }
	`)
	_, _ = b.CreateDataSource(api.APIID, &appsync.DataSource{
		Name: "NoneDS",
		Type: appsync.DataSourceTypeNone,
	})
	_, _ = b.CreateResolver(api.APIID, "Subscription", &appsync.Resolver{
		FieldName:      "onEvent",
		DataSourceName: "NoneDS",
	})

	result, err := b.ExecuteGraphQL(t.Context(), api.APIID,
		`subscription { onEvent }`, "", nil, auth)
	require.NoError(t, err)
	assert.Contains(t, result, "onEvent")
}

func TestInMemoryBackend_ExecuteGraphQL_DynamoDBResolver_GetItem(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	ddb := &mockDynamoDB{}
	b.SetDynamoDBBackend(ddb)

	api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	key, keyErr := b.CreateAPIKey(api.APIID, "", 0)
	require.NoError(t, keyErr)
	auth := appsync.GraphQLAuth{APIKey: key.ID}
	_, _ = b.StartSchemaCreation(api.APIID, `type Query { getItem(id: String): String }`)
	_, _ = b.CreateDataSource(api.APIID, &appsync.DataSource{
		Name: "DDBDataSource",
		Type: appsync.DataSourceTypeDynamoDB,
		DynamoDBConfig: &appsync.DynamoDBDataSourceConfig{
			TableName: "items",
			AWSRegion: "us-east-1",
		},
	})
	_, _ = b.CreateResolver(api.APIID, "Query", &appsync.Resolver{
		FieldName:               "getItem",
		DataSourceName:          "DDBDataSource",
		RequestMappingTemplate:  `{"operation": "GetItem", "key": {"id": "$ctx.args.id"}}`,
		ResponseMappingTemplate: `$util.toJson($context.result)`,
	})

	result, err := b.ExecuteGraphQL(t.Context(), api.APIID,
		`query { getItem(id: "123") }`, "", nil, auth)
	require.NoError(t, err)
	assert.Contains(t, result, "getItem")
}

func TestInMemoryBackend_ExecuteGraphQL_DynamoDBResolver_NoTemplate(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	ddb := &mockDynamoDB{}
	b.SetDynamoDBBackend(ddb)

	api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	key, keyErr := b.CreateAPIKey(api.APIID, "", 0)
	require.NoError(t, keyErr)
	auth := appsync.GraphQLAuth{APIKey: key.ID}
	_, _ = b.StartSchemaCreation(api.APIID, `type Query { getItem(id: String): String }`)
	_, _ = b.CreateDataSource(api.APIID, &appsync.DataSource{
		Name:           "DDBDataSource",
		Type:           appsync.DataSourceTypeDynamoDB,
		DynamoDBConfig: &appsync.DynamoDBDataSourceConfig{TableName: "items"},
	})
	_, _ = b.CreateResolver(api.APIID, "Query", &appsync.Resolver{
		FieldName:      "getItem",
		DataSourceName: "DDBDataSource",
		// No request template — uses default GetItem.
	})

	result, err := b.ExecuteGraphQL(t.Context(), api.APIID,
		`query { getItem(id: "123") }`, "", nil, auth)
	require.NoError(t, err)
	assert.Contains(t, result, "getItem")
}

func TestInMemoryBackend_ExecuteGraphQL_DynamoDBResolver_UnsupportedOperation(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	ddb := &mockDynamoDB{}
	b.SetDynamoDBBackend(ddb)

	api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	key, keyErr := b.CreateAPIKey(api.APIID, "", 0)
	require.NoError(t, keyErr)
	auth := appsync.GraphQLAuth{APIKey: key.ID}
	_, _ = b.StartSchemaCreation(api.APIID, `type Query { getItem(id: String): String }`)
	_, _ = b.CreateDataSource(api.APIID, &appsync.DataSource{
		Name:           "DDBDataSource",
		Type:           appsync.DataSourceTypeDynamoDB,
		DynamoDBConfig: &appsync.DynamoDBDataSourceConfig{TableName: "items"},
	})
	_, _ = b.CreateResolver(api.APIID, "Query", &appsync.Resolver{
		FieldName:              "getItem",
		DataSourceName:         "DDBDataSource",
		RequestMappingTemplate: `{"operation": "Scan"}`,
	})

	_, err := b.ExecuteGraphQL(t.Context(), api.APIID, `query { getItem(id: "1") }`, "", nil, auth)
	require.Error(t, err)
}

func TestInMemoryBackend_ExecuteGraphQL_DynamoDBResolver_NilConfig(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	ddb := &mockDynamoDB{}
	b.SetDynamoDBBackend(ddb)

	api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	key, keyErr := b.CreateAPIKey(api.APIID, "", 0)
	require.NoError(t, keyErr)
	auth := appsync.GraphQLAuth{APIKey: key.ID}
	_, _ = b.StartSchemaCreation(api.APIID, `type Query { getItem(id: String): String }`)
	_, _ = b.CreateDataSource(api.APIID, &appsync.DataSource{
		Name: "DDBDataSource",
		Type: appsync.DataSourceTypeDynamoDB,
		// No DynamoDBConfig.
	})
	_, _ = b.CreateResolver(api.APIID, "Query", &appsync.Resolver{
		FieldName:      "getItem",
		DataSourceName: "DDBDataSource",
	})

	_, err := b.ExecuteGraphQL(t.Context(), api.APIID, `query { getItem(id: "1") }`, "", nil, auth)
	require.Error(t, err)
}

func TestInMemoryBackend_ExecuteGraphQL_DynamoDBResolver_NilBackend(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	// Do NOT set DynamoDB backend.

	api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	key, keyErr := b.CreateAPIKey(api.APIID, "", 0)
	require.NoError(t, keyErr)
	auth := appsync.GraphQLAuth{APIKey: key.ID}
	_, _ = b.StartSchemaCreation(api.APIID, `type Query { getItem(id: String): String }`)
	_, _ = b.CreateDataSource(api.APIID, &appsync.DataSource{
		Name:           "DDBDataSource",
		Type:           appsync.DataSourceTypeDynamoDB,
		DynamoDBConfig: &appsync.DynamoDBDataSourceConfig{TableName: "items"},
	})
	_, _ = b.CreateResolver(api.APIID, "Query", &appsync.Resolver{
		FieldName:      "getItem",
		DataSourceName: "DDBDataSource",
	})

	_, err := b.ExecuteGraphQL(t.Context(), api.APIID, `query { getItem(id: "1") }`, "", nil, auth)
	require.Error(t, err)
}

func TestInMemoryBackend_ExecuteGraphQL_LambdaResolver_NilInvoker(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	// Do NOT set lambda invoker.

	api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	key, keyErr := b.CreateAPIKey(api.APIID, "", 0)
	require.NoError(t, keyErr)
	auth := appsync.GraphQLAuth{APIKey: key.ID}
	_, _ = b.StartSchemaCreation(api.APIID, `type Query { hello: String }`)
	_, _ = b.CreateDataSource(api.APIID, &appsync.DataSource{
		Name: "LambdaDS",
		Type: appsync.DataSourceTypeLambda,
		LambdaConfig: &appsync.LambdaDataSourceConfig{
			LambdaFunctionARN: "arn:aws:lambda:us-east-1:000:function:fn",
		},
	})
	_, _ = b.CreateResolver(api.APIID, "Query", &appsync.Resolver{
		FieldName:      "hello",
		DataSourceName: "LambdaDS",
	})

	_, err := b.ExecuteGraphQL(t.Context(), api.APIID, `query { hello }`, "", nil, auth)
	require.Error(t, err)
}

func TestInMemoryBackend_ExecuteGraphQL_LambdaResolver_NilLambdaConfig(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	mock := &mockLambdaInvoker{}
	b.SetLambdaInvoker(mock)

	api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	key, keyErr := b.CreateAPIKey(api.APIID, "", 0)
	require.NoError(t, keyErr)
	auth := appsync.GraphQLAuth{APIKey: key.ID}
	_, _ = b.StartSchemaCreation(api.APIID, `type Query { hello: String }`)
	_, _ = b.CreateDataSource(api.APIID, &appsync.DataSource{
		Name: "LambdaDS",
		Type: appsync.DataSourceTypeLambda,
		// No LambdaConfig set.
	})
	_, _ = b.CreateResolver(api.APIID, "Query", &appsync.Resolver{
		FieldName:      "hello",
		DataSourceName: "LambdaDS",
	})

	_, err := b.ExecuteGraphQL(t.Context(), api.APIID, `query { hello }`, "", nil, auth)
	require.Error(t, err)
}

func TestInMemoryBackend_ExecuteGraphQL_NilResolver(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	key, keyErr := b.CreateAPIKey(api.APIID, "", 0)
	require.NoError(t, keyErr)
	auth := appsync.GraphQLAuth{APIKey: key.ID}
	_, _ = b.StartSchemaCreation(api.APIID, `type Query { hello: String }`)

	// No resolvers defined at all — field should return nil.
	result, err := b.ExecuteGraphQL(t.Context(), api.APIID, `query { hello }`, "", nil, auth)
	require.NoError(t, err)
	assert.Nil(t, result["hello"])
}

func TestInMemoryBackend_ExecuteGraphQL_MissingDataSource(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	key, keyErr := b.CreateAPIKey(api.APIID, "", 0)
	require.NoError(t, keyErr)
	auth := appsync.GraphQLAuth{APIKey: key.ID}
	_, _ = b.StartSchemaCreation(api.APIID, `type Query { hello: String }`)
	// Create resolver but NOT the data source.
	_, _ = b.CreateResolver(api.APIID, "Query", &appsync.Resolver{
		FieldName:      "hello",
		DataSourceName: "MissingDS",
	})

	_, err := b.ExecuteGraphQL(t.Context(), api.APIID, `query { hello }`, "", nil, auth)
	require.Error(t, err)
}

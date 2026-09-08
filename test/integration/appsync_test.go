package integration_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	appsyncsdkv2 "github.com/aws/aws-sdk-go-v2/service/appsync"
	appsyncsdktypes "github.com/aws/aws-sdk-go-v2/service/appsync/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSchema = `type Query {
  getGreeting(name: String): String
}
type Mutation {
  setGreeting(name: String, value: String): String
}
`

func TestIntegration_AppSync_CRUD(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createAppSyncClient(t)
	ctx := t.Context()

	// Create API.
	createOut, err := client.CreateGraphqlApi(ctx, &appsyncsdkv2.CreateGraphqlApiInput{
		Name:               aws.String("integration-test-api"),
		AuthenticationType: appsyncsdktypes.AuthenticationTypeApiKey,
		OwnerContact:       aws.String("team-a@example.com"),
	})
	require.NoError(t, err)
	require.NotNil(t, createOut.GraphqlApi)
	apiID := aws.ToString(createOut.GraphqlApi.ApiId)
	assert.NotEmpty(t, apiID)
	assert.Equal(t, "integration-test-api", aws.ToString(createOut.GraphqlApi.Name))
	assert.Equal(t, "team-a@example.com", aws.ToString(createOut.GraphqlApi.OwnerContact))

	// UpdateGraphqlApi — OwnerContact round-trips through an update too.
	updateOut, err := client.UpdateGraphqlApi(ctx, &appsyncsdkv2.UpdateGraphqlApiInput{
		ApiId:              aws.String(apiID),
		Name:               aws.String("integration-test-api"),
		AuthenticationType: appsyncsdktypes.AuthenticationTypeApiKey,
		OwnerContact:       aws.String("team-b@example.com"),
	})
	require.NoError(t, err)
	assert.Equal(t, "team-b@example.com", aws.ToString(updateOut.GraphqlApi.OwnerContact))

	// Get API.
	getOut, err := client.GetGraphqlApi(ctx, &appsyncsdkv2.GetGraphqlApiInput{
		ApiId: aws.String(apiID),
	})
	require.NoError(t, err)
	assert.Equal(t, apiID, aws.ToString(getOut.GraphqlApi.ApiId))
	assert.Equal(t, "team-b@example.com", aws.ToString(getOut.GraphqlApi.OwnerContact))

	// List APIs — should include our API.
	listOut, err := client.ListGraphqlApis(ctx, &appsyncsdkv2.ListGraphqlApisInput{})
	require.NoError(t, err)
	found := false
	for _, a := range listOut.GraphqlApis {
		if aws.ToString(a.ApiId) == apiID {
			found = true

			break
		}
	}
	assert.True(t, found, "API should appear in list")

	// StartSchemaCreation.
	schemaOut, err := client.StartSchemaCreation(ctx, &appsyncsdkv2.StartSchemaCreationInput{
		ApiId:      aws.String(apiID),
		Definition: []byte(testSchema),
	})
	require.NoError(t, err)
	assert.Equal(t, appsyncsdktypes.SchemaStatusActive, schemaOut.Status)

	// GetSchemaCreationStatus.
	statusOut, err := client.GetSchemaCreationStatus(ctx, &appsyncsdkv2.GetSchemaCreationStatusInput{
		ApiId: aws.String(apiID),
	})
	require.NoError(t, err)
	assert.Equal(t, appsyncsdktypes.SchemaStatusActive, statusOut.Status)

	// CreateDataSource.
	dsOut, err := client.CreateDataSource(ctx, &appsyncsdkv2.CreateDataSourceInput{
		ApiId: aws.String(apiID),
		Name:  aws.String("NoneDS"),
		Type:  appsyncsdktypes.DataSourceTypeNone,
	})
	require.NoError(t, err)
	assert.Equal(t, "NoneDS", aws.ToString(dsOut.DataSource.Name))

	// GetDataSource.
	getDSOut, err := client.GetDataSource(ctx, &appsyncsdkv2.GetDataSourceInput{
		ApiId: aws.String(apiID),
		Name:  aws.String("NoneDS"),
	})
	require.NoError(t, err)
	assert.Equal(t, "NoneDS", aws.ToString(getDSOut.DataSource.Name))

	// ListDataSources.
	listDSOut, err := client.ListDataSources(ctx, &appsyncsdkv2.ListDataSourcesInput{
		ApiId: aws.String(apiID),
	})
	require.NoError(t, err)
	assert.Len(t, listDSOut.DataSources, 1)

	// CreateResolver.
	respTemplate := `$util.toJson($context.result)`
	reqTemplate := `{"operation": "Invoke", "payload": $util.toJson($ctx.args)}`
	resolverOut, err := client.CreateResolver(ctx, &appsyncsdkv2.CreateResolverInput{
		ApiId:                   aws.String(apiID),
		TypeName:                aws.String("Query"),
		FieldName:               aws.String("getGreeting"),
		DataSourceName:          aws.String("NoneDS"),
		RequestMappingTemplate:  aws.String(reqTemplate),
		ResponseMappingTemplate: aws.String(respTemplate),
	})
	require.NoError(t, err)
	assert.Equal(t, "getGreeting", aws.ToString(resolverOut.Resolver.FieldName))

	// GetResolver.
	getResolverOut, err := client.GetResolver(ctx, &appsyncsdkv2.GetResolverInput{
		ApiId:     aws.String(apiID),
		TypeName:  aws.String("Query"),
		FieldName: aws.String("getGreeting"),
	})
	require.NoError(t, err)
	assert.Equal(t, "getGreeting", aws.ToString(getResolverOut.Resolver.FieldName))

	// ListResolvers.
	listROut, err := client.ListResolvers(ctx, &appsyncsdkv2.ListResolversInput{
		ApiId:    aws.String(apiID),
		TypeName: aws.String("Query"),
	})
	require.NoError(t, err)
	assert.Len(t, listROut.Resolvers, 1)

	// DeleteResolver.
	_, err = client.DeleteResolver(ctx, &appsyncsdkv2.DeleteResolverInput{
		ApiId:     aws.String(apiID),
		TypeName:  aws.String("Query"),
		FieldName: aws.String("getGreeting"),
	})
	require.NoError(t, err)

	// DeleteDataSource.
	_, err = client.DeleteDataSource(ctx, &appsyncsdkv2.DeleteDataSourceInput{
		ApiId: aws.String(apiID),
		Name:  aws.String("NoneDS"),
	})
	require.NoError(t, err)

	// DeleteGraphqlApi.
	_, err = client.DeleteGraphqlApi(ctx, &appsyncsdkv2.DeleteGraphqlApiInput{
		ApiId: aws.String(apiID),
	})
	require.NoError(t, err)

	// Verify the API is gone.
	_, err = client.GetGraphqlApi(ctx, &appsyncsdkv2.GetGraphqlApiInput{
		ApiId: aws.String(apiID),
	})
	require.Error(t, err, "API should be deleted")
}

func TestIntegration_AppSync_GraphQL_NoneResolver(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createAppSyncClient(t)
	ctx := t.Context()

	// Create API.
	createOut, err := client.CreateGraphqlApi(ctx, &appsyncsdkv2.CreateGraphqlApiInput{
		Name:               aws.String("graphql-exec-test"),
		AuthenticationType: appsyncsdktypes.AuthenticationTypeApiKey,
	})
	require.NoError(t, err)

	api := createOut.GraphqlApi
	apiID := aws.ToString(api.ApiId)

	keyOut, err := client.CreateApiKey(ctx, &appsyncsdkv2.CreateApiKeyInput{ApiId: aws.String(apiID)})
	require.NoError(t, err)
	apiKey := aws.ToString(keyOut.ApiKey.Id)

	// Upload schema.
	_, err = client.StartSchemaCreation(ctx, &appsyncsdkv2.StartSchemaCreationInput{
		ApiId:      aws.String(apiID),
		Definition: []byte(`type Query { getGreeting(name: String): String }`),
	})
	require.NoError(t, err)

	// Create NONE data source.
	_, err = client.CreateDataSource(ctx, &appsyncsdkv2.CreateDataSourceInput{
		ApiId: aws.String(apiID),
		Name:  aws.String("NoneDS"),
		Type:  appsyncsdktypes.DataSourceTypeNone,
	})
	require.NoError(t, err)

	// Create resolver with response template that returns the name argument.
	_, err = client.CreateResolver(ctx, &appsyncsdkv2.CreateResolverInput{
		ApiId:                   aws.String(apiID),
		TypeName:                aws.String("Query"),
		FieldName:               aws.String("getGreeting"),
		DataSourceName:          aws.String("NoneDS"),
		RequestMappingTemplate:  aws.String(`{"name": "$ctx.args.name"}`),
		ResponseMappingTemplate: aws.String(`$util.toJson($context.result)`),
	})
	require.NoError(t, err)

	// Construct the GraphQL endpoint using the accessible test endpoint.
	// api.Uris["GRAPHQL"] contains the container-internal URL; rewrite with the accessible endpoint.
	graphqlEndpoint := endpoint + "/v1/apis/" + apiID + "/graphql"

	// Execute GraphQL query via raw HTTP POST.
	query := `query { getGreeting(name: "World") }`
	respBody := executeGraphQL(t, graphqlEndpoint, query, "", nil, apiKey)

	assert.Nil(t, respBody["errors"], "no GraphQL errors expected")

	data, ok := respBody["data"].(map[string]any)
	require.True(t, ok, "data field should be present")
	assert.NotNil(t, data["getGreeting"], "getGreeting field should be present")

	// Cleanup.
	_, _ = client.DeleteGraphqlApi(ctx, &appsyncsdkv2.DeleteGraphqlApiInput{ApiId: aws.String(apiID)})
}

// executeGraphQL sends a GraphQL request to the given endpoint and returns the
// parsed response. apiKey, if non-empty, is sent as the x-api-key header.
func executeGraphQL(
	t *testing.T, gqlEndpoint, query, operationName string, variables map[string]any, apiKey string,
) map[string]any {
	t.Helper()

	body := map[string]any{"query": query}

	if operationName != "" {
		body["operationName"] = operationName
	}

	if variables != nil {
		body["variables"] = variables
	}

	bodyBytes, err := json.Marshal(body)
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, gqlEndpoint, bytes.NewReader(bodyBytes))
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/json")

	if apiKey != "" {
		req.Header.Set("X-Api-Key", apiKey)
	}

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(
		t,
		http.StatusOK,
		resp.StatusCode,
		"GraphQL endpoint returned status %d: %s",
		resp.StatusCode,
		string(respBytes),
	)

	var result map[string]any
	require.NoError(t, json.Unmarshal(respBytes, &result))

	return result
}

func TestIntegration_AppSync_GetNotFound(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createAppSyncClient(t)
	ctx := t.Context()

	_, err := client.GetGraphqlApi(ctx, &appsyncsdkv2.GetGraphqlApiInput{
		ApiId: aws.String("nonexistent-api-id"),
	})
	require.Error(t, err, "should return error for nonexistent API")
}

func TestIntegration_AppSync_SchemaCreation_Invalid(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createAppSyncClient(t)
	ctx := t.Context()

	createOut, err := client.CreateGraphqlApi(ctx, &appsyncsdkv2.CreateGraphqlApiInput{
		Name:               aws.String("schema-error-test"),
		AuthenticationType: appsyncsdktypes.AuthenticationTypeApiKey,
	})
	require.NoError(t, err)

	apiID := aws.ToString(createOut.GraphqlApi.ApiId)

	defer func() {
		_, _ = client.DeleteGraphqlApi(ctx, &appsyncsdkv2.DeleteGraphqlApiInput{ApiId: aws.String(apiID)})
	}()

	// Upload invalid schema.
	_, err = client.StartSchemaCreation(ctx, &appsyncsdkv2.StartSchemaCreationInput{
		ApiId:      aws.String(apiID),
		Definition: []byte(`type { broken schema`),
	})
	require.Error(t, err, "should return error for invalid schema")
}

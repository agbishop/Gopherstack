package integration_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	smithy "github.com/aws/smithy-go"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// chaosFaultError is the JSON schema for a custom error in a chaos fault rule.
type chaosFaultError struct {
	Code       string `json:"code"`
	StatusCode int    `json:"statusCode"`
}

// chaosFaultRule is the JSON schema for a chaos fault rule.
// All fields are optional; omitting a field means "match any".
type chaosFaultRule struct {
	Error     *chaosFaultError `json:"error,omitempty"`
	Service   string           `json:"service,omitempty"`
	Region    string           `json:"region,omitempty"`
	Operation string           `json:"operation,omitempty"`
}

// chaosNetworkEffects is the JSON schema for the chaos network effects configuration.
type chaosNetworkEffects struct {
	Latency int `json:"latency"`
}

// startChaosContainer starts a fresh Gopherstack container with no special environment
// variables — used by chaos tests so that fault injection doesn't interfere with
// other tests running against the shared container.
func startChaosContainer(t *testing.T) string {
	t.Helper()

	ctx := t.Context()

	dockerfile, err := dockerfileFor()
	require.NoError(t, err)

	req := testcontainers.ContainerRequest{
		Context:      "../../",
		Dockerfile:   dockerfile,
		ExposedPorts: []string{"8000/tcp"},
		WaitingFor: wait.ForHTTP("/_gopherstack/health").
			WithStatusCodeMatcher(func(status int) bool { return status == 200 }).
			WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "failed to start chaos test container")

	t.Cleanup(func() {
		cleanupCtx, cancel := cleanupContext(t)
		defer cancel()

		_ = container.Terminate(cleanupCtx)
	})

	mappedPort, err := container.MappedPort(ctx, "8000")
	require.NoError(t, err)

	return "http://localhost:" + mappedPort.Port()
}

// newDynamoDBClientAt returns a DynamoDB client pointed at the given endpoint.
func newDynamoDBClientAt(t *testing.T, ep string) *dynamodb.Client {
	t.Helper()

	cfg, err := awsconfig.LoadDefaultConfig(
		t.Context(),
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err)

	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String(ep)
	})
}

// postChaosRules replaces the active fault rules by POSTing to the chaos faults endpoint.
func postChaosRules(t *testing.T, ep string, rules any) {
	t.Helper()

	body, err := json.Marshal(rules)
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		ep+"/_gopherstack/chaos/faults",
		bytes.NewReader(body),
	)
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode, "chaos faults POST should return 200")
}

// postChaosEffects sets the network effects configuration.
func postChaosEffects(t *testing.T, ep string, effects chaosNetworkEffects) {
	t.Helper()

	body, err := json.Marshal(effects)
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		ep+"/_gopherstack/chaos/effects",
		bytes.NewReader(body),
	)
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode, "chaos effects POST should return 200")
}

// TestIntegration_Chaos_FaultInjection verifies the Chaos API:
//  1. Creates a DynamoDB table and confirms it is accessible.
//  2. Enables a global DynamoDB fault via POST /_gopherstack/chaos/faults.
//  3. Asserts that subsequent DynamoDB operations return an injected error.
//  4. Disables all faults (POST []).
//  5. Confirms that DynamoDB operations succeed again.
func TestIntegration_Chaos_FaultInjection(t *testing.T) {
	t.Parallel()

	ep := startChaosContainer(t)

	ddb := newDynamoDBClientAt(t, ep)
	ctx := t.Context()

	tableName := "chaos-test-" + uuid.NewString()[:8]

	// --- Step 1: create a table and confirm normal operation ---
	_, err := ddb.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(tableName),
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash},
		},
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("pk"), AttributeType: types.ScalarAttributeTypeS},
		},
		BillingMode: types.BillingModePayPerRequest,
	})
	require.NoError(t, err, "CreateTable should succeed before chaos is enabled")

	_, err = ddb.ListTables(ctx, &dynamodb.ListTablesInput{})
	require.NoError(t, err, "ListTables should succeed before chaos is enabled")

	// --- Step 2: enable a DynamoDB fault (503 ServiceUnavailable) ---
	postChaosRules(t, ep, []chaosFaultRule{
		{
			Service: "dynamodb",
			Error:   &chaosFaultError{StatusCode: 503, Code: "ServiceUnavailable"},
		},
	})

	// --- Step 3: assert that DynamoDB requests now fail ---
	_, chaosErr := ddb.ListTables(ctx, &dynamodb.ListTablesInput{})
	require.Error(t, chaosErr, "ListTables should fail while chaos is active")

	var apiErr smithy.APIError
	require.ErrorAs(t, chaosErr, &apiErr, "error should be a smithy.APIError")
	assert.Equal(t, "ServiceUnavailable", apiErr.ErrorCode(),
		"error code should match the injected fault")

	// --- Step 4: disable all faults ---
	postChaosRules(t, ep, []chaosFaultRule{})

	// --- Step 5: confirm normal operation resumes ---
	_, err = ddb.ListTables(ctx, &dynamodb.ListTablesInput{})
	require.NoError(t, err, "ListTables should succeed after chaos is disabled")
}

// TestIntegration_Chaos_OperationScopedFault verifies that a fault targeting a
// specific operation only affects that operation while others proceed normally.
// The default 503 ServiceUnavailable error is injected (no explicit Error field).
func TestIntegration_Chaos_OperationScopedFault(t *testing.T) {
	t.Parallel()

	ep := startChaosContainer(t)

	ddb := newDynamoDBClientAt(t, ep)
	ctx := t.Context()

	tableName := "chaos-op-" + uuid.NewString()[:8]

	// Create a table so PutItem has something to work against.
	_, err := ddb.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(tableName),
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash},
		},
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("pk"), AttributeType: types.ScalarAttributeTypeS},
		},
		BillingMode: types.BillingModePayPerRequest,
	})
	require.NoError(t, err)

	// Enable a fault only for DynamoDB PutItem. Omitting Error injects the default
	// 503 ServiceUnavailable response.
	postChaosRules(t, ep, []chaosFaultRule{
		{Service: "dynamodb", Operation: "PutItem"},
	})

	// PutItem should be faulted.
	_, putErr := ddb.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: "key1"},
		},
	})
	require.Error(t, putErr, "PutItem should fail when faulted")

	// ListTables should still work (different operation).
	_, listErr := ddb.ListTables(ctx, &dynamodb.ListTablesInput{})
	require.NoError(t, listErr, "ListTables should succeed; only PutItem is faulted")
}

// TestIntegration_Chaos_NetworkEffectsLatency verifies that adding latency via the
// chaos effects API measurably slows down responses.
func TestIntegration_Chaos_NetworkEffectsLatency(t *testing.T) {
	t.Parallel()

	ep := startChaosContainer(t)

	ddb := newDynamoDBClientAt(t, ep)
	ctx := t.Context()

	// Baseline: measure how fast requests are without latency.
	const requests = 3

	baseline := time.Now()
	for range requests {
		_, err := ddb.ListTables(ctx, &dynamodb.ListTablesInput{})
		require.NoError(t, err)
	}
	baselineElapsed := time.Since(baseline)

	// Enable 200ms fixed latency.
	postChaosEffects(t, ep, chaosNetworkEffects{Latency: 200})

	// Measure with latency active.
	latencyStart := time.Now()

	for range requests {
		_, lerr := ddb.ListTables(ctx, &dynamodb.ListTablesInput{})
		require.NoError(t, lerr)
	}

	latencyElapsed := time.Since(latencyStart)

	// With 200ms latency per request × 3 requests = 600ms minimum.
	// We assert a floor of 400ms to allow generous headroom on slow CI machines.
	assert.GreaterOrEqual(t, latencyElapsed.Milliseconds(), int64(400),
		"expected significant latency when chaos effects are active; baseline=%v latency=%v",
		baselineElapsed, latencyElapsed)
}

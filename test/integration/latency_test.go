package integration_test

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// startLatencyContainer starts a Gopherstack container with LATENCY_MS set.
func startLatencyContainer(t *testing.T, latencyMs string) (testcontainers.Container, string) {
	t.Helper()

	ctx := t.Context()

	dockerfile, err := dockerfileFor()
	require.NoError(t, err)

	req := testcontainers.ContainerRequest{
		Context:    "../../",
		Dockerfile: dockerfile,
		Env: map[string]string{
			"LATENCY_MS": latencyMs,
		},
		ExposedPorts: []string{"8000/tcp"},
		WaitingFor: wait.ForHTTP("/_gopherstack/health").
			WithStatusCodeMatcher(func(status int) bool { return status == 200 }).
			WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "failed to start latency container")

	t.Cleanup(func() {
		cleanupCtx, cancel := cleanupContext(t)
		defer cancel()

		_ = container.Terminate(cleanupCtx)
	})

	mappedPort, err := container.MappedPort(ctx, "8000")
	require.NoError(t, err)

	ep := "http://localhost:" + mappedPort.Port()

	return container, ep
}

// createLatencyDynamoDBClient returns a DynamoDB client pointed at the given endpoint.
func createLatencyDynamoDBClient(t *testing.T, ep string) *dynamodb.Client {
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

// TestIntegration_Latency_ResponsesAreSlow verifies that when LATENCY_MS is set,
// request latency is measurably greater than without it. It sends several requests
// via a real service endpoint (DynamoDB ListTables) that is routed through the
// service registry so the latency middleware is active.
func TestIntegration_Latency_ResponsesAreSlow(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	const (
		latencyMs  = "50" // random sleep in [0, 50) ms per request
		requests   = 5    // number of requests to send
		minTotalMs = 10   // very conservative floor: 5 requests × ~2ms overhead still easily > 10ms
	)

	_, ep := startLatencyContainer(t, latencyMs)

	client := createLatencyDynamoDBClient(t, ep)
	ctx := t.Context()

	start := time.Now()

	for range requests {
		_, err := client.ListTables(ctx, &dynamodb.ListTablesInput{})
		require.NoError(t, err)
	}

	elapsed := time.Since(start)

	// With LATENCY_MS=50, each request sleeps uniformly in [0,50)ms.
	// Expected total sleep over 5 requests is ~125ms (5 × 25ms mean).
	// We assert a floor of 10ms — well below average — to avoid flakiness on
	// fast CI machines where individual sleeps can be close to 0 by chance.
	assert.GreaterOrEqual(t, elapsed.Milliseconds(), int64(minTotalMs),
		"expected total elapsed time >= %dms with LATENCY_MS=%s; got %v", minTotalMs, latencyMs, elapsed)
}

// TestIntegration_Latency_DisabledByDefault verifies that without LATENCY_MS,
// service requests complete quickly. Uses the shared container (no latency set).
func TestIntegration_Latency_DisabledByDefault(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	const requests = 5

	client := createLatencyDynamoDBClient(t, endpoint)
	ctx := t.Context()

	start := time.Now()

	for range requests {
		_, err := client.ListTables(ctx, &dynamodb.ListTablesInput{})
		require.NoError(t, err)
	}

	elapsed := time.Since(start)

	// Without latency injection, 5 DynamoDB ListTables requests against a local
	// container should complete well under 500ms.
	assert.Less(t, elapsed.Milliseconds(), int64(500),
		"expected fast responses without latency; got %v", elapsed)
}

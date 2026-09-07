package integration_test

import (
	"context"
	"debug/elf"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/dynamoattr"
	"github.com/blackbirdworks/gopherstack/services/dynamodb/models"
	"github.com/blackbirdworks/gopherstack/test/internal/buildcheck"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	acmsdk "github.com/aws/aws-sdk-go-v2/service/acm"
	amplifysdk "github.com/aws/aws-sdk-go-v2/service/amplify"
	apigwv2sdk "github.com/aws/aws-sdk-go-v2/service/apigatewayv2"
	applicationautoscalingsdk "github.com/aws/aws-sdk-go-v2/service/applicationautoscaling"
	appsyncsdkv2 "github.com/aws/aws-sdk-go-v2/service/appsync"
	athenasdk "github.com/aws/aws-sdk-go-v2/service/athena"
	autoscalingsdk "github.com/aws/aws-sdk-go-v2/service/autoscaling"
	backupsdk "github.com/aws/aws-sdk-go-v2/service/backup"
	batchsdk "github.com/aws/aws-sdk-go-v2/service/batch"
	bedrocksdk "github.com/aws/aws-sdk-go-v2/service/bedrock"
	cloudcontrolsdk "github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	cloudformationsdk "github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cloudfrontsdk "github.com/aws/aws-sdk-go-v2/service/cloudfront"
	cloudtrailsdk "github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	cloudwatchsdk "github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cloudwatchlogssdk "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	codeartifactsdk "github.com/aws/aws-sdk-go-v2/service/codeartifact"
	codebuildsdk "github.com/aws/aws-sdk-go-v2/service/codebuild"
	codecommitsdk "github.com/aws/aws-sdk-go-v2/service/codecommit"
	codeconnectionssdk "github.com/aws/aws-sdk-go-v2/service/codeconnections"
	codedeploysdk "github.com/aws/aws-sdk-go-v2/service/codedeploy"
	codepipelinesdk "github.com/aws/aws-sdk-go-v2/service/codepipeline"
	codestarconnectionssdk "github.com/aws/aws-sdk-go-v2/service/codestarconnections"
	cognitoidentitysdk "github.com/aws/aws-sdk-go-v2/service/cognitoidentity"
	cognitoidpsdk "github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	cesdk "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	docdbsdk "github.com/aws/aws-sdk-go-v2/service/docdb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/dynamodbstreams"
	ec2sdk "github.com/aws/aws-sdk-go-v2/service/ec2"
	ecrsdk "github.com/aws/aws-sdk-go-v2/service/ecr"
	ecssdk "github.com/aws/aws-sdk-go-v2/service/ecs"
	efssdk "github.com/aws/aws-sdk-go-v2/service/efs"
	ekssdk "github.com/aws/aws-sdk-go-v2/service/eks"
	elasticachesdk "github.com/aws/aws-sdk-go-v2/service/elasticache"
	elasticbeanstalksdk "github.com/aws/aws-sdk-go-v2/service/elasticbeanstalk"
	elbsdk "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing"
	elbv2sdk "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elasticsearchsdk "github.com/aws/aws-sdk-go-v2/service/elasticsearchservice"
	emrsdk "github.com/aws/aws-sdk-go-v2/service/emr"
	emrserverlesssdk "github.com/aws/aws-sdk-go-v2/service/emrserverless"
	eventbridgesdk "github.com/aws/aws-sdk-go-v2/service/eventbridge"
	firehosesdk "github.com/aws/aws-sdk-go-v2/service/firehose"
	gluesdk "github.com/aws/aws-sdk-go-v2/service/glue"
	iamsdk "github.com/aws/aws-sdk-go-v2/service/iam"
	identitystoresdk "github.com/aws/aws-sdk-go-v2/service/identitystore"
	kinesissdk "github.com/aws/aws-sdk-go-v2/service/kinesis"
	kmssdk "github.com/aws/aws-sdk-go-v2/service/kms"
	lambdaclientsdk "github.com/aws/aws-sdk-go-v2/service/lambda"
	neptunesdk "github.com/aws/aws-sdk-go-v2/service/neptune"
	rdssdk "github.com/aws/aws-sdk-go-v2/service/rds"
	route53sdk "github.com/aws/aws-sdk-go-v2/service/route53"
	route53resolversdk "github.com/aws/aws-sdk-go-v2/service/route53resolver"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3tablesclientsdk "github.com/aws/aws-sdk-go-v2/service/s3tables"
	schedulersdk "github.com/aws/aws-sdk-go-v2/service/scheduler"
	secretsmanagersdk "github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	sarsdk "github.com/aws/aws-sdk-go-v2/service/serverlessapplicationrepository"
	sfnsdk "github.com/aws/aws-sdk-go-v2/service/sfn"
	shieldsdk "github.com/aws/aws-sdk-go-v2/service/shield"
	snssdk "github.com/aws/aws-sdk-go-v2/service/sns"
	sqssdk "github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssoadminsdk "github.com/aws/aws-sdk-go-v2/service/ssoadmin"
	stssdk "github.com/aws/aws-sdk-go-v2/service/sts"
	supportsdk "github.com/aws/aws-sdk-go-v2/service/support"
	swfsdk "github.com/aws/aws-sdk-go-v2/service/swf"
	textractsdk "github.com/aws/aws-sdk-go-v2/service/textract"
	timestreamquerysdk "github.com/aws/aws-sdk-go-v2/service/timestreamquery"
	timestreamwritesdk "github.com/aws/aws-sdk-go-v2/service/timestreamwrite"
	transfersdk "github.com/aws/aws-sdk-go-v2/service/transfer"
	wafsdk "github.com/aws/aws-sdk-go-v2/service/waf"
	wafv2sdk "github.com/aws/aws-sdk-go-v2/service/wafv2"
	xraysdk "github.com/aws/aws-sdk-go-v2/service/xray"
	"github.com/google/go-cmp/cmp"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// endpoint is the base URL for the running Gopherstack container.
// Both DynamoDB and S3 clients connect to this single endpoint.
// This is initialized by TestMain before running integration tests.
//
//nolint:gochecknoglobals // Set in TestMain for integration tests.
var endpoint string

// mqttEndpoint is the MQTT broker URL for the running Gopherstack container.
// This is initialized by TestMain before running integration tests.
//
//nolint:gochecknoglobals // Set in TestMain for integration tests.
var mqttEndpoint string

// azureBlobEndpoint is the Azure Blob Storage-compatible endpoint for the
// running Gopherstack container (its own dedicated port -- see
// services/azureblob/provider.go and AZURE.md section 4 for why this service
// cannot share the main AWS endpoint/port). Left empty (and Azure Blob tests
// skipped) if the mapped port cannot be determined, mirroring mqttEndpoint's
// non-fatal behavior above. This is initialized by TestMain before running
// integration tests.
//
//nolint:gochecknoglobals // Set in TestMain for integration tests.
var azureBlobEndpoint string

// azureQueueEndpoint is the Azure Queue Storage-compatible endpoint for the
// running Gopherstack container (its own dedicated port -- see
// services/azurequeue/provider.go and AZURE.md section 4 for why this
// service cannot share the main AWS endpoint/port, or even AzureBlob's own
// dedicated port). Left empty (and Azure Queue tests skipped) if the mapped
// port cannot be determined, mirroring azureBlobEndpoint's non-fatal
// behavior above. This is initialized by TestMain before running
// integration tests.
//
//nolint:gochecknoglobals // Set in TestMain for integration tests.
var azureQueueEndpoint string

// azureTableEndpoint is the Azure Table Storage-compatible endpoint for the
// running Gopherstack container (its own dedicated port -- see
// services/azuretable/provider.go and AZURE.md section 4 for why this
// service cannot share the main AWS endpoint/port, or either of AzureBlob's/
// AzureQueue's own dedicated ports). Left empty (and Azure Table tests
// skipped) if the mapped port cannot be determined, mirroring
// azureQueueEndpoint's non-fatal behavior above. This is initialized by
// TestMain before running integration tests.
//
//nolint:gochecknoglobals // Set in TestMain for integration tests.
var azureTableEndpoint string

// cosmosDBEndpoint is the Azure Cosmos DB (Core/SQL API)-compatible endpoint
// for the running Gopherstack container (its own dedicated port -- see
// services/cosmosdb/provider.go and AZURE.md section 4 for why this service
// cannot share the main AWS endpoint/port, or any of AzureBlob's/
// AzureQueue's/AzureTable's own dedicated ports). Left empty (and Cosmos DB
// tests skipped) if the mapped port cannot be determined, mirroring
// azureTableEndpoint's non-fatal behavior above. This is initialized by
// TestMain before running integration tests.
//
//nolint:gochecknoglobals // Set in TestMain for integration tests.
var cosmosDBEndpoint string

// azureServiceBusEndpoint is the Azure Service Bus Brokered Messaging
// REST-compatible endpoint for the running Gopherstack container (its own
// dedicated port -- see services/azureservicebus/provider.go and AZURE.md
// section 9's M5 entry for why this service cannot share the main AWS
// endpoint/port, or any of AzureBlob's/AzureQueue's/AzureTable's own
// dedicated ports). Left empty (and Azure Service Bus tests skipped) if the
// mapped port cannot be determined, mirroring cosmosDBEndpoint's non-fatal
// behavior above. This is initialized by TestMain before running
// integration tests.
//
//nolint:gochecknoglobals // Set in TestMain for integration tests.
var azureServiceBusEndpoint string

// sharedContainer holds a reference to the container for cleanup and log dumping on test failures.
// This is initialized by TestMain before running integration tests.
//
//nolint:gochecknoglobals // Set in TestMain for integration tests.
var sharedContainer testcontainers.Container

// ErrDockerPanic is returned when the Docker availability check panics.
var ErrDockerPanic = errors.New("docker check panicked")

// ErrResetFailed is returned when the reset endpoint returns an unexpected status.
var ErrResetFailed = errors.New("reset endpoint returned unexpected status")

// ErrBinaryNotStatic is returned when preBuiltLinuxBinary exists but is not a
// statically-linked linux binary, so Dockerfile.test's `FROM scratch` image
// cannot run it. Turns the "No such container" mystery from gopherstack-gooq
// into a clear error at build time instead.
var ErrBinaryNotStatic = errors.New("not a statically-linked linux binary; run `make build-linux`")

// preBuiltLinuxBinary is where `make build-linux` writes its static binary,
// deliberately distinct from `make build`'s bin/gopherstack (gopherstack-gooq).
const preBuiltLinuxBinary = "../../bin/gopherstack-linux"

// dockerfileFor picks Dockerfile.test when a pre-built binary exists at
// preBuiltLinuxBinary, verifying it is actually static and not stale first.
func dockerfileFor() (string, error) {
	if binInfo, statErr := os.Stat(preBuiltLinuxBinary); statErr == nil {
		if err := requireStaticELF(preBuiltLinuxBinary); err != nil {
			return "", fmt.Errorf("%s: %w", preBuiltLinuxBinary, err)
		}

		logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
		if err := buildcheck.CheckFreshness(logger, binInfo); err != nil {
			return "", err
		}

		return "Dockerfile.test", nil
	}

	return "Dockerfile", nil
}

// requireStaticELF rejects a binary that isn't ELF at all (e.g. a macOS
// `make build` artifact) or that carries a PT_INTERP segment, meaning it's
// dynamically linked against libc and won't run in a `FROM scratch` image.
func requireStaticELF(path string) error {
	f, err := elf.Open(path)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrBinaryNotStatic, err)
	}
	defer func() { _ = f.Close() }()

	for _, prog := range f.Progs {
		if prog.Type == elf.PT_INTERP {
			return ErrBinaryNotStatic
		}
	}

	return nil
}

// resolveMappedEndpoint looks up container's host-mapped port for
// containerPort and, on success, returns a "<scheme>://localhost:<port>"
// endpoint after logging it as available under label. On failure it logs a
// warning naming which tests will be skipped and returns "", leaving the
// corresponding endpoint global unset -- callers must not treat "" as a
// real endpoint.
func resolveMappedEndpoint(
	ctx context.Context, container testcontainers.Container, logger *slog.Logger,
	containerPort, scheme, label, skippedTests string,
) string {
	port, err := container.MappedPort(ctx, containerPort)
	if err != nil {
		logger.WarnContext(ctx, "failed to get mapped port; tests will be skipped",
			"label", label, "skipped", skippedTests, "error", err)

		return ""
	}

	resolved := scheme + "://localhost:" + port.Port()
	logger.InfoContext(ctx, label+" running", "endpoint", resolved)

	return resolved
}

func TestMain(m *testing.M) {
	flag.Parse()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	if testing.Short() {
		logger.Info("skipping integration tests in short mode")
		os.Exit(0)
	}

	// Escape hatch: if GOPHERSTACK_ENDPOINT is set, point the suite at an
	// already-running server instead of building/starting a testcontainer.
	// This is used by `make pgo` to run the integration suite against a
	// locally-running, pprof-enabled server so that profile data covers
	// realistic request traffic. Placed at the very top of the
	// container-setup path so Docker is not required in this mode.
	// Container build/start and teardown are both skipped; sharedContainer
	// stays nil, so anything that dumps container logs on failure is a
	// no-op (see dumpContainerLogsOnFailure).
	if envEndpoint := os.Getenv("GOPHERSTACK_ENDPOINT"); envEndpoint != "" {
		endpoint = envEndpoint
		logger.Info("using external Gopherstack endpoint from GOPHERSTACK_ENDPOINT; skipping container setup",
			"endpoint", endpoint)

		if resetErr := resetGopherstackState(endpoint); resetErr != nil {
			logger.Error("failed to reset gopherstack state", "error", resetErr)
			os.Exit(1)
		}

		logger.Info("gopherstack state reset; starting tests")

		code := m.Run()

		os.Exit(code)
	}

	if err := checkDocker(); err != nil {
		logger.Error("integration tests require docker", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()

	dockerfile, err := dockerfileFor()
	if err != nil {
		logger.Error("pre-built binary unusable for Dockerfile.test", "error", err)
		os.Exit(1)
	}

	if dockerfile == "Dockerfile.test" {
		logger.Info("using pre-built binary via Dockerfile.test")
	} else {
		logger.Info("no pre-built binary found, building from source via Dockerfile")
	}

	req := testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context:       "../../",
			Dockerfile:    dockerfile,
			PrintBuildLog: true,
			BuildOptionsModifier: func(options *client.ImageBuildOptions) {
				options.NoCache = false
				options.PullParent = false
			},
		},
		ExposedPorts: []string{
			"8000/tcp", "1883/tcp", "10000/tcp", "10001/tcp", "10002/tcp", "10003/tcp", "8081/tcp",
		},
		WaitingFor: wait.ForAll(
			wait.ForHTTP("/").
				WithPort("8000/tcp").
				WithStatusCodeMatcher(func(_ int) bool { return true }).
				WithStartupTimeout(60*time.Second),
			wait.ForListeningPort("1883/tcp").
				WithStartupTimeout(60*time.Second),
			wait.ForListeningPort("10000/tcp").
				WithStartupTimeout(60*time.Second),
			wait.ForListeningPort("10001/tcp").
				WithStartupTimeout(60*time.Second),
			wait.ForListeningPort("10002/tcp").
				WithStartupTimeout(60*time.Second),
			wait.ForListeningPort("10003/tcp").
				WithStartupTimeout(60*time.Second),
			wait.ForListeningPort("8081/tcp").
				WithStartupTimeout(60*time.Second),
		),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		logger.Error("failed to start container", "error", err)

		os.Exit(1)
	}

	sharedContainer = container

	mappedPort, err := container.MappedPort(ctx, "8000")
	if err != nil {
		logger.Error("failed to get mapped port", "error", err)
		os.Exit(1)
	}

	endpoint = "http://localhost:" + mappedPort.Port()
	logger.Info("Gopherstack running", "endpoint", endpoint)

	// Verify the reset endpoint works and start all tests with clean state.
	// This runs before any parallel test is executed, so it cannot race with them.
	if resetErr := resetGopherstackState(endpoint); resetErr != nil {
		logger.Error("failed to reset gopherstack state", "error", resetErr)
		os.Exit(1)
	}

	logger.Info("gopherstack state reset; starting tests")

	mqttEndpoint = resolveMappedEndpoint(ctx, container, logger, "1883", "tcp", "MQTT broker", "IoT")
	azureBlobEndpoint = resolveMappedEndpoint(
		ctx, container, logger, "10000", "http", "Azure Blob Storage-compatible endpoint", "Azure Blob")
	azureQueueEndpoint = resolveMappedEndpoint(
		ctx, container, logger, "10001", "http", "Azure Queue Storage-compatible endpoint", "Azure Queue")
	azureTableEndpoint = resolveMappedEndpoint(
		ctx, container, logger, "10002", "http", "Azure Table Storage-compatible endpoint", "Azure Table")
	cosmosDBEndpoint = resolveMappedEndpoint(
		ctx, container, logger, "8081", "http", "Cosmos DB (Core/SQL API)-compatible endpoint", "Cosmos DB")
	azureServiceBusEndpoint = resolveMappedEndpoint(
		ctx, container, logger, "10003", "http",
		"Azure Service Bus Brokered Messaging REST-compatible endpoint", "Azure Service Bus")

	code := m.Run()

	if sharedContainer != nil {
		if tErr := sharedContainer.Terminate(ctx); tErr != nil {
			logger.Error("failed to terminate container", "error", tErr)
		}
	}

	os.Exit(code)
}

// checkDocker safely checks if the Docker daemon is available by attempting
// to create a provider and recovering from any potential panics.
func checkDocker() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: %v", ErrDockerPanic, r)
		}
	}()

	_, err = testcontainers.NewDockerProvider()

	return err
}

// cleanupContext returns a fresh, live context for use inside t.Cleanup.
// t.Context() is cancelled just before cleanup functions run, so AWS calls
// made with it fail instantly with "context canceled".
func cleanupContext(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()

	return context.WithTimeout(context.Background(), 30*time.Second)
}

// createDynamoDBClient returns a DynamoDB client pointed at the shared test container.

func createDynamoDBClient(t *testing.T) *dynamodb.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// waitForDDBTableActive polls DescribeTable until the table reaches ACTIVE.
func waitForDDBTableActive(t *testing.T, client *dynamodb.Client, tableName string) {
	t.Helper()

	require.Eventually(t, func() bool {
		out, err := client.DescribeTable(t.Context(), &dynamodb.DescribeTableInput{
			TableName: aws.String(tableName),
		})

		return err == nil && out.Table != nil && out.Table.TableStatus == types.TableStatusActive
	}, 10*time.Second, 20*time.Millisecond)
}

// createDynamoDBStreamsClient returns a DynamoDB Streams client pointed at the shared test container.
func createDynamoDBStreamsClient(t *testing.T) *dynamodbstreams.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return dynamodbstreams.NewFromConfig(cfg, func(o *dynamodbstreams.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createS3Client returns an S3 client pointed at the shared test container.
func createS3Client(t *testing.T) *s3.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = true
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createSSMClient returns an SSM client pointed at the shared test container.
func createSSMClient(t *testing.T) *ssm.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return ssm.NewFromConfig(cfg, func(o *ssm.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createSQSClient returns an SQS client pointed at the shared test container.
func createSQSClient(t *testing.T) *sqssdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return sqssdk.NewFromConfig(cfg, func(o *sqssdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createSNSClient returns an SNS client pointed at the shared test container.
func createSNSClient(t *testing.T) *snssdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return snssdk.NewFromConfig(cfg, func(o *snssdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createSTSClientWithCreds returns an STS client pointed at the shared test container
// using the provided assumed-role credentials.
func createSTSClientWithCreds(t *testing.T, accessKeyID, secretKey, sessionToken string) *stssdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKeyID, secretKey, sessionToken),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return stssdk.NewFromConfig(cfg, func(o *stssdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createSTSClient returns an STS client pointed at the shared test container.
func createSTSClient(t *testing.T) *stssdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return stssdk.NewFromConfig(cfg, func(o *stssdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createKMSClient returns a KMS client pointed at the shared test container.
func createKMSClient(t *testing.T) *kmssdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return kmssdk.NewFromConfig(cfg, func(o *kmssdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createSecretsManagerClient returns a Secrets Manager client pointed at the shared test container.
func createSecretsManagerClient(t *testing.T) *secretsmanagersdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return secretsmanagersdk.NewFromConfig(cfg, func(o *secretsmanagersdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createIAMClient returns an IAM client pointed at the shared test container.
func createIAMClient(t *testing.T) *iamsdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return iamsdk.NewFromConfig(cfg, func(o *iamsdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createEventBridgeClient returns an EventBridge client pointed at the shared test container.
func createEventBridgeClient(t *testing.T) *eventbridgesdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return eventbridgesdk.NewFromConfig(cfg, func(o *eventbridgesdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createCloudWatchClient returns a CloudWatch client pointed at the shared test container.
func createCloudWatchClient(t *testing.T) *cloudwatchsdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return cloudwatchsdk.NewFromConfig(cfg, func(o *cloudwatchsdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createCloudWatchLogsClient returns a CloudWatch Logs client pointed at the shared test container.
func createCloudWatchLogsClient(t *testing.T) *cloudwatchlogssdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return cloudwatchlogssdk.NewFromConfig(cfg, func(o *cloudwatchlogssdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createStepFunctionsClient returns a Step Functions client pointed at the shared test container.
func createStepFunctionsClient(t *testing.T) *sfnsdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return sfnsdk.NewFromConfig(cfg, func(o *sfnsdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createCloudFormationClient returns a CloudFormation client pointed at the shared test container.
func createCloudFormationClient(t *testing.T) *cloudformationsdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return cloudformationsdk.NewFromConfig(cfg, func(o *cloudformationsdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createKinesisClient returns a Kinesis client pointed at the shared test container.
func createKinesisClient(t *testing.T) *kinesissdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return kinesissdk.NewFromConfig(cfg, func(o *kinesissdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createLambdaClient returns a Lambda client pointed at the shared test container.
func createLambdaClient(t *testing.T) *lambdaclientsdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return lambdaclientsdk.NewFromConfig(cfg, func(o *lambdaclientsdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createElastiCacheClient returns an ElastiCache client pointed at the shared test container.
func createElastiCacheClient(t *testing.T) *elasticachesdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return elasticachesdk.NewFromConfig(cfg, func(o *elasticachesdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createELBv2Client returns an ELBv2 (Elastic Load Balancing v2) client pointed at the shared test container.
func createELBv2Client(t *testing.T) *elbv2sdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return elbv2sdk.NewFromConfig(cfg, func(o *elbv2sdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createELBClient returns a Classic ELB (Elastic Load Balancing v1) client pointed at the shared test container.
func createELBClient(t *testing.T) *elbsdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return elbsdk.NewFromConfig(cfg, func(o *elbsdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createGlueClient returns an AWS Glue client pointed at the shared test container.
func createGlueClient(t *testing.T) *gluesdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return gluesdk.NewFromConfig(cfg, func(o *gluesdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createBackupClient returns an AWS Backup client pointed at the shared test container.
func createBackupClient(t *testing.T) *backupsdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return backupsdk.NewFromConfig(cfg, func(o *backupsdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createCodeBuildClient returns an AWS CodeBuild client pointed at the shared test container.
func createCodeBuildClient(t *testing.T) *codebuildsdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return codebuildsdk.NewFromConfig(cfg, func(o *codebuildsdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createCodeDeployClient returns a CodeDeploy client pointed at the shared test container.
func createCodeDeployClient(t *testing.T) *codedeploysdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return codedeploysdk.NewFromConfig(cfg, func(o *codedeploysdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createTransferClient returns a Transfer Family client pointed at the shared test container.
func createTransferClient(t *testing.T) *transfersdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return transfersdk.NewFromConfig(cfg, func(o *transfersdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createAthenaClient returns an Athena client pointed at the shared test container.
func createAthenaClient(t *testing.T) *athenasdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return athenasdk.NewFromConfig(cfg, func(o *athenasdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createCodeArtifactClient returns a CodeArtifact client pointed at the shared test container.
func createCodeArtifactClient(t *testing.T) *codeartifactsdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return codeartifactsdk.NewFromConfig(cfg, func(o *codeartifactsdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createXRayClient returns an X-Ray client pointed at the shared test container.
func createXRayClient(t *testing.T) *xraysdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return xraysdk.NewFromConfig(cfg, func(o *xraysdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createAmplifyClient returns an Amplify client pointed at the shared test container.
func createAmplifyClient(t *testing.T) *amplifysdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return amplifysdk.NewFromConfig(cfg, func(o *amplifysdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createNeptuneClient returns a Neptune client pointed at the shared test container.
func createNeptuneClient(t *testing.T) *neptunesdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return neptunesdk.NewFromConfig(cfg, func(o *neptunesdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createApplicationAutoScalingClient returns an Application Auto Scaling client.
func createApplicationAutoScalingClient(t *testing.T) *applicationautoscalingsdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return applicationautoscalingsdk.NewFromConfig(cfg, func(o *applicationautoscalingsdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createCodeCommitClient returns a CodeCommit client pointed at the shared test container.
func createCodeCommitClient(t *testing.T) *codecommitsdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return codecommitsdk.NewFromConfig(cfg, func(o *codecommitsdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createElasticBeanstalkClient returns an Elastic Beanstalk client.
func createElasticBeanstalkClient(t *testing.T) *elasticbeanstalksdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return elasticbeanstalksdk.NewFromConfig(cfg, func(o *elasticbeanstalksdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createIdentityStoreClient returns an IdentityStore client.
func createIdentityStoreClient(t *testing.T) *identitystoresdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return identitystoresdk.NewFromConfig(cfg, func(o *identitystoresdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createShieldClient returns an AWS Shield client.
func createShieldClient(t *testing.T) *shieldsdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return shieldsdk.NewFromConfig(cfg, func(o *shieldsdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createCodeConnectionsClient returns a CodeConnections client.
func createCodeConnectionsClient(t *testing.T) *codeconnectionssdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return codeconnectionssdk.NewFromConfig(cfg, func(o *codeconnectionssdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createEMRServerlessClient returns an EMR Serverless client.
func createEMRServerlessClient(t *testing.T) *emrserverlesssdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return emrserverlesssdk.NewFromConfig(cfg, func(o *emrserverlesssdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createCodeStarConnectionsClient returns a CodeStar Connections client.
func createCodeStarConnectionsClient(t *testing.T) *codestarconnectionssdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return codestarconnectionssdk.NewFromConfig(cfg, func(o *codestarconnectionssdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createCloudControlClient returns an AWS CloudControl API client.
func createCloudControlClient(t *testing.T) *cloudcontrolsdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return cloudcontrolsdk.NewFromConfig(cfg, func(o *cloudcontrolsdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createSupportClient returns an AWS Support client.
func createSupportClient(t *testing.T) *supportsdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return supportsdk.NewFromConfig(cfg, func(o *supportsdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createCostExplorerClient returns an AWS Cost Explorer client.
func createCostExplorerClient(t *testing.T) *cesdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return cesdk.NewFromConfig(cfg, func(o *cesdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createTimestreamQueryClient returns an AWS Timestream Query client.
func createTimestreamQueryClient(t *testing.T) *timestreamquerysdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return timestreamquerysdk.NewFromConfig(cfg, func(o *timestreamquerysdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createTextractClient returns an AWS Textract client.
func createTextractClient(t *testing.T) *textractsdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return textractsdk.NewFromConfig(cfg, func(o *textractsdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createSSOAdminClient returns an AWS SSO Admin client.
func createSSOAdminClient(t *testing.T) *ssoadminsdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return ssoadminsdk.NewFromConfig(cfg, func(o *ssoadminsdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createRDSClient returns an RDS client pointed at the shared test container.
func createRDSClient(t *testing.T) *rdssdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return rdssdk.NewFromConfig(cfg, func(o *rdssdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createSWFClient returns a SWF client pointed at the shared test container.
func createSWFClient(t *testing.T) *swfsdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return swfsdk.NewFromConfig(cfg, func(o *swfsdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createEC2Client returns an EC2 client pointed at the shared test container.
func createEC2Client(t *testing.T) *ec2sdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return ec2sdk.NewFromConfig(cfg, func(o *ec2sdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createECRClient returns an ECR client pointed at the shared test container.
func createECRClient(t *testing.T) *ecrsdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return ecrsdk.NewFromConfig(cfg, func(o *ecrsdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createSchedulerClient returns an EventBridge Scheduler client pointed at the shared test container.
func createSchedulerClient(t *testing.T) *schedulersdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return schedulersdk.NewFromConfig(cfg, func(o *schedulersdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createACMClient returns an ACM client pointed at the shared test container.
func createACMClient(t *testing.T) *acmsdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return acmsdk.NewFromConfig(cfg, func(o *acmsdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createRoute53Client returns a Route53 client pointed at the shared test container.
func createRoute53Client(t *testing.T) *route53sdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return route53sdk.NewFromConfig(cfg, func(o *route53sdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createRoute53ResolverClient returns a Route53Resolver client pointed at the shared test container.
func createRoute53ResolverClient(t *testing.T) *route53resolversdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return route53resolversdk.NewFromConfig(cfg, func(o *route53resolversdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createECSClient returns an ECS client pointed at the shared test container.
func createECSClient(t *testing.T) *ecssdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return ecssdk.NewFromConfig(cfg, func(o *ecssdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createWafv2Client returns a WAFv2 client pointed at the shared test container.
func createWafv2Client(t *testing.T) *wafv2sdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return wafv2sdk.NewFromConfig(cfg, func(o *wafv2sdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createWAFClient returns an AWS WAF (Classic) client.
func createWAFClient(t *testing.T) *wafsdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return wafsdk.NewFromConfig(cfg, func(o *wafsdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

func createS3TablesClient(t *testing.T) *s3tablesclientsdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return s3tablesclientsdk.NewFromConfig(cfg, func(o *s3tablesclientsdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// dumpContainerLogsOnFailure dumps the container logs to stdout if the test failed.
// Call this with t.Cleanup to automatically dump logs on test failure.
func dumpContainerLogsOnFailure(t *testing.T) {
	t.Helper()

	t.Cleanup(func() {
		cleanupCtx, cancel := cleanupContext(t)
		defer cancel()

		if !t.Failed() {
			return
		}

		if sharedContainer == nil {
			t.Log("Cannot dump logs: container reference not available")

			return
		}

		t.Logf("\n========== CONTAINER LOGS FOR FAILED TEST: %s ==========\n", t.Name())

		logs, err := sharedContainer.Logs(cleanupCtx)
		if err != nil {
			t.Logf("Failed to retrieve container logs: %v", err)

			return
		}
		defer logs.Close()

		logBytes, err := io.ReadAll(logs)
		if err != nil {
			t.Logf("Failed to read container logs: %v", err)

			return
		}

		t.Logf("%s", string(logBytes))
		t.Log("\n========== END CONTAINER LOGS ==========\n")
	})
}

// AssertItem performs a deep comparison between a DynamoDB item and an expected map.
// It automatically unwraps the SDK's internal representation for easier testing.
func AssertItem(t *testing.T, item map[string]types.AttributeValue, expected map[string]any) {
	t.Helper()

	actual := unwrapItem(models.FromSDKItem(item))
	assert.Empty(t, cmp.Diff(expected, actual), "Item mismatch")
}

func unwrapItem(item map[string]any) map[string]any {
	res := make(map[string]any)
	for k, v := range item {
		res[k] = unwrapValue(v)
	}

	return res
}

func unwrapValue(v any) any {
	unwrapped := dynamoattr.UnwrapAttributeValue(v)

	switch val := unwrapped.(type) {
	case map[string]any:
		res := make(map[string]any)
		for mk, mv := range val {
			res[mk] = unwrapValue(mv)
		}

		return res
	case []any:
		res := make([]any, len(val))
		for i, iv := range val {
			res[i] = unwrapValue(iv)
		}

		return res
	default:
		return val
	}
}

// createCognitoIDPClient returns a Cognito IDP client pointed at the shared test container.
func createCognitoIDPClient(t *testing.T) *cognitoidpsdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return cognitoidpsdk.NewFromConfig(cfg, func(o *cognitoidpsdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createAppSyncClient returns an AppSync management client pointed at the shared test container.
func createAppSyncClient(t *testing.T) *appsyncsdkv2.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return appsyncsdkv2.NewFromConfig(cfg, func(o *appsyncsdkv2.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createCognitoIdentityClient returns a Cognito Identity client pointed at the shared test container.
func createCognitoIdentityClient(t *testing.T) *cognitoidentitysdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return cognitoidentitysdk.NewFromConfig(cfg, func(o *cognitoidentitysdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createBedrockClient returns a Bedrock client pointed at the shared test container.
func createBedrockClient(t *testing.T) *bedrocksdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return bedrocksdk.NewFromConfig(cfg, func(o *bedrocksdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createElasticsearchClient returns an Elasticsearch client pointed at the shared test container.
func createElasticsearchClient(t *testing.T) *elasticsearchsdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return elasticsearchsdk.NewFromConfig(cfg, func(o *elasticsearchsdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createAutoScalingClient returns an AutoScaling client pointed at the shared test container.
func createAutoScalingClient(t *testing.T) *autoscalingsdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return autoscalingsdk.NewFromConfig(cfg, func(o *autoscalingsdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createCloudFrontClient returns a CloudFront client pointed at the shared test container.
func createCloudFrontClient(t *testing.T) *cloudfrontsdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return cloudfrontsdk.NewFromConfig(cfg, func(o *cloudfrontsdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createCloudTrailClient returns a CloudTrail client pointed at the shared test container.
func createCloudTrailClient(t *testing.T) *cloudtrailsdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return cloudtrailsdk.NewFromConfig(cfg, func(o *cloudtrailsdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createEKSClient returns an EKS client pointed at the shared test container.
func createEKSClient(t *testing.T) *ekssdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return ekssdk.NewFromConfig(cfg, func(o *ekssdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createEFSClient returns an EFS client pointed at the shared test container.
func createEFSClient(t *testing.T) *efssdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return efssdk.NewFromConfig(cfg, func(o *efssdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createDocDBClient returns a DocumentDB client pointed at the shared test container.
func createDocDBClient(t *testing.T) *docdbsdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return docdbsdk.NewFromConfig(cfg, func(o *docdbsdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createBatchClient returns a Batch client pointed at the shared test container.
func createBatchClient(t *testing.T) *batchsdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return batchsdk.NewFromConfig(cfg, func(o *batchsdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createCodePipelineClient returns a CodePipeline client pointed at the shared test container.
func createCodePipelineClient(t *testing.T) *codepipelinesdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return codepipelinesdk.NewFromConfig(cfg, func(o *codepipelinesdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createEMRClient returns an EMR client pointed at the shared test container.
func createEMRClient(t *testing.T) *emrsdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return emrsdk.NewFromConfig(cfg, func(o *emrsdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// resetGopherstackState calls POST /_gopherstack/reset and verifies the response.
// It is called from TestMain before any test runs to ensure clean state.
func resetGopherstackState(ep string) error {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, ep+"/_gopherstack/reset", nil)
	if err != nil {
		return fmt.Errorf("build reset request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("call reset endpoint: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: %d", ErrResetFailed, resp.StatusCode)
	}

	return nil
}

// createServerlessRepoClient returns a Serverless Application Repository client pointed at the shared test container.
func createServerlessRepoClient(t *testing.T) *sarsdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		require.NoError(t, err, "unable to load SDK config")
	}

	return sarsdk.NewFromConfig(cfg, func(o *sarsdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createTimestreamWriteClient returns a Timestream Write client pointed at the shared test container.
// Endpoint discovery is disabled so the client uses the provided BaseEndpoint directly.
func createTimestreamWriteClient(t *testing.T) *timestreamwritesdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return timestreamwritesdk.NewFromConfig(cfg, func(o *timestreamwritesdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.EndpointDiscovery.EnableEndpointDiscovery = aws.EndpointDiscoveryDisabled
	})
}

// createAPIGatewayV2Client returns an API Gateway v2 client pointed at the shared test container.
func createAPIGatewayV2Client(t *testing.T) *apigwv2sdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return apigwv2sdk.NewFromConfig(cfg, func(o *apigwv2sdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

func createFirehoseClient(t *testing.T) *firehosesdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return firehosesdk.NewFromConfig(cfg, func(o *firehosesdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

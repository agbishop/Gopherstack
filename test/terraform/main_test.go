package terraform_test

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/blackbirdworks/gopherstack/test/internal/buildcheck"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	acmsvc "github.com/aws/aws-sdk-go-v2/service/acm"
	acmpcasvc "github.com/aws/aws-sdk-go-v2/service/acmpca"
	amplifysdkv2 "github.com/aws/aws-sdk-go-v2/service/amplify"
	apigwsvc "github.com/aws/aws-sdk-go-v2/service/apigateway"
	apigwv2svc "github.com/aws/aws-sdk-go-v2/service/apigatewayv2"
	appconfigsvc "github.com/aws/aws-sdk-go-v2/service/appconfig"
	appconfigdatasvc "github.com/aws/aws-sdk-go-v2/service/appconfigdata"
	applicationautoscalingsvc "github.com/aws/aws-sdk-go-v2/service/applicationautoscaling"
	apprunnersdkv2 "github.com/aws/aws-sdk-go-v2/service/apprunner"
	appsyncsdkv2 "github.com/aws/aws-sdk-go-v2/service/appsync"
	athenasdkv2 "github.com/aws/aws-sdk-go-v2/service/athena"
	autoscalingsvc "github.com/aws/aws-sdk-go-v2/service/autoscaling"
	backupsvc "github.com/aws/aws-sdk-go-v2/service/backup"
	batchsvc "github.com/aws/aws-sdk-go-v2/service/batch"
	bedrocksvc "github.com/aws/aws-sdk-go-v2/service/bedrock"
	bedrockagentsvc "github.com/aws/aws-sdk-go-v2/service/bedrockagent"
	bedrockruntimesvc "github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	cloudcontrolsvc "github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	cfnsvc "github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cloudfrontsvc "github.com/aws/aws-sdk-go-v2/service/cloudfront"
	cloudtrailsvc "github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	cwsvc "github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwlogssvc "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	codeartifactsvc "github.com/aws/aws-sdk-go-v2/service/codeartifact"
	codebuildsvc "github.com/aws/aws-sdk-go-v2/service/codebuild"
	codecommitsvc "github.com/aws/aws-sdk-go-v2/service/codecommit"
	codeconnectionssvc "github.com/aws/aws-sdk-go-v2/service/codeconnections"
	codedeploysvc "github.com/aws/aws-sdk-go-v2/service/codedeploy"
	codepipelinesvc "github.com/aws/aws-sdk-go-v2/service/codepipeline"
	codestarconnectionssvc "github.com/aws/aws-sdk-go-v2/service/codestarconnections"
	cognitoidentitysvc "github.com/aws/aws-sdk-go-v2/service/cognitoidentity"
	cognitoidpsvc "github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	comprehendsvc "github.com/aws/aws-sdk-go-v2/service/comprehend"
	configsvc "github.com/aws/aws-sdk-go-v2/service/configservice"
	cesvc "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	dmssvc "github.com/aws/aws-sdk-go-v2/service/databasemigrationservice"
	databrewsvc "github.com/aws/aws-sdk-go-v2/service/databrew"
	datasyncsvc "github.com/aws/aws-sdk-go-v2/service/datasync"
	detectivesvc "github.com/aws/aws-sdk-go-v2/service/detective"
	directoryservicesvc "github.com/aws/aws-sdk-go-v2/service/directoryservice"
	docdbsvc "github.com/aws/aws-sdk-go-v2/service/docdb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamodbstreamssvc "github.com/aws/aws-sdk-go-v2/service/dynamodbstreams"
	ec2svc "github.com/aws/aws-sdk-go-v2/service/ec2"
	ecrsvc "github.com/aws/aws-sdk-go-v2/service/ecr"
	ecssvc "github.com/aws/aws-sdk-go-v2/service/ecs"
	efssvc "github.com/aws/aws-sdk-go-v2/service/efs"
	ekssvc "github.com/aws/aws-sdk-go-v2/service/eks"
	elasticachesvc "github.com/aws/aws-sdk-go-v2/service/elasticache"
	elasticbeanstalksvc "github.com/aws/aws-sdk-go-v2/service/elasticbeanstalk"
	elbsvc "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing"
	elbv2svc "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elasticsearchsvc "github.com/aws/aws-sdk-go-v2/service/elasticsearchservice"
	emrsvc "github.com/aws/aws-sdk-go-v2/service/emr"
	emrserverlesssvc "github.com/aws/aws-sdk-go-v2/service/emrserverless"
	ebsvc "github.com/aws/aws-sdk-go-v2/service/eventbridge"
	firehosesvc "github.com/aws/aws-sdk-go-v2/service/firehose"
	fissvc "github.com/aws/aws-sdk-go-v2/service/fis"
	forecastsvc "github.com/aws/aws-sdk-go-v2/service/forecast"
	glaciersvc "github.com/aws/aws-sdk-go-v2/service/glacier"
	gluesvc "github.com/aws/aws-sdk-go-v2/service/glue"
	iamsvc "github.com/aws/aws-sdk-go-v2/service/iam"
	identitystoresvc "github.com/aws/aws-sdk-go-v2/service/identitystore"
	iotsvc "github.com/aws/aws-sdk-go-v2/service/iot"
	kafkasvc "github.com/aws/aws-sdk-go-v2/service/kafka"
	kinesissvc "github.com/aws/aws-sdk-go-v2/service/kinesis"
	kinesisanalyticssvc "github.com/aws/aws-sdk-go-v2/service/kinesisanalytics"
	kinesisanalyticsv2svc "github.com/aws/aws-sdk-go-v2/service/kinesisanalyticsv2"
	kmssvc "github.com/aws/aws-sdk-go-v2/service/kms"
	lakeformationsvc "github.com/aws/aws-sdk-go-v2/service/lakeformation"
	lambdasvc "github.com/aws/aws-sdk-go-v2/service/lambda"
	macie2svc "github.com/aws/aws-sdk-go-v2/service/macie2"
	mediaconvertsvc "github.com/aws/aws-sdk-go-v2/service/mediaconvert"
	medialivesvcc "github.com/aws/aws-sdk-go-v2/service/medialive"
	mediapackagesvc "github.com/aws/aws-sdk-go-v2/service/mediapackage"
	mediastoresvc "github.com/aws/aws-sdk-go-v2/service/mediastore"
	mediastoredatasvc "github.com/aws/aws-sdk-go-v2/service/mediastoredata"
	mediatailorsvc "github.com/aws/aws-sdk-go-v2/service/mediatailor"
	memorydbsvc "github.com/aws/aws-sdk-go-v2/service/memorydb"
	mqsvc "github.com/aws/aws-sdk-go-v2/service/mq"
	mwaasvc "github.com/aws/aws-sdk-go-v2/service/mwaa"
	neptunesvc "github.com/aws/aws-sdk-go-v2/service/neptune"
	networkmonitorsvc "github.com/aws/aws-sdk-go-v2/service/networkmonitor"
	opensearchsvc "github.com/aws/aws-sdk-go-v2/service/opensearch"
	organizationssvc "github.com/aws/aws-sdk-go-v2/service/organizations"
	personalizesvc "github.com/aws/aws-sdk-go-v2/service/personalize"
	pinpointsvc "github.com/aws/aws-sdk-go-v2/service/pinpoint"
	pipessvc "github.com/aws/aws-sdk-go-v2/service/pipes"
	pollysvc "github.com/aws/aws-sdk-go-v2/service/polly"
	quicksightsvc "github.com/aws/aws-sdk-go-v2/service/quicksight"
	ramsvc "github.com/aws/aws-sdk-go-v2/service/ram"
	rdssvc "github.com/aws/aws-sdk-go-v2/service/rds"
	rdsdatasvc "github.com/aws/aws-sdk-go-v2/service/rdsdata"
	redshiftsvc "github.com/aws/aws-sdk-go-v2/service/redshift"
	redshiftdatasvc "github.com/aws/aws-sdk-go-v2/service/redshiftdata"
	rekognitionsvc "github.com/aws/aws-sdk-go-v2/service/rekognition"
	resourcegroupssvc "github.com/aws/aws-sdk-go-v2/service/resourcegroups"
	taggingsvc "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	rolesanywheresvc "github.com/aws/aws-sdk-go-v2/service/rolesanywhere"
	route53svc "github.com/aws/aws-sdk-go-v2/service/route53"
	route53resolversvc "github.com/aws/aws-sdk-go-v2/service/route53resolver"
	s3svc "github.com/aws/aws-sdk-go-v2/service/s3"
	s3controlsvc "github.com/aws/aws-sdk-go-v2/service/s3control"
	s3tablessvc "github.com/aws/aws-sdk-go-v2/service/s3tables"
	sagemakersvc "github.com/aws/aws-sdk-go-v2/service/sagemaker"
	sagemakerruntimesvc "github.com/aws/aws-sdk-go-v2/service/sagemakerruntime"
	schedulersvc "github.com/aws/aws-sdk-go-v2/service/scheduler"
	secretssvc "github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	servicediscoverysvc "github.com/aws/aws-sdk-go-v2/service/servicediscovery"
	sessvc "github.com/aws/aws-sdk-go-v2/service/ses"
	sfnsvc "github.com/aws/aws-sdk-go-v2/service/sfn"
	snssvc "github.com/aws/aws-sdk-go-v2/service/sns"
	sqssvc "github.com/aws/aws-sdk-go-v2/service/sqs"
	ssmsvc "github.com/aws/aws-sdk-go-v2/service/ssm"
	ssoadminsvc "github.com/aws/aws-sdk-go-v2/service/ssoadmin"
	stssvc "github.com/aws/aws-sdk-go-v2/service/sts"
	supportsvc "github.com/aws/aws-sdk-go-v2/service/support"
	swfsvc "github.com/aws/aws-sdk-go-v2/service/swf"
	timestreamquerysvc "github.com/aws/aws-sdk-go-v2/service/timestreamquery"
	transcribesvc "github.com/aws/aws-sdk-go-v2/service/transcribe"
	transfersvc "github.com/aws/aws-sdk-go-v2/service/transfer"
	translatesvc "github.com/aws/aws-sdk-go-v2/service/translate"
	verifiedpermissionssvc "github.com/aws/aws-sdk-go-v2/service/verifiedpermissions"
	vpclatticesvc "github.com/aws/aws-sdk-go-v2/service/vpclattice"
	workmailsvc "github.com/aws/aws-sdk-go-v2/service/workmail"
	xraysvc "github.com/aws/aws-sdk-go-v2/service/xray"

	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// endpoint is the base URL for the running Gopherstack container.
//
//nolint:gochecknoglobals // Set in TestMain for terraform tests.
var endpoint string

// sharedContainer holds a reference to the container for cleanup and log dumping on test failures.
//
//nolint:gochecknoglobals // Set in TestMain for terraform tests.
var sharedContainer testcontainers.Container

// ErrDockerPanic is returned when the Docker availability check panics.
var ErrDockerPanic = errors.New("docker check panicked")

func TestMain(m *testing.M) {
	flag.Parse()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	if testing.Short() {
		logger.Info("skipping terraform tests in short mode")
		os.Exit(0)
	}

	if err := checkDocker(); err != nil {
		logger.Error("terraform tests require docker", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()

	// Use the lightweight Dockerfile.test when a pre-built binary exists
	// (e.g. from CI or `go build`), otherwise fall back to the full
	// multi-stage Dockerfile that compiles from source.
	dockerfile := "Dockerfile"
	binPath := "../../bin/gopherstack-linux"

	// If we are on Mac, we MUST build a Linux binary for the container.
	if runtime.GOOS == "darwin" {
		logger.Info("running on Darwin, building Linux binary for container tests...")
		// Use relative path but run from the directory containing go.mod to ensure embed works.
		cmd := exec.Command("go", "build", "-trimpath", "-o", "bin/gopherstack-linux", ".")
		cmd.Dir = "../../"
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOTOOLCHAIN=local")
		if out, err := cmd.CombinedOutput(); err != nil {
			logger.Error("failed to build linux binary", "error", err, "output", string(out))
			os.Exit(1)
		}
	}

	if binInfo, err := os.Stat(binPath); err == nil {
		if freshErr := buildcheck.CheckFreshness(logger, binInfo); freshErr != nil {
			logger.Error(freshErr.Error())
			os.Exit(1)
		}

		dockerfile = "Dockerfile.test"
		logger.Info("using pre-built binary via Dockerfile.test")
	} else {
		logger.Info("no pre-built binary found, building from source via Dockerfile")
	}

	req := testcontainers.ContainerRequest{
		Context:       "../../",
		Dockerfile:    dockerfile,
		PrintBuildLog: true,
		BuildOptionsModifier: func(options *client.ImageBuildOptions) {
			options.NoCache = false
			options.PullParent = false
		},
		AutoRemove:   true,
		ExposedPorts: []string{"8000/tcp"},
		WaitingFor: wait.ForHTTP("/").
			WithStatusCodeMatcher(func(_ int) bool { return true }).
			WithStartupTimeout(60 * time.Second),
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

	// Pre-download the tofu binary once in single-threaded setup so that no
	// parallel test pays the download cost.
	initTofuBinary(logger)

	// Warm the shared provider cache with a single tofu init so that parallel
	// tests don't all race to download the ~300 MB hashicorp/aws provider.
	warmProviderCache(logger)

	code := m.Run()

	// Clean up pre-initialized directories kept open for parallel tests.
	for _, d := range []string{preInitDirMain, preInitDirRDS, preInitDirDocDB, preInitDirNeptune} {
		if d != "" {
			os.RemoveAll(d)
		}
	}

	if tErr := container.Terminate(ctx); tErr != nil {
		logger.Error("failed to terminate container", "error", tErr)
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

// createS3Client returns an S3 client pointed at the shared test container.
func createS3Client(t *testing.T) *s3svc.Client {
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

	return s3svc.NewFromConfig(cfg, func(o *s3svc.Options) {
		o.UsePathStyle = true
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createSQSClient returns an SQS client pointed at the shared test container.
func createSQSClient(t *testing.T) *sqssvc.Client {
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

	return sqssvc.NewFromConfig(cfg, func(o *sqssvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createRDSClient returns an RDS client pointed at the shared test container.
func createRDSClient(t *testing.T) *rdssvc.Client {
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

	return rdssvc.NewFromConfig(cfg, func(o *rdssvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createDocDBClient returns a DocDB client pointed at the shared test container.
func createDocDBClient(t *testing.T) *docdbsvc.Client {
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

	return docdbsvc.NewFromConfig(cfg, func(o *docdbsvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createNeptuneClient returns a Neptune client pointed at the shared test container.
func createNeptuneClient(t *testing.T) *neptunesvc.Client {
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

	return neptunesvc.NewFromConfig(cfg, func(o *neptunesvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createIAMClient returns an IAM client pointed at the shared test container.
func createIAMClient(t *testing.T) *iamsvc.Client {
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

	return iamsvc.NewFromConfig(cfg, func(o *iamsvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createKMSClient returns a KMS client pointed at the shared test container.
func createKMSClient(t *testing.T) *kmssvc.Client {
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

	return kmssvc.NewFromConfig(cfg, func(o *kmssvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createSNSClient returns an SNS client pointed at the shared test container.
func createSNSClient(t *testing.T) *snssvc.Client {
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

	return snssvc.NewFromConfig(cfg, func(o *snssvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createSecretsManagerClient returns a SecretsManager client pointed at the shared test container.
func createSecretsManagerClient(t *testing.T) *secretssvc.Client {
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

	return secretssvc.NewFromConfig(cfg, func(o *secretssvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createSSMClient returns an SSM client pointed at the shared test container.
func createSSMClient(t *testing.T) *ssmsvc.Client {
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

	return ssmsvc.NewFromConfig(cfg, func(o *ssmsvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createCloudWatchLogsClient returns a CloudWatchLogs client pointed at the shared test container.
func createCloudWatchLogsClient(t *testing.T) *cwlogssvc.Client {
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

	return cwlogssvc.NewFromConfig(cfg, func(o *cwlogssvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createRoute53Client returns a Route53 client pointed at the shared test container.
func createRoute53Client(t *testing.T) *route53svc.Client {
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

	return route53svc.NewFromConfig(cfg, func(o *route53svc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createLambdaClient returns a Lambda client pointed at the shared test container.
func createLambdaClient(t *testing.T) *lambdasvc.Client {
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

	return lambdasvc.NewFromConfig(cfg, func(o *lambdasvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createSESClient returns an SES client pointed at the shared test container.
func createSESClient(t *testing.T) *sessvc.Client {
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

	return sessvc.NewFromConfig(cfg, func(o *sessvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createSFNClient returns a StepFunctions client pointed at the shared test container.
func createSFNClient(t *testing.T) *sfnsvc.Client {
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

	return sfnsvc.NewFromConfig(cfg, func(o *sfnsvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createEventBridgeClient returns an EventBridge client pointed at the shared test container.
func createEventBridgeClient(t *testing.T) *ebsvc.Client {
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

	return ebsvc.NewFromConfig(cfg, func(o *ebsvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createCloudWatchClient returns a CloudWatch client pointed at the shared test container.
func createCloudWatchClient(t *testing.T) *cwsvc.Client {
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

	return cwsvc.NewFromConfig(cfg, func(o *cwsvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createKinesisClient returns a Kinesis client pointed at the shared test container.
func createKinesisClient(t *testing.T) *kinesissvc.Client {
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

	return kinesissvc.NewFromConfig(cfg, func(o *kinesissvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createACMClient returns an ACM client pointed at the shared test container.
func createACMClient(t *testing.T) *acmsvc.Client {
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

	return acmsvc.NewFromConfig(cfg, func(o *acmsvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createACMPCAClient returns an ACM PCA client pointed at the shared test container.
func createACMPCAClient(t *testing.T) *acmpcasvc.Client {
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

	return acmpcasvc.NewFromConfig(cfg, func(o *acmpcasvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createCloudFormationClient returns a CloudFormation client pointed at the shared test container.
func createCloudFormationClient(t *testing.T) *cfnsvc.Client {
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

	return cfnsvc.NewFromConfig(cfg, func(o *cfnsvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createElastiCacheClient returns an ElastiCache client pointed at the shared test container.
func createElastiCacheClient(t *testing.T) *elasticachesvc.Client {
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

	return elasticachesvc.NewFromConfig(cfg, func(o *elasticachesvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createOpenSearchClient returns an OpenSearch client pointed at the shared test container.
func createOpenSearchClient(t *testing.T) *opensearchsvc.Client {
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

	return opensearchsvc.NewFromConfig(cfg, func(o *opensearchsvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createRedshiftClient returns a Redshift client pointed at the shared test container.
func createRedshiftClient(t *testing.T) *redshiftsvc.Client {
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

	return redshiftsvc.NewFromConfig(cfg, func(o *redshiftsvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createFirehoseClient returns a Firehose client pointed at the shared test container.
func createFirehoseClient(t *testing.T) *firehosesvc.Client {
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

	return firehosesvc.NewFromConfig(cfg, func(o *firehosesvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// initTofuBinary eagerly resolves the tofu binary path (downloading it if
// necessary) during single-threaded TestMain setup. This ensures that no
// parallel test blocks waiting for the [sync.Once]-guarded download.
func initTofuBinary(logger *slog.Logger) {
	tofuBinaryOnce.Do(func() {
		if path, err := exec.LookPath("tofu"); err == nil {
			tofuBinaryPath = path

			return
		}

		logger.Info("tofu not found in PATH; downloading from OpenTofu releases...")

		tofuBinaryPath, errTofuBinary = downloadTofuBinary(logger)
	})

	if errTofuBinary != nil {
		logger.Error("could not obtain tofu binary", "error", errTofuBinary)
		os.Exit(1)
	}
}

// warmProviderCache runs a single tofu init to ensure the shared provider cache
// is populated before parallel tests start. This avoids 8+ concurrent tests all
// racing to download the ~300 MB hashicorp/aws provider simultaneously.
// It also keeps the initialized directories so applyTofu can hard-link the
// .terraform/ directory tree instead of re-running init (which serializes on
// the plugin-cache file lock).
func warmProviderCache(logger *slog.Logger) {
	if tofuBinaryPath == "" {
		logger.Warn("skipping provider cache warm-up: tofu binary not available")

		return
	}

	if mkdirErr := os.MkdirAll(tofuProviderCacheDir, 0o755); mkdirErr != nil {
		logger.Warn("skipping provider cache warm-up", "error", mkdirErr)

		return
	}

	// Warm all provider block variants used by tests so no test pays the
	// first-access initialization cost.
	//
	// The main block is warmed first. Its generated .terraform.lock.hcl is
	// seeded into subsequent warm-up directories so that tofu init can
	// resolve provider checksums from the local plugin cache without
	// contacting registry.opentofu.org. All variants pin the same
	// hashicorp/aws version so the lock file is reusable across them.
	preInitDirMain = warmWithHCL(
		tofuBinaryPath,
		tofuProviderCacheDir,
		providerBlock(endpoint),
		nil,
		logger,
	)

	// Read the lock file produced by the main warm-up so that subsequent
	// calls can skip the registry entirely.
	var seedLockFile []byte

	if preInitDirMain != "" {
		if data, readErr := os.ReadFile(filepath.Join(preInitDirMain, ".terraform.lock.hcl")); readErr == nil {
			seedLockFile = data
		}
	}

	preInitDirRDS = warmWithHCL(
		tofuBinaryPath, tofuProviderCacheDir, rdsProviderBlock(endpoint), seedLockFile, logger,
	)
	preInitDirDocDB = warmWithHCL(
		tofuBinaryPath, tofuProviderCacheDir, docdbProviderBlock(endpoint), seedLockFile, logger,
	)
	preInitDirNeptune = warmWithHCL(
		tofuBinaryPath, tofuProviderCacheDir, neptuneProviderBlock(endpoint), seedLockFile, logger,
	)
}

// warmWithHCL runs `tofu init` in a temporary directory with the given HCL to
// populate the shared provider cache and produce a fully initialized .terraform/
// subtree (including .terraform/terraform.tfstate). It returns the directory path
// so callers can reuse the initialized .terraform/ subtree via hardLinkDir; the
// caller is responsible for cleanup (os.RemoveAll). Returns an empty string on
// failure.
//
// seedLockFile, if non-nil, is written as .terraform.lock.hcl before init so
// that tofu can resolve provider checksums from the local plugin cache without
// needing to contact registry.opentofu.org.
func warmWithHCL(tofuBin, cacheDir, hcl string, seedLockFile []byte, logger *slog.Logger) string {
	dir, err := os.MkdirTemp("", "tofu-warmup-*")
	if err != nil {
		logger.Warn("skipping provider cache warm-up", "error", err)

		return ""
	}

	if writeErr := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(hcl), 0o644); writeErr != nil {
		logger.Warn("skipping provider cache warm-up", "error", writeErr)
		if rmErr := os.RemoveAll(dir); rmErr != nil {
			logger.Warn("failed to remove warm-up temp dir", "dir", dir, "error", rmErr)
		}

		return ""
	}

	// Seed the lock file when provided so that tofu init can skip the
	// provider registry and resolve checksums from the on-disk cache.
	if seedLockFile != nil {
		if writeErr := os.WriteFile(filepath.Join(dir, ".terraform.lock.hcl"), seedLockFile, 0o644); writeErr != nil {
			logger.Warn("failed to seed lock file for warm-up", "error", writeErr)
			// Non-fatal: tofu init will regenerate it (may need network).
		}
	}

	cmd := exec.Command(tofuBin, "init", "-no-color")
	cmd.Dir = dir
	cmd.Env = append(
		os.Environ(),
		"TF_IN_AUTOMATION=1",
		"TF_PLUGIN_CACHE_DIR="+cacheDir,
		"TF_PLUGIN_CACHE_MAY_BREAK_DEPENDENCY_LOCK_FILE=true",
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		logger.Warn("provider cache warm-up failed", "error", err, "output", string(out))
		if rmErr := os.RemoveAll(dir); rmErr != nil {
			logger.Warn("failed to remove warm-up temp dir", "dir", dir, "error", rmErr)
		}

		return ""
	}

	logger.Info("provider cache warmed successfully")

	return dir
}

// dumpContainerLogsOnFailure dumps the container logs to stdout if the test failed.
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

// createEC2Client returns an EC2 client pointed at the shared test container.
func createEC2Client(t *testing.T) *ec2svc.Client {
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

	return ec2svc.NewFromConfig(cfg, func(o *ec2svc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createAPIGatewayClient returns an API Gateway client pointed at the shared test container.
func createAPIGatewayClient(t *testing.T) *apigwsvc.Client {
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

	return apigwsvc.NewFromConfig(cfg, func(o *apigwsvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createAPIGatewayV2Client returns an API Gateway V2 client pointed at the shared test container.
func createAPIGatewayV2Client(t *testing.T) *apigwv2svc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return apigwv2svc.NewFromConfig(cfg, func(o *apigwv2svc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createSchedulerClient returns a Scheduler client pointed at the shared test container.
func createSchedulerClient(t *testing.T) *schedulersvc.Client {
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

	return schedulersvc.NewFromConfig(cfg, func(o *schedulersvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createRoute53ResolverClient returns a Route53 Resolver client pointed at the shared test container.
func createRoute53ResolverClient(t *testing.T) *route53resolversvc.Client {
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

	return route53resolversvc.NewFromConfig(cfg, func(o *route53resolversvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createS3ControlClient returns an S3 Control client pointed at the shared test container.
func createS3ControlClient(t *testing.T) *s3controlsvc.Client {
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

	return s3controlsvc.NewFromConfig(cfg, func(o *s3controlsvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createAWSConfigClient returns an AWS Config client pointed at the shared test container.
func createAWSConfigClient(t *testing.T) *configsvc.Client {
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

	return configsvc.NewFromConfig(cfg, func(o *configsvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createResourceGroupsClient returns a Resource Groups client pointed at the shared test container.
func createResourceGroupsClient(t *testing.T) *resourcegroupssvc.Client {
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

	return resourcegroupssvc.NewFromConfig(cfg, func(o *resourcegroupssvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createResourceGroupsTaggingAPIClient returns a Resource Groups Tagging API client
// pointed at the shared test container.
func createResourceGroupsTaggingAPIClient(t *testing.T) *taggingsvc.Client {
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

	return taggingsvc.NewFromConfig(cfg, func(o *taggingsvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createSWFClient returns an SWF client pointed at the shared test container.
func createSWFClient(t *testing.T) *swfsvc.Client {
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

	return swfsvc.NewFromConfig(cfg, func(o *swfsvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createAppSyncClient returns an AppSync client pointed at the shared test container.
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

// createECRClient returns an ECR client pointed at the shared test container.
func createECRClient(t *testing.T) *ecrsvc.Client {
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

	return ecrsvc.NewFromConfig(cfg, func(o *ecrsvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createECSClient returns an ECS client pointed at the shared test container.
func createECSClient(t *testing.T) *ecssvc.Client {
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

	return ecssvc.NewFromConfig(cfg, func(o *ecssvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createEKSClient returns an EKS client pointed at the shared test container.
func createEKSClient(t *testing.T) *ekssvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return ekssvc.NewFromConfig(cfg, func(o *ekssvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createCognitoIdentityClient returns a Cognito Identity client pointed at the shared test container.
func createCognitoIdentityClient(t *testing.T) *cognitoidentitysvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return cognitoidentitysvc.NewFromConfig(cfg, func(o *cognitoidentitysvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createCognitoIDPClient returns a Cognito IDP client pointed at the shared test container.
func createCognitoIDPClient(t *testing.T) *cognitoidpsvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return cognitoidpsvc.NewFromConfig(cfg, func(o *cognitoidpsvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createIoTClient returns an IoT client pointed at the shared test container.
func createIoTClient(t *testing.T) *iotsvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return iotsvc.NewFromConfig(cfg, func(o *iotsvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createSTSClient returns an STS client pointed at the shared test container.
func createSTSClient(t *testing.T) *stssvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return stssvc.NewFromConfig(cfg, func(o *stssvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createSupportClient returns a Support client pointed at the shared test container.
func createSupportClient(t *testing.T) *supportsvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return supportsvc.NewFromConfig(cfg, func(o *supportsvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createAmplifyClient returns an Amplify client pointed at the shared test container.
func createAmplifyClient(t *testing.T) *amplifysdkv2.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return amplifysdkv2.NewFromConfig(cfg, func(o *amplifysdkv2.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createAutoscalingClient returns an Autoscaling client pointed at the shared test container.
func createAutoscalingClient(t *testing.T) *autoscalingsvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return autoscalingsvc.NewFromConfig(cfg, func(o *autoscalingsvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createAppConfigClient returns an AppConfig client pointed at the shared test container.
func createAppConfigClient(t *testing.T) *appconfigsvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return appconfigsvc.NewFromConfig(cfg, func(o *appconfigsvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createAppConfigDataClient returns an AppConfigData client pointed at the shared test container.
func createAppConfigDataClient(t *testing.T) *appconfigdatasvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return appconfigdatasvc.NewFromConfig(cfg, func(o *appconfigdatasvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createApplicationAutoscalingClient returns an Application Auto Scaling client pointed at the shared test container.
func createApplicationAutoscalingClient(t *testing.T) *applicationautoscalingsvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return applicationautoscalingsvc.NewFromConfig(cfg, func(o *applicationautoscalingsvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createAthenaClient returns an Athena client pointed at the shared test container.
func createAthenaClient(t *testing.T) *athenasdkv2.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return athenasdkv2.NewFromConfig(cfg, func(o *athenasdkv2.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createBackupClient returns a Backup client pointed at the shared test container.
func createBackupClient(t *testing.T) *backupsvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return backupsvc.NewFromConfig(cfg, func(o *backupsvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createBatchClient returns a Batch client pointed at the shared test container.
func createBatchClient(t *testing.T) *batchsvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return batchsvc.NewFromConfig(cfg, func(o *batchsvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createCloudTrailClient returns a CloudTrail client pointed at the shared test container.
func createCloudTrailClient(t *testing.T) *cloudtrailsvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return cloudtrailsvc.NewFromConfig(cfg, func(o *cloudtrailsvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createEFSClient returns an EFS client pointed at the shared test container.
func createEFSClient(t *testing.T) *efssvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return efssvc.NewFromConfig(cfg, func(o *efssvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createBedrockClient returns a Bedrock client pointed at the shared test container.
func createBedrockClient(t *testing.T) *bedrocksvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return bedrocksvc.NewFromConfig(cfg, func(o *bedrocksvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createBedrockRuntimeClient returns a Bedrock Runtime client pointed at the shared test container.
func createBedrockRuntimeClient(t *testing.T) *bedrockruntimesvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return bedrockruntimesvc.NewFromConfig(cfg, func(o *bedrockruntimesvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createCodeBuildClient returns a CodeBuild client pointed at the shared test container.
func createCodeBuildClient(t *testing.T) *codebuildsvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return codebuildsvc.NewFromConfig(cfg, func(o *codebuildsvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createCeClient returns a Cost Explorer client pointed at the shared test container.
func createCeClient(t *testing.T) *cesvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return cesvc.NewFromConfig(cfg, func(o *cesvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createCloudControlClient returns a CloudControl API client pointed at the shared test container.
func createCloudControlClient(t *testing.T) *cloudcontrolsvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return cloudcontrolsvc.NewFromConfig(cfg, func(o *cloudcontrolsvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createCloudFrontClient returns a CloudFront client pointed at the shared test container.
func createCloudFrontClient(t *testing.T) *cloudfrontsvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return cloudfrontsvc.NewFromConfig(cfg, func(o *cloudfrontsvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createCodeArtifactClient returns a CodeArtifact client pointed at the shared test container.
func createCodeArtifactClient(t *testing.T) *codeartifactsvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return codeartifactsvc.NewFromConfig(cfg, func(o *codeartifactsvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createCodeConnectionsClient returns a CodeConnections client pointed at the shared test container.
func createCodeConnectionsClient(t *testing.T) *codeconnectionssvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return codeconnectionssvc.NewFromConfig(cfg, func(o *codeconnectionssvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createCodeCommitClient returns a CodeCommit client pointed at the shared test container.
func createCodeCommitClient(t *testing.T) *codecommitsvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return codecommitsvc.NewFromConfig(cfg, func(o *codecommitsvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createCodePipelineClient returns a CodePipeline client pointed at the shared test container.
func createCodePipelineClient(t *testing.T) *codepipelinesvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return codepipelinesvc.NewFromConfig(cfg, func(o *codepipelinesvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createCodeDeployClient returns a CodeDeploy client pointed at the shared test container.
func createCodeDeployClient(t *testing.T) *codedeploysvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return codedeploysvc.NewFromConfig(cfg, func(o *codedeploysvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createDMSClient returns a DMS client pointed at the shared test container.
func createDMSClient(t *testing.T) *dmssvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return dmssvc.NewFromConfig(cfg, func(o *dmssvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createCodeStarConnectionsClient returns a CodeStar Connections client pointed at the shared test container.
func createCodeStarConnectionsClient(t *testing.T) *codestarconnectionssvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return codestarconnectionssvc.NewFromConfig(cfg, func(o *codestarconnectionssvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createDynamoDBStreamsClient returns a DynamoDB Streams client pointed at the shared test container.
func createDynamoDBStreamsClient(t *testing.T) *dynamodbstreamssvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return dynamodbstreamssvc.NewFromConfig(cfg, func(o *dynamodbstreamssvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createElasticbeanstalkClient returns an Elastic Beanstalk client pointed at the shared test container.
func createElasticbeanstalkClient(t *testing.T) *elasticbeanstalksvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return elasticbeanstalksvc.NewFromConfig(cfg, func(o *elasticbeanstalksvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createElasticsearchClient returns an Elasticsearch client pointed at the shared test container.
func createElasticsearchClient(t *testing.T) *elasticsearchsvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return elasticsearchsvc.NewFromConfig(cfg, func(o *elasticsearchsvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createELBClient returns a Classic ELB client pointed at the shared test container.
func createELBClient(t *testing.T) *elbsvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return elbsvc.NewFromConfig(cfg, func(o *elbsvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createEmrServerlessClient returns an EMR Serverless client pointed at the shared test container.
func createEmrServerlessClient(t *testing.T) *emrserverlesssvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return emrserverlesssvc.NewFromConfig(cfg, func(o *emrserverlesssvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createEMRClient returns an EMR client pointed at the shared test container.
func createEMRClient(t *testing.T) *emrsvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return emrsvc.NewFromConfig(cfg, func(o *emrsvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createGlacierClient returns a Glacier client pointed at the shared test container.
func createGlacierClient(t *testing.T) *glaciersvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return glaciersvc.NewFromConfig(cfg, func(o *glaciersvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createELBv2Client returns an ELBv2 client pointed at the shared test container.
func createELBv2Client(t *testing.T) *elbv2svc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return elbv2svc.NewFromConfig(cfg, func(o *elbv2svc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createFISClient returns a FIS client pointed at the shared test container.
func createFISClient(t *testing.T) *fissvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return fissvc.NewFromConfig(cfg, func(o *fissvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

func createGlueClient(t *testing.T) *gluesvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return gluesvc.NewFromConfig(cfg, func(o *gluesvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createIdentityStoreClient returns an Identity Store client pointed at the shared test container.
func createIdentityStoreClient(t *testing.T) *identitystoresvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return identitystoresvc.NewFromConfig(cfg, func(o *identitystoresvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createKinesisAnalyticsClient returns a Kinesis Analytics client pointed at the shared test container.
//

func createKinesisAnalyticsClient(t *testing.T) *kinesisanalyticssvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return kinesisanalyticssvc.NewFromConfig(cfg, func(o *kinesisanalyticssvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createKafkaClient returns an MSK Kafka client pointed at the shared test container.
func createKafkaClient(t *testing.T) *kafkasvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return kafkasvc.NewFromConfig(cfg, func(o *kafkasvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createKinesisAnalyticsV2Client returns a Kinesis Data Analytics v2 client pointed at the shared test container.
func createKinesisAnalyticsV2Client(t *testing.T) *kinesisanalyticsv2svc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return kinesisanalyticsv2svc.NewFromConfig(cfg, func(o *kinesisanalyticsv2svc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createMediaConvertClient returns a MediaConvert client pointed at the shared test container.
func createMediaConvertClient(t *testing.T) *mediaconvertsvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return mediaconvertsvc.NewFromConfig(
		cfg,
		func(o *mediaconvertsvc.Options) {
			o.BaseEndpoint = aws.String(endpoint)
		},
	)
}

// createMQClient returns an Amazon MQ client pointed at the shared test container.
func createMQClient(t *testing.T) *mqsvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return mqsvc.NewFromConfig(
		cfg,
		func(o *mqsvc.Options) {
			o.BaseEndpoint = aws.String(endpoint)
		},
	)
}

// createLakeFormationClient returns a Lake Formation client pointed at the shared test container.
func createLakeFormationClient(t *testing.T) *lakeformationsvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return lakeformationsvc.NewFromConfig(cfg, func(o *lakeformationsvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createMediaStoreClient returns a MediaStore client pointed at the shared test container.
func createMediaStoreClient(t *testing.T) *mediastoresvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return mediastoresvc.NewFromConfig(cfg, func(o *mediastoresvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createMemoryDBClient returns a MemoryDB client pointed at the shared test container.
func createMemoryDBClient(t *testing.T) *memorydbsvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return memorydbsvc.NewFromConfig(cfg, func(o *memorydbsvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createOrganizationsClient returns an Organizations client pointed at the shared test container.
func createOrganizationsClient(t *testing.T) *organizationssvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return organizationssvc.NewFromConfig(cfg, func(o *organizationssvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createMWAAClient returns an MWAA client pointed at the shared test container.
func createMWAAClient(t *testing.T) *mwaasvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return mwaasvc.NewFromConfig(cfg, func(o *mwaasvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createPinpointClient returns a Pinpoint client pointed at the shared test container.
func createPinpointClient(t *testing.T) *pinpointsvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return pinpointsvc.NewFromConfig(cfg, func(o *pinpointsvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createPipesClient returns an EventBridge Pipes client pointed at the shared test container.
func createPipesClient(t *testing.T) *pipessvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return pipessvc.NewFromConfig(cfg, func(o *pipessvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createRDSDataClient returns an RDS Data client pointed at the shared test container.
func createRDSDataClient(t *testing.T) *rdsdatasvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return rdsdatasvc.NewFromConfig(cfg, func(o *rdsdatasvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createRAMClient returns a RAM client pointed at the shared test container.
func createRAMClient(t *testing.T) *ramsvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return ramsvc.NewFromConfig(cfg, func(o *ramsvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createRedshiftDataClient returns a Redshift Data client pointed at the shared test container.
func createRedshiftDataClient(t *testing.T) *redshiftdatasvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return redshiftdatasvc.NewFromConfig(cfg, func(o *redshiftdatasvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createSageMakerClient returns a SageMaker client pointed at the shared test container.
func createSageMakerClient(t *testing.T) *sagemakersvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return sagemakersvc.NewFromConfig(cfg, func(o *sagemakersvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createServiceDiscoveryClient returns a Service Discovery client pointed at the shared test container.
func createServiceDiscoveryClient(t *testing.T) *servicediscoverysvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return servicediscoverysvc.NewFromConfig(cfg, func(o *servicediscoverysvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createSageMakerRuntimeClient returns a SageMaker Runtime client pointed at the shared test container.
func createSageMakerRuntimeClient(t *testing.T) *sagemakerruntimesvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return sagemakerruntimesvc.NewFromConfig(cfg, func(o *sagemakerruntimesvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createSsoAdminClient returns an SSO Admin client pointed at the shared test container.
func createSsoAdminClient(t *testing.T) *ssoadminsvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return ssoadminsvc.NewFromConfig(cfg, func(o *ssoadminsvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createTimestreamQueryClient returns a Timestream Query client pointed at the shared test container.
func createTimestreamQueryClient(t *testing.T) *timestreamquerysvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return timestreamquerysvc.NewFromConfig(cfg, func(o *timestreamquerysvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.EndpointDiscovery.EnableEndpointDiscovery = aws.EndpointDiscoveryDisabled
	})
}

// createTestConfig returns an AWS SDK configuration pointed at the shared test container.
func createTestConfig(t *testing.T) aws.Config {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return cfg
}

// createTransferClient returns a Transfer client pointed at the shared test container.
func createTransferClient(t *testing.T) *transfersvc.Client {
	t.Helper()

	return createClientWithEndpoint(t, transfersvc.NewFromConfig, endpoint)
}

// createVerifiedPermissionsClient returns a Verified Permissions client pointed at the shared test container.
func createVerifiedPermissionsClient(t *testing.T) *verifiedpermissionssvc.Client {
	t.Helper()

	return createClientWithEndpoint(t, verifiedpermissionssvc.NewFromConfig, endpoint)
}

// createXrayClient returns an X-Ray client pointed at the shared test container.
func createXrayClient(t *testing.T) *xraysvc.Client {
	t.Helper()

	return createClientWithEndpoint(t, xraysvc.NewFromConfig, endpoint)
}

// createVPCLatticeClient returns a VPC Lattice client pointed at the shared test container.
func createVPCLatticeClient(t *testing.T) *vpclatticesvc.Client {
	t.Helper()

	return createClientWithEndpoint(t, vpclatticesvc.NewFromConfig, endpoint)
}

// createClientWithEndpoint is a helper to create an AWS client with a base endpoint.
func createClientWithEndpoint[T any, O any](
	t *testing.T,
	newFn func(aws.Config, ...func(*O)) *T,
	endpoint string,
) *T {
	t.Helper()

	return newFn(createTestConfig(t), func(o *O) {
		// Use reflection to set BaseEndpoint because service options are not common interfaces.
		reflect.ValueOf(o).
			Elem().
			FieldByName("BaseEndpoint").
			Set(reflect.ValueOf(aws.String(endpoint)))
	})
}

func createS3TablesClient(t *testing.T) *s3tablessvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return s3tablessvc.NewFromConfig(cfg, func(o *s3tablessvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

func createBedrockAgentClient(t *testing.T) *bedrockagentsvc.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return bedrockagentsvc.NewFromConfig(cfg, func(o *bedrockagentsvc.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

func createAppRunnerClient(t *testing.T) *apprunnersdkv2.Client {
	t.Helper()

	return createClientWithEndpoint(t, apprunnersdkv2.NewFromConfig, endpoint)
}

func createComprehendClient(t *testing.T) *comprehendsvc.Client {
	t.Helper()

	return createClientWithEndpoint(t, comprehendsvc.NewFromConfig, endpoint)
}

func createDataBrewClient(t *testing.T) *databrewsvc.Client {
	t.Helper()

	return createClientWithEndpoint(t, databrewsvc.NewFromConfig, endpoint)
}

func createDataSyncClient(t *testing.T) *datasyncsvc.Client {
	t.Helper()

	return createClientWithEndpoint(t, datasyncsvc.NewFromConfig, endpoint)
}

func createDetectiveClient(t *testing.T) *detectivesvc.Client {
	t.Helper()

	return createClientWithEndpoint(t, detectivesvc.NewFromConfig, endpoint)
}

func createDirectoryServiceClient(t *testing.T) *directoryservicesvc.Client {
	t.Helper()

	return createClientWithEndpoint(t, directoryservicesvc.NewFromConfig, endpoint)
}

func createForecastClient(t *testing.T) *forecastsvc.Client {
	t.Helper()

	return createClientWithEndpoint(t, forecastsvc.NewFromConfig, endpoint)
}

func createMacie2Client(t *testing.T) *macie2svc.Client {
	t.Helper()

	return createClientWithEndpoint(t, macie2svc.NewFromConfig, endpoint)
}

func createMediaLiveClient(t *testing.T) *medialivesvcc.Client {
	t.Helper()

	return createClientWithEndpoint(t, medialivesvcc.NewFromConfig, endpoint)
}

func createMediaPackageClient(t *testing.T) *mediapackagesvc.Client {
	t.Helper()

	return createClientWithEndpoint(t, mediapackagesvc.NewFromConfig, endpoint)
}

// createMediaStoreDataClient returns a mediastoredata client. In gopherstack the
// container data-plane is served at the same base endpoint as all other services,
// so we ignore the container name and point directly at endpoint.
func createMediaStoreDataClient(t *testing.T, _ string) *mediastoredatasvc.Client {
	t.Helper()

	return createClientWithEndpoint(t, mediastoredatasvc.NewFromConfig, endpoint)
}

func createMediaTailorClient(t *testing.T) *mediatailorsvc.Client {
	t.Helper()

	return createClientWithEndpoint(t, mediatailorsvc.NewFromConfig, endpoint)
}

func createPersonalizeClient(t *testing.T) *personalizesvc.Client {
	t.Helper()

	return createClientWithEndpoint(t, personalizesvc.NewFromConfig, endpoint)
}

func createPollyClient(t *testing.T) *pollysvc.Client {
	t.Helper()

	return createClientWithEndpoint(t, pollysvc.NewFromConfig, endpoint)
}

func createQuickSightClient(t *testing.T) *quicksightsvc.Client {
	t.Helper()

	return createClientWithEndpoint(t, quicksightsvc.NewFromConfig, endpoint)
}

func createRekognitionClient(t *testing.T) *rekognitionsvc.Client {
	t.Helper()

	return createClientWithEndpoint(t, rekognitionsvc.NewFromConfig, endpoint)
}

func createRolesAnywhereClient(t *testing.T) *rolesanywheresvc.Client {
	t.Helper()

	return createClientWithEndpoint(t, rolesanywheresvc.NewFromConfig, endpoint)
}

func createTranscribeClient(t *testing.T) *transcribesvc.Client {
	t.Helper()

	return createClientWithEndpoint(t, transcribesvc.NewFromConfig, endpoint)
}

func createTranslateClient(t *testing.T) *translatesvc.Client {
	t.Helper()

	return createClientWithEndpoint(t, translatesvc.NewFromConfig, endpoint)
}

func createWorkMailClient(t *testing.T) *workmailsvc.Client {
	t.Helper()

	return createClientWithEndpoint(t, workmailsvc.NewFromConfig, endpoint)
}

func createNetworkMonitorClient(t *testing.T) *networkmonitorsvc.Client {
	t.Helper()

	return createClientWithEndpoint(t, networkmonitorsvc.NewFromConfig, endpoint)
}

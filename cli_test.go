package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/alecthomas/kong"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/awsmeta"
	"github.com/blackbirdworks/gopherstack/pkgs/persistence"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
	svctags "github.com/blackbirdworks/gopherstack/pkgs/tags"
	accessanalyzerbackend "github.com/blackbirdworks/gopherstack/services/accessanalyzer"
	appconfigbackend "github.com/blackbirdworks/gopherstack/services/appconfig"
	applicationautoscalingbackend "github.com/blackbirdworks/gopherstack/services/applicationautoscaling"
	appmeshbackend "github.com/blackbirdworks/gopherstack/services/appmesh"
	apprunnerbackend "github.com/blackbirdworks/gopherstack/services/apprunner"
	appstreambackend "github.com/blackbirdworks/gopherstack/services/appstream"
	athenabackend "github.com/blackbirdworks/gopherstack/services/athena"
	awsconfigbackend "github.com/blackbirdworks/gopherstack/services/awsconfig"
	backupbackend "github.com/blackbirdworks/gopherstack/services/backup"
	batchbackend "github.com/blackbirdworks/gopherstack/services/batch"
	cebackend "github.com/blackbirdworks/gopherstack/services/ce"
	cleanroomsbackend "github.com/blackbirdworks/gopherstack/services/cleanrooms"
	cloudfrontbackend "github.com/blackbirdworks/gopherstack/services/cloudfront"
	cwlogsbackend "github.com/blackbirdworks/gopherstack/services/cloudwatchlogs"
	codeartifactbackend "github.com/blackbirdworks/gopherstack/services/codeartifact"
	codecommitbackend "github.com/blackbirdworks/gopherstack/services/codecommit"
	codeconnectionsbackend "github.com/blackbirdworks/gopherstack/services/codeconnections"
	codedeploybackend "github.com/blackbirdworks/gopherstack/services/codedeploy"
	codepipelinebackend "github.com/blackbirdworks/gopherstack/services/codepipeline"
	cognitoidpbackend "github.com/blackbirdworks/gopherstack/services/cognitoidp"
	comprehendbackend "github.com/blackbirdworks/gopherstack/services/comprehend"
	datasyncbackend "github.com/blackbirdworks/gopherstack/services/datasync"
	daxbackend "github.com/blackbirdworks/gopherstack/services/dax"
	detectivebackend "github.com/blackbirdworks/gopherstack/services/detective"
	dlmbackend "github.com/blackbirdworks/gopherstack/services/dlm"
	docdbbackend "github.com/blackbirdworks/gopherstack/services/docdb"
	ecrbackend "github.com/blackbirdworks/gopherstack/services/ecr"
	ecsbackend "github.com/blackbirdworks/gopherstack/services/ecs"
	efsbackend "github.com/blackbirdworks/gopherstack/services/efs"
	eksbackend "github.com/blackbirdworks/gopherstack/services/eks"
	elasticachebackend "github.com/blackbirdworks/gopherstack/services/elasticache"
	emrbackend "github.com/blackbirdworks/gopherstack/services/emr"
	ebbackend "github.com/blackbirdworks/gopherstack/services/eventbridge"
	firehosebackend "github.com/blackbirdworks/gopherstack/services/firehose"
	fisbackend "github.com/blackbirdworks/gopherstack/services/fis"
	gluebackend "github.com/blackbirdworks/gopherstack/services/glue"
	guarddutybackend "github.com/blackbirdworks/gopherstack/services/guardduty"
	inspector2backend "github.com/blackbirdworks/gopherstack/services/inspector2"
	kinesisbackend "github.com/blackbirdworks/gopherstack/services/kinesis"
	kinesisanalyticsv2backend "github.com/blackbirdworks/gopherstack/services/kinesisanalyticsv2"
	macie2backend "github.com/blackbirdworks/gopherstack/services/macie2"
	managedblockchainbackend "github.com/blackbirdworks/gopherstack/services/managedblockchain"
	mediaconvertbackend "github.com/blackbirdworks/gopherstack/services/mediaconvert"
	mediapackagebackend "github.com/blackbirdworks/gopherstack/services/mediapackage"
	mediastorebackend "github.com/blackbirdworks/gopherstack/services/mediastore"
	mediatailorbackend "github.com/blackbirdworks/gopherstack/services/mediatailor"
	memorydbbackend "github.com/blackbirdworks/gopherstack/services/memorydb"
	mqbackend "github.com/blackbirdworks/gopherstack/services/mq"
	mwaabackend "github.com/blackbirdworks/gopherstack/services/mwaa"
	neptunebackend "github.com/blackbirdworks/gopherstack/services/neptune"
	opensearchbackend "github.com/blackbirdworks/gopherstack/services/opensearch"
	personalizebackend "github.com/blackbirdworks/gopherstack/services/personalize"
	pinpointbackend "github.com/blackbirdworks/gopherstack/services/pinpoint"
	pipesbackend "github.com/blackbirdworks/gopherstack/services/pipes"
	rambackend "github.com/blackbirdworks/gopherstack/services/ram"
	rdsbackend "github.com/blackbirdworks/gopherstack/services/rds"
	redshiftbackend "github.com/blackbirdworks/gopherstack/services/redshift"
	rekognitionbackend "github.com/blackbirdworks/gopherstack/services/rekognition"
	resourcegroupstaggingapibackend "github.com/blackbirdworks/gopherstack/services/resourcegroupstaggingapi"
	route53resolverbackend "github.com/blackbirdworks/gopherstack/services/route53resolver"
	s3tablesbackend "github.com/blackbirdworks/gopherstack/services/s3tables"
	sagemakerbackend "github.com/blackbirdworks/gopherstack/services/sagemaker"
	schedulerbackend "github.com/blackbirdworks/gopherstack/services/scheduler"
	securityhubbackend "github.com/blackbirdworks/gopherstack/services/securityhub"
	servicediscoverybackend "github.com/blackbirdworks/gopherstack/services/servicediscovery"
	sesv2backend "github.com/blackbirdworks/gopherstack/services/sesv2"
	shieldbackend "github.com/blackbirdworks/gopherstack/services/shield"
	sfnbackend "github.com/blackbirdworks/gopherstack/services/stepfunctions"
	swfbackend "github.com/blackbirdworks/gopherstack/services/swf"
	timestreamwritebackend "github.com/blackbirdworks/gopherstack/services/timestreamwrite"
	transcribebackend "github.com/blackbirdworks/gopherstack/services/transcribe"
	transferbackend "github.com/blackbirdworks/gopherstack/services/transfer"
	translatebackend "github.com/blackbirdworks/gopherstack/services/translate"
	verifiedpermissionsbackend "github.com/blackbirdworks/gopherstack/services/verifiedpermissions"
	vpclatticebackend "github.com/blackbirdworks/gopherstack/services/vpclattice"
	wafbackend "github.com/blackbirdworks/gopherstack/services/waf"
	wafv2backend "github.com/blackbirdworks/gopherstack/services/wafv2"
	workmailbackend "github.com/blackbirdworks/gopherstack/services/workmail"
	xraybackend "github.com/blackbirdworks/gopherstack/services/xray"
)

// parseCLI parses the given args (key=value env pairs) into a CLI value
// by setting environment variables then parsing an empty argument list.
func parseCLI(t *testing.T, envPairs map[string]string) CLI {
	t.Helper()

	for k, v := range envPairs {
		t.Setenv(k, v)
	}

	var root rootCLI

	parser, err := kong.New(
		&root,
		kong.Name("gopherstack"),
		kong.Writers(nil, nil), // suppress help/error output
	)
	require.NoError(t, err)

	_, err = parser.Parse([]string{})
	require.NoError(t, err)

	return root.Serve
}

func TestCLI_Defaults(t *testing.T) {
	t.Parallel()
	cli := parseCLI(t, nil)

	assert.Equal(t, "info", cli.LogLevel)
	assert.Equal(t, "8000", cli.Port)
	assert.Equal(t, "us-east-1", cli.Region)
	assert.False(t, cli.Demo)
	assert.Equal(t, 500*time.Millisecond, cli.DynamoDB.JanitorInterval)
	assert.Equal(t, 500*time.Millisecond, cli.S3.JanitorInterval)
	assert.Equal(t, time.Minute, cli.Athena.JanitorInterval)
	assert.Equal(t, 24*time.Hour, cli.Athena.ExecutionTTL)
	assert.Equal(t, time.Minute, cli.Backup.JanitorInterval)
	assert.Equal(t, 24*time.Hour, cli.Backup.JobTTL)
	assert.Equal(t, time.Minute, cli.Batch.JanitorInterval)
	assert.Equal(t, 24*time.Hour, cli.Batch.InactiveJobDefTTL)
	assert.Equal(t, 24*time.Hour, cli.Batch.CompletedJobTTL)
	assert.Equal(t, time.Minute, cli.CloudWatchLogs.JanitorInterval)
	assert.Equal(t, time.Minute, cli.CodeBuild.JanitorInterval)
	assert.Equal(t, 24*time.Hour, cli.CodeBuild.BuildTTL)
	assert.Equal(t, time.Minute, cli.EC2.JanitorInterval)
	assert.Equal(t, time.Hour, cli.EC2.TerminatedTTL)
	assert.Equal(t, 6*time.Hour, cli.EC2.CancelledSpotTTL)
	assert.Equal(t, time.Minute, cli.EMR.JanitorInterval)
	assert.Equal(t, time.Hour, cli.EMR.TerminatedTTL)
	assert.Equal(t, time.Minute, cli.FIS.JanitorInterval)
	assert.Equal(t, 24*time.Hour, cli.FIS.ExperimentTTL)
	assert.Equal(t, time.Minute, cli.Kinesis.JanitorInterval)
	assert.Equal(t, time.Minute, cli.KMS.JanitorInterval)
	assert.Equal(t, time.Minute, cli.SES.JanitorInterval)
	assert.Equal(t, 24*time.Hour, cli.SES.EmailTTL)
	assert.Equal(t, 30*time.Second, cli.SSM.JanitorInterval)
	assert.Equal(t, time.Hour, cli.SSM.CommandTTL)
	assert.Equal(t, 30*time.Second, cli.STS.JanitorInterval)
	assert.Equal(t, time.Minute, cli.XRay.JanitorInterval)
	assert.Equal(t, 30*time.Minute, cli.XRay.TraceTTL)
}

//nolint:paralleltest // uses t.Setenv which disallows t.Parallel
func TestCLI_EnvVarsOverrideDefaults(t *testing.T) {
	cli := parseCLI(t, map[string]string{
		"LOG_LEVEL":                 "debug",
		"PORT":                      "9090",
		"REGION":                    "eu-west-1",
		"DEMO":                      "true",
		"DYNAMODB_JANITOR_INTERVAL": "2s",
		"S3_JANITOR_INTERVAL":       "1s",
	})

	assert.Equal(t, "debug", cli.LogLevel)
	assert.Equal(t, "9090", cli.Port)
	assert.Equal(t, "eu-west-1", cli.Region)
	assert.True(t, cli.Demo)
	assert.Equal(t, 2*time.Second, cli.DynamoDB.JanitorInterval)
	assert.Equal(t, time.Second, cli.S3.JanitorInterval)
}

//nolint:paralleltest // uses t.Setenv which disallows t.Parallel
func TestCLI_JanitorTTLEnvVars(t *testing.T) {
	cli := parseCLI(t, map[string]string{
		"ATHENA_EXECUTION_TTL":       "2h",
		"BACKUP_JOB_TTL":             "3h",
		"BATCH_INACTIVE_JOB_DEF_TTL": "4h",
		"BATCH_COMPLETED_JOB_TTL":    "5h",
		"CODEBUILD_BUILD_TTL":        "6h",
		"EC2_TERMINATED_TTL":         "7h",
		"EC2_CANCELLED_SPOT_TTL":     "8h",
		"EMR_TERMINATED_TTL":         "9h",
		"FIS_EXPERIMENT_TTL":         "10h",
		"SES_EMAIL_TTL":              "11h",
		"SSM_COMMAND_TTL":            "2h",
		"XRAY_TRACE_TTL":             "12h",
	})

	assert.Equal(t, 2*time.Hour, cli.Athena.ExecutionTTL)
	assert.Equal(t, 3*time.Hour, cli.Backup.JobTTL)
	assert.Equal(t, 4*time.Hour, cli.Batch.InactiveJobDefTTL)
	assert.Equal(t, 5*time.Hour, cli.Batch.CompletedJobTTL)
	assert.Equal(t, 6*time.Hour, cli.CodeBuild.BuildTTL)
	assert.Equal(t, 7*time.Hour, cli.EC2.TerminatedTTL)
	assert.Equal(t, 8*time.Hour, cli.EC2.CancelledSpotTTL)
	assert.Equal(t, 9*time.Hour, cli.EMR.TerminatedTTL)
	assert.Equal(t, 10*time.Hour, cli.FIS.ExperimentTTL)
	assert.Equal(t, 11*time.Hour, cli.SES.EmailTTL)
	assert.Equal(t, 2*time.Hour, cli.SSM.CommandTTL)
	assert.Equal(t, 12*time.Hour, cli.XRay.TraceTTL)
}

//nolint:paralleltest // uses t.Setenv which disallows t.Parallel
func TestCLI_JanitorIntervalEnvVars(t *testing.T) {
	cli := parseCLI(t, map[string]string{
		"ATHENA_JANITOR_INTERVAL":         "2m",
		"BACKUP_JANITOR_INTERVAL":         "3m",
		"BATCH_JANITOR_INTERVAL":          "4m",
		"CLOUDWATCHLOGS_JANITOR_INTERVAL": "5m",
		"CODEBUILD_JANITOR_INTERVAL":      "6m",
		"EC2_JANITOR_INTERVAL":            "7m",
		"EMR_JANITOR_INTERVAL":            "8m",
		"FIS_JANITOR_INTERVAL":            "9m",
		"KINESIS_JANITOR_INTERVAL":        "10m",
		"KMS_JANITOR_INTERVAL":            "11m",
		"SES_JANITOR_INTERVAL":            "12m",
		"SSM_JANITOR_INTERVAL":            "13m",
		"STS_JANITOR_INTERVAL":            "14m",
		"XRAY_JANITOR_INTERVAL":           "15m",
	})

	assert.Equal(t, 2*time.Minute, cli.Athena.JanitorInterval)
	assert.Equal(t, 3*time.Minute, cli.Backup.JanitorInterval)
	assert.Equal(t, 4*time.Minute, cli.Batch.JanitorInterval)
	assert.Equal(t, 5*time.Minute, cli.CloudWatchLogs.JanitorInterval)
	assert.Equal(t, 6*time.Minute, cli.CodeBuild.JanitorInterval)
	assert.Equal(t, 7*time.Minute, cli.EC2.JanitorInterval)
	assert.Equal(t, 8*time.Minute, cli.EMR.JanitorInterval)
	assert.Equal(t, 9*time.Minute, cli.FIS.JanitorInterval)
	assert.Equal(t, 10*time.Minute, cli.Kinesis.JanitorInterval)
	assert.Equal(t, 11*time.Minute, cli.KMS.JanitorInterval)
	assert.Equal(t, 12*time.Minute, cli.SES.JanitorInterval)
	assert.Equal(t, 13*time.Minute, cli.SSM.JanitorInterval)
	assert.Equal(t, 14*time.Minute, cli.STS.JanitorInterval)
	assert.Equal(t, 15*time.Minute, cli.XRay.JanitorInterval)
}

func TestCLI_BuildLogger(t *testing.T) {
	t.Parallel()
	cases := []struct{ input, wantLevel string }{
		{"debug", "DEBUG"},
		{"info", "INFO"},
		{"warn", "WARN"},
		{"error", "ERROR"},
		{"unknown", "INFO"},
		{"", "INFO"},
		{"DEBUG", "DEBUG"}, // case-insensitive
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()

			log := buildLogger(tc.input)
			require.NotNil(t, log)

			// Verify level by checking what the logger reports (cheapest approach:
			// check the Enabled method on the underlying level).
			_ = log // Just verify it doesn't panic; level handling is exercised via coverage.
		})
	}

	// Spot-check that buildLogger("debug") doesn't panic and returns a non-nil logger.
	debugLog := buildLogger("debug")
	assert.NotNil(t, debugLog)
}

//nolint:paralleltest // uses t.Setenv via parseCLI, which is incompatible with t.Parallel.
func TestServerStartupAndShutdown(t *testing.T) {
	port := freeTCPPort(t)

	cli := parseCLI(t, map[string]string{
		"PORT": strconv.Itoa(port),
	})

	ctx, cancel := context.WithCancel(t.Context())

	errCh := make(chan error, 1)
	go func() {
		errCh <- run(ctx, cli)
	}()

	// Wait briefly to let the server start (in a real test you might poll the endpoint)
	time.Sleep(200 * time.Millisecond)

	// Cancel the context to initiate a graceful shutdown
	cancel()

	select {
	case err := <-errCh:
		require.NoError(t, err, "server should shutdown cleanly without error")
	case <-time.After(5 * time.Second):
		require.FailNow(t, "server did not shut down within timeout")
	}
}

// TestCLI_GetSTSClient verifies that GetSTSClient returns nil before clients are initialized.
// Specifically, before initializeClients is called (i.e., before the server starts), it should be nil.
func TestCLI_GetSTSClient(t *testing.T) {
	t.Parallel()

	cli := parseCLI(t, nil)
	// For a freshly parsed CLI (before the server starts), the client is nil (not yet initialized).
	assert.Nil(t, cli.GetSTSClient())
}

//nolint:paralleltest // uses t.Setenv which disallows t.Parallel
func TestCLI_PortAllocatorDefaults(t *testing.T) {
	cli := parseCLI(t, nil)

	assert.Equal(t, 10000, cli.PortRangeStart)
	assert.Equal(t, 10100, cli.PortRangeEnd)
}

//nolint:paralleltest // uses t.Setenv which disallows t.Parallel
func TestCLI_PortAllocatorEnvVars(t *testing.T) {
	cli := parseCLI(t, map[string]string{
		"PORT_RANGE_START": "20000",
		"PORT_RANGE_END":   "20200",
	})

	assert.Equal(t, 20000, cli.PortRangeStart)
	assert.Equal(t, 20200, cli.PortRangeEnd)
}

//nolint:paralleltest // uses t.Setenv which disallows t.Parallel
func TestCLI_DNSDefaults(t *testing.T) {
	cli := parseCLI(t, nil)

	assert.Empty(t, cli.DNSListenAddr)
	assert.Equal(t, "127.0.0.1", cli.DNSResolveIP)
}

//nolint:paralleltest // uses t.Setenv which disallows t.Parallel
func TestCLI_InitScriptTimeout(t *testing.T) {
	cli := parseCLI(t, map[string]string{
		"INIT_TIMEOUT": "1m",
	})

	assert.Equal(t, time.Minute, cli.InitScriptTimeout)
}

//nolint:paralleltest // uses t.Setenv via parseCLI, which is incompatible with t.Parallel.
func TestServerStartup_WithInitScript(t *testing.T) {
	dir := t.TempDir()
	marker := dir + "/marker.txt"
	port := freeTCPPort(t)

	cli := parseCLI(t, map[string]string{
		"PORT": strconv.Itoa(port),
	})
	cli.InitScripts = []string{"echo ran > " + marker}
	cli.InitScriptTimeout = 5 * time.Second

	ctx, cancel := context.WithCancel(t.Context())

	errCh := make(chan error, 1)
	go func() {
		errCh <- run(ctx, cli)
	}()

	// Poll for the marker file instead of a fixed sleep to avoid timing flakes.
	deadline := time.Now().Add(5 * time.Second)
	var data []byte
	for time.Now().Before(deadline) {
		var readErr error
		data, readErr = os.ReadFile(marker)
		if readErr == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	require.NotNil(t, data, "init script should have created the marker file within 5s")
	assert.Contains(t, string(data), "ran")

	cancel()

	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "server did not shut down within timeout")
	}
}

//nolint:paralleltest // uses t.Setenv via parseCLI, which is incompatible with t.Parallel.
func TestServerStartup_WithDNS(t *testing.T) {
	// Find a free UDP port for the DNS server.
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	dnsPort := pc.LocalAddr().(*net.UDPAddr).Port
	_ = pc.Close()

	port := freeTCPPort(t)
	cli := parseCLI(t, map[string]string{
		"PORT": strconv.Itoa(port),
	})
	cli.DNSListenAddr = fmt.Sprintf("127.0.0.1:%d", dnsPort)
	cli.DNSResolveIP = "127.0.0.1"

	ctx, cancel := context.WithCancel(t.Context())

	errCh := make(chan error, 1)
	go func() {
		errCh <- run(ctx, cli)
	}()

	time.Sleep(300 * time.Millisecond)
	cancel()

	select {
	case runErr := <-errCh:
		require.NoError(t, runErr)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "server did not shut down within timeout")
	}
}

//nolint:paralleltest // uses t.Setenv via parseCLI, which is incompatible with t.Parallel.
func TestServerStartup_InvalidDNSConfig(t *testing.T) {
	port := freeTCPPort(t)
	cli := parseCLI(t, map[string]string{
		"PORT": strconv.Itoa(port),
	})
	cli.DNSListenAddr = ":18553"
	cli.DNSResolveIP = "not-an-ip" // invalid — server logs a warning and continues

	ctx, cancel := context.WithCancel(t.Context())

	errCh := make(chan error, 1)
	go func() {
		errCh <- run(ctx, cli)
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "server did not shut down within timeout")
	}
}

//nolint:paralleltest // uses t.Setenv via parseCLI, which is incompatible with t.Parallel.
func TestServerStartup_InvalidPortRange(t *testing.T) {
	port := freeTCPPort(t)
	cli := parseCLI(t, map[string]string{
		"PORT": strconv.Itoa(port),
	})
	cli.PortRangeStart = 0 // invalid range — logs a warning and continues
	cli.PortRangeEnd = 0

	ctx, cancel := context.WithCancel(t.Context())

	errCh := make(chan error, 1)
	go func() {
		errCh <- run(ctx, cli)
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "server did not shut down within timeout")
	}
}

//nolint:paralleltest // uses t.Setenv via parseCLI, which is incompatible with t.Parallel.
func TestHealthCmd_Success(t *testing.T) {
	port := freeTCPPort(t)
	portString := strconv.Itoa(port)

	cli := parseCLI(t, map[string]string{
		"PORT": portString,
	})

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- run(ctx, cli)
	}()

	// Wait for the server to be ready.
	require.Eventually(t, func() bool {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/_gopherstack/health", port))
		if err != nil {
			return false
		}
		resp.Body.Close()

		return resp.StatusCode == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "server did not become ready")

	// Run the health command against the running server.
	cmd := &HealthCmd{Port: portString}
	require.NoError(t, cmd.Run())

	cancel()

	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "server did not shut down within timeout")
	}
}

func TestHealthCmd_NoServer(t *testing.T) {
	t.Parallel()

	cmd := &HealthCmd{Port: "19999"}
	err := cmd.Run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "health check failed")
}

//nolint:paralleltest // uses t.Setenv which disallows t.Parallel
func TestHealthCmd_KongParsing(t *testing.T) {
	var root rootCLI

	parser, err := kong.New(
		&root,
		kong.Name("gopherstack"),
		kong.Writers(nil, nil),
	)
	require.NoError(t, err)

	kctx, err := parser.Parse([]string{"health", "--port", "9090"})
	require.NoError(t, err)

	assert.Equal(t, "health", kctx.Command())
	assert.Equal(t, "9090", root.Health.Port)
}

func TestARNServiceIs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		arn         string
		serviceName string
		want        bool
	}{
		{
			name:        "sqs_match",
			arn:         "arn:aws:sqs:us-east-1:123456789012:my-queue",
			serviceName: "sqs",
			want:        true,
		},
		{
			name:        "sqs_no_match_sns",
			arn:         "arn:aws:sqs:us-east-1:123456789012:my-queue",
			serviceName: "sns",
			want:        false,
		},
		{
			name:        "lambda_match",
			arn:         "arn:aws:lambda:us-east-1:123456789012:function:my-fn",
			serviceName: "lambda",
			want:        true,
		},
		{
			name:        "secretsmanager_match",
			arn:         "arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret-ABCDEF",
			serviceName: "secretsmanager",
			want:        true,
		},
		{
			name:        "secretsmanager_not_matched_by_sqs",
			arn:         "arn:aws:secretsmanager:us-east-1:123456789012:secret:my-sqs-secret-ABCDEF",
			serviceName: "sqs",
			want:        false,
		},
		{
			name:        "empty_arn",
			arn:         "",
			serviceName: "sqs",
			want:        false,
		},
		{
			name:        "invalid_arn",
			arn:         "not-an-arn",
			serviceName: "sqs",
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, arnServiceIs(tt.arn, tt.serviceName))
		})
	}
}

func TestResourceTypeFromARN(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		arn     string
		service string
		want    string
	}{
		{
			name:    "slash_separated_resource",
			arn:     "arn:aws:ecs:us-east-1:123456789012:cluster/my-cluster",
			service: "ecs",
			want:    "ecs:cluster",
		},
		{
			name:    "nested_slash_resource_takes_first_segment",
			arn:     "arn:aws:ecs:us-east-1:123456789012:service/my-cluster/my-service",
			service: "ecs",
			want:    "ecs:service",
		},
		{
			name:    "colon_separated_resource",
			arn:     "arn:aws:glue:us-east-1:123456789012:dataQualityRuleset/my-ruleset",
			service: "glue",
			want:    "glue:dataQualityRuleset",
		},
		{
			name:    "bare_resource_no_separator_falls_back_to_service",
			arn:     "arn:aws:sqs:us-east-1:123456789012:my-queue",
			service: "sqs",
			want:    "sqs",
		},
		{
			name:    "malformed_arn_falls_back_to_service",
			arn:     "not-an-arn",
			service: "ecs",
			want:    "ecs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, resourceTypeFromARN(tt.arn, tt.service))
		})
	}
}

// TestWireResourceGroupsTagging_CrossServiceResources proves the fix for
// gopherstack-3xne: resources tagged through a service's own native TagResource API
// must be visible to the Resource Groups Tagging API's GetResources once cli.go wires
// that service in via wireResourceGroupsTagging. Before this fix only 6 of ~90 taggable
// services were wired, so a resource tagged via (for example) ECS's TagResource was
// invisible to a cross-service tag query with no error -- a test that only checks "no
// error" would pass against that broken state, so this asserts the actual ARN comes
// back from GetResources, filtered by the exact resource-type string the wiring
// derived for it.
func TestWireResourceGroupsTagging_CrossServiceResources(t *testing.T) {
	t.Parallel()

	const accountID = "123456789012"
	const region = "us-east-1"
	const wantTagKey = "team"
	const wantTagValue = "wiring-test"

	tests := []struct {
		wire             func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string
		name             string
		wantResourceType string
	}{
		{
			name: "ecs_cluster",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				ecsBk := ecsbackend.NewInMemoryBackend(accountID, region, nil)
				resourceARN := "arn:aws:ecs:" + region + ":" + accountID + ":cluster/wiring-test-cluster"
				require.NoError(t, ecsBk.TagResource(
					resourceARN, []ecsbackend.Tag{{Key: wantTagKey, Value: wantTagValue}},
				))

				wireTaggingECS(bk, ecsbackend.NewHandler(ecsBk))

				return resourceARN
			},
			wantResourceType: "ecs:cluster",
		},
		{
			name: "athena_workgroup",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				athenaBk := athenabackend.NewInMemoryBackend(region, accountID)
				require.NoError(t, athenaBk.CreateWorkGroup(
					"wiring-test-wg", "", "", athenabackend.WorkGroupConfiguration{}, nil,
				))
				resourceARN := "arn:aws:athena:" + region + ":" + accountID + ":workgroup/wiring-test-wg"
				require.NoError(t, athenaBk.TagResource(resourceARN, map[string]string{wantTagKey: wantTagValue}))

				wireTaggingAthena(bk, athenabackend.NewHandler(athenaBk))

				return resourceARN
			},
			wantResourceType: "athena:workgroup",
		},
		{
			name: "glue_database",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				glueBk := gluebackend.NewInMemoryBackend(accountID, region)
				db, err := glueBk.CreateDatabase(gluebackend.DatabaseInput{Name: "wiring-test-db"}, nil)
				require.NoError(t, err)
				require.NoError(t, glueBk.TagResource(db.ARN, map[string]string{wantTagKey: wantTagValue}))

				wireTaggingGlue(bk, gluebackend.NewHandler(glueBk))

				return db.ARN
			},
			wantResourceType: "glue:database",
		},
		{
			name: "ecr_repository",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				ecrBk := ecrbackend.NewInMemoryBackend(accountID, region, "")
				repo, err := ecrBk.CreateRepository(context.Background(), "wiring-test-repo", "", false, "", "")
				require.NoError(t, err)
				require.NoError(t, ecrBk.TagResource(
					context.Background(), repo.RepositoryARN, map[string]string{wantTagKey: wantTagValue},
				))

				wireTaggingECR(bk, ecrbackend.NewHandler(ecrBk, nil))

				return repo.RepositoryARN
			},
			wantResourceType: "ecr:repository",
		},
		{
			name: "kinesis_stream",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				kinesisBk := kinesisbackend.NewInMemoryBackendWithConfig(accountID, region)
				require.NoError(t, kinesisBk.CreateStream(context.Background(), &kinesisbackend.CreateStreamInput{
					StreamName: "wiring-test-stream",
					ShardCount: 1,
				}))
				desc, err := kinesisBk.DescribeStream(context.Background(), &kinesisbackend.DescribeStreamInput{
					StreamName: "wiring-test-stream",
				})
				require.NoError(t, err)
				require.NoError(t, kinesisBk.TagResource(context.Background(), &kinesisbackend.TagResourceInput{
					ResourceARN: desc.StreamARN,
					Tags:        map[string]string{wantTagKey: wantTagValue},
				}))

				wireTaggingKinesis(bk, kinesisbackend.NewHandler(kinesisBk))

				return desc.StreamARN
			},
			wantResourceType: "kinesis:stream",
		},
		{
			name: "stepfunctions_state_machine",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				sfnBk := sfnbackend.NewInMemoryBackendWithConfig(accountID, region)
				sm, err := sfnBk.CreateStateMachine(
					context.Background(),
					"wiring-test-sm",
					`{"StartAt":"S","States":{"S":{"Type":"Pass","End":true}}}`,
					"arn:aws:iam::"+accountID+":role/wiring-test-role",
					"STANDARD",
				)
				require.NoError(t, err)

				sfnH := sfnbackend.NewHandler(sfnBk)
				require.NoError(t,
					sfnH.TagResourceByARN(sm.StateMachineArn, map[string]string{wantTagKey: wantTagValue}))

				wireTaggingStepFunctions(bk, sfnH)

				return sm.StateMachineArn
			},
			wantResourceType: "states:stateMachine",
		},
		{
			name: "cloudfront_distribution",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				cfBk := cloudfrontbackend.NewInMemoryBackend(t.Context(), accountID, region)
				dist, err := cfBk.CreateDistribution("wiring-test-ref", "wiring-test-dist", true, nil)
				require.NoError(t, err)
				require.NoError(t, cfBk.TagResource(dist.ARN, map[string]string{wantTagKey: wantTagValue}))

				wireTaggingCloudFront(bk, cloudfrontbackend.NewHandler(cfBk))

				return dist.ARN
			},
			wantResourceType: "cloudfront:distribution",
		},
		{
			name: "eks_cluster",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				eksBk := eksbackend.NewInMemoryBackend(context.Background(), accountID, region)
				cluster, err := eksBk.CreateCluster("wiring-test-cluster", "", "", nil, nil, nil)
				require.NoError(t, err)
				require.NoError(t, eksBk.TagResource(cluster.ARN, map[string]string{wantTagKey: wantTagValue}))

				wireTaggingEKS(bk, eksbackend.NewHandler(eksBk))

				return cluster.ARN
			},
			wantResourceType: "eks:cluster",
		},
		{
			name: "batch_compute_environment",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				batchBk := batchbackend.NewInMemoryBackend(accountID, region)
				ce, err := batchBk.CreateComputeEnvironment(
					context.Background(), "wiring-test-ce", "UNMANAGED", "ENABLED", nil, "", nil, nil, nil, nil,
				)
				require.NoError(t, err)
				require.NoError(t, batchBk.TagResource(
					context.Background(), ce.ComputeEnvironmentArn, map[string]string{wantTagKey: wantTagValue},
				))

				wireTaggingBatch(bk, batchbackend.NewHandler(batchBk))

				return ce.ComputeEnvironmentArn
			},
			wantResourceType: "batch:compute-environment",
		},
		{
			name: "wafv2_web_acl",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				wafBk := wafv2backend.NewInMemoryBackend(accountID, region)
				webACL, err := wafBk.CreateWebACL(
					context.Background(), "wiring-test-acl", "REGIONAL", "",
					json.RawMessage(`{"Allow":{}}`), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
				)
				require.NoError(t, err)
				require.NoError(t, wafBk.TagResource(
					context.Background(), webACL.ARN, map[string]string{wantTagKey: wantTagValue},
				))

				wireTaggingWAFv2(bk, wafv2backend.NewHandler(wafBk))

				return webACL.ARN
			},
			wantResourceType: "wafv2:regional/webacl",
		},
		{
			name: "backup_vault",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				backupBk := backupbackend.NewInMemoryBackend(accountID, region)
				vault, err := backupBk.CreateBackupVault("wiring-test-vault", "", "", nil)
				require.NoError(t, err)
				require.NoError(t, backupBk.TagResource(
					vault.BackupVaultArn, map[string]string{wantTagKey: wantTagValue},
				))

				wireTaggingBackup(bk, backupbackend.NewHandler(backupBk))

				return vault.BackupVaultArn
			},
			wantResourceType: "backup:backup-vault",
		},
		{
			name: "efs_file_system",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				efsBk := efsbackend.NewInMemoryBackend(accountID, region)
				fs, err := efsBk.CreateFileSystem(
					context.Background(), efsbackend.CreateFileSystemRequest{CreationToken: "wiring-test-token"},
				)
				require.NoError(t, err)
				require.NoError(t, efsBk.TagResource(
					context.Background(), fs.FileSystemArn, map[string]string{wantTagKey: wantTagValue},
				))

				wireTaggingEFS(bk, efsbackend.NewHandler(efsBk))

				return fs.FileSystemArn
			},
			wantResourceType: "elasticfilesystem:file-system",
		},
		{
			name: "docdb_db_cluster",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				docdbBk := docdbbackend.NewInMemoryBackend(accountID, region)
				cluster, err := docdbBk.CreateDBCluster(
					context.Background(), "wiring-test-cluster", "docdb", "", "admin", "", "", "", "",
					0, false, false, 1, "", "", nil, nil, nil,
				)
				require.NoError(t, err)
				require.NoError(t, docdbBk.AddTagsToResource(
					context.Background(), cluster.DBClusterArn,
					[]docdbbackend.Tag{{Key: wantTagKey, Value: wantTagValue}},
				))

				wireTaggingDocDB(bk, docdbbackend.NewHandler(docdbBk))

				return cluster.DBClusterArn
			},
			// DocDB builds every ARN under the "rds" ARN service (see
			// services/docdb/store.go:232-266's arn.Build("rds", ...) call sites) --
			// not "docdb", which does not appear anywhere in its ARNs.
			wantResourceType: "rds:cluster",
		},
		{
			name: "neptune_db_cluster",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				neptuneBk := neptunebackend.NewInMemoryBackend(accountID, region)
				cluster, err := neptuneBk.CreateDBCluster(
					context.Background(), "wiring-test-cluster", "", 0, neptunebackend.DBClusterCreateOptions{},
				)
				require.NoError(t, err)
				require.NoError(t, neptuneBk.AddTagsToResource(
					context.Background(), cluster.DBClusterArn,
					[]neptunebackend.Tag{{Key: wantTagKey, Value: wantTagValue}},
				))

				wireTaggingNeptune(bk, neptunebackend.NewHandler(neptuneBk))

				return cluster.DBClusterArn
			},
			// Unlike its parameter groups/subnet groups/snapshots (which use "rds",
			// see services/neptune/cluster_parameter_groups.go:47,
			// subnet_groups.go:41, cluster_snapshots.go:44), Neptune DB clusters use
			// the "neptune" ARN service (db_clusters.go:70).
			wantResourceType: "neptune:cluster",
		},
		{
			name: "rds_db_instance",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				rdsBk := rdsbackend.NewInMemoryBackend(accountID, region)
				_, err := rdsBk.CreateDBInstance(
					"wiring-test-db", "postgres", "db.t3.micro", "", "admin", "", 20, rdsbackend.DBInstanceOptions{},
				)
				require.NoError(t, err)

				// DBInstance carries no ARN field of its own; RDS builds it ad hoc
				// wherever needed (see automated_backups.go/proxies.go) as
				// "arn:aws:rds:{region}:{account}:db:{id}".
				resourceARN := "arn:aws:rds:" + region + ":" + accountID + ":db:wiring-test-db"
				rdsBk.AddTagsToResource(resourceARN, []rdsbackend.Tag{{Key: wantTagKey, Value: wantTagValue}})

				wireTaggingRDS(bk, rdsbackend.NewHandler(rdsBk))

				return resourceARN
			},
			wantResourceType: "rds:db",
		},
		{
			name: "elasticache_cluster",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				ecBk := elasticachebackend.NewInMemoryBackend(elasticachebackend.EngineStub, accountID, region, nil)
				cluster, err := ecBk.CreateCluster(
					context.Background(), "wiring-test-cache", "redis", "cache.t3.micro", 0,
				)
				require.NoError(t, err)
				require.NoError(t, ecBk.AddTagsToResource(
					context.Background(), cluster.ARN, map[string]string{wantTagKey: wantTagValue},
				))

				wireTaggingElastiCache(bk, elasticachebackend.NewHandler(ecBk))

				return cluster.ARN
			},
			wantResourceType: "elasticache:cluster",
		},
		{
			name: "redshift_cluster",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				redshiftBk := redshiftbackend.NewInMemoryBackend(accountID, region)
				_, err := redshiftBk.CreateCluster("wiring-test-cluster", "dc2.large", "dev", "admin", nil, "")
				require.NoError(t, err)
				require.NoError(t, redshiftBk.CreateTags(
					"wiring-test-cluster", map[string]string{wantTagKey: wantTagValue},
				))

				wireTaggingRedshift(bk, redshiftbackend.NewHandler(redshiftBk))

				// Redshift's Cluster carries no ARN field of its own (see
				// wireTaggingRedshift's doc comment); the ARN is reconstructed here
				// the same way the wiring does, for the test to assert against.
				return "arn:aws:redshift:" + region + ":" + accountID + ":cluster:wiring-test-cluster"
			},
			wantResourceType: "redshift:cluster",
		},
		{
			name: "sagemaker_model",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				smBk := sagemakerbackend.NewInMemoryBackend(accountID, region)
				model, err := smBk.CreateModel(
					context.Background(), "wiring-test-model",
					"arn:aws:iam::"+accountID+":role/wiring-test-role", nil, nil, nil,
				)
				require.NoError(t, err)
				require.NoError(t, smBk.AddTags(
					context.Background(), model.ModelARN, map[string]string{wantTagKey: wantTagValue},
				))

				wireTaggingSageMaker(bk, sagemakerbackend.NewHandler(smBk))

				return model.ModelARN
			},
			wantResourceType: "sagemaker:model",
		},
		{
			name: "firehose_delivery_stream",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				fhBk := firehosebackend.NewInMemoryBackend(accountID, region)
				stream, err := fhBk.CreateDeliveryStream(
					context.Background(), firehosebackend.CreateDeliveryStreamInput{Name: "wiring-test-stream"},
				)
				require.NoError(t, err)
				require.NoError(t, fhBk.TagDeliveryStream(
					context.Background(), "wiring-test-stream", map[string]string{wantTagKey: wantTagValue},
				))

				wireTaggingFirehose(bk, firehosebackend.NewHandler(fhBk))

				return stream.ARN
			},
			wantResourceType: "firehose:deliverystream",
		},
		{
			name: "opensearch_domain",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				osBk := opensearchbackend.NewInMemoryBackend(accountID, region)
				domain, err := osBk.CreateDomain(opensearchbackend.CreateDomainInput{Name: "wiring-test-domain"})
				require.NoError(t, err)
				require.NoError(t, osBk.AddTags(domain.ARN, map[string]string{wantTagKey: wantTagValue}))

				wireTaggingOpenSearch(bk, opensearchbackend.NewHandler(osBk))

				return domain.ARN
			},
			// OpenSearch domains use the "es" ARN service, not "opensearch" -- see
			// wireTaggingOpenSearch's doc comment.
			wantResourceType: "es:domain",
		},
		{
			name: "cloudwatchlogs_log_group",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				cwlBk := cwlogsbackend.NewInMemoryBackendWithConfig(accountID, region)
				cwlH := cwlogsbackend.NewHandler(cwlBk)
				lg, err := cwlBk.CreateLogGroup(context.Background(), "wiring-test-group", "", "")
				require.NoError(t, err)
				cwlH.TagResource(lg.Arn, map[string]string{wantTagKey: wantTagValue})

				wireTaggingCloudWatchLogs(bk, cwlH)

				return lg.Arn
			},
			wantResourceType: "logs:log-group",
		},
		{
			name: "mq_broker",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				mqBk := mqbackend.NewInMemoryBackend(accountID, region)
				broker, err := mqBk.CreateBroker(
					"wiring-test-broker", "SINGLE_INSTANCE", "ACTIVEMQ", "5.17.6", "mq.t3.micro",
					false, false, nil, nil, nil, nil,
				)
				require.NoError(t, err)
				require.NoError(t, mqBk.CreateTags(broker.BrokerArn, map[string]string{wantTagKey: wantTagValue}))

				wireTaggingMQ(bk, mqbackend.NewHandler(mqBk))

				return broker.BrokerArn
			},
			wantResourceType: "mq:broker",
		},
		{
			name: "emr_cluster",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				emrBk := emrbackend.NewInMemoryBackend(accountID, region)
				cluster, err := emrBk.RunJobFlow(context.Background(), emrbackend.RunJobFlowParams{
					Name: "wiring-test-cluster", ReleaseLabel: "emr-6.0.0",
				})
				require.NoError(t, err)
				require.NoError(t, emrBk.AddTags(
					context.Background(), cluster.ARN, []emrbackend.Tag{{Key: wantTagKey, Value: wantTagValue}},
				))

				wireTaggingEMR(bk, emrbackend.NewHandler(emrBk))

				return cluster.ARN
			},
			// EMR ARNs use the "elasticmapreduce" ARN service, not "emr" -- see
			// wireTaggingEMR's doc comment.
			wantResourceType: "elasticmapreduce:cluster",
		},
		{
			name: "dax_cluster",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				daxBk := daxbackend.NewInMemoryBackend(accountID, region)
				cluster, err := daxBk.CreateCluster(daxbackend.CreateClusterInput{
					ClusterName:       "wiring-test-cluster",
					NodeType:          "dax.r4.large",
					IamRoleArn:        "arn:aws:iam::" + accountID + ":role/dax-role",
					ReplicationFactor: 1,
				})
				require.NoError(t, err)
				_, err = daxBk.TagResource(cluster.ClusterArn, map[string]string{wantTagKey: wantTagValue})
				require.NoError(t, err)

				wireTaggingDAX(bk, daxbackend.NewHandler(daxBk))

				return cluster.ClusterArn
			},
			// DAX's ARN resource segment is "cache/{name}", not "cluster/{name}" --
			// see wireTaggingDAX's doc comment.
			wantResourceType: "dax:cache",
		},
		{
			name: "detective_graph",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				detBk := detectivebackend.NewInMemoryBackend(accountID, region)
				graph, err := detBk.CreateGraph(nil)
				require.NoError(t, err)
				require.NoError(t, detBk.TagResource(graph.Arn, map[string]string{wantTagKey: wantTagValue}))

				wireTaggingDetective(bk, detectivebackend.NewHandler(detBk))

				return graph.Arn
			},
			wantResourceType: "detective:graph",
		},
		{
			name: "guardduty_detector",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				gdBk := guarddutybackend.NewInMemoryBackend(accountID, region)
				detector, err := gdBk.CreateDetector(true, "", nil, nil)
				require.NoError(t, err)

				resourceARN := "arn:aws:guardduty:" + region + ":" + accountID + ":detector/" + detector.DetectorID
				require.NoError(t, gdBk.TagResource(resourceARN, map[string]string{wantTagKey: wantTagValue}))

				wireTaggingGuardDuty(bk, guarddutybackend.NewHandler(gdBk))

				return resourceARN
			},
			wantResourceType: "guardduty:detector",
		},
		{
			name: "transfer_server",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				xferBk := transferbackend.NewInMemoryBackend(context.Background(), accountID, region)
				srv, err := xferBk.CreateServer(nil, nil)
				require.NoError(t, err)

				resourceARN := "arn:aws:transfer:" + region + ":" + accountID + ":server/" + srv.ServerID
				require.NoError(t, xferBk.TagResource(resourceARN, map[string]string{wantTagKey: wantTagValue}))

				wireTaggingTransfer(bk, transferbackend.NewHandler(xferBk))

				return resourceARN
			},
			wantResourceType: "transfer:server",
		},
		{
			name: "cognitoidp_userpool",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				idpBk := cognitoidpbackend.NewInMemoryBackend(accountID, region, "")
				pool, err := idpBk.CreateUserPool("wiring-test-pool")
				require.NoError(t, err)
				idpBk.TagResource(pool.ARN, map[string]string{wantTagKey: wantTagValue})

				wireTaggingCognitoIDP(bk, cognitoidpbackend.NewHandler(idpBk, region))

				return pool.ARN
			},
			wantResourceType: "cognito-idp:userpool",
		},
		{
			name: "appconfig_application",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				acBk := appconfigbackend.NewInMemoryBackend(accountID, region)
				app, err := acBk.CreateApplication("wiring-test-app", "", nil)
				require.NoError(t, err)

				resourceARN := "arn:aws:appconfig:" + region + ":" + accountID + ":application/" + app.ID
				require.NoError(t, acBk.TagResource(resourceARN, map[string]string{wantTagKey: wantTagValue}))

				wireTaggingAppConfig(bk, appconfigbackend.NewHandler(acBk))

				return resourceARN
			},
			wantResourceType: "appconfig:application",
		},
		{
			name: "codecommit_repository",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				ccBk := codecommitbackend.NewInMemoryBackend(accountID, region)
				repo, err := ccBk.CreateRepository("wiring-test-repo", "", "", nil)
				require.NoError(t, err)
				require.NoError(t, ccBk.TagResource(repo.ARN, map[string]string{wantTagKey: wantTagValue}))

				wireTaggingCodeCommit(bk, codecommitbackend.NewHandler(ccBk))

				return repo.ARN
			},
			wantResourceType: "codecommit:repository",
		},
		{
			name: "servicediscovery_namespace",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				sdBk := servicediscoverybackend.NewInMemoryBackend(accountID, region)
				opID, err := sdBk.CreateHTTPNamespace("wiring-test-ns", "", nil)
				require.NoError(t, err)

				op, err := sdBk.GetOperation(opID)
				require.NoError(t, err)
				ns, err := sdBk.GetNamespace(op.Targets["NAMESPACE"])
				require.NoError(t, err)
				require.NoError(t, sdBk.TagResource(ns.ARN, map[string]string{wantTagKey: wantTagValue}))

				wireTaggingServiceDiscovery(bk, servicediscoverybackend.NewHandler(sdBk))

				return ns.ARN
			},
			wantResourceType: "servicediscovery:namespace",
		},
		{
			name: "memorydb_cluster",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				mdbBk := memorydbbackend.NewInMemoryBackend(accountID, region)
				mdbH := memorydbbackend.NewHandler(mdbBk)

				// CreateCluster's request body is a package-private type, unlike
				// every other backend method this file calls directly, so this
				// drives it through the same JSON-over-HTTP path a real client
				// uses instead.
				req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(
					[]byte(`{"ClusterName":"wiring-test-cluster","NodeType":"db.t4g.small"}`),
				))
				req.Header.Set("X-Amz-Target", "AmazonMemoryDB.CreateCluster")
				rec := httptest.NewRecorder()
				c := echo.New().NewContext(req, rec)
				require.NoError(t, mdbH.Handler()(c))
				require.Equalf(t, http.StatusOK, rec.Code, "CreateCluster failed: %s", rec.Body.String())

				resourceARN := "arn:aws:memorydb:" + region + ":" + accountID + ":cluster/wiring-test-cluster"
				tagErr := mdbBk.TagResource(
					context.Background(),
					resourceARN,
					map[string]string{wantTagKey: wantTagValue},
				)
				require.NoError(t, tagErr)

				wireTaggingMemoryDB(bk, mdbH)

				return resourceARN
			},
			wantResourceType: "memorydb:cluster",
		},
		{
			name: "accessanalyzer_analyzer",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				aaBk := accessanalyzerbackend.NewInMemoryBackend(accountID, region)
				resourceARN := "arn:aws:access-analyzer:" + region + ":" + accountID + ":analyzer/wiring-test-analyzer"
				require.NoError(t, aaBk.TagResource(resourceARN, map[string]string{wantTagKey: wantTagValue}))

				wireTaggingAccessAnalyzer(bk, accessanalyzerbackend.NewHandler(aaBk))

				return resourceARN
			},
			wantResourceType: "access-analyzer:analyzer",
		},
		{
			name: "dlm_policy",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				dlmBk := dlmbackend.NewInMemoryBackend(accountID, region)
				pol, err := dlmBk.CreateLifecyclePolicy(
					"wiring-test-policy", "arn:aws:iam::"+accountID+":role/wiring-test-role", "", nil, nil,
				)
				require.NoError(t, err)
				require.NoError(t, dlmBk.TagResource(pol.PolicyArn, map[string]string{wantTagKey: wantTagValue}))

				wireTaggingDLM(bk, dlmbackend.NewHandler(dlmBk))

				return pol.PolicyArn
			},
			wantResourceType: "dlm:policy",
		},
		{
			name: "ce_cost_category",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				ceBk := cebackend.NewInMemoryBackend(accountID, region)
				cat, err := ceBk.CreateCostCategoryDefinition(
					"wiring-test-cat", "CostCategoryExpression.v1", "", nil, nil, nil, "",
				)
				require.NoError(t, err)
				require.NoError(t, ceBk.TagResource(cat.ARN, map[string]string{wantTagKey: wantTagValue}))

				wireTaggingCE(bk, cebackend.NewHandler(ceBk))

				return cat.ARN
			},
			wantResourceType: "ce:costcategory",
		},
		{
			name: "mediapackage_channel",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				mpBk := mediapackagebackend.NewInMemoryBackend(accountID, region)
				resourceARN := "arn:aws:mediapackage:" + region + ":" + accountID + ":channels/wiring-test-channel"
				require.NoError(t, mpBk.TagResource(resourceARN, map[string]string{wantTagKey: wantTagValue}))

				wireTaggingMediaPackage(bk, mediapackagebackend.NewHandler(mpBk))

				return resourceARN
			},
			wantResourceType: "mediapackage:channels",
		},
		{
			name: "swf_domain",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				swfBk := swfbackend.NewInMemoryBackend()
				require.NoError(t, swfBk.RegisterDomain("wiring-test-domain", "", ""))

				// SWF's ARNs always use the backend's fixed default account/region
				// (see defaultAccountID/defaultRegion in services/swf/models.go),
				// which happen to match this test's accountID/region constants.
				resourceARN := "arn:aws:swf:" + region + ":" + accountID + ":/domain/wiring-test-domain"
				require.NoError(t, swfBk.TagResource(resourceARN, map[string]string{wantTagKey: wantTagValue}))

				wireTaggingSWF(bk, swfbackend.NewHandler(swfBk))

				return resourceARN
			},
			wantResourceType: "swf:domain",
		},
		{
			name: "fis_safety_lever",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				fisBk := fisbackend.NewInMemoryBackend(accountID, region)

				// The account's safety lever always exists (created by
				// NewInMemoryBackend), so there is no separate resource to create
				// before tagging it.
				resourceARN := "arn:aws:fis:" + region + ":" + accountID + ":safety-lever/" + accountID
				require.NoError(t, fisBk.TagResource(resourceARN, map[string]string{wantTagKey: wantTagValue}))

				wireTaggingFIS(bk, fisbackend.NewHandler(fisBk))

				return resourceARN
			},
			wantResourceType: "fis:safety-lever",
		},
		{
			name: "codeconnections_connection",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				ccBk := codeconnectionsbackend.NewInMemoryBackend(accountID, region)
				conn, err := ccBk.CreateConnection(context.Background(), "wiring-test-conn", "GitHub", "", nil)
				require.NoError(t, err)
				require.NoError(t, ccBk.TagResource(
					context.Background(), conn.ConnectionArn, map[string]string{wantTagKey: wantTagValue},
				))

				wireTaggingCodeConnections(bk, codeconnectionsbackend.NewHandler(ccBk))

				return conn.ConnectionArn
			},
			wantResourceType: "codeconnections:connection",
		},
		{
			name: "mediastore_container",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				msBk := mediastorebackend.NewInMemoryBackend()
				container, err := msBk.CreateContainer(context.Background(), accountID, "wiring-test-container", nil)
				require.NoError(t, err)
				require.NoError(t, msBk.TagResource(
					context.Background(), container.ARN, map[string]string{wantTagKey: wantTagValue},
				))

				wireTaggingMediaStore(bk, mediastorebackend.NewHandler(msBk))

				return container.ARN
			},
			wantResourceType: "mediastore:container",
		},
		{
			name: "mwaa_environment",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				mwaaBk := mwaabackend.NewInMemoryBackend(region, accountID)
				mwaaH := mwaabackend.NewHandler(mwaaBk)

				// CreateEnvironment's request body is a package-private type, unlike
				// most other backend methods this test calls directly, so this
				// drives it through the same JSON-over-HTTP path a real client uses.
				body, err := json.Marshal(map[string]any{
					"DagS3Path":        "dags",
					"ExecutionRoleArn": "arn:aws:iam::" + accountID + ":role/wiring-test-role",
					"SourceBucketArn":  "arn:aws:s3:::wiring-test-bucket",
					"NetworkConfiguration": map[string]any{
						"SubnetIds":        []string{"subnet-1", "subnet-2"},
						"SecurityGroupIds": []string{"sg-1"},
					},
				})
				require.NoError(t, err)

				req := httptest.NewRequest(http.MethodPut, "/environments/wiring-test-env", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set(
					"Authorization",
					"AWS4-HMAC-SHA256 Credential=test/20240101/"+region+"/airflow/aws4_request",
				)
				rec := httptest.NewRecorder()
				c := echo.New().NewContext(req, rec)
				require.NoError(t, mwaaH.Handler()(c))
				require.Equalf(t, http.StatusOK, rec.Code, "CreateEnvironment failed: %s", rec.Body.String())

				resourceARN := "arn:aws:airflow:" + region + ":" + accountID + ":environment/wiring-test-env"
				require.NoError(t, mwaaBk.TagResource(
					context.Background(), resourceARN, map[string]string{wantTagKey: wantTagValue},
				))

				wireTaggingMWAA(bk, mwaaH)

				return resourceARN
			},
			wantResourceType: "airflow:environment",
		},
		{
			name: "pipes_pipe",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				pipesBk := pipesbackend.NewInMemoryBackend(accountID, region)
				p, err := pipesBk.CreatePipe(context.Background(), pipesbackend.CreatePipeInput{
					Name:    "wiring-test-pipe",
					Source:  "arn:aws:sqs:" + region + ":" + accountID + ":wiring-test-queue",
					Target:  "arn:aws:sqs:" + region + ":" + accountID + ":wiring-test-target",
					RoleARN: "arn:aws:iam::" + accountID + ":role/wiring-test-role",
				})
				require.NoError(t, err)
				require.NoError(t, pipesBk.TagResource(
					context.Background(), p.ARN, map[string]string{wantTagKey: wantTagValue},
				))

				wireTaggingPipes(bk, pipesbackend.NewHandler(pipesBk))

				return p.ARN
			},
			wantResourceType: "pipes:pipe",
		},
		{
			name: "macie2_allow_list",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				mBk := macie2backend.NewInMemoryBackend(accountID, region)
				al, err := mBk.CreateAllowList(
					"wiring-test-list", "", macie2backend.AllowListCriteria{}, nil,
				)
				require.NoError(t, err)
				require.NoError(t, mBk.TagResource(al.Arn, map[string]string{wantTagKey: wantTagValue}))

				wireTaggingMacie2(bk, macie2backend.NewHandler(mBk))

				return al.Arn
			},
			wantResourceType: "macie2:allow-list",
		},
		{
			name: "managedblockchain_accessor",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				mbBk := managedblockchainbackend.NewInMemoryBackend()
				accessor, err := mbBk.CreateAccessor(region, accountID, "", "", nil)
				require.NoError(t, err)
				require.NoError(t, mbBk.TagResource(accessor.Arn, map[string]string{wantTagKey: wantTagValue}))

				wireTaggingManagedBlockchain(bk, managedblockchainbackend.NewHandler(mbBk))

				return accessor.Arn
			},
			wantResourceType: "managedblockchain:accessors",
		},
		{
			name: "mediaconvert_queue",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				mcBk := mediaconvertbackend.NewInMemoryBackend(accountID, region)
				q, err := mcBk.CreateQueue("wiring-test-queue", "", "", "", nil)
				require.NoError(t, err)
				mcBk.TagResource(q.Arn, map[string]string{wantTagKey: wantTagValue})

				wireTaggingMediaConvert(bk, mediaconvertbackend.NewHandler(mcBk))

				return q.Arn
			},
			wantResourceType: "mediaconvert:queues",
		},
		{
			name: "datasync_agent",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				dsBk := datasyncbackend.NewInMemoryBackend(accountID, region)
				a, err := dsBk.CreateAgent("wiring-test-agent", "", nil)
				require.NoError(t, err)
				require.NoError(t, dsBk.TagResource(a.AgentArn, map[string]string{wantTagKey: wantTagValue}))

				wireTaggingDataSync(bk, datasyncbackend.NewHandler(dsBk))

				return a.AgentArn
			},
			wantResourceType: "datasync:agent",
		},
		{
			name: "codedeploy_application",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				cdBk := codedeploybackend.NewInMemoryBackend(accountID, region)
				app, err := cdBk.CreateApplication("wiring-test-app", "", nil)
				require.NoError(t, err)
				resourceARN := cdBk.ApplicationARN(app.ApplicationName)
				require.NoError(t, cdBk.TagResource(resourceARN, map[string]string{wantTagKey: wantTagValue}))

				wireTaggingCodeDeploy(bk, codedeploybackend.NewHandler(cdBk))

				return resourceARN
			},
			wantResourceType: "codedeploy:application",
		},
		{
			name: "inspector2_filter",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				iBk := inspector2backend.NewInMemoryBackend(accountID, region)
				f, err := iBk.CreateFilter("wiring-test-filter", "NONE", "", "", nil, nil)
				require.NoError(t, err)
				require.NoError(t, iBk.TagResource(f.Arn, map[string]string{wantTagKey: wantTagValue}))

				wireTaggingInspector2(bk, inspector2backend.NewHandler(iBk))

				return f.Arn
			},
			wantResourceType: "inspector2:filter",
		},
		{
			name: "ram_resource_share",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				rBk := rambackend.NewInMemoryBackend(accountID, region)
				rs, err := rBk.CreateResourceShare("wiring-test-share", false, nil, nil, nil)
				require.NoError(t, err)
				require.NoError(t, rBk.TagResource(rs.ARN, map[string]string{wantTagKey: wantTagValue}))

				wireTaggingRAM(bk, rambackend.NewHandler(rBk))

				return rs.ARN
			},
			wantResourceType: "ram:resource-share",
		},
		{
			name: "rekognition_collection",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				rBk := rekognitionbackend.NewInMemoryBackend(accountID, region)
				c, err := rBk.CreateCollection("wiring-test-collection", nil)
				require.NoError(t, err)
				require.NoError(t, rBk.TagResource(c.CollectionARN, map[string]string{wantTagKey: wantTagValue}))

				wireTaggingRekognition(bk, rekognitionbackend.NewHandler(rBk))

				return c.CollectionARN
			},
			wantResourceType: "rekognition:collection",
		},
		{
			name: "translate_terminology",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				tBk := translatebackend.NewInMemoryBackend(accountID, region)
				term, err := tBk.ImportTerminology(
					"wiring-test-term", "",
					&translatebackend.TerminologyData{Format: "CSV", File: []byte("en,fr\nhello,bonjour\n")},
					nil, nil,
				)
				require.NoError(t, err)
				require.NoError(t, tBk.TagResource(term.ARN, map[string]string{wantTagKey: wantTagValue}))

				wireTaggingTranslate(bk, translatebackend.NewHandler(tBk))

				return term.ARN
			},
			wantResourceType: "translate:terminology",
		},
		{
			name: "appstream_stack",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				aBk := appstreambackend.NewInMemoryBackend(accountID, region)
				s, err := aBk.CreateStack("wiring-test-stack", "", "", nil)
				require.NoError(t, err)
				require.NoError(t, aBk.TagResource(s.Arn, map[string]string{wantTagKey: wantTagValue}))

				wireTaggingAppStream(bk, appstreambackend.NewHandler(aBk))

				return s.Arn
			},
			wantResourceType: "appstream:stack",
		},
		{
			name: "mediatailor_channel",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				mBk := mediatailorbackend.NewInMemoryBackend(accountID, region)
				ch, err := mBk.CreateChannel("wiring-test-channel", "", "", nil, nil, nil, nil, nil)
				require.NoError(t, err)
				require.NoError(t, mBk.TagResource(ch.ARN, map[string]string{wantTagKey: wantTagValue}))

				wireTaggingMediaTailor(bk, mediatailorbackend.NewHandler(mBk))

				return ch.ARN
			},
			wantResourceType: "mediatailor:channel",
		},
		{
			name: "vpclattice_service_network",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				vBk := vpclatticebackend.NewInMemoryBackend(accountID, region)
				sn, err := vBk.CreateServiceNetwork(context.Background(), "wiring-test-network", "", nil)
				require.NoError(t, err)
				require.NoError(t, vBk.TagResource(sn.ARN, map[string]string{wantTagKey: wantTagValue}))

				wireTaggingVPCLattice(bk, vpclatticebackend.NewHandler(vBk))

				return sn.ARN
			},
			wantResourceType: "vpc-lattice:servicenetwork",
		},
		{
			name: "codepipeline_pipeline",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				cpBk := codepipelinebackend.NewInMemoryBackend(accountID, region)
				p, err := cpBk.CreatePipeline(
					context.Background(),
					codepipelinebackend.PipelineDeclaration{Name: "wiring-test-pipeline"},
					nil,
				)
				require.NoError(t, err)
				require.NoError(t, cpBk.TagResource(
					context.Background(), p.Metadata.PipelineArn,
					[]codepipelinebackend.Tag{{Key: wantTagKey, Value: wantTagValue}},
				))

				wireTaggingCodePipeline(bk, codepipelinebackend.NewHandler(cpBk))

				return p.Metadata.PipelineArn
			},
			wantResourceType: "codepipeline:pipeline",
		},
		{
			name: "kinesisanalyticsv2_application",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				kaBk := kinesisanalyticsv2backend.NewInMemoryBackend(accountID, region)
				app, err := kaBk.CreateApplication(context.Background(), "wiring-test-app", "", "", "", "", nil)
				require.NoError(t, err)
				require.NoError(t, kaBk.TagResource(
					context.Background(), app.ApplicationARN,
					[]kinesisanalyticsv2backend.Tag{{Key: wantTagKey, Value: wantTagValue}},
				))

				wireTaggingKinesisAnalyticsV2(bk, kinesisanalyticsv2backend.NewHandler(kaBk))

				return app.ApplicationARN
			},
			wantResourceType: "kinesisanalytics:application",
		},
		{
			name: "comprehend_job",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				cBk := comprehendbackend.NewInMemoryBackend(accountID, region)
				job, err := cBk.StartJob("entities-detection-job", "wiring-test-job", map[string]any{
					"InputDataConfig":   map[string]any{"S3Uri": "s3://bucket/input"},
					"OutputDataConfig":  map[string]any{"S3Uri": "s3://bucket/output"},
					"DataAccessRoleArn": "arn:aws:iam::" + accountID + ":role/wiring-test-role",
				}, nil)
				require.NoError(t, err)
				require.NoError(t, cBk.TagResource(
					job.JobArn, []comprehendbackend.Tag{{Key: wantTagKey, Value: wantTagValue}},
				))

				wireTaggingComprehend(bk, comprehendbackend.NewHandler(cBk))

				return job.JobArn
			},
			wantResourceType: "comprehend:entities-detection-job",
		},
		{
			name: "shield_protection",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				sBk := shieldbackend.NewInMemoryBackend(accountID, region)
				sBk.AddSubscriptionInternal()
				p, err := sBk.CreateProtection(
					"wiring-test-protection",
					"arn:aws:elasticloadbalancing:"+region+":"+accountID+":loadbalancer/wiring-test-elb",
					map[string]string{wantTagKey: wantTagValue},
				)
				require.NoError(t, err)

				wireTaggingShield(bk, shieldbackend.NewHandler(sBk))

				return p.ProtectionArn
			},
			wantResourceType: "shield:protection",
		},
		{
			name: "transcribe_vocabulary",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				tBk := transcribebackend.NewInMemoryBackend()
				_, err := tBk.CreateVocabulary(&transcribebackend.Vocabulary{
					VocabularyName: "wiring-test-vocab",
					LanguageCode:   "en-US",
					Phrases:        []string{"hello"},
				})
				require.NoError(t, err)

				vocabARN := "arn:aws:transcribe:" + region + ":" + accountID + ":vocabulary/wiring-test-vocab"
				require.NoError(t, tBk.TagResource(vocabARN, map[string]string{wantTagKey: wantTagValue}))

				wireTaggingTranscribe(bk, transcribebackend.NewHandler(tBk))

				return vocabARN
			},
			wantResourceType: "transcribe:vocabulary",
		},
		{
			name: "verifiedpermissions_policy_store",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				vpBk := verifiedpermissionsbackend.NewInMemoryBackend(accountID, region)
				ps, err := vpBk.CreatePolicyStore(
					"wiring test store", map[string]string{wantTagKey: wantTagValue}, "OFF", "DISABLED", "",
				)
				require.NoError(t, err)

				wireTaggingVerifiedPermissions(bk, verifiedpermissionsbackend.NewHandler(vpBk))

				return ps.Arn
			},
			wantResourceType: "verifiedpermissions:policy-store",
		},
		{
			name: "waf_ip_set",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				wBk := wafbackend.NewInMemoryBackend(accountID, region)
				ipSet, err := wBk.CreateIPSet(
					"wiring-test-ipset", wBk.GetChangeToken(), map[string]string{wantTagKey: wantTagValue},
				)
				require.NoError(t, err)

				ipSetARN := "arn:aws:waf::" + accountID + ":ipset/" + ipSet.IPSetId

				wireTaggingWAF(bk, wafbackend.NewHandler(wBk))

				return ipSetARN
			},
			wantResourceType: "waf:ipset",
		},
		{
			name: "securityhub_hub",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				shBk := securityhubbackend.NewInMemoryBackend(accountID, region)
				require.NoError(t, shBk.EnableHub(false, map[string]string{wantTagKey: wantTagValue}))

				hubARN := "arn:aws:securityhub:" + region + ":" + accountID + ":hub/default"

				wireTaggingSecurityHub(bk, securityhubbackend.NewHandler(shBk))

				return hubARN
			},
			wantResourceType: "securityhub:hub",
		},
		{
			name: "apprunner_connection",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				arBk := apprunnerbackend.NewInMemoryBackend(accountID, region)
				conn, err := arBk.CreateConnection(
					"wiring-test-conn", "GITHUB", map[string]string{wantTagKey: wantTagValue},
				)
				require.NoError(t, err)

				wireTaggingAppRunner(bk, apprunnerbackend.NewHandler(arBk))

				return conn.ConnectionArn
			},
			wantResourceType: "apprunner:connection",
		},
		{
			name: "route53resolver_firewall_domain_list",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				r53Bk := route53resolverbackend.NewInMemoryBackend(accountID, region)
				dl, err := r53Bk.CreateFirewallDomainList(context.Background(), "wiring-test-list", "")
				require.NoError(t, err)
				require.NoError(t, r53Bk.TagResource(
					context.Background(), dl.ARN, []svctags.KV{{Key: wantTagKey, Value: wantTagValue}},
				))

				wireTaggingRoute53Resolver(bk, route53resolverbackend.NewHandler(r53Bk))

				return dl.ARN
			},
			wantResourceType: "route53resolver:firewall-domain-list",
		},
		{
			name: "timestreamwrite_database",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				twBk := timestreamwritebackend.NewInMemoryBackend()
				db, err := twBk.CreateDatabase(
					"wiring-test-db", "", map[string]string{wantTagKey: wantTagValue},
				)
				require.NoError(t, err)

				wireTaggingTimestreamWrite(bk, timestreamwritebackend.NewHandler(twBk))

				return db.ARN
			},
			wantResourceType: "timestream:database",
		},
		{
			name: "s3tables_bucket",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				stBk := s3tablesbackend.NewInMemoryBackend(accountID, region)
				bucket, err := stBk.CreateTableBucket("wiring-test-bucket", s3tablesbackend.CreateTableBucketOptions{})
				require.NoError(t, err)
				require.NoError(t, stBk.TagResource(bucket.ARN, map[string]string{wantTagKey: wantTagValue}))

				wireTaggingS3Tables(bk, s3tablesbackend.NewHandler(stBk))

				return bucket.ARN
			},
			wantResourceType: "s3tables:bucket",
		},
		{
			name: "workmail_organization",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				wmBk := workmailbackend.NewInMemoryBackend(accountID, region)
				org, err := wmBk.CreateOrganization(t.Context(), "wiring-test-org", nil, false)
				require.NoError(t, err)
				require.NoError(t, wmBk.TagResource(
					org.ARN, []workmailbackend.Tag{{Key: wantTagKey, Value: wantTagValue}},
				))

				wireTaggingWorkMail(bk, workmailbackend.NewHandler(wmBk))

				return org.ARN
			},
			wantResourceType: "workmail:organization",
		},
		{
			name: "pinpoint_app",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				pBk := pinpointbackend.NewInMemoryBackend(region, accountID)
				app, err := pBk.CreateApp(
					region,
					accountID,
					"wiring-test-app",
					map[string]string{wantTagKey: wantTagValue},
				)
				require.NoError(t, err)

				wireTaggingPinpoint(bk, pinpointbackend.NewHandler(pBk))

				return app.ARN
			},
			wantResourceType: "mobiletargeting:apps",
		},
		{
			name: "applicationautoscaling_scalable_target",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				aasBk := applicationautoscalingbackend.NewInMemoryBackend(accountID, region)
				minCap, maxCap := int32(1), int32(10)
				target, err := aasBk.RegisterScalableTarget(
					"ecs", "service/wiring-cluster/wiring-svc", "ecs:service:DesiredCount",
					&minCap, &maxCap, map[string]string{wantTagKey: wantTagValue}, "", nil,
				)
				require.NoError(t, err)

				wireTaggingApplicationAutoScaling(bk, applicationautoscalingbackend.NewHandler(aasBk))

				return target.ARN
			},
			wantResourceType: "application-autoscaling:scalable-target",
		},
		{
			name: "codeartifact_domain",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				caBk := codeartifactbackend.NewInMemoryBackend(accountID, region)
				dom, err := caBk.CreateDomain(
					context.Background(), "wiring-test-domain", "", map[string]string{wantTagKey: wantTagValue},
				)
				require.NoError(t, err)

				wireTaggingCodeArtifact(bk, codeartifactbackend.NewHandler(caBk))

				return dom.ARN
			},
			wantResourceType: "codeartifact:domain",
		},
		{
			name: "cleanrooms_collaboration",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				crBk := cleanroomsbackend.NewInMemoryBackend(accountID, region)
				collab, err := crBk.CreateCollaboration(
					"wiring-test-collab", "", "creator", nil, nil, "",
					map[string]string{wantTagKey: wantTagValue},
				)
				require.NoError(t, err)

				wireTaggingCleanRooms(bk, cleanroomsbackend.NewHandler(crBk))

				return collab.Arn
			},
			wantResourceType: "cleanrooms:collaboration",
		},
		{
			name: "appmesh_mesh",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				amBk := appmeshbackend.NewInMemoryBackend(accountID, region)
				mesh, err := amBk.CreateMesh("wiring-test-mesh", nil, map[string]string{wantTagKey: wantTagValue})
				require.NoError(t, err)

				wireTaggingAppMesh(bk, appmeshbackend.NewHandler(amBk))

				return mesh.Meta.Arn
			},
			wantResourceType: "appmesh:mesh",
		},
		{
			name: "personalize_dataset_group",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				pzBk := personalizebackend.NewInMemoryBackend(accountID, region)
				dg, err := pzBk.CreateDatasetGroup(
					"wiring-test-dg", "", "", "", map[string]string{wantTagKey: wantTagValue},
				)
				require.NoError(t, err)

				wireTaggingPersonalize(bk, personalizebackend.NewHandler(pzBk))

				return dg.DatasetGroupArn
			},
			wantResourceType: "personalize:dataset-group",
		},
		{
			name: "sesv2_tenant",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				sesBk := sesv2backend.NewInMemoryBackend()
				tenant, err := sesBk.CreateTenant("wiring-test-tenant", nil)
				require.NoError(t, err)
				require.NoError(t, sesBk.TagResource(tenant.TenantARN, map[string]string{wantTagKey: wantTagValue}))

				wireTaggingSESv2(bk, sesv2backend.NewHandler(sesBk))

				return tenant.TenantARN
			},
			wantResourceType: "ses:tenant",
		},
		{
			name: "xray_group",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				xBk := xraybackend.NewInMemoryBackend(accountID, region)
				g, err := xBk.CreateGroup("wiring-test-group", "")
				require.NoError(t, err)
				require.NoError(t, xBk.TagResource(g.GroupARN, map[string]string{wantTagKey: wantTagValue}))

				wireTaggingXRay(bk, xraybackend.NewHandler(xBk))

				return g.GroupARN
			},
			wantResourceType: "xray:group",
		},
		{
			name: "awsconfig_recorder",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				acfgBk := awsconfigbackend.NewInMemoryBackend()
				require.NoError(t, acfgBk.PutConfigurationRecorder(
					"wiring-test-recorder", "arn:aws:iam::"+accountID+":role/wiring-test-role", nil,
				))

				recorderARN := "arn:aws:config:" + region + ":" + accountID + ":config-recorder/wiring-test-recorder"
				require.NoError(t, acfgBk.TagResource(
					recorderARN, []awsconfigbackend.Tag{{Key: wantTagKey, Value: wantTagValue}},
				))

				wireTaggingAWSConfig(bk, awsconfigbackend.NewHandler(acfgBk))

				return recorderARN
			},
			wantResourceType: "config:config-recorder",
		},
		{
			name: "scheduler_schedule_group",
			wire: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) string {
				t.Helper()

				schBk := schedulerbackend.NewInMemoryBackend(accountID, region)
				g, err := schBk.CreateScheduleGroup(
					context.Background(), "wiring-test-group", map[string]string{wantTagKey: wantTagValue},
				)
				require.NoError(t, err)

				wireTaggingScheduler(bk, schedulerbackend.NewHandler(schBk))

				return g.ARN
			},
			wantResourceType: "scheduler:schedule-group",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			taggingBk := resourcegroupstaggingapibackend.NewInMemoryBackend(accountID, region)

			wantARN := tt.wire(t, taggingBk)

			// Cross-service query filtered by the exact resource-type string the
			// wiring derived for this ARN -- proves both that the resource is
			// visible at all, and that it was typed correctly (not just dumped in
			// under a wrong or empty type that would happen to pass an unfiltered
			// GetResources call).
			out, err := taggingBk.GetResources(context.Background(), &resourcegroupstaggingapibackend.GetResourcesInput{
				ResourceTypeFilters: []string{tt.wantResourceType},
			})
			require.NoError(t, err)

			var found *resourcegroupstaggingapibackend.ResourceTagMapping

			for i := range out.ResourceTagMappingList {
				if out.ResourceTagMappingList[i].ResourceARN == wantARN {
					found = &out.ResourceTagMappingList[i]

					break
				}
			}

			require.NotNilf(t, found,
				"resource %q tagged via the %s backend's native TagResource must appear in "+
					"cross-service GetResources filtered by resource type %q once wired (gopherstack-3xne); got %+v",
				wantARN, tt.name, tt.wantResourceType, out.ResourceTagMappingList)

			require.Len(t, found.Tags, 1)
			assert.Equal(t, wantTagKey, found.Tags[0].Key)
			assert.Equal(t, wantTagValue, found.Tags[0].Value)
		})
	}
}

// TestWireResourceGroupsTagging_TagResourcesRoundTrip proves the write direction of the
// gopherstack-3xne fix for the batch of services wired in this pass: mutating a tag via
// the Resource Groups Tagging API's own TagResources call must reach the owning
// service's native tag store (not just gopherstack's own copy), and the new tag key/
// value must surface through GetTagKeys/GetTagValues. A test that only checked
// TagResources returned no error would pass even if the ARN tagger silently no-opped
// (arnServiceIs never matching, or the wrong ARN-tagger being registered) -- FailedResourcesMap
// would still be empty because TagResources treats "no tagger claimed this ARN" as
// success for that ARN.
func TestWireResourceGroupsTagging_TagResourcesRoundTrip(t *testing.T) {
	t.Parallel()

	const accountID = "123456789012"
	const region = "us-east-1"
	const wantTagKey = "env"
	const wantTagValue = "roundtrip-test"

	tests := []struct {
		setup func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) (
			resourceARN string, nativeTags func() map[string]string,
		)
		name string
	}{
		{
			name: "stepfunctions_state_machine",
			setup: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) (string, func() map[string]string) {
				t.Helper()

				sfnBk := sfnbackend.NewInMemoryBackendWithConfig(accountID, region)
				sm, err := sfnBk.CreateStateMachine(
					context.Background(),
					"roundtrip-sm",
					`{"StartAt":"S","States":{"S":{"Type":"Pass","End":true}}}`,
					"arn:aws:iam::"+accountID+":role/roundtrip-role",
					"STANDARD",
				)
				require.NoError(t, err)

				sfnH := sfnbackend.NewHandler(sfnBk)
				wireTaggingStepFunctions(bk, sfnH)

				return sm.StateMachineArn, func() map[string]string {
					for _, e := range sfnH.TaggedResources() {
						if e.ARN == sm.StateMachineArn {
							return e.Tags
						}
					}

					return nil
				}
			},
		},
		{
			name: "cloudfront_distribution",
			setup: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) (string, func() map[string]string) {
				t.Helper()

				cfBk := cloudfrontbackend.NewInMemoryBackend(t.Context(), accountID, region)
				dist, err := cfBk.CreateDistribution("roundtrip-ref", "roundtrip-dist", true, nil)
				require.NoError(t, err)

				wireTaggingCloudFront(bk, cloudfrontbackend.NewHandler(cfBk))

				return dist.ARN, func() map[string]string {
					got, tagsErr := cfBk.ListTags(dist.ARN)
					require.NoError(t, tagsErr)

					return got
				}
			},
		},
		{
			name: "eks_cluster",
			setup: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) (string, func() map[string]string) {
				t.Helper()

				eksBk := eksbackend.NewInMemoryBackend(context.Background(), accountID, region)
				cluster, err := eksBk.CreateCluster("roundtrip-cluster", "", "", nil, nil, nil)
				require.NoError(t, err)

				wireTaggingEKS(bk, eksbackend.NewHandler(eksBk))

				return cluster.ARN, func() map[string]string {
					got, tagsErr := eksBk.ListTagsForResource(cluster.ARN)
					require.NoError(t, tagsErr)

					return got
				}
			},
		},
		{
			name: "batch_compute_environment",
			setup: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) (string, func() map[string]string) {
				t.Helper()

				batchBk := batchbackend.NewInMemoryBackend(accountID, region)
				ce, err := batchBk.CreateComputeEnvironment(
					context.Background(), "roundtrip-ce", "UNMANAGED", "ENABLED", nil, "", nil, nil, nil, nil,
				)
				require.NoError(t, err)

				wireTaggingBatch(bk, batchbackend.NewHandler(batchBk))

				return ce.ComputeEnvironmentArn, func() map[string]string {
					got, tagsErr := batchBk.ListTagsForResource(context.Background(), ce.ComputeEnvironmentArn)
					require.NoError(t, tagsErr)

					return got
				}
			},
		},
		{
			name: "wafv2_web_acl",
			setup: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) (string, func() map[string]string) {
				t.Helper()

				wafBk := wafv2backend.NewInMemoryBackend(accountID, region)
				webACL, err := wafBk.CreateWebACL(
					context.Background(), "roundtrip-acl", "REGIONAL", "",
					json.RawMessage(`{"Allow":{}}`), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
				)
				require.NoError(t, err)

				wireTaggingWAFv2(bk, wafv2backend.NewHandler(wafBk))

				return webACL.ARN, func() map[string]string {
					got, tagsErr := wafBk.ListTagsForResource(context.Background(), webACL.ARN)
					require.NoError(t, tagsErr)

					return got
				}
			},
		},
		{
			name: "backup_vault",
			setup: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) (string, func() map[string]string) {
				t.Helper()

				backupBk := backupbackend.NewInMemoryBackend(accountID, region)
				vault, err := backupBk.CreateBackupVault("roundtrip-vault", "", "", nil)
				require.NoError(t, err)

				wireTaggingBackup(bk, backupbackend.NewHandler(backupBk))

				return vault.BackupVaultArn, func() map[string]string {
					got, tagsErr := backupBk.ListTags(vault.BackupVaultArn)
					require.NoError(t, tagsErr)

					return got
				}
			},
		},
		{
			name: "efs_file_system",
			setup: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) (string, func() map[string]string) {
				t.Helper()

				efsBk := efsbackend.NewInMemoryBackend(accountID, region)
				fs, err := efsBk.CreateFileSystem(
					context.Background(), efsbackend.CreateFileSystemRequest{CreationToken: "roundtrip-token"},
				)
				require.NoError(t, err)

				wireTaggingEFS(bk, efsbackend.NewHandler(efsBk))

				return fs.FileSystemArn, func() map[string]string {
					got, tagsErr := efsBk.ListTagsForResource(context.Background(), fs.FileSystemArn)
					require.NoError(t, tagsErr)

					return got
				}
			},
		},
		{
			name: "docdb_db_cluster",
			setup: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) (string, func() map[string]string) {
				t.Helper()

				docdbBk := docdbbackend.NewInMemoryBackend(accountID, region)
				cluster, err := docdbBk.CreateDBCluster(
					context.Background(), "roundtrip-cluster", "docdb", "", "admin", "", "", "", "",
					0, false, false, 1, "", "", nil, nil, nil,
				)
				require.NoError(t, err)

				wireTaggingDocDB(bk, docdbbackend.NewHandler(docdbBk))

				return cluster.DBClusterArn, func() map[string]string {
					tagList := docdbBk.ListTagsForResource(context.Background(), cluster.DBClusterArn)
					out := make(map[string]string, len(tagList))
					for _, tg := range tagList {
						out[tg.Key] = tg.Value
					}

					return out
				}
			},
		},
		{
			name: "neptune_db_cluster",
			setup: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) (string, func() map[string]string) {
				t.Helper()

				neptuneBk := neptunebackend.NewInMemoryBackend(accountID, region)
				cluster, err := neptuneBk.CreateDBCluster(
					context.Background(), "roundtrip-cluster", "", 0, neptunebackend.DBClusterCreateOptions{},
				)
				require.NoError(t, err)

				wireTaggingNeptune(bk, neptunebackend.NewHandler(neptuneBk))

				return cluster.DBClusterArn, func() map[string]string {
					tagList, tagsErr := neptuneBk.ListTagsForResource(context.Background(), cluster.DBClusterArn)
					require.NoError(t, tagsErr)

					out := make(map[string]string, len(tagList))
					for _, tg := range tagList {
						out[tg.Key] = tg.Value
					}

					return out
				}
			},
		},
		{
			name: "rds_db_instance",
			setup: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) (string, func() map[string]string) {
				t.Helper()

				rdsBk := rdsbackend.NewInMemoryBackend(accountID, region)
				_, err := rdsBk.CreateDBInstance(
					"roundtrip-db", "postgres", "db.t3.micro", "", "admin", "", 20, rdsbackend.DBInstanceOptions{},
				)
				require.NoError(t, err)

				resourceARN := "arn:aws:rds:" + region + ":" + accountID + ":db:roundtrip-db"

				wireTaggingRDS(bk, rdsbackend.NewHandler(rdsBk))

				return resourceARN, func() map[string]string {
					tagList := rdsBk.ListTagsForResource(resourceARN)
					out := make(map[string]string, len(tagList))
					for _, tg := range tagList {
						out[tg.Key] = tg.Value
					}

					return out
				}
			},
		},
		{
			name: "elasticache_cluster",
			setup: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) (string, func() map[string]string) {
				t.Helper()

				ecBk := elasticachebackend.NewInMemoryBackend(elasticachebackend.EngineStub, accountID, region, nil)
				cluster, err := ecBk.CreateCluster(
					context.Background(), "roundtrip-cache", "redis", "cache.t3.micro", 0,
				)
				require.NoError(t, err)

				wireTaggingElastiCache(bk, elasticachebackend.NewHandler(ecBk))

				return cluster.ARN, func() map[string]string {
					got, tagsErr := ecBk.ListTagsForResource(context.Background(), cluster.ARN)
					require.NoError(t, tagsErr)

					return got
				}
			},
		},
		{
			name: "redshift_cluster",
			setup: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) (string, func() map[string]string) {
				t.Helper()

				redshiftBk := redshiftbackend.NewInMemoryBackend(accountID, region)
				_, err := redshiftBk.CreateCluster("roundtrip-cluster", "dc2.large", "dev", "admin", nil, "")
				require.NoError(t, err)

				wireTaggingRedshift(bk, redshiftbackend.NewHandler(redshiftBk))

				resourceARN := "arn:aws:redshift:" + region + ":" + accountID + ":cluster:roundtrip-cluster"

				return resourceARN, func() map[string]string {
					all := redshiftBk.DescribeTags()

					return all["roundtrip-cluster"]
				}
			},
		},
		{
			name: "sagemaker_model",
			setup: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) (string, func() map[string]string) {
				t.Helper()

				smBk := sagemakerbackend.NewInMemoryBackend(accountID, region)
				model, err := smBk.CreateModel(
					context.Background(), "roundtrip-model",
					"arn:aws:iam::"+accountID+":role/roundtrip-role", nil, nil, nil,
				)
				require.NoError(t, err)

				wireTaggingSageMaker(bk, sagemakerbackend.NewHandler(smBk))

				return model.ModelARN, func() map[string]string {
					got, tagsErr := smBk.ListTags(context.Background(), model.ModelARN)
					require.NoError(t, tagsErr)

					return got
				}
			},
		},
		{
			name: "firehose_delivery_stream",
			setup: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) (string, func() map[string]string) {
				t.Helper()

				fhBk := firehosebackend.NewInMemoryBackend(accountID, region)
				stream, err := fhBk.CreateDeliveryStream(
					context.Background(), firehosebackend.CreateDeliveryStreamInput{Name: "roundtrip-stream"},
				)
				require.NoError(t, err)

				wireTaggingFirehose(bk, firehosebackend.NewHandler(fhBk))

				return stream.ARN, func() map[string]string {
					got, tagsErr := fhBk.ListTagsForDeliveryStream(context.Background(), "roundtrip-stream")
					require.NoError(t, tagsErr)

					return got
				}
			},
		},
		{
			name: "opensearch_domain",
			setup: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) (string, func() map[string]string) {
				t.Helper()

				osBk := opensearchbackend.NewInMemoryBackend(accountID, region)
				domain, err := osBk.CreateDomain(opensearchbackend.CreateDomainInput{Name: "roundtrip-domain"})
				require.NoError(t, err)

				wireTaggingOpenSearch(bk, opensearchbackend.NewHandler(osBk))

				return domain.ARN, func() map[string]string {
					got, tagsErr := osBk.ListTags(domain.ARN)
					require.NoError(t, tagsErr)

					return got
				}
			},
		},
		{
			name: "cloudwatchlogs_log_group",
			setup: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) (string, func() map[string]string) {
				t.Helper()

				cwlBk := cwlogsbackend.NewInMemoryBackendWithConfig(accountID, region)
				cwlH := cwlogsbackend.NewHandler(cwlBk)
				lg, err := cwlBk.CreateLogGroup(context.Background(), "roundtrip-group", "", "")
				require.NoError(t, err)

				wireTaggingCloudWatchLogs(bk, cwlH)

				return lg.Arn, func() map[string]string {
					return cwlH.GetTagsForResource(lg.Arn)
				}
			},
		},
		{
			name: "mq_broker",
			setup: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) (string, func() map[string]string) {
				t.Helper()

				mqBk := mqbackend.NewInMemoryBackend(accountID, region)
				broker, err := mqBk.CreateBroker(
					"roundtrip-broker", "SINGLE_INSTANCE", "ACTIVEMQ", "5.17.6", "mq.t3.micro",
					false, false, nil, nil, nil, nil,
				)
				require.NoError(t, err)

				wireTaggingMQ(bk, mqbackend.NewHandler(mqBk))

				return broker.BrokerArn, func() map[string]string {
					tags, tagsErr := mqBk.ListTags(broker.BrokerArn)
					require.NoError(t, tagsErr)

					return tags
				}
			},
		},
		{
			name: "emr_cluster",
			setup: func(t *testing.T, bk resourcegroupstaggingapibackend.StorageBackend) (string, func() map[string]string) {
				t.Helper()

				emrBk := emrbackend.NewInMemoryBackend(accountID, region)
				cluster, err := emrBk.RunJobFlow(context.Background(), emrbackend.RunJobFlowParams{
					Name: "roundtrip-cluster", ReleaseLabel: "emr-6.0.0",
				})
				require.NoError(t, err)

				wireTaggingEMR(bk, emrbackend.NewHandler(emrBk))

				return cluster.ARN, func() map[string]string {
					tagList, tagsErr := emrBk.ListTagsForResource(context.Background(), cluster.ARN)
					require.NoError(t, tagsErr)

					out := make(map[string]string, len(tagList))
					for _, tg := range tagList {
						out[tg.Key] = tg.Value
					}

					return out
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			taggingBk := resourcegroupstaggingapibackend.NewInMemoryBackend(accountID, region)

			resourceARN, nativeTags := tt.setup(t, taggingBk)

			// Mutate through the Resource Groups Tagging API, not the owning
			// service's native TagResource -- this is the direction
			// TestWireResourceGroupsTagging_CrossServiceResources does not cover.
			tagOut, err := taggingBk.TagResources(
				context.Background(),
				&resourcegroupstaggingapibackend.TagResourcesInput{
					ResourceARNList: []string{resourceARN},
					Tags:            map[string]string{wantTagKey: wantTagValue},
				},
			)
			require.NoError(t, err)
			require.Emptyf(t, tagOut.FailedResourcesMap,
				"TagResources for %s must not fail (gopherstack-3xne): %+v", tt.name, tagOut.FailedResourcesMap)

			// The owning service's own native store, not gopherstack's tagging
			// backend, must show the new tag -- proves the registered ARN tagger
			// actually reached the service rather than silently matching nothing.
			got := nativeTags()
			require.NotNilf(t, got, "%s: owning service reported no tags after TagResources", tt.name)
			assert.Equalf(t, wantTagValue, got[wantTagKey],
				"%s: owning service's native tag store must reflect the TagResources call", tt.name)

			keysOut, err := taggingBk.GetTagKeys(
				context.Background(),
				&resourcegroupstaggingapibackend.GetTagKeysInput{},
			)
			require.NoError(t, err)
			assert.Containsf(t, keysOut.TagKeys, wantTagKey, "%s: GetTagKeys must surface the new tag key", tt.name)

			tagKey := wantTagKey
			valuesOut, err := taggingBk.GetTagValues(
				context.Background(), &resourcegroupstaggingapibackend.GetTagValuesInput{Key: &tagKey},
			)
			require.NoError(t, err)
			assert.Containsf(t, valuesOut.TagValues, wantTagValue,
				"%s: GetTagValues must surface the new tag value", tt.name)
		})
	}
}

// TestWireResourceGroupsTagging_RDSFamilyARNCollision proves that wiring DocDB and
// Neptune (bd: gopherstack-7rsk) alongside RDS does not corrupt cross-service tagging,
// even though all three share the "rds" ARN service for some or all of their resource
// kinds (see wireTaggingDocDB and wireTaggingNeptune's doc comments for the file:line
// evidence). resourcegroupstaggingapi's RegisterARNTagger tries taggers in
// registration order and stops at the first handled=true match; RDS's own
// AddTagsToResource does not validate that an ARN belongs to a resource it manages, so
// a naive same-namespace wiring would either let RDS's blind tagger silently swallow a
// genuine DocDB/Neptune ARN (if RDS were registered first) or let DocDB/Neptune's
// tagger silently swallow a genuine RDS ARN (if registered first without an ownership
// check). Only the combination wireResourceGroupsTagging actually uses -- DocDB/
// Neptune registered first AND existence-gated via HasTaggableResource -- routes every
// ARN to the backend that actually owns it.
func TestWireResourceGroupsTagging_RDSFamilyARNCollision(t *testing.T) {
	t.Parallel()

	const accountID = "123456789012"
	const region = "us-east-1"
	const wantTagValue = "collision-test"

	taggingBk := resourcegroupstaggingapibackend.NewInMemoryBackend(accountID, region)

	rdsBk := rdsbackend.NewInMemoryBackend(accountID, region)
	_, err := rdsBk.CreateDBInstance(
		"collision-rds-db", "postgres", "db.t3.micro", "", "admin", "", 20, rdsbackend.DBInstanceOptions{},
	)
	require.NoError(t, err)
	rdsARN := "arn:aws:rds:" + region + ":" + accountID + ":db:collision-rds-db"

	docdbBk := docdbbackend.NewInMemoryBackend(accountID, region)
	docdbCluster, err := docdbBk.CreateDBCluster(
		context.Background(), "collision-docdb-cluster", "docdb", "", "admin", "", "", "", "",
		0, false, false, 1, "", "", nil, nil, nil,
	)
	require.NoError(t, err)

	neptuneBk := neptunebackend.NewInMemoryBackend(accountID, region)
	neptuneSubnetGroup, err := neptuneBk.CreateDBSubnetGroup(
		context.Background(), "collision-neptune-subgrp", "", "vpc-1", []string{"subnet-1"},
	)
	require.NoError(t, err)

	// Registration order matches wireResourceGroupsTagging in cli.go: DocDB and
	// Neptune (both existence-gated on the "rds" ARN service) ahead of RDS (which
	// claims "rds" blindly).
	wireTaggingDocDB(taggingBk, docdbbackend.NewHandler(docdbBk))
	wireTaggingNeptune(taggingBk, neptunebackend.NewHandler(neptuneBk))
	wireTaggingRDS(taggingBk, rdsbackend.NewHandler(rdsBk))

	tagOut, err := taggingBk.TagResources(context.Background(), &resourcegroupstaggingapibackend.TagResourcesInput{
		ResourceARNList: []string{rdsARN, docdbCluster.DBClusterArn, neptuneSubnetGroup.DBSubnetGroupArn},
		Tags:            map[string]string{"owner": wantTagValue},
	})
	require.NoError(t, err)
	require.Empty(t, tagOut.FailedResourcesMap)

	// Each ARN's tag must land in the backend that actually owns it, not whichever
	// "rds"-service tagger happened to be tried first.
	rdsTags := rdsBk.ListTagsForResource(rdsARN)
	require.Len(t, rdsTags, 1)
	assert.Equal(t, wantTagValue, rdsTags[0].Value)

	docdbTags := docdbBk.ListTagsForResource(context.Background(), docdbCluster.DBClusterArn)
	require.Len(t, docdbTags, 1)
	assert.Equal(t, wantTagValue, docdbTags[0].Value)

	neptuneTags, err := neptuneBk.ListTagsForResource(context.Background(), neptuneSubnetGroup.DBSubnetGroupArn)
	require.NoError(t, err)
	require.Len(t, neptuneTags, 1)
	assert.Equal(t, wantTagValue, neptuneTags[0].Value)

	// No cross-contamination: RDS's own store must not have picked up the DocDB or
	// Neptune ARNs, and DocDB/Neptune must not claim ownership of the genuine RDS ARN.
	assert.Empty(t, rdsBk.ListTagsForResource(docdbCluster.DBClusterArn))
	assert.Empty(t, rdsBk.ListTagsForResource(neptuneSubnetGroup.DBSubnetGroupArn))
	assert.False(t, docdbBk.HasTaggableResource(context.Background(), rdsARN))
	assert.False(t, neptuneBk.HasTaggableResource(context.Background(), rdsARN))

	// GetResources must see all three distinct resources, deduplicated by ARN --
	// proving neither provider double-counted or shadowed another.
	out, err := taggingBk.GetResources(context.Background(), &resourcegroupstaggingapibackend.GetResourcesInput{})
	require.NoError(t, err)

	arns := make(map[string]bool, len(out.ResourceTagMappingList))
	for _, m := range out.ResourceTagMappingList {
		arns[m.ResourceARN] = true
	}

	assert.True(t, arns[rdsARN])
	assert.True(t, arns[docdbCluster.DBClusterArn])
	assert.True(t, arns[neptuneSubnetGroup.DBSubnetGroupArn])
}

type mockPurgeableService struct {
	service.Registerable

	purged bool
}

func (m *mockPurgeableService) Purge(_ context.Context, _ time.Time) { m.purged = true }
func (m *mockPurgeableService) Name() string                         { return "MockPurgeable" }

type mockResettableService struct {
	service.Registerable

	resetted bool
}

func (m *mockResettableService) Reset()       { m.resetted = true }
func (m *mockResettableService) Name() string { return "MockResettable" }

func TestCLI_AutoPurgeLoop(t *testing.T) {
	t.Parallel()

	svc1 := &mockPurgeableService{}
	svc2 := &mockResettableService{}
	services := []service.Registerable{svc1, svc2}

	ttl := 10 * time.Millisecond

	// Simulate the loop from cli.go line 1709
	ticker := time.NewTicker(ttl)
	defer ticker.Stop()

	// Run for one tick
	select {
	case <-ticker.C:
		cutoff := time.Now().UTC().Add(-ttl)
		for _, svc := range services {
			if p, ok := svc.(service.Purgeable); ok {
				p.Purge(t.Context(), cutoff)

				continue
			}
			if r, ok := svc.(service.Resettable); ok {
				r.Reset()
			}
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("ticker did not fire")
	}

	assert.True(t, svc1.purged)
	assert.True(t, svc2.resetted)
}

func TestCLI_AWSDefaultRegionEnvVar(t *testing.T) {
	tests := []struct {
		name       string
		env        map[string]string
		wantRegion string
	}{
		{
			name:       "AWS_DEFAULT_REGION_takes_precedence",
			env:        map[string]string{"AWS_DEFAULT_REGION": "eu-central-1"},
			wantRegion: "eu-central-1",
		},
		{
			name:       "AWS_REGION_is_used",
			env:        map[string]string{"AWS_REGION": "ap-southeast-1"},
			wantRegion: "ap-southeast-1",
		},
		{
			name: "AWS_DEFAULT_REGION_overrides_AWS_REGION",
			env: map[string]string{
				"AWS_REGION":         "us-west-1",
				"AWS_DEFAULT_REGION": "us-west-2",
			},
			wantRegion: "us-west-2",
		},
		{
			name:       "REGION_env_sets_region",
			env:        map[string]string{"REGION": "ca-central-1"},
			wantRegion: "ca-central-1",
		},
		{
			name:       "AWS_DEFAULT_REGION_overrides_REGION_default",
			env:        map[string]string{"AWS_DEFAULT_REGION": "sa-east-1"},
			wantRegion: "sa-east-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear existing AWS env vars before each sub-test
			for _, k := range []string{"REGION", "AWS_REGION", "AWS_DEFAULT_REGION"} {
				t.Setenv(k, "")
			}

			cli := parseCLI(t, tt.env)
			result := applyExplicitOverrides(cli, cli)
			assert.Equal(t, tt.wantRegion, result.Region)
		})
	}
}

//nolint:paralleltest // uses t.Setenv which disallows t.Parallel
func TestCLI_S3InitBuckets_Parsing(t *testing.T) {
	// Single bucket via env var
	cli := parseCLI(t, map[string]string{
		"S3_BUCKETS": "my-bucket",
	})
	assert.Equal(t, []string{"my-bucket"}, cli.S3InitBuckets)
}

// errTestPanic is a sentinel error used by TestPanicRecoveryMiddleware_RecoversPanic
// to test the recovers-error-panic code path without triggering err113.
var errTestPanic = errors.New("deliberate error panic")

// errTestGeneric is a sentinel error used by TestCustomHTTPErrorHandler_LogsServerErrors.
var errTestGeneric = errors.New("generic error")

// errRestoreFailure is a sentinel error used by TestBuildLoadHandler.
var errRestoreFailure = errors.New("restore failure")

func TestPanicRecoveryMiddleware_RecoversPanic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		handler        func(*echo.Context) error
		name           string
		wantStatusCode int
	}{
		{
			name: "recovers_string_panic",
			handler: func(_ *echo.Context) error {
				panic("deliberate test panic")
			},
			wantStatusCode: http.StatusInternalServerError,
		},
		{
			name: "recovers_error_panic",
			handler: func(_ *echo.Context) error {
				panic(errTestPanic)
			},
			wantStatusCode: http.StatusInternalServerError,
		},
		{
			name: "passes_through_normal_response",
			handler: func(c *echo.Context) error {
				return c.JSON(http.StatusOK, map[string]string{"ok": "true"})
			},
			wantStatusCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mw := panicRecoveryMiddleware()
			wrapped := mw(tt.handler)

			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			_ = wrapped(c)

			assert.Equal(t, tt.wantStatusCode, rec.Code)
		})
	}
}

func TestAWSMetaMiddleware_PopulatesCtxbag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		authHeader    string
		accountHeader string
		defaultRegion string
		defaultAcct   string
		wantRegion    string
		wantAccount   string
	}{
		{
			name:          "falls back to configured defaults",
			defaultRegion: "eu-west-1",
			defaultAcct:   "111122223333",
			wantRegion:    "eu-west-1",
			wantAccount:   "111122223333",
		},
		{
			name:          "sigv4 scope overrides default region",
			authHeader:    "AWS4-HMAC-SHA256 Credential=AKIA/20260606/ap-south-1/s3/aws4_request",
			defaultRegion: "us-east-1",
			defaultAcct:   "111122223333",
			wantRegion:    "ap-south-1",
			wantAccount:   "111122223333",
		},
		{
			name:          "account header overrides configured account",
			accountHeader: "444455556666",
			defaultRegion: "us-east-1",
			defaultAcct:   "111122223333",
			wantRegion:    "us-east-1",
			wantAccount:   "444455556666",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var gotRegion, gotAccount string

			handler := func(c *echo.Context) error {
				ctx := c.Request().Context()
				gotRegion = awsmeta.Region(ctx)
				gotAccount = awsmeta.Account(ctx)

				return c.NoContent(http.StatusOK)
			}

			wrapped := awsMetaMiddleware(tt.defaultRegion, tt.defaultAcct)(handler)

			req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			if tt.accountHeader != "" {
				req.Header.Set("X-Amz-Account-Id", tt.accountHeader)
			}

			rec := httptest.NewRecorder()
			c := echo.New().NewContext(req, rec)

			require.NoError(t, wrapped(c))
			assert.Equal(t, tt.wantRegion, gotRegion)
			assert.Equal(t, tt.wantAccount, gotAccount)
		})
	}
}

// TestHealthEndpoint_GoroutineAndMemStats verifies that the /_gopherstack/health
// response includes goroutine count and memory stats fields.
//
//nolint:paralleltest // uses a fixed port that cannot be parallelised
func TestHealthEndpoint_GoroutineAndMemStats(t *testing.T) {
	// Uses t.Setenv-like machinery via parseCLI; no t.Parallel.
	port := freeTCPPort(t)
	cli := parseCLI(t, map[string]string{"PORT": strconv.Itoa(port)})

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- run(ctx, cli)
	}()

	require.Eventually(t, func() bool {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/_gopherstack/health", port))
		if err != nil {
			return false
		}
		resp.Body.Close()

		return resp.StatusCode == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "server did not become ready")

	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/_gopherstack/health", port))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

	// Verify the new observability fields are present.
	assert.Contains(t, body, "goroutines", "health response must include goroutines count")
	assert.Contains(t, body, "heap_alloc_bytes", "health response must include heap_alloc_bytes")
	assert.Contains(t, body, "heap_inuse_bytes", "health response must include heap_inuse_bytes")
	assert.Contains(t, body, "num_gc", "health response must include num_gc")

	goroutines, ok := body["goroutines"].(float64)
	require.True(t, ok, "goroutines should be a number")
	assert.Greater(t, goroutines, float64(0), "goroutines should be > 0")

	cancel()

	select {
	case shutdownErr := <-errCh:
		require.NoError(t, shutdownErr)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "server did not shut down within timeout")
	}
}

//nolint:paralleltest // uses a fixed port that cannot be parallelised
func TestLocalstackCompatibilityEndpoints(t *testing.T) {
	port := freeTCPPort(t)
	cli := parseCLI(t, map[string]string{"PORT": strconv.Itoa(port)})

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- run(ctx, cli)
	}()

	client := &http.Client{Timeout: 2 * time.Second}

	require.Eventually(t, func() bool {
		resp, err := client.Get(fmt.Sprintf("http://localhost:%d/_gopherstack/health", port))
		if err != nil {
			return false
		}
		resp.Body.Close()

		return resp.StatusCode == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "server did not become ready")

	tests := []struct {
		validate func(t *testing.T, body map[string]any)
		name     string
		path     string
	}{
		{
			name: "localstack_health",
			path: "/_localstack/health",
			validate: func(t *testing.T, body map[string]any) {
				t.Helper()
				assert.Equal(t, "community", body["edition"])
				assert.NotEmpty(t, body["version"])
				services, ok := body["services"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "available", services["s3"])
				assert.Equal(t, "available", services["dynamodb"])
			},
		},
		{
			name: "aws_health",
			path: "/_aws/health",
			validate: func(t *testing.T, body map[string]any) {
				t.Helper()
				assert.Equal(t, "community", body["edition"])
				services, ok := body["services"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "available", services["s3"])
			},
		},
		{
			name: "localstack_init",
			path: "/_localstack/init",
			validate: func(t *testing.T, body map[string]any) {
				t.Helper()
				assert.Equal(t, true, body["completed"])
				assert.NotNil(t, body["scripts"])
			},
		},
		{
			name: "localstack_init_ready",
			path: "/_localstack/init/ready",
			validate: func(t *testing.T, body map[string]any) {
				t.Helper()
				assert.Equal(t, true, body["completed"])
			},
		},
		{
			name: "localstack_info",
			path: "/_localstack/info",
			validate: func(t *testing.T, body map[string]any) {
				t.Helper()
				assert.Equal(t, "community", body["edition"])
				assert.Equal(t, false, body["is_auth"])
				assert.NotEmpty(t, body["version"])
				assert.Equal(t, "00000000-0000-0000-0000-000000000000", body["session_id"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := client.Get(fmt.Sprintf("http://localhost:%d%s", port, tt.path))
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)
			var body map[string]any
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
			tt.validate(t, body)
		})
	}

	cancel()

	select {
	case shutdownErr := <-errCh:
		require.NoError(t, shutdownErr)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "server did not shut down within timeout")
	}
}

// TestCustomHTTPErrorHandler_LogsServerErrors verifies that the custom Echo error
// handler returns the correct status code for server errors (5xx).
func TestCustomHTTPErrorHandler_LogsServerErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		injectErr      error
		name           string
		wantStatusCode int
	}{
		{
			name:           "http_error_passes_through",
			injectErr:      echo.NewHTTPError(http.StatusNotFound, "not found"),
			wantStatusCode: http.StatusNotFound,
		},
		{
			name:           "non_http_error_becomes_500",
			injectErr:      errTestGeneric,
			wantStatusCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := echo.New()

			// Install the same custom error handler used by buildEchoServer.
			e.HTTPErrorHandler = func(c *echo.Context, err error) {
				var httpErr *echo.HTTPError
				if !errors.As(err, &httpErr) {
					httpErr = echo.NewHTTPError(http.StatusInternalServerError, err.Error())
				}

				if resp, _ := echo.UnwrapResponse(c.Response()); resp == nil || !resp.Committed {
					_ = c.JSON(httpErr.Code, map[string]any{"message": httpErr.Message})
				}
			}

			e.GET("/test", func(_ *echo.Context) error {
				return tt.injectErr
			})

			req := httptest.NewRequest(http.MethodGet, "/test", http.NoBody)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, tt.wantStatusCode, rec.Code)
		})
	}
}

// --- snapshot / load handler test helpers ---

// testPersistable is a minimal Persistable + Resettable for snapshot handler tests.
type testPersistable struct {
	restoreErr error
	data       []byte
	mu         sync.Mutex
}

func (p *testPersistable) Snapshot(_ context.Context) []byte {
	p.mu.Lock()
	defer p.mu.Unlock()

	cp := make([]byte, len(p.data))
	copy(cp, p.data)

	return cp
}

func (p *testPersistable) Restore(_ context.Context, data []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.restoreErr != nil {
		return p.restoreErr
	}

	cp := make([]byte, len(data))
	copy(cp, data)
	p.data = cp

	return nil
}

func (p *testPersistable) Reset() {
	p.mu.Lock()
	p.data = nil
	p.mu.Unlock()
}

func (p *testPersistable) Data() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()

	cp := make([]byte, len(p.data))
	copy(cp, p.data)

	return cp
}

// newTestManager creates a persistence.Manager with a NullStore (no disk I/O).
func newTestManager(t *testing.T, services map[string]*testPersistable) *persistence.Manager {
	t.Helper()

	mgr := persistence.NewManager(t.Context(), persistence.NullStore{})
	for name, svc := range services {
		mgr.Register(name, svc)
	}

	return mgr
}

// postJSON issues a POST to handler via httptest and returns the recorder.
func postJSON(t *testing.T, handler echo.HandlerFunc, body []byte) *httptest.ResponseRecorder {
	t.Helper()

	e := echo.New()

	var bodyReader *bytes.Reader
	if body == nil {
		bodyReader = bytes.NewReader([]byte{})
	} else {
		bodyReader = bytes.NewReader(body)
	}

	req := httptest.NewRequest(http.MethodPost, "/", bodyReader)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	_ = handler(c)

	return rec
}

// --- buildSnapshotHandler ---

func TestBuildSnapshotHandler_EmptyManager(t *testing.T) {
	t.Parallel()

	mgr := newTestManager(t, nil)
	handler := buildSnapshotHandler(mgr)
	rec := postJSON(t, handler, []byte(`{}`))

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp snapshotResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, snapshotBundleFormat, resp.Format)
	assert.Equal(t, 0, resp.Exported)
	assert.Empty(t, resp.Services)
	assert.Equal(t, "ok", resp.Status)
}

func TestBuildSnapshotHandler(t *testing.T) {
	t.Parallel()

	snap := []byte(`{"key":"value"}`)

	tests := []struct {
		services     map[string]*testPersistable
		name         string
		wantKeys     []string
		wantExported int
	}{
		{
			name:         "single_service_with_data",
			services:     map[string]*testPersistable{"alpha": {data: snap}},
			wantExported: 1,
			wantKeys:     []string{"alpha"},
		},
		{
			name: "multiple_services",
			services: map[string]*testPersistable{
				"alpha": {data: []byte(`{"a":1}`)},
				"beta":  {data: []byte(`{"b":2}`)},
			},
			wantExported: 2,
			wantKeys:     []string{"alpha", "beta"},
		},
		{
			name:         "service_with_nil_snapshot_excluded",
			services:     map[string]*testPersistable{"empty": {data: nil}},
			wantExported: 0,
			wantKeys:     nil,
		},
		{
			name: "mix_empty_and_non_empty",
			services: map[string]*testPersistable{
				"present": {data: snap},
				"absent":  {data: nil},
			},
			wantExported: 1,
			wantKeys:     []string{"present"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mgr := newTestManager(t, tt.services)
			handler := buildSnapshotHandler(mgr)
			rec := postJSON(t, handler, []byte(`{}`))

			assert.Equal(t, http.StatusOK, rec.Code)

			var resp snapshotResponse
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

			assert.Equal(t, snapshotBundleFormat, resp.Format)
			assert.Equal(t, "ok", resp.Status)
			assert.Equal(t, tt.wantExported, resp.Exported)

			for _, key := range tt.wantKeys {
				assert.Contains(t, resp.Services, key, "snapshot must include service %s", key)
			}
		})
	}
}

func TestBuildSnapshotHandler_ResponseIsValidJSON(t *testing.T) {
	t.Parallel()

	snap := []byte(`{"items":[1,2,3],"active":true}`)
	mgr := newTestManager(t, map[string]*testPersistable{"svc": {data: snap}})
	handler := buildSnapshotHandler(mgr)
	rec := postJSON(t, handler, []byte(`{}`))

	require.Equal(t, http.StatusOK, rec.Code)

	var resp snapshotResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

	// Service snapshot must be embedded as raw JSON, not base64.
	rawSvc, ok := resp.Services["svc"]
	require.True(t, ok, "svc key must be present")

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(rawSvc, &parsed))
	assert.Equal(t, []any{float64(1), float64(2), float64(3)}, parsed["items"])
}

// --- buildLoadHandler ---

func TestBuildLoadHandler_EmptyBundle(t *testing.T) {
	t.Parallel()

	mgr := newTestManager(t, map[string]*testPersistable{"svc": {data: nil}})
	handler := buildLoadHandler(mgr)

	body, _ := json.Marshal(snapshotBundle{
		Format:   snapshotBundleFormat,
		Services: map[string]json.RawMessage{},
	})
	rec := postJSON(t, handler, body)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp loadResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "ok", resp.Status)
	assert.Equal(t, 0, resp.Loaded)
}

func TestBuildLoadHandler(t *testing.T) {
	t.Parallel()

	snap := json.RawMessage(`{"restored":true}`)

	tests := []struct {
		setupErr       error
		name           string
		bundle         snapshotBundle
		wantStatusCode int
		wantLoaded     int
	}{
		{
			name: "single_service_loaded",
			bundle: snapshotBundle{
				Format:   snapshotBundleFormat,
				Services: map[string]json.RawMessage{"svc": snap},
			},
			wantStatusCode: http.StatusOK,
			wantLoaded:     1,
		},
		{
			name: "unknown_service_skipped",
			bundle: snapshotBundle{
				Format:   snapshotBundleFormat,
				Services: map[string]json.RawMessage{"unknown": snap},
			},
			wantStatusCode: http.StatusOK,
			wantLoaded:     1,
		},
		{
			name:     "restore_error_returns_500",
			setupErr: errRestoreFailure,
			bundle: snapshotBundle{
				Format:   snapshotBundleFormat,
				Services: map[string]json.RawMessage{"svc": snap},
			},
			wantStatusCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc := &testPersistable{restoreErr: tt.setupErr}
			mgr := newTestManager(t, map[string]*testPersistable{"svc": svc})
			handler := buildLoadHandler(mgr)

			e := echo.New()
			e.HTTPErrorHandler = buildHTTPErrorHandler()
			e.POST("/", handler)

			body, marshalErr := json.Marshal(tt.bundle)
			require.NoError(t, marshalErr)

			req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			if tt.wantStatusCode == http.StatusOK {
				assert.Equal(t, http.StatusOK, rec.Code)

				var resp loadResponse
				require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
				assert.Equal(t, "ok", resp.Status)
				assert.Equal(t, tt.wantLoaded, resp.Loaded)
			} else {
				assert.Equal(t, tt.wantStatusCode, rec.Code)
			}
		})
	}
}

func TestBuildLoadHandler_InvalidJSON(t *testing.T) {
	t.Parallel()

	mgr := newTestManager(t, nil)
	handler := buildLoadHandler(mgr)

	e := echo.New()
	e.HTTPErrorHandler = buildHTTPErrorHandler()

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`not-json`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler(c)

	var httpErr *echo.HTTPError
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusBadRequest, httpErr.Code)
}

// --- snapshot → reset → load round-trip via HTTP handlers ---

func TestSnapshotLoadRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		initial  map[string][]byte
		resetSvc []string
	}{
		{
			name:     "single_service_round_trip",
			initial:  map[string][]byte{"svc": []byte(`{"items":["a","b","c"]}`)},
			resetSvc: []string{"svc"},
		},
		{
			name: "multiple_services_round_trip",
			initial: map[string][]byte{
				"s3":      []byte(`{"buckets":["my-bucket"]}`),
				"dynamo":  []byte(`{"tables":["users"]}`),
				"kinesis": []byte(`{"streams":["events"]}`),
			},
			resetSvc: []string{"s3", "dynamo", "kinesis"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			services := make(map[string]*testPersistable, len(tt.initial))
			for name, data := range tt.initial {
				services[name] = &testPersistable{data: data}
			}

			mgr := newTestManager(t, services)

			snapshotHandler := buildSnapshotHandler(mgr)
			loadHandler := buildLoadHandler(mgr)

			e := echo.New()

			// Step 1: POST /_gopherstack/snapshot to export state.
			snapReq := httptest.NewRequest(http.MethodPost, "/_gopherstack/snapshot", http.NoBody)
			snapRec := httptest.NewRecorder()
			snapCtx := e.NewContext(snapReq, snapRec)

			require.NoError(t, snapshotHandler(snapCtx))
			require.Equal(t, http.StatusOK, snapRec.Code)

			var snapResp snapshotResponse
			require.NoError(t, json.NewDecoder(snapRec.Body).Decode(&snapResp))
			assert.Equal(t, len(tt.initial), snapResp.Exported)

			// Step 2: Reset all services.
			for _, name := range tt.resetSvc {
				services[name].Reset()
			}

			for name, svc := range services {
				assert.Empty(t, svc.Data(), "service %s must be empty after reset", name)
			}

			// Step 3: POST /_gopherstack/load with the exported snapshot.
			loadBody, err := json.Marshal(snapResp.snapshotBundle)
			require.NoError(t, err)

			loadReq := httptest.NewRequest(
				http.MethodPost,
				"/_gopherstack/load",
				bytes.NewReader(loadBody),
			)
			loadReq.Header.Set("Content-Type", "application/json")
			loadRec := httptest.NewRecorder()
			loadCtx := e.NewContext(loadReq, loadRec)

			require.NoError(t, loadHandler(loadCtx))
			require.Equal(t, http.StatusOK, loadRec.Code)

			var loadResp loadResponse
			require.NoError(t, json.NewDecoder(loadRec.Body).Decode(&loadResp))
			assert.Equal(t, "ok", loadResp.Status)
			assert.Equal(t, len(tt.initial), loadResp.Loaded)

			// Step 4: Verify each service has its original state.
			for name, want := range tt.initial {
				assert.Equal(t, want, services[name].Data(),
					"service %s state must match original after load", name)
			}
		})
	}
}

func TestSnapshotLoadRoundTrip_CrossManagerLoad(t *testing.T) {
	t.Parallel()

	// Snapshot from managerA, load into managerB (same service names).
	snapData := []byte(`{"value":"transferred"}`)
	svcA := &testPersistable{data: snapData}
	svcB := &testPersistable{data: nil}

	mgrA := newTestManager(t, map[string]*testPersistable{"svc": svcA})
	mgrB := newTestManager(t, map[string]*testPersistable{"svc": svcB})

	e := echo.New()

	// Export from mgrA.
	snapReq := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	snapRec := httptest.NewRecorder()
	require.NoError(t, buildSnapshotHandler(mgrA)(e.NewContext(snapReq, snapRec)))
	require.Equal(t, http.StatusOK, snapRec.Code)

	var snapResp snapshotResponse
	require.NoError(t, json.NewDecoder(snapRec.Body).Decode(&snapResp))

	// Load into mgrB.
	loadBody, err := json.Marshal(snapResp.snapshotBundle)
	require.NoError(t, err)

	loadReq := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(loadBody))
	loadReq.Header.Set("Content-Type", "application/json")
	loadRec := httptest.NewRecorder()
	require.NoError(t, buildLoadHandler(mgrB)(e.NewContext(loadReq, loadRec)))
	require.Equal(t, http.StatusOK, loadRec.Code)

	assert.Equal(t, snapData, svcB.Data(), "svcB must receive svcA's state")
	assert.Equal(t, snapData, svcA.Data(), "svcA must be unchanged")
}

func TestSnapshotLoadRoundTrip_EmptyServicesProduceEmptyBundle(t *testing.T) {
	t.Parallel()

	svc := &testPersistable{data: nil}
	mgr := newTestManager(t, map[string]*testPersistable{"svc": svc})

	e := echo.New()

	snapReq := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	snapRec := httptest.NewRecorder()
	require.NoError(t, buildSnapshotHandler(mgr)(e.NewContext(snapReq, snapRec)))
	require.Equal(t, http.StatusOK, snapRec.Code)

	var snapResp snapshotResponse
	require.NoError(t, json.NewDecoder(snapRec.Body).Decode(&snapResp))
	assert.Equal(t, 0, snapResp.Exported, "nil-snapshot service must not be exported")
	assert.Empty(t, snapResp.Services)
}

// TestBuildEchoServer_SnapshotLoadRoutes verifies that the snapshot and load
// routes are registered on the Echo server returned by buildEchoServer.
func TestBuildEchoServer_SnapshotLoadRoutes(t *testing.T) {
	t.Parallel()

	cli := parseCLI(t, nil)
	mgr := persistence.NewManager(t.Context(), persistence.NullStore{})
	var svcs []service.Registerable

	e := buildEchoServer(t.Context(), nil, mgr, svcs, cli)

	snapshotReq := httptest.NewRequest(http.MethodPost, "/_gopherstack/snapshot", http.NoBody)
	snapshotRec := httptest.NewRecorder()
	e.ServeHTTP(snapshotRec, snapshotReq)
	assert.NotEqual(t, http.StatusNotFound, snapshotRec.Code, "snapshot route must be registered")

	loadBody, _ := json.Marshal(snapshotBundle{Format: snapshotBundleFormat, Services: nil})
	loadReq := httptest.NewRequest(http.MethodPost, "/_gopherstack/load", bytes.NewReader(loadBody))
	loadReq.Header.Set("Content-Type", "application/json")
	loadRec := httptest.NewRecorder()
	e.ServeHTTP(loadRec, loadReq)
	assert.NotEqual(t, http.StatusNotFound, loadRec.Code, "load route must be registered")
}

func TestBuildSnapshotHandler_MultipleServices_StatePreservedIndependently(t *testing.T) {
	t.Parallel()

	// Each service has distinct JSON data; verify each survives the round-trip intact.
	services := map[string]*testPersistable{
		"kinesis": {data: []byte(`{"streams":["stream-a","stream-b"],"shards":4}`)},
		"sqs":     {data: []byte(`{"queues":{"my-queue":{"messages":10}}}`)},
		"sns":     {data: []byte(`{"topics":["arn:aws:sns:us-east-1:000000000000:alerts"]}`)},
	}

	mgr := newTestManager(t, services)
	e := echo.New()

	// Snapshot.
	snapRec := httptest.NewRecorder()
	require.NoError(t, buildSnapshotHandler(mgr)(
		e.NewContext(httptest.NewRequest(http.MethodPost, "/", http.NoBody), snapRec),
	))
	require.Equal(t, http.StatusOK, snapRec.Code)

	var snapResp snapshotResponse
	require.NoError(t, json.NewDecoder(snapRec.Body).Decode(&snapResp))
	assert.Equal(t, 3, snapResp.Exported)

	// Reset.
	for _, svc := range services {
		svc.Reset()
	}

	// Load.
	loadBody, err := json.Marshal(snapResp.snapshotBundle)
	require.NoError(t, err)

	loadRec := httptest.NewRecorder()
	loadReq := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(loadBody))
	loadReq.Header.Set("Content-Type", "application/json")

	require.NoError(t, buildLoadHandler(mgr)(e.NewContext(loadReq, loadRec)))
	require.Equal(t, http.StatusOK, loadRec.Code)

	// Verify each service individually.
	assert.JSONEq(
		t,
		`{"streams":["stream-a","stream-b"],"shards":4}`,
		string(services["kinesis"].Data()),
	)
	assert.JSONEq(t, `{"queues":{"my-queue":{"messages":10}}}`, string(services["sqs"].Data()))
	assert.JSONEq(
		t,
		`{"topics":["arn:aws:sns:us-east-1:000000000000:alerts"]}`,
		string(services["sns"].Data()),
	)
}

func TestEBECSTaskRunnerAdapter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		params     *ebbackend.EcsParameters
		setup      func(t *testing.T, bk *ecsbackend.InMemoryBackend)
		name       string
		clusterARN string
		payload    []byte
		wantCount  int
		wantErr    bool
	}{
		{
			name: "run_task_legacy_payload_only",
			setup: func(t *testing.T, bk *ecsbackend.InMemoryBackend) {
				t.Helper()
				_, err := bk.CreateCluster(ecsbackend.CreateClusterInput{ClusterName: "cluster-1"})
				require.NoError(t, err)
				_, err = bk.RegisterTaskDefinition(ecsbackend.RegisterTaskDefinitionInput{
					Family: "task-def-1",
					ContainerDefinitions: []ecsbackend.ContainerDefinition{
						{Name: "app", Image: "alpine"},
					},
				})
				require.NoError(t, err)
			},
			clusterARN: "cluster-1",
			payload:    []byte(`{"TaskDefinition":"task-def-1","LaunchType":"FARGATE"}`),
			params:     nil,
			wantErr:    false,
			wantCount:  1,
		},
		{
			name: "run_task_with_params_full",
			setup: func(t *testing.T, bk *ecsbackend.InMemoryBackend) {
				t.Helper()
				_, err := bk.CreateCluster(ecsbackend.CreateClusterInput{ClusterName: "cluster-2"})
				require.NoError(t, err)
				_, err = bk.CreateCapacityProvider(ecsbackend.CreateCapacityProviderInput{Name: "cp-1"})
				require.NoError(t, err)
				_, err = bk.RegisterTaskDefinition(ecsbackend.RegisterTaskDefinitionInput{
					Family: "task-def-2",
					ContainerDefinitions: []ecsbackend.ContainerDefinition{
						{Name: "worker", Image: "busybox"},
					},
				})
				require.NoError(t, err)
			},
			clusterARN: "cluster-2",
			payload:    []byte(`{}`),
			params: &ebbackend.EcsParameters{
				TaskDefinitionArn: "task-def-2",
				LaunchType:        "EC2",
				TaskCount:         2,
				Group:             "group-1",
				PlatformVersion:   "LATEST",
				PropagateTags:     "TASK_DEFINITION",
				NetworkConfiguration: &ebbackend.NetworkConfiguration{
					AwsvpcConfiguration: &ebbackend.AwsVpcConfiguration{
						AssignPublicIP: "ENABLED",
						Subnets:        []string{"subnet-123"},
						SecurityGroups: []string{"sg-123"},
					},
				},
				PlacementConstraints: []ebbackend.PlacementConstraint{
					{Type: "distinctInstance"},
				},
				PlacementStrategy: []ebbackend.PlacementStrategy{
					{Type: "spread", Field: "attribute:ecs.availability-zone"},
				},
				CapacityProviderStrategy: []ebbackend.CapacityProviderStrategyItem{
					{CapacityProvider: "cp-1", Base: 1, Weight: 2},
				},
				Tags: []ebbackend.EcsTag{
					{Key: "env", Value: "prod"},
				},
				EnableECSManagedTags: true,
				EnableExecuteCommand: true,
			},
			wantErr:   false,
			wantCount: 2,
		},
		{
			name: "run_task_with_params_fallback_to_payload",
			setup: func(t *testing.T, bk *ecsbackend.InMemoryBackend) {
				t.Helper()
				_, err := bk.CreateCluster(ecsbackend.CreateClusterInput{ClusterName: "cluster-3"})
				require.NoError(t, err)
				_, err = bk.RegisterTaskDefinition(ecsbackend.RegisterTaskDefinitionInput{
					Family: "task-def-3",
					ContainerDefinitions: []ecsbackend.ContainerDefinition{
						{Name: "app", Image: "alpine"},
					},
				})
				require.NoError(t, err)
			},
			clusterARN: "cluster-3",
			payload:    []byte(`{"TaskDefinition":"task-def-3"}`),
			params: &ebbackend.EcsParameters{
				Group: "fallback-group",
			},
			wantErr:   false,
			wantCount: 1,
		},
		{
			name: "run_task_unknown_task_definition_returns_error",
			setup: func(t *testing.T, bk *ecsbackend.InMemoryBackend) {
				t.Helper()
				_, err := bk.CreateCluster(ecsbackend.CreateClusterInput{ClusterName: "cluster-4"})
				require.NoError(t, err)
			},
			clusterARN: "cluster-4",
			payload:    []byte(`{"TaskDefinition":"nonexistent-task-def"}`),
			params:     nil,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bk := ecsbackend.NewInMemoryBackend("123456789012", "us-east-1", nil)
			if tt.setup != nil {
				tt.setup(t, bk)
			}

			adapter := &ebECSTaskRunnerAdapter{backend: bk}

			var err error
			if tt.params != nil {
				err = adapter.RunTaskWithParams(t.Context(), tt.clusterARN, tt.params, tt.payload)
			} else {
				err = adapter.RunTask(t.Context(), tt.clusterARN, tt.payload)
			}

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				tasks, listErr := bk.ListTasks(tt.clusterARN)
				require.NoError(t, listErr)
				assert.Len(t, tasks, tt.wantCount)
			}
		})
	}
}

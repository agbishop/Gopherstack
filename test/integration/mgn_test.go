package integration_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	ec2sdk "github.com/aws/aws-sdk-go-v2/service/ec2"
	mgnsdk "github.com/aws/aws-sdk-go-v2/service/mgn"
	mgntypes "github.com/aws/aws-sdk-go-v2/service/mgn/types"
	organizationsSDK "github.com/aws/aws-sdk-go-v2/service/organizations"
	organizationstypes "github.com/aws/aws-sdk-go-v2/service/organizations/types"
	s3sdk "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mgnAsyncWait/mgnAsyncPoll bound require.Eventually calls against MGN's
// asyncTransitionDelay-ticked state machines (jobs.go/sourceservers.go:
// 100ms/tick, up to 4 ticks) -- generous for CI/Docker jitter, matching this
// package's own convention (e.g. directconnect_test.go).
const (
	mgnAsyncWait = 10 * time.Second
	mgnAsyncPoll = 100 * time.Millisecond
)

// createMGNClient returns an MGN client pointed at the shared test container.
func createMGNClient(t *testing.T) *mgnsdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return mgnsdk.NewFromConfig(cfg, func(o *mgnsdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// mgnCleanupCtx returns a context for use inside t.Cleanup callbacks.
// t.Context() must not be used there: Go 1.24+ cancels it before cleanups run.
func mgnCleanupCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}

// initializeMGNAccount calls InitializeService, required before any of MGN's
// 69 legacy ops (PARITY.md). Idempotent, safe to call from every test.
func initializeMGNAccount(t *testing.T, client *mgnsdk.Client) {
	t.Helper()

	_, err := client.InitializeService(t.Context(), &mgnsdk.InitializeServiceInput{})
	require.NoError(t, err, "InitializeService should succeed")
}

// importMGNSourceServer drives the real, wire-reachable StartImport path end
// to end: a real S3 bucket/object via s3Client, StartImport reading it
// through the actual wireMGNS3 cross-service binding this server wires at
// startup (not a mock, unlike the unit round-trip tests), and polling until
// exactly the row identified by userProvidedID appears via DescribeSourceServers.
func importMGNSourceServer(
	t *testing.T, client *mgnsdk.Client, s3Client *s3sdk.Client, bucket, userProvidedID, hostname string,
) mgntypes.SourceServer {
	t.Helper()

	ctx := t.Context()
	key := userProvidedID + ".csv"

	_, err := s3Client.CreateBucket(ctx, &s3sdk.CreateBucketInput{Bucket: aws.String(bucket)})
	require.NoError(t, err, "CreateBucket should succeed")

	_, err = s3Client.PutObject(ctx, &s3sdk.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body: strings.NewReader(
			"mgn:server:hostname,mgn:server:user-provided-id\n" + hostname + "," + userProvidedID + "\n",
		),
	})
	require.NoError(t, err, "PutObject should succeed")

	started, err := client.StartImport(ctx, &mgnsdk.StartImportInput{
		S3BucketSource: &mgntypes.S3BucketSource{S3Bucket: aws.String(bucket), S3Key: aws.String(key)},
	})
	require.NoError(t, err, "StartImport should succeed")
	importID := aws.ToString(started.ImportTask.ImportID)

	require.Eventually(t, func() bool {
		out, listErr := client.ListImports(ctx, &mgnsdk.ListImportsInput{
			Filters: &mgntypes.ListImportsRequestFilters{ImportIDs: []string{importID}},
		})

		return listErr == nil && len(out.Items) == 1 && out.Items[0].Status == mgntypes.ImportStatusSucceeded
	}, mgnAsyncWait, mgnAsyncPoll, "import task never reached SUCCEEDED")

	var found mgntypes.SourceServer

	require.Eventually(t, func() bool {
		out, describeErr := client.DescribeSourceServers(ctx, &mgnsdk.DescribeSourceServersInput{})
		if describeErr != nil {
			return false
		}

		for _, s := range out.Items {
			if aws.ToString(s.UserProvidedID) == userProvidedID {
				found = s

				return true
			}
		}

		return false
	}, mgnAsyncWait, mgnAsyncPoll, "imported source server never appeared")

	return found
}

// TestIntegration_MGN_SourceServerLifecycle drives the real StartImport ->
// DescribeSourceServers -> UpdateSourceServer -> ChangeServerLifeCycleState ->
// DisconnectFromService -> DeleteSourceServer chain -- a genuinely sequential
// resource lifecycle, each step consuming the previous step's state.
func TestIntegration_MGN_SourceServerLifecycle(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	ctx := t.Context()
	client := createMGNClient(t)
	s3Client := createS3Client(t)

	initializeMGNAccount(t, client)

	seeded := importMGNSourceServer(
		t, client, s3Client, "mgn-lifecycle-bucket", "lifecycle-server", "web-1.example.com",
	)
	sourceServerID := aws.ToString(seeded.SourceServerID)

	t.Cleanup(func() {
		cctx, cancel := mgnCleanupCtx()
		defer cancel()
		_, _ = client.DeleteSourceServer(
			cctx,
			&mgnsdk.DeleteSourceServerInput{SourceServerID: aws.String(sourceServerID)},
		)
	})

	require.NotEmpty(t, aws.ToString(seeded.Arn), "SourceServer ARN must be returned")
	assert.Equal(t, "lifecycle-server", aws.ToString(seeded.UserProvidedID))

	updated, err := client.UpdateSourceServer(ctx, &mgnsdk.UpdateSourceServerInput{
		SourceServerID:         aws.String(sourceServerID),
		FqdnForActionFramework: aws.String("web-1.action.example.com"),
	})
	require.NoError(t, err, "UpdateSourceServer should succeed")
	assert.Equal(t, "web-1.action.example.com", aws.ToString(updated.FqdnForActionFramework))

	// ChangeServerLifeCycleState requires CONTINUOUS replication (its
	// documented launchable precondition); this test verifies the sequential
	// resource chain, not whether the call works pre-replication, so wait for
	// it legitimately rather than changing what's being proven.
	require.Eventually(t, func() bool {
		out, describeErr := client.DescribeSourceServers(ctx, &mgnsdk.DescribeSourceServersInput{
			Filters: &mgntypes.DescribeSourceServersRequestFilters{SourceServerIDs: []string{sourceServerID}},
		})

		return describeErr == nil && len(out.Items) == 1 &&
			out.Items[0].DataReplicationInfo != nil &&
			out.Items[0].DataReplicationInfo.DataReplicationState == mgntypes.DataReplicationStateContinuous
	}, mgnAsyncWait, mgnAsyncPoll, "source server never reached CONTINUOUS")

	changed, err := client.ChangeServerLifeCycleState(ctx, &mgnsdk.ChangeServerLifeCycleStateInput{
		SourceServerID: aws.String(sourceServerID),
		LifeCycle: &mgntypes.ChangeServerLifeCycleStateSourceServerLifecycle{
			State: mgntypes.ChangeServerLifeCycleStateSourceServerLifecycleStateReadyForTest,
		},
	})
	require.NoError(t, err, "ChangeServerLifeCycleState should succeed")
	require.NotNil(t, changed.LifeCycle)
	assert.Equal(t, mgntypes.LifeCycleStateReadyForTest, changed.LifeCycle.State)

	_, err = client.DisconnectFromService(ctx, &mgnsdk.DisconnectFromServiceInput{
		SourceServerID: aws.String(sourceServerID),
	})
	require.NoError(t, err, "DisconnectFromService should succeed")

	_, err = client.DeleteSourceServer(ctx, &mgnsdk.DeleteSourceServerInput{SourceServerID: aws.String(sourceServerID)})
	require.NoError(t, err, "DeleteSourceServer should succeed")

	_, err = client.GetLaunchConfiguration(ctx, &mgnsdk.GetLaunchConfigurationInput{
		SourceServerID: aws.String(sourceServerID),
	})
	require.Error(t, err, "GetLaunchConfiguration should 404 after delete")
	assert.Equal(t, "ResourceNotFoundException", awsErrorCode(err))
}

// TestIntegration_MGN_ConfigurationTemplateLifecycle tables
// Create -> Describe -> Update -> Delete across the two template kinds
// (LaunchConfigurationTemplate/ReplicationConfigurationTemplate): same CRUD
// shape, different resource, exactly what a table is for -- see
// TestIntegration_MGN_Tagging for the same pattern applied to tag targets.
func TestIntegration_MGN_ConfigurationTemplateLifecycle(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createMGNClient(t)
	initializeMGNAccount(t, client)

	tests := []struct {
		create        func(ctx context.Context) string
		describeCheck func(t *testing.T, ctx context.Context, templateID string)
		update        func(t *testing.T, ctx context.Context, templateID string)
		delete        func(ctx context.Context, templateID string) error
		name          string
	}{
		{
			name: "launch configuration template",
			create: func(ctx context.Context) string {
				out, err := client.CreateLaunchConfigurationTemplate(
					ctx, &mgnsdk.CreateLaunchConfigurationTemplateInput{
						BootMode: mgntypes.BootModeUseSource, LaunchDisposition: mgntypes.LaunchDispositionStarted,
					},
				)
				require.NoError(t, err, "CreateLaunchConfigurationTemplate should succeed")

				return aws.ToString(out.LaunchConfigurationTemplateID)
			},
			describeCheck: func(t *testing.T, ctx context.Context, templateID string) {
				t.Helper()

				out, err := client.DescribeLaunchConfigurationTemplates(
					ctx, &mgnsdk.DescribeLaunchConfigurationTemplatesInput{
						LaunchConfigurationTemplateIDs: []string{templateID},
					},
				)
				require.NoError(t, err, "DescribeLaunchConfigurationTemplates should succeed")
				require.Len(t, out.Items, 1)
				assert.Equal(t, mgntypes.BootModeUseSource, out.Items[0].BootMode)
			},
			update: func(t *testing.T, ctx context.Context, templateID string) {
				t.Helper()

				out, err := client.UpdateLaunchConfigurationTemplate(
					ctx, &mgnsdk.UpdateLaunchConfigurationTemplateInput{
						LaunchConfigurationTemplateID: aws.String(templateID), BootMode: mgntypes.BootModeUefi,
					},
				)
				require.NoError(t, err, "UpdateLaunchConfigurationTemplate should succeed")
				assert.Equal(t, mgntypes.BootModeUefi, out.BootMode)
			},
			delete: func(ctx context.Context, templateID string) error {
				_, err := client.DeleteLaunchConfigurationTemplate(
					ctx, &mgnsdk.DeleteLaunchConfigurationTemplateInput{
						LaunchConfigurationTemplateID: aws.String(templateID),
					},
				)

				return err
			},
		},
		{
			name: "replication configuration template",
			create: func(ctx context.Context) string {
				out, err := client.CreateReplicationConfigurationTemplate(
					ctx, &mgnsdk.CreateReplicationConfigurationTemplateInput{
						AssociateDefaultSecurityGroup:       aws.Bool(true),
						BandwidthThrottling:                 100,
						CreatePublicIP:                      aws.Bool(false),
						DataPlaneRouting:                    mgntypes.ReplicationConfigurationDataPlaneRoutingPrivateIp,
						DefaultLargeStagingDiskType:         mgntypes.ReplicationConfigurationDefaultLargeStagingDiskTypeGp3,
						EbsEncryption:                       mgntypes.ReplicationConfigurationEbsEncryptionDefault,
						ReplicationServerInstanceType:       aws.String("t3.small"),
						ReplicationServersSecurityGroupsIDs: []string{"sg-integ-test"},
						StagingAreaSubnetId:                 aws.String("subnet-integ-test"),
						StagingAreaTags:                     map[string]string{},
						UseDedicatedReplicationServer:       aws.Bool(false),
					},
				)
				require.NoError(t, err, "CreateReplicationConfigurationTemplate should succeed")

				return aws.ToString(out.ReplicationConfigurationTemplateID)
			},
			describeCheck: func(t *testing.T, ctx context.Context, templateID string) {
				t.Helper()

				out, err := client.DescribeReplicationConfigurationTemplates(
					ctx, &mgnsdk.DescribeReplicationConfigurationTemplatesInput{
						ReplicationConfigurationTemplateIDs: []string{templateID},
					},
				)
				require.NoError(t, err, "DescribeReplicationConfigurationTemplates should succeed")
				require.Len(t, out.Items, 1)
				assert.Equal(t, "t3.small", aws.ToString(out.Items[0].ReplicationServerInstanceType))
			},
			update: func(t *testing.T, ctx context.Context, templateID string) {
				t.Helper()

				out, err := client.UpdateReplicationConfigurationTemplate(
					ctx, &mgnsdk.UpdateReplicationConfigurationTemplateInput{
						ReplicationConfigurationTemplateID: aws.String(templateID),
						ReplicationServerInstanceType:      aws.String("t3.medium"),
					},
				)
				require.NoError(t, err, "UpdateReplicationConfigurationTemplate should succeed")
				assert.Equal(t, "t3.medium", aws.ToString(out.ReplicationServerInstanceType))
			},
			delete: func(ctx context.Context, templateID string) error {
				_, err := client.DeleteReplicationConfigurationTemplate(
					ctx, &mgnsdk.DeleteReplicationConfigurationTemplateInput{
						ReplicationConfigurationTemplateID: aws.String(templateID),
					},
				)

				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			templateID := tt.create(ctx)
			require.NotEmpty(t, templateID)

			t.Cleanup(func() {
				cctx, cancel := mgnCleanupCtx()
				defer cancel()
				_ = tt.delete(cctx, templateID)
			})

			tt.describeCheck(t, ctx, templateID)
			tt.update(t, ctx, templateID)

			require.NoError(t, tt.delete(ctx, templateID), "delete should succeed")
		})
	}
}

// TestIntegration_MGN_JobLifecycle drives StartTest through to a COMPLETED
// Job and confirms the highest-value cross-service fix this pass made: the
// participant's LaunchedEc2InstanceID is a REAL services/ec2 instance (found
// via a real ec2 DescribeInstances call), not a synthetic, non-cross-checked
// ID -- see services/mgn/cross_service.go.
func TestIntegration_MGN_JobLifecycle(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	ctx := t.Context()
	client := createMGNClient(t)
	s3Client := createS3Client(t)
	ec2Client := createEC2ClientAt(t, endpoint)

	initializeMGNAccount(t, client)

	seeded := importMGNSourceServer(t, client, s3Client, "mgn-job-bucket", "job-server", "job-1.example.com")
	sourceServerID := aws.ToString(seeded.SourceServerID)

	t.Cleanup(func() {
		cctx, cancel := mgnCleanupCtx()
		defer cancel()
		_, _ = client.DeleteSourceServer(
			cctx,
			&mgnsdk.DeleteSourceServerInput{SourceServerID: aws.String(sourceServerID)},
		)
	})

	require.Eventually(t, func() bool {
		out, describeErr := client.DescribeSourceServers(ctx, &mgnsdk.DescribeSourceServersInput{
			Filters: &mgntypes.DescribeSourceServersRequestFilters{SourceServerIDs: []string{sourceServerID}},
		})

		return describeErr == nil && len(out.Items) == 1 &&
			out.Items[0].LifeCycle != nil && out.Items[0].LifeCycle.State == mgntypes.LifeCycleStateReadyForTest
	}, mgnAsyncWait, mgnAsyncPoll, "source server never reached READY_FOR_TEST")

	started, err := client.StartTest(ctx, &mgnsdk.StartTestInput{SourceServerIDs: []string{sourceServerID}})
	require.NoError(t, err, "StartTest should succeed")
	require.Len(t, started.Job.ParticipatingServers, 1)
	jobID := aws.ToString(started.Job.JobID)

	var completedJob mgntypes.Job

	require.Eventually(t, func() bool {
		out, listErr := client.DescribeJobs(ctx, &mgnsdk.DescribeJobsInput{
			Filters: &mgntypes.DescribeJobsRequestFilters{JobIDs: []string{jobID}},
		})
		if listErr != nil || len(out.Items) != 1 || out.Items[0].Status != mgntypes.JobStatusCompleted {
			return false
		}

		completedJob = out.Items[0]

		return true
	}, mgnAsyncWait, mgnAsyncPoll, "job never reached COMPLETED")

	require.Len(t, completedJob.ParticipatingServers, 1)
	instanceID := aws.ToString(completedJob.ParticipatingServers[0].LaunchedEc2InstanceID)
	require.NotEmpty(t, instanceID, "job must record a launched instance ID")

	logItems, err := client.DescribeJobLogItems(ctx, &mgnsdk.DescribeJobLogItemsInput{JobID: aws.String(jobID)})
	require.NoError(t, err, "DescribeJobLogItems should succeed")
	assert.NotEmpty(t, logItems.Items, "job should have recorded log events")

	descOut, err := ec2Client.DescribeInstances(ctx, &ec2sdk.DescribeInstancesInput{InstanceIds: []string{instanceID}})
	require.NoError(t, err, "the launched instance ID must resolve to a real services/ec2 instance")
	require.Len(t, descOut.Reservations, 1)
	require.Len(t, descOut.Reservations[0].Instances, 1)
	assert.Equal(t, instanceID, aws.ToString(descOut.Reservations[0].Instances[0].InstanceId))

	_, err = client.TerminateTargetInstances(ctx, &mgnsdk.TerminateTargetInstancesInput{
		SourceServerIDs: []string{sourceServerID},
	})
	require.NoError(t, err, "TerminateTargetInstances should succeed")

	require.Eventually(t, func() bool {
		out, describeErr := client.DescribeSourceServers(ctx, &mgnsdk.DescribeSourceServersInput{
			Filters: &mgntypes.DescribeSourceServersRequestFilters{SourceServerIDs: []string{sourceServerID}},
		})

		return describeErr == nil && len(out.Items) == 1 && out.Items[0].LaunchedInstance == nil
	}, mgnAsyncWait, mgnAsyncPoll, "LaunchedInstance was never cleared after TerminateTargetInstances")
}

// TestIntegration_MGN_ApplicationsAndWaves drives
// CreateApplication/CreateWave -> AssociateApplications -> DisassociateApplications
// -> DeleteWave/DeleteApplication -- a genuinely sequential association
// lifecycle (disassociate must precede delete, per services/mgn/waves.go's
// waveHasApplicationsLocked guard).
func TestIntegration_MGN_ApplicationsAndWaves(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	ctx := t.Context()
	client := createMGNClient(t)
	initializeMGNAccount(t, client)

	app, err := client.CreateApplication(ctx, &mgnsdk.CreateApplicationInput{Name: aws.String("integ-app")})
	require.NoError(t, err, "CreateApplication should succeed")
	applicationID := aws.ToString(app.ApplicationID)

	t.Cleanup(func() {
		cctx, cancel := mgnCleanupCtx()
		defer cancel()
		_, _ = client.DeleteApplication(cctx, &mgnsdk.DeleteApplicationInput{ApplicationID: aws.String(applicationID)})
	})

	wave, err := client.CreateWave(ctx, &mgnsdk.CreateWaveInput{Name: aws.String("integ-wave")})
	require.NoError(t, err, "CreateWave should succeed")
	waveID := aws.ToString(wave.WaveID)

	t.Cleanup(func() {
		cctx, cancel := mgnCleanupCtx()
		defer cancel()
		_, _ = client.DeleteWave(cctx, &mgnsdk.DeleteWaveInput{WaveID: aws.String(waveID)})
	})

	_, err = client.AssociateApplications(ctx, &mgnsdk.AssociateApplicationsInput{
		WaveID: aws.String(waveID), ApplicationIDs: []string{applicationID},
	})
	require.NoError(t, err, "AssociateApplications should succeed")

	listed, err := client.ListApplications(ctx, &mgnsdk.ListApplicationsInput{
		Filters: &mgntypes.ListApplicationsRequestFilters{WaveIDs: []string{waveID}},
	})
	require.NoError(t, err, "ListApplications should succeed")
	require.Len(t, listed.Items, 1)
	assert.Equal(t, applicationID, aws.ToString(listed.Items[0].ApplicationID))

	_, err = client.DisassociateApplications(ctx, &mgnsdk.DisassociateApplicationsInput{
		WaveID: aws.String(waveID), ApplicationIDs: []string{applicationID},
	})
	require.NoError(t, err, "DisassociateApplications should succeed")

	_, err = client.DeleteWave(ctx, &mgnsdk.DeleteWaveInput{WaveID: aws.String(waveID)})
	require.NoError(t, err, "DeleteWave should succeed once disassociated")

	_, err = client.DeleteApplication(ctx, &mgnsdk.DeleteApplicationInput{ApplicationID: aws.String(applicationID)})
	require.NoError(t, err, "DeleteApplication should succeed")
}

// TestIntegration_MGN_Tagging tables TagResource/ListTagsForResource/
// UntagResource across several of the 12 taggable resource kinds
// (services/mgn/tagging.go) -- each case independently creates its own
// resource, so cases are fully parallel-safe.
func TestIntegration_MGN_Tagging(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createMGNClient(t)
	s3Client := createS3Client(t)
	initializeMGNAccount(t, client)

	tests := []struct {
		createARN func(t *testing.T, ctx context.Context) string
		name      string
	}{
		{
			name: "source server",
			createARN: func(t *testing.T, _ context.Context) string {
				t.Helper()

				s := importMGNSourceServer(t, client, s3Client, "mgn-tag-bucket", "tag-server", "tag-1.example.com")

				return aws.ToString(s.Arn)
			},
		},
		{
			name: "application",
			createARN: func(t *testing.T, ctx context.Context) string {
				t.Helper()

				out, err := client.CreateApplication(ctx, &mgnsdk.CreateApplicationInput{Name: aws.String("tag-app")})
				require.NoError(t, err)

				return aws.ToString(out.Arn)
			},
		},
		{
			name: "wave",
			createARN: func(t *testing.T, ctx context.Context) string {
				t.Helper()

				out, err := client.CreateWave(ctx, &mgnsdk.CreateWaveInput{Name: aws.String("tag-wave")})
				require.NoError(t, err)

				return aws.ToString(out.Arn)
			},
		},
		{
			name: "launch configuration template",
			createARN: func(t *testing.T, ctx context.Context) string {
				t.Helper()

				out, err := client.CreateLaunchConfigurationTemplate(
					ctx, &mgnsdk.CreateLaunchConfigurationTemplateInput{BootMode: mgntypes.BootModeUseSource},
				)
				require.NoError(t, err)

				return aws.ToString(out.Arn)
			},
		},
		{
			name: "connector",
			createARN: func(t *testing.T, ctx context.Context) string {
				t.Helper()

				out, err := client.CreateConnector(ctx, &mgnsdk.CreateConnectorInput{
					Name: aws.String("tag-connector"), SsmInstanceID: aws.String("mi-0123456789abcdef0"),
				})
				require.NoError(t, err)

				return aws.ToString(out.Arn)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			resourceARN := tt.createARN(t, ctx)
			require.NotEmpty(t, resourceARN)

			_, err := client.TagResource(ctx, &mgnsdk.TagResourceInput{
				ResourceArn: aws.String(resourceARN), Tags: map[string]string{"env": "integ"},
			})
			require.NoError(t, err, "TagResource should succeed")

			listed, err := client.ListTagsForResource(ctx, &mgnsdk.ListTagsForResourceInput{
				ResourceArn: aws.String(resourceARN),
			})
			require.NoError(t, err, "ListTagsForResource should succeed")
			assert.Equal(t, "integ", listed.Tags["env"])

			_, err = client.UntagResource(ctx, &mgnsdk.UntagResourceInput{
				ResourceArn: aws.String(resourceARN), TagKeys: []string{"env"},
			})
			require.NoError(t, err, "UntagResource should succeed")

			afterUntag, err := client.ListTagsForResource(ctx, &mgnsdk.ListTagsForResourceInput{
				ResourceArn: aws.String(resourceARN),
			})
			require.NoError(t, err)
			assert.NotContains(t, afterUntag.Tags, "env")
		})
	}
}

// TestIntegration_MGN_NotFoundErrors tables ops against an unknown resource
// ID, confirming each returns a real ResourceNotFoundException wire code.
func TestIntegration_MGN_NotFoundErrors(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createMGNClient(t)
	initializeMGNAccount(t, client)

	tests := []struct {
		call func(ctx context.Context) error
		name string
	}{
		{
			name: "GetLaunchConfiguration unknown source server",
			call: func(ctx context.Context) error {
				_, err := client.GetLaunchConfiguration(ctx, &mgnsdk.GetLaunchConfigurationInput{
					SourceServerID: aws.String("s-unknown"),
				})

				return err
			},
		},
		{
			name: "DeleteApplication unknown application",
			call: func(ctx context.Context) error {
				_, err := client.DeleteApplication(ctx, &mgnsdk.DeleteApplicationInput{
					ApplicationID: aws.String("app-unknown"),
				})

				return err
			},
		},
		{
			name: "DeleteWave unknown wave",
			call: func(ctx context.Context) error {
				_, err := client.DeleteWave(ctx, &mgnsdk.DeleteWaveInput{WaveID: aws.String("wave-unknown")})

				return err
			},
		},
		{
			name: "DeleteConnector unknown connector",
			call: func(ctx context.Context) error {
				_, err := client.DeleteConnector(
					ctx,
					&mgnsdk.DeleteConnectorInput{ConnectorID: aws.String("conn-unknown")},
				)

				return err
			},
		},
		{
			name: "DeleteLaunchConfigurationTemplate unknown template",
			call: func(ctx context.Context) error {
				_, err := client.DeleteLaunchConfigurationTemplate(ctx, &mgnsdk.DeleteLaunchConfigurationTemplateInput{
					LaunchConfigurationTemplateID: aws.String("lct-unknown"),
				})

				return err
			},
		},
		{
			name: "GetNetworkMigrationDefinition unknown definition",
			call: func(ctx context.Context) error {
				_, err := client.GetNetworkMigrationDefinition(ctx, &mgnsdk.GetNetworkMigrationDefinitionInput{
					NetworkMigrationDefinitionID: aws.String("nmd-unknown"),
				})

				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.call(t.Context())
			require.Error(t, err)
			assert.Equal(t, "ResourceNotFoundException", awsErrorCode(err))
		})
	}
}

// TestIntegration_MGN_ValidationErrors tables required-field-missing
// requests, confirming each returns a real ValidationException wire code.
func TestIntegration_MGN_ValidationErrors(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createMGNClient(t)
	initializeMGNAccount(t, client)

	tests := []struct {
		call func(ctx context.Context) error
		name string
	}{
		{
			// S3Bucket/S3Key are client-side "required" (non-nil) per the SDK's
			// own validators.go, so a nil field never reaches the server --
			// an empty string does reach it and is this backend's own
			// server-side validation (exportimport.go's StartImport).
			name: "StartImport empty s3Bucket",
			call: func(ctx context.Context) error {
				_, err := client.StartImport(ctx, &mgnsdk.StartImportInput{
					S3BucketSource: &mgntypes.S3BucketSource{S3Bucket: aws.String(""), S3Key: aws.String("k")},
				})

				return err
			},
		},
		{
			name: "UpdateSourceServerReplicationType invalid type",
			call: func(ctx context.Context) error {
				_, err := client.UpdateSourceServerReplicationType(ctx, &mgnsdk.UpdateSourceServerReplicationTypeInput{
					SourceServerID: aws.String("s-1"), ReplicationType: "BOGUS",
				})

				return err
			},
		},
		{
			name: "StartTest empty source server list",
			call: func(ctx context.Context) error {
				_, err := client.StartTest(ctx, &mgnsdk.StartTestInput{SourceServerIDs: []string{}})

				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.call(t.Context())
			require.Error(t, err)
			assert.Equal(t, "ValidationException", awsErrorCode(err))
		})
	}
}

// TestIntegration_MGN_NetworkMigration drives CreateNetworkMigrationDefinition
// -> StartNetworkMigrationMapping (auto-vivifying a NetworkMigrationExecution,
// since no op in this 95-op surface creates one explicitly -- see
// services/mgn/networkmigrationjobs.go) -> ListNetworkMigrationExecutions,
// and confirms the mapper-segment family's documented structural gap: no
// network-analysis engine exists, so it honestly 404s rather than fabricating
// a segment.
func TestIntegration_MGN_NetworkMigration(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	ctx := t.Context()
	client := createMGNClient(t)
	initializeMGNAccount(t, client)

	def, err := client.CreateNetworkMigrationDefinition(ctx, &mgnsdk.CreateNetworkMigrationDefinitionInput{
		Name:          aws.String("integ-nm-def"),
		TargetNetwork: &mgntypes.TargetNetwork{Topology: mgntypes.TargetNetworkTopologyIsolatedVpc},
		TargetS3Configuration: &mgntypes.TargetS3Configuration{
			S3Bucket: aws.String("nm-bucket"), S3BucketOwner: aws.String("000000000000"),
		},
	})
	require.NoError(t, err, "CreateNetworkMigrationDefinition should succeed")
	definitionID := aws.ToString(def.NetworkMigrationDefinitionID)

	executionID := "exec-integ-1"

	_, err = client.StartNetworkMigrationMapping(ctx, &mgnsdk.StartNetworkMigrationMappingInput{
		NetworkMigrationDefinitionID: aws.String(definitionID),
		NetworkMigrationExecutionID:  aws.String(executionID),
	})
	require.NoError(t, err, "StartNetworkMigrationMapping should succeed")

	listed, err := client.ListNetworkMigrationExecutions(ctx, &mgnsdk.ListNetworkMigrationExecutionsInput{
		NetworkMigrationDefinitionID: aws.String(definitionID),
	})
	require.NoError(t, err, "ListNetworkMigrationExecutions should succeed")
	require.Len(t, listed.Items, 1)
	assert.Equal(t, executionID, aws.ToString(listed.Items[0].NetworkMigrationExecutionID))

	_, err = client.GetNetworkMigrationMapperSegmentConstruct(
		ctx,
		&mgnsdk.GetNetworkMigrationMapperSegmentConstructInput{
			NetworkMigrationDefinitionID: aws.String(definitionID),
			NetworkMigrationExecutionID:  aws.String(executionID),
			SegmentID:                    aws.String("segment-unknown"),
			ConstructID:                  aws.String("construct-unknown"),
		},
	)
	require.Error(t, err, "no analysis engine ever produces a segment construct to return")
	assert.Equal(t, "ResourceNotFoundException", awsErrorCode(err))
}

// TestIntegration_MGN_ListManagedAccounts confirms ListManagedAccounts'
// cross-service Organizations wiring (services/mgn/cross_service.go): once
// this account is an AWS Organizations management account, a real member
// account it creates shows up in MGN's own ListManagedAccounts, not just the
// caller's own account. The organization is shared, account-wide state (like
// test/integration/organizations_test.go's own ensureOrg helper), so
// CreateOrganization here tolerates AlreadyInOrganizationException.
func TestIntegration_MGN_ListManagedAccounts(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	ctx := t.Context()
	mgnClient := createMGNClient(t)
	orgClient := createOrganizationsClientAt(t, endpoint)

	initializeMGNAccount(t, mgnClient)

	_, err := orgClient.CreateOrganization(ctx, &organizationsSDK.CreateOrganizationInput{
		FeatureSet: organizationstypes.OrganizationFeatureSetAll,
	})
	if err != nil {
		var already *organizationstypes.AlreadyInOrganizationException
		require.ErrorAs(t, err, &already, "CreateOrganization should succeed or be AlreadyInOrganizationException")
	}

	createOut, err := orgClient.CreateAccount(ctx, &organizationsSDK.CreateAccountInput{
		AccountName: aws.String("mgn-managed-member"),
		Email:       aws.String("mgn-managed-member@example.com"),
	})
	require.NoError(t, err, "CreateAccount should succeed")
	memberAccountID := aws.ToString(createOut.CreateAccountStatus.AccountId)
	require.NotEmpty(t, memberAccountID)

	managed, err := mgnClient.ListManagedAccounts(ctx, &mgnsdk.ListManagedAccountsInput{})
	require.NoError(t, err, "ListManagedAccounts should succeed")

	found := false

	for _, a := range managed.Items {
		if aws.ToString(a.AccountId) == memberAccountID {
			found = true

			break
		}
	}

	assert.True(t, found, "ListManagedAccounts should include a real Organizations member account")
}

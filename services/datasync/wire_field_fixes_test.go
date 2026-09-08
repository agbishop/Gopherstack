package datasync_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	datasyncsdk "github.com/aws/aws-sdk-go-v2/service/datasync"
	"github.com/aws/aws-sdk-go-v2/service/datasync/types"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/services/datasync"
)

// newTestDataSyncClient stands up the real aws-sdk-go-v2 DataSync client
// against an httptest server running this package's Handler, wired through
// the same pkgs/service registry/router used in production.
func newTestDataSyncClient(t *testing.T, h *datasync.Handler) *datasyncsdk.Client {
	t.Helper()

	e := echo.New()
	registry := service.NewRegistry()
	require.NoError(t, registry.Register(h))
	e.Use(service.NewServiceRouter(registry).RouteHandler())

	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)

	cfg, err := awscfg.LoadDefaultConfig(
		t.Context(),
		awscfg.WithRegion("us-east-1"),
		awscfg.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err)

	return datasyncsdk.NewFromConfig(cfg, func(o *datasyncsdk.Options) {
		o.BaseEndpoint = aws.String(srv.URL)
	})
}

// TestTaskMode_RoundTrips_ThroughListAndDescribe_RealClient covers a
// layer-3 bug (gopherstack-g8k9): Task.TaskMode is real, tracked state --
// CreateTask stores it and DescribeTask already emits it correctly (the
// second-op signal) -- but ListTasks' TaskListEntry never carried it
// through, and the identical gap existed one level over for task
// executions: StartTaskExecution/DescribeTaskExecution/ListTaskExecutions
// never captured or emitted TaskMode at all despite the parent task's mode
// being known at execution-start time. Real fields confirmed against
// datasync@v1.61.4 deserializers.go: awsAwsjson11_deserializeDocumentTaskListEntry
// and awsAwsjson11_deserializeDocumentTaskExecutionListEntry both have a
// "TaskMode" case, as does awsAwsjson11_deserializeOpDocumentDescribeTaskExecutionOutput.
func TestTaskMode_RoundTrips_ThroughListAndDescribe_RealClient(t *testing.T) {
	t.Parallel()

	backend := datasync.NewInMemoryBackend("123456789012", "us-east-1")
	client := newTestDataSyncClient(t, datasync.NewHandler(backend))
	ctx := t.Context()

	src, err := client.CreateLocationObjectStorage(ctx, &datasyncsdk.CreateLocationObjectStorageInput{
		ServerHostname: aws.String("src.example.com"),
		BucketName:     aws.String("src-bucket"),
	})
	require.NoError(t, err)

	dst, err := client.CreateLocationObjectStorage(ctx, &datasyncsdk.CreateLocationObjectStorageInput{
		ServerHostname: aws.String("dst.example.com"),
		BucketName:     aws.String("dst-bucket"),
	})
	require.NoError(t, err)

	createdTask, err := client.CreateTask(ctx, &datasyncsdk.CreateTaskInput{
		SourceLocationArn:      src.LocationArn,
		DestinationLocationArn: dst.LocationArn,
		Name:                   aws.String("enhanced-task"),
		TaskMode:               types.TaskModeEnhanced,
	})
	require.NoError(t, err)

	listed, err := client.ListTasks(ctx, &datasyncsdk.ListTasksInput{})
	require.NoError(t, err)
	require.Len(t, listed.Tasks, 1)
	assert.Equal(t, types.TaskModeEnhanced, listed.Tasks[0].TaskMode,
		"ListTasks: TaskMode must round-trip; pre-fix it was always empty")

	started, err := client.StartTaskExecution(ctx, &datasyncsdk.StartTaskExecutionInput{
		TaskArn: createdTask.TaskArn,
	})
	require.NoError(t, err)

	described, err := client.DescribeTaskExecution(ctx, &datasyncsdk.DescribeTaskExecutionInput{
		TaskExecutionArn: started.TaskExecutionArn,
	})
	require.NoError(t, err)
	assert.Equal(t, types.TaskModeEnhanced, described.TaskMode,
		"DescribeTaskExecution: TaskMode must round-trip; pre-fix it was always empty")

	listedExecs, err := client.ListTaskExecutions(ctx, &datasyncsdk.ListTaskExecutionsInput{
		TaskArn: createdTask.TaskArn,
	})
	require.NoError(t, err)
	require.Len(t, listedExecs.TaskExecutions, 1)
	assert.Equal(t, types.TaskModeEnhanced, listedExecs.TaskExecutions[0].TaskMode,
		"ListTaskExecutions: TaskMode must round-trip; pre-fix it was always empty")
}

// TestNotFound_TypesAsInvalidRequestException_RealClient proves that every
// not-found condition in datasync decodes, through the real SDK client, as
// *types.InvalidRequestException -- never as a ResourceNotFoundException,
// which does not exist anywhere in this service. Confirmed against every one
// of datasync's 53 awsAwsjson11_deserializeOpError<Op> switches (aws-sdk-
// go-v2/service/datasync@v1.61.4 deserializers.go), which type only
// InternalException and InvalidRequestException, and against types/errors.go,
// which defines exactly those two exception structs and no
// ResourceNotFoundException/ResourceExistsException type at all. Before the
// fix, handler.go's handleError mapped ErrNotFound/ErrAlreadyExists to those
// two fabricated wire codes, which every op's own switch falls through to its
// default case for -- decoding as an untyped smithy.GenericAPIError instead
// of a real exception type, for every not-found path in the whole service.
func TestNotFound_TypesAsInvalidRequestException_RealClient(t *testing.T) {
	t.Parallel()

	backend := datasync.NewInMemoryBackend("000000000000", "us-east-1")
	h := datasync.NewHandler(backend)
	client := newTestDataSyncClient(t, h)
	ctx := t.Context()

	_, err := client.DescribeTask(ctx, &datasyncsdk.DescribeTaskInput{
		TaskArn: aws.String("arn:aws:datasync:us-east-1:000000000000:task/notexist"),
	})
	require.Error(t, err)

	var invalidRequest *types.InvalidRequestException
	require.ErrorAs(t, err, &invalidRequest)
}

// TestListLocations_NoFabricatedCreationTime covers an invented-field bug:
// types.LocationListEntry (datasync@v1.61.4 api_op_ListLocations.go) has
// exactly two members, LocationArn and LocationUri -- no CreationTime.
// gopherstack's per-item response emitted an extra "CreationTime" key that
// doesn't exist on the real wire (harmless to a typed client, which ignores
// unknown JSON fields, but incorrect against the real shape). Asserted on
// the raw body since the typed SDK response has no field to read it into.
func TestListLocations_NoFabricatedCreationTime(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	createRec := doRequest(t, h, "CreateLocationS3", map[string]any{
		"S3BucketArn":  "arn:aws:s3:::wfx-bucket",
		"Subdirectory": "/",
		"S3Config": map[string]any{
			"BucketAccessRoleArn": "arn:aws:iam::000000000000:role/Role",
		},
	})
	require.Equal(t, http.StatusOK, createRec.Code, createRec.Body.String())

	listRec := doRequest(t, h, "ListLocations", map[string]any{})
	require.Equal(t, http.StatusOK, listRec.Code, listRec.Body.String())

	var resp struct {
		Locations []map[string]any `json:"Locations"`
	}
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &resp))
	require.Len(t, resp.Locations, 1, "must exercise a non-empty collection")

	_, hasCreationTime := resp.Locations[0]["CreationTime"]
	assert.False(t, hasCreationTime,
		"ListLocations: LocationListEntry has no CreationTime member on the real wire")
	assert.Contains(t, resp.Locations[0], "LocationArn")
	assert.Contains(t, resp.Locations[0], "LocationUri")
}

// TestStartTaskExecution_Overrides_RoundTrip_RealClient covers a discarded-
// parameter bug (gopherstack-50h): StartTaskExecutionInput accepts
// OverrideOptions, Excludes, Includes, ManifestConfig and TaskReportConfig
// to override the parent task's settings for a single run (datasync@v1.61.4
// api_op_StartTaskExecution.go: "You also can override your task options for
// each task execution"), and DescribeTaskExecutionOutput has its own Options,
// Excludes, Includes, ManifestConfig and TaskReportConfig members
// (api_op_DescribeTaskExecution.go) to report them back. Before the fix,
// gopherstack's startTaskExecutionInput struct declared only TaskArn, so
// every one of those fields was silently dropped by the JSON decode -- never
// stored, never returned by DescribeTaskExecution.
func TestStartTaskExecution_Overrides_RoundTrip_RealClient(t *testing.T) {
	t.Parallel()

	backend := datasync.NewInMemoryBackend("111122223333", "us-east-1")
	client := newTestDataSyncClient(t, datasync.NewHandler(backend))
	ctx := t.Context()

	src, err := client.CreateLocationObjectStorage(ctx, &datasyncsdk.CreateLocationObjectStorageInput{
		ServerHostname: aws.String("src.example.com"),
		BucketName:     aws.String("src-bucket"),
	})
	require.NoError(t, err)

	dst, err := client.CreateLocationObjectStorage(ctx, &datasyncsdk.CreateLocationObjectStorageInput{
		ServerHostname: aws.String("dst.example.com"),
		BucketName:     aws.String("dst-bucket"),
	})
	require.NoError(t, err)

	createdTask, err := client.CreateTask(ctx, &datasyncsdk.CreateTaskInput{
		SourceLocationArn:      src.LocationArn,
		DestinationLocationArn: dst.LocationArn,
		Name:                   aws.String("override-task"),
	})
	require.NoError(t, err)

	started, err := client.StartTaskExecution(ctx, &datasyncsdk.StartTaskExecutionInput{
		TaskArn: createdTask.TaskArn,
		OverrideOptions: &types.Options{
			LogLevel: types.LogLevelTransfer,
		},
		Excludes: []types.FilterRule{
			{FilterType: types.FilterTypeSimplePattern, Value: aws.String("/tmp")},
		},
	})
	require.NoError(t, err)

	described, err := client.DescribeTaskExecution(ctx, &datasyncsdk.DescribeTaskExecutionInput{
		TaskExecutionArn: started.TaskExecutionArn,
	})
	require.NoError(t, err)

	require.NotNil(t, described.Options,
		"DescribeTaskExecution: OverrideOptions must round-trip through Options; pre-fix it was always nil")
	assert.Equal(t, types.LogLevelTransfer, described.Options.LogLevel)
	require.Len(t, described.Excludes, 1,
		"DescribeTaskExecution: Excludes must round-trip; pre-fix it was always dropped")
	assert.Equal(t, "/tmp", aws.ToString(described.Excludes[0].Value))
}

// TestTagResource_TaskExecution_RealClient covers a referential-integrity
// bug (gopherstack-50h): TagResource's doc comment (datasync@v1.61.4
// api_op_TagResource.go) names task executions as taggable DataSync
// resources ("These include DataSync resources, such as locations, tasks,
// and task executions"), but isKnownResource only checked agents, locations,
// and tasks -- so TagResource/ListTagsForResource on a valid task execution
// ARN incorrectly failed as ResourceNotFound (InvalidRequestException).
func TestTagResource_TaskExecution_RealClient(t *testing.T) {
	t.Parallel()

	backend := datasync.NewInMemoryBackend("444455556666", "us-east-1")
	client := newTestDataSyncClient(t, datasync.NewHandler(backend))
	ctx := t.Context()

	src, err := client.CreateLocationObjectStorage(ctx, &datasyncsdk.CreateLocationObjectStorageInput{
		ServerHostname: aws.String("src.example.com"),
		BucketName:     aws.String("src-bucket"),
	})
	require.NoError(t, err)

	dst, err := client.CreateLocationObjectStorage(ctx, &datasyncsdk.CreateLocationObjectStorageInput{
		ServerHostname: aws.String("dst.example.com"),
		BucketName:     aws.String("dst-bucket"),
	})
	require.NoError(t, err)

	createdTask, err := client.CreateTask(ctx, &datasyncsdk.CreateTaskInput{
		SourceLocationArn:      src.LocationArn,
		DestinationLocationArn: dst.LocationArn,
		Name:                   aws.String("taggable-task"),
	})
	require.NoError(t, err)

	started, err := client.StartTaskExecution(ctx, &datasyncsdk.StartTaskExecutionInput{
		TaskArn: createdTask.TaskArn,
	})
	require.NoError(t, err)

	_, err = client.TagResource(ctx, &datasyncsdk.TagResourceInput{
		ResourceArn: started.TaskExecutionArn,
		Tags: []types.TagListEntry{
			{Key: aws.String("owner"), Value: aws.String("parity-sweep")},
		},
	})
	require.NoError(t, err,
		"TagResource on a task execution ARN must succeed; pre-fix isKnownResource omitted executions")

	listed, err := client.ListTagsForResource(ctx, &datasyncsdk.ListTagsForResourceInput{
		ResourceArn: started.TaskExecutionArn,
	})
	require.NoError(t, err)
	require.Len(t, listed.Tags, 1)
	assert.Equal(t, "owner", aws.ToString(listed.Tags[0].Key))
	assert.Equal(t, "parity-sweep", aws.ToString(listed.Tags[0].Value))
}

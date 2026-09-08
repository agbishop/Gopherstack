package integration_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	iotanalyticssdk "github.com/aws/aws-sdk-go-v2/service/iotanalytics"
	iotanalyticstype "github.com/aws/aws-sdk-go-v2/service/iotanalytics/types"
)

// createIoTAnalyticsClient returns an IoT Analytics client pointed at the shared test stack.
func createIoTAnalyticsClient(t *testing.T) *iotanalyticssdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return iotanalyticssdk.NewFromConfig(cfg, func(o *iotanalyticssdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// TestIntegration_IoTAnalytics_ChannelLifecycle tests the full channel CRUD lifecycle via the SDK.
func TestIntegration_IoTAnalytics_ChannelLifecycle(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	client := createIoTAnalyticsClient(t)
	channelName := "integration_channel_" + t.Name()

	createOut, err := client.CreateChannel(
		ctx, &iotanalyticssdk.CreateChannelInput{ChannelName: aws.String(channelName)},
	)
	require.NoError(t, err, "CreateChannel should succeed")
	assert.Equal(t, channelName, aws.ToString(createOut.ChannelName))
	assert.NotEmpty(t, createOut.ChannelArn)

	listOut, err := client.ListChannels(
		ctx, &iotanalyticssdk.ListChannelsInput{},
	)
	require.NoError(t, err, "ListChannels should succeed")

	found := false

	for _, ch := range listOut.ChannelSummaries {
		if aws.ToString(ch.ChannelName) == channelName {
			found = true

			break
		}
	}

	assert.True(t, found, "created channel should appear in list")

	descOut, err := client.DescribeChannel(
		ctx, &iotanalyticssdk.DescribeChannelInput{ChannelName: aws.String(channelName)},
	)
	require.NoError(t, err, "DescribeChannel should succeed")
	ch := descOut.Channel
	assert.Equal(t, channelName, aws.ToString(ch.Name))

	_, err = client.UpdateChannel(
		ctx, &iotanalyticssdk.UpdateChannelInput{ChannelName: aws.String(channelName)},
	)
	require.NoError(t, err, "UpdateChannel should succeed")

	_, err = client.DeleteChannel(
		ctx, &iotanalyticssdk.DeleteChannelInput{ChannelName: aws.String(channelName)},
	)
	require.NoError(t, err, "DeleteChannel should succeed")

	_, err = client.DescribeChannel(
		ctx, &iotanalyticssdk.DescribeChannelInput{ChannelName: aws.String(channelName)},
	)
	require.Error(t, err, "DescribeChannel after delete should return error")
}

// TestIntegration_IoTAnalytics_DatastoreLifecycle tests the full datastore CRUD lifecycle via the SDK.
func TestIntegration_IoTAnalytics_DatastoreLifecycle(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	client := createIoTAnalyticsClient(t)
	datastoreName := "integration_datastore_" + t.Name()

	createOut, err := client.CreateDatastore(
		ctx, &iotanalyticssdk.CreateDatastoreInput{DatastoreName: aws.String(datastoreName)},
	)
	require.NoError(t, err, "CreateDatastore should succeed")
	assert.Equal(t, datastoreName, aws.ToString(createOut.DatastoreName))

	listOut, err := client.ListDatastores(
		ctx, &iotanalyticssdk.ListDatastoresInput{},
	)
	require.NoError(t, err, "ListDatastores should succeed")

	found := false

	for _, ds := range listOut.DatastoreSummaries {
		if aws.ToString(ds.DatastoreName) == datastoreName {
			found = true

			break
		}
	}

	assert.True(t, found, "created datastore should appear in list")

	_, err = client.DeleteDatastore(
		ctx, &iotanalyticssdk.DeleteDatastoreInput{DatastoreName: aws.String(datastoreName)},
	)
	require.NoError(t, err, "DeleteDatastore should succeed")
}

// TestIntegration_IoTAnalytics_PipelineLifecycle tests the full pipeline CRUD lifecycle via the SDK.
func TestIntegration_IoTAnalytics_PipelineLifecycle(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	client := createIoTAnalyticsClient(t)
	pipelineName := "integration_pipeline_" + t.Name()

	createOut, err := client.CreatePipeline(
		ctx, &iotanalyticssdk.CreatePipelineInput{
			PipelineName: aws.String(pipelineName),
			PipelineActivities: []iotanalyticstype.PipelineActivity{
				{
					Channel: &iotanalyticstype.ChannelActivity{
						Name:        aws.String("channel_activity"),
						ChannelName: aws.String("test_channel"),
					},
				},
				{
					Datastore: &iotanalyticstype.DatastoreActivity{
						Name:          aws.String("datastore_activity"),
						DatastoreName: aws.String("test_datastore"),
					},
				},
			},
		},
	)
	require.NoError(t, err, "CreatePipeline should succeed")
	assert.Equal(t, pipelineName, aws.ToString(createOut.PipelineName))

	listOut, err := client.ListPipelines(
		ctx, &iotanalyticssdk.ListPipelinesInput{},
	)
	require.NoError(t, err, "ListPipelines should succeed")

	found := false

	for _, p := range listOut.PipelineSummaries {
		if aws.ToString(p.PipelineName) == pipelineName {
			found = true

			break
		}
	}

	assert.True(t, found, "created pipeline should appear in list")

	descOut, err := client.DescribePipeline(
		ctx, &iotanalyticssdk.DescribePipelineInput{PipelineName: aws.String(pipelineName)},
	)
	require.NoError(t, err, "DescribePipeline should succeed")
	pipeline := descOut.Pipeline
	assert.Equal(t, pipelineName, aws.ToString(pipeline.Name))

	_, err = client.DeletePipeline(
		ctx, &iotanalyticssdk.DeletePipelineInput{PipelineName: aws.String(pipelineName)},
	)
	require.NoError(t, err, "DeletePipeline should succeed")
}

// TestIntegration_IoTAnalytics_BatchPutMessage tests BatchPutMessage via the SDK.
func TestIntegration_IoTAnalytics_BatchPutMessage(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	client := createIoTAnalyticsClient(t)
	channelName := "integration_batchput_channel_" + t.Name()

	_, err := client.CreateChannel(
		ctx, &iotanalyticssdk.CreateChannelInput{ChannelName: aws.String(channelName)},
	)
	require.NoError(t, err, "CreateChannel should succeed")

	defer func() {
		_, _ = client.DeleteChannel(
			ctx, &iotanalyticssdk.DeleteChannelInput{ChannelName: aws.String(channelName)},
		)
	}()

	out, err := client.BatchPutMessage(
		ctx, &iotanalyticssdk.BatchPutMessageInput{
			ChannelName: aws.String(channelName),
			Messages: []iotanalyticstype.Message{
				{
					MessageId: aws.String("msg-1"),
					Payload:   []byte(`{"temperature":25}`),
				},
			},
		},
	)
	require.NoError(t, err, "BatchPutMessage should succeed")
	assert.Empty(t, out.BatchPutMessageErrorEntries, "no error entries expected")
}

// TestIntegration_IoTAnalytics_SampleChannelData tests SampleChannelData via the SDK.
func TestIntegration_IoTAnalytics_SampleChannelData(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	client := createIoTAnalyticsClient(t)
	channelName := "integration_sample_channel_" + t.Name()

	_, err := client.CreateChannel(
		ctx, &iotanalyticssdk.CreateChannelInput{ChannelName: aws.String(channelName)},
	)
	require.NoError(t, err, "CreateChannel should succeed")

	defer func() {
		_, _ = client.DeleteChannel(
			ctx, &iotanalyticssdk.DeleteChannelInput{ChannelName: aws.String(channelName)},
		)
	}()

	out, err := client.SampleChannelData(
		ctx, &iotanalyticssdk.SampleChannelDataInput{ChannelName: aws.String(channelName)},
	)
	require.NoError(t, err, "SampleChannelData should succeed")
	assert.NotNil(t, out)
}

// TestIntegration_IoTAnalytics_PipelineReprocessing tests start/cancel reprocessing via the SDK.
func TestIntegration_IoTAnalytics_PipelineReprocessing(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	client := createIoTAnalyticsClient(t)
	pipelineName := "integration_reprocessing_pipeline_" + t.Name()

	_, err := client.CreatePipeline(
		ctx, &iotanalyticssdk.CreatePipelineInput{
			PipelineName: aws.String(pipelineName),
			PipelineActivities: []iotanalyticstype.PipelineActivity{
				{
					Channel: &iotanalyticstype.ChannelActivity{
						Name:        aws.String("channel_activity"),
						ChannelName: aws.String("test_channel"),
					},
				},
				{
					Datastore: &iotanalyticstype.DatastoreActivity{
						Name:          aws.String("datastore_activity"),
						DatastoreName: aws.String("test_datastore"),
					},
				},
			},
		},
	)
	require.NoError(t, err, "CreatePipeline should succeed")

	defer func() {
		_, _ = client.DeletePipeline(
			ctx, &iotanalyticssdk.DeletePipelineInput{PipelineName: aws.String(pipelineName)},
		)
	}()

	startOut, err := client.StartPipelineReprocessing(
		ctx, &iotanalyticssdk.StartPipelineReprocessingInput{PipelineName: aws.String(pipelineName)},
	)
	require.NoError(t, err, "StartPipelineReprocessing should succeed")
	reprocessingID := aws.ToString(startOut.ReprocessingId)
	assert.NotEmpty(t, reprocessingID)

	_, err = client.CancelPipelineReprocessing(
		ctx, &iotanalyticssdk.CancelPipelineReprocessingInput{
			PipelineName:   aws.String(pipelineName),
			ReprocessingId: aws.String(reprocessingID),
		},
	)
	require.NoError(t, err, "CancelPipelineReprocessing should succeed")
}

// TestIntegration_IoTAnalytics_DatasetContent tests dataset content operations via the SDK.
func TestIntegration_IoTAnalytics_DatasetContent(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	client := createIoTAnalyticsClient(t)
	datasetName := "integration_content_dataset_" + t.Name()

	_, err := client.CreateDataset(
		ctx, &iotanalyticssdk.CreateDatasetInput{
			DatasetName: aws.String(datasetName),
			Actions: []iotanalyticstype.DatasetAction{
				{
					ActionName: aws.String("myaction"),
					QueryAction: &iotanalyticstype.SqlQueryDatasetAction{
						SqlQuery: aws.String("SELECT name, value FROM my_datastore"),
					},
				},
			},
		},
	)
	require.NoError(t, err, "CreateDataset should succeed")

	defer func() {
		_, _ = client.DeleteDataset(
			ctx, &iotanalyticssdk.DeleteDatasetInput{DatasetName: aws.String(datasetName)},
		)
	}()

	createOut, err := client.CreateDatasetContent(
		ctx, &iotanalyticssdk.CreateDatasetContentInput{DatasetName: aws.String(datasetName)},
	)
	require.NoError(t, err, "CreateDatasetContent should succeed")
	assert.NotEmpty(t, createOut.VersionId)

	getOut, err := client.GetDatasetContent(
		ctx, &iotanalyticssdk.GetDatasetContentInput{DatasetName: aws.String(datasetName)},
	)
	require.NoError(t, err, "GetDatasetContent should succeed")
	assert.NotNil(t, getOut.Status)

	listOut, err := client.ListDatasetContents(
		ctx, &iotanalyticssdk.ListDatasetContentsInput{DatasetName: aws.String(datasetName)},
	)
	require.NoError(t, err, "ListDatasetContents should succeed")
	assert.Len(t, listOut.DatasetContentSummaries, 1)

	_, err = client.DeleteDatasetContent(
		ctx, &iotanalyticssdk.DeleteDatasetContentInput{DatasetName: aws.String(datasetName)},
	)
	require.NoError(t, err, "DeleteDatasetContent should succeed")
}

// TestIntegration_IoTAnalytics_LoggingOptions tests put/describe logging options via the SDK.
func TestIntegration_IoTAnalytics_LoggingOptions(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	client := createIoTAnalyticsClient(t)

	_, err := client.PutLoggingOptions(
		ctx, &iotanalyticssdk.PutLoggingOptionsInput{
			LoggingOptions: &iotanalyticstype.LoggingOptions{
				RoleArn: aws.String("arn:aws:iam::000000000000:role/iot-logging-role"),
				Level:   iotanalyticstype.LoggingLevelError,
				Enabled: true,
			},
		},
	)
	require.NoError(t, err, "PutLoggingOptions should succeed")

	descOut, err := client.DescribeLoggingOptions(
		ctx, &iotanalyticssdk.DescribeLoggingOptionsInput{},
	)
	require.NoError(t, err, "DescribeLoggingOptions should succeed")
	require.NotNil(t, descOut.LoggingOptions)
	assert.Equal(t, iotanalyticstype.LoggingLevelError,
		descOut.LoggingOptions.Level)
	assert.True(t, descOut.LoggingOptions.Enabled)
	assert.Equal(t, "arn:aws:iam::000000000000:role/iot-logging-role",
		aws.ToString(descOut.LoggingOptions.RoleArn))
}

// TestIntegration_IoTAnalytics_RunPipelineActivity tests RunPipelineActivity via the SDK.
func TestIntegration_IoTAnalytics_RunPipelineActivity(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	client := createIoTAnalyticsClient(t)

	out, err := client.RunPipelineActivity(
		ctx, &iotanalyticssdk.RunPipelineActivityInput{
			PipelineActivity: &iotanalyticstype.PipelineActivity{
				Channel: &iotanalyticstype.ChannelActivity{
					Name:        aws.String("ch"),
					ChannelName: aws.String("test_channel"),
				},
			},
			Payloads: [][]byte{
				[]byte(`{"temperature":22}`),
			},
		},
	)
	require.NoError(t, err, "RunPipelineActivity should succeed")
	assert.Len(t, out.Payloads, 1)
}

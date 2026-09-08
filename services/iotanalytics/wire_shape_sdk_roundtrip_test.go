package iotanalytics_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	iotanalyticssdk "github.com/aws/aws-sdk-go-v2/service/iotanalytics" //nolint:staticcheck // AWS has deprecated this service; gopherstack still supports it
	iotanalyticstypes "github.com/aws/aws-sdk-go-v2/service/iotanalytics/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/iotanalytics"
)

// TestDescribeChannel_Statistics_SDKRoundTrip proves DescribeChannelOutput.Statistics
// decodes through the real SDK client as a sibling of Channel, not nested inside it.
// AWS's DescribeChannelOutput declares Channel and Statistics as two separate members
// (api_op_DescribeChannel.go), and types.Channel itself has no statistics field at all
// (deserializers.go awsRestjson1_deserializeDocumentChannel has no "statistics" case).
// Before the fix, gopherstack nested statistics inside the "channel" object, so a real
// client's DescribeChannelOutput.Statistics stayed nil even with IncludeStatistics=true.
//
//nolint:staticcheck // iotanalytics is AWS-deprecated; gopherstack still emulates it
func TestDescribeChannel_Statistics_SDKRoundTrip(t *testing.T) {
	t.Parallel()

	backend := iotanalytics.NewInMemoryBackend()
	client := newTestIoTAnalyticsClient(t, iotanalytics.NewHandler(backend))

	_, err := client.CreateChannel(t.Context(), &iotanalyticssdk.CreateChannelInput{
		ChannelName: aws.String("stats_channel"),
	})
	require.NoError(t, err)

	out, err := client.DescribeChannel(t.Context(), &iotanalyticssdk.DescribeChannelInput{
		ChannelName:       aws.String("stats_channel"),
		IncludeStatistics: true,
	})
	require.NoError(t, err)
	require.NotNil(t, out.Channel)
	require.NotNil(
		t,
		out.Statistics,
		"Statistics must decode as a sibling of Channel, not nested inside it",
	)
	require.NotNil(t, out.Statistics.Size)
}

// TestDescribeDatastore_Statistics_SDKRoundTrip is the DescribeDatastore analog of
// TestDescribeChannel_Statistics_SDKRoundTrip: DescribeDatastoreOutput also declares
// Datastore and Statistics as separate top-level members (api_op_DescribeDatastore.go),
// and types.Datastore has no statistics field of its own.
//
//nolint:staticcheck // iotanalytics is AWS-deprecated; gopherstack still emulates it
func TestDescribeDatastore_Statistics_SDKRoundTrip(t *testing.T) {
	t.Parallel()

	backend := iotanalytics.NewInMemoryBackend()
	client := newTestIoTAnalyticsClient(t, iotanalytics.NewHandler(backend))

	_, err := client.CreateDatastore(t.Context(), &iotanalyticssdk.CreateDatastoreInput{
		DatastoreName: aws.String("stats_datastore"),
	})
	require.NoError(t, err)

	out, err := client.DescribeDatastore(t.Context(), &iotanalyticssdk.DescribeDatastoreInput{
		DatastoreName:     aws.String("stats_datastore"),
		IncludeStatistics: true,
	})
	require.NoError(t, err)
	require.NotNil(t, out.Datastore)
	require.NotNil(
		t,
		out.Statistics,
		"Statistics must decode as a sibling of Datastore, not nested inside it",
	)
	require.NotNil(t, out.Statistics.Size)
}

// TestCreateDatastore_Partitions_SDKRoundTrip proves DatastorePartitions round-trips
// through the real SDK client under AWS's wire key "datastorePartitions", not
// "partitions" (serializers.go awsRestjson1_serializeOpDocumentCreateDatastoreInput
// and deserializers.go awsRestjson1_deserializeDocumentDatastore both use
// "datastorePartitions"). Before the fix, gopherstack's CreateDatastore handler read
// the field under "partitions", so the real client's serialized "datastorePartitions"
// was silently dropped and every created datastore ended up with nil partitions.
//
//nolint:staticcheck // iotanalytics is AWS-deprecated; gopherstack still emulates it
func TestCreateDatastore_Partitions_SDKRoundTrip(t *testing.T) {
	t.Parallel()

	backend := iotanalytics.NewInMemoryBackend()
	client := newTestIoTAnalyticsClient(t, iotanalytics.NewHandler(backend))

	_, err := client.CreateDatastore(t.Context(), &iotanalyticssdk.CreateDatastoreInput{
		DatastoreName: aws.String("partitioned_datastore"),
		DatastorePartitions: &iotanalyticstypes.DatastorePartitions{
			Partitions: []iotanalyticstypes.DatastorePartition{
				{
					AttributePartition: &iotanalyticstypes.Partition{
						AttributeName: aws.String("deviceId"),
					},
				},
			},
		},
	})
	require.NoError(t, err)

	out, err := client.DescribeDatastore(t.Context(), &iotanalyticssdk.DescribeDatastoreInput{
		DatastoreName: aws.String("partitioned_datastore"),
	})
	require.NoError(t, err)
	require.NotNil(t, out.Datastore)
	require.NotNil(
		t,
		out.Datastore.DatastorePartitions,
		"datastorePartitions must round-trip under its real wire key",
	)
	require.Len(t, out.Datastore.DatastorePartitions.Partitions, 1)
	require.NotNil(t, out.Datastore.DatastorePartitions.Partitions[0].AttributePartition)
	assert.Equal(
		t,
		"deviceId",
		aws.ToString(
			out.Datastore.DatastorePartitions.Partitions[0].AttributePartition.AttributeName,
		),
	)
}

// TestListSummaries_NoFabricatedARN proves ListChannels/ListDatastores/ListDatasets/
// ListPipelines summaries carry no arn member -- unlike each resource's full Describe
// shape, AWS's four *Summary deserializers (deserializers.go
// awsRestjson1_deserializeDocumentChannelSummary,
// awsRestjson1_deserializeDocumentDatastoreSummary,
// awsRestjson1_deserializeDocumentDatasetSummary,
// awsRestjson1_deserializeDocumentPipelineSummary) declare no arn case at all. The real
// typed SDK client has no field to observe this on (types.ChannelSummary etc. simply
// don't expose one), so this is a raw-body absence assertion rather than an SDK
// round-trip.
func TestListSummaries_NoFabricatedARN(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		create     func(t *testing.T, h *iotanalytics.Handler)
		listPath   string
		summariesK string
		arnKey     string
	}{
		{
			name: "channel",
			create: func(t *testing.T, h *iotanalytics.Handler) {
				t.Helper()
				doRequest(
					t,
					h,
					http.MethodPost,
					"/channels",
					map[string]string{"channelName": "sumch"},
				)
			},
			listPath:   "/channels",
			summariesK: "channelSummaries",
			arnKey:     "channelArn",
		},
		{
			name: "datastore",
			create: func(t *testing.T, h *iotanalytics.Handler) {
				t.Helper()
				doRequest(
					t,
					h,
					http.MethodPost,
					"/datastores",
					map[string]string{"datastoreName": "sumds"},
				)
			},
			listPath:   "/datastores",
			summariesK: "datastoreSummaries",
			arnKey:     "datastoreArn",
		},
		{
			name: "dataset",
			create: func(t *testing.T, h *iotanalytics.Handler) {
				t.Helper()
				doRequest(
					t,
					h,
					http.MethodPost,
					"/datasets",
					map[string]string{"datasetName": "sumdset"},
				)
			},
			listPath:   "/datasets",
			summariesK: "datasetSummaries",
			arnKey:     "datasetArn",
		},
		{
			name: "pipeline",
			create: func(t *testing.T, h *iotanalytics.Handler) {
				t.Helper()
				doRequest(
					t,
					h,
					http.MethodPost,
					"/pipelines",
					map[string]any{"pipelineName": "sumpipe", "pipelineActivities": validPipelineActivitiesBody()},
				)
			},
			listPath:   "/pipelines",
			summariesK: "pipelineSummaries",
			arnKey:     "pipelineArn",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			tt.create(t, h)

			rec := doRequest(t, h, http.MethodGet, tt.listPath, nil)
			require.Equal(t, http.StatusOK, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

			summaries, ok := resp[tt.summariesK].([]any)
			require.True(t, ok)
			require.Len(t, summaries, 1)

			summary, ok := summaries[0].(map[string]any)
			require.True(t, ok)
			_, hasARN := summary[tt.arnKey]
			assert.False(t, hasARN, "%s must not appear on a summary entry", tt.arnKey)
		})
	}
}

// TestDatastore_IotSiteWiseCustomerManagedS3Storage_NoRoleArn proves the nested
// customerManagedS3Storage object under datastoreStorage.iotSiteWiseMultiLayerStorage
// carries no roleArn -- AWS's IotSiteWiseCustomerManagedDatastoreS3Storage type only has
// bucket and keyPrefix (deserializers.go
// awsRestjson1_deserializeDocumentIotSiteWiseCustomerManagedDatastoreS3Storage), unlike
// the wider CustomerManagedDatastoreS3Storage used directly under datastoreStorage,
// which does have roleArn. The real typed client has no field to observe this on
// (types.IotSiteWiseCustomerManagedDatastoreS3Storage has no RoleArn member), so this is
// a raw-body absence assertion.
func TestDatastore_IotSiteWiseCustomerManagedS3Storage_NoRoleArn(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	body := map[string]any{
		"datastoreName": "sitewise_ds",
		"datastoreStorage": map[string]any{
			"iotSiteWiseMultiLayerStorage": map[string]any{
				"customerManagedS3Storage": map[string]any{
					"bucket":    "my-bucket",
					"keyPrefix": "prefix/",
				},
			},
		},
	}

	rec := doRequest(t, h, http.MethodPost, "/datastores", body)
	require.Equal(t, http.StatusOK, rec.Code)

	descRec := doRequest(t, h, http.MethodGet, "/datastores/sitewise_ds", nil)
	require.Equal(t, http.StatusOK, descRec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &resp))

	datastore, ok := resp["datastore"].(map[string]any)
	require.True(t, ok)
	storage, ok := datastore["storage"].(map[string]any)
	require.True(t, ok)
	sitewise, ok := storage["iotSiteWiseMultiLayerStorage"].(map[string]any)
	require.True(t, ok)
	cms3, ok := sitewise["customerManagedS3Storage"].(map[string]any)
	require.True(t, ok)

	assert.Equal(t, "my-bucket", cms3["bucket"])
	_, hasRoleArn := cms3["roleArn"]
	assert.False(
		t,
		hasRoleArn,
		"iotSiteWiseMultiLayerStorage.customerManagedS3Storage must not carry roleArn",
	)
}

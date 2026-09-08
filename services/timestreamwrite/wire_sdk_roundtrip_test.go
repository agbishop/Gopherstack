package timestreamwrite_test

import (
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	timestreamwritesdk "github.com/aws/aws-sdk-go-v2/service/timestreamwrite"
	twtypes "github.com/aws/aws-sdk-go-v2/service/timestreamwrite/types"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/services/timestreamwrite"
)

// newTestTimestreamWriteSDKClient stands up the real aws-sdk-go-v2
// timestreamwrite client against an httptest server running this package's
// Handler, wired through the same pkgs/service registry/router used in
// production -- so a shape is verified by the real client's own
// awsAwsjson10_deserializeOpDocument<Op>Output, not gopherstack's own JSON
// tags.
func newTestTimestreamWriteSDKClient(t *testing.T, h *timestreamwrite.Handler) *timestreamwritesdk.Client {
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

	return timestreamwritesdk.NewFromConfig(cfg, func(o *timestreamwritesdk.Options) {
		o.BaseEndpoint = aws.String(srv.URL)
	})
}

// TestCreateBatchLoadTask_DataModelConfigurationSDKRoundTrip proves that
// CreateBatchLoadTaskInput.DataModelConfiguration -- a real, optional member
// of the wire request (aws-sdk-go-v2/service/timestreamwrite@v1.38.4
// api_op_CreateBatchLoadTask.go's CreateBatchLoadTaskInput struct, serialized
// by serializers.go:1815's awsAwsjson10_serializeOpDocumentCreateBatchLoadTaskInput,
// and echoed back on BatchLoadTaskDescription per deserializers.go:2981's
// case "DataModelConfiguration") -- survives a real SDK client round trip
// through CreateBatchLoadTask and DescribeBatchLoadTask. Before this fix,
// gopherstack's createBatchLoadTaskInput wire struct had no
// DataModelConfiguration field at all, so a compliant client sending it
// (the common case for CreateBatchLoadTask, since it is how a caller
// specifies the CSV-to-table column mapping) would have it silently dropped:
// never stored, never echoed back on Describe.
func TestCreateBatchLoadTask_DataModelConfigurationSDKRoundTrip(t *testing.T) {
	t.Parallel()

	backend := timestreamwrite.NewInMemoryBackend()
	h := timestreamwrite.NewHandler(backend)
	client := newTestTimestreamWriteSDKClient(t, h)

	_, err := client.CreateDatabase(t.Context(), &timestreamwritesdk.CreateDatabaseInput{
		DatabaseName: aws.String("dmc-db"),
	})
	require.NoError(t, err)

	_, err = client.CreateTable(t.Context(), &timestreamwritesdk.CreateTableInput{
		DatabaseName: aws.String("dmc-db"),
		TableName:    aws.String("dmc-tbl"),
	})
	require.NoError(t, err)

	created, err := client.CreateBatchLoadTask(t.Context(), &timestreamwritesdk.CreateBatchLoadTaskInput{
		TargetDatabaseName: aws.String("dmc-db"),
		TargetTableName:    aws.String("dmc-tbl"),
		RecordVersion:      aws.Int64(3),
		DataSourceConfiguration: &twtypes.DataSourceConfiguration{
			DataFormat: twtypes.BatchLoadDataFormatCsv,
			DataSourceS3Configuration: &twtypes.DataSourceS3Configuration{
				BucketName: aws.String("dmc-source-bucket"),
			},
		},
		ReportConfiguration: &twtypes.ReportConfiguration{
			ReportS3Configuration: &twtypes.ReportS3Configuration{
				BucketName: aws.String("dmc-report-bucket"),
			},
		},
		DataModelConfiguration: &twtypes.DataModelConfiguration{
			DataModel: &twtypes.DataModel{
				TimeColumn:        aws.String("time"),
				TimeUnit:          twtypes.TimeUnitSeconds,
				MeasureNameColumn: aws.String("measure_name"),
				DimensionMappings: []twtypes.DimensionMapping{
					{SourceColumn: aws.String("region"), DestinationColumn: aws.String("region")},
				},
				MixedMeasureMappings: []twtypes.MixedMeasureMapping{
					{
						MeasureName:       aws.String("cpu"),
						SourceColumn:      aws.String("cpu_value"),
						MeasureValueType:  twtypes.MeasureValueTypeDouble,
						TargetMeasureName: aws.String("cpu"),
					},
				},
				MultiMeasureMappings: &twtypes.MultiMeasureMappings{
					TargetMultiMeasureName: aws.String("metrics"),
					MultiMeasureAttributeMappings: []twtypes.MultiMeasureAttributeMapping{
						{
							SourceColumn:                    aws.String("mem_value"),
							MeasureValueType:                twtypes.ScalarMeasureValueTypeDouble,
							TargetMultiMeasureAttributeName: aws.String("mem"),
						},
					},
				},
			},
			DataModelS3Configuration: &twtypes.DataModelS3Configuration{
				BucketName: aws.String("dmc-model-bucket"),
				ObjectKey:  aws.String("model.json"),
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, created.TaskId)

	described, err := client.DescribeBatchLoadTask(t.Context(), &timestreamwritesdk.DescribeBatchLoadTaskInput{
		TaskId: created.TaskId,
	})
	require.NoError(t, err)

	desc := described.BatchLoadTaskDescription
	require.NotNil(t, desc.DataModelConfiguration)
	require.NotNil(t, desc.DataModelConfiguration.DataModel)

	dm := desc.DataModelConfiguration.DataModel
	assert.Equal(t, "time", aws.ToString(dm.TimeColumn))
	assert.Equal(t, twtypes.TimeUnitSeconds, dm.TimeUnit)
	assert.Equal(t, "measure_name", aws.ToString(dm.MeasureNameColumn))
	require.Len(t, dm.DimensionMappings, 1)
	assert.Equal(t, "region", aws.ToString(dm.DimensionMappings[0].SourceColumn))
	assert.Equal(t, "region", aws.ToString(dm.DimensionMappings[0].DestinationColumn))

	require.Len(t, dm.MixedMeasureMappings, 1)
	assert.Equal(t, "cpu", aws.ToString(dm.MixedMeasureMappings[0].MeasureName))
	assert.Equal(t, twtypes.MeasureValueTypeDouble, dm.MixedMeasureMappings[0].MeasureValueType)

	require.NotNil(t, dm.MultiMeasureMappings)
	assert.Equal(t, "metrics", aws.ToString(dm.MultiMeasureMappings.TargetMultiMeasureName))
	require.Len(t, dm.MultiMeasureMappings.MultiMeasureAttributeMappings, 1)
	assert.Equal(t, "mem",
		aws.ToString(dm.MultiMeasureMappings.MultiMeasureAttributeMappings[0].TargetMultiMeasureAttributeName))
	assert.Equal(t, twtypes.ScalarMeasureValueTypeDouble,
		dm.MultiMeasureMappings.MultiMeasureAttributeMappings[0].MeasureValueType)

	require.NotNil(t, desc.DataModelConfiguration.DataModelS3Configuration)
	assert.Equal(t, "dmc-model-bucket", aws.ToString(desc.DataModelConfiguration.DataModelS3Configuration.BucketName))
	assert.Equal(t, "model.json", aws.ToString(desc.DataModelConfiguration.DataModelS3Configuration.ObjectKey))

	assert.Equal(t, int64(3), desc.RecordVersion)
}

// TestWriteRecords_RecordsIngestedSDKRoundTrip proves the WriteRecords
// response's RecordsIngested{Total,MemoryStore,MagneticStore} shape
// (deserializers.go's awsAwsjson10_deserializeDocumentRecordsIngested)
// and the MeasureValue (string) vs MeasureValues (list of
// name/value/type structs) member split on Record both survive a real SDK
// client round trip.
func TestWriteRecords_RecordsIngestedSDKRoundTrip(t *testing.T) {
	t.Parallel()

	backend := timestreamwrite.NewInMemoryBackend()
	h := timestreamwrite.NewHandler(backend)
	client := newTestTimestreamWriteSDKClient(t, h)

	_, err := client.CreateDatabase(t.Context(), &timestreamwritesdk.CreateDatabaseInput{
		DatabaseName: aws.String("wr-db"),
	})
	require.NoError(t, err)

	_, err = client.CreateTable(t.Context(), &timestreamwritesdk.CreateTableInput{
		DatabaseName: aws.String("wr-db"),
		TableName:    aws.String("wr-tbl"),
	})
	require.NoError(t, err)

	out, err := client.WriteRecords(t.Context(), &timestreamwritesdk.WriteRecordsInput{
		DatabaseName: aws.String("wr-db"),
		TableName:    aws.String("wr-tbl"),
		Records: []twtypes.Record{
			{
				Time:             aws.String(recentTimeSeconds()),
				TimeUnit:         twtypes.TimeUnitSeconds,
				MeasureName:      aws.String("multi"),
				MeasureValueType: twtypes.MeasureValueTypeMulti,
				MeasureValues: []twtypes.MeasureValue{
					{Name: aws.String("cpu"), Value: aws.String("42.5"), Type: twtypes.MeasureValueTypeDouble},
					{Name: aws.String("mem"), Value: aws.String("1024"), Type: twtypes.MeasureValueTypeBigint},
				},
				Dimensions: []twtypes.Dimension{{Name: aws.String("host"), Value: aws.String("a")}},
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, out.RecordsIngested)
	assert.Equal(t, int32(1), out.RecordsIngested.Total)
	assert.Equal(t, out.RecordsIngested.Total, out.RecordsIngested.MemoryStore+out.RecordsIngested.MagneticStore,
		"Total must equal MemoryStore+MagneticStore")
}

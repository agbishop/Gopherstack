package firehose_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	firehosesdk "github.com/aws/aws-sdk-go-v2/service/firehose"
	firehosetypes "github.com/aws/aws-sdk-go-v2/service/firehose/types"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/firehose"
)

// TestCreateDeliveryStream_EncryptionConfigurationRoundTrip proves
// CreateDeliveryStreamInput.DeliveryStreamEncryptionConfigurationInput (a real, accepted
// request field -- serializers.go:3818) was previously silently dropped: gopherstack's
// createDeliveryStreamInput had no field for it at all, so a real client encrypting a
// stream at creation time (rather than via a separate StartDeliveryStreamEncryption call)
// got a stream that was never actually encrypted -- DescribeDeliveryStream's
// DeliveryStreamEncryptionConfiguration stayed nil and PutRecord's Encrypted field stayed
// false, with no error anywhere in the round trip.
func TestCreateDeliveryStream_EncryptionConfigurationRoundTrip(t *testing.T) {
	t.Parallel()

	b := firehose.NewInMemoryBackend("123456789012", "us-east-1")
	h := firehose.NewHandler(b)
	client := newTestFirehoseClient(t, h)

	_, err := client.CreateDeliveryStream(t.Context(), &firehosesdk.CreateDeliveryStreamInput{
		DeliveryStreamName: aws.String("encrypted-stream"),
		ExtendedS3DestinationConfiguration: &firehosetypes.ExtendedS3DestinationConfiguration{
			BucketARN: aws.String("arn:aws:s3:::bucket"),
			RoleARN:   aws.String("arn:aws:iam::123456789012:role/r"),
		},
		DeliveryStreamEncryptionConfigurationInput: &firehosetypes.DeliveryStreamEncryptionConfigurationInput{
			KeyType: firehosetypes.KeyTypeCustomerManagedCmk,
			KeyARN:  aws.String("arn:aws:kms:us-east-1:123456789012:key/k1"),
		},
	})
	require.NoError(t, err, "real SDK client's CreateDeliveryStream request must decode without error")

	out, err := client.DescribeDeliveryStream(t.Context(), &firehosesdk.DescribeDeliveryStreamInput{
		DeliveryStreamName: aws.String("encrypted-stream"),
	})
	require.NoError(t, err)
	require.NotNil(t, out.DeliveryStreamDescription)
	require.NotNil(t, out.DeliveryStreamDescription.DeliveryStreamEncryptionConfiguration)

	enc := out.DeliveryStreamDescription.DeliveryStreamEncryptionConfiguration
	require.Equal(t, firehosetypes.DeliveryStreamEncryptionStatusEnabled, enc.Status)
	require.Equal(t, firehosetypes.KeyTypeCustomerManagedCmk, enc.KeyType)
	require.NotNil(t, enc.KeyARN)
	require.Equal(t, "arn:aws:kms:us-east-1:123456789012:key/k1", *enc.KeyARN)

	putOut, err := client.PutRecord(t.Context(), &firehosesdk.PutRecordInput{
		DeliveryStreamName: aws.String("encrypted-stream"),
		Record:             &firehosetypes.Record{Data: []byte("hello")},
	})
	require.NoError(t, err)
	require.NotNil(t, putOut.Encrypted)
	require.True(t, *putOut.Encrypted)
}

// TestCreateDeliveryStream_DirectPutSourceConfigurationRoundTrip proves
// CreateDeliveryStreamInput.DirectPutSourceConfiguration (a real, accepted request field --
// serializers.go:3822 area, types.DirectPutSourceConfiguration) was previously silently
// dropped -- gopherstack had no field for it at all, so ThroughputHintInMBs never
// round-tripped and SourceDescription.DirectPutSourceDescription was never emitted.
func TestCreateDeliveryStream_DirectPutSourceConfigurationRoundTrip(t *testing.T) {
	t.Parallel()

	b := firehose.NewInMemoryBackend("123456789012", "us-east-1")
	h := firehose.NewHandler(b)
	client := newTestFirehoseClient(t, h)

	_, err := client.CreateDeliveryStream(t.Context(), &firehosesdk.CreateDeliveryStreamInput{
		DeliveryStreamName: aws.String("direct-put-stream"),
		ExtendedS3DestinationConfiguration: &firehosetypes.ExtendedS3DestinationConfiguration{
			BucketARN: aws.String("arn:aws:s3:::bucket"),
			RoleARN:   aws.String("arn:aws:iam::123456789012:role/r"),
		},
		DirectPutSourceConfiguration: &firehosetypes.DirectPutSourceConfiguration{
			ThroughputHintInMBs: aws.Int32(15),
		},
	})
	require.NoError(t, err)

	out, err := client.DescribeDeliveryStream(t.Context(), &firehosesdk.DescribeDeliveryStreamInput{
		DeliveryStreamName: aws.String("direct-put-stream"),
	})
	require.NoError(t, err)
	require.NotNil(t, out.DeliveryStreamDescription)
	require.NotNil(t, out.DeliveryStreamDescription.Source)
	require.NotNil(t, out.DeliveryStreamDescription.Source.DirectPutSourceDescription)
	require.NotNil(t, out.DeliveryStreamDescription.Source.DirectPutSourceDescription.ThroughputHintInMBs)
	require.Equal(t, int32(15), *out.DeliveryStreamDescription.Source.DirectPutSourceDescription.ThroughputHintInMBs)
}

// TestCreateDeliveryStream_DatabaseSourceConfigurationRoundTrip proves
// CreateDeliveryStreamInput.DatabaseSourceConfiguration (a real, accepted request field --
// serializers.go:3813, types.DatabaseSourceConfiguration) was previously silently dropped
// in its entirety -- gopherstack had no field, no type, and no case for it anywhere, so
// every member (Endpoint, Port, Type, Databases/Tables/Columns include-exclude lists,
// SurrogateKeys, auth/VPC configuration) was accepted and thrown away, and
// SourceDescription.DatabaseSourceDescription was never emitted on Describe. Uses
// non-empty Include/Exclude collections on Databases/Tables/Columns/SurrogateKeys per the
// campaign's never-assert-over-an-empty-collection rule.
func TestCreateDeliveryStream_DatabaseSourceConfigurationRoundTrip(t *testing.T) {
	t.Parallel()

	b := firehose.NewInMemoryBackend("123456789012", "us-east-1")
	h := firehose.NewHandler(b)
	client := newTestFirehoseClient(t, h)

	_, err := client.CreateDeliveryStream(t.Context(), &firehosesdk.CreateDeliveryStreamInput{
		DeliveryStreamName: aws.String("db-stream"),
		DeliveryStreamType: firehosetypes.DeliveryStreamTypeDatabaseAsSource,
		DatabaseSourceConfiguration: &firehosetypes.DatabaseSourceConfiguration{
			Type:                   firehosetypes.DatabaseTypeMySQL,
			Endpoint:               aws.String("db.example.com"),
			Port:                   aws.Int32(3306),
			SSLMode:                firehosetypes.SSLModeEnabled,
			SnapshotWatermarkTable: aws.String("watermark_tbl"),
			SurrogateKeys:          []string{"id", "tenant_id"},
			Databases: &firehosetypes.DatabaseList{
				Include: []string{"prod_db"},
				Exclude: []string{"test_db"},
			},
			Tables: &firehosetypes.DatabaseTableList{
				Include: []string{"prod_db.orders", "prod_db.users"},
			},
			Columns: &firehosetypes.DatabaseColumnList{
				Exclude: []string{"prod_db.orders.internal_notes"},
			},
			DatabaseSourceAuthenticationConfiguration: &firehosetypes.DatabaseSourceAuthenticationConfiguration{
				SecretsManagerConfiguration: &firehosetypes.SecretsManagerConfiguration{
					Enabled:   aws.Bool(true),
					SecretARN: aws.String("arn:aws:secretsmanager:us-east-1:123456789012:secret:s1"),
					RoleARN:   aws.String("arn:aws:iam::123456789012:role/r"),
				},
			},
			DatabaseSourceVPCConfiguration: &firehosetypes.DatabaseSourceVPCConfiguration{
				VpcEndpointServiceName: aws.String("com.amazonaws.vpce.us-east-1.vpce-svc-1"),
			},
		},
	})
	require.NoError(t, err, "real SDK client's CreateDeliveryStream request must decode without error")

	out, err := client.DescribeDeliveryStream(t.Context(), &firehosesdk.DescribeDeliveryStreamInput{
		DeliveryStreamName: aws.String("db-stream"),
	})
	require.NoError(t, err, "real SDK client must decode DescribeDeliveryStream response without error")
	require.NotNil(t, out.DeliveryStreamDescription)
	require.NotNil(t, out.DeliveryStreamDescription.Source)

	dsd := out.DeliveryStreamDescription.Source.DatabaseSourceDescription
	require.NotNil(t, dsd, "DatabaseSourceDescription must round-trip through Describe")
	require.Equal(t, firehosetypes.DatabaseTypeMySQL, dsd.Type)
	require.NotNil(t, dsd.Endpoint)
	require.Equal(t, "db.example.com", *dsd.Endpoint)
	require.NotNil(t, dsd.Port)
	require.Equal(t, int32(3306), *dsd.Port)
	require.Equal(t, firehosetypes.SSLModeEnabled, dsd.SSLMode)
	require.ElementsMatch(t, []string{"id", "tenant_id"}, dsd.SurrogateKeys)

	require.NotNil(t, dsd.Databases)
	require.Equal(t, []string{"prod_db"}, dsd.Databases.Include)
	require.Equal(t, []string{"test_db"}, dsd.Databases.Exclude)

	require.NotNil(t, dsd.Tables)
	require.ElementsMatch(t, []string{"prod_db.orders", "prod_db.users"}, dsd.Tables.Include)

	require.NotNil(t, dsd.Columns)
	require.Equal(t, []string{"prod_db.orders.internal_notes"}, dsd.Columns.Exclude)

	require.NotNil(t, dsd.DatabaseSourceAuthenticationConfiguration)
	require.NotNil(t, dsd.DatabaseSourceAuthenticationConfiguration.SecretsManagerConfiguration)
	require.NotNil(t, dsd.DatabaseSourceAuthenticationConfiguration.SecretsManagerConfiguration.SecretARN)
	require.Equal(
		t,
		"arn:aws:secretsmanager:us-east-1:123456789012:secret:s1",
		*dsd.DatabaseSourceAuthenticationConfiguration.SecretsManagerConfiguration.SecretARN,
	)

	require.NotNil(t, dsd.DatabaseSourceVPCConfiguration)
	require.NotNil(t, dsd.DatabaseSourceVPCConfiguration.VpcEndpointServiceName)
	require.Equal(
		t,
		"com.amazonaws.vpce.us-east-1.vpce-svc-1",
		*dsd.DatabaseSourceVPCConfiguration.VpcEndpointServiceName,
	)
}

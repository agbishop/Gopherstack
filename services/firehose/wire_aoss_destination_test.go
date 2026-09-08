package firehose_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	firehosesdk "github.com/aws/aws-sdk-go-v2/service/firehose"
	firehosetypes "github.com/aws/aws-sdk-go-v2/service/firehose/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/firehose"
)

// TestCreateDeliveryStream_AmazonOpenSearchServerless_RejectedNotSilentlyDropped drives
// CreateDeliveryStream through a real aws-sdk-go-v2 firehose client using ONLY
// AmazonOpenSearchServerlessDestinationConfiguration -- a real, 11th destination-
// configuration type (types.AmazonOpenSearchServerlessDestinationConfiguration) this
// backend has no field/build path for. Before this fix, createDeliveryStreamInput had
// no field for this key at all, so json.Unmarshal silently dropped it,
// validateSingleDestination saw zero destinations configured, and the call succeeded
// with a delivery stream that has NO destination -- an accept-and-drop bug, not a
// documented gap: the client believes it configured OpenSearch Serverless delivery and
// gets no error and no indication anything is wrong. The fix rejects the request
// explicitly instead of silently creating a destination-less stream.
func TestCreateDeliveryStream_AmazonOpenSearchServerless_RejectedNotSilentlyDropped(t *testing.T) {
	t.Parallel()

	b := firehose.NewInMemoryBackend("123456789012", "us-east-1")
	h := firehose.NewHandler(b)
	client := newTestFirehoseClient(t, h)

	_, err := client.CreateDeliveryStream(t.Context(), &firehosesdk.CreateDeliveryStreamInput{
		DeliveryStreamName: aws.String("aoss-stream"),
		DeliveryStreamType: firehosetypes.DeliveryStreamTypeDirectPut,
		AmazonOpenSearchServerlessDestinationConfiguration: &firehosetypes.AmazonOpenSearchServerlessDestinationConfiguration{
			IndexName: aws.String("my-index"),
			RoleARN:   aws.String("arn:aws:iam::123456789012:role/r"),
			S3Configuration: &firehosetypes.S3DestinationConfiguration{
				BucketARN: aws.String("arn:aws:s3:::my-bucket"),
				RoleARN:   aws.String("arn:aws:iam::123456789012:role/r"),
			},
		},
	})
	require.Error(t, err, "CreateDeliveryStream must reject an unsupported"+
		" AmazonOpenSearchServerlessDestinationConfiguration, not silently create a"+
		" destination-less stream")

	// Confirm no ghost stream was left behind under either outcome.
	_, describeErr := client.DescribeDeliveryStream(
		t.Context(),
		&firehosesdk.DescribeDeliveryStreamInput{DeliveryStreamName: aws.String("aoss-stream")},
	)
	assert.Error(t, describeErr, "no delivery stream should have been created")
}

// TestUpdateDestination_AmazonOpenSearchServerless_RejectedNotConfusing drives
// UpdateDestination with only AmazonOpenSearchServerlessDestinationUpdate set (via a
// stream created with a real S3 destination first). Before this fix,
// updateDestinationInput had no field for this key, so the update silently landed as
// "zero destinations supplied" and failed with a message claiming the caller supplied
// nothing -- misleading, even though it didn't corrupt state. Now it fails with an
// explicit, accurate "not supported" message.
func TestUpdateDestination_AmazonOpenSearchServerless_RejectedNotConfusing(t *testing.T) {
	t.Parallel()

	b := firehose.NewInMemoryBackend("123456789012", "us-east-1")
	h := firehose.NewHandler(b)
	client := newTestFirehoseClient(t, h)

	createOut, err := client.CreateDeliveryStream(t.Context(), &firehosesdk.CreateDeliveryStreamInput{
		DeliveryStreamName: aws.String("aoss-update-stream"),
		DeliveryStreamType: firehosetypes.DeliveryStreamTypeDirectPut,
		ExtendedS3DestinationConfiguration: &firehosetypes.ExtendedS3DestinationConfiguration{
			BucketARN: aws.String("arn:aws:s3:::my-bucket"),
			RoleARN:   aws.String("arn:aws:iam::123456789012:role/r"),
		},
	})
	require.NoError(t, err)
	require.NotNil(t, createOut.DeliveryStreamARN)

	desc, err := client.DescribeDeliveryStream(t.Context(), &firehosesdk.DescribeDeliveryStreamInput{
		DeliveryStreamName: aws.String("aoss-update-stream"),
	})
	require.NoError(t, err)
	require.Len(t, desc.DeliveryStreamDescription.Destinations, 1)

	_, err = client.UpdateDestination(t.Context(), &firehosesdk.UpdateDestinationInput{
		DeliveryStreamName:             aws.String("aoss-update-stream"),
		CurrentDeliveryStreamVersionId: aws.String("1"),
		DestinationId:                  desc.DeliveryStreamDescription.Destinations[0].DestinationId,
		AmazonOpenSearchServerlessDestinationUpdate: &firehosetypes.AmazonOpenSearchServerlessDestinationUpdate{
			IndexName: aws.String("my-index"),
		},
	})
	require.Error(t, err, "UpdateDestination must reject an unsupported"+
		" AmazonOpenSearchServerlessDestinationUpdate")

	var apiErr interface{ ErrorMessage() string }
	if assert.ErrorAs(t, err, &apiErr) {
		assert.Contains(t, apiErr.ErrorMessage(), "not supported by this emulator")
	}
}

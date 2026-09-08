package cloudtrail_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	sdk_s3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/services/cloudtrail"
)

// fakeS3 is a minimal cloudtrail.S3Backend test double: HeadBucket errors for
// any bucket not in the buckets set, and PutObject records every call it
// receives. Standing in for a wired S3 backend without depending on
// services/s3.
type fakeS3 struct {
	buckets  map[string]bool
	putCalls []fakePutCall
}

type fakePutCall struct {
	bucket string
	key    string
	body   []byte
}

var errFakeNoSuchBucket = errors.New("NoSuchBucket")

func (f *fakeS3) HeadBucket(
	_ context.Context,
	input *sdk_s3.HeadBucketInput,
) (*sdk_s3.HeadBucketOutput, error) {
	if f.buckets[aws.ToString(input.Bucket)] {
		return &sdk_s3.HeadBucketOutput{}, nil
	}

	return nil, errFakeNoSuchBucket
}

func (f *fakeS3) PutObject(
	_ context.Context,
	input *sdk_s3.PutObjectInput,
) (*sdk_s3.PutObjectOutput, error) {
	body, _ := io.ReadAll(input.Body)
	f.putCalls = append(f.putCalls, fakePutCall{
		bucket: aws.ToString(input.Bucket),
		key:    aws.ToString(input.Key),
		body:   body,
	})

	return &sdk_s3.PutObjectOutput{}, nil
}

// TestCreateTrail_BucketValidation proves that once S3 is wired via
// SetS3Backend, CreateTrail rejects a bucket that does not exist (real AWS:
// S3BucketDoesNotExistException) and allows one that does. Fails against the
// pre-fix code: CreateTrail never called into S3 at all, so a nonexistent
// bucket was silently accepted.
func TestCreateTrail_BucketValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		bucket  string
		wantErr bool
	}{
		{name: "missing_bucket_rejected", bucket: "missing-bucket", wantErr: true},
		{name: "existing_bucket_allowed", bucket: "good-bucket", wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := cloudtrail.NewInMemoryBackend("123456789012", config.DefaultRegion)
			b.SetS3Backend(&fakeS3{buckets: map[string]bool{"good-bucket": true}})

			trail, err := b.CreateTrail(
				"t1",
				tt.bucket,
				"",
				"",
				"",
				"",
				"",
				false,
				false,
				false,
				nil,
			)

			if tt.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, cloudtrail.ErrS3BucketNotFound)
				assert.Nil(t, trail)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.bucket, trail.S3BucketName)
		})
	}
}

// TestUpdateTrail_BucketValidation proves UpdateTrail applies the same
// bucket-existence check as CreateTrail (real AWS: UpdateTrail's error model
// also declares S3BucketDoesNotExistException), and that a rejected update
// does not mutate the trail's existing bucket.
func TestUpdateTrail_BucketValidation(t *testing.T) {
	t.Parallel()

	b := cloudtrail.NewInMemoryBackend("123456789012", config.DefaultRegion)
	b.SetS3Backend(&fakeS3{buckets: map[string]bool{"good-bucket": true}})

	_, err := b.CreateTrail("t1", "good-bucket", "", "", "", "", "", false, false, false, nil)
	require.NoError(t, err)

	updated, err := b.UpdateTrail("t1", "missing-bucket", "", "", "", "", "", nil, nil, nil)
	require.Error(t, err)
	require.ErrorIs(t, err, cloudtrail.ErrS3BucketNotFound)
	assert.Nil(t, updated)

	trail, err := b.GetTrail("t1")
	require.NoError(t, err)
	assert.Equal(
		t,
		"good-bucket",
		trail.S3BucketName,
		"rejected update must not mutate the trail's bucket",
	)
}

// TestCreateTrail_UnwiredS3StaysPermissive and TestUpdateTrail_UnwiredS3StaysPermissive
// prove the unwired path stays permissive: with no SetS3Backend call, a
// nonexistent bucket must still be accepted, matching this repo's
// unwired-hook-stays-permissive convention (roughly 150 services construct
// backends in tests with no cross-service hooks wired).
func TestCreateTrail_UnwiredS3StaysPermissive(t *testing.T) {
	t.Parallel()

	b := cloudtrail.NewInMemoryBackend("123456789012", config.DefaultRegion)

	trail, err := b.CreateTrail(
		"t1",
		"nonexistent-bucket",
		"",
		"",
		"",
		"",
		"",
		false,
		false,
		false,
		nil,
	)
	require.NoError(t, err)
	assert.Equal(t, "nonexistent-bucket", trail.S3BucketName)
}

func TestUpdateTrail_UnwiredS3StaysPermissive(t *testing.T) {
	t.Parallel()

	b := cloudtrail.NewInMemoryBackend("123456789012", config.DefaultRegion)

	_, err := b.CreateTrail("t1", "bucket-a", "", "", "", "", "", false, false, false, nil)
	require.NoError(t, err)

	updated, err := b.UpdateTrail("t1", "nonexistent-bucket", "", "", "", "", "", nil, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "nonexistent-bucket", updated.S3BucketName)
}

// TestHandleCreateTrail_S3BucketDoesNotExist verifies the wire shape of a
// rejected CreateTrail: __type "S3BucketDoesNotExistException" (the real
// CreateTrailOutput error, confirmed against cloudtrail@v1.58.4's
// awsAwsjson11_deserializeOpErrorCreateTrail switch and the doc comment on
// types.S3BucketDoesNotExistException, "This exception is thrown when the
// specified S3 bucket does not exist.").
func TestHandleCreateTrail_S3BucketDoesNotExist(t *testing.T) {
	t.Parallel()

	b := cloudtrail.NewInMemoryBackend("123456789012", config.DefaultRegion)
	b.SetS3Backend(&fakeS3{buckets: map[string]bool{}})
	h := cloudtrail.NewHandler(b)

	rec := doCloudTrailOp(t, h, "CreateTrail", map[string]any{
		"Name":         "my-trail",
		"S3BucketName": "missing-bucket",
	})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	resp := parseCloudTrailResp(t, rec)
	assert.Equal(t, "S3BucketDoesNotExistException", resp["__type"])
}

// TestRecordManagementEvent_DeliversLogFileToS3 proves that once S3 is wired,
// a management event recorded while a trail is logging is actually written
// as a CloudTrail log file to that trail's bucket, instead of S3BucketName
// being stored/echoed with no delivery at all. Fails against the pre-fix
// code: RecordEvent never called into S3, so fake.putCalls stays empty.
func TestRecordManagementEvent_DeliversLogFileToS3(t *testing.T) {
	t.Parallel()

	b := cloudtrail.NewInMemoryBackend("123456789012", "us-east-1")
	fake := &fakeS3{buckets: map[string]bool{"good-bucket": true}}
	b.SetS3Backend(fake)

	_, err := b.CreateTrail("t1", "good-bucket", "", "", "", "", "", false, false, false, nil)
	require.NoError(t, err)
	require.NoError(t, b.StartLogging("t1"))

	b.RecordManagementEvent(service.CloudTrailEventInput{
		EventName:   "CreateBucket",
		EventSource: "s3.amazonaws.com",
		AwsRegion:   "us-east-1",
	})

	require.Len(t, fake.putCalls, 1)
	call := fake.putCalls[0]
	assert.Equal(t, "good-bucket", call.bucket)

	// AWSLogs/{account}/CloudTrail/{region}/{yyyy}/{mm}/{dd}/{account}_CloudTrail_
	// {region}_{yyyyMMddTHHmmZ}_{unique}.json.gz -- documentation-sourced key
	// layout, see PARITY.md.
	keyPattern := regexp.MustCompile(
		`^AWSLogs/123456789012/CloudTrail/us-east-1/\d{4}/\d{2}/\d{2}/` +
			`123456789012_CloudTrail_us-east-1_\d{8}T\d{4}Z_[0-9a-f]{16}\.json\.gz$`,
	)
	assert.Regexp(t, keyPattern, call.key)

	gz, err := gzip.NewReader(bytes.NewReader(call.body))
	require.NoError(t, err)

	decoded, err := io.ReadAll(gz)
	require.NoError(t, err)

	var records struct {
		Records []map[string]any `json:"Records"`
	}
	require.NoError(t, json.Unmarshal(decoded, &records))
	require.Len(t, records.Records, 1)
	assert.Equal(t, "CreateBucket", records.Records[0]["eventName"])
}

// TestRecordManagementEvent_TrailNotLogging_NoDelivery proves a trail that
// exists but has never called StartLogging receives no log files, matching
// real AWS (a trail only delivers while logging).
func TestRecordManagementEvent_TrailNotLogging_NoDelivery(t *testing.T) {
	t.Parallel()

	b := cloudtrail.NewInMemoryBackend("123456789012", "us-east-1")
	fake := &fakeS3{buckets: map[string]bool{"good-bucket": true}}
	b.SetS3Backend(fake)

	_, err := b.CreateTrail("t1", "good-bucket", "", "", "", "", "", false, false, false, nil)
	require.NoError(t, err)

	b.RecordManagementEvent(
		service.CloudTrailEventInput{EventName: "CreateBucket", EventSource: "s3.amazonaws.com"},
	)

	assert.Empty(t, fake.putCalls)
}

// TestRecordManagementEvent_UnwiredS3StaysPermissive proves the unwired
// delivery path stays permissive: with no SetS3Backend call, recording an
// event against a logging trail must not panic or error, and the event
// remains available via LookupEvents exactly as before this change.
func TestRecordManagementEvent_UnwiredS3StaysPermissive(t *testing.T) {
	t.Parallel()

	b := cloudtrail.NewInMemoryBackend("123456789012", "us-east-1")

	_, err := b.CreateTrail("t1", "good-bucket", "", "", "", "", "", false, false, false, nil)
	require.NoError(t, err)
	require.NoError(t, b.StartLogging("t1"))

	require.NotPanics(t, func() {
		b.RecordManagementEvent(
			service.CloudTrailEventInput{
				EventName:   "CreateBucket",
				EventSource: "s3.amazonaws.com",
			},
		)
	})

	out := b.LookupEvents(cloudtrail.LookupEventsInput{})
	assert.Len(t, out.Events, 1)
}

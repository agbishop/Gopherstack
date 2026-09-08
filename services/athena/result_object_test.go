package athena_test

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sdk_s3 "github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/blackbirdworks/gopherstack/services/athena"
)

// fakeS3Storer is a minimal athena.S3Storer test double capturing PutObject calls,
// standing in for a wired S3 backend without depending on services/s3.
type fakeS3Storer struct {
	bucket string
	key    string
	sse    string
	body   []byte
	calls  int
}

func (f *fakeS3Storer) PutObject(_ context.Context, input *sdk_s3.PutObjectInput) (*sdk_s3.PutObjectOutput, error) {
	f.calls++
	f.bucket = *input.Bucket
	f.key = *input.Key
	f.sse = string(input.ServerSideEncryption)

	if input.Body != nil {
		f.body, _ = io.ReadAll(input.Body)
	}

	return &sdk_s3.PutObjectOutput{}, nil
}

// TestStartQueryExecution_WritesResultObjectToWiredS3 proves that once S3 is
// wired via SetS3Backend, a succeeded query execution with
// ResultConfiguration.OutputLocation set actually writes a result object,
// instead of only storing/echoing the configuration. This fails against the
// pre-fix code: finalizeExecution never called into S3 at all, so a client
// that ran a query and then fetched the result file from the stated
// OutputLocation found nothing.
func TestStartQueryExecution_WritesResultObjectToWiredS3(t *testing.T) {
	t.Parallel()

	b := athena.NewInMemoryBackend("us-east-1", "123456789012")

	s3 := &fakeS3Storer{}
	b.SetS3Backend(s3)

	b.InsertRows("AwsDataCatalog", "default", "sample_table", []map[string]any{
		{"id": 1, "value": "a"},
	})

	id, err := b.StartQueryExecution(
		"SELECT * FROM sample_table", "primary",
		athena.QueryExecutionContext{Database: "default", Catalog: "AwsDataCatalog"},
		athena.ResultConfiguration{
			OutputLocation: "s3://my-bucket/results/",
			EncryptionConfiguration: athena.EncryptionConfiguration{
				EncryptionOption: "SSE_S3",
			},
		},
		nil, nil,
	)
	require.NoError(t, err)
	require.NotEmpty(t, id)

	qe, err := b.GetQueryExecution(id)
	require.NoError(t, err)
	require.Equal(t, "SUCCEEDED", qe.Status.State)

	require.Equal(t, 1, s3.calls)
	assert.Equal(t, "my-bucket", s3.bucket)
	assert.Equal(t, "results/"+id+".csv", s3.key)
	assert.Equal(t, "AES256", s3.sse)
	assert.Contains(t, string(s3.body), "id,value")
	assert.Contains(t, string(s3.body), "1,a")
}

// TestStartQueryExecution_UnwiredS3StaysPermissive proves the unwired path
// stays permissive: with no SetS3Backend call, a query execution with
// OutputLocation set must still succeed (result stored/echoed as before)
// rather than erroring for lack of an S3 backend.
func TestStartQueryExecution_UnwiredS3StaysPermissive(t *testing.T) {
	t.Parallel()

	b := athena.NewInMemoryBackend("us-east-1", "123456789012")

	id, err := b.StartQueryExecution(
		"SELECT * FROM sample_table", "primary",
		athena.QueryExecutionContext{Database: "default", Catalog: "AwsDataCatalog"},
		athena.ResultConfiguration{OutputLocation: "s3://my-bucket/results/"},
		nil, nil,
	)
	require.NoError(t, err)
	require.NotEmpty(t, id)

	qe, err := b.GetQueryExecution(id)
	require.NoError(t, err)
	assert.Equal(t, "SUCCEEDED", qe.Status.State)
	assert.Equal(t, "s3://my-bucket/results/", qe.ResultConfiguration.OutputLocation)
}

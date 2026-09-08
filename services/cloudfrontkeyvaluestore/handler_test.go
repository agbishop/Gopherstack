package cloudfrontkeyvaluestore_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	cfkvssdk "github.com/aws/aws-sdk-go-v2/service/cloudfrontkeyvaluestore"
	"github.com/aws/aws-sdk-go-v2/service/cloudfrontkeyvaluestore/types"
	"github.com/aws/smithy-go"
	smithyendpoints "github.com/aws/smithy-go/endpoints"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/services/cloudfront"
	"github.com/blackbirdworks/gopherstack/services/cloudfrontkeyvaluestore"
)

// staticEndpointResolver pins every request to a fixed URL, bypassing the
// real SDK's endpoint ruleset. That ruleset derives a per-account-ID virtual
// host from the KvsARN input (see endpoints.go's EndpointParameters.KvsARN),
// which BaseEndpoint alone does not suppress -- without this override, tests
// fail with a DNS lookup on "<accountID>.127.0.0.1", not gopherstack.
type staticEndpointResolver struct{ url string }

func (r staticEndpointResolver) ResolveEndpoint(
	_ context.Context, _ cfkvssdk.EndpointParameters,
) (smithyendpoints.Endpoint, error) {
	u, err := url.Parse(r.url)
	if err != nil {
		return smithyendpoints.Endpoint{}, err
	}

	return smithyendpoints.Endpoint{URI: *u}, nil
}

// newTestHandler creates a fresh cloudfrontkeyvaluestore.Handler wired to a
// fresh cloudfront.InMemoryBackend, mirroring how cli.go's
// wireCloudFrontKeyValueStore wires the real two services together.
func newTestHandler(t *testing.T) (*cloudfrontkeyvaluestore.Handler, *cloudfront.InMemoryBackend) {
	t.Helper()

	backend := cloudfront.NewInMemoryBackend(t.Context(), "123456789012", "us-east-1")

	return cloudfrontkeyvaluestore.NewHandler(backend), backend
}

// newSDKClient starts an Echo server backed by the given handler and returns a
// CloudFront KeyValueStore SDK client pointed at it.
func newSDKClient(t *testing.T, h *cloudfrontkeyvaluestore.Handler) *cfkvssdk.Client {
	t.Helper()

	e := echo.New()
	registry := service.NewRegistry()
	require.NoError(t, registry.Register(h))
	e.Use(service.NewServiceRouter(registry).RouteHandler())

	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)

	cfg, err := awscfg.LoadDefaultConfig(
		context.Background(),
		awscfg.WithRegion("us-east-1"),
		awscfg.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err)

	return cfkvssdk.NewFromConfig(cfg, func(o *cfkvssdk.Options) {
		o.BaseEndpoint = aws.String(srv.URL)
		o.EndpointResolverV2 = staticEndpointResolver{url: srv.URL}
	})
}

// TestSDKClient_KeyLifecycle drives Put/Get/List/Update/Delete through the real
// AWS SDK v2 cloudfrontkeyvaluestore client end to end.
func TestSDKClient_KeyLifecycle(t *testing.T) {
	t.Parallel()

	h, backend := newTestHandler(t)
	kvs, err := backend.CreateKeyValueStore("lifecycle-kvs", "", nil)
	require.NoError(t, err)

	client := newSDKClient(t, h)
	ctx := context.Background()

	// A real client always obtains the current data-plane ETag via
	// DescribeKeyValueStore before its first mutation -- PutKeyInput.IfMatch
	// is a required field.
	describeOut, err := client.DescribeKeyValueStore(ctx, &cfkvssdk.DescribeKeyValueStoreInput{
		KvsARN: aws.String(kvs.ARN),
	})
	require.NoError(t, err)

	putOut, err := client.PutKey(ctx, &cfkvssdk.PutKeyInput{
		KvsARN:  aws.String(kvs.ARN),
		Key:     aws.String("greeting"),
		Value:   aws.String("hello"),
		IfMatch: describeOut.ETag,
	})
	require.NoError(t, err)
	require.NotNil(t, putOut.ETag)
	assert.NotEmpty(t, *putOut.ETag)
	require.NotNil(t, putOut.ItemCount)
	assert.Equal(t, int32(1), *putOut.ItemCount)
	require.NotNil(t, putOut.TotalSizeInBytes)
	assert.Positive(t, *putOut.TotalSizeInBytes)

	getOut, err := client.GetKey(ctx, &cfkvssdk.GetKeyInput{
		KvsARN: aws.String(kvs.ARN),
		Key:    aws.String("greeting"),
	})
	require.NoError(t, err)
	require.NotNil(t, getOut.Value)
	assert.Equal(t, "hello", *getOut.Value)
	require.NotNil(t, getOut.ItemCount)
	assert.Equal(t, int32(1), *getOut.ItemCount)

	putOut2, err := client.PutKey(ctx, &cfkvssdk.PutKeyInput{
		KvsARN:  aws.String(kvs.ARN),
		Key:     aws.String("other"),
		Value:   aws.String("value2"),
		IfMatch: putOut.ETag,
	})
	require.NoError(t, err)

	listOut, err := client.ListKeys(ctx, &cfkvssdk.ListKeysInput{KvsARN: aws.String(kvs.ARN)})
	require.NoError(t, err)
	assert.Len(t, listOut.Items, 2)

	updOut, err := client.UpdateKeys(ctx, &cfkvssdk.UpdateKeysInput{
		KvsARN:  aws.String(kvs.ARN),
		IfMatch: putOut2.ETag,
		Puts: []types.PutKeyRequestListItem{
			{Key: aws.String("third"), Value: aws.String("value3")},
		},
		Deletes: []types.DeleteKeyRequestListItem{
			{Key: aws.String("other")},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, updOut.ItemCount)
	assert.Equal(t, int32(2), *updOut.ItemCount)

	delOut, err := client.DeleteKey(ctx, &cfkvssdk.DeleteKeyInput{
		KvsARN:  aws.String(kvs.ARN),
		Key:     aws.String("greeting"),
		IfMatch: updOut.ETag,
	})
	require.NoError(t, err)
	require.NotNil(t, delOut.ItemCount)
	assert.Equal(t, int32(1), *delOut.ItemCount)

	_, err = client.GetKey(ctx, &cfkvssdk.GetKeyInput{
		KvsARN: aws.String(kvs.ARN),
		Key:    aws.String("greeting"),
	})
	require.Error(t, err)

	var apiErr smithy.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, "ResourceNotFoundException", apiErr.ErrorCode())
}

// TestSDKClient_DescribeKeyValueStore verifies the store-metadata read via the
// real SDK client, including the epoch-seconds timestamp fields.
func TestSDKClient_DescribeKeyValueStore(t *testing.T) {
	t.Parallel()

	h, backend := newTestHandler(t)
	kvs, err := backend.CreateKeyValueStore("describe-kvs", "", nil)
	require.NoError(t, err)

	client := newSDKClient(t, h)
	ctx := context.Background()

	out, err := client.DescribeKeyValueStore(ctx, &cfkvssdk.DescribeKeyValueStoreInput{
		KvsARN: aws.String(kvs.ARN),
	})
	require.NoError(t, err)
	require.NotNil(t, out.ETag)
	assert.NotEmpty(t, *out.ETag)
	require.NotNil(t, out.KvsARN)
	assert.Equal(t, kvs.ARN, *out.KvsARN)
	require.NotNil(t, out.Status)
	assert.Equal(t, "READY", *out.Status)
	require.NotNil(t, out.Created)
	assert.False(t, out.Created.IsZero())
	require.NotNil(t, out.ItemCount)
	assert.Equal(t, int32(0), *out.ItemCount)
}

// TestSDKClient_ConflictOnETagMismatch verifies that a stale If-Match surfaces
// as ConflictException (409), not the HTTP 412 the removed
// services/cloudfront data-plane handlers this package replaces used to send.
func TestSDKClient_ConflictOnETagMismatch(t *testing.T) {
	t.Parallel()

	h, backend := newTestHandler(t)
	kvs, err := backend.CreateKeyValueStore("conflict-kvs", "", nil)
	require.NoError(t, err)

	client := newSDKClient(t, h)
	ctx := context.Background()

	_, err = client.PutKey(ctx, &cfkvssdk.PutKeyInput{
		KvsARN:  aws.String(kvs.ARN),
		Key:     aws.String("k"),
		Value:   aws.String("v"),
		IfMatch: aws.String("not-the-real-etag"),
	})
	require.Error(t, err)

	var apiErr smithy.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, "ConflictException", apiErr.ErrorCode())
}

// TestSDKClient_UnknownStore verifies operations against an ARN with no
// backing KeyValueStore surface as ResourceNotFoundException.
func TestSDKClient_UnknownStore(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t)
	client := newSDKClient(t, h)

	_, err := client.GetKey(context.Background(), &cfkvssdk.GetKeyInput{
		KvsARN: aws.String("arn:aws:cloudfront::123456789012:key-value-store/does-not-exist"),
		Key:    aws.String("k"),
	})
	require.Error(t, err)

	var apiErr smithy.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, "ResourceNotFoundException", apiErr.ErrorCode())
}

// TestSDKClient_ListKeysPagination verifies MaxResults/NextToken pagination.
func TestSDKClient_ListKeysPagination(t *testing.T) {
	t.Parallel()

	h, backend := newTestHandler(t)
	kvs, err := backend.CreateKeyValueStore("paginated-kvs", "", nil)
	require.NoError(t, err)

	for i := range 5 {
		_, putErr := backend.PutKVSValue(kvs.ID, string(rune('a'+i)), "v", "")
		require.NoError(t, putErr)
	}

	client := newSDKClient(t, h)
	ctx := context.Background()

	page1, err := client.ListKeys(ctx, &cfkvssdk.ListKeysInput{
		KvsARN:     aws.String(kvs.ARN),
		MaxResults: aws.Int32(2),
	})
	require.NoError(t, err)
	assert.Len(t, page1.Items, 2)
	require.NotNil(t, page1.NextToken)

	page2, err := client.ListKeys(ctx, &cfkvssdk.ListKeysInput{
		KvsARN:     aws.String(kvs.ARN),
		MaxResults: aws.Int32(2),
		NextToken:  page1.NextToken,
	})
	require.NoError(t, err)
	assert.Len(t, page2.Items, 2)
}

// TestHandler_MutationsRequireIfMatch verifies that PutKey, DeleteKey, and
// UpdateKeys all reject a request with no If-Match header as
// ValidationException, matching IfMatch's "This member is required" doc
// comment on all three inputs (validators.go's
// validateOp{PutKey,DeleteKey,UpdateKeys}Input in
// cloudfrontkeyvaluestore@v1.15.4). The real SDK client validates this
// locally and never sends such a request, so this drives the handler
// directly with a raw HTTP request instead of going through the SDK client.
func TestHandler_MutationsRequireIfMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path   func(arn string) string
		name   string
		method string
		body   string
	}{
		{
			name:   "put key",
			method: http.MethodPut,
			path:   func(arn string) string { return "/key-value-stores/" + url.PathEscape(arn) + "/keys/k1" },
			body:   `{"Value":"v1"}`,
		},
		{
			name:   "delete key",
			method: http.MethodDelete,
			path:   func(arn string) string { return "/key-value-stores/" + url.PathEscape(arn) + "/keys/k1" },
		},
		{
			name:   "update keys",
			method: http.MethodPost,
			path:   func(arn string) string { return "/key-value-stores/" + url.PathEscape(arn) + "/keys" },
			body:   `{"Puts":[{"Key":"k1","Value":"v1"}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, backend := newTestHandler(t)
			kvs, err := backend.CreateKeyValueStore(tt.name+"-kvs", "", nil)
			require.NoError(t, err)

			e := echo.New()
			req := httptest.NewRequest(tt.method, tt.path(kvs.ARN), strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			require.NoError(t, h.Handler()(c))

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Equal(t, "ValidationException", rec.Header().Get("X-Amzn-Errortype"))
			assert.Contains(t, rec.Body.String(), "IfMatch")
		})
	}
}

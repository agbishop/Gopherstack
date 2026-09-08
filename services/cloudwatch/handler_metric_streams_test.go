package cloudwatch_test

import (
	"encoding/xml"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudwatch"
)

// TestHandler_DeleteMetricStream_CleansUpTags previously only asserted the
// PUT/DELETE HTTP status codes and never called TagResource or
// ListTagsForResource, so it never actually exercised the tag-cleanup claim
// in its own name. Strengthened to tag the stream by its real (region+account
// qualified) ARN, as a real client would, and assert the tag is gone after
// delete -- this is the exact ARN handleDeleteMetricStream must match to
// clean up the right map entry.
func TestHandler_DeleteMetricStream_CleansUpTags(t *testing.T) {
	t.Parallel()

	h := newCWHandler()

	const streamBody = "Action=PutMetricStream&Name=my-stream" +
		"&FirehoseArn=arn:aws:firehose:us-east-1:123:deliverystream/ds" +
		"&RoleArn=arn:aws:iam::123:role/r&OutputFormat=json"
	rec := postForm(t, h, streamBody)
	require.Equal(t, 200, rec.Code)

	var putResp struct {
		Result struct {
			Arn string `xml:"Arn"`
		} `xml:"PutMetricStreamResult"`
	}
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &putResp))
	streamARN := putResp.Result.Arn
	require.NotEmpty(t, streamARN)

	rec = postForm(t, h, "Action=TagResource&ResourceARN="+url.QueryEscape(streamARN)+
		"&Tags.member.1.Key=env&Tags.member.1.Value=prod")
	require.Equal(t, 200, rec.Code, "tag stream: %s", rec.Body.String())

	rec = postForm(t, h, "Action=ListTagsForResource&ResourceARN="+url.QueryEscape(streamARN))
	require.Equal(t, 200, rec.Code)
	require.Contains(t, rec.Body.String(), "prod", "tag should be visible before delete")

	rec = postForm(t, h, "Action=DeleteMetricStream&Name=my-stream")
	assert.Equal(t, 200, rec.Code, "delete stream: %s", rec.Body.String())

	rec = postForm(t, h, "Action=ListTagsForResource&ResourceARN="+url.QueryEscape(streamARN))
	require.Equal(t, 200, rec.Code)
	assert.NotContains(t, rec.Body.String(), "prod",
		"tags for the deleted metric stream's real ARN must not survive delete (ghost row)")
}

// ---------------------------------------------------------------------------
// MetricStream filters via handler (gap #8)
// ---------------------------------------------------------------------------

func TestHandler_PutMetricStream_WithIncludeFilters(t *testing.T) {
	t.Parallel()

	h := newCWHandler()
	rec := postForm(
		t, h,
		"Action=PutMetricStream"+
			"&Name=filtered-stream"+
			"&FirehoseArn=arn:aws:firehose:us-east-1:123:deliverystream/ds"+
			"&RoleArn=arn:aws:iam::123:role/r"+
			"&OutputFormat=json"+
			"&IncludeFilters.member.1.Namespace=AWS/EC2",
	)
	assert.Equal(t, 200, rec.Code, "stream with IncludeFilters: %s", rec.Body.String())
}

func TestHandler_PutMetricStream_WithExcludeFilters(t *testing.T) {
	t.Parallel()

	h := newCWHandler()
	rec := postForm(
		t, h,
		"Action=PutMetricStream"+
			"&Name=exclude-stream"+
			"&FirehoseArn=arn:aws:firehose:us-east-1:123:deliverystream/ds"+
			"&RoleArn=arn:aws:iam::123:role/r"+
			"&OutputFormat=json"+
			"&ExcludeFilters.member.1.Namespace=AWS/EC2",
	)
	assert.Equal(t, 200, rec.Code, "stream with ExcludeFilters: %s", rec.Body.String())
}

func TestHandler_MetricStream_FullCycle(t *testing.T) {
	t.Parallel()

	h := newCWHandler()

	const streamBody = "Action=PutMetricStream&Name=s1" +
		"&FirehoseArn=arn:aws:firehose:us-east-1:123:deliverystream/ds" +
		"&RoleArn=arn:aws:iam::123:role/r&OutputFormat=json"

	rec := postForm(t, h, streamBody)
	assert.Equal(t, 200, rec.Code)

	rec = postForm(t, h, "Action=GetMetricStream&Name=s1")
	assert.Equal(t, 200, rec.Code)
	assert.Contains(t, rec.Body.String(), "s1")

	rec = postForm(t, h, "Action=ListMetricStreams")
	assert.Equal(t, 200, rec.Code)

	rec = postForm(t, h, "Action=StartMetricStreams&Names.member.1=s1")
	assert.Equal(t, 200, rec.Code)

	rec = postForm(t, h, "Action=StopMetricStreams&Names.member.1=s1")
	assert.Equal(t, 200, rec.Code)

	rec = postForm(t, h, "Action=DeleteMetricStream&Name=s1")
	assert.Equal(t, 200, rec.Code)
}
func TestCW_MetricStreams(t *testing.T) {
	t.Parallel()

	h := newCWHandler()

	// StartMetricStreams
	rec := postForm(t, h, url.Values{
		"Action":         []string{"StartMetricStreams"},
		"Names.member.1": []string{"my-stream"},
	}.Encode())
	assert.Equal(t, 200, rec.Code)

	// StopMetricStreams
	rec = postForm(t, h, url.Values{
		"Action":         []string{"StopMetricStreams"},
		"Names.member.1": []string{"my-stream"},
	}.Encode())
	assert.Equal(t, 200, rec.Code)
}

func TestCloudWatchHandler_MetricStream(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup           func(t *testing.T, h *cloudwatch.Handler, b *cloudwatch.InMemoryBackend)
		name            string
		body            string
		wantContains    []string
		wantNotContains []string
		wantCode        int
	}{
		{
			name: "PutMetricStream/success",
			body: "Action=PutMetricStream&Name=my-stream" +
				"&FirehoseArn=arn%3Aaws%3Afirehose%3Aus-east-1%3A123456789012%3Adeliverystream%2Fmain" +
				"&RoleArn=arn%3Aaws%3Aiam%3A%3A123456789012%3Arole%2Fstream-role" +
				"&OutputFormat=json",
			wantCode:     http.StatusOK,
			wantContains: []string{"PutMetricStreamResponse"},
		},
		{
			name:     "PutMetricStream/missing name",
			body:     "Action=PutMetricStream",
			wantCode: http.StatusBadRequest,
		},
		{
			// Real CloudWatch has no UpdateMetricStream operation: PutMetricStream
			// is documented as create-or-update ("If you are updating a metric
			// stream, specify the name of that stream here"). Re-PUTting an
			// existing stream name must update it in place.
			name: "PutMetricStream/updates existing",
			setup: func(t *testing.T, _ *cloudwatch.Handler, b *cloudwatch.InMemoryBackend) {
				t.Helper()
				b.PutMetricStreamInternal(&cloudwatch.MetricStream{
					Name:         "my-stream",
					FirehoseArn:  "arn:aws:firehose:us-east-1:123456789012:deliverystream/main",
					RoleArn:      "arn:aws:iam::123456789012:role/stream-role",
					OutputFormat: "json",
				})
			},
			body: "Action=PutMetricStream&Name=my-stream" +
				"&FirehoseArn=arn%3Aaws%3Afirehose%3Aus-east-1%3A123456789012%3Adeliverystream%2Fmain" +
				"&RoleArn=arn%3Aaws%3Aiam%3A%3A123456789012%3Arole%2Fstream-role" +
				"&OutputFormat=opentelemetry0.7",
			wantCode:     http.StatusOK,
			wantContains: []string{"PutMetricStreamResponse"},
		},
		{
			name: "DeleteMetricStream/success",
			setup: func(t *testing.T, _ *cloudwatch.Handler, b *cloudwatch.InMemoryBackend) {
				t.Helper()
				b.PutMetricStreamInternal(&cloudwatch.MetricStream{Name: "my-stream"})
			},
			body:         "Action=DeleteMetricStream&Name=my-stream",
			wantCode:     http.StatusOK,
			wantContains: []string{"DeleteMetricStreamResponse"},
		},
		{
			name:     "DeleteMetricStream/not found",
			body:     "Action=DeleteMetricStream&Name=ghost-stream",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "DeleteMetricStream/missing name",
			body:     "Action=DeleteMetricStream",
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, b := newCWHandlerWithBackend()
			if tt.setup != nil {
				tt.setup(t, h, b)
			}

			rec := postForm(t, h, tt.body)

			assert.Equal(t, tt.wantCode, rec.Code)
			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
			for _, s := range tt.wantNotContains {
				assert.NotContains(t, rec.Body.String(), s)
			}
		})
	}
}

func TestCloudWatchHandler_ListMetricStreams(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackend()
	b.PutMetricStreamInternal(&cloudwatch.MetricStream{Name: "stream1", State: "running", OutputFormat: "json"})
	b.PutMetricStreamInternal(&cloudwatch.MetricStream{Name: "stream2", State: "running", OutputFormat: "json"})
	h := cloudwatch.NewHandler(b)

	rec := postForm(t, h, "Action=ListMetricStreams")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "stream1")
	assert.Contains(t, rec.Body.String(), "stream2")
}

func TestCloudWatchHandler_GetMetricStream(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackend()
	b.PutMetricStreamInternal(&cloudwatch.MetricStream{Name: "mystream", State: "running", OutputFormat: "json"})
	h := cloudwatch.NewHandler(b)

	rec := postForm(t, h, "Action=GetMetricStream&Name=mystream")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "mystream")
}

func TestCloudWatchHandler_GetMetricStream_NotFound(t *testing.T) {
	t.Parallel()
	h := newCWHandler()
	rec := postForm(t, h, "Action=GetMetricStream&Name=nonexistent")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCloudWatchHandler_PutMetricStream_WithFilters(t *testing.T) {
	t.Parallel()

	h := newCWHandler()

	// IncludeFilters and ExcludeFilters are mutually exclusive in real
	// CloudWatch ("You cannot include ExcludeFilters and IncludeFilters in the
	// same operation" -- PutMetricStreamInput doc comment), so this only sets
	// IncludeFilters; TestHandler_PutMetricStream_WithExcludeFilters covers the
	// ExcludeFilters-only case separately.
	body := strings.Join([]string{
		"Action=PutMetricStream",
		"Name=filtered-stream",
		"FirehoseArn=arn%3Aaws%3Afirehose%3Aus-east-1%3A123%3Adeliverystream%2Fs",
		"RoleArn=arn%3Aaws%3Aiam%3A%3A123%3Arole%2Frole",
		"OutputFormat=json",
		"IncludeFilters.member.1.Namespace=AWS%2FEC2",
		"IncludeFilters.member.1.MetricNames.member.1=CPUUtilization",
	}, "&")
	rec := postForm(t, h, body)
	require.Equal(t, http.StatusOK, rec.Code)

	// Retrieve and verify the filters were persisted.
	b := h.Backend.(*cloudwatch.InMemoryBackend)
	stream, err := b.GetMetricStream("filtered-stream")
	require.NoError(t, err)
	require.Len(t, stream.IncludeFilters, 1)
	assert.Equal(t, "AWS/EC2", stream.IncludeFilters[0].Namespace)
	assert.Equal(t, []string{"CPUUtilization"}, stream.IncludeFilters[0].MetricNames)
}

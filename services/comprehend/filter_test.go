package comprehend_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- List*Jobs Filter support ---
//
// Every job family's real Filter type (JobFilter/SentimentDetectionJobFilter/
// ...) shares the same JobName/JobStatus/SubmitTimeBefore/SubmitTimeAfter
// shape; see matchesJobFilter's doc comment in handler_jobs.go.

func TestListJobsFilterByName(t *testing.T) {
	t.Parallel()

	h := newHandler()
	request(t, h, "StartSentimentDetectionJob", map[string]any{"JobName": "alpha", "LanguageCode": "en"})
	request(t, h, "StartSentimentDetectionJob", map[string]any{"JobName": "beta", "LanguageCode": "en"})

	out := request(t, h, "ListSentimentDetectionJobs", map[string]any{
		"Filter": map[string]any{"JobName": "alpha"},
	})
	jobs, ok := out["SentimentDetectionJobPropertiesList"].([]any)
	require.True(t, ok)
	require.Len(t, jobs, 1)
	assert.Equal(t, "alpha", jobs[0].(map[string]any)["JobName"])
}

func TestListJobsFilterByStatus(t *testing.T) {
	t.Parallel()

	h := newHandler()
	started := request(t, h, "StartSentimentDetectionJob", map[string]any{
		"JobName": "advances", "LanguageCode": "en",
	})
	request(t, h, "StartSentimentDetectionJob", map[string]any{"JobName": "stays-submitted", "LanguageCode": "en"})

	// Advance the first job to IN_PROGRESS via Describe; the second stays SUBMITTED.
	request(t, h, "DescribeSentimentDetectionJob", map[string]any{"JobId": started["JobId"]})

	out := request(t, h, "ListSentimentDetectionJobs", map[string]any{
		"Filter": map[string]any{"JobStatus": "SUBMITTED"},
	})
	jobs, ok := out["SentimentDetectionJobPropertiesList"].([]any)
	require.True(t, ok)
	require.Len(t, jobs, 1)
	assert.Equal(t, "stays-submitted", jobs[0].(map[string]any)["JobName"])
}

func TestListJobsFilterNoMatchReturnsEmpty(t *testing.T) {
	t.Parallel()

	h := newHandler()
	request(t, h, "StartSentimentDetectionJob", map[string]any{"JobName": "only-job", "LanguageCode": "en"})

	out := request(t, h, "ListSentimentDetectionJobs", map[string]any{
		"Filter": map[string]any{"JobName": "no-such-job"},
	})
	assert.Empty(t, out["SentimentDetectionJobPropertiesList"])
}

// --- List* resource Filter support ---
//
// Filter shapes are NOT uniform across resource families; see
// matchesResourceFilter's doc comment in handler_resources.go.

func TestListDocumentClassifiersFilterByName(t *testing.T) {
	t.Parallel()

	h := newHandler()
	request(t, h, "CreateDocumentClassifier", map[string]any{"DocumentClassifierName": "news", "LanguageCode": "en"})
	request(t, h, "CreateDocumentClassifier", map[string]any{"DocumentClassifierName": "sports", "LanguageCode": "en"})

	out := request(t, h, "ListDocumentClassifiers", map[string]any{
		"Filter": map[string]any{"DocumentClassifierName": "news"},
	})
	list, ok := out["DocumentClassifierPropertiesList"].([]any)
	require.True(t, ok)
	require.Len(t, list, 1)
	assert.Equal(t, "news", list[0].(map[string]any)["DocumentClassifierName"])
}

func TestListEndpointsFilterByModelArn(t *testing.T) {
	t.Parallel()

	h := newHandler()
	request(t, h, "CreateEndpoint", map[string]any{
		"EndpointName": "ep-a", "ModelArn": "arn:aws:comprehend:us-east-1:123456789012:document-classifier/a",
		"DesiredInferenceUnits": 1,
	})
	request(t, h, "CreateEndpoint", map[string]any{
		"EndpointName": "ep-b", "ModelArn": "arn:aws:comprehend:us-east-1:123456789012:document-classifier/b",
		"DesiredInferenceUnits": 1,
	})

	out := request(t, h, "ListEndpoints", map[string]any{
		"Filter": map[string]any{"ModelArn": "arn:aws:comprehend:us-east-1:123456789012:document-classifier/a"},
	})
	list, ok := out["EndpointPropertiesList"].([]any)
	require.True(t, ok)
	require.Len(t, list, 1)
	assert.Equal(t, "ep-a", list[0].(map[string]any)["EndpointName"])
}

func TestListDatasetsFilterByType(t *testing.T) {
	t.Parallel()

	h := newHandler()
	request(t, h, "CreateDataset", mergedBody(datasetBody("train-set"), map[string]any{"DatasetType": "TRAIN"}))
	request(t, h, "CreateDataset", mergedBody(datasetBody("test-set"), map[string]any{"DatasetType": "TEST"}))

	out := request(t, h, "ListDatasets", map[string]any{
		"Filter": map[string]any{"DatasetType": "TEST"},
	})
	list, ok := out["DatasetPropertiesList"].([]any)
	require.True(t, ok)
	require.Len(t, list, 1)
	assert.Equal(t, "test-set", list[0].(map[string]any)["DatasetName"])
}

func TestListResourcesFilterByStatus(t *testing.T) {
	t.Parallel()

	h := newHandler()
	request(t, h, "CreateEndpoint", endpointBody("ep-active"))

	// Every freshly created endpoint is IN_SERVICE (see initialResourceStatus
	// in store.go); a Status filter for a different status must exclude it.
	out := request(t, h, "ListEndpoints", map[string]any{
		"Filter": map[string]any{"Status": "FAILED"},
	})
	assert.Empty(t, out["EndpointPropertiesList"])

	out = request(t, h, "ListEndpoints", map[string]any{
		"Filter": map[string]any{"Status": "IN_SERVICE"},
	})
	assert.Len(t, out["EndpointPropertiesList"], 1)
}

// TestListResourcesNoFilterReturnsAll verifies an absent/nil Filter matches
// everything (the common case every other pagination test already relies on
// implicitly, made explicit here as a Filter-specific regression guard).
func TestListResourcesNoFilterReturnsAll(t *testing.T) {
	t.Parallel()

	h := newHandler()
	request(t, h, "CreateFlywheel", flywheelBody("fw-a"))
	request(t, h, "CreateFlywheel", flywheelBody("fw-b"))

	out := request(t, h, "ListFlywheels", nil)
	assert.Len(t, out["FlywheelSummaryList"], 2)
}

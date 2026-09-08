package comprehend_test

import (
	"bytes"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/comprehend"
)

func newHandler() *comprehend.Handler {
	return comprehend.NewHandler(comprehend.NewInMemoryBackend("123456789012", "us-east-1"))
}

// flywheelBody returns a CreateFlywheelInput body carrying every field
// aws-sdk-go-v2/service/comprehend@v1.43.4/validators.go's
// validateOpCreateFlywheelInput marks required (FlywheelName,
// DataAccessRoleArn, DataLakeS3Uri), for tests that only care about a
// flywheel existing rather than exercising these fields directly.
func flywheelBody(name string) map[string]any {
	return map[string]any{
		"FlywheelName":      name,
		"DataAccessRoleArn": "arn:aws:iam::123456789012:role/comprehend-flywheel",
		"DataLakeS3Uri":     "s3://fk-bucket/" + name,
	}
}

// endpointBody returns a CreateEndpointInput body carrying every field
// validateOpCreateEndpointInput marks required (EndpointName,
// DesiredInferenceUnits).
func endpointBody(name string) map[string]any {
	return map[string]any{
		"EndpointName":          name,
		"DesiredInferenceUnits": 1,
	}
}

// datasetBody returns a CreateDatasetInput body carrying every field
// validateOpCreateDatasetInput marks required (DatasetName, FlywheelArn,
// InputDataConfig).
func datasetBody(name string) map[string]any {
	return map[string]any{
		"DatasetName":     name,
		"FlywheelArn":     "arn:aws:comprehend:us-east-1:123456789012:flywheel/" + name,
		"InputDataConfig": map[string]any{},
	}
}

// mergedBody returns a new map holding base's entries overlaid with extra's,
// without mutating either argument.
func mergedBody(base, extra map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(extra))
	maps.Copy(out, base)
	maps.Copy(out, extra)

	return out
}

func request(t *testing.T, handler *comprehend.Handler, operation string, input map[string]any) map[string]any {
	t.Helper()

	payload, err := json.Marshal(input)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "Comprehend_20171127."+operation)
	rec := httptest.NewRecorder()
	ctx := echo.New().NewContext(req, rec)
	require.NoError(t, handler.Handler()(ctx))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var output map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &output))

	return output
}

// rawRequest issues a Comprehend target-header request with a literal JSON
// body and returns the raw recorder, for tests that need to assert on
// non-200 status codes or malformed/edge-case request bodies (request()
// above requires a 200 response and a map[string]any input, which cannot
// express those cases).
func rawRequest(t *testing.T, handler *comprehend.Handler, operation, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "Comprehend_20171127."+operation)
	rec := httptest.NewRecorder()
	ctx := echo.New().NewContext(req, rec)
	require.NoError(t, handler.Handler()(ctx))

	return rec
}

// decodeBody unmarshals a raw response recorder body into a map, for use
// alongside rawRequest.
func decodeBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	return out
}

// toJSON marshals input for use with rawRequest, for tests that need
// rawRequest's non-200-status assertions but would otherwise duplicate
// json.Marshal boilerplate at every call site.
func toJSON(t *testing.T, input map[string]any) string {
	t.Helper()

	payload, err := json.Marshal(input)
	require.NoError(t, err)

	return string(payload)
}

func TestHandlerMetadataAndRouting(t *testing.T) {
	t.Parallel()

	handler := newHandler()
	assert.Equal(t, "Comprehend", handler.Name())
	assert.Equal(t, "comprehend", handler.ChaosServiceName())
	assert.Equal(t, []string{"us-east-1"}, handler.ChaosRegions())
	assert.Contains(t, handler.GetSupportedOperations(), "DetectSentiment")
	assert.Contains(t, handler.GetSupportedOperations(), "StartDocumentClassificationJob")

	tests := []struct {
		name   string
		target string
		op     string
		want   bool
	}{
		{name: "match", target: "Comprehend_20171127.DetectSentiment", want: true, op: "DetectSentiment"},
		{name: "foreign", target: "Textract.DetectDocumentText", want: false, op: "Unknown"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodPost, "/", nil)
			req.Header.Set("X-Amz-Target", test.target)
			ctx := echo.New().NewContext(req, httptest.NewRecorder())
			assert.Equal(t, test.want, handler.RouteMatcher()(ctx))
			assert.Equal(t, test.op, handler.ExtractOperation(ctx))
		})
	}
}

func TestSynchronousDetectionOperations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		operation string
		input     map[string]any
		field     string
	}{
		{
			name:      "sentiment",
			operation: "DetectSentiment",
			input:     map[string]any{"Text": "This product is great.", "LanguageCode": "en"},
			field:     "Sentiment",
		},
		{
			name:      "entities",
			operation: "DetectEntities",
			input:     map[string]any{"Text": "Alice works here.", "LanguageCode": "en"},
			field:     "Entities",
		},
		{
			name:      "key_phrases",
			operation: "DetectKeyPhrases",
			input:     map[string]any{"Text": "customer support response.", "LanguageCode": "en"},
			field:     "KeyPhrases",
		},
		{
			name:      "pii",
			operation: "DetectPiiEntities",
			input:     map[string]any{"Text": "Email me@example.com.", "LanguageCode": "en"},
			field:     "Entities",
		},
		{
			name:      "syntax",
			operation: "DetectSyntax",
			input:     map[string]any{"Text": "Syntax works", "LanguageCode": "en"},
			field:     "SyntaxTokens",
		},
		{
			name:      "language",
			operation: "DetectDominantLanguage",
			input:     map[string]any{"Text": "Language works"},
			field:     "Languages",
		},
		{
			name:      "toxic",
			operation: "DetectToxicContent",
			input: map[string]any{
				"TextSegments": []any{map[string]any{"Text": "I hate this"}},
				"LanguageCode": "en",
			},
			field: "ResultList",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			output := request(t, newHandler(), test.operation, test.input)
			assert.Contains(t, output, test.field)
			assert.NotEmpty(t, output[test.field])
		})
	}
}

func TestBatchDetectionOperations(t *testing.T) {
	t.Parallel()

	// "BatchDetectPiiEntities" is deliberately excluded: real Comprehend has no
	// such operation at all (PII entity detection has no Batch* form, unlike
	// the five detection types below) -- see handler.go's buildOperations
	// comment.
	for _, operation := range []string{
		"BatchDetectSentiment",
		"BatchDetectEntities",
		"BatchDetectKeyPhrases",
		"BatchDetectSyntax",
		"BatchDetectDominantLanguage",
	} {
		t.Run(operation, func(t *testing.T) {
			t.Parallel()

			output := request(t, newHandler(), operation, map[string]any{
				"TextList":     []any{"Alice has a great launch.", "Contact me@example.com."},
				"LanguageCode": "en",
			})
			results, ok := output["ResultList"].([]any)
			require.True(t, ok)
			assert.Len(t, results, 2)
			assert.Empty(t, output["ErrorList"])
		})
	}
}

func TestAsyncJobCompletionLifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		prefix      string
		objectField string
	}{
		{prefix: "DocumentClassificationJob", objectField: "DocumentClassificationJobProperties"},
		{prefix: "EntitiesDetectionJob", objectField: "EntitiesDetectionJobProperties"},
		{prefix: "KeyPhrasesDetectionJob", objectField: "KeyPhrasesDetectionJobProperties"},
		{prefix: "SentimentDetectionJob", objectField: "SentimentDetectionJobProperties"},
		{prefix: "PiiEntitiesDetectionJob", objectField: "PiiEntitiesDetectionJobProperties"},
		{prefix: "TopicsDetectionJob", objectField: "TopicsDetectionJobProperties"},
		{prefix: "TargetedSentimentDetectionJob", objectField: "TargetedSentimentDetectionJobProperties"},
		{prefix: "DominantLanguageDetectionJob", objectField: "DominantLanguageDetectionJobProperties"},
		{prefix: "EventsDetectionJob", objectField: "EventsDetectionJobProperties"},
	}
	for _, test := range tests {
		t.Run(test.prefix, func(t *testing.T) {
			t.Parallel()

			handler := newHandler()
			started := request(
				t,
				handler,
				"Start"+test.prefix,
				map[string]any{"JobName": test.prefix, "LanguageCode": "en"},
			)
			assert.Equal(t, "SUBMITTED", started["JobStatus"])
			id := started["JobId"].(string)

			first := request(t, handler, "Describe"+test.prefix, map[string]any{"JobId": id})
			assert.Equal(t, "IN_PROGRESS", first[test.objectField].(map[string]any)["JobStatus"])
			second := request(t, handler, "Describe"+test.prefix, map[string]any{"JobId": id})
			assert.Equal(t, "COMPLETED", second[test.objectField].(map[string]any)["JobStatus"])

			listed := request(t, handler, "List"+test.prefix+"s", nil)
			assert.Len(t, listed[test.objectField+"List"], 1)
		})
	}
}

func TestAsyncJobFailureAndStopLifecycle(t *testing.T) {
	t.Parallel()

	for _, prefix := range []string{"SentimentDetectionJob", "EntitiesDetectionJob"} {
		t.Run(prefix+"_failed", func(t *testing.T) {
			t.Parallel()

			handler := newHandler()
			started := request(t, handler, "Start"+prefix, map[string]any{"JobName": "[fail]-job"})
			id := started["JobId"].(string)
			request(t, handler, "Describe"+prefix, map[string]any{"JobId": id})
			failed := request(t, handler, "Describe"+prefix, map[string]any{"JobId": id})
			properties := failed[prefix+"Properties"].(map[string]any)
			assert.Equal(t, "FAILED", properties["JobStatus"])
			// Real *Properties shapes name the failure description field
			// "Message" (see jobMap's doc comment in handler_jobs.go) -- there
			// is no "FailureReason" field on any of them.
			assert.NotEmpty(t, properties["Message"])
		})
		t.Run(prefix+"_stopped", func(t *testing.T) {
			t.Parallel()

			handler := newHandler()
			started := request(t, handler, "Start"+prefix, map[string]any{"JobName": "stop-job"})
			id := started["JobId"].(string)
			stopping := request(t, handler, "Stop"+prefix, map[string]any{"JobId": id})
			assert.Equal(t, "STOP_REQUESTED", stopping["JobStatus"])
			stopped := request(t, handler, "Describe"+prefix, map[string]any{"JobId": id})
			assert.Equal(t, "STOPPED", stopped[prefix+"Properties"].(map[string]any)["JobStatus"])
		})
	}
}

func TestResourceCRUDAndTags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		extraFields  map[string]any // required fields beyond nameField (endpoint/flywheel)
		name         string
		prefix       string
		nameField    string
		nameValue    string
		arnField     string
		objectField  string
		listField    string
		update       bool
		trainingType bool // must advance lifecycle to TRAINED before Delete
		noDelete     bool // real API has no Delete op for this resource family at all
	}{
		{
			name:         "classifier",
			prefix:       "DocumentClassifier",
			nameField:    "DocumentClassifierName",
			nameValue:    "news",
			arnField:     "DocumentClassifierArn",
			objectField:  "DocumentClassifierProperties",
			listField:    "DocumentClassifierPropertiesList",
			trainingType: true,
		},
		{
			name:         "recognizer",
			prefix:       "EntityRecognizer",
			nameField:    "RecognizerName",
			nameValue:    "names",
			arnField:     "EntityRecognizerArn",
			objectField:  "EntityRecognizerProperties",
			listField:    "EntityRecognizerPropertiesList",
			trainingType: true,
		},
		{
			name:        "endpoint",
			prefix:      "Endpoint",
			nameField:   "EndpointName",
			nameValue:   "live",
			arnField:    "EndpointArn",
			objectField: "EndpointProperties",
			listField:   "EndpointPropertiesList",
			update:      true,
			extraFields: map[string]any{"DesiredInferenceUnits": 1},
		},
		{
			name:        "flywheel",
			prefix:      "Flywheel",
			nameField:   "FlywheelName",
			nameValue:   "train",
			arnField:    "FlywheelArn",
			objectField: "FlywheelProperties",
			listField:   "FlywheelSummaryList",
			update:      true,
			extraFields: map[string]any{
				"DataAccessRoleArn": "arn:aws:iam::123456789012:role/comprehend-flywheel",
				"DataLakeS3Uri":     "s3://fk-bucket/train",
			},
		},
		{
			name:        "dataset",
			prefix:      "Dataset",
			nameField:   "DatasetName",
			nameValue:   "data",
			arnField:    "DatasetArn",
			objectField: "DatasetProperties",
			listField:   "DatasetPropertiesList",
			// Real Comprehend has no DeleteDataset operation -- datasets are
			// immutable once created (see resourceSpecs' Dataset.noDelete comment
			// in handler_resources.go). "DeleteDataset" used to be advertised and
			// dispatchable here, which was itself the bug pkgs/sdkcheck's reverse
			// check caught; this test now exercises real behavior instead of the
			// fabricated op.
			noDelete: true,
			extraFields: map[string]any{
				"FlywheelArn":     "arn:aws:comprehend:us-east-1:123456789012:flywheel/data",
				"InputDataConfig": map[string]any{},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			handler := newHandler()
			body := map[string]any{
				test.nameField: test.nameValue,
				"Tags":         []any{map[string]any{"Key": "team", "Value": "nlp"}},
			}
			maps.Copy(body, test.extraFields)
			created := request(t, handler, "Create"+test.prefix, body)
			resourceARN := created[test.arnField].(string)
			assert.NotEmpty(t, resourceARN)

			described := request(t, handler, "Describe"+test.prefix, map[string]any{test.arnField: resourceARN})
			assert.Contains(t, described, test.objectField)
			listed := request(t, handler, "List"+test.prefix+"s", nil)
			assert.Len(t, listed[test.listField], 1)

			request(t, handler, "TagResource", map[string]any{
				"ResourceArn": resourceARN,
				"Tags":        []any{map[string]any{"Key": "env", "Value": "test"}},
			})
			tagged := request(t, handler, "ListTagsForResource", map[string]any{"ResourceArn": resourceARN})
			assert.Len(t, tagged["Tags"], 2)
			request(t, handler, "UntagResource", map[string]any{"ResourceArn": resourceARN, "TagKeys": []any{"team"}})
			tagged = request(t, handler, "ListTagsForResource", map[string]any{"ResourceArn": resourceARN})
			assert.Len(t, tagged["Tags"], 1)

			if test.update {
				request(
					t,
					handler,
					"Update"+test.prefix,
					map[string]any{test.arnField: resourceARN, "DesiredInferenceUnits": 2},
				)
			}
			if test.trainingType {
				// Extra describe (no-op in status since emulator starts at TRAINED).
				request(t, handler, "Describe"+test.prefix, map[string]any{test.arnField: resourceARN})
			}
			if test.noDelete {
				// No real Delete op exists for this family; the resource must
				// simply persist.
				listed = request(t, handler, "List"+test.prefix+"s", nil)
				assert.Len(t, listed[test.listField], 1)

				return
			}
			request(t, handler, "Delete"+test.prefix, map[string]any{test.arnField: resourceARN})
			listed = request(t, handler, "List"+test.prefix+"s", nil)
			assert.Empty(t, listed[test.listField])
		})
	}
}

// TestModelVersionsAndFlywheelIteration verifies that a new document
// classifier/entity recognizer version is created by calling the SAME
// Create op again with the same name and a new VersionName -- the real API
// has no separate CreateDocumentClassifierVersion/CreateEntityRecognizerVersion
// operation (confirmed: no such files exist among aws-sdk-go-v2/service/
// comprehend's generated api_op_*.go, and CreateDocumentClassifierInput/
// CreateEntityRecognizerInput both already carry an optional VersionName
// field for exactly this purpose). A prior pass invented 8 operation names
// for a fabricated "Version" resource family; they have been removed from
// resourceSpecs (see handler_resources.go) and this test now exercises the
// real versioning path instead.
func TestModelVersionsAndFlywheelIteration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		parentInput  map[string]any
		parentPrefix string
		parentARN    string
	}{
		{
			parentPrefix: "DocumentClassifier",
			parentInput:  map[string]any{"DocumentClassifierName": "documents"},
			parentARN:    "DocumentClassifierArn",
		},
		{
			parentPrefix: "EntityRecognizer",
			parentInput:  map[string]any{"RecognizerName": "entities"},
			parentARN:    "EntityRecognizerArn",
		},
	}
	for _, test := range tests {
		t.Run(test.parentPrefix, func(t *testing.T) {
			t.Parallel()

			handler := newHandler()
			base := request(t, handler, "Create"+test.parentPrefix, test.parentInput)
			baseARN, ok := base[test.parentARN].(string)
			require.True(t, ok, "base create must return %s", test.parentARN)
			require.NotEmpty(t, baseARN)

			versionInput := map[string]any{"VersionName": "v2"}
			maps.Copy(versionInput, test.parentInput)
			versioned := request(t, handler, "Create"+test.parentPrefix, versionInput)
			versionedARN, ok := versioned[test.parentARN].(string)
			require.True(t, ok, "versioned create must return %s", test.parentARN)
			require.NotEmpty(t, versionedARN)
			assert.NotEqual(t, baseARN, versionedARN, "a version must get an ARN distinct from the base resource")
			assert.Contains(t, versionedARN, "version/v2")

			listed := request(t, handler, "List"+test.parentPrefix+"s", nil)
			assert.Len(t, listed[test.parentPrefix+"PropertiesList"], 2, "base resource + one version")
		})
	}

	handler := newHandler()
	flywheel := request(t, handler, "CreateFlywheel", flywheelBody("quality"))
	flywheelARN := flywheel["FlywheelArn"].(string)
	started := request(t, handler, "StartFlywheelIteration", map[string]any{"FlywheelArn": flywheelARN})
	id := started["FlywheelIterationId"].(string)
	// "GetFlywheelIteration" is not a real Comprehend operation -- the real name
	// is DescribeFlywheelIteration (see handler.go's buildOperations comment).
	evaluating := request(t, handler, "DescribeFlywheelIteration", map[string]any{"FlywheelIterationId": id})
	assert.Equal(
		t,
		"EVALUATING",
		evaluating["FlywheelIterationProperties"].(map[string]any)["Status"],
	)
	completed := request(t, handler, "DescribeFlywheelIteration", map[string]any{"FlywheelIterationId": id})
	assert.Equal(t, "COMPLETED", completed["FlywheelIterationProperties"].(map[string]any)["Status"])
	history := request(t, handler, "ListFlywheelIterationHistory", map[string]any{"FlywheelArn": flywheelARN})
	assert.Len(t, history["FlywheelIterationPropertiesList"], 1)
}

func TestResetRemovesState(t *testing.T) {
	t.Parallel()

	handler := newHandler()
	request(t, handler, "CreateDataset", datasetBody("temporary"))
	handler.Reset()
	output := request(t, handler, "ListDatasets", nil)
	assert.Empty(t, output["DatasetPropertiesList"])
}

// TestStartJob_TagsReachSameTagStoreAsCreateResource verifies creation-time
// Tags on Start*DetectionJob requests land in the same ARN-keyed tag store
// Create* resources use, so ListTagsForResource/TagResource/UntagResource
// all work against a job's JobArn. Before this fix, StartJob's tags
// argument was dropped on the floor and the job's ARN was never registered
// in the tag store at all, so ListTagsForResource(JobArn) returned
// ResourceNotFoundException even for a job that existed.
func TestStartJob_TagsReachSameTagStoreAsCreateResource(t *testing.T) {
	t.Parallel()

	handler := newHandler()
	started := request(t, handler, "StartSentimentDetectionJob", map[string]any{
		"JobName":      "tagged-job",
		"LanguageCode": "en",
		"Tags":         []any{map[string]any{"Key": "team", "Value": "nlp"}},
	})
	jobArn, _ := started["JobArn"].(string)
	require.NotEmpty(t, jobArn)

	tagged := request(t, handler, "ListTagsForResource", map[string]any{"ResourceArn": jobArn})
	assert.Len(t, tagged["Tags"], 1)

	request(t, handler, "TagResource", map[string]any{
		"ResourceArn": jobArn,
		"Tags":        []any{map[string]any{"Key": "env", "Value": "prod"}},
	})
	tagged = request(t, handler, "ListTagsForResource", map[string]any{"ResourceArn": jobArn})
	assert.Len(t, tagged["Tags"], 2)

	request(t, handler, "UntagResource", map[string]any{"ResourceArn": jobArn, "TagKeys": []any{"team"}})
	tagged = request(t, handler, "ListTagsForResource", map[string]any{"ResourceArn": jobArn})
	assert.Len(t, tagged["Tags"], 1)
}

// TestStartJob_NoTagsStillRegistersArnInTagStore verifies a job started
// without any Tags is still registered in the tag store (as an empty tag
// set), matching how CreateResource always seeds b.tags[arn], so
// ListTagsForResource never spuriously 404s for an existing, untagged job.
func TestStartJob_NoTagsStillRegistersArnInTagStore(t *testing.T) {
	t.Parallel()

	handler := newHandler()
	started := request(t, handler, "StartSentimentDetectionJob", map[string]any{
		"JobName": "untagged-job", "LanguageCode": "en",
	})
	jobArn, _ := started["JobArn"].(string)
	require.NotEmpty(t, jobArn)

	tagged := request(t, handler, "ListTagsForResource", map[string]any{"ResourceArn": jobArn})
	assert.Empty(t, tagged["Tags"])
}

// TestResourceProperties_TimestampFieldNamesMatchAWSShape verifies each
// resource family's Describe response uses the timestamp field names its
// real AWS Properties shape actually has. These are NOT uniform:
// DocumentClassifier/EntityRecognizer properties use SubmitTime/EndTime,
// Endpoint/Flywheel properties use CreationTime/LastModifiedTime, and
// Dataset properties use CreationTime/EndTime. Before this fix every
// resource type emitted SubmitTime/EndTime, so a real SDK client describing
// an Endpoint, Flywheel, or Dataset always saw a nil CreationTime (and, for
// Endpoint/Flywheel, a nil LastModifiedTime too).
func TestResourceProperties_TimestampFieldNamesMatchAWSShape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		extraFields     map[string]any
		name            string
		createOp        string
		nameField       string
		describeOp      string
		arnField        string
		objectField     string
		absentTimeField string
		wantTimeFields  []string
	}{
		{
			name:     "document_classifier_uses_submit_end_time",
			createOp: "CreateDocumentClassifier", nameField: "DocumentClassifierName",
			describeOp: "DescribeDocumentClassifier", arnField: "DocumentClassifierArn",
			objectField: "DocumentClassifierProperties", wantTimeFields: []string{"SubmitTime", "EndTime"},
			absentTimeField: "CreationTime",
		},
		{
			name:     "endpoint_uses_creation_and_last_modified_time",
			createOp: "CreateEndpoint", nameField: "EndpointName",
			describeOp: "DescribeEndpoint", arnField: "EndpointArn",
			objectField: "EndpointProperties", wantTimeFields: []string{"CreationTime", "LastModifiedTime"},
			absentTimeField: "SubmitTime",
			extraFields:     map[string]any{"DesiredInferenceUnits": 1},
		},
		{
			name:     "flywheel_uses_creation_and_last_modified_time",
			createOp: "CreateFlywheel", nameField: "FlywheelName",
			describeOp: "DescribeFlywheel", arnField: "FlywheelArn",
			objectField: "FlywheelProperties", wantTimeFields: []string{"CreationTime", "LastModifiedTime"},
			absentTimeField: "SubmitTime",
			extraFields: map[string]any{
				"DataAccessRoleArn": "arn:aws:iam::123456789012:role/comprehend-flywheel",
				"DataLakeS3Uri":     "s3://fk-bucket/resource-name",
			},
		},
		{
			name:     "dataset_uses_creation_time_and_end_time",
			createOp: "CreateDataset", nameField: "DatasetName",
			describeOp: "DescribeDataset", arnField: "DatasetArn",
			objectField: "DatasetProperties", wantTimeFields: []string{"CreationTime", "EndTime"},
			absentTimeField: "LastModifiedTime",
			extraFields: map[string]any{
				"FlywheelArn":     "arn:aws:comprehend:us-east-1:123456789012:flywheel/resource-name",
				"InputDataConfig": map[string]any{},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			handler := newHandler()
			body := map[string]any{test.nameField: "resource-name"}
			maps.Copy(body, test.extraFields)
			created := request(t, handler, test.createOp, body)
			resourceARN, _ := created[test.arnField].(string)
			require.NotEmpty(t, resourceARN)

			described := request(t, handler, test.describeOp, map[string]any{test.arnField: resourceARN})
			props, ok := described[test.objectField].(map[string]any)
			require.True(t, ok, "describe must return %s", test.objectField)

			for _, field := range test.wantTimeFields {
				assert.NotEmpty(t, props[field], "expected %s to be set", field)
			}
			assert.NotContains(t, props, test.absentTimeField, "did not expect %s in this resource's properties")
		})
	}
}

// TestListFlywheels_UsesFlywheelSummaryListWrapper verifies ListFlywheels
// wraps its items as FlywheelSummaryList, matching the real
// ListFlywheelsOutput shape (FlywheelSummary items) -- every other List*
// response here reuses its Properties name for the list wrapper (e.g.
// EndpointPropertiesList), but Flywheel is the one exception. Before this
// fix the wrapper was named FlywheelPropertiesList, so a real SDK client's
// ListFlywheels call always saw a nil/empty FlywheelSummaryList.
func TestListFlywheels_UsesFlywheelSummaryListWrapper(t *testing.T) {
	t.Parallel()

	handler := newHandler()
	request(t, handler, "CreateFlywheel", flywheelBody("quality"))

	listed := request(t, handler, "ListFlywheels", nil)
	assert.NotContains(t, listed, "FlywheelPropertiesList")
	assert.Len(t, listed["FlywheelSummaryList"], 1)
}

// TestImportModel_RoutesResourceTypeFromSourceArn verifies ImportModel
// creates a DocumentClassifier or an EntityRecognizer depending on which
// kind of model SourceModelArn (the required input) identifies, matching
// real AWS behavior where the imported model's type mirrors its source.
// Before this fix ImportModel always created a DocumentClassifier
// regardless of SourceModelArn, so importing an entity-recognizer model
// produced a resource DescribeEntityRecognizer could never find.
func TestImportModel_RoutesResourceTypeFromSourceArn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		sourceArn   string
		describeOp  string
		arnField    string
		objectField string
	}{
		{
			name:        "document_classifier_source",
			sourceArn:   "arn:aws:comprehend:us-east-1:999999999999:document-classifier/upstream-clf",
			describeOp:  "DescribeDocumentClassifier",
			arnField:    "DocumentClassifierArn",
			objectField: "DocumentClassifierProperties",
		},
		{
			name:        "entity_recognizer_source",
			sourceArn:   "arn:aws:comprehend:us-east-1:999999999999:entity-recognizer/upstream-rec",
			describeOp:  "DescribeEntityRecognizer",
			arnField:    "EntityRecognizerArn",
			objectField: "EntityRecognizerProperties",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			handler := newHandler()
			imported := request(t, handler, "ImportModel", map[string]any{"SourceModelArn": test.sourceArn})
			modelArn, _ := imported["ModelArn"].(string)
			require.NotEmpty(t, modelArn)
			assert.Contains(t, modelArn, "upstream-")

			described := request(t, handler, test.describeOp, map[string]any{test.arnField: modelArn})
			assert.Contains(t, described, test.objectField)
		})
	}
}

// --- Protocol accuracy ---

func TestProtocolContentType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		action     string
		body       string
		wantCT     string
		wantStatus int
	}{
		{
			name:       "success_response_has_json11_content_type",
			action:     "DetectDominantLanguage",
			body:       `{"Text":"hello world"}`,
			wantStatus: http.StatusOK,
			wantCT:     "application/x-amz-json-1.1",
		},
		{
			name:       "error_response_has_json11_content_type",
			action:     "DetectSentiment",
			body:       `{"Text":"","LanguageCode":"en"}`,
			wantStatus: http.StatusBadRequest,
			wantCT:     "application/x-amz-json-1.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := rawRequest(t, newHandler(), tt.action, tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Contains(t, rec.Header().Get("Content-Type"), tt.wantCT)
		})
	}
}

func TestProtocolErrorEnvelope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		action string
		body   string
	}{
		{
			name:   "not_found_resource",
			action: "DescribeDocumentClassifier",
			body:   `{"DocumentClassifierArn":"arn:aws:comprehend:us-east-1:000000000000:document-classifier/missing"}`,
		},
		{
			name:   "missing_required_text",
			action: "DetectSentiment",
			body:   `{"Text":"","LanguageCode":"en"}`,
		},
		{
			name:   "unknown_operation",
			action: "NoSuchOperation",
			body:   `{}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := rawRequest(t, newHandler(), tt.action, tt.body)
			assert.GreaterOrEqual(t, rec.Code, http.StatusBadRequest)

			m := decodeBody(t, rec)
			assert.NotEmpty(t, m["__type"], "__type must be present in error envelope")
			assert.NotEmpty(t, m["message"], "message must be present in error envelope")
		})
	}
}

func TestProtocolHTTPRouting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		method     string
		path       string
		target     string
		wantStatus int
	}{
		{
			name:       "GET_slash_returns_op_list",
			method:     http.MethodGet,
			path:       "/",
			wantStatus: http.StatusOK,
		},
		{
			name:       "PUT_returns_405",
			method:     http.MethodPut,
			path:       "/",
			target:     "Comprehend_20171127.DetectSentiment",
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "POST_missing_target_returns_400",
			method:     http.MethodPost,
			path:       "/",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler := newHandler()
			e := echo.New()
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader("{}"))
			if tt.target != "" {
				req.Header.Set("X-Amz-Target", tt.target)
			}

			req.Header.Set("Content-Type", "application/x-amz-json-1.1")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			require.NoError(t, handler.Handler()(c))
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestProtocolOpList(t *testing.T) {
	t.Parallel()

	handler := newHandler()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	require.NoError(t, handler.Handler()(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var ops []string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &ops))
	assert.NotEmpty(t, ops, "GET / must return non-empty operation list")
	assert.Contains(t, ops, "DetectSentiment")
	assert.Contains(t, ops, "DetectEntities")
	assert.Contains(t, ops, "StartDocumentClassificationJob")
}

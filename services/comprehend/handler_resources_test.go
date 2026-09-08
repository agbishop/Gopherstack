package comprehend_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/comprehend"
)

// --- Resource lifecycle field shapes ---

func TestDocumentClassifierFieldShapes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		fn   func(t *testing.T, h *comprehend.Handler)
		name string
	}{
		{
			name: "create_returns_arn",
			fn: func(t *testing.T, h *comprehend.Handler) {
				t.Helper()
				rec := rawRequest(t, h, "CreateDocumentClassifier",
					`{"DocumentClassifierName":"audit-clf","LanguageCode":"en"}`)
				require.Equal(t, http.StatusOK, rec.Code)
				m := decodeBody(t, rec)
				arn, ok := m["DocumentClassifierArn"].(string)
				require.True(t, ok, "create response must have DocumentClassifierArn")
				assert.Contains(t, arn, "comprehend", "ARN must reference comprehend service")
				assert.Contains(t, arn, "document-classifier", "ARN must reference resource type")
				assert.Contains(t, arn, "audit-clf", "ARN must contain classifier name")
			},
		},
		{
			name: "describe_response_shape",
			fn: func(t *testing.T, h *comprehend.Handler) {
				t.Helper()
				createRec := rawRequest(t, h, "CreateDocumentClassifier",
					`{"DocumentClassifierName":"shape-clf","LanguageCode":"en"}`)
				createResp := decodeBody(t, createRec)
				arn := createResp["DocumentClassifierArn"].(string)

				descRec := rawRequest(t, h, "DescribeDocumentClassifier",
					`{"DocumentClassifierArn":"`+arn+`"}`)
				require.Equal(t, http.StatusOK, descRec.Code)
				m := decodeBody(t, descRec)
				props, ok := m["DocumentClassifierProperties"].(map[string]any)
				require.True(t, ok, "describe must return DocumentClassifierProperties")
				assert.NotEmpty(t, props["DocumentClassifierArn"], "properties must have ARN")
				assert.NotEmpty(t, props["Status"], "properties must have Status")
				assert.NotEmpty(t, props["SubmitTime"], "properties must have SubmitTime")
			},
		},
		{
			name: "duplicate_create_returns_conflict",
			fn: func(t *testing.T, h *comprehend.Handler) {
				t.Helper()
				body := `{"DocumentClassifierName":"dupe-clf","LanguageCode":"en"}`
				rec1 := rawRequest(t, h, "CreateDocumentClassifier", body)
				require.Equal(t, http.StatusOK, rec1.Code)
				rec2 := rawRequest(t, h, "CreateDocumentClassifier", body)
				assert.Equal(t, http.StatusBadRequest, rec2.Code)
				m := decodeBody(t, rec2)
				assert.Equal(t, "ResourceInUseException", m["__type"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tt.fn(t, newHandler())
		})
	}
}

func TestEndpointUpdateAndStatus(t *testing.T) {
	t.Parallel()

	h := newHandler()
	created := request(t, h, "CreateEndpoint", map[string]any{"EndpointName": "audit-ep", "DesiredInferenceUnits": 1})
	arn := created["EndpointArn"].(string)
	assert.NotEmpty(t, arn)

	described := request(t, h, "DescribeEndpoint", map[string]any{"EndpointArn": arn})
	props := described["EndpointProperties"].(map[string]any)
	assert.Equal(t, "IN_SERVICE", props["Status"], "new endpoint must be IN_SERVICE")

	request(t, h, "UpdateEndpoint", map[string]any{"EndpointArn": arn, "DesiredInferenceUnits": 4})
}

// --- Classifier/recognizer training lifecycle ---
//
// The emulator skips async training so classifiers/recognizers start
// immediately at TRAINED, allowing the Terraform provider waiter to exit
// without a long delay.

func TestTrainingResourceLifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		createBody   map[string]any
		name         string
		createAction string
		descAction   string
		arnField     string
		propsField   string
	}{
		{
			name:         "document_classifier",
			createAction: "CreateDocumentClassifier",
			descAction:   "DescribeDocumentClassifier",
			arnField:     "DocumentClassifierArn",
			propsField:   "DocumentClassifierProperties",
			createBody:   map[string]any{"DocumentClassifierName": "lifecycle-clf", "LanguageCode": "en"},
		},
		{
			name:         "entity_recognizer",
			createAction: "CreateEntityRecognizer",
			descAction:   "DescribeEntityRecognizer",
			arnField:     "EntityRecognizerArn",
			propsField:   "EntityRecognizerProperties",
			createBody:   map[string]any{"RecognizerName": "lifecycle-rec", "LanguageCode": "en"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()

			// Create → immediately TRAINED (emulator skips async training).
			created := request(t, h, tc.createAction, tc.createBody)
			arn := created[tc.arnField].(string)
			require.NotEmpty(t, arn)

			// First Describe → still TRAINED (no lifecycle advancement needed).
			desc1 := request(t, h, tc.descAction, map[string]any{tc.arnField: arn})
			props1 := desc1[tc.propsField].(map[string]any)
			assert.Equal(t, "TRAINED", props1["Status"])

			// Second Describe → still TRAINED.
			desc2 := request(t, h, tc.descAction, map[string]any{tc.arnField: arn})
			props2 := desc2[tc.propsField].(map[string]any)
			assert.Equal(t, "TRAINED", props2["Status"])
		})
	}
}

// --- Delete training-resource: immediate deletion allowed ---
//
// Classifiers/recognizers start TRAINED so they can be deleted immediately
// (no SUBMITTED/IN_PROGRESS state guard needed).

func TestDeleteTrainingResourceStateGuard(t *testing.T) {
	t.Parallel()

	tests := []struct {
		createBody   map[string]any
		name         string
		createAction string
		deleteAction string
		arnField     string
	}{
		{
			name:         "delete_classifier_immediately",
			createAction: "CreateDocumentClassifier",
			deleteAction: "DeleteDocumentClassifier",
			arnField:     "DocumentClassifierArn",
			createBody:   map[string]any{"DocumentClassifierName": "del-guard-clf", "LanguageCode": "en"},
		},
		{
			name:         "delete_recognizer_immediately",
			createAction: "CreateEntityRecognizer",
			deleteAction: "DeleteEntityRecognizer",
			arnField:     "EntityRecognizerArn",
			createBody:   map[string]any{"RecognizerName": "del-guard-rec", "LanguageCode": "en"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()

			// Create resource — immediately TRAINED, so delete succeeds right away.
			created := request(t, h, tc.createAction, tc.createBody)
			arn := created[tc.arnField].(string)

			request(t, h, tc.deleteAction, map[string]any{tc.arnField: arn})
		})
	}
}

// TestDeleteResource_BlockedByActiveInferenceJob covers
// DeleteDocumentClassifierInput/DeleteEntityRecognizerInput's own doc
// comments: "Only those classifiers [recognizers] that are in terminated
// states (IN_ERROR, TRAINED) will be deleted. If an active inference job is
// using the model, a ResourceInUseException will be returned."
// (aws-sdk-go-v2/service/comprehend@v1.43.4/api_op_DeleteDocumentClassifier.go:13-15,
// api_op_DeleteEntityRecognizer.go:13-15). Before this fix, DeleteResource
// only checked the resource's OWN training status, never whether a
// SUBMITTED/IN_PROGRESS document-classification-job/entities-detection-job
// still referenced it via DocumentClassifierArn/EntityRecognizerArn -- such
// a job's model could be deleted out from under it.
func TestDeleteResource_BlockedByActiveInferenceJob(t *testing.T) {
	t.Parallel()

	tests := []struct {
		createBody    map[string]any
		name          string
		createAction  string
		deleteAction  string
		arnField      string
		startJobOp    string
		describeJobOp string
	}{
		{
			name:          "document_classifier",
			createAction:  "CreateDocumentClassifier",
			deleteAction:  "DeleteDocumentClassifier",
			arnField:      "DocumentClassifierArn",
			startJobOp:    "StartDocumentClassificationJob",
			describeJobOp: "DescribeDocumentClassificationJob",
			createBody:    map[string]any{"DocumentClassifierName": "in-use-clf", "LanguageCode": "en"},
		},
		{
			name:          "entity_recognizer",
			createAction:  "CreateEntityRecognizer",
			deleteAction:  "DeleteEntityRecognizer",
			arnField:      "EntityRecognizerArn",
			startJobOp:    "StartEntitiesDetectionJob",
			describeJobOp: "DescribeEntitiesDetectionJob",
			createBody:    map[string]any{"RecognizerName": "in-use-rec", "LanguageCode": "en"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()
			created := request(t, h, tc.createAction, tc.createBody)
			modelArn := created[tc.arnField].(string)

			started := request(t, h, tc.startJobOp, map[string]any{
				"JobName": "in-use-job", tc.arnField: modelArn,
			})
			jobID := started["JobId"].(string)

			rec := rawRequest(t, h, tc.deleteAction, toJSON(t, map[string]any{tc.arnField: modelArn}))
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			resp := decodeBody(t, rec)
			assert.Equal(t, "ResourceInUseException", resp["__type"])

			// Advance the job to a terminal state: SUBMITTED -> IN_PROGRESS -> COMPLETED.
			request(t, h, tc.describeJobOp, map[string]any{"JobId": jobID})
			request(t, h, tc.describeJobOp, map[string]any{"JobId": jobID})

			request(t, h, tc.deleteAction, map[string]any{tc.arnField: modelArn})
		})
	}
}

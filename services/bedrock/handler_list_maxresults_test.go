package bedrock_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/bedrock"
)

// TestHandler_List_MaxResults covers the maxResults query parameter on
// ListEvaluationJobs, ListModelInvocationJobs, ListCustomModels,
// ListModelCustomizationJobs, and ListImportedModels. All five real SDK
// inputs model MaxResults *int32 (bedrock@v1.66.4 api_op_ListEvaluationJobs.go,
// api_op_ListModelInvocationJobs.go, api_op_ListCustomModels.go, api_op_
// ListModelCustomizationJobs.go, api_op_ListImportedModels.go), serialized as
// the "maxResults" query param -- but the handlers never parsed it and the
// backends always paginated with the fixed bedrockDefaultPageSize via
// paginateBedrockSlice, silently ignoring a client's smaller page-size
// request (gopherstack-kkfs for ListImportedModels).
func TestHandler_List_MaxResults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup func(t *testing.T, h *bedrock.Handler)
		name  string
		path  string
	}{
		{
			name: "ListEvaluationJobs",
			setup: func(t *testing.T, h *bedrock.Handler) {
				t.Helper()

				for _, n := range []string{"ej-a", "ej-b", "ej-c"} {
					_, err := h.Backend.CreateEvaluationJob(n, nil)
					require.NoError(t, err)
				}
			},
			path: "/evaluation-jobs",
		},
		{
			name: "ListModelInvocationJobs",
			setup: func(t *testing.T, h *bedrock.Handler) {
				t.Helper()

				for _, n := range []string{"mij-a", "mij-b", "mij-c"} {
					_, err := h.Backend.CreateModelInvocationJob(n, nil)
					require.NoError(t, err)
				}
			},
			path: "/model-invocation-jobs",
		},
		{
			name: "ListCustomModels",
			setup: func(t *testing.T, h *bedrock.Handler) {
				t.Helper()

				for _, n := range []string{"cm-a", "cm-b", "cm-c"} {
					_, err := h.Backend.CreateCustomModel(n, nil)
					require.NoError(t, err)
				}
			},
			path: "/custom-models",
		},
		{
			name: "ListModelCustomizationJobs",
			setup: func(t *testing.T, h *bedrock.Handler) {
				t.Helper()

				for _, n := range []string{"mcj-a", "mcj-b", "mcj-c"} {
					_, err := createCustomizationJob(h.Backend, n, n+"-model")
					require.NoError(t, err)
				}
			},
			path: "/model-customization-jobs",
		},
		{
			name: "ListImportedModels",
			setup: func(t *testing.T, h *bedrock.Handler) {
				t.Helper()

				for _, n := range []string{"im-a", "im-b", "im-c"} {
					_, err := h.Backend.CreateModelImportJob(
						n, n+"-model", "arn:aws:iam::000000000000:role/x", "s3://bucket/data/", nil,
					)
					require.NoError(t, err)
				}
			},
			path: "/imported-models",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			tt.setup(t, h)

			rec := doRequest(t, h, http.MethodGet, tt.path+"?maxResults=1", nil)
			require.Equal(t, http.StatusOK, rec.Code)

			var out map[string]any
			mustUnmarshal(t, rec, &out)

			items := listPayloadItems(t, out)
			assert.Len(t, items, 1)
			assert.NotEmpty(t, out["nextToken"])
		})
	}
}

// listPayloadItems finds the single list field in a List* response body --
// each op names it differently (jobSummaries, modelSummaries, ...) -- and
// returns it as a slice.
func listPayloadItems(t *testing.T, out map[string]any) []any {
	t.Helper()

	for k, v := range out {
		if k == "nextToken" {
			continue
		}

		if items, ok := v.([]any); ok {
			return items
		}
	}

	t.Fatalf("no list field found in response: %v", out)

	return nil
}

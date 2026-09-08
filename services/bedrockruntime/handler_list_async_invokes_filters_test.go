package bedrockruntime_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func startAsyncInvokeBody(i int) map[string]any {
	return map[string]any{
		"modelId": fmt.Sprintf("model-%d", i),
		"outputDataConfig": map[string]any{
			"s3OutputDataConfig": map[string]any{
				"s3Uri": fmt.Sprintf("s3://bucket/%d/", i),
			},
		},
	}
}

// TestHandler_ListAsyncInvokes_SubmitTimeFilters proves submitTimeAfter and
// submitTimeBefore (ListAsyncInvokesInput fields, httpQuery-bound in
// serializers.go's awsRestjson1_serializeOpHttpBindingsListAsyncInvokesInput)
// actually filter results. Before the fix, handleListAsyncInvokes never read
// these query params, so both silently matched every invocation.
func TestHandler_ListAsyncInvokes_SubmitTimeFilters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		queryParam string
		wantCount  int
	}{
		{
			name:       "submitTimeAfter excludes invocations submitted before it",
			queryParam: "submitTimeAfter",
			wantCount:  1,
		},
		{
			name:       "submitTimeBefore excludes invocations submitted after it",
			queryParam: "submitTimeBefore",
			wantCount:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				h := newTestHandler(t)

				rec1 := doRequest(t, h, http.MethodPost, "/async-invoke", startAsyncInvokeBody(1))
				require.Equal(t, http.StatusAccepted, rec1.Code)

				time.Sleep(2 * time.Second)
				cutoff := time.Now().UTC()
				time.Sleep(2 * time.Second)

				rec2 := doRequest(t, h, http.MethodPost, "/async-invoke", startAsyncInvokeBody(2))
				require.Equal(t, http.StatusAccepted, rec2.Code)

				query := "?" + tt.queryParam + "=" + cutoff.Format(time.RFC3339)
				rec := doRequest(t, h, http.MethodGet, "/async-invoke"+query, nil)
				require.Equal(t, http.StatusOK, rec.Code)

				var out map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

				summaries, ok := out["asyncInvokeSummaries"].([]any)
				require.True(t, ok)
				assert.Len(t, summaries, tt.wantCount)
			})
		})
	}
}

// TestHandler_ListAsyncInvokes_SortOrder proves sortOrder=Descending reverses
// the default ascending-by-submitTime order.
func TestHandler_ListAsyncInvokes_SortOrder(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		h := newTestHandler(t)

		rec1 := doRequest(t, h, http.MethodPost, "/async-invoke", startAsyncInvokeBody(1))
		require.Equal(t, http.StatusAccepted, rec1.Code)

		var firstBody map[string]any
		require.NoError(t, json.Unmarshal(rec1.Body.Bytes(), &firstBody))
		firstARN, ok := firstBody["invocationArn"].(string)
		require.True(t, ok)

		time.Sleep(time.Second)

		rec2 := doRequest(t, h, http.MethodPost, "/async-invoke", startAsyncInvokeBody(2))
		require.Equal(t, http.StatusAccepted, rec2.Code)

		var secondBody map[string]any
		require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &secondBody))
		secondARN, ok := secondBody["invocationArn"].(string)
		require.True(t, ok)

		rec := doRequest(t, h, http.MethodGet, "/async-invoke?sortOrder=Descending", nil)
		require.Equal(t, http.StatusOK, rec.Code)

		var out map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

		summaries, ok := out["asyncInvokeSummaries"].([]any)
		require.True(t, ok)
		require.Len(t, summaries, 2)

		first, ok := summaries[0].(map[string]any)
		require.True(t, ok)
		second, ok := summaries[1].(map[string]any)
		require.True(t, ok)

		assert.Equal(t, secondARN, first["invocationArn"],
			"descending order should list the most-recently-submitted invocation first")
		assert.Equal(t, firstARN, second["invocationArn"])
	})
}

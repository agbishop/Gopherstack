package bedrockruntime_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/bedrockruntime"
)

// TestStartWorker_AdvancesAsyncInvoke_NearCompletionDelay proves the janitor
// started by Handler.StartWorker ticks near the 5s completion delay rather
// than the hour StartWorker used to hardcode, which left a real server's
// invocations InProgress for up to that hour.
func TestStartWorker_AdvancesAsyncInvoke_NearCompletionDelay(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		h := newTestHandler(t)

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		require.NoError(t, h.StartWorker(ctx))
		defer h.Shutdown(t.Context())

		startBody := map[string]any{
			"modelId": "anthropic.claude-v2",
			"outputDataConfig": map[string]any{
				"s3OutputDataConfig": map[string]any{
					"s3Uri": "s3://bucket/out/",
				},
			},
		}

		rec := doRequest(t, h, http.MethodPost, "/async-invoke", startBody)
		require.Equal(t, http.StatusAccepted, rec.Code)

		var started map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &started))
		arn, ok := started["invocationArn"].(string)
		require.True(t, ok)

		// Strictly longer than the 5s completion delay: two timers due at the
		// same fake instant have no guaranteed fire order.
		time.Sleep(6 * time.Second)
		synctest.Wait()

		getRec := doRequest(t, h, http.MethodGet, "/async-invoke/"+arn, nil)
		require.Equal(t, http.StatusOK, getRec.Code)

		var got map[string]any
		require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &got))

		assert.Equal(t, bedrockruntime.AsyncInvokeStatusCompleted, got["status"])
	})
}

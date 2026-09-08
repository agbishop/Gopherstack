package codecommit_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/awsmeta"
	"github.com/blackbirdworks/gopherstack/services/codecommit"
)

// doRequestAsActor is doRequest plus an awsmeta.Principal on the request
// context, mirroring what cli.go's global principalMiddleware attaches to
// every real request before a service ever sees it (gopherstack-a7tx).
func doRequestAsActor(
	t *testing.T,
	h *codecommit.Handler,
	action, actorArn string,
	body any,
) *httptest.ResponseRecorder {
	t.Helper()

	var bodyBytes []byte

	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		require.NoError(t, err)
	}

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "CodeCommit_20150413."+action)

	if actorArn != "" {
		ctx := awsmeta.Set(req.Context(), &awsmeta.Metadata{
			Principal: &awsmeta.Principal{Kind: awsmeta.PrincipalKindUser, Arn: actorArn},
		})
		req = req.WithContext(ctx)
	}

	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	err := h.Handler()(c)
	require.NoError(t, err)

	return rec
}

// createTestPRForActorArn creates a pull request and returns its ID and
// current revisionId, both required by OverridePullRequestApprovalRules.
func createTestPRForActorArn(t *testing.T, h *codecommit.Handler) (string, string) {
	t.Helper()

	rec := doRequest(t, h, "CreatePullRequest", map[string]any{
		"title": "actor arn test PR",
		"targets": []map[string]any{
			{
				"repositoryName":  "repo",
				"sourceReference": "refs/heads/feature",
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		PullRequest struct {
			PullRequestID string `json:"pullRequestId"`
			RevisionID    string `json:"revisionId"`
		} `json:"pullRequest"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotEmpty(t, resp.PullRequest.PullRequestID)
	require.NotEmpty(t, resp.PullRequest.RevisionID)

	return resp.PullRequest.PullRequestID, resp.PullRequest.RevisionID
}

// TestOverridePullRequestApprovalRules_RecordsCallerAsActorArn is the
// regression test for gopherstack-a7tx: DescribePullRequestEventsInput.ActorArn
// ("The Amazon Resource Name (ARN) of the user whose actions resulted in the
// event", codecommit@v1.36.4 api_op_DescribePullRequestEvents.go:36-39) can only
// filter by who performed an action if gopherstack records that actor when the
// event is created. The caller's identity is already resolved onto the request
// context by cli.go's global principalMiddleware (awsmeta.CallerArn) before
// codecommit's dispatch ever runs -- this asserts codecommit now reads it
// instead of discarding ctx and hardcoding overriderARN to "".
func TestOverridePullRequestApprovalRules_RecordsCallerAsActorArn(t *testing.T) {
	t.Parallel()

	const (
		aliceArn = "arn:aws:iam::000000000000:user/alice"
		bobArn   = "arn:aws:iam::000000000000:user/bob"
	)

	h := newTestHandler(t)
	prID, revisionID := createTestPRForActorArn(t, h)

	rec := doRequestAsActor(t, h, "OverridePullRequestApprovalRules", aliceArn, map[string]any{
		"pullRequestId":  prID,
		"revisionId":     revisionID,
		"overrideStatus": "OVERRIDE",
	})
	require.Equal(t, http.StatusOK, rec.Code, "OverridePullRequestApprovalRules body: %s", rec.Body.String())

	tests := []struct {
		name          string
		actorArn      string
		wantBodyMatch string
		wantStatus    int
		wantCount     int
	}{
		{name: "matches_caller", actorArn: aliceArn, wantStatus: http.StatusOK, wantCount: 1},
		{name: "different_actor_returns_none", actorArn: bobArn, wantStatus: http.StatusOK, wantCount: 0},
		{name: "no_filter_returns_all", actorArn: "", wantStatus: http.StatusOK, wantCount: 1},
		{
			name:          "malformed_arn_rejected",
			actorArn:      "not-an-arn",
			wantStatus:    http.StatusBadRequest,
			wantBodyMatch: "InvalidActorArnException",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			body := map[string]any{"pullRequestId": prID}
			if tt.actorArn != "" {
				body["actorArn"] = tt.actorArn
			}

			evRec := doRequest(t, h, "DescribePullRequestEvents", body)
			assert.Equal(t, tt.wantStatus, evRec.Code, "body: %s", evRec.Body.String())

			if tt.wantBodyMatch != "" {
				assert.Contains(t, evRec.Body.String(), tt.wantBodyMatch)

				return
			}

			var resp struct {
				PullRequestEvents []map[string]any `json:"pullRequestEvents"`
			}
			require.NoError(t, json.Unmarshal(evRec.Body.Bytes(), &resp))
			assert.Len(t, resp.PullRequestEvents, tt.wantCount)

			if tt.wantCount > 0 {
				assert.Equal(t, aliceArn, resp.PullRequestEvents[0]["actorArn"],
					"event's actorArn should be the real caller identity, not empty")
			}
		})
	}
}

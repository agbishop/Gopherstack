package codecommit_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_CreatePullRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input      map[string]any
		name       string
		wantStatus int
	}{
		{
			name: "success",
			input: map[string]any{
				"title":       "My PR",
				"description": "A test PR",
				"targets": []map[string]any{
					{
						"repositoryName":       "repo",
						"sourceReference":      "refs/heads/feature",
						"destinationReference": "refs/heads/main",
					},
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "missing_title",
			input: map[string]any{
				"title": "",
				"targets": []map[string]any{
					{
						"repositoryName":  "repo",
						"sourceReference": "refs/heads/feature",
					},
				},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "no_targets",
			input: map[string]any{
				"title":   "My PR",
				"targets": []map[string]any{},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "sequential_ids",
			input: map[string]any{
				"title": "Second PR",
				"targets": []map[string]any{
					{
						"repositoryName":  "repo",
						"sourceReference": "refs/heads/feature",
					},
				},
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			rec := doRequest(t, h, "CreatePullRequest", tt.input)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

				pr, ok := resp["pullRequest"].(map[string]any)
				require.True(t, ok, "pullRequest key should be present")
				assert.NotEmpty(t, pr["pullRequestId"])
				assert.Equal(t, "OPEN", pr["pullRequestStatus"])
				assert.NotEmpty(t, pr["revisionId"])
			}
		})
	}
}

func TestHandler_CreatePullRequest_TargetValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		targets    []map[string]any
		wantStatus int
	}{
		{
			name: "missing_repo_name",
			targets: []map[string]any{
				{"repositoryName": "", "sourceReference": "refs/heads/feature"},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "missing_source_reference",
			targets: []map[string]any{
				{"repositoryName": "repo", "sourceReference": ""},
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			rec := doRequest(t, h, "CreatePullRequest", map[string]any{
				"title":   "My PR",
				"targets": tt.targets,
			})
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestHandler_CreatePullRequest_RepoMetadataInResponse(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateRepository", map[string]any{"repositoryName": "repo"})

	rec := doRequest(t, h, "CreatePullRequest", map[string]any{
		"title":       "My PR",
		"description": "A test pull request",
		"targets": []map[string]any{
			{
				"repositoryName":       "repo",
				"sourceReference":      "refs/heads/feat",
				"destinationReference": "refs/heads/main",
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	pr := resp["pullRequest"].(map[string]any)

	assert.NotEmpty(t, pr["pullRequestId"])
	assert.Equal(t, "My PR", pr["title"])
	assert.Equal(t, "A test pull request", pr["description"])
	assert.Equal(t, "OPEN", pr["pullRequestStatus"])
	assert.NotEmpty(t, pr["revisionId"])
	assert.NotNil(t, pr["creationDate"])
	assert.NotNil(t, pr["lastActivityDate"])

	targets := pr["pullRequestTargets"].([]any)
	require.Len(t, targets, 1)
	target := targets[0].(map[string]any)
	assert.Equal(t, "repo", target["repositoryName"])
	assert.Equal(t, "refs/heads/feat", target["sourceReference"])
	assert.Equal(t, "refs/heads/main", target["destinationReference"])
}

func TestHandler_ListPullRequests_StatusFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		filterStatus string
		wantCount    int
		wantHTTPCode int
	}{
		{
			name:         "all_no_filter",
			filterStatus: "",
			wantCount:    3,
			wantHTTPCode: http.StatusOK,
		},
		{
			name:         "open_only",
			filterStatus: "OPEN",
			wantCount:    2,
			wantHTTPCode: http.StatusOK,
		},
		{
			name:         "closed_only",
			filterStatus: "CLOSED",
			wantCount:    1,
			wantHTTPCode: http.StatusOK,
		},
		{
			name:         "invalid_status",
			filterStatus: "INVALID",
			wantHTTPCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			doRequest(t, h, "CreateRepository", map[string]any{"repositoryName": "repo"})

			// Seed 2 OPEN + 1 CLOSED PR.
			createPR := func() string {
				rec := doRequest(t, h, "CreatePullRequest", map[string]any{
					"title": "PR",
					"targets": []map[string]any{
						{"repositoryName": "repo", "sourceReference": "refs/heads/feat"},
					},
				})
				require.Equal(t, http.StatusOK, rec.Code)

				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

				return resp["pullRequest"].(map[string]any)["pullRequestId"].(string)
			}

			createPR()
			createPR()
			closedID := createPR()

			doRequest(t, h, "UpdatePullRequestStatus", map[string]any{
				"pullRequestId":     closedID,
				"pullRequestStatus": "CLOSED",
			})

			body := map[string]any{"repositoryName": "repo"}
			if tt.filterStatus != "" {
				body["pullRequestStatus"] = tt.filterStatus
			}
			rec := doRequest(t, h, "ListPullRequests", body)
			assert.Equal(t, tt.wantHTTPCode, rec.Code)

			if tt.wantHTTPCode == http.StatusOK {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				ids := resp["pullRequestIds"].([]any)
				assert.Len(t, ids, tt.wantCount)
			}
		})
	}
}

// TestHandler_ListPullRequests_ClosedFilterIncludesMerged verifies that a
// merged pull request is returned by a pullRequestStatus=CLOSED filter.
// aws-sdk-go-v2/service/codecommit@v1.36.4's types.PullRequestStatusEnum has
// exactly two members, OPEN and CLOSED -- there is no MERGED status on the
// wire (UpdatePullRequestStatusInput's own doc comment: "The only valid
// operations are to update the status from OPEN to OPEN, OPEN to CLOSED or
// from CLOSED to CLOSED"). A merge is a terminal CLOSED, distinguished from
// an explicit close only via PullRequestTarget.MergeMetadata (types.go:936),
// not via a distinct status value a real client could ever request.
func TestHandler_ListPullRequests_ClosedFilterIncludesMerged(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	prID := setupPR(t, h, "merge-filter-repo")

	rec := doRequest(t, h, "MergePullRequestByFastForward", map[string]any{"pullRequestId": prID})
	require.Equal(t, http.StatusOK, rec.Code)

	var mergeResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &mergeResp))
	mergedPR := mergeResp["pullRequest"].(map[string]any)
	assert.Equal(
		t, "CLOSED", mergedPR["pullRequestStatus"],
		"a merged PR's status must be the real CLOSED enum value, not a fabricated MERGED one",
	)

	rec = doRequest(t, h, "ListPullRequests", map[string]any{
		"repositoryName":    "merge-filter-repo",
		"pullRequestStatus": "CLOSED",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var listResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
	ids := listResp["pullRequestIds"].([]any)
	require.Len(t, ids, 1)
	assert.Equal(t, prID, ids[0])
}

func TestHandler_ListPullRequests_NumericDescendingOrder(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateRepository", map[string]any{"repositoryName": "repo"})

	// Create 3 PRs — expect IDs 1, 2, 3.
	for i := range 3 {
		doRequest(t, h, "CreatePullRequest", map[string]any{
			"title": fmt.Sprintf("PR %d", i),
			"targets": []map[string]any{
				{"repositoryName": "repo", "sourceReference": "refs/heads/feat"},
			},
		})
	}

	rec := doRequest(t, h, "ListPullRequests", map[string]any{"repositoryName": "repo"})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	ids := resp["pullRequestIds"].([]any)
	require.Len(t, ids, 3)

	// Should be descending: "3", "2", "1".
	assert.Equal(t, "3", ids[0])
	assert.Equal(t, "2", ids[1])
	assert.Equal(t, "1", ids[2])
}

func TestHandler_ListPullRequests_RepoNotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "ListPullRequests", map[string]any{"repositoryName": "no-such-repo"})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandler_MergePullRequest_AlreadyMerged(t *testing.T) {
	t.Parallel()

	strategies := []string{
		"MergePullRequestByFastForward",
		"MergePullRequestBySquash",
		"MergePullRequestByThreeWay",
	}

	for _, strategy := range strategies {
		t.Run(strategy, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			doRequest(t, h, "CreateRepository", map[string]any{"repositoryName": "repo"})

			rec := doRequest(t, h, "CreatePullRequest", map[string]any{
				"title": "PR",
				"targets": []map[string]any{
					{"repositoryName": "repo", "sourceReference": "refs/heads/feat"},
				},
			})
			require.Equal(t, http.StatusOK, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			prID := resp["pullRequest"].(map[string]any)["pullRequestId"].(string)

			// First merge — should succeed.
			rec = doRequest(t, h, strategy, map[string]any{"pullRequestId": prID})
			require.Equal(t, http.StatusOK, rec.Code)

			// Second merge — should fail with PullRequestAlreadyClosedException.
			rec = doRequest(t, h, strategy, map[string]any{"pullRequestId": prID})
			assert.Equal(t, http.StatusBadRequest, rec.Code)

			var errResp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
			assert.Equal(t, "PullRequestAlreadyClosedException", errResp["__type"])
		})
	}
}

func TestHandler_MergePullRequest_StatusBecomesClosed(t *testing.T) {
	t.Parallel()

	strategies := []string{
		"MergePullRequestByFastForward",
		"MergePullRequestBySquash",
		"MergePullRequestByThreeWay",
	}

	for _, strategy := range strategies {
		t.Run(strategy, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			doRequest(t, h, "CreateRepository", map[string]any{"repositoryName": "repo"})

			rec := doRequest(t, h, "CreatePullRequest", map[string]any{
				"title": "PR",
				"targets": []map[string]any{
					{"repositoryName": "repo", "sourceReference": "refs/heads/feat"},
				},
			})
			require.Equal(t, http.StatusOK, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			prID := resp["pullRequest"].(map[string]any)["pullRequestId"].(string)

			rec = doRequest(t, h, strategy, map[string]any{"pullRequestId": prID})
			require.Equal(t, http.StatusOK, rec.Code)

			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			mergedPR := resp["pullRequest"].(map[string]any)
			assert.Equal(t, "CLOSED", mergedPR["pullRequestStatus"])
		})
	}
}

func TestHandler_UpdatePullRequestStatus(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	prID := setupPR(t, h, "pr-repo-6")

	rec := doRequest(t, h, "UpdatePullRequestStatus", map[string]any{
		"pullRequestId":     prID,
		"pullRequestStatus": "CLOSED",
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	pr := resp["pullRequest"].(map[string]any)
	assert.Equal(t, "CLOSED", pr["pullRequestStatus"])
}

func TestHandler_UpdatePullRequestStatus_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		status     string
		wantStatus int
	}{
		{
			name:       "open_valid",
			status:     "OPEN",
			wantStatus: http.StatusOK,
		},
		{
			name:       "closed_valid",
			status:     "CLOSED",
			wantStatus: http.StatusOK,
		},
		{
			name:       "merged_invalid",
			status:     "MERGED",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty_invalid",
			status:     "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "arbitrary_invalid",
			status:     "DONE",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			doRequest(t, h, "CreateRepository", map[string]any{"repositoryName": "repo"})

			rec := doRequest(t, h, "CreatePullRequest", map[string]any{
				"title": "PR",
				"targets": []map[string]any{
					{"repositoryName": "repo", "sourceReference": "refs/heads/feat"},
				},
			})
			require.Equal(t, http.StatusOK, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			prID := resp["pullRequest"].(map[string]any)["pullRequestId"].(string)

			rec = doRequest(t, h, "UpdatePullRequestStatus", map[string]any{
				"pullRequestId":     prID,
				"pullRequestStatus": tt.status,
			})
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestHandler_UpdatePullRequestStatus_TableDriven's three rejection cases used
// to assert wantErrType "InvalidParameterException" -- wrong: UpdatePullRequestStatus's
// own deserializer (codecommit@v1.36.4 deserializers.go,
// awsAwsjson11_deserializeOpErrorUpdatePullRequestStatus) has no case for that
// code, only InvalidPullRequestStatusException (gopherstack-yatn). Strengthened,
// not weakened: the assertion now checks the code the real SDK would actually parse.
func TestHandler_UpdatePullRequestStatus_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		status      string
		wantErrType string
		wantStatus  int
	}{
		{name: "close_pr", status: "CLOSED", wantStatus: http.StatusOK},
		{name: "reopen_pr", status: "OPEN", wantStatus: http.StatusOK},
		{
			name:        "merged_rejected",
			status:      "MERGED",
			wantStatus:  http.StatusBadRequest,
			wantErrType: "InvalidPullRequestStatusException",
		},
		{
			name:        "empty_rejected",
			status:      "",
			wantStatus:  http.StatusBadRequest,
			wantErrType: "InvalidPullRequestStatusException",
		},
		{
			name:        "bad_value_rejected",
			status:      "DONE",
			wantStatus:  http.StatusBadRequest,
			wantErrType: "InvalidPullRequestStatusException",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			prID := setupPR(t, h, "repo")

			rec := doRequest(t, h, "UpdatePullRequestStatus", map[string]any{
				"pullRequestId":     prID,
				"pullRequestStatus": tt.status,
			})
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantErrType != "" {
				var errResp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
				assert.Equal(t, tt.wantErrType, errResp["__type"])
			}
		})
	}
}

func TestHandler_PullRequest_FieldsPresent(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "CreatePullRequest", map[string]any{
		"title":              "My PR",
		"description":        "Details here",
		"clientRequestToken": "tok-123",
		"targets": []map[string]any{
			{
				"repositoryName":       "repo",
				"sourceReference":      "refs/heads/feature",
				"destinationReference": "refs/heads/main",
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	pr, ok := resp["pullRequest"].(map[string]any)
	require.True(t, ok)

	assert.NotEmpty(t, pr["pullRequestId"])
	assert.Equal(t, "My PR", pr["title"])
	assert.Equal(t, "Details here", pr["description"])
	assert.Equal(t, "OPEN", pr["pullRequestStatus"])
	assert.NotNil(t, pr["authorArn"])
	assert.NotNil(t, pr["clientRequestToken"])
	assert.NotNil(t, pr["revisionId"])
	assert.NotNil(t, pr["creationDate"])
	assert.NotNil(t, pr["lastActivityDate"])

	targets, ok := pr["pullRequestTargets"].([]any)
	require.True(t, ok)
	require.Len(t, targets, 1)

	target := targets[0].(map[string]any)
	assert.Equal(t, "repo", target["repositoryName"])
	assert.Equal(t, "refs/heads/feature", target["sourceReference"])
	assert.Equal(t, "refs/heads/main", target["destinationReference"])
}

func TestHandler_UpdatePullRequestDescription(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	prID := setupPR(t, h, "pr-repo-5")

	rec := doRequest(t, h, "UpdatePullRequestDescription", map[string]any{
		"pullRequestId": prID,
		"description":   "new description",
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	pr := resp["pullRequest"].(map[string]any)
	assert.Equal(t, "new description", pr["description"])
}

func TestHandler_UpdatePullRequestDescription_Reflected(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	prID := setupPR(t, h, "repo")

	rec := doRequest(t, h, "UpdatePullRequestDescription", map[string]any{
		"pullRequestId": prID,
		"description":   "A much better description",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	pr := resp["pullRequest"].(map[string]any)
	assert.Equal(t, "A much better description", pr["description"])
}

func TestHandler_UpdatePullRequestTitle(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	prID := setupPR(t, h, "pr-repo-7")

	rec := doRequest(t, h, "UpdatePullRequestTitle", map[string]any{
		"pullRequestId": prID,
		"title":         "updated title",
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	pr := resp["pullRequest"].(map[string]any)
	assert.Equal(t, "updated title", pr["title"])
}

func TestHandler_UpdatePullRequestTitle_Reflected(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	prID := setupPR(t, h, "repo")

	rec := doRequest(t, h, "UpdatePullRequestTitle", map[string]any{
		"pullRequestId": prID,
		"title":         "Updated Title",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	pr := resp["pullRequest"].(map[string]any)
	assert.Equal(t, "Updated Title", pr["title"])
	assert.Equal(t, prID, pr["pullRequestId"])
}

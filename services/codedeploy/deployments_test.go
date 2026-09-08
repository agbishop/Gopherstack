package codedeploy_test

import (
	"encoding/json"
	"net/http"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/codedeploy"
)

// deployIDRe matches the AWS CodeDeploy deployment ID format: d-[A-Z0-9]{9}.
var deployIDRe = regexp.MustCompile(`^d-[A-Z0-9]{9}$`)

func TestHandler_CreateDeployment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input       map[string]any
		setup       func(h *codedeploy.Handler)
		name        string
		wantErrType string
		wantStatus  int
		wantID      bool
	}{
		{
			name: "success",
			setup: func(h *codedeploy.Handler) {
				_, err := h.Backend.CreateApplication("my-app", "Server", nil)
				if err != nil {
					panic(err)
				}
				_, err = createDG(h.Backend, "my-app", "my-dg", "", "", nil)
				if err != nil {
					panic(err)
				}
			},
			input: map[string]any{
				"applicationName":     "my-app",
				"deploymentGroupName": "my-dg",
				"description":         "Test deployment",
			},
			wantStatus: http.StatusOK,
			wantID:     true,
		},
		{
			name: "app_not_found",
			input: map[string]any{
				"applicationName":     "nonexistent-app",
				"deploymentGroupName": "my-dg",
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "success_valid_file_exists_behavior",
			setup: func(h *codedeploy.Handler) {
				_, err := h.Backend.CreateApplication("my-app", "Server", nil)
				if err != nil {
					panic(err)
				}
				_, err = createDG(h.Backend, "my-app", "my-dg", "", "", nil)
				if err != nil {
					panic(err)
				}
			},
			input: map[string]any{
				"applicationName":     "my-app",
				"deploymentGroupName": "my-dg",
				"fileExistsBehavior":  "OVERWRITE",
			},
			wantStatus: http.StatusOK,
			wantID:     true,
		},
		{
			name: "invalid_file_exists_behavior",
			setup: func(h *codedeploy.Handler) {
				_, err := h.Backend.CreateApplication("my-app", "Server", nil)
				if err != nil {
					panic(err)
				}
				_, err = createDG(h.Backend, "my-app", "my-dg", "", "", nil)
				if err != nil {
					panic(err)
				}
			},
			input: map[string]any{
				"applicationName":     "my-app",
				"deploymentGroupName": "my-dg",
				"fileExistsBehavior":  "BOGUS",
			},
			wantStatus:  http.StatusBadRequest,
			wantErrType: "InvalidFileExistsBehaviorException",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			if tt.setup != nil {
				tt.setup(h)
			}

			rec := doRequest(t, h, "CreateDeployment", tt.input)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantID {
				var resp map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.NotEmpty(t, resp["deploymentId"])
				assert.True(t, len(resp["deploymentId"]) > 2 && resp["deploymentId"][:2] == "d-")
			}

			if tt.wantErrType != "" {
				var resp map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Equal(t, tt.wantErrType, resp["__type"])
			}
		})
	}
}

func TestDeploymentGroups_NotFoundOnCreateDeploymentError(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, _ = h.Backend.CreateApplication("app-for-deploy", "Server", nil)

	rec := doRequest(t, h, "CreateDeployment", map[string]any{
		"applicationName":     "app-for-deploy",
		"deploymentGroupName": "missing-dg",
	})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestDeployments_CreateIDFormat(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, _ = h.Backend.CreateApplication("app", "Server", nil)
	_, _ = createDG(h.Backend, "app", "dg", "", "", nil)

	for range 10 {
		rec := doRequest(t, h, "CreateDeployment", map[string]any{
			"applicationName":     "app",
			"deploymentGroupName": "dg",
		})
		require.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]string
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		id := resp["deploymentId"]
		assert.True(t, deployIDRe.MatchString(id), "deploymentId %q does not match pattern d-[A-Z0-9]{9}", id)
	}
}

// TestDeployments_CreateIDFormat_TableDriven covers the same deployment-ID
// format guarantee as TestDeployments_CreateIDFormat but as parallel
// subtests via t.Run, exercising the same path with independent goroutines.
func TestDeployments_CreateIDFormat_TableDriven(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createAppAndDG(t, h, "id-app", "id-dg")

	tests := []struct{ name string }{
		{"first"},
		{"second"},
		{"third"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := doRequest(t, h, "CreateDeployment", map[string]any{
				"applicationName":     "id-app",
				"deploymentGroupName": "id-dg",
			})
			require.Equal(t, http.StatusOK, rec.Code)

			var out map[string]string
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			assert.Regexp(t, deployIDRe, out["deploymentId"],
				"deploymentId must match d-[A-Z0-9]{9}")
		})
	}
}

func TestDeployments_CreateExtraFields(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, _ = h.Backend.CreateApplication("app", "Server", nil)
	_, _ = createDG(h.Backend, "app", "dg", "", "", nil)

	rec := doRequest(t, h, "CreateDeployment", map[string]any{
		"applicationName":               "app",
		"deploymentGroupName":           "dg",
		"description":                   "my-description",
		"fileExistsBehavior":            "OVERWRITE",
		"updateOutdatedInstancesOnly":   true,
		"ignoreApplicationStopFailures": true,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var createResp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))

	rec2 := doRequest(t, h, "GetDeployment", map[string]any{"deploymentId": createResp["deploymentId"]})
	require.Equal(t, http.StatusOK, rec2.Code)

	var getResp struct {
		DeploymentInfo struct {
			Description                   string `json:"description"`
			FileExistsBehavior            string `json:"fileExistsBehavior"`
			UpdateOutdatedInstancesOnly   bool   `json:"updateOutdatedInstancesOnly"`
			IgnoreApplicationStopFailures bool   `json:"ignoreApplicationStopFailures"`
		} `json:"deploymentInfo"`
	}
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &getResp))

	assert.Equal(t, "my-description", getResp.DeploymentInfo.Description)
	assert.Equal(t, "OVERWRITE", getResp.DeploymentInfo.FileExistsBehavior)
	assert.True(t, getResp.DeploymentInfo.UpdateOutdatedInstancesOnly)
	assert.True(t, getResp.DeploymentInfo.IgnoreApplicationStopFailures)
}

func TestDeployments_S3RevisionRoundTrip(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createAppAndDG(t, h, "rev-app", "rev-dg")

	createRec := doRequest(t, h, "CreateDeployment", map[string]any{
		"applicationName":     "rev-app",
		"deploymentGroupName": "rev-dg",
		"revision": map[string]any{
			"revisionType": "S3",
			"s3Location": map[string]any{
				"bucket":     "my-bucket",
				"key":        "app/bundle.zip",
				"bundleType": "zip",
				"eTag":       "abc123",
				"version":    "v1",
			},
		},
	})
	require.Equal(t, http.StatusOK, createRec.Code)

	var createOut map[string]string
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createOut))
	deployID := createOut["deploymentId"]
	require.NotEmpty(t, deployID)

	getRec := doRequest(t, h, "GetDeployment", map[string]any{"deploymentId": deployID})
	require.Equal(t, http.StatusOK, getRec.Code)

	var getOut struct {
		DeploymentInfo struct {
			Revision struct {
				RevisionType string `json:"revisionType"`
				S3Location   struct {
					Bucket     string `json:"bucket"`
					Key        string `json:"key"`
					BundleType string `json:"bundleType"`
					ETag       string `json:"eTag"`
					Version    string `json:"version"`
				} `json:"s3Location"`
			} `json:"revision"`
		} `json:"deploymentInfo"`
	}
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getOut))

	rev := getOut.DeploymentInfo.Revision
	assert.Equal(t, "S3", rev.RevisionType, "revisionType must round-trip")
	assert.Equal(t, "my-bucket", rev.S3Location.Bucket, "s3Location.bucket must round-trip")
	assert.Equal(t, "app/bundle.zip", rev.S3Location.Key, "s3Location.key must round-trip")
	assert.Equal(t, "zip", rev.S3Location.BundleType, "s3Location.bundleType must round-trip")
	assert.Equal(t, "abc123", rev.S3Location.ETag, "s3Location.eTag must round-trip")
	assert.Equal(t, "v1", rev.S3Location.Version, "s3Location.version must round-trip")
}

func TestDeployments_GitHubRevisionRoundTrip(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createAppAndDG(t, h, "gh-app", "gh-dg")

	createRec := doRequest(t, h, "CreateDeployment", map[string]any{
		"applicationName":     "gh-app",
		"deploymentGroupName": "gh-dg",
		"revision": map[string]any{
			"revisionType": "GitHub",
			"gitHubLocation": map[string]any{
				"repository": "owner/repo",
				"commitId":   "deadbeef1234",
			},
		},
	})
	require.Equal(t, http.StatusOK, createRec.Code)

	var createOut map[string]string
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createOut))
	deployID := createOut["deploymentId"]

	getRec := doRequest(t, h, "GetDeployment", map[string]any{"deploymentId": deployID})
	require.Equal(t, http.StatusOK, getRec.Code)

	var getOut struct {
		DeploymentInfo struct {
			Revision struct {
				RevisionType   string `json:"revisionType"`
				GitHubLocation struct {
					Repository string `json:"repository"`
					CommitID   string `json:"commitId"`
				} `json:"gitHubLocation"`
			} `json:"revision"`
		} `json:"deploymentInfo"`
	}
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getOut))

	rev := getOut.DeploymentInfo.Revision
	assert.Equal(t, "GitHub", rev.RevisionType)
	assert.Equal(t, "owner/repo", rev.GitHubLocation.Repository)
	assert.Equal(t, "deadbeef1234", rev.GitHubLocation.CommitID)
}

func TestHandler_GetDeployment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(h *codedeploy.Handler) string
		input      func(id string) map[string]any
		name       string
		wantStatus int
	}{
		{
			name: "success",
			setup: func(h *codedeploy.Handler) string {
				_, _ = h.Backend.CreateApplication("my-app", "Server", nil)
				_, _ = createDG(h.Backend, "my-app", "my-dg", "", "", nil)
				d, _ := createDeploy(h.Backend, "my-app", "my-dg", "test", "user")

				return d.DeploymentID
			},
			input: func(id string) map[string]any {
				return map[string]any{"deploymentId": id}
			},
			wantStatus: http.StatusOK,
		},
		{
			name:  "not_found",
			setup: func(_ *codedeploy.Handler) string { return "d-nonexistent" },
			input: func(id string) map[string]any {
				return map[string]any{"deploymentId": id}
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:  "empty_id",
			setup: func(_ *codedeploy.Handler) string { return "" },
			input: func(_ string) map[string]any {
				return map[string]any{}
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			deployID := tt.setup(h)

			rec := doRequest(t, h, "GetDeployment", tt.input(deployID))
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestDeployments_InfoIncludesConfigName(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	_, _ = h.Backend.CreateApplication("my-app", "Server", nil)
	_, _ = createDG(h.Backend, "my-app", "my-dg", "", "CodeDeployDefault.AllAtOnce", nil)
	d, _ := createDeploy(h.Backend, "my-app", "my-dg", "", "")

	rec := doRequest(t, h, "GetDeployment", map[string]any{"deploymentId": d.DeploymentID})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	info := resp["deploymentInfo"]
	assert.Equal(t, "CodeDeployDefault.AllAtOnce", info["deploymentConfigName"])
}

func TestDeployments_GetDeploymentOverview(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createAppAndDG(t, h, "ov-app", "ov-dg")

	createRec := doRequest(t, h, "CreateDeployment", map[string]any{
		"applicationName":     "ov-app",
		"deploymentGroupName": "ov-dg",
	})
	require.Equal(t, http.StatusOK, createRec.Code)

	var createOut map[string]string
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createOut))

	getRec := doRequest(t, h, "GetDeployment", map[string]any{"deploymentId": createOut["deploymentId"]})
	require.Equal(t, http.StatusOK, getRec.Code)

	var getOut struct {
		DeploymentInfo struct {
			DeploymentOverview map[string]any `json:"deploymentOverview"`
		} `json:"deploymentInfo"`
	}
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getOut))

	ov := getOut.DeploymentInfo.DeploymentOverview
	assert.NotNil(t, ov, "GetDeployment must include deploymentOverview")
	_, hasSucceeded := ov["Succeeded"]
	assert.True(t, hasSucceeded, "deploymentOverview must include Succeeded field")
}

func TestHandler_ListDeployments(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Empty list.
	rec := doRequest(t, h, "ListDeployments", map[string]any{})
	assert.Equal(t, http.StatusOK, rec.Code)

	// Create some deployments.
	_, _ = h.Backend.CreateApplication("my-app", "Server", nil)
	_, _ = createDG(h.Backend, "my-app", "my-dg", "", "", nil)
	_, _ = createDeploy(h.Backend, "my-app", "my-dg", "", "")

	rec = doRequest(t, h, "ListDeployments", map[string]any{
		"applicationName":     "my-app",
		"deploymentGroupName": "my-dg",
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	deployments, ok := resp["deployments"].([]any)
	require.True(t, ok)
	assert.Len(t, deployments, 1)
}

func TestDeployments_SortedList(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	_, _ = h.Backend.CreateApplication("my-app", "Server", nil)
	_, _ = createDG(h.Backend, "my-app", "my-dg", "", "", nil)
	d1, _ := createDeploy(h.Backend, "my-app", "my-dg", "", "")
	d2, _ := createDeploy(h.Backend, "my-app", "my-dg", "", "")

	rec := doRequest(t, h, "ListDeployments", map[string]any{"applicationName": "my-app"})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Deployments []string `json:"deployments"`
	}

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Len(t, resp.Deployments, 2)

	assert.Contains(t, resp.Deployments, d1.DeploymentID)
	assert.Contains(t, resp.Deployments, d2.DeploymentID)

	// Verify the output is sorted
	assert.LessOrEqual(t, resp.Deployments[0], resp.Deployments[1], "deployments should be sorted")
}

func TestDeployments_ListStatusFilter(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, _ = h.Backend.CreateApplication("app", "Server", nil)
	_, _ = createDG(h.Backend, "app", "dg", "", "", nil)

	// Create one deployment then stop it.
	d1, _ := createDeploy(h.Backend, "app", "dg", "", "")
	_ = h.Backend.StopDeployment(d1.DeploymentID)

	// Create another deployment (stays Succeeded).
	_, _ = createDeploy(h.Backend, "app", "dg", "", "")

	tests := []struct {
		name    string
		filter  []string
		wantLen int
	}{
		{name: "all", filter: nil, wantLen: 2},
		{name: "succeeded_only", filter: []string{"Succeeded"}, wantLen: 1},
		{name: "stopped_only", filter: []string{"Stopped"}, wantLen: 1},
		{name: "failed_only", filter: []string{"Failed"}, wantLen: 0},
		{name: "stopped_or_succeeded", filter: []string{"Stopped", "Succeeded"}, wantLen: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			input := map[string]any{"applicationName": "app", "deploymentGroupName": "dg"}
			if tt.filter != nil {
				input["includeOnlyStatuses"] = tt.filter
			}

			rec := doRequest(t, h, "ListDeployments", input)
			require.Equal(t, http.StatusOK, rec.Code)

			var resp struct {
				Deployments []string `json:"deployments"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Len(t, resp.Deployments, tt.wantLen)
		})
	}
}

// TestDeployments_ListExternalIDFilter guards against ListDeployments
// silently ignoring externalId: nothing in this backend can ever set a
// deployment's ExternalID (CreateDeploymentInput has no such field --
// api_op_CreateDeployment.go), so a non-empty filter must match zero
// deployments rather than returning the unfiltered list.
func TestDeployments_ListExternalIDFilter(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, _ = h.Backend.CreateApplication("app", "Server", nil)
	_, _ = createDG(h.Backend, "app", "dg", "", "", nil)
	_, _ = createDeploy(h.Backend, "app", "dg", "", "")

	tests := []struct {
		name       string
		externalID string
		wantLen    int
	}{
		{name: "no_filter", externalID: "", wantLen: 1},
		{name: "nonexistent_external_id", externalID: "pipeline-external-id", wantLen: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			input := map[string]any{"applicationName": "app", "deploymentGroupName": "dg"}
			if tt.externalID != "" {
				input["externalId"] = tt.externalID
			}

			rec := doRequest(t, h, "ListDeployments", input)
			require.Equal(t, http.StatusOK, rec.Code)

			var resp struct {
				Deployments []string `json:"deployments"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Len(t, resp.Deployments, tt.wantLen)
		})
	}
}

// TestHandler_ContinueDeployment covers the real ContinueDeployment
// preconditions: aws-sdk-go-v2/service/codedeploy@v1.38.4/types/errors.go:556-557
// ("The deployment does not have a status of Ready and can't continue yet.")
// and :221 ("The deployment is already complete."). CreateDeployment in this
// backend completes synchronously (statusSucceeded), so a deployment only
// ever reaches Ready via direct seeding -- exercising the not-ready and
// already-completed paths this backend previously skipped entirely.
func TestHandler_ContinueDeployment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup       func(h *codedeploy.Handler) string
		input       func(deployID string) map[string]any
		name        string
		wantErrType string
		wantStatus  int
	}{
		{
			name: "success_ready_state",
			setup: func(h *codedeploy.Handler) string {
				_, _ = h.Backend.CreateApplication("my-app", "Server", nil)
				_, _ = createDG(h.Backend, "my-app", "my-dg", "", "", nil)
				d := &codedeploy.Deployment{
					DeploymentID:        "d-READY0001",
					ApplicationName:     "my-app",
					DeploymentGroupName: "my-dg",
					Status:              "Ready",
				}
				h.Backend.AddDeploymentInternal(d)

				return d.DeploymentID
			},
			input: func(deployID string) map[string]any {
				return map[string]any{
					"deploymentId":       deployID,
					"deploymentWaitType": "READY_WAIT",
				}
			},
			wantStatus: http.StatusOK,
		},
		{
			name:  "missing_deployment_id",
			setup: func(_ *codedeploy.Handler) string { return "" },
			input: func(_ string) map[string]any {
				return map[string]any{}
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:  "deployment_not_found",
			setup: func(_ *codedeploy.Handler) string { return "d-notexist" },
			input: func(deployID string) map[string]any {
				return map[string]any{"deploymentId": deployID}
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "already_completed",
			setup: func(h *codedeploy.Handler) string {
				_, _ = h.Backend.CreateApplication("my-app", "Server", nil)
				_, _ = createDG(h.Backend, "my-app", "my-dg", "", "", nil)
				d, _ := createDeploy(h.Backend, "my-app", "my-dg", "", "")

				return d.DeploymentID
			},
			input: func(deployID string) map[string]any {
				return map[string]any{"deploymentId": deployID}
			},
			wantStatus:  http.StatusConflict,
			wantErrType: "DeploymentAlreadyCompletedException",
		},
		{
			name: "not_in_ready_state",
			setup: func(h *codedeploy.Handler) string {
				_, _ = h.Backend.CreateApplication("my-app", "Server", nil)
				_, _ = createDG(h.Backend, "my-app", "my-dg", "", "", nil)
				d := &codedeploy.Deployment{
					DeploymentID:        "d-INPROG001",
					ApplicationName:     "my-app",
					DeploymentGroupName: "my-dg",
					Status:              "InProgress",
				}
				h.Backend.AddDeploymentInternal(d)

				return d.DeploymentID
			},
			input: func(deployID string) map[string]any {
				return map[string]any{"deploymentId": deployID}
			},
			wantStatus:  http.StatusConflict,
			wantErrType: "DeploymentIsNotInReadyStateException",
		},
		{
			name: "invalid_wait_type",
			setup: func(h *codedeploy.Handler) string {
				_, _ = h.Backend.CreateApplication("my-app", "Server", nil)
				_, _ = createDG(h.Backend, "my-app", "my-dg", "", "", nil)
				d := &codedeploy.Deployment{
					DeploymentID:        "d-READY0002",
					ApplicationName:     "my-app",
					DeploymentGroupName: "my-dg",
					Status:              "Ready",
				}
				h.Backend.AddDeploymentInternal(d)

				return d.DeploymentID
			},
			input: func(deployID string) map[string]any {
				return map[string]any{
					"deploymentId":       deployID,
					"deploymentWaitType": "BOGUS_WAIT",
				}
			},
			wantStatus:  http.StatusBadRequest,
			wantErrType: "InvalidDeploymentWaitTypeException",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			deployID := tt.setup(h)

			rec := doRequest(t, h, "ContinueDeployment", tt.input(deployID))
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantErrType != "" {
				var resp map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Equal(t, tt.wantErrType, resp["__type"])
			}
		})
	}
}

// TestDeployments_StopDeployment_AlreadyStopped proves stopping an
// already-stopped deployment is rejected with DeploymentAlreadyCompletedException
// (types/errors.go:221, "The deployment is already complete.") instead of
// silently succeeding a second time.
func TestDeployments_StopDeployment_AlreadyStopped(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createAppAndDG(t, h, "stop-twice-app", "stop-twice-dg")

	createRec := doRequest(t, h, "CreateDeployment", map[string]any{
		"applicationName":     "stop-twice-app",
		"deploymentGroupName": "stop-twice-dg",
	})
	require.Equal(t, http.StatusOK, createRec.Code)

	var createOut map[string]string
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createOut))
	deployID := createOut["deploymentId"]

	firstStop := doRequest(t, h, "StopDeployment", map[string]any{"deploymentId": deployID})
	require.Equal(t, http.StatusOK, firstStop.Code)

	secondStop := doRequest(t, h, "StopDeployment", map[string]any{"deploymentId": deployID})
	assert.Equal(t, http.StatusConflict, secondStop.Code)

	var errResp map[string]string
	require.NoError(t, json.Unmarshal(secondStop.Body.Bytes(), &errResp))
	assert.Equal(t, "DeploymentAlreadyCompletedException", errResp["__type"])
}

func TestDeployments_StopDeploymentStatus(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createAppAndDG(t, h, "stop-app", "stop-dg")

	createRec := doRequest(t, h, "CreateDeployment", map[string]any{
		"applicationName":     "stop-app",
		"deploymentGroupName": "stop-dg",
	})
	require.Equal(t, http.StatusOK, createRec.Code)

	var createOut map[string]string
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createOut))
	deployID := createOut["deploymentId"]

	stopRec := doRequest(t, h, "StopDeployment", map[string]any{
		"deploymentId": deployID,
	})
	require.Equal(t, http.StatusOK, stopRec.Code)

	var stopOut struct {
		Status string `json:"status"`
	}
	require.NoError(t, json.Unmarshal(stopRec.Body.Bytes(), &stopOut))
	assert.Equal(t, "Succeeded", stopOut.Status,
		"StopDeploymentOutput.status is the StopStatus enum (Pending|Succeeded), not the deployment status")

	getRec := doRequest(t, h, "GetDeployment", map[string]any{"deploymentId": deployID})
	require.Equal(t, http.StatusOK, getRec.Code)

	var getOut struct {
		DeploymentInfo struct {
			Status string `json:"status"`
		} `json:"deploymentInfo"`
	}
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getOut))
	assert.Equal(t, "Stopped", getOut.DeploymentInfo.Status,
		"GetDeployment must reflect Stopped status after StopDeployment")
}

func TestHandler_BatchGetDeployments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(h *codedeploy.Handler) []string
		input      func(ids []string) map[string]any
		name       string
		wantStatus int
		wantCount  int
	}{
		{
			name: "two_found",
			setup: func(h *codedeploy.Handler) []string {
				_, _ = h.Backend.CreateApplication("my-app", "Server", nil)
				_, _ = createDG(h.Backend, "my-app", "my-dg", "", "", nil)
				d1, _ := createDeploy(h.Backend, "my-app", "my-dg", "", "")
				d2, _ := createDeploy(h.Backend, "my-app", "my-dg", "", "")

				return []string{d1.DeploymentID, d2.DeploymentID, "d-notexist"}
			},
			input: func(ids []string) map[string]any {
				return map[string]any{"deploymentIds": ids}
			},
			wantStatus: http.StatusOK,
			wantCount:  2,
		},
		{
			name:  "missing_ids",
			setup: func(_ *codedeploy.Handler) []string { return nil },
			input: func(_ []string) map[string]any {
				return map[string]any{}
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			ids := tt.setup(h)

			rec := doRequest(t, h, "BatchGetDeployments", tt.input(ids))
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				infos, ok := resp["deploymentsInfo"].([]any)
				require.True(t, ok)
				assert.Len(t, infos, tt.wantCount)
			}
		})
	}
}

func TestDeployments_BatchGetReturnsRevision(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createAppAndDG(t, h, "batch-app", "batch-dg")

	createRec := doRequest(t, h, "CreateDeployment", map[string]any{
		"applicationName":     "batch-app",
		"deploymentGroupName": "batch-dg",
		"revision": map[string]any{
			"revisionType": "S3",
			"s3Location":   map[string]any{"bucket": "b", "key": "k", "bundleType": "zip"},
		},
	})
	require.Equal(t, http.StatusOK, createRec.Code)

	var createOut map[string]string
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createOut))

	batchRec := doRequest(t, h, "BatchGetDeployments", map[string]any{
		"deploymentIds": []string{createOut["deploymentId"]},
	})
	require.Equal(t, http.StatusOK, batchRec.Code)

	type batchRevision struct {
		RevisionType string `json:"revisionType"`
	}
	type batchItem struct {
		DeploymentOverview map[string]any `json:"deploymentOverview"`
		Revision           batchRevision  `json:"revision"`
	}
	var batchOut struct {
		DeploymentsInfo []batchItem `json:"deploymentsInfo"`
	}
	require.NoError(t, json.Unmarshal(batchRec.Body.Bytes(), &batchOut))
	require.Len(t, batchOut.DeploymentsInfo, 1)

	info := batchOut.DeploymentsInfo[0]
	assert.Equal(t, "S3", info.Revision.RevisionType,
		"BatchGetDeployments must include revision per deployment")
	assert.NotNil(t, info.DeploymentOverview,
		"BatchGetDeployments must include deploymentOverview per deployment")
}

// TestDeployments_SkipWaitTime_DeploymentNotFound verifies that
// SkipWaitTimeForInstanceTermination validates the deploymentId against the
// backend like its sibling deployment-scoped ops (GetDeploymentInstance,
// ListDeploymentTargets, etc.) instead of unconditionally succeeding for an
// unknown deployment.
func TestDeployments_SkipWaitTime_DeploymentNotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "SkipWaitTimeForInstanceTermination", map[string]any{"deploymentId": "d-NOTFOUND1"})
	require.Equal(t, http.StatusNotFound, rec.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "DeploymentDoesNotExistException", resp["__type"])
}

// TestDeployments_SkipWaitTime_DeploymentFound verifies the happy path still
// succeeds once the deployment exists, guarding against an over-eager
// existence check breaking the normal flow.
func TestDeployments_SkipWaitTime_DeploymentFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createAppAndDG(t, h, "hook-app", "hook-dg")

	createRec := doRequest(t, h, "CreateDeployment", map[string]any{
		"applicationName":     "hook-app",
		"deploymentGroupName": "hook-dg",
	})
	require.Equal(t, http.StatusOK, createRec.Code)

	var createOut map[string]string
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createOut))
	deployID := createOut["deploymentId"]

	skipRec := doRequest(t, h, "SkipWaitTimeForInstanceTermination", map[string]any{
		"deploymentId": deployID,
	})
	assert.Equal(t, http.StatusOK, skipRec.Code)
}

package emr_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/emr"
)

func TestEMR_ListSteps(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doEMRRequest(t, h, "ListSteps", map[string]any{"ClusterId": "j-123"})

	require.Equal(t, http.StatusOK, rec.Code)

	var out struct {
		Steps []any `json:"Steps"`
	}

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Empty(t, out.Steps)
}

func TestEMR_AddJobFlowSteps(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	createRec := doEMRRequest(t, h, "RunJobFlow", map[string]any{"Name": "step-cluster"})
	require.Equal(t, http.StatusOK, createRec.Code)
	var createOut struct {
		JobFlowID string `json:"JobFlowId"`
	}
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createOut))

	rec := doEMRRequest(t, h, "AddJobFlowSteps", map[string]any{
		"JobFlowId": createOut.JobFlowID,
		"Steps": []any{
			map[string]any{
				"Name":            "my-step",
				"ActionOnFailure": "CONTINUE",
				"HadoopJarStep":   map[string]any{"Jar": "command-runner.jar", "Args": []string{"spark-submit"}},
			},
		},
	})

	require.Equal(t, http.StatusOK, rec.Code)

	var out struct {
		StepIDs []string `json:"StepIds"`
	}

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.Len(t, out.StepIDs, 1)
	assert.Contains(t, out.StepIDs[0], "s-")
}

func TestEMR_AddJobFlowSteps_RejectsTerminatedCluster(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	createRec := doEMRRequest(t, h, "RunJobFlow", map[string]any{"Name": "terminated-step-cluster"})
	require.Equal(t, http.StatusOK, createRec.Code)

	var createOut struct {
		JobFlowID string `json:"JobFlowId"`
	}
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createOut))

	termRec := doEMRRequest(t, h, "TerminateJobFlows", map[string]any{
		"JobFlowIds": []string{createOut.JobFlowID},
	})
	require.Equal(t, http.StatusOK, termRec.Code)

	rec := doEMRRequest(t, h, "AddJobFlowSteps", map[string]any{
		"JobFlowId": createOut.JobFlowID,
		"Steps": []any{
			map[string]any{
				"Name":            "my-step",
				"ActionOnFailure": "CONTINUE",
				"HadoopJarStep":   map[string]any{"Jar": "command-runner.jar", "Args": []string{"spark-submit"}},
			},
		},
	})

	require.Equal(t, http.StatusBadRequest, rec.Code)

	var errBody struct {
		Type string `json:"__type"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errBody))
	assert.Equal(t, "InvalidRequestException", errBody.Type)

	listRec := doEMRRequest(t, h, "ListSteps", map[string]any{"ClusterId": createOut.JobFlowID})
	require.Equal(t, http.StatusOK, listRec.Code)

	var listOut struct {
		Steps []any `json:"Steps"`
	}
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listOut))
	assert.Empty(t, listOut.Steps)
}

func TestEMR_ListBootstrapActions(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	createRec := doEMRRequest(t, h, "RunJobFlow", map[string]any{"Name": "bootstrap-cluster"})
	require.Equal(t, http.StatusOK, createRec.Code)

	var createOut struct {
		JobFlowID string `json:"JobFlowId"`
	}
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createOut))

	rec := doEMRRequest(t, h, "ListBootstrapActions", map[string]any{
		"ClusterId": createOut.JobFlowID,
	})

	require.Equal(t, http.StatusOK, rec.Code)

	var out struct {
		BootstrapActions []any `json:"BootstrapActions"`
	}

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Empty(t, out.BootstrapActions)
}

func TestEMR_CancelSteps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		clusterID string
		wantCode  int
	}{
		{
			name:      "cancels steps on existing cluster",
			clusterID: "",
			wantCode:  http.StatusOK,
		},
		{
			name:      "returns error for non-existent cluster",
			clusterID: "j-NOTEXIST",
			wantCode:  http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			createRec := doEMRRequest(t, h, "RunJobFlow", map[string]any{"Name": "cancel-cluster"})
			require.Equal(t, http.StatusOK, createRec.Code)

			var createOut struct {
				JobFlowID string `json:"JobFlowId"`
			}
			require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createOut))

			clusterID := tt.clusterID
			if clusterID == "" {
				clusterID = createOut.JobFlowID
			}

			rec := doEMRRequest(t, h, "CancelSteps", map[string]any{
				"ClusterId": clusterID,
				"StepIds":   []string{"s-123"},
			})

			require.Equal(t, tt.wantCode, rec.Code)

			if tt.wantCode == http.StatusOK {
				var out struct {
					CancelStepsInfoList []any `json:"CancelStepsInfoList"`
				}
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
				assert.Empty(t, out.CancelStepsInfoList)
			}
		})
	}
}

func TestSteps(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	createRec := doEMRRequest(t, h, "RunJobFlow", map[string]any{"Name": "steps-cluster"})
	require.Equal(t, http.StatusOK, createRec.Code)

	var create struct {
		JobFlowID string `json:"JobFlowId"`
	}
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &create))
	clusterID := create.JobFlowID

	addRec := doEMRRequest(t, h, "AddJobFlowSteps", map[string]any{
		"JobFlowId": clusterID,
		"Steps": []any{
			map[string]any{
				"Name":            "step-one",
				"ActionOnFailure": "CONTINUE",
				"HadoopJarStep":   map[string]any{"Jar": "command-runner.jar"},
			},
			map[string]any{
				"Name":            "step-two",
				"ActionOnFailure": "TERMINATE_CLUSTER",
				"HadoopJarStep":   map[string]any{"Jar": "command-runner.jar"},
			},
		},
	})
	require.Equal(t, http.StatusOK, addRec.Code)

	var addOut struct {
		StepIDs []string `json:"StepIds"`
	}
	require.NoError(t, json.Unmarshal(addRec.Body.Bytes(), &addOut))
	require.Len(t, addOut.StepIDs, 2)
	assert.NotEmpty(t, addOut.StepIDs[0])

	listRec := doEMRRequest(t, h, "ListSteps", map[string]any{"ClusterId": clusterID})
	require.Equal(t, http.StatusOK, listRec.Code)

	var listOut struct {
		Steps []struct {
			ID string `json:"Id"`
		} `json:"Steps"`
	}
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listOut))
	assert.Len(t, listOut.Steps, 2)

	descRec := doEMRRequest(t, h, "DescribeStep", map[string]any{
		"ClusterId": clusterID,
		"StepId":    addOut.StepIDs[0],
	})
	require.Equal(t, http.StatusOK, descRec.Code)
}

func TestListBootstrapActions_RoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		bootstrapActions []map[string]any
		wantActions      []struct {
			Name       string
			ScriptPath string
			Args       []string
		}
	}{
		{
			name:             "no bootstrap actions returns empty list",
			bootstrapActions: nil,
		},
		{
			name: "single bootstrap action without args",
			bootstrapActions: []map[string]any{
				{
					"Name": "install-spark",
					"ScriptBootstrapAction": map[string]any{
						"Path": "s3://mybucket/bootstrap/install-spark.sh",
					},
				},
			},
			wantActions: []struct {
				Name       string
				ScriptPath string
				Args       []string
			}{
				{Name: "install-spark", ScriptPath: "s3://mybucket/bootstrap/install-spark.sh"},
			},
		},
		{
			name: "multiple bootstrap actions with args",
			bootstrapActions: []map[string]any{
				{
					"Name": "configure-hadoop",
					"ScriptBootstrapAction": map[string]any{
						"Path": "s3://mybucket/bootstrap/configure.sh",
						"Args": []string{"--heap-size", "4g"},
					},
				},
				{
					"Name": "install-python-libs",
					"ScriptBootstrapAction": map[string]any{
						"Path": "s3://mybucket/bootstrap/pip-install.sh",
						"Args": []string{"pandas", "numpy", "scikit-learn"},
					},
				},
			},
			wantActions: []struct {
				Name       string
				ScriptPath string
				Args       []string
			}{
				{
					Name:       "configure-hadoop",
					ScriptPath: "s3://mybucket/bootstrap/configure.sh",
					Args:       []string{"--heap-size", "4g"},
				},
				{
					Name:       "install-python-libs",
					ScriptPath: "s3://mybucket/bootstrap/pip-install.sh",
					Args:       []string{"pandas", "numpy", "scikit-learn"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			body := map[string]any{"Name": "bootstrap-cluster"}
			if tt.bootstrapActions != nil {
				body["BootstrapActions"] = tt.bootstrapActions
			}

			createRec := doEMRRequest(t, h, "RunJobFlow", body)
			require.Equal(t, http.StatusOK, createRec.Code)

			var createOut struct {
				JobFlowID string `json:"JobFlowId"`
			}
			require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createOut))

			listRec := doEMRRequest(t, h, "ListBootstrapActions", map[string]any{
				"ClusterId": createOut.JobFlowID,
			})
			require.Equal(t, http.StatusOK, listRec.Code)

			var listOut struct {
				BootstrapActions []struct {
					Name       string   `json:"Name"`
					ScriptPath string   `json:"ScriptPath"`
					Args       []string `json:"Args"`
				} `json:"BootstrapActions"`
			}
			require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listOut))

			if tt.wantActions == nil {
				assert.Empty(t, listOut.BootstrapActions)

				return
			}

			require.Len(t, listOut.BootstrapActions, len(tt.wantActions))
			for i, want := range tt.wantActions {
				got := listOut.BootstrapActions[i]
				assert.Equal(t, want.Name, got.Name, "action[%d].Name", i)
				assert.Equal(t, want.ScriptPath, got.ScriptPath, "action[%d].ScriptPath", i)
				assert.Equal(t, want.Args, got.Args, "action[%d].Args", i)
			}
		})
	}
}

func TestListBootstrapActions_Persistence(t *testing.T) {
	t.Parallel()

	src := emr.NewInMemoryBackend(testAccountID, testRegion)
	cluster, err := src.RunJobFlow(context.Background(), emr.RunJobFlowParams{
		Name: "ba-persist-cluster",
		BootstrapActions: []emr.BootstrapActionConfig{
			{
				Name: "install-lib",
				ScriptBootstrapAction: emr.BootstrapActionScript{
					Path: "s3://bucket/install.sh",
					Args: []string{"--flag"},
				},
			},
		},
	})
	require.NoError(t, err)

	snap := src.Snapshot(t.Context())
	require.NotNil(t, snap)

	dst := emr.NewInMemoryBackend("", "")
	require.NoError(t, dst.Restore(t.Context(), snap))

	cmds, _, err := dst.ListBootstrapActions(context.Background(), cluster.ID, "")
	require.NoError(t, err)
	require.Len(t, cmds, 1)
	assert.Equal(t, "install-lib", cmds[0].Name)
	assert.Equal(t, "s3://bucket/install.sh", cmds[0].ScriptPath)
	assert.Equal(t, []string{"--flag"}, cmds[0].Args)
}

// TestCancelSteps_ReturnsPerStepInfo verifies CancelSteps returns per-step status.
func TestCancelSteps_ReturnsPerStepInfo(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	createRec := doEMRRequest(t, h, "RunJobFlow", map[string]any{
		"Name": "cancel-test",
		"Steps": []map[string]any{
			{
				"Name":            "step1",
				"ActionOnFailure": "CONTINUE",
				"HadoopJarStep":   map[string]any{"Jar": "command-runner.jar"},
			},
			{
				"Name":            "step2",
				"ActionOnFailure": "CONTINUE",
				"HadoopJarStep":   map[string]any{"Jar": "command-runner.jar"},
			},
		},
	})
	require.Equal(t, http.StatusOK, createRec.Code)

	var create struct {
		JobFlowID string `json:"JobFlowId"`
	}
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &create))

	// List steps to get IDs
	listRec := doEMRRequest(t, h, "ListSteps", map[string]any{"ClusterId": create.JobFlowID})
	require.Equal(t, http.StatusOK, listRec.Code)

	var listOut struct {
		Steps []struct {
			ID string `json:"Id"`
		} `json:"Steps"`
	}
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listOut))
	require.NotEmpty(t, listOut.Steps)

	stepID := listOut.Steps[0].ID
	rec := doEMRRequest(t, h, "CancelSteps", map[string]any{
		"ClusterId": create.JobFlowID,
		"StepIds":   []string{stepID},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var out struct {
		CancelStepsInfoList []struct {
			StepID string `json:"StepId"`
			Status string `json:"Status"`
		} `json:"CancelStepsInfoList"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Len(t, out.CancelStepsInfoList, 1)
	assert.Equal(t, stepID, out.CancelStepsInfoList[0].StepID)
	assert.NotEmpty(t, out.CancelStepsInfoList[0].Status)
}

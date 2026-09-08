package emr_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/awstime"
	"github.com/blackbirdworks/gopherstack/services/emr"
)

func TestEMR_RunJobFlow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input   map[string]any
		name    string
		wantErr bool
	}{
		{
			name:  "creates cluster with name",
			input: map[string]any{"Name": "my-cluster", "ReleaseLabel": "emr-6.0.0"},
		},
		{
			name:  "creates cluster without release label uses default",
			input: map[string]any{"Name": "other-cluster"},
		},
		{
			name: "creates cluster with tags",
			input: map[string]any{
				"Name":         "tagged-cluster",
				"ReleaseLabel": "emr-6.0.0",
				"Tags":         []map[string]any{{"Key": "env", "Value": "test"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doEMRRequest(t, h, "RunJobFlow", tt.input)

			require.Equal(t, http.StatusOK, rec.Code)

			var out struct {
				JobFlowID  string `json:"JobFlowId"`
				ClusterArn string `json:"ClusterArn"`
			}

			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			assert.NotEmpty(t, out.JobFlowID)
			assert.Contains(t, out.ClusterArn, "elasticmapreduce")
		})
	}
}

func TestEMR_DescribeCluster(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		clusterID string
		wantCode  int
	}{
		{
			name:      "found",
			clusterID: "",
			wantCode:  http.StatusOK,
		},
		{
			name:      "not found",
			clusterID: "j-NOTEXIST",
			wantCode:  http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			// Create a cluster first
			createRec := doEMRRequest(t, h, "RunJobFlow", map[string]any{"Name": "test-cluster"})
			require.Equal(t, http.StatusOK, createRec.Code)

			var createOut struct {
				JobFlowID string `json:"JobFlowId"`
			}

			require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createOut))

			clusterID := tt.clusterID
			if clusterID == "" {
				clusterID = createOut.JobFlowID
			}

			rec := doEMRRequest(t, h, "DescribeCluster", map[string]any{"ClusterId": clusterID})

			require.Equal(t, tt.wantCode, rec.Code)

			if tt.wantCode == http.StatusOK {
				var out struct {
					Cluster struct {
						ID   string `json:"Id"`
						Name string `json:"Name"`
					} `json:"Cluster"`
				}

				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
				assert.Equal(t, clusterID, out.Cluster.ID)
				assert.Equal(t, "test-cluster", out.Cluster.Name)
			}
		})
	}
}

func TestEMR_ListClusters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		numClusters  int
		wantMinCount int
	}{
		{
			name:         "empty list",
			numClusters:  0,
			wantMinCount: 0,
		},
		{
			name:         "lists created clusters",
			numClusters:  2,
			wantMinCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			for range tt.numClusters {
				rec := doEMRRequest(t, h, "RunJobFlow", map[string]any{"Name": "cluster"})
				require.Equal(t, http.StatusOK, rec.Code)
			}

			rec := doEMRRequest(t, h, "ListClusters", map[string]any{})
			require.Equal(t, http.StatusOK, rec.Code)

			var out struct {
				Clusters []any `json:"Clusters"`
			}

			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			assert.GreaterOrEqual(t, len(out.Clusters), tt.wantMinCount)
		})
	}
}

func TestEMR_TerminateJobFlows(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(*emr.Handler) []string
		name     string
		wantCode int
	}{
		{
			name: "terminates existing cluster",
			setup: func(h *emr.Handler) []string {
				rec := doEMRRequest(t, h, "RunJobFlow", map[string]any{"Name": "to-terminate"})
				require.Equal(t, http.StatusOK, rec.Code)

				var out struct {
					JobFlowID string `json:"JobFlowId"`
				}
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

				return []string{out.JobFlowID}
			},
			wantCode: http.StatusOK,
		},
		{
			name: "returns error for non-existent cluster",
			setup: func(_ *emr.Handler) []string {
				return []string{"j-NOTEXIST"}
			},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			jobFlowIDs := tt.setup(h)

			rec := doEMRRequest(t, h, "TerminateJobFlows", map[string]any{"JobFlowIds": jobFlowIDs})
			require.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

func TestEMR_TerminateJobFlows_Idempotent(t *testing.T) {
	t.Parallel()

	b := emr.NewInMemoryBackend(testAccountID, testRegion)
	cluster, err := b.RunJobFlow(
		context.Background(),
		emr.RunJobFlowParams{Name: "idem-cluster", ReleaseLabel: "emr-6.0.0"},
	)
	require.NoError(t, err)

	// First termination succeeds.
	require.NoError(t, b.TerminateJobFlows(context.Background(), []string{cluster.ID}))

	// Second termination on an already-terminal cluster must be a no-op, not an error.
	require.NoError(t, b.TerminateJobFlows(context.Background(), []string{cluster.ID}))
}

func TestEMR_TerminateJobFlows_SetsEndDateTime(t *testing.T) {
	t.Parallel()

	b := emr.NewInMemoryBackend(testAccountID, testRegion)
	cluster, err := b.RunJobFlow(
		context.Background(),
		emr.RunJobFlowParams{Name: "timeline-cluster", ReleaseLabel: "emr-6.0.0"},
	)
	require.NoError(t, err)

	before := awstime.Epoch(time.Now())
	require.NoError(t, b.TerminateJobFlows(context.Background(), []string{cluster.ID}))

	c, err := b.DescribeCluster(context.Background(), cluster.ID)
	require.NoError(t, err)

	endRaw, ok := c.Status.Timeline["EndDateTime"]
	require.True(t, ok, "EndDateTime must be set in the timeline after termination")

	// EMR's awsjson1.1 wire format is epoch seconds (float64), not
	// milliseconds -- see smithytime.ParseEpochSeconds in the real SDK
	// deserializer.
	endSeconds, ok := endRaw.(float64)
	require.True(t, ok, "EndDateTime must be a float64 epoch-seconds value")
	assert.GreaterOrEqual(t, endSeconds, before, "EndDateTime should be >= time before termination")
}

func TestRunJobFlow_InvalidReleaseLabel(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doEMRRequest(t, h, "RunJobFlow", map[string]any{
		"Name":         "bad-label-cluster",
		"ReleaseLabel": "emr-0.0.0-invalid",
	})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRunJobFlow_SecurityConfigurationValidation(t *testing.T) {
	t.Parallel()

	t.Run("unknown security configuration returns error", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		rec := doEMRRequest(t, h, "RunJobFlow", map[string]any{
			"Name":                  "sc-cluster",
			"SecurityConfiguration": "does-not-exist",
		})

		require.Equal(t, http.StatusBadRequest, rec.Code)

		var errOut map[string]string
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errOut))
		assert.Equal(t, "InvalidRequestException", errOut["__type"])
	})

	t.Run("no security configuration still succeeds", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		rec := doEMRRequest(t, h, "RunJobFlow", map[string]any{"Name": "no-sc-cluster"})
		require.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("existing security configuration succeeds", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		scRec := doEMRRequest(t, h, "CreateSecurityConfiguration", map[string]any{
			"Name":                  "my-sc",
			"SecurityConfiguration": "{}",
		})
		require.Equal(t, http.StatusOK, scRec.Code)

		rec := doEMRRequest(t, h, "RunJobFlow", map[string]any{
			"Name":                  "sc-cluster-ok",
			"SecurityConfiguration": "my-sc",
		})
		require.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestRunJobFlow_DefaultReleaseLabel(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doEMRRequest(t, h, "RunJobFlow", map[string]any{"Name": "default-label-cluster"})
	require.Equal(t, http.StatusOK, rec.Code)

	var out struct {
		JobFlowID string `json:"JobFlowId"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	descRec := doEMRRequest(t, h, "DescribeCluster", map[string]any{"ClusterId": out.JobFlowID})
	require.Equal(t, http.StatusOK, descRec.Code)

	var desc struct {
		Cluster struct {
			ReleaseLabel string `json:"ReleaseLabel"`
		} `json:"Cluster"`
	}
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &desc))
	assert.Equal(t, emr.DefaultReleaseLabel, desc.Cluster.ReleaseLabel)
}

func TestRunJobFlow_ApplicationsDefaulted(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doEMRRequest(t, h, "RunJobFlow", map[string]any{
		"Name":         "apps-cluster",
		"ReleaseLabel": "emr-7.3.0",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var out struct {
		JobFlowID string `json:"JobFlowId"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	descRec := doEMRRequest(t, h, "DescribeCluster", map[string]any{"ClusterId": out.JobFlowID})
	require.Equal(t, http.StatusOK, descRec.Code)

	var desc struct {
		Cluster struct {
			Applications []struct {
				Name string `json:"Name"`
			} `json:"Applications"`
		} `json:"Cluster"`
	}
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &desc))
	assert.NotEmpty(t, desc.Cluster.Applications)
}

func TestListClusters_StateFilter(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	createRec := doEMRRequest(t, h, "RunJobFlow", map[string]any{"Name": "waiting-cluster"})
	require.Equal(t, http.StatusOK, createRec.Code)

	var create struct {
		JobFlowID string `json:"JobFlowId"`
	}
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &create))

	doEMRRequest(t, h, "TerminateJobFlows", map[string]any{
		"JobFlowIds": []string{create.JobFlowID},
	})

	listActive := doEMRRequest(t, h, "ListClusters", map[string]any{})
	require.Equal(t, http.StatusOK, listActive.Code)

	var activeOut struct {
		Clusters []struct {
			ID string `json:"Id"`
		} `json:"Clusters"`
	}
	require.NoError(t, json.Unmarshal(listActive.Body.Bytes(), &activeOut))
	assert.Empty(t, activeOut.Clusters)

	listTerminated := doEMRRequest(t, h, "ListClusters", map[string]any{
		"ClusterStates": []string{"TERMINATED"},
	})
	require.Equal(t, http.StatusOK, listTerminated.Code)

	var termOut struct {
		Clusters []struct {
			ID string `json:"Id"`
		} `json:"Clusters"`
	}
	require.NoError(t, json.Unmarshal(listTerminated.Body.Bytes(), &termOut))
	assert.Len(t, termOut.Clusters, 1)
}

func TestListClusters_DateFilter(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doEMRRequest(t, h, "RunJobFlow", map[string]any{"Name": "date-cluster"})

	// EMR's awsjson1.1 wire format for CreatedAfter/CreatedBefore is epoch
	// seconds (float64), not milliseconds -- see smithytime.FormatEpochSeconds
	// in the real SDK serializer.
	past := awstime.Epoch(time.Now().Add(-time.Hour))
	future := awstime.Epoch(time.Now().Add(time.Hour))

	listInRange := doEMRRequest(t, h, "ListClusters", map[string]any{
		"CreatedAfter":  past,
		"CreatedBefore": future,
	})
	require.Equal(t, http.StatusOK, listInRange.Code)

	var inRange struct {
		Clusters []struct {
			ID string `json:"Id"`
		} `json:"Clusters"`
	}
	require.NoError(t, json.Unmarshal(listInRange.Body.Bytes(), &inRange))
	assert.Len(t, inRange.Clusters, 1)

	listOld := doEMRRequest(t, h, "ListClusters", map[string]any{
		"CreatedBefore": past,
	})
	require.Equal(t, http.StatusOK, listOld.Code)

	var old struct {
		Clusters []struct {
			ID string `json:"Id"`
		} `json:"Clusters"`
	}
	require.NoError(t, json.Unmarshal(listOld.Body.Bytes(), &old))
	assert.Empty(t, old.Clusters)
}

func TestConfigurations_RunJobFlow(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doEMRRequest(t, h, "RunJobFlow", map[string]any{
		"Name": "config-cluster",
		"Configurations": []map[string]any{
			{
				"Classification": "spark-defaults",
				"Properties":     map[string]string{"spark.executor.memory": "4g"},
				"Configurations": []map[string]any{
					{"Classification": "nested", "Properties": map[string]string{"k": "v"}},
				},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var out struct {
		JobFlowID string `json:"JobFlowId"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	descRec := doEMRRequest(t, h, "DescribeCluster", map[string]any{"ClusterId": out.JobFlowID})
	require.Equal(t, http.StatusOK, descRec.Code)

	var desc struct {
		Cluster struct {
			Configurations []struct {
				Classification string            `json:"Classification"`
				Properties     map[string]string `json:"Properties"`
				Configurations []struct {
					Classification string `json:"Classification"`
				} `json:"Configurations"`
			} `json:"Configurations"`
		} `json:"Cluster"`
	}
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &desc))
	require.Len(t, desc.Cluster.Configurations, 1)
	assert.Equal(t, "spark-defaults", desc.Cluster.Configurations[0].Classification)
	assert.Equal(t, "4g", desc.Cluster.Configurations[0].Properties["spark.executor.memory"])
	require.Len(t, desc.Cluster.Configurations[0].Configurations, 1)
	assert.Equal(t, "nested", desc.Cluster.Configurations[0].Configurations[0].Classification)
}

func TestClusterFields_EbsAndOS(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doEMRRequest(t, h, "RunJobFlow", map[string]any{
		"Name":              "fields-cluster",
		"EbsRootVolumeSize": 64,
		"OSReleaseLabel":    "2.0.20230718",
		"CustomAmiId":       "ami-0123456789abcdef0",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var out struct {
		JobFlowID string `json:"JobFlowId"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	descRec := doEMRRequest(t, h, "DescribeCluster", map[string]any{"ClusterId": out.JobFlowID})
	require.Equal(t, http.StatusOK, descRec.Code)

	var desc struct {
		Cluster struct {
			OSReleaseLabel    string `json:"OSReleaseLabel"`
			CustomAmiID       string `json:"CustomAmiId"`
			EbsRootVolumeSize int    `json:"EbsRootVolumeSize"`
		} `json:"Cluster"`
	}
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &desc))
	assert.Equal(t, 64, desc.Cluster.EbsRootVolumeSize)
	assert.Equal(t, "2.0.20230718", desc.Cluster.OSReleaseLabel)
	assert.Equal(t, "ami-0123456789abcdef0", desc.Cluster.CustomAmiID)
}

// TestSortedListClusters verifies ListClusters returns clusters sorted by ID.
func TestSortedListClusters(t *testing.T) {
	t.Parallel()

	b := emr.NewInMemoryBackend(testAccountID, testRegion)
	_, err := b.RunJobFlow(context.Background(), emr.RunJobFlowParams{Name: "clusterA", ReleaseLabel: "emr-6.0.0"})
	require.NoError(t, err)
	_, err = b.RunJobFlow(context.Background(), emr.RunJobFlowParams{Name: "clusterB", ReleaseLabel: "emr-6.0.0"})
	require.NoError(t, err)
	_, err = b.RunJobFlow(context.Background(), emr.RunJobFlowParams{Name: "clusterC", ReleaseLabel: "emr-6.0.0"})
	require.NoError(t, err)

	clusters, _ := b.ListClusters(context.Background(), emr.ListClustersParams{})
	require.Len(t, clusters, 3)

	// ListClusters returns most recently created first (creation-time descending).
	for i := 1; i < len(clusters); i++ {
		assert.GreaterOrEqual(t, clusters[i-1].ID, clusters[i].ID,
			"clusters must be sorted by creation time descending")
	}
}

// TestCreationDateTime verifies RunJobFlow sets a non-zero CreationDateTime.
func TestCreationDateTime(t *testing.T) {
	t.Parallel()

	b := emr.NewInMemoryBackend(testAccountID, testRegion)
	before := awstime.Epoch(time.Now())
	cluster, err := b.RunJobFlow(
		context.Background(),
		emr.RunJobFlowParams{Name: "ts-cluster", ReleaseLabel: "emr-6.0.0"},
	)
	require.NoError(t, err)
	after := awstime.Epoch(time.Now())

	rawCreation, ok := cluster.Status.Timeline["CreationDateTime"]
	require.True(t, ok, "CreationDateTime must be set in the timeline")

	// EMR's awsjson1.1 wire format is epoch seconds (float64), not
	// milliseconds -- see smithytime.ParseEpochSeconds in the real SDK
	// deserializer.
	creationSeconds, ok := rawCreation.(float64)
	require.True(t, ok, "CreationDateTime must be a float64 epoch-seconds value")
	assert.GreaterOrEqual(t, creationSeconds, before, "CreationDateTime must be >= before")
	assert.LessOrEqual(t, creationSeconds, after, "CreationDateTime must be <= after")
}

// TestTerminateJobFlows_StateChangeReason verifies StateChangeReason is set on termination.
func TestTerminateJobFlows_StateChangeReason(t *testing.T) {
	t.Parallel()

	b := emr.NewInMemoryBackend(testAccountID, testRegion)
	cluster, err := b.RunJobFlow(
		context.Background(),
		emr.RunJobFlowParams{Name: "scr-cluster", ReleaseLabel: "emr-6.0.0"},
	)
	require.NoError(t, err)

	require.NoError(t, b.TerminateJobFlows(context.Background(), []string{cluster.ID}))

	c, err := b.DescribeCluster(context.Background(), cluster.ID)
	require.NoError(t, err)
	assert.Equal(t, emr.StateTerminated, c.Status.State)
	assert.NotEmpty(t, c.Status.StateChangeReason["Code"])
}

func TestDescribeCluster_TagsEmptyNotAbsent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body map[string]any
		name string
	}{
		{
			name: "no tags at creation returns Tags:[]",
			body: map[string]any{"Name": "notag-cluster"},
		},
		{
			name: "empty tags slice at creation returns Tags:[]",
			body: map[string]any{"Name": "emptytag-cluster", "Tags": []any{}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			createRec := doEMRRequest(t, h, "RunJobFlow", tt.body)
			require.Equal(t, http.StatusOK, createRec.Code)

			var create struct {
				JobFlowID string `json:"JobFlowId"`
			}
			require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &create))

			rec := doEMRRequest(t, h, "DescribeCluster", map[string]any{"ClusterId": create.JobFlowID})
			require.Equal(t, http.StatusOK, rec.Code)

			var raw map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))

			cluster, ok := raw["Cluster"].(map[string]any)
			require.True(t, ok, "Cluster must be an object")

			tags, hasKey := cluster["Tags"]
			assert.True(t, hasKey, "DescribeCluster must include 'Tags' key even when empty")
			assert.IsType(t, []any{}, tags, "'Tags' must be an array, not null or absent")
			assert.Empty(t, tags, "'Tags' must be [] not populated")
		})
	}
}

func TestDescribeCluster_TagsRoundTrip(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	createRec := doEMRRequest(t, h, "RunJobFlow", map[string]any{
		"Name": "tagged-cluster",
		"Tags": []map[string]any{{"Key": "env", "Value": "test"}},
	})
	require.Equal(t, http.StatusOK, createRec.Code)

	var create struct {
		JobFlowID string `json:"JobFlowId"`
	}
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &create))

	rec := doEMRRequest(t, h, "DescribeCluster", map[string]any{"ClusterId": create.JobFlowID})
	require.Equal(t, http.StatusOK, rec.Code)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))

	cluster := raw["Cluster"].(map[string]any)
	tags := cluster["Tags"].([]any)
	require.Len(t, tags, 1)
	tag := tags[0].(map[string]any)
	assert.Equal(t, "env", tag["Key"])
	assert.Equal(t, "test", tag["Value"])
}

func TestDescribeCluster_TagsEmptyAfterRemove(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	createRec := doEMRRequest(t, h, "RunJobFlow", map[string]any{
		"Name": "rmtag-cluster",
		"Tags": []map[string]any{{"Key": "k", "Value": "v"}},
	})
	require.Equal(t, http.StatusOK, createRec.Code)

	var create struct {
		JobFlowID string `json:"JobFlowId"`
	}
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &create))

	rmRec := doEMRRequest(t, h, "RemoveTags", map[string]any{
		"ResourceId": create.JobFlowID,
		"TagKeys":    []string{"k"},
	})
	require.Equal(t, http.StatusOK, rmRec.Code)

	rec := doEMRRequest(t, h, "DescribeCluster", map[string]any{"ClusterId": create.JobFlowID})
	require.Equal(t, http.StatusOK, rec.Code)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))

	cluster := raw["Cluster"].(map[string]any)
	tags, hasKey := cluster["Tags"]
	assert.True(t, hasKey, "Tags key must be present after RemoveTags empties all tags")
	assert.IsType(t, []any{}, tags, "Tags must be [] not null after all tags removed")
	assert.Empty(t, tags)
}

// TestErrorMapping_NotFound_ClusterNotFoundException verifies ErrNotFound maps to ClusterNotFoundException.
func TestErrorMapping_NotFound_ClusterNotFoundException(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doEMRRequest(t, h, "DescribeCluster", map[string]any{"ClusterId": "j-NONEXISTENT"})
	require.Equal(t, http.StatusBadRequest, rec.Code)

	var errOut map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errOut))
	assert.Equal(t, "InvalidRequestException", errOut["__type"])
}

// TestValidateReleaseLabel verifies valid and invalid label formats.
func TestValidateReleaseLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		label  string
		wantOK bool
	}{
		{"known registry label", "emr-6.0.0", true},
		{"known registry label 7x", "emr-7.3.0", true},
		{"arbitrary valid label", "emr-5.20.1", true},
		{"arbitrary high version", "emr-8.0.0", true},
		{"arbitrary minor only", "emr-6.99", true},
		{"empty label defaults to latest", "", true},
		{"no prefix", "6.0.0", false},
		{"bad format", "emr-abc", false},
		{"trailing dot", "emr-6.", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := emr.NewInMemoryBackend(testAccountID, testRegion)
			_, err := b.RunJobFlow(context.Background(), emr.RunJobFlowParams{
				Name:         "label-test",
				ReleaseLabel: tt.label,
			})

			if tt.wantOK {
				assert.NoError(t, err, "label %q should be accepted", tt.label)
			} else {
				assert.Error(t, err, "label %q should be rejected", tt.label)
			}
		})
	}
}

// newSessionForTest creates a cluster and starts a session on it, returning
// both IDs. Called fresh from within each subtest closure below -- never
// shared across subtests.
func newSessionForTest(t *testing.T, h *emr.Handler, clusterName string) (string, string) {
	t.Helper()

	rec := doEMRRequest(t, h, "RunJobFlow", map[string]any{"Name": clusterName, "SessionEnabled": true})
	require.Equal(t, http.StatusOK, rec.Code)

	var cluster struct {
		JobFlowID string `json:"JobFlowId"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &cluster))

	startRec := doEMRRequest(t, h, "StartSession", map[string]any{"ClusterId": cluster.JobFlowID})
	require.Equal(t, http.StatusOK, startRec.Code, startRec.Body.String())

	var session struct {
		ID string `json:"Id"`
	}
	require.NoError(t, json.Unmarshal(startRec.Body.Bytes(), &session))

	return cluster.JobFlowID, session.ID
}

// TestEMR_StartSession verifies StartSession validates the target cluster
// exists and is in a state that can host a session (real StartSession's
// doc: "The cluster must be in the RUNNING or WAITING state") -- a
// TERMINATED cluster must not accept a new session.
func TestEMR_StartSession(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(t *testing.T, h *emr.Handler) string
		name     string
		wantCode int
	}{
		{
			name: "starts session on a WAITING cluster",
			setup: func(t *testing.T, h *emr.Handler) string {
				t.Helper()

				rec := doEMRRequest(t, h, "RunJobFlow",
					map[string]any{"Name": "session-cluster", "SessionEnabled": true})
				require.Equal(t, http.StatusOK, rec.Code)

				var out struct {
					JobFlowID string `json:"JobFlowId"`
				}
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

				return out.JobFlowID
			},
			wantCode: http.StatusOK,
		},
		{
			name: "cluster not found",
			setup: func(_ *testing.T, _ *emr.Handler) string {
				return "j-NOTEXIST"
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "terminated cluster rejects new session",
			setup: func(t *testing.T, h *emr.Handler) string {
				t.Helper()

				rec := doEMRRequest(t, h, "RunJobFlow", map[string]any{"Name": "terminated-session-cluster"})
				require.Equal(t, http.StatusOK, rec.Code)

				var out struct {
					JobFlowID string `json:"JobFlowId"`
				}
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

				termRec := doEMRRequest(t, h, "TerminateJobFlows", map[string]any{
					"JobFlowIds": []string{out.JobFlowID},
				})
				require.Equal(t, http.StatusOK, termRec.Code)

				return out.JobFlowID
			},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			clusterID := tt.setup(t, h)

			rec := doEMRRequest(t, h, "StartSession", map[string]any{"ClusterId": clusterID})
			require.Equal(t, tt.wantCode, rec.Code)

			if tt.wantCode != http.StatusOK {
				return
			}

			var out struct {
				ID        string `json:"Id"`
				ClusterID string `json:"ClusterId"`
				ARN       string `json:"Arn"`
				State     string `json:"State"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			assert.NotEmpty(t, out.ID)
			assert.Equal(t, clusterID, out.ClusterID)
			assert.Equal(t, emr.SessionStateSubmitted, out.State)
			assert.Contains(t, out.ARN, "elasticmapreduce")
			assert.Contains(t, out.ARN, clusterID)
		})
	}
}

// TestEMR_GetSession verifies GetSession is scoped by both cluster ID and
// session ID, matching real GetSessionInput's two required members.
func TestEMR_GetSession(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		useBadCluster bool
		useBadSession bool
		wantCode      int
	}{
		{name: "found", wantCode: http.StatusOK},
		{name: "cluster not found", useBadCluster: true, wantCode: http.StatusBadRequest},
		{name: "session not found", useBadSession: true, wantCode: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			clusterID, sessionID := newSessionForTest(t, h, "get-session-cluster")

			if tt.useBadCluster {
				clusterID = "j-NOTEXIST"
			}

			if tt.useBadSession {
				sessionID = "sess-NOTEXIST"
			}

			rec := doEMRRequest(t, h, "GetSession", map[string]any{
				"ClusterId": clusterID, "SessionId": sessionID,
			})
			require.Equal(t, tt.wantCode, rec.Code)

			if tt.wantCode != http.StatusOK {
				return
			}

			var out struct {
				Session struct {
					ID        string `json:"Id"`
					ClusterID string `json:"ClusterId"`
					State     string `json:"State"`
				} `json:"Session"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			assert.Equal(t, sessionID, out.Session.ID)
			assert.Equal(t, clusterID, out.Session.ClusterID)
			assert.Equal(t, emr.SessionStateSubmitted, out.Session.State)
		})
	}
}

// TestEMR_ListSessions verifies ListSessions returns sessions scoped to a
// cluster and honors the SessionStates filter.
func TestEMR_ListSessions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		stateFilter []string
		numSessions int
		wantCount   int
	}{
		{name: "empty list", numSessions: 0, wantCount: 0},
		{name: "lists started sessions", numSessions: 2, wantCount: 2},
		{
			name:        "filters out sessions not in requested state",
			numSessions: 2,
			stateFilter: []string{emr.SessionStateTerminated},
			wantCount:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			createRec := doEMRRequest(t, h, "RunJobFlow",
				map[string]any{"Name": "list-sessions-cluster", "SessionEnabled": true})
			require.Equal(t, http.StatusOK, createRec.Code)

			var cluster struct {
				JobFlowID string `json:"JobFlowId"`
			}
			require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &cluster))

			for range tt.numSessions {
				startRec := doEMRRequest(t, h, "StartSession", map[string]any{"ClusterId": cluster.JobFlowID})
				require.Equal(t, http.StatusOK, startRec.Code)
			}

			body := map[string]any{"ClusterId": cluster.JobFlowID}
			if tt.stateFilter != nil {
				body["SessionStates"] = tt.stateFilter
			}

			rec := doEMRRequest(t, h, "ListSessions", body)
			require.Equal(t, http.StatusOK, rec.Code)

			var out struct {
				Sessions []struct {
					ID string `json:"Id"`
				} `json:"Sessions"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			assert.Len(t, out.Sessions, tt.wantCount)
		})
	}
}

// TestEMR_TerminateSession verifies TerminateSession transitions a session
// to TERMINATED and is idempotent on an already-terminated session.
func TestEMR_TerminateSession(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		useBadSession  bool
		terminateFirst bool
		wantCode       int
	}{
		{name: "terminates active session", wantCode: http.StatusOK},
		{name: "session not found", useBadSession: true, wantCode: http.StatusBadRequest},
		{name: "idempotent on already-terminated session", terminateFirst: true, wantCode: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			clusterID, sessionID := newSessionForTest(t, h, "terminate-session-cluster")

			if tt.terminateFirst {
				firstRec := doEMRRequest(t, h, "TerminateSession", map[string]any{
					"ClusterId": clusterID, "SessionId": sessionID,
				})
				require.Equal(t, http.StatusOK, firstRec.Code)
			}

			targetSessionID := sessionID
			if tt.useBadSession {
				targetSessionID = "sess-NOTEXIST"
			}

			rec := doEMRRequest(t, h, "TerminateSession", map[string]any{
				"ClusterId": clusterID, "SessionId": targetSessionID,
			})
			require.Equal(t, tt.wantCode, rec.Code)

			if tt.wantCode != http.StatusOK {
				return
			}

			var out struct {
				ClusterID string `json:"ClusterId"`
				SessionID string `json:"SessionId"`
				State     string `json:"State"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			assert.Equal(t, clusterID, out.ClusterID)
			assert.Equal(t, sessionID, out.SessionID)
			assert.Equal(t, emr.SessionStateTerminated, out.State)
		})
	}
}

// TestEMR_GetSessionEndpoint verifies GetSessionEndpoint returns a
// synthesized Spark Connect endpoint URL, auth token, and VPC-peering
// credentials, gated on both cluster and session existing.
func TestEMR_GetSessionEndpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		useBadCluster bool
		useBadSession bool
		wantCode      int
	}{
		{name: "found", wantCode: http.StatusOK},
		{name: "cluster not found", useBadCluster: true, wantCode: http.StatusBadRequest},
		{name: "session not found", useBadSession: true, wantCode: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			clusterID, sessionID := newSessionForTest(t, h, "endpoint-session-cluster")

			if tt.useBadCluster {
				clusterID = "j-NOTEXIST"
			}

			if tt.useBadSession {
				sessionID = "sess-NOTEXIST"
			}

			rec := doEMRRequest(t, h, "GetSessionEndpoint", map[string]any{
				"ClusterId": clusterID, "SessionId": sessionID,
			})
			require.Equal(t, tt.wantCode, rec.Code)

			if tt.wantCode != http.StatusOK {
				return
			}

			var out struct {
				Credentials map[string]any `json:"Credentials"`
				Endpoint    string         `json:"Endpoint"`
				AuthToken   string         `json:"AuthToken"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			assert.NotEmpty(t, out.Endpoint)
			assert.NotEmpty(t, out.AuthToken)
			assert.Contains(t, out.Credentials, "UsernamePassword")
		})
	}
}

// TestEMR_TerminateJobFlows_CascadesToSessions verifies terminating a
// cluster also terminates every session running on it -- a Spark Connect
// session cannot outlive its underlying cluster, and a session that
// survived cluster termination as an orphan (never reachable again once the
// janitor sweeps the cluster row) would be exactly the ghost-row bug class
// this campaign flags.
func TestEMR_TerminateJobFlows_CascadesToSessions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		preTerminateSession bool
	}{
		{name: "active session is terminated along with its cluster", preTerminateSession: false},
		{name: "already-terminated session stays terminated", preTerminateSession: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			clusterID, sessionID := newSessionForTest(t, h, "cascade-cluster")

			if tt.preTerminateSession {
				preRec := doEMRRequest(t, h, "TerminateSession", map[string]any{
					"ClusterId": clusterID, "SessionId": sessionID,
				})
				require.Equal(t, http.StatusOK, preRec.Code)
			}

			termRec := doEMRRequest(t, h, "TerminateJobFlows", map[string]any{
				"JobFlowIds": []string{clusterID},
			})
			require.Equal(t, http.StatusOK, termRec.Code)

			getRec := doEMRRequest(t, h, "GetSession", map[string]any{
				"ClusterId": clusterID, "SessionId": sessionID,
			})
			require.Equal(t, http.StatusOK, getRec.Code)

			var out struct {
				Session struct {
					State string `json:"State"`
				} `json:"Session"`
			}
			require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &out))
			assert.Equal(t, emr.SessionStateTerminated, out.Session.State,
				"session must be terminated when its cluster is terminated")
		})
	}
}

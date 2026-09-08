package elasticache_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	elasticachesdk "github.com/aws/aws-sdk-go-v2/service/elasticache"
	elasticachetypes "github.com/aws/aws-sdk-go-v2/service/elasticache/types"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	smithy "github.com/aws/smithy-go"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/services/elasticache"
)

func TestCreateCacheCluster(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(t *testing.T, client *elasticachesdk.Client)
		name         string
		clusterID    string
		engine       string
		nodeType     string
		wantStatus   string
		wantErr      bool
		wantEndpoint bool
	}{
		{
			name:         "success",
			clusterID:    "my-cluster",
			engine:       "redis",
			nodeType:     "cache.t3.micro",
			wantStatus:   "available",
			wantEndpoint: true,
		},
		{
			name:      "already_exists",
			clusterID: "dup",
			engine:    "redis",
			setup: func(t *testing.T, client *elasticachesdk.Client) {
				t.Helper()
				_, err := client.CreateCacheCluster(t.Context(), &elasticachesdk.CreateCacheClusterInput{
					CacheClusterId: aws.String("dup"),
					Engine:         aws.String("redis"),
				})
				require.NoError(t, err)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := newTestStack(t)

			if tt.setup != nil {
				tt.setup(t, client)
			}

			out, err := client.CreateCacheCluster(t.Context(), &elasticachesdk.CreateCacheClusterInput{
				CacheClusterId: aws.String(tt.clusterID),
				Engine:         aws.String(tt.engine),
				CacheNodeType:  aws.String(tt.nodeType),
			})

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, out.CacheCluster)
			assert.Equal(t, tt.clusterID, aws.ToString(out.CacheCluster.CacheClusterId))
			assert.Equal(t, tt.wantStatus, aws.ToString(out.CacheCluster.CacheClusterStatus))
			assert.Equal(t, tt.engine, aws.ToString(out.CacheCluster.Engine))

			if tt.wantEndpoint {
				require.NotEmpty(t, out.CacheCluster.CacheNodes)
				ep := out.CacheCluster.CacheNodes[0].Endpoint
				require.NotNil(t, ep)
				assert.Contains(t, aws.ToString(ep.Address), ".cache.amazonaws.com")
				assert.Positive(t, aws.ToInt32(ep.Port))
			}
		})
	}
}

func TestDescribeCacheClusters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup     func(t *testing.T, client *elasticachesdk.Client)
		name      string
		clusterID string
		wantCount int
		wantErr   bool
	}{
		{
			name:      "describe_specific",
			clusterID: "my-cluster",
			setup: func(t *testing.T, client *elasticachesdk.Client) {
				t.Helper()
				_, err := client.CreateCacheCluster(t.Context(), &elasticachesdk.CreateCacheClusterInput{
					CacheClusterId: aws.String("my-cluster"),
					Engine:         aws.String("redis"),
				})
				require.NoError(t, err)
			},
			wantCount: 1,
		},
		{
			name: "describe_all",
			setup: func(t *testing.T, client *elasticachesdk.Client) {
				t.Helper()
				for _, id := range []string{"cluster-a", "cluster-b"} {
					_, err := client.CreateCacheCluster(t.Context(), &elasticachesdk.CreateCacheClusterInput{
						CacheClusterId: aws.String(id),
						Engine:         aws.String("redis"),
					})
					require.NoError(t, err)
				}
			},
			wantCount: 2,
		},
		{
			name:      "not_found",
			clusterID: "does-not-exist",
			wantErr:   true,
		},
		{
			name:      "not_found_after_delete",
			clusterID: "my-cluster",
			setup: func(t *testing.T, client *elasticachesdk.Client) {
				t.Helper()
				_, err := client.CreateCacheCluster(t.Context(), &elasticachesdk.CreateCacheClusterInput{
					CacheClusterId: aws.String("my-cluster"),
					Engine:         aws.String("redis"),
				})
				require.NoError(t, err)
				_, err = client.DeleteCacheCluster(t.Context(), &elasticachesdk.DeleteCacheClusterInput{
					CacheClusterId: aws.String("my-cluster"),
				})
				require.NoError(t, err)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := newTestStack(t)

			if tt.setup != nil {
				tt.setup(t, client)
			}

			var clusterID *string
			if tt.clusterID != "" {
				clusterID = aws.String(tt.clusterID)
			}

			out, err := client.DescribeCacheClusters(t.Context(), &elasticachesdk.DescribeCacheClustersInput{
				CacheClusterId: clusterID,
			})

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Len(t, out.CacheClusters, tt.wantCount)

			if tt.clusterID != "" && tt.wantCount == 1 {
				assert.Equal(t, tt.clusterID, aws.ToString(out.CacheClusters[0].CacheClusterId))
			}
		})
	}
}

func TestDeleteCacheCluster(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(t *testing.T, client *elasticachesdk.Client)
		name       string
		clusterID  string
		wantStatus string
		wantErr    bool
	}{
		{
			name:      "success",
			clusterID: "my-cluster",
			setup: func(t *testing.T, client *elasticachesdk.Client) {
				t.Helper()
				_, err := client.CreateCacheCluster(t.Context(), &elasticachesdk.CreateCacheClusterInput{
					CacheClusterId: aws.String("my-cluster"),
					Engine:         aws.String("redis"),
				})
				require.NoError(t, err)
			},
			wantStatus: "deleting",
		},
		{
			name:      "not_found",
			clusterID: "does-not-exist",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := newTestStack(t)

			if tt.setup != nil {
				tt.setup(t, client)
			}

			out, err := client.DeleteCacheCluster(t.Context(), &elasticachesdk.DeleteCacheClusterInput{
				CacheClusterId: aws.String(tt.clusterID),
			})

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, out.CacheCluster)
			assert.Equal(t, tt.wantStatus, aws.ToString(out.CacheCluster.CacheClusterStatus))
		})
	}
}

// TestDeleteCacheCluster_FinalSnapshotIdentifier pins
// SnapshotFeatureNotSupportedFault (api-2.json:
// "Creating a snapshot of a cluster that is running Memcached rather than
// Valkey or Redis OSS" is unsupported) for DeleteCacheCluster's
// FinalSnapshotIdentifier. All three cases matter: a guard that rejects
// every engine regardless of FinalSnapshotIdentifier, or that fires on
// Memcached even with no FinalSnapshotIdentifier, would each pass a
// single-case test but must fail here.
func TestDeleteCacheCluster_FinalSnapshotIdentifier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		finalSnapshotID string
		name            string
		engine          string
		wantRejected    bool
	}{
		{
			name:            "memcached_with_final_snapshot_rejected",
			engine:          "memcached",
			finalSnapshotID: "final-snap",
			wantRejected:    true,
		},
		{
			name:            "memcached_without_final_snapshot_allowed",
			engine:          "memcached",
			finalSnapshotID: "",
			wantRejected:    false,
		},
		{
			name:            "redis_with_final_snapshot_allowed",
			engine:          "redis",
			finalSnapshotID: "final-snap",
			wantRejected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := newTestStack(t)
			clusterID := "fsi-" + tt.name

			_, err := client.CreateCacheCluster(t.Context(), &elasticachesdk.CreateCacheClusterInput{
				CacheClusterId: aws.String(clusterID),
				Engine:         aws.String(tt.engine),
			})
			require.NoError(t, err)

			var finalSnap *string
			if tt.finalSnapshotID != "" {
				finalSnap = aws.String(tt.finalSnapshotID)
			}

			_, err = client.DeleteCacheCluster(t.Context(), &elasticachesdk.DeleteCacheClusterInput{
				CacheClusterId:          aws.String(clusterID),
				FinalSnapshotIdentifier: finalSnap,
			})

			if tt.wantRejected {
				requireFault[elasticachetypes.SnapshotFeatureNotSupportedFault](t, err)
				requireHTTPStatus(t, err, http.StatusBadRequest)

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestBackend(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		engineMode        string
		clusterEngine     string
		wantFirstEndpoint string
		wantFirstStatus   string
		clusterIDs        []string
		clusterPort       int
		wantCount         int
		wantFirstPort     int
	}{
		{
			name:              "stub_engine_mode",
			engineMode:        elasticache.EngineStub,
			clusterIDs:        []string{"stub-cluster"},
			clusterEngine:     "redis",
			clusterPort:       0,
			wantFirstEndpoint: ".cache.amazonaws.com",
			wantFirstPort:     6379,
			wantFirstStatus:   "available",
		},
		{
			name:          "default_engine",
			engineMode:    "",
			clusterIDs:    []string{"test"},
			clusterEngine: "redis",
			clusterPort:   6379,
			wantCount:     1,
		},
		{
			name:          "list_all",
			engineMode:    elasticache.EngineStub,
			clusterIDs:    []string{"c1", "c2"},
			clusterEngine: "redis",
			wantCount:     2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := elasticache.NewInMemoryBackend(tt.engineMode, "000000000000", "us-east-1", nil)

			var firstCluster *elasticache.Cluster
			for _, id := range tt.clusterIDs {
				cluster, err := backend.CreateCluster(
					context.Background(),
					id,
					tt.clusterEngine,
					"cache.t3.micro",
					tt.clusterPort,
				)
				require.NoError(t, err)
				if firstCluster == nil {
					firstCluster = cluster
				}
			}

			if tt.wantCount > 0 {
				assert.Len(t, backend.ListAll(), tt.wantCount)
			}

			if tt.wantFirstEndpoint != "" {
				require.NotNil(t, firstCluster)
				assert.Contains(t, firstCluster.Endpoint, tt.wantFirstEndpoint)
				assert.Equal(t, tt.wantFirstPort, firstCluster.Port)
				assert.Equal(t, tt.wantFirstStatus, firstCluster.Status)
			}
		})
	}
}

func TestModifyCacheCluster(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(t *testing.T, client *elasticachesdk.Client)
		name     string
		id       string
		nodeType string
		wantErr  bool
	}{
		{
			name:     "success",
			id:       "my-cluster",
			nodeType: "cache.r6g.large",
			setup: func(t *testing.T, client *elasticachesdk.Client) {
				t.Helper()
				_, err := client.CreateCacheCluster(t.Context(), &elasticachesdk.CreateCacheClusterInput{
					CacheClusterId: aws.String("my-cluster"),
					Engine:         aws.String("redis"),
				})
				require.NoError(t, err)
			},
		},
		{
			name:    "not_found",
			id:      "does-not-exist",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := newTestStack(t)

			if tt.setup != nil {
				tt.setup(t, client)
			}

			out, err := client.ModifyCacheCluster(t.Context(), &elasticachesdk.ModifyCacheClusterInput{
				CacheClusterId: aws.String(tt.id),
				CacheNodeType:  aws.String(tt.nodeType),
			})

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, out.CacheCluster)
			assert.Equal(t, tt.nodeType, aws.ToString(out.CacheCluster.CacheNodeType))
		})
	}
}

func TestCreateClusterWithParameterGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		clusterID      string
		paramGroupName string
		wantErr        bool
	}{
		{
			name:           "with_default_param_group",
			clusterID:      "my-cluster",
			paramGroupName: "default.redis7",
		},
		{
			name:           "with_custom_param_group",
			clusterID:      "my-cluster2",
			paramGroupName: "custom-pg",
		},
		{
			name:           "param_group_not_found",
			clusterID:      "my-cluster3",
			paramGroupName: "nonexistent-pg",
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := newTestStack(t)

			if tt.paramGroupName == "custom-pg" {
				_, err := client.CreateCacheParameterGroup(t.Context(), &elasticachesdk.CreateCacheParameterGroupInput{
					CacheParameterGroupName:   aws.String("custom-pg"),
					CacheParameterGroupFamily: aws.String("redis7"),
					Description:               aws.String("custom"),
				})
				require.NoError(t, err)
			}

			out, err := client.CreateCacheCluster(t.Context(), &elasticachesdk.CreateCacheClusterInput{
				CacheClusterId:          aws.String(tt.clusterID),
				Engine:                  aws.String("redis"),
				CacheParameterGroupName: aws.String(tt.paramGroupName),
			})

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, out.CacheCluster)
		})
	}
}

func TestCreateCacheCluster_CustomerAZUsesRegion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		region string
		wantAZ string
	}{
		{name: "us-east-1 default", region: "us-east-1", wantAZ: "us-east-1a"},
		{name: "eu-west-1 cross-region", region: "eu-west-1", wantAZ: "eu-west-1a"},
		{name: "ap-southeast-2 cross-region", region: "ap-southeast-2", wantAZ: "ap-southeast-2a"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := elasticache.NewInMemoryBackend(elasticache.EngineStub, "000000000000", tt.region, nil)
			handler := elasticache.NewHandler(backend)
			handler.Region = tt.region

			e := echo.New()
			registry := service.NewRegistry()
			_ = registry.Register(handler)
			router := service.NewServiceRouter(registry)
			e.Use(router.RouteHandler())

			srv := httptest.NewServer(e)
			t.Cleanup(srv.Close)

			cfg, err := awscfg.LoadDefaultConfig(
				t.Context(),
				awscfg.WithRegion(tt.region),
				awscfg.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
			)
			require.NoError(t, err)

			client := elasticachesdk.NewFromConfig(cfg, func(o *elasticachesdk.Options) {
				o.BaseEndpoint = aws.String(srv.URL)
			})

			out, err := client.CreateCacheCluster(context.Background(), &elasticachesdk.CreateCacheClusterInput{
				CacheClusterId: aws.String("xr-cluster"),
				Engine:         aws.String("redis"),
			})
			require.NoError(t, err)
			require.NotNil(t, out.CacheCluster)
			require.NotEmpty(t, out.CacheCluster.CacheNodes)
			assert.Equal(t, tt.wantAZ,
				strings.ToLower(aws.ToString(out.CacheCluster.CacheNodes[0].CustomerAvailabilityZone)))
		})
	}
}

func TestHandler_CreateCacheCluster_Memcached_Members(t *testing.T) {
	t.Parallel()

	client := newTestStack(t)

	out, err := client.CreateCacheCluster(t.Context(), &elasticachesdk.CreateCacheClusterInput{
		CacheClusterId: aws.String("memcached-cluster"),
		Engine:         aws.String("memcached"),
		CacheNodeType:  aws.String("cache.t3.micro"),
		NumCacheNodes:  aws.Int32(3),
	})

	require.NoError(t, err)
	require.NotNil(t, out.CacheCluster)
	assert.Equal(t, int32(3), aws.ToInt32(out.CacheCluster.NumCacheNodes))
}

// ----------------------------------------
// DescribeCacheParameterGroups — Valkey families
// ----------------------------------------

func TestHandler_DescribeCacheClusters_Pagination(t *testing.T) {
	t.Parallel()

	client := newTestStack(t)

	// Create 25 clusters -- AWS rejects MaxRecords below 20, so the smallest
	// valid page size (20) needs more than 20 records to prove a second page.
	const total = 25

	for i := range total {
		_, err := client.CreateCacheCluster(t.Context(), &elasticachesdk.CreateCacheClusterInput{
			CacheClusterId: aws.String(fmt.Sprintf("paginate-cluster-%d", i)),
			Engine:         aws.String("redis"),
		})
		require.NoError(t, err)
	}

	// Page 1: max 20 (AWS's modeled minimum).
	page1, err := client.DescribeCacheClusters(t.Context(), &elasticachesdk.DescribeCacheClustersInput{
		MaxRecords: aws.Int32(20),
	})
	require.NoError(t, err)
	assert.Len(t, page1.CacheClusters, 20)
	assert.NotEmpty(t, aws.ToString(page1.Marker))

	// Page 2: rest.
	page2, err := client.DescribeCacheClusters(t.Context(), &elasticachesdk.DescribeCacheClustersInput{
		MaxRecords: aws.Int32(20),
		Marker:     page1.Marker,
	})
	require.NoError(t, err)
	assert.Len(t, page2.CacheClusters, total-20)
}

// TestHandler_DescribeCacheClusters_MaxRecordsOutOfRange locks AWS's modeled
// MaxRecords bounds ([20,100] -- InvalidParameterValueException otherwise)
// for every paginated Describe*/List* operation, verified against the
// aws-sdk-go-v2 client's typed error and HTTP status.
func TestHandler_DescribeCacheClusters_MaxRecordsOutOfRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		maxRecords int32
	}{
		{name: "below_min", maxRecords: 19},
		{name: "above_max", maxRecords: 101},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := newTestStack(t)

			_, err := client.DescribeCacheClusters(t.Context(), &elasticachesdk.DescribeCacheClustersInput{
				MaxRecords: aws.Int32(tt.maxRecords),
			})
			require.Error(t, err)
			requireFault[elasticachetypes.InvalidParameterValueException](t, err)
			requireHTTPStatus(t, err, http.StatusBadRequest)
		})
	}
}

// TestHandler_DescribeCacheClusters_MaxRecordsOutOfRange_DoesNotDoubleWrite
// locks the same gopherstack-8haq shape one layer deeper than the four call
// sites named in the issue: parsePaginationChecked itself rejects via
// xmlError and returns its result, so every one of its ~13 direct callers
// (describeCacheClusters included, at the old handler_cache_clusters.go:360)
// plus describeListChecked's own internal call -- and so, transitively,
// describeListChecked's ~7 further callers -- store a value that is nil
// even when validation genuinely failed. The typed-error assertions in
// TestHandler_DescribeCacheClusters_MaxRecordsOutOfRange above still pass
// under the bug (the SDK's XML decoder happily parses the first, correct
// ErrorResponse document off the front of the body and ignores the second
// one appended after it), so this test inspects the raw body instead, the
// same way TestModifyCacheCluster_InvalidSnapshotRetentionLimit_LeavesClusterUnchanged
// catches it for the two SnapshotRetentionLimit call sites.
func TestHandler_DescribeCacheClusters_MaxRecordsOutOfRange_DoesNotDoubleWrite(t *testing.T) {
	t.Parallel()

	srv := newErrorTestServer(t)

	form := url.Values{
		"Action":     {"DescribeCacheClusters"},
		"Version":    {"2015-02-02"},
		"MaxRecords": {"101"},
	}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL, strings.NewReader(form.Encode()))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.NotContains(t, string(body), "DescribeCacheClustersResponse",
		"an out-of-range MaxRecords must stop before writing a second (success) response onto the same body")
}

// ----------------------------------------
// CacheCluster — ARN format
// ----------------------------------------

func TestHandler_CreateCacheCluster_ARNFormat(t *testing.T) {
	t.Parallel()

	client := newTestStack(t)

	out, err := client.CreateCacheCluster(t.Context(), &elasticachesdk.CreateCacheClusterInput{
		CacheClusterId: aws.String("arn-cluster"),
		Engine:         aws.String("redis"),
	})
	require.NoError(t, err)
	arn := aws.ToString(out.CacheCluster.ARN)
	assert.Contains(t, arn, "arn:aws:elasticache:")
	assert.Contains(t, arn, ":cluster:")
	assert.Contains(t, arn, "arn-cluster")
}

func TestHandler_CreateCacheCluster_Memcached_NumNodes(t *testing.T) {
	t.Parallel()

	client := newTestStack(t)

	out, err := client.CreateCacheCluster(t.Context(), &elasticachesdk.CreateCacheClusterInput{
		CacheClusterId: aws.String("memcached-3node"),
		Engine:         aws.String("memcached"),
		CacheNodeType:  aws.String("cache.t3.micro"),
		NumCacheNodes:  aws.Int32(3),
	})
	require.NoError(t, err)
	assert.Equal(t, int32(3), aws.ToInt32(out.CacheCluster.NumCacheNodes))
	assert.Equal(t, int32(3), int32(len(out.CacheCluster.CacheNodes)))
}

// ----------------------------------------
// CacheParameterGroup — reset all parameters
// ----------------------------------------

func TestHandler_ListAllowedNodeTypeModifications_ForCluster(t *testing.T) {
	t.Parallel()

	client := newTestStack(t)

	_, err := client.CreateCacheCluster(t.Context(), &elasticachesdk.CreateCacheClusterInput{
		CacheClusterId: aws.String("mod-type-cluster"),
		Engine:         aws.String("redis"),
		CacheNodeType:  aws.String("cache.t3.micro"),
	})
	require.NoError(t, err)

	out, err := client.ListAllowedNodeTypeModifications(
		t.Context(),
		&elasticachesdk.ListAllowedNodeTypeModificationsInput{
			CacheClusterId: aws.String("mod-type-cluster"),
		},
	)
	require.NoError(t, err)
	assert.NotNil(t, out)
}

// ----------------------------------------
// DescribeEngineDefaultParameters
// ----------------------------------------

func TestRebootCacheCluster(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup     func(t *testing.T, client *elasticachesdk.Client)
		name      string
		clusterID string
		wantErr   bool
	}{
		{
			name:      "success",
			clusterID: "cluster-reboot",
			setup: func(t *testing.T, client *elasticachesdk.Client) {
				t.Helper()
				_, err := client.CreateCacheCluster(t.Context(), &elasticachesdk.CreateCacheClusterInput{
					CacheClusterId: aws.String("cluster-reboot"),
					Engine:         aws.String("redis"),
					CacheNodeType:  aws.String("cache.t3.micro"),
					NumCacheNodes:  aws.Int32(1),
				})
				require.NoError(t, err)
			},
		},
		{
			name:      "not_found",
			clusterID: "no-such-cluster",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := newTestStack(t)

			if tt.setup != nil {
				tt.setup(t, client)
			}

			out, err := client.RebootCacheCluster(t.Context(), &elasticachesdk.RebootCacheClusterInput{
				CacheClusterId:       aws.String(tt.clusterID),
				CacheNodeIdsToReboot: []string{"0001"},
			})

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.clusterID, aws.ToString(out.CacheCluster.CacheClusterId))
		})
	}
}

// ----------------------------------------
// DeleteCacheSecurityGroup
// ----------------------------------------

func TestListAllowedNodeTypeModifications(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		rgID string
	}{
		{
			name: "by_rg_id",
			rgID: "rg-mods",
		},
		{
			name: "by_cluster_id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := newTestStack(t)

			input := &elasticachesdk.ListAllowedNodeTypeModificationsInput{}
			if tt.rgID != "" {
				input.ReplicationGroupId = aws.String(tt.rgID)
			}

			out, err := client.ListAllowedNodeTypeModifications(t.Context(), input)

			require.NoError(t, err)
			assert.NotEmpty(t, out.ScaleUpModifications)
		})
	}
}

// Test_CreateCacheCluster_RestoreFromSnapshot covers the previously-unhandled
// SnapshotName parameter on CreateCacheCluster: AWS restores a new cluster
// from an existing snapshot, and the emulator's handler never even read the
// form field. It's wired now to validate the snapshot exists and inherit the
// snapshot's engine/node type when the caller doesn't override them. A
// missing snapshot surfaces as InvalidParameterValueException (400), not
// SnapshotNotFoundFault -- CreateCacheCluster's modeled error list in
// api-2.json doesn't include SnapshotNotFoundFault at all, so aws-sdk-go-v2
// has no deserializer case for it on this operation.
func Test_CreateCacheCluster_RestoreFromSnapshot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		snapshotName string
		wantErr      bool
	}{
		{name: "restores_engine_and_node_type_from_snapshot", snapshotName: "src-snap"},
		{name: "missing_snapshot_is_not_found", snapshotName: "no-such-snap", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := newTestStack(t)
			ctx := t.Context()

			_, err := client.CreateCacheCluster(ctx, &elasticachesdk.CreateCacheClusterInput{
				CacheClusterId: aws.String("src-cluster"),
				Engine:         aws.String("memcached"),
				CacheNodeType:  aws.String("cache.m5.large"),
				NumCacheNodes:  aws.Int32(1),
			})
			require.NoError(t, err)

			_, err = client.CreateSnapshot(ctx, &elasticachesdk.CreateSnapshotInput{
				SnapshotName:   aws.String("src-snap"),
				CacheClusterId: aws.String("src-cluster"),
			})
			require.NoError(t, err)

			out, err := client.CreateCacheCluster(ctx, &elasticachesdk.CreateCacheClusterInput{
				CacheClusterId: aws.String("restored-cluster"),
				SnapshotName:   aws.String(tt.snapshotName),
			})

			if tt.wantErr {
				require.Error(t, err)
				requireFault[elasticachetypes.InvalidParameterValueException](t, err)
				requireHTTPStatus(t, err, http.StatusBadRequest)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, "memcached", aws.ToString(out.CacheCluster.Engine))
			assert.Equal(t, "cache.m5.large", aws.ToString(out.CacheCluster.CacheNodeType))
		})
	}
}

// TestCreateCacheCluster_RejectedSnapshotRestore_DoesNotCreateCluster locks
// gopherstack-8haq: applySnapshotDefaults rejects via xmlError and returns
// its result, but xmlError/xmlResp return nil after a successful write, so
// createCacheCluster's "if restoreErr != nil { return restoreErr }" never
// fired and execution fell through to actually creating the cluster. The
// client saw the correct 400 InvalidParameterValueException, but the
// resource existed anyway -- reproduced here through the real SDK client,
// not by inspecting the response alone.
func TestCreateCacheCluster_RejectedSnapshotRestore_DoesNotCreateCluster(t *testing.T) {
	t.Parallel()

	client := newTestStack(t)
	ctx := t.Context()

	_, err := client.CreateCacheCluster(ctx, &elasticachesdk.CreateCacheClusterInput{
		CacheClusterId: aws.String("probe-cluster"),
		Engine:         aws.String("redis"),
		SnapshotName:   aws.String("does-not-exist"),
	})
	require.Error(t, err)
	requireFault[elasticachetypes.InvalidParameterValueException](t, err)
	requireHTTPStatus(t, err, http.StatusBadRequest)

	_, descErr := client.DescribeCacheClusters(ctx, &elasticachesdk.DescribeCacheClustersInput{
		CacheClusterId: aws.String("probe-cluster"),
	})
	require.Error(t, descErr, "rejected CreateCacheCluster must not have created the cluster")
	requireFault[elasticachetypes.CacheClusterNotFoundFault](t, descErr)
}

// subnetGroupFailingBackend wraps InMemoryBackend and fails
// SetClusterSubnetGroupName deterministically, to exercise
// applyClusterSubnetGroup's error path (gopherstack-8haq) without relying on
// a race against the cluster it was just given.
type subnetGroupFailingBackend struct {
	*elasticache.InMemoryBackend
}

var errSubnetGroupBackendFailure = errors.New("boom: subnet group backend failure")

func (b *subnetGroupFailingBackend) SetClusterSubnetGroupName(context.Context, string, string) error {
	return errSubnetGroupBackendFailure
}

// TestCreateCacheCluster_SubnetGroupFailure_StopsBeforeReplicationGroupAttach
// locks gopherstack-8haq's second call site: applyClusterSubnetGroup used to
// reject via xmlError and return its result, which xmlError/xmlResp turn
// into nil after a successful write, so createCacheCluster's
// "if sgErr != nil { return sgErr }" never fired and execution fell through
// to later steps -- including attaching the cluster to a replication group,
// which real AWS would never do once the create is rejected. The base
// cluster row is unavoidably created before this step runs (CreateCacheCluster
// isn't transactional across these helpers -- a separate, larger concern),
// so the fix is verified by the operation actually stopping: the
// ReplicationGroupId attach that comes after applyClusterSubnetGroup in
// createCacheCluster must never run.
func TestCreateCacheCluster_SubnetGroupFailure_StopsBeforeReplicationGroupAttach(t *testing.T) {
	t.Parallel()

	inner := elasticache.NewInMemoryBackend(elasticache.EngineStub, "000000000000", "us-east-1", nil)
	backend := &subnetGroupFailingBackend{InMemoryBackend: inner}
	handler := elasticache.NewHandler(backend)

	e := echo.New()
	registry := service.NewRegistry()
	require.NoError(t, registry.Register(handler))
	router := service.NewServiceRouter(registry)
	e.Use(router.RouteHandler())

	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)

	cfg, err := awscfg.LoadDefaultConfig(
		t.Context(),
		awscfg.WithRegion("us-east-1"),
		awscfg.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
	)
	require.NoError(t, err)

	client := elasticachesdk.NewFromConfig(cfg, func(o *elasticachesdk.Options) {
		o.BaseEndpoint = aws.String(srv.URL)
	})

	_, err = client.CreateReplicationGroup(t.Context(), &elasticachesdk.CreateReplicationGroupInput{
		ReplicationGroupId:          aws.String("rg-sg-guard"),
		ReplicationGroupDescription: aws.String("test"),
	})
	require.NoError(t, err)

	_, err = client.CreateCacheCluster(t.Context(), &elasticachesdk.CreateCacheClusterInput{
		CacheClusterId:       aws.String("sg-fail-cluster"),
		Engine:               aws.String("redis"),
		CacheSubnetGroupName: aws.String("some-subnet-group"),
		ReplicationGroupId:   aws.String("rg-sg-guard"),
	}, func(o *elasticachesdk.Options) { o.RetryMaxAttempts = 1 })
	require.Error(t, err)

	var apiErr smithy.APIError
	require.ErrorAs(t, err, &apiErr, "SDK must surface a typed API error, not an opaque one")
	assert.Equal(t, "InternalFailure", apiErr.ErrorCode())
	requireHTTPStatus(t, err, http.StatusInternalServerError)

	described, descErr := client.DescribeCacheClusters(t.Context(), &elasticachesdk.DescribeCacheClustersInput{
		CacheClusterId: aws.String("sg-fail-cluster"),
	})
	require.NoError(t, descErr)
	require.Len(t, described.CacheClusters, 1)
	assert.Empty(
		t,
		aws.ToString(described.CacheClusters[0].ReplicationGroupId),
		"a rejected subnet-group write must stop createCacheCluster before it reaches the ReplicationGroupId attach step",
	)
}

// TestCreateCacheCluster_InvalidSnapshotRetentionLimit_StopsBeforeReplicationGroupAttach
// locks gopherstack-8haq's third call site: applyClusterSnapshotRetentionLimit
// used to reject a non-integer SnapshotRetentionLimit via xmlError and return
// its result, silently swallowed the same way. The real SDK types
// SnapshotRetentionLimit as int32, so a malformed value can only be produced
// with a raw request, bypassing the SDK's own type safety. See the subnet-group
// test above for why the observable check is "the later ReplicationGroupId
// attach never runs" rather than "the cluster row doesn't exist".
func TestCreateCacheCluster_InvalidSnapshotRetentionLimit_StopsBeforeReplicationGroupAttach(t *testing.T) {
	t.Parallel()

	backend := elasticache.NewInMemoryBackend(elasticache.EngineStub, "000000000000", "us-east-1", nil)
	handler := elasticache.NewHandler(backend)

	e := echo.New()
	registry := service.NewRegistry()
	require.NoError(t, registry.Register(handler))
	router := service.NewServiceRouter(registry)
	e.Use(router.RouteHandler())

	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)

	cfg, err := awscfg.LoadDefaultConfig(
		t.Context(),
		awscfg.WithRegion("us-east-1"),
		awscfg.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
	)
	require.NoError(t, err)

	client := elasticachesdk.NewFromConfig(cfg, func(o *elasticachesdk.Options) {
		o.BaseEndpoint = aws.String(srv.URL)
	})

	_, err = client.CreateReplicationGroup(t.Context(), &elasticachesdk.CreateReplicationGroupInput{
		ReplicationGroupId:          aws.String("rg-srl-guard"),
		ReplicationGroupDescription: aws.String("test"),
	})
	require.NoError(t, err)

	form := url.Values{
		"Action":                 {"CreateCacheCluster"},
		"Version":                {"2015-02-02"},
		"CacheClusterId":         {"srl-bad-cluster"},
		"Engine":                 {"redis"},
		"SnapshotRetentionLimit": {"not-a-number"},
		"ReplicationGroupId":     {"rg-srl-guard"},
	}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL, strings.NewReader(form.Encode()))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	described, descErr := client.DescribeCacheClusters(t.Context(), &elasticachesdk.DescribeCacheClustersInput{
		CacheClusterId: aws.String("srl-bad-cluster"),
	})
	require.NoError(t, descErr)
	require.Len(t, described.CacheClusters, 1)
	assert.Empty(
		t,
		aws.ToString(described.CacheClusters[0].ReplicationGroupId),
		"a rejected SnapshotRetentionLimit write must stop createCacheCluster before the RG attach step",
	)
}

// TestModifyCacheCluster_InvalidSnapshotRetentionLimit_LeavesClusterUnchanged
// locks gopherstack-8haq's fourth call site: applyClusterSnapshotRetentionLimit
// used inside modifyCacheCluster the same way as createCacheCluster's copy,
// checked at the old handler_cache_clusters.go:450. Unlike the Create paths,
// nothing runs after this check in modifyCacheCluster, so the swallowed
// rejection can't be observed as an extra unwanted backend write -- what it
// actually produced was wire corruption: xmlError's write inside the helper
// already sent the 400 ErrorResponse body, and the unguarded fall-through
// then wrote a second, unrelated 200 ModifyCacheClusterResponse document
// onto the *same* HTTP response body (echo's Response.Write has no
// "already committed" guard, only WriteHeader does -- see
// response.go:50-73 in labstack/echo/v5). The client-visible status stayed
// 400 either way, so the only way to catch this is to inspect the raw body.
func TestModifyCacheCluster_InvalidSnapshotRetentionLimit_LeavesClusterUnchanged(t *testing.T) {
	t.Parallel()

	backend := elasticache.NewInMemoryBackend(elasticache.EngineStub, "000000000000", "us-east-1", nil)
	handler := elasticache.NewHandler(backend)

	e := echo.New()
	registry := service.NewRegistry()
	require.NoError(t, registry.Register(handler))
	router := service.NewServiceRouter(registry)
	e.Use(router.RouteHandler())

	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)

	cfg, err := awscfg.LoadDefaultConfig(
		t.Context(),
		awscfg.WithRegion("us-east-1"),
		awscfg.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
	)
	require.NoError(t, err)

	client := elasticachesdk.NewFromConfig(cfg, func(o *elasticachesdk.Options) {
		o.BaseEndpoint = aws.String(srv.URL)
	})

	_, err = client.CreateCacheCluster(t.Context(), &elasticachesdk.CreateCacheClusterInput{
		CacheClusterId:         aws.String("srl-modify-cluster"),
		Engine:                 aws.String("redis"),
		SnapshotRetentionLimit: aws.Int32(5),
	})
	require.NoError(t, err)

	form := url.Values{
		"Action":                 {"ModifyCacheCluster"},
		"Version":                {"2015-02-02"},
		"CacheClusterId":         {"srl-modify-cluster"},
		"SnapshotRetentionLimit": {"not-a-number"},
	}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL, strings.NewReader(form.Encode()))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.NotContains(t, string(body), "ModifyCacheClusterResponse",
		"a rejected SnapshotRetentionLimit must stop before writing a second (success) response onto the same body")

	described, err := client.DescribeCacheClusters(t.Context(), &elasticachesdk.DescribeCacheClustersInput{
		CacheClusterId: aws.String("srl-modify-cluster"),
	})
	require.NoError(t, err)
	require.Len(t, described.CacheClusters, 1)
	assert.Equal(t, int32(5), aws.ToInt32(described.CacheClusters[0].SnapshotRetentionLimit),
		"rejected ModifyCacheCluster (invalid SnapshotRetentionLimit) must not silently change other fields either")
}

func TestHandler_DescribeCacheClusters_ShowCacheClustersNotInReplicationGroups(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup   func(t *testing.T, client *elasticachesdk.Client, b *elasticache.InMemoryBackend)
		name    string
		wantIDs []string
		notInRG bool
		wantAll bool
	}{
		{
			name:    "false_or_omitted_returns_all",
			notInRG: false,
			wantAll: true,
			setup: func(t *testing.T, client *elasticachesdk.Client, b *elasticache.InMemoryBackend) {
				t.Helper()
				_, err := client.CreateCacheCluster(t.Context(), &elasticachesdk.CreateCacheClusterInput{
					CacheClusterId: aws.String("standalone-cl"),
					Engine:         aws.String("redis"),
				})
				require.NoError(t, err)
				// Seed a cluster that belongs to a RG.
				elasticache.AddClusterInRGInternal(b, "rg-member-cl", "some-rg")
			},
		},
		{
			name:    "true_returns_only_standalone_clusters",
			notInRG: true,
			wantIDs: []string{"standalone-cl"},
			setup: func(t *testing.T, client *elasticachesdk.Client, b *elasticache.InMemoryBackend) {
				t.Helper()
				_, err := client.CreateCacheCluster(t.Context(), &elasticachesdk.CreateCacheClusterInput{
					CacheClusterId: aws.String("standalone-cl"),
					Engine:         aws.String("redis"),
				})
				require.NoError(t, err)
				elasticache.AddClusterInRGInternal(b, "rg-member-cl", "some-rg")
			},
		},
		{
			name:    "true_returns_multiple_standalone",
			notInRG: true,
			wantIDs: []string{"sa-1", "sa-2"},
			setup: func(t *testing.T, client *elasticachesdk.Client, b *elasticache.InMemoryBackend) {
				t.Helper()
				for _, id := range []string{"sa-1", "sa-2"} {
					_, err := client.CreateCacheCluster(t.Context(), &elasticachesdk.CreateCacheClusterInput{
						CacheClusterId: aws.String(id),
						Engine:         aws.String("redis"),
					})
					require.NoError(t, err)
				}
				// Seed RG-member clusters — they must be excluded.
				elasticache.AddClusterInRGInternal(b, "rg-member-1", "rg-x")
				elasticache.AddClusterInRGInternal(b, "rg-member-2", "rg-x")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := elasticache.NewInMemoryBackend(elasticache.EngineStub, "000000000000", "us-east-1", nil)
			client := newTestStackSeeded(t, b)

			if tt.setup != nil {
				tt.setup(t, client, b)
			}

			out, err := client.DescribeCacheClusters(t.Context(), &elasticachesdk.DescribeCacheClustersInput{
				ShowCacheClustersNotInReplicationGroups: aws.Bool(tt.notInRG),
			})
			require.NoError(t, err)

			if tt.wantAll {
				assert.NotEmpty(t, out.CacheClusters)

				return
			}

			ids := make([]string, 0, len(out.CacheClusters))
			for _, cl := range out.CacheClusters {
				ids = append(ids, aws.ToString(cl.CacheClusterId))
			}
			assert.ElementsMatch(t, tt.wantIDs, ids)
		})
	}
}

// TestCacheCluster_SnapshotRetentionLimit locks two real bugs:
//  1. SnapshotRetentionLimit was entirely dropped on Create/Modify/Describe for
//     standalone cache clusters (unlike ReplicationGroup/ServerlessCache, which
//     already wire it) -- api_op_CreateCacheCluster.go/api_op_ModifyCacheCluster.go
//     both declare it, and api_op_ModifyCacheCluster.go documents 0 as a
//     meaningful explicit value ("If the value of SnapshotRetentionLimit is set
//     to zero (0), backups are turned off"), not "leave unchanged".
//  2. Omitting SnapshotRetentionLimit on a later ModifyCacheCluster call must
//     leave a previously-set value alone rather than clobbering it back to zero.
func TestCacheCluster_SnapshotRetentionLimit(t *testing.T) {
	t.Parallel()

	client := newTestStack(t)

	created, err := client.CreateCacheCluster(t.Context(), &elasticachesdk.CreateCacheClusterInput{
		CacheClusterId:         aws.String("srl-cluster"),
		Engine:                 aws.String("redis"),
		CacheNodeType:          aws.String("cache.t3.micro"),
		SnapshotRetentionLimit: aws.Int32(5),
	})
	require.NoError(t, err)
	require.Equal(t, int32(5), aws.ToInt32(created.CacheCluster.SnapshotRetentionLimit),
		"SnapshotRetentionLimit must round-trip from CreateCacheCluster, not be silently dropped")

	modifiedNoOp, err := client.ModifyCacheCluster(t.Context(), &elasticachesdk.ModifyCacheClusterInput{
		CacheClusterId: aws.String("srl-cluster"),
		EngineVersion:  aws.String("7.1"),
	})
	require.NoError(t, err)
	assert.Equal(t, int32(5), aws.ToInt32(modifiedNoOp.CacheCluster.SnapshotRetentionLimit),
		"omitting SnapshotRetentionLimit on Modify must leave the prior value alone, not clobber it to zero")

	_, err = client.ModifyCacheCluster(t.Context(), &elasticachesdk.ModifyCacheClusterInput{
		CacheClusterId:         aws.String("srl-cluster"),
		SnapshotRetentionLimit: aws.Int32(0),
	})
	require.NoError(t, err)

	// A second no-op Modify (SnapshotRetentionLimit omitted again) proves the
	// explicit-zero write actually landed server-side rather than being
	// silently ignored: if it had been ignored, the stored value would still
	// be 5 and this describe would show 5, not 0.
	_, err = client.ModifyCacheCluster(t.Context(), &elasticachesdk.ModifyCacheClusterInput{
		CacheClusterId: aws.String("srl-cluster"),
		EngineVersion:  aws.String("7.1"),
	})
	require.NoError(t, err)

	described, err := client.DescribeCacheClusters(t.Context(), &elasticachesdk.DescribeCacheClustersInput{
		CacheClusterId: aws.String("srl-cluster"),
	})
	require.NoError(t, err)
	require.Len(t, described.CacheClusters, 1)
	assert.Equal(t, int32(0), aws.ToInt32(described.CacheClusters[0].SnapshotRetentionLimit),
		"explicit SnapshotRetentionLimit=0 must be honoured (AWS: backups turned off) and persisted, not ignored")
}

// TestCreateCacheCluster_ReplicationGroupId_AttachesAndProtectsFromDelete
// proves gopherstack-v5fe's fix end to end through the real API surface, not
// whitebox test seeding: real CreateCacheCluster's ReplicationGroupId
// parameter (api_op_CreateCacheCluster.go:299-309) "adds the cluster to the
// specified replication group as a read replica", and DeleteCacheCluster
// must then refuse to delete it as the group's last member
// (cache_clusters.go isLastRGMemberLocked). Before the fix,
// Cluster.ReplicationGroupID was never set by any real API call, so this
// guard could not fire this way and this test failed with no error at the
// DeleteCacheCluster step.
func TestCreateCacheCluster_ReplicationGroupId_AttachesAndProtectsFromDelete(t *testing.T) {
	t.Parallel()

	client := newTestStack(t)

	_, err := client.CreateReplicationGroup(t.Context(), &elasticachesdk.CreateReplicationGroupInput{
		ReplicationGroupId:          aws.String("rg-attach-target"),
		ReplicationGroupDescription: aws.String("test"),
	})
	require.NoError(t, err)

	created, err := client.CreateCacheCluster(t.Context(), &elasticachesdk.CreateCacheClusterInput{
		CacheClusterId:     aws.String("rg-attach-replica"),
		Engine:             aws.String("redis"),
		ReplicationGroupId: aws.String("rg-attach-target"),
	})
	require.NoError(t, err)
	assert.Equal(t, "rg-attach-target", aws.ToString(created.CacheCluster.ReplicationGroupId),
		"ReplicationGroupId must round-trip from CreateCacheCluster, proving the cluster actually attached")

	described, err := client.DescribeCacheClusters(t.Context(), &elasticachesdk.DescribeCacheClustersInput{
		CacheClusterId: aws.String("rg-attach-replica"),
	})
	require.NoError(t, err)
	require.Len(t, described.CacheClusters, 1)
	assert.Equal(t, "rg-attach-target", aws.ToString(described.CacheClusters[0].ReplicationGroupId))

	_, err = client.DeleteCacheCluster(t.Context(), &elasticachesdk.DeleteCacheClusterInput{
		CacheClusterId: aws.String("rg-attach-replica"),
	})
	require.Error(t, err, "deleting the last real-API-attached RG member must be refused")
	requireFault[elasticachetypes.InvalidCacheClusterStateFault](t, err)
	requireHTTPStatus(t, err, http.StatusBadRequest)
}

// TestCreateCacheCluster_ReplicationGroupId_NotFound pins the modeled
// ReplicationGroupNotFoundFault (CreateCacheCluster's errors list in
// botocore's service-2.json includes it) for a ReplicationGroupId naming a
// replication group that does not exist, and confirms no cluster is left
// behind by the rejected create.
func TestCreateCacheCluster_ReplicationGroupId_NotFound(t *testing.T) {
	t.Parallel()

	client := newTestStack(t)

	_, err := client.CreateCacheCluster(t.Context(), &elasticachesdk.CreateCacheClusterInput{
		CacheClusterId:     aws.String("rg-attach-orphan"),
		Engine:             aws.String("redis"),
		ReplicationGroupId: aws.String("no-such-rg"),
	})
	require.Error(t, err)
	requireFault[elasticachetypes.ReplicationGroupNotFoundFault](t, err)
	requireHTTPStatus(t, err, http.StatusNotFound)

	_, err = client.DescribeCacheClusters(t.Context(), &elasticachesdk.DescribeCacheClustersInput{
		CacheClusterId: aws.String("rg-attach-orphan"),
	})
	require.Error(t, err, "a create rejected for a nonexistent replication group must not leave a cluster behind")
	requireFault[elasticachetypes.CacheClusterNotFoundFault](t, err)
}

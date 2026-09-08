package elasticache_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/elasticache"
)

// mockDNSRegistrar is a simple in-memory DNSRegistrar for testing.
type mockDNSRegistrar struct {
	registered   map[string]bool
	deregistered map[string]bool
	mu           sync.Mutex
}

func newMockDNS() *mockDNSRegistrar {
	return &mockDNSRegistrar{
		registered:   make(map[string]bool),
		deregistered: make(map[string]bool),
	}
}

func (m *mockDNSRegistrar) Register(hostname string) {
	m.mu.Lock()
	m.registered[hostname] = true
	m.mu.Unlock()
}

func (m *mockDNSRegistrar) Deregister(hostname string) {
	m.mu.Lock()
	m.deregistered[hostname] = true
	m.mu.Unlock()
}

func TestCreateCluster(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		wantPrefix string
		wantSuffix string
		withDNS    bool
	}{
		{
			name:       "dns_registration",
			withDNS:    true,
			wantPrefix: "my-cache.",
			wantSuffix: ".us-east-1.cache.amazonaws.com",
		},
		{
			name:       "no_dns_still_works",
			withDNS:    false,
			wantSuffix: ".cache.amazonaws.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var dns *mockDNSRegistrar
			backend := elasticache.NewInMemoryBackend(elasticache.EngineStub, "123456789012", "us-east-1", nil)

			if tt.withDNS {
				dns = newMockDNS()
				backend.SetDNSRegistrar(dns)
			}

			cluster, err := backend.CreateCluster(context.Background(), "my-cache", "redis", "cache.t3.micro", 0)
			require.NoError(t, err)

			if tt.wantPrefix != "" {
				assert.True(t, strings.HasPrefix(cluster.Endpoint, tt.wantPrefix))
			}
			assert.True(t, strings.HasSuffix(cluster.Endpoint, tt.wantSuffix))

			if tt.withDNS {
				assert.True(t, dns.registered[cluster.Endpoint], "hostname should be registered with DNS")
			}
		})
	}
}

func TestDeleteCluster_DNSDeregistration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{
			name: "deregisters_on_delete",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dns := newMockDNS()
			backend := elasticache.NewInMemoryBackend(elasticache.EngineStub, "123456789012", "us-east-1", nil)
			backend.SetDNSRegistrar(dns)

			cluster, err := backend.CreateCluster(context.Background(), "my-cache", "redis", "cache.t3.micro", 0)
			require.NoError(t, err)

			endpoint := cluster.Endpoint

			err = backend.DeleteCluster(context.Background(), "my-cache")
			require.NoError(t, err)

			assert.True(t, dns.deregistered[endpoint], "hostname should be deregistered from DNS on delete")
		})
	}
}

func TestCreateClusterWithOptions_AtomicNoLeak(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr        error
		name           string
		paramGroupName string
	}{
		{
			name:           "param_group_not_found_no_leak",
			paramGroupName: "nonexistent-pg",
			wantErr:        elasticache.ErrParameterGroupNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := elasticache.NewInMemoryBackend(elasticache.EngineEmbedded, "123456789012", "us-east-1", nil)

			_, err := backend.CreateClusterWithOptions(context.Background(),
				"my-cache",
				"redis",
				"cache.t3.micro",
				tt.paramGroupName,
				"",
				"",
				1,
				0,
			)
			require.ErrorIs(t, err, tt.wantErr)

			_, descErr := backend.DescribeClusters(context.Background(), "my-cache", "", 0, false)
			require.ErrorIs(t, descErr, elasticache.ErrClusterNotFound)
		})
	}
}

func TestCreateClusterWithOptions_FamilyValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr        error
		name           string
		engine         string
		paramGroupName string
	}{
		{
			name:           "redis_cluster_with_memcached_param_group",
			engine:         "redis",
			paramGroupName: "default.memcached1.6",
			wantErr:        elasticache.ErrInvalidParameterGroupFamily,
		},
		{
			name:           "memcached_cluster_with_redis_param_group",
			engine:         "memcached",
			paramGroupName: "default.redis7",
			wantErr:        elasticache.ErrInvalidParameterGroupFamily,
		},
		{
			name:           "redis_cluster_with_redis_param_group",
			engine:         "redis",
			paramGroupName: "default.redis7",
			wantErr:        nil,
		},
		{
			name:           "memcached_cluster_with_memcached_param_group",
			engine:         "memcached",
			paramGroupName: "default.memcached1.6",
			wantErr:        nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := elasticache.NewInMemoryBackend(elasticache.EngineStub, "123456789012", "us-east-1", nil)

			_, err := backend.CreateClusterWithOptions(context.Background(),
				"my-cache",
				tt.engine,
				"cache.t3.micro",
				tt.paramGroupName,
				"",
				"",
				1,
				0,
			)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestModifyCluster_ScalesAndEngineVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		engineVersion string
		wantVersion   string
		numCacheNodes int
		wantNodes     int
	}{
		{
			name:          "scale_and_engine_version",
			numCacheNodes: 3,
			engineVersion: "7.2.0",
			wantNodes:     3,
			wantVersion:   "7.2.0",
		},
		{
			name:          "engine_version_only",
			numCacheNodes: 0,
			engineVersion: "6.2.0",
			wantNodes:     1,
			wantVersion:   "6.2.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := elasticache.NewInMemoryBackend(elasticache.EngineStub, "123456789012", "us-east-1", nil)

			_, err := backend.CreateCluster(context.Background(), "mod-cluster", "redis", "cache.t3.micro", 0)
			require.NoError(t, err)

			modified, err := backend.ModifyCluster(
				context.Background(),
				"mod-cluster",
				"",
				"",
				tt.engineVersion,
				"",
				"",
				tt.numCacheNodes,
			)
			require.NoError(t, err)

			assert.Equal(t, tt.wantVersion, modified.EngineVersion)
			assert.Equal(t, tt.wantNodes, modified.NumCacheNodes)
		})
	}
}

func TestBackend_CreateCluster(t *testing.T) {
	t.Parallel()

	tests := []struct {
		check         func(t *testing.T, cl *elasticache.Cluster)
		name          string
		clusterID     string
		engine        string
		nodeType      string
		numCacheNodes int
	}{
		{
			name:          "default_engine",
			clusterID:     "default-cluster",
			engine:        "",
			nodeType:      "cache.t3.micro",
			numCacheNodes: 1,
			check: func(t *testing.T, cl *elasticache.Cluster) {
				t.Helper()
				assert.Equal(t, "default-cluster", cl.ClusterID)
				assert.Equal(t, "available", cl.Status)
				assert.NotEmpty(t, cl.ARN)
				assert.Contains(t, cl.ARN, "arn:aws:elasticache:us-east-1:000000000000:cluster:default-cluster")
			},
		},
		{
			name:          "redis",
			clusterID:     "redis-cluster",
			engine:        "redis",
			nodeType:      "cache.r6g.large",
			numCacheNodes: 1,
			check: func(t *testing.T, cl *elasticache.Cluster) {
				t.Helper()
				assert.Equal(t, "redis", cl.Engine)
			},
		},
		{
			name:          "memcached",
			clusterID:     "memcached-cluster",
			engine:        "memcached",
			nodeType:      "cache.t3.micro",
			numCacheNodes: 3,
			check: func(t *testing.T, cl *elasticache.Cluster) {
				t.Helper()
				assert.Equal(t, "memcached", cl.Engine)
				assert.Equal(t, 3, cl.NumCacheNodes)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := elasticache.NewInMemoryBackend(elasticache.EngineStub, "000000000000", "us-east-1", nil)

			cl, err := b.CreateClusterWithOptions(
				context.Background(),
				tt.clusterID,
				tt.engine,
				tt.nodeType,
				"",
				"",
				"",
				tt.numCacheNodes,
				0,
			)
			require.NoError(t, err)
			tt.check(t, cl)
		})
	}
}

func TestBackend_CreateCluster_AlreadyExists(t *testing.T) {
	t.Parallel()

	b := elasticache.NewInMemoryBackend(elasticache.EngineStub, "000000000000", "us-east-1", nil)

	_, err := b.CreateClusterWithOptions(
		context.Background(),
		"dup-cluster",
		"redis",
		"cache.t3.micro",
		"",
		"",
		"",
		1,
		0,
	)
	require.NoError(t, err)

	_, err = b.CreateClusterWithOptions(
		context.Background(),
		"dup-cluster",
		"redis",
		"cache.t3.micro",
		"",
		"",
		"",
		1,
		0,
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, elasticache.ErrClusterAlreadyExists)
}

func TestBackend_DeleteCluster_NotFound(t *testing.T) {
	t.Parallel()

	b := elasticache.NewInMemoryBackend(elasticache.EngineStub, "000000000000", "us-east-1", nil)

	err := b.DeleteCluster(context.Background(), "nonexistent")
	require.Error(t, err)
	assert.ErrorIs(t, err, elasticache.ErrClusterNotFound)
}

func TestBackend_DeleteCluster_LastRGMemberRejected(t *testing.T) {
	t.Parallel()

	b := elasticache.NewInMemoryBackend(elasticache.EngineStub, "000000000000", "us-east-1", nil)

	elasticache.AddClusterInRGInternal(b, "rg-solo-member", "solo-rg")

	err := b.DeleteCluster(context.Background(), "rg-solo-member")
	require.ErrorIs(t, err, elasticache.ErrClusterInReplicationGroup)

	// A non-last member may still be deleted directly.
	elasticache.AddClusterInRGInternal(b, "rg-member-a", "pair-rg")
	elasticache.AddClusterInRGInternal(b, "rg-member-b", "pair-rg")

	require.NoError(t, b.DeleteCluster(context.Background(), "rg-member-a"))

	// Now the sole remaining member is rejected too.
	err = b.DeleteCluster(context.Background(), "rg-member-b")
	require.ErrorIs(t, err, elasticache.ErrClusterInReplicationGroup)
}

func TestBackend_SetClusterReplicationGroupID(t *testing.T) {
	t.Parallel()

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		b := elasticache.NewInMemoryBackend(elasticache.EngineStub, "000000000000", "us-east-1", nil)

		_, err := b.CreateClusterWithOptions(
			context.Background(), "setrg-cl", "redis", "cache.t3.micro", "", "", "", 1, 0,
		)
		require.NoError(t, err)

		err = b.SetClusterReplicationGroupID(context.Background(), "setrg-cl", "rg-missing")
		require.ErrorIs(t, err, elasticache.ErrReplicationGroupNotFound)
	})

	t.Run("attaches", func(t *testing.T) {
		t.Parallel()

		b := elasticache.NewInMemoryBackend(elasticache.EngineStub, "000000000000", "us-east-1", nil)

		_, err := b.CreateReplicationGroup(context.Background(), "setrg-rg", "desc")
		require.NoError(t, err)

		_, err = b.CreateClusterWithOptions(
			context.Background(), "setrg-cl2", "redis", "cache.t3.micro", "", "", "", 1, 0,
		)
		require.NoError(t, err)

		require.NoError(t, b.SetClusterReplicationGroupID(context.Background(), "setrg-cl2", "setrg-rg"))

		p, err := b.DescribeClusters(context.Background(), "setrg-cl2", "", 0, false)
		require.NoError(t, err)
		require.Len(t, p.Data, 1)
		assert.Equal(t, "setrg-rg", p.Data[0].ReplicationGroupID)
	})
}

func TestBackend_DescribeClusters_Pagination(t *testing.T) {
	t.Parallel()

	b := elasticache.NewInMemoryBackend(elasticache.EngineStub, "000000000000", "us-east-1", nil)

	for i := range 5 {
		_, err := b.CreateClusterWithOptions(context.Background(),
			"cl-"+string(rune('a'+i)), "redis", "cache.t3.micro", "", "", "", 1, 0,
		)
		require.NoError(t, err)
	}

	p1, err := b.DescribeClusters(context.Background(), "", "", 3, false)
	require.NoError(t, err)
	assert.Len(t, p1.Data, 3)
	assert.NotEmpty(t, p1.Next)

	p2, err := b.DescribeClusters(context.Background(), "", p1.Next, 3, false)
	require.NoError(t, err)
	assert.Len(t, p2.Data, 2)
	assert.Empty(t, p2.Next)
}

func TestBackend_ModifyCluster_EngineVersion(t *testing.T) {
	t.Parallel()

	b := elasticache.NewInMemoryBackend(elasticache.EngineStub, "000000000000", "us-east-1", nil)

	_, err := b.CreateClusterWithOptions(context.Background(), "mod-cl", "redis", "cache.t3.micro", "", "", "", 1, 0)
	require.NoError(t, err)

	cl, err := b.ModifyCluster(context.Background(), "mod-cl", "", "", "7.1.0", "", "", 0)
	require.NoError(t, err)
	assert.Equal(t, "7.1.0", cl.EngineVersion)
}

func TestBackend_ModifyCluster_NodeType(t *testing.T) {
	t.Parallel()

	b := elasticache.NewInMemoryBackend(elasticache.EngineStub, "000000000000", "us-east-1", nil)

	_, err := b.CreateClusterWithOptions(context.Background(), "node-cl", "redis", "cache.t3.micro", "", "", "", 1, 0)
	require.NoError(t, err)

	cl, err := b.ModifyCluster(context.Background(), "node-cl", "cache.r6g.large", "", "", "", "", 0)
	require.NoError(t, err)
	assert.Equal(t, "cache.r6g.large", cl.NodeType)
}

// ----------------------------------------
// CacheParameterGroup CRUD (issue)
// ----------------------------------------

func TestBackend_ListAllowedNodeTypeModifications_ForCluster(t *testing.T) {
	t.Parallel()

	b := elasticache.NewInMemoryBackend(elasticache.EngineStub, "000000000000", "us-east-1", nil)

	_, err := b.CreateClusterWithOptions(
		context.Background(),
		"nodemods-cl",
		"redis",
		"cache.t3.micro",
		"",
		"",
		"",
		1,
		0,
	)
	require.NoError(t, err)

	mods, err := b.ListAllowedNodeTypeModifications(context.Background(), "nodemods-cl", "")
	require.NoError(t, err)
	assert.NotNil(t, mods)
}

func TestBackend_ListAllowedNodeTypeModifications_ForRG(t *testing.T) {
	t.Parallel()

	b := elasticache.NewInMemoryBackend(elasticache.EngineStub, "000000000000", "us-east-1", nil)

	_, err := b.CreateReplicationGroupFull(context.Background(), elasticache.ReplicationGroupCreateOpts{
		ID: "nodemods-rg", Description: "node mods test",
	})
	require.NoError(t, err)

	mods, err := b.ListAllowedNodeTypeModifications(context.Background(), "", "nodemods-rg")
	require.NoError(t, err)
	assert.NotNil(t, mods)
}

// ----------------------------------------
// EngineDefaultParameters
// ----------------------------------------

func TestBackend_DescribeClusters_NotInRG_Filter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		wantCount int
		notInRG   bool
	}{
		{name: "no_filter_returns_all", notInRG: false, wantCount: 3},
		{name: "not_in_rg_excludes_rg_members", notInRG: true, wantCount: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := elasticache.NewInMemoryBackend(elasticache.EngineStub, "000000000000", "us-east-1", nil)

			// One standalone cluster.
			_, err := b.CreateCluster(context.Background(), "standalone", "redis", "cache.t3.micro", 0)
			require.NoError(t, err)

			// Two clusters with ReplicationGroupID set (simulate RG membership).
			elasticache.AddClusterInRGInternal(b, "rg-member-1", "test-rg")
			elasticache.AddClusterInRGInternal(b, "rg-member-2", "test-rg")

			p, err := b.DescribeClusters(context.Background(), "", "", 0, tt.notInRG)
			require.NoError(t, err)
			assert.Len(t, p.Data, tt.wantCount)
		})
	}
}

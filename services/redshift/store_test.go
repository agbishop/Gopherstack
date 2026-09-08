package redshift_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/redshift"
)

func TestRedshiftCreateCluster(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr    error
		setup      func(b *redshift.InMemoryBackend)
		name       string
		clusterID  string
		nodeType   string
		dbName     string
		masterUser string
	}{
		{
			name:       "success",
			clusterID:  "my-cluster",
			nodeType:   "dc2.large",
			dbName:     "mydb",
			masterUser: "admin",
		},
		{
			name:      "empty_id",
			clusterID: "",
			wantErr:   redshift.ErrInvalidParameter,
		},
		{
			name:      "already_exists",
			clusterID: "dup-cluster",
			setup: func(b *redshift.InMemoryBackend) {
				_, _ = b.CreateCluster("dup-cluster", "", "", "", nil, "")
			},
			wantErr: redshift.ErrClusterAlreadyExists,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
			if tt.setup != nil {
				tt.setup(b)
			}
			c, err := b.CreateCluster(tt.clusterID, tt.nodeType, tt.dbName, tt.masterUser, nil, "")
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.clusterID, c.ClusterIdentifier)
			assert.Equal(t, tt.nodeType, c.NodeType)
			assert.Equal(t, tt.dbName, c.DBName)
			assert.Equal(t, tt.masterUser, c.MasterUsername)
			assert.Equal(t, "available", c.Status)
			assert.Contains(t, c.Endpoint, tt.clusterID)
		})
	}
}

func TestRedshiftDeleteCluster(t *testing.T) {
	t.Parallel()

	b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	_, err := b.CreateCluster("del-cluster", "", "", "", nil, "")
	require.NoError(t, err)

	deleted, err := b.DeleteCluster("del-cluster")
	require.NoError(t, err)
	assert.Equal(t, "del-cluster", deleted.ClusterIdentifier)

	_, _, err = b.DescribeClusters("del-cluster", "", 0, nil, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, redshift.ErrClusterNotFound)
}

// TestRedshiftDeleteCluster_ClearsLoggingStatuses verifies that the
// synchronous DeleteCluster path (no activation delay configured) clears
// loggingStatuses for the deleted cluster. Otherwise a new cluster created
// with the same (user-chosen, reusable) ClusterIdentifier inherits the
// deleted cluster's stale logging status.
func TestRedshiftDeleteCluster_ClearsLoggingStatuses(t *testing.T) {
	t.Parallel()

	b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	_, err := b.CreateCluster("reused-cluster", "", "", "", nil, "")
	require.NoError(t, err)

	_, err = b.EnableLogging("reused-cluster", "my-bucket", "")
	require.NoError(t, err)
	assert.Equal(t, 1, redshift.LoggingStatusCount(b))

	_, err = b.DeleteCluster("reused-cluster")
	require.NoError(t, err)

	_, err = b.CreateCluster("reused-cluster", "", "", "", nil, "")
	require.NoError(t, err)

	status, err := b.GetLoggingStatus("reused-cluster")
	require.NoError(t, err)
	assert.False(t, status.LoggingEnabled,
		"recreated cluster must not inherit the deleted cluster's logging status")
	assert.Empty(t, status.BucketName)
}

func TestRedshiftDescribeClusters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr   error
		setup     func(b *redshift.InMemoryBackend)
		name      string
		clusterID string
		wantCount int
	}{
		{
			name: "multiple",
			setup: func(b *redshift.InMemoryBackend) {
				_, _ = b.CreateCluster("cluster-1", "", "", "", nil, "")
				_, _ = b.CreateCluster("cluster-2", "", "", "", nil, "")
			},
			clusterID: "",
			wantCount: 2,
		},
		{
			name:      "not_found",
			clusterID: "nonexistent",
			wantErr:   redshift.ErrClusterNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
			if tt.setup != nil {
				tt.setup(b)
			}
			clusters, _, err := b.DescribeClusters(tt.clusterID, "", 0, nil, nil)
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)
			assert.Len(t, clusters, tt.wantCount)
		})
	}
}

func TestRedshiftCreateTags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr   error
		setup     func(b *redshift.InMemoryBackend)
		tags      map[string]string
		wantTags  map[string]string
		name      string
		clusterID string
	}{
		{
			name:      "success",
			clusterID: "tagged-cluster",
			setup: func(b *redshift.InMemoryBackend) {
				_, _ = b.CreateCluster("tagged-cluster", "dc2.large", "mydb", "admin", nil, "")
			},
			tags:     map[string]string{"env": "prod", "team": "platform"},
			wantTags: map[string]string{"env": "prod", "team": "platform"},
		},
		{
			name:      "overwrite",
			clusterID: "overwrite-cluster",
			setup: func(b *redshift.InMemoryBackend) {
				_, _ = b.CreateCluster("overwrite-cluster", "", "", "", nil, "")
				_ = b.CreateTags("overwrite-cluster", map[string]string{"env": "dev"})
			},
			tags:     map[string]string{"env": "prod"},
			wantTags: map[string]string{"env": "prod"},
		},
		{
			name:      "not_found",
			clusterID: "nonexistent",
			tags:      map[string]string{"k": "v"},
			wantErr:   redshift.ErrClusterNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
			if tt.setup != nil {
				tt.setup(b)
			}
			err := b.CreateTags(tt.clusterID, tt.tags)
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)
			allTags := b.DescribeTags()
			tags, ok := allTags[tt.clusterID]
			require.True(t, ok)
			for k, v := range tt.wantTags {
				assert.Equal(t, v, tags[k])
			}
		})
	}
}

func TestRedshiftDeleteTags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr        error
		setup          func(b *redshift.InMemoryBackend)
		wantTags       map[string]string
		name           string
		clusterID      string
		keysToRemove   []string
		wantAbsentKeys []string
	}{
		{
			name:      "success",
			clusterID: "del-tags-cluster",
			setup: func(b *redshift.InMemoryBackend) {
				_, _ = b.CreateCluster("del-tags-cluster", "", "", "", nil, "")
				_ = b.CreateTags("del-tags-cluster", map[string]string{"env": "prod", "team": "platform"})
			},
			keysToRemove:   []string{"env"},
			wantAbsentKeys: []string{"env"},
			wantTags:       map[string]string{"team": "platform"},
		},
		{
			name:         "not_found",
			clusterID:    "nonexistent",
			keysToRemove: []string{"k"},
			wantErr:      redshift.ErrClusterNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
			if tt.setup != nil {
				tt.setup(b)
			}
			err := b.DeleteTags(tt.clusterID, tt.keysToRemove)
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)
			allTags := b.DescribeTags()
			tags := allTags[tt.clusterID]
			for _, k := range tt.wantAbsentKeys {
				assert.NotContains(t, tags, k)
			}
			for k, v := range tt.wantTags {
				assert.Equal(t, v, tags[k])
			}
		})
	}
}

func TestRedshiftDescribeTags(t *testing.T) {
	t.Parallel()

	b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	_, _ = b.CreateCluster("empty-tags-cluster", "", "", "", nil, "")

	allTags := b.DescribeTags()
	tags, ok := allTags["empty-tags-cluster"]
	require.True(t, ok)
	assert.Empty(t, tags)
}

// ---- Backend.Reset ----

func TestBackend_Reset(t *testing.T) {
	t.Parallel()

	b := redshift.NewInMemoryBackend("000000000000", "us-east-1")

	_, err := b.CreateCluster("c1", "dc2.large", "dev", "admin", nil, "")
	require.NoError(t, err)

	b.AddSnapshotInternal(
		&redshift.Snapshot{SnapshotIdentifier: "snap-1", ClusterIdentifier: "c1", Status: "available"},
	)
	b.AddDataShareInternal(&redshift.DataShare{DataShareArn: "arn:aws:redshift::123:datashare:ds1"})

	assert.Equal(t, 1, redshift.ClusterCount(b))
	assert.Equal(t, 1, redshift.SnapshotCount(b))
	assert.Equal(t, 1, redshift.DataShareCount(b))

	b.Reset()

	assert.Equal(t, 0, redshift.ClusterCount(b))
	assert.Equal(t, 0, redshift.SnapshotCount(b))
	assert.Equal(t, 0, redshift.DataShareCount(b))
}

// ---- Export count helpers ----

func TestExportCountHelpers(t *testing.T) {
	t.Parallel()

	b := redshift.NewInMemoryBackend("000000000000", "us-east-1")

	assert.Equal(t, 0, redshift.ClusterCount(b))
	assert.Equal(t, 0, redshift.ReservedNodeCount(b))
	assert.Equal(t, 0, redshift.PartnerCount(b))
	assert.Equal(t, 0, redshift.DataShareCount(b))
	assert.Equal(t, 0, redshift.SecurityGroupCount(b))
	assert.Equal(t, 0, redshift.SnapshotCount(b))
	assert.Equal(t, 0, redshift.EndpointAuthCount(b))
	assert.Equal(t, 0, redshift.ActiveResizeCount(b))

	_, err := b.CreateCluster("c1", "dc2.large", "dev", "admin", nil, "")
	require.NoError(t, err)
	assert.Equal(t, 1, redshift.ClusterCount(b))

	b.AddReservedNodeInternal(&redshift.ReservedNode{ReservedNodeID: "rn-1"})
	assert.Equal(t, 1, redshift.ReservedNodeCount(b))

	b.AddDataShareInternal(&redshift.DataShare{DataShareArn: "ds-arn"})
	assert.Equal(t, 1, redshift.DataShareCount(b))

	b.AddSecurityGroupInternal(&redshift.ClusterSecurityGroup{ClusterSecurityGroupName: "sg-1"})
	assert.Equal(t, 1, redshift.SecurityGroupCount(b))

	b.AddSnapshotInternal(&redshift.Snapshot{SnapshotIdentifier: "snap-1"})
	assert.Equal(t, 1, redshift.SnapshotCount(b))

	b.AddActiveResizeInternal("c1", &redshift.ResizeProgress{Status: "IN_PROGRESS", AllowCancelResize: true})
	assert.Equal(t, 1, redshift.ActiveResizeCount(b))
}

// ---- Backend.Region and AccountID ----

func TestBackend_RegionAndAccountID(t *testing.T) {
	t.Parallel()

	b := redshift.NewInMemoryBackend("999888777666", "ap-southeast-1")
	assert.Equal(t, "999888777666", b.AccountID())
	assert.Equal(t, "ap-southeast-1", b.Region())
}

// ---- Backend Reset clears HSM and ScheduledAction state ----

func TestBackend_Reset_ClearsNewState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		run  func(t *testing.T, b *redshift.InMemoryBackend)
		name string
	}{
		{
			name: "reset_clears_hsm_certs",
			run: func(t *testing.T, b *redshift.InMemoryBackend) {
				t.Helper()
				_, err := b.CreateHsmClientCertificate("cert-1", nil)
				require.NoError(t, err)
				b.Reset()
				assert.Equal(t, 0, redshift.HsmClientCertCount(b))
			},
		},
		{
			name: "reset_clears_hsm_configs",
			run: func(t *testing.T, b *redshift.InMemoryBackend) {
				t.Helper()
				_, err := b.CreateHsmConfiguration("cfg-1", "desc", "10.0.0.1", "p1", nil)
				require.NoError(t, err)
				b.Reset()
				assert.Equal(t, 0, redshift.HsmConfigCount(b))
			},
		},
		{
			name: "reset_clears_scheduled_actions",
			run: func(t *testing.T, b *redshift.InMemoryBackend) {
				t.Helper()
				_, err := b.CreateScheduledAction("action-1", "cron(0 12 * * ? *)", "", "", nil, nil)
				require.NoError(t, err)
				b.Reset()
				assert.Equal(t, 0, redshift.ScheduledActionCount(b))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := redshift.NewInMemoryBackend("123456789012", "us-east-1")
			tt.run(t, b)
		})
	}
}

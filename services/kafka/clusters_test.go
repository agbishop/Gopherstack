package kafka_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/kafka"
)

func TestCreateCluster(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		clusterName string
		wantErr     bool
	}{
		{
			name:        "success",
			clusterName: "my-cluster",
		},
		{
			name:        "duplicate_name",
			clusterName: "my-cluster",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend(t)

			// Pre-create if testing duplicate
			if tt.wantErr {
				_, err := b.CreateCluster(
					context.Background(),
					"my-cluster",
					"2.8.0",
					3,
					kafka.BrokerNodeGroupInfo{},
					nil,
					nil,
				)
				require.NoError(t, err)
			}

			cluster, err := b.CreateCluster(context.Background(),
				tt.clusterName,
				"2.8.0",
				3,
				kafka.BrokerNodeGroupInfo{},
				nil,
				map[string]string{"env": "test"},
			)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.clusterName, cluster.ClusterName)
			assert.Equal(t, kafka.ClusterStateCreating, cluster.State)
			assert.NotEmpty(t, cluster.ClusterArn)
			assert.Contains(t, cluster.ClusterArn, "cluster/"+tt.clusterName+"/")
		})
	}
}

func TestDescribeCluster(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup   func(*kafka.InMemoryBackend) string
		name    string
		wantErr bool
	}{
		{
			name: "existing_cluster",
			setup: func(b *kafka.InMemoryBackend) string {
				c, err := b.CreateCluster(
					context.Background(),
					"my-cluster",
					"2.8.0",
					3,
					kafka.BrokerNodeGroupInfo{},
					nil,
					nil,
				)
				if err != nil {
					return ""
				}

				return c.ClusterArn
			},
		},
		{
			name: "not_found",
			setup: func(_ *kafka.InMemoryBackend) string {
				return "arn:aws:kafka:us-east-1:000000000000:cluster/nonexistent/uuid"
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend(t)
			arn := tt.setup(b)

			cluster, err := b.DescribeCluster(context.Background(), arn)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, arn, cluster.ClusterArn)
		})
	}
}

func TestListClusters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup     func(*kafka.InMemoryBackend)
		name      string
		wantCount int
	}{
		{
			name:      "empty",
			setup:     func(_ *kafka.InMemoryBackend) {},
			wantCount: 0,
		},
		{
			name: "multiple",
			setup: func(b *kafka.InMemoryBackend) {
				_, _ = b.CreateCluster(
					context.Background(),
					"cluster-a",
					"2.8.0",
					3,
					kafka.BrokerNodeGroupInfo{},
					nil,
					nil,
				)
				_, _ = b.CreateCluster(
					context.Background(),
					"cluster-b",
					"2.8.0",
					3,
					kafka.BrokerNodeGroupInfo{},
					nil,
					nil,
				)
			},
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend(t)
			tt.setup(b)

			clusters := b.ListClusters(context.Background())
			assert.Len(t, clusters, tt.wantCount)
		})
	}
}

func TestDeleteCluster(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup   func(*kafka.InMemoryBackend) string
		name    string
		wantErr bool
	}{
		{
			name: "success",
			setup: func(b *kafka.InMemoryBackend) string {
				c, _ := b.CreateCluster(
					context.Background(),
					"my-cluster",
					"2.8.0",
					3,
					kafka.BrokerNodeGroupInfo{},
					nil,
					nil,
				)

				return c.ClusterArn
			},
		},
		{
			name: "not_found",
			setup: func(_ *kafka.InMemoryBackend) string {
				return "arn:aws:kafka:us-east-1:000000000000:cluster/nonexistent/uuid"
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend(t)
			arn := tt.setup(b)

			err := b.DeleteCluster(context.Background(), arn)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)

			_, err = b.DescribeCluster(context.Background(), arn)
			require.Error(t, err)
		})
	}
}

func TestCreateCluster_RequiresName(t *testing.T) {
	t.Parallel()

	b := kafka.NewInMemoryBackend(testAccountID, testRegion)
	_, err := b.CreateCluster(context.Background(), "", "2.8.0", 3, kafka.BrokerNodeGroupInfo{}, nil, nil)

	require.Error(t, err)
	require.ErrorIs(t, err, kafka.ErrValidation)
}

func TestDeleteCluster_CascadesTopicsAndScram(t *testing.T) {
	t.Parallel()

	b := kafka.NewInMemoryBackend(testAccountID, testRegion)
	cl := b.AddClusterInternal("c1", "2.8.0")

	b.AddTopicInternal(cl.ClusterArn, "t1")
	b.AddTopicInternal(cl.ClusterArn, "t2")

	_, err := b.BatchAssociateScramSecret(context.Background(),
		cl.ClusterArn,
		[]string{"arn:aws:secretsmanager:us-east-1:000000000000:secret:s1"},
	)
	require.NoError(t, err)

	require.Equal(t, 2, kafka.TopicCount(b))
	require.Equal(t, 1, kafka.ScramSecretCount(b))

	err = b.DeleteCluster(context.Background(), cl.ClusterArn)
	require.NoError(t, err)

	assert.Equal(t, 0, kafka.TopicCount(b))
	assert.Equal(t, 0, kafka.ScramSecretCount(b))
}

// TestDeleteCluster_CascadesVpcConnectionsAndChannels proves DeleteCluster no
// longer leaves ghost VpcConnection/Channel rows pointing at a deleted
// cluster: both are cluster-scoped children (VpcConnection.TargetClusterArn,
// Channel.ClusterArn), and prior to this fix only topics/SCRAM secrets/the
// cluster policy were cascaded on delete.
func TestDeleteCluster_CascadesVpcConnectionsAndChannels(t *testing.T) {
	t.Parallel()

	b := kafka.NewInMemoryBackend(testAccountID, testRegion)
	cl := b.AddClusterInternal("c1", "2.8.0")

	conn := b.AddVpcConnectionInternal(cl.ClusterArn, "vpc-1")

	s3Dest, topics := s3ChannelFixtures()
	ch, err := b.CreateChannel(
		context.Background(), cl.ClusterArn, "my-channel", topics, nil, nil, s3Dest, nil, nil,
	)
	require.NoError(t, err)

	require.Len(t, b.ListVpcConnections(context.Background()), 1)

	require.NoError(t, b.DeleteCluster(context.Background(), cl.ClusterArn))

	assert.Empty(
		t,
		b.ListVpcConnections(context.Background()),
		"deleted cluster's VPC connection must not survive as a ghost row",
	)

	_, err = b.DescribeVpcConnection(context.Background(), conn.VpcConnectionArn)
	require.ErrorIs(t, err, kafka.ErrNotFound)

	_, err = b.DescribeChannel(context.Background(), cl.ClusterArn, ch.ChannelArn)
	require.ErrorIs(t, err, kafka.ErrNotFound)
}

func TestSortedListClusters(t *testing.T) {
	t.Parallel()

	b := kafka.NewInMemoryBackend(testAccountID, testRegion)
	b.AddClusterInternal("zzz-cluster", "2.8.0")
	b.AddClusterInternal("aaa-cluster", "2.8.0")
	b.AddClusterInternal("mmm-cluster", "2.8.0")

	clusters := b.ListClusters(context.Background())
	require.Len(t, clusters, 3)
	assert.Equal(t, "aaa-cluster", clusters[0].ClusterName)
	assert.Equal(t, "mmm-cluster", clusters[1].ClusterName)
	assert.Equal(t, "zzz-cluster", clusters[2].ClusterName)
}

func TestNonNilTags_Cluster(t *testing.T) {
	t.Parallel()

	b := kafka.NewInMemoryBackend(testAccountID, testRegion)
	cl := b.AddClusterInternal("c1", "2.8.0")

	assert.NotNil(t, cl.Tags)
}

func TestDeepCopy_ClusterDoesNotAlias(t *testing.T) {
	t.Parallel()

	b := kafka.NewInMemoryBackend(testAccountID, testRegion)
	cl := b.AddClusterInternal("c1", "2.8.0")

	// Mutating the returned clone must not affect the stored cluster.
	cl.ClusterName = "mutated"
	described, err := b.DescribeCluster(context.Background(), cl.ClusterArn)
	require.NoError(t, err)
	assert.Equal(t, "c1", described.ClusterName)
}

func TestClusterNameUniqueness(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		useSeed bool // true = seed via AddClusterInternal, false = via CreateCluster
	}{
		{name: "duplicate_after_create", useSeed: false},
		{name: "duplicate_after_seed", useSeed: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := kafka.NewInMemoryBackend(testAccountID, testRegion)
			ctx := context.Background()

			const clName = "unique-cluster"

			if tt.useSeed {
				b.AddClusterInternal(clName, "3.6.0")
			} else {
				_, err := b.CreateCluster(ctx, clName, "3.6.0", 3, kafka.BrokerNodeGroupInfo{
					InstanceType:  "kafka.m5.large",
					ClientSubnets: []string{"subnet-1"},
				}, nil, nil)
				require.NoError(t, err)
			}

			_, err := b.CreateCluster(ctx, clName, "3.6.0", 3, kafka.BrokerNodeGroupInfo{
				InstanceType:  "kafka.m5.large",
				ClientSubnets: []string{"subnet-1"},
			}, nil, nil)
			require.ErrorIs(t, err, kafka.ErrAlreadyExists)
		})
	}
}

// ----------------------------------------
// Growth cap: cluster map bounded at maxClustersPerRegion
// ----------------------------------------

func TestClusterGrowthCap(t *testing.T) {
	t.Parallel()

	const maxPerRegion = 500

	b := kafka.NewInMemoryBackend(testAccountID, testRegion)
	ctx := context.Background()

	// Seed up to the cap via AddClusterInternal (fast path).
	for i := range maxPerRegion {
		b.AddClusterInternal(fmt.Sprintf("seed-%04d", i), "3.6.0")
	}

	require.Equal(t, maxPerRegion, kafka.ClusterCount(b))

	// One more via CreateCluster: should evict one and stay at cap.
	_, err := b.CreateCluster(ctx, "new-cluster", "3.6.0", 3, kafka.BrokerNodeGroupInfo{
		InstanceType:  "kafka.m5.large",
		ClientSubnets: []string{"subnet-1"},
	}, nil, nil)
	require.NoError(t, err)

	assert.Equal(t, maxPerRegion, kafka.ClusterCount(b))
}

// ----------------------------------------
// Pagination token: invalid token falls back to offset 0
// ----------------------------------------

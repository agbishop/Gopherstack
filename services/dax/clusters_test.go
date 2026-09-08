package dax_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/dax"
)

// ---- CreateCluster ----

func TestCreateCluster(t *testing.T) {
	t.Parallel()

	tests := []struct {
		check   func(t *testing.T, c *dax.Cluster)
		name    string
		input   dax.CreateClusterInput
		wantErr bool
	}{
		{
			name:  "valid minimal input",
			input: validCreateInput("my-cluster"),
			check: func(t *testing.T, c *dax.Cluster) {
				t.Helper()
				assert.Equal(t, "my-cluster", c.ClusterName)
				assert.Equal(t, "dax.r5.large", c.NodeType)
				assert.Equal(t, dax.StatusAvailable, c.Status)
				assert.Equal(t, 1, c.TotalNodes)
				assert.Equal(t, 1, c.ActiveNodes)
				assert.Len(t, c.Nodes, 1)
				assert.NotEmpty(t, c.ClusterArn)
				assert.NotNil(t, c.Endpoint)
				assert.NotEmpty(t, c.Endpoint.Address)
				assert.Equal(t, 8111, c.Endpoint.Port)
				assert.Equal(t, dax.EncryptionTypeNone, c.ClusterEndpointEncryptionType)
			},
		},
		{
			name: "multiple nodes",
			input: func() dax.CreateClusterInput {
				in := validCreateInput("ha-cluster")
				in.ReplicationFactor = 3

				return in
			}(),
			check: func(t *testing.T, c *dax.Cluster) {
				t.Helper()
				assert.Equal(t, 3, c.TotalNodes)
				assert.Len(t, c.Nodes, 3)
			},
		},
		{
			name: "SSE enabled",
			input: func() dax.CreateClusterInput {
				in := validCreateInput("sse-cluster")
				in.SSESpecificationEnabled = true

				return in
			}(),
			check: func(t *testing.T, c *dax.Cluster) {
				t.Helper()
				assert.Equal(t, "ENABLED", c.SSEDescription.Status)
			},
		},
		{
			name: "with tags",
			input: func() dax.CreateClusterInput {
				in := validCreateInput("tagged")
				in.Tags = map[string]string{"env": "test", "team": "platform"}

				return in
			}(),
			check: func(t *testing.T, c *dax.Cluster) {
				t.Helper()
				assert.Equal(t, "test", c.Tags["env"])
				assert.Equal(t, "platform", c.Tags["team"])
			},
		},
		{
			name: "TLS encryption type",
			input: func() dax.CreateClusterInput {
				in := validCreateInput("tls-cluster")
				in.ClusterEndpointEncryptionType = dax.EncryptionTypeTLS

				return in
			}(),
			check: func(t *testing.T, c *dax.Cluster) {
				t.Helper()
				assert.Equal(t, dax.EncryptionTypeTLS, c.ClusterEndpointEncryptionType)
			},
		},
		{
			name: "with notification topic",
			input: func() dax.CreateClusterInput {
				in := validCreateInput("notif-cluster")
				in.NotificationTopicArn = "arn:aws:sns:us-east-1:123456789012:my-topic"

				return in
			}(),
			check: func(t *testing.T, c *dax.Cluster) {
				t.Helper()
				require.NotNil(t, c.NotificationConfiguration)
				assert.Equal(t, "arn:aws:sns:us-east-1:123456789012:my-topic", c.NotificationConfiguration.TopicArn)
				assert.Equal(t, "active", c.NotificationConfiguration.TopicStatus)
			},
		},
		{
			name:    "missing cluster name",
			input:   dax.CreateClusterInput{NodeType: "dax.r5.large", IamRoleArn: "arn:aws:iam::123456789012:role/r"},
			wantErr: true,
		},
		{
			name:    "missing node type",
			input:   dax.CreateClusterInput{ClusterName: "x", IamRoleArn: "arn:aws:iam::123456789012:role/r"},
			wantErr: true,
		},
		{
			name: "invalid node type",
			input: dax.CreateClusterInput{
				ClusterName: "x",
				NodeType:    "invalid.type",
				IamRoleArn:  "arn:aws:iam::123456789012:role/r",
			},
			wantErr: true,
		},
		{
			name:    "missing IAM role",
			input:   dax.CreateClusterInput{ClusterName: "x", NodeType: "dax.r5.large"},
			wantErr: true,
		},
		{
			name: "replication factor exceeds max",
			input: func() dax.CreateClusterInput {
				in := validCreateInput("big-cluster")
				in.ReplicationFactor = 11

				return in
			}(),
			wantErr: true,
		},
		{
			name: "unknown subnet group",
			input: func() dax.CreateClusterInput {
				in := validCreateInput("x")
				in.SubnetGroupName = "no-such-group"

				return in
			}(),
			wantErr: true,
		},
		{
			name: "unknown parameter group",
			input: func() dax.CreateClusterInput {
				in := validCreateInput("x")
				in.ParameterGroupName = "no-such-pg"

				return in
			}(),
			wantErr: true,
		},
		{
			name: "invalid encryption type",
			input: func() dax.CreateClusterInput {
				in := validCreateInput("x")
				in.ClusterEndpointEncryptionType = "INVALID"

				return in
			}(),
			wantErr: true,
		},
		{
			// If no explicit NetworkType is provided, the real API derives it from
			// the subnet group; gopherstack subnet groups are always IPv4-only.
			name:  "NetworkType defaults to ipv4",
			input: validCreateInput("network-default"),
			check: func(t *testing.T, c *dax.Cluster) {
				t.Helper()
				assert.Equal(t, dax.NetworkTypeIPv4, c.NetworkType)
			},
		},
		{
			name: "NetworkType explicit dual_stack accepted",
			input: func() dax.CreateClusterInput {
				in := validCreateInput("network-dual")
				in.NetworkType = dax.NetworkTypeDualStack

				return in
			}(),
			check: func(t *testing.T, c *dax.Cluster) {
				t.Helper()
				assert.Equal(t, dax.NetworkTypeDualStack, c.NetworkType)
			},
		},
		{
			name: "invalid NetworkType",
			input: func() dax.CreateClusterInput {
				in := validCreateInput("x")
				in.NetworkType = "ipv7"

				return in
			}(),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newTestBackend()
			c, err := b.CreateCluster(tt.input)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, c)
			}
		})
	}
}

func TestCreateCluster_DuplicateName(t *testing.T) {
	t.Parallel()
	b := newTestBackend()
	_, err := b.CreateCluster(validCreateInput("dup-cluster"))
	require.NoError(t, err)
	_, err = b.CreateCluster(validCreateInput("dup-cluster"))
	require.Error(t, err)
}

// ---- ClusterName format validation ----

func TestValidateClusterName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		errSentinel error
		name        string
		input       string
		wantErr     bool
	}{
		{name: "valid simple", input: "mycluster", wantErr: false},
		{name: "valid with hyphen", input: "my-cluster", wantErr: false},
		{name: "valid single letter", input: "a", wantErr: false},
		{name: "valid max length", input: strings.Repeat("a", 20), wantErr: false},
		{name: "empty", input: "", wantErr: true, errSentinel: dax.ErrInvalidParameterValue},
		{name: "too long", input: strings.Repeat("a", 21), wantErr: true, errSentinel: dax.ErrInvalidParameterValue},
		{name: "starts with digit", input: "1cluster", wantErr: true, errSentinel: dax.ErrInvalidParameterValue},
		{name: "starts with hyphen", input: "-cluster", wantErr: true, errSentinel: dax.ErrInvalidParameterValue},
		{name: "ends with hyphen", input: "cluster-", wantErr: true, errSentinel: dax.ErrInvalidParameterValue},
		{name: "consecutive hyphens", input: "my--cluster", wantErr: true, errSentinel: dax.ErrInvalidParameterValue},
		{
			name:        "invalid char underscore",
			input:       "my_cluster",
			wantErr:     true,
			errSentinel: dax.ErrInvalidParameterValue,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newTestBackend()
			_, err := b.CreateCluster(dax.CreateClusterInput{
				ClusterName:       tt.input,
				NodeType:          "dax.r5.large",
				IamRoleArn:        "arn:aws:iam::123456789012:role/DAXRole",
				ReplicationFactor: 1,
			})

			if tt.wantErr {
				require.Error(t, err)
				if tt.errSentinel != nil {
					require.ErrorIs(t, err, tt.errSentinel)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// ---- ReplicationFactor boundary validation ----

func TestCreateClusterReplicationFactorBounds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		errSentinel       error
		name              string
		replicationFactor int
		wantErr           bool
	}{
		{
			name:              "zero is rejected",
			replicationFactor: 0,
			wantErr:           true,
			errSentinel:       dax.ErrInvalidParameterCombination,
		},
		{
			name:              "negative is rejected",
			replicationFactor: -1,
			wantErr:           true,
			errSentinel:       dax.ErrInvalidParameterCombination,
		},
		{name: "one is valid", replicationFactor: 1, wantErr: false},
		{name: "ten is valid max", replicationFactor: 10, wantErr: false},
		{
			name:              "eleven exceeds max",
			replicationFactor: 11,
			wantErr:           true,
			errSentinel:       dax.ErrInvalidParameterCombination,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newTestBackend()
			_, err := b.CreateCluster(dax.CreateClusterInput{
				ClusterName:       "valid-name",
				NodeType:          "dax.r5.large",
				IamRoleArn:        "arn:aws:iam::123456789012:role/DAXRole",
				ReplicationFactor: tt.replicationFactor,
			})

			if tt.wantErr {
				require.Error(t, err)
				if tt.errSentinel != nil {
					require.ErrorIs(t, err, tt.errSentinel)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestCreateClusterAvailabilityZonesLengthMustMatchReplicationFactor covers
// api_op_CreateCluster.go's ReplicationFactor doc: "If the AvailabilityZones
// parameter is provided, its length must equal the ReplicationFactor.".
func TestCreateClusterAvailabilityZonesLengthMustMatchReplicationFactor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		availabilityZones []string
		replicationFactor int
		wantErr           bool
	}{
		{
			name:              "omitted is valid",
			replicationFactor: 3,
			wantErr:           false,
		},
		{
			name:              "matching length is valid",
			availabilityZones: []string{"us-east-1a", "us-east-1b", "us-east-1c"},
			replicationFactor: 3,
			wantErr:           false,
		},
		{
			name:              "fewer AZs than ReplicationFactor is rejected",
			availabilityZones: []string{"us-east-1a", "us-east-1b"},
			replicationFactor: 3,
			wantErr:           true,
		},
		{
			name:              "more AZs than ReplicationFactor is rejected",
			availabilityZones: []string{"us-east-1a", "us-east-1b", "us-east-1c", "us-east-1d"},
			replicationFactor: 3,
			wantErr:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newTestBackend()
			_, err := b.CreateCluster(dax.CreateClusterInput{
				ClusterName:       "valid-name",
				NodeType:          "dax.r5.large",
				IamRoleArn:        "arn:aws:iam::123456789012:role/DAXRole",
				ReplicationFactor: tt.replicationFactor,
				AvailabilityZones: tt.availabilityZones,
			})

			if tt.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, dax.ErrInvalidParameterCombination)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// ---- DescribeClusters ----

func TestDescribeClusters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setup     func(b *dax.InMemoryBackend)
		names     []string
		wantCount int
		wantErr   bool
	}{
		{
			name: "all clusters",
			setup: func(b *dax.InMemoryBackend) {
				_, _ = b.CreateCluster(validCreateInput("a"))
				_, _ = b.CreateCluster(validCreateInput("b"))
			},
			wantCount: 2,
		},
		{
			name: "filtered by name",
			setup: func(b *dax.InMemoryBackend) {
				_, _ = b.CreateCluster(validCreateInput("alpha"))
				_, _ = b.CreateCluster(validCreateInput("beta"))
			},
			names:     []string{"alpha"},
			wantCount: 1,
		},
		{
			name:    "not found",
			setup:   func(_ *dax.InMemoryBackend) {},
			names:   []string{"nonexistent"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newTestBackend()
			tt.setup(b)

			clusters, _, err := b.DescribeClusters(tt.names, 0, "")

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Len(t, clusters, tt.wantCount)
		})
	}
}

func TestDescribeClusters_Pagination(t *testing.T) {
	t.Parallel()
	b := newTestBackend()

	for _, name := range []string{"c1", "c2", "c3"} {
		_, err := b.CreateCluster(validCreateInput(name))
		require.NoError(t, err)
	}

	page1, nextToken, err := b.DescribeClusters(nil, 2, "")
	require.NoError(t, err)
	assert.Len(t, page1, 2)
	assert.NotEmpty(t, nextToken)

	page2, nextToken2, err := b.DescribeClusters(nil, 2, nextToken)
	require.NoError(t, err)
	assert.Len(t, page2, 1)
	assert.Empty(t, nextToken2)
}

// ---- UpdateCluster ----

func TestUpdateCluster(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup   func(b *dax.InMemoryBackend) string
		input   func(clusterName string) dax.UpdateClusterInput
		check   func(t *testing.T, c *dax.Cluster)
		name    string
		wantErr bool
	}{
		{
			name: "update description",
			setup: func(b *dax.InMemoryBackend) string {
				_, _ = b.CreateCluster(validCreateInput("upd"))

				return "upd"
			},
			input: func(name string) dax.UpdateClusterInput {
				return dax.UpdateClusterInput{ClusterName: name, Description: new("new description")}
			},
			check: func(t *testing.T, c *dax.Cluster) {
				t.Helper()
				assert.Equal(t, "new description", c.Description)
			},
		},
		{
			name: "clear description with empty string",
			setup: func(b *dax.InMemoryBackend) string {
				in := validCreateInput("desc-cluster")
				in.Description = "initial"
				_, _ = b.CreateCluster(in)

				return "desc-cluster"
			},
			input: func(name string) dax.UpdateClusterInput {
				return dax.UpdateClusterInput{ClusterName: name, Description: new("")}
			},
			check: func(t *testing.T, c *dax.Cluster) {
				t.Helper()
				assert.Empty(t, c.Description)
			},
		},
		{
			name: "nil description does not clear",
			setup: func(b *dax.InMemoryBackend) string {
				in := validCreateInput("keep-desc")
				in.Description = "kept"
				_, _ = b.CreateCluster(in)

				return "keep-desc"
			},
			input: func(name string) dax.UpdateClusterInput {
				return dax.UpdateClusterInput{ClusterName: name}
			},
			check: func(t *testing.T, c *dax.Cluster) {
				t.Helper()
				assert.Equal(t, "kept", c.Description)
			},
		},
		{
			name: "update notification topic",
			setup: func(b *dax.InMemoryBackend) string {
				_, _ = b.CreateCluster(validCreateInput("notif"))

				return "notif"
			},
			input: func(name string) dax.UpdateClusterInput {
				return dax.UpdateClusterInput{
					ClusterName:          name,
					NotificationTopicArn: "arn:aws:sns:us-east-1:123456789012:topic",
				}
			},
			check: func(t *testing.T, c *dax.Cluster) {
				t.Helper()
				require.NotNil(t, c.NotificationConfiguration)
				assert.Equal(t, "arn:aws:sns:us-east-1:123456789012:topic", c.NotificationConfiguration.TopicArn)
				assert.Equal(t, "active", c.NotificationConfiguration.TopicStatus)
			},
		},
		{
			name: "update notification topic status",
			setup: func(b *dax.InMemoryBackend) string {
				in := validCreateInput("topic-status")
				in.NotificationTopicArn = "arn:aws:sns:us-east-1:123456789012:topic"
				_, _ = b.CreateCluster(in)

				return "topic-status"
			},
			input: func(name string) dax.UpdateClusterInput {
				return dax.UpdateClusterInput{
					ClusterName:             name,
					NotificationTopicStatus: "inactive",
				}
			},
			check: func(t *testing.T, c *dax.Cluster) {
				t.Helper()
				require.NotNil(t, c.NotificationConfiguration)
				assert.Equal(t, "inactive", c.NotificationConfiguration.TopicStatus)
			},
		},
		{
			name:  "not found",
			setup: func(_ *dax.InMemoryBackend) string { return "no-such" },
			input: func(name string) dax.UpdateClusterInput {
				return dax.UpdateClusterInput{ClusterName: name}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newTestBackend()
			clusterName := tt.setup(b)

			c, err := b.UpdateCluster(tt.input(clusterName))

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, c)
			}
		})
	}
}

// TestClusterOpsRequiredFieldErrorCode verifies that a missing required field on cluster ops
// returns InvalidParameterValueException, not InvalidARNFault -- botocore's dax service-2.json
// (2017-04-19) declares InvalidARNFault only for TagResource/UntagResource/ListTags, never for
// these five cluster ops, all of which do declare InvalidParameterValueException.
func TestClusterOpsRequiredFieldErrorCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		op   func(b *dax.InMemoryBackend) error
		name string
	}{
		{
			name: "CreateCluster missing IamRoleArn",
			op: func(b *dax.InMemoryBackend) error {
				in := validCreateInput("missing-role")
				in.IamRoleArn = ""
				_, err := b.CreateCluster(in)

				return err
			},
		},
		{
			name: "UpdateCluster missing ClusterName",
			op: func(b *dax.InMemoryBackend) error {
				_, err := b.UpdateCluster(dax.UpdateClusterInput{})

				return err
			},
		},
		{
			name: "DeleteCluster missing ClusterName",
			op: func(b *dax.InMemoryBackend) error {
				_, err := b.DeleteCluster("")

				return err
			},
		},
		{
			name: "IncreaseReplicationFactor missing ClusterName",
			op: func(b *dax.InMemoryBackend) error {
				_, err := b.IncreaseReplicationFactor(dax.IncreaseReplicationFactorInput{NewReplicationFactor: 2})

				return err
			},
		},
		{
			name: "DecreaseReplicationFactor missing ClusterName",
			op: func(b *dax.InMemoryBackend) error {
				_, err := b.DecreaseReplicationFactor(dax.DecreaseReplicationFactorInput{NewReplicationFactor: 1})

				return err
			},
		},
		{
			name: "RebootNode missing ClusterName",
			op: func(b *dax.InMemoryBackend) error {
				_, err := b.RebootNode("", "node-0001")

				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newTestBackend()

			err := tt.op(b)

			require.Error(t, err)
			require.ErrorIsf(t, err, dax.ErrInvalidParameterValue,
				"want InvalidParameterValueException, got %v", err)
			assert.NotErrorIsf(t, err, dax.ErrInvalidARN,
				"InvalidARNFault is not a declared error for this operation, got %v", err)
		})
	}
}

// TestUpdateClusterRejectedRequestDoesNotMutateState verifies that a partially-invalid
// UpdateCluster request (valid PreferredMaintenanceWindow, unknown ParameterGroupName) leaves
// the cluster untouched -- AWS rejects the whole request atomically, not field-by-field.
func TestUpdateClusterRejectedRequestDoesNotMutateState(t *testing.T) {
	t.Parallel()
	b := newTestBackend()
	_, err := b.CreateCluster(validCreateInput("atomic-update"))
	require.NoError(t, err)

	_, err = b.UpdateCluster(dax.UpdateClusterInput{
		ClusterName:                "atomic-update",
		PreferredMaintenanceWindow: "mon:01:00-mon:02:00",
		SecurityGroupIDs:           []string{"sg-changed"},
		ParameterGroupName:         "no-such-pg",
	})
	require.Error(t, err)

	clusters, _, err := b.DescribeClusters([]string{"atomic-update"}, 0, "")
	require.NoError(t, err)
	require.Len(t, clusters, 1)
	assert.NotEqual(t, "mon:01:00-mon:02:00", clusters[0].PreferredMaintenanceWindow)
	assert.NotEqual(t, []string{"sg-changed"}, clusters[0].SecurityGroupIDs)
}

// ---- DeleteCluster ----

func TestDeleteCluster(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setup       func(b *dax.InMemoryBackend)
		clusterName string
		wantErr     bool
	}{
		{
			name: "success",
			setup: func(b *dax.InMemoryBackend) {
				_, _ = b.CreateCluster(validCreateInput("del"))
			},
			clusterName: "del",
		},
		{
			name:        "not found",
			setup:       func(_ *dax.InMemoryBackend) {},
			clusterName: "no-such",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newTestBackend()
			tt.setup(b)

			deleted, err := b.DeleteCluster(tt.clusterName)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.clusterName, deleted.ClusterName)
			assert.Equal(t, dax.StatusDeleting, deleted.Status)

			_, _, err = b.DescribeClusters([]string{tt.clusterName}, 0, "")
			require.Error(t, err)
		})
	}
}

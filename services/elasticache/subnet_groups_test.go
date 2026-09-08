package elasticache_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/elasticache"
)

func TestBackend_CreateSubnetGroup(t *testing.T) {
	t.Parallel()

	b := elasticache.NewInMemoryBackend(elasticache.EngineStub, "000000000000", "us-east-1", nil)

	sg, err := b.CreateSubnetGroup(context.Background(), "my-sng", "my subnet group", []string{"subnet-1", "subnet-2"})
	require.NoError(t, err)
	assert.Equal(t, "my-sng", sg.Name)
	assert.Len(t, sg.SubnetIDs, 2)
	assert.Contains(t, sg.ARN, "arn:aws:elasticache:")
}

func TestBackend_ModifySubnetGroup_AddSubnet(t *testing.T) {
	t.Parallel()

	b := elasticache.NewInMemoryBackend(elasticache.EngineStub, "000000000000", "us-east-1", nil)

	_, err := b.CreateSubnetGroup(context.Background(), "add-sng", "original", []string{"subnet-1"})
	require.NoError(t, err)

	sg, err := b.ModifySubnetGroup(
		context.Background(),
		"add-sng",
		"updated",
		[]string{"subnet-1", "subnet-2", "subnet-3"},
	)
	require.NoError(t, err)
	assert.Len(t, sg.SubnetIDs, 3)
	assert.Equal(t, "updated", sg.Description)
}

func TestBackend_DeleteSubnetGroup_NotFound(t *testing.T) {
	t.Parallel()

	b := elasticache.NewInMemoryBackend(elasticache.EngineStub, "000000000000", "us-east-1", nil)

	err := b.DeleteSubnetGroup(context.Background(), "nonexistent-sng")
	require.Error(t, err)
	assert.ErrorIs(t, err, elasticache.ErrSubnetGroupNotFound)
}

func TestBackend_DeleteSubnetGroup_InUseByCluster(t *testing.T) {
	t.Parallel()

	b := elasticache.NewInMemoryBackend(elasticache.EngineStub, "000000000000", "us-east-1", nil)
	ctx := context.Background()

	_, err := b.CreateSubnetGroup(ctx, "used-sng", "in use", []string{"subnet-1"})
	require.NoError(t, err)

	_, err = b.CreateCluster(ctx, "sng-client", "redis", "cache.t3.micro", 0)
	require.NoError(t, err)
	require.NoError(t, b.SetClusterSubnetGroupName(ctx, "sng-client", "used-sng"))

	err = b.DeleteSubnetGroup(ctx, "used-sng")
	require.ErrorIs(t, err, elasticache.ErrSubnetGroupInUse)

	require.NoError(t, b.DeleteCluster(ctx, "sng-client"))
	require.NoError(t, b.DeleteSubnetGroup(ctx, "used-sng"))
}

// ----------------------------------------
// Snapshot CRUD + ExportServerlessCacheSnapshot (issue)
// ----------------------------------------

func TestBackend_CreateSubnetGroupFull_WithVpcId(t *testing.T) {
	t.Parallel()

	b := elasticache.NewInMemoryBackend(elasticache.EngineStub, "000000000000", "us-east-1", nil)

	sg, err := b.CreateSubnetGroupFull(
		context.Background(),
		"sng-vpc",
		"with vpc",
		"vpc-0abc123",
		[]string{"subnet-1", "subnet-2"},
	)
	require.NoError(t, err)
	assert.Equal(t, "sng-vpc", sg.Name)
	assert.Equal(t, "vpc-0abc123", sg.VpcID)
	assert.Len(t, sg.SubnetIDs, 2)
	assert.NotEmpty(t, sg.ARN)
}

func TestBackend_CreateSubnetGroupFull_AlreadyExists(t *testing.T) {
	t.Parallel()

	b := elasticache.NewInMemoryBackend(elasticache.EngineStub, "000000000000", "us-east-1", nil)

	_, err := b.CreateSubnetGroupFull(context.Background(), "dup-sng", "dup", "vpc-111", nil)
	require.NoError(t, err)

	_, err = b.CreateSubnetGroupFull(context.Background(), "dup-sng", "dup", "vpc-111", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, elasticache.ErrSubnetGroupAlreadyExists)
}

package elasticache_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/elasticache"
)

func TestBackend_CreateUserGroup(t *testing.T) {
	t.Parallel()

	b := elasticache.NewInMemoryBackend(elasticache.EngineStub, "000000000000", "us-east-1", nil)

	_, err := b.CreateUser(context.Background(), "u-a", "u-a", "on ~* +@all", "redis", false)
	require.NoError(t, err)
	_, err = b.CreateUser(context.Background(), "u-b", "u-b", "on ~* +@all", "redis", false)
	require.NoError(t, err)

	ug, err := b.CreateUserGroup(context.Background(), "group-ab", "redis", []string{"u-a", "u-b"})
	require.NoError(t, err)
	assert.Equal(t, "group-ab", ug.UserGroupID)
	assert.ElementsMatch(t, []string{"u-a", "u-b"}, ug.UserIDs)
	assert.Contains(t, ug.ARN, "arn:aws:elasticache:")
}

func TestBackend_CreateUserGroup_AlreadyExists(t *testing.T) {
	t.Parallel()

	b := elasticache.NewInMemoryBackend(elasticache.EngineStub, "000000000000", "us-east-1", nil)

	_, err := b.CreateUserGroup(context.Background(), "dup-group", "redis", nil)
	require.NoError(t, err)

	_, err = b.CreateUserGroup(context.Background(), "dup-group", "redis", nil)
	require.Error(t, err)
}

func TestBackend_ModifyUserGroup_AddUsers(t *testing.T) {
	t.Parallel()

	b := elasticache.NewInMemoryBackend(elasticache.EngineStub, "000000000000", "us-east-1", nil)

	_, err := b.CreateUser(context.Background(), "gu-1", "gu-1", "on ~* +@all", "redis", false)
	require.NoError(t, err)
	_, err = b.CreateUser(context.Background(), "gu-2", "gu-2", "on ~* +@all", "redis", false)
	require.NoError(t, err)

	ug, err := b.CreateUserGroup(context.Background(), "mod-grp", "redis", []string{"gu-1"})
	require.NoError(t, err)
	assert.Len(t, ug.UserIDs, 1)

	modified, err := b.ModifyUserGroup(context.Background(), "mod-grp", []string{"gu-2"}, nil)
	require.NoError(t, err)
	assert.Contains(t, modified.UserIDs, "gu-1")
	assert.Contains(t, modified.UserIDs, "gu-2")
}

func TestBackend_ModifyUserGroup_RemoveUsers(t *testing.T) {
	t.Parallel()

	b := elasticache.NewInMemoryBackend(elasticache.EngineStub, "000000000000", "us-east-1", nil)

	_, err := b.CreateUser(context.Background(), "rm-u1", "rm-u1", "on ~* +@all", "redis", false)
	require.NoError(t, err)
	_, err = b.CreateUser(context.Background(), "rm-u2", "rm-u2", "on ~* +@all", "redis", false)
	require.NoError(t, err)

	_, err = b.CreateUserGroup(context.Background(), "remove-grp", "redis", []string{"rm-u1", "rm-u2"})
	require.NoError(t, err)

	modified, err := b.ModifyUserGroup(context.Background(), "remove-grp", nil, []string{"rm-u1"})
	require.NoError(t, err)
	assert.NotContains(t, modified.UserIDs, "rm-u1")
	assert.Contains(t, modified.UserIDs, "rm-u2")
}

func TestBackend_DescribeUserGroups_FilterByID(t *testing.T) {
	t.Parallel()

	b := elasticache.NewInMemoryBackend(elasticache.EngineStub, "000000000000", "us-east-1", nil)

	_, err := b.CreateUserGroup(context.Background(), "grp-x", "redis", nil)
	require.NoError(t, err)
	_, err = b.CreateUserGroup(context.Background(), "grp-y", "redis", nil)
	require.NoError(t, err)

	p, err := b.DescribeUserGroups(context.Background(), "grp-x", "", 0)
	require.NoError(t, err)
	require.Len(t, p.Data, 1)
	assert.Equal(t, "grp-x", p.Data[0].UserGroupID)
}

// ----------------------------------------
// GlobalReplicationGroup (issue #12)
// ----------------------------------------

func TestBackend_CreateUserGroupValidated_UsersExist(t *testing.T) {
	t.Parallel()

	b := elasticache.NewInMemoryBackend(elasticache.EngineStub, "000000000000", "us-east-1", nil)

	_, err := b.CreateUser(context.Background(), "u-valid-1", "u-valid-1", "on ~* +@all", "redis", true)
	require.NoError(t, err)
	_, err = b.CreateUser(context.Background(), "u-valid-2", "u-valid-2", "on ~* +@all", "redis", true)
	require.NoError(t, err)

	ug, err := b.CreateUserGroupValidated(
		context.Background(),
		"validated-ug",
		"redis",
		[]string{"u-valid-1", "u-valid-2"},
	)
	require.NoError(t, err)
	assert.Equal(t, "validated-ug", ug.UserGroupID)
	assert.Len(t, ug.UserIDs, 2)
}

func TestBackend_CreateUserGroupValidated_UserNotFound(t *testing.T) {
	t.Parallel()

	b := elasticache.NewInMemoryBackend(elasticache.EngineStub, "000000000000", "us-east-1", nil)

	_, err := b.CreateUserGroupValidated(context.Background(), "fail-ug", "redis", []string{"nonexistent-user"})
	require.Error(t, err)
	assert.ErrorIs(t, err, elasticache.ErrGroupUserNotFound)
}

func TestBackend_UserGroup_AssignedReplicationGroupIDs(t *testing.T) {
	t.Parallel()

	b := elasticache.NewInMemoryBackend(elasticache.EngineStub, "000000000000", "us-east-1", nil)

	_, err := b.CreateUser(context.Background(), "u1", "user1", "on ~* +@all", "redis", false)
	require.NoError(t, err)

	ug, err := b.CreateUserGroup(context.Background(), "ug1", "redis", []string{"u1"})
	require.NoError(t, err)

	assert.NotNil(t, ug)
	assert.Contains(t, ug.UserIDs, "u1")
	// AssignedReplicationGroupIDs starts empty (populated when bound to RG).
	assert.Empty(t, ug.AssignedReplicationGroupIDs)
}

// ----------------------------------------
// Persistence: new fields survive snapshot/restore
// ----------------------------------------

func TestBackend_UserGroupIds_AddRemove(t *testing.T) {
	t.Parallel()

	b := elasticache.NewInMemoryBackend(elasticache.EngineStub, "000000000000", "us-east-1", nil)

	_, err := b.CreateUser(context.Background(), "u1", "user1", "on ~* +@all", "redis", false)
	require.NoError(t, err)

	_, err = b.CreateUserGroup(context.Background(), "ug1", "redis", []string{"u1"})
	require.NoError(t, err)

	rg, err := b.CreateReplicationGroupFull(context.Background(), elasticache.ReplicationGroupCreateOpts{
		ID:           "ug-backend-rg",
		Description:  "user group backend test",
		UserGroupIDs: []string{"ug1"},
	})
	require.NoError(t, err)
	assert.Contains(t, rg.UserGroupIDs, "ug1")

	// Modify: remove ug1.
	rg2, err := b.ModifyReplicationGroupFull(
		context.Background(),
		"ug-backend-rg",
		elasticache.ReplicationGroupModifyOpts{
			UserGroupIDsToRemove: []string{"ug1"},
			ApplyImmediately:     true,
		},
	)
	require.NoError(t, err)
	assert.NotContains(t, rg2.UserGroupIDs, "ug1")
}

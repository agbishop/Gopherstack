package transfer_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/services/transfer"
)

func TestCreateUser(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		userName string
		homeDir  string
		role     string
		wantErr  bool
	}{
		{
			name:     "success",
			userName: "alice",
			homeDir:  "/alice",
			role:     "arn:aws:iam::123456789012:role/transfer-role",
		},
		{
			name:     "duplicate user",
			userName: "alice",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend(t)

			s, err := b.CreateServer(nil, nil)
			require.NoError(t, err)

			// Pre-seed alice so the duplicate case can trigger a conflict.
			if tt.wantErr {
				_, err = b.CreateUser(s.ServerID, "alice", "/alice", "", nil)
				require.NoError(t, err)
			}

			u, err := b.CreateUser(s.ServerID, tt.userName, tt.homeDir, tt.role, nil)

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, awserr.ErrConflict)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.userName, u.UserName)
			assert.Equal(t, tt.homeDir, u.HomeDir)
			assert.Equal(t, tt.role, u.Role)
		})
	}
}

func TestCreateUser_AlreadyExists(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)

	s, err := b.CreateServer(nil, nil)
	require.NoError(t, err)

	_, err = b.CreateUser(s.ServerID, "alice", "/alice", "arn:role", nil)
	require.NoError(t, err)

	_, err = b.CreateUser(s.ServerID, "alice", "/alice", "arn:role", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, awserr.ErrConflict)
}

func TestDescribeUser(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)

	s, err := b.CreateServer(nil, nil)
	require.NoError(t, err)

	_, err = b.CreateUser(s.ServerID, "bob", "/bob", "", nil)
	require.NoError(t, err)

	u, err := b.DescribeUser(s.ServerID, "bob")
	require.NoError(t, err)
	assert.Equal(t, "bob", u.UserName)
	assert.Equal(t, "/bob", u.HomeDir)
}

func TestDescribeUser_NotFound(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)

	s, err := b.CreateServer(nil, nil)
	require.NoError(t, err)

	_, err = b.DescribeUser(s.ServerID, "nobody")
	require.Error(t, err)
	assert.ErrorIs(t, err, awserr.ErrNotFound)
}

func TestListUsers(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)

	s, err := b.CreateServer(nil, nil)
	require.NoError(t, err)

	_, err = b.CreateUser(s.ServerID, "alice", "/alice", "", nil)
	require.NoError(t, err)

	_, err = b.CreateUser(s.ServerID, "bob", "/bob", "", nil)
	require.NoError(t, err)

	users, err := b.ListUsers(s.ServerID)
	require.NoError(t, err)
	assert.Len(t, users, 2)
	// Sorted by username
	assert.Equal(t, "alice", users[0].UserName)
	assert.Equal(t, "bob", users[1].UserName)
}

func TestDeleteUser(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)

	s, err := b.CreateServer(nil, nil)
	require.NoError(t, err)

	_, err = b.CreateUser(s.ServerID, "alice", "/alice", "", nil)
	require.NoError(t, err)

	require.NoError(t, b.DeleteUser(s.ServerID, "alice"))

	_, err = b.DescribeUser(s.ServerID, "alice")
	require.Error(t, err)
}

func TestDeleteUser_ClearsTagsOnRecreate(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)

	s, err := b.CreateServer(nil, nil)
	require.NoError(t, err)

	u, err := b.CreateUser(s.ServerID, "alice", "/alice", "", map[string]string{"env": "prod"})
	require.NoError(t, err)

	userArn := arn.Build("transfer", u.Region, u.AccountID, "user/"+s.ServerID+"/alice")
	require.Equal(t, map[string]string{"env": "prod"}, b.ListTagsForResource(userArn))

	require.NoError(t, b.DeleteUser(s.ServerID, "alice"))

	_, err = b.CreateUser(s.ServerID, "alice", "/alice", "", nil)
	require.NoError(t, err)

	assert.Empty(t, b.ListTagsForResource(userArn))
}

func TestDeleteUser_LeavesOtherUserTagsIntact(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)

	s, err := b.CreateServer(nil, nil)
	require.NoError(t, err)

	alice, err := b.CreateUser(s.ServerID, "alice", "/alice", "", map[string]string{"env": "prod"})
	require.NoError(t, err)
	bob, err := b.CreateUser(s.ServerID, "bob", "/bob", "", map[string]string{"env": "dev"})
	require.NoError(t, err)

	aliceArn := arn.Build("transfer", alice.Region, alice.AccountID, "user/"+s.ServerID+"/alice")
	bobArn := arn.Build("transfer", bob.Region, bob.AccountID, "user/"+s.ServerID+"/bob")

	require.NoError(t, b.DeleteUser(s.ServerID, "alice"))

	assert.Empty(t, b.ListTagsForResource(aliceArn))
	assert.Equal(t, map[string]string{"env": "dev"}, b.ListTagsForResource(bobArn))
}

func TestUpdateUser(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)

	s, err := b.CreateServer(nil, nil)
	require.NoError(t, err)

	_, err = b.CreateUser(s.ServerID, "alice", "/alice", "", nil)
	require.NoError(t, err)

	updated, err := b.UpdateUser(s.ServerID, "alice", "/home/alice", "arn:role")
	require.NoError(t, err)
	assert.Equal(t, "/home/alice", updated.HomeDir)
	assert.Equal(t, "arn:role", updated.Role)
}

func TestListUsers_ServerNotFound(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)

	_, err := b.ListUsers("s-doesnotexist")
	require.Error(t, err)
	assert.ErrorIs(t, err, awserr.ErrNotFound)
}

// TestUserCountExport verifies UserCount export.
func TestUserCountExport(t *testing.T) {
	t.Parallel()

	b := transfer.NewInMemoryBackend(t.Context(), "000000000000", "us-east-1")
	s, err := b.CreateServer(nil, nil)
	require.NoError(t, err)

	assert.Equal(t, 0, transfer.UserCount(b))

	_, err = b.CreateUser(s.ServerID, "alice", "/alice", "", nil)
	require.NoError(t, err)

	assert.Equal(t, 1, transfer.UserCount(b))
}

// TestDeleteUserAlsoDeletesSSHKeys verifies that deleting a user
// removes all of that user's SSH public keys.
func TestDeleteUserAlsoDeletesSSHKeys(t *testing.T) {
	t.Parallel()

	b := transfer.NewInMemoryBackend(t.Context(), "000000000000", "us-east-1")
	s, err := b.CreateServer(nil, nil)
	require.NoError(t, err)

	_, err = b.CreateUser(s.ServerID, "alice", "/alice", "", nil)
	require.NoError(t, err)

	_, err = b.ImportSSHPublicKey(
		s.ServerID, "alice",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl test@example",
	)
	require.NoError(t, err)
	assert.Equal(t, 1, transfer.SSHPublicKeyCount(b))

	require.NoError(t, b.DeleteUser(s.ServerID, "alice"))
	assert.Equal(t, 0, transfer.SSHPublicKeyCount(b))
}

// TestCreateUserOnNonexistentServer verifies ResourceNotFoundException.
func TestCreateUserOnNonexistentServer(t *testing.T) {
	t.Parallel()

	b := transfer.NewInMemoryBackend(t.Context(), "000000000000", "us-east-1")
	_, err := b.CreateUser("s-doesnotexist", "alice", "/alice", "", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, awserr.ErrNotFound)
}

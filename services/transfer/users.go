package transfer

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"time"
)

// CreateUserInput holds all optional fields for CreateUser.
type CreateUserInput struct {
	PosixProfile          *PosixProfile
	Tags                  map[string]string
	ServerID              string
	UserName              string
	HomeDir               string
	Role                  string
	HomeDirectoryType     string
	Policy                string
	HomeDirectoryMappings []HomeDirectoryMapEntry
}

// CreateUser creates a user on the given server.
func (b *InMemoryBackend) CreateUser(
	serverID, userName, homeDir, role string,
	tags map[string]string,
) (*User, error) {
	return b.CreateUserFull(&CreateUserInput{
		ServerID: serverID,
		UserName: userName,
		HomeDir:  homeDir,
		Role:     role,
		Tags:     tags,
	})
}

// CreateUserFull creates a user on the given server with full configuration.
func (b *InMemoryBackend) CreateUserFull(in *CreateUserInput) (*User, error) {
	b.mu.Lock("CreateUser")
	defer b.mu.Unlock()

	if !b.servers.Has(in.ServerID) {
		return nil, fmt.Errorf("%w: server %s not found", ErrServerNotFound, in.ServerID)
	}

	if b.users.Has(userKey(in.ServerID, in.UserName)) {
		return nil, fmt.Errorf(
			"%w: user %s already exists on server %s",
			ErrUserAlreadyExists,
			in.UserName,
			in.ServerID,
		)
	}

	// Validate HomeDirectoryType.
	homeDirectoryType := in.HomeDirectoryType
	if homeDirectoryType == "" {
		homeDirectoryType = homeDirectoryTypePath
	} else {
		switch homeDirectoryType {
		case homeDirectoryTypePath, homeDirectoryTypeLogical:
			// valid
		default:
			return nil, fmt.Errorf(
				"%w: HomeDirectoryType must be PATH or LOGICAL, got %q",
				ErrValidation,
				homeDirectoryType,
			)
		}
	}

	merged := make(map[string]string, len(in.Tags))
	maps.Copy(merged, in.Tags)

	u := &User{
		UserName:              in.UserName,
		ServerID:              in.ServerID,
		HomeDir:               in.HomeDir,
		Role:                  in.Role,
		HomeDirectoryType:     homeDirectoryType,
		HomeDirectoryMappings: in.HomeDirectoryMappings,
		Policy:                in.Policy,
		PosixProfile:          in.PosixProfile,
		CreatedAt:             time.Now(),
		Tags:                  merged,
		AccountID:             b.accountID,
		Region:                b.region,
	}
	b.users.Put(u)
	b.initTagsStore(userARN(b.accountID, b.region, in.ServerID, in.UserName), merged)

	return cloneUser(u), nil
}

// DescribeUser returns the user with the given name on the given server.
func (b *InMemoryBackend) DescribeUser(serverID, userName string) (*User, error) {
	b.mu.RLock("DescribeUser")
	defer b.mu.RUnlock()

	if !b.servers.Has(serverID) {
		return nil, fmt.Errorf("%w: server %s not found", ErrServerNotFound, serverID)
	}

	u, ok := b.users.Get(userKey(serverID, userName))
	if !ok {
		return nil, fmt.Errorf(
			"%w: user %s not found on server %s",
			ErrUserNotFound,
			userName,
			serverID,
		)
	}

	return cloneUser(u), nil
}

// ListUsers returns all users on a server sorted by username.
func (b *InMemoryBackend) ListUsers(serverID string) ([]User, error) {
	b.mu.RLock("ListUsers")
	defer b.mu.RUnlock()

	if !b.servers.Has(serverID) {
		return nil, fmt.Errorf("%w: server %s not found", ErrServerNotFound, serverID)
	}

	users := b.usersByServer.Get(serverID)
	out := make([]User, 0, len(users))

	for _, u := range users {
		out = append(out, *cloneUser(u))
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].UserName < out[j].UserName
	})

	return out, nil
}

// DeleteUser removes a user from the given server and also deletes all SSH public keys for that user.
func (b *InMemoryBackend) DeleteUser(serverID, userName string) error {
	b.mu.Lock("DeleteUser")
	defer b.mu.Unlock()

	if !b.servers.Has(serverID) {
		return fmt.Errorf("%w: server %s not found", ErrServerNotFound, serverID)
	}

	uKey := userKey(serverID, userName)
	if !b.users.Has(uKey) {
		return fmt.Errorf("%w: user %s not found on server %s", ErrUserNotFound, userName, serverID)
	}

	b.users.Delete(uKey)
	delete(b.tagsStore, userARN(b.accountID, b.region, serverID, userName))

	// Delete all SSH public keys for this user.
	for _, k := range slices.Clone(b.sshKeysByServerUser.Get(serverUserKey(serverID, userName))) {
		b.sshPublicKeys.Delete(sshPublicKeyKey(k.ServerID, k.UserName, k.SSHPublicKeyID))
	}

	if serverBodies, exists := b.sshKeyBodies[serverID]; exists {
		delete(serverBodies, userName)
	}

	return nil
}

// UpdateUserInput holds all optional fields for UpdateUser.
type UpdateUserInput struct {
	PosixProfile             *PosixProfile
	ServerID                 string
	UserName                 string
	HomeDir                  string
	Role                     string
	HomeDirectoryType        string
	Policy                   string
	HomeDirectoryMappings    []HomeDirectoryMapEntry
	SetPosixProfile          bool
	SetHomeDirectoryMappings bool
	SetPolicy                bool
	SetHomeDirectoryType     bool
}

// UpdateUser updates mutable fields on a user.
func (b *InMemoryBackend) UpdateUser(serverID, userName, homeDir, role string) (*User, error) {
	return b.UpdateUserFull(&UpdateUserInput{
		ServerID: serverID,
		UserName: userName,
		HomeDir:  homeDir,
		Role:     role,
	})
}

// UpdateUserFull updates all mutable fields on a user.
func (b *InMemoryBackend) UpdateUserFull(in *UpdateUserInput) (*User, error) {
	b.mu.Lock("UpdateUser")
	defer b.mu.Unlock()

	if !b.servers.Has(in.ServerID) {
		return nil, fmt.Errorf("%w: server %s not found", ErrServerNotFound, in.ServerID)
	}

	u, ok := b.users.Get(userKey(in.ServerID, in.UserName))
	if !ok {
		return nil, fmt.Errorf(
			"%w: user %s not found on server %s",
			ErrUserNotFound,
			in.UserName,
			in.ServerID,
		)
	}

	if in.HomeDir != "" {
		u.HomeDir = in.HomeDir
	}

	if in.Role != "" {
		u.Role = in.Role
	}

	if in.SetPosixProfile {
		u.PosixProfile = in.PosixProfile
	}

	if in.SetHomeDirectoryMappings {
		u.HomeDirectoryMappings = in.HomeDirectoryMappings
	}

	if in.SetPolicy {
		u.Policy = in.Policy
	}

	if in.SetHomeDirectoryType && in.HomeDirectoryType != "" {
		switch in.HomeDirectoryType {
		case homeDirectoryTypePath, homeDirectoryTypeLogical:
			u.HomeDirectoryType = in.HomeDirectoryType
		default:
			return nil, fmt.Errorf(
				"%w: HomeDirectoryType must be PATH or LOGICAL, got %q",
				ErrValidation,
				in.HomeDirectoryType,
			)
		}
	}

	return cloneUser(u), nil
}

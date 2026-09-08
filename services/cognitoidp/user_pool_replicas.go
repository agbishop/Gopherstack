package cognitoidp

import (
	"fmt"
	"maps"
	"sort"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// Replica role/status wire values (types.ReplicaRoleType / types.ReplicaStatusType).
const (
	replicaRoleSecondary = "SECONDARY"

	replicaStatusInactive = "INACTIVE"
	replicaStatusActive   = "ACTIVE"
	replicaStatusDeleting = "DELETING"
)

// UserPoolReplica represents a multi-Region secondary replica of a user pool
// (Amazon Cognito's Multi-Region replication / MRR feature). Every replica
// this backend creates has Role SECONDARY -- the primary pool is the UserPool
// record itself and is never represented as a UserPoolReplica.
type UserPoolReplica struct {
	UserPoolID string `json:"userPoolId,omitempty"`
	RegionName string `json:"regionName,omitempty"`
	Role       string `json:"role,omitempty"`
	Status     string `json:"status,omitempty"`
	ARN        string `json:"arn,omitempty"`
}

// CreateUserPoolReplica creates a secondary replica of userPoolID in
// regionName, optionally tagging the replica (tags are tracked independently
// of the primary pool's tags, matching "You can maintain tags independently
// on replica user pools" -- stored under the replica's own ARN via the
// existing resourceTags mechanism).
//
// Real AWS constraints enforced here (Cognito multi-Region replication
// developer guide): the replica Region must differ from the pool's own
// (primary) Region, and a user pool can have at most one secondary replica --
// "You can have at most one secondary replica in an additional Region per
// user directory." New replicas start INACTIVE ("New secondary user pools
// start in the INACTIVE state[.] Review and configure regional settings
// before activating the user pool for production use.") -- callers activate
// with UpdateUserPoolReplica(Status: ACTIVE).
func (b *InMemoryBackend) CreateUserPoolReplica(
	userPoolID, regionName string,
	tags map[string]string,
) (*UserPoolReplica, error) {
	b.mu.Lock("CreateUserPoolReplica")
	defer b.mu.Unlock()

	if _, ok := b.pools.Get(userPoolID); !ok {
		return nil, fmt.Errorf("%w: pool %q not found", ErrUserPoolNotFound, userPoolID)
	}

	if regionName == "" {
		return nil, fmt.Errorf("%w: RegionName is required", ErrInvalidParameter)
	}

	if regionName == b.region {
		return nil, fmt.Errorf(
			"%w: replica Region %q must differ from the primary user pool's Region %q",
			ErrInvalidParameter, regionName, b.region,
		)
	}

	if existing := b.userPoolReplicasByPool.Get(userPoolID); len(existing) > 0 {
		return nil, fmt.Errorf(
			"%w: user pool %q already has a secondary replica (at most one is allowed per user directory)",
			ErrInvalidParameter, userPoolID,
		)
	}

	replicaARN := arn.Build("cognito-idp", regionName, b.accountID, fmt.Sprintf("userpool/%s", userPoolID))

	replica := &UserPoolReplica{
		UserPoolID: userPoolID,
		RegionName: regionName,
		Role:       replicaRoleSecondary,
		Status:     replicaStatusInactive,
		ARN:        replicaARN,
	}
	b.userPoolReplicas.Put(replica)

	if len(tags) > 0 {
		if b.resourceTags[replicaARN] == nil {
			b.resourceTags[replicaARN] = make(map[string]string)
		}

		maps.Copy(b.resourceTags[replicaARN], tags)
	}

	cp := *replica

	return &cp, nil
}

// DeleteUserPoolReplica deletes the secondary replica of userPoolID in
// regionName, returning the replica's final state with Status transitioned to
// DELETING, mirroring the real (asynchronous) deletion the API documents.
func (b *InMemoryBackend) DeleteUserPoolReplica(userPoolID, regionName string) (*UserPoolReplica, error) {
	b.mu.Lock("DeleteUserPoolReplica")
	defer b.mu.Unlock()

	if _, ok := b.pools.Get(userPoolID); !ok {
		return nil, fmt.Errorf("%w: pool %q not found", ErrUserPoolNotFound, userPoolID)
	}

	replica, ok := b.userPoolReplicas.Get(replicaKey(userPoolID, regionName))
	if !ok {
		return nil, fmt.Errorf(
			"%w: replica for pool %q in Region %q not found", ErrReplicaNotFound, userPoolID, regionName,
		)
	}

	cp := *replica
	cp.Status = replicaStatusDeleting

	b.userPoolReplicas.Delete(replicaKey(userPoolID, regionName))
	delete(b.resourceTags, replica.ARN)

	return &cp, nil
}

// UpdateUserPoolReplica sets the Status (ACTIVE or INACTIVE) of an existing
// replica -- the only field UpdateUserPoolReplica allows changing.
func (b *InMemoryBackend) UpdateUserPoolReplica(userPoolID, regionName, status string) (*UserPoolReplica, error) {
	b.mu.Lock("UpdateUserPoolReplica")
	defer b.mu.Unlock()

	if _, ok := b.pools.Get(userPoolID); !ok {
		return nil, fmt.Errorf("%w: pool %q not found", ErrUserPoolNotFound, userPoolID)
	}

	if status != replicaStatusActive && status != replicaStatusInactive {
		return nil, fmt.Errorf("%w: Status must be ACTIVE or INACTIVE, got %q", ErrInvalidParameter, status)
	}

	replica, ok := b.userPoolReplicas.Get(replicaKey(userPoolID, regionName))
	if !ok {
		return nil, fmt.Errorf(
			"%w: replica for pool %q in Region %q not found", ErrReplicaNotFound, userPoolID, regionName,
		)
	}

	replica.Status = status

	cp := *replica

	return &cp, nil
}

// ListUserPoolReplicas returns every secondary replica of userPoolID, sorted
// by Region for deterministic output. Real AWS caps a user directory at one
// secondary replica (see CreateUserPoolReplica's doc comment), so this never
// has more than one item to page over in practice; NextToken is accepted on
// the wire for shape compatibility but this backend never returns one.
func (b *InMemoryBackend) ListUserPoolReplicas(userPoolID string) ([]*UserPoolReplica, error) {
	b.mu.RLock("ListUserPoolReplicas")
	defer b.mu.RUnlock()

	if _, ok := b.pools.Get(userPoolID); !ok {
		return nil, fmt.Errorf("%w: pool %q not found", ErrUserPoolNotFound, userPoolID)
	}

	replicas := b.userPoolReplicasByPool.Get(userPoolID)
	out := make([]*UserPoolReplica, 0, len(replicas))

	for _, r := range replicas {
		cp := *r
		out = append(out, &cp)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].RegionName < out[j].RegionName })

	return out, nil
}

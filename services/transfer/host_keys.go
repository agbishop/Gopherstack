package transfer

import (
	"fmt"
	"maps"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ImportHostKey imports a host key onto a server.
func (b *InMemoryBackend) ImportHostKey(
	serverID, hostKeyBody, description string,
	tags map[string]string,
) (*HostKey, error) {
	b.mu.Lock("ImportHostKey")
	defer b.mu.Unlock()

	if !b.servers.Has(serverID) {
		return nil, fmt.Errorf("%w: server %s not found", ErrServerNotFound, serverID)
	}

	hostKeyID := "hostkey-" + uuid.NewString()[:8]

	merged := make(map[string]string, len(tags))
	maps.Copy(merged, tags)

	fp, _ := computeSSHKeyFingerprintAndType(hostKeyBody)

	hk := &HostKey{
		HostKeyID:   hostKeyID,
		ServerID:    serverID,
		Description: description,
		Fingerprint: fp,
		Value:       hostKeyBody,
		Type:        detectHostKeyType(hostKeyBody),
		CreatedAt:   time.Now(),
		Tags:        merged,
		AccountID:   b.accountID,
		Region:      b.region,
	}
	b.hostKeys.Put(hk)
	b.initTagsStore(hostKeyARN(b.accountID, b.region, serverID, hostKeyID), merged)

	return cloneHostKey(hk), nil
}

// DeleteHostKey removes a host key from a server.
func (b *InMemoryBackend) DeleteHostKey(serverID, hostKeyID string) error {
	b.mu.Lock("DeleteHostKey")
	defer b.mu.Unlock()

	key := hostKeyKey(serverID, hostKeyID)
	if !b.hostKeys.Has(key) {
		return fmt.Errorf(
			"%w: host key %s not found on server %s",
			ErrHostKeyNotFound,
			hostKeyID,
			serverID,
		)
	}

	b.hostKeys.Delete(key)
	delete(b.tagsStore, hostKeyARN(b.accountID, b.region, serverID, hostKeyID))

	return nil
}

// DescribeHostKey returns a host key from a server.
func (b *InMemoryBackend) DescribeHostKey(serverID, hostKeyID string) (*HostKey, error) {
	b.mu.RLock("DescribeHostKey")
	defer b.mu.RUnlock()

	hk, ok := b.hostKeys.Get(hostKeyKey(serverID, hostKeyID))
	if !ok {
		return nil, fmt.Errorf(
			"%w: host key %s not found on server %s",
			ErrHostKeyNotFound,
			hostKeyID,
			serverID,
		)
	}

	return cloneHostKey(hk), nil
}

// ListHostKeys returns all host keys on a server sorted by hostKeyID.
func (b *InMemoryBackend) ListHostKeys(serverID string) ([]*HostKey, error) {
	b.mu.RLock("ListHostKeys")
	defer b.mu.RUnlock()

	if !b.servers.Has(serverID) {
		return nil, fmt.Errorf("%w: server %s not found", ErrServerNotFound, serverID)
	}

	serverKeys := b.hostKeysByServer.Get(serverID)
	out := make([]*HostKey, 0, len(serverKeys))

	for _, hk := range serverKeys {
		out = append(out, cloneHostKey(hk))
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].HostKeyID < out[j].HostKeyID
	})

	return out, nil
}

// UpdateHostKey updates mutable fields on a host key.
func (b *InMemoryBackend) UpdateHostKey(serverID, hostKeyID, description string) (*HostKey, error) {
	b.mu.Lock("UpdateHostKey")
	defer b.mu.Unlock()

	hk, ok := b.hostKeys.Get(hostKeyKey(serverID, hostKeyID))
	if !ok {
		return nil, fmt.Errorf(
			"%w: host key %s not found on server %s",
			ErrHostKeyNotFound,
			hostKeyID,
			serverID,
		)
	}

	if description != "" {
		hk.Description = description
	}

	return cloneHostKey(hk), nil
}

// detectHostKeyType returns the host key type string from the key body prefix.
func detectHostKeyType(hostKeyBody string) string {
	prefix := strings.Fields(hostKeyBody)
	if len(prefix) == 0 {
		return defaultHostKeyType
	}

	switch prefix[0] {
	case defaultHostKeyType:
		return defaultHostKeyType
	case sshKeyTypeECDSAP256, sshKeyTypeECDSAP384, sshKeyTypeECDSAP521:
		return prefix[0]
	case sshKeyTypeEd25519:
		return sshKeyTypeEd25519
	default:
		return defaultHostKeyType
	}
}

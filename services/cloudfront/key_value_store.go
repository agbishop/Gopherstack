package cloudfront

import (
	"fmt"
	"maps"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// keyValueStoreARN builds an ARN for a Key Value Store.
func (b *InMemoryBackend) keyValueStoreARN(id string) string {
	return arn.Build("cloudfront", "", b.accountID, fmt.Sprintf("key-value-store/%s", id))
}

// CreateKeyValueStore creates a new CloudFront Key Value Store.
func (b *InMemoryBackend) CreateKeyValueStore(name, comment string, tags map[string]string) (*KeyValueStore, error) {
	b.mu.Lock("CreateKeyValueStore")
	defer b.mu.Unlock()

	if name == "" {
		return nil, fmt.Errorf("%w: Name must not be empty", ErrValidation)
	}

	if _, exists := b.keyValueStoreByName[name]; exists {
		return nil, fmt.Errorf(
			"%w: key value store with name %q already exists",
			ErrAlreadyExists,
			name,
		)
	}

	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339)
	kvs := &KeyValueStore{
		ID:               id,
		ARN:              b.keyValueStoreARN(id),
		Name:             name,
		Comment:          comment,
		ETag:             uuid.NewString(),
		Status:           kvsStatusReady,
		LastModifiedTime: now,
		CreatedTime:      now,
	}
	if len(tags) > 0 {
		kvs.Tags = maps.Clone(tags)
	}
	b.keyValueStores.Put(kvs)
	b.keyValueStoreByName[name] = id
	cp := *kvs

	return &cp, nil
}

// GetKeyValueStore returns a Key Value Store by ID or ARN.
func (b *InMemoryBackend) GetKeyValueStore(idOrARN string) (*KeyValueStore, error) {
	b.mu.RLock("GetKeyValueStore")
	defer b.mu.RUnlock()

	if kvs, ok := b.keyValueStores.Get(idOrARN); ok {
		cp := *kvs

		return &cp, nil
	}

	for _, kvs := range b.keyValueStores.All() {
		if kvs.ARN == idOrARN {
			cp := *kvs

			return &cp, nil
		}
	}

	return nil, fmt.Errorf("%w: key value store %s not found", ErrKeyValueStoreNotFound, idOrARN)
}

// ListKeyValueStores returns all Key Value Stores sorted by name.
func (b *InMemoryBackend) ListKeyValueStores() []*KeyValueStore {
	b.mu.RLock("ListKeyValueStores")
	defer b.mu.RUnlock()

	list := make([]*KeyValueStore, 0, b.keyValueStores.Len())
	for _, kvs := range b.keyValueStores.All() {
		cp := *kvs
		list = append(list, &cp)
	}

	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })

	return list
}

// DeleteKeyValueStore deletes a Key Value Store by ID.
func (b *InMemoryBackend) DeleteKeyValueStore(id string) error {
	b.mu.Lock("DeleteKeyValueStore")
	defer b.mu.Unlock()

	kvs, ok := b.keyValueStores.Get(id)
	if !ok {
		return fmt.Errorf("%w: key value store %s not found", ErrKeyValueStoreNotFound, id)
	}

	delete(b.keyValueStoreByName, kvs.Name)
	b.keyValueStores.Delete(id)
	delete(b.keyValueStoreData, kvs.ID)
	delete(b.keyValueDataETags, kvs.ID)

	return nil
}

// --- KVS Data Plane ---

// kvsDataETag returns or creates the ETag for a KVS data store.
func (b *InMemoryBackend) kvsDataETag(id string) string {
	if etag, ok := b.keyValueDataETags[id]; ok {
		return etag
	}

	etag := uuid.NewString()
	b.keyValueDataETags[id] = etag

	return etag
}

// GetKVSValue returns the value for a key in a Key Value Store.
func (b *InMemoryBackend) GetKVSValue(kvsID, key string) (string, string, error) {
	b.mu.RLock("GetKVSValue")
	defer b.mu.RUnlock()

	if _, ok := b.keyValueStores.Get(kvsID); !ok {
		return "", "", fmt.Errorf("%w: key value store %s not found", ErrKeyValueStoreNotFound, kvsID)
	}

	data := b.keyValueStoreData[kvsID]
	val, ok := data[key]
	if !ok {
		return "", "", fmt.Errorf("%w: key %q not found in kvs %s", ErrNotFound, key, kvsID)
	}

	return val, b.keyValueDataETags[kvsID], nil
}

// PutKVSValue creates or updates a key/value pair in a Key Value Store.
func (b *InMemoryBackend) PutKVSValue(kvsID, key, value, ifMatch string) (string, error) {
	b.mu.Lock("PutKVSValue")
	defer b.mu.Unlock()

	if _, ok := b.keyValueStores.Get(kvsID); !ok {
		return "", fmt.Errorf("%w: key value store %s not found", ErrKeyValueStoreNotFound, kvsID)
	}

	currentETag := b.kvsDataETag(kvsID)
	if ifMatch != "" && ifMatch != currentETag {
		return "", fmt.Errorf("%w: If-Match ETag mismatch", ErrPreconditionFailed)
	}

	if b.keyValueStoreData[kvsID] == nil {
		b.keyValueStoreData[kvsID] = make(map[string]string)
	}
	b.keyValueStoreData[kvsID][key] = value
	newETag := uuid.NewString()
	b.keyValueDataETags[kvsID] = newETag

	return newETag, nil
}

// DeleteKVSValue deletes a key from a Key Value Store.
func (b *InMemoryBackend) DeleteKVSValue(kvsID, key, ifMatch string) (string, error) {
	b.mu.Lock("DeleteKVSValue")
	defer b.mu.Unlock()

	if _, ok := b.keyValueStores.Get(kvsID); !ok {
		return "", fmt.Errorf("%w: key value store %s not found", ErrKeyValueStoreNotFound, kvsID)
	}

	currentETag := b.kvsDataETag(kvsID)
	if ifMatch != "" && ifMatch != currentETag {
		return "", fmt.Errorf("%w: If-Match ETag mismatch", ErrPreconditionFailed)
	}

	if b.keyValueStoreData[kvsID] != nil {
		delete(b.keyValueStoreData[kvsID], key)
	}
	newETag := uuid.NewString()
	b.keyValueDataETags[kvsID] = newETag

	return newETag, nil
}

// ListKVSValues returns all key/value pairs in a Key Value Store.
func (b *InMemoryBackend) ListKVSValues(kvsID string) ([]*KVSItem, string, error) {
	b.mu.RLock("ListKVSValues")
	defer b.mu.RUnlock()

	if _, ok := b.keyValueStores.Get(kvsID); !ok {
		return nil, "", fmt.Errorf("%w: key value store %s not found", ErrKeyValueStoreNotFound, kvsID)
	}

	data := b.keyValueStoreData[kvsID]
	items := make([]*KVSItem, 0, len(data))
	for k, v := range data {
		items = append(items, &KVSItem{Key: k, Value: v})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Key < items[j].Key })

	return items, b.kvsDataETag(kvsID), nil
}

// UpdateKVSValues performs a batch put/delete on a Key Value Store.
func (b *InMemoryBackend) UpdateKVSValues(kvsID, ifMatch string, puts []*KVSItem, deletes []string) (string, error) {
	b.mu.Lock("UpdateKVSValues")
	defer b.mu.Unlock()

	if _, ok := b.keyValueStores.Get(kvsID); !ok {
		return "", fmt.Errorf("%w: key value store %s not found", ErrKeyValueStoreNotFound, kvsID)
	}

	currentETag := b.kvsDataETag(kvsID)
	if ifMatch != "" && ifMatch != currentETag {
		return "", fmt.Errorf("%w: If-Match ETag mismatch", ErrPreconditionFailed)
	}

	if b.keyValueStoreData[kvsID] == nil {
		b.keyValueStoreData[kvsID] = make(map[string]string)
	}
	for _, item := range puts {
		b.keyValueStoreData[kvsID][item.Key] = item.Value
	}
	for _, key := range deletes {
		delete(b.keyValueStoreData[kvsID], key)
	}
	newETag := uuid.NewString()
	b.keyValueDataETags[kvsID] = newETag

	return newETag, nil
}

// --- VPC Origin CRUD ---

// UpdateKeyValueStore updates a Key Value Store's comment.
func (b *InMemoryBackend) UpdateKeyValueStore(id, comment string) (*KeyValueStore, error) {
	b.mu.Lock("UpdateKeyValueStore")
	defer b.mu.Unlock()

	kvs, ok := b.keyValueStores.Get(id)
	if !ok {
		return nil, fmt.Errorf("%w: key value store %s not found", ErrKeyValueStoreNotFound, id)
	}
	if comment != "" {
		kvs.Comment = comment
	}
	kvs.ETag = uuid.NewString()
	kvs.Status = kvsStatusReady
	kvs.LastModifiedTime = time.Now().UTC().Format(time.RFC3339)
	cp := *kvs

	return &cp, nil
}

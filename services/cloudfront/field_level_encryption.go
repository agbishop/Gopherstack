package cloudfront

import (
	"fmt"
	"sort"

	"github.com/google/uuid"
)

// renameInIndex moves id from oldName to newName in a name→ID uniqueness index,
// returning false when newName is already taken. A no-op when the name is
// unchanged. Must be called with the lock held.
func renameInIndex(byName map[string]string, id, oldName, newName string) bool {
	if newName == oldName {
		return true
	}

	if _, exists := byName[newName]; exists {
		return false
	}

	delete(byName, oldName)
	byName[newName] = id

	return true
}

// requireProfilesExist verifies every referenced FLE profile ID exists.
// Must be called with the lock held.
func (b *InMemoryBackend) requireProfilesExist(profiles []FLEQueryArgProfile) error {
	for _, p := range profiles {
		if p.ProfileID == "" {
			continue
		}

		if _, ok := b.fieldLevelEncryptionProfiles.Get(p.ProfileID); !ok {
			return fmt.Errorf(
				"%w: field level encryption profile %s not found",
				ErrFLEProfileNotFound,
				p.ProfileID,
			)
		}
	}

	return nil
}

// requirePublicKeysExist verifies every public key referenced by the entities exists.
// Must be called with the lock held.
func (b *InMemoryBackend) requirePublicKeysExist(entities []EncryptionEntity) error {
	for _, e := range entities {
		if e.PublicKeyID == "" {
			continue
		}

		if _, ok := b.publicKeys.Get(e.PublicKeyID); !ok {
			return fmt.Errorf("%w: public key %s not found", ErrPublicKeyNotFound, e.PublicKeyID)
		}
	}

	return nil
}

// cloneQueryArgProfiles returns a defensive copy of the profile slice.
func cloneQueryArgProfiles(in []FLEQueryArgProfile) []FLEQueryArgProfile {
	if len(in) == 0 {
		return nil
	}

	out := make([]FLEQueryArgProfile, len(in))
	copy(out, in)

	return out
}

// cloneEncryptionEntities returns a deep copy of the entity slice.
func cloneEncryptionEntities(in []EncryptionEntity) []EncryptionEntity {
	if len(in) == 0 {
		return nil
	}

	out := make([]EncryptionEntity, len(in))
	for i, e := range in {
		e.FieldPatterns = append([]string(nil), e.FieldPatterns...)
		out[i] = e
	}

	return out
}

// fleNameInUse reports whether any existing FLE config already uses name as its
// CallerReference. Two configs may legitimately share a CallerReference after an
// UpdateFieldLevelEncryption rename (gopherstack-kpk5), so this cannot be a unique
// name->ID index -- it scans the table instead. Must be called with the lock held.
func (b *InMemoryBackend) fleNameInUse(name string) bool {
	for _, fle := range b.fieldLevelEncryptions.All() {
		if fle.Name == name {
			return true
		}
	}

	return false
}

// CreateFieldLevelEncryption creates a new Field Level Encryption config. Every
// referenced FLE profile ID must exist (referential integrity).
func (b *InMemoryBackend) CreateFieldLevelEncryption(
	name, comment string, queryArgProfiles []FLEQueryArgProfile,
) (*FieldLevelEncryption, error) {
	b.mu.Lock("CreateFieldLevelEncryption")
	defer b.mu.Unlock()

	if name == "" {
		return nil, fmt.Errorf("%w: Name must not be empty", ErrValidation)
	}

	if b.fleNameInUse(name) {
		return nil, fmt.Errorf(
			"%w: field level encryption with name %q already exists",
			ErrFLEAlreadyExists,
			name,
		)
	}

	if err := b.requireProfilesExist(queryArgProfiles); err != nil {
		return nil, err
	}

	id := generateID()
	fle := &FieldLevelEncryption{
		ID:               id,
		Name:             name,
		Comment:          comment,
		ETag:             uuid.NewString(),
		QueryArgProfiles: cloneQueryArgProfiles(queryArgProfiles),
	}
	b.fieldLevelEncryptions.Put(fle)
	cp := *fle
	cp.QueryArgProfiles = cloneQueryArgProfiles(fle.QueryArgProfiles)

	return &cp, nil
}

// GetFieldLevelEncryption returns a Field Level Encryption config by ID.
func (b *InMemoryBackend) GetFieldLevelEncryption(id string) (*FieldLevelEncryption, error) {
	b.mu.RLock("GetFieldLevelEncryption")
	defer b.mu.RUnlock()

	fle, ok := b.fieldLevelEncryptions.Get(id)
	if !ok {
		return nil, fmt.Errorf("%w: field level encryption %s not found", ErrFLENotFound, id)
	}

	cp := *fle
	cp.QueryArgProfiles = cloneQueryArgProfiles(fle.QueryArgProfiles)

	return &cp, nil
}

// ListFieldLevelEncryptions returns all Field Level Encryption configs sorted by ID.
func (b *InMemoryBackend) ListFieldLevelEncryptions() []*FieldLevelEncryption {
	b.mu.RLock("ListFieldLevelEncryptions")
	defer b.mu.RUnlock()

	list := make([]*FieldLevelEncryption, 0, b.fieldLevelEncryptions.Len())
	for _, fle := range b.fieldLevelEncryptions.All() {
		cp := *fle
		cp.QueryArgProfiles = cloneQueryArgProfiles(fle.QueryArgProfiles)
		list = append(list, &cp)
	}

	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })

	return list
}

// UpdateFieldLevelEncryption updates an existing Field Level Encryption config.
// Every referenced FLE profile ID must exist (referential integrity).
func (b *InMemoryBackend) UpdateFieldLevelEncryption(
	id, name, comment string, queryArgProfiles []FLEQueryArgProfile,
) (*FieldLevelEncryption, error) {
	b.mu.Lock("UpdateFieldLevelEncryption")
	defer b.mu.Unlock()

	fle, ok := b.fieldLevelEncryptions.Get(id)
	if !ok {
		return nil, fmt.Errorf("%w: field level encryption %s not found", ErrFLENotFound, id)
	}

	if err := b.requireProfilesExist(queryArgProfiles); err != nil {
		return nil, err
	}

	// Unlike CreateFieldLevelEncryptionConfig, real UpdateFieldLevelEncryptionConfig's
	// declared error set has no FieldLevelEncryptionConfigAlreadyExists (cloudfront@
	// v1.67.4 deserializers.go:24068 awsRestxml_deserializeOpErrorUpdateFieldLevelEncryptionConfig)
	// -- same Create-only/Update-silent split as DistributionAlreadyExists and
	// StreamingDistributionAlreadyExists -- so a CallerReference collision on Update is not
	// rejected here; gopherstack-kpk5. That means two configs can legitimately share a
	// name, so there is no unique index to update -- fleNameInUse (gopherstack-lt9v) scans
	// the table directly at Create time instead.
	fle.Name = name
	fle.Comment = comment
	fle.QueryArgProfiles = cloneQueryArgProfiles(queryArgProfiles)
	fle.ETag = uuid.NewString()
	cp := *fle
	cp.QueryArgProfiles = cloneQueryArgProfiles(fle.QueryArgProfiles)

	return &cp, nil
}

// DeleteFieldLevelEncryption deletes a Field Level Encryption config by ID.
func (b *InMemoryBackend) DeleteFieldLevelEncryption(id string) error {
	b.mu.Lock("DeleteFieldLevelEncryption")
	defer b.mu.Unlock()

	if _, ok := b.fieldLevelEncryptions.Get(id); !ok {
		return fmt.Errorf("%w: field level encryption %s not found", ErrFLENotFound, id)
	}

	b.fieldLevelEncryptions.Delete(id)

	return nil
}

// --- Field Level Encryption Profile CRUD ---

// CreateFieldLevelEncryptionProfile creates a new Field Level Encryption Profile.
// Every public key referenced by an EncryptionEntity must exist (referential integrity).
func (b *InMemoryBackend) CreateFieldLevelEncryptionProfile(
	name, comment string, entities []EncryptionEntity,
) (*FieldLevelEncryptionProfile, error) {
	b.mu.Lock("CreateFieldLevelEncryptionProfile")
	defer b.mu.Unlock()

	if name == "" {
		return nil, fmt.Errorf("%w: Name must not be empty", ErrValidation)
	}

	if _, exists := b.fieldLevelEncryptionProfileByName[name]; exists {
		return nil, fmt.Errorf(
			"%w: field level encryption profile with name %q already exists",
			ErrFLEProfileAlreadyExists,
			name,
		)
	}

	if err := b.requirePublicKeysExist(entities); err != nil {
		return nil, err
	}

	id := generateID()
	p := &FieldLevelEncryptionProfile{
		ID:                 id,
		Name:               name,
		Comment:            comment,
		ETag:               uuid.NewString(),
		EncryptionEntities: cloneEncryptionEntities(entities),
	}
	b.fieldLevelEncryptionProfiles.Put(p)
	b.fieldLevelEncryptionProfileByName[name] = id
	cp := *p
	cp.EncryptionEntities = cloneEncryptionEntities(p.EncryptionEntities)

	return &cp, nil
}

// GetFieldLevelEncryptionProfile returns a Field Level Encryption Profile by ID.
func (b *InMemoryBackend) GetFieldLevelEncryptionProfile(
	id string,
) (*FieldLevelEncryptionProfile, error) {
	b.mu.RLock("GetFieldLevelEncryptionProfile")
	defer b.mu.RUnlock()

	p, ok := b.fieldLevelEncryptionProfiles.Get(id)
	if !ok {
		return nil, fmt.Errorf(
			"%w: field level encryption profile %s not found",
			ErrFLEProfileNotFound,
			id,
		)
	}

	cp := *p
	cp.EncryptionEntities = cloneEncryptionEntities(p.EncryptionEntities)

	return &cp, nil
}

// ListFieldLevelEncryptionProfiles returns all FLE profiles sorted by ID.
func (b *InMemoryBackend) ListFieldLevelEncryptionProfiles() []*FieldLevelEncryptionProfile {
	b.mu.RLock("ListFieldLevelEncryptionProfiles")
	defer b.mu.RUnlock()

	list := make([]*FieldLevelEncryptionProfile, 0, b.fieldLevelEncryptionProfiles.Len())
	for _, p := range b.fieldLevelEncryptionProfiles.All() {
		cp := *p
		cp.EncryptionEntities = cloneEncryptionEntities(p.EncryptionEntities)
		list = append(list, &cp)
	}

	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })

	return list
}

// UpdateFieldLevelEncryptionProfile updates an existing FLE profile. Every public
// key referenced by an EncryptionEntity must exist (referential integrity).
func (b *InMemoryBackend) UpdateFieldLevelEncryptionProfile(
	id, name, comment string, entities []EncryptionEntity,
) (*FieldLevelEncryptionProfile, error) {
	b.mu.Lock("UpdateFieldLevelEncryptionProfile")
	defer b.mu.Unlock()

	p, ok := b.fieldLevelEncryptionProfiles.Get(id)
	if !ok {
		return nil, fmt.Errorf(
			"%w: field level encryption profile %s not found",
			ErrFLEProfileNotFound,
			id,
		)
	}

	if err := b.requirePublicKeysExist(entities); err != nil {
		return nil, err
	}

	if !renameInIndex(b.fieldLevelEncryptionProfileByName, id, p.Name, name) {
		return nil, fmt.Errorf(
			"%w: field level encryption profile with name %q already exists", ErrFLEProfileAlreadyExists, name)
	}

	p.Name = name
	p.Comment = comment
	p.EncryptionEntities = cloneEncryptionEntities(entities)
	p.ETag = uuid.NewString()
	cp := *p
	cp.EncryptionEntities = cloneEncryptionEntities(p.EncryptionEntities)

	return &cp, nil
}

// fleProfileReferencedBy returns the ID of an FLE config that references the given
// profile, or "" if none. Must be called with the lock held.
func (b *InMemoryBackend) fleProfileReferencedBy(profileID string) string {
	for _, fle := range b.fieldLevelEncryptions.All() {
		for _, qp := range fle.QueryArgProfiles {
			if qp.ProfileID == profileID {
				return fle.ID
			}
		}
	}

	return ""
}

// DeleteFieldLevelEncryptionProfile deletes an FLE profile by ID. It returns
// ErrFLEProfileInUse when the profile is still referenced by an FLE config.
func (b *InMemoryBackend) DeleteFieldLevelEncryptionProfile(id string) error {
	b.mu.Lock("DeleteFieldLevelEncryptionProfile")
	defer b.mu.Unlock()

	p, ok := b.fieldLevelEncryptionProfiles.Get(id)
	if !ok {
		return fmt.Errorf(
			"%w: field level encryption profile %s not found",
			ErrFLEProfileNotFound,
			id,
		)
	}

	if configID := b.fleProfileReferencedBy(id); configID != "" {
		return fmt.Errorf(
			"%w: field level encryption profile %s is referenced by config %s",
			ErrFLEProfileInUse,
			id,
			configID,
		)
	}

	delete(b.fieldLevelEncryptionProfileByName, p.Name)
	b.fieldLevelEncryptionProfiles.Delete(id)

	return nil
}

// --- Public Key CRUD ---

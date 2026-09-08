package secretsmanager

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/persistence"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// secretsManagerSnapshotVersion identifies the shape of [backendSnapshot]. It
// must be bumped whenever a change to secretSnapshot or backendSnapshot
// itself would make an older snapshot unsafe to decode as the current shape
// (e.g. a field's meaning or type changes). Restore compares this against the
// persisted value and discards (rather than attempts to partially decode) any
// mismatch — see Restore below. This is the first version: earlier
// (pre-Phase-3.3) snapshots carried no version field at all, so they also
// fail this check and are discarded rather than misread, which is the safe
// behaviour across any snapshot-format change.
const secretsManagerSnapshotVersion = 1

// secretSnapshot captures all fields of Secret including those tagged
// json:"-" and the unexported region field, which is needed to rebuild both
// the live Secret and the store.Table composite key ("region|name", see
// secretTableKey in store_setup.go) on Restore.
type secretSnapshot struct {
	Tags                           *tags.Tags                           `json:"tags,omitempty"`
	DeletedDate                    *float64                             `json:"deletedDate,omitempty"`
	LastChangedDate                *float64                             `json:"lastChangedDate,omitempty"`
	LastRotatedDate                *float64                             `json:"lastRotatedDate,omitempty"`
	LastAccessedDate               *float64                             `json:"lastAccessedDate,omitempty"`
	CreatedDate                    *float64                             `json:"createdDate,omitempty"`
	Versions                       map[string]*SecretVersion            `json:"versions"`
	RotationRules                  *RotationRulesType                   `json:"rotationRules,omitempty"`
	Name                           string                               `json:"name"`
	Region                         string                               `json:"region"`
	PrimaryRegion                  string                               `json:"primaryRegion,omitempty"`
	Description                    string                               `json:"description,omitempty"`
	KmsKeyID                       string                               `json:"kmsKeyID,omitempty"`
	RotationLambdaARN              string                               `json:"rotationLambdaARN,omitempty"`
	ARN                            string                               `json:"arn"`
	CurrentVersionID               string                               `json:"currentVersionID"`
	Type                           string                               `json:"type,omitempty"`
	ExternalSecretRotationRoleArn  string                               `json:"externalSecretRotationRoleArn,omitempty"`
	ExternalSecretRotationMetadata []ExternalSecretRotationMetadataItem `json:"externalSecretRotationMetadata,omitempty"`
	RotationEnabled                bool                                 `json:"rotationEnabled,omitempty"`
}

// secretSnapshotKeyFn is the [store.Table] key function used for the
// ephemeral DTO table built inside Snapshot/Restore. It mirrors secretKeyFn
// so the on-disk table is keyed identically to the live b.secrets table.
func secretSnapshotKeyFn(ss *secretSnapshot) string {
	return secretTableKey(ss.Region, ss.Name)
}

// backendSnapshot is the top-level on-disk shape for the Secrets Manager
// backend.
//
// Tables holds one JSON-encoded array per registered DTO table, produced by
// [store.Registry.SnapshotAll] — currently just "secrets" ([]*secretSnapshot).
// resourcePolicies and replicationConfigs are still region-nested plain maps
// (see store_setup.go for why they were not converted to store.Table) and are
// persisted directly, as before. Version guards against decoding a snapshot
// from an incompatible (older or newer) build of this backend as though it
// were the current shape; see Restore.
type backendSnapshot struct {
	Tables             map[string]json.RawMessage                    `json:"tables"`
	ResourcePolicies   map[string]map[string]string                  `json:"resourcePolicies,omitempty"`
	ReplicationConfigs map[string]map[string][]ReplicationStatusType `json:"replicationConfigs,omitempty"`
	AccountID          string                                        `json:"accountID"`
	Region             string                                        `json:"region"`
	Version            int                                           `json:"version"`
}

// Snapshot serialises the backend state to JSON.
// It implements persistence.Persistable.
func (b *InMemoryBackend) Snapshot(ctx context.Context) []byte {
	b.mu.RLock("Snapshot")
	defer b.mu.RUnlock()

	// Build a throwaway DTO registry purely to reuse store's deterministic,
	// type-erased JSON encoding (store.Registry.SnapshotAll) instead of
	// hand-rolling the marshal step. This is intentionally separate from the
	// live b.registry: Secret carries an unexported region field that
	// json.Marshal never sees, so a direct snapshot of the live table could
	// not round-trip it.
	dtoReg := store.NewRegistry()
	secretDTOs := store.Register(dtoReg, "secrets", store.New(secretSnapshotKeyFn))

	for _, s := range b.secrets.Snapshot() {
		secretDTOs.Put(&secretSnapshot{
			ARN:                            s.ARN,
			Name:                           s.Name,
			Region:                         s.region,
			PrimaryRegion:                  s.PrimaryRegion,
			Description:                    s.Description,
			KmsKeyID:                       s.KmsKeyID,
			RotationLambdaARN:              s.RotationLambdaARN,
			RotationRules:                  cloneRotationRules(s.RotationRules),
			Tags:                           s.Tags,
			DeletedDate:                    s.DeletedDate,
			LastChangedDate:                s.LastChangedDate,
			LastRotatedDate:                s.LastRotatedDate,
			LastAccessedDate:               s.LastAccessedDate,
			CreatedDate:                    s.CreatedDate,
			Versions:                       s.Versions,
			CurrentVersionID:               s.CurrentVersionID,
			RotationEnabled:                s.RotationEnabled,
			Type:                           s.Type,
			ExternalSecretRotationRoleArn:  s.ExternalSecretRotationRoleArn,
			ExternalSecretRotationMetadata: cloneExternalSecretRotationMetadata(s.ExternalSecretRotationMetadata),
		})
	}

	tables, err := dtoReg.SnapshotAll()
	if err != nil {
		// The DTOs above are plain JSON-friendly structs, so a marshal failure
		// here would indicate a programming error rather than bad input data.
		// Log and skip the snapshot rather than panic, matching the
		// persistence.Persistable contract (nil is skipped by the Manager).
		logger.Load(ctx).WarnContext(ctx,
			"secretsmanager: snapshot table marshal failed", "error", err)

		return nil
	}

	snap := backendSnapshot{
		Version:            secretsManagerSnapshotVersion,
		Tables:             tables,
		ResourcePolicies:   b.resourcePolicies,
		ReplicationConfigs: b.replicationConfigs,
		AccountID:          b.accountID,
		Region:             b.region,
	}

	return persistence.MarshalSnapshot(ctx, "secretsmanager", snap)
}

// Restore loads backend state from a JSON snapshot.
// It implements persistence.Persistable.
func (b *InMemoryBackend) Restore(ctx context.Context, data []byte) error {
	var snap backendSnapshot

	if err := persistence.UnmarshalSnapshot(ctx, "secretsmanager", data, &snap); err != nil {
		return err
	}

	b.mu.Lock("Restore")
	defer b.mu.Unlock()

	// Close Tags on any secrets that are being replaced to prevent
	// Prometheus registry leaks.
	for _, secret := range b.secrets.All() {
		if secret.Tags != nil {
			secret.Tags.Close()
		}
	}

	if snap.Version != secretsManagerSnapshotVersion {
		// An incompatible (older/newer/absent) snapshot version must never be
		// partially decoded as the current shape — that risks silently
		// misinterpreting fields. Discard cleanly and start empty instead of
		// erroring, since this is an expected, recoverable condition (e.g.
		// upgrading gopherstack across a snapshot-format change), not data
		// corruption.
		logger.Load(ctx).WarnContext(ctx,
			"secretsmanager: discarding incompatible snapshot version, starting empty",
			"gotVersion", snap.Version, "wantVersion", secretsManagerSnapshotVersion)

		b.registry.ResetAll()
		b.resourcePolicies = make(map[string]map[string]string)
		b.replicationConfigs = make(map[string]map[string][]ReplicationStatusType)
		b.accountID = snap.AccountID
		b.region = snap.Region

		return nil
	}

	dtoReg := store.NewRegistry()
	secretDTOs := store.Register(dtoReg, "secrets", store.New(secretSnapshotKeyFn))

	if err := dtoReg.RestoreAll(snap.Tables); err != nil {
		return fmt.Errorf("secretsmanager: restore snapshot tables: %w", err)
	}

	liveSecrets := make([]*Secret, 0, secretDTOs.Len())

	for _, ss := range secretDTOs.All() {
		liveSecrets = append(liveSecrets, secretFromSnapshot(ss))
	}

	b.secrets.Restore(liveSecrets)

	b.accountID = snap.AccountID
	b.region = snap.Region

	if snap.ResourcePolicies == nil {
		snap.ResourcePolicies = make(map[string]map[string]string)
	}

	b.resourcePolicies = snap.ResourcePolicies

	if snap.ReplicationConfigs == nil {
		snap.ReplicationConfigs = make(map[string]map[string][]ReplicationStatusType)
	}

	b.replicationConfigs = snap.ReplicationConfigs

	for _, secret := range b.secrets.All() {
		if secret.RotationRules != nil && secret.RotationEnabled {
			b.ensureRotationScheduler()

			return nil
		}
	}

	return nil
}

// secretFromSnapshot rebuilds a live Secret (including its region field) from
// its persisted representation.
func secretFromSnapshot(ss *secretSnapshot) *Secret {
	if ss.Versions == nil {
		ss.Versions = make(map[string]*SecretVersion)
	}

	return &Secret{
		region:                         ss.Region,
		ARN:                            ss.ARN,
		Name:                           ss.Name,
		PrimaryRegion:                  ss.PrimaryRegion,
		Description:                    ss.Description,
		KmsKeyID:                       ss.KmsKeyID,
		RotationLambdaARN:              ss.RotationLambdaARN,
		RotationRules:                  cloneRotationRules(ss.RotationRules),
		Tags:                           ss.Tags,
		DeletedDate:                    ss.DeletedDate,
		LastChangedDate:                ss.LastChangedDate,
		LastRotatedDate:                ss.LastRotatedDate,
		LastAccessedDate:               ss.LastAccessedDate,
		CreatedDate:                    ss.CreatedDate,
		Versions:                       ss.Versions,
		CurrentVersionID:               ss.CurrentVersionID,
		RotationEnabled:                ss.RotationEnabled,
		Type:                           ss.Type,
		ExternalSecretRotationRoleArn:  ss.ExternalSecretRotationRoleArn,
		ExternalSecretRotationMetadata: cloneExternalSecretRotationMetadata(ss.ExternalSecretRotationMetadata),
	}
}

// Snapshot implements persistence.Persistable by delegating to the backend.
func (h *Handler) Snapshot(ctx context.Context) []byte {
	type snapshotter interface {
		Snapshot(ctx context.Context) []byte
	}
	if s, ok := h.Backend.(snapshotter); ok {
		return s.Snapshot(ctx)
	}

	return nil
}

// Restore implements persistence.Persistable by delegating to the backend.
func (h *Handler) Restore(ctx context.Context, data []byte) error {
	type restorer interface {
		Restore(context.Context, []byte) error
	}
	if r, ok := h.Backend.(restorer); ok {
		return r.Restore(ctx, data)
	}

	return nil
}

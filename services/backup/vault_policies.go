package backup

import (
	"encoding/json"
	"fmt"
	"time"

	sdktypes "github.com/aws/aws-sdk-go-v2/service/backup/types"
)

// isValidVaultEvent derives its answer from types.BackupVaultEvent.Values()
// so it cannot drift from the real enum -- the previous hand-copied list
// misspelled COPY_JOB_FAILED as "COPY_JOB_FAILURE" and was missing 7 newer
// values (CONTINUOUS_BACKUP_INTERRUPTED, the RECOVERY_POINT_INDEX* trio, and
// the three EKS_* events).
func isValidVaultEvent(event string) bool {
	for _, v := range sdktypes.BackupVaultEvent("").Values() {
		if string(v) == event {
			return true
		}
	}

	return false
}

// PutBackupVaultAccessPolicy sets an access policy for a vault.
// The policy must be valid JSON representing an IAM policy document.
func (b *InMemoryBackend) PutBackupVaultAccessPolicy(vaultName, policy string) error {
	b.mu.Lock("PutBackupVaultAccessPolicy")
	defer b.mu.Unlock()

	if !b.vaults.Has(vaultName) {
		return fmt.Errorf("%w: vault %s not found", ErrNotFound, vaultName)
	}

	if policy != "" {
		var doc map[string]any
		if err := json.Unmarshal([]byte(policy), &doc); err != nil {
			return fmt.Errorf(
				"%w: Policy must be a valid JSON document: %s",
				ErrValidation,
				err.Error(),
			)
		}
	}

	b.vaultAccessPolicies.Put(&VaultAccessPolicy{VaultName: vaultName, Policy: policy})

	return nil
}

// GetBackupVaultAccessPolicy returns the access policy for a vault.
func (b *InMemoryBackend) GetBackupVaultAccessPolicy(vaultName string) (*VaultAccessPolicy, error) {
	b.mu.RLock("GetBackupVaultAccessPolicy")
	defer b.mu.RUnlock()

	if !b.vaults.Has(vaultName) {
		return nil, fmt.Errorf("%w: vault %s not found", ErrNotFound, vaultName)
	}

	pol, ok := b.vaultAccessPolicies.Get(vaultName)
	if !ok {
		return nil, fmt.Errorf("%w: no access policy for vault %s", ErrNotFound, vaultName)
	}

	cp := *pol

	return &cp, nil
}

// DeleteBackupVaultAccessPolicy deletes the access policy for a vault.
func (b *InMemoryBackend) DeleteBackupVaultAccessPolicy(vaultName string) error {
	b.mu.Lock("DeleteBackupVaultAccessPolicy")
	defer b.mu.Unlock()

	if !b.vaults.Has(vaultName) {
		return fmt.Errorf("%w: vault %s not found", ErrNotFound, vaultName)
	}

	b.vaultAccessPolicies.Delete(vaultName)

	return nil
}

// PutBackupVaultLockConfiguration sets the lock configuration for a vault.
// If a LockDate already exists and has passed, the configuration is immutable.
func (b *InMemoryBackend) PutBackupVaultLockConfiguration(
	vaultName string,
	cfg *VaultLockConfig,
) error {
	b.mu.Lock("PutBackupVaultLockConfiguration")
	defer b.mu.Unlock()

	if !b.vaults.Has(vaultName) {
		return fmt.Errorf("%w: vault %s not found", ErrNotFound, vaultName)
	}

	if existing, ok := b.vaultLockConfigs.Get(vaultName); ok {
		if existing.LockDate != nil && time.Now().UTC().After(*existing.LockDate) {
			return fmt.Errorf(
				"%w: vault lock configuration is immutable: LockDate %s has passed",
				ErrInvalidRequest,
				existing.LockDate.Format(time.RFC3339),
			)
		}
	}

	cp := *cfg
	cp.VaultName = vaultName
	if cp.ChangeableForDays > 0 && cp.LockDate == nil {
		lockDate := time.Now().UTC().Add(
			time.Duration(cp.ChangeableForDays) * 24 * time.Hour,
		)
		cp.LockDate = &lockDate
	}
	b.vaultLockConfigs.Put(&cp)

	return nil
}

// GetBackupVaultLockConfig returns the lock configuration for a vault.
func (b *InMemoryBackend) GetBackupVaultLockConfig(vaultName string) (*VaultLockConfig, error) {
	b.mu.RLock("GetBackupVaultLockConfig")
	defer b.mu.RUnlock()

	cfg, ok := b.vaultLockConfigs.Get(vaultName)
	if !ok {
		return nil, fmt.Errorf("%w: no lock config for vault %s", ErrNotFound, vaultName)
	}

	cp := *cfg

	return &cp, nil
}

// DeleteBackupVaultLockConfiguration deletes the lock configuration for a vault.
// PutBackupVaultLockConfiguration's own doc comment: "On and after the lock
// date, the Vault Lock becomes immutable and cannot be changed or deleted" --
// PutBackupVaultLockConfiguration already enforces this for updates; this was
// its unenforced sibling for deletes.
func (b *InMemoryBackend) DeleteBackupVaultLockConfiguration(vaultName string) error {
	b.mu.Lock("DeleteBackupVaultLockConfiguration")
	defer b.mu.Unlock()

	if !b.vaults.Has(vaultName) {
		return fmt.Errorf("%w: vault %s not found", ErrNotFound, vaultName)
	}

	if existing, ok := b.vaultLockConfigs.Get(vaultName); ok {
		if existing.LockDate != nil && time.Now().UTC().After(*existing.LockDate) {
			return fmt.Errorf(
				"%w: vault lock configuration is immutable: LockDate %s has passed",
				ErrInvalidRequest,
				existing.LockDate.Format(time.RFC3339),
			)
		}
	}

	b.vaultLockConfigs.Delete(vaultName)

	return nil
}

// PutBackupVaultNotifications sets notification configuration for a vault.
// Events are validated against the canonical AWS Backup event enum.
func (b *InMemoryBackend) PutBackupVaultNotifications(
	vaultName string,
	cfg *VaultNotificationConfig,
) error {
	b.mu.Lock("PutBackupVaultNotifications")
	defer b.mu.Unlock()

	if !b.vaults.Has(vaultName) {
		return fmt.Errorf("%w: vault %s not found", ErrNotFound, vaultName)
	}

	for _, ev := range cfg.BackupVaultEvents {
		if !isValidVaultEvent(ev) {
			return fmt.Errorf(
				"%w: invalid BackupVaultEvent %q; must be one of the allowed event types",
				ErrValidation,
				ev,
			)
		}
	}

	cp := *cfg
	cp.VaultName = vaultName
	cp.BackupVaultEvents = make([]string, len(cfg.BackupVaultEvents))
	copy(cp.BackupVaultEvents, cfg.BackupVaultEvents)
	b.vaultNotifications.Put(&cp)

	return nil
}

// GetBackupVaultNotifications returns notification configuration for a vault.
func (b *InMemoryBackend) GetBackupVaultNotifications(
	vaultName string,
) (*VaultNotificationConfig, error) {
	b.mu.RLock("GetBackupVaultNotifications")
	defer b.mu.RUnlock()

	if !b.vaults.Has(vaultName) {
		return nil, fmt.Errorf("%w: vault %s not found", ErrNotFound, vaultName)
	}

	cfg, ok := b.vaultNotifications.Get(vaultName)
	if !ok {
		return nil, fmt.Errorf(
			"%w: no notification configuration for vault %s",
			ErrNotFound,
			vaultName,
		)
	}

	cp := *cfg
	cp.BackupVaultEvents = make([]string, len(cfg.BackupVaultEvents))
	copy(cp.BackupVaultEvents, cfg.BackupVaultEvents)

	return &cp, nil
}

// DeleteBackupVaultNotifications deletes notification configuration for a vault.
func (b *InMemoryBackend) DeleteBackupVaultNotifications(vaultName string) error {
	b.mu.Lock("DeleteBackupVaultNotifications")
	defer b.mu.Unlock()

	if !b.vaults.Has(vaultName) {
		return fmt.Errorf("%w: vault %s not found", ErrNotFound, vaultName)
	}

	b.vaultNotifications.Delete(vaultName)

	return nil
}

// --- Backup Selection read/delete methods ---

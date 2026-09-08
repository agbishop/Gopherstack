package backup_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/backup"
)

func TestDeleteBackupVaultChecked(t *testing.T) {
	t.Parallel()

	t.Run("delete unlocked empty vault succeeds", func(t *testing.T) {
		t.Parallel()
		b := newTestBackend(t)
		mustVault(t, b, "unlocked")
		if err := b.DeleteBackupVaultChecked("unlocked"); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("delete vault with recovery points fails", func(t *testing.T) {
		t.Parallel()
		b := newTestBackend(t)
		mustVault(t, b, "has-rp")
		mustRP(t, b, "has-rp", "arn:aws:backup:::rp/1", "arn:aws:ec2:::instance/i-1", "EC2")
		if err := b.DeleteBackupVaultChecked("has-rp"); err == nil {
			t.Error("expected error deleting vault with recovery points")
		}
	})

	t.Run("delete locked vault fails", func(t *testing.T) {
		t.Parallel()
		b := newTestBackend(t)
		mustVault(t, b, "locked")
		// Pass a LockDate in the past directly — PutBackupVaultLockConfiguration stores it as-is
		// when ChangeableForDays == 0, so the vault is immediately in locked state.
		past := time.Now().Add(-1 * time.Hour)
		if lockErr := b.PutBackupVaultLockConfiguration("locked", &backup.VaultLockConfig{
			MinRetentionDays: 1,
			MaxRetentionDays: 365,
			LockDate:         &past,
		}); lockErr != nil {
			t.Fatalf("PutBackupVaultLockConfiguration: %v", lockErr)
		}
		if delErr := b.DeleteBackupVaultChecked("locked"); delErr == nil {
			t.Error("expected error deleting locked vault")
		}
	})

	t.Run("delete nonexistent vault returns not-found", func(t *testing.T) {
		t.Parallel()
		b := newTestBackend(t)
		if err := b.DeleteBackupVaultChecked("ghost"); err == nil {
			t.Error("expected error, got nil")
		}
	})
}

func TestDeleteBackupVaultLockConfiguration_Immutable(t *testing.T) {
	t.Parallel()

	t.Run("changeable lock deletes cleanly", func(t *testing.T) {
		t.Parallel()
		b := newTestBackend(t)
		mustVault(t, b, "changeable")
		require.NoError(t, b.PutBackupVaultLockConfiguration("changeable", &backup.VaultLockConfig{
			MinRetentionDays: 1,
			MaxRetentionDays: 365,
		}))

		require.NoError(t, b.DeleteBackupVaultLockConfiguration("changeable"))
		_, err := b.GetBackupVaultLockConfig("changeable")
		require.ErrorIs(t, err, backup.ErrNotFound)
	})

	t.Run("matured lock date is immutable", func(t *testing.T) {
		t.Parallel()
		b := newTestBackend(t)
		mustVault(t, b, "matured")
		past := time.Now().Add(-1 * time.Hour)
		require.NoError(t, b.PutBackupVaultLockConfiguration("matured", &backup.VaultLockConfig{
			MinRetentionDays: 1,
			MaxRetentionDays: 365,
			LockDate:         &past,
		}))

		err := b.DeleteBackupVaultLockConfiguration("matured")
		require.ErrorIs(t, err, backup.ErrInvalidRequest)

		cfg, getErr := b.GetBackupVaultLockConfig("matured")
		require.NoError(t, getErr)
		assert.NotNil(t, cfg)
	})
}

// ---- CompleteBackupJob ----

func TestListBackupVaultsFiltered(t *testing.T) {
	t.Parallel()
	b := newTestBackend(t)
	mustVault(t, b, "plain-vault")
	mustVault(t, b, "plain-vault2")

	// PutBackupVaultLockConfiguration only stores a lock policy (VaultLockConfig)
	// in a separate table; it does not touch Vault.VaultType or
	// Vault.MinRetentionDays. A logically air-gapped vault is a distinct
	// resource created via CreateLogicallyAirGappedBackupVault.
	if _, err := b.CreateLogicallyAirGappedBackupVault(
		"locked-vault", "", 30, 365, nil,
	); err != nil {
		t.Fatalf("CreateLogicallyAirGappedBackupVault: %v", err)
	}

	cases := []struct {
		name      string
		filter    backup.ListVaultsFilter
		wantCount int
	}{
		{
			name:      "no filter returns all",
			filter:    backup.ListVaultsFilter{},
			wantCount: 3,
		},
		{
			name:      "maxResults=1 limits page",
			filter:    backup.ListVaultsFilter{MaxResults: 1},
			wantCount: 1,
		},
		{
			name:      "ByVaultType=BACKUP_VAULT returns only regular vaults",
			filter:    backup.ListVaultsFilter{VaultType: backup.VaultTypeBackupVault},
			wantCount: 2,
		},
		{
			name:      "ByVaultType=LOGICALLY_AIR_GAPPED_BACKUP_VAULT returns only the air-gapped vault",
			filter:    backup.ListVaultsFilter{VaultType: backup.VaultTypeAirGapped},
			wantCount: 1,
		},
		{
			// types.VaultType (aws-sdk-go-v2/service/backup@v1.59.4 enums.go)
			// documents a third value, RESTORE_ACCESS_BACKUP_VAULT, that no
			// vault in this backend's store ever carries (restore access
			// vaults live in a separate table entirely) -- filtering on it
			// must return nothing, not fall through and match every vault.
			name:      "ByVaultType=RESTORE_ACCESS_BACKUP_VAULT matches nothing",
			filter:    backup.ListVaultsFilter{VaultType: "RESTORE_ACCESS_BACKUP_VAULT"},
			wantCount: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, _ := b.ListBackupVaultsFiltered(tc.filter)
			if len(got) != tc.wantCount {
				t.Errorf("count: want %d got %d", tc.wantCount, len(got))
			}
		})
	}
}

// ---- ListBackupVaults pagination ----

func TestListBackupVaultsPagination(t *testing.T) {
	t.Parallel()
	b := newTestBackend(t)
	const total = 6
	for i := range total {
		mustVault(t, b, fmt.Sprintf("pg-vault-%d", i))
	}

	t.Run("paginate all vaults", func(t *testing.T) {
		t.Parallel()
		var all []*backup.Vault
		nextToken := ""
		for {
			got, next := b.ListBackupVaultsFiltered(
				backup.ListVaultsFilter{MaxResults: 2, NextToken: nextToken},
			)
			all = append(all, got...)
			if next == "" {
				break
			}
			nextToken = next
		}
		if len(all) != total {
			t.Errorf("want %d got %d", total, len(all))
		}
	})
}

// ---- ListBackupPlansPaged pagination ----

// ---- RestoreAccessVault ----

func TestRestoreAccessVaultCreate(t *testing.T) {
	t.Parallel()

	t.Run("resolves SourceBackupVaultArn to a real vault", func(t *testing.T) {
		t.Parallel()
		b := newTestBackend(t)
		src := mustVault(t, b, "src-vault")
		rav, err := b.CreateRestoreAccessBackupVault(src.BackupVaultArn, "rav1", "", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rav.SourceBackupVaultArn != src.BackupVaultArn {
			t.Errorf("SourceBackupVaultArn: want %s got %s", src.BackupVaultArn, rav.SourceBackupVaultArn)
		}
		if rav.RestoreAccessBackupVaultName != "rav1" {
			t.Errorf("RestoreAccessBackupVaultName: want rav1 got %s", rav.RestoreAccessBackupVaultName)
		}
	})

	t.Run("unresolvable SourceBackupVaultArn is not-found", func(t *testing.T) {
		t.Parallel()
		b := newTestBackend(t)
		_, err := b.CreateRestoreAccessBackupVault(
			"arn:aws:backup:us-east-1:000000000000:backup-vault:ghost", "rav-ghost", "", nil,
		)
		if err == nil {
			t.Fatal("expected error for unresolvable source vault ARN")
		}
	})

	t.Run("missing SourceBackupVaultArn is a validation error", func(t *testing.T) {
		t.Parallel()
		b := newTestBackend(t)
		_, err := b.CreateRestoreAccessBackupVault("", "rav-noarn", "", nil)
		if err == nil {
			t.Fatal("expected error for missing SourceBackupVaultArn")
		}
	})
}

func TestRestoreAccessVaultCreateTagsAndCreatorRequestID(t *testing.T) {
	t.Parallel()

	t.Run("BackupVaultTags are stored and readable via ListTags", func(t *testing.T) {
		t.Parallel()
		b := newTestBackend(t)
		src := mustVault(t, b, "tags-src")
		rav, err := b.CreateRestoreAccessBackupVault(
			src.BackupVaultArn, "rav-tags", "", map[string]string{"env": "prod"},
		)
		require.NoError(t, err)

		got, err := b.ListTags(rav.RestoreAccessBackupVaultArn)
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"env": "prod"}, got)
	})

	t.Run("BackupVaultTags flow into TaggedResources for the tagging API hook", func(t *testing.T) {
		t.Parallel()
		b := newTestBackend(t)
		src := mustVault(t, b, "tagged-src")
		rav, err := b.CreateRestoreAccessBackupVault(
			src.BackupVaultArn, "rav-tagged", "", map[string]string{"team": "sre"},
		)
		require.NoError(t, err)

		var found *backup.TaggedEntry
		for _, e := range b.TaggedResources() {
			if e.ARN == rav.RestoreAccessBackupVaultArn {
				e := e
				found = &e
			}
		}
		require.NotNil(t, found, "restore access vault tags must be surfaced by TaggedResources")
		assert.Equal(t, map[string]string{"team": "sre"}, found.Tags)
	})

	t.Run("duplicate name without matching CreatorRequestId is AlreadyExists", func(t *testing.T) {
		t.Parallel()
		b := newTestBackend(t)
		src := mustVault(t, b, "dup-src")
		_, err := b.CreateRestoreAccessBackupVault(src.BackupVaultArn, "rav-dup", "", nil)
		require.NoError(t, err)

		_, err = b.CreateRestoreAccessBackupVault(src.BackupVaultArn, "rav-dup", "", nil)
		require.ErrorIs(t, err, backup.ErrAlreadyExists)
	})

	t.Run("duplicate name with matching CreatorRequestId is idempotent", func(t *testing.T) {
		t.Parallel()
		b := newTestBackend(t)
		src := mustVault(t, b, "idem-src")
		first, err := b.CreateRestoreAccessBackupVault(src.BackupVaultArn, "rav-idem", "req-1", nil)
		require.NoError(t, err)

		second, err := b.CreateRestoreAccessBackupVault(src.BackupVaultArn, "rav-idem", "req-1", nil)
		require.NoError(t, err)
		assert.Equal(t, first.RestoreAccessBackupVaultArn, second.RestoreAccessBackupVaultArn)

		all, err := b.ListRestoreAccessBackupVaults("idem-src")
		require.NoError(t, err)
		assert.Len(t, all, 1, "idempotent retry must not create a second restore access vault")
	})
}

func TestRestoreAccessVaultCreate_ReachesAvailable(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)
	src := mustVault(t, b, "src-vault-active")

	rav, err := b.CreateRestoreAccessBackupVault(src.BackupVaultArn, "rav-active", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The immediate CREATING status is right (matches AWS's own
	// CreateRestoreAccessBackupVault response), but an assertion that only
	// checks that moment cannot catch a machine that never advances --
	// confirm the janitor eventually reports AVAILABLE too.
	if rav.VaultState != "CREATING" {
		t.Fatalf("VaultState: want CREATING got %s", rav.VaultState)
	}

	janitor := backup.NewJanitor(b, 0, 0)
	janitor.SweepOnce(t.Context())

	vaults, err := b.ListRestoreAccessBackupVaults("src-vault-active")
	if err != nil {
		t.Fatalf("ListRestoreAccessBackupVaults: %v", err)
	}
	if len(vaults) != 1 {
		t.Fatalf("want 1 restore access vault, got %d", len(vaults))
	}
	if vaults[0].VaultState != "AVAILABLE" {
		t.Errorf("VaultState after sweep: want AVAILABLE got %s", vaults[0].VaultState)
	}
}

func TestRestoreAccessVaultList(t *testing.T) {
	t.Parallel()

	t.Run("scoped to the source vault name, sorted by name", func(t *testing.T) {
		t.Parallel()
		b := newTestBackend(t)
		srcA := mustVault(t, b, "src-a")
		srcB := mustVault(t, b, "src-b")

		if _, err := b.CreateRestoreAccessBackupVault(srcA.BackupVaultArn, "rav-a2", "", nil); err != nil {
			t.Fatalf("CreateRestoreAccessBackupVault: %v", err)
		}
		if _, err := b.CreateRestoreAccessBackupVault(srcA.BackupVaultArn, "rav-a1", "", nil); err != nil {
			t.Fatalf("CreateRestoreAccessBackupVault: %v", err)
		}
		if _, err := b.CreateRestoreAccessBackupVault(srcB.BackupVaultArn, "rav-b1", "", nil); err != nil {
			t.Fatalf("CreateRestoreAccessBackupVault: %v", err)
		}

		got, err := b.ListRestoreAccessBackupVaults("src-a")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("want 2 restore access vaults for src-a, got %d", len(got))
		}
		if got[0].RestoreAccessBackupVaultName != "rav-a1" || got[1].RestoreAccessBackupVaultName != "rav-a2" {
			t.Errorf(
				"unexpected order: %s, %s",
				got[0].RestoreAccessBackupVaultName,
				got[1].RestoreAccessBackupVaultName,
			)
		}
	})

	t.Run("unknown source vault name is not-found", func(t *testing.T) {
		t.Parallel()
		b := newTestBackend(t)
		if _, err := b.ListRestoreAccessBackupVaults("ghost"); err == nil {
			t.Fatal("expected error for unknown source vault")
		}
	})
}

func TestRestoreAccessVaultRevoke(t *testing.T) {
	t.Parallel()

	t.Run("removes a vault sourced from the given name", func(t *testing.T) {
		t.Parallel()
		b := newTestBackend(t)
		src := mustVault(t, b, "revoke-src")
		rav, err := b.CreateRestoreAccessBackupVault(src.BackupVaultArn, "revoke-rav", "", nil)
		if err != nil {
			t.Fatalf("CreateRestoreAccessBackupVault: %v", err)
		}

		if revokeErr := b.RevokeRestoreAccessBackupVault(
			"revoke-src",
			rav.RestoreAccessBackupVaultArn,
		); revokeErr != nil {
			t.Fatalf("unexpected error: %v", revokeErr)
		}

		remaining, err := b.ListRestoreAccessBackupVaults("revoke-src")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(remaining) != 0 {
			t.Errorf("want 0 remaining restore access vaults, got %d", len(remaining))
		}
	})

	t.Run("mismatched source vault name is not-found (no cross-vault revoke)", func(t *testing.T) {
		t.Parallel()
		b := newTestBackend(t)
		srcA := mustVault(t, b, "revoke-a")
		mustVault(t, b, "revoke-b")
		rav, err := b.CreateRestoreAccessBackupVault(srcA.BackupVaultArn, "revoke-cross", "", nil)
		if err != nil {
			t.Fatalf("CreateRestoreAccessBackupVault: %v", err)
		}

		if revokeErr := b.RevokeRestoreAccessBackupVault(
			"revoke-b",
			rav.RestoreAccessBackupVaultArn,
		); revokeErr == nil {
			t.Fatal("expected error revoking a restore access vault scoped to a different source vault")
		}
	})

	t.Run("unknown ARN is not-found", func(t *testing.T) {
		t.Parallel()
		b := newTestBackend(t)
		mustVault(t, b, "revoke-none")
		if err := b.RevokeRestoreAccessBackupVault(
			"revoke-none",
			"arn:aws:backup:::restore-access-backup-vault:ghost",
		); err == nil {
			t.Fatal("expected error for unknown restore access vault ARN")
		}
	})
}

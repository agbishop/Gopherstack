package backup_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/backup"
)

func TestListRecoveryPointsFiltered(t *testing.T) {
	t.Parallel()
	b := newTestBackend(t)
	mustVault(t, b, "rp-vault")

	now := time.Now().UTC()
	rps := []*backup.RecoveryPoint{
		{
			RecoveryPointArn: "arn:aws:backup:::rp/rp-1",
			BackupVaultName:  "rp-vault",
			ResourceArn:      "arn:aws:ec2:::instance/i-1",
			ResourceType:     "EC2",
			Status:           "COMPLETED",
			CreationDate:     now,
		},
		{
			RecoveryPointArn: "arn:aws:backup:::rp/rp-2",
			BackupVaultName:  "rp-vault",
			ResourceArn:      "arn:aws:rds:::db/db-1",
			ResourceType:     "RDS",
			Status:           "COMPLETED",
			CreationDate:     now.Add(-2 * time.Hour),
		},
		{
			RecoveryPointArn:       "arn:aws:backup:::rp/rp-3",
			BackupVaultName:        "rp-vault",
			ResourceArn:            "arn:aws:ec2:::instance/i-2",
			ResourceType:           "EC2",
			Status:                 "COMPLETED",
			CreationDate:           now.Add(-1 * time.Hour),
			ParentRecoveryPointArn: "arn:aws:backup:::rp/rp-parent",
		},
	}
	for _, rp := range rps {
		if err := b.AddRecoveryPoint(rp.BackupVaultName, rp); err != nil {
			t.Fatalf("AddRecoveryPoint: %v", err)
		}
	}

	createdAfter30m := now.Add(-30 * time.Minute)
	createdBefore30m := now.Add(-30 * time.Minute)

	cases := []struct {
		name      string
		vaultName string
		filter    backup.ListRPFilter
		wantArns  []string
		wantCount int
		wantErr   bool
	}{
		{
			name:      "no filter returns all",
			vaultName: "rp-vault",
			filter:    backup.ListRPFilter{},
			wantCount: 3,
		},
		{
			name:      "filter by EC2",
			vaultName: "rp-vault",
			filter:    backup.ListRPFilter{ResourceType: "EC2"},
			wantCount: 2,
		},
		{
			name:      "filter by RDS",
			vaultName: "rp-vault",
			filter:    backup.ListRPFilter{ResourceType: "RDS"},
			wantCount: 1,
			wantArns:  []string{"arn:aws:backup:::rp/rp-2"},
		},
		{
			name:      "filter by resourceArn",
			vaultName: "rp-vault",
			filter:    backup.ListRPFilter{ResourceArn: "arn:aws:ec2:::instance/i-1"},
			wantCount: 1,
			wantArns:  []string{"arn:aws:backup:::rp/rp-1"},
		},
		{
			name:      "filter by parentRecoveryPointArn",
			vaultName: "rp-vault",
			filter:    backup.ListRPFilter{ParentRecoveryPointArn: "arn:aws:backup:::rp/rp-parent"},
			wantCount: 1,
			wantArns:  []string{"arn:aws:backup:::rp/rp-3"},
		},
		{
			name:      "filter by createdAfter 30m ago returns recent ones",
			vaultName: "rp-vault",
			filter:    backup.ListRPFilter{CreatedAfter: &createdAfter30m},
			wantCount: 1,
			wantArns:  []string{"arn:aws:backup:::rp/rp-1"},
		},
		{
			name:      "filter by createdBefore 30m ago",
			vaultName: "rp-vault",
			filter:    backup.ListRPFilter{CreatedBefore: &createdBefore30m},
			wantCount: 2,
		},
		{
			name:      "nonexistent vault returns not-found error",
			vaultName: "ghost-vault",
			filter:    backup.ListRPFilter{},
			wantErr:   true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, _, err := b.ListRecoveryPointsFiltered(tc.vaultName, tc.filter)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}

				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != tc.wantCount {
				t.Errorf("count: want %d got %d", tc.wantCount, len(got))
			}
			for _, wantArn := range tc.wantArns {
				found := false
				for _, rp := range got {
					if rp.RecoveryPointArn == wantArn {
						found = true

						break
					}
				}
				if !found {
					t.Errorf("expected rp %s in results", wantArn)
				}
			}
		})
	}
}

// ---- ListRecoveryPoints pagination ----

func TestListRecoveryPointsPagination(t *testing.T) {
	t.Parallel()
	b := newTestBackend(t)
	mustVault(t, b, "rp-pg-vault")

	const total = 8
	for i := range total {
		rp := &backup.RecoveryPoint{
			RecoveryPointArn: fmt.Sprintf("arn:aws:backup:::rp/rp-%d", i),
			BackupVaultName:  "rp-pg-vault",
			ResourceArn:      fmt.Sprintf("arn:aws:ec2:::instance/i-%d", i),
			ResourceType:     "EC2",
			Status:           "COMPLETED",
			CreationDate:     time.Now().UTC(),
		}
		if err := b.AddRecoveryPoint("rp-pg-vault", rp); err != nil {
			t.Fatalf("AddRecoveryPoint: %v", err)
		}
	}

	t.Run("paginate all items", func(t *testing.T) {
		t.Parallel()
		var all []*backup.RecoveryPoint
		nextToken := ""
		for {
			got, next, err := b.ListRecoveryPointsFiltered(
				"rp-pg-vault",
				backup.ListRPFilter{MaxResults: 3, NextToken: nextToken},
			)
			if err != nil {
				t.Fatalf("ListRecoveryPointsFiltered: %v", err)
			}
			all = append(all, got...)
			if next == "" {
				break
			}
			nextToken = next
		}
		if len(all) != total {
			t.Errorf("pagination: want %d got %d", total, len(all))
		}
	})
}

// ---- DeleteRecoveryPoint ----

func TestDeleteRecoveryPointVaultLock(t *testing.T) {
	t.Parallel()

	t.Run("unlocked vault allows delete", func(t *testing.T) {
		t.Parallel()
		b := newTestBackend(t)
		mustVault(t, b, "unlocked")
		mustRP(t, b, "unlocked", "arn:aws:backup:::rp/1", "arn:aws:ec2:::instance/i-1", "EC2")

		err := b.DeleteRecoveryPoint("unlocked", "arn:aws:backup:::rp/1")
		require.NoError(t, err)
	})

	t.Run("locked vault blocks delete", func(t *testing.T) {
		t.Parallel()
		b := newTestBackend(t)
		mustVault(t, b, "locked")
		mustRP(t, b, "locked", "arn:aws:backup:::rp/1", "arn:aws:ec2:::instance/i-1", "EC2")
		require.NoError(t, b.PutBackupVaultLockConfiguration("locked", &backup.VaultLockConfig{
			MinRetentionDays: 1,
			MaxRetentionDays: 365,
		}))

		err := b.DeleteRecoveryPoint("locked", "arn:aws:backup:::rp/1")
		require.Error(t, err)

		rp, describeErr := b.DescribeRecoveryPoint("locked", "arn:aws:backup:::rp/1")
		require.NoError(t, describeErr)
		assert.NotNil(t, rp)
	})

	t.Run("delete clears index status ghost row", func(t *testing.T) {
		t.Parallel()
		b := newTestBackend(t)
		mustVault(t, b, "idx-vault")
		mustRP(t, b, "idx-vault", "arn:aws:backup:::rp/1", "arn:aws:ec2:::instance/i-1", "EC2")
		require.NoError(t, b.UpdateRecoveryPointIndexSettings("idx-vault", "arn:aws:backup:::rp/1", "ACTIVE"))

		require.NoError(t, b.DeleteRecoveryPoint("idx-vault", "arn:aws:backup:::rp/1"))

		// UpdateRecoveryPointIndexSettings/GetRecoveryPointIndexDetails key on
		// "vaultName:arn" (colon), not recoveryPointKey's "vaultName#arn" (hash)
		// -- inspect the persisted snapshot directly rather than through
		// GetRecoveryPointIndexDetails, which defaults an absent key to ACTIVE
		// and so can't distinguish "cleared" from "never leaked".
		var probe struct {
			RecoveryPointIndexStatus map[string]string `json:"recoveryPointIndexStatus"`
		}
		require.NoError(t, json.Unmarshal(b.Snapshot(t.Context()), &probe))
		assert.NotContains(t, probe.RecoveryPointIndexStatus, "idx-vault:arn:aws:backup:::rp/1")
	})
}

// ---- DeleteRecoveryPoint / DisassociateRecoveryPoint legal holds ----

func TestRecoveryPointDeletionBlockedByLegalHold(t *testing.T) {
	t.Parallel()

	t.Run("delete blocked by active legal hold", func(t *testing.T) {
		t.Parallel()
		b := newTestBackend(t)
		mustVault(t, b, "held-vault")
		mustRP(t, b, "held-vault", "arn:aws:backup:::rp/1", "arn:aws:ec2:::instance/i-1", "EC2")
		_, err := b.CreateLegalHold("litigation", "desc", nil)
		require.NoError(t, err)

		err = b.DeleteRecoveryPoint("held-vault", "arn:aws:backup:::rp/1")
		require.ErrorIs(t, err, backup.ErrInvalidRequest)

		rp, describeErr := b.DescribeRecoveryPoint("held-vault", "arn:aws:backup:::rp/1")
		require.NoError(t, describeErr)
		assert.NotNil(t, rp)
	})

	t.Run("disassociate blocked by active legal hold", func(t *testing.T) {
		t.Parallel()
		b := newTestBackend(t)
		mustVault(t, b, "held-vault-2")
		mustRP(t, b, "held-vault-2", "arn:aws:backup:::rp/2", "arn:aws:ec2:::instance/i-2", "EC2")
		_, err := b.CreateLegalHold("litigation-2", "desc", nil)
		require.NoError(t, err)

		err = b.DisassociateRecoveryPoint("held-vault-2", "arn:aws:backup:::rp/2")
		require.ErrorIs(t, err, backup.ErrInvalidRequest)
	})

	t.Run("delete succeeds once the covering hold is canceled", func(t *testing.T) {
		t.Parallel()
		b := newTestBackend(t)
		mustVault(t, b, "held-vault-3")
		mustRP(t, b, "held-vault-3", "arn:aws:backup:::rp/3", "arn:aws:ec2:::instance/i-3", "EC2")
		lh, err := b.CreateLegalHold("litigation-3", "desc", nil)
		require.NoError(t, err)
		require.NoError(t, b.CancelLegalHold(lh.LegalHoldID))

		require.NoError(t, b.DeleteRecoveryPoint("held-vault-3", "arn:aws:backup:::rp/3"))
	})

	t.Run("delete unaffected by a hold scoped to a different vault", func(t *testing.T) {
		t.Parallel()
		b := newTestBackend(t)
		mustVault(t, b, "held-vault-4")
		mustRP(t, b, "held-vault-4", "arn:aws:backup:::rp/4", "arn:aws:ec2:::instance/i-4", "EC2")
		_, err := b.CreateLegalHold("litigation-4", "desc", &backup.RecoveryPointSelection{
			VaultNames: []string{"other-vault"},
		})
		require.NoError(t, err)

		require.NoError(t, b.DeleteRecoveryPoint("held-vault-4", "arn:aws:backup:::rp/4"))
	})
}

// ---- ListCopyJobsFiltered ----

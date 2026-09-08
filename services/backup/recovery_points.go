package backup

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
)

// ListRecoveryPointsByBackupVault returns all recovery points for a vault.
func (b *InMemoryBackend) ListRecoveryPointsByBackupVault(
	vaultName string,
) ([]*RecoveryPoint, error) {
	b.mu.RLock("ListRecoveryPointsByBackupVault")
	defer b.mu.RUnlock()

	if !b.vaults.Has(vaultName) {
		return nil, fmt.Errorf("%w: vault %s not found", ErrNotFound, vaultName)
	}

	pts := b.recoveryPointsByVault.Get(vaultName)
	list := make([]*RecoveryPoint, 0, len(pts))
	for _, rp := range pts {
		cp := *rp
		list = append(list, &cp)
	}

	slices.SortFunc(list, func(a, b *RecoveryPoint) int {
		if a.CreationDate.After(b.CreationDate) {
			return -1
		}
		if a.CreationDate.Before(b.CreationDate) {
			return 1
		}

		return 0
	})

	return list, nil
}

// DescribeRecoveryPoint returns a specific recovery point.
func (b *InMemoryBackend) DescribeRecoveryPoint(
	vaultName, recoveryPointArn string,
) (*RecoveryPoint, error) {
	b.mu.RLock("DescribeRecoveryPoint")
	defer b.mu.RUnlock()

	if !b.vaults.Has(vaultName) {
		return nil, fmt.Errorf("%w: vault %s not found", ErrNotFound, vaultName)
	}

	rp, ok := b.recoveryPoints.Get(recoveryPointKey(vaultName, recoveryPointArn))
	if !ok {
		return nil, fmt.Errorf("%w: recovery point %s not found", ErrNotFound, recoveryPointArn)
	}

	cp := *rp

	return &cp, nil
}

// GetRecoveryPointRestoreMetadata returns restore metadata for a recovery point.
func (b *InMemoryBackend) GetRecoveryPointRestoreMetadata(
	vaultName, recoveryPointArn string,
) (map[string]string, error) {
	b.mu.RLock("GetRecoveryPointRestoreMetadata")
	defer b.mu.RUnlock()

	if !b.vaults.Has(vaultName) {
		return nil, fmt.Errorf("%w: vault %s not found", ErrNotFound, vaultName)
	}

	if !b.recoveryPoints.Has(recoveryPointKey(vaultName, recoveryPointArn)) {
		return nil, fmt.Errorf("%w: recovery point %s not found", ErrNotFound, recoveryPointArn)
	}

	return map[string]string{}, nil
}

// recoveryPointUnderActiveLegalHold reports whether rp is covered by any
// active legal hold's RecoveryPointSelection. Caller must already hold b.mu.
// CreateLegalHold (backup@v1.59.4 api_op_CreateLegalHold.go) says any action
// to delete or disassociate a recovery point fails if one or more active
// legal holds are on it.
func (b *InMemoryBackend) recoveryPointUnderActiveLegalHold(rp *RecoveryPoint) bool {
	for _, lh := range b.legalHolds.All() {
		if lh.Status == statusActive && recoveryPointMatchesSelection(rp, lh.RecoveryPointSelection) {
			return true
		}
	}

	return false
}

// DeleteRecoveryPoint deletes a recovery point from a vault.
func (b *InMemoryBackend) DeleteRecoveryPoint(vaultName, recoveryPointArn string) error {
	b.mu.Lock("DeleteRecoveryPoint")
	defer b.mu.Unlock()

	if !b.vaults.Has(vaultName) {
		return fmt.Errorf("%w: vault %s not found", ErrNotFound, vaultName)
	}

	key := recoveryPointKey(vaultName, recoveryPointArn)
	rp, ok := b.recoveryPoints.Get(key)
	if !ok {
		return fmt.Errorf("%w: recovery point %s not found", ErrNotFound, recoveryPointArn)
	}

	// PutBackupVaultLockConfiguration (backup@v1.59.4 api_op_PutBackupVaultLockConfiguration.go):
	// "Applies Backup Vault Lock to a backup vault, preventing attempts to
	// delete any recovery point stored in or created in a backup vault."
	if b.vaultLockConfigs.Has(vaultName) {
		return fmt.Errorf(
			"%w: vault %s is locked; recovery points cannot be deleted",
			ErrInvalidRequest, vaultName,
		)
	}

	if b.recoveryPointUnderActiveLegalHold(rp) {
		return fmt.Errorf(
			"%w: recovery point %s is under an active legal hold",
			ErrInvalidRequest, recoveryPointArn,
		)
	}

	b.recoveryPoints.Delete(key)
	// recoveryPointIndexStatus is a hand-rolled map keyed "vaultName:arn"
	// (not a store.Index), so it needs explicit cleanup here or it grows
	// forever and leaks into Snapshot().
	delete(b.recoveryPointIndexStatus, vaultName+":"+recoveryPointArn)
	if v, found := b.vaults.Get(vaultName); found {
		if v.NumberOfRecoveryPoints > 0 {
			v.NumberOfRecoveryPoints--
		}
	}

	return nil
}

// DisassociateRecoveryPoint disassociates a recovery point from a vault.
func (b *InMemoryBackend) DisassociateRecoveryPoint(vaultName, recoveryPointArn string) error {
	b.mu.Lock("DisassociateRecoveryPoint")
	defer b.mu.Unlock()

	if !b.vaults.Has(vaultName) {
		return fmt.Errorf("%w: vault %s not found", ErrNotFound, vaultName)
	}

	key := recoveryPointKey(vaultName, recoveryPointArn)
	rp, ok := b.recoveryPoints.Get(key)
	if !ok {
		return fmt.Errorf("%w: recovery point %s not found", ErrNotFound, recoveryPointArn)
	}

	// See DeleteRecoveryPoint: CreateLegalHold's doc explicitly names
	// "delete or disassociate" as the blocked actions.
	if b.recoveryPointUnderActiveLegalHold(rp) {
		return fmt.Errorf(
			"%w: recovery point %s is under an active legal hold",
			ErrInvalidRequest, recoveryPointArn,
		)
	}

	b.recoveryPoints.Delete(key)

	return nil
}

// DisassociateRecoveryPointFromParent disassociates a recovery point from its parent.
func (b *InMemoryBackend) DisassociateRecoveryPointFromParent(
	vaultName, recoveryPointArn string,
) error {
	b.mu.Lock("DisassociateRecoveryPointFromParent")
	defer b.mu.Unlock()

	if !b.vaults.Has(vaultName) {
		return fmt.Errorf("%w: vault %s not found", ErrNotFound, vaultName)
	}

	if !b.recoveryPoints.Has(recoveryPointKey(vaultName, recoveryPointArn)) {
		return fmt.Errorf("%w: recovery point %s not found", ErrNotFound, recoveryPointArn)
	}

	return nil
}

// AddRecoveryPoint adds a recovery point to a vault (used internally and in tests).
func (b *InMemoryBackend) AddRecoveryPoint(vaultName string, rp *RecoveryPoint) error {
	b.mu.Lock("AddRecoveryPoint")
	defer b.mu.Unlock()

	vault, ok := b.vaults.Get(vaultName)
	if !ok {
		return fmt.Errorf("%w: vault %s not found", ErrNotFound, vaultName)
	}

	cp := *rp
	cp.BackupVaultName = vaultName
	cp.BackupVaultArn = vault.BackupVaultArn
	b.recoveryPoints.Put(&cp)
	vault.NumberOfRecoveryPoints++

	return nil
}

// --- Vault compliance methods ---

// ListRecoveryPointsByLegalHold returns the recovery points covered by a
// legal hold's RecoveryPointSelection (by vault name, resource ARN, and/or
// creation-date range -- see recoveryPointMatchesSelection). A legal hold
// with no selection (or an all-empty one) covers every recovery point,
// matching AWS's "no additional constraint" semantics. An unknown
// legalHoldID is not an error (ListRecoveryPointsByLegalHold's real error
// list has no ResourceNotFoundException) -- it simply matches nothing.
func (b *InMemoryBackend) ListRecoveryPointsByLegalHold(legalHoldID string) []*RecoveryPoint {
	b.mu.RLock("ListRecoveryPointsByLegalHold")
	defer b.mu.RUnlock()

	lh, ok := b.legalHolds.Get(legalHoldID)
	if !ok {
		return []*RecoveryPoint{}
	}

	all := b.recoveryPoints.All()
	out := make([]*RecoveryPoint, 0, len(all))
	for _, rp := range all {
		if !recoveryPointMatchesSelection(rp, lh.RecoveryPointSelection) {
			continue
		}
		cp := *rp
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RecoveryPointArn < out[j].RecoveryPointArn })

	return out
}

// recoveryPointMatchesSelection reports whether rp is covered by sel. A nil
// selection, or one where every field is empty, matches everything; each
// non-empty field narrows the match (VaultNames/ResourceIdentifiers are
// membership checks, DateRange is an inclusive bound on CreationDate).
func recoveryPointMatchesSelection(rp *RecoveryPoint, sel *RecoveryPointSelection) bool {
	if sel == nil {
		return true
	}
	if len(sel.VaultNames) > 0 && !slices.Contains(sel.VaultNames, rp.BackupVaultName) {
		return false
	}
	if len(sel.ResourceIdentifiers) > 0 && !slices.Contains(sel.ResourceIdentifiers, rp.ResourceArn) {
		return false
	}
	if sel.DateRange != nil {
		if sel.DateRange.FromDate != nil && rp.CreationDate.Before(*sel.DateRange.FromDate) {
			return false
		}
		if sel.DateRange.ToDate != nil && rp.CreationDate.After(*sel.DateRange.ToDate) {
			return false
		}
	}

	return true
}

// ListRecoveryPointsByResource returns recovery points for a given resource ARN across all vaults.
func (b *InMemoryBackend) ListRecoveryPointsByResource(resourceArn string) []*RecoveryPoint {
	b.mu.RLock("ListRecoveryPointsByResource")
	defer b.mu.RUnlock()

	var out []*RecoveryPoint
	for _, rp := range b.recoveryPoints.All() {
		if rp.ResourceArn == resourceArn {
			cp := *rp
			out = append(out, &cp)
		}
	}
	sort.Slice(
		out,
		func(i, j int) bool { return out[i].RecoveryPointArn < out[j].RecoveryPointArn },
	)

	return out
}

// ---- Recovery Point lifecycle ----

// UpdateRecoveryPointLifecycle updates the lifecycle of a recovery point,
// recomputing its CalculatedLifecycle transition timestamps from CreationDate.
func (b *InMemoryBackend) UpdateRecoveryPointLifecycle(
	vaultName, recoveryPointArn string,
	moveToColdStorageAfterDays, deleteAfterDays int64,
) error {
	b.mu.Lock("UpdateRecoveryPointLifecycle")
	defer b.mu.Unlock()

	if !b.vaults.Has(vaultName) {
		return fmt.Errorf("%w: vault %s not found", ErrNotFound, vaultName)
	}

	rp, ok := b.recoveryPoints.Get(recoveryPointKey(vaultName, recoveryPointArn))
	if !ok {
		return fmt.Errorf("%w: recovery point %s not found", ErrNotFound, recoveryPointArn)
	}

	lc := &Lifecycle{
		MoveToColdStorageAfterDays: moveToColdStorageAfterDays,
		DeleteAfterDays:            deleteAfterDays,
	}
	rp.Lifecycle = lc
	rp.CalculatedLifecycle = calculateLifecycle(lc, rp.CreationDate)

	return nil
}

// calculateLifecycle derives CalculatedLifecycle transition timestamps for a
// recovery point's lifecycle policy, measured from the given reference time
// (normally the recovery point's CreationDate). Returns nil if lc is nil.
func calculateLifecycle(lc *Lifecycle, from time.Time) *CalculatedLifecycle {
	if lc == nil {
		return nil
	}

	cl := &CalculatedLifecycle{}
	if lc.MoveToColdStorageAfterDays > 0 {
		t := from.AddDate(0, 0, int(lc.MoveToColdStorageAfterDays))
		cl.MoveToColdStorageAt = &t
	}
	if lc.DeleteAfterDays > 0 {
		t := from.AddDate(0, 0, int(lc.DeleteAfterDays))
		cl.DeleteAt = &t
	}

	return cl
}

// GetRecoveryPointIndexDetails returns index details for a recovery point.
func (b *InMemoryBackend) GetRecoveryPointIndexDetails(
	vaultName, recoveryPointArn string,
) (string, error) {
	b.mu.RLock("GetRecoveryPointIndexDetails")
	defer b.mu.RUnlock()

	key := vaultName + ":" + recoveryPointArn
	status := b.recoveryPointIndexStatus[key]
	if status == "" {
		status = statusActive
	}

	return status, nil
}

// UpdateRecoveryPointIndexSettings updates indexing settings for a recovery point.
func (b *InMemoryBackend) UpdateRecoveryPointIndexSettings(
	vaultName, recoveryPointArn, indexStatus string,
) error {
	b.mu.Lock("UpdateRecoveryPointIndexSettings")
	defer b.mu.Unlock()

	key := vaultName + ":" + recoveryPointArn
	b.recoveryPointIndexStatus[key] = indexStatus

	return nil
}

// ListIndexedRecoveryPoints returns recovery points with an active index.
func (b *InMemoryBackend) ListIndexedRecoveryPoints() []*RecoveryPoint {
	b.mu.RLock("ListIndexedRecoveryPoints")
	defer b.mu.RUnlock()

	var out []*RecoveryPoint //nolint:prealloc // preserves nil (not empty-slice) return when no recovery points exist
	for _, rp := range b.recoveryPoints.All() {
		cp := *rp
		out = append(out, &cp)
	}
	sort.Slice(
		out,
		func(i, j int) bool { return out[i].RecoveryPointArn < out[j].RecoveryPointArn },
	)

	return out
}

// ---- Job operations ----

// ListRPFilter contains optional filter parameters for listing recovery points.
type ListRPFilter struct {
	CreatedAfter           *time.Time
	CreatedBefore          *time.Time
	ResourceArn            string
	ResourceType           string
	ParentRecoveryPointArn string
	NextToken              string
	MaxResults             int
}

// rpMatchesFilter reports whether rp satisfies all active fields in f.
func rpMatchesFilter(rp *RecoveryPoint, f ListRPFilter) bool {
	if f.ResourceArn != "" && rp.ResourceArn != f.ResourceArn {
		return false
	}
	if f.ResourceType != "" && rp.ResourceType != f.ResourceType {
		return false
	}
	if f.ParentRecoveryPointArn != "" && rp.ParentRecoveryPointArn != f.ParentRecoveryPointArn {
		return false
	}
	if f.CreatedAfter != nil && !rp.CreationDate.After(*f.CreatedAfter) {
		return false
	}
	if f.CreatedBefore != nil && !rp.CreationDate.Before(*f.CreatedBefore) {
		return false
	}

	return true
}

// ListRecoveryPointsFiltered returns recovery points for a vault with optional filters and pagination.
func (b *InMemoryBackend) ListRecoveryPointsFiltered(
	vaultName string,
	f ListRPFilter,
) ([]*RecoveryPoint, string, error) {
	b.mu.RLock("ListRecoveryPointsFiltered")
	defer b.mu.RUnlock()

	if !b.vaults.Has(vaultName) {
		return nil, "", fmt.Errorf("%w: vault %s not found", ErrNotFound, vaultName)
	}

	pts := b.recoveryPointsByVault.Get(vaultName)
	list := make([]*RecoveryPoint, 0, len(pts))
	for _, rp := range pts {
		if !rpMatchesFilter(rp, f) {
			continue
		}
		cp := *rp
		list = append(list, &cp)
	}

	slices.SortFunc(list, func(a, b *RecoveryPoint) int {
		if d := b.CreationDate.Compare(a.CreationDate); d != 0 {
			return d
		}

		return strings.Compare(a.RecoveryPointArn, b.RecoveryPointArn)
	})

	page, token := paginateByID(
		list,
		func(rp *RecoveryPoint) string { return rp.RecoveryPointArn },
		f.MaxResults,
		f.NextToken,
	)

	return page, token, nil
}

// ---- ListCopyJobs filtering + pagination ----

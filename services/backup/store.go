package backup

import (
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// NewInMemoryBackend creates a new in-memory Backup backend.
func NewInMemoryBackend(accountID, region string) *InMemoryBackend {
	b := &InMemoryBackend{
		registry:                   store.NewRegistry(),
		mpaApprovals:               make(map[string]string),
		vaultARNIndex:              make(map[string]string),
		restoreAccessVaultARNIndex: make(map[string]string),
		planARNIndex:               make(map[string]string),
		planIDIndex:                make(map[string]string),
		frameworkARNIndex:          make(map[string]string),
		reportPlanARNIndex:         make(map[string]string),
		globalSettings:             make(map[string]string),
		recoveryPointIndexStatus:   make(map[string]string),
		accountID:                  accountID,
		region:                     region,
		mu:                         lockmetrics.New("backup"),
	}

	registerAllTables(b)

	return b
}

// SetS3Backend wires S3 so StartBackupJob validates an S3-typed ResourceArn
// names a bucket that actually exists, instead of accepting any non-empty
// ResourceArn regardless of whether it resolves to a real resource
// (gopherstack-0o0q).
func (b *InMemoryBackend) SetS3Backend(s3 S3Backend) {
	b.s3 = s3
}

// Region returns the AWS region this backend is configured for.
func (b *InMemoryBackend) Region() string { return b.region }

// AccountID returns the AWS account ID this backend is configured for.
func (b *InMemoryBackend) AccountID() string { return b.accountID }

// Reset clears all state, returning the backend to a clean initial state.
// Tags resources are properly closed before discarding.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	for _, v := range b.vaults.All() {
		if v.Tags != nil {
			v.Tags.Close()
		}
	}

	for _, p := range b.plans.All() {
		if p.Tags != nil {
			p.Tags.Close()
		}
	}

	for _, f := range b.frameworks.All() {
		if f.Tags != nil {
			f.Tags.Close()
		}
	}

	for _, rp := range b.reportPlans.All() {
		if rp.Tags != nil {
			rp.Tags.Close()
		}
	}

	for _, rav := range b.restoreAccessVaults.All() {
		if rav.Tags != nil {
			rav.Tags.Close()
		}
	}

	// Resets every table (and index) registered in store_setup.go.
	b.registry.ResetAll()

	b.mpaApprovals = make(map[string]string)
	b.vaultARNIndex = make(map[string]string)
	b.restoreAccessVaultARNIndex = make(map[string]string)
	b.planARNIndex = make(map[string]string)
	b.planIDIndex = make(map[string]string)
	b.frameworkARNIndex = make(map[string]string)
	b.reportPlanARNIndex = make(map[string]string)
	b.globalSettings = make(map[string]string)
	b.globalSettingsLastUpdate = time.Time{}
	b.regionSettings = nil
	b.recoveryPointIndexStatus = make(map[string]string)
}

// --- Recovery Point methods ---

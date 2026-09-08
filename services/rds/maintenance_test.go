package rds_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/rds"
)

// TestRDSBackend_ApplyPendingMaintenanceAction tests ApplyPendingMaintenanceAction.
func TestRDSBackend_ApplyPendingMaintenanceAction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErrIs   error
		setup       func(b *rds.InMemoryBackend)
		name        string
		resourceID  string
		applyAction string
		optInType   string
		wantErr     bool
	}{
		{
			name: "success_for_instance",
			setup: func(b *rds.InMemoryBackend) {
				_, _ = b.CreateDBInstance("my-db", "postgres", "", "", "", "", 20, rds.DBInstanceOptions{})
			},
			resourceID:  "my-db",
			applyAction: "system-update",
			optInType:   "immediate",
		},
		{
			name: "success_for_cluster",
			setup: func(b *rds.InMemoryBackend) {
				_, _ = b.CreateDBCluster("my-cluster", "aurora-postgresql", "", "", "", 0, nil, rds.DBClusterOptions{})
			},
			resourceID:  "my-cluster",
			applyAction: "system-update",
			optInType:   "next-maintenance",
		},
		{
			name:        "resource_not_found",
			setup:       func(_ *rds.InMemoryBackend) {},
			resourceID:  "no-such-resource",
			applyAction: "system-update",
			optInType:   "immediate",
			wantErr:     true,
			wantErrIs:   rds.ErrResourceNotFound,
		},
		{
			name:        "empty_resource_id",
			setup:       func(_ *rds.InMemoryBackend) {},
			resourceID:  "",
			applyAction: "system-update",
			optInType:   "immediate",
			wantErr:     true,
			wantErrIs:   rds.ErrInvalidParameter,
		},
		{
			name: "empty_apply_action",
			setup: func(b *rds.InMemoryBackend) {
				_, _ = b.CreateDBInstance("my-db", "postgres", "", "", "", "", 20, rds.DBInstanceOptions{})
			},
			resourceID:  "my-db",
			applyAction: "",
			optInType:   "immediate",
			wantErr:     true,
			wantErrIs:   rds.ErrInvalidParameter,
		},
		{
			name: "empty_opt_in_type",
			setup: func(b *rds.InMemoryBackend) {
				_, _ = b.CreateDBInstance("my-db", "postgres", "", "", "", "", 20, rds.DBInstanceOptions{})
			},
			resourceID:  "my-db",
			applyAction: "system-update",
			optInType:   "",
			wantErr:     true,
			wantErrIs:   rds.ErrInvalidParameter,
		},
		{
			name: "invalid_opt_in_type",
			setup: func(b *rds.InMemoryBackend) {
				_, _ = b.CreateDBInstance("my-db", "postgres", "", "", "", "", 20, rds.DBInstanceOptions{})
			},
			resourceID:  "my-db",
			applyAction: "system-update",
			optInType:   "whenever-i-feel-like-it",
			wantErr:     true,
			wantErrIs:   rds.ErrInvalidParameter,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := rds.NewInMemoryBackend("000000000000", "us-east-1")
			tt.setup(b)

			result, err := b.ApplyPendingMaintenanceAction(tt.resourceID, tt.applyAction, tt.optInType)

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErrIs)

				return
			}

			require.NoError(t, err)
			assert.NotEmpty(t, result)
		})
	}
}

func TestDescribePendingMaintenanceActions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		resourceARN string
		wantCount   int
	}{
		{name: "all empty", resourceARN: "", wantCount: 0},
		{name: "specific resource empty", resourceARN: "arn:aws:rds:us-east-1:123456789012:db:my-db", wantCount: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newTestBackend(t)
			got := b.DescribePendingMaintenanceActions(tt.resourceARN)
			assert.Len(t, got, tt.wantCount)
		})
	}
}

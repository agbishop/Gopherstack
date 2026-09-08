package redshift_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/redshift"
)

// ---- AuthorizeSnapshotAccess ----

func TestHandler_AuthorizeSnapshotAccess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(_ *testing.T, _ *redshift.Handler, b *redshift.InMemoryBackend)
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "success",
			setup: func(_ *testing.T, _ *redshift.Handler, b *redshift.InMemoryBackend) {
				b.AddSnapshotInternal(&redshift.Snapshot{
					SnapshotIdentifier: "snap-001",
					ClusterIdentifier:  "my-cluster",
					Status:             "available",
				})
			},
			body: "Action=AuthorizeSnapshotAccess&Version=2012-12-01" +
				"&SnapshotIdentifier=snap-001&AccountWithRestoreAccess=111111111111",
			wantCode:     http.StatusOK,
			wantContains: []string{"AuthorizeSnapshotAccessResponse", "snap-001", "111111111111"},
		},
		{
			name:         "missing_snapshot_identifier",
			body:         "Action=AuthorizeSnapshotAccess&Version=2012-12-01&AccountWithRestoreAccess=111111111111",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		{
			name:         "missing_account",
			body:         "Action=AuthorizeSnapshotAccess&Version=2012-12-01&SnapshotIdentifier=snap-001",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		{
			name: "snapshot_not_found",
			body: "Action=AuthorizeSnapshotAccess&Version=2012-12-01" +
				"&SnapshotIdentifier=nonexistent&AccountWithRestoreAccess=111111111111",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ClusterSnapshotNotFound"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
			h := redshift.NewHandler(b)
			if tt.setup != nil {
				tt.setup(t, h, b)
			}

			rec := postRedshiftForm(t, h, tt.body)

			assert.Equal(t, tt.wantCode, rec.Code)
			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

// ---- BatchDeleteClusterSnapshots ----

func TestHandler_BatchDeleteClusterSnapshots(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(_ *testing.T, _ *redshift.Handler, b *redshift.InMemoryBackend)
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "success_all_deleted",
			setup: func(_ *testing.T, _ *redshift.Handler, b *redshift.InMemoryBackend) {
				b.AddSnapshotInternal(
					&redshift.Snapshot{SnapshotIdentifier: "snap-del-1", ClusterIdentifier: "c1", Status: "available"},
				)
				b.AddSnapshotInternal(
					&redshift.Snapshot{SnapshotIdentifier: "snap-del-2", ClusterIdentifier: "c1", Status: "available"},
				)
			},
			body: "Action=BatchDeleteClusterSnapshots&Version=2012-12-01" +
				"&Identifiers.DeleteClusterSnapshotMessage.1.SnapshotIdentifier=snap-del-1" +
				"&Identifiers.DeleteClusterSnapshotMessage.2.SnapshotIdentifier=snap-del-2",
			wantCode:     http.StatusOK,
			wantContains: []string{"BatchDeleteClusterSnapshotsResponse", "snap-del-1", "snap-del-2"},
		},
		{
			name: "partial_success_with_errors",
			setup: func(_ *testing.T, _ *redshift.Handler, b *redshift.InMemoryBackend) {
				b.AddSnapshotInternal(
					&redshift.Snapshot{SnapshotIdentifier: "snap-del-3", ClusterIdentifier: "c1", Status: "available"},
				)
			},
			body: "Action=BatchDeleteClusterSnapshots&Version=2012-12-01" +
				"&Identifiers.DeleteClusterSnapshotMessage.1.SnapshotIdentifier=snap-del-3" +
				"&Identifiers.DeleteClusterSnapshotMessage.2.SnapshotIdentifier=nonexistent",
			wantCode:     http.StatusOK,
			wantContains: []string{"BatchDeleteClusterSnapshotsResponse", "ClusterSnapshotNotFound"},
		},
		{
			name:         "empty_list",
			body:         "Action=BatchDeleteClusterSnapshots&Version=2012-12-01",
			wantCode:     http.StatusOK,
			wantContains: []string{"BatchDeleteClusterSnapshotsResponse"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
			h := redshift.NewHandler(b)
			if tt.setup != nil {
				tt.setup(t, h, b)
			}

			rec := postRedshiftForm(t, h, tt.body)

			assert.Equal(t, tt.wantCode, rec.Code)
			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

// ---- BatchModifyClusterSnapshots ----

func TestHandler_BatchModifyClusterSnapshots(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(_ *testing.T, _ *redshift.Handler, b *redshift.InMemoryBackend)
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "success",
			setup: func(_ *testing.T, _ *redshift.Handler, b *redshift.InMemoryBackend) {
				b.AddSnapshotInternal(
					&redshift.Snapshot{SnapshotIdentifier: "snap-mod-1", ClusterIdentifier: "c1", Status: "available"},
				)
				b.AddSnapshotInternal(
					&redshift.Snapshot{SnapshotIdentifier: "snap-mod-2", ClusterIdentifier: "c1", Status: "available"},
				)
			},
			body: "Action=BatchModifyClusterSnapshots&Version=2012-12-01" +
				"&SnapshotIdentifierList.String.1=snap-mod-1&SnapshotIdentifierList.String.2=snap-mod-2" +
				"&ManualSnapshotRetentionPeriod=7",
			wantCode:     http.StatusOK,
			wantContains: []string{"BatchModifyClusterSnapshotsResponse"},
		},
		{
			name: "partial_with_errors",
			setup: func(_ *testing.T, _ *redshift.Handler, b *redshift.InMemoryBackend) {
				b.AddSnapshotInternal(
					&redshift.Snapshot{SnapshotIdentifier: "snap-mod-3", ClusterIdentifier: "c1", Status: "available"},
				)
			},
			body: "Action=BatchModifyClusterSnapshots&Version=2012-12-01" +
				"&SnapshotIdentifierList.String.1=snap-mod-3&SnapshotIdentifierList.String.2=nonexistent" +
				"&ManualSnapshotRetentionPeriod=14",
			wantCode:     http.StatusOK,
			wantContains: []string{"BatchModifyClusterSnapshotsResponse", "ClusterSnapshotNotFound"},
		},
		{
			name: "invalid_retention_period",
			setup: func(_ *testing.T, _ *redshift.Handler, b *redshift.InMemoryBackend) {
				b.AddSnapshotInternal(
					&redshift.Snapshot{SnapshotIdentifier: "snap-mod-4", ClusterIdentifier: "c1", Status: "available"},
				)
			},
			body: "Action=BatchModifyClusterSnapshots&Version=2012-12-01" +
				"&SnapshotIdentifierList.String.1=snap-mod-4&ManualSnapshotRetentionPeriod=notanumber",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		{
			name: "with_force_flag",
			setup: func(_ *testing.T, _ *redshift.Handler, b *redshift.InMemoryBackend) {
				b.AddSnapshotInternal(
					&redshift.Snapshot{SnapshotIdentifier: "snap-mod-5", ClusterIdentifier: "c1", Status: "available"},
				)
			},
			body: "Action=BatchModifyClusterSnapshots&Version=2012-12-01" +
				"&SnapshotIdentifierList.String.1=snap-mod-5&ManualSnapshotRetentionPeriod=30&Force=true",
			wantCode:     http.StatusOK,
			wantContains: []string{"BatchModifyClusterSnapshotsResponse"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
			h := redshift.NewHandler(b)
			if tt.setup != nil {
				tt.setup(t, h, b)
			}

			rec := postRedshiftForm(t, h, tt.body)

			assert.Equal(t, tt.wantCode, rec.Code)
			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

func TestBackend_BatchDeleteAndModifySnapshots(t *testing.T) {
	t.Parallel()

	t.Run("batch_delete_all_found", func(t *testing.T) {
		t.Parallel()

		b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
		b.AddSnapshotInternal(
			&redshift.Snapshot{SnapshotIdentifier: "s1", ClusterIdentifier: "c1", Status: "available"},
		)
		b.AddSnapshotInternal(
			&redshift.Snapshot{SnapshotIdentifier: "s2", ClusterIdentifier: "c1", Status: "available"},
		)

		errs, deleted := b.BatchDeleteClusterSnapshots([]string{"s1", "s2"})
		assert.Empty(t, errs)
		assert.ElementsMatch(t, []string{"s1", "s2"}, deleted)
	})

	t.Run("batch_delete_partial_not_found", func(t *testing.T) {
		t.Parallel()

		b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
		b.AddSnapshotInternal(
			&redshift.Snapshot{SnapshotIdentifier: "s3", ClusterIdentifier: "c1", Status: "available"},
		)

		errs, deleted := b.BatchDeleteClusterSnapshots([]string{"s3", "missing"})
		assert.Len(t, errs, 1)
		assert.Equal(t, "missing", errs[0].SnapshotIdentifier)
		assert.Equal(t, []string{"s3"}, deleted)
	})

	t.Run("batch_modify_all_found", func(t *testing.T) {
		t.Parallel()

		b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
		b.AddSnapshotInternal(
			&redshift.Snapshot{SnapshotIdentifier: "m1", ClusterIdentifier: "c1", Status: "available"},
		)
		b.AddSnapshotInternal(
			&redshift.Snapshot{SnapshotIdentifier: "m2", ClusterIdentifier: "c1", Status: "available"},
		)

		retention := 7
		errs, modified := b.BatchModifyClusterSnapshots([]string{"m1", "m2"}, &retention, false)
		assert.Empty(t, errs)
		assert.ElementsMatch(t, []string{"m1", "m2"}, modified)
	})

	t.Run("batch_modify_partial_not_found", func(t *testing.T) {
		t.Parallel()

		b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
		b.AddSnapshotInternal(
			&redshift.Snapshot{SnapshotIdentifier: "m3", ClusterIdentifier: "c1", Status: "available"},
		)

		retention := 14
		errs, modified := b.BatchModifyClusterSnapshots([]string{"m3", "missing"}, &retention, true)
		assert.Len(t, errs, 1)
		assert.Equal(t, "missing", errs[0].SnapshotIdentifier)
		assert.Equal(t, []string{"m3"}, modified)
	})
}

// ---- BatchDeleteClusterSnapshots partial success ----

func TestBatchDeleteClusterSnapshots_PartialSuccess(t *testing.T) {
	t.Parallel()

	b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	h := redshift.NewHandler(b)

	b.AddSnapshotInternal(
		&redshift.Snapshot{SnapshotIdentifier: "snap-good", ClusterIdentifier: "c1", Status: "available"},
	)

	rec := postRedshiftForm(t, h,
		"Action=BatchDeleteClusterSnapshots&Version=2012-12-01"+
			"&Identifiers.DeleteClusterSnapshotMessage.1.SnapshotIdentifier=snap-good"+
			"&Identifiers.DeleteClusterSnapshotMessage.2.SnapshotIdentifier=snap-missing")

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "snap-good")
	assert.Contains(t, body, "snap-missing")
	assert.Contains(t, body, "ClusterSnapshotNotFound")

	// snap-good should be deleted
	assert.Equal(t, 0, redshift.SnapshotCount(b))
}

// ---- BatchModifyClusterSnapshots bad retention period ----

func TestBatchModifyClusterSnapshots_InvalidRetentionPeriod(t *testing.T) {
	t.Parallel()

	h := newRedshiftHandler()
	rec := postRedshiftForm(t, h,
		"Action=BatchModifyClusterSnapshots&Version=2012-12-01"+
			"&SnapshotIdentifierList.String.1=snap-1"+
			"&ManualSnapshotRetentionPeriod=not-a-number")

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "InvalidParameterValue")
}

// ---- BatchModifyClusterSnapshots success ----

func TestBatchModifyClusterSnapshots_Success(t *testing.T) {
	t.Parallel()

	b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	h := redshift.NewHandler(b)

	b.AddSnapshotInternal(
		&redshift.Snapshot{SnapshotIdentifier: "mod-snap-1", ClusterIdentifier: "c1", Status: "available"},
	)
	b.AddSnapshotInternal(
		&redshift.Snapshot{SnapshotIdentifier: "mod-snap-2", ClusterIdentifier: "c1", Status: "available"},
	)

	rec := postRedshiftForm(t, h,
		"Action=BatchModifyClusterSnapshots&Version=2012-12-01"+
			"&SnapshotIdentifierList.String.1=mod-snap-1"+
			"&SnapshotIdentifierList.String.2=mod-snap-2"+
			"&ManualSnapshotRetentionPeriod=14")

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "BatchModifyClusterSnapshotsResponse")
	assert.Contains(t, body, "mod-snap-1")
	assert.Contains(t, body, "mod-snap-2")
}

// ---- AuthorizeSnapshotAccess duplicate account ----

// TestAuthorizeSnapshotAccess_DuplicateAccount: this op's own declared error
// switch (redshift@v1.65.4 deserializers.go,
// awsAwsquery_deserializeOpErrorAuthorizeSnapshotAccess) lists
// AuthorizationAlreadyExists -- the same fault AuthorizeEndpointAccess's own
// sibling test (TestAuthorizeEndpointAccess_DuplicateReturnsError) already
// asserts for the identical re-grant condition -- so re-authorizing an
// account that already has restore access must error, not silently add a
// second entry. Previously asserted the opposite ("AWS allows multiple
// accounts"), a claim never checked against the SDK error switch.
func TestAuthorizeSnapshotAccess_DuplicateAccount(t *testing.T) {
	t.Parallel()

	b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	h := redshift.NewHandler(b)

	b.AddSnapshotInternal(
		&redshift.Snapshot{SnapshotIdentifier: "snap-dup", ClusterIdentifier: "c1", Status: "available"},
	)

	body := "Action=AuthorizeSnapshotAccess&Version=2012-12-01" +
		"&SnapshotIdentifier=snap-dup" +
		"&AccountWithRestoreAccess=111111111111"

	rec1 := postRedshiftForm(t, h, body)
	require.Equal(t, http.StatusOK, rec1.Code)
	assert.Contains(t, rec1.Body.String(), "111111111111")

	rec2 := postRedshiftForm(t, h, body)
	assert.Equal(t, http.StatusBadRequest, rec2.Code)
	assert.Contains(t, rec2.Body.String(), "AuthorizationAlreadyExists")
}

// ---- AuthorizeSnapshotAccess: snapshot not found ----

func TestAuthorizeSnapshotAccess_SnapshotNotFound(t *testing.T) {
	t.Parallel()

	h := newRedshiftHandler()
	rec := postRedshiftForm(t, h,
		"Action=AuthorizeSnapshotAccess&Version=2012-12-01"+
			"&SnapshotIdentifier=nonexistent"+
			"&AccountWithRestoreAccess=111111111111")

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "ClusterSnapshotNotFound")
}

// ---- RevokeSnapshotAccess ----

func TestHandler_RevokeSnapshotAccess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(t *testing.T, h *redshift.Handler)
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "success",
			setup: func(t *testing.T, h *redshift.Handler) {
				t.Helper()
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=rsa-cluster")
				postRedshiftForm(
					t,
					h,
					"Action=CreateClusterSnapshot&Version=2012-12-01&SnapshotIdentifier=snap-rsa&ClusterIdentifier=rsa-cluster",
				)
				postRedshiftForm(
					t,
					h,
					"Action=AuthorizeSnapshotAccess&Version=2012-12-01&SnapshotIdentifier=snap-rsa&AccountWithRestoreAccess=acc-rsa",
				)
			},
			body: "Action=RevokeSnapshotAccess&Version=2012-12-01" +
				"&SnapshotIdentifier=snap-rsa&AccountWithRestoreAccess=acc-rsa",
			wantCode:     http.StatusOK,
			wantContains: []string{"RevokeSnapshotAccessResponse", "snap-rsa"},
		},
		{
			name:     "missing_snapshot_id",
			body:     "Action=RevokeSnapshotAccess&Version=2012-12-01&SnapshotIdentifier=&AccountWithRestoreAccess=acc-rsa",
			wantCode: http.StatusBadRequest,
		},
		{
			name: "snapshot_not_found",
			body: "Action=RevokeSnapshotAccess&Version=2012-12-01" +
				"&SnapshotIdentifier=missing&AccountWithRestoreAccess=acc-rsa",
			wantCode: http.StatusBadRequest,
		},
		{
			name: "account_not_found",
			setup: func(t *testing.T, h *redshift.Handler) {
				t.Helper()
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=rsa-cluster2")
				postRedshiftForm(
					t,
					h,
					"Action=CreateClusterSnapshot&Version=2012-12-01&SnapshotIdentifier=snap-rsa2&ClusterIdentifier=rsa-cluster2",
				)
			},
			body: "Action=RevokeSnapshotAccess&Version=2012-12-01" +
				"&SnapshotIdentifier=snap-rsa2&AccountWithRestoreAccess=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"AuthorizationNotFound"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newRedshiftHandler()
			if tt.setup != nil {
				tt.setup(t, h)
			}

			rec := postRedshiftForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

// ---- ModifyClusterSnapshot ----

func TestHandler_ModifyClusterSnapshot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(t *testing.T, h *redshift.Handler)
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "success",
			setup: func(t *testing.T, h *redshift.Handler) {
				t.Helper()
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=mcs-cluster")
				postRedshiftForm(
					t,
					h,
					"Action=CreateClusterSnapshot&Version=2012-12-01&SnapshotIdentifier=snap-mcs&ClusterIdentifier=mcs-cluster",
				)
			},
			body: "Action=ModifyClusterSnapshot&Version=2012-12-01" +
				"&SnapshotIdentifier=snap-mcs&ManualSnapshotRetentionPeriod=14",
			wantCode:     http.StatusOK,
			wantContains: []string{"ModifyClusterSnapshotResponse", "snap-mcs", "14"},
		},
		{
			name:     "missing_snapshot_id",
			body:     "Action=ModifyClusterSnapshot&Version=2012-12-01&SnapshotIdentifier=",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "snapshot_not_found",
			body:     "Action=ModifyClusterSnapshot&Version=2012-12-01&SnapshotIdentifier=missing",
			wantCode: http.StatusBadRequest,
		},
		{
			name: "invalid_retention",
			body: "Action=ModifyClusterSnapshot&Version=2012-12-01" +
				"&SnapshotIdentifier=snap-mcs&ManualSnapshotRetentionPeriod=notanumber",
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newRedshiftHandler()
			if tt.setup != nil {
				tt.setup(t, h)
			}

			rec := postRedshiftForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

// ---- Backend.RevokeSnapshotAccess ----

func TestBackend_RevokeSnapshotAccess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(b *redshift.InMemoryBackend)
		wantErrIs  error
		name       string
		snapshotID string
		accountID  string
		wantErr    bool
	}{
		{
			name: "success",
			setup: func(b *redshift.InMemoryBackend) {
				_, _ = b.CreateCluster("c1", "dc2.large", "dev", "admin", nil, "")
				_, _ = b.CreateClusterSnapshot("snap1", "c1")
				_, _ = b.AuthorizeSnapshotAccess("snap1", "acc1")
			},
			snapshotID: "snap1",
			accountID:  "acc1",
			wantErr:    false,
		},
		{
			name:       "missing_snapshot_id",
			snapshotID: "",
			accountID:  "acc1",
			wantErr:    true,
			wantErrIs:  redshift.ErrInvalidParameter,
		},
		{
			name:       "missing_account_id",
			snapshotID: "snap1",
			accountID:  "",
			wantErr:    true,
			wantErrIs:  redshift.ErrInvalidParameter,
		},
		{
			name:       "snapshot_not_found",
			snapshotID: "missing",
			accountID:  "acc1",
			wantErr:    true,
			wantErrIs:  redshift.ErrSnapshotNotFound,
		},
		{
			name: "account_not_found",
			setup: func(b *redshift.InMemoryBackend) {
				_, _ = b.CreateCluster("c1", "dc2.large", "dev", "admin", nil, "")
				_, _ = b.CreateClusterSnapshot("snap2", "c1")
			},
			snapshotID: "snap2",
			accountID:  "nonexistent",
			wantErr:    true,
			wantErrIs:  redshift.ErrSnapshotAccessNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
			if tt.setup != nil {
				tt.setup(b)
			}

			snap, err := b.RevokeSnapshotAccess(tt.snapshotID, tt.accountID)

			if tt.wantErr {
				require.Error(t, err)

				if tt.wantErrIs != nil {
					require.ErrorIs(t, err, tt.wantErrIs)
				}

				return
			}

			require.NoError(t, err)
			assert.Empty(t, snap.AccountsWithRestoreAccess)
		})
	}
}

// ---- Backend.ModifyClusterSnapshot ----

func TestBackend_ModifyClusterSnapshot(t *testing.T) {
	t.Parallel()

	thirty := 30
	minusOne := -1

	tests := []struct {
		setup           func(b *redshift.InMemoryBackend)
		retentionPeriod *int
		name            string
		snapshotID      string
		wantRetention   int
		wantErr         bool
	}{
		{
			name: "success",
			setup: func(b *redshift.InMemoryBackend) {
				_, _ = b.CreateCluster("c1", "dc2.large", "dev", "admin", nil, "")
				_, _ = b.CreateClusterSnapshot("snap1", "c1")
			},
			snapshotID:      "snap1",
			retentionPeriod: &thirty,
			wantErr:         false,
			wantRetention:   30,
		},
		{
			name:       "missing_snapshot_id",
			snapshotID: "",
			wantErr:    true,
		},
		{
			name:       "snapshot_not_found",
			snapshotID: "missing",
			wantErr:    true,
		},
		{
			name: "omitted_retention_period_leaves_existing_value_unchanged",
			setup: func(b *redshift.InMemoryBackend) {
				_, _ = b.CreateCluster("c2", "dc2.large", "dev", "admin", nil, "")
				_, _ = b.CreateClusterSnapshot("snap2", "c2")

				retained := 30
				_, err := b.ModifyClusterSnapshot("snap2", &retained, false)
				require.NoError(t, err)
			},
			snapshotID:      "snap2",
			retentionPeriod: nil,
			wantErr:         false,
			wantRetention:   30,
		},
		{
			name: "explicit_negative_one_sets_indefinite_retention",
			setup: func(b *redshift.InMemoryBackend) {
				_, _ = b.CreateCluster("c3", "dc2.large", "dev", "admin", nil, "")
				_, _ = b.CreateClusterSnapshot("snap3", "c3")

				retained := 30
				_, err := b.ModifyClusterSnapshot("snap3", &retained, false)
				require.NoError(t, err)
			},
			snapshotID:      "snap3",
			retentionPeriod: &minusOne,
			wantErr:         false,
			wantRetention:   -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
			if tt.setup != nil {
				tt.setup(b)
			}

			snap, err := b.ModifyClusterSnapshot(tt.snapshotID, tt.retentionPeriod, false)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantRetention, snap.ManualSnapshotRetentionPeriod)
		})
	}
}

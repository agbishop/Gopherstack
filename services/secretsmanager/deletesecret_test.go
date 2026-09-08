package secretsmanager_test

// deletesecret_test.go consolidates every DeleteSecret-specific test that was
// previously scattered across several older test files. Ported verbatim
// (assertions unchanged).

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/secretsmanager"
)

// ---------------------------------------------------------------------------
// DeleteSecret comprehensive
// ---------------------------------------------------------------------------

func TestDeleteSecret_SoftDelete(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	_, err := b.CreateSecret(
		context.Background(),
		&secretsmanager.CreateSecretInput{Name: "soft-del", SecretString: "v"},
	)
	require.NoError(t, err)

	out, err := b.DeleteSecret(context.Background(), &secretsmanager.DeleteSecretInput{SecretID: "soft-del"})
	require.NoError(t, err)
	assert.NotEmpty(t, out.ARN)
	assert.NotZero(t, out.DeletionDate)

	// Secret still findable but marked deleted
	desc, err := b.DescribeSecret(context.Background(), &secretsmanager.DescribeSecretInput{SecretID: "soft-del"})
	require.NoError(t, err)
	assert.NotNil(t, desc.DeletedDate)
}

func TestDeleteSecret_ForceDelete(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	_, err := b.CreateSecret(
		context.Background(),
		&secretsmanager.CreateSecretInput{Name: "force-del", SecretString: "v"},
	)
	require.NoError(t, err)

	_, err = b.DeleteSecret(context.Background(), &secretsmanager.DeleteSecretInput{
		SecretID:                   "force-del",
		ForceDeleteWithoutRecovery: true,
	})
	require.NoError(t, err)

	// Secret completely gone
	_, err = b.DescribeSecret(context.Background(), &secretsmanager.DescribeSecretInput{SecretID: "force-del"})
	require.ErrorIs(t, err, secretsmanager.ErrSecretNotFound)
}

func TestDeleteSecret_RecoveryWindowMin(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	_, err := b.CreateSecret(
		context.Background(),
		&secretsmanager.CreateSecretInput{Name: "recov-min", SecretString: "v"},
	)
	require.NoError(t, err)

	days := int64(7)
	_, err = b.DeleteSecret(context.Background(), &secretsmanager.DeleteSecretInput{
		SecretID:             "recov-min",
		RecoveryWindowInDays: &days,
	})
	require.NoError(t, err)
}

func TestDeleteSecret_RecoveryWindowMax(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	_, err := b.CreateSecret(
		context.Background(),
		&secretsmanager.CreateSecretInput{Name: "recov-max", SecretString: "v"},
	)
	require.NoError(t, err)

	days := int64(30)
	_, err = b.DeleteSecret(context.Background(), &secretsmanager.DeleteSecretInput{
		SecretID:             "recov-max",
		RecoveryWindowInDays: &days,
	})
	require.NoError(t, err)
}

func TestDeleteSecret_RecoveryWindowTooShort(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	_, err := b.CreateSecret(
		context.Background(),
		&secretsmanager.CreateSecretInput{Name: "recov-short", SecretString: "v"},
	)
	require.NoError(t, err)

	days := int64(6)
	_, err = b.DeleteSecret(context.Background(), &secretsmanager.DeleteSecretInput{
		SecretID:             "recov-short",
		RecoveryWindowInDays: &days,
	})
	require.ErrorIs(t, err, secretsmanager.ErrInvalidParameter)
}

func TestDeleteSecret_RecoveryWindowTooLong(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	_, err := b.CreateSecret(
		context.Background(),
		&secretsmanager.CreateSecretInput{Name: "recov-long", SecretString: "v"},
	)
	require.NoError(t, err)

	days := int64(31)
	_, err = b.DeleteSecret(context.Background(), &secretsmanager.DeleteSecretInput{
		SecretID:             "recov-long",
		RecoveryWindowInDays: &days,
	})
	require.ErrorIs(t, err, secretsmanager.ErrInvalidParameter)
}

func TestDeleteSecret_NotFound(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	_, err := b.DeleteSecret(context.Background(), &secretsmanager.DeleteSecretInput{SecretID: "missing"})
	require.ErrorIs(t, err, secretsmanager.ErrSecretNotFound)
}

// ---------------------------------------------------------------------------
// Already-deleted secrets
// ---------------------------------------------------------------------------

// TestDeleteSecret_AlreadyDeleted verifies that deleting a secret that is already
// pending deletion returns InvalidParameterException at the backend layer.
func TestDeleteSecret_AlreadyDeleted(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	_, err := b.CreateSecret(
		context.Background(),
		&secretsmanager.CreateSecretInput{Name: "already-del", SecretString: "v"},
	)
	require.NoError(t, err)

	_, err = b.DeleteSecret(context.Background(), &secretsmanager.DeleteSecretInput{SecretID: "already-del"})
	require.NoError(t, err)

	_, err = b.DeleteSecret(context.Background(), &secretsmanager.DeleteSecretInput{SecretID: "already-del"})
	require.ErrorIs(t, err, secretsmanager.ErrInvalidParameter, "deleting an already-deleted secret must fail")
}

// TestDeleteSecret_AlreadyDeletedHTTP verifies the same rule via the HTTP handler: a
// second DeleteSecret on an already-deleted secret returns HTTP 400.
func TestDeleteSecret_AlreadyDeletedHTTP(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	h := secretsmanager.NewHandler(b)

	rec := doR1Request(t, h, "secretsmanager.CreateSecret", `{"Name":"double-delete","SecretString":"v"}`)
	require.Equal(t, http.StatusOK, rec.Code)

	// First delete: should succeed.
	rec = doR1Request(t, h, "secretsmanager.DeleteSecret", `{"SecretId":"double-delete"}`)
	require.Equal(t, http.StatusOK, rec.Code)

	// Second delete: should fail.
	rec = doR1Request(t, h, "secretsmanager.DeleteSecret", `{"SecretId":"double-delete"}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// ---------------------------------------------------------------------------
// ForceDeleteWithoutRecovery vs RecoveryWindowInDays mutual exclusivity
// ---------------------------------------------------------------------------

// TestDeleteSecret_ForceDeleteAndRecoveryWindowConflict verifies that AWS's
// mutual-exclusivity rule between ForceDeleteWithoutRecovery and
// RecoveryWindowInDays is enforced. Real AWS returns InvalidParameterException
// when both are supplied together.
func TestDeleteSecret_ForceDeleteAndRecoveryWindowConflict(t *testing.T) {
	t.Parallel()

	window := int64(7)

	tests := []struct {
		recoveryDays *int64
		name         string
		force        bool
		wantErr      bool
	}{
		{
			name:         "force_plus_recovery_window_rejected",
			recoveryDays: &window,
			force:        true,
			wantErr:      true,
		},
		{
			name:         "force_only_ok",
			recoveryDays: nil,
			force:        true,
			wantErr:      false,
		},
		{
			name:         "recovery_window_only_ok",
			recoveryDays: &window,
			force:        false,
			wantErr:      false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := secretsmanager.NewInMemoryBackend()
			_, err := b.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{
				Name:         "del-conflict-" + tc.name,
				SecretString: "v",
			})
			require.NoError(t, err)

			_, err = b.DeleteSecret(context.Background(), &secretsmanager.DeleteSecretInput{
				SecretID:                   "del-conflict-" + tc.name,
				ForceDeleteWithoutRecovery: tc.force,
				RecoveryWindowInDays:       tc.recoveryDays,
			})

			if tc.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, secretsmanager.ErrInvalidParameter,
					"force-delete + recovery window must be InvalidParameterException")

				return
			}

			require.NoError(t, err)
		})
	}
}

// TestDeleteSecret_ForceAndRecoveryWindowHTTPErrorType verifies the HTTP error
// body and the X-Amzn-Errortype header both carry InvalidParameterException.
func TestDeleteSecret_ForceAndRecoveryWindowHTTPErrorType(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	h := secretsmanager.NewHandler(b)

	rec := doR1Request(t, h, "secretsmanager.CreateSecret",
		`{"Name":"del-conflict-http","SecretString":"v"}`)
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doR1Request(t, h, "secretsmanager.DeleteSecret",
		`{"SecretId":"del-conflict-http","ForceDeleteWithoutRecovery":true,"RecoveryWindowInDays":7}`)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp secretsmanager.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "InvalidParameterException", errResp.Type)
	assert.Equal(t, "InvalidParameterException", rec.Header().Get("X-Amzn-Errortype"),
		"error response must echo the error code in X-Amzn-Errortype")
}

// ---------------------------------------------------------------------------
// ForceDeleteWithoutRecovery vs restorability
// ---------------------------------------------------------------------------

// TestDeleteSecret_ForceDeletePreventsRestore verifies that a soft delete allows a
// subsequent RestoreSecret, while a force delete removes the secret entirely (so
// RestoreSecret afterwards returns ResourceNotFoundException).
func TestDeleteSecret_ForceDeletePreventsRestore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                       string
		forceDeleteWithoutRecovery bool
		wantRestoreErr             bool
	}{
		{
			name:                       "soft_delete_allows_restore",
			forceDeleteWithoutRecovery: false,
			wantRestoreErr:             false,
		},
		{
			name:                       "force_delete_prevents_restore",
			forceDeleteWithoutRecovery: true,
			wantRestoreErr:             true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := secretsmanager.NewInMemoryBackend()

			_, err := b.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{
				Name:         "force-del-test",
				SecretString: "val",
			})
			require.NoError(t, err)

			_, err = b.DeleteSecret(context.Background(), &secretsmanager.DeleteSecretInput{
				SecretID:                   "force-del-test",
				ForceDeleteWithoutRecovery: tt.forceDeleteWithoutRecovery,
			})
			require.NoError(t, err)

			_, err = b.RestoreSecret(
				context.Background(),
				&secretsmanager.RestoreSecretInput{SecretID: "force-del-test"},
			)

			if tt.wantRestoreErr {
				require.ErrorIs(t, err, secretsmanager.ErrSecretNotFound)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Cascade: force-delete clears resource policy + replication
// ---------------------------------------------------------------------------

// TestDeleteSecret_Cascade verifies force-delete clears resource policy + replication.
func TestDeleteSecret_Cascade(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	_, err := b.CreateSecret(
		context.Background(),
		&secretsmanager.CreateSecretInput{Name: "cascade", SecretString: "v"},
	)
	require.NoError(t, err)

	_, err = b.PutResourcePolicy(context.Background(), &secretsmanager.PutResourcePolicyInput{
		SecretID:       "cascade",
		ResourcePolicy: `{"Version":"2012-10-17","Statement":[]}`,
	})
	require.NoError(t, err)

	_, err = b.ReplicateSecretToRegions(context.Background(), &secretsmanager.ReplicateSecretToRegionsInput{
		SecretID:          "cascade",
		AddReplicaRegions: []secretsmanager.ReplicaRegion{{Region: "us-west-2"}},
	})
	require.NoError(t, err)

	require.Equal(t, 1, secretsmanager.ResourcePolicyCount(b))
	require.Equal(t, 1, secretsmanager.ReplicationConfigCount(b))

	// Real AWS: "You can't delete a primary secret that is replicated to
	// other Regions. You must first delete the replicas using
	// RemoveRegionsFromReplication, and then delete the primary secret."
	// (api_op_DeleteSecret.go doc comment). Remove the replica first, as a
	// real client must.
	_, err = b.RemoveRegionsFromReplication(context.Background(), &secretsmanager.RemoveRegionsFromReplicationInput{
		SecretID:             "cascade",
		RemoveReplicaRegions: []string{"us-west-2"},
	})
	require.NoError(t, err)
	require.Equal(t, 0, secretsmanager.ReplicationConfigCount(b))

	_, err = b.DeleteSecret(context.Background(), &secretsmanager.DeleteSecretInput{
		SecretID:                   "cascade",
		ForceDeleteWithoutRecovery: true,
	})
	require.NoError(t, err)

	assert.Equal(t, 0, secretsmanager.SecretCount(b))
	assert.Equal(t, 0, secretsmanager.ResourcePolicyCount(b))
	assert.Equal(t, 0, secretsmanager.ReplicationConfigCount(b))
}

// TestDeleteSecret_RejectsWhilePrimaryHasReplicas confirms DeleteSecret
// rejects a primary secret that is still replicated to other Regions,
// instead of silently deleting the primary while leaving its replicas
// orphaned (and permanently readable) in their Regions.
func TestDeleteSecret_RejectsWhilePrimaryHasReplicas(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	_, err := b.CreateSecret(
		context.Background(),
		&secretsmanager.CreateSecretInput{Name: "still-replicated", SecretString: "v"},
	)
	require.NoError(t, err)

	_, err = b.ReplicateSecretToRegions(context.Background(), &secretsmanager.ReplicateSecretToRegionsInput{
		SecretID:          "still-replicated",
		AddReplicaRegions: []secretsmanager.ReplicaRegion{{Region: "us-west-2"}},
	})
	require.NoError(t, err)

	_, err = b.DeleteSecret(context.Background(), &secretsmanager.DeleteSecretInput{
		SecretID:                   "still-replicated",
		ForceDeleteWithoutRecovery: true,
	})
	require.ErrorIs(t, err, secretsmanager.ErrInvalidParameter)

	// The secret and its replica must both still be there.
	_, err = b.DescribeSecret(context.Background(), &secretsmanager.DescribeSecretInput{SecretID: "still-replicated"})
	require.NoError(t, err)
	assert.Equal(t, 2, secretsmanager.SecretCount(b))
}

// ---------------------------------------------------------------------------
// RecoveryWindowInDays honored via HTTP (default + custom windows)
// ---------------------------------------------------------------------------

// TestDeleteSecret_RecoveryWindowInDaysHTTP verifies DeleteSecret honors
// RecoveryWindowInDays (including the default 30-day window) at the HTTP layer.
func TestDeleteSecret_RecoveryWindowInDaysHTTP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		checkForward func(t *testing.T, deletionDate float64)
		name         string
		body         string
		wantStatus   int
		wantDeleted  bool
	}{
		{
			name:        "default_30_day_window",
			body:        `{"SecretId":"recover-test"}`,
			wantStatus:  http.StatusOK,
			wantDeleted: false,
			checkForward: func(t *testing.T, deletionDate float64) {
				t.Helper()
				// DeletionDate should be ~30 days from now.
				td := time.Unix(int64(deletionDate), 0).UTC()
				expected := time.Now().UTC().Add(30 * 24 * time.Hour)
				diff := td.Sub(expected).Abs()
				assert.Less(t, diff, 5*time.Second)
			},
		},
		{
			name:        "custom_7_day_window",
			body:        `{"SecretId":"recover-test2","RecoveryWindowInDays":7}`,
			wantStatus:  http.StatusOK,
			wantDeleted: false,
			checkForward: func(t *testing.T, deletionDate float64) {
				t.Helper()
				td := time.Unix(int64(deletionDate), 0).UTC()
				expected := time.Now().UTC().Add(7 * 24 * time.Hour)
				diff := td.Sub(expected).Abs()
				assert.Less(t, diff, 5*time.Second)
			},
		},
		{
			name:       "invalid_window_too_small",
			body:       `{"SecretId":"recover-test","RecoveryWindowInDays":5}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid_window_too_large",
			body:       `{"SecretId":"recover-test","RecoveryWindowInDays":31}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := secretsmanager.NewInMemoryBackend()
			h := secretsmanager.NewHandler(b)

			// Create the secret(s) needed for the test.
			for _, name := range []string{"recover-test", "recover-test2"} {
				body, _ := json.Marshal(map[string]string{"Name": name, "SecretString": "v"})
				rec := doR1Request(t, h, "secretsmanager.CreateSecret", string(body))
				require.Equal(t, http.StatusOK, rec.Code)
			}

			rec := doR1Request(t, h, "secretsmanager.DeleteSecret", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK && tt.checkForward != nil {
				var out secretsmanager.DeleteSecretOutput
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
				tt.checkForward(t, out.DeletionDate)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Backend scenarios (ported from table-style subtests)
// ---------------------------------------------------------------------------

func TestDeleteSecret_BackendScenarios(t *testing.T) {
	t.Parallel()

	t.Run("DeleteAndRestore", func(t *testing.T) {
		t.Parallel()

		backend := secretsmanager.NewInMemoryBackend()
		_, _ = backend.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{
			Name:         "restorable",
			SecretString: "data",
		})

		delOut, err := backend.DeleteSecret(
			context.Background(),
			&secretsmanager.DeleteSecretInput{SecretID: "restorable"},
		)
		require.NoError(t, err)
		assert.NotZero(t, delOut.DeletionDate)

		// Restore
		restOut, err := backend.RestoreSecret(
			context.Background(),
			&secretsmanager.RestoreSecretInput{SecretID: "restorable"},
		)
		require.NoError(t, err)
		assert.Equal(t, "restorable", restOut.Name)

		// Can get value again
		_, err = backend.GetSecretValue(
			context.Background(),
			&secretsmanager.GetSecretValueInput{SecretID: "restorable"},
		)
		require.NoError(t, err)
	})

	t.Run("DeleteNotFound", func(t *testing.T) {
		t.Parallel()

		backend := secretsmanager.NewInMemoryBackend()

		_, err := backend.DeleteSecret(context.Background(), &secretsmanager.DeleteSecretInput{SecretID: "missing"})
		require.ErrorIs(t, err, secretsmanager.ErrSecretNotFound)
	})
}

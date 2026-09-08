package secretsmanager_test

// replication_test.go consolidates every ReplicateSecretToRegions /
// RemoveRegionsFromReplication / StopReplicationToReplica test that was previously
// scattered across several older test files. Ported verbatim (assertions unchanged).

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/secretsmanager"
)

// ---------------------------------------------------------------------------
// Replication comprehensive
// ---------------------------------------------------------------------------

func TestReplication_AddThenRemove(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	_, err := b.CreateSecret(
		context.Background(),
		&secretsmanager.CreateSecretInput{Name: "rep-add-rm", SecretString: "v"},
	)
	require.NoError(t, err)

	_, err = b.ReplicateSecretToRegions(context.Background(), &secretsmanager.ReplicateSecretToRegionsInput{
		SecretID:          "rep-add-rm",
		AddReplicaRegions: []secretsmanager.ReplicaRegion{{Region: "eu-west-1"}},
	})
	require.NoError(t, err)

	desc, err := b.DescribeSecret(context.Background(), &secretsmanager.DescribeSecretInput{SecretID: "rep-add-rm"})
	require.NoError(t, err)
	assert.Len(t, desc.ReplicationStatus, 1)

	_, err = b.RemoveRegionsFromReplication(context.Background(), &secretsmanager.RemoveRegionsFromReplicationInput{
		SecretID:             "rep-add-rm",
		RemoveReplicaRegions: []string{"eu-west-1"},
	})
	require.NoError(t, err)

	desc2, err := b.DescribeSecret(context.Background(), &secretsmanager.DescribeSecretInput{SecretID: "rep-add-rm"})
	require.NoError(t, err)
	assert.Empty(t, desc2.ReplicationStatus)
}

func TestReplication_InSyncWithValue(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	_, err := b.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{
		Name:         "rep-insync",
		SecretString: "v",
		AddReplicaRegions: []secretsmanager.ReplicaRegion{
			{Region: "ap-southeast-1"},
		},
	})
	require.NoError(t, err)

	desc, err := b.DescribeSecret(context.Background(), &secretsmanager.DescribeSecretInput{SecretID: "rep-insync"})
	require.NoError(t, err)
	require.Len(t, desc.ReplicationStatus, 1)
	assert.Equal(t, "InSync", desc.ReplicationStatus[0].Status)
}

func TestReplication_FailedWithoutValue(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	_, err := b.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{
		Name: "rep-failed",
		AddReplicaRegions: []secretsmanager.ReplicaRegion{
			{Region: "us-west-1"},
		},
	})
	require.NoError(t, err)

	desc, err := b.DescribeSecret(context.Background(), &secretsmanager.DescribeSecretInput{SecretID: "rep-failed"})
	require.NoError(t, err)
	require.Len(t, desc.ReplicationStatus, 1)
	assert.NotEqual(t, "InSync", desc.ReplicationStatus[0].Status)
}

func TestReplication_StopReplication(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	_, err := b.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{
		Name:              "rep-stop",
		SecretString:      "v",
		AddReplicaRegions: []secretsmanager.ReplicaRegion{{Region: "ca-central-1"}},
	})
	require.NoError(t, err)

	_, err = b.StopReplicationToReplica(
		context.Background(),
		&secretsmanager.StopReplicationToReplicaInput{SecretID: "rep-stop"},
	)
	require.NoError(t, err)

	desc, err := b.DescribeSecret(context.Background(), &secretsmanager.DescribeSecretInput{SecretID: "rep-stop"})
	require.NoError(t, err)
	assert.Empty(t, desc.ReplicationStatus, "StopReplicationToReplica must clear replication config")
}

func TestReplication_UpdatedAfterPutSecretValue(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	// Create without value but with replica
	_, err := b.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{
		Name:              "rep-update",
		AddReplicaRegions: []secretsmanager.ReplicaRegion{{Region: "sa-east-1"}},
	})
	require.NoError(t, err)

	desc1, err := b.DescribeSecret(context.Background(), &secretsmanager.DescribeSecretInput{SecretID: "rep-update"})
	require.NoError(t, err)
	require.Len(t, desc1.ReplicationStatus, 1)
	assert.NotEqual(t, "InSync", desc1.ReplicationStatus[0].Status, "should not be InSync without value")

	// Now add a value
	_, err = b.PutSecretValue(context.Background(), &secretsmanager.PutSecretValueInput{
		SecretID:     "rep-update",
		SecretString: "v",
	})
	require.NoError(t, err)

	desc2, err := b.DescribeSecret(context.Background(), &secretsmanager.DescribeSecretInput{SecretID: "rep-update"})
	require.NoError(t, err)
	require.Len(t, desc2.ReplicationStatus, 1)
	assert.Equal(t, "InSync", desc2.ReplicationStatus[0].Status, "should be InSync after value added")
}

func TestReplication_NotFound(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	_, err := b.ReplicateSecretToRegions(context.Background(), &secretsmanager.ReplicateSecretToRegionsInput{
		SecretID:          "missing",
		AddReplicaRegions: []secretsmanager.ReplicaRegion{{Region: "eu-west-1"}},
	})
	require.ErrorIs(t, err, secretsmanager.ErrSecretNotFound)
}

// TestReplication_ReplicaSecretReadableInReplicaRegion confirms
// ReplicateSecretToRegions does more than bookkeep a status: a client
// switching to the replica region must be able to read the replicated value,
// not get ResourceNotFoundException.
func TestReplication_ReplicaSecretReadableInReplicaRegion(t *testing.T) {
	t.Parallel()

	h := newSMHandler()

	createRec := doSMRequestInRegion(t, h, secretsmanager.MockRegion, "secretsmanager.CreateSecret",
		`{"Name":"rep-readable","SecretString":"replicated-value"}`)
	require.Equal(t, http.StatusOK, createRec.Code)

	replicateRec := doSMRequestInRegion(t, h, secretsmanager.MockRegion, "secretsmanager.ReplicateSecretToRegions",
		`{"SecretId":"rep-readable","AddReplicaRegions":[{"Region":"us-west-2"}]}`)
	require.Equal(t, http.StatusOK, replicateRec.Code)

	getRec := doSMRequestInRegion(t, h, "us-west-2", "secretsmanager.GetSecretValue",
		`{"SecretId":"rep-readable"}`)
	require.Equal(t, http.StatusOK, getRec.Code,
		"replica region must serve the replicated secret, got: %s", getRec.Body.String())

	var getOut secretsmanager.GetSecretValueOutput
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getOut))
	assert.Equal(t, "replicated-value", getOut.SecretString)

	descRec := doSMRequestInRegion(t, h, "us-west-2", "secretsmanager.DescribeSecret",
		`{"SecretId":"rep-readable"}`)
	require.Equal(t, http.StatusOK, descRec.Code)

	var descOut secretsmanager.DescribeSecretOutput
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &descOut))
	assert.Equal(t, secretsmanager.MockRegion, descOut.PrimaryRegion)
	assert.Contains(t, descOut.ARN, "us-west-2")
}

// TestReplication_RemoveRegionsDeletesReplicaSecret confirms
// RemoveRegionsFromReplication doesn't just stop tracking a region's status
// while leaving the mirrored secret readable there forever.
func TestReplication_RemoveRegionsDeletesReplicaSecret(t *testing.T) {
	t.Parallel()

	h := newSMHandler()

	require.Equal(t, http.StatusOK, doSMRequestInRegion(t, h, secretsmanager.MockRegion,
		"secretsmanager.CreateSecret", `{"Name":"rep-removed","SecretString":"v"}`).Code)
	require.Equal(t, http.StatusOK, doSMRequestInRegion(t, h, secretsmanager.MockRegion,
		"secretsmanager.ReplicateSecretToRegions",
		`{"SecretId":"rep-removed","AddReplicaRegions":[{"Region":"ap-south-1"}]}`).Code)

	require.Equal(t, http.StatusOK, doSMRequestInRegion(t, h, "ap-south-1",
		"secretsmanager.GetSecretValue", `{"SecretId":"rep-removed"}`).Code, "replica must be readable before removal")

	require.Equal(t, http.StatusOK, doSMRequestInRegion(t, h, secretsmanager.MockRegion,
		"secretsmanager.RemoveRegionsFromReplication",
		`{"SecretId":"rep-removed","RemoveReplicaRegions":["ap-south-1"]}`).Code)

	getRec := doSMRequestInRegion(t, h, "ap-south-1", "secretsmanager.GetSecretValue", `{"SecretId":"rep-removed"}`)
	assert.Equal(t, http.StatusBadRequest, getRec.Code, "removed replica region must no longer serve the secret")
}

// ---------------------------------------------------------------------------
// Replication HTTP cycle
// ---------------------------------------------------------------------------

// TestReplication_Operations verifies ReplicateSecretToRegions, RemoveRegionsFromReplication,
// and StopReplicationToReplica.
func TestReplication_Operations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup          func(*testing.T, *secretsmanager.InMemoryBackend)
		checkFn        func(*testing.T, *httptest.ResponseRecorder)
		name           string
		target         string
		body           string
		expectedStatus int
	}{
		{
			name: "replicate_to_regions",
			setup: func(t *testing.T, b *secretsmanager.InMemoryBackend) {
				t.Helper()
				_, err := b.CreateSecret(
					context.Background(),
					&secretsmanager.CreateSecretInput{Name: "rep-secret", SecretString: "v"},
				)
				require.NoError(t, err)
			},
			target:         "secretsmanager.ReplicateSecretToRegions",
			body:           `{"SecretId":"rep-secret","AddReplicaRegions":[{"Region":"us-west-2"}]}`,
			expectedStatus: http.StatusOK,
			checkFn: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()
				var out secretsmanager.ReplicateSecretToRegionsOutput
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
				assert.NotEmpty(t, out.ARN)
				require.Len(t, out.ReplicationStatus, 1)
				assert.Equal(t, "us-west-2", out.ReplicationStatus[0].Region)
				assert.Equal(t, "InSync", out.ReplicationStatus[0].Status)
			},
		},
		{
			name: "remove_regions_from_replication",
			setup: func(t *testing.T, b *secretsmanager.InMemoryBackend) {
				t.Helper()
				_, err := b.CreateSecret(
					context.Background(),
					&secretsmanager.CreateSecretInput{Name: "rem-rep", SecretString: "v"},
				)
				require.NoError(t, err)
				_, err = b.ReplicateSecretToRegions(context.Background(), &secretsmanager.ReplicateSecretToRegionsInput{
					SecretID:          "rem-rep",
					AddReplicaRegions: []secretsmanager.ReplicaRegion{{Region: "eu-west-1"}, {Region: "ap-east-1"}},
				})
				require.NoError(t, err)
			},
			target:         "secretsmanager.RemoveRegionsFromReplication",
			body:           `{"SecretId":"rem-rep","RemoveReplicaRegions":["eu-west-1"]}`,
			expectedStatus: http.StatusOK,
			checkFn: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()
				var out secretsmanager.RemoveRegionsFromReplicationOutput
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
				require.Len(t, out.ReplicationStatus, 1)
				assert.Equal(t, "ap-east-1", out.ReplicationStatus[0].Region)
			},
		},
		{
			name: "stop_replication_to_replica",
			setup: func(t *testing.T, b *secretsmanager.InMemoryBackend) {
				t.Helper()
				_, err := b.CreateSecret(
					context.Background(),
					&secretsmanager.CreateSecretInput{Name: "stop-rep", SecretString: "v"},
				)
				require.NoError(t, err)
				_, err = b.ReplicateSecretToRegions(context.Background(), &secretsmanager.ReplicateSecretToRegionsInput{
					SecretID:          "stop-rep",
					AddReplicaRegions: []secretsmanager.ReplicaRegion{{Region: "eu-west-1"}},
				})
				require.NoError(t, err)
			},
			target:         "secretsmanager.StopReplicationToReplica",
			body:           `{"SecretId":"stop-rep"}`,
			expectedStatus: http.StatusOK,
			checkFn: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()
				var out secretsmanager.StopReplicationToReplicaOutput
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
				assert.NotEmpty(t, out.ARN)
			},
		},
		{
			name:           "replicate_not_found",
			target:         "secretsmanager.ReplicateSecretToRegions",
			body:           `{"SecretId":"nonexistent","AddReplicaRegions":[{"Region":"us-west-2"}]}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "remove_regions_not_found",
			target:         "secretsmanager.RemoveRegionsFromReplication",
			body:           `{"SecretId":"nonexistent","RemoveReplicaRegions":["us-west-2"]}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "stop_replication_not_found",
			target:         "secretsmanager.StopReplicationToReplica",
			body:           `{"SecretId":"nonexistent"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "replicate_bad_json",
			target:         "secretsmanager.ReplicateSecretToRegions",
			body:           `{bad}`,
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "remove_regions_bad_json",
			target:         "secretsmanager.RemoveRegionsFromReplication",
			body:           `{bad}`,
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "stop_replication_bad_json",
			target:         "secretsmanager.StopReplicationToReplica",
			body:           `{bad}`,
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := secretsmanager.NewInMemoryBackend()
			if tt.setup != nil {
				tt.setup(t, backend)
			}

			h := secretsmanager.NewHandler(backend)
			rec := doSMRequest(t, h, tt.target, tt.body)

			assert.Equal(t, tt.expectedStatus, rec.Code)

			if tt.checkFn != nil {
				tt.checkFn(t, rec)
			}
		})
	}
}

// TestReplication_BackendEdgeCases ports the replication-specific subtest of the
// original TestNewOpsBackend (the resource-policy/rotation/batchget subtests of
// that function belong to sibling agents).
func TestReplication_BackendEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("ReplicateSecretToRegions_idempotent_update", func(t *testing.T) {
		t.Parallel()

		b := secretsmanager.NewInMemoryBackend()
		_, err := b.CreateSecret(
			context.Background(),
			&secretsmanager.CreateSecretInput{Name: "rep-idem", SecretString: "v"},
		)
		require.NoError(t, err)

		// Add us-east-2.
		_, err = b.ReplicateSecretToRegions(context.Background(), &secretsmanager.ReplicateSecretToRegionsInput{
			SecretID:          "rep-idem",
			AddReplicaRegions: []secretsmanager.ReplicaRegion{{Region: "us-east-2"}},
		})
		require.NoError(t, err)

		// Add us-east-2 again with ForceOverwriteReplicaSecret=true (required to update).
		out, err := b.ReplicateSecretToRegions(context.Background(), &secretsmanager.ReplicateSecretToRegionsInput{
			SecretID:                    "rep-idem",
			AddReplicaRegions:           []secretsmanager.ReplicaRegion{{Region: "us-east-2", KmsKeyID: "key-123"}},
			ForceOverwriteReplicaSecret: true,
		})
		require.NoError(t, err)
		assert.Len(t, out.ReplicationStatus, 1)
		assert.Equal(t, "key-123", out.ReplicationStatus[0].KmsKeyID)
	})
}

// ---------------------------------------------------------------------------
// ForceOverwriteReplicaSecret parity (handler_parity_test.go)
// ---------------------------------------------------------------------------

// TestReplicateSecretToRegions_ExistingRegionRejectedWithoutForce verifies that
// replicating to a region that already has a replica is rejected when
// ForceOverwriteReplicaSecret is false.
//
// ReplicateSecretToRegions's own deserializeOpError (aws-sdk-go-v2/service/
// secretsmanager@v1.44.4 deserializers.go) recognizes InternalServiceError,
// InvalidParameterException, InvalidRequestException and
// ResourceNotFoundException -- no ResourceExistsException case, unlike
// CreateSecret/PutSecretValue/UpdateSecret. InvalidRequestException is the
// existing in-service precedent for "operation invalid given the resource's
// current state" (ErrSecretDeleted, ErrRotationStrategyRequired).
func TestReplicateSecretToRegions_ExistingRegionRejectedWithoutForce(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	ctx := context.Background()

	_, err := b.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
		Name:         "replicated-secret",
		SecretString: "v",
	})
	require.NoError(t, err)

	// First replication succeeds.
	_, err = b.ReplicateSecretToRegions(ctx, &secretsmanager.ReplicateSecretToRegionsInput{
		SecretID:          "replicated-secret",
		AddReplicaRegions: []secretsmanager.ReplicaRegion{{Region: "us-east-2"}},
	})
	require.NoError(t, err)

	// Second replication to same region without force must fail.
	_, err = b.ReplicateSecretToRegions(ctx, &secretsmanager.ReplicateSecretToRegionsInput{
		SecretID:          "replicated-secret",
		AddReplicaRegions: []secretsmanager.ReplicaRegion{{Region: "us-east-2"}},
	})
	assert.ErrorIs(t, err, secretsmanager.ErrReplicaAlreadyExists,
		"replicating to existing region without ForceOverwriteReplicaSecret"+
			" must return an error ReplicateSecretToRegions's own deserializer recognizes")
}

// TestReplicateSecretToRegions_ExistingRegionErrorType_WireCode verifies the
// wire __type/X-Amzn-Errortype for the same condition is InvalidRequestException,
// not ResourceExistsException -- see the ErrorIs test above for why.
func TestReplicateSecretToRegions_ExistingRegionErrorType_WireCode(t *testing.T) {
	t.Parallel()

	h := newSMHandler()

	create := doSMRequest(t, h, "secretsmanager.CreateSecret",
		`{"Name":"wire-replicated-secret","SecretString":"v"}`)
	require.Equal(t, http.StatusOK, create.Code)

	first := doSMRequest(t, h, "secretsmanager.ReplicateSecretToRegions",
		`{"SecretId":"wire-replicated-secret","AddReplicaRegions":[{"Region":"us-east-2"}]}`)
	require.Equal(t, http.StatusOK, first.Code)

	second := doSMRequest(t, h, "secretsmanager.ReplicateSecretToRegions",
		`{"SecretId":"wire-replicated-secret","AddReplicaRegions":[{"Region":"us-east-2"}]}`)
	require.Equal(t, http.StatusBadRequest, second.Code)
	assert.Equal(t, "InvalidRequestException", second.Header().Get("X-Amzn-Errortype"))

	var body struct {
		Type string `json:"__type"`
	}
	require.NoError(t, json.Unmarshal(second.Body.Bytes(), &body))
	assert.Equal(t, "InvalidRequestException", body.Type)
	assert.NotEqual(t, "ResourceExistsException", body.Type)
}

// TestReplicateSecretToRegions_ForceOverwriteAllowed verifies that
// ForceOverwriteReplicaSecret=true allows overwriting an existing replica.
func TestReplicateSecretToRegions_ForceOverwriteAllowed(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	ctx := context.Background()

	_, err := b.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
		Name:         "force-overwrite-secret",
		SecretString: "v",
	})
	require.NoError(t, err)

	_, err = b.ReplicateSecretToRegions(ctx, &secretsmanager.ReplicateSecretToRegionsInput{
		SecretID:          "force-overwrite-secret",
		AddReplicaRegions: []secretsmanager.ReplicaRegion{{Region: "eu-west-1"}},
	})
	require.NoError(t, err)

	// Force overwrite succeeds.
	out, err := b.ReplicateSecretToRegions(ctx, &secretsmanager.ReplicateSecretToRegionsInput{
		SecretID:                    "force-overwrite-secret",
		AddReplicaRegions:           []secretsmanager.ReplicaRegion{{Region: "eu-west-1", KmsKeyID: "new-key"}},
		ForceOverwriteReplicaSecret: true,
	})
	require.NoError(t, err)
	require.Len(t, out.ReplicationStatus, 1)
	assert.Equal(t, "new-key", out.ReplicationStatus[0].KmsKeyID,
		"ForceOverwriteReplicaSecret=true must update the existing replica")
}

// ---------------------------------------------------------------------------
// Replication status sync across PutSecretValue
// ---------------------------------------------------------------------------

func TestReplication_StatusSync(t *testing.T) {
	t.Parallel()

	tests := []struct {
		initialSecretString string
		name                string
		wantStatus          string
	}{
		{
			name:                "replication_fails_without_current_version",
			initialSecretString: "",
			wantStatus:          "Failed",
		},
		{
			name:                "replication_syncs_current_version",
			initialSecretString: "v1",
			wantStatus:          "InSync",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := secretsmanager.NewInMemoryBackend()
			_, err := backend.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{
				Name:         "replication-secret",
				SecretString: tt.initialSecretString,
			})
			require.NoError(t, err)

			_, err = backend.ReplicateSecretToRegions(
				context.Background(),
				&secretsmanager.ReplicateSecretToRegionsInput{
					SecretID:          "replication-secret",
					AddReplicaRegions: []secretsmanager.ReplicaRegion{{Region: "us-west-2"}},
				},
			)
			require.NoError(t, err)

			desc, err := backend.DescribeSecret(context.Background(), &secretsmanager.DescribeSecretInput{
				SecretID: "replication-secret",
			})
			require.NoError(t, err)
			require.Len(t, desc.ReplicationStatus, 1)
			assert.Equal(t, tt.wantStatus, desc.ReplicationStatus[0].Status)

			if tt.wantStatus != "InSync" {
				assert.Contains(t, desc.ReplicationStatus[0].StatusMessage, "no current secret version")

				return
			}

			initialCurrent, currentErr := backend.GetSecretValue(
				context.Background(),
				&secretsmanager.GetSecretValueInput{
					SecretID: "replication-secret",
				},
			)
			require.NoError(t, currentErr)
			assert.Contains(t, desc.ReplicationStatus[0].StatusMessage, initialCurrent.VersionID)

			_, err = backend.PutSecretValue(context.Background(), &secretsmanager.PutSecretValueInput{
				SecretID:     "replication-secret",
				SecretString: "v2",
			})
			require.NoError(t, err)

			nextCurrent, nextErr := backend.GetSecretValue(context.Background(), &secretsmanager.GetSecretValueInput{
				SecretID: "replication-secret",
			})
			require.NoError(t, nextErr)
			assert.NotEqual(t, initialCurrent.VersionID, nextCurrent.VersionID)

			descAfterPut, describeErr := backend.DescribeSecret(
				context.Background(),
				&secretsmanager.DescribeSecretInput{
					SecretID: "replication-secret",
				},
			)
			require.NoError(t, describeErr)
			require.Len(t, descAfterPut.ReplicationStatus, 1)
			assert.Equal(t, "InSync", descAfterPut.ReplicationStatus[0].Status)
			assert.Contains(t, descAfterPut.ReplicationStatus[0].StatusMessage, nextCurrent.VersionID)
		})
	}
}

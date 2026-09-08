package secretsmanager_test

// replica_write_guard_test.go covers gopherstack-ngkw: a replica secret must
// reject the writes real AWS documents as primary-only (PutSecretValue,
// UpdateSecret beyond its KmsKeyId, RotateSecret) while every write real AWS
// still allows on a replica (DeleteSecret, TagResource, CancelRotateSecret,
// and a KmsKeyId-only UpdateSecret) keeps working, and a primary secret's own
// writes are unaffected by having replicas at all. RestoreSecret is not
// exercised here: DeleteSecret on a replica is an immediate hard delete
// (deleteReplicaImmediatelyLocked, matching "When you delete a replica, it
// is deleted immediately"), so there is no soft-deleted replica state to
// restore.

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/secretsmanager"
)

const replicaRegion = "us-west-2"

// setupPrimaryWithReplica creates secretName in the mock primary region with
// value "v1" and replicates it to replicaRegion, returning the handler ready
// for requests against either region.
func setupPrimaryWithReplica(t *testing.T, secretName string) *secretsmanager.Handler {
	t.Helper()

	h := newSMHandler()

	create := doSMRequestInRegion(t, h, secretsmanager.MockRegion, "secretsmanager.CreateSecret",
		`{"Name":"`+secretName+`","SecretString":"v1"}`)
	require.Equal(t, http.StatusOK, create.Code)

	replicate := doSMRequestInRegion(t, h, secretsmanager.MockRegion, "secretsmanager.ReplicateSecretToRegions",
		`{"SecretId":"`+secretName+`","AddReplicaRegions":[{"Region":"`+replicaRegion+`"}]}`)
	require.Equal(t, http.StatusOK, replicate.Code)

	return h
}

func TestReplicaWriteGuard_BlockedOnReplica(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		target string
		body   string
	}{
		{
			name:   "putsecretvalue",
			target: "secretsmanager.PutSecretValue",
			body:   `{"SecretId":"blocked-put","SecretString":"v2"}`,
		},
		{
			name:   "updatesecret_secretstring",
			target: "secretsmanager.UpdateSecret",
			body:   `{"SecretId":"blocked-put","SecretString":"v2"}`,
		},
		{
			name:   "updatesecret_description",
			target: "secretsmanager.UpdateSecret",
			body:   `{"SecretId":"blocked-put","Description":"new description"}`,
		},
		{
			name:   "updatesecret_type",
			target: "secretsmanager.UpdateSecret",
			body:   `{"SecretId":"blocked-put","Type":"OtherType"}`,
		},
		{
			name:   "rotatesecret",
			target: "secretsmanager.RotateSecret",
			// RotationLambdaARN is supplied so a passing rotation strategy
			// check can't be mistaken for the replica guard: without this,
			// ErrRotationStrategyRequired would also produce a 400
			// InvalidRequestException on a replica that (correctly) never
			// has rotation configured, masking a neutered replica guard.
			body: `{"SecretId":"blocked-put","RotationLambdaARN":"` + testLambdaARN + `"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := setupPrimaryWithReplica(t, "blocked-put")

			rec := doSMRequestInRegion(t, h, replicaRegion, tc.target, tc.body)
			require.Equal(t, http.StatusBadRequest, rec.Code, "body: %s", rec.Body.String())
			assert.Equal(t, "InvalidRequestException", rec.Header().Get("X-Amzn-Errortype"))

			var out struct {
				Type string `json:"__type"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			assert.Equal(t, "InvalidRequestException", out.Type)
		})
	}
}

func TestReplicaWriteGuard_StillAllowedOnReplica(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		target string
		body   string
	}{
		{
			name:   "updatesecret_kmskeyid_only",
			target: "secretsmanager.UpdateSecret",
			body:   `{"SecretId":"allowed-put","KmsKeyId":"alias/replica-key"}`,
		},
		{
			name:   "tagresource",
			target: "secretsmanager.TagResource",
			body:   `{"SecretId":"allowed-put","Tags":[{"Key":"k","Value":"v"}]}`,
		},
		{
			name:   "cancelrotatesecret",
			target: "secretsmanager.CancelRotateSecret",
			body:   `{"SecretId":"allowed-put"}`,
		},
		{
			name:   "deletesecret",
			target: "secretsmanager.DeleteSecret",
			body:   `{"SecretId":"allowed-put"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := setupPrimaryWithReplica(t, "allowed-put")

			rec := doSMRequestInRegion(t, h, replicaRegion, tc.target, tc.body)
			assert.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
		})
	}
}

// TestReplicaWriteGuard_PrimaryUnaffectedByReplicas proves having replicas
// configured does not block the primary's own writes.
func TestReplicaWriteGuard_PrimaryUnaffectedByReplicas(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		target string
		body   string
	}{
		{
			name:   "putsecretvalue",
			target: "secretsmanager.PutSecretValue",
			body:   `{"SecretId":"primary-writes","SecretString":"v2"}`,
		},
		{
			name:   "updatesecret",
			target: "secretsmanager.UpdateSecret",
			body:   `{"SecretId":"primary-writes","Description":"updated"}`,
		},
		{
			name:   "rotatesecret",
			target: "secretsmanager.RotateSecret",
			body:   `{"SecretId":"primary-writes","RotationLambdaARN":"` + testLambdaARN + `"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := setupPrimaryWithReplica(t, "primary-writes")

			rec := doSMRequestInRegion(t, h, secretsmanager.MockRegion, tc.target, tc.body)
			assert.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
		})
	}
}

// TestReplicaWriteGuard_BackendErrorIs proves the backend-level error
// returned to a replica write is ErrReplicaNotWritable, not merely the right
// wire code by coincidence.
func TestReplicaWriteGuard_BackendErrorIs(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	ctx := secretsmanager.CtxWithRegion(context.Background(), replicaRegion)

	primaryCtx := secretsmanager.CtxWithRegion(context.Background(), secretsmanager.MockRegion)
	_, err := b.CreateSecret(primaryCtx, &secretsmanager.CreateSecretInput{
		Name: "backend-err", SecretString: "v1",
	})
	require.NoError(t, err)

	_, err = b.ReplicateSecretToRegions(primaryCtx, &secretsmanager.ReplicateSecretToRegionsInput{
		SecretID:          "backend-err",
		AddReplicaRegions: []secretsmanager.ReplicaRegion{{Region: replicaRegion}},
	})
	require.NoError(t, err)

	// RotationLambdaARN is supplied on the RotateSecret call for the same
	// reason as in TestReplicaWriteGuard_BlockedOnReplica: without it,
	// ErrRotationStrategyRequired would also produce an InvalidRequestException
	// on this replica (which never has rotation configured), masking a
	// broken replica guard.
	_, putErr := b.PutSecretValue(ctx, &secretsmanager.PutSecretValueInput{
		SecretID: "backend-err", SecretString: "v2",
	})
	require.ErrorIs(t, putErr, secretsmanager.ErrReplicaNotWritable)

	_, rotateErr := b.RotateSecret(ctx, &secretsmanager.RotateSecretInput{
		SecretID: "backend-err", RotationLambdaARN: testLambdaARN,
	})
	require.ErrorIs(t, rotateErr, secretsmanager.ErrReplicaNotWritable)

	_, updateErr := b.UpdateSecret(ctx, &secretsmanager.UpdateSecretInput{
		SecretID: "backend-err", SecretString: "v2",
	})
	require.ErrorIs(t, updateErr, secretsmanager.ErrReplicaNotWritable)
}

package secretsmanager_test

// createsecret_test.go consolidates every CreateSecret-specific test that was
// previously scattered across several older test files. Ported verbatim
// (assertions unchanged).

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/secretsmanager"
)

// ---------------------------------------------------------------------------
// Secret name validation (exercised via CreateSecret)
// ---------------------------------------------------------------------------

func TestCreateSecret_NameEmpty(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	_, err := b.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{Name: ""})
	require.Error(t, err)
	require.ErrorIs(t, err, secretsmanager.ErrInvalidSecretName)
}

func TestCreateSecret_NameTooLong(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	_, err := b.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{Name: strings.Repeat("a", 513)})
	require.ErrorIs(t, err, secretsmanager.ErrInvalidSecretName)
}

func TestCreateSecret_NameExactMaxLength(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	_, err := b.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{Name: strings.Repeat("a", 512)})
	require.NoError(t, err)
}

func TestCreateSecret_NameInvalidChars(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()

	for _, name := range []string{"has space", "has\ttab", "has\nnewline", "has$dollar"} {
		_, err := b.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{Name: name})
		require.ErrorIs(t, err, secretsmanager.ErrInvalidSecretName, "expected error for %q", name)
	}
}

func TestCreateSecret_NameValidSpecialChars(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	// All allowed special characters: /_+=.@-
	_, err := b.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{
		Name:         "valid/_+=.@-name",
		SecretString: "v",
	})
	require.NoError(t, err)
}

func TestCreateSecret_NameAWSPrefixRejected(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	_, err := b.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{
		Name:         "aws/my-secret",
		SecretString: "v",
	})
	require.ErrorIs(t, err, secretsmanager.ErrInvalidSecretName, "names starting with \"aws/\" must be rejected")
}

func TestCreateSecret_NameAWSPrefixHTTP(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	h := secretsmanager.NewHandler(b)

	rec := doR1Request(t, h, "secretsmanager.CreateSecret",
		`{"Name":"aws/my-secret","SecretString":"v"}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp secretsmanager.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "InvalidParameterException", errResp.Type)
}

func TestCreateSecret_NameSlashInMiddleAllowed(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	_, err := b.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{
		Name:         "prod/db/password",
		SecretString: "v",
	})
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// CreateSecret comprehensive
// ---------------------------------------------------------------------------

func TestCreateSecret_WithKmsKeyID(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	_, err := b.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{
		Name:         "kms-secret",
		SecretString: "v",
		KmsKeyID:     "arn:aws:kms:us-east-1:123456789012:key/abc-123",
	})
	require.NoError(t, err)

	desc, err := b.DescribeSecret(context.Background(), &secretsmanager.DescribeSecretInput{SecretID: "kms-secret"})
	require.NoError(t, err)
	assert.Equal(t, "arn:aws:kms:us-east-1:123456789012:key/abc-123", desc.KmsKeyID)
}

func TestCreateSecret_WithBinary(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	_, err := b.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{
		Name:         "binary-secret",
		SecretBinary: []byte{0x01, 0x02, 0x03},
	})
	require.NoError(t, err)

	val, err := b.GetSecretValue(context.Background(), &secretsmanager.GetSecretValueInput{SecretID: "binary-secret"})
	require.NoError(t, err)
	assert.Equal(t, []byte{0x01, 0x02, 0x03}, val.SecretBinary)
	assert.Empty(t, val.SecretString)
}

func TestCreateSecret_ClientRequestTokenBecomesVersionID(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	out, err := b.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{
		Name:               "token-version",
		SecretString:       "v",
		ClientRequestToken: "my-token-abc",
	})
	require.NoError(t, err)
	assert.Equal(t, "my-token-abc", out.VersionID)

	val, err := b.GetSecretValue(context.Background(), &secretsmanager.GetSecretValueInput{
		SecretID:  "token-version",
		VersionID: "my-token-abc",
	})
	require.NoError(t, err)
	assert.Equal(t, "my-token-abc", val.VersionID)
}

func TestCreateSecret_WithoutValue_NoVersionID(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	out, err := b.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{Name: "no-value"})
	require.NoError(t, err)
	assert.Empty(t, out.VersionID, "no version is created when no value is provided")
}

func TestCreateSecret_ARNFormat(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	out, err := b.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{
		Name:         "arn-check",
		SecretString: "v",
	})
	require.NoError(t, err)
	assert.Contains(t, out.ARN, "arn:aws:secretsmanager:")
	assert.Contains(t, out.ARN, "arn-check")
}

func TestCreateSecret_DuplicateNameReturnsResourceExistsException(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	_, err := b.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{Name: "dup", SecretString: "v"})
	require.NoError(t, err)

	_, err = b.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{Name: "dup", SecretString: "v"})
	require.ErrorIs(t, err, secretsmanager.ErrSecretAlreadyExists)
}

func TestCreateSecret_DuplicateHTTPStatus(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	h := secretsmanager.NewHandler(b)

	rec := doR1Request(t, h, "secretsmanager.CreateSecret",
		`{"Name":"dup2","SecretString":"v"}`)
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doR1Request(t, h, "secretsmanager.CreateSecret",
		`{"Name":"dup2","SecretString":"v"}`)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp secretsmanager.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "ResourceExistsException", errResp.Type)
}

func TestCreateSecret_TagCountLimit(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	tags := make([]secretsmanager.Tag, 51)
	for i := range tags {
		tags[i] = secretsmanager.Tag{Key: fmt.Sprintf("key%d", i), Value: "v"}
	}

	_, err := b.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{
		Name:         "too-many-tags",
		SecretString: "v",
		Tags:         tags,
	})
	require.ErrorIs(t, err, secretsmanager.ErrInvalidParameter, "must reject >50 tags at create time")
}

func TestCreateSecret_Exactly50TagsAllowed(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	tags := make([]secretsmanager.Tag, 50)
	for i := range tags {
		tags[i] = secretsmanager.Tag{Key: fmt.Sprintf("key%d", i), Value: "v"}
	}

	_, err := b.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{
		Name:         "max-tags",
		SecretString: "v",
		Tags:         tags,
	})
	require.NoError(t, err)
}

func TestCreateSecret_CreatedDateSet(t *testing.T) {
	t.Parallel()

	before := time.Now()
	b := secretsmanager.NewInMemoryBackend()
	_, err := b.CreateSecret(
		context.Background(),
		&secretsmanager.CreateSecretInput{Name: "ts-check", SecretString: "v"},
	)
	require.NoError(t, err)

	desc, err := b.DescribeSecret(context.Background(), &secretsmanager.DescribeSecretInput{SecretID: "ts-check"})
	require.NoError(t, err)
	require.NotNil(t, desc.CreatedDate)
	// UnixTimeFloat stores nanoseconds/1e9; recover with int64(f*1e9) nanoseconds.
	created := time.Unix(0, int64(*desc.CreatedDate*1e9))
	assert.False(t, created.Before(before.Add(-time.Second)),
		"CreatedDate must be at or after test start (within 1s tolerance)")
}

// ---------------------------------------------------------------------------
// ClientRequestToken idempotency
// ---------------------------------------------------------------------------

// TestCreateSecret_ClientRequestTokenIdempotency verifies the real AWS CreateSecret
// idempotency contract documented on CreateSecretInput.ClientRequestToken
// (aws-sdk-go-v2/service/secretsmanager/api_op_CreateSecret.go):
//
//   - If the token isn't already associated with a version, a new version is created.
//   - If a version with this token exists and its content matches the request, the
//     request is ignored (idempotent retry succeeds without creating anything new).
//   - If a version with this token exists but its content differs, the request fails
//     (CreateSecret cannot modify an existing version).
//
// This mock previously ignored ClientRequestToken entirely on name collisions, always
// returning ResourceExistsException even for a verbatim retry of a prior successful call.
func TestCreateSecret_ClientRequestTokenIdempotency(t *testing.T) {
	t.Parallel()

	cases := []struct {
		wantErrIs      error
		name           string
		retryToken     string
		retrySecretStr string
		wantSameVerID  bool
	}{
		{
			name:           "same_token_same_content_is_ignored",
			retryToken:     "req-token-1",
			retrySecretStr: "hunter2",
			wantSameVerID:  true,
		},
		{
			name:           "same_token_different_content_fails",
			retryToken:     "req-token-1",
			retrySecretStr: "different-value",
			wantErrIs:      secretsmanager.ErrInvalidParameter,
		},
		{
			name:           "different_token_fails_as_resource_exists",
			retryToken:     "req-token-2",
			retrySecretStr: "hunter2",
			wantErrIs:      secretsmanager.ErrSecretAlreadyExists,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := secretsmanager.NewInMemoryBackend()
			secretName := "idempotent-" + tc.name

			first, err := b.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{
				Name:               secretName,
				SecretString:       "hunter2",
				ClientRequestToken: "req-token-1",
			})
			require.NoError(t, err)

			second, err := b.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{
				Name:               secretName,
				SecretString:       tc.retrySecretStr,
				ClientRequestToken: tc.retryToken,
			})

			if tc.wantErrIs != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.wantErrIs)

				return
			}

			require.NoError(t, err)
			if tc.wantSameVerID {
				assert.Equal(t, first.VersionID, second.VersionID, "idempotent retry must return the same VersionId")
				assert.Equal(t, first.ARN, second.ARN)
			}

			// The idempotent retry must not have created a second version.
			out, err := b.ListSecretVersionIDs(context.Background(), &secretsmanager.ListSecretVersionIDsInput{
				SecretID: secretName,
			})
			require.NoError(t, err)
			assert.Len(t, out.Versions, 1, "idempotent retry must not create a new version")
		})
	}
}

// ---------------------------------------------------------------------------
// AddReplicaRegions (create-time replication)
// ---------------------------------------------------------------------------

// TestCreateSecret_ReplicaRegionsAddedCreatesReplication verifies AddReplicaRegions in
// CreateSecretInput creates replication entries immediately.
func TestCreateSecret_ReplicaRegionsAddedCreatesReplication(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	out, err := b.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{
		Name:         "create-with-replicas",
		SecretString: "value",
		AddReplicaRegions: []secretsmanager.ReplicaRegion{
			{Region: "eu-west-1"},
			{Region: "ap-southeast-1", KmsKeyID: "alias/my-key"},
		},
	})
	require.NoError(t, err)
	require.Len(t, out.ReplicationStatus, 2, "CreateSecret must return ReplicationStatus for each replica")

	regions := make(map[string]secretsmanager.ReplicationStatusType)
	for _, r := range out.ReplicationStatus {
		regions[r.Region] = r
	}

	assert.Contains(t, regions, "eu-west-1")
	assert.Contains(t, regions, "ap-southeast-1")
	assert.Equal(t, "alias/my-key", regions["ap-southeast-1"].KmsKeyID)

	// DescribeSecret should also show the replication status.
	desc, err := b.DescribeSecret(
		context.Background(),
		&secretsmanager.DescribeSecretInput{SecretID: "create-with-replicas"},
	)
	require.NoError(t, err)
	require.Len(t, desc.ReplicationStatus, 2)
}

// TestCreateSecret_ReplicaRegionsAddedWithValueSyncsInSync verifies that when a secret is
// created with an initial value the replication status transitions to InSync.
func TestCreateSecret_ReplicaRegionsAddedWithValueSyncsInSync(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	_, err := b.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{
		Name:         "replica-insync",
		SecretString: "secret-value",
		AddReplicaRegions: []secretsmanager.ReplicaRegion{
			{Region: "us-west-2"},
		},
	})
	require.NoError(t, err)

	desc, err := b.DescribeSecret(context.Background(), &secretsmanager.DescribeSecretInput{SecretID: "replica-insync"})
	require.NoError(t, err)
	require.Len(t, desc.ReplicationStatus, 1)
	assert.Equal(t, "InSync", desc.ReplicationStatus[0].Status)
}

// TestCreateSecret_ReplicaRegionsAddedIsReadableInReplicaRegion confirms
// CreateSecret's AddReplicaRegions goes through the same real-replica path as
// ReplicateSecretToRegions -- the region is not just bookkept as InSync while
// leaving GetSecretValue in that region 404ing.
func TestCreateSecret_ReplicaRegionsAddedIsReadableInReplicaRegion(t *testing.T) {
	t.Parallel()

	h := newSMHandler()

	createRec := doSMRequest(t, h, "secretsmanager.CreateSecret",
		`{"Name":"replica-created-readable","SecretString":"secret-value",`+
			`"AddReplicaRegions":[{"Region":"us-west-2"}]}`)
	require.Equal(t, http.StatusOK, createRec.Code)

	getRec := doSMRequestInRegion(t, h, "us-west-2", "secretsmanager.GetSecretValue",
		`{"SecretId":"replica-created-readable"}`)
	require.Equal(t, http.StatusOK, getRec.Code)

	var out secretsmanager.GetSecretValueOutput
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &out))
	assert.Equal(t, "secret-value", out.SecretString)
}

// TestCreateSecret_ReplicaRegionsAddedNoValue replication status stays InProgress / Failed
// when the secret has no initial value.
func TestCreateSecret_ReplicaRegionsAddedNoValue(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	out, err := b.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{
		Name: "replica-no-value",
		AddReplicaRegions: []secretsmanager.ReplicaRegion{
			{Region: "ca-central-1"},
		},
	})
	require.NoError(t, err)
	// When there is no current version replication must not be InSync.
	require.Len(t, out.ReplicationStatus, 1)
	assert.NotEqual(t, "InSync", out.ReplicationStatus[0].Status)
}

// TestCreateSecret_NoReplicaRegionsAddedReturnsEmptyStatus verifies no extra state is created
// when AddReplicaRegions is omitted.
func TestCreateSecret_NoReplicaRegionsAddedReturnsEmptyStatus(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	out, err := b.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{
		Name:         "no-replicas",
		SecretString: "v",
	})
	require.NoError(t, err)
	assert.Empty(t, out.ReplicationStatus)
	assert.Equal(t, 0, secretsmanager.ReplicationConfigCount(b))
}

// TestCreateSecret_ReplicaRegionsAddedHTTP verifies the HTTP layer parses and returns
// AddReplicaRegions correctly.
func TestCreateSecret_ReplicaRegionsAddedHTTP(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	h := secretsmanager.NewHandler(b)

	body, _ := json.Marshal(map[string]any{
		"Name":         "http-replicas",
		"SecretString": "v",
		"AddReplicaRegions": []map[string]string{
			{"Region": "eu-central-1"},
		},
	})
	rec := doR1Request(t, h, "secretsmanager.CreateSecret", string(body))
	require.Equal(t, http.StatusOK, rec.Code)

	var out secretsmanager.CreateSecretOutput
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.Len(t, out.ReplicationStatus, 1)
	assert.Equal(t, "eu-central-1", out.ReplicationStatus[0].Region)
}

// TestCreateSecret_ReplicaRegionsAddedMultipleHTTP verifies that multiple replica regions
// specified at create time are all present in DescribeSecret output.
func TestCreateSecret_ReplicaRegionsAddedMultipleHTTP(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	h := secretsmanager.NewHandler(b)

	body, _ := json.Marshal(map[string]any{
		"Name":         "multi-rep",
		"SecretString": "v",
		"AddReplicaRegions": []map[string]string{
			{"Region": "us-west-1"},
			{"Region": "eu-north-1"},
			{"Region": "ap-east-1"},
		},
	})
	rec := doR1Request(t, h, "secretsmanager.CreateSecret", string(body))
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doR1Request(t, h, "secretsmanager.DescribeSecret", `{"SecretId":"multi-rep"}`)
	require.Equal(t, http.StatusOK, rec.Code)

	var desc secretsmanager.DescribeSecretOutput
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &desc))
	assert.Len(t, desc.ReplicationStatus, 3)
}

// TestCreateSecret_ReplicaRegionsAddedThenMoreReplicas verifies that AddReplicaRegions at
// create time and subsequent ReplicateSecretToRegions calls both contribute correctly.
func TestCreateSecret_ReplicaRegionsAddedThenMoreReplicas(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	_, err := b.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{
		Name:              "grow-replicas",
		SecretString:      "v",
		AddReplicaRegions: []secretsmanager.ReplicaRegion{{Region: "us-west-2"}},
	})
	require.NoError(t, err)

	_, err = b.ReplicateSecretToRegions(context.Background(), &secretsmanager.ReplicateSecretToRegionsInput{
		SecretID:          "grow-replicas",
		AddReplicaRegions: []secretsmanager.ReplicaRegion{{Region: "eu-west-1"}},
	})
	require.NoError(t, err)

	desc, err := b.DescribeSecret(context.Background(), &secretsmanager.DescribeSecretInput{SecretID: "grow-replicas"})
	require.NoError(t, err)
	assert.Len(t, desc.ReplicationStatus, 2)

	regions := make(map[string]bool)
	for _, r := range desc.ReplicationStatus {
		regions[r.Region] = true
	}
	assert.True(t, regions["us-west-2"])
	assert.True(t, regions["eu-west-1"])
}

// ---------------------------------------------------------------------------
// Replica KMS key round-trip
// ---------------------------------------------------------------------------

// TestCreateSecret_KmsKeyStoredAndReturned verifies that the KmsKeyId specified when
// calling ReplicateSecretToRegions on a freshly created secret is preserved and returned
// in the status.
func TestCreateSecret_KmsKeyStoredAndReturned(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	_, err := b.CreateSecret(
		context.Background(),
		&secretsmanager.CreateSecretInput{Name: "kms-rep", SecretString: "v"},
	)
	require.NoError(t, err)

	_, err = b.ReplicateSecretToRegions(context.Background(), &secretsmanager.ReplicateSecretToRegionsInput{
		SecretID: "kms-rep",
		AddReplicaRegions: []secretsmanager.ReplicaRegion{
			{Region: "eu-west-1", KmsKeyID: "arn:aws:kms:eu-west-1:123456789012:key/abc-123"},
		},
	})
	require.NoError(t, err)

	desc, err := b.DescribeSecret(context.Background(), &secretsmanager.DescribeSecretInput{SecretID: "kms-rep"})
	require.NoError(t, err)
	require.Len(t, desc.ReplicationStatus, 1)
	assert.Equal(t, "arn:aws:kms:eu-west-1:123456789012:key/abc-123",
		desc.ReplicationStatus[0].KmsKeyID, "KMS key must survive round-trip")
}

// TestCreateSecret_CreateWithKmsKeyPreserved verifies that KmsKeyId in AddReplicaRegions
// at create time is returned in the output and persisted.
func TestCreateSecret_CreateWithKmsKeyPreserved(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	createOut, err := b.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{
		Name:         "create-kms-rep",
		SecretString: "v",
		AddReplicaRegions: []secretsmanager.ReplicaRegion{
			{Region: "us-west-2", KmsKeyID: "alias/replica-key"},
		},
	})
	require.NoError(t, err)
	require.Len(t, createOut.ReplicationStatus, 1)
	assert.Equal(t, "alias/replica-key", createOut.ReplicationStatus[0].KmsKeyID)

	// Verify persistence in DescribeSecret.
	desc, err := b.DescribeSecret(context.Background(), &secretsmanager.DescribeSecretInput{SecretID: "create-kms-rep"})
	require.NoError(t, err)
	require.Len(t, desc.ReplicationStatus, 1)
	assert.Equal(t, "alias/replica-key", desc.ReplicationStatus[0].KmsKeyID)
}

// ---------------------------------------------------------------------------
// Soft-deleted / active name collisions
// ---------------------------------------------------------------------------

// TestCreateSecret_DeletedNameCollision verifies that CreateSecret returns
// ErrSecretDeleted (InvalidRequestException) when a secret with the same name is
// pending deletion. AWS distinguishes the two cases: active-name → ResourceExistsException,
// deleted-name → InvalidRequestException.
func TestCreateSecret_DeletedNameCollision(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup   func(*testing.T, *secretsmanager.InMemoryBackend)
		wantFn  func(*testing.T, error)
		name    string
		newName string
	}{
		{
			name:    "deleted_name_returns_invalid_request",
			newName: "cs-del-collision",
			setup: func(t *testing.T, b *secretsmanager.InMemoryBackend) {
				t.Helper()
				_, err := b.CreateSecret(
					context.Background(),
					&secretsmanager.CreateSecretInput{Name: "cs-del-collision", SecretString: "v"},
				)
				require.NoError(t, err)
				_, err = b.DeleteSecret(
					context.Background(),
					&secretsmanager.DeleteSecretInput{SecretID: "cs-del-collision"},
				)
				require.NoError(t, err)
			},
			wantFn: func(t *testing.T, err error) {
				t.Helper()
				require.ErrorIs(t, err, secretsmanager.ErrSecretDeleted,
					"soft-deleted name collision must return ErrSecretDeleted (InvalidRequestException)")
				require.NotErrorIs(t, err, secretsmanager.ErrSecretAlreadyExists,
					"must NOT return ResourceExistsException for deleted-name collision")
			},
		},
		{
			name:    "active_name_still_returns_resource_exists",
			newName: "cs-active-collision",
			setup: func(t *testing.T, b *secretsmanager.InMemoryBackend) {
				t.Helper()
				_, err := b.CreateSecret(
					context.Background(),
					&secretsmanager.CreateSecretInput{Name: "cs-active-collision", SecretString: "v"},
				)
				require.NoError(t, err)
			},
			wantFn: func(t *testing.T, err error) {
				t.Helper()
				require.ErrorIs(t, err, secretsmanager.ErrSecretAlreadyExists,
					"active-name collision must still return ErrSecretAlreadyExists")
			},
		},
		{
			name:    "force_deleted_name_allows_recreation",
			newName: "cs-force-del",
			setup: func(t *testing.T, b *secretsmanager.InMemoryBackend) {
				t.Helper()
				_, err := b.CreateSecret(
					context.Background(),
					&secretsmanager.CreateSecretInput{Name: "cs-force-del", SecretString: "v"},
				)
				require.NoError(t, err)
				_, err = b.DeleteSecret(context.Background(), &secretsmanager.DeleteSecretInput{
					SecretID:                   "cs-force-del",
					ForceDeleteWithoutRecovery: true,
				})
				require.NoError(t, err)
			},
			wantFn: func(t *testing.T, err error) {
				t.Helper()
				require.NoError(t, err, "force-deleted secret name must be available for reuse")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := secretsmanager.NewInMemoryBackend()
			tt.setup(t, b)

			_, err := b.CreateSecret(
				context.Background(),
				&secretsmanager.CreateSecretInput{Name: tt.newName, SecretString: "new"},
			)
			tt.wantFn(t, err)
		})
	}
}

// TestCreateSecret_DeletedNameCollision_HTTP verifies that the HTTP handler
// returns 400 InvalidRequestException (not ResourceExistsException) for a deleted-name collision.
func TestCreateSecret_DeletedNameCollision_HTTP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup       func(*testing.T, *secretsmanager.InMemoryBackend)
		name        string
		body        string
		wantErrType string
		wantStatus  int
	}{
		{
			name: "deleted_name_returns_400_InvalidRequestException",
			setup: func(t *testing.T, b *secretsmanager.InMemoryBackend) {
				t.Helper()
				_, err := b.CreateSecret(
					context.Background(),
					&secretsmanager.CreateSecretInput{Name: "cs-http-del", SecretString: "v"},
				)
				require.NoError(t, err)
				_, err = b.DeleteSecret(
					context.Background(),
					&secretsmanager.DeleteSecretInput{SecretID: "cs-http-del"},
				)
				require.NoError(t, err)
			},
			body:        `{"Name":"cs-http-del","SecretString":"new"}`,
			wantStatus:  http.StatusBadRequest,
			wantErrType: "InvalidRequestException",
		},
		{
			name: "active_name_returns_400_ResourceExistsException",
			setup: func(t *testing.T, b *secretsmanager.InMemoryBackend) {
				t.Helper()
				_, err := b.CreateSecret(
					context.Background(),
					&secretsmanager.CreateSecretInput{Name: "cs-http-active", SecretString: "v"},
				)
				require.NoError(t, err)
			},
			body:        `{"Name":"cs-http-active","SecretString":"new"}`,
			wantStatus:  http.StatusBadRequest,
			wantErrType: "ResourceExistsException",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := secretsmanager.NewInMemoryBackend()
			tt.setup(t, b)

			h := secretsmanager.NewHandler(b)
			rec := doR1Request(t, h, "secretsmanager.CreateSecret", tt.body)

			assert.Equal(t, tt.wantStatus, rec.Code)

			var errResp secretsmanager.ErrorResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
			assert.Equal(t, tt.wantErrType, errResp.Type)
		})
	}
}

// ---------------------------------------------------------------------------
// Binary secrets and SecretString/SecretBinary mutual exclusivity
// ---------------------------------------------------------------------------

func TestCreateSecret_BinaryValueCreateAndGet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		binary []byte
	}{
		{name: "arbitrary_bytes", binary: []byte{0x00, 0xFF, 0xAB, 0xCD, 0x42}},
		{name: "utf8_bytes", binary: []byte("binary secret value")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := secretsmanager.NewInMemoryBackend()
			ctx := context.Background()

			_, err := b.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
				Name:         "bin-" + tc.name,
				SecretBinary: tc.binary,
			})
			require.NoError(t, err)

			out, err := b.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretID: "bin-" + tc.name})
			require.NoError(t, err)
			assert.Equal(t, tc.binary, out.SecretBinary)
			assert.Empty(t, out.SecretString, "SecretString must be empty when binary was stored")
		})
	}
}

// TestCreateSecret_StringAndBinaryMutuallyExclusive verifies that the backend rejects a
// CreateSecret call providing both SecretString and SecretBinary. See also
// TestCreateSecret_BothSecretStringAndBinaryRejected below, which covers the same rule at
// the HTTP-handler layer (kept distinct since they exercise different code paths).
func TestCreateSecret_StringAndBinaryMutuallyExclusive(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	ctx := context.Background()

	_, err := b.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
		Name:         "mutual-excl",
		SecretString: "str",
		SecretBinary: []byte("bin"),
	})
	require.Error(t, err, "specifying both SecretString and SecretBinary must fail")
}

// TestCreateSecret_BothSecretStringAndBinaryRejected verifies that CreateSecret
// rejects requests providing both SecretString and SecretBinary at the HTTP layer.
func TestCreateSecret_BothSecretStringAndBinaryRejected(t *testing.T) {
	t.Parallel()

	h := newSMHandler()

	rec := doSMRequest(t, h, "secretsmanager.CreateSecret",
		`{"Name":"both-at-create","SecretString":"str","SecretBinary":"YmluYXJ5"}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code,
		"CreateSecret with both SecretString and SecretBinary must return 400; body: %s", rec.Body.String())
}

// ---------------------------------------------------------------------------
// Backend scenarios (ported from table-style subtests)
// ---------------------------------------------------------------------------

func TestCreateSecret_BackendScenarios(t *testing.T) {
	t.Parallel()

	t.Run("WithStringValue", func(t *testing.T) {
		t.Parallel()

		backend := secretsmanager.NewInMemoryBackend()

		out, err := backend.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{
			Name:         "my-secret",
			Description:  "a test secret",
			SecretString: "mysecretvalue",
		})

		require.NoError(t, err)
		assert.NotEmpty(t, out.ARN)
		assert.Equal(t, "my-secret", out.Name)
		assert.NotEmpty(t, out.VersionID)
	})

	t.Run("WithoutValue", func(t *testing.T) {
		t.Parallel()

		backend := secretsmanager.NewInMemoryBackend()

		out, err := backend.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{
			Name: "empty-secret",
		})

		require.NoError(t, err)
		assert.NotEmpty(t, out.ARN)
		assert.Empty(t, out.VersionID) // no version when no value
	})

	t.Run("DuplicateNameFails", func(t *testing.T) {
		t.Parallel()

		backend := secretsmanager.NewInMemoryBackend()
		_, _ = backend.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{Name: "dup-secret"})

		_, err := backend.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{Name: "dup-secret"})
		require.ErrorIs(t, err, secretsmanager.ErrSecretAlreadyExists)
	})

	t.Run("WithTags", func(t *testing.T) {
		t.Parallel()

		backend := secretsmanager.NewInMemoryBackend()

		out, err := backend.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{
			Name: "tagged-secret",
			Tags: []secretsmanager.Tag{
				{Key: "env", Value: "test"},
				{Key: "team", Value: "platform"},
			},
		})

		require.NoError(t, err)
		assert.NotEmpty(t, out.ARN)
	})
}

// ---------------------------------------------------------------------------
// Secret name validation via HTTP. NOTE: TestRefinement1_KmsKeyIdRoundTrip and
// TestRefinement1_CreateSecretClientRequestToken were SKIPPED as already equivalent
// to TestCreateSecret_WithKmsKeyID and TestCreateSecret_ClientRequestTokenBecomesVersionID
// above.
// ---------------------------------------------------------------------------

// TestCreateSecret_NameValidation verifies CreateSecret rejects invalid names via HTTP.
func TestCreateSecret_NameValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		secretName string
		wantStatus int
	}{
		{
			name:       "valid_path_name",
			secretName: "prod/db/password",
			wantStatus: http.StatusOK,
		},
		{
			name:       "valid_alphanumeric",
			secretName: "mySecret123",
			wantStatus: http.StatusOK,
		},
		{
			name:       "valid_with_special_chars",
			secretName: "my-secret_name.v1@env=prod+extra",
			wantStatus: http.StatusOK,
		},
		{
			name:       "empty_name",
			secretName: "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid_space",
			secretName: "my secret",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid_dollar",
			secretName: "my$secret",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid_exclamation",
			secretName: "my!secret",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := secretsmanager.NewInMemoryBackend()
			h := secretsmanager.NewHandler(b)

			body, _ := json.Marshal(map[string]string{"Name": tt.secretName, "SecretString": "v"})
			rec := doR1Request(t, h, "secretsmanager.CreateSecret", string(body))
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestCreateSecret_CreateDescribeRoundtrip verifies a full create -> describe cycle
// with all metadata fields populated correctly.
func TestCreateSecret_CreateDescribeRoundtrip(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	h := secretsmanager.NewHandler(b)

	createBody := map[string]any{
		"Name":         "roundtrip/secret",
		"Description":  "roundtrip test",
		"SecretString": "roundtrip-value",
		"KmsKeyId":     "alias/roundtrip",
		"Tags": []map[string]string{
			{"Key": "env", "Value": "test"},
		},
	}

	body, _ := json.Marshal(createBody)
	rec := doR1Request(t, h, "secretsmanager.CreateSecret", string(body))
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doR1Request(t, h, "secretsmanager.DescribeSecret", `{"SecretId":"roundtrip/secret"}`)
	require.Equal(t, http.StatusOK, rec.Code)

	var desc secretsmanager.DescribeSecretOutput
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &desc))
	assert.Equal(t, "roundtrip/secret", desc.Name)
	assert.Equal(t, "roundtrip test", desc.Description)
	assert.Equal(t, "alias/roundtrip", desc.KmsKeyID)
	assert.NotNil(t, desc.LastChangedDate)
	assert.False(t, desc.RotationEnabled)
	assert.Contains(t, desc.ARN, "secret:roundtrip/secret-")
}

// TestCreateSecret_TypeAcceptedAndEchoed verifies that CreateSecret's Type field
// (aws-sdk-go-v2/service/secretsmanager@v1.44.4 api_op_CreateSecret.go's
// CreateSecretInput.Type, "the exact string that identifies the partner that
// holds the external secret") is stored and echoed back by DescribeSecret and
// ListSecrets (gopherstack-9wuh: previously accepted on the wire then silently
// dropped, since gopherstack's CreateSecretInput/DescribeSecretOutput carried no
// Type field at all).
func TestCreateSecret_TypeAcceptedAndEchoed(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	_, err := b.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{
		Name:         "mes-secret",
		SecretString: "v",
		Type:         "SomePartner",
	})
	require.NoError(t, err)

	desc, err := b.DescribeSecret(context.Background(), &secretsmanager.DescribeSecretInput{SecretID: "mes-secret"})
	require.NoError(t, err)
	assert.Equal(t, "SomePartner", desc.Type)

	list, err := b.ListSecrets(context.Background(), &secretsmanager.ListSecretsInput{})
	require.NoError(t, err)
	require.Len(t, list.SecretList, 1)
	assert.Equal(t, "SomePartner", list.SecretList[0].Type)
}

// TestUpdateSecret_TypeAcceptedAndEchoed verifies UpdateSecret's Type field
// (api_op_UpdateSecret.go's UpdateSecretInput.Type, the same wire field as
// CreateSecret's) is stored and echoed, mirroring how Description/KmsKeyId are
// already handled.
func TestUpdateSecret_TypeAcceptedAndEchoed(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	_, err := b.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{
		Name:         "mes-update",
		SecretString: "v",
	})
	require.NoError(t, err)

	_, err = b.UpdateSecret(context.Background(), &secretsmanager.UpdateSecretInput{
		SecretID: "mes-update",
		Type:     "AnotherPartner",
	})
	require.NoError(t, err)

	desc, err := b.DescribeSecret(context.Background(), &secretsmanager.DescribeSecretInput{SecretID: "mes-update"})
	require.NoError(t, err)
	assert.Equal(t, "AnotherPartner", desc.Type)
}

// TestRotateSecret_ExternalSecretRotationFieldsAcceptedAndEchoed verifies
// RotateSecret's ExternalSecretRotationRoleArn and ExternalSecretRotationMetadata
// fields (api_op_RotateSecret.go's RotateSecretInput, "the metadata needed to
// successfully rotate a managed external secret") are stored and echoed by
// DescribeSecret and ListSecrets (gopherstack-9wuh).
func TestRotateSecret_ExternalSecretRotationFieldsAcceptedAndEchoed(t *testing.T) {
	t.Parallel()

	b := secretsmanager.NewInMemoryBackend()
	_, err := b.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{
		Name:         "mes-rotate",
		SecretString: "v",
	})
	require.NoError(t, err)

	_, err = b.RotateSecret(context.Background(), &secretsmanager.RotateSecretInput{
		SecretID:                      "mes-rotate",
		RotationLambdaARN:             testLambdaARN,
		ExternalSecretRotationRoleArn: "arn:aws:iam::000000000000:role/mes-rotator",
		ExternalSecretRotationMetadata: []secretsmanager.ExternalSecretRotationMetadataItem{
			{Key: "partnerAccountId", Value: "12345"},
		},
	})
	require.NoError(t, err)

	desc, err := b.DescribeSecret(context.Background(), &secretsmanager.DescribeSecretInput{SecretID: "mes-rotate"})
	require.NoError(t, err)
	assert.Equal(t, "arn:aws:iam::000000000000:role/mes-rotator", desc.ExternalSecretRotationRoleArn)
	require.Len(t, desc.ExternalSecretRotationMetadata, 1)
	assert.Equal(t, "partnerAccountId", desc.ExternalSecretRotationMetadata[0].Key)
	assert.Equal(t, "12345", desc.ExternalSecretRotationMetadata[0].Value)

	list, err := b.ListSecrets(context.Background(), &secretsmanager.ListSecretsInput{})
	require.NoError(t, err)
	require.Len(t, list.SecretList, 1)
	assert.Equal(t, "arn:aws:iam::000000000000:role/mes-rotator", list.SecretList[0].ExternalSecretRotationRoleArn)
	require.Len(t, list.SecretList[0].ExternalSecretRotationMetadata, 1)
	assert.Equal(t, "partnerAccountId", list.SecretList[0].ExternalSecretRotationMetadata[0].Key)
}

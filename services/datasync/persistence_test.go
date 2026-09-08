package datasync_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/datasync"
)

// persistenceFixtureIDs carries every identifier newPersistenceTestBackend
// creates, so the round-trip assertions in
// TestInMemoryBackend_SnapshotRestore_FullState can look each resource back
// up after Restore without re-deriving IDs.
type persistenceFixtureIDs struct {
	agentArn       string
	sourceArn      string
	destinationArn string
	taskArn        string
	executionArn   string
}

// newPersistenceTestBackend creates a backend with one populated entry in
// every store.Table (agents, locations, tasks, executions -- and,
// transitively, the executionsByTask secondary store.Index) plus the one raw
// map left un-converted (tags), so a Snapshot from it exercises the entire
// persisted surface of the backend.
func newPersistenceTestBackend(t *testing.T) (*datasync.InMemoryBackend, persistenceFixtureIDs) {
	t.Helper()

	b := datasync.NewInMemoryBackend("000000000000", "us-east-1")

	agent, err := b.CreateAgent("agent1", "", map[string]string{"env": "test"})
	require.NoError(t, err)

	source, err := b.CreateLocationS3(
		"/source", "arn:aws:s3:::src-bucket", "STANDARD",
		datasync.S3Config{BucketAccessRoleArn: "arn:aws:iam::000000000000:role/r"},
		nil, nil,
	)
	require.NoError(t, err)

	destination, err := b.CreateLocationS3(
		"/dest", "arn:aws:s3:::dst-bucket", "STANDARD",
		datasync.S3Config{BucketAccessRoleArn: "arn:aws:iam::000000000000:role/r"},
		nil, nil,
	)
	require.NoError(t, err)

	settings := datasync.TaskSettings{
		Options:  map[string]any{"LogLevel": "TRANSFER"},
		Schedule: &datasync.TaskSchedule{ScheduleExpression: "rate(1 hours)", Status: "ENABLED"},
		Excludes: []datasync.FilterRule{{FilterType: "SIMPLE_PATTERN", Value: "/tmp"}},
	}

	task, err := b.CreateTask(source.LocationArn, destination.LocationArn, "task1", "", settings, nil)
	require.NoError(t, err)

	execution, err := b.StartTaskExecution(task.TaskArn, datasync.TaskExecutionOverrides{}, nil)
	require.NoError(t, err)

	require.NoError(t, b.TagResource(task.TaskArn, map[string]string{"owner": "alice"}))

	return b, persistenceFixtureIDs{
		agentArn:       agent.AgentArn,
		sourceArn:      source.LocationArn,
		destinationArn: destination.LocationArn,
		taskArn:        task.TaskArn,
		executionArn:   execution.TaskExecutionArn,
	}
}

// TestInMemoryBackend_SnapshotRestore_FullState round-trips every store.Table
// (agents, locations, tasks, executions), the executionsByTask secondary
// store.Index derived from executions, and the one raw map left un-converted
// (tags) through Snapshot -> Restore into a fresh backend.
func TestInMemoryBackend_SnapshotRestore_FullState(t *testing.T) {
	t.Parallel()

	original, ids := newPersistenceTestBackend(t)

	snap := original.Snapshot(t.Context())
	require.NotNil(t, snap)

	fresh := datasync.NewInMemoryBackend("000000000000", "us-east-1")
	require.NoError(t, fresh.Restore(t.Context(), snap))

	// agents table.
	agent, err := fresh.DescribeAgent(ids.agentArn)
	require.NoError(t, err)
	assert.Equal(t, "agent1", agent.Name)

	// locations table.
	source, err := fresh.DescribeLocationS3(ids.sourceArn)
	require.NoError(t, err)
	assert.Equal(t, "src-bucket", extractBucket(t, source.LocationURI))

	destination, err := fresh.DescribeLocationS3(ids.destinationArn)
	require.NoError(t, err)
	assert.Equal(t, "dst-bucket", extractBucket(t, destination.LocationURI))

	// tasks table.
	task, err := fresh.DescribeTask(ids.taskArn)
	require.NoError(t, err)
	assert.Equal(t, "task1", task.Name)
	assert.Equal(t, "TRANSFER", task.Options["LogLevel"])
	require.NotNil(t, task.Schedule)
	assert.Equal(t, "rate(1 hours)", task.Schedule.ScheduleExpression)
	require.Len(t, task.Excludes, 1)
	assert.Equal(t, "/tmp", task.Excludes[0].Value)

	// executions table.
	execution, err := fresh.DescribeTaskExecution(ids.executionArn)
	require.NoError(t, err)
	assert.Equal(t, ids.executionArn, execution.TaskExecutionArn)

	// executionsByTask index (exercised via the taskArn-scoped list).
	executions, _, err := fresh.ListTaskExecutions(ids.taskArn, 0, "")
	require.NoError(t, err)
	require.Len(t, executions, 1)
	assert.Equal(t, ids.executionArn, executions[0].TaskExecutionArn)

	// tags raw map (left un-converted, persisted alongside the tables).
	tags, _, err := fresh.ListTagsForResource(ids.taskArn, 0, "")
	require.NoError(t, err)
	assert.Equal(t, "alice", tags["owner"])

	// Sanity: account/region carried through too.
	assert.Equal(t, "us-east-1", fresh.Region())
	assert.Equal(t, "000000000000", fresh.AccountID())
}

// extractBucket is a small local helper for asserting on the s3://bucket/...
// LocationURI shape without duplicating the backend's own URI-building logic.
func extractBucket(t *testing.T, locationURI string) string {
	t.Helper()

	const prefix = "s3://"
	require.True(t, len(locationURI) > len(prefix) && locationURI[:len(prefix)] == prefix)

	rest := locationURI[len(prefix):]
	for i, c := range rest {
		if c == '/' {
			return rest[:i]
		}
	}

	return rest
}

// TestInMemoryBackend_RestoreVersionMismatch verifies that a snapshot whose
// version doesn't match the current backend (including the pre-Phase-3.3
// format, which decodes with Version == 0) is discarded cleanly rather than
// partially decoded: the backend resets to empty state and Restore returns
// no error.
func TestInMemoryBackend_RestoreVersionMismatch(t *testing.T) {
	t.Parallel()

	b, ids := newPersistenceTestBackend(t)

	// A syntactically valid but version-less/mismatched snapshot.
	err := b.Restore(t.Context(), []byte(`{"version":999,"tables":{}}`))
	require.NoError(t, err)

	assert.Equal(t, 0, datasync.AgentCount(b))
	assert.Equal(t, 0, datasync.LocationCount(b))
	assert.Equal(t, 0, datasync.TaskCount(b))
	assert.Equal(t, 0, datasync.ExecutionCount(b))

	_, err = b.DescribeAgent(ids.agentArn)
	require.ErrorIs(t, err, datasync.ErrNotFound)

	_, err = b.DescribeTask(ids.taskArn)
	require.ErrorIs(t, err, datasync.ErrNotFound)
}

// TestInMemoryBackend_RestoreInvalidData verifies malformed JSON surfaces as
// an error rather than being silently discarded (that path is reserved for a
// syntactically valid but version-mismatched snapshot; see
// TestInMemoryBackend_RestoreVersionMismatch).
func TestInMemoryBackend_RestoreInvalidData(t *testing.T) {
	t.Parallel()

	b := datasync.NewInMemoryBackend("000000000000", "us-east-1")
	err := b.Restore(t.Context(), []byte("not-valid-json"))
	require.Error(t, err)
}

// TestInMemoryBackend_SnapshotRestore_SecretConfigAndKerberos verifies the
// CmkSecretConfig/CustomSecretConfig and SMB Kerberos/DNS fields (added to
// storedSmbConfig/storedXConfig with omitempty -- additive, no snapshot
// version bump needed) survive a Snapshot -> Restore round trip.
func TestInMemoryBackend_SnapshotRestore_SecretConfigAndKerberos(t *testing.T) {
	t.Parallel()

	b := datasync.NewInMemoryBackend("000000000000", "us-east-1")

	loc, err := b.CreateLocationSmb(
		"smb.example.com", "/share", "", "", "", "KERBEROS",
		nil, nil, nil,
		datasync.SmbKerberosConfig{
			KerberosPrincipal: "HOST/user@REALM.ORG",
			KerberosKeytab:    "keytab-bytes",
			KerberosKrb5Conf:  "krb5-conf-bytes",
			DNSIPAddresses:    []string{"10.0.0.1"},
		},
		datasync.SecretConfig{Cmk: &datasync.CmkSecretConfig{
			SecretArn: "arn:aws:secretsmanager:us-east-1:000000000000:secret:s",
			KmsKeyArn: "arn:aws:kms:us-east-1:000000000000:key/k",
		}},
	)
	require.NoError(t, err)

	snap := b.Snapshot(t.Context())
	require.NotNil(t, snap)

	fresh := datasync.NewInMemoryBackend("000000000000", "us-east-1")
	require.NoError(t, fresh.Restore(t.Context(), snap))

	restored, err := fresh.DescribeLocationSmb(loc.LocationArn)
	require.NoError(t, err)
	assert.Equal(t, "HOST/user@REALM.ORG", restored.KerberosPrincipal)
	assert.Equal(t, []string{"10.0.0.1"}, restored.DNSIPAddresses)
	require.NotNil(t, restored.CmkSecretConfig)
	assert.Equal(t, "arn:aws:secretsmanager:us-east-1:000000000000:secret:s", restored.CmkSecretConfig.SecretArn)
	assert.Equal(t, "arn:aws:kms:us-east-1:000000000000:key/k", restored.CmkSecretConfig.KmsKeyArn)
}

// TestHandler_SnapshotRestoreDelegate verifies Handler.Snapshot/Restore
// delegate to the backend. Before Phase 3.3, datasync had no such delegation
// even though InMemoryBackend implemented both -- dead wiring, since nothing
// ever called them through the Handler/Persistable path the rest of the
// fleet uses.
func TestHandler_SnapshotRestoreDelegate(t *testing.T) {
	t.Parallel()

	backend, ids := newPersistenceTestBackend(t)
	h := datasync.NewHandler(backend)

	snap := h.Snapshot(t.Context())
	require.NotNil(t, snap)

	h2 := datasync.NewHandler(datasync.NewInMemoryBackend("000000000000", "us-east-1"))
	require.NoError(t, h2.Restore(t.Context(), snap))

	task, err := h2.Backend.DescribeTask(ids.taskArn)
	require.NoError(t, err)
	assert.Equal(t, "task1", task.Name)
}

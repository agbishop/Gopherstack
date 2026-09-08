package ssoadmin

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCascadeDeleteInstance_PurgesProvisioningState reproduces a leak: a
// permission set force-deleted by DeleteInstance (which has no "no live
// assignments" precondition, unlike DeletePermissionSet) left its
// provisionedAt and assignmentCreationIDs rows behind forever, since neither
// map was touched by cascadeDeleteInstance. Repeated instance create/delete
// cycles would grow both maps without bound.
func TestCascadeDeleteInstance_PurgesProvisioningState(t *testing.T) {
	t.Parallel()

	b := NewInMemoryBackend("111111111111", "us-east-1")

	inst, err := b.CreateInstance("test", "111111111111", "", nil)
	require.NoError(t, err)

	ps, err := b.CreatePermissionSet(inst.InstanceArn, "TestPS", "", "PT1H", "", nil)
	require.NoError(t, err)

	_, err = b.CreateAccountAssignment(inst.InstanceArn, ps.PermissionSetArn, "222222222222", "principal-1", "USER")
	require.NoError(t, err)

	key := assignmentKey(inst.InstanceArn, ps.PermissionSetArn)

	require.NotZero(t, countByPrefix(b.provisionedAt, key+"|"),
		"test setup: expected CreateAccountAssignment to record a provisionedAt row")
	require.NotZero(t, countByPrefix(b.assignmentCreationIDs, key+"|"),
		"test setup: expected CreateAccountAssignment to record an assignmentCreationIDs row")

	require.NoError(t, b.DeleteInstance(inst.InstanceArn))

	assertNoKeyWithPrefix(t, b.provisionedAt, key+"|", "provisionedAt")
	assertNoKeyWithPrefix(t, b.assignmentCreationIDs, key+"|", "assignmentCreationIDs")
}

func countByPrefix[V any](m map[string]V, prefix string) int {
	n := 0

	for k := range m {
		if strings.HasPrefix(k, prefix) {
			n++
		}
	}

	return n
}

func assertNoKeyWithPrefix[V any](t *testing.T, m map[string]V, prefix, name string) {
	t.Helper()

	for k := range m {
		if strings.HasPrefix(k, prefix) {
			t.Errorf("%s leaked key %q after DeleteInstance", name, k)
		}
	}
}

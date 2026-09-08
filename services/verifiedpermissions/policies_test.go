package verifiedpermissions_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/services/verifiedpermissions"
)

func TestBackend_Policy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup   func(*testing.T, *verifiedpermissions.InMemoryBackend) (string, string)
		name    string
		wantErr bool
	}{
		{
			name: "create and get",
			setup: func(t *testing.T, b *verifiedpermissions.InMemoryBackend) (string, string) {
				t.Helper()

				ps, err := b.CreatePolicyStore("desc", nil, "OFF", "", "")
				require.NoError(t, err)

				p, err := b.CreatePolicy(
					ps.PolicyStoreID,
					verifiedpermissions.CreatePolicyParams{
						PolicyType: "STATIC",
						Statement:  "permit(principal, action, resource);",
					},
				)
				require.NoError(t, err)

				return ps.PolicyStoreID, p.PolicyID
			},
			wantErr: false,
		},
		{
			name: "get from non-existent store",
			setup: func(t *testing.T, _ *verifiedpermissions.InMemoryBackend) (string, string) {
				t.Helper()

				return "nonexistent-store", "nonexistent-policy"
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			storeID, policyID := tt.setup(t, b)

			p, err := b.GetPolicy(storeID, policyID)
			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, policyID, p.PolicyID)
			assert.Equal(t, storeID, p.PolicyStoreID)
			assert.Equal(t, "STATIC", p.PolicyType)
		})
	}
}

func TestBackend_ListPolicies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup       func(*testing.T, *verifiedpermissions.InMemoryBackend) string
		name        string
		numPolicies int
		wantErr     bool
	}{
		{
			name: "list multiple policies",
			setup: func(t *testing.T, b *verifiedpermissions.InMemoryBackend) string {
				t.Helper()

				ps, err := b.CreatePolicyStore("desc", nil, "OFF", "", "")
				require.NoError(t, err)

				return ps.PolicyStoreID
			},
			numPolicies: 2,
			wantErr:     false,
		},
		{
			name: "list from non-existent store",
			setup: func(_ *testing.T, _ *verifiedpermissions.InMemoryBackend) string {
				return "nonexistent"
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			storeID := tt.setup(t, b)

			for range tt.numPolicies {
				_, err := b.CreatePolicy(
					storeID,
					verifiedpermissions.CreatePolicyParams{
						PolicyType: "STATIC",
						Statement:  "permit(principal, action, resource);",
					},
				)
				require.NoError(t, err)
			}

			policies, _, err := b.ListPolicies(storeID, verifiedpermissions.ListPoliciesFilter{}, "", 0)
			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Len(t, policies, tt.numPolicies)
		})
	}
}

func TestBackend_UpdatePolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup   func(*testing.T, *verifiedpermissions.InMemoryBackend) (string, string)
		name    string
		newStmt string
		wantErr bool
	}{
		{
			name: "update existing policy",
			setup: func(t *testing.T, b *verifiedpermissions.InMemoryBackend) (string, string) {
				t.Helper()

				ps, err := b.CreatePolicyStore("desc", nil, "OFF", "", "")
				require.NoError(t, err)

				p, err := b.CreatePolicy(
					ps.PolicyStoreID,
					verifiedpermissions.CreatePolicyParams{
						PolicyType: "STATIC",
						Statement:  "permit(principal, action, resource);",
					},
				)
				require.NoError(t, err)

				return ps.PolicyStoreID, p.PolicyID
			},
			newStmt: "forbid(principal, action, resource);",
			wantErr: false,
		},
		{
			name: "update non-existent policy",
			setup: func(_ *testing.T, _ *verifiedpermissions.InMemoryBackend) (string, string) {
				return "nonexistent-store", "nonexistent-policy"
			},
			newStmt: "forbid(principal, action, resource);",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			storeID, policyID := tt.setup(t, b)

			p, err := b.UpdatePolicy(storeID, policyID, verifiedpermissions.UpdatePolicyParams{Statement: tt.newStmt})
			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.newStmt, p.Statement)
		})
	}
}

// TestBackend_UpdatePolicy_TemplateLinkedRejected is the regression test for
// gopherstack-990: real AWS's UpdatePolicy can only update static policies
// ("You can directly update only static policies. To change a
// template-linked policy, you must update the template instead, using
// UpdatePolicyTemplate."). Before the fix, UpdatePolicy silently rebound a
// template-linked policy's principal/resource -- a capability the real API
// doesn't expose at all.
func TestBackend_UpdatePolicy_TemplateLinkedRejected(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	ps, err := b.CreatePolicyStore("desc", nil, "OFF", "", "")
	require.NoError(t, err)

	tmpl, err := b.CreatePolicyTemplate(
		ps.PolicyStoreID, "tmpl", `permit(principal == ?principal, action, resource);`, "", "",
	)
	require.NoError(t, err)

	linked, err := b.CreatePolicy(ps.PolicyStoreID, verifiedpermissions.CreatePolicyParams{
		PolicyType:          "TEMPLATE_LINKED",
		PolicyTemplateID:    tmpl.PolicyTemplateID,
		PrincipalEntityType: "User",
		PrincipalEntityID:   "alice",
	})
	require.NoError(t, err)

	_, err = b.UpdatePolicy(ps.PolicyStoreID, linked.PolicyID, verifiedpermissions.UpdatePolicyParams{
		Statement: "forbid(principal, action, resource);",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, awserr.ErrInvalidParameter)

	unchanged, err := b.GetPolicy(ps.PolicyStoreID, linked.PolicyID)
	require.NoError(t, err)
	assert.Equal(t, "alice", unchanged.PrincipalEntityID, "the policy's binding must be untouched")
}

func TestBackend_DeletePolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup   func(*testing.T, *verifiedpermissions.InMemoryBackend) (string, string)
		name    string
		wantErr bool
	}{
		{
			name: "delete existing",
			setup: func(t *testing.T, b *verifiedpermissions.InMemoryBackend) (string, string) {
				t.Helper()

				ps, err := b.CreatePolicyStore("desc", nil, "OFF", "", "")
				require.NoError(t, err)

				p, err := b.CreatePolicy(
					ps.PolicyStoreID,
					verifiedpermissions.CreatePolicyParams{
						PolicyType: "STATIC",
						Statement:  "permit(principal, action, resource);",
					},
				)
				require.NoError(t, err)

				return ps.PolicyStoreID, p.PolicyID
			},
			wantErr: false,
		},
		{
			// DeletePolicy is documented idempotent: deleting a
			// nonexistent policy (in an existing store) returns success.
			name: "delete non-existent policy",
			setup: func(t *testing.T, b *verifiedpermissions.InMemoryBackend) (string, string) {
				t.Helper()

				ps, err := b.CreatePolicyStore("desc", nil, "OFF", "", "")
				require.NoError(t, err)

				return ps.PolicyStoreID, "nonexistent-policy"
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			storeID, policyID := tt.setup(t, b)

			err := b.DeletePolicy(storeID, policyID)
			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)

			_, err = b.GetPolicy(storeID, policyID)
			require.Error(t, err)
		})
	}
}

func TestBackend_CreatePolicy_NonExistentStore(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	_, err := b.CreatePolicy(
		"nonexistent-store",
		verifiedpermissions.CreatePolicyParams{PolicyType: "STATIC", Statement: "permit(principal, action, resource);"},
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, awserr.ErrNotFound)
}

func TestBackend_GetPolicy_NonExistentPolicyInExistingStore(t *testing.T) {
	t.Parallel()

	b := newTestBackend()

	ps, err := b.CreatePolicyStore("desc", nil, "OFF", "", "")
	require.NoError(t, err)

	_, err = b.GetPolicy(ps.PolicyStoreID, "nonexistent-policy")
	require.Error(t, err)
	assert.ErrorIs(t, err, awserr.ErrNotFound)
}

func TestBackend_DeletePolicy_NonExistentStore(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	err := b.DeletePolicy("nonexistent-store", "nonexistent-policy")
	require.Error(t, err)
	assert.ErrorIs(t, err, awserr.ErrNotFound)
}

// TestBackend_DeletePolicy_IdempotentOnMissingPolicy is the regression test
// for the DeletePolicy idempotency fix: gopherstack-990. Real AWS
// (api_op_DeletePolicy.go doc): "This operation is idempotent; if you
// specify a policy that doesn't exist, the request response returns a
// successful HTTP 200 status code." Unlike DeletePolicyStore,
// ResourceNotFoundException stays in DeletePolicy's modelled error set --
// TestBackend_DeletePolicy_NonExistentStore above proves that case still
// errors -- so the idempotency covers only a missing policy, not a missing
// store.
func TestBackend_DeletePolicy_IdempotentOnMissingPolicy(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	ps, err := b.CreatePolicyStore("desc", nil, "OFF", "", "")
	require.NoError(t, err)

	require.NoError(t, b.DeletePolicy(ps.PolicyStoreID, "nonexistent-policy"))
	require.NoError(t, b.DeletePolicy(ps.PolicyStoreID, "nonexistent-policy"),
		"a second delete of the same missing policy must also succeed")
}

func TestBackend_UpdatePolicy_NonExistentPolicy(t *testing.T) {
	t.Parallel()

	b := newTestBackend()

	ps, err := b.CreatePolicyStore("desc", nil, "OFF", "", "")
	require.NoError(t, err)

	_, err = b.UpdatePolicy(
		ps.PolicyStoreID,
		"nonexistent-policy",
		verifiedpermissions.UpdatePolicyParams{Statement: "forbid(principal, action, resource);"},
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, awserr.ErrNotFound)
}

func TestBackend_BatchGetPolicy_EmptyArrays(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	ps := seedPolicyStore(t, b, "batch store")

	result := b.BatchGetPolicy([]verifiedpermissions.BatchGetPolicyItem{
		{PolicyStoreID: ps.PolicyStoreID, PolicyID: "nonexistent"},
	})

	assert.Empty(t, result.Results)
	assert.Len(t, result.Errors, 1)
}

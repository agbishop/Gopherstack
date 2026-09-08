package organizations_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/organizations"
)

// TestBackend_OULifecycle tests OU create, describe, and delete.
func TestBackend_OULifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		ouName  string
		wantErr bool
	}{
		{
			name:   "create_describe_delete",
			ouName: "dev-ou",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()

			_, _, err := b.CreateOrganization("ALL")
			require.NoError(t, err)

			roots, err := b.ListRoots()
			require.NoError(t, err)
			require.NotEmpty(t, roots)

			rootID := roots[0].ID

			// CreateOrganizationalUnit.
			ou, err := b.CreateOrganizationalUnit(rootID, tt.ouName, nil)
			require.NoError(t, err)
			assert.NotEmpty(t, ou.ID)
			assert.Equal(t, tt.ouName, ou.Name)

			// DescribeOrganizationalUnit.
			desc, err := b.DescribeOrganizationalUnit(ou.ID)
			require.NoError(t, err)
			assert.Equal(t, ou.ID, desc.ID)
			assert.Equal(t, tt.ouName, desc.Name)

			// ListOrganizationalUnitsForParent.
			ous, err := b.ListOrganizationalUnitsForParent(rootID)
			require.NoError(t, err)

			found := false

			for _, o := range ous {
				if o.ID == ou.ID {
					found = true

					break
				}
			}

			assert.True(t, found, "OU should appear in list")

			// UpdateOrganizationalUnit.
			updated, err := b.UpdateOrganizationalUnit(ou.ID, "renamed-ou")
			require.NoError(t, err)
			assert.Equal(t, "renamed-ou", updated.Name)

			// DeleteOrganizationalUnit.
			err = b.DeleteOrganizationalUnit(ou.ID)
			require.NoError(t, err)

			// After deletion, describe should fail.
			_, err = b.DescribeOrganizationalUnit(ou.ID)
			require.Error(t, err)
		})
	}
}

// TestBackend_ListAccountsForParent tests listing accounts under a parent.
func TestBackend_ListAccountsForParent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		expectCount int
	}{
		{
			name:        "lists_accounts_in_root",
			expectCount: 2, // management account + created account
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, rootID := newOrgBackend(t)

			// Create an account (goes to root).
			_, err := b.CreateAccount("child-account", "child@example.com", "", "", nil)
			require.NoError(t, err)

			accts, err := b.ListAccountsForParent(rootID)
			require.NoError(t, err)
			assert.GreaterOrEqual(t, len(accts), tt.expectCount)
		})
	}
}

// TestBackend_ListChildren tests listing children of a parent.
func TestBackend_ListChildren(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		childType string
		wantErr   bool
	}{
		{
			name:      "lists_account_children",
			childType: "ACCOUNT",
		},
		{
			name:      "lists_ou_children",
			childType: "ORGANIZATIONAL_UNIT",
		},
		{
			name:      "invalid_child_type",
			childType: "INVALID",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, rootID := newOrgBackend(t)

			_, err := b.CreateAccount("child-account", "child@example.com", "", "", nil)
			require.NoError(t, err)

			_, err = b.CreateOrganizationalUnit(rootID, "child-ou", nil)
			require.NoError(t, err)

			children, err := b.ListChildren(rootID, tt.childType)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.NotNil(t, children)
		})
	}
}

// TestOrganizationsChildrenIndexConsistency verifies that the accountChildrenByParent
// and ousByParent indexes stay consistent with accountParent / ouParent across account
// and OU lifecycle operations, and that ListChildren returns the indexed results.
func TestOrganizationsChildrenIndexConsistency(t *testing.T) {
	t.Parallel()

	newBackend := func(t *testing.T) (*organizations.InMemoryBackend, string) {
		t.Helper()

		b := organizations.NewInMemoryBackend("123456789012", "us-east-1")
		_, root, err := b.CreateOrganization("ALL")
		require.NoError(t, err)

		return b, root.ID
	}

	tests := []struct {
		run  func(t *testing.T)
		name string
	}{
		{
			name: "account_created_appears_in_list_children",
			run: func(t *testing.T) {
				t.Helper()

				b, rootID := newBackend(t)

				// Create an account — should appear in root's ACCOUNT children.
				status, err := b.CreateAccount("alice", "alice@example.com", "OrganizationAccountAccessRole", "", nil)
				require.NoError(t, err)

				children, err := b.ListChildren(rootID, "ACCOUNT")
				require.NoError(t, err)

				ids := make(map[string]bool, len(children))
				for _, c := range children {
					ids[c.ID] = true
				}

				require.True(t, ids[status.AccountID])
			},
		},
		{
			name: "account_moved_updates_list_children",
			run: func(t *testing.T) {
				t.Helper()

				b, rootID := newBackend(t)

				ou, err := b.CreateOrganizationalUnit(rootID, "Engineering", nil)
				require.NoError(t, err)

				status, err := b.CreateAccount("bob", "bob@example.com", "OrganizationAccountAccessRole", "", nil)
				require.NoError(t, err)

				acctID := status.AccountID

				require.NoError(t, b.MoveAccount(acctID, rootID, ou.ID))

				// Should no longer be under root.
				rootChildren, err := b.ListChildren(rootID, "ACCOUNT")
				require.NoError(t, err)

				for _, c := range rootChildren {
					require.NotEqual(t, acctID, c.ID)
				}

				// Should be under the OU now.
				ouChildren, err := b.ListChildren(ou.ID, "ACCOUNT")
				require.NoError(t, err)

				found := false
				for _, c := range ouChildren {
					if c.ID == acctID {
						found = true
					}
				}

				require.True(t, found, "account not found under OU after MoveAccount")
			},
		},
		{
			name: "account_removed_clears_list_children",
			run: func(t *testing.T) {
				t.Helper()

				b, rootID := newBackend(t)

				// Need an OU to move the account into so we can remove it
				// (root accounts can't be directly removed in some paths; move first).
				ou, err := b.CreateOrganizationalUnit(rootID, "Temp", nil)
				require.NoError(t, err)

				status, err := b.CreateAccount("carol", "carol@example.com", "OrganizationAccountAccessRole", "", nil)
				require.NoError(t, err)

				acctID := status.AccountID

				require.NoError(t, b.MoveAccount(acctID, rootID, ou.ID))
				require.NoError(t, b.RemoveAccountFromOrganization(acctID))

				children, err := b.ListChildren(ou.ID, "ACCOUNT")
				require.NoError(t, err)

				for _, c := range children {
					require.NotEqual(t, acctID, c.ID)
				}
			},
		},
		{
			name: "ou_created_appears_in_list_children",
			run: func(t *testing.T) {
				t.Helper()

				b, rootID := newBackend(t)

				ou, err := b.CreateOrganizationalUnit(rootID, "Infra", nil)
				require.NoError(t, err)

				children, err := b.ListChildren(rootID, "ORGANIZATIONAL_UNIT")
				require.NoError(t, err)

				ids := make(map[string]bool, len(children))
				for _, c := range children {
					ids[c.ID] = true
				}

				require.True(t, ids[ou.ID])
			},
		},
		{
			name: "ou_deleted_clears_list_children",
			run: func(t *testing.T) {
				t.Helper()

				b, rootID := newBackend(t)

				ou, err := b.CreateOrganizationalUnit(rootID, "ToDelete", nil)
				require.NoError(t, err)

				require.NoError(t, b.DeleteOrganizationalUnit(ou.ID))

				children, err := b.ListChildren(rootID, "ORGANIZATIONAL_UNIT")
				require.NoError(t, err)

				for _, c := range children {
					require.NotEqual(t, ou.ID, c.ID)
				}
			},
		},
		{
			name: "delete_ou_with_accounts_rejected",
			run: func(t *testing.T) {
				t.Helper()

				b, rootID := newBackend(t)

				ou, err := b.CreateOrganizationalUnit(rootID, "HasAccounts", nil)
				require.NoError(t, err)

				status, err := b.CreateAccount("dave", "dave@example.com", "OrganizationAccountAccessRole", "", nil)
				require.NoError(t, err)

				require.NoError(t, b.MoveAccount(status.AccountID, rootID, ou.ID))

				// Deletion must be rejected — OU still has an account child.
				err = b.DeleteOrganizationalUnit(ou.ID)
				require.Error(t, err)
			},
		},
		{
			name: "delete_ou_with_child_ous_rejected",
			run: func(t *testing.T) {
				t.Helper()

				b, rootID := newBackend(t)

				parent, err := b.CreateOrganizationalUnit(rootID, "Parent", nil)
				require.NoError(t, err)

				_, err = b.CreateOrganizationalUnit(parent.ID, "Child", nil)
				require.NoError(t, err)

				err = b.DeleteOrganizationalUnit(parent.ID)
				require.Error(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.run(t)
		})
	}
}

// BenchmarkListChildrenAccounts measures ListChildren for ACCOUNT type under the
// indexed path, showing O(k) retrieval independent of total account count.
func BenchmarkListChildrenAccounts(b *testing.B) {
	backend := organizations.NewInMemoryBackend("123456789012", "us-east-1")
	_, root, err := backend.CreateOrganization("ALL")
	if err != nil {
		b.Fatal(err)
	}

	ou, err := backend.CreateOrganizationalUnit(root.ID, "BenchOU", nil)
	if err != nil {
		b.Fatal(err)
	}

	const n = 200
	for i := range n {
		name := "bench" + string(rune('a'+i/26%26)) + string(rune('a'+i%26))
		email := name + "@example.com"

		acct, createErr := backend.CreateAccount(name, email, "Role", "", nil)
		if createErr != nil {
			b.Fatal(createErr)
		}

		if moveErr := backend.MoveAccount(acct.AccountID, root.ID, ou.ID); moveErr != nil {
			b.Fatal(moveErr)
		}
	}

	b.ResetTimer()

	for range b.N {
		children, listErr := backend.ListChildren(ou.ID, "ACCOUNT")
		if listErr != nil {
			b.Fatal(listErr)
		}

		if len(children) != n {
			b.Fatalf("got %d children, want %d", len(children), n)
		}
	}
}

// TestOU_Hierarchy tests deep OU nesting and ListOrganizationalUnitsForParent.
func TestOU_Hierarchy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		depth      int
		wantAtRoot int
	}{
		{name: "single_level", depth: 1, wantAtRoot: 3},
		{name: "two_levels", depth: 2, wantAtRoot: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, rootID := newOrgBackend(t)

			// Create OUs at root level.
			for i := range tt.wantAtRoot {
				_, err := b.CreateOrganizationalUnit(rootID, "root-ou-"+string(rune('a'+i)), nil)
				require.NoError(t, err)
			}

			rootOUs, err := b.ListOrganizationalUnitsForParent(rootID)
			require.NoError(t, err)
			assert.Len(t, rootOUs, tt.wantAtRoot)

			if tt.depth > 1 {
				// Create a child OU under the first root OU.
				_, err = b.CreateOrganizationalUnit(rootOUs[0].ID, "child-ou", nil)
				require.NoError(t, err)

				childOUs, childErr := b.ListOrganizationalUnitsForParent(rootOUs[0].ID)
				require.NoError(t, childErr)
				assert.Len(t, childOUs, 1)
			}
		})
	}
}

// TestOU_DeleteConstraints tests OU deletion is blocked with contents.
func TestOU_DeleteConstraints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		kind    string // "with_account", "with_child_ou", "empty"
		wantErr bool
	}{
		{name: "delete_empty_ou", kind: "empty"},
		{name: "reject_ou_with_account", kind: "with_account", wantErr: true},
		{name: "reject_ou_with_child_ou", kind: "with_child_ou", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, rootID := newOrgBackend(t)

			ou, err := b.CreateOrganizationalUnit(rootID, "target-ou", nil)
			require.NoError(t, err)

			switch tt.kind {
			case "with_account":
				acctStatus, acctErr := b.CreateAccount("child-account", "child@example.com", "", "", nil)
				require.NoError(t, acctErr)
				require.NoError(t, b.MoveAccount(acctStatus.AccountID, rootID, ou.ID))
			case "with_child_ou":
				_, childOUErr := b.CreateOrganizationalUnit(ou.ID, "child-ou", nil)
				require.NoError(t, childOUErr)
			}

			err = b.DeleteOrganizationalUnit(ou.ID)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)

			_, err = b.DescribeOrganizationalUnit(ou.ID)
			require.Error(t, err)
		})
	}
}

// TestOU_ListChildren tests ListChildren for both child types.
func TestOU_ListChildren(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		childType  string
		wantCount  int
		addAccount bool
		addOUs     int
	}{
		{
			name:       "accounts_under_root",
			childType:  "ACCOUNT",
			addAccount: true,
			wantCount:  2, // management + created
		},
		{
			name:      "ous_under_root",
			childType: "ORGANIZATIONAL_UNIT",
			addOUs:    3,
			wantCount: 3,
		},
		{
			name:      "empty_parent_no_children",
			childType: "ACCOUNT",
			wantCount: 1, // just management account
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, rootID := newOrgBackend(t)

			if tt.addAccount {
				_, err := b.CreateAccount("child", "child@example.com", "", "", nil)
				require.NoError(t, err)
			}

			for i := range tt.addOUs {
				_, err := b.CreateOrganizationalUnit(rootID, "ou-"+string(rune('a'+i)), nil)
				require.NoError(t, err)
			}

			children, err := b.ListChildren(rootID, tt.childType)
			require.NoError(t, err)
			assert.Len(t, children, tt.wantCount)

			for _, c := range children {
				assert.Equal(t, tt.childType, c.Type)
				assert.NotEmpty(t, c.ID)
			}
		})
	}
}

// TestOU_ListParents tests ListParents for both accounts and OUs.
func TestOU_ListParents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		childKind string // "account_in_root", "account_in_ou", "ou_in_root", "ou_in_ou"
		wantType  string
	}{
		{name: "account_in_root", childKind: "account_in_root", wantType: "ROOT"},
		{name: "account_in_ou", childKind: "account_in_ou", wantType: "ORGANIZATIONAL_UNIT"},
		{name: "ou_in_root", childKind: "ou_in_root", wantType: "ROOT"},
		{name: "ou_in_ou", childKind: "ou_in_ou", wantType: "ORGANIZATIONAL_UNIT"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, rootID := newOrgBackend(t)

			ou, err := b.CreateOrganizationalUnit(rootID, "parent-ou", nil)
			require.NoError(t, err)

			var childID string

			switch tt.childKind {
			case "account_in_root":
				s1, err1 := b.CreateAccount("a", "a@example.com", "", "", nil)
				require.NoError(t, err1)
				childID = s1.AccountID
			case "account_in_ou":
				s2, err2 := b.CreateAccount("a", "a@example.com", "", "", nil)
				require.NoError(t, err2)
				require.NoError(t, b.MoveAccount(s2.AccountID, rootID, ou.ID))
				childID = s2.AccountID
			case "ou_in_root":
				childOU1, childOU1Err := b.CreateOrganizationalUnit(rootID, "child-ou", nil)
				require.NoError(t, childOU1Err)
				childID = childOU1.ID
			case "ou_in_ou":
				childOU2, childOU2Err := b.CreateOrganizationalUnit(ou.ID, "nested-ou", nil)
				require.NoError(t, childOU2Err)
				childID = childOU2.ID
			}

			parents, err := b.ListParents(childID)
			require.NoError(t, err)
			require.Len(t, parents, 1)
			assert.Equal(t, tt.wantType, parents[0].Type)
		})
	}
}

// ---------------------------------------------------------------------------
// Policy attachment targets
// ---------------------------------------------------------------------------

// TestOU_DepthLimit verifies that OUs can only be nested 5 levels deep.
func TestOU_DepthLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		depth   int
		wantErr bool
	}{
		{name: "depth_1_ok", depth: 1, wantErr: false},
		{name: "depth_2_ok", depth: 2, wantErr: false},
		{name: "depth_3_ok", depth: 3, wantErr: false},
		{name: "depth_4_ok", depth: 4, wantErr: false},
		{name: "depth_5_ok", depth: 5, wantErr: false},
		{name: "depth_6_rejected", depth: 6, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, rootID := newOrgBackend(t)

			// Build a chain of OUs up to depth tt.depth.
			parentID := rootID
			var err error

			for d := 1; d <= tt.depth; d++ {
				var ou *organizations.OrganizationalUnit
				ou, err = b.CreateOrganizationalUnit(parentID, fmt.Sprintf("ou-depth-%d", d), nil)
				if err != nil {
					break
				}
				parentID = ou.ID
			}

			if tt.wantErr {
				require.Error(t, err, "depth %d should be rejected", tt.depth)
			} else {
				require.NoError(t, err, "depth %d should be allowed", tt.depth)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Item 7: OU name uniqueness
// ---------------------------------------------------------------------------

// TestOU_NameUniqueness verifies that sibling OUs cannot have the same name.
func TestOU_NameUniqueness(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup   func(b *organizations.InMemoryBackend, rootID string)
		create  func(b *organizations.InMemoryBackend, rootID string) error
		name    string
		wantErr bool
	}{
		{
			name: "duplicate_sibling_rejected",
			setup: func(b *organizations.InMemoryBackend, rootID string) {
				_, err := b.CreateOrganizationalUnit(rootID, "Engineering", nil)
				if err != nil {
					panic(err)
				}
			},
			create: func(b *organizations.InMemoryBackend, rootID string) error {
				_, err := b.CreateOrganizationalUnit(rootID, "Engineering", nil)

				return err
			},
			wantErr: true,
		},
		{
			name:  "different_name_same_parent_ok",
			setup: func(_ *organizations.InMemoryBackend, _ string) {},
			create: func(b *organizations.InMemoryBackend, rootID string) error {
				_, err := b.CreateOrganizationalUnit(rootID, "Finance", nil)
				if err != nil {
					return err
				}
				_, err = b.CreateOrganizationalUnit(rootID, "Engineering", nil)

				return err
			},
			wantErr: false,
		},
		{
			name: "same_name_different_parent_ok",
			setup: func(b *organizations.InMemoryBackend, rootID string) {
				ou, err := b.CreateOrganizationalUnit(rootID, "ParentA", nil)
				if err != nil {
					panic(err)
				}
				_, err = b.CreateOrganizationalUnit(ou.ID, "Shared", nil)
				if err != nil {
					panic(err)
				}
			},
			create: func(b *organizations.InMemoryBackend, rootID string) error {
				ou, err := b.CreateOrganizationalUnit(rootID, "ParentB", nil)
				if err != nil {
					return err
				}
				_, err = b.CreateOrganizationalUnit(ou.ID, "Shared", nil)

				return err
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, rootID := newOrgBackend(t)
			tt.setup(b, rootID)

			err := tt.create(b, rootID)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestUpdateOU_NameUniqueness verifies that UpdateOrganizationalUnit also enforces uniqueness.
func TestUpdateOU_NameUniqueness(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		rename  string
		wantErr bool
	}{
		{
			name:    "rename_to_sibling_name_rejected",
			rename:  "Finance",
			wantErr: true,
		},
		{
			name:    "rename_to_unique_name_ok",
			rename:  "UniqueNewName",
			wantErr: false,
		},
		{
			name:    "rename_to_same_name_ok",
			rename:  "Engineering",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, rootID := newOrgBackend(t)

			// Create two sibling OUs.
			ouA, err := b.CreateOrganizationalUnit(rootID, "Finance", nil)
			require.NoError(t, err)

			ouB, err := b.CreateOrganizationalUnit(rootID, "Engineering", nil)
			require.NoError(t, err)
			_ = ouA

			// Try to rename Engineering.
			_, err = b.UpdateOrganizationalUnit(ouB.ID, tt.rename)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Item 8: Account email uniqueness
// ---------------------------------------------------------------------------

// TestBackend_ListParents_OU tests ListParents for an OU.
func TestBackend_ListParents_OU(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "ou_parent_is_root"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, rootID := newOrgBackend(t)

			ou, err := b.CreateOrganizationalUnit(rootID, "test-ou", nil)
			require.NoError(t, err)

			parents, err := b.ListParents(ou.ID)
			require.NoError(t, err)
			require.Len(t, parents, 1)
			assert.Equal(t, rootID, parents[0].ID)
		})
	}
}

// TestBackend_ListParents_Error tests ListParents with invalid child.
func TestBackend_ListParents_Error(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		wantErr bool
	}{
		{
			name:    "child_not_found",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, _ := newOrgBackend(t)

			_, err := b.ListParents("ou-doesnotexist")

			if tt.wantErr {
				require.Error(t, err)
			}
		})
	}
}

// TestListOrganizationalUnitsForParent_Sorted verifies sorted output by OU name.
func TestListOrganizationalUnitsForParent_Sorted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		ouNames []string
	}{
		{
			name:    "three_ous_sorted",
			ouNames: []string{"ZOU", "AOU", "MOU"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			createOrgOn(t, b)

			roots, err := b.ListRoots()
			require.NoError(t, err)
			rootID := roots[0].ID

			for _, ouName := range tt.ouNames {
				_, ouErr := b.CreateOrganizationalUnit(rootID, ouName, nil)
				require.NoError(t, ouErr)
			}

			ous, err := b.ListOrganizationalUnitsForParent(rootID)
			require.NoError(t, err)
			require.Len(t, ous, len(tt.ouNames))

			for i := 1; i < len(ous); i++ {
				assert.LessOrEqual(t, ous[i-1].Name, ous[i].Name)
			}
		})
	}
}

// TestListChildren_Sorted verifies sorted output by ID.
func TestListChildren_Sorted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		childCount int
	}{
		{name: "three_accounts_sorted", childCount: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			createOrgOn(t, b)

			roots, err := b.ListRoots()
			require.NoError(t, err)
			rootID := roots[0].ID

			for i := range tt.childCount {
				_, childErr := b.CreateAccount("child", fmt.Sprintf("c%d@example.com", i), "", "", nil)
				require.NoError(t, childErr)
			}

			children, err := b.ListChildren(rootID, "ACCOUNT")
			require.NoError(t, err)

			for i := 1; i < len(children); i++ {
				assert.LessOrEqual(t, children[i-1].ID, children[i].ID, "children should be sorted by ID")
			}
		})
	}
}

// TestListAccountsForParent_Sorted verifies sorted output by account ID.
func TestListAccountsForParent_Sorted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		acctCount  int
		wantSorted bool
	}{
		{name: "sorted", acctCount: 3, wantSorted: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			createOrgOn(t, b)

			roots, err := b.ListRoots()
			require.NoError(t, err)
			rootID := roots[0].ID

			for i := range tt.acctCount {
				_, acctErr := b.CreateAccount("acct", fmt.Sprintf("a%d@example.com", i), "", "", nil)
				require.NoError(t, acctErr)
			}

			accts, err := b.ListAccountsForParent(rootID)
			require.NoError(t, err)

			for i := 1; i < len(accts); i++ {
				assert.LessOrEqual(t, accts[i-1].ID, accts[i].ID)
			}
		})
	}
}

// TestDeleteOrganizationalUnit_WithAccounts_Fails verifies AWS behaviour: cannot delete OU with accounts.
func TestDeleteOrganizationalUnit_WithAccounts_Fails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		wantErr bool
	}{
		{name: "fails_when_ou_has_accounts", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			createOrgOn(t, b)

			roots, err := b.ListRoots()
			require.NoError(t, err)

			ou, err := b.CreateOrganizationalUnit(roots[0].ID, "test-ou", nil)
			require.NoError(t, err)

			// Move management account into the OU.
			org, err := b.DescribeOrganization()
			require.NoError(t, err)
			err = b.MoveAccount(org.MasterAccountID, roots[0].ID, ou.ID)
			require.NoError(t, err)

			err = b.DeleteOrganizationalUnit(ou.ID)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestDeleteOrganizationalUnit_WithChildren_Fails verifies AWS behaviour: cannot delete OU with child OUs.
func TestDeleteOrganizationalUnit_WithChildren_Fails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		wantErr bool
	}{
		{name: "fails_when_ou_has_child_ou", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			createOrgOn(t, b)

			roots, err := b.ListRoots()
			require.NoError(t, err)

			parent, err := b.CreateOrganizationalUnit(roots[0].ID, "parent-ou", nil)
			require.NoError(t, err)

			_, err = b.CreateOrganizationalUnit(parent.ID, "child-ou", nil)
			require.NoError(t, err)

			err = b.DeleteOrganizationalUnit(parent.ID)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestDeleteOrganizationalUnit_NotEmpty_ReturnsOwnErrorCode verifies AWS
// behaviour: DeleteOrganizationalUnit's own per-op error switch
// (deserializers.go) models OrganizationalUnitNotEmptyException, not
// InvalidInputException, for a non-empty OU.
func TestDeleteOrganizationalUnit_NotEmpty_ReturnsOwnErrorCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup func(t *testing.T, b *organizations.InMemoryBackend, rootID string) string
		name  string
	}{
		{
			name: "ou_has_account",
			setup: func(t *testing.T, b *organizations.InMemoryBackend, rootID string) string {
				t.Helper()

				ou, err := b.CreateOrganizationalUnit(rootID, "with-account", nil)
				require.NoError(t, err)

				org, err := b.DescribeOrganization()
				require.NoError(t, err)
				require.NoError(t, b.MoveAccount(org.MasterAccountID, rootID, ou.ID))

				return ou.ID
			},
		},
		{
			name: "ou_has_child_ou",
			setup: func(t *testing.T, b *organizations.InMemoryBackend, rootID string) string {
				t.Helper()

				parent, err := b.CreateOrganizationalUnit(rootID, "with-child", nil)
				require.NoError(t, err)
				_, err = b.CreateOrganizationalUnit(parent.ID, "child", nil)
				require.NoError(t, err)

				return parent.ID
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			createOrgOn(t, b)

			roots, err := b.ListRoots()
			require.NoError(t, err)

			ouID := tt.setup(t, b, roots[0].ID)

			err = b.DeleteOrganizationalUnit(ouID)
			require.ErrorIs(t, err, organizations.ErrOrganizationalUnitNotEmpty)
			require.NotErrorIs(t, err, organizations.ErrInvalidInput)
		})
	}
}

// TestBackend_AddOUInternal verifies OU seed helper.
func TestBackend_AddOUInternal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		ouID      string
		wantCount int
	}{
		{name: "adds_ou", ouID: "ou-abcd-12345678", wantCount: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			createOrgOn(t, b)

			b.AddOUInternal(&organizations.OrganizationalUnit{
				ID:   tt.ouID,
				Name: "seed-ou",
			})

			assert.Equal(t, tt.wantCount, organizations.OUCount(b))
		})
	}
}

// TestDeleteOrganizationalUnit_CleansPolicyTargets verifies that deleting an
// OU which still has a policy attached removes the OU from that policy's
// target list too -- otherwise ListTargetsForPolicy keeps reporting the
// deleted OU as a live, attached target.
func TestDeleteOrganizationalUnit_CleansPolicyTargets(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	createOrgOn(t, b)

	roots, err := b.ListRoots()
	require.NoError(t, err)

	ou, err := b.CreateOrganizationalUnit(roots[0].ID, "leaf-ou", nil)
	require.NoError(t, err)

	p, err := b.CreatePolicy("scp", "", `{"Version":"2012-10-17"}`, "SERVICE_CONTROL_POLICY", nil)
	require.NoError(t, err)

	require.NoError(t, b.AttachPolicy(p.PolicySummary.ID, ou.ID))

	require.NoError(t, b.DeleteOrganizationalUnit(ou.ID))

	targets, err := b.ListTargetsForPolicy(p.PolicySummary.ID)
	require.NoError(t, err)
	assert.Empty(t, targets, "deleted OU must not linger as an attached policy target")
}

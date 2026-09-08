package iam_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/iam"
)

func TestIAM_BackendReset(t *testing.T) {
	t.Parallel()

	_, be := newTestHandler(t)

	// Create some resources
	_, err := be.CreateUser("reset-user", "/", "")
	require.NoError(t, err)

	// Reset exercises collectAndDeleteFunctions and cleanup paths
	be.Reset()

	// Verify reset worked
	usersPage, err := be.ListUsers("", 100)
	require.NoError(t, err)
	assert.Empty(t, usersPage.Data)
}

// TestBackendResetAndPurge covers Reset and Purge as Go methods (not IAM actions).
func TestBackendResetAndPurge(t *testing.T) {
	t.Parallel()

	h, b := newTestHandler(t)

	_, err := b.CreateUser("reset-test-user", "/", "")
	require.NoError(t, err)

	// Reset clears backend state.
	h.Reset()

	// Purge just calls through without panicking.
	h.Purge(context.Background(), timeNow())
}

// TestBackendReset_ClearsPasswordAndFederationState covers gopherstack-54of:
// Reset() must clear passwordPolicy, currentPassword, currentPasswordHistory
// and outboundFederationEnabled, all mutated by real account.go operations,
// none of which were named in Reset()'s field list.
func TestBackendReset_ClearsPasswordAndFederationState(t *testing.T) {
	t.Parallel()

	t.Run("password_policy", func(t *testing.T) {
		t.Parallel()

		b := newBackend(t)
		require.NoError(t, b.UpdateAccountPasswordPolicy(iam.PasswordPolicy{
			MinimumPasswordLength: 20,
			RequireSymbols:        true,
		}))

		b.Reset()

		got := b.GetAccountPasswordPolicy()
		assert.Equal(t, 8, got.MinimumPasswordLength,
			"passwordPolicy must be cleared to nil so GetAccountPasswordPolicy falls back to the default")
		assert.False(t, got.RequireSymbols, "custom RequireSymbols must not survive Reset")
	})

	t.Run("current_password", func(t *testing.T) {
		t.Parallel()

		b := newBackend(t)
		require.NoError(t, b.ChangePassword("InitialOld1!", "SetBeforeReset1!"))

		b.Reset()

		// currentPassword started "" and was only ever set by the ChangePassword
		// above; if Reset failed to clear it, "SetBeforeReset1!" would still be
		// the required OldPassword and this mismatched call would fail.
		err := b.ChangePassword("NotTheOldPassword1!", "AfterReset1!")
		require.NoError(t, err,
			"currentPassword must be cleared to \"\" so any OldPassword is accepted")
	})

	t.Run("current_password_history", func(t *testing.T) {
		t.Parallel()

		b := newBackend(t)
		require.NoError(t, b.UpdateAccountPasswordPolicy(iam.PasswordPolicy{
			MinimumPasswordLength:   8,
			PasswordReusePrevention: 2,
		}))
		require.NoError(t, b.ChangePassword("InitialOld1!", "Password1!"))
		require.NoError(t, b.ChangePassword("Password1!", "Password2!"))

		b.Reset()

		// Re-apply reuse prevention fresh after Reset (independent of whether
		// passwordPolicy itself was cleared) and reuse the OldPassword value
		// that satisfies the check whether or not currentPassword was cleared
		// (it matches both the cleared "" bypass and a leftover stale value),
		// isolating this assertion to currentPasswordHistory alone.
		require.NoError(t, b.UpdateAccountPasswordPolicy(iam.PasswordPolicy{
			MinimumPasswordLength:   8,
			PasswordReusePrevention: 2,
		}))

		err := b.ChangePassword("Password2!", "Password1!")
		require.NoError(t, err,
			"currentPasswordHistory must be cleared so a pre-Reset password is reusable")
	})

	t.Run("outbound_federation_enabled", func(t *testing.T) {
		t.Parallel()

		b := newBackend(t)
		b.DisableOutboundWebIdentityFederation()

		b.Reset()

		assert.True(t, b.OutboundWebIdentityFederationEnabled(),
			"outboundFederationEnabled must be reset to its constructed default (true)")
	})
}

// newBackend returns a fresh InMemoryBackend for testing.
func newBackend(t *testing.T) *iam.InMemoryBackend {
	t.Helper()

	return iam.NewInMemoryBackend()
}

func TestNormPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		wantPath string
	}{
		{
			name:     "empty_path_defaults_to_root",
			input:    "",
			wantPath: "/",
		},
		{
			name:     "path_without_trailing_slash_gets_one",
			input:    "/engineering",
			wantPath: "/engineering/",
		},
		{
			name:     "path_with_trailing_slash_unchanged",
			input:    "/engineering/",
			wantPath: "/engineering/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := iam.NewInMemoryBackend()
			u, err := b.CreateUser("normpath-user-"+tt.name, tt.input, "")
			require.NoError(t, err)
			assert.Equal(t, tt.wantPath, u.Path)
		})
	}
}

// TestListPagination_SortedOrder verifies that List operations return
// results in lexicographic (sorted) order and correctly paginate using the
// returned marker token.
// assertPaginationSorted creates tt.itemNames in the backend via tt.createFn
// (intentionally unsorted creation order), then paginates through tt.listFn
// using tt.pageSize and verifies every page respects the page size, that all
// items are returned exactly once, and that the combined results are in
// lexicographic order.
func assertPaginationSorted(t *testing.T, tt paginationTestCase) {
	t.Helper()

	b := iam.NewInMemoryBackend()

	// Create resources in the order given (intentionally unsorted).
	for _, name := range tt.itemNames {
		require.NoError(t, tt.createFn(b, name))
	}

	// Collect all items by paginating.
	var allNames []string
	marker := ""

	for {
		names, next, err := tt.listFn(b, marker, tt.pageSize)
		require.NoError(t, err)

		if len(names) == 0 && next == "" {
			break
		}

		// Each page must have at most pageSize items.
		assert.LessOrEqual(t, len(names), tt.pageSize,
			"page must not exceed pageSize=%d", tt.pageSize)

		allNames = append(allNames, names...)

		if next == "" {
			break
		}

		marker = next
	}

	assert.Len(t, allNames, len(tt.itemNames),
		"paginated result must contain all %d items", len(tt.itemNames))

	// Verify lexicographic sort order across all pages.
	for i := 1; i < len(allNames); i++ {
		assert.Less(t, allNames[i-1], allNames[i],
			"items must be in sorted order: allNames[%d]=%q must be < allNames[%d]=%q",
			i-1, allNames[i-1], i, allNames[i])
	}
}

// listFunc lists a page of resource names starting at marker, capped at pageLimit.
type listFunc func(b *iam.InMemoryBackend, marker string, pageLimit int) (names []string, next string, err error)

// createFunc creates a single named resource in the backend.
type createFunc func(b *iam.InMemoryBackend, name string) error

type paginationTestCase struct {
	listFn    listFunc
	createFn  createFunc
	name      string
	itemNames []string // names to create, must include more items than pageSize
	pageSize  int
}

func TestListPagination_SortedOrder(t *testing.T) {
	t.Parallel()

	const validTrustPolicy = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow",` +
		`"Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	const validPolicy = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow",` +
		`"Action":"s3:GetObject","Resource":"*"}]}`

	tests := []paginationTestCase{
		{
			name:      "list_users_sorted_paginated",
			pageSize:  3,
			itemNames: []string{"zara", "alice", "mike", "bob", "carol", "dave"},
			createFn: func(b *iam.InMemoryBackend, name string) error {
				_, err := b.CreateUser(name, "/", "")

				return err
			},
			listFn: func(b *iam.InMemoryBackend, marker string, pageLimit int) ([]string, string, error) {
				pg, err := b.ListUsers(marker, pageLimit)
				if err != nil {
					return nil, "", err
				}

				names := make([]string, len(pg.Data))
				for i, u := range pg.Data {
					names[i] = u.UserName
				}

				return names, pg.Next, nil
			},
		},
		{
			name:      "list_roles_sorted_paginated",
			pageSize:  2,
			itemNames: []string{"zeta-role", "alpha-role", "beta-role", "gamma-role", "delta-role"},
			createFn: func(b *iam.InMemoryBackend, name string) error {
				_, err := b.CreateRole(name, "/", validTrustPolicy, "")

				return err
			},
			listFn: func(b *iam.InMemoryBackend, marker string, pageLimit int) ([]string, string, error) {
				pg, err := b.ListRoles(marker, pageLimit)
				if err != nil {
					return nil, "", err
				}

				names := make([]string, len(pg.Data))
				for i, r := range pg.Data {
					names[i] = r.RoleName
				}

				return names, pg.Next, nil
			},
		},
		{
			name:      "list_groups_sorted_paginated",
			pageSize:  2,
			itemNames: []string{"ops", "dev", "qa", "sre", "mgmt"},
			createFn: func(b *iam.InMemoryBackend, name string) error {
				_, err := b.CreateGroup(name, "/")

				return err
			},
			listFn: func(b *iam.InMemoryBackend, marker string, pageLimit int) ([]string, string, error) {
				pg, err := b.ListGroups(marker, pageLimit)
				if err != nil {
					return nil, "", err
				}

				names := make([]string, len(pg.Data))
				for i, g := range pg.Data {
					names[i] = g.GroupName
				}

				return names, pg.Next, nil
			},
		},
		{
			name:      "list_policies_sorted_paginated",
			pageSize:  3,
			itemNames: []string{"ZPolicy", "APolicy", "MPolicy", "BPolicy", "CPolicy", "DPolicy"},
			createFn: func(b *iam.InMemoryBackend, name string) error {
				_, err := b.CreatePolicy(name, "/", validPolicy)

				return err
			},
			listFn: func(b *iam.InMemoryBackend, marker string, pageLimit int) ([]string, string, error) {
				pg, err := b.ListPolicies(marker, pageLimit)
				if err != nil {
					return nil, "", err
				}

				names := make([]string, len(pg.Data))
				for i, p := range pg.Data {
					names[i] = p.PolicyName
				}

				return names, pg.Next, nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertPaginationSorted(t, tt)
		})
	}
}

// TestSortedIndex_MaintainedAfterDelete verifies that sorted name indexes
// are correctly updated when resources are deleted, and that subsequent List
// operations return the correct remaining items in sorted order.
func TestSortedIndex_MaintainedAfterDelete(t *testing.T) {
	t.Parallel()

	type deleteTestCase struct {
		create     []string
		toDelete   []string
		name       string
		wantRemain []string
	}

	tests := []deleteTestCase{
		{
			name:       "user_delete_updates_index",
			create:     []string{"zara", "alice", "bob", "carol"},
			toDelete:   []string{"alice", "carol"},
			wantRemain: []string{"bob", "zara"},
		},
		{
			name:       "group_delete_updates_index",
			create:     []string{"ops", "dev", "qa"},
			toDelete:   []string{"dev"},
			wantRemain: []string{"ops", "qa"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := iam.NewInMemoryBackend()
			isUser := strings.Contains(tt.name, "user")

			for _, name := range tt.create {
				if isUser {
					_, err := b.CreateUser(name, "/", "")
					require.NoError(t, err)
				} else {
					_, err := b.CreateGroup(name, "/")
					require.NoError(t, err)
				}
			}

			for _, name := range tt.toDelete {
				if isUser {
					require.NoError(t, b.DeleteUser(name))
				} else {
					require.NoError(t, b.DeleteGroup(name))
				}
			}

			// List all remaining items.
			var gotNames []string

			if isUser {
				pg, err := b.ListUsers("", 100)
				require.NoError(t, err)

				for _, u := range pg.Data {
					gotNames = append(gotNames, u.UserName)
				}
			} else {
				pg, err := b.ListGroups("", 100)
				require.NoError(t, err)

				for _, g := range pg.Data {
					gotNames = append(gotNames, g.GroupName)
				}
			}

			assert.Equal(t, tt.wantRemain, gotNames,
				"remaining items after delete must match expected sorted list")
		})
	}
}

package workmail_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/workmail"
)

const (
	persistTestAccountID = "000000000000"
	persistTestRegion    = "us-east-1"
)

// newFullyPopulatedBackend creates a backend with one populated entry in
// every store.Table (see store_setup.go's registerAllTables) plus every raw
// map that persistence.go persists directly (orgsByAlias, usersByEmail,
// groupsByEmail, resourcesByEmail, groupMembers, delegates, aliases,
// mailboxQuotas, tags, inboundDmarc, identityCenterApps), so a Snapshot from
// it exercises the entire persisted surface of the backend. WorkMail had no
// persistence at all before this conversion, so -- unlike most Phase 3.3
// services -- nothing here is left deliberately unpersisted/ephemeral.
// Returns the backend and the one value (an Identity Center application ARN)
// that has no read accessor of its own, so tests can still verify it.
func newFullyPopulatedBackend(t *testing.T) (*workmail.InMemoryBackend, string) {
	t.Helper()

	b := workmail.NewInMemoryBackend(persistTestAccountID, persistTestRegion)
	ctx := t.Context()

	org, err := b.CreateOrganization(ctx, "acme", []string{"acme.com"}, false)
	require.NoError(t, err)
	orgID := org.OrgID

	user, err := b.CreateUser(orgID, "alice", workmail.CreateUserParams{
		DisplayName: "Alice A", Password: "pw", Role: "USER",
	})
	require.NoError(t, err)
	require.NoError(t, b.RegisterToWorkMail(orgID, user.UserID, "alice@acme.com"))
	require.NoError(t, b.UpdateMailboxQuota(orgID, user.UserID, 12345))
	require.NoError(t, b.CreateAlias(orgID, user.UserID, "a.alice@acme.com"))

	group, err := b.CreateGroup(orgID, "engineering", false)
	require.NoError(t, err)
	require.NoError(t, b.AssociateMemberToGroup(orgID, group.GroupID, user.UserID))

	resource, err := b.CreateResource(orgID, "conf-room", "ROOM", "desc", nil, false)
	require.NoError(t, err)
	require.NoError(t, b.AssociateDelegateToResource(orgID, resource.ResourceID, user.UserID))

	require.NoError(t, b.PutMailboxPermissions(orgID, user.UserID, group.GroupID, []string{"FULL_ACCESS"}))
	require.NoError(t, b.RegisterMailDomain(orgID, "extra.acme.com"))

	_, err = b.PutAccessControlRule(orgID, workmail.PutAccessControlRuleParams{
		Name: "allow-all", Effect: "ALLOW", Description: "desc",
	})
	require.NoError(t, err)

	role, err := b.CreateImpersonationRole(orgID, "impersonator", "FULL_ACCESS", "desc", nil)
	require.NoError(t, err)
	_, _, err = b.AssumeImpersonationRole(orgID, role.RoleID)
	require.NoError(t, err)

	require.NoError(t, b.TagResource(user.ARN, []workmail.Tag{{Key: "env", Value: "test"}}))

	_, err = b.CreateAvailabilityConfiguration(
		orgID, "avail.acme.com", nil, "arn:aws:lambda:us-east-1:000000000000:function:f",
	)
	require.NoError(t, err)

	_, err = b.CreateMobileDeviceAccessRule(
		orgID, "block-android", "DENY", "desc", nil, nil, []string{"ANDROID"}, nil, nil, nil, nil, nil,
	)
	require.NoError(t, err)
	require.NoError(t, b.PutMobileDeviceAccessOverride(orgID, user.UserID, "device-1", "ALLOW", "override desc"))

	require.NoError(t, b.PutEmailMonitoringConfiguration(
		orgID, "arn:aws:iam::000000000000:role/mon", "arn:aws:logs:us-east-1:000000000000:log-group:mon",
	))
	require.NoError(t, b.PutInboundDmarcSettings(orgID, true))
	require.NoError(t, b.PutRetentionPolicy(orgID, "", "policy1", "desc", nil))

	_, err = b.StartMailboxExportJob(
		orgID, user.UserID, "desc", "arn:aws:iam::000000000000:role/export", "", "bucket", "prefix",
	)
	require.NoError(t, err)

	identityCenterAppARN, err := b.CreateIdentityCenterApplication("arn:aws:sso:::instance/ssoins-1", "app1")
	require.NoError(t, err)

	require.NoError(t, b.PutIdentityProviderConfiguration(orgID, "IDENTITY_CENTER", "", "", "ENABLED", 30))

	_, err = b.CreatePersonalAccessToken(orgID, user.UserID, "pat1", []string{"read"})
	require.NoError(t, err)

	return b, identityCenterAppARN
}

// restoredIDs are the primary keys of the one seeded entity of each kind in
// newFullyPopulatedBackend, re-discovered from a restored backend via its
// public List API rather than threaded through from the original --
// black-box, like the rest of this file.
type restoredIDs struct {
	orgID      string
	userID     string
	groupID    string
	resourceID string
}

func fetchRestoredIDs(t *testing.T, fresh *workmail.InMemoryBackend) restoredIDs {
	t.Helper()

	ctx := t.Context()

	orgs, _, err := fresh.ListOrganizations(ctx, 0, "")
	require.NoError(t, err)
	require.Len(t, orgs, 1)
	orgID := orgs[0].OrgID

	users, _, err := fresh.ListUsers(orgID, nil, 0, "")
	require.NoError(t, err)
	require.Len(t, users, 1)

	groups, _, err := fresh.ListGroups(orgID, nil, 0, "")
	require.NoError(t, err)
	require.Len(t, groups, 1)

	resources, _, err := fresh.ListResources(orgID, nil, 0, "")
	require.NoError(t, err)
	require.Len(t, resources, 1)

	return restoredIDs{
		orgID:      orgID,
		userID:     users[0].UserID,
		groupID:    groups[0].GroupID,
		resourceID: resources[0].ResourceID,
	}
}

// TestInMemoryBackend_SnapshotRestore_FullState round-trips every store.Table
// and every raw map persistence.go persists directly, through Snapshot ->
// Restore into a fresh backend, then verifies each one via the public
// (black-box) read API -- the same pattern used by every other Phase 3.3
// service's persistence_test.go.
func TestInMemoryBackend_SnapshotRestore_FullState(t *testing.T) {
	t.Parallel()

	original, appARN := newFullyPopulatedBackend(t)
	ctx := t.Context()

	snap := original.Snapshot(ctx)
	require.NotNil(t, snap)

	fresh := workmail.NewInMemoryBackend(persistTestAccountID, persistTestRegion)
	require.NoError(t, fresh.Restore(ctx, snap))

	ids := fetchRestoredIDs(t, fresh)

	tests := []struct {
		run  func(t *testing.T)
		name string
	}{
		{name: "organizations table", run: func(t *testing.T) {
			t.Helper()

			org, err := fresh.DescribeOrganization(ids.orgID)
			require.NoError(t, err)
			assert.Equal(t, "acme", org.Alias)
		}},
		{name: "orgsByAlias raw map", run: func(t *testing.T) {
			t.Helper()
			// A duplicate CreateOrganization with the same alias must
			// conflict post-restore -- proof orgsByAlias round-tripped.
			_, err := fresh.CreateOrganization(ctx, "acme", nil, false)
			require.Error(t, err)
			assert.ErrorIs(t, err, workmail.ErrNameUnavailable)
		}},
		{name: "users table and mailboxQuotas raw map", run: func(t *testing.T) {
			t.Helper()

			user, err := fresh.DescribeUser(ids.orgID, ids.userID)
			require.NoError(t, err)
			assert.Equal(t, "alice", user.Name)
			assert.Equal(t, "alice@acme.com", user.Email)

			details, err := fresh.GetMailboxDetails(ids.orgID, ids.userID)
			require.NoError(t, err)
			assert.Equal(t, int32(12345), details.MailboxQuota)
		}},
		{name: "usersByEmail raw map and globalAliases table", run: func(t *testing.T) {
			t.Helper()
			// RegisterToWorkMail for a second, still-DISABLED entity with
			// alice's email must conflict -- proof usersByEmail/
			// globalAliases round-tripped. (Registering alice's own
			// already-ENABLED user with her own email is now a no-op per
			// RegisterToWorkMail's real doc, so it can no longer serve as
			// the round-trip proof here.)
			err := fresh.RegisterToWorkMail(ids.orgID, ids.groupID, "alice@acme.com")
			require.Error(t, err)
			assert.ErrorIs(t, err, workmail.ErrEmailInUse)
		}},
		{name: "aliases raw map", run: func(t *testing.T) {
			t.Helper()

			aliases, _, err := fresh.ListAliases(ids.orgID, ids.userID, 0, "")
			require.NoError(t, err)
			assert.Contains(t, aliases, "a.alice@acme.com")
		}},
		{name: "groups table and groupsByEmail raw map", run: func(t *testing.T) {
			t.Helper()

			group, err := fresh.DescribeGroup(ids.orgID, ids.groupID)
			require.NoError(t, err)
			assert.Equal(t, "engineering", group.Name)
		}},
		{name: "groupMembers raw map", run: func(t *testing.T) {
			t.Helper()

			members, _, err := fresh.ListGroupMembers(ids.orgID, ids.groupID, 0, "")
			require.NoError(t, err)
			require.Len(t, members, 1)
			assert.Equal(t, ids.userID, members[0].MemberID)
		}},
		{name: "resources table and resourcesByEmail raw map", run: func(t *testing.T) {
			t.Helper()

			resource, err := fresh.DescribeResource(ids.orgID, ids.resourceID)
			require.NoError(t, err)
			assert.Equal(t, "conf-room", resource.Name)
		}},
		{name: "delegates raw map", run: func(t *testing.T) {
			t.Helper()

			delegates, _, err := fresh.ListResourceDelegates(ids.orgID, ids.resourceID, 0, "")
			require.NoError(t, err)
			require.Len(t, delegates, 1)
			assert.Equal(t, ids.userID, delegates[0].DelegateID)
		}},
		{name: "permissions table", run: func(t *testing.T) {
			t.Helper()

			perms, _, err := fresh.ListMailboxPermissions(ids.orgID, ids.userID, 0, "")
			require.NoError(t, err)
			require.Len(t, perms, 1)
			assert.Equal(t, ids.groupID, perms[0].GranteeID)
			assert.Equal(t, []string{"FULL_ACCESS"}, perms[0].Permissions)
		}},
		{name: "mailDomains table", run: func(t *testing.T) {
			t.Helper()

			domains, _, err := fresh.ListMailDomains(ids.orgID, 0, "")
			require.NoError(t, err)
			names := make([]string, 0, len(domains))
			for _, d := range domains {
				names = append(names, d.DomainName)
			}
			assert.Contains(t, names, "extra.acme.com")
			assert.Contains(t, names, "acme.awsapps.com") // default domain
		}},
		{name: "accessRules table", run: func(t *testing.T) {
			t.Helper()

			rules, err := fresh.ListAccessControlRules(ids.orgID)
			require.NoError(t, err)
			require.Len(t, rules, 1)
			assert.Equal(t, "allow-all", rules[0].Name)
		}},
		{name: "impersonation table and issuedTokens table", run: func(t *testing.T) {
			t.Helper()

			roles, _, err := fresh.ListImpersonationRoles(ids.orgID, 0, "")
			require.NoError(t, err)
			require.Len(t, roles, 1)
			assert.Equal(t, "impersonator", roles[0].Name)

			// issuedTokens has no read accessor; AssumeImpersonationRole
			// succeeding again post-restore proves the impersonation table
			// (which it depends on) round-tripped, and that the issuedTokens
			// table restore didn't error.
			_, _, err = fresh.AssumeImpersonationRole(ids.orgID, roles[0].RoleID)
			require.NoError(t, err)
		}},
		{name: "tags raw map", run: func(t *testing.T) {
			t.Helper()

			user, err := fresh.DescribeUser(ids.orgID, ids.userID)
			require.NoError(t, err)

			tags, err := fresh.ListTagsForResource(user.ARN)
			require.NoError(t, err)
			require.Len(t, tags, 1)
			assert.Equal(t, workmail.Tag{Key: "env", Value: "test"}, tags[0])
		}},
		{name: "availabilityConfigs table", run: func(t *testing.T) {
			t.Helper()

			cfgs, _, err := fresh.ListAvailabilityConfigurations(ids.orgID, 0, "")
			require.NoError(t, err)
			require.Len(t, cfgs, 1)
			assert.Equal(t, "avail.acme.com", cfgs[0].DomainName)
		}},
		{name: "mobileDeviceRules table", run: func(t *testing.T) {
			t.Helper()

			rules, err := fresh.ListMobileDeviceAccessRules(ids.orgID)
			require.NoError(t, err)
			require.Len(t, rules, 1)
			assert.Equal(t, "block-android", rules[0].Name)
		}},
		{name: "mobileDeviceOverrides table", run: func(t *testing.T) {
			t.Helper()

			ov, err := fresh.GetMobileDeviceAccessOverride(ids.orgID, ids.userID, "device-1")
			require.NoError(t, err)
			assert.Equal(t, "ALLOW", ov.Effect)
		}},
		{name: "emailMonitoring table", run: func(t *testing.T) {
			t.Helper()

			cfg, err := fresh.DescribeEmailMonitoringConfiguration(ids.orgID)
			require.NoError(t, err)
			assert.Equal(t, "arn:aws:iam::000000000000:role/mon", cfg.RoleARN)
		}},
		{name: "inboundDmarc raw map", run: func(t *testing.T) {
			t.Helper()

			enforced, err := fresh.DescribeInboundDmarcSettings(ids.orgID)
			require.NoError(t, err)
			assert.True(t, enforced)
		}},
		{name: "retentionPolicies table", run: func(t *testing.T) {
			t.Helper()

			pol, err := fresh.GetDefaultRetentionPolicy(ids.orgID)
			require.NoError(t, err)
			assert.Equal(t, "policy1", pol.Name)
		}},
		{name: "exportJobs table", run: func(t *testing.T) {
			t.Helper()

			jobs, _, err := fresh.ListMailboxExportJobs(ids.orgID, 0, "")
			require.NoError(t, err)
			require.Len(t, jobs, 1)
			assert.Equal(t, "bucket", jobs[0].S3BucketName)
		}},
		{name: "identityCenterApps raw map", run: func(t *testing.T) {
			t.Helper()
			// No read accessor exists; Delete succeeding (rather than
			// ErrNotFound) proves the raw map round-tripped.
			err := fresh.DeleteIdentityCenterApplication(appARN)
			require.NoError(t, err)
		}},
		{name: "idpConfig table", run: func(t *testing.T) {
			t.Helper()

			cfg, err := fresh.DescribeIdentityProviderConfiguration(ids.orgID)
			require.NoError(t, err)
			assert.Equal(t, "IDENTITY_CENTER", cfg.AuthMode)
			require.NotNil(t, cfg.PATLifetimeDays)
			assert.Equal(t, int32(30), *cfg.PATLifetimeDays)
		}},
		{name: "personalTokens table", run: func(t *testing.T) {
			t.Helper()

			toks, _, err := fresh.ListPersonalAccessTokens(ids.orgID, "", 0, "")
			require.NoError(t, err)
			require.Len(t, toks, 1)
			assert.Equal(t, "pat1", toks[0].Name)
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.run(t)
		})
	}

	// Sanity: account/region carried through the constructor, not the
	// snapshot (matching every other Phase 3.3 service's persistence.go).
	assert.Equal(t, persistTestRegion, fresh.Region())
	assert.Equal(t, persistTestAccountID, fresh.AccountID())
}

// TestInMemoryBackend_RestoreVersionMismatch verifies that a snapshot whose
// version doesn't match the current backend is discarded cleanly rather than
// partially decoded: the backend resets to empty state and Restore returns
// no error. WorkMail had no persistence at all before this conversion, so
// there is no legacy (Version == 0) snapshot format to special-case --
// absent-or-mismatched Version is handled identically.
func TestInMemoryBackend_RestoreVersionMismatch(t *testing.T) {
	t.Parallel()

	b, _ := newFullyPopulatedBackend(t)
	ctx := t.Context()

	err := b.Restore(ctx, []byte(`{"version":999,"tables":{}}`))
	require.NoError(t, err)

	assert.Equal(t, 0, workmail.OrgCount(b))

	orgs, _, err := b.ListOrganizations(ctx, 0, "")
	require.NoError(t, err)
	assert.Empty(t, orgs)
}

// TestInMemoryBackend_RestoreInvalidData verifies malformed JSON surfaces as
// an error rather than being silently discarded (that path is reserved for a
// syntactically valid but version-mismatched snapshot; see
// TestInMemoryBackend_RestoreVersionMismatch).
func TestInMemoryBackend_RestoreInvalidData(t *testing.T) {
	t.Parallel()

	b := workmail.NewInMemoryBackend(persistTestAccountID, persistTestRegion)
	err := b.Restore(t.Context(), []byte("not-valid-json"))
	require.Error(t, err)
}

// TestHandler_SnapshotRestoreDelegate verifies Handler.Snapshot/Restore
// delegate to the backend -- the wiring cli.go's generic setupPersistence
// relies on. WorkMail previously had no Handler.Snapshot/Restore (and no
// InMemoryBackend.Snapshot/Restore either), so it was never registered with
// the persistence manager at all; see persistence.go.
func TestHandler_SnapshotRestoreDelegate(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	b, _ := newFullyPopulatedBackend(t)
	h := workmail.NewHandler(b)

	snap := h.Snapshot(ctx)
	require.NotNil(t, snap)

	h2 := workmail.NewHandler(workmail.NewInMemoryBackend(persistTestAccountID, persistTestRegion))
	require.NoError(t, h2.Restore(ctx, snap))

	orgs, _, err := h2.Backend.ListOrganizations(ctx, 0, "")
	require.NoError(t, err)
	require.Len(t, orgs, 1)
	assert.Equal(t, "acme", orgs[0].Alias)
}

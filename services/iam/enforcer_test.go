package iam_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/iam"
)

func TestGetUserByAccessKeyID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setupUser   string
		lookupKeyID string
		wantUser    string
		createKey   bool
		wantErr     bool
	}{
		{
			name:      "found",
			setupUser: "alice",
			createKey: true,
			wantUser:  "alice",
		},
		{
			name:        "not_found",
			setupUser:   "alice",
			createKey:   false,
			lookupKeyID: "AKIA_NONEXISTENT",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newIAMBackend(t)

			_, err := b.CreateUser(tt.setupUser, "/", "")
			require.NoError(t, err)

			lookupID := tt.lookupKeyID
			if tt.createKey {
				ak, err2 := b.CreateAccessKey(tt.setupUser)
				require.NoError(t, err2)
				lookupID = ak.AccessKeyID
			}

			user, err := b.GetUserByAccessKeyID(lookupID)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantUser, user.UserName)
		})
	}
}

func TestGetPoliciesForUser(t *testing.T) {
	t.Parallel()

	const policyDoc = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"*"}]}`

	tests := []struct {
		name       string
		setupUser  string
		policyName string
		policyDoc  string
		queryUser  string
		wantCount  int
		attach     bool
		wantErr    bool
	}{
		{
			name:       "user_with_one_policy",
			setupUser:  "alice",
			policyName: "AllowS3",
			policyDoc:  policyDoc,
			attach:     true,
			queryUser:  "alice",
			wantCount:  1,
		},
		{
			name:       "user_with_no_policies",
			setupUser:  "bob",
			policyName: "AllowS3",
			policyDoc:  policyDoc,
			attach:     false,
			queryUser:  "bob",
			wantCount:  0,
		},
		{
			name:      "user_not_found",
			setupUser: "alice",
			queryUser: "nobody",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newIAMBackend(t)

			_, err := b.CreateUser(tt.setupUser, "/", "")
			require.NoError(t, err)

			var polArn string
			if tt.policyName != "" {
				pol, err2 := b.CreatePolicy(tt.policyName, "/", tt.policyDoc)
				require.NoError(t, err2)
				polArn = pol.Arn
			}

			if tt.attach && polArn != "" {
				err = b.AttachUserPolicy(tt.setupUser, polArn)
				require.NoError(t, err)
			}

			docs, err := b.GetPoliciesForUser(tt.queryUser)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Len(t, docs, tt.wantCount)
		})
	}
}

// TestGetPoliciesForUser_IncludesInlineAndGroupPolicies guards the policy set
// consumed by the real enforcement middleware (middleware.go). It must match
// what collectNamedPrincipalPolicies already aggregates for policy
// simulation: the user's own inline policies plus managed and inline
// policies inherited from group membership.
func TestGetPoliciesForUser_IncludesInlineAndGroupPolicies(t *testing.T) {
	t.Parallel()

	const policyDoc = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"*"}]}`

	tests := []struct {
		setup func(t *testing.T, b *iam.InMemoryBackend)
		name  string
	}{
		{
			name: "own_inline_policy",
			setup: func(t *testing.T, b *iam.InMemoryBackend) {
				t.Helper()
				require.NoError(t, b.PutUserPolicy("alice", "Inline1", policyDoc))
			},
		},
		{
			name: "group_managed_policy",
			setup: func(t *testing.T, b *iam.InMemoryBackend) {
				t.Helper()
				_, err := b.CreateGroup("devs", "/")
				require.NoError(t, err)
				require.NoError(t, b.AddUserToGroup("devs", "alice"))
				pol, err := b.CreatePolicy("GroupManaged", "/", policyDoc)
				require.NoError(t, err)
				require.NoError(t, b.AttachGroupPolicy("devs", pol.Arn))
			},
		},
		{
			name: "group_inline_policy",
			setup: func(t *testing.T, b *iam.InMemoryBackend) {
				t.Helper()
				_, err := b.CreateGroup("ops", "/")
				require.NoError(t, err)
				require.NoError(t, b.AddUserToGroup("ops", "alice"))
				require.NoError(t, b.PutGroupPolicy("ops", "GroupInline", policyDoc))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newIAMBackend(t)
			_, err := b.CreateUser("alice", "/", "")
			require.NoError(t, err)

			tt.setup(t, b)

			docs, err := b.GetPoliciesForUser("alice")
			require.NoError(t, err)
			assert.Len(t, docs, 1)
		})
	}
}

package organizations_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/organizations"
)

// TestBackend_AccountID verifies AccountID() returns configured value.
func TestBackend_AccountID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		accountID string
	}{
		{name: "default_account", accountID: "123456789012"},
		{name: "custom_account", accountID: "999999999999"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := organizations.NewInMemoryBackend(tt.accountID, "us-east-1")
			assert.Equal(t, tt.accountID, b.AccountID())
		})
	}
}

// TestBackend_Region verifies Region() returns configured value.
func TestBackend_Region(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		region string
	}{
		{name: "us_east_1", region: "us-east-1"},
		{name: "eu_west_1", region: "eu-west-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := organizations.NewInMemoryBackend("123456789012", tt.region)
			assert.Equal(t, tt.region, b.Region())
		})
	}
}

// TestBackend_Reset verifies Backend.Reset() clears all state.
func TestBackend_Reset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "reset_clears_accounts_ous_policies"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			createOrgOn(t, b)

			// Create some data.
			_, err := b.CreateAccount("a", "a@x.com", "", "", nil)
			require.NoError(t, err)

			roots, err := b.ListRoots()
			require.NoError(t, err)

			_, err = b.CreateOrganizationalUnit(roots[0].ID, "ou1", nil)
			require.NoError(t, err)

			_, err = b.CreatePolicy("p1", "", `{}`, "SERVICE_CONTROL_POLICY", nil)
			require.NoError(t, err)

			require.Positive(t, organizations.AccountCount(b))
			require.Positive(t, organizations.OUCount(b))

			b.Reset()

			assert.Equal(t, 0, organizations.AccountCount(b), tt.name)
			assert.Equal(t, 0, organizations.OUCount(b), tt.name)
			assert.Equal(t, 0, organizations.PolicyCount(b), tt.name)
		})
	}
}

// TestExportHelpers verifies all export helpers return correct types.
func TestExportHelpers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "helpers_return_correct_counts"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			createOrgOn(t, b)

			_, err := b.CreateAccount("a", "a@x.com", "", "", nil)
			require.NoError(t, err)

			roots, err := b.ListRoots()
			require.NoError(t, err)

			_, err = b.CreateOrganizationalUnit(roots[0].ID, "ou", nil)
			require.NoError(t, err)

			_, err = b.CreatePolicy("pol", "", `{}`, "SERVICE_CONTROL_POLICY", nil)
			require.NoError(t, err)

			err = b.TagResource(roots[0].ID, []organizations.Tag{{Key: "k", Value: "v"}})
			require.NoError(t, err)

			b.AddHandshakeInternal(&organizations.Handshake{State: "OPEN"})

			assert.Equal(t, 2, organizations.AccountCount(b), tt.name+": account count")
			assert.Equal(t, 1, organizations.OUCount(b), tt.name+": ou count")
			// +1 for the default FullAWSAccess SCP every organization is created with.
			assert.Equal(t, 2, organizations.PolicyCount(b), tt.name+": policy count")
			assert.Equal(t, 1, organizations.HandshakeCount(b), tt.name+": handshake count")
			assert.Equal(t, 1, organizations.TagCount(b), tt.name+": tag count")

			h := organizations.NewHandler(b)
			assert.Positive(t, organizations.HandlerOpsLen(h), tt.name+": ops len")
		})
	}
}

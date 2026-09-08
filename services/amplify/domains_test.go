package amplify_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/amplify"
)

func TestInMemoryBackend_DomainAssociation_Lifecycle(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	app, err := b.CreateApp("TestApp", "", "", "", nil)
	require.NoError(t, err)

	subs := []amplify.SubDomainSetting{
		{Prefix: "www", BranchName: "main"},
	}

	// Create
	da, err := b.CreateDomainAssociation(app.AppID, "example.com", subs, true, nil, "", nil)
	require.NoError(t, err)
	assert.Equal(t, "example.com", da.DomainName)
	assert.Equal(t, app.AppID, da.AppID)
	assert.Len(t, da.SubDomains, 1)
	assert.NotEmpty(t, da.ARN)

	// Duplicate create
	_, err = b.CreateDomainAssociation(app.AppID, "example.com", subs, false, nil, "", nil)
	require.Error(t, err)

	// Create for nonexistent app
	_, err = b.CreateDomainAssociation("nonexistent", "example.com", subs, false, nil, "", nil)
	require.Error(t, err)

	// Get
	got, err := b.GetDomainAssociation(app.AppID, "example.com")
	require.NoError(t, err)
	assert.Equal(t, "example.com", got.DomainName)

	// Get nonexistent
	_, err = b.GetDomainAssociation(app.AppID, "nothere.com")
	require.Error(t, err)

	// List
	list, _, err := b.ListDomainAssociations(app.AppID, "", 0)
	require.NoError(t, err)
	assert.Len(t, list, 1)

	// List for nonexistent app
	_, _, err = b.ListDomainAssociations("nonexistent", "", 0)
	require.Error(t, err)

	// Update
	newSubs := []amplify.SubDomainSetting{
		{Prefix: "api", BranchName: "main"},
	}
	falseVal := false
	updated, err := b.UpdateDomainAssociation(app.AppID, "example.com", newSubs, &falseVal, nil, nil, nil)
	require.NoError(t, err)
	assert.Len(t, updated.SubDomains, 1)
	assert.Equal(t, "api", updated.SubDomains[0].SubDomainSetting.Prefix)

	// Update nonexistent
	_, err = b.UpdateDomainAssociation(app.AppID, "nothere.com", newSubs, &falseVal, nil, nil, nil)
	require.Error(t, err)

	// Delete
	deleted, err := b.DeleteDomainAssociation(app.AppID, "example.com")
	require.NoError(t, err)
	assert.Equal(t, "example.com", deleted.DomainName)

	// Delete again
	_, err = b.DeleteDomainAssociation(app.AppID, "example.com")
	require.Error(t, err)
}

// TestInMemoryBackend_UpdateDomainAssociation_PartialUpdatePreservesUnsetFields
// verifies that omitting a field on UpdateDomainAssociation (nil
// subDomains/autoSubDomainCreationPatterns/enableAutoSubDomain/
// autoSubDomainIAMRole) leaves that field's existing value unchanged. Real
// Amplify's UpdateDomainAssociationInput marks none of these fields
// required, so a caller updating only, say, AutoSubDomainIAMRole must not
// have SubDomains/EnableAutoSubDomain/AutoSubDomainCreationPatterns reset to
// zero values.
func TestInMemoryBackend_UpdateDomainAssociation_PartialUpdatePreservesUnsetFields(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	app, err := b.CreateApp("TestApp", "", "", "", nil)
	require.NoError(t, err)

	subs := []amplify.SubDomainSetting{{Prefix: "www", BranchName: "main"}}
	patterns := []string{"feature/*"}

	da, err := b.CreateDomainAssociation(app.AppID, "example.com", subs, true, patterns, "role-arn", nil)
	require.NoError(t, err)
	require.Len(t, da.SubDomains, 1)
	require.True(t, da.EnableAutoSubDomain)
	require.Equal(t, patterns, da.AutoSubDomainCreationPatterns)
	require.Equal(t, "role-arn", da.AutoSubDomainIAMRole)

	newRole := "new-role-arn"
	updated, err := b.UpdateDomainAssociation(app.AppID, "example.com", nil, nil, nil, &newRole, nil)
	require.NoError(t, err)

	assert.Len(t, updated.SubDomains, 1, "SubDomains must survive an update that omits subDomainSettings")
	assert.Equal(t, "www", updated.SubDomains[0].SubDomainSetting.Prefix)
	assert.True(t, updated.EnableAutoSubDomain, "EnableAutoSubDomain must survive an update that omits it")
	assert.Equal(
		t, patterns, updated.AutoSubDomainCreationPatterns,
		"AutoSubDomainCreationPatterns must survive an update that omits it",
	)
	assert.Equal(t, newRole, updated.AutoSubDomainIAMRole)
}

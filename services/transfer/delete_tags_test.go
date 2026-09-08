package transfer_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/services/transfer"
)

// TestDelete_ClearsTagsStore covers gopherstack's "ghost rows after delete" bug
// class (see PARITY.md, b8484292f's DeleteUser fix): a resource's tags live in
// a side map (tagsStore) keyed by ARN, separate from the resource's own row.
// Deleting the resource must also clear its tagsStore entry, or a real client's
// cross-service tag listing (ListTagsForResource / Resource Groups Tagging API)
// keeps reporting tags for a resource that no longer exists.
func TestDelete_ClearsTagsStore(t *testing.T) {
	t.Parallel()

	t.Run("agreement", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend(t)
		s, err := b.CreateServer(nil, nil)
		require.NoError(t, err)
		p1, err := b.CreateProfile("LOCAL", "as2id-local", nil)
		require.NoError(t, err)
		p2, err := b.CreateProfile("PARTNER", "as2id-partner", nil)
		require.NoError(t, err)

		ag, err := b.CreateAgreement(
			s.ServerID, "desc", p1.ProfileID, p2.ProfileID, "/base", "arn:role",
			map[string]string{"env": "prod"},
		)
		require.NoError(t, err)

		agreementARN := arn.Build(
			"transfer", testRegion, testAccountID,
			"server/"+s.ServerID+"/agreement/"+ag.AgreementID,
		)
		require.Equal(t, map[string]string{"env": "prod"}, b.ListTagsForResource(agreementARN))

		require.NoError(t, b.DeleteAgreement(s.ServerID, ag.AgreementID))
		assert.Empty(t, b.ListTagsForResource(agreementARN))
	})

	t.Run("certificate", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend(t)
		cert, err := b.ImportCertificate(
			"SIGNING", testCertPEM, "desc", time.Time{}, time.Time{},
			map[string]string{"env": "prod"},
		)
		require.NoError(t, err)

		certARN := arn.Build("transfer", testRegion, testAccountID, "certificate/"+cert.CertificateID)
		require.Equal(t, map[string]string{"env": "prod"}, b.ListTagsForResource(certARN))

		require.NoError(t, b.DeleteCertificate(cert.CertificateID))
		assert.Empty(t, b.ListTagsForResource(certARN))
	})

	t.Run("connector", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend(t)
		c, err := b.CreateConnector("https://example.com", "arn:role", nil, nil, map[string]string{"env": "prod"})
		require.NoError(t, err)

		connARN := arn.Build("transfer", testRegion, testAccountID, "connector/"+c.ConnectorID)
		require.Equal(t, map[string]string{"env": "prod"}, b.ListTagsForResource(connARN))

		require.NoError(t, b.DeleteConnector(c.ConnectorID))
		assert.Empty(t, b.ListTagsForResource(connARN))
	})

	t.Run("profile", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend(t)
		p, err := b.CreateProfile("LOCAL", "as2id", map[string]string{"env": "prod"})
		require.NoError(t, err)

		profileARN := arn.Build("transfer", testRegion, testAccountID, "profile/"+p.ProfileID)
		require.Equal(t, map[string]string{"env": "prod"}, b.ListTagsForResource(profileARN))

		require.NoError(t, b.DeleteProfile(p.ProfileID))
		assert.Empty(t, b.ListTagsForResource(profileARN))
	})

	t.Run("host key", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend(t)
		s, err := b.CreateServer(nil, nil)
		require.NoError(t, err)
		hk, err := b.ImportHostKey(s.ServerID, testHostKeyEd25519, "desc", map[string]string{"env": "prod"})
		require.NoError(t, err)

		hostKeyARN := arn.Build(
			"transfer", testRegion, testAccountID, "host-key/"+s.ServerID+"/"+hk.HostKeyID,
		)
		require.Equal(t, map[string]string{"env": "prod"}, b.ListTagsForResource(hostKeyARN))

		require.NoError(t, b.DeleteHostKey(s.ServerID, hk.HostKeyID))
		assert.Empty(t, b.ListTagsForResource(hostKeyARN))
	})

	t.Run("web app", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend(t)
		w, err := b.CreateWebApp(&transfer.CreateWebAppInput{
			IdentityCenterConfig: &transfer.WebAppIdentityCenterConfig{
				InstanceArn: "arn:aws:sso:::instance/ssoins-1234567890abcdef",
				Role:        "arn:aws:iam::123456789012:role/WebAppRole",
			},
			Tags: map[string]string{"env": "prod"},
		})
		require.NoError(t, err)

		webAppARN := arn.Build("transfer", testRegion, testAccountID, "webapp/"+w.WebAppID)
		require.Equal(t, map[string]string{"env": "prod"}, b.ListTagsForResource(webAppARN))

		require.NoError(t, b.DeleteWebApp(w.WebAppID))
		assert.Empty(t, b.ListTagsForResource(webAppARN))
	})

	t.Run("workflow", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend(t)
		wf, err := b.CreateWorkflow("desc", nil, nil, map[string]string{"env": "prod"})
		require.NoError(t, err)

		workflowARN := arn.Build("transfer", testRegion, testAccountID, "workflow/"+wf.WorkflowID)
		require.Equal(t, map[string]string{"env": "prod"}, b.ListTagsForResource(workflowARN))

		require.NoError(t, b.DeleteWorkflow(wf.WorkflowID))
		assert.Empty(t, b.ListTagsForResource(workflowARN))
	})
}

// TestDeleteServer_ClearsCascadedTags covers the DeleteServer cascade path, which
// deletes users/agreements/host keys by manipulating their tables directly rather
// than calling DeleteUser/DeleteAgreement/DeleteHostKey -- so each of those
// methods' own tagsStore cleanup does not run for a server-cascade delete. The
// server's own tag entry must also be cleared.
func TestDeleteServer_ClearsCascadedTags(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)
	s, err := b.CreateServer(nil, map[string]string{"env": "prod"})
	require.NoError(t, err)

	u, err := b.CreateUser(s.ServerID, "alice", "/alice", "", map[string]string{"team": "data"})
	require.NoError(t, err)

	p1, err := b.CreateProfile("LOCAL", "as2id-local", nil)
	require.NoError(t, err)
	p2, err := b.CreateProfile("PARTNER", "as2id-partner", nil)
	require.NoError(t, err)
	ag, err := b.CreateAgreement(
		s.ServerID, "desc", p1.ProfileID, p2.ProfileID, "/base", "arn:role",
		map[string]string{"unit": "as2"},
	)
	require.NoError(t, err)

	hk, err := b.ImportHostKey(s.ServerID, testHostKeyEd25519, "desc", map[string]string{"kind": "host"})
	require.NoError(t, err)

	serverARN := arn.Build("transfer", testRegion, testAccountID, "server/"+s.ServerID)
	userARN := arn.Build("transfer", testRegion, testAccountID, "user/"+s.ServerID+"/alice")
	agreementARN := arn.Build(
		"transfer", testRegion, testAccountID, "server/"+s.ServerID+"/agreement/"+ag.AgreementID,
	)
	hostKeyARN := arn.Build(
		"transfer", testRegion, testAccountID, "host-key/"+s.ServerID+"/"+hk.HostKeyID,
	)

	require.Equal(t, map[string]string{"env": "prod"}, b.ListTagsForResource(serverARN))
	require.Equal(t, map[string]string{"team": "data"}, b.ListTagsForResource(userARN))
	require.NotEmpty(t, u.ServerID) // sanity: user really belongs to this server

	require.NoError(t, b.DeleteServer(s.ServerID))

	assert.Empty(t, b.ListTagsForResource(serverARN))
	assert.Empty(t, b.ListTagsForResource(userARN))
	assert.Empty(t, b.ListTagsForResource(agreementARN))
	assert.Empty(t, b.ListTagsForResource(hostKeyARN))
}

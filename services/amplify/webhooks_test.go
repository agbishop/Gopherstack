package amplify_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInMemoryBackend_Webhook_Lifecycle(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	app, err := b.CreateApp("WebhookApp", "", "", "", nil)
	require.NoError(t, err)
	_, err = b.CreateBranch(app.AppID, "main", "", "", false, nil)
	require.NoError(t, err)

	// Create webhook for nonexistent app
	_, err = b.CreateWebhook("nonexistent", "main", "test")
	require.Error(t, err)

	// Create webhook
	wh, err := b.CreateWebhook(app.AppID, "main", "my webhook")
	require.NoError(t, err)
	assert.NotEmpty(t, wh.WebhookID)
	assert.Equal(t, "main", wh.BranchName)
	assert.Equal(t, "my webhook", wh.Description)

	// Get webhook
	got, err := b.GetWebhook(wh.WebhookID)
	require.NoError(t, err)
	assert.Equal(t, wh.WebhookID, got.WebhookID)

	// Get nonexistent
	_, err = b.GetWebhook("doesnotexist")
	require.Error(t, err)

	// List webhooks
	list, _, err := b.ListWebhooks(app.AppID, "", 0)
	require.NoError(t, err)
	assert.Len(t, list, 1)

	// List for nonexistent app
	_, _, err = b.ListWebhooks("nonexistent", "", 0)
	require.Error(t, err)

	// Update webhook
	updated, err := b.UpdateWebhook(wh.WebhookID, "main", "updated desc")
	require.NoError(t, err)
	assert.Equal(t, "updated desc", updated.Description)

	// Update nonexistent
	_, err = b.UpdateWebhook("nonexistent", "main", "desc")
	require.Error(t, err)

	// Delete webhook
	deleted, err := b.DeleteWebhook(wh.WebhookID)
	require.NoError(t, err)
	assert.Equal(t, wh.WebhookID, deleted.WebhookID)

	// Delete again
	_, err = b.DeleteWebhook(wh.WebhookID)
	require.Error(t, err)
}

// TestInMemoryBackend_Webhook_RequiresExistingBranch verifies real Amplify's
// CreateWebhookInput.BranchName / UpdateWebhookInput.BranchName doc ("The
// name for a branch that is part of an Amplify app") is enforced: a webhook
// can't name a branch that doesn't belong to its app, since both ops model
// NotFoundException.
func TestInMemoryBackend_Webhook_RequiresExistingBranch(t *testing.T) {
	t.Parallel()

	t.Run("create_rejects_unknown_branch", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend()
		app, err := b.CreateApp("WebhookNoBranchApp", "", "", "", nil)
		require.NoError(t, err)

		_, err = b.CreateWebhook(app.AppID, "does-not-exist", "test")
		require.Error(t, err)
	})

	t.Run("update_rejects_unknown_branch", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend()
		app, err := b.CreateApp("WebhookUpdateNoBranchApp", "", "", "", nil)
		require.NoError(t, err)
		_, err = b.CreateBranch(app.AppID, "main", "", "", false, nil)
		require.NoError(t, err)

		wh, err := b.CreateWebhook(app.AppID, "main", "test")
		require.NoError(t, err)

		_, err = b.UpdateWebhook(wh.WebhookID, "does-not-exist", "desc")
		require.Error(t, err)
	})
}

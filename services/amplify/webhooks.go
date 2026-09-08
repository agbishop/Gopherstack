package amplify

import (
	"fmt"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// CreateWebhook creates a new webhook for an app branch. Real Amplify's
// CreateWebhookInput.BranchName doc reads "The name for a branch that is
// part of an Amplify app" (aws-sdk-go-v2/service/amplify
// api_op_CreateWebhook.go), and CreateWebhook models NotFoundException, so a
// branch not belonging to appID is rejected rather than accepted as a
// dangling reference.
func (b *InMemoryBackend) CreateWebhook(appID, branchName, description string) (*Webhook, error) {
	b.mu.Lock("CreateWebhook")
	defer b.mu.Unlock()

	if !b.apps.Has(appID) {
		return nil, fmt.Errorf("%w: app %s not found", ErrNotFound, appID)
	}

	if !b.branches.Has(branchKey(appID, branchName)) {
		return nil, fmt.Errorf("%w: branch %s not found for app %s", ErrNotFound, branchName, appID)
	}

	webhookID := randomID()
	webhookARN := arn.Build(
		"amplify",
		b.region,
		b.accountID,
		fmt.Sprintf("apps/%s/webhooks/%s", appID, webhookID),
	)
	now := time.Now().UTC()

	wh := &Webhook{
		WebhookID:   webhookID,
		WebhookARN:  webhookARN,
		AppID:       appID,
		BranchName:  branchName,
		Description: description,
		WebhookURL: "https://webhooks.amplify." + b.region +
			".amazonaws.com/prod/webhooks?id=" + webhookID + "&token=" + randomID(),
		CreateTime: now,
		UpdateTime: now,
	}

	b.webhooks.Put(wh)

	cp := *wh

	return &cp, nil
}

// UpdateWebhook updates a webhook. Real Amplify's UpdateWebhookInput.BranchName
// doc reads "The name for a branch that is part of an Amplify app"
// (aws-sdk-go-v2/service/amplify api_op_UpdateWebhook.go), and UpdateWebhook
// models NotFoundException, so retargeting a webhook at a branch not
// belonging to the webhook's app is rejected rather than accepted as a
// dangling reference.
func (b *InMemoryBackend) UpdateWebhook(
	webhookID, branchName, description string,
) (*Webhook, error) {
	b.mu.Lock("UpdateWebhook")
	defer b.mu.Unlock()

	wh, ok := b.webhooks.Get(webhookID)
	if !ok {
		return nil, fmt.Errorf("%w: webhook %s not found", ErrNotFound, webhookID)
	}

	if branchName != "" {
		if !b.branches.Has(branchKey(wh.AppID, branchName)) {
			return nil, fmt.Errorf("%w: branch %s not found for app %s", ErrNotFound, branchName, wh.AppID)
		}

		wh.BranchName = branchName
	}

	if description != "" {
		wh.Description = description
	}

	wh.UpdateTime = time.Now().UTC()

	cp := *wh

	return &cp, nil
}

// DeleteWebhook deletes a webhook.
func (b *InMemoryBackend) DeleteWebhook(webhookID string) (*Webhook, error) {
	b.mu.Lock("DeleteWebhook")
	defer b.mu.Unlock()

	wh, ok := b.webhooks.Get(webhookID)
	if !ok {
		return nil, fmt.Errorf("%w: webhook %s not found", ErrNotFound, webhookID)
	}

	cp := *wh
	b.webhooks.Delete(webhookID)

	return &cp, nil
}

// GetWebhook returns a webhook by ID.
func (b *InMemoryBackend) GetWebhook(webhookID string) (*Webhook, error) {
	b.mu.RLock("GetWebhook")
	defer b.mu.RUnlock()

	wh, ok := b.webhooks.Get(webhookID)
	if !ok {
		return nil, fmt.Errorf("%w: webhook %s not found", ErrNotFound, webhookID)
	}

	cp := *wh

	return &cp, nil
}

// ListWebhooks lists webhooks for an app.
func (b *InMemoryBackend) ListWebhooks(
	appID, nextToken string,
	maxResults int,
) ([]*Webhook, string, error) {
	b.mu.RLock("ListWebhooks")
	defer b.mu.RUnlock()

	if !b.apps.Has(appID) {
		return nil, "", fmt.Errorf("%w: app %s not found", ErrNotFound, appID)
	}

	var all []*Webhook

	for _, wh := range b.webhooksByApp.Get(appID) {
		cp := *wh
		all = append(all, &cp)
	}

	sort.Slice(all, func(i, j int) bool { return all[i].WebhookID < all[j].WebhookID })

	page, token := amplifyPaginate(all, nextToken, maxResults)

	return page, token, nil
}

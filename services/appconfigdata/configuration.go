package appconfigdata

import (
	"encoding/json"
	"fmt"
	"time"
)

// SetConfiguration stores or updates configuration content for a profile,
// with no deployment attribution -- used by the dashboard's manual seeding
// path. See PublishConfiguration for deployment-originated updates.
// Returns ErrContentTooLarge if content exceeds maxContentBytes.
// Returns ErrContentTypeMismatch if contentType indicates JSON but content is not valid JSON.
func (b *InMemoryBackend) SetConfiguration(app, env, profile, content, contentType string) error {
	return b.setConfiguration(app, env, profile, content, contentType, "")
}

// PublishConfiguration stores configuration content produced by a real
// AppConfig deployment, stamping deploymentID onto the new version. It
// structurally satisfies appconfig.DeployedConfigurationPublisher -- the
// appconfig -> appconfigdata bridge (bd gopherstack-uiyi) wires
// *InMemoryBackend in directly, no adapter needed.
func (b *InMemoryBackend) PublishConfiguration(
	applicationID, environmentID, profileID, content, contentType, deploymentID string,
) error {
	return b.setConfiguration(applicationID, environmentID, profileID, content, contentType, deploymentID)
}

func (b *InMemoryBackend) setConfiguration(app, env, profile, content, contentType, deploymentID string) error {
	if len(content) > maxContentBytes {
		return ErrContentTooLarge
	}

	if isJSONContentType(contentType) {
		var v any
		if err := json.Unmarshal([]byte(content), &v); err != nil {
			return ErrContentTypeMismatch
		}
	}

	b.mu.Lock("SetConfiguration")
	defer b.mu.Unlock()

	key := profileKey(app, env, profile)
	now := time.Now().UTC()
	hash := normalizedContentHash(content, contentType)

	existing, _ := b.profiles.Get(key)
	var history []ConfigVersion
	var nextVersion int
	var changed bool

	if existing != nil && !existing.UpdatedAt.IsZero() {
		nextVersion = existing.VersionNumber + 1
		changed = existing.ContentHash != hash
		entry := ConfigVersion{
			Content:       existing.Content,
			ContentType:   existing.ContentType,
			ContentHash:   existing.ContentHash,
			UpdatedAt:     existing.UpdatedAt,
			VersionLabel:  existing.VersionLabel,
			VersionNumber: existing.VersionNumber,
			DeploymentID:  existing.DeploymentID,
		}
		history = append(history, entry)
		history = append(history, existing.History...)
		if len(history) > maxHistoryEntries {
			history = history[:maxHistoryEntries]
		}
	} else {
		nextVersion = 1
		changed = true
	}

	versionLabel := fmt.Sprintf("v%d", nextVersion)

	b.profiles.Put(&ConfigurationProfile{
		ApplicationIdentifier:          app,
		EnvironmentIdentifier:          env,
		ConfigurationProfileIdentifier: profile,
		Content:                        content,
		ContentType:                    contentType,
		ContentHash:                    hash,
		VersionLabel:                   versionLabel,
		DeploymentID:                   deploymentID,
		VersionNumber:                  nextVersion,
		UpdatedAt:                      now,
		History:                        history,
	})

	if changed {
		b.totalChanges.Add(1)
	}

	return nil
}

// validateSession looks up the session for token, checks expiry, poll interval, MAC, and
// that the underlying profile still exists.  It returns the validated session and profile,
// or an error.  Callers must hold b.mu.
func (b *InMemoryBackend) validateSession(
	token string, now time.Time,
) (*Session, *ConfigurationProfile, error) {
	sess, ok := b.sessions.Get(token)
	if !ok {
		return nil, nil, ErrSessionNotFound
	}

	if now.After(sess.ExpiresAt) {
		b.sessions.Delete(token)

		return nil, nil, ErrTokenExpired
	}

	if !sess.LastPollAt.IsZero() && sess.PollIntervalInSeconds > 0 {
		minInterval := time.Duration(sess.PollIntervalInSeconds) * time.Second
		if now.Sub(sess.LastPollAt) < minInterval {
			return nil, nil, ErrPollTooFrequent
		}
	}

	if !b.verifyTokenMAC(token, sess.TokenFamilyID) {
		b.sessions.Delete(token)

		return nil, nil, ErrTokenCorrupted
	}

	key := profileKey(sess.ApplicationIdentifier, sess.EnvironmentIdentifier, sess.ConfigurationProfileIdentifier)
	profile, _ := b.profiles.Get(key)

	if profile == nil {
		b.sessions.Delete(token)

		return nil, nil, ErrResourceRemoved
	}

	return sess, profile, nil
}

// GetLatestConfiguration retrieves configuration data for the given token and returns a new token.
// The token is rotated on every successful call; the old token enters a short grace window so
// that clients can safely retry after a transient failure without losing their session.
//
// Returned values: content, contentType, nextToken, contentHash, versionLabel, error.
func (b *InMemoryBackend) GetLatestConfiguration(
	token string,
) ([]byte, string, string, string, string, error) {
	b.mu.Lock("GetLatestConfiguration")
	defer b.mu.Unlock()

	now := time.Now().UTC()

	// Fast path: check grace tokens first to serve idempotent retries.
	if grace, ok := b.graceTokens.Get(token); ok {
		if now.Before(grace.ExpiresAt) {
			b.totalPolls.Add(1)

			return grace.Content, grace.ContentType, grace.NextToken, grace.ContentHash, grace.VersionLabel, nil
		}
		// Grace period expired; fall through to the normal "not found" path.
		b.graceTokens.Delete(token)
	}

	sess, profile, err := b.validateSession(token, now)
	if err != nil {
		b.totalFailures.Add(1)

		return nil, "", "", "", "", err
	}

	// Change detection: return empty content when the profile hash matches the previous poll.
	// VersionLabel is likewise blanked when unchanged -- per api_op_GetLatestConfiguration.go's
	// doc comment on the VersionLabel field: "If the client already has the latest version of
	// the configuration data, this value is empty."
	var content []byte
	var versionLabel string
	contentType := "application/octet-stream"
	hash := profile.ContentHash

	if hash != sess.PreviousContentHash {
		content = []byte(profile.Content)
		versionLabel = profile.VersionLabel
		if profile.ContentType != "" {
			contentType = profile.ContentType
		}
	}

	// Rotate token: the next token belongs to the same family.
	nextToken, err := b.generateToken(sess.TokenFamilyID)
	if err != nil {
		return nil, "", "", "", "", fmt.Errorf("generating next token: %w", err)
	}

	newSess := *sess
	newSess.Token = nextToken
	newSess.LastAccessedAt = now
	newSess.LastPollAt = now
	newSess.PollCount = sess.PollCount + 1
	newSess.PreviousContentHash = hash

	b.sessions.Delete(token)
	b.sessions.Put(&newSess)

	// Keep the old token alive for a short grace period (retry idempotency).
	b.graceTokens.Put(&graceEntry{
		Token:        token,
		NextToken:    nextToken,
		Content:      content,
		ContentType:  contentType,
		ContentHash:  hash,
		VersionLabel: versionLabel,
		ExpiresAt:    now.Add(tokenGracePeriod),
	})

	b.totalPolls.Add(1)

	return content, contentType, nextToken, hash, versionLabel, nil
}

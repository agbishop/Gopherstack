package appconfig

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedDeployableConfig creates an application, environment, hosted
// configuration profile, hosted configuration version, and zero-duration
// deployment strategy -- everything StartDeployment needs -- returning the
// IDs and the raw content that was uploaded.
func seedDeployableConfig(t *testing.T, b *InMemoryBackend, content []byte) (string, string, string, string) {
	t.Helper()

	app, err := b.CreateApplication("cfg-app", "", nil)
	require.NoError(t, err)

	env, err := b.CreateEnvironment(app.ID, "cfg-env", "", nil, nil)
	require.NoError(t, err)

	profile, err := b.CreateConfigurationProfile(app.ID, "cfg-profile", "", "hosted", "AWS.Freeform", "", "", nil, nil)
	require.NoError(t, err)

	_, err = b.CreateHostedConfigurationVersion(app.ID, profile.ID, "application/json", "", "", content, nil)
	require.NoError(t, err)

	strategy, err := b.CreateDeploymentStrategy("cfg-strat", "", 0, 0, 100, "LINEAR", "NONE", nil)
	require.NoError(t, err)

	return app.ID, env.ID, profile.ID, strategy.ID
}

func deployedConfigCount(b *InMemoryBackend) int {
	b.mu.RLock("test.deployedConfigCount")
	defer b.mu.RUnlock()

	return len(b.deployedConfigs)
}

// TestBackend_DeployedConfig_CascadeDeleteOnEnvironment verifies that
// deleting an environment removes its deployedConfigs tracking entry, so a
// re-created environment with the same ID never inherits stale deployed
// content.
func TestBackend_DeployedConfig_CascadeDeleteOnEnvironment(t *testing.T) {
	t.Parallel()

	b := NewInMemoryBackend("123456789012", "us-east-1")
	appID, envID, profileID, strategyID := seedDeployableConfig(t, b, []byte(`{}`))

	_, err := b.StartDeployment(appID, envID, profileID, strategyID, "1", "", nil, nil, nil)
	require.NoError(t, err)
	require.Equal(t, 1, deployedConfigCount(b))

	require.NoError(t, b.DeleteEnvironment(appID, envID, ""))
	assert.Equal(t, 0, deployedConfigCount(b), "deployedConfigs must not leak after DeleteEnvironment")
}

// TestBackend_DeployedConfig_CascadeDeleteOnApplication verifies that
// deleting an application removes every deployedConfigs entry for it.
func TestBackend_DeployedConfig_CascadeDeleteOnApplication(t *testing.T) {
	t.Parallel()

	b := NewInMemoryBackend("123456789012", "us-east-1")
	appID, envID, profileID, strategyID := seedDeployableConfig(t, b, []byte(`{}`))

	_, err := b.StartDeployment(appID, envID, profileID, strategyID, "1", "", nil, nil, nil)
	require.NoError(t, err)
	require.Equal(t, 1, deployedConfigCount(b))

	require.NoError(t, b.DeleteApplication(appID))
	assert.Equal(t, 0, deployedConfigCount(b), "deployedConfigs must not leak after DeleteApplication")
}

// TestBackend_DeployedConfig_CascadeDeleteOnProfile verifies that deleting
// a configuration profile removes its deployedConfigs entry.
func TestBackend_DeployedConfig_CascadeDeleteOnProfile(t *testing.T) {
	t.Parallel()

	b := NewInMemoryBackend("123456789012", "us-east-1")
	appID, envID, profileID, strategyID := seedDeployableConfig(t, b, []byte(`{}`))

	_, err := b.StartDeployment(appID, envID, profileID, strategyID, "1", "", nil, nil, nil)
	require.NoError(t, err)
	require.Equal(t, 1, deployedConfigCount(b))

	require.NoError(t, b.DeleteConfigurationProfile(appID, profileID, ""))
	assert.Equal(t, 0, deployedConfigCount(b), "deployedConfigs must not leak after DeleteConfigurationProfile")
}

// TestBackend_ExtensionAssociation_CascadeDeleteOnApplication verifies that
// deleting an application removes extension associations targeting the
// application, its environments, and its configuration profiles -- not
// just the resources themselves.
func TestBackend_ExtensionAssociation_CascadeDeleteOnApplication(t *testing.T) {
	t.Parallel()

	b := NewInMemoryBackend("123456789012", "us-east-1")

	app, err := b.CreateApplication("cascade-assoc-app", "", nil)
	require.NoError(t, err)

	env, err := b.CreateEnvironment(app.ID, "cascade-assoc-env", "", nil, nil)
	require.NoError(t, err)

	profile, err := b.CreateConfigurationProfile(
		app.ID, "cascade-assoc-profile", "", "hosted", "AWS.Freeform", "", "", nil,
		nil,
	)
	require.NoError(t, err)

	ext, err := b.CreateExtension("cascade-assoc-ext", "", nil, nil, nil)
	require.NoError(t, err)

	appArn := "arn:aws:appconfig:us-east-1:123456789012:application/" + app.ID
	envArn := appArn + "/environment/" + env.ID
	profileArn := appArn + "/configurationprofile/" + profile.ID

	for _, resourceArn := range []string{appArn, envArn, profileArn} {
		_, assocErr := b.CreateExtensionAssociation(ext.ID, resourceArn, nil, nil, nil)
		require.NoError(t, assocErr)
	}

	require.Equal(t, 3, b.extensionAssociations.Len())

	require.NoError(t, b.DeleteApplication(app.ID))
	assert.Equal(t, 0, b.extensionAssociations.Len(),
		"associations targeting the app/env/profile must not survive DeleteApplication")
}

// TestDeploymentTimers_DrainToZero verifies that the background progression
// goroutine (scheduleDeploymentReconcilerLocked in deployments.go) does not
// leak: every in-flight deployment's timer entry is removed once the
// deployment reaches a terminal state, and the goroutine is self-terminating.
func TestDeploymentTimers_DrainToZero(t *testing.T) {
	t.Parallel()

	b := NewInMemoryBackend("123456789012", "us-east-1")

	app, err := b.CreateApplication("timer-leak-app", "", nil)
	require.NoError(t, err)

	env, err := b.CreateEnvironment(app.ID, "timer-leak-env", "", nil, nil)
	require.NoError(t, err)

	profile, err := b.CreateConfigurationProfile(
		app.ID, "timer-leak-profile", "", "hosted", "AWS.Freeform", "", "", nil,
		nil,
	)
	require.NoError(t, err)

	_, err = b.CreateHostedConfigurationVersion(
		app.ID, profile.ID, "application/json", "", "", []byte(`{}`), nil,
	)
	require.NoError(t, err)

	// A non-zero duration and bake time forces real DEPLOYING -> BAKING
	// progression (registers a timer), rather than the synchronous
	// zero-duration path (which never touches deploymentTimers at all).
	strategy, err := b.CreateDeploymentStrategy("timer-leak-strat", "", 10, 5, 25, "LINEAR", "NONE", nil)
	require.NoError(t, err)

	const deployments = 5

	for range deployments {
		_, startErr := b.StartDeployment(app.ID, env.ID, profile.ID, strategy.ID, "1", "", nil, nil, nil)
		require.NoError(t, startErr)
	}

	deploymentTimerCount := func() int {
		b.mu.RLock("test.deploymentTimerCount")
		defer b.mu.RUnlock()

		return len(b.deploymentTimers)
	}

	assert.Positive(t, deploymentTimerCount(), "sanity: progression must actually register timers")

	deadline := time.Now().Add(2 * time.Second)
	for deploymentTimerCount() > 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	assert.Equal(t, 0, deploymentTimerCount(), "every deployment timer must drain once its deployment completes")
}

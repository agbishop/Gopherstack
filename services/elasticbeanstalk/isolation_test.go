package elasticbeanstalk //nolint:testpackage // needs access to the unexported region context key.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ebCtxRegion(region string) context.Context {
	return context.WithValue(context.Background(), regionContextKey{}, region)
}

// TestEBRegionIsolation proves that same-named EB resources created in two
// different regions are fully isolated: each region sees only its own
// resources, ARNs embed the correct region, and deleting in one region leaves
// the other untouched.
func TestEBRegionIsolation(t *testing.T) {
	t.Parallel()

	backend := NewInMemoryBackend("000000000000", "us-east-1")

	ctxEast := ebCtxRegion("us-east-1")
	ctxWest := ebCtxRegion("us-west-2")

	// 1. Create an application with the SAME name in both regions.
	eastApp, err := backend.CreateApplication(ctxEast, "shared-app", "east app", nil)
	require.NoError(t, err)
	assert.Contains(t, eastApp.ApplicationARN, "us-east-1")

	westApp, err := backend.CreateApplication(ctxWest, "shared-app", "west app", nil)
	require.NoError(t, err)
	assert.Contains(t, westApp.ApplicationARN, "us-west-2")

	// ARNs must differ even though names match.
	assert.NotEqual(t, eastApp.ApplicationARN, westApp.ApplicationARN)

	// 2. Each region reads back its own application.
	eastApps := backend.DescribeApplications(ctxEast, []string{"shared-app"})
	require.Len(t, eastApps, 1)
	assert.Equal(t, "east app", eastApps[0].Description)

	westApps := backend.DescribeApplications(ctxWest, []string{"shared-app"})
	require.Len(t, westApps, 1)
	assert.Equal(t, "west app", westApps[0].Description)

	// 3. Environments with the same name are isolated too.
	eastEnv, err := backend.CreateEnvironment(
		ctxEast, "shared-app", "shared-env",
		"64bit Amazon Linux", "", nil, CreateEnvironmentParams{},
	)
	require.NoError(t, err)
	assert.Equal(t, "us-east-1", eastEnv.Region)
	assert.Contains(t, eastEnv.EnvironmentARN, "us-east-1")
	assert.Contains(t, eastEnv.CNAME, "us-east-1")

	westEnv, err := backend.CreateEnvironment(
		ctxWest, "shared-app", "shared-env",
		"64bit Amazon Linux", "", nil, CreateEnvironmentParams{},
	)
	require.NoError(t, err)
	assert.Equal(t, "us-west-2", westEnv.Region)
	assert.Contains(t, westEnv.EnvironmentARN, "us-west-2")

	// 4. Listing without filter returns only the region's own environments.
	eastEnvs := backend.DescribeEnvironments(ctxEast, "", nil, nil)
	require.Len(t, eastEnvs, 1)
	assert.Equal(t, "us-east-1", eastEnvs[0].Region)

	westEnvs := backend.DescribeEnvironments(ctxWest, "", nil, nil)
	require.Len(t, westEnvs, 1)
	assert.Equal(t, "us-west-2", westEnvs[0].Region)

	// 5. Deleting the application in us-east-1 must not affect us-west-2.
	require.NoError(t, backend.DeleteApplication(ctxEast, "shared-app", true))

	eastGone := backend.DescribeApplications(ctxEast, []string{"shared-app"})
	assert.Empty(t, eastGone)

	// Cascade should have removed east's environments.
	eastEnvsGone := backend.DescribeEnvironments(ctxEast, "", nil, nil)
	assert.Empty(t, eastEnvsGone)

	westStill := backend.DescribeApplications(ctxWest, []string{"shared-app"})
	require.Len(t, westStill, 1)
	assert.Equal(t, "west app", westStill[0].Description)

	westEnvsStill := backend.DescribeEnvironments(ctxWest, "", nil, nil)
	require.Len(t, westEnvsStill, 1)
}

// TestEBAppVersionRegionIsolation proves that application versions with the same
// label are isolated per region.
func TestEBAppVersionRegionIsolation(t *testing.T) {
	t.Parallel()

	backend := NewInMemoryBackend("000000000000", "us-east-1")

	ctxEast := ebCtxRegion("us-east-1")
	ctxWest := ebCtxRegion("us-west-2")

	_, err := backend.CreateApplication(ctxEast, "my-app", "", nil)
	require.NoError(t, err)
	_, err = backend.CreateApplication(ctxWest, "my-app", "", nil)
	require.NoError(t, err)

	eastVer, err := backend.CreateApplicationVersion(ctxEast, "my-app", "v1", "east v1", "", "", nil)
	require.NoError(t, err)
	assert.Contains(t, eastVer.ApplicationVersionARN, "us-east-1")

	westVer, err := backend.CreateApplicationVersion(ctxWest, "my-app", "v1", "west v1", "", "", nil)
	require.NoError(t, err)
	assert.Contains(t, westVer.ApplicationVersionARN, "us-west-2")

	assert.NotEqual(t, eastVer.ApplicationVersionARN, westVer.ApplicationVersionARN)

	eastVers := backend.DescribeApplicationVersions(ctxEast, "my-app", nil)
	require.Len(t, eastVers, 1)
	assert.Equal(t, "east v1", eastVers[0].Description)

	westVers := backend.DescribeApplicationVersions(ctxWest, "my-app", nil)
	require.Len(t, westVers, 1)
	assert.Equal(t, "west v1", westVers[0].Description)
}

// TestEBTagRegionIsolation proves tags resolved by ARN are scoped to the request region.
func TestEBTagRegionIsolation(t *testing.T) {
	t.Parallel()

	backend := NewInMemoryBackend("000000000000", "us-east-1")

	ctxEast := ebCtxRegion("us-east-1")
	ctxWest := ebCtxRegion("us-west-2")

	eastApp, err := backend.CreateApplication(ctxEast, "tag-app", "", map[string]string{"env": "prod"})
	require.NoError(t, err)

	tags, err := backend.ListTagsForResource(ctxEast, eastApp.ApplicationARN)
	require.NoError(t, err)
	assert.Equal(t, "prod", tags["env"])

	// The east ARN must not be resolvable from the west region.
	_, err = backend.ListTagsForResource(ctxWest, eastApp.ApplicationARN)
	require.Error(t, err, "east ARN must not be tag-resolvable from the west region")
}

// TestEBDefaultRegionFallback verifies that a context without a region falls
// back to the backend's configured default region.
func TestEBDefaultRegionFallback(t *testing.T) {
	t.Parallel()

	backend := NewInMemoryBackend("000000000000", "eu-central-1")

	// No region in context -> default region store.
	_, err := backend.CreateApplication(context.Background(), "def-app", "default region app", nil)
	require.NoError(t, err)

	// Reading via the explicit default region sees it.
	list := backend.DescribeApplications(ebCtxRegion("eu-central-1"), nil)
	require.Len(t, list, 1)
	assert.Equal(t, "default region app", list[0].Description)

	// A different region sees nothing.
	other := backend.DescribeApplications(ebCtxRegion("ap-south-1"), nil)
	assert.Empty(t, other)
}

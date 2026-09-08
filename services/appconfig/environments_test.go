package appconfig_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/appconfig"
)

func TestBackend_CreateEnvironment_AppNotFound(t *testing.T) {
	t.Parallel()

	b := appconfig.NewInMemoryBackend("123456789012", "us-east-1")
	_, err := b.CreateEnvironment("nonexistent", "env", "", nil, nil)
	require.Error(t, err)
}

func TestBackend_GetEnvironment_NotFound(t *testing.T) {
	t.Parallel()

	b := appconfig.NewInMemoryBackend("123456789012", "us-east-1")
	_, err := b.GetEnvironment("app-1", "env-1")
	require.Error(t, err)
}

func TestBackend_ListEnvironments_AppNotFound(t *testing.T) {
	t.Parallel()

	b := appconfig.NewInMemoryBackend("123456789012", "us-east-1")
	_, _, err := b.ListEnvironments("nonexistent", "", 0)
	require.Error(t, err)
}

func TestBackend_UpdateEnvironment_NotFound(t *testing.T) {
	t.Parallel()

	b := appconfig.NewInMemoryBackend("123456789012", "us-east-1")
	_, err := b.UpdateEnvironment("app-1", "env-1", new("name"), new(""), nil)
	require.Error(t, err)
}

func TestBackend_DeleteEnvironment_NotFound(t *testing.T) {
	t.Parallel()

	b := appconfig.NewInMemoryBackend("123456789012", "us-east-1")
	err := b.DeleteEnvironment("app-1", "env-1", "")
	require.Error(t, err)
}

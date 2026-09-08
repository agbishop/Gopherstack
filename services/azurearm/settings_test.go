package azurearm_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/blackbirdworks/gopherstack/services/azurearm"
)

func TestDefaultSettings(t *testing.T) {
	t.Parallel()

	s := azurearm.DefaultSettings()

	assert.Equal(t, azurearm.DefaultTenantID, s.TenantID)
	assert.Equal(t, azurearm.DefaultSubscriptionID, s.SubscriptionID)
	assert.Equal(t, azurearm.DefaultClientID, s.ClientID)
	assert.Equal(t, azurearm.DefaultClientSecret, s.ClientSecret)
	assert.Equal(t, azurearm.DefaultEnvironmentName, s.Environment)
	assert.Equal(t, azurearm.DefaultLocation, s.Location)
	assert.Equal(t, azurearm.DefaultPort, s.Port)
	assert.False(t, s.ValidateTokens)
}

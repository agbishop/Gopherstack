package azurearm_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/services/azurearm"
)

type fakeConfig struct {
	settings azurearm.Settings
}

func (c fakeConfig) GetAzureARMSettings() azurearm.Settings { return c.settings }

func TestProvider_Init(t *testing.T) {
	t.Parallel()

	tests := []struct {
		appCtx   *service.AppContext
		name     string
		wantPort int
		wantErr  bool
	}{
		{
			name:    "nil app context errors",
			appCtx:  nil,
			wantErr: true,
		},
		{
			name:     "no ConfigProvider uses defaults",
			appCtx:   &service.AppContext{Config: struct{}{}},
			wantPort: azurearm.DefaultPort,
		},
		{
			name: "ConfigProvider settings are used",
			appCtx: &service.AppContext{Config: fakeConfig{
				settings: azurearm.Settings{Port: 19999, Environment: "custom"},
			}},
			wantPort: 19999,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := &azurearm.Provider{}

			reg, err := p.Init(tt.appCtx)
			if tt.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, azurearm.ErrNilAppContext)

				return
			}

			require.NoError(t, err)

			h, ok := reg.(*azurearm.Handler)
			require.True(t, ok)
			assert.Equal(t, tt.wantPort, h.Port)
		})
	}
}

func TestProvider_Name(t *testing.T) {
	t.Parallel()

	p := &azurearm.Provider{}
	assert.Equal(t, "AzureARM", p.Name())
}

package azureservicebus_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/services/azureservicebus"
)

type fakeConfigProvider struct {
	settings azureservicebus.Settings
}

func (f fakeConfigProvider) GetAzureServiceBusSettings() azureservicebus.Settings { return f.settings }

func TestProvider_Init(t *testing.T) {
	t.Parallel()

	tests := []struct {
		config  any
		name    string
		wantErr bool
		wantNil bool
	}{
		{
			name:    "nil app context",
			config:  nil,
			wantErr: true,
		},
		{
			name:   "no ConfigProvider uses defaults",
			config: struct{}{},
		},
		{
			name:   "ConfigProvider settings are applied",
			config: fakeConfigProvider{settings: azureservicebus.Settings{Port: 55555}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := &azureservicebus.Provider{}

			assert.Equal(t, "AzureServiceBus", p.Name())

			if tt.name == "nil app context" {
				_, err := p.Init(nil)
				require.Error(t, err)
				require.ErrorIs(t, err, azureservicebus.ErrNilAppContext)

				return
			}

			appCtx := &service.AppContext{Config: tt.config}

			reg, err := p.Init(appCtx)
			require.NoError(t, err)
			require.NotNil(t, reg)

			handler, isHandler := reg.(*azureservicebus.Handler)
			require.True(t, isHandler)

			if cp, isFake := tt.config.(fakeConfigProvider); isFake {
				assert.Equal(t, cp.settings.Port, handler.Port)
			} else {
				assert.Equal(t, azureservicebus.DefaultPort, handler.Port)
			}
		})
	}
}

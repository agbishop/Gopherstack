// Whitebox: validateEntityConfig is unexported and has no other seam to
// exercise it through directly (handler_test.go covers it indirectly over
// HTTP via TestHandler_CreateQueueAndSubscription_LockDurationValidation).
package azureservicebus

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateEntityConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     EntityConfig
		wantErr bool
	}{
		{name: "zero-valued config is valid", cfg: EntityConfig{}, wantErr: false},
		{
			name: "LockDuration at the maximum is valid",
			cfg:  EntityConfig{LockDuration: MaxLockDuration}, wantErr: false,
		},
		{
			name: "LockDuration one second over the maximum is invalid",
			cfg:  EntityConfig{LockDuration: MaxLockDuration + time.Second}, wantErr: true,
		},
		{
			name: "negative MaxDeliveryCount is invalid",
			cfg:  EntityConfig{MaxDeliveryCount: -1}, wantErr: true,
		},
		{
			name: "MaxDeliveryCount of zero is valid (indistinguishable from absent)",
			cfg:  EntityConfig{MaxDeliveryCount: 0}, wantErr: false,
		},
		{
			name: "positive MaxDeliveryCount is valid",
			cfg:  EntityConfig{MaxDeliveryCount: 5}, wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateEntityConfig(tt.cfg)

			if tt.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, ErrInvalidEntityConfig)

				return
			}

			assert.NoError(t, err)
		})
	}
}

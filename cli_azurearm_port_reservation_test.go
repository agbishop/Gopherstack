package main

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/portalloc"
	azurearmbackend "github.com/blackbirdworks/gopherstack/services/azurearm"
)

// TestReserveFixedServicePorts_AzureARM is a sibling to
// cli_azuretable_port_reservation_test.go's TestReserveFixedServicePorts_AzureTable,
// covering AzureARM's own fixed-port reservation. services/azurearm binds
// its dedicated HTTPS listener directly via net.Listen, not through
// PortAlloc, but its default port (10006) sits inside PortRangeStart/
// PortRangeEnd's own default range (10000-10100). Without reserving it,
// PortAlloc could still hand that same port number to an unrelated caller,
// which would only surface later as a confusing address-in-use failure. See
// AZURE.md section 10.7 and pkgs/portalloc.Allocator.Reserve's doc comment.
func TestReserveFixedServicePorts_AzureARM(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		armPort             int
		rangeStart          int
		rangeEnd            int
		wantBlockedFromPool bool
	}{
		{
			name:    "default azurearm port collides with default pool range",
			armPort: azurearmbackend.DefaultPort, rangeStart: 10000, rangeEnd: 10100,
			wantBlockedFromPool: true,
		},
		{
			name:    "custom azurearm port outside a custom pool range",
			armPort: 9998, rangeStart: 10000, rangeEnd: 10100,
			wantBlockedFromPool: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			alloc, err := portalloc.New(tt.rangeStart, tt.rangeEnd)
			require.NoError(t, err)

			cli := CLI{AzureARM: azurearmbackend.Settings{Port: tt.armPort}}
			reserveFixedServicePorts(t.Context(), slog.Default(), alloc, cli)

			assert.Equal(t, tt.wantBlockedFromPool, alloc.IsAllocated(tt.armPort), tt.name)
		})
	}
}

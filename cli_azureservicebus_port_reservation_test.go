package main

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/portalloc"
	azureservicebusbackend "github.com/blackbirdworks/gopherstack/services/azureservicebus"
)

// TestReserveFixedServicePorts_AzureServiceBus is a sibling to
// cli_azuretable_port_reservation_test.go's TestReserveFixedServicePorts_AzureTable,
// covering AzureServiceBus's own fixed-port reservation instead of
// restructuring that file's AzureBlob-only test table: services/azureservicebus
// binds its dedicated listener directly via net.Listen, not through
// PortAlloc, but its default port (10003) sits inside
// PortRangeStart/PortRangeEnd's own default range (10000-10100). Without
// reserving it, PortAlloc could still hand that same port number to an
// unrelated caller (e.g. ElastiCache), which would only surface later as a
// confusing address-in-use failure. See AZURE.md section 9 (M5) and
// pkgs/portalloc.Allocator.Reserve's doc comment.
func TestReserveFixedServicePorts_AzureServiceBus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		azurePort           int
		rangeStart          int
		rangeEnd            int
		wantBlockedFromPool bool
	}{
		{
			name:      "default azure service bus port collides with default pool range",
			azurePort: azureservicebusbackend.DefaultPort, rangeStart: 10000, rangeEnd: 10100,
			wantBlockedFromPool: true,
		},
		{
			name:      "custom azure service bus port outside a custom pool range",
			azurePort: 9997, rangeStart: 10000, rangeEnd: 10100,
			wantBlockedFromPool: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			alloc, err := portalloc.New(tt.rangeStart, tt.rangeEnd)
			require.NoError(t, err)

			cli := CLI{AzureServiceBus: azureservicebusbackend.Settings{Port: tt.azurePort}}
			reserveFixedServicePorts(t.Context(), slog.Default(), alloc, cli)

			assert.Equal(t, tt.wantBlockedFromPool, alloc.IsAllocated(tt.azurePort), tt.name)
		})
	}
}

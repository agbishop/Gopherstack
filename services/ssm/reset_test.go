package ssm_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/blackbirdworks/gopherstack/services/ssm"
)

// TestReset_ClearsInstancePatchAndPropertyState verifies Reset clears
// instancePatchStates, instanceProperties, instancePatches, and
// availablePatches -- none of which db_instances-style Reset tests cover.
// instancePatchStates/instanceProperties previously had a comment in Reset
// recording a deliberate decision to leave them uncleared; gopherstack-tr3i
// overturns that decision (see services/ssm/store.go's Reset for why
// clearing them in place, rather than reallocating the map, is still
// required).
func TestReset_ClearsInstancePatchAndPropertyState(t *testing.T) {
	t.Parallel()

	b := ssm.NewInMemoryBackend()

	b.AddInstancePatchStateInternal(ssm.InstancePatchState{InstanceID: "i-1", PatchGroup: "pg-1"})
	b.AddInstancePropertyInternal(ssm.InstanceProperty{InstanceID: "i-1", PlatformType: "Linux"})
	b.AddInstancePatchesInternal("i-1", []ssm.PatchComplianceData{{Classification: "Security", State: "Missing"}})
	b.AddAvailablePatchInternal(ssm.Patch{Name: "KB1234567", Product: "WindowsServer2022"})

	assert.Equal(t, 1, b.InstancePatchStateCount(), "setup: instance patch state not stored")
	assert.Equal(t, 1, b.InstancePropertyCount(), "setup: instance property not stored")
	assert.Equal(t, 1, b.InstancePatchesCount(), "setup: instance patches not stored")
	assert.Equal(t, 1, b.AvailablePatchesCount(), "setup: available patch not stored")

	b.Reset()

	assert.Zero(t, b.InstancePatchStateCount(), "Reset must clear instancePatchStates")
	assert.Zero(t, b.InstancePropertyCount(), "Reset must clear instanceProperties")
	assert.Zero(t, b.InstancePatchesCount(), "Reset must clear instancePatches")
	assert.Zero(t, b.AvailablePatchesCount(), "Reset must clear availablePatches")
}

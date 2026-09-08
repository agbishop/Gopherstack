package eks

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
)

// TestApplyVpcEndpointUpdate_BackendError_ReturnsRawError is a white-box
// companion to the black-box handler_updates.go tests. Both
// UpdateClusterConfig and UpdateClusterVpcEndpoint perform the identical
// cluster-existence lookup on the same clusterName, so within a single
// synchronous handleUpdateClusterConfig call the second cannot fail once the
// first has already succeeded -- reaching applyVpcEndpointUpdate's error
// branch requires a concurrent DeleteCluster landing between the two backend
// calls (a real race in a live server, since each call takes and releases
// the backend lock independently). This test reproduces that race
// deterministically by performing the same two backend calls the handler
// would, with a DeleteCluster inserted between them.
func TestApplyVpcEndpointUpdate_BackendError_ReturnsRawError(t *testing.T) {
	t.Parallel()

	b := NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)
	h := NewHandler(b)

	_, err := b.CreateCluster("race-cluster", "1.32", "", nil, nil, nil)
	require.NoError(t, err)

	update, err := b.UpdateClusterConfig("race-cluster", ClusterConfigUpdate{})
	require.NoError(t, err)

	_, err = b.DeleteCluster("race-cluster")
	require.NoError(t, err)

	pub := true
	vpcIn := &updateClusterConfigVpcConfig{EndpointPublicAccess: &pub}

	vpcErr := h.applyVpcEndpointUpdate("race-cluster", vpcIn, update)

	require.Error(t, vpcErr, "must return the raw backend error instead of swallowing it as nil")
	assert.ErrorIs(t, vpcErr, ErrNotFound)
}

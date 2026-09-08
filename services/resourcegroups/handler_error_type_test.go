package resourcegroups_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	resourcegroupssdk "github.com/aws/aws-sdk-go-v2/service/resourcegroups"
	smithy "github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/resourcegroups"
)

// TestGetGroup_UnknownGroupSurfacesNotFoundException drives GetGroup for a
// group that doesn't exist through a real SDK client. Before this fix,
// handleError never set X-Amzn-Errortype nor a body code/__type field, so
// restjson.GetErrorInfo had nothing to read and every error -- including
// this one -- deserialized client-side as a generic UnknownError instead of
// the modeled NotFoundException (gopherstack-ifni; same bug class fixed for
// mediatailor in f41d5b42f).
func TestGetGroup_UnknownGroupSurfacesNotFoundException(t *testing.T) {
	t.Parallel()

	backend := resourcegroups.NewInMemoryBackend("000000000000", "us-east-1")
	client := newTestResourceGroupsClient(t, resourcegroups.NewHandler(backend))

	_, err := client.GetGroup(t.Context(), &resourcegroupssdk.GetGroupInput{
		Group: aws.String("no-such-group"),
	})
	require.Error(t, err)

	var apiErr smithy.APIError
	require.ErrorAs(t, err, &apiErr, "SDK must surface a typed API error, not an opaque one")
	assert.Equal(t, "NotFoundException", apiErr.ErrorCode())
}

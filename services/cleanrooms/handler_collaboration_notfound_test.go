package cleanrooms_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	cleanroomssdk "github.com/aws/aws-sdk-go-v2/service/cleanrooms"
	smithy "github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cleanrooms"
)

// TestHandler_GetUpdateCollaboration_NonexistentIsResourceNotFound drives a
// real Clean Rooms client's GetCollaboration/UpdateCollaboration against a
// collaboration ID that was never created. cleanrooms@v1.49.4's
// awsRestjson1_deserializeOpErrorGetCollaboration/-UpdateCollaboration
// switches don't type ResourceNotFoundException at all (only
// AccessDeniedException, InternalServerException, ThrottlingException,
// ValidationException -- same trio-wide omission already documented and
// accepted for DeleteCollaboration, see
// handler_delete_collaboration_idempotent_test.go), so a real client sees an
// untyped smithy.GenericAPIError rather than a modeled exception. Unlike
// Delete, Get/Update can't fabricate a real resource for a missing ID, so
// idempotency isn't an option here; ResourceNotFoundException is still the
// only wire-legal, non-misleading code available (AccessDeniedException
// would misreport an authz failure, ValidationException a malformed
// request) and restjson1 servers aren't restricted to only the operation's
// modeled error set -- the client falls back to a generic error but still
// reads the literal code from the header/body (handler.go's handleError
// comment: the Terraform delete-waiter polls GetCollaboration and must
// recognize ResourceNotFoundException by that string). This test pins that
// the emitted code stays "ResourceNotFoundException" for both ops.
func TestHandler_GetUpdateCollaboration_NonexistentIsResourceNotFound(t *testing.T) {
	t.Parallel()

	h := cleanrooms.NewHandler(cleanrooms.NewInMemoryBackend("000000000000", "us-east-1"))
	client := newTestCleanRoomsClient(t, h)
	missingID := "00000000-0000-0000-0000-000000000000"

	t.Run("get", func(t *testing.T) {
		t.Parallel()

		_, err := client.GetCollaboration(t.Context(), &cleanroomssdk.GetCollaborationInput{
			CollaborationIdentifier: aws.String(missingID),
		})
		require.Error(t, err)

		var apiErr smithy.APIError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, "ResourceNotFoundException", apiErr.ErrorCode())
	})

	t.Run("update", func(t *testing.T) {
		t.Parallel()

		_, err := client.UpdateCollaboration(t.Context(), &cleanroomssdk.UpdateCollaborationInput{
			CollaborationIdentifier: aws.String(missingID),
			Description:             aws.String("new description"),
		})
		require.Error(t, err)

		var apiErr smithy.APIError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, "ResourceNotFoundException", apiErr.ErrorCode())
	})
}

package iot_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	iotsdk "github.com/aws/aws-sdk-go-v2/service/iot"
	"github.com/aws/aws-sdk-go-v2/service/iot/types"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/iot"
)

// TestUndeclaredErrorCodes_MatchTheirOwnRealSDKType is a regression lock for
// gopherstack-y3om: DeleteCommand, DeleteCommandExecution, DeletePackage and
// DeletePackageVersion each emit ResourceNotFoundException for a missing
// resource, a code none of their own operation models (botocore
// iot/2015-05-28/service-2.json "errors" lists, confirmed identical to the
// Go SDK's declared sets) declare. That would matter for a service still on
// the classic per-operation deserializeOpError switch (verified for
// accessanalyzer@v1.51.4: services/accessanalyzer/error_shapes_test.go
// asserts exactly that fallback). It does NOT matter here: iot's pinned SDK
// (v1.83.0) has no deserializers.go at all -- confirmed by directory listing
// -- and ships a type_registry.go/schemas package instead (smithy-go
// schema-based codegen). Error deserialization resolves the wire error code
// against the whole service's TypeRegistry, with no reference to the
// calling operation's schema, so each of these four still deserializes into
// its own specific exception type and DOES satisfy errors.As -- and does
// NOT coerce into any of the op's actually-declared types. This locks that
// behavior so it isn't silently changed, and documents why the client-
// breakage premise the issue was filed under does not apply to this
// service's current SDK: the divergence is a documentation gap, not a
// client-breaking defect.
func TestUndeclaredErrorCodes_MatchTheirOwnRealSDKType(t *testing.T) {
	t.Parallel()

	h := iot.NewHandler(iot.NewInMemoryBackend(), nil)
	client := newTestIoTClient(t, h)

	t.Run("DeleteCommand unknown id", func(t *testing.T) {
		t.Parallel()

		_, err := client.DeleteCommand(t.Context(), &iotsdk.DeleteCommandInput{
			CommandId: aws.String("no-such-command"),
		})
		require.Error(t, err)

		var rnf *types.ResourceNotFoundException
		require.ErrorAs(t, err, &rnf, "ResourceNotFoundException is what gopherstack emits today")

		var ce *types.ConflictException
		require.NotErrorAs(t, err, &ce, "declared for this op, but not what is actually emitted")
	})

	t.Run("DeleteCommandExecution unknown id", func(t *testing.T) {
		t.Parallel()

		_, err := client.DeleteCommandExecution(t.Context(), &iotsdk.DeleteCommandExecutionInput{
			ExecutionId: aws.String("no-such-exec"),
			TargetArn:   aws.String("arn:aws:iot:us-east-1:123456789012:thing/my-thing"),
		})
		require.Error(t, err)

		var rnf *types.ResourceNotFoundException
		require.ErrorAs(t, err, &rnf, "ResourceNotFoundException is what gopherstack emits today")

		var ce *types.ConflictException
		require.NotErrorAs(t, err, &ce, "declared for this op, but not what is actually emitted")
	})

	t.Run("DeletePackage unknown name", func(t *testing.T) {
		t.Parallel()

		_, err := client.DeletePackage(t.Context(), &iotsdk.DeletePackageInput{
			PackageName: aws.String("no-such-pkg"),
		})
		require.Error(t, err)

		var rnf *types.ResourceNotFoundException
		require.ErrorAs(t, err, &rnf, "ResourceNotFoundException is what gopherstack emits today")

		var ve *types.ValidationException
		require.NotErrorAs(t, err, &ve, "declared for this op, but not what is actually emitted")
	})

	t.Run("DeletePackageVersion unknown name", func(t *testing.T) {
		t.Parallel()

		_, err := client.DeletePackageVersion(t.Context(), &iotsdk.DeletePackageVersionInput{
			PackageName: aws.String("no-such-pkg"),
			VersionName: aws.String("v1"),
		})
		require.Error(t, err)

		var rnf *types.ResourceNotFoundException
		require.ErrorAs(t, err, &rnf, "ResourceNotFoundException is what gopherstack emits today")

		var ve *types.ValidationException
		require.NotErrorAs(t, err, &ve, "declared for this op, but not what is actually emitted")
	})
}

// TestUndeclaredErrorCodes_NoTypeRegistryEntry_WouldFailToDeserialize
// documents the failure mode the above test would hit if
// ResourceNotFoundException had no smithy TypeRegistry entry: this asserts
// the entry actually exists in the pinned SDK, since the four subtests
// above only prove errors.As succeeds today and would not explain why if
// the SDK were ever repinned to a version that dropped the entry.
func TestUndeclaredErrorCodes_NoTypeRegistryEntry_WouldFailToDeserialize(t *testing.T) {
	t.Parallel()

	entry, ok := iotsdk.TypeRegistry.Entries["com.amazonaws.iot#ResourceNotFoundException"]
	require.True(t, ok, "iot's TypeRegistry must carry a ResourceNotFoundException entry for errors.As to match it")
	require.NotNil(t, entry.New)

	v, ok := entry.New().(*types.ResourceNotFoundException)
	require.True(t, ok)
	require.IsType(t, &types.ResourceNotFoundException{}, v)
}

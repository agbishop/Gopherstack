package codepipeline_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	cpsdk "github.com/aws/aws-sdk-go-v2/service/codepipeline"
	"github.com/aws/aws-sdk-go-v2/service/codepipeline/types"
	"github.com/stretchr/testify/require"
)

// sdkPipeline builds a minimal real types.PipelineDeclaration for the given
// name, matching samplePipeline's shape but in SDK wire types.
func sdkPipeline(name string) *types.PipelineDeclaration {
	return &types.PipelineDeclaration{
		Name:    aws.String(name),
		RoleArn: aws.String("arn:aws:iam::000000000000:role/pipeline-role"),
		ArtifactStore: &types.ArtifactStore{
			Type:     types.ArtifactStoreTypeS3,
			Location: aws.String("my-artifact-bucket"),
		},
		Stages: []types.StageDeclaration{
			{
				Name: aws.String("Source"),
				Actions: []types.ActionDeclaration{
					{
						Name: aws.String("SourceAction"),
						ActionTypeId: &types.ActionTypeId{
							Category: types.ActionCategorySource,
							Owner:    types.ActionOwnerThirdParty,
							Provider: aws.String("GitHub"),
							Version:  aws.String("1"),
						},
					},
				},
			},
		},
	}
}

// TestUndeclaredErrorCodes_MatchTheirOwnRealSDKType is a regression lock
// for gopherstack-wlab: seven codepipeline ops each emit a wire error code
// their own operation model (botocore codepipeline/2015-07-09/service-2.json
// "errors" list, confirmed identical to the Go SDK's declared set) does not
// declare. Earlier passes on this issue (gopherstack-3djp) assumed that
// meant a real client's errors.As(err, &types.XException{}) could never
// match the specific type -- true for services still on the classic
// deserializeOpError<Op> switch codegen (verified for accessanalyzer@v1.51.4,
// which still has it: services/accessanalyzer/error_shapes_test.go asserts
// exactly that fallback). It is FALSE here: codepipeline's pinned SDK
// (v1.54.0) switched to schema-based (de)serialization in v1.50.0 ("Enable
// schema-based (de)serialization for this service", CHANGELOG.md) and has
// no deserializeOpError functions at all -- confirmed by grep against the
// pinned module. Error deserialization
// (smithy-go@v1.28.1 transport/http/protocol/awsjson/awsjson.go:191
// deserializeError) resolves the wire error code against the whole
// service's TypeRegistry, with no reference to the calling operation's
// schema. So each of these seven undeclared codes still deserializes into
// its own specific exception type and DOES satisfy errors.As -- and does
// NOT coerce into any of the op's actually-declared types. This locks that
// behavior so it isn't silently changed, and documents why the
// consequence cited when this issue was filed does not apply to this
// service's current SDK.
func TestUndeclaredErrorCodes_MatchTheirOwnRealSDKType(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	client := newTestCodePipelineClient(t, h)

	t.Run("CreateCustomActionType duplicate", func(t *testing.T) {
		t.Parallel()

		in := &cpsdk.CreateCustomActionTypeInput{
			Category:              types.ActionCategoryBuild,
			Provider:              aws.String("dup-provider"),
			Version:               aws.String("1"),
			InputArtifactDetails:  &types.ArtifactDetails{MinimumCount: 0, MaximumCount: 5},
			OutputArtifactDetails: &types.ArtifactDetails{MinimumCount: 0, MaximumCount: 5},
		}
		_, err := client.CreateCustomActionType(t.Context(), in)
		require.NoError(t, err)

		_, err = client.CreateCustomActionType(t.Context(), in)
		require.Error(t, err)

		var ise *types.InvalidStructureException
		require.ErrorAs(t, err, &ise, "InvalidStructureException is what gopherstack emits today")

		var cme *types.ConcurrentModificationException
		require.NotErrorAs(t, err, &cme, "declared for this op, but not what is actually emitted")
	})

	t.Run("DeleteCustomActionType not found", func(t *testing.T) {
		t.Parallel()

		_, err := client.DeleteCustomActionType(t.Context(), &cpsdk.DeleteCustomActionTypeInput{
			Category: types.ActionCategoryBuild,
			Provider: aws.String("no-such-provider"),
			Version:  aws.String("1"),
		})
		require.Error(t, err)

		var atnf *types.ActionTypeNotFoundException
		require.ErrorAs(t, err, &atnf, "ActionTypeNotFoundException is what gopherstack emits today")

		var cme *types.ConcurrentModificationException
		require.NotErrorAs(t, err, &cme, "declared for this op, but not what is actually emitted")
	})

	t.Run("DeletePipeline not found", func(t *testing.T) {
		t.Parallel()

		_, err := client.DeletePipeline(t.Context(), &cpsdk.DeletePipelineInput{
			Name: aws.String("no-such-pipeline"),
		})
		require.Error(t, err)

		var pnf *types.PipelineNotFoundException
		require.ErrorAs(t, err, &pnf, "PipelineNotFoundException is what gopherstack emits today")

		var cme *types.ConcurrentModificationException
		require.NotErrorAs(t, err, &cme, "declared for this op, but not what is actually emitted")
	})

	t.Run("UpdatePipeline not found", func(t *testing.T) {
		t.Parallel()

		_, err := client.UpdatePipeline(t.Context(), &cpsdk.UpdatePipelineInput{
			Pipeline: sdkPipeline("no-such-pipeline-to-update"),
		})
		require.Error(t, err)

		var pnf *types.PipelineNotFoundException
		require.ErrorAs(t, err, &pnf, "PipelineNotFoundException is what gopherstack emits today")

		var ise *types.InvalidStructureException
		require.NotErrorAs(t, err, &ise, "declared for this op, but not what is actually emitted")
	})

	t.Run("StopPipelineExecution unknown executionId", func(t *testing.T) {
		t.Parallel()

		_, err := h.Backend.CreatePipeline(t.Context(), samplePipeline("stop-exec-pipeline"), nil)
		require.NoError(t, err)

		_, err = client.StopPipelineExecution(t.Context(), &cpsdk.StopPipelineExecutionInput{
			PipelineName:        aws.String("stop-exec-pipeline"),
			PipelineExecutionId: aws.String("no-such-execution"),
		})
		require.Error(t, err)

		var penf *types.PipelineExecutionNotFoundException
		require.ErrorAs(t, err, &penf, "PipelineExecutionNotFoundException is what gopherstack emits today")

		var pnf *types.PipelineNotFoundException
		require.NotErrorAs(t, err, &pnf, "declared for this op, but not what is actually emitted")
	})

	t.Run("RetryStageExecution unknown executionId", func(t *testing.T) {
		t.Parallel()

		_, err := h.Backend.CreatePipeline(t.Context(), samplePipeline("retry-exec-pipeline"), nil)
		require.NoError(t, err)

		_, err = client.RetryStageExecution(t.Context(), &cpsdk.RetryStageExecutionInput{
			PipelineName:        aws.String("retry-exec-pipeline"),
			PipelineExecutionId: aws.String("no-such-execution"),
			StageName:           aws.String("Source"),
			RetryMode:           types.StageRetryModeFailedActions,
		})
		require.Error(t, err)

		var penf *types.PipelineExecutionNotFoundException
		require.ErrorAs(t, err, &penf, "PipelineExecutionNotFoundException is what gopherstack emits today")

		var pnf *types.PipelineNotFoundException
		require.NotErrorAs(t, err, &pnf, "declared for this op, but not what is actually emitted")
	})

	t.Run("OverrideStageCondition unknown executionId", func(t *testing.T) {
		t.Parallel()

		_, err := h.Backend.CreatePipeline(t.Context(), samplePipeline("override-exec-pipeline"), nil)
		require.NoError(t, err)

		_, err = client.OverrideStageCondition(t.Context(), &cpsdk.OverrideStageConditionInput{
			PipelineName:        aws.String("override-exec-pipeline"),
			PipelineExecutionId: aws.String("no-such-execution"),
			StageName:           aws.String("Source"),
			ConditionType:       types.ConditionTypeBeforeEntry,
		})
		require.Error(t, err)

		var penf *types.PipelineExecutionNotFoundException
		require.ErrorAs(t, err, &penf, "PipelineExecutionNotFoundException is what gopherstack emits today")

		var pnf *types.PipelineNotFoundException
		require.NotErrorAs(t, err, &pnf, "declared for this op, but not what is actually emitted")
	})
}

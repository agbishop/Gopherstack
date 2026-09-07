package stepfunctions_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	sfnsdk "github.com/aws/aws-sdk-go-v2/service/sfn"
	sfntypes "github.com/aws/aws-sdk-go-v2/service/sfn/types"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/stepfunctions"
)

// TestCreateStateMachineAlias_InvalidRoutingConfig_RealClient drives
// CreateStateMachineAlias with routing weights that don't sum to 100
// through the real aws-sdk-go-v2 sfn client and asserts errors.As unwraps
// to *types.ValidationException. CreateStateMachineAlias's own
// deserializeOpError switch (sfn@v1.45.4 deserializers.go) declares
// [ConflictException, InvalidArn, InvalidName, ResourceNotFound,
// ServiceQuotaExceededException, StateMachineDeleting, ValidationException]
// -- no InvalidRoutingConfiguration exception type exists anywhere in this
// module. AWS instead models this exact condition as a *reason* on
// ValidationException: types.ValidationExceptionReasonInvalidRoutingConfiguration
// = "INVALID_ROUTING_CONFIGURATION" (sfn@v1.45.4 types/enums.go:491).
func TestCreateStateMachineAlias_InvalidRoutingConfig_RealClient(t *testing.T) {
	t.Parallel()

	backend := stepfunctions.NewInMemoryBackend()
	h := stepfunctions.NewHandler(backend)
	client := newSFNSDKClient(t, h)
	ctx := t.Context()

	smName := "test-sm-" + uuid.NewString()[:8]
	createSM, err := client.CreateStateMachine(ctx, &sfnsdk.CreateStateMachineInput{
		Name:       aws.String(smName),
		Definition: aws.String(validPassDef),
		RoleArn:    aws.String(validRoleARN),
		Type:       sfntypes.StateMachineTypeStandard,
	})
	require.NoError(t, err)
	smArn := *createSM.StateMachineArn

	pub, err := client.PublishStateMachineVersion(ctx, &sfnsdk.PublishStateMachineVersionInput{
		StateMachineArn: aws.String(smArn),
	})
	require.NoError(t, err)

	_, err = client.CreateStateMachineAlias(ctx, &sfnsdk.CreateStateMachineAliasInput{
		Name: aws.String("bad-alias"),
		RoutingConfiguration: []sfntypes.RoutingConfigurationListItem{
			{StateMachineVersionArn: pub.StateMachineVersionArn, Weight: 50},
		},
	}, func(o *sfnsdk.Options) { o.RetryMaxAttempts = 1 })
	require.Error(t, err)

	var validationErr *sfntypes.ValidationException
	require.ErrorAs(t, err, &validationErr,
		"expected a real ValidationException from the SDK deserializer, got: %v", err)
}

// TestUpdateStateMachineAlias_InvalidRoutingConfig_RealClient is the
// UpdateStateMachineAlias sibling: it shares the same
// stepfunctions.ErrInvalidRoutingConfiguration sentinel and the same
// handler.go wire mapping. UpdateStateMachineAlias's own deserializeOpError
// switch also declares ValidationException, not InvalidRoutingConfiguration.
func TestUpdateStateMachineAlias_InvalidRoutingConfig_RealClient(t *testing.T) {
	t.Parallel()

	backend := stepfunctions.NewInMemoryBackend()
	h := stepfunctions.NewHandler(backend)
	client := newSFNSDKClient(t, h)
	ctx := t.Context()

	smName := "test-sm-" + uuid.NewString()[:8]
	createSM, err := client.CreateStateMachine(ctx, &sfnsdk.CreateStateMachineInput{
		Name:       aws.String(smName),
		Definition: aws.String(validPassDef),
		RoleArn:    aws.String(validRoleARN),
		Type:       sfntypes.StateMachineTypeStandard,
	})
	require.NoError(t, err)
	smArn := *createSM.StateMachineArn

	pub, err := client.PublishStateMachineVersion(ctx, &sfnsdk.PublishStateMachineVersionInput{
		StateMachineArn: aws.String(smArn),
	})
	require.NoError(t, err)

	// CreateStateMachineAliasInput has no stateMachineArn field on the real
	// wire, so this backend's CreateStateMachineAlias 404s through the real
	// client today (pre-existing, unrelated gap -- see
	// Test_SDKRoundTrip_StateMachineAlias_UpdateDate's comment). Set the
	// alias up directly against the backend so this test can isolate the
	// UpdateStateMachineAlias error-code bug this issue is about.
	alias, err := backend.CreateStateMachineAlias(smArn, "live", "", []stepfunctions.AliasRoutingConfig{
		{StateMachineVersionArn: *pub.StateMachineVersionArn, Weight: 100},
	})
	require.NoError(t, err)

	_, err = client.UpdateStateMachineAlias(ctx, &sfnsdk.UpdateStateMachineAliasInput{
		StateMachineAliasArn: aws.String(alias.StateMachineAliasArn),
		RoutingConfiguration: []sfntypes.RoutingConfigurationListItem{
			{StateMachineVersionArn: pub.StateMachineVersionArn, Weight: 50},
		},
	}, func(o *sfnsdk.Options) { o.RetryMaxAttempts = 1 })
	require.Error(t, err)

	var validationErr *sfntypes.ValidationException
	require.ErrorAs(t, err, &validationErr,
		"expected a real ValidationException from the SDK deserializer, got: %v", err)
}

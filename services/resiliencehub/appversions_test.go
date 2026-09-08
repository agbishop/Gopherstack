package resiliencehub_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	resiliencehubsdk "github.com/aws/aws-sdk-go-v2/service/resiliencehub"
	"github.com/aws/aws-sdk-go-v2/service/resiliencehub/types"
	"github.com/stretchr/testify/require"
)

// TestCreateAppVersionAppComponent_DuplicateNameConflicts verifies creating
// two components with the same name is rejected.
func TestCreateAppVersionAppComponent_DuplicateNameConflicts(t *testing.T) {
	t.Parallel()

	_, client := newTestHandlerAndClient(t)
	ctx := t.Context()

	appOut, err := client.CreateApp(ctx, &resiliencehubsdk.CreateAppInput{Name: aws.String("app")})
	require.NoError(t, err)

	_, err = client.CreateAppVersionAppComponent(ctx, &resiliencehubsdk.CreateAppVersionAppComponentInput{
		AppArn: appOut.App.AppArn, Name: aws.String("comp"), Type: aws.String("t"),
	})
	require.NoError(t, err)

	_, err = client.CreateAppVersionAppComponent(ctx, &resiliencehubsdk.CreateAppVersionAppComponentInput{
		AppArn: appOut.App.AppArn, Name: aws.String("comp"), Type: aws.String("t"),
	})
	require.Error(t, err)

	var conflict *types.ConflictException
	require.ErrorAs(t, err, &conflict)
}

// TestUpdateAppVersionAppComponent_DuplicateNameConflicts verifies renaming a
// component to a name already used by another component on the same version
// is rejected -- the same rule CreateAppVersionAppComponent already enforces.
func TestUpdateAppVersionAppComponent_DuplicateNameConflicts(t *testing.T) {
	t.Parallel()

	_, client := newTestHandlerAndClient(t)
	ctx := t.Context()

	appOut, err := client.CreateApp(ctx, &resiliencehubsdk.CreateAppInput{Name: aws.String("app")})
	require.NoError(t, err)

	_, err = client.CreateAppVersionAppComponent(ctx, &resiliencehubsdk.CreateAppVersionAppComponentInput{
		AppArn: appOut.App.AppArn, Name: aws.String("comp1"), Type: aws.String("t"),
	})
	require.NoError(t, err)

	comp2Out, err := client.CreateAppVersionAppComponent(ctx, &resiliencehubsdk.CreateAppVersionAppComponentInput{
		AppArn: appOut.App.AppArn, Name: aws.String("comp2"), Type: aws.String("t"),
	})
	require.NoError(t, err)

	_, err = client.UpdateAppVersionAppComponent(ctx, &resiliencehubsdk.UpdateAppVersionAppComponentInput{
		AppArn: appOut.App.AppArn, Id: comp2Out.AppComponent.Id, Name: aws.String("comp1"),
	})
	require.Error(t, err)

	var conflict *types.ConflictException
	require.ErrorAs(t, err, &conflict)
}

// TestUpdateAppVersionAppComponent_RenamePreservesResourceAssociation
// verifies that renaming a component keeps its already-assigned resources
// associated with it. This backend tracks a resource's component assignment
// by name (wire_convert.go), so a rename must repoint every resource that
// named the old value -- otherwise the resources would silently fall off the
// component and DeleteAppVersionAppComponent's "still has resources
// associated" conflict check (appversions.go) would go blind to them.
func TestUpdateAppVersionAppComponent_RenamePreservesResourceAssociation(t *testing.T) {
	t.Parallel()

	_, client := newTestHandlerAndClient(t)
	ctx := t.Context()

	appOut, err := client.CreateApp(ctx, &resiliencehubsdk.CreateAppInput{Name: aws.String("app")})
	require.NoError(t, err)

	compOut, err := client.CreateAppVersionAppComponent(ctx, &resiliencehubsdk.CreateAppVersionAppComponentInput{
		AppArn: appOut.App.AppArn, Name: aws.String("comp"), Type: aws.String("t"),
	})
	require.NoError(t, err)

	_, err = client.CreateAppVersionResource(ctx, &resiliencehubsdk.CreateAppVersionResourceInput{
		AppArn: appOut.App.AppArn, AppComponents: []string{"comp"},
		PhysicalResourceId: aws.String("arn:aws:sns:us-east-1:000000000000:topic:t"),
		ResourceType:       aws.String("AWS::SNS::Topic"),
		LogicalResourceId:  &types.LogicalResourceId{Identifier: aws.String("res")},
	})
	require.NoError(t, err)

	_, err = client.UpdateAppVersionAppComponent(ctx, &resiliencehubsdk.UpdateAppVersionAppComponentInput{
		AppArn: appOut.App.AppArn, Id: compOut.AppComponent.Id, Name: aws.String("renamed"),
	})
	require.NoError(t, err)

	_, err = client.DeleteAppVersionAppComponent(ctx, &resiliencehubsdk.DeleteAppVersionAppComponentInput{
		AppArn: appOut.App.AppArn, Id: compOut.AppComponent.Id,
	})
	require.Error(t, err, "renamed component should still be blocked by its resource association")

	var conflict *types.ConflictException
	require.ErrorAs(t, err, &conflict)
}

// TestDeleteAppVersionAppComponent_RejectsWhenResourcesAssociated verifies
// the SDK's own documented rule: a component with associated resources
// cannot be deleted.
func TestDeleteAppVersionAppComponent_RejectsWhenResourcesAssociated(t *testing.T) {
	t.Parallel()

	_, client := newTestHandlerAndClient(t)
	ctx := t.Context()

	appOut, err := client.CreateApp(ctx, &resiliencehubsdk.CreateAppInput{Name: aws.String("app")})
	require.NoError(t, err)

	compOut, err := client.CreateAppVersionAppComponent(ctx, &resiliencehubsdk.CreateAppVersionAppComponentInput{
		AppArn: appOut.App.AppArn, Name: aws.String("comp"), Type: aws.String("t"),
	})
	require.NoError(t, err)

	_, err = client.CreateAppVersionResource(ctx, &resiliencehubsdk.CreateAppVersionResourceInput{
		AppArn: appOut.App.AppArn, AppComponents: []string{"comp"},
		PhysicalResourceId: aws.String("arn:aws:sns:us-east-1:000000000000:topic:t"),
		ResourceType:       aws.String("AWS::SNS::Topic"),
		LogicalResourceId:  &types.LogicalResourceId{Identifier: aws.String("res")},
	})
	require.NoError(t, err)

	_, err = client.DeleteAppVersionAppComponent(ctx, &resiliencehubsdk.DeleteAppVersionAppComponentInput{
		AppArn: appOut.App.AppArn, Id: compOut.AppComponent.Id,
	})
	require.Error(t, err)

	var conflict *types.ConflictException
	require.ErrorAs(t, err, &conflict)
}

// TestDeleteAppVersionResource_RejectsNonManuallyAddedResource verifies the
// SDK's own documented rule: only manually-added (AppTemplate-sourced)
// resources may be deleted -- a resource discovered via
// ResolveAppVersionResources cannot be.
func TestDeleteAppVersionResource_RejectsNonManuallyAddedResource(t *testing.T) {
	t.Parallel()

	_, client := newTestHandlerAndClient(t)
	ctx := t.Context()

	appOut, err := client.CreateApp(ctx, &resiliencehubsdk.CreateAppInput{Name: aws.String("app")})
	require.NoError(t, err)

	_, err = client.AddDraftAppVersionResourceMappings(ctx, &resiliencehubsdk.AddDraftAppVersionResourceMappingsInput{
		AppArn: appOut.App.AppArn,
		ResourceMappings: []types.ResourceMapping{{
			MappingType:  types.ResourceMappingTypeResource,
			ResourceName: aws.String("discovered-res"),
			PhysicalResourceId: &types.PhysicalResourceId{
				Identifier: aws.String(
					"arn:aws:sns:us-east-1:000000000000:topic:t",
				), Type: types.PhysicalIdentifierTypeArn,
			},
		}},
	})
	require.NoError(t, err)

	resolveOut, err := client.ResolveAppVersionResources(ctx, &resiliencehubsdk.ResolveAppVersionResourcesInput{
		AppArn: appOut.App.AppArn, AppVersion: aws.String("draft"),
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		status, statusErr := client.DescribeAppVersionResourcesResolutionStatus(
			ctx, &resiliencehubsdk.DescribeAppVersionResourcesResolutionStatusInput{
				AppArn: appOut.App.AppArn, AppVersion: aws.String("draft"), ResolutionId: resolveOut.ResolutionId,
			},
		)
		require.NoError(t, statusErr)

		return status.Status == types.ResourceResolutionStatusTypeSuccess
	}, defaultAsyncWait, defaultAsyncPoll)

	_, err = client.DeleteAppVersionResource(ctx, &resiliencehubsdk.DeleteAppVersionResourceInput{
		AppArn: appOut.App.AppArn, ResourceName: aws.String("discovered-res"),
	})
	require.Error(t, err)

	var conflict *types.ConflictException
	require.ErrorAs(t, err, &conflict)
}

// TestListUnsupportedAppVersionResources_ClassifiesRealSupportedTypes
// verifies the real, derivable classification against the two closed lists
// documented on types.PhysicalResourceId.Type -- not a fabricated
// classification.
func TestListUnsupportedAppVersionResources_ClassifiesRealSupportedTypes(t *testing.T) {
	t.Parallel()

	_, client := newTestHandlerAndClient(t)
	ctx := t.Context()

	appOut, err := client.CreateApp(ctx, &resiliencehubsdk.CreateAppInput{Name: aws.String("app")})
	require.NoError(t, err)

	_, err = client.CreateAppVersionResource(ctx, &resiliencehubsdk.CreateAppVersionResourceInput{
		AppArn: appOut.App.AppArn, AppComponents: []string{},
		PhysicalResourceId: aws.String("arn:aws:sns:us-east-1:000000000000:topic:supported"),
		ResourceType:       aws.String("AWS::SNS::Topic"), // in the supported Arn list
		LogicalResourceId:  &types.LogicalResourceId{Identifier: aws.String("r1")},
	})
	require.NoError(t, err)

	_, err = client.CreateAppVersionResource(ctx, &resiliencehubsdk.CreateAppVersionResourceInput{
		AppArn: appOut.App.AppArn, AppComponents: []string{},
		PhysicalResourceId: aws.String("some-native-id"),
		ResourceType:       aws.String("AWS::Made::Up"), // not in either supported list
		LogicalResourceId:  &types.LogicalResourceId{Identifier: aws.String("r2")},
	})
	require.NoError(t, err)

	unsupported, err := client.ListUnsupportedAppVersionResources(
		ctx,
		&resiliencehubsdk.ListUnsupportedAppVersionResourcesInput{
			AppArn:     appOut.App.AppArn,
			AppVersion: aws.String("draft"),
		},
	)
	require.NoError(t, err)
	require.Len(t, unsupported.UnsupportedResources, 1)
	require.Equal(t, "AWS::Made::Up", aws.ToString(unsupported.UnsupportedResources[0].ResourceType))
}

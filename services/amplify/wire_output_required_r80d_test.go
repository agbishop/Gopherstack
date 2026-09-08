package amplify_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	amplifysdk "github.com/aws/aws-sdk-go-v2/service/amplify"
	amplifytypes "github.com/aws/aws-sdk-go-v2/service/amplify/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/amplify"
)

// TestCreateApp_RequiredOutputFieldsPresentWhenEmpty proves App's required
// (real SDK types.App) EnvironmentVariables/Description/Repository survive
// a real client round trip when the caller supplies none of them --
// previously all three were tagged omitempty/omitzero on a *pointer/map*
// real member, so an empty value dropped the wire key entirely and a real
// client's typed field decoded as nil instead of a present zero value.
func TestCreateApp_RequiredOutputFieldsPresentWhenEmpty(t *testing.T) {
	t.Parallel()

	backend := amplify.NewInMemoryBackend("000000000000", "us-east-1")
	client := newTestAmplifyClient(t, amplify.NewHandler(backend))
	ctx := t.Context()

	created, err := client.CreateApp(ctx, &amplifysdk.CreateAppInput{Name: aws.String("app-empty-fields")})
	require.NoError(t, err)

	require.NotNil(t, created.App.Description, "Description is required on App")
	assert.Empty(t, aws.ToString(created.App.Description))
	require.NotNil(t, created.App.Repository, "Repository is required on App")
	assert.Empty(t, aws.ToString(created.App.Repository))
	assert.NotNil(t, created.App.EnvironmentVariables, "EnvironmentVariables is required on App")
	assert.Empty(t, created.App.EnvironmentVariables)

	got, err := client.GetApp(ctx, &amplifysdk.GetAppInput{AppId: created.App.AppId})
	require.NoError(t, err)
	require.NotNil(t, got.App.Description)
	require.NotNil(t, got.App.Repository)
	assert.NotNil(t, got.App.EnvironmentVariables)
}

// TestCreateBranch_RequiredOutputFieldsPresentWhenEmpty proves Branch's
// required (types.Branch) ActiveJobId/CustomDomains/Description/Framework/
// EnvironmentVariables survive a real client round trip for a freshly
// created branch with none of them set. ActiveJobId in particular is a
// required *string on the real SDK: previously the key vanished entirely
// for a branch with no jobs yet (a fully reachable, unexceptional state),
// so a real client saw a nil pointer instead of a pointer to "".
func TestCreateBranch_RequiredOutputFieldsPresentWhenEmpty(t *testing.T) {
	t.Parallel()

	backend := amplify.NewInMemoryBackend("000000000000", "us-east-1")
	client := newTestAmplifyClient(t, amplify.NewHandler(backend))
	ctx := t.Context()

	app, err := client.CreateApp(ctx, &amplifysdk.CreateAppInput{Name: aws.String("app-branch-empty")})
	require.NoError(t, err)

	created, err := client.CreateBranch(ctx, &amplifysdk.CreateBranchInput{
		AppId:      app.App.AppId,
		BranchName: aws.String("main"),
	})
	require.NoError(t, err)

	b := created.Branch
	require.NotNil(t, b.ActiveJobId, "ActiveJobId is required on Branch")
	assert.Empty(t, aws.ToString(b.ActiveJobId))
	require.NotNil(t, b.Description, "Description is required on Branch")
	require.NotNil(t, b.Framework, "Framework is required on Branch")
	assert.NotNil(t, b.CustomDomains, "CustomDomains is required on Branch")
	assert.Empty(t, b.CustomDomains)
	assert.NotNil(t, b.EnvironmentVariables, "EnvironmentVariables is required on Branch")
	assert.Empty(t, b.EnvironmentVariables)

	got, err := client.GetBranch(ctx, &amplifysdk.GetBranchInput{
		AppId:      app.App.AppId,
		BranchName: aws.String("main"),
	})
	require.NoError(t, err)
	assert.NotNil(t, got.Branch.ActiveJobId)
	assert.NotNil(t, got.Branch.CustomDomains)
	assert.NotNil(t, got.Branch.EnvironmentVariables)
}

// TestCreateDomainAssociation_StatusReasonPresent proves
// DomainAssociation.StatusReason (required on types.DomainAssociation)
// survives a real client round trip. gopherstack never tracks a real
// status reason (there is no certificate/DNS-propagation flow behind this
// emulator), so this is a disclosed, honestly-empty value, not a
// fabrication -- the bug was the key being dropped entirely, leaving a real
// client's *string nil instead of a pointer to "".
func TestCreateDomainAssociation_StatusReasonPresent(t *testing.T) {
	t.Parallel()

	backend := amplify.NewInMemoryBackend("000000000000", "us-east-1")
	client := newTestAmplifyClient(t, amplify.NewHandler(backend))
	ctx := t.Context()

	app, err := client.CreateApp(ctx, &amplifysdk.CreateAppInput{Name: aws.String("app-domain")})
	require.NoError(t, err)

	created, err := client.CreateDomainAssociation(ctx, &amplifysdk.CreateDomainAssociationInput{
		AppId:      app.App.AppId,
		DomainName: aws.String("example.com"),
		SubDomainSettings: []amplifytypes.SubDomainSetting{
			{Prefix: aws.String(""), BranchName: aws.String("main")},
		},
	})
	require.NoError(t, err)

	require.NotNil(t, created.DomainAssociation.StatusReason, "StatusReason is required on DomainAssociation")
	assert.Empty(t, aws.ToString(created.DomainAssociation.StatusReason))
}

// TestCreateWebhook_DescriptionPresentWhenEmpty proves Webhook.Description
// (required on types.Webhook) survives a real client round trip when the
// caller supplies none -- CreateWebhookInput.Description is optional, so an
// unset description is a fully reachable state, and previously dropped the
// required key entirely.
func TestCreateWebhook_DescriptionPresentWhenEmpty(t *testing.T) {
	t.Parallel()

	backend := amplify.NewInMemoryBackend("000000000000", "us-east-1")
	client := newTestAmplifyClient(t, amplify.NewHandler(backend))
	ctx := t.Context()

	app, err := client.CreateApp(ctx, &amplifysdk.CreateAppInput{Name: aws.String("app-webhook")})
	require.NoError(t, err)

	_, err = client.CreateBranch(ctx, &amplifysdk.CreateBranchInput{
		AppId:      app.App.AppId,
		BranchName: aws.String("main"),
	})
	require.NoError(t, err)

	created, err := client.CreateWebhook(ctx, &amplifysdk.CreateWebhookInput{
		AppId:      app.App.AppId,
		BranchName: aws.String("main"),
	})
	require.NoError(t, err)

	require.NotNil(t, created.Webhook.Description, "Description is required on Webhook")
	assert.Empty(t, aws.ToString(created.Webhook.Description))
}

// TestStartJob_RequiredOutputFieldsPresentWhenEmpty proves JobSummary's
// required (types.JobSummary) CommitId/CommitMessage/CommitTime survive a
// real client round trip for a job started with no commit metadata at all
// (e.g. a manually deployed app). CommitId/CommitMessage previously dropped
// their keys when empty; CommitTime was deliberately omitted whenever the
// caller supplied none, per a prior sweep's documented (but, per this
// campaign's convention, incorrect) decision -- it now falls back to the
// job's own StartTime, mirroring the fallback toStepViews already applies
// to a still-running step's EndTime, rather than leaving a required
// *time.Time nil.
func TestStartJob_RequiredOutputFieldsPresentWhenEmpty(t *testing.T) {
	t.Parallel()

	backend := amplify.NewInMemoryBackend("000000000000", "us-east-1")
	client := newTestAmplifyClient(t, amplify.NewHandler(backend))
	ctx := t.Context()

	app, err := client.CreateApp(ctx, &amplifysdk.CreateAppInput{Name: aws.String("app-job")})
	require.NoError(t, err)

	_, err = client.CreateBranch(ctx, &amplifysdk.CreateBranchInput{
		AppId:      app.App.AppId,
		BranchName: aws.String("main"),
	})
	require.NoError(t, err)

	started, err := client.StartJob(ctx, &amplifysdk.StartJobInput{
		AppId:      app.App.AppId,
		BranchName: aws.String("main"),
		JobType:    "RELEASE",
	})
	require.NoError(t, err)

	js := started.JobSummary
	require.NotNil(t, js.CommitId, "CommitId is required on JobSummary")
	assert.Empty(t, aws.ToString(js.CommitId))
	require.NotNil(t, js.CommitMessage, "CommitMessage is required on JobSummary")
	assert.Empty(t, aws.ToString(js.CommitMessage))
	require.NotNil(t, js.CommitTime, "CommitTime is required on JobSummary")
	assert.False(t, js.CommitTime.IsZero())
}

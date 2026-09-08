package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	ec2sdk "github.com/aws/aws-sdk-go-v2/service/ec2"
	grafanasdk "github.com/aws/aws-sdk-go-v2/service/grafana"
	grafanatypes "github.com/aws/aws-sdk-go-v2/service/grafana/types"
	iamsdk "github.com/aws/aws-sdk-go-v2/service/iam"
	identitystoresdk "github.com/aws/aws-sdk-go-v2/service/identitystore"
	organizationssdk "github.com/aws/aws-sdk-go-v2/service/organizations"
	organizationstypes "github.com/aws/aws-sdk-go-v2/service/organizations/types"
	ssoadminsdk "github.com/aws/aws-sdk-go-v2/service/ssoadmin"
	smithy "github.com/aws/smithy-go"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createGrafanaClientAt returns a Grafana client pointed at ep.
func createGrafanaClientAt(t *testing.T, ep string) *grafanasdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return grafanasdk.NewFromConfig(cfg, func(o *grafanasdk.Options) {
		o.BaseEndpoint = aws.String(ep)
	})
}

// createGrafanaClient returns a Grafana client pointed at the shared test container.
func createGrafanaClient(t *testing.T) *grafanasdk.Client {
	t.Helper()

	return createGrafanaClientAt(t, endpoint)
}

// createIAMClientAt returns an IAM client pointed at ep.
func createIAMClientAt(t *testing.T, ep string) *iamsdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return iamsdk.NewFromConfig(cfg, func(o *iamsdk.Options) {
		o.BaseEndpoint = aws.String(ep)
	})
}

// createEC2ClientAt returns an EC2 client pointed at ep.
func createEC2ClientAt(t *testing.T, ep string) *ec2sdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return ec2sdk.NewFromConfig(cfg, func(o *ec2sdk.Options) {
		o.BaseEndpoint = aws.String(ep)
	})
}

// createOrganizationsClientAt returns an Organizations client pointed at ep.
func createOrganizationsClientAt(t *testing.T, ep string) *organizationssdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return organizationssdk.NewFromConfig(cfg, func(o *organizationssdk.Options) {
		o.BaseEndpoint = aws.String(ep)
	})
}

// grafanaCleanupCtx returns a context for use inside t.Cleanup callbacks.
// t.Context() must not be used there: Go 1.24+ cancels it before cleanups run.
func grafanaCleanupCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}

// awsErrorCode extracts the smithy error code from err, or "" if err isn't one.
func awsErrorCode(err error) string {
	if apiErr, ok := errors.AsType[smithy.APIError](err); ok {
		return apiErr.ErrorCode()
	}

	return ""
}

// TestIntegration_Grafana_WorkspaceLifecycle drives every Grafana operation
// through a real aws-sdk-go-v2 client against one workspace: creation and its
// async transition to ACTIVE, describe/list/update, authentication,
// configuration and version upgrade, licensing, permissions, API keys,
// service accounts and their tokens, tagging, and deletion.
func TestIntegration_Grafana_WorkspaceLifecycle(t *testing.T) { //nolint:tparallel // sequential subtests
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	ctx := t.Context()
	client := createGrafanaClient(t)

	workspaceName := "integ-grafana-" + uuid.NewString()[:8]

	createOut, createErr := client.CreateWorkspace(ctx, &grafanasdk.CreateWorkspaceInput{
		AccountAccessType: grafanatypes.AccountAccessTypeCurrentAccount,
		AuthenticationProviders: []grafanatypes.AuthenticationProviderTypes{
			grafanatypes.AuthenticationProviderTypesAwsSso,
		},
		PermissionType: grafanatypes.PermissionTypeCustomerManaged,
		WorkspaceName:  aws.String(workspaceName),
		GrafanaVersion: aws.String("9.4"),
	})
	require.NoError(t, createErr, "CreateWorkspace should succeed")
	require.NotNil(t, createOut.Workspace)

	workspaceID := aws.ToString(createOut.Workspace.Id)
	require.NotEmpty(t, workspaceID, "workspace ID must be returned")
	assert.Equal(t, grafanatypes.WorkspaceStatusCreating, createOut.Workspace.Status)

	// WorkspaceDescription carries no Arn field on the real SDK -- TagResource
	// et al. take the ARN as a caller-supplied parameter instead, built per
	// grafana.InMemoryBackend.WorkspaceARN's documented format.
	workspaceARN := "arn:aws:grafana:us-east-1:000000000000:/workspaces/" + workspaceID

	t.Cleanup(func() {
		cctx, cancel := grafanaCleanupCtx()
		defer cancel()
		_, _ = client.DeleteWorkspace(cctx, &grafanasdk.DeleteWorkspaceInput{WorkspaceId: aws.String(workspaceID)})
	})

	require.Eventually(t, func() bool {
		out, getErr := client.DescribeWorkspace(
			ctx,
			&grafanasdk.DescribeWorkspaceInput{WorkspaceId: aws.String(workspaceID)},
		)

		return getErr == nil && out.Workspace.Status == grafanatypes.WorkspaceStatusActive
	}, 5*time.Second, 50*time.Millisecond, "workspace should transition CREATING -> ACTIVE")

	t.Run("ListWorkspaces", func(t *testing.T) { //nolint:paralleltest // sequential by design
		listOut, err := client.ListWorkspaces(ctx, &grafanasdk.ListWorkspacesInput{})
		require.NoError(t, err, "ListWorkspaces should succeed")

		found := false
		for _, w := range listOut.Workspaces {
			if aws.ToString(w.Id) == workspaceID {
				found = true

				break
			}
		}
		assert.True(t, found, "created workspace should appear in ListWorkspaces")
	})

	t.Run("UpdateWorkspace", func(t *testing.T) { //nolint:paralleltest // sequential by design
		newName := workspaceName + "-updated"

		updOut, err := client.UpdateWorkspace(ctx, &grafanasdk.UpdateWorkspaceInput{
			WorkspaceId:   aws.String(workspaceID),
			WorkspaceName: aws.String(newName),
		})
		require.NoError(t, err, "UpdateWorkspace should succeed")
		assert.Equal(t, newName, aws.ToString(updOut.Workspace.Name))
		assert.Equal(t, grafanatypes.WorkspaceStatusUpdating, updOut.Workspace.Status)

		require.Eventually(t, func() bool {
			out, getErr := client.DescribeWorkspace(
				ctx,
				&grafanasdk.DescribeWorkspaceInput{WorkspaceId: aws.String(workspaceID)},
			)

			return getErr == nil && out.Workspace.Status == grafanatypes.WorkspaceStatusActive
		}, 5*time.Second, 50*time.Millisecond, "workspace should transition UPDATING -> ACTIVE")

		// UpdateWorkspace on a non-existent workspace is a real, wire-modeled
		// error path: assert it deserializes as ResourceNotFoundException.
		_, err = client.UpdateWorkspace(ctx, &grafanasdk.UpdateWorkspaceInput{
			WorkspaceId:   aws.String("g-does-not-exist"),
			WorkspaceName: aws.String("whatever"),
		})
		require.Error(t, err)
		assert.Equal(t, "ResourceNotFoundException", awsErrorCode(err))
	})

	t.Run("Authentication", func(t *testing.T) { //nolint:paralleltest // sequential by design
		descOut, err := client.DescribeWorkspaceAuthentication(ctx, &grafanasdk.DescribeWorkspaceAuthenticationInput{
			WorkspaceId: aws.String(workspaceID),
		})
		require.NoError(t, err, "DescribeWorkspaceAuthentication should succeed")
		require.NotNil(t, descOut.Authentication)
		require.NotNil(t, descOut.Authentication.AwsSso)
		assert.NotEmpty(t, aws.ToString(descOut.Authentication.AwsSso.SsoClientId))

		updOut, err := client.UpdateWorkspaceAuthentication(ctx, &grafanasdk.UpdateWorkspaceAuthenticationInput{
			WorkspaceId: aws.String(workspaceID),
			AuthenticationProviders: []grafanatypes.AuthenticationProviderTypes{
				grafanatypes.AuthenticationProviderTypesSaml,
			},
			SamlConfiguration: &grafanatypes.SamlConfiguration{
				IdpMetadata: &grafanatypes.IdpMetadataMemberUrl{Value: "https://idp.example.com/metadata"},
			},
		})
		require.NoError(t, err, "UpdateWorkspaceAuthentication should succeed")
		require.NotNil(t, updOut.Authentication.Saml)
		assert.Equal(t, grafanatypes.SamlConfigurationStatusConfigured, updOut.Authentication.Saml.Status)

		// A malformed SAML union (both url and xml) is a real ValidationException.
		_, err = client.UpdateWorkspaceAuthentication(ctx, &grafanasdk.UpdateWorkspaceAuthenticationInput{
			WorkspaceId: aws.String(workspaceID),
			AuthenticationProviders: []grafanatypes.AuthenticationProviderTypes{
				grafanatypes.AuthenticationProviderTypesSaml,
			},
			SamlConfiguration: &grafanatypes.SamlConfiguration{
				IdpMetadata: &grafanatypes.IdpMetadataMemberUrl{Value: "https://idp.example.com/metadata"},
			},
		})
		require.NoError(t, err, "single-member union should still be accepted")
	})

	t.Run("ConfigurationAndVersionUpgrade", func(t *testing.T) { //nolint:paralleltest // sequential by design
		_, err := client.UpdateWorkspaceConfiguration(ctx, &grafanasdk.UpdateWorkspaceConfigurationInput{
			WorkspaceId:   aws.String(workspaceID),
			Configuration: aws.String(`{"plugins":{"pluginAdminEnabled":true}}`),
		})
		require.NoError(t, err, "UpdateWorkspaceConfiguration should succeed")

		descOut, err := client.DescribeWorkspaceConfiguration(ctx, &grafanasdk.DescribeWorkspaceConfigurationInput{
			WorkspaceId: aws.String(workspaceID),
		})
		require.NoError(t, err, "DescribeWorkspaceConfiguration should succeed")
		assert.JSONEq(t, `{"plugins":{"pluginAdminEnabled":true}}`, aws.ToString(descOut.Configuration))

		_, err = client.UpdateWorkspaceConfiguration(ctx, &grafanasdk.UpdateWorkspaceConfigurationInput{
			WorkspaceId:    aws.String(workspaceID),
			Configuration:  aws.String(`{}`),
			GrafanaVersion: aws.String("10.4"),
		})
		require.NoError(t, err, "upgrading grafanaVersion should succeed")

		require.Eventually(t, func() bool {
			out, getErr := client.DescribeWorkspace(
				ctx,
				&grafanasdk.DescribeWorkspaceInput{WorkspaceId: aws.String(workspaceID)},
			)

			return getErr == nil && out.Workspace.Status == grafanatypes.WorkspaceStatusActive
		}, 5*time.Second, 50*time.Millisecond, "workspace should transition VERSION_UPDATING -> ACTIVE")

		descOut2, err := client.DescribeWorkspace(
			ctx,
			&grafanasdk.DescribeWorkspaceInput{WorkspaceId: aws.String(workspaceID)},
		)
		require.NoError(t, err)
		assert.Equal(t, "10.4", aws.ToString(descOut2.Workspace.GrafanaVersion))

		// Downgrade is rejected: real AWS's GrafanaVersion doc comment is
		// "Can only be used to upgrade ... not downgrade".
		_, err = client.UpdateWorkspaceConfiguration(ctx, &grafanasdk.UpdateWorkspaceConfigurationInput{
			WorkspaceId:    aws.String(workspaceID),
			Configuration:  aws.String(`{}`),
			GrafanaVersion: aws.String("8.4"),
		})
		require.Error(t, err, "downgrading grafanaVersion should be rejected")
		assert.Equal(t, "ValidationException", awsErrorCode(err))
	})

	t.Run("Versions", func(t *testing.T) { //nolint:paralleltest // sequential by design
		listOut, err := client.ListVersions(ctx, &grafanasdk.ListVersionsInput{})
		require.NoError(t, err, "ListVersions should succeed")
		assert.Contains(t, listOut.GrafanaVersions, "10.4")

		scoped, err := client.ListVersions(ctx, &grafanasdk.ListVersionsInput{WorkspaceId: aws.String(workspaceID)})
		require.NoError(t, err, "ListVersions scoped to a workspace should succeed")
		assert.Empty(t, scoped.GrafanaVersions, "workspace already on the latest version has no further upgrades")
	})

	t.Run("License", func(t *testing.T) { //nolint:paralleltest // sequential by design
		assocOut, err := client.AssociateLicense(ctx, &grafanasdk.AssociateLicenseInput{
			WorkspaceId: aws.String(workspaceID),
			LicenseType: grafanatypes.LicenseTypeEnterprise,
		})
		require.NoError(t, err, "AssociateLicense should succeed")
		assert.Equal(t, grafanatypes.LicenseTypeEnterprise, assocOut.Workspace.LicenseType)
		assert.Equal(t, grafanatypes.WorkspaceStatusUpgrading, assocOut.Workspace.Status)

		require.Eventually(t, func() bool {
			out, getErr := client.DescribeWorkspace(
				ctx,
				&grafanasdk.DescribeWorkspaceInput{WorkspaceId: aws.String(workspaceID)},
			)

			return getErr == nil && out.Workspace.Status == grafanatypes.WorkspaceStatusActive
		}, 5*time.Second, 50*time.Millisecond, "workspace should transition UPGRADING -> ACTIVE")

		// Free trial is a real, documented rejection: "Amazon Managed Grafana
		// workspaces no longer support Grafana Enterprise free trials".
		_, err = client.AssociateLicense(ctx, &grafanasdk.AssociateLicenseInput{
			WorkspaceId: aws.String(workspaceID),
			LicenseType: grafanatypes.LicenseTypeEnterpriseFreeTrial,
		})
		require.Error(t, err)
		assert.Equal(t, "ValidationException", awsErrorCode(err))

		disOut, err := client.DisassociateLicense(ctx, &grafanasdk.DisassociateLicenseInput{
			WorkspaceId: aws.String(workspaceID),
			LicenseType: grafanatypes.LicenseTypeEnterprise,
		})
		require.NoError(t, err, "DisassociateLicense should succeed")
		assert.Empty(t, disOut.Workspace.LicenseType)
	})

	t.Run("Permissions", func(t *testing.T) { //nolint:paralleltest // sequential by design
		// ssoadmin always seeds a default IAM Identity Center instance (see
		// services/ssoadmin's NewInMemoryBackend), so a real identity store is
		// always resolvable here -- granting a permission to an SSO_USER ID
		// that doesn't exist in it is genuinely rejected (see
		// grafana.InMemoryBackend.validatePermissionUser).
		instOut, err := createSSOAdminClient(t).ListInstances(ctx, &ssoadminsdk.ListInstancesInput{})
		require.NoError(t, err, "ListInstances should succeed")
		require.NotEmpty(t, instOut.Instances, "ssoadmin should have a seeded default instance")
		storeID := aws.ToString(instOut.Instances[0].IdentityStoreId)

		userOut, err := createIdentityStoreClient(t).CreateUser(ctx, &identitystoresdk.CreateUserInput{
			IdentityStoreId: aws.String(storeID),
			UserName:        aws.String("integ-grafana-user-" + uuid.NewString()[:8]),
		})
		require.NoError(t, err, "CreateUser should succeed")
		userID := aws.ToString(userOut.UserId)

		_, err = client.UpdatePermissions(ctx, &grafanasdk.UpdatePermissionsInput{
			WorkspaceId: aws.String(workspaceID),
			UpdateInstructionBatch: []grafanatypes.UpdateInstruction{
				{
					Action: grafanatypes.UpdateActionAdd,
					Role:   grafanatypes.RoleAdmin,
					Users:  []grafanatypes.User{{Id: aws.String(userID), Type: grafanatypes.UserTypeSsoUser}},
				},
			},
		})
		require.NoError(t, err, "UpdatePermissions ADD for a real SSO user should succeed")

		listOut, err := client.ListPermissions(
			ctx,
			&grafanasdk.ListPermissionsInput{WorkspaceId: aws.String(workspaceID)},
		)
		require.NoError(t, err, "ListPermissions should succeed")

		found := false
		for _, p := range listOut.Permissions {
			if aws.ToString(p.User.Id) == userID {
				found = true
				assert.Equal(t, grafanatypes.RoleAdmin, p.Role)
			}
		}
		assert.True(t, found, "granted permission should be listed")

		// A malformed instruction (no users) and an ADD referencing an
		// SSO_USER ID that doesn't exist in the identity store are both part
		// of UpdatePermissions's own partial-failure batch surface, not a
		// top-level error.
		batchOut, err := client.UpdatePermissions(ctx, &grafanasdk.UpdatePermissionsInput{
			WorkspaceId: aws.String(workspaceID),
			UpdateInstructionBatch: []grafanatypes.UpdateInstruction{
				{Action: grafanatypes.UpdateActionAdd, Role: grafanatypes.RoleViewer, Users: []grafanatypes.User{}},
				{
					Action: grafanatypes.UpdateActionAdd,
					Role:   grafanatypes.RoleViewer,
					Users: []grafanatypes.User{
						{Id: aws.String(uuid.NewString()), Type: grafanatypes.UserTypeSsoUser},
					},
				},
			},
		})
		require.NoError(t, err, "UpdatePermissions should still return 200 with a partial-failure batch")
		assert.Len(t, batchOut.Errors, 2, "both the empty-users and unknown-SSO-user instructions should be reported")

		_, err = client.UpdatePermissions(ctx, &grafanasdk.UpdatePermissionsInput{
			WorkspaceId: aws.String(workspaceID),
			UpdateInstructionBatch: []grafanatypes.UpdateInstruction{
				{
					Action: grafanatypes.UpdateActionRevoke,
					Role:   grafanatypes.RoleAdmin,
					Users:  []grafanatypes.User{{Id: aws.String(userID), Type: grafanatypes.UserTypeSsoUser}},
				},
			},
		})
		require.NoError(t, err, "UpdatePermissions REVOKE should succeed")
	})

	var apiKeyName string

	t.Run("APIKeys", func(t *testing.T) { //nolint:paralleltest // sequential by design
		apiKeyName = "integ-key-" + uuid.NewString()[:8]

		createOut, err := client.CreateWorkspaceApiKey(ctx, &grafanasdk.CreateWorkspaceApiKeyInput{
			WorkspaceId:   aws.String(workspaceID),
			KeyName:       aws.String(apiKeyName),
			KeyRole:       aws.String("VIEWER"),
			SecondsToLive: aws.Int32(3600),
		})
		require.NoError(t, err, "CreateWorkspaceApiKey should succeed")
		assert.NotEmpty(t, aws.ToString(createOut.Key), "API key material must be returned")
		assert.Equal(t, apiKeyName, aws.ToString(createOut.KeyName))

		_, err = client.DeleteWorkspaceApiKey(ctx, &grafanasdk.DeleteWorkspaceApiKeyInput{
			WorkspaceId: aws.String(workspaceID),
			KeyName:     aws.String(apiKeyName),
		})
		require.NoError(t, err, "DeleteWorkspaceApiKey should succeed")

		_, err = client.DeleteWorkspaceApiKey(ctx, &grafanasdk.DeleteWorkspaceApiKeyInput{
			WorkspaceId: aws.String(workspaceID),
			KeyName:     aws.String(apiKeyName),
		})
		require.Error(t, err, "deleting an already-deleted key should fail")
		assert.Equal(t, "ResourceNotFoundException", awsErrorCode(err))
	})

	t.Run("ServiceAccountsAndTokens", func(t *testing.T) { //nolint:paralleltest // sequential by design
		saName := "integ-sa-" + uuid.NewString()[:8]

		saOut, err := client.CreateWorkspaceServiceAccount(ctx, &grafanasdk.CreateWorkspaceServiceAccountInput{
			WorkspaceId: aws.String(workspaceID),
			Name:        aws.String(saName),
			GrafanaRole: grafanatypes.RoleEditor,
		})
		require.NoError(t, err, "CreateWorkspaceServiceAccount should succeed")
		saID := aws.ToString(saOut.Id)
		require.NotEmpty(t, saID)

		listSAOut, err := client.ListWorkspaceServiceAccounts(ctx, &grafanasdk.ListWorkspaceServiceAccountsInput{
			WorkspaceId: aws.String(workspaceID),
		})
		require.NoError(t, err, "ListWorkspaceServiceAccounts should succeed")

		found := false
		for _, sa := range listSAOut.ServiceAccounts {
			if aws.ToString(sa.Id) == saID {
				found = true
			}
		}
		assert.True(t, found, "created service account should be listed")

		tokOut, err := client.CreateWorkspaceServiceAccountToken(
			ctx,
			&grafanasdk.CreateWorkspaceServiceAccountTokenInput{
				WorkspaceId:      aws.String(workspaceID),
				ServiceAccountId: aws.String(saID),
				Name:             aws.String("integ-token"),
				SecondsToLive:    aws.Int32(3600),
			},
		)
		require.NoError(t, err, "CreateWorkspaceServiceAccountToken should succeed")
		require.NotNil(t, tokOut.ServiceAccountToken)
		assert.NotEmpty(t, aws.ToString(tokOut.ServiceAccountToken.Key), "token key material must be returned once")
		tokenID := aws.ToString(tokOut.ServiceAccountToken.Id)

		listTokOut, err := client.ListWorkspaceServiceAccountTokens(
			ctx,
			&grafanasdk.ListWorkspaceServiceAccountTokensInput{
				WorkspaceId:      aws.String(workspaceID),
				ServiceAccountId: aws.String(saID),
			},
		)
		require.NoError(t, err, "ListWorkspaceServiceAccountTokens should succeed")
		require.Len(t, listTokOut.ServiceAccountTokens, 1)
		assert.Equal(t, tokenID, aws.ToString(listTokOut.ServiceAccountTokens[0].Id))

		_, err = client.DeleteWorkspaceServiceAccountToken(ctx, &grafanasdk.DeleteWorkspaceServiceAccountTokenInput{
			WorkspaceId:      aws.String(workspaceID),
			ServiceAccountId: aws.String(saID),
			TokenId:          aws.String(tokenID),
		})
		require.NoError(t, err, "DeleteWorkspaceServiceAccountToken should succeed")

		_, err = client.DeleteWorkspaceServiceAccount(ctx, &grafanasdk.DeleteWorkspaceServiceAccountInput{
			WorkspaceId:      aws.String(workspaceID),
			ServiceAccountId: aws.String(saID),
		})
		require.NoError(t, err, "DeleteWorkspaceServiceAccount should succeed")
	})

	t.Run("Tags", func(t *testing.T) { //nolint:paralleltest // sequential by design
		resourceArn := workspaceARN

		_, err := client.TagResource(ctx, &grafanasdk.TagResourceInput{
			ResourceArn: aws.String(resourceArn),
			Tags:        map[string]string{"env": "integ"},
		})
		require.NoError(t, err, "TagResource should succeed")

		listOut, err := client.ListTagsForResource(
			ctx,
			&grafanasdk.ListTagsForResourceInput{ResourceArn: aws.String(resourceArn)},
		)
		require.NoError(t, err, "ListTagsForResource should succeed")
		assert.Equal(t, "integ", listOut.Tags["env"])

		_, err = client.UntagResource(ctx, &grafanasdk.UntagResourceInput{
			ResourceArn: aws.String(resourceArn),
			TagKeys:     []string{"env"},
		})
		require.NoError(t, err, "UntagResource should succeed")

		listOut2, err := client.ListTagsForResource(
			ctx,
			&grafanasdk.ListTagsForResourceInput{ResourceArn: aws.String(resourceArn)},
		)
		require.NoError(t, err)
		assert.NotContains(t, listOut2.Tags, "env")
	})

	t.Run("DeleteWorkspace", func(t *testing.T) { //nolint:paralleltest // sequential by design
		delOut, err := client.DeleteWorkspace(
			ctx,
			&grafanasdk.DeleteWorkspaceInput{WorkspaceId: aws.String(workspaceID)},
		)
		require.NoError(t, err, "DeleteWorkspace should succeed")
		assert.Equal(t, grafanatypes.WorkspaceStatusDeleting, delOut.Workspace.Status)

		_, err = client.DescribeWorkspace(ctx, &grafanasdk.DescribeWorkspaceInput{WorkspaceId: aws.String(workspaceID)})
		require.Error(t, err, "deleted workspace should no longer be describable")
		assert.Equal(t, "ResourceNotFoundException", awsErrorCode(err))
	})
}

// TestIntegration_Grafana_ServiceQuota drives CreateWorkspace past Amazon
// Managed Grafana's published default "Number of workspaces" quota (5 per
// account per Region) and asserts the real ServiceQuotaExceededException
// shape. Runs against an isolated container so its workspace count starts
// at zero and isn't shared with other tests' workspaces.
func TestIntegration_Grafana_ServiceQuota(t *testing.T) {
	t.Parallel()

	ep := startChaosContainer(t)
	ctx := t.Context()
	client := createGrafanaClientAt(t, ep)

	const maxWorkspaces = 5

	ids := make([]string, 0, maxWorkspaces)

	for i := range maxWorkspaces {
		out, err := client.CreateWorkspace(ctx, &grafanasdk.CreateWorkspaceInput{
			AccountAccessType: grafanatypes.AccountAccessTypeCurrentAccount,
			AuthenticationProviders: []grafanatypes.AuthenticationProviderTypes{
				grafanatypes.AuthenticationProviderTypesAwsSso,
			},
			PermissionType: grafanatypes.PermissionTypeCustomerManaged,
			WorkspaceName:  aws.String("integ-quota-" + uuid.NewString()[:8]),
		})
		require.NoErrorf(t, err, "CreateWorkspace %d/%d should succeed within quota", i+1, maxWorkspaces)
		ids = append(ids, aws.ToString(out.Workspace.Id))
	}

	t.Cleanup(func() {
		cctx, cancel := grafanaCleanupCtx()
		defer cancel()

		for _, id := range ids {
			_, _ = client.DeleteWorkspace(cctx, &grafanasdk.DeleteWorkspaceInput{WorkspaceId: aws.String(id)})
		}
	})

	_, err := client.CreateWorkspace(ctx, &grafanasdk.CreateWorkspaceInput{
		AccountAccessType: grafanatypes.AccountAccessTypeCurrentAccount,
		AuthenticationProviders: []grafanatypes.AuthenticationProviderTypes{
			grafanatypes.AuthenticationProviderTypesAwsSso,
		},
		PermissionType: grafanatypes.PermissionTypeCustomerManaged,
		WorkspaceName:  aws.String("integ-quota-over-" + uuid.NewString()[:8]),
	})
	require.Error(t, err, "CreateWorkspace beyond the quota should fail")
	assert.Equal(t, "ServiceQuotaExceededException", awsErrorCode(err))
}

// TestIntegration_Grafana_CrossServiceValidation drives CreateWorkspace with
// WorkspaceRoleArn/VpcConfiguration/WorkspaceOrganizationalUnits references
// against the emulator's own IAM/EC2/Organizations backends: a reference
// that doesn't exist is rejected, and a reference to a resource genuinely
// created in those backends is accepted. Runs against an isolated container
// to avoid racing other integration tests' IAM/EC2/Organizations state.
func TestIntegration_Grafana_CrossServiceValidation(t *testing.T) { //nolint:tparallel // sequential subtests
	t.Parallel()

	ep := startChaosContainer(t)
	ctx := t.Context()

	grafanaClient := createGrafanaClientAt(t, ep)
	iamClient := createIAMClientAt(t, ep)
	ec2Client := createEC2ClientAt(t, ep)
	orgClient := createOrganizationsClientAt(t, ep)

	baseInput := func(name string) *grafanasdk.CreateWorkspaceInput {
		return &grafanasdk.CreateWorkspaceInput{
			AccountAccessType: grafanatypes.AccountAccessTypeCurrentAccount,
			AuthenticationProviders: []grafanatypes.AuthenticationProviderTypes{
				grafanatypes.AuthenticationProviderTypesAwsSso,
			},
			PermissionType: grafanatypes.PermissionTypeCustomerManaged,
			WorkspaceName:  aws.String(name),
		}
	}

	cleanupWorkspace := func(t *testing.T, id string) {
		t.Helper()
		t.Cleanup(func() {
			cctx, cancel := grafanaCleanupCtx()
			defer cancel()
			_, _ = grafanaClient.DeleteWorkspace(cctx, &grafanasdk.DeleteWorkspaceInput{WorkspaceId: aws.String(id)})
		})
	}

	t.Run("RejectsNonexistentReferences", func(t *testing.T) {
		tests := []struct {
			mutate func(*grafanasdk.CreateWorkspaceInput)
			name   string
		}{
			{
				name: "role",
				mutate: func(in *grafanasdk.CreateWorkspaceInput) {
					in.WorkspaceRoleArn = aws.String(
						"arn:aws:iam::123456789012:role/does-not-exist-" + uuid.NewString()[:8],
					)
				},
			},
			{
				name: "vpc configuration",
				mutate: func(in *grafanasdk.CreateWorkspaceInput) {
					in.VpcConfiguration = &grafanatypes.VpcConfiguration{
						SubnetIds:        []string{"subnet-" + uuid.NewString()[:8]},
						SecurityGroupIds: []string{"sg-" + uuid.NewString()[:8]},
					}
				},
			},
			{
				name: "organizational unit",
				mutate: func(in *grafanasdk.CreateWorkspaceInput) {
					in.AccountAccessType = grafanatypes.AccountAccessTypeOrganization
					in.WorkspaceOrganizationalUnits = []string{"ou-doesnotexist-" + uuid.NewString()[:8]}
				},
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				in := baseInput("integ-badref-" + uuid.NewString()[:8])
				tc.mutate(in)

				_, err := grafanaClient.CreateWorkspace(ctx, in)
				require.Error(t, err, "a reference that doesn't exist should be rejected")
				assert.Equal(t, "ValidationException", awsErrorCode(err))
			})
		}
	})

	t.Run("AcceptsRealRole", func(t *testing.T) { //nolint:paralleltest // sequential by design
		roleName := "integ-grafana-role-" + uuid.NewString()[:8]

		roleOut, err := iamClient.CreateRole(ctx, &iamsdk.CreateRoleInput{
			RoleName:                 aws.String(roleName),
			AssumeRolePolicyDocument: aws.String(`{"Version":"2012-10-17","Statement":[]}`),
		})
		require.NoError(t, err, "CreateRole should succeed")

		t.Cleanup(func() {
			cctx, cancel := grafanaCleanupCtx()
			defer cancel()
			_, _ = iamClient.DeleteRole(cctx, &iamsdk.DeleteRoleInput{RoleName: aws.String(roleName)})
		})

		in := baseInput("integ-goodrole-" + uuid.NewString()[:8])
		in.WorkspaceRoleArn = roleOut.Role.Arn

		out, err := grafanaClient.CreateWorkspace(ctx, in)
		require.NoError(t, err, "a WorkspaceRoleArn for a real role should be accepted")
		cleanupWorkspace(t, aws.ToString(out.Workspace.Id))
	})

	t.Run("AcceptsRealVpcConfiguration", func(t *testing.T) { //nolint:paralleltest // sequential by design
		vpcOut, err := ec2Client.CreateVpc(ctx, &ec2sdk.CreateVpcInput{CidrBlock: aws.String("10.77.0.0/16")})
		require.NoError(t, err, "CreateVpc should succeed")
		vpcID := aws.ToString(vpcOut.Vpc.VpcId)

		subnetOut, err := ec2Client.CreateSubnet(ctx, &ec2sdk.CreateSubnetInput{
			VpcId:     aws.String(vpcID),
			CidrBlock: aws.String("10.77.1.0/24"),
		})
		require.NoError(t, err, "CreateSubnet should succeed")
		subnetID := aws.ToString(subnetOut.Subnet.SubnetId)

		sgOut, err := ec2Client.CreateSecurityGroup(ctx, &ec2sdk.CreateSecurityGroupInput{
			GroupName:   aws.String("integ-grafana-sg-" + uuid.NewString()[:8]),
			Description: aws.String("grafana integration test"),
			VpcId:       aws.String(vpcID),
		})
		require.NoError(t, err, "CreateSecurityGroup should succeed")
		sgID := aws.ToString(sgOut.GroupId)

		t.Cleanup(func() {
			cctx, cancel := grafanaCleanupCtx()
			defer cancel()
			_, _ = ec2Client.DeleteSecurityGroup(cctx, &ec2sdk.DeleteSecurityGroupInput{GroupId: aws.String(sgID)})
			_, _ = ec2Client.DeleteSubnet(cctx, &ec2sdk.DeleteSubnetInput{SubnetId: aws.String(subnetID)})
			_, _ = ec2Client.DeleteVpc(cctx, &ec2sdk.DeleteVpcInput{VpcId: aws.String(vpcID)})
		})

		in := baseInput("integ-goodvpc-" + uuid.NewString()[:8])
		in.VpcConfiguration = &grafanatypes.VpcConfiguration{
			SubnetIds:        []string{subnetID},
			SecurityGroupIds: []string{sgID},
		}

		out, err := grafanaClient.CreateWorkspace(ctx, in)
		require.NoError(t, err, "a VpcConfiguration referencing real EC2 resources should be accepted")
		cleanupWorkspace(t, aws.ToString(out.Workspace.Id))
	})

	t.Run("AcceptsRealOrganizationalUnit", func(t *testing.T) { //nolint:paralleltest // sequential by design
		_, err := orgClient.CreateOrganization(ctx, &organizationssdk.CreateOrganizationInput{
			FeatureSet: organizationstypes.OrganizationFeatureSetAll,
		})
		require.NoError(t, err, "CreateOrganization should succeed on a fresh account")

		rootsOut, err := orgClient.ListRoots(ctx, &organizationssdk.ListRootsInput{})
		require.NoError(t, err, "ListRoots should succeed")
		require.NotEmpty(t, rootsOut.Roots)
		rootID := aws.ToString(rootsOut.Roots[0].Id)

		ouOut, err := orgClient.CreateOrganizationalUnit(ctx, &organizationssdk.CreateOrganizationalUnitInput{
			ParentId: aws.String(rootID),
			Name:     aws.String("integ-grafana-ou-" + uuid.NewString()[:8]),
		})
		require.NoError(t, err, "CreateOrganizationalUnit should succeed")
		ouID := aws.ToString(ouOut.OrganizationalUnit.Id)

		in := baseInput("integ-goodou-" + uuid.NewString()[:8])
		in.AccountAccessType = grafanatypes.AccountAccessTypeOrganization
		in.WorkspaceOrganizationalUnits = []string{ouID}

		out, err := grafanaClient.CreateWorkspace(ctx, in)
		require.NoError(t, err, "a WorkspaceOrganizationalUnits entry for a real OU should be accepted")
		cleanupWorkspace(t, aws.ToString(out.Workspace.Id))
	})
}

// TestIntegration_Grafana_ChaosWorkspaceTransitions drives a chaos fault rule
// targeting service "grafana" operation "WorkspaceTransition" -- this
// backend's pseudo-op name for its async lifecycle timer -- and asserts it
// steers CreateWorkspace/UpdateWorkspace's async transition to the
// operation's *_FAILED status (or DEGRADED) instead of ACTIVE, and steers
// DeleteWorkspace to DELETION_FAILED without actually deleting. Runs on its
// own container: fault rules are global mutable state that would otherwise
// leak into every other parallel test hitting the shared container.
func TestIntegration_Grafana_ChaosWorkspaceTransitions(t *testing.T) { //nolint:tparallel // sequential subtests
	t.Parallel()

	ep := startChaosContainer(t)
	ctx := t.Context()
	client := createGrafanaClientAt(t, ep)

	newWorkspace := func(t *testing.T) string {
		t.Helper()

		out, err := client.CreateWorkspace(ctx, &grafanasdk.CreateWorkspaceInput{
			AccountAccessType: grafanatypes.AccountAccessTypeCurrentAccount,
			AuthenticationProviders: []grafanatypes.AuthenticationProviderTypes{
				grafanatypes.AuthenticationProviderTypesAwsSso,
			},
			PermissionType: grafanatypes.PermissionTypeCustomerManaged,
			WorkspaceName:  aws.String("integ-chaos-" + uuid.NewString()[:8]),
		})
		require.NoError(t, err, "CreateWorkspace should succeed")

		return aws.ToString(out.Workspace.Id)
	}

	awaitStatus := func(t *testing.T, id string, want grafanatypes.WorkspaceStatus) {
		t.Helper()
		require.Eventually(t, func() bool {
			out, err := client.DescribeWorkspace(ctx, &grafanasdk.DescribeWorkspaceInput{WorkspaceId: aws.String(id)})

			return err == nil && out.Workspace.Status == want
		}, 5*time.Second, 50*time.Millisecond, "workspace should reach status %s", want)
	}

	t.Run("CreationFailure", func(t *testing.T) { //nolint:paralleltest // sequential by design
		postChaosRules(t, ep, []chaosFaultRule{
			{
				Service:   "grafana",
				Operation: "WorkspaceTransition",
				Error:     &chaosFaultError{Code: "CREATION_FAILED", StatusCode: 500},
			},
		})
		// A plain defer, not t.Cleanup: t.Context() is cancelled before
		// t.Cleanup callbacks run, and postChaosRules needs a live context.
		defer postChaosRules(t, ep, []chaosFaultRule{})

		id := newWorkspace(t)
		t.Cleanup(func() {
			cctx, cancel := grafanaCleanupCtx()
			defer cancel()
			_, _ = client.DeleteWorkspace(cctx, &grafanasdk.DeleteWorkspaceInput{WorkspaceId: aws.String(id)})
		})

		awaitStatus(t, id, grafanatypes.WorkspaceStatusCreationFailed)

		out, err := client.DescribeWorkspace(ctx, &grafanasdk.DescribeWorkspaceInput{WorkspaceId: aws.String(id)})
		require.NoError(t, err)
		assert.NotEmpty(t, aws.ToString(out.Workspace.DegradedWorkspaceReason))
	})

	t.Run("Degraded", func(t *testing.T) { //nolint:paralleltest // sequential by design
		id := newWorkspace(t)
		t.Cleanup(func() {
			cctx, cancel := grafanaCleanupCtx()
			defer cancel()
			_, _ = client.DeleteWorkspace(cctx, &grafanasdk.DeleteWorkspaceInput{WorkspaceId: aws.String(id)})
		})
		awaitStatus(t, id, grafanatypes.WorkspaceStatusActive)

		postChaosRules(t, ep, []chaosFaultRule{
			{
				Service:   "grafana",
				Operation: "WorkspaceTransition",
				Error:     &chaosFaultError{Code: "DEGRADED", StatusCode: 500},
			},
		})
		defer postChaosRules(t, ep, []chaosFaultRule{})

		_, err := client.UpdateWorkspace(ctx, &grafanasdk.UpdateWorkspaceInput{
			WorkspaceId:   aws.String(id),
			WorkspaceName: aws.String("integ-chaos-degraded-" + uuid.NewString()[:8]),
		})
		require.NoError(t, err, "UpdateWorkspace's synchronous response is unaffected by the async chaos rule")

		awaitStatus(t, id, grafanatypes.WorkspaceStatusDegraded)
	})

	t.Run("DeletionFailure", func(t *testing.T) { //nolint:paralleltest // sequential by design
		id := newWorkspace(t)
		awaitStatus(t, id, grafanatypes.WorkspaceStatusActive)

		postChaosRules(t, ep, []chaosFaultRule{
			{
				Service:   "grafana",
				Operation: "WorkspaceTransition",
				Error:     &chaosFaultError{Code: "AccessDeniedException", StatusCode: 403},
			},
		})

		delOut, err := client.DeleteWorkspace(ctx, &grafanasdk.DeleteWorkspaceInput{WorkspaceId: aws.String(id)})
		require.NoError(t, err, "a chaos-injected deletion failure is reported via workspace status, not an API error")
		assert.Equal(t, grafanatypes.WorkspaceStatusDeletionFailed, delOut.Workspace.Status)

		descOut, err := client.DescribeWorkspace(ctx, &grafanasdk.DescribeWorkspaceInput{WorkspaceId: aws.String(id)})
		require.NoError(t, err, "the workspace should still exist after a failed deletion")
		assert.Equal(t, grafanatypes.WorkspaceStatusDeletionFailed, descOut.Workspace.Status)

		postChaosRules(t, ep, []chaosFaultRule{})

		_, err = client.DeleteWorkspace(ctx, &grafanasdk.DeleteWorkspaceInput{WorkspaceId: aws.String(id)})
		require.NoError(t, err, "DeleteWorkspace should succeed once the chaos rule is cleared")
	})
}

package grafana_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	grafanasdk "github.com/aws/aws-sdk-go-v2/service/grafana"
	"github.com/aws/aws-sdk-go-v2/service/grafana/types"
	"github.com/stretchr/testify/require"
)

func TestServiceAccountLifecycle(t *testing.T) {
	t.Parallel()

	_, client := newTestHandlerAndClient(t)
	id := createActiveWorkspace(t, client, minimalCreateWorkspaceInput())

	created, err := client.CreateWorkspaceServiceAccount(t.Context(), &grafanasdk.CreateWorkspaceServiceAccountInput{
		WorkspaceId: aws.String(id),
		Name:        aws.String("automation"),
		GrafanaRole: types.RoleEditor,
	})
	require.NoError(t, err)
	require.NotEmpty(t, aws.ToString(created.Id))
	require.Equal(t, types.RoleEditor, created.GrafanaRole)

	list, err := client.ListWorkspaceServiceAccounts(t.Context(), &grafanasdk.ListWorkspaceServiceAccountsInput{
		WorkspaceId: aws.String(id),
	})
	require.NoError(t, err)
	require.Len(t, list.ServiceAccounts, 1)
	require.Equal(t, "automation", aws.ToString(list.ServiceAccounts[0].Name))
	require.Equal(t, "false", aws.ToString(list.ServiceAccounts[0].IsDisabled),
		"IsDisabled is wire-typed as *string on the real SDK, not *bool")

	_, err = client.DeleteWorkspaceServiceAccount(t.Context(), &grafanasdk.DeleteWorkspaceServiceAccountInput{
		WorkspaceId:      aws.String(id),
		ServiceAccountId: created.Id,
	})
	require.NoError(t, err)

	after, err := client.ListWorkspaceServiceAccounts(t.Context(), &grafanasdk.ListWorkspaceServiceAccountsInput{
		WorkspaceId: aws.String(id),
	})
	require.NoError(t, err)
	require.Empty(t, after.ServiceAccounts)
}

func TestCreateWorkspaceServiceAccount_PreV9Workspace_Conflict(t *testing.T) {
	t.Parallel()

	_, client := newTestHandlerAndClient(t)

	in := minimalCreateWorkspaceInput()
	in.GrafanaVersion = aws.String("8.4")
	id := createActiveWorkspace(t, client, in)

	_, err := client.CreateWorkspaceServiceAccount(t.Context(), &grafanasdk.CreateWorkspaceServiceAccountInput{
		WorkspaceId: aws.String(id),
		Name:        aws.String("automation"),
		GrafanaRole: types.RoleEditor,
	})
	require.Error(t, err)

	var ce *types.ConflictException
	require.ErrorAs(t, err, &ce)
}

func TestCreateWorkspaceServiceAccount_DuplicateName_Conflict(t *testing.T) {
	t.Parallel()

	_, client := newTestHandlerAndClient(t)
	id := createActiveWorkspace(t, client, minimalCreateWorkspaceInput())

	_, err := client.CreateWorkspaceServiceAccount(t.Context(), &grafanasdk.CreateWorkspaceServiceAccountInput{
		WorkspaceId: aws.String(id),
		Name:        aws.String("dup"),
		GrafanaRole: types.RoleViewer,
	})
	require.NoError(t, err)

	_, err = client.CreateWorkspaceServiceAccount(t.Context(), &grafanasdk.CreateWorkspaceServiceAccountInput{
		WorkspaceId: aws.String(id),
		Name:        aws.String("dup"),
		GrafanaRole: types.RoleViewer,
	})
	require.Error(t, err)

	var ce *types.ConflictException
	require.ErrorAs(t, err, &ce)
}

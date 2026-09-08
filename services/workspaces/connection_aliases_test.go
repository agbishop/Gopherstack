package workspaces_test

import (
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	wssdk "github.com/aws/aws-sdk-go-v2/service/workspaces"
	"github.com/aws/aws-sdk-go-v2/service/workspaces/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnectionAliasCRUD(t *testing.T) { //nolint:paralleltest // existing issue.
	tests := []struct {
		name             string
		connectionString string
	}{
		{name: "simple alias", connectionString: "myalias.corp.example"},
		{name: "ip alias", connectionString: "10.0.0.1"},
	}

	for _, tc := range tests { //nolint:paralleltest // existing issue.
		t.Run(tc.name, func(t *testing.T) {
			h, _ := newTestHandlerWithBackend(t)

			// Create
			rec := doTargetRequest(t, h, "CreateConnectionAlias", map[string]any{
				"ConnectionString": tc.connectionString,
			})
			if rec.Code != http.StatusOK {
				t.Fatalf("create: expected 200, got %d: %s", rec.Code, rec.Body)
			}

			var createOut map[string]string
			decodeJSON(t, rec.Body.Bytes(), &createOut)

			aliasID := createOut["AliasId"]
			if aliasID == "" {
				t.Fatal("expected non-empty AliasId")
			}

			// Describe
			rec2 := doTargetRequest(t, h, "DescribeConnectionAliases", map[string]any{
				"AliasIds": []string{aliasID},
			})
			if rec2.Code != http.StatusOK {
				t.Fatalf("describe: expected 200, got %d", rec2.Code)
			}

			var descOut struct {
				ConnectionAliases []map[string]any `json:"ConnectionAliases"`
			}
			decodeJSON(t, rec2.Body.Bytes(), &descOut)

			if len(descOut.ConnectionAliases) != 1 {
				t.Fatalf("expected 1 alias, got %d", len(descOut.ConnectionAliases))
			}

			// Associate
			rec3 := doTargetRequest(t, h, "AssociateConnectionAlias", map[string]any{
				"AliasId":    aliasID,
				"ResourceId": "res-123",
			})
			if rec3.Code != http.StatusOK {
				t.Fatalf("associate: expected 200, got %d", rec3.Code)
			}

			// Describe permissions
			rec4 := doTargetRequest(t, h, "DescribeConnectionAliasPermissions", map[string]any{
				"AliasId": aliasID,
			})
			if rec4.Code != http.StatusOK {
				t.Fatalf("describe perms: expected 200, got %d", rec4.Code)
			}

			// Update permission
			rec5 := doTargetRequest(t, h, "UpdateConnectionAliasPermission", map[string]any{
				"AliasId": aliasID,
				"ConnectionAliasPermission": map[string]any{
					"SharedAccountId":  "999988887777",
					"AllowAssociation": true,
				},
			})
			if rec5.Code != http.StatusOK {
				t.Fatalf("update perm: expected 200, got %d", rec5.Code)
			}

			// Disassociate
			rec6 := doTargetRequest(t, h, "DisassociateConnectionAlias", map[string]any{
				"AliasId": aliasID,
			})
			if rec6.Code != http.StatusOK {
				t.Fatalf("disassociate: expected 200, got %d", rec6.Code)
			}

			// Unshare: real AWS requires "no longer shared with any accounts
			// or associated with any directories" before delete
			// (api_op_DeleteConnectionAlias.go doc comment).
			recUnshare := doTargetRequest(t, h, "UpdateConnectionAliasPermission", map[string]any{
				"AliasId": aliasID,
				"ConnectionAliasPermission": map[string]any{
					"SharedAccountId":  "999988887777",
					"AllowAssociation": false,
				},
			})
			if recUnshare.Code != http.StatusOK {
				t.Fatalf("unshare: expected 200, got %d", recUnshare.Code)
			}

			// Delete
			rec7 := doTargetRequest(t, h, "DeleteConnectionAlias", map[string]any{
				"AliasId": aliasID,
			})
			if rec7.Code != http.StatusOK {
				t.Fatalf("delete: expected 200, got %d", rec7.Code)
			}
		})
	}
}

// TestDeleteConnectionAlias_StillAssociated verifies DeleteConnectionAlias
// rejects an alias still associated with a directory/resource: "You can
// delete a connection alias only after it is no longer shared with any
// accounts or associated with any directories" (api_op_DeleteConnectionAlias.go
// doc comment).
func TestDeleteConnectionAlias_StillAssociated(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandlerWithBackend(t)

	rec := doTargetRequest(t, h, "CreateConnectionAlias", map[string]any{
		"ConnectionString": "assoc.corp.example",
	})
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body)

	var createOut map[string]string
	decodeJSON(t, rec.Body.Bytes(), &createOut)
	aliasID := createOut["AliasId"]
	require.NotEmpty(t, aliasID)

	recAssoc := doTargetRequest(t, h, "AssociateConnectionAlias", map[string]any{
		"AliasId":    aliasID,
		"ResourceId": "res-still-associated",
	})
	require.Equal(t, http.StatusOK, recAssoc.Code, "body: %s", recAssoc.Body)

	recDelete := doTargetRequest(t, h, "DeleteConnectionAlias", map[string]any{
		"AliasId": aliasID,
	})
	assert.Equal(t, http.StatusBadRequest, recDelete.Code,
		"deleting an associated alias must fail: body: %s", recDelete.Body)
}

// TestDeleteConnectionAlias_StillShared verifies DeleteConnectionAlias
// rejects an alias still shared (AllowAssociation=true) with an account,
// matching the same real-AWS precondition as
// TestDeleteConnectionAlias_StillAssociated.
func TestDeleteConnectionAlias_StillShared(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandlerWithBackend(t)

	rec := doTargetRequest(t, h, "CreateConnectionAlias", map[string]any{
		"ConnectionString": "shared.corp.example",
	})
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body)

	var createOut map[string]string
	decodeJSON(t, rec.Body.Bytes(), &createOut)
	aliasID := createOut["AliasId"]
	require.NotEmpty(t, aliasID)

	recShare := doTargetRequest(t, h, "UpdateConnectionAliasPermission", map[string]any{
		"AliasId": aliasID,
		"ConnectionAliasPermission": map[string]any{
			"SharedAccountId":  "999988887777",
			"AllowAssociation": true,
		},
	})
	require.Equal(t, http.StatusOK, recShare.Code, "body: %s", recShare.Body)

	recDelete := doTargetRequest(t, h, "DeleteConnectionAlias", map[string]any{
		"AliasId": aliasID,
	})
	assert.Equal(t, http.StatusBadRequest, recDelete.Code,
		"deleting a shared alias must fail: body: %s", recDelete.Body)
}

// TestDescribeConnectionAliases_Pagination proves the op pages through every
// connection alias exactly once instead of returning them all on a single
// page with no cursor.
func TestDescribeConnectionAliases_Pagination(t *testing.T) {
	t.Parallel()

	client := newTestHandlerAndClient(t)
	ctx := t.Context()

	strs := []string{"alias-a.example.com", "alias-b.example.com", "alias-c.example.com"}
	for _, s := range strs {
		_, err := client.CreateConnectionAlias(ctx, &wssdk.CreateConnectionAliasInput{
			ConnectionString: aws.String(s),
		})
		require.NoError(t, err)
	}

	page1, err := client.DescribeConnectionAliases(ctx, &wssdk.DescribeConnectionAliasesInput{
		Limit: aws.Int32(2),
	})
	require.NoError(t, err)
	require.Len(t, page1.ConnectionAliases, 2)
	require.NotNil(t, page1.NextToken, "first page must return a cursor when more aliases remain")

	page2, err := client.DescribeConnectionAliases(ctx, &wssdk.DescribeConnectionAliasesInput{
		Limit:     aws.Int32(2),
		NextToken: page1.NextToken,
	})
	require.NoError(t, err)
	require.Len(t, page2.ConnectionAliases, 1)
	require.Empty(t, aws.ToString(page2.NextToken))

	seen := map[string]bool{}
	for _, a := range page1.ConnectionAliases {
		seen[aws.ToString(a.AliasId)] = true
	}

	for _, a := range page2.ConnectionAliases {
		id := aws.ToString(a.AliasId)
		require.False(t, seen[id], "alias %s returned on both pages", id)
		seen[id] = true
	}

	require.Len(t, seen, len(strs))
}

// TestDescribeConnectionAliasPermissions_Pagination proves the op pages
// through every shared-account permission exactly once instead of returning
// them all on a single page with no cursor.
func TestDescribeConnectionAliasPermissions_Pagination(t *testing.T) {
	t.Parallel()

	client := newTestHandlerAndClient(t)
	ctx := t.Context()

	createOut, err := client.CreateConnectionAlias(ctx, &wssdk.CreateConnectionAliasInput{
		ConnectionString: aws.String("perms.example.com"),
	})
	require.NoError(t, err)

	accounts := []string{"111111111111", "222222222222", "333333333333"}
	for _, acct := range accounts {
		_, updateErr := client.UpdateConnectionAliasPermission(ctx, &wssdk.UpdateConnectionAliasPermissionInput{
			AliasId: createOut.AliasId,
			ConnectionAliasPermission: &types.ConnectionAliasPermission{
				SharedAccountId:  aws.String(acct),
				AllowAssociation: aws.Bool(true),
			},
		})
		require.NoError(t, updateErr)
	}

	page1, err := client.DescribeConnectionAliasPermissions(ctx, &wssdk.DescribeConnectionAliasPermissionsInput{
		AliasId:    createOut.AliasId,
		MaxResults: aws.Int32(2),
	})
	require.NoError(t, err)
	require.Len(t, page1.ConnectionAliasPermissions, 2)
	require.NotNil(t, page1.NextToken, "first page must return a cursor when more permissions remain")

	page2, err := client.DescribeConnectionAliasPermissions(ctx, &wssdk.DescribeConnectionAliasPermissionsInput{
		AliasId:    createOut.AliasId,
		MaxResults: aws.Int32(2),
		NextToken:  page1.NextToken,
	})
	require.NoError(t, err)
	require.Len(t, page2.ConnectionAliasPermissions, 1)
	require.Empty(t, aws.ToString(page2.NextToken))

	seen := map[string]bool{}
	for _, p := range page1.ConnectionAliasPermissions {
		seen[aws.ToString(p.SharedAccountId)] = true
	}

	for _, p := range page2.ConnectionAliasPermissions {
		acct := aws.ToString(p.SharedAccountId)
		require.False(t, seen[acct], "account %s returned on both pages", acct)
		seen[acct] = true
	}

	require.Len(t, seen, len(accounts))
}

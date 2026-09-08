package workspaces_test

import (
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	wssdk "github.com/aws/aws-sdk-go-v2/service/workspaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIpGroupCRUD(t *testing.T) { //nolint:paralleltest // existing issue.
	tests := []struct {
		name      string
		groupName string
		rules     []map[string]string
	}{
		{
			name:      "simple group",
			groupName: "test-group",
			rules:     []map[string]string{{"ipRule": "10.0.0.0/8", "ruleDesc": "internal"}},
		},
		{
			name:      "empty rules group",
			groupName: "empty-group",
			rules:     nil,
		},
	}

	for _, tc := range tests { //nolint:paralleltest // existing issue.
		t.Run(tc.name, func(t *testing.T) {
			h, _ := newTestHandlerWithBackend(t)

			// Create
			rec := doTargetRequest(t, h, "CreateIpGroup", map[string]any{
				"GroupName": tc.groupName,
				"GroupDesc": "desc",
				"UserRules": tc.rules,
			})
			if rec.Code != http.StatusOK {
				t.Fatalf("create: expected 200, got %d: %s", rec.Code, rec.Body)
			}

			var createOut map[string]string
			decodeJSON(t, rec.Body.Bytes(), &createOut)

			groupID := createOut["GroupId"]
			if groupID == "" {
				t.Fatal("expected non-empty GroupId")
			}

			// Describe
			rec2 := doTargetRequest(t, h, "DescribeIpGroups", map[string]any{
				"GroupIds": []string{groupID},
			})
			if rec2.Code != http.StatusOK {
				t.Fatalf("describe: expected 200, got %d", rec2.Code)
			}

			var descOut struct {
				Result []map[string]any `json:"Result"`
			}
			decodeJSON(t, rec2.Body.Bytes(), &descOut)

			if len(descOut.Result) != 1 {
				t.Fatalf("expected 1 group, got %d", len(descOut.Result))
			}

			// Authorize rules
			rec3 := doTargetRequest(t, h, "AuthorizeIpRules", map[string]any{
				"GroupId":   groupID,
				"UserRules": []map[string]string{{"ipRule": "192.168.0.0/16", "ruleDesc": "extra"}},
			})
			if rec3.Code != http.StatusOK {
				t.Fatalf("authorize: expected 200, got %d", rec3.Code)
			}

			// Update rules
			rec4 := doTargetRequest(t, h, "UpdateRulesOfIpGroup", map[string]any{
				"GroupId":   groupID,
				"UserRules": []map[string]string{{"ipRule": "172.16.0.0/12", "ruleDesc": "new"}},
			})
			if rec4.Code != http.StatusOK {
				t.Fatalf("update rules: expected 200, got %d", rec4.Code)
			}

			// Revoke rules
			rec5 := doTargetRequest(t, h, "RevokeIpRules", map[string]any{
				"GroupId":   groupID,
				"UserRules": []string{"172.16.0.0/12"},
			})
			if rec5.Code != http.StatusOK {
				t.Fatalf("revoke: expected 200, got %d", rec5.Code)
			}

			// Associate with directory
			rec6 := doTargetRequest(t, h, "AssociateIpGroups", map[string]any{
				"DirectoryId": "d-123",
				"GroupIds":    []string{groupID},
			})
			if rec6.Code != http.StatusOK {
				t.Fatalf("associate: expected 200, got %d", rec6.Code)
			}

			// Disassociate
			rec7 := doTargetRequest(t, h, "DisassociateIpGroups", map[string]any{
				"DirectoryId": "d-123",
				"GroupIds":    []string{groupID},
			})
			if rec7.Code != http.StatusOK {
				t.Fatalf("disassociate: expected 200, got %d", rec7.Code)
			}

			// Delete
			rec8 := doTargetRequest(t, h, "DeleteIpGroup", map[string]any{
				"GroupId": groupID,
			})
			if rec8.Code != http.StatusOK {
				t.Fatalf("delete: expected 200, got %d", rec8.Code)
			}

			// Describe after delete — should be empty
			rec9 := doTargetRequest(t, h, "DescribeIpGroups", map[string]any{
				"GroupIds": []string{groupID},
			})
			var afterDelete struct {
				Result []any `json:"Result"`
			}
			decodeJSON(t, rec9.Body.Bytes(), &afterDelete)

			if len(afterDelete.Result) != 0 {
				t.Fatalf("expected 0 groups after delete, got %d", len(afterDelete.Result))
			}
		})
	}
}

// TestDescribeIpGroups_Pagination proves the op pages through every IP group
// exactly once instead of returning them all on a single page with no
// cursor.
func TestDescribeIpGroups_Pagination(t *testing.T) {
	t.Parallel()

	client := newTestHandlerAndClient(t)
	ctx := t.Context()

	names := []string{"group-a", "group-b", "group-c"}
	for _, n := range names {
		_, err := client.CreateIpGroup(ctx, &wssdk.CreateIpGroupInput{
			GroupName: aws.String(n),
		})
		require.NoError(t, err)
	}

	page1, err := client.DescribeIpGroups(ctx, &wssdk.DescribeIpGroupsInput{
		MaxResults: aws.Int32(2),
	})
	require.NoError(t, err)
	require.Len(t, page1.Result, 2)
	require.NotNil(t, page1.NextToken, "first page must return a cursor when more groups remain")

	page2, err := client.DescribeIpGroups(ctx, &wssdk.DescribeIpGroupsInput{
		MaxResults: aws.Int32(2),
		NextToken:  page1.NextToken,
	})
	require.NoError(t, err)
	require.Len(t, page2.Result, 1)
	require.Empty(t, aws.ToString(page2.NextToken))

	seen := map[string]bool{}
	for _, g := range page1.Result {
		seen[aws.ToString(g.GroupId)] = true
	}

	for _, g := range page2.Result {
		id := aws.ToString(g.GroupId)
		require.False(t, seen[id], "group %s returned on both pages", id)
		seen[id] = true
	}

	require.Len(t, seen, len(names))
}

// TestDeleteIpGroup_RejectedWhileAssociatedWithDirectory locks real AWS's
// DeleteIpGroup doc comment: "You cannot delete an IP access control group
// that is associated with a directory".
func TestDeleteIpGroup_RejectedWhileAssociatedWithDirectory(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandlerWithBackend(t)

	createRec := doTargetRequest(t, h, "CreateIpGroup", map[string]any{
		"GroupName": "assoc-group",
	})
	require.Equal(t, http.StatusOK, createRec.Code)

	var created struct {
		GroupID string `json:"GroupId"`
	}
	decodeJSON(t, createRec.Body.Bytes(), &created)
	require.NotEmpty(t, created.GroupID)

	assocRec := doTargetRequest(t, h, "AssociateIpGroups", map[string]any{
		"DirectoryId": "d-123",
		"GroupIds":    []string{created.GroupID},
	})
	require.Equal(t, http.StatusOK, assocRec.Code)

	delRec := doTargetRequest(t, h, "DeleteIpGroup", map[string]any{
		"GroupId": created.GroupID,
	})
	assert.Equal(t, http.StatusBadRequest, delRec.Code)
	assert.Contains(t, delRec.Body.String(), "InvalidResourceStateException")

	disassocRec := doTargetRequest(t, h, "DisassociateIpGroups", map[string]any{
		"DirectoryId": "d-123",
		"GroupIds":    []string{created.GroupID},
	})
	require.Equal(t, http.StatusOK, disassocRec.Code)

	delRec2 := doTargetRequest(t, h, "DeleteIpGroup", map[string]any{
		"GroupId": created.GroupID,
	})
	assert.Equal(t, http.StatusOK, delRec2.Code)
}

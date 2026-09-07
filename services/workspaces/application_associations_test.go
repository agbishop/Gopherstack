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

func TestApplicationAssociations(t *testing.T) { //nolint:paralleltest // existing issue.
	h, _ := newTestHandlerWithBackend(t)

	doTargetRequest(t, h, "RegisterWorkspaceDirectory", map[string]any{"DirectoryId": "d-test"})

	// Create a workspace first
	rec := doTargetRequest(t, h, "CreateWorkspaces", map[string]any{
		"Workspaces": []map[string]any{
			{
				"UserName":    "alice",
				"DirectoryId": "d-test",
				"BundleId":    "wsb-test",
			},
		},
	})
	var wsOut struct {
		PendingRequests []map[string]any `json:"PendingRequests"`
	}
	decodeJSON(t, rec.Body.Bytes(), &wsOut)

	wsID := wsOut.PendingRequests[0]["WorkspaceId"].(string)
	appID := "app-12345"

	// DescribeImageAssociations/DescribeBundleAssociations now validate that
	// ImageId/BundleId reference real resources (previously a stub that
	// ignored the input entirely), so exercise them against a real image and
	// a real Amazon-owned bundle rather than made-up IDs.
	imgRec := doTargetRequest(t, h, "CreateWorkspaceImage", map[string]any{
		"Name":        "img-for-assoc-test",
		"Description": "test",
		"WorkspaceId": wsID,
	})
	var imgOut struct {
		ImageID string `json:"ImageId"`
	}
	decodeJSON(t, imgRec.Body.Bytes(), &imgOut)

	tests := []struct {
		fn   func(t *testing.T)
		name string
	}{
		{
			name: "AssociateWorkspaceApplication",
			fn: func(t *testing.T) {
				t.Helper()
				r := doTargetRequest(t, h, "AssociateWorkspaceApplication", map[string]any{
					"WorkspaceId":   wsID,
					"ApplicationId": appID,
				})
				if r.Code != http.StatusOK {
					t.Fatalf("expected 200, got %d", r.Code)
				}
			},
		},
		{
			name: "DescribeWorkspaceAssociations",
			fn: func(t *testing.T) {
				t.Helper()
				// AssociatedResourceTypes is a real required field (smithy
				// `required` on DescribeWorkspaceAssociationsInput).
				r := doTargetRequest(t, h, "DescribeWorkspaceAssociations", map[string]any{
					"WorkspaceId":             wsID,
					"AssociatedResourceTypes": []string{"APPLICATION"},
				})
				if r.Code != http.StatusOK {
					t.Fatalf("expected 200, got %d: %s", r.Code, r.Body)
				}
			},
		},
		{
			name: "DescribeApplicationAssociations",
			fn: func(t *testing.T) {
				t.Helper()
				// AssociatedResourceTypes is a real required field (smithy
				// `required` on DescribeApplicationAssociationsInput).
				r := doTargetRequest(t, h, "DescribeApplicationAssociations", map[string]any{
					"ApplicationId":           appID,
					"AssociatedResourceTypes": []string{"WORKSPACE"},
				})
				if r.Code != http.StatusOK {
					t.Fatalf("expected 200, got %d: %s", r.Code, r.Body)
				}
			},
		},
		{
			name: "DeployWorkspaceApplications",
			fn: func(t *testing.T) {
				t.Helper()
				r := doTargetRequest(t, h, "DeployWorkspaceApplications", map[string]any{
					"WorkspaceId": wsID,
					"Force":       false,
				})
				if r.Code != http.StatusOK {
					t.Fatalf("expected 200, got %d", r.Code)
				}
			},
		},
		{
			name: "DescribeApplications",
			fn: func(t *testing.T) {
				t.Helper()
				r := doTargetRequest(t, h, "DescribeApplications", map[string]any{})
				if r.Code != http.StatusOK {
					t.Fatalf("expected 200, got %d", r.Code)
				}
			},
		},
		{
			name: "DescribeImageAssociations",
			fn: func(t *testing.T) {
				t.Helper()
				r := doTargetRequest(t, h, "DescribeImageAssociations", map[string]any{
					"ImageId":                 imgOut.ImageID,
					"AssociatedResourceTypes": []string{"APPLICATION"},
				})
				if r.Code != http.StatusOK {
					t.Fatalf("expected 200, got %d: %s", r.Code, r.Body)
				}
			},
		},
		{
			name: "DescribeBundleAssociations",
			fn: func(t *testing.T) {
				t.Helper()
				r := doTargetRequest(t, h, "DescribeBundleAssociations", map[string]any{
					"BundleId":                "wsb-bh8rsxt14",
					"AssociatedResourceTypes": []string{"APPLICATION"},
				})
				if r.Code != http.StatusOK {
					t.Fatalf("expected 200, got %d: %s", r.Code, r.Body)
				}
			},
		},
		{
			name: "DisassociateWorkspaceApplication",
			fn: func(t *testing.T) {
				t.Helper()
				r := doTargetRequest(t, h, "DisassociateWorkspaceApplication", map[string]any{
					"WorkspaceId":   wsID,
					"ApplicationId": appID,
				})
				if r.Code != http.StatusOK {
					t.Fatalf("expected 200, got %d", r.Code)
				}
			},
		},
	}

	for _, tc := range tests { //nolint:paralleltest // existing issue.
		t.Run(tc.name, func(t *testing.T) {
			tc.fn(t)
		})
	}
}

// TestDescribeImageAssociations_Validation and
// TestDescribeBundleAssociations_Validation verify the required-field and
// existence validation these ops previously skipped entirely (a stub that
// ignored ImageId/BundleId and always returned an empty 200).
func TestDescribeImageAssociations_Validation(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandlerWithBackend(t)

	t.Run("unknown ImageId is not found", func(t *testing.T) {
		t.Parallel()

		rec := doTargetRequest(t, h, "DescribeImageAssociations", map[string]any{
			"ImageId":                 "wsi-does-not-exist",
			"AssociatedResourceTypes": []string{"APPLICATION"},
		})
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body)
		}
	})

	t.Run("missing AssociatedResourceTypes is rejected", func(t *testing.T) {
		t.Parallel()

		// CopyWorkspaceImage validates SourceImageId against b.images when
		// SourceRegion matches this backend's own region, so use one
		// actually created rather than a made-up ID.
		imgRec := doTargetRequest(t, h, "CopyWorkspaceImage", map[string]any{
			"Name":          "img-for-validation",
			"SourceImageId": createImage(t, h),
			"SourceRegion":  "us-east-1",
		})
		var imgOut map[string]string
		decodeJSON(t, imgRec.Body.Bytes(), &imgOut)

		rec := doTargetRequest(t, h, "DescribeImageAssociations", map[string]any{
			"ImageId": imgOut["ImageId"],
		})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body)
		}
	})
}

func TestDescribeBundleAssociations_Validation(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandlerWithBackend(t)

	t.Run("unknown BundleId is not found", func(t *testing.T) {
		t.Parallel()

		rec := doTargetRequest(t, h, "DescribeBundleAssociations", map[string]any{
			"BundleId":                "wsb-does-not-exist",
			"AssociatedResourceTypes": []string{"APPLICATION"},
		})
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body)
		}
	})

	t.Run("missing AssociatedResourceTypes is rejected", func(t *testing.T) {
		t.Parallel()

		rec := doTargetRequest(t, h, "DescribeBundleAssociations", map[string]any{
			"BundleId": "wsb-bh8rsxt14",
		})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body)
		}
	})

	t.Run("Amazon-owned bundle is a valid target", func(t *testing.T) {
		t.Parallel()

		rec := doTargetRequest(t, h, "DescribeBundleAssociations", map[string]any{
			"BundleId":                "wsb-bh8rsxt14",
			"AssociatedResourceTypes": []string{"APPLICATION"},
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body)
		}

		var out struct {
			Associations []any `json:"Associations"`
		}
		decodeJSON(t, rec.Body.Bytes(), &out)
		if out.Associations == nil {
			t.Fatal("expected non-nil (possibly empty) Associations array")
		}
	})
}

// TestWorkspaceApplicationAssociations_Validation verifies the
// Associate/Disassociate/Deploy/Describe*Associations family validates a
// nonexistent WorkspaceId (previously accepted unconditionally and reported
// success) and the required AssociatedResourceTypes field, and that the wire
// shape uses the real "State" key/enum rather than the previous
// "AssociationStatus": "INSTALLED" (neither exists on the real
// WorkspaceResourceAssociation type). ApplicationId is deliberately NOT
// existence-checked: this backend never populates the read-only applications
// catalog (real AWS has no CreateApplication API), so requiring a match would
// make the whole family permanently unusable.
func TestWorkspaceApplicationAssociations_Validation(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandlerWithBackend(t)

	doTargetRequest(t, h, "RegisterWorkspaceDirectory", map[string]any{"DirectoryId": "d-appval"})
	recCreate := doTargetRequest(t, h, "CreateWorkspaces", map[string]any{
		"Workspaces": []map[string]any{
			{"UserName": "bob", "DirectoryId": "d-appval", "BundleId": "wsb-test"},
		},
	})
	require.Equal(t, http.StatusOK, recCreate.Code)

	var wsOut struct {
		PendingRequests []map[string]any `json:"PendingRequests"`
	}
	decodeJSON(t, recCreate.Body.Bytes(), &wsOut)
	require.Len(t, wsOut.PendingRequests, 1)
	wsID := wsOut.PendingRequests[0]["WorkspaceId"].(string)

	t.Run("associate rejects unknown workspace", func(t *testing.T) {
		t.Parallel()

		rec := doTargetRequest(t, h, "AssociateWorkspaceApplication", map[string]any{
			"WorkspaceId":   "ws-does-not-exist",
			"ApplicationId": "app-1",
		})
		require.Equal(t, http.StatusNotFound, rec.Code, "body: %s", rec.Body)
	})

	t.Run("disassociate rejects unknown workspace", func(t *testing.T) {
		t.Parallel()

		rec := doTargetRequest(t, h, "DisassociateWorkspaceApplication", map[string]any{
			"WorkspaceId":   "ws-does-not-exist",
			"ApplicationId": "app-1",
		})
		require.Equal(t, http.StatusNotFound, rec.Code, "body: %s", rec.Body)
	})

	t.Run("deploy rejects unknown workspace", func(t *testing.T) {
		t.Parallel()

		rec := doTargetRequest(t, h, "DeployWorkspaceApplications", map[string]any{
			"WorkspaceId": "ws-does-not-exist",
		})
		require.Equal(t, http.StatusNotFound, rec.Code, "body: %s", rec.Body)
	})

	t.Run("describe workspace associations rejects unknown workspace", func(t *testing.T) {
		t.Parallel()

		rec := doTargetRequest(t, h, "DescribeWorkspaceAssociations", map[string]any{
			"WorkspaceId":             "ws-does-not-exist",
			"AssociatedResourceTypes": []string{"APPLICATION"},
		})
		require.Equal(t, http.StatusNotFound, rec.Code, "body: %s", rec.Body)
	})

	t.Run("describe workspace associations requires resource types", func(t *testing.T) {
		t.Parallel()

		rec := doTargetRequest(t, h, "DescribeWorkspaceAssociations", map[string]any{
			"WorkspaceId": wsID,
		})
		require.Equal(t, http.StatusBadRequest, rec.Code, "body: %s", rec.Body)
	})

	t.Run("describe application associations requires resource types", func(t *testing.T) {
		t.Parallel()

		rec := doTargetRequest(t, h, "DescribeApplicationAssociations", map[string]any{
			"ApplicationId": "app-1",
		})
		require.Equal(t, http.StatusBadRequest, rec.Code, "body: %s", rec.Body)
	})

	t.Run("associate reports the real State enum, not a fabricated one", func(t *testing.T) {
		t.Parallel()

		rec := doTargetRequest(t, h, "AssociateWorkspaceApplication", map[string]any{
			"WorkspaceId":   wsID,
			"ApplicationId": "app-real-shape",
		})
		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body)

		var out struct {
			Association struct {
				State             string `json:"State"`
				AssociationStatus string `json:"AssociationStatus"`
			} `json:"Association"`
		}
		decodeJSON(t, rec.Body.Bytes(), &out)
		assert.Equal(t, "COMPLETED", out.Association.State)
		assert.Empty(
			t, out.Association.AssociationStatus,
			"AssociationStatus is not a field on the real WorkspaceResourceAssociation type",
		)
	})
}

// TestDescribeApplicationAssociations_Pagination proves the op pages through
// every workspace associated with an application exactly once instead of
// returning them all on a single page with no cursor.
func TestDescribeApplicationAssociations_Pagination(t *testing.T) {
	t.Parallel()

	client := newTestHandlerAndClient(t)
	ctx := t.Context()

	_, regErr := client.RegisterWorkspaceDirectory(ctx, &wssdk.RegisterWorkspaceDirectoryInput{
		DirectoryId:            aws.String("d-00000000"),
		WorkspaceDirectoryName: aws.String("dir"),
	})
	require.NoError(t, regErr)

	const appID = "app-pagination-test"

	users := []string{"alice", "bob", "carol"}
	for _, u := range users {
		out, err := client.CreateWorkspaces(ctx, &wssdk.CreateWorkspacesInput{
			Workspaces: []types.WorkspaceRequest{
				{
					BundleId:    aws.String("wsb-00000000"),
					DirectoryId: aws.String("d-00000000"),
					UserName:    aws.String(u),
				},
			},
		})
		require.NoError(t, err)
		require.Len(t, out.PendingRequests, 1)

		_, err = client.AssociateWorkspaceApplication(ctx, &wssdk.AssociateWorkspaceApplicationInput{
			WorkspaceId:   out.PendingRequests[0].WorkspaceId,
			ApplicationId: aws.String(appID),
		})
		require.NoError(t, err)
	}

	page1, err := client.DescribeApplicationAssociations(ctx, &wssdk.DescribeApplicationAssociationsInput{
		ApplicationId: aws.String(appID),
		AssociatedResourceTypes: []types.ApplicationAssociatedResourceType{
			types.ApplicationAssociatedResourceTypeWorkspace,
		},
		MaxResults: aws.Int32(2),
	})
	require.NoError(t, err)
	require.Len(t, page1.Associations, 2)
	require.NotNil(t, page1.NextToken, "first page must return a cursor when more associations remain")

	page2, err := client.DescribeApplicationAssociations(ctx, &wssdk.DescribeApplicationAssociationsInput{
		ApplicationId: aws.String(appID),
		AssociatedResourceTypes: []types.ApplicationAssociatedResourceType{
			types.ApplicationAssociatedResourceTypeWorkspace,
		},
		MaxResults: aws.Int32(2),
		NextToken:  page1.NextToken,
	})
	require.NoError(t, err)
	require.Len(t, page2.Associations, 1)
	require.Empty(t, aws.ToString(page2.NextToken))

	seen := map[string]bool{}
	for _, a := range page1.Associations {
		seen[aws.ToString(a.AssociatedResourceId)] = true
	}

	for _, a := range page2.Associations {
		id := aws.ToString(a.AssociatedResourceId)
		require.False(t, seen[id], "workspace %s returned on both pages", id)
		seen[id] = true
	}

	require.Len(t, seen, len(users))
}

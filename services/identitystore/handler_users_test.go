package identitystore_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/identitystore"
)

// TestUserCRUD exercises CreateUser, DescribeUser, ListUsers, UpdateUser and DeleteUser.
func TestUserCRUD(t *testing.T) {
	t.Parallel()

	tests := []struct {
		run  func(t *testing.T, h *identitystore.Handler)
		name string
	}{
		{
			name: "create_user",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				rec := doRequest(t, h, "CreateUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserName":        "john.doe",
					"DisplayName":     "John Doe",
					"Name": map[string]any{
						"GivenName":  "John",
						"FamilyName": "Doe",
					},
				})

				assert.Equal(t, http.StatusOK, rec.Code)

				resp := parseResponse(t, rec)
				assert.NotEmpty(t, resp["UserId"])
				assert.Equal(t, testStoreID, resp["IdentityStoreId"])
			},
		},
		{
			name: "describe_user",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				createRec := doRequest(t, h, "CreateUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserName":        "jane.doe",
					"DisplayName":     "Jane Doe",
				})
				require.Equal(t, http.StatusOK, createRec.Code)

				userID := parseResponse(t, createRec)["UserId"].(string)

				rec := doRequest(t, h, "DescribeUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserId":          userID,
				})

				assert.Equal(t, http.StatusOK, rec.Code)

				resp := parseResponse(t, rec)
				assert.Equal(t, userID, resp["UserId"])
				assert.Equal(t, "jane.doe", resp["UserName"])
				assert.Equal(t, "Jane Doe", resp["DisplayName"])
			},
		},
		{
			name: "describe_user_not_found",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				rec := doRequest(t, h, "DescribeUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserId":          "nonexistent",
				})

				assert.Equal(t, http.StatusNotFound, rec.Code)
			},
		},
		{
			name: "list_users",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				doRequest(t, h, "CreateUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserName":        "user1",
					"DisplayName":     "User One",
				})
				doRequest(t, h, "CreateUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserName":        "user2",
					"DisplayName":     "User Two",
				})

				rec := doRequest(t, h, "ListUsers", map[string]any{
					"IdentityStoreId": testStoreID,
				})

				assert.Equal(t, http.StatusOK, rec.Code)

				resp := parseResponse(t, rec)
				users, ok := resp["Users"].([]any)
				require.True(t, ok)
				assert.Len(t, users, 2)
			},
		},
		{
			name: "update_user",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				createRec := doRequest(t, h, "CreateUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserName":        "update.me",
					"DisplayName":     "Old Name",
				})
				require.Equal(t, http.StatusOK, createRec.Code)

				userID := parseResponse(t, createRec)["UserId"].(string)

				rec := doRequest(t, h, "UpdateUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserId":          userID,
					"Operations": []map[string]any{
						{"AttributePath": "displayName", "AttributeValue": "New Name"},
						{
							"AttributePath": "emails",
							"AttributeValue": []map[string]any{
								{"Value": "new.name@example.com", "Type": "work", "Primary": true},
							},
						},
						{
							"AttributePath": "phoneNumbers",
							"AttributeValue": []map[string]any{
								{"Value": "+1-555-0100", "Type": "work", "Primary": true},
							},
						},
						{
							"AttributePath": "addresses",
							"AttributeValue": []map[string]any{
								{"Formatted": "123 Main St, Metropolis", "Type": "work", "Primary": true},
							},
						},
					},
				})

				assert.Equal(t, http.StatusOK, rec.Code)

				descRec := doRequest(t, h, "DescribeUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserId":          userID,
				})
				resp := parseResponse(t, descRec)
				assert.Equal(t, "New Name", resp["DisplayName"])
				assert.Equal(
					t,
					[]any{map[string]any{
						"Value": "new.name@example.com", "Type": "work", "Primary": true,
					}},
					resp["Emails"],
				)
				assert.Equal(
					t,
					[]any{map[string]any{
						"Value": "+1-555-0100", "Type": "work", "Primary": true,
					}},
					resp["PhoneNumbers"],
				)
				assert.Equal(
					t,
					[]any{map[string]any{
						"Formatted": "123 Main St, Metropolis", "Type": "work", "Primary": true,
					}},
					resp["Addresses"],
				)
			},
		},
		{
			name: "delete_user",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				createRec := doRequest(t, h, "CreateUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserName":        "delete.me",
				})
				require.Equal(t, http.StatusOK, createRec.Code)

				userID := parseResponse(t, createRec)["UserId"].(string)

				rec := doRequest(t, h, "DeleteUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserId":          userID,
				})

				assert.Equal(t, http.StatusOK, rec.Code)

				descRec := doRequest(t, h, "DescribeUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserId":          userID,
				})
				assert.Equal(t, http.StatusNotFound, descRec.Code)
			},
		},
		{
			name: "get_user_id",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				createRec := doRequest(t, h, "CreateUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserName":        "lookup.me",
					"DisplayName":     "Lookup User",
				})
				require.Equal(t, http.StatusOK, createRec.Code)

				wantUserID := parseResponse(t, createRec)["UserId"].(string)

				rec := doRequest(t, h, "GetUserId", map[string]any{
					"IdentityStoreId": testStoreID,
					"AlternateIdentifier": map[string]any{
						"UniqueAttribute": map[string]any{
							"AttributePath":  "userName",
							"AttributeValue": "lookup.me",
						},
					},
				})

				assert.Equal(t, http.StatusOK, rec.Code)

				resp := parseResponse(t, rec)
				assert.Equal(t, wantUserID, resp["UserId"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.run(t, newTestHandler())
		})
	}
}

// TestUpdateUserAttributes verifies updating specific user name attributes.
func TestUpdateUserAttributes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		attrPath  string
		attrValue string
	}{
		{name: "update_nickname", attrPath: "nickName", attrValue: "nick"},
		{name: "update_title", attrPath: "title", attrValue: "Dr."},
		{name: "update_locale", attrPath: "locale", attrValue: "en-US"},
		{name: "update_preferredLanguage", attrPath: "preferredLanguage", attrValue: "English"},
		{name: "update_timezone", attrPath: "timezone", attrValue: "UTC"},
		{name: "update_userType", attrPath: "userType", attrValue: "employee"},
		{name: "update_name_givenName", attrPath: "name.givenName", attrValue: "Bob"},
		{name: "update_name_familyName", attrPath: "name.familyName", attrValue: "Smith"},
		{name: "update_name_middleName", attrPath: "name.middleName", attrValue: "M"},
		{name: "update_name_formatted", attrPath: "name.formatted", attrValue: "Bob M Smith"},
		{name: "update_name_honorificPrefix", attrPath: "name.honorificPrefix", attrValue: "Mr."},
		{name: "update_name_honorificSuffix", attrPath: "name.honorificSuffix", attrValue: "Jr."},
		{name: "update_profileUrl", attrPath: "profileUrl", attrValue: "http://example.com"},
		{name: "update_username", attrPath: "username", attrValue: "new.name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()

			createRec := doRequest(t, h, "CreateUser", map[string]any{
				"IdentityStoreId": testStoreID,
				"UserName":        "attr-user-" + tt.name,
				"DisplayName":     "Attr User",
			})
			require.Equal(t, http.StatusOK, createRec.Code)
			userID := parseResponse(t, createRec)["UserId"].(string)

			patchRec := doRequest(t, h, "UpdateUser", map[string]any{
				"IdentityStoreId": testStoreID,
				"UserId":          userID,
				"Operations": []map[string]any{
					{"AttributePath": tt.attrPath, "AttributeValue": tt.attrValue},
				},
			})
			assert.Equal(t, http.StatusOK, patchRec.Code)
		})
	}
}

// TestGetUserID_WithUniqueAttribute verifies GetUserId with UniqueAttribute.
func TestGetUserID_WithUniqueAttribute(t *testing.T) {
	t.Parallel()

	h := newTestHandler()

	createRec := doRequest(t, h, "CreateUser", map[string]any{
		"IdentityStoreId": testStoreID,
		"UserName":        "unique.user",
	})
	require.Equal(t, http.StatusOK, createRec.Code)

	rec := doRequest(t, h, "GetUserId", map[string]any{
		"IdentityStoreId": testStoreID,
		"AlternateIdentifier": map[string]any{
			"UniqueAttribute": map[string]any{
				"AttributePath":  "userName",
				"AttributeValue": "unique.user",
			},
		},
	})
	assert.Equal(t, http.StatusOK, rec.Code)
	resp := parseResponse(t, rec)
	assert.NotEmpty(t, resp["UserId"])
}

// TestUserErrors covers 404/409/400 error paths and required-field validation for User operations.
func TestUserErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		run  func(t *testing.T, h *identitystore.Handler)
		name string
	}{
		{
			name: "delete_nonexistent_user",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				rec := doRequest(t, h, "DeleteUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserId":          "does-not-exist",
				})
				assert.Equal(t, http.StatusNotFound, rec.Code)
			},
		},
		{
			name: "duplicate_user",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				doRequest(t, h, "CreateUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserName":        "dup.user",
				})

				rec := doRequest(t, h, "CreateUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserName":        "dup.user",
				})
				assert.Equal(t, http.StatusConflict, rec.Code)
			},
		},
		{
			// The real ConflictException shape carries a Reason enum
			// (UNIQUENESS_CONSTRAINT_VIOLATION or CONCURRENT_MODIFICATION --
			// see types/errors.go and types/enums.go in the aws-sdk-go-v2
			// identitystore package). aws-sdk-go-v2's
			// awsAwsjson11_deserializeDocumentConflictException parses a
			// top-level "Reason" field from the response body; a real SDK
			// caller needs it populated to distinguish the two cases.
			name: "duplicate_user_conflict_reports_uniqueness_reason",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				doRequest(t, h, "CreateUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserName":        "dup.reason.user",
				})

				rec := doRequest(t, h, "CreateUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserName":        "dup.reason.user",
				})
				require.Equal(t, http.StatusConflict, rec.Code)
				resp := parseResponse(t, rec)
				assert.Equal(t, "UNIQUENESS_CONSTRAINT_VIOLATION", resp["Reason"])
			},
		},
		{
			name: "create_user_missing_store_id",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				rec := doRequest(t, h, "CreateUser", map[string]any{
					"UserName":    "no-store",
					"DisplayName": "No Store",
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
		{
			name: "update_user_duplicate_username",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				r1 := doRequest(t, h, "CreateUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserName":        "dup-rename-src",
				})
				require.Equal(t, http.StatusOK, r1.Code)
				userID := parseResponse(t, r1)["UserId"].(string)

				r2 := doRequest(t, h, "CreateUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserName":        "dup-rename-target",
				})
				require.Equal(t, http.StatusOK, r2.Code)

				rec := doRequest(t, h, "UpdateUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserId":          userID,
					"Operations": []map[string]any{
						{"AttributePath": "username", "AttributeValue": "dup-rename-target"},
					},
				})
				assert.Equal(t, http.StatusConflict, rec.Code)
			},
		},
		{
			name: "create_user_with_full_profile",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				rec := doRequest(t, h, "CreateUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserName":        "full.profile",
					"DisplayName":     "Full Profile",
					"Emails": []map[string]any{
						{"Value": "full@example.com", "Type": "work", "Primary": true},
					},
					"PhoneNumbers": []map[string]any{
						{"Value": "+1-800-0000", "Type": "mobile", "Primary": true},
					},
					"Addresses": []map[string]any{
						{
							"Formatted":     "1 AWS Way",
							"StreetAddress": "1 AWS Way",
							"Locality":      "Seattle",
							"Region":        "WA",
							"PostalCode":    "98101",
							"Country":       "US",
							"Type":          "work",
							"Primary":       true,
						},
					},
					"Name": map[string]any{
						"GivenName":  "Full",
						"FamilyName": "Profile",
					},
				})
				assert.Equal(t, http.StatusOK, rec.Code)
				userID := parseResponse(t, rec)["UserId"].(string)

				descRec := doRequest(t, h, "DescribeUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserId":          userID,
				})
				resp := parseResponse(t, descRec)
				emails, ok := resp["Emails"].([]any)
				require.True(t, ok)
				require.Len(t, emails, 1)
				assert.Equal(t, "full@example.com", emails[0].(map[string]any)["Value"])
			},
		},
		{
			name: "get_user_id_by_email",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				createRec := doRequest(t, h, "CreateUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserName":        "email.lookup",
					"Emails": []map[string]any{
						{"Value": "lookup@example.com", "Type": "work", "Primary": true},
					},
				})
				require.Equal(t, http.StatusOK, createRec.Code)
				wantID := parseResponse(t, createRec)["UserId"].(string)

				rec := doRequest(t, h, "GetUserId", map[string]any{
					"IdentityStoreId": testStoreID,
					"AlternateIdentifier": map[string]any{
						"UniqueAttribute": map[string]any{
							"AttributePath":  "emails.value",
							"AttributeValue": "lookup@example.com",
						},
					},
				})
				assert.Equal(t, http.StatusOK, rec.Code)
				assert.Equal(t, wantID, parseResponse(t, rec)["UserId"])
			},
		},
		{
			name: "delete_user_removes_memberships",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				uRec := doRequest(t, h, "CreateUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserName":        "del.cascade.user",
				})
				require.Equal(t, http.StatusOK, uRec.Code)
				userID := parseResponse(t, uRec)["UserId"].(string)

				gRec := doRequest(t, h, "CreateGroup", map[string]any{
					"IdentityStoreId": testStoreID,
					"DisplayName":     "Del Cascade Group",
				})
				require.Equal(t, http.StatusOK, gRec.Code)
				groupID := parseResponse(t, gRec)["GroupId"].(string)

				mRec := doRequest(t, h, "CreateGroupMembership", map[string]any{
					"IdentityStoreId": testStoreID,
					"GroupId":         groupID,
					"MemberId":        map[string]any{"UserId": userID},
				})
				require.Equal(t, http.StatusOK, mRec.Code)
				membershipID := parseResponse(t, mRec)["MembershipId"].(string)

				delRec := doRequest(t, h, "DeleteUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserId":          userID,
				})
				assert.Equal(t, http.StatusOK, delRec.Code)

				descMem := doRequest(t, h, "DescribeGroupMembership", map[string]any{
					"IdentityStoreId": testStoreID,
					"MembershipId":    membershipID,
				})
				assert.Equal(t, http.StatusNotFound, descMem.Code)
			},
		},
		{
			name: "delete_user_cascades_via_inverted_index",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				uRec := doRequest(t, h, "CreateUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserName":        "cascade.inv.user",
				})
				require.Equal(t, http.StatusOK, uRec.Code)
				userID := parseResponse(t, uRec)["UserId"].(string)

				membershipIDs := make([]string, 0, 3)
				for i := range 3 {
					gRec := doRequest(t, h, "CreateGroup", map[string]any{
						"IdentityStoreId": testStoreID,
						"DisplayName":     fmt.Sprintf("Cascade Inv Group %d", i),
					})
					require.Equal(t, http.StatusOK, gRec.Code)
					groupID := parseResponse(t, gRec)["GroupId"].(string)

					mRec := doRequest(t, h, "CreateGroupMembership", map[string]any{
						"IdentityStoreId": testStoreID,
						"GroupId":         groupID,
						"MemberId":        map[string]any{"UserId": userID},
					})
					require.Equal(t, http.StatusOK, mRec.Code)
					membershipIDs = append(membershipIDs, parseResponse(t, mRec)["MembershipId"].(string))
				}

				// Delete the user — should cascade remove all 3 memberships via inverted index.
				delRec := doRequest(t, h, "DeleteUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserId":          userID,
				})
				require.Equal(t, http.StatusOK, delRec.Code)

				for _, mid := range membershipIDs {
					descMem := doRequest(t, h, "DescribeGroupMembership", map[string]any{
						"IdentityStoreId": testStoreID,
						"MembershipId":    mid,
					})
					assert.Equal(t, http.StatusNotFound, descMem.Code)
				}
			},
		},
		{
			name: "resource_not_found_has_resource_type_user",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				rec := doRequest(t, h, "DescribeUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserId":          "nonexistent-user",
				})
				require.Equal(t, http.StatusNotFound, rec.Code)

				resp := parseResponse(t, rec)
				assert.Equal(t, "ResourceNotFoundException", resp["__type"])
				assert.Equal(t, "USER", resp["ResourceType"])
			},
		},
		{
			name: "describe_user_missing_user_id",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				rec := doRequest(t, h, "DescribeUser", map[string]any{
					"IdentityStoreId": testStoreID,
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
		{
			name: "describe_user_missing_store_id",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				rec := doRequest(t, h, "DescribeUser", map[string]any{
					"UserId": "user-001",
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
		{
			name: "delete_user_missing_user_id",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				rec := doRequest(t, h, "DeleteUser", map[string]any{
					"IdentityStoreId": testStoreID,
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
		{
			name: "update_user_missing_store_id",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				rec := doRequest(t, h, "UpdateUser", map[string]any{
					"UserId":     "user-001",
					"Operations": []map[string]any{},
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
		{
			name: "update_user_missing_user_id",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				rec := doRequest(t, h, "UpdateUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"Operations":      []map[string]any{},
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
		{
			name: "get_user_id_missing_store_id",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				rec := doRequest(t, h, "GetUserId", map[string]any{
					"AlternateIdentifier": map[string]any{
						"UniqueAttribute": map[string]any{
							"AttributePath":  "userName",
							"AttributeValue": "someone",
						},
					},
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
		{
			name: "list_users_missing_store_id",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				rec := doRequest(t, h, "ListUsers", map[string]any{})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
		{
			name: "create_user_identity_store_id_bad_pattern",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				rec := doRequest(t, h, "CreateUser", map[string]any{
					"IdentityStoreId": "not-a-valid-store-id",
					"UserName":        "pattern.user",
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
		{
			name: "create_user_username_bad_pattern_has_space",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				rec := doRequest(t, h, "CreateUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserName":        "has a space",
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
		{
			name: "create_user_username_reserved_administrator",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				rec := doRequest(t, h, "CreateUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserName":        "Administrator",
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
		{
			name: "create_user_username_reserved_aws_administrators",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				rec := doRequest(t, h, "CreateUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserName":        "AWSAdministrators",
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
		{
			name: "update_user_username_reserved_name_rejected",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				createRec := doRequest(t, h, "CreateUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserName":        "reserved.rename.user",
				})
				require.Equal(t, http.StatusOK, createRec.Code)
				userID := parseResponse(t, createRec)["UserId"].(string)

				rec := doRequest(t, h, "UpdateUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserId":          userID,
					"Operations": []map[string]any{
						{"AttributePath": "username", "AttributeValue": "AWSAdministrators"},
					},
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
		{
			name: "create_user_duplicate_primary_email_conflict",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				first := doRequest(t, h, "CreateUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserName":        "dup.email.first",
					"Emails": []map[string]any{
						{"Value": "shared@example.com", "Primary": true},
					},
				})
				require.Equal(t, http.StatusOK, first.Code)

				rec := doRequest(t, h, "CreateUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserName":        "dup.email.second",
					"Emails": []map[string]any{
						{"Value": "shared@example.com", "Primary": true},
					},
				})
				assert.Equal(t, http.StatusConflict, rec.Code)
			},
		},
		{
			name: "update_user_primary_email_conflict_with_another_user",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				owner := doRequest(t, h, "CreateUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserName":        "email.owner",
					"Emails": []map[string]any{
						{"Value": "owned@example.com", "Primary": true},
					},
				})
				require.Equal(t, http.StatusOK, owner.Code)

				other := doRequest(t, h, "CreateUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserName":        "email.other",
				})
				require.Equal(t, http.StatusOK, other.Code)
				otherID := parseResponse(t, other)["UserId"].(string)

				rec := doRequest(t, h, "UpdateUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserId":          otherID,
					"Operations": []map[string]any{
						{
							"AttributePath": "emails",
							"AttributeValue": []map[string]any{
								{"Value": "owned@example.com", "Primary": true},
							},
						},
					},
				})
				assert.Equal(t, http.StatusConflict, rec.Code)
			},
		},
		{
			name: "create_user_nickname_bad_pattern_control_char",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				rec := doRequest(t, h, "CreateUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserName":        "control.char.user",
					"NickName":        "bad\x01char",
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
		{
			name: "update_user_externalids_bad_pattern",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				createRec := doRequest(t, h, "CreateUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserName":        "extid.badpattern.user",
				})
				require.Equal(t, http.StatusOK, createRec.Code)
				userID := parseResponse(t, createRec)["UserId"].(string)

				rec := doRequest(t, h, "UpdateUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserId":          userID,
					"Operations": []map[string]any{
						{
							"AttributePath": "externalids",
							"AttributeValue": []map[string]any{
								{"Issuer": "bad\x01issuer", "Id": "id-1"},
							},
						},
					},
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
		{
			name: "update_user_attribute_path_bad_pattern",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				createRec := doRequest(t, h, "CreateUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserName":        "badpath.user",
				})
				require.Equal(t, http.StatusOK, createRec.Code)
				userID := parseResponse(t, createRec)["UserId"].(string)

				rec := doRequest(t, h, "UpdateUser", map[string]any{
					"IdentityStoreId": testStoreID,
					"UserId":          userID,
					"Operations": []map[string]any{
						{"AttributePath": "nick name!", "AttributeValue": "x"},
					},
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.run(t, newTestHandler())
		})
	}
}

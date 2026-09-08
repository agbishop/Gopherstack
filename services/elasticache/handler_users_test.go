package elasticache_test

import (
	"net/http"
	"testing"

	elasticachesdk "github.com/aws/aws-sdk-go-v2/service/elasticache"
	elasticachetypes "github.com/aws/aws-sdk-go-v2/service/elasticache/types"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_DeleteUser(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup   func(t *testing.T, client *elasticachesdk.Client)
		name    string
		userID  string
		wantErr bool
	}{
		{
			name:   "user_not_in_group_succeeds",
			userID: "free-user",
			setup: func(t *testing.T, client *elasticachesdk.Client) {
				t.Helper()
				_, err := client.CreateUser(t.Context(), &elasticachesdk.CreateUserInput{
					UserId:             aws.String("free-user"),
					UserName:           aws.String("free-user"),
					Engine:             aws.String("redis"),
					AccessString:       aws.String("on ~* +@all"),
					NoPasswordRequired: aws.Bool(true),
				})
				require.NoError(t, err)
			},
		},
		{
			// api_op_DeleteUser.go: "The user will be removed from all user
			// groups and in turn removed from all replication groups" --
			// membership in a group does not block DeleteUser, it cascades.
			name:   "user_in_group_succeeds",
			userID: "grouped-user",
			setup: func(t *testing.T, client *elasticachesdk.Client) {
				t.Helper()
				_, err := client.CreateUser(t.Context(), &elasticachesdk.CreateUserInput{
					UserId:             aws.String("grouped-user"),
					UserName:           aws.String("grouped-user"),
					Engine:             aws.String("redis"),
					AccessString:       aws.String("on ~* +@all"),
					NoPasswordRequired: aws.Bool(true),
				})
				require.NoError(t, err)

				_, err = client.CreateUserGroup(t.Context(), &elasticachesdk.CreateUserGroupInput{
					UserGroupId: aws.String("owns-user"),
					Engine:      aws.String("redis"),
					UserIds:     []string{"grouped-user"},
				})
				require.NoError(t, err)
			},
		},
		{
			name:    "user_not_found_rejected",
			userID:  "no-such-user",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := newTestStack(t)

			if tt.setup != nil {
				tt.setup(t, client)
			}

			out, err := client.DeleteUser(t.Context(), &elasticachesdk.DeleteUserInput{
				UserId: aws.String(tt.userID),
			})

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.userID, aws.ToString(out.UserId))
		})
	}
}

// TestHandler_DeleteUser_CascadesFromUserGroup locks the cascade side of
// DeleteUser via a real SDK round trip: the deleted user must no longer
// appear in a user group's UserIds on a subsequent DescribeUserGroups.
func TestHandler_DeleteUser_CascadesFromUserGroup(t *testing.T) {
	t.Parallel()

	client := newTestStack(t)

	_, err := client.CreateUser(t.Context(), &elasticachesdk.CreateUserInput{
		UserId:             aws.String("cascade-user"),
		UserName:           aws.String("cascade-user"),
		Engine:             aws.String("redis"),
		AccessString:       aws.String("on ~* +@all"),
		NoPasswordRequired: aws.Bool(true),
	})
	require.NoError(t, err)

	_, err = client.CreateUserGroup(t.Context(), &elasticachesdk.CreateUserGroupInput{
		UserGroupId: aws.String("cascade-group"),
		Engine:      aws.String("redis"),
		UserIds:     []string{"cascade-user"},
	})
	require.NoError(t, err)

	_, err = client.DeleteUser(t.Context(), &elasticachesdk.DeleteUserInput{
		UserId: aws.String("cascade-user"),
	})
	require.NoError(t, err)

	out, err := client.DescribeUserGroups(t.Context(), &elasticachesdk.DescribeUserGroupsInput{
		UserGroupId: aws.String("cascade-group"),
	})
	require.NoError(t, err)
	require.Len(t, out.UserGroups, 1)
	assert.NotContains(t, out.UserGroups[0].UserIds, "cascade-user")
}

// ----------------------------------------
// CreateUserGroup: validate user IDs exist (AWS accuracy)
// ----------------------------------------

func TestHandler_DescribeUsers_FilterByID(t *testing.T) {
	t.Parallel()

	client := newTestStack(t)

	for _, id := range []string{"filter-u1", "filter-u2"} {
		_, err := client.CreateUser(t.Context(), &elasticachesdk.CreateUserInput{
			UserId:             aws.String(id),
			UserName:           aws.String(id),
			Engine:             aws.String("redis"),
			AccessString:       aws.String("on ~* +@all"),
			NoPasswordRequired: aws.Bool(true),
		})
		require.NoError(t, err)
	}

	out, err := client.DescribeUsers(t.Context(), &elasticachesdk.DescribeUsersInput{
		UserId: aws.String("filter-u1"),
	})
	require.NoError(t, err)
	require.Len(t, out.Users, 1)
	assert.Equal(t, "filter-u1", aws.ToString(out.Users[0].UserId))
}

// ----------------------------------------
// User — Modify AccessString
// ----------------------------------------

func TestHandler_ModifyUser_AccessString(t *testing.T) {
	t.Parallel()

	client := newTestStack(t)

	_, err := client.CreateUser(t.Context(), &elasticachesdk.CreateUserInput{
		UserId:             aws.String("mod-access-user"),
		UserName:           aws.String("mod-access-user"),
		Engine:             aws.String("redis"),
		AccessString:       aws.String("on ~* +@all"),
		NoPasswordRequired: aws.Bool(true),
	})
	require.NoError(t, err)

	out, err := client.ModifyUser(t.Context(), &elasticachesdk.ModifyUserInput{
		UserId:       aws.String("mod-access-user"),
		AccessString: aws.String("on ~cache:* +get +set"),
	})
	require.NoError(t, err)
	assert.Equal(t, "on ~cache:* +get +set", aws.ToString(out.AccessString))
}

// ----------------------------------------
// ServiceUpdate — describe
// ----------------------------------------

func TestHandler_Tags_OnUser(t *testing.T) {
	t.Parallel()

	client := newTestStack(t)

	out, err := client.CreateUser(t.Context(), &elasticachesdk.CreateUserInput{
		UserId:       aws.String("tag-user"),
		UserName:     aws.String("tagger"),
		AccessString: aws.String("on ~* +@all"),
		Engine:       aws.String("redis"),
	})
	require.NoError(t, err)

	arn := aws.ToString(out.ARN)
	require.NotEmpty(t, arn)

	_, err = client.AddTagsToResource(t.Context(), &elasticachesdk.AddTagsToResourceInput{
		ResourceName: aws.String(arn),
		Tags: []elasticachetypes.Tag{
			{Key: aws.String("team"), Value: aws.String("platform")},
		},
	})
	require.NoError(t, err)

	tagsOut, err := client.ListTagsForResource(t.Context(), &elasticachesdk.ListTagsForResourceInput{
		ResourceName: aws.String(arn),
	})
	require.NoError(t, err)
	require.Len(t, tagsOut.TagList, 1)
	assert.Equal(t, "team", aws.ToString(tagsOut.TagList[0].Key))
}

func TestDeleteUser(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup   func(t *testing.T, client *elasticachesdk.Client)
		name    string
		userID  string
		wantErr bool
	}{
		{
			name:   "success",
			userID: "user-del-1",
			setup: func(t *testing.T, client *elasticachesdk.Client) {
				t.Helper()
				_, err := client.CreateUser(t.Context(), &elasticachesdk.CreateUserInput{
					UserId:             aws.String("user-del-1"),
					UserName:           aws.String("user-del-1"),
					Engine:             aws.String("redis"),
					AccessString:       aws.String("on ~* +@all"),
					NoPasswordRequired: aws.Bool(true),
				})
				require.NoError(t, err)
			},
		},
		{
			name:    "not_found",
			userID:  "user-nonexistent",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := newTestStack(t)

			if tt.setup != nil {
				tt.setup(t, client)
			}

			out, err := client.DeleteUser(t.Context(), &elasticachesdk.DeleteUserInput{
				UserId: aws.String(tt.userID),
			})

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.userID, aws.ToString(out.UserId))
		})
	}
}

// ----------------------------------------
// DescribeUsers
// ----------------------------------------

func TestDescribeUsers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup     func(t *testing.T, client *elasticachesdk.Client)
		name      string
		userID    string
		wantCount int
		wantErr   bool
	}{
		{
			name:      "all_users",
			wantCount: 2,
			setup: func(t *testing.T, client *elasticachesdk.Client) {
				t.Helper()
				for _, id := range []string{"u1", "u2"} {
					_, err := client.CreateUser(t.Context(), &elasticachesdk.CreateUserInput{
						UserId:             aws.String(id),
						UserName:           aws.String(id),
						Engine:             aws.String("redis"),
						AccessString:       aws.String("on ~* +@all"),
						NoPasswordRequired: aws.Bool(true),
					})
					require.NoError(t, err)
				}
			},
		},
		{
			name:      "filter_by_id",
			userID:    "u3",
			wantCount: 1,
			setup: func(t *testing.T, client *elasticachesdk.Client) {
				t.Helper()
				_, err := client.CreateUser(t.Context(), &elasticachesdk.CreateUserInput{
					UserId:             aws.String("u3"),
					UserName:           aws.String("u3"),
					Engine:             aws.String("redis"),
					AccessString:       aws.String("on ~* +@all"),
					NoPasswordRequired: aws.Bool(true),
				})
				require.NoError(t, err)
			},
		},
		{
			name:    "not_found",
			userID:  "no-such-user",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := newTestStack(t)

			if tt.setup != nil {
				tt.setup(t, client)
			}

			input := &elasticachesdk.DescribeUsersInput{}
			if tt.userID != "" {
				input.UserId = aws.String(tt.userID)
			}

			out, err := client.DescribeUsers(t.Context(), input)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Len(t, out.Users, tt.wantCount)
		})
	}
}

// ----------------------------------------
// ModifyUser
// ----------------------------------------

func TestModifyUser(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(t *testing.T, client *elasticachesdk.Client)
		name         string
		userID       string
		accessString string
		wantErr      bool
	}{
		{
			name:         "success",
			userID:       "user-mod-1",
			accessString: "on ~* +@read",
			setup: func(t *testing.T, client *elasticachesdk.Client) {
				t.Helper()
				_, err := client.CreateUser(t.Context(), &elasticachesdk.CreateUserInput{
					UserId:             aws.String("user-mod-1"),
					UserName:           aws.String("user-mod-1"),
					Engine:             aws.String("redis"),
					AccessString:       aws.String("on ~* +@all"),
					NoPasswordRequired: aws.Bool(true),
				})
				require.NoError(t, err)
			},
		},
		{
			name:    "not_found",
			userID:  "no-such-user",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := newTestStack(t)

			if tt.setup != nil {
				tt.setup(t, client)
			}

			out, err := client.ModifyUser(t.Context(), &elasticachesdk.ModifyUserInput{
				UserId:             aws.String(tt.userID),
				AccessString:       aws.String(tt.accessString),
				NoPasswordRequired: aws.Bool(true),
			})

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.userID, aws.ToString(out.UserId))
		})
	}
}

// ----------------------------------------
// CreateUserGroup
// ----------------------------------------

func TestCreateUser(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup              func(t *testing.T, client *elasticachesdk.Client)
		name               string
		userID             string
		userName           string
		accessString       string
		engine             string
		noPasswordRequired bool
		wantErr            bool
	}{
		{
			name:         "success",
			userID:       "user1",
			userName:     "alice",
			accessString: "on ~* +@all",
			engine:       "redis",
		},
		{
			name:               "success_no_password",
			userID:             "user2",
			userName:           "bob",
			accessString:       "on ~* +@read",
			engine:             "redis",
			noPasswordRequired: true,
		},
		{
			name:   "already_exists",
			userID: "dup-user",
			setup: func(t *testing.T, client *elasticachesdk.Client) {
				t.Helper()
				_, err := client.CreateUser(t.Context(), &elasticachesdk.CreateUserInput{
					UserId:             aws.String("dup-user"),
					UserName:           aws.String("charlie"),
					AccessString:       aws.String("on ~* +@all"),
					Engine:             aws.String("redis"),
					NoPasswordRequired: aws.Bool(false),
				})
				require.NoError(t, err)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := newTestStack(t)

			if tt.setup != nil {
				tt.setup(t, client)
			}

			out, err := client.CreateUser(t.Context(), &elasticachesdk.CreateUserInput{
				UserId:             aws.String(tt.userID),
				UserName:           aws.String(tt.userName),
				AccessString:       aws.String(tt.accessString),
				Engine:             aws.String(tt.engine),
				NoPasswordRequired: aws.Bool(tt.noPasswordRequired),
			})

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.userID, aws.ToString(out.UserId))
			assert.Equal(t, tt.userName, aws.ToString(out.UserName))
			assert.NotEmpty(t, aws.ToString(out.ARN))
		})
	}
}

// TestHandler_CreateUser_AuthenticationWireShape locks the User response's
// Authentication struct (Type + PasswordCount) and UserGroupIds list --
// both part of the real elasticachetypes.User shape but previously entirely
// absent from the wire response (and a gopherstack-invented NoPasswordRequired
// output field was serialized in their place, which no real ElastiCache
// response has). Field-diffed against aws-sdk-go-v2's deserializer for the
// User document.
func TestHandler_CreateUser_AuthenticationWireShape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input             *elasticachesdk.CreateUserInput
		name              string
		wantType          elasticachetypes.AuthenticationType
		wantPasswordCount int32
	}{
		{
			name: "legacy_no_password_required",
			input: &elasticachesdk.CreateUserInput{
				UserId:             aws.String("auth-legacy-nopass"),
				UserName:           aws.String("auth-legacy-nopass"),
				AccessString:       aws.String("on ~* +@all"),
				Engine:             aws.String("redis"),
				NoPasswordRequired: aws.Bool(true),
			},
			wantType:          elasticachetypes.AuthenticationTypeNoPassword,
			wantPasswordCount: 0,
		},
		{
			name: "authentication_mode_iam",
			input: &elasticachesdk.CreateUserInput{
				UserId:       aws.String("auth-iam"),
				UserName:     aws.String("auth-iam"),
				AccessString: aws.String("on ~* +@all"),
				Engine:       aws.String("redis"),
				AuthenticationMode: &elasticachetypes.AuthenticationMode{
					Type: elasticachetypes.InputAuthenticationTypeIam,
				},
			},
			wantType:          elasticachetypes.AuthenticationTypeIam,
			wantPasswordCount: 0,
		},
		{
			name: "authentication_mode_password_two_passwords",
			input: &elasticachesdk.CreateUserInput{
				UserId:       aws.String("auth-pw2"),
				UserName:     aws.String("auth-pw2"),
				AccessString: aws.String("on ~* +@all"),
				Engine:       aws.String("redis"),
				AuthenticationMode: &elasticachetypes.AuthenticationMode{
					Type:      elasticachetypes.InputAuthenticationTypePassword,
					Passwords: []string{"a-very-long-password-1234567890", "another-long-password-0987654321"},
				},
			},
			wantType:          elasticachetypes.AuthenticationTypePassword,
			wantPasswordCount: 2,
		},
		{
			name: "legacy_passwords_without_explicit_mode",
			input: &elasticachesdk.CreateUserInput{
				UserId:       aws.String("auth-legacy-pw"),
				UserName:     aws.String("auth-legacy-pw"),
				AccessString: aws.String("on ~* +@all"),
				Engine:       aws.String("redis"),
				Passwords:    []string{"a-very-long-password-1234567890"},
			},
			wantType:          elasticachetypes.AuthenticationTypePassword,
			wantPasswordCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := newTestStack(t)

			out, err := client.CreateUser(t.Context(), tt.input)
			require.NoError(t, err)
			require.NotNil(t, out.Authentication)
			assert.Equal(t, tt.wantType, out.Authentication.Type)
			assert.Equal(t, tt.wantPasswordCount, aws.ToInt32(out.Authentication.PasswordCount))
			assert.Empty(t, out.UserGroupIds, "a freshly created user belongs to no groups")

			// DescribeUsers must echo the same Authentication shape back.
			desc, err := client.DescribeUsers(t.Context(), &elasticachesdk.DescribeUsersInput{
				UserId: tt.input.UserId,
			})
			require.NoError(t, err)
			require.Len(t, desc.Users, 1)
			require.NotNil(t, desc.Users[0].Authentication)
			assert.Equal(t, tt.wantType, desc.Users[0].Authentication.Type)
		})
	}
}

// TestHandler_CreateUser_PasswordCountOutOfRange locks AWS's 1-2 password
// limit (InvalidParameterValueException otherwise) for AuthenticationMode
// Type=password.
func TestHandler_CreateUser_PasswordCountOutOfRange(t *testing.T) {
	t.Parallel()

	client := newTestStack(t)

	_, err := client.CreateUser(t.Context(), &elasticachesdk.CreateUserInput{
		UserId:       aws.String("auth-too-many-pw"),
		UserName:     aws.String("auth-too-many-pw"),
		AccessString: aws.String("on ~* +@all"),
		Engine:       aws.String("redis"),
		AuthenticationMode: &elasticachetypes.AuthenticationMode{
			Type:      elasticachetypes.InputAuthenticationTypePassword,
			Passwords: []string{"password-one-long-enough", "password-two-long-enough", "password-three-long-enough"},
		},
	})
	require.Error(t, err)
	requireFault[elasticachetypes.InvalidParameterValueException](t, err)
	requireHTTPStatus(t, err, http.StatusBadRequest)
}

// TestHandler_ModifyUser_AppendAccessString locks AppendAccessString (adds
// to the existing ACL string rather than replacing it) and UserGroupIds
// reflecting group membership after CreateUserGroup.
func TestHandler_ModifyUser_AppendAccessString(t *testing.T) {
	t.Parallel()

	client := newTestStack(t)

	_, err := client.CreateUser(t.Context(), &elasticachesdk.CreateUserInput{
		UserId:             aws.String("append-user"),
		UserName:           aws.String("append-user"),
		AccessString:       aws.String("on ~key:* +get"),
		Engine:             aws.String("redis"),
		NoPasswordRequired: aws.Bool(true),
	})
	require.NoError(t, err)

	out, err := client.ModifyUser(t.Context(), &elasticachesdk.ModifyUserInput{
		UserId:             aws.String("append-user"),
		AppendAccessString: aws.String("+set"),
	})
	require.NoError(t, err)
	assert.Contains(t, aws.ToString(out.AccessString), "+set")
	assert.Contains(t, aws.ToString(out.AccessString), "+get")

	_, err = client.CreateUserGroup(t.Context(), &elasticachesdk.CreateUserGroupInput{
		UserGroupId: aws.String("append-user-group"),
		Engine:      aws.String("redis"),
		UserIds:     []string{"append-user"},
	})
	require.NoError(t, err)

	desc, err := client.DescribeUsers(t.Context(), &elasticachesdk.DescribeUsersInput{
		UserId: aws.String("append-user"),
	})
	require.NoError(t, err)
	require.Len(t, desc.Users, 1)
	assert.Equal(t, []string{"append-user-group"}, desc.Users[0].UserGroupIds)
}

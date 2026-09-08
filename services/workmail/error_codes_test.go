package workmail_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	workmailsdk "github.com/aws/aws-sdk-go-v2/service/workmail"
	"github.com/aws/aws-sdk-go-v2/service/workmail/types"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/workmail"
)

func newWorkMailOrg(t *testing.T, client *workmailsdk.Client) *string {
	t.Helper()

	org, err := client.CreateOrganization(t.Context(), &workmailsdk.CreateOrganizationInput{
		Alias: aws.String("org-" + uuid.NewString()[:8]),
	})
	require.NoError(t, err)

	return org.OrganizationId
}

// TestCreateUser_NameUnavailable drives a real workmail client's CreateUser
// twice with the same name. CreateUser's own error model
// (workmail@v1.39.4 deserializers.go
// awsAwsjson11_deserializeOpErrorCreateUser) defines NameAvailabilityException
// for a taken name, not the fabricated "EntityAlreadyExistsException" (a
// type this service's SDK doesn't define anywhere).
func TestCreateUser_NameUnavailable(t *testing.T) {
	t.Parallel()

	client := newWorkMailSDKClient(t, workmail.NewHandler(workmail.NewInMemoryBackend("000000000000", "us-east-1")))
	orgID := newWorkMailOrg(t, client)

	in := &workmailsdk.CreateUserInput{
		OrganizationId: orgID,
		Name:           aws.String("dup-user"),
		DisplayName:    aws.String("dup-user"),
	}

	_, err := client.CreateUser(t.Context(), in)
	require.NoError(t, err)

	_, err = client.CreateUser(t.Context(), in)
	require.Error(t, err)

	var apiErr *types.NameAvailabilityException
	require.ErrorAs(t, err, &apiErr, "expected a real NameAvailabilityException from the SDK deserializer")
}

// TestRegisterMailDomain_InUse drives a real workmail client's
// RegisterMailDomain twice for the same domain. RegisterMailDomain's own
// error model defines MailDomainInUseException for this, not the fabricated
// "EntityAlreadyExistsException".
func TestRegisterMailDomain_InUse(t *testing.T) {
	t.Parallel()

	client := newWorkMailSDKClient(t, workmail.NewHandler(workmail.NewInMemoryBackend("000000000000", "us-east-1")))
	orgID := newWorkMailOrg(t, client)

	in := &workmailsdk.RegisterMailDomainInput{
		OrganizationId: orgID,
		DomainName:     aws.String("dup-domain.example"),
	}

	_, err := client.RegisterMailDomain(t.Context(), in)
	require.NoError(t, err)

	_, err = client.RegisterMailDomain(t.Context(), in)
	require.Error(t, err)

	var apiErr *types.MailDomainInUseException
	require.ErrorAs(t, err, &apiErr, "expected a real MailDomainInUseException from the SDK deserializer")
}

// TestRegisterToWorkMail_EmailInUse drives a real workmail client's
// RegisterToWorkMail against an email address already assigned to a
// different entity. RegisterToWorkMail's own error model defines
// EmailAddressInUseException for this (matching its own doc: "The email
// address that you're trying to assign is already created for a different
// user, group, or resource"), not the fabricated "EntityAlreadyExistsException".
func TestRegisterToWorkMail_EmailInUse(t *testing.T) {
	t.Parallel()

	client := newWorkMailSDKClient(t, workmail.NewHandler(workmail.NewInMemoryBackend("000000000000", "us-east-1")))
	orgID := newWorkMailOrg(t, client)

	u1, err := client.CreateUser(t.Context(), &workmailsdk.CreateUserInput{
		OrganizationId: orgID,
		Name:           aws.String("user-one"),
		DisplayName:    aws.String("user-one"),
	})
	require.NoError(t, err)

	_, err = client.RegisterToWorkMail(t.Context(), &workmailsdk.RegisterToWorkMailInput{
		OrganizationId: orgID,
		EntityId:       u1.UserId,
		Email:          aws.String("shared@dup-domain.example"),
	})
	require.NoError(t, err)

	u2, err := client.CreateUser(t.Context(), &workmailsdk.CreateUserInput{
		OrganizationId: orgID,
		Name:           aws.String("user-two"),
		DisplayName:    aws.String("user-two"),
	})
	require.NoError(t, err)

	_, err = client.RegisterToWorkMail(t.Context(), &workmailsdk.RegisterToWorkMailInput{
		OrganizationId: orgID,
		EntityId:       u2.UserId,
		Email:          aws.String("shared@dup-domain.example"),
	})
	require.Error(t, err)

	var apiErr *types.EmailAddressInUseException
	require.ErrorAs(t, err, &apiErr, "expected a real EmailAddressInUseException from the SDK deserializer")
}

// TestRegisterToWorkMail_NoOpWhenAlreadyEnabled proves a second
// RegisterToWorkMail on an already-ENABLED user/group/resource leaves its
// email untouched, per the op's own doc (api_op_RegisterToWorkMail.go):
// "It performs no change if the user, group, or resource is enabled".
func TestRegisterToWorkMail_NoOpWhenAlreadyEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		create func(t *testing.T, client *workmailsdk.Client, orgID *string) *string
		email  func(t *testing.T, client *workmailsdk.Client, orgID, entityID *string) string
		name   string
	}{
		{
			name: "user",
			create: func(t *testing.T, client *workmailsdk.Client, orgID *string) *string {
				t.Helper()

				name := "noop-user-" + uuid.NewString()[:8]
				u, err := client.CreateUser(t.Context(), &workmailsdk.CreateUserInput{
					OrganizationId: orgID,
					Name:           aws.String(name),
					DisplayName:    aws.String(name),
				})
				require.NoError(t, err)

				return u.UserId
			},
			email: func(t *testing.T, client *workmailsdk.Client, orgID, entityID *string) string {
				t.Helper()

				d, err := client.DescribeUser(t.Context(), &workmailsdk.DescribeUserInput{
					OrganizationId: orgID,
					UserId:         entityID,
				})
				require.NoError(t, err)

				return aws.ToString(d.Email)
			},
		},
		{
			name: "group",
			create: func(t *testing.T, client *workmailsdk.Client, orgID *string) *string {
				t.Helper()

				g, err := client.CreateGroup(t.Context(), &workmailsdk.CreateGroupInput{
					OrganizationId: orgID,
					Name:           aws.String("noop-group-" + uuid.NewString()[:8]),
				})
				require.NoError(t, err)

				return g.GroupId
			},
			email: func(t *testing.T, client *workmailsdk.Client, orgID, entityID *string) string {
				t.Helper()

				d, err := client.DescribeGroup(t.Context(), &workmailsdk.DescribeGroupInput{
					OrganizationId: orgID,
					GroupId:        entityID,
				})
				require.NoError(t, err)

				return aws.ToString(d.Email)
			},
		},
		{
			name: "resource",
			create: func(t *testing.T, client *workmailsdk.Client, orgID *string) *string {
				t.Helper()

				r, err := client.CreateResource(t.Context(), &workmailsdk.CreateResourceInput{
					OrganizationId: orgID,
					Name:           aws.String("noop-resource-" + uuid.NewString()[:8]),
					Type:           types.ResourceTypeRoom,
				})
				require.NoError(t, err)

				return r.ResourceId
			},
			email: func(t *testing.T, client *workmailsdk.Client, orgID, entityID *string) string {
				t.Helper()

				d, err := client.DescribeResource(t.Context(), &workmailsdk.DescribeResourceInput{
					OrganizationId: orgID,
					ResourceId:     entityID,
				})
				require.NoError(t, err)

				return aws.ToString(d.Email)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			backend := workmail.NewInMemoryBackend("000000000000", "us-east-1")
			client := newWorkMailSDKClient(t, workmail.NewHandler(backend))
			orgID := newWorkMailOrg(t, client)

			entityID := tc.create(t, client, orgID)
			origEmail := "orig-" + uuid.NewString()[:8] + "@example.com"

			_, err := client.RegisterToWorkMail(t.Context(), &workmailsdk.RegisterToWorkMailInput{
				OrganizationId: orgID,
				EntityId:       entityID,
				Email:          aws.String(origEmail),
			})
			require.NoError(t, err)
			require.Equal(t, origEmail, tc.email(t, client, orgID, entityID))

			newEmail := "new-" + uuid.NewString()[:8] + "@example.com"
			_, err = client.RegisterToWorkMail(t.Context(), &workmailsdk.RegisterToWorkMailInput{
				OrganizationId: orgID,
				EntityId:       entityID,
				Email:          aws.String(newEmail),
			})
			require.NoError(t, err, "re-registering an already-ENABLED entity must be a no-op, not an error")

			assert.Equal(t, origEmail, tc.email(t, client, orgID, entityID),
				"already-ENABLED entity's email must not change on re-registration")
		})
	}
}

package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	accountsdk "github.com/aws/aws-sdk-go-v2/service/account"
	accounttypes "github.com/aws/aws-sdk-go-v2/service/account/types"
	smithy "github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createAccountClient returns an Account Management client pointed at the shared test container.
func createAccountClient(t *testing.T) *accountsdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return accountsdk.NewFromConfig(cfg, func(o *accountsdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// accountCleanupCtx returns a context for use inside t.Cleanup callbacks.
// t.Context() must not be used there: Go 1.24+ cancels it before cleanups run.
func accountCleanupCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}

// accountErrorCode extracts the smithy error code from err, or "" if err isn't one.
func accountErrorCode(err error) string {
	if apiErr, ok := errors.AsType[smithy.APIError](err); ok {
		return apiErr.ErrorCode()
	}

	return ""
}

// TestIntegration_Account_AlternateContactLifecycle drives put->get->delete
// for each of the three AlternateContactType values. Each type is an
// independent key in the backend, so the subtests are safe to run in
// parallel with each other.
func TestIntegration_Account_AlternateContactLifecycle(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	tests := []struct {
		contactType accounttypes.AlternateContactType
		name        string
	}{
		{name: "billing", contactType: accounttypes.AlternateContactTypeBilling},
		{name: "operations", contactType: accounttypes.AlternateContactTypeOperations},
		{name: "security", contactType: accounttypes.AlternateContactTypeSecurity},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			client := createAccountClient(t)

			_, err := client.PutAlternateContact(ctx, &accountsdk.PutAlternateContactInput{
				AlternateContactType: tt.contactType,
				EmailAddress:         aws.String("contact@example.com"),
				Name:                 aws.String("Integ Contact"),
				PhoneNumber:          aws.String("555-0100"),
				Title:                aws.String("Manager"),
			})
			require.NoError(t, err, "PutAlternateContact should succeed")

			t.Cleanup(func() {
				cctx, cancel := accountCleanupCtx()
				defer cancel()

				_, _ = client.DeleteAlternateContact(cctx, &accountsdk.DeleteAlternateContactInput{
					AlternateContactType: tt.contactType,
				})
			})

			getOut, err := client.GetAlternateContact(ctx, &accountsdk.GetAlternateContactInput{
				AlternateContactType: tt.contactType,
			})
			require.NoError(t, err, "GetAlternateContact should succeed")
			require.NotNil(t, getOut.AlternateContact)
			assert.Equal(t, "contact@example.com", aws.ToString(getOut.AlternateContact.EmailAddress))
			assert.Equal(t, "Integ Contact", aws.ToString(getOut.AlternateContact.Name))
			assert.Equal(t, tt.contactType, getOut.AlternateContact.AlternateContactType)

			_, err = client.DeleteAlternateContact(ctx, &accountsdk.DeleteAlternateContactInput{
				AlternateContactType: tt.contactType,
			})
			require.NoError(t, err, "DeleteAlternateContact should succeed")

			_, err = client.GetAlternateContact(ctx, &accountsdk.GetAlternateContactInput{
				AlternateContactType: tt.contactType,
			})
			require.Error(t, err, "GetAlternateContact should fail after delete")
			assert.Equal(t, "ResourceNotFoundException", accountErrorCode(err))
		})
	}
}

// TestIntegration_Account_GetGovCloudAccountInformation_NotLinked verifies
// the real AWS-documented behavior for a standard account with no linked
// GovCloud pair (see the API reference's example 3): ResourceNotFoundException.
// This backend never has a linked account regardless of what other tests in
// this file mutate, so it is safe to run in parallel with them.
func TestIntegration_Account_GetGovCloudAccountInformation_NotLinked(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createAccountClient(t)

	_, err := client.GetGovCloudAccountInformation(t.Context(), &accountsdk.GetGovCloudAccountInformationInput{})
	require.Error(t, err)
	assert.Equal(t, "ResourceNotFoundException", accountErrorCode(err))
}

// TestIntegration_Account_SingletonLifecycle drives every operation that
// reads or mutates this service's single, un-keyed account record (contact
// information, account name, regions, primary email). Unlike
// AlternateContact (keyed by type) or most other services' named resources,
// there is exactly one account here, so these subtests share state and run
// sequentially by design rather than in parallel with each other.
//
//nolint:tparallel // subtests share the single account, so they run in order
func TestIntegration_Account_SingletonLifecycle(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	ctx := t.Context()
	client := createAccountClient(t)

	t.Run("primary email status not found", func(t *testing.T) { //nolint:paralleltest // sequential by design
		_, err := client.GetPrimaryEmailUpdateStatus(ctx, &accountsdk.GetPrimaryEmailUpdateStatusInput{})
		require.Error(t, err)
		assert.Equal(t, "ResourceNotFoundException", accountErrorCode(err))
	})

	t.Run("contact information roundtrip", func(t *testing.T) { //nolint:paralleltest // sequential by design
		_, err := client.PutContactInformation(ctx, &accountsdk.PutContactInformationInput{
			ContactInformation: &accounttypes.ContactInformation{
				AddressLine1: aws.String("123 Main St"),
				City:         aws.String("Seattle"),
				CountryCode:  aws.String("US"),
				FullName:     aws.String("Jane Doe"),
				PhoneNumber:  aws.String("555-0300"),
				PostalCode:   aws.String("98101"),
			},
		})
		require.NoError(t, err, "PutContactInformation should succeed")

		getOut, err := client.GetContactInformation(ctx, &accountsdk.GetContactInformationInput{})
		require.NoError(t, err, "GetContactInformation should succeed")
		require.NotNil(t, getOut.ContactInformation)
		assert.Equal(t, "Jane Doe", aws.ToString(getOut.ContactInformation.FullName))
		assert.Equal(t, "Seattle", aws.ToString(getOut.ContactInformation.City))
	})

	// The SDK client validates ContactInformation's required fields itself
	// (validators.go) before ever building a request, so a missing-field
	// call never reaches the wire -- only require.Error is meaningful here.
	// The server-side ValidationException path this would otherwise prove is
	// covered directly by handler_test.go's
	// TestHandler_PutContactInformation_RequiredFields.
	t.Run("contact info missing field", func(t *testing.T) { //nolint:paralleltest // sequential by design
		_, err := client.PutContactInformation(ctx, &accountsdk.PutContactInformationInput{
			ContactInformation: &accounttypes.ContactInformation{
				City: aws.String("Seattle"),
			},
		})
		require.Error(t, err)
	})

	t.Run("account name and information", func(t *testing.T) { //nolint:paralleltest // sequential by design
		_, err := client.PutAccountName(ctx, &accountsdk.PutAccountNameInput{
			AccountName: aws.String("Integ Test Co"),
		})
		require.NoError(t, err, "PutAccountName should succeed")

		infoOut, err := client.GetAccountInformation(ctx, &accountsdk.GetAccountInformationInput{})
		require.NoError(t, err, "GetAccountInformation should succeed")
		assert.Equal(t, "Integ Test Co", aws.ToString(infoOut.AccountName))
		assert.NotEmpty(t, aws.ToString(infoOut.AccountId))
		assert.Equal(t, accounttypes.AccountStateActive, infoOut.AccountState)
		assert.NotNil(t, infoOut.AccountCreatedDate)
	})

	t.Run("list and get region status", func(t *testing.T) { //nolint:paralleltest // sequential by design
		listOut, err := client.ListRegions(ctx, &accountsdk.ListRegionsInput{})
		require.NoError(t, err, "ListRegions should succeed")
		assert.NotEmpty(t, listOut.Regions)

		statusOut, err := client.GetRegionOptStatus(ctx, &accountsdk.GetRegionOptStatusInput{
			RegionName: aws.String("us-east-1"),
		})
		require.NoError(t, err, "GetRegionOptStatus should succeed")
		assert.Equal(t, accounttypes.RegionOptStatusEnabledByDefault, statusOut.RegionOptStatus)
	})

	t.Run("enable and disable opt-in region", func(t *testing.T) { //nolint:paralleltest // sequential by design
		_, err := client.DisableRegion(ctx, &accountsdk.DisableRegionInput{
			RegionName: aws.String("ap-northeast-1"),
		})
		require.NoError(t, err, "DisableRegion should succeed")

		statusOut, err := client.GetRegionOptStatus(ctx, &accountsdk.GetRegionOptStatusInput{
			RegionName: aws.String("ap-northeast-1"),
		})
		require.NoError(t, err)
		assert.Equal(t, accounttypes.RegionOptStatusDisabled, statusOut.RegionOptStatus)

		_, err = client.EnableRegion(ctx, &accountsdk.EnableRegionInput{
			RegionName: aws.String("ap-northeast-1"),
		})
		require.NoError(t, err, "EnableRegion should succeed")

		statusOut, err = client.GetRegionOptStatus(ctx, &accountsdk.GetRegionOptStatusInput{
			RegionName: aws.String("ap-northeast-1"),
		})
		require.NoError(t, err)
		assert.Equal(t, accounttypes.RegionOptStatusEnabled, statusOut.RegionOptStatus)
	})

	t.Run("enable default region fails", func(t *testing.T) { //nolint:paralleltest // sequential by design
		_, err := client.EnableRegion(ctx, &accountsdk.EnableRegionInput{
			RegionName: aws.String("us-east-1"),
		})
		require.Error(t, err)
		assert.Equal(t, "ValidationException", accountErrorCode(err))
	})

	// GetPrimaryEmail/StartPrimaryEmailUpdate/AcceptPrimaryEmailUpdate are the
	// three account operations where AccountId is required (not optional, as
	// on every other op here) -- see PARITY.md. The SDK client validates this
	// itself, so it must be set.
	t.Run("primary email update flow", func(t *testing.T) { //nolint:paralleltest // sequential by design
		getOut, err := client.GetPrimaryEmail(ctx, &accountsdk.GetPrimaryEmailInput{
			AccountId: aws.String("000000000000"),
		})
		require.NoError(t, err, "GetPrimaryEmail should succeed")
		assert.NotEmpty(t, aws.ToString(getOut.PrimaryEmail))

		startOut, err := client.StartPrimaryEmailUpdate(ctx, &accountsdk.StartPrimaryEmailUpdateInput{
			AccountId:    aws.String("000000000000"),
			PrimaryEmail: aws.String("new-primary@example.com"),
		})
		require.NoError(t, err, "StartPrimaryEmailUpdate should succeed")
		assert.Equal(t, accounttypes.PrimaryEmailUpdateStatusPending, startOut.Status)

		statusOut, err := client.GetPrimaryEmailUpdateStatus(ctx, &accountsdk.GetPrimaryEmailUpdateStatusInput{})
		require.NoError(t, err, "GetPrimaryEmailUpdateStatus should succeed")
		assert.Equal(t, accounttypes.PrimaryEmailUpdateStatusPending, statusOut.Status)
		assert.NotNil(t, statusOut.UpdatedAt)

		acceptOut, err := client.AcceptPrimaryEmailUpdate(ctx, &accountsdk.AcceptPrimaryEmailUpdateInput{
			AccountId:    aws.String("000000000000"),
			Otp:          aws.String("123456"),
			PrimaryEmail: aws.String("new-primary@example.com"),
		})
		require.NoError(t, err, "AcceptPrimaryEmailUpdate should succeed")
		assert.Equal(t, accounttypes.PrimaryEmailUpdateStatusAccepted, acceptOut.Status)

		statusOut, err = client.GetPrimaryEmailUpdateStatus(ctx, &accountsdk.GetPrimaryEmailUpdateStatusInput{})
		require.NoError(t, err)
		assert.Equal(t, accounttypes.PrimaryEmailUpdateStatusAccepted, statusOut.Status)

		getOut, err = client.GetPrimaryEmail(ctx, &accountsdk.GetPrimaryEmailInput{
			AccountId: aws.String("000000000000"),
		})
		require.NoError(t, err)
		assert.Equal(t, "new-primary@example.com", aws.ToString(getOut.PrimaryEmail))
	})
}

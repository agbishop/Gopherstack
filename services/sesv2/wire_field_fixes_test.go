package sesv2_test

import (
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	sesv2sdk "github.com/aws/aws-sdk-go-v2/service/sesv2"
	sesv2types "github.com/aws/aws-sdk-go-v2/service/sesv2/types"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/services/sesv2"
)

func newRoundTripClient(t *testing.T, h *sesv2.Handler) *sesv2sdk.Client {
	t.Helper()

	e := echo.New()
	registry := service.NewRegistry()
	require.NoError(t, registry.Register(h))
	e.Use(service.NewServiceRouter(registry).RouteHandler())

	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)

	cfg, err := awscfg.LoadDefaultConfig(
		t.Context(),
		awscfg.WithRegion("us-east-1"),
		awscfg.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err)

	return sesv2sdk.NewFromConfig(cfg, func(o *sesv2sdk.Options) {
		o.BaseEndpoint = aws.String(srv.URL)
	})
}

// TestPutAccountDetails_UseCaseDescription proves gopherstack-101r's fix for
// handler_account.go's putAccountDetailsInput: the real PutAccountDetailsInput
// member is the deprecated UseCaseDescription (aws-sdk-go-v2/service/sesv2@v1.66.4
// api_op_PutAccountDetails.go:60-63); "UseCaseName" is not a real member. Drives
// the real typed client and asserts the value round-trips through GetAccount.
// Before the fix, the handler read only "UseCaseName" and a real client's
// UseCaseDescription was silently dropped.
func TestPutAccountDetails_UseCaseDescription(t *testing.T) {
	t.Parallel()

	h := sesv2.NewHandler(sesv2.NewInMemoryBackend())
	client := newRoundTripClient(t, h)
	ctx := t.Context()

	_, err := client.PutAccountDetails(ctx, &sesv2sdk.PutAccountDetailsInput{
		MailType:   sesv2types.MailTypeMarketing,
		WebsiteURL: aws.String("https://example.com"),
		//nolint:staticcheck // exercising the real, deprecated SDK member's round trip
		UseCaseDescription: aws.String("wire fix round trip"),
	})
	require.NoError(t, err)

	out, err := client.GetAccount(ctx, &sesv2sdk.GetAccountInput{})
	require.NoError(t, err)
	require.NotNil(t, out.Details)
	//nolint:staticcheck // exercising the real, deprecated SDK member's round trip
	require.Equal(t, "wire fix round trip", aws.ToString(out.Details.UseCaseDescription))
}

// TestListEmailIdentities_VerificationStatus proves gopherstack-6flj's fix:
// types.IdentityInfo (aws-sdk-go-v2/service/sesv2@v1.66.4 types/types.go:1659,
// deserializers.go:19400 case "VerificationStatus") declares
// VerificationStatus alongside IdentityName/IdentityType/SendingEnabled, but
// ListEmailIdentities' emailIdentitySummary never carried it even though the
// backend already tracks it (GetEmailIdentity surfaces the same field
// correctly) -- a real client's ListEmailIdentities always saw a zero-value
// VerificationStatus regardless of the identity's real state.
func TestListEmailIdentities_VerificationStatus(t *testing.T) {
	t.Parallel()

	h := sesv2.NewHandler(sesv2.NewInMemoryBackend())
	client := newRoundTripClient(t, h)
	ctx := t.Context()

	_, err := client.CreateEmailIdentity(ctx, &sesv2sdk.CreateEmailIdentityInput{
		EmailIdentity: aws.String("verification-status@example.com"),
	})
	require.NoError(t, err)

	out, err := client.ListEmailIdentities(ctx, &sesv2sdk.ListEmailIdentitiesInput{})
	require.NoError(t, err)
	require.Len(t, out.EmailIdentities, 1)
	require.Equal(t, sesv2types.VerificationStatusSuccess, out.EmailIdentities[0].VerificationStatus)
}

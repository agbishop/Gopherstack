package route53_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	route53sdk "github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/route53"
)

// TestUpdateHostedZoneFeatures_UnknownZone_RealClient covers a missing-error
// bug: UpdateHostedZoneFeatures never validated that HostedZoneId names a
// real zone -- it always returned success unconditionally, ignoring the
// path entirely. UpdateHostedZoneFeatures's own deserializer
// (awsRestxml_deserializeOpErrorUpdateHostedZoneFeatures, route53@v1.65.6
// deserializers.go) models NoSuchHostedZone for exactly this case.
func TestUpdateHostedZoneFeatures_UnknownZone_RealClient(t *testing.T) {
	t.Parallel()

	backend := route53.NewInMemoryBackend()
	client := newTestRoute53Client(t, route53.NewHandler(backend))
	ctx := t.Context()

	_, err := client.UpdateHostedZoneFeatures(ctx, &route53sdk.UpdateHostedZoneFeaturesInput{
		HostedZoneId:              aws.String("Z_NO_SUCH_ZONE"),
		EnableAcceleratedRecovery: aws.Bool(true),
	})
	require.Error(t, err)

	var nshz *route53types.NoSuchHostedZone
	require.ErrorAs(t, err, &nshz, "expected a real NoSuchHostedZone from the SDK deserializer")
}

// TestGetGeoLocation_NotFound_RealClient locks in GetGeoLocation's not-found
// wire code. gopherstack-ns7j's backendErrorTable sweep found this path
// calls xmlError directly with a hardcoded "NoSuchGeoLocation" string
// instead of going through ErrNoSuchGeoLocation/backendErrorTable like every
// other op in this package -- so that table row is dead (unreachable), even
// though the wire output it would produce is correct. Left unfixed
// (PARITY.md, gopherstack-ns7j): this test guards the wire code either way.
// GetGeoLocation's own deserializer (awsRestxml_deserializeOpErrorGetGeoLocation,
// route53@v1.65.6 deserializers.go) models NoSuchGeoLocation for this case.
func TestGetGeoLocation_NotFound_RealClient(t *testing.T) {
	t.Parallel()

	backend := route53.NewInMemoryBackend()
	client := newTestRoute53Client(t, route53.NewHandler(backend))
	ctx := t.Context()

	_, err := client.GetGeoLocation(ctx, &route53sdk.GetGeoLocationInput{
		ContinentCode: aws.String("XX"),
	})
	require.Error(t, err)

	var ngl *route53types.NoSuchGeoLocation
	require.ErrorAs(t, err, &ngl, "expected a real NoSuchGeoLocation from the SDK deserializer")
}

// TestGetGeoLocation_Found_RealClient is the succeeding-case counterpart to
// TestGetGeoLocation_NotFound_RealClient: it catches a too-broad fix that
// routes every GetGeoLocation call through ErrNoSuchGeoLocation regardless
// of whether the location matches.
func TestGetGeoLocation_Found_RealClient(t *testing.T) {
	t.Parallel()

	backend := route53.NewInMemoryBackend()
	client := newTestRoute53Client(t, route53.NewHandler(backend))
	ctx := t.Context()

	out, err := client.GetGeoLocation(ctx, &route53sdk.GetGeoLocationInput{
		ContinentCode: aws.String("EU"),
	})
	require.NoError(t, err)
	require.NotNil(t, out.GeoLocationDetails)
	require.Equal(t, "EU", aws.ToString(out.GeoLocationDetails.ContinentCode))
}

package networkmonitor_test

import (
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	networkmonitorsdk "github.com/aws/aws-sdk-go-v2/service/networkmonitor"
	"github.com/aws/aws-sdk-go-v2/service/networkmonitor/types"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/services/networkmonitor"
)

const networkmonitorTagsRTRegion = "us-east-1"

// newTestNetworkMonitorClient stands up the real aws-sdk-go-v2
// CloudWatchNetworkMonitor client against an httptest server running this
// package's Handler, wired through the same pkgs/service registry/router
// used in production.
func newTestNetworkMonitorClient(t *testing.T, h *networkmonitor.Handler) *networkmonitorsdk.Client {
	t.Helper()

	e := echo.New()
	registry := service.NewRegistry()
	require.NoError(t, registry.Register(h))
	e.Use(service.NewServiceRouter(registry).RouteHandler())

	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)

	cfg, err := awscfg.LoadDefaultConfig(
		t.Context(),
		awscfg.WithRegion(networkmonitorTagsRTRegion),
		awscfg.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err)

	return networkmonitorsdk.NewFromConfig(cfg, func(o *networkmonitorsdk.Options) {
		o.BaseEndpoint = aws.String(srv.URL)
	})
}

// TestCreateOpsWithTags_RoundTrip drives every networkmonitor Create* op
// whose real Input struct accepts Tags (networkmonitor@v1.16.4:
// api_op_CreateMonitor.go, api_op_CreateProbe.go, both
// `Tags map[string]string`) through the real SDK client and asserts
// ListTagsForResource sees what was supplied at creation (gopherstack-2mwl).
func TestCreateOpsWithTags_RoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup func(t *testing.T, client *networkmonitorsdk.Client) string
		name  string
	}{
		{
			name: "monitor",
			setup: func(t *testing.T, client *networkmonitorsdk.Client) string {
				t.Helper()
				out, err := client.CreateMonitor(t.Context(), &networkmonitorsdk.CreateMonitorInput{
					MonitorName: aws.String("tagged-monitor"),
					Tags:        map[string]string{"env": "prod"},
				})
				require.NoError(t, err)

				return aws.ToString(out.MonitorArn)
			},
		},
		{
			name: "probe",
			setup: func(t *testing.T, client *networkmonitorsdk.Client) string {
				t.Helper()
				_, err := client.CreateMonitor(t.Context(), &networkmonitorsdk.CreateMonitorInput{
					MonitorName: aws.String("monitor-for-probe"),
				})
				require.NoError(t, err)

				out, err := client.CreateProbe(t.Context(), &networkmonitorsdk.CreateProbeInput{
					MonitorName: aws.String("monitor-for-probe"),
					Probe: &types.ProbeInput{
						Destination:     aws.String("10.0.0.1"),
						DestinationPort: aws.Int32(80),
						Protocol:        types.ProtocolTcp,
						SourceArn:       aws.String("arn:aws:ec2:us-east-1:000000000000:subnet/subnet-tagtest"),
					},
					Tags: map[string]string{"env": "prod"},
				})
				require.NoError(t, err)

				return aws.ToString(out.ProbeArn)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			backend := networkmonitor.NewInMemoryBackend(networkmonitorTagsRTRegion, "000000000000")
			client := newTestNetworkMonitorClient(t, networkmonitor.NewHandler(backend))

			arn := tc.setup(t, client)
			require.NotEmpty(t, arn)

			got, err := client.ListTagsForResource(t.Context(), &networkmonitorsdk.ListTagsForResourceInput{
				ResourceArn: aws.String(arn),
			})
			require.NoError(t, err)
			assert.Equal(t, map[string]string{"env": "prod"}, got.Tags)
		})
	}
}

// TestFindResourceByARN_HTTP drives findResourceByARN's monitor/probe lookup
// (used by ListTagsForResource/TagResource/UntagResource) through the real
// SDK client over the HTTP router, covering the case a single-resource test
// would pass trivially against a lookup that just returns the first thing it
// finds: two same-typed resources present, and the ARN identifies the right
// one -- plus a clean not-found for an ARN naming no resource at all.
func TestFindResourceByARN_HTTP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(t *testing.T, client *networkmonitorsdk.Client) string
		wantTags map[string]string
		name     string
		wantErr  bool
	}{
		{
			name: "monitor",
			setup: func(t *testing.T, client *networkmonitorsdk.Client) string {
				t.Helper()
				out, err := client.CreateMonitor(t.Context(), &networkmonitorsdk.CreateMonitorInput{
					MonitorName: aws.String("solo-monitor"),
					Tags:        map[string]string{"who": "solo"},
				})
				require.NoError(t, err)

				return aws.ToString(out.MonitorArn)
			},
			wantTags: map[string]string{"who": "solo"},
		},
		{
			name: "two monitors",
			setup: func(t *testing.T, client *networkmonitorsdk.Client) string {
				t.Helper()
				_, err := client.CreateMonitor(t.Context(), &networkmonitorsdk.CreateMonitorInput{
					MonitorName: aws.String("mon-a"),
					Tags:        map[string]string{"who": "a"},
				})
				require.NoError(t, err)

				out, err := client.CreateMonitor(t.Context(), &networkmonitorsdk.CreateMonitorInput{
					MonitorName: aws.String("mon-b"),
					Tags:        map[string]string{"who": "b"},
				})
				require.NoError(t, err)

				return aws.ToString(out.MonitorArn)
			},
			wantTags: map[string]string{"who": "b"},
		},
		{
			name: "two probes",
			setup: func(t *testing.T, client *networkmonitorsdk.Client) string {
				t.Helper()
				_, err := client.CreateMonitor(t.Context(), &networkmonitorsdk.CreateMonitorInput{
					MonitorName: aws.String("multi-probe-monitor"),
				})
				require.NoError(t, err)

				_, err = client.CreateProbe(t.Context(), &networkmonitorsdk.CreateProbeInput{
					MonitorName: aws.String("multi-probe-monitor"),
					Probe: &types.ProbeInput{
						Destination: aws.String("10.0.0.1"),
						Protocol:    types.ProtocolIcmp,
						SourceArn:   aws.String("arn:aws:ec2:us-east-1:000000000000:subnet/subnet-a"),
					},
					Tags: map[string]string{"who": "probe-a"},
				})
				require.NoError(t, err)

				out, err := client.CreateProbe(t.Context(), &networkmonitorsdk.CreateProbeInput{
					MonitorName: aws.String("multi-probe-monitor"),
					Probe: &types.ProbeInput{
						Destination: aws.String("10.0.0.2"),
						Protocol:    types.ProtocolIcmp,
						SourceArn:   aws.String("arn:aws:ec2:us-east-1:000000000000:subnet/subnet-b"),
					},
					Tags: map[string]string{"who": "probe-b"},
				})
				require.NoError(t, err)

				return aws.ToString(out.ProbeArn)
			},
			wantTags: map[string]string{"who": "probe-b"},
		},
		{
			name: "unknown arn",
			setup: func(t *testing.T, client *networkmonitorsdk.Client) string {
				t.Helper()
				_, err := client.CreateMonitor(t.Context(), &networkmonitorsdk.CreateMonitorInput{
					MonitorName: aws.String("real-monitor"),
				})
				require.NoError(t, err)

				return "arn:aws:networkmonitor:us-east-1:000000000000:monitor/does-not-exist"
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			backend := networkmonitor.NewInMemoryBackend(networkmonitorTagsRTRegion, "000000000000")
			client := newTestNetworkMonitorClient(t, networkmonitor.NewHandler(backend))

			arn := tc.setup(t, client)
			require.NotEmpty(t, arn)

			got, err := client.ListTagsForResource(t.Context(), &networkmonitorsdk.ListTagsForResourceInput{
				ResourceArn: aws.String(arn),
			})

			if tc.wantErr {
				require.Error(t, err)

				var nf *types.ResourceNotFoundException
				require.ErrorAs(t, err, &nf, "expected a real ResourceNotFoundException from the SDK deserializer")

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantTags, got.Tags)
		})
	}
}

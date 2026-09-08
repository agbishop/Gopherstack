package elb_test

import (
	"context"
	"encoding/xml"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/elb"
)

// TestModifyLoadBalancerAttributes tests modifying LB attributes.
func TestModifyLoadBalancerAttributes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(t *testing.T, h *elb.Handler)
		vals       url.Values
		name       string
		wantXZLB   string
		wantStatus int
	}{
		{
			name: "sets_cross_zone_and_idle_timeout",
			setup: func(t *testing.T, h *elb.Handler) {
				t.Helper()
				mustCreateLB(t, h, "attrs-lb")
			},
			vals: url.Values{
				"Action":           {"ModifyLoadBalancerAttributes"},
				"Version":          {"2012-06-01"},
				"LoadBalancerName": {"attrs-lb"},
				"LoadBalancerAttributes.CrossZoneLoadBalancing.Enabled":      {"true"},
				"LoadBalancerAttributes.ConnectionDraining.Enabled":          {"false"},
				"LoadBalancerAttributes.ConnectionDraining.Timeout":          {"300"},
				"LoadBalancerAttributes.ConnectionSettings.IdleTimeout":      {"120"},
				"LoadBalancerAttributes.AdditionalAttributes.member.1.Key":   {"elb.http.desyncmitigationmode"},
				"LoadBalancerAttributes.AdditionalAttributes.member.1.Value": {"monitor"},
			},
			wantStatus: http.StatusOK,
			wantXZLB:   "true",
		},
		{
			name: "lb_not_found",
			vals: url.Values{
				"Action":           {"ModifyLoadBalancerAttributes"},
				"Version":          {"2012-06-01"},
				"LoadBalancerName": {"no-such-lb"},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "missing_lb_name",
			vals: url.Values{
				"Action":  {"ModifyLoadBalancerAttributes"},
				"Version": {"2012-06-01"},
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			if tt.setup != nil {
				tt.setup(t, h)
			}

			rec := doELB(t, h, tt.vals)

			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantXZLB != "" {
				var resp struct {
					XMLName xml.Name `xml:"ModifyLoadBalancerAttributesResponse"`
					Result  struct {
						LoadBalancerAttributes struct {
							CrossZoneLoadBalancing struct {
								Enabled string `xml:"Enabled"`
							} `xml:"CrossZoneLoadBalancing"`
						} `xml:"LoadBalancerAttributes"`
					} `xml:"ModifyLoadBalancerAttributesResult"`
				}
				parseXMLBody(t, rec, &resp)
				assert.Equal(t, tt.wantXZLB, resp.Result.LoadBalancerAttributes.CrossZoneLoadBalancing.Enabled)
			}
		})
	}
}

// TestDescribeLoadBalancerAttributes tests reading LB attributes.
func TestDescribeLoadBalancerAttributes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(t *testing.T, h *elb.Handler)
		vals       url.Values
		name       string
		wantStatus int
		checkResp  bool
	}{
		{
			name: "returns_default_attributes",
			setup: func(t *testing.T, h *elb.Handler) {
				t.Helper()
				mustCreateLB(t, h, "descattrs-lb")
			},
			vals: url.Values{
				"Action":           {"DescribeLoadBalancerAttributes"},
				"Version":          {"2012-06-01"},
				"LoadBalancerName": {"descattrs-lb"},
			},
			wantStatus: http.StatusOK,
			checkResp:  true,
		},
		{
			name: "lb_not_found",
			vals: url.Values{
				"Action":           {"DescribeLoadBalancerAttributes"},
				"Version":          {"2012-06-01"},
				"LoadBalancerName": {"no-such-lb"},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "missing_lb_name",
			vals: url.Values{
				"Action":  {"DescribeLoadBalancerAttributes"},
				"Version": {"2012-06-01"},
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			if tt.setup != nil {
				tt.setup(t, h)
			}

			rec := doELB(t, h, tt.vals)

			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.checkResp {
				var resp struct {
					XMLName xml.Name `xml:"DescribeLoadBalancerAttributesResponse"`
					Result  struct {
						LoadBalancerAttributes struct {
							ConnectionSettings struct {
								IdleTimeout string `xml:"IdleTimeout"`
							} `xml:"ConnectionSettings"`
						} `xml:"LoadBalancerAttributes"`
					} `xml:"DescribeLoadBalancerAttributesResult"`
				}
				parseXMLBody(t, rec, &resp)
				assert.Equal(t, "60", resp.Result.LoadBalancerAttributes.ConnectionSettings.IdleTimeout)
			}
		})
	}
}

func TestAccessLogDefaultDisabled(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	mustCreateLB(t, h, "al-default-lb")

	rec := doELB(t, h, url.Values{
		"Action":           {"DescribeLoadBalancerAttributes"},
		"Version":          {"2012-06-01"},
		"LoadBalancerName": {"al-default-lb"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		XMLName xml.Name `xml:"DescribeLoadBalancerAttributesResponse"`
		Result  struct {
			LoadBalancerAttributes struct {
				AccessLog struct {
					Enabled string `xml:"Enabled"`
				} `xml:"AccessLog"`
			} `xml:"LoadBalancerAttributes"`
		} `xml:"DescribeLoadBalancerAttributesResult"`
	}
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "false", resp.Result.LoadBalancerAttributes.AccessLog.Enabled)
}

func TestAccessLogEnable(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	mustCreateLB(t, h, "al-enable-lb")

	rec := doELB(t, h, url.Values{
		"Action":           {"ModifyLoadBalancerAttributes"},
		"Version":          {"2012-06-01"},
		"LoadBalancerName": {"al-enable-lb"},
		"LoadBalancerAttributes.ConnectionSettings.IdleTimeout": {"60"},
		"LoadBalancerAttributes.AccessLog.Enabled":              {"true"},
		"LoadBalancerAttributes.AccessLog.S3BucketName":         {"my-elb-logs"},
		"LoadBalancerAttributes.AccessLog.S3BucketPrefix":       {"logs/"},
		"LoadBalancerAttributes.AccessLog.EmitInterval":         {"60"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		XMLName xml.Name `xml:"ModifyLoadBalancerAttributesResponse"`
		Result  struct {
			LoadBalancerAttributes struct {
				AccessLog struct {
					Enabled        string `xml:"Enabled"`
					S3BucketName   string `xml:"S3BucketName"`
					S3BucketPrefix string `xml:"S3BucketPrefix"`
					EmitInterval   string `xml:"EmitInterval"`
				} `xml:"AccessLog"`
			} `xml:"LoadBalancerAttributes"`
		} `xml:"ModifyLoadBalancerAttributesResult"`
	}
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
	al := resp.Result.LoadBalancerAttributes.AccessLog
	assert.Equal(t, "true", al.Enabled)
	assert.Equal(t, "my-elb-logs", al.S3BucketName)
	assert.Equal(t, "logs/", al.S3BucketPrefix)
	assert.Equal(t, "60", al.EmitInterval)
}

func TestAccessLogEnable5MinInterval(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	mustCreateLB(t, h, "al-5min-lb")

	rec := doELB(t, h, url.Values{
		"Action":           {"ModifyLoadBalancerAttributes"},
		"Version":          {"2012-06-01"},
		"LoadBalancerName": {"al-5min-lb"},
		"LoadBalancerAttributes.ConnectionSettings.IdleTimeout": {"60"},
		"LoadBalancerAttributes.AccessLog.Enabled":              {"true"},
		"LoadBalancerAttributes.AccessLog.S3BucketName":         {"my-bucket"},
		"LoadBalancerAttributes.AccessLog.EmitInterval":         {"5"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		XMLName xml.Name `xml:"ModifyLoadBalancerAttributesResponse"`
		Result  struct {
			LoadBalancerAttributes struct {
				AccessLog struct {
					EmitInterval string `xml:"EmitInterval"`
				} `xml:"AccessLog"`
			} `xml:"LoadBalancerAttributes"`
		} `xml:"ModifyLoadBalancerAttributesResult"`
	}
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "5", resp.Result.LoadBalancerAttributes.AccessLog.EmitInterval)
}

func TestAccessLogInvalidInterval(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		emitInterval string
		wantStatus   int
	}{
		{name: "interval_30_rejected", emitInterval: "30", wantStatus: http.StatusBadRequest},
		{name: "interval_1_rejected", emitInterval: "1", wantStatus: http.StatusBadRequest},
		{name: "interval_120_rejected", emitInterval: "120", wantStatus: http.StatusBadRequest},
		{name: "interval_5_accepted", emitInterval: "5", wantStatus: http.StatusOK},
		{name: "interval_60_accepted", emitInterval: "60", wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			mustCreateLB(t, h, "al-inv-lb")

			rec := doELB(t, h, url.Values{
				"Action":           {"ModifyLoadBalancerAttributes"},
				"Version":          {"2012-06-01"},
				"LoadBalancerName": {"al-inv-lb"},
				"LoadBalancerAttributes.ConnectionSettings.IdleTimeout": {"60"},
				"LoadBalancerAttributes.AccessLog.Enabled":              {"true"},
				"LoadBalancerAttributes.AccessLog.S3BucketName":         {"my-bucket"},
				"LoadBalancerAttributes.AccessLog.EmitInterval":         {tt.emitInterval},
			})
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestAccessLogEnabledRequiresBucket(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	mustCreateLB(t, h, "al-nobucket-lb")

	rec := doELB(t, h, url.Values{
		"Action":           {"ModifyLoadBalancerAttributes"},
		"Version":          {"2012-06-01"},
		"LoadBalancerName": {"al-nobucket-lb"},
		"LoadBalancerAttributes.ConnectionSettings.IdleTimeout": {"60"},
		"LoadBalancerAttributes.AccessLog.Enabled":              {"true"},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAccessLogDisableDoesNotRequireBucket(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	mustCreateLB(t, h, "al-disable-lb")

	rec := doELB(t, h, url.Values{
		"Action":           {"ModifyLoadBalancerAttributes"},
		"Version":          {"2012-06-01"},
		"LoadBalancerName": {"al-disable-lb"},
		"LoadBalancerAttributes.ConnectionSettings.IdleTimeout": {"60"},
		"LoadBalancerAttributes.AccessLog.Enabled":              {"false"},
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAccessLogRoundTripDescribeAttributes(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	mustCreateLB(t, h, "al-rt-lb")

	doELB(t, h, url.Values{
		"Action":           {"ModifyLoadBalancerAttributes"},
		"Version":          {"2012-06-01"},
		"LoadBalancerName": {"al-rt-lb"},
		"LoadBalancerAttributes.ConnectionSettings.IdleTimeout": {"60"},
		"LoadBalancerAttributes.AccessLog.Enabled":              {"true"},
		"LoadBalancerAttributes.AccessLog.S3BucketName":         {"rt-bucket"},
		"LoadBalancerAttributes.AccessLog.S3BucketPrefix":       {"prefix/"},
		"LoadBalancerAttributes.AccessLog.EmitInterval":         {"5"},
	})

	rec := doELB(t, h, url.Values{
		"Action":           {"DescribeLoadBalancerAttributes"},
		"Version":          {"2012-06-01"},
		"LoadBalancerName": {"al-rt-lb"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		XMLName xml.Name `xml:"DescribeLoadBalancerAttributesResponse"`
		Result  struct {
			LoadBalancerAttributes struct {
				AccessLog struct {
					Enabled        string `xml:"Enabled"`
					S3BucketName   string `xml:"S3BucketName"`
					S3BucketPrefix string `xml:"S3BucketPrefix"`
					EmitInterval   string `xml:"EmitInterval"`
				} `xml:"AccessLog"`
			} `xml:"LoadBalancerAttributes"`
		} `xml:"DescribeLoadBalancerAttributesResult"`
	}
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
	al := resp.Result.LoadBalancerAttributes.AccessLog
	assert.Equal(t, "true", al.Enabled)
	assert.Equal(t, "rt-bucket", al.S3BucketName)
	assert.Equal(t, "prefix/", al.S3BucketPrefix)
	assert.Equal(t, "5", al.EmitInterval)
}

func TestAccessLogSnapshotRestore(t *testing.T) {
	t.Parallel()

	b := newBackend()
	h := elb.NewHandler(b)
	mustCreateLB(t, h, "al-snap-lb")

	doELB(t, h, url.Values{
		"Action":           {"ModifyLoadBalancerAttributes"},
		"Version":          {"2012-06-01"},
		"LoadBalancerName": {"al-snap-lb"},
		"LoadBalancerAttributes.ConnectionSettings.IdleTimeout": {"60"},
		"LoadBalancerAttributes.AccessLog.Enabled":              {"true"},
		"LoadBalancerAttributes.AccessLog.S3BucketName":         {"snap-bucket"},
		"LoadBalancerAttributes.AccessLog.EmitInterval":         {"60"},
	})

	snap := b.Snapshot(t.Context())
	require.NotNil(t, snap)

	b2 := elb.NewInMemoryBackend("123456789012", "us-east-1")
	require.NoError(t, b2.Restore(t.Context(), snap))

	attrs, err := b2.DescribeLoadBalancerAttributes(context.Background(), "al-snap-lb")
	require.NoError(t, err)
	assert.True(t, attrs.AccessLog.Enabled)
	assert.Equal(t, "snap-bucket", attrs.AccessLog.S3BucketName)
	assert.Equal(t, int32(60), attrs.AccessLog.EmitInterval)
}

func TestAccessLogUpdateBucket(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	mustCreateLB(t, h, "al-upd-lb")

	doELB(t, h, url.Values{
		"Action":           {"ModifyLoadBalancerAttributes"},
		"Version":          {"2012-06-01"},
		"LoadBalancerName": {"al-upd-lb"},
		"LoadBalancerAttributes.ConnectionSettings.IdleTimeout": {"60"},
		"LoadBalancerAttributes.AccessLog.Enabled":              {"true"},
		"LoadBalancerAttributes.AccessLog.S3BucketName":         {"old-bucket"},
		"LoadBalancerAttributes.AccessLog.EmitInterval":         {"60"},
	})

	rec := doELB(t, h, url.Values{
		"Action":           {"ModifyLoadBalancerAttributes"},
		"Version":          {"2012-06-01"},
		"LoadBalancerName": {"al-upd-lb"},
		"LoadBalancerAttributes.ConnectionSettings.IdleTimeout": {"60"},
		"LoadBalancerAttributes.AccessLog.Enabled":              {"true"},
		"LoadBalancerAttributes.AccessLog.S3BucketName":         {"new-bucket"},
		"LoadBalancerAttributes.AccessLog.EmitInterval":         {"5"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	attrs, err := h.Backend.DescribeLoadBalancerAttributes(context.Background(), "al-upd-lb")
	require.NoError(t, err)
	assert.Equal(t, "new-bucket", attrs.AccessLog.S3BucketName)
	assert.Equal(t, int32(5), attrs.AccessLog.EmitInterval)
}

func TestCrossZoneLoadBalancingToggle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		enabled string
		want    string
	}{
		{name: "enable_cross_zone", enabled: "true", want: "true"},
		{name: "disable_cross_zone", enabled: "false", want: "false"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			mustCreateLB(t, h, "czlb-lb")

			rec := doELB(t, h, url.Values{
				"Action":           {"ModifyLoadBalancerAttributes"},
				"Version":          {"2012-06-01"},
				"LoadBalancerName": {"czlb-lb"},
				"LoadBalancerAttributes.ConnectionSettings.IdleTimeout": {"60"},
				"LoadBalancerAttributes.CrossZoneLoadBalancing.Enabled": {tt.enabled},
			})
			require.Equal(t, http.StatusOK, rec.Code)

			var resp struct {
				XMLName xml.Name `xml:"ModifyLoadBalancerAttributesResponse"`
				Result  struct {
					LoadBalancerAttributes struct {
						CrossZoneLoadBalancing struct {
							Enabled string `xml:"Enabled"`
						} `xml:"CrossZoneLoadBalancing"`
					} `xml:"LoadBalancerAttributes"`
				} `xml:"ModifyLoadBalancerAttributesResult"`
			}
			require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, tt.want, resp.Result.LoadBalancerAttributes.CrossZoneLoadBalancing.Enabled)
		})
	}
}

func TestCrossZoneLoadBalancingDefaultFalse(t *testing.T) {
	t.Parallel()

	b := newBackend()
	h := elb.NewHandler(b)
	mustCreateLB(t, h, "czlb-default-lb")

	attrs, err := b.DescribeLoadBalancerAttributes(context.Background(), "czlb-default-lb")
	require.NoError(t, err)
	assert.False(t, attrs.CrossZoneLoadBalancing)
}

func TestCrossZoneLoadBalancingSnapshotRestore(t *testing.T) {
	t.Parallel()

	b := newBackend()
	h := elb.NewHandler(b)
	mustCreateLB(t, h, "czlb-snap-lb")

	doELB(t, h, url.Values{
		"Action":           {"ModifyLoadBalancerAttributes"},
		"Version":          {"2012-06-01"},
		"LoadBalancerName": {"czlb-snap-lb"},
		"LoadBalancerAttributes.ConnectionSettings.IdleTimeout": {"60"},
		"LoadBalancerAttributes.CrossZoneLoadBalancing.Enabled": {"true"},
	})

	snap := b.Snapshot(t.Context())
	b2 := elb.NewInMemoryBackend("123456789012", "us-east-1")
	require.NoError(t, b2.Restore(t.Context(), snap))

	attrs, err := b2.DescribeLoadBalancerAttributes(context.Background(), "czlb-snap-lb")
	require.NoError(t, err)
	assert.True(t, attrs.CrossZoneLoadBalancing)
}

func TestConnectionDrainingEnable(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	mustCreateLB(t, h, "cd-enable-lb")

	rec := doELB(t, h, url.Values{
		"Action":           {"ModifyLoadBalancerAttributes"},
		"Version":          {"2012-06-01"},
		"LoadBalancerName": {"cd-enable-lb"},
		"LoadBalancerAttributes.ConnectionSettings.IdleTimeout": {"60"},
		"LoadBalancerAttributes.ConnectionDraining.Enabled":     {"true"},
		"LoadBalancerAttributes.ConnectionDraining.Timeout":     {"300"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		XMLName xml.Name `xml:"ModifyLoadBalancerAttributesResponse"`
		Result  struct {
			LoadBalancerAttributes struct {
				ConnectionDraining struct {
					Enabled string `xml:"Enabled"`
					Timeout string `xml:"Timeout"`
				} `xml:"ConnectionDraining"`
			} `xml:"LoadBalancerAttributes"`
		} `xml:"ModifyLoadBalancerAttributesResult"`
	}
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
	cd := resp.Result.LoadBalancerAttributes.ConnectionDraining
	assert.Equal(t, "true", cd.Enabled)
	assert.Equal(t, "300", cd.Timeout)
}

func TestConnectionDrainingDefaultValues(t *testing.T) {
	t.Parallel()

	b := newBackend()
	h := elb.NewHandler(b)
	mustCreateLB(t, h, "cd-default-lb")

	attrs, err := b.DescribeLoadBalancerAttributes(context.Background(), "cd-default-lb")
	require.NoError(t, err)
	assert.False(t, attrs.ConnectionDraining)
	assert.Equal(t, int32(300), attrs.ConnectionDrainingTimeout)
}

func TestConnectionDrainingDisable(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	mustCreateLB(t, h, "cd-disable-lb")

	doELB(t, h, url.Values{
		"Action":           {"ModifyLoadBalancerAttributes"},
		"Version":          {"2012-06-01"},
		"LoadBalancerName": {"cd-disable-lb"},
		"LoadBalancerAttributes.ConnectionSettings.IdleTimeout": {"60"},
		"LoadBalancerAttributes.ConnectionDraining.Enabled":     {"true"},
		"LoadBalancerAttributes.ConnectionDraining.Timeout":     {"100"},
	})

	rec := doELB(t, h, url.Values{
		"Action":           {"ModifyLoadBalancerAttributes"},
		"Version":          {"2012-06-01"},
		"LoadBalancerName": {"cd-disable-lb"},
		"LoadBalancerAttributes.ConnectionSettings.IdleTimeout": {"60"},
		"LoadBalancerAttributes.ConnectionDraining.Enabled":     {"false"},
		"LoadBalancerAttributes.ConnectionDraining.Timeout":     {"300"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	attrs, err := h.Backend.DescribeLoadBalancerAttributes(context.Background(), "cd-disable-lb")
	require.NoError(t, err)
	assert.False(t, attrs.ConnectionDraining)
}

func TestConnectionDrainingTimeoutBoundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		timeout    string
		wantStatus int
	}{
		{name: "min_1", timeout: "1", wantStatus: http.StatusOK},
		{name: "max_3600", timeout: "3600", wantStatus: http.StatusOK},
		{name: "zero_rejected", timeout: "0", wantStatus: http.StatusBadRequest},
		{name: "over_max_rejected", timeout: "3601", wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			mustCreateLB(t, h, "cd-bound-lb")

			rec := doELB(t, h, url.Values{
				"Action":           {"ModifyLoadBalancerAttributes"},
				"Version":          {"2012-06-01"},
				"LoadBalancerName": {"cd-bound-lb"},
				"LoadBalancerAttributes.ConnectionSettings.IdleTimeout": {"60"},
				"LoadBalancerAttributes.ConnectionDraining.Enabled":     {"true"},
				"LoadBalancerAttributes.ConnectionDraining.Timeout":     {tt.timeout},
			})
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestConnectionSettingsIdleTimeoutDefault(t *testing.T) {
	t.Parallel()

	b := newBackend()
	h := elb.NewHandler(b)
	mustCreateLB(t, h, "cs-default-lb")

	attrs, err := b.DescribeLoadBalancerAttributes(context.Background(), "cs-default-lb")
	require.NoError(t, err)
	assert.Equal(t, int32(60), attrs.IdleTimeout)
}

func TestConnectionSettingsIdleTimeoutUpdateAndRead(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	mustCreateLB(t, h, "cs-update-lb")

	rec := doELB(t, h, url.Values{
		"Action":           {"ModifyLoadBalancerAttributes"},
		"Version":          {"2012-06-01"},
		"LoadBalancerName": {"cs-update-lb"},
		"LoadBalancerAttributes.ConnectionSettings.IdleTimeout": {"120"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		XMLName xml.Name `xml:"ModifyLoadBalancerAttributesResponse"`
		Result  struct {
			LoadBalancerAttributes struct {
				ConnectionSettings struct {
					IdleTimeout string `xml:"IdleTimeout"`
				} `xml:"ConnectionSettings"`
			} `xml:"LoadBalancerAttributes"`
		} `xml:"ModifyLoadBalancerAttributesResult"`
	}
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "120", resp.Result.LoadBalancerAttributes.ConnectionSettings.IdleTimeout)
}

func TestConnectionSettingsIdleTimeoutBoundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		timeout    string
		wantStatus int
	}{
		{name: "min_1", timeout: "1", wantStatus: http.StatusOK},
		{name: "max_3600", timeout: "3600", wantStatus: http.StatusOK},
		{name: "zero_rejected", timeout: "0", wantStatus: http.StatusBadRequest},
		{name: "over_max_rejected", timeout: "3601", wantStatus: http.StatusBadRequest},
		{name: "negative_rejected", timeout: "-1", wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			mustCreateLB(t, h, "cs-bound-lb")

			rec := doELB(t, h, url.Values{
				"Action":           {"ModifyLoadBalancerAttributes"},
				"Version":          {"2012-06-01"},
				"LoadBalancerName": {"cs-bound-lb"},
				"LoadBalancerAttributes.ConnectionSettings.IdleTimeout": {tt.timeout},
			})
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestDesyncMitigationModeDefaultDefensive(t *testing.T) {
	t.Parallel()

	b := newBackend()
	h := elb.NewHandler(b)
	mustCreateLB(t, h, "desync-def-lb")

	attrs, err := b.DescribeLoadBalancerAttributes(context.Background(), "desync-def-lb")
	require.NoError(t, err)
	assert.Equal(t, "defensive", attrs.DesyncMitigationMode)
}

func TestDesyncMitigationModeInXMLResponse(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	mustCreateLB(t, h, "desync-xml-lb")

	doELB(t, h, url.Values{
		"Action":           {"ModifyLoadBalancerAttributes"},
		"Version":          {"2012-06-01"},
		"LoadBalancerName": {"desync-xml-lb"},
		"LoadBalancerAttributes.ConnectionSettings.IdleTimeout":      {"60"},
		"LoadBalancerAttributes.AdditionalAttributes.member.1.Key":   {"elb.http.desyncmitigationmode"},
		"LoadBalancerAttributes.AdditionalAttributes.member.1.Value": {"strictest"},
	})

	rec := doELB(t, h, url.Values{
		"Action":           {"DescribeLoadBalancerAttributes"},
		"Version":          {"2012-06-01"},
		"LoadBalancerName": {"desync-xml-lb"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		XMLName xml.Name `xml:"DescribeLoadBalancerAttributesResponse"`
		Result  struct {
			LoadBalancerAttributes struct {
				AdditionalAttributes struct {
					Members []struct {
						Key   string `xml:"Key"`
						Value string `xml:"Value"`
					} `xml:"member"`
				} `xml:"AdditionalAttributes"`
			} `xml:"LoadBalancerAttributes"`
		} `xml:"DescribeLoadBalancerAttributesResult"`
	}
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))

	found := false
	for _, a := range resp.Result.LoadBalancerAttributes.AdditionalAttributes.Members {
		if a.Key == "elb.http.desyncmitigationmode" {
			assert.Equal(t, "strictest", a.Value)
			found = true
		}
	}
	assert.True(t, found, "desync mode must appear in AdditionalAttributes")
}

// TestModifyLBAttributesIdleTimeout verifies idle timeout validation.
func TestModifyLBAttributesIdleTimeout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		timeout    string
		wantStatus int
	}{
		{name: "zero_rejected", timeout: "0", wantStatus: http.StatusBadRequest},
		{name: "negative_rejected", timeout: "-1", wantStatus: http.StatusBadRequest},
		{name: "over_max_rejected", timeout: "3601", wantStatus: http.StatusBadRequest},
		{name: "min_accepted", timeout: "1", wantStatus: http.StatusOK},
		{name: "max_accepted", timeout: "3600", wantStatus: http.StatusOK},
		{name: "typical_accepted", timeout: "60", wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend()
			h := elb.NewHandler(b)
			mustCreateLB(t, h, "idle-to-lb")

			rec := doELB(t, h, url.Values{
				"Action":           {"ModifyLoadBalancerAttributes"},
				"Version":          {"2012-06-01"},
				"LoadBalancerName": {"idle-to-lb"},
				"LoadBalancerAttributes.ConnectionSettings.IdleTimeout": {tt.timeout},
			})

			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestModifyLBAttributesDrainingTimeout verifies connection draining timeout validation.
func TestModifyLBAttributesDrainingTimeout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		timeout    string
		wantStatus int
	}{
		{name: "over_max_rejected", timeout: "3601", wantStatus: http.StatusBadRequest},
		{name: "zero_rejected", timeout: "0", wantStatus: http.StatusBadRequest},
		{name: "max_accepted", timeout: "3600", wantStatus: http.StatusOK},
		{name: "min_accepted", timeout: "1", wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend()
			h := elb.NewHandler(b)
			mustCreateLB(t, h, "drain-to-lb")

			rec := doELB(t, h, url.Values{
				"Action":           {"ModifyLoadBalancerAttributes"},
				"Version":          {"2012-06-01"},
				"LoadBalancerName": {"drain-to-lb"},
				"LoadBalancerAttributes.ConnectionSettings.IdleTimeout": {"60"},
				"LoadBalancerAttributes.ConnectionDraining.Enabled":     {"true"},
				"LoadBalancerAttributes.ConnectionDraining.Timeout":     {tt.timeout},
			})

			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestModifyLBAttributesDesyncMode verifies desync mitigation mode validation.
func TestModifyLBAttributesDesyncMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		mode       string
		wantStatus int
	}{
		{name: "invalid_mode_rejected", mode: "invalid-mode", wantStatus: http.StatusBadRequest},
		{name: "defensive_accepted", mode: "defensive", wantStatus: http.StatusOK},
		{name: "strictest_accepted", mode: "strictest", wantStatus: http.StatusOK},
		{name: "monitor_accepted", mode: "monitor", wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend()
			h := elb.NewHandler(b)
			mustCreateLB(t, h, "desync-lb")

			rec := doELB(t, h, url.Values{
				"Action":           {"ModifyLoadBalancerAttributes"},
				"Version":          {"2012-06-01"},
				"LoadBalancerName": {"desync-lb"},
				"LoadBalancerAttributes.ConnectionSettings.IdleTimeout":      {"60"},
				"LoadBalancerAttributes.AdditionalAttributes.member.1.Key":   {"elb.http.desyncmitigationmode"},
				"LoadBalancerAttributes.AdditionalAttributes.member.1.Value": {tt.mode},
			})
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestModifyLoadBalancerAttributesPartialUpdatePreservesOtherGroups verifies that
// a ModifyLoadBalancerAttributes call touching only one attribute group (here,
// CrossZoneLoadBalancing) does not reset the other independently-settable
// groups (AccessLog, ConnectionDraining, DesyncMitigationMode) to their
// defaults. Each group in types.LoadBalancerAttributes is optional and
// independently settable in the AWS SDK.
func TestModifyLoadBalancerAttributesPartialUpdatePreservesOtherGroups(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	mustCreateLB(t, h, "partial-upd-lb")

	doELB(t, h, url.Values{
		"Action":           {"ModifyLoadBalancerAttributes"},
		"Version":          {"2012-06-01"},
		"LoadBalancerName": {"partial-upd-lb"},
		"LoadBalancerAttributes.AccessLog.Enabled":                   {"true"},
		"LoadBalancerAttributes.AccessLog.S3BucketName":              {"keep-me-bucket"},
		"LoadBalancerAttributes.AccessLog.EmitInterval":              {"5"},
		"LoadBalancerAttributes.ConnectionDraining.Enabled":          {"true"},
		"LoadBalancerAttributes.ConnectionDraining.Timeout":          {"120"},
		"LoadBalancerAttributes.AdditionalAttributes.member.1.Key":   {"elb.http.desyncmitigationmode"},
		"LoadBalancerAttributes.AdditionalAttributes.member.1.Value": {"strictest"},
	})

	rec := doELB(t, h, url.Values{
		"Action":           {"ModifyLoadBalancerAttributes"},
		"Version":          {"2012-06-01"},
		"LoadBalancerName": {"partial-upd-lb"},
		"LoadBalancerAttributes.CrossZoneLoadBalancing.Enabled": {"true"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	attrs, err := h.Backend.DescribeLoadBalancerAttributes(context.Background(), "partial-upd-lb")
	require.NoError(t, err)

	assert.True(t, attrs.CrossZoneLoadBalancing, "the group this request actually set")
	assert.True(t, attrs.AccessLog.Enabled, "AccessLog must survive an update that omitted it")
	assert.Equal(t, "keep-me-bucket", attrs.AccessLog.S3BucketName)
	assert.Equal(t, int32(5), attrs.AccessLog.EmitInterval)
	assert.True(t, attrs.ConnectionDraining, "ConnectionDraining must survive an update that omitted it")
	assert.Equal(t, int32(120), attrs.ConnectionDrainingTimeout)
	assert.Equal(
		t, "strictest", attrs.DesyncMitigationMode, "DesyncMitigationMode must survive an update that omitted it",
	)
}

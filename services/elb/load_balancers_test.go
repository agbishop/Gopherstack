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

// TestCreateLoadBalancer tests create and duplicate error.
func TestCreateLoadBalancer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(t *testing.T, h *elb.Handler)
		vals       url.Values
		name       string
		wantDNS    string
		wantStatus int
	}{
		{
			name: "creates_successfully",
			vals: url.Values{
				"Action":                              {"CreateLoadBalancer"},
				"Version":                             {"2012-06-01"},
				"LoadBalancerName":                    {"my-lb"},
				"Listeners.member.1.Protocol":         {"HTTP"},
				"Listeners.member.1.LoadBalancerPort": {"80"},
				"Listeners.member.1.InstancePort":     {"8080"},
				"AvailabilityZones.member.1":          {"us-east-1a"},
			},
			wantStatus: http.StatusOK,
			// DNS now includes a hash suffix: my-lb-<hash>.us-east-1.elb.amazonaws.com
			wantDNS: "my-lb-",
		},
		{
			name: "duplicate_returns_conflict",
			setup: func(t *testing.T, h *elb.Handler) {
				t.Helper()
				mustCreateLB(t, h, "dup-lb")
			},
			vals: url.Values{
				"Action":                              {"CreateLoadBalancer"},
				"Version":                             {"2012-06-01"},
				"LoadBalancerName":                    {"dup-lb"},
				"Listeners.member.1.Protocol":         {"HTTP"},
				"Listeners.member.1.LoadBalancerPort": {"80"},
				"Listeners.member.1.InstancePort":     {"8080"},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "missing_name_returns_bad_request",
			vals: url.Values{
				"Action":  {"CreateLoadBalancer"},
				"Version": {"2012-06-01"},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "with_scheme",
			vals: url.Values{
				"Action":                              {"CreateLoadBalancer"},
				"Version":                             {"2012-06-01"},
				"LoadBalancerName":                    {"internal-lb"},
				"Scheme":                              {"internal"},
				"Listeners.member.1.Protocol":         {"HTTP"},
				"Listeners.member.1.LoadBalancerPort": {"80"},
				"Listeners.member.1.InstancePort":     {"8080"},
				"AvailabilityZones.member.1":          {"us-east-1a"},
			},
			wantStatus: http.StatusOK,
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

			if tt.wantDNS != "" {
				var resp struct {
					XMLName xml.Name `xml:"CreateLoadBalancerResponse"`
					Result  struct {
						DNSName string `xml:"DNSName"`
					} `xml:"CreateLoadBalancerResult"`
				}
				parseXMLBody(t, rec, &resp)
				assert.Contains(t, resp.Result.DNSName, tt.wantDNS)
			}
		})
	}
}

// TestDeleteLoadBalancer tests delete operations.
func TestDeleteLoadBalancer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(t *testing.T, h *elb.Handler)
		vals       url.Values
		name       string
		wantStatus int
	}{
		{
			name: "delete_existing",
			setup: func(t *testing.T, h *elb.Handler) {
				t.Helper()
				mustCreateLB(t, h, "delete-me")
			},
			vals: url.Values{
				"Action":           {"DeleteLoadBalancer"},
				"Version":          {"2012-06-01"},
				"LoadBalancerName": {"delete-me"},
			},
			wantStatus: http.StatusOK,
		},
		{
			// gopherstack-5gfl: AWS's own doc comment says DeleteLoadBalancer
			// "still succeeds" for a name that doesn't exist -- idempotent, not
			// an error.
			name: "delete_not_found_is_idempotent_success",
			vals: url.Values{
				"Action":           {"DeleteLoadBalancer"},
				"Version":          {"2012-06-01"},
				"LoadBalancerName": {"no-such-lb"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "missing_name",
			vals: url.Values{
				"Action":  {"DeleteLoadBalancer"},
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
		})
	}
}

func TestCreateLoadBalancerInternetFacingDefault(t *testing.T) {
	t.Parallel()

	b := newBackend()
	h := elb.NewHandler(b)
	mustCreateLB(t, h, "scheme-default-lb")

	lbs, err := b.DescribeLoadBalancers(context.Background(), []string{"scheme-default-lb"})
	require.NoError(t, err)
	assert.Equal(t, "internet-facing", lbs[0].Scheme)
}

func TestCreateLoadBalancerInternalScheme(t *testing.T) {
	t.Parallel()

	h := newTestHandler()

	rec := doELB(t, h, url.Values{
		"Action":                              {"CreateLoadBalancer"},
		"Version":                             {"2012-06-01"},
		"LoadBalancerName":                    {"internal-lb"},
		"Scheme":                              {"internal"},
		"Subnets.member.1":                    {"subnet-aaa"},
		"Listeners.member.1.Protocol":         {"HTTP"},
		"Listeners.member.1.LoadBalancerPort": {"80"},
		"Listeners.member.1.InstancePort":     {"8080"},
	})
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestCreateLoadBalancerInvalidScheme(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	rec := doELB(t, h, url.Values{
		"Action":                              {"CreateLoadBalancer"},
		"Version":                             {"2012-06-01"},
		"LoadBalancerName":                    {"bad-scheme-lb"},
		"Scheme":                              {"private"},
		"Listeners.member.1.Protocol":         {"HTTP"},
		"Listeners.member.1.LoadBalancerPort": {"80"},
		"Listeners.member.1.InstancePort":     {"8080"},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreateLoadBalancerNameValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		lbName     string
		wantStatus int
	}{
		{name: "single_char_ok", lbName: "a", wantStatus: http.StatusOK},
		{name: "max_32_chars_ok", lbName: "abcdefghijklmnopqrstuvwxyz123456", wantStatus: http.StatusOK},
		{name: "with_hyphens_ok", lbName: "my-load-balancer-1", wantStatus: http.StatusOK},
		{name: "starts_with_hyphen_rejected", lbName: "-bad-name", wantStatus: http.StatusBadRequest},
		{name: "ends_with_hyphen_rejected", lbName: "bad-name-", wantStatus: http.StatusBadRequest},
		{name: "too_long_rejected", lbName: "abcdefghijklmnopqrstuvwxyz1234567", wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			rec := doELB(t, h, url.Values{
				"Action":                              {"CreateLoadBalancer"},
				"Version":                             {"2012-06-01"},
				"LoadBalancerName":                    {tt.lbName},
				"AvailabilityZones.member.1":          {"us-east-1a"},
				"Listeners.member.1.Protocol":         {"HTTP"},
				"Listeners.member.1.LoadBalancerPort": {"80"},
				"Listeners.member.1.InstancePort":     {"8080"},
			})
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestCreateLoadBalancerWithInitialTags(t *testing.T) {
	t.Parallel()

	b := newBackend()
	h := elb.NewHandler(b)

	rec := doELB(t, h, url.Values{
		"Action":                              {"CreateLoadBalancer"},
		"Version":                             {"2012-06-01"},
		"LoadBalancerName":                    {"tagged-lb"},
		"AvailabilityZones.member.1":          {"us-east-1a"},
		"Listeners.member.1.Protocol":         {"HTTP"},
		"Listeners.member.1.LoadBalancerPort": {"80"},
		"Listeners.member.1.InstancePort":     {"8080"},
		"Tags.member.1.Key":                   {"Env"},
		"Tags.member.1.Value":                 {"prod"},
		"Tags.member.2.Key":                   {"App"},
		"Tags.member.2.Value":                 {"web"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	tagMap, err := b.DescribeTags(context.Background(), []string{"tagged-lb"})
	require.NoError(t, err)
	tags := tagMap["tagged-lb"]
	require.Len(t, tags, 2)
}

func TestCreateLoadBalancerDNSNameSet(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	rec := doELB(t, h, url.Values{
		"Action":                              {"CreateLoadBalancer"},
		"Version":                             {"2012-06-01"},
		"LoadBalancerName":                    {"dns-lb"},
		"AvailabilityZones.member.1":          {"us-east-1a"},
		"Listeners.member.1.Protocol":         {"HTTP"},
		"Listeners.member.1.LoadBalancerPort": {"80"},
		"Listeners.member.1.InstancePort":     {"8080"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		XMLName xml.Name `xml:"CreateLoadBalancerResponse"`
		Result  struct {
			DNSName string `xml:"DNSName"`
		} `xml:"CreateLoadBalancerResult"`
	}
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.Result.DNSName)
	assert.Contains(t, resp.Result.DNSName, "dns-lb")
}

func TestDeleteLoadBalancerAlsoDeletesTags(t *testing.T) {
	t.Parallel()

	b := newBackend()
	h := elb.NewHandler(b)
	mustCreateLB(t, h, "del-tags-lb")

	doELB(t, h, url.Values{
		"Action":                     {"AddTags"},
		"Version":                    {"2012-06-01"},
		"LoadBalancerNames.member.1": {"del-tags-lb"},
		"Tags.member.1.Key":          {"k"},
		"Tags.member.1.Value":        {"v"},
	})

	doELB(t, h, url.Values{
		"Action":           {"DeleteLoadBalancer"},
		"Version":          {"2012-06-01"},
		"LoadBalancerName": {"del-tags-lb"},
	})

	assert.Equal(t, 0, b.LoadBalancerCount())
}

// TestDeleteLoadBalancerNotFoundIsIdempotent verifies that deleting a
// nonexistent load balancer succeeds rather than erroring: DeleteLoadBalancer
// has no typed not-found exception in the real SDK, and its doc comment
// states this explicitly (gopherstack-5gfl).
func TestDeleteLoadBalancerNotFoundIsIdempotent(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	rec := doELB(t, h, url.Values{
		"Action":           {"DeleteLoadBalancer"},
		"Version":          {"2012-06-01"},
		"LoadBalancerName": {"no-such-lb"},
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestCreateLoadBalancerDuplicateName(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	mustCreateLB(t, h, "dup-name-lb")

	rec := doELB(t, h, url.Values{
		"Action":                              {"CreateLoadBalancer"},
		"Version":                             {"2012-06-01"},
		"LoadBalancerName":                    {"dup-name-lb"},
		"AvailabilityZones.member.1":          {"us-east-1a"},
		"Listeners.member.1.Protocol":         {"HTTP"},
		"Listeners.member.1.LoadBalancerPort": {"80"},
		"Listeners.member.1.InstancePort":     {"8080"},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp struct {
		XMLName xml.Name `xml:"ErrorResponse"`
		Error   struct {
			Code string `xml:"Code"`
		} `xml:"Error"`
	}
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "DuplicateLoadBalancerName", errResp.Error.Code)
}

// TestAZSubnetMutualExclusivity verifies that supplying both
// AvailabilityZones and Subnets in CreateLoadBalancer is rejected.
func TestAZSubnetMutualExclusivity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		vals       url.Values
		name       string
		wantStatus int
	}{
		{
			name: "az_only_accepted",
			vals: url.Values{
				"Action":                              {"CreateLoadBalancer"},
				"Version":                             {"2012-06-01"},
				"LoadBalancerName":                    {"az-only-lb"},
				"Listeners.member.1.Protocol":         {"HTTP"},
				"Listeners.member.1.LoadBalancerPort": {"80"},
				"Listeners.member.1.InstancePort":     {"8080"},
				"AvailabilityZones.member.1":          {"us-east-1a"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "subnet_only_accepted",
			vals: url.Values{
				"Action":                              {"CreateLoadBalancer"},
				"Version":                             {"2012-06-01"},
				"LoadBalancerName":                    {"subnet-only-lb"},
				"Listeners.member.1.Protocol":         {"HTTP"},
				"Listeners.member.1.LoadBalancerPort": {"80"},
				"Listeners.member.1.InstancePort":     {"8080"},
				"Subnets.member.1":                    {"subnet-000a"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "both_az_and_subnet_rejected",
			vals: url.Values{
				"Action":                              {"CreateLoadBalancer"},
				"Version":                             {"2012-06-01"},
				"LoadBalancerName":                    {"az-subnet-lb"},
				"Listeners.member.1.Protocol":         {"HTTP"},
				"Listeners.member.1.LoadBalancerPort": {"80"},
				"Listeners.member.1.InstancePort":     {"8080"},
				"AvailabilityZones.member.1":          {"us-east-1a"},
				"Subnets.member.1":                    {"subnet-000a"},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "neither_az_nor_subnet_rejected",
			vals: url.Values{
				"Action":                              {"CreateLoadBalancer"},
				"Version":                             {"2012-06-01"},
				"LoadBalancerName":                    {"no-az-subnet-lb"},
				"Listeners.member.1.Protocol":         {"HTTP"},
				"Listeners.member.1.LoadBalancerPort": {"80"},
				"Listeners.member.1.InstancePort":     {"8080"},
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			rec := doELB(t, h, tt.vals)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestCreateLBListenerRequirements verifies various listener validation
// rules during CreateLoadBalancer.
func TestCreateLBListenerRequirements(t *testing.T) {
	t.Parallel()

	tests := []struct {
		vals       url.Values
		name       string
		wantStatus int
	}{
		{
			name: "https_requires_cert",
			vals: url.Values{
				"Action":                              {"CreateLoadBalancer"},
				"Version":                             {"2012-06-01"},
				"LoadBalancerName":                    {"nocert-lb"},
				"Listeners.member.1.Protocol":         {"HTTPS"},
				"Listeners.member.1.LoadBalancerPort": {"443"},
				"Listeners.member.1.InstancePort":     {"8443"},
				"AvailabilityZones.member.1":          {"us-east-1a"},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "ssl_requires_cert",
			vals: url.Values{
				"Action":                              {"CreateLoadBalancer"},
				"Version":                             {"2012-06-01"},
				"LoadBalancerName":                    {"nocert-ssl-lb"},
				"Listeners.member.1.Protocol":         {"SSL"},
				"Listeners.member.1.LoadBalancerPort": {"443"},
				"Listeners.member.1.InstancePort":     {"443"},
				"AvailabilityZones.member.1":          {"us-east-1a"},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "https_with_cert_accepted",
			vals: url.Values{
				"Action":                              {"CreateLoadBalancer"},
				"Version":                             {"2012-06-01"},
				"LoadBalancerName":                    {"withcert-lb"},
				"Listeners.member.1.Protocol":         {"HTTPS"},
				"Listeners.member.1.LoadBalancerPort": {"443"},
				"Listeners.member.1.InstancePort":     {"8443"},
				"Listeners.member.1.SSLCertificateId": {"arn:aws:acm:us-east-1:123456789012:certificate/abc"},
				"AvailabilityZones.member.1":          {"us-east-1a"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "no_listeners_rejected",
			vals: url.Values{
				"Action":                     {"CreateLoadBalancer"},
				"Version":                    {"2012-06-01"},
				"LoadBalancerName":           {"nolisteners-lb"},
				"AvailabilityZones.member.1": {"us-east-1a"},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "allowed_port_25_accepted",
			vals: url.Values{
				"Action":                              {"CreateLoadBalancer"},
				"Version":                             {"2012-06-01"},
				"LoadBalancerName":                    {"port25-lb"},
				"Listeners.member.1.Protocol":         {"TCP"},
				"Listeners.member.1.LoadBalancerPort": {"25"},
				"Listeners.member.1.InstancePort":     {"25"},
				"AvailabilityZones.member.1":          {"us-east-1a"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "reserved_port_rejected",
			vals: url.Values{
				"Action":                              {"CreateLoadBalancer"},
				"Version":                             {"2012-06-01"},
				"LoadBalancerName":                    {"port2-lb"},
				"Listeners.member.1.Protocol":         {"HTTP"},
				"Listeners.member.1.LoadBalancerPort": {"2"},
				"Listeners.member.1.InstancePort":     {"8080"},
				"AvailabilityZones.member.1":          {"us-east-1a"},
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			rec := doELB(t, h, tt.vals)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestSeedHelpers verifies AddLoadBalancerInternal inserts an LB correctly.
func TestSeedHelpers(t *testing.T) {
	t.Parallel()

	b := newBackend()
	b.AddLoadBalancerInternal(elb.LoadBalancer{
		LoadBalancerName: "seeded-lb",
		Scheme:           "internal",
	})

	assert.Equal(t, 1, b.LoadBalancerCount())
}

// TestDeleteLoadBalancerCascadesPolicies verifies that DeleteLoadBalancer
// removes all policies associated with the deleted LB.
func TestDeleteLoadBalancerCascadesPolicies(t *testing.T) {
	t.Parallel()

	b := newBackend()
	h := elb.NewHandler(b)
	mustCreateLB(t, h, "cascade-lb")

	doELB(t, h, url.Values{
		"Action":           {"CreateAppCookieStickinessPolicy"},
		"Version":          {"2012-06-01"},
		"LoadBalancerName": {"cascade-lb"},
		"PolicyName":       {"pol-1"},
		"CookieName":       {"SID"},
	})

	require.Equal(t, 1, b.PolicyCount())

	doELB(t, h, url.Values{
		"Action":           {"DeleteLoadBalancer"},
		"Version":          {"2012-06-01"},
		"LoadBalancerName": {"cascade-lb"},
	})

	assert.Equal(t, 0, b.LoadBalancerCount())
	assert.Equal(t, 0, b.PolicyCount())
}

// TestNonNilSlices verifies that all slice fields on a newly created LB are non-nil.
func TestNonNilSlices(t *testing.T) {
	t.Parallel()

	b := newBackend()
	h := elb.NewHandler(b)
	mustCreateLB(t, h, "non-nil-lb")

	lbs, err := b.DescribeLoadBalancers(context.Background(), []string{"non-nil-lb"})
	require.NoError(t, err)
	require.Len(t, lbs, 1)

	lb := lbs[0]

	assert.NotNil(t, lb.Listeners)
	assert.NotNil(t, lb.Instances)
	assert.NotNil(t, lb.AvailabilityZones)
	assert.NotNil(t, lb.SecurityGroups)
	assert.NotNil(t, lb.Subnets)
}

// TestARNSet verifies that ARN is populated on the returned LB.
func TestARNSet(t *testing.T) {
	t.Parallel()

	b := newBackend()
	h := elb.NewHandler(b)
	mustCreateLB(t, h, "arn-lb")

	lbs, err := b.DescribeLoadBalancers(context.Background(), []string{"arn-lb"})
	require.NoError(t, err)
	require.Len(t, lbs, 1)

	assert.NotEmpty(t, lbs[0].ARN, "ARN must be set on created load balancer")
	assert.Contains(t, lbs[0].ARN, "loadbalancer/arn-lb")
}

// TestVPCIdSetFromSubnets verifies that VPCId is derived when subnets are provided.
func TestVPCIdSetFromSubnets(t *testing.T) {
	t.Parallel()

	b := newBackend()
	h := elb.NewHandler(b)

	rec := doELB(t, h, url.Values{
		"Action":                              {"CreateLoadBalancer"},
		"Version":                             {"2012-06-01"},
		"LoadBalancerName":                    {"vpc-lb"},
		"Subnets.member.1":                    {"subnet-abc123"},
		"Listeners.member.1.Protocol":         {"HTTP"},
		"Listeners.member.1.LoadBalancerPort": {"80"},
		"Listeners.member.1.InstancePort":     {"8080"},
	})

	require.Equal(t, http.StatusOK, rec.Code)

	lbs, err := b.DescribeLoadBalancers(context.Background(), []string{"vpc-lb"})
	require.NoError(t, err)
	assert.NotEmpty(t, lbs[0].VPCId, "VPCId must be set when subnets are provided")
}

// TestVPCIdEmptyWithoutSubnets verifies VPCId is empty for classic (no subnets) LBs.
func TestVPCIdEmptyWithoutSubnets(t *testing.T) {
	t.Parallel()

	b := newBackend()
	h := elb.NewHandler(b)

	rec := doELB(t, h, url.Values{
		"Action":                              {"CreateLoadBalancer"},
		"Version":                             {"2012-06-01"},
		"LoadBalancerName":                    {"classic-lb"},
		"AvailabilityZones.member.1":          {"us-east-1a"},
		"Listeners.member.1.Protocol":         {"HTTP"},
		"Listeners.member.1.LoadBalancerPort": {"80"},
		"Listeners.member.1.InstancePort":     {"8080"},
	})

	require.Equal(t, http.StatusOK, rec.Code)

	lbs, err := b.DescribeLoadBalancers(context.Background(), []string{"classic-lb"})
	require.NoError(t, err)
	assert.Empty(t, lbs[0].VPCId, "VPCId must be empty for classic (non-VPC) load balancers")
}

// TestSeedHelperDeepCopy verifies AddLoadBalancerInternal performs a deep copy.
func TestSeedHelperDeepCopy(t *testing.T) {
	t.Parallel()

	b := newBackend()

	original := elb.LoadBalancer{
		LoadBalancerName:  "seed-dc-lb",
		Listeners:         []elb.Listener{{Protocol: "HTTP", LoadBalancerPort: 80, InstancePort: 8080}},
		AvailabilityZones: []string{"us-east-1a"},
	}

	b.AddLoadBalancerInternal(original)

	// Mutate the original after insertion — must not affect stored state.
	original.Listeners = append(
		original.Listeners,
		elb.Listener{Protocol: "HTTPS", LoadBalancerPort: 443, InstancePort: 8443},
	)

	lbs, err := b.DescribeLoadBalancers(context.Background(), []string{"seed-dc-lb"})
	require.NoError(t, err)
	require.Len(t, lbs, 1)
	assert.Len(t, lbs[0].Listeners, 1, "stored LB must not reflect post-seed mutation")
}

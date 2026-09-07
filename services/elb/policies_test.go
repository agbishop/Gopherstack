package elb_test

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/elb"
)

// TestCreateAppCookieStickinessPolicy tests app cookie stickiness policy creation.
func TestCreateAppCookieStickinessPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(t *testing.T, h *elb.Handler)
		vals       url.Values
		name       string
		wantStatus int
	}{
		{
			name: "creates_app_cookie_policy",
			setup: func(t *testing.T, h *elb.Handler) {
				t.Helper()
				mustCreateLB(t, h, "appcookie-lb")
			},
			vals: url.Values{
				"Action":           {"CreateAppCookieStickinessPolicy"},
				"Version":          {"2012-06-01"},
				"LoadBalancerName": {"appcookie-lb"},
				"PolicyName":       {"my-app-cookie-policy"},
				"CookieName":       {"JSESSIONID"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "duplicate_policy_returns_conflict",
			setup: func(t *testing.T, h *elb.Handler) {
				t.Helper()
				mustCreateLB(t, h, "appcookie-dup-lb")
				doELB(t, h, url.Values{
					"Action":           {"CreateAppCookieStickinessPolicy"},
					"Version":          {"2012-06-01"},
					"LoadBalancerName": {"appcookie-dup-lb"},
					"PolicyName":       {"dup-policy"},
					"CookieName":       {"SESSION"},
				})
			},
			vals: url.Values{
				"Action":           {"CreateAppCookieStickinessPolicy"},
				"Version":          {"2012-06-01"},
				"LoadBalancerName": {"appcookie-dup-lb"},
				"PolicyName":       {"dup-policy"},
				"CookieName":       {"SESSION"},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "lb_not_found",
			vals: url.Values{
				"Action":           {"CreateAppCookieStickinessPolicy"},
				"Version":          {"2012-06-01"},
				"LoadBalancerName": {"no-lb"},
				"PolicyName":       {"my-policy"},
				"CookieName":       {"SESSION"},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "missing_lb_name",
			vals: url.Values{
				"Action":     {"CreateAppCookieStickinessPolicy"},
				"Version":    {"2012-06-01"},
				"PolicyName": {"my-policy"},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "missing_policy_name",
			vals: url.Values{
				"Action":           {"CreateAppCookieStickinessPolicy"},
				"Version":          {"2012-06-01"},
				"LoadBalancerName": {"some-lb"},
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

// TestCreateLBCookieStickinessPolicy tests LB cookie stickiness policy creation.
func TestCreateLBCookieStickinessPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(t *testing.T, h *elb.Handler)
		vals       url.Values
		name       string
		wantStatus int
	}{
		{
			name: "creates_lb_cookie_policy_with_expiration",
			setup: func(t *testing.T, h *elb.Handler) {
				t.Helper()
				mustCreateLB(t, h, "lbcookie-lb")
			},
			vals: url.Values{
				"Action":                 {"CreateLBCookieStickinessPolicy"},
				"Version":                {"2012-06-01"},
				"LoadBalancerName":       {"lbcookie-lb"},
				"PolicyName":             {"my-lb-cookie-policy"},
				"CookieExpirationPeriod": {"86400"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "creates_lb_cookie_policy_without_expiration",
			setup: func(t *testing.T, h *elb.Handler) {
				t.Helper()
				mustCreateLB(t, h, "lbcookie2-lb")
			},
			vals: url.Values{
				"Action":           {"CreateLBCookieStickinessPolicy"},
				"Version":          {"2012-06-01"},
				"LoadBalancerName": {"lbcookie2-lb"},
				"PolicyName":       {"browser-session-policy"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "invalid_expiration_period",
			setup: func(t *testing.T, h *elb.Handler) {
				t.Helper()
				mustCreateLB(t, h, "lbcookie3-lb")
			},
			vals: url.Values{
				"Action":                 {"CreateLBCookieStickinessPolicy"},
				"Version":                {"2012-06-01"},
				"LoadBalancerName":       {"lbcookie3-lb"},
				"PolicyName":             {"bad-expiry-policy"},
				"CookieExpirationPeriod": {"not-a-number"},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "lb_not_found",
			vals: url.Values{
				"Action":           {"CreateLBCookieStickinessPolicy"},
				"Version":          {"2012-06-01"},
				"LoadBalancerName": {"no-lb"},
				"PolicyName":       {"my-policy"},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "missing_lb_name",
			vals: url.Values{
				"Action":     {"CreateLBCookieStickinessPolicy"},
				"Version":    {"2012-06-01"},
				"PolicyName": {"my-policy"},
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

// TestCreateLoadBalancerPolicy tests custom LB policy creation.
func TestCreateLoadBalancerPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(t *testing.T, h *elb.Handler)
		vals       url.Values
		name       string
		wantStatus int
	}{
		{
			name: "creates_policy_with_attributes",
			setup: func(t *testing.T, h *elb.Handler) {
				t.Helper()
				mustCreateLB(t, h, "policy-lb")
			},
			vals: url.Values{
				"Action":           {"CreateLoadBalancerPolicy"},
				"Version":          {"2012-06-01"},
				"LoadBalancerName": {"policy-lb"},
				"PolicyName":       {"my-proxy-policy"},
				"PolicyTypeName":   {"ProxyProtocolPolicyType"},
				"PolicyAttributes.member.1.AttributeName":  {"ProxyProtocol"},
				"PolicyAttributes.member.1.AttributeValue": {"true"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "unknown_attribute_name_rejected",
			setup: func(t *testing.T, h *elb.Handler) {
				t.Helper()
				mustCreateLB(t, h, "policy-badattr-lb")
			},
			vals: url.Values{
				"Action":           {"CreateLoadBalancerPolicy"},
				"Version":          {"2012-06-01"},
				"LoadBalancerName": {"policy-badattr-lb"},
				"PolicyName":       {"my-badattr-policy"},
				"PolicyTypeName":   {"ProxyProtocolPolicyType"},
				"PolicyAttributes.member.1.AttributeName":  {"NotARealAttribute"},
				"PolicyAttributes.member.1.AttributeValue": {"true"},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "duplicate_policy_returns_conflict",
			setup: func(t *testing.T, h *elb.Handler) {
				t.Helper()
				mustCreateLB(t, h, "policy-dup-lb")
				doELB(t, h, url.Values{
					"Action":           {"CreateLoadBalancerPolicy"},
					"Version":          {"2012-06-01"},
					"LoadBalancerName": {"policy-dup-lb"},
					"PolicyName":       {"dup-policy"},
					"PolicyTypeName":   {"ProxyProtocolPolicyType"},
				})
			},
			vals: url.Values{
				"Action":           {"CreateLoadBalancerPolicy"},
				"Version":          {"2012-06-01"},
				"LoadBalancerName": {"policy-dup-lb"},
				"PolicyName":       {"dup-policy"},
				"PolicyTypeName":   {"ProxyProtocolPolicyType"},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "lb_not_found",
			vals: url.Values{
				"Action":           {"CreateLoadBalancerPolicy"},
				"Version":          {"2012-06-01"},
				"LoadBalancerName": {"no-lb"},
				"PolicyName":       {"my-policy"},
				"PolicyTypeName":   {"ProxyProtocolPolicyType"},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "missing_lb_name",
			vals: url.Values{
				"Action":         {"CreateLoadBalancerPolicy"},
				"Version":        {"2012-06-01"},
				"PolicyName":     {"my-policy"},
				"PolicyTypeName": {"ProxyProtocolPolicyType"},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "missing_policy_name",
			vals: url.Values{
				"Action":           {"CreateLoadBalancerPolicy"},
				"Version":          {"2012-06-01"},
				"LoadBalancerName": {"some-lb"},
				"PolicyTypeName":   {"ProxyProtocolPolicyType"},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "missing_policy_type_name",
			vals: url.Values{
				"Action":           {"CreateLoadBalancerPolicy"},
				"Version":          {"2012-06-01"},
				"LoadBalancerName": {"some-lb"},
				"PolicyName":       {"my-policy"},
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

// TestDeleteLoadBalancerPolicy tests deleting a policy from a load balancer.
func TestDeleteLoadBalancerPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(t *testing.T, h *elb.Handler)
		vals       url.Values
		name       string
		wantStatus int
	}{
		{
			name: "deletes_existing_policy",
			setup: func(t *testing.T, h *elb.Handler) {
				t.Helper()
				mustCreateLB(t, h, "delpol-lb")
				doELB(t, h, url.Values{
					"Action":           {"CreateLoadBalancerPolicy"},
					"Version":          {"2012-06-01"},
					"LoadBalancerName": {"delpol-lb"},
					"PolicyName":       {"policy-to-delete"},
					"PolicyTypeName":   {"ProxyProtocolPolicyType"},
				})
			},
			vals: url.Values{
				"Action":           {"DeleteLoadBalancerPolicy"},
				"Version":          {"2012-06-01"},
				"LoadBalancerName": {"delpol-lb"},
				"PolicyName":       {"policy-to-delete"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "policy_not_found",
			setup: func(t *testing.T, h *elb.Handler) {
				t.Helper()
				mustCreateLB(t, h, "delpol2-lb")
			},
			vals: url.Values{
				"Action":           {"DeleteLoadBalancerPolicy"},
				"Version":          {"2012-06-01"},
				"LoadBalancerName": {"delpol2-lb"},
				"PolicyName":       {"nonexistent-policy"},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "lb_not_found",
			vals: url.Values{
				"Action":           {"DeleteLoadBalancerPolicy"},
				"Version":          {"2012-06-01"},
				"LoadBalancerName": {"no-lb"},
				"PolicyName":       {"some-policy"},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "missing_lb_name",
			vals: url.Values{
				"Action":     {"DeleteLoadBalancerPolicy"},
				"Version":    {"2012-06-01"},
				"PolicyName": {"some-policy"},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "missing_policy_name",
			vals: url.Values{
				"Action":           {"DeleteLoadBalancerPolicy"},
				"Version":          {"2012-06-01"},
				"LoadBalancerName": {"some-lb"},
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

// TestDescribeLoadBalancerPolicies tests retrieving LB policies.
func TestDescribeLoadBalancerPolicies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup         func(t *testing.T, h *elb.Handler)
		vals          url.Values
		name          string
		wantPolicyLen int
		wantStatus    int
	}{
		{
			name: "describe_all_policies_for_lb",
			setup: func(t *testing.T, h *elb.Handler) {
				t.Helper()
				mustCreateLB(t, h, "pol-lb")
				doELB(t, h, url.Values{
					"Action":           {"CreateLoadBalancerPolicy"},
					"Version":          {"2012-06-01"},
					"LoadBalancerName": {"pol-lb"},
					"PolicyName":       {"pol-a"},
					"PolicyTypeName":   {"ProxyProtocolPolicyType"},
				})
				doELB(t, h, url.Values{
					"Action":           {"CreateLoadBalancerPolicy"},
					"Version":          {"2012-06-01"},
					"LoadBalancerName": {"pol-lb"},
					"PolicyName":       {"pol-b"},
					"PolicyTypeName":   {"ProxyProtocolPolicyType"},
				})
			},
			vals: url.Values{
				"Action":           {"DescribeLoadBalancerPolicies"},
				"Version":          {"2012-06-01"},
				"LoadBalancerName": {"pol-lb"},
			},
			wantStatus:    http.StatusOK,
			wantPolicyLen: 2,
		},
		{
			name: "describe_policies_by_name",
			setup: func(t *testing.T, h *elb.Handler) {
				t.Helper()
				mustCreateLB(t, h, "pol2-lb")
				doELB(t, h, url.Values{
					"Action":           {"CreateLoadBalancerPolicy"},
					"Version":          {"2012-06-01"},
					"LoadBalancerName": {"pol2-lb"},
					"PolicyName":       {"target-pol"},
					"PolicyTypeName":   {"ProxyProtocolPolicyType"},
				})
				doELB(t, h, url.Values{
					"Action":           {"CreateLoadBalancerPolicy"},
					"Version":          {"2012-06-01"},
					"LoadBalancerName": {"pol2-lb"},
					"PolicyName":       {"other-pol"},
					"PolicyTypeName":   {"ProxyProtocolPolicyType"},
				})
			},
			vals: url.Values{
				"Action":               {"DescribeLoadBalancerPolicies"},
				"Version":              {"2012-06-01"},
				"LoadBalancerName":     {"pol2-lb"},
				"PolicyNames.member.1": {"target-pol"},
			},
			wantStatus:    http.StatusOK,
			wantPolicyLen: 1,
		},
		{
			// When no LoadBalancerName is given, AWS returns only the
			// built-in sample SSL policies (not customer policies).
			name: "describe_no_lb_name_returns_sample_policies",
			vals: url.Values{
				"Action":  {"DescribeLoadBalancerPolicies"},
				"Version": {"2012-06-01"},
			},
			wantStatus:    http.StatusOK,
			wantPolicyLen: 4,
		},
		{
			name: "lb_not_found",
			vals: url.Values{
				"Action":           {"DescribeLoadBalancerPolicies"},
				"Version":          {"2012-06-01"},
				"LoadBalancerName": {"no-lb"},
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

			if tt.wantPolicyLen >= 0 && tt.wantStatus == http.StatusOK {
				var resp struct {
					XMLName xml.Name `xml:"DescribeLoadBalancerPoliciesResponse"`
					Result  struct {
						PolicyDescriptions struct {
							Members []struct {
								PolicyName string `xml:"PolicyName"`
							} `xml:"member"`
						} `xml:"PolicyDescriptions"`
					} `xml:"DescribeLoadBalancerPoliciesResult"`
				}
				parseXMLBody(t, rec, &resp)
				assert.Len(t, resp.Result.PolicyDescriptions.Members, tt.wantPolicyLen)
			}
		})
	}
}

// TestDescribeLoadBalancerPolicyTypes tests retrieving policy type descriptions.
func TestDescribeLoadBalancerPolicyTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		vals         url.Values
		name         string
		wantTypeName string
		wantTypeLen  int
		wantStatus   int
	}{
		{
			name: "returns_all_policy_types",
			vals: url.Values{
				"Action":  {"DescribeLoadBalancerPolicyTypes"},
				"Version": {"2012-06-01"},
			},
			wantStatus:  http.StatusOK,
			wantTypeLen: 6,
		},
		{
			name: "filters_by_type_name",
			vals: url.Values{
				"Action":                   {"DescribeLoadBalancerPolicyTypes"},
				"Version":                  {"2012-06-01"},
				"PolicyTypeNames.member.1": {"AppCookieStickinessPolicyType"},
			},
			wantStatus:   http.StatusOK,
			wantTypeLen:  1,
			wantTypeName: "AppCookieStickinessPolicyType",
		},
		{
			name: "returns_lb_cookie_type",
			vals: url.Values{
				"Action":                   {"DescribeLoadBalancerPolicyTypes"},
				"Version":                  {"2012-06-01"},
				"PolicyTypeNames.member.1": {"LBCookieStickinessPolicyType"},
			},
			wantStatus:   http.StatusOK,
			wantTypeLen:  1,
			wantTypeName: "LBCookieStickinessPolicyType",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			rec := doELB(t, h, tt.vals)
			assert.Equal(t, tt.wantStatus, rec.Code)

			var resp struct {
				XMLName xml.Name `xml:"DescribeLoadBalancerPolicyTypesResponse"`
				Result  struct {
					PolicyTypeDescriptions struct {
						Members []struct {
							PolicyTypeName string `xml:"PolicyTypeName"`
						} `xml:"member"`
					} `xml:"PolicyTypeDescriptions"`
				} `xml:"DescribeLoadBalancerPolicyTypesResult"`
			}
			parseXMLBody(t, rec, &resp)
			assert.Len(t, resp.Result.PolicyTypeDescriptions.Members, tt.wantTypeLen)

			if tt.wantTypeName != "" {
				require.NotEmpty(t, resp.Result.PolicyTypeDescriptions.Members)
				assert.Equal(t, tt.wantTypeName, resp.Result.PolicyTypeDescriptions.Members[0].PolicyTypeName)
			}
		})
	}
}

func TestAppCookieStickinessLifecycle(t *testing.T) {
	t.Parallel()

	b := newBackend()
	h := elb.NewHandler(b)
	mustCreateLB(t, h, "app-cookie-lb")

	// Create.
	rec := doELB(t, h, url.Values{
		"Action":           {"CreateAppCookieStickinessPolicy"},
		"Version":          {"2012-06-01"},
		"LoadBalancerName": {"app-cookie-lb"},
		"PolicyName":       {"app-pol"},
		"CookieName":       {"JSESSIONID"},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, b.PolicyCount())

	// Describe.
	rec2 := doELB(t, h, url.Values{
		"Action":           {"DescribeLoadBalancerPolicies"},
		"Version":          {"2012-06-01"},
		"LoadBalancerName": {"app-cookie-lb"},
	})
	require.Equal(t, http.StatusOK, rec2.Code)

	var pResp struct {
		XMLName xml.Name `xml:"DescribeLoadBalancerPoliciesResponse"`
		Result  struct {
			PolicyDescriptions struct {
				Members []struct {
					PolicyName     string `xml:"PolicyName"`
					PolicyTypeName string `xml:"PolicyTypeName"`
				} `xml:"member"`
			} `xml:"PolicyDescriptions"`
		} `xml:"DescribeLoadBalancerPoliciesResult"`
	}
	require.NoError(t, xml.Unmarshal(rec2.Body.Bytes(), &pResp))
	require.Len(t, pResp.Result.PolicyDescriptions.Members, 1)
	assert.Equal(t, "app-pol", pResp.Result.PolicyDescriptions.Members[0].PolicyName)
	assert.Equal(t, "AppCookieStickinessPolicyType", pResp.Result.PolicyDescriptions.Members[0].PolicyTypeName)

	// Delete.
	rec3 := doELB(t, h, url.Values{
		"Action":           {"DeleteLoadBalancerPolicy"},
		"Version":          {"2012-06-01"},
		"LoadBalancerName": {"app-cookie-lb"},
		"PolicyName":       {"app-pol"},
	})
	require.Equal(t, http.StatusOK, rec3.Code)
	assert.Equal(t, 0, b.PolicyCount())
}

func TestLBCookieStickinessNoExpiry(t *testing.T) {
	t.Parallel()

	b := newBackend()
	h := elb.NewHandler(b)
	mustCreateLB(t, h, "lb-cookie-lb")

	rec := doELB(t, h, url.Values{
		"Action":           {"CreateLBCookieStickinessPolicy"},
		"Version":          {"2012-06-01"},
		"LoadBalancerName": {"lb-cookie-lb"},
		"PolicyName":       {"lb-pol"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doELB(t, h, url.Values{
		"Action":               {"DescribeLoadBalancerPolicies"},
		"Version":              {"2012-06-01"},
		"LoadBalancerName":     {"lb-cookie-lb"},
		"PolicyNames.member.1": {"lb-pol"},
	})
	require.Equal(t, http.StatusOK, rec2.Code)

	var resp struct {
		XMLName xml.Name `xml:"DescribeLoadBalancerPoliciesResponse"`
		Result  struct {
			PolicyDescriptions struct {
				Members []struct {
					PolicyTypeName              string `xml:"PolicyTypeName"`
					PolicyAttributeDescriptions struct {
						Members []struct {
							AttributeName  string `xml:"AttributeName"`
							AttributeValue string `xml:"AttributeValue"`
						} `xml:"member"`
					} `xml:"PolicyAttributeDescriptions"`
				} `xml:"member"`
			} `xml:"PolicyDescriptions"`
		} `xml:"DescribeLoadBalancerPoliciesResult"`
	}
	require.NoError(t, xml.Unmarshal(rec2.Body.Bytes(), &resp))
	require.Len(t, resp.Result.PolicyDescriptions.Members, 1)
	p := resp.Result.PolicyDescriptions.Members[0]
	assert.Equal(t, "LBCookieStickinessPolicyType", p.PolicyTypeName)
	require.Len(t, p.PolicyAttributeDescriptions.Members, 1)
	assert.Equal(t, "CookieExpirationPeriod", p.PolicyAttributeDescriptions.Members[0].AttributeName)
	assert.Empty(t, p.PolicyAttributeDescriptions.Members[0].AttributeValue)
}

func TestLBCookieStickinessWithExpiry(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	mustCreateLB(t, h, "lb-exp-lb")

	rec := doELB(t, h, url.Values{
		"Action":                 {"CreateLBCookieStickinessPolicy"},
		"Version":                {"2012-06-01"},
		"LoadBalancerName":       {"lb-exp-lb"},
		"PolicyName":             {"lb-exp-pol"},
		"CookieExpirationPeriod": {"3600"},
	})
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestPolicyTypesListAll(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	rec := doELB(t, h, url.Values{
		"Action":  {"DescribeLoadBalancerPolicyTypes"},
		"Version": {"2012-06-01"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		XMLName xml.Name `xml:"DescribeLoadBalancerPolicyTypesResponse"`
		Result  struct {
			PolicyTypeDescriptions struct {
				Members []struct {
					PolicyTypeName string `xml:"PolicyTypeName"`
				} `xml:"member"`
			} `xml:"PolicyTypeDescriptions"`
		} `xml:"DescribeLoadBalancerPolicyTypesResult"`
	}
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))

	names := make([]string, 0, len(resp.Result.PolicyTypeDescriptions.Members))
	for _, m := range resp.Result.PolicyTypeDescriptions.Members {
		names = append(names, m.PolicyTypeName)
	}

	assert.Contains(t, names, "AppCookieStickinessPolicyType")
	assert.Contains(t, names, "LBCookieStickinessPolicyType")
	assert.Contains(t, names, "SSLNegotiationPolicyType")
	assert.Contains(t, names, "ProxyProtocolPolicyType")
	assert.Contains(t, names, "BackendServerAuthenticationPolicyType")
}

func TestPolicyTypesFilterByName(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	rec := doELB(t, h, url.Values{
		"Action":                   {"DescribeLoadBalancerPolicyTypes"},
		"Version":                  {"2012-06-01"},
		"PolicyTypeNames.member.1": {"SSLNegotiationPolicyType"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		XMLName xml.Name `xml:"DescribeLoadBalancerPolicyTypesResponse"`
		Result  struct {
			PolicyTypeDescriptions struct {
				Members []struct {
					PolicyTypeName string `xml:"PolicyTypeName"`
				} `xml:"member"`
			} `xml:"PolicyTypeDescriptions"`
		} `xml:"DescribeLoadBalancerPolicyTypesResult"`
	}
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Result.PolicyTypeDescriptions.Members, 1)
	assert.Equal(t, "SSLNegotiationPolicyType", resp.Result.PolicyTypeDescriptions.Members[0].PolicyTypeName)
}

func TestPolicyTypesUnknownReturnsError(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	rec := doELB(t, h, url.Values{
		"Action":                   {"DescribeLoadBalancerPolicyTypes"},
		"Version":                  {"2012-06-01"},
		"PolicyTypeNames.member.1": {"NoSuchPolicyType"},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestSSLCipherPolicyCreateAndDescribe(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	mustCreateLB(t, h, "ssl-cipher-lb")

	rec := doELB(t, h, url.Values{
		"Action":           {"CreateLoadBalancerPolicy"},
		"Version":          {"2012-06-01"},
		"LoadBalancerName": {"ssl-cipher-lb"},
		"PolicyName":       {"my-ssl-pol"},
		"PolicyTypeName":   {"SSLNegotiationPolicyType"},
		"PolicyAttributes.member.1.AttributeName":  {"Protocol-TLSv1.2"},
		"PolicyAttributes.member.1.AttributeValue": {"true"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doELB(t, h, url.Values{
		"Action":               {"DescribeLoadBalancerPolicies"},
		"Version":              {"2012-06-01"},
		"LoadBalancerName":     {"ssl-cipher-lb"},
		"PolicyNames.member.1": {"my-ssl-pol"},
	})
	require.Equal(t, http.StatusOK, rec2.Code)

	var resp struct {
		XMLName xml.Name `xml:"DescribeLoadBalancerPoliciesResponse"`
		Result  struct {
			PolicyDescriptions struct {
				Members []struct {
					PolicyTypeName              string `xml:"PolicyTypeName"`
					PolicyAttributeDescriptions struct {
						Members []struct {
							AttributeName  string `xml:"AttributeName"`
							AttributeValue string `xml:"AttributeValue"`
						} `xml:"member"`
					} `xml:"PolicyAttributeDescriptions"`
				} `xml:"member"`
			} `xml:"PolicyDescriptions"`
		} `xml:"DescribeLoadBalancerPoliciesResult"`
	}
	require.NoError(t, xml.Unmarshal(rec2.Body.Bytes(), &resp))
	require.Len(t, resp.Result.PolicyDescriptions.Members, 1)
	p := resp.Result.PolicyDescriptions.Members[0]
	assert.Equal(t, "SSLNegotiationPolicyType", p.PolicyTypeName)
	require.Len(t, p.PolicyAttributeDescriptions.Members, 1)
	assert.Equal(t, "Protocol-TLSv1.2", p.PolicyAttributeDescriptions.Members[0].AttributeName)
}

func TestDeletePolicyInUseByListenerRejected(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	mustCreateLB(t, h, "del-inuse-lb")

	doELB(t, h, url.Values{
		"Action":           {"CreateLBCookieStickinessPolicy"},
		"Version":          {"2012-06-01"},
		"LoadBalancerName": {"del-inuse-lb"},
		"PolicyName":       {"in-use-pol"},
	})

	doELB(t, h, url.Values{
		"Action":               {"SetLoadBalancerPoliciesOfListener"},
		"Version":              {"2012-06-01"},
		"LoadBalancerName":     {"del-inuse-lb"},
		"LoadBalancerPort":     {"80"},
		"PolicyNames.member.1": {"in-use-pol"},
	})

	rec := doELB(t, h, url.Values{
		"Action":           {"DeleteLoadBalancerPolicy"},
		"Version":          {"2012-06-01"},
		"LoadBalancerName": {"del-inuse-lb"},
		"PolicyName":       {"in-use-pol"},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestDeletePolicyAfterClearOk(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	mustCreateLB(t, h, "del-clear-lb")

	doELB(t, h, url.Values{
		"Action":           {"CreateLBCookieStickinessPolicy"},
		"Version":          {"2012-06-01"},
		"LoadBalancerName": {"del-clear-lb"},
		"PolicyName":       {"clear-pol"},
	})

	doELB(t, h, url.Values{
		"Action":               {"SetLoadBalancerPoliciesOfListener"},
		"Version":              {"2012-06-01"},
		"LoadBalancerName":     {"del-clear-lb"},
		"LoadBalancerPort":     {"80"},
		"PolicyNames.member.1": {"clear-pol"},
	})

	// Clear policies from listener.
	doELB(t, h, url.Values{
		"Action":           {"SetLoadBalancerPoliciesOfListener"},
		"Version":          {"2012-06-01"},
		"LoadBalancerName": {"del-clear-lb"},
		"LoadBalancerPort": {"80"},
	})

	// Now delete should succeed.
	rec := doELB(t, h, url.Values{
		"Action":           {"DeleteLoadBalancerPolicy"},
		"Version":          {"2012-06-01"},
		"LoadBalancerName": {"del-clear-lb"},
		"PolicyName":       {"clear-pol"},
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestDescribePoliciesFilterByPolicyName(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	mustCreateLB(t, h, "filter-pol-lb")

	for _, name := range []string{"pol-a", "pol-b", "pol-c"} {
		doELB(t, h, url.Values{
			"Action":           {"CreateLBCookieStickinessPolicy"},
			"Version":          {"2012-06-01"},
			"LoadBalancerName": {"filter-pol-lb"},
			"PolicyName":       {name},
		})
	}

	rec := doELB(t, h, url.Values{
		"Action":               {"DescribeLoadBalancerPolicies"},
		"Version":              {"2012-06-01"},
		"LoadBalancerName":     {"filter-pol-lb"},
		"PolicyNames.member.1": {"pol-b"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		XMLName xml.Name `xml:"DescribeLoadBalancerPoliciesResponse"`
		Result  struct {
			PolicyDescriptions struct {
				Members []struct {
					PolicyName string `xml:"PolicyName"`
				} `xml:"member"`
			} `xml:"PolicyDescriptions"`
		} `xml:"DescribeLoadBalancerPoliciesResult"`
	}
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Result.PolicyDescriptions.Members, 1)
	assert.Equal(t, "pol-b", resp.Result.PolicyDescriptions.Members[0].PolicyName)
}

func TestDescribePoliciesUnknownLBReturns400(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	rec := doELB(t, h, url.Values{
		"Action":           {"DescribeLoadBalancerPolicies"},
		"Version":          {"2012-06-01"},
		"LoadBalancerName": {"no-such-lb"},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestStickinessPolicyTCPRejected verifies that attaching a stickiness
// policy to a TCP or SSL listener is rejected.
func TestStickinessPolicyTCPRejected(t *testing.T) {
	t.Parallel()

	const certARN = "arn:aws:iam::123456789012:server-certificate/my-cert"

	tests := []struct {
		name       string
		protocol   string
		certARN    string
		port       string
		wantStatus int
	}{
		{"tcp_listener_rejected", "TCP", "", "80", http.StatusBadRequest},
		{"ssl_listener_rejected", "SSL", certARN, "443", http.StatusBadRequest},
		{"http_listener_accepted", "HTTP", "", "80", http.StatusOK},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			lbName := fmt.Sprintf("sticky-%d", i)

			// Create LB with specified listener.
			vals := url.Values{
				"Action":                              {"CreateLoadBalancer"},
				"Version":                             {"2012-06-01"},
				"LoadBalancerName":                    {lbName},
				"Listeners.member.1.Protocol":         {tt.protocol},
				"Listeners.member.1.LoadBalancerPort": {tt.port},
				"Listeners.member.1.InstancePort":     {"8080"},
				"AvailabilityZones.member.1":          {"us-east-1a"},
			}
			if tt.certARN != "" {
				vals.Set("Listeners.member.1.SSLCertificateId", tt.certARN)
			}

			rec := doELB(t, h, vals)
			require.Equal(t, http.StatusOK, rec.Code)

			// Create a stickiness policy.
			doELB(t, h, url.Values{
				"Action":           {"CreateLBCookieStickinessPolicy"},
				"Version":          {"2012-06-01"},
				"LoadBalancerName": {lbName},
				"PolicyName":       {"sticky-pol"},
			})

			// Attach stickiness policy to the listener.
			portStr := tt.port
			rec = doELB(t, h, url.Values{
				"Action":               {"SetLoadBalancerPoliciesOfListener"},
				"Version":              {"2012-06-01"},
				"LoadBalancerName":     {lbName},
				"LoadBalancerPort":     {portStr},
				"PolicyNames.member.1": {"sticky-pol"},
			})
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestSetLoadBalancerPoliciesOfListener(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(t *testing.T, h *elb.Handler)
		vals         url.Values
		name         string
		wantPolicies []string
		wantStatus   int
	}{
		{
			name: "sets_policies_on_listener",
			setup: func(t *testing.T, h *elb.Handler) {
				t.Helper()
				mustCreateLB(t, h, "lpol-lb")

				doELB(t, h, url.Values{
					"Action":           {"CreateAppCookieStickinessPolicy"},
					"Version":          {"2012-06-01"},
					"LoadBalancerName": {"lpol-lb"},
					"PolicyName":       {"sticky-pol"},
					"CookieName":       {"SID"},
				})
			},
			vals: url.Values{
				"Action":               {"SetLoadBalancerPoliciesOfListener"},
				"Version":              {"2012-06-01"},
				"LoadBalancerName":     {"lpol-lb"},
				"LoadBalancerPort":     {"80"},
				"PolicyNames.member.1": {"sticky-pol"},
			},
			wantStatus:   http.StatusOK,
			wantPolicies: []string{"sticky-pol"},
		},
		{
			name: "clears_policies_with_empty_list",
			setup: func(t *testing.T, h *elb.Handler) {
				t.Helper()
				mustCreateLB(t, h, "lpol-clear")

				doELB(t, h, url.Values{
					"Action":               {"SetLoadBalancerPoliciesOfListener"},
					"Version":              {"2012-06-01"},
					"LoadBalancerName":     {"lpol-clear"},
					"LoadBalancerPort":     {"80"},
					"PolicyNames.member.1": {"pol-a"},
				})
			},
			vals: url.Values{
				"Action":           {"SetLoadBalancerPoliciesOfListener"},
				"Version":          {"2012-06-01"},
				"LoadBalancerName": {"lpol-clear"},
				"LoadBalancerPort": {"80"},
			},
			wantStatus:   http.StatusOK,
			wantPolicies: []string{},
		},
		{
			name: "missing_lb_name_returns_400",
			vals: url.Values{
				"Action":           {"SetLoadBalancerPoliciesOfListener"},
				"Version":          {"2012-06-01"},
				"LoadBalancerPort": {"80"},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "missing_port_returns_400",
			setup: func(t *testing.T, h *elb.Handler) {
				t.Helper()
				mustCreateLB(t, h, "lpol-noportlb")
			},
			vals: url.Values{
				"Action":           {"SetLoadBalancerPoliciesOfListener"},
				"Version":          {"2012-06-01"},
				"LoadBalancerName": {"lpol-noportlb"},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "listener_not_found_returns_404",
			setup: func(t *testing.T, h *elb.Handler) {
				t.Helper()
				mustCreateLB(t, h, "lpol-noport")
			},
			vals: url.Values{
				"Action":           {"SetLoadBalancerPoliciesOfListener"},
				"Version":          {"2012-06-01"},
				"LoadBalancerName": {"lpol-noport"},
				"LoadBalancerPort": {"9999"},
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

			if tt.wantStatus == http.StatusOK {
				lbName := tt.vals.Get("LoadBalancerName")
				lbs, err := h.Backend.DescribeLoadBalancers(context.Background(), []string{lbName})
				require.NoError(t, err)
				require.Len(t, lbs, 1)

				var gotPolicies []string
				for _, l := range lbs[0].Listeners {
					gotPolicies = l.PolicyNames
				}

				if gotPolicies == nil {
					gotPolicies = []string{}
				}

				assert.Equal(t, tt.wantPolicies, gotPolicies)
			}
		})
	}
}

// TestPolicyNotFoundReturns400 verifies ErrPolicyNotFound maps to HTTP 400.
//
// gopherstack-5gfl: pins existing (possibly wrong) behavior, not endorsed --
// DeleteLoadBalancerPolicy's real typed-error switch doesn't declare
// PolicyNotFound at all (only InvalidConfigurationRequest/LoadBalancerNotFound),
// so a real client would get an untyped error here, not
// *types.PolicyNotFoundException. Left as-is: no AWS documentation confirms
// what the correct behavior actually is (unlike DeleteLoadBalancer, which is
// documented idempotent for a missing target).
func TestPolicyNotFoundReturns400(t *testing.T) {
	t.Parallel()

	b := newBackend()
	h := elb.NewHandler(b)
	mustCreateLB(t, h, "err-lb")

	rec := doELB(t, h, url.Values{
		"Action":           {"DeleteLoadBalancerPolicy"},
		"Version":          {"2012-06-01"},
		"LoadBalancerName": {"err-lb"},
		"PolicyName":       {"no-such-policy"},
	})

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp struct {
		XMLName xml.Name `xml:"ErrorResponse"`
		Error   struct {
			Code string `xml:"Code"`
		} `xml:"Error"`
	}

	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "PolicyNotFound", errResp.Error.Code)
}

// TestPolicyAlreadyExistsReturns400 verifies ErrPolicyAlreadyExists maps to HTTP 400.
func TestPolicyAlreadyExistsReturns400(t *testing.T) {
	t.Parallel()

	b := newBackend()
	h := elb.NewHandler(b)
	mustCreateLB(t, h, "dup-pol-lb")

	doELB(t, h, url.Values{
		"Action":           {"CreateAppCookieStickinessPolicy"},
		"Version":          {"2012-06-01"},
		"LoadBalancerName": {"dup-pol-lb"},
		"PolicyName":       {"my-pol"},
		"CookieName":       {"SID"},
	})

	rec := doELB(t, h, url.Values{
		"Action":           {"CreateAppCookieStickinessPolicy"},
		"Version":          {"2012-06-01"},
		"LoadBalancerName": {"dup-pol-lb"},
		"PolicyName":       {"my-pol"},
		"CookieName":       {"SID"},
	})

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp struct {
		XMLName xml.Name `xml:"ErrorResponse"`
		Error   struct {
			Code string `xml:"Code"`
		} `xml:"Error"`
	}

	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "DuplicatePolicyName", errResp.Error.Code)
}

// TestDescribeLoadBalancerPolicyTypesUnknownReturnsError verifies that
// requesting an unknown policy type returns an error (not a silent empty list).
func TestDescribeLoadBalancerPolicyTypesUnknownReturnsError(t *testing.T) {
	t.Parallel()

	b := newBackend()
	_, err := b.DescribeLoadBalancerPolicyTypes(context.Background(), []string{"NoSuchPolicyType"})
	require.Error(t, err)
	require.ErrorIs(t, err, elb.ErrPolicyTypeNotFound)
}

// TestPolicyNameValidation verifies policy name validation in create policy handlers.
func TestPolicyNameValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		vals       url.Values
		name       string
		wantStatus int
	}{
		{
			name: "empty_policy_name_rejected",
			vals: url.Values{
				"Action":           {"CreateAppCookieStickinessPolicy"},
				"Version":          {"2012-06-01"},
				"LoadBalancerName": {"pol-val-lb"},
				"PolicyName":       {""},
				"CookieName":       {"SESS"},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "policy_name_too_long_rejected",
			vals: url.Values{
				"Action":           {"CreateAppCookieStickinessPolicy"},
				"Version":          {"2012-06-01"},
				"LoadBalancerName": {"pol-val-lb"},
				"PolicyName":       {"a12345678901234567890123456789012"},
				"CookieName":       {"SESS"},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "valid_policy_name_accepted",
			vals: url.Values{
				"Action":           {"CreateAppCookieStickinessPolicy"},
				"Version":          {"2012-06-01"},
				"LoadBalancerName": {"pol-val-lb"},
				"PolicyName":       {"valid-policy-1"},
				"CookieName":       {"SESS"},
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend()
			h := elb.NewHandler(b)
			mustCreateLB(t, h, "pol-val-lb")

			rec := doELB(t, h, tt.vals)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestCookieNameRequired verifies AppCookieStickinessPolicy requires CookieName.
func TestCookieNameRequired(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		cookieName string
		wantStatus int
	}{
		{
			name:       "empty_cookie_name_rejected",
			cookieName: "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "non_empty_cookie_name_accepted",
			cookieName: "JSESSIONID",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend()
			h := elb.NewHandler(b)
			mustCreateLB(t, h, "cookie-lb")

			rec := doELB(t, h, url.Values{
				"Action":           {"CreateAppCookieStickinessPolicy"},
				"Version":          {"2012-06-01"},
				"LoadBalancerName": {"cookie-lb"},
				"PolicyName":       {"my-cookie-pol"},
				"CookieName":       {tt.cookieName},
			})
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestSetPoliciesOfListenerRejectsUnknownPolicy verifies that
// SetLoadBalancerPoliciesOfListener returns an error for unknown policy names.
func TestSetPoliciesOfListenerRejectsUnknownPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(t *testing.T, h *elb.Handler)
		name       string
		policyName string
		wantCode   string
		wantStatus int
	}{
		{
			name: "unknown_policy_rejected",
			setup: func(t *testing.T, h *elb.Handler) {
				t.Helper()
				mustCreateLB(t, h, "set-pol-lb")
			},
			policyName: "no-such-policy",
			wantStatus: http.StatusBadRequest,
			wantCode:   "PolicyNotFound",
		},
		{
			name: "known_policy_accepted",
			setup: func(t *testing.T, h *elb.Handler) {
				t.Helper()
				mustCreateLB(t, h, "set-pol-lb")
				doELB(t, h, url.Values{
					"Action":           {"CreateLBCookieStickinessPolicy"},
					"Version":          {"2012-06-01"},
					"LoadBalancerName": {"set-pol-lb"},
					"PolicyName":       {"lb-cookie-pol"},
				})
			},
			policyName: "lb-cookie-pol",
			wantStatus: http.StatusOK,
			wantCode:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend()
			h := elb.NewHandler(b)
			tt.setup(t, h)

			rec := doELB(t, h, url.Values{
				"Action":               {"SetLoadBalancerPoliciesOfListener"},
				"Version":              {"2012-06-01"},
				"LoadBalancerName":     {"set-pol-lb"},
				"LoadBalancerPort":     {"80"},
				"PolicyNames.member.1": {tt.policyName},
			})

			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantCode != "" {
				var errResp struct {
					XMLName xml.Name `xml:"ErrorResponse"`
					Error   struct {
						Code string `xml:"Code"`
					} `xml:"Error"`
				}
				require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &errResp))
				assert.Equal(t, tt.wantCode, errResp.Error.Code)
			}
		})
	}
}

// TestDescribePoliciesAllPoliciesWhenNoName verifies that
// DescribeLoadBalancerPolicies with no LoadBalancerName returns the built-in
// sample SSL policies (AWS behaviour: customer policies are NOT included).
func TestDescribePoliciesAllPoliciesWhenNoName(t *testing.T) {
	t.Parallel()

	b := newBackend()
	h := elb.NewHandler(b)

	// Create some customer policies — these should NOT appear in the global result.
	mustCreateLB(t, h, "multi-pol-lb1")
	mustCreateLB(t, h, "multi-pol-lb2")

	doELB(t, h, url.Values{
		"Action":           {"CreateLBCookieStickinessPolicy"},
		"Version":          {"2012-06-01"},
		"LoadBalancerName": {"multi-pol-lb1"},
		"PolicyName":       {"pol-lb1"},
	})

	doELB(t, h, url.Values{
		"Action":           {"CreateLBCookieStickinessPolicy"},
		"Version":          {"2012-06-01"},
		"LoadBalancerName": {"multi-pol-lb2"},
		"PolicyName":       {"pol-lb2"},
	})

	rec := doELB(t, h, url.Values{
		"Action":  {"DescribeLoadBalancerPolicies"},
		"Version": {"2012-06-01"},
	})

	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		XMLName xml.Name `xml:"DescribeLoadBalancerPoliciesResponse"`
		Result  struct {
			PolicyDescriptions struct {
				Members []struct {
					PolicyName string `xml:"PolicyName"`
				} `xml:"member"`
			} `xml:"PolicyDescriptions"`
		} `xml:"DescribeLoadBalancerPoliciesResult"`
	}

	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))

	// Exactly 4 built-in sample policies are returned; customer policies are excluded.
	assert.Len(t, resp.Result.PolicyDescriptions.Members, 4)

	names := make([]string, 0, 4)
	for _, m := range resp.Result.PolicyDescriptions.Members {
		names = append(names, m.PolicyName)
	}

	assert.Contains(t, names, "ELBSample-ELBDefaultNegotiationPolicy")
	assert.Contains(t, names, "ELBSample-OpenSSLDefaultCipherPolicy")
	assert.Contains(t, names, "ELBSecurityPolicy-2016-08")
	assert.Contains(t, names, "ELBSecurityPolicy-TLS-1-2-2017-01")
}

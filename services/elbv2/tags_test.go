package elbv2_test

import (
	"encoding/xml"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/elbv2"
)

// TestAddAndDescribeTags tests tag operations.
func TestAddAndDescribeTags(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	lbArn := mustCreateLB(t, h, "tagged-lb")

	rec := doELBv2(t, h, url.Values{
		"Action":                {"AddTags"},
		"Version":               {"2015-12-01"},
		"ResourceArns.member.1": {lbArn},
		"Tags.member.1.Key":     {"env"},
		"Tags.member.1.Value":   {"test"},
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	rec2 := doELBv2(t, h, url.Values{
		"Action":                {"DescribeTags"},
		"Version":               {"2015-12-01"},
		"ResourceArns.member.1": {lbArn},
	})
	require.Equal(t, http.StatusOK, rec2.Code)

	var resp struct {
		Result struct {
			TagDescriptions struct {
				Members []struct {
					ResourceArn string `xml:"ResourceArn"`
					Tags        struct {
						Members []struct {
							Key   string `xml:"Key"`
							Value string `xml:"Value"`
						} `xml:"member"`
					} `xml:"Tags"`
				} `xml:"member"`
			} `xml:"TagDescriptions"`
		} `xml:"DescribeTagsResult"`
	}
	parseXMLBody(t, rec2, &resp)
	require.Len(t, resp.Result.TagDescriptions.Members, 1)
	assert.Equal(t, lbArn, resp.Result.TagDescriptions.Members[0].ResourceArn)

	found := false
	for _, tag := range resp.Result.TagDescriptions.Members[0].Tags.Members {
		if tag.Key == "env" && tag.Value == "test" {
			found = true
		}
	}
	assert.True(t, found, "expected tag env=test to be present")
}

// TestRemoveTags tests tag removal.
func TestRemoveTags(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	lbArn := mustCreateLB(t, h, "untag-lb")

	doELBv2(t, h, url.Values{
		"Action":                {"AddTags"},
		"Version":               {"2015-12-01"},
		"ResourceArns.member.1": {lbArn},
		"Tags.member.1.Key":     {"remove-me"},
		"Tags.member.1.Value":   {"yes"},
	})

	rec := doELBv2(t, h, url.Values{
		"Action":                {"RemoveTags"},
		"Version":               {"2015-12-01"},
		"ResourceArns.member.1": {lbArn},
		"TagKeys.member.1":      {"remove-me"},
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestAddTagsMissingResourceArns tests AddTags with no resource ARNs.
func TestAddTagsMissingResourceArns(t *testing.T) {
	t.Parallel()

	h := newTestHandler()

	rec := doELBv2(t, h, url.Values{
		"Action":  {"AddTags"},
		"Version": {"2015-12-01"},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestRemoveTagsMissingResourceArns tests RemoveTags with no resource ARNs.
func TestRemoveTagsMissingResourceArns(t *testing.T) {
	t.Parallel()

	h := newTestHandler()

	rec := doELBv2(t, h, url.Values{
		"Action":  {"RemoveTags"},
		"Version": {"2015-12-01"},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestDescribeTagsMissingResourceArns tests DescribeTags with no resource ARNs.
func TestDescribeTagsMissingResourceArns(t *testing.T) {
	t.Parallel()

	h := newTestHandler()

	rec := doELBv2(t, h, url.Values{
		"Action":  {"DescribeTags"},
		"Version": {"2015-12-01"},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestDescribeTagsForTargetGroupAndListener tests describe tags for TG and listener ARNs.
func TestDescribeTagsForTargetGroupAndListener(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	tgArn := mustCreateTG(t, h, "tag-tg")

	doELBv2(t, h, url.Values{
		"Action":                {"AddTags"},
		"Version":               {"2015-12-01"},
		"ResourceArns.member.1": {tgArn},
		"Tags.member.1.Key":     {"service"},
		"Tags.member.1.Value":   {"web"},
	})

	rec := doELBv2(t, h, url.Values{
		"Action":                {"DescribeTags"},
		"Version":               {"2015-12-01"},
		"ResourceArns.member.1": {tgArn},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Result struct {
			TagDescriptions struct {
				Members []struct {
					ResourceArn string `xml:"ResourceArn"`
					Tags        struct {
						Members []struct {
							Key   string `xml:"Key"`
							Value string `xml:"Value"`
						} `xml:"member"`
					} `xml:"Tags"`
				} `xml:"member"`
			} `xml:"TagDescriptions"`
		} `xml:"DescribeTagsResult"`
	}
	parseXMLBody(t, rec, &resp)
	assert.Len(t, resp.Result.TagDescriptions.Members, 1)
}

// AWS: DescribeTags models LoadBalancerNotFound/TargetGroupNotFound/
// ListenerNotFound/RuleNotFound/TrustStoreNotFound for a resource ARN that
// does not exist -- it must raise, not silently omit the unknown ARN.
func TestDescribeTags_UnknownResourceArn_Errors(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	tgArn := mustCreateTG(t, h, "tag-tg-unknown-sibling")

	rec := doELBv2(t, h, url.Values{
		"Action":                {"DescribeTags"},
		"Version":               {"2015-12-01"},
		"ResourceArns.member.1": {tgArn},
		"ResourceArns.member.2": {"arn:aws:doesnotexist"},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestRemoveTagsFromTG tests removing tags from a target group.
func TestRemoveTagsFromTG(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	tgArn := mustCreateTG(t, h, "rm-tag-tg")

	doELBv2(t, h, url.Values{
		"Action":                {"AddTags"},
		"Version":               {"2015-12-01"},
		"ResourceArns.member.1": {tgArn},
		"Tags.member.1.Key":     {"to-remove"},
		"Tags.member.1.Value":   {"yes"},
	})

	rec := doELBv2(t, h, url.Values{
		"Action":                {"RemoveTags"},
		"Version":               {"2015-12-01"},
		"ResourceArns.member.1": {tgArn},
		"TagKeys.member.1":      {"to-remove"},
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestRemoveTagsFromListener tests removing tags from a listener.
func TestRemoveTagsFromListener(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	lbArn := mustCreateLB(t, h, "rm-tag-listener-lb")
	tgArn := mustCreateTG(t, h, "rm-tag-listener-tg")

	listenerRec := doELBv2(t, h, url.Values{
		"Action":                                 {"CreateListener"},
		"Version":                                {"2015-12-01"},
		"LoadBalancerArn":                        {lbArn},
		"Protocol":                               {"HTTP"},
		"Port":                                   {"80"},
		"DefaultActions.member.1.Type":           {"forward"},
		"DefaultActions.member.1.TargetGroupArn": {tgArn},
	})
	require.Equal(t, http.StatusOK, listenerRec.Code)

	var listenerResp struct {
		Result struct {
			Listeners struct {
				Members []struct {
					ListenerArn string `xml:"ListenerArn"`
				} `xml:"member"`
			} `xml:"Listeners"`
		} `xml:"CreateListenerResult"`
	}
	require.NoError(t, xml.Unmarshal(listenerRec.Body.Bytes(), &listenerResp))
	listenerArn := listenerResp.Result.Listeners.Members[0].ListenerArn

	doELBv2(t, h, url.Values{
		"Action":                {"AddTags"},
		"Version":               {"2015-12-01"},
		"ResourceArns.member.1": {listenerArn},
		"Tags.member.1.Key":     {"listener-tag"},
		"Tags.member.1.Value":   {"yes"},
	})

	rec := doELBv2(t, h, url.Values{
		"Action":                {"RemoveTags"},
		"Version":               {"2015-12-01"},
		"ResourceArns.member.1": {listenerArn},
		"TagKeys.member.1":      {"listener-tag"},
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestELBv2_StubOperations validates that stub operations return 200 for valid inputs.
func TestELBv2_StubOperations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(t *testing.T, h *elbv2.Handler) url.Values
		name       string
		wantStatus int
	}{
		{
			name: "get_resource_policy",
			setup: func(t *testing.T, h *elbv2.Handler) url.Values {
				t.Helper()
				lbArn := mustCreateLB(t, h, "rp-lb")

				return url.Values{
					"Action":      {"GetResourcePolicy"},
					"Version":     {"2015-12-01"},
					"ResourceArn": {lbArn},
				}
			},
			// No resource policy is set, so AWS returns ResourceNotFound (HTTP 400, AWS query-protocol status).
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "get_resource_policy_missing_arn",
			setup: func(t *testing.T, _ *elbv2.Handler) url.Values {
				t.Helper()

				return url.Values{
					"Action":  {"GetResourcePolicy"},
					"Version": {"2015-12-01"},
				}
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "get_trust_store_ca_bundle",
			setup: func(t *testing.T, h *elbv2.Handler) url.Values {
				t.Helper()

				createRec := doELBv2(t, h, url.Values{
					"Action":  {"CreateTrustStore"},
					"Version": {"2015-12-01"},
					"Name":    {"stub-ts"},
				})
				require.Equal(t, http.StatusOK, createRec.Code)

				var resp struct {
					Result struct {
						TrustStores struct {
							Members []struct {
								TrustStoreArn string `xml:"TrustStoreArn"`
							} `xml:"member"`
						} `xml:"TrustStores"`
					} `xml:"CreateTrustStoreResult"`
				}
				require.NoError(t, xml.Unmarshal(createRec.Body.Bytes(), &resp))

				return url.Values{
					"Action":        {"GetTrustStoreCaCertificatesBundle"},
					"Version":       {"2015-12-01"},
					"TrustStoreArn": {resp.Result.TrustStores.Members[0].TrustStoreArn},
				}
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "get_trust_store_ca_bundle_not_found",
			setup: func(t *testing.T, _ *elbv2.Handler) url.Values {
				t.Helper()

				return url.Values{
					"Action":        {"GetTrustStoreCaCertificatesBundle"},
					"Version":       {"2015-12-01"},
					"TrustStoreArn": {"arn:aws:elasticloadbalancing:us-east-1:123:truststore/nonexistent/abc"},
				}
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "modify_capacity_reservation",
			setup: func(t *testing.T, h *elbv2.Handler) url.Values {
				t.Helper()
				lbArn := mustCreateLB(t, h, "cap-mod-lb")

				return url.Values{
					"Action":          {"ModifyCapacityReservation"},
					"Version":         {"2015-12-01"},
					"LoadBalancerArn": {lbArn},
				}
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "modify_capacity_reservation_not_found",
			setup: func(t *testing.T, _ *elbv2.Handler) url.Values {
				t.Helper()

				return url.Values{
					"Action":          {"ModifyCapacityReservation"},
					"Version":         {"2015-12-01"},
					"LoadBalancerArn": {"arn:aws:elasticloadbalancing:us-east-1:123:loadbalancer/app/nonexistent/abc"},
				}
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "modify_ip_pools",
			setup: func(t *testing.T, h *elbv2.Handler) url.Values {
				t.Helper()
				lbArn := mustCreateLB(t, h, "ip-pools-lb")

				return url.Values{
					"Action":          {"ModifyIpPools"},
					"Version":         {"2015-12-01"},
					"LoadBalancerArn": {lbArn},
				}
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			vals := tt.setup(t, h)

			rec := doELBv2(t, h, vals)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestAddTags_LB(t *testing.T) {
	t.Parallel()

	h := newBatch1Handler()
	lbArn := b1CreateLB(t, h, "tag-lb")

	rec := doELBv2(t, h, url.Values{
		"Action":                {"AddTags"},
		"Version":               {"2015-12-01"},
		"ResourceArns.member.1": {lbArn},
		"Tags.member.1.Key":     {"env"},
		"Tags.member.1.Value":   {"prod"},
		"Tags.member.2.Key":     {"team"},
		"Tags.member.2.Value":   {"platform"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doELBv2(t, h, url.Values{
		"Action":                {"DescribeTags"},
		"Version":               {"2015-12-01"},
		"ResourceArns.member.1": {lbArn},
	})
	require.Equal(t, http.StatusOK, rec2.Code)
	body := rec2.Body.String()
	assert.Contains(t, body, "env")
	assert.Contains(t, body, "prod")
	assert.Contains(t, body, "team")
	assert.Contains(t, body, "platform")
}

func TestRemoveTags_LB(t *testing.T) {
	t.Parallel()

	h := newBatch1Handler()
	lbArn := b1CreateLB(t, h, "remove-tag-lb")

	doELBv2(t, h, url.Values{
		"Action":                {"AddTags"},
		"Version":               {"2015-12-01"},
		"ResourceArns.member.1": {lbArn},
		"Tags.member.1.Key":     {"to-remove"},
		"Tags.member.1.Value":   {"yes"},
		"Tags.member.2.Key":     {"to-keep"},
		"Tags.member.2.Value":   {"yes"},
	})

	doELBv2(t, h, url.Values{
		"Action":                {"RemoveTags"},
		"Version":               {"2015-12-01"},
		"ResourceArns.member.1": {lbArn},
		"TagKeys.member.1":      {"to-remove"},
	})

	rec := doELBv2(t, h, url.Values{
		"Action":                {"DescribeTags"},
		"Version":               {"2015-12-01"},
		"ResourceArns.member.1": {lbArn},
	})
	body := rec.Body.String()
	assert.NotContains(t, body, "to-remove")
	assert.Contains(t, body, "to-keep")
}

func TestTags_TG(t *testing.T) {
	t.Parallel()

	h := newBatch1Handler()
	tgArn := b1CreateTG(t, h, "tg-tag-test")

	doELBv2(t, h, url.Values{
		"Action":                {"AddTags"},
		"Version":               {"2015-12-01"},
		"ResourceArns.member.1": {tgArn},
		"Tags.member.1.Key":     {"service"},
		"Tags.member.1.Value":   {"api"},
	})

	rec := doELBv2(t, h, url.Values{
		"Action":                {"DescribeTags"},
		"Version":               {"2015-12-01"},
		"ResourceArns.member.1": {tgArn},
	})
	assert.Contains(t, rec.Body.String(), "service")
	assert.Contains(t, rec.Body.String(), "api")
}

func TestTags_Listener(t *testing.T) {
	t.Parallel()

	h := newBatch1Handler()
	lbArn := b1CreateLB(t, h, "listener-tag-lb")
	tgArn := b1CreateTG(t, h, "listener-tag-tg")
	lArn := b1CreateListener(t, h, lbArn, tgArn)

	doELBv2(t, h, url.Values{
		"Action":                {"AddTags"},
		"Version":               {"2015-12-01"},
		"ResourceArns.member.1": {lArn},
		"Tags.member.1.Key":     {"listener-tag"},
		"Tags.member.1.Value":   {"true"},
	})

	rec := doELBv2(t, h, url.Values{
		"Action":                {"DescribeTags"},
		"Version":               {"2015-12-01"},
		"ResourceArns.member.1": {lArn},
	})
	assert.Contains(t, rec.Body.String(), "listener-tag")
}

func TestAddTags_MissingResourceArns(t *testing.T) {
	t.Parallel()

	h := newBatch1Handler()
	rec := doELBv2(t, h, url.Values{
		"Action":            {"AddTags"},
		"Version":           {"2015-12-01"},
		"Tags.member.1.Key": {"k"},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestGetResourcePolicy(t *testing.T) {
	t.Parallel()

	h := newBatch1Handler()
	lbArn := b1CreateLB(t, h, "grp-lb")

	// No resource policy is set, so AWS returns ResourceNotFound (HTTP 400, AWS query-protocol status).
	rec := doELBv2(t, h, url.Values{
		"Action":      {"GetResourcePolicy"},
		"Version":     {"2015-12-01"},
		"ResourceArn": {lbArn},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// After a policy is stored on the backend, GetResourcePolicy returns it.
	be, ok := h.Backend.(*elbv2.InMemoryBackend)
	require.True(t, ok)
	require.NoError(t, be.PutResourcePolicy(lbArn, `{"Version":"2012-10-17"}`))

	rec2 := doELBv2(t, h, url.Values{
		"Action":      {"GetResourcePolicy"},
		"Version":     {"2015-12-01"},
		"ResourceArn": {lbArn},
	})
	require.Equal(t, http.StatusOK, rec2.Code)
	assert.Contains(t, rec2.Body.String(), "2012-10-17")
}

// TestDeleteLoadBalancer_ClearsResourcePolicy verifies DeleteLoadBalancer
// clears resourcePolicies for the deleted LB's ARN. GetResourcePolicy keys
// on ARN with no existence check against the load balancer itself, so a
// leaked entry is directly observable: querying the policy for a deleted
// LB's own ARN would otherwise still return it, and resourcePolicies is
// persisted verbatim in Snapshot(), so the leak also grows the snapshot
// without bound.
func TestDeleteLoadBalancer_ClearsResourcePolicy(t *testing.T) {
	t.Parallel()

	h := newBatch1Handler()
	lbArn := b1CreateLB(t, h, "policy-lb")
	otherArn := b1CreateLB(t, h, "policy-lb-sibling")

	be, ok := h.Backend.(*elbv2.InMemoryBackend)
	require.True(t, ok)
	require.NoError(t, be.PutResourcePolicy(lbArn, `{"Version":"2012-10-17"}`))
	require.NoError(t, be.PutResourcePolicy(otherArn, `{"Version":"2012-10-17"}`))

	rec := doELBv2(t, h, url.Values{
		"Action":          {"DeleteLoadBalancer"},
		"Version":         {"2015-12-01"},
		"LoadBalancerArn": {lbArn},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doELBv2(t, h, url.Values{
		"Action":      {"GetResourcePolicy"},
		"Version":     {"2015-12-01"},
		"ResourceArn": {lbArn},
	})
	assert.Equal(t, http.StatusBadRequest, rec2.Code)

	// Deleting one load balancer must not disturb another's resource policy.
	rec3 := doELBv2(t, h, url.Values{
		"Action":      {"GetResourcePolicy"},
		"Version":     {"2015-12-01"},
		"ResourceArn": {otherArn},
	})
	require.Equal(t, http.StatusOK, rec3.Code)
	assert.Contains(t, rec3.Body.String(), "2012-10-17")
}

func TestGetResourcePolicy_MissingArn(t *testing.T) {
	t.Parallel()

	h := newBatch1Handler()
	rec := doELBv2(t, h, url.Values{
		"Action":  {"GetResourcePolicy"},
		"Version": {"2015-12-01"},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestAddTags_KeyValueValidation verifies that AddTags enforces AWS tag limits:
// key 1-128 chars, value 0-256 chars, max 50 tags per resource.
func TestAddTags_KeyValueValidation(t *testing.T) {
	t.Parallel()

	h := newBatch2Handler()
	lbArn := mustCreateLB(t, h, "tag-val-lb")

	tests := []struct {
		name       string
		tagKey     string
		tagValue   string
		wantStatus int
	}{
		{
			name:       "valid_key_and_value",
			tagKey:     "env",
			tagValue:   "prod",
			wantStatus: http.StatusOK,
		},
		{
			name:       "key_exactly_128_chars_accepted",
			tagKey:     strings.Repeat("k", 128),
			tagValue:   "v",
			wantStatus: http.StatusOK,
		},
		{
			name:       "key_129_chars_rejected",
			tagKey:     strings.Repeat("k", 129),
			tagValue:   "v",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "value_empty_accepted",
			tagKey:     "mykey",
			tagValue:   "",
			wantStatus: http.StatusOK,
		},
		{
			name:       "value_exactly_256_chars_accepted",
			tagKey:     "mykey2",
			tagValue:   strings.Repeat("v", 256),
			wantStatus: http.StatusOK,
		},
		{
			name:       "value_257_chars_rejected",
			tagKey:     "mykey3",
			tagValue:   strings.Repeat("v", 257),
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := doELBv2(t, h, url.Values{
				"Action":                {"AddTags"},
				"Version":               {"2015-12-01"},
				"ResourceArns.member.1": {lbArn},
				"Tags.member.1.Key":     {tc.tagKey},
				"Tags.member.1.Value":   {tc.tagValue},
			})
			assert.Equal(t, tc.wantStatus, rec.Code, rec.Body.String())
		})
	}
}

// TestAddTags_MaxTagsPerResource verifies that adding tags beyond the 50-tag limit
// returns a ValidationError.
func TestAddTags_MaxTagsPerResource(t *testing.T) {
	t.Parallel()

	h := newBatch2Handler()
	lbArn := mustCreateLB(t, h, "max-tags-lb")

	// Add 50 tags one at a time.
	for i := range 50 {
		rec := doELBv2(t, h, url.Values{
			"Action":                {"AddTags"},
			"Version":               {"2015-12-01"},
			"ResourceArns.member.1": {lbArn},
			"Tags.member.1.Key":     {strings.Repeat("a", 1) + strings.Repeat("0", i%9) + string(rune('a'+i%26))},
			"Tags.member.1.Value":   {"v"},
		})
		require.Equal(t, http.StatusOK, rec.Code, "tag %d: %s", i, rec.Body.String())
	}

	// 51st distinct key should be rejected.
	rec := doELBv2(t, h, url.Values{
		"Action":                {"AddTags"},
		"Version":               {"2015-12-01"},
		"ResourceArns.member.1": {lbArn},
		"Tags.member.1.Key":     {"overflow-key-that-is-definitely-new"},
		"Tags.member.1.Value":   {"v"},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

// TestAddTags_UpdateExistingKeyDoesNotCount verifies that updating an existing tag
// key does not count toward the 50-tag limit.
func TestAddTags_UpdateExistingKeyDoesNotCount(t *testing.T) {
	t.Parallel()

	h := newBatch2Handler()
	lbArn := mustCreateLB(t, h, "tag-update-lb")

	// Add exactly 50 tags.
	vals := url.Values{
		"Action":                {"AddTags"},
		"Version":               {"2015-12-01"},
		"ResourceArns.member.1": {lbArn},
	}
	for i := range 50 {
		vals.Set("Tags.member."+itoa(i+1)+".Key", "key-"+itoa(i))
		vals.Set("Tags.member."+itoa(i+1)+".Value", "v")
	}
	rec := doELBv2(t, h, vals)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// Updating an existing key should succeed (not a new tag).
	rec2 := doELBv2(t, h, url.Values{
		"Action":                {"AddTags"},
		"Version":               {"2015-12-01"},
		"ResourceArns.member.1": {lbArn},
		"Tags.member.1.Key":     {"key-0"},
		"Tags.member.1.Value":   {"updated"},
	})
	assert.Equal(t, http.StatusOK, rec2.Code, rec2.Body.String())
}

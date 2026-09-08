package elbv2_test

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/elbv2"
)

// TestCreateTargetGroup tests target group creation.
func TestCreateTargetGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(t *testing.T, h *elbv2.Handler)
		vals       url.Values
		name       string
		wantStatus int
		wantArn    bool
	}{
		{
			name: "creates_successfully",
			vals: url.Values{
				"Action":   {"CreateTargetGroup"},
				"Version":  {"2015-12-01"},
				"Name":     {"my-tg"},
				"Protocol": {"HTTP"},
				"Port":     {"80"},
				"VpcId":    {"vpc-12345"},
			},
			wantStatus: http.StatusOK,
			wantArn:    true,
		},
		{
			name: "duplicate_returns_conflict",
			setup: func(t *testing.T, h *elbv2.Handler) {
				t.Helper()
				mustCreateTG(t, h, "dup-tg")
			},
			vals: url.Values{
				"Action":   {"CreateTargetGroup"},
				"Version":  {"2015-12-01"},
				"Name":     {"dup-tg"},
				"Protocol": {"HTTP"},
				"Port":     {"80"},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "missing_name_returns_bad_request",
			vals: url.Values{
				"Action":  {"CreateTargetGroup"},
				"Version": {"2015-12-01"},
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

			rec := doELBv2(t, h, tt.vals)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantArn {
				var resp struct {
					XMLName xml.Name `xml:"CreateTargetGroupResponse"`
					Result  struct {
						TargetGroups struct {
							Members []struct {
								TargetGroupArn string `xml:"TargetGroupArn"`
							} `xml:"member"`
						} `xml:"TargetGroups"`
					} `xml:"CreateTargetGroupResult"`
				}
				parseXMLBody(t, rec, &resp)
				require.Len(t, resp.Result.TargetGroups.Members, 1)
				assert.NotEmpty(t, resp.Result.TargetGroups.Members[0].TargetGroupArn)
			}
		})
	}
}

// TestDescribeTargetGroups tests describe target groups operations.
func TestDescribeTargetGroups(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(t *testing.T, h *elbv2.Handler)
		vals       url.Values
		name       string
		wantCount  int
		wantStatus int
	}{
		{
			name: "describe_all",
			setup: func(t *testing.T, h *elbv2.Handler) {
				t.Helper()
				mustCreateTG(t, h, "tg-a")
				mustCreateTG(t, h, "tg-b")
			},
			vals: url.Values{
				"Action":  {"DescribeTargetGroups"},
				"Version": {"2015-12-01"},
			},
			wantStatus: http.StatusOK,
			wantCount:  2,
		},
		{
			name: "filter_by_name",
			setup: func(t *testing.T, h *elbv2.Handler) {
				t.Helper()
				mustCreateTG(t, h, "filter-tg")
				mustCreateTG(t, h, "other-tg")
			},
			vals: url.Values{
				"Action":         {"DescribeTargetGroups"},
				"Version":        {"2015-12-01"},
				"Names.member.1": {"filter-tg"},
			},
			wantStatus: http.StatusOK,
			wantCount:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			if tt.setup != nil {
				tt.setup(t, h)
			}

			rec := doELBv2(t, h, tt.vals)
			require.Equal(t, tt.wantStatus, rec.Code)

			var resp struct {
				Result struct {
					TargetGroups struct {
						Members []struct {
							TargetGroupArn string `xml:"TargetGroupArn"`
						} `xml:"member"`
					} `xml:"TargetGroups"`
				} `xml:"DescribeTargetGroupsResult"`
			}
			parseXMLBody(t, rec, &resp)
			assert.Len(t, resp.Result.TargetGroups.Members, tt.wantCount)
		})
	}
}

// TestDeleteTargetGroup tests target group deletion.
func TestDeleteTargetGroup(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	tgArn := mustCreateTG(t, h, "delete-tg")

	rec := doELBv2(t, h, url.Values{
		"Action":         {"DeleteTargetGroup"},
		"Version":        {"2015-12-01"},
		"TargetGroupArn": {tgArn},
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	// Verify deletion — AWS returns TargetGroupNotFound for a deleted ARN.
	rec2 := doELBv2(t, h, url.Values{
		"Action":                   {"DescribeTargetGroups"},
		"Version":                  {"2015-12-01"},
		"TargetGroupArns.member.1": {tgArn},
	})
	require.Equal(t, http.StatusBadRequest, rec2.Code)
}

// TestDeleteTargetGroup_LifecycleMapsCleaned verifies that deleting a target
// group with targets still mid-initial-health-check or mid-drain releases
// their entries from the backend's targetReadyAt/targetDrainingUntil maps,
// rather than leaking them under the now-deleted target group's ARN forever.
func TestDeleteTargetGroup_LifecycleMapsCleaned(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup func(t *testing.T, h *elbv2.Handler, tgArn string)
		name  string
		field string
	}{
		{
			name:  "target still in initial health check",
			field: "targetReadyAt",
			setup: func(t *testing.T, h *elbv2.Handler, tgArn string) {
				t.Helper()

				rec := doELBv2(t, h, url.Values{
					"Action":              {"RegisterTargets"},
					"Version":             {"2015-12-01"},
					"TargetGroupArn":      {tgArn},
					"Targets.member.1.Id": {"10.0.0.1"},
				})
				require.Equal(t, http.StatusOK, rec.Code)
			},
		},
		{
			name:  "target still draining",
			field: "targetDrainingUntil",
			setup: func(t *testing.T, h *elbv2.Handler, tgArn string) {
				t.Helper()

				vals := url.Values{
					"Version":             {"2015-12-01"},
					"TargetGroupArn":      {tgArn},
					"Targets.member.1.Id": {"10.0.0.1"},
				}

				vals.Set("Action", "RegisterTargets")
				require.Equal(t, http.StatusOK, doELBv2(t, h, vals).Code)

				vals.Set("Action", "DeregisterTargets")
				require.Equal(t, http.StatusOK, doELBv2(t, h, vals).Code)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			tgArn := mustCreateTG(t, h, "leak-tg-"+tc.field)

			tc.setup(t, h, tgArn)

			rec := doELBv2(t, h, url.Values{
				"Action":         {"DeleteTargetGroup"},
				"Version":        {"2015-12-01"},
				"TargetGroupArn": {tgArn},
			})
			require.Equal(t, http.StatusOK, rec.Code)

			var snap struct {
				TargetReadyAt       map[string]map[string]time.Time `json:"targetReadyAt"`
				TargetDrainingUntil map[string]map[string]time.Time `json:"targetDrainingUntil"`
			}
			require.NoError(t, json.Unmarshal(h.Snapshot(context.Background()), &snap))

			byField := map[string]map[string]map[string]time.Time{
				"targetReadyAt":       snap.TargetReadyAt,
				"targetDrainingUntil": snap.TargetDrainingUntil,
			}

			assert.Empty(t, byField[tc.field][tgArn],
				"deleted target group's ARN must not linger in the backend's %s lifecycle map", tc.field)
		})
	}
}

// TestModifyTargetGroup tests target group modification.
func TestModifyTargetGroup(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	tgArn := mustCreateTG(t, h, "mod-tg")

	rec := doELBv2(t, h, url.Values{
		"Action":         {"ModifyTargetGroup"},
		"Version":        {"2015-12-01"},
		"TargetGroupArn": {tgArn},
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	// Missing arn
	rec2 := doELBv2(t, h, url.Values{
		"Action":  {"ModifyTargetGroup"},
		"Version": {"2015-12-01"},
	})
	assert.Equal(t, http.StatusBadRequest, rec2.Code)
}

// TestDescribeTargetGroupAttributes tests target group attribute retrieval.
func TestDescribeTargetGroupAttributes(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	tgArn := mustCreateTG(t, h, "desc-attrs-tg")

	tests := []struct {
		vals       url.Values
		name       string
		wantStatus int
	}{
		{
			name: "success",
			vals: url.Values{
				"Action":         {"DescribeTargetGroupAttributes"},
				"Version":        {"2015-12-01"},
				"TargetGroupArn": {tgArn},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "missing_arn",
			vals: url.Values{
				"Action":  {"DescribeTargetGroupAttributes"},
				"Version": {"2015-12-01"},
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := doELBv2(t, h, tt.vals)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestModifyTargetGroupPersistsFields verifies ModifyTargetGroup actually updates the TG.
func TestModifyTargetGroupPersistsFields(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	tgArn := mustCreateTG(t, h, "modify-tg-persist")

	rec := doELBv2(t, h, url.Values{
		"Action":                     {"ModifyTargetGroup"},
		"Version":                    {"2015-12-01"},
		"TargetGroupArn":             {tgArn},
		"HealthCheckProtocol":        {"HTTPS"},
		"HealthCheckPort":            {"8443"},
		"HealthCheckPath":            {"/health"},
		"HealthCheckEnabled":         {"true"},
		"Matcher.HTTPCode":           {"200-204"},
		"HealthCheckIntervalSeconds": {"30"},
		"HealthyThresholdCount":      {"3"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Result struct {
			TargetGroups struct {
				Members []struct {
					Matcher struct {
						HTTPCode string `xml:"HTTPCode"`
					} `xml:"Matcher"`
					HealthCheckProtocol        string `xml:"HealthCheckProtocol"`
					HealthCheckPort            string `xml:"HealthCheckPort"`
					HealthCheckPath            string `xml:"HealthCheckPath"`
					HealthCheckIntervalSeconds int32  `xml:"HealthCheckIntervalSeconds"`
					HealthyThresholdCount      int32  `xml:"HealthyThresholdCount"`
					HealthCheckEnabled         bool   `xml:"HealthCheckEnabled"`
				} `xml:"member"`
			} `xml:"TargetGroups"`
		} `xml:"ModifyTargetGroupResult"`
	}
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Result.TargetGroups.Members, 1)
	tg := resp.Result.TargetGroups.Members[0]
	assert.Equal(t, "HTTPS", tg.HealthCheckProtocol)
	assert.Equal(t, "8443", tg.HealthCheckPort)
	assert.Equal(t, "/health", tg.HealthCheckPath)
	assert.True(t, tg.HealthCheckEnabled)
	assert.Equal(t, "200-204", tg.Matcher.HTTPCode)
	assert.Equal(t, int32(30), tg.HealthCheckIntervalSeconds)
	assert.Equal(t, int32(3), tg.HealthyThresholdCount)
}

// TestDescribeTargetGroupsWithLBFilter verifies lbArn filter works.
func TestDescribeTargetGroupsWithLBFilter(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	lbArn := mustCreateLB(t, h, "lb-filter-tgs-lb")
	tg1Arn := mustCreateTG(t, h, "lb-filter-tg1")
	tg2Arn := mustCreateTG(t, h, "lb-filter-tg2")
	_ = mustCreateTG(t, h, "lb-filter-tg3") // Not attached to LB.

	mustCreateListener(t, h, lbArn, tg1Arn)

	// Create a second listener on a different port with tg2.
	rec80 := doELBv2(t, h, url.Values{
		"Action":                                 {"CreateListener"},
		"Version":                                {"2015-12-01"},
		"LoadBalancerArn":                        {lbArn},
		"Protocol":                               {"HTTP"},
		"Port":                                   {"8080"},
		"DefaultActions.member.1.Type":           {"forward"},
		"DefaultActions.member.1.TargetGroupArn": {tg2Arn},
	})
	require.Equal(t, http.StatusOK, rec80.Code)

	// Filter by lbArn should return only tg1 and tg2 (attached to listeners).
	rec := doELBv2(t, h, url.Values{
		"Action":          {"DescribeTargetGroups"},
		"Version":         {"2015-12-01"},
		"LoadBalancerArn": {lbArn},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Result struct {
			TargetGroups struct {
				Members []struct {
					TargetGroupArn string `xml:"TargetGroupArn"`
				} `xml:"member"`
			} `xml:"TargetGroups"`
		} `xml:"DescribeTargetGroupsResult"`
	}
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))

	got := make(map[string]bool)
	for _, tg := range resp.Result.TargetGroups.Members {
		got[tg.TargetGroupArn] = true
	}
	assert.True(t, got[tg1Arn], "tg1 should be in result")
	assert.True(t, got[tg2Arn], "tg2 should be in result")
	assert.Len(t, resp.Result.TargetGroups.Members, 2, "only LB-attached TGs should be returned")
}

// TestTargetGroupLoadBalancerArns tests that TG LoadBalancerArns is populated.
func TestTargetGroupLoadBalancerArns(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	lbArn := mustCreateLB(t, h, "tg-lb-arns-lb")
	tgArn := mustCreateTG(t, h, "tg-lb-arns-tg")

	// Before attaching to LB, LoadBalancerArns should be empty.
	rec := doELBv2(t, h, url.Values{
		"Action":                   {"DescribeTargetGroups"},
		"Version":                  {"2015-12-01"},
		"TargetGroupArns.member.1": {tgArn},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var beforeResp struct {
		Result struct {
			TargetGroups struct {
				Members []struct {
					LoadBalancerArns struct {
						Members []struct {
							Value string `xml:",chardata"`
						} `xml:"member"`
					} `xml:"LoadBalancerArns"`
				} `xml:"member"`
			} `xml:"TargetGroups"`
		} `xml:"DescribeTargetGroupsResult"`
	}
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &beforeResp))
	require.Len(t, beforeResp.Result.TargetGroups.Members, 1)
	assert.Empty(t, beforeResp.Result.TargetGroups.Members[0].LoadBalancerArns.Members)

	// Attach to LB via listener.
	_ = mustCreateListener(t, h, lbArn, tgArn)

	// After attaching, LoadBalancerArns should contain the LB ARN.
	rec2 := doELBv2(t, h, url.Values{
		"Action":                   {"DescribeTargetGroups"},
		"Version":                  {"2015-12-01"},
		"TargetGroupArns.member.1": {tgArn},
	})
	require.Equal(t, http.StatusOK, rec2.Code)
	// We verify via DescribeTargetGroups by LoadBalancerArn filter.
	filtRec := doELBv2(t, h, url.Values{
		"Action":          {"DescribeTargetGroups"},
		"Version":         {"2015-12-01"},
		"LoadBalancerArn": {lbArn},
	})
	require.Equal(t, http.StatusOK, filtRec.Code)

	var filtResp struct {
		Result struct {
			TargetGroups struct {
				Members []struct {
					TargetGroupArn string `xml:"TargetGroupArn"`
				} `xml:"member"`
			} `xml:"TargetGroups"`
		} `xml:"DescribeTargetGroupsResult"`
	}
	require.NoError(t, xml.Unmarshal(filtRec.Body.Bytes(), &filtResp))
	require.Len(t, filtResp.Result.TargetGroups.Members, 1)
	assert.Equal(t, tgArn, filtResp.Result.TargetGroups.Members[0].TargetGroupArn)
}

// TestTargetGroupLoadBalancerArnsAfterListener verifies that TG shows LB ARNs after listener creation.
func TestTargetGroupLoadBalancerArnsAfterListener(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	lbArn := mustCreateLB(t, h, "tg-lb-arns-lb")
	tgArn := mustCreateTG(t, h, "tg-lb-arns-tg")
	_ = mustCreateListener(t, h, lbArn, tgArn)

	rec := doELBv2(t, h, url.Values{
		"Action":                   {"DescribeTargetGroups"},
		"Version":                  {"2015-12-01"},
		"TargetGroupArns.member.1": {tgArn},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Result struct {
			TargetGroups struct {
				Members []struct {
					LoadBalancerArns struct {
						Members []struct {
							Value string `xml:",chardata"`
						} `xml:"member"`
					} `xml:"LoadBalancerArns"`
				} `xml:"member"`
			} `xml:"TargetGroups"`
		} `xml:"DescribeTargetGroupsResult"`
	}
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Result.TargetGroups.Members, 1)
	require.Len(t, resp.Result.TargetGroups.Members[0].LoadBalancerArns.Members, 1)
	assert.Equal(t, lbArn, resp.Result.TargetGroups.Members[0].LoadBalancerArns.Members[0].Value)
}

// TestDescribeTargetGroupsPagination verifies Marker/PageSize pagination for DescribeTargetGroups.
func TestDescribeTargetGroupsPagination(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	for _, name := range []string{"tg-a", "tg-b", "tg-c"} {
		mustCreateTG(t, h, name)
	}

	// Page 1: PageSize=2
	rec1 := doELBv2(t, h, url.Values{
		"Action":   {"DescribeTargetGroups"},
		"Version":  {"2015-12-01"},
		"PageSize": {"2"},
	})
	require.Equal(t, http.StatusOK, rec1.Code)

	var page1 struct {
		Result struct {
			NextMarker   string `xml:"NextMarker"`
			TargetGroups struct {
				Members []struct {
					TargetGroupName string `xml:"TargetGroupName"`
					TargetGroupArn  string `xml:"TargetGroupArn"`
				} `xml:"member"`
			} `xml:"TargetGroups"`
		} `xml:"DescribeTargetGroupsResult"`
	}
	require.NoError(t, xml.Unmarshal(rec1.Body.Bytes(), &page1))
	require.Len(t, page1.Result.TargetGroups.Members, 2)
	assert.Equal(t, "tg-a", page1.Result.TargetGroups.Members[0].TargetGroupName)
	assert.NotEmpty(t, page1.Result.NextMarker)

	// Page 2
	rec2 := doELBv2(t, h, url.Values{
		"Action":   {"DescribeTargetGroups"},
		"Version":  {"2015-12-01"},
		"PageSize": {"2"},
		"Marker":   {page1.Result.NextMarker},
	})
	require.Equal(t, http.StatusOK, rec2.Code)

	var page2 struct {
		Result struct {
			NextMarker   string `xml:"NextMarker"`
			TargetGroups struct {
				Members []struct {
					TargetGroupName string `xml:"TargetGroupName"`
				} `xml:"member"`
			} `xml:"TargetGroups"`
		} `xml:"DescribeTargetGroupsResult"`
	}
	require.NoError(t, xml.Unmarshal(rec2.Body.Bytes(), &page2))
	require.Len(t, page2.Result.TargetGroups.Members, 1)
	assert.Equal(t, "tg-c", page2.Result.TargetGroups.Members[0].TargetGroupName)
	assert.Empty(t, page2.Result.NextMarker)
}

func TestCreateTG_InstanceType(t *testing.T) {
	t.Parallel()

	h := newBatch1Handler()
	rec := doELBv2(t, h, url.Values{
		"Action":     {"CreateTargetGroup"},
		"Version":    {"2015-12-01"},
		"Name":       {"tg-instance"},
		"Protocol":   {"HTTP"},
		"Port":       {"8080"},
		"VpcId":      {"vpc-11111111"},
		"TargetType": {"instance"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Result struct {
			TargetGroups struct {
				Members []struct {
					TargetType string `xml:"TargetType"`
					VpcID      string `xml:"VpcId"`
					Port       int32  `xml:"Port"`
				} `xml:"member"`
			} `xml:"TargetGroups"`
		} `xml:"CreateTargetGroupResult"`
	}
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
	tg := resp.Result.TargetGroups.Members[0]
	assert.Equal(t, "instance", tg.TargetType)
	assert.Equal(t, "vpc-11111111", tg.VpcID)
	assert.Equal(t, int32(8080), tg.Port)
}

func TestCreateTG_IPType(t *testing.T) {
	t.Parallel()

	h := newBatch1Handler()
	rec := doELBv2(t, h, url.Values{
		"Action":     {"CreateTargetGroup"},
		"Version":    {"2015-12-01"},
		"Name":       {"tg-ip"},
		"Protocol":   {"HTTP"},
		"Port":       {"443"},
		"VpcId":      {"vpc-00000000"},
		"TargetType": {"ip"},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "ip")
}

func TestCreateTG_LambdaType(t *testing.T) {
	t.Parallel()

	h := newBatch1Handler()
	rec := doELBv2(t, h, url.Values{
		"Action":     {"CreateTargetGroup"},
		"Version":    {"2015-12-01"},
		"Name":       {"tg-lambda"},
		"TargetType": {"lambda"},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "lambda")
}

func TestCreateTG_ALBType(t *testing.T) {
	t.Parallel()

	h := newBatch1Handler()
	rec := doELBv2(t, h, url.Values{
		"Action":     {"CreateTargetGroup"},
		"Version":    {"2015-12-01"},
		"Name":       {"tg-alb"},
		"Protocol":   {"HTTP"},
		"Port":       {"80"},
		"VpcId":      {"vpc-00000000"},
		"TargetType": {"alb"},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "alb")
}

func TestDescribeTGs_ByArn(t *testing.T) {
	t.Parallel()

	h := newBatch1Handler()
	tgArn := b1CreateTG(t, h, "describe-tg-arn")
	b1CreateTG(t, h, "other-tg")

	rec := doELBv2(t, h, url.Values{
		"Action":                   {"DescribeTargetGroups"},
		"Version":                  {"2015-12-01"},
		"TargetGroupArns.member.1": {tgArn},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Result struct {
			TargetGroups struct {
				Members []struct {
					TargetGroupArn string `xml:"TargetGroupArn"`
				} `xml:"member"`
			} `xml:"TargetGroups"`
		} `xml:"DescribeTargetGroupsResult"`
	}
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Result.TargetGroups.Members, 1)
	assert.Equal(t, tgArn, resp.Result.TargetGroups.Members[0].TargetGroupArn)
}

func TestDeleteTG_Success(t *testing.T) {
	t.Parallel()

	h := newBatch1Handler()
	tgArn := b1CreateTG(t, h, "del-tg-batch1")

	rec := doELBv2(t, h, url.Values{
		"Action":         {"DeleteTargetGroup"},
		"Version":        {"2015-12-01"},
		"TargetGroupArn": {tgArn},
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestDescribeTGs_Pagination(t *testing.T) {
	t.Parallel()

	h := newBatch1Handler()
	for i := range 4 {
		b1CreateTG(t, h, "pag-tg-"+string(rune('a'+i)))
	}

	rec := doELBv2(t, h, url.Values{
		"Action":   {"DescribeTargetGroups"},
		"Version":  {"2015-12-01"},
		"PageSize": {"2"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Result struct {
			NextMarker   string `xml:"NextMarker"`
			TargetGroups struct {
				Members []struct{} `xml:"member"`
			} `xml:"TargetGroups"`
		} `xml:"DescribeTargetGroupsResult"`
	}
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Len(t, resp.Result.TargetGroups.Members, 2)
	assert.NotEmpty(t, resp.Result.NextMarker)
}

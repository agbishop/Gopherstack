package elasticsearch_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestElasticsearchHandler_ReservedInstances_Lifecycle drives
// DescribeReservedElasticsearchInstanceOfferings,
// PurchaseReservedElasticsearchInstanceOffering, and
// DescribeReservedElasticsearchInstances through the HTTP handler.
func TestElasticsearchHandler_ReservedInstances_Lifecycle(t *testing.T) {
	t.Parallel()

	h := newTestHandler()

	resp := doRequest(t, h, http.MethodGet, "/2015-01-01/es/reservedInstanceOfferings", nil)
	require.Len(t, readJSONBody(t, resp)["ReservedElasticsearchInstanceOfferings"], 1)

	resp = doRequest(t, h, http.MethodPost, "/2015-01-01/es/purchaseReservedInstanceOffering", map[string]any{
		"ReservedElasticsearchInstanceOfferingId": "offer-t3-small-1y",
		"ReservationName":                         "reserved-state",
	})
	require.NotEmpty(t, readJSONBody(t, resp)["ReservedElasticsearchInstanceId"])

	resp = doRequest(t, h, http.MethodGet, "/2015-01-01/es/reservedInstances", nil)
	require.Len(t, readJSONBody(t, resp)["ReservedElasticsearchInstances"], 1)
}

// TestElasticsearchHandler_PurchaseReservedInstanceOffering_UnknownOffering
// verifies that purchasing an offering ID that does not match any real
// offering is rejected with ResourceNotFoundException (404), matching real
// AWS: PurchaseReservedElasticsearchInstanceOffering's deserializer
// (elasticsearchservice@v1.45.4 deserializers.go,
// awsRestjson1_deserializeOpErrorPurchaseReservedElasticsearchInstanceOffering)
// declares ResourceNotFoundException among its modelled errors. Before the
// fix, the backend never validated the offering ID against the known
// offering list -- it silently created a reservation with zero-value
// InstanceType/FixedPrice/UsagePrice/Duration fields and returned 200 OK.
func TestElasticsearchHandler_PurchaseReservedInstanceOffering_UnknownOffering(t *testing.T) {
	t.Parallel()

	h := newTestHandler()

	resp := doRequest(t, h, http.MethodPost, "/2015-01-01/es/purchaseReservedInstanceOffering", map[string]any{
		"ReservedElasticsearchInstanceOfferingId": "offer-does-not-exist",
		"ReservationName":                         "bogus",
	})
	defer resp.Body.Close()

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

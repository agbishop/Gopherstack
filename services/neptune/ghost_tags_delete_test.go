package neptune_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/neptune"
)

// TestDelete_NoGhostTags proves Delete for DBParameterGroup, DBClusterEndpoint,
// EventSubscription, and GlobalCluster clears the tag store entry for the
// deleted resource's ARN -- these ARNs are deterministic (name-derived), so a
// resource recreated under the same identifier would otherwise silently
// inherit the deleted resource's tags. Mirrors the existing cascade-clean
// convention already covered for DBCluster/DBClusterParameterGroup/
// DBSubnetGroup/DBClusterSnapshot elsewhere in this package.
func TestDelete_NoGhostTags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		create func(t *testing.T, h *neptune.Handler)
		delete func(t *testing.T, h *neptune.Handler)
		arn    string
		name   string
	}{
		{
			name: "db_parameter_group",
			arn:  "arn:aws:rds:us-east-1:000000000000:pg:ghost-pg",
			create: func(t *testing.T, h *neptune.Handler) {
				t.Helper()
				rr := doRequest(t, h, url.Values{
					"Action":                 {"CreateDBParameterGroup"},
					"Version":                {"2014-10-31"},
					"DBParameterGroupName":   {"ghost-pg"},
					"DBParameterGroupFamily": {"neptune1.3"},
				})
				require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
			},
			delete: func(t *testing.T, h *neptune.Handler) {
				t.Helper()
				rr := doRequest(t, h, url.Values{
					"Action":               {"DeleteDBParameterGroup"},
					"Version":              {"2014-10-31"},
					"DBParameterGroupName": {"ghost-pg"},
				})
				require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
			},
		},
		{
			name: "db_cluster_endpoint",
			arn:  "arn:aws:rds:us-east-1:000000000000:cluster-endpoint:ghost-ep",
			create: func(t *testing.T, h *neptune.Handler) {
				t.Helper()
				createCluster(t, h, "ghost-ep-cluster")
				rr := doRequest(t, h, url.Values{
					"Action":                      {"CreateDBClusterEndpoint"},
					"Version":                     {"2014-10-31"},
					"DBClusterEndpointIdentifier": {"ghost-ep"},
					"DBClusterIdentifier":         {"ghost-ep-cluster"},
					"EndpointType":                {"READER"},
				})
				require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
			},
			delete: func(t *testing.T, h *neptune.Handler) {
				t.Helper()
				rr := doRequest(t, h, url.Values{
					"Action":                      {"DeleteDBClusterEndpoint"},
					"Version":                     {"2014-10-31"},
					"DBClusterEndpointIdentifier": {"ghost-ep"},
				})
				require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
			},
		},
		{
			name: "event_subscription",
			arn:  "arn:aws:rds:us-east-1:000000000000:es:ghost-sub",
			create: func(t *testing.T, h *neptune.Handler) {
				t.Helper()
				rr := doRequest(t, h, url.Values{
					"Action":           {"CreateEventSubscription"},
					"Version":          {"2014-10-31"},
					"SubscriptionName": {"ghost-sub"},
					"SnsTopicArn":      {"arn:aws:sns:us-east-1:000000000000:ghost-topic"},
				})
				require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
			},
			delete: func(t *testing.T, h *neptune.Handler) {
				t.Helper()
				rr := doRequest(t, h, url.Values{
					"Action":           {"DeleteEventSubscription"},
					"Version":          {"2014-10-31"},
					"SubscriptionName": {"ghost-sub"},
				})
				require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
			},
		},
		{
			name: "global_cluster",
			arn:  "arn:aws:rds::000000000000:global-cluster:ghost-gc",
			create: func(t *testing.T, h *neptune.Handler) {
				t.Helper()
				rr := doRequest(t, h, url.Values{
					"Action":                  {"CreateGlobalCluster"},
					"Version":                 {"2014-10-31"},
					"GlobalClusterIdentifier": {"ghost-gc"},
				})
				require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
			},
			delete: func(t *testing.T, h *neptune.Handler) {
				t.Helper()
				rr := doRequest(t, h, url.Values{
					"Action":                  {"DeleteGlobalCluster"},
					"Version":                 {"2014-10-31"},
					"GlobalClusterIdentifier": {"ghost-gc"},
				})
				require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			tt.create(t, h)

			rr := doRequest(t, h, url.Values{
				"Action":           {"AddTagsToResource"},
				"Version":          {"2014-10-31"},
				"ResourceName":     {tt.arn},
				"Tags.Tag.1.Key":   {"Env"},
				"Tags.Tag.1.Value": {"ghost"},
			})
			require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

			tt.delete(t, h)
			tt.create(t, h)

			rr = doRequest(t, h, url.Values{
				"Action":       {"ListTagsForResource"},
				"Version":      {"2014-10-31"},
				"ResourceName": {tt.arn},
			})
			require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
			assert.NotContains(t, rr.Body.String(), "ghost",
				"recreated resource inherited tags from the deleted resource of the same name")
		})
	}
}

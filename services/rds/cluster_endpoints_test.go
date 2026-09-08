package rds_test

import (
	"encoding/xml"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/rds"
)

// TestDeleteDBCluster_CascadeDeletesClusterEndpoints is a regression test for
// a ghost-row leak: DeleteDBClusterWithOptions must remove every custom
// cluster endpoint belonging to the deleted cluster, not just the cluster
// itself. Before the fix, DescribeDBClusterEndpoints kept returning
// endpoints pointing at a deleted cluster forever (the clusterEndpoints map
// only shrank when a client explicitly called DeleteDBClusterEndpoint).
func TestDeleteDBCluster_CascadeDeletesClusterEndpoints(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)
	_, err := b.CreateDBCluster(
		"leak-cluster", "aurora-mysql", "admin", "", "", 0, nil, rds.DBClusterOptions{},
	)
	require.NoError(t, err)

	_, err = b.CreateDBClusterEndpoint("leak-endpoint-1", "leak-cluster", "READER")
	require.NoError(t, err)
	_, err = b.CreateDBClusterEndpoint("leak-endpoint-2", "leak-cluster", "WRITER")
	require.NoError(t, err)

	// Sanity check: both endpoints exist before delete.
	before, err := b.DescribeDBClusterEndpoints("leak-cluster", "")
	require.NoError(t, err)
	assert.Len(t, before, 2)

	_, err = b.DeleteDBClusterWithOptions("leak-cluster", true, "")
	require.NoError(t, err)

	// The endpoints must be gone, not just the cluster. DBClusterEndpointIdentifier
	// is a filter (see DescribeDBClusterEndpoints), so a gone endpoint yields an
	// empty result, not a not-found error.
	got1, err := b.DescribeDBClusterEndpoints("", "leak-endpoint-1")
	require.NoError(t, err)
	assert.Empty(t, got1)
	got2, err := b.DescribeDBClusterEndpoints("", "leak-endpoint-2")
	require.NoError(t, err)
	assert.Empty(t, got2)

	// leak-cluster itself is gone too, so DBClusterIdentifier (unlike the
	// endpoint-identifier filter above) now faults instead of paging zero
	// endpoints (gopherstack-l20u).
	_, err = b.DescribeDBClusterEndpoints("leak-cluster", "")
	require.ErrorIs(t, err, rds.ErrClusterNotFound)
}

// TestDescribeDBClusterEndpoints_IdentifierVsFilter pins the two opposite
// treatments the op owes its two identifier-shaped params, since 33jc's fix
// (DBClusterEndpointIdentifier is a filter) and l20u's fix (DBClusterIdentifier
// is the op's one declared error, DBClusterNotFoundFault) sit in the same
// function and collapse into each other if either regresses.
func TestDescribeDBClusterEndpoints_IdentifierVsFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		query           string
		wantXMLCode     string
		wantContains    []string
		wantNotContains []string
		wantCode        int
	}{
		{
			name: "unknown cluster identifier faults",
			query: "Action=DescribeDBClusterEndpoints&Version=2014-10-31" +
				"&DBClusterIdentifier=missing-cluster",
			wantCode:    http.StatusBadRequest,
			wantXMLCode: "DBClusterNotFoundFault",
		},
		{
			name: "unknown endpoint identifier is an empty list",
			query: "Action=DescribeDBClusterEndpoints&Version=2014-10-31" +
				"&DBClusterEndpointIdentifier=missing-endpoint",
			wantCode:        http.StatusOK,
			wantNotContains: []string{"DBClusterEndpointNotFound", "DBClusterNotFound"},
		},
		{
			name: "valid cluster with valid endpoint filter returns the endpoint",
			query: "Action=DescribeDBClusterEndpoints&Version=2014-10-31" +
				"&DBClusterIdentifier=idvf-cluster&DBClusterEndpointIdentifier=idvf-ep",
			wantCode:     http.StatusOK,
			wantContains: []string{"idvf-ep"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newRDSHandler()
			postRDSForm(t, h, "Action=CreateDBCluster&Version=2014-10-31"+
				"&DBClusterIdentifier=idvf-cluster&Engine=aurora-postgresql")
			postRDSForm(t, h, "Action=CreateDBClusterEndpoint&Version=2014-10-31"+
				"&DBClusterEndpointIdentifier=idvf-ep&DBClusterIdentifier=idvf-cluster&EndpointType=READER")

			rec := postRDSForm(t, h, tt.query)
			require.Equal(t, tt.wantCode, rec.Code, "body: %s", rec.Body.String())

			if tt.wantXMLCode != "" {
				var resp struct {
					XMLName xml.Name `xml:"ErrorResponse"`
					Error   struct {
						Code string `xml:"Code"`
					} `xml:"Error"`
				}
				require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Equal(t, tt.wantXMLCode, resp.Error.Code)
			}
			body := rec.Body.String()
			for _, s := range tt.wantContains {
				assert.Contains(t, body, s)
			}
			for _, s := range tt.wantNotContains {
				assert.NotContains(t, body, s)
			}
		})
	}
}

func TestModifyDBClusterEndpoint(t *testing.T) {
	t.Parallel()
	tests := []struct {
		wantErrIs    error
		name         string
		endpointID   string
		endpointType string
		wantErr      bool
	}{
		{
			name:         "success updates type",
			endpointID:   "my-endpoint",
			endpointType: "READER",
		},
		{
			name:       "not found",
			endpointID: "missing",
			wantErr:    true,
			wantErrIs:  rds.ErrClusterEndpointNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newTestBackend(t)
			if !tt.wantErr {
				_, err := b.CreateDBCluster(
					"my-cluster",
					"aurora-mysql",
					"admin",
					"",
					"",
					0,
					nil,
					rds.DBClusterOptions{},
				)
				require.NoError(t, err)
				_, err = b.CreateDBClusterEndpoint(tt.endpointID, "my-cluster", "WRITER")
				require.NoError(t, err)
			}
			got, err := b.ModifyDBClusterEndpoint(tt.endpointID, tt.endpointType)
			if tt.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.wantErrIs)

				return
			}
			require.NoError(t, err)
			if tt.endpointType != "" {
				assert.Equal(t, tt.endpointType, got.EndpointType)
			}
		})
	}
}

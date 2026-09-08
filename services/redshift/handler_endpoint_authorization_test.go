package redshift_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/redshift"
)

// ---- AuthorizeEndpointAccess ----

func TestHandler_AuthorizeEndpointAccess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(t *testing.T, h *redshift.Handler, b *redshift.InMemoryBackend)
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "success_all_vpcs",
			setup: func(t *testing.T, h *redshift.Handler, _ *redshift.InMemoryBackend) {
				t.Helper()
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=ep-cluster")
			},
			body: "Action=AuthorizeEndpointAccess&Version=2012-12-01" +
				"&ClusterIdentifier=ep-cluster&Account=111111111111",
			wantCode:     http.StatusOK,
			wantContains: []string{"AuthorizeEndpointAccessResponse", "111111111111"},
		},
		{
			name: "success_specific_vpcs",
			setup: func(t *testing.T, h *redshift.Handler, _ *redshift.InMemoryBackend) {
				t.Helper()
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=ep-cluster-vpc")
			},
			body: "Action=AuthorizeEndpointAccess&Version=2012-12-01" +
				"&ClusterIdentifier=ep-cluster-vpc&Account=222222222222" +
				"&VpcIds.VpcIdentifier.1=vpc-12345&VpcIds.VpcIdentifier.2=vpc-67890",
			wantCode:     http.StatusOK,
			wantContains: []string{"AuthorizeEndpointAccessResponse", "222222222222"},
		},
		{
			name:         "missing_cluster_identifier",
			body:         "Action=AuthorizeEndpointAccess&Version=2012-12-01&Account=111111111111",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		{
			name:         "missing_account",
			body:         "Action=AuthorizeEndpointAccess&Version=2012-12-01&ClusterIdentifier=ep-cluster",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		{
			name:         "cluster_not_found",
			body:         "Action=AuthorizeEndpointAccess&Version=2012-12-01&ClusterIdentifier=nonexistent&Account=111111111111",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ClusterNotFound"},
		},
		{
			name: "duplicate_authorization",
			setup: func(t *testing.T, h *redshift.Handler, _ *redshift.InMemoryBackend) {
				t.Helper()
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=dup-ep-cluster")
				postRedshiftForm(t, h, "Action=AuthorizeEndpointAccess&Version=2012-12-01"+
					"&ClusterIdentifier=dup-ep-cluster&Account=333333333333")
			},
			body: "Action=AuthorizeEndpointAccess&Version=2012-12-01" +
				"&ClusterIdentifier=dup-ep-cluster&Account=333333333333",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"EndpointAuthorizationAlreadyExists"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
			h := redshift.NewHandler(b)
			if tt.setup != nil {
				tt.setup(t, h, b)
			}

			rec := postRedshiftForm(t, h, tt.body)

			assert.Equal(t, tt.wantCode, rec.Code)
			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

func TestBackend_AuthorizeEndpointAccess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr   error
		setup     func(b *redshift.InMemoryBackend)
		name      string
		clusterID string
		grantee   string
		vpcIDs    []string
	}{
		{
			name: "success_no_vpcs",
			setup: func(b *redshift.InMemoryBackend) {
				_, _ = b.CreateCluster("ea-cluster", "", "", "", nil, "")
			},
			clusterID: "ea-cluster",
			grantee:   "111111111111",
			vpcIDs:    nil,
		},
		{
			name: "success_with_vpcs",
			setup: func(b *redshift.InMemoryBackend) {
				_, _ = b.CreateCluster("ea-cluster-vpcs", "", "", "", nil, "")
			},
			clusterID: "ea-cluster-vpcs",
			grantee:   "222222222222",
			vpcIDs:    []string{"vpc-1", "vpc-2"},
		},
		{
			name:      "empty_cluster_id",
			clusterID: "",
			grantee:   "111111111111",
			wantErr:   redshift.ErrInvalidParameter,
		},
		{
			name:      "empty_grantee",
			clusterID: "ea-cluster",
			grantee:   "",
			wantErr:   redshift.ErrInvalidParameter,
		},
		{
			name:      "cluster_not_found",
			clusterID: "nonexistent",
			grantee:   "111111111111",
			wantErr:   redshift.ErrClusterNotFound,
		},
		{
			name: "duplicate",
			setup: func(b *redshift.InMemoryBackend) {
				_, _ = b.CreateCluster("dup-ea-cluster", "", "", "", nil, "")
				_, _ = b.AuthorizeEndpointAccess("dup-ea-cluster", "333333333333", nil)
			},
			clusterID: "dup-ea-cluster",
			grantee:   "333333333333",
			wantErr:   redshift.ErrEndpointAuthAlreadyExists,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
			if tt.setup != nil {
				tt.setup(b)
			}

			auth, err := b.AuthorizeEndpointAccess(tt.clusterID, tt.grantee, tt.vpcIDs)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.clusterID, auth.ClusterIdentifier)
			assert.Equal(t, tt.grantee, auth.Grantee)
			if len(tt.vpcIDs) == 0 {
				assert.True(t, auth.AllowedAllVPCs)
			} else {
				assert.False(t, auth.AllowedAllVPCs)
				assert.Equal(t, tt.vpcIDs, auth.AllowedVPCs)
			}
		})
	}
}

// ---- AuthorizeEndpointAccess: AllowedAllVPCs when no VPCs specified ----

func TestAuthorizeEndpointAccess_AllowedAllVPCsWhenEmpty(t *testing.T) {
	t.Parallel()

	b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	h := redshift.NewHandler(b)

	_, err := b.CreateCluster("ea-cluster", "dc2.large", "dev", "admin", nil, "")
	require.NoError(t, err)

	rec := postRedshiftForm(t, h,
		"Action=AuthorizeEndpointAccess&Version=2012-12-01"+
			"&ClusterIdentifier=ea-cluster"+
			"&Account=111111111111")

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "AuthorizeEndpointAccessResponse")
	assert.Contains(t, body, "true") // AllowedAllVPCs
}

// ---- AuthorizeEndpointAccess: duplicate returns error ----

func TestAuthorizeEndpointAccess_DuplicateReturnsError(t *testing.T) {
	t.Parallel()

	b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	h := redshift.NewHandler(b)

	_, err := b.CreateCluster("ea-dup-cluster", "dc2.large", "dev", "admin", nil, "")
	require.NoError(t, err)

	body := "Action=AuthorizeEndpointAccess&Version=2012-12-01" +
		"&ClusterIdentifier=ea-dup-cluster" +
		"&Account=111111111111"

	rec1 := postRedshiftForm(t, h, body)
	require.Equal(t, http.StatusOK, rec1.Code)

	rec2 := postRedshiftForm(t, h, body)
	assert.Equal(t, http.StatusBadRequest, rec2.Code)
	assert.Contains(t, rec2.Body.String(), "EndpointAuthorizationAlreadyExists")
}

// ---- DescribeEndpointAuthorization ----

func TestHandler_DescribeEndpointAuthorization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(t *testing.T, h *redshift.Handler)
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name:         "empty",
			body:         "Action=DescribeEndpointAuthorization&Version=2012-12-01",
			wantCode:     http.StatusOK,
			wantContains: []string{"DescribeEndpointAuthorizationResponse"},
		},
		{
			name: "with_auth",
			setup: func(t *testing.T, h *redshift.Handler) {
				t.Helper()
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=ea-cluster")
				postRedshiftForm(
					t,
					h,
					"Action=AuthorizeEndpointAccess&Version=2012-12-01&ClusterIdentifier=ea-cluster&Account=acc1",
				)
			},
			body:         "Action=DescribeEndpointAuthorization&Version=2012-12-01&ClusterIdentifier=ea-cluster",
			wantCode:     http.StatusOK,
			wantContains: []string{"DescribeEndpointAuthorizationResponse", "ea-cluster", "acc1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newRedshiftHandler()
			if tt.setup != nil {
				tt.setup(t, h)
			}

			rec := postRedshiftForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

// TestHandler_DescribeEndpointAuthorization_GranteeAccountSide verifies which
// side of the grantor/grantee pair the Account filter compares against.
// api_op_DescribeEndpointAuthorization.go documents Account precisely: "the
// Amazon Web Services account ID of either the cluster owner (grantor) or
// grantee. If Grantee parameter is true, then the Account value is of the
// grantor" -- so Grantee=true filters by Grantor, and Grantee=false (default)
// filters by Grantee, the opposite pairing from what the field names suggest
// at a glance.
func TestHandler_DescribeEndpointAuthorization_GranteeAccountSide(t *testing.T) {
	t.Parallel()

	h := newRedshiftHandler()
	postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=grantee-side")
	postRedshiftForm(t, h,
		"Action=AuthorizeEndpointAccess&Version=2012-12-01"+
			"&ClusterIdentifier=grantee-side&Account=999999999999")

	// Default (grantor) view, Account = the grantee this backend authorized.
	rec := postRedshiftForm(t, h,
		"Action=DescribeEndpointAuthorization&Version=2012-12-01&Account=999999999999")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "grantee-side",
		"grantor view (Grantee omitted) must filter Account against the grantee")

	// Grantee=true view, Account = the grantor (this backend's own account, 000000000000).
	rec = postRedshiftForm(t, h,
		"Action=DescribeEndpointAuthorization&Version=2012-12-01&Grantee=true&Account=000000000000")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "grantee-side",
		"grantee view (Grantee=true) must filter Account against the grantor")
}

// ---- RevokeEndpointAccess ----

func TestHandler_RevokeEndpointAccess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(t *testing.T, h *redshift.Handler)
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "success",
			setup: func(t *testing.T, h *redshift.Handler) {
				t.Helper()
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=rea-cluster")
				postRedshiftForm(
					t,
					h,
					"Action=AuthorizeEndpointAccess&Version=2012-12-01&ClusterIdentifier=rea-cluster&Account=acc2",
				)
			},
			body:         "Action=RevokeEndpointAccess&Version=2012-12-01&ClusterIdentifier=rea-cluster&Account=acc2",
			wantCode:     http.StatusOK,
			wantContains: []string{"RevokeEndpointAccessResponse", "Revoking"},
		},
		{
			name:     "missing_cluster_id",
			body:     "Action=RevokeEndpointAccess&Version=2012-12-01&ClusterIdentifier=&Account=acc2",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "not_found",
			body:     "Action=RevokeEndpointAccess&Version=2012-12-01&ClusterIdentifier=missing&Account=acc2",
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newRedshiftHandler()
			if tt.setup != nil {
				tt.setup(t, h)
			}

			rec := postRedshiftForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

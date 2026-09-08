package redshift_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/redshift"
)

// TestGetClusterCredentials_Expiration verifies GetClusterCredentials
// serializes the Expiration field. It was previously computed by the backend
// but never wired into the response XML at all, unlike the sibling
// GetClusterCredentialsWithIAM operation which does include it -- a client
// (e.g. a JDBC/ODBC driver) has no way to know when the temporary password
// expires.
func TestGetClusterCredentials_Expiration(t *testing.T) {
	t.Parallel()

	h := newRedshiftHandler()
	postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=cred-cluster")

	rec := postRedshiftForm(t, h,
		"Action=GetClusterCredentials&Version=2012-12-01&ClusterIdentifier=cred-cluster&DbUser=alice")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "<Expiration>")
	assert.Contains(t, rec.Body.String(), "GetClusterCredentialsResult")
}

// ---- GetClusterCredentials ----

func TestHandler_GetClusterCredentials(t *testing.T) {
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
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=gcc-cluster")
			},
			body:         "Action=GetClusterCredentials&Version=2012-12-01&ClusterIdentifier=gcc-cluster&DbUser=admin",
			wantCode:     http.StatusOK,
			wantContains: []string{"GetClusterCredentialsResponse", "admin", "Tmp1_"},
		},
		{
			name:     "missing_cluster_id",
			body:     "Action=GetClusterCredentials&Version=2012-12-01&ClusterIdentifier=&DbUser=admin",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "missing_db_user",
			body:     "Action=GetClusterCredentials&Version=2012-12-01&ClusterIdentifier=gcc-cluster&DbUser=",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "cluster_not_found",
			body:     "Action=GetClusterCredentials&Version=2012-12-01&ClusterIdentifier=missing&DbUser=admin",
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

func TestBackend_GetClusterCredentials(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup     func(b *redshift.InMemoryBackend)
		name      string
		clusterID string
		dbUser    string
		wantErr   bool
	}{
		{
			name: "success",
			setup: func(b *redshift.InMemoryBackend) {
				_, _ = b.CreateCluster("c1", "dc2.large", "dev", "admin", nil, "")
			},
			clusterID: "c1",
			dbUser:    "alice",
			wantErr:   false,
		},
		{
			name:      "missing_cluster_id",
			clusterID: "",
			dbUser:    "alice",
			wantErr:   true,
		},
		{
			name:      "missing_db_user",
			clusterID: "c1",
			dbUser:    "",
			wantErr:   true,
		},
		{
			name:      "cluster_not_found",
			clusterID: "missing",
			dbUser:    "alice",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
			if tt.setup != nil {
				tt.setup(b)
			}

			creds, err := b.GetClusterCredentials(tt.clusterID, tt.dbUser, false)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.dbUser, creds.DBUser)
			assert.NotEmpty(t, creds.DBPassword)
			assert.False(t, creds.Expiration.IsZero())
		})
	}
}

// ---- GetClusterCredentialsWithIAM ----

func TestHandler_GetClusterCredentialsWithIAM(t *testing.T) {
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
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=iam-cluster")
			},
			body:         "Action=GetClusterCredentialsWithIAM&Version=2012-12-01&ClusterIdentifier=iam-cluster&DbName=dev",
			wantCode:     http.StatusOK,
			wantContains: []string{"GetClusterCredentialsWithIAMResponse", "DbUser", "DbPassword"},
		},
		{
			name:     "cluster_not_found",
			body:     "Action=GetClusterCredentialsWithIAM&Version=2012-12-01&ClusterIdentifier=missing",
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

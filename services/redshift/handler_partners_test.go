package redshift_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/redshift"
)

// ---- AddPartner ----

func TestHandler_AddPartner(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(t *testing.T, h *redshift.Handler, b *redshift.InMemoryBackend)
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "success",
			setup: func(t *testing.T, h *redshift.Handler, _ *redshift.InMemoryBackend) {
				t.Helper()
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=partner-cluster")
			},
			body: "Action=AddPartner&Version=2012-12-01" +
				"&ClusterIdentifier=partner-cluster&DatabaseName=mydb&PartnerName=my-partner",
			wantCode:     http.StatusOK,
			wantContains: []string{"AddPartnerResponse", "mydb", "my-partner"},
		},
		{
			name:         "missing_cluster_identifier",
			body:         "Action=AddPartner&Version=2012-12-01&DatabaseName=mydb&PartnerName=my-partner",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		{
			name:         "missing_database_name",
			body:         "Action=AddPartner&Version=2012-12-01&ClusterIdentifier=c1&PartnerName=my-partner",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		{
			name:         "missing_partner_name",
			body:         "Action=AddPartner&Version=2012-12-01&ClusterIdentifier=c1&DatabaseName=mydb",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		{
			name: "cluster_not_found",
			body: "Action=AddPartner&Version=2012-12-01" +
				"&ClusterIdentifier=nonexistent&DatabaseName=mydb&PartnerName=p1",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ClusterNotFound"},
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

func TestBackend_AddPartner(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr   error
		setup     func(b *redshift.InMemoryBackend)
		name      string
		accountID string
		clusterID string
		database  string
		partner   string
	}{
		{
			name: "success",
			setup: func(b *redshift.InMemoryBackend) {
				_, _ = b.CreateCluster("p-cluster", "", "", "", nil, "")
			},
			accountID: "000000000000",
			clusterID: "p-cluster",
			database:  "mydb",
			partner:   "my-partner",
		},
		{
			name:      "empty_cluster_id",
			clusterID: "",
			database:  "mydb",
			partner:   "my-partner",
			wantErr:   redshift.ErrInvalidParameter,
		},
		{
			name:      "empty_database",
			clusterID: "p-cluster",
			database:  "",
			partner:   "my-partner",
			wantErr:   redshift.ErrInvalidParameter,
		},
		{
			name:      "empty_partner",
			clusterID: "p-cluster",
			database:  "mydb",
			partner:   "",
			wantErr:   redshift.ErrInvalidParameter,
		},
		{
			name:      "cluster_not_found",
			clusterID: "nonexistent",
			database:  "mydb",
			partner:   "p1",
			wantErr:   redshift.ErrClusterNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
			if tt.setup != nil {
				tt.setup(b)
			}

			result, err := b.AddPartner(tt.accountID, tt.clusterID, tt.database, tt.partner)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.clusterID, result.ClusterIdentifier)
			assert.Equal(t, tt.database, result.DatabaseName)
			assert.Equal(t, tt.partner, result.PartnerName)
			assert.Equal(t, "Active", result.Status)
		})
	}
}

// TestAddPartner_ResponseOmitsClusterIdentifier locks in that AddPartnerResponse
// carries only DatabaseName and PartnerName. ClusterIdentifier is not a member of
// the real AddPartnerOutput (confirmed against
// aws-sdk-go-v2/service/redshift@v1.65.4/api_op_AddPartner.go) -- this backend
// previously echoed it as an invented third field, entrenched by a test that
// merely asserted the cluster id string appeared somewhere in the body.
func TestAddPartner_ResponseOmitsClusterIdentifier(t *testing.T) {
	t.Parallel()

	h := newRedshiftHandler()
	postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=ap-cluster")

	rec := postRedshiftForm(t, h,
		"Action=AddPartner&Version=2012-12-01"+
			"&ClusterIdentifier=ap-cluster"+
			"&DatabaseName=mydb"+
			"&PartnerName=mypartner")

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "AddPartnerResponse")
	assert.Contains(t, body, "mydb")
	assert.Contains(t, body, "mypartner")
	assert.NotContains(t, body, "ap-cluster",
		"AddPartnerOutput has no ClusterIdentifier member")
	assert.NotContains(t, body, "<ClusterIdentifier>")
}

// TestPartner_WireFieldIsPartnerName locks in that the Partner family's request
// parameter and response element are named "PartnerName" end to end. Before this
// fix, every Partner op (AddPartner, DeletePartner, DescribePartners,
// UpdatePartnerStatus) read/wrote a fabricated "PartnerIntegrationId" name that
// does not exist anywhere in the real aws-sdk-go-v2/service/redshift@v1.62.3 wire
// shape (confirmed against AddPartnerInput/Output, DescribePartnersInput, and
// PartnerIntegrationInfo, which all use PartnerName) -- so a real SDK client's
// PartnerName value was silently dropped on every request, and every response
// field a real client tried to read came back empty.
func TestPartner_WireFieldIsPartnerName(t *testing.T) {
	t.Parallel()

	h := newRedshiftHandler()
	postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=wire-cluster")

	rec := postRedshiftForm(t, h,
		"Action=AddPartner&Version=2012-12-01"+
			"&ClusterIdentifier=wire-cluster&DatabaseName=mydb&PartnerName=wire-partner")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "<PartnerName>wire-partner</PartnerName>")
	assert.NotContains(t, rec.Body.String(), "PartnerIntegrationId")

	rec = postRedshiftForm(t, h,
		"Action=DescribePartners&Version=2012-12-01&ClusterIdentifier=wire-cluster")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "<PartnerName>wire-partner</PartnerName>")
	assert.NotContains(t, rec.Body.String(), "PartnerIntegrationId")
}

// ---- DeletePartner ----

func TestHandler_DeletePartner(t *testing.T) {
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
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=dp-cluster")
				postRedshiftForm(
					t,
					h,
					"Action=AddPartner&Version=2012-12-01"+
						"&ClusterIdentifier=dp-cluster&DatabaseName=mydb&PartnerName=mypartner",
				)
			},
			body: "Action=DeletePartner&Version=2012-12-01" +
				"&ClusterIdentifier=dp-cluster&DatabaseName=mydb&PartnerName=mypartner",
			wantCode:     http.StatusOK,
			wantContains: []string{"DeletePartnerResponse", "mydb", "mypartner"},
		},
		{
			name: "not_found",
			body: "Action=DeletePartner&Version=2012-12-01" +
				"&ClusterIdentifier=dp-cluster&DatabaseName=mydb&PartnerName=missing",
			wantCode: http.StatusBadRequest,
		},
		{
			name: "missing_cluster_id",
			body: "Action=DeletePartner&Version=2012-12-01" +
				"&ClusterIdentifier=&DatabaseName=mydb&PartnerName=mypartner",
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

// ---- DescribePartners ----

func TestHandler_DescribePartners(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(t *testing.T, h *redshift.Handler)
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "empty",
			body: "Action=DescribePartners&Version=2012-12-01&ClusterIdentifier=c1",
			setup: func(t *testing.T, h *redshift.Handler) {
				t.Helper()
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=c1")
			},
			wantCode:     http.StatusOK,
			wantContains: []string{"DescribePartnersResponse"},
		},
		{
			name: "with_partner",
			setup: func(t *testing.T, h *redshift.Handler) {
				t.Helper()
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=c2")
				postRedshiftForm(
					t,
					h,
					"Action=AddPartner&Version=2012-12-01&ClusterIdentifier=c2&DatabaseName=db1&PartnerName=partner1",
				)
			},
			body:         "Action=DescribePartners&Version=2012-12-01&ClusterIdentifier=c2",
			wantCode:     http.StatusOK,
			wantContains: []string{"DescribePartnersResponse", "partner1", "c2"},
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

// ---- UpdatePartnerStatus ----

func TestHandler_UpdatePartnerStatus(t *testing.T) {
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
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=ups-cluster")
				postRedshiftForm(
					t,
					h,
					"Action=AddPartner&Version=2012-12-01"+
						"&ClusterIdentifier=ups-cluster&DatabaseName=db1&PartnerName=partner1",
				)
			},
			body: "Action=UpdatePartnerStatus&Version=2012-12-01" +
				"&ClusterIdentifier=ups-cluster&DatabaseName=db1&PartnerName=partner1&Status=Active&StatusMessage=ok",
			wantCode:     http.StatusOK,
			wantContains: []string{"UpdatePartnerStatusResponse", "db1", "partner1"},
		},
		{
			name: "not_found",
			body: "Action=UpdatePartnerStatus&Version=2012-12-01" +
				"&ClusterIdentifier=ups-cluster&DatabaseName=db1&PartnerName=missing",
			wantCode: http.StatusBadRequest,
		},
		{
			name: "missing_cluster_id",
			body: "Action=UpdatePartnerStatus&Version=2012-12-01" +
				"&ClusterIdentifier=&DatabaseName=db1&PartnerName=partner1",
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

// ---- Backend.DeletePartner ----

func TestBackend_DeletePartner(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup   func(b *redshift.InMemoryBackend)
		name    string
		cluster string
		db      string
		partner string
		wantErr bool
	}{
		{
			name: "success",
			setup: func(b *redshift.InMemoryBackend) {
				_, _ = b.CreateCluster("c1", "dc2.large", "dev", "admin", nil, "")
				_, _ = b.AddPartner("acc", "c1", "mydb", "partner1")
			},
			cluster: "c1",
			db:      "mydb",
			partner: "partner1",
			wantErr: false,
		},
		{
			name:    "missing_cluster_id",
			wantErr: true,
		},
		{
			name:    "not_found",
			cluster: "c1",
			db:      "mydb",
			partner: "nonexistent",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
			if tt.setup != nil {
				tt.setup(b)
			}

			err := b.DeletePartner("acc", tt.cluster, tt.db, tt.partner)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, 0, redshift.PartnerCount(b))
		})
	}
}

package redshift_test

import (
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	redshiftsdk "github.com/aws/aws-sdk-go-v2/service/redshift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/redshift"
)

// ---- ModifyAquaConfiguration ----

// TestRedshiftHandler_ModifyAquaConfiguration proves the previous stub bug is
// fixed: the handler ignored ClusterIdentifier entirely (took `_ url.Values`)
// and returned a canned 200 regardless of whether the cluster existed. Both
// the not_found and missing_id cases fail against the unfixed handler (it
// always returned 200 with AquaConfigurationStatus=auto).
func TestRedshiftHandler_ModifyAquaConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(h *redshift.Handler)
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "success",
			setup: func(h *redshift.Handler) {
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=aqua-cluster")
			},
			body:     "Action=ModifyAquaConfiguration&Version=2012-12-01&ClusterIdentifier=aqua-cluster",
			wantCode: http.StatusOK,
			// Real AQUA is retired: both fields are always "disabled" (confirmed
			// against types.AquaConfigurationStatus/AquaStatus doc comments), the
			// same convention DescribeClusters' own AquaConfiguration already
			// uses -- not the stub's previously-mismatched "auto".
			wantContains: []string{
				"ModifyAquaConfigurationResponse",
				"<AquaConfigurationStatus>disabled</AquaConfigurationStatus>",
				"<AquaStatus>disabled</AquaStatus>",
			},
		},
		{
			name:         "not_found",
			body:         "Action=ModifyAquaConfiguration&Version=2012-12-01&ClusterIdentifier=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ClusterNotFound"},
		},
		{
			name:         "missing_id",
			body:         "Action=ModifyAquaConfiguration&Version=2012-12-01",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newRedshiftHandler()

			if tt.setup != nil {
				tt.setup(h)
			}

			rec := postRedshiftForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

// ---- ModifyLakehouseConfiguration ----

// TestRedshiftHandler_ModifyLakehouseConfiguration proves the previous stub
// bug is fixed: the handler ignored every field (took `_ url.Values`) and
// returned a bare empty response regardless of whether the cluster existed.
// not_found and missing_id fail against the unfixed handler (always 200,
// empty body); idc_application_not_found proves the new cross-reference
// validation against this backend's own RedshiftIdcApplication store.
func TestRedshiftHandler_ModifyLakehouseConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(h *redshift.Handler)
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "success",
			setup: func(h *redshift.Handler) {
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=lh-cluster")
			},
			body: "Action=ModifyLakehouseConfiguration&Version=2012-12-01" +
				"&ClusterIdentifier=lh-cluster&CatalogName=mycatalog&LakehouseRegistration=Register",
			wantCode: http.StatusOK,
			wantContains: []string{
				"ModifyLakehouseConfigurationResponse",
				"<ClusterIdentifier>lh-cluster</ClusterIdentifier>",
				"mycatalog",
				"<LakehouseRegistrationStatus>Registered</LakehouseRegistrationStatus>",
			},
		},
		{
			name: "not_found",
			body: "Action=ModifyLakehouseConfiguration&Version=2012-12-01" +
				"&ClusterIdentifier=nonexistent&CatalogName=mycatalog",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ClusterNotFound"},
		},
		{
			name:         "missing_id",
			body:         "Action=ModifyLakehouseConfiguration&Version=2012-12-01&CatalogName=mycatalog",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		{
			name: "idc_application_not_found",
			setup: func(h *redshift.Handler) {
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=lh-idc-cluster")
			},
			body: "Action=ModifyLakehouseConfiguration&Version=2012-12-01" +
				"&ClusterIdentifier=lh-idc-cluster&LakehouseIdcRegistration=Associate" +
				"&LakehouseIdcApplicationArn=arn:aws:redshift:us-east-1:000000000000:redshiftidcapplication/no-such-app",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"RedshiftIdcApplicationNotExists"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newRedshiftHandler()

			if tt.setup != nil {
				tt.setup(h)
			}

			rec := postRedshiftForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

// TestModifyLakehouseConfiguration_IdcApplicationAssociation proves
// LakehouseIdcApplicationArn is validated against a REAL, previously-created
// RedshiftIdcApplication (this backend's own idc_applications.go state, not
// a fabricated check) and that the associated ARN round-trips on the
// response, while never leaking onto DescribeClusters (Cluster has no such
// member on the real wire).
func TestModifyLakehouseConfiguration_IdcApplicationAssociation(t *testing.T) {
	t.Parallel()

	h := newRedshiftHandler()
	postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=lh-assoc-cluster")

	rec := postRedshiftForm(t, h, "Action=CreateRedshiftIdcApplication&Version=2012-12-01"+
		"&RedshiftIdcApplicationName=lh-app&IdcInstanceArn=arn:aws:sso:::instance/abc"+
		"&IamRoleArn=arn:aws:iam::000000000000:role/MyRole")
	require.Equal(t, http.StatusOK, rec.Code)

	appArn := "arn:aws:redshift:us-east-1:000000000000:redshiftidcapplication/lh-app"

	rec = postRedshiftForm(t, h, "Action=ModifyLakehouseConfiguration&Version=2012-12-01"+
		"&ClusterIdentifier=lh-assoc-cluster&LakehouseIdcRegistration=Associate"+
		"&LakehouseIdcApplicationArn="+appArn)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), appArn)

	rec = postRedshiftForm(t, h, "Action=DescribeClusters&Version=2012-12-01&ClusterIdentifier=lh-assoc-cluster")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), "LakehouseIdcApplicationArn",
		"real Cluster shape has no LakehouseIdcApplicationArn member")
}

// TestModifyLakehouseConfiguration_DryRun proves DryRun validates and
// returns the would-be result without mutating backend state: a later
// DescribeClusters must not observe the CatalogArn/LakehouseRegistrationStatus
// the dry-run response reported.
func TestModifyLakehouseConfiguration_DryRun(t *testing.T) {
	t.Parallel()

	h := newRedshiftHandler()
	postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=lh-dryrun-cluster")

	rec := postRedshiftForm(t, h, "Action=ModifyLakehouseConfiguration&Version=2012-12-01"+
		"&ClusterIdentifier=lh-dryrun-cluster&CatalogName=mycatalog"+
		"&LakehouseRegistration=Register&DryRun=true")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "mycatalog", "dry run still reports the would-be result")

	rec = postRedshiftForm(t, h, "Action=DescribeClusters&Version=2012-12-01&ClusterIdentifier=lh-dryrun-cluster")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), "mycatalog", "dry run must not mutate the cluster's CatalogArn")
	assert.NotContains(t, rec.Body.String(), "Registered",
		"dry run must not mutate the cluster's LakehouseRegistrationStatus")
}

// TestModifyLakehouseConfiguration_PersistsAndCarriesForward proves the real
// mutation is observable via a later DescribeClusters (CatalogArn and
// LakehouseRegistrationStatus are real Cluster wire members, not just
// Modify's own response) and that a later call touching only
// LakehouseRegistration does not drop the previously-set catalog.
func TestModifyLakehouseConfiguration_PersistsAndCarriesForward(t *testing.T) {
	t.Parallel()

	h := newRedshiftHandler()
	postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=lh-persist-cluster")

	rec := postRedshiftForm(t, h, "Action=ModifyLakehouseConfiguration&Version=2012-12-01"+
		"&ClusterIdentifier=lh-persist-cluster&CatalogName=mycatalog&LakehouseRegistration=Register")
	require.Equal(t, http.StatusOK, rec.Code)

	rec = postRedshiftForm(t, h, "Action=DescribeClusters&Version=2012-12-01&ClusterIdentifier=lh-persist-cluster")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "mycatalog")
	assert.Contains(t, rec.Body.String(), "<LakehouseRegistrationStatus>Registered</LakehouseRegistrationStatus>")

	rec = postRedshiftForm(t, h, "Action=ModifyLakehouseConfiguration&Version=2012-12-01"+
		"&ClusterIdentifier=lh-persist-cluster&LakehouseRegistration=Deregister")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "mycatalog",
		"a call that changes only LakehouseRegistration must not drop the previously-set catalog")
	assert.Contains(t, rec.Body.String(), "<LakehouseRegistrationStatus>Deregistered</LakehouseRegistrationStatus>")
}

// TestSDKRoundTrip_ModifyLakehouseConfiguration drives the real
// aws-sdk-go-v2 client end to end: proves ModifyLakehouseConfigurationOutput
// decodes CatalogArn/ClusterIdentifier/LakehouseIdcApplicationArn/
// LakehouseRegistrationStatus, and that a subsequent DescribeClusters call
// decodes the same Cluster.CatalogArn/LakehouseRegistrationStatus wire
// members this pass added.
func TestSDKRoundTrip_ModifyLakehouseConfiguration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	backend := redshift.NewInMemoryBackend("000000000000", rtTestRegion)
	h := redshift.NewHandler(backend)
	client := newTestRedshiftClient(t, h)

	_, err := backend.CreateCluster("rt-lh-cluster", "dc2.large", "dev", "admin", nil, "")
	require.NoError(t, err)

	out, err := client.ModifyLakehouseConfiguration(ctx, &redshiftsdk.ModifyLakehouseConfigurationInput{
		ClusterIdentifier:     aws.String("rt-lh-cluster"),
		CatalogName:           aws.String("rtcatalog"),
		LakehouseRegistration: "Register",
	})
	require.NoError(t, err)
	assert.Equal(t, "rt-lh-cluster", aws.ToString(out.ClusterIdentifier))
	assert.Contains(t, aws.ToString(out.CatalogArn), "rtcatalog")
	assert.Equal(t, "Registered", aws.ToString(out.LakehouseRegistrationStatus))

	desc, err := client.DescribeClusters(ctx, &redshiftsdk.DescribeClustersInput{
		ClusterIdentifier: aws.String("rt-lh-cluster"),
	})
	require.NoError(t, err)
	require.Len(t, desc.Clusters, 1)
	assert.Contains(t, aws.ToString(desc.Clusters[0].CatalogArn), "rtcatalog")
	assert.Equal(t, "Registered", aws.ToString(desc.Clusters[0].LakehouseRegistrationStatus))
}

package quicksight_test

import (
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	quicksightsdk "github.com/aws/aws-sdk-go-v2/service/quicksight"
	"github.com/aws/aws-sdk-go-v2/service/quicksight/types"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/services/quicksight"
)

// newTestQuickSightClient stands up the real aws-sdk-go-v2 QuickSight client
// against an httptest server running this package's Handler, wired through
// the same pkgs/service registry/router used in production. See redshift's
// handler_sdk_roundtrip_test.go for why this matters over doRequest/
// httptest.NewRequest: the SDK's own deserializer -- not a handler-shaped
// fixture -- is what proves a response is wire-compatible, and the SDK's own
// serializer/validators are what prove a request was actually exercised the
// way a real client sends it (e.g. CreateDataSetInput's client-side
// PhysicalTableMap-required check in validators.go).
func newTestQuickSightClient(t *testing.T, h *quicksight.Handler) *quicksightsdk.Client {
	t.Helper()

	e := echo.New()
	registry := service.NewRegistry()
	require.NoError(t, registry.Register(h))
	e.Use(service.NewServiceRouter(registry).RouteHandler())

	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)

	cfg, err := awscfg.LoadDefaultConfig(
		t.Context(),
		awscfg.WithRegion(rtQSTestRegion),
		awscfg.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err)

	return quicksightsdk.NewFromConfig(cfg, func(o *quicksightsdk.Options) {
		o.BaseEndpoint = aws.String(srv.URL)
	})
}

const rtQSTestRegion = "us-east-1"

// TestSDKRoundTrip_DataSetPhysicalTableMap locks the fix for gopherstack-2qk4:
// CreateDataSet/UpdateDataSet never read PhysicalTableMap (required at
// quicksight@v1.123.1 api_op_CreateDataSet.go:55, api_op_UpdateDataSet.go:55),
// so a dataset reported success with no tables behind it. Driving the real
// SDK client is what proves this: the client's own validators.go
// (validateOpCreateDataSetInput) refuses to send a request missing
// PhysicalTableMap at all, so unlike a hand-built JSON fixture this test
// cannot accidentally omit the field the way the original bug did.
func TestSDKRoundTrip_DataSetPhysicalTableMap(t *testing.T) {
	t.Parallel()

	backend := quicksight.NewInMemoryBackend("000000000000", rtQSTestRegion)
	h := quicksight.NewHandler(backend)
	client := newTestQuickSightClient(t, h)
	ctx := t.Context()

	physicalTableMap := map[string]types.PhysicalTable{
		"pt1": &types.PhysicalTableMemberRelationalTable{
			Value: types.RelationalTable{
				DataSourceArn: aws.String(
					"arn:aws:quicksight:us-east-1:000000000000:datasource/src1",
				),
				Name:   aws.String("orders"),
				Schema: aws.String("public"),
				InputColumns: []types.InputColumn{
					{Name: aws.String("id"), Type: types.InputColumnDataTypeInteger},
					{Name: aws.String("total"), Type: types.InputColumnDataTypeDecimal},
				},
			},
		},
	}
	logicalTableMap := map[string]types.LogicalTable{
		"lt1": {
			Alias:  aws.String("orders_logical"),
			Source: &types.LogicalTableSource{PhysicalTableId: aws.String("pt1")},
		},
	}

	_, err := client.CreateDataSet(ctx, &quicksightsdk.CreateDataSetInput{
		AwsAccountId:     aws.String("000000000000"),
		DataSetId:        aws.String("rt-ds1"),
		Name:             aws.String("RoundTrip DataSet"),
		ImportMode:       types.DataSetImportModeDirectQuery,
		PhysicalTableMap: physicalTableMap,
		LogicalTableMap:  logicalTableMap, //nolint:staticcheck // still wired and tested; SDK marks it legacy, not removed
	})
	require.NoError(t, err)

	out, err := client.DescribeDataSet(ctx, &quicksightsdk.DescribeDataSetInput{
		AwsAccountId: aws.String("000000000000"),
		DataSetId:    aws.String("rt-ds1"),
	})
	require.NoError(t, err)
	require.NotNil(t, out.DataSet)

	require.Len(
		t,
		out.DataSet.PhysicalTableMap,
		1,
		"the dataset created above declared exactly one physical table",
	)
	member, ok := out.DataSet.PhysicalTableMap["pt1"].(*types.PhysicalTableMemberRelationalTable)
	require.True(
		t,
		ok,
		"PhysicalTableMap[pt1] must round-trip as the RelationalTable variant it was created with",
	)
	assert.Equal(
		t,
		"arn:aws:quicksight:us-east-1:000000000000:datasource/src1",
		aws.ToString(member.Value.DataSourceArn),
	)
	assert.Equal(t, "orders", aws.ToString(member.Value.Name))
	assert.Equal(t, "public", aws.ToString(member.Value.Schema))
	require.Len(t, member.Value.InputColumns, 2)
	assert.Equal(t, "id", aws.ToString(member.Value.InputColumns[0].Name))
	assert.Equal(t, types.InputColumnDataTypeInteger, member.Value.InputColumns[0].Type)

	require.Len(
		t,
		out.DataSet.LogicalTableMap,
		1,
		"the dataset created above also declared one logical table",
	)
	lt := out.DataSet.LogicalTableMap["lt1"]
	assert.Equal(t, "orders_logical", aws.ToString(lt.Alias))
	require.NotNil(t, lt.Source)
	assert.Equal(t, "pt1", aws.ToString(lt.Source.PhysicalTableId))
}

// TestSDKRoundTrip_UpdateDataSetPhysicalTableMap proves UpdateDataSet also
// threads PhysicalTableMap through (it shares the same required-field bug
// CreateDataSet had) and that a changed table definition is observable on
// the next DescribeDataSet, not just a 2xx from the update call.
func TestSDKRoundTrip_UpdateDataSetPhysicalTableMap(t *testing.T) {
	t.Parallel()

	backend := quicksight.NewInMemoryBackend("000000000000", rtQSTestRegion)
	h := quicksight.NewHandler(backend)
	client := newTestQuickSightClient(t, h)
	ctx := t.Context()

	firstTable := map[string]types.PhysicalTable{
		"pt1": &types.PhysicalTableMemberRelationalTable{
			Value: types.RelationalTable{
				DataSourceArn: aws.String(
					"arn:aws:quicksight:us-east-1:000000000000:datasource/src1",
				),
				Name: aws.String("orders"),
				InputColumns: []types.InputColumn{
					{Name: aws.String("id"), Type: types.InputColumnDataTypeInteger},
				},
			},
		},
	}
	_, err := client.CreateDataSet(ctx, &quicksightsdk.CreateDataSetInput{
		AwsAccountId:     aws.String("000000000000"),
		DataSetId:        aws.String("rt-ds2"),
		Name:             aws.String("Original"),
		ImportMode:       types.DataSetImportModeDirectQuery,
		PhysicalTableMap: firstTable,
	})
	require.NoError(t, err)

	replacementTable := map[string]types.PhysicalTable{
		"pt2": &types.PhysicalTableMemberCustomSql{
			Value: types.CustomSql{
				DataSourceArn: aws.String(
					"arn:aws:quicksight:us-east-1:000000000000:datasource/src1",
				),
				Name:     aws.String("custom_orders"),
				SqlQuery: aws.String("SELECT * FROM orders"),
			},
		},
	}
	_, err = client.UpdateDataSet(ctx, &quicksightsdk.UpdateDataSetInput{
		AwsAccountId:     aws.String("000000000000"),
		DataSetId:        aws.String("rt-ds2"),
		Name:             aws.String("Updated"),
		ImportMode:       types.DataSetImportModeDirectQuery,
		PhysicalTableMap: replacementTable,
	})
	require.NoError(t, err)

	out, err := client.DescribeDataSet(ctx, &quicksightsdk.DescribeDataSetInput{
		AwsAccountId: aws.String("000000000000"),
		DataSetId:    aws.String("rt-ds2"),
	})
	require.NoError(t, err)
	require.NotNil(t, out.DataSet)
	require.Len(
		t,
		out.DataSet.PhysicalTableMap,
		1,
		"UpdateDataSet must replace, not merge, PhysicalTableMap",
	)

	member, ok := out.DataSet.PhysicalTableMap["pt2"].(*types.PhysicalTableMemberCustomSql)
	require.True(
		t,
		ok,
		"the updated dataset's only physical table must be the CustomSql variant just sent",
	)
	assert.Equal(t, "SELECT * FROM orders", aws.ToString(member.Value.SqlQuery))
	assert.NotContains(
		t,
		out.DataSet.PhysicalTableMap,
		"pt1",
		"the old table id must not survive the update",
	)
}

// TestSDKRoundTrip_AnalysisThemeArn locks a fix found in a low-real-client-
// coverage audit pass: CreateAnalysisInput.ThemeArn/UpdateAnalysisInput.ThemeArn
// (both real, caller-supplied *string fields, api_op_CreateAnalysis.go/
// api_op_UpdateAnalysis.go) were read nowhere -- handleCreateAnalysis/
// handleUpdateAnalysis never extracted the field from the request body, and
// Analysis (types.go) had no slot to store it in, so a real client's ThemeArn
// was silently dropped and never observable on DescribeAnalysis or
// DescribeAnalysisDefinition (both of which carry a real ThemeArn member,
// confirmed against types.Analysis and api_op_DescribeAnalysisDefinition.go's
// DescribeAnalysisDefinitionOutput). Zero fabrication: the value only ever
// comes from what the caller supplied on Create/Update.
func TestSDKRoundTrip_AnalysisThemeArn(t *testing.T) {
	t.Parallel()

	backend := quicksight.NewInMemoryBackend("000000000000", rtQSTestRegion)
	h := quicksight.NewHandler(backend)
	client := newTestQuickSightClient(t, h)
	ctx := t.Context()

	const themeArn = "arn:aws:quicksight:us-east-1:000000000000:theme/theme1"

	_, err := client.CreateAnalysis(ctx, &quicksightsdk.CreateAnalysisInput{
		AwsAccountId: aws.String("000000000000"),
		AnalysisId:   aws.String("rt-an1"),
		Name:         aws.String("RoundTrip Analysis"),
		ThemeArn:     aws.String(themeArn),
	})
	require.NoError(t, err)

	describeOut, err := client.DescribeAnalysis(ctx, &quicksightsdk.DescribeAnalysisInput{
		AwsAccountId: aws.String("000000000000"),
		AnalysisId:   aws.String("rt-an1"),
	})
	require.NoError(t, err)
	require.NotNil(t, describeOut.Analysis)
	assert.Equal(t, themeArn, aws.ToString(describeOut.Analysis.ThemeArn),
		"DescribeAnalysis must echo back the ThemeArn supplied on CreateAnalysis")

	defOut, err := client.DescribeAnalysisDefinition(
		ctx,
		&quicksightsdk.DescribeAnalysisDefinitionInput{
			AwsAccountId: aws.String("000000000000"),
			AnalysisId:   aws.String("rt-an1"),
		},
	)
	require.NoError(t, err)
	assert.Equal(t, themeArn, aws.ToString(defOut.ThemeArn),
		"DescribeAnalysisDefinition's own top-level ThemeArn must also round-trip")

	const updatedThemeArn = "arn:aws:quicksight:us-east-1:000000000000:theme/theme2"

	_, err = client.UpdateAnalysis(ctx, &quicksightsdk.UpdateAnalysisInput{
		AwsAccountId: aws.String("000000000000"),
		AnalysisId:   aws.String("rt-an1"),
		Name:         aws.String("RoundTrip Analysis"),
		ThemeArn:     aws.String(updatedThemeArn),
	})
	require.NoError(t, err)

	describeOut, err = client.DescribeAnalysis(ctx, &quicksightsdk.DescribeAnalysisInput{
		AwsAccountId: aws.String("000000000000"),
		AnalysisId:   aws.String("rt-an1"),
	})
	require.NoError(t, err)
	require.NotNil(t, describeOut.Analysis)
	assert.Equal(t, updatedThemeArn, aws.ToString(describeOut.Analysis.ThemeArn),
		"UpdateAnalysis must replace the stored ThemeArn")
}

// TestSDKRoundTrip_DashboardVersion locks a parity-sweep fix (gopherstack-86y):
// DescribeDashboardOutput.Dashboard is types.Dashboard, whose version-specific
// members (Status/ThemeArn/VersionNumber/Description) live under a nested
// "Version" object -- confirmed against deserializers.go's
// awsRestjson1_deserializeDocumentDashboard, which has a "Version" case, and
// types.DashboardVersion, which carries ThemeArn/Status/VersionNumber/Description.
// Neither dashboardToMap nor CreateDashboard/UpdateDashboard's handlers ever
// built or populated that object: a real SDK client's
// describeOut.Dashboard.Version was always nil, and CreateDashboardInput.ThemeArn/
// UpdateDashboardInput.ThemeArn (real, optional, caller-supplied *string fields)
// were silently dropped on the wire, the same bug class already fixed for
// Analysis (TestSDKRoundTrip_AnalysisThemeArn) but missed for Dashboard.
func TestSDKRoundTrip_DashboardVersion(t *testing.T) {
	t.Parallel()

	backend := quicksight.NewInMemoryBackend("000000000000", rtQSTestRegion)
	h := quicksight.NewHandler(backend)
	client := newTestQuickSightClient(t, h)
	ctx := t.Context()

	const themeArn = "arn:aws:quicksight:us-east-1:000000000000:theme/theme1"

	_, err := client.CreateDashboard(ctx, &quicksightsdk.CreateDashboardInput{
		AwsAccountId:       aws.String("000000000000"),
		DashboardId:        aws.String("rt-dash1"),
		Name:               aws.String("RoundTrip Dashboard"),
		ThemeArn:           aws.String(themeArn),
		VersionDescription: aws.String("v1 description"),
	})
	require.NoError(t, err)

	describeOut, err := client.DescribeDashboard(ctx, &quicksightsdk.DescribeDashboardInput{
		AwsAccountId: aws.String("000000000000"),
		DashboardId:  aws.String("rt-dash1"),
	})
	require.NoError(t, err)
	require.NotNil(t, describeOut.Dashboard)
	require.NotNil(
		t,
		describeOut.Dashboard.Version,
		"DescribeDashboard must populate the nested Version object real clients read Status/ThemeArn/VersionNumber from",
	)
	assert.Equal(t, themeArn, aws.ToString(describeOut.Dashboard.Version.ThemeArn),
		"Version.ThemeArn must echo back the ThemeArn supplied on CreateDashboard")
	assert.Equal(t, "v1 description", aws.ToString(describeOut.Dashboard.Version.Description))
	assert.Equal(t, int64(1), aws.ToInt64(describeOut.Dashboard.Version.VersionNumber))
	assert.Equal(t, types.ResourceStatusCreationSuccessful, describeOut.Dashboard.Version.Status)

	const updatedThemeArn = "arn:aws:quicksight:us-east-1:000000000000:theme/theme2"

	_, err = client.UpdateDashboard(ctx, &quicksightsdk.UpdateDashboardInput{
		AwsAccountId:       aws.String("000000000000"),
		DashboardId:        aws.String("rt-dash1"),
		Name:               aws.String("RoundTrip Dashboard"),
		ThemeArn:           aws.String(updatedThemeArn),
		VersionDescription: aws.String("v2 description"),
	})
	require.NoError(t, err)

	describeOut, err = client.DescribeDashboard(ctx, &quicksightsdk.DescribeDashboardInput{
		AwsAccountId: aws.String("000000000000"),
		DashboardId:  aws.String("rt-dash1"),
	})
	require.NoError(t, err)
	require.NotNil(t, describeOut.Dashboard)
	require.NotNil(t, describeOut.Dashboard.Version)
	assert.Equal(t, updatedThemeArn, aws.ToString(describeOut.Dashboard.Version.ThemeArn),
		"UpdateDashboard must replace the stored ThemeArn")
	assert.Equal(t, "v2 description", aws.ToString(describeOut.Dashboard.Version.Description))
	assert.Equal(t, int64(2), aws.ToInt64(describeOut.Dashboard.Version.VersionNumber))
}

// TestSDKRoundTrip_DeleteDashboardVersionNumber locks a parity-sweep fix
// (gopherstack-86y): DeleteDashboardInput.VersionNumber's doc comment
// (api_op_DeleteDashboard.go) says "If the version number property is provided,
// only the specified version of the dashboard is deleted." The version-number
// query param (confirmed bound via
// awsRestjson1_serializeOpHttpBindingsDeleteDashboardInput) was never read by
// handleDeleteDashboard, so a client asking to delete just one old version
// instead had its entire dashboard deleted -- a destructive, silent
// over-deletion far outside the documented contract.
func TestSDKRoundTrip_DeleteDashboardVersionNumber(t *testing.T) {
	t.Parallel()

	backend := quicksight.NewInMemoryBackend("000000000000", rtQSTestRegion)
	h := quicksight.NewHandler(backend)
	client := newTestQuickSightClient(t, h)
	ctx := t.Context()

	_, err := client.CreateDashboard(ctx, &quicksightsdk.CreateDashboardInput{
		AwsAccountId: aws.String("000000000000"),
		DashboardId:  aws.String("rt-dash2"),
		Name:         aws.String("RoundTrip Dashboard 2"),
	})
	require.NoError(t, err)

	_, err = client.UpdateDashboard(ctx, &quicksightsdk.UpdateDashboardInput{
		AwsAccountId: aws.String("000000000000"),
		DashboardId:  aws.String("rt-dash2"),
		Name:         aws.String("RoundTrip Dashboard 2 v2"),
	})
	require.NoError(t, err)

	// Deleting a single, older version must not remove the dashboard itself.
	_, err = client.DeleteDashboard(ctx, &quicksightsdk.DeleteDashboardInput{
		AwsAccountId:  aws.String("000000000000"),
		DashboardId:   aws.String("rt-dash2"),
		VersionNumber: aws.Int64(1),
	})
	require.NoError(t, err)

	_, err = client.DescribeDashboard(ctx, &quicksightsdk.DescribeDashboardInput{
		AwsAccountId: aws.String("000000000000"),
		DashboardId:  aws.String("rt-dash2"),
	})
	require.NoError(
		t,
		err,
		"deleting a single named version must not delete the whole dashboard",
	)

	// A nonexistent version number is rejected rather than silently accepted.
	_, err = client.DeleteDashboard(ctx, &quicksightsdk.DeleteDashboardInput{
		AwsAccountId:  aws.String("000000000000"),
		DashboardId:   aws.String("rt-dash2"),
		VersionNumber: aws.Int64(99),
	})
	require.Error(t, err)

	// Omitting VersionNumber still deletes the whole dashboard.
	_, err = client.DeleteDashboard(ctx, &quicksightsdk.DeleteDashboardInput{
		AwsAccountId: aws.String("000000000000"),
		DashboardId:  aws.String("rt-dash2"),
	})
	require.NoError(t, err)

	_, err = client.DescribeDashboard(ctx, &quicksightsdk.DescribeDashboardInput{
		AwsAccountId: aws.String("000000000000"),
		DashboardId:  aws.String("rt-dash2"),
	})
	require.Error(t, err, "an unqualified DeleteDashboard must remove the whole dashboard")
}

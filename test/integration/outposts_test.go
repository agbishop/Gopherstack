package integration_test

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	ec2sdk "github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	outpostssdk "github.com/aws/aws-sdk-go-v2/service/outposts"
	outpoststypes "github.com/aws/aws-sdk-go-v2/service/outposts/types"
	smithy "github.com/aws/smithy-go"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createOutpostsClient is defined in tag_routing_test.go and reused here.

// outpostsCleanupCtx returns a context for use inside t.Cleanup callbacks.
// t.Context() must not be used there: Go 1.24+ cancels it before cleanups run.
func outpostsCleanupCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}

// outpostsErrorCode extracts the smithy error code from err, or "" if err isn't one.
func outpostsErrorCode(err error) string {
	if apiErr, ok := errors.AsType[smithy.APIError](err); ok {
		return apiErr.ErrorCode()
	}

	return ""
}

func uniqueOutpostsName(t *testing.T, prefix string) string {
	t.Helper()

	return prefix + "-" + uuid.NewString()
}

// createTestSite creates a Site with no addresses and registers its cleanup.
func createTestSite(
	ctx context.Context,
	t *testing.T,
	client *outpostssdk.Client,
) *outpoststypes.Site {
	t.Helper()

	out, err := client.CreateSite(ctx, &outpostssdk.CreateSiteInput{
		Name: aws.String(uniqueOutpostsName(t, "integ-site")),
	})
	require.NoError(t, err, "CreateSite should succeed")
	require.NotNil(t, out.Site)

	siteID := aws.ToString(out.Site.SiteId)
	t.Cleanup(func() {
		cctx, cancel := outpostsCleanupCtx()
		defer cancel()
		_, _ = client.DeleteSite(cctx, &outpostssdk.DeleteSiteInput{SiteId: aws.String(siteID)})
	})

	return out.Site
}

// createTestOutpost creates an Outpost under siteID and registers its cleanup.
func createTestOutpost(
	ctx context.Context, t *testing.T, client *outpostssdk.Client, siteID string,
) *outpoststypes.Outpost {
	t.Helper()

	out, err := client.CreateOutpost(ctx, &outpostssdk.CreateOutpostInput{
		Name:                  aws.String(uniqueOutpostsName(t, "integ-outpost")),
		SiteId:                aws.String(siteID),
		SupportedHardwareType: outpoststypes.SupportedHardwareTypeRack,
	})
	require.NoError(t, err, "CreateOutpost should succeed")
	require.NotNil(t, out.Outpost)

	outpostID := aws.ToString(out.Outpost.OutpostId)
	t.Cleanup(func() {
		cctx, cancel := outpostsCleanupCtx()
		defer cancel()
		_, _ = client.DeleteOutpost(
			cctx,
			&outpostssdk.DeleteOutpostInput{OutpostId: aws.String(outpostID)},
		)
	})

	return out.Outpost
}

// seededAssetID returns the ID of the single COMPUTE asset CreateOutpost seeds.
func seededAssetID(
	ctx context.Context,
	t *testing.T,
	client *outpostssdk.Client,
	outpostID string,
) string {
	t.Helper()

	out, err := client.ListAssets(
		ctx,
		&outpostssdk.ListAssetsInput{OutpostIdentifier: aws.String(outpostID)},
	)
	require.NoError(t, err, "ListAssets should succeed")
	require.NotEmpty(t, out.Assets, "CreateOutpost should have seeded one COMPUTE asset")

	return aws.ToString(out.Assets[0].AssetId)
}

// awaitOrderStatus polls GetOrder until it reports want.
func awaitOrderStatus(
	ctx context.Context,
	t *testing.T,
	client *outpostssdk.Client,
	orderID string,
	want outpoststypes.OrderStatus,
) {
	t.Helper()

	require.Eventually(t, func() bool {
		out, err := client.GetOrder(ctx, &outpostssdk.GetOrderInput{OrderId: aws.String(orderID)})

		return err == nil && out.Order.Status == want
	}, 5*time.Second, 50*time.Millisecond, "order should reach status %s", want)
}

// awaitCapacityTaskStatus polls GetCapacityTask until it reports want.
func awaitCapacityTaskStatus(
	ctx context.Context, t *testing.T, client *outpostssdk.Client, outpostID, taskID string,
	want outpoststypes.CapacityTaskStatus,
) {
	t.Helper()

	require.Eventually(t, func() bool {
		out, err := client.GetCapacityTask(ctx, &outpostssdk.GetCapacityTaskInput{
			OutpostIdentifier: aws.String(outpostID),
			CapacityTaskId:    aws.String(taskID),
		})

		return err == nil && out.CapacityTaskStatus == want
	}, 5*time.Second, 50*time.Millisecond, "capacity task should reach status %s", want)
}

// quoteARNFromOutpostARN builds a Quote ARN from a real Outpost ARN by
// swapping the resource segment -- both share the same
// "arn:{partition}:outposts:{region}:{account}:" prefix (confirmed via
// docs.aws.amazon.com/outposts/latest/APIReference/API_Quote.html's
// QuoteIdentifier Pattern).
func quoteARNFromOutpostARN(outpostARN, quoteID string) string {
	before, _, _ := strings.Cut(outpostARN, ":outpost/")

	return before + ":quote/" + quoteID
}

// TestIntegration_Outposts_SiteLifecycle drives Site CRUD plus its nested
// address and rack-physical-properties sub-resources sequentially, sharing
// one Site across sub-steps like test/integration/grafana_test.go's
// workspace lifecycle.
//
//nolint:paralleltest // sequential by design
func TestIntegration_Outposts_SiteLifecycle(t *testing.T) {
	dumpContainerLogsOnFailure(t)

	ctx := t.Context()
	client := createOutpostsClient(t)
	site := createTestSite(ctx, t, client)
	siteID := aws.ToString(site.SiteId)

	t.Run("get", func(t *testing.T) { //nolint:paralleltest // sequential by design
		out, err := client.GetSite(ctx, &outpostssdk.GetSiteInput{SiteId: aws.String(siteID)})
		require.NoError(t, err, "GetSite should succeed")
		require.NotNil(t, out.Site)
		assert.Equal(t, siteID, aws.ToString(out.Site.SiteId))
		assert.NotEmpty(t, aws.ToString(out.Site.SiteArn))
	})

	t.Run("update", func(t *testing.T) { //nolint:paralleltest // sequential by design
		out, err := client.UpdateSite(ctx, &outpostssdk.UpdateSiteInput{
			SiteId:      aws.String(siteID),
			Description: aws.String("updated description"),
			Notes:       aws.String("updated notes"),
		})
		require.NoError(t, err, "UpdateSite should succeed")
		require.NotNil(t, out.Site)
		assert.Equal(t, "updated description", aws.ToString(out.Site.Description))
		assert.Equal(t, "updated notes", aws.ToString(out.Site.Notes))
	})

	t.Run("address", func(t *testing.T) { //nolint:paralleltest // sequential by design
		addr := &outpoststypes.Address{
			AddressLine1:       aws.String("123 Main St"),
			City:               aws.String("Seattle"),
			ContactName:        aws.String("Jane Doe"),
			ContactPhoneNumber: aws.String("+12065550100"),
			CountryCode:        aws.String("US"),
			PostalCode:         aws.String("98101"),
			StateOrRegion:      aws.String("WA"),
		}

		updateOut, err := client.UpdateSiteAddress(ctx, &outpostssdk.UpdateSiteAddressInput{
			SiteId:      aws.String(siteID),
			AddressType: outpoststypes.AddressTypeShippingAddress,
			Address:     addr,
		})
		require.NoError(t, err, "UpdateSiteAddress should succeed")
		require.NotNil(t, updateOut.Address)
		assert.Equal(t, "Seattle", aws.ToString(updateOut.Address.City))

		getOut, err := client.GetSiteAddress(ctx, &outpostssdk.GetSiteAddressInput{
			SiteId:      aws.String(siteID),
			AddressType: outpoststypes.AddressTypeShippingAddress,
		})
		require.NoError(t, err, "GetSiteAddress should succeed")
		require.NotNil(t, getOut.Address)
		assert.Equal(t, "123 Main St", aws.ToString(getOut.Address.AddressLine1))
		assert.Equal(t, "US", aws.ToString(getOut.Address.CountryCode))
	})

	t.Run("rack_properties", func(t *testing.T) { //nolint:paralleltest // sequential by design
		out, err := client.UpdateSiteRackPhysicalProperties(
			ctx,
			&outpostssdk.UpdateSiteRackPhysicalPropertiesInput{
				SiteId:                    aws.String(siteID),
				PowerConnector:            outpoststypes.PowerConnectorCs8365c,
				PowerDrawKva:              outpoststypes.PowerDrawKvaPower15Kva,
				PowerPhase:                outpoststypes.PowerPhaseThreePhase,
				FiberOpticCableType:       outpoststypes.FiberOpticCableTypeSingleMode,
				OpticalStandard:           outpoststypes.OpticalStandardOptic10gbaseLr,
				MaximumSupportedWeightLbs: outpoststypes.MaximumSupportedWeightLbsMax2000Lbs,
				UplinkGbps:                outpoststypes.UplinkGbpsUplink10g,
				UplinkCount:               outpoststypes.UplinkCountUplinkCount2,
			},
		)
		require.NoError(t, err, "UpdateSiteRackPhysicalProperties should succeed")
		require.NotNil(t, out.Site)
		require.NotNil(t, out.Site.RackPhysicalProperties)
		assert.Equal(
			t,
			outpoststypes.PowerConnectorCs8365c,
			out.Site.RackPhysicalProperties.PowerConnector,
		)
		assert.Equal(
			t,
			outpoststypes.PowerPhaseThreePhase,
			out.Site.RackPhysicalProperties.PowerPhase,
		)
	})

	t.Run("list", func(t *testing.T) { //nolint:paralleltest // sequential by design
		out, err := client.ListSites(ctx, &outpostssdk.ListSitesInput{})
		require.NoError(t, err, "ListSites should succeed")

		found := false

		for _, s := range out.Sites {
			if aws.ToString(s.SiteId) == siteID {
				found = true

				break
			}
		}

		assert.True(t, found, "created site should appear in ListSites")
	})
}

// TestIntegration_Outposts_OutpostLifecycle drives Outpost CRUD, its seeded
// Asset, instance-type lookups, and decommission sequentially against one
// Outpost.
//
//nolint:paralleltest // sequential by design
func TestIntegration_Outposts_OutpostLifecycle(t *testing.T) {
	dumpContainerLogsOnFailure(t)

	ctx := t.Context()
	client := createOutpostsClient(t)
	site := createTestSite(ctx, t, client)
	outpost := createTestOutpost(ctx, t, client, aws.ToString(site.SiteId))
	outpostID := aws.ToString(outpost.OutpostId)
	outpostARN := aws.ToString(outpost.OutpostArn)

	require.Equal(t, outpoststypes.SupportedHardwareTypeRack, outpost.SupportedHardwareType)
	require.Equal(t, "ACTIVE", aws.ToString(outpost.LifeCycleStatus))
	require.True(
		t,
		strings.HasPrefix(outpostID, "op-"),
		"OutpostId should have the confirmed op- prefix",
	)
	require.NotEmpty(t, outpostARN)

	t.Run("get_by_arn", func(t *testing.T) { //nolint:paralleltest // sequential by design
		out, err := client.GetOutpost(
			ctx,
			&outpostssdk.GetOutpostInput{OutpostId: aws.String(outpostARN)},
		)
		require.NoError(t, err, "GetOutpost by ARN should resolve id-or-ARN")
		require.NotNil(t, out.Outpost)
		assert.Equal(t, outpostID, aws.ToString(out.Outpost.OutpostId))
	})

	t.Run("update", func(t *testing.T) { //nolint:paralleltest // sequential by design
		out, err := client.UpdateOutpost(ctx, &outpostssdk.UpdateOutpostInput{
			OutpostId:   aws.String(outpostID),
			Description: aws.String("updated outpost description"),
		})
		require.NoError(t, err, "UpdateOutpost should succeed")
		require.NotNil(t, out.Outpost)
		assert.Equal(t, "updated outpost description", aws.ToString(out.Outpost.Description))
	})

	//nolint:paralleltest // sequential by design
	t.Run(
		"list_filters_by_lifecycle_status",
		func(t *testing.T) {
			out, err := client.ListOutposts(ctx, &outpostssdk.ListOutpostsInput{
				LifeCycleStatusFilter: []string{"ACTIVE"},
			})
			require.NoError(t, err, "ListOutposts should succeed")

			found := false

			for _, o := range out.Outposts {
				if aws.ToString(o.OutpostId) == outpostID {
					found = true

					break
				}
			}

			assert.True(t, found, "created outpost should appear in the ACTIVE-filtered list")
		},
	)

	t.Run("seeded_asset", func(t *testing.T) { //nolint:paralleltest // sequential by design
		out, err := client.ListAssets(
			ctx,
			&outpostssdk.ListAssetsInput{OutpostIdentifier: aws.String(outpostID)},
		)
		require.NoError(t, err, "ListAssets should succeed")
		require.Len(t, out.Assets, 1, "CreateOutpost should seed exactly one Asset")
		assert.Equal(t, outpoststypes.AssetTypeCompute, out.Assets[0].AssetType)
		require.NotNil(t, out.Assets[0].ComputeAttributes)
		assert.Equal(
			t,
			outpoststypes.ComputeAssetStateActive,
			out.Assets[0].ComputeAttributes.State,
		)
	})

	//nolint:paralleltest // sequential by design
	t.Run(
		"instance_types_before_task",
		func(t *testing.T) {
			out, err := client.GetOutpostInstanceTypes(
				ctx,
				&outpostssdk.GetOutpostInstanceTypesInput{
					OutpostId: aws.String(outpostID),
				},
			)
			require.NoError(t, err, "GetOutpostInstanceTypes should succeed")
			assert.Empty(t, out.InstanceTypes, "no capacity task has run yet")
		},
	)

	//nolint:paralleltest // sequential by design
	t.Run(
		"supported_instance_types",
		func(t *testing.T) {
			out, err := client.GetOutpostSupportedInstanceTypes(
				ctx,
				&outpostssdk.GetOutpostSupportedInstanceTypesInput{
					OutpostIdentifier: aws.String(outpostID),
				},
			)
			require.NoError(t, err, "GetOutpostSupportedInstanceTypes should succeed")
			assert.NotEmpty(
				t,
				out.InstanceTypes,
				"RACK hardware should have seeded supported instance types",
			)
		},
	)

	t.Run("decommission", func(t *testing.T) { //nolint:paralleltest // sequential by design
		firstOut, err := client.StartOutpostDecommission(
			ctx,
			&outpostssdk.StartOutpostDecommissionInput{
				OutpostIdentifier: aws.String(outpostID),
			},
		)
		require.NoError(t, err, "StartOutpostDecommission should succeed")
		assert.Equal(t, outpoststypes.DecommissionRequestStatusRequested, firstOut.Status)

		replayOut, err := client.StartOutpostDecommission(
			ctx,
			&outpostssdk.StartOutpostDecommissionInput{
				OutpostIdentifier: aws.String(outpostID),
			},
		)
		require.NoError(t, err, "idempotent replay should succeed")
		assert.Equal(t, outpoststypes.DecommissionRequestStatusSkipped, replayOut.Status)

		getOut, err := client.GetOutpost(
			ctx,
			&outpostssdk.GetOutpostInput{OutpostId: aws.String(outpostID)},
		)
		require.NoError(t, err, "GetOutpost should succeed")
		assert.Equal(t, "PENDING_DECOMMISSION", aws.ToString(getOut.Outpost.LifeCycleStatus))
	})
}

// TestIntegration_Outposts_OutpostQuota proves CreateOutpost enforces AWS's
// real published "Outposts per site" default quota of 10
// (docs.aws.amazon.com/outposts/latest/userguide/outposts-limits.html).
//
//nolint:paralleltest // shared site across sub-steps
func TestIntegration_Outposts_OutpostQuota(t *testing.T) {
	dumpContainerLogsOnFailure(t)

	const maxOutpostsPerSite = 10

	ctx := t.Context()
	client := createOutpostsClient(t)
	site := createTestSite(ctx, t, client)
	siteID := aws.ToString(site.SiteId)

	for i := range maxOutpostsPerSite {
		_ = createTestOutpost(ctx, t, client, siteID)
		_ = i
	}

	_, err := client.CreateOutpost(ctx, &outpostssdk.CreateOutpostInput{
		Name:   aws.String(uniqueOutpostsName(t, "integ-outpost-over-quota")),
		SiteId: aws.String(siteID),
	})
	require.Error(t, err, "the 11th Outpost on one site should exceed the real quota")
	assert.Equal(t, "ServiceQuotaExceededException", outpostsErrorCode(err))
}

// TestIntegration_Outposts_CatalogItems drives the static catalog family and
// its filter permutations.
func TestIntegration_Outposts_CatalogItems(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	ctx := t.Context()
	client := createOutpostsClient(t)

	t.Run("get_item", func(t *testing.T) {
		t.Parallel()

		out, err := client.GetCatalogItem(ctx, &outpostssdk.GetCatalogItemInput{
			CatalogItemId: aws.String("OR-RACKM05"),
		})
		require.NoError(t, err, "GetCatalogItem should succeed")
		require.NotNil(t, out.CatalogItem)
		assert.Equal(t, "OR-RACKM05", aws.ToString(out.CatalogItem.CatalogItemId))
		assert.NotEmpty(t, out.CatalogItem.EC2Capacities)
	})

	t.Run("get_item_not_found", func(t *testing.T) {
		t.Parallel()

		_, err := client.GetCatalogItem(ctx, &outpostssdk.GetCatalogItemInput{
			CatalogItemId: aws.String("OR-0000000"),
		})
		require.Error(t, err)
		assert.Equal(t, "NotFoundException", outpostsErrorCode(err))
	})

	filterTests := []struct {
		name       string
		wantItemID string
		filter     []outpoststypes.CatalogItemClass
	}{
		{
			name:       "rack class",
			filter:     []outpoststypes.CatalogItemClass{outpoststypes.CatalogItemClassRack},
			wantItemID: "OR-RACKM05",
		},
		{
			name:       "server class",
			filter:     []outpoststypes.CatalogItemClass{outpoststypes.CatalogItemClassServer},
			wantItemID: "OR-SRVC6ID",
		},
	}

	for _, tt := range filterTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			out, err := client.ListCatalogItems(ctx, &outpostssdk.ListCatalogItemsInput{
				ItemClassFilter: tt.filter,
			})
			require.NoError(t, err, "ListCatalogItems should succeed")

			ids := make([]string, 0, len(out.CatalogItems))
			for _, item := range out.CatalogItems {
				ids = append(ids, aws.ToString(item.CatalogItemId))
			}

			assert.Contains(t, ids, tt.wantItemID)
		})
	}

	t.Run("orderable_instance_types", func(t *testing.T) {
		t.Parallel()

		out, err := client.ListOrderableInstanceTypes(
			ctx,
			&outpostssdk.ListOrderableInstanceTypesInput{},
		)
		require.NoError(t, err, "ListOrderableInstanceTypes should succeed")
		assert.NotEmpty(t, out.InstanceTypes)
	})
}

// TestIntegration_Outposts_OrderAndQuoteLifecycle drives CreateQuote through
// its ARN-or-ID identifier, CreateOrder's async completion, and quote
// consumption sequentially against one Outpost.
//
//nolint:paralleltest // sequential by design
func TestIntegration_Outposts_OrderAndQuoteLifecycle(t *testing.T) {
	dumpContainerLogsOnFailure(t)

	ctx := t.Context()
	client := createOutpostsClient(t)
	site := createTestSite(ctx, t, client)
	outpost := createTestOutpost(ctx, t, client, aws.ToString(site.SiteId))
	outpostID := aws.ToString(outpost.OutpostId)
	outpostARN := aws.ToString(outpost.OutpostArn)

	quoteOut, quoteErr := client.CreateQuote(ctx, &outpostssdk.CreateQuoteInput{
		CountryCode:       aws.String("US"),
		OutpostIdentifier: aws.String(outpostID),
		RequestedCapacities: []outpoststypes.QuoteCapacity{
			{
				QuoteCapacityType: outpoststypes.QuoteCapacityTypeEc2,
				Quantity:          aws.Float32(1),
				Unit:              aws.String("c5.24xlarge"),
			},
		},
	})
	require.NoError(t, quoteErr, "CreateQuote should succeed")
	require.NotNil(t, quoteOut.Quote)

	quoteID := aws.ToString(quoteOut.Quote.QuoteId)
	quoteARN := quoteARNFromOutpostARN(outpostARN, quoteID)

	t.Cleanup(func() {
		cctx, cancel := outpostsCleanupCtx()
		defer cancel()
		_, _ = client.DeleteQuote(
			cctx,
			&outpostssdk.DeleteQuoteInput{QuoteIdentifier: aws.String(quoteID)},
		)
	})

	require.True(
		t,
		strings.HasPrefix(quoteID, "oq-"),
		"QuoteId should have the confirmed oq- prefix",
	)
	assert.Equal(t, outpoststypes.QuoteStatusCreated, quoteOut.Quote.QuoteStatus)
	assert.NotEmpty(t, quoteOut.Quote.OrderingRequirements)

	t.Run("get_by_arn", func(t *testing.T) { //nolint:paralleltest // sequential by design
		out, err := client.GetQuote(
			ctx,
			&outpostssdk.GetQuoteInput{QuoteIdentifier: aws.String(quoteARN)},
		)
		require.NoError(t, err, "GetQuote by the ARN-shaped QuoteIdentifier form should resolve")
		require.NotNil(t, out.Quote)
		assert.Equal(t, quoteID, aws.ToString(out.Quote.QuoteId))
	})

	t.Run("update_by_arn", func(t *testing.T) { //nolint:paralleltest // sequential by design
		out, err := client.UpdateQuote(ctx, &outpostssdk.UpdateQuoteInput{
			QuoteIdentifier: aws.String(quoteARN),
			Description:     aws.String("updated via ARN"),
		})
		require.NoError(t, err, "UpdateQuote by ARN should resolve")
		require.NotNil(t, out.Quote)
		assert.Equal(t, "updated via ARN", aws.ToString(out.Quote.Description))
	})

	orderOut, orderErr := client.CreateOrder(ctx, &outpostssdk.CreateOrderInput{
		OutpostIdentifier: aws.String(outpostID),
		PaymentOption:     outpoststypes.PaymentOptionAllUpfront,
		QuoteIdentifier:   aws.String(quoteID),
		LineItems: []outpoststypes.LineItemRequest{
			{CatalogItemId: aws.String("OR-RACKM05"), Quantity: aws.Int32(1)},
		},
	})
	require.NoError(t, orderErr, "CreateOrder should succeed")
	require.NotNil(t, orderOut.Order)

	orderID := aws.ToString(orderOut.Order.OrderId)
	require.True(
		t,
		strings.HasPrefix(orderID, "oo-"),
		"OrderId should have the confirmed oo- prefix",
	)

	t.Cleanup(func() {
		cctx, cancel := outpostsCleanupCtx()
		defer cancel()
		_, _ = client.CancelOrder(cctx, &outpostssdk.CancelOrderInput{OrderId: aws.String(orderID)})
	})

	t.Run(
		"transitions_through_real_intermediate_states",
		func(t *testing.T) { //nolint:paralleltest // sequential by design
			awaitOrderStatus(ctx, t, client, orderID, outpoststypes.OrderStatusInProgress)

			inProgress, err := client.GetOrder(
				ctx,
				&outpostssdk.GetOrderInput{OrderId: aws.String(orderID)},
			)
			require.NoError(t, err, "GetOrder should succeed")
			assert.Equal(
				t,
				outpoststypes.LineItemStatusBuilding,
				inProgress.Order.LineItems[0].Status,
			)

			awaitOrderStatus(ctx, t, client, orderID, outpoststypes.OrderStatusDelivered)

			delivered, err := client.GetOrder(
				ctx,
				&outpostssdk.GetOrderInput{OrderId: aws.String(orderID)},
			)
			require.NoError(t, err, "GetOrder should succeed")
			assert.Equal(
				t,
				outpoststypes.LineItemStatusDelivered,
				delivered.Order.LineItems[0].Status,
			)

			awaitOrderStatus(ctx, t, client, orderID, outpoststypes.OrderStatusCompleted)

			completed, err := client.GetOrder(
				ctx,
				&outpostssdk.GetOrderInput{OrderId: aws.String(orderID)},
			)
			require.NoError(t, err, "GetOrder should succeed")
			assert.Equal(
				t,
				outpoststypes.LineItemStatusInstalled,
				completed.Order.LineItems[0].Status,
			)
		},
	)

	t.Run("quote_consumed", func(t *testing.T) { //nolint:paralleltest // sequential by design
		out, err := client.GetQuote(
			ctx,
			&outpostssdk.GetQuoteInput{QuoteIdentifier: aws.String(quoteID)},
		)
		require.NoError(t, err, "GetQuote should succeed")
		assert.Equal(t, outpoststypes.QuoteStatusOrderSubmitted, out.Quote.QuoteStatus)
		assert.Equal(t, orderID, aws.ToString(out.Quote.SubmittedOrderId))
	})

	//nolint:paralleltest // sequential by design
	t.Run(
		"cancel_completed_order_conflicts",
		func(t *testing.T) {
			_, err := client.CancelOrder(
				ctx,
				&outpostssdk.CancelOrderInput{OrderId: aws.String(orderID)},
			)
			require.Error(t, err, "a COMPLETED order should not be cancellable")
			assert.Equal(t, "ConflictException", outpostsErrorCode(err))
		},
	)

	//nolint:paralleltest // sequential by design
	t.Run(
		"list_orders_by_outpost",
		func(t *testing.T) {
			out, err := client.ListOrders(ctx, &outpostssdk.ListOrdersInput{
				OutpostIdentifierFilter: aws.String(outpostID),
			})
			require.NoError(t, err, "ListOrders should succeed")

			found := false

			for _, o := range out.Orders {
				if aws.ToString(o.OrderId) == orderID {
					found = true

					break
				}
			}

			assert.True(t, found, "created order should appear in ListOrders")
		},
	)
}

// TestIntegration_Outposts_CapacityTaskLifecycle drives StartCapacityTask's
// async completion and the real capacity-ledger mutation it applies to the
// Outpost's seeded Asset.
//
//nolint:paralleltest // sequential by design
func TestIntegration_Outposts_CapacityTaskLifecycle(t *testing.T) {
	dumpContainerLogsOnFailure(t)

	ctx := t.Context()
	client := createOutpostsClient(t)
	site := createTestSite(ctx, t, client)
	outpost := createTestOutpost(ctx, t, client, aws.ToString(site.SiteId))
	outpostID := aws.ToString(outpost.OutpostId)
	assetID := seededAssetID(ctx, t, client, outpostID)

	startOut, startErr := client.StartCapacityTask(ctx, &outpostssdk.StartCapacityTaskInput{
		OutpostIdentifier: aws.String(outpostID),
		AssetId:           aws.String(assetID),
		InstancePools: []outpoststypes.InstanceTypeCapacity{
			{InstanceType: aws.String("m5.xlarge"), Count: 2},
		},
	})
	require.NoError(t, startErr, "StartCapacityTask should succeed")

	taskID := aws.ToString(startOut.CapacityTaskId)
	require.True(
		t,
		strings.HasPrefix(taskID, "cap-"),
		"CapacityTaskId should have the confirmed cap- prefix",
	)
	assert.Equal(t, outpoststypes.CapacityTaskStatusRequested, startOut.CapacityTaskStatus)

	//nolint:paralleltest // sequential by design
	t.Run(
		"transitions_through_in_progress_before_completing",
		func(t *testing.T) {
			awaitCapacityTaskStatus(
				ctx,
				t,
				client,
				outpostID,
				taskID,
				outpoststypes.CapacityTaskStatusInProgress,
			)

			preCompletion, err := client.GetOutpostInstanceTypes(
				ctx,
				&outpostssdk.GetOutpostInstanceTypesInput{OutpostId: aws.String(outpostID)},
			)
			require.NoError(t, err, "GetOutpostInstanceTypes should succeed")
			assert.Empty(t, preCompletion.InstanceTypes, "capacity must not apply until COMPLETED")
		},
	)

	//nolint:paralleltest // sequential by design
	t.Run(
		"completes_async_and_mutates_capacity_ledger",
		func(t *testing.T) {
			awaitCapacityTaskStatus(
				ctx,
				t,
				client,
				outpostID,
				taskID,
				outpoststypes.CapacityTaskStatusCompleted,
			)

			typesOut, err := client.GetOutpostInstanceTypes(
				ctx,
				&outpostssdk.GetOutpostInstanceTypesInput{
					OutpostId: aws.String(outpostID),
				},
			)
			require.NoError(t, err, "GetOutpostInstanceTypes should succeed")
			require.Len(
				t,
				typesOut.InstanceTypes,
				1,
				"the completed task's requested pool should now be configured",
			)
			assert.Equal(t, "m5.xlarge", aws.ToString(typesOut.InstanceTypes[0].InstanceType))
		},
	)

	t.Run("list_by_outpost", func(t *testing.T) { //nolint:paralleltest // sequential by design
		out, err := client.ListCapacityTasks(ctx, &outpostssdk.ListCapacityTasksInput{
			OutpostIdentifierFilter: aws.String(outpostID),
		})
		require.NoError(t, err, "ListCapacityTasks should succeed")

		found := false

		for _, task := range out.CapacityTasks {
			if aws.ToString(task.CapacityTaskId) == taskID {
				found = true

				break
			}
		}

		assert.True(t, found, "created capacity task should appear in ListCapacityTasks")
	})

	//nolint:paralleltest // sequential by design
	t.Run(
		"blocking_instances_honest_empty",
		func(t *testing.T) {
			out, err := client.ListBlockingInstancesForCapacityTask(
				ctx,
				&outpostssdk.ListBlockingInstancesForCapacityTaskInput{
					OutpostIdentifier: aws.String(outpostID),
					CapacityTaskId:    aws.String(taskID),
				},
			)
			require.NoError(
				t,
				err,
				"ListBlockingInstancesForCapacityTask should validate and succeed",
			)
			assert.Empty(
				t,
				out.BlockingInstances,
				"no cross-service EC2-on-Outposts data exists -- honest empty",
			)
		},
	)

	//nolint:paralleltest // sequential by design
	t.Run(
		"cancel_completed_task_conflicts",
		func(t *testing.T) {
			_, err := client.CancelCapacityTask(ctx, &outpostssdk.CancelCapacityTaskInput{
				OutpostIdentifier: aws.String(outpostID),
				CapacityTaskId:    aws.String(taskID),
			})
			require.Error(t, err, "a COMPLETED capacity task should not be cancellable")
			assert.Equal(t, "ConflictException", outpostsErrorCode(err))
		},
	)

	//nolint:paralleltest // sequential by design
	t.Run(
		"dry_run_preserves_capacity",
		func(t *testing.T) {
			dryOut, err := client.StartCapacityTask(ctx, &outpostssdk.StartCapacityTaskInput{
				OutpostIdentifier: aws.String(outpostID),
				AssetId:           aws.String(assetID),
				DryRun:            true,
				InstancePools: []outpoststypes.InstanceTypeCapacity{
					{InstanceType: aws.String("m5.4xlarge"), Count: 1},
				},
			})
			require.NoError(t, err, "dry-run StartCapacityTask should succeed")
			assert.Equal(t, outpoststypes.CapacityTaskStatusCompleted, dryOut.CapacityTaskStatus)

			typesOut, err := client.GetOutpostInstanceTypes(
				ctx,
				&outpostssdk.GetOutpostInstanceTypesInput{
					OutpostId: aws.String(outpostID),
				},
			)
			require.NoError(t, err, "GetOutpostInstanceTypes should succeed")
			assert.Len(
				t,
				typesOut.InstanceTypes,
				1,
				"a DryRun task must not mutate the capacity ledger",
			)
		},
	)

	//nolint:paralleltest // sequential by design
	t.Run(
		"cancel_pauses_at_cancellation_in_progress",
		func(t *testing.T) {
			cancelOut, err := client.StartCapacityTask(ctx, &outpostssdk.StartCapacityTaskInput{
				OutpostIdentifier: aws.String(outpostID),
				AssetId:           aws.String(assetID),
				InstancePools: []outpoststypes.InstanceTypeCapacity{
					{InstanceType: aws.String("c5.2xlarge"), Count: 1},
				},
			})
			require.NoError(t, err, "StartCapacityTask should succeed")

			cancelTaskID := aws.ToString(cancelOut.CapacityTaskId)

			_, err = client.CancelCapacityTask(ctx, &outpostssdk.CancelCapacityTaskInput{
				OutpostIdentifier: aws.String(outpostID),
				CapacityTaskId:    aws.String(cancelTaskID),
			})
			require.NoError(t, err, "CancelCapacityTask should succeed")

			immediate, err := client.GetCapacityTask(ctx, &outpostssdk.GetCapacityTaskInput{
				OutpostIdentifier: aws.String(outpostID),
				CapacityTaskId:    aws.String(cancelTaskID),
			})
			require.NoError(t, err, "GetCapacityTask should succeed")
			assert.Equal(
				t,
				outpoststypes.CapacityTaskStatusCancellationInProgress,
				immediate.CapacityTaskStatus,
				"cancellation should pause at the transient state before resolving async",
			)

			awaitCapacityTaskStatus(
				ctx,
				t,
				client,
				outpostID,
				cancelTaskID,
				outpoststypes.CapacityTaskStatusCancelled,
			)
		},
	)
}

// TestIntegration_Outposts_ConnectionLifecycle drives the WireGuard-style
// install-time connection flow.
//
//nolint:paralleltest // sequential by design
func TestIntegration_Outposts_ConnectionLifecycle(t *testing.T) {
	dumpContainerLogsOnFailure(t)

	ctx := t.Context()
	client := createOutpostsClient(t)
	site := createTestSite(ctx, t, client)
	outpost := createTestOutpost(ctx, t, client, aws.ToString(site.SiteId))
	assetID := seededAssetID(ctx, t, client, aws.ToString(outpost.OutpostId))

	clientKey := base64.StdEncoding.EncodeToString(make([]byte, 32))

	startOut, startErr := client.StartConnection(ctx, &outpostssdk.StartConnectionInput{
		AssetId:                     aws.String(assetID),
		ClientPublicKey:             aws.String(clientKey),
		NetworkInterfaceDeviceIndex: 0,
	})
	require.NoError(t, startErr, "StartConnection should succeed")

	connectionID := aws.ToString(startOut.ConnectionId)
	require.NotEmpty(t, connectionID)
	assert.NotEmpty(t, aws.ToString(startOut.UnderlayIpAddress))

	t.Run("get", func(t *testing.T) {
		getOut, err := client.GetConnection(
			ctx,
			&outpostssdk.GetConnectionInput{ConnectionId: aws.String(connectionID)},
		)
		require.NoError(t, err, "GetConnection should succeed")
		assert.Equal(t, connectionID, aws.ToString(getOut.ConnectionId))
		require.NotNil(t, getOut.ConnectionDetails)
		assert.Equal(t, clientKey, aws.ToString(getOut.ConnectionDetails.ClientPublicKey))
		assert.NotEmpty(t, aws.ToString(getOut.ConnectionDetails.ServerPublicKey))
	})
}

// TestIntegration_Outposts_Tagging tables TagResource/ListTagsForResource/
// UntagResource across both resource kinds this service tags -- Outpost and
// Site share one ARN-keyed store (see PARITY.md's tagging note), and this is
// also the exact regression surface the repo-wide /tags/ routing fix
// targeted, so both kinds must independently round-trip through the shared
// resourcegroupstaggingapi-style /tags/{ResourceArn} path.
func TestIntegration_Outposts_Tagging(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	ctx := t.Context()
	client := createOutpostsClient(t)
	site := createTestSite(ctx, t, client)
	outpost := createTestOutpost(ctx, t, client, aws.ToString(site.SiteId))

	tests := []struct {
		name string
		arn  string
	}{
		{name: "outpost", arn: aws.ToString(outpost.OutpostArn)},
		{name: "site", arn: aws.ToString(site.SiteArn)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := client.TagResource(ctx, &outpostssdk.TagResourceInput{
				ResourceArn: aws.String(tt.arn),
				Tags:        map[string]string{"env": "test", "team": "gopherstack"},
			})
			require.NoError(t, err, "TagResource should succeed")

			listOut, err := client.ListTagsForResource(ctx, &outpostssdk.ListTagsForResourceInput{
				ResourceArn: aws.String(tt.arn),
			})
			require.NoError(t, err, "ListTagsForResource should succeed")
			assert.Equal(t, map[string]string{"env": "test", "team": "gopherstack"}, listOut.Tags)

			_, err = client.UntagResource(ctx, &outpostssdk.UntagResourceInput{
				ResourceArn: aws.String(tt.arn),
				TagKeys:     []string{"env"},
			})
			require.NoError(t, err, "UntagResource should succeed")

			afterOut, err := client.ListTagsForResource(ctx, &outpostssdk.ListTagsForResourceInput{
				ResourceArn: aws.String(tt.arn),
			})
			require.NoError(t, err, "ListTagsForResource after untag should succeed")
			assert.Equal(t, map[string]string{"team": "gopherstack"}, afterOut.Tags)
		})
	}
}

// TestIntegration_Outposts_NotFound tables NotFoundException across every
// resource kind's Get, keyed by a syntactically well-formed but nonexistent
// identifier of that resource's confirmed real ID shape.
func TestIntegration_Outposts_NotFound(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	ctx := t.Context()
	client := createOutpostsClient(t)
	site := createTestSite(ctx, t, client)
	outpost := createTestOutpost(ctx, t, client, aws.ToString(site.SiteId))
	outpostID := aws.ToString(outpost.OutpostId)

	tests := []struct {
		call func() error
		name string
	}{
		{name: "outpost", call: func() error {
			_, err := client.GetOutpost(
				ctx,
				&outpostssdk.GetOutpostInput{OutpostId: aws.String("op-00000000000000000")},
			)

			return err
		}},
		{name: "site", call: func() error {
			_, err := client.GetSite(
				ctx,
				&outpostssdk.GetSiteInput{SiteId: aws.String("os-00000000000000000")},
			)

			return err
		}},
		{name: "order", call: func() error {
			_, err := client.GetOrder(
				ctx,
				&outpostssdk.GetOrderInput{OrderId: aws.String("oo-00000000000000000")},
			)

			return err
		}},
		{name: "quote", call: func() error {
			_, err := client.GetQuote(
				ctx,
				&outpostssdk.GetQuoteInput{QuoteIdentifier: aws.String("oq-00000000000000000")},
			)

			return err
		}},
		{name: "order references unknown catalog item", call: func() error {
			_, err := client.CreateOrder(ctx, &outpostssdk.CreateOrderInput{
				OutpostIdentifier: aws.String(outpostID),
				PaymentOption:     outpoststypes.PaymentOptionAllUpfront,
				LineItems: []outpoststypes.LineItemRequest{
					{CatalogItemId: aws.String("OR-0000000"), Quantity: aws.Int32(1)},
				},
			})

			return err
		}},
		// "connection" is deliberately omitted: gopherstack-vpoh -- GET
		// /connections/{id} is currently shadowed by services/iotdataplane's
		// higher-priority RouteMatcher, so this request never reaches
		// outposts' own NotFoundException path. See
		// TestIntegration_Outposts_ConnectionLifecycle/get for the full
		// explanation.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.call()
			require.Error(t, err)
			assert.Equal(t, "NotFoundException", outpostsErrorCode(err))
		})
	}
}

// TestIntegration_Outposts_SemanticValidation tables ValidationException
// cases the SDK's own client-side required-field checks can't intercept --
// each mutates an otherwise-valid input in a way only the server can reject.
//
//nolint:paralleltest // shared Outpost/Site fixtures
func TestIntegration_Outposts_SemanticValidation(t *testing.T) {
	dumpContainerLogsOnFailure(t)

	ctx := t.Context()
	client := createOutpostsClient(t)
	site := createTestSite(ctx, t, client)
	siteID := aws.ToString(site.SiteId)
	outpost := createTestOutpost(ctx, t, client, siteID)
	outpostID := aws.ToString(outpost.OutpostId)

	tests := []struct {
		call func() error
		name string
	}{
		{name: "invalid supported hardware type", call: func() error {
			_, err := client.CreateOutpost(ctx, &outpostssdk.CreateOutpostInput{
				Name:                  aws.String(uniqueOutpostsName(t, "bad-hw")),
				SiteId:                aws.String(siteID),
				SupportedHardwareType: "BOGUS",
			})

			return err
		}},
		{name: "invalid payment option", call: func() error {
			_, err := client.CreateOrder(ctx, &outpostssdk.CreateOrderInput{
				OutpostIdentifier: aws.String(outpostID),
				PaymentOption:     "BOGUS",
			})

			return err
		}},
		{name: "quote country code wrong length", call: func() error {
			_, err := client.CreateQuote(ctx, &outpostssdk.CreateQuoteInput{
				CountryCode: aws.String("USA"),
				RequestedCapacities: []outpoststypes.QuoteCapacity{
					{
						QuoteCapacityType: outpoststypes.QuoteCapacityTypeEc2,
						Quantity:          aws.Float32(1),
						Unit:              aws.String("c5.xlarge"),
					},
				},
			})

			return err
		}},
		{name: "capacity task invalid blocking action", call: func() error {
			assetID := seededAssetID(ctx, t, client, outpostID)
			_, err := client.StartCapacityTask(ctx, &outpostssdk.StartCapacityTaskInput{
				OutpostIdentifier: aws.String(outpostID),
				AssetId:           aws.String(assetID),
				InstancePools: []outpoststypes.InstanceTypeCapacity{
					{InstanceType: aws.String("m5.xlarge"), Count: 1},
				},
				TaskActionOnBlockingInstances: "BOGUS",
			})

			return err
		}},
	}

	for _, tt := range tests { //nolint:paralleltest // shared Outpost/Site fixtures, safe serially
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			require.Error(t, err)
			assert.Equal(t, "ValidationException", outpostsErrorCode(err))
		})
	}
}

// TestIntegration_Outposts_EC2CapacityCoupling drives the full loop this
// pass was built for, end to end, through the REAL EC2 client: create an
// Outpost and configure capacity for one instance type on it, create an
// Outpost-hosted subnet via the real EC2 client, RunInstances onto it and
// observe the Outposts capacity ledger drop (GetOutpostInstanceTypes,
// ListAssetInstances), then TerminateInstances and observe it return. A
// sequential lifecycle, not a permutation table -- see
// gopherstack-tests's guidance that lifecycles stay straight-line.
//
//nolint:paralleltest // sequential by design; shared Outpost/subnet fixtures
func TestIntegration_Outposts_EC2CapacityCoupling(t *testing.T) {
	dumpContainerLogsOnFailure(t)

	ctx := t.Context()
	outpostsClient := createOutpostsClient(t)
	ec2Client := createEC2Client(t)

	const instanceType = "m5.xlarge"

	site := createTestSite(ctx, t, outpostsClient)
	outpost := createTestOutpost(ctx, t, outpostsClient, aws.ToString(site.SiteId))
	outpostID := aws.ToString(outpost.OutpostId)
	outpostARN := aws.ToString(outpost.OutpostArn)
	assetID := seededAssetID(ctx, t, outpostsClient, outpostID)

	startOut, startErr := outpostsClient.StartCapacityTask(ctx, &outpostssdk.StartCapacityTaskInput{
		OutpostIdentifier: aws.String(outpostID),
		AssetId:           aws.String(assetID),
		InstancePools: []outpoststypes.InstanceTypeCapacity{
			{InstanceType: aws.String(instanceType), Count: 1},
		},
	})
	require.NoError(t, startErr, "StartCapacityTask should succeed")

	require.Eventually(t, func() bool {
		out, getErr := outpostsClient.GetCapacityTask(ctx, &outpostssdk.GetCapacityTaskInput{
			OutpostIdentifier: aws.String(outpostID),
			CapacityTaskId:    startOut.CapacityTaskId,
		})

		return getErr == nil && out.CapacityTaskStatus == outpoststypes.CapacityTaskStatusCompleted
	}, 5*time.Second, 50*time.Millisecond, "capacity task should complete before launching instances")

	vpcOut, err := ec2Client.CreateVpc(
		ctx,
		&ec2sdk.CreateVpcInput{CidrBlock: aws.String("10.90.0.0/16")},
	)
	require.NoError(t, err, "CreateVpc should succeed")

	vpcID := aws.ToString(vpcOut.Vpc.VpcId)
	t.Cleanup(func() {
		cctx, cancel := outpostsCleanupCtx()
		defer cancel()
		_, _ = ec2Client.DeleteVpc(cctx, &ec2sdk.DeleteVpcInput{VpcId: aws.String(vpcID)})
	})

	subnetOut, err := ec2Client.CreateSubnet(ctx, &ec2sdk.CreateSubnetInput{
		VpcId:      aws.String(vpcID),
		CidrBlock:  aws.String("10.90.1.0/24"),
		OutpostArn: aws.String(outpostARN),
	})
	require.NoError(t, err, "CreateSubnet with a real OutpostArn should be accepted")
	require.Equal(t, outpostARN, aws.ToString(subnetOut.Subnet.OutpostArn),
		"the created Subnet should echo the real OutpostArn back")

	subnetID := aws.ToString(subnetOut.Subnet.SubnetId)
	t.Cleanup(func() {
		cctx, cancel := outpostsCleanupCtx()
		defer cancel()
		_, _ = ec2Client.DeleteSubnet(
			cctx,
			&ec2sdk.DeleteSubnetInput{SubnetId: aws.String(subnetID)},
		)
	})

	runOut, err := ec2Client.RunInstances(ctx, &ec2sdk.RunInstancesInput{
		ImageId:      aws.String("ami-12345678"),
		InstanceType: ec2types.InstanceType(instanceType),
		SubnetId:     aws.String(subnetID),
		MinCount:     aws.Int32(1),
		MaxCount:     aws.Int32(1),
	})
	require.NoError(
		t,
		err,
		"RunInstances onto an Outpost subnet with available capacity should succeed",
	)
	require.Len(t, runOut.Instances, 1)
	assert.Equal(t, outpostARN, aws.ToString(runOut.Instances[0].OutpostArn),
		"the launched Instance should carry the real OutpostArn")

	instanceID := aws.ToString(runOut.Instances[0].InstanceId)

	t.Run("capacity_drops_after_launch", func(t *testing.T) {
		typesOut, typesErr := outpostsClient.GetOutpostInstanceTypes(
			ctx,
			&outpostssdk.GetOutpostInstanceTypesInput{
				OutpostId: aws.String(outpostID),
			},
		)
		require.NoError(t, typesErr, "GetOutpostInstanceTypes should succeed")
		assert.Empty(t, typesOut.InstanceTypes,
			"the single configured unit of capacity was consumed by RunInstances")

		listOut, listErr := outpostsClient.ListAssetInstances(
			ctx,
			&outpostssdk.ListAssetInstancesInput{
				OutpostIdentifier: aws.String(outpostID),
			},
		)
		require.NoError(t, listErr, "ListAssetInstances should succeed")
		require.Len(t, listOut.AssetInstances, 1)
		assert.Equal(t, instanceID, aws.ToString(listOut.AssetInstances[0].InstanceId))
		assert.Equal(t, instanceType, aws.ToString(listOut.AssetInstances[0].InstanceType))
		assert.Equal(t, assetID, aws.ToString(listOut.AssetInstances[0].AssetId))
		assert.Equal(t, outpoststypes.AWSServiceNameEc2, listOut.AssetInstances[0].AwsServiceName)
	})

	t.Run("second_launch_exceeds_capacity", func(t *testing.T) {
		_, secondErr := ec2Client.RunInstances(ctx, &ec2sdk.RunInstancesInput{
			ImageId:      aws.String("ami-12345678"),
			InstanceType: ec2types.InstanceType(instanceType),
			SubnetId:     aws.String(subnetID),
			MinCount:     aws.Int32(1),
			MaxCount:     aws.Int32(1),
		})
		require.Error(
			t,
			secondErr,
			"a second launch with no remaining configured capacity should be rejected",
		)

		var apiErr smithy.APIError
		require.ErrorAs(t, secondErr, &apiErr)
		assert.Equal(t, "InsufficientInstanceCapacity", apiErr.ErrorCode())
	})

	_, err = ec2Client.TerminateInstances(ctx, &ec2sdk.TerminateInstancesInput{
		InstanceIds: []string{instanceID},
	})
	require.NoError(t, err, "TerminateInstances should succeed")

	t.Run("capacity_returns_after_termination", func(t *testing.T) {
		typesOut, typesErr := outpostsClient.GetOutpostInstanceTypes(
			ctx,
			&outpostssdk.GetOutpostInstanceTypesInput{
				OutpostId: aws.String(outpostID),
			},
		)
		require.NoError(t, typesErr, "GetOutpostInstanceTypes should succeed")
		require.Len(
			t,
			typesOut.InstanceTypes,
			1,
			"terminating the instance should return its capacity",
		)
		assert.Equal(t, instanceType, aws.ToString(typesOut.InstanceTypes[0].InstanceType))

		listOut, listErr := outpostsClient.ListAssetInstances(
			ctx,
			&outpostssdk.ListAssetInstancesInput{
				OutpostIdentifier: aws.String(outpostID),
			},
		)
		require.NoError(t, listErr, "ListAssetInstances should succeed")
		assert.Empty(
			t,
			listOut.AssetInstances,
			"the terminated instance should no longer be listed as running",
		)

		// The freed capacity can be consumed again.
		runAgainOut, runErr := ec2Client.RunInstances(ctx, &ec2sdk.RunInstancesInput{
			ImageId:      aws.String("ami-12345678"),
			InstanceType: ec2types.InstanceType(instanceType),
			SubnetId:     aws.String(subnetID),
			MinCount:     aws.Int32(1),
			MaxCount:     aws.Int32(1),
		})
		require.NoError(t, runErr, "released capacity should be consumable again")
		require.Len(t, runAgainOut.Instances, 1)

		t.Cleanup(func() {
			cctx, cancel := outpostsCleanupCtx()
			defer cancel()
			_, _ = ec2Client.TerminateInstances(cctx, &ec2sdk.TerminateInstancesInput{
				InstanceIds: []string{aws.ToString(runAgainOut.Instances[0].InstanceId)},
			})
		})
	})
}

// TestIntegration_Outposts_EC2CapacityCoupling_NonexistentOutpostArn proves
// the AWS-accurate error for launching onto (here: subnetting onto) an
// Outpost ARN that does not exist, via the real EC2 client. Real AWS
// cross-validates CreateSubnet's OutpostArn against the Outposts control
// plane at subnet-creation time -- this is that check, not RunInstances,
// since a Subnet's OutpostArn is fixed at creation and RunInstances only
// ever inherits it.
func TestIntegration_Outposts_EC2CapacityCoupling_NonexistentOutpostArn(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	ctx := t.Context()
	ec2Client := createEC2Client(t)

	vpcOut, err := ec2Client.CreateVpc(
		ctx,
		&ec2sdk.CreateVpcInput{CidrBlock: aws.String("10.91.0.0/16")},
	)
	require.NoError(t, err, "CreateVpc should succeed")

	vpcID := aws.ToString(vpcOut.Vpc.VpcId)
	t.Cleanup(func() {
		cctx, cancel := outpostsCleanupCtx()
		defer cancel()
		_, _ = ec2Client.DeleteVpc(cctx, &ec2sdk.DeleteVpcInput{VpcId: aws.String(vpcID)})
	})

	_, err = ec2Client.CreateSubnet(ctx, &ec2sdk.CreateSubnetInput{
		VpcId:      aws.String(vpcID),
		CidrBlock:  aws.String("10.91.1.0/24"),
		OutpostArn: aws.String("arn:aws:outposts:us-east-1:000000000000:outpost/op-doesnotexist00"),
	})
	require.Error(t, err, "CreateSubnet referencing an unknown OutpostArn should be rejected")

	var apiErr smithy.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, "InvalidParameterValue", apiErr.ErrorCode())
}

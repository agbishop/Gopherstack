package ce_test

import (
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	costexplorersdk "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/ce"
)

// TestBackfillHistory_RealClient proves ListCostAllocationTagBackfillHistory
// and StartCostAllocationTagBackfill emit BackfillRequest(s) under the real
// PascalCase wire keys. Before the fix, the response body embedded the
// backend's internal BackfillJob model directly, whose JSON tags are
// lowerCamelCase ("backfillFrom", "backfillStatus", ...) -- under this
// service's case-sensitive JSON-RPC 1.1 protocol, a real client's typed
// BackfillFrom/BackfillStatus/RequestedAt were nil/empty on every item,
// regardless of backend state. A raw-body test using the same wrong keys as
// the handler could never have caught this; only decoding through the real
// SDK type can.
func TestBackfillHistory_RealClient(t *testing.T) {
	t.Parallel()

	h := ce.NewHandler(ce.NewInMemoryBackend("000000000000", "us-east-1"))
	client := newTestCEClient(t, h)

	startOut, err := client.StartCostAllocationTagBackfill(
		t.Context(),
		&costexplorersdk.StartCostAllocationTagBackfillInput{
			BackfillFrom: aws.String("2024-01-01T00:00:00Z"),
		},
	)
	require.NoError(t, err)
	require.NotNil(t, startOut.BackfillRequest)
	assert.Equal(t, "2024-01-01T00:00:00Z", aws.ToString(startOut.BackfillRequest.BackfillFrom))
	assert.Equal(t, cetypes.CostAllocationTagBackfillStatusProcessing, startOut.BackfillRequest.BackfillStatus)
	assert.NotEmpty(t, aws.ToString(startOut.BackfillRequest.RequestedAt))

	listOut, err := client.ListCostAllocationTagBackfillHistory(
		t.Context(),
		&costexplorersdk.ListCostAllocationTagBackfillHistoryInput{},
	)
	require.NoError(t, err)
	require.Len(t, listOut.BackfillRequests, 1)
	got := listOut.BackfillRequests[0]
	assert.Equal(t, "2024-01-01T00:00:00Z", aws.ToString(got.BackfillFrom))
	assert.Equal(t, cetypes.CostAllocationTagBackfillStatusProcessing, got.BackfillStatus)
	assert.NotEmpty(t, aws.ToString(got.RequestedAt))
}

// TestCommitmentPurchaseAnalysis_RealClient proves ListCommitmentPurchaseAnalyses
// emits AnalysisSummary items under the real PascalCase wire keys, and that
// StartCommitmentPurchaseAnalysis's required CommitmentPurchaseAnalysisConfiguration
// round-trips instead of being silently discarded. Before the fix: (1) the
// list response embedded the internal CommitmentAnalysis model directly
// (lowerCamelCase tags), so a real client's typed AnalysisId/AnalysisStatus
// were nil/empty on every item; (2) the handler's signature discarded the
// entire request with `_ *startCommitmentPurchaseAnalysisInput`, so the
// required Configuration was never validated, stored, or echoed back on any
// of Start/Get/List.
func TestCommitmentPurchaseAnalysis_RealClient(t *testing.T) {
	t.Parallel()

	h := ce.NewHandler(ce.NewInMemoryBackend("000000000000", "us-east-1"))
	client := newTestCEClient(t, h)

	cfg := &cetypes.CommitmentPurchaseAnalysisConfiguration{
		SavingsPlansPurchaseAnalysisConfiguration: &cetypes.SavingsPlansPurchaseAnalysisConfiguration{
			AnalysisType: cetypes.AnalysisTypeMaxSavings,
			LookBackTimePeriod: &cetypes.DateInterval{
				Start: aws.String("2024-01-01"),
				End:   aws.String("2024-02-01"),
			},
			SavingsPlansToAdd: []cetypes.SavingsPlans{
				{SavingsPlansType: cetypes.SupportedSavingsPlansTypeComputeSp},
			},
		},
	}

	startOut, err := client.StartCommitmentPurchaseAnalysis(
		t.Context(),
		&costexplorersdk.StartCommitmentPurchaseAnalysisInput{
			CommitmentPurchaseAnalysisConfiguration: cfg,
		},
	)
	require.NoError(t, err)
	require.NotEmpty(t, aws.ToString(startOut.AnalysisId))

	getOut, err := client.GetCommitmentPurchaseAnalysis(
		t.Context(),
		&costexplorersdk.GetCommitmentPurchaseAnalysisInput{
			AnalysisId: startOut.AnalysisId,
		},
	)
	require.NoError(t, err)
	assert.Equal(t, aws.ToString(startOut.AnalysisId), aws.ToString(getOut.AnalysisId))
	require.NotNil(t, getOut.CommitmentPurchaseAnalysisConfiguration)
	require.NotNil(t, getOut.CommitmentPurchaseAnalysisConfiguration.SavingsPlansPurchaseAnalysisConfiguration)
	assert.Equal(t,
		cetypes.AnalysisTypeMaxSavings,
		getOut.CommitmentPurchaseAnalysisConfiguration.SavingsPlansPurchaseAnalysisConfiguration.AnalysisType,
	)

	listOut, err := client.ListCommitmentPurchaseAnalyses(
		t.Context(),
		&costexplorersdk.ListCommitmentPurchaseAnalysesInput{},
	)
	require.NoError(t, err)
	require.Len(t, listOut.AnalysisSummaryList, 1)
	item := listOut.AnalysisSummaryList[0]
	assert.Equal(t, aws.ToString(startOut.AnalysisId), aws.ToString(item.AnalysisId))
	assert.Equal(t, cetypes.AnalysisStatusProcessing, item.AnalysisStatus)
	assert.NotEmpty(t, aws.ToString(item.AnalysisStartedTime))
}

// TestStartCommitmentPurchaseAnalysis_MissingConfigurationReturns400 proves
// the handler validates its required CommitmentPurchaseAnalysisConfiguration
// input rather than silently discarding it. A prior revision's handler
// signature was `_ *startCommitmentPurchaseAnalysisInput`, ignoring the
// entire request body, so a request missing this required field succeeded
// with 200 instead of the real API's ValidationException.
func TestStartCommitmentPurchaseAnalysis_MissingConfigurationReturns400(t *testing.T) {
	t.Parallel()

	h := ce.NewHandler(ce.NewInMemoryBackend("000000000000", "us-east-1"))
	rec := doRequest(t, h, "StartCommitmentPurchaseAnalysis", map[string]any{})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestGetCostCategories_NamesVsValues_RealClient proves GetCostCategories
// returns CostCategoryNames (not CostCategoryValues) when the request omits
// CostCategoryName, matching api_op_GetCostCategories.go's documented
// behavior. Before the fix, the handler always populated CostCategoryValues
// regardless of whether CostCategoryName was set, so a real client asking
// "what cost categories exist" (the no-name case) got an empty typed
// CostCategoryNames back every time.
func TestGetCostCategories_NamesVsValues_RealClient(t *testing.T) {
	t.Parallel()

	h := ce.NewHandler(ce.NewInMemoryBackend("000000000000", "us-east-1"))
	client := newTestCEClient(t, h)

	_, err := client.CreateCostCategoryDefinition(t.Context(), &costexplorersdk.CreateCostCategoryDefinitionInput{
		Name:        aws.String("Env"),
		RuleVersion: cetypes.CostCategoryRuleVersionCostCategoryExpressionV1,
		Rules: []cetypes.CostCategoryRule{
			{Value: aws.String("Production")},
		},
	})
	require.NoError(t, err)

	period := &cetypes.DateInterval{Start: aws.String("2024-01-01"), End: aws.String("2024-02-01")}

	byName, err := client.GetCostCategories(t.Context(), &costexplorersdk.GetCostCategoriesInput{
		TimePeriod:       period,
		CostCategoryName: aws.String("Env"),
	})
	require.NoError(t, err)
	assert.Empty(t, byName.CostCategoryNames)
	assert.Contains(t, byName.CostCategoryValues, "Production")

	noName, err := client.GetCostCategories(t.Context(), &costexplorersdk.GetCostCategoriesInput{
		TimePeriod: period,
	})
	require.NoError(t, err)
	assert.Empty(t, noName.CostCategoryValues)
	assert.Contains(t, noName.CostCategoryNames, "Env")
}

// TestCreateCostCategoryDefinition_SplitChargeRulesAndEffectiveStart_RealClient
// covers gopherstack-4shm's own class on CreateCostCategoryDefinitionInput
// (real fields: api_op_CreateCostCategoryDefinition.go): SplitChargeRules
// and EffectiveStart were both parsed off the wire (SplitChargeRules typed
// even on this package's own wire struct) and then completely discarded --
// handleCreateCostCategoryDefinition never passed either to the backend, so
// a real client's split-charge configuration silently vanished, and a
// caller-supplied EffectiveStart was always overridden with "now" instead
// of honored. UpdateCostCategoryDefinition already threaded
// SplitChargeRules correctly; Create did not.
func TestCreateCostCategoryDefinition_SplitChargeRulesAndEffectiveStart_RealClient(t *testing.T) {
	t.Parallel()

	h := ce.NewHandler(ce.NewInMemoryBackend("000000000000", "us-east-1"))
	client := newTestCEClient(t, h)

	createOut, err := client.CreateCostCategoryDefinition(
		t.Context(),
		&costexplorersdk.CreateCostCategoryDefinitionInput{
			Name:           aws.String("Splitter"),
			RuleVersion:    cetypes.CostCategoryRuleVersionCostCategoryExpressionV1,
			Rules:          []cetypes.CostCategoryRule{{Value: aws.String("Shared")}},
			EffectiveStart: aws.String("2023-06-01T00:00:00Z"),
			SplitChargeRules: []cetypes.CostCategorySplitChargeRule{
				{
					Source:  aws.String("Shared"),
					Method:  cetypes.CostCategorySplitChargeMethodProportional,
					Targets: []string{"Engineering", "Sales"},
				},
			},
		},
	)
	require.NoError(t, err)
	assert.Equal(
		t, "2023-06-01T00:00:00Z", aws.ToString(createOut.EffectiveStart),
		"a caller-supplied EffectiveStart must be honored, not silently overridden with now",
	)

	describeOut, err := client.DescribeCostCategoryDefinition(
		t.Context(),
		&costexplorersdk.DescribeCostCategoryDefinitionInput{CostCategoryArn: createOut.CostCategoryArn},
	)
	require.NoError(t, err)
	require.NotNil(t, describeOut.CostCategory)
	require.Len(
		t, describeOut.CostCategory.SplitChargeRules, 1,
		"SplitChargeRules must round-trip, not be silently dropped on create",
	)
	got := describeOut.CostCategory.SplitChargeRules[0]
	assert.Equal(t, "Shared", aws.ToString(got.Source))
	assert.Equal(t, cetypes.CostCategorySplitChargeMethodProportional, got.Method)
	assert.Equal(t, []string{"Engineering", "Sales"}, got.Targets)
}

// TestUpdateCostCategoryDefinition_EffectiveStart_RealClient proves
// UpdateCostCategoryDefinitionInput.EffectiveStart (api_op_UpdateCostCategoryDefinition.go:
// "the cost category's effective start date ... If the date isn't provided, it's the
// first day of the current month", same optional-with-default field Create has) is
// honored, not silently dropped. Before this fix, updateCostCategoryDefinitionInput had
// no EffectiveStart field at all -- the wire key was never even unmarshalled -- and the
// backend unconditionally overwrote EffectiveStart with "now" on every update.
func TestUpdateCostCategoryDefinition_EffectiveStart_RealClient(t *testing.T) {
	t.Parallel()

	h := ce.NewHandler(ce.NewInMemoryBackend("000000000000", "us-east-1"))
	client := newTestCEClient(t, h)

	createOut, err := client.CreateCostCategoryDefinition(
		t.Context(),
		&costexplorersdk.CreateCostCategoryDefinitionInput{
			Name:        aws.String("Splitter"),
			RuleVersion: cetypes.CostCategoryRuleVersionCostCategoryExpressionV1,
			Rules:       []cetypes.CostCategoryRule{{Value: aws.String("Shared")}},
		},
	)
	require.NoError(t, err)

	updateOut, err := client.UpdateCostCategoryDefinition(
		t.Context(),
		&costexplorersdk.UpdateCostCategoryDefinitionInput{
			CostCategoryArn: createOut.CostCategoryArn,
			RuleVersion:     cetypes.CostCategoryRuleVersionCostCategoryExpressionV1,
			Rules:           []cetypes.CostCategoryRule{{Value: aws.String("Shared")}},
			EffectiveStart:  aws.String("2023-06-01T00:00:00Z"),
		},
	)
	require.NoError(t, err)
	assert.Equal(
		t, "2023-06-01T00:00:00Z", aws.ToString(updateOut.EffectiveStart),
		"a caller-supplied EffectiveStart must be honored on update, not silently overridden with now",
	)

	describeOut, err := client.DescribeCostCategoryDefinition(
		t.Context(),
		&costexplorersdk.DescribeCostCategoryDefinitionInput{CostCategoryArn: createOut.CostCategoryArn},
	)
	require.NoError(t, err)
	require.NotNil(t, describeOut.CostCategory)
	assert.Equal(t, "2023-06-01T00:00:00Z", aws.ToString(describeOut.CostCategory.EffectiveStart))
}

// TestGetRightsizingRecommendation_Configuration_RealClient proves
// GetRightsizingRecommendationOutput always echoes Configuration (with
// AWS-documented server-applied defaults when the request omits it). Before
// the fix, the field was absent from the response entirely, so a real
// client's typed .Configuration was always nil.
func TestGetRightsizingRecommendation_Configuration_RealClient(t *testing.T) {
	t.Parallel()

	h := ce.NewHandler(ce.NewInMemoryBackend("000000000000", "us-east-1"))
	client := newTestCEClient(t, h)

	out, err := client.GetRightsizingRecommendation(t.Context(), &costexplorersdk.GetRightsizingRecommendationInput{
		Service: aws.String("AmazonEC2"),
	})
	require.NoError(t, err)
	require.NotNil(t, out.Configuration)
	assert.True(t, out.Configuration.BenefitsConsidered)
	assert.Equal(t, cetypes.RecommendationTarget("SAME_INSTANCE_FAMILY"), out.Configuration.RecommendationTarget)

	out2, err := client.GetRightsizingRecommendation(t.Context(), &costexplorersdk.GetRightsizingRecommendationInput{
		Service: aws.String("AmazonEC2"),
		Configuration: &cetypes.RightsizingRecommendationConfiguration{
			BenefitsConsidered:   false,
			RecommendationTarget: cetypes.RecommendationTarget("CROSS_INSTANCE_FAMILY"),
		},
	})
	require.NoError(t, err)
	require.NotNil(t, out2.Configuration)
	assert.False(t, out2.Configuration.BenefitsConsidered)
	assert.Equal(t, cetypes.RecommendationTarget("CROSS_INSTANCE_FAMILY"), out2.Configuration.RecommendationTarget)
}

// TestGetReservationPurchaseRecommendation_NoFabricatedMetadataKeys_RealClient
// covers gopherstack-y1zn. handleGetReservationPurchaseRecommendation emitted
// a Metadata map with "RecommendationTotalCount" and "USD" (the latter was a
// stray use of handlerCurrencyCode's own value as the map key, instead of
// "CurrencyCode"); types.ReservationPurchaseRecommendationMetadata
// (costexplorer@v1.67.4 types/types.go) has neither -- only
// AdditionalMetadata/GenerationTimestamp/RecommendationId. A typed client
// silently ignores unknown keys, so the proof is the raw body.
func TestGetReservationPurchaseRecommendation_NoFabricatedMetadataKeys_RealClient(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "GetReservationPurchaseRecommendation", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	body := rec.Body.String()
	assert.NotContains(t, body, `"RecommendationTotalCount"`,
		"types.ReservationPurchaseRecommendationMetadata has no RecommendationTotalCount member")
	assert.NotContains(t, body, `"USD":"USD"`,
		"the stray map key must not be the currency value itself")
	assert.NotContains(t, body, `"Metadata"`,
		"no real Metadata field is trackable, so the key should be omitted entirely")
}

// TestGetSavingsPlansPurchaseRecommendation_CurrencyCodeKey_RealClient covers
// gopherstack-y1zn. The recommendation detail and summary maps used
// handlerCurrencyCode's own value ("USD") as the map key instead of
// "CurrencyCode" (costexplorer@v1.67.4 deserializers.go); separately, the
// top-level Metadata map included a fabricated "RecommendationTotalCount" key
// (types.SavingsPlansPurchaseRecommendationMetadata has no such member).
func TestGetSavingsPlansPurchaseRecommendation_CurrencyCodeKey_RealClient(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "GetSavingsPlansPurchaseRecommendation", map[string]any{
		"SavingsPlansType":     "COMPUTE_SP",
		"TermInYears":          "ONE_YEAR",
		"PaymentOption":        "NO_UPFRONT",
		"LookbackPeriodInDays": "THIRTY_DAYS",
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	body := rec.Body.String()
	assert.NotContains(t, body, `"USD":"USD"`,
		"the map key must be CurrencyCode, not the currency value itself")
	assert.Contains(t, body, `"CurrencyCode":"USD"`,
		"the real member is CurrencyCode")
	assert.NotContains(t, body, `"RecommendationTotalCount"`,
		"types.SavingsPlansPurchaseRecommendationMetadata has no RecommendationTotalCount member")
}

// TestCreateAnomalyMonitor_MonitorSpecification_RealClient covers a
// write-only-state bug found by the primary-method sweep: real
// CreateAnomalyMonitorInput.AnomalyMonitor carries a MonitorSpecification
// *types.Expression member (required for a CUSTOM monitor, or a DIMENSIONAL
// monitor whose MonitorDimension is TAG/COST_CATEGORY -- see
// costexplorer@v1.67.4 types/types.go's AnomalyMonitor doc comment, and its
// serializer/deserializer at serializers.go:2953/deserializers.go:6476).
// This field was previously entirely absent from this package's wire
// structs and internal model: a real client's MonitorSpecification was
// accepted by nothing, stored nowhere, and every GetAnomalyMonitors
// response omitted it regardless of what was sent on Create.
func TestCreateAnomalyMonitor_MonitorSpecification_RealClient(t *testing.T) {
	t.Parallel()

	h := ce.NewHandler(ce.NewInMemoryBackend("000000000000", "us-east-1"))
	client := newTestCEClient(t, h)

	createOut, err := client.CreateAnomalyMonitor(t.Context(), &costexplorersdk.CreateAnomalyMonitorInput{
		AnomalyMonitor: &cetypes.AnomalyMonitor{
			MonitorName: aws.String("CustomTagMonitor"),
			MonitorType: cetypes.MonitorTypeCustom,
			MonitorSpecification: &cetypes.Expression{
				Tags: &cetypes.TagValues{
					Key:    aws.String("team"),
					Values: []string{"prod"},
				},
			},
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, aws.ToString(createOut.MonitorArn))

	getOut, err := client.GetAnomalyMonitors(t.Context(), &costexplorersdk.GetAnomalyMonitorsInput{
		MonitorArnList: []string{aws.ToString(createOut.MonitorArn)},
	})
	require.NoError(t, err)
	require.Len(t, getOut.AnomalyMonitors, 1)

	got := getOut.AnomalyMonitors[0]
	require.NotNil(t, got.MonitorSpecification,
		"MonitorSpecification must round-trip through Create->Get, not be silently dropped")
	require.NotNil(t, got.MonitorSpecification.Tags)
	assert.Equal(t, "team", aws.ToString(got.MonitorSpecification.Tags.Key))
	assert.Equal(t, []string{"prod"}, got.MonitorSpecification.Tags.Values)
}

// TestAnomalySubscription_ThresholdExpression_RealClient covers the sibling
// write-only-state bug in the same family: real AnomalySubscription/
// CreateAnomalySubscriptionInput/UpdateAnomalySubscriptionInput all carry a
// ThresholdExpression *types.Expression member, the non-deprecated
// replacement for Threshold ("you can specify either Threshold or
// ThresholdExpression, but not both" -- costexplorer@v1.67.4
// types/types.go). It was entirely absent from this package's wire structs
// and internal model, so a real client using only ThresholdExpression (the
// documented modern path) had it silently dropped on Create, missing on
// every Get, and any Update value discarded too.
func TestAnomalySubscription_ThresholdExpression_RealClient(t *testing.T) {
	t.Parallel()

	h := ce.NewHandler(ce.NewInMemoryBackend("000000000000", "us-east-1"))
	client := newTestCEClient(t, h)

	monOut, err := client.CreateAnomalyMonitor(t.Context(), &costexplorersdk.CreateAnomalyMonitorInput{
		AnomalyMonitor: &cetypes.AnomalyMonitor{
			MonitorName:      aws.String("Mon"),
			MonitorType:      cetypes.MonitorTypeDimensional,
			MonitorDimension: cetypes.MonitorDimensionService,
		},
	})
	require.NoError(t, err)

	thresholdExpr := &cetypes.Expression{
		Dimensions: &cetypes.DimensionValues{
			Key:    cetypes.DimensionAnomalyTotalImpactAbsolute,
			Values: []string{"100"},
		},
	}

	createOut, err := client.CreateAnomalySubscription(t.Context(), &costexplorersdk.CreateAnomalySubscriptionInput{
		AnomalySubscription: &cetypes.AnomalySubscription{
			SubscriptionName: aws.String("Sub"),
			Frequency:        cetypes.AnomalySubscriptionFrequencyDaily,
			MonitorArnList:   []string{aws.ToString(monOut.MonitorArn)},
			Subscribers: []cetypes.Subscriber{
				{Address: aws.String("a@example.com"), Type: cetypes.SubscriberTypeEmail},
			},
			ThresholdExpression: thresholdExpr,
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, aws.ToString(createOut.SubscriptionArn))

	getOut, err := client.GetAnomalySubscriptions(t.Context(), &costexplorersdk.GetAnomalySubscriptionsInput{
		SubscriptionArnList: []string{aws.ToString(createOut.SubscriptionArn)},
	})
	require.NoError(t, err)
	require.Len(t, getOut.AnomalySubscriptions, 1)

	got := getOut.AnomalySubscriptions[0]
	require.NotNil(t, got.ThresholdExpression,
		"ThresholdExpression must round-trip through Create->Get, not be silently dropped")
	require.NotNil(t, got.ThresholdExpression.Dimensions)
	assert.Equal(t, cetypes.DimensionAnomalyTotalImpactAbsolute, got.ThresholdExpression.Dimensions.Key)
	assert.Equal(t, []string{"100"}, got.ThresholdExpression.Dimensions.Values)

	// Update with a new ThresholdExpression must also round-trip, not be discarded.
	newExpr := &cetypes.Expression{
		Dimensions: &cetypes.DimensionValues{
			Key:    cetypes.DimensionAnomalyTotalImpactPercentage,
			Values: []string{"50"},
		},
	}
	_, err = client.UpdateAnomalySubscription(t.Context(), &costexplorersdk.UpdateAnomalySubscriptionInput{
		SubscriptionArn:     createOut.SubscriptionArn,
		ThresholdExpression: newExpr,
	})
	require.NoError(t, err)

	getOut2, err := client.GetAnomalySubscriptions(t.Context(), &costexplorersdk.GetAnomalySubscriptionsInput{
		SubscriptionArnList: []string{aws.ToString(createOut.SubscriptionArn)},
	})
	require.NoError(t, err)
	require.Len(t, getOut2.AnomalySubscriptions, 1)
	got2 := getOut2.AnomalySubscriptions[0]
	require.NotNil(t, got2.ThresholdExpression)
	assert.Equal(t, cetypes.DimensionAnomalyTotalImpactPercentage, got2.ThresholdExpression.Dimensions.Key)
	assert.Equal(t, []string{"50"}, got2.ThresholdExpression.Dimensions.Values)
}

// TestGetAnomalyMonitors_DimensionalValueCount_RealClient covers a
// write-only-state-style sibling bug found by sweeping AnomalyMonitor's
// other real members alongside the MonitorSpecification fix above: real
// types.AnomalyMonitor.DimensionalValueCount ("the value for evaluated
// dimensions" -- costexplorer@v1.67.4 types/types.go) was entirely absent
// from this package's wire struct and never computed, so a real client's
// typed DimensionalValueCount was always the zero value regardless of
// backend state. For a DIMENSIONAL monitor on the SERVICE or LINKED_ACCOUNT
// dimension this emulator has a real, non-fabricated source to derive it
// from: the count of that dimension's distinct values in the synthetic cost
// ledger (the same data GetDimensionValues already reads,
// syntheticServiceCatalog seeding 12 distinct SERVICE values).
func TestGetAnomalyMonitors_DimensionalValueCount_RealClient(t *testing.T) {
	t.Parallel()

	h := ce.NewHandler(ce.NewInMemoryBackend("000000000000", "us-east-1"))
	client := newTestCEClient(t, h)

	createOut, err := client.CreateAnomalyMonitor(t.Context(), &costexplorersdk.CreateAnomalyMonitorInput{
		AnomalyMonitor: &cetypes.AnomalyMonitor{
			MonitorName:      aws.String("ServiceMonitor"),
			MonitorType:      cetypes.MonitorTypeDimensional,
			MonitorDimension: cetypes.MonitorDimensionService,
		},
	})
	require.NoError(t, err)

	getOut, err := client.GetAnomalyMonitors(t.Context(), &costexplorersdk.GetAnomalyMonitorsInput{
		MonitorArnList: []string{aws.ToString(createOut.MonitorArn)},
	})
	require.NoError(t, err)
	require.Len(t, getOut.AnomalyMonitors, 1)
	assert.EqualValues(
		t,
		12,
		getOut.AnomalyMonitors[0].DimensionalValueCount,
		"DimensionalValueCount must reflect the real distinct-SERVICE-value count, not be silently dropped",
	)
}

// TestGetAnomalies_TotalImpactFilter_RealClient covers gopherstack-4shm's own
// class: GetAnomaliesInput.TotalImpact (a real
// types.TotalImpactFilter{NumericOperator, StartValue, EndValue} --
// costexplorer@v1.67.4 api_op_GetAnomalies.go / types/types.go) was
// previously typed as a bare map[string]any on this package's wire struct
// and never read anywhere in handleGetAnomalies -- parsed off the wire,
// then silently discarded, so GetAnomalies GREATER_THAN/BETWEEN dollar-
// impact filtering never narrowed the result set regardless of what a real
// client sent.
func TestGetAnomalies_TotalImpactFilter_RealClient(t *testing.T) {
	t.Parallel()

	h := ce.NewHandler(ce.NewInMemoryBackend("000000000000", "us-east-1"))
	client := newTestCEClient(t, h)

	h.Backend.AddAnomaly(ce.Anomaly{
		AnomalyID:        "low-impact",
		MonitorARN:       "arn:aws:ce::000000000000:anomalymonitor/test",
		AnomalyStartDate: "2024-01-01",
		AnomalyEndDate:   "2024-01-02",
		TotalImpact:      50,
	})
	h.Backend.AddAnomaly(ce.Anomaly{
		AnomalyID:        "high-impact",
		MonitorARN:       "arn:aws:ce::000000000000:anomalymonitor/test",
		AnomalyStartDate: "2024-01-01",
		AnomalyEndDate:   "2024-01-02",
		TotalImpact:      500,
	})

	out, err := client.GetAnomalies(t.Context(), &costexplorersdk.GetAnomaliesInput{
		DateInterval: &cetypes.AnomalyDateInterval{StartDate: aws.String("2024-01-01")},
		TotalImpact: &cetypes.TotalImpactFilter{
			NumericOperator: cetypes.NumericOperatorGreaterThan,
			StartValue:      100,
		},
	})
	require.NoError(t, err)
	require.Len(t, out.Anomalies, 1, "TotalImpact GREATER_THAN 100 must exclude the 50-impact anomaly")
	assert.Equal(t, "high-impact", aws.ToString(out.Anomalies[0].AnomalyId))
	assert.InDelta(t, 500, out.Anomalies[0].Impact.TotalImpact, 0)

	betweenOut, err := client.GetAnomalies(t.Context(), &costexplorersdk.GetAnomaliesInput{
		DateInterval: &cetypes.AnomalyDateInterval{StartDate: aws.String("2024-01-01")},
		TotalImpact: &cetypes.TotalImpactFilter{
			NumericOperator: cetypes.NumericOperatorBetween,
			StartValue:      0,
			EndValue:        100,
		},
	})
	require.NoError(t, err)
	require.Len(t, betweenOut.Anomalies, 1, "TotalImpact BETWEEN 0 and 100 must exclude the 500-impact anomaly")
	assert.Equal(t, "low-impact", aws.ToString(betweenOut.Anomalies[0].AnomalyId))
}

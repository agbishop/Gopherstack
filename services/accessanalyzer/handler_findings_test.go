package accessanalyzer_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	aasdk "github.com/aws/aws-sdk-go-v2/service/accessanalyzer"
	aatypes "github.com/aws/aws-sdk-go-v2/service/accessanalyzer/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/accessanalyzer"
)

// TestGetFinding verifies GET /finding/{id}?analyzerArn=... -- the real wire
// route (top-level /finding, analyzer carried as an ARN query param), NOT
// /analyzer/{name}/finding/{id}. Exercised through h.Handler() (not the
// backend directly) so both the RouteMatcher and the path parser are proven
// to accept the path a real SDK client sends.
func TestGetFinding(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		wantStatus int
		badArn     bool
		missingID  bool
	}{
		{name: "existing_finding", wantStatus: http.StatusOK},
		{name: "wrong_analyzer_arn", wantStatus: http.StatusNotFound, badArn: true},
		{name: "missing_finding_id", wantStatus: http.StatusNotFound, missingID: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := accessanalyzer.NewInMemoryBackend("000000000000", "us-east-1")
			h := accessanalyzer.NewHandler(b)

			analyzerArn := mustAnalyzer(t, b, "get-finding-analyzer")
			findingID := mustFinding(t, b, "get-finding-analyzer")

			if tt.badArn {
				analyzerArn = "arn:aws:access-analyzer:us-east-1:000000000000:analyzer/missing"
			}

			if tt.missingID {
				findingID = "no-such-id"
			}

			rec := doRequest(t, h, http.MethodGet, "/finding/"+findingID+"?analyzerArn="+analyzerArn, nil)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus != http.StatusOK {
				return
			}

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

			finding, ok := resp["finding"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, findingID, finding["id"])
			// Real GetFinding wire shape serializes the resource under
			// "resource", not "resourceArn" (that key is only correct for
			// AnalyzedResource); resourceOwnerAccount and analyzedAt are
			// required fields.
			assert.Equal(t, "arn:aws:s3:::bucket", finding["resource"])
			assert.Equal(t, "000000000000", finding["resourceOwnerAccount"])
			assert.NotEmpty(t, finding["analyzedAt"])

			_, hasWrongKey := finding["resourceArn"]
			assert.False(t, hasWrongKey, `wire shape must use "resource", not "resourceArn"`)

			// "condition" is a required Finding member -- it must always be
			// present (as {}, since mustFinding creates a finding with no
			// condition), never omitted.
			condition, hasCondition := finding["condition"]
			assert.True(t, hasCondition, `"condition" is required and must be present even when empty`)
			assert.Equal(t, map[string]any{}, condition)
		})
	}
}

// TestListFindings verifies POST /finding -- the real wire route (top-level
// /finding, analyzer carried as an ARN in the JSON body), NOT
// /analyzer/{name}/findings.
func TestListFindings(t *testing.T) {
	t.Parallel()

	b := accessanalyzer.NewInMemoryBackend("000000000000", "us-east-1")
	h := accessanalyzer.NewHandler(b)

	analyzerArn := mustAnalyzer(t, b, "list-findings-analyzer")
	mustFinding(t, b, "list-findings-analyzer")
	mustFinding(t, b, "list-findings-analyzer")

	t.Run("lists_findings_for_analyzer", func(t *testing.T) {
		t.Parallel()

		rec := doRequest(t, h, http.MethodPost, "/finding", map[string]any{"analyzerArn": analyzerArn})
		require.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

		findings, ok := resp["findings"].([]any)
		require.True(t, ok)
		require.Len(t, findings, 2)

		first, ok := findings[0].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "arn:aws:s3:::bucket", first["resource"])
		assert.Equal(t, "000000000000", first["resourceOwnerAccount"])
	})

	t.Run("missing_analyzer_arn_is_validation_error", func(t *testing.T) {
		t.Parallel()

		rec := doRequest(t, h, http.MethodPost, "/finding", map[string]any{})
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

// TestUpdateFindings verifies PUT /finding -- the real wire route (top-level
// /finding, analyzer carried as an ARN in the JSON body), NOT
// /analyzer/{name}/findings.
func TestUpdateFindings(t *testing.T) {
	t.Parallel()

	b := accessanalyzer.NewInMemoryBackend("000000000000", "us-east-1")
	h := accessanalyzer.NewHandler(b)

	analyzerArn := mustAnalyzer(t, b, "update-findings-analyzer")
	findingID := mustFinding(t, b, "update-findings-analyzer")

	t.Run("archives_via_put_finding", func(t *testing.T) {
		t.Parallel()

		rec := doRequest(t, h, http.MethodPut, "/finding", map[string]any{
			"analyzerArn": analyzerArn,
			"ids":         []string{findingID},
			"status":      "ARCHIVED",
		})
		require.Equal(t, http.StatusOK, rec.Code)

		got, err := b.GetFinding("update-findings-analyzer", findingID)
		require.NoError(t, err)
		assert.Equal(t, accessanalyzer.FindingStatusArchived, got.Status)
	})

	t.Run("missing_analyzer_arn_is_validation_error", func(t *testing.T) {
		t.Parallel()

		rec := doRequest(t, h, http.MethodPut, "/finding", map[string]any{
			"ids":    []string{findingID},
			"status": "ARCHIVED",
		})
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

// TestUpdateFindings_ByResourceArn verifies UpdateFindingsInput's resourceArn
// (a real, independently optional wire field -- serializers.go:3312-3315 in
// aws-sdk-go-v2/service/accessanalyzer@v1.51.4, guarded by its own `!= nil`
// check, distinct from ids) selects findings when ids is omitted. The
// handler previously never decoded "resourceArn" from the request body at
// all, so a client that identified findings by resourceArn alone silently
// updated nothing.
func TestUpdateFindings_ByResourceArn(t *testing.T) {
	t.Parallel()

	b := accessanalyzer.NewInMemoryBackend("000000000000", "us-east-1")
	h := accessanalyzer.NewHandler(b)

	analyzerArn := mustAnalyzer(t, b, "update-findings-by-resource-analyzer")

	target, err := b.AddFinding("update-findings-by-resource-analyzer",
		"AWS::S3::Bucket", "arn:aws:s3:::target-bucket", nil, nil, nil)
	require.NoError(t, err)

	other, err := b.AddFinding("update-findings-by-resource-analyzer",
		"AWS::S3::Bucket", "arn:aws:s3:::other-bucket", nil, nil, nil)
	require.NoError(t, err)

	rec := doRequest(t, h, http.MethodPut, "/finding", map[string]any{
		"analyzerArn": analyzerArn,
		"resourceArn": "arn:aws:s3:::target-bucket",
		"status":      "ARCHIVED",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	got, err := b.GetFinding("update-findings-by-resource-analyzer", target.ID)
	require.NoError(t, err)
	assert.Equal(t, accessanalyzer.FindingStatusArchived, got.Status,
		"resourceArn-only UpdateFindings must archive the matching finding")

	untouched, err := b.GetFinding("update-findings-by-resource-analyzer", other.ID)
	require.NoError(t, err)
	assert.Equal(t, accessanalyzer.FindingStatusActive, untouched.Status,
		"a finding for a different resource must be left alone")
}

// TestFindingV2Lifecycle verifies GetFindingV2 and ListFindingsV2.
func TestFindingV2Lifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		fn   func(t *testing.T, b *accessanalyzer.InMemoryBackend, h *accessanalyzer.Handler)
		name string
	}{
		{
			name: "get_finding_v2",
			fn: func(t *testing.T, b *accessanalyzer.InMemoryBackend, h *accessanalyzer.Handler) {
				t.Helper()
				arn := mustAnalyzer(t, b, "fv2-get-analyzer")
				findingID := mustFinding(t, b, "fv2-get-analyzer")

				rec := doRequest(t, h, http.MethodGet, "/findingv2/"+findingID+"?analyzerArn="+arn, nil)
				assert.Equal(t, http.StatusOK, rec.Code)

				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Equal(t, findingID, resp["id"])
				// findingType and the ExternalAccessDetails union member of
				// findingDetails are required GetFindingV2Output members.
				assert.Equal(t, "ExternalAccess", resp["findingType"])
				details, ok := resp["findingDetails"].([]any)
				require.True(t, ok)
				require.Len(t, details, 1)
				member, ok := details[0].(map[string]any)
				require.True(t, ok)
				external, ok := member["externalAccessDetails"].(map[string]any)
				require.True(t, ok)
				_, hasCondition := external["condition"]
				assert.True(t, hasCondition, `"condition" is required on ExternalAccessDetails`)
			},
		},
		{
			name: "list_findings_v2",
			fn: func(t *testing.T, b *accessanalyzer.InMemoryBackend, h *accessanalyzer.Handler) {
				t.Helper()
				arn := mustAnalyzer(t, b, "fv2-list-analyzer")
				mustFinding(t, b, "fv2-list-analyzer")
				mustFinding(t, b, "fv2-list-analyzer")

				rec := doRequest(t, h, http.MethodPost, "/findingv2", map[string]any{
					"analyzerArn": arn,
				})
				assert.Equal(t, http.StatusOK, rec.Code)

				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				findings := resp["findings"].([]any)
				assert.Len(t, findings, 2)
				first, ok := findings[0].(map[string]any)
				require.True(t, ok)
				// findingType is a required FindingSummaryV2 member.
				assert.Equal(t, "ExternalAccess", first["findingType"])
			},
		},
		{
			name: "get_missing_finding_v2",
			fn: func(t *testing.T, b *accessanalyzer.InMemoryBackend, h *accessanalyzer.Handler) {
				t.Helper()
				arn := mustAnalyzer(t, b, "fv2-miss-analyzer")

				rec := doRequest(t, h, http.MethodGet, "/findingv2/no-such-id?analyzerArn="+arn, nil)
				assert.Equal(t, http.StatusNotFound, rec.Code)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := accessanalyzer.NewInMemoryBackend("000000000000", "us-east-1")
			h := accessanalyzer.NewHandler(b)
			tt.fn(t, b, h)
		})
	}
}

// TestGetFindingsStatistics verifies POST /analyzer/findings/statistics.
func TestGetFindingsStatistics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setupFn    func(b *accessanalyzer.InMemoryBackend) string
		name       string
		wantStatus int
		wantActive int
	}{
		{
			name: "returns_counts",
			setupFn: func(b *accessanalyzer.InMemoryBackend) string {
				arn := mustAnalyzer(t, b, "stats-analyzer")
				mustFinding(t, b, "stats-analyzer")
				mustFinding(t, b, "stats-analyzer")

				return arn
			},
			wantStatus: http.StatusOK,
			wantActive: 2,
		},
		{
			name: "missing_analyzer",
			setupFn: func(_ *accessanalyzer.InMemoryBackend) string {
				return "arn:aws:access-analyzer:us-east-1:000000000000:analyzer/missing"
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := accessanalyzer.NewInMemoryBackend("000000000000", "us-east-1")
			h := accessanalyzer.NewHandler(b)
			arn := tt.setupFn(b)

			rec := doRequest(t, h, http.MethodPost, "/analyzer/findings/statistics", map[string]any{
				"analyzerArn": arn,
			})
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK && tt.wantActive > 0 {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				stats := resp["findingsStatistics"].([]any)
				require.Len(t, stats, 1)

				extStats := stats[0].(map[string]any)["externalAccessFindingsStatistics"].(map[string]any)
				assert.InDelta(t, float64(tt.wantActive), extStats["totalActiveFindings"], 0.0001)
			}
		})
	}
}

// TestFindingRecommendationLifecycle verifies Generate/Get finding recommendation.
func TestFindingRecommendationLifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		fn   func(t *testing.T, b *accessanalyzer.InMemoryBackend, h *accessanalyzer.Handler)
		name string
	}{
		{
			name: "generate_and_get",
			fn: func(t *testing.T, b *accessanalyzer.InMemoryBackend, h *accessanalyzer.Handler) {
				t.Helper()
				arn := mustAnalyzer(t, b, "rec-analyzer")
				findingID := mustFinding(t, b, "rec-analyzer")

				// AnalyzerArn is a query parameter, not a body member -- the real
				// SDK sends no body for this operation
				// (awsRestjson1_serializeOpHttpBindingsGenerateFindingRecommendationInput,
				// aws-sdk-go-v2/service/accessanalyzer@v1.51.4 serializers.go).
				rec := doRequest(t, h, http.MethodPost, "/recommendation/"+findingID+"?analyzerArn="+arn, nil)
				assert.Equal(t, http.StatusOK, rec.Code)

				rec2 := doRequest(t, h, http.MethodGet, "/recommendation/"+findingID+"?analyzerArn="+arn, nil)
				assert.Equal(t, http.StatusOK, rec2.Code)

				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &resp))
				assert.NotEmpty(t, resp["recommendationType"])
			},
		},
		{
			name: "get_missing_recommendation",
			fn: func(t *testing.T, _ *accessanalyzer.InMemoryBackend, h *accessanalyzer.Handler) {
				t.Helper()
				rec := doRequest(t, h, http.MethodGet, "/recommendation/no-such-id?analyzerArn=arn", nil)
				assert.Equal(t, http.StatusNotFound, rec.Code)
			},
		},
		{
			// Previously GenerateFindingRecommendation created a recommendation
			// record for any finding ID unconditionally, without checking it
			// exists. It must reject a bogus ID -- as ValidationException/400,
			// not ResourceNotFoundException/404: GenerateFindingRecommendation's
			// own deserializeOpError switch (aws-sdk-go-v2/service/
			// accessanalyzer@v1.51.4 deserializers.go) does not type
			// ResourceNotFoundException.
			name: "generate_for_missing_finding_is_not_found",
			fn: func(t *testing.T, b *accessanalyzer.InMemoryBackend, h *accessanalyzer.Handler) {
				t.Helper()
				arn := mustAnalyzer(t, b, "rec-missing-finding-analyzer")

				rec := doRequest(t, h, http.MethodPost, "/recommendation/no-such-finding?analyzerArn="+arn, nil)
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := accessanalyzer.NewInMemoryBackend("000000000000", "us-east-1")
			h := accessanalyzer.NewHandler(b)
			tt.fn(t, b, h)
		})
	}
}

// TestGetFindingRecommendation_WireShape verifies the required resourceArn
// and startedAt response members are present, and recommendationType uses
// the real wire value -- per aws-sdk-go-v2 api_op_GetFindingRecommendation.go
// (v1.51.4), resourceArn/startedAt/status are required
// GetFindingRecommendationOutput members, and RecommendationType's only
// known value is "UnusedPermissionRecommendation" (types/enums.go:579).
func TestGetFindingRecommendation_WireShape(t *testing.T) {
	t.Parallel()

	b := accessanalyzer.NewInMemoryBackend("000000000000", "us-east-1")
	h := accessanalyzer.NewHandler(b)

	arn := mustAnalyzer(t, b, "rec-wire-analyzer")
	findingID := mustFinding(t, b, "rec-wire-analyzer")

	// AnalyzerArn is a query parameter, not a body member -- the real SDK
	// sends no body for this operation
	// (awsRestjson1_serializeOpHttpBindingsGenerateFindingRecommendationInput,
	// aws-sdk-go-v2/service/accessanalyzer@v1.51.4 serializers.go).
	genRec := doRequest(t, h, http.MethodPost, "/recommendation/"+findingID+"?analyzerArn="+arn, nil)
	require.Equal(t, http.StatusOK, genRec.Code)

	getRec := doRequest(t, h, http.MethodGet, "/recommendation/"+findingID+"?analyzerArn="+arn, nil)
	require.Equal(t, http.StatusOK, getRec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &resp))

	assert.Equal(t, "UnusedPermissionRecommendation", resp["recommendationType"])
	assert.Equal(t, "arn:aws:s3:::bucket", resp["resourceArn"])
	assert.NotEmpty(t, resp["startedAt"])
	assert.NotEmpty(t, resp["completedAt"])
	assert.Equal(t, "SUCCEEDED", resp["status"])
}

// TestListFindings_RealClient_FilterByResourceType drives ListFindings
// through the real aws-sdk-go-v2 client with a "resourceType" Eq filter
// criterion (the real ListFindingsInput.Filter shape,
// map[string]types.Criterion) -- previously discarded entirely by the
// backend (ListFindings' filter parameter was named `_`), so a real
// client's filter was silently a no-op and every finding for the analyzer
// came back regardless of the criteria requested.
func TestListFindings_RealClient_FilterByResourceType(t *testing.T) {
	t.Parallel()

	b := accessanalyzer.NewInMemoryBackend("000000000000", "us-east-1")
	h := accessanalyzer.NewHandler(b)
	client := newTestAccessAnalyzerClient(t, h)

	analyzer, err := client.CreateAnalyzer(t.Context(), &aasdk.CreateAnalyzerInput{
		AnalyzerName: aws.String("filter-analyzer"),
		Type:         aatypes.TypeAccount,
	})
	require.NoError(t, err)

	_, err = b.AddFinding("filter-analyzer", "AWS::S3::Bucket", "arn:aws:s3:::bucket", nil, nil, nil)
	require.NoError(t, err)
	_, err = b.AddFinding("filter-analyzer", "AWS::IAM::Role", "arn:aws:iam::000000000000:role/r", nil, nil, nil)
	require.NoError(t, err)

	out, err := client.ListFindings(t.Context(), &aasdk.ListFindingsInput{
		AnalyzerArn: analyzer.Arn,
		Filter: map[string]aatypes.Criterion{
			"resourceType": {Eq: []string{"AWS::IAM::Role"}},
		},
	})
	require.NoError(t, err)
	require.Len(t, out.Findings, 1)
	assert.Equal(t, "AWS::IAM::Role", string(out.Findings[0].ResourceType))
}

// TestListFindingsV2_RealClient_FilterByResourceType is the same discarded-
// filter bug as TestListFindings_RealClient_FilterByResourceType, but for
// ListFindingsV2: that op's backend method took no filter parameter at all
// (the wire-real "filter" key was never even decoded from the request body).
func TestListFindingsV2_RealClient_FilterByResourceType(t *testing.T) {
	t.Parallel()

	b := accessanalyzer.NewInMemoryBackend("000000000000", "us-east-1")
	h := accessanalyzer.NewHandler(b)
	client := newTestAccessAnalyzerClient(t, h)

	analyzer, err := client.CreateAnalyzer(t.Context(), &aasdk.CreateAnalyzerInput{
		AnalyzerName: aws.String("filter-v2-analyzer"),
		Type:         aatypes.TypeAccount,
	})
	require.NoError(t, err)

	_, err = b.AddFinding("filter-v2-analyzer", "AWS::S3::Bucket", "arn:aws:s3:::bucket", nil, nil, nil)
	require.NoError(t, err)
	_, err = b.AddFinding("filter-v2-analyzer", "AWS::IAM::Role", "arn:aws:iam::000000000000:role/r", nil, nil, nil)
	require.NoError(t, err)

	out, err := client.ListFindingsV2(t.Context(), &aasdk.ListFindingsV2Input{
		AnalyzerArn: analyzer.Arn,
		Filter: map[string]aatypes.Criterion{
			"resourceType": {Eq: []string{"AWS::IAM::Role"}},
		},
	})
	require.NoError(t, err)
	require.Len(t, out.Findings, 1)
	assert.Equal(t, "AWS::IAM::Role", string(out.Findings[0].ResourceType))
}

// TestListFindings_RealClient_SortDescending drives ListFindings through the
// real client with Sort (ListFindingsInput.Sort, *types.SortCriteria) set to
// sort by resourceType descending. The handler never read the "sort" key
// from the request body at all, so the backend always returned findings in
// its own ascending-by-ID order regardless of what the client requested.
func TestListFindings_RealClient_SortDescending(t *testing.T) {
	t.Parallel()

	b := accessanalyzer.NewInMemoryBackend("000000000000", "us-east-1")
	h := accessanalyzer.NewHandler(b)
	client := newTestAccessAnalyzerClient(t, h)

	analyzer, err := client.CreateAnalyzer(t.Context(), &aasdk.CreateAnalyzerInput{
		AnalyzerName: aws.String("sort-analyzer"),
		Type:         aatypes.TypeAccount,
	})
	require.NoError(t, err)

	_, err = b.AddFinding("sort-analyzer", "AWS::S3::Bucket", "arn:aws:s3:::bucket", nil, nil, nil)
	require.NoError(t, err)
	_, err = b.AddFinding("sort-analyzer", "AWS::IAM::Role", "arn:aws:iam::000000000000:role/r", nil, nil, nil)
	require.NoError(t, err)
	_, err = b.AddFinding("sort-analyzer", "AWS::SQS::Queue", "arn:aws:sqs:::q", nil, nil, nil)
	require.NoError(t, err)

	out, err := client.ListFindings(t.Context(), &aasdk.ListFindingsInput{
		AnalyzerArn: analyzer.Arn,
		Sort: &aatypes.SortCriteria{
			AttributeName: aws.String("resourceType"),
			OrderBy:       aatypes.OrderByDesc,
		},
	})
	require.NoError(t, err)
	require.Len(t, out.Findings, 3)
	assert.Equal(t, []string{"AWS::SQS::Queue", "AWS::S3::Bucket", "AWS::IAM::Role"},
		[]string{
			string(out.Findings[0].ResourceType),
			string(out.Findings[1].ResourceType),
			string(out.Findings[2].ResourceType),
		})
}

// TestListFindingsV2_RealClient_SortDescending is the same missing-sort bug
// as TestListFindings_RealClient_SortDescending, for ListFindingsV2.
func TestListFindingsV2_RealClient_SortDescending(t *testing.T) {
	t.Parallel()

	b := accessanalyzer.NewInMemoryBackend("000000000000", "us-east-1")
	h := accessanalyzer.NewHandler(b)
	client := newTestAccessAnalyzerClient(t, h)

	analyzer, err := client.CreateAnalyzer(t.Context(), &aasdk.CreateAnalyzerInput{
		AnalyzerName: aws.String("sort-v2-analyzer"),
		Type:         aatypes.TypeAccount,
	})
	require.NoError(t, err)

	_, err = b.AddFinding("sort-v2-analyzer", "AWS::S3::Bucket", "arn:aws:s3:::bucket", nil, nil, nil)
	require.NoError(t, err)
	_, err = b.AddFinding("sort-v2-analyzer", "AWS::IAM::Role", "arn:aws:iam::000000000000:role/r", nil, nil, nil)
	require.NoError(t, err)
	_, err = b.AddFinding("sort-v2-analyzer", "AWS::SQS::Queue", "arn:aws:sqs:::q", nil, nil, nil)
	require.NoError(t, err)

	out, err := client.ListFindingsV2(t.Context(), &aasdk.ListFindingsV2Input{
		AnalyzerArn: analyzer.Arn,
		Sort: &aatypes.SortCriteria{
			AttributeName: aws.String("resourceType"),
			OrderBy:       aatypes.OrderByDesc,
		},
	})
	require.NoError(t, err)
	require.Len(t, out.Findings, 3)
	assert.Equal(t, []string{"AWS::SQS::Queue", "AWS::S3::Bucket", "AWS::IAM::Role"},
		[]string{
			string(out.Findings[0].ResourceType),
			string(out.Findings[1].ResourceType),
			string(out.Findings[2].ResourceType),
		})
}

// TestGetFindingsStatistics_RealClient_UnusedAccessUnion drives
// GetFindingsStatistics through the real aws-sdk-go-v2 client for an
// ACCOUNT_UNUSED_ACCESS analyzer. types.FindingsStatistics is a union keyed
// by wire name (awsRestjson1_deserializeDocumentFindingsStatistics,
// deserializers.go ~L9169): "externalAccessFindingsStatistics" vs
// "unusedAccessFindingsStatistics" decode into different Go types entirely.
// A handler that always emits the external-access key would make the real
// client's type switch land on the wrong branch for an unused-access
// analyzer's statistics.
func TestGetFindingsStatistics_RealClient_UnusedAccessUnion(t *testing.T) {
	t.Parallel()

	b := accessanalyzer.NewInMemoryBackend("000000000000", "us-east-1")
	h := accessanalyzer.NewHandler(b)
	client := newTestAccessAnalyzerClient(t, h)

	analyzer, err := client.CreateAnalyzer(t.Context(), &aasdk.CreateAnalyzerInput{
		AnalyzerName: aws.String("unused-access-analyzer"),
		Type:         aatypes.TypeAccountUnusedAccess,
	})
	require.NoError(t, err)

	mustFinding(t, b, "unused-access-analyzer")
	mustFinding(t, b, "unused-access-analyzer")

	out, err := client.GetFindingsStatistics(t.Context(), &aasdk.GetFindingsStatisticsInput{
		AnalyzerArn: analyzer.Arn,
	})
	require.NoError(t, err)
	require.Len(t, out.FindingsStatistics, 1)

	member, ok := out.FindingsStatistics[0].(*aatypes.FindingsStatisticsMemberUnusedAccessFindingsStatistics)
	require.True(t, ok, "expected unusedAccessFindingsStatistics union member, got %T", out.FindingsStatistics[0])
	assert.EqualValues(t, 2, *member.Value.TotalActiveFindings)
}

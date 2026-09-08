package inspector2_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/inspector2"
)

// TestListFindingAggregations_NonAccountType_RealClient covers gopherstack-or9.
// ListFindingAggregations always emitted an "accountAggregation"-keyed entry
// regardless of the requested AggregationType. types.AggregationResponse
// (inspector2@v1.54.1 types/types.go) is a real Smithy union with 15 members
// (accountAggregation, amiAggregation, packageAggregation,
// findingTypeAggregation, ...); the real deserializer
// (deserializers.go's awsRestjson1_deserializeDocumentAggregationResponse)
// picks which union member to populate purely from which JSON key is
// present in the response object -- it does not consult the request's
// AggregationType at all. So a real client requesting, say,
// AggregationType=PACKAGE always received an AccountAggregation value back
// (wrong union member, its own requested PackageAggregation.PackageName/etc
// never populated), silently discarding the aggregation the caller actually
// asked for, for every AggregationType except ACCOUNT (14 of 15 real
// values). The fix stops fabricating an accountAggregation-shaped entry for
// types this backend has no data for, returning an honestly empty responses
// list instead.
//
// gopherstack-f9vi later added real per-group data for TITLE, REPOSITORY,
// AWS_EC2_INSTANCE, AWS_ECR_CONTAINER, AWS_LAMBDA_FUNCTION and
// CODE_REPOSITORY (see ListFindingAggregations). TITLE and REPOSITORY stay
// in this table because the finding seeded below has no title and no
// Resources, so they still correctly produce no groups -- this pins the
// "no groupable data" case, not "these types are always unsupported".
// PACKAGE, FINDING_TYPE and AMI remain genuinely unsupported: PACKAGE has no
// backing sub-struct on FindingResource, FINDING_TYPE's response shape
// carries no group key to aggregate by, and AMI has no AMI-ID field
// anywhere in this model.
func TestListFindingAggregations_NonAccountType_RealClient(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		aggregationType string
	}{
		{name: "package", aggregationType: "PACKAGE"},
		{name: "finding_type", aggregationType: "FINDING_TYPE"},
		{name: "title", aggregationType: "TITLE"},
		{name: "repository", aggregationType: "REPOSITORY"},
		{name: "ami", aggregationType: "AMI"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := inspector2.NewInMemoryBackend("123456789012", "us-east-1")

			_, err := b.SeedFinding(inspector2.Finding{Type: "PACKAGE_VULNERABILITY"})
			require.NoError(t, err)

			got, err := b.ListFindingAggregations(tc.aggregationType, nil)
			require.NoError(t, err)

			assert.Equal(t, tc.aggregationType, got["aggregationType"],
				"aggregationType must always echo the request")
			assertEmptyAggregationResponses(t, got["responses"])
		})
	}
}

func assertEmptyAggregationResponses(t *testing.T, responses any) {
	t.Helper()

	switch r := responses.(type) {
	case []any:
		assert.Empty(t, r, "no accountAggregation-shaped entry may be returned for a non-ACCOUNT aggregationType")
	case []map[string]any:
		assert.Empty(t, r, "no accountAggregation-shaped entry may be returned for a non-ACCOUNT aggregationType")
	default:
		t.Fatalf("unexpected responses type %T", responses)
	}
}

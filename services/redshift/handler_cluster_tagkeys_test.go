package redshift_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	redshiftsdk "github.com/aws/aws-sdk-go-v2/service/redshift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/redshift"
)

// TestDescribeClusters_TagKeysFilter drives the real SDK client, whose
// DescribeClustersInput takes TagKeys/TagValues ([]string, wire-encoded as
// TagKeys.TagKey.N / TagValues.TagValue.N per redshift@v1.65.4
// serializers.go:12572), not the singular TagKey/TagValue query params the
// handler previously read. Real AWS also matches ANY tag whose key is in
// TagKeys OR whose value is in TagValues (service-2.json / api doc for
// DescribeClustersInput), not an AND of a single key/value pair.
func TestDescribeClusters_TagKeysFilter(t *testing.T) {
	t.Parallel()

	backend := redshift.NewInMemoryBackend("000000000000", rtTestRegion)
	h := redshift.NewHandler(backend)
	client := newTestRedshiftClient(t, h)
	ctx := t.Context()

	_, createErr := backend.CreateCluster("tagged-cluster", "dc2.large", "dev", "admin", nil, "")
	require.NoError(t, createErr)
	_, createErr = backend.CreateCluster("untagged-cluster", "dc2.large", "dev", "admin", nil, "")
	require.NoError(t, createErr)
	require.NoError(t, backend.CreateTags("tagged-cluster", map[string]string{"env": "prod"}))

	tests := []struct {
		name       string
		tagKeys    []string
		tagValues  []string
		wantIDs    []string
		wantAbsent []string
	}{
		{
			name:       "by key",
			tagKeys:    []string{"env"},
			wantIDs:    []string{"tagged-cluster"},
			wantAbsent: []string{"untagged-cluster"},
		},
		{
			name:       "by value",
			tagValues:  []string{"prod"},
			wantIDs:    []string{"tagged-cluster"},
			wantAbsent: []string{"untagged-cluster"},
		},
		{
			name:       "nonexistent key returns empty",
			tagKeys:    []string{"does-not-exist"},
			wantAbsent: []string{"tagged-cluster", "untagged-cluster"},
		},
		{
			name:    "no filter returns all",
			wantIDs: []string{"tagged-cluster", "untagged-cluster"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out, err := client.DescribeClusters(ctx, &redshiftsdk.DescribeClustersInput{
				TagKeys:   tc.tagKeys,
				TagValues: tc.tagValues,
			})
			require.NoError(t, err)

			gotIDs := make([]string, 0, len(out.Clusters))
			for _, c := range out.Clusters {
				gotIDs = append(gotIDs, aws.ToString(c.ClusterIdentifier))
			}

			for _, id := range tc.wantIDs {
				assert.Contains(t, gotIDs, id)
			}
			for _, id := range tc.wantAbsent {
				assert.NotContains(t, gotIDs, id)
			}
		})
	}
}

// TestDescribeClusters_TagKeysFilter_PaginationOrdering is the ordering-bug
// regression: create more tag-matching clusters than fit in one page, request
// a page smaller than the match count with TagKeys set, and confirm the first
// page is full and the Marker leads to the rest. A backend that paginates the
// raw cluster list before applying the tag filter (rather than filtering
// first) returns a short, filter-thinned first page here instead of a full
// one.
func TestDescribeClusters_TagKeysFilter_PaginationOrdering(t *testing.T) {
	t.Parallel()

	backend := redshift.NewInMemoryBackend("000000000000", rtTestRegion)
	h := redshift.NewHandler(backend)
	client := newTestRedshiftClient(t, h)
	ctx := t.Context()

	// Interleave matching and non-matching clusters so a naive "paginate
	// first" implementation puts non-matches in the early raw pages.
	ids := []string{"a-match", "b-nomatch", "c-match", "d-nomatch", "e-match", "f-nomatch"}
	for _, id := range ids {
		_, err := backend.CreateCluster(id, "dc2.large", "dev", "admin", nil, "")
		require.NoError(t, err)
	}

	for _, id := range []string{"a-match", "c-match", "e-match"} {
		require.NoError(t, backend.CreateTags(id, map[string]string{"env": "prod"}))
	}

	var (
		seen   []string
		marker *string
	)

	for {
		out, err := client.DescribeClusters(ctx, &redshiftsdk.DescribeClustersInput{
			TagKeys:    []string{"env"},
			MaxRecords: aws.Int32(2),
			Marker:     marker,
		})
		require.NoError(t, err)

		for _, c := range out.Clusters {
			seen = append(seen, aws.ToString(c.ClusterIdentifier))
		}

		if out.Marker == nil || *out.Marker == "" {
			break
		}

		marker = out.Marker
		require.LessOrEqual(t, len(seen), len(ids), "pagination did not terminate")
	}

	assert.ElementsMatch(t, []string{"a-match", "c-match", "e-match"}, seen)
}

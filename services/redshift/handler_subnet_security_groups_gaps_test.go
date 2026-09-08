package redshift_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	redshiftsdk "github.com/aws/aws-sdk-go-v2/service/redshift"
	"github.com/aws/aws-sdk-go-v2/service/redshift/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/redshift"
)

// gopherstack-igsa gap 3: DescribeClusterSubnetGroups and
// DescribeClusterSecurityGroups previously accepted no TagKeys, TagValues,
// Marker or MaxRecords at all. These tests drive the real SDK client, whose
// DescribeClusterSubnetGroupsInput/DescribeClusterSecurityGroupsInput both
// carry TagKeys/TagValues/Marker/MaxRecords (confirmed against
// awsAwsquery_serializeOpDocumentDescribeClusterSubnetGroupsInput and
// awsAwsquery_serializeOpDocumentDescribeClusterSecurityGroupsInput,
// redshift@v1.65.4 serializers.go), matching ANY tag whose key is in TagKeys
// OR whose value is in TagValues (same doc as DescribeClustersInput).

func TestDescribeClusterSubnetGroups_TagKeysFilter(t *testing.T) {
	t.Parallel()

	backend := redshift.NewInMemoryBackend("000000000000", rtTestRegion)
	h := redshift.NewHandler(backend)
	client := newTestRedshiftClient(t, h)
	ctx := t.Context()

	_, err := client.CreateClusterSubnetGroup(ctx, &redshiftsdk.CreateClusterSubnetGroupInput{
		ClusterSubnetGroupName: aws.String("tagged-sng"),
		Description:            aws.String("d"),
		SubnetIds:              []string{"subnet-1"},
		Tags:                   []types.Tag{{Key: aws.String("env"), Value: aws.String("prod")}},
	})
	require.NoError(t, err)

	_, err = client.CreateClusterSubnetGroup(ctx, &redshiftsdk.CreateClusterSubnetGroupInput{
		ClusterSubnetGroupName: aws.String("untagged-sng"),
		Description:            aws.String("d"),
		SubnetIds:              []string{"subnet-2"},
	})
	require.NoError(t, err)

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
			wantIDs:    []string{"tagged-sng"},
			wantAbsent: []string{"untagged-sng"},
		},
		{
			name:       "by value",
			tagValues:  []string{"prod"},
			wantIDs:    []string{"tagged-sng"},
			wantAbsent: []string{"untagged-sng"},
		},
		{
			name:       "nonexistent key excludes everything",
			tagKeys:    []string{"does-not-exist"},
			wantAbsent: []string{"tagged-sng", "untagged-sng"},
		},
		{
			name:    "no filter returns all",
			wantIDs: []string{"tagged-sng", "untagged-sng"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out, describeErr := client.DescribeClusterSubnetGroups(
				ctx,
				&redshiftsdk.DescribeClusterSubnetGroupsInput{
					TagKeys:   tc.tagKeys,
					TagValues: tc.tagValues,
				},
			)
			require.NoError(t, describeErr)

			gotIDs := make([]string, 0, len(out.ClusterSubnetGroups))
			for _, g := range out.ClusterSubnetGroups {
				gotIDs = append(gotIDs, aws.ToString(g.ClusterSubnetGroupName))
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

// TestDescribeClusterSubnetGroups_Pagination_Boundary verifies the
// off-by-one edges: exactly MaxRecords results in no Marker, one more than
// MaxRecords sets a Marker, and following it yields the remainder with no
// duplicates or gaps.
func TestDescribeClusterSubnetGroups_Pagination_Boundary(t *testing.T) {
	t.Parallel()

	backend := redshift.NewInMemoryBackend("000000000000", rtTestRegion)
	h := redshift.NewHandler(backend)
	client := newTestRedshiftClient(t, h)
	ctx := t.Context()

	names := []string{"sng-a", "sng-b", "sng-c"}
	for _, name := range names {
		_, err := client.CreateClusterSubnetGroup(ctx, &redshiftsdk.CreateClusterSubnetGroupInput{
			ClusterSubnetGroupName: aws.String(name),
			Description:            aws.String("d"),
			SubnetIds:              []string{"subnet-1"},
		})
		require.NoError(t, err)
	}

	// Exactly MaxRecords: no Marker.
	exact, err := client.DescribeClusterSubnetGroups(
		ctx,
		&redshiftsdk.DescribeClusterSubnetGroupsInput{
			MaxRecords: aws.Int32(int32(len(names))),
		},
	)
	require.NoError(t, err)
	assert.Len(t, exact.ClusterSubnetGroups, len(names))
	assert.Nil(t, exact.Marker)

	// One more than MaxRecords: Marker set, first page short.
	firstPage, err := client.DescribeClusterSubnetGroups(
		ctx,
		&redshiftsdk.DescribeClusterSubnetGroupsInput{
			MaxRecords: aws.Int32(int32(len(names) - 1)),
		},
	)
	require.NoError(t, err)
	require.Len(t, firstPage.ClusterSubnetGroups, len(names)-1)
	require.NotNil(t, firstPage.Marker)
	require.NotEmpty(t, *firstPage.Marker)

	// Follow the Marker: remainder, no duplicates or gaps.
	secondPage, err := client.DescribeClusterSubnetGroups(
		ctx,
		&redshiftsdk.DescribeClusterSubnetGroupsInput{
			MaxRecords: aws.Int32(int32(len(names) - 1)),
			Marker:     firstPage.Marker,
		},
	)
	require.NoError(t, err)
	assert.Nil(t, secondPage.Marker)

	seen := make(
		[]string,
		0,
		len(firstPage.ClusterSubnetGroups)+len(secondPage.ClusterSubnetGroups),
	)
	for _, g := range firstPage.ClusterSubnetGroups {
		seen = append(seen, aws.ToString(g.ClusterSubnetGroupName))
	}
	for _, g := range secondPage.ClusterSubnetGroups {
		seen = append(seen, aws.ToString(g.ClusterSubnetGroupName))
	}
	assert.ElementsMatch(t, names, seen)
}

func TestDescribeClusterSecurityGroups_TagKeysFilter(t *testing.T) {
	t.Parallel()

	backend := redshift.NewInMemoryBackend("000000000000", rtTestRegion)
	h := redshift.NewHandler(backend)
	client := newTestRedshiftClient(t, h)
	ctx := t.Context()

	_, err := client.CreateClusterSecurityGroup(ctx, &redshiftsdk.CreateClusterSecurityGroupInput{
		ClusterSecurityGroupName: aws.String("tagged-csg"),
		Description:              aws.String("d"),
		Tags:                     []types.Tag{{Key: aws.String("env"), Value: aws.String("prod")}},
	})
	require.NoError(t, err)

	_, err = client.CreateClusterSecurityGroup(ctx, &redshiftsdk.CreateClusterSecurityGroupInput{
		ClusterSecurityGroupName: aws.String("untagged-csg"),
		Description:              aws.String("d"),
	})
	require.NoError(t, err)

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
			wantIDs:    []string{"tagged-csg"},
			wantAbsent: []string{"untagged-csg"},
		},
		{
			name:       "by value",
			tagValues:  []string{"prod"},
			wantIDs:    []string{"tagged-csg"},
			wantAbsent: []string{"untagged-csg"},
		},
		{
			name:       "nonexistent key excludes everything",
			tagKeys:    []string{"does-not-exist"},
			wantAbsent: []string{"tagged-csg", "untagged-csg"},
		},
		{
			name:    "no filter returns all",
			wantIDs: []string{"tagged-csg", "untagged-csg"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out, describeErr := client.DescribeClusterSecurityGroups(
				ctx,
				&redshiftsdk.DescribeClusterSecurityGroupsInput{
					TagKeys:   tc.tagKeys,
					TagValues: tc.tagValues,
				},
			)
			require.NoError(t, describeErr)

			gotIDs := make([]string, 0, len(out.ClusterSecurityGroups))
			for _, g := range out.ClusterSecurityGroups {
				gotIDs = append(gotIDs, aws.ToString(g.ClusterSecurityGroupName))
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

// TestDescribeClusterSecurityGroups_Pagination_Boundary verifies the
// off-by-one edges: exactly MaxRecords results in no Marker, one more than
// MaxRecords sets a Marker, and following it yields the remainder with no
// duplicates or gaps.
func TestDescribeClusterSecurityGroups_Pagination_Boundary(t *testing.T) {
	t.Parallel()

	backend := redshift.NewInMemoryBackend("000000000000", rtTestRegion)
	h := redshift.NewHandler(backend)
	client := newTestRedshiftClient(t, h)
	ctx := t.Context()

	names := []string{"csg-a", "csg-b", "csg-c"}
	for _, name := range names {
		_, err := client.CreateClusterSecurityGroup(
			ctx,
			&redshiftsdk.CreateClusterSecurityGroupInput{
				ClusterSecurityGroupName: aws.String(name),
				Description:              aws.String("d"),
			},
		)
		require.NoError(t, err)
	}

	// Exactly MaxRecords: no Marker.
	exact, err := client.DescribeClusterSecurityGroups(
		ctx,
		&redshiftsdk.DescribeClusterSecurityGroupsInput{
			MaxRecords: aws.Int32(int32(len(names))),
		},
	)
	require.NoError(t, err)
	assert.Len(t, exact.ClusterSecurityGroups, len(names))
	assert.Nil(t, exact.Marker)

	// One more than MaxRecords: Marker set, first page short.
	firstPage, err := client.DescribeClusterSecurityGroups(
		ctx,
		&redshiftsdk.DescribeClusterSecurityGroupsInput{
			MaxRecords: aws.Int32(int32(len(names) - 1)),
		},
	)
	require.NoError(t, err)
	require.Len(t, firstPage.ClusterSecurityGroups, len(names)-1)
	require.NotNil(t, firstPage.Marker)
	require.NotEmpty(t, *firstPage.Marker)

	// Follow the Marker: remainder, no duplicates or gaps.
	secondPage, err := client.DescribeClusterSecurityGroups(
		ctx,
		&redshiftsdk.DescribeClusterSecurityGroupsInput{
			MaxRecords: aws.Int32(int32(len(names) - 1)),
			Marker:     firstPage.Marker,
		},
	)
	require.NoError(t, err)
	assert.Nil(t, secondPage.Marker)

	seen := make(
		[]string,
		0,
		len(firstPage.ClusterSecurityGroups)+len(secondPage.ClusterSecurityGroups),
	)
	for _, g := range firstPage.ClusterSecurityGroups {
		seen = append(seen, aws.ToString(g.ClusterSecurityGroupName))
	}
	for _, g := range secondPage.ClusterSecurityGroups {
		seen = append(seen, aws.ToString(g.ClusterSecurityGroupName))
	}
	assert.ElementsMatch(t, names, seen)
}

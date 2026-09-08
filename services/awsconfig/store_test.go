package awsconfig_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/awsconfig"
)

func TestReset_ClearsNewMaps(t *testing.T) {
	t.Parallel()

	b := awsconfig.NewInMemoryBackend()
	_ = b.TagResource("arn:test", []awsconfig.Tag{{Key: "k", Value: "v"}})
	_ = b.PutRetentionConfiguration("default", 90)
	_ = b.PutRemediationConfigurations([]awsconfig.RemediationConfiguration{{ConfigRuleName: "r1"}})
	_ = b.PutResourceConfig("AWS::S3::Bucket", "b1", "{}")

	b.Reset()

	if tags := b.ListTagsForResource("arn:test"); len(tags) != 0 {
		t.Fatal("tags not cleared by Reset")
	}

	if configs := b.DescribeRetentionConfigurations(); len(configs) != 0 {
		t.Fatal("retentionConfigs not cleared by Reset")
	}

	if rc := b.DescribeRemediationConfigurations(nil); len(rc) != 0 {
		t.Fatal("remediationConfigs not cleared by Reset")
	}

	if err := b.PutConfigurationAggregator("agg1", nil, nil, nil); err != nil {
		t.Fatalf("PutConfigurationAggregator: %v", err)
	}

	if count, err := b.GetAggregateDiscoveredResourceCounts("agg1"); err != nil || count != 0 {
		t.Fatalf("resourceConfigs not cleared by Reset, count=%d, err=%v", count, err)
	}
}

// TestReset_AggregatorAndConformancePackCountersRestart verifies that Reset()
// zeroes aggregatorCounter/conformancePackCounter, so the next aggregator/pack
// created after Reset gets ID suffix 1 again (matching ec2's fix establishing
// this codebase resets ID sequence counters on Reset -- nextPrivateIPIndex,
// nextElasticIPIndex), not a suffix that keeps climbing from the pre-Reset run.
func TestReset_AggregatorAndConformancePackCountersRestart(t *testing.T) {
	t.Parallel()

	b := awsconfig.NewInMemoryBackend()

	require.NoError(t, b.PutConfigurationAggregator("agg1", nil, nil, nil))
	require.NoError(t, b.PutConformancePack("pack1", "", "", "", "", "", nil))

	aggsBefore := b.DescribeConfigurationAggregators()
	require.Len(t, aggsBefore, 1)
	require.True(t, strings.HasSuffix(aggsBefore[0].ConfigurationAggregatorArn, "-00000001"))

	packsBefore := b.DescribeConformancePacks()
	require.Len(t, packsBefore, 1)
	require.True(t, strings.HasSuffix(packsBefore[0].ConformancePackID, "-00000001"))

	b.Reset()

	require.NoError(t, b.PutConfigurationAggregator("agg2", nil, nil, nil))
	require.NoError(t, b.PutConformancePack("pack2", "", "", "", "", "", nil))

	aggsAfter := b.DescribeConfigurationAggregators()
	require.Len(t, aggsAfter, 1)
	assert.True(t, strings.HasSuffix(aggsAfter[0].ConfigurationAggregatorArn, "-00000001"),
		"aggregatorCounter must restart at 1 after Reset, got ARN %s", aggsAfter[0].ConfigurationAggregatorArn)

	packsAfter := b.DescribeConformancePacks()
	require.Len(t, packsAfter, 1)
	assert.True(t, strings.HasSuffix(packsAfter[0].ConformancePackID, "-00000001"),
		"conformancePackCounter must restart at 1 after Reset, got ID %s", packsAfter[0].ConformancePackID)
}

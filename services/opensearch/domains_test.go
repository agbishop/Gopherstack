package opensearch_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/opensearch"
)

func TestAddDomainInternal(t *testing.T) {
	t.Parallel()

	b := opensearch.NewInMemoryBackend(testAccountID, testRegion)
	b.AddDomainInternal("seed-domain", "OpenSearch_2.3")

	d, err := b.DescribeDomain("seed-domain")
	require.NoError(t, err)
	assert.Equal(t, "OpenSearch_2.3", d.EngineVersion)
	assert.NotEmpty(t, d.ARN)
	assert.NotEmpty(t, d.Endpoint)
}

func TestAddDomainInternal_DefaultVersion(t *testing.T) {
	t.Parallel()

	b := opensearch.NewInMemoryBackend(testAccountID, testRegion)
	b.AddDomainInternal("seed-domain", "")

	d, err := b.DescribeDomain("seed-domain")
	require.NoError(t, err)
	assert.Equal(t, "OpenSearch_2.11", d.EngineVersion)
}

func TestDeleteDomain_Cascade(t *testing.T) {
	t.Parallel()

	b := opensearch.NewInMemoryBackend(testAccountID, testRegion)
	b.AddDomainInternal("my-domain", "")

	_, err := b.AddDataSource("my-domain", "ds1", "desc", json.RawMessage(`{"S3GlueDataCatalog":{}}`))
	require.NoError(t, err)

	b.AddPackageInternal("pkg-001", "test-pkg", "TXT-DICTIONARY")

	_, err = b.AssociatePackage("pkg-001", "my-domain")
	require.NoError(t, err)

	_, err = b.AuthorizeVpcEndpointAccess("my-domain", "111122223333", "")
	require.NoError(t, err)

	assert.Equal(t, 1, opensearch.DataSourceCount(b))

	_, err = b.DeleteDomain("my-domain")
	require.NoError(t, err)

	// Domain data sources cleaned up.
	assert.Equal(t, 0, opensearch.DataSourceCount(b))
	// ARN index cleaned up.
	assert.Equal(t, 0, opensearch.ARNIndexSize(b))
}

// TestDeleteDomain_ClearsScheduledActions verifies that DeleteDomain clears
// scheduledActions for the deleted domain name. Otherwise a new domain
// created with the same (user-chosen, reusable) name inherits the deleted
// domain's stale scheduled actions.
func TestDeleteDomain_ClearsScheduledActions(t *testing.T) {
	t.Parallel()

	b := opensearch.NewInMemoryBackend(testAccountID, testRegion)
	b.AddDomainInternal("reused-domain", "")

	opensearch.AddScheduledActionInternal(b, "reused-domain", &opensearch.ScheduledAction{
		ID:   "ghost-action",
		Type: "SERVICE_SOFTWARE_UPDATE",
	})
	require.NotEmpty(t, b.ListScheduledActions("reused-domain"))

	_, err := b.DeleteDomain("reused-domain")
	require.NoError(t, err)

	b.AddDomainInternal("reused-domain", "")

	assert.Empty(t, b.ListScheduledActions("reused-domain"),
		"recreated domain must not inherit the deleted domain's scheduled actions")
}

// TestDeleteDomain_ClearsVpcEndpoints verifies that DeleteDomain cleans up
// VPC endpoints associated with the deleted domain. DomainArn is deterministic
// from the domain name (arn.Build), so a new domain created with the same
// (user-chosen, reusable) name would otherwise silently inherit the deleted
// domain's stale VPC endpoints -- the same ghost-row class already guarded
// for scheduled actions in TestDeleteDomain_ClearsScheduledActions.
func TestDeleteDomain_ClearsVpcEndpoints(t *testing.T) {
	t.Parallel()

	b := opensearch.NewInMemoryBackend(testAccountID, testRegion)
	created, err := b.CreateDomain(opensearch.CreateDomainInput{Name: "reused-vpc-domain"})
	require.NoError(t, err)

	_, err = b.CreateVpcEndpoint(created.ARN, map[string]any{"SubnetIds": []any{"subnet-1"}})
	require.NoError(t, err)
	require.Len(t, b.ListVpcEndpointsForDomain(created.ARN), 1)

	_, err = b.DeleteDomain("reused-vpc-domain")
	require.NoError(t, err)

	recreated, err := b.CreateDomain(opensearch.CreateDomainInput{Name: "reused-vpc-domain"})
	require.NoError(t, err)
	require.Equal(t, created.ARN, recreated.ARN,
		"domain ARN must be deterministic from the name for this test to be meaningful")

	assert.Empty(t, b.ListVpcEndpointsForDomain(recreated.ARN),
		"recreated domain must not inherit the deleted domain's stale VPC endpoints")
}

func TestListDomainNames_Sorted(t *testing.T) {
	t.Parallel()

	b := opensearch.NewInMemoryBackend(testAccountID, testRegion)
	b.AddDomainInternal("zebra", "")
	b.AddDomainInternal("apple", "")
	b.AddDomainInternal("mango", "")

	names := b.ListDomainNames()
	require.Len(t, names, 3)
	assert.Equal(t, []string{"apple", "mango", "zebra"}, names)
}

package cloudfront_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/services/cloudfront"
)

// TestPersistenceRoundTrip_ExtendedFields verifies that batch-2 fields
// (TrustStores, StreamingDistributions, MonitoringSubscriptions,
// DistributionTenants, DistributionCachePolicies, etc.) survive a
// Snapshot → Restore cycle.
func TestPersistenceRoundTrip_ExtendedFields(t *testing.T) {
	t.Parallel()

	type restoreTC struct {
		setup  func(*cloudfront.InMemoryBackend)
		verify func(*testing.T, *cloudfront.InMemoryBackend)
		name   string
	}
	tests := []restoreTC{
		{
			name: "trust_store_survives_restore",
			setup: func(b *cloudfront.InMemoryBackend) {
				_, err := b.CreateTrustStore("my-store", "comment", cloudfront.TrustStoreCertificateBundle{}, nil)
				require.NoError(t, err)
			},
			verify: func(t *testing.T, b *cloudfront.InMemoryBackend) {
				t.Helper()
				stores := b.ListTrustStores()
				require.Len(t, stores, 1)
				assert.Equal(t, "my-store", stores[0].Name)
			},
		},
		{
			name: "streaming_distribution_survives_restore",
			setup: func(b *cloudfront.InMemoryBackend) {
				_, err := b.CreateStreamingDistribution(
					cloudfront.StreamingDistributionConfig{}, []byte(`<StreamingDistributionConfig/>`),
				)
				require.NoError(t, err)
			},
			verify: func(t *testing.T, b *cloudfront.InMemoryBackend) {
				t.Helper()
				sds := b.ListStreamingDistributions()
				assert.Len(t, sds, 1)
			},
		},
		{
			name: "monitoring_subscription_survives_restore",
			setup: func(b *cloudfront.InMemoryBackend) {
				d, err := b.CreateDistribution("ref-mon", "test", true, nil)
				require.NoError(t, err)
				require.NoError(t, b.CreateMonitoringSubscription(d.ID, true))
			},
			verify: func(t *testing.T, b *cloudfront.InMemoryBackend) {
				t.Helper()
				dists := b.ListDistributions()
				require.Len(t, dists, 1)
				ms, err := b.GetMonitoringSubscription(dists[0].ID)
				require.NoError(t, err)
				assert.Equal(t, "Enabled", ms.RealtimeMetricsSubscriptionStatus)
			},
		},
		{
			name: "distribution_tenant_survives_restore",
			setup: func(b *cloudfront.InMemoryBackend) {
				d, err := b.CreateDistribution("ref-ten", "test", true, nil)
				require.NoError(t, err)
				_, err = b.CreateDistributionTenant(d.ID, "parity-tenant", []string{"tenant.example.com"}, nil)
				require.NoError(t, err)
			},
			verify: func(t *testing.T, b *cloudfront.InMemoryBackend) {
				t.Helper()
				tenants := b.ListDistributionTenants()
				require.Len(t, tenants, 1)
				assert.Equal(t, "tenant.example.com", tenants[0].Domain)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			orig := newTestBackend(t)
			tc.setup(orig)

			snap := orig.Snapshot(t.Context())
			require.NotEmpty(t, snap)

			fresh := newTestBackend(t)
			require.NoError(t, fresh.Restore(t.Context(), snap))

			tc.verify(t, fresh)
		})
	}
}

// TestPersistenceRoundTrip_NewResourceTypes verifies the new resource types survive snapshot/restore.
func TestPersistenceRoundTrip_NewResourceTypes(t *testing.T) {
	t.Parallel()

	b := cloudfront.NewInMemoryBackend(t.Context(), "000000000000", "us-east-1")

	fle, err := b.CreateFieldLevelEncryption("persist-fle", "comment", nil)
	require.NoError(t, err)

	fleP, err := b.CreateFieldLevelEncryptionProfile("persist-fle-profile", "comment", nil)
	require.NoError(t, err)

	pk, err := b.CreatePublicKey("pk-persist-ref", "persist-pk", "comment", testRSA2048PublicKeyPEM)
	require.NoError(t, err)

	kg, err := b.CreateKeyGroup("persist-kg", "comment", []string{pk.ID})
	require.NoError(t, err)

	rl, err := b.CreateRealtimeLogConfig("persist-rl", 100, []string{"ts"}, testEndPoints())
	require.NoError(t, err)

	kvs, err := b.CreateKeyValueStore("persist-kvs", "comment", nil)
	require.NoError(t, err)

	vpc, err := b.CreateVpcOrigin(testVpcOriginEndpointConfig("persist-vpc"), nil)
	require.NoError(t, err)

	h := cloudfront.NewHandler(b)
	snap := h.Snapshot(t.Context())
	require.NotEmpty(t, snap)

	b2 := cloudfront.NewInMemoryBackend(t.Context(), "000000000000", "us-east-1")
	h2 := cloudfront.NewHandler(b2)
	require.NoError(t, h2.Restore(t.Context(), snap))

	_, err = b2.GetFieldLevelEncryption(fle.ID)
	require.NoError(t, err)

	_, err = b2.GetFieldLevelEncryptionProfile(fleP.ID)
	require.NoError(t, err)

	_, err = b2.GetPublicKey(pk.ID)
	require.NoError(t, err)

	_, err = b2.GetKeyGroup(kg.ID)
	require.NoError(t, err)

	_, err = b2.GetRealtimeLogConfig(rl.ARN)
	require.NoError(t, err)

	_, err = b2.GetKeyValueStore(kvs.ID)
	require.NoError(t, err)

	_, err = b2.GetVpcOrigin(vpc.ID)
	require.NoError(t, err)
}

// TestPersistenceRoundTrip_StringFields verifies that persistence doesn't lose data on string fields.
func TestPersistenceRoundTrip_StringFields(t *testing.T) {
	t.Parallel()

	b := cloudfront.NewInMemoryBackend(t.Context(), "000000000000", "us-east-1")

	pk, err := b.CreatePublicKey("str-ref", "str-pk", "pk-comment", testRSA2048PublicKeyPEM)
	require.NoError(t, err)

	h := cloudfront.NewHandler(b)
	snap := h.Snapshot(t.Context())
	require.NotEmpty(t, snap)

	b2 := cloudfront.NewInMemoryBackend(t.Context(), "000000000000", "us-east-1")
	h2 := cloudfront.NewHandler(b2)
	require.NoError(t, h2.Restore(t.Context(), snap))

	restored, err := b2.GetPublicKey(pk.ID)
	require.NoError(t, err)
	assert.Equal(t, "pk-comment", restored.Comment)
	assert.Equal(t, testRSA2048PublicKeyPEM, restored.EncodedKey)
	assert.Equal(t, "str-ref", restored.CallerReference)
}

func TestCloudFront_PersistenceSnapshotRestore(t *testing.T) {
	t.Parallel()

	b := cloudfront.NewInMemoryBackend(t.Context(), "000000000000", "us-east-1")
	d, err := b.CreateDistribution("ref1", "my dist", true, nil)
	require.NoError(t, err)

	_, err = b.CreateInvalidation(d.ID, "inv-ref1", []string{"/index.html", "/static/*"})
	require.NoError(t, err)

	_, err = b.CreateOAI("oai-ref1", "my oai")
	require.NoError(t, err)

	h := cloudfront.NewHandler(b)
	snap := h.Snapshot(t.Context())
	require.NotEmpty(t, snap)

	b2 := cloudfront.NewInMemoryBackend(t.Context(), "000000000000", "us-east-1")
	h2 := cloudfront.NewHandler(b2)
	require.NoError(t, h2.Restore(t.Context(), snap))

	// Distribution is restored.
	d2, err := b2.GetDistribution(d.ID)
	require.NoError(t, err)
	assert.Equal(t, "my dist", d2.Comment)

	// Invalidations are restored.
	invs, err := b2.ListInvalidations(d.ID)
	require.NoError(t, err)
	assert.Len(t, invs, 1)

	// ARN index is rebuilt — tag operations should work.
	err = b2.TagResource(d2.ARN, map[string]string{"env": "test"})
	require.NoError(t, err, "tag operation should succeed after restore (ARN index rebuilt)")
}

// TestNewOperations_PersistenceRoundTrip verifies that all new resources survive snapshot/restore.
func TestNewOperations_PersistenceRoundTrip(t *testing.T) {
	t.Parallel()

	b := cloudfront.NewInMemoryBackend(t.Context(), "000000000000", "us-east-1")

	// Create a distribution and associate an alias + web ACL.
	d, err := b.CreateDistribution("ref-persist-1", "persist-dist", true, nil)
	require.NoError(t, err)

	err = b.AssociateAlias(d.ID, "persist.example.com")
	require.NoError(t, err)

	err = b.AssociateDistributionWebACL(d.ID, "arn:aws:wafv2:us-east-1:123:global/webacl/test/abc")
	require.NoError(t, err)

	err = b.AssociateDistributionTenantWebACL("tenant-persist-001", "acl-for-tenant")
	require.NoError(t, err)

	// Copy the distribution.
	_, err = b.CopyDistribution(d.ID, "copy-persist-ref")
	require.NoError(t, err)

	// Create new resource types.
	_, err = b.CreateAnycastIPList("persist-anycast-list", 3, nil)
	require.NoError(t, err)

	_, err = b.CreateCachePolicy("persist-cache-policy", "comment", 86400, 31536000, 0)
	require.NoError(t, err)

	_, err = b.CreateConnectionFunction("persist-conn-fn", "comment")
	require.NoError(t, err)

	_, err = b.CreateConnectionGroup("persist-conn-group", "comment")
	require.NoError(t, err)

	_, err = b.CreateContinuousDeploymentPolicy(true, "")
	require.NoError(t, err)

	h := cloudfront.NewHandler(b)
	snap := h.Snapshot(t.Context())
	require.NotEmpty(t, snap)

	b2 := cloudfront.NewInMemoryBackend(t.Context(), "000000000000", "us-east-1")
	h2 := cloudfront.NewHandler(b2)
	require.NoError(t, h2.Restore(t.Context(), snap))

	// Distribution is restored.
	d2, err := b2.GetDistribution(d.ID)
	require.NoError(t, err)
	assert.Equal(t, "persist-dist", d2.Comment)
}

// TestPersistenceRoundTrip_IndexesRebuilt verifies indexes are rebuilt after snapshot/restore.
func TestPersistenceRoundTrip_IndexesRebuilt(t *testing.T) {
	t.Parallel()

	b := cloudfront.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)

	// Create resources with known CallerReferences.
	d, err := b.CreateDistribution("persist-ref-001", "persist-dist", true, nil)
	require.NoError(t, err)

	_, err = b.CreateOAI("persist-oai-ref-001", "persist-oai")
	require.NoError(t, err)

	_, err = b.CreateCachePolicy("persist-cp", "comment", 86400, 31536000, 0)
	require.NoError(t, err)

	h := cloudfront.NewHandler(b)
	snap := h.Snapshot(t.Context())
	require.NotEmpty(t, snap)

	b2 := cloudfront.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)
	h2 := cloudfront.NewHandler(b2)
	require.NoError(t, h2.Restore(t.Context(), snap))

	// The restored distributionCallerRefs index should still reject a reused
	// CallerReference after restore, exactly like it would before a
	// snapshot/restore round trip (CreateDistribution never treats CallerReference
	// reuse as idempotent -- see distributions.go's CreateDistribution doc).
	_, err = b2.CreateDistribution("persist-ref-001", "persist-dist", true, nil)
	require.Error(t, err, "CallerReference reuse should still conflict after restore")
	require.ErrorIs(t, err, cloudfront.ErrDistributionAlreadyExists)
	assert.NotEmpty(t, d.ID)

	// CachePolicy name uniqueness should work after restore.
	_, err = b2.CreateCachePolicy("persist-cp", "comment", 86400, 31536000, 0)
	require.Error(t, err, "duplicate cache policy name should be rejected after restore")
}

// TestPersistenceRoundTrip_FieldLevelEncryptionSharedCallerReference is a regression test for
// gopherstack-lt9v: two FLE configs may legitimately share a CallerReference after
// UpdateFieldLevelEncryption (gopherstack-kpk5), so a snapshot/restore round trip must not
// lose either config, and the create-time collision check must still work correctly
// afterwards.
func TestPersistenceRoundTrip_FieldLevelEncryptionSharedCallerReference(t *testing.T) {
	t.Parallel()

	b := cloudfront.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)

	owner, err := b.CreateFieldLevelEncryption("shared-ref", "owner", nil)
	require.NoError(t, err)
	other, err := b.CreateFieldLevelEncryption("other-ref", "other", nil)
	require.NoError(t, err)

	_, err = b.UpdateFieldLevelEncryption(other.ID, "shared-ref", "renamed onto owner", nil)
	require.NoError(t, err)

	h := cloudfront.NewHandler(b)
	snap := h.Snapshot(t.Context())
	require.NotEmpty(t, snap)

	b2 := cloudfront.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)
	h2 := cloudfront.NewHandler(b2)
	require.NoError(t, h2.Restore(t.Context(), snap))

	gotOwner, err := b2.GetFieldLevelEncryption(owner.ID)
	require.NoError(t, err)
	assert.Equal(t, "shared-ref", gotOwner.Name)

	gotOther, err := b2.GetFieldLevelEncryption(other.ID)
	require.NoError(t, err)
	assert.Equal(t, "shared-ref", gotOther.Name)

	_, err = b2.CreateFieldLevelEncryption("shared-ref", "should still collide", nil)
	require.ErrorIs(t, err, cloudfront.ErrFLEAlreadyExists)

	require.NoError(t, b2.DeleteFieldLevelEncryption(owner.ID))

	stillProtected, err := b2.GetFieldLevelEncryption(other.ID)
	require.NoError(t, err)
	assert.Equal(t, "shared-ref", stillProtected.Name)

	_, err = b2.CreateFieldLevelEncryption("shared-ref", "should collide with other", nil)
	require.ErrorIs(t, err, cloudfront.ErrFLEAlreadyExists)
}

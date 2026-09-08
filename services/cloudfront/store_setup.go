package cloudfront

// Code in this file supports Phase 3.3 of the datalayer refactor: every
// map[string]*T resource field on InMemoryBackend is registered exactly once,
// here, as a *store.Table[T] on b.registry. See pkgs/store's package doc and
// the services/ec2 (commit 12e611a4), services/sqs (commit 0f09d77c), and
// services/apigateway (commit 6da0334e) conversions this follows.
//
// invalidations and tenantInvalidations were previously nested per-parent maps
// (map[string]([]*Invalidation), keyed by distribution ID / tenant ID
// respectively). store.Table has no notion of nesting, so each becomes a
// single flat table keyed by a composite "<parentID>#<invalidationID>" string,
// with a secondary [store.Index] grouping by parent ID for the "all
// invalidations of parent X" lookups the nested maps used to answer directly
// (mirrors apigateway's resources/deployments/etc.). This requires Invalidation
// to carry its parent's ID as an identity-only (json:"-") field -- see the
// distID/tenantID doc on the Invalidation type in store.go.
//
// A handful of fields are deliberately NOT registered here and remain plain
// maps -- see the comment block above registerAllTables for the list and why.
import "github.com/blackbirdworks/gopherstack/pkgs/store"

func distributionKeyFn(v *Distribution) string                             { return v.ID }
func oaiKeyFn(v *OriginAccessIdentity) string                              { return v.ID }
func anycastIPListKeyFn(v *AnycastIPList) string                           { return v.ID }
func cachePolicyKeyFn(v *CachePolicy) string                               { return v.ID }
func connectionFunctionKeyFn(v *ConnectionFunction) string                 { return v.ID }
func connectionGroupKeyFn(v *ConnectionGroup) string                       { return v.ID }
func continuousDeploymentPolicyKeyFn(v *ContinuousDeploymentPolicy) string { return v.ID }
func originAccessControlKeyFn(v *OriginAccessControl) string               { return v.ID }
func responseHeadersPolicyKeyFn(v *ResponseHeadersPolicy) string           { return v.ID }
func functionKeyFn(v *Function) string                                     { return v.Name }
func originRequestPolicyKeyFn(v *OriginRequestPolicy) string               { return v.ID }
func fieldLevelEncryptionKeyFn(v *FieldLevelEncryption) string             { return v.ID }

func fieldLevelEncryptionProfileKeyFn(v *FieldLevelEncryptionProfile) string { return v.ID }
func publicKeyKeyFn(v *PublicKey) string                                     { return v.ID }
func keyGroupKeyFn(v *KeyGroup) string                                       { return v.ID }

// realtimeLogConfigKeyFn keys by ARN (not ID -- RealtimeLogConfig has no separate ID
// field; ARN is its stable identity, assigned once at creation and never mutated by
// UpdateRealtimeLogConfig), matching the pre-conversion map's "ARN → config" shape.
func realtimeLogConfigKeyFn(v *RealtimeLogConfig) string { return v.ARN }

func keyValueStoreKeyFn(v *KeyValueStore) string { return v.ID }
func vpcOriginKeyFn(v *VpcOrigin) string         { return v.ID }
func trustStoreKeyFn(v *TrustStore) string       { return v.ID }

func streamingDistributionKeyFn(v *StreamingDistribution) string { return v.ID }
func distributionTenantKeyFn(v *DistributionTenant) string       { return v.ID }

// invalidationKey builds the composite "<parentID>#<invalidationID>" key shared by
// both invalidation families. Access sites use this directly so the exact same key
// is used for Put/Get/Delete.
func invalidationKey(parentID, id string) string { return parentID + "#" + id }

func invalidationDistKeyFn(v *Invalidation) string   { return invalidationKey(v.distID, v.ID) }
func invalidationTenantKeyFn(v *Invalidation) string { return invalidationKey(v.tenantID, v.ID) }

// deleteInvalidationsForDist removes every invalidation belonging to distID from
// b.invalidations, keeping b.invalidationsByDist consistent. It replaces the old
// map-based "delete(b.invalidations, distID)" cascade delete (DeleteDistribution)
// now that invalidations is a flat composite-key table rather than one slice per
// distribution. The slice is copied before deleting: [store.Index.Get]'s result is
// owned by the index and must not be retained across a [store.Table.Delete] call,
// which may reuse or invalidate it. Must be called with the lock held.
func (b *InMemoryBackend) deleteInvalidationsForDist(distID string) {
	for _, inv := range append([]*Invalidation(nil), b.invalidationsByDist.Get(distID)...) {
		b.invalidations.Delete(invalidationKey(distID, inv.ID))
	}
}

// deleteInvalidationsForTenant is deleteInvalidationsForDist's tenant-scoped twin,
// used by DeleteDistributionTenant.
func (b *InMemoryBackend) deleteInvalidationsForTenant(tenantID string) {
	for _, inv := range append([]*Invalidation(nil), b.tenantInvalidationsByTenant.Get(tenantID)...) {
		b.tenantInvalidations.Delete(invalidationKey(tenantID, inv.ID))
	}
}

// registerAllTables sets up every converted resource collection exactly once, at
// construction time.
//
// Two groups, split by whether their store.Table can be snapshotted directly:
//
//   - "Clean" tables (distributions, oais, anycastIPLists, cachePolicies,
//     connectionFunctions, connectionGroups, continuousDeploymentPolicies,
//     originAccessControls, responseHeadersPolicies, functions,
//     originRequestPolicies, fieldLevelEncryptions, fieldLevelEncryptionProfiles,
//     publicKeys, keyGroups, realtimeLogConfigs, keyValueStores, vpcOrigins,
//     trustStores, streamingDistributions, distributionTenants) are registered on
//     b.registry via store.Register, so persistence.go's Snapshot/Restore can drive
//     them through b.registry.SnapshotAll()/RestoreAll() -- their value types
//     marshal to JSON with no information loss.
//   - "Dirty" tables (invalidations, tenantInvalidations) are built with store.New
//     but deliberately NOT registered on b.registry. Their composite key depends on
//     a field (distID / tenantID) tagged `json:"-"` on the live type (see
//     store.go), so a direct json.Marshal/Unmarshal round trip through
//     Table.Snapshot/Restore would silently drop that field and corrupt the
//     table's key on restore. persistence.go instead builds a throwaway DTO
//     [store.Registry] purely to get a correctly-tagged JSON encoding (mirrors the
//     services/apigateway conversion, commit 6da0334e, itself mirroring the
//     services/sqs pilot, commit 0f09d77c), then restores the live tables directly
//     via Table.Restore. Because they aren't registered on b.registry,
//     InMemoryBackend.Reset resets each of them with an explicit Table.Reset() call
//     alongside b.registry.ResetAll().
//
// The following fields are deliberately plain maps, not store.Table at all:
//
//   - distributionARNs, distributionCallerRefs, oaiCallerRefs, anycastIPListARNs,
//     anycastIPListByName, cachePolicyByName, connectionFunctionARNs,
//     connectionFunctionByName, connectionGroupARNs, connectionGroupByName,
//     connectionGroupByRoutingEndpoint, originAccessControlByName,
//     responseHeadersPolicyByName, originRequestPolicyByName,
//     fieldLevelEncryptionProfileByName,
//     publicKeyByName, keyGroupByName, realtimeLogConfigByName,
//     keyValueStoreByName, trustStoreARNs, trustStoreByName,
//     streamingDistributionARNs, streamingDistributionCallerRefs,
//     distributionTenantARNs, distributionTenantsByDomain: each is a plain
//     string->string secondary lookup index (ARN/CallerReference/name/domain →
//     ID), not a map[string]*T resource collection. Kept as hand-maintained
//     parallel maps (mirroring how they were maintained alongside their primary
//     map before conversion) rather than a store.Index, since an Index groups
//     into a slice (1:many) where these lookups are 1:1 overwrite-on-collision,
//     matching the pre-conversion map semantics exactly (same reasoning as
//     apigateway's apiKeysByValue, commit 6da0334e).
//   - monitoringSubscriptions, resourcePolicies, managedCertificates:
//     MonitoringSubscription / resourcePolicyEntry / ManagedCertificateDetails
//     carry no identity field of their own (the map key -- distribution ID /
//     resource ARN / tenant ID -- is not stored on the value), the same
//     "no identity field" exception EC2's instanceIMDSOptions documents
//     (commit 12e611a4).
//   - distributionAliases, distributionFunctionAssociations: slice-valued
//     (map[string][]T, not map[string]*T), so there is no single *T to key a
//     Table by; each entry is itself a list, not a resource with identity.
//   - distributionWebACLs, distributionTenantWebACLs, distributionCachePolicies,
//     distributionOriginRequestPolicies, distributionResponseHeadersPolicies,
//     distributionRealtimeLogConfigs: plain string->string association maps
//     (distribution/tenant ID → associated resource ID), not resource
//     collections.
//   - keyValueStoreData, keyValueDataETags: KVS ID -> (key -> value) / KVS ID ->
//     ETag. Values have no identity field, and (pre-existing, not introduced by
//     this conversion) neither is persisted today -- see persistence.go.
//   - distSearchTokens, distSearchInverted: the inverted config-token search
//     index; entirely derived from b.distributions and rebuilt from it on
//     Restore (rebuildDistributionSearchIndex), never itself persisted.
//   - invalidationReadyAt, tenantInvalidationReadyAt: ephemeral reconciler-only
//     state (distribution/tenant ID -> invalidation ID -> readyAt time), not
//     persisted before this conversion either.
func registerAllTables(b *InMemoryBackend) {
	for _, register := range tableRegistrations {
		register(b)
	}
}

// tableRegistrations is the data-driven list registerAllTables walks: one closure
// per resource table, each binding its own store.New/store.Register call (plus any
// secondary store.AddIndex) to the concrete field and value type.
//
//nolint:gochecknoglobals // registration table, analogous to errCodeLookup-style lookup tables elsewhere
var tableRegistrations = []func(*InMemoryBackend){
	func(b *InMemoryBackend) {
		b.distributions = store.Register(b.registry, "distributions", store.New(distributionKeyFn))
	},
	func(b *InMemoryBackend) {
		b.oais = store.Register(b.registry, "oais", store.New(oaiKeyFn))
	},
	func(b *InMemoryBackend) {
		b.anycastIPLists = store.Register(b.registry, "anycastIPLists", store.New(anycastIPListKeyFn))
	},
	func(b *InMemoryBackend) {
		b.cachePolicies = store.Register(b.registry, "cachePolicies", store.New(cachePolicyKeyFn))
	},
	func(b *InMemoryBackend) {
		b.connectionFunctions = store.Register(b.registry, "connectionFunctions", store.New(connectionFunctionKeyFn))
	},
	func(b *InMemoryBackend) {
		b.connectionGroups = store.Register(b.registry, "connectionGroups", store.New(connectionGroupKeyFn))
	},
	func(b *InMemoryBackend) {
		b.continuousDeploymentPolicies = store.Register(
			b.registry, "continuousDeploymentPolicies", store.New(continuousDeploymentPolicyKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.originAccessControls = store.Register(b.registry, "originAccessControls", store.New(originAccessControlKeyFn))
	},
	func(b *InMemoryBackend) {
		b.responseHeadersPolicies = store.Register(
			b.registry, "responseHeadersPolicies", store.New(responseHeadersPolicyKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.functions = store.Register(b.registry, "functions", store.New(functionKeyFn))
	},
	func(b *InMemoryBackend) {
		b.originRequestPolicies = store.Register(
			b.registry, "originRequestPolicies", store.New(originRequestPolicyKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.fieldLevelEncryptions = store.Register(
			b.registry, "fieldLevelEncryptions", store.New(fieldLevelEncryptionKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.fieldLevelEncryptionProfiles = store.Register(
			b.registry, "fieldLevelEncryptionProfiles", store.New(fieldLevelEncryptionProfileKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.publicKeys = store.Register(b.registry, "publicKeys", store.New(publicKeyKeyFn))
	},
	func(b *InMemoryBackend) {
		b.keyGroups = store.Register(b.registry, "keyGroups", store.New(keyGroupKeyFn))
	},
	func(b *InMemoryBackend) {
		b.realtimeLogConfigs = store.Register(b.registry, "realtimeLogConfigs", store.New(realtimeLogConfigKeyFn))
	},
	func(b *InMemoryBackend) {
		b.keyValueStores = store.Register(b.registry, "keyValueStores", store.New(keyValueStoreKeyFn))
	},
	func(b *InMemoryBackend) {
		b.vpcOrigins = store.Register(b.registry, "vpcOrigins", store.New(vpcOriginKeyFn))
	},
	func(b *InMemoryBackend) {
		b.trustStores = store.Register(b.registry, "trustStores", store.New(trustStoreKeyFn))
	},
	func(b *InMemoryBackend) {
		b.streamingDistributions = store.Register(
			b.registry, "streamingDistributions", store.New(streamingDistributionKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.distributionTenants = store.Register(b.registry, "distributionTenants", store.New(distributionTenantKeyFn))
	},
	// "Dirty" tables: store.New only, NOT store.Register -- see registerAllTables doc.
	func(b *InMemoryBackend) {
		b.invalidations = store.New(invalidationDistKeyFn)
		b.invalidationsByDist = b.invalidations.AddIndex("byDist", func(v *Invalidation) string { return v.distID })
	},
	func(b *InMemoryBackend) {
		b.tenantInvalidations = store.New(invalidationTenantKeyFn)
		b.tenantInvalidationsByTenant = b.tenantInvalidations.AddIndex(
			"byTenant", func(v *Invalidation) string { return v.tenantID },
		)
	},
}

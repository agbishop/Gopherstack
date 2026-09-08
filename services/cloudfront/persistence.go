package cloudfront

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/persistence"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// cloudfrontSnapshotVersion identifies the shape of backendSnapshot's Tables blob
// (the set of "clean" tables on b.registry plus the DTO tables built below for
// invalidations/tenantInvalidations -- see store_setup.go's registerAllTables doc
// for the clean/dirty split). It must be bumped whenever a change there, to a DTO
// type, or to backendSnapshot itself would make an older snapshot unsafe to decode
// as the current shape. Restore compares this against the persisted value and
// discards (rather than attempts to partially decode) any mismatch -- see Restore
// below. Mirrors the services/sqs pilot (commit 0f09d77c), services/ec2 (commit
// 12e611a4), and services/apigateway (commit 6da0334e).
//
// Do NOT bump this for an additive omitempty field. Restore discards ALL state
// on a version mismatch, so a bump costs every user their persisted snapshot.
// KeyValueStoreData/KeyValueDataETags were added in gopherstack-4ara without a
// bump: a v1 snapshot decodes into them as nil and Restore already seeds both.
const cloudfrontSnapshotVersion = 1

// invalidationSnapshot is the DTO used ONLY for Snapshot/Restore of both
// invalidations and tenantInvalidations. It mirrors Invalidation field for field,
// except ParentID is given a real JSON tag here instead of Invalidation's
// `json:"-"` distID/tenantID (see store.go) -- marshaling Invalidation directly
// would silently drop whichever of those was set, and unmarshaling into it would
// leave both permanently empty, corrupting the flat table's composite key on
// restore. This is the same DTO-registry technique services/apigateway uses for
// its Resource/Deployment/etc. types (commit 6da0334e), itself mirroring the
// services/sqs pilot (commit 0f09d77c). One DTO type serves both tables since
// Invalidation's shape is identical either way; toInvalidationSnapshot/
// fromInvalidationSnapshot take the parent ID explicitly rather than reading a
// field.
type invalidationSnapshot struct {
	CreateTime time.Time `json:"createTime"`
	ID         string    `json:"id"`
	Status     string    `json:"status"`
	CallerRef  string    `json:"callerRef,omitempty"`
	ParentID   string    `json:"parentId"`
	Paths      []string  `json:"paths,omitempty"`
}

func toInvalidationSnapshot(v *Invalidation, parentID string) *invalidationSnapshot {
	return &invalidationSnapshot{
		ID:         v.ID,
		Status:     v.Status,
		CallerRef:  v.CallerRef,
		ParentID:   parentID,
		Paths:      append([]string(nil), v.Paths...),
		CreateTime: v.CreateTime,
	}
}

func fromDistInvalidationSnapshot(v *invalidationSnapshot) *Invalidation {
	return &Invalidation{
		ID:         v.ID,
		Status:     v.Status,
		CallerRef:  v.CallerRef,
		distID:     v.ParentID,
		Paths:      append([]string(nil), v.Paths...),
		CreateTime: v.CreateTime,
	}
}

func fromTenantInvalidationSnapshot(v *invalidationSnapshot) *Invalidation {
	return &Invalidation{
		ID:         v.ID,
		Status:     v.Status,
		CallerRef:  v.CallerRef,
		tenantID:   v.ParentID,
		Paths:      append([]string(nil), v.Paths...),
		CreateTime: v.CreateTime,
	}
}

func invalidationSnapshotKey(v *invalidationSnapshot) string {
	return invalidationKey(v.ParentID, v.ID)
}

// dirtyTableNames lists the "dirty" table names shared by Snapshot and Restore
// (see store_setup.go's registerAllTables doc). Both build an ephemeral DTO
// [store.Registry] under these exact names, so a snapshot written by one version
// of Snapshot always lines up with the same version of Restore.
//
//nolint:gochecknoglobals // fixed lookup table, mirrors errCodeLookup-style tables elsewhere
var dirtyTableNames = struct {
	invalidations, tenantInvalidations string
}{
	invalidations:       "invalidations",
	tenantInvalidations: "tenantInvalidations",
}

// backendSnapshot is the top-level on-disk shape for the CloudFront backend.
//
// Tables holds one JSON-encoded array per table -- both the "clean" tables
// registered on b.registry (produced by [store.Registry.SnapshotAll]) and the
// "dirty" DTO tables built inline in Snapshot (see store_setup.go's
// registerAllTables doc for the clean/dirty split), merged into one map so a
// single Tables blob round-trips the whole backend.
type backendSnapshot struct {
	Tables map[string]json.RawMessage `json:"tables"`

	DistributionFunctionAssociations map[string][]FunctionAssociation `json:"distributionFunctionAssociations,omitempty"`
	DistributionAliases              map[string][]string              `json:"distributionAliases,omitempty"`
	DistributionWebACLs              map[string]string                `json:"distributionWebACLs,omitempty"`
	DistributionTenantWebACLs        map[string]string                `json:"distributionTenantWebACLs,omitempty"`

	MonitoringSubscriptions map[string]*MonitoringSubscription    `json:"monitoringSubscriptions,omitempty"`
	ResourcePolicies        map[string]*resourcePolicyEntry       `json:"resourcePolicies,omitempty"`
	ManagedCertificates     map[string]*ManagedCertificateDetails `json:"managedCertificates,omitempty"`

	DistributionCachePolicies           map[string]string `json:"distributionCachePolicies,omitempty"`
	DistributionOriginRequestPolicies   map[string]string `json:"distributionOriginRequestPolicies,omitempty"`
	DistributionResponseHeadersPolicies map[string]string `json:"distributionResponseHeadersPolicies,omitempty"`
	DistributionRealtimeLogConfigs      map[string]string `json:"distributionRealtimeLogConfigs,omitempty"`

	// KeyValueStoreData/KeyValueDataETags hold the KVS data-plane key/value pairs
	// (KVS ID -> key -> value, and KVS ID -> current data-plane ETag) served by
	// the separate cloudfrontkeyvaluestore service via this backend.
	KeyValueStoreData map[string]map[string]string `json:"keyValueStoreData,omitempty"`
	KeyValueDataETags map[string]string            `json:"keyValueDataETags,omitempty"`

	AccountID string `json:"accountId"`
	Region    string `json:"region"`
	Version   int    `json:"version"`
}

// Snapshot serialises the backend state to JSON.
// It implements persistence.Persistable.
func (b *InMemoryBackend) Snapshot(ctx context.Context) []byte {
	b.mu.RLock("Snapshot")
	defer b.mu.RUnlock()

	tables, err := b.registry.SnapshotAll()
	if err != nil {
		// The registered tables are plain JSON-friendly structs, so a marshal
		// failure here would indicate a programming error rather than bad input
		// data. Log and skip the snapshot rather than panic, matching the
		// persistence.Persistable contract (nil is skipped by the Manager).
		logger.Load(ctx).WarnContext(ctx, "cloudfront: snapshot table marshal failed", "error", err)

		return nil
	}

	dtoReg := store.NewRegistry()
	invalidationDTOs := store.Register(dtoReg, dirtyTableNames.invalidations, store.New(invalidationSnapshotKey))
	tenantInvalidationDTOs := store.Register(
		dtoReg, dirtyTableNames.tenantInvalidations, store.New(invalidationSnapshotKey),
	)

	for _, inv := range b.invalidations.Snapshot() {
		invalidationDTOs.Put(toInvalidationSnapshot(inv, inv.distID))
	}

	for _, inv := range b.tenantInvalidations.Snapshot() {
		tenantInvalidationDTOs.Put(toInvalidationSnapshot(inv, inv.tenantID))
	}

	dirtyTables, err := dtoReg.SnapshotAll()
	if err != nil {
		logger.Load(ctx).WarnContext(ctx, "cloudfront: snapshot DTO table marshal failed", "error", err)

		return nil
	}

	maps.Copy(tables, dirtyTables)

	snap := backendSnapshot{
		Version:                             cloudfrontSnapshotVersion,
		Tables:                              tables,
		DistributionFunctionAssociations:    b.distributionFunctionAssociations,
		DistributionAliases:                 b.distributionAliases,
		DistributionWebACLs:                 b.distributionWebACLs,
		DistributionTenantWebACLs:           b.distributionTenantWebACLs,
		MonitoringSubscriptions:             b.monitoringSubscriptions,
		ResourcePolicies:                    b.resourcePolicies,
		ManagedCertificates:                 b.managedCertificates,
		DistributionCachePolicies:           b.distributionCachePolicies,
		DistributionOriginRequestPolicies:   b.distributionOriginRequestPolicies,
		DistributionResponseHeadersPolicies: b.distributionResponseHeadersPolicies,
		DistributionRealtimeLogConfigs:      b.distributionRealtimeLogConfigs,
		KeyValueStoreData:                   b.keyValueStoreData,
		KeyValueDataETags:                   b.keyValueDataETags,
		AccountID:                           b.accountID,
		Region:                              b.region,
	}

	data, err := json.Marshal(snap)
	if err != nil {
		logger.Load(ctx).WarnContext(ctx, "cloudfront: Snapshot marshal failure", "error", err)

		return nil
	}

	return data
}

// Restore loads backend state from a JSON snapshot and rebuilds derived indexes.
// It implements persistence.Persistable.
func (b *InMemoryBackend) Restore(ctx context.Context, data []byte) error {
	var snap backendSnapshot

	if err := persistence.UnmarshalSnapshot(ctx, "cloudfront", data, &snap); err != nil {
		return err
	}

	b.mu.Lock("Restore")
	defer b.mu.Unlock()

	if snap.Version != cloudfrontSnapshotVersion {
		// An incompatible (older/newer/absent) snapshot version must never be
		// partially decoded as the current shape -- that risks silently
		// misinterpreting fields. Discard cleanly and start empty instead of
		// erroring, since this is an expected, recoverable condition (e.g.
		// upgrading gopherstack across a snapshot-format change), not data
		// corruption. Mirrors the services/sqs pilot (commit 0f09d77c).
		logger.Load(ctx).WarnContext(ctx,
			"cloudfront: discarding incompatible snapshot version, starting empty",
			"gotVersion", snap.Version, "wantVersion", cloudfrontSnapshotVersion)

		b.registry.ResetAll()
		b.invalidations.Reset()
		b.tenantInvalidations.Reset()
		b.resetDistributions()
		b.resetPoliciesAndKeys()
		b.seedManagedPoliciesLocked()
		b.rebuildDistributionSearchIndex()
		b.accountID = snap.AccountID
		b.region = snap.Region

		return nil
	}

	if err := b.registry.RestoreAll(snap.Tables); err != nil {
		return fmt.Errorf("cloudfront: restore snapshot tables: %w", err)
	}

	if err := b.restoreDirtyTables(snap.Tables); err != nil {
		return err
	}

	ensureNonNil(&snap)
	idx := rebuildIndexes(&snap)
	b.restoreAssociationMaps(&snap)
	b.restoreIndexes(&idx)
	// The managed cache/origin-request/response-headers policies always exist in a
	// real AWS account regardless of what a snapshot happened to capture (e.g. a
	// pre-existing snapshot taken before this seeding was added, or a hand-built
	// snapshot in a test). Re-seed defensively: Put on a fixed, deterministic ID is
	// idempotent, so this can never create duplicates for snapshots that already
	// captured them.
	b.seedManagedPoliciesLocked()
	b.rebuildDistributionSearchIndex()
	b.rearmPendingDistributionDeploysLocked()
	b.accountID = snap.AccountID
	b.region = snap.Region

	return nil
}

// restoreDirtyTables loads the invalidations/tenantInvalidations tables (not on
// b.registry -- see store_setup.go's registerAllTables doc) from an ephemeral DTO
// [store.Registry] built from tables, converting each DTO back to a live
// Invalidation. Must be called with the lock held.
func (b *InMemoryBackend) restoreDirtyTables(tables map[string]json.RawMessage) error {
	dtoReg := store.NewRegistry()
	invalidationDTOs := store.Register(dtoReg, dirtyTableNames.invalidations, store.New(invalidationSnapshotKey))
	tenantInvalidationDTOs := store.Register(
		dtoReg, dirtyTableNames.tenantInvalidations, store.New(invalidationSnapshotKey),
	)

	if err := dtoReg.RestoreAll(tables); err != nil {
		return fmt.Errorf("cloudfront: restore snapshot DTO tables: %w", err)
	}

	invs := make([]*Invalidation, 0, invalidationDTOs.Len())
	for _, v := range invalidationDTOs.All() {
		invs = append(invs, fromDistInvalidationSnapshot(v))
	}
	b.invalidations.Restore(invs)

	tenantInvs := make([]*Invalidation, 0, tenantInvalidationDTOs.Len())
	for _, v := range tenantInvalidationDTOs.All() {
		tenantInvs = append(tenantInvs, fromTenantInvalidationSnapshot(v))
	}
	b.tenantInvalidations.Restore(tenantInvs)

	return nil
}

// restoreAssociationMaps assigns the plain (non-store.Table) association maps
// from a snapshot onto the backend. Must be called with the lock held.
func (b *InMemoryBackend) restoreAssociationMaps(snap *backendSnapshot) {
	b.distributionFunctionAssociations = snap.DistributionFunctionAssociations
	b.distributionAliases = snap.DistributionAliases
	b.distributionWebACLs = snap.DistributionWebACLs
	b.distributionTenantWebACLs = snap.DistributionTenantWebACLs
	b.monitoringSubscriptions = snap.MonitoringSubscriptions
	b.resourcePolicies = snap.ResourcePolicies
	b.managedCertificates = snap.ManagedCertificates
	b.distributionCachePolicies = snap.DistributionCachePolicies
	b.distributionOriginRequestPolicies = snap.DistributionOriginRequestPolicies
	b.distributionResponseHeadersPolicies = snap.DistributionResponseHeadersPolicies
	b.distributionRealtimeLogConfigs = snap.DistributionRealtimeLogConfigs
	b.keyValueStoreData = snap.KeyValueStoreData
	b.keyValueDataETags = snap.KeyValueDataETags
}

// backendIndexes holds the derived lookup indexes rebuilt from a snapshot.
type backendIndexes struct {
	distributionARNs                  map[string]string
	distributionCallerRefs            map[string]string
	streamingDistributionARNs         map[string]string
	streamingDistributionCallerRefs   map[string]string
	trustStoreARNs                    map[string]string
	trustStoreByName                  map[string]string
	connectionGroupARNs               map[string]string
	connectionGroupByName             map[string]string
	connectionGroupByRoutingEndpoint  map[string]string
	connectionFunctionARNs            map[string]string
	oaiCallerRefs                     map[string]string
	cachePolicyByName                 map[string]string
	originAccessControlByName         map[string]string
	responseHeadersPolicyByName       map[string]string
	originRequestPolicyByName         map[string]string
	fieldLevelEncryptionProfileByName map[string]string
	publicKeyByName                   map[string]string
	keyGroupByName                    map[string]string
	realtimeLogConfigByName           map[string]string
	keyValueStoreByName               map[string]string
	distributionTenantARNs            map[string]string
	distributionTenantsByDomain       map[string]string
	anycastIPListARNs                 map[string]string
	anycastIPListByName               map[string]string
	connectionFunctionByName          map[string]string
}

// rebuildDistributionIndexes derives the ARN and CallerReference indexes for distributions
// and streaming distributions.
func rebuildDistributionIndexes(
	snap *backendSnapshot,
) (map[string]string, map[string]string, map[string]string, map[string]string) {
	distributions := tableFrom[Distribution](snap, "distributions")
	streamingDistributions := tableFrom[StreamingDistribution](snap, "streamingDistributions")

	arnIndex := make(map[string]string, len(distributions))
	callerRefIndex := make(map[string]string, len(distributions))

	for _, d := range distributions {
		arnIndex[d.ARN] = d.ID
		if d.CallerReference != "" {
			callerRefIndex[d.CallerReference] = d.ID
		}
	}

	sdARNIndex := make(map[string]string, len(streamingDistributions))
	sdCallerRefIndex := make(map[string]string, len(streamingDistributions))

	for _, sd := range streamingDistributions {
		sdARNIndex[sd.ARN] = sd.ID
		if sd.Config.CallerReference != "" {
			sdCallerRefIndex[sd.Config.CallerReference] = sd.ID
		}
	}

	return arnIndex, callerRefIndex, sdARNIndex, sdCallerRefIndex
}

// rebuildTenantIndexes derives the ARN and domain indexes for distribution tenants.
func rebuildTenantIndexes(snap *backendSnapshot) (map[string]string, map[string]string) {
	tenants := tableFrom[DistributionTenant](snap, "distributionTenants")

	tenantARNIndex := make(map[string]string, len(tenants))
	tenantByDomain := make(map[string]string, len(tenants))

	for _, t := range tenants {
		tenantARNIndex[t.ARN] = t.ID
		for _, d := range t.Domains {
			tenantByDomain[d] = t.ID
		}
		if t.Domain != "" {
			tenantByDomain[t.Domain] = t.ID
		}
	}

	return tenantARNIndex, tenantByDomain
}

// rebuildNameIndexes derives the various name → ID lookup indexes for policy- and key-like
// resources that are uniqued by name.
func rebuildNameIndexes(snap *backendSnapshot) backendIndexes {
	cachePolicyByName := make(map[string]string)
	for _, cp := range tableFrom[CachePolicy](snap, "cachePolicies") {
		cachePolicyByName[cp.Name] = cp.ID
	}

	oacByName := make(map[string]string)
	for _, oac := range tableFrom[OriginAccessControl](snap, "originAccessControls") {
		oacByName[oac.Name] = oac.ID
	}

	rhpByName := make(map[string]string)
	for _, p := range tableFrom[ResponseHeadersPolicy](snap, "responseHeadersPolicies") {
		rhpByName[p.Name] = p.ID
	}

	orpByName := make(map[string]string)
	for _, p := range tableFrom[OriginRequestPolicy](snap, "originRequestPolicies") {
		orpByName[p.Name] = p.ID
	}

	// FieldLevelEncryption has no name index to rebuild here -- two configs may share a
	// CallerReference (gopherstack-kpk5), so CreateFieldLevelEncryption's collision check
	// scans fieldLevelEncryptions directly instead of a unique name->ID map
	// (gopherstack-lt9v).
	flePByName := make(map[string]string)
	for _, p := range tableFrom[FieldLevelEncryptionProfile](snap, "fieldLevelEncryptionProfiles") {
		flePByName[p.Name] = p.ID
	}

	pkByName := make(map[string]string)
	for _, pk := range tableFrom[PublicKey](snap, "publicKeys") {
		pkByName[pk.Name] = pk.ID
	}

	kgByName := make(map[string]string)
	for _, kg := range tableFrom[KeyGroup](snap, "keyGroups") {
		kgByName[kg.Name] = kg.ID
	}

	rlcByName := make(map[string]string)
	for _, rlc := range tableFrom[RealtimeLogConfig](snap, "realtimeLogConfigs") {
		rlcByName[rlc.Name] = rlc.ARN
	}

	kvsByName := make(map[string]string)
	for _, kvs := range tableFrom[KeyValueStore](snap, "keyValueStores") {
		kvsByName[kvs.Name] = kvs.ID
	}

	cfnByName := make(map[string]string)
	for _, fn := range tableFrom[ConnectionFunction](snap, "connectionFunctions") {
		cfnByName[fn.Name] = fn.ID
	}

	return backendIndexes{
		cachePolicyByName:                 cachePolicyByName,
		originAccessControlByName:         oacByName,
		responseHeadersPolicyByName:       rhpByName,
		originRequestPolicyByName:         orpByName,
		fieldLevelEncryptionProfileByName: flePByName,
		publicKeyByName:                   pkByName,
		keyGroupByName:                    kgByName,
		realtimeLogConfigByName:           rlcByName,
		keyValueStoreByName:               kvsByName,
		connectionFunctionByName:          cfnByName,
	}
}

// rebuildIndexes derives all secondary indexes from a snapshot.
func rebuildIndexes(snap *backendSnapshot) backendIndexes {
	arnIndex, callerRefIndex, sdARNIndex, sdCallerRefIndex := rebuildDistributionIndexes(snap)

	oaiCallerRefIndex := make(map[string]string)
	for _, oai := range tableFrom[OriginAccessIdentity](snap, "oais") {
		if oai.CallerReference != "" {
			oaiCallerRefIndex[oai.CallerReference] = oai.ID
		}
	}

	trustStoreARNIndex := make(map[string]string)
	trustStoreByName := make(map[string]string)

	for _, ts := range tableFrom[TrustStore](snap, "trustStores") {
		trustStoreARNIndex[ts.ARN] = ts.ID
		trustStoreByName[ts.Name] = ts.ID
	}

	tenantARNIndex, tenantByDomain := rebuildTenantIndexes(snap)
	idx := rebuildNameIndexes(snap)

	connectionGroupARNIndex := make(map[string]string)
	connectionGroupByNameIndex := make(map[string]string)
	connectionGroupByRoutingEndpointIndex := make(map[string]string)

	for _, cg := range tableFrom[ConnectionGroup](snap, "connectionGroups") {
		connectionGroupARNIndex[cg.ARN] = cg.ID
		connectionGroupByNameIndex[cg.Name] = cg.ID
		if cg.RoutingEndpoint != "" {
			connectionGroupByRoutingEndpointIndex[cg.RoutingEndpoint] = cg.ID
		}
	}

	connectionFunctionARNIndex := make(map[string]string)
	for _, fn := range tableFrom[ConnectionFunction](snap, "connectionFunctions") {
		connectionFunctionARNIndex[fn.ARN] = fn.ID
	}

	anycastIPListARNIndex := make(map[string]string)
	anycastIPListByNameIndex := make(map[string]string)
	for _, ail := range tableFrom[AnycastIPList](snap, "anycastIPLists") {
		anycastIPListARNIndex[ail.ARN] = ail.ID
		anycastIPListByNameIndex[ail.Name] = ail.ID
	}

	idx.distributionARNs = arnIndex
	idx.distributionCallerRefs = callerRefIndex
	idx.streamingDistributionARNs = sdARNIndex
	idx.streamingDistributionCallerRefs = sdCallerRefIndex
	idx.trustStoreARNs = trustStoreARNIndex
	idx.trustStoreByName = trustStoreByName
	idx.oaiCallerRefs = oaiCallerRefIndex
	idx.distributionTenantARNs = tenantARNIndex
	idx.distributionTenantsByDomain = tenantByDomain
	idx.connectionGroupARNs = connectionGroupARNIndex
	idx.connectionGroupByName = connectionGroupByNameIndex
	idx.connectionGroupByRoutingEndpoint = connectionGroupByRoutingEndpointIndex
	idx.connectionFunctionARNs = connectionFunctionARNIndex
	idx.anycastIPListARNs = anycastIPListARNIndex
	idx.anycastIPListByName = anycastIPListByNameIndex

	return idx
}

// tableFrom unmarshals the named "clean" table (see store_setup.go's
// registerAllTables doc) out of a snapshot's raw Tables blob, for deriving the
// secondary lookup indexes below. A missing or malformed entry yields an empty
// slice rather than an error: b.registry.RestoreAll (called before
// rebuildIndexes in Restore) is the authoritative loader and has already
// validated/loaded the same data onto the live tables, so this is purely a
// second, read-only pass over already-trusted bytes to derive indexes.
func tableFrom[T any](snap *backendSnapshot, name string) []*T {
	raw, ok := snap.Tables[name]
	if !ok {
		return nil
	}

	var out []*T
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}

	return out
}

// restoreIndexes assigns the derived lookup indexes onto the backend. Must be called with the
// lock held.
func (b *InMemoryBackend) restoreIndexes(idx *backendIndexes) {
	b.distributionARNs = idx.distributionARNs
	b.distributionCallerRefs = idx.distributionCallerRefs
	b.streamingDistributionARNs = idx.streamingDistributionARNs
	b.streamingDistributionCallerRefs = idx.streamingDistributionCallerRefs
	b.trustStoreARNs = idx.trustStoreARNs
	b.trustStoreByName = idx.trustStoreByName
	b.oaiCallerRefs = idx.oaiCallerRefs
	b.cachePolicyByName = idx.cachePolicyByName
	b.originAccessControlByName = idx.originAccessControlByName
	b.responseHeadersPolicyByName = idx.responseHeadersPolicyByName
	b.originRequestPolicyByName = idx.originRequestPolicyByName
	b.fieldLevelEncryptionProfileByName = idx.fieldLevelEncryptionProfileByName
	b.publicKeyByName = idx.publicKeyByName
	b.keyGroupByName = idx.keyGroupByName
	b.realtimeLogConfigByName = idx.realtimeLogConfigByName
	b.keyValueStoreByName = idx.keyValueStoreByName
	b.distributionTenantARNs = idx.distributionTenantARNs
	b.distributionTenantsByDomain = idx.distributionTenantsByDomain
	b.connectionGroupARNs = idx.connectionGroupARNs
	b.connectionGroupByName = idx.connectionGroupByName
	b.connectionGroupByRoutingEndpoint = idx.connectionGroupByRoutingEndpoint
	b.connectionFunctionARNs = idx.connectionFunctionARNs
	b.connectionFunctionByName = idx.connectionFunctionByName
	b.anycastIPListARNs = idx.anycastIPListARNs
	b.anycastIPListByName = idx.anycastIPListByName
}

// ensureNonNil initialises any nil maps in a snapshot to empty maps so that
// the backend never holds nil map references.
func ensureNonNil(snap *backendSnapshot) {
	ensureNonNilTenantExtras(snap)
	ensureNonNilNewResources(snap)
	ensureNonNilDistributionPolicyMaps(snap)

	if snap.DistributionAliases == nil {
		snap.DistributionAliases = make(map[string][]string)
	}

	if snap.DistributionWebACLs == nil {
		snap.DistributionWebACLs = make(map[string]string)
	}

	if snap.DistributionTenantWebACLs == nil {
		snap.DistributionTenantWebACLs = make(map[string]string)
	}
}

// ensureNonNilTenantExtras initialises the per-distribution-tenant and per-distribution maps
// added alongside AnycastIPList/ContinuousDeploymentPolicy/GetManagedCertificateDetails parity work.
func ensureNonNilTenantExtras(snap *backendSnapshot) {
	if snap.MonitoringSubscriptions == nil {
		snap.MonitoringSubscriptions = make(map[string]*MonitoringSubscription)
	}

	if snap.ResourcePolicies == nil {
		snap.ResourcePolicies = make(map[string]*resourcePolicyEntry)
	}

	if snap.ManagedCertificates == nil {
		snap.ManagedCertificates = make(map[string]*ManagedCertificateDetails)
	}
}

func ensureNonNilNewResources(snap *backendSnapshot) {
	if snap.DistributionFunctionAssociations == nil {
		snap.DistributionFunctionAssociations = make(map[string][]FunctionAssociation)
	}
}

// ensureNonNilDistributionPolicyMaps initialises the distribution-to-policy association maps
// (added alongside the ListDistributionsBy{CachePolicy,OriginRequestPolicy,
// ResponseHeadersPolicy,RealtimeLogConfig} parity work) to empty maps when absent from an older
// snapshot.
func ensureNonNilDistributionPolicyMaps(snap *backendSnapshot) {
	if snap.DistributionCachePolicies == nil {
		snap.DistributionCachePolicies = make(map[string]string)
	}

	if snap.DistributionOriginRequestPolicies == nil {
		snap.DistributionOriginRequestPolicies = make(map[string]string)
	}

	if snap.DistributionResponseHeadersPolicies == nil {
		snap.DistributionResponseHeadersPolicies = make(map[string]string)
	}

	if snap.DistributionRealtimeLogConfigs == nil {
		snap.DistributionRealtimeLogConfigs = make(map[string]string)
	}

	if snap.KeyValueStoreData == nil {
		snap.KeyValueStoreData = make(map[string]map[string]string)
	}

	if snap.KeyValueDataETags == nil {
		snap.KeyValueDataETags = make(map[string]string)
	}
}

// Snapshot implements persistence.Persistable by delegating to the backend.
func (h *Handler) Snapshot(ctx context.Context) []byte { return h.Backend.Snapshot(ctx) }

// Restore implements persistence.Persistable by delegating to the backend.
func (h *Handler) Restore(ctx context.Context, data []byte) error {
	return h.Backend.Restore(ctx, data)
}

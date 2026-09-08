package cloudfront

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
	"github.com/blackbirdworks/gopherstack/pkgs/worker"
)

const (
	statusDeployed = "Deployed"
	// kvsStatusReady is the terminal status of a synchronously provisioned KVS.
	kvsStatusReady   = "READY"
	statusInProgress = "InProgress"

	// functionStageDevelopment and functionStageLive are the two stages a CloudFront Function
	// or connection function can be in; Publish promotes DEVELOPMENT to LIVE.
	functionStageDevelopment = "DEVELOPMENT"
	functionStageLive        = "LIVE"

	// maxInvalidationPaths is the AWS limit on paths per invalidation batch.
	maxInvalidationPaths = 3000
	// maxAnycastIPCount bounds the IpCount accepted for an Anycast static IP list. AWS caps
	// the real static IP pool tightly (well under this); the bound here mainly guards
	// generateAnycastIPs' allocation against an unbounded/adversarial IpCount value.
	maxAnycastIPCount = 1000
	// maxCachePolicyTTL is the AWS upper bound for CachePolicy MaxTTL (1 year).
	maxCachePolicyTTL = 31536000
	// minSamplingRate and maxSamplingRate bound RealtimeLogConfig SamplingRate.
	minSamplingRate = 1
	maxSamplingRate = 100
	// minPublicKeyBits is the minimum RSA key size accepted by CloudFront.
	minPublicKeyBits = 2048
	// distributionDeployDelay is the simulated delay before a distribution's
	// async InProgress -> Deployed transition, mirroring
	// services/grafana's workspaceTransitionDelay and
	// services/outposts's orderTransitionDelay.
	distributionDeployDelay = 100 * time.Millisecond
)

const (
	// idChars are the uppercase alphanumeric characters used for CloudFront IDs.
	idChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	// idLen is the length of generated CloudFront IDs.
	idLen = 14
)

// generateID generates a random uppercase alphanumeric ID of length 14.
func generateID() string {
	b := make([]byte, idLen)
	for i := range b {
		b[i] = idChars[rand.IntN(len(idChars))] //nolint:gosec // mock service, not security-sensitive
	}

	return string(b)
}

// InMemoryBackend stores CloudFront resources in memory.
//
// Every map[string]*T resource collection is a *store.Table[T] registered exactly
// once on b.registry (see store_setup.go) instead of a hand-rolled map field, per
// Phase 3.3 of the datalayer refactor (see pkgs/store's package doc and the
// services/ec2 (12e611a4) / services/sqs (0f09d77c) / services/apigateway
// (6da0334e) conversions this follows). store.Table performs no locking of its
// own; b.mu (below) remains the single coarse lock guarding every table exactly
// as it guarded the raw maps it replaces.
type InMemoryBackend struct {
	distributions                     *store.Table[Distribution]
	distributionARNs                  map[string]string          // ARN → distribution ID (O(1) tag lookups)
	distributionCallerRefs            map[string]string          // CallerReference → distribution ID (idempotency)
	distributionAliases               map[string][]string        // distribution ID → aliases
	distributionWebACLs               map[string]string          // distribution ID → web ACL ID
	distributionTenantWebACLs         map[string]string          // tenant ID → web ACL ID
	invalidations                     *store.Table[Invalidation] // composite key: distID + "#" + invID
	invalidationsByDist               *store.Index[Invalidation]
	oais                              *store.Table[OriginAccessIdentity]
	oaiCallerRefs                     map[string]string // CallerReference → OAI ID (idempotency)
	anycastIPLists                    *store.Table[AnycastIPList]
	anycastIPListARNs                 map[string]string // ARN → anycast IP list ID (tag lookups)
	anycastIPListByName               map[string]string // name → anycast IP list ID (uniqueness)
	cachePolicies                     *store.Table[CachePolicy]
	cachePolicyByName                 map[string]string // name → policy ID (uniqueness)
	connectionFunctions               *store.Table[ConnectionFunction]
	connectionFunctionARNs            map[string]string // ARN → connection function ID (tag lookups)
	connectionFunctionByName          map[string]string // name → connection function ID (uniqueness / Identifier lookups)
	connectionGroups                  *store.Table[ConnectionGroup]
	connectionGroupARNs               map[string]string // ARN → connection group ID (tag lookups)
	connectionGroupByName             map[string]string // name → connection group ID (uniqueness)
	connectionGroupByRoutingEndpoint  map[string]string // routing endpoint → connection group ID
	continuousDeploymentPolicies      *store.Table[ContinuousDeploymentPolicy]
	originAccessControls              *store.Table[OriginAccessControl]
	originAccessControlByName         map[string]string // name → OAC ID (uniqueness)
	responseHeadersPolicies           *store.Table[ResponseHeadersPolicy]
	responseHeadersPolicyByName       map[string]string      // name → policy ID (uniqueness)
	functions                         *store.Table[Function] // name → function
	originRequestPolicies             *store.Table[OriginRequestPolicy]
	originRequestPolicyByName         map[string]string // name → policy ID (uniqueness)
	fieldLevelEncryptions             *store.Table[FieldLevelEncryption]
	fieldLevelEncryptionProfiles      *store.Table[FieldLevelEncryptionProfile]
	fieldLevelEncryptionProfileByName map[string]string // name → ID
	publicKeys                        *store.Table[PublicKey]
	publicKeyByName                   map[string]string // name → ID
	keyGroups                         *store.Table[KeyGroup]
	keyGroupByName                    map[string]string               // name → ID
	realtimeLogConfigs                *store.Table[RealtimeLogConfig] // ARN → config
	realtimeLogConfigByName           map[string]string               // name → ARN
	keyValueStores                    *store.Table[KeyValueStore]
	keyValueStoreByName               map[string]string // name → ID
	vpcOrigins                        *store.Table[VpcOrigin]
	distributionFunctionAssociations  map[string][]FunctionAssociation // distribution ID → associations
	// Batch 1 additions.
	trustStores                     *store.Table[TrustStore]
	trustStoreARNs                  map[string]string // ARN → trust store ID (tag lookups)
	trustStoreByName                map[string]string // name → trust store ID (uniqueness)
	streamingDistributions          *store.Table[StreamingDistribution]
	streamingDistributionARNs       map[string]string // ARN → streaming dist ID (tag lookups)
	streamingDistributionCallerRefs map[string]string // CallerRef → streaming dist ID (idempotency)
	// monitoringSubscriptions, resourcePolicies, and managedCertificates are deliberately
	// plain maps, not store.Table: MonitoringSubscription/resourcePolicyEntry/
	// ManagedCertificateDetails carry no identity field of their own (the map key -
	// distribution ID / resource ARN / tenant ID - is not stored on the value), the same
	// "no identity field" exception documented for EC2's instanceIMDSOptions (commit
	// 12e611a4). See store_setup.go's registerAllTables doc for the full list of exceptions.
	monitoringSubscriptions map[string]*MonitoringSubscription // distribution ID → subscription
	resourcePolicies        map[string]*resourcePolicyEntry    // resource ARN → policy
	// managedCertificates maps distribution tenant ID → cached managed cert details.
	managedCertificates map[string]*ManagedCertificateDetails
	// distributionCachePolicies, distributionOriginRequestPolicies,
	// distributionResponseHeadersPolicies, and distributionRealtimeLogConfigs map a
	// distribution ID to the policy/config it is currently associated with, backing the
	// ListDistributionsBy{CachePolicy,OriginRequestPolicy,ResponseHeadersPolicy,RealtimeLogConfig}
	// lookups.
	distributionCachePolicies           map[string]string
	distributionOriginRequestPolicies   map[string]string
	distributionResponseHeadersPolicies map[string]string
	distributionRealtimeLogConfigs      map[string]string
	// Batch 2 additions.
	distributionTenants         *store.Table[DistributionTenant] // key: tenant ID
	distributionTenantARNs      map[string]string                // ARN → tenant ID (tag lookups)
	distributionTenantsByDomain map[string]string                // key: domain → tenant ID
	tenantInvalidations         *store.Table[Invalidation]       // composite key: tenantID + "#" + invID
	tenantInvalidationsByTenant *store.Index[Invalidation]
	// Audit batch additions.
	keyValueStoreData map[string]map[string]string // KVS ID → key → value
	keyValueDataETags map[string]string            // KVS ID → current data-plane ETag
	// distSearchTokens maps a distribution ID to the set of config tokens it
	// contains; distSearchInverted is the inverted index (token → distribution IDs).
	// Together they make ListDistributionsBy* lookups O(k) instead of O(n×config)
	// and eliminate the substring false positives of the previous raw scan.
	distSearchTokens   map[string]map[string]struct{}
	distSearchInverted map[string]map[string]struct{}
	// registry holds every "clean" store.Table above (see store_setup.go) so
	// Reset/Snapshot/Restore collapse to one registry call each. The "dirty"
	// invalidations/tenantInvalidations tables are NOT on registry -- see
	// store_setup.go's registerAllTables doc.
	registry *store.Registry
	mu       *lockmetrics.RWMutex
	// lifecycle: tracks when InProgress invalidations become Completed.
	invalidationReadyAt       map[string]map[string]time.Time // distributionID → invID → readyAt
	tenantInvalidationReadyAt map[string]map[string]time.Time // tenantID → invID → readyAt
	stopCh                    chan struct{}
	// work schedules each distribution's async InProgress -> Deployed
	// transition (distributions.go), the same pkgs/worker idiom
	// services/mgn/exportimport.go and services/outposts's order lifecycle
	// use -- distinct from the older stopCh-based invalidation reconciler
	// above.
	work      *worker.Group
	accountID string
	region    string
}

// NewInMemoryBackend creates a new in-memory CloudFront backend. ctx roots
// the lifetime of its background distribution-deployment timers (see
// distributions.go); it is normally service.AppContext.JanitorCtx, never
// context.Background() in production wiring.
func NewInMemoryBackend(ctx context.Context, accountID, region string) *InMemoryBackend {
	b := &InMemoryBackend{
		work:                                worker.NewGroup(ctx, "cloudfront"),
		distributionARNs:                    make(map[string]string),
		distributionCallerRefs:              make(map[string]string),
		distributionAliases:                 make(map[string][]string),
		distributionWebACLs:                 make(map[string]string),
		distributionTenantWebACLs:           make(map[string]string),
		oaiCallerRefs:                       make(map[string]string),
		anycastIPListARNs:                   make(map[string]string),
		anycastIPListByName:                 make(map[string]string),
		cachePolicyByName:                   make(map[string]string),
		connectionFunctionARNs:              make(map[string]string),
		connectionFunctionByName:            make(map[string]string),
		connectionGroupARNs:                 make(map[string]string),
		connectionGroupByName:               make(map[string]string),
		connectionGroupByRoutingEndpoint:    make(map[string]string),
		originAccessControlByName:           make(map[string]string),
		responseHeadersPolicyByName:         make(map[string]string),
		originRequestPolicyByName:           make(map[string]string),
		fieldLevelEncryptionProfileByName:   make(map[string]string),
		publicKeyByName:                     make(map[string]string),
		keyGroupByName:                      make(map[string]string),
		realtimeLogConfigByName:             make(map[string]string),
		keyValueStoreByName:                 make(map[string]string),
		distSearchTokens:                    make(map[string]map[string]struct{}),
		distSearchInverted:                  make(map[string]map[string]struct{}),
		distributionFunctionAssociations:    make(map[string][]FunctionAssociation),
		trustStoreARNs:                      make(map[string]string),
		trustStoreByName:                    make(map[string]string),
		streamingDistributionARNs:           make(map[string]string),
		streamingDistributionCallerRefs:     make(map[string]string),
		monitoringSubscriptions:             make(map[string]*MonitoringSubscription),
		resourcePolicies:                    make(map[string]*resourcePolicyEntry),
		managedCertificates:                 make(map[string]*ManagedCertificateDetails),
		distributionCachePolicies:           make(map[string]string),
		distributionOriginRequestPolicies:   make(map[string]string),
		distributionResponseHeadersPolicies: make(map[string]string),
		distributionRealtimeLogConfigs:      make(map[string]string),
		distributionTenantARNs:              make(map[string]string),
		distributionTenantsByDomain:         make(map[string]string),
		keyValueStoreData:                   make(map[string]map[string]string),
		keyValueDataETags:                   make(map[string]string),
		invalidationReadyAt:                 make(map[string]map[string]time.Time),
		tenantInvalidationReadyAt:           make(map[string]map[string]time.Time),
		stopCh:                              make(chan struct{}),
		registry:                            store.NewRegistry(),
		mu:                                  lockmetrics.New("cloudfront"),
		accountID:                           accountID,
		region:                              region,
	}

	registerAllTables(b)
	b.seedManagedPoliciesLocked()

	go b.runInvalidationReconciler()

	return b
}

// Close stops the background reconciler goroutine and every scheduled
// distribution-deployment timer.
func (b *InMemoryBackend) Close() {
	select {
	case <-b.stopCh:
	default:
		close(b.stopCh)
	}

	b.work.Stop()
}

// runInvalidationReconciler transitions InProgress invalidations to Completed.
func (b *InMemoryBackend) runInvalidationReconciler() {
	const tick = 20 * time.Millisecond

	timer := time.NewTicker(tick)
	defer timer.Stop()

	for {
		select {
		case <-b.stopCh:
			return
		case <-timer.C:
			b.mu.Lock("invalidationReconciler")
			b.reconcileInvalidationsLocked()
			b.mu.Unlock()
		}
	}
}

// reconcileInvalidationsLocked completes ready invalidations. Must hold b.mu.
func (b *InMemoryBackend) reconcileInvalidationsLocked() {
	now := time.Now()

	for distID, invMap := range b.invalidationReadyAt {
		reconcileInvMap(invMap, b.invalidationsByDist.Get(distID), now)
	}

	for tenantID, invMap := range b.tenantInvalidationReadyAt {
		reconcileInvMap(invMap, b.tenantInvalidationsByTenant.Get(tenantID), now)
	}
}

// reconcileInvMap marks ready InProgress invalidations as Completed and removes them from readyAt.
func reconcileInvMap(invMap map[string]time.Time, invs []*Invalidation, now time.Time) {
	for invID, readyAt := range invMap {
		if !now.After(readyAt) {
			continue
		}

		for _, inv := range invs {
			if inv.ID == invID && inv.Status == statusInProgress {
				inv.Status = "Completed"
			}
		}

		delete(invMap, invID)
	}
}

// Reset clears all stored state, returning the backend to a pristine empty state.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	b.registry.ResetAll()
	// invalidations/tenantInvalidations are "dirty" tables, not on b.registry (see
	// store_setup.go's registerAllTables doc), so they need an explicit Reset call.
	b.invalidations.Reset()
	b.tenantInvalidations.Reset()
	b.invalidationReadyAt = make(map[string]map[string]time.Time)
	b.tenantInvalidationReadyAt = make(map[string]map[string]time.Time)

	b.resetDistributions()
	b.resetPoliciesAndKeys()
	b.seedManagedPoliciesLocked()
}

// resetDistributions clears distribution-related maps not covered by b.registry.ResetAll().
func (b *InMemoryBackend) resetDistributions() {
	b.distSearchTokens = make(map[string]map[string]struct{})
	b.distSearchInverted = make(map[string]map[string]struct{})
	b.distributionARNs = make(map[string]string)
	b.distributionCallerRefs = make(map[string]string)
	b.distributionAliases = make(map[string][]string)
	b.distributionWebACLs = make(map[string]string)
	b.distributionTenantWebACLs = make(map[string]string)
	b.oaiCallerRefs = make(map[string]string)
	b.anycastIPListARNs = make(map[string]string)
	b.anycastIPListByName = make(map[string]string)
	b.cachePolicyByName = make(map[string]string)
	b.connectionFunctionARNs = make(map[string]string)
	b.connectionFunctionByName = make(map[string]string)
	b.connectionGroupARNs = make(map[string]string)
	b.connectionGroupByName = make(map[string]string)
	b.connectionGroupByRoutingEndpoint = make(map[string]string)
	b.originAccessControlByName = make(map[string]string)
	b.responseHeadersPolicyByName = make(map[string]string)
	b.originRequestPolicyByName = make(map[string]string)
	b.distributionFunctionAssociations = make(map[string][]FunctionAssociation)
	b.distributionCachePolicies = make(map[string]string)
	b.distributionOriginRequestPolicies = make(map[string]string)
	b.distributionResponseHeadersPolicies = make(map[string]string)
	b.distributionRealtimeLogConfigs = make(map[string]string)
	b.distributionTenantARNs = make(map[string]string)
	b.distributionTenantsByDomain = make(map[string]string)
}

// resetPoliciesAndKeys clears encryption, key, and store maps not covered by
// b.registry.ResetAll().
func (b *InMemoryBackend) resetPoliciesAndKeys() {
	b.fieldLevelEncryptionProfileByName = make(map[string]string)
	b.publicKeyByName = make(map[string]string)
	b.keyGroupByName = make(map[string]string)
	b.realtimeLogConfigByName = make(map[string]string)
	b.keyValueStoreByName = make(map[string]string)
	b.trustStoreARNs = make(map[string]string)
	b.trustStoreByName = make(map[string]string)
	b.streamingDistributionARNs = make(map[string]string)
	b.streamingDistributionCallerRefs = make(map[string]string)
	b.monitoringSubscriptions = make(map[string]*MonitoringSubscription)
	b.resourcePolicies = make(map[string]*resourcePolicyEntry)
	b.managedCertificates = make(map[string]*ManagedCertificateDetails)
	b.keyValueStoreData = make(map[string]map[string]string)
	b.keyValueDataETags = make(map[string]string)
}

// Region returns the AWS region this backend is configured for.
func (b *InMemoryBackend) Region() string { return b.region }

// AccountID returns the AWS account ID this backend is configured for.
func (b *InMemoryBackend) AccountID() string { return b.accountID }

package cloudfront

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// distributionARN builds an ARN for a CloudFront distribution.
// CloudFront ARNs have no region component.
func (b *InMemoryBackend) distributionARN(id string) string {
	return arn.Build("cloudfront", "", b.accountID, fmt.Sprintf("distribution/%s", id))
}

// CreateDistribution creates a new CloudFront distribution.
//
// Unlike OAI/PublicKey/KeyGroup/FLE-profile, reusing a CallerReference here is
// NOT idempotent even when the new config is byte-identical to the original:
// the real CreateDistribution API docs state that CloudFront returns a
// DistributionAlreadyExists error whenever CallerReference was already used to
// create a distribution, "regardless of the content of the DistributionConfig
// object".
func (b *InMemoryBackend) CreateDistribution(
	callerRef, comment string,
	enabled bool,
	rawConfig []byte,
) (*Distribution, error) {
	b.mu.Lock("CreateDistribution")
	defer b.mu.Unlock()

	if callerRef == "" {
		return nil, fmt.Errorf("%w: CallerReference must not be empty", ErrValidation)
	}

	if _, ok := b.distributionCallerRefs[callerRef]; ok {
		return nil, fmt.Errorf(
			"%w: CallerReference %q is associated with another distribution",
			ErrDistributionAlreadyExists, callerRef,
		)
	}

	id := generateID()
	d := &Distribution{
		ID:               id,
		ARN:              b.distributionARN(id),
		DomainName:       strings.ToLower(id) + ".cloudfront.net",
		Status:           statusDeployed,
		ETag:             uuid.NewString(),
		CallerReference:  callerRef,
		Comment:          comment,
		Enabled:          enabled,
		RawConfig:        rawConfig,
		LastModifiedTime: time.Now().UTC().Format(time.RFC3339),
		Tags:             make(map[string]string),
	}
	b.distributions.Put(d)
	b.distributionARNs[d.ARN] = id
	b.distributionCallerRefs[callerRef] = id
	b.indexDistributionConfig(id, rawConfig)
	cp := b.copyDistribution(d)

	return cp, nil
}

// GetDistribution returns a distribution by ID.
func (b *InMemoryBackend) GetDistribution(id string) (*Distribution, error) {
	b.mu.RLock("GetDistribution")
	defer b.mu.RUnlock()

	d, ok := b.distributions.Get(id)
	if !ok {
		return nil, fmt.Errorf("%w: distribution %s not found", ErrNotFound, id)
	}

	return b.copyDistribution(d), nil
}

// UpdateDistribution updates an existing distribution's config.
func (b *InMemoryBackend) UpdateDistribution(
	id, comment string,
	enabled bool,
	rawConfig []byte,
) (*Distribution, error) {
	b.mu.Lock("UpdateDistribution")
	defer b.mu.Unlock()

	d, ok := b.distributions.Get(id)
	if !ok {
		return nil, fmt.Errorf("%w: distribution %s not found", ErrNotFound, id)
	}

	d.Comment = comment
	d.Enabled = enabled
	d.RawConfig = rawConfig
	d.ETag = uuid.NewString()
	d.Status = statusInProgress
	d.LastModifiedTime = time.Now().UTC().Format(time.RFC3339)
	b.reindexDistributionConfig(id, rawConfig)
	b.scheduleDistributionDeployed(id)
	cp := b.copyDistribution(d)

	return cp, nil
}

// scheduleDistributionDeployed schedules distribution id's async InProgress
// -> Deployed transition, the same pkgs/worker b.work.After idiom
// services/mgn/exportimport.go and services/outposts's order lifecycle use.
// A no-op if the distribution is no longer InProgress by the time the timer
// fires (already re-updated, or deleted) -- mirrors
// services/outposts/orders.go's advanceOrderStatusLocked doc comment.
// Callers may hold b.mu (After only schedules; the callback takes its own
// lock).
func (b *InMemoryBackend) scheduleDistributionDeployed(id string) {
	b.work.After("DistributionDeployed", distributionDeployDelay, func() {
		b.mu.Lock("DistributionDeployed-async")
		defer b.mu.Unlock()

		d, ok := b.distributions.Get(id)
		if !ok || d.Status != statusInProgress {
			return
		}

		d.Status = statusDeployed
		d.LastModifiedTime = time.Now().UTC().Format(time.RFC3339)
	})
}

// rearmPendingDistributionDeploysLocked re-schedules the InProgress ->
// Deployed transition for every distribution Restore just loaded still
// InProgress. A live b.work.After timer never survives a Snapshot/Restore
// round trip (Snapshot only persists Distribution.Status, not in-flight
// timer state), so without this an InProgress distribution restored from a
// snapshot would stay InProgress forever -- unlike a bare process restart,
// where the same timer is still running. Must be called with the lock held.
func (b *InMemoryBackend) rearmPendingDistributionDeploysLocked() {
	for _, d := range b.distributions.All() {
		if d.Status == statusInProgress {
			b.scheduleDistributionDeployed(d.ID)
		}
	}
}

// DeleteDistribution deletes a distribution by ID and cleans up related state.
func (b *InMemoryBackend) DeleteDistribution(id string) error {
	b.mu.Lock("DeleteDistribution")
	defer b.mu.Unlock()

	d, ok := b.distributions.Get(id)
	if !ok {
		return fmt.Errorf("%w: distribution %s not found", ErrNotFound, id)
	}

	if d.Enabled {
		return fmt.Errorf("%w: distribution %s must be disabled before deletion", ErrDistributionNotDisabled, id)
	}

	delete(b.distributionARNs, b.distributionARN(id))
	delete(b.distributionCallerRefs, d.CallerReference)
	b.distributions.Delete(id)
	b.deleteInvalidationsForDist(id)
	delete(b.distributionAliases, id)
	delete(b.distributionWebACLs, id)
	delete(b.distributionFunctionAssociations, id)
	delete(b.monitoringSubscriptions, id)
	b.deindexDistributionConfig(id)

	return nil
}

// ListDistributions returns all distributions sorted by ID.
func (b *InMemoryBackend) ListDistributions() []*Distribution {
	b.mu.RLock("ListDistributions")
	defer b.mu.RUnlock()

	list := make([]*Distribution, 0, b.distributions.Len())
	for _, d := range b.distributions.All() {
		list = append(list, b.copyDistribution(d))
	}

	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })

	return list
}

// ListAliases returns the aliases for a distribution by ID.
func (b *InMemoryBackend) ListAliases(distributionID string) []string {
	b.mu.RLock("ListAliases")
	defer b.mu.RUnlock()

	aliases := b.distributionAliases[distributionID]
	if len(aliases) == 0 {
		return nil
	}

	cp := make([]string, len(aliases))
	copy(cp, aliases)

	return cp
}

// AssociateAlias associates a CNAME alias with the specified distribution.
func (b *InMemoryBackend) AssociateAlias(distributionID, alias string) error {
	b.mu.Lock("AssociateAlias")
	defer b.mu.Unlock()

	if _, ok := b.distributions.Get(distributionID); !ok {
		return fmt.Errorf("%w: distribution %s not found", ErrNotFound, distributionID)
	}

	if alias == "" {
		return fmt.Errorf("%w: alias must not be empty", ErrValidation)
	}

	existing := b.distributionAliases[distributionID]
	if slices.Contains(existing, alias) {
		return nil // already associated, idempotent
	}

	b.distributionAliases[distributionID] = append(existing, alias)

	return nil
}

// AssociateDistributionWebACL associates a WAF web ACL with the specified distribution.
func (b *InMemoryBackend) AssociateDistributionWebACL(distributionID, webACLID string) error {
	b.mu.Lock("AssociateDistributionWebACL")
	defer b.mu.Unlock()

	if _, ok := b.distributions.Get(distributionID); !ok {
		return fmt.Errorf("%w: distribution %s not found", ErrNotFound, distributionID)
	}

	b.distributionWebACLs[distributionID] = webACLID

	return nil
}

// CopyDistribution creates a copy of an existing distribution.
func (b *InMemoryBackend) CopyDistribution(primaryDistID, callerRef string) (*Distribution, error) {
	b.mu.Lock("CopyDistribution")
	defer b.mu.Unlock()

	src, ok := b.distributions.Get(primaryDistID)
	if !ok {
		return nil, fmt.Errorf("%w: distribution %s not found", ErrNotFound, primaryDistID)
	}

	if callerRef == "" {
		return nil, fmt.Errorf("%w: CallerReference must not be empty", ErrValidation)
	}

	if _, exists := b.distributionCallerRefs[callerRef]; exists {
		return nil, fmt.Errorf(
			"%w: CallerReference %q is associated with another distribution",
			ErrDistributionAlreadyExists, callerRef,
		)
	}

	id := generateID()
	rawCopy := make([]byte, len(src.RawConfig))
	copy(rawCopy, src.RawConfig)

	d := &Distribution{
		ID:               id,
		ARN:              b.distributionARN(id),
		DomainName:       strings.ToLower(id) + ".cloudfront.net",
		Status:           statusDeployed,
		ETag:             uuid.NewString(),
		CallerReference:  callerRef,
		Comment:          src.Comment,
		Enabled:          src.Enabled,
		RawConfig:        rawCopy,
		LastModifiedTime: time.Now().UTC().Format(time.RFC3339),
		Tags:             make(map[string]string),
	}

	b.distributions.Put(d)
	b.distributionARNs[d.ARN] = id
	b.distributionCallerRefs[callerRef] = id
	b.indexDistributionConfig(id, rawCopy)

	return b.copyDistribution(d), nil
}

// SetDistributionFunctionAssociations replaces function associations for a distribution.
func (b *InMemoryBackend) SetDistributionFunctionAssociations(
	distributionID string,
	associations []FunctionAssociation,
) error {
	b.mu.Lock("SetDistributionFunctionAssociations")
	defer b.mu.Unlock()

	if _, ok := b.distributions.Get(distributionID); !ok {
		return fmt.Errorf("%w: distribution %s not found", ErrNotFound, distributionID)
	}

	cp := make([]FunctionAssociation, len(associations))
	copy(cp, associations)
	b.distributionFunctionAssociations[distributionID] = cp

	return nil
}

// GetDistributionFunctionAssociations returns function associations for a distribution.
func (b *InMemoryBackend) GetDistributionFunctionAssociations(distributionID string) ([]FunctionAssociation, error) {
	b.mu.RLock("GetDistributionFunctionAssociations")
	defer b.mu.RUnlock()

	if _, ok := b.distributions.Get(distributionID); !ok {
		return nil, fmt.Errorf("%w: distribution %s not found", ErrNotFound, distributionID)
	}

	src := b.distributionFunctionAssociations[distributionID]
	cp := make([]FunctionAssociation, len(src))
	copy(cp, src)

	return cp, nil
}

func (b *InMemoryBackend) copyDistribution(d *Distribution) *Distribution {
	cp := *d
	rawCopy := make([]byte, len(d.RawConfig))
	copy(rawCopy, d.RawConfig)
	cp.RawConfig = rawCopy

	tagsCopy := make(map[string]string, len(d.Tags))
	maps.Copy(tagsCopy, d.Tags)
	cp.Tags = tagsCopy

	return &cp
}

// --- Cache Policy CRUD ---

// DisassociateDistributionWebACL clears the web ACL association for a distribution.
func (b *InMemoryBackend) DisassociateDistributionWebACL(distID string) error {
	b.mu.Lock("DisassociateDistributionWebACL")
	defer b.mu.Unlock()

	if _, ok := b.distributions.Get(distID); !ok {
		return fmt.Errorf("%w: distribution %s not found", ErrNotFound, distID)
	}
	delete(b.distributionWebACLs, distID)

	return nil
}

// UpdateDistributionWithStagingConfig copies the staging distribution's config to the primary.
func (b *InMemoryBackend) UpdateDistributionWithStagingConfig(primaryID, stagingID string) (*Distribution, error) {
	b.mu.Lock("UpdateDistributionWithStagingConfig")
	defer b.mu.Unlock()

	primary, ok := b.distributions.Get(primaryID)
	if !ok {
		return nil, fmt.Errorf("%w: distribution %s not found", ErrNotFound, primaryID)
	}

	staging, ok := b.distributions.Get(stagingID)
	if !ok {
		return nil, fmt.Errorf("%w: staging distribution %s not found", ErrNotFound, stagingID)
	}

	rawCopy := make([]byte, len(staging.RawConfig))
	copy(rawCopy, staging.RawConfig)
	primary.RawConfig = rawCopy
	primary.ETag = uuid.NewString()
	primary.LastModifiedTime = time.Now().UTC().Format(time.RFC3339)
	b.reindexDistributionConfig(primaryID, rawCopy)

	return b.copyDistribution(primary), nil
}

// ListDistributionsByKeyGroup returns distributions that reference a key group.
func (b *InMemoryBackend) ListDistributionsByKeyGroup(keyGroupID string) []*Distribution {
	b.mu.RLock("ListDistributionsByKeyGroup")
	defer b.mu.RUnlock()

	return b.distributionsByConfigSearch(keyGroupID)
}

// ListDistributionsByVpcOriginID returns distributions that reference a VPC origin.
func (b *InMemoryBackend) ListDistributionsByVpcOriginID(vpcOriginID string) []*Distribution {
	b.mu.RLock("ListDistributionsByVpcOriginID")
	defer b.mu.RUnlock()

	return b.distributionsByConfigSearch(vpcOriginID)
}

// ListDistributionsByAnycastIPListID returns distributions that reference an anycast IP list.
func (b *InMemoryBackend) ListDistributionsByAnycastIPListID(anycastID string) []*Distribution {
	b.mu.RLock("ListDistributionsByAnycastIPListID")
	defer b.mu.RUnlock()

	return b.distributionsByConfigSearch(anycastID)
}

// ListDistributionsByConnectionFunction returns distributions that reference a connection function.
func (b *InMemoryBackend) ListDistributionsByConnectionFunction(funcID string) []*Distribution {
	b.mu.RLock("ListDistributionsByConnectionFunction")
	defer b.mu.RUnlock()

	return b.distributionsByConfigSearch(funcID)
}

// ListDistributionsByConnectionMode returns distributions that match a connection mode.
func (b *InMemoryBackend) ListDistributionsByConnectionMode(mode string) []*Distribution {
	b.mu.RLock("ListDistributionsByConnectionMode")
	defer b.mu.RUnlock()

	return b.distributionsByConfigSearch(mode)
}

// ListDistributionsByTrustStore returns distributions that reference a trust store.
func (b *InMemoryBackend) ListDistributionsByTrustStore(trustStoreID string) []*Distribution {
	b.mu.RLock("ListDistributionsByTrustStore")
	defer b.mu.RUnlock()

	return b.distributionsByConfigSearch(trustStoreID)
}

// ListDistributionsByOwnedResource returns distributions that reference an owned resource ARN.
func (b *InMemoryBackend) ListDistributionsByOwnedResource(resourceARN string) []*Distribution {
	b.mu.RLock("ListDistributionsByOwnedResource")
	defer b.mu.RUnlock()

	return b.distributionsByConfigSearch(resourceARN)
}

// ListConflictingAliasesByDomain returns distributions that have a conflicting CNAME alias.
func (b *InMemoryBackend) ListConflictingAliasesByDomain(domain string) []*Distribution {
	b.mu.RLock("ListConflictingAliasesByDomain")
	defer b.mu.RUnlock()

	var out []*Distribution
	for distID, aliases := range b.distributionAliases {
		if slices.Contains(aliases, domain) {
			if d, ok := b.distributions.Get(distID); ok {
				cp := *d
				out = append(out, &cp)
			}
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })

	return out
}

// ListDistributionsByWebACLID returns distribution IDs associated with a web ACL.
func (b *InMemoryBackend) ListDistributionsByWebACLID(webACLID string) []*Distribution {
	b.mu.RLock("ListDistributionsByWebACLID")
	defer b.mu.RUnlock()

	var out []*Distribution
	for distID, wID := range b.distributionWebACLs {
		if wID == webACLID {
			if d, ok := b.distributions.Get(distID); ok {
				cp := *d
				out = append(out, &cp)
			}
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })

	return out
}

// ListDistributionsByCachePolicyID returns distributions whose stored config (default
// cache behavior or any ordered cache behavior) references the given cache policy ID.
func (b *InMemoryBackend) ListDistributionsByCachePolicyID(policyID string) []*Distribution {
	b.mu.RLock("ListDistributionsByCachePolicyID")
	defer b.mu.RUnlock()

	return b.distributionsByConfigSearch(policyID)
}

// ListDistributionsByOriginRequestPolicyID returns distributions whose stored config
// (default or ordered cache behaviors) references the given origin request policy ID.
func (b *InMemoryBackend) ListDistributionsByOriginRequestPolicyID(policyID string) []*Distribution {
	b.mu.RLock("ListDistributionsByOriginRequestPolicyID")
	defer b.mu.RUnlock()

	return b.distributionsByConfigSearch(policyID)
}

// ListDistributionsByResponseHeadersPolicyID returns distributions whose stored config
// (default or ordered cache behaviors) references the given response headers policy ID.
func (b *InMemoryBackend) ListDistributionsByResponseHeadersPolicyID(policyID string) []*Distribution {
	b.mu.RLock("ListDistributionsByResponseHeadersPolicyID")
	defer b.mu.RUnlock()

	return b.distributionsByConfigSearch(policyID)
}

// ListDistributionsByRealtimeLogConfigARN returns distributions whose stored config
// (default or ordered cache behaviors) references the given realtime log config ARN.
func (b *InMemoryBackend) ListDistributionsByRealtimeLogConfigARN(arn string) []*Distribution {
	b.mu.RLock("ListDistributionsByRealtimeLogConfigARN")
	defer b.mu.RUnlock()

	return b.distributionsByConfigSearch(arn)
}

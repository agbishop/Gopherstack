package opensearch

import (
	"slices"
	"time"
)

// clock returns the backend's current time, honouring an injected clock when set.
func (b *InMemoryBackend) clock() time.Time {
	if b.now != nil {
		return b.now()
	}

	return time.Now()
}

// SetProcessingDelay configures how long create/update/upgrade/delete lifecycle
// windows remain observable before settling to their terminal state. A delay of
// 0 (the default) keeps the historical fast behaviour: transitions still run
// through the real code path but settle immediately.
func (b *InMemoryBackend) SetProcessingDelay(d time.Duration) {
	b.mu.Lock("SetProcessingDelay")
	defer b.mu.Unlock()

	b.processingDelay = d
}

// SetClock installs a deterministic clock, primarily for tests. Passing nil
// restores the default time.Now clock.
func (b *InMemoryBackend) SetClock(fn func() time.Time) {
	b.mu.Lock("SetClock")
	defer b.mu.Unlock()

	b.now = fn
}

// beginProcessing opens a processing window on the domain with the given
// DomainProcessingStatus. The caller must hold the write lock.
func (b *InMemoryBackend) beginProcessing(d *Domain, status string) {
	d.ProcessingStatus = status
	d.ProcessingUntil = b.clock().Add(b.processingDelay)
}

// domainProcessing reports the resolved processing / upgrade-processing flags
// and DomainProcessingStatus for a domain copy at the given instant. It never
// mutates stored state: the window naturally expires as the clock advances past
// ProcessingUntil.
func domainProcessing(d *Domain, now time.Time) (bool, bool, string) {
	if d.ProcessingUntil.IsZero() || !now.Before(d.ProcessingUntil) {
		return false, false, domainStatusActive
	}

	if d.ProcessingStatus == dpsUpgrading {
		return true, true, dpsUpgrading
	}

	return true, false, d.ProcessingStatus
}

// deleteWindowElapsed reports whether a domain marked for deletion has passed
// the end of its deleting window and should now be treated as gone.
func deleteWindowElapsed(d *Domain, now time.Time) bool {
	return d.Deleted && !now.Before(d.ProcessingUntil)
}

// statusWindowElapsed reports whether a sub-resource in a transient DELETING
// state has passed the end of its window and should now be treated as removed.
func statusWindowElapsed(status string, until, now time.Time) bool {
	return status == statusDeleting && !until.IsZero() && !now.Before(until)
}

// removeDomainLocked performs the full cascade removal of a domain and all its
// domain-scoped resources. The caller must hold the write lock.
func (b *InMemoryBackend) removeDomainLocked(name string) {
	d, ok := b.domains.Get(name)
	if !ok {
		return
	}

	b.domains.Delete(name)

	if d.Tags != nil {
		d.Tags.Close()
	}

	// Cascade-clean all domain-scoped resources. The byDomain index results
	// are cloned before deleting from the underlying table, since Index.Get
	// returns a slice owned by the index that a concurrent Delete may
	// invalidate mid-range.
	for _, ds := range slices.Clone(b.domainDataSourcesByDomain.Get(name)) {
		b.domainDataSources.Delete(dataSourceKey(ds.DomainName, ds.Name))
	}

	delete(b.vpcAuthorizations, name)

	for pkgID := range b.domainPackages[name] {
		if domains, has := b.packageAssociations[pkgID]; has {
			delete(domains, name)
			if len(domains) == 0 {
				delete(b.packageAssociations, pkgID)
			}
		}
	}
	delete(b.domainPackages, name)

	delete(b.domainMaintenances, name)
	delete(b.scheduledActions, name)
	delete(b.upgradeHistory, upgradeHistoryKey(name))
	b.autoTunes.Delete(autoTuneKey(name))
	b.dryRuns.Delete(name)

	for _, idx := range slices.Clone(b.domainIndexesByDomain.Get(name)) {
		b.domainIndexes.Delete(domainIndexKey(idx.DomainName, idx.IndexName))
	}

	if b.dnsRegistrar != nil {
		b.dnsRegistrar.Deregister(d.Endpoint)
	}

	// Cascade-clean cross-cluster connections owned by this domain: inbound
	// connections where this domain is the destination (LocalDomainInfo), and
	// outbound connections where this domain is the source (LocalDomainInfo).
	// Table.All returns a fresh slice, so deleting while ranging is safe.
	for _, c := range b.inboundConnections.All() {
		if c.LocalDomainInfo.DomainName == name {
			b.inboundConnections.Delete(c.ConnectionID)
		}
	}

	for _, c := range b.outboundConnections.All() {
		if c.LocalDomainInfo.DomainName == name {
			b.outboundConnections.Delete(c.ConnectionID)
		}
	}

	// VPC endpoints are domain-scoped (DomainArn) but not a byDomain-indexed
	// table; DomainArn is deterministic from the domain name (arn.Build), so
	// a recreated domain would otherwise silently inherit stale endpoints.
	for _, ep := range b.vpcEndpoints.All() {
		if ep.DomainArn == d.ARN {
			b.vpcEndpoints.Delete(ep.VpcEndpointID)
		}
	}
}

// purgeExpiredDomainsLocked finalises any domains whose deleting window has
// elapsed, cascading their removal. The caller must hold the write lock.
func (b *InMemoryBackend) purgeExpiredDomainsLocked() {
	now := b.clock()

	// Table.All returns a fresh slice, so cascading removeDomainLocked deletes
	// from the table while ranging over it here is safe.
	for _, d := range b.domains.All() {
		if deleteWindowElapsed(d, now) {
			b.removeDomainLocked(d.Name)
		}
	}
}

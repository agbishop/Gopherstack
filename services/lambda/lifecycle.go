package lambda

import (
	"context"
	"strings"
	"sync"
	"time"
)

// Reset clears all in-memory state from the backend. It is used by the
// POST /_gopherstack/reset endpoint for CI pipelines and rapid local development.
// All active function URL server listeners are shut down before state is cleared
// so ports are released and stale handlers are removed.
func (b *InMemoryBackend) Reset() {
	// Snapshot URL servers and runtimes for shutdown outside the lock.
	var urlServers []*functionURLServer

	var rts []*functionRuntime

	func() {
		b.mu.Lock("Reset")
		defer b.mu.Unlock()

		urlServers = make([]*functionURLServer, 0, len(b.functionURLServers))
		for _, srv := range b.functionURLServers {
			urlServers = append(urlServers, srv)
		}

		rts = make([]*functionRuntime, 0, len(b.runtimes))
		for _, rt := range b.runtimes {
			rts = append(rts, rt)
		}

		// Table/Index-backed resources collapse to two registry sweeps instead of
		// one make() per map -- see store_setup.go for what each registry holds.
		// b.permissions is deliberately NOT on either registry (see store_setup.go
		// and persistence.go's DTO handling) so it is reset explicitly.
		b.registry.ResetAll()
		b.ephemeralRegistry.ResetAll()
		b.permissions.Reset()

		b.versionCounters = make(map[string]int)
		b.versions = make(map[string][]*FunctionVersion)
		b.layers = make(map[string][]*LayerVersion)
		b.layerVersionCounters = make(map[string]int64)
		b.layerPolicies = make(map[string]map[int64]map[string]*LayerVersionStatement)
		b.esmByFunctionARN = make(map[string]map[string]struct{})
		b.versionIndex = make(map[string]map[string]*FunctionVersion)
		b.eventInvokeConfigs = make(map[string]*FunctionEventInvokeConfig)
		b.functionConcurrencies = make(map[string]int)
		b.activeConcurrencies = make(map[string]int)
		b.fisFaults = make(map[string]*FISInvocationFault)
		b.runtimes = make(map[string]*functionRuntime)
		b.functionURLServers = make(map[string]*functionURLServer)
		b.fnCodeSigningConfigs = make(map[string]string)
		b.runtimeManagementConfigs = make(map[string]*RuntimeManagementConfig)
		b.functionRecursionConfigs = make(map[string]*FunctionRecursionConfig)
		b.functionScalingConfigs = make(map[string]*FunctionScalingConfig)
		b.cscIDCounter = 0
		b.durableExecs.reset()

		// Replace semaphore channels so that goroutines launched after Reset() use fresh
		// channels. Goroutines launched before Reset() captured the old channel references
		// (via the RLock capture pattern) and release correctly to those old channels.
		b.cleanupSem = make(chan struct{}, maxCleanupConcurrency)
		b.logSem = make(chan struct{}, maxConcurrentInvocationLogs)
	}()

	// Shut down URL servers and release ports outside the lock.
	ctx, cancel := context.WithTimeout(b.ctx, containerShutdownTimeout)
	defer cancel()

	var wg sync.WaitGroup

	for _, srv := range urlServers {
		wg.Go(func() {
			_ = srv.server.Shutdown(ctx)

			if b.portAlloc != nil {
				_ = b.portAlloc.Release(srv.port)
			}
		})
	}

	for _, rt := range rts {
		wg.Go(func() { b.cleanupRuntime(ctx, rt) })
	}

	wg.Wait()
}

// Purge removes all functions older than the given cutoff time.
func (b *InMemoryBackend) Purge(ctx context.Context, cutoff time.Time) {
	if ctx.Err() != nil {
		return
	}
	purgedFunctions, urlServers, rts := b.collectAndDeleteFunctions(cutoff)

	if len(purgedFunctions) == 0 {
		return
	}

	b.shutdownPurgedResources(urlServers, rts)
}

// collectAndDeleteFunctions removes functions older than cutoff under the lock and returns
// the names, URL servers, and runtimes that need external cleanup.
func (b *InMemoryBackend) collectAndDeleteFunctions(cutoff time.Time) (
	[]string, []*functionURLServer, []*functionRuntime,
) {
	b.mu.Lock("Purge")
	defer b.mu.Unlock()

	var purgedFunctions []string
	var urlServers []*functionURLServer
	var rts []*functionRuntime

	for _, fn := range b.functions.All() {
		if !fn.CreatedAt.Before(cutoff) {
			continue
		}
		name := fn.FunctionName
		purgedFunctions = append(purgedFunctions, name)
		if srv, ok := b.functionURLServers[name]; ok {
			urlServers = append(urlServers, srv)
		}
		if rt, ok := b.runtimes[name]; ok {
			rts = append(rts, rt)
		}
		b.deleteFunctionMapsLocked(name)
	}

	return purgedFunctions, urlServers, rts
}

// deleteFunctionMapsLocked removes all map entries for a function.
// Caller must hold b.mu.
func (b *InMemoryBackend) deleteFunctionMapsLocked(name string) {
	b.functions.Delete(name)
	delete(b.runtimes, name)
	delete(b.functionURLServers, name)
	b.functionURLConfigs.Delete(name)
	b.deleteAliasesForFunctionLocked(name)
	delete(b.versionCounters, name)
	delete(b.versions, name)
	delete(b.eventInvokeConfigs, name)
	delete(b.functionConcurrencies, name)
	delete(b.activeConcurrencies, name)
	b.deleteProvisionedConcurrenciesForFunctionLocked(name)
	delete(b.fisFaults, name)
	// Removes both the unqualified resource-policy and any qualifier-scoped
	// policies (e.g. "name:PROD") for the function. Without this, deleting and
	// recreating a function with the same name would inherit stale
	// qualifier-scoped policy statements — a real leak since permissionMapKey
	// scopes policies per qualifier.
	b.deletePermissionsForFunctionLocked(name)
	delete(b.fnCodeSigningConfigs, name)
	delete(b.runtimeManagementConfigs, name)
	delete(b.functionRecursionConfigs, name)
	delete(b.functionScalingConfigs, name)
	for _, m := range b.eventSourceMappings.All() {
		if strings.HasSuffix(m.FunctionARN, ":function:"+name) {
			if ids, ok := b.esmByFunctionARN[m.FunctionARN]; ok {
				delete(ids, m.UUID)
				if len(ids) == 0 {
					delete(b.esmByFunctionARN, m.FunctionARN)
				}
			}
			b.eventSourceMappings.Delete(m.UUID)
		}
	}
	delete(b.versionIndex, name)
}

// shutdownPurgedResources shuts down URL servers and runtimes outside the lock.
func (b *InMemoryBackend) shutdownPurgedResources(
	urlServers []*functionURLServer,
	rts []*functionRuntime,
) {
	ctx, cancel := context.WithTimeout(b.ctx, containerShutdownTimeout)
	defer cancel()

	var wg sync.WaitGroup

	for _, srv := range urlServers {
		wg.Go(func() {
			_ = srv.server.Shutdown(ctx)
			if b.portAlloc != nil {
				_ = b.portAlloc.Release(srv.port)
			}
		})
	}

	for _, rt := range rts {
		wg.Go(func() { b.cleanupRuntime(ctx, rt) })
	}

	wg.Wait()
}

package lambda

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// defaultEphemeralStorageSize is the default /tmp storage size in MB for Lambda functions.
const defaultEphemeralStorageSize int32 = 512

// minEphemeralStorageSize is the minimum /tmp size in MB accepted by AWS Lambda.
const minEphemeralStorageSize int32 = 512

// maxEphemeralStorageSize is the maximum /tmp size in MB accepted by AWS Lambda.
const maxEphemeralStorageSize int32 = 10240

// CreateFunction stores a new Lambda function configuration.
// validateEphemeralStorage normalises fn.EphemeralStorage, setting the default when nil and
// returning an error when the supplied size is outside the allowed range.
func validateEphemeralStorage(fn *FunctionConfiguration) error {
	if fn.EphemeralStorage == nil {
		fn.EphemeralStorage = &EphemeralStorageConfig{Size: defaultEphemeralStorageSize}

		return nil
	}

	if fn.EphemeralStorage.Size < minEphemeralStorageSize ||
		fn.EphemeralStorage.Size > maxEphemeralStorageSize {
		return fmt.Errorf(
			"%w: EphemeralStorage.Size must be between %d and %d MB",
			ErrInvalidParameterValue, minEphemeralStorageSize, maxEphemeralStorageSize,
		)
	}

	return nil
}

func (b *InMemoryBackend) CreateFunction(fn *FunctionConfiguration) error {
	// AWS rejects function names longer than 64 chars (function name only,
	// not including any qualifier or ARN).
	const maxFunctionNameLength = 64
	if l := len(fn.FunctionName); l == 0 || l > maxFunctionNameLength {
		return fmt.Errorf("%w: FunctionName must be 1-%d characters",
			ErrInvalidParameterValue, maxFunctionNameLength)
	}

	b.mu.Lock("CreateFunction")
	defer b.mu.Unlock()

	if _, exists := b.functions.Get(fn.FunctionName); exists {
		return ErrFunctionAlreadyExists
	}

	if fn.MemorySize != 0 &&
		(fn.MemorySize < 128 || fn.MemorySize > 10240 || fn.MemorySize%64 != 0) {
		return fmt.Errorf(
			"%w: MemorySize must be between 128 and 10240 and divisible by 64",
			ErrInvalidParameterValue,
		)
	}

	if err := validateEphemeralStorage(fn); err != nil {
		return err
	}

	applyFunctionCreateDefaults(fn)

	// AWS Lambda always sets Version to "$LATEST" for the live (mutable) code.
	// Published versions have numbered versions (1, 2, …) in separate records.
	fn.Version = versionLatest

	// Reproduce AWS's Pending→Active creation window when an activation delay is
	// configured. With no delay (the default) the function is created directly
	// Active, matching the near-instant activation of ZIP functions.
	if b.activationDelay > 0 {
		fn.State = FunctionStatePending
		fn.StateReason = "The function is being created."
		fn.StateReasonCode = "Creating"
		b.scheduleFunctionActive(fn.FunctionName, b.activationDelay)
	}

	b.functions.Put(fn)

	return nil
}

// applyFunctionCreateDefaults fills in the AWS default values for optional
// function fields left unset by the caller.
func applyFunctionCreateDefaults(fn *FunctionConfiguration) {
	if fn.Tags == nil {
		fn.Tags = make(map[string]string)
	}

	if len(fn.Architectures) == 0 {
		fn.Architectures = []string{"x86_64"}
	}

	if fn.TracingConfig == nil {
		fn.TracingConfig = &TracingConfig{Mode: "PassThrough"}
	}

	if fn.LoggingConfig == nil {
		fn.LoggingConfig = &LoggingConfig{
			LogFormat: "Text",
			LogGroup:  "/aws/lambda/" + fn.FunctionName,
		}
	}

	if fn.PackageType == "" {
		fn.PackageType = "Zip"
		if fn.ImageURI != "" {
			fn.PackageType = "Image"
		}
	}
}

// scheduleFunctionActive transitions a Pending function to Active after the delay.
func (b *InMemoryBackend) scheduleFunctionActive(name string, delay time.Duration) {
	b.asyncWG.Go(func() {
		timer := time.NewTimer(delay)
		defer timer.Stop()

		select {
		case <-timer.C:
		case <-b.shutdown:
			return
		case <-b.ctx.Done():
			return
		}

		b.mu.Lock("scheduleFunctionActive")
		defer b.mu.Unlock()

		fn, ok := b.functions.Get(name)
		if !ok || fn.State != FunctionStatePending {
			return
		}

		fn.State = FunctionStateActive
		fn.StateReason = ""
		fn.StateReasonCode = ""
	})
}

// GetFunction retrieves a Lambda function configuration by name.
func (b *InMemoryBackend) GetFunction(name string) (*FunctionConfiguration, error) {
	b.mu.RLock("GetFunction")
	defer b.mu.RUnlock()

	name = extractFunctionName(name)
	fn, ok := b.functions.Get(name)
	if !ok {
		return nil, ErrFunctionNotFound
	}

	return fn, nil
}

// GetFunctionByQualifier returns the configuration for a specific qualifier
// (version number, alias name, "$LATEST", or empty for $LATEST).
//
// Matching real AWS GetFunction/GetFunctionConfiguration behaviour:
//   - "" or "$LATEST" returns the live function configuration unchanged.
//   - A numeric version returns the immutable published snapshot, with
//     FunctionArn suffixed ":<version>" and Version set to that number.
//   - An alias name resolves to the alias's primary target version, but the
//     returned FunctionArn is suffixed with the alias name (":<alias>") — AWS
//     echoes the qualifier you asked for in the ARN while reporting the
//     resolved Version. Weighted routing config does NOT affect GetFunction.
//
// Returns ErrFunctionNotFound when the function does not exist and
// ErrVersionNotFound when the qualifier resolves to no known version/alias.
func (b *InMemoryBackend) GetFunctionByQualifier(
	name, qualifier string,
) (*FunctionConfiguration, error) {
	if qualifier == "" || qualifier == versionLatest {
		return b.GetFunction(name)
	}

	b.mu.RLock("GetFunctionByQualifier")
	defer b.mu.RUnlock()

	if _, ok := b.functions.Get(name); !ok {
		return nil, ErrFunctionNotFound
	}

	// Resolve an alias qualifier to its primary target version, but remember the
	// alias name so the returned ARN carries the alias suffix (AWS behaviour).
	resolved := qualifier
	aliasSuffix := ""

	if alias, ok := b.aliases.Get(aliasKey(name, qualifier)); ok {
		resolved = alias.FunctionVersion
		aliasSuffix = qualifier
	}

	if resolved == versionLatest {
		// Alias points at $LATEST: return the live config but with the alias ARN.
		fn, _ := b.functions.Get(name)
		cfg := versionToConfig(fnToVersion(fn))
		cfg.FunctionArn = buildVersionARN(b.region, b.accountID, name, aliasSuffix)

		return cfg, nil
	}

	vMap := b.versionIndex[name]
	if vMap == nil {
		return nil, ErrVersionNotFound
	}

	v, ok := vMap[resolved]
	if !ok {
		return nil, ErrVersionNotFound
	}

	cfg := versionToConfig(v)

	// For an alias qualifier, AWS returns the ARN with the alias suffix while
	// the Version field reports the resolved numeric version.
	if aliasSuffix != "" {
		cfg.FunctionArn = buildVersionARN(b.region, b.accountID, name, aliasSuffix)
	}

	return cfg, nil
}

// ListFunctions returns a page of Lambda function configurations sorted by name.
func (b *InMemoryBackend) ListFunctions(
	marker string,
	maxItems int,
) page.Page[*FunctionConfiguration] {
	b.mu.RLock("ListFunctions")
	defer b.mu.RUnlock()

	fns := b.functions.All()

	sort.Slice(fns, func(i, j int) bool {
		return fns[i].FunctionName < fns[j].FunctionName
	})

	return page.New(fns, marker, maxItems, lambdaDefaultMaxItems)
}

// ListFunctionsAll returns a page of all published versions across all functions,
// sorted by FunctionName then numerically by version. This is the response for
// ListFunctions?FunctionVersion=ALL.
func (b *InMemoryBackend) ListFunctionsAll(
	marker string,
	maxItems int,
) page.Page[*FunctionConfiguration] {
	b.mu.RLock("ListFunctionsAll")
	defer b.mu.RUnlock()

	// Include $LATEST for each function.
	fns := b.functions.All()

	// Include all published versions.
	for name, vMap := range b.versionIndex {
		for _, v := range vMap {
			cfg := versionToConfig(v)
			cfg.FunctionName = name
			fns = append(fns, cfg)
		}
	}

	// Sort by FunctionName, then by Version (numerically: $LATEST sorts last).
	sort.Slice(fns, func(i, j int) bool {
		if fns[i].FunctionName != fns[j].FunctionName {
			return fns[i].FunctionName < fns[j].FunctionName
		}
		// $LATEST > any number
		if fns[i].Version == versionLatest {
			return false
		}
		if fns[j].Version == versionLatest {
			return true
		}
		// Both are version numbers — compare numerically.
		ni, _ := strconv.Atoi(fns[i].Version)
		nj, _ := strconv.Atoi(fns[j].Version)

		return ni < nj
	})

	return page.New(fns, marker, maxItems, lambdaDefaultMaxItems)
}

// DeleteFunction removes a Lambda function and cleans up its runtime server.
func (b *InMemoryBackend) DeleteFunction(name string) error {
	var (
		found          bool
		rt             *functionRuntime
		esmIDsToRemove []string
	)

	func() {
		b.mu.Lock("DeleteFunction")
		defer b.mu.Unlock()

		if _, ok := b.functions.Get(name); !ok {
			return
		}

		found = true
		rt = b.runtimes[name]

		// Capture ESM IDs before deleteFunctionMapsLocked deletes them, so the
		// kinesisPoller cascade below still runs.
		fnARN := arn.Build("lambda", b.region, b.accountID, "function:"+name)
		if ids, ok := b.esmByFunctionARN[fnARN]; ok {
			for id := range ids {
				esmIDsToRemove = append(esmIDsToRemove, id)
			}
		}

		b.deleteFunctionMapsLocked(name)
	}()

	if !found {
		return ErrFunctionNotFound
	}

	for _, id := range esmIDsToRemove {
		if b.kinesisPoller != nil {
			b.kinesisPoller.RemoveMapping(id)
		}
	}

	// Clean up runtime resources; must not hold b.mu while stopping the server.
	if rt != nil {
		shutdownCtx, cancel := context.WithTimeout(b.ctx, containerShutdownTimeout)
		defer cancel()
		b.cleanupRuntime(shutdownCtx, rt)
	}

	return nil
}

// DeleteFunctionVersion deletes a single published version of a function,
// leaving $LATEST, every other version, and every alias that doesn't
// reference this version intact. This is DeleteFunctionInput's real
// query-bound Qualifier member (lambda@v1.101.2 serializers.go
// awsRestjson1_serializeOpHttpBindingsDeleteFunctionInput) -- api_op_DeleteFunction.go's
// doc comment: "To delete a specific function version, use the Qualifier
// parameter. Otherwise, all versions and aliases are deleted", and you can't
// delete a version that an alias references.
func (b *InMemoryBackend) DeleteFunctionVersion(name, qualifier string) error {
	if qualifier == "" {
		return b.DeleteFunction(name)
	}

	if qualifier == versionLatest {
		return ErrInvalidParameterValue
	}

	b.mu.Lock("DeleteFunctionVersion")
	defer b.mu.Unlock()

	if _, ok := b.functions.Get(name); !ok {
		return ErrFunctionNotFound
	}

	vMap := b.versionIndex[name]
	if vMap == nil {
		return ErrVersionNotFound
	}
	if _, ok := vMap[qualifier]; !ok {
		return ErrVersionNotFound
	}

	for _, a := range b.aliasesByFunction.Get(name) {
		if a.FunctionVersion == qualifier {
			return ErrVersionReferencedByAlias
		}
	}

	delete(vMap, qualifier)

	kept := b.versions[name][:0]
	for _, v := range b.versions[name] {
		if v.Version != qualifier {
			kept = append(kept, v)
		}
	}
	b.versions[name] = kept

	return nil
}

// UpdateFunction replaces a Lambda function's configuration.
// Any running container is evicted so the next invocation picks up the new code/config.
func (b *InMemoryBackend) UpdateFunction(fn *FunctionConfiguration) error {
	var (
		found bool
		rt    *functionRuntime
	)

	func() {
		b.mu.Lock("UpdateFunction")
		defer b.mu.Unlock()

		if _, ok := b.functions.Get(fn.FunctionName); !ok {
			return
		}

		found = true

		b.functions.Put(fn)

		// Evict the running runtime so the next invocation gets a fresh container with the
		// updated code or configuration (mirrors AWS/LocalStack behaviour).
		rt = b.runtimes[fn.FunctionName]
		if rt != nil {
			delete(b.runtimes, fn.FunctionName)
		}
	}()

	if !found {
		return ErrFunctionNotFound
	}

	// Clean up the old container asynchronously — we must not hold b.mu while stopping.
	// rt is passed as a parameter to make the capture explicit and safe against future refactoring.
	if rt != nil {
		// Capture sem under RLock so that a concurrent Reset() cannot replace b.cleanupSem
		// between the send and the goroutine's deferred release.
		var sem chan struct{}

		func() {
			b.mu.RLock("cleanupSem.updateFn")
			defer b.mu.RUnlock()

			sem = b.cleanupSem
		}()

		select {
		case sem <- struct{}{}:
			go func(evicted *functionRuntime) { // #nosec G118 -- intentional detached context for background cleanup
				defer func() { <-sem }()
				shutdownCtx, cancel := context.WithTimeout(
					b.ctx,
					containerShutdownTimeout,
				)
				defer cancel()
				b.cleanupRuntime(shutdownCtx, evicted)
			}(
				rt,
			)
		default:
			// Already at max concurrent cleanups; run inline (rare, only under extreme load).
			shutdownCtx, cancel := context.WithTimeout(
				b.ctx,
				containerShutdownTimeout,
			)
			defer cancel()
			b.cleanupRuntime(shutdownCtx, rt)
		}
	}

	return nil
}

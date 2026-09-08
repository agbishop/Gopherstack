package lambda

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/container"
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
)

// maxCleanupConcurrency is the maximum number of concurrent runtime cleanup goroutines.
const maxCleanupConcurrency = 64

const extractParentDirPerm = 0o750

// functionRuntime holds the runtime server and startup state for a single Lambda function.
//
// Locking discipline:
//   - lastUsed is read and written while b.mu is held (write lock); it does not require rt.mu.
//   - started, startErr, srv, port, zipDir, layerDirs, and containerID are protected by rt.mu.
//     They are set once during startup and read afterwards without b.mu held.
type functionRuntime struct {
	lastUsed    time.Time
	startErr    error
	srv         *runtimeServer
	mu          *lockmetrics.RWMutex
	zipDir      string
	containerID string
	layerDirs   []string
	port        int
	started     bool
}

// containerShutdownTimeout is the maximum time to wait for a container to stop.
const containerShutdownTimeout = 5 * time.Second

// evictLRURuntimeLocked removes the least-recently-used runtime entry (other than
// skipName) when the runtimes map exceeds the configured MaxRuntimes limit.
// It returns the evicted entry so the caller can release its resources outside the lock.
// The early-return on len(b.runtimes) <= maxRuntimes keeps the common (under-limit) path
// cost to a single comparison, so callers do not need to guard the call site.
// Must be called with b.mu held (write lock).
func (b *InMemoryBackend) evictLRURuntimeLocked(skipName string) *functionRuntime {
	maxRuntimes := b.settings.MaxRuntimes
	if maxRuntimes <= 0 {
		maxRuntimes = defaultMaxRuntimes
	}

	if len(b.runtimes) <= maxRuntimes {
		return nil
	}

	var (
		lruName string
		lruRT   *functionRuntime
	)

	for name, rt := range b.runtimes {
		if name == skipName {
			continue
		}

		if lruRT == nil || rt.lastUsed.Before(lruRT.lastUsed) {
			lruName = name
			lruRT = rt
		}
	}

	if lruRT != nil {
		delete(b.runtimes, lruName)
	}

	return lruRT
}

// cleanupRuntime stops the container, shuts down the runtime server, releases the port,
// and removes any temp directories associated with rt. It is safe to call with a nil rt.
// Fields are snapshotted under rt.mu to avoid data races with concurrent startup.
func (b *InMemoryBackend) cleanupRuntime(ctx context.Context, rt *functionRuntime) {
	if rt == nil {
		return
	}

	// Snapshot all resource handles under rt.mu so we don't race with getOrCreateRuntime
	// which writes containerID, srv, port, zipDir, and layerDirs under rt.mu.
	var (
		containerID string
		srv         *runtimeServer
		port        int
		zipDir      string
		layerDirs   []string
	)

	func() {
		rt.mu.Lock("cleanupRuntime")
		defer rt.mu.Unlock()

		containerID = rt.containerID
		srv = rt.srv
		port = rt.port
		zipDir = rt.zipDir
		layerDirs = rt.layerDirs
	}()

	shutdownCtx, cancel := context.WithTimeout(ctx, containerShutdownTimeout)
	defer cancel()

	defer func() {
		// Stop and remove the container
		if containerID != "" && b.docker != nil {
			if !b.settings.KeepContainers {
				_ = b.docker.StopAndRemove(ctx, containerID)
			}
		}
	}()

	if srv != nil {
		srv.stop(shutdownCtx)
	}

	if port > 0 && b.portAlloc != nil {
		_ = b.portAlloc.Release(port)
	}

	if zipDir != "" {
		_ = os.RemoveAll(zipDir) // #nosec G703
	}

	for _, d := range layerDirs {
		_ = os.RemoveAll(d) // #nosec G703
	}

	rt.mu.Close()
}

// cleanupTimedOutRuntime removes the named runtime from the runtimes map and
// asynchronously stops its container and releases its resources. It is called when
// an invocation times out so that the next invocation creates a fresh container.
func (b *InMemoryBackend) cleanupTimedOutRuntime(functionName string) {
	var rt *functionRuntime

	func() {
		b.mu.Lock("cleanupTimedOutRuntime")
		defer b.mu.Unlock()

		rt = b.runtimes[functionName]
		delete(b.runtimes, functionName)
	}()

	if rt == nil {
		return
	}

	var sem chan struct{}

	func() {
		b.mu.RLock("cleanupSem.timedOut")
		defer b.mu.RUnlock()

		sem = b.cleanupSem
	}()

	select {
	case sem <- struct{}{}:
		go func() {
			defer func() { <-sem }()
			ctx, cancel := context.WithTimeout(b.ctx, containerShutdownTimeout)
			defer cancel()
			b.cleanupRuntime(ctx, rt)
		}()
	default:
		// cleanupSem is full; run inline to avoid leaking the timed-out runtime's
		// container/port/temp dirs (rt was already removed from b.runtimes above,
		// so nothing else will ever revisit it for cleanup).
		ctx, cancel := context.WithTimeout(b.ctx, containerShutdownTimeout)
		defer cancel()
		b.cleanupRuntime(ctx, rt)
	}
}

// getOrCreateRuntime returns the runtime server for a function, creating it on first use.
// Must not be called with b.mu held.
func (b *InMemoryBackend) getOrCreateRuntime(
	ctx context.Context,
	fn *FunctionConfiguration,
) (*runtimeServer, error) {
	rt, initErr := b.lookupOrRegisterRuntime(fn)
	if initErr != nil {
		return nil, initErr
	}

	// rt.mu guards the whole startup sequence below (not just field access) so
	// concurrent callers for the same function serialize on it. lockHeld
	// tracks whether rt.mu is still our responsibility to release: it is
	// cleared just before handing off to handleContainerStartFailure, which
	// takes over unlocking rt.mu itself.
	rt.mu.Lock("getOrCreateRuntime")

	lockHeld := true

	defer func() {
		if lockHeld {
			rt.mu.Unlock()
		}
	}()

	if rt.started {
		return rt.srv, rt.startErr
	}

	port, portErr := b.portAlloc.Acquire("lambda:" + fn.FunctionName)
	if portErr != nil {
		rt.startErr = fmt.Errorf("%w: port allocation failed: %w", ErrLambdaUnavailable, portErr)
		rt.started = true

		return nil, rt.startErr
	}

	srv := newRuntimeServer(port)

	if startErr := srv.start(ctx); startErr != nil {
		_ = b.portAlloc.Release(port)
		rt.startErr = fmt.Errorf(
			"%w: runtime server start failed: %w",
			ErrLambdaUnavailable,
			startErr,
		)
		rt.started = true

		return nil, rt.startErr
	}

	rt.srv = srv
	rt.port = port
	rt.started = true

	zipDir, layerDirs, containerID, containerErr := b.startContainer(ctx, fn, port)
	if containerErr != nil {
		// handleContainerStartFailure receives rt.mu already locked and releases
		// it itself once its cleanup completes, before doing further b.mu work.
		lockHeld = false
		startErr := b.handleContainerStartFailure(
			ctx, fn.FunctionName, rt, srv, port, zipDir, layerDirs, containerID, containerErr,
		)

		return nil, startErr
	}

	rt.zipDir = zipDir
	rt.layerDirs = layerDirs
	rt.containerID = containerID

	return srv, nil
}

// lookupOrRegisterRuntime returns the existing runtime entry for fn, or
// registers a fresh one when none exists yet. On the creation path it also
// evicts the LRU runtime (if the backend is over its configured limit) and
// cleans up the evicted entry asynchronously outside b.mu.
func (b *InMemoryBackend) lookupOrRegisterRuntime(fn *FunctionConfiguration) (*functionRuntime, error) {
	var (
		rt      *functionRuntime
		initErr error
		evicted *functionRuntime
	)

	func() {
		b.mu.Lock("getOrCreateRuntime")
		defer b.mu.Unlock()

		var ok bool
		rt, ok = b.runtimes[fn.FunctionName]

		if ok {
			// Touch lastUsed under the lock so concurrent callers see a consistent value.
			rt.lastUsed = time.Now()

			return
		}

		// Only check for required infrastructure when actually creating a new runtime.
		if b.portAlloc == nil {
			initErr = fmt.Errorf("%w: no port range configured", ErrLambdaUnavailable)

			return
		}

		if b.docker == nil {
			initErr = fmt.Errorf("%w: container runtime unavailable", ErrLambdaUnavailable)

			return
		}

		rt = &functionRuntime{mu: lockmetrics.New("lambda.runtime"), lastUsed: time.Now()}
		b.runtimes[fn.FunctionName] = rt
		evicted = b.evictLRURuntimeLocked(fn.FunctionName)
	}()

	if initErr != nil {
		return nil, initErr
	}

	// Clean up the evicted runtime asynchronously outside b.mu.
	if evicted != nil {
		// Capture sem under RLock so that a concurrent Reset() cannot replace
		// b.cleanupSem between the send and the goroutine's deferred release.
		var sem chan struct{}

		func() {
			b.mu.RLock("cleanupSem.evict")
			defer b.mu.RUnlock()

			sem = b.cleanupSem
		}()

		select {
		case sem <- struct{}{}:
			go func(rt *functionRuntime) { // #nosec G118 -- intentional detached cleanup goroutine
				defer func() { <-sem }()
				cleanupCtx, cancel := context.WithTimeout(b.ctx, containerShutdownTimeout)
				defer cancel()
				b.cleanupRuntime(cleanupCtx, rt)
			}(evicted)
		default:
			// cleanupSem is full; run inline to avoid leaking the evicted runtime's resources.
			cleanupCtx, cancel := context.WithTimeout(b.ctx, containerShutdownTimeout)
			defer cancel()
			b.cleanupRuntime(cleanupCtx, evicted)
		}
	}

	return rt, nil
}

// handleContainerStartFailure cleans up all resources after startContainer returns an error.
// It stops the runtime server, releases the port, removes any partial container and temp dirs,
// sets rt.startErr, unlocks rt.mu, and then removes the stale map entry under b.mu so that
// the next invocation can retry. This helper exists to keep getOrCreateRuntime within the
// statement-count limit.
//
// The caller must hold rt.mu when calling this function; it releases rt.mu itself
// (before acquiring b.mu, to keep lock ordering safe) partway through.
func (b *InMemoryBackend) handleContainerStartFailure(
	ctx context.Context,
	functionName string,
	rt *functionRuntime,
	srv *runtimeServer,
	port int,
	zipDir string,
	layerDirs []string,
	containerID string,
	containerErr error,
) error {
	var startErr error

	func() {
		// rt.mu was locked by the caller; this closure takes over responsibility
		// for releasing it once the cleanup below completes.
		defer rt.mu.Unlock()

		// Container startup failure is fatal: stop the runtime server, release the
		// port, and surface the error so the caller gets an immediate failure instead
		// of silently timing out on every subsequent invoke.
		shutdownCtx, cancel := context.WithTimeout(ctx, containerShutdownTimeout)
		defer cancel()
		srv.stop(shutdownCtx)
		_ = b.portAlloc.Release(port)

		// Stop any container that was created before the error occurred.
		if containerID != "" && b.docker != nil {
			if !b.settings.KeepContainers {
				_ = b.docker.StopAndRemove(ctx, containerID)
			}
		}

		for _, d := range layerDirs {
			_ = os.RemoveAll(d) // #nosec G703
		}

		if zipDir != "" {
			_ = os.RemoveAll(zipDir) // #nosec G703
		}

		startErr = fmt.Errorf("%w: container startup failed: %w", ErrLambdaUnavailable, containerErr)
		rt.startErr = startErr
	}()

	// Remove the stale entry so the next invocation gets a fresh attempt rather
	// than perpetually returning this error. Lock ordering is safe: rt.mu was
	// released above before acquiring b.mu.
	func() {
		b.mu.Lock("getOrCreateRuntime-evict-failed")
		defer b.mu.Unlock()

		if b.runtimes[functionName] == rt {
			delete(b.runtimes, functionName)
		}
	}()

	return startErr
}

// runtimeImageForRuntime maps a Lambda runtime identifier to the corresponding
// AWS public ECR base image reference.
//
//nolint:gochecknoglobals // intentional package-level lookup table
var runtimeBaseImages = map[string]string{
	"python3.13":      "public.ecr.aws/lambda/python:3.13",
	"python3.12":      "public.ecr.aws/lambda/python:3.12",
	"python3.11":      "public.ecr.aws/lambda/python:3.11",
	"python3.10":      "public.ecr.aws/lambda/python:3.10",
	"python3.9":       "public.ecr.aws/lambda/python:3.9",
	"nodejs22.x":      "public.ecr.aws/lambda/nodejs:22",
	"nodejs20.x":      "public.ecr.aws/lambda/nodejs:20",
	"nodejs18.x":      "public.ecr.aws/lambda/nodejs:18",
	"java21":          "public.ecr.aws/lambda/java:21",
	"java17":          "public.ecr.aws/lambda/java:17",
	"java11":          "public.ecr.aws/lambda/java:11",
	"dotnet9":         "public.ecr.aws/lambda/dotnet:9",
	"dotnet8":         "public.ecr.aws/lambda/dotnet:8",
	"ruby3.3":         "public.ecr.aws/lambda/ruby:3.3",
	"ruby3.2":         "public.ecr.aws/lambda/ruby:3.2",
	"provided.al2023": "public.ecr.aws/lambda/provided:al2023",
	"provided.al2":    "public.ecr.aws/lambda/provided:al2",
	"provided":        "public.ecr.aws/lambda/provided:alami",
}

// baseImageForRuntime returns the ECR base image for the given runtime string.
// Returns "" if the runtime is unknown.
func baseImageForRuntime(runtime string) string {
	return runtimeBaseImages[runtime]
}

// deprecatedRuntimes are AWS Lambda runtime identifiers that are valid (AWS
// recognises them and CreateFunction is accepted) but are past their
// deprecation date and can no longer be executed. We accept them at the
// control plane for parity — real AWS returns InvalidParameterValueException
// only for runtimes it has never heard of, not for deprecated ones — but they
// are deliberately absent from runtimeBaseImages so the Docker run path treats
// them as unknown.
//
//nolint:gochecknoglobals // intentional package-level lookup set
var deprecatedRuntimes = map[string]struct{}{
	"nodejs":         {},
	"nodejs4.3":      {},
	"nodejs4.3-edge": {},
	"nodejs6.10":     {},
	"nodejs8.10":     {},
	"nodejs10.x":     {},
	"nodejs12.x":     {},
	"nodejs14.x":     {},
	"nodejs16.x":     {},
	"python2.7":      {},
	"python3.6":      {},
	"python3.7":      {},
	"python3.8":      {},
	"java8":          {},
	"java8.al2":      {},
	"dotnetcore1.0":  {},
	"dotnetcore2.0":  {},
	"dotnetcore2.1":  {},
	"dotnetcore3.1":  {},
	"dotnet5.0":      {},
	"dotnet6":        {},
	"dotnet7":        {},
	"go1.x":          {},
	"ruby2.5":        {},
	"ruby2.7":        {},
	"provided.al2":   {}, // still runnable, but listed here as a safety net
}

// isValidRuntime reports whether a runtime identifier is one AWS Lambda
// recognises — either a currently runnable runtime (runtimeBaseImages) or a
// known-but-deprecated one. Unknown identifiers are rejected by CreateFunction
// with InvalidParameterValueException, matching real AWS, which enforces an
// enum constraint on the 'runtime' member.
func isValidRuntime(runtime string) bool {
	if _, ok := runtimeBaseImages[runtime]; ok {
		return true
	}

	_, ok := deprecatedRuntimes[runtime]

	return ok
}

// extractZip extracts zip bytes into a new temporary directory and returns the directory path.
// The caller is responsible for calling [os.RemoveAll] on the returned path when done.
func extractZip(zipData []byte) (string, error) {
	dir, err := os.MkdirTemp("", "gopherstack-lambda-zip-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}

	r, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		_ = os.RemoveAll(dir)

		return "", fmt.Errorf("open zip: %w", err)
	}

	for _, f := range r.File {
		if extractErr := extractZipFile(dir, f); extractErr != nil {
			_ = os.RemoveAll(dir)

			return "", extractErr
		}
	}

	return dir, nil
}

// extractZipFile extracts a single [zip.File] entry into destDir.
func extractZipFile(destDir string, f *zip.File) error {
	cleanDest := filepath.Clean(destDir)
	cleanName := filepath.Clean(f.Name)
	if cleanName == "" || cleanName == "." {
		return nil // skip empty / current-dir entries
	}

	if filepath.IsAbs(cleanName) {
		return fmt.Errorf("%w: %q has absolute path", ErrZipSlip, f.Name)
	}

	destPath := filepath.Join(cleanDest, cleanName)
	if !strings.HasPrefix(destPath, cleanDest+string(filepath.Separator)) {
		return fmt.Errorf("%w: %q", ErrZipSlip, f.Name)
	}

	if f.FileInfo().IsDir() {
		return os.MkdirAll(destPath, f.Mode()) // #nosec G703
	}

	parentDir := filepath.Dir(destPath)
	if err := os.MkdirAll(parentDir, extractParentDirPerm); err != nil { // #nosec G703
		return fmt.Errorf("mkdir %q: %w", parentDir, err)
	}

	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("open zip entry %q: %w", f.Name, err)
	}
	defer rc.Close()

	outFile, err := os.OpenFile(
		destPath,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC,
		f.Mode(),
	) // #nosec G304 G703
	if err != nil {
		return fmt.Errorf("create file %q: %w", destPath, err)
	}
	defer outFile.Close()

	//nolint:gosec // zip input is generated by the local build pipeline.
	if _, copyErr := io.Copy(
		outFile,
		rc,
	); copyErr != nil {
		return fmt.Errorf("extract file %q: %w", f.Name, copyErr)
	}

	return nil
}

// startContainer creates and starts a Lambda container for the given function.
// For Zip functions it extracts the code to a temp directory and bind-mounts it.
// Returns the temp directory path (non-empty only for Zip functions), a list of
// layer temp directories, the container ID (empty if creation failed), and any error.
func (b *InMemoryBackend) startContainer(
	ctx context.Context,
	fn *FunctionConfiguration,
	runtimePort int,
) (string, []string, string, error) {
	env := []string{
		fmt.Sprintf("AWS_LAMBDA_RUNTIME_API=%s:%d", b.settings.DockerHost, runtimePort),
		"AWS_DEFAULT_REGION=" + b.region,
		"AWS_REGION=" + b.region,
		"AWS_LAMBDA_FUNCTION_NAME=" + fn.FunctionName,
		fmt.Sprintf("AWS_LAMBDA_FUNCTION_MEMORY_SIZE=%d", fn.MemorySize),
		fmt.Sprintf("AWS_LAMBDA_FUNCTION_TIMEOUT=%d", fn.Timeout),
	}

	if fn.Handler != "" {
		env = append(env, "AWS_LAMBDA_FUNCTION_HANDLER="+fn.Handler)
	}

	if fn.Environment != nil {
		for k, v := range fn.Environment.Variables {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	// Extract layer zips and prepare the /opt mount for both Zip and Image functions.
	layerMount, layerDirs, layerErr := b.prepareLayerMount(fn)
	if layerErr != nil {
		logger.Load(ctx).WarnContext(ctx, "lambda: layer extraction failed",
			"function", fn.FunctionName, "error", layerErr)
	}

	if fn.PackageType == PackageTypeZip {
		zipDir, containerID, err := b.startZipContainer(ctx, fn, env, layerMount)

		return zipDir, layerDirs, containerID, err
	}

	spec := container.Spec{
		Image: fn.ImageURI,
		Name:  fmt.Sprintf("gopherstack-lambda-%s-%s", fn.FunctionName, uuid.New().String()[:8]),
		Env:   env,
	}

	if layerMount != "" {
		spec.Mounts = append(spec.Mounts, layerMount)
	}

	containerID, err := b.docker.CreateAndStart(ctx, spec)

	return "", layerDirs, containerID, err
}

// startZipContainer handles container startup for Zip-packaged Lambda functions.
// It fetches the zip (from inline ZipData or S3), extracts it to a temp directory,
// and bind-mounts the directory into the appropriate AWS base image container.
// An optional layerMount bind-mount string (host:/opt:ro) can also be provided.
// Returns the temp directory path, the container ID, and any error.
func (b *InMemoryBackend) startZipContainer(
	ctx context.Context,
	fn *FunctionConfiguration,
	env []string,
	layerMount string,
) (string, string, error) {
	baseImage := baseImageForRuntime(fn.Runtime)
	if baseImage == "" {
		return "", "", fmt.Errorf("%w: unsupported runtime %q", ErrLambdaUnavailable, fn.Runtime)
	}

	// Resolve zip bytes from inline data or S3.
	zipData := fn.ZipData
	if len(zipData) == 0 && fn.S3BucketCode != "" && fn.S3KeyCode != "" {
		if b.s3Fetcher == nil {
			return "", "", fmt.Errorf(
				"%w: S3 code delivery requires S3 integration",
				ErrLambdaUnavailable,
			)
		}

		var fetchErr error

		zipData, fetchErr = b.s3Fetcher.GetObjectBytes(ctx, fn.S3BucketCode, fn.S3KeyCode)
		if fetchErr != nil {
			return "", "", fmt.Errorf(
				"%w: failed to fetch zip from S3: %w",
				ErrLambdaUnavailable,
				fetchErr,
			)
		}
	}

	if len(zipData) == 0 {
		return "", "", fmt.Errorf(
			"%w: no zip data available for function %q",
			ErrLambdaUnavailable,
			fn.FunctionName,
		)
	}

	zipDir, extractErr := extractZip(zipData)
	if extractErr != nil {
		return "", "", fmt.Errorf("%w: zip extraction failed: %w", ErrLambdaUnavailable, extractErr)
	}

	mounts := []string{zipDir + ":/var/task:ro"}
	if layerMount != "" {
		mounts = append(mounts, layerMount)
	}

	spec := container.Spec{
		Image:  baseImage,
		Name:   fmt.Sprintf("gopherstack-lambda-%s-%s", fn.FunctionName, uuid.New().String()[:8]),
		Env:    env,
		Mounts: mounts,
	}

	// Custom runtimes (provided.*) ship the executable as the zip's "bootstrap"
	// file at /var/task. The provided.al2 base image's default entrypoint runs
	// /var/runtime/bootstrap, so run the function's bootstrap directly instead
	// (matching real AWS, which execs /var/task/bootstrap for custom runtimes).
	if strings.HasPrefix(fn.Runtime, "provided") {
		spec.Entrypoint = []string{"/var/task/bootstrap"}
	} else if fn.Handler != "" {
		spec.Cmd = []string{fn.Handler}
	}

	containerID, err := b.docker.CreateAndStart(ctx, spec)
	if err != nil {
		_ = os.RemoveAll(zipDir) // #nosec G703

		return "", "", err
	}

	return zipDir, containerID, nil
}

// layerEntry holds a single Lambda layer's zip bytes, collected under the backend
// lock so extraction can proceed outside it.
type layerEntry struct {
	name    string
	zipData []byte
	version int64
}

// collectLayerEntries gathers zip bytes for all layers attached to fn under the read
// lock, copying the data out so extraction can proceed without holding the lock.
func (b *InMemoryBackend) collectLayerEntries(fn *FunctionConfiguration) []layerEntry {
	b.mu.RLock("prepareLayerMount")
	defer b.mu.RUnlock()

	var entries []layerEntry

	for _, fl := range fn.Layers {
		if fl == nil || fl.Arn == "" {
			continue
		}

		layerName, layerVersion := parseLayerARN(fl.Arn)
		if layerName == "" {
			continue
		}

		versions := b.layers[layerName]

		for _, lv := range versions {
			if lv.Version == layerVersion && len(lv.ZipData) > 0 {
				data := make([]byte, len(lv.ZipData))
				copy(data, lv.ZipData)
				entries = append(
					entries,
					layerEntry{name: layerName, version: layerVersion, zipData: data},
				)

				break
			}
		}
	}

	return entries
}

// prepareLayerMount extracts all layers attached to fn into a single merged temp directory
// and returns the bind-mount string ("{dir}:/opt:ro"), a list of temp dirs (for cleanup),
// and any error. If the function has no layers with zip data, returns ("", nil, nil).
// ZIP extraction is performed outside the backend lock to avoid blocking concurrent operations.
func (b *InMemoryBackend) prepareLayerMount(fn *FunctionConfiguration) (string, []string, error) {
	if len(fn.Layers) == 0 {
		return "", nil, nil
	}

	entries := b.collectLayerEntries(fn)

	if len(entries) == 0 {
		return "", nil, nil
	}

	// Create a single temp directory to merge all layer contents into (matches AWS behaviour).
	optDir, mkErr := os.MkdirTemp("", "gopherstack-lambda-layers-*")
	if mkErr != nil {
		return "", nil, fmt.Errorf("create layer temp dir: %w", mkErr)
	}

	for _, entry := range entries {
		if extractErr := extractZipIntoDir(optDir, entry.zipData); extractErr != nil {
			_ = os.RemoveAll(optDir)

			return "", nil, fmt.Errorf(
				"extract layer %q v%d: %w",
				entry.name,
				entry.version,
				extractErr,
			)
		}
	}

	return optDir + ":/opt:ro", []string{optDir}, nil
}

// extractZipIntoDir extracts zip bytes into an existing directory (used for layer extraction).
func extractZipIntoDir(dir string, zipData []byte) error {
	r, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}

	for _, f := range r.File {
		if extractErr := extractZipFile(dir, f); extractErr != nil {
			return extractErr
		}
	}

	return nil
}

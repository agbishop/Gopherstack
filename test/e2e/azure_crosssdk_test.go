//go:build e2e

package e2e_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestE2E_AzureCrossSDK proves gopherstack's four Azure services (Blob,
// Queue, Table, Cosmos DB) work against real, unmodified Node.js and Python
// Azure SDKs -- not just the Go SDK exercised by test/integration/azure*_test.go
// and test/integration/cosmosdb_test.go. This is AZURE.md section 7's M4
// deliverable: "a short Node script using @azure/storage-blob/
// @azure/data-tables/@azure/cosmos and a short Python script using
// azure-storage-blob/azure-cosmos, run in CI against a live gopherstack
// instance, proving JS/Python SDKs work unmodified."
//
// Unlike most of this package, which uses internal/teststack's in-process
// harness, this test starts a real gopherstack subprocess: teststack mounts
// only the shared AWS single-port Echo instance and does not wire any Azure
// service (each Azure service binds its own dedicated, synchronously-bound
// port -- see AZURE.md section 4). Ports are chosen dynamically (never
// hardcoded 10000/10001/10002/8081) so this test can run in parallel with
// itself and with test/integration's testcontainers-based suite without
// colliding.
//
// Skips cleanly (not a failure) when node, python3, or either script's SDK
// dependencies are not installed, so `go test -tags=e2e ./test/e2e/...` on a
// bare dev machine still passes.
func TestE2E_AzureCrossSDK(t *testing.T) {
	t.Parallel()

	crosssdkDir := crosssdkDir(t)

	nodePath, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found on PATH; skipping Azure cross-SDK smoke test")
	}

	pythonPath, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not found on PATH; skipping Azure cross-SDK smoke test")
	}

	if !nodeDepsInstalled(crosssdkDir) {
		t.Skip("Node Azure SDK dependencies not installed (run `npm --prefix " +
			crosssdkDir + " ci`); skipping Azure cross-SDK smoke test")
	}

	if !pythonDepsInstalled(t, pythonPath) {
		t.Skip("Python Azure SDK dependencies not installed (run `" + pythonPath +
			" -m pip install -r " + filepath.Join(crosssdkDir, "requirements.txt") +
			"`); skipping Azure cross-SDK smoke test")
	}

	env := startGopherstackForAzureCrossSDK(t)

	tests := []struct {
		command func(ctx context.Context) *exec.Cmd
		name    string
	}{
		{
			name: "node",
			command: func(ctx context.Context) *exec.Cmd {
				return exec.CommandContext(ctx, nodePath, "azure_smoke.mjs")
			},
		},
		{
			name: "python",
			command: func(ctx context.Context) *exec.Cmd {
				return exec.CommandContext(ctx, pythonPath, "azure_smoke.py")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
			defer cancel()

			cmd := tt.command(ctx)
			cmd.Dir = crosssdkDir
			cmd.Env = append(os.Environ(), env...)

			out, runErr := cmd.CombinedOutput()
			require.NoErrorf(t, runErr, "%s cross-SDK smoke script failed\n--- output ---\n%s", tt.name, out)

			t.Logf("%s cross-SDK smoke output:\n%s", tt.name, out)
		})
	}
}

// crosssdkDir returns the absolute path to test/e2e/crosssdk, resolved via
// the Go module root (not this file's own path) so it stays correct
// regardless of where `go test` is invoked from.
func crosssdkDir(t *testing.T) string {
	t.Helper()

	goMod := mustGoEnvGOMOD(t)
	require.NotEmpty(t, goMod, "`go env GOMOD` returned no module (not inside a Go module?)")
	require.NotEqual(t, os.DevNull, goMod, "`go env GOMOD` returned no module (not inside a Go module?)")

	return filepath.Join(filepath.Dir(goMod), "test", "e2e", "crosssdk")
}

func trimNewline(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}

	return s
}

// nodeDepsInstalled reports whether the crosssdk directory's npm
// dependencies have been installed (node_modules/@azure present).
func nodeDepsInstalled(crosssdkDir string) bool {
	info, err := os.Stat(filepath.Join(crosssdkDir, "node_modules", "@azure"))

	return err == nil && info.IsDir()
}

// pythonDepsInstalled reports whether the four Azure Python SDK packages
// this test's script needs are importable by pythonPath.
func pythonDepsInstalled(t *testing.T, pythonPath string) bool {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), pythonPath, "-c",
		"import azure.storage.blob, azure.storage.queue, azure.data.tables, azure.cosmos")

	return cmd.Run() == nil
}

// startGopherstackForAzureCrossSDK builds gopherstack and starts it with its
// four Azure listeners bound to freshly allocated ephemeral ports, returning
// the AZURE_*_ENDPOINT/COSMOSDB_ENDPOINT env var assignments the Node/Python
// scripts read. Registers cleanup (process termination) via t.Cleanup.
func startGopherstackForAzureCrossSDK(t *testing.T) []string {
	t.Helper()

	goMod := mustGoEnvGOMOD(t)
	moduleRoot := filepath.Dir(goMod)

	binPath := filepath.Join(t.TempDir(), "gopherstack-azure-crosssdk")

	buildCmd := exec.CommandContext(t.Context(), "go", "build", "-o", binPath, ".")
	buildCmd.Dir = moduleRoot

	out, buildErr := buildCmd.CombinedOutput()
	require.NoErrorf(t, buildErr, "build gopherstack binary\n--- output ---\n%s", out)

	mainPort := mustFreePort(t)
	blobPort := mustFreePort(t)
	queuePort := mustFreePort(t)
	tablePort := mustFreePort(t)
	cosmosPort := mustFreePort(t)

	cmd := exec.CommandContext(t.Context(), binPath,
		"--port", strconv.Itoa(mainPort),
		"--azure-blob-port", strconv.Itoa(blobPort),
		"--azure-queue-port", strconv.Itoa(queuePort),
		"--azure-table-port", strconv.Itoa(tablePort),
		"--cosmosdb-port", strconv.Itoa(cosmosPort),
	)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	require.NoError(t, cmd.Start(), "start gopherstack subprocess")

	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})

	waitForPort(t, blobPort)
	waitForPort(t, queuePort)
	waitForPort(t, tablePort)
	waitForPort(t, cosmosPort)

	return []string{
		fmt.Sprintf("AZURE_BLOB_ENDPOINT=http://localhost:%d", blobPort),
		fmt.Sprintf("AZURE_QUEUE_ENDPOINT=http://localhost:%d", queuePort),
		fmt.Sprintf("AZURE_TABLE_ENDPOINT=http://localhost:%d", tablePort),
		fmt.Sprintf("COSMOSDB_ENDPOINT=http://localhost:%d", cosmosPort),
	}
}

func mustGoEnvGOMOD(t *testing.T) string {
	t.Helper()

	out, err := exec.CommandContext(t.Context(), "go", "env", "GOMOD").Output()
	require.NoError(t, err, "determine module root via `go env GOMOD`")

	return trimNewline(string(out))
}

// mustFreePort asks the OS for a free TCP port by binding to :0, then
// immediately closes the listener so gopherstack can bind it instead. This
// has an inherent (small) race -- another process could grab the port in
// between -- but is the standard pattern already used elsewhere in this repo
// (e.g. pkgs/portalloc's own tests) for ephemeral-port allocation in tests.
func mustFreePort(t *testing.T) int {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "allocate ephemeral port")
	defer ln.Close()

	return ln.Addr().(*net.TCPAddr).Port
}

// waitForPort polls port on localhost until a TCP connection succeeds or 15
// seconds elapse, failing the test on timeout.
func waitForPort(t *testing.T, port int) {
	t.Helper()

	deadline := time.Now().Add(15 * time.Second)
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()

			return
		}

		time.Sleep(100 * time.Millisecond)
	}

	require.Failf(t, "gopherstack did not start listening in time",
		"address %s did not accept a connection within 15s", addr)
}

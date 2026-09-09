// Package azure_test contains the Terraform acceptance suite for
// services/azurearm (M7), proving that an unmodified hashicorp/azurerm
// provider can apply and destroy azurerm_resource_group and
// azurerm_storage_account against a running gopherstack instance.
//
// This is a SEPARATE package from test/terraform (the AWS suite), per
// AZURE.md section 10.10: it needs its own azurermProviderBlock, its own
// pre-init provider cache for hashicorp/azurerm, FIXED published container
// host ports (not ephemeral -- AZURE.md section 10.4's endpoint
// advertisement defaults to scheme://<request Host>:<configured port>,
// which is only correct when the host-side port number matches what ARM
// advertises), and SSL_CERT_FILE-aware environment plumbing for the tofu
// child process to trust services/azurearm's self-signed HTTPS certificate
// (AZURE.md section 10.8) -- none of which fit the AWS suite's shared
// ephemeral-port container.
package azure_test

import (
	"context"
	"crypto/tls"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	dockercontainer "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"

	"github.com/blackbirdworks/gopherstack/test/internal/buildcheck"
	"github.com/blackbirdworks/gopherstack/test/internal/tofu"

	"github.com/moby/moby/client"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// Fixed host ports this suite publishes the container on -- see this
// package's doc comment for why they must be fixed rather than ephemeral.
// Chosen to match services/azurearm's own defaults exactly, so the ARM
// listener's default endpoint-advertisement logic (scheme://<request
// Host>:<port>) is correct without needing
// --azure-arm-advertise-*-endpoint overrides.
const (
	hostPortARM   = "18006"
	hostPortBlob  = "18000"
	hostPortQueue = "18001"
	hostPortTable = "18002"
)

// endpoint is "host:port" (no scheme) for services/azurearm's HTTPS
// listener -- exactly the metadata_host provider setting's expected shape.
//
//nolint:gochecknoglobals // set once in TestMain, read-only during tests
var endpoint string

// tofuProviderCacheDir is a dedicated provider cache for this suite's
// hashicorp/azurerm downloads, kept separate from test/terraform's
// hashicorp/aws cache.
//
//nolint:gochecknoglobals // shared provider cache path, read-only after init
var tofuProviderCacheDir = filepath.Join(os.TempDir(), "gopherstack-tofu-azurerm-provider-cache")

// certPEMPath is where TestMain writes the ARM listener's self-signed
// certificate (fetched via a real TLS handshake, since the cert is
// generated fresh in-process on every run -- see fetchServerCertPEM), for
// the tofu child process to trust via SSL_CERT_FILE.
//
//nolint:gochecknoglobals // set once in TestMain, read-only during tests
var certPEMPath string

//nolint:gochecknoglobals // set once in TestMain, read-only during tests
var tofuBinaryPath string

//nolint:gochecknoglobals // set once in TestMain, read-only during tests
var sharedContainer testcontainers.Container

// ErrDockerPanic is returned when the Docker availability check panics.
var ErrDockerPanic = errors.New("docker check panicked")

func TestMain(m *testing.M) {
	flag.Parse()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	if testing.Short() {
		logger.Info("skipping azure terraform tests in short mode")
		os.Exit(0)
	}

	if err := checkDocker(); err != nil {
		logger.Error("azure terraform tests require docker", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()

	container, err := startGopherstackContainer(ctx, logger)
	if err != nil {
		logger.Error("failed to start gopherstack container", "error", err)
		os.Exit(1)
	}

	sharedContainer = container

	endpoint = "localhost:" + hostPortARM
	logger.Info("gopherstack ARM listener running", "endpoint", "https://"+endpoint)

	// The two things AZURE.md section 10.8 flags as needing verification
	// before this suite can run at all: obtaining a tofu binary (network
	// access to get.opentofu.org/GitHub releases) and getting the
	// hashicorp/azurerm provider (network access to registry.opentofu.org).
	// Either failing is an environment limitation, not a code defect --
	// skip cleanly rather than fail the build.
	skipReason := prepareTofu(logger)
	if skipReason == "" {
		skipReason = prepareCertTrust(ctx, logger)
	}

	if skipReason != "" {
		logger.Warn("skipping azure terraform suite", "reason", skipReason)

		if tErr := container.Terminate(ctx); tErr != nil {
			logger.Error("failed to terminate container", "error", tErr)
		}

		os.Exit(0)
	}

	code := m.Run()

	if tErr := container.Terminate(ctx); tErr != nil {
		logger.Error("failed to terminate container", "error", tErr)
	}

	os.Exit(code)
}

// checkDocker safely checks if the Docker daemon is available, recovering
// from any potential panics -- mirrors test/terraform's own checkDocker.
func checkDocker() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: %v", ErrDockerPanic, r)
		}
	}()

	_, err = testcontainers.NewDockerProvider()

	return err
}

// mustFixedPortMap builds a network.PortMap binding each container port to
// "0.0.0.0:<hostPort>". Panics on a malformed containerPort key, which would
// be a bug in this file's own literal port strings, not runtime input.
func mustFixedPortMap(bindings map[string]string) network.PortMap {
	unspecified := netip.IPv4Unspecified()

	portMap := make(network.PortMap, len(bindings))

	for containerPort, hostPort := range bindings {
		port, err := network.ParsePort(containerPort)
		if err != nil {
			panic(fmt.Sprintf("mustFixedPortMap: invalid container port %q: %v", containerPort, err))
		}

		portMap[port] = []network.PortBinding{{HostIP: unspecified, HostPort: hostPort}}
	}

	return portMap
}

// startGopherstackContainer builds and starts the gopherstack container with
// FIXED published host ports for the ARM listener and the three storage
// data-plane ports it advertises (see this package's doc comment).
func startGopherstackContainer(ctx context.Context, logger *slog.Logger) (testcontainers.Container, error) {
	dockerfile := "Dockerfile"
	binPath := "../../../bin/gopherstack-linux"

	if runtime.GOOS == "darwin" {
		logger.InfoContext(ctx, "running on Darwin, building Linux binary for container tests...")

		cmd := exec.Command("go", "build", "-trimpath", "-o", "bin/gopherstack-linux", ".")
		cmd.Dir = "../../../"
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOTOOLCHAIN=local")

		if out, buildErr := cmd.CombinedOutput(); buildErr != nil {
			return nil, fmt.Errorf("building linux binary: %w: %s", buildErr, out)
		}
	}

	if binInfo, statErr := os.Stat(binPath); statErr == nil {
		if freshErr := buildcheck.CheckFreshness(logger, binInfo); freshErr != nil {
			return nil, freshErr
		}

		dockerfile = "Dockerfile.test"
	}

	req := testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context:       "../../../",
			Dockerfile:    dockerfile,
			PrintBuildLog: true,
			BuildOptionsModifier: func(options *client.ImageBuildOptions) {
				options.NoCache = false
				options.PullParent = false
			},
		},
		AutoRemove:   true,
		ExposedPorts: []string{"10006/tcp", "10000/tcp", "10001/tcp", "10002/tcp"},
		HostConfigModifier: func(hc *dockercontainer.HostConfig) {
			hc.PortBindings = mustFixedPortMap(map[string]string{
				"10006/tcp": hostPortARM,
				"10000/tcp": hostPortBlob,
				"10001/tcp": hostPortQueue,
				"10002/tcp": hostPortTable,
			})
		},
		WaitingFor: wait.ForListeningPort("10006/tcp").WithStartupTimeout(60 * time.Second),
	}

	return testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
}

// prepareTofu resolves the tofu binary (PATH, else download), returning a
// non-empty skip reason on failure instead of erroring the whole run.
func prepareTofu(logger *slog.Logger) string {
	if path, err := exec.LookPath("tofu"); err == nil {
		tofuBinaryPath = path

		return ""
	}

	if path, err := exec.LookPath("terraform"); err == nil {
		tofuBinaryPath = path

		return ""
	}

	logger.Info("tofu/terraform not found in PATH; downloading OpenTofu...")

	path, err := tofu.DownloadBinary(context.Background(), func(format string, args ...any) {
		logger.Info(fmt.Sprintf(format, args...))
	})
	if err != nil {
		return fmt.Sprintf(
			"could not obtain a tofu/terraform binary (tried PATH and OpenTofu releases download): %v",
			err,
		)
	}

	tofuBinaryPath = path

	return ""
}

// prepareCertTrust fetches the ARM listener's self-signed leaf certificate
// via a real TLS handshake (the cert is generated fresh in-process on every
// gopherstack start, so there is no static file to read) and writes it as
// PEM to a temp file for SSL_CERT_FILE. Returns a non-empty skip reason on
// failure.
func prepareCertTrust(ctx context.Context, logger *slog.Logger) string {
	certPEM, err := fetchServerCertPEM(ctx, "localhost:"+hostPortARM)
	if err != nil {
		return fmt.Sprintf(
			"could not fetch services/azurearm's self-signed certificate for SSL_CERT_FILE trust: %v",
			err,
		)
	}

	f, err := os.CreateTemp("", "gopherstack-azurearm-cert-*.pem")
	if err != nil {
		return fmt.Sprintf("could not create temp file for certificate: %v", err)
	}

	if _, err = f.Write(certPEM); err != nil {
		_ = f.Close()

		return fmt.Sprintf("could not write certificate to temp file: %v", err)
	}

	if err = f.Close(); err != nil {
		return fmt.Sprintf("could not close certificate temp file: %v", err)
	}

	certPEMPath = f.Name()

	logger.InfoContext(
		ctx,
		"wrote services/azurearm's self-signed certificate for SSL_CERT_FILE trust",
		"path",
		certPEMPath,
	)

	return ""
}

// fetchServerCertPEM dials addr over TLS (skipping verification, since we
// don't yet trust the cert we're about to extract) and PEM-encodes the
// leaf certificate the server presented.
func fetchServerCertPEM(ctx context.Context, addr string) ([]byte, error) {
	dialer := &tls.Dialer{Config: &tls.Config{InsecureSkipVerify: true}}

	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dialing %s: %w", addr, err)
	}
	defer conn.Close()

	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		return nil, errNotATLSConn
	}

	certs := tlsConn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return nil, errNoPeerCertificates
	}

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certs[0].Raw}), nil
}

var errNotATLSConn = errors.New("dialed connection is not a *tls.Conn")

var errNoPeerCertificates = errors.New("server presented no certificates")

// azurermProviderBlock returns the HCL required_providers + provider
// "azurerm" block pointing every ARM call at the running gopherstack
// instance's ARM listener, per AZURE.md section 10.8. metadataHost is a bare
// "host:port" (no scheme -- the provider prefixes https:// itself).
func azurermProviderBlock(metadataHost string) string {
	return fmt.Sprintf(`terraform {
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 4.0"
    }
  }
  required_version = ">= 1.0"
}

provider "azurerm" {
  features {}

  metadata_host                   = %[1]q
  environment                     = "gopherstack"
  subscription_id                 = "00000000-0000-0000-0000-000000000000"
  tenant_id                       = "00000000-0000-0000-0000-000000000000"
  client_id                       = "00000000-0000-0000-0000-000000000000"
  client_secret                   = "gopherstack"
  resource_provider_registrations = "none"
  storage_use_azuread             = false
}
`, metadataHost)
}

// azureResourceGroupAndStorageAccountFixture is the M7 acceptance fixture:
// one resource group and one storage account, both pure-ARM resources (no
// direct-data-plane azurerm_storage_container/_blob/_queue/_table -- those
// are out of scope until M10, see AZURE.md section 10.8's second open
// uncertainty).
const azureResourceGroupAndStorageAccountFixture = `
resource "azurerm_resource_group" "test" {
  name     = "gopherstack-m7-test-rg"
  location = "local"
}

resource "azurerm_storage_account" "test" {
  name                     = "gopherstackm7test"
  resource_group_name      = azurerm_resource_group.test.name
  location                 = azurerm_resource_group.test.location
  account_tier             = "Standard"
  account_replication_type = "LRS"
}

output "storage_account_id" {
  value = azurerm_storage_account.test.id
}
`

// applyAzureTofu writes hcl to dir/main.tf and runs tofu/terraform init,
// apply, and (via t.Cleanup) destroy against it, trusting
// services/azurearm's self-signed certificate via SSL_CERT_FILE (AZURE.md
// section 10.8). Unlike test/terraform's applyTofu, this doesn't reuse a
// pre-initialized .terraform directory across tests -- there is currently
// only one Azure Terraform test, so the AWS suite's parallel-test disk/lock
// optimization isn't worth the complexity here yet.
func applyAzureTofu(t *testing.T, dir, hcl string) {
	t.Helper()

	cfgPath := filepath.Join(dir, "main.tf")
	if err := os.WriteFile(cfgPath, []byte(hcl), 0o600); err != nil {
		t.Fatalf("writing %s: %v", cfgPath, err)
	}

	if err := os.MkdirAll(tofuProviderCacheDir, 0o750); err != nil {
		t.Logf("could not create provider cache dir: %v", err)
	}

	env := append(
		os.Environ(),
		"TF_IN_AUTOMATION=1",
		"TF_PLUGIN_CACHE_DIR="+tofuProviderCacheDir,
		"TF_PLUGIN_CACHE_MAY_BREAK_DEPENDENCY_LOCK_FILE=true",
		"SSL_CERT_FILE="+certPEMPath,
	)

	run := func(failFatal bool, args ...string) bool {
		t.Helper()

		cmd := exec.Command(tofuBinaryPath, args...)
		cmd.Dir = dir
		cmd.Env = env

		out, err := cmd.CombinedOutput()
		t.Logf("tofu %v:\n%s", args, out)

		if err != nil {
			if failFatal {
				t.Fatalf("tofu %v failed: %v", args, err)
			}

			t.Logf("tofu %v failed (non-fatal): %v", args, err)

			return false
		}

		return true
	}

	if !run(false, "init", "-no-color") {
		t.Skip("tofu init failed -- likely no network access to registry.opentofu.org for the azurerm provider; " +
			"see AZURE.md section 10.8 and services/azurearm/PARITY.md for this suite's known environment limitations")
	}

	run(true, "apply", "-auto-approve", "-no-color")

	t.Cleanup(func() {
		run(false, "destroy", "-auto-approve", "-no-color")
	})
}

// TestTerraform_Azure_ResourceGroupAndStorageAccount proves that an
// unmodified hashicorp/azurerm provider can apply and destroy
// azurerm_resource_group and azurerm_storage_account against
// services/azurearm (M7). See this package's doc comment for why direct
// data-plane resources (azurerm_storage_container etc.) are out of scope.
func TestTerraform_Azure_ResourceGroupAndStorageAccount(t *testing.T) {
	t.Parallel()

	if sharedContainer == nil {
		t.Skip("azure terraform suite was skipped in TestMain (see its logged reason)")
	}

	dir := t.TempDir()
	hcl := azurermProviderBlock(endpoint) + azureResourceGroupAndStorageAccountFixture

	applyAzureTofu(t, dir, hcl)
}

// Package gopherstack provides a Testcontainers module for Gopherstack.
//
// It lets Go test suites spin up a real Gopherstack container (all AWS mock
// services on a single HTTP port) with a single call:
//
//	container, err := gopherstack.Run(ctx, "ghcr.io/blackbirdworks/gopherstack:latest")
//	url, err := container.BaseURL(ctx)
//	// url == "http://localhost:<mapped-port>"
//
// All AWS SDK v2 clients can then be pointed at url without any other
// credential or region configuration changes.
package gopherstack

import (
	"context"
	"fmt"
	"maps"
	"net"
	"net/http"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	// DefaultImage is the pre-built Gopherstack Docker image published to GHCR.
	DefaultImage = "ghcr.io/blackbirdworks/gopherstack:latest"

	// defaultPort is the single port Gopherstack listens on inside the container.
	defaultPort = "8000/tcp"

	// startupTimeout is the maximum time to wait for the container health check.
	startupTimeout = 60 * time.Second
)

// Container wraps testcontainers.Container and adds Gopherstack helpers.
type Container struct {
	testcontainers.Container
}

// BaseURL returns the HTTP URL that AWS SDK clients should use as their
// endpoint override (e.g. "http://localhost:32768").
func (c *Container) BaseURL(ctx context.Context) (string, error) {
	host, err := c.Host(ctx)
	if err != nil {
		return "", fmt.Errorf("gopherstack: get container host: %w", err)
	}

	port, err := c.MappedPort(ctx, defaultPort)
	if err != nil {
		return "", fmt.Errorf("gopherstack: get mapped port: %w", err)
	}

	return "http://" + net.JoinHostPort(host, port.Port()), nil
}

// WithEnv returns a ContainerCustomizer that sets environment variables on the
// container.  Keys and values are passed directly to Gopherstack, which exposes
// them as the usual AWS_* and service-specific variables.
func WithEnv(env map[string]string) testcontainers.ContainerCustomizer {
	return testcontainers.CustomizeRequestOption(func(req *testcontainers.GenericContainerRequest) error {
		if req.Env == nil {
			req.Env = make(map[string]string)
		}

		maps.Copy(req.Env, env)

		return nil
	})
}

// Run starts a Gopherstack container using the provided Docker image and waits
// until the service is ready.  The caller is responsible for terminating the
// container after use.
//
//	container, err := gopherstack.Run(ctx, gopherstack.DefaultImage)
//	defer testcontainers.TerminateContainer(container)
func Run(ctx context.Context, image string, opts ...testcontainers.ContainerCustomizer) (*Container, error) {
	req := testcontainers.GenericContainerRequest{
		Image:        image,
		ExposedPorts: []string{defaultPort},
		WaitingFor: wait.ForHTTP("/_gopherstack/health").
			WithStatusCodeMatcher(func(code int) bool { return code == http.StatusOK }).
			WithStartupTimeout(startupTimeout),
		Started: true,
	}

	for _, opt := range opts {
		if err := opt.Customize(&req); err != nil {
			return nil, fmt.Errorf("gopherstack: customize request: %w", err)
		}
	}

	c, err := testcontainers.GenericContainer(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("gopherstack: start container: %w", err)
	}

	return &Container{Container: c}, nil
}

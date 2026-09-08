package gopherstack_test

import (
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"

	gopherstackmodule "github.com/blackbirdworks/gopherstack/modules/gopherstack"
)

// TestRun verifies that a Gopherstack container starts, exposes a valid
// endpoint, and responds to the health check.
// Requires Docker and a published Gopherstack image; only runs when
// GOPHERSTACK_MODULE_TESTS=1 is set.
func TestRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		image string
	}{
		{
			name:  "default image starts and responds to health check",
			image: gopherstackmodule.DefaultImage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if os.Getenv("GOPHERSTACK_MODULE_TESTS") != "1" {
				t.Skip("skipping: set GOPHERSTACK_MODULE_TESTS=1 to run Docker-based module tests")
			}

			testcontainers.SkipIfProviderIsNotHealthy(t)

			ctx := t.Context()

			container, err := gopherstackmodule.Run(ctx, tt.image)
			require.NoError(t, err)

			defer testcontainers.TerminateContainer(container)

			url, err := container.BaseURL(ctx)
			require.NoError(t, err)
			assert.NotEmpty(t, url, "expected non-empty base URL")

			resp, err := http.Get(url + "/_gopherstack/health")
			require.NoError(t, err)

			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})
	}
}

// TestWithEnv verifies that the WithEnv option correctly applies environment
// variables to the container request, including initializing a nil Env map.
func TestWithEnv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		initial map[string]string
		input   map[string]string
		wantEnv map[string]string
		name    string
		wantErr bool
	}{
		{
			name:    "applies multiple env vars to existing map",
			initial: nil,
			input:   map[string]string{"LOG_LEVEL": "debug", "DEMO": "true"},
			wantEnv: map[string]string{"LOG_LEVEL": "debug", "DEMO": "true"},
		},
		{
			name:    "initializes nil Env map and sets key",
			initial: nil,
			input:   map[string]string{"KEY": "value"},
			wantEnv: map[string]string{"KEY": "value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := testcontainers.GenericContainerRequest{
				Image: "dummy",
				Env:   tt.initial,
			}

			opt := gopherstackmodule.WithEnv(tt.input)
			err := opt.Customize(&req)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, req.Env)

			for k, v := range tt.wantEnv {
				assert.Equal(t, v, req.Env[k])
			}
		})
	}
}

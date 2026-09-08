// Package tofu downloads a local OpenTofu binary for test suites that don't
// find one in PATH. Extracted from test/terraform/terraform_test.go (the AWS
// Terraform suite) so test/terraform/azure (the Azure Terraform suite, M7)
// can share it instead of duplicating the download/extract logic.
package tofu

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var (
	errNoTofuVersions = errors.New("no stable versions found in OpenTofu API response")
	errTofuNotInZip   = errors.New("tofu binary not found in zip archive")
)

// DownloadBinary fetches the latest stable OpenTofu release for the current
// platform, extracts the binary to [os.TempDir], and returns its path.
func DownloadBinary(ctx context.Context, logf func(format string, args ...any)) (string, error) {
	versionReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://get.opentofu.org/tofu/api.json", nil)
	if err != nil {
		return "", fmt.Errorf("creating OpenTofu version request: %w", err)
	}

	resp, err := http.DefaultClient.Do(versionReq)
	if err != nil {
		return "", fmt.Errorf("fetching OpenTofu version list: %w", err)
	}
	defer resp.Body.Close()

	var api struct {
		Versions []struct {
			ID string `json:"id"`
		} `json:"versions"`
	}

	if err = json.NewDecoder(resp.Body).Decode(&api); err != nil {
		return "", fmt.Errorf("decoding OpenTofu version list: %w", err)
	}

	version := latestStableVersion(api.Versions)
	if version == "" {
		return "", errNoTofuVersions
	}

	goos := runtime.GOOS
	goarch := runtime.GOARCH

	downloadURL := fmt.Sprintf(
		"https://github.com/opentofu/opentofu/releases/download/v%s/tofu_%s_%s_%s.zip",
		version, version, goos, goarch,
	)
	if logf != nil {
		logf("downloading OpenTofu %s from %s", version, downloadURL)
	}

	zipReq, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating OpenTofu download request: %w", err)
	}

	zipResp, err := http.DefaultClient.Do(zipReq)
	if err != nil {
		return "", fmt.Errorf("downloading OpenTofu zip: %w", err)
	}
	defer zipResp.Body.Close()

	zipData, err := io.ReadAll(zipResp.Body)
	if err != nil {
		return "", fmt.Errorf("reading OpenTofu zip: %w", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return "", fmt.Errorf("opening OpenTofu zip: %w", err)
	}

	binaryName := "tofu"
	if goos == "windows" {
		binaryName = "tofu.exe"
	}

	for _, f := range zr.File {
		if f.Name != binaryName {
			continue
		}

		return extractBinary(f, binaryName)
	}

	return "", errTofuNotInZip
}

// extractBinary writes a single [zip.File] to [os.TempDir] with executable
// permissions and returns its path.
func extractBinary(f *zip.File, binaryName string) (string, error) {
	destPath := filepath.Join(os.TempDir(), binaryName)

	rc, err := f.Open()
	if err != nil {
		return "", fmt.Errorf("opening %s in zip: %w", binaryName, err)
	}
	defer rc.Close()

	const executableMode = 0o755 // needs the exec bit to run as `tofu`

	out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, executableMode)
	if err != nil {
		return "", fmt.Errorf("creating tofu binary: %w", err)
	}
	defer out.Close()

	if _, err = io.Copy(out, rc); err != nil { //nolint:gosec // zip entry is from OpenTofu's own signed GitHub release
		return "", fmt.Errorf("writing tofu binary: %w", err)
	}

	return destPath, nil
}

// latestStableVersion returns the first stable (non-pre-release) version ID
// from the OpenTofu API versions list, or an empty string if none is found.
func latestStableVersion(versions []struct {
	ID string `json:"id"`
},
) string {
	for _, v := range versions {
		if !strings.Contains(v.ID, "-") {
			return v.ID
		}
	}

	return ""
}

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvaluatePin(t *testing.T) {
	t.Parallel()

	goModVersions := map[string]string{
		"dlm":            "v1.39.4",
		"opsworks":       "v1.31.0",
		"storage/azblob": "v1.8.0",
	}

	tests := []struct {
		content  string
		slug     string
		name     string
		wantMsg  string
		wantKind resultKind
	}{
		{
			name:     "matching pin",
			slug:     "dlm",
			content:  "service: dlm\nsdk_module: aws-sdk-go-v2/service/dlm@v1.39.4   # audited\n",
			wantKind: resultOK,
		},
		{
			name:     "mismatched pin",
			slug:     "dlm",
			content:  "service: dlm\nsdk_module: aws-sdk-go-v2/service/dlm@v1.30.0   # stale\n",
			wantKind: resultMismatch,
			wantMsg:  "dlm: recorded v1.30.0, go.mod v1.39.4",
		},
		{
			name:     "matching azure pin",
			slug:     "azureblob",
			content:  "service: azureblob\nsdk_module: azure-sdk-for-go/sdk/storage/azblob@v1.8.0   # audited\n",
			wantKind: resultOK,
		},
		{
			name:     "mismatched azure pin",
			slug:     "azureblob",
			content:  "service: azureblob\nsdk_module: azure-sdk-for-go/sdk/storage/azblob@v1.7.0   # stale\n",
			wantKind: resultMismatch,
			wantMsg:  "azureblob: recorded v1.7.0, go.mod v1.8.0",
		},
		{
			name: "module cache only documented",
			slug: "opsworks",
			content: "service: opsworks\n" +
				"sdk_module: aws-sdk-go-v2/service/opsworks@v1.31.0   # exists in the module cache but is NOT\n" +
				"                                                       # a go.mod dependency of this repo\n",
			wantKind: resultOK,
		},
		{
			name: "hashicorp terraform provider pin, never a go.mod dependency",
			slug: "azurearm",
			content: "service: azurearm\n" +
				"sdk_module: hashicorp/terraform-provider-azurerm@v4.81.0   # not a go.mod dependency\n",
			wantKind: resultOK,
		},
		{
			name:     "hashicorp pin missing version",
			slug:     "azurearm",
			content:  "service: azurearm\nsdk_module: hashicorp/terraform-provider-azurerm\n",
			wantKind: resultMismatch,
			wantMsg:  "no parseable @version",
		},
		{
			name:     "module missing and undocumented",
			slug:     "foo",
			content:  "service: foo\nsdk_module: aws-sdk-go-v2/service/foo@v1.0.0   # some other note\n",
			wantKind: resultMismatch,
			wantMsg:  `foo: recorded module "foo" not found in go.mod`,
		},
		{
			name:     "malformed value",
			slug:     "dlm",
			content:  "service: dlm\nsdk_module: aws-sdk-go-v2/service/dlm@v1.39.4 (now a real dependency\n",
			wantKind: resultMismatch,
			wantMsg:  "no parseable @version",
		},
		{
			name:     "missing field",
			slug:     "dlm",
			content:  "service: dlm\nlast_audit_commit: abc123\n",
			wantKind: resultMismatch,
			wantMsg:  "no sdk_module field",
		},
		{
			name:     "missing version prefix",
			slug:     "dlm",
			content:  "service: dlm\nsdk_module: aws-sdk-go-v2/service/dlm@1.39.4\n",
			wantKind: resultMismatch,
			wantMsg:  "no parseable @version",
		},
		{
			name: "unterminated trailing note",
			slug: "dlm",
			content: "service: dlm\nsdk_module: aws-sdk-go-v2/service/dlm@v1.39.4 (now a real go.mod/go.sum\n" +
				"  dependency, added this pass via `go get`)\n",
			wantKind: resultMismatch,
			wantMsg:  "no parseable @version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := evaluatePin(tt.slug, tt.content, goModVersions)

			require.Equal(t, tt.wantKind, got.kind)
			if tt.wantMsg != "" {
				assert.Contains(t, got.message, tt.wantMsg)
			}
		})
	}
}

func TestLoadGoModVersions(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "go.mod")
	content := `module example.com/thing

go 1.26.5

require (
	github.com/aws/aws-sdk-go-v2/service/dlm v1.39.4
	github.com/Azure/azure-sdk-for-go/sdk/storage/azblob v1.8.0
	github.com/other/pkg v0.1.0
)

require github.com/aws/aws-sdk-go-v2/service/opsworks v1.31.0
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	versions, err := loadGoModVersions(path)
	require.NoError(t, err)

	assert.Equal(t, "v1.39.4", versions["dlm"])
	assert.Equal(t, "v1.31.0", versions["opsworks"])
	assert.Equal(t, "v1.8.0", versions["storage/azblob"])
	assert.NotContains(t, versions, "pkg")
}

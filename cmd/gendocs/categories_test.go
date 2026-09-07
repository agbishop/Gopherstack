package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDisplayName_Azure proves the four Azure services (M0-M3) get curated
// display names instead of falling back to titleCaseSlug's ugly
// run-together capitalization (e.g. "Azureblob", "Cosmosdb") -- see AZURE.md
// section 7's M4 docs-polish entry.
func TestDisplayName_Azure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		slug string
		want string
	}{
		{slugAzureBlob, "Azure Blob Storage"},
		{slugAzureQueue, "Azure Queue Storage"},
		{slugAzureTable, "Azure Table Storage"},
		{slugCosmosDB, "Azure Cosmos DB"},
	}

	for _, tc := range tests {
		t.Run(tc.slug, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, displayName(tc.slug))
		})
	}
}

// TestCategoryFor_Azure proves the four Azure services are grouped under a
// dedicated "Azure" category rather than falling through to the "Other"
// catch-all.
func TestCategoryFor_Azure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		slug string
	}{
		{slugAzureBlob},
		{slugAzureQueue},
		{slugAzureTable},
		{slugCosmosDB},
	}

	for _, tc := range tests {
		t.Run(tc.slug, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, "Azure", categoryFor(tc.slug))
		})
	}
}

// TestCategoryGroups_EverySlugInExactlyOneGroup guards against a slug
// (Azure or otherwise) being listed in two categories at once, or duplicated
// within the same category -- either would silently corrupt the root
// README's service table.
func TestCategoryGroups_EverySlugInExactlyOneGroup(t *testing.T) {
	t.Parallel()

	seen := make(map[string]string)

	for _, g := range categoryGroups() {
		for _, slug := range g.Slugs {
			if prevGroup, ok := seen[slug]; ok {
				require.Failf(t, "duplicate slug", "slug %q appears in both %q and %q", slug, prevGroup, g.Name)
			}

			seen[slug] = g.Name
		}
	}
}

package lakeformation_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/lakeformation"
)

func TestCreateGetDeleteLFTag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		catalogID string
		tagKey    string
		tagValues []string
		wantErr   bool
	}{
		{
			name:      "success",
			catalogID: "123456789012",
			tagKey:    "env",
			tagValues: []string{"dev", "prod"},
		},
		{
			name:      "duplicate",
			catalogID: "123456789012",
			tagKey:    "env",
			tagValues: []string{"dev"},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := lakeformation.NewInMemoryBackend()

			err := b.CreateLFTag(tt.catalogID, tt.tagKey, tt.tagValues)
			require.NoError(t, err)

			if tt.wantErr {
				err = b.CreateLFTag(tt.catalogID, tt.tagKey, tt.tagValues)
				require.Error(t, err)

				return
			}

			tag, err := b.GetLFTag(tt.catalogID, tt.tagKey)
			require.NoError(t, err)
			assert.Equal(t, tt.tagKey, tag.TagKey)
			assert.ElementsMatch(t, tt.tagValues, tag.TagValues)

			err = b.DeleteLFTag(tt.catalogID, tt.tagKey)
			require.NoError(t, err)

			_, err = b.GetLFTag(tt.catalogID, tt.tagKey)
			require.Error(t, err)
		})
	}
}

func TestUpdateLFTag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		initial      []string
		toAdd        []string
		toDelete     []string
		wantValues   []string
		wantNotFound bool
	}{
		{
			name:       "add_value",
			initial:    []string{"dev"},
			toAdd:      []string{"prod"},
			wantValues: []string{"dev", "prod"},
		},
		{
			name:       "delete_value",
			initial:    []string{"dev", "prod"},
			toDelete:   []string{"prod"},
			wantValues: []string{"dev"},
		},
		{
			name:         "not_found",
			wantNotFound: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := lakeformation.NewInMemoryBackend()

			if tt.wantNotFound {
				err := b.UpdateLFTag("cat", "missing-key", nil, nil)
				require.Error(t, err)

				return
			}

			require.NoError(t, b.CreateLFTag("cat", "env", tt.initial))
			require.NoError(t, b.UpdateLFTag("cat", "env", tt.toAdd, tt.toDelete))

			tag, err := b.GetLFTag("cat", "env")
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.wantValues, tag.TagValues)
		})
	}
}

func TestListLFTags_AllCatalogs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		wantCount int
	}{
		{name: "empty catalog ID returns all tags", wantCount: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := lakeformation.NewInMemoryBackend()

			require.NoError(t, b.CreateLFTag("cat1", "env", []string{"prod", "dev"}))
			require.NoError(t, b.CreateLFTag("cat2", "tier", []string{"gold", "silver"}))

			tags, _ := b.ListLFTags("", "", 0, "")
			assert.Len(t, tags, tt.wantCount)
		})
	}
}

func TestDeleteLFTag_NotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		tagKey  string
		wantErr bool
	}{
		{name: "delete non-existent tag returns error", tagKey: "nonexistent", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := lakeformation.NewInMemoryBackend()
			err := b.DeleteLFTag("cat1", tt.tagKey)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestGetLFTag_ReturnsCopy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "mutating returned LFTag does not affect backend state"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := lakeformation.NewInMemoryBackend()

			require.NoError(t, b.CreateLFTag("cat1", "env", []string{"prod", "dev"}))

			tag, err := b.GetLFTag("cat1", "env")
			require.NoError(t, err)

			// Mutate the returned copy.
			tag.TagValues = append(tag.TagValues, "injected")
			tag.TagKey = "MUTATED"

			// Backend state must be unchanged.
			tag2, err := b.GetLFTag("cat1", "env")
			require.NoError(t, err)
			assert.Equal(t, "env", tag2.TagKey, "mutating returned LFTag must not affect backend TagKey")
			assert.Len(t, tag2.TagValues, 2, "mutating returned LFTag TagValues must not affect backend")
		})
	}
}

func TestBackend_AddLFTagsToResource_UpdateExisting(t *testing.T) {
	t.Parallel()

	b := lakeformation.NewInMemoryBackend()
	require.NoError(t, b.CreateLFTag("", "env", []string{"dev", "prod"}))

	resource := &lakeformation.Resource{
		Database: &lakeformation.DatabaseResource{Name: "mydb"},
	}
	tags := []lakeformation.LFTagPair{{TagKey: "env", TagValues: []string{"dev"}}}

	failures := b.AddLFTagsToResource("", resource, tags)
	assert.Empty(t, failures)

	// Update same tag with a valid value from the allowed set
	tags2 := []lakeformation.LFTagPair{{TagKey: "env", TagValues: []string{"prod"}}}
	failures2 := b.AddLFTagsToResource("", resource, tags2)
	assert.Empty(t, failures2)
}

func TestUpdateLFTag_SortsTagValues(t *testing.T) {
	t.Parallel()

	b := lakeformation.NewInMemoryBackend()
	b.AddLFTagInternal("", "env", []string{"prod"})

	err := b.UpdateLFTag("", "env", []string{"dev", "staging"}, nil)
	require.NoError(t, err)

	tag, err := b.GetLFTag("", "env")
	require.NoError(t, err)
	assert.Equal(t, []string{"dev", "prod", "staging"}, tag.TagValues)
}

func TestDeleteLFTag_DetachesFromResources(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "deleting a tag removes its resource attachments"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := lakeformation.NewInMemoryBackend()
			require.NoError(t, b.CreateLFTag("cat1", "env", []string{"prod", "dev"}))

			resource := &lakeformation.Resource{
				Database: &lakeformation.DatabaseResource{Name: "mydb"},
			}
			pairs := []lakeformation.LFTagPair{{CatalogID: "cat1", TagKey: "env", TagValues: []string{"prod"}}}

			require.Empty(t, b.AddLFTagsToResource("cat1", resource, pairs))

			attached, err := b.GetResourceLFTags("cat1", resource)
			require.NoError(t, err)
			require.Len(t, attached, 1, "precondition: tag must be attached before delete")

			require.NoError(t, b.DeleteLFTag("cat1", "env"))

			remaining, err := b.GetResourceLFTags("cat1", resource)
			require.NoError(t, err)
			assert.Empty(t, remaining, "GetResourceLFTags must not return a ghost attachment for a deleted tag")

			// A tag key is reusable; a freshly created tag with the same key
			// must not inherit the old attachment.
			require.NoError(t, b.CreateLFTag("cat1", "env", []string{"staging"}))

			afterRecreate, err := b.GetResourceLFTags("cat1", resource)
			require.NoError(t, err)
			assert.Empty(t, afterRecreate,
				"recreated tag must not inherit stale attachments from before it was deleted")
		})
	}
}

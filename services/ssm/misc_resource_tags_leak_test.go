package ssm_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/ssm"
)

// TestDeletePaths_CleanMiscResourceTags covers gopherstack-0bqq: five
// Delete* paths (PatchBaseline, MaintenanceWindow, Association, OpsItem,
// OpsMetadata) write into miscResourceTags on create but never cleaned it
// up on delete. Each case is asserted independently through the observable
// path (ListTagsForResource, which does a bare map lookup with no
// existence check) so a fix to one path cannot mask a still-broken one.
func TestDeletePaths_CleanMiscResourceTags(t *testing.T) {
	t.Parallel()

	t.Run("patch_baseline", func(t *testing.T) {
		t.Parallel()

		b := ssm.NewInMemoryBackend()
		ctx := context.Background()

		created, err := b.CreatePatchBaseline(ctx, &ssm.CreatePatchBaselineInput{
			Name:            "leak-baseline",
			OperatingSystem: "AMAZON_LINUX_2",
			Tags:            []ssm.Tag{{Key: "k", Value: "v"}},
		})
		require.NoError(t, err)

		_, err = b.DeletePatchBaseline(ctx, &ssm.DeletePatchBaselineInput{BaselineID: created.BaselineID})
		require.NoError(t, err)

		listOut, err := b.ListTagsForResource(ctx, &ssm.ListTagsForResourceInput{
			ResourceType: "PatchBaseline",
			ResourceID:   created.BaselineID,
		})
		require.NoError(t, err)
		assert.Empty(t, listOut.TagList, "deleted patch baseline's id must not still resolve tags")
	})

	t.Run("maintenance_window", func(t *testing.T) {
		t.Parallel()

		b := ssm.NewInMemoryBackend()
		ctx := context.Background()

		created, err := b.CreateMaintenanceWindow(ctx, &ssm.CreateMaintenanceWindowInput{
			Name:     "leak-window",
			Schedule: "cron(0 9 ? * MON *)",
			Duration: 2,
			Cutoff:   1,
			Tags:     []ssm.Tag{{Key: "k", Value: "v"}},
		})
		require.NoError(t, err)

		_, err = b.DeleteMaintenanceWindow(ctx, &ssm.DeleteMaintenanceWindowInput{WindowID: created.WindowID})
		require.NoError(t, err)

		listOut, err := b.ListTagsForResource(ctx, &ssm.ListTagsForResourceInput{
			ResourceType: "MaintenanceWindow",
			ResourceID:   created.WindowID,
		})
		require.NoError(t, err)
		assert.Empty(t, listOut.TagList, "deleted maintenance window's id must not still resolve tags")
	})

	t.Run("association", func(t *testing.T) {
		t.Parallel()

		b := ssm.NewInMemoryBackend()
		ctx := context.Background()

		_, err := b.CreateDocument(ctx, &ssm.CreateDocumentInput{
			Name:    "leak-assoc-doc",
			Content: `{"schemaVersion":"2.2"}`,
		})
		require.NoError(t, err)

		created, err := b.CreateAssociation(ctx, &ssm.CreateAssociationInput{
			Name:       "leak-assoc-doc",
			InstanceID: "i-001",
			Tags:       []ssm.Tag{{Key: "k", Value: "v"}},
		})
		require.NoError(t, err)
		assocID := created.AssociationDescription.AssociationID

		_, err = b.DeleteAssociation(ctx, &ssm.DeleteAssociationInput{AssociationID: assocID})
		require.NoError(t, err)

		listOut, err := b.ListTagsForResource(ctx, &ssm.ListTagsForResourceInput{
			ResourceType: "Association",
			ResourceID:   assocID,
		})
		require.NoError(t, err)
		assert.Empty(t, listOut.TagList, "deleted association's id must not still resolve tags")
	})

	t.Run("ops_item", func(t *testing.T) {
		t.Parallel()

		b := ssm.NewInMemoryBackend()
		ctx := context.Background()

		created, err := b.CreateOpsItem(ctx, &ssm.CreateOpsItemInput{
			Title:       "leak-item",
			Source:      "test",
			Description: "test description",
			Tags:        []ssm.Tag{{Key: "k", Value: "v"}},
		})
		require.NoError(t, err)

		_, err = b.DeleteOpsItem(ctx, &ssm.DeleteOpsItemInput{OpsItemID: created.OpsItemID})
		require.NoError(t, err)

		listOut, err := b.ListTagsForResource(ctx, &ssm.ListTagsForResourceInput{
			ResourceType: "OpsItem",
			ResourceID:   created.OpsItemID,
		})
		require.NoError(t, err)
		assert.Empty(t, listOut.TagList, "deleted ops item's id must not still resolve tags")
	})

	t.Run("ops_metadata", func(t *testing.T) {
		t.Parallel()

		b := ssm.NewInMemoryBackend()
		ctx := context.Background()

		created, err := b.CreateOpsMetadata(ctx, &ssm.CreateOpsMetadataInput{
			ResourceID: "arn:aws:ec2:us-east-1:123456789012:instance/i-leak-meta",
			Tags:       []ssm.Tag{{Key: "k", Value: "v"}},
		})
		require.NoError(t, err)

		_, err = b.DeleteOpsMetadata(ctx, &ssm.DeleteOpsMetadataInput{OpsMetadataArn: created.OpsMetadataArn})
		require.NoError(t, err)

		listOut, err := b.ListTagsForResource(ctx, &ssm.ListTagsForResourceInput{
			ResourceType: "OpsMetadata",
			ResourceID:   created.OpsMetadataArn,
		})
		require.NoError(t, err)
		assert.Empty(t, listOut.TagList, "deleted ops metadata's arn must not still resolve tags")
	})
}

// TestDeletePatchBaseline_DoesNotDisturbSiblingTags is the negative case:
// deleting one PatchBaseline's tags must not touch another's.
func TestDeletePatchBaseline_DoesNotDisturbSiblingTags(t *testing.T) {
	t.Parallel()

	b := ssm.NewInMemoryBackend()
	ctx := context.Background()

	kept, err := b.CreatePatchBaseline(ctx, &ssm.CreatePatchBaselineInput{
		Name:            "keep-baseline",
		OperatingSystem: "AMAZON_LINUX_2",
		Tags:            []ssm.Tag{{Key: "keep", Value: "me"}},
	})
	require.NoError(t, err)

	doomed, err := b.CreatePatchBaseline(ctx, &ssm.CreatePatchBaselineInput{
		Name:            "doomed-baseline",
		OperatingSystem: "AMAZON_LINUX_2",
		Tags:            []ssm.Tag{{Key: "doomed", Value: "yes"}},
	})
	require.NoError(t, err)

	_, err = b.DeletePatchBaseline(ctx, &ssm.DeletePatchBaselineInput{BaselineID: doomed.BaselineID})
	require.NoError(t, err)

	listOut, err := b.ListTagsForResource(ctx, &ssm.ListTagsForResourceInput{
		ResourceType: "PatchBaseline",
		ResourceID:   kept.BaselineID,
	})
	require.NoError(t, err)
	require.Len(t, listOut.TagList, 1)
	assert.Equal(t, "keep", listOut.TagList[0].Key)
}

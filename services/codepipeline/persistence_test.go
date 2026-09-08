package codepipeline //nolint:testpackage // needs cpCtxRegion (isolation_test.go) + the unexported region context key.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInMemoryBackend_RestoreRejectsBadSnapshots is the Phase 3.3
// version-guard test: it verifies Restore never partially decodes a snapshot
// it cannot trust -- malformed JSON is reported as an error, and any version
// mismatch (including the pre-Phase-3.3 shape, which has no
// "version"/"tables" keys at all and so decodes with Version == 0) is
// discarded cleanly (backend reset to empty, no error) rather than
// misinterpreted.
func TestInMemoryBackend_RestoreRejectsBadSnapshots(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		data      []byte
		wantError bool
	}{
		{
			name:      "malformed JSON is an error",
			data:      []byte("not-valid-json"),
			wantError: true,
		},
		{
			name:      "version mismatch is discarded cleanly",
			data:      []byte(`{"version":999,"tables":{}}`),
			wantError: false,
		},
		{
			name: "pre-Phase-3.3 shape decodes Version as zero and is discarded",
			data: []byte(
				`{"pipelines":{"us-east-1":{"seed":{"declaration":{"name":"seed"}}}},"pipelineARNIndex":{}}`,
			),
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := NewInMemoryBackend("000000000000", "us-east-1")
			_, err := b.CreatePipeline(cpCtxRegion("us-east-1"), samplePipelineDecl("seed-pipeline"), nil)
			require.NoError(t, err)

			err = b.Restore(t.Context(), tt.data)

			if tt.wantError {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, 0, b.PipelineCount())
		})
	}
}

// TestInMemoryBackend_SnapshotRestore_EmptyBackend verifies Snapshot/Restore
// round trips cleanly on a backend with no data at all -- every registered
// table and the raw executions/actionExecutions maps must all decode back to
// empty, not nil-panic or error.
func TestInMemoryBackend_SnapshotRestore_EmptyBackend(t *testing.T) {
	t.Parallel()

	original := NewInMemoryBackend("000000000000", "us-east-1")
	snap := original.Snapshot(t.Context())
	require.NotNil(t, snap)

	fresh := NewInMemoryBackend("000000000000", "us-east-1")
	require.NoError(t, fresh.Restore(t.Context(), snap))

	assert.Equal(t, 0, fresh.PipelineCount())
	assert.Equal(t, 0, fresh.CustomActionTypeCount())
	assert.Equal(t, 0, fresh.JobCount())
	assert.Equal(t, 0, fresh.WebhookCount())
	assert.Equal(t, 0, fresh.StageTransitionCount())

	list := fresh.ListPipelines(cpCtxRegion("us-east-1"))
	assert.Empty(t, list)
}

// TestInMemoryBackend_SnapshotRestore_FullState is the Phase 3.3 full-state
// round-trip test: it exercises every store.Table this conversion introduced
// (pipelines, customActionTypes, jobs, webhooks, stageTransitions -- each a
// flat table keyed by a composite "region|id" string, per store_setup.go)
// using the SAME resource name in TWO regions where the backend's public API
// allows it, to prove the composite key survives a round trip and keeps
// same-named resources in different regions fully isolated. It also covers
// the raw (non-store.Table) region-nested executions map, and confirms
// actionExecutions -- snapshot version 2, see persistence.go's version-2 note
// -- now survives a restore too: an approval token or in-flight action
// history would otherwise be unrecoverably lost across any snapshot/restore
// cycle, permanently stranding a paused pipeline execution.
func TestInMemoryBackend_SnapshotRestore_FullState(t *testing.T) {
	t.Parallel()

	original := NewInMemoryBackend("111122223333", "us-east-1")

	ctxEast := cpCtxRegion("us-east-1")
	ctxWest := cpCtxRegion("us-west-2")

	const (
		sharedPipeline = "shared-pipeline"
		sharedWebhook  = "shared-webhook"
		catCategory    = "Build"
		catProvider    = keyOwnerCustom
		catVersion     = "1"
	)

	// --- pipelines: same name, two regions, tags only on east.
	eastDecl := samplePipelineDecl(sharedPipeline)
	eastDecl.PipelineType = PipelineTypeV1
	eastP, err := original.CreatePipeline(ctxEast, eastDecl, map[string]string{"env": "east"})
	require.NoError(t, err)

	westDecl := samplePipelineDecl(sharedPipeline)
	westDecl.PipelineType = PipelineTypeV2
	westP, err := original.CreatePipeline(ctxWest, westDecl, map[string]string{"env": "west"})
	require.NoError(t, err)

	// --- custom action types: same category/provider/version, two regions,
	// tags only on east (mirrors the pipeline tags case above, exercising the
	// customActionTypesByARN index gopherstack-2mwl added).
	eastCAT, err := original.CreateCustomActionType(
		ctxEast, &CustomActionType{
			Category: catCategory, Provider: catProvider, Version: catVersion,
			Tags: map[string]string{"env": "east"},
		},
	)
	require.NoError(t, err)
	_, err = original.CreateCustomActionType(
		ctxWest, &CustomActionType{Category: catCategory, Provider: catProvider, Version: catVersion},
	)
	require.NoError(t, err)

	// --- webhooks: same name, two regions, tags only on east.
	eastWH, err := original.PutWebhook(ctxEast, &Webhook{
		Name: sharedWebhook, TargetPipeline: sharedPipeline, TargetAction: stageNameSource,
		Tags: map[string]string{"env": "east"},
	})
	require.NoError(t, err)
	westWH, err := original.PutWebhook(ctxWest, &Webhook{
		Name: sharedWebhook, TargetPipeline: sharedPipeline, TargetAction: stageNameSource,
	})
	require.NoError(t, err)

	// --- stage transitions: disabled only in east. Outbound (not Inbound):
	// sharedPipeline's only stage is Source, and runPipelineActions now
	// actually enforces DisableStageTransition (see action_engine.go), so
	// disabling Source's Inbound transition here would prevent the
	// StartPipelineExecution/PutActionRevision calls below from ever
	// recording an action execution at all. Outbound has no next stage to
	// gate on a single-stage pipeline, so it stays a pure persistence
	// fixture here, which is this test's actual point.
	require.NoError(t, original.DisableStageTransition(ctxEast, sharedPipeline, stageNameSource, "Outbound", "paused"))

	// --- jobs: AddJobInternal always seeds the backend's own default region (us-east-1).
	original.AddJobInternal(&Job{ID: "job-1", Status: "Queued"})

	// --- executions (raw map): started only in east; also populates the
	// actionExecutions map (version-2: also persisted, see below).
	_, err = original.StartPipelineExecution(ctxEast, sharedPipeline)
	require.NoError(t, err)

	// --- action revisions (raw map, version-2 addition): tracked only in
	// east. PutActionRevision also triggers a second execution, so the
	// "before" snapshot of actionExecutions is captured AFTER this call.
	_, _, err = original.PutActionRevision(
		ctxEast,
		sharedPipeline,
		stageNameSource,
		"SourceAction",
		"rev-1",
		"change-1",
	)
	require.NoError(t, err)

	eastActionsBefore, err := original.ListActionExecutions(ctxEast, sharedPipeline, "")
	require.NoError(t, err)
	require.NotEmpty(t, eastActionsBefore, "sanity: action executions were recorded pre-snapshot")

	snap := original.Snapshot(t.Context())
	require.NotEmpty(t, snap)

	fresh := NewInMemoryBackend("111122223333", "us-east-1")
	require.NoError(t, fresh.Restore(t.Context(), snap))

	// Composite key kept both same-named pipelines distinct rather than one
	// overwriting the other.
	assert.Equal(t, 2, fresh.PipelineCount())

	gotEast, err := fresh.GetPipeline(ctxEast, sharedPipeline)
	require.NoError(t, err)
	assert.Equal(t, PipelineTypeV1, gotEast.Declaration.PipelineType)
	assert.Contains(t, gotEast.Metadata.PipelineArn, "us-east-1")
	assert.Equal(t, eastP.Metadata.PipelineArn, gotEast.Metadata.PipelineArn)

	gotWest, err := fresh.GetPipeline(ctxWest, sharedPipeline)
	require.NoError(t, err)
	assert.Equal(t, PipelineTypeV2, gotWest.Declaration.PipelineType)
	assert.Contains(t, gotWest.Metadata.PipelineArn, "us-west-2")
	assert.Equal(t, westP.Metadata.PipelineArn, gotWest.Metadata.PipelineArn)

	// byARN index (tags/resolveResourceARN): survives per-region.
	eastTags, err := fresh.ListTagsForResource(ctxEast, eastP.Metadata.PipelineArn)
	require.NoError(t, err)
	require.Len(t, eastTags, 1)
	assert.Equal(t, "east", eastTags[0].Value)

	_, err = fresh.ListTagsForResource(ctxWest, eastP.Metadata.PipelineArn)
	require.Error(t, err, "east ARN must not be tag-resolvable from the west region")

	// customActionTypes: both regions kept their own copy.
	assert.Equal(t, 2, fresh.CustomActionTypeCount())

	_, err = fresh.GetActionType(ctxEast, catCategory, keyOwnerCustom, catProvider, catVersion)
	require.NoError(t, err)
	_, err = fresh.GetActionType(ctxWest, catCategory, keyOwnerCustom, catProvider, catVersion)
	require.NoError(t, err)

	// customActionTypesByARN index (tags/resolveResourceARN): survives
	// restore. eastCAT.arn is recomputed in Restore since the real API's
	// ActionType has no Arn field to carry it across a JSON DTO round trip.
	catTags, err := fresh.ListTagsForResource(ctxEast, eastCAT.arn)
	require.NoError(t, err)
	require.Len(t, catTags, 1)
	assert.Equal(t, "east", catTags[0].Value)

	// webhooks: both regions kept their own copy, byARN index intact.
	assert.Equal(t, 2, fresh.WebhookCount())
	require.Len(t, fresh.ListWebhooks(ctxEast), 1)
	require.Len(t, fresh.ListWebhooks(ctxWest), 1)
	assert.NotEqual(t, eastWH.ARN, westWH.ARN)

	whTags, err := fresh.ListTagsForResource(ctxEast, eastWH.ARN)
	require.NoError(t, err)
	require.Len(t, whTags, 1)
	assert.Equal(t, "east", whTags[0].Value)

	// stageTransitions: byPipeline index and region scoping intact.
	assert.Equal(t, 1, fresh.StageTransitionCount())
	east := fresh.GetStageTransitionState(ctxEast, sharedPipeline, stageNameSource, "Outbound")
	require.NotNil(t, east)
	assert.True(t, east.Disabled)
	assert.Equal(t, "paused", east.Reason)

	west := fresh.GetStageTransitionState(ctxWest, sharedPipeline, stageNameSource, "Outbound")
	assert.Nil(t, west, "west must not see the east-only disabled transition")

	// jobs: survives restore.
	assert.Equal(t, 1, fresh.JobCount())
	gotJob, err := fresh.GetJobDetails(ctxEast, "job-1")
	require.NoError(t, err)
	assert.Equal(t, "Queued", gotJob.Status)

	// executions (raw map, persisted): survives restore, region-scoped. Two
	// executions: StartPipelineExecution above, plus the one PutActionRevision
	// triggered when recording the action revision below.
	eastExecs, err := fresh.ListPipelineExecutions(ctxEast, sharedPipeline)
	require.NoError(t, err)
	require.Len(t, eastExecs, 2)

	westExecs, err := fresh.ListPipelineExecutions(ctxWest, sharedPipeline)
	require.NoError(t, err)
	assert.Empty(t, westExecs)

	// actionExecutions (raw map, now persisted -- snapshot version 2): must
	// survive the restore with the same action-execution history recorded
	// before the snapshot.
	eastActionsAfter, err := fresh.ListActionExecutions(ctxEast, sharedPipeline, "")
	require.NoError(t, err)
	assert.Equal(t, eastActionsBefore, eastActionsAfter, "actionExecutions must survive a restore")

	// actionRevisions (raw map, version-2 addition): survives restore, region-scoped.
	eastStates, err := fresh.GetPipelineState(ctxEast, sharedPipeline)
	require.NoError(t, err)
	require.Len(t, eastStates, 1)
	require.Len(t, eastStates[0].ActionStates, 1)
	eastRevision, _ := eastStates[0].ActionStates[0]["currentRevision"].(map[string]any)
	require.NotNil(t, eastRevision, "actionRevisions must survive a restore")
	assert.Equal(t, "rev-1", eastRevision["revisionId"])

	westStates, err := fresh.GetPipelineState(ctxWest, sharedPipeline)
	require.NoError(t, err)
	require.Len(t, westStates, 1)
	require.Len(t, westStates[0].ActionStates, 1)
	assert.Nil(t, westStates[0].ActionStates[0]["currentRevision"], "west must not see the east-only action revision")
}

// TestHandler_SnapshotRestoreDelegate documents that Handler.Snapshot/Restore
// (added by Phase 3.3, following the services/codecommit pattern) simply
// delegate to the backend, which is what makes cli.go's generic
// setupPersistence pick up this service without any service-specific wiring.
func TestHandler_SnapshotRestoreDelegate(t *testing.T) {
	t.Parallel()

	backend := NewInMemoryBackend("000000000000", "us-east-1")
	h := NewHandler(backend)

	_, err := backend.CreatePipeline(cpCtxRegion("us-east-1"), samplePipelineDecl("delegate-pipeline"), nil)
	require.NoError(t, err)

	snap := h.Snapshot(t.Context())
	require.NotNil(t, snap)

	backend2 := NewInMemoryBackend("000000000000", "us-east-1")
	h2 := NewHandler(backend2)
	require.NoError(t, h2.Restore(t.Context(), snap))

	got, err := h2.Backend.GetPipeline(cpCtxRegion("us-east-1"), "delegate-pipeline")
	require.NoError(t, err)
	assert.Equal(t, "delegate-pipeline", got.Declaration.Name)
}

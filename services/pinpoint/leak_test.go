package pinpoint_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/pinpoint"
)

const (
	leakRegion    = "us-east-1"
	leakAccountID = "123456789012"
)

// TestDeleteApp_ReleasesPerAppState verifies that deleting a Pinpoint app drops
// every piece of per-application state so the backing maps return to baseline.
func TestDeleteApp_ReleasesPerAppState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		nEvents  int
		campaign bool
		segment  bool
		journey  bool
	}{
		{name: "events only", nEvents: 5},
		{name: "events + campaign", nEvents: 3, campaign: true},
		{name: "full graph", nEvents: 4, campaign: true, segment: true, journey: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := pinpoint.NewInMemoryBackend(leakRegion, leakAccountID)
			baseline := pinpoint.TotalPerAppEntries(b)

			app, err := b.CreateApp(leakRegion, leakAccountID, "leak-app", nil)
			require.NoError(t, err)

			if tc.campaign {
				require.NoError(t, pinpoint.CreateCampaignForTest(b, leakRegion, leakAccountID, app.ID))
			}
			if tc.segment {
				require.NoError(t, pinpoint.CreateSegmentForTest(b, leakRegion, leakAccountID, app.ID))
			}
			if tc.journey {
				require.NoError(t, pinpoint.CreateJourneyForTest(b, leakRegion, leakAccountID, app.ID))
			}
			if tc.nEvents > 0 {
				require.NoError(t, pinpoint.PutEventsForTest(b, app.ID, tc.nEvents))
			}

			require.Greater(t, pinpoint.TotalPerAppEntries(b), baseline,
				"per-app state should have accumulated before delete")

			_, err = b.DeleteApp(app.ID)
			require.NoError(t, err)

			require.Equal(t, baseline, pinpoint.TotalPerAppEntries(b),
				"all per-app state must be released when the app is deleted")
		})
	}
}

// TestPutEvents_CapsAppEvents verifies that ingested events are bounded per app
// so the appEvents slice cannot grow without limit.
func TestPutEvents_CapsAppEvents(t *testing.T) {
	t.Parallel()

	b := pinpoint.NewInMemoryBackend(leakRegion, leakAccountID)
	app, err := b.CreateApp(leakRegion, leakAccountID, "cap-app", nil)
	require.NoError(t, err)

	const batches = 3
	for range batches {
		require.NoError(t, pinpoint.PutEventsForTest(b, app.ID, pinpoint.MaxAppEvents))
	}

	require.LessOrEqual(t, pinpoint.AppEventCount(b, app.ID), pinpoint.MaxAppEvents,
		"retained events must not exceed the cap")
}

// TestDeleteCampaign_ReleasesVersionHistory verifies that deleting a campaign
// removes its version history entry so the campaignVersions map returns to baseline.
func TestDeleteCampaign_ReleasesVersionHistory(t *testing.T) {
	t.Parallel()

	b := pinpoint.NewInMemoryBackend(leakRegion, leakAccountID)
	app, err := b.CreateApp(leakRegion, leakAccountID, "campaign-leak-app", nil)
	require.NoError(t, err)

	campaignID, err := pinpoint.CreateCampaignIDForTest(b, leakRegion, leakAccountID, app.ID)
	require.NoError(t, err)

	require.Equal(t, 1, pinpoint.CampaignVersionCount(b, app.ID, campaignID),
		"version entry must exist after create")

	_, err = b.DeleteCampaign(app.ID, campaignID)
	require.NoError(t, err)

	require.Equal(t, 0, pinpoint.CampaignVersionCount(b, app.ID, campaignID),
		"version entry must be removed after delete")
}

// TestDeleteCampaign_ReleasesActivities verifies that deleting a campaign
// removes its campaignActivities entry (created unconditionally by
// CreateCampaign) so the map does not grow without bound as campaigns churn.
func TestDeleteCampaign_ReleasesActivities(t *testing.T) {
	t.Parallel()

	b := pinpoint.NewInMemoryBackend(leakRegion, leakAccountID)
	app, err := b.CreateApp(leakRegion, leakAccountID, "campaign-activity-leak-app", nil)
	require.NoError(t, err)

	campaignID, err := pinpoint.CreateCampaignIDForTest(b, leakRegion, leakAccountID, app.ID)
	require.NoError(t, err)

	require.Equal(t, 1, pinpoint.CampaignActivityCount(b, app.ID, campaignID),
		"activity entry must exist after create")

	_, err = b.DeleteCampaign(app.ID, campaignID)
	require.NoError(t, err)

	require.Equal(t, 0, pinpoint.CampaignActivityCount(b, app.ID, campaignID),
		"activity entry must be removed after delete")
}

// TestDeleteJourney_ReleasesRuns verifies that deleting a journey removes its
// journeyRuns entry (created when the journey is transitioned to ACTIVE) so
// the map does not grow without bound as journeys churn.
func TestDeleteJourney_ReleasesRuns(t *testing.T) {
	t.Parallel()

	b := pinpoint.NewInMemoryBackend(leakRegion, leakAccountID)
	app, err := b.CreateApp(leakRegion, leakAccountID, "journey-run-leak-app", nil)
	require.NoError(t, err)

	journeyID, err := pinpoint.CreateJourneyIDForTest(b, leakRegion, leakAccountID, app.ID)
	require.NoError(t, err)

	_, err = b.UpdateJourneyState(app.ID, journeyID, "ACTIVE")
	require.NoError(t, err)

	require.Equal(t, 1, pinpoint.JourneyRunCount(b, app.ID, journeyID),
		"run entry must exist after activation")

	_, err = b.DeleteJourney(app.ID, journeyID)
	require.NoError(t, err)

	require.Equal(t, 0, pinpoint.JourneyRunCount(b, app.ID, journeyID),
		"run entry must be removed after delete")
}

// TestDeleteSegment_ReleasesVersionHistory verifies that deleting a segment
// removes its version history entry so the segmentVersions map returns to baseline.
func TestDeleteSegment_ReleasesVersionHistory(t *testing.T) {
	t.Parallel()

	b := pinpoint.NewInMemoryBackend(leakRegion, leakAccountID)
	app, err := b.CreateApp(leakRegion, leakAccountID, "segment-leak-app", nil)
	require.NoError(t, err)

	segmentID, err := pinpoint.CreateSegmentIDForTest(b, leakRegion, leakAccountID, app.ID)
	require.NoError(t, err)

	require.Equal(t, 1, pinpoint.SegmentVersionCount(b, app.ID, segmentID),
		"version entry must exist after create")

	_, err = b.DeleteSegment(app.ID, segmentID)
	require.NoError(t, err)

	require.Equal(t, 0, pinpoint.SegmentVersionCount(b, app.ID, segmentID),
		"version entry must be removed after delete")
}

// TestDeleteEmailTemplate_ReleasesVersionHistory verifies that deleting a template
// removes its version history from templateVersionHistory.
func TestDeleteEmailTemplate_ReleasesVersionHistory(t *testing.T) {
	t.Parallel()

	b := pinpoint.NewInMemoryBackend(leakRegion, leakAccountID)

	require.NoError(t, pinpoint.CreateEmailTemplateForTest(b, leakRegion, leakAccountID, "my-tpl"))

	require.Equal(t, 1, pinpoint.TemplateVersionCount(b, "my-tpl", "EMAIL"),
		"version entry must exist after create")

	require.NoError(t, pinpoint.DeleteEmailTemplateForTest(b, "my-tpl"))

	require.Equal(t, 0, pinpoint.TemplateVersionCount(b, "my-tpl", "EMAIL"),
		"version history must be removed after delete")
}

// TestUpdateEmailTemplate_CapsVersionHistory verifies that repeated updates to a
// template do not grow templateVersionHistory beyond maxTemplateVersions.
func TestUpdateEmailTemplate_CapsVersionHistory(t *testing.T) {
	t.Parallel()

	b := pinpoint.NewInMemoryBackend(leakRegion, leakAccountID)

	require.NoError(t, pinpoint.CreateEmailTemplateForTest(b, leakRegion, leakAccountID, "cap-tpl"))

	const updates = pinpoint.MaxTemplateVersions * 2
	for range updates {
		require.NoError(t, pinpoint.UpdateEmailTemplateForTest(b, "cap-tpl"))
	}

	count := pinpoint.TemplateVersionCount(b, "cap-tpl", "EMAIL")
	require.LessOrEqual(t, count, pinpoint.MaxTemplateVersions,
		"template version history must not exceed the cap")
}

// TestDeleteVoiceTemplate_ReleasesVersionHistory verifies that deleting a
// voice template removes its version history from templateVersionHistory,
// mirroring TestDeleteEmailTemplate_ReleasesVersionHistory. DeleteVoiceTemplate
// previously omitted this cleanup (unlike Delete{Email,InApp,Push,Sms}Template,
// which all clean up their own version-history entry) -- fixed as part of
// bringing voice templates to full field parity with the other template
// types, and locked here so it cannot regress.
func TestDeleteVoiceTemplate_ReleasesVersionHistory(t *testing.T) {
	t.Parallel()

	b := pinpoint.NewInMemoryBackend(leakRegion, leakAccountID)

	require.NoError(t, pinpoint.CreateVoiceTemplateForTest(b, leakRegion, leakAccountID, "my-voice-tpl", "hello"))

	require.Equal(t, 1, pinpoint.TemplateVersionCount(b, "my-voice-tpl", "VOICE"),
		"version entry must exist after create")

	_, err := b.DeleteVoiceTemplate("my-voice-tpl")
	require.NoError(t, err)

	require.Equal(t, 0, pinpoint.TemplateVersionCount(b, "my-voice-tpl", "VOICE"),
		"version history must be removed after delete")
}

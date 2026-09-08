package personalize_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPersonalize_Campaign_CRUD(t *testing.T) {
	t.Parallel()

	h := personalizeHandler(t)

	svArn := personalizeCreateSolutionVersion(t, h, "sol")
	newSvArn := personalizeCreateSolutionVersion(t, h, "sol-v2")

	rec := personalizeDo(t, h, "CreateCampaign", map[string]any{
		"name":               "my-campaign",
		"solutionVersionArn": svArn,
		"minProvisionedTPS":  float64(5),
	})
	require.Equal(t, http.StatusOK, rec.Code)
	campArn := personalizeUnmarshal(t, rec)["campaignArn"].(string)
	assert.NotEmpty(t, campArn)

	rec = personalizeDo(t, h, "DescribeCampaign", map[string]any{"campaignArn": campArn})
	require.Equal(t, http.StatusOK, rec.Code)
	c := personalizeUnmarshal(t, rec)["campaign"].(map[string]any)
	assert.Equal(t, "my-campaign", c["name"])
	assert.Equal(t, svArn, c["solutionVersionArn"])
	assert.InDelta(t, float64(5), c["minProvisionedTPS"], 0)
	assert.Equal(t, "ACTIVE", c["status"])

	// Update
	rec = personalizeDo(t, h, "UpdateCampaign", map[string]any{
		"campaignArn":        campArn,
		"solutionVersionArn": newSvArn,
		"minProvisionedTPS":  float64(10),
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = personalizeDo(t, h, "DescribeCampaign", map[string]any{"campaignArn": campArn})
	c = personalizeUnmarshal(t, rec)["campaign"].(map[string]any)
	assert.Equal(t, newSvArn, c["solutionVersionArn"])
	assert.InDelta(t, float64(10), c["minProvisionedTPS"], 0)

	// List
	rec = personalizeDo(t, h, "ListCampaigns", map[string]any{})
	listed := personalizeUnmarshal(t, rec)
	assert.Len(t, listed["campaigns"].([]any), 1)

	// Delete
	rec = personalizeDo(t, h, "DeleteCampaign", map[string]any{"campaignArn": campArn})
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestPersonalize_Campaign_ConfigAndLatestUpdate locks two previously-missing
// wire fields: campaignConfig (the enableMetadataWithRecommendations/
// itemExplorationConfig/etc. sub-object on CreateCampaignInput/
// UpdateCampaignInput) round-trips on Describe, and latestCampaignUpdate
// (types.CampaignUpdateSummary) appears only after the campaign has had at
// least one UpdateCampaign call, matching the real API's doc comment.
func TestPersonalize_Campaign_ConfigAndLatestUpdate(t *testing.T) {
	t.Parallel()

	h := personalizeHandler(t)
	svArn := personalizeCreateSolutionVersion(t, h, "cfg-sol")

	rec := personalizeDo(t, h, "CreateCampaign", map[string]any{
		"name":               "cfg-camp",
		"solutionVersionArn": svArn,
		"campaignConfig": map[string]any{
			"enableMetadataWithRecommendations": true,
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	campArn := personalizeUnmarshal(t, rec)["campaignArn"].(string)

	rec = personalizeDo(t, h, "DescribeCampaign", map[string]any{"campaignArn": campArn})
	require.Equal(t, http.StatusOK, rec.Code)
	c := personalizeUnmarshal(t, rec)["campaign"].(map[string]any)
	cfg, ok := c["campaignConfig"].(map[string]any)
	require.True(t, ok, "campaignConfig must round-trip on Describe")
	assert.Equal(t, true, cfg["enableMetadataWithRecommendations"])
	// Never updated yet -- latestCampaignUpdate must be absent, not a
	// fabricated empty object.
	assert.NotContains(t, c, "latestCampaignUpdate")

	newSvArn := personalizeCreateSolutionVersion(t, h, "cfg-sol-2")
	rec = personalizeDo(t, h, "UpdateCampaign", map[string]any{
		"campaignArn":        campArn,
		"solutionVersionArn": newSvArn,
		"campaignConfig": map[string]any{
			"enableMetadataWithRecommendations": false,
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = personalizeDo(t, h, "DescribeCampaign", map[string]any{"campaignArn": campArn})
	c = personalizeUnmarshal(t, rec)["campaign"].(map[string]any)
	update, ok := c["latestCampaignUpdate"].(map[string]any)
	require.True(t, ok, "latestCampaignUpdate must appear after an UpdateCampaign call")
	assert.Equal(t, newSvArn, update["solutionVersionArn"])
	assert.Equal(t, "ACTIVE", update["status"])
	assert.NotEmpty(t, update["creationDateTime"])
}

// TestPersonalize_Campaign_LatestSolutionVersionFormat locks that
// CreateCampaign and UpdateCampaign accept the documented
// SolutionArn/$LATEST shorthand for solutionVersionArn
// (api_op_CreateCampaign.go field doc on SolutionVersionArn: "To specify the
// latest solution version of your solution, specify the ARN of your
// solution in SolutionArn/$LATEST format.") and resolve it to the solution's
// actual latest version.
func TestPersonalize_Campaign_LatestSolutionVersionFormat(t *testing.T) {
	t.Parallel()

	h := personalizeHandler(t)
	svArn := personalizeCreateSolutionVersion(t, h, "latest-sol")
	solArn := svArn[:strings.LastIndex(svArn, "/")]

	rec := personalizeDo(t, h, "CreateCampaign", map[string]any{
		"name":               "latest-campaign",
		"solutionVersionArn": solArn + "/$LATEST",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	campArn := personalizeUnmarshal(t, rec)["campaignArn"].(string)

	rec = personalizeDo(t, h, "DescribeCampaign", map[string]any{"campaignArn": campArn})
	require.Equal(t, http.StatusOK, rec.Code)
	c := personalizeUnmarshal(t, rec)["campaign"].(map[string]any)
	assert.Equal(t, svArn, c["solutionVersionArn"])

	newSvArn := personalizeDo(t, h, "CreateSolutionVersion", map[string]any{"solutionArn": solArn})
	require.Equal(t, http.StatusOK, newSvArn.Code)
	latestSvArn := personalizeUnmarshal(t, newSvArn)["solutionVersionArn"].(string)
	require.NotEqual(t, svArn, latestSvArn)

	rec = personalizeDo(t, h, "UpdateCampaign", map[string]any{
		"campaignArn":        campArn,
		"solutionVersionArn": solArn + "/$LATEST",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = personalizeDo(t, h, "DescribeCampaign", map[string]any{"campaignArn": campArn})
	c = personalizeUnmarshal(t, rec)["campaign"].(map[string]any)
	assert.Equal(t, latestSvArn, c["solutionVersionArn"])
}

// --- EventTracker ---

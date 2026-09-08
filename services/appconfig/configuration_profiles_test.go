package appconfig_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/appconfig"
)

// seedExperimentApp creates an application, environment, and a
// feature-flag configuration profile -- everything
// CreateExperimentDefinition needs -- returning their IDs.
func seedExperimentApp(t *testing.T, b *appconfig.InMemoryBackend) (string, string, string) {
	t.Helper()

	app, err := b.CreateApplication("exp-app", "", nil)
	require.NoError(t, err)

	env, err := b.CreateEnvironment(app.ID, "exp-env", "", nil, nil)
	require.NoError(t, err)

	profile, err := b.CreateConfigurationProfile(
		app.ID, "exp-profile", "", "hosted", "AWS.AppConfig.FeatureFlags", "", "", nil,
		nil,
	)
	require.NoError(t, err)

	return app.ID, env.ID, profile.ID
}

// experimentTreatment builds a *appconfig.Treatment carrying the given
// enabled flag value and weight, for use as an experiment definition's
// control or one of its treatments.
func experimentTreatment(enabled bool, weight float32) *appconfig.Treatment {
	return &appconfig.Treatment{FlagValue: &appconfig.FlagValue{Enabled: enabled}, Weight: weight}
}

// experimentDefParams is the mutable set of CreateExperimentDefinition
// arguments TestBackend_CreateExperimentDefinition_Validation's cases start
// from a valid baseline and selectively break.
type experimentDefParams struct {
	control      *appconfig.Treatment
	appID        string
	envID        string
	profileID    string
	name         string
	audienceRule string
	flagKey      string
	treatments   []appconfig.Treatment
}

// TestBackend_CreateExperimentDefinition_Validation covers every required-
// field and reference-validation rule CreateExperimentDefinition enforces:
// the "This member is required" fields real CreateExperimentDefinitionInput
// marks, and that ApplicationIdentifier/EnvironmentIdentifier/
// ConfigurationProfileIdentifier are validated against real backend state
// (not accepted as any string) including the feature-flag profile Type
// check.
func TestBackend_CreateExperimentDefinition_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		mutate  func(t *testing.T, b *appconfig.InMemoryBackend, p *experimentDefParams)
		name    string
		wantErr bool
	}{
		{name: "valid baseline", wantErr: false},
		{
			name:    "missing name",
			mutate:  func(_ *testing.T, _ *appconfig.InMemoryBackend, p *experimentDefParams) { p.name = "" },
			wantErr: true,
		},
		{
			name: "missing audience rule",
			mutate: func(_ *testing.T, _ *appconfig.InMemoryBackend, p *experimentDefParams) {
				p.audienceRule = ""
			},
			wantErr: true,
		},
		{
			name:    "missing flag key",
			mutate:  func(_ *testing.T, _ *appconfig.InMemoryBackend, p *experimentDefParams) { p.flagKey = "" },
			wantErr: true,
		},
		{
			name:    "nil control",
			mutate:  func(_ *testing.T, _ *appconfig.InMemoryBackend, p *experimentDefParams) { p.control = nil },
			wantErr: true,
		},
		{
			name: "control missing flag value",
			mutate: func(_ *testing.T, _ *appconfig.InMemoryBackend, p *experimentDefParams) {
				p.control = &appconfig.Treatment{Weight: 100}
			},
			wantErr: true,
		},
		{
			name: "no treatments",
			mutate: func(_ *testing.T, _ *appconfig.InMemoryBackend, p *experimentDefParams) {
				p.treatments = nil
			},
			wantErr: true,
		},
		{
			name: "treatment missing flag value",
			mutate: func(_ *testing.T, _ *appconfig.InMemoryBackend, p *experimentDefParams) {
				p.treatments = []appconfig.Treatment{{Weight: 50}}
			},
			wantErr: true,
		},
		{
			name: "application not found",
			mutate: func(_ *testing.T, _ *appconfig.InMemoryBackend, p *experimentDefParams) {
				p.appID = "missing-app"
			},
			wantErr: true,
		},
		{
			name: "environment not found",
			mutate: func(_ *testing.T, _ *appconfig.InMemoryBackend, p *experimentDefParams) {
				p.envID = "missing-env"
			},
			wantErr: true,
		},
		{
			name: "environment from a different application rejected",
			mutate: func(t *testing.T, b *appconfig.InMemoryBackend, p *experimentDefParams) {
				t.Helper()

				otherApp, err := b.CreateApplication("other-app", "", nil)
				require.NoError(t, err)
				otherEnv, err := b.CreateEnvironment(otherApp.ID, "other-env", "", nil, nil)
				require.NoError(t, err)
				p.envID = otherEnv.ID
			},
			wantErr: true,
		},
		{
			name: "profile not found",
			mutate: func(_ *testing.T, _ *appconfig.InMemoryBackend, p *experimentDefParams) {
				p.profileID = "missing-profile"
			},
			wantErr: true,
		},
		{
			name: "profile not a feature-flag type",
			mutate: func(t *testing.T, b *appconfig.InMemoryBackend, p *experimentDefParams) {
				t.Helper()

				prof, err := b.CreateConfigurationProfile(
					p.appID, "freeform-profile", "", "hosted", "AWS.Freeform", "", "", nil,
					nil,
				)
				require.NoError(t, err)
				p.profileID = prof.ID
			},
			wantErr: true,
		},
		{
			// CreateExperimentDefinitionInput.FlagKey's doc comment reads
			// "The key of the existing feature flag to use with the
			// experiment" -- once the profile has real
			// AWS.AppConfig.FeatureFlags content uploaded, a flag key absent
			// from that content's "flags" object is rejected instead of
			// only checked for non-emptiness.
			name: "flag key not present in feature flag content rejected",
			mutate: func(t *testing.T, b *appconfig.InMemoryBackend, p *experimentDefParams) {
				t.Helper()

				content := `{"version":"1","flags":{"otherflag":{"name":"Other"}},` +
					`"values":{"otherflag":{"enabled":true}}}`
				_, err := b.CreateHostedConfigurationVersion(
					p.appID, p.profileID, "application/json", "", "", []byte(content), nil,
				)
				require.NoError(t, err)
			},
			wantErr: true,
		},
		{
			name: "flag key present in feature flag content accepted",
			mutate: func(t *testing.T, b *appconfig.InMemoryBackend, p *experimentDefParams) {
				t.Helper()

				content := `{"version":"1","flags":{"flag1":{"name":"Flag One"}},` +
					`"values":{"flag1":{"enabled":true}}}`
				_, err := b.CreateHostedConfigurationVersion(
					p.appID, p.profileID, "application/json", "", "", []byte(content), nil,
				)
				require.NoError(t, err)
			},
			wantErr: false,
		},
		{
			// Content that isn't structured AWS.AppConfig.FeatureFlags JSON
			// (freeform text/bytes, matching how CreateHostedConfigurationVersion
			// accepts arbitrary content for any profile) cannot be checked
			// against, so this stays permissive rather than rejecting --
			// same "unspecified, not wrong" treatment as no content at all.
			name: "non-feature-flag content stays permissive",
			mutate: func(t *testing.T, b *appconfig.InMemoryBackend, p *experimentDefParams) {
				t.Helper()

				_, err := b.CreateHostedConfigurationVersion(
					p.appID, p.profileID, "text/plain", "", "", []byte("not json"), nil,
				)
				require.NoError(t, err)
			},
			wantErr: false,
		},
		{
			name: "duplicate name in application rejected",
			mutate: func(t *testing.T, b *appconfig.InMemoryBackend, p *experimentDefParams) {
				t.Helper()

				_, err := b.CreateExperimentDefinition(
					p.appID, p.name, p.envID, p.profileID, p.flagKey, p.audienceRule, "", "", "",
					p.control, p.treatments, nil,
				)
				require.NoError(t, err)
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := appconfig.NewInMemoryBackend("123456789012", "us-east-1")
			appID, envID, profileID := seedExperimentApp(t, b)

			p := &experimentDefParams{
				appID: appID, envID: envID, profileID: profileID,
				name: "exp-def", audienceRule: "true", flagKey: "flag1",
				control:    experimentTreatment(false, 100),
				treatments: []appconfig.Treatment{*experimentTreatment(true, 100)},
			}
			if tc.mutate != nil {
				tc.mutate(t, b, p)
			}

			def, err := b.CreateExperimentDefinition(
				p.appID, p.name, p.envID, p.profileID, p.flagKey,
				p.audienceRule, "", "", "",
				p.control, p.treatments, nil,
			)

			if tc.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, "IDLE", def.Status)
			assert.Equal(t, "Control", def.Control.Key)
			assert.Equal(t, "Treatment1", def.Treatments[0].Key)
		})
	}
}

// TestBackend_CreateExperimentDefinition_TagsAppliedInline verifies that
// inline Tags passed to CreateExperimentDefinition are actually applied
// (unlike the six pre-existing Create* handlers tracked by bd
// gopherstack-lcan, which silently drop them).
func TestBackend_CreateExperimentDefinition_TagsAppliedInline(t *testing.T) {
	t.Parallel()

	b := appconfig.NewInMemoryBackend("123456789012", "us-east-1")
	appID, envID, profileID := seedExperimentApp(t, b)

	def, err := b.CreateExperimentDefinition(
		appID, "tagged-def", envID, profileID, "flag1", "true", "", "", "",
		experimentTreatment(false, 100), []appconfig.Treatment{*experimentTreatment(true, 100)},
		map[string]string{"team": "growth"},
	)
	require.NoError(t, err)

	arn := "arn:aws:appconfig:us-east-1:123456789012:application/" + appID + "/experimentdefinition/" + def.ID
	tags, err := b.ListTagsForResource(arn)
	require.NoError(t, err)
	assert.Equal(t, "growth", tags["team"])
}

// TestBackend_ExperimentDefinition_GetByNameAndUpdate verifies that
// GetExperimentDefinition resolves by name as well as ID, and that
// UpdateExperimentDefinition applies nil-means-unchanged semantics and
// rejects further updates while a run is active.
func TestBackend_ExperimentDefinition_GetByNameAndUpdate(t *testing.T) {
	t.Parallel()

	b := appconfig.NewInMemoryBackend("123456789012", "us-east-1")
	appID, envID, profileID := seedExperimentApp(t, b)

	def, err := b.CreateExperimentDefinition(
		appID, "lookup-def", envID, profileID, "flag1", "true", "everyone", "h0", "lc0",
		experimentTreatment(false, 100), []appconfig.Treatment{*experimentTreatment(true, 100)},
		nil,
	)
	require.NoError(t, err)

	byName, err := b.GetExperimentDefinition(appID, "lookup-def")
	require.NoError(t, err)
	assert.Equal(t, def.ID, byName.ID)

	newRule := "country == 'US'"
	updated, err := b.UpdateExperimentDefinition(appID, def.ID, nil, &newRule, nil, nil, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, newRule, updated.AudienceRule)
	assert.Equal(t, "everyone", updated.AudienceDescription, "nil field must be left unchanged")
	assert.Equal(t, "h0", updated.Hypothesis, "nil field must be left unchanged")

	newTreatments := []appconfig.Treatment{*experimentTreatment(true, 30), *experimentTreatment(true, 70)}
	updated, err = b.UpdateExperimentDefinition(appID, def.ID, nil, nil, nil, nil, nil, &newTreatments)
	require.NoError(t, err)
	require.Len(t, updated.Treatments, 2)
	assert.Equal(t, "Treatment1", updated.Treatments[0].Key)
	assert.Equal(t, "Treatment2", updated.Treatments[1].Key)

	exposure := float32(0)
	_, err = b.StartExperimentRun(appID, def.ID, "", &exposure, nil, nil)
	require.NoError(t, err)

	_, err = b.UpdateExperimentDefinition(appID, def.ID, nil, &newRule, nil, nil, nil, nil)
	require.Error(t, err, "must reject updates while a run is active")
}

// TestBackend_DeleteExperimentDefinition_DeleteType covers ARCHIVE (the
// default this backend applies when delete_type is omitted) vs DESTROY:
// ARCHIVE leaves the definition gettable with Status ARCHIVED, DESTROY
// permanently removes it.
func TestBackend_DeleteExperimentDefinition_DeleteType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		deleteType      string
		wantStatusAfter string
		wantDestroyed   bool
	}{
		{name: "empty defaults to archive", deleteType: "", wantStatusAfter: "ARCHIVED"},
		{name: "explicit archive", deleteType: "ARCHIVE", wantStatusAfter: "ARCHIVED"},
		{name: "explicit destroy", deleteType: "DESTROY", wantDestroyed: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := appconfig.NewInMemoryBackend("123456789012", "us-east-1")
			appID, envID, profileID := seedExperimentApp(t, b)

			def, err := b.CreateExperimentDefinition(
				appID, "del-def", envID, profileID, "flag1", "true", "", "", "",
				experimentTreatment(false, 100), []appconfig.Treatment{*experimentTreatment(true, 100)},
				nil,
			)
			require.NoError(t, err)

			require.NoError(t, b.DeleteExperimentDefinition(appID, def.ID, tc.deleteType))

			got, err := b.GetExperimentDefinition(appID, def.ID)
			if tc.wantDestroyed {
				require.Error(t, err, "DESTROY must permanently remove the definition")

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantStatusAfter, got.Status)
		})
	}
}

// TestBackend_DeleteExperimentDefinition_InvalidDeleteType verifies that an
// unrecognized delete_type is rejected rather than silently defaulting.
func TestBackend_DeleteExperimentDefinition_InvalidDeleteType(t *testing.T) {
	t.Parallel()

	b := appconfig.NewInMemoryBackend("123456789012", "us-east-1")
	appID, envID, profileID := seedExperimentApp(t, b)

	def, err := b.CreateExperimentDefinition(
		appID, "bad-delete-type-def", envID, profileID, "flag1", "true", "", "", "",
		experimentTreatment(false, 100), []appconfig.Treatment{*experimentTreatment(true, 100)},
		nil,
	)
	require.NoError(t, err)

	err = b.DeleteExperimentDefinition(appID, def.ID, "BOGUS")
	require.Error(t, err)
}

// TestBackend_DeleteExperimentDefinition_DestroyCascadesRunsAndTags verifies
// that DESTROY removes every run scoped to the definition (and its
// tags/events), not just the definition row itself -- otherwise repeated
// create/destroy cycles would leak ghost run rows.
func TestBackend_DeleteExperimentDefinition_DestroyCascadesRunsAndTags(t *testing.T) {
	t.Parallel()

	b := appconfig.NewInMemoryBackend("123456789012", "us-east-1")
	appID, envID, profileID := seedExperimentApp(t, b)

	def, err := b.CreateExperimentDefinition(
		appID, "cascade-def", envID, profileID, "flag1", "true", "", "", "",
		experimentTreatment(false, 100), []appconfig.Treatment{*experimentTreatment(true, 100)},
		nil,
	)
	require.NoError(t, err)

	exposure := float32(5)
	run, err := b.StartExperimentRun(appID, def.ID, "", &exposure, nil, map[string]string{"env": "prod"})
	require.NoError(t, err)

	runArn := "arn:aws:appconfig:us-east-1:123456789012:application/" + appID +
		"/experimentdefinition/" + def.ID + "/experimentrun/1"
	tags, err := b.ListTagsForResource(runArn)
	require.NoError(t, err)
	require.Equal(t, "prod", tags["env"])

	require.NoError(t, b.DeleteExperimentDefinition(appID, def.ID, "DESTROY"))

	_, err = b.GetExperimentRun(appID, def.ID, run.Run)
	require.Error(t, err, "DESTROY must remove runs scoped to the definition")

	tagsAfter, err := b.ListTagsForResource(runArn)
	require.NoError(t, err)
	assert.Empty(t, tagsAfter, "DESTROY must remove tags scoped to deleted runs")
}

func TestBackend_GetConfigurationProfile_NotFound(t *testing.T) {
	t.Parallel()

	b := appconfig.NewInMemoryBackend("123456789012", "us-east-1")
	_, err := b.GetConfigurationProfile("app-1", "prof-1")
	require.Error(t, err)
}

func TestBackend_ListConfigurationProfiles_AppNotFound(t *testing.T) {
	t.Parallel()

	b := appconfig.NewInMemoryBackend("123456789012", "us-east-1")
	_, _, err := b.ListConfigurationProfiles("nonexistent", "", "", 0)
	require.Error(t, err)
}

func TestBackend_UpdateConfigurationProfile_NotFound(t *testing.T) {
	t.Parallel()

	b := appconfig.NewInMemoryBackend("123456789012", "us-east-1")
	_, err := b.UpdateConfigurationProfile("app-1", "prof-1", new("name"), new(""), nil, nil, nil)
	require.Error(t, err)
}

func TestBackend_DeleteConfigurationProfile_NotFound(t *testing.T) {
	t.Parallel()

	b := appconfig.NewInMemoryBackend("123456789012", "us-east-1")
	err := b.DeleteConfigurationProfile("app-1", "prof-1", "")
	require.Error(t, err)
}

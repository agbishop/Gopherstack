package appconfig_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/services/appconfig"
	"github.com/blackbirdworks/gopherstack/services/appconfigdata"
)

// deletionProtectionSiblings implements the appconfig package's unexported
// siblingServices interface structurally (GetAppConfigDataHandler() service.Registerable),
// the same way *CLI does in production -- see services/mgn/cross_service.go
// for the established pattern this mirrors.
type deletionProtectionSiblings struct {
	acdHandler *appconfigdata.Handler
}

func (s *deletionProtectionSiblings) GetAppConfigDataHandler() service.Registerable {
	return s.acdHandler
}

// deletionProtectionFixture wires a real AppConfig backend to a real
// AppConfigData backend via SetAppConfig -- the same lazy sibling-lookup
// gopherstack-z4v1 says codedeploy/ec2/grafana/mgn/resiliencehub/guardduty
// already use -- around one application/environment/configuration profile
// with account-level deletion protection enabled.
type deletionProtectionFixture struct {
	h         *appconfig.Handler
	acd       *appconfigdata.InMemoryBackend
	appID     string
	envID     string
	profileID string
}

func newDeletionProtectionFixture(t *testing.T) *deletionProtectionFixture {
	t.Helper()

	backend := appconfig.NewInMemoryBackend("123456789012", "us-east-1")
	acd := appconfigdata.NewInMemoryBackend()
	backend.SetAppConfig(&deletionProtectionSiblings{acdHandler: appconfigdata.NewHandler(acd)})

	_, err := backend.UpdateAccountSettings(
		&appconfig.DeletionProtectionSettings{Enabled: aws.Bool(true), ProtectionPeriodInMinutes: aws.Int32(60)},
		nil,
	)
	require.NoError(t, err)

	h := appconfig.NewHandler(backend)

	rec := doRequest(t, h, http.MethodPost, "/applications", []byte(`{"name":"dp-app"}`))
	require.Equal(t, http.StatusCreated, rec.Code)

	var app appconfig.Application
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &app))

	rec = doRequest(t, h, http.MethodPost, "/applications/"+app.ID+"/environments", []byte(`{"name":"dp-env"}`))
	require.Equal(t, http.StatusCreated, rec.Code)

	var env appconfig.Environment
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))

	rec = doRequest(t, h, http.MethodPost, "/applications/"+app.ID+"/configurationprofiles",
		[]byte(`{"name":"dp-profile","locationUri":"hosted","type":"AWS.Freeform"}`))
	require.Equal(t, http.StatusCreated, rec.Code)

	var profile appconfig.ConfigurationProfile
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &profile))

	return &deletionProtectionFixture{h: h, acd: acd, appID: app.ID, envID: env.ID, profileID: profile.ID}
}

// recordRecentRead simulates a client that called GetLatestConfiguration
// against f's profile moments ago, the exact signal DeletionProtectionCheck
// keys off (DeletionProtectionSettings doc, appconfig@v1.48.4
// types/types.go:222-233).
func (f *deletionProtectionFixture) recordRecentRead(t *testing.T) {
	t.Helper()

	require.NoError(t, f.acd.SetConfiguration(f.appID, f.envID, f.profileID, "content", "text/plain"))

	token, err := f.acd.StartSession(f.appID, f.envID, f.profileID, 0)
	require.NoError(t, err)

	_, _, _, _, _, err = f.acd.GetLatestConfiguration(token)
	require.NoError(t, err)
}

// TestHandler_DeleteEnvironment_DeletionProtectionCheck_EnforcesRecentRead
// proves DeleteEnvironment actually blocks on a recent GetLatestConfiguration
// call once the appconfig -> appconfigdata sibling handle is wired
// (gopherstack-z4v1): APPLY forces the check to run even against a
// resource created within the past hour and must reject with
// ConflictException while leaving the environment intact; BYPASS must
// still delete despite the same recent read.
func TestHandler_DeleteEnvironment_DeletionProtectionCheck_EnforcesRecentRead(t *testing.T) {
	t.Parallel()

	t.Run("apply blocks a recently read environment", func(t *testing.T) {
		t.Parallel()

		f := newDeletionProtectionFixture(t)
		f.recordRecentRead(t)

		path := "/applications/" + f.appID + "/environments/" + f.envID
		delRec := doRequestWithHeader(t, f.h, http.MethodDelete, path,
			"X-Amzn-Deletion-Protection-Check", "APPLY", nil)

		require.Equal(t, http.StatusConflict, delRec.Code)
		assert.Equal(t, "ConflictException", delRec.Header().Get("X-Amzn-Errortype"))

		getRec := doRequest(t, f.h, http.MethodGet, path, nil)
		assert.Equal(t, http.StatusOK, getRec.Code, "environment must survive a blocked delete")
	})

	t.Run("bypass deletes despite a recent read", func(t *testing.T) {
		t.Parallel()

		f := newDeletionProtectionFixture(t)
		f.recordRecentRead(t)

		path := "/applications/" + f.appID + "/environments/" + f.envID
		delRec := doRequestWithHeader(t, f.h, http.MethodDelete, path,
			"X-Amzn-Deletion-Protection-Check", "BYPASS", nil)

		require.Equal(t, http.StatusNoContent, delRec.Code)

		getRec := doRequest(t, f.h, http.MethodGet, path, nil)
		assert.Equal(t, http.StatusNotFound, getRec.Code, "environment should have been deleted")
	})
}

// TestHandler_DeleteConfigurationProfile_DeletionProtectionCheck_EnforcesRecentRead
// is DeleteEnvironment's sibling test scoped to DeleteConfigurationProfile.
func TestHandler_DeleteConfigurationProfile_DeletionProtectionCheck_EnforcesRecentRead(t *testing.T) {
	t.Parallel()

	t.Run("apply blocks a recently read profile", func(t *testing.T) {
		t.Parallel()

		f := newDeletionProtectionFixture(t)
		f.recordRecentRead(t)

		path := "/applications/" + f.appID + "/configurationprofiles/" + f.profileID
		delRec := doRequestWithHeader(t, f.h, http.MethodDelete, path,
			"X-Amzn-Deletion-Protection-Check", "APPLY", nil)

		require.Equal(t, http.StatusConflict, delRec.Code)
		assert.Equal(t, "ConflictException", delRec.Header().Get("X-Amzn-Errortype"))

		getRec := doRequest(t, f.h, http.MethodGet, path, nil)
		assert.Equal(t, http.StatusOK, getRec.Code, "profile must survive a blocked delete")
	})

	t.Run("bypass deletes despite a recent read", func(t *testing.T) {
		t.Parallel()

		f := newDeletionProtectionFixture(t)
		f.recordRecentRead(t)

		path := "/applications/" + f.appID + "/configurationprofiles/" + f.profileID
		delRec := doRequestWithHeader(t, f.h, http.MethodDelete, path,
			"X-Amzn-Deletion-Protection-Check", "BYPASS", nil)

		require.Equal(t, http.StatusNoContent, delRec.Code)

		getRec := doRequest(t, f.h, http.MethodGet, path, nil)
		assert.Equal(t, http.StatusNotFound, getRec.Code, "profile should have been deleted")
	})
}

// TestHandler_DeleteEnvironment_DeletionProtectionCheck_AccountDefaultGraceExcusesFreshResource
// proves ACCOUNT_DEFAULT does not force the check against a resource created
// within the past hour (DeletionProtectionSettings doc, types/types.go:229-231:
// "DeletionProtectionCheck skips configuration profiles and environments that
// were created in the past hour") -- only APPLY forces that.
func TestHandler_DeleteEnvironment_DeletionProtectionCheck_AccountDefaultGraceExcusesFreshResource(t *testing.T) {
	t.Parallel()

	f := newDeletionProtectionFixture(t)
	f.recordRecentRead(t)

	path := "/applications/" + f.appID + "/environments/" + f.envID
	delRec := doRequestWithHeader(t, f.h, http.MethodDelete, path,
		"X-Amzn-Deletion-Protection-Check", "ACCOUNT_DEFAULT", nil)

	require.Equal(t, http.StatusNoContent, delRec.Code,
		"a freshly created environment is excluded from the check under ACCOUNT_DEFAULT")
}

package medialive_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	medialivesdk "github.com/aws/aws-sdk-go-v2/service/medialive"
	"github.com/aws/aws-sdk-go-v2/service/medialive/types"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/services/medialive"
)

func TestChannel_Create(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     any
		check    func(t *testing.T, body []byte)
		name     string
		wantCode int
	}{
		{
			name:     "create returns channel with ARN and IDLE state",
			body:     map[string]any{"name": "my-channel", "channelClass": "STANDARD"},
			wantCode: http.StatusCreated,
			check: func(t *testing.T, body []byte) {
				t.Helper()

				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				ch := resp["channel"].(map[string]any)
				assert.Contains(t, ch["arn"], "arn:aws:medialive:us-east-1:000000000000:channel:")
				assert.Equal(t, "IDLE", ch["state"])
				assert.Equal(t, "STANDARD", ch["channelClass"])
				assert.NotEmpty(t, ch["id"])
			},
		},
		{
			name:     "create missing Name returns 400",
			body:     map[string]any{},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			rec := doRequest(t, h, http.MethodPost, "/prod/channels", tc.body)
			assert.Equal(t, tc.wantCode, rec.Code)
			if tc.check != nil {
				tc.check(t, rec.Body.Bytes())
			}
		})
	}
}

func TestChannel_CRUD(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	channelID := createTestChannel(t, h)

	assert.Equal(t, 1, medialive.ChannelCount(h.Backend.(*medialive.InMemoryBackend)))

	// Describe
	rec := doRequest(t, h, http.MethodGet, "/prod/channels/"+channelID, nil)
	assert.Equal(t, http.StatusOK, rec.Code)
	var descResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
	assert.Equal(t, "test-channel", descResp["name"])
	assert.Equal(t, "IDLE", descResp["state"])

	// Update
	rec = doRequest(t, h, http.MethodPut, "/prod/channels/"+channelID, map[string]any{
		"name": "updated-channel",
	})
	assert.Equal(t, http.StatusOK, rec.Code)
	var updateResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &updateResp))
	ch := updateResp["channel"].(map[string]any)
	assert.Equal(t, "updated-channel", ch["name"])

	// List
	rec = doRequest(t, h, http.MethodGet, "/prod/channels", nil)
	assert.Equal(t, http.StatusOK, rec.Code)
	var listResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
	assert.Len(t, listResp["channels"], 1)

	// Delete
	rec = doRequest(t, h, http.MethodDelete, "/prod/channels/"+channelID, nil)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 0, medialive.ChannelCount(h.Backend.(*medialive.InMemoryBackend)))

	// Describe deleted returns 404
	rec = doRequest(t, h, http.MethodGet, "/prod/channels/"+channelID, nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestChannel_StartStop(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	channelID := createTestChannel(t, h)

	// Start
	rec := doRequest(t, h, http.MethodPost, "/prod/channels/"+channelID+"/start", nil)
	assert.Equal(t, http.StatusOK, rec.Code)
	var startResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &startResp))
	assert.Equal(t, "STARTING", startResp["state"])

	// Start again returns conflict
	rec = doRequest(t, h, http.MethodPost, "/prod/channels/"+channelID+"/start", nil)
	assert.Equal(t, http.StatusConflict, rec.Code)

	// Stop
	rec = doRequest(t, h, http.MethodPost, "/prod/channels/"+channelID+"/stop", nil)
	assert.Equal(t, http.StatusOK, rec.Code)
	var stopResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &stopResp))
	assert.Equal(t, "STOPPING", stopResp["state"])
}

// TestChannel_PipelinesRunningCount locks in the wire-accurate
// pipelinesRunningCount field (real DescribeChannelOutput reports the
// number of currently healthy pipelines: 0 while IDLE/STARTING/STOPPING, 2
// for a STANDARD channel once RUNNING).
func TestChannel_PipelinesRunningCount(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	channelID := createTestChannel(t, h)

	rec := doRequest(t, h, http.MethodGet, "/prod/channels/"+channelID, nil)
	var descResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
	assert.InDelta(t, 0, descResp["pipelinesRunningCount"], 0, "IDLE channel reports 0 running pipelines")

	rec = doRequest(t, h, http.MethodPost, "/prod/channels/"+channelID+"/start", nil)
	var startResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &startResp))
	assert.InDelta(t, 0, startResp["pipelinesRunningCount"], 0, "STARTING channel reports 0 running pipelines")

	// The backend advances internal state to RUNNING immediately (see
	// StartChannel's doc comment), so an immediate Describe should already
	// report the class's full pipeline count.
	rec = doRequest(t, h, http.MethodGet, "/prod/channels/"+channelID, nil)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
	assert.Equal(t, "RUNNING", descResp["state"])
	assert.InDelta(t, 2, descResp["pipelinesRunningCount"], 0, "RUNNING STANDARD channel reports 2 running pipelines")
}

// TestChannel_DeleteReportsDeleting locks in a fix for gopherstack-1um:
// DeleteChannel's response reported State DELETED, but api_op_DeleteChannel.
// go's doc comment ("Starts deletion of channel.") mirrors StartChannel's
// "Starts an existing channel" -- which this same file already reports as
// the intermediate STARTING state, not the eventual RUNNING one. DeleteChannel
// should carry the matching intermediate DELETING state.
func TestChannel_DeleteReportsDeleting(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	channelID := createTestChannel(t, h)

	rec := doRequest(t, h, http.MethodDelete, "/prod/channels/"+channelID, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	deleted := decodeBody(t, rec.Body.Bytes())
	assert.Equal(t, "DELETING", deleted["state"])
}

func TestChannel_DeleteRunning(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	channelID := createTestChannel(t, h)

	// Start channel
	doRequest(t, h, http.MethodPost, "/prod/channels/"+channelID+"/start", nil)

	// Delete running channel returns conflict
	rec := doRequest(t, h, http.MethodDelete, "/prod/channels/"+channelID, nil)
	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestChannel_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"describe unknown returns 404", http.MethodGet, "/prod/channels/notexist"},
		{"update unknown returns 404", http.MethodPut, "/prod/channels/notexist"},
		{"delete unknown returns 404", http.MethodDelete, "/prod/channels/notexist"},
		{"start unknown returns 404", http.MethodPost, "/prod/channels/notexist/start"},
		{"stop unknown returns 404", http.MethodPost, "/prod/channels/notexist/stop"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := doRequest(t, h, tc.method, tc.path, nil)
			assert.Equal(t, http.StatusNotFound, rec.Code)
		})
	}
}

func TestListChannels_Empty(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, http.MethodGet, "/prod/channels", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Empty(t, resp["channels"])
}

func TestChannelLifecycleExtras(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	channelID := createTestChannel(t, h)

	rec := doRequest(
		t,
		h,
		http.MethodPut,
		"/prod/channels/"+channelID+"/channelClass",
		map[string]any{
			"channelClass": "SINGLE_PIPELINE",
		},
	)
	require.Equal(t, http.StatusOK, rec.Code)
	ch := decodeBody(t, rec.Body.Bytes())["channel"].(map[string]any)
	assert.Equal(t, "SINGLE_PIPELINE", ch["channelClass"])

	rec = doRequest(
		t,
		h,
		http.MethodPost,
		"/prod/channels/"+channelID+"/restartChannelPipelines",
		nil,
	)
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, http.MethodGet, "/prod/channels/"+channelID+"/thumbnails", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotNil(t, decodeBody(t, rec.Body.Bytes())["thumbnailDetails"])

	rec = doRequest(t, h, http.MethodPut, "/prod/channels/missing/channelClass", map[string]any{
		"channelClass": "STANDARD",
	})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestAlertsAndVersions covers ListAlerts (per-channel) and ListVersions
// (account-wide channel engine versions), which share a handler section.
func TestAlertsAndVersions(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	channelID := createTestChannel(t, h)

	rec := doRequest(t, h, http.MethodGet, "/prod/channels/"+channelID+"/alerts", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotNil(t, decodeBody(t, rec.Body.Bytes())["alerts"])

	rec = doRequest(t, h, http.MethodGet, "/prod/channels/missing/alerts", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)

	rec = doRequest(t, h, http.MethodGet, "/prod/versions", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	versions := decodeBody(t, rec.Body.Bytes())["versions"].([]any)
	require.NotEmpty(t, versions)
	first := versions[0].(map[string]any)
	assert.NotEmpty(t, first["version"])
	_, hasExpiration := first["expirationDate"]
	assert.False(t, hasExpiration, "empty expirationDate must be omitted, not emitted as \"\"")
}

// newTestChannelClient stands up the real aws-sdk-go-v2 MediaLive client
// against an httptest server running this package's Handler, wired through
// the same pkgs/service registry/router used in production -- same pattern
// as services/codedeploy's handler_sdk_roundtrip_test.go. Round-tripping
// through the genuine SDK serializer/deserializer (rather than decoding the
// raw JSON body with ad-hoc structs, as most other tests in this package do)
// is the only credible proof that gopherstack-jb9i's 12 added Channel fields
// actually survive the wire in the shape a real client reads named
// sub-fields from -- backend-struct assertions alone repeatedly hid dropped
// fields in past parity sweeps.
func newTestChannelClient(t *testing.T, h *medialive.Handler) *medialivesdk.Client {
	t.Helper()

	e := echo.New()
	registry := service.NewRegistry()
	require.NoError(t, registry.Register(h))
	e.Use(service.NewServiceRouter(registry).RouteHandler())

	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)

	cfg, err := awscfg.LoadDefaultConfig(
		t.Context(),
		awscfg.WithRegion("us-east-1"),
		awscfg.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err)

	return medialivesdk.NewFromConfig(cfg, func(o *medialivesdk.Options) {
		o.BaseEndpoint = aws.String(srv.URL)
	})
}

// minimalValidEncoderSettings returns an EncoderSettings that passes the
// real SDK client's OWN request-side validation (AudioDescriptions/
// VideoDescriptions/OutputGroups/TimecodeConfig are required, and each
// OutputGroup requires an OutputGroupSettings + each Output an
// OutputSettings -- verified against validators.go's validateEncoderSettings/
// validateOutputGroup/validateOutput). ArchiveGroupSettings/
// ArchiveOutputSettings are the smallest concrete OutputGroupSettings/
// OutputSettings union variants that satisfy their own nested validation
// (Destination/ContainerSettings, neither of which has further required
// sub-fields). gopherstack does not model OutputGroupSettings/
// OutputSettings at all (see EncoderSettings' gopherstack doc comment), so
// this exists purely to get a real, validation-passing request out the
// door -- the round-trip assertions only check the fields gopherstack DOES
// model.
func minimalValidEncoderSettings() *types.EncoderSettings {
	return &types.EncoderSettings{
		AudioDescriptions: []types.AudioDescription{
			{
				Name:                aws.String("audio1"),
				AudioSelectorName:   aws.String("sel1"),
				AudioType:           types.AudioTypeCleanEffects,
				AudioTypeControl:    types.AudioDescriptionAudioTypeControlUseConfigured,
				LanguageCode:        aws.String("eng"),
				LanguageCodeControl: types.AudioDescriptionLanguageCodeControlUseConfigured,
				StreamName:          aws.String("English"),
			},
		},
		VideoDescriptions: []types.VideoDescription{
			{
				Name: aws.String("video1"), Height: aws.Int32(720), Width: aws.Int32(1280),
				RespondToAfd:    types.VideoDescriptionRespondToAfdPassthrough,
				ScalingBehavior: types.VideoDescriptionScalingBehaviorDefault,
				Sharpness:       aws.Int32(50),
			},
		},
		CaptionDescriptions: []types.CaptionDescription{
			{
				Name: aws.String("cap1"), CaptionSelectorName: aws.String("capsel1"),
				LanguageCode: aws.String("eng"), LanguageDescription: aws.String("English"),
				Accessibility: types.AccessibilityTypeDoesNotImplementAccessibilityFeatures,
			},
		},
		TimecodeConfig: &types.TimecodeConfig{
			Source: types.TimecodeConfigSourceSystemclock, SyncThreshold: aws.Int32(1),
		},
		AvailBlanking: &types.AvailBlanking{State: types.AvailBlankingStateEnabled},
		BlackoutSlate: &types.BlackoutSlate{
			State: types.BlackoutSlateStateEnabled, NetworkId: aws.String("10.5240/F98B-A522"),
		},
		FeatureActivations: &types.FeatureActivations{
			InputPrepareScheduleActions: types.FeatureActivationsInputPrepareScheduleActionsEnabled,
		},
		GlobalConfiguration: &types.GlobalConfiguration{
			InitialAudioGain: aws.Int32(-1), InputEndAction: types.GlobalConfigurationInputEndActionNone,
			OutputLockingMode: types.GlobalConfigurationOutputLockingModeEpochLocking,
			OutputLockingSettings: &types.OutputLockingSettings{
				EpochLockingSettings: &types.EpochLockingSettings{JamSyncTime: aws.String("00:00:00")},
			},
			InputLossBehavior: &types.InputLossBehavior{BlackFrameMsec: aws.Int32(1000)},
		},
		ThumbnailConfiguration: &types.ThumbnailConfiguration{State: types.ThumbnailStateAuto},
		OutputGroups: []types.OutputGroup{
			{
				Name: aws.String("group1"),
				OutputGroupSettings: &types.OutputGroupSettings{
					ArchiveGroupSettings: &types.ArchiveGroupSettings{
						Destination: &types.OutputLocationRef{DestinationRefId: aws.String("dest1")},
					},
				},
				Outputs: []types.Output{
					{
						OutputName: aws.String("output1"), VideoDescriptionName: aws.String("video1"),
						AudioDescriptionNames: []string{"audio1"}, CaptionDescriptionNames: []string{"cap1"},
						OutputSettings: &types.OutputSettings{
							ArchiveOutputSettings: &types.ArchiveOutputSettings{
								ContainerSettings: &types.ArchiveContainerSettings{},
							},
						},
					},
				},
			},
		},
	}
}

// TestChannel_ExtendedFieldsSDKRoundTrip proves gopherstack-jb9i's fix for
// medialive/gopherstack-jb9i: CreateChannelInput/UpdateChannelInput have 17
// real top-level members (verified against
// aws-sdk-go-v2/service/medialive@v1.101.4's api_op_CreateChannel.go), and
// before this fix gopherstack modeled only 5. Every case below drives a real
// SDK client through the actual router path (see newTestChannelClient) and
// asserts on the SDK's OWN decoded response type, not a hand-rolled JSON
// decode -- the only credible proof a field survives the wire in a shape a
// real client can read.
func TestChannel_ExtendedFieldsSDKRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		run  func(t *testing.T, client *medialivesdk.Client)
		name string
	}{
		{
			name: "flat scalar/list fields (cdiInputSpecification/inputSpecification/logLevel/" +
				"channelEngineVersion/channelSecurityGroups/vpc/maintenance)",
			run: func(t *testing.T, client *medialivesdk.Client) {
				t.Helper()

				out, err := client.CreateChannel(t.Context(), &medialivesdk.CreateChannelInput{
					Name: aws.String("rt-flat"), ChannelClass: types.ChannelClassStandard,
					LogLevel: types.LogLevelInfo,
					CdiInputSpecification: &types.CdiInputSpecification{
						Resolution: types.CdiInputResolutionHd,
					},
					InputSpecification: &types.InputSpecification{
						Codec: types.InputCodecAvc, MaximumBitrate: types.InputMaximumBitrateMax20Mbps,
						Resolution: types.InputResolutionHd,
					},
					ChannelEngineVersion:  &types.ChannelEngineVersionRequest{Version: aws.String("2_0")},
					ChannelSecurityGroups: []string{"sg-1", "sg-2"},
					Vpc: &types.VpcOutputSettings{
						SubnetIds: []string{"subnet-1", "subnet-2"}, SecurityGroupIds: []string{"sg-vpc"},
					},
					Maintenance: &types.MaintenanceCreateSettings{
						MaintenanceDay: types.MaintenanceDayMonday, MaintenanceStartTime: aws.String("02:00"),
					},
				})
				require.NoError(t, err)
				require.NotNil(t, out.Channel)

				desc, err := client.DescribeChannel(t.Context(), &medialivesdk.DescribeChannelInput{
					ChannelId: out.Channel.Id,
				})
				require.NoError(t, err)

				assert.Equal(t, types.LogLevelInfo, desc.LogLevel)
				require.NotNil(t, desc.CdiInputSpecification)
				assert.Equal(t, types.CdiInputResolutionHd, desc.CdiInputSpecification.Resolution)
				require.NotNil(t, desc.InputSpecification)
				assert.Equal(t, types.InputCodecAvc, desc.InputSpecification.Codec)
				assert.Equal(t, types.InputMaximumBitrateMax20Mbps, desc.InputSpecification.MaximumBitrate)
				assert.Equal(t, types.InputResolutionHd, desc.InputSpecification.Resolution)
				require.NotNil(t, desc.ChannelEngineVersion)
				assert.Equal(t, "2_0", aws.ToString(desc.ChannelEngineVersion.Version))
				assert.ElementsMatch(t, []string{"sg-1", "sg-2"}, desc.ChannelSecurityGroups)
				require.NotNil(t, desc.Vpc)
				assert.ElementsMatch(t, []string{"subnet-1", "subnet-2"}, desc.Vpc.SubnetIds)
				assert.ElementsMatch(t, []string{"sg-vpc"}, desc.Vpc.SecurityGroupIds)
				require.NotNil(t, desc.Maintenance)
				assert.Equal(t, types.MaintenanceDayMonday, desc.Maintenance.MaintenanceDay)
				assert.Equal(t, "02:00", aws.ToString(desc.Maintenance.MaintenanceStartTime))
			},
		},
		{
			name: "inferenceSettings round-trips audioFeedInputs and feedArn",
			run: func(t *testing.T, client *medialivesdk.Client) {
				t.Helper()

				out, err := client.CreateChannel(t.Context(), &medialivesdk.CreateChannelInput{
					Name: aws.String("rt-inference"), ChannelClass: types.ChannelClassStandard,
					InferenceSettings: &types.InferenceSettings{
						FeedArn: aws.String("arn:aws:elemental-inference:us-east-1:000000000000:feed:f1"),
						AudioFeedInputs: []types.AudioFeedInput{
							{AudioSelectorName: aws.String("sel1"), FeedInput: aws.String("feed1")},
						},
					},
				})
				require.NoError(t, err)

				desc, err := client.DescribeChannel(t.Context(), &medialivesdk.DescribeChannelInput{
					ChannelId: out.Channel.Id,
				})
				require.NoError(t, err)
				require.NotNil(t, desc.InferenceSettings)
				assert.Equal(
					t, "arn:aws:elemental-inference:us-east-1:000000000000:feed:f1",
					aws.ToString(desc.InferenceSettings.FeedArn),
				)
				require.Len(t, desc.InferenceSettings.AudioFeedInputs, 1)
				assert.Equal(t, "sel1", aws.ToString(desc.InferenceSettings.AudioFeedInputs[0].AudioSelectorName))
				assert.Equal(t, "feed1", aws.ToString(desc.InferenceSettings.AudioFeedInputs[0].FeedInput))
			},
		},
		{
			name: "destinations round-trips every per-technology settings variant",
			run: func(t *testing.T, client *medialivesdk.Client) {
				t.Helper()

				out, err := client.CreateChannel(t.Context(), &medialivesdk.CreateChannelInput{
					Name: aws.String("rt-destinations"), ChannelClass: types.ChannelClassStandard,
					Destinations: []types.OutputDestination{
						{
							Id:                    aws.String("dest1"),
							LogicalInterfaceNames: []string{"if-a"},
							Settings: []types.OutputDestinationSettings{
								{Url: aws.String("rtmp://example.com/live"), StreamName: aws.String("stream1")},
							},
							MediaPackageSettings: []types.MediaPackageOutputDestinationSettings{
								{ChannelId: aws.String("mp-channel-1")},
							},
							MultiplexSettings: &types.MultiplexProgramChannelDestinationSettings{
								MultiplexId: aws.String("mux-1"), ProgramName: aws.String("prog-1"),
							},
							MediaConnectRouterSettings: []types.MediaConnectRouterOutputDestinationSettings{
								{EncryptionType: types.MediaConnectRouterOutputEncryptionTypeAutomatic},
							},
							SrtSettings: []types.SrtOutputDestinationSettings{
								{ConnectionMode: types.ConnectionModeCaller, StreamId: aws.String("srt-1")},
							},
						},
					},
				})
				require.NoError(t, err)

				desc, err := client.DescribeChannel(t.Context(), &medialivesdk.DescribeChannelInput{
					ChannelId: out.Channel.Id,
				})
				require.NoError(t, err)
				require.Len(t, desc.Destinations, 1)

				d := desc.Destinations[0]
				assert.Equal(t, "dest1", aws.ToString(d.Id))
				assert.ElementsMatch(t, []string{"if-a"}, d.LogicalInterfaceNames)
				require.Len(t, d.Settings, 1)
				assert.Equal(t, "rtmp://example.com/live", aws.ToString(d.Settings[0].Url))
				assert.Equal(t, "stream1", aws.ToString(d.Settings[0].StreamName))
				require.Len(t, d.MediaPackageSettings, 1)
				assert.Equal(t, "mp-channel-1", aws.ToString(d.MediaPackageSettings[0].ChannelId))
				require.NotNil(t, d.MultiplexSettings)
				assert.Equal(t, "mux-1", aws.ToString(d.MultiplexSettings.MultiplexId))
				assert.Equal(t, "prog-1", aws.ToString(d.MultiplexSettings.ProgramName))
				require.Len(t, d.MediaConnectRouterSettings, 1)
				assert.Equal(
					t, types.MediaConnectRouterOutputEncryptionTypeAutomatic,
					d.MediaConnectRouterSettings[0].EncryptionType,
				)
				require.Len(t, d.SrtSettings, 1)
				assert.Equal(t, types.ConnectionModeCaller, d.SrtSettings[0].ConnectionMode)
				assert.Equal(t, "srt-1", aws.ToString(d.SrtSettings[0].StreamId))
			},
		},
		{
			name: "inputAttachments round-trips automaticInputFailoverSettings and all 3 failover condition variants",
			run: func(t *testing.T, client *medialivesdk.Client) {
				t.Helper()

				inpOut, err := client.CreateInput(t.Context(), &medialivesdk.CreateInputInput{
					Name: aws.String("rt-input"), Type: types.InputTypeUdpPush,
				})
				require.NoError(t, err)

				out, err := client.CreateChannel(t.Context(), &medialivesdk.CreateChannelInput{
					Name: aws.String("rt-attachments"), ChannelClass: types.ChannelClassStandard,
					InputAttachments: []types.InputAttachment{
						{
							InputAttachmentName: aws.String("primary"), InputId: inpOut.Input.Id,
							LogicalInterfaceNames: []string{"if-a"},
							AutomaticInputFailoverSettings: &types.AutomaticInputFailoverSettings{
								SecondaryInputId:   inpOut.Input.Id,
								ErrorClearTimeMsec: aws.Int32(5000),
								InputPreference:    types.InputPreferencePrimaryInputPreferred,
								FailoverConditions: []types.FailoverCondition{
									{FailoverConditionSettings: &types.FailoverConditionSettings{
										AudioSilenceSettings: &types.AudioSilenceFailoverSettings{
											AudioSelectorName:         aws.String("sel1"),
											AudioSilenceThresholdMsec: aws.Int32(3000),
										},
									}},
									{FailoverConditionSettings: &types.FailoverConditionSettings{
										InputLossSettings: &types.InputLossFailoverSettings{
											InputLossThresholdMsec: aws.Int32(4000),
										},
									}},
									{FailoverConditionSettings: &types.FailoverConditionSettings{
										VideoBlackSettings: &types.VideoBlackFailoverSettings{
											BlackDetectThreshold:    aws.Float64(0.1),
											VideoBlackThresholdMsec: aws.Int32(2000),
										},
									}},
								},
							},
						},
					},
				})
				require.NoError(t, err)

				desc, err := client.DescribeChannel(t.Context(), &medialivesdk.DescribeChannelInput{
					ChannelId: out.Channel.Id,
				})
				require.NoError(t, err)
				require.Len(t, desc.InputAttachments, 1)

				a := desc.InputAttachments[0]
				assert.Equal(t, "primary", aws.ToString(a.InputAttachmentName))
				assert.Equal(t, aws.ToString(inpOut.Input.Id), aws.ToString(a.InputId))
				assert.ElementsMatch(t, []string{"if-a"}, a.LogicalInterfaceNames)
				require.NotNil(t, a.AutomaticInputFailoverSettings)
				fo := a.AutomaticInputFailoverSettings
				assert.Equal(t, aws.ToString(inpOut.Input.Id), aws.ToString(fo.SecondaryInputId))
				assert.Equal(t, int32(5000), aws.ToInt32(fo.ErrorClearTimeMsec))
				assert.Equal(t, types.InputPreferencePrimaryInputPreferred, fo.InputPreference)
				require.Len(t, fo.FailoverConditions, 3)

				audioSilence := fo.FailoverConditions[0].FailoverConditionSettings.AudioSilenceSettings
				require.NotNil(t, audioSilence)
				assert.Equal(t, "sel1", aws.ToString(audioSilence.AudioSelectorName))

				inputLoss := fo.FailoverConditions[1].FailoverConditionSettings.InputLossSettings
				require.NotNil(t, inputLoss)
				assert.Equal(t, int32(4000), aws.ToInt32(inputLoss.InputLossThresholdMsec))

				videoBlack := fo.FailoverConditions[2].FailoverConditionSettings.VideoBlackSettings
				require.NotNil(t, videoBlack)
				assert.InDelta(t, 0.1, aws.ToFloat64(videoBlack.BlackDetectThreshold), 0)
			},
		},
		{
			name: "encoderSettings round-trips the modeled subset (audio/video/caption descriptions, " +
				"timecodeConfig, availBlanking, blackoutSlate, featureActivations, globalConfiguration, " +
				"thumbnailConfiguration, outputGroups' name/outputs)",
			run: func(t *testing.T, client *medialivesdk.Client) {
				t.Helper()

				out, err := client.CreateChannel(t.Context(), &medialivesdk.CreateChannelInput{
					Name: aws.String("rt-encoder"), ChannelClass: types.ChannelClassStandard,
					EncoderSettings: minimalValidEncoderSettings(),
				})
				require.NoError(t, err)

				desc, err := client.DescribeChannel(t.Context(), &medialivesdk.DescribeChannelInput{
					ChannelId: out.Channel.Id,
				})
				require.NoError(t, err)
				require.NotNil(t, desc.EncoderSettings)
				es := desc.EncoderSettings

				require.Len(t, es.AudioDescriptions, 1)
				assert.Equal(t, "audio1", aws.ToString(es.AudioDescriptions[0].Name))
				assert.Equal(t, "sel1", aws.ToString(es.AudioDescriptions[0].AudioSelectorName))
				assert.Equal(t, types.AudioTypeCleanEffects, es.AudioDescriptions[0].AudioType)

				require.Len(t, es.VideoDescriptions, 1)
				assert.Equal(t, "video1", aws.ToString(es.VideoDescriptions[0].Name))
				assert.Equal(t, int32(720), aws.ToInt32(es.VideoDescriptions[0].Height))
				assert.Equal(t, int32(1280), aws.ToInt32(es.VideoDescriptions[0].Width))

				require.Len(t, es.CaptionDescriptions, 1)
				assert.Equal(t, "cap1", aws.ToString(es.CaptionDescriptions[0].Name))
				assert.Equal(t, "capsel1", aws.ToString(es.CaptionDescriptions[0].CaptionSelectorName))

				require.NotNil(t, es.TimecodeConfig)
				assert.Equal(t, types.TimecodeConfigSourceSystemclock, es.TimecodeConfig.Source)
				assert.Equal(t, int32(1), aws.ToInt32(es.TimecodeConfig.SyncThreshold))

				require.NotNil(t, es.AvailBlanking)
				assert.Equal(t, types.AvailBlankingStateEnabled, es.AvailBlanking.State)

				require.NotNil(t, es.BlackoutSlate)
				assert.Equal(t, types.BlackoutSlateStateEnabled, es.BlackoutSlate.State)
				assert.Equal(t, "10.5240/F98B-A522", aws.ToString(es.BlackoutSlate.NetworkId))

				require.NotNil(t, es.FeatureActivations)
				assert.Equal(
					t, types.FeatureActivationsInputPrepareScheduleActionsEnabled,
					es.FeatureActivations.InputPrepareScheduleActions,
				)

				require.NotNil(t, es.GlobalConfiguration)
				assert.Equal(t, int32(-1), aws.ToInt32(es.GlobalConfiguration.InitialAudioGain))
				assert.Equal(t, types.GlobalConfigurationInputEndActionNone, es.GlobalConfiguration.InputEndAction)
				assert.Equal(
					t, types.GlobalConfigurationOutputLockingModeEpochLocking, es.GlobalConfiguration.OutputLockingMode,
				)
				require.NotNil(t, es.GlobalConfiguration.OutputLockingSettings)
				epochLocking := es.GlobalConfiguration.OutputLockingSettings.EpochLockingSettings
				require.NotNil(t, epochLocking)
				assert.Equal(t, "00:00:00", aws.ToString(epochLocking.JamSyncTime))
				require.NotNil(t, es.GlobalConfiguration.InputLossBehavior)
				assert.Equal(t, int32(1000), aws.ToInt32(es.GlobalConfiguration.InputLossBehavior.BlackFrameMsec))

				require.NotNil(t, es.ThumbnailConfiguration)
				assert.Equal(t, types.ThumbnailStateAuto, es.ThumbnailConfiguration.State)

				require.Len(t, es.OutputGroups, 1)
				assert.Equal(t, "group1", aws.ToString(es.OutputGroups[0].Name))
				require.Len(t, es.OutputGroups[0].Outputs, 1)
				assert.Equal(t, "output1", aws.ToString(es.OutputGroups[0].Outputs[0].OutputName))
				assert.Equal(t, "video1", aws.ToString(es.OutputGroups[0].Outputs[0].VideoDescriptionName))
				assert.ElementsMatch(t, []string{"audio1"}, es.OutputGroups[0].Outputs[0].AudioDescriptionNames)
				assert.ElementsMatch(t, []string{"cap1"}, es.OutputGroups[0].Outputs[0].CaptionDescriptionNames)
			},
		},
		{
			name: "encoderSettings.availConfiguration round-trips the esam variant and scte35SegmentationScope",
			run: func(t *testing.T, client *medialivesdk.Client) {
				t.Helper()

				es := minimalValidEncoderSettings()
				es.AvailConfiguration = &types.AvailConfiguration{
					Scte35SegmentationScope: types.Scte35SegmentationScopeScte35EnabledOutputGroups,
					AvailSettings: &types.AvailSettings{
						Esam: &types.Esam{
							AcquisitionPointId: aws.String("acq-1"),
							PoisEndpoint:       aws.String("https://pois.example.com"),
							AdAvailOffset:      aws.Int32(500),
							ZoneIdentity:       aws.String("zone-1"),
						},
					},
				}

				out, err := client.CreateChannel(t.Context(), &medialivesdk.CreateChannelInput{
					Name: aws.String("rt-avail-esam"), ChannelClass: types.ChannelClassStandard,
					EncoderSettings: es,
				})
				require.NoError(t, err)

				desc, err := client.DescribeChannel(t.Context(), &medialivesdk.DescribeChannelInput{
					ChannelId: out.Channel.Id,
				})
				require.NoError(t, err)
				require.NotNil(t, desc.EncoderSettings.AvailConfiguration)
				avail := desc.EncoderSettings.AvailConfiguration
				assert.Equal(t, types.Scte35SegmentationScopeScte35EnabledOutputGroups, avail.Scte35SegmentationScope)
				require.NotNil(t, avail.AvailSettings)
				require.NotNil(t, avail.AvailSettings.Esam)
				assert.Equal(t, "acq-1", aws.ToString(avail.AvailSettings.Esam.AcquisitionPointId))
				assert.Equal(t, "https://pois.example.com", aws.ToString(avail.AvailSettings.Esam.PoisEndpoint))
				assert.Equal(t, int32(500), aws.ToInt32(avail.AvailSettings.Esam.AdAvailOffset))
				assert.Equal(t, "zone-1", aws.ToString(avail.AvailSettings.Esam.ZoneIdentity))
			},
		},
		{
			name: "encoderSettings.availConfiguration round-trips the scte35SpliceInsert and " +
				"scte35TimeSignalApos variants",
			run: func(t *testing.T, client *medialivesdk.Client) {
				t.Helper()

				esSplice := minimalValidEncoderSettings()
				esSplice.AvailConfiguration = &types.AvailConfiguration{
					AvailSettings: &types.AvailSettings{
						Scte35SpliceInsert: &types.Scte35SpliceInsert{
							AdAvailOffset:          aws.Int32(-100),
							NoRegionalBlackoutFlag: types.Scte35SpliceInsertNoRegionalBlackoutBehaviorIgnore,
							WebDeliveryAllowedFlag: types.Scte35SpliceInsertWebDeliveryAllowedBehaviorFollow,
						},
					},
				}

				spliceOut, err := client.CreateChannel(t.Context(), &medialivesdk.CreateChannelInput{
					Name: aws.String("rt-avail-splice"), ChannelClass: types.ChannelClassStandard,
					EncoderSettings: esSplice,
				})
				require.NoError(t, err)

				spliceDesc, err := client.DescribeChannel(t.Context(), &medialivesdk.DescribeChannelInput{
					ChannelId: spliceOut.Channel.Id,
				})
				require.NoError(t, err)
				require.NotNil(t, spliceDesc.EncoderSettings.AvailConfiguration.AvailSettings.Scte35SpliceInsert)
				splice := spliceDesc.EncoderSettings.AvailConfiguration.AvailSettings.Scte35SpliceInsert
				assert.Equal(t, int32(-100), aws.ToInt32(splice.AdAvailOffset))
				assert.Equal(t, types.Scte35SpliceInsertNoRegionalBlackoutBehaviorIgnore, splice.NoRegionalBlackoutFlag)
				assert.Equal(t, types.Scte35SpliceInsertWebDeliveryAllowedBehaviorFollow, splice.WebDeliveryAllowedFlag)

				esApos := minimalValidEncoderSettings()
				esApos.AvailConfiguration = &types.AvailConfiguration{
					AvailSettings: &types.AvailSettings{
						Scte35TimeSignalApos: &types.Scte35TimeSignalApos{
							AdAvailOffset:          aws.Int32(200),
							NoRegionalBlackoutFlag: types.Scte35AposNoRegionalBlackoutBehaviorFollow,
						},
					},
				}

				aposOut, err := client.CreateChannel(t.Context(), &medialivesdk.CreateChannelInput{
					Name: aws.String("rt-avail-apos"), ChannelClass: types.ChannelClassStandard,
					EncoderSettings: esApos,
				})
				require.NoError(t, err)

				aposDesc, err := client.DescribeChannel(t.Context(), &medialivesdk.DescribeChannelInput{
					ChannelId: aposOut.Channel.Id,
				})
				require.NoError(t, err)
				require.NotNil(t, aposDesc.EncoderSettings.AvailConfiguration.AvailSettings.Scte35TimeSignalApos)
				apos := aposDesc.EncoderSettings.AvailConfiguration.AvailSettings.Scte35TimeSignalApos
				assert.Equal(t, int32(200), aws.ToInt32(apos.AdAvailOffset))
				assert.Equal(t, types.Scte35AposNoRegionalBlackoutBehaviorFollow, apos.NoRegionalBlackoutFlag)
			},
		},
		{
			name: "encoderSettings.colorCorrectionSettings round-trips globalColorCorrections",
			run: func(t *testing.T, client *medialivesdk.Client) {
				t.Helper()

				es := minimalValidEncoderSettings()
				es.ColorCorrectionSettings = &types.ColorCorrectionSettings{
					GlobalColorCorrections: []types.ColorCorrection{
						{
							InputColorSpace: types.ColorSpaceRec709, OutputColorSpace: types.ColorSpaceHdr10,
							Uri: aws.String("s3://my-bucket/lut1.cube"),
						},
					},
				}

				out, err := client.CreateChannel(t.Context(), &medialivesdk.CreateChannelInput{
					Name: aws.String("rt-color-correction"), ChannelClass: types.ChannelClassStandard,
					EncoderSettings: es,
				})
				require.NoError(t, err)

				desc, err := client.DescribeChannel(t.Context(), &medialivesdk.DescribeChannelInput{
					ChannelId: out.Channel.Id,
				})
				require.NoError(t, err)
				require.NotNil(t, desc.EncoderSettings.ColorCorrectionSettings)
				require.Len(t, desc.EncoderSettings.ColorCorrectionSettings.GlobalColorCorrections, 1)
				cc := desc.EncoderSettings.ColorCorrectionSettings.GlobalColorCorrections[0]
				assert.Equal(t, types.ColorSpaceRec709, cc.InputColorSpace)
				assert.Equal(t, types.ColorSpaceHdr10, cc.OutputColorSpace)
				assert.Equal(t, "s3://my-bucket/lut1.cube", aws.ToString(cc.Uri))
			},
		},
		{
			name: "encoderSettings.motionGraphicsConfiguration and nielsenConfiguration round-trip",
			run: func(t *testing.T, client *medialivesdk.Client) {
				t.Helper()

				es := minimalValidEncoderSettings()
				es.MotionGraphicsConfiguration = &types.MotionGraphicsConfiguration{
					MotionGraphicsInsertion: types.MotionGraphicsInsertionEnabled,
					MotionGraphicsSettings: &types.MotionGraphicsSettings{
						HtmlMotionGraphicsSettings: &types.HtmlMotionGraphicsSettings{},
					},
				}
				es.NielsenConfiguration = &types.NielsenConfiguration{
					DistributorId:          aws.String("dist-1"),
					NielsenPcmToId3Tagging: types.NielsenPcmToId3TaggingStateEnabled,
				}

				out, err := client.CreateChannel(t.Context(), &medialivesdk.CreateChannelInput{
					Name: aws.String("rt-motion-nielsen"), ChannelClass: types.ChannelClassStandard,
					EncoderSettings: es,
				})
				require.NoError(t, err)

				desc, err := client.DescribeChannel(t.Context(), &medialivesdk.DescribeChannelInput{
					ChannelId: out.Channel.Id,
				})
				require.NoError(t, err)

				require.NotNil(t, desc.EncoderSettings.MotionGraphicsConfiguration)
				mg := desc.EncoderSettings.MotionGraphicsConfiguration
				assert.Equal(t, types.MotionGraphicsInsertionEnabled, mg.MotionGraphicsInsertion)
				require.NotNil(t, mg.MotionGraphicsSettings)
				assert.NotNil(t, mg.MotionGraphicsSettings.HtmlMotionGraphicsSettings)

				require.NotNil(t, desc.EncoderSettings.NielsenConfiguration)
				assert.Equal(t, "dist-1", aws.ToString(desc.EncoderSettings.NielsenConfiguration.DistributorId))
				assert.Equal(
					t, types.NielsenPcmToId3TaggingStateEnabled,
					desc.EncoderSettings.NielsenConfiguration.NielsenPcmToId3Tagging,
				)
			},
		},
		{
			name: "encoderSettings.audioDescriptions round-trips codecSettings' aacSettings variant plus " +
				"audioNormalizationSettings/remixSettings/audioWatermarkingSettings/audioDashRoles/" +
				"dvbDashAccessibility",
			run: func(t *testing.T, client *medialivesdk.Client) {
				t.Helper()

				es := minimalValidEncoderSettings()
				es.AudioDescriptions[0].AudioDashRoles = []types.DashRoleAudio{types.DashRoleAudioMain}
				es.AudioDescriptions[0].DvbDashAccessibility = types.DvbDashAccessibilityDvbdash6MainProgram
				es.AudioDescriptions[0].CodecSettings = &types.AudioCodecSettings{
					AacSettings: &types.AacSettings{
						Bitrate: aws.Float64(128000), CodingMode: types.AacCodingModeCodingMode20,
						InputType: types.AacInputTypeNormal, Profile: types.AacProfileLc,
						RateControlMode: types.AacRateControlModeVbr, RawFormat: types.AacRawFormatNone,
						SampleRate: aws.Float64(48000), Spec: types.AacSpecMpeg4,
						VbrQuality: types.AacVbrQualityHigh,
					},
				}
				es.AudioDescriptions[0].AudioNormalizationSettings = &types.AudioNormalizationSettings{
					Algorithm:  types.AudioNormalizationAlgorithmItu17701,
					TargetLkfs: aws.Float64(-24),
				}
				es.AudioDescriptions[0].RemixSettings = &types.RemixSettings{
					ChannelsIn: aws.Int32(2), ChannelsOut: aws.Int32(1),
					ChannelMappings: []types.AudioChannelMapping{
						{
							OutputChannel: aws.Int32(0),
							InputChannelLevels: []types.InputChannelLevel{
								{InputChannel: aws.Int32(0), Gain: aws.Int32(0)},
								{InputChannel: aws.Int32(1), Gain: aws.Int32(-60)},
							},
						},
					},
				}
				es.AudioDescriptions[0].AudioWatermarkingSettings = &types.AudioWatermarkSettings{
					NielsenWatermarksSettings: &types.NielsenWatermarksSettings{
						NielsenDistributionType: types.NielsenWatermarksDistributionTypesProgramContent,
						NielsenCbetSettings: &types.NielsenCBET{
							CbetCheckDigitString: aws.String("12345"),
							CbetStepaside:        types.NielsenWatermarksCbetStepasideDisabled,
							Csid:                 aws.String("csid-1"),
						},
					},
				}

				out, err := client.CreateChannel(t.Context(), &medialivesdk.CreateChannelInput{
					Name: aws.String("rt-audio-codec"), ChannelClass: types.ChannelClassStandard,
					EncoderSettings: es,
				})
				require.NoError(t, err)

				desc, err := client.DescribeChannel(t.Context(), &medialivesdk.DescribeChannelInput{
					ChannelId: out.Channel.Id,
				})
				require.NoError(t, err)
				require.Len(t, desc.EncoderSettings.AudioDescriptions, 1)
				ad := desc.EncoderSettings.AudioDescriptions[0]

				assert.ElementsMatch(t, []types.DashRoleAudio{types.DashRoleAudioMain}, ad.AudioDashRoles)
				assert.Equal(t, types.DvbDashAccessibilityDvbdash6MainProgram, ad.DvbDashAccessibility)

				require.NotNil(t, ad.CodecSettings)
				require.NotNil(t, ad.CodecSettings.AacSettings)
				aac := ad.CodecSettings.AacSettings
				assert.InDelta(t, 128000, aws.ToFloat64(aac.Bitrate), 0)
				assert.Equal(t, types.AacCodingModeCodingMode20, aac.CodingMode)
				assert.Equal(t, types.AacProfileLc, aac.Profile)
				assert.Equal(t, types.AacVbrQualityHigh, aac.VbrQuality)

				require.NotNil(t, ad.AudioNormalizationSettings)
				assert.Equal(t, types.AudioNormalizationAlgorithmItu17701, ad.AudioNormalizationSettings.Algorithm)
				assert.InDelta(t, -24, aws.ToFloat64(ad.AudioNormalizationSettings.TargetLkfs), 0)

				require.NotNil(t, ad.RemixSettings)
				assert.Equal(t, int32(2), aws.ToInt32(ad.RemixSettings.ChannelsIn))
				require.Len(t, ad.RemixSettings.ChannelMappings, 1)
				require.Len(t, ad.RemixSettings.ChannelMappings[0].InputChannelLevels, 2)
				assert.Equal(t, int32(-60), aws.ToInt32(ad.RemixSettings.ChannelMappings[0].InputChannelLevels[1].Gain))

				require.NotNil(t, ad.AudioWatermarkingSettings)
				require.NotNil(t, ad.AudioWatermarkingSettings.NielsenWatermarksSettings)
				nw := ad.AudioWatermarkingSettings.NielsenWatermarksSettings
				assert.Equal(t, types.NielsenWatermarksDistributionTypesProgramContent, nw.NielsenDistributionType)
				require.NotNil(t, nw.NielsenCbetSettings)
				assert.Equal(t, "12345", aws.ToString(nw.NielsenCbetSettings.CbetCheckDigitString))
				assert.Equal(t, "csid-1", aws.ToString(nw.NielsenCbetSettings.Csid))
			},
		},
		{
			name: "encoderSettings.audioDescriptions round-trips codecSettings' passThroughSettings empty-marker variant",
			run: func(t *testing.T, client *medialivesdk.Client) {
				t.Helper()

				es := minimalValidEncoderSettings()
				es.AudioDescriptions[0].CodecSettings = &types.AudioCodecSettings{
					PassThroughSettings: &types.PassThroughSettings{},
				}

				out, err := client.CreateChannel(t.Context(), &medialivesdk.CreateChannelInput{
					Name: aws.String("rt-audio-passthrough"), ChannelClass: types.ChannelClassStandard,
					EncoderSettings: es,
				})
				require.NoError(t, err)

				desc, err := client.DescribeChannel(t.Context(), &medialivesdk.DescribeChannelInput{
					ChannelId: out.Channel.Id,
				})
				require.NoError(t, err)
				require.Len(t, desc.EncoderSettings.AudioDescriptions, 1)
				require.NotNil(t, desc.EncoderSettings.AudioDescriptions[0].CodecSettings)
				assert.NotNil(t, desc.EncoderSettings.AudioDescriptions[0].CodecSettings.PassThroughSettings)
			},
		},
		{
			name: "linkedChannelSettings: primary's followingChannelArns is derived from another " +
				"channel's follower settings, not stored on the primary itself",
			run: func(t *testing.T, client *medialivesdk.Client) {
				t.Helper()

				primaryOut, err := client.CreateChannel(t.Context(), &medialivesdk.CreateChannelInput{
					Name: aws.String("rt-primary"), ChannelClass: types.ChannelClassStandard,
					LinkedChannelSettings: &types.LinkedChannelSettings{
						PrimaryChannelSettings: &types.PrimaryChannelSettings{
							LinkedChannelType: types.LinkedChannelTypePrimaryChannel,
						},
					},
				})
				require.NoError(t, err)
				require.NotNil(t, primaryOut.Channel.Arn)

				followerOut, err := client.CreateChannel(t.Context(), &medialivesdk.CreateChannelInput{
					Name: aws.String("rt-follower"), ChannelClass: types.ChannelClassStandard,
					LinkedChannelSettings: &types.LinkedChannelSettings{
						FollowerChannelSettings: &types.FollowerChannelSettings{
							LinkedChannelType: types.LinkedChannelTypeFollowingChannel,
							PrimaryChannelArn: primaryOut.Channel.Arn,
						},
					},
				})
				require.NoError(t, err)

				followerDesc, err := client.DescribeChannel(t.Context(), &medialivesdk.DescribeChannelInput{
					ChannelId: followerOut.Channel.Id,
				})
				require.NoError(t, err)
				require.NotNil(t, followerDesc.LinkedChannelSettings)
				require.NotNil(t, followerDesc.LinkedChannelSettings.FollowerChannelSettings)
				assert.Equal(
					t, aws.ToString(primaryOut.Channel.Arn),
					aws.ToString(followerDesc.LinkedChannelSettings.FollowerChannelSettings.PrimaryChannelArn),
				)

				primaryDesc, err := client.DescribeChannel(t.Context(), &medialivesdk.DescribeChannelInput{
					ChannelId: primaryOut.Channel.Id,
				})
				require.NoError(t, err)
				require.NotNil(t, primaryDesc.LinkedChannelSettings)
				require.NotNil(t, primaryDesc.LinkedChannelSettings.PrimaryChannelSettings)
				assert.ElementsMatch(
					t, []string{aws.ToString(followerOut.Channel.Arn)},
					primaryDesc.LinkedChannelSettings.PrimaryChannelSettings.FollowingChannelArns,
				)
			},
		},
		{
			name: "inputAttachments.inputSettings round-trips audioSelectors' 4 selector-settings " +
				"variants plus premixer/remix/normalization nesting",
			run: func(t *testing.T, client *medialivesdk.Client) {
				t.Helper()

				inpOut, err := client.CreateInput(t.Context(), &medialivesdk.CreateInputInput{
					Name: aws.String("rt-input-audio"), Type: types.InputTypeUdpPush,
				})
				require.NoError(t, err)

				out, err := client.CreateChannel(t.Context(), &medialivesdk.CreateChannelInput{
					Name: aws.String("rt-input-settings-audio"), ChannelClass: types.ChannelClassStandard,
					InputAttachments: []types.InputAttachment{
						{
							InputAttachmentName: aws.String("primary"), InputId: inpOut.Input.Id,
							InputSettings: &types.InputSettings{
								AudioSelectors: []types.AudioSelector{
									{
										Name: aws.String("aud-lang"),
										SelectorSettings: &types.AudioSelectorSettings{
											AudioLanguageSelection: &types.AudioLanguageSelection{
												LanguageCode:            aws.String("eng"),
												LanguageSelectionPolicy: types.AudioLanguageSelectionPolicyStrict,
											},
										},
									},
									{
										Name: aws.String("aud-hls"),
										SelectorSettings: &types.AudioSelectorSettings{
											AudioHlsRenditionSelection: &types.AudioHlsRenditionSelection{
												GroupId: aws.String("grp1"), Name: aws.String("rend1"),
											},
										},
									},
									{
										Name: aws.String("aud-pid"),
										SelectorSettings: &types.AudioSelectorSettings{
											AudioPidSelection: &types.AudioPidSelection{
												Pid: aws.Int32(101),
												Pids: []types.AudioPid{
													{
														Pid: aws.Int32(102),
														DolbyEDecode: &types.AudioDolbyEDecode{
															ProgramSelection: types.DolbyEProgramSelectionProgram1,
														},
														PremixSettings: &types.AudioPreMixerSettings{
															Channels: aws.Int32(2),
															GainDb:   aws.Float64(3.5),
															AudioNormalizationSettings: &types.AudioNormalizationSettings{
																Algorithm:            types.AudioNormalizationAlgorithmItu17701,
																AlgorithmControl:     types.AudioNormalizationAlgorithmControlCorrectAudio,
																PeakCalculation:      types.AudioNormalizationPeakCalculationTruePeak,
																PeakLimiterThreshold: aws.Float64(-2),
																TargetLkfs:           aws.Float64(-23),
															},
															RemixSettings: &types.RemixSettings{
																ChannelsIn: aws.Int32(2), ChannelsOut: aws.Int32(2),
																ChannelMappings: []types.AudioChannelMapping{
																	{
																		OutputChannel: aws.Int32(0),
																		InputChannelLevels: []types.InputChannelLevel{
																			{
																				InputChannel: aws.Int32(0),
																				Gain:         aws.Int32(-3),
																			},
																		},
																	},
																},
															},
														},
													},
												},
											},
										},
									},
									{
										Name: aws.String("aud-track"),
										SelectorSettings: &types.AudioSelectorSettings{
											AudioTrackSelection: &types.AudioTrackSelection{
												DolbyEDecode: &types.AudioDolbyEDecode{
													ProgramSelection: types.DolbyEProgramSelectionAllChannels,
												},
												Tracks: []types.AudioTrack{{Track: aws.Int32(1)}},
											},
										},
									},
								},
							},
						},
					},
				})
				require.NoError(t, err)

				desc, err := client.DescribeChannel(t.Context(), &medialivesdk.DescribeChannelInput{
					ChannelId: out.Channel.Id,
				})
				require.NoError(t, err)
				require.Len(t, desc.InputAttachments, 1)
				require.NotNil(t, desc.InputAttachments[0].InputSettings)
				sel := desc.InputAttachments[0].InputSettings.AudioSelectors
				require.Len(t, sel, 4)

				require.NotNil(t, sel[0].SelectorSettings.AudioLanguageSelection)
				assert.Equal(t, "eng", aws.ToString(sel[0].SelectorSettings.AudioLanguageSelection.LanguageCode))
				assert.Equal(
					t, types.AudioLanguageSelectionPolicyStrict,
					sel[0].SelectorSettings.AudioLanguageSelection.LanguageSelectionPolicy,
				)

				require.NotNil(t, sel[1].SelectorSettings.AudioHlsRenditionSelection)
				assert.Equal(t, "grp1", aws.ToString(sel[1].SelectorSettings.AudioHlsRenditionSelection.GroupId))

				require.NotNil(t, sel[2].SelectorSettings.AudioPidSelection)
				pidSel := sel[2].SelectorSettings.AudioPidSelection
				assert.Equal(t, int32(101), aws.ToInt32(pidSel.Pid))
				require.Len(t, pidSel.Pids, 1)
				premix := pidSel.Pids[0].PremixSettings
				require.NotNil(t, premix)
				assert.Equal(t, int32(2), aws.ToInt32(premix.Channels))
				assert.InDelta(t, 3.5, aws.ToFloat64(premix.GainDb), 0)
				require.NotNil(t, premix.AudioNormalizationSettings)
				assert.Equal(t, types.AudioNormalizationAlgorithmItu17701, premix.AudioNormalizationSettings.Algorithm)
				require.NotNil(t, premix.RemixSettings)
				require.Len(t, premix.RemixSettings.ChannelMappings, 1)
				assert.Equal(
					t, int32(-3),
					aws.ToInt32(premix.RemixSettings.ChannelMappings[0].InputChannelLevels[0].Gain),
				)

				require.NotNil(t, sel[3].SelectorSettings.AudioTrackSelection)
				assert.Equal(
					t, types.DolbyEProgramSelectionAllChannels,
					sel[3].SelectorSettings.AudioTrackSelection.DolbyEDecode.ProgramSelection,
				)
				require.Len(t, sel[3].SelectorSettings.AudioTrackSelection.Tracks, 1)
				assert.Equal(t, int32(1), aws.ToInt32(sel[3].SelectorSettings.AudioTrackSelection.Tracks[0].Track))
			},
		},
		{
			name: "inputAttachments.inputSettings round-trips captionSelectors' source-format variants " +
				"including the empty aribSourceSettings marker",
			run: func(t *testing.T, client *medialivesdk.Client) {
				t.Helper()

				inpOut, err := client.CreateInput(t.Context(), &medialivesdk.CreateInputInput{
					Name: aws.String("rt-input-caption"), Type: types.InputTypeUdpPush,
				})
				require.NoError(t, err)

				out, err := client.CreateChannel(t.Context(), &medialivesdk.CreateChannelInput{
					Name: aws.String("rt-input-settings-caption"), ChannelClass: types.ChannelClassStandard,
					InputAttachments: []types.InputAttachment{
						{
							InputAttachmentName: aws.String("primary"), InputId: inpOut.Input.Id,
							InputSettings: &types.InputSettings{
								CaptionSelectors: []types.CaptionSelector{
									{
										Name: aws.String("cap-dvb"), LanguageCode: aws.String("eng"),
										SelectorSettings: &types.CaptionSelectorSettings{
											DvbSubSourceSettings: &types.DvbSubSourceSettings{
												OcrLanguage: types.DvbSubOcrLanguageEng, Pid: aws.Int32(200),
											},
										},
									},
									{
										Name: aws.String("cap-embedded"),
										SelectorSettings: &types.CaptionSelectorSettings{
											EmbeddedSourceSettings: &types.EmbeddedSourceSettings{
												Convert608To708:        types.EmbeddedConvert608To708Upconvert,
												Scte20Detection:        types.EmbeddedScte20DetectionAuto,
												Source608ChannelNumber: aws.Int32(1),
											},
										},
									},
									{
										Name: aws.String("cap-teletext"),
										SelectorSettings: &types.CaptionSelectorSettings{
											TeletextSourceSettings: &types.TeletextSourceSettings{
												PageNumber: aws.String("100"),
												OutputRectangle: &types.CaptionRectangle{
													Height: aws.Float64(50), LeftOffset: aws.Float64(10),
													TopOffset: aws.Float64(10), Width: aws.Float64(80),
												},
											},
										},
									},
									{
										Name: aws.String("cap-arib"),
										SelectorSettings: &types.CaptionSelectorSettings{
											AribSourceSettings: &types.AribSourceSettings{},
										},
									},
								},
							},
						},
					},
				})
				require.NoError(t, err)

				desc, err := client.DescribeChannel(t.Context(), &medialivesdk.DescribeChannelInput{
					ChannelId: out.Channel.Id,
				})
				require.NoError(t, err)
				require.Len(t, desc.InputAttachments, 1)
				require.NotNil(t, desc.InputAttachments[0].InputSettings)
				sel := desc.InputAttachments[0].InputSettings.CaptionSelectors
				require.Len(t, sel, 4)

				require.NotNil(t, sel[0].SelectorSettings.DvbSubSourceSettings)
				assert.Equal(t, types.DvbSubOcrLanguageEng, sel[0].SelectorSettings.DvbSubSourceSettings.OcrLanguage)
				assert.Equal(t, int32(200), aws.ToInt32(sel[0].SelectorSettings.DvbSubSourceSettings.Pid))

				require.NotNil(t, sel[1].SelectorSettings.EmbeddedSourceSettings)
				assert.Equal(
					t, types.EmbeddedConvert608To708Upconvert,
					sel[1].SelectorSettings.EmbeddedSourceSettings.Convert608To708,
				)

				require.NotNil(t, sel[2].SelectorSettings.TeletextSourceSettings)
				assert.Equal(t, "100", aws.ToString(sel[2].SelectorSettings.TeletextSourceSettings.PageNumber))
				require.NotNil(t, sel[2].SelectorSettings.TeletextSourceSettings.OutputRectangle)
				assert.InDelta(
					t, 50, aws.ToFloat64(sel[2].SelectorSettings.TeletextSourceSettings.OutputRectangle.Height), 0,
				)

				assert.NotNil(t, sel[3].SelectorSettings.AribSourceSettings)
			},
		},
		{
			name: "inputAttachments.inputSettings round-trips videoSelector, networkInputSettings, and flat filter fields",
			run: func(t *testing.T, client *medialivesdk.Client) {
				t.Helper()

				inpOut, err := client.CreateInput(t.Context(), &medialivesdk.CreateInputInput{
					Name: aws.String("rt-input-video"), Type: types.InputTypeUdpPush,
				})
				require.NoError(t, err)

				out, err := client.CreateChannel(t.Context(), &medialivesdk.CreateChannelInput{
					Name: aws.String("rt-input-settings-video"), ChannelClass: types.ChannelClassStandard,
					InputAttachments: []types.InputAttachment{
						{
							InputAttachmentName: aws.String("primary"), InputId: inpOut.Input.Id,
							InputSettings: &types.InputSettings{
								DeblockFilter:           types.InputDeblockFilterEnabled,
								DenoiseFilter:           types.InputDenoiseFilterEnabled,
								FilterStrength:          aws.Int32(3),
								InputFilter:             types.InputFilterAuto,
								Scte35Pid:               aws.Int32(500),
								Smpte2038DataPreference: types.Smpte2038DataPreferencePrefer,
								SourceEndBehavior:       types.InputSourceEndBehaviorLoop,
								VideoSelector: &types.VideoSelector{
									ColorSpace:      types.VideoSelectorColorSpaceHdr10,
									ColorSpaceUsage: types.VideoSelectorColorSpaceUsageForce,
									ColorSpaceSettings: &types.VideoSelectorColorSpaceSettings{
										Hdr10Settings: &types.Hdr10Settings{
											MaxCll:  aws.Int32(1000),
											MaxFall: aws.Int32(400),
										},
									},
									SelectorSettings: &types.VideoSelectorSettings{
										VideoSelectorProgramId: &types.VideoSelectorProgramId{ProgramId: aws.Int32(7)},
									},
								},
								NetworkInputSettings: &types.NetworkInputSettings{
									ServerValidation: types.NetworkInputServerValidationCheckCryptographyOnly,
									HlsInputSettings: &types.HlsInputSettings{
										Bandwidth: aws.Int32(5000000), BufferSegments: aws.Int32(3),
										Retries: aws.Int32(5), RetryInterval: aws.Int32(2),
										Scte35Source: types.HlsScte35SourceTypeManifest,
									},
								},
							},
						},
					},
				})
				require.NoError(t, err)

				desc, err := client.DescribeChannel(t.Context(), &medialivesdk.DescribeChannelInput{
					ChannelId: out.Channel.Id,
				})
				require.NoError(t, err)
				require.Len(t, desc.InputAttachments, 1)
				s := desc.InputAttachments[0].InputSettings
				require.NotNil(t, s)

				assert.Equal(t, types.InputDeblockFilterEnabled, s.DeblockFilter)
				assert.Equal(t, types.InputDenoiseFilterEnabled, s.DenoiseFilter)
				assert.Equal(t, int32(3), aws.ToInt32(s.FilterStrength))
				assert.Equal(t, types.InputFilterAuto, s.InputFilter)
				assert.Equal(t, int32(500), aws.ToInt32(s.Scte35Pid))
				assert.Equal(t, types.Smpte2038DataPreferencePrefer, s.Smpte2038DataPreference)
				assert.Equal(t, types.InputSourceEndBehaviorLoop, s.SourceEndBehavior)

				require.NotNil(t, s.VideoSelector)
				assert.Equal(t, types.VideoSelectorColorSpaceHdr10, s.VideoSelector.ColorSpace)
				assert.Equal(t, types.VideoSelectorColorSpaceUsageForce, s.VideoSelector.ColorSpaceUsage)
				require.NotNil(t, s.VideoSelector.ColorSpaceSettings)
				require.NotNil(t, s.VideoSelector.ColorSpaceSettings.Hdr10Settings)
				assert.Equal(t, int32(1000), aws.ToInt32(s.VideoSelector.ColorSpaceSettings.Hdr10Settings.MaxCll))
				require.NotNil(t, s.VideoSelector.SelectorSettings)
				require.NotNil(t, s.VideoSelector.SelectorSettings.VideoSelectorProgramId)
				assert.Equal(
					t,
					int32(7),
					aws.ToInt32(s.VideoSelector.SelectorSettings.VideoSelectorProgramId.ProgramId),
				)

				require.NotNil(t, s.NetworkInputSettings)
				assert.Equal(
					t, types.NetworkInputServerValidationCheckCryptographyOnly, s.NetworkInputSettings.ServerValidation,
				)
				require.NotNil(t, s.NetworkInputSettings.HlsInputSettings)
				assert.Equal(t, int32(5000000), aws.ToInt32(s.NetworkInputSettings.HlsInputSettings.Bandwidth))
				assert.Equal(t, types.HlsScte35SourceTypeManifest, s.NetworkInputSettings.HlsInputSettings.Scte35Source)
			},
		},
		{
			name: "UpdateChannel with an omitted extras key preserves the previously-configured value",
			run: func(t *testing.T, client *medialivesdk.Client) {
				t.Helper()

				out, err := client.CreateChannel(t.Context(), &medialivesdk.CreateChannelInput{
					Name: aws.String("rt-update-preserve"), ChannelClass: types.ChannelClassStandard,
					LogLevel: types.LogLevelDebug,
					CdiInputSpecification: &types.CdiInputSpecification{
						Resolution: types.CdiInputResolutionUhd,
					},
				})
				require.NoError(t, err)

				// Update only the name; every extras key is omitted from the request.
				_, err = client.UpdateChannel(t.Context(), &medialivesdk.UpdateChannelInput{
					ChannelId: out.Channel.Id, Name: aws.String("rt-update-preserve-renamed"),
				})
				require.NoError(t, err)

				desc, err := client.DescribeChannel(t.Context(), &medialivesdk.DescribeChannelInput{
					ChannelId: out.Channel.Id,
				})
				require.NoError(t, err)
				assert.Equal(t, "rt-update-preserve-renamed", aws.ToString(desc.Name))
				assert.Equal(t, types.LogLevelDebug, desc.LogLevel, "logLevel must survive an update that omits it")
				require.NotNil(t, desc.CdiInputSpecification)
				assert.Equal(
					t, types.CdiInputResolutionUhd, desc.CdiInputSpecification.Resolution,
					"cdiInputSpecification must survive an update that omits it",
				)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			backend := medialive.NewInMemoryBackend("000000000000", "us-east-1")
			h := medialive.NewHandler(backend)
			client := newTestChannelClient(t, h)
			tc.run(t, client)
		})
	}
}

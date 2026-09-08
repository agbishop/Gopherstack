package mq_test

// Additional/less-central DescribeBroker response fields: actions
// required, security groups, subnet IDs, empty-collection shapes,
// ID/ARN formats, and basic snapshot/restore round trips.

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/mq"
)

func TestDescribeBroker_PasswordNotInUsers(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, http.MethodPost, "/v1/brokers", map[string]any{
		"brokerName": "br-userpwd",
		"engineType": mq.EngineTypeActiveMQ,
		"users": []map[string]any{
			{
				"username": "admin",
				"password": "AdminPassword123!",
				"groups":   []string{"admins"},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	brokerID := parseResponse(t, rec)["brokerId"].(string)
	out := describeTestBroker(t, h, brokerID)

	users, _ := out["users"].([]any)
	for _, u := range users {
		um := u.(map[string]any)
		_, hasPassword := um["password"]
		assert.False(t, hasPassword, "password must NOT appear in DescribeBroker users list")
	}
}

func TestActionsRequired_FieldPresentInDescribeBroker(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)
	br := &mq.Broker{
		BrokerID:    "b-actions-001",
		BrokerName:  "actions-broker",
		BrokerArn:   "arn:aws:mq:us-east-1:123456789012:broker:actions-broker",
		BrokerState: mq.BrokerStateRunning,
		EngineType:  mq.EngineTypeActiveMQ,
		ActionsRequired: []mq.ActionRequired{
			{
				ActionRequiredCode: "ACTION_UPDATE",
				ActionRequiredInfo: "Broker requires upgrade to maintain support.",
			},
		},
	}
	mq.AddBrokerInternal(b, br)

	got, err := b.DescribeBroker("b-actions-001")
	require.NoError(t, err)
	require.Len(t, got.ActionsRequired, 1)
	assert.Equal(t, "ACTION_UPDATE", got.ActionsRequired[0].ActionRequiredCode)
}

func TestActionsRequired_HTTPInDescribeBrokerResponse(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)
	br := &mq.Broker{
		BrokerID:    "b-actions-002",
		BrokerName:  "actions-http-broker",
		BrokerArn:   "arn:aws:mq:us-east-1:123456789012:broker:actions-http-broker",
		BrokerState: mq.BrokerStateRunning,
		EngineType:  mq.EngineTypeActiveMQ,
		ActionsRequired: []mq.ActionRequired{
			{ActionRequiredCode: "NEED_REBOOT", ActionRequiredInfo: "A reboot is required."},
		},
	}
	mq.AddBrokerInternal(b, br)

	h := mq.NewHandler(b)
	out := describeTestBroker(t, h, "b-actions-002")

	actions, ok := out["actionsRequired"].([]any)
	require.True(t, ok, "actionsRequired must be in DescribeBroker response")
	require.Len(t, actions, 1)

	action := actions[0].(map[string]any)
	assert.Equal(t, "NEED_REBOOT", action["actionRequiredCode"])
}

func TestCreateBroker_WithSecurityGroups_RoundTrip(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, http.MethodPost, "/v1/brokers", map[string]any{
		"brokerName":     "sg-broker",
		"engineType":     mq.EngineTypeActiveMQ,
		"securityGroups": []string{"sg-aabbccdd", "sg-11223344"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	brokerID := parseResponse(t, rec)["brokerId"].(string)
	out := describeTestBroker(t, h, brokerID)

	sgs, ok := out["securityGroups"].([]any)
	require.True(t, ok, "securityGroups must be in DescribeBroker")
	assert.Len(t, sgs, 2)
}

// TestCreateBroker_SecurityGroupsCount_Validated locks in
// CreateBrokerInput.SecurityGroups' documented length bound ("1 minimum,
// 125 maximum", api_op_CreateBroker.go in aws-sdk-go-v2/service/mq). The
// field is optional (no "This member is required" doc marker), so omitting
// it entirely must still succeed -- only an explicitly supplied
// out-of-bounds list is rejected.
func TestCreateBroker_SecurityGroupsCount_Validated(t *testing.T) {
	t.Parallel()

	const (
		createMaxSecurityGroups = 125
		createOverMax           = createMaxSecurityGroups + 1
	)

	tests := []struct {
		name           string
		brokerName     string
		securityGroups []string
		wantStatus     int
	}{
		{"omitted", "sg-count-omitted", nil, http.StatusOK},
		{"within-bounds", "sg-count-within", []string{"sg-1", "sg-2"}, http.StatusOK},
		{"explicit-empty", "sg-count-empty", []string{}, http.StatusBadRequest},
		{"over-max", "sg-count-overmax", make([]string, createOverMax), http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			body := map[string]any{
				"brokerName": tt.brokerName,
				"engineType": mq.EngineTypeActiveMQ,
			}
			if tt.securityGroups != nil {
				body["securityGroups"] = tt.securityGroups
			}

			rec := doRequest(t, h, http.MethodPost, "/v1/brokers", body)
			assert.Equal(t, tt.wantStatus, rec.Code, rec.Body.String())
		})
	}
}

// TestUpdateBroker_SecurityGroupsCount_Validated locks in
// UpdateBrokerInput.SecurityGroups' documented length bound ("1 minimum, 5
// maximum", api_op_UpdateBroker.go) -- deliberately smaller than
// CreateBroker's 125.
func TestUpdateBroker_SecurityGroupsCount_Validated(t *testing.T) {
	t.Parallel()

	const (
		updateMaxSecurityGroups = 5
		updateOverMax           = updateMaxSecurityGroups + 1
	)

	tests := []struct {
		name           string
		securityGroups []string
		wantStatus     int
	}{
		{"omitted", nil, http.StatusOK},
		{"within-bounds", []string{"sg-1", "sg-2"}, http.StatusOK},
		{"explicit-empty", []string{}, http.StatusBadRequest},
		{"over-max", make([]string, updateOverMax), http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			brokerID := createTestBroker(t, h, "sg-upd-"+tt.name, mq.EngineTypeActiveMQ)

			body := map[string]any{}
			if tt.securityGroups != nil {
				body["securityGroups"] = tt.securityGroups
			}

			rec := doRequest(t, h, http.MethodPut, "/v1/brokers/"+brokerID, body)
			assert.Equal(t, tt.wantStatus, rec.Code, rec.Body.String())
		})
	}
}

func TestCreateBroker_WithSubnetIDs_RoundTrip(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, http.MethodPost, "/v1/brokers", map[string]any{
		"brokerName": "subnet-broker",
		"engineType": mq.EngineTypeActiveMQ,
		"subnetIds":  []string{"subnet-11111111", "subnet-22222222"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	brokerID := parseResponse(t, rec)["brokerId"].(string)
	out := describeTestBroker(t, h, brokerID)

	subnets, ok := out["subnetIds"].([]any)
	require.True(t, ok, "subnetIds must be in DescribeBroker")
	assert.Len(t, subnets, 2)
}

func TestDescribeBroker_TagsEmptyNotNull(t *testing.T) {
	t.Parallel()

	tests := []struct {
		tags map[string]string
		name string
	}{
		{
			name: "no creation-time tags returns tags:{}",
			tags: nil,
		},
		{
			name: "empty creation-time tags map returns tags:{}",
			tags: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			body := map[string]any{
				"brokerName": "notag-broker",
				"engineType": mq.EngineTypeActiveMQ,
			}
			if tt.tags != nil {
				body["tags"] = tt.tags
			}

			rec := doRequest(t, h, http.MethodPost, "/v1/brokers", body)
			require.Equal(t, http.StatusOK, rec.Code)
			bid := parseResponse(t, rec)["brokerId"].(string)

			resp := describeTestBroker(t, h, bid)

			tags, hasTagsKey := resp["tags"]
			assert.True(t, hasTagsKey, "DescribeBroker must include 'tags' key even when empty")
			assert.IsType(t, map[string]any{}, tags, "'tags' must be an object, not null")
			assert.Empty(t, tags, "'tags' must be empty {} not populated")
		})
	}
}

func TestDescribeBroker_UsersEmptyNotAbsent(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	bid := createTestBroker(t, h, "nouser-broker", mq.EngineTypeActiveMQ)
	resp := describeTestBroker(t, h, bid)

	users, hasUsersKey := resp["users"]
	assert.True(t, hasUsersKey, "DescribeBroker must include 'users' key even when empty")
	assert.IsType(t, []any{}, users, "'users' must be an array, not null")
	assert.Empty(t, users, "'users' must be [] not populated")
}

func TestBrokerID_Shape(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	bid := createTestBroker(t, h, "shape-broker", mq.EngineTypeActiveMQ)

	// UUID format: 8-4-4-4-12 lowercase hex with dashes = 36 chars total
	assert.Len(t, bid, 36, "broker ID must be UUID format (36 chars)")
}

func TestBrokerARN_Shape(t *testing.T) {
	t.Parallel()

	b := mq.NewInMemoryBackend("123456789012", "us-east-1")
	h := mq.NewHandler(b)

	brokerID := createTestBroker(t, h, "arn-broker", mq.EngineTypeActiveMQ)
	arnStr := describeTestBroker(t, h, brokerID)["brokerArn"].(string)

	assert.Equal(t, "arn:aws:mq:us-east-1:123456789012:broker:arn-broker", arnStr)
}

func TestDescribeBroker_CreatedTimestamp(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	bid := createTestBroker(t, h, "ts-broker", mq.EngineTypeActiveMQ)
	resp := describeTestBroker(t, h, bid)

	created, ok := resp["created"].(string)
	require.True(t, ok, "'created' must be a string")
	_, err := time.Parse(time.RFC3339, created)
	assert.NoError(t, err, "'created' must be RFC3339: %q", created)
}

func TestSnapshotRestore_PreservesEndpoints(t *testing.T) {
	t.Parallel()

	b1 := newTestBackend(t)
	br, err := b1.CreateBroker(
		"snap-endpoint-broker", mq.DeploymentModeSingleInstance,
		mq.EngineTypeActiveMQ, "", "",
		false, false, nil, nil, nil, nil,
	)
	require.NoError(t, err)
	require.NotEmpty(t, br.BrokerInstances)

	snap := b1.Snapshot(t.Context())
	require.NotNil(t, snap)

	b2 := mq.NewInMemoryBackend("123456789012", "us-east-1")
	require.NoError(t, b2.Restore(t.Context(), snap))

	restored, err := b2.DescribeBroker(br.BrokerID)
	require.NoError(t, err)
	require.NotEmpty(t, restored.BrokerInstances,
		"broker instances must survive snapshot/restore")
	assert.Equal(t, br.BrokerInstances[0].Endpoints, restored.BrokerInstances[0].Endpoints)
}

func TestSnapshotRestore_PreservesDeploymentMode(t *testing.T) {
	t.Parallel()

	b1 := newTestBackend(t)
	br, err := b1.CreateBroker(
		"snap-mode-broker", mq.DeploymentModeActiveStandby,
		mq.EngineTypeActiveMQ, "", "",
		false, false, nil, nil, nil, nil,
	)
	require.NoError(t, err)
	assert.Equal(t, mq.DeploymentModeActiveStandby, br.DeploymentMode)

	snap := b1.Snapshot(t.Context())
	b2 := mq.NewInMemoryBackend("123456789012", "us-east-1")
	require.NoError(t, b2.Restore(t.Context(), snap))

	restored, err := b2.DescribeBroker(br.BrokerID)
	require.NoError(t, err)
	assert.Equal(t, mq.DeploymentModeActiveStandby, restored.DeploymentMode)
}

func TestBrokerState_InitiallyRunning(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	brokerID := createTestBroker(t, h, "state-init-broker", mq.EngineTypeActiveMQ)
	assert.Equal(t, mq.BrokerStateRunning, describeTestBroker(t, h, brokerID)["brokerState"])
}

func TestBrokerState_AfterDelete_BrokerGone(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	brokerID := createTestBroker(t, h, "state-del-broker", mq.EngineTypeActiveMQ)

	delRec := doRequest(t, h, http.MethodDelete, "/v1/brokers/"+brokerID, nil)
	require.Equal(t, http.StatusOK, delRec.Code)

	descRec := doRequest(t, h, http.MethodGet, "/v1/brokers/"+brokerID, nil)
	assert.Equal(t, http.StatusNotFound, descRec.Code,
		"deleted broker must return 404 on describe")
}

func TestDescribeBroker_ByName(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	brokerID := createTestBroker(t, h, "by-name-broker", mq.EngineTypeActiveMQ)

	rec := doRequest(t, h, http.MethodGet, "/v1/brokers/by-name-broker", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseResponse(t, rec)
	assert.Equal(t, brokerID, out["brokerId"])
	assert.Equal(t, "by-name-broker", out["brokerName"])
}

func TestDescribeBroker_ByName_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, http.MethodGet, "/v1/brokers/nonexistent-name", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestUpdateBrokerWithOptions_NotFound_Backend(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)
	_, err := b.UpdateBrokerWithOptions("no-such-broker", "", "", nil, nil, nil)
	require.Error(t, err)
	require.ErrorIs(t, err, mq.ErrNotFound)
}

func TestDescribeBroker_AllCoreFieldsPresent(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, http.MethodPost, "/v1/brokers", map[string]any{
		"brokerName":              "full-describe-broker",
		"engineType":              mq.EngineTypeActiveMQ,
		"engineVersion":           "5.18.3",
		"hostInstanceType":        "mq.m5.large",
		"deploymentMode":          mq.DeploymentModeSingleInstance,
		"publiclyAccessible":      true,
		"autoMinorVersionUpgrade": true,
	})
	require.Equal(t, http.StatusOK, rec.Code)
	brokerID := parseResponse(t, rec)["brokerId"].(string)

	out := describeTestBroker(t, h, brokerID)

	fields := []string{
		"brokerId", "brokerArn", "brokerName", "brokerState",
		"engineType", "engineVersion", "hostInstanceType",
		"deploymentMode", "publiclyAccessible", "autoMinorVersionUpgrade",
		"created",
	}
	for _, f := range fields {
		assert.Contains(t, out, f, "DescribeBroker response must contain %q", f)
	}

	assert.Equal(t, "full-describe-broker", out["brokerName"])
	assert.Equal(t, mq.EngineTypeActiveMQ, out["engineType"])
	assert.Equal(t, "5.18.3", out["engineVersion"])
	assert.Equal(t, mq.BrokerStateRunning, out["brokerState"])
	assert.Equal(t, true, out["publiclyAccessible"])
	assert.Equal(t, true, out["autoMinorVersionUpgrade"])
}

func TestDescribeBroker_BrokerARNFormat(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	brokerID := createTestBroker(t, h, "arn-test-broker", mq.EngineTypeActiveMQ)
	out := describeTestBroker(t, h, brokerID)
	brokerARN := out["brokerArn"].(string)

	assert.True(t, strings.HasPrefix(brokerARN, "arn:aws:mq:"),
		"brokerArn must start with arn:aws:mq:, got %s", brokerARN)
	assert.Contains(t, brokerARN, "123456789012", "brokerArn must contain the account ID")
	assert.Contains(t, brokerARN, "us-east-1", "brokerArn must contain the region")
}

func TestDescribeBroker_NotFound_Returns404(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, http.MethodGet, "/v1/brokers/nonexistent-id", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, "NotFoundException", parseResponse(t, rec)["__type"])
}

func TestDescribeBroker_TagsRoundTrip(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, http.MethodPost, "/v1/brokers", map[string]any{
		"brokerName": "tagged-describe-broker",
		"engineType": mq.EngineTypeActiveMQ,
		"tags":       map[string]string{"env": "test", "team": "platform"},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	brokerID := parseResponse(t, rec)["brokerId"].(string)

	out := describeTestBroker(t, h, brokerID)
	tags, ok := out["tags"].(map[string]any)
	require.True(t, ok, "tags must be present in DescribeBroker response")
	assert.Equal(t, "test", tags["env"])
	assert.Equal(t, "platform", tags["team"])
}

func TestDeleteBroker_Returns200WithBrokerID(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	brokerID := createTestBroker(t, h, "del-test-broker", mq.EngineTypeActiveMQ)

	rec := doRequest(t, h, http.MethodDelete, "/v1/brokers/"+brokerID, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, brokerID, parseResponse(t, rec)["brokerId"],
		"DeleteBroker must return the deleted broker ID")
}

func TestDeleteBroker_NotInListAfterDelete(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	brokerID := createTestBroker(t, h, "del-list-broker", mq.EngineTypeActiveMQ)

	rec := doRequest(t, h, http.MethodDelete, "/v1/brokers/"+brokerID, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	listRec := doRequest(t, h, http.MethodGet, "/v1/brokers", nil)
	require.Equal(t, http.StatusOK, listRec.Code)

	summaries := parseResponse(t, listRec)["brokerSummaries"].([]any)
	for _, s := range summaries {
		assert.NotEqual(t, brokerID, s.(map[string]any)["brokerId"],
			"deleted broker must not appear in ListBrokers")
	}
}

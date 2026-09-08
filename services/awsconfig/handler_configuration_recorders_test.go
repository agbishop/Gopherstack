package awsconfig_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/awsconfig"
)

func TestAWSConfigHandler_DeleteConfigurationRecorder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(t *testing.T, h *awsconfig.Handler)
		body     any
		name     string
		wantCode int
	}{
		{
			name: "success",
			setup: func(t *testing.T, h *awsconfig.Handler) {
				t.Helper()
				doAWSConfigRequest(t, h, "PutConfigurationRecorder", map[string]any{
					"ConfigurationRecorder": map[string]any{
						"name":    "default",
						"roleARN": "arn:aws:iam::000000000000:role/config",
					},
				})
			},
			body:     map[string]any{"ConfigurationRecorderName": "default"},
			wantCode: http.StatusOK,
		},
		{
			name:     "not_found",
			body:     map[string]any{"ConfigurationRecorderName": "nonexistent"},
			wantCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestAWSConfigHandler(t)
			if tt.setup != nil {
				tt.setup(t, h)
			}

			rec := doAWSConfigRequest(t, h, "DeleteConfigurationRecorder", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

// TestRecorderRecordingGroupRoundtrip verifies RecordingGroup is stored and returned.
func TestRecorderRecordingGroupRoundtrip(t *testing.T) {
	t.Parallel()

	h := newTestAWSConfigHandler(t)
	rec := doAWSConfigRequest(t, h, "PutConfigurationRecorder", map[string]any{
		"ConfigurationRecorder": map[string]any{
			"name":    "default",
			"roleARN": "arn:aws:iam::123456789012:role/config",
			"recordingGroup": map[string]any{
				"allSupported":               true,
				"includeGlobalResourceTypes": true,
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doAWSConfigRequest(t, h, "DescribeConfigurationRecorders", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var out struct {
		ConfigurationRecorders []struct {
			RecordingGroup *struct {
				AllSupported               bool `json:"allSupported"`
				IncludeGlobalResourceTypes bool `json:"includeGlobalResourceTypes"`
			} `json:"recordingGroup"`
		} `json:"ConfigurationRecorders"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.Len(t, out.ConfigurationRecorders, 1)
	require.NotNil(t, out.ConfigurationRecorders[0].RecordingGroup)
	assert.True(t, out.ConfigurationRecorders[0].RecordingGroup.AllSupported)
	assert.True(t, out.ConfigurationRecorders[0].RecordingGroup.IncludeGlobalResourceTypes)
}

// TestRecorderStatusLastStatus verifies DescribeConfigurationRecorderStatus returns lastStatus.
func TestRecorderStatusLastStatus(t *testing.T) {
	t.Parallel()

	b := newTestAWSConfigHandler(t).Backend
	require.NoError(t, b.PutConfigurationRecorder("default", "arn:aws:iam::123:role/r", nil))
	require.NoError(t, b.PutDeliveryChannel("default", "my-bucket", "", "", nil))

	statusBefore := b.DescribeConfigurationRecorderStatus(nil)
	require.Len(t, statusBefore, 1)
	assert.Equal(t, "PENDING", statusBefore[0].LastStatus)
	assert.False(t, statusBefore[0].Recording)

	require.NoError(t, b.StartConfigurationRecorder("default"))

	statusAfter := b.DescribeConfigurationRecorderStatus(nil)
	require.Len(t, statusAfter, 1)
	assert.Equal(t, "SUCCESS", statusAfter[0].LastStatus)
	assert.True(t, statusAfter[0].Recording)
}

// TestListConfigurationRecordersSummaries verifies ListConfigurationRecorders returns summaries.
func TestListConfigurationRecordersSummaries(t *testing.T) {
	t.Parallel()

	h := newTestAWSConfigHandler(t)
	b := h.Backend
	require.NoError(t, b.PutConfigurationRecorder("default", "arn:aws:iam::123:role/r", nil))

	rec := doAWSConfigRequest(t, h, "ListConfigurationRecorders", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var out struct {
		ConfigurationRecorderSummaries []struct {
			Arn            string `json:"arn"`
			Name           string `json:"name"`
			RecordingScope string `json:"recordingScope"`
		} `json:"ConfigurationRecorderSummaries"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.Len(t, out.ConfigurationRecorderSummaries, 1)
	assert.Equal(t, "default", out.ConfigurationRecorderSummaries[0].Name)
	assert.Contains(t, out.ConfigurationRecorderSummaries[0].Arn, "arn:aws:config:")
	assert.Equal(t, "INTERNAL", out.ConfigurationRecorderSummaries[0].RecordingScope)
}

func TestAWSConfigHandler_PutConfigurationRecorder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     any
		name     string
		wantCode int
	}{
		{
			name: "success",
			body: map[string]any{
				"ConfigurationRecorder": map[string]any{
					"name":    "default",
					"roleARN": "arn:aws:iam::000000000000:role/config",
				},
			},
			wantCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestAWSConfigHandler(t)
			rec := doAWSConfigRequest(t, h, "PutConfigurationRecorder", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

func TestAWSConfigHandler_DescribeConfigurationRecorders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(t *testing.T, h *awsconfig.Handler)
		name         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "with_recorder",
			setup: func(t *testing.T, h *awsconfig.Handler) {
				t.Helper()
				doAWSConfigRequest(t, h, "PutConfigurationRecorder", map[string]any{
					"ConfigurationRecorder": map[string]any{
						"name":    "default",
						"roleARN": "arn:aws:iam::000000000000:role/config",
					},
				})
			},
			wantCode:     http.StatusOK,
			wantContains: []string{"ConfigurationRecorders"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestAWSConfigHandler(t)
			if tt.setup != nil {
				tt.setup(t, h)
			}

			rec := doAWSConfigRequest(t, h, "DescribeConfigurationRecorders", nil)
			assert.Equal(t, tt.wantCode, rec.Code)

			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

func TestAWSConfigHandler_StartConfigurationRecorder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     any
		setup    func(t *testing.T, h *awsconfig.Handler)
		name     string
		wantCode int
	}{
		{
			name: "success",
			setup: func(t *testing.T, h *awsconfig.Handler) {
				t.Helper()
				doAWSConfigRequest(t, h, "PutConfigurationRecorder", map[string]any{
					"ConfigurationRecorder": map[string]any{
						"name":    "default",
						"roleARN": "arn:aws:iam::000000000000:role/config",
					},
				})
				doAWSConfigRequest(t, h, "PutDeliveryChannel", map[string]any{
					"DeliveryChannel": map[string]any{
						"name":         "default",
						"s3BucketName": "my-bucket",
					},
				})
			},
			body:     map[string]any{"ConfigurationRecorderName": "default"},
			wantCode: http.StatusOK,
		},
		{
			name:     "not_found",
			body:     map[string]any{"ConfigurationRecorderName": "nonexistent"},
			wantCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestAWSConfigHandler(t)
			if tt.setup != nil {
				tt.setup(t, h)
			}

			rec := doAWSConfigRequest(t, h, "StartConfigurationRecorder", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

func TestAWSConfigHandler_DescribeConfigurationRecorderStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup            func(t *testing.T, h *awsconfig.Handler)
		name             string
		wantCode         int
		wantRecordingLen int
		wantRecording    bool
	}{
		{
			name:             "empty_returns_empty_list",
			wantCode:         http.StatusOK,
			wantRecordingLen: 0,
		},
		{
			name: "pending_recorder_is_not_recording",
			setup: func(t *testing.T, h *awsconfig.Handler) {
				t.Helper()
				doAWSConfigRequest(t, h, "PutConfigurationRecorder", map[string]any{
					"ConfigurationRecorder": map[string]any{"name": "default", "roleARN": "arn:aws:iam::123:role/r"},
				})
			},
			wantCode:         http.StatusOK,
			wantRecordingLen: 1,
			wantRecording:    false,
		},
		{
			name: "active_recorder_is_recording",
			setup: func(t *testing.T, h *awsconfig.Handler) {
				t.Helper()
				doAWSConfigRequest(t, h, "PutConfigurationRecorder", map[string]any{
					"ConfigurationRecorder": map[string]any{"name": "default", "roleARN": "arn:aws:iam::123:role/r"},
				})
				doAWSConfigRequest(t, h, "PutDeliveryChannel", map[string]any{
					"DeliveryChannel": map[string]any{"name": "default", "s3BucketName": "my-bucket"},
				})
				doAWSConfigRequest(t, h, "StartConfigurationRecorder", map[string]any{
					"ConfigurationRecorderName": "default",
				})
			},
			wantCode:         http.StatusOK,
			wantRecordingLen: 1,
			wantRecording:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestAWSConfigHandler(t)
			if tt.setup != nil {
				tt.setup(t, h)
			}

			rec := doAWSConfigRequest(t, h, "DescribeConfigurationRecorderStatus", map[string]any{})

			assert.Equal(t, tt.wantCode, rec.Code)

			var out struct {
				ConfigurationRecordersStatus []struct {
					Name      string `json:"name"`
					Recording bool   `json:"recording"`
				} `json:"ConfigurationRecordersStatus"`
			}

			require.NoError(t, json.NewDecoder(rec.Body).Decode(&out))
			assert.Len(t, out.ConfigurationRecordersStatus, tt.wantRecordingLen)

			if tt.wantRecordingLen > 0 {
				assert.Equal(t, tt.wantRecording, out.ConfigurationRecordersStatus[0].Recording)
			}
		})
	}
}

func TestAWSConfigHandler_AssociateResourceTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body         any
		name         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "success_with_arn",
			body: map[string]any{
				"ConfigurationRecorderArn": "arn:aws:config:us-east-1:000000000000:config-recorder/default",
				"ResourceTypes":            []string{"AWS::EC2::Instance"},
			},
			wantCode:     http.StatusOK,
			wantContains: []string{"ConfigurationRecorder", "AWS::EC2::Instance"},
		},
		{
			name: "empty_resource_types",
			body: map[string]any{
				"ConfigurationRecorderArn": "arn:aws:config:us-east-1:000000000000:config-recorder/default",
				"ResourceTypes":            []string{},
			},
			wantCode:     http.StatusOK,
			wantContains: []string{"ConfigurationRecorder"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestAWSConfigHandler(t)
			require.NoError(t, h.Backend.PutConfigurationRecorder("default", "arn:aws:iam::000000000000:role/r", nil))

			rec := doAWSConfigRequest(t, h, "AssociateResourceTypes", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

func TestAWSConfigHandler_AssociateResourceTypes_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestAWSConfigHandler(t)
	rec := doAWSConfigRequest(t, h, "AssociateResourceTypes", map[string]any{
		"ConfigurationRecorderArn": "arn:aws:config:us-east-1:000000000000:config-recorder/unknown",
		"ResourceTypes":            []string{"AWS::EC2::Instance"},
	})
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "NoSuchConfigurationRecorderException")
}

func TestAWSConfigHandler_StopConfigurationRecorder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(t *testing.T, h *awsconfig.Handler)
		body     any
		name     string
		wantCode int
	}{
		{
			name: "success",
			setup: func(t *testing.T, h *awsconfig.Handler) {
				t.Helper()
				require.NoError(t, h.Backend.PutConfigurationRecorder("default", "arn:aws:iam::000:role/r", nil))
				require.NoError(t, h.Backend.PutDeliveryChannel("default", "my-bucket", "", "", nil))
				require.NoError(t, h.Backend.StartConfigurationRecorder("default"))
			},
			body:     map[string]any{"ConfigurationRecorderName": "default"},
			wantCode: http.StatusOK,
		},
		{
			name:     "not_found",
			body:     map[string]any{"ConfigurationRecorderName": "nonexistent"},
			wantCode: http.StatusNotFound,
		},
		{
			name:     "empty_name_returns_400",
			body:     map[string]any{"ConfigurationRecorderName": ""},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestAWSConfigHandler(t)
			if tt.setup != nil {
				tt.setup(t, h)
			}

			rec := doAWSConfigRequest(t, h, "StopConfigurationRecorder", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

func TestAWSConfigHandler_PutConfigurationRecorder_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     any
		name     string
		wantWire string
		wantCode int
	}{
		{
			name: "empty_name_returns_400",
			body: map[string]any{
				"ConfigurationRecorder": map[string]any{"name": "", "roleARN": "arn:aws:iam::000:role/r"},
			},
			wantCode: http.StatusBadRequest,
			wantWire: "InvalidConfigurationRecorderNameException",
		},
		{
			name: "empty_role_arn_returns_400",
			body: map[string]any{
				"ConfigurationRecorder": map[string]any{"name": "default", "roleARN": ""},
			},
			wantCode: http.StatusBadRequest,
			wantWire: "InvalidRoleException",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestAWSConfigHandler(t)
			rec := doAWSConfigRequest(t, h, "PutConfigurationRecorder", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.wantWire)
		})
	}
}

func TestAWSConfigHandler_DescribeConfigurationRecorders_NameFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body      any
		name      string
		wantCode  int
		wantCount int
	}{
		{
			name:      "no_filter_returns_all",
			body:      map[string]any{},
			wantCode:  http.StatusOK,
			wantCount: 2,
		},
		{
			name:      "filter_one_recorder",
			body:      map[string]any{"ConfigurationRecorderNames": []string{"rec-a"}},
			wantCode:  http.StatusOK,
			wantCount: 1,
		},
		{
			name:      "filter_nonexistent",
			body:      map[string]any{"ConfigurationRecorderNames": []string{"no-such"}},
			wantCode:  http.StatusOK,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestAWSConfigHandler(t)
			require.NoError(t, h.Backend.PutConfigurationRecorder("rec-a", "arn:aws:iam::123:role/r", nil))
			_, _, err := h.Backend.PutServiceLinkedConfigurationRecorder("guardduty.amazonaws.com", nil)
			require.NoError(t, err)

			rec := doAWSConfigRequest(t, h, "DescribeConfigurationRecorders", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			var out map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

			var recorders []any
			require.NoError(t, json.Unmarshal(out["ConfigurationRecorders"], &recorders))
			assert.Len(t, recorders, tt.wantCount)
		})
	}
}

func TestAWSConfigHandler_AssociateResourceTypes_EmptyARN(t *testing.T) {
	t.Parallel()

	h := newTestAWSConfigHandler(t)
	rec := doAWSConfigRequest(t, h, "AssociateResourceTypes", map[string]any{
		"ConfigurationRecorderArn": "",
		"ResourceTypes":            []string{"AWS::EC2::Instance"},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "ValidationException")
}

func TestAWSConfigHandler_ServiceLinkedConfigurationRecorder(t *testing.T) {
	t.Parallel()

	h := newTestAWSConfigHandler(t)

	putRec := doAWSConfigRequest(t, h, "PutServiceLinkedConfigurationRecorder", map[string]any{
		"ServicePrincipal": "guardduty.amazonaws.com",
	})
	require.Equal(t, http.StatusOK, putRec.Code)

	var putOut struct {
		Arn  string `json:"Arn"`
		Name string `json:"Name"`
	}
	require.NoError(t, json.Unmarshal(putRec.Body.Bytes(), &putOut))
	assert.Equal(t, "AWSConfigurationRecorderForGuardduty", putOut.Name)
	assert.NotEmpty(t, putOut.Arn)

	delRec := doAWSConfigRequest(t, h, "DeleteServiceLinkedConfigurationRecorder", map[string]any{
		"ServicePrincipal": "guardduty.amazonaws.com",
	})
	require.Equal(t, http.StatusOK, delRec.Code)

	var delOut struct {
		Arn  string `json:"Arn"`
		Name string `json:"Name"`
	}
	require.NoError(t, json.Unmarshal(delRec.Body.Bytes(), &delOut))
	assert.Equal(t, putOut.Name, delOut.Name)

	// A second delete now 404s (NoSuchConfigurationRecorderException).
	delAgain := doAWSConfigRequest(t, h, "DeleteServiceLinkedConfigurationRecorder", map[string]any{
		"ServicePrincipal": "guardduty.amazonaws.com",
	})
	assert.Equal(t, http.StatusNotFound, delAgain.Code)
}

// putConnectorBody builds a valid PutConnector request body for an Azure
// connector, used across the connector/third-party-recorder handler tests.
func putConnectorBody(clientID, tenantID string) map[string]any {
	return map[string]any{
		"ConnectorConfiguration": map[string]any{
			"azure": map[string]any{
				"clientIdentifier": clientID,
				"tenantIdentifier": tenantID,
			},
		},
	}
}

func TestAWSConfigHandler_Connectors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(t *testing.T, h *awsconfig.Handler)
		body     any
		name     string
		op       string
		wantCode int
	}{
		{
			name:     "put_success",
			op:       "PutConnector",
			body:     putConnectorBody("client-1", "tenant-1"),
			wantCode: http.StatusOK,
		},
		{
			name:     "put_missing_configuration_is_400",
			op:       "PutConnector",
			body:     map[string]any{},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "put_duplicate_configuration_conflicts",
			op:   "PutConnector",
			setup: func(t *testing.T, h *awsconfig.Handler) {
				t.Helper()
				doAWSConfigRequest(t, h, "PutConnector", putConnectorBody("client-1", "tenant-1"))
			},
			body:     putConnectorBody("client-1", "tenant-1"),
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "get_unknown_arn_is_404_equivalent",
			op:       "GetConnector",
			body:     map[string]any{"Arn": "arn:aws:config:us-east-1:000000000000:connector/nonexistent"},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "delete_unknown_arn",
			op:       "DeleteConnector",
			body:     map[string]any{"Arn": "arn:aws:config:us-east-1:000000000000:connector/nonexistent"},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "list_empty",
			op:       "ListConnectors",
			body:     map[string]any{},
			wantCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestAWSConfigHandler(t)
			if tt.setup != nil {
				tt.setup(t, h)
			}

			rec := doAWSConfigRequest(t, h, tt.op, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

// TestAWSConfigHandler_ConnectorsRoundtrip verifies PutConnector's returned
// Arn is retrievable via GetConnector/ListConnectors and gone after
// DeleteConnector, exercising the full wire round trip (not just status codes).
func TestAWSConfigHandler_ConnectorsRoundtrip(t *testing.T) {
	t.Parallel()

	h := newTestAWSConfigHandler(t)

	putRec := doAWSConfigRequest(t, h, "PutConnector", putConnectorBody("client-1", "tenant-1"))
	require.Equal(t, http.StatusOK, putRec.Code)

	var putOut struct {
		Arn string `json:"Arn"`
	}
	require.NoError(t, json.Unmarshal(putRec.Body.Bytes(), &putOut))
	require.NotEmpty(t, putOut.Arn)

	getRec := doAWSConfigRequest(t, h, "GetConnector", map[string]any{"Arn": putOut.Arn})
	require.Equal(t, http.StatusOK, getRec.Code)

	var getOut struct {
		Connector struct {
			Arn                    string `json:"arn"`
			ConnectorConfiguration struct {
				Azure struct {
					ClientIdentifier string `json:"clientIdentifier"`
				} `json:"azure"`
			} `json:"connectorConfiguration"`
		} `json:"Connector"`
	}
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getOut))
	assert.Equal(t, putOut.Arn, getOut.Connector.Arn)
	assert.Equal(t, "client-1", getOut.Connector.ConnectorConfiguration.Azure.ClientIdentifier)

	listRec := doAWSConfigRequest(t, h, "ListConnectors", map[string]any{})
	require.Equal(t, http.StatusOK, listRec.Code)

	var listOut struct {
		ConnectorSummaries []struct {
			Arn      string `json:"arn"`
			Provider string `json:"provider"`
		} `json:"ConnectorSummaries"`
	}
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listOut))
	require.Len(t, listOut.ConnectorSummaries, 1)
	assert.Equal(t, putOut.Arn, listOut.ConnectorSummaries[0].Arn)
	assert.Equal(t, "AZURE", listOut.ConnectorSummaries[0].Provider)

	delRec := doAWSConfigRequest(t, h, "DeleteConnector", map[string]any{"Arn": putOut.Arn})
	require.Equal(t, http.StatusOK, delRec.Code)

	getAgain := doAWSConfigRequest(t, h, "GetConnector", map[string]any{"Arn": putOut.Arn})
	assert.Equal(t, http.StatusBadRequest, getAgain.Code)
	assert.Contains(t, getAgain.Body.String(), "ResourceNotFoundException")
}

func TestAWSConfigHandler_PutThirdPartyServiceLinkedConfigurationRecorder(t *testing.T) {
	t.Parallel()

	h := newTestAWSConfigHandler(t)

	putConnRec := doAWSConfigRequest(t, h, "PutConnector", putConnectorBody("client-1", "tenant-1"))
	require.Equal(t, http.StatusOK, putConnRec.Code)

	var connOut struct {
		Arn string `json:"Arn"`
	}
	require.NoError(t, json.Unmarshal(putConnRec.Body.Bytes(), &connOut))

	body := map[string]any{
		"ServicePrincipal": "thirdparty.amazonaws.com",
		"ConnectorArn":     connOut.Arn,
		"ScopeConfiguration": map[string]any{
			"scopeType":  "tenant",
			"allRegions": true,
		},
	}

	putRec := doAWSConfigRequest(t, h, "PutThirdPartyServiceLinkedConfigurationRecorder", body)
	require.Equal(t, http.StatusOK, putRec.Code)

	var putOut struct {
		Arn  string `json:"Arn"`
		Name string `json:"Name"`
	}
	require.NoError(t, json.Unmarshal(putRec.Body.Bytes(), &putOut))
	assert.NotEmpty(t, putOut.Name)
	assert.NotEmpty(t, putOut.Arn)

	// Visible through the pre-existing DescribeConfigurationRecorders path --
	// the key correctness point: a third-party service-linked recorder must
	// not be an orphan no existing op can observe.
	describeRec := doAWSConfigRequest(t, h, "DescribeConfigurationRecorders", map[string]any{})
	require.Equal(t, http.StatusOK, describeRec.Code)
	assert.Contains(t, describeRec.Body.String(), putOut.Name)
	assert.Contains(t, describeRec.Body.String(), connOut.Arn)

	// Repeating the call with a different connector for the same service
	// principal conflicts (one recorder per service principal).
	putConnRec2 := doAWSConfigRequest(t, h, "PutConnector", putConnectorBody("client-2", "tenant-2"))
	require.Equal(t, http.StatusOK, putConnRec2.Code)

	var connOut2 struct {
		Arn string `json:"Arn"`
	}
	require.NoError(t, json.Unmarshal(putConnRec2.Body.Bytes(), &connOut2))

	conflictBody := map[string]any{
		"ServicePrincipal": "thirdparty.amazonaws.com",
		"ConnectorArn":     connOut2.Arn,
		"ScopeConfiguration": map[string]any{
			"scopeType":  "tenant",
			"allRegions": true,
		},
	}
	conflictRec := doAWSConfigRequest(t, h, "PutThirdPartyServiceLinkedConfigurationRecorder", conflictBody)
	assert.Equal(t, http.StatusBadRequest, conflictRec.Code)
	assert.Contains(t, conflictRec.Body.String(), "ConflictException")
}

package kafka_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/kafka"
)

func TestStateInfo_Roundtrip(t *testing.T) {
	t.Parallel()

	b := kafka.NewInMemoryBackend(testAccountID, testRegion)
	cl := b.AddClusterInternal("state-cl", "3.5.1")

	// Set state info via internal access.
	stored := kafka.GetStoredCluster(b, cl.ClusterArn)
	stored.State = kafka.ClusterStateFailed
	stored.StateInfo = &kafka.StateInfo{
		Code:    "BROKER_STORAGE_FAILURE",
		Message: "EBS volume ran out of space",
	}

	described, err := b.DescribeCluster(context.Background(), cl.ClusterArn)
	require.NoError(t, err)
	assert.Equal(t, kafka.ClusterStateFailed, described.State)
	require.NotNil(t, described.StateInfo)
	assert.Equal(t, "BROKER_STORAGE_FAILURE", described.StateInfo.Code)
	assert.Equal(t, "EBS volume ran out of space", described.StateInfo.Message)
}

// TestRefinement2_NewClusterStateConstants verifies all new state constants are defined.

func TestNewClusterStateConstants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		constant string
		value    string
	}{
		{constant: "UPDATING", value: kafka.ClusterStateUpdating},
		{constant: "REBOOTING_BROKER", value: kafka.ClusterStateRebootingBroker},
		{constant: "MAINTENANCE", value: kafka.ClusterStateMaintenance},
		{constant: "HEALING", value: kafka.ClusterStateHealing},
	}

	for _, tt := range tests {
		t.Run(tt.constant, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.constant, tt.value)
		})
	}
}

// TestRefinement2_EnhancedMonitoring_Constants verifies monitoring constant values.

func TestEnhancedMonitoring_Constants(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "DEFAULT", kafka.EnhancedMonitoringDefault)
	assert.Equal(t, "PER_BROKER", kafka.EnhancedMonitoringPerBroker)
	assert.Equal(t, "PER_TOPIC_PER_BROKER", kafka.EnhancedMonitoringPerTopicPerBroker)
	assert.Equal(t, "PER_TOPIC_PER_PARTITION", kafka.EnhancedMonitoringPerTopicPerPartition)
}

// TestRefinement2_StorageMode_Constants verifies storage mode constant values.

func TestStorageMode_Constants(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "LOCAL", kafka.StorageModeLocal)
	assert.Equal(t, "TIERED", kafka.StorageModeTiered)
}

// TestRefinement2_EncryptionInTransit_Constants verifies encryption constant values.

func TestEncryptionInTransit_Constants(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "TLS", kafka.EncryptionInTransitTLS)
	assert.Equal(t, "TLS_PLAINTEXT", kafka.EncryptionInTransitTLSPlaintext)
	assert.Equal(t, "PLAINTEXT", kafka.EncryptionInTransitPlaintext)
}

// TestRefinement2_ClusterType_Constants verifies cluster type constant values.

func TestClusterType_Constants(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "PROVISIONED", kafka.ClusterTypeProvisioned)
	assert.Equal(t, "SERVERLESS", kafka.ClusterTypeServerless)
}

// TestRefinement2_StorageMode_Roundtrip verifies StorageMode is stored and returned.

func TestStorageMode_Roundtrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mode string
	}{
		{name: "tiered", mode: kafka.StorageModeTiered},
		{name: "local", mode: kafka.StorageModeLocal},
		{name: "empty", mode: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := kafka.NewInMemoryBackend(testAccountID, testRegion)
			cl := b.AddClusterInternal("sm-cl", "3.5.1")

			stored := kafka.GetStoredCluster(b, cl.ClusterArn)
			stored.StorageMode = tt.mode

			described, err := b.DescribeCluster(context.Background(), cl.ClusterArn)
			require.NoError(t, err)
			assert.Equal(t, tt.mode, described.StorageMode)
		})
	}
}

// TestRefinement2_EnhancedMonitoring_Roundtrip verifies EnhancedMonitoring round-trip.

func TestEnhancedMonitoring_Roundtrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		level string
	}{
		{"default", kafka.EnhancedMonitoringDefault},
		{"per_broker", kafka.EnhancedMonitoringPerBroker},
		{"per_topic_per_broker", kafka.EnhancedMonitoringPerTopicPerBroker},
		{"per_topic_per_partition", kafka.EnhancedMonitoringPerTopicPerPartition},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := kafka.NewInMemoryBackend(testAccountID, testRegion)
			cl := b.AddClusterInternal("em-cl", "3.5.1")

			stored := kafka.GetStoredCluster(b, cl.ClusterArn)
			stored.EnhancedMonitoring = tt.level

			described, err := b.DescribeCluster(context.Background(), cl.ClusterArn)
			require.NoError(t, err)
			assert.Equal(t, tt.level, described.EnhancedMonitoring)
		})
	}
}

// TestRefinement2_ListClustersV2_ClusterType verifies ClusterType propagates in ListClustersV2.

func TestListClustersV2_ClusterType(t *testing.T) {
	t.Parallel()

	h, backend := newTestHandlerWithBackend(t)

	// Create one provisioned and one serverless.
	_, err := backend.CreateCluster(context.Background(), "prov-list", "3.5.1", 3, kafka.BrokerNodeGroupInfo{
		InstanceType:  "kafka.m5.large",
		ClientSubnets: []string{"subnet-1"},
	}, nil, nil)
	require.NoError(t, err)

	_, err = backend.CreateServerlessCluster(context.Background(), "srv-list", &kafka.ServerlessClusterInfo{
		VpcConfigs: []kafka.ServerlessVpcConfig{{SubnetIDs: []string{"subnet-2"}}},
	}, nil)
	require.NoError(t, err)

	rec := doKafkaRequest(t, h, http.MethodGet, "/api/v2/clusters", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	clusterList := resp["clusterInfoList"].([]any)
	require.Len(t, clusterList, 2)

	typesByName := make(map[string]string)
	for _, ci := range clusterList {
		info := ci.(map[string]any)
		typesByName[info["clusterName"].(string)] = info["clusterType"].(string)
	}

	assert.Equal(t, kafka.ClusterTypeProvisioned, typesByName["prov-list"])
	assert.Equal(t, kafka.ClusterTypeServerless, typesByName["srv-list"])
}

// TestListClustersV2_Filters verifies clusterNameFilter and clusterTypeFilter,
// documented on ListClustersV2Input (api_op_ListClustersV2.go, kafka@v1.57.2):
// "Specify a prefix of the names of the clusters that you want to list. The
// service lists all the clusters whose names start with this prefix." and
// "Specify either PROVISIONED or SERVERLESS.".
func TestListClustersV2_Filters(t *testing.T) {
	t.Parallel()

	h, backend := newTestHandlerWithBackend(t)

	_, err := backend.CreateCluster(context.Background(), "prod-alpha", "3.5.1", 3, kafka.BrokerNodeGroupInfo{
		InstanceType:  "kafka.m5.large",
		ClientSubnets: []string{"subnet-1"},
	}, nil, nil)
	require.NoError(t, err)

	_, err = backend.CreateCluster(context.Background(), "prod-beta", "3.5.1", 3, kafka.BrokerNodeGroupInfo{
		InstanceType:  "kafka.m5.large",
		ClientSubnets: []string{"subnet-1"},
	}, nil, nil)
	require.NoError(t, err)

	_, err = backend.CreateServerlessCluster(context.Background(), "dev-serverless", &kafka.ServerlessClusterInfo{
		VpcConfigs: []kafka.ServerlessVpcConfig{{SubnetIDs: []string{"subnet-2"}}},
	}, nil)
	require.NoError(t, err)

	tests := []struct {
		name      string
		query     string
		wantNames []string
	}{
		{
			name:      "no_filter",
			query:     "",
			wantNames: []string{"prod-alpha", "prod-beta", "dev-serverless"},
		},
		{
			name:      "name_prefix_matches_two",
			query:     "?clusterNameFilter=prod-",
			wantNames: []string{"prod-alpha", "prod-beta"},
		},
		{
			name:      "name_prefix_matches_none",
			query:     "?clusterNameFilter=zzz",
			wantNames: []string{},
		},
		{
			name:      "type_filter_serverless",
			query:     "?clusterTypeFilter=SERVERLESS",
			wantNames: []string{"dev-serverless"},
		},
		{
			name:      "type_filter_provisioned",
			query:     "?clusterTypeFilter=PROVISIONED",
			wantNames: []string{"prod-alpha", "prod-beta"},
		},
		{
			name:      "name_and_type_filter_combined",
			query:     "?clusterNameFilter=prod-&clusterTypeFilter=PROVISIONED",
			wantNames: []string{"prod-alpha", "prod-beta"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := doKafkaRequest(t, h, http.MethodGet, "/api/v2/clusters"+tt.query, nil)
			require.Equal(t, http.StatusOK, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

			clusterList, _ := resp["clusterInfoList"].([]any)

			gotNames := make([]string, 0, len(clusterList))
			for _, ci := range clusterList {
				gotNames = append(gotNames, ci.(map[string]any)["clusterName"].(string))
			}

			assert.ElementsMatch(t, tt.wantNames, gotNames)
		})
	}
}

// TestListClusters_NameFilter verifies clusterNameFilter on the V1 ListClusters
// op, documented on ListClustersInput (api_op_ListClusters.go, kafka@v1.57.2):
// "Specify a prefix of the name of the clusters that you want to list. The
// service lists all the clusters whose names start with this prefix.".
func TestListClusters_NameFilter(t *testing.T) {
	t.Parallel()

	h, backend := newTestHandlerWithBackend(t)
	backend.AddClusterInternal("prod-alpha", "3.5.1")
	backend.AddClusterInternal("prod-beta", "3.5.1")
	backend.AddClusterInternal("dev-gamma", "3.5.1")

	rec := doKafkaRequest(t, h, http.MethodGet, "/v1/clusters?clusterNameFilter=prod-", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	clusterList, _ := resp["clusterInfoList"].([]any)

	gotNames := make([]string, 0, len(clusterList))
	for _, ci := range clusterList {
		gotNames = append(gotNames, ci.(map[string]any)["clusterName"].(string))
	}

	assert.ElementsMatch(t, []string{"prod-alpha", "prod-beta"}, gotNames)
}

// TestRefinement2_ListClustersV2_ServerlessHasNoProvisionedArm verifies V2 list shape.

func TestListClustersV2_ServerlessHasNoProvisionedArm(t *testing.T) {
	t.Parallel()

	h, backend := newTestHandlerWithBackend(t)
	_, err := backend.CreateServerlessCluster(context.Background(), "srv-noarm", &kafka.ServerlessClusterInfo{
		VpcConfigs: []kafka.ServerlessVpcConfig{{SubnetIDs: []string{"subnet-1"}}},
	}, nil)
	require.NoError(t, err)

	rec := doKafkaRequest(t, h, http.MethodGet, "/api/v2/clusters", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	clusterList := resp["clusterInfoList"].([]any)
	require.Len(t, clusterList, 1)

	ci := clusterList[0].(map[string]any)
	assert.Nil(t, ci["provisioned"], "serverless cluster should not have provisioned arm")
	assert.NotNil(t, ci["serverless"], "serverless cluster should have serverless arm")
}

// TestRefinement2_Persistence_ServerlessCluster verifies serverless cluster survives snapshot/restore.

func TestDeepCopy_ProvisionedThroughput(t *testing.T) {
	t.Parallel()

	b := kafka.NewInMemoryBackend(testAccountID, testRegion)
	cl, err := b.CreateCluster(context.Background(), "pt-alias", "3.5.1", 3, kafka.BrokerNodeGroupInfo{
		InstanceType:  "kafka.m5.large",
		ClientSubnets: []string{"subnet-1"},
		StorageInfo: &kafka.StorageInfo{
			EbsStorageInfo: &kafka.EBSStorageInfo{
				VolumeSize: 100,
				ProvisionedThroughput: &kafka.ProvisionedThroughput{
					Enabled:          true,
					VolumeThroughput: 250,
				},
			},
		},
	}, nil, nil)
	require.NoError(t, err)

	// Mutate returned cluster's ProvisionedThroughput — should not affect stored.
	cl.BrokerNodeGroupInfo.StorageInfo.EbsStorageInfo.ProvisionedThroughput.VolumeThroughput = 999

	described, err := b.DescribeCluster(context.Background(), cl.ClusterArn)
	require.NoError(t, err)
	assert.Equal(t, int32(250),
		described.BrokerNodeGroupInfo.StorageInfo.EbsStorageInfo.ProvisionedThroughput.VolumeThroughput)
}

// TestRefinement2_DeepCopy_ZoneIds verifies no aliasing in ZoneIDs.

func TestDeepCopy_ZoneIds(t *testing.T) {
	t.Parallel()

	b := kafka.NewInMemoryBackend(testAccountID, testRegion)
	zones := []string{"us-east-1a", "us-east-1b"}
	cl, err := b.CreateCluster(context.Background(), "zone-alias", "3.5.1", 3, kafka.BrokerNodeGroupInfo{
		InstanceType:  "kafka.m5.large",
		ClientSubnets: []string{"subnet-1"},
		ZoneIDs:       zones,
	}, nil, nil)
	require.NoError(t, err)

	// Mutate original zones slice — should not affect stored.
	zones[0] = "mutated"
	cl.BrokerNodeGroupInfo.ZoneIDs[0] = "also-mutated"

	described, err := b.DescribeCluster(context.Background(), cl.ClusterArn)
	require.NoError(t, err)
	assert.Equal(t, "us-east-1a", described.BrokerNodeGroupInfo.ZoneIDs[0])
}

// TestRefinement2_OpenMonitoring_InV2Response verifies OpenMonitoring in V2 provisioned arm.

func TestOpenMonitoring_InV2Response(t *testing.T) {
	t.Parallel()

	h, backend := newTestHandlerWithBackend(t)
	cl := backend.AddClusterInternal("om-v2", "3.5.1")

	stored := kafka.GetStoredCluster(backend, cl.ClusterArn)
	stored.OpenMonitoring = &kafka.OpenMonitoring{
		Prometheus: &kafka.PrometheusInfo{
			JmxExporter:  &kafka.JmxExporter{EnabledInBroker: true},
			NodeExporter: &kafka.NodeExporter{EnabledInBroker: false},
		},
	}

	rec := doKafkaRequest(t, h, http.MethodGet, "/api/v2/clusters/"+url.PathEscape(cl.ClusterArn), nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	clInfo := resp["clusterInfo"].(map[string]any)
	provisioned := clInfo["provisioned"].(map[string]any)

	om, ok := provisioned["openMonitoring"].(map[string]any)
	require.True(t, ok, "openMonitoring should be present in V2 provisioned")

	prometheus := om["prometheus"].(map[string]any)
	jmx := prometheus["jmxExporter"].(map[string]any)
	assert.True(t, jmx["enabledInBroker"].(bool))
}

// TestRefinement2_CreateCluster_AllAuthModes verifies various auth mode combinations.

func TestCreateCluster_AllAuthModes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		auth *kafka.ClientAuthentication
		name string
	}{
		{
			name: "no_auth",
			auth: nil,
		},
		{
			name: "sasl_scram",
			auth: &kafka.ClientAuthentication{
				Sasl: &kafka.SaslSettings{
					Scram: &kafka.SaslScram{Enabled: true},
				},
			},
		},
		{
			name: "sasl_iam",
			auth: &kafka.ClientAuthentication{
				Sasl: &kafka.SaslSettings{
					Iam: &kafka.SaslIam{Enabled: true},
				},
			},
		},
		{
			name: "tls_with_ca_arns",
			auth: &kafka.ClientAuthentication{
				TLS: &kafka.TLSSettings{
					Enabled: true,
					CertificateAuthorityArnList: []string{
						"arn:aws:acm-pca:us-east-1:123:certificate-authority/abc",
					},
				},
			},
		},
		{
			name: "unauthenticated",
			auth: &kafka.ClientAuthentication{
				Unauthenticated: &kafka.UnauthenticatedSettings{Enabled: true},
			},
		},
		{
			name: "combined_sasl_and_tls",
			auth: &kafka.ClientAuthentication{
				Sasl: &kafka.SaslSettings{
					Scram: &kafka.SaslScram{Enabled: true},
					Iam:   &kafka.SaslIam{Enabled: true},
				},
				TLS: &kafka.TLSSettings{
					Enabled: true,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := kafka.NewInMemoryBackend(testAccountID, testRegion)
			cl, err := b.CreateCluster(context.Background(), "auth-cl", "3.5.1", 3, kafka.BrokerNodeGroupInfo{
				InstanceType:  "kafka.m5.large",
				ClientSubnets: []string{"subnet-1"},
			}, tt.auth, nil)
			require.NoError(t, err)
			require.NotNil(t, cl)

			// Verify round-trip.
			described, err := b.DescribeCluster(context.Background(), cl.ClusterArn)
			require.NoError(t, err)

			if tt.auth == nil {
				assert.Nil(t, described.ClientAuthentication)
			} else {
				require.NotNil(t, described.ClientAuthentication)
			}

			if tt.auth != nil && tt.auth.Unauthenticated != nil {
				require.NotNil(t, described.ClientAuthentication.Unauthenticated)
				assert.Equal(t, tt.auth.Unauthenticated.Enabled,
					described.ClientAuthentication.Unauthenticated.Enabled)
			}

			if tt.auth != nil && tt.auth.TLS != nil && len(tt.auth.TLS.CertificateAuthorityArnList) > 0 {
				require.NotNil(t, described.ClientAuthentication.TLS)
				assert.Equal(t, tt.auth.TLS.CertificateAuthorityArnList,
					described.ClientAuthentication.TLS.CertificateAuthorityArnList)
			}
		})
	}
}

// TestRefinement2_CreateCluster_V1_HTTP_AuthModes verifies HTTP CreateCluster with auth modes.

func TestCreateCluster_V1_HTTP_AuthModes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body map[string]any
		name string
	}{
		{
			name: "sasl_scram",
			body: map[string]any{
				"clusterName":         "scram-cl",
				"kafkaVersion":        "3.5.1",
				"numberOfBrokerNodes": 3,
				"brokerNodeGroupInfo": map[string]any{
					"instanceType":  "kafka.m5.large",
					"clientSubnets": []string{"subnet-1"},
				},
				"clientAuthentication": map[string]any{
					"sasl": map[string]any{
						"scram": map[string]any{"enabled": true},
					},
				},
			},
		},
		{
			name: "unauthenticated",
			body: map[string]any{
				"clusterName":         "ua-cl",
				"kafkaVersion":        "3.5.1",
				"numberOfBrokerNodes": 3,
				"brokerNodeGroupInfo": map[string]any{
					"instanceType":  "kafka.m5.large",
					"clientSubnets": []string{"subnet-1"},
				},
				"clientAuthentication": map[string]any{
					"unauthenticated": map[string]any{"enabled": true},
				},
			},
		},
		{
			name: "tls_with_ca_arns",
			body: map[string]any{
				"clusterName":         "tls-cl",
				"kafkaVersion":        "3.5.1",
				"numberOfBrokerNodes": 3,
				"brokerNodeGroupInfo": map[string]any{
					"instanceType":  "kafka.m5.large",
					"clientSubnets": []string{"subnet-1"},
				},
				"clientAuthentication": map[string]any{
					"tls": map[string]any{
						"enabled": true,
						"certificateAuthorityArnList": []string{
							"arn:aws:acm-pca:us-east-1:123:certificate-authority/abc",
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doKafkaRequest(t, h, http.MethodPost, "/v1/clusters", tt.body)
			assert.Equal(t, http.StatusOK, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.NotEmpty(t, resp["clusterArn"])
		})
	}
}

// TestRefinement2_UpdateClusterConfiguration_HTTP verifies the HTTP endpoint persists config.

func TestGetBootstrapBrokers_Variants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		auth          *kafka.ClientAuthentication
		connectivity  *kafka.ConnectivityInfo
		name          string
		wantSCRAM     bool
		wantIAM       bool
		wantPublicTLS bool
		wantVpcTLS    bool
	}{
		{
			name:          "no_auth",
			auth:          nil,
			wantSCRAM:     false,
			wantIAM:       false,
			wantPublicTLS: false,
		},
		{
			name: "scram_enabled",
			auth: &kafka.ClientAuthentication{
				Sasl: &kafka.SaslSettings{
					Scram: &kafka.SaslScram{Enabled: true},
				},
			},
			wantSCRAM: true,
			wantIAM:   false,
		},
		{
			name: "iam_enabled",
			auth: &kafka.ClientAuthentication{
				Sasl: &kafka.SaslSettings{
					Iam: &kafka.SaslIam{Enabled: true},
				},
			},
			wantSCRAM: false,
			wantIAM:   true,
		},
		{
			name: "scram_and_iam",
			auth: &kafka.ClientAuthentication{
				Sasl: &kafka.SaslSettings{
					Scram: &kafka.SaslScram{Enabled: true},
					Iam:   &kafka.SaslIam{Enabled: true},
				},
			},
			wantSCRAM: true,
			wantIAM:   true,
		},
		{
			name: "public_access_eips",
			auth: &kafka.ClientAuthentication{
				TLS: &kafka.TLSSettings{Enabled: true},
			},
			connectivity: &kafka.ConnectivityInfo{
				PublicAccess: &kafka.PublicAccess{Type: "SERVICE_PROVIDED_EIPS"},
			},
			wantPublicTLS: true,
		},
		{
			name: "vpc_connectivity_tls",
			auth: &kafka.ClientAuthentication{
				TLS: &kafka.TLSSettings{Enabled: true},
			},
			connectivity: &kafka.ConnectivityInfo{
				VpcConnectivity: &kafka.VpcConnectivity{
					ClientAuthentication: &kafka.VpcConnectivityClientAuthentication{
						TLS: &kafka.VpcConnectivityTLS{Enabled: true},
					},
				},
			},
			wantVpcTLS: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, backend := newTestHandlerWithBackend(t)
			cl, err := backend.CreateCluster(context.Background(), "bs-cl", "3.5.1", 3,
				kafka.BrokerNodeGroupInfo{
					InstanceType:     "kafka.m5.large",
					ClientSubnets:    []string{"subnet-1"},
					ConnectivityInfo: tt.connectivity,
				}, tt.auth, nil)
			require.NoError(t, err)

			rec := doKafkaRequest(t, h, http.MethodGet,
				"/v1/clusters/"+url.PathEscape(cl.ClusterArn)+"/bootstrap-brokers", nil)
			require.Equal(t, http.StatusOK, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

			assert.NotEmpty(t, resp["bootstrapBrokerString"], "plaintext broker string always present")
			assert.NotEmpty(t, resp["bootstrapBrokerStringTls"], "TLS broker string always present")

			if tt.wantSCRAM {
				assert.NotEmpty(t, resp["bootstrapBrokerStringSaslScram"], "SCRAM broker string should be present")
			} else {
				assert.Empty(t, resp["bootstrapBrokerStringSaslScram"], "SCRAM broker string should be absent")
			}

			if tt.wantIAM {
				assert.NotEmpty(t, resp["bootstrapBrokerStringSaslIam"], "IAM broker string should be present")
			} else {
				assert.Empty(t, resp["bootstrapBrokerStringSaslIam"], "IAM broker string should be absent")
			}

			if tt.wantPublicTLS {
				assert.NotEmpty(t, resp["bootstrapBrokerStringPublicTls"], "public TLS broker string should be present")
			}

			if tt.wantVpcTLS {
				assert.NotEmpty(
					t,
					resp["bootstrapBrokerStringVpcConnectivityTls"],
					"VPC TLS broker string should be present",
				)
			}
		})
	}
}

// TestRefinement2_ZookeeperConnectString verifies ZookeeperConnectString in V1 responses.

func TestGetBootstrapBrokers_ScramPublic(t *testing.T) {
	t.Parallel()

	h, backend := newTestHandlerWithBackend(t)
	cl, err := backend.CreateCluster(context.Background(), "scram-pub", "3.5.1", 3,
		kafka.BrokerNodeGroupInfo{
			InstanceType:  "kafka.m5.large",
			ClientSubnets: []string{"subnet-1"},
			ConnectivityInfo: &kafka.ConnectivityInfo{
				PublicAccess: &kafka.PublicAccess{Type: "SERVICE_PROVIDED_EIPS"},
			},
		},
		&kafka.ClientAuthentication{
			Sasl: &kafka.SaslSettings{
				Scram: &kafka.SaslScram{Enabled: true},
			},
		},
		nil)
	require.NoError(t, err)

	rec := doKafkaRequest(t, h, http.MethodGet,
		"/v1/clusters/"+url.PathEscape(cl.ClusterArn)+"/bootstrap-brokers", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	assert.NotEmpty(t, resp["bootstrapBrokerStringSaslScram"])
	assert.NotEmpty(t, resp["bootstrapBrokerStringPublicTls"])
	assert.NotEmpty(t, resp["bootstrapBrokerStringPublicSaslScram"])
}

// TestRefinement2_DescribeCluster_V1_IncludesNewFields verifies new fields present in V1.

func TestZookeeperConnectString(t *testing.T) {
	t.Parallel()

	h, backend := newTestHandlerWithBackend(t)
	cl := backend.AddClusterInternal("zk-conn", "3.5.1")

	rec := doKafkaRequest(t, h, http.MethodGet, "/v1/clusters/"+url.PathEscape(cl.ClusterArn), nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	clInfo := resp["clusterInfo"].(map[string]any)
	zkStr, ok := clInfo["zookeeperConnectString"].(string)
	require.True(t, ok, "zookeeperConnectString should be present in V1 response")
	assert.Contains(t, zkStr, ":2181")
	assert.Contains(t, zkStr, "z-1.")
}

// TestRefinement2_StateInfo_Roundtrip verifies StateInfo roundtrip.

func TestDescribeCluster_V1_IncludesNewFields(t *testing.T) {
	t.Parallel()

	h, backend := newTestHandlerWithBackend(t)
	cl := backend.AddClusterInternal("new-fields-v1", "3.5.1")

	stored := kafka.GetStoredCluster(backend, cl.ClusterArn)
	stored.EnhancedMonitoring = kafka.EnhancedMonitoringPerBroker
	stored.OpenMonitoring = &kafka.OpenMonitoring{
		Prometheus: &kafka.PrometheusInfo{
			JmxExporter: &kafka.JmxExporter{EnabledInBroker: true},
		},
	}
	stored.LoggingInfo = &kafka.LoggingInfo{
		BrokerLogs: &kafka.BrokerLogs{
			CloudWatchLogs: &kafka.CloudWatchLogs{
				Enabled:  true,
				LogGroup: "/aws/msk/test",
			},
		},
	}

	rec := doKafkaRequest(t, h, http.MethodGet, "/v1/clusters/"+url.PathEscape(cl.ClusterArn), nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	clInfo := resp["clusterInfo"].(map[string]any)
	assert.Equal(t, kafka.EnhancedMonitoringPerBroker, clInfo["enhancedMonitoring"])
	assert.NotNil(t, clInfo["openMonitoring"])
	assert.NotNil(t, clInfo["loggingInfo"])
	assert.NotEmpty(t, clInfo["zookeeperConnectString"])
}

// hasSuffix is a helper for test assertions.

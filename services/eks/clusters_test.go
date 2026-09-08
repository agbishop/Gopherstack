package eks_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/services/eks"
)

// countLockSeries counts Prometheus metric series whose "lock" label equals
// name, mirroring pkgs/lockmetrics's own TestRWMutex_CloseRemovesLabelValues
// technique. This package's tests run heavily t.Parallel() and many reuse
// generic fixture names (e.g. "c1") without ever closing them, so unrelated
// concurrent tests can make the global gatherer report a MultiError for
// *other* lock names; Gather still returns every non-conflicting family
// (including this test's own uniquely-named series), so the error itself is
// deliberately not asserted on here.
func countLockSeries(t *testing.T, name string) int {
	t.Helper()

	mfs, _ := prometheus.DefaultGatherer.Gather()

	count := 0

	for _, mf := range mfs {
		for _, m := range mf.GetMetric() {
			for _, lp := range m.GetLabel() {
				if lp.GetName() == "lock" && lp.GetValue() == name {
					count++
				}
			}
		}
	}

	return count
}

func TestEKS_RegisterDeregisterCluster(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)

	// Register cluster. Real path is global POST /cluster-registrations (no
	// cluster name in the URI -- Name comes from the body).
	rec := doREST(t, h, http.MethodPost, "/cluster-registrations", map[string]any{
		"name":                    "my-ext-cluster",
		"connectorConfigProvider": "EKS_ANYWHERE",
		"connectorConfigRoleArn":  "arn:aws:iam::123:role/connector",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// Register cluster missing name
	rec = doREST(t, h, http.MethodPost, "/cluster-registrations", map[string]any{})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// Deregister cluster. Real path is DELETE /cluster-registrations/{name}.
	rec = doREST(t, h, http.MethodDelete, "/cluster-registrations/my-ext-cluster", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	// Deregister nonexistent cluster
	rec = doREST(t, h, http.MethodDelete, "/cluster-registrations/nonexistent", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestClusterVpcConfig_RoundTrip(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	rec := doREST(t, h, http.MethodPost, "/clusters", map[string]any{
		"name":    "vpc-cluster",
		"version": "1.32",
		"roleArn": "arn:aws:iam::123456789012:role/eks",
		"resourcesVpcConfig": map[string]any{
			"subnetIds":             []string{"subnet-aaa", "subnet-bbb"},
			"securityGroupIds":      []string{"sg-111"},
			"endpointPrivateAccess": true,
			"endpointPublicAccess":  false,
			"publicAccessCidrs":     []string{"10.0.0.0/8"},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// Describe cluster returns vpcConfig.
	rec2 := doREST(t, h, http.MethodGet, "/clusters/vpc-cluster", nil)
	require.Equal(t, http.StatusOK, rec2.Code)
	resp := parseResp(t, rec2)
	cluster := resp["cluster"].(map[string]any)

	vpc, ok := cluster["resourcesVpcConfig"].(map[string]any)
	require.True(t, ok, "resourcesVpcConfig should be present")

	subs, _ := vpc["subnetIds"].([]any)
	require.Len(t, subs, 2)
	assert.Equal(t, "subnet-aaa", subs[0])

	sgs, _ := vpc["securityGroupIds"].([]any)
	require.Len(t, sgs, 1)
	assert.Equal(t, "sg-111", sgs[0])

	assert.Equal(t, true, vpc["endpointPrivateAccess"])
	assert.Equal(t, false, vpc["endpointPublicAccess"])

	cidrs, _ := vpc["publicAccessCidrs"].([]any)
	require.Len(t, cidrs, 1)
	assert.Equal(t, "10.0.0.0/8", cidrs[0])
}

func TestClusterVpcConfig_Absent_When_Not_Provided(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "no-vpc"})

	rec := doREST(t, h, http.MethodGet, "/clusters/no-vpc", nil)
	resp := parseResp(t, rec)
	cluster := resp["cluster"].(map[string]any)

	_, hasVpc := cluster["resourcesVpcConfig"]
	assert.False(t, hasVpc, "resourcesVpcConfig should be absent when not provided")
}

func TestClusterVpcConfig_Backend_DeepCopy(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)
	vpcCfg := &eks.VpcConfig{
		SubnetIDs:            []string{"subnet-orig"},
		EndpointPublicAccess: true,
	}
	_, err := b.CreateCluster("c1", "1.32", "", vpcCfg, nil, nil)
	require.NoError(t, err)

	// Mutating the original slice must not affect the stored copy.
	vpcCfg.SubnetIDs[0] = "subnet-mutated"

	c, err := b.DescribeCluster("c1")
	require.NoError(t, err)
	require.NotNil(t, c.VpcConfig)
	assert.Equal(t, "subnet-orig", c.VpcConfig.SubnetIDs[0], "deep copy must isolate stored VpcConfig")
}

func TestKubernetesNetworkConfig_RoundTrip(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{
		"name": "net-cluster",
		"kubernetesNetworkConfig": map[string]any{
			"ipFamily":        "ipv4",
			"serviceIpv4Cidr": "10.96.0.0/12",
		},
	})

	rec := doREST(t, h, http.MethodGet, "/clusters/net-cluster", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	resp := parseResp(t, rec)
	cluster := resp["cluster"].(map[string]any)

	net, ok := cluster["kubernetesNetworkConfig"].(map[string]any)
	require.True(t, ok, "kubernetesNetworkConfig should be present")
	assert.Equal(t, "ipv4", net["ipFamily"])
	assert.Equal(t, "10.96.0.0/12", net["serviceIpv4Cidr"])
}

func TestKubernetesNetworkConfig_IPv6(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)
	netCfg := &eks.KubernetesNetworkConfig{
		IPFamily:        "ipv6",
		ServiceIPv6CIDR: "fd00::/112",
	}
	c, err := b.CreateCluster("ipv6-cluster", "1.32", "", nil, netCfg, nil)
	require.NoError(t, err)
	require.NotNil(t, c.KubernetesNetworkConfig)
	assert.Equal(t, "ipv6", c.KubernetesNetworkConfig.IPFamily)
	assert.Equal(t, "fd00::/112", c.KubernetesNetworkConfig.ServiceIPv6CIDR)
}

func TestKubernetesNetworkConfig_Absent_When_Not_Provided(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "plain"})

	rec := doREST(t, h, http.MethodGet, "/clusters/plain", nil)
	resp := parseResp(t, rec)
	cluster := resp["cluster"].(map[string]any)

	_, hasNet := cluster["kubernetesNetworkConfig"]
	assert.False(t, hasNet)
}

func TestCluster_CreateAndDescribe_AllNewFields(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	rec := doREST(t, h, http.MethodPost, "/clusters", map[string]any{
		"name":    "full-cluster",
		"version": "1.32",
		"roleArn": "arn:aws:iam::123456789012:role/eks",
		"resourcesVpcConfig": map[string]any{
			"subnetIds":             []string{"subnet-1", "subnet-2"},
			"securityGroupIds":      []string{"sg-1"},
			"endpointPrivateAccess": true,
			"endpointPublicAccess":  true,
			"publicAccessCidrs":     []string{"0.0.0.0/0"},
		},
		"kubernetesNetworkConfig": map[string]any{
			"ipFamily":        "ipv4",
			"serviceIpv4Cidr": "172.20.0.0/16",
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doREST(t, h, http.MethodGet, "/clusters/full-cluster", nil)
	require.Equal(t, http.StatusOK, rec2.Code)
	cluster := parseResp(t, rec2)["cluster"].(map[string]any)

	assert.Equal(t, "full-cluster", cluster["name"])
	assert.NotNil(t, cluster["resourcesVpcConfig"])
	assert.NotNil(t, cluster["kubernetesNetworkConfig"])
}

func TestVpcConfig_HandlerJSON_Fields(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{
		"name": "c1",
		"resourcesVpcConfig": map[string]any{
			"subnetIds": []string{"subnet-x"},
		},
	})

	rec := doREST(t, h, http.MethodGet, "/clusters/c1", nil)
	body := rec.Body.Bytes()
	var raw map[string]any
	require.NoError(t, json.Unmarshal(body, &raw))

	cluster := raw["cluster"].(map[string]any)
	vpc := cluster["resourcesVpcConfig"].(map[string]any)

	// All 5 VPC fields should be present.
	_, hasSubnets := vpc["subnetIds"]
	_, hasSGs := vpc["securityGroupIds"]
	_, hasEPA := vpc["endpointPrivateAccess"]
	_, hasEPU := vpc["endpointPublicAccess"]
	_, hasCIDRs := vpc["publicAccessCidrs"]
	assert.True(t, hasSubnets)
	assert.True(t, hasSGs)
	assert.True(t, hasEPA)
	assert.True(t, hasEPU)
	assert.True(t, hasCIDRs)
}

func TestVpcConfig_ClusterSecurityGroupID_Present(t *testing.T) {
	t.Parallel()

	h, _ := newHandlerAndBackend(t)
	rec := doREST(t, h, http.MethodPost, "/clusters", map[string]any{
		"name": "vpc-sg-cluster",
		"resourcesVpcConfig": map[string]any{
			"subnetIds":        []string{"subnet-aaa"},
			"securityGroupIds": []string{"sg-user"},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	cluster := parseClusterResp(t, rec)
	vpc := cluster["resourcesVpcConfig"].(map[string]any)
	clusterSG, ok := vpc["clusterSecurityGroupId"].(string)
	require.True(t, ok, "resourcesVpcConfig must include clusterSecurityGroupId")
	assert.True(t, strings.HasPrefix(clusterSG, "sg-"),
		"clusterSecurityGroupId must start with 'sg-', got: %s", clusterSG)
}

func TestVpcConfig_VpcId_Present(t *testing.T) {
	t.Parallel()

	h, _ := newHandlerAndBackend(t)
	rec := doREST(t, h, http.MethodPost, "/clusters", map[string]any{
		"name": "vpc-id-cluster",
		"resourcesVpcConfig": map[string]any{
			"subnetIds": []string{"subnet-xyz"},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	cluster := parseClusterResp(t, rec)
	vpc := cluster["resourcesVpcConfig"].(map[string]any)
	vpcID, ok := vpc["vpcId"].(string)
	require.True(t, ok, "resourcesVpcConfig must include vpcId")
	assert.True(t, strings.HasPrefix(vpcID, "vpc-"),
		"vpcId must start with 'vpc-', got: %s", vpcID)
}

func TestVpcConfig_ClusterSecurityGroupID_Stable_Per_Cluster(t *testing.T) {
	t.Parallel()

	h, _ := newHandlerAndBackend(t)
	vpcCfg := map[string]any{"subnetIds": []string{"subnet-aaa"}}

	rec1 := doREST(t, h, http.MethodPost, "/clusters",
		map[string]any{"name": "stable-sg-c1", "resourcesVpcConfig": vpcCfg})
	require.Equal(t, http.StatusOK, rec1.Code)

	rec2 := doREST(t, h, http.MethodGet, "/clusters/stable-sg-c1", nil)
	cluster1a := parseClusterResp(t, rec1)
	cluster1b := parseClusterResp(t, rec2)

	sg1a := cluster1a["resourcesVpcConfig"].(map[string]any)["clusterSecurityGroupId"]
	sg1b := cluster1b["resourcesVpcConfig"].(map[string]any)["clusterSecurityGroupId"]
	assert.Equal(t, sg1a, sg1b, "clusterSecurityGroupId must be stable across Describe calls")
}

func TestVpcConfig_DifferentClusters_DifferentSG(t *testing.T) {
	t.Parallel()

	h, _ := newHandlerAndBackend(t)
	vpcCfg := map[string]any{"subnetIds": []string{"subnet-aaa"}}

	rec1 := doREST(t, h, http.MethodPost, "/clusters",
		map[string]any{"name": "diff-sg-c1", "resourcesVpcConfig": vpcCfg})
	rec2 := doREST(t, h, http.MethodPost, "/clusters",
		map[string]any{"name": "diff-sg-c2", "resourcesVpcConfig": vpcCfg})

	sg1 := parseClusterResp(t, rec1)["resourcesVpcConfig"].(map[string]any)["clusterSecurityGroupId"]
	sg2 := parseClusterResp(t, rec2)["resourcesVpcConfig"].(map[string]any)["clusterSecurityGroupId"]
	assert.NotEqual(t, sg1, sg2, "different clusters must get different clusterSecurityGroupIds")
}

func TestCluster_Status_CREATING_On_Create(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	c, err := b.CreateCluster("lifecycle-c1", "1.32", "", nil, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "CREATING", c.Status, "CreateCluster must return CREATING status (async transition to ACTIVE)")
}

func TestCluster_Status_DELETING_On_Delete(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	mustCreateClusterNoVpc(t, b, "del-status-c1")

	deleted, err := b.DeleteCluster("del-status-c1")
	require.NoError(t, err)
	assert.Equal(t, "DELETING", deleted.Status, "DeleteCluster must return DELETING status")
}

func TestCluster_Status_DELETING_Via_Handler(t *testing.T) {
	t.Parallel()

	h, b := newHandlerAndBackend(t)
	mustCreateClusterNoVpc(t, b, "del-handler-c1")

	rec := doREST(t, h, http.MethodDelete, "/clusters/del-handler-c1", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	cluster := parseClusterResp(t, rec)
	assert.Equal(t, "DELETING", cluster["status"])
}

func TestCluster_ARN_Format(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	c, err := b.CreateCluster("arn-test-cluster", "1.32", "", nil, nil, nil)
	require.NoError(t, err)

	expected := fmt.Sprintf("arn:aws:eks:%s:123456789012:cluster/arn-test-cluster", config.DefaultRegion)
	assert.Equal(t, expected, c.ARN)
}

func TestVpcConfig_EndpointPublicAccess_ExplicitTrue_RoundTrip(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	c, err := b.CreateCluster("vpc-explicit", "1.32", "", &eks.VpcConfig{
		SubnetIDs:            []string{"subnet-aaa"},
		EndpointPublicAccess: true,
	}, nil, nil)
	require.NoError(t, err)
	assert.True(t, c.VpcConfig.EndpointPublicAccess)
}

func TestVpcConfig_EndpointPrivateAccess_RoundTrip(t *testing.T) {
	t.Parallel()

	h, _ := newHandlerAndBackend(t)
	rec := doREST(t, h, http.MethodPost, "/clusters", map[string]any{
		"name": "vpc-private",
		"resourcesVpcConfig": map[string]any{
			"subnetIds":             []string{"subnet-aaa"},
			"endpointPrivateAccess": true,
			"endpointPublicAccess":  false,
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	cluster := parseClusterResp(t, rec)
	vpc := cluster["resourcesVpcConfig"].(map[string]any)
	assert.Equal(t, true, vpc["endpointPrivateAccess"])
	assert.Equal(t, false, vpc["endpointPublicAccess"])
}

func TestDescribeCluster_ARN_Present(t *testing.T) {
	t.Parallel()

	h, b := newHandlerAndBackend(t)
	mustCreateClusterNoVpc(t, b, "arn-present-cluster")

	rec := doREST(t, h, http.MethodGet, "/clusters/arn-present-cluster", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	cluster := parseClusterResp(t, rec)
	arn, ok := cluster["arn"].(string)
	require.True(t, ok)
	assert.Contains(t, arn, "arn:aws:eks:")
	assert.Contains(t, arn, "arn-present-cluster")
}

func TestDeleteCluster_Returns_DELETING_Status(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	mustCreateClusterNoVpc(t, b, "del-status-cascade-cluster")

	deleted, err := b.DeleteCluster("del-status-cascade-cluster")
	require.NoError(t, err)
	assert.Equal(t, "DELETING", deleted.Status, "DeleteCluster must return DELETING status in response")
}

func TestCluster_PlatformVersion_Present(t *testing.T) {
	t.Parallel()

	h, b := newHandlerAndBackend(t)
	mustCreateClusterNoVpc(t, b, "pv-cluster")

	rec := doREST(t, h, http.MethodGet, "/clusters/pv-cluster", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	cluster := parseClusterResp(t, rec)
	pv, ok := cluster["platformVersion"].(string)
	require.True(t, ok)
	assert.NotEmpty(t, pv, "platformVersion must be non-empty")
}

func TestListClusters_Sorted(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	for _, name := range []string{"cluster-z", "cluster-a", "cluster-m"} {
		mustCreateClusterNoVpc(t, b, name)
	}

	names := b.ListClusters(false)
	require.Len(t, names, 3)
	assert.Equal(t, "cluster-a", names[0])
	assert.Equal(t, "cluster-m", names[1])
	assert.Equal(t, "cluster-z", names[2])
}

func TestComputeConfig_RoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantValue any
		body      map[string]any
		name      string
		wantField string
	}{
		{
			name: "compute_config_enabled",
			body: map[string]any{
				"name": "auto-cluster",
				"computeConfig": map[string]any{
					"enabled":     true,
					"nodeRoleArn": "arn:aws:iam::123456789012:role/eks-node",
					"nodePools":   []string{"general-purpose", "system"},
				},
			},
			wantField: "enabled",
			wantValue: true,
		},
		{
			name: "compute_config_disabled",
			body: map[string]any{
				"name": "manual-cluster",
				"computeConfig": map[string]any{
					"enabled": false,
				},
			},
			wantField: "enabled",
			wantValue: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestEKSHandler(t)
			rec := doREST(t, h, http.MethodPost, "/clusters", tt.body)
			require.Equal(t, http.StatusOK, rec.Code)

			resp := parseResp(t, rec)
			cluster, ok := resp["cluster"].(map[string]any)
			require.True(t, ok)

			cc, ok := cluster["computeConfig"].(map[string]any)
			require.True(t, ok, "computeConfig must be present in response")
			assert.Equal(t, tt.wantValue, cc[tt.wantField])
		})
	}
}

func TestComputeConfig_NodeRoleArn_And_NodePools(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	rec := doREST(t, h, http.MethodPost, "/clusters", map[string]any{
		"name": "auto-full",
		"computeConfig": map[string]any{
			"enabled":     true,
			"nodeRoleArn": "arn:aws:iam::123456789012:role/eks-node",
			"nodePools":   []string{"general-purpose"},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	desc := doREST(t, h, http.MethodGet, "/clusters/auto-full", nil)
	require.Equal(t, http.StatusOK, desc.Code)
	cluster := parseResp(t, desc)["cluster"].(map[string]any)

	cc := cluster["computeConfig"].(map[string]any)
	assert.Equal(t, "arn:aws:iam::123456789012:role/eks-node", cc["nodeRoleArn"])

	pools, _ := cc["nodePools"].([]any)
	assert.Equal(t, "general-purpose", pools[0])
}

func TestStorageConfig_RoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body        map[string]any
		name        string
		wantEnabled bool
	}{
		{
			name: "storage_enabled",
			body: map[string]any{
				"name": "storage-on",
				"storageConfig": map[string]any{
					"blockStorage": map[string]any{"enabled": true},
				},
			},
			wantEnabled: true,
		},
		{
			name: "storage_disabled",
			body: map[string]any{
				"name": "storage-off",
				"storageConfig": map[string]any{
					"blockStorage": map[string]any{"enabled": false},
				},
			},
			wantEnabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestEKSHandler(t)
			rec := doREST(t, h, http.MethodPost, "/clusters", tt.body)
			require.Equal(t, http.StatusOK, rec.Code)

			desc := doREST(t, h, http.MethodGet, "/clusters/"+tt.body["name"].(string), nil)
			cluster := parseResp(t, desc)["cluster"].(map[string]any)

			sc, ok := cluster["storageConfig"].(map[string]any)
			require.True(t, ok, "storageConfig must be present")
			bs := sc["blockStorage"].(map[string]any)
			assert.Equal(t, tt.wantEnabled, bs["enabled"])
		})
	}
}

// TestNetworkingConfig_RoundTrip covers gopherstack-tp8x: the real
// KubernetesNetworkConfigRequest/Response (eks@v1.90.4 types/types.go:1597,
// 1645) both declare ElasticLoadBalancing as a sibling of ipFamily/
// serviceIpv4Cidr/serviceIpv6Cidr under ONE "kubernetesNetworkConfig" key --
// there is no separate top-level "networkingConfig" object in real AWS. A
// real client's ElasticLoadBalancing setting must be sent inside
// kubernetesNetworkConfig and is returned the same way.
func TestNetworkingConfig_RoundTrip(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	rec := doREST(t, h, http.MethodPost, "/clusters", map[string]any{
		"name": "net-cluster",
		"kubernetesNetworkConfig": map[string]any{
			"ipFamily":             "ipv4",
			"elasticLoadBalancing": map[string]any{"enabled": true},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	createCluster := parseResp(t, rec)["cluster"].(map[string]any)
	assert.NotContains(t, createCluster, "networkingConfig",
		"ElasticLoadBalancing must not be echoed under a separate top-level key")

	createNC, ok := createCluster["kubernetesNetworkConfig"].(map[string]any)
	require.True(t, ok, "kubernetesNetworkConfig must be present")
	assert.Equal(t, "ipv4", createNC["ipFamily"], "siblings of elasticLoadBalancing must still round-trip")
	createELB := createNC["elasticLoadBalancing"].(map[string]any)
	assert.Equal(t, true, createELB["enabled"])

	desc := doREST(t, h, http.MethodGet, "/clusters/net-cluster", nil)
	cluster := parseResp(t, desc)["cluster"].(map[string]any)

	assert.NotContains(t, cluster, "networkingConfig")

	nc, ok := cluster["kubernetesNetworkConfig"].(map[string]any)
	require.True(t, ok, "kubernetesNetworkConfig must be present")
	elb, ok := nc["elasticLoadBalancing"].(map[string]any)
	require.True(t, ok, "elasticLoadBalancing must live inside kubernetesNetworkConfig")
	assert.Equal(t, true, elb["enabled"])
}

func TestAccessConfig_RoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body          map[string]any
		name          string
		wantAuthMode  string
		wantBootstrap bool
	}{
		{
			name: "access_config_api_mode",
			body: map[string]any{
				"name": "api-cluster",
				"accessConfig": map[string]any{
					"authenticationMode":                      "API",
					"bootstrapClusterCreatorAdminPermissions": true,
				},
			},
			wantAuthMode:  "API",
			wantBootstrap: true,
		},
		{
			name: "access_config_config_map",
			body: map[string]any{
				"name": "cm-cluster",
				"accessConfig": map[string]any{
					"authenticationMode":                      "CONFIG_MAP",
					"bootstrapClusterCreatorAdminPermissions": false,
				},
			},
			wantAuthMode:  "CONFIG_MAP",
			wantBootstrap: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestEKSHandler(t)
			rec := doREST(t, h, http.MethodPost, "/clusters", tt.body)
			require.Equal(t, http.StatusOK, rec.Code)

			desc := doREST(t, h, http.MethodGet, "/clusters/"+tt.body["name"].(string), nil)
			cluster := parseResp(t, desc)["cluster"].(map[string]any)

			ac, ok := cluster["accessConfig"].(map[string]any)
			require.True(t, ok, "accessConfig must be present in response")
			assert.Equal(t, tt.wantAuthMode, ac["authenticationMode"])
			assert.Equal(t, tt.wantBootstrap, ac["bootstrapClusterCreatorAdminPermissions"])
		})
	}
}

func TestAccessConfig_Absent_When_Not_Provided(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "no-ac"})

	desc := doREST(t, h, http.MethodGet, "/clusters/no-ac", nil)
	cluster := parseResp(t, desc)["cluster"].(map[string]any)
	_, hasAC := cluster["accessConfig"]
	assert.False(t, hasAC, "accessConfig must be absent when not provided")
}

func TestRegisterCluster_ConnectorConfig_Nested(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body map[string]any
		name string
	}{
		{
			name: "with_connector_config",
			body: map[string]any{
				"name": "reg-cluster-1",
				"connectorConfig": map[string]any{
					"provider": "EKS_ANYWHERE",
					"roleArn":  "arn:aws:iam::123456789012:role/connector",
				},
			},
		},
		{
			name: "without_connector_config",
			body: map[string]any{
				"name": "reg-cluster-2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestEKSHandler(t)
			// RegisterCluster path is the global POST /cluster-registrations.
			// The cluster name comes from the request body, not the path.
			rec := doREST(t, h, http.MethodPost, "/cluster-registrations", tt.body)
			require.Equal(t, http.StatusOK, rec.Code)

			cluster := parseResp(t, rec)["cluster"].(map[string]any)
			assert.Equal(t, tt.body["name"], cluster["name"])
		})
	}
}

func TestSortedListClusters(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)

	for _, name := range []string{"zzz", "aaa", "mmm", "bbb"} {
		doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": name})
	}

	rec := doREST(t, h, http.MethodGet, "/clusters", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	resp := parseResp(t, rec)
	raw, err := json.Marshal(resp["clusters"])
	require.NoError(t, err)

	var names []string
	require.NoError(t, json.Unmarshal(raw, &names))

	require.Equal(t, []string{"aaa", "bbb", "mmm", "zzz"}, names)
}

func TestCreateClusterInitsAllMaps(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)

	_, err := b.CreateCluster("c1", "1.32", "", nil, nil, nil)
	require.NoError(t, err)

	// These should not panic even before any resources have been added.
	_, err = b.CreateAccessEntry("c1", "arn:aws:iam::123:role/r", "STANDARD", "", nil, nil)
	require.NoError(t, err)

	_, err = b.CreateAddon("c1", "coredns", "v1.0", "", "", "", "", nil, nil)
	require.NoError(t, err)
}

// TestDeleteClusterCascade verifies that DeleteCluster still cascades access
// entries (which AWS does not require removed first), but rejects the delete
// outright while a nodegroup is attached (which AWS does require removed first).
func TestDeleteClusterCascade(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)

	_, err := b.CreateCluster("c1", "1.32", "", nil, nil, nil)
	require.NoError(t, err)

	_, err = b.CreateNodegroup("c1", "ng1", "", "", "", "", "", nil, 1, 1, 2, eks.NodegroupInput{}, nil)
	require.NoError(t, err)

	_, err = b.CreateAccessEntry("c1", "arn:aws:iam::123:role/r", "STANDARD", "", nil, nil)
	require.NoError(t, err)

	assert.Equal(t, 1, b.NodegroupCount())
	assert.Equal(t, 1, b.AccessEntryCount())

	_, err = b.DeleteCluster("c1")
	require.ErrorIs(t, err, eks.ErrAlreadyExists, "DeleteCluster must reject a cluster with attached nodegroups")

	_, err = b.DeleteNodegroup("c1", "ng1")
	require.NoError(t, err)

	_, err = b.DeleteCluster("c1")
	require.NoError(t, err)

	assert.Equal(t, 0, b.ClusterCount())
	assert.Equal(t, 0, b.NodegroupCount())
	assert.Equal(t, 0, b.AccessEntryCount())
}

func TestDeleteCluster_RejectedWithFargateProfile(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)

	_, err := b.CreateCluster("fp-c1", "1.32", "", nil, nil, nil)
	require.NoError(t, err)

	_, err = b.CreateFargateProfile("fp-c1", "fp1", "arn:aws:iam::123456789012:role/fp",
		nil, []string{"subnet-aaa"}, nil)
	require.NoError(t, err)

	_, err = b.DeleteCluster("fp-c1")
	require.ErrorIs(t, err, eks.ErrAlreadyExists,
		"DeleteCluster must reject a cluster with an attached fargate profile")

	_, err = b.DeleteFargateProfile("fp-c1", "fp1")
	require.NoError(t, err)

	_, err = b.DeleteCluster("fp-c1")
	require.NoError(t, err, "DeleteCluster must succeed once the fargate profile is removed")
}

func TestClusterHasCertificateAuthority(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "cluster_has_certificate_authority"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestEKSHandler(t)
			rec := doREST(t, h, http.MethodPost, "/clusters", map[string]any{
				"name":    "cl",
				"version": "1.32",
				"roleArn": "arn:aws:iam::123456789012:role/role",
			})
			require.Equal(t, http.StatusOK, rec.Code, tc.name)

			resp := parseResp(t, rec)
			cluster, ok := resp["cluster"].(map[string]any)
			require.True(t, ok, tc.name)

			ca, ok := cluster["certificateAuthority"].(map[string]any)
			require.True(t, ok, tc.name)
			assert.NotEmpty(t, ca["data"], tc.name)
		})
	}
}

func TestRegisterClusterStoresConnectorConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		provider string
	}{
		{name: "gke_provider", provider: "GKE"},
		{name: "eks_anywhere_provider", provider: "EKS_ANYWHERE"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestEKSHandler(t)
			rec := doREST(t, h, http.MethodPost, "/cluster-registrations", map[string]any{
				"name": "ext-cluster",
				"connectorConfig": map[string]any{
					"provider": tc.provider,
					"roleArn":  "arn:aws:iam::123456789012:role/connector-role",
				},
			})
			require.Equal(t, http.StatusOK, rec.Code, tc.name)

			resp := parseResp(t, rec)
			cluster, ok := resp["cluster"].(map[string]any)
			require.True(t, ok, tc.name)

			cc, ok := cluster["connectorConfig"].(map[string]any)
			require.True(t, ok, tc.name)
			assert.Equal(t, tc.provider, cc["provider"], tc.name)
			assert.NotEmpty(t, cc["activationId"], tc.name)
			assert.NotEmpty(t, cc["activationCode"], tc.name)
		})
	}
}

func TestDescribeClusterVersionsHasBooleanDefaultVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "default_version_is_boolean"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestEKSHandler(t)
			rec := doREST(t, h, http.MethodGet, "/cluster-versions", nil)
			require.Equal(t, http.StatusOK, rec.Code, tc.name)

			resp := parseResp(t, rec)
			versions, ok := resp["clusterVersions"].([]any)
			require.True(t, ok, tc.name)
			require.NotEmpty(t, versions, tc.name)

			first, ok := versions[0].(map[string]any)
			require.True(t, ok, tc.name)

			defaultVer, exists := first["defaultVersion"]
			require.True(t, exists, tc.name)
			_, isBool := defaultVer.(bool)
			assert.True(t, isBool, "defaultVersion should be bool, got %T", defaultVer)
		})
	}
}

func TestEKSClusterCRUD(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ops  func(t *testing.T, h *eks.Handler)
		name string
	}{
		{
			name: "create_cluster",
			ops: func(t *testing.T, h *eks.Handler) {
				t.Helper()
				rec := doREST(t, h, http.MethodPost, "/clusters", map[string]any{
					"name":    "my-cluster",
					"version": "1.32",
					"roleArn": "arn:aws:iam::123456789012:role/eks-role",
					"tags":    map[string]string{"Env": "test"},
				})
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseResp(t, rec)
				cluster, ok := resp["cluster"].(map[string]any)
				require.True(t, ok, "response should have cluster key")
				assert.Equal(t, "my-cluster", cluster["name"])
				assert.NotEmpty(t, cluster["arn"])
				assert.Equal(t, "CREATING", cluster["status"])
				assert.Equal(t, "1.32", cluster["version"])
			},
		},
		{
			name: "describe_cluster",
			ops: func(t *testing.T, h *eks.Handler) {
				t.Helper()
				doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "my-cluster"})
				rec := doREST(t, h, http.MethodGet, "/clusters/my-cluster", nil)
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseResp(t, rec)
				cluster, ok := resp["cluster"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "my-cluster", cluster["name"])
			},
		},
		{
			name: "describe_cluster_not_found",
			ops: func(t *testing.T, h *eks.Handler) {
				t.Helper()
				rec := doREST(t, h, http.MethodGet, "/clusters/nonexistent", nil)
				assert.Equal(t, http.StatusNotFound, rec.Code)
			},
		},
		{
			name: "list_clusters",
			ops: func(t *testing.T, h *eks.Handler) {
				t.Helper()
				doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "cluster-a"})
				doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "cluster-b"})
				rec := doREST(t, h, http.MethodGet, "/clusters", nil)
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseResp(t, rec)
				names, ok := resp["clusters"].([]any)
				require.True(t, ok, "clusters key should be a list")
				assert.Len(t, names, 2)
			},
		},
		{
			name: "delete_cluster",
			ops: func(t *testing.T, h *eks.Handler) {
				t.Helper()
				doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "to-delete"})
				rec := doREST(t, h, http.MethodDelete, "/clusters/to-delete", nil)
				assert.Equal(t, http.StatusOK, rec.Code)
				// verify it is gone
				rec2 := doREST(t, h, http.MethodGet, "/clusters/to-delete", nil)
				assert.Equal(t, http.StatusNotFound, rec2.Code)
			},
		},
		{
			name: "create_cluster_duplicate",
			ops: func(t *testing.T, h *eks.Handler) {
				t.Helper()
				doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "dup-cluster"})
				rec := doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "dup-cluster"})
				assert.Equal(t, http.StatusConflict, rec.Code)
			},
		},
		{
			name: "create_cluster_missing_name",
			ops: func(t *testing.T, h *eks.Handler) {
				t.Helper()
				rec := doREST(t, h, http.MethodPost, "/clusters", map[string]any{"version": "1.32"})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.ops(t, newTestEKSHandler(t))
		})
	}
}

func TestEKSBackendListAllClusters(t *testing.T) {
	t.Parallel()

	backend := eks.NewInMemoryBackend(t.Context(), "123456789012", "us-east-1")
	_, _ = backend.CreateCluster("a", "1.32", "", nil, nil, nil)
	_, _ = backend.CreateCluster("b", "1.32", "", nil, nil, nil)

	all := backend.ListAllClusters()
	assert.Len(t, all, 2)
}

// TestDescribeClusterVpcConfigRace exercises DescribeCluster concurrently
// with UpdateClusterConfig. DescribeCluster shallow-copies *Cluster
// (cp := *c), which only copies the VpcConfig/AccessConfig pointers -- the
// returned copy aliases the exact same *VpcConfig/*AccessConfig that
// UpdateClusterConfig mutates in place under lock (c.VpcConfig.SubnetIDs =
// ..., c.AccessConfig.AuthenticationMode = ...). Reading through the
// returned copy's pointer races those in-place writes.
func TestDescribeClusterVpcConfigRace(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", "us-east-1")
	_, err := b.CreateCluster(
		"vpc-race-cl", "1.31", "arn:aws:iam::123456789012:role/role",
		&eks.VpcConfig{SubnetIDs: []string{"subnet-a"}}, nil, nil,
	)
	require.NoError(t, err)

	const iterations = 2000

	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()

		for range iterations {
			c, describeErr := b.DescribeCluster("vpc-race-cl")
			if describeErr != nil {
				continue
			}

			if c.VpcConfig != nil {
				_ = c.VpcConfig.SubnetIDs
			}

			if c.AccessConfig != nil {
				_ = c.AccessConfig.AuthenticationMode
			}
		}
	}()

	go func() {
		defer wg.Done()

		for i := range iterations {
			_, _ = b.UpdateClusterConfig("vpc-race-cl", eks.ClusterConfigUpdate{
				SubnetIDs: []string{fmt.Sprintf("subnet-%d", i)},
			})
		}
	}()

	go func() {
		defer wg.Done()

		for range iterations {
			_, _ = b.UpdateClusterConfig("vpc-race-cl", eks.ClusterConfigUpdate{
				AccessConfig: &eks.AccessConfig{AuthenticationMode: "API"},
			})
		}
	}()

	wg.Wait()
}

// TestDeleteCluster_ClosesAddonTags guards against a resource leak:
// DeleteCluster bulk-removes a cluster's addons from the addons map but must
// also close each addon's *tags.Tags, which owns a lockmetrics.RWMutex
// registered with the global Prometheus collector (pkgs/tags: "It should be
// called when the Tags instance is no longer needed to prevent unbounded
// growth of the global collector."). Every sibling cascade in DeleteCluster
// (capabilities, access entries, identity provider configs, pod identity
// associations) already closes tags; addons previously did not.
func TestDeleteCluster_ClosesAddonTags(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)

	const clusterName = "leak-addon-cluster"
	const addonName = "leak-addon"

	_, err := b.CreateCluster(clusterName, "1.32", "", nil, nil, nil)
	require.NoError(t, err)

	_, err = b.CreateAddon(clusterName, addonName, "", "", "", "", "", map[string]string{"env": "prod"}, nil)
	require.NoError(t, err)

	lockName := "eks.addon." + clusterName + "." + addonName + ".tags"

	before := countLockSeries(t, lockName)
	require.Positive(t, before, "expected at least one lock series before DeleteCluster")

	_, err = b.DeleteCluster(clusterName)
	require.NoError(t, err)

	after := countLockSeries(t, lockName)
	assert.Equal(t, 0, after, "DeleteCluster must close addon Tags, removing its lock series from the collector")
}

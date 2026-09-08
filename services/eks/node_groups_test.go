package eks_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/services/eks"
)

func TestEKS_NodegroupVersionUpdate(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)

	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "ng-upd-cluster"})
	doREST(t, h, http.MethodPost, "/clusters/ng-upd-cluster/node-groups", map[string]any{
		"nodegroupName": "my-ng",
		"nodeRole":      "arn:aws:iam::123:role/ng",
		"subnets":       []string{"subnet-1"},
	})

	// Update nodegroup version
	rec := doREST(
		t,
		h,
		http.MethodPost,
		"/clusters/ng-upd-cluster/node-groups/my-ng/update-version",
		map[string]any{
			"version": "1.33",
		},
	)
	require.Equal(t, http.StatusOK, rec.Code)

	resp := parseResp(t, rec)
	updateID, _ := resp["update"].(map[string]any)["id"].(string)
	require.NotEmpty(t, updateID)

	// Describe the update
	rec = doREST(t, h, http.MethodGet, "/clusters/ng-upd-cluster/updates/"+updateID, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	// DescribeUpdate returns 404 for an unknown update ID.
	rec = doREST(t, h, http.MethodGet, "/clusters/ng-upd-cluster/updates/nonexistent", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestNodegroupSubnets_RoundTrip(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "c1"})

	rec := doREST(t, h, http.MethodPost, "/clusters/c1/node-groups", map[string]any{
		"nodegroupName": "ng1",
		"subnets":       []string{"subnet-aaa", "subnet-bbb"},
		"nodeRole":      "arn:aws:iam::123456789012:role/ng",
		"scalingConfig": map[string]any{"desiredSize": 1, "minSize": 1, "maxSize": 3},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doREST(t, h, http.MethodGet, "/clusters/c1/node-groups/ng1", nil)
	resp := parseResp(t, rec2)
	ng := resp["nodegroup"].(map[string]any)

	subs, _ := ng["subnets"].([]any)
	require.Len(t, subs, 2)
	assert.Equal(t, "subnet-aaa", subs[0])
	assert.Equal(t, "subnet-bbb", subs[1])
}

func TestNodegroupSubnets_Backend_DeepCopy(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)
	_, err := b.CreateCluster("c1", "1.32", "", nil, nil, nil)
	require.NoError(t, err)

	input := eks.NodegroupInput{Subnets: []string{"subnet-x"}}
	ng, err := b.CreateNodegroup("c1", "ng1", "", "", "", "", "", nil, 1, 1, 2, input, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"subnet-x"}, ng.Subnets)

	// Mutate returned copy — must not affect stored copy.
	ng.Subnets[0] = "mutated"
	ng2, err := b.DescribeNodegroup("c1", "ng1")
	require.NoError(t, err)
	assert.Equal(t, "subnet-x", ng2.Subnets[0])
}

func TestNodegroupLabels_RoundTrip(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "c1"})

	rec := doREST(t, h, http.MethodPost, "/clusters/c1/node-groups", map[string]any{
		"nodegroupName": "ng-labeled",
		"nodeRole":      "arn:aws:iam::123456789012:role/ng",
		"subnets":       []string{"subnet-abc"},
		"labels":        map[string]string{"app": "backend", "tier": "compute"},
		"scalingConfig": map[string]any{"desiredSize": 1, "minSize": 1, "maxSize": 3},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doREST(t, h, http.MethodGet, "/clusters/c1/node-groups/ng-labeled", nil)
	ng := parseResp(t, rec2)["nodegroup"].(map[string]any)
	labels, _ := ng["labels"].(map[string]any)
	assert.Equal(t, "backend", labels["app"])
	assert.Equal(t, "compute", labels["tier"])
}

func TestNodegroupTaints_RoundTrip(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "c1"})

	rec := doREST(t, h, http.MethodPost, "/clusters/c1/node-groups", map[string]any{
		"nodegroupName": "ng-tainted",
		"nodeRole":      "arn:aws:iam::123456789012:role/ng",
		"subnets":       []string{"subnet-abc"},
		"taints": []map[string]any{
			{"key": "dedicated", "value": "gpu", "effect": "NoSchedule"},
		},
		"scalingConfig": map[string]any{"desiredSize": 1, "minSize": 1, "maxSize": 3},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doREST(t, h, http.MethodGet, "/clusters/c1/node-groups/ng-tainted", nil)
	ng := parseResp(t, rec2)["nodegroup"].(map[string]any)
	taints, _ := ng["taints"].([]any)
	require.Len(t, taints, 1)
	taint := taints[0].(map[string]any)
	assert.Equal(t, "dedicated", taint["key"])
	assert.Equal(t, "gpu", taint["value"])
	assert.Equal(t, "NoSchedule", taint["effect"])
}

func TestNodegroupTaints_Backend_DeepCopy(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)
	_, err := b.CreateCluster("c1", "1.32", "", nil, nil, nil)
	require.NoError(t, err)

	input := eks.NodegroupInput{
		Taints: []eks.NodegroupTaint{{Key: "k", Value: "v", Effect: "NoSchedule"}},
	}
	ng, err := b.CreateNodegroup("c1", "ng1", "", "", "", "", "", nil, 1, 1, 2, input, nil)
	require.NoError(t, err)

	// Mutate returned taints — must not affect stored copy.
	ng.Taints[0].Key = "mutated"
	ng2, err := b.DescribeNodegroup("c1", "ng1")
	require.NoError(t, err)
	assert.Equal(t, "k", ng2.Taints[0].Key)
}

func TestNodegroupRemoteAccess_RoundTrip(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "c1"})

	rec := doREST(t, h, http.MethodPost, "/clusters/c1/node-groups", map[string]any{
		"nodegroupName": "ng-ssh",
		"nodeRole":      "arn:aws:iam::123456789012:role/ng",
		"subnets":       []string{"subnet-abc"},
		"remoteAccess": map[string]any{
			"ec2SshKey":            "my-keypair",
			"sourceSecurityGroups": []string{"sg-bastion"},
		},
		"scalingConfig": map[string]any{"desiredSize": 1, "minSize": 1, "maxSize": 3},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doREST(t, h, http.MethodGet, "/clusters/c1/node-groups/ng-ssh", nil)
	ng := parseResp(t, rec2)["nodegroup"].(map[string]any)
	ra, ok := ng["remoteAccess"].(map[string]any)
	require.True(t, ok, "remoteAccess should be present")
	assert.Equal(t, "my-keypair", ra["ec2SshKey"])
	sgs, _ := ra["sourceSecurityGroups"].([]any)
	require.Len(t, sgs, 1)
	assert.Equal(t, "sg-bastion", sgs[0])
}

func TestNodegroupRemoteAccess_Backend_DeepCopy(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)
	_, err := b.CreateCluster("c1", "1.32", "", nil, nil, nil)
	require.NoError(t, err)

	ra := &eks.RemoteAccess{EC2SSHKey: "key1", SourceSecurityGroups: []string{"sg-x"}}
	input := eks.NodegroupInput{RemoteAccess: ra}
	ng, err := b.CreateNodegroup("c1", "ng1", "", "", "", "", "", nil, 1, 1, 2, input, nil)
	require.NoError(t, err)
	require.NotNil(t, ng.RemoteAccess)

	// Mutate original.
	ra.SourceSecurityGroups[0] = "sg-mutated"
	ng2, err := b.DescribeNodegroup("c1", "ng1")
	require.NoError(t, err)
	assert.Equal(t, "sg-x", ng2.RemoteAccess.SourceSecurityGroups[0])
}

func TestNodegroupLaunchTemplate_RoundTrip(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "c1"})

	rec := doREST(t, h, http.MethodPost, "/clusters/c1/node-groups", map[string]any{
		"nodegroupName": "ng-lt",
		"nodeRole":      "arn:aws:iam::123456789012:role/ng",
		"launchTemplate": map[string]any{
			"id":      "lt-0123456789abcdef0",
			"name":    "my-lt",
			"version": "$Default",
		},
		"scalingConfig": map[string]any{"desiredSize": 1, "minSize": 1, "maxSize": 3},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doREST(t, h, http.MethodGet, "/clusters/c1/node-groups/ng-lt", nil)
	ng := parseResp(t, rec2)["nodegroup"].(map[string]any)
	lt, ok := ng["launchTemplate"].(map[string]any)
	require.True(t, ok, "launchTemplate should be present")
	assert.Equal(t, "lt-0123456789abcdef0", lt["id"])
	assert.Equal(t, "my-lt", lt["name"])
	assert.Equal(t, "$Default", lt["version"])
}

func TestNodegroupLaunchTemplate_AbsentWhenNotSet(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "c1"})
	doREST(t, h, http.MethodPost, "/clusters/c1/node-groups", map[string]any{
		"nodegroupName": "ng-nolt",
		"nodeRole":      "arn:aws:iam::123456789012:role/ng",
		"subnets":       []string{"subnet-abc"},
		"scalingConfig": map[string]any{"desiredSize": 1, "minSize": 1, "maxSize": 3},
	})

	rec := doREST(t, h, http.MethodGet, "/clusters/c1/node-groups/ng-nolt", nil)
	ng := parseResp(t, rec)["nodegroup"].(map[string]any)
	_, hasLT := ng["launchTemplate"]
	assert.False(t, hasLT, "launchTemplate must be absent when not provided")
}

func TestNodegroupDiskSize_RoundTrip(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "c1"})

	rec := doREST(t, h, http.MethodPost, "/clusters/c1/node-groups", map[string]any{
		"nodegroupName": "ng-disk",
		"nodeRole":      "arn:aws:iam::123456789012:role/ng",
		"subnets":       []string{"subnet-abc"},
		"diskSize":      100,
		"scalingConfig": map[string]any{"desiredSize": 1, "minSize": 1, "maxSize": 3},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doREST(t, h, http.MethodGet, "/clusters/c1/node-groups/ng-disk", nil)
	ng := parseResp(t, rec2)["nodegroup"].(map[string]any)
	diskSize, _ := ng["diskSize"].(float64)
	assert.InDelta(t, 100.0, diskSize, 0)
}

func TestNodegroupDiskSize_TooSmall_Rejected(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)
	_, err := b.CreateCluster("c1", "1.32", "", nil, nil, nil)
	require.NoError(t, err)

	_, err = b.CreateNodegroup("c1", "ng1", "", "", "", "", "", nil, 1, 1, 2,
		eks.NodegroupInput{DiskSize: 5}, nil)
	require.ErrorIs(t, err, eks.ErrValidation, "diskSize < 20 must return ErrValidation")
}

func TestNodegroupDiskSize_TooLarge_Rejected(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)
	_, err := b.CreateCluster("c1", "1.32", "", nil, nil, nil)
	require.NoError(t, err)

	_, err = b.CreateNodegroup("c1", "ng1", "", "", "", "", "", nil, 1, 1, 2,
		eks.NodegroupInput{DiskSize: 20000}, nil)
	require.ErrorIs(t, err, eks.ErrValidation, "diskSize > 16384 must return ErrValidation")
}

func TestNodegroupDiskSize_Boundary_Min_OK(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)
	_, err := b.CreateCluster("c1", "1.32", "", nil, nil, nil)
	require.NoError(t, err)

	ng, err := b.CreateNodegroup("c1", "ng1", "", "", "", "", "", nil, 1, 1, 2,
		eks.NodegroupInput{DiskSize: 20}, nil)
	require.NoError(t, err)
	assert.Equal(t, int32(20), ng.DiskSize)
}

func TestNodegroupDiskSize_Boundary_Max_OK(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)
	_, err := b.CreateCluster("c1", "1.32", "", nil, nil, nil)
	require.NoError(t, err)

	ng, err := b.CreateNodegroup("c1", "ng1", "", "", "", "", "", nil, 1, 1, 2,
		eks.NodegroupInput{DiskSize: 16384}, nil)
	require.NoError(t, err)
	assert.Equal(t, int32(16384), ng.DiskSize)
}

// TestCreateNodegroup_Version_MismatchRejected guards api_op_CreateNodegroup.go's
// Version field doc: "By default, the Kubernetes version of the cluster is
// used, and this is the only accepted specified value" for its cluster.
func TestCreateNodegroup_Version_MismatchRejected(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)
	_, err := b.CreateCluster("c1", "1.32", "", nil, nil, nil)
	require.NoError(t, err)

	_, err = b.CreateNodegroup("c1", "ng1", "", "", "", "1.31", "", nil, 1, 1, 2, eks.NodegroupInput{}, nil)
	require.ErrorIs(t, err, eks.ErrValidation, "a nodegroup version that does not match the cluster's must be rejected")
}

// TestCreateNodegroup_Version_DefaultsToClusterVersion guards the same doc
// sentence's default case: an omitted version must resolve to (and be
// echoed back as) the cluster's own Kubernetes version, not stay empty.
func TestCreateNodegroup_Version_DefaultsToClusterVersion(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)
	_, err := b.CreateCluster("c1", "1.32", "", nil, nil, nil)
	require.NoError(t, err)

	ng, err := b.CreateNodegroup("c1", "ng1", "", "", "", "", "", nil, 1, 1, 2, eks.NodegroupInput{}, nil)
	require.NoError(t, err)
	assert.Equal(t, "1.32", ng.Version, "an omitted nodegroup version must default to the cluster's Kubernetes version")
}

func TestNodegroupDiskSize_Zero_Omitted(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "c1"})
	doREST(t, h, http.MethodPost, "/clusters/c1/node-groups", map[string]any{
		"nodegroupName": "ng-nodisk",
		"nodeRole":      "arn:aws:iam::123456789012:role/ng",
		"subnets":       []string{"subnet-abc"},
		"scalingConfig": map[string]any{"desiredSize": 1, "minSize": 1, "maxSize": 3},
	})

	rec := doREST(t, h, http.MethodGet, "/clusters/c1/node-groups/ng-nodisk", nil)
	ng := parseResp(t, rec)["nodegroup"].(map[string]any)
	_, hasDisk := ng["diskSize"]
	assert.False(t, hasDisk, "diskSize must be absent when zero")
}

func TestNodegroupASG_PresentOnDescribe(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "c1"})
	doREST(t, h, http.MethodPost, "/clusters/c1/node-groups", map[string]any{
		"nodegroupName": "ng-asg",
		"nodeRole":      "arn:aws:iam::123456789012:role/ng",
		"subnets":       []string{"subnet-abc"},
		"scalingConfig": map[string]any{"desiredSize": 2, "minSize": 1, "maxSize": 5},
	})

	rec := doREST(t, h, http.MethodGet, "/clusters/c1/node-groups/ng-asg", nil)
	ng := parseResp(t, rec)["nodegroup"].(map[string]any)
	resources, ok := ng["resources"].(map[string]any)
	require.True(t, ok, "resources should be present on describe")
	asgs, _ := resources["autoScalingGroups"].([]any)
	require.Len(t, asgs, 1, "exactly one ASG expected")
	asg := asgs[0].(map[string]any)
	assert.NotEmpty(t, asg["name"], "ASG name must be non-empty")
}

func TestNodegroupASG_Name_StableAcrossDescribes(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)
	_, err := b.CreateCluster("c1", "1.32", "", nil, nil, nil)
	require.NoError(t, err)
	ng, err := b.CreateNodegroup("c1", "ng1", "", "", "", "", "", nil, 1, 1, 2, eks.NodegroupInput{}, nil)
	require.NoError(t, err)
	require.NotNil(t, ng.Resources)
	require.Len(t, ng.Resources.AutoScalingGroups, 1)
	asgName := ng.Resources.AutoScalingGroups[0].Name

	ng2, err := b.DescribeNodegroup("c1", "ng1")
	require.NoError(t, err)
	assert.Equal(t, asgName, ng2.Resources.AutoScalingGroups[0].Name, "ASG name must be stable")
}

func TestNodegroupASG_Name_DifferentPerNodegroup(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)
	_, err := b.CreateCluster("c1", "1.32", "", nil, nil, nil)
	require.NoError(t, err)

	ng1, err := b.CreateNodegroup("c1", "ng-a", "", "", "", "", "", nil, 1, 1, 2, eks.NodegroupInput{}, nil)
	require.NoError(t, err)
	ng2, err := b.CreateNodegroup("c1", "ng-b", "", "", "", "", "", nil, 1, 1, 2, eks.NodegroupInput{}, nil)
	require.NoError(t, err)

	assert.NotEqual(t, ng1.Resources.AutoScalingGroups[0].Name, ng2.Resources.AutoScalingGroups[0].Name,
		"each nodegroup must get a unique ASG name")
}

func TestDeleteCluster_RejectedWithNodegroups(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)
	_, err := b.CreateCluster("c1", "1.32", "", nil, nil, nil)
	require.NoError(t, err)
	_, err = b.CreateNodegroup("c1", "ng1", "", "", "", "", "", nil, 1, 1, 2, eks.NodegroupInput{}, nil)
	require.NoError(t, err)

	assert.Equal(t, 1, b.NodegroupCount())

	_, err = b.DeleteCluster("c1")
	require.ErrorIs(t, err, eks.ErrAlreadyExists, "DeleteCluster must reject a cluster with attached nodegroups")
	assert.Equal(t, 1, b.NodegroupCount())

	_, err = b.DeleteNodegroup("c1", "ng1")
	require.NoError(t, err)

	_, err = b.DeleteCluster("c1")
	require.NoError(t, err, "DeleteCluster must succeed once nodegroups are removed")
}

func TestNodegroup_AllOptionalFields_RoundTrip(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "c1"})

	rec := doREST(t, h, http.MethodPost, "/clusters/c1/node-groups", map[string]any{
		"nodegroupName": "ng-full",
		"nodeRole":      "arn:aws:iam::123456789012:role/ng",
		"subnets":       []string{"subnet-1"},
		"labels":        map[string]string{"env": "prod"},
		"taints": []map[string]any{
			{"key": "dedicated", "value": "ml", "effect": "NoSchedule"},
		},
		"diskSize": 200,
		"remoteAccess": map[string]any{
			"ec2SshKey": "prod-key",
		},
		"launchTemplate": map[string]any{
			"id": "lt-abc123",
		},
		"scalingConfig": map[string]any{"desiredSize": 2, "minSize": 1, "maxSize": 10},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doREST(t, h, http.MethodGet, "/clusters/c1/node-groups/ng-full", nil)
	ng := parseResp(t, rec2)["nodegroup"].(map[string]any)

	assert.NotNil(t, ng["subnets"])
	assert.NotNil(t, ng["labels"])
	assert.NotNil(t, ng["taints"])
	assert.NotNil(t, ng["diskSize"])
	assert.NotNil(t, ng["remoteAccess"])
	assert.NotNil(t, ng["launchTemplate"])
	assert.NotNil(t, ng["resources"])
}

func TestNodegroupASG_Resources_DeepCopy(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)
	_, err := b.CreateCluster("c1", "1.32", "", nil, nil, nil)
	require.NoError(t, err)
	ng, err := b.CreateNodegroup("c1", "ng1", "", "", "", "", "", nil, 1, 1, 2, eks.NodegroupInput{}, nil)
	require.NoError(t, err)

	// Mutate returned resources — must not affect stored copy.
	ng.Resources.AutoScalingGroups[0].Name = "mutated"
	ng2, err := b.DescribeNodegroup("c1", "ng1")
	require.NoError(t, err)
	assert.NotEqual(t, "mutated", ng2.Resources.AutoScalingGroups[0].Name)
}

func TestUpdate_Params_NodegroupVersion(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	mustCreateCluster(t, b, "ng-upd-cluster")
	mustCreateNodegroup(t, b, "ng-upd-cluster")

	upd, err := b.UpdateNodegroupVersion("ng-upd-cluster", "ng1", "1.33")
	require.NoError(t, err)
	require.NotEmpty(t, upd.Params, "UpdateNodegroupVersion must populate Params")
	assert.Equal(t, "Version", upd.Params[0].Type)
	assert.Equal(t, "1.33", upd.Params[0].Value)
}

func TestNodegroup_Status_CREATING_On_Create(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	mustCreateClusterNoVpc(t, b, "ng-status-cluster")
	ng := mustCreateNodegroup(t, b, "ng-status-cluster")
	assert.Equal(t, "CREATING", ng.Status, "CreateNodegroup must return CREATING status (async transition to ACTIVE)")
}

func TestNodegroup_Status_DELETING_On_Delete(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	mustCreateClusterNoVpc(t, b, "ng-del-cluster")
	mustCreateNodegroup(t, b, "ng-del-cluster")

	deleted, err := b.DeleteNodegroup("ng-del-cluster", "ng1")
	require.NoError(t, err)
	assert.Equal(t, "DELETING", deleted.Status, "DeleteNodegroup must return DELETING status")
}

func TestNodegroup_ARN_Format(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	mustCreateClusterNoVpc(t, b, "arn-ng-cluster")
	ng := mustCreateNodegroup(t, b, "arn-ng-cluster")

	assert.Contains(t, ng.ARN, "arn:aws:eks:")
	assert.Contains(t, ng.ARN, ":nodegroup/")
	assert.Contains(t, ng.ARN, "arn-ng-cluster")
	assert.Contains(t, ng.ARN, "ng1")
}

func TestNodegroup_ScalingConfig_RoundTrip(t *testing.T) {
	t.Parallel()

	h, b := newHandlerAndBackend(t)
	mustCreateClusterNoVpc(t, b, "ng-scaling-cluster")

	rec := doREST(t, h, http.MethodPost, "/clusters/ng-scaling-cluster/node-groups", map[string]any{
		"nodegroupName": "ng-scaled",
		"nodeRole":      "arn:aws:iam::123:role/ng",
		"subnets":       []string{"subnet-aaa"},
		"scalingConfig": map[string]any{
			"desiredSize": 3,
			"minSize":     1,
			"maxSize":     10,
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	ng := parseNodegroupResp(t, rec)
	scaling := ng["scalingConfig"].(map[string]any)
	assert.InDelta(t, float64(3), scaling["desiredSize"], 0)
	assert.InDelta(t, float64(1), scaling["minSize"], 0)
	assert.InDelta(t, float64(10), scaling["maxSize"], 0)
}

func TestNodegroup_UpdateScalingConfig(t *testing.T) {
	t.Parallel()

	h, b := newHandlerAndBackend(t)
	mustCreateClusterNoVpc(t, b, "ng-upd-scale-cluster")

	doREST(t, h, http.MethodPost, "/clusters/ng-upd-scale-cluster/node-groups", map[string]any{
		"nodegroupName": "ng-to-scale",
		"nodeRole":      "arn:aws:iam::123:role/ng",
		"subnets":       []string{"subnet-aaa"},
		"scalingConfig": map[string]any{"desiredSize": 1, "minSize": 1, "maxSize": 5},
	})

	rec := doREST(t, h, http.MethodPost, "/clusters/ng-upd-scale-cluster/node-groups/ng-to-scale/update-config",
		map[string]any{
			"scalingConfig": map[string]any{"desiredSize": 4, "minSize": 2, "maxSize": 8},
		})
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doREST(t, h, http.MethodGet, "/clusters/ng-upd-scale-cluster/node-groups/ng-to-scale", nil)
	ng := parseNodegroupResp(t, rec2)
	scaling := ng["scalingConfig"].(map[string]any)
	assert.InDelta(t, float64(4), scaling["desiredSize"], 0)
	assert.InDelta(t, float64(2), scaling["minSize"], 0)
	assert.InDelta(t, float64(8), scaling["maxSize"], 0)
}

func TestDescribeNodegroup_ARN_Present(t *testing.T) {
	t.Parallel()

	h, b := newHandlerAndBackend(t)
	mustCreateClusterNoVpc(t, b, "ng-arn-cluster")

	doREST(t, h, http.MethodPost, "/clusters/ng-arn-cluster/node-groups", map[string]any{
		"nodegroupName": "ng-arn",
		"nodeRole":      "arn:aws:iam::123:role/ng",
		"subnets":       []string{"subnet-aaa"},
	})

	rec := doREST(t, h, http.MethodGet, "/clusters/ng-arn-cluster/node-groups/ng-arn", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	ng := parseNodegroupResp(t, rec)
	arn, ok := ng["nodegroupArn"].(string)
	require.True(t, ok)
	assert.Contains(t, arn, "arn:aws:eks:")
	assert.Contains(t, arn, "ng-arn-cluster")
}

func TestDeleteCluster_RejectedThenSucceedsAfterNodegroupRemoved(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	mustCreateClusterNoVpc(t, b, "del-cascade-cluster2")
	mustCreateNodegroup(t, b, "del-cascade-cluster2")

	_, err := b.DeleteCluster("del-cascade-cluster2")
	require.ErrorIs(t, err, eks.ErrAlreadyExists)
	assert.Equal(t, 1, b.NodegroupCount())

	_, err = b.DeleteNodegroup("del-cascade-cluster2", "ng1")
	require.NoError(t, err)

	_, err = b.DeleteCluster("del-cascade-cluster2")
	require.NoError(t, err)
	assert.Equal(t, 0, b.NodegroupCount())
}

func TestUpdateNodegroupVersion_Status_InProgress(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	mustCreateClusterNoVpc(t, b, "ng-inprog-cluster")
	mustCreateNodegroup(t, b, "ng-inprog-cluster")

	upd, err := b.UpdateNodegroupVersion("ng-inprog-cluster", "ng1", "1.33")
	require.NoError(t, err)
	assert.Equal(t, "InProgress", upd.Status)

	require.Eventually(t, func() bool {
		got, descErr := b.DescribeUpdate("ng-inprog-cluster", upd.ID)

		return descErr == nil && got.Status == "Successful"
	}, 2*time.Second, 10*time.Millisecond)
}

func TestNodegroup_UpdateConfig_Create(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "ng-uc-cluster"})

	maxUnavail := int32(2)
	rec := doREST(t, h, http.MethodPost, "/clusters/ng-uc-cluster/node-groups", map[string]any{
		"nodegroupName": "ng1",
		"nodeRole":      "arn:aws:iam::123456789012:role/ng",
		"scalingConfig": map[string]any{"desiredSize": 3, "minSize": 1, "maxSize": 5},
		"subnets":       []string{"subnet-aaa"},
		"updateConfig":  map[string]any{"maxUnavailable": maxUnavail},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	ng := parseResp(t, rec)["nodegroup"].(map[string]any)
	uc, ok := ng["updateConfig"].(map[string]any)
	require.True(t, ok, "updateConfig must be present")
	assert.InDelta(t, float64(2), uc["maxUnavailable"], 0.001)
}

func TestNodegroup_UpdateConfig_Via_UpdateNodegroupConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body       map[string]any
		wantMaxU   *float64
		wantMaxPct *float64
		name       string
	}{
		{
			name: "set_max_unavailable",
			body: map[string]any{
				"updateConfig": map[string]any{"maxUnavailable": 2},
			},
			wantMaxU: &[]float64{2}[0],
		},
		{
			name: "set_max_unavailable_percentage",
			body: map[string]any{
				"updateConfig": map[string]any{"maxUnavailablePercentage": 25},
			},
			wantMaxPct: &[]float64{25}[0],
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestEKSHandler(t)
			doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "ng-uc-upd-" + tt.name})
			doREST(t, h, http.MethodPost, "/clusters/ng-uc-upd-"+tt.name+"/node-groups", map[string]any{
				"nodegroupName": "ng1",
				"nodeRole":      "arn:aws:iam::123456789012:role/ng",
				"subnets":       []string{"subnet-x"},
			})

			path := "/clusters/ng-uc-upd-" + tt.name + "/node-groups/ng1/update-config"
			rec := doREST(t, h, http.MethodPost, path, tt.body)
			require.Equal(t, http.StatusOK, rec.Code)

			desc := doREST(t, h, http.MethodGet, "/clusters/ng-uc-upd-"+tt.name+"/node-groups/ng1", nil)
			ng := parseResp(t, desc)["nodegroup"].(map[string]any)
			uc, ok := ng["updateConfig"].(map[string]any)
			require.True(t, ok, "updateConfig must be present after update")

			if tt.wantMaxU != nil {
				assert.InDelta(t, *tt.wantMaxU, uc["maxUnavailable"], 0.001)
			}

			if tt.wantMaxPct != nil {
				assert.InDelta(t, *tt.wantMaxPct, uc["maxUnavailablePercentage"], 0.001)
			}
		})
	}
}

func TestNodegroup_UpdateConfig_UpdateRecordReachesSuccessful(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "ng-uc-successful"})
	doREST(t, h, http.MethodPost, "/clusters/ng-uc-successful/node-groups", map[string]any{
		"nodegroupName": "ng1",
		"nodeRole":      "arn:aws:iam::123456789012:role/ng",
		"subnets":       []string{"subnet-x"},
	})

	rec := doREST(t, h, http.MethodPost, "/clusters/ng-uc-successful/node-groups/ng1/update-config", map[string]any{
		"updateConfig": map[string]any{"maxUnavailable": 2},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	upd := parseResp(t, rec)["update"].(map[string]any)
	updateID, ok := upd["id"].(string)
	require.True(t, ok, "response must carry the update id")
	assert.Equal(t, "InProgress", upd["status"])

	// The immediate InProgress status is right, but a machine that never
	// advances would pass that assertion forever -- confirm DescribeUpdate
	// eventually reports Successful too.
	require.Eventually(t, func() bool {
		desc := doREST(t, h, http.MethodGet, "/clusters/ng-uc-successful/updates/"+updateID, nil)
		if desc.Code != http.StatusOK {
			return false
		}

		got := parseResp(t, desc)["update"].(map[string]any)

		return got["status"] == "Successful"
	}, 2*time.Second, 10*time.Millisecond)
}

func TestUpdateNodegroupConfig_Labels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup       map[string]any
		update      map[string]any
		name        string
		wantLabel   string
		wantValue   string
		wantMissing string
	}{
		{
			name: "add_label",
			setup: map[string]any{
				"nodegroupName": "ng1",
				"nodeRole":      "arn:aws:iam::123456789012:role/ng",
				"subnets":       []string{"subnet-x"},
				"labels":        map[string]any{"existing": "value"},
			},
			update: map[string]any{
				"labels": map[string]any{
					"addOrUpdateLabels": map[string]any{"env": "prod"},
				},
			},
			wantLabel: "env",
			wantValue: "prod",
		},
		{
			name: "remove_label",
			setup: map[string]any{
				"nodegroupName": "ng1",
				"nodeRole":      "arn:aws:iam::123456789012:role/ng",
				"subnets":       []string{"subnet-x"},
				"labels":        map[string]any{"to-remove": "bye"},
			},
			update: map[string]any{
				"labels": map[string]any{
					"removeLabels": []string{"to-remove"},
				},
			},
			wantMissing: "to-remove",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			clusterName := "ng-lbl-" + tt.name
			h := newTestEKSHandler(t)
			doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": clusterName})
			doREST(t, h, http.MethodPost, "/clusters/"+clusterName+"/node-groups", tt.setup)

			rec := doREST(t, h, http.MethodPost, "/clusters/"+clusterName+"/node-groups/ng1/update-config", tt.update)
			require.Equal(t, http.StatusOK, rec.Code)

			desc := doREST(t, h, http.MethodGet, "/clusters/"+clusterName+"/node-groups/ng1", nil)
			ng := parseResp(t, desc)["nodegroup"].(map[string]any)
			labels, _ := ng["labels"].(map[string]any)

			if tt.wantLabel != "" {
				assert.Equal(t, tt.wantValue, labels[tt.wantLabel])
			}

			if tt.wantMissing != "" {
				_, hasKey := labels[tt.wantMissing]
				assert.False(t, hasKey, "label %q should have been removed", tt.wantMissing)
			}
		})
	}
}

func TestUpdateNodegroupConfig_Taints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      map[string]any
		update     map[string]any
		name       string
		wantTaints int
	}{
		{
			name: "add_taint",
			setup: map[string]any{
				"nodegroupName": "ng1",
				"nodeRole":      "arn:aws:iam::123456789012:role/ng",
				"subnets":       []string{"subnet-x"},
			},
			update: map[string]any{
				"taints": map[string]any{
					"addOrUpdateTaints": []any{
						map[string]any{"key": "dedicated", "value": "gpu", "effect": "NO_SCHEDULE"},
					},
				},
			},
			wantTaints: 1,
		},
		{
			name: "remove_taint",
			setup: map[string]any{
				"nodegroupName": "ng1",
				"nodeRole":      "arn:aws:iam::123456789012:role/ng",
				"subnets":       []string{"subnet-x"},
				"taints": []any{
					map[string]any{"key": "spot", "effect": "NO_SCHEDULE"},
				},
			},
			update: map[string]any{
				"taints": map[string]any{
					"removeTaints": []any{
						map[string]any{"key": "spot", "effect": "NO_SCHEDULE"},
					},
				},
			},
			wantTaints: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			clusterName := "ng-tnt-" + tt.name
			h := newTestEKSHandler(t)
			doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": clusterName})
			doREST(t, h, http.MethodPost, "/clusters/"+clusterName+"/node-groups", tt.setup)

			rec := doREST(t, h, http.MethodPost, "/clusters/"+clusterName+"/node-groups/ng1/update-config", tt.update)
			require.Equal(t, http.StatusOK, rec.Code)

			desc := doREST(t, h, http.MethodGet, "/clusters/"+clusterName+"/node-groups/ng1", nil)
			ng := parseResp(t, desc)["nodegroup"].(map[string]any)
			taints, _ := ng["taints"].([]any)
			assert.Len(t, taints, tt.wantTaints)
		})
	}
}

func TestUpdateNodegroupConfig_Backend_Taints(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)
	_, err := b.CreateCluster("c1", "1.32", "", nil, nil, nil)
	require.NoError(t, err)

	_, err = b.CreateNodegroup("c1", "ng1", "", "", "", "", "", nil, 1, 1, 3,
		eks.NodegroupInput{
			Taints: []eks.NodegroupTaint{
				{Key: "env", Value: "prod", Effect: "NO_SCHEDULE"},
			},
		},
		nil)
	require.NoError(t, err)

	upd := eks.NodegroupConfigUpdate{
		AddOrUpdateTaints: []eks.NodegroupTaint{
			{Key: "env", Value: "staging", Effect: "NO_SCHEDULE"},
			{Key: "spot", Effect: "PREFER_NO_SCHEDULE"},
		},
	}
	ng, err := b.UpdateNodegroupConfig("c1", "ng1", upd)
	require.NoError(t, err)
	assert.Len(t, ng.Taints, 2)

	var envTaint eks.NodegroupTaint
	for _, taint := range ng.Taints {
		if taint.Key == "env" {
			envTaint = taint
		}
	}
	assert.Equal(t, "staging", envTaint.Value, "taint value should be updated")

	// Remove by key+effect
	ng2, err := b.UpdateNodegroupConfig("c1", "ng1", eks.NodegroupConfigUpdate{
		RemoveTaints: []eks.NodegroupTaint{{Key: "spot", Effect: "PREFER_NO_SCHEDULE"}},
	})
	require.NoError(t, err)
	assert.Len(t, ng2.Taints, 1)
}

func TestNodegroup_UpdateConfig_AbsentWhenNotSet(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "ng-no-uc-cluster"})
	doREST(t, h, http.MethodPost, "/clusters/ng-no-uc-cluster/node-groups", map[string]any{
		"nodegroupName": "ng1",
		"nodeRole":      "arn:aws:iam::123456789012:role/ng",
		"subnets":       []string{"subnet-x"},
	})

	desc := doREST(t, h, http.MethodGet, "/clusters/ng-no-uc-cluster/node-groups/ng1", nil)
	ng := parseResp(t, desc)["nodegroup"].(map[string]any)
	_, hasUC := ng["updateConfig"]
	assert.False(t, hasUC, "updateConfig should be absent when not set at creation")
}

func TestSortedListNodegroups(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)

	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "c1"})

	for _, ng := range []string{"zzz", "aaa", "mmm"} {
		doREST(t, h, http.MethodPost, "/clusters/c1/node-groups", map[string]any{
			"nodegroupName": ng,
			"nodeRole":      "arn:aws:iam::123456789012:role/ng",
			"subnets":       []string{"subnet-abc"},
		})
	}

	rec := doREST(t, h, http.MethodGet, "/clusters/c1/node-groups", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	resp := parseResp(t, rec)
	raw, err := json.Marshal(resp["nodegroups"])
	require.NoError(t, err)

	var names []string
	require.NoError(t, json.Unmarshal(raw, &names))

	require.Equal(t, []string{"aaa", "mmm", "zzz"}, names)
}

func TestCreateNodegroup_SubnetsRequired(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "c1"})

	rec := doREST(t, h, http.MethodPost, "/clusters/c1/node-groups", map[string]any{
		"nodegroupName": "ng-no-subnets",
		"nodeRole":      "arn:aws:iam::123456789012:role/ng",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreateNodegroup_SubnetsRequired_EmptySlice_Rejected(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "c1"})

	rec := doREST(t, h, http.MethodPost, "/clusters/c1/node-groups", map[string]any{
		"nodegroupName": "ng-empty-subnets",
		"nodeRole":      "arn:aws:iam::123456789012:role/ng",
		"subnets":       []string{},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreateNodegroup_SubnetsNotRequired_WithLaunchTemplate(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "c1"})

	// subnets omitted but launchTemplate present — must succeed.
	rec := doREST(t, h, http.MethodPost, "/clusters/c1/node-groups", map[string]any{
		"nodegroupName": "ng-lt-nosubnets",
		"nodeRole":      "arn:aws:iam::123456789012:role/ng",
		"launchTemplate": map[string]any{
			"id":      "lt-0123456789abcdef0",
			"version": "$Default",
		},
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestCreateNodegroup_NodeRoleRequired(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "c1"})

	rec := doREST(t, h, http.MethodPost, "/clusters/c1/node-groups", map[string]any{
		"nodegroupName": "ng-no-role",
		"subnets":       []string{"subnet-abc"},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestNodegroupHasHealthField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "nodegroup_has_health_with_issues"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestEKSHandler(t)
			doREST(t, h, http.MethodPost, "/clusters", map[string]any{
				"name":    "cl",
				"version": "1.32",
				"roleArn": "arn:aws:iam::123456789012:role/role",
			})

			rec := doREST(t, h, http.MethodPost, "/clusters/cl/node-groups", map[string]any{
				"nodegroupName": "ng1",
				"nodeRole":      "arn:aws:iam::123456789012:role/ng-role",
				"scalingConfig": map[string]any{"desiredSize": 1, "minSize": 1, "maxSize": 3},
				"subnets":       []string{"subnet-abc"},
			})
			require.Equal(t, http.StatusOK, rec.Code, tc.name)

			resp := parseResp(t, rec)
			ng, ok := resp["nodegroup"].(map[string]any)
			require.True(t, ok, tc.name)

			health, ok := ng["health"].(map[string]any)
			require.True(t, ok, tc.name)
			_, ok = health["issues"]
			assert.True(t, ok, tc.name)
		})
	}
}

func TestEKSNodegroupCRUD(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ops  func(t *testing.T, h *eks.Handler)
		name string
	}{
		{
			name: "create_nodegroup",
			ops: func(t *testing.T, h *eks.Handler) {
				t.Helper()
				doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "my-cluster"})
				rec := doREST(t, h, http.MethodPost, "/clusters/my-cluster/node-groups", map[string]any{
					"nodegroupName": "my-ng",
					"nodeRole":      "arn:aws:iam::123456789012:role/ng-role",
					"subnets":       []string{"subnet-abc"},
					"scalingConfig": map[string]any{"desiredSize": 2, "minSize": 1, "maxSize": 5},
				})
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseResp(t, rec)
				ng, ok := resp["nodegroup"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "my-ng", ng["nodegroupName"])
				assert.Equal(t, "CREATING", ng["status"])
			},
		},
		{
			name: "describe_nodegroup",
			ops: func(t *testing.T, h *eks.Handler) {
				t.Helper()
				doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "my-cluster"})
				doREST(t, h, http.MethodPost, "/clusters/my-cluster/node-groups", map[string]any{
					"nodegroupName": "my-ng",
					"nodeRole":      "arn:aws:iam::123456789012:role/ng",
					"subnets":       []string{"subnet-abc"},
					"scalingConfig": map[string]any{},
				})
				rec := doREST(t, h, http.MethodGet, "/clusters/my-cluster/node-groups/my-ng", nil)
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseResp(t, rec)
				ng, ok := resp["nodegroup"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "my-ng", ng["nodegroupName"])
			},
		},
		{
			name: "list_nodegroups",
			ops: func(t *testing.T, h *eks.Handler) {
				t.Helper()
				doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "my-cluster"})
				doREST(t, h, http.MethodPost, "/clusters/my-cluster/node-groups", map[string]any{
					"nodegroupName": "ng-1",
					"nodeRole":      "arn:aws:iam::123456789012:role/ng",
					"subnets":       []string{"subnet-abc"},
					"scalingConfig": map[string]any{},
				})
				doREST(t, h, http.MethodPost, "/clusters/my-cluster/node-groups", map[string]any{
					"nodegroupName": "ng-2",
					"nodeRole":      "arn:aws:iam::123456789012:role/ng",
					"subnets":       []string{"subnet-abc"},
					"scalingConfig": map[string]any{},
				})
				rec := doREST(t, h, http.MethodGet, "/clusters/my-cluster/node-groups", nil)
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseResp(t, rec)
				names, ok := resp["nodegroups"].([]any)
				require.True(t, ok)
				assert.Len(t, names, 2)
			},
		},
		{
			name: "delete_nodegroup",
			ops: func(t *testing.T, h *eks.Handler) {
				t.Helper()
				doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "my-cluster"})
				doREST(t, h, http.MethodPost, "/clusters/my-cluster/node-groups", map[string]any{
					"nodegroupName": "to-delete",
					"nodeRole":      "arn:aws:iam::123456789012:role/ng",
					"subnets":       []string{"subnet-abc"},
					"scalingConfig": map[string]any{},
				})
				rec := doREST(t, h, http.MethodDelete, "/clusters/my-cluster/node-groups/to-delete", nil)
				assert.Equal(t, http.StatusOK, rec.Code)
				rec2 := doREST(t, h, http.MethodGet, "/clusters/my-cluster/node-groups/to-delete", nil)
				assert.Equal(t, http.StatusNotFound, rec2.Code)
			},
		},
		{
			// CreateNodegroup's own deserializer (eks@v1.90.4
			// deserializers.go) has no ResourceNotFoundException case -- an
			// unknown cluster is InvalidParameterException (400).
			name: "nodegroup_cluster_not_found",
			ops: func(t *testing.T, h *eks.Handler) {
				t.Helper()
				rec := doREST(t, h, http.MethodPost, "/clusters/nonexistent/node-groups", map[string]any{
					"nodegroupName": "ng",
					"nodeRole":      "arn:aws:iam::123456789012:role/ng",
					"subnets":       []string{"subnet-abc"},
					"scalingConfig": map[string]any{},
				})
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

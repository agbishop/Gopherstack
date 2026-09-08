package eks_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/services/eks"
)

func TestCluster_Tags_RoundTrip(t *testing.T) {
	t.Parallel()

	h, _ := newHandlerAndBackend(t)
	rec := doREST(t, h, http.MethodPost, "/clusters", map[string]any{
		"name":    "tagged-cluster",
		"version": "1.32",
		"tags":    map[string]string{"env": "prod", "team": "infra"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	cluster := parseClusterResp(t, rec)
	tags, ok := cluster["tags"].(map[string]any)
	require.True(t, ok, "cluster response must include 'tags' field")
	assert.Equal(t, "prod", tags["env"])
	assert.Equal(t, "infra", tags["team"])
}

func TestCluster_Tags_EmptyMap_When_No_Tags(t *testing.T) {
	t.Parallel()

	h, _ := newHandlerAndBackend(t)
	rec := doREST(t, h, http.MethodPost, "/clusters", map[string]any{
		"name": "notags-cluster",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	cluster := parseClusterResp(t, rec)
	tags, ok := cluster["tags"]
	require.True(t, ok, "cluster response must always include 'tags' field")
	// Should be an empty map, not nil/absent.
	asMap, isMap := tags.(map[string]any)
	require.True(t, isMap, "tags must be a map even when empty")
	assert.Empty(t, asMap, "tags map must be empty when no tags set")
}

func TestCluster_Tags_Persist_On_Describe(t *testing.T) {
	t.Parallel()

	h, _ := newHandlerAndBackend(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{
		"name": "persist-tags",
		"tags": map[string]string{"cost-center": "123"},
	})

	rec := doREST(t, h, http.MethodGet, "/clusters/persist-tags", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	cluster := parseClusterResp(t, rec)
	tags := cluster["tags"].(map[string]any)
	assert.Equal(t, "123", tags["cost-center"])
}

func TestCluster_TagResource_Reflected_In_Describe(t *testing.T) {
	t.Parallel()

	h, b := newHandlerAndBackend(t)
	c := mustCreateClusterNoVpc(t, b, "tag-reflect")

	// Add tag via TagResource.
	rec := doREST(t, h, http.MethodPost, "/tags/"+c.ARN, map[string]any{
		"tags": map[string]string{"added": "yes"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// Tags from TagResource appear in DescribeCluster.
	rec2 := doREST(t, h, http.MethodGet, "/clusters/tag-reflect", nil)
	cluster := parseClusterResp(t, rec2)
	tags := cluster["tags"].(map[string]any)
	assert.Equal(t, "yes", tags["added"])
}

func TestNodegroup_Tags_RoundTrip(t *testing.T) {
	t.Parallel()

	h, b := newHandlerAndBackend(t)
	mustCreateClusterNoVpc(t, b, "ng-tags-cluster")

	rec := doREST(t, h, http.MethodPost, "/clusters/ng-tags-cluster/node-groups", map[string]any{
		"nodegroupName": "ng-tagged",
		"nodeRole":      "arn:aws:iam::123:role/ng",
		"subnets":       []string{"subnet-aaa"},
		"tags":          map[string]string{"env": "staging", "app": "web"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	ng := parseNodegroupResp(t, rec)
	tags, ok := ng["tags"].(map[string]any)
	require.True(t, ok, "nodegroup response must include 'tags' field")
	assert.Equal(t, "staging", tags["env"])
	assert.Equal(t, "web", tags["app"])
}

func TestNodegroup_Tags_EmptyMap_When_No_Tags(t *testing.T) {
	t.Parallel()

	h, b := newHandlerAndBackend(t)
	mustCreateClusterNoVpc(t, b, "ng-notags-cluster")

	rec := doREST(t, h, http.MethodPost, "/clusters/ng-notags-cluster/node-groups", map[string]any{
		"nodegroupName": "ng-no-tags",
		"nodeRole":      "arn:aws:iam::123:role/ng",
		"subnets":       []string{"subnet-aaa"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	ng := parseNodegroupResp(t, rec)
	tagsRaw, ok := ng["tags"]
	require.True(t, ok, "nodegroup response must always include 'tags' field")
	asMap, isMap := tagsRaw.(map[string]any)
	require.True(t, isMap)
	assert.Empty(t, asMap)
}

func TestNodegroup_Tags_Persist_On_Describe(t *testing.T) {
	t.Parallel()

	h, b := newHandlerAndBackend(t)
	mustCreateClusterNoVpc(t, b, "ng-persist-cluster")

	doREST(t, h, http.MethodPost, "/clusters/ng-persist-cluster/node-groups", map[string]any{
		"nodegroupName": "ng-persist",
		"nodeRole":      "arn:aws:iam::123:role/ng",
		"subnets":       []string{"subnet-aaa"},
		"tags":          map[string]string{"key1": "val1"},
	})

	rec := doREST(t, h, http.MethodGet, "/clusters/ng-persist-cluster/node-groups/ng-persist", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	ng := parseNodegroupResp(t, rec)
	tags := ng["tags"].(map[string]any)
	assert.Equal(t, "val1", tags["key1"])
}

func TestAddon_Tags_RoundTrip(t *testing.T) {
	t.Parallel()

	h, b := newHandlerAndBackend(t)
	mustCreateClusterNoVpc(t, b, "addon-tags-cluster")

	rec := doREST(t, h, http.MethodPost, "/clusters/addon-tags-cluster/addons", map[string]any{
		"addonName": "vpc-cni",
		"tags":      map[string]string{"managed": "true"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	m := parseResp(t, rec)
	addon, ok := m["addon"].(map[string]any)
	require.True(t, ok)
	tags, ok := addon["tags"].(map[string]any)
	require.True(t, ok, "addon response must include 'tags' field")
	assert.Equal(t, "true", tags["managed"])
}

func TestAddon_Tags_EmptyMap_When_No_Tags(t *testing.T) {
	t.Parallel()

	h, b := newHandlerAndBackend(t)
	mustCreateClusterNoVpc(t, b, "addon-notags-cluster")

	rec := doREST(t, h, http.MethodPost, "/clusters/addon-notags-cluster/addons", map[string]any{
		"addonName": "coredns",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	m := parseResp(t, rec)
	addon := m["addon"].(map[string]any)
	tagsRaw, ok := addon["tags"]
	require.True(t, ok, "addon must always emit 'tags' field")
	asMap, isMap := tagsRaw.(map[string]any)
	require.True(t, isMap)
	assert.Empty(t, asMap)
}

func TestAddon_Tags_Persist_On_Describe(t *testing.T) {
	t.Parallel()

	h, b := newHandlerAndBackend(t)
	mustCreateClusterNoVpc(t, b, "addon-desc-cluster")

	doREST(t, h, http.MethodPost, "/clusters/addon-desc-cluster/addons", map[string]any{
		"addonName": "aws-ebs-csi-driver",
		"tags":      map[string]string{"billing": "infra"},
	})

	rec := doREST(t, h, http.MethodGet, "/clusters/addon-desc-cluster/addons/aws-ebs-csi-driver", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	m := parseResp(t, rec)
	addon := m["addon"].(map[string]any)
	tags := addon["tags"].(map[string]any)
	assert.Equal(t, "infra", tags["billing"])
}

func TestFargateProfile_Tags_RoundTrip(t *testing.T) {
	t.Parallel()

	h, b := newHandlerAndBackend(t)
	mustCreateClusterNoVpc(t, b, "fargate-tags-cluster")

	rec := doREST(t, h, http.MethodPost, "/clusters/fargate-tags-cluster/fargate-profiles", map[string]any{
		"fargateProfileName":  "fp-tagged",
		"podExecutionRoleArn": "arn:aws:iam::123:role/fargate",
		"selectors":           []map[string]any{{"namespace": "default"}},
		"tags":                map[string]string{"team": "platform"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	m := parseResp(t, rec)
	fp, ok := m["fargateProfile"].(map[string]any)
	require.True(t, ok)
	tags, ok := fp["tags"].(map[string]any)
	require.True(t, ok, "fargateProfile response must include 'tags' field")
	assert.Equal(t, "platform", tags["team"])
}

func TestFargateProfile_Tags_EmptyMap_When_No_Tags(t *testing.T) {
	t.Parallel()

	h, b := newHandlerAndBackend(t)
	mustCreateClusterNoVpc(t, b, "fargate-notags-cluster")

	rec := doREST(t, h, http.MethodPost, "/clusters/fargate-notags-cluster/fargate-profiles", map[string]any{
		"fargateProfileName":  "fp-notags",
		"podExecutionRoleArn": "arn:aws:iam::123:role/fargate",
		"selectors":           []map[string]any{{"namespace": "default"}},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	m := parseResp(t, rec)
	fp := m["fargateProfile"].(map[string]any)
	tagsRaw, ok := fp["tags"]
	require.True(t, ok, "fargateProfile must always emit 'tags' field")
	asMap, isMap := tagsRaw.(map[string]any)
	require.True(t, isMap)
	assert.Empty(t, asMap)
}

func TestPodIdentity_Tags_RoundTrip(t *testing.T) {
	t.Parallel()

	h, b := newHandlerAndBackend(t)
	mustCreateClusterNoVpc(t, b, "pi-tags-cluster")

	rec := doREST(t, h, http.MethodPost, "/clusters/pi-tags-cluster/pod-identity-associations", map[string]any{
		"namespace":      "kube-system",
		"serviceAccount": "aws-node",
		"roleArn":        "arn:aws:iam::123:role/pi",
		"tags":           map[string]string{"owner": "ops"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	m := parseResp(t, rec)
	assoc, ok := m["association"].(map[string]any)
	require.True(t, ok)
	tags, ok := assoc["tags"].(map[string]any)
	require.True(t, ok, "association response must include 'tags' field")
	assert.Equal(t, "ops", tags["owner"])
}

func TestPodIdentity_Tags_EmptyMap_When_No_Tags(t *testing.T) {
	t.Parallel()

	h, b := newHandlerAndBackend(t)
	mustCreateClusterNoVpc(t, b, "pi-notags-cluster")

	rec := doREST(t, h, http.MethodPost, "/clusters/pi-notags-cluster/pod-identity-associations", map[string]any{
		"namespace":      "default",
		"serviceAccount": "default",
		"roleArn":        "arn:aws:iam::123:role/pi",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	m := parseResp(t, rec)
	assoc := m["association"].(map[string]any)
	tagsRaw, ok := assoc["tags"]
	require.True(t, ok, "association must always emit 'tags' field")
	asMap, isMap := tagsRaw.(map[string]any)
	require.True(t, isMap)
	assert.Empty(t, asMap)
}

func TestAccessEntry_Tags_RoundTrip(t *testing.T) {
	t.Parallel()

	h, b := newHandlerAndBackend(t)
	mustCreateClusterNoVpc(t, b, "ae-tags-cluster")

	rec := doREST(t, h, http.MethodPost, "/clusters/ae-tags-cluster/access-entries", map[string]any{
		"principalArn": "arn:aws:iam::123456789012:role/DevRole",
		"type":         "STANDARD",
		"tags":         map[string]string{"role": "dev"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	m := parseResp(t, rec)
	ae, ok := m["accessEntry"].(map[string]any)
	require.True(t, ok)
	tags, ok := ae["tags"].(map[string]any)
	require.True(t, ok, "accessEntry response must include 'tags' field")
	assert.Equal(t, "dev", tags["role"])
}

func TestAccessEntry_Tags_EmptyMap_When_No_Tags(t *testing.T) {
	t.Parallel()

	h, b := newHandlerAndBackend(t)
	mustCreateClusterNoVpc(t, b, "ae-notags-cluster")

	rec := doREST(t, h, http.MethodPost, "/clusters/ae-notags-cluster/access-entries", map[string]any{
		"principalArn": "arn:aws:iam::123456789012:role/OpRole",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	m := parseResp(t, rec)
	ae := m["accessEntry"].(map[string]any)
	tagsRaw, ok := ae["tags"]
	require.True(t, ok, "accessEntry must always emit 'tags' field")
	asMap, isMap := tagsRaw.(map[string]any)
	require.True(t, isMap)
	assert.Empty(t, asMap)
}

func TestAccessEntry_Tags_Persist_On_Describe(t *testing.T) {
	t.Parallel()

	h, b := newHandlerAndBackend(t)
	mustCreateClusterNoVpc(t, b, "ae-persist-cluster")

	doREST(t, h, http.MethodPost, "/clusters/ae-persist-cluster/access-entries", map[string]any{
		"principalArn": "arn:aws:iam::123456789012:role/PersistRole",
		"tags":         map[string]string{"persisted": "yes"},
	})

	rec := doREST(t, h, http.MethodGet,
		"/clusters/ae-persist-cluster/access-entries/arn:aws:iam::123456789012:role/PersistRole", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	m := parseResp(t, rec)
	ae := m["accessEntry"].(map[string]any)
	tags := ae["tags"].(map[string]any)
	assert.Equal(t, "yes", tags["persisted"])
}

func TestTagResource(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)

	rec := doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "c1"})
	require.Equal(t, http.StatusOK, rec.Code)

	clusterARN := "arn:aws:eks:" + config.DefaultRegion + ":123456789012:cluster/c1"

	rec = doREST(t, h, http.MethodPost, "/tags/"+clusterARN, map[string]any{
		"tags": map[string]any{"env": "prod"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	tags, err := h.Backend.ListTagsForResource(clusterARN)
	require.NoError(t, err)
	assert.Equal(t, "prod", tags["env"])

	doREST(t, h, http.MethodDelete, "/tags/"+clusterARN+"?tagKeys=env", nil)

	tags2, err := h.Backend.ListTagsForResource(clusterARN)
	require.NoError(t, err)
	assert.Empty(t, tags2["env"])
}

func TestTagResourceNotFound(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)

	rec := doREST(t, h, http.MethodPost, "/tags/arn:aws:eks:us-east-1:123:cluster/nonexistent",
		map[string]any{"tags": map[string]any{"k": "v"}})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestTagResourceOnAddon(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)

	_, err := b.CreateCluster("c1", "1.32", "", nil, nil, nil)
	require.NoError(t, err)

	addon, err := b.CreateAddon("c1", "coredns", "v1.0", "", "", "", "", nil, nil)
	require.NoError(t, err)

	err = b.TagResource(addon.ARN, map[string]string{"k": "v"})
	require.NoError(t, err)

	t2, err := b.ListTagsForResource(addon.ARN)
	require.NoError(t, err)
	assert.Equal(t, "v", t2["k"])
}

func TestTagResourceOnFargateProfile(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)

	_, err := b.CreateCluster("c1", "1.32", "", nil, nil, nil)
	require.NoError(t, err)

	fp, err := b.CreateFargateProfile("c1", "fp1", "arn:aws:iam::123:role/r", nil, nil, nil)
	require.NoError(t, err)

	err = b.TagResource(fp.ARN, map[string]string{"env": "staging"})
	require.NoError(t, err)

	t2, err := b.ListTagsForResource(fp.ARN)
	require.NoError(t, err)
	assert.Equal(t, "staging", t2["env"])
}

func TestTagResourceOnPodIdentity(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)

	_, err := b.CreateCluster("c1", "1.32", "", nil, nil, nil)
	require.NoError(t, err)

	assoc, err := b.CreatePodIdentityAssociation(
		"c1", "default", "my-sa", "arn:aws:iam::123:role/r", nil, eks.PodIdentityAssociationInput{},
	)
	require.NoError(t, err)

	err = b.TagResource(assoc.ARN, map[string]string{"a": "b"})
	require.NoError(t, err)

	t2, err := b.ListTagsForResource(assoc.ARN)
	require.NoError(t, err)
	assert.Equal(t, "b", t2["a"])
}

// TestTagResourceOnIdentityProviderConfig verifies the generic ARN-based
// TagResource/ListTagsForResource/TaggedResources chain covers identity
// provider configs. Real EKS supports tags on OIDC IDP configs
// (botocore eks/2017-11-01: AssociateIdentityProviderConfigRequest.tags,
// OidcIdentityProviderConfig.tags), and gopherstack decodes/renders them on
// create/describe, but the ARN-keyed lookup used by TagResource et al only
// searched clusters, nodegroups, access entries, addons, fargate profiles,
// pod identity associations, capabilities, and subscriptions.
func TestTagResourceOnIdentityProviderConfig(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)

	_, err := b.CreateCluster("c1", "1.32", "", nil, nil, nil)
	require.NoError(t, err)

	cfg, err := b.AssociateIdentityProviderConfig(
		"c1", "oidc", "idp1",
		map[string]string{"issuerUrl": "https://issuer.example.com", "clientId": "client1"},
		nil, nil,
	)
	require.NoError(t, err)

	err = b.TagResource(cfg.ARN, map[string]string{"env": "prod"})
	require.NoError(t, err, "TagResource must find identity provider config by ARN")

	got, err := b.ListTagsForResource(cfg.ARN)
	require.NoError(t, err)
	assert.Equal(t, "prod", got["env"])

	found := false

	for _, e := range b.TaggedResources() {
		if e.ARN == cfg.ARN {
			found = true

			assert.Equal(t, "prod", e.Tags["env"])
		}
	}

	assert.True(t, found, "TaggedResources must include the tagged identity provider config")
}

func TestTagResource_KeyTooLong_Rejected(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "c1"})

	longKey := strings.Repeat("k", 129)
	rec := doREST(t, h, http.MethodPost, "/tags/arn:aws:eks:us-east-1:123456789012:cluster/c1",
		map[string]any{"tags": map[string]string{longKey: "v"}})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestTagResource_KeyEmpty_Rejected(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "c1"})

	rec := doREST(t, h, http.MethodPost, "/tags/arn:aws:eks:us-east-1:123456789012:cluster/c1",
		map[string]any{"tags": map[string]string{"": "v"}})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestTagResource_ValueTooLong_Rejected(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "c1"})

	longVal := strings.Repeat("v", 257)
	rec := doREST(t, h, http.MethodPost, "/tags/arn:aws:eks:us-east-1:123456789012:cluster/c1",
		map[string]any{"tags": map[string]string{"k": longVal}})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestTagResource_ValidBoundary_Accepted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		key   string
		value string
		name  string
	}{
		{name: "key_128", key: strings.Repeat("k", 128), value: "v"},
		{name: "value_256", key: "k", value: strings.Repeat("v", 256)},
		{name: "value_empty", key: "k", value: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestEKSHandler(t)
			doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "c1"})

			rec := doREST(t, h, http.MethodPost, "/tags/arn:aws:eks:us-east-1:123456789012:cluster/c1",
				map[string]any{"tags": map[string]string{tt.key: tt.value}})
			assert.Equal(t, http.StatusOK, rec.Code)
		})
	}
}

func TestTagResource_MaxTags_Enforced(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "c1"})

	clusterARN := "arn:aws:eks:us-east-1:123456789012:cluster/c1"

	// Add 50 tags in batches.
	for i := range 5 {
		batch := map[string]string{}
		for j := range 10 {
			batch[strings.Repeat("a", 1)+string(rune('a'+i))+string(rune('0'+j))] = "v"
		}
		rec := doREST(t, h, http.MethodPost, "/tags/"+clusterARN,
			map[string]any{"tags": batch})
		require.Equal(t, http.StatusOK, rec.Code)
	}

	// Adding one more tag must fail.
	rec := doREST(t, h, http.MethodPost, "/tags/"+clusterARN,
		map[string]any{"tags": map[string]string{"extra": "v"}})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreateCluster_InitialTagKeyTooLong_Rejected(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	longKey := strings.Repeat("k", 129)
	rec := doREST(t, h, http.MethodPost, "/clusters", map[string]any{
		"name": "bad-tag-cluster",
		"tags": map[string]string{longKey: "v"},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreateCluster_InitialTagValueTooLong_Rejected(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	longVal := strings.Repeat("v", 257)
	rec := doREST(t, h, http.MethodPost, "/clusters", map[string]any{
		"name": "bad-tag-cluster",
		"tags": map[string]string{"k": longVal},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreateCluster_ValidTags_Accepted(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	rec := doREST(t, h, http.MethodPost, "/clusters", map[string]any{
		"name": "ok-tag-cluster",
		"tags": map[string]string{"Env": "prod", "Team": "platform"},
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestCreateNodegroup_InitialTagKeyTooLong_Rejected(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "c1"})

	longKey := strings.Repeat("k", 129)
	rec := doREST(t, h, http.MethodPost, "/clusters/c1/node-groups", map[string]any{
		"nodegroupName": "ng-bad-tag",
		"nodeRole":      "arn:aws:iam::123456789012:role/ng",
		"subnets":       []string{"subnet-abc"},
		"tags":          map[string]string{longKey: "v"},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreateNodegroup_ValidTags_Accepted(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "c1"})

	rec := doREST(t, h, http.MethodPost, "/clusters/c1/node-groups", map[string]any{
		"nodegroupName": "ng-ok-tag",
		"nodeRole":      "arn:aws:iam::123456789012:role/ng",
		"subnets":       []string{"subnet-abc"},
		"tags":          map[string]string{"Env": "prod"},
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestEKSTagging(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ops  func(t *testing.T, h *eks.Handler)
		name string
	}{
		{
			name: "tag_and_list_cluster",
			ops: func(t *testing.T, h *eks.Handler) {
				t.Helper()
				rec := doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "tagged-cluster"})
				require.Equal(t, http.StatusOK, rec.Code)
				resp := parseResp(t, rec)
				cluster := resp["cluster"].(map[string]any)
				clusterARN := cluster["arn"].(string)

				tagRec := doREST(t, h, http.MethodPost, "/tags/"+clusterARN, map[string]any{
					"tags": map[string]string{"Project": "test"},
				})
				assert.Equal(t, http.StatusOK, tagRec.Code)

				listRec := doREST(t, h, http.MethodGet, "/tags/"+clusterARN, nil)
				assert.Equal(t, http.StatusOK, listRec.Code)
				listResp := parseResp(t, listRec)
				tagsMap, ok := listResp["tags"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "test", tagsMap["Project"])
			},
		},
		{
			name: "list_tags_not_found",
			ops: func(t *testing.T, h *eks.Handler) {
				t.Helper()
				rec := doREST(t, h, http.MethodGet, "/tags/arn:aws:eks:us-east-1:123456789012:cluster/missing", nil)
				assert.Equal(t, http.StatusNotFound, rec.Code)
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

func TestEKSUntagResource(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	rec := doREST(t, h, http.MethodPost, "/clusters", map[string]any{
		"name": "tag-cluster",
		"tags": map[string]string{"Env": "test", "Project": "demo"},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	resp := parseResp(t, rec)
	clusterARN := resp["cluster"].(map[string]any)["arn"].(string)

	// Remove the "Env" tag via query param
	e := echo.New()
	req := httptest.NewRequest(http.MethodDelete, "/tags/"+clusterARN+"?tagKeys=Env", nil)
	untagRec := httptest.NewRecorder()
	c := e.NewContext(req, untagRec)
	err := h.Handler()(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, untagRec.Code)

	// Verify "Env" is gone but "Project" remains
	listRec := doREST(t, h, http.MethodGet, "/tags/"+clusterARN, nil)
	assert.Equal(t, http.StatusOK, listRec.Code)
	listResp := parseResp(t, listRec)
	tagsMap, ok := listResp["tags"].(map[string]any)
	require.True(t, ok)
	assert.NotContains(t, tagsMap, "Env")
}

func TestEKSTagNodegroup(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "my-cluster"})
	ngRec := doREST(t, h, http.MethodPost, "/clusters/my-cluster/node-groups", map[string]any{
		"nodegroupName": "my-ng",
		"nodeRole":      "arn:aws:iam::123456789012:role/ng",
		"subnets":       []string{"subnet-abc"},
		"scalingConfig": map[string]any{},
	})
	require.Equal(t, http.StatusOK, ngRec.Code)
	ngResp := parseResp(t, ngRec)
	ngARN := ngResp["nodegroup"].(map[string]any)["nodegroupArn"].(string)

	tagRec := doREST(t, h, http.MethodPost, "/tags/"+ngARN, map[string]any{
		"tags": map[string]string{"Tier": "worker"},
	})
	assert.Equal(t, http.StatusOK, tagRec.Code)

	listRec := doREST(t, h, http.MethodGet, "/tags/"+ngARN, nil)
	assert.Equal(t, http.StatusOK, listRec.Code)
	listResp := parseResp(t, listRec)
	tagsMap, ok := listResp["tags"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "worker", tagsMap["Tier"])
}

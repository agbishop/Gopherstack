package eks

import (
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// addonPodIdentityNamespace is the Kubernetes namespace EKS installs
// AWS-managed add-ons into by default (vpc-cni, coredns, kube-proxy, etc).
// This backend does not track a per-addon NamespaceConfig override, so
// addon-owned pod identity associations always land here.
const addonPodIdentityNamespace = "kube-system"

// addonTransitionDelay is the async delay before a CREATING addon reaches ACTIVE.
const addonTransitionDelay = 100 * time.Millisecond

const addonVPCCNI = "vpc-cni"

// defaultAddonVersion returns a realistic default addon version for well-known addons.
func defaultAddonVersion(addonName string) string {
	switch addonName {
	case addonVPCCNI:
		return "v1.18.5-eksbuild.1"
	case "coredns":
		return "v1.11.4-eksbuild.2"
	case "kube-proxy":
		return "v1.32.0-eksbuild.1"
	case "aws-ebs-csi-driver":
		return "v1.37.0-eksbuild.1"
	case "aws-efs-csi-driver":
		return "v2.1.0-eksbuild.1"
	default:
		return "v1.16.1-eksbuild.1"
	}
}

const (
	resolveConflictsOverwrite = "OVERWRITE"
	resolveConflictsNone      = "NONE"
	resolveConflictsPreserve  = "PRESERVE"
)

// isValidResolveConflicts reports whether s is an accepted resolveConflicts value.
func isValidResolveConflicts(s string) bool {
	return s == resolveConflictsOverwrite || s == resolveConflictsNone || s == resolveConflictsPreserve
}

// CreateAddon creates a new managed add-on in a cluster.
func (b *InMemoryBackend) CreateAddon(
	clusterName, addonName, addonVersion, serviceAccountRoleARN, configuration, resolveConflicts string,
	kv map[string]string,
) (*Addon, error) {
	b.mu.Lock("CreateAddon")
	defer b.mu.Unlock()

	if _, ok := b.clusters.Get(clusterName); !ok {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrNotFound, clusterName)
	}

	if _, ok := b.addons.Get(addonKey(clusterName, addonName)); ok {
		return nil, fmt.Errorf("%w: addon %s already exists in cluster %s", ErrAlreadyExists, addonName, clusterName)
	}

	if resolveConflicts != "" && !isValidResolveConflicts(resolveConflicts) {
		return nil, fmt.Errorf(
			"%w: resolveConflicts %q must be one of OVERWRITE, NONE, PRESERVE",
			ErrValidation, resolveConflicts,
		)
	}

	addonARN := arn.Build(
		"eks",
		b.region,
		b.accountID,
		"addon/"+clusterName+"/"+addonName+"/"+stableID(clusterName+"/"+addonName),
	)

	t := tags.New("eks.addon." + clusterName + "." + addonName + ".tags")
	if len(kv) > 0 {
		t.Merge(kv)
	}

	if addonVersion == "" {
		addonVersion = defaultAddonVersion(addonName)
	}

	addon := &Addon{
		ClusterName:        clusterName,
		AddonName:          addonName,
		ARN:                addonARN,
		AddonVersion:       addonVersion,
		MarketplaceVersion: addonVersion,
		Health: &AddonHealth{
			Issues: []map[string]string{},
		},
		ServiceAccountRoleARN: serviceAccountRoleARN,
		Status:                statusCreating,
		CreatedAt:             time.Now().UTC(),
		Tags:                  t,
		Configuration:         configuration,
		ResolveConflicts:      resolveConflicts,
	}
	b.addons.Put(addon)

	b.work.After("AddonTransition", addonTransitionDelay, func() {
		b.mu.Lock("CreateAddon-async")
		defer b.mu.Unlock()

		if a, ok := b.addons.Get(addonKey(clusterName, addonName)); ok && a.Status == statusCreating {
			a.Status = statusActive
		}
	})

	cp := *addon

	return &cp, nil
}

// DeleteAddon removes an add-on from a cluster.
func (b *InMemoryBackend) DeleteAddon(clusterName, addonName string) (*Addon, error) {
	b.mu.Lock("DeleteAddon")
	defer b.mu.Unlock()

	if _, ok := b.clusters.Get(clusterName); !ok {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrNotFound, clusterName)
	}

	addon, ok := b.addons.Get(addonKey(clusterName, addonName))
	if !ok {
		return nil, fmt.Errorf("%w: addon %s not found in cluster %s", ErrNotFound, addonName, clusterName)
	}

	cp := *addon
	if addon.Tags != nil {
		addon.Tags.Close()
	}

	b.addons.Delete(addonKey(clusterName, addonName))

	cp.Status = statusDeleting

	return &cp, nil
}

// DescribeAddon returns an add-on by cluster and add-on name.
func (b *InMemoryBackend) DescribeAddon(clusterName, addonName string) (*Addon, error) {
	b.mu.RLock("DescribeAddon")
	defer b.mu.RUnlock()

	if _, ok := b.clusters.Get(clusterName); !ok {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrNotFound, clusterName)
	}

	addon, ok := b.addons.Get(addonKey(clusterName, addonName))
	if !ok {
		return nil, fmt.Errorf("%w: addon %s not found in cluster %s", ErrNotFound, addonName, clusterName)
	}

	cp := *addon

	return &cp, nil
}

// ListAddons returns all add-on names in a cluster sorted alphabetically.
func (b *InMemoryBackend) ListAddons(clusterName string) ([]string, error) {
	b.mu.RLock("ListAddons")
	defer b.mu.RUnlock()

	if _, ok := b.clusters.Get(clusterName); !ok {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrNotFound, clusterName)
	}

	items := b.addonsByCluster.Get(clusterName)
	names := make([]string, len(items))

	for i, a := range items {
		names[i] = a.AddonName
	}

	slices.Sort(names)

	return names, nil
}

// UpdateAddon updates an existing add-on.
//
// podIdentityAssociations implements UpdateAddonInput's tri-state field: nil
// means "left blank, no change"; a non-nil pointer to an empty slice deletes
// every association owned by the add-on; a non-nil pointer to a populated
// slice replaces them.
func (b *InMemoryBackend) UpdateAddon(
	clusterName, addonName, addonVersion, serviceAccountRoleARN, configuration, resolveConflicts string,
	podIdentityAssociations *[]PodIdentityAssociationSpec,
) (*Addon, error) {
	b.mu.Lock("UpdateAddon")
	defer b.mu.Unlock()

	if _, ok := b.clusters.Get(clusterName); !ok {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrNotFound, clusterName)
	}

	addon, ok := b.addons.Get(addonKey(clusterName, addonName))
	if !ok {
		return nil, fmt.Errorf("%w: addon %s not found in cluster %s", ErrNotFound, addonName, clusterName)
	}

	if resolveConflicts != "" && !isValidResolveConflicts(resolveConflicts) {
		return nil, fmt.Errorf(
			"%w: resolveConflicts %q must be one of OVERWRITE, NONE, PRESERVE",
			ErrValidation, resolveConflicts,
		)
	}

	if podIdentityAssociations != nil {
		for _, s := range *podIdentityAssociations {
			if s.RoleARN == "" || s.ServiceAccount == "" {
				return nil, fmt.Errorf(
					"%w: podIdentityAssociations roleArn and serviceAccount are required",
					ErrValidation,
				)
			}
		}
	}

	if addonVersion != "" {
		addon.AddonVersion = addonVersion
	}

	if serviceAccountRoleARN != "" {
		addon.ServiceAccountRoleARN = serviceAccountRoleARN
	}

	if configuration != "" {
		addon.Configuration = configuration
	}

	if resolveConflicts != "" {
		addon.ResolveConflicts = resolveConflicts
	}

	if podIdentityAssociations != nil {
		b.replaceAddonPodIdentityAssociationsLocked(clusterName, addon, *podIdentityAssociations)
	}

	cp := *addon

	return &cp, nil
}

// replaceAddonPodIdentityAssociationsLocked deletes every pod identity
// association owned by addon (OwnerARN == addon.ARN) and creates one per
// spec. Caller must hold b.mu for writing and must have already validated
// specs.
func (b *InMemoryBackend) replaceAddonPodIdentityAssociationsLocked(
	clusterName string, addon *Addon, specs []PodIdentityAssociationSpec,
) {
	for _, a := range b.podIdentityAssociationsByCluster.Get(clusterName) {
		if a.OwnerARN != addon.ARN {
			continue
		}

		if a.Tags != nil {
			a.Tags.Close()
		}

		b.podIdentityAssociations.Delete(podIdentityAssociationKey(clusterName, a.AssociationID))
	}

	ids := make([]string, 0, len(specs))
	now := time.Now().UTC()

	for _, s := range specs {
		assocID := uuid.NewString()
		assoc := &PodIdentityAssociation{
			ClusterName:    clusterName,
			AssociationID:  assocID,
			ARN:            arn.Build("eks", b.region, b.accountID, "podidentityassociation/"+clusterName+"/"+assocID),
			Namespace:      addonPodIdentityNamespace,
			ServiceAccount: s.ServiceAccount,
			RoleARN:        s.RoleARN,
			OwnerARN:       addon.ARN,
			CreatedAt:      now,
			ModifiedAt:     now,
			ExternalID:     uuid.NewString(),
			Tags:           tags.New("eks.podidentity." + clusterName + "." + assocID + ".tags"),
		}
		b.podIdentityAssociations.Put(assoc)
		ids = append(ids, assocID)
	}

	addon.PodIdentityAssociations = ids
}

// DescribeAddonVersions returns static addon version metadata.
func (b *InMemoryBackend) DescribeAddonVersions() []map[string]any {
	return []map[string]any{
		{
			keyAddonName: addonVPCCNI,
			keyType:      keyNetworking,
			keyAddonVersions: []map[string]any{
				{
					keyAddonVersion: "v1.18.5-eksbuild.1",
					keyCompatibilities: []map[string]string{
						{keyClusterVersion: defaultK8sVersion},
						{keyClusterVersion: priorK8sVersion},
					},
				},
				{
					keyAddonVersion:    "v1.17.1-eksbuild.1",
					keyCompatibilities: []map[string]string{{keyClusterVersion: "1.30"}, {keyClusterVersion: "1.29"}},
				},
			},
		},
		{
			keyAddonName: "coredns",
			keyType:      keyNetworking,
			keyAddonVersions: []map[string]any{
				{
					keyAddonVersion: "v1.11.4-eksbuild.2",
					keyCompatibilities: []map[string]string{
						{keyClusterVersion: defaultK8sVersion},
						{keyClusterVersion: priorK8sVersion},
					},
				},
			},
		},
		{
			keyAddonName: "kube-proxy",
			keyType:      keyNetworking,
			keyAddonVersions: []map[string]any{
				{
					keyAddonVersion:    "v1.32.0-eksbuild.1",
					keyCompatibilities: []map[string]string{{keyClusterVersion: defaultK8sVersion}},
				},
				{
					keyAddonVersion:    "v1.31.3-eksbuild.1",
					keyCompatibilities: []map[string]string{{keyClusterVersion: priorK8sVersion}},
				},
			},
		},
		{
			keyAddonName: "aws-ebs-csi-driver",
			keyType:      "storage",
			keyAddonVersions: []map[string]any{
				{
					keyAddonVersion: "v1.37.0-eksbuild.1",
					keyCompatibilities: []map[string]string{
						{keyClusterVersion: defaultK8sVersion},
						{keyClusterVersion: priorK8sVersion},
					},
				},
			},
		},
		{
			keyAddonName: "aws-efs-csi-driver",
			keyType:      "storage",
			keyAddonVersions: []map[string]any{
				{
					keyAddonVersion: "v2.1.0-eksbuild.1",
					keyCompatibilities: []map[string]string{
						{keyClusterVersion: defaultK8sVersion},
						{keyClusterVersion: priorK8sVersion},
					},
				},
			},
		},
	}
}

// DescribeAddonConfiguration returns static addon configuration schema.
//
// configurationSchema is the JSON schema encoded AS A STRING -- confirmed
// against aws-sdk-go-v2/service/eks@v1.90.4's deserializers.go
// (awsRestjson1_deserializeOpDocumentDescribeAddonConfigurationOutput, case
// "configurationSchema": value.(string)) -- not a nested JSON object. A real
// client decoding this response would fail with "expected String to be of
// type string, got map[string]interface {} instead".
func (b *InMemoryBackend) DescribeAddonConfiguration(addonName, addonVersion string) map[string]any {
	schema, _ := json.Marshal(map[string]any{
		keyType:      "object",
		"properties": map[string]any{},
	})

	return map[string]any{
		keyAddonName:          addonName,
		keyAddonVersion:       addonVersion,
		"configurationSchema": string(schema),
	}
}

// AddAddonInternal inserts a pre-built add-on into the backend.
// Intended only for test seeding.
func (b *InMemoryBackend) AddAddonInternal(a *Addon) {
	b.mu.Lock("AddAddonInternal")
	defer b.mu.Unlock()

	if a.Tags == nil {
		a.Tags = tags.New("eks.addon." + a.ClusterName + "." + a.AddonName + ".tags")
	}

	b.addons.Put(a)
}

// ListAllAddons returns all addons across all clusters.
func (b *InMemoryBackend) ListAllAddons() []*Addon {
	b.mu.RLock("ListAllAddons")
	defer b.mu.RUnlock()

	items := b.addons.All()
	list := make([]*Addon, 0, len(items))

	for _, a := range items {
		cp := *a
		list = append(list, &cp)
	}

	return list
}

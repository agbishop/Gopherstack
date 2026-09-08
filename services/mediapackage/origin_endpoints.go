package mediapackage

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// validatePackagingConfig checks the required-together sub-fields of the
// typed packaging blocks (Authorization, MssPackage's SPEKE chain). Each
// block itself is optional; if present, these members are required (SDK
// doc comments, aws-sdk-go-v2/service/mediapackage@v1.42.4):
// types.Authorization (types.go:10-26), types.MssEncryption (types.go:
// 563-572), types.SpekeKeyProvider (types.go:681-721),
// types.EncryptionContractConfiguration (types.go:246-259).
func validatePackagingConfig(pkg PackagingConfig) error {
	if pkg.Authorization != nil {
		a := pkg.Authorization
		if a.CdnIdentifierSecret == "" || a.SecretsRoleArn == "" {
			return fmt.Errorf(
				"%w: Authorization.CdnIdentifierSecret and Authorization.SecretsRoleArn are required",
				ErrInvalidParameter,
			)
		}
	}

	if pkg.MssPackage == nil || pkg.MssPackage.Encryption == nil {
		return nil
	}

	speke := pkg.MssPackage.Encryption.SpekeKeyProvider
	if speke == nil {
		return fmt.Errorf("%w: MssPackage.Encryption.SpekeKeyProvider is required", ErrInvalidParameter)
	}

	if speke.ResourceID == "" || speke.RoleArn == "" || speke.URL == "" || len(speke.SystemIDs) == 0 {
		return fmt.Errorf(
			"%w: SpekeKeyProvider.ResourceId, RoleArn, SystemIds, and Url are required",
			ErrInvalidParameter,
		)
	}

	ecc := speke.EncryptionContractConfiguration
	if ecc != nil && (ecc.PresetSpeke20Audio == "" || ecc.PresetSpeke20Video == "") {
		return fmt.Errorf(
			"%w: EncryptionContractConfiguration.PresetSpeke20Audio and PresetSpeke20Video are required",
			ErrInvalidParameter,
		)
	}

	return nil
}

// CreateOriginEndpoint creates a new origin endpoint.
func (b *InMemoryBackend) CreateOriginEndpoint(
	channelID, id, description, manifestName string,
	startoverWindowSeconds, timeDelaySeconds int,
	origination string,
	whitelist []string,
	tags map[string]string,
	pkg PackagingConfig,
) (*OriginEndpoint, error) {
	if channelID == "" {
		return nil, fmt.Errorf("%w: ChannelId is required", ErrInvalidParameter)
	}

	if id == "" {
		return nil, fmt.Errorf("%w: Id is required", ErrInvalidParameter)
	}

	if err := validatePackagingConfig(pkg); err != nil {
		return nil, err
	}

	b.mu.Lock("CreateOriginEndpoint")
	defer b.mu.Unlock()

	if !b.channels.Has(channelID) {
		return nil, fmt.Errorf("%w: channel %q not found", ErrNotFound, channelID)
	}

	if b.originEndpoints.Has(id) {
		return nil, fmt.Errorf("%w: origin endpoint %q already exists", ErrConflict, id)
	}

	if origination == "" {
		origination = originationAllow
	}

	if manifestName == "" {
		manifestName = id
	}

	tagsCopy := make(map[string]string, len(tags))
	maps.Copy(tagsCopy, tags)

	wl := make([]string, len(whitelist))
	copy(wl, whitelist)

	ep := &storedOriginEndpoint{
		ARN:          b.buildOriginEndpointARN(id),
		ChannelID:    channelID,
		ID:           id,
		Description:  description,
		ManifestName: manifestName,
		URL: fmt.Sprintf(
			"https://cf.mediapackage.%s.amazonaws.com/out/v1/%s/%s.m3u8",
			b.region,
			id,
			manifestName,
		),
		Origination:            origination,
		CreatedAt:              time.Now().UTC().Format(time.RFC3339),
		StartoverWindowSeconds: startoverWindowSeconds,
		TimeDelaySeconds:       timeDelaySeconds,
		Whitelist:              wl,
		Tags:                   tagsCopy,
		Authorization:          copyAuthorization(pkg.Authorization),
		CmafPackage:            copyAnyMap(pkg.CmafPackage),
		DashPackage:            copyAnyMap(pkg.DashPackage),
		HlsPackage:             copyAnyMap(pkg.HlsPackage),
		MssPackage:             copyMssPackage(pkg.MssPackage),
	}

	b.originEndpoints.Put(ep)

	if len(tagsCopy) > 0 {
		b.tags[ep.ARN] = make(map[string]string, len(tagsCopy))
		maps.Copy(b.tags[ep.ARN], tagsCopy)
	}

	return ep.toOriginEndpoint(), nil
}

// DescribeOriginEndpoint returns an origin endpoint by ID.
func (b *InMemoryBackend) DescribeOriginEndpoint(id string) (*OriginEndpoint, error) {
	b.mu.RLock("DescribeOriginEndpoint")
	defer b.mu.RUnlock()

	ep, ok := b.originEndpoints.Get(id)
	if !ok {
		return nil, fmt.Errorf("%w: origin endpoint %q not found", ErrNotFound, id)
	}

	return ep.toOriginEndpoint(), nil
}

// UpdateOriginEndpoint updates an origin endpoint's configuration.
func (b *InMemoryBackend) UpdateOriginEndpoint(
	id, description, manifestName string,
	startoverWindowSeconds, timeDelaySeconds int,
	origination string,
	whitelist []string,
	pkg PackagingConfig,
) (*OriginEndpoint, error) {
	if err := validatePackagingConfig(pkg); err != nil {
		return nil, err
	}

	b.mu.Lock("UpdateOriginEndpoint")
	defer b.mu.Unlock()

	ep, ok := b.originEndpoints.Get(id)
	if !ok {
		return nil, fmt.Errorf("%w: origin endpoint %q not found", ErrNotFound, id)
	}

	if description != "" {
		ep.Description = description
	}

	if manifestName != "" {
		ep.ManifestName = manifestName
	}

	if origination != "" {
		ep.Origination = origination
	}

	if startoverWindowSeconds >= 0 {
		ep.StartoverWindowSeconds = startoverWindowSeconds
	}

	if timeDelaySeconds >= 0 {
		ep.TimeDelaySeconds = timeDelaySeconds
	}

	if whitelist != nil {
		wl := make([]string, len(whitelist))
		copy(wl, whitelist)
		ep.Whitelist = wl
	}

	if pkg.Authorization != nil {
		ep.Authorization = copyAuthorization(pkg.Authorization)
	}

	if pkg.CmafPackage != nil {
		ep.CmafPackage = copyAnyMap(pkg.CmafPackage)
	}

	if pkg.DashPackage != nil {
		ep.DashPackage = copyAnyMap(pkg.DashPackage)
	}

	if pkg.HlsPackage != nil {
		ep.HlsPackage = copyAnyMap(pkg.HlsPackage)
	}

	if pkg.MssPackage != nil {
		ep.MssPackage = copyMssPackage(pkg.MssPackage)
	}

	return ep.toOriginEndpoint(), nil
}

// DeleteOriginEndpoint deletes an origin endpoint.
func (b *InMemoryBackend) DeleteOriginEndpoint(id string) (*OriginEndpoint, error) {
	b.mu.Lock("DeleteOriginEndpoint")
	defer b.mu.Unlock()

	ep, ok := b.originEndpoints.Get(id)
	if !ok {
		return nil, fmt.Errorf("%w: origin endpoint %q not found", ErrNotFound, id)
	}

	b.originEndpoints.Delete(id)
	delete(b.tags, ep.ARN)

	return ep.toOriginEndpoint(), nil
}

// ListOriginEndpoints returns a paginated list of origin endpoints.
func (b *InMemoryBackend) ListOriginEndpoints(
	channelID string,
	maxResults int,
	nextToken string,
) ([]*OriginEndpoint, string, error) {
	b.mu.RLock("ListOriginEndpoints")
	defer b.mu.RUnlock()

	var all []*storedOriginEndpoint
	if channelID == "" {
		all = b.originEndpoints.Snapshot()
	} else {
		all = slices.Clone(b.originEndpointsByChannel.Get(channelID))
		sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	}

	p := page.New(all, nextToken, maxResults, defaultMaxResults)

	endpoints := make([]*OriginEndpoint, 0, len(p.Data))
	for _, ep := range p.Data {
		endpoints = append(endpoints, ep.toOriginEndpoint())
	}

	return endpoints, p.Next, nil
}

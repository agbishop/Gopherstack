package transfer

import (
	"fmt"
	"maps"
	"sort"
	"time"

	"github.com/google/uuid"
)

// CreateWebAppInput holds all fields for CreateWebApp.
type CreateWebAppInput struct {
	IdentityCenterConfig *WebAppIdentityCenterConfig
	VpcConfig            *WebAppVpcConfig
	Tags                 map[string]string
	AccessEndpoint       string
	WebAppEndpointPolicy string
	WebAppUnits          int32
}

const defaultWebAppEndpointPolicy = "STANDARD"

// CreateWebApp creates a Transfer web application. IdentityCenterConfig is required
// by real AWS (CreateWebAppInput.IdentityProviderDetails is a required member, and
// IdentityCenterConfig is the only identity-provider type web apps support).
func (b *InMemoryBackend) CreateWebApp(in *CreateWebAppInput) (*WebApp, error) {
	if in.IdentityCenterConfig == nil ||
		in.IdentityCenterConfig.InstanceArn == "" ||
		in.IdentityCenterConfig.Role == "" {
		return nil, fmt.Errorf(
			"%w: IdentityProviderDetails.IdentityCenterConfig with InstanceArn and Role is required",
			ErrValidation,
		)
	}

	b.mu.Lock("CreateWebApp")
	defer b.mu.Unlock()

	webAppID := "webapp-" + uuid.NewString()[:20]

	merged := make(map[string]string, len(in.Tags))
	maps.Copy(merged, in.Tags)

	icc := *in.IdentityCenterConfig
	icc.ApplicationArn = "arn:aws:sso::" + b.accountID + ":application/apl-" + uuid.NewString()[:16]

	var vpcConfig *WebAppVpcConfig
	if in.VpcConfig != nil {
		vc := *in.VpcConfig
		vc.VpcEndpointID = "vpce-" + uuid.NewString()[:17]
		vpcConfig = &vc
	}

	webAppEndpoint := "https://" + webAppID + ".transfer-webapp." + b.region + ".on.aws"

	accessEndpoint := in.AccessEndpoint
	if accessEndpoint == "" {
		accessEndpoint = webAppEndpoint
	}

	webAppEndpointPolicy := in.WebAppEndpointPolicy
	if webAppEndpointPolicy == "" {
		webAppEndpointPolicy = defaultWebAppEndpointPolicy
	}

	webAppUnits := in.WebAppUnits
	if webAppUnits == 0 {
		webAppUnits = 1
	}

	w := &WebApp{
		WebAppID:             webAppID,
		IdentityCenterConfig: &icc,
		VpcConfig:            vpcConfig,
		AccessEndpoint:       accessEndpoint,
		WebAppEndpoint:       webAppEndpoint,
		WebAppEndpointPolicy: webAppEndpointPolicy,
		WebAppUnits:          webAppUnits,
		CreatedAt:            time.Now(),
		Tags:                 merged,
		AccountID:            b.accountID,
		Region:               b.region,
	}
	b.webApps.Put(w)
	b.initTagsStore(webAppARN(b.accountID, b.region, webAppID), merged)

	return cloneWebApp(w), nil
}

// DeleteWebApp removes a web app by ID.
func (b *InMemoryBackend) DeleteWebApp(webAppID string) error {
	b.mu.Lock("DeleteWebApp")
	defer b.mu.Unlock()

	if !b.webApps.Has(webAppID) {
		return fmt.Errorf("%w: web app %s not found", ErrWebAppNotFound, webAppID)
	}

	b.webApps.Delete(webAppID)
	delete(b.tagsStore, webAppARN(b.accountID, b.region, webAppID))

	return nil
}

// DescribeWebApp returns a web app by ID.
func (b *InMemoryBackend) DescribeWebApp(webAppID string) (*WebApp, error) {
	b.mu.RLock("DescribeWebApp")
	defer b.mu.RUnlock()

	w, ok := b.webApps.Get(webAppID)
	if !ok {
		return nil, fmt.Errorf("%w: web app %s not found", ErrWebAppNotFound, webAppID)
	}

	return cloneWebApp(w), nil
}

// ListWebApps returns all web apps sorted by webAppID.
func (b *InMemoryBackend) ListWebApps() []*WebApp {
	b.mu.RLock("ListWebApps")
	defer b.mu.RUnlock()

	all := b.webApps.All()
	out := make([]*WebApp, 0, len(all))

	for _, w := range all {
		out = append(out, cloneWebApp(w))
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].WebAppID < out[j].WebAppID
	})

	return out
}

// UpdateWebAppInput holds all optional fields for UpdateWebApp. Real AWS only allows
// updating a subset of fields set at creation: AccessEndpoint, the VPC subnet list
// (not VpcId/SecurityGroupIds -- those are immutable after creation), the IAM
// Identity Center Role (not InstanceArn), and WebAppUnits.
type UpdateWebAppInput struct {
	IdentityCenterRole *string
	WebAppUnits        *int32
	VpcIPAddressType   *string
	WebAppID           string
	AccessEndpoint     string
	VpcSubnetIDs       []string
}

// UpdateWebApp updates mutable fields on a web app.
func (b *InMemoryBackend) UpdateWebApp(in *UpdateWebAppInput) (*WebApp, error) {
	b.mu.Lock("UpdateWebApp")
	defer b.mu.Unlock()

	w, ok := b.webApps.Get(in.WebAppID)
	if !ok {
		return nil, fmt.Errorf("%w: web app %s not found", ErrWebAppNotFound, in.WebAppID)
	}

	if in.AccessEndpoint != "" {
		w.AccessEndpoint = in.AccessEndpoint
	}

	if in.VpcSubnetIDs != nil {
		if w.VpcConfig == nil {
			w.VpcConfig = &WebAppVpcConfig{VpcEndpointID: "vpce-" + uuid.NewString()[:17]}
		}

		w.VpcConfig.SubnetIDs = append([]string(nil), in.VpcSubnetIDs...)
	}

	if in.VpcIPAddressType != nil {
		if w.VpcConfig == nil {
			w.VpcConfig = &WebAppVpcConfig{VpcEndpointID: "vpce-" + uuid.NewString()[:17]}
		}

		w.VpcConfig.IPAddressType = *in.VpcIPAddressType
	}

	if in.IdentityCenterRole != nil {
		if w.IdentityCenterConfig == nil {
			w.IdentityCenterConfig = &WebAppIdentityCenterConfig{}
		}

		w.IdentityCenterConfig.Role = *in.IdentityCenterRole
	}

	if in.WebAppUnits != nil {
		w.WebAppUnits = *in.WebAppUnits
	}

	return cloneWebApp(w), nil
}

// DescribeWebAppCustomization returns the customization for a web app.
// Returns empty customization (not an error) when none has been set.
func (b *InMemoryBackend) DescribeWebAppCustomization(webAppID string) (*WebAppCustomization, error) {
	b.mu.RLock("DescribeWebAppCustomization")
	defer b.mu.RUnlock()

	if !b.webApps.Has(webAppID) {
		return nil, fmt.Errorf("%w: web app %s not found", ErrWebAppNotFound, webAppID)
	}

	if c, ok := b.webAppCustomizations.Get(webAppID); ok {
		cp := *c

		return &cp, nil
	}

	return &WebAppCustomization{WebAppID: webAppID}, nil
}

// UpdateWebAppCustomization sets or overwrites the customization for a web app.
func (b *InMemoryBackend) UpdateWebAppCustomization(
	webAppID, title, logoFile, faviconFile string,
) (*WebAppCustomization, error) {
	b.mu.Lock("UpdateWebAppCustomization")
	defer b.mu.Unlock()

	if !b.webApps.Has(webAppID) {
		return nil, fmt.Errorf("%w: web app %s not found", ErrWebAppNotFound, webAppID)
	}

	c := &WebAppCustomization{
		WebAppID:    webAppID,
		Title:       title,
		LogoFile:    logoFile,
		FaviconFile: faviconFile,
	}
	b.webAppCustomizations.Put(c)

	cp := *c

	return &cp, nil
}

// DeleteWebAppCustomization clears the customization for a web app.
func (b *InMemoryBackend) DeleteWebAppCustomization(webAppID string) error {
	b.mu.Lock("DeleteWebAppCustomization")
	defer b.mu.Unlock()

	if !b.webApps.Has(webAppID) {
		return fmt.Errorf("%w: web app %s not found", ErrWebAppNotFound, webAppID)
	}

	b.webAppCustomizations.Delete(webAppID)

	return nil
}

// AddWebAppInternal seeds a web app for testing purposes.
func (b *InMemoryBackend) AddWebAppInternal(webAppID string) {
	b.mu.Lock("AddWebAppInternal")
	defer b.mu.Unlock()

	b.webApps.Put(&WebApp{
		WebAppID:  webAppID,
		CreatedAt: time.Now(),
		Tags:      make(map[string]string),
		AccountID: b.accountID,
		Region:    b.region,
	})
}

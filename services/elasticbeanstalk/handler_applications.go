package elasticbeanstalk

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/url"
	"strconv"
)

// --- Application operations ---

// appConfigTemplatesXML wraps the ConfigurationTemplates list for XML encoding.
// Using a pointer to this struct enables omitempty to suppress the outer element when empty.
type appConfigTemplatesXML struct {
	Members []string `xml:"member"`
}

// appVersionsXML wraps the Versions list for XML encoding, mirroring
// appConfigTemplatesXML. Real AWS's ApplicationDescription.Versions lists the
// version labels belonging to the application (see the CreateApplication API
// doc example response, which renders an empty `<Versions/>` element on a
// freshly created application).
type appVersionsXML struct {
	Members []string `xml:"member"`
}

// applicationDescType is used in XML responses.
type applicationDescType struct {
	ConfigurationTemplates  *appConfigTemplatesXML              `xml:"ConfigurationTemplates,omitempty"`
	Versions                *appVersionsXML                     `xml:"Versions,omitempty"`
	ResourceLifecycleConfig *applicationResourceLifecycleConfig `xml:"ResourceLifecycleConfig,omitempty"`
	ApplicationName         string                              `xml:"ApplicationName"`
	ApplicationArn          string                              `xml:"ApplicationArn"`
	Description             string                              `xml:"Description,omitempty"`
	DateCreated             string                              `xml:"DateCreated,omitempty"`
	DateUpdated             string                              `xml:"DateUpdated,omitempty"`
}

func toApplicationDesc(app *Application, configTemplateNames, versionLabels []string) applicationDescType {
	var templates *appConfigTemplatesXML
	if len(configTemplateNames) > 0 {
		templates = &appConfigTemplatesXML{Members: configTemplateNames}
	}

	var versions *appVersionsXML
	if len(versionLabels) > 0 {
		versions = &appVersionsXML{Members: versionLabels}
	}

	// ResourceLifecycleConfig is only rendered once a lifecycle service role
	// has been set via UpdateApplicationResourceLifecycle: the backend stores
	// it on the Application, but until this field existed it was never
	// surfaced back through CreateApplication/DescribeApplications/
	// UpdateApplication, making the stored value permanently unreadable.
	var lifecycleConfig *applicationResourceLifecycleConfig
	if app.ResourceLifecycleServiceRole != "" {
		lifecycleConfig = &applicationResourceLifecycleConfig{ServiceRole: app.ResourceLifecycleServiceRole}
	}

	return applicationDescType{
		ApplicationName:         app.ApplicationName,
		ApplicationArn:          app.ApplicationARN,
		Description:             app.Description,
		DateCreated:             app.DateCreated,
		DateUpdated:             app.DateUpdated,
		ConfigurationTemplates:  templates,
		Versions:                versions,
		ResourceLifecycleConfig: lifecycleConfig,
	}
}

// applicationConfigTemplateNames returns the sorted configuration template
// names belonging to appName, for embedding in an ApplicationDescription.
func (h *Handler) applicationConfigTemplateNames(ctx context.Context, appName string) []string {
	templates := h.Backend.DescribeConfigurationTemplates(ctx, appName)
	names := make([]string, 0, len(templates))

	for _, tmpl := range templates {
		names = append(names, tmpl.TemplateName)
	}

	return names
}

// applicationVersionLabels returns the sorted application version labels
// belonging to appName, for embedding in an ApplicationDescription.
func (h *Handler) applicationVersionLabels(ctx context.Context, appName string) []string {
	versions := h.Backend.DescribeApplicationVersions(ctx, appName, nil)
	labels := make([]string, 0, len(versions))

	for _, ver := range versions {
		labels = append(labels, ver.VersionLabel)
	}

	return labels
}

type createApplicationResult struct {
	Application applicationDescType `xml:"Application"`
}

type createApplicationResponse struct {
	XMLName                 xml.Name                `xml:"CreateApplicationResponse"`
	Xmlns                   string                  `xml:"xmlns,attr"`
	CreateApplicationResult createApplicationResult `xml:"CreateApplicationResult"`
	ResponseMetadata        responseMetadata        `xml:"ResponseMetadata"`
}

func (h *Handler) handleCreateApplication(ctx context.Context, vals url.Values) (any, error) {
	name := vals.Get("ApplicationName")
	if name == "" {
		return nil, fmt.Errorf("%w: ApplicationName is required", ErrInvalidParameter)
	}

	description := vals.Get("Description")

	tags := parseTagList(vals, "Tags.member")

	app, err := h.Backend.CreateApplication(ctx, name, description, tags)
	if err != nil {
		return nil, err
	}

	templateNames := h.applicationConfigTemplateNames(ctx, name)

	return &createApplicationResponse{
		Xmlns: ebXMLNS,
		CreateApplicationResult: createApplicationResult{
			Application: toApplicationDesc(app, templateNames, nil),
		},
		ResponseMetadata: responseMetadata{RequestID: "eb-create-app"},
	}, nil
}

type describeApplicationsResult struct {
	Applications []applicationDescType `xml:"Applications>member"`
}

type describeApplicationsResponse struct {
	XMLName                    xml.Name                   `xml:"DescribeApplicationsResponse"`
	Xmlns                      string                     `xml:"xmlns,attr"`
	ResponseMetadata           responseMetadata           `xml:"ResponseMetadata"`
	DescribeApplicationsResult describeApplicationsResult `xml:"DescribeApplicationsResult"`
}

func (h *Handler) handleDescribeApplications(ctx context.Context, vals url.Values) (any, error) {
	names := parseMembers(vals, "ApplicationNames.member")
	apps := h.Backend.DescribeApplications(ctx, names)

	members := make([]applicationDescType, 0, len(apps))

	for _, app := range apps {
		templateNames := h.applicationConfigTemplateNames(ctx, app.ApplicationName)
		versionLabels := h.applicationVersionLabels(ctx, app.ApplicationName)

		members = append(members, toApplicationDesc(app, templateNames, versionLabels))
	}

	return &describeApplicationsResponse{
		Xmlns:                      ebXMLNS,
		DescribeApplicationsResult: describeApplicationsResult{Applications: members},
		ResponseMetadata:           responseMetadata{RequestID: "eb-describe-apps"},
	}, nil
}

type updateApplicationResult struct {
	Application applicationDescType `xml:"Application"`
}

type updateApplicationResponse struct {
	XMLName                 xml.Name                `xml:"UpdateApplicationResponse"`
	Xmlns                   string                  `xml:"xmlns,attr"`
	UpdateApplicationResult updateApplicationResult `xml:"UpdateApplicationResult"`
	ResponseMetadata        responseMetadata        `xml:"ResponseMetadata"`
}

func (h *Handler) handleUpdateApplication(ctx context.Context, vals url.Values) (any, error) {
	name := vals.Get("ApplicationName")
	if name == "" {
		return nil, fmt.Errorf("%w: ApplicationName is required", ErrInvalidParameter)
	}

	description := vals.Get("Description")

	app, err := h.Backend.UpdateApplication(ctx, name, description)
	if err != nil {
		return nil, err
	}

	templateNames := h.applicationConfigTemplateNames(ctx, name)
	versionLabels := h.applicationVersionLabels(ctx, name)

	return &updateApplicationResponse{
		Xmlns: ebXMLNS,
		UpdateApplicationResult: updateApplicationResult{
			Application: toApplicationDesc(app, templateNames, versionLabels),
		},
		ResponseMetadata: responseMetadata{RequestID: "eb-update-app"},
	}, nil
}

type deleteApplicationResponse struct {
	XMLName          xml.Name         `xml:"DeleteApplicationResponse"`
	Xmlns            string           `xml:"xmlns,attr"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

func (h *Handler) handleDeleteApplication(ctx context.Context, vals url.Values) (any, error) {
	name := vals.Get("ApplicationName")
	if name == "" {
		return nil, fmt.Errorf("%w: ApplicationName is required", ErrInvalidParameter)
	}

	terminateEnvByForce, _ := strconv.ParseBool(vals.Get("TerminateEnvByForce"))

	if err := h.Backend.DeleteApplication(ctx, name, terminateEnvByForce); err != nil {
		return nil, err
	}

	return &deleteApplicationResponse{
		Xmlns:            ebXMLNS,
		ResponseMetadata: responseMetadata{RequestID: "eb-delete-app"},
	}, nil
}

// updateApplicationResourceLifecycleResponse is the XML response for UpdateApplicationResourceLifecycle.
type applicationResourceLifecycleConfig struct {
	ServiceRole string `xml:"ServiceRole,omitempty"`
}

type updateApplicationResourceLifecycleResult struct {
	ApplicationName         string                             `xml:"ApplicationName"`
	ResourceLifecycleConfig applicationResourceLifecycleConfig `xml:"ResourceLifecycleConfig"`
}

type updateApplicationResourceLifecycleResponse struct { //nolint:lll // AWS XML operation name causes inherently long struct declaration
	XMLName                                  xml.Name                                 `xml:"UpdateApplicationResourceLifecycleResponse"` //nolint:lll // AWS XML operation name is inherently long
	Xmlns                                    string                                   `xml:"xmlns,attr"`
	UpdateApplicationResourceLifecycleResult updateApplicationResourceLifecycleResult `xml:"UpdateApplicationResourceLifecycleResult"` //nolint:lll // AWS XML operation name is inherently long
	ResponseMetadata                         responseMetadata                         `xml:"ResponseMetadata"`
}

func (h *Handler) handleUpdateApplicationResourceLifecycle(ctx context.Context, vals url.Values) (any, error) {
	appName := vals.Get("ApplicationName")
	if appName == "" {
		return nil, fmt.Errorf("%w: ApplicationName is required", ErrInvalidParameter)
	}

	serviceRole := vals.Get("ResourceLifecycleConfig.ServiceRole")

	// Store lifecycle service role in the application (improvement #7)
	if _, err := h.Backend.UpdateApplicationResourceLifecycle(ctx, appName, serviceRole); err != nil {
		return nil, err
	}

	return &updateApplicationResourceLifecycleResponse{
		Xmlns: ebXMLNS,
		UpdateApplicationResourceLifecycleResult: updateApplicationResourceLifecycleResult{
			ApplicationName: appName,
			ResourceLifecycleConfig: applicationResourceLifecycleConfig{
				ServiceRole: serviceRole,
			},
		},
		ResponseMetadata: responseMetadata{RequestID: "eb-update-app-lifecycle"},
	}, nil
}

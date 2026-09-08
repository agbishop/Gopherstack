package serverlessrepo

import (
	"fmt"
	"strconv"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

const (
	// templateStatusActive is the status of an active CloudFormation template.
	templateStatusActive = "ACTIVE"
	// templateExpirationHours is the number of hours before a template expires.
	templateExpirationHours = 1
)

// cloneTemplate returns a deep copy of t.
func cloneTemplate(t *CloudFormationTemplate) *CloudFormationTemplate {
	cp := *t

	return &cp
}

func cloneChangeSet(cs *CloudFormationChangeSet) *CloudFormationChangeSet {
	cp := *cs
	cp.Capabilities = cloneStringSlice(cs.Capabilities)
	cp.Tags = cloneTags(cs.Tags)

	return &cp
}

// CreateCloudFormationTemplate creates a new CloudFormation template for an application.
func (b *InMemoryBackend) CreateCloudFormationTemplate(
	appName, semanticVersion string,
) (*CloudFormationTemplate, error) {
	b.mu.Lock("CreateCloudFormationTemplate")
	defer b.mu.Unlock()

	app, ok := b.applications.Get(appName)
	if !ok {
		return nil, fmt.Errorf("%w: could not find application %q", ErrApplicationNotFound, appName)
	}

	now := time.Now()
	templateID := fmt.Sprintf("%s-%d", appName, now.UnixNano())
	t := &CloudFormationTemplate{
		ApplicationID:   app.ApplicationID,
		TemplateID:      templateID,
		AppName:         appName,
		SemanticVersion: semanticVersion,
		Status:          templateStatusActive,
		CreationTime:    now,
		ExpirationTime:  now.Add(templateExpirationHours * time.Hour),
		TemplateURL: fmt.Sprintf(
			"https://s3.amazonaws.com/serverlessrepo-templates/%s/%s.template",
			appName,
			templateID,
		),
	}
	b.cfTemplates.Put(t)

	return cloneTemplate(t), nil
}

// GetCloudFormationTemplate returns a CloudFormation template by application name and template ID.
func (b *InMemoryBackend) GetCloudFormationTemplate(appName, templateID string) (*CloudFormationTemplate, error) {
	b.mu.RLock("GetCloudFormationTemplate")
	defer b.mu.RUnlock()

	if !b.applications.Has(appName) {
		return nil, fmt.Errorf("%w: could not find application %q", ErrApplicationNotFound, appName)
	}

	t, ok := b.cfTemplates.Get(templateID)
	if !ok || t.AppName != appName {
		return nil, fmt.Errorf(
			"%w: could not find template %q for application %q",
			ErrTemplateNotFound,
			templateID,
			appName,
		)
	}

	return cloneTemplate(t), nil
}

// CreateCloudFormationChangeSet creates a new CloudFormation change set for an application.
func (b *InMemoryBackend) CreateCloudFormationChangeSet(
	appName string,
	stackName string,
	changeSetName string,
	semanticVersion string,
) (*CloudFormationChangeSet, error) {
	return b.CreateCloudFormationChangeSetWithOptions(
		appName,
		stackName,
		changeSetName,
		semanticVersion,
		CreateCloudFormationChangeSetOptions{},
	)
}

// CreateCloudFormationChangeSetWithOptions creates a deployment change set with request metadata.
func (b *InMemoryBackend) CreateCloudFormationChangeSetWithOptions(
	appName string,
	stackName string,
	changeSetName string,
	semanticVersion string,
	opts CreateCloudFormationChangeSetOptions,
) (*CloudFormationChangeSet, error) {
	b.mu.Lock("CreateCloudFormationChangeSet")
	defer b.mu.Unlock()

	app, ok := b.applications.Get(appName)
	if !ok {
		// CreateCloudFormationChangeSet's modelled error set has no NotFoundException
		// (deserializers.go awsRestjson1_deserializeOpErrorCreateCloudFormationChangeSet):
		// an unknown ApplicationId is a BadRequestException, not a 404.
		return nil, fmt.Errorf("%w: could not find application %q", ErrValidation, appName)
	}

	for _, capability := range opts.Capabilities {
		if !isValidCapability(capability) {
			return nil, fmt.Errorf("%w: unsupported capability %q", ErrValidation, capability)
		}
	}

	// TemplateId, when supplied, must reference a template previously created for this
	// application via CreateCloudFormationTemplate, rather than being silently ignored.
	// CreateCloudFormationChangeSet models no NotFoundException, so an unknown or
	// mismatched-application TemplateId is a BadRequestException, not a 404.
	if opts.TemplateID != "" {
		t, foundTemplate := b.cfTemplates.Get(opts.TemplateID)
		if !foundTemplate || t.AppName != appName {
			return nil, fmt.Errorf(
				"%w: could not find template %q for application %q",
				ErrValidation,
				opts.TemplateID,
				appName,
			)
		}
	}

	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)

	csName := changeSetName
	if csName == "" {
		csName = stackName + "-" + suffix
	}

	changeSetID := arn.Build(
		"cloudformation",
		b.region,
		b.accountID,
		"changeSet/"+csName,
	)
	stackID := arn.Build(
		"cloudformation",
		b.region,
		b.accountID,
		"stack/"+stackName+"/"+suffix,
	)

	cs := &CloudFormationChangeSet{
		ApplicationID:   app.ApplicationID,
		ChangeSetID:     changeSetID,
		AppName:         appName,
		SemanticVersion: semanticVersion,
		StackID:         stackID,
		Capabilities:    cloneStringSlice(opts.Capabilities),
		Tags:            cloneTags(opts.Tags),
	}
	b.cfChangeSets.Put(cloneChangeSet(cs))

	return cloneChangeSet(cs), nil
}

func isValidCapability(capability string) bool {
	switch capability {
	case "CAPABILITY_IAM",
		"CAPABILITY_NAMED_IAM",
		"CAPABILITY_AUTO_EXPAND",
		"CAPABILITY_RESOURCE_POLICY":
		return true
	default:
		return false
	}
}

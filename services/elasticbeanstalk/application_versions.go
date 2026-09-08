package elasticbeanstalk

import (
	"context"
	"fmt"
	"slices"
	"sort"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// --- ApplicationVersion store.Table/Index helpers. Callers must hold b.mu. ---

func (b *InMemoryBackend) appVersionGet(region, appName, versionLabel string) (*ApplicationVersion, bool) {
	return b.appVersions.Get(regionKey(region, appVersionKey(appName, versionLabel)))
}

func (b *InMemoryBackend) appVersionPut(v *ApplicationVersion) { b.appVersions.Put(v) }

func (b *InMemoryBackend) appVersionDelete(region, appName, versionLabel string) {
	b.appVersions.Delete(regionKey(region, appVersionKey(appName, versionLabel)))
}

func (b *InMemoryBackend) appVersionsInRegion(region string) []*ApplicationVersion {
	return b.appVersionsByRegion.Get(region)
}

func (b *InMemoryBackend) appVersionByARN(region, resourceARN string) (*ApplicationVersion, bool) {
	list := b.appVersionsByARN.Get(regionKey(region, resourceARN))
	if len(list) == 0 {
		return nil, false
	}

	return list[0], true
}

// appVersionKey returns the map key for an application version.
func appVersionKey(appName, versionLabel string) string {
	return appName + "\x00" + versionLabel
}

// --- ApplicationVersion operations ---

// CreateApplicationVersion creates a new application version.
func (b *InMemoryBackend) CreateApplicationVersion(
	ctx context.Context,
	appName, versionLabel, description string,
	s3Bucket, s3Key string,
	tags map[string]string,
) (*ApplicationVersion, error) {
	return b.CreateApplicationVersionWithParams(ctx, appName, versionLabel, ApplicationVersionParams{
		Description: description,
		S3Bucket:    s3Bucket,
		S3Key:       s3Key,
		Tags:        tags,
		Process:     true,
	})
}

// ApplicationVersionParams holds optional CreateApplicationVersion properties.
type ApplicationVersionParams struct {
	SourceBuildInformation *SourceBuildInformation
	Tags                   map[string]string
	Description            string
	S3Bucket               string
	S3Key                  string
	Process                bool
	AutoCreateApplication  bool
}

// CreateApplicationVersionWithParams creates a new application version with source and processing state.
func (b *InMemoryBackend) CreateApplicationVersionWithParams(
	ctx context.Context,
	appName, versionLabel string,
	params ApplicationVersionParams,
) (*ApplicationVersion, error) {
	b.mu.Lock("CreateApplicationVersion")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	if _, ok := b.appVersionGet(region, appName, versionLabel); ok {
		return nil, fmt.Errorf(
			"%w: application version %s already exists",
			ErrAlreadyExists,
			versionLabel,
		)
	}

	vARN := arn.Build("elasticbeanstalk", region, b.accountID,
		"applicationversion/"+appName+"/"+versionLabel)

	if _, ok := b.applicationGet(region, appName); !ok {
		// AWS: "The name of the application. If no application is found with
		// this name, and AutoCreateApplication is false, returns an
		// InvalidParameterValue error."
		if !params.AutoCreateApplication {
			return nil, fmt.Errorf("%w: no application found named %s", ErrInvalidParameter, appName)
		}

		appARN := arn.Build("elasticbeanstalk", region, b.accountID, "application/"+appName)
		b.applicationPut(&Application{
			ApplicationName: appName,
			ApplicationARN:  appARN,
			Tags:            map[string]string{},
			DateCreated:     nowISO8601(),
			DateUpdated:     nowISO8601(),
			region:          region,
		})
		// Auto-creation goes through the same underlying "create application"
		// state transition as CreateApplication, so it carries the same
		// auto-provisioned Default configuration template -- see
		// defaultConfigTemplateName.
		b.createDefaultConfigurationTemplate(region, appName)
	}

	status := appVersionStatusUnprocessed
	if params.Process {
		status = appVersionStatusProcessed
	}

	ver := &ApplicationVersion{
		ApplicationName:        appName,
		VersionLabel:           versionLabel,
		ApplicationVersionARN:  vARN,
		Description:            params.Description,
		DateCreated:            nowISO8601(),
		DateUpdated:            nowISO8601(),
		Status:                 status,
		Process:                params.Process,
		S3Bucket:               params.S3Bucket,
		S3Key:                  params.S3Key,
		SourceBuildInformation: params.SourceBuildInformation,
		Tags:                   copyTags(params.Tags),
		region:                 region,
	}
	b.appVersionPut(ver)

	return cloneApplicationVersion(ver), nil
}

// DescribeApplicationVersions returns application versions, optionally filtered.
// Results are sorted by VersionLabel for deterministic output.
func (b *InMemoryBackend) DescribeApplicationVersions(
	ctx context.Context,
	appName string,
	versionLabels []string,
) []*ApplicationVersion {
	b.mu.RLock("DescribeApplicationVersions")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)
	vers := b.appVersionsInRegion(region)

	list := make([]*ApplicationVersion, 0, len(vers))

	for _, ver := range vers {
		if appName != "" && ver.ApplicationName != appName {
			continue
		}

		if len(versionLabels) > 0 {
			found := slices.Contains(versionLabels, ver.VersionLabel)

			if !found {
				continue
			}
		}

		list = append(list, cloneApplicationVersion(ver))
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].VersionLabel < list[j].VersionLabel
	})

	return list
}

// DeleteApplicationVersion removes an application version. Real AWS: "You
// cannot delete an application version that is associated with a running
// environment".
func (b *InMemoryBackend) DeleteApplicationVersion(ctx context.Context, appName, versionLabel string) error {
	b.mu.Lock("DeleteApplicationVersion")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	if _, ok := b.appVersionGet(region, appName, versionLabel); !ok {
		return fmt.Errorf("%w: application version %s not found", ErrNotFound, versionLabel)
	}

	for _, env := range b.environmentsInRegion(region) {
		if env.ApplicationName == appName && env.VersionLabel == versionLabel {
			return fmt.Errorf(
				"%w: application version %s is associated with a running environment",
				ErrInvalidParameter, versionLabel,
			)
		}
	}

	b.appVersionDelete(region, appName, versionLabel)

	return nil
}

// UpdateApplicationVersion updates an application version's description.
func (b *InMemoryBackend) UpdateApplicationVersion(
	ctx context.Context,
	appName, versionLabel, description string,
) (*ApplicationVersion, error) {
	b.mu.Lock("UpdateApplicationVersion")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	ver, ok := b.appVersionGet(region, appName, versionLabel)
	if !ok {
		return nil, fmt.Errorf("%w: application version %s not found", ErrNotFound, versionLabel)
	}

	ver.Description = description
	ver.DateUpdated = nowISO8601()

	return cloneApplicationVersion(ver), nil
}

// addAppVersionInternal seeds an application version directly into the backend, bypassing validation.
// Caller must hold the write lock.
func (b *InMemoryBackend) addAppVersionInternal(region string, ver *ApplicationVersion) {
	cp := cloneApplicationVersion(ver)
	cp.region = region
	b.appVersionPut(cp)
}

package elasticbeanstalk

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// --- Environment store.Table/Index helpers. Callers must hold b.mu. ---

func (b *InMemoryBackend) environmentGet(region, appName, envName string) (*Environment, bool) {
	return b.environments.Get(regionKey(region, envKey(appName, envName)))
}

func (b *InMemoryBackend) environmentPut(v *Environment) { b.environments.Put(v) }

func (b *InMemoryBackend) environmentDeleteKey(region, appName, envName string) {
	b.environments.Delete(regionKey(region, envKey(appName, envName)))
}

func (b *InMemoryBackend) environmentsInRegion(region string) []*Environment {
	return b.environmentsByRegion.Get(region)
}

func (b *InMemoryBackend) environmentByARN(region, resourceARN string) (*Environment, bool) {
	list := b.environmentsByARN.Get(regionKey(region, resourceARN))
	if len(list) == 0 {
		return nil, false
	}

	return list[0], true
}

func (b *InMemoryBackend) environmentByName(region, envName string) (*Environment, bool) {
	list := b.environmentsByName.Get(regionKey(region, envName))
	if len(list) == 0 {
		return nil, false
	}

	return list[0], true
}

func (b *InMemoryBackend) environmentCNAMETaken(region, cname string) bool {
	return len(b.environmentsByCNAME.Get(regionKey(region, cname))) > 0
}

func (b *InMemoryBackend) nextEnvID(region string) string {
	b.envCounters[region]++

	return fmt.Sprintf("e-%08d", b.envCounters[region])
}

// envKey returns the map key for an environment.
func envKey(appName, envName string) string {
	return appName + "\x00" + envName
}

// --- Environment operations ---

// CreateEnvironmentParams holds optional parameters for CreateEnvironment (improvements #1, #5, #14, #15, #16).
type CreateEnvironmentParams struct {
	TierType         string
	TierName         string
	TierVersion      string
	CNAMEPrefix      string
	PlatformARN      string
	TemplateName     string
	VersionLabel     string
	OperationsRole   string
	LoadBalancerType string
	VPCID            string
	Subnets          string
	InstanceProfile  string
	CustomAMI        string
	OptionSettings   []OptionSetting
}

// UpdateEnvironmentParams holds state changes accepted by UpdateEnvironment.
type UpdateEnvironmentParams struct {
	SolutionStackName string
	PlatformARN       string
	TemplateName      string
	VersionLabel      string
	Description       string
	TierType          string
	TierName          string
	TierVersion       string
	OptionSettings    []OptionSetting
	OptionsToRemove   []OptionSetting
}

// ValidateInstanceProfileARN validates that an instance profile ARN has the correct format (improvement #16).
func ValidateInstanceProfileARN(instanceProfile string) error {
	if instanceProfile == "" {
		return nil
	}

	if !strings.HasPrefix(instanceProfile, arnPrefixIAM) {
		return fmt.Errorf(
			"%w: InstanceProfile must be a valid IAM ARN starting with %s",
			ErrInvalidParameter,
			arnPrefixIAM,
		)
	}

	return nil
}

// CreateEnvironment creates a new Elastic Beanstalk environment.
func (b *InMemoryBackend) CreateEnvironment(
	ctx context.Context,
	appName, envName, solutionStack, description string,
	tags map[string]string,
	params CreateEnvironmentParams,
) (*Environment, error) {
	b.mu.Lock("CreateEnvironment")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	if _, ok := b.environmentGet(region, appName, envName); ok {
		return nil, fmt.Errorf("%w: environment %s already exists", ErrAlreadyExists, envName)
	}

	envID := b.nextEnvID(region)
	envARN := arn.Build("elasticbeanstalk", region, b.accountID, "environment/"+appName+"/"+envName)

	// Resolve tier fields (improvement #1)
	tierName := params.TierName
	if tierName == "" {
		tierName = defaultEnvironmentTierName
	}

	tierType := params.TierType
	if tierType == "" {
		tierType = defaultEnvironmentTierType
	}

	cnamePrefix := params.CNAMEPrefix
	if cnamePrefix == "" {
		cnamePrefix = envName
	}
	cname := cnamePrefix + "." + region + ".elasticbeanstalk.com"

	env := &Environment{
		OptionSettings:    slices.Clone(params.OptionSettings),
		ApplicationName:   appName,
		EnvironmentName:   envName,
		EnvironmentID:     envID,
		EnvironmentARN:    envARN,
		SolutionStackName: solutionStack,
		PlatformARN:       params.PlatformARN,
		TemplateName:      params.TemplateName,
		VersionLabel:      params.VersionLabel,
		Description:       description,
		OperationsRole:    params.OperationsRole,
		Status:            envStatusReady,
		Health:            envHealthGreen,
		Tier:              tierName,
		TierType:          tierType,
		TierName:          tierName,
		TierVersion:       params.TierVersion,
		CNAME:             cname,
		CNAMEPrefix:       cnamePrefix,
		LoadBalancerType:  params.LoadBalancerType,
		VPCID:             params.VPCID,
		Subnets:           params.Subnets,
		InstanceProfile:   params.InstanceProfile,
		CustomAMI:         params.CustomAMI,
		DateCreated:       nowISO8601(),
		DateUpdated:       nowISO8601(),
		Region:            region,
		Tags:              copyTags(tags),
	}
	b.environmentPut(env)

	b.appendEvent(region, env, "Successfully launched environment: "+envName+".", eventSeverityInfo)

	return cloneEnvironment(env), nil
}

// DescribeEnvironments returns environments, optionally filtered by app/environment names or IDs.
// Results are sorted by EnvironmentName for deterministic output.
func (b *InMemoryBackend) DescribeEnvironments(
	ctx context.Context,
	appName string,
	envNames []string,
	envIDs []string,
) []*Environment {
	b.mu.RLock("DescribeEnvironments")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)
	envs := b.environmentsInRegion(region)

	list := make([]*Environment, 0, len(envs))

	for _, env := range envs {
		if appName != "" && env.ApplicationName != appName {
			continue
		}

		if len(envNames) > 0 {
			found := slices.Contains(envNames, env.EnvironmentName)

			if !found {
				continue
			}
		}

		if len(envIDs) > 0 {
			found := slices.Contains(envIDs, env.EnvironmentID)

			if !found {
				continue
			}
		}

		list = append(list, cloneEnvironment(env))
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].EnvironmentName < list[j].EnvironmentName
	})

	return list
}

// UpdateEnvironment updates an environment's description or solution stack.
func (b *InMemoryBackend) UpdateEnvironment(
	ctx context.Context,
	appName, envName, description, solutionStack string,
) (*Environment, error) {
	return b.UpdateEnvironmentWithParams(ctx, appName, envName, UpdateEnvironmentParams{
		Description:       description,
		SolutionStackName: solutionStack,
	})
}

// UpdateEnvironmentWithParams applies all mutable environment properties.
func (b *InMemoryBackend) UpdateEnvironmentWithParams(
	ctx context.Context,
	appName, envName string,
	params UpdateEnvironmentParams,
) (*Environment, error) {
	b.mu.Lock("UpdateEnvironment")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	env, ok := b.environmentGet(region, appName, envName)
	if !ok {
		return nil, fmt.Errorf("%w: environment %s not found", ErrNotFound, envName)
	}

	if params.Description != "" {
		env.Description = params.Description
	}

	if params.SolutionStackName != "" {
		env.SolutionStackName = params.SolutionStackName
		env.PlatformARN = ""
		env.TemplateName = ""
	}

	if params.PlatformARN != "" {
		env.PlatformARN = params.PlatformARN
		env.SolutionStackName = ""
		env.TemplateName = ""
	}

	if params.TemplateName != "" {
		env.TemplateName = params.TemplateName
		env.SolutionStackName = ""
		env.PlatformARN = ""
	}

	if params.VersionLabel != "" {
		env.VersionLabel = params.VersionLabel
	}

	if params.TierName != "" {
		env.Tier = params.TierName
		env.TierName = params.TierName
	}

	if params.TierType != "" {
		env.TierType = params.TierType
	}

	if params.TierVersion != "" {
		env.TierVersion = params.TierVersion
	}

	env.OptionSettings = updateOptionSettings(
		env.OptionSettings,
		params.OptionSettings,
		params.OptionsToRemove,
	)

	env.DateUpdated = nowISO8601()

	b.appendEvent(region, env, "Environment update completed successfully.", eventSeverityInfo)

	return cloneEnvironment(env), nil
}

// updateOptionSettings applies updates and removals while preserving deterministic output ordering.
func updateOptionSettings(current, updates, removals []OptionSetting) []OptionSetting {
	byKey := make(map[string]OptionSetting, len(current)+len(updates))
	for _, setting := range current {
		byKey[optionSettingKey(setting)] = setting
	}
	for _, setting := range updates {
		byKey[optionSettingKey(setting)] = setting
	}
	for _, setting := range removals {
		delete(byKey, optionSettingKey(setting))
	}

	result := make([]OptionSetting, 0, len(byKey))
	for _, setting := range byKey {
		result = append(result, setting)
	}
	sort.Slice(result, func(i, j int) bool {
		return optionSettingKey(result[i]) < optionSettingKey(result[j])
	})

	return result
}

func optionSettingKey(setting OptionSetting) string {
	return setting.Namespace + "\x00" + setting.OptionName + "\x00" + setting.ResourceName
}

// TerminateEnvironment marks an environment as Terminated and removes it from storage.
func (b *InMemoryBackend) TerminateEnvironment(ctx context.Context, appName, envName string) (*Environment, error) {
	b.mu.Lock("TerminateEnvironment")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	env, ok := b.environmentGet(region, appName, envName)
	if !ok {
		return nil, fmt.Errorf("%w: environment %s not found", ErrNotFound, envName)
	}

	return b.terminateEnvironmentLocked(region, env), nil
}

// terminateEnvironmentLocked marks env as Terminated and removes it from
// storage. Caller must hold b.mu.
func (b *InMemoryBackend) terminateEnvironmentLocked(region string, env *Environment) *Environment {
	env.Status = "Terminated"
	out := cloneEnvironment(env)
	b.environmentDeleteKey(region, env.ApplicationName, env.EnvironmentName)
	delete(b.managedActionHistory[region], env.EnvironmentName)

	b.appendEvent(region, env, "terminateEnvironment completed successfully.", eventSeverityInfo)

	return out
}

// AbortEnvironmentUpdate aborts an in-progress environment configuration update.
// This is a no-op in the in-memory backend since updates complete instantly.
func (b *InMemoryBackend) AbortEnvironmentUpdate(_ context.Context, _ string) error {
	return nil
}

// AssociateEnvironmentOperationsRole associates an operations IAM role with an environment.
func (b *InMemoryBackend) AssociateEnvironmentOperationsRole(
	ctx context.Context,
	envName, role string,
) error {
	b.mu.Lock("AssociateEnvironmentOperationsRole")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	env, ok := b.environmentByName(region, envName)
	if !ok {
		return fmt.Errorf("%w: environment %s not found", ErrNotFound, envName)
	}

	env.OperationsRole = role

	return nil
}

// CheckDNSAvailability checks whether the specified CNAME prefix is available.
// Returns available=true when no existing environment in the request region uses that prefix.
func (b *InMemoryBackend) CheckDNSAvailability(ctx context.Context, cnamePrefix string) (bool, string) {
	b.mu.RLock("CheckDNSAvailability")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)
	fqcname := cnamePrefix + "." + region + ".elasticbeanstalk.com"

	if b.environmentCNAMETaken(region, fqcname) {
		return false, fqcname
	}

	if _, ok := b.environmentByName(region, cnamePrefix); ok {
		return false, fqcname
	}

	return true, fqcname
}

// ComposeEnvironments returns existing environments for an application.
// In a real deployment this would create multiple environments; the stub
// returns the already-running environments for the given application.
// Results are sorted by EnvironmentName for deterministic output.
func (b *InMemoryBackend) ComposeEnvironments(ctx context.Context, appName string) []*Environment {
	b.mu.RLock("ComposeEnvironments")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)
	envs := b.environmentsInRegion(region)
	list := make([]*Environment, 0, len(envs))

	for _, env := range envs {
		if env.ApplicationName == appName {
			list = append(list, cloneEnvironment(env))
		}
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].EnvironmentName < list[j].EnvironmentName
	})

	return list
}

// DescribeEnvironmentHealth returns the health and status of an environment by name.
func (b *InMemoryBackend) DescribeEnvironmentHealth(ctx context.Context, envName string) (string, string, error) {
	b.mu.RLock("DescribeEnvironmentHealth")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)

	env, ok := b.environmentByName(region, envName)
	if !ok {
		return "", "", fmt.Errorf("%w: environment %s not found", ErrNotFound, envName)
	}

	return env.Health, env.Status, nil
}

// DisassociateEnvironmentOperationsRole removes the operations role from an environment.
func (b *InMemoryBackend) DisassociateEnvironmentOperationsRole(ctx context.Context, envName string) error {
	b.mu.Lock("DisassociateEnvironmentOperationsRole")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	env, ok := b.environmentByName(region, envName)
	if !ok {
		return fmt.Errorf("%w: environment %s not found", ErrNotFound, envName)
	}

	env.OperationsRole = ""

	return nil
}

// SwapEnvironmentCNAMEs swaps the CNAME values between two environments (improvement #10).
func (b *InMemoryBackend) SwapEnvironmentCNAMEs(ctx context.Context, sourceEnvName, destEnvName string) error {
	b.mu.Lock("SwapEnvironmentCNAMEs")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	var srcEnv, dstEnv *Environment

	for _, env := range b.environmentsInRegion(region) {
		switch env.EnvironmentName {
		case sourceEnvName:
			srcEnv = env
		case destEnvName:
			dstEnv = env
		}
	}

	if srcEnv == nil {
		return fmt.Errorf("%w: source environment %s not found", ErrNotFound, sourceEnvName)
	}

	if dstEnv == nil {
		return fmt.Errorf("%w: destination environment %s not found", ErrNotFound, destEnvName)
	}

	// CNAME is an indexed field (environmentsByCNAME); mutating it in place
	// would leave a stale index entry (see pkgs/store gotcha), so both
	// entries are deleted, mutated, and re-Put to rebuild every index.
	b.environmentDeleteKey(region, srcEnv.ApplicationName, srcEnv.EnvironmentName)
	b.environmentDeleteKey(region, dstEnv.ApplicationName, dstEnv.EnvironmentName)

	srcEnv.CNAME, dstEnv.CNAME = dstEnv.CNAME, srcEnv.CNAME

	b.environmentPut(srcEnv)
	b.environmentPut(dstEnv)

	return nil
}

// addEnvironmentInternal seeds an environment directly into the backend, bypassing validation.
// Caller must hold the write lock.
func (b *InMemoryBackend) addEnvironmentInternal(region string, env *Environment) {
	cp := cloneEnvironment(env)
	cp.Region = region
	b.environmentPut(cp)
}

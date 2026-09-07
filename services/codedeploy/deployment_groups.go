package codedeploy

import (
	"fmt"
	"sort"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// CreateDeploymentGroup creates a deployment group for an application.
func (b *InMemoryBackend) CreateDeploymentGroup(
	appName, dgName string,
	input DeploymentGroupInput,
	kv map[string]string,
) (*DeploymentGroup, error) {
	b.mu.Lock("CreateDeploymentGroup")
	defer b.mu.Unlock()

	app, ok := b.applications.Get(appName)
	if !ok {
		return nil, fmt.Errorf("%w: application %s not found", ErrNotFound, appName)
	}

	if b.deploymentGroups.Has(dgKey(appName, dgName)) {
		return nil, fmt.Errorf("%w: deployment group %s already exists", ErrDeploymentGroupAlreadyExists, dgName)
	}

	if err := validateDeploymentGroupTagFilters(input); err != nil {
		return nil, err
	}

	dgID := uuid.NewString()
	t := tags.New("codedeploy.dg." + appName + "." + dgName + ".tags")
	if len(kv) > 0 {
		t.Merge(kv)
	}

	if input.DeploymentConfigName == "" {
		input.DeploymentConfigName = "CodeDeployDefault.AllAtOnce"
	}

	dg := &DeploymentGroup{
		ApplicationName:                  appName,
		DeploymentGroupName:              dgName,
		DeploymentGroupID:                dgID,
		ServiceRoleArn:                   input.ServiceRoleArn,
		DeploymentConfigName:             input.DeploymentConfigName,
		ComputePlatform:                  app.ComputePlatform,
		AccountID:                        b.accountID,
		Region:                           b.region,
		Tags:                             t,
		Ec2TagFilters:                    input.Ec2TagFilters,
		OnPremisesInstanceTagFilters:     input.OnPremisesInstanceTagFilters,
		AutoScalingGroups:                input.AutoScalingGroups,
		LoadBalancerInfo:                 input.LoadBalancerInfo,
		DeploymentStyle:                  input.DeploymentStyle,
		Ec2TagSet:                        input.Ec2TagSet,
		OnPremisesTagSet:                 input.OnPremisesTagSet,
		BlueGreenDeploymentConfiguration: input.BlueGreenDeploymentConfiguration,
		AlarmConfiguration:               input.AlarmConfiguration,
		AutoRollbackConfiguration:        input.AutoRollbackConfiguration,
		TriggerConfigurations:            input.TriggerConfigurations,
		ECSServices:                      input.ECSServices,
		OutdatedInstancesStrategy:        input.OutdatedInstancesStrategy,
		TerminationHookEnabled:           input.TerminationHookEnabled,
	}

	b.deploymentGroups.Put(dg)

	cp := *dg

	return &cp, nil
}

// GetDeploymentGroup returns a deployment group by application and group name.
func (b *InMemoryBackend) GetDeploymentGroup(appName, dgName string) (*DeploymentGroup, error) {
	b.mu.RLock("GetDeploymentGroup")
	defer b.mu.RUnlock()

	dg, ok := b.deploymentGroups.Get(dgKey(appName, dgName))
	if !ok {
		return nil, fmt.Errorf("%w: deployment group %s not found", ErrDeploymentGroupNotFound, dgName)
	}

	cp := *dg

	return &cp, nil
}

// UpdateDeploymentGroup updates a deployment group, optionally renaming it.
// Returns true if alarms or triggers were removed (hooksNotCleanedUp).
func (b *InMemoryBackend) UpdateDeploymentGroup(
	appName, currentDGName, newDGName string,
	input DeploymentGroupInput,
) (bool, error) {
	b.mu.Lock("UpdateDeploymentGroup")
	defer b.mu.Unlock()

	if !b.applications.Has(appName) {
		return false, fmt.Errorf("%w: application %s not found", ErrNotFound, appName)
	}

	oldKey := dgKey(appName, currentDGName)

	dg, ok := b.deploymentGroups.Get(oldKey)
	if !ok {
		return false, fmt.Errorf("%w: deployment group %s not found", ErrDeploymentGroupNotFound, currentDGName)
	}

	if err := validateDeploymentGroupTagFilters(input); err != nil {
		return false, err
	}

	// Track whether hooks/alarms were previously configured and are now being removed.
	hooksNotCleanedUp := dg.AlarmConfiguration != nil && dg.AlarmConfiguration.Enabled &&
		(input.AlarmConfiguration == nil || !input.AlarmConfiguration.Enabled)

	if len(dg.TriggerConfigurations) > 0 && len(input.TriggerConfigurations) == 0 {
		hooksNotCleanedUp = true
	}

	if input.ServiceRoleArn != "" {
		dg.ServiceRoleArn = input.ServiceRoleArn
	}
	if input.DeploymentConfigName != "" {
		dg.DeploymentConfigName = input.DeploymentConfigName
	}
	if input.OutdatedInstancesStrategy != "" {
		dg.OutdatedInstancesStrategy = input.OutdatedInstancesStrategy
	}

	dg.Ec2TagFilters = input.Ec2TagFilters
	dg.OnPremisesInstanceTagFilters = input.OnPremisesInstanceTagFilters
	dg.AutoScalingGroups = input.AutoScalingGroups
	dg.LoadBalancerInfo = input.LoadBalancerInfo
	dg.DeploymentStyle = input.DeploymentStyle
	dg.Ec2TagSet = input.Ec2TagSet
	dg.OnPremisesTagSet = input.OnPremisesTagSet
	dg.BlueGreenDeploymentConfiguration = input.BlueGreenDeploymentConfiguration
	dg.AlarmConfiguration = input.AlarmConfiguration
	dg.AutoRollbackConfiguration = input.AutoRollbackConfiguration
	dg.TriggerConfigurations = input.TriggerConfigurations
	dg.ECSServices = input.ECSServices
	dg.TerminationHookEnabled = input.TerminationHookEnabled

	if newDGName != "" && newDGName != currentDGName {
		// DeploymentGroupName is part of the store.Table primary key (see
		// dgKey), so Put-after-in-place-mutate would leave a stale entry at
		// oldKey: delete first, then mutate, then Put under the new key.
		b.deploymentGroups.Delete(oldKey)
		dg.DeploymentGroupName = newDGName
		b.deploymentGroups.Put(dg)
	}

	return hooksNotCleanedUp, nil
}

// ListDeploymentGroups returns all deployment group names for an application in sorted order.
func (b *InMemoryBackend) ListDeploymentGroups(appName string) ([]string, error) {
	b.mu.RLock("ListDeploymentGroups")
	defer b.mu.RUnlock()

	if !b.applications.Has(appName) {
		return nil, fmt.Errorf("%w: application %s not found", ErrNotFound, appName)
	}

	entries := b.deploymentGroupsByApp.Get(appName)
	names := make([]string, 0, len(entries))
	for _, dg := range entries {
		names = append(names, dg.DeploymentGroupName)
	}

	sort.Strings(names)

	return names, nil
}

// ListDeploymentGroupDetails returns all deployment groups for an application.
func (b *InMemoryBackend) ListDeploymentGroupDetails(appName string) ([]*DeploymentGroup, error) {
	b.mu.RLock("ListDeploymentGroupDetails")
	defer b.mu.RUnlock()

	if !b.applications.Has(appName) {
		return nil, fmt.Errorf("%w: application %s not found", ErrNotFound, appName)
	}

	entries := b.deploymentGroupsByApp.Get(appName)
	list := make([]*DeploymentGroup, 0, len(entries))
	for _, dg := range entries {
		cp := *dg
		list = append(list, &cp)
	}

	return list, nil
}

// DeleteDeploymentGroup deletes a deployment group.
func (b *InMemoryBackend) DeleteDeploymentGroup(appName, dgName string) error {
	b.mu.Lock("DeleteDeploymentGroup")
	defer b.mu.Unlock()

	if !b.applications.Has(appName) {
		// DeleteDeploymentGroup's deserializer models neither
		// ApplicationDoesNotExistException nor DeploymentGroupDoesNotExistException
		// (aws-sdk-go-v2/service/codedeploy deserializers.go) -- both this code and
		// the one below are provably wrong here, but idempotent-success vs. a
		// different code is unconfirmed. Do NOT "fix" this by guessing; needs real
		// evidence (gopherstack-3pz8).
		return fmt.Errorf("%w: application %s not found", ErrNotFound, appName)
	}

	key := dgKey(appName, dgName)

	dg, ok := b.deploymentGroups.Get(key)
	if !ok {
		return fmt.Errorf("%w: deployment group %s not found", ErrDeploymentGroupNotFound, dgName)
	}

	dg.Tags.Close()
	b.deploymentGroups.Delete(key)

	return nil
}

// BatchGetDeploymentGroups returns deployment group info for the given names under an app.
// Groups that do not exist are silently omitted.
func (b *InMemoryBackend) BatchGetDeploymentGroups(appName string, dgNames []string) ([]*DeploymentGroup, error) {
	b.mu.RLock("BatchGetDeploymentGroups")
	defer b.mu.RUnlock()

	if !b.applications.Has(appName) {
		return nil, fmt.Errorf("%w: application %s not found", ErrNotFound, appName)
	}

	result := make([]*DeploymentGroup, 0, len(dgNames))

	for _, name := range dgNames {
		dg, ok := b.deploymentGroups.Get(dgKey(appName, name))
		if !ok {
			continue
		}

		cp := *dg
		result = append(result, &cp)
	}

	return result, nil
}

// AddDeploymentGroupInternal adds a deployment group directly to the backend without validation.
// Used for test seeding only.
func (b *InMemoryBackend) AddDeploymentGroupInternal(dg *DeploymentGroup) {
	b.mu.Lock("AddDeploymentGroupInternal")
	defer b.mu.Unlock()

	dg.Tags = ensureTags(dg.Tags, "codedeploy.dg."+dg.ApplicationName+"."+dg.DeploymentGroupName+".tags")

	if dg.DeploymentGroupID == "" {
		dg.DeploymentGroupID = uuid.NewString()
	}

	b.deploymentGroups.Put(dg)
}

// DeploymentGroupARN builds an ARN for a CodeDeploy deployment group.
func (b *InMemoryBackend) DeploymentGroupARN(appName, dgName string) string {
	return arn.Build("codedeploy", b.region, b.accountID, "deploymentgroup:"+appName+"/"+dgName)
}

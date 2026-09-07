package codedeploy

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// Default config values for built-in deployment configurations.
const (
	defaultHealthyHostPct = 50
	defaultCanaryPct      = 10
	defaultCanaryInterval = 5
	defaultLinearPct      = 10
	defaultLinearInterval = 1
)

// seedDefaultConfigs pre-populates the standard AWS CodeDeploy built-in deployment configurations.
func (b *InMemoryBackend) seedDefaultConfigs() {
	allAtOnce := &TrafficRoutingConfig{Type: "AllAtOnce"}

	defaults := []*DeploymentConfig{
		{
			DeploymentConfigName: "CodeDeployDefault.AllAtOnce",
			ComputePlatform:      computePlatformServer,
			MinimumHealthyHosts:  &MinimumHealthyHosts{Type: "FLEET_PERCENT", Value: 0},
			TrafficRoutingConfig: allAtOnce,
			IsDefault:            true,
		},
		{
			DeploymentConfigName: "CodeDeployDefault.OneAtATime",
			ComputePlatform:      computePlatformServer,
			MinimumHealthyHosts:  &MinimumHealthyHosts{Type: "HOST_COUNT", Value: 1},
			TrafficRoutingConfig: allAtOnce,
			IsDefault:            true,
		},
		{
			DeploymentConfigName: "CodeDeployDefault.HalfAtATime",
			ComputePlatform:      computePlatformServer,
			MinimumHealthyHosts:  &MinimumHealthyHosts{Type: "FLEET_PERCENT", Value: defaultHealthyHostPct},
			TrafficRoutingConfig: allAtOnce,
			IsDefault:            true,
		},
		{
			DeploymentConfigName: "CodeDeployDefault.LambdaAllAtOnce",
			ComputePlatform:      computePlatformLambda,
			TrafficRoutingConfig: allAtOnce,
			IsDefault:            true,
		},
		{
			DeploymentConfigName: "CodeDeployDefault.LambdaCanary10Percent5Minutes",
			ComputePlatform:      computePlatformLambda,
			TrafficRoutingConfig: &TrafficRoutingConfig{
				Type: "TimeBasedCanary",
				TimeBasedCanary: &TimeBasedCanary{
					CanaryPercentage: defaultCanaryPct,
					CanaryInterval:   defaultCanaryInterval,
				},
			},
			IsDefault: true,
		},
		{
			DeploymentConfigName: "CodeDeployDefault.LambdaLinear10PercentEvery1Minute",
			ComputePlatform:      computePlatformLambda,
			TrafficRoutingConfig: &TrafficRoutingConfig{
				Type: "TimeBasedLinear",
				TimeBasedLinear: &TimeBasedLinear{
					LinearPercentage: defaultLinearPct,
					LinearInterval:   defaultLinearInterval,
				},
			},
			IsDefault: true,
		},
		{
			DeploymentConfigName: "CodeDeployDefault.ECSAllAtOnce",
			ComputePlatform:      computePlatformECS,
			TrafficRoutingConfig: allAtOnce,
			IsDefault:            true,
		},
		{
			DeploymentConfigName: "CodeDeployDefault.ECSCanary10Percent5Minutes",
			ComputePlatform:      computePlatformECS,
			TrafficRoutingConfig: &TrafficRoutingConfig{
				Type: "TimeBasedCanary",
				TimeBasedCanary: &TimeBasedCanary{
					CanaryPercentage: defaultCanaryPct,
					CanaryInterval:   defaultCanaryInterval,
				},
			},
			IsDefault: true,
		},
		{
			DeploymentConfigName: "CodeDeployDefault.ECSLinear10PercentEvery1Minute",
			ComputePlatform:      computePlatformECS,
			TrafficRoutingConfig: &TrafficRoutingConfig{
				Type: "TimeBasedLinear",
				TimeBasedLinear: &TimeBasedLinear{
					LinearPercentage: defaultLinearPct,
					LinearInterval:   defaultLinearInterval,
				},
			},
			IsDefault: true,
		},
	}

	now := time.Now().UTC()
	for _, cfg := range defaults {
		cfg.DeploymentConfigID = uuid.NewString()
		cfg.CreateTime = now
		b.deploymentConfigs.Put(cfg)
	}
}

// CreateDeploymentConfig creates a named deployment configuration.
func (b *InMemoryBackend) CreateDeploymentConfig(
	name, computePlatform string,
	minHealthyHosts *MinimumHealthyHosts,
	trafficRouting *TrafficRoutingConfig,
	zonalConfig *ZonalConfig,
) (*DeploymentConfig, error) {
	b.mu.Lock("CreateDeploymentConfig")
	defer b.mu.Unlock()

	if b.deploymentConfigs.Has(name) {
		return nil, fmt.Errorf("%w: deployment config %s already exists", ErrDeploymentConfigAlreadyExists, name)
	}

	if computePlatform == "" {
		computePlatform = computePlatformServer
	}

	if _, ok := validComputePlatforms()[computePlatform]; !ok {
		return nil, fmt.Errorf("%w: invalid computePlatform %q, must be Server, Lambda, or ECS",
			ErrInvalidComputePlatform, computePlatform)
	}

	cfg := &DeploymentConfig{
		DeploymentConfigName: name,
		DeploymentConfigID:   uuid.NewString(),
		ComputePlatform:      computePlatform,
		CreateTime:           time.Now().UTC(),
		MinimumHealthyHosts:  minHealthyHosts,
		TrafficRoutingConfig: trafficRouting,
		ZonalConfig:          zonalConfig,
	}
	b.deploymentConfigs.Put(cfg)

	cp := *cfg

	return &cp, nil
}

// GetDeploymentConfig returns a deployment configuration by name.
func (b *InMemoryBackend) GetDeploymentConfig(name string) (*DeploymentConfig, error) {
	b.mu.RLock("GetDeploymentConfig")
	defer b.mu.RUnlock()

	cfg, ok := b.deploymentConfigs.Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: deployment config %s not found", ErrDeploymentConfigNotFound, name)
	}

	cp := *cfg

	return &cp, nil
}

// ListDeploymentConfigs returns all deployment config names in sorted order.
func (b *InMemoryBackend) ListDeploymentConfigs() []string {
	b.mu.RLock("ListDeploymentConfigs")
	defer b.mu.RUnlock()

	// Table.Snapshot returns entries sorted by primary key, which for
	// deploymentConfigs is DeploymentConfigName, so this is already alphabetical.
	items := b.deploymentConfigs.Snapshot()
	names := make([]string, len(items))
	for i, cfg := range items {
		names[i] = cfg.DeploymentConfigName
	}

	return names
}

// DeleteDeploymentConfig deletes a deployment configuration by name.
// AWS-default configs (IsDefault=true) cannot be deleted.
func (b *InMemoryBackend) DeleteDeploymentConfig(name string) error {
	b.mu.Lock("DeleteDeploymentConfig")
	defer b.mu.Unlock()

	cfg, ok := b.deploymentConfigs.Get(name)
	if !ok {
		// DeleteDeploymentConfig's deserializer models no
		// DeploymentConfigDoesNotExistException (aws-sdk-go-v2/service/codedeploy
		// deserializers.go) -- this code is provably wrong here. InvalidOperationException
		// (used below for the built-in-config case) is a plausible candidate but its
		// description is generic, not evidence this case reuses it; idempotent-success
		// is equally plausible. Do NOT "fix" this by guessing; needs real evidence
		// (gopherstack-3pz8).
		return fmt.Errorf("%w: deployment config %s not found", ErrDeploymentConfigNotFound, name)
	}

	if cfg.IsDefault {
		return fmt.Errorf("%w: cannot delete built-in deployment config %s", ErrDeploymentConfigIsDefault, name)
	}

	for _, dg := range b.deploymentGroups.All() {
		if dg.DeploymentConfigName == name {
			return fmt.Errorf(
				"%w: deployment config %s is used by deployment group %s",
				ErrDeploymentConfigInUse, name, dg.DeploymentGroupName,
			)
		}
	}

	b.deploymentConfigs.Delete(name)

	return nil
}

// AddDeploymentConfigInternal adds a deployment config directly to the backend without validation.
// Used for test seeding only.
func (b *InMemoryBackend) AddDeploymentConfigInternal(cfg *DeploymentConfig) {
	b.mu.Lock("AddDeploymentConfigInternal")
	defer b.mu.Unlock()

	if cfg.DeploymentConfigID == "" {
		cfg.DeploymentConfigID = uuid.NewString()
	}

	if cfg.CreateTime.IsZero() {
		cfg.CreateTime = time.Now().UTC()
	}

	b.deploymentConfigs.Put(cfg)
}

// DeploymentConfigARN builds an ARN for a CodeDeploy deployment configuration.
func (b *InMemoryBackend) DeploymentConfigARN(name string) string {
	return arn.Build("codedeploy", b.region, b.accountID, "deploymentconfig:"+name)
}

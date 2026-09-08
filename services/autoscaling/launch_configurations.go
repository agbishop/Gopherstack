package autoscaling

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
)

// CreateLaunchConfiguration creates a new launch configuration.
func (b *InMemoryBackend) CreateLaunchConfiguration(
	input CreateLaunchConfigurationInput,
) (*LaunchConfiguration, error) {
	b.mu.Lock("CreateLaunchConfiguration")
	defer b.mu.Unlock()

	if b.launchConfigurations.Has(input.LaunchConfigurationName) {
		return nil, fmt.Errorf(
			"%w: launch configuration %q already exists",
			ErrLaunchConfigurationAlreadyExists,
			input.LaunchConfigurationName,
		)
	}

	if input.LaunchConfigurationName == "" {
		return nil, fmt.Errorf("%w: LaunchConfigurationName is required", ErrInvalidParameter)
	}

	lc := &LaunchConfiguration{
		LaunchConfigurationName: input.LaunchConfigurationName,
		LaunchConfigurationARN: fmt.Sprintf(
			"arn:aws:autoscaling:%s:%s:launchConfiguration:%s:launchConfigurationName/%s",
			config.DefaultRegion, config.DefaultAccountID, uuid.NewString(), input.LaunchConfigurationName,
		),
		ImageID:                      input.ImageID,
		InstanceType:                 input.InstanceType,
		KeyName:                      input.KeyName,
		IAMInstanceProfile:           input.IAMInstanceProfile,
		UserData:                     input.UserData,
		KernelID:                     input.KernelID,
		RamdiskID:                    input.RamdiskID,
		SpotPrice:                    input.SpotPrice,
		PlacementTenancy:             input.PlacementTenancy,
		ClassicLinkVPCID:             input.ClassicLinkVPCID,
		SecurityGroups:               input.SecurityGroups,
		ClassicLinkVPCSecurityGroups: input.ClassicLinkVPCSecurityGroups,
		BlockDeviceMappings:          input.BlockDeviceMappings,
		AssociatePublicIPAddress:     input.AssociatePublicIPAddress,
		EbsOptimized:                 input.EbsOptimized,
		InstanceMonitoring:           input.InstanceMonitoring,
		CreatedTime:                  time.Now(),
	}

	b.launchConfigurations.Put(lc)

	cp := *lc

	return &cp, nil
}

// DescribeLaunchConfigurations returns launch configurations, optionally filtered by name.
func (b *InMemoryBackend) DescribeLaunchConfigurations(names []string) ([]LaunchConfiguration, error) {
	b.mu.RLock("DescribeLaunchConfigurations")
	defer b.mu.RUnlock()

	return describeByNames(b.launchConfigurations, names, ErrLaunchConfigurationNotFound,
		func(a, c *LaunchConfiguration) bool {
			return a.LaunchConfigurationName < c.LaunchConfigurationName
		})
}

// DeleteLaunchConfiguration removes a launch configuration by name.
// api_op_DeleteLaunchConfiguration.go: "The launch configuration must not be
// attached to an Auto Scaling group".
func (b *InMemoryBackend) DeleteLaunchConfiguration(name string) error {
	b.mu.Lock("DeleteLaunchConfiguration")
	defer b.mu.Unlock()

	if !b.launchConfigurations.Has(name) {
		return fmt.Errorf("%w: %q", ErrLaunchConfigurationNotFound, name)
	}

	for _, g := range b.groups.All() {
		if g.LaunchConfigurationName == name {
			return fmt.Errorf("%w: launch configuration %q is still attached to Auto Scaling group %q",
				ErrLaunchConfigurationInUse, name, g.AutoScalingGroupName)
		}
	}

	b.launchConfigurations.Delete(name)

	return nil
}

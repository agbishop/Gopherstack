package autoscaling

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
)

// InstanceLaunchSpec carries the EC2 launch parameters an Auto Scaling group
// derives from its LaunchConfiguration (LaunchTemplate/MixedInstancesPolicy
// support is a follow-up — see EC2Launcher doc comment) for a single scale-out
// batch, passed to an EC2Launcher so it can create real backing EC2 instances.
type InstanceLaunchSpec struct {
	Tags             map[string]string
	ImageID          string
	InstanceType     string
	SubnetID         string
	AvailabilityZone string
	KeyName          string
	SecurityGroups   []string
}

// EC2Launcher lets the Auto Scaling backend create and destroy real (mock) EC2
// instances backing an Auto Scaling group's members, so instances an ASG
// scales out are visible to EC2 DescribeInstances and instances it scales in
// are actually terminated there too. When no EC2Launcher is configured (the
// default), the backend falls back to its historical behavior of fabricating
// instance IDs with no EC2-side record — see SetEC2Launcher.
type EC2Launcher interface {
	// LaunchInstances launches count instances per spec and returns the real
	// EC2 instance IDs created. A partial success (fewer IDs than count) is
	// treated as the actual outcome, not an error.
	LaunchInstances(ctx context.Context, spec InstanceLaunchSpec, count int) ([]string, error)
	// TerminateInstances terminates the given real EC2 instance IDs.
	TerminateInstances(ctx context.Context, ids []string) error
	// ResolveLaunchTemplate resolves a LaunchTemplate ID or name (and version) to its ImageID and InstanceType.
	ResolveLaunchTemplate(ctx context.Context, id, name, version string) (imageID, instanceType string, err error)
}

// SetEC2Launcher wires an EC2Launcher so subsequent scale-out/scale-in
// operations launch and terminate real (mock) EC2 instances instead of
// fabricating instance IDs. Passing nil restores the historical,
// fabrication-only behavior. Intended to be called once during service
// wiring, before the backend serves traffic.
func (b *InMemoryBackend) SetEC2Launcher(l EC2Launcher) {
	b.mu.Lock("SetEC2Launcher")
	defer b.mu.Unlock()
	b.ec2Launcher = l
}

// makeInstances creates count new Instance records belonging to g: real (mock)
// EC2 instances launched via b.ec2Launcher when one is configured and g's
// launch configuration resolves to a usable spec, or synthetic fabricated
// instance IDs otherwise (the historical, EC2-less behavior). It does not
// mutate g or b.instanceIndex; callers remain responsible for both, exactly as
// they were with the pre-existing free-function makeInstances/adjustInstances
// helpers this replaces. Must be called with b.mu held (write lock).
func (b *InMemoryBackend) makeInstances(g *AutoScalingGroup, count int32) []Instance {
	n := max(0, min(maxDesiredCapacity, int(count)))
	if n == 0 {
		return []Instance{}
	}

	az := defaultAvailabilityZone
	if len(g.AvailabilityZones) > 0 {
		az = g.AvailabilityZones[0]
	}

	instanceType := lcInstanceType(b.launchConfigurations, g.LaunchConfigurationName)

	if instances, ok := b.launchInEC2(g, az, n, instanceType); ok {
		return instances
	}

	instances := fabricateInstances(n, az, g.LaunchConfigurationName, instanceType)
	b.registerELBTargets(instanceIDsOf(instances), g.TargetGroupARNs)
	b.registerELBInstances(instanceIDsOf(instances), g.LoadBalancerNames)

	return instances
}

// launchInEC2 attempts to launch n real instances in the wired EC2 launcher.
// Reports ok=false on any error or missing spec so the caller can fall back to fabrication.
func (b *InMemoryBackend) launchInEC2(
	g *AutoScalingGroup, az string, n int, instanceType string,
) ([]Instance, bool) {
	if b.ec2Launcher == nil {
		return nil, false
	}

	spec, ok := b.launchSpecForGroup(g, az)
	if !ok {
		return nil, false
	}

	ids, err := b.ec2Launcher.LaunchInstances(context.Background(), spec, n)
	if err != nil {
		logger.Load(context.Background()).Error(
			"autoscaling: EC2 launch failed, falling back to synthetic instances",
			"error", err, "group", g.AutoScalingGroupName)

		return nil, false
	}

	if instanceType == "" {
		instanceType = spec.InstanceType
	}

	instances := instancesFromIDs(ids, az, g.LaunchConfigurationName, instanceType)
	b.registerELBTargets(ids, g.TargetGroupARNs)
	b.registerELBInstances(ids, g.LoadBalancerNames)

	return instances, true
}

// adjustInstances adjusts g's existing instance slice to match the new desired
// count, adding (real or fabricated — see makeInstances) or removing
// instances from the end while preserving existing instance IDs. Must be
// called with b.mu held (write lock).
func (b *InMemoryBackend) adjustInstances(g *AutoScalingGroup, existing []Instance, desired int32) []Instance {
	current := len(existing)
	want := int(desired)

	if want == current {
		return existing
	}

	if want < current {
		return existing[:want]
	}

	// Add new instances for the delta.
	delta := desired - int32(current) //nolint:gosec // current <= math.MaxInt32 (bounded by desired which is int32)

	return append(existing, b.makeInstances(g, delta)...)
}

// terminateInEC2 best-effort terminates ids in the wired EC2 backend so EC2
// DescribeInstances stops listing instances this ASG has removed. No-op when
// no EC2Launcher is configured (the historical, EC2-less behavior). Errors
// are logged, not propagated: by the time this runs the instance has already
// been removed from the group's own bookkeeping (mirroring how AWS itself
// terminates the backing EC2 instance asynchronously, after the ASG-side
// state change), so there is nothing left to roll back. Must be called with
// b.mu held (write lock) — matches every existing call site.
func (b *InMemoryBackend) terminateInEC2(ids []string) {
	if b.ec2Launcher == nil || len(ids) == 0 {
		return
	}

	if err := b.ec2Launcher.TerminateInstances(context.Background(), ids); err != nil {
		logger.Load(context.Background()).Error(
			"autoscaling: EC2 terminate failed", "error", err, "instanceIDs", ids)
	}
}

// launchSpecForGroup derives an InstanceLaunchSpec from g's LaunchConfiguration,
// LaunchTemplate, or MixedInstancesPolicy. It reports ok=false when g cannot be
// resolved to a usable ImageId, in which case the caller falls back to fabricating
// instances.
func (b *InMemoryBackend) launchSpecForGroup(g *AutoScalingGroup, az string) (InstanceLaunchSpec, bool) {
	if g.LaunchConfigurationName != "" {
		lc, ok := b.launchConfigurations.Get(g.LaunchConfigurationName)
		if !ok || lc.ImageID == "" {
			return InstanceLaunchSpec{}, false
		}

		return InstanceLaunchSpec{
			ImageID:          lc.ImageID,
			InstanceType:     lc.InstanceType,
			SubnetID:         firstSubnetID(g.VPCZoneIdentifier),
			AvailabilityZone: az,
			KeyName:          lc.KeyName,
			SecurityGroups:   lc.SecurityGroups,
			Tags:             launchTagsForGroup(g),
		}, true
	}

	if b.ec2Launcher == nil {
		return InstanceLaunchSpec{}, false
	}

	ltSpec := extractLaunchTemplateSpec(g)
	if ltSpec == nil || (ltSpec.LaunchTemplateID == "" && ltSpec.LaunchTemplateName == "") {
		return InstanceLaunchSpec{}, false
	}

	imageID, instanceType, err := b.ec2Launcher.ResolveLaunchTemplate(
		context.Background(),
		ltSpec.LaunchTemplateID,
		ltSpec.LaunchTemplateName,
		ltSpec.Version,
	)
	if err != nil || imageID == "" {
		return InstanceLaunchSpec{}, false
	}

	if override := extractOverrideInstanceType(g); override != "" {
		instanceType = override
	}

	return InstanceLaunchSpec{
		ImageID:          imageID,
		InstanceType:     instanceType,
		SubnetID:         firstSubnetID(g.VPCZoneIdentifier),
		AvailabilityZone: az,
		Tags:             launchTagsForGroup(g),
	}, true
}

func extractLaunchTemplateSpec(g *AutoScalingGroup) *LaunchTemplateSpecification {
	if g.LaunchTemplate != nil {
		return g.LaunchTemplate
	}
	if g.MixedInstancesPolicy != nil {
		return &g.MixedInstancesPolicy.LaunchTemplate.LaunchTemplateSpecification
	}

	return nil
}

func extractOverrideInstanceType(g *AutoScalingGroup) string {
	if g.MixedInstancesPolicy == nil {
		return ""
	}
	for _, o := range g.MixedInstancesPolicy.LaunchTemplate.Overrides {
		if o.InstanceType != "" {
			return o.InstanceType
		}
	}

	return ""
}

// firstSubnetID returns the first subnet ID from a VPCZoneIdentifier
// (AWS represents this as a comma-separated list), or "" if unset.
func firstSubnetID(vpcZoneIdentifier string) string {
	if vpcZoneIdentifier == "" {
		return ""
	}

	first, _, _ := strings.Cut(vpcZoneIdentifier, ",")

	return strings.TrimSpace(first)
}

// launchTagsForGroup builds the tag set applied to instances g launches:
// every group tag with PropagateAtLaunch set, plus the synthetic
// "aws:autoscaling:groupName" tag AWS itself attaches to ASG-managed instances.
func launchTagsForGroup(g *AutoScalingGroup) map[string]string {
	tags := make(map[string]string, len(g.Tags)+1)
	tags["aws:autoscaling:groupName"] = g.AutoScalingGroupName

	for _, t := range g.Tags {
		if t.PropagateAtLaunch {
			tags[t.Key] = t.Value
		}
	}

	return tags
}

// fabricateInstances creates n Instance records with synthetic, EC2-less
// instance IDs. This is the pre-EC2Launcher behavior, preserved verbatim as
// the fallback used when no launcher is configured or a group's launch
// configuration can't be resolved to a usable EC2 launch spec.
func fabricateInstances(n int, az, launchConfigName, instanceType string) []Instance {
	// No capacity hint — append grows naturally; any user-derived value in
	// the capacity position would trigger CodeQL go/slice-memory-allocation-excessive-size.
	//nolint:prealloc,nolintlint // satisfies CodeQL by removing tainted capacity hint
	instances := make([]Instance, 0)

	now := time.Now()
	for range n {
		// Use full UUID (stripped of dashes) to generate a unique, collision-free instance ID.
		id := "i-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:17]
		instances = append(instances, Instance{
			InstanceID:              id,
			AvailabilityZone:        az,
			LifecycleState:          lifecycleStateInService,
			HealthStatus:            healthStatusHealthy,
			LaunchConfigurationName: launchConfigName,
			InstanceType:            instanceType,
			LaunchTime:              now,
		})
	}

	return instances
}

// instancesFromIDs creates one Instance record per real EC2 instance ID
// returned by an EC2Launcher, in the same InService/Healthy shape
// fabricateInstances produces (any lifecycle-hook gating is applied afterward
// by the caller, exactly as it is for fabricated instances).
func instancesFromIDs(ids []string, az, launchConfigName, instanceType string) []Instance {
	now := time.Now()
	instances := make([]Instance, 0, len(ids))

	for _, id := range ids {
		instances = append(instances, Instance{
			InstanceID:              id,
			AvailabilityZone:        az,
			LifecycleState:          lifecycleStateInService,
			HealthStatus:            healthStatusHealthy,
			LaunchConfigurationName: launchConfigName,
			InstanceType:            instanceType,
			LaunchTime:              now,
		})
	}

	return instances
}

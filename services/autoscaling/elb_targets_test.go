package autoscaling_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/autoscaling"
)

// fakeELBRegistrar is a minimal autoscaling.ELBInstanceRegistrar fake that
// records every Register/Deregister call so tests can assert on classic ELB
// side effects without a real elb backend.
type fakeELBRegistrar struct {
	registerErr  error
	registered   []elbRegistrarCall
	deregistered []elbRegistrarCall
	mu           sync.Mutex
}

type elbRegistrarCall struct {
	loadBalancerName string
	ids              []string
}

func (f *fakeELBRegistrar) RegisterInstances(_ context.Context, lbName string, instanceIDs []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.registered = append(f.registered, elbRegistrarCall{loadBalancerName: lbName, ids: instanceIDs})

	return f.registerErr
}

func (f *fakeELBRegistrar) DeregisterInstances(_ context.Context, lbName string, instanceIDs []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.deregistered = append(f.deregistered, elbRegistrarCall{loadBalancerName: lbName, ids: instanceIDs})

	return nil
}

// registeredIDsFor returns the union of instance IDs registered against
// lbName across all RegisterInstances calls observed so far.
func (f *fakeELBRegistrar) registeredIDsFor(lbName string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	var ids []string

	for _, c := range f.registered {
		if c.loadBalancerName == lbName {
			ids = append(ids, c.ids...)
		}
	}

	return ids
}

func (f *fakeELBRegistrar) deregisteredIDsFor(lbName string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	var ids []string

	for _, c := range f.deregistered {
		if c.loadBalancerName == lbName {
			ids = append(ids, c.ids...)
		}
	}

	return ids
}

const testLBName = "classic-lb-1"

// newLBGroup creates an ASG (with a launch configuration and no EC2Launcher,
// so instances are fabricated) whose LoadBalancerNames includes testLBName.
func newLBGroup(t *testing.T, b *autoscaling.InMemoryBackend, name string, desired int32) {
	t.Helper()

	_, err := b.CreateLaunchConfiguration(autoscaling.CreateLaunchConfigurationInput{
		LaunchConfigurationName: name + "-lc",
		ImageID:                 "ami-12345678",
		InstanceType:            "t3.micro",
	})
	require.NoError(t, err)

	_, err = b.CreateAutoScalingGroup(autoscaling.CreateAutoScalingGroupInput{
		AutoScalingGroupName:    name,
		LaunchConfigurationName: name + "-lc",
		MinSize:                 0,
		MaxSize:                 10,
		DesiredCapacity:         desired,
		AvailabilityZones:       []string{"us-east-1a"},
		LoadBalancerNames:       []string{testLBName},
	})
	require.NoError(t, err)
}

func TestInMemoryBackend_ELBRegistrar_NoRegistrar_NoEffect(t *testing.T) {
	t.Parallel()

	b := autoscaling.NewInMemoryBackend()
	t.Cleanup(b.Close)

	// No SetELBRegistrar call: must behave exactly as before this feature,
	// i.e. not panic and not attempt any registration.
	newLBGroup(t, b, "asg-no-elb-registrar", 2)

	require.NoError(t, b.SetDesiredCapacity("asg-no-elb-registrar", 4))
	_, err := b.TerminateInstanceInAutoScalingGroup(mustFirstInstanceID(t, b, "asg-no-elb-registrar"), false)
	require.NoError(t, err)
}

func TestInMemoryBackend_ELBRegistrar_CreateGroupRegisters(t *testing.T) {
	t.Parallel()

	b := autoscaling.NewInMemoryBackend()
	t.Cleanup(b.Close)

	reg := &fakeELBRegistrar{}
	b.SetELBRegistrar(reg)

	newLBGroup(t, b, "asg-elb-create", 2)

	assert.Len(t, reg.registeredIDsFor(testLBName), 2)
}

func TestInMemoryBackend_ELBRegistrar_ScaleOutRegistersNewInstances(t *testing.T) {
	t.Parallel()

	b := autoscaling.NewInMemoryBackend()
	t.Cleanup(b.Close)

	reg := &fakeELBRegistrar{}
	b.SetELBRegistrar(reg)

	newLBGroup(t, b, "asg-elb-scale-out", 1)
	require.Len(t, reg.registeredIDsFor(testLBName), 1)

	require.NoError(t, b.SetDesiredCapacity("asg-elb-scale-out", 3))

	assert.Len(t, reg.registeredIDsFor(testLBName), 3)
}

func TestInMemoryBackend_ELBRegistrar_ScaleInDeregisters(t *testing.T) {
	t.Parallel()

	b := autoscaling.NewInMemoryBackend()
	t.Cleanup(b.Close)

	reg := &fakeELBRegistrar{}
	b.SetELBRegistrar(reg)

	newLBGroup(t, b, "asg-elb-scale-in", 3)
	require.NoError(t, b.SetDesiredCapacity("asg-elb-scale-in", 1))

	assert.Len(t, reg.deregisteredIDsFor(testLBName), 2)
}

func TestInMemoryBackend_ELBRegistrar_TerminateInstanceDeregisters(t *testing.T) {
	t.Parallel()

	b := autoscaling.NewInMemoryBackend()
	t.Cleanup(b.Close)

	reg := &fakeELBRegistrar{}
	b.SetELBRegistrar(reg)

	newLBGroup(t, b, "asg-elb-terminate", 2)
	id := mustFirstInstanceID(t, b, "asg-elb-terminate")

	_, err := b.TerminateInstanceInAutoScalingGroup(id, false)
	require.NoError(t, err)

	assert.Contains(t, reg.deregisteredIDsFor(testLBName), id)
}

func TestInMemoryBackend_ELBRegistrar_AttachDetachInstances(t *testing.T) {
	t.Parallel()

	b := autoscaling.NewInMemoryBackend()
	t.Cleanup(b.Close)

	reg := &fakeELBRegistrar{}
	b.SetELBRegistrar(reg)

	newLBGroup(t, b, "asg-elb-attach", 0)

	require.NoError(t, b.AttachInstances("asg-elb-attach", []string{"i-manual1", "i-manual2"}))
	assert.ElementsMatch(t, []string{"i-manual1", "i-manual2"}, reg.registeredIDsFor(testLBName))

	_, err := b.DetachInstances("asg-elb-attach", []string{"i-manual1"}, false)
	require.NoError(t, err)
	assert.Contains(t, reg.deregisteredIDsFor(testLBName), "i-manual1")
	assert.NotContains(t, reg.deregisteredIDsFor(testLBName), "i-manual2")
}

func TestInMemoryBackend_ELBRegistrar_AttachDetachLoadBalancers(t *testing.T) {
	t.Parallel()

	const secondLB = "classic-lb-2"

	b := autoscaling.NewInMemoryBackend()
	t.Cleanup(b.Close)

	reg := &fakeELBRegistrar{}
	b.SetELBRegistrar(reg)

	newLBGroup(t, b, "asg-elb-lb-attach", 2)
	reg.registered = nil // reset from CreateAutoScalingGroup

	// Attaching a second load balancer registers the group's existing instances.
	require.NoError(t, b.AttachLoadBalancers("asg-elb-lb-attach", []string{secondLB}))
	assert.Len(t, reg.registeredIDsFor(secondLB), 2)
	assert.Empty(t, reg.registeredIDsFor(testLBName)) // untouched, already-attached LB is not re-registered

	// Detaching the original load balancer deregisters the group's instances from it.
	require.NoError(t, b.DetachLoadBalancers("asg-elb-lb-attach", []string{testLBName}))
	assert.Len(t, reg.deregisteredIDsFor(testLBName), 2)
	assert.Empty(t, reg.deregisteredIDsFor(secondLB))
}

func TestInMemoryBackend_ELBRegistrar_EC2LauncherScaleOutRegisters(t *testing.T) {
	t.Parallel()

	b := autoscaling.NewInMemoryBackend()
	t.Cleanup(b.Close)

	reg := &fakeELBRegistrar{}
	b.SetELBRegistrar(reg)
	b.SetEC2Launcher(&fakeEC2Launcher{})

	newLBGroup(t, b, "asg-elb-ec2-launcher", 2)

	registered := reg.registeredIDsFor(testLBName)
	assert.Len(t, registered, 2)

	for _, id := range registered {
		assert.Contains(t, id, "ec2fake")
	}
}

func TestInMemoryBackend_ELBRegistrar_LifecycleHookTerminationDeregisters(t *testing.T) {
	t.Parallel()

	b := autoscaling.NewInMemoryBackend()
	t.Cleanup(b.Close)

	reg := &fakeELBRegistrar{}
	b.SetELBRegistrar(reg)

	newLBGroup(t, b, "asg-elb-hook-terminate", 1)
	id := mustFirstInstanceID(t, b, "asg-elb-hook-terminate")

	require.NoError(t, b.PutLifecycleHook(autoscaling.LifecycleHook{
		LifecycleHookName:    "term-hook",
		AutoScalingGroupName: "asg-elb-hook-terminate",
		LifecycleTransition:  "autoscaling:EC2_INSTANCE_TERMINATING",
		HeartbeatTimeout:     300,
	}))

	_, err := b.TerminateInstanceInAutoScalingGroup(id, false)
	require.NoError(t, err)

	// Paused in Terminating:Wait for the hook: no deregistration yet.
	assert.Empty(t, reg.deregisteredIDsFor(testLBName))

	require.NoError(t, b.CompleteLifecycleAction(autoscaling.CompleteLifecycleActionInput{
		AutoScalingGroupName:  "asg-elb-hook-terminate",
		LifecycleHookName:     "term-hook",
		InstanceID:            id,
		LifecycleActionResult: "CONTINUE",
	}))

	assert.Contains(t, reg.deregisteredIDsFor(testLBName), id)
}

func TestInMemoryBackend_ELBRegistrar_RegisterErrorDoesNotFailCall(t *testing.T) {
	t.Parallel()

	b := autoscaling.NewInMemoryBackend()
	t.Cleanup(b.Close)

	reg := &fakeELBRegistrar{registerErr: assert.AnError}
	b.SetELBRegistrar(reg)

	// Best-effort registration: a registrar error must not fail the ASG-side
	// operation or leave the group instance list inconsistent.
	newLBGroup(t, b, "asg-elb-reg-err", 2)

	groups, err := b.DescribeAutoScalingGroups([]string{"asg-elb-reg-err"}, nil)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.Len(t, groups[0].Instances, 2)
}

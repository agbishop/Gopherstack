package codedeploy_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/services/codedeploy"
	ec2backend "github.com/blackbirdworks/gopherstack/services/ec2"
)

// fakeSiblingServices structurally satisfies codedeploy's unexported
// siblingServices interface (matched by SetAppConfig's type assertion),
// mirroring how the real *CLI wires GetEC2Handler.
type fakeSiblingServices struct {
	ec2Handler service.Registerable
}

func (f *fakeSiblingServices) GetEC2Handler() service.Registerable { return f.ec2Handler }

// TestDeploymentTargets_Ec2TagFilters_RealInstances proves
// deploymentTargets resolves real services/ec2 instances matched by a
// deployment group's Ec2TagFilters, once the EC2 backend is wired via
// SetAppConfig -- not the zero targets this backend previously always
// returned for EC2 targeting, regardless of wiring.
func TestDeploymentTargets_Ec2TagFilters_RealInstances(t *testing.T) {
	t.Parallel()

	ec2Bk := ec2backend.NewInMemoryBackend(config.DefaultAccountID, config.DefaultRegion)
	ec2Handler := ec2backend.NewHandler(ec2Bk)

	h := codedeploy.NewHandler(codedeploy.NewInMemoryBackend(config.DefaultAccountID, config.DefaultRegion))
	h.Backend.SetAppConfig(&fakeSiblingServices{ec2Handler: ec2Handler})

	matchedInstances, err := ec2Bk.RunInstances("ami-fake", "t3.micro", "", 2)
	require.NoError(t, err)
	require.NoError(t, ec2Bk.CreateTags(
		[]string{matchedInstances[0].ID, matchedInstances[1].ID},
		map[string]string{"env": "prod"},
	))

	noMatchInstances, err := ec2Bk.RunInstances("ami-fake", "t3.micro", "", 1)
	require.NoError(t, err)
	require.NoError(t, ec2Bk.CreateTags([]string{noMatchInstances[0].ID}, map[string]string{"env": "dev"}))

	_, err = h.Backend.CreateApplication("my-app", "Server", nil)
	require.NoError(t, err)

	_, err = h.Backend.CreateDeploymentGroup("my-app", "my-dg", codedeploy.DeploymentGroupInput{
		ServiceRoleArn: "arn:aws:iam::000000000000:role/role",
		Ec2TagFilters: []codedeploy.TagFilter{
			{Key: "env", Value: "prod", Type: "KEY_AND_VALUE"},
		},
	}, nil)
	require.NoError(t, err)

	d, err := h.Backend.CreateDeployment("my-app", "my-dg", codedeploy.DeploymentOptions{Creator: "user"})
	require.NoError(t, err)

	targetIDs, err := h.Backend.ListDeploymentTargets(d.DeploymentID, codedeploy.TargetListFilter{})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{matchedInstances[0].ID, matchedInstances[1].ID}, targetIDs)

	target, err := h.Backend.GetDeploymentTarget(d.DeploymentID, matchedInstances[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "Succeeded", target.Status)
	assert.Equal(t, "BLUE", target.InstanceLabel)
	assert.Contains(t, target.TargetArn, "arn:aws:ec2:")
}

// TestDeploymentTargets_Ec2TagFilters_NoEc2Wired proves the pre-existing
// documented simplification still holds when the EC2 backend isn't wired
// (e.g. constructing InMemoryBackend directly, as most unit tests do):
// Ec2TagFilters resolves zero targets rather than erroring.
func TestDeploymentTargets_Ec2TagFilters_NoEc2Wired(t *testing.T) {
	t.Parallel()

	h := codedeploy.NewHandler(codedeploy.NewInMemoryBackend(config.DefaultAccountID, config.DefaultRegion))

	_, err := h.Backend.CreateApplication("my-app", "Server", nil)
	require.NoError(t, err)

	_, err = h.Backend.CreateDeploymentGroup("my-app", "my-dg", codedeploy.DeploymentGroupInput{
		ServiceRoleArn: "arn:aws:iam::000000000000:role/role",
		Ec2TagFilters: []codedeploy.TagFilter{
			{Key: "env", Value: "prod", Type: "KEY_AND_VALUE"},
		},
	}, nil)
	require.NoError(t, err)

	d, err := h.Backend.CreateDeployment("my-app", "my-dg", codedeploy.DeploymentOptions{Creator: "user"})
	require.NoError(t, err)

	targetIDs, err := h.Backend.ListDeploymentTargets(d.DeploymentID, codedeploy.TargetListFilter{})
	require.NoError(t, err)
	assert.Empty(t, targetIDs)
}

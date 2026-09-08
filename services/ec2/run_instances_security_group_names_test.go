package ec2_test

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/ec2"
)

// TestRunInstances_SecurityGroupNames covers RunInstances' SecurityGroup.N
// (group-name) parameter (gopherstack-2mk2). Real RunInstancesInput.SecurityGroups
// is documented "[Default VPC] The names of the security groups"
// (ec2@v1.319.1 api_op_RunInstances.go) -- a name resolves only in the
// default VPC; supplying one for a subnet in a non-default VPC is rejected.
func TestRunInstances_SecurityGroupNames(t *testing.T) {
	t.Parallel()

	t.Run("name resolves in default VPC", func(t *testing.T) {
		t.Parallel()

		bk := ec2.NewInMemoryBackend("123456789012", "us-east-1")
		h := ec2.NewHandler(bk)
		h.AccountID = "123456789012"
		h.Region = "us-east-1"

		sg, err := bk.CreateSecurityGroup("named-sg", "named sg", "vpc-default")
		require.NoError(t, err)

		runVals := url.Values{
			"Action":          {"RunInstances"},
			"Version":         {"2016-11-15"},
			"ImageId":         {"ami-123"},
			"InstanceType":    {"t3.micro"},
			"MinCount":        {"1"},
			"MaxCount":        {"1"},
			"SecurityGroup.1": {"named-sg"},
		}

		resp, err := ec2.ExportDispatch(h, runVals)
		require.NoError(t, err)
		assert.Contains(t, resp, sg.ID)
	})

	t.Run("name rejected for subnet in non-default VPC", func(t *testing.T) {
		t.Parallel()

		bk := ec2.NewInMemoryBackend("123456789012", "us-east-1")
		h := ec2.NewHandler(bk)
		h.AccountID = "123456789012"
		h.Region = "us-east-1"

		vpc, err := bk.CreateVpc("10.0.0.0/16", "default")
		require.NoError(t, err)
		subnet, err := bk.CreateSubnet(vpc.ID, "10.0.1.0/24", "us-east-1a")
		require.NoError(t, err)
		_, err = bk.CreateSecurityGroup("named-sg", "named sg", vpc.ID)
		require.NoError(t, err)

		runVals := url.Values{
			"Action":          {"RunInstances"},
			"Version":         {"2016-11-15"},
			"ImageId":         {"ami-123"},
			"InstanceType":    {"t3.micro"},
			"MinCount":        {"1"},
			"MaxCount":        {"1"},
			"SubnetId":        {subnet.ID},
			"SecurityGroup.1": {"named-sg"},
		}

		_, err = ec2.ExportDispatch(h, runVals)
		require.Error(t, err)
		require.ErrorIs(t, err, ec2.ErrInvalidParameterCombination)
	})

	t.Run("ids still work exactly as before", func(t *testing.T) {
		t.Parallel()

		bk := ec2.NewInMemoryBackend("123456789012", "us-east-1")
		h := ec2.NewHandler(bk)
		h.AccountID = "123456789012"
		h.Region = "us-east-1"

		vpc, err := bk.CreateVpc("10.1.0.0/16", "default")
		require.NoError(t, err)
		sg, err := bk.CreateSecurityGroup("app-sg", "App SG", vpc.ID)
		require.NoError(t, err)

		runVals := url.Values{
			"Action":            {"RunInstances"},
			"Version":           {"2016-11-15"},
			"ImageId":           {"ami-123"},
			"InstanceType":      {"t3.micro"},
			"MinCount":          {"1"},
			"MaxCount":          {"1"},
			"SecurityGroupId.1": {sg.ID},
		}

		resp, err := ec2.ExportDispatch(h, runVals)
		require.NoError(t, err)
		assert.Contains(t, resp, sg.ID)
	})
}

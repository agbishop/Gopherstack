package ec2_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/ec2"
)

// TestDescribeSecurityGroupRules verifies SG rule listing.
func TestDescribeSecurityGroupRules(t *testing.T) {
	t.Parallel()

	b := ec2.NewInMemoryBackend("123456789012", "us-east-1")

	sg, err := b.CreateSecurityGroup("test-sg", "testing", "vpc-default")
	require.NoError(t, err)

	require.NoError(t, b.AuthorizeSecurityGroupIngress(sg.ID, []ec2.SecurityGroupRule{
		{Protocol: "tcp", IPRange: "0.0.0.0/0", FromPort: 80, ToPort: 80},
	}))

	rules, err := b.DescribeSecurityGroupRules(sg.ID)
	require.NoError(t, err)
	// 2 = 1 ingress rule added above + 1 default egress allow-all rule.
	require.Len(t, rules, 2)
	// DescribeSecurityGroupRules appends ingress first.
	assert.False(t, rules[0].IsEgress)
	assert.Equal(t, "tcp", rules[0].Protocol)
}

// TestModifySecurityGroupRules verifies SG rule replacement.

// TestModifySecurityGroupRules verifies SG rule replacement.
func TestModifySecurityGroupRules(t *testing.T) {
	t.Parallel()

	b := ec2.NewInMemoryBackend("123456789012", "us-east-1")

	sg, err := b.CreateSecurityGroup("test-sg", "testing", "vpc-default")
	require.NoError(t, err)

	require.NoError(t, b.AuthorizeSecurityGroupIngress(sg.ID, []ec2.SecurityGroupRule{
		{Protocol: "tcp", IPRange: "0.0.0.0/0", FromPort: 80, ToPort: 80},
	}))

	// replace with port 443
	require.NoError(t, b.ModifySecurityGroupRules(sg.ID, []ec2.SecurityGroupRule{
		{Protocol: "tcp", IPRange: "0.0.0.0/0", FromPort: 443, ToPort: 443},
	}, false))

	rules, err := b.DescribeSecurityGroupRules(sg.ID)
	require.NoError(t, err)
	// 2 = 1 modified ingress rule + 1 default egress allow-all rule.
	require.Len(t, rules, 2)
	// Ingress rules are returned first.
	assert.Equal(t, 443, rules[0].FromPort)
}

// TestDeleteLaunchTemplate verifies launch template deletion.

func TestSecurityGroupRule_SGReference(t *testing.T) {
	t.Parallel()

	b := ec2.NewInMemoryBackend("123456789012", "us-east-1")

	sg1, err := b.CreateSecurityGroup("sg-source", "source sg", "vpc-default")
	require.NoError(t, err)

	sg2, err := b.CreateSecurityGroup("sg-target", "target sg", "vpc-default")
	require.NoError(t, err)

	rule := ec2.SecurityGroupRule{
		Protocol:      "tcp",
		FromPort:      80,
		ToPort:        80,
		SourceGroupID: sg1.ID,
	}

	err = b.AuthorizeSecurityGroupIngress(sg2.ID, []ec2.SecurityGroupRule{rule})
	require.NoError(t, err)

	sgs := b.DescribeSecurityGroups([]string{sg2.ID})
	require.Len(t, sgs, 1)
	require.Len(t, sgs[0].IngressRules, 1)
	assert.Equal(t, sg1.ID, sgs[0].IngressRules[0].SourceGroupID)
}

// TestDeleteSecurityGroup_DependencyViolation verifies AWS's DeleteSecurityGroup
// guard: a group attached to an instance or referenced by another group's
// rules in the same VPC cannot be deleted until the dependency is cleared.
func TestDeleteSecurityGroup_DependencyViolation(t *testing.T) {
	t.Parallel()

	t.Run("attached_to_instance", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend()

		sg, err := b.CreateSecurityGroup("in-use-sg", "testing", "vpc-default")
		require.NoError(t, err)

		instances, err := b.RunInstances("ami-test", "t3.micro", "", 1)
		require.NoError(t, err)
		require.NoError(t, b.SetInstanceLaunchConfig(instances[0].ID, "", []string{sg.ID}))

		err = b.DeleteSecurityGroup(sg.ID)
		require.ErrorIs(t, err, ec2.ErrDependencyViolation)
		assert.NotEmpty(t, b.DescribeSecurityGroups([]string{sg.ID}),
			"security group must survive a failed DeleteSecurityGroup")

		_, err = b.TerminateInstances([]string{instances[0].ID})
		require.NoError(t, err)
		b.TickLifecycleForTest() // shutting-down -> terminated

		require.NoError(t, b.DeleteSecurityGroup(sg.ID))
	})

	t.Run("referenced_by_other_group", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend()

		source, err := b.CreateSecurityGroup("source-sg", "testing", "vpc-default")
		require.NoError(t, err)

		target, err := b.CreateSecurityGroup("target-sg", "testing", "vpc-default")
		require.NoError(t, err)

		require.NoError(t, b.AuthorizeSecurityGroupIngress(target.ID, []ec2.SecurityGroupRule{
			{Protocol: "tcp", FromPort: 80, ToPort: 80, SourceGroupID: source.ID},
		}))

		err = b.DeleteSecurityGroup(source.ID)
		require.ErrorIs(t, err, ec2.ErrDependencyViolation)
		assert.NotEmpty(t, b.DescribeSecurityGroups([]string{source.ID}),
			"security group must survive a failed DeleteSecurityGroup")

		_, _, err = b.RevokeSecurityGroupIngress(target.ID, []ec2.SecurityGroupRule{
			{Protocol: "tcp", FromPort: 80, ToPort: 80, SourceGroupID: source.ID},
		})
		require.NoError(t, err)

		require.NoError(t, b.DeleteSecurityGroup(source.ID))
	})

	t.Run("no_dependencies", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend()

		sg, err := b.CreateSecurityGroup("standalone-sg", "testing", "vpc-default")
		require.NoError(t, err)

		require.NoError(t, b.DeleteSecurityGroup(sg.ID))
	})
}

// ---- Gap 7: DryRun ----

// TestHTTP_DescribeSecurityGroupRules verifies the HTTP handler.
func TestHTTP_DescribeSecurityGroupRules(t *testing.T) {
	t.Parallel()

	h := newHandler()

	// use the default sg — real AWS clients send filters as
	// Filter.N.Name / Filter.N.Value.M, never a bare Filter.N.Value.
	rec := postForm(
		t,
		h,
		"Action=DescribeSecurityGroupRules&Version=2016-11-15&Filter.1.Name=group-id&Filter.1.Value.1=sg-default",
	)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "DescribeSecurityGroupRulesResponse")
}

func TestSecurityGroupRuleOperations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		op      string
		wantErr bool
	}{
		{name: "authorize_ingress", op: "auth_ingress", wantErr: false},
		{name: "authorize_egress", op: "auth_egress", wantErr: false},
		{name: "revoke_ingress", op: "revoke_ingress", wantErr: false},
		{name: "revoke_egress", op: "revoke_egress", wantErr: false},
		{name: "revoke_egress_idempotent", op: "revoke_egress_idempotent", wantErr: false},
		{name: "authorize_ingress_bad_sg", op: "auth_ingress_bad_sg", wantErr: true},
		{name: "authorize_egress_bad_sg", op: "auth_egress_bad_sg", wantErr: true},
		{name: "revoke_ingress_bad_sg", op: "revoke_ingress_bad_sg", wantErr: true},
		{name: "revoke_egress_bad_sg", op: "revoke_egress_bad_sg", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			rule := ec2.SecurityGroupRule{
				Protocol: "tcp",
				FromPort: 80,
				ToPort:   80,
				IPRange:  "0.0.0.0/0",
			}

			switch tt.op {
			case "auth_ingress":
				sg, err := b.CreateSecurityGroup("test-sg", "test", "vpc-default")
				require.NoError(t, err)
				err = b.AuthorizeSecurityGroupIngress(sg.ID, []ec2.SecurityGroupRule{rule})
				require.NoError(t, err)
				sgs := b.DescribeSecurityGroups([]string{sg.ID})
				require.Len(t, sgs, 1)
				assert.Len(t, sgs[0].IngressRules, 1)

			case "auth_egress":
				sg, err := b.CreateSecurityGroup("test-sg-egress", "test", "vpc-default")
				require.NoError(t, err)
				err = b.AuthorizeSecurityGroupEgress(sg.ID, []ec2.SecurityGroupRule{rule})
				require.NoError(t, err)
				sgs := b.DescribeSecurityGroups([]string{sg.ID})
				require.Len(t, sgs, 1)
				// 2 = 1 default allow-all egress + 1 explicitly added rule.
				assert.Len(t, sgs[0].EgressRules, 2)

			case "revoke_ingress":
				sg, err := b.CreateSecurityGroup("test-sg-revoke", "test", "vpc-default")
				require.NoError(t, err)
				err = b.AuthorizeSecurityGroupIngress(sg.ID, []ec2.SecurityGroupRule{rule})
				require.NoError(t, err)
				_, _, err = b.RevokeSecurityGroupIngress(sg.ID, []ec2.SecurityGroupRule{rule})
				require.NoError(t, err)
				sgs := b.DescribeSecurityGroups([]string{sg.ID})
				require.Len(t, sgs, 1)
				assert.Empty(t, sgs[0].IngressRules)

			case "revoke_egress":
				sg, err := b.CreateSecurityGroup("test-sg-revoke-egr", "test", "vpc-default")
				require.NoError(t, err)
				err = b.AuthorizeSecurityGroupEgress(sg.ID, []ec2.SecurityGroupRule{rule})
				require.NoError(t, err)
				_, err = b.RevokeSecurityGroupEgress(sg.ID, []ec2.SecurityGroupRule{rule})
				require.NoError(t, err)
				sgs := b.DescribeSecurityGroups([]string{sg.ID})
				require.Len(t, sgs, 1)
				// Default allow-all egress rule remains after revoking the explicitly added rule.
				assert.Len(t, sgs[0].EgressRules, 1)

			case "revoke_egress_idempotent":
				// Revoking a rule that was never added must return InvalidPermission.NotFound (AWS behavior).
				sg, err := b.CreateSecurityGroup("test-sg-revoke-egr-idem", "test", "vpc-default")
				require.NoError(t, err)
				_, err = b.RevokeSecurityGroupEgress(sg.ID, []ec2.SecurityGroupRule{rule})
				require.Error(t, err)
				require.ErrorIs(t, err, ec2.ErrNetworkInterfacePermissionNotFound)

			case "revoke_egress_bad_sg":
				_, err := b.RevokeSecurityGroupEgress("sg-nonexistent", []ec2.SecurityGroupRule{rule})
				require.Error(t, err)

			case "auth_ingress_bad_sg":
				err := b.AuthorizeSecurityGroupIngress(
					"sg-nonexistent",
					[]ec2.SecurityGroupRule{rule},
				)
				require.Error(t, err)

			case "auth_egress_bad_sg":
				err := b.AuthorizeSecurityGroupEgress(
					"sg-nonexistent",
					[]ec2.SecurityGroupRule{rule},
				)
				require.Error(t, err)

			case "revoke_ingress_bad_sg":
				_, _, err := b.RevokeSecurityGroupIngress("sg-nonexistent", []ec2.SecurityGroupRule{rule})
				require.Error(t, err)
			}
		})
	}
}

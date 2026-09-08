package integration_test

import (
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	ec2sdk "github.com/aws/aws-sdk-go-v2/service/ec2"
	elbv2sdk "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mustCreateELBv2NetworkPrereqs creates n real EC2 subnets (in a fresh VPC
// rooted at cidrBase, e.g. "10.91") plus one security group, for ELBv2 test
// fixtures. Now that ELBv2 validates SecurityGroups/Subnets against the real
// EC2 backend (gopherstack-t74c), fabricated IDs like "subnet-aaaaaaaa" are
// rejected by CreateLoadBalancer once cli.go's composition root wires the two
// services together, which the full-server integration container does.
// Cleanup is registered automatically.
func mustCreateELBv2NetworkPrereqs(t *testing.T, cidrBase string, n int) ([]string, string) {
	t.Helper()

	ec2Client := createEC2Client(t)
	ctx := t.Context()

	vpcOut, err := ec2Client.CreateVpc(ctx, &ec2sdk.CreateVpcInput{
		CidrBlock: aws.String(cidrBase + ".0.0/16"),
	})
	require.NoError(t, err, "CreateVpc should succeed")
	vpcID := aws.ToString(vpcOut.Vpc.VpcId)

	t.Cleanup(func() {
		cleanupCtx, cancel := cleanupContext(t)
		defer cancel()

		_, _ = ec2Client.DeleteVpc(cleanupCtx, &ec2sdk.DeleteVpcInput{VpcId: aws.String(vpcID)})
	})

	subnetIDs := make([]string, 0, n)

	for i := range n {
		subnetOut, subnetErr := ec2Client.CreateSubnet(ctx, &ec2sdk.CreateSubnetInput{
			VpcId:     aws.String(vpcID),
			CidrBlock: aws.String(fmt.Sprintf("%s.%d.0/24", cidrBase, i+1)),
		})
		require.NoError(t, subnetErr, "CreateSubnet should succeed")
		subnetID := aws.ToString(subnetOut.Subnet.SubnetId)
		subnetIDs = append(subnetIDs, subnetID)

		t.Cleanup(func() {
			cleanupCtx, cancel := cleanupContext(t)
			defer cancel()

			_, _ = ec2Client.DeleteSubnet(cleanupCtx, &ec2sdk.DeleteSubnetInput{SubnetId: aws.String(subnetID)})
		})
	}

	sgOut, err := ec2Client.CreateSecurityGroup(ctx, &ec2sdk.CreateSecurityGroupInput{
		GroupName:   aws.String("elbv2-it-sg-" + uuid.NewString()[:8]),
		Description: aws.String("elbv2 integration test sg"),
		VpcId:       aws.String(vpcID),
	})
	require.NoError(t, err, "CreateSecurityGroup should succeed")
	sgID := aws.ToString(sgOut.GroupId)

	t.Cleanup(func() {
		cleanupCtx, cancel := cleanupContext(t)
		defer cancel()

		_, _ = ec2Client.DeleteSecurityGroup(cleanupCtx, &ec2sdk.DeleteSecurityGroupInput{GroupId: aws.String(sgID)})
	})

	return subnetIDs, sgID
}

// TestIntegration_ELBv2_ALBLifecycle exercises the most common Application Load Balancer
// workflow end-to-end via the AWS SDK v2: create LB, target group, listener with rules,
// register targets, modify attributes, inspect tags, then tear everything down.
//
// This is the primary integration coverage for elasticloadbalancingv2 and protects against
// regressions in URL/form parsing, XML response shape, and ARN derivation across the
// handler. AWS calls go through real SigV4 signing against the gopherstack endpoint.
func TestIntegration_ELBv2_ALBLifecycle(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createELBv2Client(t)
	ctx := t.Context()

	const (
		lbName          = "it-alb-lifecycle"
		tgName          = "it-tg-lifecycle"
		listPort        = int32(80)
		rulePathPattern = "/api/*"
		targetIP        = "10.0.1.10"
		targetIP2       = "10.0.1.11"
	)

	subnetIDs, sgID := mustCreateELBv2NetworkPrereqs(t, "10.91", 2)

	// CreateLoadBalancer (ALB).
	createLBOut, err := client.CreateLoadBalancer(ctx, &elbv2sdk.CreateLoadBalancerInput{
		Name:           aws.String(lbName),
		Scheme:         elbv2types.LoadBalancerSchemeEnumInternetFacing,
		Type:           elbv2types.LoadBalancerTypeEnumApplication,
		IpAddressType:  elbv2types.IpAddressTypeIpv4,
		Subnets:        subnetIDs,
		SecurityGroups: []string{sgID},
		Tags: []elbv2types.Tag{
			{Key: aws.String("env"), Value: aws.String("test")},
		},
	})
	require.NoError(t, err)
	require.Len(t, createLBOut.LoadBalancers, 1)
	lbArn := aws.ToString(createLBOut.LoadBalancers[0].LoadBalancerArn)
	require.NotEmpty(t, lbArn)
	assert.Equal(t, lbName, aws.ToString(createLBOut.LoadBalancers[0].LoadBalancerName))
	assert.Equal(t, elbv2types.LoadBalancerTypeEnumApplication, createLBOut.LoadBalancers[0].Type)

	t.Cleanup(func() {
		cleanupCtx, cancel := cleanupContext(t)
		defer cancel()

		_, _ = client.DeleteLoadBalancer(cleanupCtx, &elbv2sdk.DeleteLoadBalancerInput{
			LoadBalancerArn: aws.String(lbArn),
		})
	})

	// Duplicate name should fail.
	_, dupErr := client.CreateLoadBalancer(ctx, &elbv2sdk.CreateLoadBalancerInput{
		Name:    aws.String(lbName),
		Subnets: subnetIDs,
	})
	require.Error(t, dupErr)

	// DescribeLoadBalancers by ARN and by Name.
	descByArn, err := client.DescribeLoadBalancers(ctx, &elbv2sdk.DescribeLoadBalancersInput{
		LoadBalancerArns: []string{lbArn},
	})
	require.NoError(t, err)
	require.Len(t, descByArn.LoadBalancers, 1)
	assert.Equal(t, lbArn, aws.ToString(descByArn.LoadBalancers[0].LoadBalancerArn))

	descByName, err := client.DescribeLoadBalancers(ctx, &elbv2sdk.DescribeLoadBalancersInput{
		Names: []string{lbName},
	})
	require.NoError(t, err)
	require.Len(t, descByName.LoadBalancers, 1)

	// CreateTargetGroup (IP target type).
	createTGOut, err := client.CreateTargetGroup(ctx, &elbv2sdk.CreateTargetGroupInput{
		Name:       aws.String(tgName),
		Protocol:   elbv2types.ProtocolEnumHttp,
		Port:       aws.Int32(8080),
		VpcId:      aws.String("vpc-00000001"),
		TargetType: elbv2types.TargetTypeEnumIp,
		Tags: []elbv2types.Tag{
			{Key: aws.String("tier"), Value: aws.String("api")},
		},
	})
	require.NoError(t, err)
	require.Len(t, createTGOut.TargetGroups, 1)
	tgArn := aws.ToString(createTGOut.TargetGroups[0].TargetGroupArn)
	require.NotEmpty(t, tgArn)
	assert.Equal(t, tgName, aws.ToString(createTGOut.TargetGroups[0].TargetGroupName))

	t.Cleanup(func() {
		cleanupCtx, cancel := cleanupContext(t)
		defer cancel()

		_, _ = client.DeleteTargetGroup(cleanupCtx, &elbv2sdk.DeleteTargetGroupInput{
			TargetGroupArn: aws.String(tgArn),
		})
	})

	// RegisterTargets.
	_, err = client.RegisterTargets(ctx, &elbv2sdk.RegisterTargetsInput{
		TargetGroupArn: aws.String(tgArn),
		Targets: []elbv2types.TargetDescription{
			{Id: aws.String(targetIP), Port: aws.Int32(8080)},
			{Id: aws.String(targetIP2), Port: aws.Int32(8080)},
		},
	})
	require.NoError(t, err)

	// DescribeTargetHealth - both targets should be reported.
	healthOut, err := client.DescribeTargetHealth(ctx, &elbv2sdk.DescribeTargetHealthInput{
		TargetGroupArn: aws.String(tgArn),
	})
	require.NoError(t, err)
	require.Len(t, healthOut.TargetHealthDescriptions, 2)

	// DeregisterTargets - deregister one target; it transitions to draining state.
	_, err = client.DeregisterTargets(ctx, &elbv2sdk.DeregisterTargetsInput{
		TargetGroupArn: aws.String(tgArn),
		Targets: []elbv2types.TargetDescription{
			{Id: aws.String(targetIP2), Port: aws.Int32(8080)},
		},
	})
	require.NoError(t, err)

	// Deregistered target remains visible in draining state; the other stays healthy.
	healthOut2, err := client.DescribeTargetHealth(ctx, &elbv2sdk.DescribeTargetHealthInput{
		TargetGroupArn: aws.String(tgArn),
	})
	require.NoError(t, err)
	require.Len(t, healthOut2.TargetHealthDescriptions, 2)
	healthByID := map[string]elbv2types.TargetHealthDescription{}
	for _, d := range healthOut2.TargetHealthDescriptions {
		healthByID[aws.ToString(d.Target.Id)] = d
	}
	require.Contains(t, healthByID, targetIP2)
	assert.Equal(t, elbv2types.TargetHealthStateEnumDraining, healthByID[targetIP2].TargetHealth.State)

	// CreateListener forwarding to the target group.
	createListOut, err := client.CreateListener(ctx, &elbv2sdk.CreateListenerInput{
		LoadBalancerArn: aws.String(lbArn),
		Protocol:        elbv2types.ProtocolEnumHttp,
		Port:            aws.Int32(listPort),
		DefaultActions: []elbv2types.Action{
			{
				Type:           elbv2types.ActionTypeEnumForward,
				TargetGroupArn: aws.String(tgArn),
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, createListOut.Listeners, 1)
	listenerArn := aws.ToString(createListOut.Listeners[0].ListenerArn)
	require.NotEmpty(t, listenerArn)

	t.Cleanup(func() {
		cleanupCtx, cancel := cleanupContext(t)
		defer cancel()

		_, _ = client.DeleteListener(cleanupCtx, &elbv2sdk.DeleteListenerInput{
			ListenerArn: aws.String(listenerArn),
		})
	})

	// Duplicate listener on the same port should fail.
	_, dupListErr := client.CreateListener(ctx, &elbv2sdk.CreateListenerInput{
		LoadBalancerArn: aws.String(lbArn),
		Protocol:        elbv2types.ProtocolEnumHttp,
		Port:            aws.Int32(listPort),
		DefaultActions: []elbv2types.Action{
			{Type: elbv2types.ActionTypeEnumForward, TargetGroupArn: aws.String(tgArn)},
		},
	})
	require.Error(t, dupListErr)

	// CreateRule with a PathPattern condition.
	createRuleOut, err := client.CreateRule(ctx, &elbv2sdk.CreateRuleInput{
		ListenerArn: aws.String(listenerArn),
		Priority:    aws.Int32(10),
		Conditions: []elbv2types.RuleCondition{
			{
				Field: aws.String("path-pattern"),
				PathPatternConfig: &elbv2types.PathPatternConditionConfig{
					Values: []string{rulePathPattern},
				},
			},
		},
		Actions: []elbv2types.Action{
			{Type: elbv2types.ActionTypeEnumForward, TargetGroupArn: aws.String(tgArn)},
		},
	})
	require.NoError(t, err)
	require.Len(t, createRuleOut.Rules, 1)
	ruleArn := aws.ToString(createRuleOut.Rules[0].RuleArn)
	require.NotEmpty(t, ruleArn)

	t.Cleanup(func() {
		cleanupCtx, cancel := cleanupContext(t)
		defer cancel()

		_, _ = client.DeleteRule(cleanupCtx, &elbv2sdk.DeleteRuleInput{RuleArn: aws.String(ruleArn)})
	})

	// DescribeRules should return at least the new rule + the auto-created default rule.
	rulesOut, err := client.DescribeRules(ctx, &elbv2sdk.DescribeRulesInput{
		ListenerArn: aws.String(listenerArn),
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(rulesOut.Rules), 2)

	// ModifyLoadBalancerAttributes - flip deletion_protection.enabled.
	_, err = client.ModifyLoadBalancerAttributes(ctx, &elbv2sdk.ModifyLoadBalancerAttributesInput{
		LoadBalancerArn: aws.String(lbArn),
		Attributes: []elbv2types.LoadBalancerAttribute{
			{Key: aws.String("deletion_protection.enabled"), Value: aws.String("false")},
			{Key: aws.String("idle_timeout.timeout_seconds"), Value: aws.String("120")},
		},
	})
	require.NoError(t, err)

	attrsOut, err := client.DescribeLoadBalancerAttributes(ctx, &elbv2sdk.DescribeLoadBalancerAttributesInput{
		LoadBalancerArn: aws.String(lbArn),
	})
	require.NoError(t, err)

	gotAttrs := make(map[string]string, len(attrsOut.Attributes))
	for _, a := range attrsOut.Attributes {
		gotAttrs[aws.ToString(a.Key)] = aws.ToString(a.Value)
	}
	assert.Equal(t, "120", gotAttrs["idle_timeout.timeout_seconds"])

	// AddTags / DescribeTags / RemoveTags on the load balancer.
	_, err = client.AddTags(ctx, &elbv2sdk.AddTagsInput{
		ResourceArns: []string{lbArn},
		Tags: []elbv2types.Tag{
			{Key: aws.String("owner"), Value: aws.String("integration")},
		},
	})
	require.NoError(t, err)

	tagsOut, err := client.DescribeTags(ctx, &elbv2sdk.DescribeTagsInput{
		ResourceArns: []string{lbArn},
	})
	require.NoError(t, err)
	require.Len(t, tagsOut.TagDescriptions, 1)

	gotTags := make(map[string]string)
	for _, kv := range tagsOut.TagDescriptions[0].Tags {
		gotTags[aws.ToString(kv.Key)] = aws.ToString(kv.Value)
	}
	assert.Equal(t, "integration", gotTags["owner"])
	assert.Equal(t, "test", gotTags["env"])

	_, err = client.RemoveTags(ctx, &elbv2sdk.RemoveTagsInput{
		ResourceArns: []string{lbArn},
		TagKeys:      []string{"owner"},
	})
	require.NoError(t, err)

	tagsOut2, err := client.DescribeTags(ctx, &elbv2sdk.DescribeTagsInput{
		ResourceArns: []string{lbArn},
	})
	require.NoError(t, err)
	require.Len(t, tagsOut2.TagDescriptions, 1)

	for _, kv := range tagsOut2.TagDescriptions[0].Tags {
		assert.NotEqual(t, "owner", aws.ToString(kv.Key))
	}

	// DescribeListeners returns the listener we created.
	listOut, err := client.DescribeListeners(ctx, &elbv2sdk.DescribeListenersInput{
		LoadBalancerArn: aws.String(lbArn),
	})
	require.NoError(t, err)
	require.Len(t, listOut.Listeners, 1)
	assert.Equal(t, listenerArn, aws.ToString(listOut.Listeners[0].ListenerArn))

	// DescribeTargetGroups by TargetGroupArn must return the registered TG.
	tgOut, err := client.DescribeTargetGroups(ctx, &elbv2sdk.DescribeTargetGroupsInput{
		TargetGroupArns: []string{tgArn},
	})
	require.NoError(t, err)
	require.Len(t, tgOut.TargetGroups, 1)
	assert.Equal(t, tgArn, aws.ToString(tgOut.TargetGroups[0].TargetGroupArn))
}

// TestIntegration_ELBv2_NLB exercises a basic Network Load Balancer flow to ensure
// TCP protocol validation and the network LB type are handled correctly.
func TestIntegration_ELBv2_NLB(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createELBv2Client(t)
	ctx := t.Context()

	const (
		nlbName = "it-nlb-basic"
		tgName  = "it-nlb-tg"
	)

	subnetIDs, _ := mustCreateELBv2NetworkPrereqs(t, "10.92", 1)

	createOut, err := client.CreateLoadBalancer(ctx, &elbv2sdk.CreateLoadBalancerInput{
		Name:    aws.String(nlbName),
		Type:    elbv2types.LoadBalancerTypeEnumNetwork,
		Scheme:  elbv2types.LoadBalancerSchemeEnumInternal,
		Subnets: subnetIDs,
	})
	require.NoError(t, err)
	require.Len(t, createOut.LoadBalancers, 1)
	lbArn := aws.ToString(createOut.LoadBalancers[0].LoadBalancerArn)
	assert.Equal(t, elbv2types.LoadBalancerTypeEnumNetwork, createOut.LoadBalancers[0].Type)

	t.Cleanup(func() {
		cleanupCtx, cancel := cleanupContext(t)
		defer cancel()

		_, _ = client.DeleteLoadBalancer(cleanupCtx, &elbv2sdk.DeleteLoadBalancerInput{
			LoadBalancerArn: aws.String(lbArn),
		})
	})

	tgOut, err := client.CreateTargetGroup(ctx, &elbv2sdk.CreateTargetGroupInput{
		Name:       aws.String(tgName),
		Protocol:   elbv2types.ProtocolEnumTcp,
		Port:       aws.Int32(443),
		VpcId:      aws.String("vpc-00000001"),
		TargetType: elbv2types.TargetTypeEnumInstance,
	})
	require.NoError(t, err)
	require.Len(t, tgOut.TargetGroups, 1)
	tgArn := aws.ToString(tgOut.TargetGroups[0].TargetGroupArn)

	t.Cleanup(func() {
		cleanupCtx, cancel := cleanupContext(t)
		defer cancel()

		_, _ = client.DeleteTargetGroup(cleanupCtx, &elbv2sdk.DeleteTargetGroupInput{
			TargetGroupArn: aws.String(tgArn),
		})
	})

	listOut, err := client.CreateListener(ctx, &elbv2sdk.CreateListenerInput{
		LoadBalancerArn: aws.String(lbArn),
		Protocol:        elbv2types.ProtocolEnumTcp,
		Port:            aws.Int32(443),
		DefaultActions: []elbv2types.Action{
			{Type: elbv2types.ActionTypeEnumForward, TargetGroupArn: aws.String(tgArn)},
		},
	})
	require.NoError(t, err)
	require.Len(t, listOut.Listeners, 1)
	listenerArn := aws.ToString(listOut.Listeners[0].ListenerArn)

	t.Cleanup(func() {
		cleanupCtx, cancel := cleanupContext(t)
		defer cancel()

		_, _ = client.DeleteListener(cleanupCtx, &elbv2sdk.DeleteListenerInput{
			ListenerArn: aws.String(listenerArn),
		})
	})

	// HTTPS rules are invalid on NLB listeners; a forward action is fine but a
	// rule with a path-pattern condition should be rejected on TCP listeners on
	// real AWS. We at minimum verify the listener exists and is described.
	descOut, err := client.DescribeListeners(ctx, &elbv2sdk.DescribeListenersInput{
		LoadBalancerArn: aws.String(lbArn),
	})
	require.NoError(t, err)
	require.Len(t, descOut.Listeners, 1)
	assert.Equal(t, elbv2types.ProtocolEnumTcp, descOut.Listeners[0].Protocol)
}

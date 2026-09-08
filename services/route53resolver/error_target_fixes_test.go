package route53resolver_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	route53resolversdk "github.com/aws/aws-sdk-go-v2/service/route53resolver"
	"github.com/aws/aws-sdk-go-v2/service/route53resolver/types"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/route53resolver"
)

// TestCreateFirewallRuleGroup_EmptyName_RealClient drives CreateFirewallRuleGroup
// through the real client with an explicit empty Name. gopherstack previously
// classified this with the shared ErrValidation sentinel
// (InvalidRequestException) -- but CreateFirewallRuleGroup's own deserializer
// (awsAwsjson11_deserializeOpErrorCreateFirewallRuleGroup) models
// AccessDeniedException/InternalServiceErrorException/LimitExceededException/
// ThrottlingException/ValidationException, with NO InvalidRequestException.
// validateOpCreateFirewallRuleGroupInput only checks Name != nil (not
// non-empty), so aws.String("") passes client-side validation and this path
// is reachable (gopherstack-6flj/uox6).
func TestCreateFirewallRuleGroup_EmptyName_RealClient(t *testing.T) {
	t.Parallel()

	backend := route53resolver.NewInMemoryBackend("000000000000", "us-east-1")
	h := route53resolver.NewHandler(backend)
	client := newTestRoute53ResolverClient(t, h)

	_, err := client.CreateFirewallRuleGroup(t.Context(), &route53resolversdk.CreateFirewallRuleGroupInput{
		Name: aws.String(""),
	})
	require.Error(t, err)

	var ve *types.ValidationException
	require.ErrorAs(t, err, &ve, "expected a real ValidationException from the SDK deserializer")
}

// TestCreateFirewallRule_DuplicateDomainList_RealClient drives CreateFirewallRule
// through the real client with a domain list already used by another rule in
// the same rule group. gopherstack previously emitted ResourceExistsException
// (via ErrAlreadyExists) for this case -- but ResourceExistsException is
// exclusively a Resolver*-association error in this SDK (confirmed: only
// AssociateResolverEndpointIpAddress/AssociateResolverQueryLogConfig/
// AssociateResolverRule/CreateResolverEndpoint/CreateResolverQueryLogConfig/
// CreateResolverRule declare it). CreateFirewallRule's own deserializer
// declares ValidationException instead (gopherstack-6flj/uox6).
func TestCreateFirewallRule_DuplicateDomainList_RealClient(t *testing.T) {
	t.Parallel()

	backend := route53resolver.NewInMemoryBackend("000000000000", "us-east-1")
	h := route53resolver.NewHandler(backend)
	client := newTestRoute53ResolverClient(t, h)

	group, err := client.CreateFirewallRuleGroup(t.Context(), &route53resolversdk.CreateFirewallRuleGroupInput{
		Name: aws.String("dup-domain-list-group"),
	})
	require.NoError(t, err)

	domainList, err := client.CreateFirewallDomainList(t.Context(), &route53resolversdk.CreateFirewallDomainListInput{
		Name: aws.String("dup-domain-list"),
	})
	require.NoError(t, err)

	_, err = client.CreateFirewallRule(t.Context(), &route53resolversdk.CreateFirewallRuleInput{
		FirewallRuleGroupId:  group.FirewallRuleGroup.Id,
		FirewallDomainListId: domainList.FirewallDomainList.Id,
		Name:                 aws.String("rule-one"),
		Action:               types.ActionAllow,
		Priority:             aws.Int32(100),
		CreatorRequestId:     aws.String("req-one"),
	})
	require.NoError(t, err)

	_, err = client.CreateFirewallRule(t.Context(), &route53resolversdk.CreateFirewallRuleInput{
		FirewallRuleGroupId:  group.FirewallRuleGroup.Id,
		FirewallDomainListId: domainList.FirewallDomainList.Id,
		Name:                 aws.String("rule-two"),
		Action:               types.ActionAllow,
		Priority:             aws.Int32(200),
		CreatorRequestId:     aws.String("req-two"),
	})
	require.Error(t, err)

	var ve *types.ValidationException
	require.ErrorAs(t, err, &ve, "expected a real ValidationException from the SDK deserializer")
}

// TestGetFirewallRuleGroup_EmptyID_RealClient drives GetFirewallRuleGroup
// through the real client with an explicit empty FirewallRuleGroupId.
// gopherstack previously classified this with ErrValidation
// (InvalidRequestException) -- but GetFirewallRuleGroup's own deserializer
// declares only AccessDeniedException/InternalServiceErrorException/
// ResourceNotFoundException/ThrottlingException: no ValidationException, no
// InvalidRequestException. The handler-level check was removed entirely,
// letting the backend's natural lookup-miss produce ResourceNotFoundException,
// a type the op does declare (gopherstack-6flj/uox6).
func TestGetFirewallRuleGroup_EmptyID_RealClient(t *testing.T) {
	t.Parallel()

	backend := route53resolver.NewInMemoryBackend("000000000000", "us-east-1")
	h := route53resolver.NewHandler(backend)
	client := newTestRoute53ResolverClient(t, h)

	_, err := client.GetFirewallRuleGroup(t.Context(), &route53resolversdk.GetFirewallRuleGroupInput{
		FirewallRuleGroupId: aws.String(""),
	})
	require.Error(t, err)

	var nf *types.ResourceNotFoundException
	require.ErrorAs(t, err, &nf, "expected a real ResourceNotFoundException from the SDK deserializer")
}

// TestGetResolverRulePolicy_EmptyArn_RealClient drives GetResolverRulePolicy
// through the real client with an explicit empty Arn. gopherstack previously
// classified this with ErrValidation (InvalidRequestException) -- but
// GetResolverRulePolicy's own deserializer declares
// AccessDeniedException/InternalServiceErrorException/
// InvalidParameterException/UnknownResourceException: no
// InvalidRequestException, no ValidationException. The backend policy lookup
// is a blind map read with no natural not-found path, so the fix points at
// InvalidParameterException, which the op does declare
// (gopherstack-6flj/uox6).
func TestGetResolverRulePolicy_EmptyArn_RealClient(t *testing.T) {
	t.Parallel()

	backend := route53resolver.NewInMemoryBackend("000000000000", "us-east-1")
	h := route53resolver.NewHandler(backend)
	client := newTestRoute53ResolverClient(t, h)

	_, err := client.GetResolverRulePolicy(t.Context(), &route53resolversdk.GetResolverRulePolicyInput{
		Arn: aws.String(""),
	})
	require.Error(t, err)

	var ip *types.InvalidParameterException
	require.ErrorAs(t, err, &ip, "expected a real InvalidParameterException from the SDK deserializer")
}

// TestGetFirewallConfig_EmptyResourceID_RealClient drives GetFirewallConfig
// through the real client with an explicit empty ResourceId. gopherstack
// previously classified this with the shared requireResourceID helper's
// default ErrValidation (InvalidRequestException) -- but GetFirewallConfig's
// own deserializer declares only AccessDeniedException/
// InternalServiceErrorException/ResourceNotFoundException/
// ThrottlingException/ValidationException: no InvalidRequestException.
// requireResourceID/getSimpleConfig now take an explicit per-caller sentinel
// so FirewallConfig/ResolverConfig (ValidationException) and
// ResolverDnssecConfig (InvalidRequestException, genuinely correct, left
// alone) each get their own real code (gopherstack-6flj/uox6).
func TestGetFirewallConfig_EmptyResourceID_RealClient(t *testing.T) {
	t.Parallel()

	backend := route53resolver.NewInMemoryBackend("000000000000", "us-east-1")
	h := route53resolver.NewHandler(backend)
	client := newTestRoute53ResolverClient(t, h)

	_, err := client.GetFirewallConfig(t.Context(), &route53resolversdk.GetFirewallConfigInput{
		ResourceId: aws.String(""),
	})
	require.Error(t, err)

	var ve *types.ValidationException
	require.ErrorAs(t, err, &ve, "expected a real ValidationException from the SDK deserializer")
}

// TestAssociateDuplicate_ResourceExistsException_RealClient drives
// AssociateResolverRule and AssociateResolverQueryLogConfig through the real
// client, associating the same (ResolverRuleId, VPCId) /
// (ResolverQueryLogConfigId, ResourceId) pair twice. Both ops declare
// ResourceExistsException in their own deserializer (verified against
// deserializers.go's awsAwsjson11_deserializeOpError<Op> switches), and the
// AWS API reference's Errors section for each op describes it as "The
// resource that you tried to create already exists" -- but gopherstack had
// no duplicate-detection logic at all for either op, so a second identical
// Associate call silently created a second association instead of being
// rejected (gopherstack-6flj's error-target audit had already flagged this
// class of gap as "not fixed here, recorded for a future pass").
// DisassociateResolverRule/DisassociateResolverQueryLogConfig already treat
// that same pair as the association's identity, so re-associating it is the
// duplicate condition fixed here.
func TestAssociateDuplicate_ResourceExistsException_RealClient(t *testing.T) {
	t.Parallel()

	tests := []struct {
		run  func(t *testing.T, client *route53resolversdk.Client) error
		name string
	}{
		{
			name: "resolver_rule",
			run: func(t *testing.T, client *route53resolversdk.Client) error {
				t.Helper()

				rule, err := client.CreateResolverRule(t.Context(), &route53resolversdk.CreateResolverRuleInput{
					CreatorRequestId: aws.String("dup-rule-req"),
					Name:             aws.String("dup-rule"),
					DomainName:       aws.String("dup.example.com"),
					RuleType:         types.RuleTypeOptionForward,
					TargetIps: []types.TargetAddress{
						{Ip: aws.String("10.0.0.1"), Port: aws.Int32(53)},
					},
				})
				require.NoError(t, err)

				_, err = client.AssociateResolverRule(t.Context(), &route53resolversdk.AssociateResolverRuleInput{
					ResolverRuleId: rule.ResolverRule.Id,
					VPCId:          aws.String("vpc-dup-rule"),
				})
				require.NoError(t, err)

				_, err = client.AssociateResolverRule(t.Context(), &route53resolversdk.AssociateResolverRuleInput{
					ResolverRuleId: rule.ResolverRule.Id,
					VPCId:          aws.String("vpc-dup-rule"),
				})

				return err
			},
		},
		{
			name: "query_log_config",
			run: func(t *testing.T, client *route53resolversdk.Client) error {
				t.Helper()

				cfg, err := client.CreateResolverQueryLogConfig(
					t.Context(),
					&route53resolversdk.CreateResolverQueryLogConfigInput{
						CreatorRequestId: aws.String("dup-qlc-req"),
						Name:             aws.String("dup-qlc"),
						DestinationArn:   aws.String("arn:aws:s3:::dup-qlc-bucket"),
					},
				)
				require.NoError(t, err)

				_, err = client.AssociateResolverQueryLogConfig(
					t.Context(),
					&route53resolversdk.AssociateResolverQueryLogConfigInput{
						ResolverQueryLogConfigId: cfg.ResolverQueryLogConfig.Id,
						ResourceId:               aws.String("vpc-dup-qlc"),
					},
				)
				require.NoError(t, err)

				_, err = client.AssociateResolverQueryLogConfig(
					t.Context(),
					&route53resolversdk.AssociateResolverQueryLogConfigInput{
						ResolverQueryLogConfigId: cfg.ResolverQueryLogConfig.Id,
						ResourceId:               aws.String("vpc-dup-qlc"),
					},
				)

				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := route53resolver.NewInMemoryBackend("000000000000", "us-east-1")
			h := route53resolver.NewHandler(backend)
			client := newTestRoute53ResolverClient(t, h)

			err := tt.run(t, client)
			require.Error(t, err)

			var ree *types.ResourceExistsException
			require.ErrorAs(t, err, &ree, "expected a real ResourceExistsException from the SDK deserializer")
		})
	}
}

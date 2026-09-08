package route53resolver_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	route53resolversdk "github.com/aws/aws-sdk-go-v2/service/route53resolver"
	"github.com/aws/aws-sdk-go-v2/service/route53resolver/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/route53resolver"
)

type createResult struct {
	err error
	id  string
}

// TestCreate_CreatorRequestId_Idempotency covers the three Create ops whose
// own SDK deserializer declares ResourceExistsException
// (CreateResolverEndpoint/CreateResolverQueryLogConfig/CreateResolverRule --
// confirmed against the botocore route53resolver 2018-04-01 model's
// operations[op].errors, gopherstack-tihg). CreatorRequestId's doc comment
// (api_op_CreateResolverRule.go:33-35, aws-sdk-go-v2 v1.53.0) states it
// "allows failed requests to be retried without the risk of running the
// operation twice" -- a matching retry must return the original resource,
// not create a second one.
func TestCreate_CreatorRequestId_Idempotency(t *testing.T) {
	t.Parallel()

	tests := []struct {
		create func(t *testing.T, client *route53resolversdk.Client, creatorRequestID, variant string) createResult
		count  func(t *testing.T, client *route53resolversdk.Client) int
		name   string
	}{
		{
			name: "resolver_endpoint",
			create: func(t *testing.T, client *route53resolversdk.Client, creatorRequestID, variant string) createResult {
				t.Helper()

				subnet := "subnet-1"
				if variant == "different" {
					subnet = "subnet-2"
				}

				out, err := client.CreateResolverEndpoint(t.Context(), &route53resolversdk.CreateResolverEndpointInput{
					CreatorRequestId: aws.String(creatorRequestID),
					Name:             aws.String("idem-ep"),
					Direction:        types.ResolverEndpointDirectionInbound,
					IpAddresses: []types.IpAddressRequest{
						{SubnetId: aws.String(subnet)},
					},
					SecurityGroupIds: []string{"sg-1"},
				})
				if err != nil {
					return createResult{err: err}
				}

				return createResult{id: aws.ToString(out.ResolverEndpoint.Id)}
			},
			count: func(t *testing.T, client *route53resolversdk.Client) int {
				t.Helper()

				out, err := client.ListResolverEndpoints(t.Context(), &route53resolversdk.ListResolverEndpointsInput{})
				require.NoError(t, err)

				return len(out.ResolverEndpoints)
			},
		},
		{
			name: "resolver_rule",
			create: func(t *testing.T, client *route53resolversdk.Client, creatorRequestID, variant string) createResult {
				t.Helper()

				domain := "idem.example.com"
				if variant == "different" {
					domain = "idem-other.example.com"
				}

				out, err := client.CreateResolverRule(t.Context(), &route53resolversdk.CreateResolverRuleInput{
					CreatorRequestId: aws.String(creatorRequestID),
					Name:             aws.String("idem-rule"),
					DomainName:       aws.String(domain),
					RuleType:         types.RuleTypeOptionForward,
					TargetIps: []types.TargetAddress{
						{Ip: aws.String("10.0.0.1"), Port: aws.Int32(53)},
					},
				})
				if err != nil {
					return createResult{err: err}
				}

				return createResult{id: aws.ToString(out.ResolverRule.Id)}
			},
			count: func(t *testing.T, client *route53resolversdk.Client) int {
				t.Helper()

				out, err := client.ListResolverRules(t.Context(), &route53resolversdk.ListResolverRulesInput{})
				require.NoError(t, err)

				return len(out.ResolverRules)
			},
		},
		{
			name: "query_log_config",
			create: func(t *testing.T, client *route53resolversdk.Client, creatorRequestID, variant string) createResult {
				t.Helper()

				dest := "arn:aws:s3:::idem-bucket"
				if variant == "different" {
					dest = "arn:aws:s3:::idem-bucket-2"
				}

				out, err := client.CreateResolverQueryLogConfig(
					t.Context(),
					&route53resolversdk.CreateResolverQueryLogConfigInput{
						CreatorRequestId: aws.String(creatorRequestID),
						Name:             aws.String("idem-qlc"),
						DestinationArn:   aws.String(dest),
					},
				)
				if err != nil {
					return createResult{err: err}
				}

				return createResult{id: aws.ToString(out.ResolverQueryLogConfig.Id)}
			},
			count: func(t *testing.T, client *route53resolversdk.Client) int {
				t.Helper()

				out, err := client.ListResolverQueryLogConfigs(
					t.Context(),
					&route53resolversdk.ListResolverQueryLogConfigsInput{},
				)
				require.NoError(t, err)

				return len(out.ResolverQueryLogConfigs)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			t.Run("matching retry returns same resource", func(t *testing.T) {
				t.Parallel()

				backend := route53resolver.NewInMemoryBackend("000000000000", "us-east-1")
				h := route53resolver.NewHandler(backend)
				client := newTestRoute53ResolverClient(t, h)

				first := tt.create(t, client, "creator-match", "same")
				require.NoError(t, first.err)
				require.NotEmpty(t, first.id)

				second := tt.create(t, client, "creator-match", "same")
				require.NoError(t, second.err)

				assert.Equal(t, first.id, second.id,
					"retry with the same CreatorRequestId and parameters must return the original resource")
				assert.Equal(t, 1, tt.count(t, client),
					"retry with the same CreatorRequestId must not create a second resource")
			})

			t.Run("conflicting retry errors", func(t *testing.T) {
				t.Parallel()

				backend := route53resolver.NewInMemoryBackend("000000000000", "us-east-1")
				h := route53resolver.NewHandler(backend)
				client := newTestRoute53ResolverClient(t, h)

				first := tt.create(t, client, "creator-conflict", "same")
				require.NoError(t, first.err)

				second := tt.create(t, client, "creator-conflict", "different")
				require.Error(t, second.err)

				var ree *types.ResourceExistsException
				require.ErrorAs(
					t, second.err, &ree,
					"expected a real ResourceExistsException from the SDK deserializer",
				)
				assert.Equal(t, 1, tt.count(t, client),
					"a conflicting retry must not create a second resource either")
			})
		})
	}
}

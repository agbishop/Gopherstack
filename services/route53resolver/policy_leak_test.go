package route53resolver_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/route53resolver"
)

// TestDelete_ClearsResourcePolicy verifies DeleteFirewallRuleGroup,
// DeleteResolverQueryLogConfig and DeleteResolverRule all clear their entry
// in the resource-policy map (they already clear tags, but missed this
// sibling map). Get*Policy has no existence check against the resource
// itself, so a leaked entry is directly observable: querying the policy for
// a deleted resource's own ARN would otherwise still return it, and every
// policy map is persisted verbatim in Snapshot(), so the leak also grows the
// snapshot without bound.
func TestDelete_ClearsResourcePolicy(t *testing.T) {
	t.Parallel()

	t.Run("firewall rule group", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		b := route53resolver.NewInMemoryBackend("000000000000", "us-east-1")

		grp, err := b.CreateFirewallRuleGroup(ctx, "policy-leak-frg", "req-1")
		require.NoError(t, err)
		require.NoError(t, b.PutFirewallRuleGroupPolicy(ctx, grp.ARN, `{"Version":"2012-10-17"}`))
		require.NotEmpty(t, b.GetFirewallRuleGroupPolicy(ctx, grp.ARN))

		_, err = b.DeleteFirewallRuleGroup(ctx, grp.ID)
		require.NoError(t, err)

		assert.Empty(t, b.GetFirewallRuleGroupPolicy(ctx, grp.ARN))
	})

	t.Run("resolver query log config", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		b := route53resolver.NewInMemoryBackend("000000000000", "us-east-1")

		cfg, err := b.CreateResolverQueryLogConfig(
			ctx, "policy-leak-qlc", "req-1", "arn:aws:s3:::policy-leak-bucket",
		)
		require.NoError(t, err)
		require.NoError(t, b.PutResolverQueryLogConfigPolicy(ctx, cfg.ARN, `{"Version":"2012-10-17"}`))
		require.NotEmpty(t, b.GetResolverQueryLogConfigPolicy(ctx, cfg.ARN))

		_, err = b.DeleteResolverQueryLogConfig(ctx, cfg.ID)
		require.NoError(t, err)

		assert.Empty(t, b.GetResolverQueryLogConfigPolicy(ctx, cfg.ARN))
	})

	t.Run("resolver rule", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		b := route53resolver.NewInMemoryBackend("000000000000", "us-east-1")

		rule, err := b.CreateResolverRule(
			ctx, "policy-leak-rule", "example.com", "SYSTEM", "", "req-1", "", nil,
		)
		require.NoError(t, err)
		otherRule, err := b.CreateResolverRule(
			ctx, "policy-leak-rule-sibling", "example.org", "SYSTEM", "", "req-2", "", nil,
		)
		require.NoError(t, err)
		require.NoError(t, b.PutResolverRulePolicy(ctx, rule.ARN, `{"Version":"2012-10-17"}`))
		require.NoError(t, b.PutResolverRulePolicy(ctx, otherRule.ARN, `{"Version":"2012-10-17"}`))
		require.NotEmpty(t, b.GetResolverRulePolicy(ctx, rule.ARN))

		require.NoError(t, b.DeleteResolverRule(ctx, rule.ID))

		assert.Empty(t, b.GetResolverRulePolicy(ctx, rule.ARN))
		assert.NotEmpty(t, b.GetResolverRulePolicy(ctx, otherRule.ARN),
			"deleting one resolver rule must not disturb another's policy")
	})
}

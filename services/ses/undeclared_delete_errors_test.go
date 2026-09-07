package ses_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	sessdk "github.com/aws/aws-sdk-go-v2/service/ses"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/ses"
)

// TestDeleteOps_MissingTarget_Idempotent proves that DeleteCustomVerification-
// EmailTemplate, DeleteReceiptRule, DeleteReceiptRuleSet, and
// DeleteReceiptFilter succeed (rather than error) when their target does not
// exist, matching what each operation's own deserializer can decode:
// DeleteCustomVerificationEmailTemplate declares no exception at all
// (ses@v1.37.4 deserializers.go, empty switch); DeleteReceiptRule declares
// only RuleSetDoesNotExist, not RuleDoesNotExist; DeleteReceiptRuleSet
// declares only CannotDelete, not RuleSetDoesNotExist; DeleteReceiptFilter
// declares no exception at all (empty switch, and botocore's ses/2010-12-01
// service-2.json has no "errors" key on this op whatsoever -- not even a
// parent-resource check). Emitting the not-found code these ops don't
// declare left the client with only a generic, untyped error either way, so
// the real service treats a missing delete target as already-deleted.
func TestDeleteOps_MissingTarget_Idempotent(t *testing.T) {
	t.Parallel()

	backend := ses.NewInMemoryBackend()
	h := ses.NewHandler(backend)

	client := newTestSESClient(t, h)
	ctx := t.Context()

	t.Run("custom_verification_template", func(t *testing.T) {
		t.Parallel()

		_, err := client.DeleteCustomVerificationEmailTemplate(ctx, &sessdk.DeleteCustomVerificationEmailTemplateInput{
			TemplateName: aws.String("no-such-template"),
		})
		require.NoError(t, err)
	})

	t.Run("receipt_rule", func(t *testing.T) {
		t.Parallel()

		_, err := client.CreateReceiptRuleSet(ctx, &sessdk.CreateReceiptRuleSetInput{
			RuleSetName: aws.String("rule-delete-rs"),
		})
		require.NoError(t, err)

		_, err = client.DeleteReceiptRule(ctx, &sessdk.DeleteReceiptRuleInput{
			RuleSetName: aws.String("rule-delete-rs"),
			RuleName:    aws.String("no-such-rule"),
		})
		require.NoError(t, err)
	})

	t.Run("receipt_rule_set", func(t *testing.T) {
		t.Parallel()

		_, err := client.DeleteReceiptRuleSet(ctx, &sessdk.DeleteReceiptRuleSetInput{
			RuleSetName: aws.String("no-such-rule-set"),
		})
		require.NoError(t, err)
	})

	t.Run("receipt_filter", func(t *testing.T) {
		t.Parallel()

		_, err := client.DeleteReceiptFilter(ctx, &sessdk.DeleteReceiptFilterInput{
			FilterName: aws.String("no-such-filter"),
		})
		require.NoError(t, err)
	})
}

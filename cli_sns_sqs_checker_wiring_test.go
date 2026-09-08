package main

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/chaos"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
	snsbackend "github.com/blackbirdworks/gopherstack/services/sns"
)

// TestInitializeServices_SNSSQSCheckerWiring drives the actual composition root
// (initializeServices, the function cli.go's Run() calls) rather than invoking
// wireSNSToLambdaFirehose directly, so that deleting the SetSQSChecker call from
// wireSNSToLambdaFirehose -- not just breaking the helper itself -- is what this test is
// sensitive to.
//
// Regression test for sns.SetSQSChecker: without it, checkDLQExists (reached from
// SetSubscriptionAttributes) returns nil whenever no checker is configured, so a
// RedrivePolicy naming a nonexistent SQS queue is silently accepted -- unlike real SNS,
// which rejects it. This asserts SetSubscriptionAttributes now rejects a RedrivePolicy
// pointing at a queue that was never created.
func TestInitializeServices_SNSSQSCheckerWiring(t *testing.T) {
	t.Parallel()

	cli := &CLI{AccountID: "000000000000", Region: "us-east-1"}
	appCtx := &service.AppContext{
		Logger:     slog.Default(),
		Config:     cli,
		JanitorCtx: t.Context(),
	}
	cli.faultStore = chaos.NewFaultStore()

	services, err := initializeServices(appCtx)
	require.NoError(t, err)

	byName := serviceByName(services)

	snsH, ok := byName["SNS"].(*snsbackend.Handler)
	require.True(t, ok, "SNS handler must be registered")

	snsBk, ok := snsH.Backend.(*snsbackend.InMemoryBackend)
	require.True(t, ok, "SNS backend must be an InMemoryBackend")

	topic, err := snsBk.CreateTopic("sns-sqs-checker-wiring-topic", nil)
	require.NoError(t, err)

	sub, err := snsBk.Subscribe(
		topic.TopicArn, "sqs", "arn:aws:sqs:us-east-1:000000000000:sns-sqs-checker-wiring-endpoint", "",
	)
	require.NoError(t, err)

	redrivePolicy := `{"deadLetterTargetArn":"arn:aws:sqs:us-east-1:000000000000:sns-sqs-checker-wiring-missing-dlq"}`

	err = snsBk.SetSubscriptionAttributes(sub.SubscriptionArn, "RedrivePolicy", redrivePolicy)
	require.Error(
		t,
		err,
		"SetSubscriptionAttributes must reject a RedrivePolicy naming a nonexistent SQS queue "+
			"through the real cli.go composition root's wiring (SetSQSChecker alongside "+
			"SetSQSSender), not silently accept it",
	)
}

package sns_test

import (
	"context"
	"sync"
	"testing"

	"github.com/blackbirdworks/gopherstack/pkgs/events"
	"github.com/blackbirdworks/gopherstack/services/sns"
)

// noopLambdaInvoker and noopFirehosePutter are minimal stand-ins used only to
// exercise concurrent SetLambdaBackend/SetFirehoseBackend calls; delivery
// content is not asserted by these tests.
type noopLambdaInvoker struct{}

func (noopLambdaInvoker) InvokeFunction(
	_ context.Context, _, _ string, _ []byte,
) ([]byte, int, error) {
	return nil, 200, nil
}

type noopFirehosePutter struct{}

func (noopFirehosePutter) PutRecordBatch(_ string, _ [][]byte) (int, error) {
	return 0, nil
}

// TestConcurrentWiringVsPublish reproduces gopherstack-0k0: SetLambdaBackend,
// SetFirehoseBackend, and SetPublishEmitter write b.lambdaBackend/
// b.firehoseBackend/b.emitter under the backend lock, but Publish's delivery
// fan-out (deliverToLambdaSubscriptions, deliverToFirehoseSubscriptions,
// emitPublishedEvent) used to read those same fields with no lock at all —
// an unsynchronized concurrent read/write that -race flags reliably once a
// wiring call and a Publish call overlap. A production server only calls the
// Set* wiring methods once at startup before serving traffic, so this never
// surfaces there, but any concurrent test or dynamic-rewiring caller would
// hit it. Confirmed failing (WARNING: DATA RACE) against the pre-fix code.
func TestConcurrentWiringVsPublish(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		rewire   func(b *sns.InMemoryBackend)
		protocol string
		endpoint string
	}{
		{
			name:     "lambda_backend",
			rewire:   func(b *sns.InMemoryBackend) { b.SetLambdaBackend(noopLambdaInvoker{}) },
			protocol: "lambda",
			endpoint: "arn:aws:lambda:us-east-1:123456789012:function:my-fn",
		},
		{
			name:     "firehose_backend",
			rewire:   func(b *sns.InMemoryBackend) { b.SetFirehoseBackend(noopFirehosePutter{}) },
			protocol: "firehose",
			endpoint: "arn:aws:firehose:us-east-1:123456789012:deliverystream/my-stream",
		},
		{
			name: "publish_emitter",
			rewire: func(b *sns.InMemoryBackend) {
				b.SetPublishEmitter(events.NewInMemoryEmitter[*events.SNSPublishedEvent]())
			},
			protocol: "lambda",
			endpoint: "arn:aws:lambda:us-east-1:123456789012:function:my-fn",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := sns.NewInMemoryBackend()
			topic, err := b.CreateTopic("wiring-race-topic", nil)
			if err != nil {
				t.Fatalf("CreateTopic: %v", err)
			}

			if _, subErr := b.Subscribe(topic.TopicArn, tt.protocol, tt.endpoint, ""); subErr != nil {
				t.Fatalf("Subscribe: %v", subErr)
			}

			const iterations = 500

			var wg sync.WaitGroup
			wg.Add(2)

			go func() {
				defer wg.Done()

				for range iterations {
					tt.rewire(b)
				}
			}()

			go func() {
				defer wg.Done()

				for range iterations {
					_, _ = b.Publish(topic.TopicArn, "hello", "", "", nil)
				}
			}()

			wg.Wait()
		})
	}
}

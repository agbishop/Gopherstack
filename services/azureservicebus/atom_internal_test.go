// Whitebox: parseAtomEntityBody and the entry/feed builders are unexported
// and have no other seam to exercise them through directly (handler_test.go
// covers them indirectly over HTTP).
package azureservicebus

import (
	"encoding/xml"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAtomEntityBody(t *testing.T) {
	t.Parallel()

	const newNSQueueBody = `<entry xmlns="http://www.w3.org/2005/Atom">
		<content type="application/xml">
			<QueueDescription xmlns="http://schemas.microsoft.com/netservices/2010/10/servicebus/connect">
				<LockDuration>PT1M</LockDuration>
				<MaxDeliveryCount>5</MaxDeliveryCount>
				<DefaultMessageTimeToLive>P1D</DefaultMessageTimeToLive>
			</QueueDescription>
		</content>
	</entry>`

	const oldNSTopicBody = `<entry xmlns="http://www.w3.org/2005/Atom">
		<content type="application/xml">
			<TopicDescription xmlns="http://schemas.microsoft.com/servicebus/2010/10/">
				<DefaultMessageTimeToLive>PT30S</DefaultMessageTimeToLive>
			</TopicDescription>
		</content>
	</entry>`

	const subscriptionBodyWithFilter = `<entry xmlns="http://www.w3.org/2005/Atom">
		<content type="application/xml">
			<SubscriptionDescription xmlns="http://schemas.microsoft.com/netservices/2010/10/servicebus/connect">
				<LockDuration>PT5M</LockDuration>
				<MaxDeliveryCount>3</MaxDeliveryCount>
				<Rule>
					<Filter xsi:type="SqlFilter" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
						<SqlExpression>1=1</SqlExpression>
					</Filter>
				</Rule>
			</SubscriptionDescription>
		</content>
	</entry>`

	const bareQueueBody = `<entry><content><QueueDescription/></content></entry>`

	tests := []struct {
		name       string
		body       string
		wantOK     bool
		wantKind   entityKind
		wantConfig EntityConfig
	}{
		{
			name: "queue with new-style namespace and full config", body: newNSQueueBody, wantOK: true,
			wantKind: entityKindQueue,
			wantConfig: EntityConfig{
				LockDuration: time.Minute, MaxDeliveryCount: 5, DefaultMessageTTL: 24 * time.Hour,
			},
		},
		{
			name: "topic with old-style namespace", body: oldNSTopicBody, wantOK: true,
			wantKind:   entityKindTopic,
			wantConfig: EntityConfig{DefaultMessageTTL: 30 * time.Second},
		},
		{
			name: "subscription with filter rule accepted and discarded", body: subscriptionBodyWithFilter,
			wantOK: true, wantKind: entityKindSubscription,
			wantConfig: EntityConfig{LockDuration: 5 * time.Minute, MaxDeliveryCount: 3},
		},
		{
			name: "bare queue body with no properties", body: bareQueueBody, wantOK: true,
			wantKind: entityKindQueue, wantConfig: EntityConfig{},
		},
		{name: "malformed xml falls through", body: "<entry><content>", wantOK: false},
		{name: "well-formed but no known description element", body: "<entry><content/></entry>", wantOK: false},
		{name: "empty body", body: "", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := parseAtomEntityBody([]byte(tt.body))
			require.Equal(t, tt.wantOK, ok)

			if !tt.wantOK {
				return
			}

			assert.Equal(t, tt.wantKind, got.Kind)
			assert.Equal(t, tt.wantConfig, got.Config)
		})
	}
}

func TestQueueEntryXML(t *testing.T) {
	t.Parallel()

	entry := queueEntryXML(QueueInfo{
		Name: "q1", LockDuration: time.Minute, MaxDeliveryCount: 7, DefaultMessageTTL: 24 * time.Hour,
	})

	body, err := xml.Marshal(entry)
	require.NoError(t, err)

	var roundTrip atomEntryIn

	require.NoError(t, xml.Unmarshal(body, &roundTrip))
	require.NotNil(t, roundTrip.Content.QueueDescription)
	assert.Equal(t, "PT1M", roundTrip.Content.QueueDescription.LockDuration)
	assert.Equal(t, "7", roundTrip.Content.QueueDescription.MaxDeliveryCount)
	assert.Equal(t, "P1D", roundTrip.Content.QueueDescription.DefaultMessageTimeToLive)
}

func TestTopicEntryXML(t *testing.T) {
	t.Parallel()

	entry := topicEntryXML(TopicInfo{Name: "t1", DefaultMessageTTL: 30 * time.Second})

	body, err := xml.Marshal(entry)
	require.NoError(t, err)

	var roundTrip atomEntryIn

	require.NoError(t, xml.Unmarshal(body, &roundTrip))
	require.NotNil(t, roundTrip.Content.TopicDescription)
	assert.Equal(t, "PT30S", roundTrip.Content.TopicDescription.DefaultMessageTimeToLive)
}

func TestSubscriptionEntryXML(t *testing.T) {
	t.Parallel()

	entry := subscriptionEntryXML(SubscriptionInfo{Name: "s1", LockDuration: 5 * time.Minute, MaxDeliveryCount: 3})

	body, err := xml.Marshal(entry)
	require.NoError(t, err)

	var roundTrip atomEntryIn

	require.NoError(t, xml.Unmarshal(body, &roundTrip))
	require.NotNil(t, roundTrip.Content.SubscriptionDescription)
	assert.Equal(t, "PT5M", roundTrip.Content.SubscriptionDescription.LockDuration)
	assert.Equal(t, "3", roundTrip.Content.SubscriptionDescription.MaxDeliveryCount)
}

func TestQueueFeedXML(t *testing.T) {
	t.Parallel()

	feed := queueFeedXML("Queues", []QueueInfo{{Name: "a"}, {Name: "b"}})
	assert.Equal(t, "Queues", feed.Title)
	require.Len(t, feed.Entries, 2)
	assert.Equal(t, "a", feed.Entries[0].Title)
	assert.Equal(t, "b", feed.Entries[1].Title)
}

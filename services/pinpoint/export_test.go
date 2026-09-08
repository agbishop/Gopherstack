package pinpoint

import "strconv"

// MaxAppEvents exposes the per-app event cap for tests.
const MaxAppEvents = maxAppEvents

// MaxTemplateVersions exposes the template version cap for tests.
const MaxTemplateVersions = maxTemplateVersions

// TotalPerAppEntries counts every entry across the per-application maps so a
// leak (an entry surviving DeleteApp) shows up as a non-zero delta from
// baseline in tests.
func TotalPerAppEntries(b *InMemoryBackend) int {
	b.mu.RLock("TotalPerAppEntries")
	defer b.mu.RUnlock()

	return len(b.appEvents) + b.eventStreams.Len() + len(b.otpCodes) +
		len(b.appSettings) + len(b.sentMessages) +
		b.endpoints.Len() + b.channels.Len() +
		len(b.campaignVersions) + len(b.segmentVersions) +
		len(b.campaignActivities) + len(b.journeyRuns) +
		b.campaigns.Len() + b.segments.Len() + b.journeys.Len()
}

// AppEventCount returns the number of retained events for an application.
func AppEventCount(b *InMemoryBackend, appID string) int {
	b.mu.RLock("AppEventCount")
	defer b.mu.RUnlock()

	return len(b.appEvents[appID])
}

// CreateCampaignForTest creates a campaign for the app using a minimal request,
// exercising the per-app campaign maps without exporting the request type.
func CreateCampaignForTest(b *InMemoryBackend, region, accountID, appID string) error {
	_, err := b.CreateCampaign(region, accountID, appID, createCampaignRequest{})

	return err
}

// CreateCampaignIDForTest creates a campaign and returns its ID.
func CreateCampaignIDForTest(b *InMemoryBackend, region, accountID, appID string) (string, error) {
	c, err := b.CreateCampaign(region, accountID, appID, createCampaignRequest{})
	if err != nil {
		return "", err
	}

	return c.ID, nil
}

// CreateSegmentForTest creates a segment for the app using a minimal request.
func CreateSegmentForTest(b *InMemoryBackend, region, accountID, appID string) error {
	_, err := b.CreateSegment(region, accountID, appID, createSegmentRequest{})

	return err
}

// CreateSegmentIDForTest creates a segment and returns its ID.
func CreateSegmentIDForTest(b *InMemoryBackend, region, accountID, appID string) (string, error) {
	s, err := b.CreateSegment(region, accountID, appID, createSegmentRequest{})
	if err != nil {
		return "", err
	}

	return s.ID, nil
}

// CreateJourneyForTest creates a journey for the app using a minimal request.
func CreateJourneyForTest(b *InMemoryBackend, region, accountID, appID string) error {
	_, err := b.CreateJourney(region, accountID, appID, createJourneyRequest{})

	return err
}

// CreateJourneyIDForTest creates a journey and returns its ID.
func CreateJourneyIDForTest(b *InMemoryBackend, region, accountID, appID string) (string, error) {
	j, err := b.CreateJourney(region, accountID, appID, createJourneyRequest{})
	if err != nil {
		return "", err
	}

	return j.ID, nil
}

// CampaignActivityCount returns the number of stored activity entries for a campaign.
func CampaignActivityCount(b *InMemoryBackend, appID, campaignID string) int {
	b.mu.RLock("CampaignActivityCount")
	defer b.mu.RUnlock()

	return len(b.campaignActivities[appID+"/"+campaignID])
}

// JourneyRunCount returns the number of stored run entries for a journey.
func JourneyRunCount(b *InMemoryBackend, appID, journeyID string) int {
	b.mu.RLock("JourneyRunCount")
	defer b.mu.RUnlock()

	return len(b.journeyRuns[appID+"/"+journeyID])
}

// TemplateVersionCount returns the number of stored versions for a template.
func TemplateVersionCount(b *InMemoryBackend, templateName, templateType string) int {
	b.mu.RLock("TemplateVersionCount")
	defer b.mu.RUnlock()

	return len(b.templateVersionHistory[templateName+"/"+templateType])
}

// CampaignVersionCount returns the number of stored version entries for a campaign.
func CampaignVersionCount(b *InMemoryBackend, appID, campaignID string) int {
	b.mu.RLock("CampaignVersionCount")
	defer b.mu.RUnlock()

	return len(b.campaignVersions[appID+"/"+campaignID])
}

// SegmentVersionCount returns the number of stored version entries for a segment.
func SegmentVersionCount(b *InMemoryBackend, appID, segmentID string) int {
	b.mu.RLock("SegmentVersionCount")
	defer b.mu.RUnlock()

	return len(b.segmentVersions[appID+"/"+segmentID])
}

// CreateEmailTemplateForTest creates an email template with the given name.
func CreateEmailTemplateForTest(b *InMemoryBackend, region, accountID, templateName string) error {
	_, err := b.CreateEmailTemplate(region, accountID, templateName, createEmailTemplateRequest{})

	return err
}

// DeleteEmailTemplateForTest deletes an email template by name.
func DeleteEmailTemplateForTest(b *InMemoryBackend, templateName string) error {
	_, err := b.DeleteEmailTemplate(templateName)

	return err
}

// UpdateEmailTemplateForTest updates an email template (increments its version).
func UpdateEmailTemplateForTest(b *InMemoryBackend, templateName string) error {
	_, err := b.UpdateEmailTemplate(templateName, createEmailTemplateRequest{})

	return err
}

// CreateVoiceTemplateForTest creates a voice template with the given name,
// exercising the voiceTemplates table without exporting the request type.
func CreateVoiceTemplateForTest(b *InMemoryBackend, region, accountID, templateName, body string) error {
	_, err := b.CreateVoiceTemplate(region, accountID, templateName, createVoiceTemplateRequest{Body: body})

	return err
}

// UpdateEndpointForTest creates or updates an endpoint with a minimal address,
// exercising the endpoints table without exporting the request type.
func UpdateEndpointForTest(b *InMemoryBackend, appID, endpointID, address string) error {
	_, err := b.UpdateEndpoint(appID, endpointID, updateEndpointRequest{Address: address})

	return err
}

// PutEventStreamForTest creates or updates the event stream for an app,
// exercising the eventStreams table without exporting the request type.
func PutEventStreamForTest(b *InMemoryBackend, appID, destinationStreamArn, roleArn string) error {
	_, err := b.PutEventStream(appID, putEventStreamRequest{
		DestinationStreamArn: destinationStreamArn,
		RoleArn:              roleArn,
	})

	return err
}

// PutEventsForTest records n events for a single endpoint of the app, exercising
// the appEvents append/cap path without exporting the request type.
func PutEventsForTest(b *InMemoryBackend, appID string, n int) error {
	events := make(map[string]eventItem, n)
	for i := range n {
		events[strconv.Itoa(i)] = eventItem{EventType: "custom"}
	}

	req := putEventsRequest{
		BatchItem: map[string]endpointEvents{
			"endpoint-1": {Events: events},
		},
	}

	_, err := b.PutEvents(appID, req)

	return err
}

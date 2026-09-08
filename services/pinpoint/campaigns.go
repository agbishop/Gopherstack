package pinpoint

import (
	"fmt"
	"sort"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// AddCampaignInternal seeds a campaign directly without going through the HTTP layer.
func (b *InMemoryBackend) AddCampaignInternal(c *Campaign) {
	b.mu.Lock("AddCampaignInternal")
	defer b.mu.Unlock()

	b.campaigns.Put(c)
	b.arnIndex[c.ARN] = c
}

// CreateCampaign creates a new Pinpoint campaign for an application.
func (b *InMemoryBackend) CreateCampaign(
	region, accountID, appID string,
	req createCampaignRequest,
) (*Campaign, error) {
	b.mu.Lock("CreateCampaign")
	defer b.mu.Unlock()

	if _, ok := b.apps.Get(appID); !ok {
		return nil, ErrAppNotFound
	}

	id := uuid.NewString()
	campaignARN := arn.Build("mobiletargeting", region, accountID, fmt.Sprintf("apps/%s/campaigns/%s", appID, id))
	now := nowRFC3339()

	status := campaignStatusScheduled
	if req.IsPaused {
		status = campaignStatusPaused
	}

	c := &Campaign{
		ApplicationID:               appID,
		ARN:                         campaignARN,
		ID:                          id,
		Name:                        req.Name,
		SegmentID:                   req.SegmentID,
		SegmentVersion:              req.SegmentVersion,
		Tags:                        nonNilTagsCopy(req.Tags),
		MessageConfiguration:        cloneAnyMap(req.MessageConfiguration),
		Schedule:                    cloneAnyMap(req.Schedule),
		Hook:                        cloneAnyMap(req.Hook),
		Limits:                      cloneAnyMap(req.Limits),
		TemplateConfiguration:       cloneAnyMap(req.TemplateConfiguration),
		CustomDeliveryConfiguration: cloneAnyMap(req.CustomDeliveryConfiguration),
		TreatmentDescription:        req.TreatmentDescription,
		TreatmentName:               req.TreatmentName,
		Priority:                    req.Priority,
		IsPaused:                    req.IsPaused,
		Status:                      status,
		CreationDate:                now,
		LastModifiedDate:            now,
	}

	if req.AdditionalTreatments != nil {
		c.AdditionalTreatments = make([]map[string]any, len(req.AdditionalTreatments))
		for i, t := range req.AdditionalTreatments {
			c.AdditionalTreatments[i] = cloneAnyMap(t)
		}
	}

	c.Version = 1
	b.campaigns.Put(c)
	b.arnIndex[campaignARN] = c

	// Track campaign version history.
	versionKey := appID + "/" + id
	b.campaignVersions[versionKey] = []*Campaign{cloneCampaign(c)}

	// Create an initial activity record.
	actKey := appID + "/" + id
	b.campaignActivities[actKey] = []campaignActivity{
		{ApplicationID: appID, CampaignID: id, ID: uuid.NewString()},
	}

	return cloneCampaign(c), nil
}

func cloneCampaign(c *Campaign) *Campaign {
	cp := *c
	cp.Tags = nonNilTagsCopy(c.Tags)
	cp.MessageConfiguration = cloneAnyMap(c.MessageConfiguration)
	cp.Schedule = cloneAnyMap(c.Schedule)
	cp.Hook = cloneAnyMap(c.Hook)
	cp.Limits = cloneAnyMap(c.Limits)
	cp.TemplateConfiguration = cloneAnyMap(c.TemplateConfiguration)
	cp.CustomDeliveryConfiguration = cloneAnyMap(c.CustomDeliveryConfiguration)

	if c.AdditionalTreatments != nil {
		cp.AdditionalTreatments = make([]map[string]any, len(c.AdditionalTreatments))
		for i, t := range c.AdditionalTreatments {
			cp.AdditionalTreatments[i] = cloneAnyMap(t)
		}
	}

	return &cp
}

// GetCampaign retrieves a Pinpoint campaign by appID and campaignID.
func (b *InMemoryBackend) GetCampaign(appID, campaignID string) (*Campaign, error) {
	b.mu.RLock("GetCampaign")
	defer b.mu.RUnlock()

	c, ok := b.campaigns.Get(campaignID)
	if !ok || c.ApplicationID != appID {
		return nil, ErrAppNotFound
	}

	return cloneCampaign(c), nil
}

// GetCampaigns returns all campaigns for an application.
func (b *InMemoryBackend) GetCampaigns(appID string) ([]*Campaign, error) {
	b.mu.RLock("GetCampaigns")
	defer b.mu.RUnlock()

	if _, ok := b.apps.Get(appID); !ok {
		return nil, ErrAppNotFound
	}

	var campaigns []*Campaign

	for _, c := range b.campaigns.All() {
		if c.ApplicationID == appID {
			campaigns = append(campaigns, cloneCampaign(c))
		}
	}

	sort.Slice(campaigns, func(i, j int) bool {
		if campaigns[i].Name != campaigns[j].Name {
			return campaigns[i].Name < campaigns[j].Name
		}

		return campaigns[i].ID < campaigns[j].ID
	})

	return campaigns, nil
}

// applyCampaignScalarFields copies scalar fields from req into c when set.
func applyCampaignScalarFields(c *Campaign, req updateCampaignRequest) {
	if req.Name != "" {
		c.Name = req.Name
	}

	if req.SegmentID != "" {
		c.SegmentID = req.SegmentID
	}

	if req.SegmentVersion != 0 {
		c.SegmentVersion = req.SegmentVersion
	}

	if req.TreatmentDescription != "" {
		c.TreatmentDescription = req.TreatmentDescription
	}

	if req.TreatmentName != "" {
		c.TreatmentName = req.TreatmentName
	}

	if req.Priority != 0 {
		c.Priority = req.Priority
	}
}

// applyCampaignMapFields copies map fields from req into c when non-empty.
func applyCampaignMapFields(c *Campaign, req updateCampaignRequest) {
	if len(req.MessageConfiguration) > 0 {
		c.MessageConfiguration = cloneAnyMap(req.MessageConfiguration)
	}

	if len(req.Schedule) > 0 {
		c.Schedule = cloneAnyMap(req.Schedule)
	}

	if len(req.Hook) > 0 {
		c.Hook = cloneAnyMap(req.Hook)
	}

	if len(req.Limits) > 0 {
		c.Limits = cloneAnyMap(req.Limits)
	}

	if len(req.TemplateConfiguration) > 0 {
		c.TemplateConfiguration = cloneAnyMap(req.TemplateConfiguration)
	}

	if len(req.CustomDeliveryConfiguration) > 0 {
		c.CustomDeliveryConfiguration = cloneAnyMap(req.CustomDeliveryConfiguration)
	}

	if req.AdditionalTreatments != nil {
		c.AdditionalTreatments = make([]map[string]any, len(req.AdditionalTreatments))
		for i, t := range req.AdditionalTreatments {
			c.AdditionalTreatments[i] = cloneAnyMap(t)
		}
	}
}

// UpdateCampaign updates an existing Pinpoint campaign.
func (b *InMemoryBackend) UpdateCampaign(
	appID, campaignID string,
	req updateCampaignRequest,
) (*Campaign, error) {
	b.mu.Lock("UpdateCampaign")
	defer b.mu.Unlock()

	c, ok := b.campaigns.Get(campaignID)
	if !ok || c.ApplicationID != appID {
		return nil, ErrAppNotFound
	}

	applyCampaignScalarFields(c, req)
	applyCampaignMapFields(c, req)

	c.IsPaused = req.IsPaused

	if req.IsPaused {
		c.Status = campaignStatusPaused
	} else if c.Status == campaignStatusPaused {
		c.Status = campaignStatusScheduled
	}

	c.LastModifiedDate = nowRFC3339()
	c.Version++

	versionKey := appID + "/" + campaignID
	b.campaignVersions[versionKey] = append(b.campaignVersions[versionKey], cloneCampaign(c))

	return cloneCampaign(c), nil
}

// DeleteCampaign deletes a Pinpoint campaign.
func (b *InMemoryBackend) DeleteCampaign(appID, campaignID string) (*Campaign, error) {
	b.mu.Lock("DeleteCampaign")
	defer b.mu.Unlock()

	c, ok := b.campaigns.Get(campaignID)
	if !ok || c.ApplicationID != appID {
		return nil, ErrAppNotFound
	}

	b.campaigns.Delete(campaignID)
	delete(b.arnIndex, c.ARN)
	delete(b.campaignVersions, appID+"/"+campaignID)
	delete(b.campaignActivities, appID+"/"+campaignID)

	return cloneCampaign(c), nil
}

// GetCampaignDateRangeKpi returns stub KPI data for a campaign.
func (b *InMemoryBackend) GetCampaignDateRangeKpi(
	appID, campaignID, kpiName, startTime, endTime string,
) (*kpiResult, error) {
	b.mu.RLock("GetCampaignDateRangeKpi")
	defer b.mu.RUnlock()

	c, ok := b.campaigns.Get(campaignID)
	if !ok || c.ApplicationID != appID {
		return nil, ErrAppNotFound
	}

	return &kpiResult{
		ApplicationID: appID,
		CampaignID:    campaignID,
		KpiName:       kpiName,
		StartTime:     startTime,
		EndTime:       endTime,
		KpiResult:     kpiRows{Rows: []kpiRow{}},
	}, nil
}

// GetCampaignVersion returns a specific campaign version.
func (b *InMemoryBackend) GetCampaignVersion(
	appID, campaignID string,
	version int,
) (*Campaign, error) {
	b.mu.RLock("GetCampaignVersion")
	defer b.mu.RUnlock()

	versionKey := appID + "/" + campaignID
	versions := b.campaignVersions[versionKey]

	for _, v := range versions {
		if v.Version == version {
			return cloneCampaign(v), nil
		}
	}

	// AWS's GetCampaignVersion resource docs list 404 NotFoundException as
	// the documented response when "the specified resource was not found" --
	// a requested version number that isn't in this campaign's history is
	// exactly that case, so it must 404 rather than silently substitute the
	// current campaign (which would return a Version the caller didn't ask
	// for under the version they did ask for).
	return nil, ErrAppNotFound
}

// GetCampaignVersions returns all stored versions of a campaign.
func (b *InMemoryBackend) GetCampaignVersions(appID, campaignID string) ([]*Campaign, error) {
	b.mu.RLock("GetCampaignVersions")
	defer b.mu.RUnlock()

	if c, ok := b.campaigns.Get(campaignID); !ok || c.ApplicationID != appID {
		return nil, ErrAppNotFound
	}

	versionKey := appID + "/" + campaignID
	versions := b.campaignVersions[versionKey]

	result := make([]*Campaign, len(versions))
	for i, v := range versions {
		result[i] = cloneCampaign(v)
	}

	return result, nil
}

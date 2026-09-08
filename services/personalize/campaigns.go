package personalize

import (
	"fmt"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/awstime"
)

// --- Campaign ---

// latestSuffix is the CreateCampaignInput.SolutionVersionArn magic suffix
// (api_op_CreateCampaign.go field doc): to specify the latest solution
// version of a solution, specify the ARN of the solution in
// SolutionArn/$LATEST format.
const latestSuffix = "/$LATEST"

// resolveSolutionVersionArn resolves a SolutionArn/$LATEST reference to the
// solution's actual latest solution version ARN; any other input is
// returned unchanged. Callers must already hold b.mu.
func (b *InMemoryBackend) resolveSolutionVersionArn(solutionVersionArn string) (string, error) {
	solutionArn, ok := strings.CutSuffix(solutionVersionArn, latestSuffix)
	if !ok {
		return solutionVersionArn, nil
	}
	sv := b.latestSolutionVersionLocked(solutionArn)
	if sv == nil {
		return "", fmt.Errorf("%w: solution %q has no solution versions", ErrNotFound, solutionArn)
	}

	return sv.SolutionVersionArn, nil
}

// CreateCampaign creates a new campaign.
func (b *InMemoryBackend) CreateCampaign(
	name, solutionVersionArn string,
	minProvisionedTPS int32,
	campaignConfig *CampaignConfig,
	tags map[string]string,
) (*Campaign, error) {
	b.mu.Lock("CreateCampaign")
	defer b.mu.Unlock()

	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrValidation)
	}
	if b.campaigns.Has(name) {
		return nil, fmt.Errorf("%w: campaign %q already exists", ErrAlreadyExists, name)
	}
	solutionVersionArn, err := b.resolveSolutionVersionArn(solutionVersionArn)
	if err != nil {
		return nil, err
	}
	if !b.solutionVersions.Has(solutionVersionArn) {
		return nil, fmt.Errorf("%w: solution version %q not found", ErrNotFound, solutionVersionArn)
	}

	now := time.Now().UTC()
	c := &Campaign{
		CampaignArn:         b.personalizeARN("campaign", name),
		Name:                name,
		SolutionVersionArn:  solutionVersionArn,
		MinProvisionedTPS:   minProvisionedTPS,
		CampaignConfig:      campaignConfig,
		Status:              statusActive,
		CreationDateTime:    now,
		LastUpdatedDateTime: now,
	}
	b.campaigns.Put(c)
	if len(tags) > 0 {
		b.tags[c.CampaignArn] = copyStringMap(tags)
	}

	return c, nil
}

// DescribeCampaign returns a campaign by name or ARN.
func (b *InMemoryBackend) DescribeCampaign(nameOrArn string) (*Campaign, error) {
	b.mu.RLock("DescribeCampaign")
	defer b.mu.RUnlock()

	if c := b.findCampaign(nameOrArn); c != nil {
		return c, nil
	}

	return nil, fmt.Errorf("%w: campaign %q not found", ErrNotFound, nameOrArn)
}

// UpdateCampaign updates a campaign's solution version, TPS, or config. Every
// successful call records a types.CampaignUpdateSummary-shaped snapshot on
// LatestCampaignUpdate -- the real API only populates that field once the
// campaign has had at least one update.
func (b *InMemoryBackend) UpdateCampaign(
	nameOrArn, solutionVersionArn string,
	minProvisionedTPS int32,
	campaignConfig *CampaignConfig,
) (*Campaign, error) {
	b.mu.Lock("UpdateCampaign")
	defer b.mu.Unlock()

	c := b.findCampaign(nameOrArn)
	if c == nil {
		return nil, fmt.Errorf("%w: campaign %q not found", ErrNotFound, nameOrArn)
	}
	if solutionVersionArn != "" {
		resolved, err := b.resolveSolutionVersionArn(solutionVersionArn)
		if err != nil {
			return nil, err
		}
		if !b.solutionVersions.Has(resolved) {
			return nil, fmt.Errorf("%w: solution version %q not found", ErrNotFound, resolved)
		}
		c.SolutionVersionArn = resolved
	}
	if minProvisionedTPS > 0 {
		c.MinProvisionedTPS = minProvisionedTPS
	}
	if campaignConfig != nil {
		c.CampaignConfig = campaignConfig
	}
	c.LastUpdatedDateTime = time.Now().UTC()
	c.LatestCampaignUpdate = map[string]any{
		"campaignConfig":       c.CampaignConfig,
		keyCreationDateTime:    awstime.Epoch(c.LastUpdatedDateTime),
		keyLastUpdatedDateTime: awstime.Epoch(c.LastUpdatedDateTime),
		"minProvisionedTPS":    c.MinProvisionedTPS,
		keySolutionVersionArn:  c.SolutionVersionArn,
		keyStatus:              c.Status,
	}

	return c, nil
}

// DeleteCampaign removes a campaign.
func (b *InMemoryBackend) DeleteCampaign(nameOrArn string) error {
	b.mu.Lock("DeleteCampaign")
	defer b.mu.Unlock()

	c := b.findCampaign(nameOrArn)
	if c == nil {
		return fmt.Errorf("%w: campaign %q not found", ErrNotFound, nameOrArn)
	}
	b.campaigns.Delete(c.Name)
	delete(b.tags, c.CampaignArn)

	return nil
}

// ListCampaigns returns campaigns, optionally filtered by solution ARN.
func (b *InMemoryBackend) ListCampaigns(solutionArn string, maxResults int, nextToken string) ([]*Campaign, string) {
	b.mu.RLock("ListCampaigns")
	defer b.mu.RUnlock()

	all := b.campaigns.Snapshot()
	filtered := make([]*Campaign, 0, len(all))
	for _, c := range all {
		// SolutionVersionArn is SolutionArn + "/" + versionID (solutions.go:208),
		// so matching on the campaign's underlying solution requires a prefix
		// check, not equality against the bare SolutionArn a client sends.
		if solutionArn == "" || strings.HasPrefix(c.SolutionVersionArn, solutionArn+"/") {
			filtered = append(filtered, c)
		}
	}

	return paginateItems(filtered, campaignKeyFn, maxResults, nextToken)
}

func (b *InMemoryBackend) findCampaign(nameOrArn string) *Campaign {
	if c, ok := b.campaigns.Get(nameOrArn); ok {
		return c
	}
	for _, c := range b.campaigns.All() {
		if c.CampaignArn == nameOrArn {
			return c
		}
	}

	return nil
}

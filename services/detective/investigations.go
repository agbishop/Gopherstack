package detective

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Indicator type constants mirroring Amazon Detective's IndicatorType enum
// (all 8 real values: aws-sdk-go-v2/service/detective/types.IndicatorType).
const (
	indicatorImpossibleTravel    = "IMPOSSIBLE_TRAVEL"
	indicatorFlaggedIPAddress    = "FLAGGED_IP_ADDRESS"
	indicatorNewGeolocation      = "NEW_GEOLOCATION"
	indicatorNewASO              = "NEW_ASO"
	indicatorNewUserAgent        = "NEW_USER_AGENT"
	indicatorTTPObserved         = "TTP_OBSERVED"
	indicatorRelatedFinding      = "RELATED_FINDING"
	indicatorRelatedFindingGroup = "RELATED_FINDING_GROUP"
)

// validIndicatorTypes is the real IndicatorType enum (botocore
// detective/2018-10-26 service-2.json shapes.IndicatorType) -- ListIndicators
// documents ValidationException in its error set, so a filter value outside
// this set must be rejected, not silently filtered to an empty result.
var validIndicatorTypes = map[string]bool{ //nolint:gochecknoglobals // static enum lookup table, never mutated.
	indicatorImpossibleTravel:    true,
	indicatorFlaggedIPAddress:    true,
	indicatorNewGeolocation:      true,
	indicatorNewASO:              true,
	indicatorNewUserAgent:        true,
	indicatorTTPObserved:         true,
	indicatorRelatedFinding:      true,
	indicatorRelatedFindingGroup: true,
}

// builtInIndicators returns a deterministic set of indicators for an investigation.
// Real Detective derives indicators from VPC Flow Logs, CloudTrail, and GuardDuty.
// The emulator generates a fixed representative set seeded by the investigation ID
// so repeated calls for the same investigation return consistent results.
//
// Each Indicator's Detail carries exactly one populated type-specific
// sub-struct, matching the real (union-like) IndicatorDetail wire shape --
// the real API has no free-text "Title" member.
func builtInIndicators(inv *storedInvestigation) []*Indicator {
	indicators := []*Indicator{
		{
			IndicatorType: indicatorTTPObserved,
			Detail: IndicatorDetail{
				TTPsObserved: &TTPsObservedDetail{
					Tactic:          "Discovery",
					Technique:       "Permission Groups Discovery",
					Procedure:       "Enumerated IAM permissions for " + inv.EntityARN,
					APIName:         "ListRoles",
					APISuccessCount: 1,
				},
			},
		},
		{
			IndicatorType: indicatorNewGeolocation,
			Detail: IndicatorDetail{
				NewGeolocation: &NewGeolocationDetail{
					Location:              "Unknown",
					IsNewForEntireAccount: false,
				},
			},
		},
		{
			IndicatorType: indicatorNewASO,
			Detail: IndicatorDetail{
				NewASO: &NewASODetail{
					ASO:                   "Unknown",
					IsNewForEntireAccount: false,
				},
			},
		},
		{
			IndicatorType: indicatorNewUserAgent,
			Detail: IndicatorDetail{
				NewUserAgent: &NewUserAgentDetail{
					UserAgent:             "aws-sdk-unknown",
					IsNewForEntireAccount: false,
				},
			},
		},
	}

	return indicators
}

// deriveEntityType returns the IAM_ROLE or IAM_USER entity type for entityARN.
// The real StartInvestigation request has no EntityType input member -- Detective
// derives it server-side from the resource segment of the IAM entity ARN -- so
// the emulator must do the same instead of trusting a client-supplied value.
func deriveEntityType(entityARN string) (string, error) {
	parts := strings.SplitN(entityARN, ":", iamARNPartsLen)
	if len(parts) != iamARNPartsLen || parts[0] != "arn" || parts[2] != "iam" {
		return "", fmt.Errorf("%w: EntityArn must be an IAM user or role ARN", ErrValidation)
	}

	resource := parts[iamARNPartsLen-1]

	switch {
	case strings.HasPrefix(resource, "role/"):
		return entityTypeIAMRole, nil
	case strings.HasPrefix(resource, "user/"):
		return entityTypeIAMUser, nil
	default:
		return "", fmt.Errorf("%w: EntityArn must be an IAM user or role ARN", ErrValidation)
	}
}

// StartInvestigation creates a new investigation for an entity within a graph.
func (b *InMemoryBackend) StartInvestigation(
	graphARN, entityARN string,
	scopeStart, scopeEnd time.Time,
) (string, error) {
	entityType, typeErr := deriveEntityType(entityARN)
	if typeErr != nil {
		return "", typeErr
	}

	b.mu.Lock("StartInvestigation")
	defer b.mu.Unlock()

	if !b.graphs.Has(graphARN) {
		return "", ErrGraphNotFound
	}

	id := strings.ReplaceAll(uuid.NewString(), "-", "")
	now := time.Now().UTC()

	inv := &storedInvestigation{
		CreatedTime:     now,
		ScopeStartTime:  scopeStart,
		ScopeEndTime:    scopeEnd,
		GraphARN:        graphARN,
		InvestigationID: id,
		EntityARN:       entityARN,
		EntityType:      entityType,
		Severity:        severityInformational,
		State:           investigationStateActive,
		// builtInIndicators is a pure function computed synchronously, so
		// indicator derivation is already complete by the time this
		// returns -- there is no real async pipeline that could still be
		// RUNNING or ever FAILED, so completion is immediate.
		Status: investigationStatusSucceeded,
	}

	b.investigations.Put(inv)

	return id, nil
}

// GetInvestigation returns an investigation by graph ARN and investigation ID.
func (b *InMemoryBackend) GetInvestigation(graphARN, investigationID string) (*Investigation, error) {
	b.mu.RLock("GetInvestigation")
	defer b.mu.RUnlock()

	if !b.graphs.Has(graphARN) {
		return nil, ErrGraphNotFound
	}

	inv, ok := b.investigations.Get(investigationKey(graphARN, investigationID))
	if !ok {
		return nil, ErrMemberNotFound
	}

	cp := inv.toInvestigation()

	return &cp, nil
}

// ListInvestigations returns investigations for a graph.
func (b *InMemoryBackend) ListInvestigations(
	graphARN string,
	maxResults int32,
	nextToken string,
) ([]*InvestigationDetail, string, error) {
	b.mu.RLock("ListInvestigations")
	defer b.mu.RUnlock()

	if !b.graphs.Has(graphARN) {
		return nil, "", ErrGraphNotFound
	}

	items := slices.Clone(b.investigationsByGraph.Get(graphARN))
	sort.Slice(items, func(i, j int) bool { return items[i].InvestigationID < items[j].InvestigationID })

	start, err := decodePageToken(nextToken)
	if err != nil {
		return nil, "", err
	}

	if start > len(items) {
		start = len(items)
	}

	limit := int(maxResults)
	if limit <= 0 || limit > maxInvestigationsPerPage {
		limit = maxInvestigationsPerPage
	}

	end := min(start+limit, len(items))

	result := make([]*InvestigationDetail, 0, end-start)
	for _, inv := range items[start:end] {
		d := inv.toDetail()
		result = append(result, &d)
	}

	var outToken string
	if end < len(items) {
		outToken = encodePageToken(end)
	}

	return result, outToken, nil
}

// UpdateInvestigationState updates the state of an investigation.
func (b *InMemoryBackend) UpdateInvestigationState(graphARN, investigationID, state string) error {
	if state != investigationStateActive && state != investigationStateArchived {
		return fmt.Errorf("%w: invalid State %q", ErrValidation, state)
	}

	b.mu.Lock("UpdateInvestigationState")
	defer b.mu.Unlock()

	if !b.graphs.Has(graphARN) {
		return ErrGraphNotFound
	}

	inv, ok := b.investigations.Get(investigationKey(graphARN, investigationID))
	if !ok {
		return ErrMemberNotFound
	}

	inv.State = state

	return nil
}

// ListIndicators returns indicators for an investigation.
func (b *InMemoryBackend) ListIndicators(
	graphARN, investigationID, indicatorType string,
	maxResults int32,
	nextToken string,
) ([]*Indicator, string, error) {
	if indicatorType != "" && !validIndicatorTypes[indicatorType] {
		return nil, "", fmt.Errorf("%w: invalid IndicatorType %q", ErrValidation, indicatorType)
	}

	b.mu.RLock("ListIndicators")
	defer b.mu.RUnlock()

	if !b.graphs.Has(graphARN) {
		return nil, "", ErrGraphNotFound
	}

	inv, ok := b.investigations.Get(investigationKey(graphARN, investigationID))
	if !ok {
		return nil, "", ErrMemberNotFound
	}

	all := builtInIndicators(inv)

	if indicatorType != "" {
		filtered := make([]*Indicator, 0, len(all))
		for _, ind := range all {
			if ind.IndicatorType == indicatorType {
				filtered = append(filtered, ind)
			}
		}
		all = filtered
	}

	start, err := decodePageToken(nextToken)
	if err != nil {
		return nil, "", err
	}

	if start > len(all) {
		start = len(all)
	}

	limit := int(maxResults)
	if limit <= 0 || limit > maxIndicatorsPerPage {
		limit = maxIndicatorsPerPage
	}

	end := min(start+limit, len(all))

	var outToken string
	if end < len(all) {
		outToken = encodePageToken(end)
	}

	return all[start:end], outToken, nil
}

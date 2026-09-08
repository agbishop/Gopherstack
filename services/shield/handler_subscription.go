package shield

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
)

func (h *Handler) handleCreateSubscription(ctx context.Context) error {
	if err := h.Backend.CreateSubscription(); err != nil {
		// Shield returns empty body on success; ignore "already exists" per AWS behavior
		if errors.Is(err, awserr.ErrConflict) {
			return nil
		}

		return err
	}

	log := logger.Load(ctx)
	log.InfoContext(ctx, "shield: created subscription")

	return nil
}

// subscriptionLimits returns the standard Shield Advanced subscription limits.
func subscriptionLimits() map[string]any {
	const maxPerType = int64(subscriptionMaxProtectionsPerType)

	// No "MaxProtections" key: types.ProtectionLimits (shield@v1.37.4
	// types/types.go) has only ProtectedResourceTypeLimits.
	return map[string]any{
		"ProtectionLimits": map[string]any{
			"ProtectedResourceTypeLimits": []map[string]any{
				{keyType: ResourceTypeCloudFrontDistribution, keyMax: maxPerType},
				{keyType: ResourceTypeRoute53HostedZone, keyMax: maxPerType},
				{keyType: ResourceTypeApplicationLoadBalancer, keyMax: maxPerType},
				{keyType: ResourceTypeClassicLoadBalancer, keyMax: maxPerType},
				{keyType: ResourceTypeElasticIPAllocation, keyMax: maxPerType},
				{keyType: ResourceTypeGlobalAccelerator, keyMax: maxPerType},
			},
		},
		"ProtectionGroupLimits": map[string]any{
			"MaxProtectionGroups": int64(subscriptionMaxProtectionGroups),
			"PatternTypeLimits": map[string]any{
				"ArbitraryPatternLimits": map[string]any{
					"MaxMembers": int64(subscriptionMaxMembersPerGroup),
				},
			},
		},
	}
}

// subscriptionResourceLimits returns per-resource-type max count limits.
func subscriptionResourceLimits() []map[string]any {
	const maxInt = int64(100)

	return []map[string]any{
		{keyType: ResourceTypeCloudFrontDistribution, keyMax: maxInt},
		{keyType: ResourceTypeRoute53HostedZone, keyMax: maxInt},
		{keyType: ResourceTypeApplicationLoadBalancer, keyMax: maxInt},
		{keyType: ResourceTypeClassicLoadBalancer, keyMax: maxInt},
		{keyType: ResourceTypeElasticIPAllocation, keyMax: maxInt},
		{keyType: ResourceTypeGlobalAccelerator, keyMax: maxInt},
	}
}

func (h *Handler) handleDescribeSubscription() ([]byte, error) {
	sub, err := h.Backend.DescribeSubscription()
	if err != nil {
		return nil, err
	}

	// Gap 22: correct SubscriptionArn format — no trailing path segment.
	subscriptionArn := arn.Build("shield", "", h.Backend.AccountID(), "subscription")

	return json.Marshal(map[string]any{
		"Subscription": map[string]any{
			keyStartTime: floatSeconds(sub.StartTime),
			keyEndTime:   floatSeconds(sub.EndTime),
			"AutoRenew":  sub.AutoRenew,
			// AWS wire field is TimeCommitmentInSeconds (seconds), not days -- see
			// types.Subscription.TimeCommitmentInSeconds in aws-sdk-go-v2.
			"TimeCommitmentInSeconds":   sub.TimeCommitmentInDays * secondsPerDay,
			"SubscriptionArn":           subscriptionArn,
			"ProactiveEngagementStatus": h.Backend.GetProactiveEngagementStatus(),
			"Limits":                    subscriptionResourceLimits(),
			"SubscriptionLimits":        subscriptionLimits(),
		},
	})
}

// updateSubscriptionRequest is the request body for UpdateSubscription.
type updateSubscriptionRequest struct {
	AutoRenew string `json:"AutoRenew"`
}

func (h *Handler) handleUpdateSubscription(body []byte) error {
	var req updateSubscriptionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if req.AutoRenew != "" && req.AutoRenew != AutoRenewEnabled && req.AutoRenew != AutoRenewDisabled {
		return fmt.Errorf("%w: AutoRenew must be ENABLED or DISABLED", errInvalidRequest)
	}

	return h.Backend.UpdateSubscription(req.AutoRenew)
}

func (h *Handler) handleGetSubscriptionState() ([]byte, error) {
	state := h.Backend.GetSubscriptionState()

	return json.Marshal(map[string]string{
		"SubscriptionState": state,
	})
}

func (h *Handler) handleDeleteSubscription() error {
	return h.Backend.DeleteSubscription()
}

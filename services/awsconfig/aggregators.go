package awsconfig

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// aggregationAuthKey returns a composite key for an aggregation authorization.
func aggregationAuthKey(accountID, region string) string {
	return accountID + "#" + region
}

// PutAggregationAuthorization creates or updates an aggregation authorization.
func (b *InMemoryBackend) PutAggregationAuthorization(accountID, region string, tags []Tag) error {
	b.mu.Lock("PutAggregationAuthorization")
	defer b.mu.Unlock()

	arn := fmt.Sprintf(
		"arn:aws:config:%s:%s:aggregation-authorization/%s/%s",
		b.region, b.accountID, accountID, region,
	)

	b.aggregationAuths.Put(&AggregationAuthorization{
		AuthorizedAccountID:         accountID,
		AuthorizedAwsRegion:         region,
		AggregationAuthorizationArn: arn,
	})
	b.setResourceTagsLocked(arn, tags)

	return nil
}

// DescribeAggregationAuthorizations returns all aggregation authorizations sorted by
// account ID then region.
func (b *InMemoryBackend) DescribeAggregationAuthorizations() []AggregationAuthorization {
	b.mu.RLock("DescribeAggregationAuthorizations")
	defer b.mu.RUnlock()

	all := b.aggregationAuths.All()
	out := make([]AggregationAuthorization, 0, len(all))

	for _, a := range all {
		out = append(out, *a)
	}

	slices.SortFunc(out, func(a, b AggregationAuthorization) int {
		if a.AuthorizedAccountID != b.AuthorizedAccountID {
			if a.AuthorizedAccountID < b.AuthorizedAccountID {
				return -1
			}

			return 1
		}

		if a.AuthorizedAwsRegion < b.AuthorizedAwsRegion {
			return -1
		}

		if a.AuthorizedAwsRegion > b.AuthorizedAwsRegion {
			return 1
		}

		return 0
	})

	return out
}

// DeleteAggregationAuthorization deletes an aggregation authorization by account ID
// and region. Real AWS Config's DeleteAggregationAuthorization is idempotent -- its
// error model (verified against aws-sdk-go-v2/service/configservice's deserializer)
// only lists InvalidParameterValueException, never a not-found exception -- so
// deleting a nonexistent authorization succeeds silently, matching AWS.
func (b *InMemoryBackend) DeleteAggregationAuthorization(accountID, region string) error {
	if accountID == "" {
		return fmt.Errorf("%w: AuthorizedAccountId is required", ErrInvalidParameterValue)
	}

	if region == "" {
		return fmt.Errorf("%w: AuthorizedAwsRegion is required", ErrInvalidParameterValue)
	}

	b.mu.Lock("DeleteAggregationAuthorization")
	defer b.mu.Unlock()

	b.aggregationAuths.Delete(aggregationAuthKey(accountID, region))

	return nil
}

// PutConfigurationAggregator creates or updates a configuration aggregator.
// A pre-existing aggregator keeps its ARN across updates, matching real AWS
// Config Put (create-or-update) semantics.
func (b *InMemoryBackend) PutConfigurationAggregator(
	name string,
	accountSources []AccountAggregationSource,
	orgSource *OrganizationAggregationSource,
	tags []Tag,
) error {
	if name == "" {
		return fmt.Errorf("%w: ConfigurationAggregatorName is required", ErrInvalidParameterValue)
	}

	b.mu.Lock("PutConfigurationAggregator")
	defer b.mu.Unlock()

	arn, ok := existingAggregatorArnLocked(b, name)
	if !ok {
		b.aggregatorCounter++
		arn = fmt.Sprintf(
			"arn:aws:config:%s:%s:config-aggregator/config-aggregator-%08d",
			b.region, b.accountID, b.aggregatorCounter,
		)
	}

	b.aggregators.Put(&ConfigurationAggregator{
		ConfigurationAggregatorName:   name,
		ConfigurationAggregatorArn:    arn,
		AccountAggregationSources:     accountSources,
		OrganizationAggregationSource: orgSource,
	})
	b.setResourceTagsLocked(arn, tags)

	return nil
}

// existingAggregatorArnLocked returns the ARN of the aggregator named name, if any.
func existingAggregatorArnLocked(b *InMemoryBackend, name string) (string, bool) {
	existing, ok := b.aggregators.Get(name)
	if !ok {
		return "", false
	}

	return existing.ConfigurationAggregatorArn, true
}

// DeleteConfigurationAggregator deletes a configuration aggregator by name.
func (b *InMemoryBackend) DeleteConfigurationAggregator(name string) error {
	if name == "" {
		// DeleteConfigurationAggregator's deserializer declares only
		// NoSuchConfigurationAggregatorException -- no validation-shaped code fits an
		// empty name (verified against configservice@v1.68.4's deserializers.go).
		return fmt.Errorf("%w: ConfigurationAggregatorName is required", ErrValidation)
	}

	b.mu.Lock("DeleteConfigurationAggregator")
	defer b.mu.Unlock()

	if !b.aggregators.Has(name) {
		return fmt.Errorf("%w: %s", ErrNoSuchAggregator, name)
	}

	b.aggregators.Delete(name)

	return nil
}

// DescribeConfigurationAggregators returns all aggregators sorted by name.
func (b *InMemoryBackend) DescribeConfigurationAggregators() []ConfigurationAggregator {
	b.mu.RLock("DescribeConfigurationAggregators")
	defer b.mu.RUnlock()

	all := b.aggregators.All()
	out := make([]ConfigurationAggregator, 0, len(all))

	for _, a := range all {
		out = append(out, *a)
	}

	return out
}

// requireAggregatorLocked errors NoSuchConfigurationAggregatorException when
// name does not identify a configured aggregator, matching every aggregate-*
// operation's declared error model (verified against
// aws-sdk-go-v2/service/configservice's GetAggregateComplianceDetailsByConfigRule/
// GetAggregateConfigRuleComplianceSummary/GetAggregateConformancePackComplianceSummary/
// DescribeAggregateComplianceByConformancePacks/ListAggregateDiscoveredResources/
// DescribeConfigurationAggregatorSourcesStatus deserializers, which all declare
// it). Caller must already hold at least a read lock.
func (b *InMemoryBackend) requireAggregatorLocked(name string) error {
	if !b.aggregators.Has(name) {
		return fmt.Errorf("%w: %s", ErrNoSuchAggregator, name)
	}

	return nil
}

// DescribeConfigurationAggregatorSourcesStatus returns one status entry per
// account/region source configured on the aggregator (from
// PutConfigurationAggregator's AccountAggregationSources/
// OrganizationAggregationSource, already stored on the aggregator), reporting
// SUCCEEDED since this emulator has no real per-source sync failures to model.
func (b *InMemoryBackend) DescribeConfigurationAggregatorSourcesStatus(
	aggregatorName string,
) ([]AggregatedSourceStatus, error) {
	b.mu.RLock("DescribeConfigurationAggregatorSourcesStatus")
	defer b.mu.RUnlock()

	if err := b.requireAggregatorLocked(aggregatorName); err != nil {
		return nil, err
	}

	agg, _ := b.aggregators.Get(aggregatorName)
	now := float64(time.Now().Unix())
	out := make([]AggregatedSourceStatus, 0)

	for _, src := range agg.AccountAggregationSources {
		regions := src.AwsRegions
		if src.AllAwsRegions || len(regions) == 0 {
			regions = []string{b.region}
		}

		for _, acct := range src.AccountIDs {
			for _, region := range regions {
				out = append(out, AggregatedSourceStatus{
					SourceID: acct, SourceType: "ACCOUNT", AwsRegion: region,
					LastUpdateStatus: statusSucceeded, LastUpdateTime: now,
				})
			}
		}
	}

	if agg.OrganizationAggregationSource != nil {
		regions := agg.OrganizationAggregationSource.AwsRegions
		if agg.OrganizationAggregationSource.AllAwsRegions || len(regions) == 0 {
			regions = []string{b.region}
		}

		for _, region := range regions {
			out = append(out, AggregatedSourceStatus{
				SourceID: b.accountID, SourceType: "ORGANIZATION", AwsRegion: region,
				LastUpdateStatus: statusSucceeded, LastUpdateTime: now,
			})
		}
	}

	return out, nil
}

// aggregatorConsumes reports whether sources (an aggregator's configured
// AccountAggregationSources) already incorporates accountID+region.
func aggregatorConsumes(sources []AccountAggregationSource, accountID, region string) bool {
	for _, s := range sources {
		if !slices.Contains(s.AccountIDs, accountID) {
			continue
		}

		if s.AllAwsRegions || slices.Contains(s.AwsRegions, region) {
			return true
		}
	}

	return false
}

// DescribePendingAggregationRequests returns every aggregation authorization
// this account has granted (via PutAggregationAuthorization) that no local
// configuration aggregator has yet incorporated into its
// AccountAggregationSources -- the only cross-account "pending" state a
// single-account emulator can genuinely derive, since b.aggregationAuths
// already records exactly which (account, region) pairs were granted
// permission to aggregate this account's data.
func (b *InMemoryBackend) DescribePendingAggregationRequests() []PendingAggregationRequest {
	b.mu.RLock("DescribePendingAggregationRequests")
	defer b.mu.RUnlock()

	aggregators := b.aggregators.All()
	out := make([]PendingAggregationRequest, 0)

	for _, auth := range b.aggregationAuths.All() {
		consumed := false

		for _, agg := range aggregators {
			if aggregatorConsumes(agg.AccountAggregationSources, auth.AuthorizedAccountID, auth.AuthorizedAwsRegion) {
				consumed = true

				break
			}
		}

		if !consumed {
			out = append(out, PendingAggregationRequest{
				RequesterAccountID: auth.AuthorizedAccountID,
				RequesterAwsRegion: auth.AuthorizedAwsRegion,
			})
		}
	}

	slices.SortFunc(out, func(a, c PendingAggregationRequest) int {
		if a.RequesterAccountID != c.RequesterAccountID {
			return strings.Compare(a.RequesterAccountID, c.RequesterAccountID)
		}

		return strings.Compare(a.RequesterAwsRegion, c.RequesterAwsRegion)
	})

	return out
}

// DeletePendingAggregationRequest dismisses a pending aggregation request,
// removing the underlying aggregation authorization it was derived from.
// Idempotent -- like DeleteAggregationAuthorization, real AWS Config's
// declared error model for this op has no not-found exception (verified
// against aws-sdk-go-v2/service/configservice's DeletePendingAggregationRequest
// deserializer, which declares only InvalidParameterValueException).
func (b *InMemoryBackend) DeletePendingAggregationRequest(accountID, region string) error {
	if accountID == "" {
		return fmt.Errorf("%w: RequesterAccountId is required", ErrInvalidParameterValue)
	}

	if region == "" {
		return fmt.Errorf("%w: RequesterAwsRegion is required", ErrInvalidParameterValue)
	}

	b.mu.Lock("DeletePendingAggregationRequest")
	defer b.mu.Unlock()

	b.aggregationAuths.Delete(aggregationAuthKey(accountID, region))

	return nil
}

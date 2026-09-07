package awsconfig

import (
	"fmt"
	"slices"
	"time"
)

// PutRemediationConfigurations stores remediation configurations keyed by rule name.
func (b *InMemoryBackend) PutRemediationConfigurations(configs []RemediationConfiguration) error {
	b.mu.Lock("PutRemediationConfigurations")
	defer b.mu.Unlock()

	for i := range configs {
		cp := configs[i]
		b.remediationConfigs.Put(&cp)
	}

	return nil
}

// DescribeRemediationConfigurations returns remediation configurations for the given rule names.
// If ruleNames is empty, all configurations are returned.
func (b *InMemoryBackend) DescribeRemediationConfigurations(ruleNames []string) []RemediationConfiguration {
	b.mu.RLock("DescribeRemediationConfigurations")
	defer b.mu.RUnlock()

	if len(ruleNames) == 0 {
		all := b.remediationConfigs.All()
		out := make([]RemediationConfiguration, 0, len(all))

		for _, rc := range all {
			out = append(out, *rc)
		}

		return out
	}

	out := make([]RemediationConfiguration, 0, len(ruleNames))

	for _, name := range ruleNames {
		if rc, ok := b.remediationConfigs.Get(name); ok {
			out = append(out, *rc)
		}
	}

	return out
}

// PutRemediationExceptions stores a remediation exception per resource key
// for a rule (real Config adds one exception per key in the request, e.g. 3
// exceptions for 3 resource keys).
func (b *InMemoryBackend) PutRemediationExceptions(ruleName string, keys []RemediationExceptionResourceKey) error {
	b.mu.Lock("PutRemediationExceptions")
	defer b.mu.Unlock()

	for _, k := range keys {
		b.putRemediationExceptionLocked(ruleName, k.ResourceType, k.ResourceID)
	}

	return nil
}

// putRemediationExceptionLocked upserts a single exception; callers must hold b.mu.
func (b *InMemoryBackend) putRemediationExceptionLocked(ruleName, resourceType, resourceID string) {
	ex := RemediationException{
		ConfigRuleName: ruleName,
		ResourceType:   resourceType,
		ResourceID:     resourceID,
	}

	existing := b.remediationExceptions[ruleName]

	for i, e := range existing {
		if e.ResourceType == resourceType && e.ResourceID == resourceID {
			existing[i] = ex

			b.remediationExceptions[ruleName] = existing

			return
		}
	}

	b.remediationExceptions[ruleName] = append(existing, ex)
}

// DescribeRemediationExceptions returns all remediation exceptions for the given rule name.
func (b *InMemoryBackend) DescribeRemediationExceptions(ruleName string) []RemediationException {
	b.mu.RLock("DescribeRemediationExceptions")
	defer b.mu.RUnlock()

	exs := b.remediationExceptions[ruleName]
	if len(exs) == 0 {
		return []RemediationException{}
	}

	out := make([]RemediationException, len(exs))
	copy(out, exs)

	return out
}

// DeleteRemediationConfiguration removes the remediation configuration for the
// given rule, cascade-deleting any recorded remediation executions for it too
// (StartRemediationExecution/DescribeRemediationExecutionStatus both require a
// remediation configuration to exist, so leaving them behind would strand
// permanently-unreachable rows instead of a clean delete). Errors with
// ErrNoSuchRemediationConfiguration when ruleName has no remediation
// configuration, matching real AWS Config's declared error model (verified
// against aws-sdk-go-v2/service/configservice's DeleteRemediationConfiguration
// deserializer).
func (b *InMemoryBackend) DeleteRemediationConfiguration(ruleName string) error {
	b.mu.Lock("DeleteRemediationConfiguration")
	defer b.mu.Unlock()

	if !b.remediationConfigs.Has(ruleName) {
		return fmt.Errorf("%w: %s", ErrNoSuchRemediationConfiguration, ruleName)
	}

	b.remediationConfigs.Delete(ruleName)

	for _, e := range slices.Clone(b.remediationExecutionsByRule.Get(ruleName)) {
		b.remediationExecutions.Delete(remediationExecutionKeyFn(e))
	}

	delete(b.remediationExceptions, ruleName)

	return nil
}

// DeleteRemediationExceptions removes the exceptions matching the given
// resource keys (type + ID) for a rule. Real Config's declared error model
// for this op has no ValidationException (only NoSuchRemediationException,
// surfaced per-item via FailedBatches -- not modeled, since gopherstack never
// fails to delete an exception it holds), so an empty ruleName/keys is
// treated as a no-op rather than an invented validation error.
func (b *InMemoryBackend) DeleteRemediationExceptions(ruleName string, keys []RemediationExceptionResourceKey) error {
	b.mu.Lock("DeleteRemediationExceptions")
	defer b.mu.Unlock()

	if len(keys) == 0 {
		return nil
	}

	existing := b.remediationExceptions[ruleName]
	filtered := existing[:0]

	for _, e := range existing {
		if !containsRemediationExceptionKey(keys, e.ResourceType, e.ResourceID) {
			filtered = append(filtered, e)
		}
	}

	b.remediationExceptions[ruleName] = filtered

	return nil
}

// containsRemediationExceptionKey reports whether keys contains a
// (resourceType, resourceID) pair.
func containsRemediationExceptionKey(keys []RemediationExceptionResourceKey, resourceType, resourceID string) bool {
	for _, k := range keys {
		if k.ResourceType == resourceType && k.ResourceID == resourceID {
			return true
		}
	}

	return false
}

// remediationExecutionStepName is the single synthetic step this emulator
// records for every remediation execution, since it has no real SSM Automation
// runner to model multi-step document execution.
const remediationExecutionStepName = "ExecuteAutomation"

// StartRemediationExecution runs the remediation configured for ruleName
// against each resource key, recording a SUCCEEDED execution for each
// (readable back via DescribeRemediationExecutionStatus) since this emulator
// has no real SSM Automation runner to execute against. Errors with
// ErrNoSuchRemediationConfiguration (wire type
// NoSuchRemediationConfigurationException) when ruleName has no remediation
// configuration, matching real AWS Config's declared error model (verified
// against aws-sdk-go-v2/service/configservice's StartRemediationExecution
// deserializer).
func (b *InMemoryBackend) StartRemediationExecution(ruleName string, keys []ResourceKey) error {
	if ruleName == "" {
		return fmt.Errorf("%w: ConfigRuleName is required", ErrInvalidParameterValue)
	}

	b.mu.Lock("StartRemediationExecution")
	defer b.mu.Unlock()

	if !b.remediationConfigs.Has(ruleName) {
		return fmt.Errorf("%w: %s", ErrNoSuchRemediationConfiguration, ruleName)
	}

	now := float64(time.Now().Unix())

	for _, k := range keys {
		b.remediationExecutions.Put(&RemediationExecutionStatusEntry{
			RuleName:        ruleName,
			ResourceKey:     k,
			State:           statusSucceeded,
			InvocationTime:  now,
			LastUpdatedTime: now,
			StepDetails: []RemediationExecutionStepStatus{{
				Name: remediationExecutionStepName, State: statusSucceeded, StartTime: now, StopTime: now,
			}},
		})
	}

	return nil
}

// DescribeRemediationExecutionStatus returns the recorded remediation
// executions for ruleName, optionally narrowed to specific resource keys.
// Errors with ErrNoSuchRemediationConfiguration when ruleName has no
// remediation configuration, matching real AWS Config's declared error model
// (verified against aws-sdk-go-v2/service/configservice's
// DescribeRemediationExecutionStatus deserializer).
func (b *InMemoryBackend) DescribeRemediationExecutionStatus(
	ruleName string,
	keys []ResourceKey,
) ([]RemediationExecutionStatusEntry, error) {
	b.mu.RLock("DescribeRemediationExecutionStatus")
	defer b.mu.RUnlock()

	if !b.remediationConfigs.Has(ruleName) {
		return nil, fmt.Errorf("%w: %s", ErrNoSuchRemediationConfiguration, ruleName)
	}

	all := b.remediationExecutionsByRule.Get(ruleName)

	out := make([]RemediationExecutionStatusEntry, 0, len(all))

	for _, e := range all {
		if len(keys) > 0 && !containsResourceKey(keys, e.ResourceKey) {
			continue
		}

		out = append(out, *e)
	}

	return out, nil
}

// containsResourceKey reports whether keys contains key.
func containsResourceKey(keys []ResourceKey, key ResourceKey) bool {
	for _, k := range keys {
		if k.ResourceType == key.ResourceType && k.ResourceID == key.ResourceID {
			return true
		}
	}

	return false
}

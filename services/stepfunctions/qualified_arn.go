package stepfunctions

import (
	"fmt"
	"math/rand/v2"
)

// resolvedExecutionTarget is the outcome of resolving a stateMachineArn
// argument to StartExecution/StartSyncExecution, which AWS documents as
// accepting three shapes: an unqualified state machine ARN, a version ARN
// (stateMachineArn:N), or an alias ARN (stateMachineArn:name). See the
// StartExecutionInput.StateMachineArn doc in aws-sdk-go-v2/service/sfn.
//
// SM carries the resolved Definition/RoleArn/Type to execute (frozen at the
// target version for qualified ARNs) but its StateMachineArn field is always
// the base *unqualified* ARN: AWS never includes a version/alias qualifier
// in execution ARNs, so all downstream ARN construction (execARN, map run
// ARNs, etc.) must key off the base ARN, not the ARN the caller passed in.
// VersionArn/AliasArn record which qualifier (if any) was used, for callers
// that need to stamp them onto the resulting Execution record.
type resolvedExecutionTarget struct {
	SM         *StateMachine
	VersionArn string
	AliasArn   string
}

// resolveExecutionTarget resolves stateMachineArn (unqualified, version-
// qualified, or alias-qualified) to the concrete state machine definition to
// run. Caller must hold b.mu (read lock is sufficient; this never mutates
// state).
func (b *InMemoryBackend) resolveExecutionTarget(stateMachineArn string) (*resolvedExecutionTarget, error) {
	if sm, ok := b.stateMachines.Get(stateMachineArn); ok {
		cp := *sm

		return &resolvedExecutionTarget{SM: &cp}, nil
	}

	if v, ok := b.versions.Get(stateMachineArn); ok {
		sm, err := b.stateMachineForVersionLocked(v)
		if err != nil {
			return nil, err
		}

		return &resolvedExecutionTarget{SM: sm, VersionArn: v.StateMachineVersionArn}, nil
	}

	if a, ok := b.aliases.Get(stateMachineArn); ok {
		targetVersionArn, err := pickRoutedVersion(a.RoutingConfiguration)
		if err != nil {
			return nil, err
		}

		v, vOK := b.versions.Get(targetVersionArn)
		if !vOK {
			return nil, fmt.Errorf("%w: %s", ErrStateMachineVersionDoesNotExist, targetVersionArn)
		}

		sm, err := b.stateMachineForVersionLocked(v)
		if err != nil {
			return nil, err
		}

		return &resolvedExecutionTarget{SM: sm, VersionArn: v.StateMachineVersionArn, AliasArn: stateMachineArn}, nil
	}

	return nil, fmt.Errorf("%w: %s", ErrStateMachineDoesNotExist, stateMachineArn)
}

// stateMachineForVersionLocked builds a synthetic *StateMachine reflecting a
// published version's frozen Definition/RoleArn/Type, keyed by the base
// (unqualified) state machine ARN. Caller must hold b.mu.
func (b *InMemoryBackend) stateMachineForVersionLocked(v *StateMachineVersion) (*StateMachine, error) {
	base, ok := b.stateMachines.Get(v.StateMachineArn)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrStateMachineDoesNotExist, v.StateMachineArn)
	}

	cp := *base
	cp.Definition = v.Definition
	cp.RoleArn = v.RoleArn
	cp.Type = v.Type

	return &cp, nil
}

// pickRoutedVersion selects a target version ARN from an alias's routing
// configuration. AWS aliases route to 1 or 2 versions; with 2, traffic is
// split by weight (validateRoutingConfig already enforces weights sum to
// 100 and there are 1-2 entries, so this never needs to handle more).
func pickRoutedVersion(routing []AliasRoutingConfig) (string, error) {
	if len(routing) == 0 {
		// Unreachable today (gopherstack-t8iz): CreateStateMachineAlias
		// validates routing, UpdateStateMachineAlias only assigns when
		// len(routing) > 0, and Restore never persists aliases at all. If it
		// ever fires via StartSyncExecution, that op declares nothing fitting
		// -- ValidationException included -- so this needs a remedy, not a
		// code swap.
		return "", fmt.Errorf("%w: alias has no routing configuration", ErrInvalidRoutingConfiguration)
	}

	if len(routing) == 1 {
		return routing[0].StateMachineVersionArn, nil
	}

	const maxWeight = 100

	r := rand.IntN(maxWeight) //nolint:gosec // non-cryptographic weighted routing pick, matches ASL retry jitter
	if r < routing[0].Weight {
		return routing[0].StateMachineVersionArn, nil
	}

	return routing[1].StateMachineVersionArn, nil
}

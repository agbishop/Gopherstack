package rds

import "fmt"

// isValidOptInType reports whether optInType is one of
// ApplyPendingMaintenanceActionInput.OptInType's legal values, taken from
// its own doc comment (rds@v1.124.1 api_op_ApplyPendingMaintenanceAction.go:53-63)
// since OptInType is an untyped *string on the real SDK, not a generated
// enum type with a Values() method. A function (not a package var) to stay
// lint-clean without a global, matching this repo's builtInConnectionTypes()
// convention.
func isValidOptInType(optInType string) bool {
	switch optInType {
	case "immediate", "next-maintenance", "undo-opt-in":
		return true
	default:
		return false
	}
}

// ApplyPendingMaintenanceAction applies a pending maintenance action to a resource.
// The resource is identified by its ARN. This implementation validates the resource exists
// and returns a stub response. OptInType is required and validated against
// its documented enum, but this backend has no mechanism anywhere that ever
// generates a real pending maintenance action for a resource (see
// DescribePendingMaintenanceActions, hardcoded to return none), so
// OptInType's immediate/next-window/undo semantics have no state to act on
// -- validated and rejected if invalid, not silently accepted, but not
// wired to any further effect.
func (b *InMemoryBackend) ApplyPendingMaintenanceAction(
	resourceID, applyAction, optInType string,
) (string, error) {
	if resourceID == "" {
		return "", fmt.Errorf("%w: ResourceIdentifier must not be empty", ErrInvalidParameter)
	}
	if applyAction == "" {
		return "", fmt.Errorf("%w: ApplyAction must not be empty", ErrInvalidParameter)
	}
	if optInType == "" {
		return "", fmt.Errorf("%w: OptInType must not be empty", ErrInvalidParameter)
	}
	if !isValidOptInType(optInType) {
		return "", fmt.Errorf(
			"%w: OptInType must be one of immediate, next-maintenance, undo-opt-in; got %q",
			ErrInvalidParameter, optInType,
		)
	}

	b.mu.RLock("ApplyPendingMaintenanceAction")
	defer b.mu.RUnlock()

	id := rdsIDFromARN(resourceID)

	// Validate that the referenced resource exists (instance or cluster).
	if _, ok := b.instances.Get(normalizeID(id)); !ok {
		if _, ok2 := b.clusters.Get(normalizeID(id)); !ok2 {
			return "", fmt.Errorf("%w: resource %s not found", ErrResourceNotFound, resourceID)
		}
	}

	return resourceID, nil
}

// DescribePendingMaintenanceActions returns pending maintenance actions.
func (b *InMemoryBackend) DescribePendingMaintenanceActions(_ string) []PendingMaintenanceAction {
	return []PendingMaintenanceAction{}
}

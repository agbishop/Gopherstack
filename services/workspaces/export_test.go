package workspaces

// WorkspaceCount returns the number of stored workspaces.
func WorkspaceCount(b *InMemoryBackend) int {
	b.mu.RLock("WorkspaceCount")
	defer b.mu.RUnlock()

	return b.workspaces.Len()
}

// HandlerOpsLen returns the count of GetSupportedOperations.
func HandlerOpsLen(h *Handler) int {
	return len(h.GetSupportedOperations())
}

// WorkspaceState returns the state of a workspace by ID.
func WorkspaceState(b *InMemoryBackend, id string) string {
	b.mu.RLock("WorkspaceState")
	defer b.mu.RUnlock()

	w, ok := b.workspaces.Get(id)
	if !ok {
		return ""
	}

	return w.State
}

// SetWorkspaceState force-sets a workspace's state, bypassing ModifyWorkspaceState's
// AVAILABLE/ADMIN_MAINTENANCE restriction, for tests that need to isolate the
// running-mode half of Start/StopWorkspaces's precondition from the state half.
func SetWorkspaceState(b *InMemoryBackend, id, state string) {
	b.mu.Lock("SetWorkspaceState")
	defer b.mu.Unlock()

	if w, ok := b.workspaces.Get(id); ok {
		w.State = state
	}
}

// WorkspaceProps returns a copy of the properties stored for a workspace.
func WorkspaceProps(b *InMemoryBackend, id string) *WorkspaceProperties {
	b.mu.RLock("WorkspaceProps")
	defer b.mu.RUnlock()

	w, ok := b.workspaces.Get(id)
	if !ok || w.Properties == nil {
		return nil
	}

	p := *w.Properties

	return &p
}

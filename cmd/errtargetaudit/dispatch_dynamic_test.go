package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// ifChainDispatchFixture isolates forecast's real blind-spot-3 shape
// (handler.go:131-153): dispatch routes through an `if action == "X"`
// chain rather than a switch, calling a method whose name matches NEITHER
// findHandlersByName's candidates nor its case-insensitive fold fallback
// (resolveop.go) -- so before collectIfChainDispatchEntries, resolveOpRoots
// must return empty for "StopThing".
const ifChainDispatchFixture = `
package fixture

type Handler struct {
	Backend *Backend
}

func (h *Handler) dispatch(action string) error {
	if action == "StopThing" {
		return h.dispatchStopThing()
	}
	return nil
}

func (h *Handler) dispatchStopThing() error {
	return h.Backend.UpdateStatus()
}

type Backend struct{}

func (b *Backend) UpdateStatus() error { return nil }
`

func TestResolveOpRoots_IfChainDispatch(t *testing.T) {
	t.Parallel()

	idx := parseSrc(t, ifChainDispatchFixture)

	roots := resolveOpRoots("StopThing", idx)
	require.NotEmpty(t, roots, "StopThing must resolve via the if/else-if action chain (dispatch.go:131-153 shape)")
	require.Equal(t, "(Handler).dispatchStopThing", roots[0].Name)
}

// indexAssignDispatchFixture isolates comprehend's real blind-spot-2 shape
// (handler.go:242,261-268): a func-typed dispatch map populated partly by
// direct index-assignment after its initial composite literal
// ("TagResource") and partly through a `for prefix := range familySpecs()`
// loop concatenating the loop variable ("StartWidgetJob") -- both use
// method names that match no findHandlersByName candidate.
const indexAssignDispatchFixture = `
package fixture

type Handler struct {
	ops map[string]func() error
}

func familySpecs() map[string]int {
	return map[string]int{"Widget": 1}
}

func buildOps(h *Handler) map[string]func() error {
	ops := map[string]func() error{}
	ops["TagResource"] = h.tagHandler
	for prefix := range familySpecs() {
		ops["Start"+prefix+"Job"] = h.jobStarter
	}
	return ops
}

func (h *Handler) tagHandler() error { return nil }
func (h *Handler) jobStarter() error { return nil }
`

func TestResolveOpRoots_IndexAssignDispatch(t *testing.T) {
	t.Parallel()

	idx := parseSrc(t, indexAssignDispatchFixture)

	roots := resolveOpRoots("TagResource", idx)
	require.NotEmpty(t, roots, "TagResource must resolve via the direct index-assignment (handler.go:261-263 shape)")
	require.Equal(t, "(Handler).tagHandler", roots[0].Name)

	roots = resolveOpRoots("StartWidgetJob", idx)
	require.NotEmpty(
		t, roots,
		"StartWidgetJob must resolve via the range-loop index-assignment (handler.go:265-272 shape)",
	)
	require.Equal(t, "(Handler).jobStarter", roots[0].Name)
}

// sharedExecutorDispatchFixture isolates forecast's real blind-spot-1 shape
// (handler.go:33,51,156-167,782-804): a struct-VALUED dispatch map
// (operationSpec, not a func) populated by a helper (addCRUD) that builds
// its keys by concatenating a literal onto its own STRING PARAMETER, bound
// only at each call site -- and every key routes through the SAME shared
// executor method rather than a per-key handler.
const sharedExecutorDispatchFixture = `
package fixture

type operationSpec struct {
	mode string
}

type Handler struct {
	Backend *Backend
	ops     map[string]operationSpec
}

func buildSpecs() map[string]operationSpec {
	ops := make(map[string]operationSpec)
	addCRUD(ops, "Widget")

	return ops
}

func addCRUD(ops map[string]operationSpec, base string) {
	ops["Create"+base] = operationSpec{mode: "create"}
	ops["Delete"+base] = operationSpec{mode: "delete"}
}

func wireHandler(h *Handler) {
	service.HandleTarget(nil, nil, "", "", nil, h.dispatch, h.handleError)
}

func (h *Handler) dispatch(action string) error {
	spec, ok := h.ops[action]
	if !ok {
		return nil
	}

	return h.execute(action, spec)
}

func (h *Handler) execute(action string, spec operationSpec) error {
	return h.Backend.Do(spec)
}

func (h *Handler) handleError() {}

type Backend struct{}

func (b *Backend) Do(spec operationSpec) error { return nil }
`

func TestResolveOpRoots_SharedExecutorDataMapDispatch(t *testing.T) {
	t.Parallel()

	idx := parseSrc(t, sharedExecutorDispatchFixture)

	for _, op := range []string{"CreateWidget", "DeleteWidget"} {
		roots := resolveOpRoots(op, idx)
		require.NotEmpty(
			t, roots, "%s must resolve via addCRUD's call-site-bound keys and the shared execute() root", op,
		)
		require.Equal(t, "(Handler).execute", roots[0].Name)
	}
}

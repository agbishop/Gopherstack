package main

import "go/ast"

// This file covers gopherstack-sgbw's other two blind spots, both about a
// dispatch map dispatch.go's collectMapLiteralEntries cannot see the FULL
// population of:
//
//   - blind spot 2 (comprehend, handler.go:266-307): the map's VALUE type is
//     func-shaped (so isDispatchMapType already accepts it), but most
//     entries arrive by index-assignment (`ops["X"] = ...`) after the
//     initial composite literal, some inside a `for prefix, spec := range
//     someLiteralMap() { ops["Start"+prefix] = h.startJob(spec) }` loop.
//     collectIndexAssignDispatchEntries below walks both shapes and binds
//     each recovered key straight to its own value expression, exactly like
//     collectMapLiteralEntries does for the literal it already sees.
//
//   - blind spot 1 (forecast, handler.go:33,51,782-804): the map's VALUE
//     type is a plain DATA struct (operationSpec), so no per-key value
//     expression is ever callable -- every key instead flows through ONE
//     shared executor method (`h.execute(action, spec, input)`), and the
//     keys themselves are built by string concatenation inside a helper
//     function (addCRUD) parameterized over call-site literal arguments,
//     not over anything visible at the map's own declaration.
//     collectPopulatorHelperKeys recovers the key STRINGS by evaluating
//     addCRUD's body once per call site, substituting each site's literal
//     arguments; collectSharedExecutorFallback recovers the single ROOT
//     every one of them actually resolves to, and binds it to every
//     recovered key that collectIndexAssignDispatchEntries could not
//     already resolve on its own (a func-shaped map has no need of it).
//
// Both blind spots ultimately fall back to the SAME philosophy as
// dispatch.go's three original collectors: recover a key -> root-expression
// binding wherever the shape allows it, over-inclusively, and leave
// resolveOpRoots's union with the name-convention fallback to absorb
// anything still missed.

// isStringKeyedMapType reports whether t is `map[string]T` for any T --
// broader than isDispatchMapType (dispatch.go), which additionally requires
// T to be func-shaped. A data-typed dispatch table (forecast's
// map[string]operationSpec) needs its KEYS recognized the same way even
// though its VALUES can never resolve to a callable on their own.
func isStringKeyedMapType(t ast.Expr) bool {
	mt, ok := t.(*ast.MapType)
	if !ok {
		return false
	}

	id, ok := mt.Key.(*ast.Ident)

	return ok && id.Name == stringTypeName
}

// collectDynamicDispatchMapEntries orchestrates the index-assignment,
// range-loop, populator-helper and shared-executor collectors below.
// dataKeys accumulates every recovered key whose own value expression is
// not itself callable (a struct/data literal, or a key recovered from a
// helper function's body rather than any expression at all) -- these are
// resolved, if at all, only by collectSharedExecutorFallback at the end.
func collectDynamicDispatchMapEntries(
	files []*ast.File,
	pkgConsts map[string]string,
	funcs map[string]*ast.FuncDecl,
	out map[string]ast.Expr,
) {
	dataKeys := map[string]bool{}

	collectIndexAssignDispatchEntries(files, pkgConsts, funcs, out, dataKeys)
	collectPopulatorHelperKeys(files, pkgConsts, funcs, dataKeys)
	collectSharedExecutorFallback(files, funcs, dataKeys, out)
}

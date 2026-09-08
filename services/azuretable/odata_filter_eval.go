package azuretable

import "github.com/blackbirdworks/gopherstack/pkgs/odatatable"

// EvaluateFilter reports whether entity satisfies the parsed $filter tree
// node. See pkgs/odatatable.EvaluateFilter (this package's re-exported
// engine -- see interfaces.go's package doc comment) for the full semantics.
func EvaluateFilter(node Node, entity EntityInfo) bool {
	return odatatable.EvaluateFilter(node, entity)
}

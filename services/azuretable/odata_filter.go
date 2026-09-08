package azuretable

import "github.com/blackbirdworks/gopherstack/pkgs/odatatable"

// Node is a node in a parsed $filter expression tree. Re-exported from
// pkgs/odatatable (see interfaces.go's package doc comment): the $filter
// lexer/parser/evaluator itself now lives there.
type Node = odatatable.Node

// ParseFilter parses a complete $filter expression string into a Node tree.
// See pkgs/odatatable.ParseFilter for the full grammar and error contract.
func ParseFilter(s string) (Node, error) {
	return odatatable.ParseFilter(s)
}

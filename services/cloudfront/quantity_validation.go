package cloudfront

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
)

// quantityNode is a generic XML element used to walk an arbitrary CloudFront config
// document (DistributionConfig, CachePolicyConfig, StreamingDistributionConfig, ...)
// looking for the pervasive
//
//	<X><Quantity>N</Quantity><Items>...</Items></X>
//
// pattern, without needing a fully-typed Go struct for every nested list in the
// CloudFront schema. AWS validates every one of these pairings and rejects a
// mismatch with InconsistentQuantities; because Go slices carry their own length,
// it is easy for an emulator to accept a caller-supplied Quantity that disagrees
// with the actual Items and silently ignore the mismatch.
type quantityNode struct {
	XMLName xml.Name
	Content string         `xml:",chardata"`
	Nodes   []quantityNode `xml:",any"`
}

// quantityMismatchError describes one Quantity/Items pairing found inconsistent by
// findQuantityMismatch. Its Error() is the shared message text; callers choose which
// sentinel it means by wrapping it themselves (validateQuantities,
// validateFunctionConfigQuantities).
type quantityMismatchError struct {
	element  string
	declared int
	actual   int
}

func (m *quantityMismatchError) Error() string {
	return fmt.Sprintf(
		"the parameter Quantity (%d) for the %s list does not match the number of Items provided (%d)",
		m.declared, m.element, m.actual,
	)
}

// validateQuantities parses rawConfig as generic XML and verifies every
// Quantity/Items pairing found anywhere in the document is internally consistent.
// It returns an error wrapping ErrInconsistentQuantities describing the first
// mismatch found, or nil if the document is consistent (or not parseable as XML --
// malformed XML is the caller's own strict typed-unmarshal's job to report, as
// MalformedXML, not this generic pass's).
func validateQuantities(rawConfig []byte) error {
	m := findQuantityMismatch(rawConfig)
	if m == nil {
		return nil
	}

	return fmt.Errorf("%w: %w", ErrInconsistentQuantities, m)
}

// validateFunctionConfigQuantities is validateQuantities for
// CreateFunction/UpdateFunction/CreateConnectionFunction/UpdateConnectionFunction: their
// own SDK error deserializer never declares InconsistentQuantities, only InvalidArgument,
// even though FunctionConfig.KeyValueStoreAssociations is a real Quantity/Items pair
// (cloudfront@v1.67.4 serializers.go's awsRestxml_serializeDocumentKeyValueStoreAssociations).
func validateFunctionConfigQuantities(rawConfig []byte) error {
	m := findQuantityMismatch(rawConfig)
	if m == nil {
		return nil
	}

	return fmt.Errorf("%w: %w", ErrValidation, m)
}

// findQuantityMismatch parses rawConfig as generic XML and returns the first
// Quantity/Items mismatch found anywhere in the document, or nil if the document is
// consistent (or not parseable as XML -- malformed XML is the caller's own strict
// typed-unmarshal's job to report, as MalformedXML, not this generic pass's).
func findQuantityMismatch(rawConfig []byte) *quantityMismatchError {
	if len(rawConfig) == 0 {
		return nil
	}

	var root quantityNode
	//nolint:musttag,nilerr // deliberately generic: any element shape is accepted, and
	// malformed/unrecognized XML here is intentionally not an error for this pass --
	// the caller's own strict typed-unmarshal already reports MalformedXML.
	if err := xml.Unmarshal(rawConfig, &root); err != nil {
		return nil
	}

	return checkQuantityNode(&root)
}

// checkQuantityNode recursively checks n and its descendants for Quantity/Items
// mismatches, depth-first, returning the first one found.
func checkQuantityNode(n *quantityNode) *quantityMismatchError {
	var (
		quantity    int
		hasQuantity bool
		items       *quantityNode
	)

	for i := range n.Nodes {
		child := &n.Nodes[i]

		switch child.XMLName.Local {
		case "Quantity":
			if v, err := strconv.Atoi(strings.TrimSpace(child.Content)); err == nil {
				quantity, hasQuantity = v, true
			}
		case "Items":
			items = child
		}
	}

	if hasQuantity {
		actual := 0
		if items != nil {
			actual = len(items.Nodes)
		}

		if quantity != actual {
			return &quantityMismatchError{element: n.XMLName.Local, declared: quantity, actual: actual}
		}
	}

	for i := range n.Nodes {
		if m := checkQuantityNode(&n.Nodes[i]); m != nil {
			return m
		}
	}

	return nil
}

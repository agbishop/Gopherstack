package odatatable

import (
	"bytes"
	"strings"
	"time"
)

// EvaluateFilter reports whether entity satisfies the parsed $filter tree
// node. A comparison against a property missing from entity always
// evaluates to false (never an error, never a panic) -- real Table Storage
// semantics: an absent property simply never matches. A type mismatch
// between the two operands of a comparison (e.g. a string compared against
// a number) likewise evaluates to false rather than erroring.
func EvaluateFilter(node Node, entity EntityInfo) bool {
	switch n := node.(type) {
	case *andNode:
		return EvaluateFilter(n.left, entity) && EvaluateFilter(n.right, entity)
	case *orNode:
		return EvaluateFilter(n.left, entity) || EvaluateFilter(n.right, entity)
	case *notNode:
		return !EvaluateFilter(n.expr, entity)
	case *cmpNode:
		return evalComparison(n, entity)
	default:
		return false
	}
}

func evalComparison(n *cmpNode, entity EntityInfo) bool {
	left, leftOK := resolveOperand(n.left, entity)
	right, rightOK := resolveOperand(n.right, entity)

	if !leftOK || !rightOK {
		return false
	}

	return compareOperands(left, right, n.op)
}

// resolveOperand resolves op against entity: a literal resolves to itself;
// an identifier resolves against the three system properties or, failing
// that, entity.Properties. ok is false when an identifier names a property
// entity does not have.
func resolveOperand(op operand, entity EntityInfo) (operand, bool) {
	if !op.isIdent {
		return op, true
	}

	switch op.ident {
	case PartitionKeyProperty:
		return operand{litType: tString, strVal: entity.PartitionKey}, true
	case RowKeyProperty:
		return operand{litType: tString, strVal: entity.RowKey}, true
	case TimestampProperty:
		return operand{litType: tDateTime, timeVal: entity.Timestamp}, true
	default:
		prop, ok := entity.Properties[op.ident]
		if !ok {
			return operand{}, false
		}

		return propertyOperand(prop), true
	}
}

func propertyOperand(p EntityProperty) operand {
	switch p.Type {
	case EdmString:
		s, _ := p.Value.(string)

		return operand{litType: tString, strVal: s}
	case EdmInt32:
		n, _ := p.Value.(int32)

		return operand{litType: tInt, isInt32: true, intVal: int64(n)}
	case EdmInt64:
		n, _ := p.Value.(int64)

		return operand{litType: tInt64, intVal: n}
	case EdmDouble:
		f, _ := p.Value.(float64)

		return operand{litType: tFloat, floatVal: f}
	case EdmBoolean:
		b, _ := p.Value.(bool)

		return operand{litType: tBool, boolVal: b}
	case EdmDateTime:
		t, _ := p.Value.(time.Time)

		return operand{litType: tDateTime, timeVal: t}
	case EdmGUID:
		s, _ := p.Value.(string)

		return operand{litType: tGUID, strVal: s}
	case EdmBinary:
		b, _ := p.Value.([]byte)

		return operand{litType: tBinary, bytesVal: b}
	default:
		return operand{}
	}
}

// compareOperands applies op to left/right if they fall in the same
// comparable category (numeric, string, datetime, bool, guid, binary);
// otherwise returns false. Numeric comparison spans Int32/Int64/Double,
// matching real Table Storage's type-coercing numeric comparisons -- but see
// compareNumeric for why that coercion is NOT a blanket float64 conversion.
//
//nolint:cyclop // per-EDM-category dispatch; splitting would obscure it
func compareOperands(left, right operand, op tokenType) bool {
	switch {
	case isNumericOperand(left) && isNumericOperand(right):
		return applyCompare(compareNumeric(left, right), op)
	case left.litType == tString && right.litType == tString:
		return applyCompare(strings.Compare(left.strVal, right.strVal), op)
	case left.litType == tDateTime && right.litType == tDateTime:
		return applyCompare(cmpTime(left.timeVal, right.timeVal), op)
	case left.litType == tBool && right.litType == tBool:
		if op != tEq && op != tNe {
			return false
		}

		return applyCompare(cmpBool(left.boolVal, right.boolVal), op)
	case left.litType == tGUID && right.litType == tGUID:
		return applyCompare(strings.Compare(left.strVal, right.strVal), op)
	case left.litType == tBinary && right.litType == tBinary:
		return applyCompare(bytes.Compare(left.bytesVal, right.bytesVal), op)
	default:
		return false
	}
}

func isNumericOperand(o operand) bool {
	return o.litType == tInt || o.litType == tInt64 || o.litType == tFloat
}

// compareNumeric compares two numeric operands. When BOTH are integer-typed
// (Int32 or Int64 -- never Double), it compares their int64 values directly
// rather than converting through float64: float64 has only a 53-bit
// mantissa, so a blanket float64(intVal) conversion silently rounds any
// Int64 magnitude beyond 2^53, which would make e.g.
// "9007199254740993L eq 9007199254740992L" evaluate true. A comparison
// involving a Double operand still goes through float64, since Double
// itself is already an inexact 64-bit float and there is no wider common
// type to compare it against exactly.
func compareNumeric(left, right operand) int {
	if left.litType != tFloat && right.litType != tFloat {
		return cmpInt64(left.intVal, right.intVal)
	}

	return cmpFloat(numericValue(left), numericValue(right))
}

func numericValue(o operand) float64 {
	switch o.litType {
	case tInt, tInt64:
		return float64(o.intVal)
	case tFloat:
		return o.floatVal
	default:
		return 0
	}
}

func cmpInt64(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func applyCompare(cmp int, op tokenType) bool {
	switch op {
	case tEq:
		return cmp == 0
	case tNe:
		return cmp != 0
	case tLt:
		return cmp < 0
	case tLe:
		return cmp <= 0
	case tGt:
		return cmp > 0
	case tGe:
		return cmp >= 0
	default:
		return false
	}
}

func cmpFloat(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func cmpTime(a, b time.Time) int {
	switch {
	case a.Before(b):
		return -1
	case a.After(b):
		return 1
	default:
		return 0
	}
}

func cmpBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case !a && b:
		return -1
	default:
		return 1
	}
}

package waf

import "fmt"

// applyEntryUpdate inserts or deletes a single entry from a WAF Classic
// Update op's tuple/descriptor list. Real AWS rejects a redundant insert or
// a delete of an absent entry with WAFInvalidOperationException
// (types/errors.go: "The operation failed because there was nothing to
// do... You tried to add a ByteMatchTuple to a ByteMatchSet, but the
// ByteMatchTuple already exists... You tried to remove an IP address from
// an IPSet, but the IP address isn't in the specified IPSet").
func applyEntryUpdate[T any](items []T, action string, entry T, equal func(T, T) bool) ([]T, error) {
	exists := false
	for _, it := range items {
		if equal(it, entry) {
			exists = true

			break
		}
	}

	switch {
	case action == updateInsert && exists:
		return items, fmt.Errorf("%w: entry already exists in this set", ErrInvalidOperation)
	case action == updateInsert:
		return append(items, entry), nil
	case action == updateDelete && !exists:
		return items, fmt.Errorf("%w: entry isn't in this set", ErrInvalidOperation)
	case action == updateDelete:
		filtered := items[:0]
		for _, it := range items {
			if !equal(it, entry) {
				filtered = append(filtered, it)
			}
		}

		return filtered, nil
	}

	return items, nil
}

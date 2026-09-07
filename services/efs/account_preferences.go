package efs

import "fmt"

// DescribeAccountPreferences returns the current account preferences.
func (b *InMemoryBackend) DescribeAccountPreferences() AccountPreferences {
	b.mu.RLock("DescribeAccountPreferences")
	defer b.mu.RUnlock()

	return b.accountPreferences
}

// PutAccountPreferences sets the account-level resource ID preference.
func (b *InMemoryBackend) PutAccountPreferences(resourceIDType string) (AccountPreferences, error) {
	b.mu.Lock("PutAccountPreferences")
	defer b.mu.Unlock()

	if resourceIDType != resourceIDTypeLong && resourceIDType != resourceIDTypeShort {
		// PutAccountPreferences declares BadRequest/InternalServerError only, never
		// ValidationException (efs@v1.44.4 deserializers.go).
		return AccountPreferences{}, fmt.Errorf(
			"%w: invalid ResourceIdType %q, must be LONG_ID or SHORT_ID",
			ErrBadRequest,
			resourceIDType,
		)
	}

	b.accountPreferences.ResourceIDType = resourceIDType

	return b.accountPreferences, nil
}

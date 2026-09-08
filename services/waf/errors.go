package waf

import "github.com/blackbirdworks/gopherstack/pkgs/awserr"

const (
	errResourceNotFound = "WAFNonexistentItemException"
	errStaleData        = "WAFStaleDataException"
	errInvalidParameter = "WAFInvalidParameterException"
	errReferencedItem   = "WAFReferencedItemException"
	errNonEmptyEntity   = "WAFNonEmptyEntityException"
	errInvalidOperation = "WAFInvalidOperationException"
)

var (
	// ErrNotFound is returned when a resource does not exist.
	ErrNotFound = awserr.New(errResourceNotFound, awserr.ErrNotFound)
	// ErrStaleToken is returned when the change token is stale.
	ErrStaleToken = awserr.New(errStaleData, awserr.ErrConflict)
	// ErrInvalidParameter is returned on invalid input.
	ErrInvalidParameter = awserr.New(errInvalidParameter, awserr.ErrInvalidParameter)
	// ErrReferencedItem is returned when a resource is still referenced.
	ErrReferencedItem = awserr.New(errReferencedItem, awserr.ErrConflict)
	// ErrNonEmptyEntity is returned when a resource still contains child
	// entities (e.g. a WebACL that still has Rules, a Rule that still has
	// Predicates, a ByteMatchSet that still has ByteMatchTuples).
	ErrNonEmptyEntity = awserr.New(errNonEmptyEntity, awserr.ErrConflict)
	// ErrInvalidOperation is returned when an Update request has nothing to
	// do: inserting an item that is already present, or deleting one that
	// isn't (types/errors.go WAFInvalidOperationException in aws-sdk-go-v2
	// service/waf@v1.33.4).
	ErrInvalidOperation = awserr.New(errInvalidOperation, awserr.ErrInvalidParameter)
)

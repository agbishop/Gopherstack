package mediastoredata

import (
	"fmt"
	"strings"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
)

// maxPathLength is the maximum allowed byte-length of an object path,
// matching the AWS MediaStore Data limit.
const maxPathLength = 900

// Object size limits per aws-sdk-go-v2/service/mediastoredata's PutObject doc
// comment (api_op_PutObject.go:13-14): object sizes are limited to 25 MB for
// standard upload availability and 10 MB for streaming upload availability.
const (
	maxObjectSizeStandard  = 25 * 1024 * 1024
	maxObjectSizeStreaming = 10 * 1024 * 1024
)

var (
	// ErrNotFound is returned when a requested object does not exist.
	ErrNotFound = awserr.New("ObjectNotFoundException", awserr.ErrNotFound)

	// ErrInvalidPath is returned when a path fails validation.
	//
	// The wire __type for this is "ValidationException", NOT a fabricated
	// "InvalidPathException" -- the real mediastoredata SDK error model
	// (aws-sdk-go-v2/service/mediastoredata/types/errors.go) defines exactly
	// four exceptions (ContainerNotFoundException, InternalServerError,
	// ObjectNotFoundException, RequestedRangeNotSatisfiableException) and none
	// of them cover client-side parameter validation. "ValidationException" is
	// the AWS-wide/gopherstack-wide convention for this situation (see e.g.
	// services/mediastore/handler.go, and this handler's own MaxResults bound
	// check), which is a real error name actually used across AWS APIs, unlike
	// a wholly invented service-specific exception name.
	ErrInvalidPath = awserr.New("ValidationException", awserr.ErrInvalidParameter)

	// ErrInvalidStorageClass is returned when an unknown storage class is
	// supplied. See [ErrInvalidPath]'s doc comment for why the wire __type is
	// "ValidationException" rather than a fabricated
	// "InvalidStorageClassException".
	ErrInvalidStorageClass = awserr.New("ValidationException", awserr.ErrInvalidParameter)

	// ErrInvalidUploadAvailability is returned when x-amz-upload-availability
	// is set to a value other than the two real enum members (STANDARD,
	// STREAMING -- aws-sdk-go-v2/service/mediastoredata/types.
	// UploadAvailability's only Values()). See [ErrInvalidPath]'s doc comment
	// for why the wire __type is "ValidationException".
	ErrInvalidUploadAvailability = awserr.New("ValidationException", awserr.ErrInvalidParameter)

	// ErrObjectTooLarge is returned when a PutObject body exceeds the size
	// limit for its upload availability. See [maxObjectSizeStandard]/
	// [maxObjectSizeStreaming]. See [ErrInvalidPath]'s doc comment for why the
	// wire __type is "ValidationException" -- this limit has no exception of
	// its own in mediastoredata's 4-exception model (types/errors.go).
	ErrObjectTooLarge = awserr.New("ValidationException", awserr.ErrInvalidParameter)
)

// isValidStorageClass reports whether sc is a known MediaStore Data storage
// class. Real AWS Elemental MediaStore Data has exactly one StorageClass
// value ("TEMPORAL" -- see aws-sdk-go-v2/service/mediastoredata/types.
// StorageClass, whose only enum member is StorageClassTemporal). "STANDARD"
// is NOT a storage class: it is a value of the unrelated
// x-amz-upload-availability header (UploadAvailability), and must not be
// accepted here.
func isValidStorageClass(sc string) bool {
	return sc == "TEMPORAL"
}

// isValidUploadAvailability reports whether ua is a known MediaStore Data
// UploadAvailability value ("STANDARD" or "STREAMING" -- see
// aws-sdk-go-v2/service/mediastoredata/types.UploadAvailability).
func isValidUploadAvailability(ua string) bool {
	return ua == "STANDARD" || ua == "STREAMING"
}

// normalizePath normalises an object path (strips leading slash).
func normalizePath(p string) string {
	return strings.TrimPrefix(p, "/")
}

// ValidatePath checks that path is a legal MediaStore object path.
func ValidatePath(p string) error {
	key := normalizePath(p)
	if key == "" {
		return fmt.Errorf("%w: path cannot be empty", ErrInvalidPath)
	}
	if len(key) > maxPathLength {
		return fmt.Errorf("%w: path exceeds %d characters", ErrInvalidPath, maxPathLength)
	}
	if strings.Contains(key, "..") {
		return fmt.Errorf("%w: path cannot contain '..'", ErrInvalidPath)
	}
	if strings.ContainsRune(key, 0) {
		return fmt.Errorf("%w: path contains null byte", ErrInvalidPath)
	}

	return nil
}

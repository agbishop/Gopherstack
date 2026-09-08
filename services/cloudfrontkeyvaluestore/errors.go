package cloudfrontkeyvaluestore

import (
	"errors"
	"net/http"

	cloudfrontbackend "github.com/blackbirdworks/gopherstack/services/cloudfront"
)

// Exception type names verified against cloudfrontkeyvaluestore@v1.15.4
// types/errors.go and deserializers.go's per-op
// awsRestjson1_deserializeOpError<Op> switches: AccessDeniedException,
// ConflictException, InternalServerException, ResourceNotFoundException,
// ServiceQuotaExceededException, ValidationException. This emulator has no
// IAM enforcement and no per-store size quota (see PARITY.md), so only the
// four exceptions below -- the ones a real backend/state-driven bug can
// actually produce here -- are modeled. Status codes follow the repo-wide
// convention for these exact exception names (see e.g. services/fis/handler.go,
// services/grafana/handler.go for ServiceQuotaExceededException -> 402).
const (
	exceptionConflict         = "ConflictException"
	exceptionInternalServer   = "InternalServerException"
	exceptionResourceNotFound = "ResourceNotFoundException"
	exceptionValidation       = "ValidationException"
	// errorTypeHeader uses Go's canonical MIME header casing (matches
	// services/apigatewayv2's errTypeHeader) -- http.Header canonicalizes on
	// Set/Get regardless, so the wire bytes are identical either way, but
	// golangci-lint's canonicalheader linter wants the literal to match.
	errorTypeHeader = "X-Amzn-Errortype"
)

// errInvalidMaxResults is returned by parseMaxResults when the MaxResults
// query parameter falls outside the real API's documented bounds.
var errInvalidMaxResults = errors.New("MaxResults must be between 1 and 50")

// errIfMatchRequired is returned when a PutKey/DeleteKey/UpdateKeys request
// carries no If-Match header. IfMatch is a required member on all three
// inputs (validators.go's validateOp{PutKey,DeleteKey,UpdateKeys}Input in
// cloudfrontkeyvaluestore@v1.15.4), so a missing header must be rejected
// rather than treated as "skip the ETag check" -- see requireIfMatch's
// callers in handler.go.
var errIfMatchRequired = errors.New("IfMatch is required")

// classifyError maps a backend error to the (status, exceptionType) pair a
// real cloudfrontkeyvaluestore server would return. The backend calls here
// are the same InMemoryBackend methods services/cloudfront's own (removed)
// data-plane handlers used, so the sentinel errors are cloudfront's.
func classifyError(err error) (int, string) {
	switch {
	case errors.Is(err, cloudfrontbackend.ErrKeyValueStoreNotFound),
		errors.Is(err, cloudfrontbackend.ErrNotFound):
		return http.StatusNotFound, exceptionResourceNotFound
	case errors.Is(err, cloudfrontbackend.ErrPreconditionFailed):
		// The real API models ETag mismatches as ConflictException (409), not
		// HTTP 412 -- the removed services/cloudfront data-plane handlers this
		// package replaces got this wrong (see PARITY.md).
		return http.StatusConflict, exceptionConflict
	case errors.Is(err, cloudfrontbackend.ErrValidation),
		errors.Is(err, errInvalidMaxResults),
		errors.Is(err, errIfMatchRequired):
		return http.StatusBadRequest, exceptionValidation
	default:
		return http.StatusInternalServerError, exceptionInternalServer
	}
}

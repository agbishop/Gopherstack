package main

// genericProtocolCodes are error codes AWS's wire protocols (JSON-RPC,
// Query, REST) recognize at the frontend/gateway layer for every service --
// USUALLY never modeled as a per-service typed exception, but not always:
// gopherstack-udkm found ValidationException alone declared per-op in 57 of
// the pinned SDK's 166 service modules. That is not a bug here, though:
// scan.go's classify already checks gt.codes (this service's OWN resolved
// module's code union) before falling back to this map, so a service whose
// own module DOES declare the code (gt.codes[c.Code]) is never routed
// through this allowlist at all -- consulting it only matters, and is only
// correct, for a service whose own SDK module has no record of the code
// anywhere. gopherstack-q9bs checked that zero-record case for kms
// specifically (kms@v1.55.4 and its full api-2.json model have no
// ValidationException anywhere -- gopherstack-i4q8) and confirmed the entry
// still holds: kms's own GetPublicKey doc (aws-sdk-go models/apis/kms/
// 2014-11-01/docs-2.json) quotes a live wire ValidationException for a
// malformed PublicKey that no operation declares, and deserializeOpError's
// default case (kms@v1.55.4 deserializers.go) preserves an unmodeled wire
// code rather than rejecting it -- exactly the pre-dispatch, model-independent
// fault this allowlist exists for.
//
// gopherstack-oshm reconciled this list with cmd/errtargetaudit's by REMOVING
// InternalServerException from that one, not by adding it here. The entry
// failed the standard q9bs set for it: a search of every vendored
// aws-sdk-go@v1.55.8 service model's docs-2.json for an operation described
// as returning InternalServerException where api-2.json declares no such
// shape found zero hits, while the identical search finds two for
// ValidationException. Meanwhile 51 of the pinned SDK's 166 modules declare
// InternalServerException as a per-op typed exception -- evidence that it is
// a modeled exception, not a pre-dispatch protocol fault. Suppressing it hid
// three real findings, one of them confident (services/forecast and
// services/personalize, neither of whose modules declares any server-fault
// type at all).

var genericProtocolCodes = map[string]bool{ //nolint:gochecknoglobals // read-only lookup table
	"ValidationError":             true,
	"ValidationException":         true,
	"InvalidAction":               true,
	"MissingParameter":            true,
	"MissingRequiredParameter":    true,
	"MissingAuthenticationToken":  true,
	"Throttling":                  true,
	"ThrottlingException":         true,
	"TooManyRequestsException":    true,
	"RequestLimitExceeded":        true,
	"InternalFailure":             true,
	"InternalError":               true,
	"InternalServerError":         true,
	"ServerException":             true,
	"ServiceException":            true,
	"ServiceUnavailable":          true,
	"ServiceUnavailableException": true,
	"AccessDenied":                true,
	"AccessDeniedException":       true,
	"UnauthorizedException":       true,
	"UnrecognizedClientException": true,
	"SignatureDoesNotMatch":       true,
	"InvalidClientTokenId":        true,
	"ExpiredToken":                true,
	"ExpiredTokenException":       true,
	"RequestExpired":              true,
	"IncompleteSignature":         true,
	"InvalidParameterValue":       true,
	"InvalidParameterCombination": true,
	"InvalidQueryParameter":       true,
	"OptInRequired":               true,
	"PendingVerification":         true,
	"AuthFailure":                 true,
	"Blocked":                     true,
	"UnknownOperationException":   true,
	"UnknownOperation":            true,
	"SerializationException":      true,
	"MethodNotAllowedException":   true,
	"MissingAction":               true,
	"NotImplementedException":     true,
}

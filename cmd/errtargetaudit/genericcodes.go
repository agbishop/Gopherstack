package main

// genericProtocolCodes are error codes AWS's wire protocols recognize at the
// frontend/gateway layer for every service, never modeled as a per-operation
// typed exception -- so an operation's own deserializer legitimately
// contains none of them, and flagging their absence there would be a false
// positive by construction. Same list as cmd/errcodeaudit/genericcodes.go
// (reimplemented, not imported -- see this package's doc comment for why);
// see that file's doc comment for the sourcing and live confirmations
// behind each entry. ONE ADDITION beyond that list, made during this tool's
// own validation pass: "InternalServerException" (the "Exception"-suffixed
// sibling of the "InternalServerError" that file already allowlists).
// services/mgn's shared internalServerError() constructor is called from
// ~90 operations for a genuine unexpected-failure fallback; every one
// sampled was confirmed, directly against mgn@v1.48.4's own
// deserializers.go, to legitimately declare no InternalServerException at
// all (e.g. ArchiveApplication's real set is {ConflictException,
// ResourceNotFoundException, ServiceQuotaExceededException,
// UninitializedAccountException} -- no server-fault type whatsoever). That
// is the exact "gateway/runtime fallback, not a per-operation contract"
// reasoning cmd/errcodeaudit's own doc already applies to InternalError/
// InternalServerError/ServerException/ServiceException; before this
// addition it produced 90 false positives in one service alone.
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
	"InternalServerException":     true,
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

// wireFaultTypeDiscriminators are AWS Query/SOAP-style envelope Type field
// values (<Error><Type>Sender</Type></Error>) -- never an operation error
// code itself, structurally: a real AWS exception name is always a
// specific multi-word PascalCase name, never a bare member of this fixed
// four-value fault-type vocabulary. Confirmed as a false positive on
// services/sns (writeError's `Error{Type: "Sender", Code: code, ...}`) and
// services/glacier (writeError's `errorResponse{..., Type: "Client", ...}`)
// during gopherstack-zofv's validation pass: firstCodeLiteral's one-hop
// recursion into either shared helper finds "Sender"/"Client" before ever
// reaching the real (parameter-carried, non-literal) code, since both
// happen to satisfy codeShapeRe same as any real code would. Excluded
// unconditionally, same as genericProtocolCodes -- neither ever appears in
// any module's AllCodes, so this changes no existing finding.
var wireFaultTypeDiscriminators = map[string]bool{ //nolint:gochecknoglobals // read-only lookup table
	"Sender":   true,
	"Receiver": true,
	"Client":   true,
	"Server":   true,
}

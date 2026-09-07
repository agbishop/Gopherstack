package main

// genericProtocolCodes are error codes AWS's wire protocols recognize at the
// frontend/gateway layer for every service -- USUALLY never modeled as a
// per-operation typed exception, but not always: gopherstack-udkm found
// ValidationException alone declared per-op in 57 of the pinned SDK's 166
// service modules (grep -rl '"ValidationException"' .../service/*/
// deserializers.go), and InternalServerException genuinely declared (both a
// types/errors.go type AND 3 deserializeOpError<Op> switch cases) in
// services/mgn's OWN pinned module -- the exact service this list's
// InternalServerException entry was added for (see below), meaning that
// addition's sampling missed the ops that DO declare it. classifyEmission
// (scan.go) therefore consults this map only when the code is absent from
// allCodes (the service's own module-wide AllCodes union) too -- a code
// present in allCodes is real for this service and reported class A
// instead, never waved through unconditionally. That still leaves the
// reverse gap open -- a service whose module has NO record of a
// genericProtocolCodes entry anywhere is excused unconditionally, same as
// one that models it correctly per-op. gopherstack-q9bs checked that gap for
// kms and ValidationException (kms@v1.55.4 has zero occurrences, per
// gopherstack-i4q8) and confirmed the entry, not kms's 32 landmine
// comments: see cmd/errcodeaudit/genericcodes.go's doc comment for the
// evidence. Same list as
// cmd/errcodeaudit/genericcodes.go (reimplemented, not imported -- see this
// package's doc comment for why); see that file's doc comment for the
// sourcing and live confirmations behind each entry. ONE ADDITION beyond
// that list, made during this tool's own validation pass:
// "InternalServerException" (the "Exception"-suffixed sibling of the
// "InternalServerError" that file already allowlists). services/mgn's
// shared internalServerError() constructor is called from ~90 operations
// for a genuine unexpected-failure fallback; the ops sampled during that
// pass (e.g. ArchiveApplication's real declared set is {ConflictException,
// ResourceNotFoundException, ServiceQuotaExceededException,
// UninitializedAccountException} -- no server-fault type) declare no
// InternalServerException of their OWN, which is still correct -- they are
// now class A findings (real in mgn's module, wrong operation), not
// generic, since the module-conditional check above applies.
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

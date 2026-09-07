package main

// genericProtocolCodes are error codes AWS's wire protocols recognize at the
// frontend/gateway layer for every service -- USUALLY never modeled as a
// per-operation typed exception, but not always: gopherstack-udkm found
// ValidationException declared per-op in 57 of the pinned SDK's 166 service
// modules. classifyEmission therefore consults this map only when the code
// is absent from the service's own allCodes, so a service whose module DOES
// declare the code is never routed through the allowlist at all.
//
// Same list as cmd/errcodeaudit/genericcodes.go (reimplemented, not imported
// -- see this package's doc comment for why); that file carries the sourcing
// and live confirmations behind each entry.
//
// gopherstack-oshm reconciled the two by REMOVING InternalServerException
// from this list, not by adding it to errcodeaudit's. It failed the standard
// gopherstack-q9bs set for ValidationException: no service's docs-2.json
// describes returning it where api-2.json declares no such shape (zero hits
// across every vendored aws-sdk-go@v1.55.8 model; the same search finds two
// for ValidationException), while 51 modules declare it as a per-op typed
// exception -- evidence of a modeled exception, not a pre-dispatch fault.
// mgn, the service the entry was added for, declares it in its own module,
// so those emissions are class A findings rather than generic ones.
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

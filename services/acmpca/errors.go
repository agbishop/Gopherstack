package acmpca

import "errors"

var (
	// ErrCANotFound is returned when a Certificate Authority is not found.
	ErrCANotFound = errors.New("ResourceNotFoundException")
	// ErrCertNotFound is returned when an issued certificate is not found.
	ErrCertNotFound = errors.New("ResourceNotFoundException")
	// ErrInvalidArgs is returned when an operation argument fails validation.
	// acm-pca's own deserializeOpError models InvalidArgsException, not the
	// fabricated InvalidParameterException gopherstack previously emitted
	// (gopherstack-r3pr): see aws-sdk-go-v2/service/acmpca deserializers.go,
	// e.g. awsAwsjson11_deserializeOpErrorCreateCertificateAuthority.
	ErrInvalidArgs = errors.New("InvalidArgsException")
	// ErrInvalidArn is returned when a CA/certificate/resource ARN fails
	// validation or lookup, matching InvalidArnException (modeled by nearly
	// every acm-pca operation's deserializeOpError).
	ErrInvalidArn = errors.New("InvalidArnException")
	// ErrInvalidRequest is returned when the request action cannot be
	// performed or is prohibited, matching InvalidRequestException
	// (RevokeCertificate, ImportCertificateAuthorityCertificate).
	ErrInvalidRequest = errors.New("InvalidRequestException")
	// ErrInvalidPolicy is returned when a resource policy is invalid or
	// missing a required statement, matching InvalidPolicyException
	// (PutPolicy).
	ErrInvalidPolicy = errors.New("InvalidPolicyException")
	// ErrMalformedCertificate is returned when an imported certificate fails
	// to decode/parse, matching MalformedCertificateException
	// (ImportCertificateAuthorityCertificate).
	ErrMalformedCertificate = errors.New("MalformedCertificateException")
	// ErrMalformedCSR is returned when a certificate signing request fails
	// to decode/parse, matching MalformedCSRException (IssueCertificate).
	ErrMalformedCSR = errors.New("MalformedCSRException")
	// ErrInvalidState is returned when the CA is in an invalid state for the operation.
	ErrInvalidState = errors.New("InvalidStateException")
	// ErrPermissionNotFound is returned when a CA permission is not found.
	ErrPermissionNotFound = errors.New("ResourceNotFoundException")
	// ErrPermissionAlreadyExists is returned when a permission for the same
	// principal/source-account pair already exists on the CA.
	ErrPermissionAlreadyExists = errors.New("PermissionAlreadyExistsException")
	// ErrPolicyNotFound is returned when a CA policy is not found.
	ErrPolicyNotFound = errors.New("ResourceNotFoundException")
	// ErrAuditReportNotFound is returned when a CA audit report is not found.
	ErrAuditReportNotFound = errors.New("ResourceNotFoundException")
	// ErrTooManyTags is returned when tagging a CA would exceed the 50-tag limit.
	ErrTooManyTags = errors.New("TooManyTagsException")
	// ErrRequestAlreadyProcessed is returned when RevokeCertificate is called
	// on a certificate that is already revoked, matching
	// RequestAlreadyProcessedException ("Your request has already been
	// completed") -- modeled only by RevokeCertificate's own deserializeOpError
	// (acmpca@v1.50.0 deserializers.go), not by any other operation in this
	// service.
	ErrRequestAlreadyProcessed = errors.New("RequestAlreadyProcessedException")

	errCAPrivKeyNil    = errors.New("CA private key is nil")
	errDecodeCSRPEM    = errors.New("failed to decode CSR PEM")
	errDecodeCACertPEM = errors.New("failed to decode CA certificate PEM")
)

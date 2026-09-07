package acm

import "errors"

var (
	ErrCertNotFound      = errors.New("ResourceNotFoundException")
	ErrInvalidParameter  = errors.New("ValidationException")
	ErrNotEligible       = errors.New("RequestInProgressException")
	ErrRequestInProgress = errors.New("RequestInProgressException")
	ErrAlreadyRevoked    = errors.New("InvalidStateException")
	ErrInvalidState      = errors.New("InvalidStateException")
	ErrResourceInUse     = errors.New("ResourceInUseException")
	ErrConflict          = errors.New("ConflictException")
	// ErrInvalidArn is returned when a CertificateArn does not match the
	// expected ACM ARN shape (arn:<partition>:acm:<region>:<account>:certificate/<id>).
	// Real AWS returns InvalidArnException for malformed ARNs, distinct from
	// ResourceNotFoundException (well-formed ARN, no such resource).
	ErrInvalidArn = errors.New("InvalidArnException")
	// ErrLimitExceeded is returned when an ACM account/resource quota (e.g. the
	// per-certificate domain-name count) is exceeded.
	ErrLimitExceeded = errors.New("LimitExceededException")
	// ErrTooManyTags is returned when a tagging operation would exceed the
	// maximum of 50 tags per certificate.
	ErrTooManyTags = errors.New("TooManyTagsException")
	// ErrInvalidTag is returned when a tag key or value fails AWS tag
	// constraints (e.g. the reserved "aws:" prefix). Only the legacy
	// certificate-tag ops (AddTagsToCertificate, ImportCertificate,
	// RequestCertificate) declare it; the ACME resource families and
	// TagResource declare ValidationException instead -- see ErrInvalidParameter.
	ErrInvalidTag = errors.New("InvalidTagException")
	// ErrServiceQuotaExceeded is TagResource's and the ACME resource
	// families' declared "too many tags" code -- unlike the legacy
	// certificate-tag ops, which declare TooManyTagsException instead
	// (ErrTooManyTags).
	ErrServiceQuotaExceeded = errors.New("ServiceQuotaExceededException")
	// ErrInvalidDomainValidationOptions is returned when the
	// DomainValidationOptions input to RequestCertificate references a domain
	// not in the request, or specifies a ValidationDomain that is not the
	// same as or a superdomain of its DomainName.
	ErrInvalidDomainValidationOptions = errors.New("InvalidDomainValidationOptionsException")
	errInvalidPEM                     = errors.New("failed to decode PEM block")
	// ErrAcmeResourceNotFound is returned when an ACME endpoint, external
	// account binding, ACME account, or domain validation referenced by ARN
	// (or, for accounts, by AccountUrl) does not exist. It maps to the same
	// ResourceNotFoundException code as ErrCertNotFound but is kept as a
	// distinct identity so acme_*.go call sites read clearly and do not imply
	// a certificate was involved.
	ErrAcmeResourceNotFound = errors.New("ResourceNotFoundException")
	// ErrInvalidArgs is ListCertificates' own invalid-input error.
	// ListCertificates' deserializer recognizes exactly two errors --
	// InvalidArgsException and ValidationException (deserializers.go:2735-2739,
	// aws-sdk-go-v2/service/acm@v1.43.4) -- unlike every other op in this
	// package, which uses ValidationException/InvalidParameterException alone.
	ErrInvalidArgs = errors.New("InvalidArgsException")
)

var errWeakKey = errors.New("RSA_1024 is not supported due to weak security")

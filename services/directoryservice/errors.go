package directoryservice

import (
	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
)

const (
	errEntityNotExistsException     = "EntityDoesNotExistException"
	errEntityAlreadyExistsException = "EntityAlreadyExistsException"
	errInvalidParameterException    = "InvalidParameterException"

	defaultSimpleADLimit    int32 = 10
	defaultMicrosoftADLimit int32 = 20
	defaultSnapshotLimit    int32 = 5
)

var (
	// ErrDirectoryNotFound is returned when a directory does not exist.
	ErrDirectoryNotFound = awserr.New(errEntityNotExistsException, awserr.ErrNotFound)
	// ErrSnapshotNotFound is returned when a snapshot does not exist.
	ErrSnapshotNotFound = awserr.New(errEntityNotExistsException, awserr.ErrNotFound)
	// ErrAliasAlreadyExists is returned when the alias is already taken.
	ErrAliasAlreadyExists = awserr.New(errEntityAlreadyExistsException, awserr.ErrAlreadyExists)
	// ErrInvalidParameter is returned on invalid input.
	ErrInvalidParameter = awserr.New(errInvalidParameterException, awserr.ErrInvalidParameter)
	// ErrDirectoryLimitExceeded is returned when the directory limit for the region is reached.
	ErrDirectoryLimitExceeded = awserr.New("DirectoryLimitExceededException", awserr.ErrConflict)
	// ErrSnapshotLimitExceeded is returned when the manual snapshot limit for a directory is reached.
	ErrSnapshotLimitExceeded = awserr.New("SnapshotLimitExceededException", awserr.ErrConflict)
	// ErrUnsupportedOperation is returned when an operation is not supported by the directory type.
	ErrUnsupportedOperation = awserr.New("UnsupportedOperationException", awserr.ErrConflict)

	// ErrTrustNotFound is returned when a trust does not exist.
	ErrTrustNotFound = awserr.New(errEntityNotExistsException, awserr.ErrNotFound)
	// ErrConditionalForwarderNotFound is returned when a conditional forwarder does not exist.
	ErrConditionalForwarderNotFound = awserr.New(errEntityNotExistsException, awserr.ErrNotFound)
	// ErrSchemaExtensionNotFound is returned when a schema extension does not exist.
	ErrSchemaExtensionNotFound = awserr.New(errEntityNotExistsException, awserr.ErrNotFound)
	// ErrSharedDirectoryNotFound is returned when a shared directory does not exist.
	ErrSharedDirectoryNotFound = awserr.New(errEntityNotExistsException, awserr.ErrNotFound)
	// ErrAssessmentNotFound is returned when an AD assessment does not exist.
	ErrAssessmentNotFound = awserr.New(errEntityNotExistsException, awserr.ErrNotFound)
	// ErrInvalidCertificate is returned when CertificateData is not a parseable PEM certificate.
	ErrInvalidCertificate = awserr.New("InvalidCertificateException", awserr.ErrInvalidParameter)

	// ErrDirectoryNotFoundDDNE is returned when a directory does not exist,
	// for operations whose own deserializeOpError switch types
	// DirectoryDoesNotExistException rather than the generic
	// EntityDoesNotExistException -- verified per-op against
	// aws-sdk-go-v2/service/directoryservice@v1.41.4 deserializers.go.
	// EntityDoesNotExistException is unmodeled on these ops, so returning it
	// there makes errors.As into the real SDK's typed exception fail.
	ErrDirectoryNotFoundDDNE = awserr.New("DirectoryDoesNotExistException", awserr.ErrNotFound)
	// ErrDirectoryAlreadyInRegion is returned by AddRegion when the
	// directory is already replicated into the requested Region.
	ErrDirectoryAlreadyInRegion = awserr.New("DirectoryAlreadyInRegionException", awserr.ErrAlreadyExists)
	// ErrCertificateDoesNotExist is returned when a certificate does not
	// exist, for DeregisterCertificate/DescribeCertificate specifically --
	// both type CertificateDoesNotExistException, not the generic
	// EntityDoesNotExistException.
	ErrCertificateDoesNotExist = awserr.New("CertificateDoesNotExistException", awserr.ErrNotFound)

	// ErrRadiusAlreadyEnabled is returned by EnableRadius when RADIUS is
	// already enabled for the directory -- EnableRadius's own deserializer
	// models EntityAlreadyExistsException (directoryservice@v1.41.4
	// deserializers.go), unlike DisableRadius/UpdateRadius which don't.
	ErrRadiusAlreadyEnabled = awserr.New(errEntityAlreadyExistsException, awserr.ErrAlreadyExists)

	// ErrInvalidLDAPSStatus is returned by EnableLDAPS/DisableLDAPS when the
	// requested transition is redundant (enabling already-enabled LDAPS, or
	// disabling LDAPS that isn't enabled) -- both ops model
	// InvalidLDAPSStatusException (directoryservice@v1.41.4 types/errors.go:708).
	ErrInvalidLDAPSStatus = awserr.New("InvalidLDAPSStatusException", awserr.ErrConflict)

	// ErrInvalidClientAuthStatus is returned by EnableClientAuthentication/
	// DisableClientAuthentication when the requested transition is redundant
	// -- both ops model InvalidClientAuthStatusException, whose doc comment
	// reads "Client authentication is already enabled."
	// (directoryservice@v1.41.4 types/errors.go:678-679).
	ErrInvalidClientAuthStatus = awserr.New("InvalidClientAuthStatusException", awserr.ErrConflict)

	// ErrSnapshotUnsupportedForADConnector is returned by CreateSnapshot for
	// an AD Connector directory -- CreateSnapshot's doc comment states "You
	// cannot take snapshots of AD Connector directories."
	// (directoryservice@v1.41.4 api_op_CreateSnapshot.go). CreateSnapshot's
	// own deserializer models no dedicated exception for this, so it maps to
	// the generic ClientException, the same as every other op's catch-all
	// client error.
	ErrSnapshotUnsupportedForADConnector = awserr.New("ClientException", awserr.ErrConflict)
)

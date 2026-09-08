package rekognition

import (
	"errors"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
)

const (
	errResourceNotFound      = "ResourceNotFoundException"
	errConflictException     = "ConflictException"
	errResourceInUse         = "ResourceInUseException"
	errValidation            = "ValidationException"
	errResourceAlreadyExists = "ResourceAlreadyExistsException"
)

var (
	// ErrNameInUse is a sentinel for Create* operations whose Name/Arn is
	// already taken by a resource type that the real AWS API reports as
	// ResourceInUseException -- stream processors, projects, and project
	// versions -- as opposed to ResourceAlreadyExistsException (collections,
	// datasets) or ConflictException (users). Verified against
	// aws-sdk-go-v2/service/rekognition's per-operation deserializers.go
	// error switches (each generated Create* op only recognizes a specific
	// exception type; anything else deserializes as an untyped
	// smithy.GenericAPIError, breaking SDK-side `errors.As` typed matching).
	//
	// Also reused (via the ErrProjectHasVersions/ErrProjectVersionInUse/
	// ErrDatasetInUse sentinels below) for Delete* preconditions that the
	// same operations' deserializers.go error switches model as
	// ResourceInUseException.
	ErrNameInUse = errors.New("resource name already in use")
	// ErrUserConflict is returned when CreateUser is called with a UserId
	// that already exists; AWS reports this as ConflictException (not
	// ResourceAlreadyExistsException).
	ErrUserConflict = errors.New("user already exists")

	// ErrCollectionNotFound is returned when a collection does not exist.
	ErrCollectionNotFound = awserr.New(errResourceNotFound, awserr.ErrNotFound)
	// ErrCollectionAlreadyExists is returned when a collection already exists.
	ErrCollectionAlreadyExists = awserr.New(errConflictException, awserr.ErrAlreadyExists)
	// ErrStreamProcessorNotFound is returned when a stream processor does not exist.
	ErrStreamProcessorNotFound = awserr.New(errResourceNotFound, awserr.ErrNotFound)
	// ErrStreamProcessorAlreadyExists is returned when a stream processor already
	// exists. Maps to ResourceInUseException, not ResourceAlreadyExistsException.
	ErrStreamProcessorAlreadyExists = awserr.New(errResourceInUse, ErrNameInUse)
	// ErrFaceNotFound is returned when a face does not exist in a collection.
	ErrFaceNotFound = awserr.New(errResourceNotFound, awserr.ErrNotFound)
	// ErrValidation is returned on invalid input.
	ErrValidation = awserr.New(errValidation, awserr.ErrInvalidParameter)
	// ErrUnknownOperation is returned when the requested operation is not implemented.
	ErrUnknownOperation = errors.New("unknown operation")

	// ErrProjectNotFound is returned when a project does not exist.
	ErrProjectNotFound = awserr.New(errResourceNotFound, awserr.ErrNotFound)
	// ErrProjectAlreadyExists is returned when a project name is already in
	// use. Maps to ResourceInUseException (not ResourceAlreadyExistsException
	// -- see ErrNameInUse).
	ErrProjectAlreadyExists = awserr.New(errResourceInUse, ErrNameInUse)
	// ErrProjectVersionNotFound is returned when a project version does not exist.
	ErrProjectVersionNotFound = awserr.New(errResourceNotFound, awserr.ErrNotFound)
	// ErrProjectVersionAlreadyExists is returned when a project version name
	// is already in use. Maps to ResourceInUseException (see ErrNameInUse).
	ErrProjectVersionAlreadyExists = awserr.New(errResourceInUse, ErrNameInUse)
	// ErrDatasetNotFound is returned when a dataset does not exist.
	ErrDatasetNotFound = awserr.New(errResourceNotFound, awserr.ErrNotFound)
	// ErrDatasetAlreadyExists is returned when CreateDataset is called for a
	// project that already has a dataset of the requested DatasetType.
	// Maps to ResourceAlreadyExistsException, same exception type as
	// CreateCollection (verified against aws-sdk-go-v2/service/rekognition's
	// CreateDataset error deserializer switch -- see ErrNameInUse's doc
	// comment for why this varies per Create* op).
	ErrDatasetAlreadyExists = awserr.New(errResourceAlreadyExists, awserr.ErrAlreadyExists)
	// ErrUserNotFound is returned when a user does not exist.
	ErrUserNotFound = awserr.New(errResourceNotFound, awserr.ErrNotFound)
	// ErrUserAlreadyExists is returned when a UserId is already registered
	// in the collection. Maps to ConflictException (not
	// ResourceAlreadyExistsException -- see ErrUserConflict).
	ErrUserAlreadyExists = awserr.New(errConflictException, ErrUserConflict)
	// ErrLivenessSessionNotFound is returned when a liveness session does not exist.
	ErrLivenessSessionNotFound = awserr.New(errResourceNotFound, awserr.ErrNotFound)
	// ErrAsyncJobNotFound is returned when an async job does not exist.
	ErrAsyncJobNotFound = awserr.New(errResourceNotFound, awserr.ErrNotFound)
	// ErrMediaAnalysisJobNotFound is returned when a media analysis job does not exist.
	ErrMediaAnalysisJobNotFound = awserr.New(errResourceNotFound, awserr.ErrNotFound)
	// ErrInvalidS3Object is returned when an Image/Video/Input references an
	// S3Object the wired S3 backend cannot find. Real AWS reports this as
	// InvalidS3ObjectException (rekognition@v1.54.4 types/errors.go:344):
	// "Amazon Rekognition is unable to access the S3 object specified in the
	// request".
	ErrInvalidS3Object = errors.New("s3 object is not accessible")

	// ErrProjectHasVersions is returned when DeleteProject is called on a
	// project that still has project versions. DeleteProject's own doc
	// comment (api_op_DeleteProject.go): "To delete a project you must
	// first delete all models or adapters associated with the project.".
	ErrProjectHasVersions = awserr.New(errResourceInUse, ErrNameInUse)
	// ErrProjectVersionInUse is returned when DeleteProjectVersion is called
	// on a version that is running or training. DeleteProjectVersion's own
	// doc comment (api_op_DeleteProjectVersion.go): "You can't delete a
	// project version if it is running or if it is training.".
	ErrProjectVersionInUse = awserr.New(errResourceInUse, ErrNameInUse)
	// ErrDatasetInUse is returned when DeleteDataset is called on a dataset
	// that is creating or updating. DeleteDataset's own doc comment
	// (api_op_DeleteDataset.go): "You can't delete a dataset while it is
	// creating (Status = CREATE_IN_PROGRESS) or if the dataset is updating
	// (Status = UPDATE_IN_PROGRESS).".
	ErrDatasetInUse = awserr.New(errResourceInUse, ErrNameInUse)
)

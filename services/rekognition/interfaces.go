package rekognition

import (
	"context"
	"time"

	sdk_s3 "github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Backend is the subset of S3 operations Rekognition needs to validate
// that an Image/Video/Input S3Object exists, wired via SetS3Backend. When
// unset, S3Object references are stored/echoed but never validated.
type S3Backend interface {
	HeadObject(ctx context.Context, input *sdk_s3.HeadObjectInput) (*sdk_s3.HeadObjectOutput, error)
}

// StorageBackend is the interface for Rekognition storage operations.
type StorageBackend interface {
	CreateCollection(collectionID string, tags map[string]string) (*Collection, error)
	DeleteCollection(collectionID string) error
	DescribeCollection(collectionID string) (*Collection, error)
	ListCollections(maxResults int32, nextToken string) ([]*Collection, string, error)

	IndexFaces(collectionID, externalImageID string) ([]*Face, error)
	DeleteFaces(collectionID string, faceIDs []string) ([]string, error)
	ListFaces(
		collectionID string, faceIDs []string, userID string, maxResults int32, nextToken string,
	) ([]*Face, string, error)
	SearchFaces(collectionID, faceID string, maxFaces int32) ([]*FaceMatch, error)
	SearchFacesByImage(collectionID string, maxFaces int32, imageKey string) ([]*FaceMatch, error)

	CreateStreamProcessor(
		name, roleARN string,
		params CreateStreamProcessorParams,
		tags map[string]string,
	) (*StreamProcessor, error)
	DeleteStreamProcessor(name string) error
	DescribeStreamProcessor(name string) (*StreamProcessor, error)
	ListStreamProcessors(maxResults int32, nextToken string) ([]*StreamProcessor, string, error)
	StartStreamProcessor(name string) error
	StopStreamProcessor(name string) error
	UpdateStreamProcessor(name string, params UpdateStreamProcessorParams) error

	TagResource(resourceARN string, tags map[string]string) error
	UntagResource(resourceARN string, tagKeys []string) error
	ListTagsForResource(resourceARN string) (map[string]string, error)

	// Projects and Project Versions
	CreateProject(name string, params CreateProjectParams) (*Project, error)
	DeleteProject(projectARN string) error
	DescribeProjects(projectARNs, features []string, maxResults int32, nextToken string) ([]*Project, string, error)
	CreateProjectVersion(
		projectARN, versionName string,
		params CreateProjectVersionParams,
		tags map[string]string,
	) (*ProjectVersion, error)
	DeleteProjectVersion(projectVersionARN string) error
	DescribeProjectVersions(projectARN string, versionNames []string, maxResults int32, nextToken string) (
		[]*ProjectVersion, string, error)
	CopyProjectVersion(
		sourceProjectVersionARN, destinationProjectARN, versionName string,
		params CopyProjectVersionParams,
	) (*ProjectVersion, error)
	StartProjectVersion(projectVersionARN string, minInferenceUnits, maxInferenceUnits int32) error
	StopProjectVersion(projectVersionARN string) error
	ListProjectPolicies(projectARN string, maxResults int32, nextToken string) ([]*ProjectPolicy, string, error)
	PutProjectPolicy(projectARN, policyName, policyDocument, policyRevisionID string) (string, error)
	DeleteProjectPolicy(projectARN, policyName, policyRevisionID string) error

	// Datasets
	CreateDataset(projectARN, datasetType string) (*Dataset, error)
	DeleteDataset(datasetARN string) error
	DescribeDataset(datasetARN string) (*Dataset, error)
	ListDatasetEntries(
		datasetARN string, filter ListDatasetEntriesFilter, maxResults int32, nextToken string,
	) ([]string, string, error)
	ListDatasetLabels(datasetARN string, maxResults int32, nextToken string) ([]*DatasetLabel, string, error)
	UpdateDatasetEntries(datasetARN string, changes []byte) error
	DistributeDatasetEntries(datasets []DatasetDistribution) error

	// Users
	CreateUser(collectionID, userID string) error
	DeleteUser(collectionID, userID string) error
	ListUsers(collectionID string, maxResults int32, nextToken string) ([]*User, string, error)
	AssociateFaces(
		collectionID, userID string,
		faceIDs []string,
	) ([]*AssociatedFace, []*UnsuccessfulFaceAssociation, error)
	DisassociateFaces(
		collectionID, userID string,
		faceIDs []string,
	) ([]*DisassociatedFace, []*UnsuccessfulFaceDisassociation, error)
	SearchUsers(collectionID, userID string, maxUsers int32) ([]*UserMatch, error)
	SearchUsersByFace(collectionID, faceID string, maxUsers int32) ([]*UserMatch, error)
	SearchUsersByImage(collectionID string, maxUsers int32, imageKey string) ([]*UserMatch, error)

	// Face Liveness
	CreateFaceLivenessSession() (string, error)
	GetFaceLivenessSessionResults(sessionID string) (*LivenessSessionResult, error)

	// Async video jobs
	StartAsyncJob(params StartAsyncJobParams) (string, error)
	GetAsyncJob(jobID string) (*AsyncJob, error)
	StartMediaAnalysisJob(jobName string, params StartMediaAnalysisJobParams) (string, error)
	GetMediaAnalysisJob(jobID string) (*MediaAnalysisJob, error)
	ListMediaAnalysisJobs(maxResults int32, nextToken string) ([]*MediaAnalysisJob, string, error)

	AccountID() string
	Region() string
	Reset()
	Snapshot(ctx context.Context) []byte
	Restore(ctx context.Context, data []byte) error
}

// Collection represents an Amazon Rekognition face collection.
// CreationTimestamp is first so its non-pointer prefix reduces GC pointer bytes.
type Collection struct {
	CreationTimestamp time.Time
	Tags              map[string]string
	CollectionID      string
	CollectionARN     string
	FaceModelVersion  string
	UserCount         int64
}

// Face represents an indexed face.
type Face struct {
	FaceID          string
	ImageID         string
	ExternalImageID string
	CollectionID    string
	Confidence      float64
}

// FaceMatch represents a face match result.
type FaceMatch struct {
	Face       *Face
	Similarity float64
}

// StreamProcessorInput mirrors AWS's types.StreamProcessorInput: the Kinesis
// video stream that provides the source streaming video.
type StreamProcessorInput struct {
	KinesisVideoStreamARN string
}

// StreamProcessorOutput mirrors AWS's types.StreamProcessorOutput: either a
// Kinesis data stream (face search) or an S3 destination (label detection).
type StreamProcessorOutput struct {
	KinesisDataStreamARN string
	S3Bucket             string
	S3KeyPrefix          string
}

// StreamProcessorSettings mirrors AWS's types.StreamProcessorSettings: either
// ConnectedHome (label detection) or FaceSearch settings.
type StreamProcessorSettings struct {
	ConnectedHomeMinConfidence   *float32
	FaceSearchFaceMatchThreshold *float32
	FaceSearchCollectionID       string
	ConnectedHomeLabels          []string
}

// Point mirrors AWS's types.Point: a single vertex of a RegionOfInterest polygon.
type Point struct {
	X *float32
	Y *float32
}

// BoundingBox mirrors AWS's types.BoundingBox: a rectangular region of interest.
type BoundingBox struct {
	Height *float32
	Left   *float32
	Top    *float32
	Width  *float32
}

// RegionOfInterest mirrors AWS's types.RegionOfInterest: a box or polygon
// area a stream processor checks for objects/people.
type RegionOfInterest struct {
	BoundingBox *BoundingBox
	Polygon     []Point
}

// StreamProcessorNotificationChannel mirrors AWS's
// types.StreamProcessorNotificationChannel.
type StreamProcessorNotificationChannel struct {
	SNSTopicARN string
}

// StreamProcessorDataSharingPreference mirrors AWS's
// types.StreamProcessorDataSharingPreference.
type StreamProcessorDataSharingPreference struct {
	OptIn bool
}

// CreateStreamProcessorParams groups CreateStreamProcessorInput's
// AWS-modeled fields beyond Name/RoleArn/Tags (Input/Output/Settings/
// NotificationChannel/DataSharingPreference/RegionsOfInterest/KmsKeyId), so
// the CreateStreamProcessor backend method signature doesn't grow an
// unbounded positional parameter list as fields are added.
type CreateStreamProcessorParams struct {
	Input                 *StreamProcessorInput
	Output                *StreamProcessorOutput
	Settings              *StreamProcessorSettings
	NotificationChannel   *StreamProcessorNotificationChannel
	DataSharingPreference *StreamProcessorDataSharingPreference
	KmsKeyID              string
	RegionsOfInterest     []RegionOfInterest
}

// UpdateStreamProcessorParams groups UpdateStreamProcessorInput's
// update-only fields. Presence/absence is signaled the same way the AWS wire
// shape does: a nil pointer/slice means "leave unchanged", a non-nil
// (possibly empty) pointer/slice means "the caller supplied this field".
// ParametersToDelete additionally clears RegionsOfInterest or
// ConnectedHomeMinConfidence regardless of what else is set, matching
// AWS's documented apply-then-delete semantics.
type UpdateStreamProcessorParams struct {
	DataSharingPreference      *StreamProcessorDataSharingPreference
	ConnectedHomeMinConfidence *float32
	ParametersToDelete         []string
	RegionsOfInterest          []RegionOfInterest
	ConnectedHomeLabels        []string
}

// StreamProcessor represents a Rekognition stream processor. Field order is
// fieldalignment-optimal (see `fieldalignment -fix`), not meaningful otherwise.
type StreamProcessor struct {
	LastUpdateTimestamp   time.Time
	CreationTimestamp     time.Time
	Input                 *StreamProcessorInput
	Tags                  map[string]string
	DataSharingPreference *StreamProcessorDataSharingPreference
	NotificationChannel   *StreamProcessorNotificationChannel
	Settings              *StreamProcessorSettings
	Output                *StreamProcessorOutput
	Name                  string
	KmsKeyID              string
	StatusMessage         string
	Status                string
	RoleARN               string
	StreamProcessorARN    string
	RegionsOfInterest     []RegionOfInterest
}

// Project represents a Rekognition Custom Labels project.
type Project struct {
	CreationTimestamp time.Time
	ProjectARN        string
	Status            string
	AutoUpdate        string
	Feature           string
}

// CreateProjectParams groups CreateProjectInput's fields beyond
// ProjectName/Tags. Feature defaults to CUSTOM_LABELS when empty per
// api_op_CreateProject.go's documented "If no value is provided
// CUSTOM_LABELS is used as a default." AutoUpdate has no documented
// default, so an empty value is stored and echoed back as empty rather
// than guessed.
type CreateProjectParams struct {
	AutoUpdate string
	Feature    string
}

// ProjectVersion represents a model version within a project.
type ProjectVersion struct {
	CreationTimestamp                       time.Time
	Tags                                    map[string]string
	FeatureConfigContentModConfidenceThresh *float32
	StatusMessage                           string
	VersionName                             string
	Status                                  string
	ProjectARN                              string
	OutputConfigS3Bucket                    string
	OutputConfigS3KeyPrefix                 string
	KmsKeyID                                string
	VersionDescription                      string
	SourceProjectVersionARN                 string
	ProjectVersionARN                       string
	MinInferenceUnits                       int32
	MaxInferenceUnits                       int32
}

// CreateProjectVersionParams groups CreateProjectVersionInput's fields
// beyond ProjectArn/VersionName/Tags (OutputConfig/KmsKeyId/
// VersionDescription), so the CreateProjectVersion backend method signature
// stays manageable as fields are added. FeatureConfigContentModConfidenceThresh
// is CustomizationFeatureConfig.ContentModeration.ConfidenceThreshold, a
// 2-level struct with no unions (types.go:486,495) -- shallow enough to model
// verbatim. TrainingData/TestingData are intentionally NOT modeled: both
// reference an external Custom Labels S3 manifest that this in-memory backend
// never trains against, so there is nowhere downstream (TrainingDataResult/
// TestingDataResult require a training-completion lifecycle this backend
// doesn't have) to surface a stored copy -- see PARITY.md deferred. Their
// presence is still cross-validated (see handleCreateProjectVersion).
type CreateProjectVersionParams struct {
	FeatureConfigContentModConfidenceThresh *float32
	OutputConfigS3Bucket                    string
	OutputConfigS3KeyPrefix                 string
	KmsKeyID                                string
	VersionDescription                      string
}

// CopyProjectVersionParams groups CopyProjectVersionInput's fields beyond
// SourceProjectVersionArn/DestinationProjectArn/VersionName: SourceProjectArn
// (the source project the copied version must belong to) and OutputConfig
// (where the copied training results are stored in the destination account).
type CopyProjectVersionParams struct {
	SourceProjectARN        string
	OutputConfigS3Bucket    string
	OutputConfigS3KeyPrefix string
}

// ProjectPolicy represents a project policy.
type ProjectPolicy struct {
	CreationTimestamp    time.Time
	LastUpdatedTimestamp time.Time
	ProjectARN           string
	PolicyName           string
	PolicyRevisionID     string
	PolicyDocument       string
}

// Dataset represents a Rekognition Custom Labels dataset.
type Dataset struct {
	CreationTimestamp    time.Time
	LastUpdatedTimestamp time.Time
	DatasetARN           string
	ProjectARN           string
	DatasetType          string
	Status               string
	StatusMessage        string
	Stats                DatasetStats
}

// DatasetStats mirrors types.DatasetStats (TotalEntries/LabeledEntries/
// TotalLabels, computed from the dataset's stored manifest entries;
// ErrorEntries is always 0 -- this backend has no entry-level error
// concept, so 0 is the accurate value, not a fabrication).
type DatasetStats struct {
	TotalEntries   int64
	LabeledEntries int64
	TotalLabels    int64
	ErrorEntries   int64
}

// DatasetLabel represents a label entry in a dataset.
type DatasetLabel struct {
	LabelName  string
	EntryCount int64
}

// DatasetDistribution is a dataset reference for DistributeDatasetEntries.
type DatasetDistribution struct {
	DatasetARN string
}

// User represents a Rekognition user in a collection.
type User struct {
	UserID     string
	UserStatus string
}

// AssociatedFace is a face successfully associated to a user.
type AssociatedFace struct {
	FaceID string
}

// UnsuccessfulFaceAssociation represents a face that couldn't be associated.
type UnsuccessfulFaceAssociation struct {
	FaceID  string
	Reasons []string
}

// DisassociatedFace is a face successfully disassociated from a user.
type DisassociatedFace struct {
	FaceID string
}

// UnsuccessfulFaceDisassociation represents a face that couldn't be disassociated.
type UnsuccessfulFaceDisassociation struct {
	FaceID  string
	Reasons []string
}

// UserMatch represents a user match result.
type UserMatch struct {
	User       *User
	Similarity float64
}

// LivenessSessionResult holds the result of a face liveness session.
type LivenessSessionResult struct {
	SessionID  string
	Status     string
	Confidence float32
}

// AsyncJob represents a Rekognition async video analysis job.
type AsyncJob struct {
	JobID          string
	JobStatus      string
	NextToken      string
	JobTag         string
	VideoS3Bucket  string
	VideoS3Name    string
	VideoS3Version string
	SegmentTypes   []string
}

// StartAsyncJobParams groups the StartXxx request fields common to every
// async video job family (Video/JobTag are echoed back verbatim by the
// matching GetXxx response; SegmentTypes only applies to
// StartSegmentDetection but is harmless zero-valued for the others).
type StartAsyncJobParams struct {
	JobType        string
	CollectionID   string
	JobTag         string
	VideoS3Bucket  string
	VideoS3Name    string
	VideoS3Version string
	SegmentTypes   []string
}

// MediaAnalysisJob represents a Rekognition media analysis job.
type MediaAnalysisJob struct {
	CreationTimestamp                    time.Time
	DetectModerationLabelsMinConfidence  *float32
	JobID                                string
	JobName                              string
	Status                               string
	InputS3Bucket                        string
	InputS3Name                          string
	InputS3Version                       string
	OutputConfigS3Bucket                 string
	OutputConfigS3KeyPrefix              string
	DetectModerationLabelsProjectVersion string
	HasDetectModerationLabels            bool
}

// StartMediaAnalysisJobParams groups StartMediaAnalysisJobInput's required
// Input/OperationsConfig/OutputConfig members beyond JobName, so the
// StartMediaAnalysisJob backend method signature stays manageable.
type StartMediaAnalysisJobParams struct {
	DetectModerationLabelsMinConfidence  *float32
	InputS3Bucket                        string
	InputS3Name                          string
	InputS3Version                       string
	OutputConfigS3Bucket                 string
	OutputConfigS3KeyPrefix              string
	DetectModerationLabelsProjectVersion string
	HasDetectModerationLabels            bool
}

var _ StorageBackend = (*InMemoryBackend)(nil)

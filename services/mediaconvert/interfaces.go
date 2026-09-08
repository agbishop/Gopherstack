package mediaconvert

import "context"

// StorageBackend defines the interface for MediaConvert backend implementations.
// All mutating methods must be safe for concurrent use.
type StorageBackend interface {
	// Queue operations
	CreateQueue(name, description, pricingPlan, status string, tags map[string]string) (*Queue, error)
	CreateQueueFull(
		name, description, pricingPlan, status string,
		tags map[string]string,
		concurrentJobs *int,
		reservationPlan *ReservationPlan,
		extras ...QueueCreateExtras,
	) (*Queue, error)
	GetQueue(name string) (*Queue, error)
	ListQueues() []*Queue
	UpdateQueue(
		name, description, status string,
		concurrentJobs *int,
		reservationPlanSettings *ReservationPlan,
		maximumConcurrentFeeds *int,
	) (*Queue, error)
	DeleteQueue(name string) error

	// Job Template operations
	CreateJobTemplate(
		name, description, category, queue string,
		priority int,
		settings map[string]any,
		tags map[string]string,
	) (*JobTemplate, error)
	CreateJobTemplateFull(
		name, description, category, queue string,
		priority int,
		settings map[string]any,
		tags map[string]string,
		accelerationMode, statusUpdateInterval string,
		hopDestinations []HopDestination,
	) (*JobTemplate, error)
	GetJobTemplate(name string) (*JobTemplate, error)
	ListJobTemplates() []*JobTemplate
	UpdateJobTemplate(
		name, description, category, queue string,
		priority *int,
		settings map[string]any,
	) (*JobTemplate, error)
	UpdateJobTemplateFull(
		name, description, category, queue string,
		priority *int,
		settings map[string]any,
		accelerationSettings *AccelerationSettings,
		statusUpdateInterval string,
		hopDestinations []HopDestination,
	) (*JobTemplate, error)
	DeleteJobTemplate(name string) error

	// Job operations
	CreateJob(
		role, queue, jobTemplate string,
		settings map[string]any,
		tags map[string]string,
		userMetadata map[string]string,
		billingTagsSource string,
	) (*Job, error)
	CreateJobFull(
		role, queue, jobTemplate string,
		settings map[string]any,
		tags map[string]string,
		userMetadata map[string]string,
		billingTagsSource, clientRequestToken, accelerationMode, jobEngineVersionReq string,
		priority int,
		hopDestinations []HopDestination,
		extras ...JobCreateExtras,
	) (*Job, error)
	GetJob(id string) (*Job, error)
	ListJobs() []*Job
	ListJobsFiltered(status, queue, order string) []*Job
	CancelJob(id string) error
	UpdateJob(id, queue string, priority *int, hopDestinations []HopDestination) (*Job, error)

	// Preset operations
	CreatePreset(name, description, category string, settings map[string]any, tags map[string]string) (*Preset, error)
	GetPreset(name string) (*Preset, error)
	ListPresets() []*Preset
	UpdatePreset(name, description, category string, settings map[string]any) (*Preset, error)
	DeletePreset(name string) error

	// Policy operations
	GetPolicy() (*Policy, error)
	PutPolicy(httpInputs, httpsInputs, s3Inputs string) *Policy
	DeletePolicy() error

	// Certificate operations
	AssociateCertificate(certARN string) error
	DisassociateCertificate(certARN string) error

	// Jobs query / resource share operations
	GetJobsQueryResults(queryID string) []*Job
	StartJobsQuery(filterList []map[string]any, maxResults int, order string) (string, error)
	CreateResourceShare(jobID, supportCaseID string) (string, error)

	// Tag operations
	GetTags(resourceARN string) map[string]string
	TagResource(resourceARN string, tags map[string]string)
	UntagResource(resourceARN string, tagKeys []string)

	// Lifecycle
	Reset()
	Region() string
	AccountID() string
	Snapshot(ctx context.Context) []byte
	Restore(ctx context.Context, data []byte) error
}

// compile-time assertion that InMemoryBackend satisfies StorageBackend.
var _ StorageBackend = (*InMemoryBackend)(nil)

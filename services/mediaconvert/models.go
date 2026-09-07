package mediaconvert

import "time"

// ReservationPlan holds reservation plan details for a queue.
type ReservationPlan struct {
	Status        string  `json:"status,omitempty"`
	Commitment    string  `json:"commitment,omitempty"`
	RenewalType   string  `json:"renewalType,omitempty"`
	ExpiresAt     float64 `json:"expiresAt,omitempty"`
	PurchasedAt   float64 `json:"purchasedAt,omitempty"`
	ReservedSlots int     `json:"reservedSlots,omitempty"`
}

// Queue represents a MediaConvert queue.
type Queue struct {
	ReservationPlan *ReservationPlan  `json:"reservationPlan,omitempty"`
	Tags            map[string]string `json:"tags,omitempty"`
	// MaximumConcurrentFeeds is *int32 on the real wire (CreateQueueInput/
	// UpdateQueueInput/Queue, aws-sdk-go-v2/service/mediaconvert@v1.97.1
	// api_op_CreateQueue.go:47-49, deserializers.go:24653+96), so nil vs a
	// caller-supplied 0 must stay distinguishable -- never default/guess.
	MaximumConcurrentFeeds *int `json:"maximumConcurrentFeeds,omitempty"`
	// ConcurrentJobs is *int32 on the real wire too (CreateQueueInput/
	// UpdateQueueInput/types.Queue.ConcurrentJobs, api_op_CreateQueue.go:42,
	// api_op_UpdateQueue.go:40, types/types.go:8622; gopherstack-7bxb): a
	// plain int here collapsed "not set" (unlimited) and "explicitly 0" into
	// the same value.
	ConcurrentJobs       *int    `json:"concurrentJobs,omitempty"`
	Arn                  string  `json:"arn"`
	Name                 string  `json:"name"`
	Description          string  `json:"description,omitempty"`
	PricingPlan          string  `json:"pricingPlan"`
	Status               string  `json:"status"`
	Type                 string  `json:"type"`
	CreatedAt            float64 `json:"createdAt"`
	LastUpdated          float64 `json:"lastUpdated"`
	ProgressingJobsCount int     `json:"progressingJobsCount"`
	SubmittedJobsCount   int     `json:"submittedJobsCount"`
}

// JobTemplate represents a MediaConvert job template.
type JobTemplate struct {
	AccelerationSettings *AccelerationSettings `json:"accelerationSettings,omitempty"`
	Settings             map[string]any        `json:"settings,omitempty"`
	Tags                 map[string]string     `json:"tags,omitempty"`
	Type                 string                `json:"type"`
	Arn                  string                `json:"arn"`
	Name                 string                `json:"name"`
	Description          string                `json:"description,omitempty"`
	Category             string                `json:"category,omitempty"`
	Queue                string                `json:"queue,omitempty"`
	StatusUpdateInterval string                `json:"statusUpdateInterval,omitempty"`
	HopDestinations      []HopDestination      `json:"hopDestinations,omitempty"`
	CreatedAt            float64               `json:"createdAt"`
	LastUpdated          float64               `json:"lastUpdated"`
	Priority             int                   `json:"priority"`
}

// JobTiming holds timing information for a MediaConvert job.
type JobTiming struct {
	SubmitTime float64 `json:"submitTime,omitempty"`
	StartTime  float64 `json:"startTime,omitempty"`
	FinishTime float64 `json:"finishTime,omitempty"`
}

// HopDestination represents a queue hop destination for a job.
type HopDestination struct {
	Queue       string `json:"queue,omitempty"`
	WaitMinutes int    `json:"waitMinutes,omitempty"`
	Priority    int    `json:"priority,omitempty"`
}

// QueueTransition records a queue change event for a job.
type QueueTransition struct {
	SourceQueue      string  `json:"sourceQueue,omitempty"`
	DestinationQueue string  `json:"destinationQueue,omitempty"`
	Timestamp        float64 `json:"timestamp,omitempty"`
}

// OutputDetail contains output-level detail for a completed job.
type OutputDetail struct {
	VideoDetails *VideoDetail `json:"videoDetails,omitempty"`
	DurationInMs int          `json:"durationInMs,omitempty"`
}

// VideoDetail holds video dimension details.
type VideoDetail struct {
	WidthInPx  int `json:"widthInPx,omitempty"`
	HeightInPx int `json:"heightInPx,omitempty"`
}

// OutputGroupDetail contains details for one output group.
type OutputGroupDetail struct {
	OutputDetails []OutputDetail `json:"outputDetails,omitempty"`
}

// JobMessages holds informational and warning messages for a job.
type JobMessages struct {
	Info    []string `json:"info,omitempty"`
	Warning []string `json:"warning,omitempty"`
}

// WarningGroup represents an aggregated warning count.
type WarningGroup struct {
	Code  string `json:"code,omitempty"`
	Count int    `json:"count,omitempty"`
}

// AccelerationSettings holds the requested acceleration mode.
type AccelerationSettings struct {
	Mode string `json:"mode,omitempty"`
}

// Job represents a MediaConvert transcoding job.
type Job struct {
	AccelerationSettings *AccelerationSettings `json:"accelerationSettings,omitempty"`
	Messages             *JobMessages          `json:"messages,omitempty"`
	// LastShareDetails is a *string* on the real wire (aws-sdk-go-v2/service/
	// mediaconvert@v1.97.1 types/types.go:6202, deserializers.go:19625 expects
	// value.(string)) -- NOT a nested object. A structured object here fails
	// the entire GetJob/ListJobs deserialization for a real SDK client.
	LastShareDetails          *string             `json:"lastShareDetails,omitempty"`
	Timing                    *JobTiming          `json:"timing,omitempty"`
	Settings                  map[string]any      `json:"settings,omitempty"`
	Tags                      map[string]string   `json:"tags,omitempty"`
	UserMetadata              map[string]string   `json:"userMetadata,omitempty"`
	Arn                       string              `json:"arn"`
	ID                        string              `json:"id"`
	Queue                     string              `json:"queue,omitempty"`
	QueueArn                  string              `json:"queueArn,omitempty"`
	Role                      string              `json:"role"`
	Status                    string              `json:"status"`
	CurrentPhase              string              `json:"currentPhase,omitempty"`
	JobTemplate               string              `json:"jobTemplate,omitempty"`
	ErrorMessage              string              `json:"errorMessage,omitempty"`
	BillingTagsSource         string              `json:"billingTagsSource,omitempty"`
	AccelerationStatus        string              `json:"accelerationStatus,omitempty"`
	StatusUpdateInterval      string              `json:"statusUpdateInterval,omitempty"`
	SimulateReservedQueue     string              `json:"simulateReservedQueue,omitempty"`
	ClientRequestToken        string              `json:"clientRequestToken,omitempty"`
	JobEngineVersionRequested string              `json:"jobEngineVersionRequested,omitempty"`
	JobEngineVersionUsed      string              `json:"jobEngineVersionUsed,omitempty"`
	ShareStatus               string              `json:"shareStatus,omitempty"`
	OutputGroupDetails        []OutputGroupDetail `json:"outputGroupDetails,omitempty"`
	QueueTransitions          []QueueTransition   `json:"queueTransitions,omitempty"`
	HopDestinations           []HopDestination    `json:"hopDestinations,omitempty"`
	Warnings                  []WarningGroup      `json:"warnings,omitempty"`
	CreatedAt                 float64             `json:"createdAt"`
	ErrorCode                 int                 `json:"errorCode,omitempty"`
	JobPercentComplete        int                 `json:"jobPercentComplete"`
	Priority                  int                 `json:"priority"`
	RetryCount                int                 `json:"retryCount"`
}

// Preset represents a MediaConvert output preset.
type Preset struct {
	Settings    map[string]any    `json:"settings,omitempty"`
	Tags        map[string]string `json:"tags,omitempty"`
	Arn         string            `json:"arn"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Category    string            `json:"category,omitempty"`
	Type        string            `json:"type"`
	CreatedAt   float64           `json:"createdAt"`
	LastUpdated float64           `json:"lastUpdated"`
}

// Policy represents a MediaConvert account policy.
type Policy struct {
	HTTPInputs  string `json:"httpInputs,omitempty"`
	HTTPSInputs string `json:"httpsInputs,omitempty"`
	S3Inputs    string `json:"s3Inputs,omitempty"`
}

// jobsQuery stores the parameters of a StartJobsQuery call for deferred execution.
//
// queryID is the store.Table key (see store_setup.go). The queries table is
// never persisted (StartJobsQuery results were never part of backendSnapshot
// pre-Phase-3.3 either), so queryID needs no json tag -- it just has to be a
// pure function of the value for store.Table's keyFn.
type jobsQuery struct {
	queryID    string
	order      string
	filterList []map[string]any
	maxResults int
}

// tokenEntry records a ClientRequestToken for deduplication.
//
// token is the store.Table key (see store_setup.go). Like createdAt/jobID it
// is unexported, so it (and they) are silently excluded from tokenEntry's own
// JSON encoding; persistence.go instead round-trips tokenIndex through a
// dedicated tokenSnapshot DTO that carries token as an exported field -- see
// persistence.go's file doc comment. This preserves the pre-Phase-3.3
// behavior byte for byte: createdAt/jobID were already unexported before this
// refactor, so a persisted tokenIndex entry was already encoded as `{}` and
// restored with zeroed createdAt/jobID; that quirk is intentionally left
// unchanged here.
type tokenEntry struct {
	createdAt time.Time
	jobID     string
	token     string
}

// queueJobCounter tracks active job counts for a single queue.
//
// queueArn is the store.Table key (see store_setup.go), unexported for the
// same reason as tokenEntry.token above: submitted/progressing were already
// unexported pre-Phase-3.3, so persisted queueCounters entries were already
// lossy (`{}`, restored as zero) -- see persistence.go's counterSnapshot DTO,
// which preserves that existing quirk rather than fixing it.
type queueJobCounter struct {
	queueArn    string
	submitted   int
	progressing int
}

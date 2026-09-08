package ses

import "time"

// Tag is an email metadata key-value pair.
type Tag struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// IdentityRecord stores per-identity verification and attribute state.
type IdentityRecord struct {
	// Identity is the address or domain this record is keyed by in the
	// identities Table (see store_setup.go). It is tagged json:"-" because
	// the identities Table is a "dirty" table -- persistence.go instead
	// round-trips it through a dedicated identitySnapshot DTO that carries
	// the identity as a real JSON field, so it survives the round trip
	// despite being excluded here. It must never change after the record is
	// created (store.Table's keyFn purity requirement).
	Identity           string   `json:"-"`
	DeliveryTopic      string   `json:"deliveryTopic,omitempty"`
	MailFromDomain     string   `json:"mailFromDomain,omitempty"`
	MailFromStatus     string   `json:"mailFromStatus,omitempty"`
	BehaviorOnMXFail   string   `json:"behaviorOnMXFailure,omitempty"`
	BounceTopic        string   `json:"bounceTopic,omitempty"`
	ComplaintTopic     string   `json:"complaintTopic,omitempty"`
	DkimTokens         []string `json:"dkimTokens,omitempty"`
	DkimEnabled        bool     `json:"dkimEnabled"`
	ForwardingEnabled  bool     `json:"forwardingEnabled"`
	HeadersInBounce    bool     `json:"headersInBounce"`
	HeadersInComplaint bool     `json:"headersInComplaint"`
	HeadersInDelivery  bool     `json:"headersInDelivery"`
	Verified           bool     `json:"verified"`
}

// ConfigurationSet stores per-configuration-set state.
type ConfigurationSet struct {
	// Name is the configuration set name this value is keyed by in the
	// configSets Table (see store_setup.go). Tagged json:"-" for the same
	// reason as IdentityRecord.Identity -- see its doc comment.
	Name              string `json:"-"`
	TLSPolicy         string `json:"tlsPolicy,omitempty"`
	SendingEnabled    bool   `json:"sendingEnabled"`
	ReputationMetrics bool   `json:"reputationMetrics"`
}

// BulkEmailDestination is a single destination entry for SendBulkTemplatedEmail.
// ReplacementTags mirrors the real SendBulkTemplatedEmailInput
// BulkEmailDestination.ReplacementTags member: when non-empty it overrides
// the request-level SendBulkTemplatedEmailInput.DefaultTags for this
// destination's stored Email record.
type BulkEmailDestination struct {
	ReplacementTemplateData string
	To                      []string
	Cc                      []string
	Bcc                     []string
	ReplacementTags         []Tag
}

// SendEmailInput contains all parameters for sending an email.
type SendEmailInput struct {
	Tags                 []Tag
	From                 string
	Subject              string
	BodyHTML             string
	BodyText             string
	ConfigurationSetName string
	ReturnPath           string
	ReturnPathArn        string
	SourceArn            string
	To                   []string
	Cc                   []string
	Bcc                  []string
	ReplyTo              []string
}

// SendTemplatedEmailInput contains all parameters for sending a templated email.
type SendTemplatedEmailInput struct {
	Tags                 []Tag
	From                 string
	TemplateName         string
	TemplateData         string
	ConfigurationSetName string
	ReturnPath           string
	ReturnPathArn        string
	SourceArn            string
	To                   []string
	Cc                   []string
	Bcc                  []string
	ReplyTo              []string
}

// SendBulkTemplatedEmailInput contains all parameters for
// SendBulkTemplatedEmail, mirroring aws-sdk-go-v2/service/ses's
// SendBulkTemplatedEmailInput. DefaultTags is applied to every destination's
// stored Email record unless overridden by that destination's
// BulkEmailDestination.ReplacementTags.
type SendBulkTemplatedEmailInput struct {
	Source               string
	TemplateName         string
	DefaultTemplateData  string
	ConfigurationSetName string
	ReturnPath           string
	ReturnPathArn        string
	SourceArn            string
	ReplyTo              []string
	DefaultTags          []Tag
	Destinations         []BulkEmailDestination
}

// Email captures a sent email for local inspection.
type Email struct {
	Tags                 []Tag     `json:"tags,omitempty"`
	Timestamp            time.Time `json:"timestamp"`
	From                 string    `json:"from"`
	Subject              string    `json:"subject"`
	BodyHTML             string    `json:"bodyHTML"`
	BodyText             string    `json:"bodyText"`
	MessageID            string    `json:"messageID"`
	ConfigurationSetName string    `json:"configurationSetName,omitempty"`
	ReturnPath           string    `json:"returnPath,omitempty"`
	ReturnPathArn        string    `json:"returnPathArn,omitempty"`
	SourceArn            string    `json:"sourceArn,omitempty"`
	To                   []string  `json:"to"`
	Cc                   []string  `json:"cc,omitempty"`
	Bcc                  []string  `json:"bcc,omitempty"`
	ReplyTo              []string  `json:"replyTo,omitempty"`
	// Bounced/Complained are set when a recipient is one of the AWS SES mailbox
	// simulator's documented deterministic addresses (bounce@/suppressionlist@/
	// complaint@simulator.amazonses.com), the only real, publicly documented way
	// to trigger a bounce or complaint outcome deterministically. See
	// classifySimulatedRecipients.
	Bounced    bool `json:"bounced,omitempty"`
	Complained bool `json:"complained,omitempty"`
}

// EmailTemplate represents a stored SES email template.
type EmailTemplate struct {
	TemplateName string `json:"templateName"`
	SubjectPart  string `json:"subjectPart"`
	TextPart     string `json:"textPart"`
	HTMLPart     string `json:"htmlPart"`
}

// ReceiptRuleSet represents an SES receipt rule set.
type ReceiptRuleSet struct {
	Name      string        `json:"name"`
	CreatedAt time.Time     `json:"createdAt"`
	Rules     []ReceiptRule `json:"rules"`
}

const (
	ReceiptActionTypeS3        = "S3"
	ReceiptActionTypeSNS       = "SNS"
	ReceiptActionTypeLambda    = "Lambda"
	ReceiptActionTypeAddHeader = "AddHeader"
	ReceiptActionTypeBounce    = "Bounce"
	ReceiptActionTypeStop      = "Stop"
)

// ReceiptAction is a single action within a receipt rule.
// Type identifies which action fields apply: S3, SNS, Lambda, AddHeader, Bounce, Stop.
type ReceiptAction struct {
	Type              string `json:"type"`
	S3BucketName      string `json:"s3BucketName,omitempty"`
	S3KeyPrefix       string `json:"s3KeyPrefix,omitempty"`
	S3TopicARN        string `json:"s3TopicARN,omitempty"`
	SNSTopicARN       string `json:"snsTopicARN,omitempty"`
	LambdaFunctionARN string `json:"lambdaFunctionARN,omitempty"`
	LambdaTopicARN    string `json:"lambdaTopicARN,omitempty"`
	// SQSQueueARN and SQSTopicARN are vestigial (gopherstack-brmq): SQS was
	// never a real ReceiptAction member (ses@v1.37.4 types.ReceiptAction has
	// no SQS variant) and no code path sets Type to "SQS" any more. Kept
	// only so a pre-existing persisted snapshot that has one still decodes
	// without silently dropping the ARNs -- removing these fields would
	// change backendSnapshot's shape without a version bump.
	SQSQueueARN    string `json:"sqsQueueARN,omitempty"`
	SQSTopicARN    string `json:"sqsTopicARN,omitempty"`
	HeaderName     string `json:"headerName,omitempty"`
	HeaderValue    string `json:"headerValue,omitempty"`
	SMTPReplyCode  string `json:"smtpReplyCode,omitempty"`
	StatusCode     string `json:"statusCode,omitempty"`
	Message        string `json:"message,omitempty"`
	Sender         string `json:"sender,omitempty"`
	BounceTopicARN string `json:"bounceTopicARN,omitempty"`
}

// ReceiptRule represents a single receipt rule within a rule set.
type ReceiptRule struct {
	Name        string          `json:"name"`
	TLSPolicy   string          `json:"tlsPolicy"`
	Recipients  []string        `json:"recipients"`
	Actions     []ReceiptAction `json:"actions,omitempty"`
	Enabled     bool            `json:"enabled"`
	ScanEnabled bool            `json:"scanEnabled"`
}

// ReceiptFilter represents an IP-based receipt filter.
type ReceiptFilter struct {
	Name   string `json:"name"`
	Policy string `json:"policy"`
	CIDR   string `json:"cidr"`
}

// EventDestination represents a configuration set event destination.
type EventDestination struct {
	// ConfigSetName is the parent configuration set name. Combined with Name
	// it forms the composite key ("<ConfigSetName>#<Name>", see
	// eventDestinationKey in store_setup.go) the flattened eventDestinations
	// Table is keyed by -- this Table replaces what was previously a nested
	// map[string]map[string]*EventDestination. Tagged json:"-" for the same
	// reason as IdentityRecord.Identity -- see its doc comment.
	ConfigSetName      string   `json:"-"`
	Name               string   `json:"name"`
	SNSTopicARN        string   `json:"snsTopicARN,omitempty"`
	MatchingEventTypes []string `json:"matchingEventTypes"`
	Enabled            bool     `json:"enabled"`
}

// TrackingOptions represents the tracking options for a configuration set.
type TrackingOptions struct {
	// ConfigSetName is the configuration set name this value is keyed by in
	// the trackingOptions Table (see store_setup.go). Tagged json:"-" for
	// the same reason as IdentityRecord.Identity -- see its doc comment.
	ConfigSetName        string `json:"-"`
	CustomRedirectDomain string `json:"customRedirectDomain"`
}

// CustomVerificationEmailTemplate represents a custom verification email template.
type CustomVerificationEmailTemplate struct {
	TemplateName          string `json:"templateName"`
	FromEmailAddress      string `json:"fromEmailAddress"`
	TemplateSubject       string `json:"templateSubject"`
	TemplateContent       string `json:"templateContent"`
	SuccessRedirectionURL string `json:"successRedirectionURL"`
	FailureRedirectionURL string `json:"failureRedirectionURL"`
}

// SendQuota holds the simulated SES sending quota values.
type SendQuota struct {
	Max24HourSend   float64
	MaxSendRate     float64
	SentLast24Hours float64
}

// SendDataPoint represents a single send statistics time bucket.
type SendDataPoint struct {
	Timestamp        time.Time `json:"timestamp"`
	DeliveryAttempts float64   `json:"deliveryAttempts"`
	Bounces          float64   `json:"bounces"`
	Complaints       float64   `json:"complaints"`
	Rejects          float64   `json:"rejects"`
}

const (
	FilterPolicyAllow   = "Allow"
	FilterPolicyBlock   = "Block"
	TLSPolicyOptional   = "Optional"
	TLSPolicyRequire    = "Require"
	ruleSetStatusActive = "Active"

	// identityStatusSuccess is the verification status for verified identities.
	identityStatusSuccess = "Success"
	// identityStatusNotStarted is the default status for unverified identities.
	identityStatusNotStarted = "NotStarted"
)

const (
	notifTypeBounce    = "Bounce"
	notifTypeComplaint = "Complaint"
	notifTypeDelivery  = "Delivery"
)

// DkimAttributes holds DKIM verification attributes for an identity.
type DkimAttributes struct {
	DkimVerificationStatus string
	DkimTokens             []string
	DkimEnabled            bool
}

// MailFromDomainAttributes holds MailFrom domain attributes for an identity.
type MailFromDomainAttributes struct {
	MailFromDomain       string
	MailFromDomainStatus string
	BehaviorOnMXFailure  string
}

// behaviorOnMXFailureUseDefault and behaviorOnMXFailureReject are the two legal
// values of the SetIdentityMailFromDomain BehaviorOnMXFailure parameter,
// matching the AWS SES BehaviorOnMXFailure enum. UseDefaultValue is the
// real-AWS default when the parameter is omitted.
const (
	behaviorOnMXFailureUseDefault = "UseDefaultValue"
	behaviorOnMXFailureReject     = "RejectMessage"
)

// NotificationAttributes holds notification topic attributes for an identity.
type NotificationAttributes struct {
	BounceTopic        string
	ComplaintTopic     string
	DeliveryTopic      string
	ForwardingEnabled  bool
	HeadersInBounce    bool
	HeadersInComplaint bool
	HeadersInDelivery  bool
}

// ConfigurationSetDescription holds full details of a configuration set.
type ConfigurationSetDescription struct {
	TrackingOptions          *TrackingOptions
	DeliveryOptions          *DeliveryOptions
	Name                     string
	EventDestinations        []EventDestination
	SendingEnabled           bool
	ReputationMetricsEnabled bool
}

// DeliveryOptions holds the delivery options for a configuration set.
type DeliveryOptions struct {
	TLSPolicy string `json:"tlsPolicy,omitempty"`
}

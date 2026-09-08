package bedrock

import (
	"encoding/json"
	"time"
)

// Tag represents a key-value tag on a Bedrock resource.
type Tag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// GuardrailContentFilter defines a single content filter rule within a guardrail.
type GuardrailContentFilter struct {
	Type           string `json:"type"`
	InputStrength  string `json:"inputStrength"`
	OutputStrength string `json:"outputStrength"`
}

// GuardrailContentPolicyConfig configures content filtering for a guardrail.
type GuardrailContentPolicyConfig struct {
	FiltersConfig []GuardrailContentFilter `json:"filtersConfig"`
}

// GuardrailTopic defines a topic that the guardrail denies.
type GuardrailTopic struct {
	Name       string   `json:"name"`
	Definition string   `json:"definition"`
	Type       string   `json:"type"`
	Examples   []string `json:"examples,omitempty"`
}

// GuardrailTopicPolicyConfig configures topic-level denial policies.
type GuardrailTopicPolicyConfig struct {
	TopicsConfig []GuardrailTopic `json:"topicsConfig"`
}

// GuardrailManagedWordList references a managed word list by type.
type GuardrailManagedWordList struct {
	Type string `json:"type"`
}

// GuardrailWordConfig defines a single custom word to block.
type GuardrailWordConfig struct {
	Text string `json:"text"`
}

// GuardrailWordPolicyConfig configures word-level blocking for a guardrail.
type GuardrailWordPolicyConfig struct {
	WordsConfig            []GuardrailWordConfig      `json:"wordsConfig,omitempty"`
	ManagedWordListsConfig []GuardrailManagedWordList `json:"managedWordListsConfig,omitempty"`
}

// GuardrailPIIEntity describes a PII entity type and the action to take.
type GuardrailPIIEntity struct {
	Type   string `json:"type"`
	Action string `json:"action"`
}

// GuardrailRegexConfig defines a custom regex-based filter.
type GuardrailRegexConfig struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Pattern     string `json:"pattern"`
	Action      string `json:"action"`
}

// GuardrailSensitiveInformationPolicyConfig configures PII and regex-based filters.
type GuardrailSensitiveInformationPolicyConfig struct {
	PiiEntitiesConfig []GuardrailPIIEntity   `json:"piiEntitiesConfig,omitempty"`
	RegexesConfig     []GuardrailRegexConfig `json:"regexesConfig,omitempty"`
}

// GuardrailContextualGroundingFilter is a single contextual grounding rule.
type GuardrailContextualGroundingFilter struct {
	Type      string  `json:"type"`
	Threshold float64 `json:"threshold"`
}

// GuardrailContextualGroundingPolicyConfig configures contextual grounding checks.
type GuardrailContextualGroundingPolicyConfig struct {
	FiltersConfig []GuardrailContextualGroundingFilter `json:"filtersConfig"`
}

// GuardrailPolicies groups all optional policy configurations for a guardrail.
type GuardrailPolicies struct {
	ContentPolicy              *GuardrailContentPolicyConfig              `json:"contentPolicyConfig,omitempty"`
	TopicPolicy                *GuardrailTopicPolicyConfig                `json:"topicPolicyConfig,omitempty"`
	WordPolicy                 *GuardrailWordPolicyConfig                 `json:"wordPolicyConfig,omitempty"`
	SensitiveInformationPolicy *GuardrailSensitiveInformationPolicyConfig `json:"sensitiveInformationPolicyConfig,omitempty"` //nolint:lll // AWS API field name is long.
	ContextualGroundingPolicy  *GuardrailContextualGroundingPolicyConfig  `json:"contextualGroundingPolicyConfig,omitempty"`  //nolint:lll // AWS API field name is long.
}

// Guardrail represents an Amazon Bedrock guardrail.
type Guardrail struct {
	CreatedAt               time.Time          `json:"createdAt"`
	UpdatedAt               time.Time          `json:"updatedAt"`
	Policies                *GuardrailPolicies `json:"policies,omitempty"`
	GuardrailID             string             `json:"guardrailId"`
	GuardrailArn            string             `json:"guardrailArn"`
	Name                    string             `json:"name"`
	Description             string             `json:"description,omitempty"`
	Status                  string             `json:"status"`
	Version                 string             `json:"version"`
	BlockedInputMessaging   string             `json:"blockedInputMessaging,omitempty"`
	BlockedOutputsMessaging string             `json:"blockedOutputsMessaging,omitempty"`
	Tags                    []Tag              `json:"tags,omitempty"`
	// versionCounter tracks the next version number for this specific guardrail.
	versionCounter int
}

// GuardrailSummary is used in list operations.
type GuardrailSummary struct {
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	GuardrailID string    `json:"id"`
	Arn         string    `json:"arn"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Status      string    `json:"status"`
	Version     string    `json:"version"`
}

// ProvisionedModelThroughput represents a provisioned model throughput resource.
type ProvisionedModelThroughput struct {
	CreationTime         time.Time `json:"creationTime"`
	LastModifiedTime     time.Time `json:"lastModifiedTime"`
	ProvisionedModelArn  string    `json:"provisionedModelArn"`
	ProvisionedModelName string    `json:"provisionedModelName"`
	ModelArn             string    `json:"modelArn"`
	DesiredModelArn      string    `json:"desiredModelArn"`
	FoundationModelArn   string    `json:"foundationModelArn"`
	Status               string    `json:"status"`
	CommitmentDuration   string    `json:"commitmentDuration,omitempty"`
	Tags                 []Tag     `json:"tags,omitempty"`
	ModelUnits           int32     `json:"modelUnits"`
	DesiredModelUnits    int32     `json:"desiredModelUnits"`
}

// FoundationModelLifecycle holds the lifecycle status of a foundation model.
type FoundationModelLifecycle struct {
	Status string `json:"status"`
}

// FoundationModelSummary represents a foundation model.
type FoundationModelSummary struct {
	ModelLifecycle             *FoundationModelLifecycle `json:"modelLifecycle,omitempty"`
	ModelArn                   string                    `json:"modelArn"`
	ModelID                    string                    `json:"modelId"`
	ModelName                  string                    `json:"modelName"`
	ProviderName               string                    `json:"providerName"`
	InputModalities            []string                  `json:"inputModalities,omitempty"`
	OutputModalities           []string                  `json:"outputModalities,omitempty"`
	InferenceTypesSupported    []string                  `json:"inferenceTypesSupported,omitempty"`
	CustomizationsSupported    []string                  `json:"customizationsSupported,omitempty"`
	ResponseStreamingSupported bool                      `json:"responseStreamingSupported"`
}

// EvaluationPerformanceConfig mirrors types.PerformanceConfiguration
// (bedrock@v1.66.4 types/types.go:5787-5794).
type EvaluationPerformanceConfig struct {
	Latency string `json:"latency,omitempty"`
}

// EvaluationBedrockModel mirrors types.EvaluationBedrockModel (bedrock@v1.66.4
// types/types.go:2827-2842).
type EvaluationBedrockModel struct {
	PerformanceConfig *EvaluationPerformanceConfig `json:"performanceConfig,omitempty"`
	ModelIdentifier   string                       `json:"modelIdentifier"`
	InferenceParams   string                       `json:"inferenceParams,omitempty"`
}

// EvaluationPrecomputedInferenceSource mirrors
// types.EvaluationPrecomputedInferenceSource (bedrock@v1.66.4
// types/types.go:3064-3075).
type EvaluationPrecomputedInferenceSource struct {
	InferenceSourceIdentifier string `json:"inferenceSourceIdentifier,omitempty"`
}

// EvaluationModelConfig models bedrock's evaluationModelConfig union
// (types.EvaluationModelConfig, bedrock@v1.66.4 types/types.go:3008-3034).
// smithy-go serializes/deserializes it as a single-key object naming the
// variant ("bedrockModel" or "precomputedInferenceSource",
// serializers.go:10841-10863); encoding/json's field-presence dispatch on the
// struct tags mirrors that mechanism directly.
type EvaluationModelConfig struct {
	BedrockModel               *EvaluationBedrockModel               `json:"bedrockModel,omitempty"`
	PrecomputedInferenceSource *EvaluationPrecomputedInferenceSource `json:"precomputedInferenceSource,omitempty"`
}

// EvaluatorModel mirrors types.BedrockEvaluatorModel (bedrock@v1.66.4
// types/types.go:2448-2460).
type EvaluatorModel struct {
	ModelIdentifier string `json:"modelIdentifier"`
}

// EvaluatorModelConfig models bedrock's evaluatorModelConfig union
// (types.EvaluatorModelConfig, bedrock@v1.66.4 types/types.go:3226-3235).
// Only one variant exists today ("bedrockEvaluatorModels").
type EvaluatorModelConfig struct {
	BedrockEvaluatorModels []EvaluatorModel `json:"bedrockEvaluatorModels,omitempty"`
}

// EvaluationDatasetLocation models bedrock's evaluationDatasetLocation union
// (types.EvaluationDatasetLocation, bedrock@v1.66.4 types/types.go:2892-2908);
// s3Uri is its sole real variant today.
type EvaluationDatasetLocation struct {
	S3URI string `json:"s3Uri,omitempty"`
}

// EvaluationDataset references an evaluation dataset (types.EvaluationDataset,
// bedrock@v1.66.4 types/types.go:2873-2890).
type EvaluationDataset struct {
	Location *EvaluationDatasetLocation `json:"datasetLocation,omitempty"`
	Name     string                     `json:"name,omitempty"`
}

// EvaluationDatasetMetricConfig mirrors types.EvaluationDatasetMetricConfig
// (bedrock@v1.66.4 types/types.go:2910-2951). MetricNames is a flat []string
// on the real wire, not a wrapped-object list.
type EvaluationDatasetMetricConfig struct {
	Dataset     *EvaluationDataset `json:"dataset,omitempty"`
	TaskType    string             `json:"taskType,omitempty"`
	MetricNames []string           `json:"metricNames,omitempty"`
}

// AutomatedEvaluationConfig mirrors types.AutomatedEvaluationConfig
// (bedrock@v1.66.4 types/types.go:145-166). CustomMetricConfig is stored
// verbatim: it wraps its own further union
// (AutomatedEvaluationCustomMetricSource, types/types.go:196, plus a nested
// CustomMetricEvaluatorModelConfig) that gopherstack's inert evaluation-job
// store never interprets -- same rationale as
// AutomatedReasoningPolicy.PolicyDefinition above.
type AutomatedEvaluationConfig struct {
	EvaluatorModelConfig *EvaluatorModelConfig           `json:"evaluatorModelConfig,omitempty"`
	DatasetMetricConfigs []EvaluationDatasetMetricConfig `json:"datasetMetricConfigs,omitempty"`
	CustomMetricConfig   json.RawMessage                 `json:"customMetricConfig,omitempty"`
}

// HumanEvaluationCustomMetric mirrors types.HumanEvaluationCustomMetric
// (bedrock@v1.66.4 types/types.go:4786-4805).
type HumanEvaluationCustomMetric struct {
	Name         string `json:"name,omitempty"`
	RatingMethod string `json:"ratingMethod,omitempty"`
	Description  string `json:"description,omitempty"`
}

// HumanWorkflowConfig mirrors types.HumanWorkflowConfig (bedrock@v1.66.4
// types/types.go:4809-4820).
type HumanWorkflowConfig struct {
	FlowDefinitionArn string `json:"flowDefinitionArn,omitempty"`
	Instructions      string `json:"instructions,omitempty"`
}

// HumanEvaluationConfig mirrors types.HumanEvaluationConfig (bedrock@v1.66.4
// types/types.go:4767-4782).
type HumanEvaluationConfig struct {
	HumanWorkflowConfig  *HumanWorkflowConfig            `json:"humanWorkflowConfig,omitempty"`
	DatasetMetricConfigs []EvaluationDatasetMetricConfig `json:"datasetMetricConfigs,omitempty"`
	CustomMetrics        []HumanEvaluationCustomMetric   `json:"customMetrics,omitempty"`
}

// EvaluationConfig models bedrock's evaluationConfig union
// (types.EvaluationConfig, bedrock@v1.66.4 types/types.go:2844-2852).
// smithy-go serializes/deserializes it as a single-key object naming the
// variant ("automated" or "human", serializers.go:10697-10719,
// deserializers.go:27131-27179); encoding/json's field-presence dispatch on
// the struct tags mirrors that mechanism directly rather than flattening the
// two variants into one shape.
type EvaluationConfig struct {
	Automated *AutomatedEvaluationConfig `json:"automated,omitempty"`
	Human     *HumanEvaluationConfig     `json:"human,omitempty"`
}

// JobType reports the real AWS EvaluationJobType ("Automated" or "Human",
// bedrock@v1.66.4 types/enums.go:548-551) implied by which EvaluationConfig
// variant was set on Create -- the same signal GetEvaluationJobOutput.JobType
// (api_op_GetEvaluationJob.go:72) is required to carry.
func (c *EvaluationConfig) JobType() string {
	switch {
	case c == nil:
		return ""
	case c.Automated != nil:
		return "Automated"
	case c.Human != nil:
		return "Human"
	default:
		return ""
	}
}

// RAGConfig models bedrock's ragConfigs union member (types.RAGConfig,
// bedrock@v1.66.4 types/types.go:5979-5996): a single-key object naming
// "knowledgeBaseConfig" or "precomputedRagSourceConfig". Both variants are
// themselves further unions bottoming out in a recursive ~12-variant
// RetrievalFilter tree (types/types.go:6165-6344) that gopherstack's inert
// evaluation-job store never interprets; stored verbatim as received rather
// than partially modeled, same rationale as AutomatedEvaluationConfig.
// CustomMetricConfig above.
type RAGConfig struct {
	KnowledgeBaseConfig        json.RawMessage `json:"knowledgeBaseConfig,omitempty"`
	PrecomputedRagSourceConfig json.RawMessage `json:"precomputedRagSourceConfig,omitempty"`
}

// EvaluationInferenceConfig models bedrock's inferenceConfig union
// (types.EvaluationInferenceConfig, bedrock@v1.66.4 types/types.go:2953-2966).
// smithy-go serializes/deserializes it as a single-key object naming the
// variant ("models" or "ragConfigs", serializers.go:10795-10817,
// deserializers.go:27352-27391).
type EvaluationInferenceConfig struct {
	Models     []EvaluationModelConfig `json:"models,omitempty"`
	RagConfigs []RAGConfig             `json:"ragConfigs,omitempty"`
}

// EvaluationJob represents a model evaluation job.
type EvaluationJob struct {
	CreationTime     time.Time                  `json:"creationTime"`
	LastModifiedTime time.Time                  `json:"lastModifiedTime"`
	InferenceConfig  *EvaluationInferenceConfig `json:"inferenceConfig,omitempty"`
	EvaluationConfig *EvaluationConfig          `json:"evaluationConfig,omitempty"`
	ApplicationType  string                     `json:"applicationType,omitempty"`
	JobName          string                     `json:"jobName"`
	JobDescription   string                     `json:"jobDescription,omitempty"`
	RoleArn          string                     `json:"roleArn,omitempty"`
	Status           string                     `json:"status"`
	JobArn           string                     `json:"jobArn"`
	OutputDataConfig OutputDataConfig           `json:"outputDataConfig"`
	Tags             []Tag                      `json:"tags,omitempty"`
}

// AutomatedReasoningPolicy represents an Automated Reasoning policy.
type AutomatedReasoningPolicy struct {
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	PolicyArn string    `json:"policyArn"`
	Name      string    `json:"name"`
	// PolicyDefinition is stored verbatim from UpdateAutomatedReasoningPolicy's
	// required policyDefinition body member (bedrock@v1.66.4
	// api_op_UpdateAutomatedReasoningPolicy.go:37-63). Kept as raw JSON rather
	// than the real nested rules/types/variables union -- inert, never
	// interpreted, but genuinely the caller's own content, not fabricated.
	PolicyDefinition json.RawMessage `json:"policyDefinition,omitempty"`
	Description      string          `json:"description,omitempty"`
	Status           string          `json:"status"`
	DefinitionHash   string          `json:"definitionHash,omitempty"`
	Version          string          `json:"version,omitempty"`
	Tags             []Tag           `json:"tags,omitempty"`
}

// AutomatedReasoningPolicyBuildWorkflow represents a build workflow for a policy.
type AutomatedReasoningPolicyBuildWorkflow struct {
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
	BuildWorkflowID string    `json:"buildWorkflowId"`
	PolicyArn       string    `json:"policyArn"`
	Status          string    `json:"status"`
	// BuildWorkflowType and SourceContent come from
	// StartAutomatedReasoningPolicyBuildWorkflow's real path/body
	// (bedrock@v1.66.4 serializers.go:8008: buildWorkflowType is a path
	// label, sourceContent is the entire JSON payload). SourceContent is
	// stored verbatim and never interpreted -- same rationale as
	// AutomatedReasoningPolicy.PolicyDefinition above.
	BuildWorkflowType string          `json:"buildWorkflowType,omitempty"`
	SourceContent     json.RawMessage `json:"sourceContent,omitempty"`
}

// AutomatedReasoningPolicyTestCase represents a test case for a policy.
type AutomatedReasoningPolicyTestCase struct {
	CreatedAt                        time.Time `json:"createdAt"`
	UpdatedAt                        time.Time `json:"updatedAt"`
	ConfidenceThreshold              *float64  `json:"confidenceThreshold,omitempty"`
	TestCaseID                       string    `json:"testCaseId"`
	PolicyArn                        string    `json:"policyArn"`
	GuardContent                     string    `json:"guardContent,omitempty"`
	QueryContent                     string    `json:"queryContent,omitempty"`
	ExpectedAggregatedFindingsResult string    `json:"expectedAggregatedFindingsResult,omitempty"`
}

// AutomatedReasoningPolicyVersion represents a version of a policy.
type AutomatedReasoningPolicyVersion struct {
	CreatedAt      time.Time `json:"createdAt"`
	PolicyArn      string    `json:"policyArn"`
	Name           string    `json:"name"`
	DefinitionHash string    `json:"definitionHash"`
	Version        string    `json:"version"`
	Tags           []Tag     `json:"tags,omitempty"`
}

// CustomModel represents a custom model, either imported via CreateCustomModel
// (BaseModelArn empty: the wire input never supplies a base model, and
// gopherstack does not process model artifacts to derive one) or produced by
// a completed CreateModelCustomizationJob (BaseModelArn/BaseModelName/JobArn/
// JobName populated from the job).
type CustomModel struct {
	CreationTime      time.Time `json:"creationTime"`
	ModelArn          string    `json:"modelArn"`
	ModelName         string    `json:"modelName"`
	ModelStatus       string    `json:"modelStatus"`
	BaseModelArn      string    `json:"baseModelArn,omitempty"`
	BaseModelName     string    `json:"baseModelName,omitempty"`
	CustomizationType string    `json:"customizationType,omitempty"`
	JobArn            string    `json:"jobArn,omitempty"`
	JobName           string    `json:"jobName,omitempty"`
	Tags              []Tag     `json:"tags,omitempty"`
}

// CustomModelDeployment represents a custom model deployment.
type CustomModelDeployment struct {
	CreationTime             time.Time `json:"creationTime"`
	LastModifiedTime         time.Time `json:"lastModifiedTime"`
	CustomModelDeploymentArn string    `json:"customModelDeploymentArn"`
	ModelDeploymentName      string    `json:"modelDeploymentName"`
	ModelArn                 string    `json:"modelArn"`
	Status                   string    `json:"status"`
	Tags                     []Tag     `json:"tags,omitempty"`
}

// FoundationModelAgreement represents an agreement for foundation model access.
type FoundationModelAgreement struct {
	ModelID string `json:"modelId"`
}

// FoundationModelAgreementOffer represents a single agreement offer for a
// foundation model, as returned by ListFoundationModelAgreementOffers. Mirrors
// (a lean subset of) types.Offer/types.TermDetails in aws-sdk-go-v2/service/bedrock.
type FoundationModelAgreementOffer struct {
	OfferToken   string
	OfferID      string
	LegalTermURL string
}

// GuardrailVersion represents a numbered, immutable snapshot of a guardrail taken at the
// time CreateGuardrailVersion was called. AWS freezes the guardrail's full configuration
// into each numbered version, so GetGuardrail(id, version) must be able to serve this
// snapshot independently of the (still-editable) DRAFT.
type GuardrailVersion struct {
	CreatedAt               time.Time          `json:"createdAt"`
	Policies                *GuardrailPolicies `json:"policies,omitempty"`
	GuardrailID             string             `json:"guardrailId"`
	GuardrailArn            string             `json:"guardrailArn"`
	Version                 string             `json:"version"`
	Name                    string             `json:"name"`
	Description             string             `json:"description,omitempty"`
	BlockedInputMessaging   string             `json:"blockedInputMessaging,omitempty"`
	BlockedOutputsMessaging string             `json:"blockedOutputsMessaging,omitempty"`
	Tags                    []Tag              `json:"tags,omitempty"`
}

// ModelCopyJob represents a model copy job.
type ModelCopyJob struct {
	CreationTime     time.Time `json:"creationTime"`
	LastModifiedTime time.Time `json:"lastModifiedTime"`
	JobArn           string    `json:"jobArn"`
	SourceModelArn   string    `json:"sourceModelArn"`
	TargetModelArn   string    `json:"targetModelArn"`
	// TargetModelName is the caller's real input (CreateModelCopyJobInput's
	// required targetModelName, bedrock@v1.66.4 serializers.go:1720-1750) --
	// not surfaced by GetModelCopyJobOutput itself, but kept as real backing
	// state rather than discarded now that TargetModelArn is built from it.
	TargetModelName string `json:"targetModelName,omitempty"`
	Status          string `json:"status"`
	FailureMessage  string `json:"failureMessage,omitempty"`
	Tags            []Tag  `json:"tags,omitempty"`
}

// ModelImportJob represents a model import job.
type ModelImportJob struct {
	CreationTime      time.Time  `json:"creationTime"`
	LastModifiedTime  time.Time  `json:"lastModifiedTime"`
	EndTime           *time.Time `json:"endTime,omitempty"`
	JobArn            string     `json:"jobArn"`
	JobName           string     `json:"jobName"`
	ImportedModelArn  string     `json:"importedModelArn"`
	ImportedModelName string     `json:"importedModelName"`
	RoleArn           string     `json:"roleArn"`
	ModelDataSourceS3 string     `json:"modelDataSourceS3,omitempty"`
	Status            string     `json:"status"`
	Tags              []Tag      `json:"tags,omitempty"`
}

// OutputDataConfig mirrors bedrock@v1.66.4 types.OutputDataConfig
// (api_op_CreateModelCustomizationJob.go), the S3 location a completed job
// writes its output to.
type OutputDataConfig struct {
	S3Uri string `json:"s3Uri"`
}

// TrainingDataConfig mirrors bedrock@v1.66.4 types.TrainingDataConfig
// (api_op_CreateModelCustomizationJob.go). InvocationLogSource is flattened
// to InvocationLogSourceS3Uri, the same way ModelImportJob.ModelDataSourceS3
// flattens ModelDataSource above -- it is the union's only member
// (types.InvocationLogSourceMemberS3Uri). RequestMetadataFilters is not
// modeled: a recursive filter-expression union (AndAll/OrAll/Equals/NotEquals)
// that only prunes which invocation logs a Distillation job trains on, and
// this backend has no invocation-log pipeline for such filters to act on.
type TrainingDataConfig struct {
	S3Uri                    string `json:"s3Uri,omitempty"`
	InvocationLogSourceS3Uri string `json:"invocationLogSourceS3Uri,omitempty"`
	UsePromptResponse        bool   `json:"usePromptResponse,omitempty"`
}

// ModelCustomizationJob represents a model customization job. BaseModelName is
// the display name of the foundation model resolved from BaseModelArn (best
// effort: only populated when the base model identifier matches a seeded
// foundation model), carried here so the CustomModel materialized on
// completion (see AdvanceCustomizationJobStatuses) can populate
// CustomModelSummary's required baseModelName without a second lookup.
type ModelCustomizationJob struct {
	CreationTime       time.Time          `json:"creationTime"`
	LastModifiedTime   time.Time          `json:"lastModifiedTime"`
	EndTime            time.Time          `json:"endTime"`
	JobArn             string             `json:"jobArn"`
	JobName            string             `json:"jobName"`
	BaseModelArn       string             `json:"baseModelArn"`
	BaseModelName      string             `json:"baseModelName,omitempty"`
	OutputModelArn     string             `json:"outputModelArn"`
	CustomModelName    string             `json:"customModelName"`
	Status             string             `json:"status"`
	CustomizationType  string             `json:"customizationType,omitempty"`
	RoleArn            string             `json:"roleArn"`
	OutputDataConfig   OutputDataConfig   `json:"outputDataConfig"`
	TrainingDataConfig TrainingDataConfig `json:"trainingDataConfig"`
	// ValidatorS3Uris holds Validator.S3Uri from the optional
	// CreateModelCustomizationJobInput.ValidationDataConfig (bedrock@v1.66.4
	// serializers.go:13249); GetModelCustomizationJobOutput.ValidationDataConfig
	// is required regardless, so it's always emitted, empty or not.
	ValidatorS3Uris []string `json:"validatorS3Uris,omitempty"`
	Tags            []Tag    `json:"tags,omitempty"`
}

// InferenceProfile represents an inference profile resource. ModelSource is
// the CopyFrom ARN from types.InferenceProfileModelSource
// (api_op_CreateInferenceProfile.go), the union's only member -- the
// foundation model or system-defined inference profile this profile tracks.
// GetInferenceProfileOutput echoes it back as the required Models list
// (types.InferenceProfileModel), not as ModelSource itself; see
// inferenceProfileToOutput.
type InferenceProfile struct {
	CreatedAt            time.Time `json:"createdAt"`
	UpdatedAt            time.Time `json:"updatedAt"`
	InferenceProfileArn  string    `json:"inferenceProfileArn"`
	InferenceProfileID   string    `json:"inferenceProfileId"`
	InferenceProfileName string    `json:"inferenceProfileName"`
	Status               string    `json:"status"`
	Type                 string    `json:"type"`
	Description          string    `json:"description,omitempty"`
	ModelSource          string    `json:"modelSource"`
	Tags                 []Tag     `json:"tags,omitempty"`
}

// MarketplaceModelEndpoint represents a marketplace model endpoint.
type MarketplaceModelEndpoint struct {
	CreatedAt      time.Time                `json:"createdAt"`
	UpdatedAt      time.Time                `json:"updatedAt"`
	EndpointConfig *SageMakerEndpointConfig `json:"endpointConfig,omitempty"`
	EndpointArn    string                   `json:"endpointArn"`
	EndpointName   string                   `json:"endpointName"`
	ModelSourceID  string                   `json:"modelSourceIdentifier"`
	Status         string                   `json:"status"`
	Tags           []Tag                    `json:"tags,omitempty"`
}

// SageMakerEndpointConfig mirrors types.SageMakerEndpoint in aws-sdk-go-v2/service/bedrock
// (the sole real member of the EndpointConfig union today). Real AWS wire shape:
// {"sageMaker": {"executionRole":,"initialInstanceCount":,"instanceType":,"kmsEncryptionKey":}}.
type SageMakerEndpointConfig struct {
	ExecutionRole        string `json:"executionRole"`
	InstanceType         string `json:"instanceType"`
	KmsEncryptionKey     string `json:"kmsEncryptionKey,omitempty"`
	InitialInstanceCount int32  `json:"initialInstanceCount"`
}

// ModelInvocationLoggingConfiguration mirrors types.LoggingConfig in
// aws-sdk-go-v2/service/bedrock@v1.66.4 (deserializers.go:awsRestjson1_deserializeDocumentLoggingConfig,
// serializers.go:awsRestjson1_serializeDocumentLoggingConfig). All members are optional; the real
// shape has no s3BucketName/loggingEnabled fields at all.
type ModelInvocationLoggingConfiguration struct {
	CloudWatchConfig             *CloudWatchLoggingConfig `json:"cloudWatchConfig,omitempty"`
	S3Config                     *S3LoggingConfig         `json:"s3Config,omitempty"`
	AudioDataDeliveryEnabled     *bool                    `json:"audioDataDeliveryEnabled,omitempty"`
	EmbeddingDataDeliveryEnabled *bool                    `json:"embeddingDataDeliveryEnabled,omitempty"`
	ImageDataDeliveryEnabled     *bool                    `json:"imageDataDeliveryEnabled,omitempty"`
	TextDataDeliveryEnabled      *bool                    `json:"textDataDeliveryEnabled,omitempty"`
	VideoDataDeliveryEnabled     *bool                    `json:"videoDataDeliveryEnabled,omitempty"`
}

// CloudWatchLoggingConfig mirrors types.CloudWatchConfig. LogGroupName/RoleArn are required
// when CloudWatchConfig is present at all.
type CloudWatchLoggingConfig struct {
	LargeDataDeliveryS3Config *S3LoggingConfig `json:"largeDataDeliveryS3Config,omitempty"`
	LogGroupName              string           `json:"logGroupName"`
	RoleArn                   string           `json:"roleArn"`
}

// S3LoggingConfig mirrors types.S3Config. BucketName is required when S3Config is present.
type S3LoggingConfig struct {
	BucketName string `json:"bucketName"`
	KeyPrefix  string `json:"keyPrefix,omitempty"`
}

// CreateEvaluationJobInput holds all parameters for CreateEvaluationJob.
type CreateEvaluationJobInput struct {
	InferenceConfig  *EvaluationInferenceConfig
	EvaluationConfig *EvaluationConfig
	JobName          string
	JobDescription   string
	RoleArn          string
	ApplicationType  string
	OutputDataConfig OutputDataConfig
	Tags             []Tag
}

// BatchDeleteEvaluationJobError describes a single job deletion failure.
type BatchDeleteEvaluationJobError struct {
	JobARN  string `json:"jobIdentifier"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// BatchDeleteEvaluationJobItem describes a successfully scheduled deletion.
type BatchDeleteEvaluationJobItem struct {
	JobARN string `json:"jobIdentifier"`
	Status string `json:"jobStatus"`
}

// ModelInvocationJob represents a batch model invocation job.
type ModelInvocationJob struct {
	LastModifiedTime time.Time      `json:"lastModifiedTime"`
	CreationTime     time.Time      `json:"creationTime"`
	InputDataConfig  map[string]any `json:"inputDataConfig,omitempty"`
	EndTime          *time.Time     `json:"endTime,omitempty"`
	OutputDataConfig map[string]any `json:"outputDataConfig,omitempty"`
	JobArn           string         `json:"jobArn"`
	ModelID          string         `json:"modelId,omitempty"`
	Status           string         `json:"status"`
	RoleArn          string         `json:"roleArn,omitempty"`
	JobName          string         `json:"jobName"`
	FailureMessage   string         `json:"failureMessage,omitempty"`
	ClientToken      string         `json:"clientRequestToken,omitempty"`
	Tags             []Tag          `json:"tags,omitempty"`
}

// CreateModelInvocationJobInput holds the full set of fields for CreateModelInvocationJob.
type CreateModelInvocationJobInput struct {
	RoleArn          string         `json:"roleArn"`
	ModelID          string         `json:"modelId"`
	InputDataConfig  map[string]any `json:"inputDataConfig,omitempty"`
	OutputDataConfig map[string]any `json:"outputDataConfig,omitempty"`
	ClientToken      string         `json:"clientRequestToken,omitempty"`
}

// PromptRouter represents a prompt router resource.
type PromptRouter struct {
	CreatedAt                  time.Time `json:"createdAt"`
	UpdatedAt                  time.Time `json:"updatedAt"`
	PromptRouterArn            string    `json:"promptRouterArn"`
	PromptRouterName           string    `json:"promptRouterName"`
	Status                     string    `json:"status"`
	Type                       string    `json:"type"`
	Description                string    `json:"description,omitempty"`
	FallbackModelArn           string    `json:"fallbackModelArn"`
	ModelArns                  []string  `json:"modelArns"`
	Tags                       []Tag     `json:"tags,omitempty"`
	RoutingResponseQualityDiff float64   `json:"routingResponseQualityDiff"`
}

// AccountEnforcedGuardrailConfig represents an account-level enforced guardrail
// configuration (real AWS ops: PutEnforcedGuardrailConfiguration /
// ListEnforcedGuardrailsConfiguration / DeleteEnforcedGuardrailConfiguration).
// ConfigID-keyed and wraps a GuardrailIdentifier+GuardrailVersion reference plus
// an InputTags honor/ignore flag and an optional model-scoped enforcement list,
// matching types.AccountEnforcedGuardrailOutputConfiguration in aws-sdk-go-v2.
type AccountEnforcedGuardrailConfig struct {
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
	ConfigID         string    `json:"configId"`
	GuardrailID      string    `json:"guardrailId"`
	GuardrailArn     string    `json:"guardrailArn"`
	GuardrailVersion string    `json:"guardrailVersion"`
	InputTags        string    `json:"inputTags"`
	Owner            string    `json:"owner"`
	CreatedBy        string    `json:"createdBy"`
	UpdatedBy        string    `json:"updatedBy"`
	IncludedModels   []string  `json:"includedModels,omitempty"`
	ExcludedModels   []string  `json:"excludedModels,omitempty"`
}

// ListModelInvocationJobsInput holds filter/pagination params for ListModelInvocationJobs.
type ListModelInvocationJobsInput struct {
	StatusEquals     string
	NameContains     string
	SubmitTimeAfter  *time.Time
	SubmitTimeBefore *time.Time
	SortBy           string // CreationTime (default)
	SortOrder        string // Ascending (default) | Descending
	NextToken        string
	MaxResults       int32
}

// ListEvaluationJobsInput holds filter/pagination params for ListEvaluationJobs.
type ListEvaluationJobsInput struct {
	StatusEquals          string
	ApplicationTypeEquals string
	NameContains          string
	CreationTimeAfter     *time.Time
	CreationTimeBefore    *time.Time
	SortBy                string // CreationTime (default)
	SortOrder             string // Ascending (default) | Descending
	NextToken             string
	MaxResults            int32
}

// ListModelCustomizationJobsInput holds filter/pagination params for
// ListModelCustomizationJobs.
type ListModelCustomizationJobsInput struct {
	StatusEquals       string
	NameContains       string
	CreationTimeAfter  *time.Time
	CreationTimeBefore *time.Time
	SortBy             string // CreationTime (default)
	SortOrder          string // Ascending (default) | Descending
	NextToken          string
	MaxResults         int32
}

// ListCustomModelsInput holds filter/pagination params for ListCustomModels.
type ListCustomModelsInput struct {
	CreationTimeAfter        *time.Time
	CreationTimeBefore       *time.Time
	IsOwned                  *bool
	ModelStatus              string
	NameContains             string
	BaseModelArnEquals       string
	FoundationModelArnEquals string
	SortBy                   string
	SortOrder                string
	NextToken                string
	MaxResults               int32
}

// ListModelCopyJobsInput holds filter/pagination params for ListModelCopyJobs
// (bedrock@v1.66.4 api_op_ListModelCopyJobs.go).
type ListModelCopyJobsInput struct {
	CreationTimeAfter       *time.Time
	CreationTimeBefore      *time.Time
	StatusEquals            string
	SourceAccountEquals     string
	SourceModelArnEquals    string
	TargetModelNameContains string
	SortBy                  string
	SortOrder               string
	NextToken               string
	MaxResults              int32
}

// ListModelImportJobsInput holds filter/pagination params for
// ListModelImportJobs (bedrock@v1.66.4 api_op_ListModelImportJobs.go).
type ListModelImportJobsInput struct {
	CreationTimeAfter  *time.Time
	CreationTimeBefore *time.Time
	StatusEquals       string
	NameContains       string
	SortBy             string
	SortOrder          string
	NextToken          string
	MaxResults         int32
}

// ListImportedModelsInput holds filter/pagination params for
// ListImportedModels (bedrock@v1.66.4 api_op_ListImportedModels.go). Real
// SortBy/SortOrder not modeled here: gopherstack has always sorted
// CreationTime ascending regardless of client input; out of scope for
// gopherstack-kkfs's MaxResults fix.
type ListImportedModelsInput struct {
	CreationTimeAfter  *time.Time
	CreationTimeBefore *time.Time
	NameContains       string
	NextToken          string
	MaxResults         int32
}

// ListCustomModelDeploymentsInput holds filter/pagination params for
// ListCustomModelDeployments (bedrock@v1.66.4 api_op_ListCustomModelDeployments.go).
type ListCustomModelDeploymentsInput struct {
	CreatedAfter   *time.Time
	CreatedBefore  *time.Time
	StatusEquals   string
	ModelArnEquals string
	NameContains   string
	SortBy         string
	SortOrder      string
	NextToken      string
	MaxResults     int32
}

// ListProvisionedModelThroughputsInput holds filter/pagination params for
// ListProvisionedModelThroughputs (bedrock@v1.66.4
// api_op_ListProvisionedModelThroughputs.go).
type ListProvisionedModelThroughputsInput struct {
	CreationTimeAfter  *time.Time
	CreationTimeBefore *time.Time
	StatusEquals       string
	ModelArnEquals     string
	NameContains       string
	SortBy             string
	SortOrder          string
	NextToken          string
	MaxResults         int32
}

// Agent represents an Amazon Bedrock Agent.
type Agent struct {
	CreatedAt              time.Time `json:"createdAt"`
	UpdatedAt              time.Time `json:"updatedAt"`
	preparationDueAt       time.Time
	Tags                   map[string]string `json:"tags,omitempty"`
	GuardrailConfiguration map[string]any    `json:"guardrailConfiguration,omitempty"`
	MemoryConfiguration    map[string]any    `json:"memoryConfiguration,omitempty"`
	AgentStatus            string            `json:"agentStatus"`
	AgentName              string            `json:"agentName"`
	AgentArn               string            `json:"agentArn"`
	AgentVersion           string            `json:"agentVersion"`
	AgentCollaboration     string            `json:"agentCollaboration,omitempty"`
	Description            string            `json:"description,omitempty"`
	FoundationModel        string            `json:"foundationModel,omitempty"`
	Instruction            string            `json:"instruction,omitempty"`
	RoleArn                string            `json:"agentResourceRoleArn,omitempty"`
	AgentID                string            `json:"agentId"`
	FailureReasons         []string          `json:"failureReasons,omitempty"`
}

// AgentActionGroup represents an action group for a Bedrock Agent.
type AgentActionGroup struct {
	CreatedAt           time.Time      `json:"createdAt"`
	UpdatedAt           time.Time      `json:"updatedAt"`
	ActionGroupExecutor map[string]any `json:"actionGroupExecutor,omitempty"`
	APISchema           map[string]any `json:"apiSchema,omitempty"`
	FunctionSchema      map[string]any `json:"functionSchema,omitempty"`
	ActionGroupID       string         `json:"actionGroupId"`
	ActionGroupName     string         `json:"actionGroupName"`
	AgentID             string         `json:"agentId"`
	AgentVersion        string         `json:"agentVersion"`
	ActionGroupState    string         `json:"actionGroupState"`
	Description         string         `json:"description,omitempty"`
}

// AgentAlias represents an alias for a Bedrock Agent.
type AgentAlias struct {
	CreatedAt               time.Time                `json:"createdAt"`
	UpdatedAt               time.Time                `json:"updatedAt"`
	AgentAliasID            string                   `json:"agentAliasId"`
	AgentAliasArn           string                   `json:"agentAliasArn"`
	AgentAliasName          string                   `json:"agentAliasName"`
	AgentID                 string                   `json:"agentId"`
	AliasStatus             string                   `json:"agentAliasStatus"`
	RoutingConfiguration    []AgentAliasRouting      `json:"routingConfiguration"`
	AgentAliasHistoryEvents []AgentAliasHistoryEvent `json:"agentAliasHistoryEvents,omitempty"`
}

// AgentAliasRouting identifies the version receiving alias traffic.
type AgentAliasRouting struct {
	AgentVersion string `json:"agentVersion"`
}

// AgentAliasHistoryEvent records an alias routing interval.
type AgentAliasHistoryEvent struct {
	StartDate            time.Time           `json:"startDate"`
	EndDate              *time.Time          `json:"endDate,omitempty"`
	RoutingConfiguration []AgentAliasRouting `json:"routingConfiguration"`
}

// AgentKnowledgeBaseAssociation represents an association between agent and knowledge base.
type AgentKnowledgeBaseAssociation struct {
	AgentID         string `json:"agentId"`
	AgentVersion    string `json:"agentVersion"`
	KnowledgeBaseID string `json:"knowledgeBaseId"`
	Description     string `json:"description,omitempty"`
	KBState         string `json:"knowledgeBaseState"`
}

// KnowledgeBase represents an Amazon Bedrock Knowledge Base.
type KnowledgeBase struct {
	CreatedAt                  time.Time         `json:"createdAt"`
	UpdatedAt                  time.Time         `json:"updatedAt"`
	KnowledgeBaseConfiguration map[string]any    `json:"knowledgeBaseConfiguration,omitempty"`
	StorageConfiguration       map[string]any    `json:"storageConfiguration,omitempty"`
	Tags                       map[string]string `json:"tags,omitempty"`
	KnowledgeBaseID            string            `json:"knowledgeBaseId"`
	KnowledgeBaseArn           string            `json:"knowledgeBaseArn"`
	Name                       string            `json:"name"`
	Description                string            `json:"description,omitempty"`
	Status                     string            `json:"status"`
	RoleArn                    string            `json:"roleArn,omitempty"`
}

// DataSource represents a data source for a Knowledge Base.
type DataSource struct {
	CreatedAt               time.Time      `json:"createdAt"`
	UpdatedAt               time.Time      `json:"updatedAt"`
	DataSourceConfiguration map[string]any `json:"dataSourceConfiguration,omitempty"`
	VectorIngestionConfig   map[string]any `json:"vectorIngestionConfiguration,omitempty"`
	DataSourceID            string         `json:"dataSourceId"`
	DataSourceStatus        string         `json:"dataSourceStatus"`
	KnowledgeBaseID         string         `json:"knowledgeBaseId"`
	Name                    string         `json:"name"`
	Description             string         `json:"description,omitempty"`
	DataDeletionPolicy      string         `json:"dataDeletionPolicy,omitempty"`
}

// IngestionJob represents a data source ingestion job.
type IngestionJob struct {
	StartedAt       time.Time `json:"startedAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
	completionDueAt time.Time
	IngestionJobID  string `json:"ingestionJobId"`
	KnowledgeBaseID string `json:"knowledgeBaseId"`
	DataSourceID    string `json:"dataSourceId"`
	Status          string `json:"status"`
	Description     string `json:"description,omitempty"`
}

// AgentConfiguration stores optional settings accepted by agent create and update requests.
type AgentConfiguration struct {
	Tags                   map[string]string
	GuardrailConfiguration map[string]any
	MemoryConfiguration    map[string]any
	AgentName              string
	AgentCollaboration     string
	Description            string
	FoundationModel        string
	Instruction            string
	RoleArn                string
}

// Flow represents an Amazon Bedrock Flow. CreateFlowResponse/GetFlowResponse/
// UpdateFlowResponse have no httpPayload member (botocore bedrock-agent
// 2023-06-05), so id/arn are flat wire keys, not flowId/flowArn.
type Flow struct {
	CreatedAt   time.Time         `json:"createdAt"`
	UpdatedAt   time.Time         `json:"updatedAt"`
	Tags        map[string]string `json:"tags,omitempty"`
	FlowID      string            `json:"id"`
	FlowArn     string            `json:"arn"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Status      string            `json:"status"`
}

// FlowAlias represents an alias for a Bedrock Flow. Its own id/arn are flat
// "id"/"arn"; "flowId" names only the parent flow (see Flow's doc comment).
type FlowAlias struct {
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
	FlowAliasID  string    `json:"id"`
	FlowAliasArn string    `json:"arn"`
	FlowID       string    `json:"flowId"`
	Name         string    `json:"name"`
	Description  string    `json:"description,omitempty"`
}

// FlowVersion represents a snapshot version of a Flow. GetFlowVersionResponse
// has no "flowId" member; the flow's own id/arn ride in "id"/"arn" (see
// Flow's doc comment).
type FlowVersion struct {
	CreatedAt time.Time `json:"createdAt"`
	FlowID    string    `json:"id"`
	FlowArn   string    `json:"arn"`
	Version   string    `json:"version"`
	Status    string    `json:"status"`
}

// Prompt represents an Amazon Bedrock Prompt (see Flow's doc comment for why
// id/arn are flat wire keys).
type Prompt struct {
	CreatedAt   time.Time         `json:"createdAt"`
	UpdatedAt   time.Time         `json:"updatedAt"`
	Tags        map[string]string `json:"tags,omitempty"`
	PromptID    string            `json:"id"`
	PromptArn   string            `json:"arn"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
}

// PromptVersion represents a numbered version of a Prompt.
type PromptVersion struct {
	CreatedAt time.Time `json:"createdAt"`
	PromptID  string    `json:"promptId"`
	Version   string    `json:"version"`
	Name      string    `json:"name,omitempty"`
}

// AgentVersion represents a numbered version of an Agent.
type AgentVersion struct {
	CreatedAt    time.Time `json:"createdAt"`
	AgentID      string    `json:"agentId"`
	AgentVersion string    `json:"agentVersion"`
	AgentStatus  string    `json:"agentStatus"`
}

// AgentCollaborator represents an agent collaboration association.
type AgentCollaborator struct {
	CreatedAt         time.Time `json:"createdAt"`
	CollaboratorID    string    `json:"collaboratorId"`
	AgentID           string    `json:"agentId"`
	AgentVersion      string    `json:"agentVersion"`
	CollaboratorArn   string    `json:"collaboratorArn"`
	RelayConversation string    `json:"relayConversationHistory"`
}

// KnowledgeBaseDocument represents a document in a KB data source.
type KnowledgeBaseDocument struct {
	KnowledgeBaseID string `json:"knowledgeBaseId"`
	DataSourceID    string `json:"dataSourceId"`
	DocumentID      string `json:"documentId"`
	Status          string `json:"status"`
}

// AdvancedPromptOptimizationInputConfig specifies the S3 location of the
// JSONL input file (prompt templates + evaluation samples) for an advanced
// prompt optimization job. Matches
// types.AdvancedPromptOptimizationInputConfig.
type AdvancedPromptOptimizationInputConfig struct {
	S3URI string `json:"s3Uri"`
}

// AdvancedPromptOptimizationOutputConfig specifies the S3 location prefix
// where an advanced prompt optimization job writes its results. Matches
// types.AdvancedPromptOptimizationOutputConfig.
type AdvancedPromptOptimizationOutputConfig struct {
	S3URI string `json:"s3Uri"`
}

// InferenceConfiguration holds inference parameters (maxTokens, temperature,
// topP, stopSequences) for a model used in an advanced prompt optimization
// job. Matches types.InferenceConfiguration.
type InferenceConfiguration struct {
	MaxTokens     *int32   `json:"maxTokens,omitempty"`
	Temperature   *float32 `json:"temperature,omitempty"`
	TopP          *float32 `json:"topP,omitempty"`
	StopSequences []string `json:"stopSequences,omitempty"`
}

// ModelConfiguration specifies a target model and its inference parameters
// for an advanced prompt optimization job. Matches types.ModelConfiguration.
type ModelConfiguration struct {
	InferenceConfig              *InferenceConfiguration `json:"inferenceConfig,omitempty"`
	AdditionalModelRequestFields map[string]any          `json:"additionalModelRequestFields,omitempty"`
	ModelID                      string                  `json:"modelId"`
}

// AdvancedPromptOptimizationJob represents a Bedrock advanced prompt
// optimization job.
//
// Real AWS's GetAdvancedPromptOptimizationJobOutput does NOT return the
// optimized prompt text anywhere in its wire shape -- optimization results
// are written by the real service to the caller-supplied
// OutputConfig.S3Uri location, entirely outside the API response. This
// backend therefore never fabricates an "optimized prompt" result (doing so
// would require actual model inference this backend does not perform); it
// models only the job's real lifecycle/status fields, which is everything
// the real wire shape actually carries. See handler_advanced_prompt_optimization_jobs.go.
type AdvancedPromptOptimizationJob struct {
	CreationTime        time.Time
	LastModifiedTime    time.Time
	JobArn              string
	JobName             string
	JobDescription      string
	JobStatus           string
	EncryptionKeyArn    string
	FailureMessage      string
	InputConfig         AdvancedPromptOptimizationInputConfig
	OutputConfig        AdvancedPromptOptimizationOutputConfig
	ModelConfigurations []ModelConfiguration
	Tags                []Tag
}

// CreateAdvancedPromptOptimizationJobInput holds the full set of fields for
// CreateAdvancedPromptOptimizationJob.
type CreateAdvancedPromptOptimizationJobInput struct {
	JobName             string
	JobDescription      string
	EncryptionKeyArn    string
	InputConfig         AdvancedPromptOptimizationInputConfig
	OutputConfig        AdvancedPromptOptimizationOutputConfig
	ModelConfigurations []ModelConfiguration
	Tags                []Tag
}

// ListAdvancedPromptOptimizationJobsInput holds filter/sort/pagination params
// for ListAdvancedPromptOptimizationJobs. Real AWS has no name/status filter
// for this op (unlike ListEvaluationJobs/ListModelInvocationJobs) -- only
// sortBy/sortOrder/maxResults/nextToken.
type ListAdvancedPromptOptimizationJobsInput struct {
	SortBy     string // CreationTime (only supported value)
	SortOrder  string // Ascending (default) | Descending
	NextToken  string
	MaxResults int32
}

// BatchDeleteAdvancedPromptOptimizationJobError describes a single job
// deletion failure. Matches types.BatchDeleteAdvancedPromptOptimizationJobError.
type BatchDeleteAdvancedPromptOptimizationJobError struct {
	JobIdentifier string `json:"jobIdentifier"`
	Code          string `json:"code"`
	Message       string `json:"message,omitempty"`
}

// BatchDeleteAdvancedPromptOptimizationJobItem describes a successfully
// deleted job. Matches types.BatchDeleteAdvancedPromptOptimizationJobItem.
type BatchDeleteAdvancedPromptOptimizationJobItem struct {
	JobIdentifier string `json:"jobIdentifier"`
	JobStatus     string `json:"jobStatus"`
}

// AccountDataRetention represents the account-wide Bedrock data retention
// setting (GetAccountDataRetention / PutAccountDataRetention).
type AccountDataRetention struct {
	UpdatedAt time.Time
	Mode      string
}

// ResourcePolicy represents a resource-based policy attached to a Bedrock
// resource (core bedrock's PutResourcePolicy/GetResourcePolicy/
// DeleteResourcePolicy) or, in the bedrock-agent domain, to a knowledge base
// (bedrock-agent's PutResourcePolicy/GetResourcePolicy/DeleteResourcePolicy).
// RevisionID is only meaningful for the bedrock-agent flavor, which supports
// optimistic-concurrency updates via expectedRevisionId; core bedrock's
// ResourcePolicy shape has no revision concept and simply ignores this field.
type ResourcePolicy struct {
	CreatedAt      time.Time
	UpdatedAt      time.Time
	ResourceArn    string
	PolicyDocument string
	RevisionID     string
}

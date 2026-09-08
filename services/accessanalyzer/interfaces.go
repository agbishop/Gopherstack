package accessanalyzer

import (
	"context"
	"encoding/json"
)

// StorageBackend defines the interface for Access Analyzer backend implementations.
// All mutating methods must be safe for concurrent use.
type StorageBackend interface {
	// Analyzer operations. configuration is the optional raw
	// AnalyzerConfiguration union wire body -- see Analyzer.Configuration.
	CreateAnalyzer(
		name string, analyzerType AnalyzerType, tags map[string]string, configuration ...json.RawMessage,
	) (*Analyzer, error)
	GetAnalyzer(name string) (*Analyzer, error)
	ListAnalyzers(analyzerType string) ([]*Analyzer, error)
	DeleteAnalyzer(name string) error
	UpdateAnalyzer(name string, configuration ...json.RawMessage) (*Analyzer, error)
	CreateServiceLinkedAnalyzer(analyzerType AnalyzerType, configuration ...json.RawMessage) (*Analyzer, error)

	// Archive rule operations
	CreateArchiveRule(analyzerName, ruleName string, filter map[string]FilterCriterion) (*ArchiveRule, error)
	GetArchiveRule(analyzerName, ruleName string) (*ArchiveRule, error)
	ListArchiveRules(analyzerName string) ([]*ArchiveRule, error)
	DeleteArchiveRule(analyzerName, ruleName string) error
	UpdateArchiveRule(analyzerName, ruleName string, filter map[string]FilterCriterion) (*ArchiveRule, error)
	ApplyArchiveRule(analyzerArn, ruleName string) error

	// Finding operations
	AddFinding(
		analyzerName, resourceType, resourceArn string,
		action []string,
		principal map[string]string,
		isPublic *bool,
	) (*Finding, error)
	GetFinding(analyzerName, findingID string) (*Finding, error)
	ListFindings(
		analyzerName string,
		filter map[string]FilterCriterion,
		status string,
		sortCrit *FindingSortCriteria,
		maxResults int,
		nextToken string,
	) ([]*Finding, string, error)
	// UpdateFindings updates findings selected by findingIDs when non-empty,
	// otherwise by resourceArn (UpdateFindingsInput.ResourceArn,
	// serializers.go:3312-3315, an independent optional wire field).
	UpdateFindings(analyzerName string, findingIDs []string, resourceArn string, status FindingStatus) error
	GetFindingV2(analyzerArn, findingID string) (*Finding, error)
	ListFindingsV2(
		analyzerArn, status string,
		filter map[string]FilterCriterion,
		sortCrit *FindingSortCriteria,
		maxResults int,
		nextToken string,
	) ([]*Finding, string, error)
	GetFindingsStatistics(analyzerArn string) (map[string]int, error)

	// Finding recommendations
	GenerateFindingRecommendation(analyzerArn, findingID string) error
	GetFindingRecommendation(analyzerArn, findingID string) (*FindingRecommendation, error)

	// Analyzed resources
	AddAnalyzedResource(analyzerArn, resourceArn, resourceType string, isPublic bool) (*AnalyzedResource, error)
	GetAnalyzedResource(analyzerArn, resourceArn string) (*AnalyzedResource, error)
	ListAnalyzedResources(
		analyzerArn, resourceType string,
		maxResults int,
		nextToken string,
	) ([]*AnalyzedResource, string, error)

	// Scan
	StartResourceScan(analyzerARN, resourceARN string) error

	// Policy generation
	StartPolicyGeneration(
		principalArn string, cloudTrailDetails *PolicyGenerationCloudTrailDetails,
	) (*PolicyGeneration, error)
	GetPolicyGeneration(jobID string) (*PolicyGeneration, error)
	CancelPolicyGeneration(jobID string) error
	ListPolicyGenerations(principalArn string, maxResults int, nextToken string) ([]*PolicyGeneration, string, error)

	// Access previews
	CreateAccessPreview(analyzerArn string, configurations map[string]json.RawMessage) (*AccessPreview, error)
	GetAccessPreview(accessPreviewID string) (*AccessPreview, error)
	ListAccessPreviews(analyzerArn string) ([]*AccessPreview, error)
	ListAccessPreviewFindings(
		accessPreviewID string,
		filter map[string]FilterCriterion,
		maxResults int,
		nextToken string,
	) ([]*Finding, string, error)

	// Tag operations
	TagResource(resourceARN string, kv map[string]string) error
	UntagResource(resourceARN string, tagKeys []string) error
	ListTagsForResource(resourceARN string) (map[string]string, error)

	// Lifecycle
	Reset()
	Region() string
	AccountID() string
	Snapshot(ctx context.Context) []byte
	Restore(ctx context.Context, data []byte) error
}

// compile-time assertion.
var _ StorageBackend = (*InMemoryBackend)(nil)

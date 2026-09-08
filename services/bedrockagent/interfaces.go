package bedrockagent

import "context"

// StorageBackend defines all persistence operations for the Bedrock Agent service.
type StorageBackend interface {
	// Snapshot and Restore implement persistence.Persistable. Handler
	// delegates to them (see persistence.go) so cli.go's generic
	// setupPersistence picks BedrockAgent up. Before Phase 3.3, BedrockAgent
	// had no persistence at all: neither Handler nor InMemoryBackend
	// implemented Snapshot/Restore, so state never survived a restart. This
	// is the dead-wiring fix in scope for the Phase 3.3 datalayer
	// conversion, built from scratch following the codecommit/cleanrooms
	// pattern (see persistence.go).
	Snapshot(ctx context.Context) []byte
	Restore(ctx context.Context, data []byte) error

	// Agent operations.
	CreateAgent(ctx context.Context, cfg AgentConfig) (*Agent, error)
	GetAgent(ctx context.Context, agentID string) (*Agent, error)
	UpdateAgent(ctx context.Context, agentID string, cfg AgentConfig) (*Agent, error)
	DeleteAgent(ctx context.Context, agentID string) error
	ListAgents(ctx context.Context, maxResults int, nextToken string) ([]*AgentSummary, string, error)
	PrepareAgent(ctx context.Context, agentID string) (*Agent, error)

	// Agent version operations.
	CreateAgentVersion(ctx context.Context, agentID, description string) (*AgentVersion, error)
	GetAgentVersion(ctx context.Context, agentID, agentVersion string) (*AgentVersion, error)
	DeleteAgentVersion(ctx context.Context, agentID, agentVersion string, skipResourceInUseCheck bool) error
	ListAgentVersions(
		ctx context.Context, agentID string, maxResults int, nextToken string,
	) ([]*AgentVersionSummary, string, error)

	// Agent action group operations.
	CreateAgentActionGroup(
		ctx context.Context, agentID, agentVersion string, cfg ActionGroupConfig,
	) (*AgentActionGroup, error)
	GetAgentActionGroup(
		ctx context.Context, agentID, agentVersion, actionGroupID string,
	) (*AgentActionGroup, error)
	UpdateAgentActionGroup(
		ctx context.Context, agentID, agentVersion, actionGroupID string, cfg ActionGroupConfig,
	) (*AgentActionGroup, error)
	DeleteAgentActionGroup(
		ctx context.Context, agentID, agentVersion, actionGroupID string,
	) error
	ListAgentActionGroups(
		ctx context.Context, agentID, agentVersion string, maxResults int, nextToken string,
	) ([]*ActionGroupSummary, string, error)

	// Agent alias operations.
	CreateAgentAlias(ctx context.Context, agentID string, cfg AliasConfig) (*AgentAlias, error)
	GetAgentAlias(ctx context.Context, agentID, agentAliasID string) (*AgentAlias, error)
	UpdateAgentAlias(ctx context.Context, agentID, agentAliasID string, cfg AliasConfig) (*AgentAlias, error)
	DeleteAgentAlias(ctx context.Context, agentID, agentAliasID string) error
	ListAgentAliases(
		ctx context.Context, agentID string, maxResults int, nextToken string,
	) ([]*AgentAliasSummary, string, error)

	// Agent collaborator operations.
	AssociateAgentCollaborator(
		ctx context.Context, agentID, agentVersion string, cfg CollaboratorConfig,
	) (*AgentCollaborator, error)
	GetAgentCollaborator(
		ctx context.Context, agentID, agentVersion, collaboratorID string,
	) (*AgentCollaborator, error)
	UpdateAgentCollaborator(
		ctx context.Context, agentID, agentVersion, collaboratorID string, cfg CollaboratorConfig,
	) (*AgentCollaborator, error)
	DisassociateAgentCollaborator(
		ctx context.Context, agentID, agentVersion, collaboratorID string,
	) error
	ListAgentCollaborators(
		ctx context.Context, agentID, agentVersion string, maxResults int, nextToken string,
	) ([]*AgentCollaborator, string, error)

	// Knowledge base operations.
	CreateKnowledgeBase(ctx context.Context, cfg KnowledgeBaseConfig) (*KnowledgeBase, error)
	GetKnowledgeBase(ctx context.Context, kbID string) (*KnowledgeBase, error)
	UpdateKnowledgeBase(ctx context.Context, kbID string, cfg KnowledgeBaseConfig) (*KnowledgeBase, error)
	DeleteKnowledgeBase(ctx context.Context, kbID string) error
	ListKnowledgeBases(ctx context.Context, maxResults int, nextToken string) ([]*KnowledgeBaseSummary, string, error)

	// Agent–knowledge base association operations.
	AssociateAgentKnowledgeBase(
		ctx context.Context, agentID, agentVersion, kbID, description, kbState string,
	) (*AgentKnowledgeBase, error)
	GetAgentKnowledgeBase(
		ctx context.Context, agentID, agentVersion, kbID string,
	) (*AgentKnowledgeBase, error)
	UpdateAgentKnowledgeBase(
		ctx context.Context, agentID, agentVersion, kbID, description, kbState string,
	) (*AgentKnowledgeBase, error)
	DisassociateAgentKnowledgeBase(
		ctx context.Context, agentID, agentVersion, kbID string,
	) error
	ListAgentKnowledgeBases(
		ctx context.Context, agentID, agentVersion string, maxResults int, nextToken string,
	) ([]*AgentKnowledgeBaseSummary, string, error)

	// Data source operations.
	CreateDataSource(ctx context.Context, kbID string, cfg DataSourceConfig) (*DataSource, error)
	GetDataSource(ctx context.Context, kbID, dataSourceID string) (*DataSource, error)
	UpdateDataSource(ctx context.Context, kbID, dataSourceID string, cfg DataSourceConfig) (*DataSource, error)
	DeleteDataSource(ctx context.Context, kbID, dataSourceID string) error
	ListDataSources(
		ctx context.Context, kbID string, maxResults int, nextToken string,
	) ([]*DataSourceSummary, string, error)

	// Ingestion job operations.
	StartIngestionJob(ctx context.Context, kbID, dataSourceID, description string) (*IngestionJob, error)
	GetIngestionJob(ctx context.Context, kbID, dataSourceID, ingestionJobID string) (*IngestionJob, error)
	StopIngestionJob(ctx context.Context, kbID, dataSourceID, ingestionJobID string) (*IngestionJob, error)
	ListIngestionJobs(
		ctx context.Context, kbID, dataSourceID string, filters []IngestionJobFilter, sortBy *IngestionJobSortBy,
		maxResults int, nextToken string,
	) ([]*IngestionJob, string, error)

	// Flow operations.
	CreateFlow(ctx context.Context, cfg FlowConfig) (*Flow, error)
	GetFlow(ctx context.Context, flowID string) (*Flow, error)
	UpdateFlow(ctx context.Context, flowID string, cfg FlowConfig) (*Flow, error)
	DeleteFlow(ctx context.Context, flowID string) error
	ListFlows(ctx context.Context, maxResults int, nextToken string) ([]*FlowSummary, string, error)
	PrepareFlow(ctx context.Context, flowID string) (*Flow, error)
	ValidateFlowDefinition(ctx context.Context, definition map[string]any) ([]FlowValidationError, error)

	// Flow version operations.
	CreateFlowVersion(ctx context.Context, flowID, description string) (*FlowVersion, error)
	GetFlowVersion(ctx context.Context, flowID, flowVersion string) (*FlowVersion, error)
	DeleteFlowVersion(ctx context.Context, flowID, flowVersion string, skipResourceInUseCheck bool) error
	ListFlowVersions(
		ctx context.Context, flowID string, maxResults int, nextToken string,
	) ([]*FlowVersionSummary, string, error)

	// Flow alias operations.
	CreateFlowAlias(ctx context.Context, flowID string, cfg FlowAliasConfig) (*FlowAlias, error)
	GetFlowAlias(ctx context.Context, flowID, aliasID string) (*FlowAlias, error)
	UpdateFlowAlias(ctx context.Context, flowID, aliasID string, cfg FlowAliasConfig) (*FlowAlias, error)
	DeleteFlowAlias(ctx context.Context, flowID, aliasID string) error
	ListFlowAliases(
		ctx context.Context, flowID string, maxResults int, nextToken string,
	) ([]*FlowAliasSummary, string, error)

	// Prompt operations.
	CreatePrompt(ctx context.Context, cfg PromptConfig) (*Prompt, error)
	GetPrompt(ctx context.Context, promptID string) (*Prompt, error)
	UpdatePrompt(ctx context.Context, promptID string, cfg PromptConfig) (*Prompt, error)
	DeletePrompt(ctx context.Context, promptID string) error
	ListPrompts(ctx context.Context, maxResults int, nextToken string) ([]*PromptSummary, string, error)

	// Prompt version operations.
	CreatePromptVersion(ctx context.Context, promptID, description string) (*PromptVersion, error)
	GetPromptVersion(ctx context.Context, promptID, version string) (*PromptVersion, error)
	DeletePromptVersion(ctx context.Context, promptID, version string) error

	// Knowledge base document operations.
	IngestKnowledgeBaseDocuments(
		ctx context.Context, kbID, dataSourceID string, docs []KBDocument,
	) ([]KBDocumentDetail, error)
	GetKnowledgeBaseDocuments(
		ctx context.Context, kbID, dataSourceID string, ids []KBDocumentIdentifier,
	) ([]KBDocumentDetail, error)
	DeleteKnowledgeBaseDocuments(
		ctx context.Context, kbID, dataSourceID string, ids []KBDocumentIdentifier,
	) ([]KBDocumentDetail, error)
	ListKnowledgeBaseDocuments(
		ctx context.Context, kbID, dataSourceID string, maxResults int, nextToken string,
	) ([]KBDocumentDetail, string, error)

	// Tagging operations.
	ListTagsForResource(ctx context.Context, resourceARN string) (map[string]string, error)
	TagResource(ctx context.Context, resourceARN string, tags map[string]string) error
	UntagResource(ctx context.Context, resourceARN string, tagKeys []string) error

	// Resource policy operations (knowledge bases only -- see
	// resource_policy.go's package doc comment).
	PutResourcePolicy(ctx context.Context, resourceArn, policy, expectedRevisionID string) (*ResourcePolicy, error)
	GetResourcePolicy(ctx context.Context, resourceArn string) (*ResourcePolicy, error)
	DeleteResourcePolicy(ctx context.Context, resourceArn, expectedRevisionID string) (string, error)
}

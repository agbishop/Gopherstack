package amplify

import (
	"context"
	"time"
)

// StorageBackend defines the interface for Amplify storage operations.
type StorageBackend interface {
	// Snapshot and Restore implement persistence.Persistable. Handler
	// delegates to them (see persistence.go) so cli.go's generic
	// setupPersistence picks Amplify up.
	Snapshot(ctx context.Context) []byte
	Restore(ctx context.Context, data []byte) error

	CreateApp(
		name, description, repository, platform string,
		tagMap map[string]string,
		opts ...AppOptions,
	) (*App, error)
	GetApp(appID string) (*App, error)
	ListApps(nextToken string, maxResults int) ([]*App, string, error)
	DeleteApp(appID string) (*App, error)
	UpdateApp(
		appID, name, description, repository, platform string,
		opts ...AppOptions,
	) (*App, error)
	CreateBranch(
		appID, branchName, description, stage string,
		enableAutoBuild bool,
		tagMap map[string]string,
		opts ...BranchOptions,
	) (*Branch, error)
	GetBranch(appID, branchName string) (*Branch, error)
	ListBranches(appID, nextToken string, maxResults int) ([]*Branch, string, error)
	DeleteBranch(appID, branchName string) (*Branch, error)
	UpdateBranch(
		appID, branchName, description, stage string,
		enableAutoBuild bool,
		opts ...BranchOptions,
	) (*Branch, error)
	TagResource(resourceARN string, tagMap map[string]string) error
	UntagResource(resourceARN string, tagKeys []string) error
	ListTagsForResource(resourceARN string) (map[string]string, error)
	// Jobs
	StartJob(
		appID, branchName, jobType, jobID, commitID, commitMsg string,
		commitTime time.Time,
	) (*Job, error)
	StopJob(appID, branchName, jobID string) (*Job, error)
	GetJob(appID, branchName, jobID string) (*Job, error)
	ListJobs(appID, branchName, nextToken string, maxResults int) ([]*Job, string, error)
	DeleteJob(appID, branchName, jobID string) (*Job, error)
	CreateDeployment(appID, branchName string) (string, string, error)
	StartDeployment(appID, branchName, jobID, sourceURL string) (*Job, error)
	// Domains
	CreateDomainAssociation(
		appID, domainName string, subDomains []SubDomainSetting, enableAutoSubDomain bool,
		autoSubDomainCreationPatterns []string, autoSubDomainIAMRole string,
		certSettings *domainCertificateSettings,
	) (*DomainAssociation, error)
	UpdateDomainAssociation(
		appID, domainName string, subDomains []SubDomainSetting, enableAutoSubDomain *bool,
		autoSubDomainCreationPatterns []string, autoSubDomainIAMRole *string,
		certSettings *domainCertificateSettings,
	) (*DomainAssociation, error)
	DeleteDomainAssociation(appID, domainName string) (*DomainAssociation, error)
	GetDomainAssociation(appID, domainName string) (*DomainAssociation, error)
	ListDomainAssociations(
		appID, nextToken string,
		maxResults int,
	) ([]*DomainAssociation, string, error)
	// Webhooks
	CreateWebhook(appID, branchName, description string) (*Webhook, error)
	UpdateWebhook(webhookID, branchName, description string) (*Webhook, error)
	DeleteWebhook(webhookID string) (*Webhook, error)
	GetWebhook(webhookID string) (*Webhook, error)
	ListWebhooks(appID, nextToken string, maxResults int) ([]*Webhook, string, error)
	// Backend environments
	CreateBackendEnvironment(
		appID, environmentName, stackName, deploymentArtifacts string,
	) (*BackendEnvironment, error)
	GetBackendEnvironment(appID, environmentName string) (*BackendEnvironment, error)
	DeleteBackendEnvironment(appID, environmentName string) (*BackendEnvironment, error)
	ListBackendEnvironments(
		appID, environmentName, nextToken string,
		maxResults int,
	) ([]*BackendEnvironment, string, error)
	// Logs and artifacts
	GenerateAccessLogs(appID, domainName, startTime, endTime string) (string, error)
	GetArtifactURL(artifactID string) (string, string, error)
	ListArtifacts(
		appID, branchName, jobID, nextToken string,
		maxResults int,
	) ([]*Artifact, string, error)
}

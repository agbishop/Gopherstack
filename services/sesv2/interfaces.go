package sesv2

import (
	"context"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// StorageBackend is the interface for sesv2 storage operations.
type StorageBackend interface {
	// Core identity ops
	CreateEmailIdentity(
		identity, configurationSetName string,
		tags map[string]string,
	) (*EmailIdentity, error)
	GetEmailIdentity(identity string) (*EmailIdentity, error)
	ListEmailIdentities(nextToken string, pageSize int) page.Page[*EmailIdentity]
	DeleteEmailIdentity(identity string) error

	// Email identity policy ops
	CreateEmailIdentityPolicy(identity, policyName, policy string) error
	GetEmailIdentityPolicies(identity string) (map[string]string, error)
	DeleteEmailIdentityPolicy(identity, policyName string) error
	UpdateEmailIdentityPolicy(identity, policyName, policy string) error

	// Email identity attribute ops
	PutEmailIdentityConfigurationSetAttributes(identity, configSetName string) error
	PutEmailIdentityDkimAttributes(identity string, signingEnabled bool) error
	PutEmailIdentityDkimSigningAttributes(identity string) (*EmailIdentity, error)
	PutEmailIdentityFeedbackAttributes(identity string, emailForwardingEnabled bool) error
	PutEmailIdentityMailFromAttributes(identity, mailFromDomain, behaviorOnMxFailure string) error

	// Configuration set ops
	CreateConfigurationSet(name string, tags map[string]string) (*ConfigurationSet, error)
	GetConfigurationSet(name string) (*ConfigurationSet, error)
	ListConfigurationSets(nextToken string, pageSize int) page.Page[*ConfigurationSet]
	DeleteConfigurationSet(name string) error

	// Configuration set attribute ops
	PutConfigurationSetArchivingOptions(name, archiveARN string) error
	PutConfigurationSetDeliveryOptions(name, tlsPolicy, sendingPoolName string) error
	PutConfigurationSetReputationOptions(name string, metricsEnabled bool) error
	PutConfigurationSetSendingOptions(name string, sendingEnabled bool) error
	PutConfigurationSetSuppressionOptions(name string, suppressedReasons []string) error
	PutConfigurationSetTrackingOptions(name, customRedirectDomain, httpsPolicy string) error
	PutConfigurationSetVdmOptions(
		name string,
		dashboardOptions, guardianOptions map[string]any,
	) error

	// Event destination ops
	CreateConfigurationSetEventDestination(
		configSetName, destName string,
		enabled bool,
		matchingEventTypes []string,
	) (*EventDestination, error)
	GetConfigurationSetEventDestinations(configSetName string) ([]*EventDestination, error)
	DeleteConfigurationSetEventDestination(configSetName, destName string) error
	UpdateConfigurationSetEventDestination(
		configSetName, destName string,
		enabled bool,
		matchingEventTypes []string,
	) error

	// Email sending ops
	SendEmail(
		from string,
		to []string,
		subject, bodyHTML, bodyText string,
		template *bulkEmailTemplate,
	) (string, error)
	SendBulkEmail(
		fromEmailAddress string,
		defaultContent *bulkEmailContent,
		bulkEmailEntries []bulkEmailEntry,
	) ([]bulkEmailEntryResultOutput, error)
	SendCustomVerificationEmail(emailAddress, templateName string) (string, error)
	ListEmails() []Email

	// Contact list ops
	CreateContactList(name, description string, tags map[string]string) (*ContactList, error)
	GetContactList(name string) (*ContactList, error)
	DeleteContactList(name string) error
	UpdateContactList(name, description string) error
	ListContactLists(nextToken string, pageSize int) page.Page[*ContactList]

	// Contact ops
	CreateContact(
		contactListName, emailAddress string,
		topicPreferences []TopicPreference,
	) (*Contact, error)
	GetContact(contactListName, emailAddress string) (*Contact, error)
	DeleteContact(contactListName, emailAddress string) error
	UpdateContact(contactListName, emailAddress string, topicPreferences []TopicPreference) error
	ListContacts(contactListName, nextToken string, pageSize int) (page.Page[*Contact], error)

	// Custom verification template ops
	CreateCustomVerificationEmailTemplate(
		in *CustomVerificationEmailTemplate,
	) (*CustomVerificationEmailTemplate, error)
	GetCustomVerificationEmailTemplate(name string) (*CustomVerificationEmailTemplate, error)
	DeleteCustomVerificationEmailTemplate(name string) error
	UpdateCustomVerificationEmailTemplate(in *CustomVerificationEmailTemplate) error
	ListCustomVerificationEmailTemplates(
		nextToken string,
		pageSize int,
	) page.Page[*CustomVerificationEmailTemplate]

	// Dedicated IP pool ops
	CreateDedicatedIPPool(
		poolName, scalingMode string,
		tags map[string]string,
	) (*DedicatedIPPool, error)
	GetDedicatedIPPool(poolName string) (*DedicatedIPPool, error)
	DeleteDedicatedIPPool(poolName string) error
	ListDedicatedIPPools(nextToken string, pageSize int) page.Page[string]
	GetDedicatedIP(ip string) (map[string]any, error)
	GetDedicatedIps(poolName, nextToken string, pageSize int) page.Page[map[string]any]
	PutDedicatedIPInPool(ip, poolName string) error
	PutDedicatedIPPoolScalingAttributes(poolName, scalingMode string) error
	PutDedicatedIPWarmupAttributes(ip string, warmupPercentage int) error

	// Deliverability test report ops
	CreateDeliverabilityTestReport(
		reportName, fromEmailAddress string,
	) (*DeliverabilityTestReport, error)
	GetDeliverabilityTestReport(reportID string) (*DeliverabilityTestReport, error)
	ListDeliverabilityTestReports(
		nextToken string,
		pageSize int,
	) page.Page[*DeliverabilityTestReport]

	// Deliverability dashboard ops
	GetDeliverabilityDashboardOptions() (map[string]any, error)
	PutDeliverabilityDashboardOption(enabled bool, subscribedDomains []string) error
	GetDomainDeliverabilityCampaign(campaignID string) (map[string]any, error)
	GetDomainStatisticsReport(domain, startDate, endDate string) (map[string]any, error)
	ListDomainDeliverabilityCampaigns(
		startDate, endDate, domain, nextToken string,
	) ([]map[string]any, string, error)

	// Email template ops
	CreateEmailTemplate(
		templateName string,
		content *EmailTemplateContent,
		tags map[string]string,
	) (*EmailTemplate, error)
	GetEmailTemplate(name string) (*EmailTemplate, error)
	DeleteEmailTemplate(name string) error
	UpdateEmailTemplate(name string, content *EmailTemplateContent) error
	ListEmailTemplates(nextToken string, pageSize int) page.Page[*EmailTemplate]
	TestRenderEmailTemplate(name, templateData string) (string, error)

	// Export job ops
	CreateExportJob(sourceType string) (*ExportJob, error)
	GetExportJob(jobID string) (*ExportJob, error)
	ListExportJobs(
		exportSourceType, jobStatus, nextToken string,
		pageSize int,
	) page.Page[*ExportJob]
	CancelExportJob(jobID string) error

	// Import job ops
	CreateImportJob(destination ImportDestination) (*ImportJob, error)
	GetImportJob(jobID string) (*ImportJob, error)
	ListImportJobs(importDestinationType, nextToken string, pageSize int) page.Page[*ImportJob]

	// Suppressed destination ops
	PutSuppressedDestination(email, reason string) error
	GetSuppressedDestination(email string) (*SuppressedDestination, error)
	DeleteSuppressedDestination(email string) error
	ListSuppressedDestinations(
		reasons []string,
		startDate, endDate *time.Time,
		nextToken string,
		pageSize int,
	) page.Page[*SuppressedDestination]

	// Account ops
	GetAccount() (*AccountDetails, error)
	PutAccountDetails(details *AccountDetails) error
	PutAccountPricingAttributes(plan string) error
	PutAccountSendingAttributes(sendingEnabled bool) error
	PutAccountSuppressionAttributes(suppressedReasons []string) error
	PutAccountVdmAttributes(vdmAttributes map[string]any) error
	PutAccountDedicatedIPWarmupAttributes(autoWarmupEnabled bool) error
	GetBlacklistReports() (map[string][]string, error)

	TagResource(arn string, tags map[string]string) error
	UntagResource(arn string, tagKeys []string) error
	ListTagsForResource(arn string) (map[string]string, error)

	// Insights / analytics -- GetMessageInsights is real (round-trips
	// against actually-sent messages); GetEmailAddressInsights/
	// ListRecommendations are partly-real, partly honest placeholder; see
	// PARITY.md.
	GetEmailAddressInsights(emailAddress string) (map[string]any, error)
	GetMessageInsights(messageID string) (map[string]any, error)
	ListRecommendations(
		filter map[string]string,
		nextToken string,
		pageSize int,
	) ([]recommendationOutput, string, error)

	// Multi-region endpoint ops
	CreateMultiRegionEndpoint(
		endpointName string,
		secondaryRegions []string,
		tags map[string]string,
	) (*createMultiRegionEndpointOutput, error)
	GetMultiRegionEndpoint(endpointName string) (*multiRegionEndpointOutput, error)
	DeleteMultiRegionEndpoint(endpointName string) (string, error)
	ListMultiRegionEndpoints(
		nextToken string,
		pageSize int,
	) ([]multiRegionEndpointSummaryOutput, string, error)

	// Tenant ops
	CreateTenant(tenantName string, tags map[string]string) (*tenantOutput, error)
	GetTenant(tenantName string) (*tenantOutput, error)
	DeleteTenant(tenantName string) error
	ListTenants(nextToken string, pageSize int) ([]tenantInfoOutput, string, error)
	CreateTenantResourceAssociation(tenantName, resourceArn string) error
	DeleteTenantResourceAssociation(tenantName, resourceArn string) error
	PutTenantSuppressionAttributes(
		tenantName string,
		suppressedReasons []string,
		suppressionScope string,
	) error
	ListResourceTenants(
		resourceArn, nextToken string,
		pageSize int,
	) ([]resourceTenantOutput, string, error)
	ListTenantResources(
		tenantName string,
		filter map[string]string,
		nextToken string,
		pageSize int,
	) ([]tenantResourceOutput, string, error)

	// Reputation entity ops -- real (derived from stored customer-managed
	// status/policy overrides), not stubs; see PARITY.md.
	GetReputationEntity(entityID string) (reputationEntityOutput, error)
	ListReputationEntities(
		filter map[string]string,
		nextToken string,
		pageSize int,
	) ([]reputationEntityOutput, string, error)
	UpdateReputationEntityCustomerManagedStatus(entityID, status string) error
	UpdateReputationEntityPolicy(entityID, policy string) error

	// Metrics
	BatchGetMetricData(queries []MetricDataQuery) ([]MetricDataResult, error)

	// Infrastructure
	Region() string
	AccountID() string
	Reset()
	Snapshot(ctx context.Context) []byte
	Restore(ctx context.Context, data []byte) error
}

var _ StorageBackend = (*InMemoryBackend)(nil)

package ses

import (
	"context"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// SNSPublisher can publish a message to an SNS topic by ARN. Mirrors the
// SNSPublisher interface already declared by cloudwatch, eventbridge, pipes,
// s3, and scheduler (see services/cloudwatch/interfaces.go) -- same
// consuming-service-declares-the-interface convention, wired in cli.go.
type SNSPublisher interface {
	PublishToTopic(topicARN, message string) error
}

// StorageBackend defines the persistence contract for the SES service.
type StorageBackend interface {
	VerifyEmailIdentity(identity string) error
	DeleteIdentity(identity string)
	ListIdentities(nextToken string, maxItems int, identityType string) page.Page[string]
	GetIdentityVerificationAttributes(identities []string) map[string]string
	SendEmail(in SendEmailInput) (string, error)
	SendTemplatedEmail(in SendTemplatedEmailInput) (string, error)
	ListEmails() []Email
	GetEmailByID(messageID string) (Email, error)
	SearchEmails(query string) []Email
	CreateTemplate(tmpl EmailTemplate) error
	UpdateTemplate(tmpl EmailTemplate) error
	GetTemplate(name string) (EmailTemplate, error)
	DeleteTemplate(name string)
	ListTemplates(nextToken string, maxItems int) page.Page[string]
	CreateConfigurationSet(name string) error
	DeleteConfigurationSet(name string) error
	ListConfigurationSets(nextToken string, maxItems int) page.Page[string]
	DescribeConfigurationSet(name string) (ConfigurationSetDescription, error)
	PutConfigurationSetDeliveryOptions(configSetName, tlsPolicy string) error
	GetSendQuota() SendQuota
	GetSendStatistics() []SendDataPoint
	CreateReceiptRuleSet(name string) error
	CloneReceiptRuleSet(originalName, newName string) error
	CreateReceiptRule(ruleSetName string, rule ReceiptRule, after string) error
	DescribeReceiptRule(ruleSetName, ruleName string) (ReceiptRule, error)
	UpdateReceiptRule(ruleSetName string, rule ReceiptRule) error
	ReorderReceiptRuleSet(ruleSetName string, ruleNames []string) error
	SetReceiptRulePosition(ruleSetName, ruleName, after string) error
	CreateReceiptFilter(filter ReceiptFilter) error
	CreateConfigurationSetEventDestination(configSetName string, dest EventDestination) error
	DeleteConfigurationSetEventDestination(configSetName, destName string) error
	UpdateConfigurationSetEventDestination(configSetName string, dest EventDestination) error
	UpdateConfigurationSetReputationMetricsEnabled(configSetName string, enabled bool) error
	UpdateConfigurationSetSendingEnabled(configSetName string, enabled bool) error
	CreateConfigurationSetTrackingOptions(configSetName, customRedirectDomain string) error
	DeleteConfigurationSetTrackingOptions(configSetName string) error
	UpdateConfigurationSetTrackingOptions(configSetName, customRedirectDomain string) error
	CreateCustomVerificationEmailTemplate(tmpl CustomVerificationEmailTemplate) error
	DeleteCustomVerificationEmailTemplate(templateName string) error
	UpdateCustomVerificationEmailTemplate(tmpl CustomVerificationEmailTemplate) error
	ListReceiptFilters() []ReceiptFilter
	ListReceiptRuleSets(nextToken string) page.Page[ReceiptRuleSet]
	DeleteReceiptFilter(name string) error
	DeleteReceiptRule(ruleSetName, ruleName string) error
	DeleteReceiptRuleSet(name string) error
	GetCustomVerificationEmailTemplate(templateName string) (CustomVerificationEmailTemplate, error)
	ListCustomVerificationEmailTemplates(nextToken string, maxResults int) page.Page[CustomVerificationEmailTemplate]
	DescribeReceiptRuleSet(name string) (ReceiptRuleSet, error)
	SetActiveReceiptRuleSet(name string) error
	DescribeActiveReceiptRuleSet() (ReceiptRuleSet, bool, error)
	// Identity policy ops
	PutIdentityPolicy(identity, policyName, policy string) error
	DeleteIdentityPolicy(identity, policyName string) error
	GetIdentityPolicies(identity string, policyNames []string) (map[string]string, error)
	ListIdentityPolicies(identity string) ([]string, error)
	// Identity attribute ops
	GetIdentityDkimAttributes(identities []string) map[string]DkimAttributes
	GetIdentityMailFromDomainAttributes(identities []string) map[string]MailFromDomainAttributes
	GetIdentityNotificationAttributes(identities []string) map[string]NotificationAttributes
	SetIdentityDkimEnabled(identity string, enabled bool) error
	SetIdentityFeedbackForwardingEnabled(identity string, enabled bool) error
	SetIdentityHeadersInNotificationsEnabled(identity, notificationType string, enabled bool) error
	SetIdentityMailFromDomain(identity, mailFromDomain, behaviorOnMXFailure string) error
	SetIdentityNotificationTopic(identity, notificationType, snsTopic string) error
	// Domain verification
	VerifyDomainIdentity(domain string) (string, error)
	VerifyDomainDkim(domain string) ([]string, error)
	VerifyEmailAddress(email string) error
	DeleteVerifiedEmailAddress(email string)
	ListVerifiedEmailAddresses() []string
	// Account-level
	UpdateAccountSendingEnabled(enabled bool)
	GetAccountSendingEnabled() bool
	// Send ops
	SendBounce(originalMsgID, bounceSender string, recipients []string) (string, error)
	SendBulkTemplatedEmail(in SendBulkTemplatedEmailInput) ([]string, error)
	SendCustomVerificationEmail(email, templateName, configurationSetName string) (string, error)
	TestRenderTemplate(templateName, templateData string) (string, error)
	Region() string
	AccountID() string
	Reset()
	Snapshot(ctx context.Context) []byte
	Restore(ctx context.Context, data []byte) error
}

// Compile-time check that InMemoryBackend implements StorageBackend.
var _ StorageBackend = (*InMemoryBackend)(nil)

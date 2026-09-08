package ses

import (
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

const defaultAccountID = "123456789012"

// defaultEmailTTL is the default time-to-live for retained emails.
const defaultEmailTTL = 24 * time.Hour

const sesDefaultMaxItems = 100

// InMemoryBackend is an in-memory store for SES emails, verified identities,
// email templates, and configuration sets.
type InMemoryBackend struct {
	snsPublisher         SNSPublisher
	customVerifTemplates *store.Table[CustomVerificationEmailTemplate]
	emailsByID           *store.Table[Email]
	templates            *store.Table[EmailTemplate]
	configSets           *store.Table[ConfigurationSet]
	receiptRuleSets      *store.Table[ReceiptRuleSet]
	receiptFilters       *store.Table[ReceiptFilter]
	eventDestinations    *store.Table[EventDestination]
	// eventDestinationsByConfigSet is a secondary index over
	// eventDestinations grouping by ConfigSetName, answering the "all event
	// destinations of configuration set X" lookups the previous nested
	// map[string]map[string]*EventDestination answered directly.
	eventDestinationsByConfigSet *store.Index[EventDestination]
	// registry holds every "clean" store.Table (templates, receiptRuleSets,
	// receiptFilters, customVerifTemplates) so their Reset/Snapshot/Restore
	// collapse to one call each. "Dirty" tables (identities, configSets,
	// trackingOptions, eventDestinations) and the emailsByID derived cache
	// are NOT on this registry -- see store_setup.go's file doc comment.
	registry              *store.Registry
	trackingOptions       *store.Table[TrackingOptions]
	mu                    *lockmetrics.RWMutex
	identities            *store.Table[IdentityRecord]
	policies              map[string]map[string]string // identity → policyName → policyDocument
	activeRuleSet         string
	region                string
	accountID             string
	emails                []Email
	emailTTL              time.Duration
	configuredEmailTTL    time.Duration
	accountSendingEnabled bool
}

// SetSNSPublisher registers an SNS publisher used to deliver bounce/complaint/
// delivery notifications to identity notification topics (SetIdentityNotificationTopic)
// and configuration-set event destinations (CreateConfigurationSetEventDestination).
func (b *InMemoryBackend) SetSNSPublisher(pub SNSPublisher) {
	b.mu.Lock("SetSNSPublisher")
	defer b.mu.Unlock()

	b.snsPublisher = pub
}

// NewInMemoryBackend creates a new InMemoryBackend with the default email TTL.
func NewInMemoryBackend() *InMemoryBackend {
	b := &InMemoryBackend{
		registry:              store.NewRegistry(),
		policies:              make(map[string]map[string]string),
		emailTTL:              defaultEmailTTL,
		configuredEmailTTL:    defaultEmailTTL,
		accountSendingEnabled: true,
		region:                config.DefaultRegion,
		accountID:             defaultAccountID,
		mu:                    lockmetrics.New("ses"),
	}
	registerAllTables(b)

	return b
}

// WithRegion sets the AWS region for this backend instance and returns it for chaining.
func (b *InMemoryBackend) WithRegion(region string) *InMemoryBackend {
	if region != "" {
		b.region = region
	}

	return b
}

// WithAccountID sets the AWS account ID for this backend instance and returns it for chaining.
func (b *InMemoryBackend) WithAccountID(accountID string) *InMemoryBackend {
	if accountID != "" {
		b.accountID = accountID
	}

	return b
}

// WithEmailTTL sets the TTL for stored sent emails and returns the backend for chaining.
// Zero falls back to the default TTL.
// The configured TTL is preserved across Reset() calls.
func (b *InMemoryBackend) WithEmailTTL(ttl time.Duration) *InMemoryBackend {
	if ttl > 0 {
		b.emailTTL = ttl
		b.configuredEmailTTL = ttl
	}

	return b
}

// Reset clears all in-memory state, restoring the backend to its initial empty state.
// The configured email TTL (set via WithEmailTTL) is preserved.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	b.resetTablesLocked()
	b.emails = nil
	b.policies = make(map[string]map[string]string)
	b.activeRuleSet = ""
	b.accountSendingEnabled = true
	b.emailTTL = b.configuredEmailTTL
}

// resetTablesLocked resets every store.Table-backed resource field to empty:
// the "clean" tables via one b.registry.ResetAll() call, plus the "dirty"
// tables and the emailsByID derived cache individually since they are not
// registered on b.registry (see store_setup.go). The caller MUST hold b.mu
// for writing.
func (b *InMemoryBackend) resetTablesLocked() {
	b.registry.ResetAll()
	b.identities.Reset()
	b.configSets.Reset()
	b.trackingOptions.Reset()
	b.eventDestinations.Reset()
	b.emailsByID.Reset()
}

// ttl returns the current email TTL under a read lock.
func (b *InMemoryBackend) ttl() time.Duration {
	b.mu.RLock("ttl")
	defer b.mu.RUnlock()

	return b.emailTTL
}

// Region returns the AWS region for this backend instance.
func (b *InMemoryBackend) Region() string {
	return b.region
}

// AccountID returns the simulated AWS account ID.
func (b *InMemoryBackend) AccountID() string {
	return b.accountID
}

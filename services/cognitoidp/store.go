package cognitoidp

import (
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

const (
	// bcryptCost is deliberately low (not DefaultCost): this is a mock backend
	// with no real secrets to protect, and DefaultCost under -race measured
	// ~1.3s per hash (vs ~95ms unraced), which was the dominant cost across
	// the package's -race test time (gopherstack CI flake investigation).
	bcryptCost = bcrypt.MinCost

	// poolIDSuffixLen is the length of the random suffix in pool IDs.
	poolIDSuffixLen = 8

	// clientIDLen is the length of randomly generated client IDs.
	clientIDLen = 26

	// clientSecretLen is the length of randomly generated client secrets.
	clientSecretLen = 51

	// clientSecretIDLen is the length of the random suffix used for
	// AddUserPoolClientSecret's ClientSecretId (real AWS's format is
	// documented as "--", an opaque identifier this emulator does not
	// attempt to reproduce structurally).
	clientSecretIDLen = 20

	// alphanumChars contains characters used for random ID generation.
	alphanumChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	// UserStatusUnconfirmed indicates the user has signed up but not confirmed their account.
	UserStatusUnconfirmed = "UNCONFIRMED"

	// UserStatusConfirmed indicates the user has confirmed their account.
	UserStatusConfirmed = "CONFIRMED"

	// UserStatusForceChangePassword indicates the user must change their password on next login.
	UserStatusForceChangePassword = "FORCE_CHANGE_PASSWORD"

	// defaultRefreshTokenTTL is the lifetime for refresh tokens.
	defaultRefreshTokenTTL = 30 * 24 * time.Hour
)

// InMemoryBackend is the in-memory store for Cognito IDP resources.
//
// Most resource collections are *store.Table[T] registered on b.registry (see
// store_setup.go for keyFns/composite keys and the full per-field rationale).
// A handful of fields remain plain maps because their value carries no pure
// identity for a store.Table key; store_setup.go's registerAllTables doc
// comment lists each one and why.
type InMemoryBackend struct {
	mu       *lockmetrics.RWMutex
	registry *store.Registry
	pools    *store.Table[UserPool]
	// poolsByName is a secondary index on pools keyed by Name.
	poolsByName *store.Index[UserPool]
	clients     *store.Table[UserPoolClient]
	// clientsByPool is a secondary index on clients keyed by UserPoolID, for
	// efficient per-pool listing and deletes.
	clientsByPool *store.Index[UserPoolClient]
	// users is keyed by the composite userKey(poolID, username).
	users *store.Table[User]
	// usersByPool is a secondary index on users keyed by UserPoolID.
	usersByPool *store.Index[User]
	// usersBySub is a secondary index on users keyed by userSubKey(poolID,
	// sub), for O(1) access token resolution.
	usersBySub *store.Index[User]
	// refreshTokens maps refresh token → poolID/username for REFRESH_TOKEN_AUTH flow.
	refreshTokens map[string]*refreshTokenEntry
	// refreshTokensByClient maps clientID -> refreshToken set for efficient client cleanup.
	refreshTokensByClient map[string]map[string]struct{}
	// refreshTokensByUser maps poolID+":"+username → refreshToken set for efficient per-user token cleanup.
	refreshTokensByUser map[string]map[string]struct{}
	// mfaSessions maps session token → pending challenge context (MFA or NEW_PASSWORD_REQUIRED).
	mfaSessions map[string]*mfaSessionEntry
	// groups is keyed by the composite groupKey(poolID, groupName).
	groups *store.Table[Group]
	// groupsByPool is a secondary index on groups keyed by UserPoolID.
	groupsByPool *store.Index[Group]
	// groupMembers maps poolID → groupName → set of usernames
	groupMembers map[string]map[string]map[string]struct{}
	// resourceServers is keyed by the composite resourceServerKey(poolID, identifier).
	resourceServers *store.Table[ResourceServer]
	// resourceServersByPool is a secondary index on resourceServers keyed by UserPoolID.
	resourceServersByPool *store.Index[ResourceServer]
	// tokenRevokedBefore maps poolID+":"+username → revocation time for GlobalSignOut.
	// Access tokens with auth_time before this timestamp are rejected.
	tokenRevokedBefore map[string]time.Time
	// identityProviders is keyed by the composite identityProviderKey(poolID, providerName).
	identityProviders *store.Table[IdentityProvider]
	// identityProvidersByPool is a secondary index on identityProviders keyed by UserPoolID.
	identityProvidersByPool *store.Index[IdentityProvider]
	// domains maps domain → UserPoolDomain (domain names are globally unique in Cognito)
	domains *store.Table[UserPoolDomain]
	// resourceTags maps ARN → tag key → tag value
	resourceTags map[string]map[string]string
	// riskConfigurations maps poolID+":"+clientID → RiskConfiguration (clientID="" for pool-level)
	riskConfigurations map[string]*RiskConfiguration
	// logDeliveryConfigs maps poolID → LogDeliveryConfig
	logDeliveryConfigs map[string]*LogDeliveryConfig
	// uiCustomizations is keyed by the composite uiKey(poolID, clientID).
	uiCustomizations *store.Table[UICustomization]
	// managedLoginBrandings is keyed by the composite managedLoginBrandingKey(poolID, brandingID).
	managedLoginBrandings *store.Table[ManagedLoginBranding]
	// managedLoginBrandingsByPool is a secondary index on managedLoginBrandings keyed by UserPoolID.
	managedLoginBrandingsByPool *store.Index[ManagedLoginBranding]
	// terms is keyed by TermsID (client-scoped, per real Cognito).
	terms *store.Table[Terms]
	// termsByPool is a secondary index on terms keyed by UserPoolID, for ListTerms.
	termsByPool *store.Index[Terms]
	// userImportJobs is keyed by the composite userImportJobKey(poolID, jobID).
	userImportJobs *store.Table[UserImportJob]
	// userImportJobsByPool is a secondary index on userImportJobs keyed by UserPoolID.
	userImportJobsByPool *store.Index[UserImportJob]
	// poolMfaConfigs maps poolID → full MFA config (SMS/TOTP/Email sub-configs)
	poolMfaConfigs map[string]*UserPoolMfaFullConfig
	// attrVerificationCodes maps poolID+":"+username+":"+attrName → pending verification entry
	attrVerificationCodes map[string]*attrVerificationEntry
	// typedRiskConfigurations maps poolID+":"+clientID → typed risk configuration
	typedRiskConfigurations *store.Table[TypedRiskConfiguration]
	// devices maps poolID+":"+username → deviceKey → *Device (device tracking / "remember this device").
	devices map[string]map[string]*Device
	// webauthnCredentials maps poolID+":"+username → credentialID → *WebAuthnCredential.
	webauthnCredentials map[string]map[string]*WebAuthnCredential
	// authEvents maps poolID+":"+username → eventID → *AuthEvent (adaptive-auth event feedback tracking).
	authEvents map[string]map[string]*AuthEvent
	// userPoolReplicas is keyed by the composite replicaKey(poolID, regionName).
	userPoolReplicas *store.Table[UserPoolReplica]
	// userPoolReplicasByPool is a secondary index on userPoolReplicas keyed by UserPoolID.
	userPoolReplicasByPool *store.Index[UserPoolReplica]
	// provisionedLimits maps API_CATEGORY Category (e.g. "UserAuthentication") →
	// the current provisioned RPS value for that category. Provisioned limits
	// are account+Region-level (see provisioned_limits.go), not per-user-pool,
	// so this is a flat map rather than something keyed off a user pool.
	provisionedLimits map[string]int32
	// lambdaInvoker fires configured User Pool Lambda triggers (PreSignUp,
	// PostConfirmation, PreTokenGeneration, CustomMessage, ...). nil disables
	// trigger invocation entirely -- see lambda_triggers.go.
	lambdaInvoker LambdaTriggerInvoker
	accountID     string
	region        string
	endpoint      string
}

// NewInMemoryBackend creates a new InMemoryBackend.
func NewInMemoryBackend(accountID, region, endpoint string) *InMemoryBackend {
	b := &InMemoryBackend{
		mu:                    lockmetrics.New("cognitoidp"),
		registry:              store.NewRegistry(),
		refreshTokens:         make(map[string]*refreshTokenEntry),
		refreshTokensByClient: make(map[string]map[string]struct{}),
		refreshTokensByUser:   make(map[string]map[string]struct{}),
		mfaSessions:           make(map[string]*mfaSessionEntry),
		groupMembers:          make(map[string]map[string]map[string]struct{}),
		tokenRevokedBefore:    make(map[string]time.Time),
		resourceTags:          make(map[string]map[string]string),
		riskConfigurations:    make(map[string]*RiskConfiguration),
		logDeliveryConfigs:    make(map[string]*LogDeliveryConfig),
		poolMfaConfigs:        make(map[string]*UserPoolMfaFullConfig),
		attrVerificationCodes: make(map[string]*attrVerificationEntry),
		devices:               make(map[string]map[string]*Device),
		webauthnCredentials:   make(map[string]map[string]*WebAuthnCredential),
		authEvents:            make(map[string]map[string]*AuthEvent),
		provisionedLimits:     make(map[string]int32),
		accountID:             accountID,
		region:                region,
		endpoint:              endpoint,
	}

	registerAllTables(b)

	return b
}

// Reset clears all backend state. Useful for test isolation.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	b.registry.ResetAll()

	b.refreshTokens = make(map[string]*refreshTokenEntry)
	b.refreshTokensByClient = make(map[string]map[string]struct{})
	b.refreshTokensByUser = make(map[string]map[string]struct{})
	b.mfaSessions = make(map[string]*mfaSessionEntry)
	b.groupMembers = make(map[string]map[string]map[string]struct{})
	b.tokenRevokedBefore = make(map[string]time.Time)
	b.resourceTags = make(map[string]map[string]string)
	b.riskConfigurations = make(map[string]*RiskConfiguration)
	b.logDeliveryConfigs = make(map[string]*LogDeliveryConfig)
	b.poolMfaConfigs = make(map[string]*UserPoolMfaFullConfig)
	b.attrVerificationCodes = make(map[string]*attrVerificationEntry)
	b.devices = make(map[string]map[string]*Device)
	b.webauthnCredentials = make(map[string]map[string]*WebAuthnCredential)
	b.authEvents = make(map[string]map[string]*AuthEvent)
	b.provisionedLimits = make(map[string]int32)
}

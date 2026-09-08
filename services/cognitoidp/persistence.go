package cognitoidp

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/persistence"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

var (
	errDecodePEM = errors.New("failed to decode RSA private key PEM block")
	errNotRSAKey = errors.New("private key is not *rsa.PrivateKey")
)

// cognitoidpSnapshotVersion identifies the shape of [backendSnapshot]. It must
// be bumped whenever a change to a DTO or to backendSnapshot itself would make
// an older snapshot unsafe to decode as the current shape. Restore compares
// this against the persisted value and discards (rather than attempts to
// partially decode) any mismatch — see Restore below. There was no version
// guard prior to Phase 3.3's pkgs/store conversion, so any pre-existing
// snapshot decodes with Version 0 and is treated as incompatible.
//
// Deliberately NOT bumped for the terms/ redesign (gopherstack-kxow): a
// version bump here discards the ENTIRE snapshot on mismatch (see Restore
// below), not just the changed table -- every user pool, user, password hash,
// and MFA setting in a real deployment would be lost on upgrade to save a
// table that cannot hold real data anyway (CreateTerms was unreachable by any
// real SDK client pre-redesign; its own required-member validation rejects
// the request before it is ever sent). restoreTermsLocked handles the old
// terms shape defensively instead -- see its doc comment.
// Bumped 1 -> 2 for gopherstack-xasq: SchemaAttribute's constraint fields went from
// flattened int64/float64 top-level fields to nested string-valued sub-objects
// (StringAttributeConstraints/NumberAttributeConstraints), matching the real wire shape.
// A structural reshape, not a rename or addition -- an old snapshot's flattened numeric
// fields are simply gone, so decoding it as the new shape would silently lose them.
const cognitoidpSnapshotVersion = 2

// userPoolSnapshot holds the serializable fields of a UserPool.
type userPoolSnapshot struct {
	LambdaConfig           map[string]any    `json:"lambdaConfig,omitempty"`
	EmailConfiguration     map[string]any    `json:"emailConfiguration,omitempty"`
	AccountRecoverySetting map[string]any    `json:"accountRecoverySetting,omitempty"`
	PasswordPolicy         *PasswordPolicy   `json:"passwordPolicy,omitempty"`
	CreatedAt              string            `json:"createdAt,omitempty"`
	ID                     string            `json:"id,omitempty"`
	Name                   string            `json:"name,omitempty"`
	ARN                    string            `json:"arn,omitempty"`
	IssuerURL              string            `json:"issuerUrl,omitempty"`
	KeyID                  string            `json:"keyId,omitempty"`
	PrivKeyPEM             string            `json:"privKeyPem,omitempty"`
	MfaConfiguration       string            `json:"mfaConfiguration,omitempty"`
	DeletionProtection     string            `json:"deletionProtection,omitempty"`
	CustomAttributes       []SchemaAttribute `json:"customAttributes,omitempty"`
	AutoVerifiedAttributes []string          `json:"autoVerifiedAttributes,omitempty"`
}

// poolSnapshotKeyFn is the [store.Table] key function for the ephemeral pool
// DTO table, mirroring poolsKeyFn.
func poolSnapshotKeyFn(v *userPoolSnapshot) string { return v.ID }

// userSnapshot is a copy of User safe for JSON serialization.
type userSnapshot struct {
	CreatedAt            string            `json:"createdAt,omitempty"`
	UpdatedAt            string            `json:"updatedAt,omitempty"`
	ConfirmCodeExpiresAt string            `json:"confirmCodeExpiresAt,omitempty"`
	LastAuthTime         string            `json:"lastAuthTime,omitempty"`
	TempPasswordIssuedAt string            `json:"tempPasswordIssuedAt,omitempty"`
	Attributes           map[string]string `json:"attributes,omitempty"`
	Sub                  string            `json:"sub,omitempty"`
	Username             string            `json:"username,omitempty"`
	UserPoolID           string            `json:"userPoolId,omitempty"`
	PasswordHash         string            `json:"passwordHash,omitempty"`
	SRPSalt              string            `json:"srpSalt,omitempty"`
	SRPVerifier          string            `json:"srpVerifier,omitempty"`
	Status               string            `json:"status,omitempty"`
	ConfirmCode          string            `json:"confirmCode,omitempty"`
	PreferredMfaSetting  string            `json:"preferredMfaSetting,omitempty"`
	TOTPSecret           string            `json:"totpSecret,omitempty"`
	UserMFASettingList   []string          `json:"userMFASettingList,omitempty"`
	MFAOptions           []MFAOptionType   `json:"mfaOptions,omitempty"`
	LinkedProviders      []ProviderLink    `json:"linkedProviders,omitempty"`
	Enabled              bool              `json:"enabled,omitempty"`
	TOTPVerified         bool              `json:"totpVerified,omitempty"`
}

// userSnapshotKeyFn is the [store.Table] key function for the ephemeral user
// DTO table, mirroring usersKeyFn.
func userSnapshotKeyFn(v *userSnapshot) string { return userKey(v.UserPoolID, v.Username) }

// backendSnapshot is the top-level on-disk shape for the cognitoidp backend.
//
// Tables holds one JSON-encoded array per registered table, produced by
// [store.Registry.SnapshotAll]: "pools" and "users" go through a DTO
// transform (UserPool/User carry unexported/derived fields that cannot
// round-trip through JSON directly -- see buildPoolSnapshot/buildUserSnapshot),
// while every other converted table (clients, groups, resourceServers,
// identityProviders, domains, terms, userImportJobs, managedLoginBrandings,
// uiCustomizations, typedRiskConfigurations) is clean enough to register
// directly, with no DTO needed. "terms" is snapshotted this same way but
// restored separately and defensively -- see restoreTermsLocked.
//
// Every field below the Tables map is a resource left as a plain map on
// InMemoryBackend (see store_setup.go's registerAllTables doc for why each
// one can't be a store.Table) and is persisted exactly as it was before this
// conversion.
type backendSnapshot struct {
	Tables              map[string]json.RawMessage                `json:"tables,omitempty"`
	RefreshTokens       map[string]*refreshTokenEntry             `json:"refreshTokens,omitempty"`
	GroupMembers        map[string]map[string]map[string]struct{} `json:"groupMembers,omitempty"`
	TokenRevokedBefore  map[string]time.Time                      `json:"tokenRevokedBefore,omitempty"`
	ResourceTags        map[string]map[string]string              `json:"resourceTags,omitempty"`
	RiskConfigurations  map[string]*RiskConfiguration             `json:"riskConfigurations,omitempty"`
	LogDeliveryConfigs  map[string]*LogDeliveryConfig             `json:"logDeliveryConfigs,omitempty"`
	PoolMfaConfigs      map[string]*UserPoolMfaFullConfig         `json:"poolMfaConfigs,omitempty"`
	Devices             map[string]map[string]*Device             `json:"devices,omitempty"`
	WebAuthnCredentials map[string]map[string]*WebAuthnCredential `json:"webauthnCredentials,omitempty"`
	AuthEvents          map[string]map[string]*AuthEvent          `json:"authEvents,omitempty"`
	ProvisionedLimits   map[string]int32                          `json:"provisionedLimits,omitempty"`
	AccountID           string                                    `json:"accountId,omitempty"`
	Region              string                                    `json:"region,omitempty"`
	Endpoint            string                                    `json:"endpoint,omitempty"`
	Version             int                                       `json:"version"`
}

func marshalRSAKey(key *rsa.PrivateKey) (string, error) {
	if key == nil {
		return "", nil
	}

	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", fmt.Errorf("marshal RSA private key: %w", err)
	}

	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), nil
}

func unmarshalRSAKey(pemStr string) (*rsa.PrivateKey, error) {
	if pemStr == "" {
		return nil, nil //nolint:nilnil // intentional: absent key is valid
	}

	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, errDecodePEM
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse PKCS8 private key: %w", err)
	}

	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("%w: got %T", errNotRSAKey, key)
	}

	return rsaKey, nil
}

// buildPoolSnapshot converts a live UserPool into its serializable form.
func buildPoolSnapshot(ctx context.Context, p *UserPool) *userPoolSnapshot {
	pem, err := marshalRSAKey(p.issuer.privateKey)
	if err != nil {
		logger.Load(ctx).
			WarnContext(ctx, "cognitoidp: failed to marshal RSA key for pool snapshot", "poolId", p.ID, "error", err)
		pem = ""
	}

	var ppSnap *PasswordPolicy
	if p.PasswordPolicy != nil {
		pp := *p.PasswordPolicy
		ppSnap = &pp
	}

	var avAttrs []string
	if len(p.AutoVerifiedAttributes) > 0 {
		avAttrs = make([]string, len(p.AutoVerifiedAttributes))
		copy(avAttrs, p.AutoVerifiedAttributes)
	}

	return &userPoolSnapshot{
		CreatedAt:              p.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		ID:                     p.ID,
		Name:                   p.Name,
		ARN:                    p.ARN,
		IssuerURL:              p.issuer.issuerURL,
		KeyID:                  p.issuer.keyID,
		PrivKeyPEM:             pem,
		CustomAttributes:       p.CustomAttributes,
		MfaConfiguration:       p.MfaConfiguration,
		DeletionProtection:     p.DeletionProtection,
		PasswordPolicy:         ppSnap,
		AutoVerifiedAttributes: avAttrs,
		LambdaConfig:           p.LambdaConfig,
		EmailConfiguration:     p.EmailConfiguration,
		AccountRecoverySetting: p.AccountRecoverySetting,
	}
}

// buildUserSnapshot converts a live User into its serializable form.
func buildUserSnapshot(u *User) *userSnapshot {
	var codeExpiry string
	if !u.ConfirmCodeExpiresAt.IsZero() {
		codeExpiry = u.ConfirmCodeExpiresAt.UTC().Format("2006-01-02T15:04:05Z")
	}

	var lastAuth string
	if !u.LastAuthTime.IsZero() {
		// .UTC() matters here: Format's "Z" is a literal, not a zone conversion, so a
		// local-time value would round-trip through Parse (which defaults to UTC) offset
		// by the local zone.
		lastAuth = u.LastAuthTime.UTC().Format("2006-01-02T15:04:05Z")
	}

	var tempPasswordIssuedAt string
	if !u.TempPasswordIssuedAt.IsZero() {
		tempPasswordIssuedAt = u.TempPasswordIssuedAt.UTC().Format("2006-01-02T15:04:05Z")
	}

	return &userSnapshot{
		CreatedAt:            u.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt:            u.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		ConfirmCodeExpiresAt: codeExpiry,
		LastAuthTime:         lastAuth,
		TempPasswordIssuedAt: tempPasswordIssuedAt,
		Attributes:           u.Attributes,
		Sub:                  u.Sub,
		Username:             u.Username,
		UserPoolID:           u.UserPoolID,
		PasswordHash:         u.PasswordHash,
		SRPSalt:              u.SRPSalt,
		SRPVerifier:          u.SRPVerifier,
		Status:               u.Status,
		ConfirmCode:          u.ConfirmCode,
		PreferredMfaSetting:  u.PreferredMfaSetting,
		TOTPSecret:           u.TOTPSecret,
		TOTPVerified:         u.TOTPVerified,
		UserMFASettingList:   u.UserMFASettingList,
		MFAOptions:           u.MFAOptions,
		LinkedProviders:      u.LinkedProviders,
		Enabled:              u.Enabled,
	}
}

// Snapshot serialises the backend state to JSON.
func (b *InMemoryBackend) Snapshot(ctx context.Context) []byte {
	b.mu.RLock("Snapshot")
	defer b.mu.RUnlock()

	// Build a throwaway DTO registry purely to reuse store's deterministic,
	// type-erased JSON encoding (store.Registry.SnapshotAll) instead of
	// hand-rolling the marshal step. pools/users need a DTO transform (see
	// buildPoolSnapshot/buildUserSnapshot); every other converted table is
	// already cleanly serializable, so its live *store.Table is registered
	// directly -- Register only stores a reference, so this does not disturb
	// b.registry.
	dtoReg := store.NewRegistry()

	poolDTOs := store.Register(dtoReg, "pools", store.New(poolSnapshotKeyFn))
	for _, p := range b.pools.All() {
		poolDTOs.Put(buildPoolSnapshot(ctx, p))
	}

	userDTOs := store.Register(dtoReg, "users", store.New(userSnapshotKeyFn))
	for _, u := range b.users.All() {
		userDTOs.Put(buildUserSnapshot(u))
	}

	store.Register(dtoReg, "clients", b.clients)
	store.Register(dtoReg, "groups", b.groups)
	store.Register(dtoReg, "resourceServers", b.resourceServers)
	store.Register(dtoReg, "identityProviders", b.identityProviders)
	store.Register(dtoReg, "domains", b.domains)
	store.Register(dtoReg, "terms", b.terms)
	store.Register(dtoReg, "userImportJobs", b.userImportJobs)
	store.Register(dtoReg, "managedLoginBrandings", b.managedLoginBrandings)
	store.Register(dtoReg, "uiCustomizations", b.uiCustomizations)
	store.Register(dtoReg, "typedRiskConfigurations", b.typedRiskConfigurations)
	store.Register(dtoReg, "userPoolReplicas", b.userPoolReplicas)

	tables, err := dtoReg.SnapshotAll()
	if err != nil {
		logger.Load(ctx).WarnContext(ctx, "cognitoidp: failed to marshal backend snapshot", "error", err)

		return nil
	}

	snap := backendSnapshot{
		Version:             cognitoidpSnapshotVersion,
		Tables:              tables,
		RefreshTokens:       b.refreshTokens,
		GroupMembers:        b.groupMembers,
		TokenRevokedBefore:  b.tokenRevokedBefore,
		ResourceTags:        b.resourceTags,
		RiskConfigurations:  b.riskConfigurations,
		LogDeliveryConfigs:  b.logDeliveryConfigs,
		PoolMfaConfigs:      b.poolMfaConfigs,
		Devices:             b.devices,
		WebAuthnCredentials: b.webauthnCredentials,
		AuthEvents:          b.authEvents,
		ProvisionedLimits:   b.provisionedLimits,
		AccountID:           b.accountID,
		Region:              b.region,
		Endpoint:            b.endpoint,
	}

	data, err := json.Marshal(snap)
	if err != nil {
		logger.Load(ctx).WarnContext(ctx, "cognitoidp: failed to marshal backend snapshot", "error", err)

		return nil
	}

	return data
}

// Restore loads backend state from a JSON snapshot.
func (b *InMemoryBackend) Restore(ctx context.Context, data []byte) error {
	var snap backendSnapshot

	if err := persistence.UnmarshalSnapshot(ctx, "cognitoidp", data, &snap); err != nil {
		return err
	}

	b.mu.Lock("Restore")
	defer b.mu.Unlock()

	if snap.Version != cognitoidpSnapshotVersion {
		// An incompatible (older/newer/absent) snapshot version must never be
		// partially decoded as the current shape -- that risks silently
		// misinterpreting fields. Discard cleanly and start empty instead of
		// erroring, since this is an expected, recoverable condition (e.g.
		// upgrading gopherstack across a snapshot-format change), not data
		// corruption.
		logger.Load(ctx).WarnContext(ctx,
			"cognitoidp: discarding incompatible snapshot version, starting empty",
			"gotVersion", snap.Version, "wantVersion", cognitoidpSnapshotVersion)

		b.resetForIncompatibleSnapshotLocked()

		return nil
	}

	normalizeBackendSnapshot(&snap)

	if err := b.restoreTablesLocked(snap.Tables); err != nil {
		return err
	}

	b.restoreTermsLocked(ctx, snap.Tables["terms"])
	b.restoreRawMapsLocked(&snap)

	return nil
}

// resetForIncompatibleSnapshotLocked clears every table and raw map to the
// empty state a fresh NewInMemoryBackend would have. Caller must hold b.mu in
// write mode.
func (b *InMemoryBackend) resetForIncompatibleSnapshotLocked() {
	b.registry.ResetAll()
	b.refreshTokens = make(map[string]*refreshTokenEntry)
	b.refreshTokensByClient = make(map[string]map[string]struct{})
	b.refreshTokensByUser = make(map[string]map[string]struct{})
	b.groupMembers = make(map[string]map[string]map[string]struct{})
	b.tokenRevokedBefore = make(map[string]time.Time)
	b.resourceTags = make(map[string]map[string]string)
	b.riskConfigurations = make(map[string]*RiskConfiguration)
	b.logDeliveryConfigs = make(map[string]*LogDeliveryConfig)
	b.devices = make(map[string]map[string]*Device)
	b.webauthnCredentials = make(map[string]map[string]*WebAuthnCredential)
	b.authEvents = make(map[string]map[string]*AuthEvent)
	b.provisionedLimits = make(map[string]int32)
}

// restoreTablesLocked decodes tables into every store.Table-backed resource
// EXCEPT terms, which restoreTermsLocked handles separately and defensively.
// pools/users go through a DTO transform (see restorePoolsFromSnapshot/
// restoreUsersFromSnapshot); every other converted table restores straight
// into its live *store.Table (Restore rebuilds the table's primary map AND
// every secondary index registered on it, e.g. clientsByPool/groupsByPool, so
// no separate index rebuild step is needed for these). Caller must hold b.mu
// in write mode.
func (b *InMemoryBackend) restoreTablesLocked(tables map[string]json.RawMessage) error {
	dtoReg := store.NewRegistry()

	poolDTOs := store.Register(dtoReg, "pools", store.New(poolSnapshotKeyFn))
	userDTOs := store.Register(dtoReg, "users", store.New(userSnapshotKeyFn))
	store.Register(dtoReg, "clients", b.clients)
	store.Register(dtoReg, "groups", b.groups)
	store.Register(dtoReg, "resourceServers", b.resourceServers)
	store.Register(dtoReg, "identityProviders", b.identityProviders)
	store.Register(dtoReg, "domains", b.domains)
	store.Register(dtoReg, "userImportJobs", b.userImportJobs)
	store.Register(dtoReg, "managedLoginBrandings", b.managedLoginBrandings)
	store.Register(dtoReg, "uiCustomizations", b.uiCustomizations)
	store.Register(dtoReg, "typedRiskConfigurations", b.typedRiskConfigurations)
	store.Register(dtoReg, "userPoolReplicas", b.userPoolReplicas)

	if err := dtoReg.RestoreAll(tables); err != nil {
		return fmt.Errorf("cognitoidp: restore snapshot tables: %w", err)
	}

	pools, err := restorePoolsFromSnapshot(poolDTOs.All())
	if err != nil {
		return err
	}

	b.pools.Restore(pools)
	b.users.Restore(restoreUsersFromSnapshot(userDTOs.All()))

	return nil
}

// restoreTermsLocked decodes the "terms" table on its own, outside
// restoreTablesLocked's shared dtoReg.RestoreAll, because it must tolerate a
// v1 snapshot predating the terms/ redesign (gopherstack-kxow): those rows
// were {UserPoolID, Text} with no TermsID at all. CreateTerms was unreachable
// by any real SDK client before the redesign (its own required-member
// validation rejects the request client-side), so no real snapshot can
// contain a genuine pre-redesign terms row -- dropping anything that doesn't
// decode into the current shape, or decodes but lacks a TermsID, is correct
// and loses nothing. A raw payload that fails to unmarshal at all (corrupt or
// a shape from some other future change) is likewise dropped rather than
// failing the whole Restore, unlike dtoReg.RestoreAll's other tables. Caller
// must hold b.mu in write mode.
func (b *InMemoryBackend) restoreTermsLocked(ctx context.Context, raw json.RawMessage) {
	if len(raw) == 0 {
		b.terms.Restore(nil)

		return
	}

	var items []*Terms

	if err := json.Unmarshal(raw, &items); err != nil {
		logger.Load(ctx).WarnContext(ctx, "cognitoidp: dropping unparseable terms snapshot", "error", err)
		b.terms.Restore(nil)

		return
	}

	valid := make([]*Terms, 0, len(items))

	for _, t := range items {
		if t != nil && t.TermsID != "" {
			valid = append(valid, t)
		}
	}

	if len(valid) != len(items) {
		logger.Load(ctx).WarnContext(ctx, "cognitoidp: dropped pre-redesign terms rows on restore",
			"total", len(items), "kept", len(valid))
	}

	b.terms.Restore(valid)
}

// restoreRawMapsLocked loads every resource left as a plain map (see
// store_setup.go's registerAllTables doc for why) plus top-level backend
// scalars from snap. Caller must hold b.mu in write mode.
func (b *InMemoryBackend) restoreRawMapsLocked(snap *backendSnapshot) {
	b.refreshTokens = snap.RefreshTokens
	b.refreshTokensByClient = buildRefreshTokensByClientIndex(b.refreshTokens)
	b.refreshTokensByUser = buildRefreshTokensByUserIndex(b.refreshTokens)
	b.groupMembers = snap.GroupMembers
	b.tokenRevokedBefore = snap.TokenRevokedBefore
	b.resourceTags = snap.ResourceTags
	b.riskConfigurations = snap.RiskConfigurations
	b.logDeliveryConfigs = snap.LogDeliveryConfigs
	b.poolMfaConfigs = snap.PoolMfaConfigs
	b.devices = snap.Devices
	b.webauthnCredentials = snap.WebAuthnCredentials
	b.authEvents = snap.AuthEvents
	b.provisionedLimits = snap.ProvisionedLimits
	b.accountID = snap.AccountID
	b.region = snap.Region
	b.endpoint = snap.Endpoint
}

func buildRefreshTokensByClientIndex(
	refreshTokens map[string]*refreshTokenEntry,
) map[string]map[string]struct{} {
	index := make(map[string]map[string]struct{})
	for token, entry := range refreshTokens {
		if index[entry.ClientID] == nil {
			index[entry.ClientID] = make(map[string]struct{})
		}

		index[entry.ClientID][token] = struct{}{}
	}

	return index
}

func buildRefreshTokensByUserIndex(
	refreshTokens map[string]*refreshTokenEntry,
) map[string]map[string]struct{} {
	index := make(map[string]map[string]struct{})
	for token, entry := range refreshTokens {
		key := entry.PoolID + ":" + entry.Username
		if index[key] == nil {
			index[key] = make(map[string]struct{})
		}

		index[key][token] = struct{}{}
	}

	return index
}

func normalizeBackendSnapshot(snap *backendSnapshot) {
	if snap.RefreshTokens == nil {
		snap.RefreshTokens = make(map[string]*refreshTokenEntry)
	}

	defaultExpiry := time.Now().UTC().Add(defaultRefreshTokenTTL)
	for _, entry := range snap.RefreshTokens {
		if entry.ExpiresAt.IsZero() {
			entry.ExpiresAt = defaultExpiry
		}
	}

	if snap.GroupMembers == nil {
		snap.GroupMembers = make(map[string]map[string]map[string]struct{})
	}

	if snap.TokenRevokedBefore == nil {
		snap.TokenRevokedBefore = make(map[string]time.Time)
	}

	if snap.Devices == nil {
		snap.Devices = make(map[string]map[string]*Device)
	}

	if snap.WebAuthnCredentials == nil {
		snap.WebAuthnCredentials = make(map[string]map[string]*WebAuthnCredential)
	}

	if snap.AuthEvents == nil {
		snap.AuthEvents = make(map[string]map[string]*AuthEvent)
	}

	if snap.ProvisionedLimits == nil {
		snap.ProvisionedLimits = make(map[string]int32)
	}
}

// restorePoolsFromSnapshot rebuilds live UserPools (including their RSA token
// issuers) from persisted DTOs.
func restorePoolsFromSnapshot(poolSnapshots []*userPoolSnapshot) ([]*UserPool, error) {
	pools := make([]*UserPool, 0, len(poolSnapshots))

	for _, ps := range poolSnapshots {
		rsaKey, err := unmarshalRSAKey(ps.PrivKeyPEM)
		if err != nil {
			return nil, fmt.Errorf("restoring user pool %q: %w", ps.ID, err)
		}

		createdAt, _ := time.Parse("2006-01-02T15:04:05Z", ps.CreatedAt)

		pool := &UserPool{
			ID:                     ps.ID,
			Name:                   ps.Name,
			ARN:                    ps.ARN,
			CreatedAt:              createdAt,
			CustomAttributes:       ps.CustomAttributes,
			MfaConfiguration:       ps.MfaConfiguration,
			DeletionProtection:     ps.DeletionProtection,
			PasswordPolicy:         ps.PasswordPolicy,
			AutoVerifiedAttributes: ps.AutoVerifiedAttributes,
			LambdaConfig:           ps.LambdaConfig,
			EmailConfiguration:     ps.EmailConfiguration,
			AccountRecoverySetting: ps.AccountRecoverySetting,
		}

		if rsaKey != nil {
			pool.issuer = newTokenIssuerFromKey(rsaKey, ps.KeyID, ps.IssuerURL)
		}

		pools = append(pools, pool)
	}

	return pools, nil
}

// restoreUsersFromSnapshot rebuilds live Users from persisted DTOs.
func restoreUsersFromSnapshot(userSnapshots []*userSnapshot) []*User {
	users := make([]*User, 0, len(userSnapshots))

	for _, us := range userSnapshots {
		createdAt, _ := time.Parse("2006-01-02T15:04:05Z", us.CreatedAt)
		updatedAt, _ := time.Parse("2006-01-02T15:04:05Z", us.UpdatedAt)
		codeExpiry, _ := time.Parse("2006-01-02T15:04:05Z", us.ConfirmCodeExpiresAt)
		lastAuth, _ := time.Parse("2006-01-02T15:04:05Z", us.LastAuthTime)
		tempPasswordIssuedAt, _ := time.Parse("2006-01-02T15:04:05Z", us.TempPasswordIssuedAt)

		if updatedAt.IsZero() {
			updatedAt = createdAt
		}

		users = append(users, &User{
			CreatedAt:            createdAt,
			UpdatedAt:            updatedAt,
			ConfirmCodeExpiresAt: codeExpiry,
			LastAuthTime:         lastAuth,
			TempPasswordIssuedAt: tempPasswordIssuedAt,
			Attributes:           us.Attributes,
			Sub:                  us.Sub,
			Username:             us.Username,
			UserPoolID:           us.UserPoolID,
			PasswordHash:         us.PasswordHash,
			SRPSalt:              us.SRPSalt,
			SRPVerifier:          us.SRPVerifier,
			Status:               us.Status,
			ConfirmCode:          us.ConfirmCode,
			PreferredMfaSetting:  us.PreferredMfaSetting,
			TOTPSecret:           us.TOTPSecret,
			TOTPVerified:         us.TOTPVerified,
			UserMFASettingList:   us.UserMFASettingList,
			MFAOptions:           us.MFAOptions,
			LinkedProviders:      us.LinkedProviders,
			Enabled:              us.Enabled,
		})
	}

	return users
}

// Snapshot implements persistence.Persistable by delegating to the backend.
func (h *Handler) Snapshot(ctx context.Context) []byte { return h.Backend.Snapshot(ctx) }

// Restore implements persistence.Persistable by delegating to the backend.
func (h *Handler) Restore(ctx context.Context, data []byte) error {
	return h.Backend.Restore(ctx, data)
}

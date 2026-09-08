package iam

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/page"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// AWS IAM inline policy size limits (UTF-8 bytes, including whitespace) per
// https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_iam-quotas.html.
const (
	maxUserPolicySize  = 2048
	maxRolePolicySize  = 10240
	maxGroupPolicySize = 5120
)

// StorageBackend defines the interface for the IAM in-memory store.
type StorageBackend interface {
	// Users
	CreateUser(userName, path, permissionsBoundary string) (*User, error)
	DeleteUser(userName string) error
	ListUsers(marker string, maxItems int) (page.Page[User], error)
	GetUser(userName string) (*User, error)

	// Roles
	CreateRole(roleName, path, assumeRolePolicyDocument, permissionsBoundary string) (*Role, error)
	DeleteRole(roleName string) error
	DeleteServiceLinkedRole(roleName string) error
	ListRoles(marker string, maxItems int) (page.Page[Role], error)
	GetRole(roleName string) (*Role, error)
	GetRoleByArn(roleArn string) (*Role, error)
	UpdateRoleMaxSessionDuration(roleName string, maxSessionDuration int32) error

	// Policies
	CreatePolicy(policyName, path, policyDocument string) (*Policy, error)
	DeletePolicy(policyArn string) error
	ListPolicies(marker string, maxItems int) (page.Page[Policy], error)
	PermissionsBoundaryARNs() map[string]bool
	PermissionsBoundaryEntities(policyArn string) (userNames, roleNames []string)
	AttachUserPolicy(userName, policyArn string) error
	DetachUserPolicy(userName, policyArn string) error
	AttachRolePolicy(roleName, policyArn string) error
	DetachRolePolicy(roleName, policyArn string) error
	ListAttachedUserPolicies(userName string) ([]AttachedPolicy, error)
	ListAttachedRolePolicies(roleName string) ([]AttachedPolicy, error)
	GetPolicy(policyArn string) (*Policy, error)
	ListPolicyVersions(policyArn string) ([]StoredPolicyVersion, error)
	GetPolicyVersion(policyArn, versionID string) (*StoredPolicyVersion, error)

	// Inline Policies - Users
	PutUserPolicy(userName, policyName, policyDocument string) error
	GetUserPolicy(userName, policyName string) (string, error)
	DeleteUserPolicy(userName, policyName string) error
	ListUserPolicies(userName string) ([]string, error)

	// Inline Policies - Roles
	PutRolePolicy(roleName, policyName, policyDocument string) error
	GetRolePolicy(roleName, policyName string) (string, error)
	DeleteRolePolicy(roleName, policyName string) error
	ListRolePolicies(roleName string) ([]string, error)

	// Inline Policies - Groups
	PutGroupPolicy(groupName, policyName, policyDocument string) error
	GetGroupPolicy(groupName, policyName string) (string, error)
	DeleteGroupPolicy(groupName, policyName string) error
	ListGroupPolicies(groupName string) ([]string, error)

	// Permission Boundaries
	PutUserPermissionsBoundary(userName, policyArn string) error
	DeleteUserPermissionsBoundary(userName string) error
	PutRolePermissionsBoundary(roleName, policyArn string) error
	DeleteRolePermissionsBoundary(roleName string) error

	// Groups
	CreateGroup(groupName, path string) (*Group, error)
	DeleteGroup(groupName string) error
	GetGroup(groupName string) (*Group, error)
	GetGroupUsers(groupName string) ([]User, error)
	ListGroups(marker string, maxItems int) (page.Page[Group], error)
	AddUserToGroup(groupName, userName string) error
	RemoveUserFromGroup(groupName, userName string) error
	AttachGroupPolicy(groupName, policyArn string) error
	DetachGroupPolicy(groupName, policyArn string) error
	ListAttachedGroupPolicies(groupName string) ([]AttachedPolicy, error)

	// Assume Role Policy
	UpdateAssumeRolePolicy(roleName, policyDocument string) error

	// Reporting and simulation
	GetAccountAuthorizationDetails(marker string, maxItems int, filter []string) (AccountAuthorizationDetails, string)
	SimulatePrincipalPolicy(
		principalArn, callerArn, resourceOwner string,
		resourcePolicyList, actionNames, resourceArns []string,
		ctx ConditionContext,
	) ([]SimulationResult, error)
	GetCredentialReport() string
	GetAccountSummary() AccountSummary

	// Access Keys
	CreateAccessKey(userName string) (*AccessKey, error)
	DeleteAccessKey(userName, accessKeyID string) error
	ListAccessKeys(userName, marker string, maxItems int) (page.Page[AccessKey], error)

	// Instance Profiles
	CreateInstanceProfile(name, path string) (*InstanceProfile, error)
	DeleteInstanceProfile(name string) error
	ListInstanceProfiles(marker string, maxItems int) (page.Page[InstanceProfile], error)
	AddRoleToInstanceProfile(instanceProfileName, roleName string) error
	RemoveRoleFromInstanceProfile(instanceProfileName, roleName string) error

	// SAML Providers
	CreateSAMLProvider(name, samlMetadataDocument string) (*SAMLProvider, error)
	UpdateSAMLProvider(providerArn, samlMetadataDocument string) (*SAMLProvider, error)
	DeleteSAMLProvider(providerArn string) error
	GetSAMLProvider(providerArn string) (*SAMLProvider, error)
	ListSAMLProviders() ([]SAMLProvider, error)

	// OIDC Providers
	CreateOpenIDConnectProvider(
		rawURL string,
		clientIDs, thumbprints []string,
	) (*OIDCProvider, error)
	UpdateOpenIDConnectProviderThumbprint(providerArn string, thumbprints []string) error
	DeleteOpenIDConnectProvider(providerArn string) error
	GetOpenIDConnectProvider(providerArn string) (*OIDCProvider, error)
	ListOpenIDConnectProviders() ([]OIDCProvider, error)

	// Login Profiles
	CreateLoginProfile(userName, password string, passwordResetRequired bool) (*LoginProfile, error)
	UpdateLoginProfile(userName, password string, passwordResetRequired bool) error
	DeleteLoginProfile(userName string) error
	GetLoginProfile(userName string) (*LoginProfile, error)

	// Account Aliases
	CreateAccountAlias(alias string) error
	ListAccountAliases() []string
	DeleteAccountAlias(alias string) error

	// Outbound Web Identity Federation
	EnableOutboundWebIdentityFederation() string
	DisableOutboundWebIdentityFederation()
	GetOutboundWebIdentityFederationInfo() (issuerURL string, enabled bool)
	OutboundWebIdentityFederationEnabled() bool

	// Policy Versions
	CreatePolicyVersion(
		policyArn, policyDocument string,
		setAsDefault bool,
	) (*StoredPolicyVersion, error)
	SetDefaultPolicyVersion(policyArn, versionID string) error
	DeletePolicyVersion(policyArn, versionID string) error

	// Service-Linked Roles
	CreateServiceLinkedRole(awsServiceName, description, customSuffix string) (*Role, error)
	GetServiceLinkedRoleDeletionStatus(deletionTaskID string) (string, error)

	// Service-Specific Credentials
	CreateServiceSpecificCredential(
		userName, serviceName string,
	) (*ServiceSpecificCredential, error)
	ListServiceSpecificCredentials(
		userName, serviceName string,
	) ([]ServiceSpecificCredential, error)
	DeleteServiceSpecificCredential(userName, credentialID string) error
	UpdateServiceSpecificCredential(userName, credentialID, status string) error

	// Virtual MFA Devices
	CreateVirtualMFADevice(virtualMFADeviceName, path string) (*VirtualMFADevice, error)
	CreateVirtualMFADeviceFull(virtualMFADeviceName, path string) (*VirtualMFADevice, error)
	ListVirtualMFADevices(marker string, maxItems int) (page.Page[VirtualMFADevice], error)
	DeleteVirtualMFADevice(serialNumber string) error
	EnableMFADevice(userName, serialNumber, authCode1, authCode2 string) error
	DeactivateMFADevice(userName, serialNumber string) error
	ResyncMFADevice(userName, serialNumber, authCode1, authCode2 string) error
	GetMFADeviceOwner(serialNumber string) string
	GetVirtualMFADevice(serialNumber string) (VirtualMFADevice, string, error)
	ListMFADevicesForUser(userName, marker string, maxItems int) (page.Page[VirtualMFADevice], error)

	// SSH Public Keys
	UploadSSHPublicKey(userName, body string) (*SSHPublicKey, error)
	GetSSHPublicKey(userName, keyID string) (*SSHPublicKey, error)
	ListSSHPublicKeys(userName string, marker string, maxItems int) (page.Page[SSHPublicKey], error)
	UpdateSSHPublicKey(userName, keyID, status string) error
	DeleteSSHPublicKey(userName, keyID string) error

	// Access Advisor
	GenerateServiceLastAccessedDetailsForEntity(entityARN string) string
	GetServiceLastAccessedDetails(
		jobID string,
	) (status string, details []ServiceLastAccessedDetail, err error)
	RecordServiceAccess(entityARN, serviceNamespace, serviceName string)

	// Organizations Access Report
	GenerateOrganizationsAccessReport(entityPath string) string
	GetOrganizationsAccessReport(jobID string) (status string, createdAt time.Time, found bool)

	// Reset service-specific credential password
	ResetServiceSpecificCredentialFull(
		userName, credentialID string,
	) (*ServiceSpecificCredential, error)

	// OIDC provider existence check (implements sts.OIDCLookup)
	OIDCProviderExists(issuerURL string) bool

	// Delegation Requests
	CreateDelegationRequest(req CreateDelegationRequestInput) (*DelegationRequest, error)
	AcceptDelegationRequest(delegationID string) error
	AssociateDelegationRequest(delegationID string) error
	DelegationRequestExists(delegationID string) bool
	GetDelegationRequest(delegationID string) (*DelegationRequest, error)
	ListDelegationRequests(marker string, maxItems int) (page.Page[DelegationRequest], error)
	RejectDelegationRequest(delegationID, notes string) error
	SendDelegationToken(delegationID string) error
	UpdateDelegationRequest(delegationID, notes string) error

	// Security Token Service preferences
	SetSecurityTokenServicePreferences(globalEndpointTokenVersion string) error

	// Change Password
	ChangePassword(oldPassword, newPassword string) error
	ChangePasswordForCaller(accessKeyID, oldPassword, newPassword string) error

	// OIDC Client IDs
	AddClientIDToOpenIDConnectProvider(providerArn, clientID string) error
	RemoveClientIDFromOpenIDConnectProvider(providerArn, clientID string) error

	// Access Key management
	UpdateAccessKey(userName, accessKeyID, status string) error
	GetAccessKeyLastUsed(accessKeyID string) (*AccessKeyLastUsed, error)
	GetUserArnByAccessKeyID(accessKeyID string) (string, error)
	RecordAccessKeyUsage(accessKeyID, region, serviceName string)

	// Tags on resources (embedded in model, returned with resource)
	TagUser(userName string, tags map[string]string) error
	UntagUser(userName string, keys []string) error
	TagRole(roleName string, tags map[string]string) error
	UntagRole(roleName string, keys []string) error
	TagPolicy(policyArn string, tags map[string]string) error
	UntagPolicy(policyArn string, keys []string) error
	// Note: Group is not taggable in real IAM — no TagGroup/UntagGroup here.

	// Signing Certificates
	UploadSigningCertificate(userName, body string) (*SigningCertificate, error)
	ListSigningCertificates(userName, marker string, maxItems int) (page.Page[SigningCertificate], error)
	UpdateSigningCertificate(userName, certificateID, status string) error
	DeleteSigningCertificate(userName, certificateID string) error

	// Server Certificates
	UploadServerCertificate(name, path, certBody, certChain string) (*ServerCertificate, error)
	GetServerCertificate(name string) (*ServerCertificate, error)
	ListServerCertificates(pathPrefix string) ([]ServerCertificate, error)
	UpdateServerCertificate(name, newName, newPath string) error
	DeleteServerCertificate(name string) error

	// Group membership queries
	ListGroupsForUser(userName string) ([]Group, error)

	// Account Password Policy
	GetAccountPasswordPolicy() *PasswordPolicy
	UpdateAccountPasswordPolicy(pp PasswordPolicy) error
	DeleteAccountPasswordPolicy() error

	// Policy entity queries
	ListEntitiesForPolicy(policyArn, entityFilter string) (*PolicyEntities, error)

	// Entity mutations
	UpdateUser(userName, newPath, newUserName string) error
	UpdateRole(roleName, description string) error
	UpdateGroup(groupName, newPath, newGroupName string) error

	// Instance profiles extended
	GetInstanceProfile(name string) (*InstanceProfile, error)
	ListInstanceProfilesForRole(roleName string) ([]InstanceProfile, error)

	// Simulation
	SimulateCustomPolicy(
		policyInputList, permissionsBoundaryPolicyInputList, actionNames, resourceArns []string,
		ctx ConditionContext,
	) ([]SimulationResult, error)

	// Dashboard helpers
	ListAllUsers() []User
	ListAllRoles() []Role
	ListAllPolicies() []Policy
	ListAllGroups() []Group
	ListAllAccessKeys() []AccessKey
	ListAllInstanceProfiles() []InstanceProfile

	// Enforcement helpers
	GetUserByAccessKeyID(accessKeyID string) (*User, error)
	GetPoliciesForUser(userName string) ([]string, error)

	Purge(ctx context.Context, cutoff time.Time)
}

// iamDefaultMaxItems is the default page size for IAM list operations.
const iamDefaultMaxItems = 100

// accessKeyStatusActive is the "Active" access-key status string.
const accessKeyStatusActive = "Active"

// globalEndpointTokenVersionV1/V2 are SetSecurityTokenServicePreferences's
// GlobalEndpointTokenVersion values (aws-sdk-go-v2/service/iam/types.GlobalEndpointTokenVersion).
const (
	globalEndpointTokenVersionV1 = "v1Token"
	globalEndpointTokenVersionV2 = "v2Token"
)

// InMemoryBackend implements StorageBackend using in-memory maps.
type InMemoryBackend struct {
	rolePolicies               map[string][]string
	loginProfiles              *store.Table[LoginProfile]
	policies                   *store.Table[Policy]
	policyByARN                map[string]string
	roleByARN                  map[string]string
	accessKeys                 *store.Table[AccessKey]
	userAccessKeys             map[string][]string
	instanceProfiles           *store.Table[InstanceProfile]
	samlProviders              *store.Table[SAMLProvider]
	groupMembers               map[string][]string
	groupPolicies              map[string][]string
	userPolicies               map[string][]string
	policyAttachments          map[string]policyAttachmentRefs
	mu                         *lockmetrics.RWMutex
	groupInlinePolicies        map[string]map[string]string
	groups                     *store.Table[Group]
	oidcProviders              *store.Table[OIDCProvider]
	userInlinePolicies         map[string]map[string]string
	delegationRequests         *store.Table[DelegationRequest]
	roleInlinePolicies         map[string]map[string]string
	virtualMFADevices          *store.Table[VirtualMFADevice]
	policyVersions             map[string][]StoredPolicyVersion
	policyVersionCounters      map[string]int
	deletedV1Policies          map[string]bool
	serviceSpecificCreds       *store.Table[ServiceSpecificCredential]
	roles                      *store.Table[Role]
	signingCertificates        *store.Table[SigningCertificate]
	serverCertificates         *store.Table[ServerCertificate]
	passwordPolicy             *PasswordPolicy
	users                      *store.Table[User]
	registry                   *store.Registry
	comprehensive              *comprehensiveBackend
	accountID                  string
	currentPassword            string
	globalEndpointTokenVersion string
	accountAliases             []string
	currentPasswordHistory     []string
	sortedUserNames            []string
	sortedRoleNames            []string
	sortedPolicyNames          []string
	sortedGroupNames           []string
	sortedIPNames              []string
	outboundFederationEnabled  bool
}

type policyAttachmentRefs struct {
	users  map[string]struct{}
	roles  map[string]struct{}
	groups map[string]struct{}
}

// NewInMemoryBackend creates a new empty IAM InMemoryBackend with default account ID.
func NewInMemoryBackend() *InMemoryBackend {
	return NewInMemoryBackendWithConfig(IAMAccountID)
}

// NewInMemoryBackendWithConfig creates a new IAM InMemoryBackend with the given account ID.
func NewInMemoryBackendWithConfig(accountID string) *InMemoryBackend {
	b := &InMemoryBackend{
		roleByARN:                  make(map[string]string),
		policyByARN:                make(map[string]string),
		userAccessKeys:             make(map[string][]string),
		userPolicies:               make(map[string][]string),
		rolePolicies:               make(map[string][]string),
		groupPolicies:              make(map[string][]string),
		groupMembers:               make(map[string][]string),
		userInlinePolicies:         make(map[string]map[string]string),
		roleInlinePolicies:         make(map[string]map[string]string),
		groupInlinePolicies:        make(map[string]map[string]string),
		policyAttachments:          make(map[string]policyAttachmentRefs),
		accountAliases:             nil,
		policyVersions:             make(map[string][]StoredPolicyVersion),
		policyVersionCounters:      make(map[string]int),
		deletedV1Policies:          make(map[string]bool),
		accountID:                  accountID,
		mu:                         lockmetrics.New("iam"),
		registry:                   store.NewRegistry(),
		comprehensive:              newComprehensiveBackend(),
		outboundFederationEnabled:  true,
		globalEndpointTokenVersion: globalEndpointTokenVersionV1,
		// Sorted name indexes start empty; populated via insertSorted on create.
		sortedUserNames:   nil,
		sortedRoleNames:   nil,
		sortedPolicyNames: nil,
		sortedGroupNames:  nil,
		sortedIPNames:     nil,
	}

	registerAllTables(b)

	return b
}

// normPath returns a normalized IAM path, defaulting to "/" if empty.
// Non-root paths are ensured to end with "/" so that ARN construction
// produces the correct "resource/path/name" form.
func normPath(path string) string {
	if path == "" {
		return "/"
	}

	if !strings.HasSuffix(path, "/") {
		return path + "/"
	}

	return path
}

func (b *InMemoryBackend) addPolicyAttachmentLocked(policyArn, entityName, entityType string) {
	refs := b.policyAttachments[policyArn]
	if refs.users == nil {
		refs.users = make(map[string]struct{})
	}

	if refs.roles == nil {
		refs.roles = make(map[string]struct{})
	}

	if refs.groups == nil {
		refs.groups = make(map[string]struct{})
	}

	switch entityType {
	case "user":
		refs.users[entityName] = struct{}{}
	case "role":
		refs.roles[entityName] = struct{}{}
	case "group":
		refs.groups[entityName] = struct{}{}
	}

	b.policyAttachments[policyArn] = refs

	if polName, ok := b.policyByARN[policyArn]; ok {
		if pol, ok2 := b.policies.Get(polName); ok2 {
			pol.AttachmentCount = len(refs.users) + len(refs.roles) + len(refs.groups)
			b.policies.Put(pol)
		}
	}
}

func (b *InMemoryBackend) removePolicyAttachmentLocked(policyArn, entityName, entityType string) {
	refs, exists := b.policyAttachments[policyArn]
	if !exists {
		return
	}

	switch entityType {
	case "user":
		delete(refs.users, entityName)
	case "role":
		delete(refs.roles, entityName)
	case "group":
		delete(refs.groups, entityName)
	}

	if len(refs.users) == 0 && len(refs.roles) == 0 && len(refs.groups) == 0 {
		delete(b.policyAttachments, policyArn)

		if polName, ok := b.policyByARN[policyArn]; ok {
			if pol, ok2 := b.policies.Get(polName); ok2 {
				pol.AttachmentCount = 0
				b.policies.Put(pol)
			}
		}

		return
	}

	b.policyAttachments[policyArn] = refs

	if polName, ok := b.policyByARN[policyArn]; ok {
		if pol, ok2 := b.policies.Get(polName); ok2 {
			pol.AttachmentCount = len(refs.users) + len(refs.roles) + len(refs.groups)
			b.policies.Put(pol)
		}
	}
}

// renamePolicyAttachmentsLocked moves entityType's key from oldName to newName
// in the reverse policyAttachments index (policyArn -> attached entity
// names), for every policy ARN in policyARNs. Must be called whenever a user
// or group is renamed (UpdateUser / UpdateGroup) so that DeletePolicy's
// attachment check, ListEntitiesForPolicy, and DetachUserPolicy/
// DetachGroupPolicy keyed by the NEW name continue to find the attachment —
// otherwise the reverse index keeps referencing the entity's old name forever
// (a ghost attachment that can neither be detached under the new name nor
// ever clears, permanently blocking DeletePolicy with a stale DeleteConflict).
func (b *InMemoryBackend) renamePolicyAttachmentsLocked(entityType, oldName, newName string, policyARNs []string) {
	for _, policyArn := range policyARNs {
		b.removePolicyAttachmentLocked(policyArn, oldName, entityType)
		b.addPolicyAttachmentLocked(policyArn, newName, entityType)
	}
}

// firstKey returns an arbitrary key from a set-like map, or an empty string if the map is empty.
func firstKey(values map[string]struct{}) string {
	for value := range values {
		return value
	}

	return ""
}

func (b *InMemoryBackend) getPolicyByARNLocked(policyArn string) (Policy, bool) {
	policyName, exists := b.policyByARN[policyArn]
	if !exists {
		return Policy{}, false
	}

	pol, exists := b.policies.Get(policyName)
	if !exists {
		return Policy{}, false
	}

	return *pol, true
}

func (b *InMemoryBackend) rebuildIndexesLocked() {
	b.roleByARN = make(map[string]string, b.roles.Len())
	for _, role := range b.roles.All() {
		b.roleByARN[role.Arn] = role.RoleName
	}

	b.policyByARN = make(map[string]string, b.policies.Len())
	for _, policy := range b.policies.All() {
		b.policyByARN[policy.Arn] = policy.PolicyName
	}

	b.policyAttachments = make(map[string]policyAttachmentRefs)
	for userName, policyARNs := range b.userPolicies {
		for _, policyARN := range policyARNs {
			b.addPolicyAttachmentLocked(policyARN, userName, "user")
		}
	}

	for roleName, policyARNs := range b.rolePolicies {
		for _, policyARN := range policyARNs {
			b.addPolicyAttachmentLocked(policyARN, roleName, "role")
		}
	}

	for groupName, policyARNs := range b.groupPolicies {
		for _, policyARN := range policyARNs {
			b.addPolicyAttachmentLocked(policyARN, groupName, "group")
		}
	}

	// Rebuild sorted name indexes.
	b.sortedUserNames = sortedTableNames(b.users, usersKeyFn)
	b.sortedRoleNames = sortedTableNames(b.roles, rolesKeyFn)
	b.sortedPolicyNames = sortedTableNames(b.policies, policiesKeyFn)
	b.sortedGroupNames = sortedTableNames(b.groups, groupsKeyFn)
	b.sortedIPNames = sortedTableNames(b.instanceProfiles, instanceProfilesKeyFn)
}

func sortedUsers(t *store.Table[User]) []User {
	// t.Snapshot() is already ordered by key ascending, and the key is
	// UserName (see usersKeyFn), so this matches the prior sort.Slice by
	// UserName exactly.
	items := t.Snapshot()
	users := make([]User, 0, len(items))

	for _, u := range items {
		users = append(users, *u)
	}

	return users
}

// idAlphabet is the set of characters used in AWS IAM entity IDs:
// uppercase letters A–Z and digits 0–9 (no lowercase, no dashes).
const idAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// randomAlphanumString returns a cryptographically random uppercase
// alphanumeric string of length n using idAlphabet.
func randomAlphanumString(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// Fall back to UUID-derived string on entropy failure (should never happen).
		id := uuid.New().String()
		id = strings.ToUpper(strings.ReplaceAll(id, "-", ""))
		if len(id) >= n {
			return id[:n]
		}

		return id
	}

	result := make([]byte, n)
	for i, raw := range b {
		result[i] = idAlphabet[int(raw)%len(idAlphabet)]
	}

	return string(result)
}

// iamEntityIDSuffixLen is the length of the random suffix in IAM entity IDs (e.g. AIDA, AROA).
const iamEntityIDSuffixLen = 16

// newID generates a short unique identifier with the given prefix.
// The suffix is 16 uppercase alphanumeric characters matching AWS IAM ID format.
func newID(prefix string) string {
	return prefix + randomAlphanumString(iamEntityIDSuffixLen)
}

// newAccessKeyID generates a 20-character access key ID matching AWS format:
// "AKIA" followed by 16 uppercase alphanumeric characters.
func newAccessKeyID() string {
	return "AKIA" + randomAlphanumString(iamEntityIDSuffixLen)
}

// secretKeyBytes is the number of random bytes to generate for a secret access key.
// 30 random bytes produce exactly 40 standard base64 characters (30 * 8/6 = 40).
const secretKeyBytes = 30

// newSecretAccessKey generates a cryptographically secure 40-character secret access key.
// It uses 30 random bytes encoded as standard base64, which produces exactly 40 characters.
func newSecretAccessKey() (string, error) {
	b := make([]byte, secretKeyBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("iam: generate secret access key: %w", err)
	}

	return base64.StdEncoding.EncodeToString(b), nil
}

// Reset clears all in-memory state from the backend. It is used by the
// POST /_gopherstack/reset endpoint for CI pipelines and rapid local development.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	b.registry.ResetAll()
	b.roleByARN = make(map[string]string)
	b.policyByARN = make(map[string]string)
	b.policyAttachments = make(map[string]policyAttachmentRefs)
	b.userAccessKeys = make(map[string][]string)
	b.userPolicies = make(map[string][]string)
	b.rolePolicies = make(map[string][]string)
	b.groupPolicies = make(map[string][]string)
	b.groupMembers = make(map[string][]string)
	b.userInlinePolicies = make(map[string]map[string]string)
	b.roleInlinePolicies = make(map[string]map[string]string)
	b.groupInlinePolicies = make(map[string]map[string]string)
	b.accountAliases = nil
	b.globalEndpointTokenVersion = globalEndpointTokenVersionV1
	b.policyVersions = make(map[string][]StoredPolicyVersion)
	b.policyVersionCounters = make(map[string]int)
	b.deletedV1Policies = make(map[string]bool)
	b.sortedUserNames = nil
	b.sortedRoleNames = nil
	b.sortedPolicyNames = nil
	b.sortedGroupNames = nil
	b.sortedIPNames = nil
	b.passwordPolicy = nil
	b.currentPassword = ""
	b.currentPasswordHistory = nil
	b.outboundFederationEnabled = true
	b.resetComprehensiveLocked()
}

// Purge removes all resources older than the given cutoff time.
func (b *InMemoryBackend) Purge(ctx context.Context, cutoff time.Time) {
	if ctx.Err() != nil {
		return
	}

	b.mu.Lock("Purge")
	defer b.mu.Unlock()

	b.purgeUsersLocked(cutoff)
	b.purgeRolesLocked(cutoff)
	b.purgePoliciesLocked(cutoff)
	b.purgeGroupsLocked(cutoff)
	b.purgeAccessKeysLocked(cutoff)
	b.purgeInstanceProfilesLocked(cutoff)
	b.purgeSAMLProvidersLocked(cutoff)
	b.purgeOIDCProvidersLocked(cutoff)
	b.rebuildIndexesLocked()
}

// insertSorted inserts v into a sorted []string slice in lexicographic order.
// Uses binary search for O(log n) position, O(n) element shift.
func insertSorted(s []string, v string) []string {
	i, _ := slices.BinarySearch(s, v)

	return slices.Insert(s, i, v)
}

// deleteSorted removes the first occurrence of v from a sorted []string slice.
// Uses binary search; no-op if v is not present.
func deleteSorted(s []string, v string) []string {
	i, found := slices.BinarySearch(s, v)
	if !found {
		return s
	}

	return slices.Delete(s, i, i+1)
}

// pageFromSortedNames paginates over a pre-sorted name slice, building the value page
// by looking up each name via lookup (typically a [store.Table.Get] method value).
// Marker is a base64-encoded integer index (opaque to callers) enabling O(1) position
// resolution without scanning all names.
func pageFromSortedNames[T any](
	names []string,
	lookup func(string) (*T, bool),
	marker string,
	limit, defaultLimit int,
) page.Page[T] {
	if limit <= 0 {
		limit = defaultLimit
	}

	start := page.DecodeToken(marker)
	if start >= len(names) {
		return page.Page[T]{Data: []T{}}
	}

	end := start + limit
	var next string

	if end < len(names) {
		next = page.EncodeToken(end)
	} else {
		end = len(names)
	}

	window := names[start:end]
	data := make([]T, 0, len(window))

	for _, name := range window {
		if v, ok := lookup(name); ok {
			data = append(data, *v)
		}
	}

	return page.Page[T]{Data: data, Next: next}
}

// comprehensiveBackend stores SSH keys, MFA user links, access advisor jobs, and
// service-last-accessed data. It has no lock of its own: all its fields are guarded
// by the owning InMemoryBackend's coarse b.mu, same as every other backend map (see
// the locking rule in .claude/memories/pkgs-catalog.md). Every method/field access
// below must happen with b.mu held (read or write, as appropriate) by the caller.
type comprehensiveBackend struct {
	// SSH public keys: keyID → SSHPublicKey
	sshPublicKeys map[string]SSHPublicKey

	// MFA device to user link: serialNumber → userName (empty = unassigned)
	mfaUserLinks map[string]string

	// Access advisor jobs: jobID → job
	accessAdvisorJobs map[string]*accessAdvisorJob

	// Service last accessed: entityARN → service namespace → detail
	serviceLastAccessed map[string]map[string]ServiceLastAccessedDetail

	// Org access report jobs: jobID → creation time
	orgReportJobs map[string]time.Time
}

func newComprehensiveBackend() *comprehensiveBackend {
	return &comprehensiveBackend{
		sshPublicKeys:       make(map[string]SSHPublicKey),
		mfaUserLinks:        make(map[string]string),
		accessAdvisorJobs:   make(map[string]*accessAdvisorJob),
		serviceLastAccessed: make(map[string]map[string]ServiceLastAccessedDetail),
		orgReportJobs:       make(map[string]time.Time),
	}
}

// comprehensiveSnapshot is the serializable snapshot of comprehensiveBackend
// state, embedded in backendSnapshot.Comprehensive so SSH public keys, MFA
// user links, access advisor jobs, service-last-accessed data, and org report
// jobs survive a persistence restore rather than being silently dropped.
type comprehensiveSnapshot struct {
	SSHPublicKeys       map[string]SSHPublicKey                         `json:"sshPublicKeys,omitempty"`
	MFAUserLinks        map[string]string                               `json:"mfaUserLinks,omitempty"`
	AccessAdvisorJobs   map[string]*accessAdvisorJob                    `json:"accessAdvisorJobs,omitempty"`
	ServiceLastAccessed map[string]map[string]ServiceLastAccessedDetail `json:"serviceLastAccessed,omitempty"`
	OrgReportJobs       map[string]time.Time                            `json:"orgReportJobs,omitempty"`
}

// snapshot returns a deep copy of the comprehensive backend's state for
// persistence. Caller must hold b.mu (read or write).
func (c *comprehensiveBackend) snapshot() comprehensiveSnapshot {
	return comprehensiveSnapshot{
		SSHPublicKeys:       maps.Clone(c.sshPublicKeys),
		MFAUserLinks:        maps.Clone(c.mfaUserLinks),
		AccessAdvisorJobs:   maps.Clone(c.accessAdvisorJobs),
		ServiceLastAccessed: maps.Clone(c.serviceLastAccessed),
		OrgReportJobs:       maps.Clone(c.orgReportJobs),
	}
}

// restore replaces the comprehensive backend's state from a snapshot.
// Caller must hold b.mu (write).
func (c *comprehensiveBackend) restore(snap comprehensiveSnapshot) {
	c.sshPublicKeys = snap.SSHPublicKeys
	if c.sshPublicKeys == nil {
		c.sshPublicKeys = make(map[string]SSHPublicKey)
	}

	c.mfaUserLinks = snap.MFAUserLinks
	if c.mfaUserLinks == nil {
		c.mfaUserLinks = make(map[string]string)
	}

	c.accessAdvisorJobs = snap.AccessAdvisorJobs
	if c.accessAdvisorJobs == nil {
		c.accessAdvisorJobs = make(map[string]*accessAdvisorJob)
	}

	c.serviceLastAccessed = snap.ServiceLastAccessed
	if c.serviceLastAccessed == nil {
		c.serviceLastAccessed = make(map[string]map[string]ServiceLastAccessedDetail)
	}

	c.orgReportJobs = snap.OrgReportJobs
	if c.orgReportJobs == nil {
		c.orgReportJobs = make(map[string]time.Time)
	}
}

// comp returns the comprehensiveBackend associated with this InMemoryBackend.
// It is always non-nil because NewInMemoryBackendWithConfig initialises it in
// the constructor, and it is never reassigned afterward (resetComprehensiveLocked
// clears its fields in place rather than replacing the pointer). A lazy
// nil-check-then-assign here would be a data race with no benefit, since the
// field is already guaranteed non-nil for the object's entire lifetime.
// Every field read/write on the returned value must happen with b.mu held.
func (b *InMemoryBackend) comp() *comprehensiveBackend {
	return b.comprehensive
}

// ResetComprehensiveBackend clears all comprehensive backend state.
// Exported for direct test use; Reset() calls resetComprehensiveLocked
// instead since it already holds b.mu.
func (b *InMemoryBackend) ResetComprehensiveBackend() {
	b.mu.Lock("ResetComprehensiveBackend")
	defer b.mu.Unlock()

	b.resetComprehensiveLocked()
}

// resetComprehensiveLocked clears all comprehensive backend state.
// Caller must hold b.mu (write).
func (b *InMemoryBackend) resetComprehensiveLocked() {
	c := b.comp()
	c.sshPublicKeys = make(map[string]SSHPublicKey)
	c.mfaUserLinks = make(map[string]string)
	c.accessAdvisorJobs = make(map[string]*accessAdvisorJob)
	c.serviceLastAccessed = make(map[string]map[string]ServiceLastAccessedDetail)
	c.orgReportJobs = make(map[string]time.Time)
}

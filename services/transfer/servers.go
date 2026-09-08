package transfer

import (
	"fmt"
	"maps"
	"net/http"
	"slices"
	"sort"
	"time"

	"github.com/google/uuid"
)

// CreateServerInput holds all optional fields for CreateServer.
type CreateServerInput struct {
	IdentityProviderDetails       *IdentityProviderDetails
	EndpointDetails               *EndpointDetails
	ProtocolDetails               *ProtocolDetails
	WorkflowDetails               *WorkflowDetails
	S3StorageOptions              *S3StorageOptions
	Protocols                     []string
	Tags                          map[string]string
	IdentityProviderType          string
	EndpointType                  string
	LoggingRole                   string
	PreAuthenticationLoginBanner  string
	PostAuthenticationLoginBanner string
	HostKey                       string
	Certificate                   string
	Domain                        string
	SecurityPolicyName            string
	IPAddressType                 string
	StructuredLogDestinations     []string
}

// CreateServer creates a new Transfer Family server.
func (b *InMemoryBackend) CreateServer(
	protocols []string,
	tags map[string]string,
) (*Server, error) {
	return b.CreateServerFull(&CreateServerInput{
		Protocols: protocols,
		Tags:      tags,
	})
}

// CreateServerFull creates a new Transfer Family server with full configuration.
// validateAndDefaultServerProtocols validates protocols and returns the default if empty.
func validateAndDefaultServerProtocols(protocols []string) ([]string, error) {
	if len(protocols) == 0 {
		return []string{protocolSFTP}, nil
	}

	for _, p := range protocols {
		switch p {
		case protocolSFTP, "FTP", "FTPS", "AS2":
			// valid
		default:
			return nil, fmt.Errorf("%w: %s", ErrInvalidProtocol, p)
		}
	}

	return protocols, nil
}

// validateAndDefaultIdentityProviderType validates and defaults the identity provider type.
func validateAndDefaultIdentityProviderType(t string) (string, error) {
	if t == "" {
		return identityProviderServiceManaged, nil
	}

	switch t {
	case identityProviderServiceManaged, identityProviderAPIGateway,
		identityProviderDirectoryService, identityProviderLambda:
		return t, nil
	default:
		return "", fmt.Errorf(
			"%w: IdentityProviderType must be one of SERVICE_MANAGED, API_GATEWAY,"+
				" AWS_DIRECTORY_SERVICE, AWS_LAMBDA, got %q",
			ErrValidation, t,
		)
	}
}

// validateAndDefaultDomain validates and defaults the domain.
func validateAndDefaultDomain(domain string) (string, error) {
	if domain == "" {
		return "S3", nil
	}

	switch domain {
	case "S3", "EFS":
		return domain, nil
	default:
		return "", fmt.Errorf("%w: Domain must be S3 or EFS, got %q", ErrValidation, domain)
	}
}

// validateAndDefaultEndpointType validates and defaults the endpoint type.
func validateAndDefaultEndpointType(t string) (string, error) {
	if t == "" {
		return endpointTypePublic, nil
	}

	switch t {
	case endpointTypePublic, endpointTypeVPC, endpointTypeVPCEndpoint:
		return t, nil
	default:
		return "", fmt.Errorf(
			"%w: EndpointType must be PUBLIC, VPC, or VPC_ENDPOINT, got %q",
			ErrValidation, t,
		)
	}
}

// validateProtocolDetails validates the protocol details TLS mode.
func validateProtocolDetails(pd *ProtocolDetails) error {
	if pd == nil || pd.TLSSessionResumptionMode == "" {
		return nil
	}

	switch pd.TLSSessionResumptionMode {
	case "DISABLED", "ENABLED", "ENFORCED":
		return nil
	default:
		return fmt.Errorf(
			"%w: TlsSessionResumptionMode must be DISABLED, ENABLED, or ENFORCED, got %q",
			ErrValidation, pd.TLSSessionResumptionMode,
		)
	}
}

func (b *InMemoryBackend) CreateServerFull(in *CreateServerInput) (*Server, error) {
	b.mu.Lock("CreateServer")
	defer b.mu.Unlock()

	serverID := "s-" + uuid.NewString()[:20]

	protocols, err := validateAndDefaultServerProtocols(in.Protocols)
	if err != nil {
		return nil, err
	}

	identityProviderType, err := validateAndDefaultIdentityProviderType(in.IdentityProviderType)
	if err != nil {
		return nil, err
	}

	domain, err := validateAndDefaultDomain(in.Domain)
	if err != nil {
		return nil, err
	}

	endpointType, err := validateAndDefaultEndpointType(in.EndpointType)
	if err != nil {
		return nil, err
	}

	if pdErr := validateProtocolDetails(in.ProtocolDetails); pdErr != nil {
		return nil, pdErr
	}

	merged := make(map[string]string, len(in.Tags))
	maps.Copy(merged, in.Tags)

	// AWS creates servers OFFLINE; StartServer is required to bring them
	// ONLINE. See https://docs.aws.amazon.com/transfer/latest/userguide/create-server.html.
	s := &Server{
		ServerID:                      serverID,
		State:                         serverStatusOffline,
		Endpoint:                      fmt.Sprintf("%s.server.transfer.%s.amazonaws.com", serverID, b.region),
		Protocols:                     protocols,
		Domain:                        domain,
		IdentityProviderType:          identityProviderType,
		EndpointType:                  endpointType,
		LoggingRole:                   in.LoggingRole,
		PreAuthenticationLoginBanner:  in.PreAuthenticationLoginBanner,
		PostAuthenticationLoginBanner: in.PostAuthenticationLoginBanner,
		HostKey:                       in.HostKey,
		Certificate:                   in.Certificate,
		SecurityPolicyName:            in.SecurityPolicyName,
		IPAddressType:                 in.IPAddressType,
		IdentityProviderDetails:       in.IdentityProviderDetails,
		EndpointDetails:               in.EndpointDetails,
		ProtocolDetails:               in.ProtocolDetails,
		WorkflowDetails:               in.WorkflowDetails,
		S3StorageOptions:              in.S3StorageOptions,
		StructuredLogDestinations:     in.StructuredLogDestinations,
		CreatedAt:                     time.Now(),
		Tags:                          merged,
		AccountID:                     b.accountID,
		Region:                        b.region,
	}
	b.servers.Put(s)
	b.initTagsStore(serverARN(b.accountID, b.region, serverID), merged)

	return cloneServer(s), nil
}

// DescribeServer returns the server with the given ID.
func (b *InMemoryBackend) DescribeServer(serverID string) (*Server, error) {
	b.mu.RLock("DescribeServer")
	defer b.mu.RUnlock()

	s, ok := b.servers.Get(serverID)
	if !ok {
		return nil, fmt.Errorf("%w: server %s not found", ErrServerNotFound, serverID)
	}

	return cloneServer(s), nil
}

// ServerUserCount returns the number of users on the given server.
// Returns 0 if the server does not exist.
func (b *InMemoryBackend) ServerUserCount(serverID string) int {
	b.mu.RLock("ServerUserCount")
	defer b.mu.RUnlock()

	return len(b.usersByServer.Get(serverID))
}

// ListServers returns all servers sorted by ServerId (ascending, deterministic).
func (b *InMemoryBackend) ListServers() []Server {
	b.mu.RLock("ListServers")
	defer b.mu.RUnlock()

	all := b.servers.All()
	out := make([]Server, 0, len(all))

	for _, s := range all {
		out = append(out, *cloneServer(s))
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].ServerID < out[j].ServerID
	})

	return out
}

// DeleteServer removes a server and all of its associated resources (users, accesses, agreements,
// SSH keys, and host keys). The server must be OFFLINE; returns ErrServerOnline otherwise.
func (b *InMemoryBackend) DeleteServer(serverID string) error {
	b.mu.Lock("DeleteServer")
	defer b.mu.Unlock()

	s, ok := b.servers.Get(serverID)
	if !ok {
		return fmt.Errorf("%w: server %s not found", ErrServerNotFound, serverID)
	}

	// AWS requires the server to be OFFLINE before deletion.
	// STOPPING is also accepted — server is already transitioning offline.
	if s.State == serverStatusOnline || s.State == serverStatusStarting {
		return fmt.Errorf("%w: server %s is in state %s", ErrServerOnline, serverID, s.State)
	}

	b.servers.Delete(serverID)
	delete(b.tagsStore, serverARN(b.accountID, b.region, serverID))

	for _, u := range slices.Clone(b.usersByServer.Get(serverID)) {
		b.users.Delete(userKey(u.ServerID, u.UserName))
		delete(b.tagsStore, userARN(b.accountID, b.region, u.ServerID, u.UserName))
	}

	for _, a := range slices.Clone(b.accessesByServer.Get(serverID)) {
		b.accesses.Delete(accessKey(a.ServerID, a.ExternalID))
	}

	for _, ag := range slices.Clone(b.agreementsByServer.Get(serverID)) {
		b.agreements.Delete(agreementKey(ag.ServerID, ag.AgreementID))
		delete(b.tagsStore, agreementARN(b.accountID, b.region, ag.ServerID, ag.AgreementID))
	}

	for _, k := range slices.Clone(b.sshKeysByServer.Get(serverID)) {
		b.sshPublicKeys.Delete(sshPublicKeyKey(k.ServerID, k.UserName, k.SSHPublicKeyID))
	}

	delete(b.sshKeyBodies, serverID)

	for _, hk := range slices.Clone(b.hostKeysByServer.Get(serverID)) {
		b.hostKeys.Delete(hostKeyKey(hk.ServerID, hk.HostKeyID))
		delete(b.tagsStore, hostKeyARN(b.accountID, b.region, hk.ServerID, hk.HostKeyID))
	}

	return nil
}

// startServerTransitionDelay is the async delay before a server reaches ONLINE/OFFLINE state.
const startServerTransitionDelay = 100 * time.Millisecond

// StartServer transitions a server to ONLINE state (via STARTING).
// Calling StartServer on an already-ONLINE server is idempotent (no-op, no error).
func (b *InMemoryBackend) StartServer(serverID string) error {
	b.mu.Lock("StartServer")
	defer b.mu.Unlock()

	s, ok := b.servers.Get(serverID)
	if !ok {
		return fmt.Errorf("%w: server %s not found", ErrServerNotFound, serverID)
	}

	// Idempotent: already ONLINE is a no-op.
	if s.State == serverStatusOnline {
		return nil
	}

	// Set to STARTING, then transition to ONLINE asynchronously.
	s.State = serverStatusStarting

	b.work.After("StartServerTransition", startServerTransitionDelay, func() {
		b.mu.Lock("StartServer-async")
		defer b.mu.Unlock()

		if sv, found := b.servers.Get(serverID); found && sv.State == serverStatusStarting {
			sv.State = serverStatusOnline
		}
	})

	return nil
}

// StopServer transitions a server to OFFLINE state (via STOPPING).
// Calling StopServer on an already-OFFLINE server is idempotent (no-op, no error).
func (b *InMemoryBackend) StopServer(serverID string) error {
	b.mu.Lock("StopServer")
	defer b.mu.Unlock()

	s, ok := b.servers.Get(serverID)
	if !ok {
		return fmt.Errorf("%w: server %s not found", ErrServerNotFound, serverID)
	}

	// Idempotent: already OFFLINE is a no-op.
	if s.State == serverStatusOffline {
		return nil
	}

	// Set to STOPPING, then transition to OFFLINE asynchronously.
	s.State = serverStatusStopping

	b.work.After("StopServerTransition", startServerTransitionDelay, func() {
		b.mu.Lock("StopServer-async")
		defer b.mu.Unlock()

		if sv, found := b.servers.Get(serverID); found && sv.State == serverStatusStopping {
			sv.State = serverStatusOffline
		}
	})

	return nil
}

// UpdateServerInput holds all optional fields for UpdateServer.
type UpdateServerInput struct {
	IdentityProviderDetails       *IdentityProviderDetails
	EndpointDetails               *EndpointDetails
	ProtocolDetails               *ProtocolDetails
	WorkflowDetails               *WorkflowDetails
	S3StorageOptions              *S3StorageOptions
	SecurityPolicyName            string
	ServerID                      string
	Certificate                   string
	EndpointType                  string
	HostKey                       string
	LoggingRole                   string
	PreAuthenticationLoginBanner  string
	PostAuthenticationLoginBanner string
	IPAddressType                 string
	StructuredLogDestinations     []string
	Protocols                     []string
	SetLoggingRole                bool
	SetIdentityProviderDetails    bool
	SetCertificate                bool
	SetPreAuthBanner              bool
	SetPostAuthBanner             bool
	SetSecurityPolicyName         bool
	SetIPAddressType              bool
	SetEndpointType               bool
	SetEndpointDetails            bool
	SetProtocolDetails            bool
	SetWorkflowDetails            bool
	SetS3StorageOptions           bool
	SetStructuredLogDestinations  bool
	SetHostKey                    bool
}

// UpdateServer updates mutable fields on an existing server.
func (b *InMemoryBackend) UpdateServer(serverID string, protocols []string) (*Server, error) {
	return b.UpdateServerFull(&UpdateServerInput{
		ServerID:  serverID,
		Protocols: protocols,
	})
}

// applyServerStringFields applies optional string updates to a server.
func applyServerStringFields(s *Server, in *UpdateServerInput) {
	if len(in.Protocols) > 0 {
		s.Protocols = in.Protocols
	}

	if in.SetCertificate {
		s.Certificate = in.Certificate
	}

	if in.SetEndpointType && in.EndpointType != "" {
		s.EndpointType = in.EndpointType
	}

	if in.SetLoggingRole {
		s.LoggingRole = in.LoggingRole
	}

	if in.SetPreAuthBanner {
		s.PreAuthenticationLoginBanner = in.PreAuthenticationLoginBanner
	}

	if in.SetPostAuthBanner {
		s.PostAuthenticationLoginBanner = in.PostAuthenticationLoginBanner
	}

	if in.SetSecurityPolicyName {
		s.SecurityPolicyName = in.SecurityPolicyName
	}

	if in.SetIPAddressType {
		s.IPAddressType = in.IPAddressType
	}

	if in.SetHostKey {
		s.HostKey = in.HostKey
	}
}

// applyServerStructFields applies optional struct updates to a server.
func applyServerStructFields(s *Server, in *UpdateServerInput) {
	if in.SetIdentityProviderDetails {
		s.IdentityProviderDetails = in.IdentityProviderDetails
	}

	if in.SetEndpointDetails {
		s.EndpointDetails = in.EndpointDetails
	}

	if in.SetProtocolDetails {
		s.ProtocolDetails = in.ProtocolDetails
	}

	if in.SetWorkflowDetails {
		s.WorkflowDetails = in.WorkflowDetails
	}

	if in.SetS3StorageOptions {
		s.S3StorageOptions = in.S3StorageOptions
	}

	if in.SetStructuredLogDestinations {
		s.StructuredLogDestinations = in.StructuredLogDestinations
	}
}

// UpdateServerFull updates all mutable fields on an existing server.
func (b *InMemoryBackend) UpdateServerFull(in *UpdateServerInput) (*Server, error) {
	b.mu.Lock("UpdateServer")
	defer b.mu.Unlock()

	s, ok := b.servers.Get(in.ServerID)
	if !ok {
		return nil, fmt.Errorf("%w: server %s not found", ErrServerNotFound, in.ServerID)
	}

	applyServerStringFields(s, in)
	applyServerStructFields(s, in)

	return cloneServer(s), nil
}

// HTTP status code constants for TestIdentityProvider.
const (
	httpStatusOK           = http.StatusOK
	httpStatusBadRequest   = http.StatusBadRequest
	httpStatusUnauthorized = http.StatusUnauthorized
)

// TestIdentityProvider tests the identity provider for a server.
func (b *InMemoryBackend) TestIdentityProvider(serverID, userName string) (int, string) {
	b.mu.RLock("TestIdentityProvider")
	defer b.mu.RUnlock()

	s, found := b.servers.Get(serverID)
	if !found {
		return httpStatusBadRequest, "server not found"
	}

	if s.IdentityProviderType != identityProviderServiceManaged && s.IdentityProviderType != "" {
		return httpStatusOK, "No validation for this provider type"
	}

	// SERVICE_MANAGED: check if user exists.
	if b.users.Has(userKey(serverID, userName)) {
		return httpStatusOK, ""
	}

	return httpStatusUnauthorized, "user not found"
}

// AddServerInternal seeds a server for testing purposes.
func (b *InMemoryBackend) AddServerInternal(serverID string) {
	b.mu.Lock("AddServerInternal")
	defer b.mu.Unlock()

	b.servers.Put(&Server{
		ServerID:  serverID,
		State:     serverStatusOnline,
		Protocols: []string{protocolSFTP},
		Domain:    "S3",
		Endpoint:  fmt.Sprintf("%s.server.transfer.%s.amazonaws.com", serverID, b.region),
		CreatedAt: time.Now(),
		Tags:      make(map[string]string),
		AccountID: b.accountID,
		Region:    b.region,
	})
}

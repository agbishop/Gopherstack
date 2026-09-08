package elbv2

import (
	"fmt"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

type InMemoryBackend struct {
	ec2Resolver   EC2Resolver
	certResolver  CertificateResolver
	registry      *store.Registry
	loadBalancers *store.Table[LoadBalancer] // keyed by ARN
	targetGroups  *store.Table[TargetGroup]  // keyed by ARN
	listeners     *store.Table[Listener]     // keyed by ARN
	// listenersByLB indexes listeners by their owning load balancer ARN.
	listenersByLB *store.Index[Listener]
	rules         *store.Table[Rule] // keyed by ARN
	// rulesByListener indexes rules by their owning listener ARN.
	rulesByListener *store.Index[Rule]
	trustStores     *store.Table[TrustStore] // keyed by ARN
	// resourcePolicies stores resource policies keyed by ResourceArn. Left as a
	// plain map (not a store.Table) because the value is a bare string with no
	// identity field of its own -- see store_setup.go.
	resourcePolicies map[string]string
	// lifecycle: tracks when initial targets become healthy / start draining.
	// Left as plain maps (not store.Tables) because they are doubly-nested
	// (tgArn → targetKey → timestamp), not a map[string]*V shape -- see store_setup.go.
	targetReadyAt       map[string]map[string]time.Time // tgArn → targetKey → readyAt (initial→healthy)
	targetDrainingUntil map[string]map[string]time.Time // tgArn → targetKey → drainExpiresAt
	mu                  *lockmetrics.RWMutex
	stopCh              chan struct{}
	accountID           string
	region              string
	ruleCounter         int // monotonically increasing counter for rule ARN generation
	// revocationIDCounter mints RevocationId values for AddTrustStoreRevocations.
	// Real AWS assigns RevocationId (int64) itself when it parses an uploaded
	// revocation file -- callers never supply one -- so this emulator hands out a
	// monotonically increasing int64 per trust store revocation, matching the wire
	// type (types.TrustStoreRevocation.RevocationId *int64).
	revocationIDCounter int64
}

// NewInMemoryBackend creates a new in-memory ELBv2 backend.
func NewInMemoryBackend(accountID, region string) *InMemoryBackend {
	b := &InMemoryBackend{
		registry:            store.NewRegistry(),
		resourcePolicies:    make(map[string]string),
		accountID:           accountID,
		region:              region,
		mu:                  lockmetrics.New("elbv2"),
		targetReadyAt:       make(map[string]map[string]time.Time),
		targetDrainingUntil: make(map[string]map[string]time.Time),
		stopCh:              make(chan struct{}),
	}

	registerAllTables(b)

	go b.runHealthReconciler()

	return b
}

// SetEC2Resolver wires the backend to validate SecurityGroups/Subnets
// against the real services/ec2 backend -- see EC2Resolver's doc comment.
// Called from cli.go's wireELBv2CrossService.
func (b *InMemoryBackend) SetEC2Resolver(r EC2Resolver) {
	b.mu.Lock("SetEC2Resolver")
	defer b.mu.Unlock()

	b.ec2Resolver = r
}

// SetCertificateResolver wires the backend to validate listener
// CertificateArns and report their attach/detach to ACM -- see
// CertificateResolver's doc comment. Called from cli.go's
// wireELBv2CrossService.
func (b *InMemoryBackend) SetCertificateResolver(r CertificateResolver) {
	b.mu.Lock("SetCertificateResolver")
	defer b.mu.Unlock()

	b.certResolver = r
}

// Close stops the background health reconciler.
func (b *InMemoryBackend) Close() {
	select {
	case <-b.stopCh:
	default:
		close(b.stopCh)
	}
}

// validatePort returns ErrInvalidParameter if port is not in the valid range 1-65535.
func validatePort(port int32) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("%w: port must be between 1 and 65535", ErrInvalidParameter)
	}

	return nil
}

// validateResourceName returns ErrInvalidParameter if name violates the ELBv2 naming rules:
// non-empty, alphanumeric characters, hyphens, and underscores;
// cannot start or end with a hyphen.
func validateResourceName(name, kind string) error {
	if len(name) == 0 {
		return fmt.Errorf("%w: %s name must not be empty", ErrInvalidParameter, kind)
	}

	if len(name) > maxNameLength {
		return fmt.Errorf("%w: %s name cannot exceed 32 characters", ErrInvalidParameter, kind)
	}

	if name[0] == '-' || name[len(name)-1] == '-' {
		return fmt.Errorf(
			"%w: %s name cannot start or end with a hyphen",
			ErrInvalidParameter,
			kind,
		)
	}

	for _, c := range name {
		lowerAlpha := c >= 'a' && c <= 'z'
		upperAlpha := c >= 'A' && c <= 'Z'
		digit := c >= '0' && c <= '9'
		hyphen := c == '-'
		underscore := c == '_'

		if !lowerAlpha && !upperAlpha && !digit && !hyphen && !underscore {
			return fmt.Errorf(
				"%w: %s name may only contain alphanumeric characters, hyphens, and underscores",
				ErrInvalidParameter, kind,
			)
		}
	}

	return nil
}

const (
	healthStateHealthy = "healthy"
	ipAddressTypeIPv4  = "ipv4"
	protoHTTP          = "HTTP"
	protoHTTPS         = "HTTPS"
	protoTLS           = "TLS"
	lbTypeApplication  = "application"
	lbTypeNetwork      = "network"
	lbTypeGateway      = "gateway"
	targetTypeLambda   = "lambda"
	priorityDefault    = "default"
	maxNameLength      = 32
	minLBNameLength    = 2
	maxTagKeyLen       = 128
	maxTagValueLen     = 256
	maxTagsPerRes      = 50

	attrAccessLogsS3Enabled           = "access_logs.s3.enabled"
	attrDeletionProtectionEnabled     = "deletion_protection.enabled"
	attrCrossZoneLoadBalancingEnabled = "load_balancing.cross_zone.enabled"
)

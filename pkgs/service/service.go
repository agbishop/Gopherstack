package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/portalloc"
)

// Middleware defines a function that wraps an Echo handler.
type Middleware func(echo.HandlerFunc) echo.HandlerFunc

// Service represents an AWS-compatible service that can be registered
// with the service router. Each service provides an Echo handler,
// routing matcher, and observability information.
type Service interface {
	// Name returns the service identifier (e.g., "DynamoDB", "S3").
	// Used in metrics labels and logging.
	Name() string

	// Handler returns the Echo handler function for this service.
	Handler() echo.HandlerFunc

	// RouteMatcher returns a function that determines if an incoming
	// Echo request should be routed to this service. Matchers are
	// evaluated in registration order. Return true to route here,
	// false to continue to next service.
	RouteMatcher() Matcher

	// GetSupportedOperations returns a list of operations this service
	// supports (e.g., ["GetItem", "PutItem", "Query"] for DynamoDB).
	GetSupportedOperations() []string
}

// Matcher determines whether an incoming request should be routed
// to a particular service handler. Matchers are evaluated by priority
// (higher priority = evaluated first).
type Matcher func(c *echo.Context) bool

// ResourceObserver extracts metrics labels from requests for a specific
// service. Used by the metrics wrapper to instrument operations.
type ResourceObserver interface {
	// ExtractOperation returns the operation name (e.g., "GetItem", "PutObject").
	// Used in metrics labels.
	ExtractOperation(c *echo.Context) string

	// ExtractResource returns the resource identifier (e.g., table name, bucket name).
	// Used in metrics labels.
	ExtractResource(c *echo.Context) string
}

// Registerable combines Service and ResourceObserver into a single interface
// for services that want to be registered with the service registry.
// This unified interface simplifies the registration contract.
type Registerable interface {
	Service
	ResourceObserver

	// MatchPriority returns the priority for this service's matcher.
	// Higher values are evaluated first. Examples:
	// - Header-based matchers (DynamoDB): 100
	// - Path-based matchers (Dashboard): 50
	// - Catch-all matchers (S3): 0
	MatchPriority() int
}

// DashboardProvider is an optional interface that services can implement
// to provide a dashboard UI. The dashboard automatically discovers and
// integrates services that implement this interface.
type DashboardProvider interface {
	// DashboardName returns the user-facing name for this service's dashboard tab.
	// Example: "DynamoDB", "S3"
	DashboardName() string

	// DashboardRoutePrefix returns the URL path prefix for this service's dashboard routes.
	// Example: "dynamodb", "s3"
	DashboardRoutePrefix() string

	// RegisterDashboardRoutes registers all dashboard routes for this service
	// under the given Echo group. The group is mounted at /dashboard/{prefix}.
	// The httpClient and endpoint are provided for services that need to make
	// SDK calls back to the service (e.g., to list tables or buckets).
	RegisterDashboardRoutes(
		group *echo.Group,
		httpClient any, // *http.Client
		endpoint string,
	)
}

// ChaosProvider is an optional interface services implement to declare
// their chaos-injectable surface area. Services that implement this interface
// are automatically discovered by the Chaos API via the registry.
type ChaosProvider interface {
	// ChaosServiceName returns the lowercase AWS-style service name used in
	// fault rules (e.g. "s3", "dynamodb", "sqs").
	ChaosServiceName() string

	// ChaosOperations returns all operations that can be fault-injected.
	// Implementations typically delegate to GetSupportedOperations().
	ChaosOperations() []string

	// ChaosRegions returns all regions this service instance handles.
	// Typically returns the configured default region plus any regions
	// that have active resources.
	ChaosRegions() []string
}

// BackgroundWorker is an optional interface that services can implement
// to start background tasks (e.g. async deletion janitors).
type BackgroundWorker interface {
	StartWorker(ctx context.Context) error
}

// Shutdowner is an optional interface that services can implement to perform
// cleanup of background goroutines and resources during graceful shutdown.
// It is called after the HTTP server has stopped accepting new requests.
// Services that do not implement this interface are silently skipped.
type Shutdowner interface {
	Shutdown(ctx context.Context)
}

// Resettable is an optional interface that services can implement to support
// clearing all in-memory state without restarting the process. Used by the
// POST /_gopherstack/reset endpoint for CI pipelines and rapid local development.
// Services that do not implement this interface are silently skipped during reset.
type Resettable interface {
	Reset()
}

// Purgeable is an optional interface for services that support automatically removing
// resources older than a specific cutoff time (used by AUTO_PURGE_TTL).
type Purgeable interface {
	Purge(ctx context.Context, cutoff time.Time)
}

// FISParamDef describes a single parameter accepted by a FIS action.
type FISParamDef struct {
	Name        string
	Description string
	Default     string
	Required    bool
}

// FISActionDefinition describes a FIS action supported by a service.
type FISActionDefinition struct {
	ActionID    string // e.g., "aws:ec2:stop-instances"
	Description string
	TargetType  string // e.g., "aws:ec2:instance"; empty if action has no targets
	TargetKey   string // key name used in the Targets map (e.g., "Instances", "Roles"); defaults to "Targets"
	Parameters  []FISParamDef
}

// FISActionExecution carries the runtime context for executing a single FIS action.
type FISActionExecution struct {
	ActionID   string
	Parameters map[string]string
	Targets    []string      // resolved resource ARNs
	Duration   time.Duration // 0 means run indefinitely until stopped
}

// FISActionProvider is an optional interface services implement to declare
// FIS actions they support and to execute them.
// Services that implement this interface are automatically discovered by the
// FIS backend via the service registry, enabling zero-config action registration.
type FISActionProvider interface {
	// FISActions returns the FIS action definitions this service supports.
	FISActions() []FISActionDefinition

	// ExecuteFISAction executes a FIS action against resolved targets.
	// It is called by the FIS backend when an experiment's action begins.
	// The implementation must be non-blocking or respect ctx cancellation.
	ExecuteFISAction(ctx context.Context, action FISActionExecution) error
}

// AppContext contains shared resources needed by services during initialization.
//
// Config is also the carrier for cross-service backend lookup: a backend
// stores it via SetAppConfig(ctx.Config) in its provider.Init, then
// type-asserts it to a narrow siblingServices interface (matched
// structurally against *CLI) to reach another service's StorageBackend
// lazily, after every provider has finished Init. See
// services/grafana/cross_service.go for the reference implementation and its
// SetAppConfig doc comment for why the lookup must be lazy. When the sibling
// isn't wired (e.g. a unit test constructing a backend directly), callers
// treat it as unknown and degrade gracefully -- skip the check, or fall back
// -- rather than rejecting the request.
type AppContext struct {
	Config         any
	JanitorCtx     context.Context
	Logger         *slog.Logger
	PortAlloc      *portalloc.Allocator
	JanitorTimeout time.Duration
}

// Provider encapsulates the logic to initialize a service.
type Provider interface {
	// Name returns the name of the service provider.
	Name() string

	// Init initializes the service using the provided application context.
	// It returns the Registerable service, or an error if initialization fails.
	Init(ctx *AppContext) (Registerable, error)
}

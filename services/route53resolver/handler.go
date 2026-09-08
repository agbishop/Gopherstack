package route53resolver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/collections"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/page"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	keyMessageField = "message"

	defaultPageSizeSmall = 10
	defaultPageSizeLarge = 100
)

const resolverTargetPrefix = "Route53Resolver."

var (
	errUnknownAction  = errors.New("unknown action")
	errInvalidRequest = errors.New("invalid request")
)

type Handler struct {
	Backend           StorageBackend
	ops               map[string]service.JSONOpFunc
	supportedOpsCache []string
}

func NewHandler(backend StorageBackend) *Handler {
	h := &Handler{Backend: backend}
	h.ops = h.buildOps()
	// Pre-compute the sorted ops slice once.
	keys := collections.SortedKeys(h.ops)
	h.supportedOpsCache = keys

	return h
}

func (h *Handler) buildOps() map[string]service.JSONOpFunc {
	groups := []map[string]service.JSONOpFunc{
		h.opsResolverEndpoints(),
		h.opsResolverRules(),
		h.opsTags(),
		h.opsFirewallRuleGroups(),
		h.opsFirewallDomainLists(),
		h.opsFirewallRules(),
		h.opsOutpostResolvers(),
		h.opsQueryLogConfigs(),
		h.opsQueryLogAssociations(),
		h.opsRuleAssociations(),
		h.opsFirewallConfigs(),
		h.opsResolverConfigs(),
		h.opsDnssecConfigs(),
	}

	ops := make(map[string]service.JSONOpFunc)
	for _, g := range groups {
		maps.Copy(ops, g)
	}

	return ops
}

// Reset clears all backend state.
func (h *Handler) Reset() {
	h.Backend.Reset()
}

func (h *Handler) Name() string { return "Route53Resolver" }

func (h *Handler) GetSupportedOperations() []string {
	return h.supportedOpsCache
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return "route53resolver" }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this Route53 Resolver instance handles.
func (h *Handler) ChaosRegions() []string { return []string{h.Backend.Region()} }

func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		return strings.HasPrefix(c.Request().Header.Get("X-Amz-Target"), resolverTargetPrefix)
	}
}

func (h *Handler) MatchPriority() int { return service.PriorityHeaderExact }

func (h *Handler) ExtractOperation(c *echo.Context) string {
	target := c.Request().Header.Get("X-Amz-Target")
	action := strings.TrimPrefix(target, resolverTargetPrefix)
	if action == "" || action == target {
		return "Unknown"
	}

	return action
}

type extractResolverResourceInput struct {
	Name string `json:"Name"`
}

func (h *Handler) ExtractResource(c *echo.Context) string {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return ""
	}
	var req extractResolverResourceInput
	_ = json.Unmarshal(body, &req)

	return req.Name
}

func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		region := httputils.ExtractRegionFromRequest(c.Request(), h.Backend.Region())

		return service.HandleTarget(
			c, logger.Load(c.Request().Context()),
			"Route53Resolver", "application/x-amz-json-1.1",
			h.GetSupportedOperations(),
			func(ctx context.Context, action string, body []byte) ([]byte, error) {
				return h.dispatch(context.WithValue(ctx, regionContextKey{}, region), action, body)
			},
			h.handleError,
		)
	}
}

func (h *Handler) dispatch(ctx context.Context, action string, body []byte) ([]byte, error) {
	fn, ok := h.ops[action]
	if !ok {
		return nil, fmt.Errorf("%w: %s", errUnknownAction, action)
	}

	result, err := fn(ctx, body)
	if err != nil {
		return nil, err
	}

	return json.Marshal(result)
}

func (h *Handler) handleError(_ context.Context, c *echo.Context, _ string, err error) error {
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError

	switch {
	case errors.Is(err, ErrNotFound):
		payload, _ := json.Marshal(service.JSONErrorResponse{
			Type:    "ResourceNotFoundException",
			Message: err.Error(),
		})

		return c.JSONBlob(http.StatusNotFound, payload)
	case errors.Is(err, ErrAlreadyExists):
		payload, _ := json.Marshal(service.JSONErrorResponse{
			Type:    "ResourceExistsException",
			Message: err.Error(),
		})

		return c.JSONBlob(http.StatusBadRequest, payload)
	case errors.Is(err, ErrInvalidParameter):
		payload, _ := json.Marshal(service.JSONErrorResponse{
			Type:    "InvalidParameterException",
			Message: err.Error(),
		})

		return c.JSONBlob(http.StatusBadRequest, payload)
	case errors.Is(err, ErrValidation):
		payload, _ := json.Marshal(service.JSONErrorResponse{
			Type:    "InvalidRequestException",
			Message: err.Error(),
		})

		return c.JSONBlob(http.StatusBadRequest, payload)
	case errors.Is(err, ErrBatchValidation):
		payload, _ := json.Marshal(service.JSONErrorResponse{
			Type:    "ValidationException",
			Message: err.Error(),
		})

		return c.JSONBlob(http.StatusBadRequest, payload)
	// route53resolver@v1.48.4 splits its bad-request vocabulary by resource
	// family: the singular Resolver* ops model InvalidRequestException (see
	// ErrValidation) while the Firewall/Outpost ops and the three Batch*
	// FirewallRule ops model ValidationException (see ErrBatchValidation) --
	// left untyped rather than guessing which family reached this path.
	case errors.Is(err, errInvalidRequest), errors.Is(err, errUnknownAction),
		errors.As(err, &syntaxErr), errors.As(err, &typeErr):
		return c.JSON(http.StatusBadRequest, map[string]string{keyMessageField: err.Error()})
	default:
		// InternalServiceErrorException is modeled on all 72 operations
		// (verified by scanning every awsAwsjson11_deserializeOpError* switch),
		// so it is a safe blanket fallback regardless of which operation reached
		// this path.
		payload, _ := json.Marshal(service.JSONErrorResponse{
			Type:    "InternalServiceErrorException",
			Message: err.Error(),
		})

		return c.JSONBlob(http.StatusInternalServerError, payload)
	}
}

// requireResourceID validates that a ResourceId path/body parameter was
// supplied, shared by every op keyed on a bare resourceID (FirewallConfig,
// ResolverConfig, ResolverDnssecConfig, query-log-config associations, ...).
// validationErr is the caller's modeled bad-request sentinel: this family
// splits it the same way the Firewall/Resolver op families do elsewhere in
// this package (see ErrBatchValidation's doc comment) -- FirewallConfig and
// ResolverConfig model ValidationException, not InvalidRequestException,
// while ResolverDnssecConfig and the query-log-config association ops model
// InvalidRequestException, so there is no single correct default here.
func requireResourceID(resourceID string, validationErr error) error {
	if resourceID == "" {
		return fmt.Errorf("%w: ResourceId is required", validationErr)
	}

	return nil
}

// paginate wraps pkgs/page.New for the common List* handler shape shared
// across every op family: page the items, and only surface a NextToken
// pointer when there is in fact a next page.
func paginate[T any](items []T, nextToken string, maxResults int32, defaultSize int) ([]T, *string) {
	pg := page.New(items, nextToken, int(maxResults), defaultSize)

	var next *string
	if pg.Next != "" {
		next = &pg.Next
	}

	return pg.Data, next
}

// mapSlice converts every element of in via f. It is the shared shape
// behind every List* handler's backend-model -> wire-output conversion loop.
func mapSlice[T, U any](in []T, f func(T) U) []U {
	out := make([]U, 0, len(in))
	for _, v := range in {
		out = append(out, f(v))
	}

	return out
}

// boolValue dereferences an optional *bool, defaulting to false when the
// client omitted the field entirely (e.g. RniEnhancedMetricsEnabled /
// TargetNameServerMetricsEnabled on CreateResolverEndpoint).
func boolValue(b *bool) bool {
	if b == nil {
		return false
	}

	return *b
}

// getSimpleConfig implements the shared Get<X>Config shape used by the
// bare-resourceID config families (FirewallConfig, ResolverConfig,
// ResolverDnssecConfig): validate ResourceId, fetch from the backend, and
// convert the result to its wire output.
func getSimpleConfig[TConfig, TOutput any](
	resourceID string,
	validationErr error,
	fetch func() TConfig,
	toOutput func(TConfig) TOutput,
) (TOutput, error) {
	var zero TOutput
	if err := requireResourceID(resourceID, validationErr); err != nil {
		return zero, err
	}

	return toOutput(fetch()), nil
}

// updateSimpleConfig implements the shared Update<X>Config shape: validate
// ResourceId, apply the backend mutation, and convert the result.
func updateSimpleConfig[TConfig, TOutput any](
	resourceID string,
	validationErr error,
	update func() (TConfig, error),
	toOutput func(TConfig) TOutput,
) (TOutput, error) {
	var zero TOutput
	if err := requireResourceID(resourceID, validationErr); err != nil {
		return zero, err
	}

	cfg, err := update()
	if err != nil {
		return zero, err
	}

	return toOutput(cfg), nil
}

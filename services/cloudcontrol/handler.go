package cloudcontrol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	cloudControlTargetPrefix = "CloudApiService."
	cloudControlContentType  = "application/x-amz-json-1.0"
)

var errUnknownAction = errors.New("unknown action")

// Handler is the Echo HTTP handler for CloudControl API operations.
type Handler struct {
	Backend       *InMemoryBackend
	dispatchTable map[string]service.JSONOpFunc
}

// NewHandler creates a new CloudControl handler backed by backend.
// backend must not be nil.
func NewHandler(backend *InMemoryBackend) *Handler {
	h := &Handler{Backend: backend}
	h.dispatchTable = h.buildDispatchTable()

	return h
}

// Name returns the service name.
func (h *Handler) Name() string { return "CloudControl" }

// GetSupportedOperations returns the list of supported operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		"CancelResourceRequest",
		"CreateResource",
		"DeleteResource",
		"GetResource",
		"GetResourceRequestStatus",
		"ListResourceRequests",
		"ListResources",
		"UpdateResource",
	}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return "cloudcontrol" }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this handler instance handles.
func (h *Handler) ChaosRegions() []string { return []string{h.Backend.Region()} }

// RouteMatcher returns a function that matches CloudControl requests.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		return strings.HasPrefix(c.Request().Header.Get("X-Amz-Target"), cloudControlTargetPrefix)
	}
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return service.PriorityHeaderExact }

// ExtractOperation extracts the CloudControl action from the X-Amz-Target header.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	target := c.Request().Header.Get("X-Amz-Target")

	return strings.TrimPrefix(target, cloudControlTargetPrefix)
}

// ExtractResource extracts the resource type name from the request body for metrics/logging.
// Returns "cloudcontrol" as a stable low-cardinality label when a TypeName is absent.
func (h *Handler) ExtractResource(_ *echo.Context) string {
	return "cloudcontrol"
}

// Reset clears all backend state. Useful for test isolation.
func (h *Handler) Reset() { h.Backend.Reset() }

// Handler returns the Echo handler function for CloudControl requests.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		return service.HandleTarget(
			c, logger.Load(c.Request().Context()),
			"CloudControl", cloudControlContentType,
			h.GetSupportedOperations(),
			h.dispatch,
			h.handleError,
		)
	}
}

func (h *Handler) buildDispatchTable() map[string]service.JSONOpFunc {
	return map[string]service.JSONOpFunc{
		"CancelResourceRequest":    service.WrapOp(h.handleCancelResourceRequest),
		"CreateResource":           service.WrapOp(h.handleCreateResource),
		"DeleteResource":           service.WrapOp(h.handleDeleteResource),
		"GetResource":              service.WrapOp(h.handleGetResource),
		"GetResourceRequestStatus": service.WrapOp(h.handleGetResourceRequestStatus),
		"ListResourceRequests":     service.WrapOp(h.handleListResourceRequests),
		"ListResources":            service.WrapOp(h.handleListResources),
		"UpdateResource":           service.WrapOp(h.handleUpdateResource),
	}
}

func (h *Handler) dispatch(ctx context.Context, action string, body []byte) ([]byte, error) {
	fn, ok := h.dispatchTable[action]
	if !ok {
		return nil, fmt.Errorf("%w: %s", errUnknownAction, action)
	}

	result, err := fn(ctx, body)
	if err != nil {
		return nil, err
	}

	return json.Marshal(result)
}

// marshalError serialises a typed AWS error response into bytes.
func marshalError(errType, message string) []byte {
	payload, _ := json.Marshal(service.JSONErrorResponse{Type: errType, Message: message})

	return payload
}

// handleError maps a backend error to the AWS-accurate CloudControl error envelope.
// Per the real API reference, nearly every modeled CloudControl exception -- including
// ResourceNotFoundException, AlreadyExistsException, RequestTokenNotFoundException and
// InvalidRequestException -- carries HTTP status 400; only the handful of server-fault
// exceptions (e.g. ConcurrentModificationException, ServiceInternalErrorException) use 500.
// See e.g. https://docs.aws.amazon.com/cloudcontrolapi/latest/APIReference/API_GetResource.html#API_GetResource_Errors
func (h *Handler) handleError(_ context.Context, c *echo.Context, _ string, err error) error {
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError

	switch {
	case errors.Is(err, ErrClientTokenConflict):
		return c.JSONBlob(http.StatusBadRequest, marshalError("ClientTokenConflictException", err.Error()))
	case errors.Is(err, ErrRequestTokenNotFound):
		return c.JSONBlob(http.StatusBadRequest, marshalError("RequestTokenNotFoundException", err.Error()))
	case errors.Is(err, ErrConcurrentModification):
		return c.JSONBlob(http.StatusInternalServerError, marshalError("ConcurrentModificationException", err.Error()))
	case errors.Is(err, ErrNotFound):
		return c.JSONBlob(http.StatusBadRequest, marshalError("ResourceNotFoundException", err.Error()))
	case errors.Is(err, ErrAlreadyExists):
		return c.JSONBlob(http.StatusBadRequest, marshalError("AlreadyExistsException", err.Error()))
	case errors.Is(err, ErrValidation):
		return c.JSONBlob(http.StatusBadRequest, marshalError("InvalidRequestException", err.Error()))
	case errors.Is(err, errUnknownAction), errors.As(err, &syntaxErr), errors.As(err, &typeErr):
		return c.JSONBlob(http.StatusBadRequest, marshalError("InvalidRequestException", err.Error()))
	default:
		return c.JSONBlob(http.StatusInternalServerError, marshalError("ServiceInternalErrorException", err.Error()))
	}
}

// --- CreateResource ---

type createResourceInput struct {
	TypeName     string `json:"TypeName"`
	DesiredState string `json:"DesiredState"`
	ClientToken  string `json:"ClientToken,omitempty"`
}

type createResourceOutput struct {
	ProgressEvent *ProgressEvent `json:"ProgressEvent"`
}

func (h *Handler) handleCreateResource(
	_ context.Context,
	in *createResourceInput,
) (*createResourceOutput, error) {
	if in.TypeName == "" {
		return nil, fmt.Errorf("%w: TypeName is required", ErrValidation)
	}

	if in.DesiredState == "" {
		return nil, fmt.Errorf("%w: DesiredState is required", ErrValidation)
	}

	event, err := h.Backend.CreateResource(in.TypeName, in.DesiredState, in.ClientToken)
	if err != nil {
		return nil, err
	}

	return &createResourceOutput{ProgressEvent: event}, nil
}

// --- DeleteResource ---

type deleteResourceInput struct {
	TypeName    string `json:"TypeName"`
	Identifier  string `json:"Identifier"`
	ClientToken string `json:"ClientToken,omitempty"`
}

type deleteResourceOutput struct {
	ProgressEvent *ProgressEvent `json:"ProgressEvent"`
}

func (h *Handler) handleDeleteResource(
	_ context.Context,
	in *deleteResourceInput,
) (*deleteResourceOutput, error) {
	if in.TypeName == "" {
		return nil, fmt.Errorf("%w: TypeName is required", ErrValidation)
	}

	if in.Identifier == "" {
		return nil, fmt.Errorf("%w: Identifier is required", ErrValidation)
	}

	event, err := h.Backend.DeleteResource(in.TypeName, in.Identifier, in.ClientToken)
	if err != nil {
		return nil, err
	}

	return &deleteResourceOutput{ProgressEvent: event}, nil
}

// --- GetResource ---

type getResourceInput struct {
	TypeName   string `json:"TypeName"`
	Identifier string `json:"Identifier"`
}

type resourceDescription struct {
	Identifier string `json:"Identifier"`
	Properties string `json:"Properties"`
}

type getResourceOutput struct {
	ResourceDescription *resourceDescription `json:"ResourceDescription"`
	TypeName            string               `json:"TypeName"`
}

func (h *Handler) handleGetResource(
	_ context.Context,
	in *getResourceInput,
) (*getResourceOutput, error) {
	if in.TypeName == "" {
		return nil, fmt.Errorf("%w: TypeName is required", ErrValidation)
	}

	if in.Identifier == "" {
		return nil, fmt.Errorf("%w: Identifier is required", ErrValidation)
	}

	r, err := h.Backend.GetResource(in.TypeName, in.Identifier)
	if err != nil {
		return nil, err
	}

	return &getResourceOutput{
		TypeName: in.TypeName,
		ResourceDescription: &resourceDescription{
			Identifier: r.Identifier,
			Properties: r.Properties,
		},
	}, nil
}

// --- ListResources ---

type listResourcesInput struct {
	NextToken  *string `json:"NextToken,omitempty"`
	MaxResults *int32  `json:"MaxResults,omitempty"`
	TypeName   string  `json:"TypeName"`
	// ResourceModel is a JSON object of property name/value pairs used to select which
	// resources of TypeName to return (real ListResourcesInput.ResourceModel field).
	ResourceModel string `json:"ResourceModel,omitempty"`
}

type listResourcesOutput struct {
	NextToken            *string                `json:"NextToken,omitempty"`
	TypeName             string                 `json:"TypeName"`
	ResourceDescriptions []*resourceDescription `json:"ResourceDescriptions"`
}

func (h *Handler) handleListResources(
	_ context.Context,
	in *listResourcesInput,
) (*listResourcesOutput, error) {
	if in.TypeName == "" {
		return nil, fmt.Errorf("%w: TypeName is required", ErrValidation)
	}

	maxResults := 0
	if in.MaxResults != nil {
		maxResults = int(*in.MaxResults)
	}

	nextToken := ""
	if in.NextToken != nil {
		nextToken = *in.NextToken
	}

	resources, outToken := h.Backend.ListResources(in.TypeName, maxResults, nextToken, in.ResourceModel)
	if resources == nil {
		// TypeName did not pass validation — backend returned nil.
		return nil, fmt.Errorf("%w: invalid TypeName %q", ErrValidation, in.TypeName)
	}

	descs := make([]*resourceDescription, 0, len(resources))

	for _, r := range resources {
		descs = append(descs, &resourceDescription{
			Identifier: r.Identifier,
			Properties: r.Properties,
		})
	}

	out := &listResourcesOutput{
		TypeName:             in.TypeName,
		ResourceDescriptions: descs,
	}

	if outToken != "" {
		out.NextToken = &outToken
	}

	return out, nil
}

// --- UpdateResource ---

type updateResourceInput struct {
	TypeName      string `json:"TypeName"`
	Identifier    string `json:"Identifier"`
	PatchDocument string `json:"PatchDocument"`
	ClientToken   string `json:"ClientToken,omitempty"`
}

type updateResourceOutput struct {
	ProgressEvent *ProgressEvent `json:"ProgressEvent"`
}

func (h *Handler) handleUpdateResource(
	_ context.Context,
	in *updateResourceInput,
) (*updateResourceOutput, error) {
	if in.TypeName == "" {
		return nil, fmt.Errorf("%w: TypeName is required", ErrValidation)
	}

	if in.Identifier == "" {
		return nil, fmt.Errorf("%w: Identifier is required", ErrValidation)
	}

	if in.PatchDocument == "" {
		return nil, fmt.Errorf("%w: PatchDocument is required", ErrValidation)
	}

	event, err := h.Backend.UpdateResource(in.TypeName, in.Identifier, in.PatchDocument, in.ClientToken)
	if err != nil {
		return nil, err
	}

	return &updateResourceOutput{ProgressEvent: event}, nil
}

// --- GetResourceRequestStatus ---

type getResourceRequestStatusInput struct {
	RequestToken string `json:"RequestToken"`
}

type getResourceRequestStatusOutput struct {
	ProgressEvent *ProgressEvent `json:"ProgressEvent"`
	// HooksProgressEvent lists Hook invocations for the request's target.
	// This backend has no Hooks concept, so it is always empty/omitted --
	// modeled as a real (always-nil) field for wire-shape parity with
	// GetResourceRequestStatusOutput.HooksProgressEvent rather than being
	// absent from the struct entirely.
	HooksProgressEvent []hookProgressEvent `json:"HooksProgressEvent,omitempty"`
}

// hookProgressEvent mirrors types.HookProgressEvent. This backend never
// populates it (no Hooks concept), but the field is modeled on the output
// struct for wire-shape parity -- see getResourceRequestStatusOutput.
type hookProgressEvent struct {
	FailureMode       string         `json:"FailureMode,omitempty"`
	HookEventTime     *unixEpochTime `json:"HookEventTime,omitempty"`
	HookStatus        string         `json:"HookStatus,omitempty"`
	HookStatusMessage string         `json:"HookStatusMessage,omitempty"`
	HookTypeArn       string         `json:"HookTypeArn,omitempty"`
	HookTypeName      string         `json:"HookTypeName,omitempty"`
	HookTypeVersionId string         `json:"HookTypeVersionId,omitempty"` //nolint:revive,staticcheck // matches SDK name
	InvocationPoint   string         `json:"InvocationPoint,omitempty"`
}

func (h *Handler) handleGetResourceRequestStatus(
	_ context.Context,
	in *getResourceRequestStatusInput,
) (*getResourceRequestStatusOutput, error) {
	// No InvalidRequestException guard here: GetResourceRequestStatus declares only
	// RequestTokenNotFoundException (confirmed: deserializeOpErrorGetResourceRequestStatus
	// in the pinned SDK's deserializers.go). An empty RequestToken never matches a
	// tracked request, so it naturally falls through to that same error below.
	event, err := h.Backend.GetResourceRequestStatus(in.RequestToken)
	if err != nil {
		return nil, err
	}

	return &getResourceRequestStatusOutput{ProgressEvent: event}, nil
}

// --- CancelResourceRequest ---

type cancelResourceRequestInput struct {
	RequestToken string `json:"RequestToken"`
}

type cancelResourceRequestOutput struct {
	ProgressEvent *ProgressEvent `json:"ProgressEvent"`
}

func (h *Handler) handleCancelResourceRequest(
	_ context.Context,
	in *cancelResourceRequestInput,
) (*cancelResourceRequestOutput, error) {
	// No InvalidRequestException guard here: CancelResourceRequest declares only
	// RequestTokenNotFoundException/ConcurrentModificationException (confirmed:
	// deserializeOpErrorCancelResourceRequest in the pinned SDK's deserializers.go).
	// An empty RequestToken never matches a tracked request, so it naturally falls
	// through to that same not-found error below.
	event, err := h.Backend.CancelResourceRequest(in.RequestToken)
	if err != nil {
		return nil, err
	}

	return &cancelResourceRequestOutput{ProgressEvent: event}, nil
}

// --- ListResourceRequests ---

// resourceRequestStatusFilter mirrors the real SDK's types.ResourceRequestStatusFilter,
// which has exactly two members: Operations and OperationStatuses. It does NOT
// have a TypeName member -- confirmed against both the aws-sdk-go-v2 types
// package and botocore's service-2.json model -- so ListResourceRequests has no
// way to filter by resource type on the wire. (A prior gopherstack pass invented
// a TypeName field here; removed.)
type resourceRequestStatusFilter struct {
	Operations        []string `json:"Operations"`
	OperationStatuses []string `json:"OperationStatuses"`
}

type listResourceRequestsInput struct {
	ResourceRequestStatusFilter *resourceRequestStatusFilter `json:"ResourceRequestStatusFilter"`
	NextToken                   *string                      `json:"NextToken"`
	MaxResults                  *int32                       `json:"MaxResults"`
}

type listResourceRequestsOutput struct {
	NextToken                      *string          `json:"NextToken,omitempty"`
	ResourceRequestStatusSummaries []*ProgressEvent `json:"ResourceRequestStatusSummaries"`
}

func (h *Handler) handleListResourceRequests(
	_ context.Context,
	in *listResourceRequestsInput,
) (*listResourceRequestsOutput, error) {
	var filter *ResourceRequestFilter
	if in.ResourceRequestStatusFilter != nil {
		filter = &ResourceRequestFilter{
			Operations:        in.ResourceRequestStatusFilter.Operations,
			OperationStatuses: in.ResourceRequestStatusFilter.OperationStatuses,
		}
	}

	maxResults := 0
	if in.MaxResults != nil {
		maxResults = int(*in.MaxResults)
	}

	nextToken := ""
	if in.NextToken != nil {
		nextToken = *in.NextToken
	}

	events, outToken, err := h.Backend.ListResourceRequests(filter, maxResults, nextToken)
	if err != nil {
		return nil, err
	}

	out := &listResourceRequestsOutput{
		ResourceRequestStatusSummaries: events,
	}

	if outToken != "" {
		out.NextToken = &outToken
	}

	return out, nil
}

package autoscaling

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/pkgs/worker"
)

const (
	autoscalingVersion           = "2011-01-01"
	autoscalingXMLNS             = "http://autoscaling.amazonaws.com/doc/2011-01-01/"
	errValidationError           = "ValidationError"
	resourceTypeAutoScalingGroup = "auto-scaling-group"
	formValueTrue                = "true"
	unknownOp                    = "Unknown"
)

// Handler is the Echo HTTP handler for Autoscaling operations.
type Handler struct {
	Backend       StorageBackend
	dispatchTable map[string]func(url.Values) (any, error)
	scheduler     *ScheduledActionScheduler
	schedulerRun  worker.SingleRun
}

// NewHandler creates a new Autoscaling handler. If backend is an
// *InMemoryBackend, a ScheduledActionScheduler is attached automatically (see
// StartWorker/Shutdown) so PutScheduledUpdateGroupAction/
// BatchPutScheduledUpdateGroupAction actions actually fire at their scheduled
// time instead of only being reflected by DescribeScheduledActions.
func NewHandler(backend StorageBackend) *Handler {
	h := &Handler{Backend: backend}
	h.dispatchTable = h.buildDispatchTable()

	if mem, ok := backend.(*InMemoryBackend); ok {
		h.scheduler = NewScheduledActionScheduler(mem, 0)
	}

	return h
}

// StartWorker starts the scheduled-action scheduler as a background worker.
// Implements service.BackgroundWorker.
func (h *Handler) StartWorker(ctx context.Context) error {
	if h.scheduler != nil {
		h.schedulerRun.Start(ctx, h.scheduler)
	}

	return nil
}

func (h *Handler) buildDispatchTable() map[string]func(url.Values) (any, error) {
	return map[string]func(url.Values) (any, error){
		"CreateAutoScalingGroup":              h.handleCreateAutoScalingGroup,
		"DescribeAutoScalingGroups":           h.handleDescribeAutoScalingGroups,
		"UpdateAutoScalingGroup":              h.handleUpdateAutoScalingGroup,
		"DeleteAutoScalingGroup":              h.handleDeleteAutoScalingGroup,
		"CreateLaunchConfiguration":           h.handleCreateLaunchConfiguration,
		"DescribeLaunchConfigurations":        h.handleDescribeLaunchConfigurations,
		"DeleteLaunchConfiguration":           h.handleDeleteLaunchConfiguration,
		"DescribeScalingActivities":           h.handleDescribeScalingActivities,
		"AttachInstances":                     h.handleAttachInstances,
		"AttachLoadBalancerTargetGroups":      h.handleAttachLoadBalancerTargetGroups,
		"AttachLoadBalancers":                 h.handleAttachLoadBalancers,
		"AttachTrafficSources":                h.handleAttachTrafficSources,
		"BatchDeleteScheduledAction":          h.handleBatchDeleteScheduledAction,
		"BatchPutScheduledUpdateGroupAction":  h.handleBatchPutScheduledUpdateGroupAction,
		"CancelInstanceRefresh":               h.handleCancelInstanceRefresh,
		"CompleteLifecycleAction":             h.handleCompleteLifecycleAction,
		"CreateOrUpdateTags":                  h.handleCreateOrUpdateTags,
		"DeleteLifecycleHook":                 h.handleDeleteLifecycleHook,
		"SetDesiredCapacity":                  h.handleSetDesiredCapacity,
		"TerminateInstanceInAutoScalingGroup": h.handleTerminateInstanceInAutoScalingGroup,
		"PutLifecycleHook":                    h.handlePutLifecycleHook,
		"DescribeLifecycleHooks":              h.handleDescribeLifecycleHooks,
		"DescribeScheduledActions":            h.handleDescribeScheduledActions,
		"DeleteTags":                          h.handleDeleteTags,
		"DescribeTags":                        h.handleDescribeTags,
		"DescribeAutoScalingInstances":        h.handleDescribeAutoScalingInstances,
		// New operations
		"DeleteNotificationConfiguration":      h.handleDeleteNotificationConfiguration,
		"DeletePolicy":                         h.handleDeletePolicy,
		"DeleteScheduledAction":                h.handleDeleteScheduledAction,
		"DeleteWarmPool":                       h.handleDeleteWarmPool,
		"DescribeAccountLimits":                h.handleDescribeAccountLimits,
		"DescribeAdjustmentTypes":              h.handleDescribeAdjustmentTypes,
		"DescribeAutoScalingNotificationTypes": h.handleDescribeAutoScalingNotificationTypes,
		"DescribeInstanceRefreshes":            h.handleDescribeInstanceRefreshes,
		"DescribeLifecycleHookTypes":           h.handleDescribeLifecycleHookTypes,
		"DescribeLoadBalancerTargetGroups":     h.handleDescribeLoadBalancerTargetGroups,
		"DescribeLoadBalancers":                h.handleDescribeLoadBalancers,
		"DescribeMetricCollectionTypes":        h.handleDescribeMetricCollectionTypes,
		"DescribeNotificationConfigurations":   h.handleDescribeNotificationConfigurations,
		"DescribePolicies":                     h.handleDescribePolicies,
		"DescribeScalingProcessTypes":          h.handleDescribeScalingProcessTypes,
		"DescribeTerminationPolicyTypes":       h.handleDescribeTerminationPolicyTypes,
		"DescribeTrafficSources":               h.handleDescribeTrafficSources,
		"DescribeWarmPool":                     h.handleDescribeWarmPool,
		"DetachInstances":                      h.handleDetachInstances,
		"DetachLoadBalancerTargetGroups":       h.handleDetachLoadBalancerTargetGroups,
		"DetachLoadBalancers":                  h.handleDetachLoadBalancers,
		"DetachTrafficSources":                 h.handleDetachTrafficSources,
		"DisableMetricsCollection":             h.handleDisableMetricsCollection,
		"EnableMetricsCollection":              h.handleEnableMetricsCollection,
		"EnterStandby":                         h.handleEnterStandby,
		"ExecutePolicy":                        h.handleExecutePolicy,
		"ExitStandby":                          h.handleExitStandby,
		"GetPredictiveScalingForecast":         h.handleGetPredictiveScalingForecast,
		"LaunchInstances":                      h.handleLaunchInstances,
		"PutNotificationConfiguration":         h.handlePutNotificationConfiguration,
		"PutScalingPolicy":                     h.handlePutScalingPolicy,
		"PutScheduledUpdateGroupAction":        h.handlePutScheduledUpdateGroupAction,
		"PutWarmPool":                          h.handlePutWarmPool,
		"RecordLifecycleActionHeartbeat":       h.handleRecordLifecycleActionHeartbeat,
		"ResumeProcesses":                      h.handleResumeProcesses,
		"RollbackInstanceRefresh":              h.handleRollbackInstanceRefresh,
		"SetInstanceHealth":                    h.handleSetInstanceHealth,
		"SetInstanceProtection":                h.handleSetInstanceProtection,
		"StartInstanceRefresh":                 h.handleStartInstanceRefresh,
		"SuspendProcesses":                     h.handleSuspendProcesses,
	}
}

// Shutdown stops the scheduled-action scheduler and the backend's in-flight
// lifecycle-hook timers so no goroutine outlives the service. Invoked on
// server shutdown via service.Shutdowner.
func (h *Handler) Shutdown(ctx context.Context) {
	h.schedulerRun.Stop(ctx)

	if c, ok := h.Backend.(interface{ Close() }); ok {
		c.Close()
	}
}

// Ensure Handler implements service.BackgroundWorker and service.Shutdowner at
// compile time.
var (
	_ service.BackgroundWorker = (*Handler)(nil)
	_ service.Shutdowner       = (*Handler)(nil)
)

// Name returns the service name.
func (h *Handler) Name() string { return "Autoscaling" }

// GetSupportedOperations returns the list of supported Autoscaling operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		"CreateAutoScalingGroup",
		"DescribeAutoScalingGroups",
		"UpdateAutoScalingGroup",
		"DeleteAutoScalingGroup",
		"CreateLaunchConfiguration",
		"DescribeLaunchConfigurations",
		"DeleteLaunchConfiguration",
		"DescribeScalingActivities",
		"AttachInstances",
		"AttachLoadBalancerTargetGroups",
		"AttachLoadBalancers",
		"AttachTrafficSources",
		"BatchDeleteScheduledAction",
		"BatchPutScheduledUpdateGroupAction",
		"CancelInstanceRefresh",
		"CompleteLifecycleAction",
		"CreateOrUpdateTags",
		"DeleteLifecycleHook",
		"SetDesiredCapacity",
		"TerminateInstanceInAutoScalingGroup",
		"PutLifecycleHook",
		"DescribeLifecycleHooks",
		"DescribeScheduledActions",
		"DeleteTags",
		"DescribeTags",
		"DescribeAutoScalingInstances",
		// New operations
		"DeleteNotificationConfiguration",
		"DeletePolicy",
		"DeleteScheduledAction",
		"DeleteWarmPool",
		"DescribeAccountLimits",
		"DescribeAdjustmentTypes",
		"DescribeAutoScalingNotificationTypes",
		"DescribeInstanceRefreshes",
		"DescribeLifecycleHookTypes",
		"DescribeLoadBalancerTargetGroups",
		"DescribeLoadBalancers",
		"DescribeMetricCollectionTypes",
		"DescribeNotificationConfigurations",
		"DescribePolicies",
		"DescribeScalingProcessTypes",
		"DescribeTerminationPolicyTypes",
		"DescribeTrafficSources",
		"DescribeWarmPool",
		"DetachInstances",
		"DetachLoadBalancerTargetGroups",
		"DetachLoadBalancers",
		"DetachTrafficSources",
		"DisableMetricsCollection",
		"EnableMetricsCollection",
		"EnterStandby",
		"ExecutePolicy",
		"ExitStandby",
		"GetPredictiveScalingForecast",
		"LaunchInstances",
		"PutNotificationConfiguration",
		"PutScalingPolicy",
		"PutScheduledUpdateGroupAction",
		"PutWarmPool",
		"RecordLifecycleActionHeartbeat",
		"ResumeProcesses",
		"RollbackInstanceRefresh",
		"SetInstanceHealth",
		"SetInstanceProtection",
		"StartInstanceRefresh",
		"SuspendProcesses",
	}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return "autoscaling" }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this handler instance handles.
func (h *Handler) ChaosRegions() []string { return []string{config.DefaultRegion} }

// RouteMatcher returns a function that matches Autoscaling requests.
// Autoscaling requests are form-encoded POSTs with Version=2011-01-01.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		r := c.Request()
		if r.Method != http.MethodPost {
			return false
		}

		if strings.HasPrefix(r.URL.Path, "/dashboard/") {
			return false
		}

		ct := r.Header.Get("Content-Type")
		if !strings.Contains(ct, "application/x-www-form-urlencoded") {
			return false
		}

		body, err := httputils.ReadBody(r)
		if err != nil {
			// Body unreadable (e.g. oversized): fall back to the User-Agent
			// marker every aws-sdk-go-v2 autoscaling client sets
			// (api_client.go's AddSDKAgentKeyValue -- "api/autoscaling").
			// That still identifies this as ours, so claim it and let
			// Handler() produce the typed error instead of masking the
			// read failure as a 404.
			return service.MatchesUserAgentMarker(r.Header, "api/autoscaling")
		}

		vals, err := url.ParseQuery(string(body))
		if err != nil {
			return false
		}

		return vals.Get("Version") == autoscalingVersion
	}
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return service.PriorityFormStandard }

// ExtractOperation extracts the Autoscaling action from the request.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return unknownOp
	}

	vals, err := url.ParseQuery(string(body))
	if err != nil {
		return unknownOp
	}

	action := vals.Get("Action")
	if action == "" {
		return unknownOp
	}

	return action
}

// ExtractResource extracts the Auto Scaling group name from the request.
func (h *Handler) ExtractResource(c *echo.Context) string {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return ""
	}

	vals, err := url.ParseQuery(string(body))
	if err != nil {
		return ""
	}

	return vals.Get("AutoScalingGroupName")
}

// Handler returns the Echo handler function for Autoscaling operations.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		r := c.Request()
		body, err := httputils.ReadBody(r)
		if err != nil {
			return h.writeError(c, http.StatusInternalServerError, "InternalFailure", "failed to read request body")
		}

		vals, err := url.ParseQuery(string(body))
		if err != nil {
			return h.writeError(c, http.StatusInternalServerError, "InternalFailure", "failed to parse request body")
		}

		action := vals.Get("Action")
		if action == "" {
			return h.writeError(c, http.StatusBadRequest, "MissingAction", "missing Action parameter")
		}

		log := logger.Load(r.Context())
		log.Debug("autoscaling request", "action", action)

		resp, opErr := h.dispatch(action, vals)
		if opErr != nil {
			return h.handleOpError(c, action, opErr)
		}

		xmlBytes, err := marshalXML(resp)
		if err != nil {
			return h.writeError(c, http.StatusInternalServerError, "InternalFailure", "internal server error")
		}

		return c.Blob(http.StatusOK, "text/xml", xmlBytes)
	}
}

// dispatch routes the Autoscaling action to the appropriate handler.
func (h *Handler) dispatch(action string, vals url.Values) (any, error) {
	fn, ok := h.dispatchTable[action]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownAction, action)
	}

	return fn(vals)
}

// handleOpError translates an operation error into an HTTP response.
func (h *Handler) handleOpError(c *echo.Context, action string, opErr error) error {
	statusCode := http.StatusBadRequest
	code := autoscalingErrorCode(opErr)

	if code == "" {
		code = "InternalFailure"
		statusCode = http.StatusInternalServerError
		logger.Load(c.Request().Context()).Error("autoscaling internal error", "error", opErr, "action", action)
	}

	return h.writeError(c, statusCode, code, opErr.Error())
}

func autoscalingErrorCode(opErr error) string {
	type errorMapping struct {
		sentinel error
		code     string
	}

	mappings := []errorMapping{
		{ErrGroupNotFound, errValidationError},
		{ErrGroupAlreadyExists, "AlreadyExists"},
		{ErrLaunchConfigurationNotFound, errValidationError},
		{ErrLaunchConfigurationAlreadyExists, "AlreadyExists"},
		{ErrInvalidParameter, errValidationError},
		{ErrUnknownAction, "InvalidAction"},
		{ErrActiveInstanceRefreshNotFound, "ActiveInstanceRefreshNotFound"},
		{ErrLifecycleHookNotFound, errValidationError},
		{ErrScalingActivityInProgress, "ScalingActivityInProgress"},
		{ErrInstanceNotFound, errValidationError},
		{ErrWarmPoolNotFound, errValidationError},
		{ErrPolicyNotFound, errValidationError},
		{ErrDeletionProtected, "ResourceInUse"},
		{ErrLaunchConfigurationInUse, "ResourceInUse"},
		{ErrInstanceRefreshInProgress, "InstanceRefreshInProgress"},
	}

	for _, m := range mappings {
		if errors.Is(opErr, m.sentinel) {
			return m.code
		}
	}

	return ""
}

func (h *Handler) writeError(c *echo.Context, statusCode int, code, message string) error {
	errResp := &autoscalingErrorResponse{
		Xmlns:     autoscalingXMLNS,
		Error:     autoscalingError{Code: code, Message: message, Type: "Sender"},
		RequestID: "autoscaling-error",
	}

	xmlBytes, err := marshalXML(errResp)
	if err != nil {
		return c.String(http.StatusInternalServerError, "internal server error")
	}

	return c.Blob(statusCode, "text/xml", xmlBytes)
}

func marshalXML(v any) ([]byte, error) {
	raw, err := xml.Marshal(v)
	if err != nil {
		return nil, err
	}

	return append([]byte(xml.Header), raw...), nil
}

// --- helper functions ---

// paginateItems applies cursor-based pagination over a slice of string-keyed items.
// nameOf returns the name used for the cursor key from each item.
// Returns the page slice and the next-page token (empty string if last page).
// parseIntVal parses a string to int32. Empty string returns 0, nil.
func parseIntVal(s string) (int32, error) {
	if s == "" {
		return 0, nil
	}

	n, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0, err
	}

	return int32(n), nil
}

// parseTimeVal parses an AWS query-protocol DateTime value (ISO8601, e.g.
// "2024-01-02T15:04:05Z"). Returns the zero time (and no error) for an empty or
// unparseable string; scheduled-action times are optional AWS request fields, and
// silently dropping an unparseable one matches the historical "omit if invalid"
// behavior of the rest of this handler's optional-field parsing.
func parseTimeVal(s string) time.Time {
	if s == "" {
		return time.Time{}
	}

	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}

	return t
}

// parseMembers extracts indexed form values with the given prefix (e.g. "AvailabilityZones.member").
func parseMembers(vals url.Values, prefix string) []string {
	result := make([]string, 0)

	for i := 1; ; i++ {
		key := fmt.Sprintf("%s.%d", prefix, i)
		v := vals.Get(key)

		if v == "" {
			break
		}

		result = append(result, v)
	}

	return result
}

// parseTags extracts tags from the form values using the standard AWS Tags.member.N.Key/Value pattern.
// PropagateAtLaunch defaults to true when omitted (AWS behavior).
func parseTags(vals url.Values, prefix string) []Tag {
	result := make([]Tag, 0)

	for i := 1; ; i++ {
		keyParam := fmt.Sprintf("%s.%d.Key", prefix, i)
		k := vals.Get(keyParam)

		if k == "" {
			break
		}

		propagate := true
		if v := vals.Get(fmt.Sprintf("%s.%d.PropagateAtLaunch", prefix, i)); v != "" {
			propagate = v == formValueTrue
		}

		result = append(result, Tag{
			Key:               k,
			Value:             vals.Get(fmt.Sprintf("%s.%d.Value", prefix, i)),
			PropagateAtLaunch: propagate,
		})
	}

	return result
}

// toXMLScalingActivity converts a ScalingActivity to the XML response type.
func toXMLScalingActivity(a *ScalingActivity) xmlScalingActivity {
	endTime := ""
	if !a.EndTime.IsZero() {
		endTime = a.EndTime.UTC().Format(time.RFC3339)
	}

	return xmlScalingActivity{
		ActivityID:           a.ActivityID,
		AutoScalingGroupName: a.AutoScalingGroupName,
		Description:          a.Description,
		Cause:                a.Cause,
		StatusCode:           a.StatusCode,
		StatusMessage:        a.StatusMessage,
		Progress:             a.Progress,
		StartTime:            a.StartTime.UTC().Format(time.RFC3339),
		EndTime:              endTime,
	}
}

// --- XML response types ---

type xmlResponseMetadata struct {
	RequestID string `xml:"RequestId"`
}

// emptyResultXML is the empty "<Op>Result" element real Autoscaling responses carry even
// when the op's SDK output shape has no members. Their deserializers (e.g.
// autoscaling@v1.70.4 deserializers.go) unconditionally call
// decoder.GetElement("<Op>Result") for query-protocol ops that aren't among the ones
// whose deserializer discards the body outright, so omitting the element fails
// deserialization with "node not found" for every real SDK client.
type emptyResultXML struct{}

type autoscalingError struct {
	Code    string `xml:"Code"`
	Message string `xml:"Message"`
	Type    string `xml:"Type"`
}

type autoscalingErrorResponse struct {
	XMLName   xml.Name         `xml:"ErrorResponse"`
	Xmlns     string           `xml:"xmlns,attr"`
	Error     autoscalingError `xml:"Error"`
	RequestID string           `xml:"RequestId"`
}

type xmlStringValue struct {
	Value string `xml:",chardata"`
}

type xmlStringValueList struct {
	Members []xmlStringValue `xml:"member"`
}

type xmlScalingActivity struct {
	ActivityID           string `xml:"ActivityId"`
	AutoScalingGroupName string `xml:"AutoScalingGroupName"`
	Description          string `xml:"Description,omitempty"`
	// Cause is required (types.go:298, Activity) and, unlike Description, has no
	// omitempty -- every construction site below sets it to a real, non-empty
	// narrative of why the activity happened.
	Cause         string `xml:"Cause"`
	StatusCode    string `xml:"StatusCode"`
	StatusMessage string `xml:"StatusMessage,omitempty"`
	StartTime     string `xml:"StartTime"`
	EndTime       string `xml:"EndTime,omitempty"`
	Progress      int32  `xml:"Progress"`
}

type xmlScalingActivityList struct {
	Members []xmlScalingActivity `xml:"member"`
}

// parseTrafficSources parses TrafficSources from form values using the standard AWS pattern.
func parseTrafficSources(vals url.Values) []TrafficSource {
	result := make([]TrafficSource, 0)

	for i := 1; ; i++ {
		idKey := fmt.Sprintf("TrafficSources.member.%d.Identifier", i)
		typeKey := fmt.Sprintf("TrafficSources.member.%d.Type", i)
		id := vals.Get(idKey)

		if id == "" {
			break
		}

		result = append(result, TrafficSource{
			Identifier: id,
			Type:       vals.Get(typeKey),
		})
	}

	return result
}

// Purge implements service.Purgeable by removing all Auto Scaling resources older than cutoff.
func (h *Handler) Purge(ctx context.Context, cutoff time.Time) {
	if b, ok := h.Backend.(*InMemoryBackend); ok {
		b.Purge(ctx, cutoff)
	}
}

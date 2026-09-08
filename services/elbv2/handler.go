package elbv2

import (
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

const (
	elbv2Version   = "2015-12-01"
	elbv2XMLNS     = "http://elasticloadbalancing.amazonaws.com/doc/2015-12-01/"
	attrValueFalse = "false"
	attrValueTrue  = "true"
	unknownOp      = "Unknown"
)

// Handler is the Echo HTTP handler for ELBv2 operations.
type Handler struct {
	Backend       StorageBackend
	dispatchTable map[string]dispatchFunc
}

// NewHandler creates a new ELBv2 handler.
func NewHandler(backend StorageBackend) *Handler {
	h := &Handler{Backend: backend}
	h.dispatchTable = h.buildDispatchTable()

	return h
}

// Name returns the service name.
func (h *Handler) Name() string { return "ELBv2" }

// GetSupportedOperations returns the list of supported ELBv2 operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		"CreateLoadBalancer",
		"DeleteLoadBalancer",
		"DescribeLoadBalancers",
		"ModifyLoadBalancerAttributes",
		"DescribeLoadBalancerAttributes",
		"CreateTargetGroup",
		"DeleteTargetGroup",
		"DescribeTargetGroups",
		"ModifyTargetGroup",
		"ModifyTargetGroupAttributes",
		"DescribeTargetGroupAttributes",
		"RegisterTargets",
		"DeregisterTargets",
		"DescribeTargetHealth",
		"CreateListener",
		"DeleteListener",
		"DescribeListeners",
		"ModifyListener",
		"ModifyListenerAttributes",
		"DescribeListenerAttributes",
		"CreateRule",
		"DeleteRule",
		"DescribeRules",
		"ModifyRule",
		"SetRulePriorities",
		"AddTags",
		"RemoveTags",
		"DescribeTags",
		"SetSecurityGroups",
		"SetSubnets",
		"SetIpAddressType",
		"AddListenerCertificates",
		"AddTrustStoreRevocations",
		"CreateTrustStore",
		"DeleteSharedTrustStoreAssociation",
		"DeleteTrustStore",
		"DescribeAccountLimits",
		"DescribeCapacityReservation",
		"DescribeListenerCertificates",
		"DescribeSSLPolicies",
		"DescribeTrustStoreAssociations",
		"DescribeTrustStoreRevocations",
		"DescribeTrustStores",
		"GetResourcePolicy",
		"GetTrustStoreCaCertificatesBundle",
		"GetTrustStoreRevocationContent",
		"ModifyCapacityReservation",
		"ModifyIpPools",
		"ModifyTrustStore",
		"RemoveListenerCertificates",
		"RemoveTrustStoreRevocations",
	}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return "elasticloadbalancingv2" }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this handler instance handles.
func (h *Handler) ChaosRegions() []string { return []string{config.DefaultRegion} }

// RouteMatcher returns a function that matches ELBv2 requests.
// ELBv2 requests are form-encoded POSTs with Version=2015-12-01.
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
			// marker every aws-sdk-go-v2 elasticloadbalancingv2 client sets
			// (api_client.go's AddSDKAgentKeyValue -- "api/elasticloadbalancingv2").
			// That still identifies this as ours, so claim it and let
			// Handler() produce the typed error instead of masking the
			// read failure as a 404.
			return service.MatchesUserAgentMarker(r.Header, "api/elasticloadbalancingv2")
		}

		vals, err := url.ParseQuery(string(body))
		if err != nil {
			return false
		}

		return vals.Get("Version") == elbv2Version
	}
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return service.PriorityFormStandard }

// ExtractOperation extracts the ELBv2 action from the request.
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

// ExtractResource extracts the primary resource identifier from the request.
func (h *Handler) ExtractResource(c *echo.Context) string {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return ""
	}

	vals, err := url.ParseQuery(string(body))
	if err != nil {
		return ""
	}

	if name := vals.Get("Name"); name != "" {
		return name
	}

	return vals.Get("LoadBalancerArn")
}

// Handler returns the Echo handler function for ELBv2 operations.
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
		log.Debug("elbv2 request", "action", action)

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

// dispatch routes the ELBv2 action to the appropriate handler.
type dispatchFunc func(url.Values) (any, error)

func (h *Handler) buildDispatchTable() map[string]dispatchFunc {
	return map[string]dispatchFunc{
		"CreateLoadBalancer":                h.handleCreateLoadBalancer,
		"DeleteLoadBalancer":                h.handleDeleteLoadBalancer,
		"DescribeLoadBalancers":             h.handleDescribeLoadBalancers,
		"ModifyLoadBalancerAttributes":      h.handleModifyLoadBalancerAttributes,
		"DescribeLoadBalancerAttributes":    h.handleDescribeLoadBalancerAttributes,
		"SetSecurityGroups":                 h.handleSetSecurityGroups,
		"SetSubnets":                        h.handleSetSubnets,
		"SetIpAddressType":                  h.handleSetIPAddressType,
		"CreateTargetGroup":                 h.handleCreateTargetGroup,
		"DeleteTargetGroup":                 h.handleDeleteTargetGroup,
		"DescribeTargetGroups":              h.handleDescribeTargetGroups,
		"ModifyTargetGroup":                 h.handleModifyTargetGroup,
		"ModifyTargetGroupAttributes":       h.handleModifyTargetGroupAttributes,
		"DescribeTargetGroupAttributes":     h.handleDescribeTargetGroupAttributes,
		"RegisterTargets":                   h.handleRegisterTargets,
		"DeregisterTargets":                 h.handleDeregisterTargets,
		"DescribeTargetHealth":              h.handleDescribeTargetHealth,
		"CreateListener":                    h.handleCreateListener,
		"DeleteListener":                    h.handleDeleteListener,
		"DescribeListeners":                 h.handleDescribeListeners,
		"ModifyListener":                    h.handleModifyListener,
		"ModifyListenerAttributes":          h.handleModifyListenerAttributes,
		"DescribeListenerAttributes":        h.handleDescribeListenerAttributes,
		"CreateRule":                        h.handleCreateRule,
		"DeleteRule":                        h.handleDeleteRule,
		"DescribeRules":                     h.handleDescribeRules,
		"ModifyRule":                        h.handleModifyRule,
		"SetRulePriorities":                 h.handleSetRulePriorities,
		"AddTags":                           h.handleAddTags,
		"RemoveTags":                        h.handleRemoveTags,
		"DescribeTags":                      h.handleDescribeTags,
		"AddListenerCertificates":           h.handleAddListenerCertificates,
		"AddTrustStoreRevocations":          h.handleAddTrustStoreRevocations,
		"CreateTrustStore":                  h.handleCreateTrustStore,
		"DeleteSharedTrustStoreAssociation": h.handleDeleteSharedTrustStoreAssociation,
		"DeleteTrustStore":                  h.handleDeleteTrustStore,
		"DescribeAccountLimits":             h.handleDescribeAccountLimits,
		"DescribeCapacityReservation":       h.handleDescribeCapacityReservation,
		"DescribeListenerCertificates":      h.handleDescribeListenerCertificates,
		"DescribeSSLPolicies":               h.handleDescribeSSLPolicies,
		"DescribeTrustStoreAssociations":    h.handleDescribeTrustStoreAssociations,
		"DescribeTrustStoreRevocations":     h.handleDescribeTrustStoreRevocations,
		"DescribeTrustStores":               h.handleDescribeTrustStores,
		"GetResourcePolicy":                 h.handleGetResourcePolicy,
		"GetTrustStoreCaCertificatesBundle": h.handleGetTrustStoreCaCertificatesBundle,
		"GetTrustStoreRevocationContent":    h.handleGetTrustStoreRevocationContent,
		"ModifyCapacityReservation":         h.handleModifyCapacityReservation,
		"ModifyIpPools":                     h.handleModifyIPPools,
		"ModifyTrustStore":                  h.handleModifyTrustStore,
		"RemoveListenerCertificates":        h.handleRemoveListenerCertificates,
		"RemoveTrustStoreRevocations":       h.handleRemoveTrustStoreRevocations,
	}
}

func (h *Handler) dispatch(action string, vals url.Values) (any, error) {
	fn, ok := h.dispatchTable[action]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownAction, action)
	}

	return fn(vals)
}

// handleOpError translates an operation error into an HTTP response.
func (h *Handler) handleOpError(c *echo.Context, action string, opErr error) error {
	code, statusCode := elbv2ErrorCode(opErr)

	if code == "" {
		code = "InternalFailure"
		statusCode = http.StatusInternalServerError
		logger.Load(c.Request().Context()).Error("elbv2 internal error", "error", opErr, "action", action)
	}

	return h.writeError(c, statusCode, code, opErr.Error())
}

func elbv2ErrorCode(opErr error) (string, int) {
	type errorMapping struct {
		sentinel error
		code     string
		httpCode int
	}

	// NOTE: real AWS ELBv2 uses the classic AWS "Query" protocol (like EC2), where
	// EVERY client error — including NotFound and AlreadyExists conditions — is
	// returned with HTTP status 400; the client SDK dispatches on the XML <Code>
	// element, not the HTTP status. Verified against the elasticloadbalancingv2
	// API model (api-2.json), which sets httpStatusCode=400 for every exception
	// shape in this service. Using 404/409 here (as a REST-JSON service would)
	// is wire-inaccurate for a query-protocol service.
	mappings := []errorMapping{
		{ErrLoadBalancerNotFound, "LoadBalancerNotFound", http.StatusBadRequest},
		{ErrTargetGroupNotFound, "TargetGroupNotFound", http.StatusBadRequest},
		{ErrListenerNotFound, "ListenerNotFound", http.StatusBadRequest},
		{ErrRuleNotFound, "RuleNotFound", http.StatusBadRequest},
		{ErrTrustStoreNotFound, "TrustStoreNotFound", http.StatusBadRequest},
		{ErrResourcePolicyNotFound, "ResourceNotFound", http.StatusBadRequest},
		{ErrTrustStoreAssociationNotFound, "AssociationNotFound", http.StatusBadRequest},
		{ErrRevocationIDNotFound, "RevocationIdNotFound", http.StatusBadRequest},
		{ErrLoadBalancerAlreadyExists, "DuplicateLoadBalancerName", http.StatusBadRequest},
		{ErrTargetGroupAlreadyExists, "DuplicateTargetGroupName", http.StatusBadRequest},
		{ErrTrustStoreAlreadyExists, "DuplicateTrustStoreName", http.StatusBadRequest},
		{ErrDuplicateListener, "DuplicateListener", http.StatusBadRequest},
		{ErrDuplicateRulePriority, "PriorityInUse", http.StatusBadRequest},
		{ErrTargetGroupInUse, "ResourceInUse", http.StatusBadRequest},
		{ErrOperationNotPermitted, "OperationNotPermitted", http.StatusBadRequest},
		{ErrInvalidConfigurationRequest, "InvalidConfigurationRequest", http.StatusBadRequest},
		{ErrUnknownAction, "InvalidAction", http.StatusBadRequest},
		{ErrCertificateNotFound, "CertificateNotFound", http.StatusBadRequest},
		{ErrInvalidSecurityGroup, "InvalidSecurityGroup", http.StatusBadRequest},
		{ErrSubnetNotFound, "SubnetNotFound", http.StatusBadRequest},
		{awserr.ErrInvalidParameter, "ValidationError", http.StatusBadRequest},
	}

	for _, m := range mappings {
		if errors.Is(opErr, m.sentinel) {
			return m.code, m.httpCode
		}
	}

	return "", http.StatusInternalServerError
}

func (h *Handler) writeError(c *echo.Context, statusCode int, code, message string) error {
	errResp := &elbv2ErrorResponse{
		Xmlns:     elbv2XMLNS,
		Error:     elbv2Error{Code: code, Message: message, Type: "Sender"},
		RequestID: "elbv2-error",
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

func parseInt32(s string) (int32, error) {
	if s == "" {
		return 0, nil
	}

	n, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0, err
	}

	return int32(n), nil
}

// defaultPageSize is the default number of results to return per page, matching AWS defaults.
const defaultPageSize = 400

// parsePagination extracts Marker and PageSize from form values.
// Returns the marker string and the effective page size.
func parsePagination(vals url.Values) (string, int) {
	m := vals.Get("Marker")
	ps := defaultPageSize

	if pageSizeStr := vals.Get("PageSize"); pageSizeStr != "" {
		if n, err := parseInt32(pageSizeStr); err == nil && n > 0 {
			ps = int(n)
		}
	}

	return m, ps
}

// applyMarkerPage applies the shared marker-based pagination scheme every
// Describe* op in this service uses: skip past marker (an item's own key),
// then cut to pageSize, returning the last returned item's key as the next
// marker when more remain.
func applyMarkerPage[T any](items []T, marker string, pageSize int, keyOf func(T) string) ([]T, string) {
	if marker != "" {
		for i, it := range items {
			if keyOf(it) == marker {
				items = items[i+1:]

				break
			}
		}
	}

	var nextMarker string
	if len(items) > pageSize {
		nextMarker = keyOf(items[pageSize-1])
		items = items[:pageSize]
	}

	return items, nextMarker
}

// parseMembers extracts indexed form values (e.g. "Names.member.1").
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

// parseKVAttrs extracts key-value attribute pairs from Attributes.member.N.Key/Value form values.
func parseKVAttrs(vals url.Values, prefix string) map[string]string {
	attrs := make(map[string]string)

	for i := 1; ; i++ {
		k := vals.Get(fmt.Sprintf("%s.%d.Key", prefix, i))
		if k == "" {
			break
		}

		attrs[k] = vals.Get(fmt.Sprintf("%s.%d.Value", prefix, i))
	}

	return attrs
}

// parseTagKVs extracts key-value tag pairs from Tags.member.N.Key/Value form values.
func parseTagKVs(vals url.Values) []tags.KV {
	const prefix = "Tags.member"

	result := make([]tags.KV, 0)

	for i := 1; ; i++ {
		k := vals.Get(fmt.Sprintf("%s.%d.Key", prefix, i))
		if k == "" {
			break
		}

		result = append(result, tags.KV{Key: k, Value: vals.Get(fmt.Sprintf("%s.%d.Value", prefix, i))})
	}

	return result
}

// parseTagKeys extracts tag keys from TagKeys.member.N form values (for RemoveTags).
func parseTagKeys(vals url.Values, prefix string) []string {
	result := make([]string, 0)

	for i := 1; ; i++ {
		k := vals.Get(fmt.Sprintf("%s.%d", prefix, i))
		if k == "" {
			break
		}

		result = append(result, k)
	}

	return result
}

type elbv2Error struct {
	Code    string `xml:"Code"`
	Message string `xml:"Message"`
	Type    string `xml:"Type"`
}

type elbv2ErrorResponse struct {
	XMLName   xml.Name   `xml:"ErrorResponse"`
	Xmlns     string     `xml:"xmlns,attr"`
	Error     elbv2Error `xml:"Error"`
	RequestID string     `xml:"RequestId"`
}

type xmlResponseMetadata struct {
	RequestID string `xml:"RequestId"`
}

// emptyResultXML is the empty "<Op>Result" element real ELBv2 responses carry even when
// the op's SDK output shape has no members. Their deserializers (e.g.
// elasticloadbalancingv2@v1.58.5 deserializers.go:1277) unconditionally call
// decoder.GetElement("<Op>Result"), so omitting the element entirely fails
// deserialization with "node not found" for every real SDK client.
type emptyResultXML struct{}

type xmlStringValue struct {
	Value string `xml:",chardata"`
}

type xmlStringList struct {
	Members []xmlStringValue `xml:"member"`
}

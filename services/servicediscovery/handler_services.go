package servicediscovery

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/blackbirdworks/gopherstack/pkgs/awstime"
)

type dnsRecordRequest struct {
	Type string `json:"Type"`
	TTL  int64  `json:"TTL"`
}

type dnsConfigRequest struct {
	NamespaceID   string             `json:"NamespaceId"`
	RoutingPolicy string             `json:"RoutingPolicy"`
	DNSRecords    []dnsRecordRequest `json:"DnsRecords"`
}

type healthCheckConfigRequest struct {
	Type             string `json:"Type"`
	ResourcePath     string `json:"ResourcePath"`
	FailureThreshold int    `json:"FailureThreshold"`
}

type healthCheckCustomConfigRequest struct {
	FailureThreshold int `json:"FailureThreshold"`
}

type createServiceRequest struct {
	Name                    string                          `json:"Name"`
	Description             string                          `json:"Description"`
	NamespaceID             string                          `json:"NamespaceId"`
	Type                    string                          `json:"Type"`
	CreatorRequestID        string                          `json:"CreatorRequestId"`
	DNSConfig               *dnsConfigRequest               `json:"DnsConfig"`
	HealthCheckConfig       *healthCheckConfigRequest       `json:"HealthCheckConfig"`
	HealthCheckCustomConfig *healthCheckCustomConfigRequest `json:"HealthCheckCustomConfig"`
	Tags                    []tagEntry                      `json:"Tags"`
}

func parseDNSConfig(req *dnsConfigRequest) *DNSConfig {
	if req == nil {
		return nil
	}

	dc := &DNSConfig{
		NamespaceID:   req.NamespaceID,
		RoutingPolicy: req.RoutingPolicy,
	}

	for _, r := range req.DNSRecords {
		dc.DNSRecords = append(dc.DNSRecords, DNSRecord(r))
	}

	return dc
}

func parseHealthCheckConfig(req *healthCheckConfigRequest) *HealthCheckConfig {
	if req == nil {
		return nil
	}

	return &HealthCheckConfig{
		Type:             req.Type,
		ResourcePath:     req.ResourcePath,
		FailureThreshold: req.FailureThreshold,
	}
}

func parseHealthCheckCustomConfig(req *healthCheckCustomConfigRequest) *HealthCheckCustomConfig {
	if req == nil {
		return nil
	}

	return &HealthCheckCustomConfig{
		FailureThreshold: req.FailureThreshold,
	}
}

// validateDNSConfigEnums enforces the closed enums from the botocore model
// (servicediscovery/2017-03-14/service-2.json): shape RoutingPolicy{enum:
// [MULTIVALUE,WEIGHTED]} (optional) and RecordType{enum:[SRV,A,AAAA,CNAME]}
// (required per DnsRecord).
func validateDNSConfigEnums(dc *DNSConfig) error {
	if dc == nil {
		return nil
	}

	switch dc.RoutingPolicy {
	case "", "MULTIVALUE", "WEIGHTED":
	default:
		return fmt.Errorf("%w: RoutingPolicy %q is not one of MULTIVALUE, WEIGHTED", ErrInvalidInput, dc.RoutingPolicy)
	}

	for _, r := range dc.DNSRecords {
		switch r.Type {
		case "SRV", "A", "AAAA", "CNAME":
		default:
			return fmt.Errorf("%w: DnsRecords Type %q is not one of SRV, A, AAAA, CNAME", ErrInvalidInput, r.Type)
		}
	}

	return nil
}

// validateHealthCheckConfigEnum enforces the closed HealthCheckType enum
// (service-2.json: HealthCheckType{enum:[HTTP,HTTPS,TCP]}).
func validateHealthCheckConfigEnum(hc *HealthCheckConfig) error {
	if hc == nil {
		return nil
	}

	switch hc.Type {
	case "HTTP", "HTTPS", "TCP":
	default:
		return fmt.Errorf("%w: HealthCheckConfig Type %q is not one of HTTP, HTTPS, TCP", ErrInvalidInput, hc.Type)
	}

	return nil
}

func (h *Handler) handleCreateService(_ context.Context, body []byte) ([]byte, error) {
	var req createServiceRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if req.Name == "" {
		return nil, fmt.Errorf("%w: Name is required", errInvalidRequest)
	}

	if req.HealthCheckConfig != nil && req.HealthCheckCustomConfig != nil {
		return nil, fmt.Errorf(
			"%w: HealthCheckConfig and HealthCheckCustomConfig are mutually exclusive",
			ErrInvalidInput,
		)
	}

	dnsConfig := parseDNSConfig(req.DNSConfig)
	if err := validateDNSConfigEnums(dnsConfig); err != nil {
		return nil, err
	}

	healthCheckConfig := parseHealthCheckConfig(req.HealthCheckConfig)
	if err := validateHealthCheckConfigEnum(healthCheckConfig); err != nil {
		return nil, err
	}

	if err := validateTags(req.Tags); err != nil {
		return nil, err
	}

	svc, err := h.Backend.CreateService(
		req.Name,
		req.NamespaceID,
		req.Description,
		req.Type,
		dnsConfig,
		healthCheckConfig,
		parseHealthCheckCustomConfig(req.HealthCheckCustomConfig),
		tagsToMap(req.Tags),
	)
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]any{
		keyService: serviceToMap(svc),
	})
}

type deleteServiceRequest struct {
	ID string `json:"Id"`
}

// handleDeleteService deletes a service. It intentionally does NOT deregister
// instances on the caller's behalf: real Cloud Map's DeleteService "fails if
// the service still contains one or more registered instances" -- the caller
// must deregister every instance first. h.Backend.DeleteService already
// enforces this (returns ErrResourceInUse).
func (h *Handler) handleDeleteService(_ context.Context, body []byte) error {
	var req deleteServiceRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if req.ID == "" {
		return fmt.Errorf("%w: Id is required", errInvalidRequest)
	}

	return h.Backend.DeleteService(req.ID)
}

type getServiceRequest struct {
	ID string `json:"Id"`
}

func (h *Handler) handleGetService(_ context.Context, body []byte) ([]byte, error) {
	var req getServiceRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if req.ID == "" {
		return nil, fmt.Errorf("%w: Id is required", errInvalidRequest)
	}

	svc, err := h.Backend.GetService(req.ID)
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]any{
		keyService: serviceToMap(svc),
	})
}

type serviceFilter struct {
	Name      string   `json:"Name"`
	Condition string   `json:"Condition"`
	Values    []string `json:"Values"`
}

type listServicesRequest struct {
	MaxResults *int            `json:"MaxResults"`
	NextToken  string          `json:"NextToken"`
	Filters    []serviceFilter `json:"Filters"`
}

// buildServicesFilter converts the wire-level filter entries into a
// ListServicesFilter, per ServiceFilter's documented Name values: NAMESPACE_ID
// and RESOURCE_OWNER (types.ServiceFilter doc comment).
func buildServicesFilter(filters []serviceFilter) ListServicesFilter {
	f := ListServicesFilter{}

	for _, entry := range filters {
		if len(entry.Values) == 0 {
			continue
		}

		fv := FilterValue{Condition: entry.Condition, Values: entry.Values}

		switch entry.Name {
		case "NAMESPACE_ID":
			f.NamespaceID = fv
		case "RESOURCE_OWNER":
			f.ResourceOwner = fv
		}
	}

	return f
}

func (h *Handler) handleListServices(_ context.Context, body []byte) ([]byte, error) {
	var req listServicesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	services := h.Backend.ListServices(buildServicesFilter(req.Filters))

	maxResults := maxResultsDefault
	if req.MaxResults != nil && *req.MaxResults > 0 {
		maxResults = *req.MaxResults
	}

	page, nextToken := applyPaginationServices(services, req.NextToken, maxResults)

	items := make([]map[string]any, 0, len(page))
	for i := range page {
		items = append(items, serviceSummaryToMap(&page[i]))
	}

	resp := map[string]any{
		"Services": items,
	}

	if nextToken != "" {
		resp["NextToken"] = nextToken
	}

	return json.Marshal(resp)
}

// serviceToMap converts a Service to a JSON-serialisable map including DNS and
// health check config, for the full types.Service shape (CreateService/
// GetService). Tags are intentionally NOT included: real Cloud Map's
// types.Service and types.ServiceSummary both omit Tags -- tags are only
// retrievable via ListTagsForResource.
func serviceToMap(svc *Service) map[string]any {
	m := serviceSummaryToMap(svc)
	m[keyNamespaceID] = svc.NamespaceID

	return m
}

// serviceSummaryToMap builds the types.ServiceSummary shape (types.go:1215)
// -- no top-level NamespaceId; unlike types.Service, ServiceSummary does not
// declare that member (confirmed against
// awsAwsjson11_deserializeDocumentServiceSummary). The nested, deprecated
// DnsConfig.NamespaceId is a distinct field shared by both shapes and is
// unaffected.
func serviceSummaryToMap(svc *Service) map[string]any {
	m := map[string]any{
		"Id":            svc.ID,
		keyArn:          svc.ARN,
		"Name":          svc.Name,
		"Description":   svc.Description,
		keyCreateDate:   awstime.Epoch(svc.CreatedAt),
		"InstanceCount": svc.InstanceCount,
	}

	if svc.Type != "" {
		m[keyType] = svc.Type
	}

	if svc.DNSConfig != nil {
		dc := map[string]any{
			keyNamespaceID:  svc.DNSConfig.NamespaceID,
			"RoutingPolicy": svc.DNSConfig.RoutingPolicy,
		}

		records := make([]map[string]any, 0, len(svc.DNSConfig.DNSRecords))
		for _, r := range svc.DNSConfig.DNSRecords {
			records = append(records, map[string]any{
				keyType: r.Type,
				"TTL":   r.TTL,
			})
		}

		dc["DnsRecords"] = records
		m["DnsConfig"] = dc
	}

	if svc.HealthCheckConfig != nil {
		m["HealthCheckConfig"] = map[string]any{
			keyType:            svc.HealthCheckConfig.Type,
			"ResourcePath":     svc.HealthCheckConfig.ResourcePath,
			"FailureThreshold": svc.HealthCheckConfig.FailureThreshold,
		}
	}

	if svc.HealthCheckCustomConfig != nil {
		m["HealthCheckCustomConfig"] = map[string]any{
			"FailureThreshold": svc.HealthCheckCustomConfig.FailureThreshold,
		}
	}

	return m
}

type updateServiceChange struct {
	DNSConfig         *dnsConfigRequest         `json:"DnsConfig"`
	HealthCheckConfig *healthCheckConfigRequest `json:"HealthCheckConfig"`
	Description       string                    `json:"Description"`
}

type updateServiceRequest struct {
	Service updateServiceChange `json:"Service"`
	ID      string              `json:"Id"`
}

func (h *Handler) handleUpdateService(_ context.Context, body []byte) ([]byte, error) {
	var req updateServiceRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if req.ID == "" {
		return nil, fmt.Errorf("%w: Id is required", errInvalidRequest)
	}

	dnsConfig := parseDNSConfig(req.Service.DNSConfig)
	if err := validateDNSConfigEnums(dnsConfig); err != nil {
		return nil, err
	}

	healthCheckConfig := parseHealthCheckConfig(req.Service.HealthCheckConfig)
	if err := validateHealthCheckConfigEnum(healthCheckConfig); err != nil {
		return nil, err
	}

	opID, err := h.Backend.UpdateService(
		req.ID,
		req.Service.Description,
		dnsConfig,
		healthCheckConfig,
	)
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]string{keyOperationID: opID})
}

type getServiceAttributesRequest struct {
	ServiceID string `json:"ServiceId"`
}

func (h *Handler) handleGetServiceAttributes(_ context.Context, body []byte) ([]byte, error) {
	var req getServiceAttributesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if req.ServiceID == "" {
		return nil, fmt.Errorf("%w: ServiceId is required", errInvalidRequest)
	}

	arn, attrs, err := h.Backend.GetServiceAttributes(req.ServiceID)
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]any{
		"ServiceAttributes": map[string]any{
			// ServiceAttributes.ServiceArn (deserializers.go:6001), not keyArn's
			// "Arn" -- unlike Service/Namespace, this shape's own field is named
			// ServiceArn.
			"ServiceArn":  arn,
			keyAttributes: attrs,
		},
	})
}

type updateServiceAttributesRequest struct {
	Attributes map[string]string `json:"Attributes"`
	ServiceID  string            `json:"ServiceId"`
}

func (h *Handler) handleUpdateServiceAttributes(_ context.Context, body []byte) error {
	var req updateServiceAttributesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if req.ServiceID == "" {
		return fmt.Errorf("%w: ServiceId is required", errInvalidRequest)
	}

	if err := validateServiceAttributeShape(req.Attributes); err != nil {
		return err
	}

	return h.Backend.UpdateServiceAttributes(req.ServiceID, req.Attributes)
}

type deleteServiceAttributesRequest struct {
	ServiceID  string   `json:"ServiceId"`
	Attributes []string `json:"Attributes"`
}

func (h *Handler) handleDeleteServiceAttributes(_ context.Context, body []byte) error {
	var req deleteServiceAttributesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if req.ServiceID == "" {
		return fmt.Errorf("%w: ServiceId is required", errInvalidRequest)
	}

	if len(req.Attributes) == 0 {
		return fmt.Errorf("%w: Attributes is required", errInvalidRequest)
	}

	return h.Backend.DeleteServiceAttributes(req.ServiceID, req.Attributes)
}

func applyPaginationServices(items []Service, nextToken string, maxResults int) ([]Service, string) {
	if maxResults <= 0 || maxResults > maxResultsCap {
		maxResults = maxResultsDefault
	}

	offset := decodeCursor(nextToken)
	if offset >= len(items) {
		return nil, ""
	}

	end := offset + maxResults

	var newToken string

	if end < len(items) {
		newToken = encodeCursor(end)
	} else {
		end = len(items)
	}

	return items[offset:end], newToken
}

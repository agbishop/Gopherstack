package servicediscovery

import (
	"slices"
	"strings"
	"time"
)

// DNSRecord represents a single DNS record configuration in a Cloud Map service.
type DNSRecord struct {
	Type string `json:"type"`
	TTL  int64  `json:"ttl"`
}

// DNSConfig holds the DNS configuration for a Cloud Map service.
type DNSConfig struct {
	NamespaceID   string      `json:"namespaceID,omitempty"`
	RoutingPolicy string      `json:"routingPolicy,omitempty"`
	DNSRecords    []DNSRecord `json:"dnsRecords,omitempty"`
}

// HealthCheckConfig holds the configuration for an AWS-managed HTTP/TCP health check.
type HealthCheckConfig struct {
	Type             string `json:"type"`
	ResourcePath     string `json:"resourcePath,omitempty"`
	FailureThreshold int    `json:"failureThreshold,omitempty"`
}

// HealthCheckCustomConfig holds the configuration for a custom health check.
type HealthCheckCustomConfig struct {
	FailureThreshold int `json:"failureThreshold,omitempty"`
}

// SOA holds the Start of Authority TTL for a DNS namespace.
type SOA struct {
	TTL int64 `json:"ttl"`
}

// DNSProperties holds the DNS-specific properties of a namespace.
type DNSProperties struct {
	SOA          *SOA   `json:"soa,omitempty"`
	HostedZoneID string `json:"hostedZoneId,omitempty"`
}

// HTTPProperties holds the HTTP-specific properties of a namespace.
type HTTPProperties struct {
	HTTPName string `json:"httpName,omitempty"`
}

// NamespaceProperties holds the type-specific properties of a namespace.
type NamespaceProperties struct {
	DNSProperties  *DNSProperties  `json:"dnsProperties,omitempty"`
	HTTPProperties *HTTPProperties `json:"httpProperties,omitempty"`
}

// Namespace represents an AWS Cloud Map namespace.
type Namespace struct {
	CreatedAt    time.Time            `json:"createdAt"`
	Tags         map[string]string    `json:"tags,omitempty"`
	Properties   *NamespaceProperties `json:"properties,omitempty"`
	ID           string               `json:"id"`
	ARN          string               `json:"arn"`
	Name         string               `json:"name"`
	Type         string               `json:"type"`
	Description  string               `json:"description,omitempty"`
	VPC          string               `json:"vpc,omitempty"`
	ServiceCount int                  `json:"serviceCount,omitempty"`
}

// Service represents an AWS Cloud Map service.
type Service struct {
	CreatedAt               time.Time                `json:"createdAt"`
	Tags                    map[string]string        `json:"tags,omitempty"`
	DNSConfig               *DNSConfig               `json:"dnsConfig,omitempty"`
	HealthCheckConfig       *HealthCheckConfig       `json:"healthCheckConfig,omitempty"`
	HealthCheckCustomConfig *HealthCheckCustomConfig `json:"healthCheckCustomConfig,omitempty"`
	ID                      string                   `json:"id"`
	ARN                     string                   `json:"arn"`
	Name                    string                   `json:"name"`
	NamespaceID             string                   `json:"namespaceID"`
	Description             string                   `json:"description,omitempty"`
	Type                    string                   `json:"type,omitempty"`
	InstanceCount           int                      `json:"instanceCount,omitempty"`
}

// Instance represents a registered instance in a Cloud Map service.
type Instance struct {
	Attributes map[string]string `json:"attributes,omitempty"`
	ID         string            `json:"id"`
	ServiceID  string            `json:"serviceID"`
}

// DNSRegistrar can register and deregister hostnames with an embedded DNS server.
// RegisterRecord stores the actual record value (IP for A/AAAA, hostname for CNAME).
type DNSRegistrar interface {
	RegisterRecord(hostname, recordType string, values []string)
	Deregister(hostname string)
}

// HostedZoneCreator is the subset of Route 53 operations Cloud Map needs to create a real
// hosted zone for a DNS namespace, wired via SetHostedZoneCreator. It is distinct from
// DNSRegistrar above: DNSRegistrar resolves individual instance hostnames, this creates the
// zone record itself. Returns the new zone's ID.
type HostedZoneCreator interface {
	CreateHostedZone(name, callerRef, comment string, private bool, vpcID, vpcRegion string) (string, error)
}

// DiscoveredInstance is the richer per-instance response for DiscoverInstances.
type DiscoveredInstance struct {
	Attributes    map[string]string
	InstanceID    string
	NamespaceName string
	ServiceName   string
	HealthStatus  string
}

// Operation represents an async Cloud Map operation (e.g., create/delete namespace).
type Operation struct {
	CreateDate   time.Time         `json:"createDate"`
	UpdateDate   time.Time         `json:"updateDate"`
	Targets      map[string]string `json:"targets,omitempty"`
	ID           string            `json:"id"`
	Type         string            `json:"type"`
	Status       string            `json:"status"`
	ErrorCode    string            `json:"errorCode,omitempty"`
	ErrorMessage string            `json:"errorMessage,omitempty"`
}

// FilterValue models one AWS ListXxxFilter entry: the Values to compare
// against and the comparison operator (Condition). An empty/unset Condition
// defaults to EQ, matching every ListXxxFilter's documented default. A zero
// FilterValue (no Values) means "no filter" and matches everything.
type FilterValue struct {
	Condition string
	Values    []string
}

// empty reports whether no filter was specified for this field.
func (f FilterValue) empty() bool { return len(f.Values) == 0 }

// matches reports whether actual satisfies this filter. Supported
// conditions: EQ (default, single-value equality), BEGINS_WITH (single-value
// prefix match), IN (actual must equal one of the values). An unrecognized
// Condition matches everything rather than rejecting the request, consistent
// with this backend's existing lenient-filter-parsing convention.
func (f FilterValue) matches(actual string) bool {
	if f.empty() {
		return true
	}

	switch f.Condition {
	case "", "EQ":
		return actual == f.Values[0]
	case "BEGINS_WITH":
		return strings.HasPrefix(actual, f.Values[0])
	case "IN":
		return slices.Contains(f.Values, actual)
	default:
		return true
	}
}

// resourceOwnerMatches evaluates a RESOURCE_OWNER FilterValue (Values one or
// both of "SELF"/"OTHER_ACCOUNTS") against this backend's single-account
// model, where every resource is always self-owned: it matches whenever
// "SELF" is among the requested values (or the filter is unset), and never
// matches an OTHER_ACCOUNTS-only request since no cross-account sharing is
// emulated.
func resourceOwnerMatches(f FilterValue) bool {
	if f.empty() {
		return true
	}

	return slices.Contains(f.Values, "SELF")
}

// ListNamespacesFilter contains optional filter parameters for ListNamespaces.
type ListNamespacesFilter struct {
	Type          FilterValue
	Name          FilterValue
	HTTPName      FilterValue
	ResourceOwner FilterValue
}

// ListServicesFilter contains optional filter parameters for ListServices.
type ListServicesFilter struct {
	NamespaceID   FilterValue
	ResourceOwner FilterValue
}

// ListOperationsFilter contains optional filter parameters for ListOperations.
type ListOperationsFilter struct {
	UpdateDateStart *time.Time
	UpdateDateEnd   *time.Time
	NamespaceID     FilterValue
	ServiceID       FilterValue
	Status          FilterValue
	Type            FilterValue
}

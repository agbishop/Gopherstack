package servicediscovery

import "context"

// StorageBackend defines the operations required from the Cloud Map storage layer.
// All methods must be safe for concurrent use.
type StorageBackend interface {
	// Namespace operations.
	CreateHTTPNamespace(name, description string, tags map[string]string) (string, error)
	CreatePrivateDNSNamespace(name, description, vpc string, soaTTL int64, tags map[string]string) (string, error)
	CreatePublicDNSNamespace(name, description string, soaTTL int64, tags map[string]string) (string, error)
	DeleteNamespace(id string) (string, error)
	GetNamespace(id string) (*Namespace, error)
	ListNamespaces(filter ListNamespacesFilter) []Namespace
	UpdateHTTPNamespace(id, description string) (string, error)
	UpdatePrivateDNSNamespace(id, description string, soaTTL int64, hasSOATTL bool) (string, error)
	UpdatePublicDNSNamespace(id, description string, soaTTL int64, hasSOATTL bool) (string, error)

	// Service operations.
	CreateService(
		name, namespaceID, description, svcType string,
		dnsConfig *DNSConfig,
		hcc *HealthCheckConfig,
		hccc *HealthCheckCustomConfig,
		tags map[string]string,
	) (*Service, error)
	DeleteService(id string) error
	GetService(id string) (*Service, error)
	ListServices(filter ListServicesFilter) []Service
	UpdateService(id, description string, dnsConfig *DNSConfig, hcc *HealthCheckConfig) (string, error)
	GetServiceAttributes(serviceID string) (string, map[string]string, error)
	UpdateServiceAttributes(serviceARN string, attributes map[string]string) error
	DeleteServiceAttributes(serviceID string, keys []string) error

	// Instance operations.
	RegisterInstance(serviceID, instanceID string, attrs map[string]string) (string, error)
	DeregisterInstance(serviceID, instanceID string) (string, error)
	GetInstance(serviceID, instanceID string) (*Instance, error)
	ListInstances(serviceID string) ([]Instance, error)
	DiscoverInstances(
		namespaceName, serviceName, healthStatus string,
		queryParams, optionalParams map[string]string,
	) ([]DiscoveredInstance, int64, error)
	DiscoverInstancesRevision(namespaceName, serviceName string) (int64, error)
	GetInstancesHealthStatus(serviceID string, instanceIDs []string) (map[string]string, error)
	UpdateInstanceCustomHealthStatus(serviceID, instanceID, status string) error

	// Operation operations.
	GetOperation(id string) (*Operation, error)
	ListOperations(filter ListOperationsFilter) []Operation

	// Tag operations.
	ListTagsForResource(arn string) (map[string]string, error)
	TagResource(arn string, tags map[string]string) error
	UntagResource(arn string, tagKeys []string) error

	// Lifecycle methods.
	Reset()
	Region() string
	AccountID() string
	Snapshot(ctx context.Context) []byte
	Restore(ctx context.Context, data []byte) error
}

// Ensure InMemoryBackend implements StorageBackend at compile time.
var _ StorageBackend = (*InMemoryBackend)(nil)

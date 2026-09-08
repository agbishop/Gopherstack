package elb

import (
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// Listener is a single protocol/port mapping on a load balancer.
type Listener struct {
	Protocol         string   `json:"protocol"`
	InstanceProtocol string   `json:"instanceProtocol"`
	SSLCertificateID string   `json:"sslCertificateId,omitempty"`
	PolicyNames      []string `json:"policyNames,omitempty"`
	LoadBalancerPort int32    `json:"loadBalancerPort"`
	InstancePort     int32    `json:"instancePort"`
}

// BackendServerDescription maps an instance port to the policies applied to it.
type BackendServerDescription struct {
	PolicyNames  []string `json:"policyNames"`
	InstancePort int32    `json:"instancePort"`
}

// AccessLog holds access-log configuration for a Classic ELB.
type AccessLog struct {
	S3BucketName   string `json:"s3BucketName"`
	S3BucketPrefix string `json:"s3BucketPrefix"`
	EmitInterval   int32  `json:"emitInterval"`
	Enabled        bool   `json:"enabled"`
}

// LoadBalancerAttributesMask marks which independent attribute groups of a
// ModifyLoadBalancerAttributes request were actually present in the caller's
// input. AWS's LoadBalancerAttributes groups (AccessLog, ConnectionDraining,
// ConnectionSettings, CrossZoneLoadBalancing) are each optional and
// independently settable; a group absent from the request must leave the
// load balancer's current value for that group untouched, not reset it to
// the service default.
type LoadBalancerAttributesMask struct {
	CrossZoneLoadBalancing bool
	ConnectionDraining     bool
	ConnectionSettings     bool
	AccessLog              bool
	DesyncMitigationMode   bool
}

// LoadBalancerAttributes holds tunable attributes for a Classic ELB.
type LoadBalancerAttributes struct {
	DesyncMitigationMode      string    `json:"desyncMitigationMode"`
	AccessLog                 AccessLog `json:"accessLog"`
	ConnectionDrainingTimeout int32     `json:"connectionDrainingTimeout"`
	IdleTimeout               int32     `json:"idleTimeout"`
	CrossZoneLoadBalancing    bool      `json:"crossZoneLoadBalancing"`
	ConnectionDraining        bool      `json:"connectionDraining"`
}

const (
	// defaultConnectionDrainingTimeout is the default connection-draining timeout in seconds.
	defaultConnectionDrainingTimeout int32 = 300

	// defaultIdleTimeout is the default idle connection timeout in seconds.
	defaultIdleTimeout int32 = 60

	// defaultAccessLogEmitInterval is the default access log emit interval in minutes.
	defaultAccessLogEmitInterval int32 = 60
)

// defaultLBAttributes returns the default LoadBalancerAttributes used at
// creation time, matching the AWS service defaults.
func defaultLBAttributes() LoadBalancerAttributes {
	return LoadBalancerAttributes{
		CrossZoneLoadBalancing:    false,
		ConnectionDraining:        false,
		ConnectionDrainingTimeout: defaultConnectionDrainingTimeout,
		IdleTimeout:               defaultIdleTimeout,
		DesyncMitigationMode:      "defensive",
		AccessLog:                 AccessLog{Enabled: false, EmitInterval: defaultAccessLogEmitInterval},
	}
}

// HealthCheck holds health-check configuration for a load balancer.
type HealthCheck struct {
	Target             string `json:"target"`
	Interval           int32  `json:"interval"`
	Timeout            int32  `json:"timeout"`
	UnhealthyThreshold int32  `json:"unhealthyThreshold"`
	HealthyThreshold   int32  `json:"healthyThreshold"`
}

// Instance is an EC2 instance registered with a load balancer.
type Instance struct {
	InstanceID string `json:"instanceId"`
}

// LoadBalancer represents a Classic ELB load balancer.
type LoadBalancer struct {
	CreatedTime               time.Time
	HealthCheck               *HealthCheck
	Tags                      *tags.Tags
	ARN                       string
	VPCId                     string
	Region                    string
	CanonicalHostedZoneName   string
	CanonicalHostedZoneNameID string
	Scheme                    string
	LoadBalancerName          string
	AccountID                 string
	DNSName                   string
	Listeners                 []Listener
	Instances                 []Instance
	BackendServerDescriptions []BackendServerDescription
	AvailabilityZones         []string
	SecurityGroups            []string
	Subnets                   []string
	Attributes                LoadBalancerAttributes
	IsVPC                     bool
}

// CreateLoadBalancerInput holds input for CreateLoadBalancer.
type CreateLoadBalancerInput struct {
	LoadBalancerName  string
	Scheme            string
	AvailabilityZones []string
	SecurityGroups    []string
	Subnets           []string
	Listeners         []Listener
}

// PolicyAttribute is a single attribute for a load balancer policy.
type PolicyAttribute struct {
	AttributeName  string `json:"attributeName"`
	AttributeValue string `json:"attributeValue"`
}

// LoadBalancerPolicy represents a Classic ELB policy.
type LoadBalancerPolicy struct {
	PolicyName                  string            `json:"policyName"`
	PolicyTypeName              string            `json:"policyTypeName"`
	LoadBalancerName            string            `json:"loadBalancerName"`
	Region                      string            `json:"region,omitempty"`
	PolicyAttributeDescriptions []PolicyAttribute `json:"policyAttributeDescriptions"`
}

// InstanceState represents the health state of a registered instance.
type InstanceState struct {
	InstanceID  string
	State       string
	ReasonCode  string
	Description string
}

// AccountLimit represents a single ELB account limit.
type AccountLimit struct {
	Name string
	Max  string
}

// PolicyAttributeTypeDescription describes the attributes of a policy type.
type PolicyAttributeTypeDescription struct {
	AttributeName string
	AttributeType string
	Cardinality   string
	DefaultValue  string
	Description   string
}

// PolicyTypeDescription describes a Classic ELB policy type.
type PolicyTypeDescription struct {
	PolicyTypeName                  string
	Description                     string
	PolicyAttributeTypeDescriptions []PolicyAttributeTypeDescription
}

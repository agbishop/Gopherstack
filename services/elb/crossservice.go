package elb

import "context"

// EC2Resolver lets this backend validate SecurityGroups/Subnets/instance ids
// passed to ApplySecurityGroupsToLoadBalancer/AttachLoadBalancerToSubnets/
// RegisterInstancesWithLoadBalancer against the real services/ec2 backend,
// mirroring services/directconnect's EC2GatewayResolver
// (services/directconnect/store.go) and services/networkmanager's
// EC2Resolver. Wired in by cli.go's wireComputeAndObservabilityIntegrations.
// A nil resolver (the default) accepts every security-group/subnet/instance
// id unvalidated -- e.g. isolated unit tests with no EC2 backend wired.
type EC2Resolver interface {
	SecurityGroupExists(id string) bool
	SubnetExists(id string) bool
	InstanceExists(id string) bool
}

// CertificateResolver lets this backend validate an HTTPS/SSL listener's
// SSLCertificateId against the real services/acm and services/iam backends.
// AWS accepts either an ACM or an IAM server-certificate ARN here --
// aws-sdk-go-v2/service/elasticloadbalancing@v1.36.4 types/errors.go:36-39's
// CertificateNotFoundException doc comment: "does not refer to a valid SSL
// certificate in AWS Identity and Access Management (IAM) or AWS Certificate
// Manager (ACM)" -- so both must be consulted. Wired in by cli.go's
// wireComputeAndObservabilityIntegrations. A nil resolver (the default)
// accepts every SSLCertificateId unvalidated.
type CertificateResolver interface {
	ResolveCertificate(ctx context.Context, certARN string) bool
}

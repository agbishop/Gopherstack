package elbv2

// EC2Resolver lets this backend validate SecurityGroups/Subnets passed to
// CreateLoadBalancer against the real services/ec2 backend, mirroring
// services/elb's EC2Resolver. Wired in by cli.go's wireELBv2CrossService. A
// nil resolver (the default) accepts every security-group/subnet id
// unvalidated -- e.g. isolated unit tests with no EC2 backend wired.
type EC2Resolver interface {
	SecurityGroupExists(id string) bool
	SubnetExists(id string) bool
}

// CertificateResolver lets this backend validate a listener's
// CertificateArn against the real services/acm and services/iam backends,
// and report attach/detach so ACM's InUseBy tracking -- and therefore
// DeleteCertificate's ResourceInUseException guard -- becomes reachable.
// Wired in by cli.go's wireELBv2CrossService. A nil resolver (the default)
// accepts every CertificateArn unvalidated and reports no usage.
type CertificateResolver interface {
	ResolveCertificate(certARN string) bool
	AddInUseBy(certARN, resourceARN string)
	RemoveInUseBy(certARN, resourceARN string)
}

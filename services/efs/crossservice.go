package efs

// EC2Resolver lets this backend validate CreateMountTarget's SubnetId
// against the real services/ec2 backend so it can enforce
// api_op_CreateMountTarget.go's documented placement rule: "you can create
// mount targets for a file system in only one VPC, and there can be only
// one mount target per Availability Zone" -- if the file system already has
// one or more mount targets, a new subnet "Must belong to the same VPC as
// the subnets of the existing mount targets" and "Must not be in the same
// Availability Zone as any of the subnets of the existing mount targets".
// Mirrors services/elb's EC2Resolver. Wired in by cli.go's
// wireComputeAndObservabilityIntegrations. A nil resolver (the default)
// accepts every subnet ID unvalidated -- e.g. isolated unit tests with no
// EC2 backend wired.
type EC2Resolver interface {
	SubnetExists(id string) bool
	SubnetVPC(id string) string
	SubnetAZ(id string) string
}

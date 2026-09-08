package lambda

// ECRResolver lets this backend validate a Code.ImageUri against the real
// services/ecr backend at CreateFunction/UpdateFunctionCode time. Real AWS
// rejects an Image package-type function whose image does not exist in ECR
// immediately, at create/update time, with InvalidParameterValueException
// ("Source image <uri> does not exist. Provide a valid source image.") --
// not only later at pull time. Wired in by cli.go's wireLambdaECR. A nil
// resolver (the default) accepts every ImageUri unvalidated, matching
// services/networkmanager's EC2Resolver pattern for isolated unit tests
// that never wire a cross-service backend.
type ECRResolver interface {
	ResolveImage(imageURI string) bool
}

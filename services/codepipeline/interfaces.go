package codepipeline

import "context"

// CodeBuildStarter is the subset of CodeBuild operations CodePipeline needs
// to run a Build/CodeBuild action, wired via SetCodeBuildBackend. When
// unset, Build actions complete instantly with no cross-service call,
// matching this backend's original behavior.
type CodeBuildStarter interface {
	// StartBuild starts a build for the named CodeBuild project. An error
	// (e.g. the project does not exist) fails the pipeline action.
	StartBuild(projectName string) error
}

// LambdaInvoker is the subset of Lambda operations CodePipeline needs to run
// an Invoke/Lambda action, wired via SetLambdaBackend. When unset, Invoke
// actions complete instantly with no cross-service call, matching this
// backend's original behavior. Same shape as the LambdaInvoker interface
// repeated across sns, stepfunctions/asl, eventbridge, etc. --
// lambda.InMemoryBackend's InvokeFunction (invocationType aliased to string)
// satisfies it directly with no adapter.
type LambdaInvoker interface {
	InvokeFunction(
		ctx context.Context,
		name, invocationType string,
		payload []byte,
	) ([]byte, int, error)
}

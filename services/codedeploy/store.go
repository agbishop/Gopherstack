// Package codedeploy provides an in-memory implementation of the AWS CodeDeploy service.
package codedeploy

import (
	"fmt"

	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

const (
	statusSucceeded       = "Succeeded"
	statusStopped         = "Stopped"
	statusFailed          = "Failed"
	statusReady           = "Ready"
	computePlatformServer = "Server"
	computePlatformLambda = "Lambda"
	computePlatformECS    = "ECS"
)

// DeploymentWaitType enum values accepted by ContinueDeployment. types.DeploymentWaitType
// Values(), aws-sdk-go-v2/service/codedeploy@v1.38.4/types/enums.go:247-251.
const (
	waitTypeReadyWait       = "READY_WAIT"
	waitTypeTerminationWait = "TERMINATION_WAIT"
)

// Deployment target/instance type discriminators shared between the backend
// computation (deployment_instances.go) and its wire conversion
// (handler_deployment_instances.go).
const (
	targetTypeInstance = "instanceTarget"
	targetTypeECS      = "ecsTarget"
	targetTypeLambda   = "lambdaTarget"
)

// InMemoryBackend is the in-memory store for CodeDeploy resources.
//
// deployments, deploymentConfigs, and applicationRevisions carry a real,
// wire-visible identity field and no live *tags.Tags field, so each
// registers directly on b.registry as a "clean" *store.Table.
// applicationRevisions is keyed by a composite appName+canonical-revision-JSON
// string (see applicationRevisionKey in store_setup.go), with
// applicationRevisionsByApp replacing a per-application scan for
// ListApplicationRevisions.
//
// applications, deploymentGroups, and onPremisesInstances each carry a live
// *tags.Tags field marked json:"-", so each is a "dirty" table (store.New
// only, NOT store.Register-ed onto b.registry -- see store_setup.go)
// round-tripped through a DTO wrapper in persistence.go. deploymentGroups
// was previously nested by application; it flattens to one *store.Table
// keyed by the composite "appName/dgName" string (see dgKey in
// store_setup.go), with deploymentGroupsByApp replacing the old
// map[string]map[string]*DeploymentGroup nesting for per-application scans.
//
// githubTokens is deliberately NOT converted: it is an identity-less set
// (map[string]struct{}), so there is no *T value for store.Table to key on.
// It remains a plain map, unchanged by this refactor.
type InMemoryBackend struct {
	registry                  *store.Registry
	applications              *store.Table[Application]
	deploymentGroups          *store.Table[DeploymentGroup]
	deploymentGroupsByApp     *store.Index[DeploymentGroup]
	deployments               *store.Table[Deployment]
	onPremisesInstances       *store.Table[OnPremisesInstance]
	deploymentConfigs         *store.Table[DeploymentConfig]
	applicationRevisions      *store.Table[ApplicationRevision]
	applicationRevisionsByApp *store.Index[ApplicationRevision]
	githubTokens              map[string]struct{}
	mu                        *lockmetrics.RWMutex
	appConfig                 any
	accountID                 string
	region                    string
}

// NewInMemoryBackend creates a new in-memory CodeDeploy backend with pre-seeded default configs.
func NewInMemoryBackend(accountID, region string) *InMemoryBackend {
	b := &InMemoryBackend{
		registry:     store.NewRegistry(),
		githubTokens: make(map[string]struct{}),
		accountID:    accountID,
		region:       region,
		mu:           lockmetrics.New("codedeploy"),
	}

	registerAllTables(b)
	b.seedDefaultConfigs()

	return b
}

// Reset clears all state, returning the backend to a fresh empty state (with default configs re-seeded).
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	for _, app := range b.applications.All() {
		if app.Tags != nil {
			app.Tags.Close()
		}
	}

	for _, dg := range b.deploymentGroups.All() {
		if dg.Tags != nil {
			dg.Tags.Close()
		}
	}

	for _, inst := range b.onPremisesInstances.All() {
		if inst.Tags != nil {
			inst.Tags.Close()
		}
	}

	b.registry.ResetAll()
	b.applications.Reset()
	b.deploymentGroups.Reset()
	b.onPremisesInstances.Reset()
	b.githubTokens = make(map[string]struct{})

	b.seedDefaultConfigs()
}

// ensureTags returns the given tags if non-nil, or creates a new tags.Tags with the given key.
func ensureTags(existing *tags.Tags, key string) *tags.Tags {
	if existing != nil {
		return existing
	}

	return tags.New(key)
}

// Region returns the AWS region this backend is configured for.
func (b *InMemoryBackend) Region() string { return b.region }

// validateComputePlatform returns an error if the given platform is not a valid CodeDeploy compute platform.
func validateComputePlatform(platform string) error {
	if _, ok := validComputePlatforms()[platform]; !ok {
		return fmt.Errorf("%w: invalid computePlatform %q, must be Server, Lambda, or ECS",
			ErrInvalidComputePlatform, platform)
	}

	return nil
}

// validComputePlatforms lists the accepted CodeDeploy compute platforms.
func validComputePlatforms() map[string]struct{} {
	return map[string]struct{}{
		computePlatformServer: {},
		computePlatformLambda: {},
		computePlatformECS:    {},
	}
}

// fileExistsBehavior enum values. types.FileExistsBehavior Values(),
// aws-sdk-go-v2/service/codedeploy@v1.38.4/types/enums.go:362-364.
const (
	fileExistsBehaviorDisallow  = "DISALLOW"
	fileExistsBehaviorOverwrite = "OVERWRITE"
	fileExistsBehaviorRetain    = "RETAIN"
)

// validateFileExistsBehavior returns an error if behavior is set to a value
// other than the empty string (real AWS default) or a real FileExistsBehavior
// enum value.
func validateFileExistsBehavior(behavior string) error {
	switch behavior {
	case "", fileExistsBehaviorDisallow, fileExistsBehaviorOverwrite, fileExistsBehaviorRetain:
		return nil
	default:
		return fmt.Errorf("%w: invalid fileExistsBehavior %q, must be DISALLOW, OVERWRITE, or RETAIN",
			ErrInvalidFileExistsBehavior, behavior)
	}
}

// ec2TagFilterType enum values. types.EC2TagFilterType Values(),
// aws-sdk-go-v2/service/codedeploy@v1.38.4/types/enums.go:258-260.
const (
	ec2TagFilterTypeKeyOnly     = "KEY_ONLY"
	ec2TagFilterTypeValueOnly   = "VALUE_ONLY"
	ec2TagFilterTypeKeyAndValue = "KEY_AND_VALUE"
)

// validateDeploymentGroupTagFilters rejects a DeploymentGroupInput that
// specifies both halves of the Ec2TagFilters/Ec2TagSet or
// OnPremisesInstanceTagFilters/OnPremisesTagSet pairs (only one of each pair
// may be used per call: types/errors.go:1579-1580 and 2122-2123 in
// aws-sdk-go-v2/service/codedeploy@v1.38.4), and rejects an Ec2TagFilters
// entry whose Type is set but isn't a real EC2TagFilterType value. validators.go
// does not require EC2TagFilter.Type, so an empty Type stays legal.
func validateDeploymentGroupTagFilters(input DeploymentGroupInput) error {
	if len(input.Ec2TagFilters) > 0 && input.Ec2TagSet != nil {
		return fmt.Errorf("%w: cannot specify both ec2TagFilters and ec2TagSet", ErrInvalidEC2TagCombination)
	}

	if len(input.OnPremisesInstanceTagFilters) > 0 && input.OnPremisesTagSet != nil {
		return fmt.Errorf("%w: cannot specify both onPremisesInstanceTagFilters and onPremisesTagSet",
			ErrInvalidOnPremisesTagCombination)
	}

	for _, f := range input.Ec2TagFilters {
		switch f.Type {
		case "", ec2TagFilterTypeKeyOnly, ec2TagFilterTypeValueOnly, ec2TagFilterTypeKeyAndValue:
			continue
		default:
			return fmt.Errorf("%w: invalid ec2TagFilters type %q, must be KEY_ONLY, VALUE_ONLY, or KEY_AND_VALUE",
				ErrInvalidEC2Tag, f.Type)
		}
	}

	return nil
}

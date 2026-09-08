package ecs

import (
	"errors"
	"fmt"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

// ErrNilAppContext is returned when Provider.Init is called with a nil AppContext.
var ErrNilAppContext = errors.New("AppContext is required")

// taskCompletionRunner is implemented by TaskRunners that can report when a
// container they started exits on its own, not via an explicit StopTask call
// (currently only realDockerRunner). Runners that don't implement it
// (noopRunner, test fakes) stay unwired: their tasks only reach STOPPED via
// StopTask.
type taskCompletionRunner interface {
	SetTaskCompletionHandler(fn func(taskArn, containerName string, exitCode int))
}

// Provider implements service.Provider for Amazon ECS.
type Provider struct{}

// Name returns the provider name.
func (p *Provider) Name() string { return "ECS" }

// Init initializes the ECS service backend and handler.
//
//nolint:ireturn,nolintlint // architecturally required to return interface
func (p *Provider) Init(appCtx *service.AppContext) (service.Registerable, error) {
	if appCtx == nil {
		return nil, ErrNilAppContext
	}

	accountID := config.DefaultAccountID
	region := config.DefaultRegion

	if cfgProvider, ok := appCtx.Config.(config.Provider); ok {
		cfg := cfgProvider.GetGlobalConfig()
		if cfg.GetAccountID() != "" {
			accountID = cfg.GetAccountID()
		}

		if cfg.GetRegion() != "" {
			region = cfg.GetRegion()
		}
	}

	runner, err := newTaskRunner(appCtx.JanitorCtx)
	if err != nil {
		return nil, fmt.Errorf("init ECS task runner: %w", err)
	}

	backend := NewInMemoryBackend(accountID, region, runner)

	if r, ok := runner.(taskCompletionRunner); ok {
		r.SetTaskCompletionHandler(backend.markTaskStoppedByContainerExit)
	}

	reconciler := NewReconciler(backend)
	janitor := NewJanitor(backend, 0)

	// Evict per-cluster reconciler state when a cluster is deleted or purged so
	// Reconciler.sems does not grow one permanent entry per cluster ever created.
	backend.RegisterClusterDeleteHook(reconciler.EvictCluster)

	if appCtx.JanitorCtx != nil {
		go reconciler.Start(appCtx.JanitorCtx)
		go janitor.Run(appCtx.JanitorCtx)
	}

	appCtx.Logger.Info("ECS service initialized")

	return NewHandler(backend), nil
}

// compile-time assertion that Provider implements service.Provider.
var _ service.Provider = (*Provider)(nil)

package codedeploy

import (
	"fmt"
	"math/rand/v2"
	"sort"
	"time"
)

const (
	deployIDChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	deployIDLen   = 9

	defaultDeploymentCreator = "user"
)

// simulatedDeployDuration is the simulated time for a deployment to complete.
const simulatedDeployDuration = 5 * time.Second

// generateDeploymentID produces an AWS-format deployment ID: d- followed by 9 uppercase alphanumeric chars.
func generateDeploymentID() string {
	b := make([]byte, deployIDLen)
	for i := range b {
		b[i] = deployIDChars[rand.IntN(len(deployIDChars))] //nolint:gosec // non-crypto ID for test mock
	}

	return "d-" + string(b)
}

// CreateDeployment creates a new deployment.
func (b *InMemoryBackend) CreateDeployment(appName, dgName string, opts DeploymentOptions) (*Deployment, error) {
	b.mu.Lock("CreateDeployment")
	defer b.mu.Unlock()

	if !b.applications.Has(appName) {
		return nil, fmt.Errorf("%w: application %s not found", ErrNotFound, appName)
	}

	dg, ok := b.deploymentGroups.Get(dgKey(appName, dgName))
	if !ok {
		return nil, fmt.Errorf("%w: deployment group %s not found", ErrDeploymentGroupNotFound, dgName)
	}

	if err := validateFileExistsBehavior(opts.FileExistsBehavior); err != nil {
		return nil, err
	}

	if opts.Creator == "" {
		opts.Creator = defaultDeploymentCreator
	}

	deployID := generateDeploymentID()
	now := time.Now().UTC()
	completed := now.Add(simulatedDeployDuration)

	d := &Deployment{
		DeploymentID:                  deployID,
		ApplicationName:               appName,
		DeploymentGroupName:           dgName,
		DeploymentConfigName:          dg.DeploymentConfigName,
		Status:                        statusSucceeded,
		Creator:                       opts.Creator,
		Description:                   opts.Description,
		FileExistsBehavior:            opts.FileExistsBehavior,
		UpdateOutdatedInstancesOnly:   opts.UpdateOutdatedInstancesOnly,
		IgnoreApplicationStopFailures: opts.IgnoreApplicationStopFailures,
		Revision:                      opts.Revision,
		CreateTime:                    now,
		CompleteTime:                  &completed,
		AccountID:                     b.accountID,
		Region:                        b.region,
	}
	b.deployments.Put(d)
	b.touchApplicationRevisionForDeployment(appName, dgName, opts.Revision)

	cp := *d

	return &cp, nil
}

// GetDeployment returns a deployment by ID.
func (b *InMemoryBackend) GetDeployment(deploymentID string) (*Deployment, error) {
	b.mu.RLock("GetDeployment")
	defer b.mu.RUnlock()

	d, ok := b.deployments.Get(deploymentID)
	if !ok {
		return nil, fmt.Errorf("%w: deployment %s not found", ErrDeploymentNotFound, deploymentID)
	}

	cp := *d

	return &cp, nil
}

// deploymentMatchesFilter reports whether d passes every criterion in filter.
// statusSet is the precomputed set of filter.Statuses (empty means no status filter).
func deploymentMatchesFilter(d *Deployment, filter DeploymentFilter, statusSet map[string]struct{}) bool {
	if filter.ApplicationName != "" && d.ApplicationName != filter.ApplicationName {
		return false
	}

	if filter.DeploymentGroupName != "" && d.DeploymentGroupName != filter.DeploymentGroupName {
		return false
	}

	if filter.ExternalID != "" && d.ExternalID != filter.ExternalID {
		return false
	}

	if len(statusSet) > 0 {
		if _, ok := statusSet[d.Status]; !ok {
			return false
		}
	}

	if filter.CreateTimeStart != nil && d.CreateTime.Before(*filter.CreateTimeStart) {
		return false
	}

	if filter.CreateTimeEnd != nil && d.CreateTime.After(*filter.CreateTimeEnd) {
		return false
	}

	return true
}

// ListDeployments returns deployment IDs in sorted order, filtered by the provided criteria.
func (b *InMemoryBackend) ListDeployments(filter DeploymentFilter) []string {
	b.mu.RLock("ListDeployments")
	defer b.mu.RUnlock()

	statusSet := make(map[string]struct{}, len(filter.Statuses))
	for _, s := range filter.Statuses {
		statusSet[s] = struct{}{}
	}

	all := b.deployments.All()
	ids := make([]string, 0, len(all))

	for _, d := range all {
		if deploymentMatchesFilter(d, filter, statusSet) {
			ids = append(ids, d.DeploymentID)
		}
	}

	sort.Strings(ids)

	return ids
}

// StopDeployment marks a deployment as Stopped. Real AWS rejects stopping a
// deployment that is already in a terminal state (types/errors.go:221, "The
// deployment is already complete."); Stopped is the only terminal state this
// backend's instant-completion CreateDeployment can ever reach through a
// second StopDeployment call, since it always creates deployments Succeeded.
func (b *InMemoryBackend) StopDeployment(deploymentID string) error {
	b.mu.Lock("StopDeployment")
	defer b.mu.Unlock()

	d, ok := b.deployments.Get(deploymentID)
	if !ok {
		return fmt.Errorf("%w: deployment %s not found", ErrDeploymentNotFound, deploymentID)
	}

	if d.Status == statusStopped {
		return fmt.Errorf("%w: deployment %s is already complete", ErrDeploymentAlreadyCompleted, deploymentID)
	}

	d.Status = statusStopped

	return nil
}

// ContinueDeployment marks a blue/green deployment as continuing past the wait point.
// Real AWS requires the deployment to be in Ready status (types/errors.go:556-557,
// "The deployment does not have a status of Ready and can't continue yet.");
// terminal statuses get DeploymentAlreadyCompletedException instead
// (types/errors.go:221, "The deployment is already complete.").
func (b *InMemoryBackend) ContinueDeployment(deploymentID string) error {
	b.mu.Lock("ContinueDeployment")
	defer b.mu.Unlock()

	d, ok := b.deployments.Get(deploymentID)
	if !ok {
		return fmt.Errorf("%w: deployment %s not found", ErrDeploymentNotFound, deploymentID)
	}

	switch d.Status {
	case statusReady:
		return nil
	case statusSucceeded, statusFailed, statusStopped:
		return fmt.Errorf("%w: deployment %s is already complete", ErrDeploymentAlreadyCompleted, deploymentID)
	default:
		return fmt.Errorf(
			"%w: deployment %s does not have a status of Ready", ErrDeploymentNotInReadyState, deploymentID,
		)
	}
}

// LastDeploymentsForGroup returns the most recently attempted and most
// recently successful deployment for an application/deployment-group pair
// (nil if none exist for that outcome). Mirrors ListDeployments' own
// scan-based lookup -- there is no per-group deployment index.
func (b *InMemoryBackend) LastDeploymentsForGroup(appName, dgName string) (*Deployment, *Deployment) {
	b.mu.RLock("LastDeploymentsForGroup")
	defer b.mu.RUnlock()

	var attempted, successful *Deployment

	for _, d := range b.deployments.All() {
		if d.ApplicationName != appName || d.DeploymentGroupName != dgName {
			continue
		}

		if attempted == nil || d.CreateTime.After(attempted.CreateTime) {
			cp := *d
			attempted = &cp
		}

		if d.Status == statusSucceeded && (successful == nil || d.CreateTime.After(successful.CreateTime)) {
			cp := *d
			successful = &cp
		}
	}

	return attempted, successful
}

// BatchGetDeployments returns deployment structs for the given IDs.
// Deployment IDs that do not exist are silently omitted.
func (b *InMemoryBackend) BatchGetDeployments(deploymentIDs []string) []*Deployment {
	b.mu.RLock("BatchGetDeployments")
	defer b.mu.RUnlock()

	result := make([]*Deployment, 0, len(deploymentIDs))

	for _, id := range deploymentIDs {
		d, ok := b.deployments.Get(id)
		if !ok {
			continue
		}

		cp := *d
		result = append(result, &cp)
	}

	return result
}

// AddDeploymentInternal adds a deployment directly to the backend without validation.
// Used for test seeding only.
func (b *InMemoryBackend) AddDeploymentInternal(d *Deployment) {
	b.mu.Lock("AddDeploymentInternal")
	defer b.mu.Unlock()

	if d.DeploymentID == "" {
		d.DeploymentID = generateDeploymentID()
	}

	if d.CreateTime.IsZero() {
		d.CreateTime = time.Now().UTC()
	}

	b.deployments.Put(d)
}

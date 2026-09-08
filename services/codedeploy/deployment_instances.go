package codedeploy

import (
	"fmt"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/strs"

	ec2backend "github.com/blackbirdworks/gopherstack/services/ec2"
)

// targetStatusForDeployment maps a Deployment's own lifecycle Status to the
// TargetStatus enum value every target (instance/ECS/Lambda) participating in
// it should report, instead of the fixed "Succeeded" this backend used to
// return regardless of the deployment's real status.
func targetStatusForDeployment(deploymentStatus string) string {
	switch deploymentStatus {
	case statusSucceeded:
		return "Succeeded"
	case statusFailed:
		return statusFailed
	case statusStopped:
		return "Skipped"
	default:
		return "InProgress"
	}
}

// deploymentTargets computes the real set of participants in a deployment
// from the deployment's owning deployment group configuration: matched
// on-premises instances for the Server compute platform, one target per
// configured ECS service for ECS, or a single synthetic target for Lambda
// (real CodeDeploy Lambda/ECS deployments always have exactly one and,
// respectively, len(ECSServices) targets -- there is no "discover running
// EC2 instances" step this backend can perform without a live services/ec2
// integration, so the Server case is limited to instances this service
// itself tracks: registered on-premises instances). Results are sorted by
// TargetID for deterministic List/Batch output. If the deployment's
// deployment group has since been deleted, returns an empty (not error)
// slice: the deployment record itself still exists and is a valid target for
// Get/List, it simply now resolves to zero participants.
func (b *InMemoryBackend) deploymentTargets(d *Deployment) []DeploymentTargetRecord {
	dg, ok := b.deploymentGroups.Get(dgKey(d.ApplicationName, d.DeploymentGroupName))
	if !ok {
		return nil
	}

	status := targetStatusForDeployment(d.Status)
	updatedAt := d.CreateTime
	if d.CompleteTime != nil {
		updatedAt = *d.CompleteTime
	}

	var targets []DeploymentTargetRecord

	switch dg.ComputePlatform {
	case computePlatformECS:
		targets = b.ecsDeploymentTargets(d, dg, status, updatedAt)
	case computePlatformLambda:
		targets = []DeploymentTargetRecord{{
			DeploymentID:  d.DeploymentID,
			TargetID:      d.DeploymentID,
			TargetType:    targetTypeLambda,
			Status:        status,
			TargetArn:     arn.Build("codedeploy", b.region, b.accountID, "deploymentgroup:"+d.DeploymentGroupName),
			LastUpdatedAt: updatedAt,
		}}
	default: // Server (EC2/On-premises)
		targets = b.instanceDeploymentTargets(d, dg, status, updatedAt)
	}

	sort.Slice(targets, func(i, j int) bool { return targets[i].TargetID < targets[j].TargetID })

	return targets
}

// ecsDeploymentTargets builds one ecsTarget per ECS service configured on
// the deployment group -- real, already-tracked backend data rather than a
// fabricated identity.
func (b *InMemoryBackend) ecsDeploymentTargets(
	d *Deployment, dg *DeploymentGroup, status string, updatedAt time.Time,
) []DeploymentTargetRecord {
	targets := make([]DeploymentTargetRecord, 0, len(dg.ECSServices))

	for _, svc := range dg.ECSServices {
		targetID := svc.ClusterName + "/" + svc.ServiceName
		targets = append(targets, DeploymentTargetRecord{
			DeploymentID:  d.DeploymentID,
			TargetID:      targetID,
			TargetType:    targetTypeECS,
			Status:        status,
			TargetArn:     arn.Build("codedeploy", b.region, b.accountID, "deploymentgroup:"+d.DeploymentGroupName),
			ClusterName:   svc.ClusterName,
			ServiceName:   svc.ServiceName,
			LastUpdatedAt: updatedAt,
		})
	}

	return targets
}

// instanceDeploymentTargets builds one instanceTarget per registered,
// currently-registered (not deregistered) on-premises instance whose tags
// satisfy the deployment group's on-premises targeting configuration, plus
// one instanceTarget per non-terminated services/ec2 instance whose tags
// satisfy Ec2TagFilters/Ec2TagSet, when the EC2 backend is wired (see
// cross_service.go). Without EC2 wired (e.g. unit tests constructing
// InMemoryBackend directly), Ec2TagFilters/Ec2TagSet still match zero
// instances -- a documented simplification, not silently fabricated.
func (b *InMemoryBackend) instanceDeploymentTargets(
	d *Deployment, dg *DeploymentGroup, status string, updatedAt time.Time,
) []DeploymentTargetRecord {
	var targets []DeploymentTargetRecord

	for _, inst := range b.onPremisesInstances.All() {
		if inst.DeregisterTime != nil {
			continue
		}

		if !matchesOnPremisesTargeting(inst.Tags, dg) {
			continue
		}

		targets = append(targets, DeploymentTargetRecord{
			DeploymentID:  d.DeploymentID,
			TargetID:      inst.InstanceName,
			TargetType:    targetTypeInstance,
			Status:        status,
			TargetArn:     arn.Build("codedeploy", b.region, b.accountID, "instance:"+inst.InstanceName),
			InstanceLabel: "BLUE",
			LastUpdatedAt: updatedAt,
		})
	}

	targets = append(targets, b.ec2DeploymentTargets(d, dg, status, updatedAt)...)

	return targets
}

// ec2DeploymentTargets builds one instanceTarget per services/ec2 instance
// whose tags satisfy dg's Ec2TagFilters/Ec2TagSet, excluding instances that
// are terminated or terminating (TerminateInstances moves an instance to
// shutting-down immediately and to terminated only once the ec2 backend's
// background reconciler advances it -- see ec2/instances.go:921's "any state
// -> shutting-down -> terminated" comment -- so both states must be excluded,
// not just the terminal one, or a just-terminated instance stays targetable
// until the reconciler catches up). Returns nil when the EC2 backend isn't wired.
func (b *InMemoryBackend) ec2DeploymentTargets(
	d *Deployment, dg *DeploymentGroup, status string, updatedAt time.Time,
) []DeploymentTargetRecord {
	ec2Bk, ok := b.ec2Backend()
	if !ok {
		return nil
	}

	var targets []DeploymentTargetRecord

	for _, inst := range ec2Bk.DescribeInstances(nil, "") {
		if inst.State.Name == ec2backend.StateTerminated.Name || inst.State.Name == ec2backend.StateShuttingDown.Name {
			continue
		}

		if !matchesEc2Targeting(ec2Bk.TagsForResource(inst.ID), dg) {
			continue
		}

		targets = append(targets, DeploymentTargetRecord{
			DeploymentID:  d.DeploymentID,
			TargetID:      inst.ID,
			TargetType:    targetTypeInstance,
			Status:        status,
			TargetArn:     arn.Build("ec2", b.region, b.accountID, "instance/"+inst.ID),
			InstanceLabel: "BLUE",
			LastUpdatedAt: updatedAt,
		})
	}

	return targets
}

// GetDeploymentTarget returns a single computed deployment target by ID.
func (b *InMemoryBackend) GetDeploymentTarget(deploymentID, targetID string) (*DeploymentTargetRecord, error) {
	b.mu.RLock("GetDeploymentTarget")
	defer b.mu.RUnlock()

	d, ok := b.deployments.Get(deploymentID)
	if !ok {
		return nil, fmt.Errorf("%w: deployment %s not found", ErrDeploymentNotFound, deploymentID)
	}

	for _, t := range b.deploymentTargets(d) {
		if t.TargetID == targetID {
			cp := t

			return &cp, nil
		}
	}

	return nil, fmt.Errorf(
		"%w: target %s not found for deployment %s", ErrDeploymentTargetNotFound, targetID, deploymentID,
	)
}

// ListDeploymentTargets returns the sorted target IDs participating in a
// deployment, narrowed by filter's TargetStatus/ServerInstanceLabel values
// (the two TargetFilterName keys the real API defines).
func (b *InMemoryBackend) ListDeploymentTargets(deploymentID string, filter TargetListFilter) ([]string, error) {
	b.mu.RLock("ListDeploymentTargets")
	defer b.mu.RUnlock()

	d, ok := b.deployments.Get(deploymentID)
	if !ok {
		return nil, fmt.Errorf("%w: deployment %s not found", ErrDeploymentNotFound, deploymentID)
	}

	targets := b.deploymentTargets(d)
	ids := make([]string, 0, len(targets))
	for _, t := range targets {
		if !targetMatchesFilter(t, filter) {
			continue
		}
		ids = append(ids, t.TargetID)
	}

	return ids, nil
}

// targetMatchesFilter reports whether t satisfies a ListDeploymentTargets
// targetFilters map (empty per-key lists impose no constraint for that key).
func targetMatchesFilter(t DeploymentTargetRecord, filter TargetListFilter) bool {
	if len(filter.TargetStatus) > 0 && !strs.ContainsFold(filter.TargetStatus, t.Status) {
		return false
	}

	if len(filter.ServerInstanceLabel) > 0 && !strs.ContainsFold(filter.ServerInstanceLabel, t.InstanceLabel) {
		return false
	}

	return true
}

// BatchGetDeploymentTargets returns the computed targets matching the given
// target IDs. IDs that do not resolve to a real target are silently omitted,
// matching this codebase's Batch* "not found -> omitted" convention.
func (b *InMemoryBackend) BatchGetDeploymentTargets(
	deploymentID string, targetIDs []string,
) ([]*DeploymentTargetRecord, error) {
	b.mu.RLock("BatchGetDeploymentTargets")
	defer b.mu.RUnlock()

	d, ok := b.deployments.Get(deploymentID)
	if !ok {
		return nil, fmt.Errorf("%w: deployment %s not found", ErrDeploymentNotFound, deploymentID)
	}

	byID := make(map[string]*DeploymentTargetRecord)
	for _, t := range b.deploymentTargets(d) {
		cp := t
		byID[t.TargetID] = &cp
	}

	result := make([]*DeploymentTargetRecord, 0, len(targetIDs))

	for _, id := range targetIDs {
		if t, found := byID[id]; found {
			result = append(result, t)
		}
	}

	return result, nil
}

// ListDeploymentInstances returns instance IDs for a Server/Lambda-platform
// deployment (ECS deployments have no per-instance concept and always
// resolve to an empty list here, matching the legacy API's EC2/On-premises +
// Lambda scope).
func (b *InMemoryBackend) ListDeploymentInstances(deploymentID string, filter InstanceListFilter) ([]string, error) {
	b.mu.RLock("ListDeploymentInstances")
	defer b.mu.RUnlock()

	d, ok := b.deployments.Get(deploymentID)
	if !ok {
		return nil, fmt.Errorf("%w: deployment %s not found", ErrDeploymentNotFound, deploymentID)
	}

	dg, ok := b.deploymentGroups.Get(dgKey(d.ApplicationName, d.DeploymentGroupName))
	if !ok || dg.ComputePlatform == computePlatformECS {
		return []string{}, nil
	}

	ids := make([]string, 0)
	for _, t := range b.deploymentTargets(d) {
		if t.TargetType != targetTypeInstance {
			continue
		}

		if len(filter.StatusFilter) > 0 && !strs.ContainsFold(filter.StatusFilter, t.Status) {
			continue
		}

		if len(filter.TypeFilter) > 0 && !strs.ContainsFold(filter.TypeFilter, t.InstanceLabel) {
			continue
		}

		ids = append(ids, t.TargetID)
	}

	sort.Strings(ids)

	return ids, nil
}

// GetDeploymentInstance returns instance-summary information for a single
// instance participating in a deployment.
func (b *InMemoryBackend) GetDeploymentInstance(deploymentID, instanceID string) (*DeploymentTargetRecord, error) {
	b.mu.RLock("GetDeploymentInstance")
	defer b.mu.RUnlock()

	d, ok := b.deployments.Get(deploymentID)
	if !ok {
		return nil, fmt.Errorf("%w: deployment %s not found", ErrDeploymentNotFound, deploymentID)
	}

	for _, t := range b.deploymentTargets(d) {
		if t.TargetType == targetTypeInstance && t.TargetID == instanceID {
			cp := t

			return &cp, nil
		}
	}

	return nil, fmt.Errorf("%w: instance %s not found", ErrOnPremisesInstanceNotFound, instanceID)
}

// BatchGetDeploymentInstances returns instance-summary information for the
// given instance IDs. IDs that do not resolve to a real instance target are
// silently omitted, matching this codebase's Batch* "not found -> omitted"
// convention.
func (b *InMemoryBackend) BatchGetDeploymentInstances(
	deploymentID string, instanceIDs []string,
) ([]*DeploymentTargetRecord, error) {
	b.mu.RLock("BatchGetDeploymentInstances")
	defer b.mu.RUnlock()

	d, ok := b.deployments.Get(deploymentID)
	if !ok {
		return nil, fmt.Errorf("%w: deployment %s not found", ErrDeploymentNotFound, deploymentID)
	}

	byID := make(map[string]*DeploymentTargetRecord)
	for _, t := range b.deploymentTargets(d) {
		if t.TargetType != targetTypeInstance {
			continue
		}
		cp := t
		byID[t.TargetID] = &cp
	}

	result := make([]*DeploymentTargetRecord, 0, len(instanceIDs))

	for _, id := range instanceIDs {
		if t, found := byID[id]; found {
			result = append(result, t)
		}
	}

	return result, nil
}

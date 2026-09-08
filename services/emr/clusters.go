package emr

import (
	"context"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/awstime"
	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// effectiveStepStatus derives a step's live status, promoting a still-PENDING
// step to COMPLETED once stepCompletionDelay has elapsed since creation. AWS
// steps execute asynchronously against a real Hadoop job; gopherstack has no
// workload to run, so it simulates near-instant completion here rather than
// leaving every step parked in PENDING forever -- which would hang a real
// client's StepComplete waiter (DescribeStep/ListSteps are the only two read
// paths for step state, so both call this before returning). CANCELLED steps
// and steps already promoted are returned unchanged.
func effectiveStepStatus(s StepStatus) StepStatus {
	if s.State != StepStatePending {
		return s
	}

	created := time.Unix(0, int64(s.Timeline.CreationDateTime*float64(time.Second)))
	if time.Since(created) < stepCompletionDelay {
		return s
	}

	cp := s
	cp.State = StepStateCompleted
	cp.Timeline.StartDateTime = s.Timeline.CreationDateTime
	cp.Timeline.EndDateTime = awstime.Epoch(created.Add(stepCompletionDelay))

	return cp
}

// allStepsTerminal reports whether every step on a cluster has reached a
// terminal status (COMPLETED or CANCELLED), using effectiveStepStatus so a
// still-PENDING step within its completion delay counts as not terminal. A
// cluster with no steps at all is not "all steps terminal" -- there is
// nothing for KeepJobFlowAliveWhenNoSteps=false to have completed yet.
func allStepsTerminal(steps []Step) bool {
	if len(steps) == 0 {
		return false
	}

	for _, s := range steps {
		if effectiveStepStatus(s.Status).State == StepStatePending {
			return false
		}
	}

	return true
}

// clusterAcceptsSteps reports whether a cluster in the given state may
// accept new steps via AddJobFlowSteps, per real AddJobFlowSteps' doc
// ("You can only add steps to a cluster that is in one of the following
// states: STARTING, BOOTSTRAPPING, RUNNING, or WAITING").
func clusterAcceptsSteps(state string) bool {
	switch state {
	case StateStarting, StateBootstrapping, StateRunning, StateWaiting:
		return true
	default:
		return false
	}
}

// The following Get/Has/Put/Delete/InRegion helpers replace the old lazy
// per-region map accessors (clustersStore(region) etc.) with store.Table /
// store.Index operations. Callers must still hold b.mu, exactly as before --
// store.Table performs no locking of its own (see pkgs/store's package doc).

func (b *InMemoryBackend) clusterGet(region, id string) (*Cluster, bool) {
	return b.clusters.Get(regionKey(region, id))
}

func (b *InMemoryBackend) clusterPut(v *Cluster) { b.clusters.Put(v) }

func (b *InMemoryBackend) clusterDelete(region, id string) { b.clusters.Delete(regionKey(region, id)) }

func (b *InMemoryBackend) clustersInRegion(region string) []*Cluster {
	return b.clustersByRegion.Get(region)
}

// releaseLabelRe matches any emr-X.Y.Z label (e.g. emr-6.14.0, emr-7.3.0).
var releaseLabelRe = regexp.MustCompile(`^emr-\d+\.\d+(\.\d+)*$`)

// validateReleaseLabel returns an error if the label is not a valid EMR release label.
func validateReleaseLabel(label string) error {
	if _, ok := releaseLabelApps[label]; ok {
		return nil
	}

	if releaseLabelRe.MatchString(label) {
		return nil
	}

	return fmt.Errorf("%w: invalid ReleaseLabel %q", ErrValidation, label)
}

// cloneConfigurations deep-copies a Configuration slice.
func cloneConfigurations(cfgs []Configuration) []Configuration {
	if cfgs == nil {
		return nil
	}

	out := make([]Configuration, len(cfgs))
	for i, c := range cfgs {
		out[i] = cloneConfiguration(c)
	}

	return out
}

// cloneBootstrapActions deep-copies a slice of BootstrapActionConfig.
func cloneBootstrapActions(src []BootstrapActionConfig) []BootstrapActionConfig {
	if src == nil {
		return nil
	}

	out := make([]BootstrapActionConfig, len(src))
	for i, ba := range src {
		out[i] = BootstrapActionConfig{
			Name: ba.Name,
			ScriptBootstrapAction: BootstrapActionScript{
				Path: ba.ScriptBootstrapAction.Path,
				Args: slices.Clone(ba.ScriptBootstrapAction.Args),
			},
		}
	}

	return out
}

// cloneConfiguration deep-copies a single Configuration (recursive).
func cloneConfiguration(c Configuration) Configuration {
	cp := Configuration{
		Classification: c.Classification,
	}

	if c.Properties != nil {
		cp.Properties = maps.Clone(c.Properties)
	}

	if c.Configurations != nil {
		cp.Configurations = cloneConfigurations(c.Configurations)
	}

	return cp
}

// buildDefaultApplications returns the default application list for a release label.
func buildDefaultApplications(releaseLabel string) []Application {
	apps, ok := releaseLabelApps[releaseLabel]
	if !ok {
		return nil
	}

	result := make([]Application, 0, len(apps))
	for _, name := range apps {
		result = append(result, Application{Name: name})
	}

	return result
}

// buildEC2Attrs populates EC2InstanceAttributes from the RunJobFlow instances
// block. jobFlowRole is the top-level RunJobFlowInput.JobFlowRole field --
// real EMR echoes it back as Ec2InstanceAttributes.IamInstanceProfile (there
// is no IamInstanceProfile member on the Instances block itself).
func buildEC2Attrs(inst RunJobFlowInstances, jobFlowRole string) *EC2InstanceAttributes {
	return &EC2InstanceAttributes{
		Ec2KeyName:                     inst.Ec2KeyName,
		Ec2SubnetID:                    inst.Ec2SubnetID,
		EmrManagedMasterSecurityGroup:  inst.EmrManagedMasterSecurityGroup,
		EmrManagedSlaveSecurityGroup:   inst.EmrManagedSlaveSecurityGroup,
		ServiceAccessSecurityGroup:     inst.ServiceAccessSecurityGroup,
		IamInstanceProfile:             jobFlowRole,
		AdditionalMasterSecurityGroups: inst.AdditionalMasterSecurityGroups,
		AdditionalSlaveSecurityGroups:  inst.AdditionalSlaveSecurityGroups,
		RequestedEc2SubnetIDs:          inst.Ec2SubnetIDs,
	}
}

// instanceCollectionType returns the real INSTANCE_GROUP/INSTANCE_FLEET
// discriminator for a RunJobFlow request, based on which of
// Instances.InstanceGroups/InstanceFleets was populated (the two are
// mutually exclusive on the real API).
func instanceCollectionType(hasFleets bool) string {
	if hasFleets {
		return "INSTANCE_FLEET"
	}

	return "INSTANCE_GROUP"
}

// validateRunJobFlowParams checks RunJobFlow's inline-validatable input
// (release label plus the optional inline ManagedScalingPolicy/
// AutoTerminationPolicy, which reuse the same validation their standalone
// Put* operations use) and returns the normalized release label. Factored
// out of RunJobFlow to keep its own cognitive complexity/length down.
func validateRunJobFlowParams(params RunJobFlowParams) (string, error) {
	releaseLabel := params.ReleaseLabel
	if releaseLabel == "" {
		releaseLabel = defaultReleaseLabel
	}

	if err := validateReleaseLabel(releaseLabel); err != nil {
		return "", err
	}

	if params.ManagedScalingPolicy != nil {
		if err := validateManagedScalingPolicy(*params.ManagedScalingPolicy); err != nil {
			return "", err
		}
	}

	if params.AutoTerminationPolicy != nil {
		if err := validateAutoTerminationPolicy(*params.AutoTerminationPolicy); err != nil {
			return "", err
		}
	}

	return releaseLabel, nil
}

// buildNewCluster constructs the Cluster record for a RunJobFlow call.
// Factored out of RunJobFlow to keep its own length/complexity down. Caller
// must hold b.mu.Lock and have already validated params via
// validateRunJobFlowParams.
func (b *InMemoryBackend) buildNewCluster(region, id, releaseLabel string, params RunJobFlowParams) *Cluster {
	clusterARN := arn.Build("elasticmapreduce", region, b.accountID, "cluster/"+id)

	tagsCopy := make([]Tag, len(params.Tags))
	copy(tagsCopy, params.Tags)

	apps := params.Applications
	if len(apps) == 0 {
		apps = buildDefaultApplications(releaseLabel)
	}

	stepConcurrency := params.StepConcurrencyLevel
	if stepConcurrency == 0 {
		stepConcurrency = defaultStepConcurrency
	}

	// Real JobFlowInstancesConfig accepts either InstanceGroups or
	// InstanceFleets (mutually exclusive); InstanceFleets takes precedence
	// here when both are somehow set.
	hasFleets := len(params.Instances.InstanceFleets) > 0

	var groups []InstanceGroup

	var fleets []InstanceFleet

	if hasFleets {
		fleets = b.buildInstanceFleets(params.Instances.InstanceFleets)
	} else {
		groups = b.buildInstanceGroups(params.Instances.InstanceGroups)
	}

	steps := b.buildInitialSteps(params.Steps, params.StepExecutionRoleArn)

	// Clusters are created directly in WAITING state (no simulated
	// STARTING/BOOTSTRAPPING/RUNNING transition), so the cluster is
	// immediately ready to run steps -- ReadyDateTime equals CreationDateTime.
	nowEpoch := awstime.Epoch(time.Now())

	return &Cluster{
		ID:                      id,
		Name:                    params.Name,
		ReleaseLabel:            releaseLabel,
		OSReleaseLabel:          params.OSReleaseLabel,
		ARN:                     clusterARN,
		Ec2InstanceAttributes:   buildEC2Attrs(params.Instances, params.JobFlowRole),
		KerberosAttributes:      clonePtr(params.KerberosAttributes),
		MonitoringConfiguration: clonePtr(params.MonitoringConfiguration),
		Status: ClusterStatus{
			State:             StateWaiting,
			StateChangeReason: map[string]any{"Code": "USER_REQUEST", "Message": ""},
			Timeline: map[string]any{
				timelineKeyCreation: nowEpoch,
				timelineKeyReady:    nowEpoch,
			},
		},
		Tags:                        tagsCopy,
		Applications:                apps,
		Configurations:              cloneConfigurations(params.Configurations),
		PlacementGroups:             slices.Clone(params.PlacementGroupConfigs),
		LogURI:                      params.LogURI,
		LogEncryptionKmsKeyID:       params.LogEncryptionKmsKeyID,
		RepoUpgradeOnBoot:           params.RepoUpgradeOnBoot,
		RequestedAmiVersion:         params.AmiVersion,
		RunningAmiVersion:           params.AmiVersion,
		ServiceRole:                 params.ServiceRole,
		AutoScalingRole:             params.AutoScalingRole,
		ScaleDownBehavior:           params.ScaleDownBehavior,
		SecurityConfiguration:       params.SecurityConfiguration,
		CustomAmiID:                 params.CustomAmiID,
		InstanceCollectionType:      instanceCollectionType(hasFleets),
		StepConcurrencyLevel:        stepConcurrency,
		EbsRootVolumeSize:           params.EbsRootVolumeSize,
		EbsRootVolumeIops:           params.EbsRootVolumeIops,
		EbsRootVolumeThroughput:     params.EbsRootVolumeThroughput,
		VisibleToAllUsers:           params.VisibleToAllUsers,
		SessionEnabled:              params.SessionEnabled,
		TerminationProtected:        params.Instances.TerminationProtected,
		KeepJobFlowAliveWhenNoSteps: params.Instances.KeepJobFlowAliveWhenNoSteps,
		AutoTerminate:               !params.Instances.KeepJobFlowAliveWhenNoSteps,
		instanceGroups:              groups,
		instanceFleets:              fleets,
		steps:                       steps,
		bootstrapActions:            cloneBootstrapActions(params.BootstrapActions),
		managedScalingPolicy:        clonePtr(params.ManagedScalingPolicy),
		autoTerminationPolicy:       clonePtr(params.AutoTerminationPolicy),
		region:                      region,
	}
}

// RunJobFlow creates a new EMR cluster.
func (b *InMemoryBackend) RunJobFlow(ctx context.Context, params RunJobFlowParams) (*Cluster, error) {
	releaseLabel, err := validateRunJobFlowParams(params)
	if err != nil {
		return nil, err
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("RunJobFlow")
	defer b.mu.Unlock()

	if params.SecurityConfiguration != "" {
		if _, ok := b.securityConfigGet(region, params.SecurityConfiguration); !ok {
			return nil, fmt.Errorf(
				"%w: security configuration %s not found", ErrNotFound, params.SecurityConfiguration,
			)
		}
	}

	id := b.nextID()
	cluster := b.buildNewCluster(region, id, releaseLabel, params)

	b.clusterPut(cluster)
	b.arnIndexStore(region)[cluster.ARN] = id
	cp := cluster.clone()

	return &cp, nil
}

// DescribeCluster returns a cluster by its ID.
func (b *InMemoryBackend) DescribeCluster(ctx context.Context, id string) (*Cluster, error) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("DescribeCluster")
	defer b.mu.RUnlock()

	cluster, ok := b.clusterGet(region, id)
	if !ok {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrNotFound, id)
	}

	cp := cluster.clone()

	return &cp, nil
}

// clone returns a deep copy of the Cluster.
func (c Cluster) clone() Cluster {
	cp := c

	if c.Tags != nil {
		cp.Tags = make([]Tag, len(c.Tags))
		copy(cp.Tags, c.Tags)
	}

	if c.Applications != nil {
		cp.Applications = make([]Application, len(c.Applications))
		copy(cp.Applications, c.Applications)
	}

	cp.Configurations = cloneConfigurations(c.Configurations)
	cp.PlacementGroups = slices.Clone(c.PlacementGroups)
	cp.KerberosAttributes = clonePtr(c.KerberosAttributes)

	if c.instanceGroups != nil {
		cp.instanceGroups = make([]InstanceGroup, len(c.instanceGroups))
		copy(cp.instanceGroups, c.instanceGroups)
	}

	if c.instanceFleets != nil {
		cp.instanceFleets = make([]InstanceFleet, len(c.instanceFleets))
		copy(cp.instanceFleets, c.instanceFleets)
	}

	if c.steps != nil {
		cp.steps = make([]Step, len(c.steps))
		copy(cp.steps, c.steps)
	}

	if c.sessions != nil {
		cp.sessions = make([]Session, len(c.sessions))
		for i, s := range c.sessions {
			cp.sessions[i] = s.clone()
		}
	}

	cp.bootstrapActions = cloneBootstrapActions(c.bootstrapActions)

	if c.managedScalingPolicy != nil {
		msp := *c.managedScalingPolicy
		cp.managedScalingPolicy = &msp
	}

	if c.autoTerminationPolicy != nil {
		atp := *c.autoTerminationPolicy
		cp.autoTerminationPolicy = &atp
	}

	cp.Status.StateChangeReason = maps.Clone(c.Status.StateChangeReason)
	cp.Status.Timeline = maps.Clone(c.Status.Timeline)

	return cp
}

// ListClusters returns cluster summaries matching the given filter, sorted by creation time descending.
func (b *InMemoryBackend) ListClusters(ctx context.Context, params ListClustersParams) ([]ClusterSummary, string) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("ListClusters")
	defer b.mu.RUnlock()

	stateSet := buildStateSet(params.ClusterStates)
	list := b.gatherClusterSummaries(region, stateSet, params)

	sort.Slice(list, func(i, j int) bool {
		ti := clusterCreationSeconds(list[i])
		tj := clusterCreationSeconds(list[j])
		if ti != tj {
			return ti > tj
		}

		return list[i].ID > list[j].ID
	})

	p := page.New(list, params.Marker, listClustersPageSize, listClustersPageSize)

	return p.Data, p.Next
}

// buildStateSet converts a slice of state strings to a set.
// An empty slice means "all non-terminal states".
func buildStateSet(states []string) map[string]bool {
	if len(states) == 0 {
		return nil
	}

	set := make(map[string]bool, len(states))
	for _, s := range states {
		set[s] = true
	}

	return set
}

// gatherClusterSummaries collects filtered cluster summaries. Caller holds read lock.
func (b *InMemoryBackend) gatherClusterSummaries(
	region string,
	stateSet map[string]bool,
	params ListClustersParams,
) []ClusterSummary {
	clusters := b.clustersInRegion(region)
	list := make([]ClusterSummary, 0, len(clusters))

	for _, c := range clusters {
		if !clusterMatchesFilter(c, stateSet, params) {
			continue
		}

		status := ClusterStatus{
			State:             c.Status.State,
			StateChangeReason: c.Status.StateChangeReason,
			// Timeline must be carried through: ListClusters' real response
			// includes Status.Timeline per cluster, and the sort below relies
			// on reading CreationDateTime back out of this same field.
			Timeline: c.Status.Timeline,
		}
		list = append(list, ClusterSummary{
			ID:         c.ID,
			Name:       c.Name,
			Status:     status,
			ClusterArn: c.ARN,
		})
	}

	return list
}

// clusterMatchesFilter reports whether c satisfies the given filter.
func clusterMatchesFilter(c *Cluster, stateSet map[string]bool, params ListClustersParams) bool {
	if stateSet != nil {
		if !stateSet[c.Status.State] {
			return false
		}
	} else {
		if c.Status.State == StateTerminated || c.Status.State == StateTerminatedWithErrors {
			return false
		}
	}

	creationSeconds := clusterCreationSecondsFromCluster(c)
	if params.CreatedAfter != nil {
		if creationSeconds < awstime.Epoch(*params.CreatedAfter) {
			return false
		}
	}

	if params.CreatedBefore != nil {
		if creationSeconds > awstime.Epoch(*params.CreatedBefore) {
			return false
		}
	}

	return true
}

func clusterCreationSeconds(cs ClusterSummary) float64 {
	return timelineSeconds(cs.Status.Timeline, timelineKeyCreation)
}

func clusterCreationSecondsFromCluster(c *Cluster) float64 {
	return timelineSeconds(c.Status.Timeline, timelineKeyCreation)
}

// timelineSeconds reads an epoch-seconds value out of a Timeline map. Values
// written by this backend are always float64 (see awstime.Epoch), but int64
// is also accepted defensively (e.g. a hand-built map from a caller/test).
func timelineSeconds(timeline map[string]any, key string) float64 {
	switch v := timeline[key].(type) {
	case float64:
		return v
	case int64:
		return float64(v)
	default:
		return 0
	}
}

// TerminateJobFlows marks the specified clusters as TERMINATED.
// Returns ValidationException if any cluster has termination protection.
func (b *InMemoryBackend) TerminateJobFlows(ctx context.Context, ids []string) error {
	region := getRegion(ctx, b.region)

	b.mu.Lock("TerminateJobFlows")
	defer b.mu.Unlock()

	for _, id := range ids {
		cluster, ok := b.clusterGet(region, id)
		if !ok {
			return fmt.Errorf("%w: cluster %s not found", ErrNotFound, id)
		}

		if err := terminateSingle(cluster, id, "USER_REQUEST", "Terminated by user request"); err != nil {
			return err
		}
	}

	return nil
}

// terminateSingle transitions cluster straight to TERMINATED (this backend
// never simulates TERMINATING). reasonCode/reasonMessage populate
// Status.StateChangeReason -- callers pass real
// types.ClusterStateChangeReasonCode values (emr@v1.64.4 types/enums.go),
// e.g. "USER_REQUEST" for an explicit TerminateJobFlows call or
// "ALL_STEPS_COMPLETED" for the janitor's auto-termination sweep.
func terminateSingle(cluster *Cluster, id, reasonCode, reasonMessage string) error {
	if cluster.Status.State == StateTerminated ||
		cluster.Status.State == StateTerminatedWithErrors {
		return nil
	}

	if cluster.TerminationProtected {
		return fmt.Errorf("%w: cluster %s", errTerminationProtected, id)
	}

	now := time.Now()
	cluster.Status.State = StateTerminated
	cluster.Status.StateChangeReason = map[string]any{
		"Code":    reasonCode,
		"Message": reasonMessage,
	}
	cluster.Status.Timeline[timelineKeyEnd] = awstime.Epoch(now)
	cluster.terminatedAt = now

	// A Spark Connect session cannot outlive the cluster it runs on -- see
	// terminateClusterSessions (sessions.go) for the full cascade rationale.
	terminateClusterSessions(cluster, now)

	return nil
}

// findClusterByIDOrARN looks up a cluster by either its ID or ARN within the
// given region. Caller must hold at least a read lock.
func (b *InMemoryBackend) findClusterByIDOrARN(region, idOrARN string) *Cluster {
	if c, ok := b.clusterGet(region, idOrARN); ok {
		return c
	}

	if id, ok := b.arnIndexStoreRO(region)[idOrARN]; ok {
		if c, found := b.clusterGet(region, id); found {
			return c
		}
	}

	return nil
}

// AddClusterInternal seeds a cluster directly into the backend for testing.
func (b *InMemoryBackend) AddClusterInternal(ctx context.Context, cluster *Cluster) {
	region := getRegion(ctx, b.region)

	b.mu.Lock("AddClusterInternal")
	defer b.mu.Unlock()

	cp := cluster.clone()
	cp.region = region
	b.clusterPut(&cp)
	b.arnIndexStore(region)[cluster.ARN] = cluster.ID
}

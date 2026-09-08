package mgn

import (
	"slices"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// Real AWS creates a SourceServer only via the MGN Replication Agent's internal,
// non-public registration call. The only PUBLIC creation path is StartImport's
// bulk metadata load (exportimport.go): s3import.go reads the caller-supplied S3
// object and parses it as a documented, best-effort CSV schema (AWS does not
// publish the real one), creating one SourceServer per valid row via
// createSourceServerLocked. Every server starts NOT_READY/INITIATING and
// progresses to READY_FOR_TEST/CONTINUOUS over 3 asyncTransitionDelay ticks
// (scheduleReplicationLocked).
//
// LifeCycleState transitions (documented, SDK-inferred, not independently
// confirmed against AWS's unpublished state machine):
//
//	PENDING_INSTALLATION/DISCOVERED are skipped -- seeded directly into NOT_READY.
//	NOT_READY -> READY_FOR_TEST   once DataReplicationState reaches CONTINUOUS.
//	READY_FOR_TEST -> TESTING     via StartTest.
//	TESTING -> READY_FOR_CUTOVER  once the Job completes.
//	READY_FOR_CUTOVER -> CUTTING_OVER  via StartCutover.
//	CUTTING_OVER -> CUTOVER       via FinalizeCutover (a distinct call, not automatic).
//	CUTOVER -> DISCONNECTED       via DisconnectFromService, the terminal state.
//
// ChangeServerLifeCycleState can force State directly to
// READY_FOR_TEST/READY_FOR_CUTOVER/CUTOVER, bypassing the table above. Real AWS
// documents a precondition, not a manual-override purpose: "This command only
// works if the Source Server is already launchable (dataReplicationInfo.
// lagDuration is not null)" (mgn api_op_ChangeServerLifeCycleState.go:12-15).
// gopherstack never fabricates LagDuration (models.go's DataReplicationInfo doc
// comment), so DataReplicationState == CONTINUOUS is the equivalent launchable
// signal used below; an earlier call is rejected with ConflictException.

// resolveSourceServerLocked resolves sourceServerID to its stored
// SourceServer. Callers must hold b.mu.
func (b *InMemoryBackend) resolveSourceServerLocked(sourceServerID string) (*SourceServer, bool) {
	return b.sourceServers.Get(sourceServerID)
}

// resolveSourceServerByUserProvidedIDLocked finds the SourceServer whose
// UserProvidedID matches, or false if none does. Backs StartImport's
// re-import dedup: real AWS documents "mgn:server:user-provided-id" as "used
// by MGN to consistently recognize the server replication, and avoid
// duplication when importing inventory from a CSV file" (MGN User Guide,
// Import parameters) -- the natural key this backend uses to decide whether
// a row updates an existing SourceServer (ModifiedCount) or creates a new
// one (CreatedCount). Callers must hold b.mu.
func (b *InMemoryBackend) resolveSourceServerByUserProvidedIDLocked(userProvidedID string) (*SourceServer, bool) {
	if userProvidedID == "" {
		return nil, false
	}

	for _, s := range b.sourceServers.Snapshot() {
		if s.UserProvidedID == userProvidedID {
			return s, true
		}
	}

	return nil, false
}

// applyImportRowLocked overwrites an existing SourceServer's
// SourceProperties/FqdnForActionFramework/tags with a re-imported row's
// values -- the "update" half of StartImport's dedup-by-UserProvidedID
// convention (see resolveSourceServerByUserProvidedIDLocked). Callers must
// hold b.mu.
func (b *InMemoryBackend) applyImportRowLocked(s *SourceServer, seed sourceServerSeed) {
	s.SourceProperties = seed.SourceProperties

	if seed.FqdnForActionFramework != "" {
		s.FqdnForActionFramework = seed.FqdnForActionFramework
	}

	if s.SourceProperties != nil {
		s.SourceProperties.LastUpdatedDateTime = nowRFC3339()
	}

	if s.Tags != nil {
		s.Tags.Merge(seed.ImportTags)
	}
}

// sourceServerSeed configures createSourceServerLocked -- the single
// creation path behind every SourceServer this backend ever stores (see
// this file's package doc comment). Every field is optional; zero values
// fall back to a documented, invented default (see each field's own
// comment) rather than an AWS-confirmed one, since no real source machine
// exists to derive defaults from.
type sourceServerSeed struct {
	SourceProperties       *SourceProperties
	ImportTags             map[string]string
	SourceServerID         string
	UserProvidedID         string
	FqdnForActionFramework string
	ReplicationType        string
	DiskDeviceName         string
	TotalStorageBytes      int64
}

// createSourceServerLocked creates a new SourceServer directly in this
// backend -- called only by StartImport's real CSV-parsed rows
// (s3import.go). The new server starts in LifeCycleState NOT_READY /
// DataReplicationState INITIATING and progresses on a deterministic timer
// toward READY_FOR_TEST / CONTINUOUS (see scheduleReplicationLocked).
// Callers must hold b.mu.
func (b *InMemoryBackend) createSourceServerLocked(seed sourceServerSeed) *SourceServer {
	id := seed.SourceServerID
	if id == "" {
		id = newSourceServerID()
	}

	replicationType := seed.ReplicationType
	if replicationType == "" {
		replicationType = ReplicationTypeAgentBased
	}

	totalBytes := seed.TotalStorageBytes
	if totalBytes <= 0 {
		totalBytes = defaultTotalStorageBytes
	}

	deviceName := seed.DiskDeviceName
	if deviceName == "" {
		deviceName = "/dev/sda1"
	}

	now := nowRFC3339()
	t := tags.New("mgn.sourceserver." + id + ".tags")
	t.Merge(seed.ImportTags)

	s := &SourceServer{
		SourceServerID:         id,
		Arn:                    b.sourceServerARN(id),
		UserProvidedID:         seed.UserProvidedID,
		FqdnForActionFramework: seed.FqdnForActionFramework,
		ReplicationType:        replicationType,
		SourceProperties:       seed.SourceProperties,
		Tags:                   t,
		LifeCycle: &LifeCycle{
			State:                     LifeCycleStateNotReady,
			AddedToServiceDateTime:    now,
			LastSeenByServiceDateTime: now,
		},
		DataReplicationInfo: &DataReplicationInfo{
			DataReplicationState: DataReplicationStateInitiating,
			DataReplicationInitiation: &DataReplicationInitiation{
				StartDateTime: now,
				Steps:         seedInitiationSteps(),
			},
			ReplicatedDisks: []DataReplicationInfoReplicatedDisk{
				{DeviceName: deviceName, TotalStorageBytes: totalBytes},
			},
		},
	}

	b.sourceServers.Put(s)

	b.launchConfigs.Put(&LaunchConfiguration{
		SourceServerID:    id,
		Name:              "Dr " + id,
		BootMode:          BootModeUseSource,
		LaunchDisposition: LaunchDispositionStarted,
	})
	b.replicationConfigs.Put(&ReplicationConfiguration{
		SourceServerID:                id,
		Name:                          "Replication Configuration " + id,
		DataPlaneRouting:              DataPlaneRoutingPrivateIP,
		DefaultLargeStagingDiskType:   StagingDiskTypeGp3,
		EbsEncryption:                 EbsEncryptionDefault,
		AssociateDefaultSecurityGroup: true,
		ReplicationServerInstanceType: "t3.small",
	})

	b.scheduleReplicationLocked(id)

	return s
}

// seedInitiationSteps returns the fixed 12-step DataReplicationInitiation
// walk, all NOT_STARTED except the first (WAIT), which begins IN_PROGRESS.
func seedInitiationSteps() []DataReplicationInitiationStep {
	names := dataReplicationInitiationStepNames()
	steps := make([]DataReplicationInitiationStep, len(names))

	for i, name := range names {
		status := StepStatusNotStarted
		if i == 0 {
			status = StepStatusInProgress
		}

		steps[i] = DataReplicationInitiationStep{Name: name, Status: status}
	}

	return steps
}

// scheduleReplicationLocked walks a newly seeded/restarted SourceServer's
// DataReplicationInfo through INITIATING -> INITIAL_SYNC -> BACKLOG -> CONTINUOUS
// over 3 asyncTransitionDelay ticks, monotonically increasing
// ReplicatedStorageBytes toward TotalStorageBytes -- deterministic, time-based
// progression per PARITY.md, never a fabricated bandwidth/lag figure. All 12
// DataReplicationInitiationStep entries are marked SUCCEEDED together at the
// first tick rather than one real timer-tick per step (PARITY.md sanctions this
// as a defensible simpler pass). Every tick re-checks the server still exists and
// is still in the state this scheduler put it in, so a later
// Stop/Pause/DisconnectFromService halts progression without cancelling the timer.
func (b *InMemoryBackend) scheduleReplicationLocked(sourceServerID string) {
	b.work.After("ReplicationInitiated", asyncTransitionDelay, func() {
		b.tickReplicationInitiatedLocked(sourceServerID)

		b.work.After("ReplicationBacklog", asyncTransitionDelay, func() {
			b.tickReplicationBacklogLocked(sourceServerID)

			b.work.After("ReplicationContinuous", asyncTransitionDelay, func() {
				b.tickReplicationContinuousLocked(sourceServerID)
			})
		})
	})
}

// tickReplicationInitiatedLocked marks every DataReplicationInitiation step
// SUCCEEDED and moves DataReplicationState INITIATING -> INITIAL_SYNC.
func (b *InMemoryBackend) tickReplicationInitiatedLocked(sourceServerID string) {
	b.mu.Lock("ReplicationInitiated-async")
	defer b.mu.Unlock()

	s, ok := b.sourceServers.Get(sourceServerID)
	if !ok || s.DataReplicationInfo == nil ||
		s.DataReplicationInfo.DataReplicationState != DataReplicationStateInitiating {
		return
	}

	for i := range s.DataReplicationInfo.DataReplicationInitiation.Steps {
		s.DataReplicationInfo.DataReplicationInitiation.Steps[i].Status = StepStatusSucceeded
	}

	s.DataReplicationInfo.DataReplicationState = DataReplicationStateInitialSync
}

// tickReplicationBacklogLocked moves DataReplicationState INITIAL_SYNC ->
// BACKLOG, bringing each replicated disk halfway to TotalStorageBytes.
func (b *InMemoryBackend) tickReplicationBacklogLocked(sourceServerID string) {
	const halfway = 2

	b.mu.Lock("ReplicationBacklog-async")
	defer b.mu.Unlock()

	s, ok := b.sourceServers.Get(sourceServerID)
	if !ok || s.DataReplicationInfo == nil ||
		s.DataReplicationInfo.DataReplicationState != DataReplicationStateInitialSync {
		return
	}

	s.DataReplicationInfo.DataReplicationState = DataReplicationStateBacklog

	for i := range s.DataReplicationInfo.ReplicatedDisks {
		d := &s.DataReplicationInfo.ReplicatedDisks[i]
		d.ReplicatedStorageBytes = d.TotalStorageBytes / halfway
		d.BackloggedStorageBytes = d.TotalStorageBytes - d.ReplicatedStorageBytes
	}
}

// tickReplicationContinuousLocked moves DataReplicationState BACKLOG ->
// CONTINUOUS, bringing each replicated disk fully caught up, and flips
// LifeCycleState NOT_READY -> READY_FOR_TEST.
func (b *InMemoryBackend) tickReplicationContinuousLocked(sourceServerID string) {
	b.mu.Lock("ReplicationContinuous-async")
	defer b.mu.Unlock()

	s, ok := b.sourceServers.Get(sourceServerID)
	if !ok || s.DataReplicationInfo == nil ||
		s.DataReplicationInfo.DataReplicationState != DataReplicationStateBacklog {
		return
	}

	s.DataReplicationInfo.DataReplicationState = DataReplicationStateContinuous
	s.DataReplicationInfo.LastSnapshotDateTime = nowRFC3339()

	for i := range s.DataReplicationInfo.ReplicatedDisks {
		d := &s.DataReplicationInfo.ReplicatedDisks[i]
		d.ReplicatedStorageBytes = d.TotalStorageBytes
		d.BackloggedStorageBytes = 0
	}

	if s.LifeCycle != nil && s.LifeCycle.State == LifeCycleStateNotReady {
		s.LifeCycle.State = LifeCycleStateReadyForTest
	}
}

// DescribeSourceServersFilters mirrors types.DescribeSourceServersRequestFilters.
type DescribeSourceServersFilters struct {
	ApplicationIDs   []string
	IsArchived       *bool
	LifeCycleStates  []string
	ReplicationTypes []string
	SourceServerIDs  []string
}

func matchesSourceServerFilter(s *SourceServer, f DescribeSourceServersFilters) bool {
	if len(f.SourceServerIDs) > 0 && !containsStr(f.SourceServerIDs, s.SourceServerID) {
		return false
	}

	if len(f.ApplicationIDs) > 0 && !containsStr(f.ApplicationIDs, s.ApplicationID) {
		return false
	}

	if f.IsArchived != nil && s.IsArchived != *f.IsArchived {
		return false
	}

	if len(f.ReplicationTypes) > 0 && !containsStr(f.ReplicationTypes, s.ReplicationType) {
		return false
	}

	if len(f.LifeCycleStates) > 0 {
		state := ""
		if s.LifeCycle != nil {
			state = s.LifeCycle.State
		}

		if !containsStr(f.LifeCycleStates, state) {
			return false
		}
	}

	return true
}

// DescribeSourceServers returns a page of SourceServers matching f.
func (b *InMemoryBackend) DescribeSourceServers(
	f DescribeSourceServersFilters,
	token string,
	limit int,
) (page.Page[*SourceServer], error) {
	b.mu.RLock("DescribeSourceServers")
	defer b.mu.RUnlock()

	if err := b.requireInitializedLocked(); err != nil {
		return page.Page[*SourceServer]{}, err
	}

	all := b.sourceServers.Snapshot()
	filtered := make([]*SourceServer, 0, len(all))

	for _, s := range all {
		if matchesSourceServerFilter(s, f) {
			filtered = append(filtered, s.clone())
		}
	}

	return page.New(filtered, token, limit, defaultPageLimit), nil
}

// SourceServerUpdate configures UpdateSourceServer -- every field is a
// pointer so a field absent from the caller's JSON body (nil) leaves the
// corresponding SourceServer field unchanged, matching real AWS's
// partial-update semantics for this op (never wiping ConnectorAction just
// because a caller only meant to set FqdnForActionFramework). Platform has
// no field to land in deliberately: the real SDK's own SourceServer/
// SourceProperties output shape has no Platform field to read it back from
// either (confirmed by direct SDK read, same as s3import.go's identical
// finding for mgn:server:platform) -- accepted and silently dropped, not a
// bug.
type SourceServerUpdate struct {
	ConnectorAction        *SourceServerConnectorAction
	FqdnForActionFramework *string
	UserProvidedID         *string
}

// UpdateSourceServer applies update to sourceServerID and returns the
// flattened SourceServer (PARITY.md wire-trap #1).
func (b *InMemoryBackend) UpdateSourceServer(sourceServerID string, update SourceServerUpdate) (*SourceServer, error) {
	b.mu.Lock("UpdateSourceServer")
	defer b.mu.Unlock()

	if err := b.requireInitializedLocked(); err != nil {
		return nil, err
	}

	s, ok := b.resolveSourceServerLocked(sourceServerID)
	if !ok {
		return nil, notFoundError(resourceSourceServer, sourceServerID)
	}

	if update.ConnectorAction != nil {
		s.ConnectorAction = update.ConnectorAction
	}

	if update.FqdnForActionFramework != nil {
		s.FqdnForActionFramework = *update.FqdnForActionFramework
	}

	if update.UserProvidedID != nil {
		s.UserProvidedID = *update.UserProvidedID
	}

	return s.clone(), nil
}

// UpdateSourceServerReplicationType changes sourceServerID's ReplicationType.
func (b *InMemoryBackend) UpdateSourceServerReplicationType(
	sourceServerID, replicationType string,
) (*SourceServer, error) {
	b.mu.Lock("UpdateSourceServerReplicationType")
	defer b.mu.Unlock()

	if err := b.requireInitializedLocked(); err != nil {
		return nil, err
	}

	if replicationType != ReplicationTypeAgentBased && replicationType != ReplicationTypeSnapshotShipping {
		return nil, validationError("replicationType must be AGENT_BASED or SNAPSHOT_SHIPPING")
	}

	s, ok := b.resolveSourceServerLocked(sourceServerID)
	if !ok {
		return nil, notFoundError(resourceSourceServer, sourceServerID)
	}

	s.ReplicationType = replicationType

	return s.clone(), nil
}

// DeleteSourceServer deletes sourceServerID and its per-server launch/
// replication configuration and post-launch actions.
func (b *InMemoryBackend) DeleteSourceServer(sourceServerID string) error {
	b.mu.Lock("DeleteSourceServer")
	defer b.mu.Unlock()

	if err := b.requireInitializedLocked(); err != nil {
		return err
	}

	s, ok := b.resolveSourceServerLocked(sourceServerID)
	if !ok {
		return notFoundError(resourceSourceServer, sourceServerID)
	}

	if s.Tags != nil {
		s.Tags.Close()
	}

	b.sourceServers.Delete(sourceServerID)
	b.launchConfigs.Delete(sourceServerID)
	b.replicationConfigs.Delete(sourceServerID)

	for _, a := range b.sourceServerActionsByServer.Get(sourceServerID) {
		b.sourceServerActions.Delete(actionKey(a.SourceServerID, a.ActionID))
	}

	return nil
}

// ChangeServerLifeCycleState forces sourceServerID's LifeCycleState to one
// of the 3 caller-settable targets (see this file's doc comment).
func (b *InMemoryBackend) ChangeServerLifeCycleState(sourceServerID, targetState string) (*SourceServer, error) {
	b.mu.Lock("ChangeServerLifeCycleState")
	defer b.mu.Unlock()

	if err := b.requireInitializedLocked(); err != nil {
		return nil, err
	}

	switch targetState {
	case ChangeLifecycleReadyForTest, ChangeLifecycleReadyForCutover, ChangeLifecycleCutover:
	default:
		return nil, validationError("lifeCycle.state must be READY_FOR_TEST, READY_FOR_CUTOVER, or CUTOVER")
	}

	s, ok := b.resolveSourceServerLocked(sourceServerID)
	if !ok {
		return nil, notFoundError(resourceSourceServer, sourceServerID)
	}

	if s.DataReplicationInfo == nil || s.DataReplicationInfo.DataReplicationState != DataReplicationStateContinuous {
		return nil, conflictErrorWithResource(
			resourceSourceServer, sourceServerID,
			"source server is not yet launchable: "+sourceServerID,
		)
	}

	if s.LifeCycle == nil {
		s.LifeCycle = &LifeCycle{}
	}

	s.LifeCycle.State = targetState

	return s.clone(), nil
}

// DisconnectFromService moves sourceServerID to its terminal DISCONNECTED
// state (both LifeCycleState and DataReplicationState).
func (b *InMemoryBackend) DisconnectFromService(sourceServerID string) (*SourceServer, error) {
	b.mu.Lock("DisconnectFromService")
	defer b.mu.Unlock()

	if err := b.requireInitializedLocked(); err != nil {
		return nil, err
	}

	s, ok := b.resolveSourceServerLocked(sourceServerID)
	if !ok {
		return nil, notFoundError(resourceSourceServer, sourceServerID)
	}

	if s.LifeCycle == nil {
		s.LifeCycle = &LifeCycle{}
	}

	s.LifeCycle.State = LifeCycleStateDisconnected

	if s.DataReplicationInfo != nil {
		s.DataReplicationInfo.DataReplicationState = DataReplicationStateDisconnected
	}

	return s.clone(), nil
}

// FinalizeCutover completes a cutover already started by StartCutover --
// see this file's doc comment: CUTTING_OVER -> CUTOVER is the one
// LifeCycleState transition that happens ONLY here, not automatically on
// Job completion.
func (b *InMemoryBackend) FinalizeCutover(sourceServerID string) (*SourceServer, error) {
	b.mu.Lock("FinalizeCutover")
	defer b.mu.Unlock()

	if err := b.requireInitializedLocked(); err != nil {
		return nil, err
	}

	s, ok := b.resolveSourceServerLocked(sourceServerID)
	if !ok {
		return nil, notFoundError(resourceSourceServer, sourceServerID)
	}

	if s.LifeCycle == nil || s.LifeCycle.State != LifeCycleStateCuttingOver {
		return nil, conflictErrorWithResource(
			resourceSourceServer, sourceServerID,
			"source server is not in a cutting-over state: "+sourceServerID,
		)
	}

	s.LifeCycle.State = LifeCycleStateCutover

	if s.LifeCycle.LastCutover == nil {
		s.LifeCycle.LastCutover = &LifeCycleLastCutover{}
	}

	s.LifeCycle.LastCutover.Finalized = &timestamped{APICallDateTime: nowRFC3339()}

	return s.clone(), nil
}

// MarkAsArchived sets sourceServerID's IsArchived flag. Real AWS only allows
// this for a SourceServer whose LifeCycleState is DISCONNECTED or CUTOVER
// (api_op_MarkAsArchived.go:13-14: "This command only works for SourceServers
// with a lifecycle. state which equals DISCONNECTED or CUTOVER.").
func (b *InMemoryBackend) MarkAsArchived(sourceServerID string) (*SourceServer, error) {
	b.mu.Lock("MarkAsArchived")
	defer b.mu.Unlock()

	if err := b.requireInitializedLocked(); err != nil {
		return nil, err
	}

	s, ok := b.resolveSourceServerLocked(sourceServerID)
	if !ok {
		return nil, notFoundError(resourceSourceServer, sourceServerID)
	}

	state := ""
	if s.LifeCycle != nil {
		state = s.LifeCycle.State
	}

	if state != LifeCycleStateDisconnected && state != LifeCycleStateCutover {
		return nil, conflictErrorWithResource(
			resourceSourceServer, sourceServerID,
			"source server lifecycle state must be DISCONNECTED or CUTOVER to archive: "+sourceServerID,
		)
	}

	s.IsArchived = true

	return s.clone(), nil
}

// StartReplication restarts data replication for a stopped/disconnected
// source server -- a void-result op (StartReplicationOutput genuinely has
// no fields beyond ResultMetadata, confirmed by direct SDK read, matching
// PARITY.md's note that this asymmetry with its Stop/Pause/Resume siblings
// is real, not an oversight).
func (b *InMemoryBackend) StartReplication(sourceServerID string) error {
	b.mu.Lock("StartReplication")
	defer b.mu.Unlock()

	if err := b.requireInitializedLocked(); err != nil {
		return err
	}

	s, ok := b.resolveSourceServerLocked(sourceServerID)
	if !ok {
		return notFoundError(resourceSourceServer, sourceServerID)
	}

	if s.DataReplicationInfo == nil {
		s.DataReplicationInfo = &DataReplicationInfo{}
	}

	s.DataReplicationInfo.DataReplicationState = DataReplicationStateInitiating
	s.DataReplicationInfo.DataReplicationInitiation = &DataReplicationInitiation{
		StartDateTime: nowRFC3339(),
		Steps:         seedInitiationSteps(),
	}

	b.scheduleReplicationLocked(sourceServerID)

	return nil
}

// StopReplication halts data replication. Any in-flight
// scheduleReplicationLocked tick becomes a no-op once it observes the state
// is no longer what it expects (see that function's own doc comment).
func (b *InMemoryBackend) StopReplication(sourceServerID string) (*SourceServer, error) {
	return b.setReplicationState(sourceServerID, DataReplicationStateStopped)
}

// PauseReplication pauses data replication.
func (b *InMemoryBackend) PauseReplication(sourceServerID string) (*SourceServer, error) {
	return b.setReplicationState(sourceServerID, DataReplicationStatePaused)
}

// ResumeReplication resumes a paused replication back to CONTINUOUS --
// simplified: this emulator has no real backlog to re-drain, so resuming
// goes directly back to CONTINUOUS rather than re-walking INITIAL_SYNC/
// BACKLOG, a documented simplification (not an SDK-specified behavior).
func (b *InMemoryBackend) ResumeReplication(sourceServerID string) (*SourceServer, error) {
	b.mu.Lock("ResumeReplication")
	defer b.mu.Unlock()

	if err := b.requireInitializedLocked(); err != nil {
		return nil, err
	}

	s, ok := b.resolveSourceServerLocked(sourceServerID)
	if !ok {
		return nil, notFoundError(resourceSourceServer, sourceServerID)
	}

	if s.DataReplicationInfo == nil || s.DataReplicationInfo.DataReplicationState != DataReplicationStatePaused {
		return nil, conflictErrorWithResource(
			resourceSourceServer,
			sourceServerID,
			"source server is not paused: "+sourceServerID,
		)
	}

	s.DataReplicationInfo.DataReplicationState = DataReplicationStateContinuous

	return s.clone(), nil
}

// RetryDataReplication restarts a stalled/errored replication. This emulator
// never fabricates a failure condition (no DataReplicationErrorString value
// is ever set -- PARITY.md), so there is no real "stalled" state to retry
// out of; this always succeeds by restarting the same deterministic
// progression StartReplication uses, documented as a simplification.
func (b *InMemoryBackend) RetryDataReplication(sourceServerID string) (*SourceServer, error) {
	b.mu.Lock("RetryDataReplication")
	defer b.mu.Unlock()

	if err := b.requireInitializedLocked(); err != nil {
		return nil, err
	}

	s, ok := b.resolveSourceServerLocked(sourceServerID)
	if !ok {
		return nil, notFoundError(resourceSourceServer, sourceServerID)
	}

	s.DataReplicationInfo = &DataReplicationInfo{
		DataReplicationState: DataReplicationStateInitiating,
		DataReplicationInitiation: &DataReplicationInitiation{
			StartDateTime: nowRFC3339(),
			Steps:         seedInitiationSteps(),
		},
		ReplicatedDisks: s.DataReplicationInfo.ReplicatedDisks,
	}

	b.scheduleReplicationLocked(sourceServerID)

	return s.clone(), nil
}

// setReplicationState is the shared helper backing Stop/PauseReplication --
// both simply pin DataReplicationState to a caller-requested terminal-ish
// value.
func (b *InMemoryBackend) setReplicationState(sourceServerID, state string) (*SourceServer, error) {
	b.mu.Lock("setReplicationState:" + state)
	defer b.mu.Unlock()

	if err := b.requireInitializedLocked(); err != nil {
		return nil, err
	}

	s, ok := b.resolveSourceServerLocked(sourceServerID)
	if !ok {
		return nil, notFoundError(resourceSourceServer, sourceServerID)
	}

	if s.DataReplicationInfo == nil {
		s.DataReplicationInfo = &DataReplicationInfo{}
	}

	s.DataReplicationInfo.DataReplicationState = state

	return s.clone(), nil
}

// StartTest starts a batch test Job across sourceServerIDs. Real AWS
// requires READY_FOR_TEST; StartTest's own error set (Conflict,
// UninitializedAccount, Validation -- no ResourceNotFound, confirmed by
// direct SDK read) means an unknown SourceServerID cannot 404 here, so this
// backend folds it into ValidationException instead (a documented
// necessity, not a guess -- there is no other error shape this op can use
// for that condition).
func (b *InMemoryBackend) StartTest(sourceServerIDs []string, jobTags map[string]string) (*Job, error) {
	return b.startBatchJob(sourceServerIDs, jobTags, JobTypeLaunch, InitiatedByStartTest)
}

// StartCutover starts a batch cutover Job across sourceServerIDs. Same
// error-shape constraint as StartTest -- see its doc comment.
func (b *InMemoryBackend) StartCutover(sourceServerIDs []string, jobTags map[string]string) (*Job, error) {
	return b.startBatchJob(sourceServerIDs, jobTags, JobTypeLaunch, InitiatedByStartCutover)
}

// TerminateTargetInstances starts a batch terminate Job across
// sourceServerIDs. Same error-shape constraint as StartTest -- see its doc
// comment.
func (b *InMemoryBackend) TerminateTargetInstances(sourceServerIDs []string, jobTags map[string]string) (*Job, error) {
	return b.startBatchJob(sourceServerIDs, jobTags, JobTypeTerminate, InitiatedByTerminate)
}

// startBatchJob validates sourceServerIDs and each one's LifeCycleState
// precondition for kind, then delegates Job creation/scheduling to
// jobs.go's createAndScheduleJobLocked. StartTest requires READY_FOR_TEST
// and StartCutover requires READY_FOR_CUTOVER -- this package's own
// inference from field/enum semantics, not independently SDK-confirmed
// (PARITY.md). TerminateTargetInstances is SDK-doc-confirmed: see
// requireLifecyclePrecondition.
func (b *InMemoryBackend) startBatchJob(
	sourceServerIDs []string,
	jobTags map[string]string,
	jobType, initiatedBy string,
) (*Job, error) {
	b.mu.Lock("startBatchJob:" + initiatedBy)
	defer b.mu.Unlock()

	if err := b.requireInitializedLocked(); err != nil {
		return nil, err
	}

	if len(sourceServerIDs) == 0 {
		return nil, validationError("sourceServerIDs must not be empty")
	}

	servers := make([]*SourceServer, 0, len(sourceServerIDs))

	for _, id := range sourceServerIDs {
		s, ok := b.resolveSourceServerLocked(id)
		if !ok {
			return nil, validationError("unknown source server: " + id)
		}

		if err := requireLifecyclePrecondition(s, initiatedBy); err != nil {
			return nil, err
		}

		servers = append(servers, s)
	}

	return b.createAndScheduleJobLocked(servers, jobTags, jobType, initiatedBy), nil
}

// requireLifecyclePrecondition enforces the (documented, SDK-inferred)
// legal precondition for starting a batch job of the given kind on s.
// TerminateTargetInstances' block list is confirmed by
// api_op_TerminateTargetInstances.go:13-14 ("This command will not work for
// any Source Server with a lifecycle.state of TESTING, CUTTING_OVER, or
// CUTOVER").
func requireLifecyclePrecondition(s *SourceServer, initiatedBy string) error {
	state := ""
	if s.LifeCycle != nil {
		state = s.LifeCycle.State
	}

	switch initiatedBy {
	case InitiatedByStartTest:
		if state != LifeCycleStateReadyForTest {
			return conflictErrorWithResource(
				resourceSourceServer, s.SourceServerID,
				"source server is not ready for test: "+s.SourceServerID,
			)
		}
	case InitiatedByStartCutover:
		if state != LifeCycleStateReadyForCutover {
			return conflictErrorWithResource(
				resourceSourceServer, s.SourceServerID,
				"source server is not ready for cutover: "+s.SourceServerID,
			)
		}
	case InitiatedByTerminate:
		if state == LifeCycleStateTesting || state == LifeCycleStateCuttingOver || state == LifeCycleStateCutover {
			return conflictErrorWithResource(
				resourceSourceServer, s.SourceServerID,
				"cannot terminate target instances while source server lifecycle state is "+state+": "+s.SourceServerID,
			)
		}
	}

	return nil
}

// containsStr reports whether ss contains s.
func containsStr(ss []string, s string) bool {
	return slices.Contains(ss, s)
}

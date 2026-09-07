package workspaces

import (
	"context"
	"encoding/base64"
	"fmt"
	"maps"
	"sort"
	"time"

	sdktypes "github.com/aws/aws-sdk-go-v2/service/workspaces/types"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// connectionStatusPageSize is this backend's internal page size for the
// unfiltered (WorkspaceIds omitted) path of DescribeWorkspacesConnectionStatus.
// The real DescribeWorkspacesConnectionStatusInput has no MaxResults field
// (only NextToken), so the page size is entirely server-chosen, matching the
// pattern already used by DescribeAccountModifications/
// ListAvailableManagementCidrRanges (account.go).
const connectionStatusPageSize = 100

const (
	workspaceIDPrefix = "ws-"
	// AWS workspace IDs use 8 lowercase hex characters after the prefix.
	workspaceIDHexLen     = 8
	stateAvailable        = "AVAILABLE"
	stateAdminMaintenance = "ADMIN_MAINTENANCE"
	stateStopped          = "STOPPED"
	statePending          = "PENDING"
	stateUnhealthy        = "UNHEALTHY"
	errMsgNotFound        = "Workspace not found"

	// describeWorkspacesMaxResults is the AWS maximum results per page.
	describeWorkspacesMaxResults = 25
	// maxWorkspacesPerCreate is the AWS limit per CreateWorkspaces call.
	maxWorkspacesPerCreate = 25
)

// isRebootableWorkspaceState matches RebootWorkspaces's real precondition:
// "You cannot reboot a WorkSpace unless its state is AVAILABLE, UNHEALTHY, or
// REBOOTING" (api_op_RebootWorkspaces.go doc comment). This backend never
// produces UNHEALTHY/REBOOTING, so only AVAILABLE is reachable here, but the
// full real allow-list is checked for correctness.
func isRebootableWorkspaceState(state string) bool {
	switch state {
	case stateAvailable, stateUnhealthy, "REBOOTING":
		return true
	}

	return false
}

// isRebuildableWorkspaceState matches RebuildWorkspaces's real precondition:
// "You cannot rebuild a WorkSpace unless its state is AVAILABLE, ERROR,
// UNHEALTHY, STOPPED, or REBOOTING" (api_op_RebuildWorkspaces.go doc
// comment).
func isRebuildableWorkspaceState(state string) bool {
	switch state {
	case stateAvailable, "ERROR", stateUnhealthy, stateStopped, "REBOOTING":
		return true
	}

	return false
}

// isStartableWorkspaceState matches StartWorkspaces's real precondition:
// "You cannot start a WorkSpace unless it has a running mode of AutoStop or
// Manual and a state of STOPPED" (api_op_StartWorkspaces.go doc comment).
func isStartableWorkspaceState(state, runningMode string) bool {
	return state == stateStopped && isEligibleRunningMode(runningMode)
}

// isStoppableWorkspaceState matches StopWorkspaces's real precondition:
// "You cannot stop a WorkSpace unless it has a running mode of AutoStop or
// Manual and a state of AVAILABLE, IMPAIRED, UNHEALTHY, or ERROR"
// (api_op_StopWorkspaces.go doc comment).
func isStoppableWorkspaceState(state, runningMode string) bool {
	switch state {
	case stateAvailable, "IMPAIRED", stateUnhealthy, "ERROR":
		return isEligibleRunningMode(runningMode)
	}

	return false
}

// isEligibleRunningMode matches the running-mode half of the Start/StopWorkspaces
// precondition ("a running mode of AutoStop or Manual", api_op_StartWorkspaces.go /
// api_op_StopWorkspaces.go doc comments). MANUAL is WorkSpaces Core-only and
// unreachable here -- isValidRunningMode rejects it in ModifyWorkspaceProperties --
// but is still checked, matching isRebootableWorkspaceState's precedent for
// unreachable-but-documented values.
func isEligibleRunningMode(mode string) bool {
	return mode == string(sdktypes.RunningModeAutoStop) || mode == string(sdktypes.RunningModeManual)
}

// workspaceRunningMode returns w's running mode, or "" if it has no properties set.
func workspaceRunningMode(w *storedWorkspace) string {
	if w.Properties == nil {
		return ""
	}

	return w.Properties.RunningMode
}

// isValidComputeTypeName derives its answer from types.Compute.Values() so it
// cannot fall behind AWS adding new bundle families (e.g. the G6/GR6 GPU
// tiers) the way a hand-copied literal list did.
func isValidComputeTypeName(name string) bool {
	for _, v := range sdktypes.Compute("").Values() {
		if string(v) == name {
			return true
		}
	}

	return false
}

func isValidRunningMode(mode string) bool {
	return mode == "ALWAYS_ON" || mode == "AUTO_STOP"
}

func (w *storedWorkspace) toWorkspace() *Workspace {
	tags := make(map[string]string)
	maps.Copy(tags, w.Tags)

	var props *WorkspaceProperties
	if w.Properties != nil {
		p := *w.Properties
		props = &p
	}

	var dataRepl *DataReplicationSettings
	if w.DataReplicationSettings != nil {
		d := *w.DataReplicationSettings
		dataRepl = &d
	}

	var related []RelatedWorkspace
	if len(w.RelatedWorkspaces) > 0 {
		related = make([]RelatedWorkspace, len(w.RelatedWorkspaces))
		copy(related, w.RelatedWorkspaces)
	}

	var standbyProps []StandbyWorkspaceProperties
	if len(w.StandbyWorkspacesProperties) > 0 {
		standbyProps = make([]StandbyWorkspaceProperties, len(w.StandbyWorkspacesProperties))
		copy(standbyProps, w.StandbyWorkspacesProperties)
	}

	var modStates []ModificationState
	if len(w.ModificationStates) > 0 {
		modStates = make([]ModificationState, len(w.ModificationStates))
		copy(modStates, w.ModificationStates)
	}

	return &Workspace{
		WorkspaceID:                 w.WorkspaceID,
		WorkspaceName:               w.WorkspaceName,
		DirectoryID:                 w.DirectoryID,
		UserName:                    w.UserName,
		IPAddress:                   w.IPAddress,
		BundleID:                    w.BundleID,
		State:                       w.State,
		ComputerName:                w.ComputerName,
		SubnetID:                    w.SubnetID,
		VolumeEncryptionKey:         w.VolumeEncryptionKey,
		UserVolumeEncryptionEnabled: w.UserVolumeEncryptionEnabled,
		RootVolumeEncryptionEnabled: w.RootVolumeEncryptionEnabled,
		ErrorCode:                   w.ErrorCode,
		ErrorMessage:                w.ErrorMessage,
		Tags:                        tags,
		Properties:                  props,
		DataReplicationSettings:     dataRepl,
		StandbyWorkspacesProperties: standbyProps,
		RelatedWorkspaces:           related,
		ModificationStates:          modStates,
	}
}

// CreateWorkspace creates a new WorkSpace and returns it.
// Returns InvalidParameterValuesException when spec.DirectoryID is not registered.
func (b *InMemoryBackend) CreateWorkspace(
	ctx context.Context,
	spec *WorkspaceCreationSpec,
) (*Workspace, error) {
	region := b.regionFor(ctx)

	b.mu.Lock("CreateWorkspace")
	defer b.mu.Unlock()

	if !b.dirSettings.Has(spec.DirectoryID) {
		return nil, awserr.Newf(
			"directory %q is not registered", awserr.ErrInvalidParameter, spec.DirectoryID)
	}

	b.counter++
	workspaceID := fmt.Sprintf("%s%0*x", workspaceIDPrefix, workspaceIDHexLen, b.counter)

	storedTags := make(map[string]string)
	maps.Copy(storedTags, spec.Tags)

	var props *WorkspaceProperties
	if spec.Properties != nil {
		p := *spec.Properties
		props = &p
	}

	const maxDefaultSubnetHost = 250
	ipAddr := fmt.Sprintf("172.16.0.%d", (b.counter%maxDefaultSubnetHost)+1)

	w := &storedWorkspace{
		WorkspaceID: workspaceID,
		// WorkspaceName is only ever "not applicable" (real AWS: absent) for a
		// user-assigned WorkSpace and real, caller-supplied data for a
		// user-decoupled one (types.WorkspaceRequest.WorkspaceName's doc
		// comment) -- never derived from UserName/WorkspaceID.
		WorkspaceName:               spec.WorkspaceName,
		DirectoryID:                 spec.DirectoryID,
		UserName:                    spec.UserName,
		IPAddress:                   ipAddr,
		BundleID:                    spec.BundleID,
		SubnetID:                    spec.SubnetID,
		VolumeEncryptionKey:         spec.VolumeEncryptionKey,
		UserVolumeEncryptionEnabled: spec.UserVolumeEncryptionEnabled,
		RootVolumeEncryptionEnabled: spec.RootVolumeEncryptionEnabled,
		State:                       stateAvailable,
		Tags:                        storedTags,
		Properties:                  props,
		Region:                      region,
	}

	b.workspaces.Put(w)
	b.tags[workspaceID] = storedTags

	return w.toWorkspace(), nil
}

// DescribeWorkspaces returns workspaces matching the given filters.
// Results are sorted by WorkspaceId and paginated (max 25 per page, matching AWS).
func (b *InMemoryBackend) DescribeWorkspaces(
	ctx context.Context,
	workspaceIDs, directoryIDs, userIDs, bundleIDs []string,
	limit int32, nextToken string,
) ([]*Workspace, string, error) {
	b.mu.RLock("DescribeWorkspaces")
	defer b.mu.RUnlock()

	matched := b.filterWorkspaces(b.regionFor(ctx), workspaceIDs, directoryIDs, userIDs, bundleIDs)

	sort.Slice(matched, func(i, j int) bool {
		return matched[i].WorkspaceID < matched[j].WorkspaceID
	})

	matched = advanceCursor(matched, nextToken)

	pageSize := resolvePageSize(limit)

	var newToken string

	if len(matched) > pageSize {
		newToken = base64.StdEncoding.EncodeToString([]byte(matched[pageSize].WorkspaceID))
		matched = matched[:pageSize]
	}

	result := make([]*Workspace, 0, len(matched))
	for _, w := range matched {
		result = append(result, w.toWorkspace())
	}

	return result, newToken, nil
}

// filterWorkspaces returns all stored workspaces that match all provided filters.
// Must be called with a read lock held.
func (b *InMemoryBackend) filterWorkspaces(
	region string,
	workspaceIDs, directoryIDs, userIDs, bundleIDs []string,
) []*storedWorkspace {
	idFilter := buildFilter(workspaceIDs)
	dirFilter := buildFilter(directoryIDs)
	userFilter := buildFilter(userIDs)
	bundleFilter := buildFilter(bundleIDs)

	var matched []*storedWorkspace

	for _, w := range b.workspaces.All() {
		if region != "" && w.Region != "" && w.Region != region {
			continue
		}

		if matchesFilter(idFilter, w.WorkspaceID) &&
			matchesFilter(dirFilter, w.DirectoryID) &&
			matchesFilter(userFilter, w.UserName) &&
			matchesFilter(bundleFilter, w.BundleID) {
			matched = append(matched, w)
		}
	}

	return matched
}

// advanceCursor removes all items that sort before the decoded nextToken cursor.
func advanceCursor(items []*storedWorkspace, nextToken string) []*storedWorkspace {
	if nextToken == "" {
		return items
	}

	cursorBytes, err := base64.StdEncoding.DecodeString(nextToken)
	if err != nil {
		return items
	}

	cursor := string(cursorBytes)

	for i, w := range items {
		if w.WorkspaceID >= cursor {
			return items[i:]
		}
	}

	return nil
}

// resolvePageSize clamps limit to the AWS-allowed range.
func resolvePageSize(limit int32) int {
	if limit <= 0 || int(limit) > describeWorkspacesMaxResults {
		return describeWorkspacesMaxResults
	}

	return int(limit)
}

// GetWorkspacesConnectionStatus returns connection status for the given workspace IDs.
// If no IDs are provided, returns status for all workspaces. AVAILABLE workspaces
// report DISCONNECTED (not yet connected in this emulator); STOPPED workspaces
// report NOT_CONNECTED, matching real AWS behaviour for offline workspaces.
func (b *InMemoryBackend) GetWorkspacesConnectionStatus(
	workspaceIDs []string, nextToken string,
) ([]*WorkspaceConnectionStatus, string, error) {
	b.mu.RLock("GetWorkspacesConnectionStatus")
	defer b.mu.RUnlock()

	connectionStateFor := func(state string) string {
		switch state {
		case stateStopped:
			return "NOT_CONNECTED"
		default:
			return "DISCONNECTED"
		}
	}

	// checkedAt is the timestamp of this connection-status check -- computed
	// once so every WorkSpace in the response reports the same check time,
	// matching a single point-in-time DescribeWorkspacesConnectionStatus call.
	checkedAt := time.Now().UTC()

	if len(workspaceIDs) == 0 {
		all := b.workspaces.All()

		// Real AWS's DescribeWorkspacesConnectionStatusInput/Output both
		// declare NextToken (unlike WorkspaceIds, which is capped at 25 by
		// the real doc comment), so the no-filter path genuinely paginates
		// and must be sorted -- All() is unspecified map order.
		sort.Slice(all, func(i, j int) bool { return all[i].WorkspaceID < all[j].WorkspaceID })

		pg := page.New(all, nextToken, 0, connectionStatusPageSize)

		result := make([]*WorkspaceConnectionStatus, 0, len(pg.Data))
		for _, w := range pg.Data {
			result = append(result, &WorkspaceConnectionStatus{
				WorkspaceID:                   w.WorkspaceID,
				ConnectionState:               connectionStateFor(w.State),
				ConnectionStateCheckTimestamp: checkedAt,
			})
		}

		return result, pg.Next, nil
	}

	result := make([]*WorkspaceConnectionStatus, 0, len(workspaceIDs))

	for _, id := range workspaceIDs {
		w, ok := b.workspaces.Get(id)
		if !ok {
			continue
		}

		result = append(result, &WorkspaceConnectionStatus{
			WorkspaceID:                   w.WorkspaceID,
			ConnectionState:               connectionStateFor(w.State),
			ConnectionStateCheckTimestamp: checkedAt,
		})
	}

	return result, "", nil
}

// ModifyWorkspaceProperties updates and persists mutable properties of a WorkSpace.
// Returns InvalidParameterValuesException for unknown compute type names or running modes.
func (b *InMemoryBackend) ModifyWorkspaceProperties(
	workspaceID string,
	props WorkspaceProperties,
) error {
	if props.ComputeTypeName != "" && !isValidComputeTypeName(props.ComputeTypeName) {
		return awserr.Newf(
			"invalid ComputeTypeName: %q", awserr.ErrInvalidParameter, props.ComputeTypeName)
	}

	if props.RunningMode != "" && !isValidRunningMode(props.RunningMode) {
		return awserr.Newf(
			"invalid RunningMode: %q, must be ALWAYS_ON or AUTO_STOP",
			awserr.ErrInvalidParameter, props.RunningMode)
	}

	if props.RunningModeAutoStopTimeoutInMinutes != 0 {
		// AWS requires the timeout to be a multiple of 60 and between 60 and 600.
		t := props.RunningModeAutoStopTimeoutInMinutes
		if t < 60 || t > 600 || t%60 != 0 {
			return awserr.Newf(
				"RunningModeAutoStopTimeoutInMinutes must be a multiple of 60 between 60 and 600, got %d",
				awserr.ErrInvalidParameter,
				t,
			)
		}
	}

	b.mu.Lock("ModifyWorkspaceProperties")
	defer b.mu.Unlock()

	w, ok := b.workspaces.Get(workspaceID)
	if !ok {
		return ErrWorkspaceNotFound
	}

	p := props
	w.Properties = &p

	return nil
}

// ModifyWorkspaceState updates the administrative state of a WorkSpace.
func (b *InMemoryBackend) ModifyWorkspaceState(workspaceID, state string) error {
	b.mu.Lock("ModifyWorkspaceState")
	defer b.mu.Unlock()

	w, ok := b.workspaces.Get(workspaceID)
	if !ok {
		return ErrWorkspaceNotFound
	}

	if state != stateAvailable && state != stateAdminMaintenance {
		return ErrInvalidParameter
	}

	w.State = state

	return nil
}

// RebootWorkspaces reboots the given workspaces, returning failures for
// unknown IDs or a workspace whose state doesn't support rebooting.
func (b *InMemoryBackend) RebootWorkspaces(workspaceIDs []string) ([]FailedRequest, error) {
	b.mu.Lock("RebootWorkspaces")
	defer b.mu.Unlock()

	return b.collectStateFailures(workspaceIDs, isRebootableWorkspaceState), nil
}

// RebuildWorkspaces rebuilds the given workspaces, returning failures for
// unknown IDs or a workspace whose state doesn't support rebuilding.
func (b *InMemoryBackend) RebuildWorkspaces(workspaceIDs []string) ([]FailedRequest, error) {
	b.mu.Lock("RebuildWorkspaces")
	defer b.mu.Unlock()

	return b.collectStateFailures(workspaceIDs, isRebuildableWorkspaceState), nil
}

// StartWorkspaces starts the given workspaces, transitioning STOPPED workspaces to
// AVAILABLE. Returns a per-item failure for unknown IDs or a workspace whose state
// or running mode doesn't support starting.
func (b *InMemoryBackend) StartWorkspaces(workspaceIDs []string) ([]FailedRequest, error) {
	b.mu.Lock("StartWorkspaces")
	defer b.mu.Unlock()

	var failures []FailedRequest

	for _, id := range workspaceIDs {
		w, ok := b.workspaces.Get(id)
		if !ok {
			failures = append(failures, FailedRequest{
				WorkspaceID:  id,
				ErrorCode:    errResourceNotFound,
				ErrorMessage: errMsgNotFound,
			})

			continue
		}

		if !isStartableWorkspaceState(w.State, workspaceRunningMode(w)) {
			failures = append(failures, startStopFailure(id, w.State, workspaceRunningMode(w)))

			continue
		}

		w.State = stateAvailable
	}

	return failures, nil
}

// StopWorkspaces stops the given workspaces, transitioning them to STOPPED state.
// Returns a per-item failure for unknown IDs or a workspace whose state or
// running mode doesn't support stopping.
func (b *InMemoryBackend) StopWorkspaces(workspaceIDs []string) ([]FailedRequest, error) {
	b.mu.Lock("StopWorkspaces")
	defer b.mu.Unlock()

	var failures []FailedRequest

	for _, id := range workspaceIDs {
		w, ok := b.workspaces.Get(id)
		if !ok {
			failures = append(failures, FailedRequest{
				WorkspaceID:  id,
				ErrorCode:    errResourceNotFound,
				ErrorMessage: errMsgNotFound,
			})

			continue
		}

		if !isStoppableWorkspaceState(w.State, workspaceRunningMode(w)) {
			failures = append(failures, startStopFailure(id, w.State, workspaceRunningMode(w)))

			continue
		}

		w.State = stateStopped
	}

	return failures, nil
}

// startStopFailure builds a per-item FailedRequest for a workspace whose current
// state or running mode doesn't support Start/StopWorkspaces. Start/StopWorkspaces
// uniquely model InvalidResourceStateException at the operation level (unlike
// Reboot/Rebuild, which model only OperationNotSupportedException -- see
// collectStateFailures), so that is the ErrorCode used here for both halves of
// their documented precondition.
func startStopFailure(id, state, runningMode string) FailedRequest {
	return FailedRequest{
		WorkspaceID: id,
		ErrorCode:   errInvalidResourceState,
		ErrorMessage: fmt.Sprintf(
			"WorkSpace %s is not in a state that supports this operation "+
				"(current state: %s, running mode: %s)",
			id, state, runningMode,
		),
	}
}

// TerminateWorkspaces terminates (deletes) the given workspaces, returning failures for unknown IDs.
func (b *InMemoryBackend) TerminateWorkspaces(workspaceIDs []string) ([]FailedRequest, error) {
	b.mu.Lock("TerminateWorkspaces")
	defer b.mu.Unlock()

	var failures []FailedRequest

	for _, id := range workspaceIDs {
		if !b.workspaces.Has(id) {
			failures = append(failures, FailedRequest{
				WorkspaceID:  id,
				ErrorCode:    errResourceNotFound,
				ErrorMessage: errMsgNotFound,
			})

			continue
		}

		delete(b.tags, id)
		b.workspaces.Delete(id)
	}

	return failures, nil
}

// collectStateFailures reports a per-item failure for an unknown workspace
// ID or one whose current state isAllowed rejects, matching the batch
// FailedWorkspaceChangeRequest shape RebootWorkspaces/RebuildWorkspaces use
// for both cases (there is no separate operation-level exception for a bad
// state -- deserializers.go's error switch for both ops only lists
// OperationNotSupportedException, used here as the per-item ErrorCode).
func (b *InMemoryBackend) collectStateFailures(
	workspaceIDs []string, isAllowed func(state string) bool,
) []FailedRequest {
	var failures []FailedRequest

	for _, id := range workspaceIDs {
		w, ok := b.workspaces.Get(id)
		if !ok {
			failures = append(failures, FailedRequest{
				WorkspaceID:  id,
				ErrorCode:    errResourceNotFound,
				ErrorMessage: errMsgNotFound,
			})

			continue
		}

		if !isAllowed(w.State) {
			failures = append(failures, FailedRequest{
				WorkspaceID: id,
				ErrorCode:   errOperationNotSupported,
				ErrorMessage: fmt.Sprintf(
					"WorkSpace %s is not in a state that supports this operation (current state: %s)",
					id, w.State,
				),
			})
		}
	}

	return failures
}

// MigrateWorkspace migrates a workspace to a new bundle.
func (b *InMemoryBackend) MigrateWorkspace( //nolint:nonamedreturns // existing issue.
	sourceWorkspaceID, bundleID string,
) (sourceID, targetID string, err error) {
	b.mu.Lock("MigrateWorkspace")
	defer b.mu.Unlock()

	src, ok := b.workspaces.Get(sourceWorkspaceID)
	if !ok {
		return "", "", ErrWorkspaceNotFound
	}

	b.counter++
	newID := fmt.Sprintf("%s%0*x", workspaceIDPrefix, workspaceIDHexLen, b.counter)

	newWs := &storedWorkspace{
		WorkspaceID: newID,
		DirectoryID: src.DirectoryID,
		UserName:    src.UserName,
		BundleID:    bundleID,
		State:       stateAvailable,
		Tags:        cloneTags(src.Tags),
	}
	b.workspaces.Put(newWs)

	// Terminate old
	b.workspaces.Delete(sourceWorkspaceID)
	delete(b.tags, sourceWorkspaceID)

	return sourceWorkspaceID, newID, nil
}

// RestoreWorkspace restores a WorkSpace from its most recent snapshot. This backend
// does not model snapshots, so the operation is otherwise a no-op beyond existence
// validation, matching real AWS's ResourceNotFoundException for unknown WorkspaceIds.
func (b *InMemoryBackend) RestoreWorkspace(workspaceID string) error {
	b.mu.RLock("RestoreWorkspace")
	defer b.mu.RUnlock()

	if !b.workspaces.Has(workspaceID) {
		return ErrWorkspaceNotFound
	}

	return nil
}

// CreateStandbyWorkspace creates a single standby WorkSpace and returns it in
// PENDING state. Returns InvalidParameterValuesException when spec.DirectoryID
// is not registered, matching the same per-item runtime validation as
// CreateWorkspace. The real StandbyWorkspace request shape carries no
// UserName/BundleId (see StandbyWorkspaceSpec) -- those fields belong to the
// primary WorkSpace, which may live in a different region's backend that this
// in-memory store cannot see, so the created record has no way to inherit
// them; PendingCreateStandbyWorkspacesRequest's real shape doesn't surface
// BundleId at all, and its UserName is left empty for the same reason.
func (b *InMemoryBackend) CreateStandbyWorkspace(
	_ context.Context, spec StandbyWorkspaceSpec,
) (*PendingStandbyWorkspace, error) {
	b.mu.Lock("CreateStandbyWorkspace")
	defer b.mu.Unlock()

	if !b.dirSettings.Has(spec.DirectoryID) {
		return nil, awserr.Newf(
			"directory %q is not registered", awserr.ErrInvalidParameter, spec.DirectoryID)
	}

	id := b.nextID(workspaceIDPrefix)
	tags := cloneTags(spec.Tags)

	w := &storedWorkspace{
		WorkspaceID:         id,
		WorkspaceName:       id,
		DirectoryID:         spec.DirectoryID,
		PrimaryWorkspaceID:  spec.PrimaryWorkspaceID,
		DataReplication:     spec.DataReplication,
		VolumeEncryptionKey: spec.VolumeEncryptionKey,
		State:               statePending,
		Tags:                tags,
	}
	if spec.DataReplication != "" {
		w.DataReplicationSettings = &DataReplicationSettings{
			DataReplication: spec.DataReplication,
		}
	}
	if spec.PrimaryWorkspaceID != "" {
		w.RelatedWorkspaces = []RelatedWorkspace{
			{
				WorkspaceID: spec.PrimaryWorkspaceID,
				Type:        "PRIMARY",
			},
		}
	}
	b.workspaces.Put(w)
	b.tags[id] = tags

	return &PendingStandbyWorkspace{
		WorkspaceID: id,
		DirectoryID: spec.DirectoryID,
		State:       statePending,
	}, nil
}

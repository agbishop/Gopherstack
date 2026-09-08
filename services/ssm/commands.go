package ssm

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// validMaxConcurrency matches SendCommandInput.MaxConcurrency and
// CreateAssociationInput.MaxConcurrency: an absolute node count with no
// leading zero, or a 1-100% percentage. ssm/2014-11-06/service-2.json
// MaxConcurrency shape: pattern "^([1-9][0-9]*|[1-9][0-9]%|[1-9]%|100%)$",
// min 1, max 7.
var validMaxConcurrency = regexp.MustCompile(`^([1-9][0-9]*|[1-9][0-9]%|[1-9]%|100%)$`)

// validMaxErrors matches SendCommandInput.MaxErrors and
// CreateAssociationInput.MaxErrors: like MaxConcurrency but also allows the
// literal "0" and "0%" (AWS: "If you specify 0, then the system stops
// sending requests after the first error"). ssm/2014-11-06/service-2.json
// MaxErrors shape: pattern "^([1-9][0-9]*|[0]|[1-9][0-9]%|[0-9]%|100%)$",
// min 1, max 7.
var validMaxErrors = regexp.MustCompile(`^([1-9][0-9]*|[0]|[1-9][0-9]%|[0-9]%|100%)$`)

const maxConcurrencyErrorsLen = 7

// validateMaxConcurrency rejects a MaxConcurrency value that doesn't match
// the wire model's pattern. An empty string (unset) is valid.
func validateMaxConcurrency(v string) error {
	if v == "" {
		return nil
	}

	if len(v) > maxConcurrencyErrorsLen || !validMaxConcurrency.MatchString(v) {
		return fmt.Errorf(
			"%w: MaxConcurrency must be an absolute number (e.g. 10) or a percentage (e.g. 10%%)",
			ErrValidationException,
		)
	}

	return nil
}

// validateMaxErrors rejects a MaxErrors value that doesn't match the wire
// model's pattern. An empty string (unset) is valid.
func validateMaxErrors(v string) error {
	if v == "" {
		return nil
	}

	if len(v) > maxConcurrencyErrorsLen || !validMaxErrors.MatchString(v) {
		return fmt.Errorf(
			"%w: MaxErrors must be an absolute number (e.g. 10) or a percentage (e.g. 10%%)",
			ErrValidationException,
		)
	}

	return nil
}

func (b *InMemoryBackend) commandsStore(region string) *store.Table[Command] {
	return getOrCreateTable(b, b.commands, "commands", region, commandKeyFn)
}
func (b *InMemoryBackend) commandInvocationsStore(region string) map[string][]CommandInvocation {
	return b.commandInvocations[region]
}

// SendCommand creates a command and drives it through the AWS state machine:
// Pending → InProgress → Success (synchronous no-op runner path).
func (b *InMemoryBackend) SendCommand(
	ctx context.Context,
	input *SendCommandInput,
) (*SendCommandOutput, error) {
	if err := validateMaxConcurrency(input.MaxConcurrency); err != nil {
		return nil, err
	}

	if err := validateMaxErrors(input.MaxErrors); err != nil {
		return nil, err
	}

	region := getRegion(ctx)
	b.mu.Lock("SendCommand")
	defer b.mu.Unlock()

	// AWS-RunPatchBaseline is one of AWS's ~150 built-in Systems Manager
	// documents that exist account-wide without needing to be created first;
	// only it (of the ones this emulator can act on) is recognised implicitly
	// here rather than requiring pre-registration like customer documents.
	exists := b.documentsStore(region).Has(input.DocumentName)
	if !exists && input.DocumentName != docRunPatchBaseline {
		return nil, ErrDocumentNotFound
	}

	now := UnixTimeFloat(time.Now())
	cmdID := uuid.NewString()

	// resolvedInstanceIDs drives the actual invocation set: InstanceIds plus
	// whatever Targets resolves to (AWS: "Targets is required if you don't
	// provide one or more managed node IDs in the call"). A Targets-only
	// caller -- the pattern AWS documents as required for at-scale sends --
	// must still get real invocations, not zero.
	resolvedInstanceIDs := mergeUniqueInstanceIDs(input.InstanceIDs, commandTargetInstanceIDs(input.Targets))

	timeoutSecs := input.TimeoutSeconds
	if timeoutSecs == 0 {
		timeoutSecs = 3600
	}

	// Start in Pending state; transition through InProgress to Success so callers
	// that snapshot state between transitions observe correct intermediate values.
	cmd := Command{
		CommandID:          cmdID,
		DocumentName:       input.DocumentName,
		DocumentVersion:    input.DocumentVersion,
		Parameters:         input.Parameters,
		Status:             commandStatusPending,
		StatusDetails:      commandStatusPending,
		RequestedDateTime:  now,
		ExpiresAfter:       now + b.commandExpirySecs,
		InstanceIDs:        input.InstanceIDs,
		Targets:            input.Targets,
		Comment:            input.Comment,
		TimeoutSeconds:     timeoutSecs,
		OutputS3BucketName: input.OutputS3BucketName,
		OutputS3KeyPrefix:  input.OutputS3KeyPrefix,
		OutputS3Region:     input.OutputS3Region,
		ServiceRole:        input.ServiceRoleArn,
		MaxConcurrency:     input.MaxConcurrency,
		MaxErrors:          input.MaxErrors,
	}

	b.commandsStore(region).Put(&cmd)

	stdout, stderr, finalStatus := renderCommandOutput(input.DocumentName, input.Parameters)

	if input.DocumentName == docRunPatchBaseline {
		for _, instanceID := range resolvedInstanceIDs {
			b.applyPatchBaselineOperation(region, instanceID, input.Parameters)
		}
	}

	invocations := make([]CommandInvocation, 0, len(resolvedInstanceIDs))
	for _, instanceID := range resolvedInstanceIDs {
		inv := CommandInvocation{
			CommandID:         cmdID,
			InstanceID:        instanceID,
			DocumentName:      input.DocumentName,
			DocumentVersion:   input.DocumentVersion,
			Status:            commandStatusPending,
			StatusDetails:     commandStatusPending,
			RequestedDateTime: now,
			Comment:           input.Comment,
			pendingStdout:     stdout,
			pendingStderr:     stderr,
			finalStatus:       finalStatus,
		}
		invocations = append(invocations, inv)
	}
	if b.commandInvocations[region] == nil {
		b.commandInvocations[region] = make(map[string][]CommandInvocation)
	}
	b.commandInvocationsStore(region)[cmdID] = invocations

	// Drive Pending → InProgress immediately so the InProgress window is always
	// observable. When no exec delay is configured the command then completes
	// synchronously (revealing output); otherwise it stays InProgress and is
	// lazily completed by reads once b.commandExecDelaySecs has elapsed.
	b.setCommandStatus(region, cmdID, commandStatusInProgress)

	if b.commandExecDelaySecs <= 0 {
		b.completeCommand(region, cmdID)
	} else {
		pendingPtr, _ := b.commandsStore(region).Get(cmdID)
		pending := *pendingPtr
		pending.completeAfter = now + b.commandExecDelaySecs
		b.commandsStore(region).Put(&pending)
	}

	// Return a snapshot of the current state.
	finalCmdPtr, _ := b.commandsStore(region).Get(cmdID)
	finalCmd := *finalCmdPtr
	finalCmd.TargetCount, finalCmd.CompletedCount, finalCmd.ErrorCount = commandCounts(invocations)

	return &SendCommandOutput{Command: finalCmd}, nil
}

// commandTargetInstanceIDs flattens a SendCommand Targets list into instance
// IDs, treating every target's Values as literal node IDs regardless of Key
// -- the same simplification buildAssocExecTargets (associations.go) already
// makes for AssociationTarget, kept consistent here rather than adding
// tag-based resolution this backend has no matching infra for.
func commandTargetInstanceIDs(targets []CommandTarget) []string {
	ids := make([]string, 0, len(targets))
	for _, t := range targets {
		ids = append(ids, t.Values...)
	}

	return ids
}

// mergeUniqueInstanceIDs unions a and b, preserving first-seen order and
// dropping duplicates so a node named in both InstanceIds and Targets is
// only invoked once.
func mergeUniqueInstanceIDs(a, b []string) []string {
	seen := make(map[string]struct{}, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))

	for _, id := range slices.Concat(a, b) {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}

	return out
}

// commandCounts computes real types.Command members TargetCount/
// CompletedCount/ErrorCount (api_op_SendCommand.go/api_op_ListCommands.go)
// from a command's invocations rather than storing them redundantly, so
// they can never drift out of sync with the invocations they summarize.
func commandCounts(invs []CommandInvocation) (int32, int32, int32) {
	var completed, errorCount int32

	for _, inv := range invs {
		switch inv.Status {
		case commandStatusSuccess:
			completed++
		case commandStatusFailed:
			completed++
			errorCount++
		}
	}

	target := int32(len(invs)) // #nosec G115 -- bounded by one SendCommand's InstanceIds, never near int32 max

	return target, completed, errorCount
}

// setCommandStatus mutates the command and all its invocations to the given
// non-terminal status. Must be called with b.mu held for writing.
func (b *InMemoryBackend) setCommandStatus(region, cmdID, status string) {
	cmdTable := b.commandsStore(region)

	cmdPtr, ok := cmdTable.Get(cmdID)
	if !ok {
		return
	}

	cmd := *cmdPtr
	cmd.Status = status
	cmd.StatusDetails = status
	cmdTable.Put(&cmd)

	invStore := b.commandInvocationsStore(region)
	invs := invStore[cmdID]

	for i := range invs {
		invs[i].Status = status
		invs[i].StatusDetails = status
	}

	invStore[cmdID] = invs
}

// completeCommand transitions an InProgress command to its terminal status and
// reveals the rendered output on each invocation. The command status is the
// worst per-invocation status (Failed dominates Success). Must be called with
// b.mu held for writing.
func (b *InMemoryBackend) completeCommand(region, cmdID string) {
	cmdTable := b.commandsStore(region)

	cmdPtr, ok := cmdTable.Get(cmdID)
	if !ok {
		return
	}

	cmd := *cmdPtr

	invStore := b.commandInvocationsStore(region)
	invs := invStore[cmdID]

	overall := commandStatusSuccess
	completionTime := UnixTimeFloat(time.Now())

	for i := range invs {
		final := invs[i].finalStatus
		if final == "" {
			final = commandStatusSuccess
		}

		invs[i].Status = final
		invs[i].StatusDetails = final
		invs[i].StandardOutputContent = invs[i].pendingStdout
		invs[i].StandardErrorContent = invs[i].pendingStderr
		invs[i].executionEndUnix = completionTime

		if final != commandStatusSuccess {
			overall = final
		}
	}

	invStore[cmdID] = invs

	cmd.Status = overall
	cmd.StatusDetails = overall
	cmd.completeAfter = 0
	cmdTable.Put(&cmd)
}

// materializeCommandLocked lazily completes an InProgress command whose exec
// delay has elapsed. Must be called with b.mu held for writing.
func (b *InMemoryBackend) materializeCommandLocked(region, cmdID string, nowUnix float64) {
	cmdPtr, ok := b.commandsStore(region).Get(cmdID)
	if !ok || cmdPtr.Status != commandStatusInProgress {
		return
	}

	if cmdPtr.completeAfter == 0 || nowUnix >= cmdPtr.completeAfter {
		b.completeCommand(region, cmdID)
	}
}

// materializeCommandsLocked lazily completes every eligible InProgress command
// in the region. Must be called with b.mu held for writing.
func (b *InMemoryBackend) materializeCommandsLocked(region string, nowUnix float64) {
	for _, cmdPtr := range b.commandsStore(region).All() {
		b.materializeCommandLocked(region, cmdPtr.CommandID, nowUnix)
	}
}

// ListCommands returns recorded commands.
func (b *InMemoryBackend) ListCommands(
	ctx context.Context,
	input *ListCommandsInput,
) (*ListCommandsOutput, error) {
	region := getRegion(ctx)
	b.mu.Lock("ListCommands")
	defer b.mu.Unlock()

	b.materializeCommandsLocked(region, UnixTimeFloat(timeNow()))

	cmdTable := b.commandsStore(region)
	invStore := b.commandInvocationsStore(region)
	all := make([]Command, 0, cmdTable.Len())
	for _, cmdPtr := range cmdTable.All() {
		if input.CommandID != "" && cmdPtr.CommandID != input.CommandID {
			continue
		}
		if input.InstanceID != "" && !slices.Contains(cmdPtr.InstanceIDs, input.InstanceID) {
			continue
		}
		cmd := *cmdPtr
		cmd.TargetCount, cmd.CompletedCount, cmd.ErrorCount = commandCounts(invStore[cmd.CommandID])
		all = append(all, cmd)
	}

	sort.Slice(all, func(i, j int) bool { return all[i].CommandID < all[j].CommandID })

	startIdx := parseNextToken(input.NextToken)

	maxResults := int64(defaultListDocMaxResults)
	if input.MaxResults != nil && *input.MaxResults > 0 {
		maxResults = *input.MaxResults
	}

	if startIdx >= len(all) {
		return &ListCommandsOutput{Commands: []Command{}}, nil
	}

	end := startIdx + int(maxResults)

	var nextToken string

	if end < len(all) {
		nextToken = strconv.Itoa(end)
	} else {
		end = len(all)
	}

	return &ListCommandsOutput{
		Commands:  all[startIdx:end],
		NextToken: nextToken,
	}, nil
}

// GetCommandInvocation returns the stored invocation for the given command and instance.
func (b *InMemoryBackend) GetCommandInvocation(
	ctx context.Context,
	input *GetCommandInvocationInput,
) (*GetCommandInvocationOutput, error) {
	region := getRegion(ctx)
	b.mu.Lock("GetCommandInvocation")
	defer b.mu.Unlock()

	if !b.commandsStore(region).Has(input.CommandID) {
		return nil, ErrCommandNotFound
	}

	b.materializeCommandLocked(region, input.CommandID, UnixTimeFloat(timeNow()))

	for _, inv := range b.commandInvocationsStore(region)[input.CommandID] {
		if inv.InstanceID == input.InstanceID {
			out := &GetCommandInvocationOutput{
				CommandID:              input.CommandID,
				InstanceID:             input.InstanceID,
				DocumentName:           inv.DocumentName,
				DocumentVersion:        inv.DocumentVersion,
				PluginName:             input.PluginName,
				Status:                 inv.Status,
				StatusDetails:          inv.StatusDetails,
				StandardOutputContent:  inv.StandardOutputContent,
				StandardErrorContent:   inv.StandardErrorContent,
				StandardOutputURL:      inv.StandardOutputURL,
				StandardErrorURL:       inv.StandardErrorURL,
				Comment:                inv.Comment,
				ExecutionStartDateTime: formatCommandTime(inv.RequestedDateTime),
			}
			if inv.executionEndUnix != 0 {
				out.ExecutionEndDateTime = formatCommandTime(inv.executionEndUnix)
			}

			return out, nil
		}
	}

	return nil, ErrCommandNotFound
}

// ListCommandInvocations returns invocations for a given command.
func (b *InMemoryBackend) ListCommandInvocations(
	ctx context.Context,
	input *ListCommandInvocationsInput,
) (*ListCommandInvocationsOutput, error) {
	region := getRegion(ctx)
	b.mu.Lock("ListCommandInvocations")
	defer b.mu.Unlock()

	b.materializeCommandsLocked(region, UnixTimeFloat(timeNow()))

	all := make([]CommandInvocation, 0, len(b.commandInvocationsStore(region)))
	for cmdID, invs := range b.commandInvocationsStore(region) {
		if input.CommandID != "" && cmdID != input.CommandID {
			continue
		}
		for _, inv := range invs {
			if input.InstanceID != "" && inv.InstanceID != input.InstanceID {
				continue
			}
			all = append(all, inv)
		}
	}

	sort.Slice(all, func(i, j int) bool {
		if all[i].CommandID != all[j].CommandID {
			return all[i].CommandID < all[j].CommandID
		}

		return all[i].InstanceID < all[j].InstanceID
	})

	startIdx := parseNextToken(input.NextToken)

	maxResults := int64(defaultListDocMaxResults)
	if input.MaxResults != nil && *input.MaxResults > 0 {
		maxResults = *input.MaxResults
	}

	if startIdx >= len(all) {
		return &ListCommandInvocationsOutput{CommandInvocations: []CommandInvocation{}}, nil
	}

	end := startIdx + int(maxResults)

	var nextToken string

	if end < len(all) {
		nextToken = strconv.Itoa(end)
	} else {
		end = len(all)
	}

	return &ListCommandInvocationsOutput{
		CommandInvocations: all[startIdx:end],
		NextToken:          nextToken,
	}, nil
}

// CancelCommand cancels a running command (sets status to Cancelled).
func (b *InMemoryBackend) CancelCommand(
	ctx context.Context,
	input *CancelCommandInput,
) (*CancelCommandOutput, error) {
	region := getRegion(ctx)
	b.mu.Lock("CancelCommand")
	defer b.mu.Unlock()

	cmdTable := b.commandsStore(region)
	cmdPtr, exists := cmdTable.Get(input.CommandID)
	if !exists {
		return nil, ErrCommandNotFound
	}

	// InstanceIds scopes cancellation to specific managed nodes -- if
	// omitted, every node the command was requested on is canceled
	// (api_op_CancelCommand.go: "If not provided, the command is canceled
	// on every node on which it was requested").
	invStore := b.commandInvocationsStore(region)
	invs := invStore[input.CommandID]
	allCancelled := true

	for i := range invs {
		if len(input.InstanceIDs) > 0 && !slices.Contains(input.InstanceIDs, invs[i].InstanceID) {
			if invs[i].Status != commandStatusCancelled {
				allCancelled = false
			}

			continue
		}

		invs[i].Status = commandStatusCancelled
		invs[i].StatusDetails = commandStatusCancelled
	}

	invStore[input.CommandID] = invs

	if allCancelled {
		cmd := *cmdPtr
		cmd.Status = commandStatusCancelled
		cmdTable.Put(&cmd)
	}

	return &CancelCommandOutput{}, nil
}

// formatCommandTime renders a Unix-seconds float64 as the ISO 8601 string
// GetCommandInvocationOutput's ExecutionStartDateTime/ExecutionEndDateTime
// use (api_op_GetCommandInvocation.go:90-109) -- these two members are real
// wire strings, not epoch numbers like every other timestamp in this
// package.
func formatCommandTime(unixSeconds float64) string {
	return time.Unix(int64(unixSeconds), 0).UTC().Format(time.RFC3339)
}

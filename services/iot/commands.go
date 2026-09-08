package iot

import (
	"fmt"
	"maps"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// IoTCommand represents an AWS IoT command.
//
//nolint:revive // IoTCommand is intentional to maintain AWS API naming clarity
type IoTCommand struct {
	Tags            map[string]string `json:"tags,omitempty"`
	Payload         map[string]any    `json:"payload,omitempty"`
	CommandARN      string            `json:"commandArn"`
	CommandID       string            `json:"commandId"`
	DisplayName     string            `json:"displayName,omitempty"`
	Description     string            `json:"description,omitempty"`
	Namespace       string            `json:"namespace,omitempty"`
	CreationDate    float64           `json:"creationDate,omitempty"`
	LastUpdated     float64           `json:"lastUpdatedAt,omitempty"`
	Deprecated      bool              `json:"deprecated"`
	PendingDeletion bool              `json:"pendingDeletion"`
}

func cloneIoTCommand(cmd *IoTCommand) *IoTCommand {
	cp := *cmd
	cp.Tags = make(map[string]string, len(cmd.Tags))
	maps.Copy(cp.Tags, cmd.Tags)
	cp.Payload = make(map[string]any, len(cmd.Payload))
	maps.Copy(cp.Payload, cmd.Payload)

	return &cp
}

func (b *InMemoryBackend) commandARN(id string) string {
	return arn.Build("iot", b.region, b.accountID, fmt.Sprintf("command/%s", id))
}

func (b *InMemoryBackend) CreateCommand(
	id, displayName, description, namespace string,
	payload map[string]any,
	tags map[string]string,
) (*IoTCommand, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.commands.Has(id) {
		return nil, fmt.Errorf("command %q already exists: %w", id, ErrAlreadyExists)
	}
	now := float64(time.Now().Unix())
	cmd := &IoTCommand{
		CommandID:    id,
		CommandARN:   b.commandARN(id),
		DisplayName:  displayName,
		Description:  description,
		Namespace:    namespace,
		Tags:         make(map[string]string),
		Payload:      make(map[string]any),
		CreationDate: now,
		LastUpdated:  now,
	}
	maps.Copy(cmd.Tags, tags)
	maps.Copy(cmd.Payload, payload)
	b.commands.Put(cmd)
	b.putResourceTagsLocked(cmd.CommandARN, cmd.Tags)

	return cloneIoTCommand(cmd), nil
}

func (b *InMemoryBackend) GetCommand(id string) (*IoTCommand, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	cmd, ok := b.commands.Get(id)
	if !ok {
		return nil, fmt.Errorf("command %q not found: %w", id, ErrResourceNotFound)
	}

	return cloneIoTCommand(cmd), nil
}

func (b *InMemoryBackend) UpdateCommand(id, displayName, description string, deprecated bool) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	cmd, ok := b.commands.Get(id)
	if !ok {
		return fmt.Errorf("command %q not found: %w", id, ErrResourceNotFound)
	}
	if displayName != "" {
		cmd.DisplayName = displayName
	}
	if description != "" {
		cmd.Description = description
	}
	cmd.Deprecated = deprecated
	cmd.LastUpdated = float64(time.Now().Unix())

	return nil
}

func (b *InMemoryBackend) DeleteCommand(id string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.commands.Has(id) {
		return fmt.Errorf("command %q not found: %w", id, ErrResourceNotFound)
	}
	b.commands.Delete(id)
	delete(b.resourceTags, b.commandARN(id))

	return nil
}

func (b *InMemoryBackend) ListCommands() []*IoTCommand {
	b.mu.RLock()
	defer b.mu.RUnlock()

	items := b.commands.Snapshot()
	out := make([]*IoTCommand, 0, len(items))
	for _, v := range items {
		out = append(out, cloneIoTCommand(v))
	}

	return out
}

// IoTCommandExecution represents an execution of an IoT command.
//
//nolint:revive // IoTCommandExecution is intentional to maintain AWS API naming clarity
type IoTCommandExecution struct {
	CommandARN   string  `json:"commandArn"`
	ExecutionID  string  `json:"executionId"`
	ThingARN     string  `json:"thingArn"`
	Status       string  `json:"status"`
	CreationDate float64 `json:"createdAt,omitempty"`
}

func (b *InMemoryBackend) commandExecutionKey(commandID, executionID string) string {
	return commandID + "/" + executionID
}

func (b *InMemoryBackend) GetCommandExecution(commandID, executionID string) (*IoTCommandExecution, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	key := b.commandExecutionKey(commandID, executionID)
	ex, ok := b.commandExecutions[key]
	if !ok {
		return nil, fmt.Errorf("command execution %q/%q not found: %w", commandID, executionID, ErrResourceNotFound)
	}
	cp := *ex

	return &cp, nil
}

func (b *InMemoryBackend) ListCommandExecutions(commandID string) []*IoTCommandExecution {
	b.mu.RLock()
	defer b.mu.RUnlock()

	prefix := commandID + "/"
	var out []*IoTCommandExecution
	for k, ex := range b.commandExecutions {
		if len(k) > len(prefix) && k[:len(prefix)] == prefix {
			cp := *ex
			out = append(out, &cp)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ExecutionID < out[j].ExecutionID })

	return out
}

// GetCommandExecutionByID looks up a command execution by executionId alone
// (optionally scoped by targetARN), matching the real GetCommandExecution
// request shape where executions are addressed by executionId+targetArn,
// not commandId+executionId (mirrors DeleteCommandExecution below).
func (b *InMemoryBackend) GetCommandExecutionByID(executionID, targetARN string) (*IoTCommandExecution, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, ex := range b.commandExecutions {
		if ex.ExecutionID != executionID {
			continue
		}
		if targetARN != "" && ex.ThingARN != targetARN {
			continue
		}
		cp := *ex

		return &cp, nil
	}

	return nil, fmt.Errorf("command execution %q not found: %w", executionID, ErrResourceNotFound)
}

// ListCommandExecutionsByFilter returns command executions matching the
// real ListCommandExecutions filters (commandARN and/or targetARN and/or
// status; each optional, empty means unfiltered). Backs the real
// POST /command-executions route. ListCommandExecutions above backs the
// separate legacy path-scoped route instead.
func (b *InMemoryBackend) ListCommandExecutionsByFilter(commandARN, targetARN, status string) []*IoTCommandExecution {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var out []*IoTCommandExecution
	for _, ex := range b.commandExecutions {
		if commandARN != "" && ex.CommandARN != commandARN {
			continue
		}
		if targetARN != "" && ex.ThingARN != targetARN {
			continue
		}
		if status != "" && ex.Status != status {
			continue
		}
		cp := *ex
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ExecutionID < out[j].ExecutionID })

	return out
}

// DeleteCommandExecution removes a stored command execution identified by
// its executionId and (optionally) the ARN of its target device, matching
// AWS's real request shape where executions are addressed by
// executionId+targetArn rather than commandId.
func (b *InMemoryBackend) DeleteCommandExecution(executionID, targetARN string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	for key, ex := range b.commandExecutions {
		if ex.ExecutionID != executionID {
			continue
		}

		if targetARN != "" && ex.ThingARN != targetARN {
			continue
		}

		delete(b.commandExecutions, key)

		return nil
	}

	return fmt.Errorf("command execution %q not found: %w", executionID, ErrResourceNotFound)
}

// AddCommandExecutionInternal seeds a command execution directly into the
// backend for testing (there is no public CreateCommandExecution control-
// plane operation; executions are normally created by device SDKs).
func (b *InMemoryBackend) AddCommandExecutionInternal(commandID, executionID string, ex IoTCommandExecution) {
	b.mu.Lock()
	defer b.mu.Unlock()

	cp := ex
	cp.ExecutionID = executionID
	b.commandExecutions[b.commandExecutionKey(commandID, executionID)] = &cp
}

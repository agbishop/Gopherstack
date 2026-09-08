package container

import (
	mobycontainer "github.com/moby/moby/api/types/container"

	compatfilters "github.com/blackbirdworks/gopherstack/internal/dockercompat/api/types/filters"
)

// Config aliases the Moby container config type.
type Config = mobycontainer.Config

// HostConfig aliases the Moby host config type.
type HostConfig = mobycontainer.HostConfig

// CreateResponse aliases the Moby create response type.
type CreateResponse = mobycontainer.CreateResponse

// Summary aliases the Moby container summary type.
type Summary = mobycontainer.Summary

// StartOptions models the subset of container start options used in this repo.
type StartOptions struct {
	CheckpointDir string
	CheckpointID  string
}

// StopOptions models the subset of container stop options used in this repo.
type StopOptions struct {
	Timeout *int
	Signal  string
}

// RemoveOptions models the subset of container remove options used in this repo.
type RemoveOptions struct {
	Force         bool
	RemoveLinks   bool
	RemoveVolumes bool
}

// ListOptions models the subset of container list options used in this repo.
type ListOptions struct {
	Filters compatfilters.Args
	Limit   int
	All     bool
	Size    bool
}

// LogsOptions models the subset of container logs options used in this repo.
type LogsOptions struct {
	Since      string
	Until      string
	Tail       string
	ShowStdout bool
	ShowStderr bool
	Timestamps bool
	Follow     bool
	Details    bool
}

// WaitOptions models the subset of container wait options used in this repo.
type WaitOptions struct {
	Condition mobycontainer.WaitCondition
}

// WaitResponse aliases the Moby container wait response type (StatusCode, Error).
type WaitResponse = mobycontainer.WaitResponse

// WaitResult mirrors moby/moby/client's ContainerWaitResult: exactly one of
// Result or Error receives a value once the wait resolves.
type WaitResult struct {
	Result <-chan WaitResponse
	Error  <-chan error
}

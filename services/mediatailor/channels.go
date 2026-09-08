package mediatailor

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

type storedChannel struct {
	FillerSlate      *SlateSource            `json:"fillerSlate,omitempty"`
	TimeShift        *TimeShiftConfiguration `json:"timeShiftConfiguration,omitempty"`
	CreationTime     time.Time               `json:"creationTime"`
	LastModified     time.Time               `json:"lastModified"`
	Tags             map[string]string       `json:"tags"`
	Name             string                  `json:"name"`
	ARN              string                  `json:"arn"`
	PlaybackMode     string                  `json:"playbackMode"`
	ChannelState     string                  `json:"channelState"`
	Tier             string                  `json:"tier"`
	Outputs          []OutputItem            `json:"outputs"`
	Audiences        []string                `json:"audiences,omitempty"`
	LogConfiguration ChannelLogConfiguration `json:"logConfiguration"`
}

func (c *storedChannel) toChannel() *Channel {
	tags := make(map[string]string, len(c.Tags))
	maps.Copy(tags, c.Tags)

	outputs := make([]OutputItem, len(c.Outputs))
	copy(outputs, c.Outputs)

	ch := &Channel{
		CreationTime:     c.CreationTime,
		LastModified:     c.LastModified,
		Tags:             tags,
		Name:             c.Name,
		ARN:              c.ARN,
		PlaybackMode:     c.PlaybackMode,
		ChannelState:     c.ChannelState,
		Tier:             c.Tier,
		Outputs:          outputs,
		Audiences:        slices.Clone(c.Audiences),
		LogConfiguration: ChannelLogConfiguration{LogTypes: slices.Clone(c.LogConfiguration.LogTypes)},
	}

	if c.FillerSlate != nil {
		ch.FillerSlate = &SlateSource{
			SourceLocationName: c.FillerSlate.SourceLocationName,
			VodSourceName:      c.FillerSlate.VodSourceName,
		}
	}

	if c.TimeShift != nil {
		ts := *c.TimeShift
		ch.TimeShift = &ts
	}

	return ch
}

func (c *storedChannel) toSummary() *ChannelSummary {
	tags := make(map[string]string, len(c.Tags))
	maps.Copy(tags, c.Tags)

	s := &ChannelSummary{
		CreationTime:     c.CreationTime,
		LastModified:     c.LastModified,
		Tags:             tags,
		Name:             c.Name,
		ARN:              c.ARN,
		PlaybackMode:     c.PlaybackMode,
		ChannelState:     c.ChannelState,
		Tier:             c.Tier,
		Outputs:          slices.Clone(c.Outputs),
		Audiences:        slices.Clone(c.Audiences),
		LogConfiguration: ChannelLogConfiguration{LogTypes: slices.Clone(c.LogConfiguration.LogTypes)},
	}

	if c.FillerSlate != nil {
		s.FillerSlate = &SlateSource{
			SourceLocationName: c.FillerSlate.SourceLocationName,
			VodSourceName:      c.FillerSlate.VodSourceName,
		}
	}

	if c.TimeShift != nil {
		ts := *c.TimeShift
		s.TimeShift = &ts
	}

	return s
}

// --- Channel operations ---

// CreateChannel creates a new channel.
func (b *InMemoryBackend) CreateChannel(
	name, playbackMode, tier string,
	outputs []OutputItem,
	fillerSlate *SlateSource,
	audiences []string,
	timeShift *TimeShiftConfiguration,
	tags map[string]string,
) (*Channel, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: ChannelName required", ErrInvalidParameter)
	}

	switch playbackMode {
	case "", playbackModeLoop:
		playbackMode = playbackModeLoop
	case playbackModeLinear:
	default:
		return nil, fmt.Errorf(
			"%w: PlaybackMode must be %s or %s",
			ErrInvalidParameter, playbackModeLinear, playbackModeLoop,
		)
	}

	switch tier {
	case "":
		tier = tierBasic
	case tierBasic, tierStandard:
	default:
		return nil, fmt.Errorf(
			"%w: Tier must be %s or %s", ErrInvalidParameter, tierBasic, tierStandard,
		)
	}

	b.mu.Lock("CreateChannel")
	defer b.mu.Unlock()

	if b.channels.Has(name) {
		return nil, fmt.Errorf("%w: channel %s already exists", ErrConflict, name)
	}

	out := make([]OutputItem, len(outputs))
	copy(out, outputs)

	now := time.Now().UTC()
	chARN := b.channelARN(name)
	ch := &storedChannel{
		Tags:         copyTags(tags),
		Name:         name,
		ARN:          chARN,
		PlaybackMode: playbackMode,
		ChannelState: channelStateStopped,
		Tier:         tier,
		CreationTime: now,
		LastModified: now,
		Outputs:      out,
		Audiences:    slices.Clone(audiences),
	}

	if fillerSlate != nil {
		ch.FillerSlate = &SlateSource{
			SourceLocationName: fillerSlate.SourceLocationName,
			VodSourceName:      fillerSlate.VodSourceName,
		}
	}

	if timeShift != nil {
		ts := *timeShift
		ch.TimeShift = &ts
	}

	b.channels.Put(ch)
	b.tags[chARN] = copyTags(tags)

	return ch.toChannel(), nil
}

// DescribeChannel returns a channel by name.
func (b *InMemoryBackend) DescribeChannel(name string) (*Channel, error) {
	b.mu.RLock("DescribeChannel")
	defer b.mu.RUnlock()

	ch, ok := b.channels.Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: channel %s not found", ErrNotFound, name)
	}

	result := ch.toChannel()
	result.Tags = make(map[string]string)
	maps.Copy(result.Tags, b.tags[ch.ARN])

	return result, nil
}

// UpdateChannel updates a channel's outputs, filler slate, audiences, and
// time-shift configuration.
func (b *InMemoryBackend) UpdateChannel(
	name string,
	outputs []OutputItem,
	fillerSlate *SlateSource,
	audiences []string,
	timeShift *TimeShiftConfiguration,
) (*Channel, error) {
	b.mu.Lock("UpdateChannel")
	defer b.mu.Unlock()

	ch, ok := b.channels.Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: channel %s not found", ErrNotFound, name)
	}

	out := make([]OutputItem, len(outputs))
	copy(out, outputs)
	ch.Outputs = out
	ch.Audiences = slices.Clone(audiences)
	ch.LastModified = time.Now().UTC()

	if fillerSlate != nil {
		ch.FillerSlate = &SlateSource{
			SourceLocationName: fillerSlate.SourceLocationName,
			VodSourceName:      fillerSlate.VodSourceName,
		}
	} else {
		ch.FillerSlate = nil
	}

	if timeShift != nil {
		ts := *timeShift
		ch.TimeShift = &ts
	} else {
		ch.TimeShift = nil
	}

	return ch.toChannel(), nil
}

// DeleteChannel deletes a channel, cascade-deleting its policy and every
// program scheduled on it so no ghost rows remain in the programs table
// (or its byChannel index) after the channel itself is gone.
func (b *InMemoryBackend) DeleteChannel(name string) error {
	b.mu.Lock("DeleteChannel")
	defer b.mu.Unlock()

	ch, ok := b.channels.Get(name)
	if !ok {
		return fmt.Errorf("%w: channel %s not found", ErrNotFound, name)
	}

	if ch.ChannelState == channelStateRunning {
		return fmt.Errorf("%w: channel must be stopped before deleting", ErrConflict)
	}

	for _, prog := range slices.Clone(b.programsByChannel.Get(name)) {
		b.programs.Delete(programKey(prog.ChannelName, prog.ProgramName))
	}

	delete(b.channelPolicies, name)
	delete(b.tags, ch.ARN)
	b.channels.Delete(name)

	return nil
}

// ListChannels returns a paginated list of channels.
func (b *InMemoryBackend) ListChannels(maxResults int, nextToken string) ([]*ChannelSummary, string, error) {
	b.mu.RLock("ListChannels")
	defer b.mu.RUnlock()

	all := b.channels.All()

	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })

	pg := page.New(all, nextToken, maxResults, defaultMaxResults)

	summaries := make([]*ChannelSummary, 0, len(pg.Data))
	for _, ch := range pg.Data {
		s := ch.toSummary()
		s.Tags = make(map[string]string)
		maps.Copy(s.Tags, b.tags[ch.ARN])
		summaries = append(summaries, s)
	}

	return summaries, pg.Next, nil
}

// StartChannel transitions a channel to RUNNING.
// Idempotent: no error if already running.
func (b *InMemoryBackend) StartChannel(name string) error {
	b.mu.Lock("StartChannel")
	defer b.mu.Unlock()

	ch, ok := b.channels.Get(name)
	if !ok {
		return fmt.Errorf("%w: channel %s not found", ErrNotFound, name)
	}

	ch.ChannelState = channelStateRunning

	return nil
}

// StopChannel transitions a channel to STOPPED.
// Idempotent: no error if already stopped.
func (b *InMemoryBackend) StopChannel(name string) error {
	b.mu.Lock("StopChannel")
	defer b.mu.Unlock()

	ch, ok := b.channels.Get(name)
	if !ok {
		return fmt.Errorf("%w: channel %s not found", ErrNotFound, name)
	}

	ch.ChannelState = channelStateStopped

	return nil
}

// ConfigureLogsForChannel sets log types on a channel and persists them so
// they are queryable back from Describe/List/CreateChannel's LogConfiguration.
func (b *InMemoryBackend) ConfigureLogsForChannel(channelName string, logTypes []string) (string, []string, error) {
	// LogTypes is "This member is required" (api_op_ConfigureLogsForChannel.go)
	// and LogType is a single-value enum, AS_RUN (types/enums.go).
	if len(logTypes) == 0 {
		return "", nil, fmt.Errorf("%w: LogTypes required", ErrInvalidParameter)
	}

	for _, lt := range logTypes {
		if lt != logTypeAsRun {
			return "", nil, fmt.Errorf("%w: LogTypes must be %s", ErrInvalidParameter, logTypeAsRun)
		}
	}

	b.mu.Lock("ConfigureLogsForChannel")
	defer b.mu.Unlock()

	ch, ok := b.channels.Get(channelName)
	if !ok {
		return "", nil, fmt.Errorf("%w: channel %s not found", ErrNotFound, channelName)
	}

	result := slices.Clone(logTypes)
	ch.LogConfiguration = ChannelLogConfiguration{LogTypes: slices.Clone(result)}

	return channelName, result, nil
}

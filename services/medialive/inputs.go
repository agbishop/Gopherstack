package medialive

import (
	"fmt"
	"sort"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// --- Input operations ---

// CreateInput creates a new input.
func (b *InMemoryBackend) CreateInput(
	name, inputType, roleArn string,
	sdiSources []string,
	tags map[string]string,
) (*Input, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: name required", ErrInvalidParameter)
	}

	if inputType == "" {
		inputType = inputTypeUDPPush
	}

	sources := make([]string, len(sdiSources))
	copy(sources, sdiSources)

	id := newID()
	inp := &storedInput{
		ARN:        b.inputARN(id),
		ID:         id,
		Name:       name,
		InputType:  inputType,
		RoleARN:    roleArn,
		State:      stateDetached,
		Tags:       copyTags(tags),
		SdiSources: sources,
	}

	b.mu.Lock("CreateInput")
	defer b.mu.Unlock()

	b.inputs.Put(inp)

	return inp.toInput(), nil
}

// DescribeInput returns an input by ID.
func (b *InMemoryBackend) DescribeInput(inputID string) (*Input, error) {
	b.mu.RLock("DescribeInput")
	defer b.mu.RUnlock()

	inp, ok := b.inputs.Get(inputID)
	if !ok {
		return nil, fmt.Errorf("%w: input %s not found", ErrNotFound, inputID)
	}

	return inp.toInput(), nil
}

// UpdateInput updates an input's mutable fields. sdiSourcesSet distinguishes
// "caller didn't mention sdiSources" from "caller sent an explicit []":
// like name and roleArn above, an absent field must leave the existing
// value alone -- AWS's doc comment doesn't say so, but silently wiping
// SdiSources on a rename would be a bug regardless of what the doc omits.
func (b *InMemoryBackend) UpdateInput(
	inputID, name, roleArn string,
	sdiSources []string,
	sdiSourcesSet bool,
) (*Input, error) {
	b.mu.Lock("UpdateInput")
	defer b.mu.Unlock()

	inp, ok := b.inputs.Get(inputID)
	if !ok {
		return nil, fmt.Errorf("%w: input %s not found", ErrNotFound, inputID)
	}

	if name != "" {
		inp.Name = name
	}

	if roleArn != "" {
		inp.RoleARN = roleArn
	}

	if sdiSourcesSet {
		sources := make([]string, len(sdiSources))
		copy(sources, sdiSources)
		inp.SdiSources = sources
	}

	return inp.toInput(), nil
}

// DeleteInput marks an input DELETED rather than removing it outright.
// terraform-provider-aws's waitInputDeleted (internal/service/medialive/
// input.go) polls DescribeInput waiting for State to reach InputStateDeleted;
// its refresh function treats a NotFoundException as "not yet observed", not
// as success -- Target is InputStateDeleted, never the empty state NotFound
// maps to. Deleting the record outright made DescribeInput 404 immediately,
// which the waiter cannot distinguish from "still deleting", so it burned its
// NotFoundChecks budget and failed with "couldn't find resource".
func (b *InMemoryBackend) DeleteInput(inputID string) error {
	b.mu.Lock("DeleteInput")
	defer b.mu.Unlock()

	inp, ok := b.inputs.Get(inputID)
	if !ok {
		return fmt.Errorf("%w: input %s not found", ErrNotFound, inputID)
	}

	inp.State = stateDeleted

	return nil
}

// ListInputs returns a paginated list of inputs.
func (b *InMemoryBackend) ListInputs(
	maxResults int,
	nextToken string,
) ([]*InputSummary, string, error) {
	b.mu.RLock("ListInputs")
	defer b.mu.RUnlock()

	all := b.inputs.All()

	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })

	pg := page.New(all, nextToken, maxResults, defaultMaxResults)

	summaries := make([]*InputSummary, 0, len(pg.Data))
	for _, inp := range pg.Data {
		summaries = append(summaries, inp.toSummary())
	}

	return summaries, pg.Next, nil
}

// --- Partner inputs ---

// CreatePartnerInput creates a partner input linked to an existing input.
func (b *InMemoryBackend) CreatePartnerInput(
	inputID string,
	tags map[string]string,
) (*Input, error) {
	b.mu.Lock("CreatePartnerInput")
	defer b.mu.Unlock()

	parent, ok := b.inputs.Get(inputID)
	if !ok {
		return nil, fmt.Errorf("%w: input %s not found", ErrNotFound, inputID)
	}

	id := newID()
	partner := &storedInput{
		Tags:      copyTags(tags),
		ARN:       b.inputARN(id),
		ID:        id,
		Name:      parent.Name,
		InputType: parent.InputType,
		RoleARN:   parent.RoleARN,
		State:     stateDetached,
	}
	b.inputs.Put(partner)

	return partner.toInput(), nil
}

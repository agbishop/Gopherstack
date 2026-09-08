package pipes

import (
	"context"
	"encoding/base64"
	"fmt"
	"slices"
	"strings"

	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

func (b *InMemoryBackend) ListPipes(ctx context.Context, f ListPipesFilter) (ListPipesResult, error) {
	b.mu.RLock("ListPipes")
	defer b.mu.RUnlock()

	pipesTable := b.pipesTable(getRegion(ctx, b.region))

	limit := f.Limit
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}

	names := sortedPipeNames(pipesTable)

	startIdx, err := resolveStartIndex(names, f.NextToken)
	if err != nil {
		return ListPipesResult{}, err
	}

	result, lastIncluded := collectMatchingPipes(pipesTable, names, startIdx, limit, f)
	nextToken := buildNextToken(pipesTable, names, startIdx, len(result), limit, lastIncluded, f)

	return ListPipesResult{Pipes: result, NextToken: nextToken}, nil
}

// allRunningPipes returns all RUNNING pipes across every region. Used by the
// background runner which must poll all regions without a request context.
func (b *InMemoryBackend) allRunningPipes() []*Pipe {
	b.mu.RLock("allRunningPipes")
	defer b.mu.RUnlock()

	var result []*Pipe

	for _, regionTable := range b.pipes {
		for _, p := range regionTable.All() {
			if p.CurrentState == stateRunning {
				result = append(result, clonePipe(p))
			}
		}
	}

	return result
}

func sortedPipeNames(pipesTable *store.Table[Pipe]) []string {
	names := make([]string, 0, pipesTable.Len())
	for _, p := range pipesTable.All() {
		names = append(names, p.Name)
	}
	slices.Sort(names)

	return names
}

func resolveStartIndex(names []string, nextToken string) (int, error) {
	if nextToken == "" {
		return 0, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(nextToken)
	if err != nil {
		return 0, fmt.Errorf("%w: invalid NextToken", ErrValidation)
	}
	cursor := strings.TrimSuffix(string(decoded), nextTokenSep)
	startIdx := len(names)
	for i, n := range names {
		if n > cursor {
			startIdx = i

			break
		}
	}

	return startIdx, nil
}

func collectMatchingPipes(
	pipesTable *store.Table[Pipe],
	names []string, startIdx, limit int, f ListPipesFilter,
) ([]*Pipe, string) {
	var result []*Pipe
	var lastIncluded string
	for i := startIdx; i < len(names); i++ {
		if len(result) >= limit {
			break
		}
		p, _ := pipesTable.Get(names[i])
		if !matchesFilter(p, f) {
			continue
		}
		result = append(result, clonePipe(p))
		lastIncluded = p.Name
	}

	return result, lastIncluded
}

func buildNextToken(
	pipesTable *store.Table[Pipe],
	names []string, startIdx, resultLen, limit int, lastIncluded string, f ListPipesFilter,
) string {
	if resultLen < limit || lastIncluded == "" {
		return ""
	}
	for i := startIdx + resultLen; i < len(names); i++ {
		p, _ := pipesTable.Get(names[i])
		if matchesFilter(p, f) {
			return base64.StdEncoding.EncodeToString([]byte(lastIncluded + nextTokenSep))
		}
	}

	return ""
}

func matchesFilter(p *Pipe, f ListPipesFilter) bool {
	if f.NamePrefix != "" && !strings.HasPrefix(p.Name, f.NamePrefix) {
		return false
	}
	if f.DesiredState != "" && p.DesiredState != f.DesiredState {
		return false
	}
	if f.CurrentState != "" && p.CurrentState != f.CurrentState {
		return false
	}
	if f.SourcePrefix != "" && !strings.HasPrefix(p.Source, f.SourcePrefix) {
		return false
	}
	if f.TargetPrefix != "" && !strings.HasPrefix(p.Target, f.TargetPrefix) {
		return false
	}

	return true
}

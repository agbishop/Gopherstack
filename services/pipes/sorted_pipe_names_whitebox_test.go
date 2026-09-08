package pipes

import (
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

func TestSortedPipeNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		names []string
		want  []string
	}{
		{name: "empty", names: nil, want: []string{}},
		{name: "single", names: []string{"a"}, want: []string{"a"}},
		{name: "already_sorted", names: []string{"a", "b", "c"}, want: []string{"a", "b", "c"}},
		{name: "reverse_sorted", names: []string{"c", "b", "a"}, want: []string{"a", "b", "c"}},
		{
			name:  "mixed",
			names: []string{"pipe-b", "pipe-a", "pipe-c", "pipe-1", "pipe-A"},
			want:  []string{"pipe-1", "pipe-A", "pipe-a", "pipe-b", "pipe-c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			table := store.New(pipeKeyFn)
			for i, n := range tt.names {
				table.Put(
					&Pipe{
						Name: n,
						ARN:  fmt.Sprintf("arn:aws:pipes:us-west-2:123456789012:pipe/%s-%d", n, i),
					},
				)
			}

			got := sortedPipeNames(table)
			if len(got) != len(tt.want) {
				t.Fatalf("sortedPipeNames() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("sortedPipeNames() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

// TestSortedPipeNames_NoDuplicates documents that pipesTable is keyed by Name,
// so re-Put of an existing name overwrites rather than adding an entry:
// sortedPipeNames can never actually see duplicate names.
func TestSortedPipeNames_NoDuplicates(t *testing.T) {
	t.Parallel()

	table := store.New(pipeKeyFn)
	table.Put(&Pipe{Name: "dup", ARN: "arn:aws:pipes:us-west-2:123456789012:pipe/dup-1"})
	table.Put(&Pipe{Name: "dup", ARN: "arn:aws:pipes:us-west-2:123456789012:pipe/dup-2"})

	got := sortedPipeNames(table)
	if len(got) != 1 {
		t.Fatalf("sortedPipeNames() = %v, want a single deduplicated entry", got)
	}
}

func BenchmarkSortedPipeNames(b *testing.B) {
	const n = 10000

	table := store.New(pipeKeyFn)
	for range n {
		name := fmt.Sprintf("pipe-%08d", rand.IntN(n*10))
		table.Put(&Pipe{Name: name, ARN: "arn:aws:pipes:us-west-2:123456789012:pipe/" + name})
	}

	b.ResetTimer()
	for range b.N {
		sortedPipeNames(table)
	}
}

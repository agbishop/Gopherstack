package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const parityFileName = "PARITY.md"

// topLevelKeyRe matches a frontmatter key at column 0, e.g. "gaps:" --
// mirrors cmd/gendocs/parser.go's topLevelKeyRe (same file shape, this tool
// only needs raw block extents, not full structured values).
var topLevelKeyRe = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*):(.*)$`)

// manifest is one services/<svc>/PARITY.md, split into the regions this
// tool's checks operate over.
type manifest struct {
	service string
	path    string
	lines   []string     // 0-indexed, whole file
	claims  []claimBlock // gaps:/items_still_open:/residual_gaps: blocks found
	fmOther []lineRange  // frontmatter segments NOT inside any claim block
	body    lineRange    // lines[frontEnd:], i.e. the whole body

	// frontEnd is the 0-indexed line just past the frontmatter (the closing
	// "---", a "## " heading, or len(lines) if neither exists) -- mirrors
	// cmd/gendocs/parser.go's extractFrontmatter tolerance.
	frontStart, frontEnd int
}

// claimBlock is one claimFields block: its field name and the line range
// (within the whole file) its list items span.
type claimBlock struct {
	field string
	start int // inclusive, 0-indexed
	end   int // exclusive
}

// lineRange is a contiguous, half-open span of 0-indexed file lines. Kept
// contiguous (rather than a flat index list) so a window search can look at
// real surrounding context instead of stitching together lines from
// unrelated parts of the file.
type lineRange struct {
	start, end int // [start, end)
}

func discoverManifests(servicesDir string) ([]manifest, error) {
	entries, err := os.ReadDir(servicesDir)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", servicesDir, err)
	}

	var slugs []string
	for _, e := range entries {
		if e.IsDir() {
			slugs = append(slugs, e.Name())
		}
	}
	sort.Strings(slugs)

	manifests := make([]manifest, 0, len(slugs))
	for _, slug := range slugs {
		path := filepath.Join(servicesDir, slug, parityFileName)

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				continue
			}

			return nil, fmt.Errorf("read %s: %w", path, readErr)
		}

		manifests = append(manifests, parseManifest(slug, path, string(data)))
	}

	return manifests, nil
}

func parseManifest(service, path, content string) manifest {
	lines := strings.Split(content, "\n")
	frontStart, frontEnd := extractFrontmatterRange(lines)

	m := manifest{service: service, path: path, lines: lines, frontStart: frontStart, frontEnd: frontEnd,
		claims: findClaimBlocks(lines, frontStart, frontEnd)}
	m.fmOther = complementRanges(frontStart, frontEnd, m.claims)
	m.body = lineRange{start: frontEnd, end: len(lines)}

	return m
}

// complementRanges returns the segments of [start, end) not covered by any
// of blocks' [start, end) spans, preserving order.
func complementRanges(start, end int, blocks []claimBlock) []lineRange {
	cursor := start

	var out []lineRange
	for _, b := range blocks {
		if b.start > cursor {
			out = append(out, lineRange{start: cursor, end: b.start})
		}
		if b.end > cursor {
			cursor = b.end
		}
	}
	if cursor < end {
		out = append(out, lineRange{start: cursor, end: end})
	}

	return out
}

// extractFrontmatterRange finds [start, end) of the frontmatter block,
// tolerating a missing opening/closing "---" the same way
// cmd/gendocs/parser.go's extractFrontmatter does.
func extractFrontmatterRange(lines []string) (int, int) {
	start := 0
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		start = 1
	}

	for i := start; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if t == "---" || strings.HasPrefix(t, "## ") {
			return start, i
		}
	}

	return start, len(lines)
}

// findClaimBlocks locates each claimFields block within lines[frontStart:frontEnd]:
// from the field's own key line up to (not including) the next column-0 key
// line, or frontEnd.
func findClaimBlocks(lines []string, frontStart, frontEnd int) []claimBlock {
	var blocks []claimBlock

	for i := frontStart; i < frontEnd; i++ {
		m := topLevelKeyRe.FindStringSubmatch(lines[i])
		if m == nil || !isClaimField(m[1]) {
			continue
		}

		end := frontEnd
		for j := i + 1; j < frontEnd; j++ {
			if km := topLevelKeyRe.FindStringSubmatch(lines[j]); km != nil {
				end = j

				break
			}
		}

		blocks = append(blocks, claimBlock{field: m[1], start: i, end: end})
	}

	return blocks
}

// isClaimField reports whether key is a front-matter list field this tool
// treats as an open/not-fixed claim. structural_gaps is deliberately
// excluded: by definition (services/_PARITY_TEMPLATE.md) it names
// divergences that can NEVER be fixed, so a later section describing one as
// fixed would itself be the anomaly worth a human's attention, not evidence
// the claim is stale -- out of scope for this detector. deferred is excluded
// too: "consciously not audited this pass" is a scope statement, not a
// fix-status claim.
func isClaimField(key string) bool {
	switch key {
	case "gaps", "items_still_open", "residual_gaps":
		return true
	default:
		return false
	}
}

// blockEmpty reports whether a claim block carries no real content -- just
// its "field: []" / "field:" key line, or a key line plus only blank/comment
// continuation lines.
func (m manifest) blockEmpty(c claimBlock) bool {
	for i := c.start; i < c.end; i++ {
		line := m.lines[i]
		if i == c.start {
			_, rest, _ := strings.Cut(line, ":")
			rest = strings.TrimSpace(rest)
			// Strip a trailing " #comment" the same way the template
			// documents every field ("gaps: # known divergences..."), even
			// on an otherwise-empty "gaps: []" line -- without this, a
			// genuinely empty block reads as non-empty and its documentation
			// comment gets scanned as if it were real claim text (confirmed
			// live: services/codebuild/PARITY.md's "gaps: [] # known
			// divergences NOT fixed..." line).
			if before, _, found := strings.Cut(rest, " #"); found {
				rest = strings.TrimSpace(before)
			}
			if rest != "" && rest != "[]" {
				return false
			}

			continue
		}

		t := strings.TrimSpace(line)
		if t != "" && !strings.HasPrefix(t, "#") {
			return false
		}
	}

	return true
}

// linesIn returns m.lines[r.start:r.end], the physical lines of r.
func (m manifest) linesIn(r lineRange) []string {
	return m.lines[r.start:r.end]
}

// fullText joins the whole file, for commonTokenMax's file-wide frequency
// check.
func (m manifest) fullText() string {
	return strings.Join(m.lines, "\n")
}

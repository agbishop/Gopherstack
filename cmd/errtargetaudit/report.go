package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// lowResolutionThreshold gates the implausible-resolution guard --
// cmd/reqfielddiff/cmd/reqfieldscan's identical discipline, and this
// package's doc comment explains why it is not optional -- if a service
// emits 200 error codes and this scan resolves 3 to operations, that is a
// bug in this tool, not a finding about the service.
const lowResolutionThreshold = 0.5

// minOpsForResolutionGuard avoids firing the guard on a tiny service where a
// low ratio is noise at small N.
const minOpsForResolutionGuard = 5

func coverageWarnings(sr serviceScan) []string {
	warnings := untraceableModuleWarnings(sr)
	warnings = append(warnings, mixedGovernanceWarnings(sr)...)
	warnings = append(warnings, sparseModuleWarnings(sr)...)

	if sr.OpsGroundTruth == 0 {
		return warnings
	}

	if sr.OpsResolved == 0 {
		warnings = append(warnings, fmt.Sprintf(
			"ZERO of %d operations with SDK ground truth resolved to an emulator handler at all -- "+
				"treat this service as UNSCANNED, not clean; this scan likely doesn't recognise its "+
				"dispatch or naming convention", sr.OpsGroundTruth))

		return warnings
	}

	if sr.OpsGroundTruth >= minOpsForResolutionGuard {
		ratio := float64(sr.OpsResolved) / float64(sr.OpsGroundTruth)
		if ratio < lowResolutionThreshold {
			warnings = append(warnings, fmt.Sprintf(
				"only %d/%d (%.0f%%) of operations with SDK ground truth (borrowed-module ops already "+
					"excluded) resolved to a handler -- treat this service's coverage as UNVERIFIED, not "+
					"clean; this tool cannot tell whether that's a resolution gap in itself or a genuine "+
					"gap in this service's own implementation",
				sr.OpsResolved, sr.OpsGroundTruth, pct(sr.OpsResolved, sr.OpsGroundTruth)))
		}
	}

	warnings = append(warnings, emissionCoverageWarnings(sr)...)

	return warnings
}

// untraceableModuleWarnings flags gopherstack-zkpi's outcome: a resolved
// module (ModulesNoOpFuncs) defines real, service-wide error types but has
// no per-operation ground truth at all, because its pinned SDK version
// generates no deserializers.go file (a newer smithy schema-based codegen --
// confirmed live for 11 modules as of gopherstack-84mn, including 8 whose
// service dir's OpsGroundTruth is truly zero: acm, amplify, codedeploy,
// codepipeline, route53resolver, sqs, transcribe, workspaces). Fired
// UNCONDITIONALLY, ahead of the OpsGroundTruth==0
// short-circuit below, so this service is never silently indistinguishable
// from "nothing to audit" -- the same "zero reads as clean" failure
// emissionCoverageWarnings' BLIND case already exists to catch, for a
// different mechanism.
func untraceableModuleWarnings(sr serviceScan) []string {
	warnings := make([]string, 0, len(sr.ModulesNoOpFuncs))

	for _, mod := range sr.ModulesNoOpFuncs {
		warnings = append(warnings, fmt.Sprintf(
			"module %q defines real error types but NO per-operation deserializeOpError<Op>-shaped "+
				"function was found for ANY operation (likely a newer SDK codegen with no deserializers.go "+
				"file at all) -- treat this service as UNTRACEABLE, not clean; this tool has no "+
				"per-operation ground truth for it, so a report of zero class A findings below proves nothing",
			mod))
	}

	return warnings
}

// mixedGovernanceWarnings is gopherstack-f3ql's crux, recorded where the
// reasoning is actually applied. A module in ModulesNoOpFuncs contributes NO
// entries to unionOpFuncs (deser.go/scan.go), so it structurally can never
// be the module a class A finding is checked against -- scanOneOp's
// `!mgt.OpFuncs[op]` guard (scan.go) sees it as empty for every op. So when
// such a service ALSO has class A findings, every one of them was built
// against a DIFFERENT, co-resolved classic-codegen module (moduleassign.go's
// bestOverlapModule assigns a whole domain -- a receiver type -- to one
// module by total op-name overlap, never per operation).
//
// That co-resolved module genuinely governs wire-level error resolution
// when the finding's op is uniquely its own: confirmed live for eventbridge
// (SearchSchemas, governed by the classic-codegen "schemas" module -- the
// real, separate AWS Schemas API) and iot (UpdateThingShadow, governed by
// "iotdataplane" -- the real, separate IoT Data Plane API). Those two
// findings are exactly as trustworthy as any class A finding elsewhere in
// this corpus, and must not be suppressed -- suppression would delete real,
// verified parity information.
//
// But moduleassign.go's domain-wide (not per-operation) assignment cannot
// tell that apart from a shared/generic op name the SCHEMA-BASED module's
// own real API also exposes: eventbridge's own TagResource, UntagResource
// and ListTagsForResource are real EventBridge operations too (confirmed:
// aws-sdk-go-v2/service/eventbridge's own Client has all three), served by
// the SAME shared handler (services/eventbridge/handler_tags.go's
// tagActions(), on *Handler, the same domain SearchSchemas resolves under).
// A future finding on one of those op names would be checked only against
// schemas' declared set, even for an invocation reached via EventBridge's
// own real wire route -- where the actual governing resolution is
// eventbridge's own whole-service TypeRegistry (smithy-go transport/http/
// protocol/awsjson), which matches by wire code regardless of declaring
// operation and would accept a code this scan calls "undeclared" without
// complaint. So for that shape, "op emits a code it does not declare" is a
// documentation divergence against the co-resolved module's model, not a
// confirmed client-breaking defect -- this scan cannot yet tell the two
// shapes apart per finding, so the whole section is labelled, not filtered.
func mixedGovernanceWarnings(sr serviceScan) []string {
	if len(sr.ModulesNoOpFuncs) == 0 || len(sr.Findings) == 0 {
		return nil
	}

	return []string{fmt.Sprintf(
		"module(s) %s are schema-based with no per-operation ground truth of their own (see the UNTRACEABLE "+
			"warning above) -- every class A finding below is therefore built against a DIFFERENT, co-resolved "+
			"module's declared sets, not %s's. Trustworthy where the finding's op is uniquely that co-resolved "+
			"module's own. But where the op name is one %s's OWN real API also exposes (a shared/generic "+
			"handler serving both), treat the finding as a DOCUMENTATION DIVERGENCE, not a confirmed "+
			"client-breaking defect: %s's real error resolution is a whole-service TypeRegistry match by wire "+
			"code, not scoped to the declaring operation, so it may legitimately accept a code this scan calls "+
			"undeclared",
		moduleList(sr.ModulesNoOpFuncs), moduleList(sr.ModulesNoOpFuncs),
		moduleList(sr.ModulesNoOpFuncs), moduleList(sr.ModulesNoOpFuncs))}
}

// sparseModuleWarnings flags a resolved module whose own deserializer
// matched a code for under half its OpFuncs (deser.go's sparselyModeled) --
// gopherstack-zofv's OrphanFindings is unconditionally suppressed for such
// a module (scan.go), so this warning is the only visible trace that some
// "declared nowhere in this service" codes may have gone unchecked here,
// not confirmed absent.
func sparseModuleWarnings(sr serviceScan) []string {
	warnings := make([]string, 0, len(sr.ModulesSparse))

	for _, mod := range sr.ModulesSparse {
		warnings = append(warnings, fmt.Sprintf(
			"module %q matched a declared code for under half its own deserializeOpError<Op> functions "+
				"(s3-class sparse modeling) -- orphan-code findings are suppressed for its operations, "+
				"since this tool cannot tell a genuine gap apart from a code this SDK version's "+
				"deserializer simply never modeled", mod))
	}

	return warnings
}

// emissionCoverageWarnings guards the other half of this tool's own
// reliability: a service can resolve every operation to a handler (the
// guard above stays quiet) while this scan's emission mechanisms still
// don't recognise its error-mapping shape at all -- kms and sqs's
// data-driven tables were exactly this, reported as a silent "zero class A
// findings" (a clean bill of health) rather than a loud one, until this
// guard existed. Mirrors the resolution guard above in shape: an
// unconditional check at exactly zero (unambiguous regardless of N), and a
// ratio check at N >= minOpsForResolutionGuard for the partial case
// (services/s3's own append-composed table, still only reaching 19/109
// resolved ops through mechanisms this scan does understand).
func emissionCoverageWarnings(sr serviceScan) []string {
	if sr.OpsResolved == 0 {
		return nil
	}

	if sr.OpsWithEmission == 0 {
		return []string{fmt.Sprintf(
			"%d operations resolved to a handler but ZERO emissions were found for any of them -- "+
				"treat this service as BLIND, not clean; this scan's emission mechanisms likely don't "+
				"recognise this service's own error-mapping shape (a report of zero class A findings "+
				"below is not a clean bill of health)", sr.OpsResolved)}
	}

	if sr.OpsResolved >= minOpsForResolutionGuard {
		ratio := float64(sr.OpsWithEmission) / float64(sr.OpsResolved)
		if ratio < lowResolutionThreshold {
			return []string{fmt.Sprintf(
				"only %d/%d (%.0f%%) of resolved operations had ANY emission found -- treat this "+
					"service as MOSTLY BLIND, not clean; likely a partial emission-mechanism gap in "+
					"this tool (services/s3's own append-composed error table is a confirmed instance), "+
					"not a service this thin on error paths",
				sr.OpsWithEmission, sr.OpsResolved, pct(sr.OpsWithEmission, sr.OpsResolved))}
		}
	}

	return nil
}

func pct(n, total int) float64 {
	if total == 0 {
		return 0
	}

	const percent = 100

	return float64(n) / float64(total) * percent
}

func writeJSON(path string, scans []serviceScan) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")

	return enc.Encode(scans)
}

func printServiceScan(sr serviceScan) {
	if len(sr.Findings) == 0 && len(sr.OrphanFindings) == 0 && len(sr.Warnings) == 0 {
		return
	}

	fmt.Fprintf(os.Stdout, "## %s (%s)\n", sr.Dir, moduleList(sr.Modules))

	for _, w := range sr.Warnings {
		fmt.Fprintf(os.Stdout, "*** COVERAGE WARNING: %s ***\n", w)
	}

	fmt.Fprintf(os.Stdout, "operations with SDK ground truth: %d, resolved: %d, with an emission found: %d\n",
		sr.OpsGroundTruth, sr.OpsResolved, sr.OpsWithEmission)

	if sr.OpsGroundTruthBorrowed > 0 {
		fmt.Fprintf(os.Stdout, "  (%d further ops belong to an imported SDK module never assigned to a "+
			"resolved handler here -- excluded from ground truth as borrowed-type-only)\n", sr.OpsGroundTruthBorrowed)
	}

	if len(sr.Findings) == 0 {
		fmt.Fprintln(os.Stdout, "no class A findings (real code, wrong operation)")
	} else {
		fmt.Fprintf(os.Stdout, "class A findings (%d), grouped by emission site:\n", len(sr.Findings))
		printSiteGroups(sr.Findings, sr.OpsResolved)
	}

	printOrphanFindings(sr.OrphanFindings, sr.OpsResolved)

	fmt.Fprintln(os.Stdout)
}

// printOrphanFindings reports gopherstack-zofv's class: a code declared by
// no operation anywhere in this service's resolved module(s), not just the
// emitting operation's own set. Silent when empty -- most services have
// none, and the empty case is already implied by this tool's own doc
// comment, not worth a line every run.
func printOrphanFindings(findings []finding, opsResolved int) {
	if len(findings) == 0 {
		return
	}

	fmt.Fprintf(
		os.Stdout,
		"orphan-code findings (%d, declared by NO operation anywhere in this service's SDK), grouped by emission site:\n",
		len(findings),
	)
	printSiteGroups(findings, opsResolved)
}

// siteGroup collapses a section's findings by the actual emission SITE
// (file:line, plus code since two different codes can share a line in a
// switch) rather than the (op, code) pair a finding is keyed by --
// gopherstack-2evc: mgn's 122 class A findings are 90 ops funneled through
// one line and 32 through another, and a reader could not tell that apart
// from 122 independent defects without doing this collapse by hand, which
// is exactly what gopherstack-mq6m had to do manually.
type siteGroup struct {
	File      string
	Code      string
	Mechanism string
	// EnclosingFunc mirrors evidenceSite.EnclosingFunc -- invariant per site
	// (a file:line's enclosing function never changes finding to finding).
	EnclosingFunc string
	Ops           []string
	AcceptedBy    []string
	Line          int
}

type siteKey struct {
	File string
	Code string
	Line int
}

// siteAccum is groupFindingsBySite's working state per site -- Ops as a set
// while findings are still being folded in, since the same op can reach one
// site via more than one finding path (rare, but AcceptedBy's own per-code
// invariance below assumes nothing about it).
type siteAccum struct {
	ops           map[string]bool
	mechanism     string
	enclosingFunc string
	acceptedBy    []string
}

// groupFindingsBySite is the collapse gopherstack-2evc asks for. AcceptedBy
// is siblingsAccepting's result (helpers.go): fixed by (module, code) alone,
// never by op, so every finding folded into the same site+code carries an
// identical list -- taking the first one is exact, not an approximation.
func groupFindingsBySite(findings []finding) []siteGroup {
	groups := map[siteKey]*siteAccum{}

	for _, f := range findings {
		for _, s := range f.Sites {
			key := siteKey{File: s.File, Line: s.Line, Code: f.Code}

			g, ok := groups[key]
			if !ok {
				g = &siteAccum{
					mechanism:     s.Mechanism,
					enclosingFunc: s.EnclosingFunc,
					acceptedBy:    f.AcceptedBy,
					ops:           map[string]bool{},
				}
				groups[key] = g
			}

			g.ops[f.Op] = true
		}
	}

	return finalizeSiteGroups(groups)
}

func finalizeSiteGroups(groups map[siteKey]*siteAccum) []siteGroup {
	out := make([]siteGroup, 0, len(groups))

	for key, g := range groups {
		ops := make([]string, 0, len(g.ops))
		for op := range g.ops {
			ops = append(ops, op)
		}

		sort.Strings(ops)

		out = append(out, siteGroup{
			File:          key.File,
			Line:          key.Line,
			Code:          key.Code,
			Mechanism:     g.mechanism,
			EnclosingFunc: g.enclosingFunc,
			Ops:           ops,
			AcceptedBy:    g.acceptedBy,
		})
	}

	// Most ops behind a site first -- a funnel point (mgn's 90/95) belongs
	// at the top of the section, not buried alphabetically among singletons.
	sort.Slice(out, func(i, j int) bool {
		if len(out[i].Ops) != len(out[j].Ops) {
			return len(out[i].Ops) > len(out[j].Ops)
		}

		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}

		return out[i].Line < out[j].Line
	})

	return out
}

// sharedPlumbingRatio and sharedPlumbingMinOps decide when a site is loudly
// tagged shared plumbing instead of leaving the reader to notice the op
// count unaided. Corpus check (2026-09-07, all 160 services via -json,
// every class A/orphan finding's sites grouped the way groupFindingsBySite
// does here): of 354 resulting site groups, only mgn's two sites that
// gopherstack-mq6m already confirmed generic exceed 0.25 --
// marshalResponse's internalServerError at 90/95=0.947 and
// decodeJSONBody's validationError at 32/95=0.337. Every other group falls
// at or under 0.162 (cloudfront's own quantity_validation.go:56, a real but
// narrower-scoped shared validator, 27/167). The threshold sits inside that
// empty 0.162-0.337 gap, so it fires exactly on the two confirmed cases and
// nothing else in the corpus available to check it against.
const sharedPlumbingRatio = 0.25

// sharedPlumbingMinOps guards the ratio at small N the same way
// minOpsForResolutionGuard does above: 1-of-3 ops (0.33) on a tiny service
// is noise, not a funnel point.
const sharedPlumbingMinOps = minOpsForResolutionGuard

func isSharedPlumbing(nOps, opsResolved int) bool {
	if opsResolved == 0 || nOps < sharedPlumbingMinOps {
		return false
	}

	return float64(nOps)/float64(opsResolved) >= sharedPlumbingRatio
}

// ctorMechanismPrefix marks a call-site emission resolved through
// classifiers.go's constructor-classifier table, e.g. "constructor
// classifier: validateQuantities" -- ctorName strips it to bare fn.
const ctorMechanismPrefix = "constructor classifier: "

func ctorName(mechanism string) (string, bool) {
	if !strings.HasPrefix(mechanism, ctorMechanismPrefix) {
		return "", false
	}

	return mechanism[len(ctorMechanismPrefix):], true
}

// ctorUnions maps (code, fn) to the union of ops across every
// "constructor classifier: fn" row in groups sharing that code -- the
// population a same-fn "sentinel reference" row (deep in fn's OWN body, one
// row per fn regardless of caller count) is checked against below.
func ctorUnions(groups []siteGroup) map[[2]string]map[string]bool {
	out := map[[2]string]map[string]bool{}

	for _, g := range groups {
		fn, ok := ctorName(g.Mechanism)
		if !ok {
			continue
		}

		key := [2]string{g.Code, fn}

		u, exists := out[key]
		if !exists {
			u = map[string]bool{}
			out[key] = u
		}

		for _, op := range g.Ops {
			u[op] = true
		}
	}

	return out
}

// rollupTag reports gopherstack-s0dw's duplication: g is a "sentinel
// reference" site sitting inside a constructor's OWN body, reached at hop 1
// from every op that calls it at hop 0 -- so g's op set can only ever be a
// SUBSET of the union of that same constructor's own "constructor
// classifier" call-site rows (an op reaches g only by having made that call
// first). Measured across the full corpus (2026-09-07): 18 (service, code,
// fn) tuples show both mechanisms for the same fn, every one a subset --
// exact in 15, a proper subset in 3 (a caller reaching the constructor only
// at hop 1 itself, past this scan's one-hop recursion budget, so its own
// call-site row never appears) -- never the reverse, never a partial
// overlap neither side contains. A g outside that shape (no matching
// "constructor classifier" rows for its own fn, or an op present that no
// such row has -- structurally impossible per the above, checked anyway
// rather than assumed) gets no tag.
func rollupTag(g siteGroup, unions map[[2]string]map[string]bool) string {
	if g.Mechanism != "sentinel reference" || g.EnclosingFunc == "" {
		return ""
	}

	union, ok := unions[[2]string{g.Code, g.EnclosingFunc}]
	if !ok || len(union) == 0 {
		return ""
	}

	for _, op := range g.Ops {
		if !union[op] {
			return ""
		}
	}

	if len(g.Ops) == len(union) {
		return fmt.Sprintf(
			"  -- ROLLUP: same ops as %s's own call-site row(s) elsewhere in this list -- do not add to totals",
			g.EnclosingFunc,
		)
	}

	return fmt.Sprintf(
		"  -- PARTIAL ROLLUP: a subset of %s's own call-site row(s) elsewhere in this list -- do not add to totals",
		g.EnclosingFunc,
	)
}

// printSiteGroups prints one line per emission site: the site, the code, how
// many of the service's resolved ops route through it, the emission
// mechanism (composite literal field vs sentinel reference is gopherstack-
// 2evc's other ask -- six of the ten residual orphan findings this campaign
// closed with are the composite-literal false-positive shape, so surfacing
// it here rather than only in JSON is load-bearing), and a shared-plumbing
// tag when isSharedPlumbing says so. The op list is printed in full, never
// truncated: this campaign's set-diff regression guard extracts (op, code,
// site) triples mechanically from this output, and an elided op list would
// make that extraction lossy.
func printSiteGroups(findings []finding, opsResolved int) {
	const percentScale = 100

	groups := groupFindingsBySite(findings)
	unions := ctorUnions(groups)

	for _, g := range groups {
		n := len(g.Ops)

		tag := ""
		if isSharedPlumbing(n, opsResolved) {
			tag = fmt.Sprintf("  -- SHARED PLUMBING (>=%.0f%% of resolved ops)", sharedPlumbingRatio*percentScale)
		}

		tag += rollupTag(g, unions)

		fmt.Fprintf(os.Stdout, "  %s:%d  %s  %d/%d ops  [%s]%s\n",
			g.File, g.Line, g.Code, n, opsResolved, g.Mechanism, tag)
		fmt.Fprintf(os.Stdout, "    %s\n", strings.Join(g.Ops, ", "))

		if len(g.AcceptedBy) > 0 {
			fmt.Fprintf(os.Stdout, "    declared correctly by: %v\n", g.AcceptedBy)
		}
	}
}

func moduleList(mods []string) string {
	return strings.Join(mods, ",")
}

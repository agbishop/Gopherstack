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
// confirmed live only for cloudwatch's own module, out of every module this
// repo's go.mod pins). Fired UNCONDITIONALLY, ahead of the OpsGroundTruth==0
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
		fmt.Fprintf(os.Stdout, "class A findings (%d):\n", len(sr.Findings))
		printCauseGroups(sr.Findings)

		for _, f := range sr.Findings {
			printFinding(f)
		}
	}

	printOrphanFindings(sr.OrphanFindings)

	fmt.Fprintln(os.Stdout)
}

// printOrphanFindings reports gopherstack-zofv's class: a code declared by
// no operation anywhere in this service's resolved module(s), not just the
// emitting operation's own set. Silent when empty -- most services have
// none, and the empty case is already implied by this tool's own doc
// comment, not worth a line every run.
func printOrphanFindings(findings []finding) {
	if len(findings) == 0 {
		return
	}

	fmt.Fprintf(os.Stdout, "orphan-code findings (%d, declared by NO operation anywhere in this service's SDK):\n",
		len(findings))
	printCauseGroups(findings)

	for _, f := range findings {
		printFinding(f)
	}
}

// causeKey groups findings sharing the same wrongly-emitted code AND the
// same first-site emission mechanism -- the two shared-classifier collisions
// this campaign has actually hit (gopherstack-0yva's 49 same-collision
// findings here, an earlier 33-finding event) each had ONE root, and both
// were obvious only after tracing every finding by hand. Requiring both
// Code and Mechanism to match, not just Code alone, keeps two unrelated
// collisions that happen to emit the same code from being blurred into one
// bucket.
type causeKey struct {
	Code      string
	Mechanism string
}

// printCauseGroups prints a one-line-per-cause summary before the full
// finding list, so a bulk collision (many findings, one root) is visible
// immediately rather than only after reading every finding. Silent when
// every finding already has a distinct cause -- nothing to summarize.
func printCauseGroups(findings []finding) {
	groups := map[causeKey][]finding{}

	for _, f := range findings {
		mech := ""
		if len(f.Sites) > 0 {
			mech = f.Sites[0].Mechanism
		}

		key := causeKey{Code: f.Code, Mechanism: mech}
		groups[key] = append(groups[key], f)
	}

	if len(groups) == len(findings) {
		return
	}

	keys := make([]causeKey, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}

	sort.Slice(keys, func(i, j int) bool {
		if len(groups[keys[i]]) != len(groups[keys[j]]) {
			return len(groups[keys[i]]) > len(groups[keys[j]])
		}

		if keys[i].Code != keys[j].Code {
			return keys[i].Code < keys[j].Code
		}

		return keys[i].Mechanism < keys[j].Mechanism
	})

	fmt.Fprintln(os.Stdout, "  grouped by cause (code + mechanism):")

	for _, k := range keys {
		fs := groups[k]

		ops := make([]string, 0, len(fs))
		for _, f := range fs {
			ops = append(ops, f.Op)
		}

		sort.Strings(ops)
		fmt.Fprintf(os.Stdout, "    %d finding(s): code=%s mechanism=%s ops=%v\n", len(fs), k.Code, k.Mechanism, ops)
	}
}

func printFinding(f finding) {
	domain := f.Domain
	if domain == "" {
		domain = "-"
	}

	fmt.Fprintf(os.Stdout, "  op=%s domain=%s code=%s\n", f.Op, domain, f.Code)

	for _, s := range f.Sites {
		fmt.Fprintf(os.Stdout, "    %s:%d  [%s]\n", s.File, s.Line, s.Mechanism)
	}

	if len(f.AcceptedBy) > 0 {
		fmt.Fprintf(os.Stdout, "    declared correctly by: %v\n", f.AcceptedBy)
	}
}

func moduleList(mods []string) string {
	return strings.Join(mods, ",")
}

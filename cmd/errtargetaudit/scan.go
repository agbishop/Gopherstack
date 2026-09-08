package main

import (
	"go/token"
	"path/filepath"
	"sort"
)

// evidenceSite is one source location contributing to a finding -- a
// service typically has 2-3 (the handler's own override/dispatch call, the
// backend method's sentinel return, sometimes a constructor helper) for the
// SAME underlying bug, which is why findings are grouped by (op, domain,
// code) rather than reported one row per site.
type evidenceSite struct {
	File      string `json:"file"`
	Mechanism string `json:"mechanism"`
	// EnclosingFunc mirrors emission.EnclosingFunc -- "" for a site found in
	// an operation's own hop-0 body, otherwise the hop-1 callee whose body
	// it sits in.
	EnclosingFunc string `json:"enclosingFunc,omitempty"`
	Line          int    `json:"line"`
}

// finding is one class A error-envelope-shape bug candidate: a real,
// correctly-spelled code (present somewhere in this service's own pinned
// SDK) emitted reachable from op, but absent from op's OWN declared set --
// gopherstack-o46l's class, invisible to cmd/errcodeaudit by construction
// (that tool only ever asks "is this code real anywhere", never "is it
// real for THIS operation").
type finding struct {
	Op         string         `json:"op"`
	Domain     string         `json:"domain"`
	Code       string         `json:"code"`
	Sites      []evidenceSite `json:"sites"`
	AcceptedBy []string       `json:"acceptedBy,omitempty"`
}

// serviceScan is one services/<dir>'s full result. OpsGroundTruth counts
// only operations belonging to a module actually assigned to a resolved
// domain (moduleassign.go) -- OpsGroundTruthBorrowed is the rest: ops from a
// module this dir imports (module attribution, modresolve.go) but whose
// handlers this scan never traced to, so gopherstack-2kud's dax/stepfunctions
// artifact (module attributed for shared types only, e.g. dax's dataplane
// importing services/dynamodb) no longer pads the denominator.
type serviceScan struct {
	Dir      string    `json:"dir"`
	Modules  []string  `json:"modules"`
	Findings []finding `json:"findings,omitempty"`
	// OrphanFindings is gopherstack-zofv's class: a code absent from EVERY
	// operation's declared set AND from every resolved module's AllCodes --
	// this service's SDK has no record of it anywhere, not just under the
	// wrong operation. A different bug shape from Findings (class A: right
	// code, wrong operation) -- "not a code this service has at all" -- so
	// reported in its own list rather than folded in. Never populated for
	// an op whose assigned module is ModulesSparse (see that field):
	// without matched ground truth to compare against, "declared nowhere"
	// is not evidence, only this tool's own blind spot.
	OrphanFindings []finding `json:"orphanFindings,omitempty"`
	Warnings       []string  `json:"warnings,omitempty"`
	// ModulesNoOpFuncs is every resolved module (deser.go's smt.Modules)
	// that models real, service-wide error types but zero per-operation
	// ones -- gopherstack-zkpi: cloudwatch's own pinned SDK module has no
	// deserializers.go file at all (a newer smithy schema-based codegen),
	// so it can never contribute an OpFuncs entry no matter how deser.go
	// is taught to parse deserializers.go, because that file does not
	// exist. See report.go's untraceableModuleWarnings. When this service
	// ALSO has class A findings, see report.go's mixedGovernanceWarnings
	// (gopherstack-f3ql): they are necessarily built against a different,
	// co-resolved module -- legitimate where the op is uniquely that
	// module's own, a documentation divergence (not client-breaking) where
	// the op name collides with this module's own real API.
	ModulesNoOpFuncs []string `json:"modulesNoOpFuncs,omitempty"`
	// ModulesSparse is every resolved module whose own deserializer matched
	// a code for under half its OpFuncs (deser.go's sparselyModeled) --
	// cmd/errcodeaudit's own s3-class caveat, reused here to gate
	// OrphanFindings: a module parsed this thinly cannot tell "genuinely
	// declared nowhere" apart from "this SDK version's deserializer just
	// didn't model it", so treating either as a finding would be noise.
	ModulesSparse          []string `json:"modulesSparse,omitempty"`
	OpsGroundTruth         int      `json:"opsGroundTruth"`
	OpsGroundTruthBorrowed int      `json:"opsGroundTruthBorrowed,omitempty"`
	OpsResolved            int      `json:"opsResolved"`
	OpsWithEmission        int      `json:"opsWithEmission"`
}

// scanServiceDir resolves dir's pinned SDK module(s), builds their per-op
// ground truth, resolves every operation to its emulator handler(s), walks
// each for error-code emissions, and reports every one absent from that
// operation's own declared set but present somewhere else in the service.
// A service with no resolvable module, or whose resolved module(s) model no
// per-operation error codes at all, contributes nothing -- not an error,
// same "nothing to check" discipline as cmd/errcodeaudit.
func scanServiceDir(dir, repoRoot, cache string, goModVersions map[string]string) (serviceScan, error) {
	name := filepath.Base(dir)

	mods, err := resolveServiceModules(dir)
	if err != nil {
		return serviceScan{}, err
	}

	if len(mods) == 0 {
		return serviceScan{}, nil
	}

	smt, err := buildServiceModuleTruth(cache, mods, goModVersions)
	if err != nil {
		return serviceScan{}, err
	}

	if len(smt.Modules) == 0 {
		return serviceScan{}, nil
	}

	idx, err := buildPkgIndex(dir)
	if err != nil {
		return serviceScan{}, err
	}

	return scanWithIndex(name, mods, repoRoot, idx, smt), nil
}

// modulesWithoutOpFuncs returns every mod in mods whose ground truth
// resolved into smt.Modules but has zero OpFuncs -- by buildServiceModuleTruth's
// own inclusion rule (deser.go), a module only reaches smt.Modules at all
// when OpFuncs or AllCodes is non-empty, so an OpFuncs-empty entry here
// always has real AllCodes: a module with error types but no per-operation
// ground truth to check them against at all, never a module with nothing.
func modulesWithoutOpFuncs(mods []string, smt *serviceModuleTruth) []string {
	var out []string

	for _, mod := range mods {
		mgt, ok := smt.Modules[mod]
		if !ok || len(mgt.OpFuncs) > 0 {
			continue
		}

		out = append(out, mod)
	}

	return out
}

// modulesSparse returns every mod in mods whose ground truth resolved into
// smt.Modules and is sparselyModeled (deser.go) -- see ModulesSparse's doc
// comment on serviceScan for why OrphanFindings must never fire against one.
func modulesSparse(mods []string, smt *serviceModuleTruth) []string {
	var out []string

	for _, mod := range mods {
		mgt, ok := smt.Modules[mod]
		if !ok || !mgt.sparselyModeled() {
			continue
		}

		out = append(out, mod)
	}

	return out
}

// findingKey groups evidence sites into one finding per (operation, domain,
// code) triple.
type findingKey struct {
	Op, Domain, Code string
}

func scanWithIndex(name string, mods []string, repoRoot string, idx *pkgIndex, smt *serviceModuleTruth) serviceScan {
	opUniverse := unionOpFuncs(smt)
	cls := buildClassifiers(idx, opUniverse)

	resolved := map[string][]opRoot{}
	for op := range opUniverse {
		resolved[op] = resolveOpRoots(op, idx)
	}

	domainModule := assignDomainModules(buildDomainOps(resolved), smt)
	groundTruthOps := assignedGroundTruth(smt, domainModule, opUniverse)

	sr := serviceScan{
		Dir:                    name,
		Modules:                mods,
		OpsGroundTruth:         len(groundTruthOps),
		OpsGroundTruthBorrowed: len(opUniverse) - len(groundTruthOps),
		ModulesNoOpFuncs:       modulesWithoutOpFuncs(mods, smt),
		ModulesSparse:          modulesSparse(mods, smt),
	}

	allCodes := smt.allServiceCodes()
	grouped := map[findingKey]*finding{}
	orphanGrouped := map[findingKey]*finding{}

	for op := range groundTruthOps {
		scanOneOp(op, resolved[op], idx, cls, smt, allCodes, domainModule, repoRoot, &sr, grouped, orphanGrouped)
	}

	sr.Findings = finalizeFindings(grouped)
	sr.OrphanFindings = finalizeFindings(orphanGrouped)
	sr.Warnings = coverageWarnings(sr)

	return sr
}

func finalizeFindings(grouped map[findingKey]*finding) []finding {
	out := make([]finding, 0, len(grouped))

	for _, f := range grouped {
		sort.Slice(f.Sites, func(i, j int) bool {
			if f.Sites[i].File != f.Sites[j].File {
				return f.Sites[i].File < f.Sites[j].File
			}

			return f.Sites[i].Line < f.Sites[j].Line
		})

		out = append(out, *f)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Op != out[j].Op {
			return out[i].Op < out[j].Op
		}

		if out[i].Domain != out[j].Domain {
			return out[i].Domain < out[j].Domain
		}

		return out[i].Code < out[j].Code
	})

	return out
}

func scanOneOp(
	op string,
	roots []opRoot,
	idx *pkgIndex,
	cls *classifiers,
	smt *serviceModuleTruth,
	allCodes map[string]bool,
	domainModule map[string]string,
	repoRoot string,
	sr *serviceScan,
	grouped map[findingKey]*finding,
	orphanGrouped map[findingKey]*finding,
) {
	if len(roots) > 0 {
		sr.OpsResolved++
	}

	emittedAny := false

	for domain, domRoots := range groupRootsByDomain(roots) {
		mod, ok := effectiveModule(domain, domainModule, smt)
		if !ok {
			continue
		}

		mgt := smt.Modules[mod]
		if !mgt.OpFuncs[op] {
			continue
		}

		emissions := walkOpEmissions(domRoots, idx, cls)
		if len(emissions) > 0 {
			emittedAny = true
		}

		declared := mgt.PerOp[op]

		for _, e := range emissions {
			classifyAndAddFinding(op, domain, e, declared, mgt, allCodes, idx.Fset, repoRoot, grouped, orphanGrouped)
		}
	}

	if emittedAny {
		sr.OpsWithEmission++
	}
}

// emissionClass is one emitted code's verdict against its operation's own
// declared set and this service's SDK-wide code universe.
type emissionClass int

const (
	// emissionDeclaredOrGeneric covers both the common case (this op's own
	// declared set has the code) and a protocol-level code every operation
	// may legitimately emit regardless of its own declared set -- neither
	// is a finding of any class.
	emissionDeclaredOrGeneric emissionClass = iota
	// emissionOrphan is gopherstack-zofv's class: absent from EVERY
	// operation's declared set anywhere in this service's resolved
	// module(s), not just this one's -- "not a code this service has at
	// all", distinct from class A below.
	emissionOrphan
	// emissionClassA is real somewhere in this service, absent from this
	// operation's own declared set -- gopherstack-o46l's class.
	emissionClassA
)

// classifyEmission decides e's class purely from set membership: declared[]
// is this operation's own declared set, allCodes is the service-wide union
// used to tell a real-but-misplaced code (class A) apart from one this
// service's SDK never declares anywhere (emissionOrphan). genericProtocolCodes
// is consulted only when allCodes lacks the code too (gopherstack-udkm): a
// module assigned to this service that DOES declare the code somewhere
// (e.g. ValidationException, real per-op in 57 pinned SDK modules) means an
// op emitting it without declaring it is class A, not a protocol freebie.
func classifyEmission(code string, declared, allCodes map[string]bool) emissionClass {
	if declared[code] || wireFaultTypeDiscriminators[code] {
		return emissionDeclaredOrGeneric
	}

	if !allCodes[code] {
		if genericProtocolCodes[code] {
			return emissionDeclaredOrGeneric
		}

		return emissionOrphan
	}

	return emissionClassA
}

// classifyAndAddFinding classifies one emission and, for class A or
// emissionOrphan, records it into the matching grouped map by (op, domain,
// code) rather than appending its own row -- see evidenceSite's doc
// comment. emissionOrphan is dropped, not recorded, in two cases: mgt is
// sparselyModeled (deser.go: without matched ground truth to compare
// against, "declared nowhere" would be this tool's own parsing gap
// mislabeled as a finding, the noise gopherstack-zofv's third test fixture
// exists to rule out), or e.WeakLabel (the ambiguous "Type" composite-field
// mechanism: zero true positives, dozens of false ones -- see emission's
// doc comment).
func classifyAndAddFinding(
	op, domain string,
	e emission,
	declared map[string]bool,
	mgt *moduleGroundTruth,
	allCodes map[string]bool,
	fset *token.FileSet,
	repoRoot string,
	grouped map[findingKey]*finding,
	orphanGrouped map[findingKey]*finding,
) {
	target := grouped

	switch classifyEmission(e.Code, declared, allCodes) {
	case emissionDeclaredOrGeneric:
		return
	case emissionOrphan:
		if mgt.sparselyModeled() || e.WeakLabel {
			return
		}

		target = orphanGrouped
	case emissionClassA:
	}

	pos := fset.Position(e.Pos)

	file, err := filepath.Rel(repoRoot, pos.Filename)
	if err != nil {
		file = pos.Filename
	}

	key := findingKey{Op: op, Domain: domain, Code: e.Code}

	f, ok := target[key]
	if !ok {
		f = &finding{Op: op, Domain: domain, Code: e.Code, AcceptedBy: siblingsAccepting(mgt, op, e.Code)}
		target[key] = f
	}

	f.Sites = append(f.Sites, evidenceSite{
		File: file, Line: pos.Line, Mechanism: e.Mechanism, EnclosingFunc: e.EnclosingFunc,
	})
}

package main

import "sort"

// unionOpFuncs is every operation name any resolved SDK module has its own
// deserializeOpError<Op> function for -- the full set of operations this
// scan has SOME per-op ground truth to check against.
func unionOpFuncs(smt *serviceModuleTruth) map[string]bool {
	out := map[string]bool{}

	for _, mgt := range smt.Modules {
		for op := range mgt.OpFuncs {
			out[op] = true
		}
	}

	return out
}

// assignedGroundTruth narrows opUniverse to the operations belonging to a
// module domainModule actually assigned to some resolved domain
// (moduleassign.go) -- a module this dir only imports for shared types
// (dax's dataplane importing services/dynamodb, stepfunctions importing
// dynamodb and s3) is never assigned any domain, so its op count no longer
// pads ground truth for a service that never dispatches it. Falls back to
// the full union when domainModule is empty -- nothing resolved at all yet,
// so which module is real is genuinely unknown and coverageWarnings' "ZERO
// resolved" wording still needs the original denominator.
func assignedGroundTruth(
	smt *serviceModuleTruth,
	domainModule map[string]string,
	opUniverse map[string]bool,
) map[string]bool {
	assigned := map[string]bool{}
	for _, mod := range domainModule {
		assigned[mod] = true
	}

	if len(assigned) == 0 {
		return opUniverse
	}

	out := map[string]bool{}

	for mod := range assigned {
		for op := range smt.Modules[mod].OpFuncs {
			out[op] = true
		}
	}

	return out
}

// buildDomainOps groups, across every resolved operation, which domains
// (receiver-type names) resolved at least one root for it -- moduleassign.go's
// input for picking which module governs each domain.
func buildDomainOps(resolved map[string][]opRoot) map[string]map[string]bool {
	out := map[string]map[string]bool{}

	for op, roots := range resolved {
		for domain := range groupRootsByDomain(roots) {
			if out[domain] == nil {
				out[domain] = map[string]bool{}
			}

			out[domain][op] = true
		}
	}

	return out
}

func groupRootsByDomain(roots []opRoot) map[string][]opRoot {
	out := map[string][]opRoot{}

	for _, r := range roots {
		out[r.Domain] = append(out[r.Domain], r)
	}

	return out
}

// effectiveModule resolves which module governs domain: the service's only
// resolved module when there is exactly one (the common case, bypassing
// domain assignment entirely so a service with zero receiver-typed handlers
// still gets checked), or moduleassign.go's data-driven per-domain pick when
// there are several.
func effectiveModule(domain string, domainModule map[string]string, smt *serviceModuleTruth) (string, bool) {
	if len(smt.Modules) == 1 {
		for mod := range smt.Modules {
			return mod, true
		}
	}

	mod, ok := domainModule[domain]

	return mod, ok
}

// siblingsAccepting lists other operations (this module's own PerOp set,
// excluding op itself) whose declared codes DO include code -- the evidence
// that a finding is a real, misplaced code rather than a fabricated one:
// "this shared sentinel is right for these callers, wrong for this one."
// Capped and sorted for stable, readable output.
func siblingsAccepting(mgt *moduleGroundTruth, op, code string) []string {
	const maxSiblings = 5

	var out []string

	for other, codes := range mgt.PerOp {
		if other == op || !codes[code] {
			continue
		}

		out = append(out, other)
	}

	sort.Strings(out)

	if len(out) > maxSiblings {
		out = out[:maxSiblings]
	}

	return out
}

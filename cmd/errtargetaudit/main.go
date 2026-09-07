// Command errtargetaudit finds gopherstack-o46l's class: a REAL,
// correctly-spelled AWS error code -- present in the pinned SDK, legitimately
// correct elsewhere in the same service -- emitted by an operation whose OWN
// error deserializer never declares it. A real client's errors.As into the
// typed exception it should see never fires; the request fails with an
// opaque smithy.GenericAPIError instead. This is a DIFFERENT question from
// cmd/errcodeaudit's: that tool finds a code the SDK never defines ANYWHERE
// (fabricated out of nothing). Two manual sweeps (commits d7149d0f8,
// 19f3d65f0) found 29 of this tool's class across five services;
// cmd/errcodeaudit reported zero findings in all five, correctly -- it was
// answering a different question, not missing this one.
//
// GROUND TRUTH is per-operation, not per-service (deser.go): for each
// services/<dir>, every resolved pinned SDK module's own deserializers.go is
// read for every function whose name contains "deserializeOpError" (the
// same protocol-agnostic marker cmd/errcodeaudit's sdktruth.go already
// relies on -- confirmed the same shape across every protocol in this
// repo's pinned SDKs). The codes matched via strings.EqualFold inside THAT
// FUNCTION'S OWN BODY are that operation's declared set -- never merged
// across operations, which is the entire point: a code declared for a
// sibling operation must never leak into this one's. types/errors.go's own
// ErrorCode() literals are unioned with every operation's declared codes
// into a separate, SERVICE-WIDE "real code universe", used only to tell a
// real-but-misplaced code (class A, this tool) apart from a fabricated one
// (class B, cmd/errcodeaudit's job -- silently excluded here, never
// double-reported).
//
// RESOLVING WHICH HANDLER SERVES AN OPERATION reuses cmd/reqfielddiff's
// solved half of this problem (resolveop.go, dispatch.go, pkgindex.go):
// dispatch-table recognition (map literal, slice-of-struct binder,
// switch-statement dispatch) UNIONED with a name-convention fallback
// ("handle"+Op, the Full/Accurate/WithOpts suffixes, lowerCamel(Op)+"Action",
// bare lowerCamel(Op), then a case-insensitive match) -- reimplemented, not
// imported, because cmd/reqfielddiff and cmd/reqfieldscan are existing
// tools this campaign does not modify; see gopherstack-o46l's own filing
// for why re-deriving that resolution here, rather than reusing it, is
// exactly the mistake to avoid -- this reimplementation deliberately tracks
// the same shapes and the same "union, don't pick one and stop" philosophy.
//
// WHERE THIS TOOL GENERALIZES PAST THAT RESOLUTION, because a request FIELD
// and an error CODE live in structurally different places:
//
//   - Recursion depth is the same one hop (maxEmitHop in emit.go) as
//     cmd/reqfieldscan/cmd/reqfielddiff's maxHop, but the receiver it
//     follows is NOT restricted to this repo's uniform "h" Handler name.
//     Those tools stop at "h.<Method>" specifically to keep a BACKEND's
//     internal field names from leaking in as false "declared wire field"
//     matches. That hazard does not exist here: in three of the four
//     commits this tool was validated against, the actual sentinel-error
//     return sits in the BACKEND method a handler calls, one hop away, and
//     finding it is the whole point. So this tool follows any
//     `X.Method(...)` or bare `func(...)` call one hop, any receiver.
//   - Ground truth is never a request TYPE, only a function BODY -- so
//     resolveop.go's roots carry no struct bindings at all, only the
//     *ast.BlockStmt to scan and the resolved FuncDecl's receiver-type name
//     ("domain"), needed only for module assignment below.
//
// THE CLASSIFIER LAYER (classifiers.go) is this tool's own addition, built
// once per service and shared across every operation's walk, because the
// real emission site in this repo is almost never a literal code string --
// it is a SENTINEL passed through a shared mapper. Three shapes, all
// observed directly in the four validated commits:
//
//   - services/bedrock, services/iot, services/backup: a package-level
//     `var ErrX = errors.New(...)` sentinel, matched via `errors.Is(err,
//     ErrX)` in a switch or if-chain whose branch renders a fixed code
//     literal (`c.JSON(status, errorResponse("ConflictException", ...))`).
//     sentinelCodes scans every such switch/if in the package (there can be
//     more than one mapper -- services/iot's real shape has a general one
//     plus a stricter override the FIX introduces, never the pre-fix bug
//     state this tool targets) and builds one flat sentinel-name -> code
//     table.
//   - services/networkmanager: the sentinel is wrapped one hop deeper,
//     inside a locally-declared error TYPE's field (`&apiError{cause:
//     errNotFoundSentinel, ...}`), built by a constructor function
//     (notFoundError, validationError, ...) whose own body never mentions a
//     code literal at all -- the SAME sentinel table still resolves it,
//     because that constructor's cause field is just another bare
//     reference to a known sentinel one AST level down. constructorCode
//     follows exactly one hop of this indirection (any package-level func
//     whose LAST result is bare `error`, scanning its own return
//     statements -- including nested composite-literal field values and
//     fmt.Errorf's %w slot -- for a sentinel reference), matching this
//     repo's standing one-hop discipline. A constructor that itself calls
//     ANOTHER constructor, rather than referencing a sentinel directly, is
//     NOT resolved -- disclosed below, not silently missed.
//   - Outside the sentinel-mapper pattern entirely: services/ecs's own
//     direct mechanism (awserr.New/Newf, a Code/ErrorCode/Type-labeled
//     composite-literal field, a code-named var/const) is matched too, a
//     narrowed subset of cmd/errcodeaudit/extract.go's six rules (no
//     sink.go call-signature table, no positional-struct-field resolution,
//     no mapper.go central-table detection) -- enough to catch a service
//     that emits codes directly, at the cost of a call-site-argument sink
//     this tool cannot recognise as one; see BLIND SPOTS.
//
// AN OPERATION NAME COLLIDING ACROSS TWO PINNED MODULES
// (services/bedrock's PutResourcePolicy: a real, DIFFERENT operation in
// each of the bedrock and bedrockagent APIs, sharing one op-name string) is
// resolved by moduleassign.go DATA-DRIVEN, never by matching a Go type name
// to a module name: each domain (the resolved handler's own receiver-type
// name) is assigned to whichever candidate module's own known-operation set
// overlaps it most -- bedrock's "Handler" domain resolves ~108 operations
// that overlap heavily with the "bedrock" module and barely with
// "bedrockagent", and vice versa for "AgentsHandler". A domain whose best
// overlap is zero or tied is left UNASSIGNED, and every operation reachable
// only through it is skipped rather than checked against a guessed module.
// This is the one piece of machinery neither cmd/reqfieldscan nor
// cmd/reqfielddiff needed at all: a request FIELD's ground truth is always
// exactly one operation's Input struct, never ambiguous across modules the
// way an error code's operation-name key can be.
//
// INHERITED BLIND SPOTS, checked one by one against cmd/reqfieldscan's
// seven and cmd/reqfielddiff's identical list:
//
//  1. Slice-of-struct dispatch table: generalized in binderFields, same fix.
//
//  2. Local generic wrapper (cognitoidp's wrapAccuracy[I,O](fn)):
//     collectLocalWrapOpWrappers, identical logic.
//
//  3. Handler name suffixes (Full/Accurate/WithOpts): findHandlersByName
//     tries all three explicitly.
//
//  4. Go type alias in the struct collector: DOES NOT APPLY -- this tool
//     collects no structs at all, only function bodies, so there is no
//     struct-alias indirection to miss in the first place.
//
//  5. Anonymous inline struct decoding (opsworks): DOES NOT APPLY, same
//     reason as (4) -- this tool never needs to know a decode TARGET TYPE,
//     only whether a call site emits a code.
//
//  6. Method receiver not bound during local-binding collection: DOES NOT
//     APPLY -- this tool collects no per-function local bindings at all
//     (no field reads to resolve), only calls and returns.
//
//  7. A second in-package dispatch table behind suffixed/colliding names:
//     CHECKED DIRECTLY against this tool's own bedrock validation target,
//     which is exactly this shape (two real dispatch mechanisms in one
//     package, one op name -- PutResourcePolicy -- shared between them).
//     It does NOT bite here, but not because collectDispatchEntries
//     resolves the collision: bedrock's own two PutResourcePolicy handlers
//     have DIFFERENT Go names (handlePutResourcePolicy vs
//     handlePutKnowledgeBaseResourcePolicy), so the name-convention
//     fallback resolves each uniquely without ever needing to disambiguate
//     a shared key. A service where two same-named handlers ALSO shared a
//     literal Go function name would still silently collide in
//     idx.Dispatch exactly as reqfieldscan/reqfielddiff's own disclosed
//     blind spot describes -- unpatched here for the same reason those
//     tools give: no concrete failing instance has surfaced to design
//     against.
//
//     CHECKED SEPARATELY (gopherstack-fr30): cmd/reqfielddiff's
//     findHandlerByName and cmd/reqfieldscan's lowerKeyedHandlers both had a
//     DETERMINISM bug in their case-insensitive name-fallback -- picking
//     whichever match Go's randomized map iteration order visited first (or
//     last), so a service with 2+ case-insensitive matches resolved a
//     different handler from one run to the next. findHandlersByNameFold
//     here (resolveop.go) does NOT share that bug, structurally: it UNIONS
//     every case-insensitive match into the returned slice rather than
//     picking one, and every caller (walkOpEmissions, groupRootsByDomain,
//     buildDomainOps) treats that slice as a set, deduplicated by AST
//     position -- so which order the map happened to visit names in never
//     changes the final result, only an internal, unobserved ordering. No
//     fix was needed here; this was verified, not assumed.
//
// THIS TOOL'S OWN BLIND SPOTS, new to this class rather than inherited:
//   - errors.As / type-switch classification (`switch err.(type) { case
//     *NotFoundError: ... }`) is NOT modeled -- only errors.Is-against-a-
//     sentinel is. Every one of the 29 validated bugs resolves through a
//     sentinel-var mapper (even services/networkmanager's apiError type
//     ultimately renders via classifyError's own errors.Is switch on its
//     wrapped cause), so this cut cost nothing against known ground truth,
//     but a service whose ONLY mapper switches on concrete error TYPES
//     with no underlying sentinel at all would be invisible to this scan.
//   - A constructor function that wraps ANOTHER constructor, rather than a
//     sentinel directly, resolves to nothing (one hop only, matching this
//     repo's standing discipline) -- silently unresolved, never a false
//     finding. And a function IS EXCLUDED from constructor candidacy the
//     moment its own name matches a real ground-truth operation name
//     (buildClassifiers's opNames parameter) -- confirmed necessary, not
//     precautionary: an early version treated every backend method with a
//     bare `error` return (services/iot's DeleteThing/CancelJob/... shape,
//     extremely common in this repo) as a constructor too, which not only
//     double-counted a finding under two mechanisms but, worse, BYPASSED
//     the override-suppression below entirely (a misclassified backend
//     method's code is baked in at buildClassifiers time, before any
//     op-specific override is known) -- caught by this tool's own
//     TestScan_OverrideMapper_SuppressesGeneralMapping test failing against
//     the pre-fix implementation.
//   - An "override" mapper -- a helper taking the comparison sentinel as
//     its OWN parameter (services/iot's post-fix respondAsInvalidRequest
//     shape: `if errors.Is(err, sentinel) { return fixedCode }`) IS modeled
//     (detectOverrideFuncs/effectiveClassifiers), added during this tool's
//     own validation pass after it produced two confirmed false positives
//     on iot's CancelJob and DeleteThing (already-fixed, post-fix code
//     using exactly this shape) -- see classifiers.go's doc comment for the
//     mechanism. Still not modeled: an override whose comparison argument
//     at the call site is itself a computed/indirect expression rather than
//     a bare sentinel identifier, and an override applied only at hop 1 or
//     deeper (this scan looks for the override call in hop-0 roots only).
//   - SENTINEL-TO-CODE RESOLUTION IS SCOPED PER MAPPER FUNCTION, not one
//     flat package-wide table (gopherstack-0yva, fixed in the commit this
//     comment ships with). Before this fix, sentinelCodes built ONE map
//     keyed by sentinel IDENTIFIER NAME across the whole package: when two
//     DIFFERENT mapper functions branched on the SAME identifier to
//     DIFFERENT codes (services/eks's handleError and handleTagError, both
//     `errors.Is(err, ErrNotFound)`, mapping to ResourceNotFoundException
//     and NotFoundException respectively -- a real, deliberate difference
//     between that service's two tagging-API families), the second mapper
//     scanned silently overwrote the first's entry, and every operation
//     reachable only through the OVERWRITTEN mapper was measured against the
//     wrong code -- one collision produced 49 false findings in a single
//     service, all in one scan. classifiers.go's funcSentinelCodes now keeps
//     each mapper function's own table separately; emit.go's
//     localMapperScope finds which mapper(s) an OPERATION'S OWN hop-0 root
//     actually calls and resolves through ONLY those, re-resolving
//     constructor-classifier codes (cls.Funcs) through the same narrowed
//     table so a constructor whose call site never reaches the resolving
//     mapper (services/eks's validateTagMap, called from TagResource, which
//     never dispatches its error through ANY mapper) is not attributed that
//     mapper's code either. What THIS FIX STILL DOES NOT COVER, stated
//     plainly: (1) when no mapper call is found in an operation's own hop-0
//     root at all, resolution falls back to the package-wide flat table --
//     harmless for a service with exactly one mapper (the common case, and
//     this package's own sharedSentinelFixture/constructorFixture tests
//     rely on this fallback), but an operation whose ambiguous sentinel is
//     resolved this way, in a service where the responsible mapper is
//     invoked outside this scan's modeled call graph (framework-level
//     middleware, an indirection deeper than the one hop this tool follows),
//     is not scoped by this fix at all; (2) when a collision CANNOT be
//     pinned to a reachable mapper -- either via the flat-table fallback, or
//     because two DIFFERENT mappers are BOTH reachable from the SAME
//     operation and disagree -- the colliding identifier is dropped from
//     resolution entirely rather than guessed: a real bug hiding behind such
//     an unresolvable collision would be silently missed, not misreported,
//     matching this tool's standing "silent miss over false finding"
//     discipline elsewhere in this list, but it is a discipline, not a
//     guarantee of full recall; (3) the census this fix's own validation
//     ran (all services, not just eks) found 9 more services with at least
//     one same-name sentinel collision across mapper functions
//     (cloudfront, cloudwatch, elasticache, eventbridge, iotdataplane,
//     kinesis, lambda, s3, plus eks itself) -- each is now scoped the same
//     way, but none besides eks was individually hand-verified against its
//     own pinned SDK the way eks was in commit 43416bbd7, so treat a finding
//     newly surfaced or newly suppressed by this fix in any of those eight
//     with the same care as any other finding from this tool, not as
//     pre-verified.
//   - Direct-literal extraction is a narrowed subset of
//     cmd/errcodeaudit/extract.go's six rules -- no sink.go call-argument
//     position table (a "...Error"-suffixed call is invisible here, where
//     errcodeaudit resolves its actual sink argument), no mapper.go central
//     table detection, no positional (unkeyed) struct-field resolution. A
//     composite-literal "Code"-labeled field is DELIBERATELY excluded (only
//     "ErrorCode"/"Type" are read) after a confirmed false positive on
//     services/bedrock's BatchDeleteAdvancedPromptOptimizationJobError{Code:
//     "ResourceNotFoundException", ...} -- a per-ITEM result field in a 200
//     OK batch response, not a wire error envelope; see emit.go's
//     isCodeFieldLabel doc comment. A service relying on one of those
//     narrowed-out shapes for its ONLY emission mechanism is under-covered
//     here, though such a service would also need the sentinel-mapper
//     machinery above to be absent for a finding to be missed entirely.
//   - A code assembled through string concatenation, fmt.Sprintf, or read
//     from a request field is invisible, same limitation cmd/errcodeaudit
//     already discloses.
//   - PER-OPERATION GROUND TRUTH ITSELF IS ABSENT for a pinned SDK module
//     with NO deserializers.go file at all -- confirmed, at the aws-sdk-go-v2
//     versions this repo's go.mod pins, to be true of exactly ONE service
//     module out of all 168: cloudwatch's own "cloudwatch" module
//     (v1.66.3). services/appstream is a DIFFERENT case entirely, despite
//     both once being described together here: appstream's own module DOES
//     have a deserializers.go, with one rpc2_deserializeOpError<Op>
//     function per operation, just using a plain string switch (`switch
//     string(errorName) { case "ConflictException": ... }`) instead of
//     strings.EqualFold -- deser.go's stringSwitchCaseLiteral reads that
//     shape fine (before it existed, every RPCv2CBOR-protocol operation
//     read as declaring ZERO codes, a confirmed large false-positive
//     source: ~15 of 17 emitting appstream ops, all spurious). cloudwatch's
//     module has migrated to a newer smithy schema-based client codegen
//     that generates NO deserializers.go, no api_client.go-level
//     deserializeOpError<Op>-shaped function of any kind, and no per-op
//     switch/case anywhere in its api_op_<Op>.go files either (confirmed by
//     reading them: error resolution is one GENERIC middleware,
//     smithy-go's protocol.deserializeError, keyed on the service's own
//     shared type_registry.go -- a SERVICE-WIDE map from error name to Go
//     type with no operation attribution at all). types/errors.go still
//     exists and still lists real ErrorCode() literals, so this module still
//     contributes to AllCodes (cmd/errcodeaudit's class B boundary) -- it is
//     PerOp/OpFuncs specifically that is empty, because the per-operation
//     information this tool's whole premise rests on genuinely does not
//     exist in this module's Go source, for any service using this codegen
//     generation. Gopherstack-zkpi: rather than let that render as a
//     misleading 0/0 (indistinguishable from "nothing to audit", and before
//     that fix, not even printed at all -- run()'s own OpsGroundTruth==0
//     filter dropped it before printServiceScan ever saw it), scan.go's
//     modulesWithoutOpFuncs and report.go's untraceableModuleWarnings make a
//     module in this state a distinct, unconditional "UNTRACEABLE" warning.
//     cloudwatch is the only pinned module in this state today; nothing else
//     in the corpus has this shape to extend the fix to.
//   - A REPO-WIDE PATTERN, not specific to this tool: many services/<dir>
//     test files import an entirely unrelated service (s3 and dynamodb by
//     far the most common, confirmed live across cloudformation, cloudwatch,
//     dax, dynamodb, firehose, glacier, iam, kinesisanalytics, mgn,
//     stepfunctions) for some shared cross-service test helper --
//     cmd/errcodeaudit's own doc comment names the identical ec2/outposts
//     case. resolveServiceModules (matching cmd/errcodeaudit's own,
//     deliberately test-file-inclusive resolution) has no way to tell "the
//     service's own module" apart from "an incidental cross-import," so a
//     service whose OWN module contributes little or no per-op ground truth
//     (the RPCv2CBOR case above, or simply a module this repo's go.mod
//     doesn't pin a version for) can have its OpsGroundTruth count silently
//     dominated by the unrelated import's op names instead -- exactly what
//     happened to services/cloudwatch. The coverage guard's "resolved
//     ratio" check is what actually catches this in practice (a foreign
//     service's op names essentially never resolve to this service's own
//     handlers), and did so for all 13 services this pattern or the one
//     above touches in this scan's own full run -- but the guard is a
//     symptom check, not a diagnosis; a human still has to read which case
//     produced the warning, as this section did for two of the thirteen.
//
// REACHABILITY THROUGH A SHARED MAPPER (gopherstack-axs3, guardindex.go/
// reachability.go): before this fix, a candidate code from a shared mapper
// was attributed to EVERY operation that merely called the mapper, with no
// check on whether that operation's own backend could ever produce the
// SPECIFIC sentinel gating that mapper's branch. Measured in bulk, twice:
// services/bedrockagent (27 findings, all false -- a mapper classifying via
// errors.Is against pkgs/awserr's PACKAGE-QUALIFIED base sentinels, a shape
// the sentinel-identity scan below never even recognised as a mapper at
// all) and services/account (33 findings, all false -- a mapper classifying
// by strings.Contains(err.Error(), "CodeLiteral") instead of errors.Is,
// a second, unrelated guard mechanism). Both were one shared mapper each,
// so each was one root cause producing dozens of rows, not dozens of bugs.
//
// THE FIX: guardindex.go scans every switch/if in the package for a case
// gated by errors.Is(err, <sentinel>) (bare OR package-qualified, e.g.
// `awserr.ErrNotFound`) or strings.Contains(err.Error(), "<CodeLiteral>"),
// and records every code-shaped literal inside that case against the
// sentinel identity (or message substring) that guards it -- regardless of
// which of this file's literal-emission mechanisms (code-named var/const,
// composite-lit field, direct awserr/errors.New call) produced it, since
// all of them ultimately emit at a string literal's own position.
// reachability.go then computes, per operation, which sentinel identities
// its OWN hop-0/hop-1 backend calls (the SAME calls emit.go's own walk
// already follows -- no separate recursion policy to keep in sync) can
// actually return, following one hop of pkgs/awserr's New/Newf wrapping
// (a local `ErrNotFound = awserr.New("...", awserr.ErrNotFound)` resolves
// to the qualified base a mapper's errors.Is check compares against) and
// collecting each reachable sentinel's own literal message text for the
// strings.Contains guard shape. A candidate finding whose guard's identity
// (or message substring) is not in that reachable set is dropped.
//
// WHAT THIS FIX DELIBERATELY DOES NOT DO, matching this tool's standing
// "silent miss/report over false suppress" discipline:
//   - A guard this scan cannot parse at all (errors.As, or a case mixing a
//     recognised comparison with one it does not understand) is left
//     UNGUARDED: every literal inside it is always reported, never
//     suppressed on a guess. A case with NO recognisable guard at all (a
//     bare `default:`, or a literal outside any switch/if) is unguarded for
//     the same reason.
//   - When this operation's OWN call graph could not be resolved at hop 1
//     AT ALL (no callee this scan could find and read a body for --
//     reachSet.Determined stays false), every guarded finding for that
//     operation is still reported: an unresolved call graph is evidence of
//     nothing, and treating it as "therefore unreachable" would trade a
//     traceable false positive for a silent false negative.
//   - Reachability follows exactly ONE hop past an operation's handler
//     (the same maxEmitHop discipline emit.go's own emission walk uses) and
//     ONE hop of sentinel-wrapping unwrap. A sentinel returned two calls
//     deep, or wrapped by a helper this scan cannot see through, is
//     invisible to this reachability check the same way it would be
//     invisible to the emission walk itself -- silently under-suppressing
//     (reporting a finding that IS actually unreachable), never
//     over-suppressing.
//   - This scoping is entirely STRUCTURAL sentinel/message identity, never
//     data-flow or type-checking: a bare identifier or qualified selector
//     that merely LOOKS like a sentinel reference (any Ident/SelectorExpr
//     appearing in a return statement) is accepted into an operation's
//     reachable set without verifying it is actually the same value the
//     mapper compares against -- deliberately permissive, since an
//     over-broad reachable set can only ever cause under-suppression (more
//     findings kept), never the reverse.
//   - Cause grouping (report.go's printCauseGroups, already present)
//     surfaces a bulk shared-mapper event as "N findings, all via
//     <mechanism>+<code>" in the summary printed before the full finding
//     list, so a service shaped like bedrockagent or account is visible as
//     one root cause at a glance even for a finding this reachability
//     filter could not resolve and therefore still reports.
//
// WHAT THIS TOOL CANNOT TELL YOU, stated plainly:
//   - It cannot distinguish a REACHABLE handler from an unreachable one at
//     the OPERATION level -- only, now, whether a specific mapper BRANCH is
//     reachable from an operation whose handler it can resolve. A dead
//     operation a real client can never route to at all (this campaign has
//     found at least one, in services/iot) produces a finding exactly as
//     confident as a live one. Only driving a real client and watching the
//     router settles that.
//   - A code being DECLARED for an operation does not mean it is the RIGHT
//     code for the actual failure condition -- only that a real client's
//     errors.As into it would succeed. This tool checks wire-shape
//     reachability, never semantic correctness (gopherstack-uox6's axis
//     entirely).
//   - Attribution through a shared sentinel is APPROXIMATE, not certain: a
//     sentinel or constructor used correctly by most callers and wrongly by
//     one is attributed per (operation, domain) pair as precisely as this
//     tool's one-hop resolution reaches, but a deeper or more indirect
//     emission path than what classifiers.go models will simply not
//     surface, silently, not as a wrong finding.
//   - A "declared" verdict is drawn from EqualFold code LITERALS in the
//     operation's own deserializer switch; an SDK version whose
//     deserializer falls straight through to a generic decode for most
//     codes (cmd/errcodeaudit's own s3-class "sparsely modeled" case) would
//     make every absence here weak evidence, not strong -- this tool does
//     not currently carry that module's own sparse-coverage flag the way
//     cmd/errcodeaudit's moduleCodes.sparselyModeled does; a finding
//     against a thinly-modeled module should be treated with the same
//     caution that flag exists for.
//
// Usage:
//
//	go run ./cmd/errtargetaudit                       # scan every services/<dir>
//	go run ./cmd/errtargetaudit -dir bedrock,iot       # scan only these
//	go run ./cmd/errtargetaudit -json out.json         # also write the full report as JSON
//
// Exit codes: 0 no findings and no coverage warning in any scanned service,
// 1 a run error, 2 at least one class A finding, or at least one service
// tripped the resolution guard above, in at least one scanned service.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	exitClean    = 0
	exitRunError = 1
	exitFindings = 2
)

func main() {
	dirFlag := flag.String("dir", "", "comma-separated services/<dir> basenames to scan (default: all)")
	jsonOut := flag.String("json", "", "write the full scan list to this path as JSON")
	flag.Parse()

	scans, err := run(*dirFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(exitRunError)
	}

	if *jsonOut != "" {
		if werr := writeJSON(*jsonOut, scans); werr != nil {
			fmt.Fprintln(os.Stderr, "write json:", werr)
			os.Exit(exitRunError)
		}
	}

	findings := 0
	warned := 0

	for _, sr := range scans {
		printServiceScan(sr)

		findings += len(sr.Findings)
		if len(sr.Warnings) > 0 {
			warned++
		}
	}

	summarize(scans, findings, warned)

	if findings > 0 || warned > 0 {
		os.Exit(exitFindings)
	}

	os.Exit(exitClean)
}

func summarize(scans []serviceScan, findings, warned int) {
	scanned := 0

	for _, sr := range scans {
		if sr.OpsGroundTruth > 0 {
			scanned++
		}
	}

	fmt.Fprintf(os.Stdout, "# %d services scanned, %d class A findings, %d coverage warnings\n",
		scanned, findings, warned)
}

func run(dirFlag string) ([]serviceScan, error) {
	repoRoot, err := repoRootDir()
	if err != nil {
		return nil, err
	}

	cache, err := gomodcacheDir(repoRoot)
	if err != nil {
		return nil, err
	}

	goModVersions, err := loadGoModVersions(filepath.Join(repoRoot, "go.mod"))
	if err != nil {
		return nil, err
	}

	dirs, err := targetDirs(filepath.Join(repoRoot, "services"), dirFlag)
	if err != nil {
		return nil, err
	}

	var scans []serviceScan

	for _, dir := range dirs {
		sr, scanErr := scanServiceDir(dir, repoRoot, cache, goModVersions)
		if scanErr != nil {
			return nil, fmt.Errorf("%s: %w", dir, scanErr)
		}

		if !worthReporting(sr) {
			continue
		}

		scans = append(scans, sr)
	}

	return scans, nil
}

// worthReporting is false only for a service run() must drop before it ever
// reaches printServiceScan -- OpsGroundTruth==0 alone used to be that test,
// which silently dropped cloudwatch (gopherstack-zkpi: a resolved module
// with real error types but zero per-op ground truth still needs its
// UNTRACEABLE warning printed, not discarded here before coverageWarnings'
// own output is ever seen).
func worthReporting(sr serviceScan) bool {
	return sr.OpsGroundTruth > 0 || len(sr.Warnings) > 0
}

func targetDirs(svcRoot, dirFlag string) ([]string, error) {
	if dirFlag != "" {
		dirs := make([]string, 0, strings.Count(dirFlag, ",")+1)
		for d := range strings.SplitSeq(dirFlag, ",") {
			dirs = append(dirs, filepath.Join(svcRoot, strings.TrimSpace(d)))
		}

		sort.Strings(dirs)

		return dirs, nil
	}

	return serviceDirs(svcRoot)
}

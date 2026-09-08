package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// parseSrc parses one in-memory Go source file into a *pkgIndex -- fixtures
// below never touch the filesystem, matching cmd/reqfielddiff's own
// parseSrc test helper.
func parseSrc(t *testing.T, src string) *pkgIndex {
	t.Helper()

	fset := token.NewFileSet()

	f, err := parser.ParseFile(fset, "fixture.go", src, 0)
	require.NoError(t, err)

	return buildPkgIndexFromFiles([]*ast.File{f}, fset)
}

// newTestModuleGroundTruth builds a synthetic single-module ground truth --
// perOp is (op name -> declared code set), allCodes is every code this
// service's SDK models anywhere (the class A/B boundary).
func newTestModuleGroundTruth(perOp map[string]map[string]bool, allCodes map[string]bool) *moduleGroundTruth {
	mgt := newModuleGroundTruth()
	mgt.PerOp = perOp
	mgt.AllCodes = allCodes

	for op := range perOp {
		mgt.OpFuncs[op] = true
	}

	return mgt
}

func singleModuleTruth(mgt *moduleGroundTruth) *serviceModuleTruth {
	const mod = "fixture"

	return &serviceModuleTruth{Modules: map[string]*moduleGroundTruth{mod: mgt}}
}

// findingCodes returns the (op, code) pairs a scan reported, for compact
// assertions.
func findingCodes(findings []finding) map[string]string {
	out := map[string]string{}
	for _, f := range findings {
		out[f.Op] = f.Code
	}

	return out
}

// sharedSentinelFixture is the exact shape this tool exists to catch:
// GetThing and DeleteThing both call into a Backend method (a DIFFERENT
// receiver from the Handler, exercising this tool's ANY-receiver one-hop
// recursion) that returns the same package-level sentinel, mapped through
// one shared switch-based mapper (writeError) to a single wire code. Op
// resolution goes through switch-statement dispatch (dispatch(action)),
// covering the switch-dispatch structural shape this tool inherited from
// cmd/reqfieldscan/cmd/reqfielddiff.
const sharedSentinelFixture = `
package fixture

import (
	"errors"
	"fmt"
)

var ErrNotFound = errors.New("not found")

func writeError(err error) string {
	switch {
	case errors.Is(err, ErrNotFound):
		return "ResourceNotFoundException"
	}
	return "UnmappedFailureCode"
}

const opGetThing = "GetThing"
const opDeleteThing = "DeleteThing"

type Handler struct {
	Backend *Backend
}

func (h *Handler) dispatch(action string) error {
	switch action {
	case opGetThing:
		return h.handleGetThing()
	case opDeleteThing:
		return h.handleDeleteThing()
	}
	return nil
}

func (h *Handler) handleGetThing() error {
	return h.Backend.GetThing()
}

func (h *Handler) handleDeleteThing() error {
	return h.Backend.DeleteThing()
}

type Backend struct{}

func (b *Backend) GetThing() error {
	return fmt.Errorf("%w: thing", ErrNotFound)
}

func (b *Backend) DeleteThing() error {
	return fmt.Errorf("%w: thing", ErrNotFound)
}
`

// TestScan_SharedSentinel_AttributedPerOperation is this tool's central
// case: a shared sentinel/mapper is correct for GetThing (its own declared
// set has ResourceNotFoundException: no finding) and wrong for DeleteThing
// (its declared set does not: a finding, attributed to DeleteThing alone,
// never bleeding into GetThing). Covers "declared code -> no finding" and
// "undeclared code -> finding" in one fixture, since both are the same
// mapper output checked against two different operations' truth.
func TestScan_SharedSentinel_AttributedPerOperation(t *testing.T) {
	t.Parallel()

	idx := parseSrc(t, sharedSentinelFixture)
	smt := singleModuleTruth(newTestModuleGroundTruth(
		map[string]map[string]bool{
			"GetThing":    {"ResourceNotFoundException": true},
			"DeleteThing": {"UnmappedFailureCode": true},
		},
		map[string]bool{"ResourceNotFoundException": true, "UnmappedFailureCode": true},
	))

	sr := scanWithIndex("fixture", []string{"fixture"}, "/repo", idx, smt)

	codes := findingCodes(sr.Findings)
	require.NotContains(t, codes, "GetThing", "declared code must not be flagged")
	require.Equal(
		t,
		"ResourceNotFoundException",
		codes["DeleteThing"],
		"undeclared code must be flagged and attributed to DeleteThing",
	)
	require.Len(t, sr.Findings, 1, "the shared sentinel must not also produce a spurious GetThing finding")
	require.Empty(t, sr.OrphanFindings,
		"declared by a DIFFERENT op (GetThing) must stay class A, never also reported as the orphan class")

	require.Equal(t, 2, sr.OpsResolved)
}

// TestScan_ClassB_NotFabricated_NoFinding confirms a code absent from the
// SERVICE-WIDE code universe (never declared by ANY operation -- class B,
// cmd/errcodeaudit's job) produces no class A finding here, even though it
// is also absent from the specific operation's own declared set. This is
// gopherstack-zofv's orphan class instead -- see
// TestScan_OrphanCode_DeclaredByNoOp_NewClass below, same fixture.
func TestScan_ClassB_NotFabricated_NoFinding(t *testing.T) {
	t.Parallel()

	idx := parseSrc(t, sharedSentinelFixture)
	smt := singleModuleTruth(newTestModuleGroundTruth(
		map[string]map[string]bool{
			"GetThing":    {"SomeOtherException": true},
			"DeleteThing": {"SomeOtherException": true},
		},
		map[string]bool{"SomeOtherException": true}, // ResourceNotFoundException never declared anywhere
	))

	sr := scanWithIndex("fixture", []string{"fixture"}, "/repo", idx, smt)

	require.Empty(t, sr.Findings, "a code no operation ever declares is class B, out of this tool's scope")
}

// TestScan_OrphanCode_DeclaredByNoOp_NewClass is gopherstack-zofv's central
// case: ResourceNotFoundException is absent from EVERY operation's declared
// set AND from allCodes -- the tool previously had no way to report this at
// all (silently deferred to cmd/errcodeaudit, never double-reported here).
// Same fixture/ground truth as TestScan_ClassB_NotFabricated_NoFinding
// above: still zero class A findings, but now two orphan-code findings,
// one per operation that actually emits it.
func TestScan_OrphanCode_DeclaredByNoOp_NewClass(t *testing.T) {
	t.Parallel()

	idx := parseSrc(t, sharedSentinelFixture)
	smt := singleModuleTruth(newTestModuleGroundTruth(
		map[string]map[string]bool{
			"GetThing":    {"SomeOtherException": true},
			"DeleteThing": {"SomeOtherException": true},
		},
		map[string]bool{"SomeOtherException": true},
	))

	sr := scanWithIndex("fixture", []string{"fixture"}, "/repo", idx, smt)

	require.Empty(t, sr.Findings)
	require.Len(t, sr.OrphanFindings, 2)

	codes := findingCodes(sr.OrphanFindings)
	require.Equal(t, "ResourceNotFoundException", codes["GetThing"])
	require.Equal(t, "ResourceNotFoundException", codes["DeleteThing"])
}

// TestScan_OrphanCode_SparseModule_Suppressed is gopherstack-zofv's third,
// decisive fixture: a module whose own deserializer matched a declared code
// for only 1 of 3 OpFuncs (a 33% ratio, cmd/errcodeaudit's own sparse
// threshold) must not flood the orphan class with findings it has no real
// ground truth to back -- ResourceNotFoundException would otherwise qualify
// exactly as it does in TestScan_OrphanCode_DeclaredByNoOp_NewClass above,
// but this module cannot be trusted to say so.
func TestScan_OrphanCode_SparseModule_Suppressed(t *testing.T) {
	t.Parallel()

	idx := parseSrc(t, sharedSentinelFixture)
	mgt := newModuleGroundTruth()
	mgt.PerOp = map[string]map[string]bool{
		"GetThing": {"SomeOtherException": true},
	}
	mgt.OpFuncs = map[string]bool{
		"GetThing":    true,
		"DeleteThing": true, // has its own deserializeOpError func, but it matched zero codes
		"PutThing":    true, // ditto -- 1/3 matched, well under the 0.5 sparse threshold
	}
	mgt.AllCodes = map[string]bool{"SomeOtherException": true}

	smt := singleModuleTruth(mgt)

	sr := scanWithIndex("fixture", []string{"fixture"}, "/repo", idx, smt)

	require.NotEmpty(t, sr.ModulesSparse, "1/3 matched OpFuncs must be flagged sparse")
	require.Empty(t, sr.OrphanFindings,
		"a sparsely-modeled module must not flood the orphan class -- absence here is this tool's own "+
			"parsing gap, not evidence")
}

// TestScan_BorrowedModule_NotCountedInGroundTruth is gopherstack-2kud's dax
// and stepfunctions case: this fixture only dispatches GetThing/DeleteThing
// (its own module's ops), but its dir also imports a second SDK module
// (dynamodb-shaped: PutItem/Query/Scan) purely for shared types. That
// borrowed module's 3 ops must never inflate ground truth -- resolveOpRoots
// finds no handler named PutItem/Query/Scan, so no domain ever gets assigned
// the borrowed module (moduleassign.go), and assignedGroundTruth excludes it.
func TestScan_BorrowedModule_NotCountedInGroundTruth(t *testing.T) {
	t.Parallel()

	idx := parseSrc(t, sharedSentinelFixture)
	smt := &serviceModuleTruth{
		Modules: map[string]*moduleGroundTruth{
			"own": newTestModuleGroundTruth(
				map[string]map[string]bool{
					"GetThing":    {"ResourceNotFoundException": true},
					"DeleteThing": {"UnmappedFailureCode": true},
				},
				map[string]bool{"ResourceNotFoundException": true, "UnmappedFailureCode": true},
			),
			"borrowed": newTestModuleGroundTruth(
				map[string]map[string]bool{
					"PutItem": {"ConditionalCheckFailedException": true},
					"Query":   {"ResourceNotFoundException": true},
					"Scan":    {"ResourceNotFoundException": true},
				},
				map[string]bool{"ConditionalCheckFailedException": true, "ResourceNotFoundException": true},
			),
		},
	}

	sr := scanWithIndex("fixture", []string{"own", "borrowed"}, "/repo", idx, smt)

	require.Equal(t, 2, sr.OpsGroundTruth, "borrowed module's 3 ops must not inflate ground truth")
	require.Equal(t, 3, sr.OpsGroundTruthBorrowed)
	require.Equal(t, 2, sr.OpsResolved)

	codes := findingCodes(sr.Findings)
	require.NotContains(t, codes, "GetThing")
	require.Equal(t, "ResourceNotFoundException", codes["DeleteThing"])
	require.Len(t, sr.Findings, 1, "class A findings must not move when a borrowed module is present")
}

// TestScan_SingleModule_UnimplementedOpsStillCountUnresolved guards the
// other side of gopherstack-2kud: a service's OWN module (never borrowed)
// declaring more ops than this fixture dispatches must still report them
// unresolved -- a genuinely thin service's ratio is not a module-attribution
// artifact and must not be silently dropped from ground truth.
func TestScan_SingleModule_UnimplementedOpsStillCountUnresolved(t *testing.T) {
	t.Parallel()

	idx := parseSrc(t, sharedSentinelFixture)
	smt := singleModuleTruth(newTestModuleGroundTruth(
		map[string]map[string]bool{
			"GetThing":    {"ResourceNotFoundException": true},
			"DeleteThing": {"UnmappedFailureCode": true},
			"PutThing":    {"UnmappedFailureCode": true},
			"ListThings":  {"UnmappedFailureCode": true},
			"UpdateThing": {"UnmappedFailureCode": true},
		},
		map[string]bool{
			"ResourceNotFoundException": true,
			"UnmappedFailureCode":       true,
		},
	))

	sr := scanWithIndex("fixture", []string{"fixture"}, "/repo", idx, smt)

	require.Equal(t, 5, sr.OpsGroundTruth, "unimplemented ops from the service's OWN module must still count")
	require.Zero(t, sr.OpsGroundTruthBorrowed)
	require.Equal(t, 2, sr.OpsResolved)
}

// overlapFixture is gopherstack-2kud's dynamodb/opensearch/personalize
// case: a domain that mostly dispatches its OWN module's ops (GetThing,
// DeleteThing) also happens to implement one op literally named after a
// borrowed, unassigned module's op (PutItem: dynamodb has 58 own ops,
// dynamodbstreams only 4, so overlap still assigns this domain to the "own"
// module) -- PutItem must not count toward OpsResolved once its module is
// excluded from ground truth, or the reported ratio exceeds 100%.
const overlapFixture = `
package fixture

type Handler struct{}

func (h *Handler) dispatch(action string) error {
	switch action {
	case "GetThing":
		return h.handleGetThing()
	case "DeleteThing":
		return h.handleDeleteThing()
	case "PutItem":
		return h.handlePutItem()
	}
	return nil
}

func (h *Handler) handleGetThing() error { return nil }
func (h *Handler) handleDeleteThing() error { return nil }
func (h *Handler) handlePutItem() error { return nil }
`

// TestScan_OverlappingDomain_ResolvedNeverExceedsGroundTruth guards the
// numerator against the same fix that guards the denominator: PutItem
// resolves to a real handler, but its module ("borrowed") never gets
// assigned any domain (the domain's 2-op overlap with "own" beats its 1-op
// overlap with "borrowed"), so PutItem must be excluded from OpsResolved
// exactly as it is from OpsGroundTruth -- otherwise resolved > ground truth.
func TestScan_OverlappingDomain_ResolvedNeverExceedsGroundTruth(t *testing.T) {
	t.Parallel()

	idx := parseSrc(t, overlapFixture)
	smt := &serviceModuleTruth{
		Modules: map[string]*moduleGroundTruth{
			"own": newTestModuleGroundTruth(
				map[string]map[string]bool{
					"GetThing":    {"ResourceNotFoundException": true},
					"DeleteThing": {"UnmappedFailureCode": true},
				},
				map[string]bool{"ResourceNotFoundException": true, "UnmappedFailureCode": true},
			),
			"borrowed": newTestModuleGroundTruth(
				map[string]map[string]bool{
					"PutItem": {"ConditionalCheckFailedException": true},
					"Query":   {"ResourceNotFoundException": true},
					"Scan":    {"ResourceNotFoundException": true},
				},
				map[string]bool{"ConditionalCheckFailedException": true, "ResourceNotFoundException": true},
			),
		},
	}

	sr := scanWithIndex("fixture", []string{"own", "borrowed"}, "/repo", idx, smt)

	require.Equal(t, 2, sr.OpsGroundTruth)
	require.Equal(t, 2, sr.OpsResolved, "PutItem resolves to a handler but its module was never assigned")
	require.LessOrEqual(t, sr.OpsResolved, sr.OpsGroundTruth, "resolved must never exceed ground truth")
}

// constructorFixture is services/networkmanager's real shape: a
// constructor function (notFoundError) that never mentions a code literal
// itself, building a locally-declared error type whose field wraps a known
// sentinel one hop down. Also exercises name-convention-only resolution
// (no dispatch table at all), the anonymous-inline-struct blind spot's
// closest analogue for a tool with no decode-struct concept: a handler this
// tool can find ONLY via "handle"+Op naming, never through a dispatch
// table entry.
const constructorFixture = `
package fixture

import "errors"

var errNotFoundSentinel = errors.New("resource not found")

type apiError struct {
	cause error
	message string
}

func (e *apiError) Error() string { return e.message }
func (e *apiError) Unwrap() error { return e.cause }

func notFoundError(msg string) error {
	return &apiError{cause: errNotFoundSentinel, message: msg}
}

func classifyError(err error) string {
	switch {
	case errors.Is(err, errNotFoundSentinel):
		return "ResourceNotFoundException"
	}
	return "InternalServerException"
}

type Handler struct {
	Backend *Backend
}

func (h *Handler) handleCreateThing() error {
	return h.Backend.CreateThing()
}

type Backend struct{}

func (b *Backend) CreateThing() error {
	return notFoundError("thing not found")
}
`

// TestScan_ConstructorPropagation_NameConventionOnly resolves CreateThing
// with NO dispatch table entry present at all (findHandlersByName's
// "handle"+Op fallback is the only path), and the finding's code must come
// from following the constructor one hop to its wrapped sentinel.
func TestScan_ConstructorPropagation_NameConventionOnly(t *testing.T) {
	t.Parallel()

	idx := parseSrc(t, constructorFixture)
	require.Empty(t, idx.Dispatch, "fixture deliberately has no dispatch table")

	smt := singleModuleTruth(newTestModuleGroundTruth(
		map[string]map[string]bool{"CreateThing": {"SomePlaceholderCode": true}},
		map[string]bool{"ResourceNotFoundException": true, "SomePlaceholderCode": true},
	))

	sr := scanWithIndex("fixture", []string{"fixture"}, "/repo", idx, smt)

	codes := findingCodes(sr.Findings)
	require.Equal(t, "ResourceNotFoundException", codes["CreateThing"])
	require.Equal(t, "constructor classifier: notFoundError", sr.Findings[0].Sites[0].Mechanism)
}

// builtinShadowFixture is services/forecast/store.go's real shape
// (gopherstack-bfb3): Backend.delete is a real method reached via selector
// from handleDeleteThing, but its own body also calls Go's builtin
// delete(map, key) bare and unqualified -- the same identifier as the
// method's own name.
const builtinShadowFixture = `
package fixture

import (
	"errors"
	"fmt"
)

var errNotFoundSentinel = errors.New("resource not found")

func classifyError(err error) string {
	switch {
	case errors.Is(err, errNotFoundSentinel):
		return "ResourceNotFoundException"
	}
	return "InternalServerException"
}

type Handler struct {
	Backend *Backend
}

func (h *Handler) handleDeleteThing() error {
	return h.Backend.delete("thing", "id")
}

type Backend struct {
	arnIndex map[string]string
	tags     map[string]string
}

func (b *Backend) delete(kind, nameOrARN string) error {
	if nameOrARN == "" {
		return fmt.Errorf("%w: %s %q", errNotFoundSentinel, kind, nameOrARN)
	}

	delete(b.arnIndex, nameOrARN)
	delete(b.tags, nameOrARN)

	return nil
}
`

// TestScan_BuiltinDeleteNotConstructorClassifier is gopherstack-bfb3's
// regression: before the fix, callExprEmissions matched every bare `delete`
// call against cls.Funcs["delete"] (populated by the same-named method),
// so the two builtin delete(map, key) calls inside Backend.delete's own
// body -- which cannot raise at all -- were double-counted as their own
// "constructor classifier: delete" emission sites.
func TestScan_BuiltinDeleteNotConstructorClassifier(t *testing.T) {
	t.Parallel()

	idx := parseSrc(t, builtinShadowFixture)

	smt := singleModuleTruth(newTestModuleGroundTruth(
		map[string]map[string]bool{"DeleteThing": {"SomePlaceholderCode": true}},
		map[string]bool{"ResourceNotFoundException": true, "SomePlaceholderCode": true},
	))

	sr := scanWithIndex("fixture", []string{"fixture"}, "/repo", idx, smt)

	require.Len(t, sr.Findings, 1)

	f := sr.Findings[0]
	require.Equal(t, "DeleteThing", f.Op)
	require.Equal(t, "ResourceNotFoundException", f.Code)

	var ctorSites int
	for _, s := range f.Sites {
		if s.Mechanism == "constructor classifier: delete" {
			ctorSites++
		}
	}

	require.Equal(t, 1, ctorSites,
		"only the selector call h.Backend.delete(...) is a real constructor-classifier site; "+
			"the two bare builtin delete(map,key) calls inside Backend.delete's own body must not count")
}

// TestCoverageWarnings_ImplausibleResolution covers the loud-failure guard:
// a service where most ground-truth operations never resolved to a handler
// is reported as UNVERIFIED, not silently clean.
func TestCoverageWarnings_ImplausibleResolution(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		sr         serviceScan
		wantWarned bool
	}{
		{"zero resolved of many", serviceScan{OpsGroundTruth: 40, OpsResolved: 0}, true},
		{"low ratio", serviceScan{OpsGroundTruth: 40, OpsResolved: 10, OpsWithEmission: 10}, true},
		{"healthy ratio", serviceScan{OpsGroundTruth: 40, OpsResolved: 38, OpsWithEmission: 38}, false},
		{"small N below guard threshold", serviceScan{OpsGroundTruth: 3, OpsResolved: 1, OpsWithEmission: 1}, false},
		{"no ground truth at all", serviceScan{OpsGroundTruth: 0, OpsResolved: 0}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			warnings := coverageWarnings(tt.sr)
			if tt.wantWarned {
				require.NotEmpty(t, warnings)
			} else {
				require.Empty(t, warnings)
			}
		})
	}
}

// switchDispatchFixture isolates switch-statement dispatch resolution --
// cmd/reqfieldscan's own "took one service from 0 of 23 to 23 of 23" shape
// -- with no map literal anywhere.
const switchDispatchFixture = `
package fixture

type Handler struct{}

func (h *Handler) route(action string) error {
	switch action {
	case "PutWidget", "ReplaceWidget":
		return h.handlePutWidget()
	default:
		return nil
	}
}

func (h *Handler) handlePutWidget() error { return nil }
`

func TestResolveOpRoots_SwitchDispatch(t *testing.T) {
	t.Parallel()

	idx := parseSrc(t, switchDispatchFixture)

	for _, op := range []string{"PutWidget", "ReplaceWidget"} {
		roots := resolveOpRoots(op, idx)
		require.NotEmpty(t, roots, "op %s must resolve via switch-case dispatch (multi-value case list)", op)
	}
}

func TestResolveOpRoots_NameConventionFallback(t *testing.T) {
	t.Parallel()

	idx := parseSrc(t, constructorFixture)

	roots := resolveOpRoots("CreateThing", idx)
	require.Len(t, roots, 1)
	require.Equal(t, "Handler", roots[0].Domain)
}

// overrideFixture is services/iot's real post-fix shape: an override
// helper takes the comparison sentinel as ITS OWN parameter, so a hop-0
// call site can locally override what a hop-1 backend sentinel reference
// renders as.
const overrideFixture = `
package fixture

import (
	"errors"
	"fmt"
)

var ErrResourceNotFound = errors.New("not found")

const errTypeInvalidRequest = "InvalidRequestException"

func writeError(err error) string {
	switch {
	case errors.Is(err, ErrResourceNotFound):
		return "ResourceNotFoundException"
	}
	return "UnmappedFailureCode"
}

func respondAsInvalidRequest(err, sentinel error) string {
	if errors.Is(err, sentinel) {
		return errTypeInvalidRequest
	}
	return writeError(err)
}

type Handler struct {
	Backend *Backend
}

func (h *Handler) handleCancelJob() error {
	err := h.Backend.CancelJob()
	if err != nil {
		respondAsInvalidRequest(err, ErrResourceNotFound)
	}
	return err
}

type Backend struct{}

func (b *Backend) CancelJob() error {
	return fmt.Errorf("%w: job", ErrResourceNotFound)
}
`

// TestScan_OverrideMapper_SuppressesGeneralMapping confirms the
// respondAsInvalidRequest shape: CancelJob's own declared set includes
// InvalidRequestException (the override's code) but not
// ResourceNotFoundException (the general mapper's code) -- with the
// override modeled, this must be CLEAN, not a false positive.
func TestScan_OverrideMapper_SuppressesGeneralMapping(t *testing.T) {
	t.Parallel()

	idx := parseSrc(t, overrideFixture)
	smt := singleModuleTruth(newTestModuleGroundTruth(
		map[string]map[string]bool{"CancelJob": {"InvalidRequestException": true}},
		map[string]bool{
			"ResourceNotFoundException": true,
			"InvalidRequestException":   true,
			"UnmappedFailureCode":       true,
		},
	))

	sr := scanWithIndex("fixture", []string{"fixture"}, "/repo", idx, smt)

	require.Empty(t, sr.Findings, "override-mapper resolution must prevent the general-mapper false positive")
}

func TestDetectOverrideFuncs(t *testing.T) {
	t.Parallel()

	idx := parseSrc(t, overrideFixture)
	cls := buildClassifiers(idx, map[string]bool{"CancelJob": true})

	ov, ok := cls.Overrides["respondAsInvalidRequest"]
	require.True(t, ok)
	require.Equal(t, 1, ov.ParamIndex)
	require.Equal(t, "InvalidRequestException", ov.Code)
	require.Equal(t, -1, ov.CodeParamIndex, "fixed-code override must not also carry a code param index")
}

// paramCodeOverrideFixture is services/iot's real respondAsConflictCode
// shape (gopherstack-il42): the override helper takes BOTH the comparison
// sentinel AND the emitted code as its OWN parameters, so -- unlike
// respondAsInvalidRequest above -- the code literal never appears inside the
// helper's own body at all, only at each call site. CreateThing routes
// through the override (ConflictException); GetThing does NOT, and stays on
// the general mapper's ResourceAlreadyExistsException -- exercising both
// paths from the same package so the override detector's precision fix can't
// be mistaken for a recall loss.
const paramCodeOverrideFixture = `
package fixture

import (
	"errors"
	"fmt"
)

var ErrAlreadyExists = errors.New("already exists")

func writeError(err error) string {
	switch {
	case errors.Is(err, ErrAlreadyExists):
		return "ResourceAlreadyExistsException"
	}
	return "UnmappedFailureCode"
}

func respondAsConflictCode(err, sentinel error, code string) string {
	if errors.Is(err, sentinel) {
		return code
	}
	return writeError(err)
}

type Handler struct {
	Backend *Backend
}

func (h *Handler) handleCreateThing() string {
	err := h.Backend.CreateThing()
	if err != nil {
		return respondAsConflictCode(err, ErrAlreadyExists, "ConflictException")
	}
	return ""
}

func (h *Handler) handleGetThing() string {
	err := h.Backend.GetThing()
	if err != nil {
		return writeError(err)
	}
	return ""
}

type Backend struct{}

func (b *Backend) CreateThing() error {
	return fmt.Errorf("%w: thing", ErrAlreadyExists)
}

func (b *Backend) GetThing() error {
	return fmt.Errorf("%w: thing", ErrAlreadyExists)
}
`

func TestDetectOverrideFuncs_ParamCode(t *testing.T) {
	t.Parallel()

	idx := parseSrc(t, paramCodeOverrideFixture)
	cls := buildClassifiers(idx, map[string]bool{"CreateThing": true, "GetThing": true})

	ov, ok := cls.Overrides["respondAsConflictCode"]
	require.True(t, ok)
	require.Equal(t, 1, ov.ParamIndex, "sentinel is respondAsConflictCode's 2nd param")
	require.Equal(t, 2, ov.CodeParamIndex, "code is respondAsConflictCode's 3rd (string-typed) param")
	require.Empty(t, ov.Code, "a param-code override has no fixed code baked into its own body")
}

// TestScan_ParamCodeOverride_SuppressesGeneralMapping confirms the
// respondAsConflictCode shape: CreateThing's own declared set includes
// ConflictException (the override's call-site code) but not
// ResourceAlreadyExistsException (the general mapper's code) -- with the
// call-site code resolved, this must be CLEAN, not the false positive
// gopherstack-il42 reported (six of ten iot findings were exactly this
// shape). GetThing, which never touches the override, correctly declares the
// general mapper's own code and must also stay clean -- proving the fix
// doesn't accidentally widen suppression to non-override call sites in the
// same package.
func TestScan_ParamCodeOverride_SuppressesGeneralMapping(t *testing.T) {
	t.Parallel()

	idx := parseSrc(t, paramCodeOverrideFixture)
	smt := singleModuleTruth(newTestModuleGroundTruth(
		map[string]map[string]bool{
			"CreateThing": {"ConflictException": true},
			"GetThing":    {"ResourceAlreadyExistsException": true},
		},
		map[string]bool{
			"ResourceAlreadyExistsException": true,
			"ConflictException":              true,
			"UnmappedFailureCode":            true,
		},
	))

	sr := scanWithIndex("fixture", []string{"fixture"}, "/repo", idx, smt)

	require.Empty(t, sr.Findings, "override resolution must prevent the general-mapper false positive on CreateThing")
}

// TestScan_ParamCodeOverride_RegressionGuard is the regression guard the
// override fix must not defeat: GetThing never calls respondAsConflictCode,
// so its own genuinely undeclared code (the general mapper's
// ResourceAlreadyExistsException) must still be reported -- proving the
// override detector narrows the tool rather than blinding it. CreateThing,
// routed through the override with its declared code, stays clean in the
// same scan.
func TestScan_ParamCodeOverride_RegressionGuard(t *testing.T) {
	t.Parallel()

	idx := parseSrc(t, paramCodeOverrideFixture)
	smt := singleModuleTruth(newTestModuleGroundTruth(
		map[string]map[string]bool{
			"CreateThing": {"ConflictException": true},
			"GetThing":    {"SomeOtherCode": true},
		},
		map[string]bool{
			"ResourceAlreadyExistsException": true,
			"ConflictException":              true,
			"UnmappedFailureCode":            true,
			"SomeOtherCode":                  true,
		},
	))

	sr := scanWithIndex("fixture", []string{"fixture"}, "/repo", idx, smt)

	codes := findingCodes(sr.Findings)
	require.NotContains(t, codes, "CreateThing", "override-declared code must stay clean")
	require.Equal(t, "ResourceAlreadyExistsException", codes["GetThing"],
		"a genuinely undeclared code with no override in play must still be flagged")
	require.Len(t, sr.Findings, 1)
}

// TestScan_ParamCodeOverride_WrongOverrideCodeStillFlagged proves an
// override is not automatically correct: CreateThing's declared set here
// does NOT include ConflictException (the override's own call-site code),
// so the finding must still surface -- attributed to the OVERRIDE's code,
// never silently to the general mapper's, since that is what this operation
// actually renders on the wire.
func TestScan_ParamCodeOverride_WrongOverrideCodeStillFlagged(t *testing.T) {
	t.Parallel()

	idx := parseSrc(t, paramCodeOverrideFixture)
	smt := singleModuleTruth(newTestModuleGroundTruth(
		map[string]map[string]bool{
			"CreateThing": {"SomeOtherCode": true},
			"GetThing":    {"ResourceAlreadyExistsException": true},
		},
		map[string]bool{
			"ResourceAlreadyExistsException": true,
			"ConflictException":              true,
			"UnmappedFailureCode":            true,
			"SomeOtherCode":                  true,
		},
	))

	sr := scanWithIndex("fixture", []string{"fixture"}, "/repo", idx, smt)

	codes := findingCodes(sr.Findings)
	require.NotContains(t, codes, "GetThing", "declared code must not be flagged")
	require.Equal(t, "ConflictException", codes["CreateThing"],
		"an override's own code must still be checked against ground truth, not assumed correct")
	require.Len(t, sr.Findings, 1)
}

func TestSentinelCodes_ErrorsIsSwitch(t *testing.T) {
	t.Parallel()

	idx := parseSrc(t, sharedSentinelFixture)
	codes := sentinelCodes(idx)

	require.Equal(t, "ResourceNotFoundException", codes["ErrNotFound"])
}

func TestConstructorCode_OneHopPropagation(t *testing.T) {
	t.Parallel()

	idx := parseSrc(t, constructorFixture)
	sentinels := sentinelCodes(idx)

	var found string

	for _, f := range idx.Files {
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Name.Name != "notFoundError" {
				continue
			}

			code, ok := constructorCode(fd, sentinels)
			require.True(t, ok)
			found = code
		}
	}

	require.Equal(t, "ResourceNotFoundException", found)
}

// TestBatchItemCodeField_NotFlagged is the confirmed false positive found
// during this tool's own validation (services/bedrock's
// BatchDeleteAdvancedPromptOptimizationJobError{Code: "..."}): a per-item
// result field named "Code" inside a 200-OK batch response, not a wire
// error envelope. isCodeFieldLabel must exclude bare "Code".
func TestBatchItemCodeField_NotFlagged(t *testing.T) {
	t.Parallel()

	require.False(t, isCodeFieldLabel("Code"))
	require.True(t, isCodeFieldLabel("ErrorCode"))
	require.True(t, isCodeFieldLabel("Type"))
}

func TestStringSwitchCaseLiteral_RPCv2CBORShape(t *testing.T) {
	t.Parallel()

	src := `
package fixture

func deserializeOpErrorPutThing() error {
	switch string(errorName) {
	case "ConflictException":
		return nil
	}
	return nil
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "d.go", src, 0)
	require.NoError(t, err)

	var found []string

	ast.Inspect(f, func(n ast.Node) bool {
		if lit, ok := stringSwitchCaseLiteral(n); ok {
			found = append(found, lit)
		}

		return true
	})

	require.Equal(t, []string{"ConflictException"}, found)
}

// TestGenericProtocolCodes_InternalServerException replaces an assertion that
// required the opposite. That one justified the entry by "the 90-false-positive
// mgn case", a premise gopherstack-udkm disproved: mgn@v1.48.4 declares
// InternalServerException in types/errors.go and in 3 deserializeOpError
// switches, so those emissions are class A findings, and the mgn concern was
// really fixed by making the allowlist module-conditional. gopherstack-oshm
// then found the entry fails the q9bs sourcing standard outright -- 51 modules
// model it per-op, and no docs-2.json describes returning it where api-2.json
// declares no such shape.
func TestGenericProtocolCodes_InternalServerException(t *testing.T) {
	t.Parallel()

	require.False(
		t,
		genericProtocolCodes["InternalServerException"],
		"must NOT be allowlisted: suppressing it hides real findings in services "+
			"whose own module declares no server-fault type (forecast, personalize)",
	)
}

// TestFirstCodeLiteral_SkipsMapLiteralKey is services/quicksight's confirmed
// false positive during gopherstack-zofv's own validation pass:
// writeError(c, code, errCode, msg)'s body builds map[string]any{"Code":
// errCode, "Message": msg}. Before firstCodeLiteral learned to skip a
// KeyValueExpr's Key, ast.Inspect's blind traversal found the map KEY
// ("Code", a plain string literal) before ever reaching errCode -- the
// PARAMETER that actually carries the real code at each call site, which
// this fixture deliberately can't resolve to a literal at all (matching
// the real writeError: the real code only exists at the CALLER).
func TestFirstCodeLiteral_SkipsMapLiteralKey(t *testing.T) {
	t.Parallel()

	src := `
package fixture

func writeError(code, msg string) error {
	_ = map[string]any{"Code": code, "Message": msg}
	return nil
}
`
	idx := parseSrc(t, src)
	fn := findFuncDecl(t, idx, "writeError")

	_, found := firstCodeLiteral(fn.Body, idx, 0)
	require.False(t, found, `a map literal's KEY ("Code") must never be read as the emitted code`)
}

// TestFirstCodeLiteral_SkipsConstKeyedMapLiteralKey is services/securityhub's
// confirmed false positive: map[string]any{keyMessage: msg}, where
// keyMessage is a package-level const ("Message") -- codeLiteralAtNode's
// *ast.Ident branch resolves it via idx.PkgConsts, but only the KEY
// position should ever be considered a map key, never a candidate code.
func TestFirstCodeLiteral_SkipsConstKeyedMapLiteralKey(t *testing.T) {
	t.Parallel()

	src := `
package fixture

const keyMessage = "Message"

func writeError(msg string) error {
	_ = map[string]any{keyMessage: msg}
	return nil
}
`
	idx := parseSrc(t, src)
	fn := findFuncDecl(t, idx, "writeError")

	_, found := firstCodeLiteral(fn.Body, idx, 0)
	require.False(t, found, "a const-keyed map literal's KEY must never be read as the emitted code")
}

// findFuncDecl locates a top-level function by name in idx, for tests that
// need to call an internal AST-walking function directly.
func findFuncDecl(t *testing.T, idx *pkgIndex, name string) *ast.FuncDecl {
	t.Helper()

	for _, f := range idx.Files {
		for _, decl := range f.Decls {
			if fd, ok := decl.(*ast.FuncDecl); ok && fd.Name.Name == name {
				return fd
			}
		}
	}

	t.Fatalf("no func %q found", name)

	return nil
}

// TestCompositeLitEmissions_TypeLabelMarkedWeak is gopherstack-zofv's second
// extraction fix: the "Type" composite-field label is legitimately
// ambiguous (iam/ecs's own per-op code field vs. AWS Query's <Error><Type>
// Sender</Type></Error> fault-type discriminator), confirmed to have
// produced zero true positives and 25+ false ones (Client/Sender/CNAME/
// GROUP/USER/...) once nothing else filtered its output. "ErrorCode" has no
// such ambiguity and must stay trusted.
func TestCompositeLitEmissions_TypeLabelMarkedWeak(t *testing.T) {
	t.Parallel()

	src := `
package fixture

var _ = errorResponse{Type: "Sender", ErrorCode: "RealException"}
`
	idx := parseSrc(t, src)

	var lit *ast.CompositeLit

	for _, f := range idx.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			if cl, ok := n.(*ast.CompositeLit); ok && lit == nil {
				lit = cl
			}

			return true
		})
	}

	require.NotNil(t, lit)

	byCode := map[string]emission{}
	for _, e := range compositeLitEmissions(lit) {
		byCode[e.Code] = e
	}

	require.True(t, byCode["Sender"].WeakLabel, `"Type" field label must be marked weak`)
	require.False(t, byCode["RealException"].WeakLabel, `"ErrorCode" field label must stay trusted`)
}

// TestClassifyEmission_WireFaultTypeDiscriminator_Excluded is gopherstack-
// zofv's third extraction fix: services/sns and services/glacier's own
// writeError helpers hold "Sender"/"Client" in a field firstCodeLiteral's
// one-hop callee recursion can still reach directly (not through
// compositeLitEmissions' own WeakLabel path at all), so the value itself
// must be excluded regardless of mechanism.
func TestClassifyEmission_WireFaultTypeDiscriminator_Excluded(t *testing.T) {
	t.Parallel()

	declared := map[string]bool{}
	allCodes := map[string]bool{}

	require.Equal(t, emissionDeclaredOrGeneric, classifyEmission("Sender", declared, allCodes))
	require.Equal(t, emissionDeclaredOrGeneric, classifyEmission("Client", declared, allCodes))
	require.Equal(t, emissionDeclaredOrGeneric, classifyEmission("Receiver", declared, allCodes))
	require.Equal(t, emissionDeclaredOrGeneric, classifyEmission("Server", declared, allCodes))
}

// TestClassifyEmission_GenericCodeConditionalOnAllCodes is gopherstack-udkm:
// ValidationException is a real per-operation exception in 57 pinned SDK
// modules (confirmed via grep -rl '"ValidationException"'
// $(go env GOMODCACHE)/.../service/*/deserializers.go), so genericProtocolCodes
// must excuse it only when this service's own AllCodes lacks it entirely --
// an op emitting it for a service where AllCodes has it (declared by SOME
// other operation) is a real class A finding, not a protocol-level freebie.
func TestClassifyEmission_GenericCodeConditionalOnAllCodes(t *testing.T) {
	t.Parallel()

	declared := map[string]bool{}
	allCodes := map[string]bool{"ValidationException": true}

	require.Equal(t, emissionClassA, classifyEmission("ValidationException", declared, allCodes))
}

// TestClassifyEmission_GenericCodeStillExcusedWhenAbsentEverywhere is the
// conditional check's non-regression half: a genuinely generic code with no
// module modeling it anywhere in this service must stay excused, not
// reported as an orphan finding.
func TestClassifyEmission_GenericCodeStillExcusedWhenAbsentEverywhere(t *testing.T) {
	t.Parallel()

	declared := map[string]bool{}
	allCodes := map[string]bool{}

	require.Equal(t, emissionDeclaredOrGeneric, classifyEmission("Throttling", declared, allCodes))
	require.Equal(t, emissionDeclaredOrGeneric, classifyEmission("ValidationException", declared, allCodes))
}

// TestScan_OrphanCode_WeakTypeLabel_Suppressed is the scan-level version of
// TestCompositeLitEmissions_TypeLabelMarkedWeak: services/workmail's real
// shape (DescribeEntity{Type: "GROUP"}) is ordinary API response data, not
// a code, and must never surface as an orphan-code finding even though
// "GROUP" is absent from allCodes exactly like a genuine orphan would be.
func TestScan_OrphanCode_WeakTypeLabel_Suppressed(t *testing.T) {
	t.Parallel()

	src := `
package fixture

type Handler struct{ Backend *Backend }

func (h *Handler) dispatch(action string) error {
	switch action {
	case "GetThing":
		return h.handleGetThing()
	}
	return nil
}

func (h *Handler) handleGetThing() error {
	return h.Backend.GetThing()
}

type Backend struct{}

func (b *Backend) GetThing() error {
	_ = entityInfo{Type: "GROUP", Name: "foo"}
	return nil
}
`
	idx := parseSrc(t, src)
	smt := singleModuleTruth(newTestModuleGroundTruth(
		map[string]map[string]bool{"GetThing": {"SomeException": true}},
		map[string]bool{"SomeException": true},
	))

	sr := scanWithIndex("fixture", []string{"fixture"}, "/repo", idx, smt)

	require.Empty(t, sr.Findings)
	require.Empty(t, sr.OrphanFindings,
		`a "Type"-labeled composite field holding unrelated domain data ("GROUP") must never surface as `+
			"an orphan-code finding")
}

// collisionScopedFixture is services/eks's real shape (gopherstack-0yva,
// commit 43416bbd7): handleError and handleTagError both branch on the SAME
// identifier ErrNotFound to DIFFERENT codes. DescribeThing's own path calls
// only handleError; TagResourceValidated's calls only handleTagError. It
// also carries the "mirror" false positive from the same commit: a
// constructor (validateTagInput, returning bare ErrValidation) whose ONLY
// package-wide resolution comes from handleError, called from an operation
// whose own path never reaches handleError at all.
const collisionScopedFixture = `
package fixture

import (
	"errors"
	"fmt"
)

var ErrNotFound = errors.New("not found")
var ErrValidation = errors.New("invalid")

type Handler struct {
	Backend *Backend
}

func (h *Handler) handleError(err error) string {
	switch {
	case errors.Is(err, ErrNotFound):
		return "ResourceNotFoundException"
	case errors.Is(err, ErrValidation):
		return "InvalidParameterException"
	}
	return "InternalFailure"
}

func (h *Handler) handleTagError(err error) string {
	if errors.Is(err, ErrNotFound) {
		return "NotFoundException"
	}
	return "BadRequestException"
}

func (h *Handler) handleDescribeThing() error {
	err := h.Backend.DescribeThing()
	if err != nil {
		h.handleError(err)
	}
	return err
}

func validateTagInput() error {
	return ErrValidation
}

func (h *Handler) handleTagResourceValidated() error {
	if err := validateTagInput(); err != nil {
		return err
	}

	err := h.Backend.TagResourceInternal()
	if err != nil {
		h.handleTagError(err)
	}
	return err
}

type Backend struct{}

func (b *Backend) DescribeThing() error {
	return fmt.Errorf("%w: thing", ErrNotFound)
}

func (b *Backend) TagResourceInternal() error {
	return fmt.Errorf("%w: thing", ErrNotFound)
}
`

// TestFlattenSentinelCodes_CollisionOmitted confirms the package-wide
// fallback table never silently picks a winner between two mapper functions
// that map the same identifier to different codes.
func TestFlattenSentinelCodes_CollisionOmitted(t *testing.T) {
	t.Parallel()

	idx := parseSrc(t, collisionScopedFixture)
	flat := flattenSentinelCodes(funcSentinelCodes(idx))

	_, collided := flat["ErrNotFound"]
	require.False(t, collided, "a sentinel mapped to two different codes by two mappers must not resolve to either")

	require.Equal(t, "InvalidParameterException", flat["ErrValidation"], "a non-colliding sentinel must still resolve")
}

// TestLocalMapperScope_ScopesPerReachableMapper confirms an operation's own
// effective sentinel table comes from ONLY the mapper(s) its own hop-0 root
// actually calls, resolving the same identifier to two different codes for
// two different operations in the same package.
func TestLocalMapperScope_ScopesPerReachableMapper(t *testing.T) {
	t.Parallel()

	idx := parseSrc(t, collisionScopedFixture)
	cls := buildClassifiers(idx, map[string]bool{"DescribeThing": true, "TagResourceValidated": true})

	describeRoots := resolveOpRoots("DescribeThing", idx)
	require.NotEmpty(t, describeRoots)

	describeScope, describeScoped := localMapperScope(describeRoots, cls.ByFunc)
	require.True(t, describeScoped)
	require.Equal(t, "ResourceNotFoundException", describeScope["ErrNotFound"])

	tagRoots := resolveOpRoots("TagResourceValidated", idx)
	require.NotEmpty(t, tagRoots)

	tagScope, tagScoped := localMapperScope(tagRoots, cls.ByFunc)
	require.True(t, tagScoped)
	require.Equal(t, "NotFoundException", tagScope["ErrNotFound"])
}

// TestScan_SentinelCollision_ScopedPerMapper_NoFalsePositives is the
// gopherstack-0yva regression: services/eks's real 49-finding event,
// reproduced structurally. Must fail (produce findings) against a version
// that reverts to a single package-wide flat sentinel table.
func TestScan_SentinelCollision_ScopedPerMapper_NoFalsePositives(t *testing.T) {
	t.Parallel()

	idx := parseSrc(t, collisionScopedFixture)
	smt := singleModuleTruth(newTestModuleGroundTruth(
		map[string]map[string]bool{
			"DescribeThing":        {"ResourceNotFoundException": true},
			"TagResourceValidated": {"NotFoundException": true},
		},
		map[string]bool{
			"ResourceNotFoundException": true,
			"NotFoundException":         true,
			"InvalidParameterException": true,
			"BadRequestException":       true,
			"InternalFailure":           true,
		},
	))

	sr := scanWithIndex("fixture", []string{"fixture"}, "/repo", idx, smt)

	require.Empty(t, sr.Findings,
		"same-named sentinels resolved through different reachable mappers must not cross-contaminate, and a "+
			"constructor whose call site never reaches the resolving mapper must not be attributed that code")
}

// unresolvedCollisionFixture has two mapper functions colliding on the same
// identifier, like collisionScopedFixture, but NEITHER is called from the
// operation's own hop-0 root -- mirroring a mapper invoked outside this
// scan's modeled call graph. Neither code may be attributed.
const unresolvedCollisionFixture = `
package fixture

import (
	"errors"
	"fmt"
)

var ErrNotFound = errors.New("not found")

func mapperA(err error) string {
	switch {
	case errors.Is(err, ErrNotFound):
		return "ResourceNotFoundException"
	}
	return "InternalFailure"
}

func mapperB(err error) string {
	switch {
	case errors.Is(err, ErrNotFound):
		return "NotFoundException"
	}
	return "InternalFailure"
}

type Handler struct {
	Backend *Backend
}

func (h *Handler) handleGetThing() error {
	return h.Backend.GetThing()
}

type Backend struct{}

func (b *Backend) GetThing() error {
	return fmt.Errorf("%w: thing", ErrNotFound)
}
`

// TestScan_UnresolvableCollision_RefusesRatherThanGuesses confirms the
// "loud failure" fallback: when a collision cannot be pinned to a reachable
// mapper, the sentinel is dropped from resolution entirely -- neither
// mapper's code is attributed. Must fail (produce a finding for whichever
// mapper is visited last) against a version without collision detection.
func TestScan_UnresolvableCollision_RefusesRatherThanGuesses(t *testing.T) {
	t.Parallel()

	idx := parseSrc(t, unresolvedCollisionFixture)
	smt := singleModuleTruth(newTestModuleGroundTruth(
		map[string]map[string]bool{"GetThing": {"SomeOtherException": true}},
		map[string]bool{
			"SomeOtherException":        true,
			"ResourceNotFoundException": true,
			"NotFoundException":         true,
		},
	))

	sr := scanWithIndex("fixture", []string{"fixture"}, "/repo", idx, smt)

	require.Empty(t, sr.Findings,
		"neither colliding mapper's code is reachable through this operation's own call path; "+
			"the tool must refuse to report rather than guess which one applies")
}

// TestScan_SharedSentinel_NonCollidingManyCallers is a table-driven
// confirmation that flattenSentinelCodes/localMapperScope leave a NON-
// colliding shared sentinel's normal attribution untouched: many operations
// legitimately declare the shared mapper's code, and only the one that
// doesn't is reported -- gopherstack-0yva's fix must not suppress this
// shape, only the collision shape.
func TestScan_SharedSentinel_NonCollidingManyCallers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		declare bool
		wantLen int
	}{
		{"declares the shared mapper's code: clean", true, 0},
		{"does not declare it: reported", false, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			idx := parseSrc(t, sharedSentinelFixture)

			declared := map[string]bool{"UnmappedFailureCode": true}
			if tt.declare {
				declared = map[string]bool{"ResourceNotFoundException": true}
			}

			smt := singleModuleTruth(newTestModuleGroundTruth(
				map[string]map[string]bool{
					"GetThing":    {"ResourceNotFoundException": true},
					"DeleteThing": declared,
				},
				map[string]bool{"ResourceNotFoundException": true, "UnmappedFailureCode": true},
			))

			sr := scanWithIndex("fixture", []string{"fixture"}, "/repo", idx, smt)

			codes := findingCodes(sr.Findings)
			require.NotContains(t, codes, "GetThing")
			require.Len(t, sr.Findings, tt.wantLen)
		})
	}
}

// findingPairs renders findings as "Op/Code" strings for compact set
// membership assertions across more than one operation in a single scan.
func findingPairs(findings []finding) map[string]bool {
	out := map[string]bool{}
	for _, f := range findings {
		out[f.Op+"/"+f.Code] = true
	}

	return out
}

// qualifiedGuardFixture is services/bedrockagent's real, measured shape
// (gopherstack-axs3, 27 false positives): a mapper (handleErr) that
// classifies via errors.Is against a PACKAGE-QUALIFIED base sentinel
// (pkgs/awserr's own ErrNotFound/ErrAlreadyExists, wrapped locally with
// awserr.New) and renders the code through a "code = literal" assignment
// inside each switch case, NOT a bare `return "literal"` -- the shape
// funcSentinelCodes' bare-identifier-only errors.Is scan never saw at all,
// so the raw "code-named var" mechanism reported every case's code for
// every operation that merely called the mapper, regardless of which
// sentinel that operation's own backend could actually produce.
// GetThing's backend can only ever return ErrNotFound; CreateThing's can
// only ever return ErrAlreadyExists -- so only one of the mapper's two
// codes is a real finding for each.
const qualifiedGuardFixture = `
package fixture

import (
	"errors"
	"fmt"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
)

var ErrNotFound = awserr.New("ResourceNotFoundException", awserr.ErrNotFound)
var ErrAlreadyExists = awserr.New("ConflictException", awserr.ErrAlreadyExists)

func handleErr(err error) string {
	code := "InternalServerErrorException"
	switch {
	case errors.Is(err, awserr.ErrNotFound):
		code = "ResourceNotFoundException"
	case errors.Is(err, awserr.ErrAlreadyExists):
		code = "ConflictException"
	}
	return code
}

type Handler struct {
	Backend *Backend
}

func (h *Handler) handleGetThing() string {
	err := h.Backend.GetThing()
	if err != nil {
		return handleErr(err)
	}
	return ""
}

func (h *Handler) handleCreateThing() string {
	err := h.Backend.CreateThing()
	if err != nil {
		return handleErr(err)
	}
	return ""
}

func (h *Handler) handleWeirdThing(err error) string {
	if err != nil {
		return handleErr(err)
	}
	return ""
}

type Backend struct{}

func (b *Backend) GetThing() error {
	return fmt.Errorf("%w: thing", ErrNotFound)
}

func (b *Backend) CreateThing() error {
	return fmt.Errorf("%w: thing", ErrAlreadyExists)
}
`

func qualifiedGuardTruth() *serviceModuleTruth {
	allCodes := map[string]bool{
		"ResourceNotFoundException":    true,
		"ConflictException":            true,
		"InternalServerErrorException": true,
	}

	return singleModuleTruth(newTestModuleGroundTruth(
		map[string]map[string]bool{
			"GetThing":    {"InternalServerErrorException": true},
			"CreateThing": {"InternalServerErrorException": true},
			"WeirdThing":  {"InternalServerErrorException": true},
		},
		allCodes,
	))
}

// TestReachability_ReachableSentinel_Reported is scenario 1: an operation
// whose own backend method CAN return the sentinel behind a shared mapper's
// code must still be reported when it doesn't declare that code -- the
// reachability fix must not turn into a blanket shared-mapper suppression.
func TestReachability_ReachableSentinel_Reported(t *testing.T) {
	t.Parallel()

	idx := parseSrc(t, qualifiedGuardFixture)
	sr := scanWithIndex("fixture", []string{"fixture"}, "/repo", idx, qualifiedGuardTruth())

	pairs := findingPairs(sr.Findings)
	require.True(t, pairs["GetThing/ResourceNotFoundException"],
		"GetThing's own backend returns ErrNotFound, so this finding is real and must survive")
}

// TestReachability_UnreachableSentinel_Suppressed is scenario 2: the SAME
// mapper's OTHER code, gated by a sentinel GetThing's backend can never
// produce, must be suppressed -- this is the exact 27-finding bedrockagent
// shape and the 33-finding account shape gopherstack-axs3 measured.
func TestReachability_UnreachableSentinel_Suppressed(t *testing.T) {
	t.Parallel()

	idx := parseSrc(t, qualifiedGuardFixture)
	sr := scanWithIndex("fixture", []string{"fixture"}, "/repo", idx, qualifiedGuardTruth())

	pairs := findingPairs(sr.Findings)
	require.False(t, pairs["GetThing/ConflictException"],
		"GetThing's backend can only ever return ErrNotFound, never ErrAlreadyExists -- "+
			"the ConflictException branch is structurally unreachable for this operation")
}

// TestReachability_SharedMapper_OnlyReachableReported is scenario 3: one
// mapper serving two operations, where each operation's own reachable
// sentinel differs, must report exactly the reachable pairing for each --
// never both codes for both operations, and never neither.
func TestReachability_SharedMapper_OnlyReachableReported(t *testing.T) {
	t.Parallel()

	idx := parseSrc(t, qualifiedGuardFixture)
	sr := scanWithIndex("fixture", []string{"fixture"}, "/repo", idx, qualifiedGuardTruth())

	pairs := findingPairs(sr.Findings)
	require.True(t, pairs["GetThing/ResourceNotFoundException"])
	require.True(t, pairs["CreateThing/ConflictException"])
	require.False(t, pairs["GetThing/ConflictException"])
	require.False(t, pairs["CreateThing/ResourceNotFoundException"])
}

// TestReachability_UndeterminedReachability_StillReported is scenario 4:
// WeirdThing's error comes from a parameter, not a resolvable backend call
// -- this scan cannot determine what it can or cannot be, so BOTH of the
// mapper's codes must still be reported rather than suppressed. This is
// this package's own documented conservative default: an unresolved call
// graph is not evidence of unreachability.
func TestReachability_UndeterminedReachability_StillReported(t *testing.T) {
	t.Parallel()

	idx := parseSrc(t, qualifiedGuardFixture)
	sr := scanWithIndex("fixture", []string{"fixture"}, "/repo", idx, qualifiedGuardTruth())

	pairs := findingPairs(sr.Findings)
	require.True(t, pairs["WeirdThing/ResourceNotFoundException"],
		"reachability for WeirdThing could not be determined, so this finding must not be suppressed")
	require.True(t, pairs["WeirdThing/ConflictException"],
		"reachability for WeirdThing could not be determined, so this finding must not be suppressed")
}

// messageGuardFixture is services/account's real shape (also part of
// gopherstack-axs3's 33 false positives): a mapper classifying by
// strings.Contains(err.Error(), "CodeLiteral") rather than errors.Is at
// all -- a completely different guard mechanism from qualifiedGuardFixture,
// which caseGuard/indexCaseLiterals must recognise on its own terms.
const messageGuardFixture = `
package fixture

import (
	"errors"
	"strings"
)

var errNotFound = errors.New("ResourceNotFoundException: thing missing")
var errConflict = errors.New("ConflictException: thing exists")

func writeBackendError(err error) string {
	code := "InternalServerErrorException"
	switch {
	case strings.Contains(err.Error(), "ResourceNotFoundException"):
		code = "ResourceNotFoundException"
	case strings.Contains(err.Error(), "ConflictException"):
		code = "ConflictException"
	}
	return code
}

type Handler struct {
	Backend *Backend
}

func (h *Handler) handleGetThing() string {
	err := h.Backend.GetThing()
	if err != nil {
		return writeBackendError(err)
	}
	return ""
}

type Backend struct{}

func (b *Backend) GetThing() error {
	return errNotFound
}
`

// TestReachability_MessageSubstringGuard_OnlyReachableReported confirms the
// strings.Contains(err.Error(), ...) guard shape (services/account's real
// mechanism, distinct from errors.Is) is filtered the same way: GetThing's
// backend can only ever return errNotFound, so ConflictException must be
// suppressed even though it is never declared either.
func TestReachability_MessageSubstringGuard_OnlyReachableReported(t *testing.T) {
	t.Parallel()

	idx := parseSrc(t, messageGuardFixture)
	smt := singleModuleTruth(newTestModuleGroundTruth(
		map[string]map[string]bool{"GetThing": {"InternalServerErrorException": true}},
		map[string]bool{
			"ResourceNotFoundException":    true,
			"ConflictException":            true,
			"InternalServerErrorException": true,
		},
	))

	sr := scanWithIndex("fixture", []string{"fixture"}, "/repo", idx, smt)

	pairs := findingPairs(sr.Findings)
	require.True(t, pairs["GetThing/ResourceNotFoundException"])
	require.False(t, pairs["GetThing/ConflictException"],
		"GetThing's backend never returns a message containing ConflictException")
}

// directCallFixture exercises this scan's oldest mechanism -- a direct
// awserr.New("Code", ...) call at the actual emission site
// (awserrLiteralEmissions), services/ecs's own shape -- as a control: this
// must keep working exactly as it did before gopherstack-yn2o's fix.
const directCallFixture = `
package fixture

import "github.com/blackbirdworks/gopherstack/pkgs/awserr"

const opGetThing = "GetThing"

type Handler struct {
	Backend *Backend
}

func (h *Handler) dispatch(action string) error {
	switch action {
	case opGetThing:
		return h.handleGetThing()
	}
	return nil
}

func (h *Handler) handleGetThing() error {
	return h.Backend.GetThing()
}

type Backend struct{}

func (b *Backend) GetThing() error {
	return awserr.New("ResourceNotFoundException", "thing not found")
}
`

// TestScan_DirectCall_Detected is the control case: a direct-literal
// emission must be detected exactly as before -- gopherstack-yn2o's fix
// must not have disturbed this pre-existing mechanism.
func TestScan_DirectCall_Detected(t *testing.T) {
	t.Parallel()

	idx := parseSrc(t, directCallFixture)
	smt := singleModuleTruth(newTestModuleGroundTruth(
		map[string]map[string]bool{"GetThing": {}},
		map[string]bool{"ResourceNotFoundException": true},
	))

	sr := scanWithIndex("fixture", []string{"fixture"}, "/repo", idx, smt)

	require.Equal(t, 1, sr.OpsWithEmission, "a direct awserr.New call must be detected as an emission")
	require.Equal(
		t, "ResourceNotFoundException", findingCodes(sr.Findings)["GetThing"],
		"GetThing's own declared set is empty, so the undeclared code must be flagged",
	)
}

// tableShapeFixture is services/kms and services/sqs's own real mechanism,
// reduced to its essential shape: a data-driven table of {sentinel, code}
// rows, iterated at runtime via a for-range loop and errors.Is against the
// loop VARIABLE's own field (m.sentinel) -- never a bare sentinel
// identifier, which is exactly what made addIfSentinelCodes /
// addSwitchSentinelCodes blind to it before gopherstack-yn2o. classify is
// deliberately never called from any op's own body -- kms's real
// classifyKMSError is reached only through a framework error callback, not
// through any per-operation call graph -- to prove this scan's package-wide
// mapper-table resolution does not depend on hop-reachability the way
// walkOpEmissions' own sentinel-return detection still correctly does.
const tableShapeFixture = `
package fixture

import "errors"

var ErrThingNotFound = errors.New("not found")

type errorMapping struct {
	sentinel error
	code     string
}

func classify(err error) string {
	mappings := []errorMapping{
		{ErrThingNotFound, "ResourceNotFoundException"},
	}
	for _, m := range mappings {
		if errors.Is(err, m.sentinel) {
			return m.code
		}
	}
	return ""
}

const opGetThing = "GetThing"

type Handler struct {
	Backend *Backend
}

func (h *Handler) dispatch(action string) error {
	switch action {
	case opGetThing:
		return h.handleGetThing()
	}
	return nil
}

func (h *Handler) handleGetThing() error {
	return h.Backend.GetThing()
}

type Backend struct{}

func (b *Backend) GetThing() error {
	return ErrThingNotFound
}
`

// TestScan_TableShape_DetectedAfterFix is the case gopherstack-yn2o exists
// to fix: this fixture is a REDUCTION of services/kms's real
// kmsErrorTable/classifyKMSError shape, and must be detected as an
// emission source. Hollow before addRangeTableSentinelCodes existed --
// see this file's revert-proof note below.
func TestScan_TableShape_DetectedAfterFix(t *testing.T) {
	t.Parallel()

	idx := parseSrc(t, tableShapeFixture)
	smt := singleModuleTruth(newTestModuleGroundTruth(
		map[string]map[string]bool{"GetThing": {}},
		map[string]bool{"ResourceNotFoundException": true},
	))

	sr := scanWithIndex("fixture", []string{"fixture"}, "/repo", idx, smt)

	require.Equal(
		t, 1, sr.OpsWithEmission,
		"the for-range table mapper (services/kms's own shape) must be detected as an emission source",
	)
	require.Equal(
		t, "ResourceNotFoundException", findingCodes(sr.Findings)["GetThing"],
		"GetThing's own declared set is empty, so the undeclared table-mapped code must be flagged",
	)
}

// unknownShapeFixture is services/docdb's own real shape: a bare `[]error`
// slice (no code field at all -- the sentinel's OWN .Error() message IS the
// code, resolved only at runtime) walked the same errors.Is-in-a-for-range
// way as tableShapeFixture. Neither addIfSentinelCodes/addSwitchSentinelCodes
// nor addRangeTableSentinelCodes can extract a code from this: there is no
// code-shaped literal anywhere in the row, because there is no row -- the
// slice element IS the sentinel. This is the shape gopherstack-yn2o's design
// question (A vs B) turns on: A cannot reach it, so only B -- reporting
// zero emissions as a loud, distinct BLIND warning -- keeps this scan
// honest about it.
const unknownShapeFixture = `
package fixture

import "errors"

var ErrThingNotFound = errors.New("ThingNotFoundFault")

const opGetThing = "GetThing"

type Handler struct {
	Backend *Backend
}

func (h *Handler) dispatch(action string) error {
	switch action {
	case opGetThing:
		return h.handleGetThing()
	}
	return nil
}

func (h *Handler) handleGetThing() error {
	return h.Backend.GetThing()
}

type Backend struct{}

func (b *Backend) GetThing() error {
	return ErrThingNotFound
}

func classify(err error) string {
	sentinels := []error{ErrThingNotFound}
	for _, s := range sentinels {
		if errors.Is(err, s) {
			return s.Error()
		}
	}
	return ""
}
`

// TestScan_UnknownShape_ReportedBlindNotZero is the third, load-bearing
// case: a shape NEITHER mechanism understands must surface as a loud BLIND
// warning, not a silent "zero class A findings" clean bill of health --
// this is the whole point of implementing option B (see emissionCoverageWarnings).
func TestScan_UnknownShape_ReportedBlindNotZero(t *testing.T) {
	t.Parallel()

	idx := parseSrc(t, unknownShapeFixture)
	smt := singleModuleTruth(newTestModuleGroundTruth(
		map[string]map[string]bool{"GetThing": {}},
		map[string]bool{"ThingNotFoundFault": true},
	))

	sr := scanWithIndex("fixture", []string{"fixture"}, "/repo", idx, smt)

	require.Equal(t, 1, sr.OpsResolved)
	require.Equal(
		t, 0, sr.OpsWithEmission,
		"this scan cannot statically resolve a code computed via sentinel.Error() at runtime",
	)
	require.NotEmpty(t, sr.Warnings, "zero emissions with ops resolved must be reported as BLIND, never silently clean")
	require.Contains(t, sr.Warnings[0], "BLIND")
	require.Empty(t, sr.Findings, "nothing was detected, so there is nothing to find")
}

// noOpFuncsFixture is a trivial package -- its content does not matter,
// because the module ground truth below has an empty PerOp/OpFuncs, so
// opUniverse is empty and nothing in this source is ever walked. This is
// cloudwatch's own real shape: types/errors.go models real error types
// (AllCodes non-empty) but deserializers.go does not exist at all, so
// deser.go's OpFuncs for this module is empty regardless of what this
// tool's parsing logic is taught -- the per-operation information simply
// is not present anywhere in the module's Go source.
const noOpFuncsFixture = `
package fixture

type Handler struct{}
`

// TestScan_ModuleWithoutOpFuncs_ReportedUntraceable is gopherstack-zkpi's
// load-bearing case: a resolved module with real error types but zero
// per-operation ones must surface as a distinct, loud UNTRACEABLE warning --
// never as a silent 0/0 that reads as "nothing to audit". Before this fix,
// scan.go computed no ModulesNoOpFuncs at all and report.go's
// coverageWarnings returned immediately on OpsGroundTruth==0, so this exact
// fixture produced zero warnings.
func TestScan_ModuleWithoutOpFuncs_ReportedUntraceable(t *testing.T) {
	t.Parallel()

	idx := parseSrc(t, noOpFuncsFixture)
	smt := singleModuleTruth(newTestModuleGroundTruth(
		map[string]map[string]bool{}, // no operation has a deserializeOpError-shaped function
		map[string]bool{"ConcurrentModificationException": true, "ResourceNotFoundException": true},
	))

	sr := scanWithIndex("fixture", []string{"fixture"}, "/repo", idx, smt)

	require.Zero(t, sr.OpsGroundTruth, "a module with no OpFuncs contributes no ground-truth ops")
	require.Equal(t, []string{"fixture"}, sr.ModulesNoOpFuncs,
		"the module must be recorded as contributing real error types but zero per-op ground truth")
	require.NotEmpty(t, sr.Warnings, "must not silently read as 0/0 clean")
	require.Contains(t, sr.Warnings[0], "UNTRACEABLE")
	require.Empty(t, sr.Findings, "nothing was scanned, so there is nothing to find")
	require.True(t, worthReporting(sr), "run() must not drop this service before its warning is ever printed")
}

// TestScan_ModuleWithOpFuncs_NotFlaggedUntraceable proves the fix above
// EXTENDS this tool rather than replacing its existing behavior: a normal
// module (real per-op ground truth present, the shape every other test in
// this file already exercises) must never be recorded in ModulesNoOpFuncs
// or warned about, and its findings must be exactly what
// TestScan_SharedSentinel_AttributedPerOperation already expects.
func TestScan_ModuleWithOpFuncs_NotFlaggedUntraceable(t *testing.T) {
	t.Parallel()

	idx := parseSrc(t, sharedSentinelFixture)
	smt := singleModuleTruth(newTestModuleGroundTruth(
		map[string]map[string]bool{
			"GetThing":    {"ResourceNotFoundException": true},
			"DeleteThing": {"UnmappedFailureCode": true},
		},
		map[string]bool{"ResourceNotFoundException": true, "UnmappedFailureCode": true},
	))

	sr := scanWithIndex("fixture", []string{"fixture"}, "/repo", idx, smt)

	require.Empty(t, sr.ModulesNoOpFuncs, "a module with real OpFuncs must never be flagged untraceable")

	for _, w := range sr.Warnings {
		require.NotContains(t, w, "UNTRACEABLE")
	}

	require.Equal(t, "ResourceNotFoundException", findingCodes(sr.Findings)["DeleteThing"],
		"the already-supported shared-sentinel finding must survive unchanged")
}

// TestCoverageWarnings_UntraceableModule_FiresEvenAtZeroGroundTruth checks
// report.go's own unit directly: untraceableModuleWarnings must fire ahead
// of coverageWarnings' OpsGroundTruth==0 early return, not after it -- a
// module recorded in ModulesNoOpFuncs necessarily has OpsGroundTruth==0
// (it contributes no ops), so the ordering is the entire fix.
func TestCoverageWarnings_UntraceableModule_FiresEvenAtZeroGroundTruth(t *testing.T) {
	t.Parallel()

	sr := serviceScan{OpsGroundTruth: 0, OpsResolved: 0, ModulesNoOpFuncs: []string{"cloudwatch"}}

	warnings := coverageWarnings(sr)

	require.NotEmpty(t, warnings, "a service in this state must never report zero warnings")
	require.Contains(t, warnings[0], "UNTRACEABLE")
	require.Contains(t, warnings[0], `"cloudwatch"`)
}

// TestWorthReporting_KeepsZeroGroundTruthServiceWithWarnings is main.go's
// run() filter, unit-tested directly since no existing test drives run()
// against the real filesystem/module cache.
func TestWorthReporting_KeepsZeroGroundTruthServiceWithWarnings(t *testing.T) {
	t.Parallel()

	require.True(t, worthReporting(serviceScan{OpsGroundTruth: 0, Warnings: []string{"UNTRACEABLE"}}),
		"a zero-ground-truth service with a warning must still be reported")
	require.False(t, worthReporting(serviceScan{OpsGroundTruth: 0}),
		"a service with truly nothing to report must still be dropped")
	require.True(t, worthReporting(serviceScan{OpsGroundTruth: 5}),
		"a normal scanned service must still be reported")
}

// TestScanServiceDir_RealCorpus_WarningsBranchReachable is gopherstack-84mn's
// regression guard: worthReporting's `len(sr.Warnings) > 0` disjunct
// (main.go:548) must be reachable running the REAL production pipeline
// (scanServiceDir -> resolveServiceModules/buildServiceModuleTruth/
// buildPkgIndex) against this repo's actual services/ tree, not only against
// TestWorthReporting_KeepsZeroGroundTruthServiceWithWarnings' fabricated
// serviceScan literals. sqs's own pinned SDK module (go.mod's current pin)
// generates no deserializers.go file at all (newer smithy schema-based
// codegen, gopherstack-zkpi's class) so its real scan has OpsGroundTruth==0;
// without the disjunct, run() would silently drop it and its UNTRACEABLE
// warning would never print. Confirmed as of this filing that acm, amplify,
// codedeploy, codepipeline, route53resolver, transcribe, and workspaces hit
// this exact same real-corpus case -- re-point this test at one of those if
// a future go.mod bump ever gives sqs a deserializers.go file.
func TestScanServiceDir_RealCorpus_WarningsBranchReachable(t *testing.T) {
	t.Parallel()

	repoRoot, err := repoRootDir()
	require.NoError(t, err)

	cache, err := gomodcacheDir(repoRoot)
	require.NoError(t, err)

	goModVersions, err := loadGoModVersions(filepath.Join(repoRoot, "go.mod"))
	require.NoError(t, err)

	sr, err := scanServiceDir(filepath.Join(repoRoot, "services", "sqs"), repoRoot, cache, goModVersions)
	require.NoError(t, err)

	require.Zero(t, sr.OpsGroundTruth,
		"sqs's real pinned SDK module must have zero per-op ground truth for this case to be load-bearing")
	require.Contains(t, sr.ModulesNoOpFuncs, "sqs")
	require.NotEmpty(t, sr.Warnings)
	require.Contains(t, sr.Warnings[0], "UNTRACEABLE")
	require.True(t, worthReporting(sr),
		"worthReporting's warnings disjunct must keep this real, zero-ground-truth service reachable to run()")
}

// fixtureInternalServerException and fixtureValidationException name-share
// mgn's own two confirmed-generic codes (gopherstack-mq6m) across the
// fixtures below, rather than repeating the literal -- goconst's count is
// package-wide, not per-file, and repeating these two particular strings a
// few more times tips genericcodes.go's own pre-existing occurrences over
// its threshold.
const (
	fixtureInternalServerException = "InternalServerException"
	fixtureValidationException     = "ValidationException"
)

// mgnShapedFindings reproduces gopherstack-mq6m's confirmed shape: 3 ops
// funnel InternalServerException through one shared site
// (handler.go:372), and one of those 3 also emits ValidationException from
// its own dedicated line (applications.go:27) -- the exact fan-out/fan-in
// mix that makes site grouping worth doing at all.
func mgnShapedFindings() []finding {
	shared := evidenceSite{
		File: "services/mgn/handler.go", Line: 372,
		Mechanism: "constructor classifier: internalServerError",
	}
	own := evidenceSite{
		File: "services/mgn/applications.go", Line: 27,
		Mechanism: "constructor classifier: validationError",
	}

	return []finding{
		{Op: "ArchiveApplication", Code: fixtureInternalServerException, Sites: []evidenceSite{shared}},
		{Op: "CreateApplication", Code: fixtureInternalServerException, Sites: []evidenceSite{shared}},
		{Op: "CreateApplication", Code: fixtureValidationException, Sites: []evidenceSite{own}},
		{Op: "DeleteJob", Code: fixtureInternalServerException, Sites: []evidenceSite{shared}},
	}
}

// TestGroupFindingsBySite_CollapsesSharedSite is gopherstack-2evc's core
// ask: report.go:341's printSiteGroups (via groupFindingsBySite,
// report.go:246) must show handler.go:372 ONCE with 3 ops behind it, not 3
// independent findings -- exactly the collapse gopherstack-mq6m had to do
// by hand across 122 findings.
func TestGroupFindingsBySite_CollapsesSharedSite(t *testing.T) {
	t.Parallel()

	groups := groupFindingsBySite(mgnShapedFindings())

	require.Len(t, groups, 2, "3 findings at one shared site plus 1 at its own site must collapse to 2 groups, not 4")

	shared := groups[0]
	require.Equal(t, "services/mgn/handler.go", shared.File)
	require.Equal(t, 372, shared.Line)
	require.Equal(t, fixtureInternalServerException, shared.Code)
	require.Equal(t, []string{"ArchiveApplication", "CreateApplication", "DeleteJob"}, shared.Ops)

	own := groups[1]
	require.Equal(t, "services/mgn/applications.go", own.File)
	require.Equal(t, []string{"CreateApplication"}, own.Ops)
}

// TestGroupFindingsBySite_SortedByOpCountDescending: the funnel point
// belongs at the top of the section (report.go:287's comment), not buried
// alphabetically among singletons a reader has to scan past first.
func TestGroupFindingsBySite_SortedByOpCountDescending(t *testing.T) {
	t.Parallel()

	groups := groupFindingsBySite(mgnShapedFindings())

	require.Len(t, groups, 2)
	require.GreaterOrEqual(t, len(groups[0].Ops), len(groups[1].Ops),
		"the site reached by more ops must sort first")
}

// TestGroupFindingsBySite_AcceptedByCarriedFromSite verifies AcceptedBy
// survives the collapse -- helpers.go's siblingsAccepting is invariant per
// (module, code), so any finding folded into the group carries the same
// list, and dropping it during grouping would silently lose the "declared
// correctly by" evidence line.
func TestGroupFindingsBySite_AcceptedByCarriedFromSite(t *testing.T) {
	t.Parallel()

	findings := []finding{
		{
			Op:   "ArchiveApplication",
			Code: fixtureInternalServerException,
			Sites: []evidenceSite{
				{File: "services/mgn/handler.go", Line: 372, Mechanism: "constructor classifier: internalServerError"},
			},
			AcceptedBy: []string{"ListTagsForResource", "TagResource"},
		},
	}

	groups := groupFindingsBySite(findings)

	require.Len(t, groups, 1)
	require.Equal(t, []string{"ListTagsForResource", "TagResource"}, groups[0].AcceptedBy)
}

// TestIsSharedPlumbing_ThresholdFromCorpus pins report.go:316's
// sharedPlumbingRatio (0.25) against the actual corpus values gopherstack-
// 2evc's design decision rests on: mgn's two gopherstack-mq6m-confirmed
// generic sites (90/95=0.947, 32/95=0.337) must fire, and the highest
// ratio among every OTHER site group in the full 160-service corpus --
// cloudfront's own real-but-narrower-scoped quantity_validation.go:56 at
// 27/167=0.162 -- must not. A regression here would either stop flagging
// mgn's confirmed funnel points or start flagging cloudfront's, both of
// which changed WHAT is found, which gopherstack-2evc forbids.
func TestIsSharedPlumbing_ThresholdFromCorpus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		nOps        int
		opsResolved int
		want        bool
	}{
		{"mgn marshalResponse 90/95", 90, 95, true},
		{"mgn decodeJSONBody 32/95", 32, 95, true},
		{"cloudfront quantity validator 27/167", 27, 167, false},
		{"kms grant token check 5/54", 5, 54, false},
		{"small N below op-count guard", 2, 4, false},
		{"zero resolved", 3, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, isSharedPlumbing(tt.nOps, tt.opsResolved))
		})
	}
}

// quantityValidationFixture reproduces cloudfront's real duplication
// (gopherstack-s0dw): validateQuantities wraps its sentinel via fmt.Errorf,
// and BOTH AdjustQuantity and ReviseQuantity call it directly from their own
// hop-0 body, so each op's walk both (a) sees the call itself at hop 0
// ("constructor classifier: validateQuantities") and (b) recurses one hop
// into validateQuantities' own body and finds the same bare sentinel return
// there ("sentinel reference") -- the two mechanisms' op sets come out
// identical, not just overlapping.
const quantityValidationFixture = `
package fixture

import (
	"errors"
	"fmt"
)

var ErrBadQuantity = errors.New("bad quantity")

func classifyQuantityError(err error) string {
	switch {
	case errors.Is(err, ErrBadQuantity):
		return "InconsistentQuantities"
	}
	return "InternalServerException"
}

func validateQuantities(n int) error {
	if n < 0 {
		return fmt.Errorf("%w: negative quantity", ErrBadQuantity)
	}
	return nil
}

type Handler struct{}

func (h *Handler) AdjustQuantity() error {
	return validateQuantities(1)
}

func (h *Handler) ReviseQuantity() error {
	return validateQuantities(2)
}
`

func quantityValidationTruth() *serviceModuleTruth {
	return singleModuleTruth(newTestModuleGroundTruth(
		map[string]map[string]bool{
			"AdjustQuantity":               {},
			"ReviseQuantity":               {},
			"DeclareQuantityCodeElsewhere": {"InconsistentQuantities": true},
		},
		map[string]bool{"InconsistentQuantities": true, fixtureInternalServerException: true},
	))
}

// partialQuantityFixture is kms's real InvalidGrantTokenException/
// validateGrantTokenPresence shape (gopherstack-s0dw's measured "partial"
// case): DirectFix calls validateWidget straight from its own hop-0 body
// (both mechanisms fire), but IndirectFix reaches the SAME constructor only
// through Backend.DoIndirect -- one hop already spent getting there, so
// recursing a further hop into validateWidget's own body to find its
// sentinel return exceeds maxEmitHop, and IndirectFix never gets a
// "sentinel reference" row at all, only "constructor classifier".
const partialQuantityFixture = `
package fixture

import (
	"errors"
	"fmt"
)

var ErrBadWidget = errors.New("bad widget")

func classifyWidgetError(err error) string {
	switch {
	case errors.Is(err, ErrBadWidget):
		return "BadWidgetException"
	}
	return "InternalServerException"
}

func validateWidget(n int) error {
	if n < 0 {
		return fmt.Errorf("%w: negative widget", ErrBadWidget)
	}
	return nil
}

type Handler struct {
	Backend *Backend
}

func (h *Handler) DirectFix() error {
	return validateWidget(1)
}

func (h *Handler) IndirectFix() error {
	return h.Backend.DoIndirect()
}

type Backend struct{}

func (b *Backend) DoIndirect() error {
	return validateWidget(2)
}
`

func partialQuantityTruth() *serviceModuleTruth {
	return singleModuleTruth(newTestModuleGroundTruth(
		map[string]map[string]bool{
			"DirectFix":                  {},
			"IndirectFix":                {},
			"DeclareWidgetCodeElsewhere": {"BadWidgetException": true},
		},
		map[string]bool{"BadWidgetException": true, fixtureInternalServerException: true},
	))
}

// captureStdout redirects os.Stdout for fn's duration -- printSiteGroups
// (report.go) writes there directly, with no injectable writer. Not run
// t.Parallel(): a later top-level test mutating the package-global
// os.Stdout must not race a still-pending parallel one, and it does not,
// because every t.Parallel() test in this file pauses at that call until
// every top-level test (this one included) has been started and returned.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	require.NoError(t, err)

	orig := os.Stdout
	os.Stdout = w //nolint:reassign // captured and restored below; see this func's own doc comment

	fn()

	require.NoError(t, w.Close())
	os.Stdout = orig //nolint:reassign // restoring the original value

	out, err := io.ReadAll(r)
	require.NoError(t, err)

	return string(out)
}

// TestPrintSiteGroups_RollupTag_ExactMatch is gopherstack-s0dw's own
// regression guard: report.go's printSiteGroups (unchanged signature, so
// this test also compiles against the pre-fix source) must flag
// validateQuantities' shared definition-site row as a ROLLUP of the two
// call-site rows below it, since AdjustQuantity/ReviseQuantity's op sets on
// both sides come out identical -- before this fix, printSiteGroups had no
// such concept, and this line does not appear at all.
//
//nolint:paralleltest // mutates the package-global os.Stdout; see captureStdout's own doc comment
func TestPrintSiteGroups_RollupTag_ExactMatch(t *testing.T) {
	idx := parseSrc(t, quantityValidationFixture)
	sr := scanWithIndex("fixture", []string{"fixture"}, "/repo", idx, quantityValidationTruth())

	require.Len(t, sr.Findings, 2, "both ops must be flagged for the misplaced code")

	out := captureStdout(t, func() {
		printSiteGroups(sr.Findings, sr.OpsResolved)
	})

	require.Contains(t, out, "ROLLUP: same ops as validateQuantities's own call-site row(s)")
	require.NotContains(t, out, "PARTIAL ROLLUP")
}

// TestPrintSiteGroups_RollupTag_PartialSubset is the other confirmed corpus
// shape (kms's validateGrantTokenPresence): the definition-site row's op
// set is a proper SUBSET of its constructor's own call-site rows, not
// equal, so it must be flagged PARTIAL ROLLUP -- collapsing it outright
// would silently drop IndirectFix's own evidence.
//
//nolint:paralleltest // mutates the package-global os.Stdout; see captureStdout's own doc comment
func TestPrintSiteGroups_RollupTag_PartialSubset(t *testing.T) {
	idx := parseSrc(t, partialQuantityFixture)
	sr := scanWithIndex("fixture", []string{"fixture"}, "/repo", idx, partialQuantityTruth())

	require.Len(t, sr.Findings, 2, "both ops must be flagged for the misplaced code")

	out := captureStdout(t, func() {
		printSiteGroups(sr.Findings, sr.OpsResolved)
	})

	require.Contains(t, out, "PARTIAL ROLLUP: a subset of validateWidget's own call-site row(s)")
}

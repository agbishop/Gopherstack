package main

import (
	"go/ast"
	"go/token"
	"maps"
	"regexp"
	"strconv"
	"strings"
)

// pkgErrors is the stdlib "errors" package identifier this file and emit.go
// both match against (errors.Is, errors.New) when recognizing a call's
// package qualifier.
const pkgErrors = "errors"

// codeShapeRe separates an AWS-style error code ("ResourceNotFoundException",
// "ConflictException") from every other string literal a mapper branch's
// body might contain (a human-readable message, a header name) --
// PascalCase-or-SCREAMING, no spaces/punctuation, at least 3 characters.
// Identical filter to cmd/errcodeaudit/extract.go's codeShapeRe.
var codeShapeRe = regexp.MustCompile(`^[A-Z][A-Za-z0-9]{2,}$`)

func looksLikeCode(s string) bool {
	return codeShapeRe.MatchString(s)
}

// classifiers is the package-wide map from an error-emission SOURCE (a
// sentinel variable's name, or a constructor function's name) to the wire
// code it renders as -- built once per service and shared across every
// operation's emission walk (emit.go). See this package's doc comment for
// why this table, not a per-call-site literal, is the right ground truth
// for most of this repo's real shape: services/bedrock and services/iot
// emit a SENTINEL (ErrAlreadyExists, ErrThingNotFound, ...), never a code
// literal, at the actual bug site -- the literal only ever appears once,
// in a shared mapper function every operation in the package funnels
// through.
type classifiers struct {
	Sentinels    map[string]string
	ByFunc       map[string]map[string]string
	Funcs        map[string]string
	Overrides    map[string]overrideFunc
	GuardsByPos  map[token.Pos]guard
	SentinelMeta map[string]sentinelMeta
	MapperNames  map[string]bool
	Constructors []*ast.FuncDecl
}

// overrideFunc is a helper like services/iot's respondAsInvalidRequest(c,
// err, sentinel error) -- a function that takes the COMPARISON sentinel as
// its OWN parameter rather than a fixed identifier, and emits Code
// specifically when errors.Is(err, thatParam) holds. ParamIndex is the
// flattened parameter position of the comparison argument, so a call site
// passing a literal sentinel there can be resolved without knowing the
// helper's implementation.
type overrideFunc struct {
	Code       string
	ParamIndex int
}

// buildClassifiers finds the package's own errors.Is-to-code mapper(s)
// (sentinelCodes) and propagates through one hop of constructor-function
// indirection (funcCodes) -- services/networkmanager's real shape:
// notFoundError(...) never mentions a code literal itself, it builds
// &apiError{cause: errNotFoundSentinel, ...}, and errNotFoundSentinel is
// what the package's real mapper (classifyError) associates with
// "ResourceNotFoundException". A constructor that wraps ANOTHER constructor,
// rather than a sentinel directly, is not resolved -- disclosed in the
// package doc as a blind spot, matching this repo's other tools' one-hop
// discipline.
//
// opNames excludes every function whose OWN name matches a real ground-truth
// operation name from constructor candidacy -- an ordinary backend method
// (`func (b *Backend) DeleteThing(id string) error`) also returns bare
// `error` and would otherwise be misread as a small error-builder helper,
// double-counting its own hop-1 emission under a second mechanism AND, worse,
// bypassing emit.go's per-op override suppression entirely (that helper's
// code is baked in at buildClassifiers time, before any op-specific override
// is known). A real constructor (notFoundError, validationError,
// conflictError) is never itself named after an AWS operation; a backend
// method implementing one always is -- confirmed as a false positive on a
// synthetic CancelJob-shaped fixture during this tool's own test-writing.
func buildClassifiers(idx *pkgIndex, opNames map[string]bool) *classifiers {
	byFunc := funcSentinelCodes(idx)
	flat := flattenSentinelCodes(byFunc)

	guardsByPos, mapperNames := buildGuardIndex(idx)

	c := &classifiers{
		Sentinels:    flat,
		ByFunc:       byFunc,
		Overrides:    detectOverrideFuncs(idx),
		GuardsByPos:  guardsByPos,
		SentinelMeta: buildSentinelMeta(idx),
		MapperNames:  mapperNames,
	}

	for _, f := range idx.Files {
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil || opNames[fd.Name.Name] || !returnsOnlyError(fd) {
				continue
			}

			c.Constructors = append(c.Constructors, fd)
		}
	}

	c.Funcs = resolveConstructorCodes(c.Constructors, flat)

	return c
}

func resolveConstructorCodes(
	candidates []*ast.FuncDecl,
	sentinels map[string]string,
) map[string]string {
	out := map[string]string{}

	for _, fd := range candidates {
		if code, found := constructorCode(fd, sentinels); found {
			out[fd.Name.Name] = code
		}
	}

	return out
}

// funcSentinelCodes scans every switch statement and if-statement in the
// package for an errors.Is(<err>, <sentinel>) condition whose branch body
// contains a code-shaped literal, associating the sentinel's own name with
// that code -- SCOPED per enclosing mapper function (gopherstack-0yva),
// unlike a single package-wide table: services/eks's real shape has two
// mapper functions, handleError and handleTagError, that both branch on the
// SAME identifier ErrNotFound to DIFFERENT codes (ResourceNotFoundException
// vs NotFoundException, a real, deliberate difference between the two
// tagging-API families' own deserializers), and a flat table keyed by
// identifier alone can only record one winner -- silently misattributing
// every operation reachable through the LOSING mapper. Every switch/if found
// inside one FuncDecl's body contributes to THAT function's own table;
// flattenSentinelCodes below builds the package-wide fallback used only when
// a call site's own mapper cannot be determined (emit.go's
// localMapperScope).
func funcSentinelCodes(idx *pkgIndex) map[string]map[string]string {
	out := map[string]map[string]string{}

	collect := func(name string, body *ast.BlockStmt) {
		if body == nil {
			return
		}

		table := map[string]string{}

		ast.Inspect(body, func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.SwitchStmt:
				addSwitchSentinelCodes(v, idx, table)
			case *ast.IfStmt:
				addIfSentinelCodes(v, idx, table)
			case *ast.RangeStmt:
				addRangeTableSentinelCodes(v, idx, body, table)
			}

			return true
		})

		if len(table) == 0 {
			return
		}

		if existing, ok := out[name]; ok {
			maps.Copy(existing, table)
		} else {
			out[name] = table
		}
	}

	for _, fd := range idx.Funcs {
		collect(fd.Name.Name, fd.Body)
	}

	for name, fds := range idx.Methods {
		for _, fd := range fds {
			collect(name, fd.Body)
		}
	}

	return out
}

// flattenSentinelCodes merges every mapper function's own table (built by
// funcSentinelCodes) into one package-wide fallback -- used only when an
// operation's own call path cannot be pinned to a specific mapper
// (emit.go's localMapperScope finds none reachable). When two DIFFERENT
// mapper functions map the SAME identifier to DIFFERENT codes, that
// identifier is a COLLISION: dropped from the flat map entirely, never
// silently resolved to whichever mapper this scan happened to visit
// first -- this is deterministic regardless of map iteration order, because
// any two DIFFERING values for the same identifier mark it a collision
// however the functions are visited (verified in
// TestFlattenSentinelCodes_CollisionOmitted). gopherstack-0yva's other,
// preferred resolution -- resolving through the mapper an operation's OWN
// call path actually reaches -- lives in emit.go's localMapperScope, and
// wins over this fallback whenever it finds one.
func flattenSentinelCodes(byFunc map[string]map[string]string) map[string]string {
	out := map[string]string{}
	collide := map[string]bool{}

	for _, table := range byFunc {
		for ident, code := range table {
			prev, seen := out[ident]
			if !seen {
				out[ident] = code

				continue
			}

			if prev != code {
				collide[ident] = true
			}
		}
	}

	for ident := range collide {
		delete(out, ident)
	}

	return out
}

// sentinelCodes is funcSentinelCodes's flat, package-wide view -- kept as
// its own entry point because it is the shape most of this file's own
// resolution (constructorCode's default candidacy, this package's tests)
// needs, and because a package with exactly one mapper (the common case)
// never triggers the ambiguity flattenSentinelCodes exists to catch.
func sentinelCodes(idx *pkgIndex) map[string]string {
	return flattenSentinelCodes(funcSentinelCodes(idx))
}

func addSwitchSentinelCodes(sw *ast.SwitchStmt, idx *pkgIndex, out map[string]string) {
	for _, stmt := range sw.Body.List {
		cc, ok := stmt.(*ast.CaseClause)
		if !ok {
			continue
		}

		names := map[string]bool{}
		for _, expr := range cc.List {
			collectErrorsIsSentinels(expr, idx.Sentinels, names)
		}

		if len(names) == 0 {
			continue
		}

		code, found := firstCodeLiteral(&ast.BlockStmt{List: cc.Body}, idx, 0)
		if !found {
			continue
		}

		for name := range names {
			out[name] = code
		}
	}
}

func addIfSentinelCodes(ifs *ast.IfStmt, idx *pkgIndex, out map[string]string) {
	names := map[string]bool{}
	collectErrorsIsSentinels(ifs.Cond, idx.Sentinels, names)

	if len(names) == 0 || ifs.Body == nil {
		return
	}

	code, found := firstCodeLiteral(ifs.Body, idx, 0)
	if !found {
		return
	}

	for name := range names {
		out[name] = code
	}
}

// maxTableResolveHop bounds how far addRangeTableSentinelCodes follows a
// range loop's source expression before giving up: a local var assigned in
// the SAME function (services/autoscaling's own shape, `mappings :=
// []errorMapping{...}` inside autoscalingErrorCode), a package-level var
// (services/route53's package-scoped backendErrorTable), or a zero-arg
// package-local function call that returns the literal directly
// (services/kms's kmsErrorTable()) -- each one hop, so a function whose OWN
// table is itself another function call still resolves. Does NOT follow an
// append(...) composition (services/s3's errorTable(), which concatenates
// three sub-tables) or a sync.OnceValue-wrapped closure
// (services/servicediscovery's sentinelErrorCodes) -- both stay a
// disclosed blind spot, caught loudly by coverageWarnings' zero-emission
// guard rather than silently.
const maxTableResolveHop = 4

// addRangeTableSentinelCodes recognises services/kms, services/sqs,
// services/route53 and roughly two dozen other services' own mapper shape:
// `for _, m := range table { if errors.Is(err, m.sentinel) { ... m.code
// ... } }` -- a runtime loop over a data table, invisible to
// addSwitchSentinelCodes/addIfSentinelCodes because the comparison's second
// argument is a SELECTOR (m.sentinel), never a bare sentinel identifier.
// Only fires when the loop body actually guards on errors.Is against a
// selector rooted at the range loop's OWN value variable -- an ordinary
// range loop that happens to iterate a slice is not, by itself, evidence of
// an error-code table.
func addRangeTableSentinelCodes(rs *ast.RangeStmt, idx *pkgIndex, body *ast.BlockStmt, out map[string]string) {
	valueName, ok := rangeValueName(rs)
	if !ok || rs.Body == nil || !rangeGuardsErrorsIs(rs.Body, valueName) {
		return
	}

	for _, row := range resolveTableRows(rs.X, idx, body, 0) {
		addRowSentinelCode(row, idx, out)
	}
}

func rangeValueName(rs *ast.RangeStmt) (string, bool) {
	id, ok := rs.Value.(*ast.Ident)
	if !ok || id.Name == "_" {
		return "", false
	}

	return id.Name, true
}

// rangeGuardsErrorsIs reports whether body contains an errors.Is(<x>,
// <valueName>.<field>) call -- the loop variable's OWN field, not a bare
// identifier (that shape is addIfSentinelCodes' job, not this one's).
func rangeGuardsErrorsIs(body *ast.BlockStmt, valueName string) bool {
	found := false

	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}

		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 2 {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Is" {
			return true
		}

		pkgIdent, ok := sel.X.(*ast.Ident)
		if !ok || pkgIdent.Name != pkgErrors {
			return true
		}

		argSel, ok := call.Args[1].(*ast.SelectorExpr)
		if !ok {
			return true
		}

		argIdent, ok := argSel.X.(*ast.Ident)
		if ok && argIdent.Name == valueName {
			found = true

			return false
		}

		return true
	})

	return found
}

// resolveTableRows follows expr -- a range loop's source, or one hop of
// indirection from it -- to the row literals it ultimately names: a slice
// composite literal directly, a local variable defined earlier in the SAME
// function body, a package-level variable, or a zero-arg package-local
// function's own (single, non-composed) returned literal.
func resolveTableRows(expr ast.Expr, idx *pkgIndex, body *ast.BlockStmt, hop int) []*ast.CompositeLit {
	if hop > maxTableResolveHop {
		return nil
	}

	switch e := expr.(type) {
	case *ast.CompositeLit:
		return compositeLitRows(e)
	case *ast.Ident:
		return resolveTableIdent(e.Name, idx, body, hop)
	case *ast.CallExpr:
		return resolveTableCall(e, idx, hop)
	default:
		return nil
	}
}

func resolveTableIdent(name string, idx *pkgIndex, body *ast.BlockStmt, hop int) []*ast.CompositeLit {
	if body != nil {
		if rhs, ok := resolveLocalVar(body, name); ok {
			return resolveTableRows(rhs, idx, body, hop+1)
		}
	}

	if rhs, ok := idx.PkgVars[name]; ok {
		return resolveTableRows(rhs, idx, nil, hop+1)
	}

	return nil
}

func resolveTableCall(call *ast.CallExpr, idx *pkgIndex, hop int) []*ast.CompositeLit {
	name, ok := calleeSimpleName(call.Fun)
	if !ok || len(call.Args) != 0 {
		return nil
	}

	fd, ok := idx.Funcs[name]
	if !ok || fd.Body == nil {
		return nil
	}

	var out []*ast.CompositeLit

	for _, ret := range funcReturnExprs(fd.Body) {
		out = append(out, resolveTableRows(ret, idx, fd.Body, hop+1)...)
	}

	return out
}

// funcReturnExprs collects every top-level ReturnStmt's own single result
// expression in fd's body -- deliberately not recursing into nested
// FuncLits, whose own returns belong to a different function.
func funcReturnExprs(body *ast.BlockStmt) []ast.Expr {
	var out []ast.Expr

	ast.Inspect(body, func(n ast.Node) bool {
		if _, isFuncLit := n.(*ast.FuncLit); isFuncLit {
			return false
		}

		ret, ok := n.(*ast.ReturnStmt)
		if ok && len(ret.Results) == 1 {
			out = append(out, ret.Results[0])
		}

		return true
	})

	return out
}

// resolveLocalVar finds `name := <expr>` defined anywhere in body -- the
// SAME function the range loop itself lives in (services/autoscaling's
// `mappings := []errorMapping{...}` inside autoscalingErrorCode).
func resolveLocalVar(body *ast.BlockStmt, name string) (ast.Expr, bool) {
	var result ast.Expr

	var found bool

	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}

		as, ok := n.(*ast.AssignStmt)
		if !ok || as.Tok != token.DEFINE {
			return true
		}

		for i, lhs := range as.Lhs {
			id, idOK := lhs.(*ast.Ident)
			if idOK && id.Name == name && i < len(as.Rhs) {
				result, found = as.Rhs[i], true

				return false
			}
		}

		return true
	})

	return result, found
}

func compositeLitRows(cl *ast.CompositeLit) []*ast.CompositeLit {
	var out []*ast.CompositeLit

	for _, elt := range cl.Elts {
		if row, ok := elt.(*ast.CompositeLit); ok {
			out = append(out, row)
		}
	}

	return out
}

// maxTableRowHop bounds how far addRowSentinelCode descends into a row's
// own nested composite literal before giving up -- services/s3's real shape
// needs exactly one: {ErrNoSuchBucket, s3ErrorInfo{"NoSuchBucket", "...",
// http.StatusNotFound}} nests its code one level inside the row.
const maxTableRowHop = 1

// addRowSentinelCode pairs the sentinel identifier this scan already knows
// about (idx.Sentinels) with the code-shaped string literal(s) sitting
// beside it in the SAME row -- positional ({ErrKeyNotFound, awsErrNotFound})
// or keyed ({sentinel: ErrX, awsType: "Y"}); field NAME is deliberately
// ignored, since it varies across every one of this shape's confirmed
// instances (kms's own field is "awsType"; route53's and sqs's rows are
// positional, no field name at all). Only the row's OWN top-level elements
// are checked for the sentinel -- not a nested composite literal -- because
// every confirmed instance keeps the sentinel as a direct sibling field.
func addRowSentinelCode(row *ast.CompositeLit, idx *pkgIndex, out map[string]string) {
	sentinel, ok := rowSentinelName(row, idx)
	if !ok {
		return
	}

	codes := rowCodeLiterals(row, 0)
	if len(codes) == 0 {
		return
	}

	out[sentinel] = codes[0]
}

func rowSentinelName(row *ast.CompositeLit, idx *pkgIndex) (string, bool) {
	for _, elt := range row.Elts {
		id, ok := eltValue(elt).(*ast.Ident)
		if ok && idx.Sentinels[id.Name] {
			return id.Name, true
		}
	}

	return "", false
}

func rowCodeLiterals(row *ast.CompositeLit, hop int) []string {
	var out []string

	for _, elt := range row.Elts {
		switch e := eltValue(elt).(type) {
		case *ast.BasicLit:
			if code, ok := rowCodeLiteral(e); ok {
				out = append(out, code)
			}
		case *ast.CompositeLit:
			if hop < maxTableRowHop {
				out = append(out, rowCodeLiterals(e, hop+1)...)
			}
		}
	}

	return out
}

// rowCodeLiteral mirrors literalCode, but additionally strips a smithy
// AWS-JSON-protocol namespace prefix ("com.amazonaws.sqs#") before applying
// the code-shape filter -- services/sqs's own wire literal for its error
// table's code field, matching the BARE name aws-sdk-go-v2's own
// ErrorCode() method returns (service/sqs/types/errors.go), which is what
// this scan's ground truth is keyed by.
func rowCodeLiteral(lit *ast.BasicLit) (string, bool) {
	if lit.Kind != token.STRING {
		return "", false
	}

	v, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}

	if i := strings.LastIndexByte(v, '#'); i >= 0 {
		v = v[i+1:]
	}

	if !looksLikeCode(v) {
		return "", false
	}

	return v, true
}

func eltValue(elt ast.Expr) ast.Expr {
	if kv, ok := elt.(*ast.KeyValueExpr); ok {
		return kv.Value
	}

	return elt
}

// collectErrorsIsSentinels finds every errors.Is(<x>, <sentinel>) call
// reachable in expr (an entire case-list entry, or an if's -- possibly
// &&/||-combined -- condition) whose second argument is a known sentinel
// identifier.
func collectErrorsIsSentinels(expr ast.Expr, sentinels map[string]bool, out map[string]bool) {
	ast.Inspect(expr, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 2 {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Is" {
			return true
		}

		pkgIdent, ok := sel.X.(*ast.Ident)
		if !ok || pkgIdent.Name != pkgErrors {
			return true
		}

		id, ok := call.Args[1].(*ast.Ident)
		if ok && sentinels[id.Name] {
			out[id.Name] = true
		}

		return true
	})
}

// maxLiteralHop bounds how far firstCodeLiteral follows a mapper branch's
// own call into another package-local function before giving up --
// services/iot's real shape needs exactly one: writeIoTError's
// ResourceNotFoundException branch is `return respondNotFound(c,
// err.Error())`, and the literal "ResourceNotFoundException" lives inside
// respondNotFound's OWN body, not the branch that calls it.
const maxLiteralHop = 1

// firstCodeLiteral returns the first code-shaped value found anywhere in n,
// in AST traversal order: a direct string literal, a bare identifier
// resolving to a package-level string const (services/iot's
// errTypeInvalidRequest), or -- up to maxLiteralHop -- a call to a
// package-local function/method, recursed into for the same two shapes.
func firstCodeLiteral(n ast.Node, idx *pkgIndex, hop int) (string, bool) {
	var found string

	var ok bool

	ast.Inspect(n, func(node ast.Node) bool {
		if ok {
			return false
		}

		if code, matched := codeLiteralAtNode(node, idx, hop); matched {
			found, ok = code, true

			return false
		}

		return true
	})

	return found, ok
}

// codeLiteralAtNode checks node itself (not its children -- ast.Inspect's
// own traversal covers those) for one of firstCodeLiteral's three shapes.
func codeLiteralAtNode(node ast.Node, idx *pkgIndex, hop int) (string, bool) {
	switch v := node.(type) {
	case *ast.BasicLit:
		return literalCode(v)
	case *ast.Ident:
		if code, matched := idx.PkgConsts[v.Name]; matched && looksLikeCode(code) {
			return code, true
		}
	case *ast.CallExpr:
		if hop < maxLiteralHop {
			return firstCalleeCodeLiteral(v.Fun, idx, hop)
		}
	}

	return "", false
}

func firstCalleeCodeLiteral(fn ast.Expr, idx *pkgIndex, hop int) (string, bool) {
	for _, fd := range calleeFuncDecls(fn, idx) {
		if fd.Body == nil {
			continue
		}

		if code, matched := firstCodeLiteral(fd.Body, idx, hop+1); matched {
			return code, true
		}
	}

	return "", false
}

func literalCode(lit *ast.BasicLit) (string, bool) {
	if lit.Kind != token.STRING {
		return "", false
	}

	v, err := strconv.Unquote(lit.Value)
	if err != nil || !looksLikeCode(v) {
		return "", false
	}

	return v, true
}

// returnsOnlyError reports whether fd declares EXACTLY one result, the
// built-in `error` type -- the shape every constructor in this repo's
// mapper-adjacent files (notFoundError, validationError, conflictError, ...)
// shares. Deliberately narrower than "last result is error": an ordinary
// backend method (`func (b *InMemoryBackend) CancelJob(...) (*Job, error)`)
// also ends in error but is not a constructor, and treating it as one
// double-counted a finding through both the "constructor classifier" and
// "sentinel reference" mechanisms during this tool's own validation pass,
// confirmed on services/iot's CancelJob before this narrowing.
func returnsOnlyError(fd *ast.FuncDecl) bool {
	if fd.Type.Results == nil || len(fd.Type.Results.List) != 1 {
		return false
	}

	field := fd.Type.Results.List[0]
	if len(field.Names) > 1 {
		return false
	}

	id, ok := field.Type.(*ast.Ident)

	return ok && id.Name == "error"
}

// constructorCode inspects fd's own return statements (including nested
// composite-literal field values and fmt.Errorf's %w slot) for a bare
// reference to a known sentinel, one hop of indirection past sentinelCodes
// itself.
func constructorCode(fd *ast.FuncDecl, sentinelCodes map[string]string) (string, bool) {
	var found string

	var ok bool

	ast.Inspect(fd.Body, func(n ast.Node) bool {
		if ok {
			return false
		}

		ret, isRet := n.(*ast.ReturnStmt)
		if !isRet {
			return true
		}

		for _, result := range ret.Results {
			if code, matched := sentinelRefCode(result, sentinelCodes); matched {
				found, ok = code, true

				return false
			}
		}

		return true
	})

	return found, ok
}

// sentinelRefCode reports whether expr is, or directly carries, a bare
// reference to a known sentinel: the expression itself, a unary `&`, a
// composite literal's own field values (services/networkmanager's
// `&apiError{cause: errNotFoundSentinel, ...}` shape, recursed into nested
// composite literals), or an argument to fmt.Errorf specifically (the
// `fmt.Errorf("%w: ...", ErrX, ...)` wrap idiom). It deliberately does NOT
// descend into the arguments of any OTHER call: services/iot's real
// post-fix shape, `respondAsInvalidRequest(c, err, ErrInvalidStateTransition)`,
// passes a sentinel as a COMPARISON target (errors.Is(err, thatParam)
// inside the callee), not as the value being emitted -- a confirmed false
// positive during this tool's own validation pass before this exclusion was
// added (classifiers.go's own doc comment records the concrete instance).
func sentinelRefCode(expr ast.Expr, sentinelCodes map[string]string) (string, bool) {
	switch e := expr.(type) {
	case *ast.Ident:
		if code, ok := sentinelCodes[e.Name]; ok {
			return code, true
		}
	case *ast.UnaryExpr:
		if e.Op == token.AND {
			return sentinelRefCode(e.X, sentinelCodes)
		}
	case *ast.CompositeLit:
		return sentinelRefCodeInElts(e.Elts, sentinelCodes)
	case *ast.CallExpr:
		if isFmtErrorfCall(e) {
			return sentinelRefCodeInArgs(e.Args, sentinelCodes)
		}
	}

	return "", false
}

func sentinelRefCodeInElts(elts []ast.Expr, sentinelCodes map[string]string) (string, bool) {
	for _, elt := range elts {
		v := elt
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			v = kv.Value
		}

		if code, ok := sentinelRefCode(v, sentinelCodes); ok {
			return code, true
		}
	}

	return "", false
}

func sentinelRefCodeInArgs(args []ast.Expr, sentinelCodes map[string]string) (string, bool) {
	for _, a := range args {
		if code, ok := sentinelRefCode(a, sentinelCodes); ok {
			return code, true
		}
	}

	return "", false
}

func isFmtErrorfCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	pkgIdent, ok := sel.X.(*ast.Ident)

	return ok && pkgIdent.Name == "fmt" && sel.Sel.Name == "Errorf"
}

// detectOverrideFuncs finds every package-level function shaped like
// services/iot's respondAsInvalidRequest(c, err, sentinel error): it takes
// the comparison sentinel as ITS OWN parameter (rather than a fixed package
// identifier) and, in an `if errors.Is(<x>, <thatParam>) { ... }` branch,
// emits a fixed code. Detecting this matters for PRECISION, not recall: a
// service that only ever uses such a helper post-fix (this repo's own
// pattern for the fix commits this tool validates against) would otherwise
// have its call sites misread as still emitting the PRE-fix, general
// mapper's code -- confirmed as a false positive on services/iot's
// (post-fix) CancelJob/DeleteThing during this tool's own validation pass,
// before this detector was added.
func detectOverrideFuncs(idx *pkgIndex) map[string]overrideFunc {
	out := map[string]overrideFunc{}

	for _, f := range idx.Files {
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}

			params := flattenParamNames(fd.Type.Params)
			if ov, found := findOverrideShape(fd.Body, idx, params); found {
				out[fd.Name.Name] = ov
			}
		}
	}

	return out
}

func flattenParamNames(fl *ast.FieldList) []string {
	if fl == nil {
		return nil
	}

	var out []string

	for _, f := range fl.List {
		if len(f.Names) == 0 {
			out = append(out, "")

			continue
		}

		for _, n := range f.Names {
			out = append(out, n.Name)
		}
	}

	return out
}

func findOverrideShape(body *ast.BlockStmt, idx *pkgIndex, params []string) (overrideFunc, bool) {
	var result overrideFunc

	var found bool

	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}

		ifs, ok := n.(*ast.IfStmt)
		if !ok || ifs.Body == nil {
			return true
		}

		paramIdx, condOK := errorsIsParamIndex(ifs.Cond, params)
		if !condOK {
			return true
		}

		code, codeOK := firstCodeLiteral(ifs.Body, idx, 0)
		if !codeOK {
			return true
		}

		result, found = overrideFunc{ParamIndex: paramIdx, Code: code}, true

		return false
	})

	return result, found
}

// errorsIsParamIndex reports whether cond contains an errors.Is(<x>, <y>)
// call where y names one of fd's own parameters, returning that
// parameter's flattened index.
func errorsIsParamIndex(cond ast.Expr, params []string) (int, bool) {
	var result int

	var found bool

	ast.Inspect(cond, func(n ast.Node) bool {
		if found {
			return false
		}

		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 2 {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Is" {
			return true
		}

		pkgIdent, ok := sel.X.(*ast.Ident)
		if !ok || pkgIdent.Name != pkgErrors {
			return true
		}

		id, ok := call.Args[1].(*ast.Ident)
		if !ok {
			return true
		}

		for i, p := range params {
			if p == id.Name {
				result, found = i, true

				return false
			}
		}

		return true
	})

	return result, found
}

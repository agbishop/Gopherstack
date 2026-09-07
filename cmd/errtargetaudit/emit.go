package main

import (
	"go/ast"
	"go/token"
	"maps"
	"strconv"
	"strings"
)

// maxEmitHop bounds how far this scan follows a resolved handler's own
// calls into other package-local functions before giving up: hop 0 is the
// resolved root itself, hop 1 is any function or method it calls directly
// -- ANY receiver, not only this repo's uniform "h" Handler receiver name,
// because the real bug site in three of the four validated commits
// (d7149d0f8, 19f3d65f0) sits in the BACKEND method a handler calls, one
// hop away, never in the handler itself. This is deliberately WIDER than
// cmd/reqfieldscan/cmd/reqfielddiff's own single-hop discipline, which
// restricts recursion to "h.<Method>" specifically to keep a backend's
// internal FIELD names from leaking in as false "declared wire field"
// matches -- that hazard does not apply here: a backend method's own
// sentinel-error return IS exactly the site this tool exists to find.
const maxEmitHop = 1

// emission is one candidate error-code emission found reachable from an
// operation's resolved root(s).
type emission struct {
	Code      string
	Mechanism string
	// EnclosingFunc is the name of the hop-1 callee whose OWN body this
	// emission was found in ("" at hop 0, the op's own root) -- lets
	// report.go's rollup detection (gopherstack-s0dw) tell a "sentinel
	// reference" deep in a constructor's body apart from one sitting
	// directly in an operation's own code, without re-parsing anything.
	EnclosingFunc string
	Pos           token.Pos
	// WeakLabel is true when Code was resolved via the ambiguous "Type"
	// composite-literal field label specifically (isCodeFieldLabel's
	// comment already flags the risk: AWS Query's <Error><Type>Sender
	// </Type></Error> fault-type discriminator shares this field name with
	// iam/ecs's own per-op code field). Confirmed during gopherstack-zofv's
	// validation pass: this exact label produced ZERO class A findings
	// anywhere in the corpus, but produced Client/Sender/CNAME/GROUP/USER/
	// ... (real API data, never a code) at 25+ sites once nothing else
	// filtered its output -- so it is trusted for class A (its accidental
	// AllCodes cross-check has never let one of these through) but never
	// for the orphan class, which has no such backstop.
	WeakLabel bool
}

// walkOpEmissions finds every emission reachable from roots (hop 0 each
// root's own body, hop 1 any function/method call it makes directly),
// deduplicated by source position. Before walking, it looks for an
// override-mapper call (cls.Overrides) at hop 0 ONLY -- the handler's own
// body, where a call like services/iot's `respondAsInvalidRequest(c, err,
// ErrInvalidStateTransition)` sits -- and builds a PER-OP effective sentinel
// table so a hop-1 backend return of that same sentinel resolves to the
// override's code rather than the general mapper's, matching what this
// operation's real response actually renders.
func walkOpEmissions(roots []opRoot, idx *pkgIndex, cls *classifiers) []emission {
	effective := effectiveClassifiers(roots, idx, cls)

	visited := map[*ast.BlockStmt]bool{}

	out := make([]emission, 0, len(roots))

	for _, r := range roots {
		out = append(out, scanBodyEmissions(r.Body, idx, effective, 0, visited, "")...)
	}

	return filterUnreachable(dedupEmissions(out), roots, idx, cls)
}

// effectiveClassifiers builds this operation's OWN sentinel table before
// walking it, resolving gopherstack-0yva: a package-wide flat table cannot
// serve an operation whose own call path reaches a mapper (handleTagError)
// that disagrees with a DIFFERENT mapper (handleError) reachable only from
// other operations, on the very same sentinel identifier. localMapperScope
// finds which mapper(s), if any, THIS operation's own hop-0 root(s) call
// directly, and when it finds at least one, that table -- and ONLY that
// table -- replaces cls.Sentinels wholesale (not merged with the package-wide
// fallback, which belongs to operations this scan cannot pin to one mapper
// at all). Constructors are then re-resolved against that same narrowed
// table, so a constructor classified through the LOSING mapper's code
// (services/eks's validateTagMap: resolved package-wide via handleError's
// ErrValidation->InvalidParameterException, but TagResource's own path never
// reaches handleError, only handleTagError, which has no ErrValidation case
// at all) stops being attributed to an operation it cannot reach.
// localSentinelOverrides' own, more specific per-call-site mechanism is
// layered on top last, same precedence as before this fix.
func effectiveClassifiers(hop0Roots []opRoot, idx *pkgIndex, cls *classifiers) *classifiers {
	mapperScope, scoped := localMapperScope(hop0Roots, cls.ByFunc)
	overrides := localSentinelOverrides(hop0Roots, idx, cls.Overrides)

	if !scoped && len(overrides) == 0 {
		return cls
	}

	sentinels := cls.Sentinels
	if scoped {
		sentinels = mapperScope
	}

	if len(overrides) > 0 {
		merged := make(map[string]string, len(sentinels)+len(overrides))
		maps.Copy(merged, sentinels)
		maps.Copy(merged, overrides)
		sentinels = merged
	}

	funcs := cls.Funcs
	if scoped {
		funcs = resolveConstructorCodes(cls.Constructors, sentinels)
	}

	return &classifiers{
		Sentinels:    sentinels,
		ByFunc:       cls.ByFunc,
		Funcs:        funcs,
		Constructors: cls.Constructors,
		Overrides:    cls.Overrides,
	}
}

// localMapperScope finds every call, in hop0Roots' OWN bodies only (never
// recursing -- the same discipline localSentinelOverrides uses), to a
// package function classifiers.go's funcSentinelCodes recognises as a mapper
// (one containing its own errors.Is-based switch/if code table). When at
// least one is found, the SECOND return value is true and the caller must
// use ONLY this table -- even empty, after collision removal -- rather than
// falling back to the package-wide flat table this operation's own call path
// never reaches (services/eks's TagResource calling handleTagError, never
// handleError). When NONE is found, false is returned and the caller keeps
// using the package-wide fallback, preserving this tool's original recall
// for a service whose mapper is invoked outside the modeled call graph (a
// framework-level error handler this scan never sees literally called --
// this package's own sharedSentinelFixture/constructorFixture tests are
// exactly this shape).
//
// Two mappers reachable from the SAME operation that disagree on the same
// identifier are, like flattenSentinelCodes' package-wide case, dropped
// rather than resolved arbitrarily -- a loud absence, not a guessed winner.
func localMapperScope(hop0Roots []opRoot, byFunc map[string]map[string]string) (map[string]string, bool) {
	out := map[string]string{}
	conflict := map[string]bool{}
	found := false

	for _, r := range hop0Roots {
		if r.Body == nil {
			continue
		}

		ast.Inspect(r.Body, func(n ast.Node) bool {
			if table, ok := reachableMapperTable(n, byFunc); ok {
				found = true
				mergeMapperTable(table, out, conflict)
			}

			return true
		})
	}

	for ident := range conflict {
		delete(out, ident)
	}

	return out, found
}

// reachableMapperTable reports whether n is a call to a function byFunc
// recognises as a mapper, returning that mapper's own sentinel table.
func reachableMapperTable(n ast.Node, byFunc map[string]map[string]string) (map[string]string, bool) {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return nil, false
	}

	name, ok := calleeSimpleName(call.Fun)
	if !ok {
		return nil, false
	}

	table, known := byFunc[name]

	return table, known
}

// mergeMapperTable adds table's entries into out, marking any identifier
// that already has a DIFFERENT code (from a different mapper reachable from
// the same operation) in conflict, for the caller to drop afterward -- the
// same "refuse rather than guess" rule flattenSentinelCodes applies
// package-wide, applied here to the operation's own narrower scope.
func mergeMapperTable(table, out map[string]string, conflict map[string]bool) {
	for ident, code := range table {
		prev, exists := out[ident]
		if !exists {
			out[ident] = code

			continue
		}

		if prev != code {
			conflict[ident] = true
		}
	}
}

// localSentinelOverrides scans hop0Roots' OWN bodies (never recursing) for
// a call to a known override function, reading the actual sentinel argument
// passed at that call site -- and, for a respondAsConflictCode-shaped
// override (CodeParamIndex >= 0), the actual CODE argument too, since that
// helper's own body never contains the literal at all.
func localSentinelOverrides(hop0Roots []opRoot, idx *pkgIndex, overrides map[string]overrideFunc) map[string]string {
	out := map[string]string{}

	for _, r := range hop0Roots {
		if r.Body == nil {
			continue
		}

		ast.Inspect(r.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			name, ok := calleeSimpleName(call.Fun)
			if !ok {
				return true
			}

			ov, known := overrides[name]
			if !known || ov.ParamIndex >= len(call.Args) {
				return true
			}

			id, ok := call.Args[ov.ParamIndex].(*ast.Ident)
			if !ok || !idx.Sentinels[id.Name] {
				return true
			}

			if code, codeOK := resolveOverrideCallCode(call, ov, idx); codeOK {
				out[id.Name] = code
			}

			return true
		})
	}

	return out
}

// resolveOverrideCallCode returns the code an override call site actually
// renders: the helper's own fixed Code, or, when CodeParamIndex is >= 0, the
// code-shaped literal (or package-level const) passed as THAT call's own
// argument -- a call site passing anything else there (a variable, a
// computed expression) resolves to nothing, matching this tool's standing
// "silent miss over false finding" discipline elsewhere in this package: the
// operation falls back to the package-wide sentinel table rather than being
// suppressed on a guess.
func resolveOverrideCallCode(call *ast.CallExpr, ov overrideFunc, idx *pkgIndex) (string, bool) {
	if ov.CodeParamIndex < 0 {
		return ov.Code, true
	}

	if ov.CodeParamIndex >= len(call.Args) {
		return "", false
	}

	return firstCodeLiteral(call.Args[ov.CodeParamIndex], idx, 0)
}

func dedupEmissions(in []emission) []emission {
	seen := map[token.Pos]bool{}

	var out []emission

	for _, e := range in {
		if seen[e.Pos] {
			continue
		}

		seen[e.Pos] = true

		out = append(out, e)
	}

	return out
}

// enclosingFunc names the hop-1 callee body being walked ("" at hop 0) --
// stamped onto every emission found in it (emission.EnclosingFunc's own doc
// comment).
func scanBodyEmissions(
	body *ast.BlockStmt,
	idx *pkgIndex,
	cls *classifiers,
	hop int,
	visited map[*ast.BlockStmt]bool,
	enclosingFunc string,
) []emission {
	if body == nil || visited[body] {
		return nil
	}

	visited[body] = true

	var out []emission

	ast.Inspect(body, func(n ast.Node) bool {
		for _, e := range nodeEmissions(n, idx, cls) {
			e.EnclosingFunc = enclosingFunc
			out = append(out, e)
		}

		if hop < maxEmitHop {
			out = append(out, recurseCallEmissions(n, idx, cls, hop, visited)...)
		}

		return true
	})

	return out
}

func nodeEmissions(n ast.Node, idx *pkgIndex, cls *classifiers) []emission {
	switch v := n.(type) {
	case *ast.ReturnStmt:
		return returnStmtEmissions(v, cls)
	case *ast.CallExpr:
		return callExprEmissions(v, idx, cls)
	case *ast.CompositeLit:
		return compositeLitEmissions(v)
	case *ast.AssignStmt:
		return assignEmissions(v)
	case *ast.GenDecl:
		return genDeclEmissions(v)
	default:
		return nil
	}
}

// returnStmtEmissions catches a bare sentinel return (`return ErrX` /
// `return nil, ErrX`) and a wrapped one (`return fmt.Errorf("%w: ...", ErrX,
// ...)`) uniformly, via the same deep sentinel scan classifiers.go uses to
// resolve a constructor function's own code.
func returnStmtEmissions(ret *ast.ReturnStmt, cls *classifiers) []emission {
	var out []emission

	for _, res := range ret.Results {
		if code, ok := sentinelRefCode(res, cls.Sentinels); ok {
			out = append(out, emission{Code: code, Mechanism: "sentinel reference", Pos: res.Pos()})
		}
	}

	return out
}

// callExprEmissions catches a call to a known constructor classifier
// (services/networkmanager's notFoundError/validationError shape) and the
// direct-literal mechanisms this repo also uses outside the sentinel-mapper
// pattern (awserr.New/Newf).
func callExprEmissions(call *ast.CallExpr, idx *pkgIndex, cls *classifiers) []emission {
	var out []emission

	if name, ok := calleeSimpleName(call.Fun); ok && !bareBuiltinCall(call.Fun, name, idx) {
		if code, known := cls.Funcs[name]; known {
			out = append(out, emission{Code: code, Mechanism: "constructor classifier: " + name, Pos: call.Pos()})
		}
	}

	out = append(out, awserrLiteralEmissions(call)...)

	return out
}

func calleeSimpleName(fn ast.Expr) (string, bool) {
	switch v := fn.(type) {
	case *ast.Ident:
		return v.Name, true
	case *ast.SelectorExpr:
		return v.Sel.Name, true
	default:
		return "", false
	}
}

// isPredeclaredFuncIdent reports whether name is one of Go's predeclared
// function identifiers (language spec, "Predeclared identifiers").
func isPredeclaredFuncIdent(name string) bool {
	switch name {
	case "append", "cap", "clear", "close", "complex", "copy", "delete", "imag",
		"len", "make", "max", "min", "new", "panic", "print", "println", "real", "recover":
		return true
	default:
		return false
	}
}

// bareBuiltinCall reports whether fn is Go's builtin, not a same-named
// package method (gopherstack-bfb3): services/forecast's InMemoryBackend.delete
// method and the builtin delete(map, key) it calls internally share the
// identifier "delete", and cls.Funcs indexes by bare name alone. A method
// can only be invoked through a selector (b.delete(...)) or a method value
// -- never as a bare unqualified identifier -- so a bare call to a
// predeclared name resolves to a declared function only when the package
// itself declares a receiver-less func of that name (idx.Funcs) shadowing
// the builtin at package scope; otherwise it can only be the builtin.
func bareBuiltinCall(fn ast.Expr, name string, idx *pkgIndex) bool {
	if _, selector := fn.(*ast.SelectorExpr); selector {
		return false
	}

	if !isPredeclaredFuncIdent(name) {
		return false
	}

	_, shadowed := idx.Funcs[name]

	return !shadowed
}

// awserrLiteralEmissions covers services/ecs's own direct mechanism:
// awserr.New("Code", sentinel) / awserr.Newf("Code", format, args...) and
// stdlib errors.New("Code") where the sentinel's message IS the code --
// cmd/errcodeaudit's mechAwserrNew/mechStdlibErr, reimplemented narrowly (no
// sink-position table, see this package's doc comment for what that costs).
func awserrLiteralEmissions(call *ast.CallExpr) []emission {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}

	pkgIdent, ok := sel.X.(*ast.Ident)
	if !ok {
		return nil
	}

	switch {
	case pkgIdent.Name == "awserr" && (sel.Sel.Name == "New" || sel.Sel.Name == "Newf"):
		return literalArgEmissions(call.Args, 1, "awserr."+sel.Sel.Name+" arg")
	case pkgIdent.Name == pkgErrors && sel.Sel.Name == "New":
		return literalArgEmissions(call.Args, len(call.Args), "errors.New arg")
	default:
		return nil
	}
}

func literalArgEmissions(args []ast.Expr, limit int, mech string) []emission {
	var out []emission

	for i, arg := range args {
		if i >= limit {
			break
		}

		lit, ok := arg.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			continue
		}

		v, err := strconv.Unquote(lit.Value)
		if err != nil || !looksLikeCode(v) {
			continue
		}

		out = append(out, emission{Code: v, Mechanism: mech, Pos: lit.Pos()})
	}

	return out
}

// compositeLitEmissions covers a mapping-table row: a struct/map composite
// literal's Code/Type/ErrorCode-labeled field holding a code-shaped literal
// -- services/iam and services/ecs's own mechanism, narrowed to keyed
// elements only (no positional-field struct-order resolution, unlike
// cmd/errcodeaudit's fuller version -- see this package's doc for the cost).
func compositeLitEmissions(cl *ast.CompositeLit) []emission {
	var out []emission

	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}

		id, ok := kv.Key.(*ast.Ident)
		if !ok || !isCodeFieldLabel(id.Name) {
			continue
		}

		lit, ok := kv.Value.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			continue
		}

		v, err := strconv.Unquote(lit.Value)
		if err != nil || !looksLikeCode(v) {
			continue
		}

		out = append(out, emission{
			Code:      v,
			Mechanism: "composite literal field: " + id.Name,
			Pos:       lit.Pos(),
			WeakLabel: strings.EqualFold(id.Name, "type"),
		})
	}

	return out
}

// isCodeFieldLabel deliberately excludes a bare "Code" label: this repo's
// AWS-shaped batch operations (BatchDeleteXError{JobIdentifier, Code,
// Message}) legitimately carry a per-ITEM result code as part of a 200 OK
// response, not a wire error envelope -- a confirmed false positive on
// services/bedrock's BatchDeleteAdvancedPromptOptimizationJob during this
// tool's own validation pass, before this narrowing. "ErrorCode" and "Type"
// (the classic AWS Query <Error><Type>Sender</Type></Error> label, and
// services/iam/services/ecs's own field name) are narrow enough in practice
// that neither has produced that failure mode.
func isCodeFieldLabel(name string) bool {
	lower := strings.ToLower(name)

	return lower == "errorcode" || lower == "type"
}

// assignEmissions/genDeclEmissions cover `code := "ValidationError"` /
// `const errCodeValidation = "ValidationError"` -- a code-shaped literal
// assigned to an identifier whose own name marks it as an error code,
// services/cloudformation's mechanism.
func assignEmissions(as *ast.AssignStmt) []emission {
	if len(as.Lhs) != len(as.Rhs) {
		return nil
	}

	var out []emission

	for i, lhs := range as.Lhs {
		id, ok := lhs.(*ast.Ident)
		if !ok || !looksLikeCodeVarName(id.Name) {
			continue
		}

		if e, found := codeLitEmission(as.Rhs[i], "code-named var"); found {
			out = append(out, e)
		}
	}

	return out
}

func genDeclEmissions(gd *ast.GenDecl) []emission {
	if gd.Tok != token.CONST && gd.Tok != token.VAR {
		return nil
	}

	var out []emission

	for _, spec := range gd.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok || len(vs.Names) != len(vs.Values) {
			continue
		}

		for i, name := range vs.Names {
			if !looksLikeCodeVarName(name.Name) {
				continue
			}

			if e, found := codeLitEmission(vs.Values[i], "code-named const/var"); found {
				out = append(out, e)
			}
		}
	}

	return out
}

func codeLitEmission(expr ast.Expr, mech string) (emission, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return emission{}, false
	}

	v, err := strconv.Unquote(lit.Value)
	if err != nil || !looksLikeCode(v) {
		return emission{}, false
	}

	return emission{Code: v, Mechanism: mech, Pos: lit.Pos()}, true
}

// looksLikeCodeVarName mirrors cmd/errcodeaudit/extract.go's function of the
// same name: a name starting with "code", "errtype"/"errortype", or
// containing both "err" and "code" -- excluding a "key"/"field" prefix,
// this repo's own convention for a wire KEY-NAME constant rather than a
// code value.
func looksLikeCodeVarName(name string) bool {
	lower := strings.ToLower(name)
	if strings.HasPrefix(lower, "key") || strings.HasPrefix(lower, "field") {
		return false
	}

	if strings.HasPrefix(lower, "code") || strings.HasPrefix(lower, "errtype") ||
		strings.HasPrefix(lower, "errortype") {
		return true
	}

	return strings.Contains(lower, "err") && strings.Contains(lower, "code")
}

func recurseCallEmissions(
	n ast.Node,
	idx *pkgIndex,
	cls *classifiers,
	hop int,
	visited map[*ast.BlockStmt]bool,
) []emission {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return nil
	}

	var out []emission

	for _, fd := range calleeFuncDecls(call.Fun, idx) {
		out = append(out, scanBodyEmissions(fd.Body, idx, cls, hop+1, visited, fd.Name.Name)...)
	}

	return out
}

func calleeFuncDecls(fn ast.Expr, idx *pkgIndex) []*ast.FuncDecl {
	switch v := fn.(type) {
	case *ast.SelectorExpr:
		return idx.Methods[v.Sel.Name]
	case *ast.Ident:
		if fd, ok := idx.Funcs[v.Name]; ok {
			return []*ast.FuncDecl{fd}
		}
	}

	return nil
}

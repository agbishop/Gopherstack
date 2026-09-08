package main

import (
	"go/ast"
	"maps"
)

// collectIndexAssignDispatchEntries handles comprehend's shape
// (handler.go:266-307, gopherstack-sgbw blind spot 2): a dispatch map
// declared via `:=` or `make(...)` (not just the *ast.CompositeLit
// dispatch.go's collectMapLiteralEntries already sees) and populated by
// later index-assignment, some directly (`ops["TagResource"] = h.tagResource`)
// and some through a `for prefix, spec := range someLiteralMap() { ... }`
// loop whose key is built by concatenating a literal onto the loop
// variable.
//
// A value that is itself a struct/data literal (forecast's
// `operations["CreateAutoPredictor"] = operationSpec{...}`, handler.go:631)
// can never resolve to a callable no matter which collector finds it, so it
// is routed into dataKeys instead of out -- collectSharedExecutorFallback
// is the only mechanism that can ever resolve those.
func collectIndexAssignDispatchEntries(
	files []*ast.File,
	pkgConsts map[string]string,
	funcs map[string]*ast.FuncDecl,
	out map[string]ast.Expr,
	dataKeys map[string]bool,
) {
	bind := func(key string, value ast.Expr) {
		if _, isCompositeLit := unwrapParen(value).(*ast.CompositeLit); isCompositeLit {
			dataKeys[key] = true

			return
		}

		out[key] = value
	}

	for _, f := range files {
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}

			dispatchVars := findDispatchVarDecls(fd.Body)
			if len(dispatchVars) == 0 {
				continue
			}

			walkIndexAssigns(fd.Body.List, dispatchVars, nil, pkgConsts, funcs, bind)
		}
	}
}

// findDispatchVarDecls finds every local identifier in body declared as
// `x := map[string]T{...}` or `x := make(map[string]T)` for any T -- the
// two shapes comprehend's `ops` and forecast's `operations` locals actually
// use (handler.go:242, forecast/handler.go:628). Does not cross into a
// nested func literal, matching firstReturnExpr's own scoping rule.
func findDispatchVarDecls(body *ast.BlockStmt) map[string]bool {
	out := map[string]bool{}

	ast.Inspect(body, func(n ast.Node) bool {
		if _, isFuncLit := n.(*ast.FuncLit); isFuncLit {
			return false
		}

		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}

		id, ok := assign.Lhs[0].(*ast.Ident)
		if !ok || !isDispatchMapDecl(assign.Rhs[0]) {
			return true
		}

		out[id.Name] = true

		return true
	})

	return out
}

func isDispatchMapDecl(rhs ast.Expr) bool {
	switch v := rhs.(type) {
	case *ast.CompositeLit:
		return v.Type != nil && isStringKeyedMapType(v.Type)
	case *ast.CallExpr:
		fnIdent, ok := v.Fun.(*ast.Ident)

		return ok && fnIdent.Name == "make" && len(v.Args) > 0 && isStringKeyedMapType(v.Args[0])
	default:
		return false
	}
}

// walkIndexAssigns recurses through if/range statements (the only
// compound statements comprehend's and forecast's populators actually use)
// looking for `<dispatchVar>[<key>] = <value>` assignments. It does not
// evaluate `if` conditions -- an assignment inside a conditional branch is
// bound unconditionally, the same over-inclusive tradeoff
// resolveOpRoots's own doc comment already accepts elsewhere in this tool:
// a spurious candidate key is silently unused (never a real op name), a
// missed one is a resolution gap.
func walkIndexAssigns(
	stmts []ast.Stmt,
	dispatchVars map[string]bool,
	env, pkgConsts map[string]string,
	funcs map[string]*ast.FuncDecl,
	bind func(key string, value ast.Expr),
) {
	for _, stmt := range stmts {
		switch v := stmt.(type) {
		case *ast.AssignStmt:
			bindIndexAssign(v, dispatchVars, env, pkgConsts, funcs, bind)
		case *ast.IfStmt:
			walkIndexAssigns(v.Body.List, dispatchVars, env, pkgConsts, funcs, bind)
			walkIfElse(v.Else, dispatchVars, env, pkgConsts, funcs, bind)
		case *ast.RangeStmt:
			walkRangeIndexAssigns(v, dispatchVars, env, pkgConsts, funcs, bind)
		}
	}
}

func walkIfElse(
	els ast.Stmt,
	dispatchVars map[string]bool,
	env, pkgConsts map[string]string,
	funcs map[string]*ast.FuncDecl,
	bind func(key string, value ast.Expr),
) {
	switch v := els.(type) {
	case *ast.BlockStmt:
		walkIndexAssigns(v.List, dispatchVars, env, pkgConsts, funcs, bind)
	case *ast.IfStmt:
		walkIndexAssigns([]ast.Stmt{v}, dispatchVars, env, pkgConsts, funcs, bind)
	}
}

func bindIndexAssign(
	v *ast.AssignStmt,
	dispatchVars map[string]bool,
	env, pkgConsts map[string]string,
	funcs map[string]*ast.FuncDecl,
	bind func(key string, value ast.Expr),
) {
	if len(v.Lhs) != 1 || len(v.Rhs) != 1 {
		return
	}

	idx, ok := v.Lhs[0].(*ast.IndexExpr)
	if !ok {
		return
	}

	id, ok := idx.X.(*ast.Ident)
	if !ok || !dispatchVars[id.Name] {
		return
	}

	key, ok := evalConcatKey(idx.Index, env, pkgConsts, funcs, 0)
	if !ok {
		return
	}

	bind(key, v.Rhs[0])
}

func walkRangeIndexAssigns(
	rs *ast.RangeStmt,
	dispatchVars map[string]bool,
	env, pkgConsts map[string]string,
	funcs map[string]*ast.FuncDecl,
	bind func(key string, value ast.Expr),
) {
	keyIdent, ok := rs.Key.(*ast.Ident)
	if !ok || rs.Body == nil {
		return
	}

	for _, sourceKey := range resolveRangeSourceKeys(rs.X, pkgConsts, funcs) {
		childEnv := make(map[string]string, len(env)+1)
		maps.Copy(childEnv, env)
		childEnv[keyIdent.Name] = sourceKey

		walkIndexAssigns(rs.Body.List, dispatchVars, childEnv, pkgConsts, funcs, bind)
	}
}

// resolveRangeSourceKeys resolves a `range X` source to its own literal
// string keys -- either X is itself a map composite literal, or (comprehend's
// asyncJobSpecs()/resourceSpecs() shape, handler_jobs.go:20,
// handler_resources.go:25) a zero-arg call to a local function whose only
// return statement is one.
func resolveRangeSourceKeys(src ast.Expr, pkgConsts map[string]string, funcs map[string]*ast.FuncDecl) []string {
	switch v := unwrapParen(src).(type) {
	case *ast.CompositeLit:
		return literalMapKeys(v, pkgConsts)
	case *ast.CallExpr:
		return resolveZeroArgMapCallKeys(v, pkgConsts, funcs)
	default:
		return nil
	}
}

func resolveZeroArgMapCallKeys(
	call *ast.CallExpr,
	pkgConsts map[string]string,
	funcs map[string]*ast.FuncDecl,
) []string {
	if len(call.Args) != 0 {
		return nil
	}

	fnIdent, ok := call.Fun.(*ast.Ident)
	if !ok {
		return nil
	}

	fd, ok := funcs[fnIdent.Name]
	if !ok || fd.Body == nil {
		return nil
	}

	ret := firstReturnExpr(fd.Body)
	if ret == nil {
		return nil
	}

	cl, ok := unwrapParen(ret).(*ast.CompositeLit)
	if !ok {
		return nil
	}

	return literalMapKeys(cl, pkgConsts)
}

func literalMapKeys(cl *ast.CompositeLit, pkgConsts map[string]string) []string {
	var out []string

	for _, elt := range cl.Elts {
		kv, isKV := elt.(*ast.KeyValueExpr)
		if !isKV {
			continue
		}

		if s, resolved := resolveStringExpr(kv.Key, pkgConsts); resolved {
			out = append(out, s)
		}
	}

	return out
}

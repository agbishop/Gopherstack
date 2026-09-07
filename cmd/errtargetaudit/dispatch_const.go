package main

import (
	"go/ast"
	"go/token"
)

// maxConcatDepth bounds evalConcatKey's recursion through nested helper
// calls (forecast's "List"+plural(base), handler.go:799) -- deep enough for
// any real shape this repo uses, shallow enough that a pathological or
// self-referential helper cannot hang the scan.
const maxConcatDepth = 6

// evalConcatKey evaluates a string-key expression built from literals,
// package consts, identifiers bound in env (a loop variable or a
// call-site-substituted parameter), and calls to a local single-string-
// param helper function evaluated via evalHelperCall. This is NOT a general
// constant-folder: only the shapes forecast's addCRUD (handler.go:797-802)
// and comprehend's asyncJobSpecs/resourceSpecs loops (handler.go:265-283)
// actually use are supported; anything else reports unresolved rather than
// guessing.
func evalConcatKey(
	e ast.Expr,
	env, pkgConsts map[string]string,
	funcs map[string]*ast.FuncDecl,
	depth int,
) (string, bool) {
	if depth > maxConcatDepth {
		return "", false
	}

	e = unwrapParen(e)

	if id, isIdent := e.(*ast.Ident); isIdent {
		if s, ok := env[id.Name]; ok {
			return s, true
		}
	}

	if s, ok := resolveStringExpr(e, pkgConsts); ok {
		return s, true
	}

	switch v := e.(type) {
	case *ast.BinaryExpr:
		return evalConcatBinary(v, env, pkgConsts, funcs, depth)
	case *ast.CallExpr:
		return evalHelperCall(v, env, pkgConsts, funcs, depth)
	default:
		return "", false
	}
}

func evalConcatBinary(
	v *ast.BinaryExpr,
	env, pkgConsts map[string]string,
	funcs map[string]*ast.FuncDecl,
	depth int,
) (string, bool) {
	if v.Op != token.ADD {
		return "", false
	}

	l, lok := evalConcatKey(v.X, env, pkgConsts, funcs, depth+1)
	if !lok {
		return "", false
	}

	r, rok := evalConcatKey(v.Y, env, pkgConsts, funcs, depth+1)
	if !rok {
		return "", false
	}

	return l + r, true
}

// evalHelperCall evaluates a call to a local, package-level, single-string-
// parameter function (forecast's plural, handler.go:812-823) by resolving
// its argument first, then walking the callee's own body for a switch
// statement over that parameter.
func evalHelperCall(
	call *ast.CallExpr,
	env, pkgConsts map[string]string,
	funcs map[string]*ast.FuncDecl,
	depth int,
) (string, bool) {
	fnIdent, ok := call.Fun.(*ast.Ident)
	if !ok || len(call.Args) != 1 {
		return "", false
	}

	fd, ok := funcs[fnIdent.Name]
	if !ok || fd.Body == nil || fd.Type.Params == nil || len(fd.Type.Params.List) != 1 {
		return "", false
	}

	names := fd.Type.Params.List[0].Names
	if len(names) != 1 {
		return "", false
	}

	argVal, ok := evalConcatKey(call.Args[0], env, pkgConsts, funcs, depth+1)
	if !ok {
		return "", false
	}

	return evalSwitchHelperBody(fd.Body, names[0].Name, argVal, pkgConsts, funcs, depth+1)
}

func evalSwitchHelperBody(
	body *ast.BlockStmt,
	paramName, argVal string,
	pkgConsts map[string]string,
	funcs map[string]*ast.FuncDecl,
	depth int,
) (string, bool) {
	childEnv := map[string]string{paramName: argVal}

	for _, stmt := range body.List {
		sw, ok := stmt.(*ast.SwitchStmt)
		if !ok {
			continue
		}

		tag, ok := sw.Tag.(*ast.Ident)
		if !ok || tag.Name != paramName {
			continue
		}

		return evalSwitchCases(sw, argVal, childEnv, pkgConsts, funcs, depth)
	}

	return "", false
}

func evalSwitchCases(
	sw *ast.SwitchStmt,
	argVal string,
	env, pkgConsts map[string]string,
	funcs map[string]*ast.FuncDecl,
	depth int,
) (string, bool) {
	var defaultClause *ast.CaseClause

	for _, stmt := range sw.Body.List {
		cc, ok := stmt.(*ast.CaseClause)
		if !ok {
			continue
		}

		if cc.List == nil {
			defaultClause = cc

			continue
		}

		if caseListMatches(cc.List, argVal, pkgConsts) {
			return evalClauseReturn(cc, env, pkgConsts, funcs, depth)
		}
	}

	if defaultClause != nil {
		return evalClauseReturn(defaultClause, env, pkgConsts, funcs, depth)
	}

	return "", false
}

func evalClauseReturn(
	cc *ast.CaseClause,
	env, pkgConsts map[string]string,
	funcs map[string]*ast.FuncDecl,
	depth int,
) (string, bool) {
	ret := firstReturnExpr(&ast.BlockStmt{List: cc.Body})
	if ret == nil {
		return "", false
	}

	return evalConcatKey(ret, env, pkgConsts, funcs, depth+1)
}

func caseListMatches(list []ast.Expr, val string, pkgConsts map[string]string) bool {
	for _, e := range list {
		if s, ok := resolveStringExpr(e, pkgConsts); ok && s == val {
			return true
		}
	}

	return false
}

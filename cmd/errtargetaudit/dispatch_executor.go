package main

import "go/ast"

// collectPopulatorHelperKeys handles forecast's addCRUD shape
// (handler.go:782-804, gopherstack-sgbw blind spot 1's key-recovery half):
// a package-level function whose first string-keyed-map parameter it
// populates by index-assignment, keyed by concatenating a literal onto ONE
// OF ITS OWN OTHER PARAMETERS (`operations["Create"+base]`) rather than
// anything visible at any single call site the way collectIndexAssignDispatchEntries's
// dispatch vars are. Every direct call site's literal arguments are bound
// to those other parameters and the function's body evaluated once per
// site; the map parameter's own name is never resolved to a value (it does
// not need to be -- only the KEYS this recovers, dataKeys collects them for
// collectSharedExecutorFallback since a struct/data value can never resolve
// to a callable on its own, see this file's own doc comment on that
// function for why).
func collectPopulatorHelperKeys(
	files []*ast.File,
	pkgConsts map[string]string,
	funcs map[string]*ast.FuncDecl,
	dataKeys map[string]bool,
) {
	bind := func(key string, _ ast.Expr) { dataKeys[key] = true }

	for _, fd := range funcs {
		mapParam, ok := findMapParam(fd)
		if !ok {
			continue
		}

		dispatchVars := map[string]bool{mapParam: true}

		for _, f := range files {
			collectPopulatorCallSites(f, fd, mapParam, dispatchVars, pkgConsts, funcs, bind)
		}
	}
}

func collectPopulatorCallSites(
	f *ast.File,
	fd *ast.FuncDecl,
	mapParam string,
	dispatchVars map[string]bool,
	pkgConsts map[string]string,
	funcs map[string]*ast.FuncDecl,
	bind func(key string, value ast.Expr),
) {
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		calleeIdent, ok := call.Fun.(*ast.Ident)
		if !ok || calleeIdent.Name != fd.Name.Name {
			return true
		}

		env, ok := bindPopulatorCallEnv(fd, call, mapParam, pkgConsts, funcs)
		if !ok {
			return true
		}

		walkIndexAssigns(fd.Body.List, dispatchVars, env, pkgConsts, funcs, bind)

		return true
	})
}

// findMapParam reports fd's first string-keyed-map parameter, by name.
func findMapParam(fd *ast.FuncDecl) (string, bool) {
	if fd.Body == nil || fd.Type.Params == nil {
		return "", false
	}

	for _, field := range fd.Type.Params.List {
		if !isStringKeyedMapType(field.Type) {
			continue
		}

		if len(field.Names) == 1 {
			return field.Names[0].Name, true
		}
	}

	return "", false
}

// bindPopulatorCallEnv binds fd's OTHER (non-map) parameters to the literal
// value of the matching argument at this call site, skipping any parameter
// whose argument does not evaluate to a literal -- a call site with a
// non-literal argument for a parameter no key expression happens to need
// costs nothing; one for a parameter a key expression DOES need means that
// key is simply not recovered from this site, the same partial-recovery
// tradeoff evalConcatKey already accepts.
func bindPopulatorCallEnv(
	fd *ast.FuncDecl,
	call *ast.CallExpr,
	mapParam string,
	pkgConsts map[string]string,
	funcs map[string]*ast.FuncDecl,
) (map[string]string, bool) {
	names := flattenParamNames(fd.Type.Params)
	if len(names) != len(call.Args) {
		return nil, false
	}

	env := map[string]string{}

	for i, name := range names {
		if name == "" || name == mapParam {
			continue
		}

		if s, ok := evalConcatKey(call.Args[i], nil, pkgConsts, funcs, 0); ok {
			env[name] = s
		}
	}

	return env, true
}

// collectSharedExecutorFallback handles blind spot 1's other half: forecast
// routes every op through `spec, ok := h.ops[action]` followed by
// `h.execute(action, spec, input)` (handler.go:156,161) rather than calling
// a per-op handler directly. It is deliberately scoped to only the
// function(s) this service registers as its actual dispatcher --
// service.HandleTarget's own func-valued arguments (every service in this
// repo uses the identical convention, see this package's doc comment on
// RESOLVING WHICH HANDLER SERVES AN OPERATION) -- rather than searching the
// whole package for a comma-ok map lookup that flows into a call: an
// unrelated backend method with that same shape (a cache lookup passed to a
// helper, say) would otherwise bind EVERY key in dataKeys to the wrong
// root.
func collectSharedExecutorFallback(
	files []*ast.File,
	funcs map[string]*ast.FuncDecl,
	dataKeys map[string]bool,
	out map[string]ast.Expr,
) {
	if len(dataKeys) == 0 {
		return
	}

	methods := map[string][]*ast.FuncDecl{}

	for _, f := range files {
		for _, decl := range f.Decls {
			if fd, ok := decl.(*ast.FuncDecl); ok && fd.Recv != nil {
				methods[fd.Name.Name] = append(methods[fd.Name.Name], fd)
			}
		}
	}

	executor := findSharedExecutorCall(findDispatchFuncBodies(files, methods, funcs))
	if executor == nil {
		return
	}

	for key := range dataKeys {
		if _, exists := out[key]; exists {
			continue
		}

		out[key] = executor
	}
}

func findDispatchFuncBodies(
	files []*ast.File,
	methods map[string][]*ast.FuncDecl,
	funcs map[string]*ast.FuncDecl,
) []*ast.BlockStmt {
	var out []*ast.BlockStmt

	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			pkgIdent, ok := sel.X.(*ast.Ident)
			if !ok || pkgIdent.Name != "service" || sel.Sel.Name != "HandleTarget" {
				return true
			}

			for _, arg := range call.Args {
				out = append(out, resolveFuncArgBodies(arg, methods, funcs)...)
			}

			return true
		})
	}

	return out
}

func resolveFuncArgBodies(
	arg ast.Expr,
	methods map[string][]*ast.FuncDecl,
	funcs map[string]*ast.FuncDecl,
) []*ast.BlockStmt {
	var out []*ast.BlockStmt

	switch v := arg.(type) {
	case *ast.SelectorExpr:
		for _, fd := range methods[v.Sel.Name] {
			if fd.Body != nil {
				out = append(out, fd.Body)
			}
		}
	case *ast.Ident:
		if fd, ok := funcs[v.Name]; ok && fd.Body != nil {
			out = append(out, fd.Body)
		}
	}

	return out
}

// findSharedExecutorCall finds, in any of bodies, a comma-ok map-index
// lookup (`v, ok := X[idx]`) whose bound value later flows into a call as
// an argument -- forecast's `spec, ok := h.ops[action]` ...
// `h.execute(action, spec, input)`. Type-agnostic on purpose: this tool
// does no type-checking, and the pattern is unambiguous enough on its own
// once scoped to the real dispatcher bodies (see the doc comment on this
// function's only caller).
func findSharedExecutorCall(bodies []*ast.BlockStmt) ast.Expr {
	for _, body := range bodies {
		if call := findExecutorInBlock(body); call != nil {
			return call
		}
	}

	return nil
}

func findExecutorInBlock(body *ast.BlockStmt) ast.Expr {
	for i, stmt := range body.List {
		assign, ok := stmt.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 2 || len(assign.Rhs) != 1 {
			continue
		}

		if _, isIndexExpr := assign.Rhs[0].(*ast.IndexExpr); !isIndexExpr {
			continue
		}

		valueIdent, ok := assign.Lhs[0].(*ast.Ident)
		if !ok || valueIdent.Name == "_" {
			continue
		}

		if call := findCallUsingIdent(body.List[i+1:], valueIdent.Name); call != nil {
			return call
		}
	}

	return nil
}

func findCallUsingIdent(stmts []ast.Stmt, name string) ast.Expr {
	var found ast.Expr

	for _, stmt := range stmts {
		if found != nil {
			break
		}

		ast.Inspect(stmt, func(n ast.Node) bool {
			if found != nil {
				return false
			}

			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			for _, arg := range call.Args {
				if id, isIdent := arg.(*ast.Ident); isIdent && id.Name == name {
					found = call

					return false
				}
			}

			return true
		})
	}

	return found
}

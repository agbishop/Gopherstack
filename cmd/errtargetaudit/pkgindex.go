package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// pkgIndex is the structural index of one services/<dir> package this tool
// builds once and resolves every operation against -- the same shape of
// object cmd/reqfielddiff's handlerResolveCtx plays, generalized: no struct
// field collection at all (this tool never needs a request TYPE, only a
// function BODY to scan for error-code emission), but the same dispatch-table
// and name-convention machinery, plus a package-wide sentinel/classifier
// index those tools have no analogue for.
type pkgIndex struct {
	Fset           *token.FileSet
	Methods        map[string][]*ast.FuncDecl
	Funcs          map[string]*ast.FuncDecl
	PkgConsts      map[string]string
	FuncTypeNames  map[string]bool
	WrapOpWrappers map[string]bool
	Dispatch       map[string]ast.Expr
	Sentinels      map[string]bool
	PkgVars        map[string]ast.Expr
	Files          []*ast.File
}

func buildPkgIndex(dir string) (*pkgIndex, error) {
	fset := token.NewFileSet()

	files, err := parseNonTestDirFiles(fset, dir)
	if err != nil {
		return nil, err
	}

	return buildPkgIndexFromFiles(files, fset), nil
}

// buildPkgIndexFromFiles builds a pkgIndex from already-parsed files --
// the entry point tests use to build fixtures from in-memory source,
// without touching the filesystem (same split as cmd/reqfielddiff's
// buildPackageIndex/buildPackageIndexFromFiles).
func buildPkgIndexFromFiles(files []*ast.File, fset *token.FileSet) *pkgIndex {
	idx := &pkgIndex{Fset: fset, Files: files}
	idx.Methods, idx.Funcs = collectFuncs(files)
	idx.PkgConsts = collectPackageStringConsts(files)
	idx.FuncTypeNames = collectLocalFuncTypeNames(files)
	idx.WrapOpWrappers = collectLocalWrapOpWrappers(files)
	idx.Dispatch = collectDispatchEntries(files, idx.PkgConsts, idx.FuncTypeNames)
	idx.Sentinels = collectSentinelVars(files)
	idx.PkgVars = collectPackageVarExprs(files)

	return idx
}

// collectPackageVarExprs indexes every package-level `var X = <expr>`
// declaration (one name, one value) by name, regardless of the value's own
// shape -- classifiers.go's resolveTableRows uses this to follow a
// range-loop's source identifier (services/route53's backendErrorTable,
// services/kms's declared-elsewhere tables) back to its own literal, the
// same one hop of indirection idx.PkgConsts already gives string constants.
func collectPackageVarExprs(files []*ast.File) map[string]ast.Expr {
	out := map[string]ast.Expr{}

	for _, f := range files {
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}

			for _, spec := range gd.Specs {
				vs, vsOK := spec.(*ast.ValueSpec)
				if !vsOK || len(vs.Names) != 1 || len(vs.Values) != 1 {
					continue
				}

				out[vs.Names[0].Name] = vs.Values[0]
			}
		}
	}

	return out
}

func parseNonTestDirFiles(fset *token.FileSet, dir string) ([]*ast.File, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var files []*ast.File

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}

		f, perr := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, 0)
		if perr != nil {
			return nil, perr
		}

		files = append(files, f)
	}

	return files, nil
}

func collectFuncs(files []*ast.File) (map[string][]*ast.FuncDecl, map[string]*ast.FuncDecl) {
	methods := map[string][]*ast.FuncDecl{}
	funcs := map[string]*ast.FuncDecl{}

	for _, f := range files {
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}

			if fd.Recv != nil {
				methods[fd.Name.Name] = append(methods[fd.Name.Name], fd)
			} else {
				funcs[fd.Name.Name] = fd
			}
		}
	}

	return methods, funcs
}

func collectPackageStringConsts(files []*ast.File) map[string]string {
	out := map[string]string{}

	for _, f := range files {
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}

			for _, spec := range gd.Specs {
				addStringValueSpec(spec, out)
			}
		}
	}

	return out
}

func addStringValueSpec(spec ast.Spec, out map[string]string) {
	vs, ok := spec.(*ast.ValueSpec)
	if !ok || len(vs.Names) != 1 || len(vs.Values) != 1 {
		return
	}

	lit, ok := vs.Values[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return
	}

	if v, err := strconv.Unquote(lit.Value); err == nil {
		out[vs.Names[0].Name] = v
	}
}

// collectLocalFuncTypeNames finds every package-level `type X func(...)...`
// declaration, so a dispatch table keyed by such a named type is recognised
// the same way a literal func type or service.JSONOpFunc would be --
// identical to cmd/reqfielddiff's function of the same name.
func collectLocalFuncTypeNames(files []*ast.File) map[string]bool {
	out := map[string]bool{}

	for _, f := range files {
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}

			for _, spec := range gd.Specs {
				ts, tsOK := spec.(*ast.TypeSpec)
				if !tsOK {
					continue
				}

				if _, isFunc := ts.Type.(*ast.FuncType); isFunc {
					out[ts.Name.Name] = true
				}
			}
		}
	}

	return out
}

// collectSentinelVars finds every package-level `var ErrX = errors.New(...)`
// or `var ErrX = fmt.Errorf(...)` declaration -- this repo's uniform sentinel
// shape (services/bedrock's ErrNotFound/ErrAlreadyExists/ErrValidation,
// services/iot's ErrThingNotFound/ErrRuleNotFound/..., services/backup's
// ErrNotFound/ErrValidation/ErrInvalidRequest). A name is also admitted on
// its OWN shape (an "Err"-prefixed identifier assigned any call expression),
// so a sentinel built through a small local helper (e.g. a repo-local
// `newSentinel("msg")`) is not silently dropped just because this scan
// doesn't recognise the specific stdlib call.
func collectSentinelVars(files []*ast.File) map[string]bool {
	out := map[string]bool{}

	for _, f := range files {
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}

			for _, spec := range gd.Specs {
				addSentinelValueSpec(spec, out)
			}
		}
	}

	return out
}

func addSentinelValueSpec(spec ast.Spec, out map[string]bool) {
	vs, ok := spec.(*ast.ValueSpec)
	if !ok || len(vs.Names) != len(vs.Values) {
		return
	}

	for i, name := range vs.Names {
		if !strings.HasPrefix(name.Name, "Err") && !strings.HasPrefix(name.Name, "err") {
			continue
		}

		if _, isCall := vs.Values[i].(*ast.CallExpr); isCall {
			out[name.Name] = true
		}
	}
}

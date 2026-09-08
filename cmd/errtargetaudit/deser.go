package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
)

// moduleGroundTruth is one pinned SDK module's error-code ground truth, read
// straight from its own source in the module cache (same two files
// cmd/errcodeaudit reads, same shapes -- see its main.go doc for why each
// shape is trustworthy). PerOp is keyed by operation name and holds exactly
// the codes THAT OPERATION'S OWN awsRestjson1_deserializeOpError<Op> (or
// protocol-equivalent) function matches via strings.EqualFold -- the
// per-operation ground truth this tool's whole premise rests on, per
// gopherstack-o46l: "the only way to see it is to read the specific
// operation's own deserializer and confirm it declares that code."
// OpFuncs is every operation name that HAS such a function at all (whether
// or not it matched any code), used to assign an operation to the right
// module when a service resolves more than one (moduleassign.go). AllCodes
// is the module-wide "real code universe" -- typeCodes (every declared
// exception type's own ErrorCode() literal) unioned with every op's
// deserCodes -- used only to tell a real-but-misplaced code (class A) apart
// from a fabricated one (class B, cmd/errcodeaudit's job, not this tool's).
type moduleGroundTruth struct {
	PerOp     map[string]map[string]bool
	OpFuncs   map[string]bool
	AllCodes  map[string]bool
	TypeCodes map[string]bool
}

func newModuleGroundTruth() *moduleGroundTruth {
	return &moduleGroundTruth{
		PerOp:     map[string]map[string]bool{},
		OpFuncs:   map[string]bool{},
		AllCodes:  map[string]bool{},
		TypeCodes: map[string]bool{},
	}
}

// loadModuleGroundTruth reads modPath's types/errors.go and deserializers.go.
// A module missing either file (or the whole module dir) contributes an
// empty ground truth, never an error -- "nothing to check" is a normal
// outcome here, same discipline as cmd/errcodeaudit's loadModuleCodes.
func loadModuleGroundTruth(modPath string) (*moduleGroundTruth, error) {
	mgt := newModuleGroundTruth()

	errorsPath := filepath.Join(modPath, "types", "errors.go")
	if exists, statErr := fileExists(errorsPath); statErr != nil {
		return nil, statErr
	} else if exists {
		codes, err := parseErrorCodeMethods(errorsPath)
		if err != nil {
			return nil, err
		}

		mgt.TypeCodes = codes

		for c := range codes {
			mgt.AllCodes[c] = true
		}
	}

	deserPath := filepath.Join(modPath, "deserializers.go")
	if exists, statErr := fileExists(deserPath); statErr != nil {
		return nil, statErr
	} else if exists {
		if err := parsePerOpDeserializerCodes(deserPath, mgt); err != nil {
			return nil, err
		}
	}

	return mgt, nil
}

// parseErrorCodeMethods reads every `func (e *X) ErrorCode() string { ... }`
// and collects the literal it can directly return -- see
// cmd/errcodeaudit/sdktruth.go's identical function for why only the
// fallback-branch literal is collected, never the override-pointer branch.
func parseErrorCodeMethods(errorsGoPath string) (map[string]bool, error) {
	fset := token.NewFileSet()

	f, err := parser.ParseFile(fset, errorsGoPath, nil, 0)
	if err != nil {
		return nil, err
	}

	codes := map[string]bool{}

	for _, decl := range f.Decls {
		fd, isFD := decl.(*ast.FuncDecl)
		if !isFD || fd.Recv == nil || fd.Name.Name != "ErrorCode" || fd.Body == nil {
			continue
		}

		ast.Inspect(fd.Body, func(n ast.Node) bool {
			ret, isRet := n.(*ast.ReturnStmt)
			if !isRet || len(ret.Results) != 1 {
				return true
			}

			if lit, litOK := ret.Results[0].(*ast.BasicLit); litOK && lit.Kind == token.STRING {
				if v, uqErr := strconv.Unquote(lit.Value); uqErr == nil {
					codes[v] = true
				}
			}

			return true
		})
	}

	return codes, nil
}

// deserializeOpErrorMarker is the substring every generated per-operation
// error-deserialize function name contains, across every protocol observed
// in this repo's pinned SDKs (awsRestjson1_deserializeOpError<Op>,
// awsAwsjson11_deserializeOpError<Op>, awsAwsquery_deserializeOpError<Op>,
// awsRestxml_deserializeOpError<Op>, awsEc2query_deserializeOpError<Op>, ...)
// -- confirmed the same shape cmd/errcodeaudit's sdktruth.go already relies
// on protocol-agnostically.
const deserializeOpErrorMarker = "deserializeOpError"

// parsePerOpDeserializerCodes reads every deserializeOpError<Op> function in
// deserGoPath, recording its own operation name (everything after the
// marker) against the EqualFold code literals found in ITS OWN body only --
// never merged across functions, which is the entire point: a code declared
// for a sibling operation must never leak into this one's set.
func parsePerOpDeserializerCodes(deserGoPath string, mgt *moduleGroundTruth) error {
	fset := token.NewFileSet()

	f, err := parser.ParseFile(fset, deserGoPath, nil, 0)
	if err != nil {
		return err
	}

	for _, decl := range f.Decls {
		fd, isFD := decl.(*ast.FuncDecl)
		if !isFD || fd.Body == nil {
			continue
		}

		idx := strings.Index(fd.Name.Name, deserializeOpErrorMarker)
		if idx < 0 {
			continue
		}

		op := fd.Name.Name[idx+len(deserializeOpErrorMarker):]
		if op == "" {
			continue
		}

		mgt.OpFuncs[op] = true

		codes := map[string]bool{}
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			if lit, litOK := equalFoldCodeLiteral(n); litOK {
				codes[lit] = true
				mgt.AllCodes[lit] = true
			}

			if lit, litOK := stringSwitchCaseLiteral(n); litOK {
				codes[lit] = true
				mgt.AllCodes[lit] = true
			}

			return true
		})

		if len(codes) > 0 {
			mgt.PerOp[op] = codes
		}
	}

	return nil
}

// stringSwitchCaseLiteral reports a plain string-literal case label of a
// *ast.CaseClause -- the newer RPCv2CBOR/Smithy-CBOR codegen's shape
// (confirmed live: services/appstream's pinned SDK, `switch
// string(errorName) { case "ConcurrentModificationException": ... }`, no
// strings.EqualFold anywhere in the function), a DIFFERENT deserializeOpError
// shape from the classic restjson1/awsjson1.1/query one equalFoldCodeLiteral
// reads. Discovered as a false-positive source during this tool's own
// validation pass: every op in an RPCv2CBOR-protocol module read as
// declaring ZERO codes without this, so every emission there was flagged
// "undeclared" regardless of truth.
func stringSwitchCaseLiteral(n ast.Node) (string, bool) {
	cc, ok := n.(*ast.CaseClause)
	if !ok || len(cc.List) != 1 {
		return "", false
	}

	lit, ok := cc.List[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}

	v, err := strconv.Unquote(lit.Value)
	if err != nil || !looksLikeCode(v) {
		return "", false
	}

	return v, true
}

// equalFoldCodeLiteral reports the literal first argument of a
// strings.EqualFold(<literal>, <ident>) call, the shape every
// deserializeOpError* switch case uses -- identical to
// cmd/errcodeaudit/sdktruth.go's function of the same name.
func equalFoldCodeLiteral(n ast.Node) (string, bool) {
	call, ok := n.(*ast.CallExpr)
	if !ok || len(call.Args) != 2 {
		return "", false
	}

	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "EqualFold" {
		return "", false
	}

	pkgIdent, ok := sel.X.(*ast.Ident)
	if !ok || pkgIdent.Name != "strings" {
		return "", false
	}

	lit, ok := call.Args[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}

	v, err := strconv.Unquote(lit.Value)

	return v, err == nil
}

// serviceModuleTruth is every resolved SDK module's ground truth for one
// service directory, keyed by module name.
type serviceModuleTruth struct {
	Modules map[string]*moduleGroundTruth
}

func buildServiceModuleTruth(
	cache string,
	mods []string,
	goModVersions map[string]string,
) (*serviceModuleTruth, error) {
	smt := &serviceModuleTruth{Modules: map[string]*moduleGroundTruth{}}

	for _, mod := range mods {
		ver, ok := goModVersions[mod]
		if !ok {
			continue
		}

		modPath := filepath.Join(cache, "github.com", "aws", "aws-sdk-go-v2", "service", mod+"@"+ver)

		exists, statErr := fileExists(modPath)
		if statErr != nil {
			return nil, statErr
		}

		if !exists {
			continue
		}

		mgt, err := loadModuleGroundTruth(modPath)
		if err != nil {
			return nil, err
		}

		if len(mgt.OpFuncs) == 0 && len(mgt.AllCodes) == 0 {
			continue
		}

		smt.Modules[mod] = mgt
	}

	return smt, nil
}

// sparseModuleThreshold mirrors cmd/errcodeaudit's moduleCodes.sparselyModeled
// (same 0.5 cutoff, same s3-class rationale): below it, most of this
// module's own deserializeOpError<Op> functions matched zero codes, so an
// operation's absence from PerOp is weak evidence of a real gap, not strong
// -- gopherstack-zofv's orphan-code class (deser.go's mgt.sparselyModeled)
// must not fire against a module this thinly parsed, or "no parseable ops"
// reads as a flood of findings instead of the noise it is.
const sparseModuleThreshold = 0.5

// sparselyModeled reports whether mgt's own deserializer matched at least
// one code for under half of its own OpFuncs -- see sparseModuleThreshold.
// A module with zero OpFuncs (cloudwatch's newer-codegen case, deser.go's
// doc comment) is never "sparse": it is excluded upstream by the
// !mgt.OpFuncs[op] guard in scan.go before this is ever consulted.
func (mgt *moduleGroundTruth) sparselyModeled() bool {
	if len(mgt.OpFuncs) == 0 {
		return false
	}

	return float64(len(mgt.PerOp))/float64(len(mgt.OpFuncs)) < sparseModuleThreshold
}

// allServiceCodes is the union of every resolved module's AllCodes -- the
// "real code somewhere in this service's SDK" universe used to separate a
// class A finding (real code, wrong operation) from class B (cmd/errcodeaudit's
// job: a code no module defines at all).
func (smt *serviceModuleTruth) allServiceCodes() map[string]bool {
	out := map[string]bool{}

	for _, mgt := range smt.Modules {
		for c := range mgt.AllCodes {
			out[c] = true
		}
	}

	return out
}

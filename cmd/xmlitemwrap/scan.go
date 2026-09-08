package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
)

type variantKind string

const (
	variantDoubleWrap variantKind = "double-wrap"
	variantNamedChild variantKind = "named-child"

	sentinelItem   = "item"
	sentinelMember = "member"
)

// sentinelTagNames are the generic per-element tag names AWS's XML-family
// protocols use for a repeated list element, never a real field name in any
// AWS-modeled type: "item" for the EC2-Query protocol (`awsEc2query_`,
// ec2 only), "member" for the classic AWS Query protocol (`awsAwsquery_`,
// 14 services per services/_PROTOCOLS.md -- rds, sns, iam, autoscaling,
// cloudformation, ...) and REST-XML's list wrapper. A slice field must be
// tagged with one of these (or "...>"+one of these) to be a candidate at
// all; a struct's single meaningful member tagged with one of these is the
// double-wrap tell, regardless of which sentinel the outer field itself
// used.
var sentinelTagNames = []string{sentinelItem, sentinelMember} //nolint:gochecknoglobals // read-only lookup table

type finding struct {
	File      string      `json:"file"`
	Path      string      `json:"path"`
	Elem      string      `json:"elem"`
	Variant   variantKind `json:"variant"`
	Line      int         `json:"line"`
	Confident bool        `json:"confident"`
}

type member struct {
	name string
	tag  string
}

// scanServices walks every package directory under root/services and
// returns every double-wrap/named-child candidate found, sorted by
// file:line.
func scanServices(root string) ([]finding, error) {
	svcRoot := filepath.Join(root, "services")

	dirs, err := packageDirs(svcRoot)
	if err != nil {
		return nil, err
	}

	var out []finding

	for _, dir := range dirs {
		found, scanErr := scanDir(dir, root)
		if scanErr != nil {
			return nil, scanErr
		}

		out = append(out, found...)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}

		return out[i].Line < out[j].Line
	})

	return out, nil
}

// packageDirs returns every directory under svcRoot that directly contains
// at least one non-test .go file, since a service can nest sub-packages
// (services/stepfunctions/asl, services/dynamodb/models, ...).
func packageDirs(svcRoot string) ([]string, error) {
	var dirs []string

	walkErr := filepath.WalkDir(svcRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() {
			return nil
		}

		hasGoFile, checkErr := dirHasGoFile(path)
		if checkErr != nil {
			return checkErr
		}

		if hasGoFile {
			dirs = append(dirs, path)
		}

		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	return dirs, nil
}

func dirHasGoFile(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}

	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") && !strings.HasSuffix(e.Name(), "_test.go") {
			return true, nil
		}
	}

	return false, nil
}

// scanDir parses every non-test .go file in dir as one package, builds a
// name->struct registry for resolving locally-declared element types, then
// examines every top-level struct declaration for the item-wrap shape.
func scanDir(dir, repoRoot string) ([]finding, error) {
	fset := token.NewFileSet()

	files, err := parseDirFiles(fset, dir)
	if err != nil {
		return nil, err
	}

	structTypes := map[string]*ast.StructType{}

	for _, f := range files {
		maps.Copy(structTypes, topLevelStructs(f))
	}

	var out []finding

	for _, f := range files {
		for name, st := range topLevelStructs(f) {
			examineStruct(st, name, structTypes, fset, repoRoot, &out)
		}
	}

	return out, nil
}

func parseDirFiles(fset *token.FileSet, dir string) ([]*ast.File, error) {
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

func topLevelStructs(f *ast.File) map[string]*ast.StructType {
	out := map[string]*ast.StructType{}

	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}

		for _, spec := range gd.Specs {
			ts, isSpec := spec.(*ast.TypeSpec)
			if !isSpec {
				continue
			}

			if st, isStruct := ts.Type.(*ast.StructType); isStruct {
				out[ts.Name.Name] = st
			}
		}
	}

	return out
}

// examineStruct walks st's own fields (not the fields of any named type a
// field merely references -- that type gets its own top-level examination
// under its own name, since it has its own top-level declaration).
func examineStruct(
	st *ast.StructType, path string, structTypes map[string]*ast.StructType,
	fset *token.FileSet, repoRoot string, out *[]finding,
) {
	if st.Fields == nil {
		return
	}

	for _, field := range st.Fields.List {
		examineField(field, path, structTypes, fset, repoRoot, out)
	}
}

func examineField(
	field *ast.Field, path string, structTypes map[string]*ast.StructType,
	fset *token.FileSet, repoRoot string, out *[]finding,
) {
	if len(field.Names) == 0 {
		return
	}

	xmlVal, hasXML := xmlTagOf(field)

	for _, id := range field.Names {
		fieldPath := path + "." + id.Name

		switch t := field.Type.(type) {
		case *ast.StructType:
			examineStruct(t, fieldPath, structTypes, fset, repoRoot, out)
		case *ast.ArrayType:
			if t.Len != nil || !hasXML {
				continue
			}

			examineListField(t, xmlVal, fieldPath, id, structTypes, fset, repoRoot, out)
		}
	}
}

// examineListField flags a slice field tagged xml:"item"/"member" (or
// "...>item"/"...>member") whose element type is a struct with exactly one
// meaningful member. When that member is ITSELF tagged with a sentinel name
// this is the double-wrap shape, which is structurally never a real AWS
// wire shape (no query/XML deserializer in this repo's pinned SDKs ever
// nests a literal <item>/<member> under another repeated-element wrapper)
// and is always reported CONFIDENT.
//
// When the member carries some other name, this is only a CANDIDATE: AWS
// itself genuinely has many single-member (and partially-implemented
// multi-member) object-list types -- confirmed live checking this tool's
// own repo-wide findings against ec2@v1.319.1/deserializers.go, where every
// one of ~19 initial named-child hits turned out to be either an exact
// match to a real single-member SDK type (types.AttributeValue,
// types.IpamOperatingRegion, types.PoolCidrBlock, ...) or an
// under-implemented real multi-member type (types.UnsuccessfulItem,
// types.CapacityReservationGroup, types.SnapshotRecycleBinInfo) -- neither
// of which decode-crashes a real client, unlike the confirmed
// double-wrap/named-child bugs this tool was built from (RunScheduledInstances
// InstanceIDSet, commit 3337c961d). A field-name suffix like "...Set" was
// tried as a confidence signal and rejected: it fires identically on both
// classes (compare InstanceIDSet, a real bug, against InstanceTypeSet from
// GetInstanceTypesFromInstanceRequirements, a real correct shape) -- there
// is no purely-syntactic signal that tells them apart. Every named-child hit
// is therefore reported as NEEDS REVIEW, never confident: distinguishing
// them requires reading the pinned SDK's deserializer for that element type,
// exactly as the confirmed bugs above were found by hand.
func examineListField(
	arr *ast.ArrayType, xmlVal, fieldPath string, id *ast.Ident,
	structTypes map[string]*ast.StructType, fset *token.FileSet, repoRoot string, out *[]finding,
) {
	if !isSentinelTag(xmlVal) {
		return
	}

	elemStruct, ok := resolveElemStruct(structTypes, arr.Elt)
	if !ok {
		return
	}

	members := meaningfulMembers(elemStruct)
	if len(members) != 1 {
		return
	}
	// xml:",chardata"/",cdata"/",innerxml" all capture the element's own
	// text or raw XML directly (no child element at all) -- the correct,
	// already-decode-safe way to wrap a plain scalar in a struct,
	// structurally equivalent to using the scalar slice directly.
	// Confirmed live: autoscaling/elb/elbv2/neptune/rds/ses all use exactly
	// this (xmlStringValue{Value string `xml:",chardata"`}) for their
	// classic-Query <member>value</member> string lists.
	if isTextCaptureTag(members[0].tag) {
		return
	}

	innerName := xmlBaseName(members[0].tag)
	pos := fset.Position(id.Pos())

	relFile, relErr := filepath.Rel(repoRoot, pos.Filename)
	if relErr != nil {
		relFile = pos.Filename
	}

	f := finding{File: relFile, Line: pos.Line, Path: fieldPath, Elem: innerName}

	if slices.Contains(sentinelTagNames, innerName) {
		f.Variant = variantDoubleWrap
		f.Confident = true
	} else {
		f.Variant = variantNamedChild
	}

	*out = append(*out, f)
}

func xmlTagOf(field *ast.Field) (string, bool) {
	if field.Tag == nil {
		return "", false
	}

	tagVal, err := strconv.Unquote(field.Tag.Value)
	if err != nil {
		return "", false
	}

	return reflect.StructTag(tagVal).Lookup("xml")
}

// isSentinelTag reports whether xmlVal names a plain sentinel element
// ("item" or "member") or a nested "...>item"/"...>member" path.
func isSentinelTag(xmlVal string) bool {
	return slices.Contains(sentinelTagNames, xmlBaseName(xmlVal))
}

// xmlBaseName returns the last path segment of an xml tag's name (before
// any comma-separated options), e.g. "cidrSet>item" -> "item",
// "instanceId,omitempty" -> "instanceId".
func xmlBaseName(xmlVal string) string {
	namePath, _, _ := strings.Cut(xmlVal, ",")
	if idx := strings.LastIndex(namePath, ">"); idx >= 0 {
		return namePath[idx+1:]
	}

	return namePath
}

func isAttrTag(xmlVal string) bool {
	return slices.Contains(strings.Split(xmlVal, ",")[1:], "attr")
}

func isTextCaptureTag(xmlVal string) bool {
	opts := strings.Split(xmlVal, ",")[1:]

	return slices.Contains(opts, "chardata") || slices.Contains(opts, "cdata") || slices.Contains(opts, "innerxml")
}

// resolveElemStruct resolves a slice element type expression to its struct
// definition: an inline anonymous struct directly, or a locally-declared
// named type looked up in structTypes. A built-in scalar (string, the
// already-fixed shape) or an externally-declared type resolves to false --
// this scanner only understands types this repo itself declares.
func resolveElemStruct(structTypes map[string]*ast.StructType, expr ast.Expr) (*ast.StructType, bool) {
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}

	switch e := expr.(type) {
	case *ast.StructType:
		return e, true
	case *ast.Ident:
		st, ok := structTypes[e.Name]

		return st, ok
	default:
		return nil, false
	}
}

// meaningfulMembers returns every field of st that actually marshals as an
// XML value member: exported, not XMLName, not an xml:"-" or xml:",attr"
// field. A field with no xml tag falls back to its Go name, matching
// encoding/xml's own default.
func meaningfulMembers(st *ast.StructType) []member {
	if st.Fields == nil {
		return nil
	}

	var out []member

	for _, field := range st.Fields.List {
		if len(field.Names) == 0 {
			continue
		}

		xmlVal, hasXML := xmlTagOf(field)
		if hasXML && (xmlVal == "-" || isAttrTag(xmlVal)) {
			continue
		}

		for _, id := range field.Names {
			if !id.IsExported() || id.Name == "XMLName" {
				continue
			}

			tag := xmlVal
			if !hasXML {
				tag = id.Name
			}

			out = append(out, member{name: id.Name, tag: tag})
		}
	}

	return out
}

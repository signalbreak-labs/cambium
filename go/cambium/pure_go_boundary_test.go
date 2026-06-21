// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/internal/yangparse"
)

func TestYangParseRawStatementPreservesCrossKindOrder(t *testing.T) {
	const module = `module cambium-gate-a {
  namespace "urn:cambium:gate-a";
  prefix cga;

  grouping g {
    leaf g1 { type string; }
    container g2 { leaf nested { type string; } }
  }

  container root {
    leaf a { type string; }
    container b { leaf x { type string; } }
    leaf-list c { type string; }
    uses g;
    list d { key "name"; leaf name { type string; } }
    choice e { case one { leaf f { type string; } } }
  }
}
`
	stmts, err := yangparse.Parse(module, "gate-a.yang")
	if err != nil {
		t.Fatalf("parse gate-a fixture: %v", err)
	}
	if len(stmts) != 1 {
		t.Fatalf("top-level statements = %d, want 1", len(stmts))
	}

	root := findStatement(stmts[0], "container", "root")
	if root == nil {
		t.Fatal("container root not found in raw statement tree")
	}
	got := statementChildKeywords(root)
	want := []string{"leaf:a", "container:b", "leaf-list:c", "uses:g", "list:d", "choice:e"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("raw statement child order = %v, want %v", got, want)
	}
}

func TestPureGoNoCGODependencyClosure(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", "./cambium", "./codegen", "./compat")
	cmd.Dir = filepath.Join(repoRoot(t), "go")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list cgo-free dependency closure failed:\n%s", out)
	}
	for _, dep := range strings.Fields(string(out)) {
		if isForbiddenBackendImport(dep) {
			t.Fatalf("default dependency closure contains backend package %q\n%s", dep, out)
		}
		if dep == "runtime/cgo" {
			t.Fatalf("default dependency closure contains cgo runtime package %q\n%s", dep, out)
		}
	}

	cmd = exec.Command("go", "list", "-deps", "-f", "{{if .CgoFiles}}{{.ImportPath}} {{.CgoFiles}}{{end}}", "./cambium", "./codegen", "./compat")
	cmd.Dir = filepath.Join(repoRoot(t), "go")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list cgo file scan failed:\n%s", out)
	}
	if strings.TrimSpace(string(out)) != "" {
		t.Fatalf("default dependency closure contains packages with cgo files:\n%s", out)
	}
	assertNoImportC(t, filepath.Join(repoRoot(t), "go", "cambium"), filepath.Join(repoRoot(t), "go", "codegen"), filepath.Join(repoRoot(t), "go", "compat"))
}

func TestDefaultPackageHasNoCGOTaggedTests(t *testing.T) {
	dir := filepath.Join(repoRoot(t), "go", "cambium")
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if hasPositiveCGOBuildTag(string(b)) {
			t.Fatalf("%s is cgo-tagged; backend tests belong under go/libyangbackend", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPureGoDefaultPackagesDoNotImportUnsafe(t *testing.T) {
	root := repoRoot(t)
	for _, dir := range []string{
		filepath.Join(root, "go", "cambium"),
		filepath.Join(root, "go", "codegen"),
		filepath.Join(root, "go", "compat"),
	} {
		pkg := parsePackageFiles(t, dir)
		for _, file := range pkg.files {
			for _, imp := range file.Imports {
				if strings.Trim(imp.Path.Value, `"`) == "unsafe" {
					t.Fatalf("%s imports unsafe", dir)
				}
			}
		}
	}
}

func TestPureGoNoGoyangDependencyClosure(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", "./cambium", "./codegen", "./compat")
	cmd.Dir = filepath.Join(repoRoot(t), "go")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list cgo-free dependency closure failed:\n%s", out)
	}
	for _, dep := range strings.Fields(string(out)) {
		if strings.HasPrefix(dep, "github.com/openconfig/goyang") {
			t.Fatalf("default dependency closure contains goyang package %q\n%s", dep, out)
		}
	}
}

// TestCoreHasNoVendoredGoyangDependency asserts the shipping schema/codegen core
// is free of the vendored goyang parser and uses Cambium's own native RFC 7950
// parser instead. compat has its own package-level guard because it still uses
// goyang as a test oracle while production code must stay Cambium-native.
func TestCoreHasNoVendoredGoyangDependency(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", "./cambium", "./codegen")
	cmd.Dir = filepath.Join(repoRoot(t), "go")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list dependency closure failed:\n%s", out)
	}
	const vendoredGoyang = "github.com/signalbreak-labs/cambium/go/internal/yangparse/upstream"
	for _, dep := range strings.Fields(string(out)) {
		if strings.HasPrefix(dep, vendoredGoyang) {
			t.Fatalf("schema/codegen core depends on vendored goyang package %q; the core must use Cambium's native parser\n%s", dep, out)
		}
	}
}

func TestVendoredGoyangAttribution(t *testing.T) {
	upstreamRoot := filepath.Join(repoRoot(t), "go", "internal", "yangparse", "upstream")
	for _, name := range []string{"LICENSE", "AUTHORS", "CONTRIBUTORS", "UPSTREAM.md"} {
		if _, err := os.Stat(filepath.Join(upstreamRoot, name)); err != nil {
			t.Fatalf("vendored goyang attribution file %s missing: %v", name, err)
		}
	}

	upstreamDoc := readTextFile(t, filepath.Join(upstreamRoot, "UPSTREAM.md"))
	for _, want := range []string{
		"github.com/openconfig/goyang",
		"v1.6.3",
		"274b3b50006c99113ae0670d8d250a4d093536cb",
		"Patch Log",
		"Parser behavior changes in this vendored copy are limited",
	} {
		if !strings.Contains(upstreamDoc, want) {
			t.Fatalf("UPSTREAM.md missing %q", want)
		}
	}

	modifiedFiles := []string{
		filepath.Join(upstreamRoot, "yang", "entry.go"),
		filepath.Join(upstreamRoot, "yang", "lex.go"),
		filepath.Join(upstreamRoot, "yang", "node.go"),
		filepath.Join(upstreamRoot, "yang", "yangtype.go"),
	}
	for _, path := range modifiedFiles {
		src := readTextFile(t, path)
		if !strings.Contains(src, "Licensed under the Apache License") {
			t.Fatalf("%s missing Apache license header", path)
		}
		if !strings.Contains(src, "Modified by signalbreak-labs for Cambium") {
			t.Fatalf("%s missing Cambium modified-file notice", path)
		}
	}
}

func TestPureGoPublicSurfaceDoesNotExposeBackendHandles(t *testing.T) {
	root := repoRoot(t)
	scanDirs := []string{
		filepath.Join(root, "go", "cambium"),
		filepath.Join(root, "go", "codegen"),
		filepath.Join(root, "go", "compat"),
	}

	for _, dir := range scanDirs {
		pkg := parsePackageFiles(t, dir)
		for _, file := range pkg.files {
			for _, decl := range file.Decls {
				switch decl := decl.(type) {
				case *ast.GenDecl:
					for _, spec := range decl.Specs {
						ts, ok := spec.(*ast.TypeSpec)
						if !ok || !ts.Name.IsExported() {
							continue
						}
						if ref, ok := forbiddenPublicTypeRef(ts.Type, pkg, map[string]bool{}); ok {
							t.Fatalf("%s: exported type %s exposes forbidden backend/C/unsafe type %s", dir, ts.Name.Name, ref)
						}
					}
				case *ast.FuncDecl:
					if !isExportedFuncOrMethod(decl) {
						continue
					}
					if ref, ok := forbiddenPublicTypeRef(decl.Type, pkg, map[string]bool{}); ok {
						t.Fatalf("%s: exported function or method %s exposes forbidden backend/C/unsafe type %s", dir, decl.Name.Name, ref)
					}
				}
			}
		}
	}
}

type parsedPackage struct {
	files            []*ast.File
	typeSpecs        map[string]*ast.TypeSpec
	forbiddenAliases map[string]string
}

func parsePackageFiles(t *testing.T, dir string) parsedPackage {
	t.Helper()

	pkg := parsedPackage{
		typeSpecs:        map[string]*ast.TypeSpec{},
		forbiddenAliases: map[string]string{},
	}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, src, parser.ParseComments)
		if err != nil {
			return err
		}
		pkg.files = append(pkg.files, file)
		for _, imp := range file.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			alias := filepath.Base(importPath)
			if imp.Name != nil {
				alias = imp.Name.Name
			}
			if importPath == "C" || importPath == "unsafe" || isForbiddenBackendImport(importPath) {
				pkg.forbiddenAliases[alias] = importPath
			}
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gen.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if ok {
					pkg.typeSpecs[ts.Name.Name] = ts
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return pkg
}

func assertNoImportC(t *testing.T, dirs ...string) {
	t.Helper()
	for _, dir := range dirs {
		pkg := parsePackageFiles(t, dir)
		for _, file := range pkg.files {
			for _, imp := range file.Imports {
				if strings.Trim(imp.Path.Value, `"`) == "C" {
					t.Fatalf("%s imports C", dir)
				}
			}
		}
	}
}

func isForbiddenBackendImport(importPath string) bool {
	for _, fragment := range []string{
		"libyang",
		"internal/libyang",
		"libyangbackend",
		"cambium-libyang",
		"go-libyang",
	} {
		if strings.Contains(importPath, fragment) {
			return true
		}
	}
	return false
}

func isExportedFuncOrMethod(fn *ast.FuncDecl) bool {
	if fn.Name.IsExported() {
		return true
	}
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return false
	}
	return receiverBaseName(fn.Recv.List[0].Type).IsExported()
}

func receiverBaseName(expr ast.Expr) *ast.Ident {
	switch x := expr.(type) {
	case *ast.Ident:
		return x
	case *ast.StarExpr:
		return receiverBaseName(x.X)
	case *ast.IndexExpr:
		return receiverBaseName(x.X)
	case *ast.IndexListExpr:
		return receiverBaseName(x.X)
	default:
		return ast.NewIdent("")
	}
}

func forbiddenPublicTypeRef(expr ast.Expr, pkg parsedPackage, seen map[string]bool) (string, bool) {
	switch x := expr.(type) {
	case nil:
		return "", false
	case *ast.Ident:
		if x.Name == "C" {
			return "C", true
		}
		ts, ok := pkg.typeSpecs[x.Name]
		if !ok || seen[x.Name] {
			return "", false
		}
		seen[x.Name] = true
		return forbiddenPublicTypeRef(ts.Type, pkg, seen)
	case *ast.SelectorExpr:
		ident, ok := x.X.(*ast.Ident)
		if !ok {
			return forbiddenPublicTypeRef(x.X, pkg, seen)
		}
		if importPath, forbidden := pkg.forbiddenAliases[ident.Name]; forbidden {
			return importPath + "." + x.Sel.Name, true
		}
		if ident.Name == "C" {
			return "C." + x.Sel.Name, true
		}
	case *ast.StarExpr:
		return forbiddenPublicTypeRef(x.X, pkg, seen)
	case *ast.ArrayType:
		return forbiddenPublicTypeRef(x.Elt, pkg, seen)
	case *ast.MapType:
		if ref, ok := forbiddenPublicTypeRef(x.Key, pkg, seen); ok {
			return ref, true
		}
		return forbiddenPublicTypeRef(x.Value, pkg, seen)
	case *ast.ChanType:
		return forbiddenPublicTypeRef(x.Value, pkg, seen)
	case *ast.Ellipsis:
		return forbiddenPublicTypeRef(x.Elt, pkg, seen)
	case *ast.ParenExpr:
		return forbiddenPublicTypeRef(x.X, pkg, seen)
	case *ast.IndexExpr:
		if ref, ok := forbiddenPublicTypeRef(x.X, pkg, seen); ok {
			return ref, true
		}
		return forbiddenPublicTypeRef(x.Index, pkg, seen)
	case *ast.IndexListExpr:
		if ref, ok := forbiddenPublicTypeRef(x.X, pkg, seen); ok {
			return ref, true
		}
		for _, index := range x.Indices {
			if ref, ok := forbiddenPublicTypeRef(index, pkg, seen); ok {
				return ref, true
			}
		}
	case *ast.FuncType:
		if ref, ok := forbiddenFieldListTypeRef(x.Params, pkg, seen); ok {
			return ref, true
		}
		return forbiddenFieldListTypeRef(x.Results, pkg, seen)
	case *ast.InterfaceType:
		return forbiddenFieldListTypeRef(x.Methods, pkg, seen)
	case *ast.StructType:
		return forbiddenFieldListTypeRef(x.Fields, pkg, seen)
	}
	return "", false
}

func forbiddenFieldListTypeRef(fields *ast.FieldList, pkg parsedPackage, seen map[string]bool) (string, bool) {
	if fields == nil {
		return "", false
	}
	for _, field := range fields.List {
		if ref, ok := forbiddenPublicTypeRef(field.Type, pkg, seen); ok {
			return ref, true
		}
	}
	return "", false
}

func TestPureGoBuildsAndLoadsSchemaWithoutCGO(t *testing.T) {
	dir := t.TempDir()
	modulePath := filepath.Join(dir, "cambium-pure-go-smoke.yang")
	const module = `module cambium-pure-go-smoke {
  namespace "urn:cambium:pure-go-smoke";
  prefix cpgs;

  container root {
    leaf z { type string; }
    container middle { leaf value { type uint32; } }
    leaf-list a { type string; }
  }
}
`
	if err := os.WriteFile(modulePath, []byte(module), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatalf("SetSearchPath: %v", err)
	}
	if err := ctx.LoadModule("cambium-pure-go-smoke"); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	mod, err := ctx.Schema("cambium-pure-go-smoke")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	root, err := mod.FindPath("/cpgs:root")
	if err != nil {
		t.Fatalf("FindPath root: %v", err)
	}
	got := schemaChildNames(root.Children())
	want := []string{"z", "middle", "a"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("children = %v, want %v", got, want)
	}
}

func TestSchemaChildrenMapDoesNotDriveOrder(t *testing.T) {
	const module = `module cambium-map-order {
  namespace "urn:cambium:map-order";
  prefix cmo;

  container root {
    leaf a { type string; }
    container b { leaf x { type string; } }
    leaf-list c { type string; }
    list d { key "name"; leaf name { type string; } }
    choice e { case one { leaf f { type string; } } }
  }
}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "cambium-map-order.yang")
	if err := os.WriteFile(path, []byte(module), 0o644); err != nil {
		t.Fatal(err)
	}

	want := []string{"a", "b", "c", "d", "e"}
	for i := 0; i < 50; i++ {
		ctx, err := cambium.NewContext()
		if err != nil {
			t.Fatalf("iteration %d: NewContext: %v", i, err)
		}
		if err := ctx.SetSearchPath(dir); err != nil {
			ctx.Close()
			t.Fatalf("iteration %d: SetSearchPath: %v", i, err)
		}
		if err := ctx.LoadModule("cambium-map-order"); err != nil {
			ctx.Close()
			t.Fatalf("iteration %d: LoadModule: %v", i, err)
		}
		mod, err := ctx.Schema("cambium-map-order")
		if err != nil {
			ctx.Close()
			t.Fatalf("iteration %d: Schema: %v", i, err)
		}
		root, err := mod.FindPath("/cmo:root")
		if err != nil {
			ctx.Close()
			t.Fatalf("iteration %d: FindPath: %v", i, err)
		}
		got := schemaChildNames(root.Children())
		if strings.Join(got, ",") != strings.Join(want, ",") {
			ctx.Close()
			t.Fatalf("iteration %d: children = %v, want %v", i, got, want)
		}
		ctx.Close()
	}
}

func TestNoSilentNoCGOBackendStub(t *testing.T) {
	root := repoRoot(t)
	scanDirs := []string{
		filepath.Join(root, "go", "cambium"),
		filepath.Join(root, "go", "codegen"),
	}

	backendNames := map[string]bool{
		"Parse":     true,
		"Serialize": true,
		"Validate":  true,
		"Diff":      true,
		"Merge":     true,
	}

	for _, dir := range scanDirs {
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			src := string(b)
			if !hasNoCGOBuildTag(src) {
				return nil
			}
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, path, b, parser.ParseComments)
			if err != nil {
				return err
			}
			for _, decl := range f.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok {
					continue
				}
				name := fn.Name.Name
				if !backendNames[name] {
					continue
				}
				if fn.Body == nil {
					continue
				}
				if returnsNilErrorWithoutWork(fn) {
					t.Fatalf("%s: %s appears to be a silent !cgo backend stub (returns nil error without real work)", path, name)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func hasNoCGOBuildTag(src string) bool {
	for _, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if !constraint.IsGoBuild(line) && !constraint.IsPlusBuild(line) {
			continue
		}
		expr, err := constraint.Parse(line)
		if err != nil {
			continue
		}
		if strings.Contains(expr.String(), "!cgo") {
			return true
		}
	}
	return false
}

func hasPositiveCGOBuildTag(src string) bool {
	for _, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if !constraint.IsGoBuild(line) && !constraint.IsPlusBuild(line) {
			continue
		}
		expr, err := constraint.Parse(line)
		if err != nil {
			continue
		}
		normalized := expr.String()
		if strings.Contains(normalized, "cgo") && !strings.Contains(normalized, "!cgo") {
			return true
		}
	}
	return false
}

func returnsNilErrorWithoutWork(fn *ast.FuncDecl) bool {
	// Heuristic: a backend stub returns a nil error as its last result and does
	// no real work in its body. Real work is any non-builtin function/method call.
	hasRealWork := false
	hasNilErrorReturn := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.ReturnStmt:
			if len(x.Results) == 0 {
				return true
			}
			last := x.Results[len(x.Results)-1]
			if ident, ok := last.(*ast.Ident); ok && ident.Name == "nil" {
				hasNilErrorReturn = true
			}
		case *ast.CallExpr:
			// Any non-builtin call is considered real work.
			if ident, ok := x.Fun.(*ast.Ident); ok && (ident.Name == "len" || ident.Name == "cap" || ident.Name == "append" || ident.Name == "make") {
				return true
			}
			hasRealWork = true
		}
		return true
	})
	return !hasRealWork && hasNilErrorReturn
}

func TestHasNoCGOBuildTag(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want bool
	}{
		{"go_build_negated", "//go:build !cgo\npackage p\n", true},
		{"plus_build_negated", "// +build !cgo\npackage p\n", true},
		{"go_build_positive", "//go:build cgo\npackage p\n", false},
		{"plus_build_positive", "// +build cgo\npackage p\n", false},
		{"compound_with_negation", "//go:build !cgo && linux\npackage p\n", true},
		{"no_tag", "package p\n", false},
		{"multiple_plus_build", "// +build linux\n// +build !cgo\npackage p\n", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasNoCGOBuildTag(tc.src); got != tc.want {
				t.Fatalf("hasNoCGOBuildTag(%q) = %v, want %v", tc.src, got, tc.want)
			}
		})
	}
}

func TestHasPositiveCGOBuildTag(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want bool
	}{
		{"go_build_positive", "//go:build cgo\npackage p\n", true},
		{"compound_positive", "//go:build cgo && linux\npackage p\n", true},
		{"plus_build_positive", "// +build cgo\npackage p\n", true},
		{"go_build_negated", "//go:build !cgo\npackage p\n", false},
		{"compound_negated", "//go:build !cgo || linux\npackage p\n", false},
		{"no_tag", "package p\n", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasPositiveCGOBuildTag(tc.src); got != tc.want {
				t.Fatalf("hasPositiveCGOBuildTag(%q) = %v, want %v", tc.src, got, tc.want)
			}
		})
	}
}

func TestReturnsNilErrorWithoutWork(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want bool
	}{
		{"nil_nil", "package p\nfunc F() (any, error) { return nil, nil }\n", true},
		{"nil_explicit_error", "package p\nfunc F() (any, error) { return nil, errors.New(\"unsupported\") }\n", false},
		{"nil_fmt_errorf", "package p\nfunc F() (any, error) { return nil, fmt.Errorf(\"unsupported: %w\", err) }\n", false},
		{"nil_error_var", "package p\nfunc F() (any, error) { return nil, err }\n", false},
		{"real_work_then_nil", "package p\nfunc F() (any, error) { work(); return nil, nil }\n", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fn := parseFuncDecl(t, tc.src, "F")
			if got := returnsNilErrorWithoutWork(fn); got != tc.want {
				t.Fatalf("returnsNilErrorWithoutWork() = %v, want %v", got, tc.want)
			}
		})
	}
}

func parseFuncDecl(t *testing.T, src, name string) *ast.FuncDecl {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, d := range f.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok && fn.Name.Name == name {
			return fn
		}
	}
	t.Fatalf("function %q not found", name)
	return nil
}

func readTextFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	return string(b)
}

func TestPureGoNoCGO(t *testing.T) {
	root := repoRoot(t)
	dir := t.TempDir()

	modDir := filepath.Join(dir, "mod")
	if err := os.MkdirAll(modDir, 0o755); err != nil {
		t.Fatal(err)
	}
	moduleFile := filepath.Join(modDir, "go.mod")
	moduleContent := `module cambium-pure-go-smoke

go 1.25.0

require github.com/signalbreak-labs/cambium/go v0.0.0

replace github.com/signalbreak-labs/cambium/go => ` + filepath.Join(root, "go") + `
`
	if err := os.WriteFile(moduleFile, []byte(moduleContent), 0o644); err != nil {
		t.Fatal(err)
	}

	yangDir := filepath.Join(modDir, "yang")
	if err := os.MkdirAll(yangDir, 0o755); err != nil {
		t.Fatal(err)
	}
	const yang = `module cambium-pure-go-smoke {
  namespace "urn:cambium:pure-go-smoke";
  prefix cpgs;

  container root {
    leaf z { type string; }
    container middle { leaf value { type uint32; } }
    leaf-list a { type string; }
  }
}
`
	if err := os.WriteFile(filepath.Join(yangDir, "cambium-pure-go-smoke.yang"), []byte(yang), 0o644); err != nil {
		t.Fatal(err)
	}
	const compatImport = `module compat-external-import {
  yang-version 1.1;
  namespace "urn:compat-external-import";
  prefix cei;
  organization "Signalbreak Labs";
  contact "ops@example.test";
  description "External compat module.";
  reference "External compat reference.";
  revision 2026-01-02 {
    description "External revision.";
  }
}
`
	if err := os.WriteFile(filepath.Join(yangDir, "compat-external-import@2026-01-02.yang"), []byte(compatImport), 0o644); err != nil {
		t.Fatal(err)
	}
	const compatSubmodule = `submodule compat-external-sub {
  belongs-to compat-external-owner { prefix ceo; }
  revision 2026-01-03;
}
`
	if err := os.WriteFile(filepath.Join(yangDir, "compat-external-sub@2026-01-03.yang"), []byte(compatSubmodule), 0o644); err != nil {
		t.Fatal(err)
	}

	mainFile := filepath.Join(modDir, "main.go")
	mainContent := `package main

import (
	"fmt"
	"os"

	"github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/compat"
)

func main() {
	ctx, err := cambium.NewContext()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath("yang"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := ctx.LoadModule("cambium-pure-go-smoke"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	mod, err := ctx.Schema("cambium-pure-go-smoke")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	root, err := mod.FindPath("/cpgs:root")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var names []string
	children := root.Children()
	for i := 0; i < children.Len(); i++ {
		c, _ := children.Get(i)
		names = append(names, c.Name())
	}
	want := "z,middle,a"
	got := ""
	for i, n := range names {
		if i > 0 {
			got += ","
		}
		got += n
	}
	if got != want {
		fmt.Fprintf(os.Stderr, "children = %s, want %s\n", got, want)
		os.Exit(1)
	}
	entry := compat.FromModule(mod)
	rootEntry := entry.Lookup("root")
	if rootEntry == nil {
		fmt.Fprintln(os.Stderr, "compat entry lookup failed")
		os.Exit(1)
	}
	var entryNode compat.Node = entry.Node
	if entryNode == nil || entryNode.Kind() != "module" || entryNode.NName() != "cambium-pure-go-smoke" {
		fmt.Fprintf(os.Stderr, "compat entry Node shape failed: %#v\n", entryNode)
		os.Exit(1)
	}
	zEntry := rootEntry.Lookup("z")
	if zEntry == nil || zEntry.Type == nil || zEntry.Type.Root == nil || zEntry.Type.Root.Name != "string" || zEntry.Type.Root.Kind != compat.Ystring {
		fmt.Fprintf(os.Stderr, "compat projected YangType root failed: %#v\n", zEntry)
		os.Exit(1)
	}
	var zEntryNode compat.Node = zEntry.Node
	if zEntryNode == nil || zEntryNode.Kind() != "leaf" || zEntryNode.NName() != "z" {
		fmt.Fprintf(os.Stderr, "compat child entry Node shape failed: %#v\n", zEntryNode)
		os.Exit(1)
	}
	entry.Augments = []*compat.Entry{{Name: "local-augment"}}
	entry.Augmented = []*compat.Entry{{Name: "merged-augment"}}
	entry.Deviations = []*compat.DeviatedEntry{{Type: compat.DeviationReplace, DeviatedPath: "/cpgs:root/cpgs:z", Entry: &compat.Entry{Name: "z"}}}
	entry.Uses = []*compat.UsesStmt{{Uses: &compat.Uses{Name: "common"}, Grouping: &compat.Entry{Name: "common-grouping"}}}
	if entry.Augments[0].Name != "local-augment" || entry.Augmented[0].Name != "merged-augment" || entry.Deviations[0].Type.String() != "replace" || entry.Uses[0].Grouping.Name != "common-grouping" || len(entry.Deviate) != 0 {
		fmt.Fprintf(os.Stderr, "compat entry augment/deviation/uses shape failed: %#v\n", entry)
		os.Exit(1)
	}
	entry.Exts = []*compat.Statement{{Keyword: "cambium-pure-go-smoke:marker", Argument: "entry", HasArgument: true}}
	entryMatches, err := compat.MatchingEntryExtensions(entry, "cambium-pure-go-smoke", "marker")
	if err != nil || len(entryMatches) != 1 || entryMatches[0].Argument != "entry" {
		fmt.Fprintf(os.Stderr, "compat matching entry extensions failed: %v %#v\n", err, entryMatches)
		os.Exit(1)
	}
	stmts, err := compat.Parse(` + "`" + `module compat-external-smoke { namespace "urn:compat-external-smoke"; prefix ces; }` + "`" + `, "compat-external-smoke.yang")
	if err != nil || len(stmts) != 1 {
		fmt.Fprintf(os.Stderr, "compat parse failed: %v\n", err)
		os.Exit(1)
	}
	astEntry := compat.ToEntry(&compat.ASTModule{Name: "compat-external-smoke", Source: stmts[0], Prefix: &compat.ASTValue{Name: "ces"}})
	if astEntry == nil || astEntry.Name != "compat-external-smoke" || astEntry.Prefix == nil || astEntry.Prefix.Name != "ces" {
		fmt.Fprintf(os.Stderr, "compat ToEntry AST module failed: %#v\n", astEntry)
		os.Exit(1)
	}
	value := &compat.Value{
		Name: "ces",
		Source: stmts[0],
		Extensions: []*compat.Statement{{Keyword: "ext:marker", Argument: "ok", HasArgument: true}},
		Description: &compat.Value{Name: "external prefix"},
	}
	var node compat.Node = value
	if node.Kind() != "string" || compat.Source(node) != "compat-external-smoke.yang:1:1" || value.Description.Name != "external prefix" {
		fmt.Fprintf(os.Stderr, "compat value node mismatch: %s %s\n", node.Kind(), compat.Source(node))
		os.Exit(1)
	}
	if compat.CamelCase("ietf-interfaces.foo") != "IETFInterfacesFoo" {
		fmt.Fprintln(os.Stderr, "compat CamelCase failed")
		os.Exit(1)
	}
	if td := compat.BaseTypedefs["string"]; td == nil || td.Type == nil || td.Type.Name != "string" {
		fmt.Fprintf(os.Stderr, "compat BaseTypedefs string failed: %#v\n", td)
		os.Exit(1)
	}
	shape := &compat.YangType{Name: "typedef-string", Kind: compat.Ystring}
	shape.Base = &compat.Type{Name: "string"}
	shape.Root = &compat.YangType{Name: "string", Kind: compat.Ystring}
	if shape.Base.Name != "string" || shape.Root.Kind != compat.Ystring {
		fmt.Fprintf(os.Stderr, "compat YangType Base/Root shape failed: %#v\n", shape)
		os.Exit(1)
	}
	ranges, err := compat.ParseRangesInt("1..5")
	if err != nil || !ranges.Contains(compat.YangRange{{Min: compat.FromInt(2), Max: compat.FromInt(3)}}) {
		fmt.Fprintf(os.Stderr, "compat range helper failed: %v %s\n", err, ranges)
		os.Exit(1)
	}
	astMod := &compat.ASTModule{Name: "compat-external-ast", Prefix: &compat.ASTValue{Name: "cea"}}
	astLeaf := &compat.Leaf{
		Name: "value",
		Parent: astMod,
		Extensions: []*compat.Statement{{Keyword: "cea:marker", Argument: "ok", HasArgument: true}},
	}
	astRoot := compat.RootNode(astLeaf)
	var _ *compat.Module = astRoot
	astPrefixed := compat.FindModuleByPrefix(astLeaf, "cea")
	var _ *compat.Module = astPrefixed
	if astRoot == nil || astRoot.Name != "compat-external-ast" || astRoot.GetPrefix() != "cea" || astPrefixed == nil || astPrefixed.Name != "compat-external-ast" {
		fmt.Fprintf(os.Stderr, "compat AST root/prefix helpers failed: %#v %#v\n", astRoot, astPrefixed)
		os.Exit(1)
	}
	astIdentity := &compat.ASTIdentity{Name: "transport", Parent: astMod}
	identityRoot := compat.RootNode(astIdentity)
	if astIdentity.Kind() != "identity" || identityRoot == nil || identityRoot.Name != "compat-external-ast" {
		fmt.Fprintf(os.Stderr, "compat AST identity alias failed: %#v\n", identityRoot)
		os.Exit(1)
	}
	projectedIdentity := &compat.Identity{
		Name: "transport",
		Source: &compat.Statement{Keyword: "identity", Argument: "transport", HasArgument: true},
		Parent: value,
		Extensions: []*compat.Statement{{Keyword: "ext:marker", Argument: "identity", HasArgument: true}},
	}
	var projectedIdentityNode compat.Node = projectedIdentity
	if projectedIdentityNode.Kind() != "identity" || projectedIdentityNode.NName() != "transport" || projectedIdentityNode.Statement() != projectedIdentity.Source || len(projectedIdentityNode.Exts()) != 1 {
		fmt.Fprintf(os.Stderr, "compat projected identity node shape failed: %#v\n", projectedIdentityNode)
		os.Exit(1)
	}
	matches, err := compat.MatchingExtensions(astLeaf, "compat-external-ast", "marker")
	if err != nil || len(matches) != 1 || matches[0].Argument != "ok" {
		fmt.Fprintf(os.Stderr, "compat matching extensions failed: %v %#v\n", err, matches)
		os.Exit(1)
	}
	ms := compat.NewModules()
	ms.AddPath("yang")
	foundImport := ms.FindModule(&compat.Import{Name: "compat-external-import", RevisionDate: &compat.ASTValue{Name: "2026-01-02"}})
	if foundImport == nil || foundImport.FullName() != "compat-external-import@2026-01-02" || foundImport.GetPrefix() != "cei" {
		fmt.Fprintf(os.Stderr, "compat FindModule import failed: %#v\n", foundImport)
		os.Exit(1)
	}
	var foundImportNode compat.Node = foundImport
	importEntry := compat.ToEntry(foundImportNode)
	if importEntry == nil || importEntry.Name != "compat-external-import" || importEntry.Prefix == nil || importEntry.Prefix.Name != "cei" {
		fmt.Fprintf(os.Stderr, "compat ToEntry module node failed: %#v\n", importEntry)
		os.Exit(1)
	}
	if foundImport.YangVersion == nil || foundImport.YangVersion.Name != "1.1" || foundImport.Organization == nil || foundImport.Organization.Name != "Signalbreak Labs" || foundImport.Contact == nil || foundImport.Contact.Name != "ops@example.test" || foundImport.Description == nil || foundImport.Description.Name != "External compat module." || len(foundImport.Revision) != 1 || foundImport.Revision[0].Description.Name != "External revision." {
		fmt.Fprintf(os.Stderr, "compat module metadata failed: %#v\n", foundImport)
		os.Exit(1)
	}
	var moduleIdentities []*compat.Identity = foundImport.Identity
	if moduleIdentities == nil {
		moduleIdentities = []*compat.Identity{}
	}
	var moduleNode compat.Node = foundImport
	if moduleNode.Kind() != "module" || moduleNode.NName() != "compat-external-import" || foundImport.Source == nil || foundImport.Source.Keyword != "module" {
		fmt.Fprintf(os.Stderr, "compat module node shape failed: %s %s %#v\n", moduleNode.Kind(), moduleNode.NName(), foundImport.Source)
		os.Exit(1)
	}
	foundInclude := ms.FindModule(&compat.Include{Name: "compat-external-sub", RevisionDate: &compat.ASTValue{Name: "2026-01-03"}})
	if foundInclude == nil || foundInclude.Current() != "2026-01-03" || foundInclude.GetPrefix() != "ceo" {
		fmt.Fprintf(os.Stderr, "compat FindModule include failed: %#v\n", foundInclude)
		os.Exit(1)
	}
}
`
	if err := os.WriteFile(mainFile, []byte(mainContent), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("go", "run", ".")
	cmd.Dir = modDir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pure-Go subcommand failed:\n%s\n%v", out, err)
	}
}

func TestNoEntryDirOrTypedSliceDependency(t *testing.T) {
	// Spec-named alias for the existing source-order scan.
	scanNoEntryDirOrTypedSlice(t)
}

func TestNoEntryDirOrTypedSliceTraversal(t *testing.T) {
	scanNoEntryDirOrTypedSlice(t)
}

func TestCompatEntryTraversalAPIsDoNotRangeDir(t *testing.T) {
	path := filepath.Join(repoRoot(t), "go", "compat", "entry.go")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		t.Fatalf("ParseFile(%s): %v", path, err)
	}

	traversalFuncs := map[string]bool{
		"(*Entry).Children": true,
		"(*Entry).Find":     true,
		"(*Entry).Print":    true,
		"printEntry":        true,
	}
	seen := map[string]bool{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		name := funcDeclQualifiedName(fn)
		if !traversalFuncs[name] {
			continue
		}
		seen[name] = true
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			rng, ok := n.(*ast.RangeStmt)
			if !ok || !rangesOverEntryDir(rng.X) {
				return true
			}
			pos := fset.Position(rng.Pos())
			t.Fatalf("%s ranges over Entry.Dir at %s; traversal must use ordered children", name, pos)
			return false
		})
	}
	for name := range traversalFuncs {
		if !seen[name] {
			t.Fatalf("compat traversal guard did not find %s in %s", name, path)
		}
	}
}

func scanNoEntryDirOrTypedSlice(t *testing.T) {
	root := repoRoot(t)
	scanDirs := []string{
		filepath.Join(root, "go", "cambium"),
		filepath.Join(root, "go", "codegen"),
	}
	for _, dir := range scanDirs {
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
				return err
			}
			if strings.HasSuffix(path, "_test.go") {
				return nil
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			src := string(b)
			for _, forbidden := range []string{
				".Dir",
				"Entry.Dir",
				"[]*yang.Leaf",
				"[]*yang.Container",
				"[]*yang.LeafList",
				"[]*yang.List",
				"[]*yang.Choice",
			} {
				if strings.Contains(src, forbidden) {
					t.Fatalf("%s contains forbidden ordered traversal surface %q", path, forbidden)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func funcDeclQualifiedName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	return "(" + receiverTypeString(fn.Recv.List[0].Type) + ")." + fn.Name.Name
}

func receiverTypeString(expr ast.Expr) string {
	switch x := expr.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.StarExpr:
		return "*" + receiverTypeString(x.X)
	case *ast.IndexExpr:
		return receiverTypeString(x.X)
	case *ast.IndexListExpr:
		return receiverTypeString(x.X)
	default:
		return ""
	}
}

func rangesOverEntryDir(expr ast.Expr) bool {
	switch x := expr.(type) {
	case *ast.SelectorExpr:
		return x.Sel.Name == "Dir"
	case *ast.ParenExpr:
		return rangesOverEntryDir(x.X)
	default:
		return false
	}
}

func findStatement(stmt *yangparse.Statement, keyword, arg string) *yangparse.Statement {
	for _, child := range stmt.SubStatements() {
		if child.Keyword == keyword && child.Argument == arg {
			return child
		}
		if found := findStatement(child, keyword, arg); found != nil {
			return found
		}
	}
	return nil
}

func statementChildKeywords(stmt *yangparse.Statement) []string {
	out := make([]string, 0)
	for _, child := range stmt.SubStatements() {
		switch child.Keyword {
		case "leaf", "container", "leaf-list", "uses", "list", "choice":
			out = append(out, child.Keyword+":"+child.Argument)
		}
	}
	return out
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

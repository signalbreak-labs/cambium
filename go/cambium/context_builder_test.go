package cambium_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

func TestContextBuilderLoadModuleWithFeatures(t *testing.T) {
	dir := t.TempDir()
	module := `module cambium-builder-feature {
    namespace "urn:cambium:builder-feature";
    prefix cbf;
    yang-version 1.1;

    feature advanced;

    container top {
        leaf always { type string; }
        leaf gated {
            if-feature advanced;
            type string;
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-builder-feature.yang"), []byte(module))

	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	if err := builder.SearchPath(dir); err != nil {
		t.Fatalf("SearchPath: %v", err)
	}
	if err := builder.LoadModule("cambium-builder-feature", nil, []string{"advanced"}); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()

	mod, err := ctx.Schema("cambium-builder-feature")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	top, err := mod.FindPath("/cbf:top")
	if err != nil {
		t.Fatalf("FindPath top: %v", err)
	}
	got := schemaChildNames(top.Children())
	want := []string{"always", "gated"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("top children = %v, want %v", got, want)
	}
}

func TestContextBuilderRevisionPin(t *testing.T) {
	dir := t.TempDir()
	oldModule := `module cambium-builder-revision {
    namespace "urn:cambium:builder-revision";
    prefix cbr;
    revision 2024-01-01;

    container top {
        leaf old { type string; }
    }
}
`
	newModule := `module cambium-builder-revision {
    namespace "urn:cambium:builder-revision";
    prefix cbr;
    revision 2025-01-01;

    container top {
        leaf new { type string; }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-builder-revision@2024-01-01.yang"), []byte(oldModule))
	writeModuleFile(t, filepath.Join(dir, "cambium-builder-revision@2025-01-01.yang"), []byte(newModule))

	revision := "2024-01-01"
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	if err := builder.SearchPath(dir); err != nil {
		t.Fatalf("SearchPath: %v", err)
	}
	if err := builder.LoadModule("cambium-builder-revision", &revision, nil); err != nil {
		t.Fatalf("LoadModule revision: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()

	mod, err := ctx.Schema("cambium-builder-revision")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	if got, ok := mod.Revision(); !ok || got != revision {
		t.Fatalf("Revision = (%q,%v), want %q,true", got, ok, revision)
	}
	top, err := mod.FindPath("/cbr:top")
	if err != nil {
		t.Fatalf("FindPath top: %v", err)
	}
	got := schemaChildNames(top.Children())
	want := []string{"old"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("top children = %v, want %v", got, want)
	}
}

func TestContextBuilderInvalidRevisionPinReturnsContextRuleCode(t *testing.T) {
	cases := []string{
		"2024-02-31",
		"2024-2-01",
		" 2024-01-01",
		"2024-01-01 ",
	}

	for _, revision := range cases {
		t.Run(revision, func(t *testing.T) {
			builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
			if err != nil {
				t.Fatalf("NewContextBuilder: %v", err)
			}
			err = builder.LoadModule("cambium-builder-invalid-revision", &revision, nil)
			if err == nil {
				t.Fatal("LoadModule accepted invalid revision pin")
			}
			var ce *cambium.Error
			if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
				t.Fatalf("LoadModule error = %v, want RuleCodeContext", err)
			}
			if !strings.Contains(err.Error(), `invalid revision "`+revision+`"`) {
				t.Fatalf("LoadModule error = %q, want invalid revision message", err.Error())
			}
		})
	}
}

func TestContextBuilderLoadModuleStr(t *testing.T) {
	source := `module cambium-builder-memory {
    namespace "urn:cambium:builder-memory";
    prefix cbm;

    container top {
        leaf from-memory { type string; }
    }
}
`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()

	mod, err := ctx.Schema("cambium-builder-memory")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	top, err := mod.FindPath("/cbm:top")
	if err != nil {
		t.Fatalf("FindPath top: %v", err)
	}
	got := schemaChildNames(top.Children())
	want := []string{"from-memory"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("top children = %v, want %v", got, want)
	}

	if err := builder.LoadModuleStr(source); err == nil {
		t.Fatal("builder accepted LoadModuleStr after Build")
	}
}

func TestContextBuilderRejectsDuplicateModuleIdentity(t *testing.T) {
	first := `module cambium-builder-duplicate {
    namespace "urn:cambium:builder-duplicate";
    prefix cbd;
    revision 2024-01-01;

    leaf first { type string; }
}`
	second := `module cambium-builder-duplicate {
    namespace "urn:cambium:builder-duplicate";
    prefix cbd;
    revision 2024-01-01;

    leaf second { type string; }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	if err := builder.LoadModuleStr(first); err != nil {
		t.Fatalf("Load first module: %v", err)
	}
	err = builder.LoadModuleStr(second)
	if err == nil {
		t.Fatal("LoadModuleStr accepted duplicate module identity")
	}
	var cambiumErr *cambium.Error
	if !errors.As(err, &cambiumErr) {
		t.Fatalf("LoadModuleStr error %T, want *cambium.Error", err)
	}
	if got, want := cambiumErr.RuleCode(), cambium.RuleCodeContext; got != want {
		t.Fatalf("LoadModuleStr RuleCode = %s, want %s", got, want)
	}
	if got := err.Error(); !strings.Contains(got, `module "cambium-builder-duplicate" revision "2024-01-01" already loaded`) {
		t.Fatalf("LoadModuleStr error = %q, want duplicate module identity", got)
	}
}

func TestContextBuilderLoadModuleStrRejectsDeepInput(t *testing.T) {
	var source strings.Builder
	source.WriteString(`module cambium-builder-too-deep { namespace "urn:cambium:builder-too-deep"; prefix cbtd;`)
	for i := 0; i < 10050; i++ {
		source.WriteString(" container c {")
	}
	for i := 0; i < 10050; i++ {
		source.WriteByte('}')
	}
	source.WriteByte('}')

	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	err = builder.LoadModuleStr(source.String())
	if err == nil {
		t.Fatal("LoadModuleStr accepted excessively deep YANG input")
	}
	var cambiumErr *cambium.Error
	if !errors.As(err, &cambiumErr) {
		t.Fatalf("LoadModuleStr error %T, want *cambium.Error", err)
	}
	if got, want := cambiumErr.RuleCode(), cambium.RuleCodeContext; got != want {
		t.Fatalf("LoadModuleStr RuleCode = %s, want %s", got, want)
	}
	if got := err.Error(); !strings.Contains(got, "nesting depth exceeds maximum") {
		t.Fatalf("LoadModuleStr error = %q, want nesting-depth error", got)
	}
}

func TestContextBuilderLoadModuleFromPathRejectsOversizedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cambium-builder-too-large.yang")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create oversized YANG file: %v", err)
	}
	if err := file.Truncate(64<<20 + 1); err != nil {
		_ = file.Close()
		t.Fatalf("truncate oversized YANG file: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close oversized YANG file: %v", err)
	}

	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	err = builder.LoadModuleFromPath(path)
	if err == nil {
		t.Fatal("LoadModuleFromPath accepted oversized YANG file")
	}
	var cambiumErr *cambium.Error
	if !errors.As(err, &cambiumErr) {
		t.Fatalf("LoadModuleFromPath error %T, want *cambium.Error", err)
	}
	if got, want := cambiumErr.RuleCode(), cambium.RuleCodeContext; got != want {
		t.Fatalf("LoadModuleFromPath RuleCode = %s, want %s", got, want)
	}
	if got := err.Error(); !strings.Contains(got, "exceeds maximum") {
		t.Fatalf("LoadModuleFromPath error = %q, want size-bound error", got)
	}
}

func TestContextBuilderSearchesCurrentDirectoryByDefault(t *testing.T) {
	dir := t.TempDir()
	module := `module cambium-builder-cwd {
    namespace "urn:cambium:builder-cwd";
    prefix cbcwd;

    container top {
        leaf from-cwd { type string; }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-builder-cwd.yang"), []byte(module))

	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldwd) })

	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	if err := builder.LoadModule("cambium-builder-cwd", nil, nil); err != nil {
		t.Fatalf("LoadModule from cwd: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()

	mod, err := ctx.Schema("cambium-builder-cwd")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	top, err := mod.FindPath("/cbcwd:top")
	if err != nil {
		t.Fatalf("FindPath top: %v", err)
	}
	got := schemaChildNames(top.Children())
	want := []string{"from-cwd"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("top children = %v, want %v", got, want)
	}
}

func TestContextBuilderDisableSearchdirCwd(t *testing.T) {
	dir := t.TempDir()
	module := `module cambium-builder-cwd-disabled {
    namespace "urn:cambium:builder-cwd-disabled";
    prefix cbcwdd;

    container top {
        leaf value { type string; }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-builder-cwd-disabled.yang"), []byte(module))

	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldwd) })

	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{DisableSearchdirCwd: true})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	err = builder.LoadModule("cambium-builder-cwd-disabled", nil, nil)
	if err == nil {
		t.Fatal("LoadModule unexpectedly found module in cwd with DisableSearchdirCwd")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
		t.Fatalf("LoadModule error = %v, want RuleCodeContext", err)
	}
}

func TestContextBuilderLoadModuleFromPathResolvesSiblingImport(t *testing.T) {
	dir := t.TempDir()
	target := `module cambium-builder-path-target {
    namespace "urn:cambium:builder-path-target";
    prefix cbpt;

    grouping shared {
        leaf from-target { type string; }
    }
}
`
	user := `module cambium-builder-path-user {
    namespace "urn:cambium:builder-path-user";
    prefix cbpu;

    import cambium-builder-path-target {
        prefix cbpt;
    }

    container top {
        uses cbpt:shared;
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-builder-path-target.yang"), []byte(target))
	userPath := filepath.Join(dir, "cambium-builder-path-user.yang")
	writeModuleFile(t, userPath, []byte(user))

	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	if err := builder.LoadModuleFromPath(userPath); err != nil {
		t.Fatalf("LoadModuleFromPath: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()

	mod, err := ctx.Schema("cambium-builder-path-user")
	if err != nil {
		t.Fatalf("Schema user: %v", err)
	}
	top, err := mod.FindPath("/cbpu:top")
	if err != nil {
		t.Fatalf("FindPath top: %v", err)
	}
	got := schemaChildNames(top.Children())
	want := []string{"from-target"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("top children = %v, want %v", got, want)
	}
}

func TestContextBuilderRecursiveSearchPathEllipsis(t *testing.T) {
	dir := t.TempDir()
	moduleDir := filepath.Join(dir, "vendor", "nested")
	if err := os.MkdirAll(moduleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := `module cambium-builder-recursive-target {
    namespace "urn:cambium:builder-recursive-target";
    prefix cbrt;

    grouping shared {
        leaf from-recursive-target { type string; }
    }
}
`
	user := `module cambium-builder-recursive-user {
    namespace "urn:cambium:builder-recursive-user";
    prefix cbru;

    import cambium-builder-recursive-target {
        prefix cbrt;
    }

    container top {
        uses cbrt:shared;
    }
}
`
	writeModuleFile(t, filepath.Join(moduleDir, "cambium-builder-recursive-target.yang"), []byte(target))
	writeModuleFile(t, filepath.Join(moduleDir, "cambium-builder-recursive-user.yang"), []byte(user))

	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	if err := builder.SearchPath(filepath.Join(dir, "...")); err != nil {
		t.Fatalf("SearchPath: %v", err)
	}
	if err := builder.LoadModule("cambium-builder-recursive-user", nil, nil); err != nil {
		t.Fatalf("LoadModule recursive user: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()

	mod, err := ctx.Schema("cambium-builder-recursive-user")
	if err != nil {
		t.Fatalf("Schema user: %v", err)
	}
	top, err := mod.FindPath("/cbru:top")
	if err != nil {
		t.Fatalf("FindPath top: %v", err)
	}
	got := schemaChildNames(top.Children())
	want := []string{"from-recursive-target"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("top children = %v, want %v", got, want)
	}
}

func TestLoadModuleRejectsTraversalNameBeforeLookup(t *testing.T) {
	root := t.TempDir()
	searchDir := filepath.Join(root, "search")
	if err := os.MkdirAll(searchDir, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := `module outside {
    namespace "urn:outside";
    prefix out;

    leaf value {
        type string;
    }
}
`
	writeModuleFile(t, filepath.Join(root, "outside.yang"), []byte(outside))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(searchDir); err != nil {
		t.Fatal(err)
	}

	err = ctx.LoadModule("../outside")
	if err == nil {
		t.Fatal("LoadModule accepted traversal module name")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
		t.Fatalf("LoadModule traversal error = %v, want RuleCodeContext", err)
	}
	if _, err := ctx.Schema("outside"); err == nil {
		t.Fatal("traversal load populated outside module")
	}
}

func TestLoadModuleRejectsWhitespacePaddedNameBeforeLookup(t *testing.T) {
	dir := t.TempDir()
	module := `module wanted {
    namespace "urn:wanted";
    prefix wanted;

    leaf value {
        type string;
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "wanted.yang"), []byte(module))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}

	err = ctx.LoadModule(" wanted ")
	if err == nil {
		t.Fatal("LoadModule accepted whitespace-padded module name")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
		t.Fatalf("LoadModule whitespace error = %v, want RuleCodeContext", err)
	}
	if !strings.Contains(err.Error(), `invalid module name " wanted "`) {
		t.Fatalf("LoadModule whitespace error = %q, want invalid module name", err)
	}
	if _, err := ctx.Schema("wanted"); err == nil {
		t.Fatal("whitespace-padded load populated trimmed module")
	}
}

func TestLoadModuleRejectsFileDeclaringDifferentName(t *testing.T) {
	dir := t.TempDir()
	mismatched := `module other {
    namespace "urn:other";
    prefix oth;

    leaf value {
        type string;
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "wanted.yang"), []byte(mismatched))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}

	err = ctx.LoadModule("wanted")
	if err == nil {
		t.Fatal("LoadModule accepted file that declares a different module")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
		t.Fatalf("LoadModule mismatch error = %v, want RuleCodeContext", err)
	}
	if _, err := ctx.Schema("other"); err == nil {
		t.Fatal("mismatched load populated declared module")
	}
}

func TestLoadModuleRejectsRevisionedFileWithMismatchedDeclaredRevision(t *testing.T) {
	dir := t.TempDir()
	mismatched := `module wanted {
    namespace "urn:wanted";
    prefix want;
    revision 2026-06-17;

    leaf value {
        type string;
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "wanted@2026-06-18.yang"), []byte(mismatched))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}

	err = ctx.LoadModule("wanted")
	if err == nil {
		t.Fatal("LoadModule accepted revisioned file whose declared revision does not match filename")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
		t.Fatalf("LoadModule mismatched revision error = %v, want RuleCodeContext", err)
	}
	if _, err := ctx.SchemaRevision("wanted", "2026-06-17"); err == nil {
		t.Fatal("mismatched revisioned file populated declared revision")
	}
}

func TestLoadModuleRejectsRevisionedFileWhenFilenameRevisionIsNotLatest(t *testing.T) {
	dir := t.TempDir()
	staleFilename := `module wanted-stale {
    namespace "urn:wanted-stale";
    prefix ws;
    revision 2026-06-19;
    revision 2026-06-18;
}
`
	writeModuleFile(t, filepath.Join(dir, "wanted-stale@2026-06-18.yang"), []byte(staleFilename))

	revision := "2026-06-18"
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.SearchPath(dir); err != nil {
		t.Fatal(err)
	}
	err = builder.LoadModule("wanted-stale", &revision, nil)
	if err == nil {
		t.Fatal("LoadModule accepted revisioned file whose latest declared revision differs from filename")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
		t.Fatalf("LoadModule stale filename revision error = %v, want RuleCodeContext", err)
	}
}

func TestLoadModuleRevisionPinRejectsMismatchedRevisionedCandidate(t *testing.T) {
	dir := t.TempDir()
	mismatched := `module wanted {
    namespace "urn:wanted:mismatched";
    prefix wm;
    revision 2026-06-17;
}
`
	writeModuleFile(t, filepath.Join(dir, "wanted@2026-06-18.yang"), []byte(mismatched))
	fallback := `module wanted {
    namespace "urn:wanted:fallback";
    prefix wf;
    revision 2026-06-18;
}
`
	writeModuleFile(t, filepath.Join(dir, "wanted.yang"), []byte(fallback))

	revision := "2026-06-18"
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.SearchPath(dir); err != nil {
		t.Fatal(err)
	}
	err = builder.LoadModule("wanted", &revision, nil)
	if err == nil {
		t.Fatal("LoadModule accepted fallback file after mismatched revisioned candidate")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
		t.Fatalf("LoadModule mismatched revisioned candidate error = %v, want RuleCodeContext", err)
	}
}

func TestRecursiveLoadModuleRevisionPinRejectsMismatchedRevisionedCandidate(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "nested")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mismatched := `module wanted-recursive {
    namespace "urn:wanted-recursive:mismatched";
    prefix wrm;
    revision 2026-06-17;
}
`
	writeModuleFile(t, filepath.Join(dir, "wanted-recursive@2026-06-18.yang"), []byte(mismatched))
	fallback := `module wanted-recursive {
    namespace "urn:wanted-recursive:fallback";
    prefix wrf;
    revision 2026-06-18;
}
`
	writeModuleFile(t, filepath.Join(dir, "wanted-recursive.yang"), []byte(fallback))

	revision := "2026-06-18"
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.SearchPath(filepath.Join(root, "...")); err != nil {
		t.Fatal(err)
	}
	err = builder.LoadModule("wanted-recursive", &revision, nil)
	if err == nil {
		t.Fatal("recursive LoadModule accepted fallback file after mismatched revisioned candidate")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
		t.Fatalf("recursive LoadModule mismatched revisioned candidate error = %v, want RuleCodeContext", err)
	}
}

func TestRecursiveLoadModulePrefersCurrentDirectoryPlainFile(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "a")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	current := `module recursive-current-plain {
    namespace "urn:recursive-current-plain:current";
    prefix rcp;

    leaf current {
        type string;
    }
}
`
	nestedOlder := `module recursive-current-plain {
    namespace "urn:recursive-current-plain:nested";
    prefix rcpn;
    revision 2026-06-17;

    leaf nested {
        type string;
    }
}
`
	writeModuleFile(t, filepath.Join(root, "recursive-current-plain.yang"), []byte(current))
	writeModuleFile(t, filepath.Join(nested, "recursive-current-plain@2026-06-17.yang"), []byte(nestedOlder))

	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.SearchPath(filepath.Join(root, "...")); err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModule("recursive-current-plain", nil, nil); err != nil {
		t.Fatalf("LoadModule recursive-current-plain: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()

	mod, err := ctx.Schema("recursive-current-plain")
	if err != nil {
		t.Fatalf("Schema recursive-current-plain: %v", err)
	}
	if got, ok := mod.Revision(); ok || got != "" {
		t.Fatalf("recursive current plain revision = (%q,%v), want empty,false", got, ok)
	}
	if got := mod.Namespace(); got != "urn:recursive-current-plain:current" {
		t.Fatalf("recursive current plain namespace = %q, want current directory file", got)
	}
}

func TestRecursiveLoadModulePrefersCurrentDirectoryRevisionedFile(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "a")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	currentRevisioned := `module recursive-current-revisioned {
    namespace "urn:recursive-current-revisioned:current";
    prefix rcr;
    revision 2026-06-18;

    leaf current {
        type string;
    }
}
`
	nestedPlain := `module recursive-current-revisioned {
    namespace "urn:recursive-current-revisioned:nested";
    prefix rcrn;

    leaf nested {
        type string;
    }
}
`
	writeModuleFile(t, filepath.Join(root, "recursive-current-revisioned@2026-06-18.yang"), []byte(currentRevisioned))
	writeModuleFile(t, filepath.Join(nested, "recursive-current-revisioned.yang"), []byte(nestedPlain))

	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.SearchPath(filepath.Join(root, "...")); err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModule("recursive-current-revisioned", nil, nil); err != nil {
		t.Fatalf("LoadModule recursive-current-revisioned: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()

	mod, err := ctx.Schema("recursive-current-revisioned")
	if err != nil {
		t.Fatalf("Schema recursive-current-revisioned: %v", err)
	}
	if got, ok := mod.Revision(); !ok || got != "2026-06-18" {
		t.Fatalf("recursive current revisioned revision = (%q,%v), want 2026-06-18,true", got, ok)
	}
	if got := mod.Namespace(); got != "urn:recursive-current-revisioned:current" {
		t.Fatalf("recursive current revisioned namespace = %q, want current directory revisioned file", got)
	}
}

func TestImportRejectsFileDeclaringDifferentName(t *testing.T) {
	dir := t.TempDir()
	user := `module user {
    namespace "urn:user";
    prefix usr;

    import target {
        prefix tgt;
    }

    leaf value {
        type string;
    }
}
`
	mismatched := `module other {
    namespace "urn:other";
    prefix oth;
}
`
	writeModuleFile(t, filepath.Join(dir, "user.yang"), []byte(user))
	writeModuleFile(t, filepath.Join(dir, "target.yang"), []byte(mismatched))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}

	err = ctx.LoadModule("user")
	if err == nil {
		t.Fatal("LoadModule accepted import file that declares a different module")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
		t.Fatalf("LoadModule import mismatch error = %v, want RuleCodeContext", err)
	}
	if _, err := ctx.Schema("other"); err == nil {
		t.Fatal("mismatched import populated declared module")
	}
	if _, err := ctx.Schema("user"); err == nil {
		t.Fatal("failed import left requesting module visible")
	}
}

func TestFailedLoadRollsBackLoadedDependencies(t *testing.T) {
	dir := t.TempDir()
	user := `module user {
    namespace "urn:user";
    prefix usr;

    import good {
        prefix good;
    }
    import bad {
        prefix bad;
    }
}
`
	good := `module good {
    namespace "urn:good";
    prefix good;
}
`
	bad := `module other {
    namespace "urn:other";
    prefix other;
}
`
	writeModuleFile(t, filepath.Join(dir, "user.yang"), []byte(user))
	writeModuleFile(t, filepath.Join(dir, "good.yang"), []byte(good))
	writeModuleFile(t, filepath.Join(dir, "bad.yang"), []byte(bad))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}

	if err := ctx.LoadModule("user"); err == nil {
		t.Fatal("LoadModule accepted module with failing second import")
	}
	if _, err := ctx.Schema("user"); err == nil {
		t.Fatal("failed load left requesting module visible")
	}
	if _, err := ctx.Schema("good"); err == nil {
		t.Fatal("failed load left previously loaded dependency visible")
	}
}

func TestLoadModuleRejectsSubmoduleFile(t *testing.T) {
	dir := t.TempDir()
	submodule := `submodule child-submodule {
    belongs-to parent-module {
        prefix pm;
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "child-submodule.yang"), []byte(submodule))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}

	err = ctx.LoadModule("child-submodule")
	if err == nil {
		t.Fatal("LoadModule accepted a submodule file")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
		t.Fatalf("LoadModule submodule error = %v, want RuleCodeContext", err)
	}
	if _, err := ctx.Schema("parent-module"); err == nil {
		t.Fatal("submodule load populated parent placeholder")
	}
}

func TestDirectLoadRejectsValidSubmoduleSource(t *testing.T) {
	dir := t.TempDir()
	submodule := `submodule valid-direct-submodule {
    belongs-to valid-direct-parent {
        prefix vdp;
    }

    leaf value {
        type string;
    }
}
`
	path := filepath.Join(dir, "valid-direct-submodule.yang")
	writeModuleFile(t, path, []byte(submodule))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()

	err = ctx.LoadModuleFromPath(path)
	if err == nil {
		t.Fatal("LoadModuleFromPath accepted a direct submodule source")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
		t.Fatalf("LoadModuleFromPath submodule error = %v, want RuleCodeContext", err)
	}
	if _, ok := ctx.GetModule("valid-direct-parent", nil); ok {
		t.Fatal("direct submodule load populated parent placeholder")
	}

	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(submodule); err == nil {
		t.Fatal("ContextBuilder.LoadModuleStr accepted a direct submodule source")
	}
}

func TestLoadModuleFromPathDoesNotKeepSearchPathOnFailure(t *testing.T) {
	dir := t.TempDir()
	invalid := `module invalid-direct-path {
    prefix idp;
}
`
	invalidPath := filepath.Join(dir, "invalid-direct-path.yang")
	writeModuleFile(t, invalidPath, []byte(invalid))
	target := `module sibling-target {
    namespace "urn:sibling-target";
    prefix st;
}
`
	writeModuleFile(t, filepath.Join(dir, "sibling-target.yang"), []byte(target))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()

	if err := ctx.LoadModuleFromPath(invalidPath); err == nil {
		t.Fatal("LoadModuleFromPath accepted invalid module")
	}
	if got := ctx.SearchPaths(); len(got) != 0 {
		t.Fatalf("SearchPaths after failed LoadModuleFromPath = %v, want empty", got)
	}
	if err := ctx.LoadModule("sibling-target"); err == nil {
		t.Fatal("failed LoadModuleFromPath kept a search path that allowed sibling lookup")
	}
}

func TestSetFeaturesRejectsMalformedNames(t *testing.T) {
	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()

	cases := []struct {
		name string
		fn   func() error
	}{
		{name: "context module traversal", fn: func() error { return ctx.SetFeatures("../outside", []string{"enabled"}) }},
		{name: "context feature traversal", fn: func() error { return ctx.SetFeatures("valid-module", []string{"../enabled"}) }},
		{name: "context module whitespace", fn: func() error { return ctx.SetFeatures(" valid-module ", []string{"enabled"}) }},
		{name: "context feature whitespace", fn: func() error { return ctx.SetFeatures("valid-module", []string{" enabled "}) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.fn()
			if err == nil {
				t.Fatalf("%s accepted malformed name", tc.name)
			}
			var ce *cambium.Error
			if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
				t.Fatalf("%s error = %v, want RuleCodeContext", tc.name, err)
			}
		})
	}

	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.SetFeatures("../outside", []string{"enabled"}); err == nil {
		t.Fatal("builder SetFeatures accepted malformed module name")
	}
	if err := builder.SetFeatures("valid-module", []string{" enabled "}); err == nil {
		t.Fatal("builder SetFeatures accepted whitespace-padded feature name")
	}
}

func TestSetFeaturesRejectsMalformedReplacementWithoutMutation(t *testing.T) {
	dir := t.TempDir()
	module := `module cambium-feature-replacement {
    namespace "urn:cambium:feature-replacement";
    prefix cfr;

    feature advanced;

    container top {
        leaf base {
            type string;
        }
        leaf gated {
            if-feature advanced;
            type string;
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-feature-replacement.yang"), []byte(module))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatalf("SetSearchPath: %v", err)
	}
	if err := ctx.SetFeatures("cambium-feature-replacement", []string{"advanced"}); err != nil {
		t.Fatalf("SetFeatures initial: %v", err)
	}
	if err := ctx.SetFeatures("cambium-feature-replacement", []string{"advanced", "../bad"}); err == nil {
		t.Fatal("SetFeatures accepted malformed replacement feature")
	}
	if err := ctx.LoadModule("cambium-feature-replacement"); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	mod, err := ctx.Schema("cambium-feature-replacement")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	top, err := mod.FindPath("/cfr:top")
	if err != nil {
		t.Fatalf("FindPath top: %v", err)
	}
	if got := schemaChildNames(top.Children()); strings.Join(got, ",") != "base,gated" {
		t.Fatalf("top children = %v, want base,gated", got)
	}

	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.SearchPath(dir); err != nil {
		t.Fatalf("builder SearchPath: %v", err)
	}
	if err := builder.SetFeatures("cambium-feature-replacement", []string{"advanced"}); err != nil {
		t.Fatalf("builder SetFeatures initial: %v", err)
	}
	if err := builder.SetFeatures("cambium-feature-replacement", []string{"advanced", "../bad"}); err == nil {
		t.Fatal("builder SetFeatures accepted malformed replacement feature")
	}
	if err := builder.LoadModule("cambium-feature-replacement", nil, nil); err != nil {
		t.Fatalf("builder LoadModule: %v", err)
	}
	built, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer built.Close()
	builtMod, err := built.Schema("cambium-feature-replacement")
	if err != nil {
		t.Fatalf("built Schema: %v", err)
	}
	builtTop, err := builtMod.FindPath("/cfr:top")
	if err != nil {
		t.Fatalf("built FindPath top: %v", err)
	}
	if got := schemaChildNames(builtTop.Children()); strings.Join(got, ",") != "base,gated" {
		t.Fatalf("built top children = %v, want base,gated", got)
	}
}

func TestContextBuilderFailedLoadModuleRollsBackFeatures(t *testing.T) {
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	err = builder.LoadModule("feature-rollback", nil, []string{"advanced"})
	if err == nil {
		t.Fatal("LoadModule succeeded for missing module")
	}

	source := `module feature-rollback {
    namespace "urn:feature-rollback";
    prefix fr;

    leaf name {
        type string;
    }
}`
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr after failed LoadModule: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build after failed LoadModule retained feature state: %v", err)
	}
	defer ctx.Close()
}

func TestContextBuilderSearchPathReadbackAndUnset(t *testing.T) {
	dir := t.TempDir()
	removed := filepath.Join(dir, "removed")
	kept := filepath.Join(dir, "kept")
	if err := os.MkdirAll(removed, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(kept, 0o755); err != nil {
		t.Fatal(err)
	}
	module := `module cambium-builder-searchpath {
    namespace "urn:cambium:builder-searchpath";
    prefix cbsp;

    leaf value { type string; }
}
`
	writeModuleFile(t, filepath.Join(removed, "cambium-builder-searchpath.yang"), []byte(module))

	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	if err := builder.SearchPath(removed); err != nil {
		t.Fatalf("SearchPath removed: %v", err)
	}
	if err := builder.SearchPath(kept); err != nil {
		t.Fatalf("SearchPath kept: %v", err)
	}
	paths := builder.SearchPaths()
	wantPaths := []string{removed, kept}
	if strings.Join(paths, ",") != strings.Join(wantPaths, ",") {
		t.Fatalf("SearchPaths = %v, want %v", paths, wantPaths)
	}
	paths[0] = "mutated"
	if got := builder.SearchPaths()[0]; got != removed {
		t.Fatalf("SearchPaths exposed mutable backing store, got first path %q", got)
	}
	if err := builder.UnsetSearchPath(removed); err != nil {
		t.Fatalf("UnsetSearchPath removed: %v", err)
	}
	if got := builder.SearchPaths(); len(got) != 1 || got[0] != kept {
		t.Fatalf("SearchPaths after unset = %v, want [%s]", got, kept)
	}
	if err := builder.LoadModule("cambium-builder-searchpath", nil, nil); err == nil {
		t.Fatal("LoadModule found module in unset search path")
	}
	err = builder.UnsetSearchPath(removed)
	if err == nil {
		t.Fatal("UnsetSearchPath accepted missing search path")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
		t.Fatalf("UnsetSearchPath error = %v, want RuleCodeContext", err)
	}
}

func TestContextSearchPathReadbackAndUnset(t *testing.T) {
	dir := t.TempDir()
	removed := filepath.Join(dir, "removed")
	kept := filepath.Join(dir, "kept")
	if err := os.MkdirAll(removed, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(kept, 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(removed); err != nil {
		t.Fatalf("SetSearchPath removed: %v", err)
	}
	if err := ctx.SetSearchPath(kept); err != nil {
		t.Fatalf("SetSearchPath kept: %v", err)
	}
	paths := ctx.SearchPaths()
	wantPaths := []string{removed, kept}
	if strings.Join(paths, ",") != strings.Join(wantPaths, ",") {
		t.Fatalf("SearchPaths = %v, want %v", paths, wantPaths)
	}
	paths[1] = "mutated"
	if got := ctx.SearchPaths()[1]; got != kept {
		t.Fatalf("SearchPaths exposed mutable backing store, got second path %q", got)
	}
	if err := ctx.UnsetSearchPath(removed); err != nil {
		t.Fatalf("UnsetSearchPath: %v", err)
	}
	if got := ctx.SearchPaths(); len(got) != 1 || got[0] != kept {
		t.Fatalf("SearchPaths after unset = %v, want [%s]", got, kept)
	}
}

func TestContextModuleLookupHelpers(t *testing.T) {
	dir := t.TempDir()
	oldModule := `module cambium-builder-lookup {
    namespace "urn:cambium:builder-lookup";
    prefix cbl;
    revision 2024-01-01;

    leaf old { type string; }
}
`
	newModule := `module cambium-builder-lookup {
    namespace "urn:cambium:builder-lookup";
    prefix cbl;
    revision 2025-01-01;

    leaf new { type string; }
}
`
	target := `module cambium-builder-lookup-target {
    namespace "urn:cambium:builder-lookup-target";
    prefix cblt;

    grouping shared {
        leaf from-target { type string; }
    }
}
`
	user := `module cambium-builder-lookup-user {
    namespace "urn:cambium:builder-lookup-user";
    prefix cblu;

    import cambium-builder-lookup-target {
        prefix cblt;
    }

    container top {
        uses cblt:shared;
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-builder-lookup@2024-01-01.yang"), []byte(oldModule))
	writeModuleFile(t, filepath.Join(dir, "cambium-builder-lookup@2025-01-01.yang"), []byte(newModule))
	writeModuleFile(t, filepath.Join(dir, "cambium-builder-lookup-target.yang"), []byte(target))
	writeModuleFile(t, filepath.Join(dir, "cambium-builder-lookup-user.yang"), []byte(user))

	oldRevision := "2024-01-01"
	newRevision := "2025-01-01"
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	if err := builder.SearchPath(dir); err != nil {
		t.Fatalf("SearchPath: %v", err)
	}
	for _, load := range []struct {
		name     string
		revision *string
	}{
		{"cambium-builder-lookup", &oldRevision},
		{"cambium-builder-lookup", &newRevision},
		{"cambium-builder-lookup-user", nil},
	} {
		if err := builder.LoadModule(load.name, load.revision, nil); err != nil {
			t.Fatalf("LoadModule %s: %v", load.name, err)
		}
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()

	latest, ok := ctx.GetModule("cambium-builder-lookup", nil)
	if !ok {
		t.Fatal("GetModule latest returned false")
	}
	if got, _ := latest.Revision(); got != newRevision {
		t.Fatalf("GetModule latest revision = %q, want %q", got, newRevision)
	}
	old, ok := ctx.GetModule("cambium-builder-lookup", &oldRevision)
	if !ok {
		t.Fatal("GetModule old revision returned false")
	}
	if got, _ := old.Revision(); got != oldRevision {
		t.Fatalf("GetModule old revision = %q, want %q", got, oldRevision)
	}
	byNS, ok := ctx.FindModuleByNS("urn:cambium:builder-lookup")
	if !ok {
		t.Fatal("FindModuleByNS returned false")
	}
	if got, _ := byNS.Revision(); got != newRevision {
		t.Fatalf("FindModuleByNS revision = %q, want %q", got, newRevision)
	}
	importOnly, ok := ctx.GetModule("cambium-builder-lookup-target", nil)
	if !ok {
		t.Fatal("GetModule did not return import-only dependency")
	}
	if importOnly.IsImplemented() {
		t.Fatal("import-only lookup target should not be marked implemented")
	}
	if _, ok := ctx.GetModule("missing", nil); ok {
		t.Fatal("GetModule returned true for missing module")
	}
	if _, ok := ctx.FindModuleByNS("urn:cambium:missing"); ok {
		t.Fatal("FindModuleByNS returned true for missing namespace")
	}
}

func TestContextBuilderAllImplementedMarksImportsImplemented(t *testing.T) {
	dir := t.TempDir()
	target := `module cambium-builder-all-target {
    namespace "urn:cambium:builder-all-target";
    prefix cbat;

    leaf target-leaf { type string; }
}
`
	user := `module cambium-builder-all-user {
    namespace "urn:cambium:builder-all-user";
    prefix cbau;

    import cambium-builder-all-target {
        prefix cbat;
    }

    leaf user-leaf { type string; }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-builder-all-target.yang"), []byte(target))
	userPath := filepath.Join(dir, "cambium-builder-all-user.yang")
	writeModuleFile(t, userPath, []byte(user))

	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{AllImplemented: true})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	if err := builder.LoadModuleFromPath(userPath); err != nil {
		t.Fatalf("LoadModuleFromPath: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()

	names := make(map[string]bool)
	for _, mod := range ctx.Modules() {
		names[mod.Name()] = true
	}
	for _, name := range []string{"cambium-builder-all-user", "cambium-builder-all-target"} {
		if !names[name] {
			t.Fatalf("Modules() did not include implemented module %q; got %v", name, names)
		}
	}
}

func TestContextBuilderLeafrefTargetImportIsImplemented(t *testing.T) {
	dir := t.TempDir()
	target := `module cambium-builder-leafref-target {
    namespace "urn:cambium:builder-leafref-target";
    prefix cblrt;

    container top {
        leaf name { type string; }
    }
}
`
	user := `module cambium-builder-leafref-user {
    namespace "urn:cambium:builder-leafref-user";
    prefix cblru;

    import cambium-builder-leafref-target {
        prefix cblrt;
    }

    leaf selected {
        type leafref {
            path "/cblrt:top/cblrt:name";
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-builder-leafref-target.yang"), []byte(target))
	writeModuleFile(t, filepath.Join(dir, "cambium-builder-leafref-user.yang"), []byte(user))

	ctx := buildContextModule(t, dir, "cambium-builder-leafref-user", cambium.ContextFlags{})
	defer ctx.Close()

	names := contextModuleNameSet(ctx)
	for _, name := range []string{"cambium-builder-leafref-user", "cambium-builder-leafref-target"} {
		if !names[name] {
			t.Fatalf("Modules() did not include leafref-related implemented module %q; got %v", name, names)
		}
	}
}

func TestContextBuilderAugmentTargetImportIsImplemented(t *testing.T) {
	dir := t.TempDir()
	target := `module cambium-builder-augment-target {
    namespace "urn:cambium:builder-augment-target";
    prefix cbat;

    container top {
        leaf base { type string; }
    }
}
`
	user := `module cambium-builder-augment-user {
    namespace "urn:cambium:builder-augment-user";
    prefix cbau;

    import cambium-builder-augment-target {
        prefix cbat;
    }

    augment "/cbat:top" {
        leaf added { type string; }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-builder-augment-target.yang"), []byte(target))
	writeModuleFile(t, filepath.Join(dir, "cambium-builder-augment-user.yang"), []byte(user))

	ctx := buildContextModule(t, dir, "cambium-builder-augment-user", cambium.ContextFlags{})
	defer ctx.Close()

	names := contextModuleNameSet(ctx)
	for _, name := range []string{"cambium-builder-augment-user", "cambium-builder-augment-target"} {
		if !names[name] {
			t.Fatalf("Modules() did not include augment-related implemented module %q; got %v", name, names)
		}
	}
	mod, err := ctx.Schema("cambium-builder-augment-target")
	if err != nil {
		t.Fatalf("Schema target: %v", err)
	}
	top, err := mod.FindPath("/cbat:top")
	if err != nil {
		t.Fatalf("FindPath top: %v", err)
	}
	got := schemaChildNames(top.Children())
	want := []string{"base", "added"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("top children = %v, want %v", got, want)
	}
}

func TestContextBuilderDeviationTargetImportIsImplemented(t *testing.T) {
	dir := t.TempDir()
	target := `module cambium-builder-deviation-target {
    namespace "urn:cambium:builder-deviation-target";
    prefix cbdt;

    container top {
        leaf metric { type string; }
    }
}
`
	user := `module cambium-builder-deviation-user {
    namespace "urn:cambium:builder-deviation-user";
    prefix cbdu;

    import cambium-builder-deviation-target {
        prefix cbdt;
    }

    deviation "/cbdt:top/cbdt:metric" {
        deviate replace {
            type uint32;
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-builder-deviation-target.yang"), []byte(target))
	writeModuleFile(t, filepath.Join(dir, "cambium-builder-deviation-user.yang"), []byte(user))

	ctx := buildContextModule(t, dir, "cambium-builder-deviation-user", cambium.ContextFlags{})
	defer ctx.Close()

	names := contextModuleNameSet(ctx)
	for _, name := range []string{"cambium-builder-deviation-user", "cambium-builder-deviation-target"} {
		if !names[name] {
			t.Fatalf("Modules() did not include deviation-related implemented module %q; got %v", name, names)
		}
	}
	metric, err := ctx.Schema("cambium-builder-deviation-target")
	if err != nil {
		t.Fatalf("Schema target: %v", err)
	}
	leaf, err := metric.FindPath("/cbdt:top/cbdt:metric")
	if err != nil {
		t.Fatalf("FindPath metric: %v", err)
	}
	info, ok := leaf.LeafType()
	if !ok || info.Base() != cambium.BaseTypeUint32 {
		t.Fatalf("deviated metric type = (%v,%v), want uint32,true", info.Base(), ok)
	}
}

func TestContextBuilderRefImplementedMarksMustReferencedImport(t *testing.T) {
	dir := t.TempDir()
	target := `module cambium-builder-ref-target {
    namespace "urn:cambium:builder-ref-target";
    prefix cbrt;

    container limits {
        leaf enabled { type boolean; }
    }
}
`
	user := `module cambium-builder-ref-user {
    namespace "urn:cambium:builder-ref-user";
    prefix cbru;

    import cambium-builder-ref-target {
        prefix cbrt;
    }

    container local {
        must "/cbrt:limits/cbrt:enabled = 'true'";
        leaf value { type string; }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-builder-ref-target.yang"), []byte(target))
	writeModuleFile(t, filepath.Join(dir, "cambium-builder-ref-user.yang"), []byte(user))

	ctx := buildContextModule(t, dir, "cambium-builder-ref-user", cambium.ContextFlags{})
	names := contextModuleNameSet(ctx)
	ctx.Close()
	if names["cambium-builder-ref-target"] {
		t.Fatalf("Modules() included must-referenced import without RefImplemented: %v", names)
	}

	ctx = buildContextModule(t, dir, "cambium-builder-ref-user", cambium.ContextFlags{RefImplemented: true})
	defer ctx.Close()
	names = contextModuleNameSet(ctx)
	for _, name := range []string{"cambium-builder-ref-user", "cambium-builder-ref-target"} {
		if !names[name] {
			t.Fatalf("Modules() did not include RefImplemented module %q; got %v", name, names)
		}
	}
}

func TestContextBuilderIdentityRefDefaultRejectsImportOnlyIdentity(t *testing.T) {
	dir := t.TempDir()
	target := `module cambium-builder-identity-default-target {
    namespace "urn:cambium:builder-identity-default-target";
    prefix cbidt;

    identity base;
    identity child {
        base base;
    }
}
`
	user := `module cambium-builder-identity-default-user {
    namespace "urn:cambium:builder-identity-default-user";
    prefix cbidu;

    import cambium-builder-identity-default-target {
        prefix cbidt;
    }

    leaf value {
        type identityref {
            base cbidt:base;
        }
        default cbidt:child;
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-builder-identity-default-target.yang"), []byte(target))
	writeModuleFile(t, filepath.Join(dir, "cambium-builder-identity-default-user.yang"), []byte(user))

	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	if err := builder.SearchPath(dir); err != nil {
		t.Fatalf("SearchPath: %v", err)
	}
	if err := builder.LoadModule("cambium-builder-identity-default-user", nil, nil); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	_, err = builder.Build()
	if err == nil {
		t.Fatal("Build accepted identityref default from import-only module")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
		t.Fatalf("Build error = %v, want RuleCodeContext", err)
	}
	if got, want := err.Error(), `default "cbidt:child" references identity "child" from non-implemented module "cambium-builder-identity-default-target"`; !strings.Contains(got, want) {
		t.Fatalf("Build error = %q, want %q", got, want)
	}
}

func TestContextBuilderRefImplementedMarksIdentityRefDefaultImport(t *testing.T) {
	dir := t.TempDir()
	target := `module cambium-builder-ref-identity-target {
    namespace "urn:cambium:builder-ref-identity-target";
    prefix cbrit;

    identity base;
    identity child {
        base base;
    }
}
`
	user := `module cambium-builder-ref-identity-user {
    namespace "urn:cambium:builder-ref-identity-user";
    prefix cbriu;

    import cambium-builder-ref-identity-target {
        prefix cbrit;
    }

    leaf value {
        type identityref {
            base cbrit:base;
        }
        default cbrit:child;
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-builder-ref-identity-target.yang"), []byte(target))
	writeModuleFile(t, filepath.Join(dir, "cambium-builder-ref-identity-user.yang"), []byte(user))

	ctx := buildContextModule(t, dir, "cambium-builder-ref-identity-user", cambium.ContextFlags{RefImplemented: true})
	defer ctx.Close()
	names := contextModuleNameSet(ctx)
	for _, name := range []string{"cambium-builder-ref-identity-user", "cambium-builder-ref-identity-target"} {
		if !names[name] {
			t.Fatalf("Modules() did not include RefImplemented module %q; got %v", name, names)
		}
	}
}

func TestContextBuilderFreezesContext(t *testing.T) {
	dir := t.TempDir()
	module := `module cambium-builder-freeze {
    namespace "urn:cambium:builder-freeze";
    prefix cbfz;

    container top {
        leaf value { type string; }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-builder-freeze.yang"), []byte(module))

	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	if err := builder.SearchPath(dir); err != nil {
		t.Fatalf("SearchPath: %v", err)
	}
	if err := builder.LoadModule("cambium-builder-freeze", nil, nil); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()

	for _, err := range []error{
		ctx.SetSearchPath(dir),
		ctx.SetFeatures("cambium-builder-freeze", []string{"advanced"}),
		ctx.LoadModule("cambium-builder-freeze"),
		ctx.LoadModuleFromPath(filepath.Join(dir, "cambium-builder-freeze.yang")),
	} {
		if err == nil {
			t.Fatal("builder-produced context accepted post-build mutation")
		}
		var ce *cambium.Error
		if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
			t.Fatalf("post-build mutation error = %v, want RuleCodeContext", err)
		}
	}

	if err := builder.SearchPath(dir); err == nil {
		t.Fatal("builder accepted SearchPath after Build")
	}
	if err := builder.UnsetSearchPath(dir); err == nil {
		t.Fatal("builder accepted UnsetSearchPath after Build")
	}
	if _, err := builder.Build(); err == nil {
		t.Fatal("builder accepted second Build")
	}
}

func TestNilContextPublicMethodsReturnContextErrors(t *testing.T) {
	var ctx *cambium.Context
	mutators := []struct {
		name string
		fn   func() error
	}{
		{name: "SetSearchPath", fn: func() error { return ctx.SetSearchPath("yang") }},
		{name: "UnsetSearchPath", fn: func() error { return ctx.UnsetSearchPath("yang") }},
		{name: "SetFeatures", fn: func() error { return ctx.SetFeatures("missing", []string{"enabled"}) }},
		{name: "LoadModule", fn: func() error { return ctx.LoadModule("missing") }},
		{name: "LoadModuleFromPath", fn: func() error { return ctx.LoadModuleFromPath("missing.yang") }},
	}
	for _, tc := range mutators {
		t.Run(tc.name, func(t *testing.T) {
			err := callContextErrorWithoutPanic(t, tc.fn)
			if err == nil {
				t.Fatalf("%s returned nil error for nil context", tc.name)
			}
			var ce *cambium.Error
			if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
				t.Fatalf("%s error = %v, want RuleCodeContext", tc.name, err)
			}
		})
	}

	if _, err := ctx.Schema("missing"); err == nil {
		t.Fatal("Schema returned nil error for nil context")
	} else {
		var ce *cambium.Error
		if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
			t.Fatalf("Schema error = %v, want RuleCodeContext", err)
		}
	}
	if _, err := ctx.SchemaRevision("missing", "2026-06-18"); err == nil {
		t.Fatal("SchemaRevision returned nil error for nil context")
	} else {
		var ce *cambium.Error
		if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
			t.Fatalf("SchemaRevision error = %v, want RuleCodeContext", err)
		}
	}
	if _, ok := ctx.GetModule("missing", nil); ok {
		t.Fatal("GetModule returned ok for nil context")
	}
	if _, ok := ctx.FindModuleByNS("urn:missing"); ok {
		t.Fatal("FindModuleByNS returned ok for nil context")
	}
	if got := ctx.Modules(); got != nil {
		t.Fatalf("Modules = %v, want nil for nil context", got)
	}
}

func callContextErrorWithoutPanic(t *testing.T, fn func() error) (err error) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("public context method panicked: %v", r)
		}
	}()
	return fn()
}

func buildContextModule(t *testing.T, dir, module string, flags cambium.ContextFlags) *cambium.Context {
	t.Helper()
	builder, err := cambium.NewContextBuilder(flags)
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	if err := builder.SearchPath(dir); err != nil {
		t.Fatalf("SearchPath: %v", err)
	}
	if err := builder.LoadModule(module, nil, nil); err != nil {
		t.Fatalf("LoadModule %s: %v", module, err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return ctx
}

func contextModuleNameSet(ctx *cambium.Context) map[string]bool {
	names := make(map[string]bool)
	for _, mod := range ctx.Modules() {
		names[mod.Name()] = true
	}
	return names
}

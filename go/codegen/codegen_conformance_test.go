package codegen_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/codegen"
	"github.com/signalbreak-labs/cambium/go/internal/confmanifest"
)

func conformanceRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(file, "..", "..", "..", "conformance")
}

// tryGenerate loads a module (non-fatally) and runs GenerateGo.
func tryGenerate(moduleDir, module string) (string, error) {
	ctx, err := cambium.NewContext()
	if err != nil {
		return "", err
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(moduleDir); err != nil {
		return "", err
	}
	if err := ctx.LoadModule(module); err != nil {
		return "", err
	}
	return codegen.GenerateGo(ctx, module)
}

// G-09: bind codegen to the SHARED conformance corpus (the canonical I1-I6 gate),
// not just the hand-written per-fixture tests. For every backend-data fixture
// that follows the module/<name>.yang convention, GenerateGo MUST succeed and
// emit a non-trivial typed-struct file. This catches the G-02 regression class
// (codegen erroring/panicking/dropping on some construct) corpus-wide. Byte
// equality remains the dedicated per-fixture tests' job.
func TestCodegenGeneratesAllManifestModules(t *testing.T) {
	root := conformanceRoot(t)
	cases, err := confmanifest.Load(filepath.Join(root, "manifest.toml"))
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	covered, skipped := 0, 0
	for _, c := range cases {
		if c.EffectiveTier() != confmanifest.TierBackendData {
			continue
		}
		moduleDir := filepath.Join(root, c.Module)
		// Drive only the clear single-module convention; multi-module fixtures
		// (whose primary module name != fixture name) stay covered by the
		// bespoke byte-equality tests in codegen_test.go.
		if _, statErr := os.Stat(filepath.Join(moduleDir, c.Name+".yang")); statErr != nil {
			skipped++
			continue
		}
		src, genErr := tryGenerate(moduleDir, c.Name)
		if genErr != nil {
			t.Errorf("codegen failed for fixture %q: %v", c.Name, genErr)
			continue
		}
		if len(src) < 100 {
			t.Errorf("codegen for fixture %q produced suspiciously small output (%d bytes)", c.Name, len(src))
			continue
		}
		covered++
	}
	t.Logf("codegen conformance: generated %d manifest modules, skipped %d multi-module fixtures (covered by per-fixture tests)", covered, skipped)
	if covered == 0 {
		t.Fatal("no manifest module was codegen-covered; the manifest-driven codegen gate is not wired")
	}
}

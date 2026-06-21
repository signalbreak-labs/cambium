// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"path/filepath"
	"sort"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
	upstream "github.com/signalbreak-labs/cambium/go/internal/yangparse/upstream/yang"
)

func TestNativeMatchingExtensionsMatchGoyang(t *testing.T) {
	root := t.TempDir()
	defPath := filepath.Join(root, "native-extension-def.yang")
	userPath := filepath.Join(root, "native-extension-user.yang")
	const defSource = `module native-extension-def {
  namespace "urn:native-extension-def";
  prefix ned;

  extension marker {
    argument value;
  }

  extension flag;
}`
	const userSource = `module native-extension-user {
  namespace "urn:native-extension-user";
  prefix neu;

  import native-extension-def {
    prefix ned;
  }

  ned:marker "module-level";

  container top {
    ned:marker "node-one";
    ned:flag;
    leaf value { type string; }
  }
}`
	writeTestFile(t, defPath, defSource)
	writeTestFile(t, userPath, userSource)

	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	if err := builder.SearchPath(root); err != nil {
		t.Fatalf("SearchPath: %v", err)
	}
	if err := builder.LoadModuleFromPath(userPath); err != nil {
		t.Fatalf("LoadModuleFromPath: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	nativeMod, err := ctx.Schema("native-extension-user")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	nativeTop, ok := nativeMod.TopLevel().Lookup("top")
	if !ok {
		t.Fatal("native top not found")
	}

	goyangRoot := parseGoyangEntrySet(t, "native-extension-user", map[string]string{
		defPath:  defSource,
		userPath: userSource,
	})
	goyangTop := goyangRoot.Dir["top"]
	if goyangTop == nil {
		t.Fatal("goyang top not found")
	}

	assertNativeExtensionsMatchGoyang(t, "module marker", nativeMod.MatchingExtensions("native-extension-def", "marker"), goyangMatchingEntryExtensions(t, goyangRoot, "native-extension-def", "marker"))
	assertNativeExtensionsMatchGoyang(t, "node marker", nativeTop.MatchingExtensions("native-extension-def", "marker"), goyangMatchingEntryExtensions(t, goyangTop, "native-extension-def", "marker"))
	assertNativeExtensionsMatchGoyang(t, "node flag", nativeTop.MatchingExtensions("native-extension-def", "flag"), goyangMatchingEntryExtensions(t, goyangTop, "native-extension-def", "flag"))
	assertNativeExtensionsMatchGoyang(t, "missing", nativeTop.MatchingExtensions("native-extension-def", "missing"), goyangMatchingEntryExtensions(t, goyangTop, "native-extension-def", "missing"))
}

func assertNativeExtensionsMatchGoyang(t *testing.T, label string, native []cambium.Extension, goyang []*upstream.Statement) {
	t.Helper()
	if len(native) != len(goyang) {
		t.Fatalf("%s extension count = %d, want goyang %d", label, len(native), len(goyang))
	}
	for i := range native {
		if got, want := native[i].Name(), localNameForTest(goyang[i].Keyword); got != want {
			t.Fatalf("%s extension[%d] Name = %q, want goyang %q", label, i, got, want)
		}
		gotArg, gotHasArg := native[i].Argument()
		if gotHasArg != goyang[i].HasArgument {
			t.Fatalf("%s extension[%d] has-argument = %v, want goyang %v", label, i, gotHasArg, goyang[i].HasArgument)
		}
		if gotArg != goyang[i].Argument {
			t.Fatalf("%s extension[%d] Argument = %q, want goyang %q", label, i, gotArg, goyang[i].Argument)
		}
	}
}

func goyangMatchingEntryExtensions(t *testing.T, entry *upstream.Entry, module, identifier string) []*upstream.Statement {
	t.Helper()
	matches, err := upstream.MatchingEntryExtensions(entry, module, identifier)
	if err != nil {
		t.Fatalf("goyang MatchingEntryExtensions(%s,%s): %v", module, identifier, err)
	}
	return matches
}

func parseGoyangEntrySet(t *testing.T, moduleName string, sources map[string]string) *upstream.Entry {
	t.Helper()
	modules := upstream.NewModules()
	paths := make([]string, 0, len(sources))
	for path := range sources {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if err := modules.Parse(sources[path], path); err != nil {
			t.Fatalf("goyang Parse(%s): %v", path, err)
		}
	}
	entry, errs := modules.GetModule(moduleName)
	if len(errs) != 0 {
		t.Fatalf("goyang GetModule(%s) errors: %v", moduleName, errs)
	}
	return entry
}

func localNameForTest(keyword string) string {
	for i := range keyword {
		if keyword[i] == ':' {
			return keyword[i+1:]
		}
	}
	return keyword
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// A device enforces its own patterns, so a vendor-compatible load keeps a
// pattern Cambium cannot compile, unenforced, instead of failing. Strict loads
// still reject it.
func TestVendorCompatibleKeepsUncompilablePatternsUnenforced(t *testing.T) {
	deep := strings.Repeat("(", 300) + "a" + strings.Repeat(")", 300)
	source := `module odd-patterns {
    yang-version 1.1;
    namespace "urn:test:odd-patterns";
    prefix op;

    typedef unused-name {
        type string { pattern '[\w-.]+'; }
    }
    typedef short-name {
        type string {
            length "1..5";
            pattern 'a{,5}';
        }
    }
    grouping labelled {
        leaf label {
            type string { pattern '` + deep + `'; }
        }
    }
    container one { uses labelled; }
    container two { uses labelled; }
    leaf name {
        type short-name;
        default "abc";
    }
    leaf plain {
        type string { pattern '[a-z]+'; }
    }
}`
	load := func(mode cambium.ValidationMode) (*cambium.Context, error) {
		builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
		if err != nil {
			t.Fatal(err)
		}
		if err := builder.SetValidationMode(mode); err != nil {
			t.Fatal(err)
		}
		if err := builder.LoadModuleStr(source); err != nil {
			return nil, err
		}
		return builder.Build()
	}
	if ctx, err := load(cambium.ValidationStrict); err == nil {
		ctx.Close()
		t.Fatal("strict load accepted patterns Cambium cannot compile")
	}
	ctx, err := load(cambium.ValidationVendorCompatible)
	if err != nil {
		t.Fatalf("vendor-compatible load: %v", err)
	}
	t.Cleanup(func() { ctx.Close() })
	mod, err := ctx.Schema("odd-patterns")
	if err != nil {
		t.Fatal(err)
	}

	name := resolvedTypeFor[cambium.ResolvedString](t, mod, "/op:name")
	if len(name.Patterns) != 0 || len(name.Unenforced) != 1 || name.Unenforced[0].Regex() != "a{,5}" {
		t.Fatalf("name patterns = %v, unenforced = %v", name.Patterns, name.Unenforced)
	}
	if _, err := name.Unenforced[0].Program(); err == nil {
		t.Fatal("unenforced pattern compiled")
	}
	if len(name.Length) != 1 {
		t.Fatalf("name length = %v, want the typedef's length kept", name.Length)
	}
	for _, path := range []string{"/op:one/op:label", "/op:two/op:label"} {
		label := resolvedTypeFor[cambium.ResolvedString](t, mod, path)
		if len(label.Patterns) != 0 || len(label.Unenforced) != 1 || label.Unenforced[0].Regex() != deep {
			t.Fatalf("%s patterns = %v, unenforced = %d", path, label.Patterns, len(label.Unenforced))
		}
	}
	plain := resolvedTypeFor[cambium.ResolvedString](t, mod, "/op:plain")
	if len(plain.Patterns) != 1 || len(plain.Unenforced) != 0 {
		t.Fatalf("plain patterns = %v, unenforced = %v", plain.Patterns, plain.Unenforced)
	}

	report := ctx.LoadReport()
	var warned []string
	for _, warning := range report.Warnings {
		if strings.Contains(warning.Message, "not enforced") {
			warned = append(warned, warning.Message)
		}
	}
	if len(warned) != 3 {
		t.Fatalf("warnings = %q, want one for each pattern Cambium cannot compile", warned)
	}
	for _, source := range []string{`[\w-.]+`, "a{,5}", deep} {
		if !strings.Contains(strings.Join(warned, "\n"), strconv.Quote(source)) {
			t.Errorf("no warning names pattern %q", source)
		}
	}
	if omitted := report.OmittedContent(); len(omitted) != 0 {
		t.Fatalf("an unenforced pattern is not omitted content: %v", omitted)
	}
}

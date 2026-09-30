// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

type includeRevisionSource struct {
	filename, text string
}

func includeRevisionBuilder(t *testing.T, catalog bool, mode cambium.ValidationMode, sources []includeRevisionSource) *cambium.ContextBuilder {
	t.Helper()
	dir := t.TempDir()
	b := catalogBuilder(t)
	if err := b.SetValidationMode(mode); err != nil {
		t.Fatal(err)
	}
	var paths []string
	for i, source := range sources {
		filename := source.filename
		if catalog {
			filename = fmt.Sprintf("source-%d.yang", i)
		}
		paths = append(paths, catalogSource(t, dir, filename, source.text))
	}
	if catalog {
		if err := b.RegisterSourcePaths(paths...); err != nil {
			t.Fatal(err)
		}
	} else if err := b.SearchPath(dir); err != nil {
		t.Fatal(err)
	}
	return b
}

func includeRevisionStatement(name, revision string) string {
	if revision == "" {
		return "include " + name + ";"
	}
	return fmt.Sprintf("include %s { revision-date %s; }", name, revision)
}

func TestNestedIncludeUsesResolvedParentRevision(t *testing.T) {
	for _, mode := range []cambium.ValidationMode{cambium.ValidationStrict, cambium.ValidationVendorCompatible} {
		for _, catalog := range []bool{false, true} {
			for _, sharedFirst := range []bool{false, true} {
				for _, tc := range []struct {
					name, parentRevision, nestedRevision, wantRevision, wantLeaf string
				}{
					{"unpinned-parent-pinned-nested", "", "2025-01-01", "2025-01-01", "from-new"},
					{"pinned-parent-unpinned-nested", "2024-01-01", "", "2024-01-01", "from-old"},
					{"matching-pins", "2024-01-01", "2024-01-01", "2024-01-01", "from-old"},
				} {
					t.Run(fmt.Sprintf("mode=%d/catalog=%t/shared-first=%t/%s", mode, catalog, sharedFirst, tc.name), func(t *testing.T) {
						includes := []string{"include branch;", "include middle;", includeRevisionStatement("shared", tc.parentRevision)}
						if sharedFirst {
							slices.Reverse(includes)
						}
						b := includeRevisionBuilder(t, catalog, mode, []includeRevisionSource{
							{"parent.yang", fmt.Sprintf(`module parent {
  yang-version 1.1; namespace "urn:parent"; prefix p;
  %s
  leaf local { type string; }
}`, strings.Join(includes, "\n"))},
							{"branch.yang", `submodule branch {
  yang-version 1.1; belongs-to parent { prefix p; }
  include middle;
  leaf from-branch { type string; }
}`},
							{"middle.yang", fmt.Sprintf(`submodule middle {
  yang-version 1.1; belongs-to parent { prefix p; }
  %s
  leaf from-middle { type string; }
}`, includeRevisionStatement("shared", tc.nestedRevision))},
							{"shared@2024-01-01.yang", `submodule shared {
  yang-version 1.1; belongs-to parent { prefix p; }
  revision 2024-01-01;
  leaf from-old { type string; }
}`},
							{"shared@2025-01-01.yang", `submodule shared {
  yang-version 1.1; belongs-to parent { prefix p; }
  revision 2025-01-01;
  leaf from-new { type string; }
}`},
						})
						if err := b.LoadModule("parent", nil, nil); err != nil {
							t.Fatalf("LoadModule: %v", err)
						}
						ctx, err := b.Build()
						if err != nil {
							t.Fatal(err)
						}
						t.Cleanup(ctx.Close)
						mod, err := ctx.Schema("parent")
						if err != nil {
							t.Fatal(err)
						}
						want := []string{tc.wantLeaf, "from-middle", "from-branch", "local"}
						if got := childNamesForProfile(mod.Children()); !slices.Equal(got, want) {
							t.Errorf("effective declarations = %v, want %v", got, want)
						}
						report := ctx.LoadReport()
						var shared []cambium.SubmoduleLoadInfo
						for _, sub := range report.IncludedSubmodules {
							if sub.Name == "shared" {
								shared = append(shared, sub)
							}
						}
						if len(shared) != 1 || shared[0].Revision != tc.wantRevision {
							t.Errorf("resolved shared submodules = %+v, want one revision %s", shared, tc.wantRevision)
						}
						if len(report.Warnings) != 0 || len(report.OmittedContent()) != 0 {
							t.Errorf("valid include graph produced diagnostics: %+v", report)
						}
					})
				}
			}
		}
	}
}

func TestNestedIncludeRejectsConflictingRevisions(t *testing.T) {
	for _, version := range []string{"1", "1.1"} {
		for _, mode := range []cambium.ValidationMode{cambium.ValidationStrict, cambium.ValidationVendorCompatible} {
			for _, catalog := range []bool{false, true} {
				for _, sharedFirst := range []bool{false, true} {
					t.Run(fmt.Sprintf("yang=%s/mode=%d/catalog=%t/shared-first=%t", version, mode, catalog, sharedFirst), func(t *testing.T) {
						includes := []string{"include branch;", "include shared { revision-date 2024-01-01; }"}
						if sharedFirst {
							slices.Reverse(includes)
						}
						b := includeRevisionBuilder(t, catalog, mode, []includeRevisionSource{
							{"parent.yang", fmt.Sprintf(`module parent {
  yang-version %s; namespace "urn:parent"; prefix p; %s
}`, version, strings.Join(includes, "\n"))},
							{"branch.yang", fmt.Sprintf(`submodule branch {
  yang-version %s; belongs-to parent { prefix p; }
  include shared { revision-date 2025-01-01; }
}`, version)},
							{"shared@2024-01-01.yang", fmt.Sprintf(`submodule shared {
  yang-version %s; belongs-to parent { prefix p; }
  revision 2024-01-01; leaf old { type string; }
}`, version)},
							{"shared@2025-01-01.yang", fmt.Sprintf(`submodule shared {
  yang-version %s; belongs-to parent { prefix p; }
  revision 2025-01-01; leaf newer { type string; }
}`, version)},
						})
						err := b.LoadModule("parent", nil, nil)
						if err == nil {
							t.Fatal("LoadModule accepted conflicting explicit submodule revisions")
						}
						if !strings.Contains(err.Error(), "shared") || !strings.Contains(err.Error(), "revision") {
							t.Fatalf("LoadModule returned an unrelated error: %v", err)
						}
					})
				}
			}
		}
	}
}

func TestNestedIncludeBindingIsLocalToParentRevision(t *testing.T) {
	for _, catalog := range []bool{false, true} {
		for _, newestFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("catalog=%t/newest-first=%t", catalog, newestFirst), func(t *testing.T) {
				var sources []includeRevisionSource
				for _, year := range []string{"2024", "2025"} {
					sources = append(sources,
						includeRevisionSource{"parent@" + year + "-01-01.yang", fmt.Sprintf(`module parent {
  yang-version 1.1; namespace "urn:parent"; prefix p;
  include branch;
  include shared { revision-date %[1]s-01-01; }
  revision %[1]s-01-01;
}`, year)},
						includeRevisionSource{"shared@" + year + "-01-01.yang", fmt.Sprintf(`submodule shared {
  yang-version 1.1; belongs-to parent { prefix p; }
  revision %[1]s-01-01;
  leaf from-%[1]s { type string; }
}`, year)},
					)
				}
				sources = append(sources, includeRevisionSource{"branch.yang", `submodule branch {
  yang-version 1.1; belongs-to parent { prefix p; }
  include shared;
  leaf from-branch { type string; }
}`})
				b := includeRevisionBuilder(t, catalog, cambium.ValidationStrict, sources)
				revisions := []string{"2024-01-01", "2025-01-01"}
				if newestFirst {
					slices.Reverse(revisions)
				}
				for _, revision := range revisions {
					if err := b.LoadModule("parent", &revision, nil); err != nil {
						t.Fatal(err)
					}
				}
				ctx, err := b.Build()
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(ctx.Close)
				for _, year := range []string{"2024", "2025"} {
					mod, err := ctx.SchemaRevision("parent", year+"-01-01")
					if err != nil {
						t.Fatal(err)
					}
					want := []string{"from-" + year, "from-branch"}
					if got := childNamesForProfile(mod.Children()); !slices.Equal(got, want) {
						t.Errorf("parent revision %s declarations = %v, want %v", year, got, want)
					}
				}
				if got := len(ctx.LoadReport().IncludedSubmodules); got != 4 {
					t.Errorf("included submodules = %d, want two per parent revision", got)
				}
			})
		}
	}
}

func TestYang10TransitiveIncludeBinding(t *testing.T) {
	for _, catalog := range []bool{false, true} {
		t.Run(fmt.Sprintf("catalog=%t", catalog), func(t *testing.T) {
			b := includeRevisionBuilder(t, catalog, cambium.ValidationStrict, []includeRevisionSource{
				{"parent.yang", `module parent {
  namespace "urn:parent"; prefix p;
  include branch;
  include other;
}`},
				{"branch.yang", `submodule branch {
  belongs-to parent { prefix p; }
  include shared { revision-date 2024-01-01; }
  leaf from-branch { type string; }
}`},
				{"other.yang", `submodule other {
  belongs-to parent { prefix p; }
  include shared;
  leaf from-other { type string; }
}`},
				{"shared@2024-01-01.yang", `submodule shared {
  belongs-to parent { prefix p; }
  revision 2024-01-01; leaf from-old { type string; }
}`},
				{"shared@2025-01-01.yang", `submodule shared {
  belongs-to parent { prefix p; }
  revision 2025-01-01; leaf from-new { type string; }
}`},
			})
			if err := b.LoadModule("parent", nil, nil); err != nil {
				t.Fatal(err)
			}
			ctx, err := b.Build()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(ctx.Close)
			mod, err := ctx.Schema("parent")
			if err != nil {
				t.Fatal(err)
			}
			want := []string{"from-old", "from-branch", "from-other"}
			if got := childNamesForProfile(mod.Children()); !slices.Equal(got, want) {
				t.Errorf("transitive include declarations = %v, want %v", got, want)
			}
			if got := len(ctx.LoadReport().IncludedSubmodules); got != 3 {
				t.Errorf("included submodules = %d, want branch, shared, other", got)
			}
		})
	}
}

func TestIncludedRevisionDeclarationsAreNotSilentlyOmitted(t *testing.T) {
	for _, catalog := range []bool{false, true} {
		for _, mode := range []cambium.ValidationMode{cambium.ValidationStrict, cambium.ValidationVendorCompatible} {
			t.Run(fmt.Sprintf("catalog=%t/mode=%d", catalog, mode), func(t *testing.T) {
				b := includeRevisionBuilder(t, catalog, mode, []includeRevisionSource{
					{"parent.yang", `module parent {
  yang-version 1.1; namespace "urn:parent"; prefix p;
  include shared { revision-date 2024-01-01; }
  include branch;
}`},
					{"branch.yang", `submodule branch {
  yang-version 1.1; belongs-to parent { prefix p; }
  include shared;
}`},
					{"shared@2024-01-01.yang", `submodule shared {
  yang-version 1.1; belongs-to parent { prefix p; }
  revision 2024-01-01;
  leaf broken { type missing-type; }
}`},
					{"shared@2025-01-01.yang", `submodule shared {
  yang-version 1.1; belongs-to parent { prefix p; }
  revision 2025-01-01;
  leaf valid { type string; }
}`},
				})
				if err := b.LoadModule("parent", nil, nil); err != nil {
					t.Fatal(err)
				}
				ctx, err := b.Build()
				if err == nil {
					ctx.Close()
					t.Fatal("Build silently dropped the pinned revision's invalid declaration")
				}
				if !strings.Contains(err.Error(), "missing-type") {
					t.Fatalf("Build failed for an unrelated reason: %v", err)
				}
			})
		}
	}
}

func TestFailedIncludeResolutionPreservesLoadedParentRevision(t *testing.T) {
	b := includeRevisionBuilder(t, true, cambium.ValidationStrict, []includeRevisionSource{
		{"parent@2024-01-01.yang", `module parent {
  yang-version 1.1; namespace "urn:parent"; prefix p;
  include shared { revision-date 2024-01-01; }
  revision 2024-01-01;
}`},
		{"parent@2025-01-01.yang", `module parent {
  yang-version 1.1; namespace "urn:parent"; prefix p;
  include shared { revision-date 2025-01-01; }
  include branch;
  revision 2025-01-01;
}`},
		{"branch.yang", `submodule branch {
  yang-version 1.1; belongs-to parent { prefix p; }
  include shared { revision-date 2024-01-01; }
}`},
		{"shared@2024-01-01.yang", `submodule shared {
  yang-version 1.1; belongs-to parent { prefix p; }
  revision 2024-01-01;
  leaf retained { type string; }
}`},
		{"shared@2025-01-01.yang", `submodule shared {
  yang-version 1.1; belongs-to parent { prefix p; }
  revision 2025-01-01;
  leaf wrong { type string; }
}`},
	})
	if err := b.LoadModule("parent", new("2024-01-01"), nil); err != nil {
		t.Fatal(err)
	}
	if err := b.LoadModule("parent", new("2025-01-01"), nil); err == nil {
		t.Fatal("LoadModule accepted conflicting nested revision")
	}
	ctx, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ctx.Close)
	mod, err := ctx.Schema("parent")
	if err != nil {
		t.Fatal(err)
	}
	if revision, _ := mod.Revision(); revision != "2024-01-01" {
		t.Errorf("parent revision after failed load = %q", revision)
	}
	if got := childNamesForProfile(mod.Children()); !slices.Equal(got, []string{"retained"}) {
		t.Errorf("declarations after failed load = %v", got)
	}
	report := ctx.LoadReport()
	if len(report.RequestedModules) != 1 || len(report.IncludedSubmodules) != 1 || report.IncludedSubmodules[0].Revision != "2024-01-01" {
		t.Errorf("failed load leaked module or include bindings: %+v", report)
	}
}

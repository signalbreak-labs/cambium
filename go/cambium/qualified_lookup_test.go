// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"reflect"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

func TestSchemaChildrenLookupQualifiedDisambiguatesAugments(t *testing.T) {
	const targetSource = `module qualified-target {
  namespace "urn:qualified-target";
  prefix qt;

  container top;
}`
	const leftSource = `module qualified-left {
  namespace "urn:qualified-left";
  prefix ql;

  import qualified-target { prefix qt; }

  augment "/qt:top" {
    leaf state { type string; }
  }
}`
	const rightSource = `module qualified-right {
  namespace "urn:qualified-right";
  prefix qr;

  import qualified-target { prefix qt; }

  augment "/qt:top" {
    leaf state { type boolean; }
  }
}`

	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	for _, source := range []string{targetSource, leftSource, rightSource} {
		if err := builder.LoadModuleStr(source); err != nil {
			t.Fatalf("LoadModuleStr: %v", err)
		}
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	target, err := ctx.Schema("qualified-target")
	if err != nil {
		t.Fatalf("Schema(qualified-target): %v", err)
	}
	top, ok := target.TopLevel().Lookup("top")
	if !ok {
		t.Fatal("top not found")
	}

	children := top.Children()
	states := children.LookupAll("state")
	if states.Len() != 2 {
		t.Fatalf("LookupAll(state).Len() = %d, want 2", states.Len())
	}
	firstState, ok := states.Get(0)
	if !ok || firstState.Module().Name() != "qualified-left" {
		t.Fatalf("LookupAll(state)[0] = (%q,%v), want qualified-left", firstState.Module().Name(), ok)
	}
	secondState, ok := states.Get(1)
	if !ok || secondState.Module().Name() != "qualified-right" {
		t.Fatalf("LookupAll(state)[1] = (%q,%v), want qualified-right", secondState.Module().Name(), ok)
	}
	if states.QualifiedNames()[0] != (cambium.QualifiedName{Module: "qualified-left", Prefix: "ql", Namespace: "urn:qualified-left", Name: "state"}) {
		t.Fatalf("LookupAll(state).QualifiedNames()[0] = %#v", states.QualifiedNames()[0])
	}
	if children.LookupAll("missing").Len() != 0 {
		t.Fatal("LookupAll(missing) returned matches")
	}

	left, ok := children.LookupQualified("qualified-left", "state")
	if !ok {
		t.Fatal("LookupQualified(qualified-left, state) not found")
	}
	right, ok := children.LookupQualified("qualified-right", "state")
	if !ok {
		t.Fatal("LookupQualified(qualified-right, state) not found")
	}
	if left.Name() != "state" || right.Name() != "state" {
		t.Fatalf("qualified lookups = %q/%q, want state/state", left.Name(), right.Name())
	}
	if left.Module().Name() != "qualified-left" || right.Module().Name() != "qualified-right" {
		t.Fatalf("qualified lookup modules = %q/%q, want qualified-left/qualified-right", left.Module().Name(), right.Module().Name())
	}
	if left.QualifiedName() != (cambium.QualifiedName{Module: "qualified-left", Prefix: "ql", Namespace: "urn:qualified-left", Name: "state"}) {
		t.Fatalf("left QualifiedName = %#v", left.QualifiedName())
	}
	if right.QualifiedName() != (cambium.QualifiedName{Module: "qualified-right", Prefix: "qr", Namespace: "urn:qualified-right", Name: "state"}) {
		t.Fatalf("right QualifiedName = %#v", right.QualifiedName())
	}

	leftByQName, ok := children.LookupQualifiedName(left.QualifiedName())
	if !ok || leftByQName.Module().Name() != "qualified-left" {
		t.Fatalf("LookupQualifiedName(left) = (%#v,%v), want qualified-left", leftByQName, ok)
	}
	rightByNamespace, ok := children.LookupQualifiedName(cambium.QualifiedName{Namespace: "urn:qualified-right", Name: "state"})
	if !ok || rightByNamespace.Module().Name() != "qualified-right" {
		t.Fatalf("LookupQualifiedName(namespace right) = (%#v,%v), want qualified-right", rightByNamespace, ok)
	}
	if _, ok := children.LookupQualified("qualified-missing", "state"); ok {
		t.Fatal("LookupQualified(qualified-missing, state) succeeded")
	}

	if got, want := left.QualifiedPath(), "/qualified-target:top/qualified-left:state"; got != want {
		t.Fatalf("left QualifiedPath = %q, want %q", got, want)
	}
	if got, want := right.QualifiedPath(), "/qualified-target:top/qualified-right:state"; got != want {
		t.Fatalf("right QualifiedPath = %q, want %q", got, want)
	}
	if left.QualifiedPath() == right.QualifiedPath() {
		t.Fatalf("qualified paths collide: %q", left.QualifiedPath())
	}

	leftFromTarget, err := target.FindPath(left.QualifiedPath())
	if err != nil {
		t.Fatalf("target FindPath(left qualified path): %v", err)
	}
	if leftFromTarget.QualifiedName() != left.QualifiedName() {
		t.Fatalf("target FindPath(left qualified path) = %#v, want %#v", leftFromTarget.QualifiedName(), left.QualifiedName())
	}
	leftFromOwnModule, err := left.Module().FindPath(left.QualifiedPath())
	if err != nil {
		t.Fatalf("left module FindPath(left qualified path): %v", err)
	}
	if leftFromOwnModule.QualifiedName() != left.QualifiedName() {
		t.Fatalf("left module FindPath(left qualified path) = %#v, want %#v", leftFromOwnModule.QualifiedName(), left.QualifiedName())
	}
}

func TestCrossModuleUsesLookupByGroupingModuleQualifier(t *testing.T) {
	const provider = `module grouping-provider {
  namespace "urn:test:grouping-provider";
  prefix gp;

  grouping config-block {
    container config {
      leaf enabled { type boolean; }
    }
  }

  grouping service-top {
    container service {
      uses config-block;
      container entries {
        list entry {
          key "name";
          leaf name { type string; }
        }
      }
    }
  }
}`
	const user = `module grouping-user {
  namespace "urn:test:grouping-user";
  prefix gu;

  import grouping-provider { prefix gp; }

  container root {
    uses gp:service-top;
  }
}`

	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	for _, source := range []string{provider, user} {
		if err := builder.LoadModuleStr(source); err != nil {
			t.Fatalf("LoadModuleStr: %v", err)
		}
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	userMod, err := ctx.Schema("grouping-user")
	if err != nil {
		t.Fatalf("Schema(grouping-user): %v", err)
	}
	root, err := userMod.FindPath("/gu:root")
	if err != nil {
		t.Fatalf("FindPath root: %v", err)
	}
	serviceByUser, ok := root.Children().LookupQualified("grouping-user", "service")
	if !ok {
		t.Fatal("root children LookupQualified(grouping-user, service) not found")
	}
	if got, want := serviceByUser.QualifiedPath(), "/grouping-user:root/grouping-user:service"; got != want {
		t.Fatalf("service LookupQualified QualifiedPath = %q, want %q", got, want)
	}
	if serviceByProvider, ok := root.Children().LookupQualified("grouping-provider", "service"); ok {
		t.Fatalf("root children LookupQualified(grouping-provider, service) = %#v, want no effective-module match", serviceByProvider.QualifiedName())
	}

	for _, path := range []string{
		"/gu:root/gp:service",
		"/gu:root/gp:service/gp:config",
		"/gu:root/gp:service/gp:config/gp:enabled",
		"/gu:root/gp:service/gp:entries/gp:entry",
		"/gu:root/gp:service/gp:entries/gp:entry/gp:name",
		"/grouping-user:root/grouping-provider:service/grouping-provider:config",
		"/grouping-user:root/grouping-provider:service/grouping-provider:entries/grouping-provider:entry",
	} {
		if _, err := userMod.FindPath(path); err != nil {
			t.Fatalf("FindPath(%s): %v", path, err)
		}
	}

	for _, path := range []string{
		"/gu:root/gu:service",
		"/gu:root/gu:service/gu:config",
		"/gu:root/gu:service/gu:entries/gu:entry",
	} {
		if _, err := userMod.FindPath(path); err != nil {
			t.Fatalf("FindPath(%s): %v", path, err)
		}
	}

	entry, err := userMod.FindPath("/gu:root/gp:service/gp:entries/gp:entry")
	if err != nil {
		t.Fatalf("FindPath entry: %v", err)
	}
	if got, want := entry.QualifiedPath(), "/grouping-user:root/grouping-user:service/grouping-user:entries/grouping-user:entry"; got != want {
		t.Fatalf("entry QualifiedPath = %q, want %q", got, want)
	}
	if got, want := entry.QualifiedName().Module, "grouping-user"; got != want {
		t.Fatalf("entry QualifiedName.Module = %q, want %q", got, want)
	}
	if got, want := entry.Module().Name(), "grouping-user"; got != want {
		t.Fatalf("entry Module().Name() = %q, want %q", got, want)
	}
	if got, want := entry.SourceModule().Name(), "grouping-provider"; got != want {
		t.Fatalf("entry SourceModule().Name() = %q, want %q", got, want)
	}
	if got, want := entry.InstantiatingModule().Name(), "grouping-user"; got != want {
		t.Fatalf("entry InstantiatingModule().Name() = %q, want %q", got, want)
	}
	if got, want := entry.Namespace(), "urn:test:grouping-user"; got != want {
		t.Fatalf("entry Namespace = %q, want %q", got, want)
	}
	if got, want := entry.KeyNames(), []string{"name"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("entry KeyNames = %v, want %v", got, want)
	}
	keys := entry.ListKeys()
	if keys.Len() != 1 {
		t.Fatalf("entry ListKeys().Len() = %d, want 1", keys.Len())
	}
	key, ok := keys.Get(0)
	if !ok {
		t.Fatal("entry key missing")
	}
	if got, want := key.QualifiedPath(), "/grouping-user:root/grouping-user:service/grouping-user:entries/grouping-user:entry/grouping-user:name"; got != want {
		t.Fatalf("key QualifiedPath = %q, want %q", got, want)
	}
	if got, want := key.Module().Name(), "grouping-user"; got != want {
		t.Fatalf("key Module().Name() = %q, want %q", got, want)
	}
	if got, want := key.SourceModule().Name(), "grouping-provider"; got != want {
		t.Fatalf("key SourceModule().Name() = %q, want %q", got, want)
	}
	if got, want := key.InstantiatingModule().Name(), "grouping-user"; got != want {
		t.Fatalf("key InstantiatingModule().Name() = %q, want %q", got, want)
	}

	ir := userMod.SchemaIR()
	if len(ir.Children) != 1 || ir.Children[0].Name != "root" {
		t.Fatalf("SchemaIR root children = %v, want root", schemaIRNodeNames(ir.Children))
	}
	rootIR := ir.Children[0]
	if len(rootIR.Children) != 1 || rootIR.Children[0].Name != "service" {
		t.Fatalf("SchemaIR root children = %v, want service", schemaIRNodeNames(rootIR.Children))
	}
	serviceIR := rootIR.Children[0]
	if got, want := serviceIR.QualifiedPath, "/grouping-user:root/grouping-user:service"; got != want {
		t.Fatalf("SchemaIR service QualifiedPath = %q, want %q", got, want)
	}
	if got, want := serviceIR.Provenance.DefiningModule, "grouping-provider"; got != want {
		t.Fatalf("SchemaIR service DefiningModule = %q, want %q", got, want)
	}
	if got, want := serviceIR.Provenance.InstantiatingModule, "grouping-user"; got != want {
		t.Fatalf("SchemaIR service InstantiatingModule = %q, want %q", got, want)
	}
	if got, want := serviceIR.Provenance.Grouping, "service-top"; got != want {
		t.Fatalf("SchemaIR service Grouping = %q, want %q", got, want)
	}
	if len(serviceIR.Children) != 2 {
		t.Fatalf("SchemaIR service children = %v, want config, entries", schemaIRNodeNames(serviceIR.Children))
	}
	configIR := serviceIR.Children[0]
	if got, want := configIR.Provenance.Grouping, "config-block"; got != want {
		t.Fatalf("SchemaIR config Grouping = %q, want %q", got, want)
	}
	entriesIR := serviceIR.Children[1]
	entryIR := entriesIR.Children[0]
	if len(entryIR.ListKeys) != 1 {
		t.Fatalf("SchemaIR entry ListKeys length = %d, want 1", len(entryIR.ListKeys))
	}
	if got, want := entryIR.ListKeys[0].QualifiedPath, "/grouping-user:root/grouping-user:service/grouping-user:entries/grouping-user:entry/grouping-user:name"; got != want {
		t.Fatalf("SchemaIR key QualifiedPath = %q, want %q", got, want)
	}
}

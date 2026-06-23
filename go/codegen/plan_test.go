// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package codegen_test

import (
	"reflect"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/codegen"
)

func TestPlanExposesOrderedGenericModelBeforeRendering(t *testing.T) {
	source := `module codegen-plan-demo {
    namespace "urn:codegen-plan-demo";
    prefix cpd;

    identity transport;
    identity tcp { base transport; }

    container top {
        list iface {
            key "name vrf";
            leaf description { type string; }
            leaf name { type string; }
            leaf vrf { type string; }
            leaf protocol {
                type identityref { base transport; }
            }
        }
    }
}`
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
	t.Cleanup(func() { ctx.Close() })

	plan, err := codegen.Plan(ctx, "codegen-plan-demo")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if got, want := plan.Version, codegen.PlanVersion; got != want {
		t.Fatalf("plan version = %q, want %q", got, want)
	}
	if plan.Module.Name() != "codegen-plan-demo" {
		t.Fatalf("plan module = %q", plan.Module.Name())
	}
	if len(plan.Records) == 0 {
		t.Fatal("plan records = empty")
	}
	root := plan.Records[0]
	if root.Name != "CodegenPlanDemo" {
		t.Fatalf("root record name = %q, want CodegenPlanDemo", root.Name)
	}
	if got, want := fieldWireNames(root.Fields), []string{"top"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("root fields = %v, want %v", got, want)
	}
	iface := findRecordByPath(plan.Records, "/top/iface")
	if iface == nil {
		t.Fatal("record /top/iface not found")
	}
	if got, want := fieldWireNames(iface.Fields), []string{"name", "vrf", "description", "protocol"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("iface fields = %v, want %v", got, want)
	}
	if got, want := iface.SerializerFieldOrder, []string{"name", "vrf", "description", "protocol"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("iface serializer field order = %v, want %v", got, want)
	}
	protocol := iface.Fields[3]
	if !protocol.Type.IsIdentityRef || protocol.Type.Base != cambium.BaseTypeIdentityRef {
		t.Fatalf("protocol type plan = %#v", protocol.Type)
	}
	if got, want := identityPlanNames(plan.Identities), []string{"transport", "tcp"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("identity plans = %v, want %v", got, want)
	}
}

func fieldWireNames(fields []codegen.FieldPlan) []string {
	out := make([]string, len(fields))
	for i, field := range fields {
		out[i] = field.WireName
	}
	return out
}

func findRecordByPath(records []codegen.RecordPlan, path string) *codegen.RecordPlan {
	for i := range records {
		if records[i].Path == path {
			return &records[i]
		}
	}
	return nil
}

func identityPlanNames(identities []codegen.IdentityPlan) []string {
	out := make([]string, len(identities))
	for i, identity := range identities {
		out[i] = identity.Name
	}
	return out
}

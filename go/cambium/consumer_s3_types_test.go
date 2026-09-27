// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// S3: types, defaults, constraints and references. Each group loads its own
// small valid module so one intentional error cannot mask another.

func s3Module(t *testing.T, sources ...string) cambium.Module {
	t.Helper()
	ctx, err := buildPolicyContext(t, cambium.DeviationPolicy{}, sources...)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return ctx.Modules()[0]
}

func leafType(t *testing.T, mod cambium.Module, path string) cambium.TypeInfo {
	t.Helper()
	typ, ok := schemaNodeAt(t, mod, path).LeafType()
	if !ok {
		t.Fatalf("%s has no leaf type", path)
	}
	return typ
}

type bound struct{ min, max string }

func bounds(rs []cambium.RangeBound) []bound {
	var out []bound
	for _, r := range rs {
		out = append(out, bound{r.Min(), r.Max()})
	}
	return out
}

func TestS3ExactNumericValues(t *testing.T) {
	mod := s3Module(t, `module s3-num {
  yang-version 1.1; namespace "urn:s3:num"; prefix n;
  leaf i64 { type int64 { range "min..-5 | 5..max"; } }
  leaf u64 { type uint64 { range "0 | 10..18446744073709551615"; } }
  leaf dec { type decimal64 { fraction-digits 18; range "-9.223372036854775808..-0.000000000000000001 | 0.5..9.223372036854775807"; } }
  leaf str { type string { length "0..3 | 7 | 10..max"; } }
  leaf plain { type int64; }
}`)
	i64 := leafType(t, mod, "/n:i64").Resolved().(cambium.ResolvedInt)
	if i64.Kind != cambium.IntKindI64 || !reflect.DeepEqual(bounds(i64.Range), []bound{{"-9223372036854775808", "-5"}, {"5", "9223372036854775807"}}) {
		t.Fatalf("int64 = %v %v", i64.Kind, bounds(i64.Range))
	}
	// Bounds are exact lexical values; Number parses them without float rounding.
	lo, err := cambium.ParseInt(i64.Range[0].Min())
	if err != nil || lo.String() != "-9223372036854775808" {
		t.Fatalf("ParseInt(min) = %v, %v", lo, err)
	}
	u64 := leafType(t, mod, "/n:u64").Resolved().(cambium.ResolvedInt)
	if u64.Kind != cambium.IntKindU64 || !reflect.DeepEqual(bounds(u64.Range), []bound{{"0", "0"}, {"10", "18446744073709551615"}}) {
		t.Fatalf("uint64 = %v %v", u64.Kind, bounds(u64.Range))
	}
	hi, err := cambium.ParseInt(u64.Range[1].Max())
	if err != nil || hi.String() != "18446744073709551615" {
		t.Fatalf("ParseInt(uint64 max) = %v, %v", hi, err)
	}
	dec := leafType(t, mod, "/n:dec").Resolved().(cambium.ResolvedDecimal64)
	if dec.FractionDigits().Value() != 18 {
		t.Fatalf("fraction digits = %d", dec.FractionDigits().Value())
	}
	if got, want := bounds(dec.Range), []bound{{"-9.223372036854775808", "-0.000000000000000001"}, {"0.5", "9.223372036854775807"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("decimal64 ranges = %v, want %v", got, want)
	}
	tiny, err := cambium.ParseDecimal(dec.Range[0].Max(), 18)
	if err != nil || tiny.String() != "-0.000000000000000001" {
		t.Fatalf("ParseDecimal = %v, %v", tiny, err)
	}
	str := leafType(t, mod, "/n:str").Resolved().(cambium.ResolvedString)
	// Length bounds keep the YANG min/max keywords lexically; range bounds on
	// numeric types are resolved to the type's limits.
	if got, want := bounds(str.Length), []bound{{"0", "3"}, {"7", "7"}, {"10", "max"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("length = %v, want %v", got, want)
	}
	// An unrestricted type has no range bounds rather than invented ones.
	if plain := leafType(t, mod, "/n:plain").Resolved().(cambium.ResolvedInt); len(plain.Range) != 0 {
		t.Fatalf("unrestricted int64 range = %v", bounds(plain.Range))
	}
}

func TestS3OrderedTypeFacts(t *testing.T) {
	mod := s3Module(t, `module s3-ord {
  yang-version 1.1; namespace "urn:s3:ord"; prefix o;
  typedef base-str { type string { length "1..10"; pattern '[a-z]+' { error-app-tag "lower"; } } }
  typedef derived-str { type base-str { pattern 'x.*' { modifier invert-match; } } }
  leaf en { type enumeration { enum a { value -3; } enum b; enum c { value 10; } enum d; } }
  leaf bi { type bits { bit x { position 5; } bit y; bit z { position 0; } } }
  leaf un { type union { type int8; type union { type derived-str; type boolean; } type uint16; } }
  leaf ds { type derived-str; }
}`)
	var enums []string
	for _, v := range leafType(t, mod, "/o:en").Resolved().(cambium.ResolvedEnumeration).Values() {
		enums = append(enums, v.Name()+"="+itoa(v.Value()))
	}
	if want := []string{"a=-3", "b=-2", "c=10", "d=11"}; !reflect.DeepEqual(enums, want) {
		t.Fatalf("enums = %v, want %v", enums, want)
	}
	var bitsOut []string
	for _, v := range leafType(t, mod, "/o:bi").Resolved().(cambium.ResolvedBits).Values() {
		bitsOut = append(bitsOut, v.Name()+"="+itoa(v.Value()))
	}
	if want := []string{"x=5", "y=6", "z=0"}; !reflect.DeepEqual(bitsOut, want) {
		t.Fatalf("bits = %v, want %v", bitsOut, want)
	}
	un := leafType(t, mod, "/o:un").Resolved().(cambium.ResolvedUnion).Members()
	var members []string
	for _, m := range un {
		members = append(members, m.Base().String())
	}
	if want := []string{"int8", "union", "uint16"}; !reflect.DeepEqual(members, want) {
		t.Fatalf("union members = %v, want %v", members, want)
	}
	nested := un[1].Resolved().(cambium.ResolvedUnion).Members()
	if name, _ := nested[0].TypedefName(); name != "derived-str" || nested[1].Base() != cambium.BaseTypeBoolean {
		t.Fatalf("nested union = %q, %v", name, nested[1].Base())
	}
	ds := leafType(t, mod, "/o:ds")
	if got, want := ds.TypedefChain(), []string{"derived-str", "base-str"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("typedef chain = %v, want %v", got, want)
	}
	str := ds.Resolved().(cambium.ResolvedString)
	if len(str.Patterns) != 2 || str.Patterns[0].Regex() != "[a-z]+" || str.Patterns[0].IsInverted() || str.Patterns[1].Regex() != "x.*" || !str.Patterns[1].IsInverted() {
		t.Fatalf("patterns = %+v", str.Patterns)
	}
	if tag, ok := str.Patterns[0].ErrorAppTag(); !ok || tag != "lower" {
		t.Fatalf("inherited pattern app tag = %q,%v", tag, ok)
	}
	if !reflect.DeepEqual(bounds(str.Length), []bound{{"1", "10"}}) {
		t.Fatalf("inherited length = %v", bounds(str.Length))
	}
}

func TestS3PatternStandardsGuard(t *testing.T) {
	mk := func(pattern string) string {
		return `module s3-pat {
  yang-version 1.1; namespace "urn:s3:pat"; prefix p;
  leaf v { type string { ` + pattern + ` } }
}`
	}
	if _, err := buildPolicyContext(t, cambium.DeviationPolicy{}, mk(`pattern '\x41+';`)); err == nil {
		t.Fatal(`pattern '\x41+' accepted; \x is not an XSD escape`)
	}
	mod := s3Module(t, mk(`pattern 'A+';`))
	if p := leafType(t, mod, "/p:v").Resolved().(cambium.ResolvedString).Patterns; len(p) != 1 || p[0].Regex() != "A+" || p[0].IsInverted() {
		t.Fatalf("A+ patterns = %+v", p)
	}
	mod = s3Module(t, mk(`pattern 'A+' { modifier invert-match; }`))
	if p := leafType(t, mod, "/p:v").Resolved().(cambium.ResolvedString).Patterns; len(p) != 1 || !p[0].IsInverted() {
		t.Fatalf("invert-match patterns = %+v", p)
	}
}

func TestS3Defaults(t *testing.T) {
	const lib = `module s3-def {
  yang-version 1.1; namespace "urn:s3:def"; prefix d;
  typedef with-def { type string; default "td"; }
  grouping g { leaf refined { type string; default "from-grouping"; } }
  container top {
    leaf e1 { type string; default ""; }
    leaf e2 { type boolean; default "false"; }
    leaf e3 { type uint8; default "0"; }
    leaf none { type string; }
    leaf inherited { type with-def; }
    leaf overridden { type with-def; default "own"; }
    uses g { refine refined { default "from-refine"; } }
    leaf deviated { type string; default "before"; }
    leaf-list ll { type string; default "q"; default "a"; }
  }
}`
	const dev = `module s3-def-dev {
  yang-version 1.1; namespace "urn:s3:def-dev"; prefix dd;
  import s3-def { prefix d; }
  deviation "/d:top/d:deviated" { deviate replace { default "after"; } }
}`
	ctx, err := buildPolicyContext(t, cambium.DeviationPolicy{}, lib, dev)
	if err != nil {
		t.Fatal(err)
	}
	mod, _ := ctx.Schema("s3-def")
	cases := []struct {
		path   string
		value  string
		has    bool
		origin cambium.DefaultOrigin
	}{
		{"/d:top/e1", "", true, cambium.DefaultOriginNode},
		{"/d:top/e2", "false", true, cambium.DefaultOriginNode},
		{"/d:top/e3", "0", true, cambium.DefaultOriginNode},
		{"/d:top/none", "", false, 0},
		{"/d:top/inherited", "td", true, cambium.DefaultOriginTypedef},
		{"/d:top/overridden", "own", true, cambium.DefaultOriginNode},
		{"/d:top/refined", "from-refine", true, cambium.DefaultOriginRefine},
		{"/d:top/deviated", "after", true, cambium.DefaultOriginDeviation},
	}
	for _, tc := range cases {
		node := schemaNodeAt(t, mod, tc.path)
		entry, ok := node.DefaultEntry()
		if ok != tc.has || entry.Value() != tc.value || (ok && entry.Origin() != tc.origin) {
			t.Errorf("%s default = %q,%v origin %v; want %q,%v origin %v", tc.path, entry.Value(), ok, entry.Origin(), tc.value, tc.has, tc.origin)
		}
	}
	if entry, _ := schemaNodeAt(t, mod, "/d:top/deviated").DefaultEntry(); entry.SourceModule().Name() != "s3-def-dev" {
		t.Fatalf("deviated default source = %q", entry.SourceModule().Name())
	}
	if got := schemaNodeAt(t, mod, "/d:top/ll").DefaultValues(); !reflect.DeepEqual(got, []string{"q", "a"}) {
		t.Fatalf("leaf-list defaults = %v", got)
	}
}

func TestS3CardinalityAndMetadata(t *testing.T) {
	mod := s3Module(t, `module s3-meta {
  yang-version 1.1; namespace "urn:s3:meta"; prefix m;
  extension note { argument text; }
  container top {
    presence "top enables things";
    description "top container";
    list unbounded { key k; leaf k { type string; } max-elements unbounded; }
    list bounded { key k; leaf k { type string; } min-elements 0; max-elements 5; }
    list implicit { key k; leaf k { type string; } }
    leaf-list at-least { type string; min-elements 2; }
    leaf speed { type uint32; units "kbit/s"; status deprecated; m:note ""; mandatory true; }
    container state { config false; leaf s { type string; mandatory false; } }
    leaf obsolete-leaf { type string; status obsolete; m:note "gone"; }
  }
}`)
	top := schemaNodeAt(t, mod, "/m:top")
	if !top.IsPresenceContainer() {
		t.Fatal("top is not presence")
	}
	if d, ok := top.Description(); !ok || d != "top container" {
		t.Fatalf("description = %q,%v", d, ok)
	}
	type card struct {
		min, max       uint32
		hasMin, hasMax bool
	}
	get := func(path string) card {
		n := schemaNodeAt(t, mod, path)
		mn, hmn := n.MinElements()
		mx, hmx := n.MaxElements()
		return card{mn, mx, hmn, hmx}
	}
	// Absent bounds and explicit "unbounded" are both reported as no max.
	if got := get("/m:top/unbounded"); got.hasMax || got.hasMin {
		t.Fatalf("unbounded = %+v", got)
	}
	if got := get("/m:top/bounded"); !got.hasMin || got.min != 0 || !got.hasMax || got.max != 5 {
		t.Fatalf("bounded = %+v", got)
	}
	if got := get("/m:top/implicit"); got.hasMin || got.hasMax {
		t.Fatalf("implicit = %+v", got)
	}
	if got := get("/m:top/at-least"); !got.hasMin || got.min != 2 {
		t.Fatalf("at-least = %+v", got)
	}
	speed := schemaNodeAt(t, mod, "/m:top/speed")
	if u, ok := speed.Units(); !ok || u != "kbit/s" {
		t.Fatalf("units = %q,%v", u, ok)
	}
	if speed.Status() != cambium.StatusDeprecated || !speed.IsMandatory() {
		t.Fatalf("speed status/mandatory = %v/%v", speed.Status(), speed.IsMandatory())
	}
	// An empty extension argument is present, not absent.
	if ext, ok := speed.Extension("note"); !ok {
		t.Fatal("note extension missing")
	} else if arg, ok := ext.Argument(); !ok || arg != "" {
		t.Fatalf("empty extension argument = %q,%v", arg, ok)
	}
	s := schemaNodeAt(t, mod, "/m:top/state/s")
	if s.Config() != cambium.ConfigRo || s.IsMandatory() {
		t.Fatalf("state leaf config/mandatory = %v/%v", s.Config(), s.IsMandatory())
	}
	if schemaNodeAt(t, mod, "/m:top/obsolete-leaf").Status() != cambium.StatusObsolete {
		t.Fatal("obsolete status lost")
	}
}

func TestS3Constraints(t *testing.T) {
	mod := s3Module(t, `module s3-con {
  yang-version 1.1; namespace "urn:s3:con"; prefix c;
  container top {
    leaf enabled { type boolean; }
    leaf limit {
      type uint8;
      must "../c:enabled = 'true'" { error-message "enable first"; error-app-tag "needs-enable"; }
      when "../enabled";
    }
    list peer {
      key name;
      unique "addr port";
      unique "label";
      leaf name { type string; }
      leaf addr { type string; }
      leaf port { type uint16; }
      leaf label { type string; }
    }
  }
}`)
	limit := schemaNodeAt(t, mod, "/c:top/limit")
	musts := limit.Musts()
	if len(musts) != 1 || musts[0].Expression() != "../c:enabled = 'true'" {
		t.Fatalf("musts = %+v", musts)
	}
	if msg, ok := musts[0].ErrorMessage(); !ok || msg != "enable first" {
		t.Fatalf("must error-message = %q,%v", msg, ok)
	}
	if tag, ok := musts[0].ErrorAppTag(); !ok || tag != "needs-enable" {
		t.Fatalf("must error-app-tag = %q,%v", tag, ok)
	}
	// The prefix context for evaluating the expression is the source module.
	if musts[0].SourceModule().Name() != "s3-con" {
		t.Fatalf("must source module = %q", musts[0].SourceModule().Name())
	}
	whens := limit.Whens()
	if len(whens) != 1 || whens[0].Expression() != "../enabled" || whens[0].ContextAncestorDepth() != 0 || whens[0].SourceModule().Name() != "s3-con" {
		t.Fatalf("whens = %+v", whens)
	}
	peer := schemaNodeAt(t, mod, "/c:top/peer")
	var uniques [][]string
	for _, u := range peer.UniqueConstraints() {
		var leafs []string
		for _, l := range u.Leafs() {
			leafs = append(leafs, l.Name())
		}
		uniques = append(uniques, leafs)
	}
	if want := [][]string{{"addr", "port"}, {"label"}}; !reflect.DeepEqual(uniques, want) {
		t.Fatalf("uniques = %v, want %v", uniques, want)
	}
}

func TestS3SupportedReferences(t *testing.T) {
	const other = `module s3-ref-other {
  yang-version 1.1; namespace "urn:s3:ref-other"; prefix o;
  container remote { leaf id { type uint32; } }
}`
	const src = `module s3-ref {
  yang-version 1.1; namespace "urn:s3:ref"; prefix r;
  import s3-ref-other { prefix o; }
  list iface { key name; leaf name { type string; } leaf mtu { type uint16; } }
  container cfg {
    leaf if-name { type leafref { path "/r:iface/r:name"; } }
    leaf mtu { type leafref { path "/r:iface[r:name = current()/../if-name]/r:mtu"; } }
    leaf hop2 { type leafref { path "../mtu"; } }
    leaf remote-id { type leafref { path "/o:remote/o:id"; } }
    leaf u { type union { type leafref { path "../if-name"; } type uint8; } }
    choice c { case x { leaf in-choice { type string; } } }
    leaf to-choice { type leafref { path "../in-choice"; } }
    leaf to-choice-abs { type leafref { path "/r:cfg/r:in-choice"; } }
  }
}`
	ctx, err := buildPolicyContext(t, cambium.DeviationPolicy{}, other, src)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	mod, _ := ctx.Schema("s3-ref")
	targets := map[string]string{
		"/r:cfg/if-name":       "/s3-ref:iface/s3-ref:name",
		"/r:cfg/mtu":           "/s3-ref:iface/s3-ref:mtu",
		"/r:cfg/remote-id":     "/s3-ref-other:remote/s3-ref-other:id",
		"/r:cfg/to-choice":     "/s3-ref:cfg/s3-ref:c/s3-ref:x/s3-ref:in-choice",
		"/r:cfg/to-choice-abs": "/s3-ref:cfg/s3-ref:c/s3-ref:x/s3-ref:in-choice",
	}
	for path, want := range targets {
		res, err := cambium.ResolveLeafref(schemaNodeAt(t, mod, path))
		if err != nil {
			t.Errorf("ResolveLeafref(%s): %v", path, err)
			continue
		}
		if got := res.Target.QualifiedPath(); got != want {
			t.Errorf("ResolveLeafref(%s) = %s, want %s", path, got, want)
		}
	}
	chain, err := cambium.ResolveLeafrefChain(schemaNodeAt(t, mod, "/r:cfg/hop2"))
	if err != nil {
		t.Fatalf("chain: %v", err)
	}
	if len(chain.Trace) != 2 || chain.Target.Name() != "mtu" || chain.Realtype == nil || chain.Realtype.Base() != cambium.BaseTypeUint16 {
		t.Fatalf("chain = %d hops to %s (%v)", len(chain.Trace), chain.Target.QualifiedPath(), chain.Realtype)
	}
	if chain.Trace[0].SourceModule.Name() != "s3-ref" {
		t.Fatalf("trace source module = %q", chain.Trace[0].SourceModule.Name())
	}
	// A leafref union member resolves inside the union.
	u := leafType(t, mod, "/r:cfg/u").Resolved().(cambium.ResolvedUnion).Members()
	lr, ok := u[0].Resolved().(cambium.ResolvedLeafRef)
	if !ok {
		t.Fatalf("union member 0 = %#v", u[0].Resolved())
	}
	if target, ok := lr.Target(); !ok || target.Name() != "if-name" {
		t.Fatalf("union leafref target = %v,%v", target.Name(), ok)
	}
}

func TestS3NegativeReferences(t *testing.T) {
	cases := []struct {
		name, source, want string
		kind               cambium.DiagnosticKind
	}{
		{"missing target", `module neg1 { yang-version 1.1; namespace "urn:n1"; prefix n;
  leaf a { type leafref { path "/n:nope"; } } }`, `target not found`, cambium.DiagnosticUnresolvedPath},
		{"unknown prefix", `module neg2 { yang-version 1.1; namespace "urn:n2"; prefix n;
  leaf t { type string; } leaf a { type leafref { path "/zz:t"; } } }`, `unknown prefix "zz"`, cambium.DiagnosticInvalidIdentifier},
		{"invalid path syntax", `module neg3 { yang-version 1.1; namespace "urn:n3"; prefix n;
  leaf t { type string; } leaf a { type leafref { path "/n:t | /n:t"; } } }`, `invalid leafref path`, cambium.DiagnosticSemanticSchemaError},
	}
	for _, tc := range cases {
		_, err := buildPolicyContext(t, cambium.DeviationPolicy{}, tc.source)
		if err == nil {
			t.Errorf("%s: Build succeeded", tc.name)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error = %v, want %q", tc.name, err, tc.want)
		}
		diag := cambium.DiagnosticFromError(err)
		if diag.Kind != tc.kind {
			t.Errorf("%s: kind = %q, want %q (%v)", tc.name, diag.Kind, tc.kind, err)
		}
		if diag.Code != cambium.RuleCodeContext {
			t.Errorf("%s: code = %q", tc.name, diag.Code)
		}
	}

	// A leafref cycle is reported by chain resolution as a distinct cause.
	mod := s3Module(t, `module cyc { yang-version 1.1; namespace "urn:cyc"; prefix c;
  leaf a { type leafref { path "/c:b"; } }
  leaf b { type leafref { path "/c:a"; } } }`)
	_, err := cambium.ResolveLeafrefChain(schemaNodeAt(t, mod, "/c:a"))
	var lerr *cambium.LeafrefResolutionError
	if !errors.As(err, &lerr) || lerr.Reason != cambium.LeafrefFailureCycle {
		t.Fatalf("cycle error = %v", err)
	}
}

func TestS3OperationsAndOpaqueKinds(t *testing.T) {
	mod := s3Module(t, `module s3-ops {
  yang-version 1.1; namespace "urn:s3:ops"; prefix op;
  container top {
    anydata blob;
    anyxml doc;
    action reset { input { leaf z { type string; } leaf a { type string; } } output { leaf done { type boolean; } } }
  }
  rpc ping { input { leaf host { type string; } leaf count { type uint8; } } output { leaf rtt { type uint32; } } }
  notification alarm { leaf severity { type uint8; } leaf text { type string; } }
}`)
	if got, want := names(mod.Children()), "top,ping,alarm"; got != want {
		t.Fatalf("module children = %s, want %s", got, want)
	}
	// Data selection excludes operations; TopLevel lists data nodes only.
	if got, want := names(mod.TopLevel()), "top"; got != want {
		t.Fatalf("top-level data = %s, want %s", got, want)
	}
	if !schemaNodeAt(t, mod, "/op:top/blob").IsAnyData() || !schemaNodeAt(t, mod, "/op:top/doc").IsAnyXML() {
		t.Fatal("anydata/anyxml kinds wrong")
	}
	ping := schemaNodeAt(t, mod, "/op:ping")
	in, _ := ping.Input()
	out, _ := ping.Output()
	if got := names(in.Children()); got != "host,count" {
		t.Fatalf("ping input = %s", got)
	}
	if got := names(out.Children()); got != "rtt" {
		t.Fatalf("ping output = %s", got)
	}
	reset := schemaNodeAt(t, mod, "/op:top/reset")
	rin, _ := reset.Input()
	if !reset.IsAction() || names(rin.Children()) != "z,a" {
		t.Fatalf("action input = %s", names(rin.Children()))
	}
	if got := names(schemaNodeAt(t, mod, "/op:alarm").Children()); got != "severity,text" {
		t.Fatalf("notification = %s", got)
	}
	if got := names(schemaNodeAt(t, mod, "/op:top").DataChildren(true)); got != "blob,doc" {
		t.Fatalf("top data children = %s, want blob,doc", got)
	}
}

func itoa(v int64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	var b []byte
	for {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
		if v == 0 {
			break
		}
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

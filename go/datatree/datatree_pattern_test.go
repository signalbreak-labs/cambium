// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package datatree_test

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/datatree"
	"github.com/signalbreak-labs/cambium/go/yangpattern"
)

func patternModule(t *testing.T, body string) cambium.Module {
	t.Helper()
	return loadModSrc(t, `module pm {yang-version 1.1; namespace "urn:pm"; prefix p; `+body+`}`, "pm")
}
func patternTree(t *testing.T, mod cambium.Module, data string) *datatree.Tree {
	t.Helper()
	tree, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(data))
	if err != nil {
		t.Fatal(err)
	}
	return tree
}
func patternWitness() string {
	b := make([]byte, 1_000_000)
	x := uint64(0x9e3779b97f4a7c15)
	for i := range b {
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		b[i] = 'a' + byte(x&1)
	}
	return string(b)
}
func requirePatternCause(t *testing.T, err error, resource bool, context string) {
	t.Helper()
	if resource {
		cause, ok := errors.AsType[*yangpattern.ResourceError](err)
		if !ok || cause.Phase != "match" {
			t.Fatalf("want match ResourceError, got %T: %v", err, err)
		}
	} else if _, ok := errors.AsType[*yangpattern.ConfigurationError](err); !ok {
		t.Fatalf("want ConfigurationError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), context) {
		t.Fatalf("missing context %q: %v", context, err)
	}
}
func TestDatatreePatternLanguages(t *testing.T) {
	for _, tc := range []struct{ name, pattern, good, bad string }{
		{"large", "a{1001}", strings.Repeat("a", 1001), "a"},
		{"leading", "a{0001}", "a", "a{0001}"},
		{"range", "a{01,01024}", strings.Repeat("a", 1024), strings.Repeat("a", 1025)},
		{"nullable", "(a?){01001}", "", strings.Repeat("a", 1002)},
		{"unicode", `\p{L}{1001}`, strings.Repeat("λ", 1001), "1"},
		{"subtraction", `[a-z-[aeiou]]{1001}`, strings.Repeat("b", 1001), strings.Repeat("a", 1001)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, invert := range []bool{false, true} {
				modifier := ";"
				if invert {
					modifier = " {modifier invert-match;}"
				}
				mod := patternModule(t, "leaf value {type string {pattern '"+tc.pattern+"'"+modifier+"}}")
				for _, sample := range []struct {
					value string
					valid bool
				}{{tc.good, !invert}, {tc.bad, invert}} {
					tree := patternTree(t, mod, `{"pm:value":`+strconv.Quote(sample.value)+`}`)
					err := tree.Validate()
					if (err == nil) != sample.valid {
						t.Fatalf("invert=%v valid=%v got %v", invert, sample.valid, err)
					}
					if err != nil {
						if _, ok := errors.AsType[*datatree.ValidationError](err); !ok {
							t.Fatal(err)
						}
					}
				}
			}
		})
	}
}
func TestDatatreePatternValidateOperational(t *testing.T) {
	for _, resource := range []bool{false, true} {
		pattern, value := "[", "a"
		if resource {
			pattern, value = "[ab]*a[ab]{1001}", patternWitness()
		}
		expr := "re-match(/p:input, '" + pattern + "')"
		for _, tc := range []struct{ name, body, data, context string }{
			{"must", `leaf value {type string;must "` + expr + `";}`, `,"pm:value":"x"`, "/value"},
			{"presentWhen", `leaf value {type string;when "` + expr + `";}`, `,"pm:value":"x"`, "/value"},
			{"missingLeaf", `leaf value {type string;mandatory true;when "` + expr + `";}`, "", "/value"},
			{"missingContainer", `container value {when "` + expr + `";leaf required {type string;mandatory true;}}`, "", "/value"},
			{"choice", `choice value {mandatory true;when "` + expr + `";leaf a {type string;}}`, "", "value"},
			{"unique", `list value {key id;unique d;leaf id {type string;}leaf d {type string;default ok;when "` + expr + `";}}`, `,"pm:value":[{"id":"a"},{"id":"b"}]`, "/value"},
		} {
			t.Run(fmt.Sprintf("%s/resource=%v", tc.name, resource), func(t *testing.T) {
				mod := patternModule(t, `leaf input {type string;} `+tc.body)
				tree := patternTree(t, mod, `{"pm:input":`+strconv.Quote(value)+tc.data+`}`)
				requirePatternCause(t, tree.Validate(), resource, tc.context)
			})
		}
	}
}
func TestDatatreePatternParseOperational(t *testing.T) {
	mod := patternModule(t, `typedef u {type union {type string {pattern '[ab]*a[ab]{1001}';}type string;}}leaf value {type u;}`)
	data := []byte(`{"pm:value":` + strconv.Quote(patternWitness()) + `}`)
	for _, multi := range []bool{false, true} {
		t.Run(fmt.Sprint(multi), func(t *testing.T) {
			var tree *datatree.Tree
			var err error
			if multi {
				tree, err = datatree.ParseModules([]cambium.Module{mod}, datatree.FormatJSONIETF, data)
			} else {
				tree, err = datatree.Parse(mod, datatree.FormatJSONIETF, data)
			}
			requirePatternCause(t, err, true, "value")
			if tree != nil {
				t.Fatal("partial tree returned after canonicalization failure")
			}
		})
	}
}
func TestDatatreePatternDefaultsRollback(t *testing.T) {
	mod := patternModule(t, `leaf early {type string;default changed;}container nested {leaf d {type string;default changed;}}list item {key id;leaf id {type string;}leaf d {type string;default changed;}}leaf late {type string;default x;when "re-match('a', '[')";}`)
	tree := patternTree(t, mod, `{"pm:nested":{},"pm:item":[{"id":"b"},{"id":"a"}]}`)
	before, err := tree.Serialize(datatree.FormatJSONIETF)
	if err != nil {
		t.Fatal(err)
	}
	requirePatternCause(t, tree.ApplyDefaults(), false, "/late")
	after, err := tree.Serialize(datatree.FormatJSONIETF)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("failed default guard changed tree: before=%s after=%s", before, after)
	}
}

func TestDatatreePatternDefaultsOperational(t *testing.T) {
	for _, count := range []int{1000, 1001} {
		for _, resource := range []bool{false, true} {
			pattern, value := "[", "a"
			if resource {
				pattern, value = fmt.Sprintf("[ab]*a[ab]{%d}", count), patternWitness()
			}
			guard := `when "re-match(/p:input, '` + pattern + `')";`
			for _, tc := range []struct{ name, body string }{
				{"leaf", `leaf late {type string;default x;` + guard + `}`},
				{"leafList", `leaf-list late {type string;default z;default a;` + guard + `}`},
				{"inherited", `grouping g {leaf late {type string;default x;}} uses g {` + guard + `}`},
				{"choice", `container guarded {choice choice {default selected;` + guard + `case selected {leaf late {type string;default x;}}}}`},
				{"case", `container guarded {choice choice {default selected;case selected {` + guard + `leaf late {type string;default x;}}}}`},
			} {
				t.Run(fmt.Sprintf("%s/%d/resource=%v", tc.name, count, resource), func(t *testing.T) {
					mod := patternModule(t, `leaf input {type string;} leaf early {type string;default changed;} container nested {leaf d {type string;default changed;}} list item {key id;ordered-by user;leaf d {type string;default changed;} leaf id {type string;}} `+tc.body)
					extra := ""
					if tc.name == "choice" || tc.name == "case" {
						extra = `,"pm:guarded":{}`
					}
					tree := patternTree(t, mod, `{"pm:input":`+strconv.Quote(value)+`,"pm:nested":{},"pm:item":[{"id":"b"},{"id":"a"}]`+extra+`}`)
					before, err := tree.Serialize(datatree.FormatJSONIETF)
					if err != nil {
						t.Fatal(err)
					}
					requirePatternCause(t, tree.ApplyDefaults(), resource, "/late")
					after, err := tree.Serialize(datatree.FormatJSONIETF)
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(before, after) {
						t.Fatal("failed defaults mutated original tree")
					}
				})
			}
		}
	}
}

func TestDatatreePatternPredicateControls(t *testing.T) {
	for _, tc := range []struct {
		name, expr string
		active     bool
	}{
		{"trueShortCircuit", `true() or re-match('a', '[')`, true},
		{"falseShortCircuit", `false() and re-match('a', '[')`, false},
		{"false", `false()`, false},
		{"function", `deref(.)`, false},
		{"axis", `child::missing`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mod := patternModule(t, `leaf present {type string;must "`+tc.expr+`";} leaf value {type string;default x;when "`+tc.expr+`";}`)
			tree := patternTree(t, mod, `{"pm:present":"x"}`)
			err := tree.Validate()
			if tc.name == "false" || tc.name == "falseShortCircuit" {
				if _, ok := errors.AsType[*datatree.ValidationError](err); !ok {
					t.Fatalf("want ordinary violation, got %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if err := tree.ApplyDefaults(); err != nil {
				t.Fatal(err)
			}
			_, present := tree.Find("value")
			if present != tc.active {
				t.Fatalf("default present=%v", present)
			}
			first, err := tree.Serialize(datatree.FormatJSONIETF)
			if err != nil {
				t.Fatal(err)
			}
			if err := tree.ApplyDefaults(); err != nil {
				t.Fatal(err)
			}
			second, err := tree.Serialize(datatree.FormatJSONIETF)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(first, second) {
				t.Fatal("defaults not idempotent")
			}
		})
	}
}

// Canonicalization removes an own-module identity qualifier. That changes an
// easily disproven pattern candidate into the pinned resource witness, making
// these downstream boundaries reachable without private mutation or hooks.
func TestDatatreePatternCanonicalIdentityBoundaries(t *testing.T) {
	value := patternWitness()
	for _, count := range []int{1000, 1001} {
		union := fmt.Sprintf(`type union {type string {pattern '[ab]*a[ab]{%d}';} type identityref {base root;}}`, count)
		identity := `identity root;identity ` + value + ` {base root;} `
		for _, multi := range []bool{false, true} {
			parse := func(t *testing.T, mod cambium.Module, format datatree.Format, input string) (*datatree.Tree, error) {
				t.Helper()
				if multi {
					return datatree.ParseModules([]cambium.Module{mod}, format, []byte(input))
				}
				return datatree.Parse(mod, format, []byte(input))
			}
			t.Run(fmt.Sprintf("serialize/%d/multi=%v", count, multi), func(t *testing.T) {
				mod := patternModule(t, identity+`leaf value {`+union+`}`)
				tree, err := parse(t, mod, datatree.FormatJSONIETF, `{"pm:value":"pm:`+value+`"}`)
				if err != nil {
					t.Fatal(err)
				}
				out, err := tree.Serialize(datatree.FormatXML)
				requirePatternCause(t, err, true, "value")
				if out != nil {
					t.Fatal("partial XML returned")
				}
				requirePatternCause(t, tree.Validate(), true, "value")
			})
			for _, tc := range []struct{ name, body, input string }{
				{"leafListSort", `leaf-list value {` + union + `}`, `{"pm:value":["pm:` + value + `","a"]}`},
				{"listSort", `list item {key id;leaf other {type string;}leaf id {` + union + `}}`, `{"pm:item":[{"other":"first","id":"pm:` + value + `"},{"other":"second","id":"a"}]}`},
			} {
				t.Run(fmt.Sprintf("%s/%d/multi=%v", tc.name, count, multi), func(t *testing.T) {
					mod := patternModule(t, identity+tc.body)
					tree, err := parse(t, mod, datatree.FormatJSONIETF, tc.input)
					requirePatternCause(t, err, true, "sort key")
					if tree != nil {
						t.Fatal("partial tree returned")
					}
				})
			}
			t.Run(fmt.Sprintf("xmlSelection/%d/multi=%v", count, multi), func(t *testing.T) {
				mod := patternModule(t, identity+`leaf value {`+union+`}`)
				tree, err := parse(t, mod, datatree.FormatXML, `<value xmlns="urn:pm" xmlns:p="urn:pm">p:`+value+`</value>`)
				requirePatternCause(t, err, true, "value")
				if tree != nil {
					t.Fatal("partial tree returned")
				}
			})
		}
		for _, kind := range []string{"leaf", "leaf-list"} {
			t.Run(fmt.Sprintf("default/%s/%d", kind, count), func(t *testing.T) {
				mod := patternModule(t, identity+`leaf early {type string;default changed;} `+kind+` value {`+union+`default 'p:`+value+`';}`)
				tree := patternTree(t, mod, `{}`)
				before, err := tree.Serialize(datatree.FormatJSONIETF)
				if err != nil {
					t.Fatal(err)
				}
				requirePatternCause(t, tree.ApplyDefaults(), true, "value")
				after, err := tree.Serialize(datatree.FormatJSONIETF)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(before, after) {
					t.Fatal("default canonicalization failure mutated tree")
				}
			})
		}
	}
}

func TestDatatreePatternValueOperational(t *testing.T) {
	for _, count := range []int{1000, 1001} {
		for _, inverted := range []bool{false, true} {
			modifier := ";"
			if inverted {
				modifier = " {modifier invert-match;}"
			}
			pattern := fmt.Sprintf("[ab]*a[ab]{%d}", count)
			mod := patternModule(t, `leaf value {type string {pattern '`+pattern+`'`+modifier+`}} leaf ref {type leafref {path /p:value;require-instance false;}}`)
			for _, leaf := range []string{"value", "ref"} {
				t.Run(fmt.Sprintf("%s/%d/inverted=%v", leaf, count, inverted), func(t *testing.T) {
					tree := patternTree(t, mod, `{"pm:`+leaf+`":`+strconv.Quote(patternWitness())+`}`)
					requirePatternCause(t, tree.Validate(), true, "/"+leaf)
				})
			}
		}
	}
}

func TestDatatreePatternXPathLanguages(t *testing.T) {
	for _, tc := range []struct{ pattern, good, bad string }{
		{"a{1001}", strings.Repeat("a", 1001), "a"},
		{"a{0001}", "a", "a{0001}"},
		{"(a?){01001}", "", strings.Repeat("a", 1002)},
		{`\p{L}{1001}`, strings.Repeat("λ", 1001), "1"},
		{`[a-z-[aeiou]]{1001}`, strings.Repeat("b", 1001), strings.Repeat("a", 1001)},
	} {
		t.Run(tc.pattern, func(t *testing.T) {
			// A single-quoted YANG expression preserves the XSD backslash literally.
			mod := patternModule(t, `leaf value {type string;must 're-match(., "`+tc.pattern+`")';}`)
			for _, sample := range []struct {
				value string
				valid bool
			}{{tc.good, true}, {tc.bad, false}} {
				tree := patternTree(t, mod, `{"pm:value":`+strconv.Quote(sample.value)+`}`)
				err := tree.Validate()
				if (err == nil) != sample.valid {
					t.Fatalf("valid=%v, got %v", sample.valid, err)
				}
				if err != nil {
					if _, ok := errors.AsType[*datatree.ValidationError](err); !ok {
						t.Fatal(err)
					}
				}
			}
		})
	}
}

func TestDatatreePatternPrefixCompatibility(t *testing.T) {
	// Schema prefixes are eagerly checked by Build. This public entry point
	// prevents constructing an unknown-prefix must/when fixture for Validate.
	b, err := cambium.NewContextBuilder(cambium.ContextFlags{DisableSearchdirCwd: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.LoadModuleStr(`module pm {namespace "urn:pm";prefix p;leaf value {type string;must "unknown:missing";}}`); err != nil {
		t.Fatal(err)
	}
	ctx, err := b.Build()
	if ctx != nil {
		ctx.Close()
	}
	if _, ok := errors.AsType[*cambium.Error](err); !ok || !strings.Contains(err.Error(), `unknown prefix "unknown"`) {
		t.Fatalf("want schema prefix rejection, got %v", err)
	}
	// A data-provided instance-identifier can still contain an unresolved prefix;
	// the historical unsupported-expression policy remains unchanged.
	mod := patternModule(t, `leaf value {type instance-identifier;}`)
	tree := patternTree(t, mod, `{"pm:value":"/unknown:missing"}`)
	if err := tree.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestDatatreePatternUnionOrderingAndFallback(t *testing.T) {
	// Both members accept some values. Ordered union selection must use the
	// normalized XSD program before assigning canonical representation/order.
	mod := patternModule(t, `typedef u {type union {type string {pattern '[a-z]{0001}';}type int8;}} leaf-list sys {type u;} leaf-list user {type u;ordered-by user;} list item {key id;leaf other {type string;}leaf id {type u;}}`)
	tree := patternTree(t, mod, `{"pm:sys":["z",2,"a",1],"pm:user":["z",2,"a",1],"pm:item":[{"other":"last","id":"z"},{"other":"first","id":1}]}`)
	if err := tree.Validate(); err != nil {
		t.Fatal(err)
	}
	out, err := tree.Serialize(datatree.FormatJSONIETF)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"pm:sys":[1,2,"a","z"],"pm:user":["z",2,"a",1],"pm:item":[{"id":1,"other":"first"},{"id":"z","other":"last"}]}`
	if string(out) != want {
		t.Fatalf("canonical order: got %s want %s", out, want)
	}
	xml, err := tree.Serialize(datatree.FormatXML)
	if err != nil {
		t.Fatal(err)
	}
	again, err := datatree.Parse(mod, datatree.FormatXML, xml)
	if err != nil {
		t.Fatal(err)
	}
	roundTrip, err := again.Serialize(datatree.FormatJSONIETF)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, roundTrip) {
		t.Fatal("union XML representation changed round-trip values/order")
	}
	invalid := patternTree(t, mod, `{"pm:sys":["invalid"]}`)
	if _, ok := errors.AsType[*datatree.ValidationError](invalid.Validate()); !ok {
		t.Fatal("ordinary union mismatch did not return ValidationError")
	}
}

func TestDatatreePatternUniqueCanonicalValue(t *testing.T) {
	value := patternWitness()
	mod := patternModule(t, `identity root;identity `+value+` {base root;} list item {key id;unique value;ordered-by user;leaf id {type string;} leaf value {type union {type string {pattern '[ab]*a[ab]{1001}';}type identityref {base root;}}}}`)
	tree := patternTree(t, mod, `{"pm:item":[{"id":"first","value":"pm:`+value+`"},{"id":"second","value":"pm:`+value+`"}]}`)
	requirePatternCause(t, tree.Validate(), true, "comparison key")
}

func TestDatatreePatternJoinedViolations(t *testing.T) {
	mod := patternModule(t, `leaf required {type string;mandatory true;} leaf value {type string;must "re-match('a','[')";}`)
	tree := patternTree(t, mod, `{"pm:value":"x"}`)
	err := tree.Validate()
	requirePatternCause(t, err, false, "/value")
	ordinary, ok := errors.AsType[*datatree.ValidationError](err)
	if !ok || len(ordinary.Violations) != 1 || !strings.Contains(ordinary.Violations[0], "/required") {
		t.Fatalf("earlier ordinary violation lost: %v", err)
	}
}

// Only the unique walker visits the default case of this absent optional
// choice. Ordinary entry validation must not mask a swallowed activation error
// by evaluating the same missing leaf's guard a second time.
func TestDatatreePatternUniqueImplicitDefaultGuard(t *testing.T) {
	for _, tc := range []struct {
		name, pattern, value string
		resource             bool
	}{
		{"syntax", "[", "a", false},
		{"resource1000", "[ab]*a[ab]{1000}", patternWitness(), true},
		{"resource1001", "[ab]*a[ab]{1001}", patternWitness(), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mod := patternModule(t, `
    leaf input { type string; }
    list item {
     key id;
     unique "optional/selected/value";
     leaf id { type string; }
     choice optional {
      default selected;
      case selected {
       leaf value {
        type string;
        default ok;
        when "re-match(/p:input, '`+tc.pattern+`')";
       }
      }
     }
    }`)
			tree := patternTree(t, mod, `{"pm:input":`+strconv.Quote(tc.value)+`,"pm:item":[{"id":"first"},{"id":"second"}]}`)
			requirePatternCause(t, tree.Validate(), tc.resource, "/item/value")
		})
	}
}

// Neither list entry supplies value. Schema Build accepts the qualified
// identity default, but unique's implicit-default token conversion strips its
// qualifier and must surface the resulting matcher resource failure.
func TestDatatreePatternUniqueImplicitDefaultToken(t *testing.T) {
	value := patternWitness()
	for _, count := range []int{1000, 1001} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			mod := patternModule(t, `
    identity root;
    identity `+value+` { base root; }
    list item {
     key id;
     unique value;
     leaf id { type string; }
     leaf value {
      type union {
       type string { pattern '[ab]*a[ab]{`+strconv.Itoa(count)+`}'; }
       type identityref { base root; }
      }
      default 'p:`+value+`';
     }
    }`)
			tree := patternTree(t, mod, `{"pm:item":[{"id":"first"},{"id":"second"}]}`)
			requirePatternCause(t, tree.Validate(), true, "/item/value")
		})
	}
}

// A vendor-compatible schema keeps patterns Cambium cannot compile unenforced;
// the data tree leaves them to the device and still enforces the others.
func TestDatatreeLeavesUnenforcedPatternsToTheDevice(t *testing.T) {
	b, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.SetValidationMode(cambium.ValidationVendorCompatible); err != nil {
		t.Fatal(err)
	}
	if err := b.LoadModuleStr(`module pm {yang-version 1.1; namespace "urn:pm"; prefix p;
	  leaf name {type string {pattern 'a{,5}';}}
	  leaf plain {type string {pattern '[a-z]+';}}}`); err != nil {
		t.Fatal(err)
	}
	ctx, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ctx.Close() })
	mod, err := ctx.Schema("pm")
	if err != nil {
		t.Fatal(err)
	}
	if err := patternTree(t, mod, `{"pm:name":"zzz","pm:plain":"abc"}`).Validate(); err != nil {
		t.Fatalf("unenforced pattern rejected a value: %v", err)
	}
	if _, ok := errors.AsType[*datatree.ValidationError](patternTree(t, mod, `{"pm:plain":"ABC"}`).Validate()); !ok {
		t.Fatal("enforced pattern accepted ABC")
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package datatree_test

import (
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/datatree"
)

// Validation verdicts checked against libyang v5.4.9 (lyd_validate_all with
// implicit defaults): every "want" below is the verdict libyang gives for the
// same schema and data.

type verdictCase struct {
	name string
	in   string
	want []string // violation substrings; nil means the data must be valid
	not  []string // substrings that must NOT appear in the violations
}

func checkVerdicts(t *testing.T, mod cambium.Module, cases []verdictCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateOne(t, mod, tc.in)
			if len(tc.want) == 0 {
				if err != nil {
					t.Fatalf("expected valid data, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected violations %q, got nil", tc.want)
			}
			for _, w := range tc.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("violation %q not found in: %v", w, err)
				}
			}
			for _, n := range tc.not {
				if strings.Contains(err.Error(), n) {
					t.Errorf("unexpected violation %q in: %v", n, err)
				}
			}
		})
	}
}

const choiceVerdictSchema = `module vc {
    yang-version 1.1; namespace "urn:vc"; prefix vc;
    container c {
        choice ch {
            case x { leaf x1 { type string; mandatory true; } leaf x2 { type string; default "d"; } }
            case y { leaf y1 { type string; } list yl { key id; min-elements 1; leaf id { type string; } } }
        }
        leaf after { type string; }
    }
    container c2 { presence "p"; choice mc { mandatory true; leaf p { type string; } leaf q { type string; } } }
    container nest {
        choice outer {
            case o1 {
                choice inner { mandatory true; case i1 { leaf a { type string; } } case i2 { leaf b { type string; } } }
                leaf o1leaf { type string; }
            }
            case o2 { leaf c { type string; } }
        }
    }
    container w {
        leaf flag { type boolean; }
        choice wc { mandatory true; when "flag = 'true'"; leaf wa { type string; } leaf wb { type string; } }
    }
    list e {
        key k;
        leaf k { type string; }
        choice ec { mandatory true; leaf ea { type string; } leaf eb { type string; } }
    }
    choice top { leaf-list tl { type string; max-elements 1; } leaf tx { type string; } }
    augment "/vc:c/vc:ch" { leaf-list z { type string; max-elements 1; } }
    grouping g { choice gc { mandatory true; leaf ga { type string; } leaf gb { type string; } } }
    container u { leaf flag { type boolean; } uses g { when "flag = 'true'"; } }
    container au { leaf flag { type boolean; } }
    augment "/vc:au" {
        when "vc:flag = 'true'";
        choice ac { mandatory true; leaf aa { type string; } leaf ab { type string; } }
    }
}`

// RFC 7950 §7.9: only the case whose data exists is validated, at most one
// case of a choice may have data, and a mandatory choice needs one case.
func TestValidateChoiceCaseSemantics(t *testing.T) {
	checkVerdicts(t, loadModSrc(t, choiceVerdictSchema, "vc"), []verdictCase{
		{name: "unselected case mandatory leaf", in: `{"vc:c":{"y1":"v","yl":[{"id":"1"}]}}`},
		{name: "unselected case min-elements", in: `{"vc:c":{"x1":"v"}}`},
		{name: "no case selected", in: `{"vc:c":{"after":"v"}}`},
		{name: "selected case mandatory leaf", in: `{"vc:c":{"x2":"v"}}`,
			want: []string{"missing mandatory node /c/x1"}, not: []string{"yl"}},
		{name: "selected case min-elements", in: `{"vc:c":{"y1":"v"}}`,
			want: []string{"/c/yl has 0 entries"}, not: []string{"x1"}},
		{name: "two cases", in: `{"vc:c":{"x1":"v","y1":"v"}}`,
			want: []string{`/c: data for both cases "x" and "y" of choice "ch" exist`}, not: []string{"yl"}},
		{name: "mandatory choice missing", in: `{"vc:c2":{}}`,
			want: []string{`missing mandatory choice /c2/mc`}},
		{name: "mandatory choice present", in: `{"vc:c2":{"q":"1"}}`},
		{name: "shorthand cases both present", in: `{"vc:c2":{"p":"1","q":"1"}}`,
			want: []string{`/c2: data for both cases "p" and "q" of choice "mc" exist`}},
		{name: "nested other outer case", in: `{"vc:nest":{"c":"1"}}`},
		{name: "nested inner mandatory in selected case", in: `{"vc:nest":{"o1leaf":"1"}}`,
			want: []string{"missing mandatory choice /nest/inner"}},
		{name: "nested inner case selected", in: `{"vc:nest":{"a":"1"}}`},
		{name: "nested inner two cases", in: `{"vc:nest":{"a":"1","b":"2"}}`,
			want: []string{`/nest: data for both cases "i1" and "i2" of choice "inner" exist`}},
		{name: "nested outer two cases", in: `{"vc:nest":{"a":"1","c":"2"}}`,
			want: []string{`/nest: data for both cases "o1" and "o2" of choice "outer" exist`}},
		{name: "nested nothing selected", in: `{"vc:nest":{}}`},
		{name: "mandatory choice when false", in: `{"vc:w":{"flag":false}}`},
		{name: "mandatory choice when true", in: `{"vc:w":{"flag":true}}`,
			want: []string{"missing mandatory choice /w/wc"}},
		{name: "mandatory choice in list entry", in: `{"vc:e":[{"k":"1","ea":"x"},{"k":"2"}]}`,
			want: []string{"missing mandatory choice /e[1]/ec"}, not: []string{"/e[0]"}},
		{name: "top-level shorthand case validated", in: `{"vc:tl":["a","b"]}`,
			want: []string{"/tl has 2 entries, more than max-elements 1"}},
		{name: "top-level shorthand cases both present", in: `{"vc:tl":["a"],"vc:tx":"x"}`,
			want: []string{`/: data for both cases "tl" and "tx" of choice "top" exist`}},
		{name: "augmented shorthand case validated", in: `{"vc:c":{"z":["a","b"]}}`,
			want: []string{"/c/z has 2 entries, more than max-elements 1"}},
		{name: "augmented shorthand case and another", in: `{"vc:c":{"z":["a"],"y1":"b"}}`,
			want: []string{`/c: data for both cases "y" and "z" of choice "ch" exist`}},
		{name: "mandatory choice under uses when false", in: `{"vc:u":{"flag":false}}`},
		{name: "mandatory choice under uses when true", in: `{"vc:u":{"flag":true}}`,
			want: []string{"missing mandatory choice /u/gc"}},
		{name: "mandatory choice augmented when false", in: `{"vc:au":{"flag":false}}`},
		{name: "mandatory choice augmented when true", in: `{"vc:au":{"flag":true}}`,
			want: []string{"missing mandatory choice /au/ac"}},
	})
}

// RFC 7950 §7.9.3 / §7.6.1: defaults are in use only in the case whose data
// exists or, when no case has data, in the choice's default case (recursively
// through nested choices); leaf-list defaults are applied like leaf defaults.
func TestApplyDefaultsChoiceCases(t *testing.T) {
	mod := loadModSrc(t, `module dc {
        yang-version 1.1; namespace "urn:dc"; prefix dc;
        container c {
            choice ch {
                case x { leaf x1 { type string; } leaf x2 { type string; default "d"; } }
                case y { leaf y1 { type string; } leaf y2 { type string; default "e"; } }
            }
        }
        container dfl {
            choice d {
                default dc;
                case dc {
                    leaf dl { type string; default "dv"; }
                    leaf-list dll { type string; default "z"; default "a"; }
                    choice dn {
                        default dn2;
                        case dn1 { leaf dn1l { type string; default "n1"; } }
                        case dn2 { leaf dn2l { type string; default "n2"; } }
                    }
                }
                case oc { leaf ol { type string; default "ov"; } leaf oset { type string; } }
            }
        }
        container ll {
            leaf-list sys { type int8; default 3; default 1; }
            leaf-list usr { type int8; ordered-by user; default 3; default 1; }
            leaf-list set { type string; default "x"; }
        }
        choice payload {
            leaf-list tags { type string; }
            leaf other { type string; default "o"; }
        }
        augment "/dc:c/dc:ch" { leaf z { type string; default "zz"; } }
        container e {
            choice ec {
                default d1;
                case d1 { leaf dl1 { type string; default "x"; } }
                case d2 { container d2c { leaf dl2 { type string; } } }
            }
        }
    }`, "dc")
	cases := []struct{ name, in, want string }{
		{"selected case only", `{"dc:c":{"y1":"v"}}`, `{"dc:c":{"y1":"v","y2":"e"}}`},
		{"no case and no default case", `{"dc:c":{}}`, `{"dc:c":{}}`},
		{"default case when none selected", `{"dc:dfl":{}}`, `{"dc:dfl":{"dl":"dv","dll":["a","z"],"dn2l":"n2"}}`},
		{"other case selected", `{"dc:dfl":{"oset":"s"}}`, `{"dc:dfl":{"ol":"ov","oset":"s"}}`},
		{"nested case selects outer case", `{"dc:dfl":{"dn1l":"s"}}`, `{"dc:dfl":{"dl":"dv","dll":["a","z"],"dn1l":"s"}}`},
		{"leaf-list defaults", `{"dc:ll":{"set":["y"]}}`, `{"dc:ll":{"sys":[1,3],"usr":[3,1],"set":["y"]}}`},
		{"top-level shorthand case kept", `{"dc:tags":["b","a"]}`, `{"dc:tags":["a","b"]}`},
		{"augmented shorthand case kept", `{"dc:c":{"z":"v"}}`, `{"dc:c":{"z":"v"}}`},
		{"empty container does not select its case", `{"dc:e":{"d2c":{}}}`, `{"dc:e":{"dl1":"x"}}`},
		{"container selects its case", `{"dc:e":{"d2c":{"dl2":"a"}}}`, `{"dc:e":{"d2c":{"dl2":"a"}}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tree, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(tc.in))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			tree.ApplyDefaults()
			got, err := tree.Serialize(datatree.FormatJSONIETF)
			if err != nil {
				t.Fatalf("Serialize: %v", err)
			}
			if string(got) != tc.want {
				t.Fatalf("ApplyDefaults:\n got: %s\nwant: %s", got, tc.want)
			}
			if err := tree.Validate(); err != nil {
				t.Fatalf("defaults-applied tree should validate: %v", err)
			}
		})
	}
}

// RFC 7950 §3 and §7.5.1: a non-presence container exists conceptually
// whenever its parent does, so constraints below an absent one still apply
// (libyang instantiates it). An absent presence container, a false when, and
// an unselected case switch that off.
func TestValidateMandatoryUnderAbsentNonPresenceContainer(t *testing.T) {
	const src = `module vn {
        yang-version 1.1; namespace "urn:vn"; prefix vn;
        leaf top { type string; }
        container np { leaf must1 { type string; mandatory true; } }
        container pc { presence "p"; container inner { leaf m { type string; mandatory true; } } }
        container npw { when "../top = 'go'"; leaf m { type string; mandatory true; } }
        container nplist { list l { key k; min-elements 1; leaf k { type string; } } }
        container npch { choice c { mandatory true; leaf a { type string; } leaf b { type string; } } }
        container deep { container deeper { leaf m { type string; mandatory true; } } }
        container st { config false; leaf m { type string; mandatory true; } }
        list ent { key k; leaf k { type string; } container sub { leaf m { type string; mandatory true; } } }
        container cs { choice c { case a { container ca { leaf m { type string; mandatory true; } } } case b { leaf lb { type string; } } } }
    }`
	const ok = `"vn:np":{"must1":"a"},"vn:nplist":{"l":[{"k":"1"}]},"vn:npch":{"a":"1"},"vn:deep":{"deeper":{"m":"1"}},"vn:st":{"m":"1"}`
	checkVerdicts(t, loadModSrc(t, src, "vn"), []verdictCase{
		{name: "absent containers", in: `{"vn:top":"x"}`, want: []string{
			"missing mandatory node /np/must1",
			"/nplist/l has 0 entries, fewer than min-elements 1",
			"missing mandatory choice /npch/c",
			"missing mandatory node /deep/deeper/m",
			"missing mandatory node /st/m",
		}, not: []string{"/pc/", "/npw/", "/ent", "/cs/"}},
		{name: "all satisfied", in: `{"vn:top":"x",` + ok + `}`},
		{name: "presence container present", in: `{"vn:top":"x",` + ok + `,"vn:pc":{}}`,
			want: []string{"missing mandatory node /pc/inner/m"}},
		{name: "when true", in: `{"vn:top":"go",` + ok + `}`,
			want: []string{"missing mandatory node /npw/m"}},
		{name: "list entry", in: `{"vn:top":"x",` + ok + `,"vn:ent":[{"k":"1"}]}`,
			want: []string{"missing mandatory node /ent[0]/sub/m"}},
		{name: "unselected case", in: `{"vn:top":"x",` + ok + `,"vn:cs":{"lb":"1"}}`},
		{name: "empty container does not select its case", in: `{"vn:top":"x",` + ok + `,"vn:cs":{"ca":{}}}`},
		{name: "empty container of another case", in: `{"vn:top":"x",` + ok + `,"vn:cs":{"ca":{},"lb":"1"}}`,
			want: []string{`/cs: data for both cases "a" and "b" of choice "c" exist`}, not: []string{"/cs/ca/m"}},
	})
}

const uniqueVerdictSchema = `module vu {
    yang-version 1.1; namespace "urn:vu"; prefix vu;
    identity base; identity one { base base; }
    list u { key k; unique "v"; leaf k { type string; } leaf v { type string; } }
    list u2 { key k; unique "v w"; leaf k { type string; } leaf v { type string; } leaf w { type int8; } }
    list u3 { key k; unique "c/v"; leaf k { type string; } container c { leaf v { type int64; } } }
    list u4 { key k; unique "v"; leaf k { type string; } leaf v { type string; default "dd"; } }
    list u5 { key k; unique "id"; leaf k { type string; } leaf id { type identityref { base base; } } }
    list u6 { key k; unique "d"; leaf k { type string; } leaf d { type decimal64 { fraction-digits 2; } } }
    list u7 { key k; unique "c/v"; leaf k { type string; } container c { leaf v { type string; default "cd"; } } }
    list u8 {
        key k; unique "ch/cd/cv";
        leaf k { type string; }
        choice ch { default cd; case cd { leaf cv { type string; default "x"; } } case ce { leaf ce { type string; } } }
    }
    list st { config false; key k; unique "v"; leaf k { type string; } leaf v { type string; } }
}`

// RFC 7950 §7.8.3: the combined values of the unique leaves, including leaves
// with default values, must differ between entries in which all of them exist
// or have default values. Values compare canonically.
func TestValidateUnique(t *testing.T) {
	checkVerdicts(t, loadModSrc(t, uniqueVerdictSchema, "vu"), []verdictCase{
		{name: "violation", in: `{"vu:u":[{"k":"1","v":"s"},{"k":"2","v":"s"}]}`,
			want: []string{`/u[1]: unique constraint "v" is violated: same values as /u[0]`}},
		{name: "distinct", in: `{"vu:u":[{"k":"1","v":"s"},{"k":"2","v":"t"}]}`},
		{name: "missing leaf ignored", in: `{"vu:u":[{"k":"1"},{"k":"2"}]}`},
		{name: "three entries", in: `{"vu:u":[{"k":"1","v":"s"},{"k":"2","v":"t"},{"k":"3","v":"s"}]}`,
			want: []string{`/u[2]: unique constraint "v" is violated: same values as /u[0]`}, not: []string{"/u[1]"}},
		{name: "multi-leaf partial equal", in: `{"vu:u2":[{"k":"1","v":"s","w":1},{"k":"2","v":"s","w":2}]}`},
		{name: "multi-leaf equal", in: `{"vu:u2":[{"k":"1","v":"s","w":1},{"k":"2","v":"s","w":1}]}`,
			want: []string{`unique constraint "v w" is violated`}},
		{name: "multi-leaf one missing", in: `{"vu:u2":[{"k":"1","v":"s"},{"k":"2","v":"s"}]}`},
		{name: "descendant path canonical int64", in: `{"vu:u3":[{"k":"1","c":{"v":"01"}},{"k":"2","c":{"v":"+1"}}]}`,
			want: []string{`/u3[1]: unique constraint "c/v" is violated`}},
		{name: "defaults both absent", in: `{"vu:u4":[{"k":"1"},{"k":"2"}]}`,
			want: []string{`unique constraint "v" is violated`}},
		{name: "default and explicit", in: `{"vu:u4":[{"k":"1","v":"dd"},{"k":"2"}]}`,
			want: []string{`unique constraint "v" is violated`}},
		{name: "default distinct", in: `{"vu:u4":[{"k":"1","v":"x"},{"k":"2"}]}`},
		{name: "identityref canonical", in: `{"vu:u5":[{"k":"1","id":"one"},{"k":"2","id":"vu:one"}]}`,
			want: []string{`unique constraint "id" is violated`}},
		{name: "decimal64 canonical", in: `{"vu:u6":[{"k":"1","d":"1.5"},{"k":"2","d":"1.50"}]}`,
			want: []string{`unique constraint "d" is violated`}},
		{name: "default under non-presence container", in: `{"vu:u7":[{"k":"1"},{"k":"2"}]}`,
			want: []string{`unique constraint "c/v" is violated`}},
		{name: "default in default case", in: `{"vu:u8":[{"k":"1"},{"k":"2"}]}`,
			want: []string{`unique constraint "ch/cd/cv" is violated`}},
		{name: "default in unselected case", in: `{"vu:u8":[{"k":"1","ce":"a"},{"k":"2","ce":"b"}]}`},
		{name: "state list", in: `{"vu:st":[{"k":"1","v":"s"},{"k":"2","v":"s"}]}`,
			want: []string{`unique constraint "v" is violated`}},
	})
}

// RFC 7950 §9.9: every leaf-list value of leafref type needs a target
// instance (unless require-instance is false), and the comparison is by value
// (an identityref names the same identity with or without its module).
func TestValidateLeafrefLeafListAndCanonicalCompare(t *testing.T) {
	base := `module vl {
        yang-version 1.1; namespace "urn:vl"; prefix vl;
        identity base; identity one { base base; }
        list t { key k; leaf k { type string; } }
        leaf-list refs { type leafref { path "/vl:t/vl:k"; } }
        leaf-list refsni { type leafref { path "/vl:t/vl:k"; require-instance false; } }
        leaf-list ids { type identityref { base base; } }
        container holder { }
    }`
	aug := `module vla {
        yang-version 1.1; namespace "urn:vla"; prefix vla;
        import vl { prefix vl; }
        augment /vl:holder {
            leaf r { type leafref { path "/vl:ids"; } }
            leaf-list rl { type leafref { path "/vl:ids"; } }
        }
    }`
	checkVerdicts(t, loadMultiModSrc(t, "vl", base, aug), []verdictCase{
		{name: "dangling leaf-list value", in: `{"vl:t":[{"k":"a"}],"vl:refs":["a","nope"]}`,
			want: []string{`/refs[1]: leafref value "nope" has no matching instance`}, not: []string{"/refs[0]"}},
		{name: "resolved leaf-list values", in: `{"vl:t":[{"k":"a"},{"k":"b"}],"vl:refs":["a","b"]}`},
		{name: "require-instance false", in: `{"vl:refsni":["nope"]}`},
		{name: "cross-module identityref leaf", in: `{"vl:ids":["one"],"vl:holder":{"vla:r":"vl:one"}}`},
		{name: "cross-module identityref leaf-list", in: `{"vl:ids":["one"],"vl:holder":{"vla:rl":["vl:one"]}}`},
		{name: "cross-module identityref dangling", in: `{"vl:holder":{"vla:r":"vl:one"}}`,
			want: []string{`/holder/r: leafref value "vl:one" has no matching instance`}},
	})
}

// RFC 7950 §7.7.1: leaf-list values must be unique only in configuration
// data; state leaf-lists may repeat a value. Duplicates compare canonically.
func TestValidateLeafListDuplicates(t *testing.T) {
	checkVerdicts(t, loadModSrc(t, `module vd {
        yang-version 1.1; namespace "urn:vd"; prefix vd;
        identity base; identity one { base base; }
        container c3 { config false; leaf-list st { type string; } leaf-list sti { type int8; } }
        leaf-list ids { type identityref { base base; } }
        leaf-list i64l { type int64; }
    }`, "vd"), []verdictCase{
		{name: "state leaf-list duplicates", in: `{"vd:c3":{"st":["x","x"],"sti":[1,1]}}`},
		{name: "identityref duplicates", in: `{"vd:ids":["one","vd:one"]}`,
			want: []string{"/ids[1] has a duplicate leaf-list value"}},
		{name: "int64 duplicates", in: `{"vd:i64l":["1","01","+1"]}`,
			want: []string{"/i64l[1] has a duplicate leaf-list value", "/i64l[2] has a duplicate leaf-list value"}},
	})
}

// RFC 7950 §9.3.4: a decimal64 value is an int64 scaled by 10^fraction-digits,
// so fraction-digits n bounds the value to ±9223372036854775807 / 10^n.
func TestValidateDecimal64ImplicitRange(t *testing.T) {
	checkVerdicts(t, loadModSrc(t, `module vx {
        yang-version 1.1; namespace "urn:vx"; prefix vx;
        leaf d18 { type decimal64 { fraction-digits 18; } }
        leaf d1 { type decimal64 { fraction-digits 1; } }
    }`, "vx"), []verdictCase{
		{name: "fd18 overflow", in: `{"vx:d18":"100.0"}`,
			want: []string{`/d18: 100.0 is outside the decimal64 range for fraction-digits 18`}},
		{name: "fd18 max", in: `{"vx:d18":"9.223372036854775807"}`},
		{name: "fd18 max+1", in: `{"vx:d18":"9.223372036854775808"}`,
			want: []string{"outside the decimal64 range"}},
		{name: "fd18 min", in: `{"vx:d18":"-9.223372036854775808"}`},
		{name: "fd18 min-1", in: `{"vx:d18":"-9.223372036854775809"}`,
			want: []string{"outside the decimal64 range"}},
		{name: "fd1 max", in: `{"vx:d1":"922337203685477580.7"}`},
		{name: "fd1 max+1", in: `{"vx:d1":"922337203685477580.8"}`,
			want: []string{"outside the decimal64 range"}},
	})
}

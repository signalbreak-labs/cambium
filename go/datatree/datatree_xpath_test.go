package datatree

import "testing"

func TestXPathParseSupported(t *testing.T) {
	exprs := []string{
		"true()",
		"not(../enabled = 'false')",
		"count(server) <= 1",
		"../type = 'ethernet'",
		"a and b or c",
		"/m:x/m:y",
		"../../z",
		"name != 'root'",
		"port > 0 and port < 65536",
		"foo[bar = 'x']",
		"count(//item) > 0",
		"a | b",
		"1 + 2 * 3",
		"current()",
		"derived-from-or-self(type, 'mod:base')",
	}
	for _, e := range exprs {
		if _, err := parseXPath(e); err != nil {
			t.Errorf("parseXPath(%q) unexpected error: %v", e, err)
		}
	}
}

func TestXPathParseRejected(t *testing.T) {
	exprs := []string{
		"@attr",         // attribute axis
		"child::foo",    // explicit axis
		"$var",          // variable reference
		"a =",           // dangling operator
		"(unclosed",     // missing ')'
		"'unterminated", // bad literal
		"a ! b",         // stray '!'
		"foo[",          // unclosed predicate
	}
	for _, e := range exprs {
		if _, err := parseXPath(e); err == nil {
			t.Errorf("parseXPath(%q) expected error, got none", e)
		}
	}
}

func TestXPathParseShape(t *testing.T) {
	cases := map[string]string{
		"../x = 'y'":          "(= ../x 'y')",
		"count(/a:b/a:c) > 0": "(> count(/b/c) 0)",
		"a and b":             "(and a b)",
		"not(z)":              "not(z)",
	}
	for in, want := range cases {
		e, err := parseXPath(in)
		if err != nil {
			t.Fatalf("parseXPath(%q): %v", in, err)
		}
		if got := e.String(); got != want {
			t.Errorf("parseXPath(%q).String() = %q, want %q", in, got, want)
		}
	}
}

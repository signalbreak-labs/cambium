package datatree_test

import (
	"testing"

	"github.com/signalbreak-labs/cambium/go/datatree"
)

const mdfSchema = `module mdf {
    namespace "urn:mdf"; prefix mdf;
    leaf a { type string; default "hello"; }
    leaf b { type int32;  default 5; }
    leaf c { type string; }
    container k { leaf x { type string; default "kx"; } }
}`

func applyDefaultsJSON(t *testing.T, src, name, in string) string {
	t.Helper()
	mod := loadModSrc(t, src, name)
	tree, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(in))
	if err != nil {
		t.Fatalf("Parse(%s): %v", in, err)
	}
	tree.ApplyDefaults()
	out, err := tree.Serialize(datatree.FormatJSONIETF)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	return string(out)
}

func TestApplyDefaultsFillsAbsentLeaves(t *testing.T) {
	got := applyDefaultsJSON(t, mdfSchema, "mdf", `{"mdf:c":"given"}`)
	want := `{"mdf:a":"hello","mdf:b":5,"mdf:c":"given"}`
	if got != want {
		t.Fatalf("ApplyDefaults:\n got: %s\nwant: %s", got, want)
	}
}

func TestApplyDefaultsDoesNotOverride(t *testing.T) {
	got := applyDefaultsJSON(t, mdfSchema, "mdf", `{"mdf:a":"custom","mdf:c":"x"}`)
	want := `{"mdf:a":"custom","mdf:b":5,"mdf:c":"x"}`
	if got != want {
		t.Fatalf("ApplyDefaults override:\n got: %s\nwant: %s", got, want)
	}
}

func TestApplyDefaultsRecursesPresentContainer(t *testing.T) {
	got := applyDefaultsJSON(t, mdfSchema, "mdf", `{"mdf:k":{}}`)
	want := `{"mdf:a":"hello","mdf:b":5,"mdf:k":{"x":"kx"}}`
	if got != want {
		t.Fatalf("ApplyDefaults container:\n got: %s\nwant: %s", got, want)
	}
}

func TestApplyDefaultsIdempotent(t *testing.T) {
	mod := loadModSrc(t, mdfSchema, "mdf")
	tree, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(`{"mdf:c":"given"}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	tree.ApplyDefaults()
	first, _ := tree.Serialize(datatree.FormatJSONIETF)
	tree.ApplyDefaults()
	second, _ := tree.Serialize(datatree.FormatJSONIETF)
	if string(first) != string(second) {
		t.Fatalf("ApplyDefaults not idempotent:\n first: %s\nsecond: %s", first, second)
	}
}

const miSchema = `module mi {
    namespace "urn:mi"; prefix mi;
    identity base-id;
    identity der1 { base base-id; }
    identity der2 { base der1; }
    identity other;
    leaf ref { type identityref { base base-id; } }
}`

func TestIdentityRefDerivation(t *testing.T) {
	mod := loadModSrc(t, miSchema, "mi")
	cases := []struct {
		value string
		valid bool
	}{
		{`"der1"`, true},      // direct derived
		{`"der2"`, true},      // transitively derived
		{`"base-id"`, true},   // the base itself
		{`"other"`, false},    // unrelated identity
		{`"nonexist"`, false}, // unknown
	}
	for _, tc := range cases {
		err := validateOne(t, mod, `{"mi:ref":`+tc.value+`}`)
		if tc.valid && err != nil {
			t.Errorf("identityref %s should be valid: %v", tc.value, err)
		}
		if !tc.valid && err == nil {
			t.Errorf("identityref %s should be rejected", tc.value)
		}
	}
}

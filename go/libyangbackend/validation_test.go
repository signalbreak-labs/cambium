//go:build cgo

package libyangbackend_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	cambium "github.com/signalbreak-labs/cambium/go/libyangbackend"
)

func loadValidationContext(t *testing.T) (*cambium.Context, string) {
	t.Helper()
	dir := t.TempDir()
	module := filepath.Join(dir, "cambium-validation-demo.yang")
	src := `module cambium-validation-demo {
  namespace "urn:cambium:validation";
  prefix cvd;
  yang-version 1.1;
  revision 2026-06-14;

  container top {
    leaf name { type string; }
    leaf ref { type leafref { path "../name"; } }
    container c {
      leaf x {
        type uint8;
        must "../../name = 'open'" { error-app-tag "must-violation"; }
      }
    }
    leaf y {
      when "../name = 'enable'";
      type string;
    }
    leaf z {
      mandatory "true";
      type string;
    }
  }
}`
	if err := os.WriteFile(module, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	if err := ctx.SetSearchPath(dir); err != nil {
		ctx.Close()
		t.Fatalf("SetSearchPath: %v", err)
	}
	if err := ctx.LoadModule("cambium-validation-demo"); err != nil {
		ctx.Close()
		t.Fatalf("LoadModule: %v", err)
	}
	return ctx, dir
}

func parseValidationData(t *testing.T, ctx *cambium.Context, innerXML string) *cambium.DataTree {
	t.Helper()
	data := []byte(`<top xmlns="urn:cambium:validation">` + innerXML + `</top>`)
	tree, err := ctx.Parse(cambium.FormatXML, cambium.ParseMode{ParseOnly: true}, data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return tree
}

func validationCodes(errors *cambium.ValidationErrors) []cambium.ValidationCode {
	var out []cambium.ValidationCode
	for _, d := range errors.Diagnostics() {
		if d.ValidationCode != cambium.ValidationNone {
			out = append(out, d.ValidationCode)
		}
	}
	return out
}

func TestValidateMustAndWhen(t *testing.T) {
	ctx, _ := loadValidationContext(t)
	defer ctx.Close()

	tree := parseValidationData(t, ctx, `<name>closed</name><c><x>1</x></c><y>foo</y><z>ok</z>`)
	defer tree.Close()

	err := tree.Validate(cambium.ValidateMode{Present: true, MultiError: true})
	if err == nil {
		t.Fatal("expected validation error")
	}
	var ve *cambium.ValidationErrors
	if !errors.As(err, &ve) {
		t.Fatalf("error does not wrap ValidationErrors: %v", err)
	}
	codes := validationCodes(ve)
	if !slices.Contains(codes, cambium.ValidationMust) {
		t.Fatalf("missing Must code in %v", codes)
	}
	// G-14 wording pin: `When` violations carry no libyang app-tag, so
	// validationCodeFromRaw classifies them by the message prefix "When
	// condition". If a libyang bump changes that wording this assertion fails —
	// update the prefix in validation.go to match the new message.
	if !slices.Contains(codes, cambium.ValidationWhen) {
		t.Fatalf("missing When code in %v — libyang `When condition ...` message wording may have changed; "+
			"update validationCodeFromRaw's prefix match in validation.go", codes)
	}

	var must *cambium.Diagnostic
	for _, d := range ve.Diagnostics() {
		if d.ValidationCode == cambium.ValidationMust {
			must = &d
			break
		}
	}
	if must == nil {
		t.Fatal("Must diagnostic not found")
	}
	if must.ErrorAppTag != "must-violation" {
		t.Fatalf("Must app-tag = %q, want must-violation", must.ErrorAppTag)
	}
	if must.ErrorType != cambium.ErrorTypeApplication {
		t.Fatalf("Must error type = %v", must.ErrorType)
	}
	if !strings.Contains(must.DataPath, "/c") {
		t.Fatalf("Must data path = %q, want /c", must.DataPath)
	}
}

func TestValidateMandatoryMissing(t *testing.T) {
	ctx, _ := loadValidationContext(t)
	defer ctx.Close()

	tree := parseValidationData(t, ctx, `<name>anything</name>`)
	defer tree.Close()

	err := tree.Validate(cambium.ValidateMode{Present: true})
	if err == nil {
		t.Fatal("expected validation error")
	}
	var ve *cambium.ValidationErrors
	if !errors.As(err, &ve) {
		t.Fatalf("error does not wrap ValidationErrors: %v", err)
	}
	primary, ok := ve.Primary()
	if !ok {
		t.Fatal("expected primary diagnostic")
	}
	if primary.ValidationCode != cambium.ValidationMandatory {
		t.Fatalf("primary code = %v, want Mandatory", primary.ValidationCode)
	}
	if primary.ErrorAppTag != "" {
		t.Fatalf("primary app-tag = %q, want empty", primary.ErrorAppTag)
	}
	if !strings.Contains(strings.ToLower(primary.Message), "mandatory") {
		t.Fatalf("primary message = %q, want mandatory", primary.Message)
	}
}

func TestValidateLeafrefUnresolved(t *testing.T) {
	ctx, _ := loadValidationContext(t)
	defer ctx.Close()

	tree := parseValidationData(t, ctx, `<name>a</name><ref>no-such</ref><z>ok</z>`)
	defer tree.Close()

	err := tree.Validate(cambium.ValidateMode{Present: true})
	if err == nil {
		t.Fatal("expected validation error")
	}
	var ve *cambium.ValidationErrors
	if !errors.As(err, &ve) {
		t.Fatalf("error does not wrap ValidationErrors: %v", err)
	}
	primary, _ := ve.Primary()
	if primary.ValidationCode != cambium.ValidationLeafref {
		t.Fatalf("primary code = %v, want Leafref", primary.ValidationCode)
	}
	if primary.ErrorAppTag != "instance-required" {
		t.Fatalf("primary app-tag = %q, want instance-required", primary.ErrorAppTag)
	}
	if !strings.Contains(primary.DataPath, "/ref") {
		t.Fatalf("primary data path = %q, want /ref", primary.DataPath)
	}
}

func TestValidateMultiError(t *testing.T) {
	ctx, _ := loadValidationContext(t)
	defer ctx.Close()

	tree := parseValidationData(t, ctx, `<name>a</name><ref>no-such</ref>`)
	defer tree.Close()

	err := tree.Validate(cambium.ValidateMode{Present: true, MultiError: true})
	if err == nil {
		t.Fatal("expected validation error")
	}
	var ve *cambium.ValidationErrors
	if !errors.As(err, &ve) {
		t.Fatalf("error does not wrap ValidationErrors: %v", err)
	}
	if ve.Len() < 2 {
		t.Fatalf("expected at least 2 diagnostics, got %d", ve.Len())
	}
	codes := validationCodes(ve)
	if !slices.Contains(codes, cambium.ValidationMandatory) {
		t.Fatalf("missing Mandatory code in %v", codes)
	}
	if !slices.Contains(codes, cambium.ValidationLeafref) {
		t.Fatalf("missing Leafref code in %v", codes)
	}
}

func TestValidatePresentRunningDatastorePasses(t *testing.T) {
	ctx, _ := loadValidationContext(t)
	defer ctx.Close()

	tree := parseValidationData(t, ctx, `<name>open</name><z>ok</z>`)
	defer tree.Close()

	if err := tree.Validate(cambium.ValidateMode{Present: true}); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

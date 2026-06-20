//go:build cgo

package libyangbackend_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cambium "github.com/signalbreak-labs/cambium/go/libyangbackend"
)

func conformanceDir() string {
	dir, err := filepath.Abs("../../conformance")
	if err != nil {
		panic(err)
	}
	return dir
}

func loadConformanceFixture(t *testing.T, name string) *cambium.Context {
	t.Helper()
	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ctx.Close)
	moduleDir := filepath.Join(conformanceDir(), "fixtures", name, "module")
	if err := ctx.SetSearchPath(moduleDir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule(name); err != nil {
		t.Fatal(err)
	}
	return ctx
}

func assertParseErrorCode(t *testing.T, ctx *cambium.Context, xml string, want cambium.RuleCode) {
	t.Helper()
	_, err := ctx.Parse(cambium.FormatXML, cambium.ParseModeDataOnly, []byte(xml))
	if err == nil {
		t.Fatal("expected parse error")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) {
		t.Fatalf("expected *cambium.Error, got %T", err)
	}
	if ce.RuleCode() != want {
		t.Fatalf("rule code = %s, want %s", ce.RuleCode(), want)
	}
}

// TestRuleCodeContext: loading a missing module yields CAMBIUM_E0001 (Context),
// the same code the Rust core assigns for the same failure.
func TestRuleCodeContext(t *testing.T) {
	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()

	err = ctx.LoadModule("definitely-not-a-real-module")
	if err == nil {
		t.Fatal("expected error loading missing module")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) {
		t.Fatalf("expected *cambium.Error, got %T: %v", err, err)
	}
	if ce.RuleCode() != cambium.RuleCodeContext {
		t.Fatalf("rule code = %s, want %s", ce.RuleCode(), cambium.RuleCodeContext)
	}
}

// TestRuleCodeParse: an interior-NUL input is rejected with CAMBIUM_E0002 (Parse).
func TestRuleCodeParse(t *testing.T) {
	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()

	_, err = ctx.Parse(cambium.FormatXML, cambium.ParseModeDataOnly, []byte("<x>\x00</x>"))
	if err == nil {
		t.Fatal("expected parse error on interior NUL")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) {
		t.Fatalf("expected *cambium.Error, got %T", err)
	}
	if ce.RuleCode() != cambium.RuleCodeParse {
		t.Fatalf("rule code = %s, want %s", ce.RuleCode(), cambium.RuleCodeParse)
	}
}

// TestRuleCodeValidateInt32Gap: a value inside a multipart range gap is rejected with E0003.
func TestRuleCodeValidateInt32Gap(t *testing.T) {
	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	moduleDir := filepath.Join(conformanceDir(), "fixtures/types-int-int32-range-multipart/module")
	if err := ctx.SetSearchPath(moduleDir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule("types-int-int32-range-multipart"); err != nil {
		t.Fatal(err)
	}
	_, err = ctx.Parse(cambium.FormatXML, cambium.ParseModeDataOnly,
		[]byte(`<priority xmlns="urn:types-int-int32-range-multipart">0</priority>`))
	if err == nil {
		t.Fatal("expected validation error on gap value")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeParse {
		t.Fatalf("rule code = %s, want %s", ce.RuleCode(), cambium.RuleCodeParse)
	}
}

// TestRuleCodeValidatePortZero: a uint16 port value 0 is rejected with E0002.
func TestRuleCodeValidatePortZero(t *testing.T) {
	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	moduleDir := filepath.Join(conformanceDir(), "fixtures/types-uint-uint16-range-port/module")
	if err := ctx.SetSearchPath(moduleDir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule("types-uint-uint16-range-port"); err != nil {
		t.Fatal(err)
	}
	_, err = ctx.Parse(cambium.FormatXML, cambium.ParseModeDataOnly,
		[]byte(`<port xmlns="urn:types-uint-uint16-range-port">0</port>`))
	if err == nil {
		t.Fatal("expected validation error on port 0")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeParse {
		t.Fatalf("rule code = %s, want %s", ce.RuleCode(), cambium.RuleCodeParse)
	}
}

// Theme 2: type-restrictions rejection parity (parse-time violations -> E0002).

func TestRuleCodeInvertMatchLowercase(t *testing.T) {
	ctx := loadConformanceFixture(t, "types-string-pattern-modifier-invert-match")
	assertParseErrorCode(t, ctx,
		`<name xmlns="urn:types-string-pattern-modifier-invert-match">abc</name>`,
		cambium.RuleCodeParse)
}

func TestRuleCodeMultiplePatternsLetterOnly(t *testing.T) {
	ctx := loadConformanceFixture(t, "types-string-multiple-patterns-conjunction")
	assertParseErrorCode(t, ctx,
		`<token xmlns="urn:types-string-multiple-patterns-conjunction">abc</token>`,
		cambium.RuleCodeParse)
}

func TestRuleCodeMultiplePatternsDigitOnly(t *testing.T) {
	ctx := loadConformanceFixture(t, "types-string-multiple-patterns-conjunction")
	assertParseErrorCode(t, ctx,
		`<token xmlns="urn:types-string-multiple-patterns-conjunction">123</token>`,
		cambium.RuleCodeParse)
}

func TestRuleCodeLengthPatternTooLong(t *testing.T) {
	ctx := loadConformanceFixture(t, "types-string-length-pattern-anchor-posix")
	assertParseErrorCode(t, ctx,
		`<code xmlns="urn:types-string-length-pattern-anchor-posix">12345678901</code>`,
		cambium.RuleCodeParse)
}

func TestRuleCodeLengthPatternNonAlnum(t *testing.T) {
	ctx := loadConformanceFixture(t, "types-string-length-pattern-anchor-posix")
	assertParseErrorCode(t, ctx,
		`<code xmlns="urn:types-string-length-pattern-anchor-posix">abc-</code>`,
		cambium.RuleCodeParse)
}

func TestRuleCodeRangeLengthRejectUnderflow(t *testing.T) {
	ctx := loadConformanceFixture(t, "constraints-range-length-reject")
	assertParseErrorCode(t, ctx,
		`<top xmlns="urn:constraints-range-length-reject"><n>0</n><s>abc</s></top>`,
		cambium.RuleCodeParse)
}

func TestRuleCodeRangeLengthRejectOverflow(t *testing.T) {
	ctx := loadConformanceFixture(t, "constraints-range-length-reject")
	assertParseErrorCode(t, ctx,
		`<top xmlns="urn:constraints-range-length-reject"><n>101</n><s>abc</s></top>`,
		cambium.RuleCodeParse)
}

func TestRuleCodeRangeLengthRejectStringTooLong(t *testing.T) {
	ctx := loadConformanceFixture(t, "constraints-range-length-reject")
	assertParseErrorCode(t, ctx,
		`<top xmlns="urn:constraints-range-length-reject"><n>42</n><s>toolong</s></top>`,
		cambium.RuleCodeParse)
}

func TestRuleCodeDecimal64ExponentInput(t *testing.T) {
	ctx := loadConformanceFixture(t, "json-ietf-decimal64-no-exponent")
	assertParseErrorCode(t, ctx,
		`<rate xmlns="urn:json-ietf-decimal64-no-exponent">1E-9</rate>`,
		cambium.RuleCodeParse)
}

func TestRuleCodeTypedefNarrowingOverflow(t *testing.T) {
	ctx := loadConformanceFixture(t, "types-typedef-restriction-narrowing")
	assertParseErrorCode(t, ctx,
		`<ssh xmlns="urn:types-typedef-restriction-narrowing">2000</ssh>`,
		cambium.RuleCodeParse)
}

// Theme 13: linkage rejection parity.

func loadConformanceFixtureDir(t *testing.T, name string) *cambium.Context {
	t.Helper()
	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ctx.Close)
	moduleDir := filepath.Join(conformanceDir(), "fixtures", name, "module")
	if err := ctx.SetSearchPath(moduleDir); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(moduleDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yang") {
			continue
		}
		path := filepath.Join(moduleDir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(strings.TrimSpace(string(data)), "submodule ") {
			continue
		}
		modName := strings.TrimSuffix(e.Name(), ".yang")
		modName = strings.Split(modName, "@")[0]
		if err := ctx.LoadModule(modName); err != nil {
			t.Fatal(err)
		}
	}
	return ctx
}

func assertValidateErrorCode(t *testing.T, ctx *cambium.Context, xml string, want cambium.RuleCode) {
	t.Helper()
	tree, err := ctx.Parse(cambium.FormatXML, cambium.ParseMode{ParseOnly: true}, []byte(xml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Close()
	err = tree.Validate(cambium.ValidateMode{Present: true})
	if err == nil {
		t.Fatal("expected validation error")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) {
		t.Fatalf("expected *cambium.Error, got %T", err)
	}
	if ce.RuleCode() != want {
		t.Fatalf("rule code = %s, want %s", ce.RuleCode(), want)
	}
}

func TestRuleCodeRefinePresenceMustRejected(t *testing.T) {
	ctx := loadConformanceFixtureDir(t, "linkage-refine-presence-must")
	assertValidateErrorCode(t, ctx,
		`<system xmlns="urn:linkage-refine-presence-must"><opts><val>0</val></opts></system>`,
		cambium.RuleCodeValidate)
}

func TestRuleCodeRefineMinMaxEmptyTagsRejected(t *testing.T) {
	ctx := loadConformanceFixtureDir(t, "linkage-refine-min-max-iffeature")
	assertValidateErrorCode(t, ctx,
		`<policy xmlns="urn:linkage-refine-min-max-iffeature"></policy>`+
			`<service xmlns="urn:linkage-refine-min-max-iffeature"><mode>auto</mode></service>`,
		cambium.RuleCodeValidate)
}

func TestRuleCodeAugmentIntraModuleWhenFalseRejected(t *testing.T) {
	ctx := loadConformanceFixtureDir(t, "linkage-augment-intra-module")
	assertValidateErrorCode(t, ctx,
		`<interfaces xmlns="urn:linkage-augment-intra-module"><interface><name>lag0</name><type>lag</type><speed>1000</speed></interface></interfaces>`,
		cambium.RuleCodeValidate)
}

func TestRuleCodeAugmentWhenTargetContextDisabledRejected(t *testing.T) {
	ctx := loadConformanceFixtureDir(t, "augment-when-target-context")
	tree, err := ctx.Parse(cambium.FormatXML, cambium.ParseMode{ParseOnly: true}, []byte(
		`<system xmlns="urn:augment-when-target-context-base" xmlns:awtca="urn:augment-when-target-context"><mode>disabled</mode><ospf><router-id>1.1.1.1</router-id><awtca:area>0</awtca:area></ospf></system>`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Close()
	err = tree.Validate(cambium.ValidateMode{})
	if err == nil {
		t.Fatal("expected validation error")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeValidate {
		t.Fatalf("rule code = %s, want %s", ce.RuleCode(), cambium.RuleCodeValidate)
	}
}

func TestRuleCodeDeviationNotSupportedUnknownRejected(t *testing.T) {
	ctx := loadConformanceFixtureDir(t, "linkage-deviation-not-supported")
	_, err := ctx.Parse(cambium.FormatXML, cambium.ParseMode{Strict: true, ParseOnly: true},
		[]byte(`<c xmlns="urn:linkage-deviation-not-supported-base"><deprecated-field>x</deprecated-field><active-field>ok</active-field></c>`))
	if err == nil {
		t.Fatal("expected strict parse error")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeParse {
		t.Fatalf("rule code = %s, want %s", ce.RuleCode(), cambium.RuleCodeParse)
	}
}

func TestRuleCodeDeviationReplaceTypeOverflowRejected(t *testing.T) {
	ctx := loadConformanceFixtureDir(t, "linkage-deviation-replace-type")
	_, err := ctx.Parse(cambium.FormatXML, cambium.ParseModeDataOnly,
		[]byte(`<c xmlns="urn:linkage-deviation-replace-type-base"><count>2000</count></c>`))
	if err == nil {
		t.Fatal("expected parse error")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeParse {
		t.Fatalf("rule code = %s, want %s", ce.RuleCode(), cambium.RuleCodeParse)
	}
}

func TestRuleCodeDeviationAddMandatoryMissingRejected(t *testing.T) {
	ctx := loadConformanceFixtureDir(t, "linkage-deviation-add")
	assertValidateErrorCode(t, ctx,
		`<c xmlns="urn:linkage-deviation-add-base"/>`,
		cambium.RuleCodeValidate)
}

func TestRuleCodeDeviationMultiLegacyRejected(t *testing.T) {
	ctx := loadConformanceFixtureDir(t, "linkage-deviation-multi")
	_, err := ctx.Parse(cambium.FormatXML, cambium.ParseMode{Strict: true, ParseOnly: true},
		[]byte(`<c xmlns="urn:linkage-deviation-multi-base"><legacy>old</legacy><name>x</name><maximum>500</maximum></c>`))
	if err == nil {
		t.Fatal("expected strict parse error")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeParse {
		t.Fatalf("rule code = %s, want %s", ce.RuleCode(), cambium.RuleCodeParse)
	}
}

func TestRuleCodeDeviationMultiMaximumOverflowRejected(t *testing.T) {
	ctx := loadConformanceFixtureDir(t, "linkage-deviation-multi")
	_, err := ctx.Parse(cambium.FormatXML, cambium.ParseModeDataOnly,
		[]byte(`<c xmlns="urn:linkage-deviation-multi-base"><name>x</name><maximum>2000</maximum></c>`))
	if err == nil {
		t.Fatal("expected parse error")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeParse {
		t.Fatalf("rule code = %s, want %s", ce.RuleCode(), cambium.RuleCodeParse)
	}
}

func TestRuleCodeDeviationMultiNameMissingRejected(t *testing.T) {
	ctx := loadConformanceFixtureDir(t, "linkage-deviation-multi")
	assertValidateErrorCode(t, ctx,
		`<c xmlns="urn:linkage-deviation-multi-base"><maximum>500</maximum></c>`,
		cambium.RuleCodeValidate)
}

func TestRuleCodeDeviationReplaceDefaultConfigMustRejected(t *testing.T) {
	ctx := loadConformanceFixtureDir(t, "linkage-deviation-replace-default-config")
	assertValidateErrorCode(t, ctx,
		`<system xmlns="urn:linkage-deviation-replace-default-config-base"><mode>disabled</mode><ospf><router-id>1.1.1.1</router-id></ospf></system>`,
		cambium.RuleCodeValidate)
}

func TestRuleCodeImportNonTransitiveAFails(t *testing.T) {
	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	moduleDir := filepath.Join(conformanceDir(), "fixtures", "linkage-import-non-transitive", "module")
	if err := ctx.SetSearchPath(moduleDir); err != nil {
		t.Fatal(err)
	}
	err = ctx.LoadModule("linkage-import-non-transitive-a")
	if err == nil {
		t.Fatal("expected error loading module A")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
		t.Fatalf("rule code = %s, want %s", ce.RuleCode(), cambium.RuleCodeContext)
	}
}

func TestRuleCodeImportNonTransitiveBLoads(t *testing.T) {
	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	moduleDir := filepath.Join(conformanceDir(), "fixtures", "linkage-import-non-transitive", "module")
	if err := ctx.SetSearchPath(moduleDir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule("linkage-import-non-transitive-b"); err != nil {
		t.Fatalf("loading module B: %v", err)
	}
}

// Theme 15: edge-illegality rejection parity.

func loadEdgeFixture(t *testing.T, name string) *cambium.Context {
	t.Helper()
	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ctx.Close)
	moduleDir := filepath.Join(conformanceDir(), "fixtures", name, "module")
	if err := ctx.SetSearchPath(moduleDir); err != nil {
		t.Fatal(err)
	}
	return ctx
}

func TestRuleCodeParseMalformedTruncated(t *testing.T) {
	ctx := loadEdgeFixture(t, "parse-malformed-e0002")
	if err := ctx.LoadModule("parse-malformed-e0002"); err != nil {
		t.Fatal(err)
	}
	_, err := ctx.Parse(cambium.FormatXML, cambium.ParseModeDataOnly,
		[]byte(`<top xmlns="urn:parse-malformed-e0002"><name>incomplete</name>`))
	if err == nil {
		t.Fatal("expected parse error")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeParse {
		t.Fatalf("rule code = %s, want %s", ce.RuleCode(), cambium.RuleCodeParse)
	}
}

func TestRuleCodeParseMalformedInteriorNUL(t *testing.T) {
	ctx := loadEdgeFixture(t, "parse-malformed-e0002")
	if err := ctx.LoadModule("parse-malformed-e0002"); err != nil {
		t.Fatal(err)
	}
	xml := []byte(`<top xmlns="urn:parse-malformed-e0002"><name>`)
	xml = append(xml, 0)
	xml = append(xml, []byte(`x</name></top>`)...)
	_, err := ctx.Parse(cambium.FormatXML, cambium.ParseModeDataOnly, xml)
	if err == nil {
		t.Fatal("expected parse error")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeParse {
		t.Fatalf("rule code = %s, want %s", ce.RuleCode(), cambium.RuleCodeParse)
	}
}

func TestRuleCodeParseMalformedStrictUnknown(t *testing.T) {
	ctx := loadEdgeFixture(t, "parse-malformed-e0002")
	if err := ctx.LoadModule("parse-malformed-e0002"); err != nil {
		t.Fatal(err)
	}
	_, err := ctx.Parse(cambium.FormatXML, cambium.ParseMode{Strict: true, ParseOnly: true},
		[]byte(`<top xmlns="urn:parse-malformed-e0002"><name>ok</name><unknown>x</unknown></top>`))
	if err == nil {
		t.Fatal("expected parse error")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeParse {
		t.Fatalf("rule code = %s, want %s", ce.RuleCode(), cambium.RuleCodeParse)
	}
}

func TestRuleCodeEmptyDefaultIllegal(t *testing.T) {
	ctx := loadEdgeFixture(t, "types-empty-edge-illegality")
	err := ctx.LoadModule("empty-default-illegal")
	if err == nil {
		t.Fatal("expected schema load error")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
		t.Fatalf("rule code = %s, want %s", ce.RuleCode(), cambium.RuleCodeContext)
	}
}

func TestRuleCodeEmptyLeaflist10Illegal(t *testing.T) {
	ctx := loadEdgeFixture(t, "types-empty-edge-illegality")
	err := ctx.LoadModule("empty-leaflist-1_0-illegal")
	if err == nil {
		t.Fatal("expected schema load error")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
		t.Fatalf("rule code = %s, want %s", ce.RuleCode(), cambium.RuleCodeContext)
	}
}

func TestRuleCodeEmptyLeaflist11Legal(t *testing.T) {
	ctx := loadEdgeFixture(t, "types-empty-edge-illegality")
	if err := ctx.LoadModule("empty-leaflist-1_1-legal"); err != nil {
		t.Fatalf("expected schema to load: %v", err)
	}
}

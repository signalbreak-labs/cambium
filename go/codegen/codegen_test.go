package codegen_test

import (
	"bytes"
	"context"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/codegen"
)

func schemaFixtureDir(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(file, "..", "..", "..", "conformance", "fixtures", name)
}

func schemaCorpusDir(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(schemaFixtureDir(t, name), "..", "..", "corpus", name)
}

func goldenPath(t *testing.T, fixture, name string) string {
	t.Helper()
	return filepath.Join(schemaFixtureDir(t, fixture), "..", "..", "golden", fixture, name)
}

func loadModule(t *testing.T, moduleDir, module string) *cambium.Context {
	t.Helper()
	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatalf("new context: %v", err)
	}
	if err := ctx.SetSearchPath(moduleDir); err != nil {
		ctx.Close()
		t.Fatalf("set search path: %v", err)
	}
	if err := ctx.LoadModule(module); err != nil {
		ctx.Close()
		t.Fatalf("load module: %v", err)
	}
	return ctx
}

func generatedFixtureSource(t *testing.T, fixture, module string) string {
	t.Helper()
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, fixture), "module"), module)
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, module)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	return src
}

func readFixtureGoldenPair(t *testing.T, fixture string) (string, string) {
	t.Helper()
	wantXML, err := os.ReadFile(goldenPath(t, fixture, "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, fixture, "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}
	return string(wantXML), string(wantJSON)
}

func TestRunGeneratedCommandHonorsContextDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := runGeneratedCommand(ctx, t.TempDir(), "/bin/sh", "-c", "sleep 5")
	if err == nil {
		t.Fatal("runGeneratedCommand completed a command past its context deadline")
	}
	if ctx.Err() == nil {
		t.Fatalf("context was not canceled after runGeneratedCommand returned: %v", err)
	}
}

func TestGenerateGoIsDeterministic(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "scrambled-children"), "module"), "order-demo")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "order-demo")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	src2, err := codegen.GenerateGo(ctx, "order-demo")
	if err != nil {
		t.Fatalf("generate second: %v", err)
	}
	if src != src2 {
		t.Fatal("codegen output must be deterministic")
	}

	if !strings.Contains(src, `var OrderDemoTopFieldOrder = []string{"z", "m", "a"}`) {
		t.Fatalf("missing field-order manifest in generated source:\n%s", src)
	}
	if !strings.Contains(src, "type OrderDemoTop struct {") {
		t.Fatalf("missing OrderDemoTop struct in generated source:\n%s", src)
	}
	if !strings.Contains(src, "\tZ *string") || !strings.Contains(src, "\tM *string") || !strings.Contains(src, "\tA *string") {
		t.Fatalf("missing optional typed leaf fields in generated source:\n%s", src)
	}
}

func TestGenerateGoRangeLengthHelpersDeterministic(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "codegen-determinism-range-length"), "module"), "codegen-determinism-range-length")
	defer ctx.Close()

	var first string
	for i := 0; i < 5; i++ {
		src, err := codegen.GenerateGo(ctx, "codegen-determinism-range-length")
		if err != nil {
			t.Fatalf("generate iteration %d: %v", i, err)
		}
		if i == 0 {
			first = src
			continue
		}
		if src != first {
			t.Fatalf("codegen output changed between iteration 0 and %d", i)
		}
	}

	if !strings.Contains(first, "type CodegenDeterminismRangeLengthI8Range struct {") {
		t.Fatalf("missing int8 range helper in generated source:\n%s", first)
	}
	if !strings.Contains(first, "type CodegenDeterminismRangeLengthU16Range struct {") {
		t.Fatalf("missing uint16 range helper in generated source:\n%s", first)
	}
	if !strings.Contains(first, "type CodegenDeterminismRangeLengthLabelLength struct {") {
		t.Fatalf("missing label length helper in generated source:\n%s", first)
	}
	if !strings.Contains(first, "type CodegenDeterminismRangeLengthCodeLength struct {") {
		t.Fatalf("missing code length helper in generated source:\n%s", first)
	}
}

func TestGeneratedGoFieldOrderManifestKeysFirst(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "scrambled-children"), "module"), "order-demo")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "order-demo")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, `var OrderDemoTopFieldOrder = []string{"z", "m", "a"}`) {
		t.Fatalf("scrambled-children field order mismatch:\n%s", src)
	}

	ctx2 := loadModule(t, filepath.Join(schemaFixtureDir(t, "keys-first"), "module"), "keys-first-demo")
	defer ctx2.Close()
	src2, err := codegen.GenerateGo(ctx2, "keys-first-demo")
	if err != nil {
		t.Fatalf("generate keys-first: %v", err)
	}
	if !strings.Contains(src2, "type KeysFirstDemoServerEntry struct {") {
		t.Fatalf("missing list entry struct:\n%s", src2)
	}
	if !strings.Contains(src2, `var KeysFirstDemoServerEntryFieldOrder = []string{"name", "class", "description"}`) {
		t.Fatalf("keys-first field order missing or wrong:\n%s", src2)
	}
}

func TestGeneratedGoScrambledChildrenXMLMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "scrambled-children"), "module"), "order-demo")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "order-demo")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	want, err := os.ReadFile(goldenPath(t, "scrambled-children", "output.xml"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedOrderDemo(t *testing.T) {
	demo := OrderDemo{Top: OrderDemoTop{Z: ptr("z1"), M: ptr("m1"), A: ptr("a1")}}
	got := demo.ToXML()
	want := %q
	if got != want {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", got, want)
	}
}
`, string(want))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoScrambledChildrenJSONMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "scrambled-children"), "module"), "order-demo")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "order-demo")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	want, err := os.ReadFile(goldenPath(t, "scrambled-children", "output.json"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedOrderDemoJSON(t *testing.T) {
	demo := OrderDemo{Top: OrderDemoTop{Z: ptr("z1"), M: ptr("m1"), A: ptr("a1")}}
	got := demo.ToJSONIETF()
	want := %q
	if got != want {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", got, want)
	}
}
`, string(want))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoKeysFirstXMLMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "keys-first"), "module"), "keys-first-demo")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "keys-first-demo")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	want, err := os.ReadFile(goldenPath(t, "keys-first", "output.xml"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedKeysFirst(t *testing.T) {
	demo := KeysFirstDemo{
		Server: []KeysFirstDemoServerEntry{{
			Name:        "s1",
			Class:       "c1",
			Description: ptr("main"),
		}},
	}
	got := demo.ToXML()
	want := %q
	if got != want {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", got, want)
	}
}
`, string(want))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoKeysFirstJSONMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "keys-first"), "module"), "keys-first-demo")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "keys-first-demo")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	want, err := os.ReadFile(goldenPath(t, "keys-first", "output.json"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedKeysFirstJSON(t *testing.T) {
	demo := KeysFirstDemo{
		Server: []KeysFirstDemoServerEntry{{
			Name:        "s1",
			Class:       "c1",
			Description: ptr("main"),
		}},
	}
	got := demo.ToJSONIETF()
	want := %q
	if got != want {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", got, want)
	}
}
`, string(want))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoModuleNamespaceQualificationJSONMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "json-ietf-module-namespace-qualification"), "module"), "json-ietf-module-namespace-qualification")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "json-ietf-module-namespace-qualification")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	want, err := os.ReadFile(goldenPath(t, "json-ietf-module-namespace-qualification", "output.json"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedModuleNamespaceQualificationJSON(t *testing.T) {
	demo := JsonIetfModuleNamespaceQualification{
		Top: JsonIetfModuleNamespaceQualificationTop{
			LocalLeaf: ptr("one"),
			Nested: JsonIetfModuleNamespaceQualificationTopNested{
				Inner: ptr("two"),
			},
		},
	}
	got := demo.ToJSONIETF()
	want := %q
	if got != want {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", got, want)
	}
}
`, string(want))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoBooleanDefaultFalse(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-boolean-default-false"), "module"), "types-boolean-default-false")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-boolean-default-false")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-boolean-default-false", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-boolean-default-false", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedBooleanDefaultFalse(t *testing.T) {
	demo := TypesBooleanDefaultFalse{
		Top: TypesBooleanDefaultFalseTop{
			Enabled: ptr(false),
			Active:  ptr(true),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF anydata JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoInt8(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-int-int8-range"), "module"), "types-int-int8-range")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-int-int8-range")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-int-int8-range", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-int-int8-range", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedInt8(t *testing.T) {
	demo := TypesIntInt8Range{
		Top: TypesIntInt8RangeTop{
			I8Min:  ptr(int8(-128)),
			I8Zero: ptr(int8(0)),
			I8Max:  ptr(int8(127)),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	if _, err := FromJSONIETF([]byte(`+"`"+`{"types-int-int8-range:top":{"i8-zero":"0"}}`+"`"+`)); err == nil {
		t.Fatal("FromJSONIETF accepted quoted JSON_IETF int8")
	} else if got, want := err.Error(), "i8-zero must be a JSON integer"; got != want {
		t.Fatalf("FromJSONIETF error = %%q, want %%q", got, want)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoInt16Range(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-int-int16-range"), "module"), "types-int-int16-range")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-int-int16-range")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-int-int16-range", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-int-int16-range", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedInt16Range(t *testing.T) {
	r, err := NewTypesIntInt16RangeTopI16RangeRange(-500)
	if err != nil {
		t.Fatalf("new range: %%v", err)
	}
	demo := TypesIntInt16Range{
		Top: TypesIntInt16RangeTop{
			I16Full:  ptr(int16(-32768)),
			I16Range: ptr(r),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF range JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed range JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoInt64Quoted(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-int-int64-range-quoted"), "module"), "types-int-int64-range-quoted")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-int-int64-range-quoted")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	wantJSON, err := os.ReadFile(goldenPath(t, "types-int-int64-range-quoted", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedInt64Quoted(t *testing.T) {
	demo := TypesIntInt64RangeQuoted{Count: ptr(int64(-9223372036854775808))}
	got := demo.ToJSONIETF()
	want := %q
	if got != want {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", got, want)
	}
	parsed, err := FromJSONIETF([]byte(`+"`"+`{"types-int-int64-range-quoted:count":" -9223372036854775808 "}`+"`"+`))
	if err != nil {
		t.Fatalf("FromJSONIETF rejected whitespace-padded int64 string: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != want {
		t.Fatalf("canonical int64 JSON mismatch:\n got: %%q\nwant: %%q", got, want)
	}
}
`, string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoUint16RangePort(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-uint-uint16-range-port"), "module"), "types-uint-uint16-range-port")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-uint-uint16-range-port")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	wantJSON, err := os.ReadFile(goldenPath(t, "types-uint-uint16-range-port", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedUint16RangePort(t *testing.T) {
	port, err := NewTypesUintUint16RangePortPortRange(443)
	if err != nil {
		t.Fatalf("new range: %%v", err)
	}
	demo := TypesUintUint16RangePort{Port: ptr(port)}
	got := demo.ToJSONIETF()
	want := %q
	if got != want {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", got, want)
	}
	def := DefaultTypesUintUint16RangePortPortRange()
	if def.Get() != 1 {
		t.Fatalf("default value should be range minimum 1, got %%d", def.Get())
	}
	var invalid TypesUintUint16RangePortPortRange
	invalidDemo := TypesUintUint16RangePort{Port: &invalid}
	if err := invalidDemo.Validate(); err == nil {
		t.Fatal("Validate accepted zero-value range wrapper")
	} else if got, want := err.Error(), "/types-uint-uint16-range-port/port: value 0 out of range for TypesUintUint16RangePortPortRange"; got != want {
		t.Fatalf("Validate error = %%q, want %%q", got, want)
	}
}
`, string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoInt32RangeMultipart(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-int-int32-range-multipart"), "module"), "types-int-int32-range-multipart")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-int-int32-range-multipart")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-int-int32-range-multipart", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-int-int32-range-multipart", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedInt32RangeMultipart(t *testing.T) {
	priority, err := NewTypesIntInt32RangeMultipartPriorityRange(-50)
	if err != nil {
		t.Fatalf("new range: %%v", err)
	}
	demo := TypesIntInt32RangeMultipart{Priority: ptr(priority)}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoUint32RangeMulti(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-uint-uint32-range-multi"), "module"), "types-uint-uint32-range-multi")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-uint-uint32-range-multi")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-uint-uint32-range-multi", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-uint-uint32-range-multi", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedUint32RangeMulti(t *testing.T) {
	asNumber, err := NewTypesUintUint32RangeMultiAsNumberRange(65000)
	if err != nil {
		t.Fatalf("new range: %%v", err)
	}
	demo := TypesUintUint32RangeMulti{AsNumber: ptr(asNumber)}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoUint64Quoted(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-uint-uint64-range-quoted"), "module"), "types-uint-uint64-range-quoted")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-uint-uint64-range-quoted")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	wantJSON, err := os.ReadFile(goldenPath(t, "types-uint-uint64-range-quoted", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedUint64Quoted(t *testing.T) {
	demo := TypesUintUint64RangeQuoted{BytesSent: ptr(uint64(18446744073709551615))}
	got := demo.ToJSONIETF()
	want := %q
	if got != want {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", got, want)
	}
	if _, err := FromJSONIETF([]byte(`+"`"+`{"types-uint-uint64-range-quoted:bytes-sent":42}`+"`"+`)); err == nil {
		t.Fatal("FromJSONIETF accepted bare JSON_IETF uint64")
	} else if got, want := err.Error(), "types-uint-uint64-range-quoted:bytes-sent must be a JSON string"; got != want {
		t.Fatalf("FromJSONIETF error = %%q, want %%q", got, want)
	}
	parsed, err := FromJSONIETF([]byte(`+"`"+`{"types-uint-uint64-range-quoted:bytes-sent":" +42 "}`+"`"+`))
	if err != nil {
		t.Fatalf("FromJSONIETF rejected whitespace-padded uint64 string: %%v", err)
	}
	trimmedDemo := TypesUintUint64RangeQuoted{BytesSent: ptr(uint64(42))}
	wantTrimmed := trimmedDemo.ToJSONIETF()
	if got := parsed.ToJSONIETF(); got != wantTrimmed {
		t.Fatalf("canonical uint64 JSON mismatch:\n got: %%q\nwant: %%q", got, wantTrimmed)
	}
	zeroParsed, err := FromJSONIETF([]byte(`+"`"+`{"types-uint-uint64-range-quoted:bytes-sent":" -0 "}`+"`"+`))
	if err != nil {
		t.Fatalf("FromJSONIETF rejected whitespace-padded uint64 negative zero: %%v", err)
	}
	zeroDemo := TypesUintUint64RangeQuoted{BytesSent: ptr(uint64(0))}
	wantZero := zeroDemo.ToJSONIETF()
	if got := zeroParsed.ToJSONIETF(); got != wantZero {
		t.Fatalf("canonical uint64 -0 JSON mismatch:\n got: %%q\nwant: %%q", got, wantZero)
	}
}
`, string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoDecimal64Fraction1Range(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-decimal64-fraction1-range"), "module"), "types-decimal64-fraction1-range")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-decimal64-fraction1-range")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-decimal64-fraction1-range", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-decimal64-fraction1-range", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedDecimal64Fraction1Range(t *testing.T) {
	demo := TypesDecimal64Fraction1Range{Temp: ptr(NewDecimal64(-5, 1))}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	wrongFractionDigits := TypesDecimal64Fraction1Range{Temp: ptr(NewDecimal64(-5, 2))}
	if err := wrongFractionDigits.Validate(); err == nil {
		t.Fatal("Validate accepted decimal64 with wrong fraction-digits")
	} else if got, want := err.Error(), "/types-decimal64-fraction1-range/temp: decimal64 fraction-digits 2, want 1"; got != want {
		t.Fatalf("Validate error = %%q, want %%q", got, want)
	}
	tooLow := TypesDecimal64Fraction1Range{Temp: ptr(NewDecimal64(-501, 1))}
	if err := tooLow.Validate(); err == nil {
		t.Fatal("Validate accepted out-of-range decimal64")
	} else if got, want := err.Error(), "/types-decimal64-fraction1-range/temp: decimal64 value -50.1 out of bounds"; got != want {
		t.Fatalf("Validate error = %%q, want %%q", got, want)
	}
	if _, err := FromJSONIETF([]byte(`+"`"+`{"types-decimal64-fraction1-range:temp":"100.1"}`+"`"+`)); err == nil {
		t.Fatal("FromJSONIETF accepted out-of-range decimal64")
	} else if got, want := err.Error(), "types-decimal64-fraction1-range:temp decimal64 value 100.1 out of bounds"; got != want {
		t.Fatalf("FromJSONIETF error = %%q, want %%q", got, want)
	}
	spaced, err := FromJSONIETF([]byte(`+"`"+`{"types-decimal64-fraction1-range:temp":" -0.5 "}`+"`"+`))
	if err != nil {
		t.Fatalf("FromJSONIETF rejected libyang-compatible decimal64 numeric string whitespace: %%v", err)
	}
	if got := spaced.ToJSONIETF(); got != wantJSON {
		t.Fatalf("spaced decimal64 JSON canonicalization mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
	for _, payload := range []string{
		`+"`"+`{"types-decimal64-fraction1-range:temp":""}`+"`"+`,
		`+"`"+`{"types-decimal64-fraction1-range:temp":"."}`+"`"+`,
		`+"`"+`{"types-decimal64-fraction1-range:temp":"1."}`+"`"+`,
		`+"`"+`{"types-decimal64-fraction1-range:temp":".5"}`+"`"+`,
		`+"`"+`{"types-decimal64-fraction1-range:temp":"9223372036854775807.0"}`+"`"+`,
	} {
		if _, err := FromJSONIETF([]byte(payload)); err == nil {
			t.Fatalf("FromJSONIETF accepted malformed decimal64 JSON value %%s", payload)
		}
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoDecimal64Fraction2CanonicalRound(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-decimal64-fraction2-canonical-round"), "module"), "types-decimal64-fraction2-canonical-round")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-decimal64-fraction2-canonical-round")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-decimal64-fraction2-canonical-round", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-decimal64-fraction2-canonical-round", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedDecimal64Fraction2CanonicalRound(t *testing.T) {
	demo := TypesDecimal64Fraction2CanonicalRound{Rate: ptr(NewDecimal64(314, 2))}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF decimal64 JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed decimal64 JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoDecimal64Fraction3And6(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-decimal64-fraction3-and-6"), "module"), "types-decimal64-fraction3-and-6")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-decimal64-fraction3-and-6")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-decimal64-fraction3-and-6", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-decimal64-fraction3-and-6", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedDecimal64Fraction3And6(t *testing.T) {
	demo := TypesDecimal64Fraction3And6{
		Top: TypesDecimal64Fraction3And6Top{
			Milli: ptr(NewDecimal64(1500, 3)),
			Micro: ptr(NewDecimal64(1, 6)),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoDecimal64Fraction9Negative(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-decimal64-fraction9-negative"), "module"), "types-decimal64-fraction9-negative")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-decimal64-fraction9-negative")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-decimal64-fraction9-negative", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-decimal64-fraction9-negative", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedDecimal64Fraction9Negative(t *testing.T) {
	demo := TypesDecimal64Fraction9Negative{Delay: ptr(NewDecimal64(-1, 9))}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoDecimal64Fraction18MaxMagnitude(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-decimal64-fraction18-max-magnitude"), "module"), "types-decimal64-fraction18-max-magnitude")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-decimal64-fraction18-max-magnitude")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-decimal64-fraction18-max-magnitude", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-decimal64-fraction18-max-magnitude", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedDecimal64Fraction18MaxMagnitude(t *testing.T) {
	demo := TypesDecimal64Fraction18MaxMagnitude{Precise: ptr(NewDecimal64(9223372036854775807, 18))}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoJSONIETFDecimal64CanonicalQuoting(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "json-ietf-decimal64-canonical-quoting"), "module"), "json-ietf-decimal64-canonical-quoting")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "json-ietf-decimal64-canonical-quoting")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	wantJSON, err := os.ReadFile(goldenPath(t, "json-ietf-decimal64-canonical-quoting", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedJSONIETFDecimal64CanonicalQuoting(t *testing.T) {
	demo := JsonIetfDecimal64CanonicalQuoting{
		Top: JsonIetfDecimal64CanonicalQuotingTop{
			NegativeValue: ptr(NewDecimal64(-314, 2)),
			PositiveValue: ptr(NewDecimal64(250, 2)),
			ZeroValue:     ptr(NewDecimal64(0, 2)),
		},
	}
	got := demo.ToJSONIETF()
	want := %q
	if got != want {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", got, want)
	}
}
`, string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoJSONIETFDecimal64NoExponentMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "json-ietf-decimal64-no-exponent"), "module"), "json-ietf-decimal64-no-exponent")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "json-ietf-decimal64-no-exponent")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "json-ietf-decimal64-no-exponent", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "json-ietf-decimal64-no-exponent", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedJSONIETFDecimal64NoExponentMatchesLibyang(t *testing.T) {
	demo := JsonIetfDecimal64NoExponent{Rate: ptr(NewDecimal64(1, 9))}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoJSONStringEscapingMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "json-ietf-string-escaping-control-unicode"), "module"), "json-ietf-string-escaping-control-unicode")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "json-ietf-string-escaping-control-unicode")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "json-ietf-string-escaping-control-unicode", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "json-ietf-string-escaping-control-unicode", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedJSONStringEscapingMatchesLibyang(t *testing.T) {
	demo := JsonIetfStringEscapingControlUnicode{
		Top: JsonIetfStringEscapingControlUnicodeTop{
			WithControl:   ptr("line1\nline2\tend"),
			WithQuote:     ptr("say \"hello\""),
			WithBackslash: ptr("C:\\path"),
			French:        ptr("caf\u00e9"),
			Emoji:         ptr("\U0001F680"),
			Greek:         ptr("\u03b1\u03b2\u03b3"),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoValidateRejectsInvalidStringCharacters(t *testing.T) {
	const source = `module string-validation-codegen {
    namespace "urn:string-validation-codegen";
    prefix svc;

    leaf label {
        type string;
    }

    leaf-list tag {
        type string;
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	src, err := codegen.GenerateGo(ctx, "string-validation-codegen")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	testBody := `
func TestGeneratedValidateRejectsInvalidStringCharacters(t *testing.T) {
	valid := StringValidationCodegen{
		Label: ptr("line1\nline2\tend"),
		Tag:   []string{"alpha", "beta"},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate rejected valid string characters: %v", err)
	}

	invalidControl := StringValidationCodegen{Label: ptr("bad\x01value")}
	if err := invalidControl.Validate(); err == nil {
		t.Fatal("Validate accepted invalid control character in string leaf")
	} else if got, want := err.Error(), "/string-validation-codegen/label: invalid string character U+0001"; got != want {
		t.Fatalf("Validate control error = %q, want %q", got, want)
	}

	invalidUTF8 := StringValidationCodegen{Tag: []string{string([]byte{0xff})}}
	if err := invalidUTF8.Validate(); err == nil {
		t.Fatal("Validate accepted invalid UTF-8 in string leaf-list")
	} else if got, want := err.Error(), "/string-validation-codegen/tag: string value is not valid UTF-8"; got != want {
		t.Fatalf("Validate UTF-8 error = %q, want %q", got, want)
	}
}
`

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoJSONScalarQuotingMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "json-ietf-scalar-quoting-int-spans"), "module"), "json-ietf-scalar-quoting-int-spans")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "json-ietf-scalar-quoting-int-spans")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "json-ietf-scalar-quoting-int-spans", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "json-ietf-scalar-quoting-int-spans", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedJSONScalarQuotingMatchesLibyang(t *testing.T) {
	demo := JsonIetfScalarQuotingIntSpans{
		Top: JsonIetfScalarQuotingIntSpansTop{
			I8:  ptr(int8(127)),
			I16: ptr(int16(32767)),
			I32: ptr(int32(2147483647)),
			I64: ptr(int64(9223372036854775807)),
			U8:  ptr(uint8(255)),
			U16: ptr(uint16(65535)),
			U32: ptr(uint32(4294967295)),
			U64: ptr(uint64(18446744073709551615)),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoJSONNestedContainerObjectMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "json-ietf-nested-container-object"), "module"), "json-ietf-nested-container-object")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "json-ietf-nested-container-object")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "json-ietf-nested-container-object", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "json-ietf-nested-container-object", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedJSONNestedContainerObjectMatchesLibyang(t *testing.T) {
	demo := JsonIetfNestedContainerObject{
		Top: JsonIetfNestedContainerObjectTop{
			Middle: JsonIetfNestedContainerObjectTopMiddle{
				Deep: JsonIetfNestedContainerObjectTopMiddleDeep{
					Value: ptr("bottom"),
				},
			},
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoJSONObjectDeterminismMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "json-object-determinism"), "module"), "json-object-determinism")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "json-object-determinism")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "JsonObjectDeterminismTopFieldOrder = []string{\"zebra\", \"alpha\", \"mid\", \"middle\"}") {
		t.Fatalf("generated source should keep JSON object members in schema order, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "json-object-determinism", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "json-object-determinism", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedJSONObjectDeterminismMatchesLibyang(t *testing.T) {
	demo := JsonObjectDeterminism{
		Top: JsonObjectDeterminismTop{
			Zebra:  ptr("z"),
			Alpha:  ptr("a"),
			Mid:    JsonObjectDeterminismTopMid{Nine: ptr(uint8(9)), One: ptr(uint8(1))},
			Middle: ptr("m"),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoJSONLeafrefUnionResolvedFormMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "json-ietf-leafref-union-resolved-form"), "module"), "json-ietf-leafref-union-resolved-form")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "json-ietf-leafref-union-resolved-form")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "type JsonIetfLeafrefUnionResolvedFormTopValueUnion interface") {
		t.Fatalf("generated source should emit typed union for JSON resolved form fixture, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "json-ietf-leafref-union-resolved-form", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "json-ietf-leafref-union-resolved-form", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedJSONLeafrefUnionResolvedFormMatchesLibyang(t *testing.T) {
	demo := JsonIetfLeafrefUnionResolvedForm{
		Top: JsonIetfLeafrefUnionResolvedFormTop{
			Iface: []JsonIetfLeafrefUnionResolvedFormTopIfaceEntry{{
				Name: "eth0",
			}},
			PrimaryIface: ptr("eth0"),
			Value:        JsonIetfLeafrefUnionResolvedFormTopValueUnionInt64(9223372036854775807),
			SelectedValue: JsonIetfLeafrefUnionResolvedFormTopSelectedValueUnionInt64(
				9223372036854775807,
			),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF leafref-to-union JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed leafref-to-union JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoJSONInstanceIdentifierStringMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "json-ietf-instance-identifier-string"), "module"), "json-ietf-instance-identifier-string")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "json-ietf-instance-identifier-string")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "ActiveDevice *InstanceIdentifier") {
		t.Fatalf("generated source should use InstanceIdentifier helper, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "json-ietf-instance-identifier-string", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "json-ietf-instance-identifier-string", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedJSONInstanceIdentifierStringMatchesLibyang(t *testing.T) {
	active := NewInstanceIdentifierWithXMLNS(
		"/jiiis:top/jiiis:device[jiiis:name='eth0']",
		"/json-ietf-instance-identifier-string:top/device[name='eth0']",
		"jiiis",
		"urn:json-ietf-instance-identifier-string",
	)
	demo := JsonIetfInstanceIdentifierString{
		Top: JsonIetfInstanceIdentifierStringTop{
			Device: []JsonIetfInstanceIdentifierStringTopDeviceEntry{{
				Name: "eth0",
			}},
			ActiveDevice: &active,
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoAnydataUntypedContainerMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "anydata-untyped-container"), "module"), "anydata-untyped-container")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "anydata-untyped-container")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "Metrics *AnyData") {
		t.Fatalf("generated source should emit AnyData fields, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "anydata-untyped-container", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "anydata-untyped-container", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedAnydataUntypedContainerMatchesLibyang(t *testing.T) {
	metrics := NewAnyData(
		"<custom>value</custom>",
		"{\n  \"custom\": \"value\"\n}",
	)
	demo := AnydataUntypedContainer{
		Top: AnydataUntypedContainerTop{
			Metrics: &metrics,
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoTopLevelAnydataMatchesLibyang(t *testing.T) {
	const source = `module anydata-top-level-codegen {
    yang-version 1.1;
    namespace "urn:anydata-top-level-codegen";
    prefix atlc;

    anydata payload;
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	src, err := codegen.GenerateGo(ctx, "anydata-top-level-codegen")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "Payload *AnyData") {
		t.Fatalf("generated source should emit top-level AnyData field, got:\n%s", src)
	}

	testBody := `
func TestGeneratedTopLevelAnydataMatchesLibyang(t *testing.T) {
	payload := NewAnyData(
		"<custom>value</custom>",
		"{\n  \"custom\": \"value\"\n}",
	)
	demo := AnydataTopLevelCodegen{Payload: &payload}
	if got, want := demo.ToXML(), "<payload xmlns=\"urn:anydata-top-level-codegen\">\n  <custom>value</custom>\n</payload>\n"; got != want {
		t.Fatalf("XML mismatch:\n got: %q\nwant: %q", got, want)
	}
	if got, want := demo.ToJSONIETF(), "{\n  \"anydata-top-level-codegen:payload\": {\n    \"custom\": \"value\"\n  }\n}\n"; got != want {
		t.Fatalf("JSON mismatch:\n got: %q\nwant: %q", got, want)
	}
}
`

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoAnyxmlOpaquePassthroughMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "anyxml-opaque-passthrough"), "module"), "anyxml-opaque-passthrough")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "anyxml-opaque-passthrough")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "Data *AnyData") {
		t.Fatalf("generated source should emit AnyData field for anyxml, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "anyxml-opaque-passthrough", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "anyxml-opaque-passthrough", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedAnyxmlOpaquePassthroughMatchesLibyang(t *testing.T) {
	data := NewAnyData(
		"<foo>\n  <bar>x</bar>\n</foo>",
		"{\n  \"foo\": {\n    \"bar\": \"x\"\n  }\n}",
	)
	demo := AnyxmlOpaquePassthrough{
		RpcReply: AnyxmlOpaquePassthroughRpcReply{
			Data: &data,
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoAnyxmlAttributesNamespacedXMLMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "anyxml-attributes-namespaced"), "module"), "anyxml-attributes-namespaced")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "anyxml-attributes-namespaced")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "Payload *AnyData") {
		t.Fatalf("generated source should emit AnyData field for namespaced anyxml, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "anyxml-attributes-namespaced", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedAnyxmlAttributesNamespacedXMLMatchesLibyang(t *testing.T) {
	payload := NewAnyData(
		"<foo xmlns=\"urn:x\" attr=\"1\">\n  <bar xmlns=\"urn:anyxml-attributes-namespaced\">v</bar>\n</foo>",
		"",
	)
	demo := AnyxmlAttributesNamespaced{
		Top: AnyxmlAttributesNamespacedTop{
			Payload: &payload,
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
}
`, string(wantXML))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoContainerPresenceEmptyMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "container-presence-empty"), "module"), "container-presence-empty")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "container-presence-empty")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "EnableSsh *ContainerPresenceEmptyEnableSsh") {
		t.Fatalf("generated source should represent presence container as optional struct, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "container-presence-empty", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "container-presence-empty", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedContainerPresenceEmptyMatchesLibyang(t *testing.T) {
	demo := ContainerPresenceEmpty{EnableSsh: &ContainerPresenceEmptyEnableSsh{}}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoDeclarationOrderOutOfAlphabeticalMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "declaration-order-out-of-alphabetical"), "module"), "declaration-order-out-of-alphabetical")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "declaration-order-out-of-alphabetical")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "DeclarationOrderOutOfAlphabeticalSystemFieldOrder = []string{\"zebra\", \"apple\", \"mango\", \"banana\"}") {
		t.Fatalf("generated source should preserve non-alphabetical declaration order, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "declaration-order-out-of-alphabetical", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "declaration-order-out-of-alphabetical", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedDeclarationOrderOutOfAlphabeticalMatchesLibyang(t *testing.T) {
	demo := DeclarationOrderOutOfAlphabetical{
		System: DeclarationOrderOutOfAlphabeticalSystem{
			Zebra:  ptr("4"),
			Apple:  ptr("1"),
			Mango:  ptr("3"),
			Banana: ptr("2"),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoConfigFalseStateSubtreeMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "config-false-state-subtree"), "module"), "config-false-state-subtree")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "config-false-state-subtree")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "Uptime *uint64") {
		t.Fatalf("generated source should preserve state uint64 leaf type, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "config-false-state-subtree", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "config-false-state-subtree", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedConfigFalseStateSubtreeMatchesLibyang(t *testing.T) {
	demo := ConfigFalseStateSubtree{
		System: ConfigFalseStateSubtreeSystem{
			Uptime:  ptr(uint64(123456)),
			LoadAvg: ptr("0.42"),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoMetadataYangVersionUnitsMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "metadata-yang-version-units"), "module"), "metadata-yang-version-units")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "metadata-yang-version-units")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "MemoryFree *uint64") ||
		!strings.Contains(src, "ResetMetrics *MetadataYangVersionUnitsPerformanceResetMetrics") {
		t.Fatalf("generated source should keep metadata leaves and expose nested action as opt-in operation field, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "metadata-yang-version-units", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "metadata-yang-version-units", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedMetadataYangVersionUnitsMatchesLibyang(t *testing.T) {
	demo := MetadataYangVersionUnits{
		Performance: MetadataYangVersionUnitsPerformance{
			CpuUsage:   ptr(NewDecimal64(4550, 2)),
			MemoryFree: ptr(uint64(8589934592)),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoWideHeterogeneousSiblingsMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "wide-heterogeneous-siblings-all-types"), "module"), "wide-heterogeneous-siblings-all-types")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "wide-heterogeneous-siblings-all-types")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "WideHeterogeneousSiblingsAllTypesPlatformFieldOrder = []string{\"hardware\", \"name\", \"software\", \"dns-servers\", \"services\", \"static-route\", \"ospf-area\", \"timezone\", \"state\", \"features\"}") {
		t.Fatalf("generated source should preserve wide heterogeneous schema order with flattened choice, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "wide-heterogeneous-siblings-all-types", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "wide-heterogeneous-siblings-all-types", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedWideHeterogeneousSiblingsMatchesLibyang(t *testing.T) {
	demo := WideHeterogeneousSiblingsAllTypes{
		Platform: WideHeterogeneousSiblingsAllTypesPlatform{
			Hardware: WideHeterogeneousSiblingsAllTypesPlatformHardware{Sku: ptr("ABC")},
			Name:     ptr("r1"),
			Software: WideHeterogeneousSiblingsAllTypesPlatformSoftware{Version: ptr("1.0")},
			DnsServers: []string{"8.8.8.8"},
			Services:   WideHeterogeneousSiblingsAllTypesPlatformServices{Enabled: ptr(true)},
			StaticRoute: ptr("0.0.0.0/0"),
			Timezone:    ptr("UTC"),
			State:       WideHeterogeneousSiblingsAllTypesPlatformState{Uptime: ptr(uint32(1234))},
			Features:    []string{"vlan", "mpls"},
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoConfigTrueSubtreeMatchesLibyang(t *testing.T) {
	src := generatedFixtureSource(t, "config-true-subtree", "config-true-subtree")
	wantXML, wantJSON := readFixtureGoldenPair(t, "config-true-subtree")

	testBody := fmt.Sprintf(`
func TestGeneratedConfigTrueSubtreeMatchesLibyang(t *testing.T) {
	demo := ConfigTrueSubtree{
		Settings: ConfigTrueSubtreeSettings{
			Hostname:   ptr("core"),
			DebugLevel: ptr(uint8(3)),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, wantXML, wantJSON)

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoContainerNestedDepthMatchesLibyang(t *testing.T) {
	src := generatedFixtureSource(t, "container-nested-depth", "container-nested-depth")
	wantXML, wantJSON := readFixtureGoldenPair(t, "container-nested-depth")

	testBody := fmt.Sprintf(`
func TestGeneratedContainerNestedDepthMatchesLibyang(t *testing.T) {
	demo := ContainerNestedDepth{
		Level1: ContainerNestedDepthLevel1{
			A: ptr("top"),
			Level2: ContainerNestedDepthLevel1Level2{
				B: ptr("middle"),
				Level3: ContainerNestedDepthLevel1Level2Level3{
					C: ptr("bottom"),
				},
			},
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, wantXML, wantJSON)

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoContainerWithinListSchemaOrderMatchesLibyang(t *testing.T) {
	src := generatedFixtureSource(t, "container-within-list-schema-order", "container-within-list-schema-order")
	wantXML, wantJSON := readFixtureGoldenPair(t, "container-within-list-schema-order")

	testBody := fmt.Sprintf(`
func TestGeneratedContainerWithinListSchemaOrderMatchesLibyang(t *testing.T) {
	demo := ContainerWithinListSchemaOrder{
		Device: []ContainerWithinListSchemaOrderDeviceEntry{{
			Id: "d1",
			Config: ContainerWithinListSchemaOrderDeviceEntryConfig{
				Enabled: ptr(true),
			},
		}},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, wantXML, wantJSON)

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoSingleKeyListFixturesMatchLibyang(t *testing.T) {
	tests := []struct {
		name     string
		fixture  string
		module   string
		testBody func(wantXML, wantJSON string) string
	}{
		{
			name:    "numeric",
			fixture: "list-single-key-numeric",
			module:  "list-single-key-numeric",
			testBody: func(wantXML, wantJSON string) string {
				return fmt.Sprintf(`
func TestGeneratedListSingleKeyNumericMatchesLibyang(t *testing.T) {
	demo := ListSingleKeyNumeric{
		Vlan: []ListSingleKeyNumericVlanEntry{
			{VlanId: 100, Description: ptr("mgmt")},
			{VlanId: 200, Description: ptr("prod")},
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, wantXML, wantJSON)
			},
		},
		{
			name:    "string",
			fixture: "list-single-key-string",
			module:  "list-single-key-string",
			testBody: func(wantXML, wantJSON string) string {
				return fmt.Sprintf(`
func TestGeneratedListSingleKeyStringMatchesLibyang(t *testing.T) {
	demo := ListSingleKeyString{
		Interface_: []ListSingleKeyStringInterfaceEntry{
			{Name: "eth0", Mtu: ptr(uint16(1500))},
			{Name: "eth1", Mtu: ptr(uint16(9000))},
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF list JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed list JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
}
`, wantXML, wantJSON)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := generatedFixtureSource(t, tt.fixture, tt.module)
			wantXML, wantJSON := readFixtureGoldenPair(t, tt.fixture)
			runGeneratedGoTest(t, src, tt.testBody(wantXML, wantJSON))
		})
	}
}

func TestGeneratedGoCompositeKeyListFixturesMatchLibyang(t *testing.T) {
	tests := []struct {
		name     string
		fixture  string
		module   string
		testBody func(wantXML, wantJSON string) string
	}{
		{
			name:    "two-key",
			fixture: "list-composite-key-two",
			module:  "list-composite-key-two",
			testBody: func(wantXML, wantJSON string) string {
				return fmt.Sprintf(`
func TestGeneratedListCompositeKeyTwoMatchesLibyang(t *testing.T) {
	demo := ListCompositeKeyTwo{
		Edge: []ListCompositeKeyTwoEdgeEntry{{
			SrcIp:  "10.0.0.1",
			DstIp:  "10.0.0.2",
			Metric: ptr(uint32(10)),
		}},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, wantXML, wantJSON)
			},
		},
		{
			name:    "three-key",
			fixture: "list-composite-key-three",
			module:  "list-composite-key-three",
			testBody: func(wantXML, wantJSON string) string {
				return fmt.Sprintf(`
func TestGeneratedListCompositeKeyThreeMatchesLibyang(t *testing.T) {
	demo := ListCompositeKeyThree{
		Route: []ListCompositeKeyThreeRouteEntry{{
			Prefix:    "192.0.2.0/24",
			NexthopIp: "198.51.100.1",
			Afi:       "ipv4",
			Preference: ptr(uint8(5)),
			Metric:     ptr(uint32(100)),
		}},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, wantXML, wantJSON)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := generatedFixtureSource(t, tt.fixture, tt.module)
			wantXML, wantJSON := readFixtureGoldenPair(t, tt.fixture)
			runGeneratedGoTest(t, src, tt.testBody(wantXML, wantJSON))
		})
	}
}

func TestGeneratedGoMixedConfigStateNestedMatchesLibyang(t *testing.T) {
	src := generatedFixtureSource(t, "mixed-config-state-nested", "mixed-config-state-nested")
	wantXML, wantJSON := readFixtureGoldenPair(t, "mixed-config-state-nested")

	testBody := fmt.Sprintf(`
func TestGeneratedMixedConfigStateNestedMatchesLibyang(t *testing.T) {
	demo := MixedConfigStateNested{
		System: MixedConfigStateNestedSystem{
			Hostname: ptr("core"),
			State: MixedConfigStateNestedSystemState{
				Uptime: ptr(uint64(123456)),
			},
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, wantXML, wantJSON)

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoOrderedUserFixturesMatchLibyang(t *testing.T) {
	tests := []struct {
		name     string
		fixture  string
		module   string
		testBody func(wantXML, wantJSON string) string
	}{
		{
			name:    "config",
			fixture: "ordered-user",
			module:  "ordered-user-demo",
			testBody: func(wantXML, wantJSON string) string {
				return fmt.Sprintf(`
func TestGeneratedOrderedUserMatchesLibyang(t *testing.T) {
	demo := OrderedUserDemo{
		Config: OrderedUserDemoConfig{
			Entry: NewUserOrderedVec([]OrderedUserDemoConfigEntryEntry{
				{Name: "c", Value: ptr("3")},
				{Name: "a", Value: ptr("1")},
				{Name: "b", Value: ptr("2")},
			}),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, wantXML, wantJSON)
			},
		},
		{
			name:    "config-false-state",
			fixture: "ordered-user-config-false-state",
			module:  "ordered-user-config-false-state",
			testBody: func(wantXML, wantJSON string) string {
				return fmt.Sprintf(`
func TestGeneratedOrderedUserConfigFalseStateMatchesLibyang(t *testing.T) {
	demo := OrderedUserConfigFalseState{
		State: OrderedUserConfigFalseStateState{
			Event: []OrderedUserConfigFalseStateStateEventEntry{
				{Seq: ptr(uint32(3)), Msg: ptr("first")},
				{Seq: ptr(uint32(1)), Msg: ptr("second")},
				{Seq: ptr(uint32(2)), Msg: ptr("third")},
			},
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, wantXML, wantJSON)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := generatedFixtureSource(t, tt.fixture, tt.module)
			if tt.fixture == "ordered-user" && !strings.Contains(src, "Entry UserOrderedVec[OrderedUserDemoConfigEntryEntry]") {
				t.Fatalf("ordered-by user list should generate UserOrderedVec, got:\n%s", src)
			}
			wantXML, wantJSON := readFixtureGoldenPair(t, tt.fixture)
			runGeneratedGoTest(t, src, tt.testBody(wantXML, wantJSON))
		})
	}
}

func TestGeneratedGoStatusCurrentDeprecatedObsoleteMatchesLibyang(t *testing.T) {
	src := generatedFixtureSource(t, "status-current-deprecated-obsolete", "status-current-deprecated-obsolete")
	wantXML, wantJSON := readFixtureGoldenPair(t, "status-current-deprecated-obsolete")

	testBody := fmt.Sprintf(`
func TestGeneratedStatusCurrentDeprecatedObsoleteMatchesLibyang(t *testing.T) {
	demo := StatusCurrentDeprecatedObsolete{
		CurrentItem:    ptr("a"),
		DeprecatedItem: ptr("b"),
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, wantXML, wantJSON)

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoJSONChoiceCaseTransparencyMatchesLibyang(t *testing.T) {
	src := generatedFixtureSource(t, "json-ietf-choice-case-transparency", "json-ietf-choice-case-transparency")
	wantXML, wantJSON := readFixtureGoldenPair(t, "json-ietf-choice-case-transparency")

	testBody := fmt.Sprintf(`
func TestGeneratedJSONChoiceCaseTransparencyMatchesLibyang(t *testing.T) {
	demo := JsonIetfChoiceCaseTransparency{
		Top: JsonIetfChoiceCaseTransparencyTop{
			Priority:   ptr(uint8(7)),
			Reason:     ptr("policy"),
			LocalLevel: ptr(uint8(3)),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, wantXML, wantJSON)

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoAnydataAnyxmlRepresentationMatchesLibyang(t *testing.T) {
	src := generatedFixtureSource(t, "json-ietf-anydata-anyxml-representation", "json-ietf-anydata-anyxml-representation")
	if !strings.Contains(src, "Metadata *AnyData") || !strings.Contains(src, "Data *AnyData") {
		t.Fatalf("generated source should emit raw AnyData helpers for anydata and anyxml, got:\n%s", src)
	}
	wantXML, wantJSON := readFixtureGoldenPair(t, "json-ietf-anydata-anyxml-representation")

	testBody := fmt.Sprintf(`
func TestGeneratedAnydataAnyxmlRepresentationMatchesLibyang(t *testing.T) {
	metadata := NewAnyData(
		"<custom>1</custom>\n<custom>2</custom>\n<custom>\n  <nested>true</nested>\n</custom>",
		"{\n  \"custom\": [\n    1,\n    2,\n    ,{\n      \"nested\": true\n    }\n  ]\n}",
	)
	data := NewAnyData(
		"<foo>\n  <bar>test</bar>\n</foo>",
		"{\n  \"foo\": {\n    \"bar\": \"test\"\n  }\n}",
	)
	demo := JsonIetfAnydataAnyxmlRepresentation{
		Top: JsonIetfAnydataAnyxmlRepresentationTop{
			Config: JsonIetfAnydataAnyxmlRepresentationTopConfig{
				Metadata: &metadata,
			},
			Payload: JsonIetfAnydataAnyxmlRepresentationTopPayload{
				Data: &data,
			},
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, wantXML, wantJSON)

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoRFC6991InetYangTypesRoundtripMatchesLibyang(t *testing.T) {
	src := generatedFixtureSource(t, "rfc6991-inet-yang-types-roundtrip", "rfc6991-inet-yang-types-roundtrip")
	if !strings.Contains(src, "type Rfc6991InetYangTypesRoundtripTopAddrUnion interface") {
		t.Fatalf("generated source should preserve imported typedef union surface, got:\n%s", src)
	}
	wantXML, wantJSON := readFixtureGoldenPair(t, "rfc6991-inet-yang-types-roundtrip")

	testBody := fmt.Sprintf(`
func TestGeneratedRFC6991InetYangTypesRoundtripMatchesLibyang(t *testing.T) {
	demo := Rfc6991InetYangTypesRoundtrip{
		Top: Rfc6991InetYangTypesRoundtripTop{
			Addr: Rfc6991InetYangTypesRoundtripTopAddrUnionIpv4Address("192.0.2.1"),
			Pfx:  ptr("2001:db8::/32"),
			Mac:  ptr("00:1b:44:11:3a:b7"),
			Ts:   ptr("2026-06-15T00:00:00Z"),
			Ctr:  ptr(uint64(18446744073709551615)),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, wantXML, wantJSON)

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoStringPatternFixturesMatchLibyang(t *testing.T) {
	tests := []struct {
		name     string
		fixture  string
		module   string
		testBody func(wantXML, wantJSON string) string
	}{
		{
			name:    "multiple-patterns",
			fixture: "types-string-multiple-patterns-conjunction",
			module:  "types-string-multiple-patterns-conjunction",
			testBody: func(wantXML, wantJSON string) string {
				return fmt.Sprintf(`
func TestGeneratedStringMultiplePatternsConjunctionMatchesLibyang(t *testing.T) {
	demo := TypesStringMultiplePatternsConjunction{Token: ptr("abc123")}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	invalid := TypesStringMultiplePatternsConjunction{Token: ptr("abc")}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate accepted string missing required digit pattern")
	} else if got, want := err.Error(), "/types-string-multiple-patterns-conjunction/token: pattern violation"; got != want {
		t.Fatalf("Validate error = %%q, want %%q", got, want)
	}
	if _, err := FromJSONIETF([]byte(`+"`"+`{"types-string-multiple-patterns-conjunction:token":"123"}`+"`"+`)); err == nil {
		t.Fatal("FromJSONIETF accepted string missing required letter pattern")
	} else if got, want := err.Error(), "types-string-multiple-patterns-conjunction:token pattern violation"; got != want {
		t.Fatalf("FromJSONIETF error = %%q, want %%q", got, want)
	}
}
`, wantXML, wantJSON)
			},
		},
		{
			name:    "invert-match",
			fixture: "types-string-pattern-modifier-invert-match",
			module:  "types-string-pattern-modifier-invert-match",
			testBody: func(wantXML, wantJSON string) string {
				return fmt.Sprintf(`
func TestGeneratedStringPatternModifierInvertMatchMatchesLibyang(t *testing.T) {
	demo := TypesStringPatternModifierInvertMatch{Name: ptr("ABC9")}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	invalid := TypesStringPatternModifierInvertMatch{Name: ptr("abc")}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate accepted inverted pattern match")
	} else if got, want := err.Error(), "/types-string-pattern-modifier-invert-match/name: pattern violation"; got != want {
		t.Fatalf("Validate error = %%q, want %%q", got, want)
	}
}
`, wantXML, wantJSON)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := generatedFixtureSource(t, tt.fixture, tt.module)
			wantXML, wantJSON := readFixtureGoldenPair(t, tt.fixture)
			runGeneratedGoTest(t, src, tt.testBody(wantXML, wantJSON))
		})
	}
}

func TestGeneratedGoExtensionFixturesMatchLibyang(t *testing.T) {
	tests := []struct {
		name     string
		fixture  string
		module   string
		testBody func(wantXML, wantJSON string) string
	}{
		{
			name:    "typedef-collision",
			fixture: "extension-and-typedef-collision",
			module:  "extension-and-typedef-collision",
			testBody: func(wantXML, wantJSON string) string {
				return fmt.Sprintf(`
func TestGeneratedExtensionAndTypedefCollisionMatchesLibyang(t *testing.T) {
	meta, err := NewExtensionAndTypedefCollisionTopMetaLength("data")
	if err != nil {
		t.Fatalf("new meta: %%v", err)
	}
	demo := ExtensionAndTypedefCollision{
		Top: ExtensionAndTypedefCollisionTop{Meta: &meta},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, wantXML, wantJSON)
			},
		},
		{
			name:    "definition-and-usage",
			fixture: "extension-definition-and-usage",
			module:  "extension-definition-and-usage",
			testBody: func(wantXML, wantJSON string) string {
				return fmt.Sprintf(`
func TestGeneratedExtensionDefinitionAndUsageMatchesLibyang(t *testing.T) {
	demo := ExtensionDefinitionAndUsage{
		System: ExtensionDefinitionAndUsageSystem{
			Hostname: ptr("r1"),
			Domain:   ptr("example.com"),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, wantXML, wantJSON)
			},
		},
		{
			name:    "yin-element-modes",
			fixture: "extension-yin-element-modes",
			module:  "extension-yin-element-modes",
			testBody: func(wantXML, wantJSON string) string {
				return fmt.Sprintf(`
func TestGeneratedExtensionYinElementModesMatchesLibyang(t *testing.T) {
	demo := ExtensionYinElementModes{
		Top: ExtensionYinElementModesTop{X: ptr("value")},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, wantXML, wantJSON)
			},
		},
		{
			name:    "vendor-junos",
			fixture: "vendor-extension-junos-passthrough",
			module:  "vendor-extension-junos-passthrough",
			testBody: func(wantXML, wantJSON string) string {
				return fmt.Sprintf(`
func TestGeneratedVendorExtensionJunosPassthroughMatchesLibyang(t *testing.T) {
	demo := VendorExtensionJunosPassthrough{
		Config: VendorExtensionJunosPassthroughConfig{
			Users: VendorExtensionJunosPassthroughConfigUsers{
				User: []VendorExtensionJunosPassthroughConfigUsersUserEntry{{
					Name:     "admin",
					Password: ptr("hunter2"),
				}},
			},
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, wantXML, wantJSON)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := generatedFixtureSource(t, tt.fixture, tt.module)
			wantXML, wantJSON := readFixtureGoldenPair(t, tt.fixture)
			runGeneratedGoTest(t, src, tt.testBody(wantXML, wantJSON))
		})
	}
}

func TestGeneratedGoOrderingNestedUserCascadingMatchesLibyang(t *testing.T) {
	src := generatedFixtureSource(t, "ordering-nested-user-cascading", "ordering-nested-user-cascading")
	if !strings.Contains(src, "Statement UserOrderedVec[OrderingNestedUserCascadingTopStatementEntry]") ||
		!strings.Contains(src, "Actions UserOrderedVec[string]") {
		t.Fatalf("generated source should keep user-ordering at both list levels, got:\n%s", src)
	}
	wantXML, wantJSON := readFixtureGoldenPair(t, "ordering-nested-user-cascading")

	testBody := fmt.Sprintf(`
func TestGeneratedOrderingNestedUserCascadingMatchesLibyang(t *testing.T) {
	demo := OrderingNestedUserCascading{
		Top: OrderingNestedUserCascadingTop{
			Statement: NewUserOrderedVec([]OrderingNestedUserCascadingTopStatementEntry{
				{Name: "s2", Actions: NewUserOrderedVec([]string{"b", "a"})},
				{Name: "s1", Actions: NewUserOrderedVec([]string{"b", "a"})},
			}),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, wantXML, wantJSON)

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoRPCDocumentFixturesMatchLibyang(t *testing.T) {
	tests := []struct {
		name     string
		fixture  string
		module   string
		testBody func(wantXML, wantJSON string) string
	}{
		{
			name:    "rpc-order",
			fixture: "rpc-order",
			module:  "rpc-order-demo",
			testBody: func(wantXML, wantJSON string) string {
				return fmt.Sprintf(`
func TestGeneratedRPCOrderMatchesLibyang(t *testing.T) {
	demo := RpcOrderDemoResetRPC{
		Z: ptr("z1"),
		M: ptr("m1"),
		A: ptr("a1"),
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromRpcOrderDemoResetRPCJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF RPC JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed RPC JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
}
`, wantXML, wantJSON)
			},
		},
		{
			name:    "input-only",
			fixture: "rpc-input-only",
			module:  "operations-rpc-input-only",
			testBody: func(wantXML, wantJSON string) string {
				return fmt.Sprintf(`
func TestGeneratedRPCInputOnlyMatchesLibyang(t *testing.T) {
	checkType := OperationsRpcInputOnlyRequestStatusRPCCheckTypeEnumIcmp
	demo := OperationsRpcInputOnlyRequestStatusRPC{
		InterfaceName: ptr("eth0"),
		CheckType:     &checkType,
		Timeout:       ptr(uint32(30)),
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromOperationsRpcInputOnlyRequestStatusRPCJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF RPC JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed RPC JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
}
`, wantXML, wantJSON)
			},
		},
		{
			name:    "output-only",
			fixture: "rpc-output-only",
			module:  "operations-rpc-output-only",
			testBody: func(wantXML, wantJSON string) string {
				return fmt.Sprintf(`
func TestGeneratedRPCOutputOnlyMatchesLibyang(t *testing.T) {
	demo := OperationsRpcOutputOnlyGetVersionRPC{
		VersionString: ptr("1.2.3"),
		BuildDate:     ptr("2026-06-15"),
		RevisionId:    ptr("r42"),
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromOperationsRpcOutputOnlyGetVersionRPCJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF RPC JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed RPC JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
}
`, wantXML, wantJSON)
			},
		},
		{
			name:    "input-output",
			fixture: "rpc-input-output-interleaved",
			module:  "operations-rpc-input-output-interleaved",
			testBody: func(wantXML, wantJSON string) string {
				return fmt.Sprintf(`
func TestGeneratedRPCInputOutputInterleavedMatchesLibyang(t *testing.T) {
	mode := OperationsRpcInputOutputInterleavedConfigureInterfaceRPCModeEnumAuto
	demo := OperationsRpcInputOutputInterleavedConfigureInterfaceRPC{
		Name: ptr("eth0"),
		Mode: &mode,
		Mtu:  ptr(uint32(1500)),
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromOperationsRpcInputOutputInterleavedConfigureInterfaceRPCJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF RPC JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed RPC JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
}
`, wantXML, wantJSON)
			},
		},
		{
			name:    "heterogeneous",
			fixture: "rpc-io-heterogeneous-nodes",
			module:  "operations-rpc-io-heterogeneous-nodes",
			testBody: func(wantXML, wantJSON string) string {
				return fmt.Sprintf(`
func TestGeneratedRPCIOHeterogeneousNodesMatchesLibyang(t *testing.T) {
	demo := OperationsRpcIoHeterogeneousNodesQueryMetricsRPC{
		QueryId: ptr("q1"),
		Metrics: []string{
			"cpu",
			"memory",
		},
		Filter: OperationsRpcIoHeterogeneousNodesQueryMetricsRPCFilter{
			Pattern: ptr(".*"),
		},
		NodeId: ptr("node-1"),
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromOperationsRpcIoHeterogeneousNodesQueryMetricsRPCJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF RPC JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed RPC JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
}
`, wantXML, wantJSON)
			},
		},
		{
			name:    "nested-containers",
			fixture: "rpc-io-nested-containers",
			module:  "operations-rpc-io-nested-containers",
			testBody: func(wantXML, wantJSON string) string {
				return fmt.Sprintf(`
func TestGeneratedRPCIONestedContainersMatchesLibyang(t *testing.T) {
	demo := OperationsRpcIoNestedContainersDeployServiceRPC{
		ServiceName: ptr("svc"),
		Config: OperationsRpcIoNestedContainersDeployServiceRPCConfig{
			ReplicaCount: ptr(uint16(3)),
			ResourceLimits: OperationsRpcIoNestedContainersDeployServiceRPCConfigResourceLimits{
				Cpu:    ptr("100m"),
				Memory: ptr("512M"),
			},
			Timeout: ptr(uint32(30)),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromOperationsRpcIoNestedContainersDeployServiceRPCJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF RPC JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed RPC JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
}
`, wantXML, wantJSON)
			},
		},
		{
			name:    "numeric-types",
			fixture: "rpc-io-decimal64-numeric-types",
			module:  "operations-rpc-io-decimal64-numeric-types",
			testBody: func(wantXML, wantJSON string) string {
				return fmt.Sprintf(`
func TestGeneratedRPCIODecimal64NumericTypesMatchesLibyang(t *testing.T) {
	demo := OperationsRpcIoDecimal64NumericTypesComputeAggregateRPC{
		SamplesCount: ptr(uint64(10)),
		Offset:       ptr(int64(-5)),
		ScaleFactor:  ptr(NewDecimal64(150, 2)),
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromOperationsRpcIoDecimal64NumericTypesComputeAggregateRPCJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF RPC JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed RPC JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
}
`, wantXML, wantJSON)
			},
		},
		{
			name:    "coexistence-rpc",
			fixture: "rpc-action-notification-coexistence",
			module:  "operations-rpc-action-notification-coexistence",
			testBody: func(wantXML, wantJSON string) string {
				return fmt.Sprintf(`
func TestGeneratedRPCActionNotificationCoexistenceRPCMatchesLibyang(t *testing.T) {
	demo := OperationsRpcActionNotificationCoexistenceGlobalOperationRPC{
		ParamA: ptr("a"),
		ParamB: ptr("b"),
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromOperationsRpcActionNotificationCoexistenceGlobalOperationRPCJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF RPC JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed RPC JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
}
`, wantXML, wantJSON)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := generatedFixtureSource(t, tt.fixture, tt.module)
			if !strings.Contains(src, "RPCFieldOrder") {
				t.Fatalf("generated source should emit RPC document field order, got:\n%s", src)
			}
			wantXML, wantJSON := readFixtureGoldenPair(t, tt.fixture)
			runGeneratedGoTest(t, src, tt.testBody(wantXML, wantJSON))
		})
	}
}

func TestGeneratedRPCInputLeafListAllowsDuplicatePayloadValues(t *testing.T) {
	const source = `module operations-rpc-leaflist-duplicates {
    namespace "urn:operations-rpc-leaflist-duplicates";
    prefix orld;
    yang-version 1.1;

    rpc collect {
        input {
            leaf-list tag {
                type string;
            }
        }
    }
}
`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "operations-rpc-leaflist-duplicates")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	testBody := `
func TestGeneratedRPCInputLeafListAllowsDuplicatePayloadValues(t *testing.T) {
	demo := OperationsRpcLeaflistDuplicatesCollectRPC{
		Tag: []string{"alpha", "alpha"},
	}
	if err := demo.Validate(); err != nil {
		t.Fatalf("Validate rejected duplicate RPC input leaf-list values: %v", err)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := "{\n  \"operations-rpc-leaflist-duplicates:collect\": {\n    \"tag\": [\n      \"alpha\",\n      \"alpha\"\n    ]\n  }\n}\n"
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %q\nwant: %q", gotJSON, wantJSON)
	}
	parsed, err := FromOperationsRpcLeaflistDuplicatesCollectRPCJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF rejected duplicate RPC input leaf-list values: %v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed JSON mismatch:\n got: %q\nwant: %q", got, wantJSON)
	}
}
`
	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedRPCWithInputAlsoEmitsOutputDocument(t *testing.T) {
	const source = `module operations-rpc-input-output-docs {
    namespace "urn:operations-rpc-input-output-docs";
    prefix oriod;
    yang-version 1.1;

    rpc configure-interface {
        input {
            leaf name {
                type string;
            }
        }
        output {
            leaf status {
                type enumeration {
                    enum ok;
                    enum fail;
                }
            }
            container stats {
                leaf packets-sent {
                    type uint64;
                }
            }
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "operations-rpc-input-output-docs")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	testBody := `
func TestGeneratedRPCWithInputAlsoEmitsOutputDocument(t *testing.T) {
	status := OperationsRpcInputOutputDocsConfigureInterfaceRPCOutputStatusEnumOk
	demo := OperationsRpcInputOutputDocsConfigureInterfaceRPCOutput{
		Status: &status,
		Stats: OperationsRpcInputOutputDocsConfigureInterfaceRPCOutputStats{
			PacketsSent: ptr(uint64(42)),
		},
	}
	gotXML := demo.ToXML()
	wantXML := "<configure-interface xmlns=\"urn:operations-rpc-input-output-docs\">\n  <status>ok</status>\n  <stats>\n    <packets-sent>42</packets-sent>\n  </stats>\n</configure-interface>\n"
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %q\nwant: %q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := "{\n  \"operations-rpc-input-output-docs:configure-interface\": {\n    \"status\": \"ok\",\n    \"stats\": {\n      \"packets-sent\": \"42\"\n    }\n  }\n}\n"
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %q\nwant: %q", gotJSON, wantJSON)
	}
	parsed, err := FromOperationsRpcInputOutputDocsConfigureInterfaceRPCOutputJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF output JSON: %v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed output JSON mismatch:\n got: %q\nwant: %q", got, wantJSON)
	}

	request := OperationsRpcInputOutputDocsConfigureInterfaceRPC{Name: ptr("eth0")}
	wantRequestJSON := "{\n  \"operations-rpc-input-output-docs:configure-interface\": {\n    \"name\": \"eth0\"\n  }\n}\n"
	if got := request.ToJSONIETF(); got != wantRequestJSON {
		t.Fatalf("request RPC document lost input payload:\n got: %q\nwant: %q", got, wantRequestJSON)
	}
}
`
	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoRPCWithAnyxmlXMLMatchesLibyang(t *testing.T) {
	src := generatedFixtureSource(t, "rpc-io-with-anyxml", "operations-rpc-io-with-anyxml")
	if !strings.Contains(src, "Parameters *AnyData") {
		t.Fatalf("generated source should emit raw AnyData for RPC anyxml, got:\n%s", src)
	}
	wantXML, err := os.ReadFile(goldenPath(t, "rpc-io-with-anyxml", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedRPCWithAnyxmlXMLMatchesLibyang(t *testing.T) {
	parameters := NewAnyData(
		"<args>\n  <arg>a</arg>\n  <arg>b</arg>\n</args>",
		"{}",
	)
	demo := OperationsRpcIoWithAnyxmlExecuteScriptRPC{
		ScriptName:  ptr("backup.sh"),
		Parameters:  &parameters,
		Timeout:     ptr(uint32(60)),
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
}
`, string(wantXML))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoNotificationDocumentFixturesMatchLibyang(t *testing.T) {
	tests := []struct {
		name     string
		fixture  string
		module   string
		testBody func(wantXML, wantJSON string) string
	}{
		{
			name:    "top-level",
			fixture: "notification-top-level",
			module:  "operations-notification-top-level",
			testBody: func(wantXML, wantJSON string) string {
				return fmt.Sprintf(`
func TestGeneratedNotificationTopLevelMatchesLibyang(t *testing.T) {
	linkState := OperationsNotificationTopLevelLinkStateChangeNotificationLinkStateEnumUp
	demo := OperationsNotificationTopLevelLinkStateChangeNotification{
		Timestamp:     ptr("2026-06-15T12:00:00Z"),
		InterfaceName: ptr("eth0"),
		LinkState:     &linkState,
		Reason:        ptr("port-up"),
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromOperationsNotificationTopLevelLinkStateChangeNotificationJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF notification JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed notification JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
}
`, wantXML, wantJSON)
			},
		},
		{
			name:    "container-leaflist",
			fixture: "notification-with-container-leaflist",
			module:  "operations-notification-with-container-leaflist",
			testBody: func(wantXML, wantJSON string) string {
				return fmt.Sprintf(`
func TestGeneratedNotificationWithContainerLeaflistMatchesLibyang(t *testing.T) {
	status := OperationsNotificationWithContainerLeaflistBackupCompleteNotificationSummaryStatusEnumSuccess
	demo := OperationsNotificationWithContainerLeaflistBackupCompleteNotification{
		BackupId: ptr("bk-1"),
		Summary: OperationsNotificationWithContainerLeaflistBackupCompleteNotificationSummary{
			Duration:    ptr("PT5M"),
			FilesBacked: ptr(uint64(42)),
			Status:      &status,
		},
		Timestamp: ptr("2026-06-15T12:00:00Z"),
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromOperationsNotificationWithContainerLeaflistBackupCompleteNotificationJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF notification JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed notification JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
}
`, wantXML, wantJSON)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := generatedFixtureSource(t, tt.fixture, tt.module)
			if !strings.Contains(src, "NotificationFieldOrder") {
				t.Fatalf("generated source should emit notification document field order, got:\n%s", src)
			}
			wantXML, wantJSON := readFixtureGoldenPair(t, tt.fixture)
			runGeneratedGoTest(t, src, tt.testBody(wantXML, wantJSON))
		})
	}
}

func TestGeneratedGoValidConstraintDataFixturesMatchLibyang(t *testing.T) {
	tests := []struct {
		name     string
		fixture  string
		module   string
		testBody func(wantXML, wantJSON string) string
	}{
		{
			name:    "default-types",
			fixture: "constraints-default-types",
			module:  "constraints-default-types",
			testBody: func(wantXML, wantJSON string) string {
				return fmt.Sprintf(`
func TestGeneratedConstraintsDefaultTypesMatchesLibyang(t *testing.T) {
	demo := ConstraintsDefaultTypes{
		Hostname:   ptr("localhost"),
		Port:       ptr(uint16(8080)),
		Enabled:    ptr(true),
		Nameserver: []string{"8.8.8.8", "8.8.4.4"},
		Facility:   ptr("local0"),
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF constraints JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed constraints JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
}
`, wantXML, wantJSON)
			},
		},
		{
			name:    "mandatory-interaction",
			fixture: "constraints-mandatory-interaction",
			module:  "constraints-mandatory-interaction",
			testBody: func(wantXML, wantJSON string) string {
				return fmt.Sprintf(`
func TestGeneratedConstraintsMandatoryInteractionMatchesLibyang(t *testing.T) {
	demo := ConstraintsMandatoryInteraction{
		Hostname: "router1",
		Domain:   ptr("example.com"),
		Mode:     ptr("operational"),
		Services: ConstraintsMandatoryInteractionServices{
			PrimaryServer: "10.0.0.1",
		},
		Auth: ConstraintsMandatoryInteractionAuth{
			Username: "admin",
			Password: ptr("unset"),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF constraints JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed constraints JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
}
`, wantXML, wantJSON)
			},
		},
		{
			name:    "must-multiple",
			fixture: "constraints-must-multiple",
			module:  "constraints-must-multiple",
			testBody: func(wantXML, wantJSON string) string {
				return fmt.Sprintf(`
func TestGeneratedConstraintsMustMultipleMatchesLibyang(t *testing.T) {
	mtu, err := NewConstraintsMustMultipleNetworkMtuRange(1500)
	if err != nil {
		t.Fatalf("new mtu: %%v", err)
	}
	demo := ConstraintsMustMultiple{
		Network: ConstraintsMustMultipleNetwork{
			Mtu:     &mtu,
			Payload: ptr(uint16(1000)),
			Qos:     ptr("best-effort"),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF constraints JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed constraints JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
}
`, wantXML, wantJSON)
			},
		},
		{
			name:    "when-must",
			fixture: "constraints-when-must",
			module:  "constraints-when-must",
			testBody: func(wantXML, wantJSON string) string {
				return fmt.Sprintf(`
func TestGeneratedConstraintsWhenMustMatchesLibyang(t *testing.T) {
	demo := ConstraintsWhenMust{
		Kind: ptr("a"),
		Detail: ConstraintsWhenMustDetail{
			Info: ptr("x"),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF constraints JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed constraints JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
}
`, wantXML, wantJSON)
			},
		},
		{
			name:    "when-xpath-functions",
			fixture: "constraints-when-xpath-functions",
			module:  "constraints-when-xpath-functions",
			testBody: func(wantXML, wantJSON string) string {
				return fmt.Sprintf(`
func TestGeneratedConstraintsWhenXpathFunctionsMatchesLibyang(t *testing.T) {
	serviceID := ConstraintsWhenXpathFunctionsServiceIdEnumNextGenFirewall
	demo := ConstraintsWhenXpathFunctions{
		ServiceId: &serviceID,
		FwRules: ConstraintsWhenXpathFunctionsFwRules{
			RuleCount: ptr(uint32(10)),
		},
		Hostname: ptr("prod-router-1"),
		RegexCfg: ConstraintsWhenXpathFunctionsRegexCfg{
			Region: ptr("us-east"),
		},
		Tags: []string{"red"},
		TagCfg: ConstraintsWhenXpathFunctionsTagCfg{
			TagCount: ptr(uint32(1)),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF constraints JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed constraints JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
}
`, wantXML, wantJSON)
			},
		},
		{
			name:    "unique-composite",
			fixture: "constraints-unique-composite",
			module:  "constraints-unique-composite",
			testBody: func(wantXML, wantJSON string) string {
				return fmt.Sprintf(`
func TestGeneratedConstraintsUniqueCompositeMatchesLibyang(t *testing.T) {
	demo := ConstraintsUniqueComposite{
		Policy: []ConstraintsUniqueCompositePolicyEntry{
			{
				Id:       1,
				Name:     ptr("allow"),
				Priority: ptr(uint8(10)),
				Rules: ConstraintsUniqueCompositePolicyEntryRules{
					Rule: []ConstraintsUniqueCompositePolicyEntryRulesRuleEntry{{
						RuleId: 100,
						SrcIp:  ptr("10.0.0.0/8"),
					}},
				},
			},
			{
				Id:       2,
				Name:     ptr("deny"),
				Priority: ptr(uint8(20)),
				Rules: ConstraintsUniqueCompositePolicyEntryRules{
					Rule: []ConstraintsUniqueCompositePolicyEntryRulesRuleEntry{{
						RuleId: 200,
						SrcIp:  ptr("192.168.0.0/16"),
					}},
				},
			},
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF constraints JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed constraints JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
}
`, wantXML, wantJSON)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := generatedFixtureSource(t, tt.fixture, tt.module)
			wantXML, wantJSON := readFixtureGoldenPair(t, tt.fixture)
			runGeneratedGoTest(t, src, tt.testBody(wantXML, wantJSON))
		})
	}
}

func TestGeneratedGoRangeLengthConstraintConstructorsMatchRejectFixture(t *testing.T) {
	src := generatedFixtureSource(t, "constraints-range-length-reject", "constraints-range-length-reject")
	wantXML, wantJSON := readFixtureGoldenPair(t, "constraints-range-length-reject")

	testBody := fmt.Sprintf(`
func TestGeneratedRangeLengthConstraintConstructorsMatchRejectFixture(t *testing.T) {
	n, err := NewConstraintsRangeLengthRejectTopNRange(42)
	if err != nil {
		t.Fatalf("new n range: %%v", err)
	}
	s, err := NewConstraintsRangeLengthRejectTopSLength("abc")
	if err != nil {
		t.Fatalf("new s length: %%v", err)
	}
	demo := ConstraintsRangeLengthReject{
		Top: ConstraintsRangeLengthRejectTop{
			N: &n,
			S: &s,
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	if _, err := NewConstraintsRangeLengthRejectTopNRange(0); err == nil {
		t.Fatal("NewConstraintsRangeLengthRejectTopNRange(0) succeeded, want error")
	}
	if _, err := NewConstraintsRangeLengthRejectTopNRange(101); err == nil {
		t.Fatal("NewConstraintsRangeLengthRejectTopNRange(101) succeeded, want error")
	}
	if _, err := NewConstraintsRangeLengthRejectTopSLength("a"); err == nil {
		t.Fatal("NewConstraintsRangeLengthRejectTopSLength(short) succeeded, want error")
	}
	if _, err := NewConstraintsRangeLengthRejectTopSLength("toolong"); err == nil {
		t.Fatal("NewConstraintsRangeLengthRejectTopSLength(long) succeeded, want error")
	}
}
`, wantXML, wantJSON)

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoMetadataAnnotationRFC7952MatchesLibyang(t *testing.T) {
	src := generatedFixtureSource(t, "metadata-annotation-rfc7952", "metadata-annotation-rfc7952")
	if !strings.Contains(src, "CambiumMetadata map[string][]MetadataAnnotation") {
		t.Fatalf("generated source should expose metadata annotations, got:\n%s", src)
	}
	wantXML, wantJSON := readFixtureGoldenPair(t, "metadata-annotation-rfc7952")

	testBody := fmt.Sprintf(`
func TestGeneratedMetadataAnnotationRFC7952MatchesLibyang(t *testing.T) {
	lastModified := NewMetadataAnnotation(
		"mar",
		MetadataAnnotationRfc7952ModuleNS,
		"metadata-annotation-rfc7952:last-modified",
		"2026-06-15T00:00:00Z",
	)
	demo := MetadataAnnotationRfc7952{
		Top: MetadataAnnotationRfc7952Top{
			Name: ptr("primary"),
			CambiumMetadata: map[string][]MetadataAnnotation{
				"name": {lastModified},
			},
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF metadata JSON: %%v", err)
	}
	if got := parsed.ToXML(); got != wantXML {
		t.Fatalf("parsed metadata XML mismatch:\n got: %%q\nwant: %%q", got, wantXML)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed metadata JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
	quotedMetadata := MetadataAnnotationRfc7952{
		Top: MetadataAnnotationRfc7952Top{
			Name: ptr("primary"),
			CambiumMetadata: map[string][]MetadataAnnotation{
				"name": {NewMetadataAnnotation("mar", MetadataAnnotationRfc7952ModuleNS, "metadata-annotation-rfc7952:last-modified", "owner \"primary\" & <active>")},
			},
		},
	}
	wantQuotedXML := "<top xmlns=\"urn:metadata-annotation-rfc7952\">\n  <name xmlns:mar=\"urn:metadata-annotation-rfc7952\" mar:last-modified=\"owner &quot;primary&quot; &amp; &lt;active&gt;\">primary</name>\n</top>\n"
	if got := quotedMetadata.ToXML(); got != wantQuotedXML {
		t.Fatalf("metadata XML attribute escaping mismatch:\n got: %%q\nwant: %%q", got, wantQuotedXML)
	}
	if _, err := FromJSONIETF([]byte(`+"`"+`{
  "metadata-annotation-rfc7952:top": {
    "@name": {
      "metadata-annotation-rfc7952:last-modified": "2026-06-15T00:00:00Z"
    }
  }
}`+"`"+`)); err == nil {
		t.Fatal("FromJSONIETF accepted metadata for an absent data node")
	}
	if _, err := FromJSONIETF([]byte(`+"`"+`{
  "metadata-annotation-rfc7952:top": {
    "name": "primary",
    "@name": {
      "unknown-module:last-modified": "2026-06-15T00:00:00Z"
    }
  }
}`+"`"+`)); err == nil {
		t.Fatal("FromJSONIETF accepted metadata with an unknown annotation module")
	}
	if _, err := FromJSONIETF([]byte(`+"`"+`{
  "metadata-annotation-rfc7952:top": {
    "name": "primary",
    "@name": {
      "last-modified": "2026-06-15T00:00:00Z"
    }
  }
}`+"`"+`)); err == nil {
		t.Fatal("FromJSONIETF accepted unqualified metadata annotation")
	}
	if _, err := FromJSONIETF([]byte(`+"`"+`{
  "metadata-annotation-rfc7952:top": {
    "name": "primary",
    "@name": {
      "metadata-annotation-rfc7952:missing-annotation": "2026-06-15T00:00:00Z"
    }
  }
}`+"`"+`)); err == nil {
		t.Fatal("FromJSONIETF accepted an undeclared metadata annotation")
	}
	missingNodeMetadata := MetadataAnnotationRfc7952{
		Top: MetadataAnnotationRfc7952Top{
			CambiumMetadata: map[string][]MetadataAnnotation{
				"name": {lastModified},
			},
		},
	}
	if err := missingNodeMetadata.Validate(); err == nil {
		t.Fatal("Validate accepted metadata for an absent data node")
	}
	unknownFieldMetadata := MetadataAnnotationRfc7952{
		Top: MetadataAnnotationRfc7952Top{
			Name: ptr("primary"),
			CambiumMetadata: map[string][]MetadataAnnotation{
				"missing": {lastModified},
			},
		},
	}
	if err := unknownFieldMetadata.Validate(); err == nil {
		t.Fatal("Validate accepted metadata for an unknown data node")
	}
	unknownAnnotation := MetadataAnnotationRfc7952{
		Top: MetadataAnnotationRfc7952Top{
			Name: ptr("primary"),
			CambiumMetadata: map[string][]MetadataAnnotation{
				"name": {NewMetadataAnnotation("bad", "", "unknown-module:last-modified", "x")},
			},
		},
	}
	if err := unknownAnnotation.Validate(); err == nil {
		t.Fatal("Validate accepted metadata with an unknown annotation module")
	}
	undeclaredAnnotation := MetadataAnnotationRfc7952{
		Top: MetadataAnnotationRfc7952Top{
			Name: ptr("primary"),
			CambiumMetadata: map[string][]MetadataAnnotation{
				"name": {NewMetadataAnnotation("mar", MetadataAnnotationRfc7952ModuleNS, "metadata-annotation-rfc7952:missing-annotation", "x")},
			},
		},
	}
	if err := undeclaredAnnotation.Validate(); err == nil {
		t.Fatal("Validate accepted an undeclared metadata annotation")
	}
	invalidPrefixAnnotation := MetadataAnnotationRfc7952{
		Top: MetadataAnnotationRfc7952Top{
			Name: ptr("primary"),
			CambiumMetadata: map[string][]MetadataAnnotation{
				"name": {NewMetadataAnnotation("bad prefix", MetadataAnnotationRfc7952ModuleNS, "metadata-annotation-rfc7952:last-modified", "x")},
			},
		},
	}
	if err := invalidPrefixAnnotation.Validate(); err == nil {
		t.Fatal("Validate accepted metadata annotation with malformed XML namespace prefix")
	}
	if got, want := invalidPrefixAnnotation.ToXML(), "<top xmlns=\"urn:metadata-annotation-rfc7952\">\n  <name>primary</name>\n</top>\n"; got != want {
		t.Fatalf("ToXML emitted unsafe metadata annotation:\n got: %%q\nwant: %%q", got, want)
	}
	invalidValueAnnotation := MetadataAnnotationRfc7952{
		Top: MetadataAnnotationRfc7952Top{
			Name: ptr("primary"),
			CambiumMetadata: map[string][]MetadataAnnotation{
				"name": {NewMetadataAnnotation("mar", MetadataAnnotationRfc7952ModuleNS, "metadata-annotation-rfc7952:last-modified", "bad\x01value")},
			},
		},
	}
	if err := invalidValueAnnotation.Validate(); err == nil {
		t.Fatal("Validate accepted metadata annotation with invalid value character")
	} else if got, want := err.Error(), "/metadata-annotation-rfc7952/top/name: invalid string character U+0001"; got != want {
		t.Fatalf("Validate metadata value error = %%q, want %%q", got, want)
	}
}
`, wantXML, wantJSON)

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoJSONIETFWithDefaultsModesMatchLibyang(t *testing.T) {
	src := generatedFixtureSource(t, "json-ietf-with-defaults-modes", "json-ietf-with-defaults-modes")
	goldenDir := filepath.Join(schemaFixtureDir(t, "json-ietf-with-defaults-modes"), "..", "..", "golden")
	readJSONIETF := func(name string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(goldenDir, name, "output.json_ietf"))
		if err != nil {
			t.Fatalf("read %s golden: %v", name, err)
		}
		return string(data)
	}
	wantExplicit := readJSONIETF("json-ietf-with-defaults-modes-explicit")
	wantTrim := readJSONIETF("json-ietf-with-defaults-modes-trim")
	wantAll := readJSONIETF("json-ietf-with-defaults-modes-all")
	wantAllTagged := readJSONIETF("json-ietf-with-defaults-modes-all-tagged")

	testBody := fmt.Sprintf(`
func TestGeneratedJSONIETFWithDefaultsModesMatchLibyang(t *testing.T) {
	explicit := JsonIetfWithDefaultsModes{
		Settings: JsonIetfWithDefaultsModesSettings{
			Timeout: ptr(uint32(30)),
			Retries: ptr(uint8(3)),
			Name:    ptr("primary"),
		},
	}
	if got, want := explicit.ToJSONIETFWithDefaults(WithDefaultsExplicit), %q; got != want {
		t.Fatalf("explicit JSON mismatch:\n got: %%q\nwant: %%q", got, want)
	}
	if got, want := explicit.ToJSONIETF(), %q; got != want {
		t.Fatalf("default ToJSONIETF JSON mismatch:\n got: %%q\nwant: %%q", got, want)
	}
	if got, want := explicit.ToJSONIETFWithDefaults(WithDefaultsTrim), %q; got != want {
		t.Fatalf("trim JSON mismatch:\n got: %%q\nwant: %%q", got, want)
	}

	reportAll := JsonIetfWithDefaultsModes{
		Settings: JsonIetfWithDefaultsModesSettings{
			Name: ptr("primary"),
		},
	}
	if got, want := reportAll.ToJSONIETFWithDefaults(WithDefaultsAll), %q; got != want {
		t.Fatalf("all JSON mismatch:\n got: %%q\nwant: %%q", got, want)
	}
	if got, want := reportAll.ToJSONIETFWithDefaults(WithDefaultsAllTagged), %q; got != want {
		t.Fatalf("all-tagged JSON mismatch:\n got: %%q\nwant: %%q", got, want)
	}
}
`, wantExplicit, wantExplicit, wantTrim, wantAll, wantAllTagged)

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoUnionDefaultJSONIETFResolvesMemberOrder(t *testing.T) {
	const source = `module union-default-codegen {
    namespace "urn:union-default-codegen";
    prefix udc;

    leaf mtu {
        type union {
            type uint16;
            type string;
        }
        default "5";
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	src, err := codegen.GenerateGo(ctx, "union-default-codegen")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	testBody := `
func TestGeneratedUnionDefaultJSONIETFResolvesMemberOrder(t *testing.T) {
	if got, want := (&UnionDefaultCodegen{}).ToJSONIETFWithDefaults(WithDefaultsAll), "{\n  \"union-default-codegen:mtu\": 5\n}\n"; got != want {
		t.Fatalf("all JSON mismatch:\n got: %q\nwant: %q", got, want)
	}

	numeric := UnionDefaultCodegenMtuUnionUint16(uint16(5))
	if got, want := (&UnionDefaultCodegen{Mtu: numeric}).ToJSONIETFWithDefaults(WithDefaultsTrim), "{\n}\n"; got != want {
		t.Fatalf("numeric trim JSON mismatch:\n got: %q\nwant: %q", got, want)
	}

	text := UnionDefaultCodegenMtuUnionString("5")
	if got, want := (&UnionDefaultCodegen{Mtu: text}).ToJSONIETFWithDefaults(WithDefaultsTrim), "{\n  \"union-default-codegen:mtu\": \"5\"\n}\n"; got != want {
		t.Fatalf("string trim JSON mismatch:\n got: %q\nwant: %q", got, want)
	}
}
`

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoScalarDefaultsUseCanonicalJSONIETFLiterals(t *testing.T) {
	const source = `module scalar-default-canonical-codegen {
    namespace "urn:scalar-default-canonical-codegen";
    prefix sdcc;

    leaf flags {
        type bits {
            bit read { position 0; }
            bit write { position 1; }
        }
        default "write read";
    }

    leaf ratio {
        type decimal64 {
            fraction-digits 2;
        }
        default "5.00";
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	src, err := codegen.GenerateGo(ctx, "scalar-default-canonical-codegen")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	testBody := `
func TestGeneratedScalarDefaultsUseCanonicalJSONIETFLiterals(t *testing.T) {
	if got, want := (&ScalarDefaultCanonicalCodegen{}).ToJSONIETFWithDefaults(WithDefaultsAll), "{\n  \"scalar-default-canonical-codegen:flags\": \"read write\",\n  \"scalar-default-canonical-codegen:ratio\": \"5.0\"\n}\n"; got != want {
		t.Fatalf("all JSON mismatch:\n got: %q\nwant: %q", got, want)
	}

	flags, err := NewScalarDefaultCanonicalCodegenFlagsBits([]string{"write", "read"})
	if err != nil {
		t.Fatalf("new flags: %v", err)
	}
	ratio := NewDecimal64(500, 2)
	if got, want := (&ScalarDefaultCanonicalCodegen{Flags: &flags, Ratio: &ratio}).ToJSONIETFWithDefaults(WithDefaultsTrim), "{\n}\n"; got != want {
		t.Fatalf("trim JSON mismatch:\n got: %q\nwant: %q", got, want)
	}
}
`

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoScalarDefaultsCoverBinaryEnumIdentityref(t *testing.T) {
	const source = `module scalar-default-more-codegen {
    namespace "urn:scalar-default-more-codegen";
    prefix sdmc;

    identity transport;
    identity tcp {
        base transport;
    }

    leaf blob {
        type binary {
            length "2..2";
        }
        default "AQI=";
    }

    leaf mode {
        type enumeration {
            enum beta;
            enum alpha;
        }
        default "alpha";
    }

    leaf proto {
        type identityref {
            base transport;
        }
        default "tcp";
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	src, err := codegen.GenerateGo(ctx, "scalar-default-more-codegen")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	testBody := `
func TestGeneratedScalarDefaultsCoverBinaryEnumIdentityref(t *testing.T) {
	if got, want := (&ScalarDefaultMoreCodegen{}).ToJSONIETFWithDefaults(WithDefaultsAll), "{\n  \"scalar-default-more-codegen:blob\": \"AQI=\",\n  \"scalar-default-more-codegen:mode\": \"alpha\",\n  \"scalar-default-more-codegen:proto\": \"tcp\"\n}\n"; got != want {
		t.Fatalf("all JSON mismatch:\n got: %q\nwant: %q", got, want)
	}

	mode := ScalarDefaultMoreCodegenModeEnumAlpha
	proto := ScalarDefaultMoreCodegenProtoEnumTcp
	explicit := ScalarDefaultMoreCodegen{
		Blob:  ptr("AQI="),
		Mode:  &mode,
		Proto: &proto,
	}
	if got, want := explicit.ToJSONIETFWithDefaults(WithDefaultsTrim), "{\n}\n"; got != want {
		t.Fatalf("trim JSON mismatch:\n got: %q\nwant: %q", got, want)
	}
}
`

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoSchemaRejectsInvalidBitsDefaultUnicodeWhitespace(t *testing.T) {
	source := `module invalid-bits-default-unicode-whitespace {
    namespace "urn:invalid-bits-default-unicode-whitespace";
    prefix ibduw;

    leaf flags {
        type bits {
            bit read { position 0; }
            bit write { position 1; }
        }
        default "read` + "\u2003" + `write";
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	if _, err := builder.Build(); err == nil {
		t.Fatal("Build accepted bits default separated by Unicode whitespace")
	} else if !strings.Contains(err.Error(), `default "read\u2003write" is not valid for bits leaf "flags"`) {
		t.Fatalf("Build error = %q, want invalid bits default", err)
	}
}

func TestGeneratedGoJSONIETFParseRoundtripDataMatchesLibyang(t *testing.T) {
	src := generatedFixtureSource(t, "json-ietf-parse-roundtrip", "json-ietf-parse-roundtrip")
	if !strings.Contains(src, "JsonIetfParseRoundtripTopKindEnumDerivedId") {
		t.Fatalf("generated source should expose identityref values for parse-roundtrip fixture, got:\n%s", src)
	}
	if !strings.Contains(src, "func FromJSONIETF(data []byte) (*JsonIetfParseRoundtrip, error)") {
		t.Fatalf("generated source should expose native JSON_IETF parser, got:\n%s", src)
	}
	wantXML, wantJSON := readFixtureGoldenPair(t, "json-ietf-parse-roundtrip")
	inputJSON, err := os.ReadFile(filepath.Join(schemaFixtureDir(t, "json-ietf-parse-roundtrip"), "input.json"))
	if err != nil {
		t.Fatalf("read input json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedJSONIETFParseRoundtripDataMatchesLibyang(t *testing.T) {
	demo, err := FromJSONIETF([]byte(%q))
	if err != nil {
		t.Fatalf("FromJSONIETF: %%v", err)
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	if _, err := FromJSONIETF([]byte(%q)); err == nil {
		t.Fatal("FromJSONIETF accepted unknown top-level JSON member")
	}
	if _, err := FromJSONIETF([]byte(%q)); err == nil {
		t.Fatal("FromJSONIETF accepted unknown nested JSON member")
	}
	if _, err := FromJSONIETF([]byte(%q)); err == nil {
		t.Fatal("FromJSONIETF accepted duplicate top-level JSON member")
	}
	if _, err := FromJSONIETF([]byte(%q)); err == nil {
		t.Fatal("FromJSONIETF accepted duplicate nested JSON member")
	}
	badUTF8 := append([]byte(`+"`"+`{"json-ietf-parse-roundtrip:top":{"tags":["`+"`"+`), 0xff)
	badUTF8 = append(badUTF8, []byte(`+"`"+`"]}}`+"`"+`)...)
	if _, err := FromJSONIETF(badUTF8); err == nil {
		t.Fatal("FromJSONIETF accepted invalid UTF-8 JSON input")
	}
	if _, err := FromJSONIETF([]byte(`+"`"+`{"json-ietf-parse-roundtrip:top":{"tags":["\u0001"]}}`+"`"+`)); err == nil {
		t.Fatal("FromJSONIETF accepted escaped invalid JSON string control character")
	}
		if _, err := FromJSONIETF([]byte(`+"`"+`{"json-ietf-parse-roundtrip:top":{"tags":["\uD800"]}}`+"`"+`)); err == nil {
			t.Fatal("FromJSONIETF accepted escaped JSON surrogate code point")
		}
	validPair, err := FromJSONIETF([]byte(`+"`"+`{"json-ietf-parse-roundtrip:top":{"tags":["\uD83D\uDE00"]}}`+"`"+`))
	if err != nil {
		t.Fatalf("FromJSONIETF rejected escaped JSON surrogate pair: %%v", err)
	}
	if got, want := validPair.ToJSONIETF(), "{\n  \"json-ietf-parse-roundtrip:top\": {\n    \"tags\": [\n      \"\U0001F600\"\n    ]\n  }\n}\n"; got != want {
		t.Fatalf("surrogate-pair JSON mismatch:\n got: %%q\nwant: %%q", got, want)
	}
		for i := 0; i < 200; i++ {
			_, err := FromJSONIETF([]byte(`+"`"+`{"json-ietf-parse-roundtrip:top":{"big":"1"},"json-ietf-parse-roundtrip:zzz":true,"json-ietf-parse-roundtrip:aaa":true}`+"`"+`))
			if err == nil {
				t.Fatal("FromJSONIETF accepted multiple unknown top-level JSON members")
			}
			if got, want := err.Error(), `+"`"+`unknown JSON_IETF field "json-ietf-parse-roundtrip:aaa"`+"`"+`; got != want {
				t.Fatalf("unknown top-level field error = %%q, want %%q", got, want)
			}
		}
		for i := 0; i < 200; i++ {
			_, err := FromJSONIETF([]byte(`+"`"+`{"json-ietf-parse-roundtrip:top":{"big":"1","zzz":true,"aaa":true}}`+"`"+`))
			if err == nil {
				t.Fatal("FromJSONIETF accepted multiple unknown nested JSON members")
			}
			if got, want := err.Error(), `+"`"+`unknown JSON_IETF field "aaa"`+"`"+`; got != want {
				t.Fatalf("unknown nested field error = %%q, want %%q", got, want)
			}
		}
	}
	`, string(inputJSON), wantXML, wantJSON,
		`{"json-ietf-parse-roundtrip:top":{"big":"1"},"json-ietf-parse-roundtrip:unknown":true}`,
		`{"json-ietf-parse-roundtrip:top":{"big":"1","unknown":true}}`,
		`{"json-ietf-parse-roundtrip:top":{"big":"1"},"json-ietf-parse-roundtrip:top":{"big":"2"}}`,
		`{"json-ietf-parse-roundtrip:top":{"big":"1","big":"2"}}`)

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoJSONParserHasNoUnsupportedScalarFallbacks(t *testing.T) {
	const unsupportedParser = "JSON_IETF parse for %s is not supported by generated parser"
	cases := []struct {
		fixture string
		module  string
	}{
		{"json-ietf-parse-roundtrip", "json-ietf-parse-roundtrip"},
		{"json-ietf-scalar-quoting-int-spans", "json-ietf-scalar-quoting-int-spans"},
		{"types-int-int8-range", "types-int-int8-range"},
		{"types-int-int64-range-quoted", "types-int-int64-range-quoted"},
		{"types-uint-uint64-range-quoted", "types-uint-uint64-range-quoted"},
		{"types-decimal64-fraction1-range", "types-decimal64-fraction1-range"},
		{"types-empty-leaf-null-json", "types-empty-leaf-null-json"},
		{"types-binary-length-base64", "types-binary-length-base64"},
		{"types-bits-explicit-positions-gaps", "types-bits-explicit-positions-gaps"},
		{"types-identityref-single-base", "types-identityref-single-base"},
		{"types-instance-identifier-complex-path", "types-instance-identifier-complex-path"},
		{"types-leafref-cross-module", "types-leafref-cross-module"},
		{"types-union-scalar-all-members", "types-union-scalar-all-members"},
		{"types-union-identityref-member", "types-union-identityref-member"},
	}
	for _, tt := range cases {
		t.Run(tt.fixture, func(t *testing.T) {
			src := generatedFixtureSource(t, tt.fixture, tt.module)
			if strings.Contains(src, unsupportedParser) {
				t.Fatalf("generated source contains unsupported JSON parser fallback for %s", tt.fixture)
			}
		})
	}
}

func TestGeneratedGoCrossModuleLeafrefDocumentMatchesLibyang(t *testing.T) {
	src := generatedFixtureSource(t, "types-leafref-cross-module", "types-leafref-cross-module")
	if !strings.Contains(src, "Interface_ []TypesLeafrefCrossModuleInterfaceEntry") {
		t.Fatalf("generated source should include imported module top-level document data, got:\n%s", src)
	}
	wantXML, wantJSON := readFixtureGoldenPair(t, "types-leafref-cross-module")

	testBody := fmt.Sprintf(`
func TestGeneratedCrossModuleLeafrefDocumentMatchesLibyang(t *testing.T) {
	demo := TypesLeafrefCrossModule{
		BoundIf: ptr("eth0"),
		Interface_: []TypesLeafrefCrossModuleInterfaceEntry{{
			Name: "eth0",
		}},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	if err := demo.Validate(); err != nil {
		t.Fatalf("Validate cross-module document: %%v", err)
	}
	parsed, err := FromJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF cross-module JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed cross-module JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
}
`, wantXML, wantJSON)

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoSubmoduleImportContributesDocumentFields(t *testing.T) {
	dir := t.TempDir()
	base := `module submodule-import-base {
  yang-version 1.1;
  namespace "urn:submodule-import-base";
  prefix sib;

  leaf target { type string; }
}
`
	user := `module submodule-import-user {
  yang-version 1.1;
  namespace "urn:submodule-import-user";
  prefix siu;

  include submodule-import-user-part;
}
`
	part := `submodule submodule-import-user-part {
  yang-version 1.1;
  belongs-to submodule-import-user {
    prefix siu;
  }

  import submodule-import-base {
    prefix base;
  }

  leaf bound {
    type leafref {
      path "/base:target";
    }
  }
}
`
	if err := os.WriteFile(filepath.Join(dir, "submodule-import-base.yang"), []byte(base), 0o644); err != nil {
		t.Fatalf("write base module: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "submodule-import-user.yang"), []byte(user), 0o644); err != nil {
		t.Fatalf("write user module: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "submodule-import-user-part.yang"), []byte(part), 0o644); err != nil {
		t.Fatalf("write submodule: %v", err)
	}

	ctx := loadModule(t, dir, "submodule-import-user")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "submodule-import-user")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, `SubmoduleImportUserFieldOrder = []string{"bound", "target"}`) {
		t.Fatalf("generated source should include submodule-imported top-level data, got:\n%s", src)
	}

	testBody := `
func TestGeneratedSubmoduleImportContributesDocumentFields(t *testing.T) {
	demo := SubmoduleImportUser{
		Bound: ptr("eth0"),
		Target: ptr("eth0"),
	}
	wantXML := "<bound xmlns=\"urn:submodule-import-user\">eth0</bound>\n<target xmlns=\"urn:submodule-import-base\">eth0</target>\n"
	if got := demo.ToXML(); got != wantXML {
		t.Fatalf("XML mismatch:\n got: %q\nwant: %q", got, wantXML)
	}
	wantJSON := "{\n  \"submodule-import-user:bound\": \"eth0\",\n  \"submodule-import-base:target\": \"eth0\"\n}\n"
	if got := demo.ToJSONIETF(); got != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %q\nwant: %q", got, wantJSON)
	}
	parsed, err := FromJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF: %v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed JSON mismatch:\n got: %q\nwant: %q", got, wantJSON)
	}
}
`
	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoImportedTopLevelChoiceValidate(t *testing.T) {
	dir := t.TempDir()
	base := `module root-choice-base {
  yang-version 1.1;
  namespace "urn:root-choice-base";
  prefix rcb;

  choice transport {
    case tcp {
      leaf tcp-port { type uint16; }
    }
    case udp {
      leaf udp-port { type uint16; }
    }
  }
}
`
	user := `module root-choice-user {
  yang-version 1.1;
  namespace "urn:root-choice-user";
  prefix rcu;

  import root-choice-base { prefix rcb; }

  leaf local { type string; }
}
`
	if err := os.WriteFile(filepath.Join(dir, "root-choice-base.yang"), []byte(base), 0o644); err != nil {
		t.Fatalf("write base module: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "root-choice-user.yang"), []byte(user), 0o644); err != nil {
		t.Fatalf("write user module: %v", err)
	}

	ctx := loadModule(t, dir, "root-choice-user")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "root-choice-user")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, `RootChoiceUserFieldOrder = []string{"local", "tcp-port", "udp-port"}`) {
		t.Fatalf("generated source should include selected fields before imported choice fields, got:\n%s", src)
	}
	if !strings.Contains(src, `choice has multiple cases selected`) {
		t.Fatalf("generated source should validate imported top-level choices, got:\n%s", src)
	}

	testBody := `
func TestGeneratedImportedTopLevelChoiceValidate(t *testing.T) {
	invalid := RootChoiceUser{
		Local: ptr("ok"),
		TcpPort: ptr(uint16(22)),
		UdpPort: ptr(uint16(53)),
	}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate accepted two imported top-level choice cases")
	}

	valid := RootChoiceUser{
		TcpPort: ptr(uint16(22)),
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate rejected one imported top-level choice case: %v", err)
	}
}
`

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoNotificationInterleavedDataMatchesLibyang(t *testing.T) {
	src := generatedFixtureSource(t, "notification-interleaved-siblings", "operations-notification-interleaved-siblings")
	if !strings.Contains(src, "Raised *OperationsNotificationInterleavedSiblingsAlarmsRaised") {
		t.Fatalf("generated source should expose nested notification as opt-in operation field, got:\n%s", src)
	}
	wantXML, wantJSON := readFixtureGoldenPair(t, "notification-interleaved-siblings")

	testBody := fmt.Sprintf(`
func TestGeneratedNotificationInterleavedDataMatchesLibyang(t *testing.T) {
	demo := OperationsNotificationInterleavedSiblings{
		Alarms: OperationsNotificationInterleavedSiblingsAlarms{
			Count:     ptr(uint32(5)),
			Threshold: ptr(uint32(10)),
			Summary: OperationsNotificationInterleavedSiblingsAlarmsSummary{
				Last: ptr("2026-06-15T12:00:00Z"),
			},
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, wantXML, wantJSON)

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoNestedActionFixturesMatchLibyang(t *testing.T) {
	tests := []struct {
		name     string
		fixture  string
		module   string
		testBody func(wantXML, wantJSON string) string
	}{
		{
			name:    "container-simple",
			fixture: "action-container-simple",
			module:  "operations-action-container-simple",
			testBody: func(wantXML, wantJSON string) string {
				return fmt.Sprintf(`
func TestGeneratedActionContainerSimpleMatchesLibyang(t *testing.T) {
	demo := OperationsActionContainerSimple{
		Device: OperationsActionContainerSimpleDevice{
			Name: ptr("d1"),
			Restart: &OperationsActionContainerSimpleDeviceRestart{
				DelaySeconds: ptr(uint32(5)),
				Force:        ptr(true),
			},
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF action JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed action JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
}
`, wantXML, wantJSON)
			},
		},
		{
			name:    "wide-siblings",
			fixture: "action-container-wide-siblings",
			module:  "operations-action-container-wide-siblings",
			testBody: func(wantXML, wantJSON string) string {
				return fmt.Sprintf(`
func TestGeneratedActionContainerWideSiblingsMatchesLibyang(t *testing.T) {
	demo := OperationsActionContainerWideSiblings{
		Config: OperationsActionContainerWideSiblingsConfig{
			SettingA: ptr("a"),
			SettingB: ptr("b"),
			SettingC: ptr("c"),
			SettingD: ptr("d"),
			Validate_: &OperationsActionContainerWideSiblingsConfigValidate{
				StrictMode: ptr(true),
			},
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF action JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed action JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
}
`, wantXML, wantJSON)
			},
		},
		{
			name:    "list-key-context",
			fixture: "action-list-keys-context",
			module:  "operations-action-list-keys-context",
			testBody: func(wantXML, wantJSON string) string {
				return fmt.Sprintf(`
func TestGeneratedActionListKeysContextMatchesLibyang(t *testing.T) {
	resetType := OperationsActionListKeysContextServiceEntryResetResetTypeEnumSoft
	demo := OperationsActionListKeysContext{
		Service: []OperationsActionListKeysContextServiceEntry{{
			Name: "svc1",
			Reset: &OperationsActionListKeysContextServiceEntryReset{
				ResetType:     &resetType,
				PreserveState: ptr(true),
			},
		}},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF action JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed action JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
}
`, wantXML, wantJSON)
			},
		},
		{
			name:    "nested-containers",
			fixture: "action-nested-containers",
			module:  "operations-action-nested-containers",
			testBody: func(wantXML, wantJSON string) string {
				return fmt.Sprintf(`
func TestGeneratedActionNestedContainersMatchesLibyang(t *testing.T) {
	format := OperationsActionNestedContainersSystemManagementAuditLogFormatEnumJson
	demo := OperationsActionNestedContainers{
		System: OperationsActionNestedContainersSystem{
			Management: OperationsActionNestedContainersSystemManagement{
				Enabled:       ptr(true),
				RetentionDays: ptr(uint16(30)),
				AuditLog: &OperationsActionNestedContainersSystemManagementAuditLog{
					DurationHours: ptr(uint16(24)),
					Format:        &format,
				},
			},
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF action JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed action JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
}
`, wantXML, wantJSON)
			},
		},
		{
			name:    "heterogeneous",
			fixture: "action-io-heterogeneous",
			module:  "operations-action-io-heterogeneous",
			testBody: func(wantXML, wantJSON string) string {
				return fmt.Sprintf(`
func TestGeneratedActionIOHeterogeneousMatchesLibyang(t *testing.T) {
	demo := OperationsActionIoHeterogeneous{
		Interface_: OperationsActionIoHeterogeneousInterface{
			Diagnostic: &OperationsActionIoHeterogeneousInterfaceDiagnostic{
				TestType:   ptr("ping"),
				Parameters: []string{"p1", "p2"},
				Config: OperationsActionIoHeterogeneousInterfaceDiagnosticConfig{
					Timeout: ptr(uint16(30)),
				},
			},
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF action JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed action JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
}
`, wantXML, wantJSON)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := generatedFixtureSource(t, tt.fixture, tt.module)
			wantXML, wantJSON := readFixtureGoldenPair(t, tt.fixture)
			runGeneratedGoTest(t, src, tt.testBody(wantXML, wantJSON))
		})
	}
}

func TestGeneratedGoNestedNotificationFixturesMatchLibyang(t *testing.T) {
	tests := []struct {
		name     string
		fixture  string
		module   string
		testBody func(wantXML, wantJSON string) string
	}{
		{
			name:    "container",
			fixture: "notification-nested-container",
			module:  "operations-notification-nested-container",
			testBody: func(wantXML, wantJSON string) string {
				return fmt.Sprintf(`
func TestGeneratedNotificationNestedContainerMatchesLibyang(t *testing.T) {
	severity := OperationsNotificationNestedContainerSystemErrorAlarmSeverityEnumCritical
	demo := OperationsNotificationNestedContainer{
		System: OperationsNotificationNestedContainerSystem{
			Hostname: ptr("core-1"),
			ErrorAlarm: &OperationsNotificationNestedContainerSystemErrorAlarm{
				Severity:    &severity,
				ErrorCode:   ptr(uint32(42)),
				Description: ptr("overheating"),
				Timestamp:   ptr("2026-06-15T12:00:00Z"),
			},
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, wantXML, wantJSON)
			},
		},
		{
			name:    "list",
			fixture: "notification-nested-list",
			module:  "operations-notification-nested-list",
			testBody: func(wantXML, wantJSON string) string {
				return fmt.Sprintf(`
func TestGeneratedNotificationNestedListMatchesLibyang(t *testing.T) {
	actionTaken := OperationsNotificationNestedListInterfaceEntryMtuExceededActionTakenEnumDrop
	demo := OperationsNotificationNestedList{
		Interface_: []OperationsNotificationNestedListInterfaceEntry{{
			Name: "eth0",
			Mtu:  ptr(uint32(1500)),
			MtuExceeded: &OperationsNotificationNestedListInterfaceEntryMtuExceeded{
				PacketSize:  ptr(uint32(2000)),
				ActionTaken: &actionTaken,
			},
		}},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, wantXML, wantJSON)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := generatedFixtureSource(t, tt.fixture, tt.module)
			wantXML, wantJSON := readFixtureGoldenPair(t, tt.fixture)
			runGeneratedGoTest(t, src, tt.testBody(wantXML, wantJSON))
		})
	}
}

func TestGeneratedStringPatternValidatorAvoidsMustCompile(t *testing.T) {
	src := generatedFixtureSource(t, "types-string-multiple-patterns-conjunction", "types-string-multiple-patterns-conjunction")
	if strings.Contains(src, "regexp.MustCompile") {
		t.Fatal("generated pattern validator must not panic on regexp compile failure")
	}
	if !strings.Contains(src, "regexp.Compile") {
		t.Fatal("generated pattern validator should compile patterns through the error-returning regexp API")
	}
}

func TestGeneratedGoStringPatternXSDUnicodeBlock(t *testing.T) {
	const source = `module pattern-xsd-unicode-block-codegen {
    namespace "urn:pattern-xsd-unicode-block-codegen";
    prefix pxubc;

    leaf value {
        type string {
            pattern "\\p{IsBasicLatin}+";
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	src, err := codegen.GenerateGo(ctx, "pattern-xsd-unicode-block-codegen")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, `expr: "\\p{IsBasicLatin}+"`) {
		t.Fatalf("generated source should preserve original XSD pattern text, got:\n%s", src)
	}

	testBody := `
func TestGeneratedStringPatternXSDUnicodeBlock(t *testing.T) {
	valid := PatternXsdUnicodeBlockCodegen{Value: ptr("ASCII")}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate rejected BasicLatin value: %v", err)
	}
	invalid := PatternXsdUnicodeBlockCodegen{Value: ptr("snowman ☃")}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate accepted non-BasicLatin value")
	} else if got, want := err.Error(), "/pattern-xsd-unicode-block-codegen/value: pattern violation"; got != want {
		t.Fatalf("Validate error = %q, want %q", got, want)
	}
	if _, err := FromJSONIETF([]byte(` + "`" + `{"pattern-xsd-unicode-block-codegen:value":"snowman ☃"}` + "`" + `)); err == nil {
		t.Fatal("FromJSONIETF accepted non-BasicLatin value")
	} else if got, want := err.Error(), "pattern-xsd-unicode-block-codegen:value pattern violation"; got != want {
		t.Fatalf("FromJSONIETF error = %q, want %q", got, want)
	}
}
`
	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoStringPatternXSDNonASCIIUnicodeBlock(t *testing.T) {
	const source = `module pattern-xsd-nonascii-unicode-block-codegen {
    namespace "urn:pattern-xsd-nonascii-unicode-block-codegen";
    prefix pxnubc;

    leaf value {
        type string {
            pattern "\\p{IsGreek}+";
        }
    }
    leaf non_greek {
        type string {
            pattern "\\P{IsGreek}+";
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	src, err := codegen.GenerateGo(ctx, "pattern-xsd-nonascii-unicode-block-codegen")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, `expr: "\\p{IsGreek}+"`) {
		t.Fatalf("generated source should preserve original XSD Greek pattern text, got:\n%s", src)
	}
	if !strings.Contains(src, `expr: "\\P{IsGreek}+"`) {
		t.Fatalf("generated source should preserve original XSD non-Greek pattern text, got:\n%s", src)
	}

	testBody := `
func TestGeneratedStringPatternXSDNonASCIIUnicodeBlock(t *testing.T) {
	valid := PatternXsdNonasciiUnicodeBlockCodegen{Value: ptr("\u03a9"), NonGreek: ptr("abc")}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate rejected valid block values: %v", err)
	}
	invalidGreek := PatternXsdNonasciiUnicodeBlockCodegen{Value: ptr("A")}
	if err := invalidGreek.Validate(); err == nil {
		t.Fatal("Validate accepted non-Greek value")
	} else if got, want := err.Error(), "/pattern-xsd-nonascii-unicode-block-codegen/value: pattern violation"; got != want {
		t.Fatalf("Validate error = %q, want %q", got, want)
	}
	invalidComplement := PatternXsdNonasciiUnicodeBlockCodegen{NonGreek: ptr("\u03a9")}
	if err := invalidComplement.Validate(); err == nil {
		t.Fatal("Validate accepted Greek value for complemented block")
	} else if got, want := err.Error(), "/pattern-xsd-nonascii-unicode-block-codegen/non_greek: pattern violation"; got != want {
		t.Fatalf("Validate error = %q, want %q", got, want)
	}
	if _, err := FromJSONIETF([]byte(` + "`" + `{"pattern-xsd-nonascii-unicode-block-codegen:value":"A"}` + "`" + `)); err == nil {
		t.Fatal("FromJSONIETF accepted non-Greek value")
	} else if got, want := err.Error(), "pattern-xsd-nonascii-unicode-block-codegen:value pattern violation"; got != want {
		t.Fatalf("FromJSONIETF error = %q, want %q", got, want)
	}
	if _, err := FromJSONIETF([]byte(` + "`" + `{"pattern-xsd-nonascii-unicode-block-codegen:non_greek":"\u03a9"}` + "`" + `)); err == nil {
		t.Fatal("FromJSONIETF accepted Greek value for complemented block")
	} else if got, want := err.Error(), "pattern-xsd-nonascii-unicode-block-codegen:non_greek pattern violation"; got != want {
		t.Fatalf("FromJSONIETF error = %q, want %q", got, want)
	}
}
`
	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoStringPatternXSDMultiCharacterEscapes(t *testing.T) {
	const source = `module pattern-xsd-multichar-escapes-codegen {
    namespace "urn:pattern-xsd-multichar-escapes-codegen";
    prefix pxmec;

    leaf unicode_digit {
        type string {
            pattern "\\d+";
        }
    }
    leaf word_symbol {
        type string {
            pattern "\\w+";
        }
    }
    leaf identifier {
        type string {
            pattern "\\i\\c*";
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	src, err := codegen.GenerateGo(ctx, "pattern-xsd-multichar-escapes-codegen")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	for _, want := range []string{
		`expr: "\\d+"`,
		`expr: "\\w+"`,
		`expr: "\\i\\c*"`,
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("generated source should preserve original XSD pattern %s, got:\n%s", want, src)
		}
	}

	testBody := `
func TestGeneratedStringPatternXSDMultiCharacterEscapes(t *testing.T) {
	valid := PatternXsdMulticharEscapesCodegen{
		UnicodeDigit: ptr("\u0661"),
		WordSymbol: ptr("$"),
		Identifier: ptr("name-1"),
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate rejected valid XML Schema escape values: %v", err)
	}
	invalidDigit := PatternXsdMulticharEscapesCodegen{UnicodeDigit: ptr("A")}
	if err := invalidDigit.Validate(); err == nil {
		t.Fatal("Validate accepted non-digit value")
	} else if got, want := err.Error(), "/pattern-xsd-multichar-escapes-codegen/unicode_digit: pattern violation"; got != want {
		t.Fatalf("Validate error = %q, want %q", got, want)
	}
	invalidIdentifier := PatternXsdMulticharEscapesCodegen{Identifier: ptr("1bad")}
	if err := invalidIdentifier.Validate(); err == nil {
		t.Fatal("Validate accepted invalid XML name-start value")
	} else if got, want := err.Error(), "/pattern-xsd-multichar-escapes-codegen/identifier: pattern violation"; got != want {
		t.Fatalf("Validate error = %q, want %q", got, want)
	}
	if _, err := FromJSONIETF([]byte(` + "`" + `{"pattern-xsd-multichar-escapes-codegen:unicode_digit":"A"}` + "`" + `)); err == nil {
		t.Fatal("FromJSONIETF accepted non-digit value")
	} else if got, want := err.Error(), "pattern-xsd-multichar-escapes-codegen:unicode_digit pattern violation"; got != want {
		t.Fatalf("FromJSONIETF error = %q, want %q", got, want)
	}
	if _, err := FromJSONIETF([]byte(` + "`" + `{"pattern-xsd-multichar-escapes-codegen:identifier":"1bad"}` + "`" + `)); err == nil {
		t.Fatal("FromJSONIETF accepted invalid XML name-start value")
	} else if got, want := err.Error(), "pattern-xsd-multichar-escapes-codegen:identifier pattern violation"; got != want {
		t.Fatalf("FromJSONIETF error = %q, want %q", got, want)
	}
}
`
	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoStringPatternXSDLiteralAnchors(t *testing.T) {
	const source = `module pattern-xsd-literal-anchors-codegen {
    namespace "urn:pattern-xsd-literal-anchors-codegen";
    prefix pxlac;

    leaf value {
        type string {
            pattern "^foo$";
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	src, err := codegen.GenerateGo(ctx, "pattern-xsd-literal-anchors-codegen")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, `expr: "^foo$"`) {
		t.Fatalf("generated source should preserve original XSD pattern text, got:\n%s", src)
	}

	testBody := `
func TestGeneratedStringPatternXSDLiteralAnchors(t *testing.T) {
	valid := PatternXsdLiteralAnchorsCodegen{Value: ptr("^foo$")}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate rejected literal anchor value: %v", err)
	}
	invalid := PatternXsdLiteralAnchorsCodegen{Value: ptr("foo")}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate accepted value as if ^ and $ were anchors")
	} else if got, want := err.Error(), "/pattern-xsd-literal-anchors-codegen/value: pattern violation"; got != want {
		t.Fatalf("Validate error = %q, want %q", got, want)
	}
	if _, err := FromJSONIETF([]byte(` + "`" + `{"pattern-xsd-literal-anchors-codegen:value":"foo"}` + "`" + `)); err == nil {
		t.Fatal("FromJSONIETF accepted value as if ^ and $ were anchors")
	} else if got, want := err.Error(), "pattern-xsd-literal-anchors-codegen:value pattern violation"; got != want {
		t.Fatalf("FromJSONIETF error = %q, want %q", got, want)
	}
}
`
	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoStringPatternXSDClassSubtraction(t *testing.T) {
	const source = `module pattern-xsd-class-subtraction-codegen {
    namespace "urn:pattern-xsd-class-subtraction-codegen";
    prefix pxcsc;

    leaf value {
        type string {
            pattern "[a-z-[aeiou]]+";
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	src, err := codegen.GenerateGo(ctx, "pattern-xsd-class-subtraction-codegen")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, `expr: "[a-z-[aeiou]]+"`) {
		t.Fatalf("generated source should preserve original XSD pattern text, got:\n%s", src)
	}

	testBody := `
func TestGeneratedStringPatternXSDClassSubtraction(t *testing.T) {
	valid := PatternXsdClassSubtractionCodegen{Value: ptr("bcdf")}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate rejected consonant value: %v", err)
	}
	invalid := PatternXsdClassSubtractionCodegen{Value: ptr("face")}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate accepted value containing vowels")
	} else if got, want := err.Error(), "/pattern-xsd-class-subtraction-codegen/value: pattern violation"; got != want {
		t.Fatalf("Validate error = %q, want %q", got, want)
	}
	if _, err := FromJSONIETF([]byte(` + "`" + `{"pattern-xsd-class-subtraction-codegen:value":"face"}` + "`" + `)); err == nil {
		t.Fatal("FromJSONIETF accepted value containing vowels")
	} else if got, want := err.Error(), "pattern-xsd-class-subtraction-codegen:value pattern violation"; got != want {
		t.Fatalf("FromJSONIETF error = %q, want %q", got, want)
	}
}
`
	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoStringPatternXSDCategoryClassSubtraction(t *testing.T) {
	const source = `module pattern-xsd-category-class-subtraction-codegen {
    namespace "urn:pattern-xsd-category-class-subtraction-codegen";
    prefix pxccsc;

    leaf value {
        type string {
            pattern "[\\p{IsGreek}-[\\p{Lu}]]+";
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	src, err := codegen.GenerateGo(ctx, "pattern-xsd-category-class-subtraction-codegen")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, `expr: "[\\p{IsGreek}-[\\p{Lu}]]+"`) {
		t.Fatalf("generated source should preserve original XSD pattern text, got:\n%s", src)
	}

	testBody := `
func TestGeneratedStringPatternXSDCategoryClassSubtraction(t *testing.T) {
	valid := PatternXsdCategoryClassSubtractionCodegen{Value: ptr("\u03c9")}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate rejected lowercase Greek value: %v", err)
	}
	invalidUpper := PatternXsdCategoryClassSubtractionCodegen{Value: ptr("\u03a9")}
	if err := invalidUpper.Validate(); err == nil {
		t.Fatal("Validate accepted uppercase Greek value")
	} else if got, want := err.Error(), "/pattern-xsd-category-class-subtraction-codegen/value: pattern violation"; got != want {
		t.Fatalf("Validate error = %q, want %q", got, want)
	}
	if _, err := FromJSONIETF([]byte(` + "`" + `{"pattern-xsd-category-class-subtraction-codegen:value":"\u03a9"}` + "`" + `)); err == nil {
		t.Fatal("FromJSONIETF accepted uppercase Greek value")
	} else if got, want := err.Error(), "pattern-xsd-category-class-subtraction-codegen:value pattern violation"; got != want {
		t.Fatalf("FromJSONIETF error = %q, want %q", got, want)
	}
}
	`
	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoStringPatternXSDNestedClassSubtraction(t *testing.T) {
	const source = `module pattern-xsd-nested-class-subtraction-codegen {
    namespace "urn:pattern-xsd-nested-class-subtraction-codegen";
    prefix pxncsc;

    leaf value {
        type string {
            pattern "[a-z-[a-m-[aeiou]]]+";
        }
    }
    leaf vowels {
        type string {
            pattern "[a-z-[^aeiou]]+";
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	src, err := codegen.GenerateGo(ctx, "pattern-xsd-nested-class-subtraction-codegen")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	for _, want := range []string{
		`expr: "[a-z-[a-m-[aeiou]]]+"`,
		`expr: "[a-z-[^aeiou]]+"`,
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("generated source should preserve original XSD pattern %s, got:\n%s", want, src)
		}
	}

	testBody := `
func TestGeneratedStringPatternXSDNestedClassSubtraction(t *testing.T) {
	valid := PatternXsdNestedClassSubtractionCodegen{
		Value: ptr("aeiounz"),
		Vowels: ptr("aeiou"),
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate rejected values allowed by nested subtraction: %v", err)
	}

	invalidNested := PatternXsdNestedClassSubtractionCodegen{Value: ptr("b")}
	if err := invalidNested.Validate(); err == nil {
		t.Fatal("Validate accepted value removed by nested subtraction")
	} else if got, want := err.Error(), "/pattern-xsd-nested-class-subtraction-codegen/value: pattern violation"; got != want {
		t.Fatalf("Validate nested subtraction error = %q, want %q", got, want)
	}
	if _, err := FromJSONIETF([]byte(` + "`" + `{"pattern-xsd-nested-class-subtraction-codegen:value":"b"}` + "`" + `)); err == nil {
		t.Fatal("FromJSONIETF accepted value removed by nested subtraction")
	} else if got, want := err.Error(), "pattern-xsd-nested-class-subtraction-codegen:value pattern violation"; got != want {
		t.Fatalf("FromJSONIETF nested subtraction error = %q, want %q", got, want)
	}

	invalidNegated := PatternXsdNestedClassSubtractionCodegen{Vowels: ptr("b")}
	if err := invalidNegated.Validate(); err == nil {
		t.Fatal("Validate accepted consonant for negated subtractor")
	} else if got, want := err.Error(), "/pattern-xsd-nested-class-subtraction-codegen/vowels: pattern violation"; got != want {
		t.Fatalf("Validate negated subtractor error = %q, want %q", got, want)
	}
	if _, err := FromJSONIETF([]byte(` + "`" + `{"pattern-xsd-nested-class-subtraction-codegen:vowels":"b"}` + "`" + `)); err == nil {
		t.Fatal("FromJSONIETF accepted consonant for negated subtractor")
	} else if got, want := err.Error(), "pattern-xsd-nested-class-subtraction-codegen:vowels pattern violation"; got != want {
		t.Fatalf("FromJSONIETF negated subtractor error = %q, want %q", got, want)
	}
}
`
	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoStringLengthPatternAnchorPosix(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-string-length-pattern-anchor-posix"), "module"), "types-string-length-pattern-anchor-posix")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-string-length-pattern-anchor-posix")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-string-length-pattern-anchor-posix", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-string-length-pattern-anchor-posix", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedStringLengthPatternAnchorPosix(t *testing.T) {
	code, err := NewTypesStringLengthPatternAnchorPosixCodeLength("abc_123")
	if err != nil {
		t.Fatalf("new length: %%v", err)
	}
	demo := TypesStringLengthPatternAnchorPosix{Code: ptr(code)}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	def := DefaultTypesStringLengthPatternAnchorPosixCodeLength()
	if len(def.String()) != 1 {
		t.Fatalf("default value should have length 1, got %%d", len(def.String()))
	}
	var invalid TypesStringLengthPatternAnchorPosixCodeLength
	invalidDemo := TypesStringLengthPatternAnchorPosix{Code: &invalid}
	if err := invalidDemo.Validate(); err == nil {
		t.Fatal("Validate accepted zero-value length wrapper")
	} else if got, want := err.Error(), "/types-string-length-pattern-anchor-posix/code: length 0 out of bounds for TypesStringLengthPatternAnchorPosixCodeLength"; got != want {
		t.Fatalf("Validate error = %%q, want %%q", got, want)
	}
	patternInvalid, err := NewTypesStringLengthPatternAnchorPosixCodeLength("abc-")
	if err != nil {
		t.Fatalf("new pattern-invalid length: %%v", err)
	}
	patternInvalidDemo := TypesStringLengthPatternAnchorPosix{Code: &patternInvalid}
	if err := patternInvalidDemo.Validate(); err == nil {
		t.Fatal("Validate accepted pattern-invalid length wrapper")
	} else if got, want := err.Error(), "/types-string-length-pattern-anchor-posix/code: pattern violation"; got != want {
		t.Fatalf("Validate error = %%q, want %%q", got, want)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoStringLengthCountsUnicodeCharacters(t *testing.T) {
	const source = `module string-length-unicode-codegen {
    namespace "urn:string-length-unicode-codegen";
    prefix sluc;

    leaf label {
        type string {
            length "1";
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "string-length-unicode-codegen")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	testBody := `
func TestGeneratedStringLengthCountsUnicodeCharacters(t *testing.T) {
	value, err := NewStringLengthUnicodeCodegenLabelLength("\u00e9")
	if err != nil {
		t.Fatalf("single Unicode character should satisfy length 1: %v", err)
	}
	demo := StringLengthUnicodeCodegen{Label: &value}
	if err := demo.Validate(); err != nil {
		t.Fatalf("Validate rejected single Unicode character: %v", err)
	}
	parsed, err := FromJSONIETF([]byte(` + "`" + `{"string-length-unicode-codegen:label":"\u00e9"}` + "`" + `))
	if err != nil {
		t.Fatalf("FromJSONIETF rejected single Unicode character: %v", err)
	}
	if parsed.Label == nil || parsed.Label.String() != "\u00e9" {
		t.Fatalf("parsed label = %#v, want single Unicode character", parsed.Label)
	}
	if _, err := NewStringLengthUnicodeCodegenLabelLength("\u00e9x"); err == nil {
		t.Fatal("constructor accepted two-character string for length 1")
	}
	if _, err := FromJSONIETF([]byte(` + "`" + `{"string-length-unicode-codegen:label":"\u00e9x"}` + "`" + `)); err == nil {
		t.Fatal("FromJSONIETF accepted two-character string for length 1")
	}
}
`
	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoStaticNamespaceAttributeEscaping(t *testing.T) {
	const source = `module static-namespace-escape-codegen {
    namespace "urn:static-namespace-escape&active";
    prefix snse;

    leaf label {
        type string;
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "static-namespace-escape-codegen")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	testBody := `
func TestGeneratedStaticNamespaceAttributeEscaping(t *testing.T) {
	demo := StaticNamespaceEscapeCodegen{Label: ptr("ok")}
	wantXML := "<label xmlns=\"urn:static-namespace-escape&amp;active\">ok</label>\n"
	if got := demo.ToXML(); got != wantXML {
		t.Fatalf("static namespace XML escaping mismatch:\n got: %q\nwant: %q", got, wantXML)
	}
}
`
	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoRangeLengthMinMaxKeywords(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-range-length-min-max-keywords"), "module"), "types-range-length-min-max-keywords")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-range-length-min-max-keywords")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-range-length-min-max-keywords", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-range-length-min-max-keywords", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedRangeLengthMinMaxKeywords(t *testing.T) {
	code, err := NewTypesRangeLengthMinMaxKeywordsTopCodeRange(-2147483648)
	if err != nil {
		t.Fatalf("new range: %%v", err)
	}
	label, err := NewTypesRangeLengthMinMaxKeywordsTopLabelLength("max")
	if err != nil {
		t.Fatalf("new length: %%v", err)
	}
	demo := TypesRangeLengthMinMaxKeywords{
		Top: TypesRangeLengthMinMaxKeywordsTop{
			Code:  ptr(code),
			Label: ptr(label),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	emptyLabel, err := NewTypesRangeLengthMinMaxKeywordsTopLabelLength("")
	if err != nil {
		t.Fatalf("empty label should be valid for min length 0: %%v", err)
	}
	if emptyLabel.String() != "" {
		t.Fatalf("empty label should round-trip, got %%q", emptyLabel.String())
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoJSONIETFListArrayKeysFirst(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "json-ietf-list-array-keys-first"), "module"), "json-ietf-list-array-keys-first")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "json-ietf-list-array-keys-first")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "json-ietf-list-array-keys-first", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "json-ietf-list-array-keys-first", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedJSONIETFListArrayKeysFirst(t *testing.T) {
	demo := JsonIetfListArrayKeysFirst{
		Top: JsonIetfListArrayKeysFirstTop{
			Interface_: []JsonIetfListArrayKeysFirstTopInterfaceEntry{
				{
					Name:     "eth0",
					Protocol: "ip",
					Mtu:      ptr(uint16(1500)),
				},
			},
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoEnumMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-enumeration-explicit-values-sparse"), "module"), "types-enumeration-explicit-values-sparse")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-enumeration-explicit-values-sparse")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-enumeration-explicit-values-sparse", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-enumeration-explicit-values-sparse", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedEnumMatchesLibyang(t *testing.T) {
	ipv6 := TypesEnumerationExplicitValuesSparseIpVersionEnumIpv6
	demo := TypesEnumerationExplicitValuesSparse{IpVersion: &ipv6}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF enum JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed enum JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoBitsMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-bits-explicit-positions-gaps"), "module"), "types-bits-explicit-positions-gaps")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-bits-explicit-positions-gaps")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-bits-explicit-positions-gaps", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-bits-explicit-positions-gaps", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedBitsMatchesLibyang(t *testing.T) {
	ops, err := NewTypesBitsExplicitPositionsGapsOpsBits([]string{"delete", "read"})
	if err != nil {
		t.Fatalf("new bits: %%v", err)
	}
	demo := TypesBitsExplicitPositionsGaps{Ops: &ops}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF bits JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed bits JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
	spaced, err := FromJSONIETF([]byte(`+"`"+`{"types-bits-explicit-positions-gaps:ops":" read  delete "}`+"`"+`))
	if err != nil {
		t.Fatalf("FromJSONIETF rejected libyang-compatible space-padded bits value: %%v", err)
	}
	if got := spaced.ToJSONIETF(); got != wantJSON {
		t.Fatalf("space-padded bits JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
	if _, err := NewTypesBitsExplicitPositionsGapsOpsBits([]string{"read", "read"}); err == nil {
		t.Fatal("New bits accepted duplicate bit name")
	} else if got, want := err.Error(), "duplicate bit name: read"; got != want {
		t.Fatalf("New bits error = %%q, want %%q", got, want)
	}
	if _, err := FromJSONIETF([]byte(`+"`"+`{"types-bits-explicit-positions-gaps:ops":"read read"}`+"`"+`)); err == nil {
		t.Fatal("FromJSONIETF accepted duplicate bit name")
	} else if got, want := err.Error(), "duplicate bit name: read"; got != want {
		t.Fatalf("FromJSONIETF error = %%q, want %%q", got, want)
	}
	if _, err := FromJSONIETF([]byte(`+"`"+`{"types-bits-explicit-positions-gaps:ops":"read\tdelete"}`+"`"+`)); err == nil {
		t.Fatal("FromJSONIETF accepted tab-separated bits value")
	}
	if _, err := FromJSONIETF([]byte(`+"`"+`{"types-bits-explicit-positions-gaps:ops":"read\u2003delete"}`+"`"+`)); err == nil {
		t.Fatal("FromJSONIETF accepted Unicode-whitespace-separated bits value")
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoEnumBitsAutoPositionMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-enum-bits-auto-position"), "module"), "types-enum-bits-auto-position")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-enum-bits-auto-position")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "TypesEnumBitsAutoPositionTopEEnumB") || !strings.Contains(src, "NewTypesEnumBitsAutoPositionTopFBits") {
		t.Fatalf("generated source should include enum and bits helpers with auto positions, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-enum-bits-auto-position", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-enum-bits-auto-position", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedEnumBitsAutoPositionMatchesLibyang(t *testing.T) {
	e := TypesEnumBitsAutoPositionTopEEnumB
	f, err := NewTypesEnumBitsAutoPositionTopFBits([]string{"x", "y"})
	if err != nil {
		t.Fatalf("new bits: %%v", err)
	}
	demo := TypesEnumBitsAutoPosition{
		Top: TypesEnumBitsAutoPositionTop{
			E: &e,
			F: &f,
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoEnumerationZeroValueMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-enumeration-zero-value-disabled"), "module"), "types-enumeration-zero-value-disabled")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-enumeration-zero-value-disabled")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "TypesEnumerationZeroValueDisabledStatusEnumDisabled") {
		t.Fatalf("generated source should include zero-valued enum constant, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-enumeration-zero-value-disabled", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-enumeration-zero-value-disabled", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedEnumerationZeroValueMatchesLibyang(t *testing.T) {
	status := TypesEnumerationZeroValueDisabledStatusEnumDisabled
	demo := TypesEnumerationZeroValueDisabled{Status: &status}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	invalid := TypesEnumerationZeroValueDisabledStatusEnum(99)
	invalidDemo := TypesEnumerationZeroValueDisabled{Status: &invalid}
	if err := invalidDemo.Validate(); err == nil {
		t.Fatal("Validate accepted invalid enum value")
	} else if got, want := err.Error(), "/types-enumeration-zero-value-disabled/status: invalid enum value"; got != want {
		t.Fatalf("Validate error = %%q, want %%q", got, want)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoUint8RangeMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-uint-uint8-range"), "module"), "types-uint-uint8-range")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-uint-uint8-range")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "U8Dscp *TypesUintUint8RangeTopU8DscpRange") {
		t.Fatalf("generated source should include uint8 range helper, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-uint-uint8-range", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-uint-uint8-range", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedUint8RangeMatchesLibyang(t *testing.T) {
	dscp, err := NewTypesUintUint8RangeTopU8DscpRange(63)
	if err != nil {
		t.Fatalf("dscp range: %%v", err)
	}
	demo := TypesUintUint8Range{
		Top: TypesUintUint8RangeTop{
			U8Min:  ptr(uint8(0)),
			U8Mid:  ptr(uint8(128)),
			U8Max:  ptr(uint8(255)),
			U8Dscp: &dscp,
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoEmptyLeafMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-empty-leaf-null-json"), "module"), "types-empty-leaf-null-json")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-empty-leaf-null-json")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "Enabled *struct{}") {
		t.Fatalf("generated source should represent optional empty leaf as *struct{}, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-empty-leaf-null-json", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-empty-leaf-null-json", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedEmptyLeafMatchesLibyang(t *testing.T) {
	demo := TypesEmptyLeafNullJson{
		Top: TypesEmptyLeafNullJsonTop{
			Enabled: &struct{}{},
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF empty leaf JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed empty leaf JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoBinaryLengthMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-binary-length-base64"), "module"), "types-binary-length-base64")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-binary-length-base64")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "ExtComm *string") {
		t.Fatalf("generated source should represent binary as base64 string, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-binary-length-base64", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-binary-length-base64", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedBinaryLengthMatchesLibyang(t *testing.T) {
	demo := TypesBinaryLengthBase64{
		ExtComm: ptr("AQIDBAUGBwg="),
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF binary JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed binary JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
	if _, err := FromJSONIETF([]byte(`+"`"+`{"types-binary-length-base64:ext-comm":"@"}`+"`"+`)); err == nil {
		t.Fatal("FromJSONIETF accepted invalid base64 binary")
	} else if got, want := err.Error(), "types-binary-length-base64:ext-comm must be base64 binary: illegal base64 data at input byte 0"; got != want {
		t.Fatalf("FromJSONIETF error = %%q, want %%q", got, want)
	}
	if _, err := FromJSONIETF([]byte(`+"`"+`{"types-binary-length-base64:ext-comm":"AQID"}`+"`"+`)); err == nil {
		t.Fatal("FromJSONIETF accepted length-invalid binary")
	} else if got, want := err.Error(), "types-binary-length-base64:ext-comm binary length 3 out of bounds"; got != want {
		t.Fatalf("FromJSONIETF error = %%q, want %%q", got, want)
	}
	if _, err := FromJSONIETF([]byte(`+"`"+`{"types-binary-length-base64:ext-comm":"AQIDBAUG\nBwg="}`+"`"+`)); err == nil {
		t.Fatal("FromJSONIETF accepted newline-split base64 binary")
	}
	invalidBase64 := TypesBinaryLengthBase64{ExtComm: ptr("@")}
	if err := invalidBase64.Validate(); err == nil {
		t.Fatal("Validate accepted invalid base64 binary")
	} else if got, want := err.Error(), "/types-binary-length-base64/ext-comm: must be base64 binary: illegal base64 data at input byte 0"; got != want {
		t.Fatalf("Validate error = %%q, want %%q", got, want)
	}
	invalidLength := TypesBinaryLengthBase64{ExtComm: ptr("AQID")}
	if err := invalidLength.Validate(); err == nil {
		t.Fatal("Validate accepted length-invalid binary")
	} else if got, want := err.Error(), "/types-binary-length-base64/ext-comm: binary length 3 out of bounds"; got != want {
		t.Fatalf("Validate error = %%q, want %%q", got, want)
	}
	newlineBase64 := TypesBinaryLengthBase64{ExtComm: ptr("AQIDBAUG\nBwg=")}
	if err := newlineBase64.Validate(); err == nil {
		t.Fatal("Validate accepted newline-split base64 binary")
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoBinaryPEMNewlineParseCanonicalizes(t *testing.T) {
	const source = `module binary-pem-newline-codegen {
    namespace "urn:binary-pem-newline-codegen";
    prefix bpnc;

    leaf blob {
        type binary {
            length "49";
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "binary-pem-newline-codegen")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	testBody := `
func TestGeneratedBinaryPEMNewlineParseCanonicalizes(t *testing.T) {
	parsed, err := FromJSONIETF([]byte("{\"binary-pem-newline-codegen:blob\":\"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA\\nAA==\"}"))
	if err != nil {
		t.Fatalf("FromJSONIETF rejected PEM-newline binary: %v", err)
	}
	wantJSON := "{\n  \"binary-pem-newline-codegen:blob\": \"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==\"\n}\n"
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("canonical JSON mismatch:\n got: %q\nwant: %q", got, wantJSON)
	}
}
`
	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoUserOrderedListMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "list-ordered-by-user-insertion"), "module"), "list-ordered-by-user-insertion")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "list-ordered-by-user-insertion")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "list-ordered-by-user-insertion", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "list-ordered-by-user-insertion", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedUserOrderedListMatchesLibyang(t *testing.T) {
	var rules UserOrderedVec[ListOrderedByUserInsertionTopRuleEntry]
	rules.InsertLast(ListOrderedByUserInsertionTopRuleEntry{Name: "c", Action: ptr("drop")})
	rules.InsertLast(ListOrderedByUserInsertionTopRuleEntry{Name: "a", Action: ptr("accept")})
	rules.InsertLast(ListOrderedByUserInsertionTopRuleEntry{Name: "b", Action: ptr("reject")})
	var names []string
	rules.Iter(func(rule ListOrderedByUserInsertionTopRuleEntry) bool {
		names = append(names, rule.Name)
		return true
	})
	if len(names) != 3 || names[0] != "c" || names[1] != "a" || names[2] != "b" {
		t.Fatalf("Iter order = %%v", names)
	}
	demo := ListOrderedByUserInsertion{Top: ListOrderedByUserInsertionTop{Rule: rules}}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF user-ordered list JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed user-ordered list JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoSystemLeafListCanonicalizesMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "leaflist-ordered-by-system"), "module"), "leaflist-ordered-by-system")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "leaflist-ordered-by-system")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "Ports []uint16") {
		t.Fatalf("generated source should use a plain slice for ordered-by system leaf-list, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "leaflist-ordered-by-system", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "leaflist-ordered-by-system", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedSystemLeafListCanonicalizesMatchesLibyang(t *testing.T) {
	demo := LeaflistOrderedBySystem{
		Top: LeaflistOrderedBySystemTop{
			Ports: []uint16{30, 10, 20},
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromJSONIETF([]byte(`+"`"+`{
  "leaflist-ordered-by-system:top": {
    "ports": [
      30,
      10,
      20
    ]
  }
}`+"`"+`))
	if err != nil {
		t.Fatalf("FromJSONIETF system leaf-list JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed system leaf-list JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoUserOrderedLeafListPreservesOrderMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "leaflist-ordered-by-user"), "module"), "leaflist-ordered-by-user")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "leaflist-ordered-by-user")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "Actions UserOrderedVec[string]") {
		t.Fatalf("generated source should use UserOrderedVec for ordered-by user leaf-list, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "leaflist-ordered-by-user", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "leaflist-ordered-by-user", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedUserOrderedLeafListPreservesOrderMatchesLibyang(t *testing.T) {
	demo := LeaflistOrderedByUser{
		Top: LeaflistOrderedByUserTop{
			Actions: NewUserOrderedVec([]string{"c", "a", "b"}),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF user-ordered leaf-list JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed user-ordered leaf-list JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoSystemListCanonicalizesNumericKeyMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "list-ordered-by-system-canonical"), "module"), "list-ordered-by-system-canonical")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "list-ordered-by-system-canonical")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "Vlan []ListOrderedBySystemCanonicalTopVlanEntry") {
		t.Fatalf("generated source should use a plain slice for ordered-by system list, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "list-ordered-by-system-canonical", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "list-ordered-by-system-canonical", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedSystemListCanonicalizesNumericKeyMatchesLibyang(t *testing.T) {
	demo := ListOrderedBySystemCanonical{
		Top: ListOrderedBySystemCanonicalTop{
			Vlan: []ListOrderedBySystemCanonicalTopVlanEntry{
				{VlanId: 300, Name: ptr("lab")},
				{VlanId: 100, Name: ptr("mgmt")},
				{VlanId: 200, Name: ptr("prod")},
			},
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromJSONIETF([]byte(`+"`"+`{
  "list-ordered-by-system-canonical:top": {
    "vlan": [
      {
        "vlan-id": 300,
        "name": "lab"
      },
      {
        "vlan-id": 100,
        "name": "mgmt"
      },
      {
        "vlan-id": 200,
        "name": "prod"
      }
    ]
  }
}`+"`"+`))
	if err != nil {
		t.Fatalf("FromJSONIETF system list JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed system list JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoSystemListCanonicalizesStringKeyMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "system-list-canonical"), "module"), "system-list-demo")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "system-list-demo")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "Server []SystemListDemoConfigServerEntry") {
		t.Fatalf("generated source should use a plain slice for ordered-by system list, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "system-list-canonical", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "system-list-canonical", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedSystemListCanonicalizesStringKeyMatchesLibyang(t *testing.T) {
	demo := SystemListDemo{
		Config: SystemListDemoConfig{
			Server: []SystemListDemoConfigServerEntry{
				{Name: "c", Value: ptr("3")},
				{Name: "a", Value: ptr("1")},
				{Name: "b", Value: ptr("2")},
			},
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoMixedLeafListOrderingMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "json-ietf-leaflist-array-user-system"), "module"), "json-ietf-leaflist-array-user-system")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "json-ietf-leaflist-array-user-system")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "Priorities UserOrderedVec[uint8]") || !strings.Contains(src, "Ports []uint16") {
		t.Fatalf("generated source should distinguish user and system leaf-list order, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "json-ietf-leaflist-array-user-system", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "json-ietf-leaflist-array-user-system", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedMixedLeafListOrderingMatchesLibyang(t *testing.T) {
	demo := JsonIetfLeaflistArrayUserSystem{
		Top: JsonIetfLeaflistArrayUserSystemTop{
			Priorities: NewUserOrderedVec([]uint8{100, 50, 75}),
			Ports:      []uint16{8080, 22, 443, 80},
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoLeafListWithinUserOrderedListEntryMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "leaf-list-within-list-entry"), "module"), "leaf-list-within-list-entry")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "leaf-list-within-list-entry")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "Policy UserOrderedVec[LeafListWithinListEntryPolicyEntry]") || !strings.Contains(src, "Actions UserOrderedVec[string]") {
		t.Fatalf("generated source should preserve nested user ordering, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "leaf-list-within-list-entry", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "leaf-list-within-list-entry", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedLeafListWithinUserOrderedListEntryMatchesLibyang(t *testing.T) {
	policies := NewUserOrderedVec([]LeafListWithinListEntryPolicyEntry{
		{Name: "p2", Actions: NewUserOrderedVec([]string{"b", "a"})},
		{Name: "p1", Actions: NewUserOrderedVec([]string{"b", "a"})},
	})
	demo := LeafListWithinListEntry{Policy: policies}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoOrderingCompositeKeyWideMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "ordering-composite-key-wide"), "module"), "ordering-composite-key-wide")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "ordering-composite-key-wide")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "OrderingCompositeKeyWideEdgesEntryFieldOrder = []string{\"src-type\", \"src-slot\", \"src-pfe\", \"dst-type\", \"dst-slot\", \"dst-pfe\", \"weight\"}") {
		t.Fatalf("generated source should place all composite keys before non-keys, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "ordering-composite-key-wide", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "ordering-composite-key-wide", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedOrderingCompositeKeyWideMatchesLibyang(t *testing.T) {
	demo := OrderingCompositeKeyWide{
		Edges: []OrderingCompositeKeyWideEdgesEntry{{
			SrcType: "fpc",
			SrcSlot: 1,
			SrcPfe:  0,
			DstType: "fpc",
			DstSlot: 2,
			DstPfe:  1,
			Weight:  ptr(uint32(100)),
		}},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoCompositeKeyWithInterleavedContainersMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "composite-key-with-interleaved-containers"), "module"), "composite-key-with-interleaved-containers")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "composite-key-with-interleaved-containers")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "CompositeKeyWithInterleavedContainersRouteEntryFieldOrder = []string{\"dest-prefix\", \"next-hop-ip\", \"metrics\"}") {
		t.Fatalf("generated source should move list keys before interleaved containers, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "composite-key-with-interleaved-containers", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "composite-key-with-interleaved-containers", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedCompositeKeyWithInterleavedContainersMatchesLibyang(t *testing.T) {
	demo := CompositeKeyWithInterleavedContainers{
		Route: []CompositeKeyWithInterleavedContainersRouteEntry{{
			DestPrefix: "10.0.0.0/8",
			NextHopIp:  "192.0.2.1",
			Metrics: CompositeKeyWithInterleavedContainersRouteEntryMetrics{
				Distance: ptr(uint8(10)),
			},
		}},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoLeafListDefaultsMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "leaflist-with-defaults"), "module"), "leaflist-with-defaults")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "leaflist-with-defaults")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "Servers []string") {
		t.Fatalf("generated source should represent system leaf-list defaults as explicit values, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "leaflist-with-defaults", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "leaflist-with-defaults", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedLeafListDefaultsMatchesLibyang(t *testing.T) {
	demo := LeaflistWithDefaults{
		Top: LeaflistWithDefaultsTop{
			Servers: []string{"8.8.8.8", "8.8.4.4"},
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	if got, want := demo.ToJSONIETFWithDefaults(WithDefaultsTrim), "{\n}\n"; got != want {
		t.Fatalf("with-defaults trim JSON mismatch:\n got: %%q\nwant: %%q", got, want)
	}

	defaulted := LeaflistWithDefaults{Top: LeaflistWithDefaultsTop{}}
	if got, want := defaulted.ToJSONIETFWithDefaults(WithDefaultsAll), %q; got != want {
		t.Fatalf("with-defaults all JSON mismatch:\n got: %%q\nwant: %%q", got, want)
	}
	if got, want := defaulted.ToJSONIETFWithDefaults(WithDefaultsAllTagged), %q; got != want {
		t.Fatalf("with-defaults all-tagged JSON mismatch:\n got: %%q\nwant: %%q", got, want)
	}
}
`, string(wantXML), string(wantJSON), string(wantJSON), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoGroupingConfigStateMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "linkage-grouping-config-state"), "module"), "linkage-grouping-config-state")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "linkage-grouping-config-state")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "LinkageGroupingConfigStateIntfFieldOrder = []string{\"name\", \"config\", \"state\"}") {
		t.Fatalf("generated source should preserve grouping expansion under config and state containers, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "linkage-grouping-config-state", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "linkage-grouping-config-state", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedGroupingConfigStateMatchesLibyang(t *testing.T) {
	demo := LinkageGroupingConfigState{
		Intf: LinkageGroupingConfigStateIntf{
			Name: ptr("eth0"),
			Config: LinkageGroupingConfigStateIntfConfig{
				Ip:   ptr("192.0.2.1"),
				Mask: ptr(uint8(24)),
			},
			State: LinkageGroupingConfigStateIntfState{
				Ip:   ptr("192.0.2.1"),
				Mask: ptr(uint8(24)),
			},
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoRefineDefaultMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "linkage-refine-default"), "module"), "linkage-refine-default")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "linkage-refine-default")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "Ntp *LinkageRefineDefaultNtp") {
		t.Fatalf("generated source should keep refined presence container, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "linkage-refine-default", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "linkage-refine-default", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedRefineDefaultMatchesLibyang(t *testing.T) {
	demo := LinkageRefineDefault{
		Ntp: &LinkageRefineDefaultNtp{
			Alg: ptr("sha256"),
			Key: ptr("secret"),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoRefinePresenceMustMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "linkage-refine-presence-must"), "module"), "linkage-refine-presence-must")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "linkage-refine-presence-must")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "Opts *LinkageRefinePresenceMustSystemOpts") {
		t.Fatalf("generated source should apply refine presence to grouping container, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "linkage-refine-presence-must", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "linkage-refine-presence-must", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedRefinePresenceMustMatchesLibyang(t *testing.T) {
	demo := LinkageRefinePresenceMust{
		System: LinkageRefinePresenceMustSystem{
			Opts: &LinkageRefinePresenceMustSystemOpts{
				Val: ptr(uint8(5)),
			},
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoRefineMinMaxIfFeatureMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "linkage-refine-min-max-iffeature"), "module"), "linkage-refine-min-max-iffeature")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "linkage-refine-min-max-iffeature")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "Tags []string") || !strings.Contains(src, "AdvancedOpt *string") {
		t.Fatalf("generated source should keep grouped nodes while skipping properties from disabled refine if-feature, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "linkage-refine-min-max-iffeature", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "linkage-refine-min-max-iffeature", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedRefineMinMaxIfFeatureMatchesLibyang(t *testing.T) {
	demo := LinkageRefineMinMaxIffeature{
		Policy: LinkageRefineMinMaxIffeaturePolicy{
			Tags: []string{"a", "b"},
		},
		Service: LinkageRefineMinMaxIffeatureService{
			Mode: ptr("auto"),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoFeatureIfFeatureMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "constraints-feature-iffeature"), "module"), "constraints-feature-iffeature")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "constraints-feature-iffeature")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if strings.Contains(src, "DroppedPackets") || !strings.Contains(src, "PacketCount *uint64") {
		t.Fatalf("generated source should filter nested if-feature leaves with disabled features, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "constraints-feature-iffeature", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "constraints-feature-iffeature", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedFeatureIfFeatureMatchesLibyang(t *testing.T) {
	demo := ConstraintsFeatureIffeature{
		Stats: ConstraintsFeatureIffeatureStats{
			PacketCount: ptr(uint64(42)),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoFeaturePresenceMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "constraints-feature-presence"), "module"), "constraints-feature-presence")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "constraints-feature-presence")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if strings.Contains(src, "CandidateStore") || !strings.Contains(src, "Ssh *ConstraintsFeaturePresenceSsh") {
		t.Fatalf("generated source should filter feature-gated presence container, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "constraints-feature-presence", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "constraints-feature-presence", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedFeaturePresenceMatchesLibyang(t *testing.T) {
	demo := ConstraintsFeaturePresence{Ssh: &ConstraintsFeaturePresenceSsh{}}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoFeatureDependencyMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "constraints-feature-dependency"), "module"), "constraints-feature-dependency")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "constraints-feature-dependency")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "Gated *string") {
		t.Fatalf("generated source should evaluate dependent feature false by default and keep not-b leaf, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "constraints-feature-dependency", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "constraints-feature-dependency", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedFeatureDependencyMatchesLibyang(t *testing.T) {
	demo := ConstraintsFeatureDependency{
		Top: ConstraintsFeatureDependencyTop{
			Gated: ptr("enabled"),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoListKeylessPositionalMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "list-keyless-positional"), "module"), "list-keyless-positional")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "list-keyless-positional")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if strings.Contains(src, "sort.Slice") {
		t.Fatalf("generated source should not canonicalize keyless state lists, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "list-keyless-positional", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "list-keyless-positional", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedListKeylessPositionalMatchesLibyang(t *testing.T) {
	demo := ListKeylessPositional{
		State: ListKeylessPositionalState{
			Sample: []ListKeylessPositionalStateSampleEntry{
				{Reading: ptr(uint64(300))},
				{Reading: ptr(uint64(100))},
				{Reading: ptr(uint64(200))},
			},
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoIdentityStandaloneMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "identity-standalone"), "module"), "identity-standalone")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "identity-standalone")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "IdentityStandaloneInventoryComponentTypeEnumChassis") {
		t.Fatalf("generated source should include standalone identityref derived value, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "identity-standalone", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "identity-standalone", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedIdentityStandaloneMatchesLibyang(t *testing.T) {
	componentType := IdentityStandaloneInventoryComponentTypeEnumChassis
	demo := IdentityStandalone{
		Inventory: IdentityStandaloneInventory{
			ComponentType: &componentType,
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF identityref JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed identityref JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoIdentityHierarchyWithIdentityrefMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "identity-hierarchy-with-identityref"), "module"), "identity-hierarchy-with-identityref")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "identity-hierarchy-with-identityref")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "IdentityHierarchyWithIdentityrefPrimaryEnumRsvpTunnel") || !strings.Contains(src, "IdentityHierarchyWithIdentityrefBackupEnumRsvpTunnel") {
		t.Fatalf("generated source should include transitive identityref derived values for both bases, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "identity-hierarchy-with-identityref", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "identity-hierarchy-with-identityref", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedIdentityHierarchyWithIdentityrefMatchesLibyang(t *testing.T) {
	primary := IdentityHierarchyWithIdentityrefPrimaryEnumRsvpTunnel
	backup := IdentityHierarchyWithIdentityrefBackupEnumRsvpTunnel
	demo := IdentityHierarchyWithIdentityref{
		Primary: &primary,
		Backup:  &backup,
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF identityref JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed identityref JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoForeignIdentityrefMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-identityref-foreign-module-prefix"), "module"), "types-identityref-foreign-module-prefix")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-identityref-foreign-module-prefix")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-identityref-foreign-module-prefix", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-identityref-foreign-module-prefix", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedForeignIdentityrefMatchesLibyang(t *testing.T) {
	cpu := TypesIdentityrefForeignModulePrefixComponentEnumCpu
	demo := TypesIdentityrefForeignModulePrefix{Component: &cpu}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF identityref JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed identityref JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
	if parsed, ok := ParseTypesIdentityrefForeignModulePrefixComponentEnum("types-identityref-foreign-base:cpu"); !ok || parsed != cpu {
		t.Fatalf("qualified foreign identity parse = (%%v,%%v), want cpu,true", parsed, ok)
	}
	if _, ok := ParseTypesIdentityrefForeignModulePrefixComponentEnum("cpu"); ok {
		t.Fatal("foreign identity parser accepted bare identity name")
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoIANAIdentityrefForeignMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, schemaCorpusDir(t, "identityref-iana-if-type-foreign"), "identityref-iana-if-type-foreign")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "identityref-iana-if-type-foreign")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "IdentityrefIanaIfTypeForeignTypeEnumEthernetCsmacd") {
		t.Fatalf("generated source should contain IANA ethernet identityref enum, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "identityref-iana-if-type-foreign", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "identityref-iana-if-type-foreign", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedIANAIdentityrefForeignMatchesLibyang(t *testing.T) {
	typ := IdentityrefIanaIfTypeForeignTypeEnumEthernetCsmacd
	parsed, ok := ParseIdentityrefIanaIfTypeForeignTypeEnum("iana-if-type:ethernetCsmacd")
	if !ok || parsed != typ {
		t.Fatalf("parse iana-if-type:ethernetCsmacd = %%v,%%v", parsed, ok)
	}
	demo := IdentityrefIanaIfTypeForeign{Type_: &typ}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	roundTrip, err := FromJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF identityref JSON: %%v", err)
	}
	if got := roundTrip.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed identityref JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoIETFInterfacesMatchesLibyang(t *testing.T) {
	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatalf("new context: %v", err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(schemaCorpusDir(t, "ietf-interfaces")); err != nil {
		t.Fatalf("set search path: %v", err)
	}
	if err := ctx.LoadModule("ietf-interfaces"); err != nil {
		t.Fatalf("load ietf-interfaces: %v", err)
	}
	if err := ctx.LoadModule("iana-if-type"); err != nil {
		t.Fatalf("load iana-if-type: %v", err)
	}

	src, err := codegen.GenerateGo(ctx, "ietf-interfaces")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "IetfInterfacesInterfacesInterfaceEntryTypeEnumEthernetCsmacd") ||
		!strings.Contains(src, "IetfInterfacesInterfacesInterfaceEntryTypeEnumSoftwareLoopback") {
		t.Fatalf("generated source should contain IANA interface identityrefs, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "ietf-interfaces", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "ietf-interfaces", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedIETFInterfacesMatchesLibyang(t *testing.T) {
	ethType := IetfInterfacesInterfacesInterfaceEntryTypeEnumEthernetCsmacd
	loopType := IetfInterfacesInterfacesInterfaceEntryTypeEnumSoftwareLoopback
	demo := IetfInterfaces{
		Interfaces: IetfInterfacesInterfaces{
			Interface_: []IetfInterfacesInterfacesInterfaceEntry{
				{
					Name:    "eth1",
					Type_:   loopType,
					Enabled: ptr(false),
				},
				{
					Name:    "eth0",
					Type_:   ethType,
					Enabled: ptr(true),
				},
			},
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF identityref JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed identityref JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoIdentityrefSingleBaseMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-identityref-single-base"), "module"), "types-identityref-single-base")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-identityref-single-base")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "type TypesIdentityrefSingleBaseProtoEnum int") {
		t.Fatalf("generated source should contain identityref enum, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-identityref-single-base", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-identityref-single-base", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedIdentityrefSingleBaseMatchesLibyang(t *testing.T) {
	proto := TypesIdentityrefSingleBaseProtoEnumTcp
	parsed, ok := ParseTypesIdentityrefSingleBaseProtoEnum("tcp")
	if !ok || parsed != proto {
		t.Fatalf("parse tcp = %%v,%%v", parsed, ok)
	}
	demo := TypesIdentityrefSingleBase{Proto: &proto}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	invalid := TypesIdentityrefSingleBaseProtoEnum(99)
	invalidDemo := TypesIdentityrefSingleBase{Proto: &invalid}
	if err := invalidDemo.Validate(); err == nil {
		t.Fatal("Validate accepted invalid identityref value")
	} else if got, want := err.Error(), "/types-identityref-single-base/proto: invalid identityref value"; got != want {
		t.Fatalf("Validate error = %%q, want %%q", got, want)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoIdentityrefMultipleBasesMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-identityref-multiple-bases"), "module"), "types-identityref-multiple-bases")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-identityref-multiple-bases")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "type TypesIdentityrefMultipleBasesClassEnum int") {
		t.Fatalf("generated source should contain identityref enum, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-identityref-multiple-bases", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-identityref-multiple-bases", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedIdentityrefMultipleBasesMatchesLibyang(t *testing.T) {
	class := TypesIdentityrefMultipleBasesClassEnumEthernet
	demo := TypesIdentityrefMultipleBases{Class: &class}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoIdentityrefDerivedHierarchyMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-identityref-derived-hierarchy"), "module"), "types-identityref-derived-hierarchy")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-identityref-derived-hierarchy")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "type TypesIdentityrefDerivedHierarchyItemEnum int") {
		t.Fatalf("generated source should contain identityref enum, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-identityref-derived-hierarchy", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-identityref-derived-hierarchy", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedIdentityrefDerivedHierarchyMatchesLibyang(t *testing.T) {
	item := TypesIdentityrefDerivedHierarchyItemEnumFpc
	demo := TypesIdentityrefDerivedHierarchy{Item: &item}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoIdentityCrossModuleDerivationMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "identity-cross-module-derivation"), "module"), "identity-cross-module-derivation")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "identity-cross-module-derivation")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "type IdentityCrossModuleDerivationProtoEnum int") {
		t.Fatalf("generated source should contain identityref enum, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "identity-cross-module-derivation", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "identity-cross-module-derivation", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedIdentityCrossModuleDerivationMatchesLibyang(t *testing.T) {
	proto := IdentityCrossModuleDerivationProtoEnumTcp
	parsed, ok := ParseIdentityCrossModuleDerivationProtoEnum("tcp")
	if !ok || parsed != proto {
		t.Fatalf("parse tcp = %%v,%%v", parsed, ok)
	}
	demo := IdentityCrossModuleDerivation{Proto: &proto}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoIdentityMultiBaseCrossModuleMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "identity-multi-base-cross-module"), "module"), "identity-multi-base-cross-module")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "identity-multi-base-cross-module")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "type IdentityMultiBaseCrossModuleTypeEnum int") {
		t.Fatalf("generated source should contain identityref enum, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "identity-multi-base-cross-module", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "identity-multi-base-cross-module", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedIdentityMultiBaseCrossModuleMatchesLibyang(t *testing.T) {
	typ := IdentityMultiBaseCrossModuleTypeEnumHighSpeedInterface
	parsed, ok := ParseIdentityMultiBaseCrossModuleTypeEnum("high-speed-interface")
	if !ok || parsed != typ {
		t.Fatalf("parse high-speed-interface = %%v,%%v", parsed, ok)
	}
	demo := IdentityMultiBaseCrossModule{Type_: &typ}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoIdentityrefDuplicateLocalNamesSortVariants(t *testing.T) {
	dir := t.TempDir()
	for name, source := range map[string]string{
		"z-ident.yang": `module z-ident {
    namespace "urn:z-ident";
    prefix z;
    identity base;
    identity thing { base base; }
}`,
		"a-ident.yang": `module a-ident {
    namespace "urn:a-ident";
    prefix a;
    identity base;
    identity thing { base base; }
}`,
		"identityref-variant-sort.yang": `module identityref-variant-sort {
    yang-version 1.1;
    namespace "urn:identityref-variant-sort";
    prefix ivs;
    import z-ident { prefix z; }
    import a-ident { prefix a; }
    leaf kind {
        type identityref {
            base z:base;
            base a:base;
        }
    }
}`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(source), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatalf("new context: %v", err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatalf("SetSearchPath: %v", err)
	}
	if err := ctx.LoadModule("identityref-variant-sort"); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	src, err := codegen.GenerateGo(ctx, "identityref-variant-sort")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	testBody := `
func TestIdentityrefDuplicateLocalNamesSortVariants(t *testing.T) {
	aThing, ok := ParseIdentityrefVariantSortKindEnum("a-ident:thing")
	if !ok {
		t.Fatal("Parse a-ident:thing failed")
	}
	if aThing != IdentityrefVariantSortKindEnumThing {
		t.Fatalf("a-ident:thing parsed to %v, want unsuffixed Thing variant", aThing)
	}
	zThing, ok := ParseIdentityrefVariantSortKindEnum("z-ident:thing")
	if !ok {
		t.Fatal("Parse z-ident:thing failed")
	}
	if zThing != IdentityrefVariantSortKindEnumThing2 {
		t.Fatalf("z-ident:thing parsed to %v, want suffixed Thing2 variant", zThing)
	}
	if got, want := IdentityrefVariantSortKindEnumThing.AsJSONName(), "a-ident:thing"; got != want {
		t.Fatalf("Thing AsJSONName = %q, want %q", got, want)
	}
	if got, want := IdentityrefVariantSortKindEnumThing2.AsJSONName(), "z-ident:thing"; got != want {
		t.Fatalf("Thing2 AsJSONName = %q, want %q", got, want)
	}
}
`
	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoUnionScalarAllMembersMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-union-scalar-all-members"), "module"), "types-union-scalar-all-members")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-union-scalar-all-members")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-union-scalar-all-members", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-union-scalar-all-members", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedUnionScalarAllMembersMatchesLibyang(t *testing.T) {
	e := TypesUnionScalarAllMembersTopEUnionEnumeration(TypesUnionScalarAllMembersTopEUnionEnumerationEnumBeta)
	bits, err := NewTypesUnionScalarAllMembersTopBitsUnionBitsBits([]string{"flag2", "flag1"})
	if err != nil {
		t.Fatalf("new bits: %%v", err)
	}
	iid := NewInstanceIdentifierWithXMLNS(
		"/typesunionscalarallmembers:top/typesunionscalarallmembers:s",
		"/types-union-scalar-all-members:top/s",
		"typesunionscalarallmembers",
		"urn:types-union-scalar-all-members",
	)
	demo := TypesUnionScalarAllMembers{
		Top: TypesUnionScalarAllMembersTop{
			S:    TypesUnionScalarAllMembersTopSUnionString("str"),
			B:    TypesUnionScalarAllMembersTopBUnionBoolean(false),
			I8:   TypesUnionScalarAllMembersTopI8UnionInt8(int8(-128)),
			U16:  TypesUnionScalarAllMembersTopU16UnionUint16(uint16(65000)),
			I64:  TypesUnionScalarAllMembersTopI64UnionInt64(int64(-9223372036854775808)),
			U64:  TypesUnionScalarAllMembersTopU64UnionUint64(uint64(18446744073709551615)),
			D:    TypesUnionScalarAllMembersTopDUnionDecimal64(NewDecimal64(-150, 2)),
			E:    e,
			Bits: TypesUnionScalarAllMembersTopBitsUnionBits(bits),
			Bin:  TypesUnionScalarAllMembersTopBinUnionBinary("SGVsbA=="),
			Iid:  TypesUnionScalarAllMembersTopIidUnionInstanceIdentifier(iid),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF union JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed union JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
	invalidEnum := TypesUnionScalarAllMembersTopEUnionEnumeration(TypesUnionScalarAllMembersTopEUnionEnumerationEnum(99))
	invalidEnumDemo := TypesUnionScalarAllMembers{Top: TypesUnionScalarAllMembersTop{E: invalidEnum}}
	if err := invalidEnumDemo.Validate(); err == nil {
		t.Fatal("Validate accepted invalid union enum value")
	} else if got, want := err.Error(), "/types-union-scalar-all-members/top/e: invalid enum value"; got != want {
		t.Fatalf("Validate error = %%q, want %%q", got, want)
	}
	invalidIid := NewInstanceIdentifierWithXMLNS(
		"/bad prefix:top/bad prefix:s",
		"/types-union-scalar-all-members:top/s",
		"bad prefix",
		"urn:types-union-scalar-all-members",
	)
	invalidIidDemo := TypesUnionScalarAllMembers{
		Top: TypesUnionScalarAllMembersTop{
			Iid: TypesUnionScalarAllMembersTopIidUnionInstanceIdentifier(invalidIid),
		},
	}
	if err := invalidIidDemo.Validate(); err == nil {
		t.Fatal("Validate accepted instance-identifier union value with malformed XML namespace prefix")
	}
	if _, err := FromJSONIETF([]byte(`+"`"+`{"types-union-scalar-all-members:top":{"bits":"flag1\tflag2"}}`+"`"+`)); err == nil {
		t.Fatal("FromJSONIETF accepted tab-separated union bits value")
	}
	invalidBinaryDemo := TypesUnionScalarAllMembers{Top: TypesUnionScalarAllMembersTop{Bin: TypesUnionScalarAllMembersTopBinUnionBinary("AQID")}}
	if err := invalidBinaryDemo.Validate(); err == nil {
		t.Fatal("Validate accepted length-invalid union binary value")
	} else if got, want := err.Error(), "/types-union-scalar-all-members/top/bin: binary length 3 out of bounds"; got != want {
		t.Fatalf("Validate error = %%q, want %%q", got, want)
	}
	invalidDecimalDemo := TypesUnionScalarAllMembers{Top: TypesUnionScalarAllMembersTop{D: TypesUnionScalarAllMembersTopDUnionDecimal64(NewDecimal64(-150, 3))}}
	if err := invalidDecimalDemo.Validate(); err == nil {
		t.Fatal("Validate accepted union decimal64 with wrong fraction-digits")
	} else if got, want := err.Error(), "/types-union-scalar-all-members/top/d: decimal64 fraction-digits 3, want 2"; got != want {
		t.Fatalf("Validate error = %%q, want %%q", got, want)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoUnionEnumAndScalarMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-union-enum-and-scalar"), "module"), "types-union-enum-and-scalar")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-union-enum-and-scalar")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "type TypesUnionEnumAndScalarModeUnion interface") {
		t.Fatalf("generated source should contain typed union, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-union-enum-and-scalar", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-union-enum-and-scalar", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedUnionEnumAndScalarMatchesLibyang(t *testing.T) {
	mode := TypesUnionEnumAndScalarModeUnionEnumeration(TypesUnionEnumAndScalarModeUnionEnumerationEnumAuto)
	demo := TypesUnionEnumAndScalar{Mode: mode}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoUnionMemberResolutionOrderMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-union-member-resolution-order"), "module"), "types-union-member-resolution-order")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-union-member-resolution-order")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "type TypesUnionMemberResolutionOrderCodeUnion interface") {
		t.Fatalf("generated source should contain typed union, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-union-member-resolution-order", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-union-member-resolution-order", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedUnionMemberResolutionOrderMatchesLibyang(t *testing.T) {
	demo := TypesUnionMemberResolutionOrder{Code: TypesUnionMemberResolutionOrderCodeUnionUint32(uint32(5))}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	if _, err := FromJSONIETF([]byte(`+"`"+`{"types-union-member-resolution-order:code":"5"}`+"`"+`)); err == nil {
		t.Fatal("FromJSONIETF accepted quoted JSON_IETF uint32 union value")
	} else if got, want := err.Error(), "types-union-member-resolution-order:code does not match any generated union member"; got != want {
		t.Fatalf("FromJSONIETF error = %%q, want %%q", got, want)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoUnionHeterogeneousQuotingMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-union-heterogeneous-members-quoting"), "module"), "types-union-heterogeneous-members-quoting")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-union-heterogeneous-members-quoting")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "type TypesUnionHeterogeneousMembersQuotingTopBigUnion interface") {
		t.Fatalf("generated source should contain typed union, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-union-heterogeneous-members-quoting", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-union-heterogeneous-members-quoting", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedUnionHeterogeneousQuotingMatchesLibyang(t *testing.T) {
	demo := TypesUnionHeterogeneousMembersQuoting{
		Top: TypesUnionHeterogeneousMembersQuotingTop{
			Text: TypesUnionHeterogeneousMembersQuotingTopTextUnionString("text"),
			Flag: TypesUnionHeterogeneousMembersQuotingTopFlagUnionBoolean(true),
			Big:  TypesUnionHeterogeneousMembersQuotingTopBigUnionInt64(int64(9223372036854775807)),
			Huge: TypesUnionHeterogeneousMembersQuotingTopHugeUnionUint64(uint64(18446744073709551615)),
			Rate: TypesUnionHeterogeneousMembersQuotingTopRateUnionDecimal64(NewDecimal64(314, 2)),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoUnionLeafrefMemberMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-union-leafref-member"), "module"), "types-union-leafref-member")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-union-leafref-member")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "type TypesUnionLeafrefMemberPrimaryOrStaticUnion interface") {
		t.Fatalf("generated source should contain typed union, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-union-leafref-member", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-union-leafref-member", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedUnionLeafrefMemberMatchesLibyang(t *testing.T) {
	demo := TypesUnionLeafrefMember{
		Interface_: []TypesUnionLeafrefMemberInterfaceEntry{{
			Name: "eth0",
		}},
		PrimaryOrStatic: TypesUnionLeafrefMemberPrimaryOrStaticUnionLeafref("eth0"),
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoUnionIdentityrefMemberMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-union-identityref-member"), "module"), "types-union-identityref-member")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-union-identityref-member")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "type TypesUnionIdentityrefMemberTransportUnion interface") {
		t.Fatalf("generated source should contain typed union, got:\n%s", src)
	}
	if !strings.Contains(src, "TypesUnionIdentityrefMemberTransportUnionUint16Range") {
		t.Fatalf("generated source should range-bound restricted union uint16 member, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-union-identityref-member", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-union-identityref-member", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedUnionIdentityrefMemberMatchesLibyang(t *testing.T) {
	transport := TypesUnionIdentityrefMemberTransportUnionIdentityref(TypesUnionIdentityrefMemberTransportUnionIdentityrefEnumTcp)
	demo := TypesUnionIdentityrefMember{Transport: transport}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	port, err := NewTypesUnionIdentityrefMemberTransportUnionUint16Range(443)
	if err != nil {
		t.Fatalf("new union port range: %%v", err)
	}
	numericDemo := TypesUnionIdentityrefMember{Transport: TypesUnionIdentityrefMemberTransportUnionUint16(port)}
	if err := numericDemo.Validate(); err != nil {
		t.Fatalf("Validate valid numeric union range: %%v", err)
	}
	invalid := TypesUnionIdentityrefMemberTransportUnionIdentityref(TypesUnionIdentityrefMemberTransportUnionIdentityrefEnum(99))
	invalidDemo := TypesUnionIdentityrefMember{Transport: invalid}
	if err := invalidDemo.Validate(); err == nil {
		t.Fatal("Validate accepted invalid union identityref value")
	} else if got, want := err.Error(), "/types-union-identityref-member/transport: invalid identityref value"; got != want {
		t.Fatalf("Validate error = %%q, want %%q", got, want)
	}
	invalidPortDemo := TypesUnionIdentityrefMember{Transport: TypesUnionIdentityrefMemberTransportUnionUint16(TypesUnionIdentityrefMemberTransportUnionUint16Range{})}
	if err := invalidPortDemo.Validate(); err == nil {
		t.Fatal("Validate accepted invalid union range value")
	} else if got, want := err.Error(), "/types-union-identityref-member/transport: value 0 out of range for TypesUnionIdentityrefMemberTransportUnionUint16Range"; got != want {
		t.Fatalf("Validate error = %%q, want %%q", got, want)
	}
	if _, err := FromJSONIETF([]byte(`+"`"+`{"types-union-identityref-member:transport":0}`+"`"+`)); err == nil {
		t.Fatal("FromJSONIETF accepted invalid union range value")
	} else if got, want := err.Error(), "types-union-identityref-member:transport does not match any generated union member"; got != want {
		t.Fatalf("FromJSONIETF error = %%q, want %%q", got, want)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoUnionNestedTypedefChainMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-union-nested-typedef-chain"), "module"), "types-union-nested-typedef-chain")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-union-nested-typedef-chain")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "type TypesUnionNestedTypedefChainTopExtCommUnion interface") {
		t.Fatalf("generated source should contain typed union, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-union-nested-typedef-chain", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-union-nested-typedef-chain", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedUnionNestedTypedefChainMatchesLibyang(t *testing.T) {
	demo := TypesUnionNestedTypedefChain{
		Top: TypesUnionNestedTypedefChainTop{
			ExtComm:    TypesUnionNestedTypedefChainTopExtCommUnionCommonString("65000:100"),
			ExtCommBin: TypesUnionNestedTypedefChainTopExtCommBinUnionBinary("AQIDBAUGBwg="),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF nested typedef union JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed nested typedef union JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
	invalid := TypesUnionNestedTypedefChain{Top: TypesUnionNestedTypedefChainTop{ExtComm: TypesUnionNestedTypedefChainTopExtCommUnionCommonString("invalid")}}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate accepted pattern-invalid union string")
	} else if got, want := err.Error(), "/types-union-nested-typedef-chain/top/ext-comm: pattern violation"; got != want {
		t.Fatalf("Validate error = %%q, want %%q", got, want)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoTypedefUnionCompositionMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-typedef-union-composition"), "module"), "types-typedef-union-composition")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-typedef-union-composition")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "type TypesTypedefUnionCompositionTopServerUnion interface") {
		t.Fatalf("generated source should contain typed union, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-typedef-union-composition", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-typedef-union-composition", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedTypedefUnionCompositionMatchesLibyang(t *testing.T) {
	mode := TypesTypedefUnionCompositionTopModeUnionEnumeration(TypesTypedefUnionCompositionTopModeUnionEnumerationEnumTcp)
	demo := TypesTypedefUnionComposition{
		Top: TypesTypedefUnionCompositionTop{
			Server: TypesTypedefUnionCompositionTopServerUnionString("example.com"),
			Mode:   mode,
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoTypedefSubmoduleCrossFileMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-typedef-submodule-cross-file"), "module"), "main")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "main")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "Val *Decimal64") {
		t.Fatalf("generated source should resolve submodule typedef decimal64, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-typedef-submodule-cross-file", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-typedef-submodule-cross-file", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedTypedefSubmoduleCrossFileMatchesLibyang(t *testing.T) {
	val := NewDecimal64(3141, 3)
	demo := Main{Val: &val}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoTypedefSimpleBaseMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-typedef-simple-base"), "module"), "types-typedef-simple-base")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-typedef-simple-base")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "Port *TypesTypedefSimpleBasePortRange") {
		t.Fatalf("generated source should preserve typedef range helper, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-typedef-simple-base", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-typedef-simple-base", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedTypedefSimpleBaseMatchesLibyang(t *testing.T) {
	port, err := NewTypesTypedefSimpleBasePortRange(443)
	if err != nil {
		t.Fatalf("port range: %%v", err)
	}
	demo := TypesTypedefSimpleBase{Port: &port}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoTypedefChainTwoDeepMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-typedef-chain-2deep"), "module"), "types-typedef-chain-2deep")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-typedef-chain-2deep")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "Gw *string") {
		t.Fatalf("generated source should resolve two-deep typedef chain to string pointer, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-typedef-chain-2deep", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-typedef-chain-2deep", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedTypedefChainTwoDeepMatchesLibyang(t *testing.T) {
	demo := TypesTypedefChain2deep{Gw: ptr("192.168.1.1")}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoTypedefChainThreeDeepMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-typedef-chain-3deep"), "module"), "types-typedef-chain-3deep")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-typedef-chain-3deep")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "Tsid *uint32") {
		t.Fatalf("generated source should resolve three-deep typedef chain to uint32 pointer, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-typedef-chain-3deep", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-typedef-chain-3deep", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedTypedefChainThreeDeepMatchesLibyang(t *testing.T) {
	demo := TypesTypedefChain3deep{Tsid: ptr(uint32(12345))}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoTypedefRestrictionNarrowingMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-typedef-restriction-narrowing"), "module"), "types-typedef-restriction-narrowing")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-typedef-restriction-narrowing")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "Ssh *TypesTypedefRestrictionNarrowingSshRange") {
		t.Fatalf("generated source should emit narrowed typedef range helper, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-typedef-restriction-narrowing", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-typedef-restriction-narrowing", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedTypedefRestrictionNarrowingMatchesLibyang(t *testing.T) {
	ssh, err := NewTypesTypedefRestrictionNarrowingSshRange(22)
	if err != nil {
		t.Fatalf("ssh range: %%v", err)
	}
	demo := TypesTypedefRestrictionNarrowing{Ssh: &ssh}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoTypedefDefaultInheritanceMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-typedef-default-inheritance"), "module"), "types-typedef-default-inheritance")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-typedef-default-inheritance")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "A *TypesTypedefDefaultInheritanceTopARange") || !strings.Contains(src, "B *TypesTypedefDefaultInheritanceTopBRange") {
		t.Fatalf("generated source should preserve typedef/default leaf range helpers, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-typedef-default-inheritance", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-typedef-default-inheritance", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedTypedefDefaultInheritanceMatchesLibyang(t *testing.T) {
	a, err := NewTypesTypedefDefaultInheritanceTopARange(50)
	if err != nil {
		t.Fatalf("a range: %%v", err)
	}
	b, err := NewTypesTypedefDefaultInheritanceTopBRange(75)
	if err != nil {
		t.Fatalf("b range: %%v", err)
	}
	demo := TypesTypedefDefaultInheritance{
		Top: TypesTypedefDefaultInheritanceTop{
			A: &a,
			B: &b,
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoUnionTwoIdentityrefsMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-union-two-identityrefs-distinct-bases"), "module"), "types-union-two-identityrefs-distinct-bases")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-union-two-identityrefs-distinct-bases")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "type TypesUnionTwoIdentityrefsDistinctBasesComponentTypeUnion interface") {
		t.Fatalf("generated source should contain typed union, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-union-two-identityrefs-distinct-bases", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-union-two-identityrefs-distinct-bases", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedUnionTwoIdentityrefsMatchesLibyang(t *testing.T) {
	value := TypesUnionTwoIdentityrefsDistinctBasesComponentTypeUnionIdentityref(TypesUnionTwoIdentityrefsDistinctBasesComponentTypeUnionIdentityrefEnumLinecard)
	demo := TypesUnionTwoIdentityrefsDistinctBases{ComponentType: value}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoLeafrefToListKeyMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-leafref-to-list-key"), "module"), "types-leafref-to-list-key")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-leafref-to-list-key")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-leafref-to-list-key", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-leafref-to-list-key", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedLeafrefToListKeyMatchesLibyang(t *testing.T) {
	demo := TypesLeafrefToListKey{
		Device: []TypesLeafrefToListKeyDeviceEntry{
			{
				Hostname: "router1",
				Version:  ptr("1.0"),
			},
		},
		PrimaryDevice: ptr("router1"),
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoLeafrefAbsolutePathMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-leafref-absolute-path"), "module"), "types-leafref-absolute-path")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-leafref-absolute-path")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-leafref-absolute-path", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-leafref-absolute-path", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedLeafrefAbsolutePathMatchesLibyang(t *testing.T) {
	demo := TypesLeafrefAbsolutePath{
		Top: TypesLeafrefAbsolutePathTop{
			Iface: []TypesLeafrefAbsolutePathTopIfaceEntry{{
				Name: "eth0",
			}},
		},
		PrimaryIface: ptr("eth0"),
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoLeafrefToLeafListMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-leafref-to-leaf-list"), "module"), "types-leafref-to-leaf-list")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-leafref-to-leaf-list")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-leafref-to-leaf-list", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-leafref-to-leaf-list", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedLeafrefToLeafListMatchesLibyang(t *testing.T) {
	demo := TypesLeafrefToLeafList{
		Tag:         []string{"blue", "red"},
		AssignedTag: ptr("red"),
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoLeafrefRequireInstanceFalseMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-leafref-require-instance-false"), "module"), "types-leafref-require-instance-false")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-leafref-require-instance-false")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-leafref-require-instance-false", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-leafref-require-instance-false", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedLeafrefRequireInstanceFalseMatchesLibyang(t *testing.T) {
	demo := TypesLeafrefRequireInstanceFalse{
		FutureTarget: ptr("missing"),
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoRelativeLeafrefMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-leafref-relative-parent-path"), "module"), "types-leafref-relative-parent-path")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-leafref-relative-parent-path")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-leafref-relative-parent-path", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-leafref-relative-parent-path", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedRelativeLeafrefMatchesLibyang(t *testing.T) {
	demo := TypesLeafrefRelativeParentPath{
		Iface: []TypesLeafrefRelativeParentPathIfaceEntry{{
			Name: "eth0",
			Config: TypesLeafrefRelativeParentPathIfaceEntryConfig{
				RouteId: ptr("eth0"),
			},
		}},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoLeafrefChainMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-leafref-to-leafref-chain"), "module"), "types-leafref-to-leafref-chain")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-leafref-to-leafref-chain")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-leafref-to-leafref-chain", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-leafref-to-leafref-chain", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedLeafrefChainMatchesLibyang(t *testing.T) {
	demo := TypesLeafrefToLeafrefChain{
		Device: []TypesLeafrefToLeafrefChainDeviceEntry{{
			Name: "core1",
		}},
		PrimaryDevice: ptr("core1"),
		ActiveAlias:   ptr("core1"),
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoLeafrefCurrentContextMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-leafref-current-context"), "module"), "types-leafref-current-context")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-leafref-current-context")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "ProtoName *string") {
		t.Fatalf("generated source should resolve proto-name leafref to string pointer, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-leafref-current-context", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-leafref-current-context", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedLeafrefCurrentContextMatchesLibyang(t *testing.T) {
	demo := TypesLeafrefCurrentContext{
		Proto: []TypesLeafrefCurrentContextProtoEntry{{
			Name:  "ospf",
			State: ptr("up"),
		}},
		ProtoName: ptr("ospf"),
		Selected:  ptr("ospf"),
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoLeafrefDerefMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-leafref-deref-function"), "module"), "types-leafref-deref-function")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-leafref-deref-function")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "HomeDir *string") {
		t.Fatalf("generated source should resolve home-dir leafref to string pointer, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-leafref-deref-function", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-leafref-deref-function", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedLeafrefDerefMatchesLibyang(t *testing.T) {
	demo := TypesLeafrefDerefFunction{
		User: []TypesLeafrefDerefFunctionUserEntry{{
			Uid:  "1001",
			Home: ptr("/home/alice"),
		}},
		LoggedInUser: ptr("1001"),
		HomeDir:      ptr("/home/alice"),
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoInstanceIdentifierRequireDefaultMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-instance-identifier-require-default"), "module"), "types-instance-identifier-require-default")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-instance-identifier-require-default")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "type InstanceIdentifier struct") || !strings.Contains(src, "Ref *InstanceIdentifier") {
		t.Fatalf("generated source should contain typed instance-identifier support, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-instance-identifier-require-default", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-instance-identifier-require-default", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedInstanceIdentifierRequireDefaultMatchesLibyang(t *testing.T) {
	ref := NewInstanceIdentifierWithXMLNS(
		"/typesinstanceidentifierrequiredefault:node[typesinstanceidentifierrequiredefault:id='x']",
		"/types-instance-identifier-require-default:node[id='x']",
		"typesinstanceidentifierrequiredefault",
		"urn:types-instance-identifier-require-default",
	)
	demo := TypesInstanceIdentifierRequireDefault{
		Node: []TypesInstanceIdentifierRequireDefaultNodeEntry{{
			Id: "x",
		}},
		Ref: &ref,
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
	parsed, err := FromJSONIETF([]byte(wantJSON))
	if err != nil {
		t.Fatalf("FromJSONIETF instance-identifier JSON: %%v", err)
	}
	if got := parsed.ToJSONIETF(); got != wantJSON {
		t.Fatalf("parsed instance-identifier JSON mismatch:\n got: %%q\nwant: %%q", got, wantJSON)
	}
	invalidControl := NewInstanceIdentifier("bad\x01path")
	invalidControlDemo := TypesInstanceIdentifierRequireDefault{
		Node: []TypesInstanceIdentifierRequireDefaultNodeEntry{{Id: "x"}},
		Ref:  &invalidControl,
	}
	if err := invalidControlDemo.Validate(); err == nil {
		t.Fatal("Validate accepted invalid control character in instance-identifier")
	} else if got, want := err.Error(), "/types-instance-identifier-require-default/ref: invalid string character U+0001"; got != want {
		t.Fatalf("Validate control error = %%q, want %%q", got, want)
	}
	invalidUTF8 := NewInstanceIdentifier(string([]byte{0xff}))
	invalidUTF8Demo := TypesInstanceIdentifierRequireDefault{
		Node: []TypesInstanceIdentifierRequireDefaultNodeEntry{{Id: "x"}},
		Ref:  &invalidUTF8,
	}
	if err := invalidUTF8Demo.Validate(); err == nil {
		t.Fatal("Validate accepted invalid UTF-8 in instance-identifier")
	} else if got, want := err.Error(), "/types-instance-identifier-require-default/ref: string value is not valid UTF-8"; got != want {
		t.Fatalf("Validate UTF-8 error = %%q, want %%q", got, want)
	}
	quotedNS := NewInstanceIdentifierWithXMLNS(
		"/p:node[p:id='x']",
		"/types-instance-identifier-require-default:node[id='x']",
		"p",
		`+"`"+`urn:test"quoted&active`+"`"+`,
	)
	quotedNSDemo := TypesInstanceIdentifierRequireDefault{
		Node: []TypesInstanceIdentifierRequireDefaultNodeEntry{{Id: "x"}},
		Ref:  &quotedNS,
	}
	wantQuotedNSXML := "<node xmlns=\"urn:types-instance-identifier-require-default\">\n  <id>x</id>\n</node>\n<ref xmlns=\"urn:types-instance-identifier-require-default\" xmlns:p=\"urn:test&quot;quoted&amp;active\">/p:node[p:id='x']</ref>\n"
	if got := quotedNSDemo.ToXML(); got != wantQuotedNSXML {
		t.Fatalf("instance-identifier XML namespace escaping mismatch:\n got: %%q\nwant: %%q", got, wantQuotedNSXML)
	}
	invalidPrefix := NewInstanceIdentifierWithXMLNS(
		"/bad prefix:node[bad prefix:id='x']",
		"/types-instance-identifier-require-default:node[id='x']",
		"bad prefix",
		"urn:types-instance-identifier-require-default",
	)
	invalidPrefixDemo := TypesInstanceIdentifierRequireDefault{
		Node: []TypesInstanceIdentifierRequireDefaultNodeEntry{{Id: "x"}},
		Ref:  &invalidPrefix,
	}
	if err := invalidPrefixDemo.Validate(); err == nil {
		t.Fatal("Validate accepted instance-identifier with malformed XML namespace prefix")
	}
	wantInvalidPrefixXML := "<node xmlns=\"urn:types-instance-identifier-require-default\">\n  <id>x</id>\n</node>\n<ref xmlns=\"urn:types-instance-identifier-require-default\">/bad prefix:node[bad prefix:id='x']</ref>\n"
	if got := invalidPrefixDemo.ToXML(); got != wantInvalidPrefixXML {
		t.Fatalf("ToXML emitted unsafe instance-identifier namespace:\n got: %%q\nwant: %%q", got, wantInvalidPrefixXML)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoInstanceIdentifierNoRequireMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-instance-identifier-no-require"), "module"), "types-instance-identifier-no-require")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-instance-identifier-no-require")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "AnyPath *InstanceIdentifier") {
		t.Fatalf("generated source should contain typed instance-identifier field, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-instance-identifier-no-require", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-instance-identifier-no-require", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedInstanceIdentifierNoRequireMatchesLibyang(t *testing.T) {
	anyPath := NewInstanceIdentifierWithXMLNS(
		"/typesinstanceidentifiernorequire:arbitrary/typesinstanceidentifiernorequire:path[typesinstanceidentifiernorequire:key='value']",
		"/types-instance-identifier-no-require:arbitrary/path[key='value']",
		"typesinstanceidentifiernorequire",
		"urn:types-instance-identifier-no-require",
	)
	demo := TypesInstanceIdentifierNoRequire{AnyPath: &anyPath}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoInstanceIdentifierComplexPathMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "types-instance-identifier-complex-path"), "module"), "types-instance-identifier-complex-path")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "types-instance-identifier-complex-path")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "SelectedMember *InstanceIdentifier") {
		t.Fatalf("generated source should contain typed instance-identifier field, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "types-instance-identifier-complex-path", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "types-instance-identifier-complex-path", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedInstanceIdentifierComplexPathMatchesLibyang(t *testing.T) {
	selected := NewInstanceIdentifierWithXMLNS(
		"/typesinstanceidentifiercomplexpath:vlan[typesinstanceidentifiercomplexpath:id='100'][typesinstanceidentifiercomplexpath:name='mgmt']/typesinstanceidentifiercomplexpath:member[typesinstanceidentifiercomplexpath:port='eth0']",
		"/types-instance-identifier-complex-path:vlan[id='100'][name='mgmt']/member[port='eth0']",
		"typesinstanceidentifiercomplexpath",
		"urn:types-instance-identifier-complex-path",
	)
	demo := TypesInstanceIdentifierComplexPath{
		Vlan: []TypesInstanceIdentifierComplexPathVlanEntry{{
			Id:   100,
			Name: "mgmt",
			Member: []TypesInstanceIdentifierComplexPathVlanEntryMemberEntry{{
				Port: "eth0",
			}},
		}},
		SelectedMember: &selected,
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoChoiceSingleNodeCaseMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "choice-single-node-case"), "module"), "choice-single-node-case")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "choice-single-node-case")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "Name *string") {
		t.Fatalf("generated source should flatten top-level choice leaves, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "choice-single-node-case", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "choice-single-node-case", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedChoiceSingleNodeCaseMatchesLibyang(t *testing.T) {
	demo := ChoiceSingleNodeCase{Name: ptr("new-entry")}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoChoiceInterleavedSiblingsMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "choice-cases-interleaved-siblings"), "module"), "choice-cases-interleaved-siblings")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "choice-cases-interleaved-siblings")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "ChoiceCasesInterleavedSiblingsRuleFieldOrder = []string{\"id\", \"priority\", \"reason\", \"description\"}") {
		t.Fatalf("generated source should flatten choice at declaration site, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "choice-cases-interleaved-siblings", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "choice-cases-interleaved-siblings", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedChoiceInterleavedSiblingsMatchesLibyang(t *testing.T) {
	priority := uint8(10)
	demo := ChoiceCasesInterleavedSiblings{
		Rule: ChoiceCasesInterleavedSiblingsRule{
			Id:          ptr("r1"),
			Priority:    &priority,
			Description: ptr("allow"),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoListEntryChoiceOrderMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "list-entry-with-choice-schema-order"), "module"), "list-entry-with-choice-schema-order")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "list-entry-with-choice-schema-order")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "ListEntryWithChoiceSchemaOrderVlanEntryFieldOrder = []string{\"vlan-id\", \"tag\", \"untag\"}") {
		t.Fatalf("generated source should keep list key first and flatten choice leaves, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "list-entry-with-choice-schema-order", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "list-entry-with-choice-schema-order", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedListEntryChoiceOrderMatchesLibyang(t *testing.T) {
	demo := ListEntryWithChoiceSchemaOrder{
		Vlan: []ListEntryWithChoiceSchemaOrderVlanEntry{{
			VlanId: 10,
			Untag:  &struct{}{},
		}},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoChoiceNestedInCaseMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "choice-nested-in-case"), "module"), "choice-nested-in-case")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "choice-nested-in-case")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "Metric *uint8") || !strings.Contains(src, "Area *uint32") || !strings.Contains(src, "Prefix *string") {
		t.Fatalf("generated source should flatten nested choice/case leaves, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "choice-nested-in-case", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "choice-nested-in-case", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedChoiceNestedInCaseMatchesLibyang(t *testing.T) {
	demo := ChoiceNestedInCase{Metric: ptr(uint8(2))}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoChoiceShorthandLeaflistListMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "choice-shorthand-leaflist-list"), "module"), "choice-shorthand-leaflist-list")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "choice-shorthand-leaflist-list")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "Tags []string") || !strings.Contains(src, "Rows []ChoiceShorthandLeaflistListRowsEntry") {
		t.Fatalf("generated source should flatten shorthand leaf-list/list choice branches, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "choice-shorthand-leaflist-list", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "choice-shorthand-leaflist-list", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedChoiceShorthandLeaflistListMatchesLibyang(t *testing.T) {
	demo := ChoiceShorthandLeaflistList{Tags: []string{"blue", "red"}}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoChoiceWithLeaflistBranchMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "choice-with-leaflist-branch"), "module"), "choice-with-leaflist-branch")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "choice-with-leaflist-branch")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "Paths UserOrderedVec[string]") || !strings.Contains(src, "Facility *string") {
		t.Fatalf("generated source should flatten user-ordered leaf-list choice branch, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "choice-with-leaflist-branch", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "choice-with-leaflist-branch", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedChoiceWithLeaflistBranchMatchesLibyang(t *testing.T) {
	demo := ChoiceWithLeaflistBranch{
		Paths: NewUserOrderedVec([]string{"/var/log/p3", "/var/log/p1", "/var/log/p2"}),
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoChoiceMultipleCasesDefaultMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "choice-multiple-cases-default"), "module"), "choice-multiple-cases-default")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "choice-multiple-cases-default")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "PassString *string") || !strings.Contains(src, "KeyData *string") || !strings.Contains(src, "TokenValue *string") {
		t.Fatalf("generated source should flatten every default-choice case, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "choice-multiple-cases-default", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "choice-multiple-cases-default", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedChoiceMultipleCasesDefaultMatchesLibyang(t *testing.T) {
	demo := ChoiceMultipleCasesDefault{TokenValue: ptr("tok-123")}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoChoiceDefaultCaseWithDefaultsOnlyWhenSelected(t *testing.T) {
	const source = `module choice-default-codegen {
    namespace "urn:choice-default-codegen";
    prefix cdc;

    choice transport {
        default syslog;
        case syslog {
            leaf severity {
                type string;
                default "info";
            }
        }
        case token {
            leaf token {
                type string;
            }
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	src, err := codegen.GenerateGo(ctx, "choice-default-codegen")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	testBody := `
func TestGeneratedChoiceDefaultCaseWithDefaultsOnlyWhenSelected(t *testing.T) {
	empty := ChoiceDefaultCodegen{}
	if got, want := empty.ToJSONIETFWithDefaults(WithDefaultsAll), "{\n  \"choice-default-codegen:severity\": \"info\"\n}\n"; got != want {
		t.Fatalf("default case all JSON mismatch:\n got: %q\nwant: %q", got, want)
	}

	alternate := ChoiceDefaultCodegen{Token: ptr("tok-123")}
	if got, want := alternate.ToJSONIETFWithDefaults(WithDefaultsAll), "{\n  \"choice-default-codegen:token\": \"tok-123\"\n}\n"; got != want {
		t.Fatalf("alternate case all JSON mismatch:\n got: %q\nwant: %q", got, want)
	}
}
`

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoChoiceDefaultCaseContainerWithDefaultsOnlyWhenSelected(t *testing.T) {
	const source = `module choice-default-container-codegen {
    namespace "urn:choice-default-container-codegen";
    prefix cdcc;

    choice transport {
        default syslog;
        case syslog {
            container settings {
                leaf severity {
                    type string;
                    default "info";
                }
            }
        }
        case token {
            leaf token {
                type string;
            }
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	src, err := codegen.GenerateGo(ctx, "choice-default-container-codegen")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	testBody := `
func TestGeneratedChoiceDefaultCaseContainerWithDefaultsOnlyWhenSelected(t *testing.T) {
	empty := ChoiceDefaultContainerCodegen{}
	if got, want := empty.ToJSONIETFWithDefaults(WithDefaultsAll), "{\n  \"choice-default-container-codegen:settings\": {\n    \"severity\": \"info\"\n  }\n}\n"; got != want {
		t.Fatalf("default case container all JSON mismatch:\n got: %q\nwant: %q", got, want)
	}

	alternate := ChoiceDefaultContainerCodegen{Token: ptr("tok-123")}
	if got, want := alternate.ToJSONIETFWithDefaults(WithDefaultsAll), "{\n  \"choice-default-container-codegen:token\": \"tok-123\"\n}\n"; got != want {
		t.Fatalf("alternate case all JSON mismatch:\n got: %q\nwant: %q", got, want)
	}
}
`

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoChoiceNonDefaultCaseDefaultsDoNotCreateContainer(t *testing.T) {
	const source = `module choice-nondefault-container-codegen {
    namespace "urn:choice-nondefault-container-codegen";
    prefix cncc;

    container top {
        choice transport {
            case syslog {
                container settings {
                    leaf severity {
                        type string;
                        default "info";
                    }
                }
            }
            case token {
                leaf token {
                    type string;
                }
            }
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	src, err := codegen.GenerateGo(ctx, "choice-nondefault-container-codegen")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	testBody := `
func TestGeneratedChoiceNonDefaultCaseDefaultsDoNotCreateContainer(t *testing.T) {
	empty := ChoiceNondefaultContainerCodegen{}
	if got, want := empty.ToJSONIETFWithDefaults(WithDefaultsAll), "{\n}\n"; got != want {
		t.Fatalf("unselected non-default case all JSON mismatch:\n got: %q\nwant: %q", got, want)
	}

	alternate := ChoiceNondefaultContainerCodegen{
		Top: ChoiceNondefaultContainerCodegenTop{Token: ptr("tok-123")},
	}
	if got, want := alternate.ToJSONIETFWithDefaults(WithDefaultsAll), "{\n  \"choice-nondefault-container-codegen:top\": {\n    \"token\": \"tok-123\"\n  }\n}\n"; got != want {
		t.Fatalf("alternate case all JSON mismatch:\n got: %q\nwant: %q", got, want)
	}
}
`

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoChoiceDefaultCaseMandatoryDescendantRejects(t *testing.T) {
	const source = `module choice-default-mandatory-codegen {
    namespace "urn:choice-default-mandatory-codegen";
    prefix cdmc;

    choice auth {
        default password;
        case password {
            leaf username {
                mandatory true;
                type string;
            }
        }
        case token {
            leaf token {
                type string;
            }
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	src, err := codegen.GenerateGo(ctx, "choice-default-mandatory-codegen")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	testBody := `
func TestGeneratedChoiceDefaultCaseMandatoryDescendantRejects(t *testing.T) {
	empty := ChoiceDefaultMandatoryCodegen{}
	err := empty.Validate()
	if err == nil {
		t.Fatal("Validate accepted missing mandatory descendant in default choice case")
	}
	if got, want := err.Error(), "/choice-default-mandatory-codegen/auth/password/username: missing mandatory field"; got != want {
		t.Fatalf("Validate error = %q, want %q", got, want)
	}
	if _, err := FromJSONIETF([]byte("{}")); err == nil {
		t.Fatal("FromJSONIETF accepted missing mandatory descendant in default choice case")
	} else if got, want := err.Error(), "/choice-default-mandatory-codegen/auth/password/username: missing mandatory field"; got != want {
		t.Fatalf("FromJSONIETF error = %q, want %q", got, want)
	}

	alternate := ChoiceDefaultMandatoryCodegen{Token: ptr("tok-123")}
	if err := alternate.Validate(); err != nil {
		t.Fatalf("Validate rejected alternate case: %v", err)
	}
	if _, err := FromJSONIETF([]byte("{\"choice-default-mandatory-codegen:token\":\"tok-123\"}")); err != nil {
		t.Fatalf("FromJSONIETF rejected alternate case: %v", err)
	}

	selectedDefault := ChoiceDefaultMandatoryCodegen{Username: ptr("alice")}
	if err := selectedDefault.Validate(); err != nil {
		t.Fatalf("Validate rejected default case with mandatory descendant: %v", err)
	}
}
`

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoGroupingSimpleMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "linkage-grouping-simple"), "module"), "linkage-grouping-simple")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "linkage-grouping-simple")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "LinkageGroupingSimpleIntfFieldOrder = []string{\"name\", \"ip\", \"mask\", \"mtu\"}") {
		t.Fatalf("generated source should expand grouping at uses site, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "linkage-grouping-simple", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "linkage-grouping-simple", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedGroupingSimpleMatchesLibyang(t *testing.T) {
	demo := LinkageGroupingSimple{
		Intf: LinkageGroupingSimpleIntf{
			Name: ptr("eth0"),
			Ip:   ptr("192.0.2.1"),
			Mask: ptr(uint8(24)),
			Mtu:  ptr(uint16(1500)),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoGroupingNestedUsesMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "linkage-grouping-nested-uses"), "module"), "linkage-grouping-nested-uses")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "linkage-grouping-nested-uses")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "LinkageGroupingNestedUsesIntfFieldOrder = []string{\"name\", \"ip\", \"mask\", \"gateway\"}") {
		t.Fatalf("generated source should expand nested uses at uses site, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "linkage-grouping-nested-uses", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "linkage-grouping-nested-uses", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedGroupingNestedUsesMatchesLibyang(t *testing.T) {
	demo := LinkageGroupingNestedUses{
		Intf: LinkageGroupingNestedUsesIntf{
			Name:    ptr("eth0"),
			Ip:      ptr("192.0.2.1"),
			Mask:    ptr(uint8(24)),
			Gateway: ptr("192.0.2.254"),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoGroupingCrossModuleMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "linkage-grouping-cross-module"), "module"), "linkage-grouping-cross-module")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "linkage-grouping-cross-module")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "LinkageGroupingCrossModuleIntfFieldOrder = []string{\"ip\", \"mask\"}") {
		t.Fatalf("generated source should expand imported grouping, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "linkage-grouping-cross-module", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "linkage-grouping-cross-module", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedGroupingCrossModuleMatchesLibyang(t *testing.T) {
	demo := LinkageGroupingCrossModule{
		Intf: LinkageGroupingCrossModuleIntf{
			Ip:   ptr("192.0.2.1"),
			Mask: ptr(uint8(24)),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoRefineMandatoryConfigMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "linkage-refine-mandatory-config"), "module"), "linkage-refine-mandatory-config")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "linkage-refine-mandatory-config")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "Name string") || !strings.Contains(src, "Secret *string") {
		t.Fatalf("generated source should apply refine mandatory/config metadata, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "linkage-refine-mandatory-config", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "linkage-refine-mandatory-config", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedRefineMandatoryConfigMatchesLibyang(t *testing.T) {
	demo := LinkageRefineMandatoryConfig{
		Service: LinkageRefineMandatoryConfigService{
			Name: "demo",
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoAugmentIntraModuleMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "linkage-augment-intra-module"), "module"), "linkage-augment-intra-module")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "linkage-augment-intra-module")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "LinkageAugmentIntraModuleInterfacesInterfaceEntryFieldOrder = []string{\"name\", \"type\", \"speed\"}") {
		t.Fatalf("generated source should include intra-module augment at target site, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "linkage-augment-intra-module", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "linkage-augment-intra-module", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedAugmentIntraModuleMatchesLibyang(t *testing.T) {
	demo := LinkageAugmentIntraModule{
		Interfaces: LinkageAugmentIntraModuleInterfaces{
			Interface_: []LinkageAugmentIntraModuleInterfacesInterfaceEntry{{
				Name:   "eth0",
				Type_:  ptr("eth"),
				Speed:  ptr(uint32(1000)),
			}},
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoAugmentInterModuleMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "linkage-augment-inter-module"), "module"), "linkage-augment-inter-module")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "linkage-augment-inter-module-base")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "LinkageAugmentInterModuleBaseInterfacesInterfaceEntryFieldOrder = []string{\"name\", \"type\", \"speed\"}") {
		t.Fatalf("generated source should include inter-module augment at target site, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "linkage-augment-inter-module", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "linkage-augment-inter-module", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedAugmentInterModuleMatchesLibyang(t *testing.T) {
	demo := LinkageAugmentInterModuleBase{
		Interfaces: LinkageAugmentInterModuleBaseInterfaces{
			Interface_: []LinkageAugmentInterModuleBaseInterfacesInterfaceEntry{{
				Name:   "eth0",
				Type_:  ptr("eth"),
				Speed:  ptr(uint32(1000)),
			}},
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoAugmentContainerLeafListMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "linkage-augment-container-leaf-list"), "module"), "linkage-augment-container-leaf-list")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "linkage-augment-container-leaf-list")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "LinkageAugmentContainerLeafListTopFieldOrder = []string{\"id\", \"stats\", \"mtu\", \"tags\"}") {
		t.Fatalf("generated source should place augment container/leaf/leaf-list at target site, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "linkage-augment-container-leaf-list", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "linkage-augment-container-leaf-list", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedAugmentContainerLeafListMatchesLibyang(t *testing.T) {
	demo := LinkageAugmentContainerLeafList{
		Top: LinkageAugmentContainerLeafListTop{
			Id: ptr("node-1"),
			Stats: LinkageAugmentContainerLeafListTopStats{
				Packets: ptr(uint64(64)),
				Bytes:   ptr(uint64(4096)),
			},
			Mtu:  ptr(uint16(9000)),
			Tags: []string{"red", "blue"},
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoAugmentNestedMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "linkage-augment-nested"), "module"), "linkage-augment-nested")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "linkage-augment-nested")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "LinkageAugmentNestedDevStatusFieldOrder = []string{\"ready\", \"timestamp\"}") {
		t.Fatalf("generated source should apply nested augment at nested target site, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "linkage-augment-nested", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "linkage-augment-nested", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedAugmentNestedMatchesLibyang(t *testing.T) {
	demo := LinkageAugmentNested{
		Dev: LinkageAugmentNestedDev{
			Id: ptr("dev-1"),
			Status: LinkageAugmentNestedDevStatus{
				Ready:     ptr(true),
				Timestamp: ptr("2026-06-15T00:00:00Z"),
			},
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoAugmentChoiceCaseMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "linkage-augment-choice-case"), "module"), "linkage-augment-choice-case")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "linkage-augment-choice-case")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "LinkageAugmentChoiceCaseConfigFieldOrder = []string{\"tcp-port\", \"udp-port\", \"timeout\", \"quic-port\"}") {
		t.Fatalf("generated source should flatten augment choice/case descendants in effective order, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "linkage-augment-choice-case", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "linkage-augment-choice-case", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedAugmentChoiceCaseMatchesLibyang(t *testing.T) {
	demo := LinkageAugmentChoiceCase{
		Config: LinkageAugmentChoiceCaseConfig{
			UdpPort: ptr(uint16(5000)),
			Timeout: ptr(uint32(30)),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoAugmentCrossModuleIdentCollisionMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "augment-cross-module-ident-collision"), "module"), "augment-cross-module-ident-collision")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "augment-cross-module-ident-collision-base")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "AugmentCrossModuleIdentCollisionBaseTopFieldOrder = []string{\"config\", \"status\", \"config-extra\", \"config-status\"}") {
		t.Fatalf("generated source should preserve base/augment identifier collisions in effective order, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "augment-cross-module-ident-collision", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "augment-cross-module-ident-collision", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedAugmentCrossModuleIdentCollisionMatchesLibyang(t *testing.T) {
	demo := AugmentCrossModuleIdentCollisionBase{
		Top: AugmentCrossModuleIdentCollisionBaseTop{
			Config: AugmentCrossModuleIdentCollisionBaseTopConfig{
				V: ptr("cfg"),
			},
			Status: ptr("up"),
			ConfigExtra: AugmentCrossModuleIdentCollisionBaseTopConfigExtra{
				W: ptr("extra"),
			},
			ConfigStatus: ptr("ok"),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoAugmentWhenTargetContextMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "augment-when-target-context"), "module"), "augment-when-target-context")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "augment-when-target-context-base")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "AugmentWhenTargetContextBaseSystemOspfFieldOrder = []string{\"router-id\", \"area\"}") {
		t.Fatalf("generated source should emit augment with target-context when metadata at target site, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "augment-when-target-context", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "augment-when-target-context", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedAugmentWhenTargetContextMatchesLibyang(t *testing.T) {
	demo := AugmentWhenTargetContextBase{
		System: AugmentWhenTargetContextBaseSystem{
			Mode: ptr("enabled"),
			Ospf: AugmentWhenTargetContextBaseSystemOspf{
				RouterId: ptr("1.1.1.1"),
				Area:     ptr("0"),
			},
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoConditionalMandatoryAugmentIsOptional(t *testing.T) {
	const base = `module mandatory-augment-codegen-base {
    yang-version 1.1;
    namespace "urn:mandatory-augment-codegen-base";
    prefix macb;

    container top {
        leaf mode { type string; }
    }
}`
	const augment = `module mandatory-augment-codegen {
    yang-version 1.1;
    namespace "urn:mandatory-augment-codegen";
    prefix mac;

    import mandatory-augment-codegen-base {
        prefix base;
    }

    augment "/base:top" {
        when "base:mode = 'enabled'";
        leaf required-name {
            mandatory true;
            type string;
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(base); err != nil {
		t.Fatalf("LoadModuleStr base: %v", err)
	}
	if err := builder.LoadModuleStr(augment); err != nil {
		t.Fatalf("LoadModuleStr augment: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	src, err := codegen.GenerateGo(ctx, "mandatory-augment-codegen-base")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "RequiredName *string") {
		t.Fatalf("conditional mandatory augmented leaf should be optional in generated source, got:\n%s", src)
	}

	testBody := `
func TestGeneratedConditionalMandatoryAugmentIsOptional(t *testing.T) {
	demo := MandatoryAugmentCodegenBase{
		Top: MandatoryAugmentCodegenBaseTop{
			Mode: ptr("disabled"),
		},
	}
	if err := demo.Validate(); err != nil {
		t.Fatalf("Validate without conditional mandatory augment leaf: %v", err)
	}
	if _, err := FromJSONIETF([]byte("{\"mandatory-augment-codegen-base:top\":{\"mode\":\"disabled\"}}")); err != nil {
		t.Fatalf("FromJSONIETF without conditional mandatory augment leaf: %v", err)
	}
}
`

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoCrossModuleAugmentDeviationWhenMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "json-ietf-cross-module-augment-deviation-when"), "module"), "json-ietf-cross-module-aug")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "json-ietf-cross-module-base")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "CustomField *string") || !strings.Contains(src, "Counter *JsonIetfCrossModuleBaseInterfacesInterfaceEntryCounterRange") {
		t.Fatalf("generated source should combine cross-module augment and deviation, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "json-ietf-cross-module-augment-deviation-when", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "json-ietf-cross-module-augment-deviation-when", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedCrossModuleAugmentDeviationWhenMatchesLibyang(t *testing.T) {
	counter, err := NewJsonIetfCrossModuleBaseInterfacesInterfaceEntryCounterRange(500)
	if err != nil {
		t.Fatalf("counter range: %%v", err)
	}
	demo := JsonIetfCrossModuleBase{
		Interfaces: JsonIetfCrossModuleBaseInterfaces{
			Interface_: []JsonIetfCrossModuleBaseInterfacesInterfaceEntry{{
				Name:        "eth0",
				Counter:     &counter,
				CustomField: ptr("abc"),
			}},
		},
		System: JsonIetfCrossModuleBaseSystem{
			Mode:   ptr("basic"),
			Secret: ptr("shh"),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoDeviationNotSupportedMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "linkage-deviation-not-supported"), "module"), "linkage-deviation-not-supported-dev")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "linkage-deviation-not-supported-base")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if strings.Contains(src, "DeprecatedField") || !strings.Contains(src, "ActiveField *string") {
		t.Fatalf("generated source should apply not-supported deviation, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "linkage-deviation-not-supported", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "linkage-deviation-not-supported", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedDeviationNotSupportedMatchesLibyang(t *testing.T) {
	demo := LinkageDeviationNotSupportedBase{
		C: LinkageDeviationNotSupportedBaseC{
			ActiveField: ptr("ok"),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoDeviationAddMandatoryMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "linkage-deviation-add"), "module"), "linkage-deviation-add-dev")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "linkage-deviation-add-base")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "OptionalName string") {
		t.Fatalf("generated source should apply mandatory add deviation, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "linkage-deviation-add", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "linkage-deviation-add", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedDeviationAddMandatoryMatchesLibyang(t *testing.T) {
	demo := LinkageDeviationAddBase{
		C: LinkageDeviationAddBaseC{
			OptionalName: "present",
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoDeviationDeleteDefaultMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "linkage-deviation-delete"), "module"), "linkage-deviation-delete-dev")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "linkage-deviation-delete-base")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "C *LinkageDeviationDeleteBaseC") {
		t.Fatalf("generated source should keep deviated presence container, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "linkage-deviation-delete", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "linkage-deviation-delete", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedDeviationDeleteDefaultMatchesLibyang(t *testing.T) {
	demo := LinkageDeviationDeleteBase{C: &LinkageDeviationDeleteBaseC{}}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoDeviationReplaceTypeMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "linkage-deviation-replace-type"), "module"), "linkage-deviation-replace-type-dev")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "linkage-deviation-replace-type-base")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "Count *LinkageDeviationReplaceTypeBaseCCountRange") {
		t.Fatalf("generated source should apply replace type/range deviation, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "linkage-deviation-replace-type", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "linkage-deviation-replace-type", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedDeviationReplaceTypeMatchesLibyang(t *testing.T) {
	count, err := NewLinkageDeviationReplaceTypeBaseCCountRange(500)
	if err != nil {
		t.Fatalf("count range: %%v", err)
	}
	demo := LinkageDeviationReplaceTypeBase{
		C: LinkageDeviationReplaceTypeBaseC{
			Count: &count,
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoDeviationReplaceDefaultConfigMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "linkage-deviation-replace-default-config"), "module"), "linkage-deviation-replace-default-config-dev")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "linkage-deviation-replace-default-config-base")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "RouterId *string") || !strings.Contains(src, "Mode *string") {
		t.Fatalf("generated source should keep replaced default/config leaves, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "linkage-deviation-replace-default-config", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "linkage-deviation-replace-default-config", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedDeviationReplaceDefaultConfigMatchesLibyang(t *testing.T) {
	demo := LinkageDeviationReplaceDefaultConfigBase{
		System: &LinkageDeviationReplaceDefaultConfigBaseSystem{
			Mode: ptr("enabled"),
			Ospf: LinkageDeviationReplaceDefaultConfigBaseSystemOspf{
				RouterId: ptr("1.1.1.1"),
			},
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoDeviationMultiMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "linkage-deviation-multi"), "module"), "linkage-deviation-multi-dev")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "linkage-deviation-multi-base")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if strings.Contains(src, "Legacy") || !strings.Contains(src, "Maximum *LinkageDeviationMultiBaseCMaximumRange") || !strings.Contains(src, "Name string") {
		t.Fatalf("generated source should apply combined deviations, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "linkage-deviation-multi", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "linkage-deviation-multi", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedDeviationMultiMatchesLibyang(t *testing.T) {
	maximum, err := NewLinkageDeviationMultiBaseCMaximumRange(500)
	if err != nil {
		t.Fatalf("maximum range: %%v", err)
	}
	demo := LinkageDeviationMultiBase{
		C: LinkageDeviationMultiBaseC{
			Maximum: &maximum,
			Name:    "valid",
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoImportPrefixMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "linkage-import-prefix"), "module"), "linkage-import-prefix")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "linkage-import-prefix")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "Port *LinkageImportPrefixPortRange") {
		t.Fatalf("generated source should resolve imported prefixed typedef range, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "linkage-import-prefix", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "linkage-import-prefix", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedImportPrefixMatchesLibyang(t *testing.T) {
	port, err := NewLinkageImportPrefixPortRange(8080)
	if err != nil {
		t.Fatalf("port range: %%v", err)
	}
	demo := LinkageImportPrefix{Port: &port}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoImportMultipleMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "linkage-import-multiple"), "module"), "linkage-import-multiple")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "linkage-import-multiple")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "LinkageImportMultipleObjFieldOrder = []string{\"id\", \"name\", \"mode\"}") {
		t.Fatalf("generated source should keep imported typedef children in declaration order, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "linkage-import-multiple", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "linkage-import-multiple", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedImportMultipleMatchesLibyang(t *testing.T) {
	demo := LinkageImportMultiple{
		Obj: LinkageImportMultipleObj{
			Id:   ptr("42"),
			Name: ptr("test"),
			Mode: ptr("active"),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoImportRevisionDateMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "linkage-import-revision-date"), "module"), "linkage-import-revision-date")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "linkage-import-revision-date")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "OldField *string") {
		t.Fatalf("generated source should resolve revision-pinned imported typedef, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "linkage-import-revision-date", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "linkage-import-revision-date", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedImportRevisionDateMatchesLibyang(t *testing.T) {
	demo := LinkageImportRevisionDate{OldField: ptr("legacy-value")}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoSubmoduleSimpleMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "linkage-submodule-simple"), "module"), "linkage-submodule-simple")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "linkage-submodule-simple")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "LinkageSubmoduleSimpleAaaFieldOrder = []string{\"username\", \"password\"}") {
		t.Fatalf("generated source should expand submodule grouping in declaration order, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "linkage-submodule-simple", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "linkage-submodule-simple", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedSubmoduleSimpleMatchesLibyang(t *testing.T) {
	demo := LinkageSubmoduleSimple{
		Aaa: LinkageSubmoduleSimpleAaa{
			Username: ptr("admin"),
			Password: ptr("hunter2"),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoSubmoduleMultiMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "linkage-submodule-multi"), "module"), "linkage-submodule-multi")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "linkage-submodule-multi")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "LinkageSubmoduleMultiDeviceFieldOrder = []string{\"id\", \"online\"}") {
		t.Fatalf("generated source should resolve nested submodule include typedefs/groupings, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "linkage-submodule-multi", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "linkage-submodule-multi", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedSubmoduleMultiMatchesLibyang(t *testing.T) {
	demo := LinkageSubmoduleMulti{
		Device: LinkageSubmoduleMultiDevice{
			Id:     ptr("dev-1"),
			Online: ptr(true),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoSubmoduleImportsForeignMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "linkage-submodule-imports-foreign"), "module"), "linkage-submodule-imports-foreign")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "linkage-submodule-imports-foreign")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "LinkageSubmoduleImportsForeignAaaFieldOrder = []string{\"server\", \"secret\"}") {
		t.Fatalf("generated source should resolve submodule-owned foreign imports, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "linkage-submodule-imports-foreign", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "linkage-submodule-imports-foreign", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedSubmoduleImportsForeignMatchesLibyang(t *testing.T) {
	demo := LinkageSubmoduleImportsForeign{
		Aaa: LinkageSubmoduleImportsForeignAaa{
			Server: ptr("192.0.2.1"),
			Secret: ptr("shared"),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoPresenceVsNonpresenceJSONMatchesLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "json-ietf-presence-vs-nonpresence"), "module"), "json-ietf-presence-vs-nonpresence")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "json-ietf-presence-vs-nonpresence")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	wantJSON, err := os.ReadFile(goldenPath(t, "json-ietf-presence-vs-nonpresence", "output.json_ietf"))
	if err != nil {
		t.Fatalf("read golden json_ietf: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedPresenceVsNonpresenceJSONMatchesLibyang(t *testing.T) {
	demo := JsonIetfPresenceVsNonpresence{
		Top: JsonIetfPresenceVsNonpresenceTop{
			Ssh: &JsonIetfPresenceVsNonpresenceTopSsh{},
		},
	}
	got := demo.ToJSONIETF()
	want := %q
	if got != want {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", got, want)
	}
}
`, string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoKeywordIdentifiersCompileAndMatchLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "idents-keywords-go"), "module"), "idents-keywords-go")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "idents-keywords-go")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "idents-keywords-go", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "idents-keywords-go", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedKeywordIdentifiersCompileAndMatchLibyang(t *testing.T) {
	demo := IdentsKeywordsGo{
		Top: IdentsKeywordsGoTop{
			Func_:    ptr("f"),
			Range_:   ptr("r"),
			Chan_:    ptr("c"),
			Map_:     ptr("m"),
			Select_:  ptr("s"),
			Go_:      ptr("g"),
			Defer_:   ptr("d"),
			Interface_: IdentsKeywordsGoTopInterface{
				Enabled: &struct{}{},
			},
			Package_: ptr("p"),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoIdentifierCollisionsCompileAndMatchLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "idents-collision-hyphen-underscore"), "module"), "idents-collision-hyphen-underscore")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "idents-collision-hyphen-underscore")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "idents-collision-hyphen-underscore", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "idents-collision-hyphen-underscore", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedIdentifierCollisionsCompileAndMatchLibyang(t *testing.T) {
	demo := IdentsCollisionHyphenUnderscore{
		Top: IdentsCollisionHyphenUnderscoreTop{
			FooBar:  ptr("a"),
			FooBar2: ptr("b"),
			BazQux: IdentsCollisionHyphenUnderscoreTopBazQux{
				Enabled: &struct{}{},
			},
			BazQux2: IdentsCollisionHyphenUnderscoreTopBazQux2{
				Status: ptr("ok"),
			},
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoContainerLeafIdentifierEdgesCompileAndMatchLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "idents-container-leaf-collision"), "module"), "idents-container-leaf-collision")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "idents-container-leaf-collision")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "Interface_ *string") {
		t.Fatalf("generated source should escape Go keyword leaf identifiers, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "idents-container-leaf-collision", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "idents-container-leaf-collision", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedContainerLeafIdentifierEdgesCompileAndMatchLibyang(t *testing.T) {
	demo := IdentsContainerLeafCollision{
		Top: IdentsContainerLeafCollisionTop{
			Config: IdentsContainerLeafCollisionTopConfig{
				Enabled: &struct{}{},
			},
			ConfigBackup: ptr("bak"),
			InterfaceState: IdentsContainerLeafCollisionTopInterfaceState{
				Up: ptr(true),
			},
			Interface_: ptr("eth0"),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoEnumValueIdentifierCollisionsCompileAndMatchLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "idents-enum-value-collision"), "module"), "idents-enum-value-collision")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "idents-enum-value-collision")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "IdentsEnumValueCollisionTopModeEnumEnableDefault2") {
		t.Fatalf("generated source should disambiguate enum variants with Go-ident collisions, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "idents-enum-value-collision", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "idents-enum-value-collision", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedEnumValueIdentifierCollisionsCompileAndMatchLibyang(t *testing.T) {
	mode := IdentsEnumValueCollisionTopModeEnumEnableDefault
	demo := IdentsEnumValueCollision{
		Top: IdentsEnumValueCollisionTop{
			Mode: &mode,
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoLongIdentifiersCompileAndMatchLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "idents-long-name"), "module"), "idents-long-name")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "idents-long-name")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "GetServiceAccountingAggregationSourceDestinationPrefixInformation *string") {
		t.Fatalf("generated source should preserve long names as stable Go identifiers, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "idents-long-name", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "idents-long-name", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedLongIdentifiersCompileAndMatchLibyang(t *testing.T) {
	demo := IdentsLongName{
		Top: IdentsLongNameTop{
			GetServiceAccountingAggregationSourceDestinationPrefixInformation: ptr("one"),
			VeryLongInterfaceConfigurationWithManyPolicyAndRouteSettingsApplied: ptr("two"),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoMixedCaseIdentifiersCompileAndMatchLibyang(t *testing.T) {
	ctx := loadModule(t, filepath.Join(schemaFixtureDir(t, "idents-unicode-mixed-case"), "module"), "idents-unicode-mixed-case")
	defer ctx.Close()

	src, err := codegen.GenerateGo(ctx, "idents-unicode-mixed-case")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "SystemStatus *string") || !strings.Contains(src, "ConfigUrl *string") {
		t.Fatalf("generated source should preserve mixed-case wire names with legal Go identifiers, got:\n%s", src)
	}

	wantXML, err := os.ReadFile(goldenPath(t, "idents-unicode-mixed-case", "output.xml"))
	if err != nil {
		t.Fatalf("read golden xml: %v", err)
	}
	wantJSON, err := os.ReadFile(goldenPath(t, "idents-unicode-mixed-case", "output.json"))
	if err != nil {
		t.Fatalf("read golden json: %v", err)
	}

	testBody := fmt.Sprintf(`
func TestGeneratedMixedCaseIdentifiersCompileAndMatchLibyang(t *testing.T) {
	demo := IdentsUnicodeMixedCase{
		Top: IdentsUnicodeMixedCaseTop{
			SystemStatus: ptr("s"),
			ConfigUrl:    ptr("c"),
			PortId:       ptr("p"),
			PortNum:      ptr("1"),
		},
	}
	gotXML := demo.ToXML()
	wantXML := %q
	if gotXML != wantXML {
		t.Fatalf("XML mismatch:\n got: %%q\nwant: %%q", gotXML, wantXML)
	}
	gotJSON := demo.ToJSONIETF()
	wantJSON := %q
	if gotJSON != wantJSON {
		t.Fatalf("JSON mismatch:\n got: %%q\nwant: %%q", gotJSON, wantJSON)
	}
}
`, string(wantXML), string(wantJSON))

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoMinElementsValidateRejects(t *testing.T) {
	src := generatedFixtureSource(t, "min-elements-reject", "min-elements-reject")

	testBody := `
func TestGeneratedMinElementsValidateRejects(t *testing.T) {
	demo := MinElementsReject{Servers: []string{"10.0.0.1"}}
	err := demo.Validate()
	if err == nil {
		t.Fatal("Validate succeeded, want min-elements violation")
	}
	if got, want := err.Error(), "/min-elements-reject/servers: min-elements violation"; got != want {
		t.Fatalf("Validate error = %q, want %q", got, want)
	}

	demo.Servers = []string{"10.0.0.1", "10.0.0.2"}
	if err := demo.Validate(); err != nil {
		t.Fatalf("Validate valid min-elements data: %v", err)
	}

	if _, err := FromJSONIETF([]byte("{\"min-elements-reject:servers\":[\"10.0.0.1\"]}")); err == nil {
		t.Fatal("FromJSONIETF succeeded, want min-elements validation violation")
	} else if got, want := err.Error(), "/min-elements-reject/servers: min-elements violation"; got != want {
		t.Fatalf("FromJSONIETF error = %q, want %q", got, want)
	}
}
`

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoListKeyDuplicateValidateRejects(t *testing.T) {
	src := generatedFixtureSource(t, "list-single-key-string", "list-single-key-string")

	testBody := `
func TestGeneratedListKeyDuplicateValidateRejects(t *testing.T) {
	demo := ListSingleKeyString{
		Interface_: []ListSingleKeyStringInterfaceEntry{
			{Name: "eth0"},
			{Name: "eth0"},
		},
	}
	err := demo.Validate()
	if err == nil {
		t.Fatal("Validate succeeded, want duplicate key violation")
	}
	if got, want := err.Error(), "/list-single-key-string/interface: duplicate key violation"; got != want {
		t.Fatalf("Validate error = %q, want %q", got, want)
	}

	demo.Interface_[1].Name = "eth1"
	if err := demo.Validate(); err != nil {
		t.Fatalf("Validate distinct list keys: %v", err)
	}

	_, err = FromJSONIETF([]byte("{\"list-single-key-string:interface\":[{\"name\":\"eth0\"},{\"name\":\"eth0\"}]}"))
	if err == nil {
		t.Fatal("FromJSONIETF accepted duplicate list keys")
	}
	if got, want := err.Error(), "/list-single-key-string/interface: duplicate key violation"; got != want {
		t.Fatalf("FromJSONIETF error = %q, want %q", got, want)
	}
}
`

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoCompositeListKeyDuplicateValidateRejects(t *testing.T) {
	src := generatedFixtureSource(t, "list-composite-key-two", "list-composite-key-two")

	testBody := `
func TestGeneratedCompositeListKeyDuplicateValidateRejects(t *testing.T) {
	demo := ListCompositeKeyTwo{
		Edge: []ListCompositeKeyTwoEdgeEntry{
			{SrcIp: "10.0.0.1", DstIp: "10.0.0.2"},
			{SrcIp: "10.0.0.1", DstIp: "10.0.0.2"},
		},
	}
	err := demo.Validate()
	if err == nil {
		t.Fatal("Validate succeeded, want duplicate key violation")
	}
	if got, want := err.Error(), "/list-composite-key-two/edge: duplicate key violation"; got != want {
		t.Fatalf("Validate error = %q, want %q", got, want)
	}

	demo.Edge[1].DstIp = "10.0.0.3"
	if err := demo.Validate(); err != nil {
		t.Fatalf("Validate distinct composite list keys: %v", err)
	}

	_, err = FromJSONIETF([]byte("{\"list-composite-key-two:edge\":[{\"src-ip\":\"10.0.0.1\",\"dst-ip\":\"10.0.0.2\"},{\"src-ip\":\"10.0.0.1\",\"dst-ip\":\"10.0.0.2\"}]}"))
	if err == nil {
		t.Fatal("FromJSONIETF accepted duplicate composite list keys")
	}
	if got, want := err.Error(), "/list-composite-key-two/edge: duplicate key violation"; got != want {
		t.Fatalf("FromJSONIETF error = %q, want %q", got, want)
	}
}
`

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoCompositeListKeyDelimiterCollision(t *testing.T) {
	src := generatedFixtureSource(t, "list-composite-key-two", "list-composite-key-two")
	if !strings.Contains(src, `key := strings.Join([]string{cambiumJSONEscape(entry.SrcIp), cambiumJSONEscape(entry.DstIp)}, "\x00")`) {
		t.Fatalf("generated composite key validation should use escaped JSON literal key parts, got:\n%s", src)
	}

	testBody := `
func TestGeneratedCompositeListKeyDelimiterCollision(t *testing.T) {
	demo := ListCompositeKeyTwo{
		Edge: []ListCompositeKeyTwoEdgeEntry{
			{SrcIp: "a", DstIp: "b\x00c"},
			{SrcIp: "a\x00b", DstIp: "c"},
		},
	}
	if err := demo.Validate(); err == nil {
		t.Fatal("Validate accepted invalid NUL character in composite key leaf")
	} else if got, want := err.Error(), "/list-composite-key-two/edge/dst-ip: invalid string character U+0000"; got != want {
		t.Fatalf("Validate error = %q, want %q", got, want)
	}
}
`

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoYang11EmptyListKeyCompilesAndValidates(t *testing.T) {
	const source = `module list-empty-key-yang11 {
    yang-version 1.1;
    namespace "urn:list-empty-key-yang11";
    prefix leky;

    list item {
        key "id";
        leaf id { type empty; }
        leaf label { type string; }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	src, err := codegen.GenerateGo(ctx, "list-empty-key-yang11")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if strings.Contains(src, ".Id.String()") {
		t.Fatalf("generated source should not sort empty key fields with String(), got:\n%s", src)
	}

	testBody := `
func TestGeneratedYang11EmptyListKeyValidates(t *testing.T) {
	demo := ListEmptyKeyYang11{
		Item: []ListEmptyKeyYang11ItemEntry{{
			Id:    struct{}{},
			Label: ptr("first"),
		}},
	}
	if err := demo.Validate(); err != nil {
		t.Fatalf("Validate single empty-key entry: %v", err)
	}
	if got, want := demo.ToJSONIETF(), "{\n  \"list-empty-key-yang11:item\": [\n    {\n      \"id\": [null],\n      \"label\": \"first\"\n    }\n  ]\n}\n"; got != want {
		t.Fatalf("JSON mismatch:\n got: %q\nwant: %q", got, want)
	}

	demo.Item = append(demo.Item, ListEmptyKeyYang11ItemEntry{Id: struct{}{}, Label: ptr("duplicate")})
	err := demo.Validate()
	if err == nil {
		t.Fatal("Validate accepted duplicate empty key")
	}
	if got, want := err.Error(), "/list-empty-key-yang11/item: duplicate key violation"; got != want {
		t.Fatalf("Validate error = %q, want %q", got, want)
	}
}
`

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoLeafListDuplicateValidateRejects(t *testing.T) {
	const source = `module leaflist-duplicate-reject {
    namespace "urn:leaflist-duplicate-reject";
    prefix lldr;

    leaf-list tag {
        type string;
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	src, err := codegen.GenerateGo(ctx, "leaflist-duplicate-reject")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	testBody := `
func TestGeneratedLeafListDuplicateValidateRejects(t *testing.T) {
	demo := LeaflistDuplicateReject{Tag: []string{"alpha", "alpha"}}
	err := demo.Validate()
	if err == nil {
		t.Fatal("Validate accepted duplicate leaf-list values")
	}
	if got, want := err.Error(), "/leaflist-duplicate-reject/tag: duplicate leaf-list value"; got != want {
		t.Fatalf("Validate error = %q, want %q", got, want)
	}

	if _, err := FromJSONIETF([]byte("{\"leaflist-duplicate-reject:tag\":[\"alpha\",\"alpha\"]}")); err == nil {
		t.Fatal("FromJSONIETF accepted duplicate leaf-list values")
	} else if got, want := err.Error(), "/leaflist-duplicate-reject/tag: duplicate leaf-list value"; got != want {
		t.Fatalf("FromJSONIETF error = %q, want %q", got, want)
	}

	demo.Tag = []string{"alpha", "beta"}
	if err := demo.Validate(); err != nil {
		t.Fatalf("Validate distinct leaf-list values: %v", err)
	}
}
`

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoYang11StateLeafListAllowsDuplicateValues(t *testing.T) {
	const source = `module leaflist-state-duplicate-yang11 {
    yang-version 1.1;
    namespace "urn:leaflist-state-duplicate-yang11";
    prefix llsdy;

    container state {
        config false;
        leaf-list sample {
            type string;
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	src, err := codegen.GenerateGo(ctx, "leaflist-state-duplicate-yang11")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	testBody := `
func TestGeneratedYang11StateLeafListAllowsDuplicateValues(t *testing.T) {
	demo := LeaflistStateDuplicateYang11{
		State: LeaflistStateDuplicateYang11State{
			Sample: []string{"alpha", "alpha"},
		},
	}
	if err := demo.Validate(); err != nil {
		t.Fatalf("Validate rejected duplicate state leaf-list values: %v", err)
	}
	parsed, err := FromJSONIETF([]byte("{\"leaflist-state-duplicate-yang11:state\":{\"sample\":[\"alpha\",\"alpha\"]}}"))
	if err != nil {
		t.Fatalf("FromJSONIETF rejected duplicate state leaf-list values: %v", err)
	}
	if got, want := parsed.ToJSONIETF(), "{\n  \"leaflist-state-duplicate-yang11:state\": {\n    \"sample\": [\n      \"alpha\",\n      \"alpha\"\n    ]\n  }\n}\n"; got != want {
		t.Fatalf("JSON mismatch:\n got: %q\nwant: %q", got, want)
	}
}
`

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoJSONParseMissingListKeyRejects(t *testing.T) {
	src := generatedFixtureSource(t, "list-single-key-string", "list-single-key-string")

	testBody := `
func TestGeneratedJSONParseMissingListKeyRejects(t *testing.T) {
	_, err := FromJSONIETF([]byte(` + "`" + `{
  "list-single-key-string:interface": [
    {
      "mtu": 1500
    }
  ]
}` + "`" + `))
	if err == nil {
		t.Fatal("FromJSONIETF accepted a list entry without its key")
	}
	if got, want := err.Error(), "missing JSON_IETF field \"name\""; got != want {
		t.Fatalf("FromJSONIETF error = %q, want %q", got, want)
	}
}
`

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoJSONParseRejectsOversizedInput(t *testing.T) {
	src := generatedFixtureSource(t, "json-ietf-parse-roundtrip", "json-ietf-parse-roundtrip")

	testBody := `
func TestGeneratedJSONParseRejectsOversizedInput(t *testing.T) {
	payload := make([]byte, cambiumJSONMaxBytes+1)
	_, err := FromJSONIETF(payload)
	if err == nil {
		t.Fatal("FromJSONIETF accepted oversized JSON_IETF input")
	}
	if got, want := err.Error(), "JSON_IETF document is 67108865 bytes, exceeds maximum 67108864 bytes"; got != want {
		t.Fatalf("FromJSONIETF error = %q, want %q", got, want)
	}
}
`

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoJSONParseRejectsExcessiveNesting(t *testing.T) {
	src := generatedFixtureSource(t, "json-ietf-parse-roundtrip", "json-ietf-parse-roundtrip")

	testBody := `
func TestGeneratedJSONParseRejectsExcessiveNesting(t *testing.T) {
	payload := make([]byte, 0, cambiumJSONMaxDepth*2+4)
	for i := 0; i < cambiumJSONMaxDepth+2; i++ {
		payload = append(payload, '[')
	}
	for i := 0; i < cambiumJSONMaxDepth+2; i++ {
		payload = append(payload, ']')
	}
	_, err := FromJSONIETF(payload)
	if err == nil {
		t.Fatal("FromJSONIETF accepted excessively nested JSON_IETF input")
	}
	if got, want := err.Error(), "JSON_IETF document exceeds maximum nesting depth"; got != want {
		t.Fatalf("FromJSONIETF error = %q, want %q", got, want)
	}
}
`

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoJSONParseMissingCompositeListKeyRejects(t *testing.T) {
	src := generatedFixtureSource(t, "list-composite-key-two", "list-composite-key-two")

	testBody := `
func TestGeneratedJSONParseMissingCompositeListKeyRejects(t *testing.T) {
	_, err := FromJSONIETF([]byte(` + "`" + `{
  "list-composite-key-two:edge": [
    {
      "src-ip": "10.0.0.1",
      "metric": 10
    }
  ]
}` + "`" + `))
	if err == nil {
		t.Fatal("FromJSONIETF accepted a composite-key list entry without all keys")
	}
	if got, want := err.Error(), "missing JSON_IETF field \"dst-ip\""; got != want {
		t.Fatalf("FromJSONIETF error = %q, want %q", got, want)
	}
}
`

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoJSONParseMissingMandatoryLeafRejects(t *testing.T) {
	src := generatedFixtureSource(t, "constraints-mandatory-interaction", "constraints-mandatory-interaction")

	testBody := `
func TestGeneratedJSONParseMissingMandatoryLeafRejects(t *testing.T) {
	_, err := FromJSONIETF([]byte(` + "`" + `{
  "constraints-mandatory-interaction:mode": "operational",
  "constraints-mandatory-interaction:services": {
    "primary-server": "10.0.0.1"
  },
  "constraints-mandatory-interaction:auth": {
    "username": "admin"
  }
}` + "`" + `))
	if err == nil {
		t.Fatal("FromJSONIETF accepted a document without a mandatory top-level leaf")
	}
	if got, want := err.Error(), "missing JSON_IETF field \"constraints-mandatory-interaction:hostname\""; got != want {
		t.Fatalf("FromJSONIETF error = %q, want %q", got, want)
	}

	_, err = FromJSONIETF([]byte(` + "`" + `{
  "constraints-mandatory-interaction:hostname": "router1",
  "constraints-mandatory-interaction:mode": "operational",
  "constraints-mandatory-interaction:services": {
    "primary-server": "10.0.0.1"
  },
  "constraints-mandatory-interaction:auth": {
    "password": "unset"
  }
}` + "`" + `))
	if err == nil {
		t.Fatal("FromJSONIETF accepted a nested object without a refined mandatory leaf")
	}
	if got, want := err.Error(), "missing JSON_IETF field \"username\""; got != want {
		t.Fatalf("FromJSONIETF error = %q, want %q", got, want)
	}

	_, err = FromJSONIETF([]byte(` + "`" + `{
  "constraints-mandatory-interaction:hostname": "router1",
  "constraints-mandatory-interaction:mode": "operational",
  "constraints-mandatory-interaction:services": {
    "primary-server": "10.0.0.1"
  }
}` + "`" + `))
	if err == nil {
		t.Fatal("FromJSONIETF accepted a document without a non-presence container that carries mandatory data")
	}
	if got, want := err.Error(), "missing JSON_IETF field \"constraints-mandatory-interaction:auth\""; got != want {
		t.Fatalf("FromJSONIETF error = %q, want %q", got, want)
	}
}
`

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoMaxElementsValidateRejects(t *testing.T) {
	src := generatedFixtureSource(t, "max-elements-reject", "max-elements-reject")

	testBody := `
func TestGeneratedMaxElementsValidateRejects(t *testing.T) {
	demo := MaxElementsReject{Servers: []string{"a", "b", "c", "d", "e"}}
	err := demo.Validate()
	if err == nil {
		t.Fatal("Validate succeeded, want max-elements violation")
	}
	if got, want := err.Error(), "/max-elements-reject/servers: max-elements violation"; got != want {
		t.Fatalf("Validate error = %q, want %q", got, want)
	}

	demo.Servers = []string{"a", "b", "c", "d"}
	if err := demo.Validate(); err != nil {
		t.Fatalf("Validate valid max-elements data: %v", err)
	}
}
`

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoUniqueConstraintValidateRejects(t *testing.T) {
	src := generatedFixtureSource(t, "constraints-unique-violation-reject", "constraints-unique-violation-reject")

	testBody := `
func TestGeneratedUniqueConstraintValidateRejects(t *testing.T) {
	demo := ConstraintsUniqueViolationReject{
		Entry: []ConstraintsUniqueViolationRejectEntryEntry{
			{Id: 1, A: ptr("same"), B: ptr("tuple")},
			{Id: 2, A: ptr("same"), B: ptr("tuple")},
		},
	}
	err := demo.Validate()
	if err == nil {
		t.Fatal("Validate succeeded, want unique violation")
	}
	if got, want := err.Error(), "/constraints-unique-violation-reject/entry: unique violation"; got != want {
		t.Fatalf("Validate error = %q, want %q", got, want)
	}

	demo.Entry[1].B = ptr("other")
	if err := demo.Validate(); err != nil {
		t.Fatalf("Validate distinct unique tuples: %v", err)
	}
}
`

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoDescendantUniqueConstraintValidateRejects(t *testing.T) {
	const source = `module unique-descendant-codegen {
    namespace "urn:unique-descendant-codegen";
    prefix udc;

    list item {
        key "id";
        unique "nested/code";
        leaf id { type string; }
        container nested {
            leaf code { type string; }
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	src, err := codegen.GenerateGo(ctx, "unique-descendant-codegen")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	testBody := `
func TestGeneratedDescendantUniqueConstraintValidateRejects(t *testing.T) {
	demo := UniqueDescendantCodegen{
		Item: []UniqueDescendantCodegenItemEntry{
			{Id: "one", Nested: UniqueDescendantCodegenItemEntryNested{Code: ptr("dup")}},
			{Id: "two", Nested: UniqueDescendantCodegenItemEntryNested{Code: ptr("dup")}},
		},
	}
	err := demo.Validate()
	if err == nil {
		t.Fatal("Validate accepted descendant unique violation")
	}
	if got, want := err.Error(), "/unique-descendant-codegen/item: unique violation"; got != want {
		t.Fatalf("Validate error = %q, want %q", got, want)
	}

	if _, err := FromJSONIETF([]byte(` + "`" + `{"unique-descendant-codegen:item":[{"id":"one","nested":{"code":"dup"}},{"id":"two","nested":{"code":"dup"}}]}` + "`" + `)); err == nil {
		t.Fatal("FromJSONIETF accepted descendant unique violation")
	} else if got, want := err.Error(), "/unique-descendant-codegen/item: unique violation"; got != want {
		t.Fatalf("FromJSONIETF error = %q, want %q", got, want)
	}

	demo.Item[1].Nested.Code = ptr("other")
	if err := demo.Validate(); err != nil {
		t.Fatalf("Validate distinct descendant unique values: %v", err)
	}
}
`

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoChoiceCaseUniqueConstraintValidateRejects(t *testing.T) {
	const source = `module unique-choice-codegen {
    namespace "urn:unique-choice-codegen";
    prefix ucc;

    list item {
        key "id";
        unique "mode/serial/code";
        leaf id { type string; }
        choice mode {
            case serial {
                leaf code { type string; }
            }
            case other {
                leaf description { type string; }
            }
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	src, err := codegen.GenerateGo(ctx, "unique-choice-codegen")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	testBody := `
func TestGeneratedChoiceCaseUniqueConstraintValidateRejects(t *testing.T) {
	demo := UniqueChoiceCodegen{
		Item: []UniqueChoiceCodegenItemEntry{
			{Id: "one", Code: ptr("dup")},
			{Id: "two", Code: ptr("dup")},
		},
	}
	err := demo.Validate()
	if err == nil {
		t.Fatal("Validate accepted choice/case unique violation")
	}
	if got, want := err.Error(), "/unique-choice-codegen/item: unique violation"; got != want {
		t.Fatalf("Validate error = %q, want %q", got, want)
	}

	demo.Item[1].Code = ptr("other")
	if err := demo.Validate(); err != nil {
		t.Fatalf("Validate distinct choice/case unique values: %v", err)
	}
}
`

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoDefaultedUniqueConstraintValidateRejects(t *testing.T) {
	const source = `module unique-default-codegen {
    namespace "urn:unique-default-codegen";
    prefix udc;

    list item {
        key "id";
        unique "code";
        leaf id { type string; }
        leaf code {
            type string;
            default "dup";
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	src, err := codegen.GenerateGo(ctx, "unique-default-codegen")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	testBody := `
func TestGeneratedDefaultedUniqueConstraintValidateRejects(t *testing.T) {
	demo := UniqueDefaultCodegen{
		Item: []UniqueDefaultCodegenItemEntry{
			{Id: "one"},
			{Id: "two"},
		},
	}
	err := demo.Validate()
	if err == nil {
		t.Fatal("Validate accepted duplicate effective default unique values")
	}
	if got, want := err.Error(), "/unique-default-codegen/item: unique violation"; got != want {
		t.Fatalf("Validate error = %q, want %q", got, want)
	}

	demo.Item[1].Code = ptr("other")
	if err := demo.Validate(); err != nil {
		t.Fatalf("Validate explicit value distinct from default: %v", err)
	}
}
`

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoMandatoryChoiceValidateRejects(t *testing.T) {
	src := generatedFixtureSource(t, "choice-mandatory-reject", "choice-mandatory-reject")

	testBody := `
func TestGeneratedMandatoryChoiceValidateRejects(t *testing.T) {
	demo := ChoiceMandatoryReject{}
	err := demo.Validate()
	if err == nil {
		t.Fatal("Validate succeeded, want mandatory choice violation")
	}
	if got, want := err.Error(), "/choice-mandatory-reject/tunnel-mode: mandatory choice has no selected case"; got != want {
		t.Fatalf("Validate error = %q, want %q", got, want)
	}

	demo = ChoiceMandatoryReject{Remote: ptr("203.0.113.1"), Label: ptr(uint32(16))}
	err = demo.Validate()
	if err == nil {
		t.Fatal("Validate succeeded, want multi-case choice violation")
	}
	if got, want := err.Error(), "/choice-mandatory-reject/tunnel-mode: choice has multiple cases selected"; got != want {
		t.Fatalf("Validate error = %q, want %q", got, want)
	}

	demo = ChoiceMandatoryReject{Remote: ptr("203.0.113.1")}
	if err := demo.Validate(); err != nil {
		t.Fatalf("Validate selected mandatory choice: %v", err)
	}

	if _, err := FromJSONIETF([]byte("{}")); err == nil {
		t.Fatal("FromJSONIETF succeeded, want mandatory choice validation violation")
	} else if got, want := err.Error(), "/choice-mandatory-reject/tunnel-mode: mandatory choice has no selected case"; got != want {
		t.Fatalf("FromJSONIETF error = %q, want %q", got, want)
	}
}
`

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoChoiceCaseMandatoryDescendantRejects(t *testing.T) {
	const source = `module choice-case-mandatory-descendant {
    namespace "urn:choice-case-mandatory-descendant";
    prefix ccmd;

    choice auth {
        case password {
            leaf username {
                type string;
            }
            leaf password {
                mandatory true;
                type string;
            }
        }
        case token {
            leaf token {
                type string;
            }
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	src, err := codegen.GenerateGo(ctx, "choice-case-mandatory-descendant")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	testBody := `
func TestGeneratedChoiceCaseMandatoryDescendantRejects(t *testing.T) {
	demo := ChoiceCaseMandatoryDescendant{Username: ptr("alice")}
	err := demo.Validate()
	if err == nil {
		t.Fatal("Validate accepted selected choice case with missing mandatory descendant")
	}
	if got, want := err.Error(), "/choice-case-mandatory-descendant/auth/password/password: missing mandatory field"; got != want {
		t.Fatalf("Validate error = %q, want %q", got, want)
	}

	if _, err := FromJSONIETF([]byte("{\"choice-case-mandatory-descendant:username\":\"alice\"}")); err == nil {
		t.Fatal("FromJSONIETF accepted selected choice case with missing mandatory descendant")
	} else if got, want := err.Error(), "/choice-case-mandatory-descendant/auth/password/password: missing mandatory field"; got != want {
		t.Fatalf("FromJSONIETF error = %q, want %q", got, want)
	}

	demo = ChoiceCaseMandatoryDescendant{Username: ptr("alice"), Password: ptr("secret")}
	if err := demo.Validate(); err != nil {
		t.Fatalf("Validate selected case with mandatory descendant: %v", err)
	}
	demo = ChoiceCaseMandatoryDescendant{Token: ptr("tok-123")}
	if err := demo.Validate(); err != nil {
		t.Fatalf("Validate alternate case: %v", err)
	}
}
`

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoChoiceCaseMinElementsOnlyWhenSelected(t *testing.T) {
	const source = `module choice-case-min-elements {
    namespace "urn:choice-case-min-elements";
    prefix ccme;

    choice auth {
        case password {
            leaf username {
                type string;
            }
            leaf-list challenge {
                min-elements 2;
                type string;
            }
        }
        case token {
            leaf token {
                type string;
            }
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	src, err := codegen.GenerateGo(ctx, "choice-case-min-elements")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	testBody := `
func TestGeneratedChoiceCaseMinElementsOnlyWhenSelected(t *testing.T) {
	demo := ChoiceCaseMinElements{Token: ptr("tok-123")}
	if err := demo.Validate(); err != nil {
		t.Fatalf("Validate alternate case with absent min-elements leaf-list: %v", err)
	}

	demo = ChoiceCaseMinElements{Username: ptr("alice")}
	err := demo.Validate()
	if err == nil {
		t.Fatal("Validate accepted selected case with missing min-elements leaf-list")
	}
	if got, want := err.Error(), "/choice-case-min-elements/auth/password/challenge: min-elements violation"; got != want {
		t.Fatalf("Validate error = %q, want %q", got, want)
	}

	if _, err := FromJSONIETF([]byte("{\"choice-case-min-elements:token\":\"tok-123\"}")); err != nil {
		t.Fatalf("FromJSONIETF rejected alternate case with absent min-elements leaf-list: %v", err)
	}
	if _, err := FromJSONIETF([]byte("{\"choice-case-min-elements:username\":\"alice\"}")); err == nil {
		t.Fatal("FromJSONIETF accepted selected case with missing min-elements leaf-list")
	} else if got, want := err.Error(), "/choice-case-min-elements/auth/password/challenge: min-elements violation"; got != want {
		t.Fatalf("FromJSONIETF error = %q, want %q", got, want)
	}

	demo = ChoiceCaseMinElements{Challenge: []string{"one"}}
	err = demo.Validate()
	if err == nil {
		t.Fatal("Validate accepted selected case with too few leaf-list entries")
	}
	if got, want := err.Error(), "/choice-case-min-elements/auth/password/challenge: min-elements violation"; got != want {
		t.Fatalf("Validate partial min-elements error = %q, want %q", got, want)
	}
	if _, err := FromJSONIETF([]byte("{\"choice-case-min-elements:challenge\":[\"one\"]}")); err == nil {
		t.Fatal("FromJSONIETF accepted selected case with too few leaf-list entries")
	} else if got, want := err.Error(), "/choice-case-min-elements/auth/password/challenge: min-elements violation"; got != want {
		t.Fatalf("FromJSONIETF partial min-elements error = %q, want %q", got, want)
	}

	demo = ChoiceCaseMinElements{Username: ptr("alice"), Challenge: []string{"one", "two"}}
	if err := demo.Validate(); err != nil {
		t.Fatalf("Validate selected case with min-elements satisfied: %v", err)
	}
}
`

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoChoiceCaseNestedMandatoryDescendantRejects(t *testing.T) {
	const source = `module choice-case-nested-mandatory {
    namespace "urn:choice-case-nested-mandatory";
    prefix ccnm;

    choice auth {
        case password {
            container credentials {
                leaf username {
                    type string;
                }
                leaf password {
                    mandatory true;
                    type string;
                }
            }
        }
        case token {
            leaf token {
                type string;
            }
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	src, err := codegen.GenerateGo(ctx, "choice-case-nested-mandatory")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	testBody := `
func TestGeneratedChoiceCaseNestedMandatoryDescendantRejects(t *testing.T) {
	demo := ChoiceCaseNestedMandatory{
		Credentials: ChoiceCaseNestedMandatoryCredentials{
			Username: ptr("alice"),
		},
	}
	err := demo.Validate()
	if err == nil {
		t.Fatal("Validate accepted selected choice case with missing nested mandatory descendant")
	}
	if got, want := err.Error(), "/choice-case-nested-mandatory/auth/password/credentials/password: missing mandatory field"; got != want {
		t.Fatalf("Validate error = %q, want %q", got, want)
	}

	if _, err := FromJSONIETF([]byte("{\"choice-case-nested-mandatory:credentials\":{\"username\":\"alice\"}}")); err == nil {
		t.Fatal("FromJSONIETF accepted selected choice case with missing nested mandatory descendant")
	} else if got, want := err.Error(), "/choice-case-nested-mandatory/auth/password/credentials/password: missing mandatory field"; got != want {
		t.Fatalf("FromJSONIETF error = %q, want %q", got, want)
	}

	demo = ChoiceCaseNestedMandatory{
		Credentials: ChoiceCaseNestedMandatoryCredentials{
			Username: ptr("alice"),
			Password: ptr("secret"),
		},
	}
	if err := demo.Validate(); err != nil {
		t.Fatalf("Validate selected case with nested mandatory descendant: %v", err)
	}
	demo = ChoiceCaseNestedMandatory{Token: ptr("tok-123")}
	if err := demo.Validate(); err != nil {
		t.Fatalf("Validate alternate case: %v", err)
	}
}
`

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoChoiceCaseNonPresenceContainerDoesNotSelectEmptyCase(t *testing.T) {
	const source = `module choice-case-nonpresence-container {
    namespace "urn:choice-case-nonpresence-container";
    prefix ccnpc;

    choice auth {
        case password {
            container credentials {
                leaf username {
                    type string;
                }
                leaf password {
                    mandatory true;
                    type string;
                }
            }
        }
        case token {
            leaf token {
                type string;
            }
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	src, err := codegen.GenerateGo(ctx, "choice-case-nonpresence-container")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	testBody := `
func TestGeneratedChoiceCaseNonPresenceContainerDoesNotSelectEmptyCase(t *testing.T) {
	empty := ChoiceCaseNonpresenceContainer{}
	if err := empty.Validate(); err != nil {
		t.Fatalf("Validate rejected unselected optional choice case: %v", err)
	}
	if _, err := FromJSONIETF([]byte("{}")); err != nil {
		t.Fatalf("FromJSONIETF rejected unselected optional choice case: %v", err)
	}
	if _, err := FromJSONIETF([]byte("{\"choice-case-nonpresence-container:credentials\":{}}")); err == nil {
		t.Fatal("FromJSONIETF accepted explicit empty non-presence container case with missing mandatory descendant")
	} else if got, want := err.Error(), "/choice-case-nonpresence-container/auth/password/credentials/password: missing mandatory field"; got != want {
		t.Fatalf("FromJSONIETF empty container error = %q, want %q", got, want)
	}

	selectedWithoutMandatory := ChoiceCaseNonpresenceContainer{
		Credentials: ChoiceCaseNonpresenceContainerCredentials{
			Username: ptr("alice"),
		},
	}
	err := selectedWithoutMandatory.Validate()
	if err == nil {
		t.Fatal("Validate accepted selected non-presence container case with missing mandatory descendant")
	}
	if got, want := err.Error(), "/choice-case-nonpresence-container/auth/password/credentials/password: missing mandatory field"; got != want {
		t.Fatalf("Validate error = %q, want %q", got, want)
	}
	if _, err := FromJSONIETF([]byte("{\"choice-case-nonpresence-container:credentials\":{\"username\":\"alice\"}}")); err == nil {
		t.Fatal("FromJSONIETF accepted selected non-presence container case with missing mandatory descendant")
	} else if got, want := err.Error(), "/choice-case-nonpresence-container/auth/password/credentials/password: missing mandatory field"; got != want {
		t.Fatalf("FromJSONIETF error = %q, want %q", got, want)
	}

	token := ChoiceCaseNonpresenceContainer{Token: ptr("tok-123")}
	if err := token.Validate(); err != nil {
		t.Fatalf("Validate rejected alternate case: %v", err)
	}
}
`

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoChoiceCaseNestedMinElementsOnlyWhenSelected(t *testing.T) {
	const source = `module choice-case-nested-min-elements {
    namespace "urn:choice-case-nested-min-elements";
    prefix ccnme;

    choice auth {
        case password {
            container credentials {
                leaf username {
                    type string;
                }
                leaf-list challenge {
                    min-elements 2;
                    type string;
                }
            }
        }
        case token {
            leaf token {
                type string;
            }
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	src, err := codegen.GenerateGo(ctx, "choice-case-nested-min-elements")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	testBody := `
func TestGeneratedChoiceCaseNestedMinElementsOnlyWhenSelected(t *testing.T) {
	demo := ChoiceCaseNestedMinElements{Token: ptr("tok-123")}
	if err := demo.Validate(); err != nil {
		t.Fatalf("Validate alternate case with absent nested min-elements leaf-list: %v", err)
	}

	demo = ChoiceCaseNestedMinElements{
		Credentials: ChoiceCaseNestedMinElementsCredentials{
			Username: ptr("alice"),
		},
	}
	err := demo.Validate()
	if err == nil {
		t.Fatal("Validate accepted selected choice case with missing nested min-elements leaf-list")
	}
	if got, want := err.Error(), "/choice-case-nested-min-elements/auth/password/credentials/challenge: min-elements violation"; got != want {
		t.Fatalf("Validate error = %q, want %q", got, want)
	}

	if _, err := FromJSONIETF([]byte("{\"choice-case-nested-min-elements:token\":\"tok-123\"}")); err != nil {
		t.Fatalf("FromJSONIETF rejected alternate case with absent nested min-elements leaf-list: %v", err)
	}
	if _, err := FromJSONIETF([]byte("{\"choice-case-nested-min-elements:credentials\":{\"username\":\"alice\"}}")); err == nil {
		t.Fatal("FromJSONIETF accepted selected choice case with missing nested min-elements leaf-list")
	} else if got, want := err.Error(), "/choice-case-nested-min-elements/auth/password/credentials/challenge: min-elements violation"; got != want {
		t.Fatalf("FromJSONIETF error = %q, want %q", got, want)
	}

	demo = ChoiceCaseNestedMinElements{
		Credentials: ChoiceCaseNestedMinElementsCredentials{
			Challenge: []string{"one"},
		},
	}
	err = demo.Validate()
	if err == nil {
		t.Fatal("Validate accepted selected case with too few nested leaf-list entries")
	}
	if got, want := err.Error(), "/choice-case-nested-min-elements/auth/password/credentials/challenge: min-elements violation"; got != want {
		t.Fatalf("Validate partial nested min-elements error = %q, want %q", got, want)
	}
	if _, err := FromJSONIETF([]byte("{\"choice-case-nested-min-elements:credentials\":{\"challenge\":[\"one\"]}}")); err == nil {
		t.Fatal("FromJSONIETF accepted selected case with too few nested leaf-list entries")
	} else if got, want := err.Error(), "/choice-case-nested-min-elements/auth/password/credentials/challenge: min-elements violation"; got != want {
		t.Fatalf("FromJSONIETF partial nested min-elements error = %q, want %q", got, want)
	}

	demo = ChoiceCaseNestedMinElements{
		Credentials: ChoiceCaseNestedMinElementsCredentials{
			Username:  ptr("alice"),
			Challenge: []string{"one", "two"},
		},
	}
	if err := demo.Validate(); err != nil {
		t.Fatalf("Validate selected case with nested min-elements satisfied: %v", err)
	}
}
`

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoChoiceCaseMaxElementsUsesSelectedCasePath(t *testing.T) {
	const source = `module choice-case-max-elements {
    namespace "urn:choice-case-max-elements";
    prefix ccme;

    choice auth {
        case password {
            leaf username {
                type string;
            }
            leaf-list challenge {
                max-elements 2;
                type string;
            }
        }
        case token {
            leaf token {
                type string;
            }
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	src, err := codegen.GenerateGo(ctx, "choice-case-max-elements")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	testBody := `
func TestGeneratedChoiceCaseMaxElementsUsesSelectedCasePath(t *testing.T) {
	demo := ChoiceCaseMaxElements{Token: ptr("tok-123")}
	if err := demo.Validate(); err != nil {
		t.Fatalf("Validate alternate case with absent max-elements leaf-list: %v", err)
	}

	demo = ChoiceCaseMaxElements{Challenge: []string{"one", "two", "three"}}
	err := demo.Validate()
	if err == nil {
		t.Fatal("Validate accepted selected case with too many leaf-list entries")
	}
	if got, want := err.Error(), "/choice-case-max-elements/auth/password/challenge: max-elements violation"; got != want {
		t.Fatalf("Validate max-elements error = %q, want %q", got, want)
	}
	if _, err := FromJSONIETF([]byte("{\"choice-case-max-elements:challenge\":[\"one\",\"two\",\"three\"]}")); err == nil {
		t.Fatal("FromJSONIETF accepted selected case with too many leaf-list entries")
	} else if got, want := err.Error(), "/choice-case-max-elements/auth/password/challenge: max-elements violation"; got != want {
		t.Fatalf("FromJSONIETF max-elements error = %q, want %q", got, want)
	}

	demo = ChoiceCaseMaxElements{Username: ptr("alice"), Challenge: []string{"one", "two"}}
	if err := demo.Validate(); err != nil {
		t.Fatalf("Validate selected case with max-elements satisfied: %v", err)
	}
}
`

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoNestedMandatoryChoiceOnlyWhenParentCaseSelected(t *testing.T) {
	const source = `module nested-mandatory-choice-in-case {
    namespace "urn:nested-mandatory-choice-in-case";
    prefix nmcic;

    choice auth {
        case password {
            leaf username {
                type string;
            }
            choice secret {
                mandatory true;
                leaf password {
                    type string;
                }
                leaf otp {
                    type string;
                }
            }
        }
        case token {
            leaf token {
                type string;
            }
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	src, err := codegen.GenerateGo(ctx, "nested-mandatory-choice-in-case")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	testBody := `
func TestGeneratedNestedMandatoryChoiceOnlyWhenParentCaseSelected(t *testing.T) {
	demo := NestedMandatoryChoiceInCase{Token: ptr("tok-123")}
	if err := demo.Validate(); err != nil {
		t.Fatalf("Validate rejected alternate case with nested mandatory choice in unselected case: %v", err)
	}
	if _, err := FromJSONIETF([]byte("{\"nested-mandatory-choice-in-case:token\":\"tok-123\"}")); err != nil {
		t.Fatalf("FromJSONIETF rejected alternate case with nested mandatory choice in unselected case: %v", err)
	}

	demo = NestedMandatoryChoiceInCase{Username: ptr("alice")}
	err := demo.Validate()
	if err == nil {
		t.Fatal("Validate accepted selected parent case with missing nested mandatory choice")
	}
	if got, want := err.Error(), "/nested-mandatory-choice-in-case/auth/password/secret: mandatory choice has no selected case"; got != want {
		t.Fatalf("Validate error = %q, want %q", got, want)
	}
	if _, err := FromJSONIETF([]byte("{\"nested-mandatory-choice-in-case:username\":\"alice\"}")); err == nil {
		t.Fatal("FromJSONIETF accepted selected parent case with missing nested mandatory choice")
	} else if got, want := err.Error(), "/nested-mandatory-choice-in-case/auth/password/secret: mandatory choice has no selected case"; got != want {
		t.Fatalf("FromJSONIETF error = %q, want %q", got, want)
	}

	demo = NestedMandatoryChoiceInCase{Username: ptr("alice"), Password: ptr("secret")}
	if err := demo.Validate(); err != nil {
		t.Fatalf("Validate selected nested choice: %v", err)
	}
	demo = NestedMandatoryChoiceInCase{Username: ptr("alice"), Password: ptr("secret"), Otp: ptr("123456")}
	err = demo.Validate()
	if err == nil {
		t.Fatal("Validate accepted multiple nested choice cases")
	}
	if got, want := err.Error(), "/nested-mandatory-choice-in-case/auth/password/secret: choice has multiple cases selected"; got != want {
		t.Fatalf("Validate multiple nested choice error = %q, want %q", got, want)
	}
}
`

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoNestedMandatoryChoiceInsideCaseContainerOnlyWhenSelected(t *testing.T) {
	const source = `module nested-mandatory-choice-in-container {
    namespace "urn:nested-mandatory-choice-in-container";
    prefix nmcic;

    choice auth {
        case password {
            container credentials {
                leaf username {
                    type string;
                }
                choice secret {
                    mandatory true;
                    leaf password {
                        type string;
                    }
                    leaf otp {
                        type string;
                    }
                }
            }
        }
        case token {
            leaf token {
                type string;
            }
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	src, err := codegen.GenerateGo(ctx, "nested-mandatory-choice-in-container")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	testBody := `
func TestGeneratedNestedMandatoryChoiceInsideCaseContainerOnlyWhenSelected(t *testing.T) {
	demo := NestedMandatoryChoiceInContainer{Token: ptr("tok-123")}
	if err := demo.Validate(); err != nil {
		t.Fatalf("Validate rejected alternate case with nested mandatory choice in unselected container: %v", err)
	}
	if _, err := FromJSONIETF([]byte("{\"nested-mandatory-choice-in-container:token\":\"tok-123\"}")); err != nil {
		t.Fatalf("FromJSONIETF rejected alternate case with nested mandatory choice in unselected container: %v", err)
	}

	demo = NestedMandatoryChoiceInContainer{
		Credentials: NestedMandatoryChoiceInContainerCredentials{
			Username: ptr("alice"),
		},
	}
	err := demo.Validate()
	if err == nil {
		t.Fatal("Validate accepted selected parent case with missing nested mandatory choice")
	}
	if got, want := err.Error(), "/nested-mandatory-choice-in-container/auth/password/credentials/secret: mandatory choice has no selected case"; got != want {
		t.Fatalf("Validate error = %q, want %q", got, want)
	}
	if _, err := FromJSONIETF([]byte("{\"nested-mandatory-choice-in-container:credentials\":{\"username\":\"alice\"}}")); err == nil {
		t.Fatal("FromJSONIETF accepted selected parent case with missing nested mandatory choice")
	} else if got, want := err.Error(), "/nested-mandatory-choice-in-container/auth/password/credentials/secret: mandatory choice has no selected case"; got != want {
		t.Fatalf("FromJSONIETF error = %q, want %q", got, want)
	}

	demo = NestedMandatoryChoiceInContainer{
		Credentials: NestedMandatoryChoiceInContainerCredentials{
			Username: ptr("alice"),
			Password: ptr("secret"),
		},
	}
	if err := demo.Validate(); err != nil {
		t.Fatalf("Validate selected nested choice in case container: %v", err)
	}
	demo = NestedMandatoryChoiceInContainer{
		Credentials: NestedMandatoryChoiceInContainerCredentials{
			Username: ptr("alice"),
			Password: ptr("secret"),
			Otp:      ptr("123456"),
		},
	}
	err = demo.Validate()
	if err == nil {
		t.Fatal("Validate accepted multiple nested choice cases")
	}
	if got, want := err.Error(), "/nested-mandatory-choice-in-container/auth/password/credentials/secret: choice has multiple cases selected"; got != want {
		t.Fatalf("Validate multiple nested choice error = %q, want %q", got, want)
	}
}
`

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoChoiceCaseScalarValidationUsesSelectedCasePath(t *testing.T) {
	const source = `module choice-case-scalar-validation {
    namespace "urn:choice-case-scalar-validation";
    prefix ccsv;

    choice auth {
        case password {
            leaf secret {
                type string {
                    pattern '[a-z]+';
                }
            }
        }
        case token {
            leaf token {
                type string;
            }
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	src, err := codegen.GenerateGo(ctx, "choice-case-scalar-validation")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	testBody := `
func TestGeneratedChoiceCaseScalarValidationUsesSelectedCasePath(t *testing.T) {
	demo := ChoiceCaseScalarValidation{Token: ptr("tok-123")}
	if err := demo.Validate(); err != nil {
		t.Fatalf("Validate rejected alternate case with absent pattern leaf: %v", err)
	}

	demo = ChoiceCaseScalarValidation{Secret: ptr("INVALID")}
	err := demo.Validate()
	if err == nil {
		t.Fatal("Validate accepted selected case with pattern-invalid leaf")
	}
	if got, want := err.Error(), "/choice-case-scalar-validation/auth/password/secret: pattern violation"; got != want {
		t.Fatalf("Validate pattern error = %q, want %q", got, want)
	}

	demo = ChoiceCaseScalarValidation{Secret: ptr("valid")}
	if err := demo.Validate(); err != nil {
		t.Fatalf("Validate selected case with valid pattern leaf: %v", err)
	}
}
`

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoChoiceCaseListMandatoryEntryDescendantRejects(t *testing.T) {
	const source = `module choice-case-list-mandatory {
    namespace "urn:choice-case-list-mandatory";
    prefix cclm;

    choice auth {
        case password {
            list credential {
                key "name";
                unique "secret";
                leaf name {
                    type string;
                }
                leaf secret {
                    mandatory true;
                    type string;
                }
            }
        }
        case token {
            leaf token {
                type string;
            }
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	src, err := codegen.GenerateGo(ctx, "choice-case-list-mandatory")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	testBody := `
func TestGeneratedChoiceCaseListMandatoryEntryDescendantRejects(t *testing.T) {
	demo := ChoiceCaseListMandatory{Token: ptr("tok-123")}
	if err := demo.Validate(); err != nil {
		t.Fatalf("Validate rejected alternate case with absent list: %v", err)
	}

	demo = ChoiceCaseListMandatory{
		Credential: []ChoiceCaseListMandatoryCredentialEntry{
			{Name: "alice"},
		},
	}
	err := demo.Validate()
	if err == nil {
		t.Fatal("Validate accepted selected case list entry with missing mandatory descendant")
	}
	if got, want := err.Error(), "/choice-case-list-mandatory/auth/password/credential/secret: missing mandatory field"; got != want {
		t.Fatalf("Validate error = %q, want %q", got, want)
	}
	if _, err := FromJSONIETF([]byte("{\"choice-case-list-mandatory:credential\":[{\"name\":\"alice\"}]}")); err == nil {
		t.Fatal("FromJSONIETF accepted selected case list entry with missing mandatory descendant")
	} else if got, want := err.Error(), "/choice-case-list-mandatory/auth/password/credential/secret: missing mandatory field"; got != want {
		t.Fatalf("FromJSONIETF error = %q, want %q", got, want)
	}

	demo = ChoiceCaseListMandatory{
		Credential: []ChoiceCaseListMandatoryCredentialEntry{
			{Name: "alice", Secret: ptr("one")},
			{Name: "alice", Secret: ptr("two")},
		},
	}
	err = demo.Validate()
	if err == nil {
		t.Fatal("Validate accepted selected case list with duplicate keys")
	}
	if got, want := err.Error(), "/choice-case-list-mandatory/auth/password/credential: duplicate key violation"; got != want {
		t.Fatalf("Validate duplicate key error = %q, want %q", got, want)
	}
	if _, err := FromJSONIETF([]byte("{\"choice-case-list-mandatory:credential\":[{\"name\":\"alice\",\"secret\":\"one\"},{\"name\":\"alice\",\"secret\":\"two\"}]}")); err == nil {
		t.Fatal("FromJSONIETF accepted selected case list with duplicate keys")
	} else if got, want := err.Error(), "/choice-case-list-mandatory/auth/password/credential: duplicate key violation"; got != want {
		t.Fatalf("FromJSONIETF duplicate key error = %q, want %q", got, want)
	}

	demo = ChoiceCaseListMandatory{
		Credential: []ChoiceCaseListMandatoryCredentialEntry{
			{Name: "alice", Secret: ptr("same")},
			{Name: "bob", Secret: ptr("same")},
		},
	}
	err = demo.Validate()
	if err == nil {
		t.Fatal("Validate accepted selected case list with unique violation")
	}
	if got, want := err.Error(), "/choice-case-list-mandatory/auth/password/credential: unique violation"; got != want {
		t.Fatalf("Validate unique error = %q, want %q", got, want)
	}
	if _, err := FromJSONIETF([]byte("{\"choice-case-list-mandatory:credential\":[{\"name\":\"alice\",\"secret\":\"same\"},{\"name\":\"bob\",\"secret\":\"same\"}]}")); err == nil {
		t.Fatal("FromJSONIETF accepted selected case list with unique violation")
	} else if got, want := err.Error(), "/choice-case-list-mandatory/auth/password/credential: unique violation"; got != want {
		t.Fatalf("FromJSONIETF unique error = %q, want %q", got, want)
	}

	demo = ChoiceCaseListMandatory{
		Credential: []ChoiceCaseListMandatoryCredentialEntry{
			{Name: "alice", Secret: ptr("secret")},
		},
	}
	if err := demo.Validate(); err != nil {
		t.Fatalf("Validate selected case list entry with mandatory descendant: %v", err)
	}
}
`

	runGeneratedGoTest(t, src, testBody)
}

func TestGeneratedGoChoiceCaseNestedMaxElementsUsesSelectedCasePath(t *testing.T) {
	const source = `module choice-case-nested-max-elements {
    namespace "urn:choice-case-nested-max-elements";
    prefix ccnme;

    choice auth {
        case password {
            container credentials {
                leaf username {
                    type string;
                }
                leaf-list challenge {
                    max-elements 2;
                    type string;
                }
            }
        }
        case token {
            leaf token {
                type string;
            }
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	src, err := codegen.GenerateGo(ctx, "choice-case-nested-max-elements")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	testBody := `
func TestGeneratedChoiceCaseNestedMaxElementsUsesSelectedCasePath(t *testing.T) {
	demo := ChoiceCaseNestedMaxElements{Token: ptr("tok-123")}
	if err := demo.Validate(); err != nil {
		t.Fatalf("Validate alternate case with absent nested max-elements leaf-list: %v", err)
	}

	demo = ChoiceCaseNestedMaxElements{
		Credentials: ChoiceCaseNestedMaxElementsCredentials{
			Challenge: []string{"one", "two", "three"},
		},
	}
	err := demo.Validate()
	if err == nil {
		t.Fatal("Validate accepted selected case with too many nested leaf-list entries")
	}
	if got, want := err.Error(), "/choice-case-nested-max-elements/auth/password/credentials/challenge: max-elements violation"; got != want {
		t.Fatalf("Validate nested max-elements error = %q, want %q", got, want)
	}
	if _, err := FromJSONIETF([]byte("{\"choice-case-nested-max-elements:credentials\":{\"challenge\":[\"one\",\"two\",\"three\"]}}")); err == nil {
		t.Fatal("FromJSONIETF accepted selected case with too many nested leaf-list entries")
	} else if got, want := err.Error(), "/choice-case-nested-max-elements/auth/password/credentials/challenge: max-elements violation"; got != want {
		t.Fatalf("FromJSONIETF nested max-elements error = %q, want %q", got, want)
	}

	demo = ChoiceCaseNestedMaxElements{
		Credentials: ChoiceCaseNestedMaxElementsCredentials{
			Username:  ptr("alice"),
			Challenge: []string{"one", "two"},
		},
	}
	if err := demo.Validate(); err != nil {
		t.Fatalf("Validate selected case with nested max-elements satisfied: %v", err)
	}
}
`

	runGeneratedGoTest(t, src, testBody)
}

const generatedGoCommandTimeout = 5 * time.Minute

func runGeneratedGoTest(t *testing.T, generatedSrc, testBody string) {
	t.Helper()

	// The generator must already emit gofmt-stable source; re-assert it here so
	// regressions in generated formatting fail the harness, not just gofmt.
	formatted, err := format.Source([]byte(generatedSrc))
	if err != nil {
		t.Fatalf("generated source is not gofmt-clean:\n%s\n%v", generatedSrc, err)
	}
	if !bytes.Equal(formatted, []byte(generatedSrc)) {
		t.Fatalf("generated source is not gofmt-stable:\n%s", string(formatted))
	}

	dir := t.TempDir()
	goMod := `module generated

go 1.25.0
`
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "gen.go"), []byte(generatedSrc), 0o644); err != nil {
		t.Fatalf("write generated source: %v", err)
	}

	helper := `
func ptr[T any](v T) *T { return &v }
`
	genTest := "package generated\n\nimport \"testing\"\n" + helper + testBody
	if err := os.WriteFile(filepath.Join(dir, "gen_test.go"), []byte(genTest), 0o644); err != nil {
		t.Fatalf("write test source: %v", err)
	}

	testCtx, cancelTest := context.WithTimeout(context.Background(), generatedGoCommandTimeout)
	defer cancelTest()
	out, err := runGeneratedCommand(testCtx, dir, "go", "test", "-vet=off", "-v", "./...")
	if err != nil {
		if testCtx.Err() != nil {
			t.Fatalf("generated Go test timed out after %s:\n%s\n%v", generatedGoCommandTimeout, string(out), err)
		}
		t.Fatalf("generated Go test failed:\n%s\n%v", string(out), err)
	}
	if !strings.Contains(string(out), "PASS") {
		t.Fatalf("expected PASS in generated test output:\n%s", string(out))
	}

	vetCtx, cancelVet := context.WithTimeout(context.Background(), generatedGoCommandTimeout)
	defer cancelVet()
	vetOut, err := runGeneratedCommand(vetCtx, dir, "go", "vet", "./...")
	if err != nil {
		if vetCtx.Err() != nil {
			t.Fatalf("go vet timed out after %s on generated source:\n%s\n%v", generatedGoCommandTimeout, string(vetOut), err)
		}
		t.Fatalf("go vet failed on generated source:\n%s\n%v", string(vetOut), err)
	}
}

func runGeneratedCommand(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = generatedCommandEnv(os.Environ())
	cmd.WaitDelay = 5 * time.Second
	return cmd.CombinedOutput()
}

func generatedCommandEnv(base []string) []string {
	env := append([]string(nil), base...)
	env = setEnv(env, "CGO_ENABLED", "0")
	env = setEnv(env, "GOTOOLCHAIN", "local")
	return env
}

func setEnv(env []string, key, value string) []string {
	prefix := key + "="
	replacement := prefix + value
	for i, item := range env {
		if strings.HasPrefix(item, prefix) {
			env[i] = replacement
			return env
		}
	}
	return append(env, replacement)
}

package codegen

import (
	_ "embed"
	"strings"
	"text/template"
)

// renderType executes a type-helper template into a string, recording any
// template error on the emitter so GenerateGo fails loudly.
func (g *goEmitter) renderType(tmpl *template.Template, data any) string {
	var b strings.Builder
	if err := tmpl.Execute(&b, data); err != nil {
		g.recordEmitError(err)
	}
	return b.String()
}

// Fully-static generated-code helper blocks. These carry no per-node data, so
// they are stored verbatim as embedded files instead of being assembled with
// hundreds of hand-escaped strings.Builder.WriteString calls. The bytes are the
// exact output of the former emit*Helper bodies; regenerate with
// `CAMBIUM_REGEN_TEMPLATES=1 go test ./codegen -run TestRegenerateStaticTemplates`.

//go:embed templates/binaryparse.tmpl
var tmplBinaryParse string

//go:embed templates/patternvalidate.tmpl
var tmplPatternValidate string

//go:embed templates/bitsparse.tmpl
var tmplBitsParse string

//go:embed templates/decimal64.tmpl
var tmplDecimal64 string

//go:embed templates/userorderedvec.tmpl
var tmplUserOrderedVec string

//go:embed templates/instanceidentifier.tmpl
var tmplInstanceIdentifier string

//go:embed templates/anydata.tmpl
var tmplAnyData string

//go:embed templates/cambiumstructiface.tmpl
var tmplCambiumStructIface string

// The RFC-7952 metadata helper is a hybrid: a static shell plus two data-driven
// switches (module bindings, declared annotations). It is a text/template (not a
// verbatim embed) so the static code reads naturally while the two switches are
// {{range}} blocks over view data. Generated output is gofmt-normalized by
// GenerateGo, so only the token structure must match.
//
//go:embed templates/metadata.go.tmpl
var tmplMetadataText string

var tmplMetadata = template.Must(template.New("metadata").Parse(tmplMetadataText))

type metadataBinding struct{ Name, Prefix, Namespace string }

type metadataAnnotationGroup struct {
	Module string
	Locals []string
}

type metadataView struct {
	Bindings    []metadataBinding
	Annotations []metadataAnnotationGroup
}

// Type-helper templates render the generated enum/bits/identityref types from a
// computed view (the per-variant logic stays in Go; the template renders the
// type + its methods). Output is gofmt-normalized by GenerateGo.

//go:embed templates/enum.go.tmpl
var tmplEnumText string

var tmplEnum = template.Must(template.New("enum").Parse(tmplEnumText))

//go:embed templates/bits.go.tmpl
var tmplBitsText string

var tmplBits = template.Must(template.New("bits").Parse(tmplBitsText))

//go:embed templates/identityref.go.tmpl
var tmplIdentityrefText string

var tmplIdentityref = template.Must(template.New("identityref").Parse(tmplIdentityrefText))

type identityrefMemberView struct {
	Const     string
	Name      string
	JSONName  string
	Foreign   bool
	Prefix    string
	Namespace string
}

type identityrefView struct {
	Name     string
	YangName string
	Members  []identityrefMemberView
}

//go:embed templates/structdef.go.tmpl
var tmplStructDefText string

var tmplStructDef = template.Must(template.New("structdef").Parse(tmplStructDefText))

//go:embed templates/fieldorder.go.tmpl
var tmplFieldOrderText string

var tmplFieldOrder = template.Must(template.New("fieldorder").Parse(tmplFieldOrderText))

//go:embed templates/union.go.tmpl
var tmplUnionText string

var tmplUnion = template.Must(template.New("union").Parse(tmplUnionText))

type structFieldView struct {
	Doc    string
	Ident  string
	GoType string
}

type structDefView struct {
	Name        string
	Fields      []structFieldView
	HasMetadata bool
}

type fieldOrderView struct {
	Name  string
	Wires []string
}

type unionMemberView struct {
	VariantType string
	Variant     string
	PayloadType string
	Wrapper     bool
	Fallback    bool
	Methods     string
}

type unionView struct {
	Name     string
	YangName string
	Members  []unionMemberView
}

type enumVariantView struct {
	Const string
	Value int64
	Name  string
}

type enumView struct {
	Name     string
	YangName string
	Variants []enumVariantView
}

type bitVariantView struct {
	Name string
	Pos  int64
}

type bitsView struct {
	Name     string
	YangName string
	Bits     []bitVariantView
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package codegen

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

func (g *goEmitter) emitJSONParseHelper(out *strings.Builder) {
	out.WriteString("const cambiumJSONMaxBytes = 64 << 20\n")
	out.WriteString("const cambiumJSONMaxDepth = 10000\n\n")
	out.WriteString("func cambiumJSONDecodeObject(data []byte) (map[string]any, error) {\n")
	out.WriteString("\tif len(data) > cambiumJSONMaxBytes {\n")
	out.WriteString("\t\treturn nil, fmt.Errorf(\"JSON_IETF document is %d bytes, exceeds maximum %d bytes\", len(data), cambiumJSONMaxBytes)\n")
	out.WriteString("\t}\n")
	out.WriteString("\tif !utf8.Valid(data) {\n")
	out.WriteString("\t\treturn nil, fmt.Errorf(\"JSON_IETF document is not valid UTF-8\")\n")
	out.WriteString("\t}\n")
	out.WriteString("\tif err := cambiumValidateJSONTextEscapes(data); err != nil {\n")
	out.WriteString("\t\treturn nil, err\n")
	out.WriteString("\t}\n")
	out.WriteString("\tdec := json.NewDecoder(strings.NewReader(string(data)))\n")
	out.WriteString("\tdec.UseNumber()\n")
	out.WriteString("\traw, err := cambiumJSONDecodeValue(dec, 0)\n")
	out.WriteString("\tif err != nil {\n")
	out.WriteString("\t\treturn nil, err\n")
	out.WriteString("\t}\n")
	out.WriteString("\tif _, err := dec.Token(); err == nil {\n")
	out.WriteString("\t\treturn nil, fmt.Errorf(\"JSON_IETF document has trailing data\")\n")
	out.WriteString("\t} else if err != io.EOF {\n")
	out.WriteString("\t\treturn nil, err\n")
	out.WriteString("\t}\n")
	out.WriteString("\treturn cambiumJSONObject(raw, \"document\")\n")
	out.WriteString("}\n\n")
	out.WriteString("func cambiumJSONDecodeValue(dec *json.Decoder, depth int) (any, error) {\n")
	out.WriteString("\tif depth > cambiumJSONMaxDepth {\n")
	out.WriteString("\t\treturn nil, fmt.Errorf(\"JSON_IETF document exceeds maximum nesting depth\")\n")
	out.WriteString("\t}\n")
	out.WriteString("\ttok, err := dec.Token()\n")
	out.WriteString("\tif err != nil {\n")
	out.WriteString("\t\treturn nil, err\n")
	out.WriteString("\t}\n")
	out.WriteString("\tswitch v := tok.(type) {\n")
	out.WriteString("\tcase json.Delim:\n")
	out.WriteString("\t\tswitch v {\n")
	out.WriteString("\t\tcase '{':\n")
	out.WriteString("\t\t\tobj := make(map[string]any)\n")
	out.WriteString("\t\t\tfor dec.More() {\n")
	out.WriteString("\t\t\t\tkeyTok, err := dec.Token()\n")
	out.WriteString("\t\t\t\tif err != nil {\n")
	out.WriteString("\t\t\t\t\treturn nil, err\n")
	out.WriteString("\t\t\t\t}\n")
	out.WriteString("\t\t\t\tkey, ok := keyTok.(string)\n")
	out.WriteString("\t\t\t\tif !ok {\n")
	out.WriteString("\t\t\t\t\treturn nil, fmt.Errorf(\"JSON_IETF object member name must be a string\")\n")
	out.WriteString("\t\t\t\t}\n")
	out.WriteString("\t\t\t\tif err := cambiumValidateJSONStringChars(key, \"JSON_IETF object member name\"); err != nil {\n")
	out.WriteString("\t\t\t\t\treturn nil, err\n")
	out.WriteString("\t\t\t\t}\n")
	out.WriteString("\t\t\t\tif _, exists := obj[key]; exists {\n")
	out.WriteString("\t\t\t\t\treturn nil, fmt.Errorf(\"duplicate JSON_IETF field %q\", key)\n")
	out.WriteString("\t\t\t\t}\n")
	out.WriteString("\t\t\t\tvalue, err := cambiumJSONDecodeValue(dec, depth+1)\n")
	out.WriteString("\t\t\t\tif err != nil {\n")
	out.WriteString("\t\t\t\t\treturn nil, err\n")
	out.WriteString("\t\t\t\t}\n")
	out.WriteString("\t\t\t\tobj[key] = value\n")
	out.WriteString("\t\t\t}\n")
	out.WriteString("\t\t\tend, err := dec.Token()\n")
	out.WriteString("\t\t\tif err != nil {\n")
	out.WriteString("\t\t\t\treturn nil, err\n")
	out.WriteString("\t\t\t}\n")
	out.WriteString("\t\t\tif end != json.Delim('}') {\n")
	out.WriteString("\t\t\t\treturn nil, fmt.Errorf(\"JSON_IETF object is not closed\")\n")
	out.WriteString("\t\t\t}\n")
	out.WriteString("\t\t\treturn obj, nil\n")
	out.WriteString("\t\tcase '[':\n")
	out.WriteString("\t\t\tarr := make([]any, 0)\n")
	out.WriteString("\t\t\tfor dec.More() {\n")
	out.WriteString("\t\t\t\tvalue, err := cambiumJSONDecodeValue(dec, depth+1)\n")
	out.WriteString("\t\t\t\tif err != nil {\n")
	out.WriteString("\t\t\t\t\treturn nil, err\n")
	out.WriteString("\t\t\t\t}\n")
	out.WriteString("\t\t\t\tarr = append(arr, value)\n")
	out.WriteString("\t\t\t}\n")
	out.WriteString("\t\t\tend, err := dec.Token()\n")
	out.WriteString("\t\t\tif err != nil {\n")
	out.WriteString("\t\t\t\treturn nil, err\n")
	out.WriteString("\t\t\t}\n")
	out.WriteString("\t\t\tif end != json.Delim(']') {\n")
	out.WriteString("\t\t\t\treturn nil, fmt.Errorf(\"JSON_IETF array is not closed\")\n")
	out.WriteString("\t\t\t}\n")
	out.WriteString("\t\t\treturn arr, nil\n")
	out.WriteString("\t\tdefault:\n")
	out.WriteString("\t\t\treturn nil, fmt.Errorf(\"unexpected JSON_IETF delimiter %q\", v)\n")
	out.WriteString("\t\t}\n")
	out.WriteString("\tcase string:\n")
	out.WriteString("\t\tif err := cambiumValidateJSONStringChars(v, \"JSON_IETF string\"); err != nil {\n")
	out.WriteString("\t\t\treturn nil, err\n")
	out.WriteString("\t\t}\n")
	out.WriteString("\t\treturn v, nil\n")
	out.WriteString("\tdefault:\n")
	out.WriteString("\t\treturn v, nil\n")
	out.WriteString("\t}\n")
	out.WriteString("}\n\n")
	out.WriteString("func cambiumValidateJSONTextEscapes(data []byte) error {\n")
	out.WriteString("\tinString := false\n")
	out.WriteString("\tfor i := 0; i < len(data); i++ {\n")
	out.WriteString("\t\tswitch data[i] {\n")
	out.WriteString("\t\tcase '\"':\n")
	out.WriteString("\t\t\tinString = !inString\n")
	out.WriteString("\t\tcase '\\\\':\n")
	out.WriteString("\t\t\tif !inString || i+1 >= len(data) { continue }\n")
	out.WriteString("\t\t\tnext := data[i+1]\n")
	out.WriteString("\t\t\tif next != 'u' {\n")
	out.WriteString("\t\t\t\ti++\n")
	out.WriteString("\t\t\t\tcontinue\n")
	out.WriteString("\t\t\t}\n")
	out.WriteString("\t\t\tif i+5 >= len(data) { continue }\n")
	out.WriteString("\t\t\tvalue, err := strconv.ParseUint(string(data[i+2:i+6]), 16, 32)\n")
	out.WriteString("\t\t\tif err == nil {\n")
	out.WriteString("\t\t\t\tr := rune(value)\n")
	out.WriteString("\t\t\t\tif r >= 0xd800 && r <= 0xdbff {\n")
	out.WriteString("\t\t\t\t\tif i+11 >= len(data) || data[i+6] != '\\\\' || data[i+7] != 'u' {\n")
	out.WriteString("\t\t\t\t\t\treturn fmt.Errorf(\"JSON_IETF string contains invalid character escape \\\\u%04X\", value)\n")
	out.WriteString("\t\t\t\t\t}\n")
	out.WriteString("\t\t\t\t\tlow, lowErr := strconv.ParseUint(string(data[i+8:i+12]), 16, 32)\n")
	out.WriteString("\t\t\t\t\tif lowErr != nil || low < 0xdc00 || low > 0xdfff {\n")
	out.WriteString("\t\t\t\t\t\treturn fmt.Errorf(\"JSON_IETF string contains invalid character escape \\\\u%04X\", value)\n")
	out.WriteString("\t\t\t\t\t}\n")
	out.WriteString("\t\t\t\t\tcombined := 0x10000 + ((r - 0xd800) << 10) + (rune(low) - 0xdc00)\n")
	out.WriteString("\t\t\t\t\tif !cambiumValidJSONStringRune(combined) {\n")
	out.WriteString("\t\t\t\t\t\treturn fmt.Errorf(\"JSON_IETF string contains invalid character escape \\\\u%04X\", value)\n")
	out.WriteString("\t\t\t\t\t}\n")
	out.WriteString("\t\t\t\t\ti += 11\n")
	out.WriteString("\t\t\t\t\tcontinue\n")
	out.WriteString("\t\t\t\t}\n")
	out.WriteString("\t\t\t\tif !cambiumValidJSONStringRune(r) {\n")
	out.WriteString("\t\t\t\t\treturn fmt.Errorf(\"JSON_IETF string contains invalid character escape \\\\u%04X\", value)\n")
	out.WriteString("\t\t\t\t}\n")
	out.WriteString("\t\t\t}\n")
	out.WriteString("\t\t\ti += 5\n")
	out.WriteString("\t\t}\n")
	out.WriteString("\t}\n")
	out.WriteString("\treturn nil\n")
	out.WriteString("}\n\n")
	out.WriteString("func cambiumValidateJSONStringChars(s, name string) error {\n")
	out.WriteString("\tif err := cambiumValidateStringValue(s); err != nil { return fmt.Errorf(\"%s %v\", name, err) }\n")
	out.WriteString("\treturn nil\n")
	out.WriteString("}\n\n")
	out.WriteString("func cambiumValidateStringValue(s string) error {\n")
	out.WriteString("\tif !utf8.ValidString(s) { return fmt.Errorf(\"string value is not valid UTF-8\") }\n")
	out.WriteString("\tfor _, r := range s {\n")
	out.WriteString("\t\tif !cambiumValidJSONStringRune(r) { return fmt.Errorf(\"invalid string character U+%04X\", r) }\n")
	out.WriteString("\t}\n")
	out.WriteString("\treturn nil\n")
	out.WriteString("}\n\n")
	out.WriteString("func cambiumValidJSONStringRune(r rune) bool {\n")
	out.WriteString("\tif r < 0x20 {\n")
	out.WriteString("\t\treturn r == '\\t' || r == '\\n' || r == '\\r'\n")
	out.WriteString("\t}\n")
	out.WriteString("\tif r >= 0xd800 && r <= 0xdfff {\n")
	out.WriteString("\t\treturn false\n")
	out.WriteString("\t}\n")
	out.WriteString("\tif r >= 0xfdd0 && r <= 0xfdef {\n")
	out.WriteString("\t\treturn false\n")
	out.WriteString("\t}\n")
	out.WriteString("\tif r >= 0xfffe && r <= 0x10ffff && (r&0xffe) == 0xffe {\n")
	out.WriteString("\t\treturn false\n")
	out.WriteString("\t}\n")
	out.WriteString("\treturn r <= 0x10ffff\n")
	out.WriteString("}\n\n")
	out.WriteString("func cambiumJSONMarshalIndent(raw any) (string, error) {\n")
	out.WriteString("\tvar b strings.Builder\n")
	out.WriteString("\tenc := json.NewEncoder(&b)\n")
	out.WriteString("\tenc.SetEscapeHTML(false)\n")
	out.WriteString("\tenc.SetIndent(\"\", \"  \")\n")
	out.WriteString("\tif err := enc.Encode(raw); err != nil {\n")
	out.WriteString("\t\treturn \"\", err\n")
	out.WriteString("\t}\n")
	out.WriteString("\treturn strings.TrimSuffix(b.String(), \"\\n\"), nil\n")
	out.WriteString("}\n\n")
	out.WriteString("func cambiumJSONObject(raw any, name string) (map[string]any, error) {\n")
	out.WriteString("\tobj, ok := raw.(map[string]any)\n")
	out.WriteString("\tif !ok {\n")
	out.WriteString("\t\treturn nil, fmt.Errorf(\"%s must be a JSON object\", name)\n")
	out.WriteString("\t}\n")
	out.WriteString("\treturn obj, nil\n")
	out.WriteString("}\n\n")
	out.WriteString("func cambiumSortedJSONKeys(obj map[string]any) []string {\n")
	out.WriteString("\tkeys := make([]string, 0, len(obj))\n")
	out.WriteString("\tfor key := range obj { keys = append(keys, key) }\n")
	out.WriteString("\tsort.Strings(keys)\n")
	out.WriteString("\treturn keys\n")
	out.WriteString("}\n\n")
	out.WriteString("func cambiumJSONArray(raw any, name string) ([]any, error) {\n")
	out.WriteString("\tarr, ok := raw.([]any)\n")
	out.WriteString("\tif !ok {\n")
	out.WriteString("\t\treturn nil, fmt.Errorf(\"%s must be a JSON array\", name)\n")
	out.WriteString("\t}\n")
	out.WriteString("\treturn arr, nil\n")
	out.WriteString("}\n\n")
	out.WriteString("func cambiumJSONString(raw any, name string) (string, error) {\n")
	out.WriteString("\ts, ok := raw.(string)\n")
	out.WriteString("\tif !ok {\n")
	out.WriteString("\t\treturn \"\", fmt.Errorf(\"%s must be a JSON string\", name)\n")
	out.WriteString("\t}\n")
	out.WriteString("\treturn s, nil\n")
	out.WriteString("}\n\n")
	out.WriteString("func cambiumJSONBool(raw any, name string) (bool, error) {\n")
	out.WriteString("\tb, ok := raw.(bool)\n")
	out.WriteString("\tif !ok {\n")
	out.WriteString("\t\treturn false, fmt.Errorf(\"%s must be a JSON boolean\", name)\n")
	out.WriteString("\t}\n")
	out.WriteString("\treturn b, nil\n")
	out.WriteString("}\n\n")
	out.WriteString("func cambiumTrimJSONNumericSpace(s string) string {\n")
	out.WriteString("\tstart, end := 0, len(s)\n")
	out.WriteString("\tfor start < end && cambiumIsJSONNumericSpace(s[start]) { start++ }\n")
	out.WriteString("\tfor start < end && cambiumIsJSONNumericSpace(s[end-1]) { end-- }\n")
	out.WriteString("\treturn s[start:end]\n")
	out.WriteString("}\n\n")
	out.WriteString("func cambiumIsJSONNumericSpace(b byte) bool {\n")
	out.WriteString("\tswitch b {\n")
	out.WriteString("\tcase ' ', '\\t', '\\n', '\\r':\n")
	out.WriteString("\t\treturn true\n")
	out.WriteString("\tdefault:\n")
	out.WriteString("\t\treturn false\n")
	out.WriteString("\t}\n")
	out.WriteString("}\n\n")
	out.WriteString("func cambiumJSONInt(raw any, name string, bits int, quoted bool) (int64, error) {\n")
	out.WriteString("\tif quoted {\n")
	out.WriteString("\t\ts, err := cambiumJSONString(raw, name)\n")
	out.WriteString("\t\tif err != nil { return 0, err }\n")
	out.WriteString("\t\ts = cambiumTrimJSONNumericSpace(s)\n")
	out.WriteString("\t\treturn strconv.ParseInt(s, 10, bits)\n")
	out.WriteString("\t}\n")
	out.WriteString("\tn, ok := raw.(json.Number)\n")
	out.WriteString("\tif !ok {\n")
	out.WriteString("\t\treturn 0, fmt.Errorf(\"%s must be a JSON integer\", name)\n")
	out.WriteString("\t}\n")
	out.WriteString("\treturn strconv.ParseInt(n.String(), 10, bits)\n")
	out.WriteString("}\n\n")
	out.WriteString("func cambiumJSONUint(raw any, name string, bits int, quoted bool) (uint64, error) {\n")
	out.WriteString("\tif quoted {\n")
	out.WriteString("\t\ts, err := cambiumJSONString(raw, name)\n")
	out.WriteString("\t\tif err != nil { return 0, err }\n")
	out.WriteString("\t\ts = cambiumTrimJSONNumericSpace(s)\n")
	out.WriteString("\t\treturn cambiumParseJSONUint(s, bits)\n")
	out.WriteString("\t}\n")
	out.WriteString("\tn, ok := raw.(json.Number)\n")
	out.WriteString("\tif !ok {\n")
	out.WriteString("\t\treturn 0, fmt.Errorf(\"%s must be a JSON unsigned integer\", name)\n")
	out.WriteString("\t}\n")
	out.WriteString("\treturn cambiumParseJSONUint(n.String(), bits)\n")
	out.WriteString("}\n\n")
	out.WriteString("func cambiumParseJSONUint(s string, bits int) (uint64, error) {\n")
	out.WriteString("\tif strings.HasPrefix(s, \"+\") {\n")
	out.WriteString("\t\treturn strconv.ParseUint(strings.TrimPrefix(s, \"+\"), 10, bits)\n")
	out.WriteString("\t}\n")
	out.WriteString("\tif strings.HasPrefix(s, \"-\") {\n")
	out.WriteString("\t\tv, err := strconv.ParseUint(strings.TrimPrefix(s, \"-\"), 10, bits)\n")
	out.WriteString("\t\tif err == nil && v == 0 { return 0, nil }\n")
	out.WriteString("\t\t_, err = strconv.ParseUint(s, 10, bits)\n")
	out.WriteString("\t\treturn 0, err\n")
	out.WriteString("\t}\n")
	out.WriteString("\treturn strconv.ParseUint(s, 10, bits)\n")
	out.WriteString("}\n\n")
	if g.emittedDecimal64 {
		out.WriteString("func cambiumJSONDecimal64(raw any, name string, fractionDigits uint8) (Decimal64, error) {\n")
		out.WriteString("\ts, err := cambiumJSONString(raw, name)\n")
		out.WriteString("\tif err != nil { return Decimal64{}, err }\n")
		out.WriteString("\toriginal := s\n")
		out.WriteString("\ts = cambiumTrimJSONNumericSpace(s)\n")
		out.WriteString("\tinvalid := func() (Decimal64, error) { return Decimal64{}, fmt.Errorf(\"%s has invalid decimal64 value %q\", name, original) }\n")
		out.WriteString("\tneg := false\n")
		out.WriteString("\tif strings.HasPrefix(s, \"-\") { neg = true; s = strings.TrimPrefix(s, \"-\") } else if strings.HasPrefix(s, \"+\") { s = strings.TrimPrefix(s, \"+\") }\n")
		out.WriteString("\twholeText, fracText, hasFraction := strings.Cut(s, \".\")\n")
		out.WriteString("\tif wholeText == \"\" || (hasFraction && fracText == \"\") { return invalid() }\n")
		out.WriteString("\tif !cambiumDecimal64DigitsOnly(wholeText) || (fracText != \"\" && !cambiumDecimal64DigitsOnly(fracText)) { return invalid() }\n")
		out.WriteString("\tif len(fracText) > int(fractionDigits) { return Decimal64{}, fmt.Errorf(\"%s exceeds decimal64 fraction-digits\", name) }\n")
		out.WriteString("\tfor len(fracText) < int(fractionDigits) { fracText += \"0\" }\n")
		out.WriteString("\twhole := new(big.Int)\n")
		out.WriteString("\tif _, ok := whole.SetString(wholeText, 10); !ok { return invalid() }\n")
		out.WriteString("\tscale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(fractionDigits)), nil)\n")
		out.WriteString("\trawValue := new(big.Int).Mul(whole, scale)\n")
		out.WriteString("\tif fracText != \"\" {\n")
		out.WriteString("\t\tfrac := new(big.Int)\n")
		out.WriteString("\t\tif _, ok := frac.SetString(fracText, 10); !ok { return invalid() }\n")
		out.WriteString("\t\trawValue.Add(rawValue, frac)\n")
		out.WriteString("\t}\n")
		out.WriteString("\tif neg { rawValue.Neg(rawValue) }\n")
		out.WriteString("\tif !rawValue.IsInt64() { return Decimal64{}, fmt.Errorf(\"%s decimal64 value %q is outside decimal64 range\", name, original) }\n")
		out.WriteString("\treturn NewDecimal64(rawValue.Int64(), fractionDigits), nil\n")
		out.WriteString("}\n\n")
		out.WriteString("func cambiumDecimal64DigitsOnly(s string) bool {\n")
		out.WriteString("\tif s == \"\" { return false }\n")
		out.WriteString("\tfor _, r := range s { if r < '0' || r > '9' { return false } }\n")
		out.WriteString("\treturn true\n")
		out.WriteString("}\n\n")
	}
	out.WriteString("func cambiumJSONEmpty(raw any, name string) error {\n")
	out.WriteString("\tarr, err := cambiumJSONArray(raw, name)\n")
	out.WriteString("\tif err != nil {\n")
	out.WriteString("\t\treturn err\n")
	out.WriteString("\t}\n")
	out.WriteString("\tif len(arr) != 1 || arr[0] != nil {\n")
	out.WriteString("\t\treturn fmt.Errorf(\"%s must be [null]\", name)\n")
	out.WriteString("\t}\n")
	out.WriteString("\treturn nil\n")
	out.WriteString("}\n\n")
}

func (g *goEmitter) emitStructParser(name string, fields []fieldInfo, currentModule string, out *strings.Builder) {
	g.helpers["jsonParse"] = true
	fmt.Fprintf(out, "func (n *%s) parseJSONIETF(obj map[string]any) error {\n", name)
	out.WriteString("\tif n == nil { return nil }\n")
	byNode := fieldsByNode(fields)
	g.emitJSONUnknownFieldCheck(fields, func(f fieldInfo) string {
		return jsonWireName(f, currentModule)
	}, out)
	for _, f := range fields {
		g.emitJSONParseField("n", jsonWireName(f, currentModule), f, byNode, out)
	}
	out.WriteString("\treturn nil\n")
	out.WriteString("}\n\n")
}

func (g *goEmitter) emitRootParser(name string, fields []fieldInfo, out *strings.Builder) {
	g.helpers["jsonParse"] = true
	fmt.Fprintf(out, "func FromJSONIETF(data []byte) (*%s, error) {\n", name)
	out.WriteString("\tobj, err := cambiumJSONDecodeObject(data)\n")
	out.WriteString("\tif err != nil { return nil, err }\n")
	fmt.Fprintf(out, "\tout := &%s{}\n", name)
	out.WriteString("\tif err := out.parseJSONIETF(obj); err != nil { return nil, err }\n")
	out.WriteString("\tif err := out.Validate(); err != nil { return nil, err }\n")
	out.WriteString("\treturn out, nil\n")
	out.WriteString("}\n\n")

	fmt.Fprintf(out, "func (m *%s) parseJSONIETF(obj map[string]any) error {\n", name)
	out.WriteString("\tif m == nil { return nil }\n")
	byNode := fieldsByNode(fields)
	g.emitJSONUnknownFieldCheck(fields, func(f fieldInfo) string {
		moduleName := f.moduleName
		if moduleName == "" {
			moduleName = g.moduleName
		}
		return moduleName + ":" + f.wire
	}, out)
	for _, f := range fields {
		moduleName := f.moduleName
		if moduleName == "" {
			moduleName = g.moduleName
		}
		g.emitJSONParseField("m", moduleName+":"+f.wire, f, byNode, out)
	}
	out.WriteString("\treturn nil\n")
	out.WriteString("}\n\n")
}

func (g *goEmitter) emitOperationRootParser(name string, op cambium.SchemaNodeRef, out *strings.Builder) {
	g.helpers["jsonParse"] = true
	key := op.Module().Name() + ":" + op.Name()
	fmt.Fprintf(out, "func From%sJSONIETF(data []byte) (*%s, error) {\n", name, name)
	out.WriteString("\tobj, err := cambiumJSONDecodeObject(data)\n")
	out.WriteString("\tif err != nil { return nil, err }\n")
	fmt.Fprintf(out, "\tconst rootKey = %q\n", key)
	out.WriteString("\tfor _, key := range cambiumSortedJSONKeys(obj) {\n")
	out.WriteString("\t\tif key != rootKey {\n")
	out.WriteString("\t\t\treturn nil, fmt.Errorf(\"unknown JSON_IETF field %q\", key)\n")
	out.WriteString("\t\t}\n")
	out.WriteString("\t}\n")
	out.WriteString("\traw, ok := obj[rootKey]\n")
	out.WriteString("\tif !ok {\n")
	out.WriteString("\t\treturn nil, fmt.Errorf(\"missing JSON_IETF field %q\", rootKey)\n")
	out.WriteString("\t}\n")
	out.WriteString("\tchildObj, err := cambiumJSONObject(raw, rootKey)\n")
	out.WriteString("\tif err != nil { return nil, err }\n")
	fmt.Fprintf(out, "\tout := &%s{}\n", name)
	out.WriteString("\tif err := out.parseJSONIETF(childObj); err != nil { return nil, err }\n")
	out.WriteString("\tif err := out.Validate(); err != nil { return nil, err }\n")
	out.WriteString("\treturn out, nil\n")
	out.WriteString("}\n\n")
}

func (g *goEmitter) emitJSONUnknownFieldCheck(fields []fieldInfo, keyFor func(fieldInfo) string, out *strings.Builder) {
	out.WriteString("\tfor _, key := range cambiumSortedJSONKeys(obj) {\n")
	out.WriteString("\t\tswitch key {\n")
	for _, f := range fields {
		key := keyFor(f)
		labels := []string{strconv.Quote(key)}
		switch f.node.Kind() {
		case cambium.SchemaNodeKindLeaf, cambium.SchemaNodeKindLeafList:
			labels = append(labels, strconv.Quote("@"+key))
		}
		fmt.Fprintf(out, "\t\tcase %s:\n", strings.Join(labels, ", "))
	}
	out.WriteString("\t\tdefault:\n")
	out.WriteString("\t\t\treturn fmt.Errorf(\"unknown JSON_IETF field %q\", key)\n")
	out.WriteString("\t\t}\n")
	out.WriteString("\t}\n")
}

func (g *goEmitter) emitJSONParseField(owner, key string, f fieldInfo, byNode map[cambium.SchemaNodeRef]fieldInfo, out *strings.Builder) {
	keyExpr := strconv.Quote(key)
	fmt.Fprintf(out, "\tif raw, ok := obj[%s]; ok {\n", keyExpr)
	out.WriteString("\t\t_ = raw\n")
	switch f.node.Kind() {
	case cambium.SchemaNodeKindLeaf:
		if g.emitJSONParseScalarValue("v", "raw", keyExpr, f, out, "\t\t") {
			if f.optional && !f.isUnion {
				fmt.Fprintf(out, "\t\t%s.%s = &v\n", owner, f.ident)
			} else {
				fmt.Fprintf(out, "\t\t%s.%s = v\n", owner, f.ident)
			}
		}
	case cambium.SchemaNodeKindLeafList:
		base := fieldConcreteType(f)
		fmt.Fprintf(out, "\t\tarr, err := cambiumJSONArray(raw, %s)\n", keyExpr)
		out.WriteString("\t\tif err != nil { return err }\n")
		fmt.Fprintf(out, "\t\titems := make([]%s, 0, len(arr))\n", base)
		out.WriteString("\t\tfor _, item := range arr {\n")
		if g.emitJSONParseScalarValue("v", "item", keyExpr, f, out, "\t\t\t") {
			out.WriteString("\t\t\titems = append(items, v)\n")
		}
		out.WriteString("\t\t}\n")
		if strings.HasPrefix(f.goType, "UserOrderedVec[") {
			fmt.Fprintf(out, "\t\t%s.%s = NewUserOrderedVec(items)\n", owner, f.ident)
		} else {
			fmt.Fprintf(out, "\t\t%s.%s = items\n", owner, f.ident)
		}
	case cambium.SchemaNodeKindContainer, cambium.SchemaNodeKindAction, cambium.SchemaNodeKindNotification:
		targetType := fieldConcreteType(f)
		fmt.Fprintf(out, "\t\tchildObj, err := cambiumJSONObject(raw, %s)\n", keyExpr)
		out.WriteString("\t\tif err != nil { return err }\n")
		fmt.Fprintf(out, "\t\tchild := %s{}\n", targetType)
		out.WriteString("\t\tif err := child.parseJSONIETF(childObj); err != nil { return err }\n")
		if f.optional {
			fmt.Fprintf(out, "\t\t%s.%s = &child\n", owner, f.ident)
		} else {
			fmt.Fprintf(out, "\t\t%s.%s = child\n", owner, f.ident)
		}
	case cambium.SchemaNodeKindList:
		entryType := fieldConcreteType(f)
		fmt.Fprintf(out, "\t\tarr, err := cambiumJSONArray(raw, %s)\n", keyExpr)
		out.WriteString("\t\tif err != nil { return err }\n")
		fmt.Fprintf(out, "\t\titems := make([]%s, 0, len(arr))\n", entryType)
		out.WriteString("\t\tfor _, item := range arr {\n")
		fmt.Fprintf(out, "\t\t\tchildObj, err := cambiumJSONObject(item, %s)\n", keyExpr)
		out.WriteString("\t\t\tif err != nil { return err }\n")
		fmt.Fprintf(out, "\t\t\tchild := %s{}\n", entryType)
		out.WriteString("\t\t\tif err := child.parseJSONIETF(childObj); err != nil { return err }\n")
		out.WriteString("\t\t\titems = append(items, child)\n")
		out.WriteString("\t\t}\n")
		if strings.HasPrefix(f.goType, "UserOrderedVec[") {
			fmt.Fprintf(out, "\t\t%s.%s = NewUserOrderedVec(items)\n", owner, f.ident)
		} else {
			fmt.Fprintf(out, "\t\t%s.%s = items\n", owner, f.ident)
		}
	case cambium.SchemaNodeKindAnyData:
		out.WriteString("\t\tb, err := cambiumJSONMarshalIndent(raw)\n")
		out.WriteString("\t\tif err != nil { return err }\n")
		out.WriteString("\t\tv := NewAnyData(\"\", b)\n")
		if f.optional {
			fmt.Fprintf(out, "\t\t%s.%s = &v\n", owner, f.ident)
		} else {
			fmt.Fprintf(out, "\t\t%s.%s = v\n", owner, f.ident)
		}
	}
	g.emitExplicitChoiceCaseParseValidation(owner, f, byNode, out, "\t\t")
	out.WriteString("\t}\n")
	if requiresJSONField(f.node) {
		fmt.Fprintf(out, "\tif _, ok := obj[%s]; !ok {\n", keyExpr)
		fmt.Fprintf(out, "\t\treturn fmt.Errorf(\"missing JSON_IETF field %%q\", %s)\n", keyExpr)
		out.WriteString("\t}\n")
	}
	switch f.node.Kind() {
	case cambium.SchemaNodeKindLeaf, cambium.SchemaNodeKindLeafList:
		module := f.node.Module()
		fmt.Fprintf(out, "\tif rawMeta, ok := obj[%q]; ok {\n", "@"+key)
		fmt.Fprintf(out, "\t\tif _, dataOK := obj[%s]; !dataOK { return fmt.Errorf(\"metadata JSON member %%q has no corresponding data node %%q\", %q, %s) }\n", keyExpr, "@"+key, keyExpr)
		fmt.Fprintf(out, "\t\titems, err := cambiumParseMetadataJSON(rawMeta, %q, %q, %q)\n", module.Name(), module.Prefix(), module.Namespace())
		out.WriteString("\t\tif err != nil { return err }\n")
		fmt.Fprintf(out, "\t\tif %s.CambiumMetadata == nil { %s.CambiumMetadata = make(map[string][]MetadataAnnotation) }\n", owner, owner)
		fmt.Fprintf(out, "\t\t%s.CambiumMetadata[%q] = items\n", owner, f.wire)
		out.WriteString("\t}\n")
	}
}

func schemaPathElements(node cambium.SchemaNodeRef) []string {
	path := strings.Trim(node.Path(), "/")
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}

func requiresJSONField(node cambium.SchemaNodeRef) bool {
	switch node.Kind() {
	case cambium.SchemaNodeKindLeaf:
		return requiresJSONLeafField(node)
	case cambium.SchemaNodeKindAnyData:
		return node.IsMandatory() && !node.IsChoiceDescendant() && len(node.Whens()) == 0
	case cambium.SchemaNodeKindContainer:
		return !node.IsPresenceContainer() && len(node.Whens()) == 0 && hasRequiredJSONDescendant(node)
	default:
		return false
	}
}

func requiresJSONLeafField(node cambium.SchemaNodeRef) bool {
	if node.Kind() != cambium.SchemaNodeKindLeaf {
		return false
	}
	if node.IsListKey() {
		return true
	}
	return node.IsMandatory() && !node.IsChoiceDescendant() && len(node.Whens()) == 0
}

func hasRequiredJSONDescendant(node cambium.SchemaNodeRef) bool {
	for child := range node.Children().Iter() {
		if len(child.Whens()) > 0 {
			continue
		}
		switch child.Kind() {
		case cambium.SchemaNodeKindLeaf:
			if requiresJSONLeafField(child) {
				return true
			}
		case cambium.SchemaNodeKindAnyData:
			if child.IsMandatory() && !child.IsChoiceDescendant() {
				return true
			}
		case cambium.SchemaNodeKindContainer:
			if !child.IsPresenceContainer() && hasRequiredJSONDescendant(child) {
				return true
			}
		case cambium.SchemaNodeKindChoice, cambium.SchemaNodeKindCase:
			// Choice/case descendants are governed by generated choice validation.
			continue
		}
	}
	return false
}

func (g *goEmitter) emitJSONParseScalarValue(varName, rawExpr, keyExpr string, f fieldInfo, out *strings.Builder, indent string) bool {
	base := fieldConcreteType(f)
	unsupported := func(reason string) bool {
		g.recordEmitError(fmt.Errorf("unsupported JSON_IETF parser for %s: %s", codegenNodePath(f.node), reason))
		return false
	}
	if f.isUnion {
		return g.emitJSONParseUnionValue(varName, rawExpr, keyExpr, f, out, indent)
	}
	if f.isIdentityref || f.isEnum {
		fmt.Fprintf(out, "%ss, err := cambiumJSONString(%s, %s)\n", indent, rawExpr, keyExpr)
		out.WriteString(indent + "if err != nil { return err }\n")
		fmt.Fprintf(out, "%s%s, ok := Parse%s(s)\n", indent, varName, base)
		fmt.Fprintf(out, "%sif !ok { return fmt.Errorf(\"%%s has invalid identity or enum value %%q\", %s, s) }\n", indent, keyExpr)
		return true
	}
	if f.isBits {
		fmt.Fprintf(out, "%ss, err := cambiumJSONString(%s, %s)\n", indent, rawExpr, keyExpr)
		out.WriteString(indent + "if err != nil { return err }\n")
		fmt.Fprintf(out, "%sbitNames, err := cambiumBitsNames(s)\n", indent)
		out.WriteString(indent + "if err != nil { return err }\n")
		fmt.Fprintf(out, "%s%s, err := New%s(bitNames)\n", indent, varName, base)
		out.WriteString(indent + "if err != nil { return err }\n")
		return true
	}
	switch f.jsonKind {
	case "Empty":
		fmt.Fprintf(out, "%sif err := cambiumJSONEmpty(%s, %s); err != nil { return err }\n", indent, rawExpr, keyExpr)
		fmt.Fprintf(out, "%s%s := struct{}{}\n", indent, varName)
		return true
	case "InstanceIdentifier":
		fmt.Fprintf(out, "%ss, err := cambiumJSONString(%s, %s)\n", indent, rawExpr, keyExpr)
		out.WriteString(indent + "if err != nil { return err }\n")
		fmt.Fprintf(out, "%s%s := NewInstanceIdentifier(s)\n", indent, varName)
		return true
	case "String":
		if info, ok := binaryTypeInfo(f.node); ok {
			g.helpers["binaryParse"] = true
			fmt.Fprintf(out, "%s%s, err := cambiumJSONBinary(%s, %s, %s)\n", indent, varName, rawExpr, keyExpr, binaryLengthRangeLiteral(info))
			out.WriteString(indent + "if err != nil { return err }\n")
			return true
		}
		fmt.Fprintf(out, "%ss, err := cambiumJSONString(%s, %s)\n", indent, rawExpr, keyExpr)
		out.WriteString(indent + "if err != nil { return err }\n")
		if f.isNewtype {
			fmt.Fprintf(out, "%s%s, err := New%s(s)\n", indent, varName, base)
			out.WriteString(indent + "if err != nil { return err }\n")
		} else if base == "string" {
			fmt.Fprintf(out, "%s%s := s\n", indent, varName)
		} else {
			fmt.Fprintf(out, "%s%s := %s(s)\n", indent, varName, base)
		}
		if info, ok := stringPatternInfo(f.node); ok {
			g.helpers["patternValidate"] = true
			fmt.Fprintf(out, "%sif err := cambiumValidateStringPatterns(s, %s); err != nil { return fmt.Errorf(\"%%s %%v\", %s, err) }\n", indent, stringPatternLiteral(info), keyExpr)
		}
		return true
	case "Bool":
		fmt.Fprintf(out, "%s%s, err := cambiumJSONBool(%s, %s)\n", indent, varName, rawExpr, keyExpr)
		out.WriteString(indent + "if err != nil { return err }\n")
		return true
	case "BareNumber", "QuotedNumber":
		if base == "Decimal64" {
			info, ok := decimal64TypeInfo(f.node)
			if !ok {
				return unsupported("missing decimal64 type info")
			}
			fd, ranges := decimal64ValidationArgs(info)
			fmt.Fprintf(out, "%s%s, err := cambiumJSONDecimal64(%s, %s, %d)\n", indent, varName, rawExpr, keyExpr, fd)
			out.WriteString(indent + "if err != nil { return err }\n")
			fmt.Fprintf(out, "%sif err := cambiumValidateDecimal64Value(%s, %d, %s); err != nil { return fmt.Errorf(\"%%s %%v\", %s, err) }\n", indent, varName, fd, ranges, keyExpr)
			return true
		}
		quoted := f.jsonKind == "QuotedNumber"
		if f.isNewtype {
			info, ok := g.intRangeTypes[base]
			if !ok {
				return unsupported("missing integer range type info")
			}
			underlying := goTypeForIntKind(info.kind)
			if isSignedIntKind(info.kind) {
				fmt.Fprintf(out, "%sparsed, err := cambiumJSONInt(%s, %s, %d, %t)\n", indent, rawExpr, keyExpr, intKindBitSize(info.kind), quoted)
				out.WriteString(indent + "if err != nil { return err }\n")
				fmt.Fprintf(out, "%s%s, err := New%s(%s(parsed))\n", indent, varName, base, underlying)
			} else {
				fmt.Fprintf(out, "%sparsed, err := cambiumJSONUint(%s, %s, %d, %t)\n", indent, rawExpr, keyExpr, intKindBitSize(info.kind), quoted)
				out.WriteString(indent + "if err != nil { return err }\n")
				fmt.Fprintf(out, "%s%s, err := New%s(%s(parsed))\n", indent, varName, base, underlying)
			}
			out.WriteString(indent + "if err != nil { return err }\n")
			return true
		}
		if f.intSigned {
			fmt.Fprintf(out, "%sparsed, err := cambiumJSONInt(%s, %s, %d, %t)\n", indent, rawExpr, keyExpr, intBitSizeForGoType(base), quoted)
			out.WriteString(indent + "if err != nil { return err }\n")
			fmt.Fprintf(out, "%s%s := %s(parsed)\n", indent, varName, base)
		} else {
			fmt.Fprintf(out, "%sparsed, err := cambiumJSONUint(%s, %s, %d, %t)\n", indent, rawExpr, keyExpr, intBitSizeForGoType(base), quoted)
			out.WriteString(indent + "if err != nil { return err }\n")
			fmt.Fprintf(out, "%s%s := %s(parsed)\n", indent, varName, base)
		}
		return true
	default:
		return unsupported("unknown JSON kind " + strconv.Quote(f.jsonKind))
	}
}

func (g *goEmitter) emitJSONParseUnionValue(varName, rawExpr, keyExpr string, f fieldInfo, out *strings.Builder, indent string) bool {
	info, ok := f.node.LeafType()
	if !ok {
		g.recordEmitError(fmt.Errorf("unsupported JSON_IETF parser for %s: missing union type info", codegenNodePath(f.node)))
		return false
	}
	union, ok := resolvedUnionType(info)
	if !ok {
		g.recordEmitError(fmt.Errorf("unsupported JSON_IETF parser for %s: unresolved union type", codegenNodePath(f.node)))
		return false
	}
	unionName := fieldConcreteType(f)
	fmt.Fprintf(out, "%svar %s %s\n", indent, varName, unionName)
	out.WriteString(indent + "matched := false\n")
	used := make(map[string]bool)
	for _, member := range union.Members() {
		variant := safeVariantIdent(unionMemberLabel(member), used)
		variantType := unionName + variant
		payloadType := unionPayloadTypeName(unionName, variant, member)
		if !g.emitJSONParseUnionMemberAttempt(varName, "matched", variantType, payloadType, rawExpr, keyExpr, member, out, indent) {
			g.recordEmitError(fmt.Errorf("unsupported JSON_IETF parser for %s: unsupported union member", codegenNodePath(f.node)))
			return false
		}
	}
	fmt.Fprintf(out, "%sif !matched { return fmt.Errorf(\"%%s does not match any generated union member\", %s) }\n", indent, keyExpr)
	return true
}

func codegenNodePath(node cambium.SchemaNodeRef) string {
	if path := node.Path(); path != "" {
		return path
	}
	return "<unknown>"
}

func resolvedUnionType(info cambium.TypeInfo) (cambium.ResolvedUnion, bool) {
	switch r := info.Resolved().(type) {
	case cambium.ResolvedUnion:
		return r, true
	case cambium.ResolvedLeafRef:
		if realtype, ok := r.Realtype(); ok {
			return resolvedUnionType(*realtype)
		}
	}
	return cambium.ResolvedUnion{}, false
}

func (g *goEmitter) emitJSONParseUnionMemberAttempt(targetVar, matchedVar, variantType, payloadType, rawExpr, keyExpr string, member cambium.TypeInfo, out *strings.Builder, indent string) bool {
	switch r := member.Resolved().(type) {
	case cambium.ResolvedLeafRef:
		if realtype, ok := r.Realtype(); ok {
			return g.emitJSONParseUnionMemberAttempt(targetVar, matchedVar, variantType, payloadType, rawExpr, keyExpr, *realtype, out, indent)
		}
		g.emitUnionStringAttempt(targetVar, matchedVar, variantType, rawExpr, keyExpr, false, member, out, indent)
		return true
	case cambium.ResolvedString:
		patterns := stringPatternLiteral(member)
		if patterns != "nil" {
			g.helpers["patternValidate"] = true
		}
		if len(r.Length) > 0 {
			fmt.Fprintf(out, "%sif !%s {\n", indent, matchedVar)
			fmt.Fprintf(out, "%s\ts, err := cambiumJSONString(%s, %s)\n", indent, rawExpr, keyExpr)
			if patterns != "nil" {
				fmt.Fprintf(out, "%s\tif err == nil { if candidate, err := New%s(s); err == nil { if err := cambiumValidateStringPatterns(s, %s); err == nil { %s = %s; %s = true } } }\n", indent, payloadType, patterns, targetVar, unionVariantAssignExpr(variantType, member), matchedVar)
			} else {
				fmt.Fprintf(out, "%s\tif err == nil { if candidate, err := New%s(s); err == nil { %s = %s; %s = true } }\n", indent, payloadType, targetVar, unionVariantAssignExpr(variantType, member), matchedVar)
			}
			out.WriteString(indent + "}\n")
			return true
		}
		if patterns != "nil" {
			fmt.Fprintf(out, "%sif !%s {\n", indent, matchedVar)
			fmt.Fprintf(out, "%s\ts, err := cambiumJSONString(%s, %s)\n", indent, rawExpr, keyExpr)
			fmt.Fprintf(out, "%s\tif err == nil { if err := cambiumValidateStringPatterns(s, %s); err == nil { candidate := s; %s = %s; %s = true } }\n", indent, patterns, targetVar, unionVariantAssignExpr(variantType, member), matchedVar)
			out.WriteString(indent + "}\n")
			return true
		}
		g.emitUnionStringAttempt(targetVar, matchedVar, variantType, rawExpr, keyExpr, false, member, out, indent)
		return true
	case cambium.ResolvedUnknown:
		g.emitUnionStringAttempt(targetVar, matchedVar, variantType, rawExpr, keyExpr, false, member, out, indent)
		return true
	case cambium.ResolvedBinary:
		g.helpers["binaryParse"] = true
		fmt.Fprintf(out, "%sif !%s {\n", indent, matchedVar)
		fmt.Fprintf(out, "%s\tcandidate, err := cambiumJSONBinary(%s, %s, %s)\n", indent, rawExpr, keyExpr, binaryLengthRangeLiteral(member))
		fmt.Fprintf(out, "%s\tif err == nil { %s = %s; %s = true }\n", indent, targetVar, unionVariantAssignExpr(variantType, member), matchedVar)
		out.WriteString(indent + "}\n")
		return true
	case cambium.ResolvedBoolean:
		fmt.Fprintf(out, "%sif !%s {\n", indent, matchedVar)
		fmt.Fprintf(out, "%s\tcandidate, err := cambiumJSONBool(%s, %s)\n", indent, rawExpr, keyExpr)
		fmt.Fprintf(out, "%s\tif err == nil { %s = %s; %s = true }\n", indent, targetVar, unionVariantAssignExpr(variantType, member), matchedVar)
		out.WriteString(indent + "}\n")
		return true
	case cambium.ResolvedEmpty:
		fmt.Fprintf(out, "%sif !%s {\n", indent, matchedVar)
		fmt.Fprintf(out, "%s\terr := cambiumJSONEmpty(%s, %s)\n", indent, rawExpr, keyExpr)
		fmt.Fprintf(out, "%s\tif err == nil { candidate := struct{}{}; %s = %s; %s = true }\n", indent, targetVar, unionVariantAssignExpr(variantType, member), matchedVar)
		out.WriteString(indent + "}\n")
		return true
	case cambium.ResolvedInt:
		fmt.Fprintf(out, "%sif !%s {\n", indent, matchedVar)
		quoted := jsonIntegerKindQuoted(r.Kind)
		if isSignedIntKind(r.Kind) {
			fmt.Fprintf(out, "%s\tparsed, err := cambiumJSONInt(%s, %s, %d, %t)\n", indent, rawExpr, keyExpr, intKindBitSize(r.Kind), quoted)
			if len(r.Range) > 0 {
				fmt.Fprintf(out, "%s\tif err == nil { if candidate, err := New%s(%s(parsed)); err == nil { %s = %s; %s = true } }\n", indent, payloadType, goTypeForIntKind(r.Kind), targetVar, unionVariantAssignExpr(variantType, member), matchedVar)
			} else {
				fmt.Fprintf(out, "%s\tif err == nil { candidate := %s(parsed); %s = %s; %s = true }\n", indent, goTypeForIntKind(r.Kind), targetVar, unionVariantAssignExpr(variantType, member), matchedVar)
			}
		} else {
			fmt.Fprintf(out, "%s\tparsed, err := cambiumJSONUint(%s, %s, %d, %t)\n", indent, rawExpr, keyExpr, intKindBitSize(r.Kind), quoted)
			if len(r.Range) > 0 {
				fmt.Fprintf(out, "%s\tif err == nil { if candidate, err := New%s(%s(parsed)); err == nil { %s = %s; %s = true } }\n", indent, payloadType, goTypeForIntKind(r.Kind), targetVar, unionVariantAssignExpr(variantType, member), matchedVar)
			} else {
				fmt.Fprintf(out, "%s\tif err == nil { candidate := %s(parsed); %s = %s; %s = true }\n", indent, goTypeForIntKind(r.Kind), targetVar, unionVariantAssignExpr(variantType, member), matchedVar)
			}
		}
		out.WriteString(indent + "}\n")
		return true
	case cambium.ResolvedDecimal64:
		fd, ranges := decimal64ValidationArgs(member)
		fmt.Fprintf(out, "%sif !%s {\n", indent, matchedVar)
		fmt.Fprintf(out, "%s\tcandidate, err := cambiumJSONDecimal64(%s, %s, %d)\n", indent, rawExpr, keyExpr, fd)
		fmt.Fprintf(out, "%s\tif err == nil { if err := cambiumValidateDecimal64Value(candidate, %d, %s); err == nil { %s = %s; %s = true } }\n", indent, fd, ranges, targetVar, unionVariantAssignExpr(variantType, member), matchedVar)
		out.WriteString(indent + "}\n")
		return true
	case cambium.ResolvedEnumeration:
		fmt.Fprintf(out, "%sif !%s {\n", indent, matchedVar)
		fmt.Fprintf(out, "%s\ts, err := cambiumJSONString(%s, %s)\n", indent, rawExpr, keyExpr)
		fmt.Fprintf(out, "%s\tif err == nil { if candidate, ok := Parse%s(s); ok { %s = %s; %s = true } }\n", indent, payloadType, targetVar, unionVariantAssignExpr(variantType, member), matchedVar)
		out.WriteString(indent + "}\n")
		return true
	case cambium.ResolvedBits:
		fmt.Fprintf(out, "%sif !%s {\n", indent, matchedVar)
		fmt.Fprintf(out, "%s\ts, err := cambiumJSONString(%s, %s)\n", indent, rawExpr, keyExpr)
		fmt.Fprintf(out, "%s\tif err == nil { if bitNames, err := cambiumBitsNames(s); err == nil { if candidate, err := New%s(bitNames); err == nil { %s = %s; %s = true } } }\n", indent, payloadType, targetVar, unionVariantAssignExpr(variantType, member), matchedVar)
		out.WriteString(indent + "}\n")
		return true
	case cambium.ResolvedIdentityRef:
		fmt.Fprintf(out, "%sif !%s {\n", indent, matchedVar)
		fmt.Fprintf(out, "%s\ts, err := cambiumJSONString(%s, %s)\n", indent, rawExpr, keyExpr)
		fmt.Fprintf(out, "%s\tif err == nil { if candidate, ok := Parse%s(s); ok { %s = %s; %s = true } }\n", indent, payloadType, targetVar, unionVariantAssignExpr(variantType, member), matchedVar)
		out.WriteString(indent + "}\n")
		return true
	case cambium.ResolvedInstanceIdentifier:
		g.emitUnionStringAttempt(targetVar, matchedVar, variantType, rawExpr, keyExpr, true, member, out, indent)
		return true
	case cambium.ResolvedUnion:
		fmt.Fprintf(out, "%sif !%s {\n", indent, matchedVar)
		fmt.Fprintf(out, "%s\tvar nestedValue %s\n", indent, payloadType)
		out.WriteString(indent + "\tnestedMatched := false\n")
		used := make(map[string]bool)
		for _, nested := range r.Members() {
			variant := safeVariantIdent(unionMemberLabel(nested), used)
			nestedVariantType := payloadType + variant
			nestedPayloadType := unionPayloadTypeName(payloadType, variant, nested)
			g.emitJSONParseUnionMemberAttempt("nestedValue", "nestedMatched", nestedVariantType, nestedPayloadType, rawExpr, keyExpr, nested, out, indent+"\t")
		}
		fmt.Fprintf(out, "%s\tif nestedMatched { candidate := nestedValue; %s = %s; %s = true }\n", indent, targetVar, unionVariantAssignExpr(variantType, member), matchedVar)
		out.WriteString(indent + "}\n")
		return true
	default:
		return false
	}
}

func (g *goEmitter) emitUnionStringAttempt(targetVar, matchedVar, variantType, rawExpr, keyExpr string, instanceIdentifier bool, member cambium.TypeInfo, out *strings.Builder, indent string) {
	const candidate = "candidate"
	fmt.Fprintf(out, "%sif !%s {\n", indent, matchedVar)
	fmt.Fprintf(out, "%s\ts, err := cambiumJSONString(%s, %s)\n", indent, rawExpr, keyExpr)
	if instanceIdentifier {
		fmt.Fprintf(out, "%s\tif err == nil { %s := NewInstanceIdentifier(s); %s = %s; %s = true }\n", indent, candidate, targetVar, unionVariantAssignExpr(variantType, member), matchedVar)
	} else {
		fmt.Fprintf(out, "%s\tif err == nil { %s := s; %s = %s; %s = true }\n", indent, candidate, targetVar, unionVariantAssignExpr(variantType, member), matchedVar)
	}
	out.WriteString(indent + "}\n")
}

func unionPayloadTypeName(unionName, variant string, member cambium.TypeInfo) string {
	switch r := member.Resolved().(type) {
	case cambium.ResolvedLeafRef:
		if realtype, ok := r.Realtype(); ok {
			return unionPayloadTypeName(unionName, variant, *realtype)
		}
		return "string"
	case cambium.ResolvedBoolean:
		return "bool"
	case cambium.ResolvedInt:
		if len(r.Range) > 0 {
			return unionIntRangePayloadTypeName(unionName, variant)
		}
		return goTypeForIntKind(r.Kind)
	case cambium.ResolvedDecimal64:
		return "Decimal64"
	case cambium.ResolvedString:
		if len(r.Length) > 0 {
			return unionStringLengthPayloadTypeName(unionName, variant)
		}
		return "string"
	case cambium.ResolvedBinary, cambium.ResolvedUnknown:
		return "string"
	case cambium.ResolvedEmpty:
		return "struct{}"
	case cambium.ResolvedEnumeration:
		return unionName + variant + "Enum"
	case cambium.ResolvedBits:
		return unionName + variant + "Bits"
	case cambium.ResolvedIdentityRef:
		return unionName + variant + "Enum"
	case cambium.ResolvedInstanceIdentifier:
		return "InstanceIdentifier"
	case cambium.ResolvedUnion:
		return unionName + variant + "Union"
	default:
		return "string"
	}
}

func unionIntRangePayloadTypeName(unionName, variant string) string {
	return unionName + variant + "Range"
}

func unionStringLengthPayloadTypeName(unionName, variant string) string {
	return unionName + variant + "Length"
}

func unionVariantAssignExpr(variantType string, member cambium.TypeInfo) string {
	const candidate = "candidate"
	if unionMemberNeedsWrapper(member) {
		return fmt.Sprintf("%s{Value: %s}", variantType, candidate)
	}
	return fmt.Sprintf("%s(%s)", variantType, candidate)
}

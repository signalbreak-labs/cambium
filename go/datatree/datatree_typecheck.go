package datatree

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/internal/xsdregex"
)

// intImplicitBounds is the inclusive range each integer base type permits before
// any explicit range restriction, as decimal text (uint64's max exceeds int64,
// so bounds are compared with math/big).
var intImplicitBounds = map[cambium.IntKind][2]string{
	cambium.IntKindI8:  {"-128", "127"},
	cambium.IntKindI16: {"-32768", "32767"},
	cambium.IntKindI32: {"-2147483648", "2147483647"},
	cambium.IntKindI64: {"-9223372036854775808", "9223372036854775807"},
	cambium.IntKindU8:  {"0", "255"},
	cambium.IntKindU16: {"0", "65535"},
	cambium.IntKindU32: {"0", "4294967295"},
	cambium.IntKindU64: {"0", "18446744073709551615"},
}

// validateLeafValue checks one leaf value (the raw JSON token) against its
// resolved YANG type and appends any violations. It covers string length and
// patterns, integer ranges (including the base-type width), decimal64
// fraction-digits and ranges, boolean and empty shapes, enumeration and bits
// membership, binary base64 + length, unions (first matching member wins), and
// leafrefs (delegated to the referenced type). Leafref instance existence,
// identityref base derivation, and instance-identifier resolution need
// data/identity context and are later slices; those get a shape check only.
func validateLeafValue(ti cambium.TypeInfo, raw json.RawMessage, path string, out *[]string) {
	switch r := ti.Resolved().(type) {
	case cambium.ResolvedString:
		s, ok := jsonStringValue(raw)
		if !ok {
			*out = append(*out, fmt.Sprintf("%s: expected a JSON string", path))
			return
		}
		checkLengthRanges(int64(utf8.RuneCountInString(s)), r.Length, path, out)
		checkPatterns(s, r.Patterns, path, out)
	case cambium.ResolvedInt:
		text, ok := numericText(raw)
		if !ok {
			*out = append(*out, fmt.Sprintf("%s: expected an integer", path))
			return
		}
		v, ok := new(big.Int).SetString(text, 10)
		if !ok {
			*out = append(*out, fmt.Sprintf("%s: %q is not a valid integer", path, text))
			return
		}
		checkIntImplicit(v, r.Kind, path, out)
		if len(r.Range) > 0 && !intInRanges(v, r.Range) {
			*out = append(*out, fmt.Sprintf("%s: %s is outside the permitted range", path, v.String()))
		}
	case cambium.ResolvedDecimal64:
		s, ok := jsonStringValue(raw)
		if !ok {
			*out = append(*out, fmt.Sprintf("%s: decimal64 must be a JSON string", path))
			return
		}
		checkDecimal(s, r, path, out)
	case cambium.ResolvedBoolean:
		if !isJSONBool(raw) {
			*out = append(*out, fmt.Sprintf("%s: expected true or false", path))
		}
	case cambium.ResolvedEmpty:
		if strings.TrimSpace(string(raw)) != "[null]" {
			*out = append(*out, fmt.Sprintf("%s: empty leaf must be encoded as [null]", path))
		}
	case cambium.ResolvedEnumeration:
		s, ok := jsonStringValue(raw)
		if !ok || !enumHasName(r.Values(), s) {
			*out = append(*out, fmt.Sprintf("%s: %s is not a defined enum value", path, rawDisplay(raw)))
		}
	case cambium.ResolvedBits:
		checkBits(raw, r.Values(), path, out)
	case cambium.ResolvedBinary:
		s, ok := jsonStringValue(raw)
		if !ok {
			*out = append(*out, fmt.Sprintf("%s: binary must be a base64 JSON string", path))
			return
		}
		decoded, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			*out = append(*out, fmt.Sprintf("%s: invalid base64: %v", path, err))
			return
		}
		checkLengthRanges(int64(len(decoded)), r.Length, path, out)
	case cambium.ResolvedUnion:
		members := r.Members()
		if len(members) == 0 {
			return
		}
		for _, m := range members {
			var trial []string
			validateLeafValue(m, raw, path, &trial)
			if len(trial) == 0 {
				return // first matching member type wins
			}
		}
		*out = append(*out, fmt.Sprintf("%s: %s matches no union member type", path, rawDisplay(raw)))
	case cambium.ResolvedLeafRef:
		if rt, ok := r.Realtype(); ok && rt != nil {
			validateLeafValue(*rt, raw, path, out)
		}
		// instance existence is a later slice
	case cambium.ResolvedIdentityRef:
		s, ok := jsonStringValue(raw)
		if !ok {
			*out = append(*out, fmt.Sprintf("%s: expected a JSON string", path))
			return
		}
		validateIdentityRef(r, s, path, out)
	case cambium.ResolvedInstanceIdentifier:
		// instance-identifier resolution needs data/XPath context (deferred);
		// verify the JSON shape only.
		if _, ok := jsonStringValue(raw); !ok {
			*out = append(*out, fmt.Sprintf("%s: expected a JSON string", path))
		}
	default:
		// ResolvedUnknown or a future type: do not guess, do not false-flag.
	}
}

func jsonStringValue(raw json.RawMessage) (string, bool) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, true
}

func isJSONBool(raw json.RawMessage) bool {
	var b bool
	return json.Unmarshal(raw, &b) == nil
}

// numericText returns the integer text whether it was JSON-encoded as a number
// (int8..32 / uint8..32) or as a string (int64 / uint64, per RFC 7951 §6.1).
func numericText(raw json.RawMessage) (string, bool) {
	if s, ok := jsonStringValue(raw); ok {
		return strings.TrimSpace(s), true
	}
	t := strings.TrimSpace(string(raw))
	if t == "" {
		return "", false
	}
	return t, true
}

func rawDisplay(raw json.RawMessage) string {
	if s, ok := jsonStringValue(raw); ok {
		return strconv.Quote(s)
	}
	return string(raw)
}

// validateIdentityRef checks that value names an identity that is one of the
// required bases or transitively derived from one. A "module:name" value is
// matched against qualified names, a bare value against same-module bare names.
func validateIdentityRef(r cambium.ResolvedIdentityRef, value, path string, out *[]string) {
	qualified := make(map[string]bool)
	bare := make(map[string]bool)
	seen := make(map[string]bool)
	for _, base := range r.Bases() {
		collectIdentity(base, qualified, bare, seen)
	}
	valid := bare[value]
	if strings.Contains(value, ":") {
		valid = qualified[value]
	}
	if !valid {
		*out = append(*out, fmt.Sprintf("%s: %q is not an identity derived from the required base identity", path, value))
	}
}

func collectIdentity(id cambium.Identity, qualified, bare, seen map[string]bool) {
	qn := id.Module().Name() + ":" + id.Name()
	if seen[qn] {
		return
	}
	seen[qn] = true
	qualified[qn] = true
	bare[id.Name()] = true
	for _, derived := range id.Derived() {
		collectIdentity(derived, qualified, bare, seen)
	}
}

func enumHasName(values []cambium.EnumValue, name string) bool {
	for _, v := range values {
		if v.Name() == name {
			return true
		}
	}
	return false
}

func checkIntImplicit(v *big.Int, kind cambium.IntKind, path string, out *[]string) {
	bounds, ok := intImplicitBounds[kind]
	if !ok {
		return
	}
	lo, _ := new(big.Int).SetString(bounds[0], 10)
	hi, _ := new(big.Int).SetString(bounds[1], 10)
	if v.Cmp(lo) < 0 || v.Cmp(hi) > 0 {
		*out = append(*out, fmt.Sprintf("%s: %s is outside the base type range [%s, %s]", path, v.String(), bounds[0], bounds[1]))
	}
}

// The "min" and "max" range keywords mean "no bound on this side" — the schema
// leaves them as literal text for length and decimal64 constraints (only the
// integer path resolves them to numbers), so every comparator below treats them
// as unbounded rather than parsing them.
func intInRanges(v *big.Int, ranges []cambium.RangeBound) bool {
	for _, r := range ranges {
		if intInRange(v, r) {
			return true
		}
	}
	return false
}

func intInRange(v *big.Int, r cambium.RangeBound) bool {
	if lo := strings.TrimSpace(r.Min()); lo != "min" {
		loV, ok := new(big.Int).SetString(lo, 10)
		if !ok || v.Cmp(loV) < 0 {
			return false
		}
	}
	if hi := strings.TrimSpace(r.Max()); hi != "max" {
		hiV, ok := new(big.Int).SetString(hi, 10)
		if !ok || v.Cmp(hiV) > 0 {
			return false
		}
	}
	return true
}

func ratInRanges(v *big.Rat, ranges []cambium.RangeBound) bool {
	for _, r := range ranges {
		if ratInRange(v, r) {
			return true
		}
	}
	return false
}

func ratInRange(v *big.Rat, r cambium.RangeBound) bool {
	if lo := strings.TrimSpace(r.Min()); lo != "min" {
		loV, ok := new(big.Rat).SetString(lo)
		if !ok || v.Cmp(loV) < 0 {
			return false
		}
	}
	if hi := strings.TrimSpace(r.Max()); hi != "max" {
		hiV, ok := new(big.Rat).SetString(hi)
		if !ok || v.Cmp(hiV) > 0 {
			return false
		}
	}
	return true
}

func checkLengthRanges(n int64, ranges []cambium.RangeBound, path string, out *[]string) {
	if len(ranges) == 0 {
		return
	}
	for _, r := range ranges {
		if lengthInRange(n, r) {
			return
		}
	}
	*out = append(*out, fmt.Sprintf("%s: length %d is outside the permitted length", path, n))
}

func lengthInRange(n int64, r cambium.RangeBound) bool {
	if lo := strings.TrimSpace(r.Min()); lo != "min" {
		loV, err := strconv.ParseInt(lo, 10, 64)
		if err != nil || n < loV {
			return false
		}
	}
	if hi := strings.TrimSpace(r.Max()); hi != "max" {
		hiV, err := strconv.ParseInt(hi, 10, 64)
		if err != nil || n > hiV {
			return false
		}
	}
	return true
}

func checkPatterns(s string, patterns []cambium.Pattern, path string, out *[]string) {
	for _, p := range patterns {
		re, err := regexp.Compile("^(?:" + xsdregex.NativePattern(p.Regex()) + ")$")
		if err != nil {
			// Pattern compilability was already checked at schema build; skip
			// rather than emit a spurious data violation.
			continue
		}
		matched := re.MatchString(s)
		if p.IsInverted() {
			if matched {
				*out = append(*out, fmt.Sprintf("%s: value matches inverted pattern %q", path, p.Regex()))
			}
		} else if !matched {
			*out = append(*out, fmt.Sprintf("%s: value does not match pattern %q", path, p.Regex()))
		}
	}
}

func checkBits(raw json.RawMessage, values []cambium.EnumValue, path string, out *[]string) {
	s, ok := jsonStringValue(raw)
	if !ok {
		*out = append(*out, fmt.Sprintf("%s: bits must be a space-separated JSON string", path))
		return
	}
	defined := make(map[string]bool, len(values))
	for _, v := range values {
		defined[v.Name()] = true
	}
	for _, bit := range strings.Fields(s) {
		if !defined[bit] {
			*out = append(*out, fmt.Sprintf("%s: %q is not a defined bit", path, bit))
		}
	}
}

// decimal64Lexical matches the RFC 7950 decimal64 lexical form (optional sign,
// integer digits, optional fractional part). It rejects forms big.Rat would
// otherwise accept but YANG does not: hex (0x10), exponent (1.5e1), rational
// (1/2), and underscore-separated (1_000) literals.
var decimal64Lexical = regexp.MustCompile(`^[+-]?[0-9]+(\.[0-9]+)?$`)

func checkDecimal(s string, r cambium.ResolvedDecimal64, path string, out *[]string) {
	s = strings.TrimSpace(s)
	if !decimal64Lexical.MatchString(s) {
		*out = append(*out, fmt.Sprintf("%s: %q is not a valid decimal64", path, s))
		return
	}
	val, ok := new(big.Rat).SetString(s)
	if !ok {
		*out = append(*out, fmt.Sprintf("%s: %q is not a valid decimal64", path, s))
		return
	}
	if dot := strings.IndexByte(s, '.'); dot >= 0 {
		frac := len(s) - dot - 1
		if max := int(r.FractionDigits().Value()); frac > max {
			*out = append(*out, fmt.Sprintf("%s: %s has %d fraction digits, more than fraction-digits %d", path, s, frac, max))
		}
	}
	if len(r.Range) > 0 && !ratInRanges(val, r.Range) {
		*out = append(*out, fmt.Sprintf("%s: %s is outside the permitted range", path, s))
	}
}

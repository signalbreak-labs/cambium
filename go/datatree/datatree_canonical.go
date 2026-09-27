// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package datatree

import (
	"cmp"
	"encoding/json"
	"math/big"
	"slices"
	"strconv"
	"strings"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// maxNumericLexicalLen bounds the length of an integer or decimal64 lexical
// value before it is parsed. Every valid YANG integer and decimal64 value fits
// in a few dozen characters; anything longer is rejected as invalid without
// being handed to math/big, whose decimal conversion is superlinear in the
// digit count (a multi-megabyte digit string would otherwise cost seconds).
const maxNumericLexicalLen = 256

// canonicalLeafToken returns the canonical token for a value of the leaf or
// leaf-list sn (see canonicalToken).
func canonicalLeafToken(sn cambium.SchemaNodeRef, raw json.RawMessage) json.RawMessage {
	ti, ok := sn.LeafType()
	if !ok {
		return raw
	}
	return canonicalToken(ti, raw, sn.Module())
}

// canonicalToken returns the canonical form of a leaf value token of type ti —
// the RFC 7950 §9 canonical lexical form in its RFC 7951 encoding, which is
// what libyang stores and prints — or raw unchanged when the value is not
// valid for the type, so Validate still reports it as written (an
// instance-identifier that does not resolve against the schema is also kept
// as written). leafModule is the leaf's module: an identityref naming one of
// its identities is written without a module qualifier (RFC 7951 §6.8).
func canonicalToken(ti cambium.TypeInfo, raw json.RawMessage, leafModule cambium.Module) json.RawMessage {
	switch ti.Base() {
	case cambium.BaseTypeString, cambium.BaseTypeBoolean, cambium.BaseTypeEmpty,
		cambium.BaseTypeEnumeration, cambium.BaseTypeBinary:
		return raw // already canonical as parsed; skip resolving the type
	}
	switch r := ti.Resolved().(type) {
	case cambium.ResolvedInt:
		text, ok := integerText(raw, r.Kind)
		if !ok || len(text) > maxNumericLexicalLen {
			return raw
		}
		v, ok := new(big.Int).SetString(text, 10)
		if !ok {
			return raw
		}
		if integerJSONQuoted(r.Kind) {
			return jsonStringToken(v.String())
		}
		return json.RawMessage(v.String())
	case cambium.ResolvedDecimal64:
		s, ok := jsonStringValue(raw)
		if !ok {
			return raw
		}
		fd := int(r.FractionDigits().Value())
		if num, ok := parseDecimal64(s, fd); ok {
			return jsonStringToken(formatDecimal64(num, fd))
		}
	case cambium.ResolvedBits:
		if s, ok := canonicalBits(raw, r.Values()); ok {
			return jsonStringToken(s)
		}
	case cambium.ResolvedIdentityRef:
		s, ok := jsonStringValue(raw)
		if !ok {
			return raw
		}
		if mod, name, qualified := strings.Cut(s, ":"); qualified && mod == leafModule.Name() {
			var bad []string
			validateIdentityRef(r, s, "", leafModule.Name(), &bad)
			if len(bad) == 0 {
				return jsonStringToken(name)
			}
		}
	case cambium.ResolvedInstanceIdentifier:
		if s, ok := jsonStringValue(raw); ok {
			if path, ok := canonicalInstanceIdentifier(s, leafModule); ok {
				return jsonStringToken(path)
			}
		}
	case cambium.ResolvedLeafRef:
		if rt, ok := r.Realtype(); ok && rt != nil {
			return canonicalToken(*rt, raw, leafModule)
		}
	case cambium.ResolvedUnion:
		for _, member := range r.Members() {
			var trial []string
			validateLeafValue(member, raw, "", leafModule.Name(), &trial)
			if len(trial) == 0 {
				return canonicalToken(member, raw, leafModule) // first matching member wins
			}
		}
	}
	return raw
}

// valueKey returns the comparison key of a canonical value token of type ti,
// so two values compare equal exactly when they are the same value, as
// libyang compares them: the token itself, except that an identityref names
// its identity with the module it belongs to — the JSON_IETF form leaves the
// leaf's own module implicit, so the same identity reads "one" in a leaf of
// its module and "m:one" in a leaf of another. leafModule is the name of the
// module of the leaf holding the value.
func valueKey(ti cambium.TypeInfo, raw json.RawMessage, leafModule string) string {
	if !valueKeyQualifies(ti) {
		return string(raw)
	}
	switch r := ti.Resolved().(type) {
	case cambium.ResolvedIdentityRef:
		if s, ok := jsonStringValue(raw); ok && !strings.Contains(s, ":") {
			return string(jsonStringToken(leafModule + ":" + s))
		}
	case cambium.ResolvedLeafRef:
		if rt, ok := r.Realtype(); ok && rt != nil {
			return valueKey(*rt, raw, leafModule)
		}
	case cambium.ResolvedUnion:
		for _, member := range r.Members() {
			var trial []string
			validateLeafValue(member, raw, "", leafModule, &trial)
			if len(trial) == 0 {
				return valueKey(member, raw, leafModule) // first matching member wins
			}
		}
	}
	return string(raw)
}

// valueKeyQualifies reports whether a value of type ti may name an identity,
// so its comparison key can differ from its token.
func valueKeyQualifies(ti cambium.TypeInfo) bool {
	switch ti.Base() {
	case cambium.BaseTypeIdentityRef, cambium.BaseTypeLeafRef, cambium.BaseTypeUnion:
		return true
	default:
		return false
	}
}

// leafValueKey returns the comparison key of a value of the leaf or leaf-list
// sn (see valueKey).
func leafValueKey(sn cambium.SchemaNodeRef, raw json.RawMessage) string {
	ti, ok := sn.LeafType()
	if !ok {
		return string(raw)
	}
	return valueKey(ti, raw, sn.Module().Name())
}

// parseDecimal64 returns s as a decimal64 scaled by 10^fractionDigits (the
// integer libyang stores), rejecting non-lexical input, more fraction digits
// than the type allows, and values outside the int64 range.
func parseDecimal64(s string, fractionDigits int) (int64, bool) {
	if len(s) > maxNumericLexicalLen || !decimal64Lexical.MatchString(s) {
		return 0, false
	}
	neg := false
	switch s[0] {
	case '-':
		neg, s = true, s[1:]
	case '+':
		s = s[1:]
	}
	intPart, frac, _ := strings.Cut(s, ".")
	if len(frac) > fractionDigits {
		return 0, false
	}
	digits := strings.TrimLeft(intPart+frac+strings.Repeat("0", fractionDigits-len(frac)), "0")
	if digits == "" {
		return 0, true
	}
	if neg {
		digits = "-" + digits
	}
	v, err := strconv.ParseInt(digits, 10, 64)
	return v, err == nil
}

// formatDecimal64 renders a scaled decimal64 in canonical form (RFC 7950
// §9.3.2), exactly as libyang does: no leading zeros before the one integer
// digit of a value below one, trailing fraction zeros dropped but at least one
// fraction digit kept, no plus sign, and zero as "0.0".
func formatDecimal64(num int64, fractionDigits int) string {
	if num == 0 {
		return "0.0"
	}
	digits := strconv.FormatInt(num, 10)
	sign := ""
	if num < 0 {
		sign, digits = "-", digits[1:]
	}
	if len(digits) <= fractionDigits {
		digits = strings.Repeat("0", fractionDigits-len(digits)+1) + digits
	}
	cut := len(digits) - fractionDigits
	frac := strings.TrimRight(digits[cut:], "0")
	if frac == "" {
		frac = "0"
	}
	return sign + digits[:cut] + "." + frac
}

// canonicalBits returns a bits value with its bit names in ascending position
// order separated by single spaces (RFC 7950 §9.7.2), or false when it names
// an undefined or duplicate bit.
func canonicalBits(raw json.RawMessage, values []cambium.EnumValue) (string, bool) {
	s, ok := jsonStringValue(raw)
	if !ok {
		return "", false
	}
	type bit struct {
		name string
		pos  uint64
	}
	names := strings.Fields(s)
	bits := make([]bit, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		pos, ok := bitPosition(values, name)
		if !ok || seen[name] {
			return "", false
		}
		seen[name] = true
		bits = append(bits, bit{name: name, pos: pos})
	}
	slices.SortFunc(bits, func(a, b bit) int { return cmp.Compare(a.pos, b.pos) })
	out := make([]string, len(bits))
	for i, b := range bits {
		out[i] = b.name
	}
	return strings.Join(out, " "), true
}

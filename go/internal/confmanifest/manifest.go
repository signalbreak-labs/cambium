// Package confmanifest parses the shared /conformance manifest.toml.
//
// It lives in an internal package so both the pure-Go schema floor
// (go/cambium) and the backend-only conformance runner (go/conformance) can
// load the manifest without pulling cgo/libyang into the default import
// closure.
package confmanifest

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Tier separates schema-IR conformance cases from backend/data cases.
type Tier string

const (
	TierBackendData Tier = "backend-data"
	TierSchemaIR    Tier = "schema-ir"
)

// Case is one entry in conformance/manifest.toml.
type Case struct {
	Name              string
	Tier              Tier
	Module            string
	Input             string
	InputFormat       string
	OpType            string
	SerializeDefaults string
	Oracle            bool
	ExpectedIR        string
	Expected          map[string]string // format -> golden path
	// Invariants are the ordering invariant ids (e.g. "I2", "I3") this case
	// exercises, per spec/ordering-invariants.md §6. Optional; lets CI assert
	// every invariant has a passing fixture (see manifest_test.go).
	Invariants []string
}

// EffectiveTier returns the explicit tier, defaulting empty values to the
// backend/data tier so existing manifest cases keep working.
func (c Case) EffectiveTier() Tier {
	if c.Tier == "" {
		return TierBackendData
	}
	return c.Tier
}

// Load parses the subset of TOML used by conformance/manifest.toml:
//
//   - top-level [[case]] arrays
//   - simple key = "value" pairs
//   - [case.expected] inline tables
//   - boolean literals
//
// It is intentionally small and dependency-free.
func Load(path string) ([]Case, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var cases []Case
	inExpected := false

	scan := bufio.NewScanner(f)
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		switch {
		case line == "[[case]]":
			cases = append(cases, Case{Expected: map[string]string{}})
			inExpected = false
			continue
		case line == "[case.expected]":
			inExpected = true
			continue
		case strings.HasPrefix(line, "["):
			// Any other table header leaves the expected section.
			inExpected = false
			continue
		}

		key, val, ok := splitKeyValue(line)
		if !ok || len(cases) == 0 {
			continue
		}

		cur := &cases[len(cases)-1]
		if inExpected {
			cur.Expected[key] = unquote(val)
			continue
		}

		switch key {
		case "name":
			cur.Name = unquote(val)
		case "tier":
			cur.Tier = Tier(unquote(val))
		case "module":
			cur.Module = unquote(val)
		case "input":
			cur.Input = unquote(val)
		case "input-format":
			cur.InputFormat = unquote(val)
		case "op-type":
			cur.OpType = unquote(val)
		case "serialize-defaults":
			cur.SerializeDefaults = unquote(val)
		case "oracle":
			b, err := strconv.ParseBool(val)
			if err != nil {
				return nil, fmt.Errorf("case %q: invalid oracle %q: %w", cur.Name, val, err)
			}
			cur.Oracle = b
		case "expected-ir":
			cur.ExpectedIR = unquote(val)
		case "invariants":
			cur.Invariants = parseStringArray(val)
		}
	}

	if err := scan.Err(); err != nil {
		return nil, err
	}

	for _, c := range cases {
		if err := c.Tier.validate(); err != nil {
			return nil, fmt.Errorf("case %q: invalid tier %q", c.Name, c.Tier)
		}
	}

	return cases, nil
}

func (t Tier) validate() error {
	switch t {
	case "", TierBackendData, TierSchemaIR:
		return nil
	default:
		return fmt.Errorf("invalid tier %q", t)
	}
}

func splitKeyValue(line string) (string, string, bool) {
	i := strings.IndexByte(line, '=')
	if i < 0 {
		return "", "", false
	}
	return strings.TrimSpace(line[:i]), strings.TrimSpace(line[i+1:]), true
}

func unquote(s string) string { return strings.Trim(s, `"`) }

// parseStringArray parses an inline TOML string array: ["I2", "I3"].
func parseStringArray(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, unquote(part))
	}
	return out
}

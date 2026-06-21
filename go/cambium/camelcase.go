// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium

// CamelCase returns the goyang-compatible exported identifier spelling for a
// YANG identifier.
func CamelCase(s string) string {
	if s == "" {
		return ""
	}

	out := make([]byte, 0, len(s)+1)
	i := 0
	if normalizedIdentifierSeparator(s[0]) == '_' {
		out = append(out, 'X')
		i = 1
	}

	for i < len(s) {
		c := normalizedIdentifierSeparator(s[i])
		if c == '_' && i+1 < len(s) && isASCIILower(s[i+1]) {
			i++
			continue
		}
		if isASCIIDigit(c) {
			out = append(out, c)
			i++
			continue
		}
		if isASCIILower(c) {
			c -= 'a' - 'A'
		}

		start := len(out)
		out = append(out, c)
		i++
		for i < len(s) && isASCIILower(s[i]) {
			out = append(out, s[i])
			i++
		}
		if known := knownCamelCaseWord(string(out[start:])); known != "" {
			out = append(out[:start], known...)
		}
	}
	return string(out)
}

func knownCamelCaseWord(word string) string {
	if word == "Ietf" {
		return "IETF"
	}
	return ""
}

func normalizedIdentifierSeparator(c byte) byte {
	if c == '-' || c == '.' {
		return '_'
	}
	return c
}

func isASCIILower(c byte) bool { return c >= 'a' && c <= 'z' }

func isASCIIDigit(c byte) bool { return c >= '0' && c <= '9' }

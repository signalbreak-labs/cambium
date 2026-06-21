// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/signalbreak-labs/cambium/go/internal/xsdregex"
	"github.com/signalbreak-labs/cambium/go/internal/yangparse"
)

// Pattern is a textual pattern constraint for strings.
type Pattern struct {
	regex                                string
	errorMessage, description, reference string
	appTag                               *string
	inverted                             bool
}

func (p Pattern) Regex() string { return p.regex }
func (m *moduleData) patternModifierRequiresYang11(st *yangparse.Statement) bool {
	if st == nil || st.Keyword != "modifier" || st.Argument != "invert-match" {
		return false
	}
	parent := m.statementParents[st]
	if parent == nil || parent.Keyword != "pattern" {
		return false
	}
	modifiers := direct(parent, "modifier")
	return len(modifiers) == 1 && modifiers[0] == st
}

func stringPatternMatchesDefault(pattern Pattern, value string) bool {
	re, err := regexp.Compile("^(?:" + xsdregex.NativePattern(pattern.Regex()) + ")$")
	if err != nil {
		return false
	}
	matched := re.MatchString(value)
	if pattern.IsInverted() {
		matched = !matched
	}
	return matched
}

func patterns(st *yangparse.Statement) ([]Pattern, error) {
	var out []Pattern
	for _, p := range direct(st, "pattern") {
		if err := validateYANGPatternSyntax(p.Argument); err != nil {
			return nil, fmt.Errorf("invalid pattern %q at %s: %w", p.Argument, p.Location(), err)
		}
		if err := validateNativeStringPattern(p.Argument); err != nil {
			return nil, fmt.Errorf("pattern %q does not compile for native full-string regexp checks at %s: %w", p.Argument, p.Location(), err)
		}
		var tag *string
		errorMessage, err := constraintMetadataArg(p, "error-message")
		if err != nil {
			return nil, err
		}
		if s, err := constraintMetadataArg(p, "error-app-tag"); err != nil {
			return nil, err
		} else if s != "" {
			tag = ptr(s)
		}
		description, err := constraintMetadataArg(p, "description")
		if err != nil {
			return nil, err
		}
		reference, err := constraintMetadataArg(p, "reference")
		if err != nil {
			return nil, err
		}
		inverted := false
		modifiers := direct(p, "modifier")
		if len(modifiers) > 1 {
			return nil, fmt.Errorf("pattern %q has multiple modifier statements at %s", p.Argument, modifiers[1].Location())
		}
		if len(modifiers) == 1 {
			if modifiers[0].Argument != "invert-match" {
				return nil, fmt.Errorf("invalid pattern modifier %q at %s", modifiers[0].Argument, modifiers[0].Location())
			}
			inverted = true
		}
		out = append(out, Pattern{
			regex:        p.Argument,
			errorMessage: errorMessage,
			appTag:       tag,
			description:  description,
			reference:    reference,
			inverted:     inverted,
		})
	}
	return out, nil
}

func validateYANGPatternSyntax(pattern string) error {
	// YANG uses XML Schema regex syntax, not Go regexp syntax. This structural
	// sanity check runs before the native-regexp compatibility gate below so
	// malformed YANG patterns fail with parser-oriented diagnostics.
	runes := []rune(pattern)
	parenDepth := 0
	canRepeat := false
	lastWasQuantifier := false
	for i := 0; i < len(runes); i++ {
		switch r := runes[i]; r {
		case '\\':
			if i == len(runes)-1 {
				return fmt.Errorf("trailing escape")
			}
			end, err := validateYANGPatternEscape(runes, i+1)
			if err != nil {
				return err
			}
			i = end
			canRepeat = true
			lastWasQuantifier = false
		case '[':
			end, err := validateYANGPatternClass(runes, i)
			if err != nil {
				return err
			}
			i = end
			canRepeat = true
			lastWasQuantifier = false
		case '(':
			parenDepth++
			canRepeat = false
			lastWasQuantifier = false
		case ')':
			if parenDepth == 0 {
				return fmt.Errorf("unmatched ')'")
			}
			parenDepth--
			canRepeat = true
			lastWasQuantifier = false
		case '*', '+', '?':
			if !canRepeat || lastWasQuantifier {
				return fmt.Errorf("invalid repeated quantifier")
			}
			canRepeat = false
			lastWasQuantifier = true
		case '{':
			end, ok, err := parseYANGPatternQuantifier(runes, i)
			if err != nil {
				return err
			}
			if !ok {
				canRepeat = true
				lastWasQuantifier = false
				continue
			}
			if !canRepeat || lastWasQuantifier {
				return fmt.Errorf("invalid quantifier")
			}
			i = end
			canRepeat = false
			lastWasQuantifier = true
		case '|':
			canRepeat = false
			lastWasQuantifier = false
		default:
			canRepeat = true
			lastWasQuantifier = false
		}
	}
	if parenDepth != 0 {
		return fmt.Errorf("unterminated group")
	}
	return nil
}

func validateNativeStringPattern(pattern string) error {
	if unsupported := xsdregex.UnsupportedNativeSyntax(pattern); unsupported != "" {
		return errors.New(unsupported)
	}
	_, err := regexp.Compile("^(?:" + xsdregex.NativePattern(pattern) + ")$")
	return err
}

type yangPatternClassFrame struct {
	items    int
	atStart  bool
	prev     rune
	havePrev bool
}

func validateYANGPatternClass(runes []rune, start int) (int, error) {
	frames := []yangPatternClassFrame{{atStart: true}}
	for i := start + 1; i < len(runes); i++ {
		frame := &frames[len(frames)-1]
		switch r := runes[i]; r {
		case '\\':
			if i == len(runes)-1 {
				return 0, fmt.Errorf("trailing escape")
			}
			end, err := validateYANGPatternEscape(runes, i+1)
			if err != nil {
				return 0, err
			}
			i = end
			frame.items++
			frame.atStart = false
			frame.havePrev = false
		case '[':
			frame.items++
			frame.atStart = false
			frame.havePrev = false
			frames = append(frames, yangPatternClassFrame{atStart: true})
		case ']':
			if frame.items == 0 {
				return 0, fmt.Errorf("empty character class")
			}
			frames = frames[:len(frames)-1]
			if len(frames) == 0 {
				return i, nil
			}
			frames[len(frames)-1].havePrev = false
		case '^':
			if frame.atStart {
				frame.atStart = false
				frame.havePrev = false
				continue
			}
			recordYANGPatternClassRune(frame, runes, i)
		default:
			if frame.havePrev && r == '-' && i+1 < len(runes) {
				next := runes[i+1]
				if next != ']' && next != '[' && next != '\\' && frame.prev > next {
					return 0, fmt.Errorf("reversed character range")
				}
			}
			recordYANGPatternClassRune(frame, runes, i)
		}
	}
	return 0, fmt.Errorf("unterminated character class")
}

func validateYANGPatternEscape(runes []rune, start int) (int, error) {
	switch r := runes[start]; r {
	case 'p', 'P':
		if start+1 >= len(runes) || runes[start+1] != '{' {
			return 0, fmt.Errorf("invalid character category escape")
		}
		end := start + 2
		for end < len(runes) && runes[end] != '}' {
			end++
		}
		if end >= len(runes) || end == start+2 {
			return 0, fmt.Errorf("invalid character category escape")
		}
		if !validYANGPatternCategory(string(runes[start+2 : end])) {
			return 0, fmt.Errorf("unknown character category")
		}
		return end, nil
	case 'c', 'C', 'd', 'D', 'i', 'I', 'n', 'r', 's', 'S', 't', 'w', 'W':
		return start, nil
	default:
		if isYANGPatternEscapedChar(r) {
			return start, nil
		}
		return 0, fmt.Errorf("unrecognized escape")
	}
}

func validYANGPatternCategory(category string) bool {
	if _, ok := yangPatternCategories[category]; ok {
		return true
	}
	if !strings.HasPrefix(category, "Is") {
		return false
	}
	_, ok := yangPatternUnicodeBlocks[category[len("Is"):]]
	return ok
}

func isYANGPatternEscapedChar(r rune) bool {
	switch r {
	case '\\', '|', '-', '.', '?', '*', '+', '{', '}', '(', ')', '[', ']', '^', '$':
		return true
	default:
		return false
	}
}

func recordYANGPatternClassRune(frame *yangPatternClassFrame, runes []rune, i int) {
	frame.items++
	frame.atStart = false
	frame.prev = runes[i]
	frame.havePrev = true
}

func parseYANGPatternQuantifier(runes []rune, start int) (int, bool, error) {
	i := start + 1
	if i >= len(runes) || !isYANGPatternDigit(runes[i]) {
		return 0, false, nil
	}
	minStart := i
	for i < len(runes) && isYANGPatternDigit(runes[i]) {
		i++
	}
	min := string(runes[minStart:i])
	if i >= len(runes) {
		return 0, false, nil
	}
	if runes[i] == '}' {
		return i, true, nil
	}
	if runes[i] != ',' {
		return 0, false, nil
	}
	i++
	if i >= len(runes) {
		return 0, false, nil
	}
	if runes[i] == '}' {
		return i, true, nil
	}
	maxStart := i
	if !isYANGPatternDigit(runes[i]) {
		return 0, false, nil
	}
	for i < len(runes) && isYANGPatternDigit(runes[i]) {
		i++
	}
	if i >= len(runes) || runes[i] != '}' {
		return 0, false, nil
	}
	maximum := string(runes[maxStart:i])
	if compareYANGPatternDecimal(min, maximum) > 0 {
		return 0, true, fmt.Errorf("reversed quantifier range")
	}
	return i, true, nil
}

func isYANGPatternDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

func compareYANGPatternDecimal(a, b string) int {
	a = strings.TrimLeft(a, "0")
	b = strings.TrimLeft(b, "0")
	if a == "" {
		a = "0"
	}
	if b == "" {
		b = "0"
	}
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

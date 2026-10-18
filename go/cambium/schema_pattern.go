// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium

import (
	"fmt"
	"regexp"

	"github.com/signalbreak-labs/cambium/go/internal/xsdregex"
	"github.com/signalbreak-labs/cambium/go/internal/yangparse"
	"github.com/signalbreak-labs/cambium/go/yangpattern"
)

// Pattern is a textual pattern constraint for strings.
type Pattern struct {
	regex                                string
	errorMessage, description, reference string
	appTag                               *string
	inverted                             bool
}

// Regex returns the raw YANG/XML Schema regular expression text.
func (p Pattern) Regex() string { return p.regex }

// GoRegexp returns an equivalent, full-string Go regular expression, or an error
// if the pattern cannot be represented by Cambium's native regexp bridge.
// It does not apply invert-match; callers must apply IsInverted separately.
// Translation failure does not make the source pattern invalid YANG.
func (p Pattern) GoRegexp() (string, error) {
	program, err := p.Program()
	if err != nil {
		return "", err
	}
	return program.GoRegexp()
}

// Program compiles the raw XSD pattern to a bounded, portable matcher.
// Like GoRegexp, it leaves invert-match to the caller.
func (p Pattern) Program() (*yangpattern.Program, error) {
	return yangpattern.Compile(p.regex)
}

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
	_, err := yangpattern.Compile(pattern)
	return err
}

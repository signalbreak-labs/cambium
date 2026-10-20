// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium

import (
	"errors"
	"fmt"

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

func stringPatternMatchesDefault(pattern Pattern, value string) (bool, error) {
	program, err := pattern.Program()
	if err != nil {
		return false, err
	}
	matched, err := program.MatchString(value)
	if err != nil {
		return false, err
	}
	if pattern.IsInverted() {
		matched = !matched
	}
	return matched, nil
}

// patterns returns the statement's patterns that Cambium enforces and, for a
// ValidationVendorCompatible context, the ones it cannot compile. Those stay
// unenforced: the device that owns the schema enforces them.
func (m *moduleData) patterns(st *yangparse.Statement) (enforced, unenforced []Pattern, err error) {
	for _, p := range direct(st, "pattern") {
		compiled := true
		if err := validateYANGPatternSyntax(p.Argument); err != nil {
			if m.ctx == nil || m.ctx.validationMode != ValidationVendorCompatible {
				if _, ok := errors.AsType[*yangpattern.ConfigurationError](err); ok {
					return nil, nil, fmt.Errorf("invalid pattern %q at %s: %w", p.Argument, p.Location(), err)
				}
				return nil, nil, fmt.Errorf("cannot compile pattern %q at %s: %w", p.Argument, p.Location(), err)
			}
			if reported := m.ctx.build.unenforcedPatterns; reported == nil || !reported[p] {
				if reported != nil {
					reported[p] = true
				}
				m.recordVendorCompatibleWarning(p, nil, "pattern %q at %s is not enforced in vendor-compatible mode: %v", p.Argument, p.Location(), err)
			}
			compiled = false
		}
		var tag *string
		errorMessage, err := constraintMetadataArg(p, "error-message")
		if err != nil {
			return nil, nil, err
		}
		if s, err := constraintMetadataArg(p, "error-app-tag"); err != nil {
			return nil, nil, err
		} else if s != "" {
			tag = ptr(s)
		}
		description, err := constraintMetadataArg(p, "description")
		if err != nil {
			return nil, nil, err
		}
		reference, err := constraintMetadataArg(p, "reference")
		if err != nil {
			return nil, nil, err
		}
		inverted := false
		modifiers := direct(p, "modifier")
		if len(modifiers) > 1 {
			return nil, nil, fmt.Errorf("pattern %q has multiple modifier statements at %s", p.Argument, modifiers[1].Location())
		}
		if len(modifiers) == 1 {
			if modifiers[0].Argument != "invert-match" {
				return nil, nil, fmt.Errorf("invalid pattern modifier %q at %s", modifiers[0].Argument, modifiers[0].Location())
			}
			inverted = true
		}
		pattern := Pattern{
			regex:        p.Argument,
			errorMessage: errorMessage,
			appTag:       tag,
			description:  description,
			reference:    reference,
			inverted:     inverted,
		}
		if compiled {
			enforced = append(enforced, pattern)
		} else {
			unenforced = append(unenforced, pattern)
		}
	}
	return enforced, unenforced, nil
}

func validateYANGPatternSyntax(pattern string) error {
	_, err := yangpattern.Compile(pattern)
	return err
}

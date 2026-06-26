// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium

import (
	"fmt"
	"strings"

	"github.com/signalbreak-labs/cambium/go/internal/yangparse"
)

func validateTopLevelStatementPlacement(st *yangparse.Statement) error {
	if st == nil || strings.Contains(st.Keyword, ":") || topLevelStatementAllowed(st.Keyword) {
		return nil
	}
	return fmt.Errorf("%s at %s is not valid at module or submodule top level", st.Keyword, st.Location())
}

func validateTopLevelStatementOrderMode(root *yangparse.Statement, mode ValidationMode) ([]Diagnostic, error) {
	if root == nil || root.Keyword != "module" && root.Keyword != "submodule" {
		return nil, nil
	}
	phase := topLevelOrderHeader
	var warnings []Diagnostic
	var previousRevision *yangparse.Statement
	for _, st := range root.SubStatements() {
		if mode == ValidationVendorCompatible && strings.Contains(st.Keyword, ":") {
			continue
		}
		next, ok := topLevelOrderPhase(st.Keyword)
		if !ok {
			continue
		}
		if next < phase {
			return nil, fmt.Errorf("%s %q is out of order in %s %q at %s", st.Keyword, st.Argument, root.Keyword, root.Argument, st.Location())
		}
		if next > phase {
			phase = next
		}
		if st.Keyword != "revision" {
			continue
		}
		if previousRevision != nil &&
			validRevisionDate(previousRevision.Argument) &&
			validRevisionDate(st.Argument) &&
			st.Argument > previousRevision.Argument {
			if mode != ValidationVendorCompatible {
				return nil, diagnosticErrorf(
					st,
					[]*yangparse.Statement{previousRevision},
					"revision %q is out of order in %s %q at %s",
					st.Argument,
					root.Keyword,
					root.Argument,
					st.Location(),
				)
			}
			warnings = append(warnings, outOfOrderRevisionWarning(root, previousRevision, st))
		}
		previousRevision = st
	}
	return warnings, nil
}

func outOfOrderRevisionWarning(owner, previous, current *yangparse.Statement) Diagnostic {
	ownerKind := "module"
	ownerName := ""
	if owner != nil {
		ownerKind = owner.Keyword
		ownerName = owner.Argument
	}
	currentRevision := ""
	previousRevision := ""
	if current != nil {
		currentRevision = current.Argument
	}
	if previous != nil {
		previousRevision = previous.Argument
	}
	message := fmt.Sprintf(
		"%s %q has out of order revision %q at %s; previous revision %q at %s",
		ownerKind,
		ownerName,
		currentRevision,
		locationText(current),
		previousRevision,
		locationText(previous),
	)
	return Diagnostic{
		Kind:       DiagnosticSemanticSchemaError,
		Code:       RuleCodeContext,
		Message:    message,
		Module:     ownerName,
		Source:     sourceLocation(current),
		Related:    sourceLocations([]*yangparse.Statement{previous}),
		Underlying: fmt.Errorf("%s", message),
	}
}

func (m *moduleData) collectScopedDefinitions(st *yangparse.Statement) error {
	if st == nil {
		return nil
	}
	for _, child := range st.SubStatements() {
		if err := validateNestedRootStatementPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedTypeStatementPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedModuleBodyStatementPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedTopLevelDefinitionPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedChildPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedAugmentPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedRefinePlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedOperationIOStatementChildPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedDeviatePlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedDeviateStatementChildPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedRevisionDatePlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedPrefixPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedNoStandardChildrenStatementChildPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedArgumentPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedYinElementPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedEnumBitValuePlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedTypeBodyStatementPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedConstraintMetadataPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedModifierPlacement(st, child); err != nil {
			return err
		}
		switch child.Keyword {
		case "typedef":
			if err := validateScopedDefinitionPlacement(st, child); err != nil {
				return err
			}
			if err := m.addTypedefDefinition(st, child); err != nil {
				return err
			}
		case "grouping":
			if err := validateScopedDefinitionPlacement(st, child); err != nil {
				return err
			}
			if err := m.addGroupingDefinition(st, child); err != nil {
				return err
			}
		}
		if err := m.collectScopedDefinitions(child); err != nil {
			return err
		}
	}
	return nil
}

func validateNestedRootStatementPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil {
		return nil
	}
	switch st.Keyword {
	case "module", "submodule":
	default:
		return nil
	}
	return fmt.Errorf("%s %q is not valid under %s %q at %s", st.Keyword, st.Argument, scope.Keyword, scope.Argument, st.Location())
}

func validateNestedTypeStatementPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || st.Keyword != "type" {
		return nil
	}
	if hasPrefix(scope.Keyword) {
		return nil
	}
	switch scope.Keyword {
	case "deviate", "type", "typedef":
		return nil
	}
	if kindForKeyword(scope.Keyword) != SchemaNodeKindUnknown {
		return nil
	}
	return fmt.Errorf("type %q is not valid under %s %q at %s", st.Argument, scope.Keyword, scope.Argument, st.Location())
}

func validateNestedAugmentPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || st.Keyword != "augment" || scope.Keyword == "uses" {
		return nil
	}
	return fmt.Errorf("augment %q is not valid under %s %q at %s", st.Argument, scope.Keyword, scope.Argument, st.Location())
}

func validateNestedRefinePlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || st.Keyword != "refine" || scope.Keyword == "uses" {
		return nil
	}
	return fmt.Errorf("refine %q is not valid under %s %q at %s", st.Argument, scope.Keyword, scope.Argument, st.Location())
}

func validateNestedDeviatePlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || st.Keyword != "deviate" || scope.Keyword == "deviation" {
		return nil
	}
	return fmt.Errorf("deviate %q is not valid under %s %q at %s", st.Argument, scope.Keyword, scope.Argument, st.Location())
}

func validateNestedDeviateStatementChildPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || scope.Keyword != "deviate" || hasPrefix(st.Keyword) {
		return nil
	}
	if !validDeviationType(scope.Argument) {
		return nil
	}
	if deviateStatementChildAllowed(scope.Argument, st.Keyword) {
		return nil
	}
	return fmt.Errorf("%s %q is not valid under deviate %q at %s", st.Keyword, st.Argument, scope.Argument, st.Location())
}

func deviateStatementChildAllowed(operation, keyword string) bool {
	switch operation {
	case "not-supported":
		return false
	case "add":
		switch keyword {
		case "config", "default", "mandatory", "max-elements", "min-elements", "must", "unique", "units":
			return true
		}
	case "replace":
		switch keyword {
		case "config", "default", "mandatory", "max-elements", "min-elements", "must", "type", "unique", "units":
			return true
		}
	case "delete":
		switch keyword {
		case "default", "max-elements", "min-elements", "must", "unique", "units":
			return true
		}
	}
	return false
}

func validateNestedRevisionDatePlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || st.Keyword != "revision-date" || scope.Keyword == "import" || scope.Keyword == "include" {
		return nil
	}
	return fmt.Errorf("revision-date %q is not valid under %s %q at %s", st.Argument, scope.Keyword, scope.Argument, st.Location())
}

func validateNestedPrefixPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || st.Keyword != "prefix" || scope.Keyword == "import" || scope.Keyword == "belongs-to" {
		return nil
	}
	return fmt.Errorf("prefix %q is not valid under %s %q at %s", st.Argument, scope.Keyword, scope.Argument, st.Location())
}

func validateNestedNoStandardChildrenStatementChildPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || hasPrefix(st.Keyword) {
		return nil
	}
	if !statementHasNoStandardChildren(scope.Keyword) {
		return nil
	}
	return fmt.Errorf("%s %q is not valid under %s %q at %s", st.Keyword, st.Argument, scope.Keyword, scope.Argument, st.Location())
}

func validateNestedArgumentPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || st.Keyword != "argument" || scope.Keyword == "extension" {
		return nil
	}
	return fmt.Errorf("argument %q is not valid under %s %q at %s", st.Argument, scope.Keyword, scope.Argument, st.Location())
}

func validateNestedYinElementPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || st.Keyword != "yin-element" || scope.Keyword == "argument" {
		return nil
	}
	return fmt.Errorf("yin-element %q is not valid under %s %q at %s", st.Argument, scope.Keyword, scope.Argument, st.Location())
}

func validateNestedEnumBitValuePlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil {
		return nil
	}
	switch st.Keyword {
	case "value":
		if scope.Keyword == "enum" {
			return nil
		}
	case "position":
		if scope.Keyword == "bit" {
			return nil
		}
	default:
		return nil
	}
	return fmt.Errorf("%s %q is not valid under %s %q at %s", st.Keyword, st.Argument, scope.Keyword, scope.Argument, st.Location())
}

func validateNestedTypeBodyStatementPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil {
		return nil
	}
	switch st.Keyword {
	case "base":
		if scope.Keyword == "identity" || scope.Keyword == "type" {
			return nil
		}
	case "bit", "enum", "fraction-digits", "length", "path", "pattern", "range", "require-instance":
		if scope.Keyword == "type" {
			return nil
		}
	default:
		return nil
	}
	return fmt.Errorf("%s %q is not valid under %s %q at %s", st.Keyword, st.Argument, scope.Keyword, scope.Argument, st.Location())
}

func validateNestedConstraintMetadataPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil {
		return nil
	}
	switch st.Keyword {
	case "error-message", "error-app-tag":
		if constraintMetadataScopeAllowed(scope.Keyword) {
			return nil
		}
	default:
		return nil
	}
	return fmt.Errorf("%s %q is not valid under %s %q at %s", st.Keyword, st.Argument, scope.Keyword, scope.Argument, st.Location())
}

func validateNestedModifierPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || st.Keyword != "modifier" || scope.Keyword == "pattern" {
		return nil
	}
	return fmt.Errorf("modifier %q is not valid under %s %q at %s", st.Argument, scope.Keyword, scope.Argument, st.Location())
}

func validateNestedModuleBodyStatementPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || !moduleBodyOnlyKeyword(st.Keyword) {
		return nil
	}
	return fmt.Errorf("%s %q is not valid under %s %q at %s", st.Keyword, st.Argument, scope.Keyword, scope.Argument, st.Location())
}

func validateNestedTopLevelDefinitionPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || !topLevelDefinitionKeyword(st.Keyword) {
		return nil
	}
	return fmt.Errorf("%s %q is not valid under %s %q at %s", st.Keyword, st.Argument, scope.Keyword, scope.Argument, st.Location())
}

func validateNestedOperationIOStatementChildPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || scope.Keyword != "input" && scope.Keyword != "output" || hasPrefix(st.Keyword) {
		return nil
	}
	if operationIOStatementChildAllowed(st.Keyword) {
		return nil
	}
	return fmt.Errorf("%s %q is not valid under %s at %s", st.Keyword, st.Argument, scope.Keyword, st.Location())
}

func operationIOStatementChildAllowed(keyword string) bool {
	switch keyword {
	case "anydata", "anyxml", "choice", "container", "grouping", "leaf", "leaf-list", "list", "must", "typedef", "uses":
		return true
	default:
		return false
	}
}

func keywordSet(keywords ...string) map[string]bool {
	m := make(map[string]bool, len(keywords))
	for _, k := range keywords {
		m[k] = true
	}
	return m
}

// nestedChildRules maps a scope statement keyword to the child statement
// keywords RFC 7950 permits directly under it. It is the single source of truth
// for validateNestedChildPlacement, replacing ~21 near-identical validator funcs
// and their switch predicates. A scope absent from the map is not constrained
// here (its children are validated elsewhere). An empty set means no child
// statements are allowed (e.g. yang-version).
var nestedChildRules = map[string]map[string]bool{
	"augment":      keywordSet("action", "anydata", "anyxml", "case", "choice", "container", "description", "if-feature", "leaf", "leaf-list", "list", "notification", "reference", "status", "uses", "when"),
	"refine":       keywordSet("config", "default", "description", "if-feature", "mandatory", "max-elements", "min-elements", "must", "presence", "reference"),
	"uses":         keywordSet("augment", "description", "if-feature", "reference", "refine", "status", "when"),
	"choice":       keywordSet("anydata", "anyxml", "case", "choice", "config", "container", "default", "description", "if-feature", "leaf", "leaf-list", "list", "mandatory", "reference", "status", "uses", "when"),
	"case":         keywordSet("anydata", "anyxml", "choice", "container", "description", "if-feature", "leaf", "leaf-list", "list", "reference", "status", "uses", "when"),
	"rpc":          keywordSet("description", "grouping", "if-feature", "input", "output", "reference", "status", "typedef"),
	"action":       keywordSet("description", "grouping", "if-feature", "input", "output", "reference", "status", "typedef"),
	"deviation":    keywordSet("description", "deviate", "if-feature", "reference"),
	"import":       keywordSet("description", "prefix", "reference", "revision-date"),
	"include":      keywordSet("description", "reference", "revision-date"),
	"revision":     keywordSet("description", "reference"),
	"belongs-to":   keywordSet("prefix"),
	"yang-version": keywordSet(),
	"argument":     keywordSet("yin-element"),
	"enum":         keywordSet("description", "if-feature", "reference", "status", "value"),
	"bit":          keywordSet("description", "if-feature", "position", "reference", "status"),
	"type":         keywordSet("base", "bit", "enum", "fraction-digits", "length", "path", "pattern", "range", "require-instance", "type"),
	"pattern":      keywordSet("description", "error-app-tag", "error-message", "modifier", "reference"),
	"range":        keywordSet("description", "error-app-tag", "error-message", "reference"),
	"length":       keywordSet("description", "error-app-tag", "error-message", "reference"),
	"must":         keywordSet("description", "error-app-tag", "error-message", "reference"),
	"when":         keywordSet("description", "reference"),
	"identity":     keywordSet("base", "description", "if-feature", "reference", "status"),
	"feature":      keywordSet("description", "if-feature", "reference", "status"),
	"extension":    keywordSet("argument", "description", "reference", "status"),
	"typedef":      keywordSet("default", "description", "reference", "status", "type", "units"),
	"grouping":     keywordSet("action", "anydata", "anyxml", "choice", "container", "description", "grouping", "leaf", "leaf-list", "list", "notification", "reference", "status", "typedef", "uses"),
	"notification": keywordSet("anydata", "anyxml", "choice", "container", "description", "grouping", "if-feature", "leaf", "leaf-list", "list", "must", "reference", "status", "typedef", "uses"),
}

// validateNestedChildPlacement rejects a child statement that RFC 7950 does not
// permit directly under its scope, using the shared diagnostic format. Scopes
// absent from nestedChildRules are not constrained here. Irregular scopes whose
// rule depends on more than the scope keyword (deviate, input/output) keep their
// own validators.
func validateNestedChildPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || hasPrefix(st.Keyword) {
		return nil
	}
	allowed, ok := nestedChildRules[scope.Keyword]
	if !ok || allowed[st.Keyword] {
		return nil
	}
	return fmt.Errorf("%s %q is not valid under %s %q at %s", st.Keyword, st.Argument, scope.Keyword, scope.Argument, st.Location())
}

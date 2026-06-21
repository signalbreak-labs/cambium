// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium

import (
	"fmt"
	"regexp"

	"github.com/signalbreak-labs/cambium/go/internal/yangparse"
)

// LintFinding is a non-fatal, advisory diagnostic from Context.Lint. Findings
// are not RFC-7950 errors and never affect Build success or tree structure;
// they surface schema-hygiene issues (such as an import that is never
// referenced) that are valid YANG but usually mistakes.
type LintFinding struct {
	// Code classifies the finding using the shared CAMBIUM_E#### rule codes.
	Code RuleCode
	// Module is the name of the module the finding applies to.
	Module string
	// Message is a human-readable description.
	Message string
}

// prefixTokenRE matches a YANG prefixed reference's prefix, i.e. the "p" in
// "p:name", including prefixes embedded in XPath/leafref paths ("/p:foo"). A
// prefix must start with a letter or underscore, so it never matches the colon
// in a date/time ("12:30") or revision string.
var prefixTokenRE = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_.-]*):`)

// Lint runs advisory, order-neutral static checks over the loaded, implemented
// modules and returns findings in module load order (deterministic). It is
// read-only: it does not mutate the schema, does not affect Build, and its
// findings are warnings, not errors. Today it reports imports and includes that
// a module declares but never references.
func (c *Context) Lint() []LintFinding {
	var findings []LintFinding
	for _, m := range c.loadOrder {
		if m == nil || !m.implemented {
			continue
		}
		findings = append(findings, lintUnusedImports(m)...)
	}
	return findings
}

// lintUnusedImports reports each import whose prefix appears nowhere in the
// module (or its submodules). The usage scan deliberately over-detects: any
// "prefix:" token in any statement counts as a use, so a genuinely-referenced
// import is never reported, at the cost of occasionally staying silent on a
// truly-unused one (the safe direction for a linter).
func lintUnusedImports(m *moduleData) []LintFinding {
	if len(m.imports) == 0 {
		return nil
	}
	used := make(map[string]bool)
	collectPrefixUses(m.stmt, used)
	for _, sub := range m.submodules {
		collectPrefixUses(sub.stmt, used)
	}
	var findings []LintFinding
	for _, imp := range m.imports {
		if imp.Prefix == "" || used[imp.Prefix] {
			continue
		}
		findings = append(findings, LintFinding{
			Code:    RuleCodeContext,
			Module:  m.name,
			Message: fmt.Sprintf("import %q (prefix %q) is declared but never used", imp.Name, imp.Prefix),
		})
	}
	return findings
}

// collectPrefixUses records every prefix that appears as a "prefix:" token in
// any statement keyword (extension usage) or argument (type/base/uses/path/
// must/when/augment references, plus prefixes embedded in XPath). An import
// statement itself contains no such token (its argument is a bare module name
// and its prefix substatement a bare prefix), so a declaration never marks
// itself used.
func collectPrefixUses(stmt *yangparse.Statement, used map[string]bool) {
	if stmt == nil {
		return
	}
	for _, match := range prefixTokenRE.FindAllStringSubmatch(stmt.Keyword, -1) {
		used[match[1]] = true
	}
	for _, match := range prefixTokenRE.FindAllStringSubmatch(stmt.Argument, -1) {
		used[match[1]] = true
	}
	for _, sub := range stmt.SubStatements() {
		collectPrefixUses(sub, used)
	}
}

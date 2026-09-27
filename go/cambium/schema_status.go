// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium

import (
	"strings"

	"github.com/signalbreak-labs/cambium/go/internal/yangparse"
)

// validateStatusReferences enforces RFC 7950 §7.21.2 within one module: a
// current definition must not reference a deprecated or obsolete definition,
// and a deprecated definition must not reference an obsolete one. It covers
// references to typedefs (type), groupings (uses), identities (base), and
// features (if-feature). A definition without a status statement takes the
// status of its closest ancestor that has one, as libyang and pyang do.
// Vendor-compatible mode reports each violation as a LoadReport warning.
func (m *moduleData) validateStatusReferences() {
	if m == nil || m.stmt == nil || m.schemaErr != nil {
		return
	}
	for _, top := range m.sourceTopStatements() {
		if top.Keyword == "deviation" {
			continue
		}
		m.walkStatusReferences(top)
		if m.schemaErr != nil {
			return
		}
	}
}

func (m *moduleData) walkStatusReferences(st *yangparse.Statement) {
	if strings.Contains(st.Keyword, ":") {
		// Extension statements carry no YANG definitions to check.
		return
	}
	switch st.Keyword {
	case "type":
		if mod, td := m.lookupTypedefModuleFrom(st.Argument, st); td != nil && mod == m {
			m.checkStatusReference(st, "typedef", td)
		}
	case "uses":
		if mod, def := m.findGroupingFrom(st.Argument, st); def != nil && mod == m {
			m.checkStatusReference(st, "grouping", def)
		}
	case "base":
		if mod := m.resolveSourceQNameModuleFrom(st.Argument, st); mod == m {
			if id := m.identityMap[localName(st.Argument)]; id != nil {
				m.checkStatusReference(st, "identity", id.stmt)
			}
		}
	case "if-feature":
		for _, ref := range m.ifFeatureExprRefs(st.Argument, st) {
			if mod := m.resolveSourceQNameModuleFrom(ref, st); mod == m {
				if feature := m.featureMap[localName(ref)]; feature != nil {
					m.checkStatusReference(st, "feature", feature.stmt)
				}
			}
		}
	}
	for _, child := range st.SubStatements() {
		if m.schemaErr != nil {
			return
		}
		m.walkStatusReferences(child)
	}
}

func (m *moduleData) checkStatusReference(ref *yangparse.Statement, targetKind string, target *yangparse.Statement) {
	if target == nil {
		return
	}
	owner := m.statusOwner(ref)
	if owner == nil {
		return
	}
	ownerStatus := m.effectiveStatementStatus(owner)
	targetStatus := m.effectiveStatementStatus(target)
	if targetStatus <= ownerStatus {
		return
	}
	if err := m.sourceRuleViolation(ref, []*yangparse.Statement{target}, "%s %s %q must not reference %s %s %q at %s",
		statusKeyword(ownerStatus), owner.Keyword, owner.Argument, statusKeyword(targetStatus), targetKind, target.Argument, ref.Location()); err != nil {
		m.recordSchemaError(err)
	}
}

// statusOwner returns the closest definition, ref itself included, that can
// carry a status statement.
func (m *moduleData) statusOwner(ref *yangparse.Statement) *yangparse.Statement {
	for cur := ref; cur != nil; cur = m.statementParents[cur] {
		if statusBearingKeyword(cur.Keyword) {
			return cur
		}
	}
	return nil
}

// effectiveStatementStatus returns the status of st's own status statement,
// or else that of its closest ancestor with one.
func (m *moduleData) effectiveStatementStatus(st *yangparse.Statement) Status {
	for cur := st; cur != nil; cur = m.statementParents[cur] {
		if first(cur, "status") != nil {
			return statusFromStatement(cur)
		}
	}
	return StatusCurrent
}

// ifFeatureExprRefs returns the feature references of an if-feature
// expression in source order.
func (m *moduleData) ifFeatureExprRefs(expr string, from *yangparse.Statement) []string {
	if ref, ok := m.yang10SingleIfFeatureRef(expr, from); ok {
		return []string{ref}
	}
	tokens, ok := tokenizeIfFeatureExpr(expr)
	if !ok {
		return nil
	}
	var refs []string
	for _, token := range tokens {
		if token.kind == ifFeatureTokenIdent {
			refs = append(refs, token.text)
		}
	}
	return refs
}

func statusBearingKeyword(keyword string) bool {
	switch keyword {
	case "action", "anydata", "anyxml", "augment", "bit", "case", "choice", "container", "enum", "extension", "feature", "grouping", "identity", "leaf", "leaf-list", "list", "notification", "rpc", "typedef", "uses":
		return true
	default:
		return false
	}
}

func statusKeyword(status Status) string {
	switch status {
	case StatusDeprecated:
		return "deprecated"
	case StatusObsolete:
		return "obsolete"
	default:
		return "current"
	}
}

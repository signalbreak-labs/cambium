// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium

import "github.com/signalbreak-labs/cambium/go/internal/yangparse"

// RefinementPolicy controls uses/refine effects on the effective schema.
// The zero value applies RFC refinements. IgnoreAll is an explicit compatibility
// mode that preserves grouping declarations, including their original
// constraints, instead of applying any refine property or feature condition.
// Declaration syntax and shape are validated; ignored targets need not exist.
type RefinementPolicy struct {
	IgnoreAll bool
}

// SetRefinementPolicy selects how uses/refine statements affect this context.
// It must be called before Build, including when the builder has been copied.
func (b *ContextBuilder) SetRefinementPolicy(policy RefinementPolicy) error {
	if err := b.ensureMutable(); err != nil {
		return err
	}
	if b.ctx.refinementPolicy != policy {
		b.ctx.refinementPolicy = policy
		b.ctx.dirty = true
	}
	return nil
}

func validateIgnoredRefinement(refine *yangparse.Statement) error {
	for _, keyword := range []string{"default", "description", "reference", "mandatory", "config", "presence", "min-elements", "max-elements"} {
		if _, err := singletonChild(refine, keyword); err != nil {
			return err
		}
	}
	for _, prop := range nonExtensionSubStatements(refine) {
		if err := validateIgnoredAmendmentProperty(prop); err != nil {
			return err
		}
	}
	return nil
}

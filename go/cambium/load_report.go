// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium

import "fmt"

// LoadReport describes what participated in a built schema context. It is for
// observability and downstream tooling; it does not change validation behavior.
type LoadReport struct {
	RequestedModules   []ModuleLoadInfo
	TransitiveImports  []ModuleLoadInfo
	IncludedSubmodules []SubmoduleLoadInfo
	DeviationModules   []ModuleLoadInfo
	EnabledFeatures    []FeatureSelection
	DisabledFeatures   []FeatureSelection
	SkippedModules     []ModuleLoadInfo
	Warnings           []Diagnostic
	SourceFiles        []string
	// DeviationPolicy is the policy the context was built with.
	DeviationPolicy DeviationPolicy
	// IgnoredDeviations lists, in load order, the deviations that
	// DeviationPolicy kept from changing the effective schema.
	IgnoredDeviations []Deviation
}

// ModuleLoadInfo is stable metadata for one loaded module.
type ModuleLoadInfo struct {
	Module      Module
	Name        string
	Revision    string
	Namespace   string
	Prefix      string
	SourceFile  string
	Implemented bool
	Requested   bool
}

// SubmoduleLoadInfo is stable metadata for an included submodule.
type SubmoduleLoadInfo struct {
	Name       string
	Parent     string
	Revision   string
	SourceFile string
	Source     SourceLocation
}

// FeatureSelection reports the enabled/disabled state of one declared feature.
type FeatureSelection struct {
	Module  string
	Feature string
	Enabled bool
}

// LoadReport returns a stable snapshot of loaded modules, transitive imports,
// includes, feature states, deviations, diagnostics, and source paths.
func (c *Context) LoadReport() LoadReport {
	if c == nil || c.closed {
		return LoadReport{}
	}
	rebuildErr := c.rebuildIfDirty()

	var report LoadReport
	report.DeviationPolicy = c.deviationPolicy
	report.Warnings = append(report.Warnings, c.loadWarnings...)
	if rebuildErr != nil {
		diag := DiagnosticFromError(wrap("load report: schema rebuild", rebuildErr))
		diag.Message = "schema rebuild: " + diag.Message
		report.Warnings = append(report.Warnings, diag)
	}
	seenSource := make(map[string]bool)
	addSource := func(path string) {
		if path == "" || seenSource[path] {
			return
		}
		seenSource[path] = true
		report.SourceFiles = append(report.SourceFiles, path)
	}

	for _, mod := range c.loadOrder {
		if mod == nil || mod.stmt == nil {
			continue
		}
		info := moduleLoadInfo(mod)
		addSource(info.SourceFile)
		if mod.requested {
			report.RequestedModules = append(report.RequestedModules, info)
		} else {
			report.TransitiveImports = append(report.TransitiveImports, info)
		}
		if len(mod.deviations) > 0 {
			report.DeviationModules = append(report.DeviationModules, info)
		}
		for _, dev := range mod.deviations {
			if !dev.Applied() {
				report.IgnoredDeviations = append(report.IgnoredDeviations, dev)
			}
		}
		for _, sub := range mod.submodules {
			if sub == nil || sub.stmt == nil {
				continue
			}
			subInfo := SubmoduleLoadInfo{
				Name:       sub.stmt.Argument,
				Parent:     mod.name,
				Revision:   moduleRevision(sub.stmt),
				SourceFile: sub.file,
				Source:     sourceLocation(sub.stmt),
			}
			report.IncludedSubmodules = append(report.IncludedSubmodules, subInfo)
			addSource(subInfo.SourceFile)
		}
		for _, feature := range mod.features {
			if feature == nil {
				continue
			}
			enabled := mod.featureEnabled(feature.name)
			if _, requested := c.enabledFeatures[mod.name][feature.name]; requested && !enabled {
				// The caller asked for the feature, but its own if-feature
				// condition is false, so it is effectively disabled.
				report.Warnings = append(report.Warnings, Diagnostic{
					Kind:       DiagnosticSemanticSchemaError,
					Code:       RuleCodeContext,
					Message:    fmt.Sprintf("feature %q for module %q was enabled but its if-feature condition is false; it is effectively disabled", feature.name, mod.name),
					Module:     mod.name,
					Source:     sourceLocation(feature.stmt),
					Underlying: fmt.Errorf("feature %q effectively disabled", feature.name),
				})
			}
			selection := FeatureSelection{Module: mod.name, Feature: feature.name, Enabled: enabled}
			if enabled {
				report.EnabledFeatures = append(report.EnabledFeatures, selection)
			} else {
				report.DisabledFeatures = append(report.DisabledFeatures, selection)
			}
		}
		if mod.schemaErr != nil {
			report.Warnings = append(report.Warnings, DiagnosticFromError(wrap("schema tree", mod.schemaErr)))
		}
	}
	return report
}

// OmittedContent returns the warnings for declared schema content that a
// vendor-compatible relaxation left out of the effective schema, in report
// order. An empty result means no relaxation dropped declared content; callers
// that need a complete schema should reject a non-empty result, and callers
// that reject every relaxation should reject any Warnings.
func (r LoadReport) OmittedContent() []Diagnostic {
	var out []Diagnostic
	for _, diag := range r.Warnings {
		if diag.Kind == DiagnosticOmittedSchemaContent {
			out = append(out, diag)
		}
	}
	return out
}

func moduleLoadInfo(mod *moduleData) ModuleLoadInfo {
	if mod == nil {
		return ModuleLoadInfo{}
	}
	return ModuleLoadInfo{
		Module:      Module{mod: mod},
		Name:        mod.name,
		Revision:    mod.revision,
		Namespace:   mod.namespace,
		Prefix:      mod.prefix,
		SourceFile:  mod.file,
		Implemented: mod.implemented,
		Requested:   mod.requested,
	}
}

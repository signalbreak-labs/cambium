// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium

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
	_ = c.rebuildIfDirty()

	var report LoadReport
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

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package compat

import "github.com/signalbreak-labs/cambium/go/cambium"

// LoadReport aliases the native Cambium load report for compat callers.
type LoadReport = cambium.LoadReport

// ModuleLoadInfo aliases native module load metadata for compat callers.
type ModuleLoadInfo = cambium.ModuleLoadInfo

// SubmoduleLoadInfo aliases native submodule load metadata for compat callers.
type SubmoduleLoadInfo = cambium.SubmoduleLoadInfo

// FeatureSelection aliases native feature selection metadata for compat callers.
type FeatureSelection = cambium.FeatureSelection

// LoadReport returns the native load report for the last successfully processed
// module set. Call Process or GetModule first; otherwise the zero report is
// returned.
func (ms *Modules) LoadReport() LoadReport {
	if ms == nil || ms.ctx == nil || !ms.built {
		return LoadReport{}
	}
	return ms.ctx.LoadReport()
}

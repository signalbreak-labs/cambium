// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package yangpattern

import "fmt"

// ResourceError means an engine could not establish a result within its policy.
// It is never a successful match or a proven nonmatch.
type ResourceError struct {
	Phase, Stage, Limit string
	Allowance, Observed uint64
}

func (e *ResourceError) Error() string {
	return fmt.Sprintf("pattern %s/%s: %s allowance %d exceeded by %d", e.Phase, e.Stage, e.Limit, e.Allowance, e.Observed)
}

// ConfigurationError reports malformed syntax or portable metadata.
type ConfigurationError struct{ Detail string }

func (e *ConfigurationError) Error() string { return "pattern configuration: " + e.Detail }

// CompileLimits bounds source parsing and normalization. Zero fields select defaults.
// Defaults: 256 KiB source, 32768 nodes, 65536 edges, 256 depth, 65536 decimal
// digits, 131072 ranges, 16 MiB accounted storage, and 5000000 work units.
type CompileLimits struct{ SourceBytes, Nodes, Edges, Depth, Digits, Ranges, Bytes, Work uint64 }

// RestoreLimits bounds validation and copying, separately from compilation/matching.
// Defaults match CompileLimits, with 16 MiB of DTO storage including Source.
type RestoreLimits struct{ Bytes, Nodes, Edges, Depth, Digits, Ranges, Work uint64 }

// MatchLimits bounds amplification over the unavoidable streaming input scan.
// Zero fields select defaults: 32768 nodes, 131072 transitions, 16 MiB storage,
// 5000000 structural work, 16384 native instructions, 256 KiB native source,
// and 200000000 native instruction*(runes+1) units. Input length is not limited.
type MatchLimits struct{ Nodes, Transitions, Bytes, Work, NativeInstructions, NativeSourceBytes, NativeWork uint64 }

func defaultValue(v, d uint64) uint64 {
	if v == 0 {
		return d
	}
	return v
}
func (l CompileLimits) defaults() CompileLimits {
	return CompileLimits{defaultValue(l.SourceBytes, 256<<10), defaultValue(l.Nodes, 32768), defaultValue(l.Edges, 65536), defaultValue(l.Depth, 256), defaultValue(l.Digits, 65536), defaultValue(l.Ranges, 131072), defaultValue(l.Bytes, 16<<20), defaultValue(l.Work, 5000000)}
}
func (l RestoreLimits) defaults() RestoreLimits {
	return RestoreLimits{defaultValue(l.Bytes, 16<<20), defaultValue(l.Nodes, 32768), defaultValue(l.Edges, 65536), defaultValue(l.Depth, 256), defaultValue(l.Digits, 65536), defaultValue(l.Ranges, 131072), defaultValue(l.Work, 5000000)}
}
func (l MatchLimits) defaults() MatchLimits {
	return MatchLimits{defaultValue(l.Nodes, 32768), defaultValue(l.Transitions, 131072), defaultValue(l.Bytes, 16<<20), defaultValue(l.Work, 5000000), defaultValue(l.NativeInstructions, 16384), defaultValue(l.NativeSourceBytes, 256<<10), defaultValue(l.NativeWork, 200000000)}
}
func addSat(a, b uint64) uint64 {
	if ^uint64(0)-a < b {
		return ^uint64(0)
	}
	return a + b
}
func mulSat(a, b uint64) uint64 {
	if a != 0 && b > ^uint64(0)/a {
		return ^uint64(0)
	}
	return a * b
}
func limitError(phase, stage, name string, limit, observed uint64) error {
	// Saturation represents an unrepresentable estimate, never permission to
	// allocate, including when a caller supplies the largest uint64 allowance.
	if observed == ^uint64(0) {
		return &ResourceError{phase, stage, name + " overflow", min(limit, observed-1), observed}
	}
	if observed > limit {
		return &ResourceError{phase, stage, name, limit, observed}
	}
	return nil
}

type ledger struct {
	phase, stage                      string
	bytes, work, byteLimit, workLimit uint64
	err                               error
}

func (b *ledger) charge(bytes, work uint64) bool {
	if b.err != nil {
		return false
	}
	nextBytes, nextWork := addSat(b.bytes, bytes), addSat(b.work, work)
	if err := limitError(b.phase, b.stage, "bytes", b.byteLimit, nextBytes); err != nil {
		b.err = err
		return false
	}
	if err := limitError(b.phase, b.stage, "work", b.workLimit, nextWork); err != nil {
		b.err = err
		return false
	}
	b.bytes, b.work = nextBytes, nextWork
	return true
}

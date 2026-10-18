// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package yangpattern

import "embed"

// SourceFile is one independently copyable runtime file. Generators must preserve
// Name, package declaration, attribution and Data bytes exactly.
type SourceFile struct {
	Name string
	Data []byte
}

//go:embed runtime_limits.go runtime_program.go runtime_native.go runtime_match.go runtime_derivative.go
var runtimeSources embed.FS

// RuntimeSources returns an explicit ordered inventory with fresh byte slices.
// These files compile together using only the Go 1.26 standard library. They
// expose Restore and matching; the source compiler is deliberately not included.
func RuntimeSources() []SourceFile {
	names := [...]string{"runtime_limits.go", "runtime_program.go", "runtime_native.go", "runtime_match.go", "runtime_derivative.go"}
	out := make([]SourceFile, 0, len(names))
	for _, name := range names {
		// Every name is a literal required by go:embed, so build-time validation
		// guarantees it exists. ReadFile returns a new independently owned slice.
		data, _ := runtimeSources.ReadFile(name)
		out = append(out, SourceFile{Name: name, Data: data})
	}
	return out
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium

import (
	"errors"
	"fmt"
	"strings"

	"github.com/signalbreak-labs/cambium/go/internal/yangparse"
)

// DiagnosticKind classifies a schema/load diagnostic for machine inspection.
type DiagnosticKind string

const (
	// DiagnosticUnknown is the fallback when Cambium cannot classify a failure.
	DiagnosticUnknown DiagnosticKind = "unknown"
	// DiagnosticSyntaxError is a parse/syntax failure.
	DiagnosticSyntaxError DiagnosticKind = "syntax_error"
	// DiagnosticInvalidIdentifier is an invalid YANG identifier or identifier-ref.
	DiagnosticInvalidIdentifier DiagnosticKind = "invalid_identifier"
	// DiagnosticMissingModule is a missing module/import/include failure.
	DiagnosticMissingModule DiagnosticKind = "missing_module"
	// DiagnosticUnresolvedPath is an unresolved schema path or XPath target.
	DiagnosticUnresolvedPath DiagnosticKind = "unresolved_path"
	// DiagnosticInvalidDeviation is an invalid deviation statement or effect.
	DiagnosticInvalidDeviation DiagnosticKind = "invalid_deviation"
	// DiagnosticSemanticSchemaError is a schema semantic validation failure.
	DiagnosticSemanticSchemaError DiagnosticKind = "semantic_schema_error"
	// DiagnosticUnsupportedConstruct is a syntactically valid construct Cambium
	// does not support in the selected tier.
	DiagnosticUnsupportedConstruct DiagnosticKind = "unsupported_construct"
)

// Diagnostic is a structured error or warning. Related contains secondary
// locations when Cambium can identify them.
type Diagnostic struct {
	Kind       DiagnosticKind
	Code       RuleCode
	Message    string
	Module     string
	Path       string
	Source     SourceLocation
	Related    []SourceLocation
	Underlying error
}

// DiagnosticError is a structured diagnostic cause that can be recovered with
// errors.As. Cambium wraps these in *Error at public operation boundaries while
// preserving source and related locations for DiagnosticFromError.
type DiagnosticError struct {
	Kind    DiagnosticKind
	Module  string
	Path    string
	Source  SourceLocation
	Related []SourceLocation
	Err     error
}

func (e *DiagnosticError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *DiagnosticError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// Diagnostic returns a structured representation of this Cambium error.
func (e *Error) Diagnostic() Diagnostic {
	if e == nil {
		return Diagnostic{}
	}
	return diagnosticForError(e)
}

// DiagnosticFromError converts err to a structured Diagnostic. It understands
// Cambium wrapper errors via errors.As and falls back to message classification.
func DiagnosticFromError(err error) Diagnostic {
	if err == nil {
		return Diagnostic{}
	}
	var cambiumErr *Error
	if errors.As(err, &cambiumErr) {
		return cambiumErr.Diagnostic()
	}
	diag := Diagnostic{
		Kind:       classifyDiagnostic(err.Error()),
		Code:       RuleCodeUnknown,
		Message:    err.Error(),
		Source:     SourceLocation{Text: "unknown"},
		Underlying: err,
	}
	applyDiagnosticCause(&diag, err)
	return diag
}

func diagnosticForError(err *Error) Diagnostic {
	diag := Diagnostic{
		Kind:       classifyDiagnostic(err.Err.Error()),
		Code:       err.Code,
		Message:    err.Err.Error(),
		Source:     SourceLocation{Text: "unknown"},
		Underlying: err.Err,
	}
	applyDiagnosticCause(&diag, err.Err)
	var pathErr *SchemaPathError
	if errors.As(err.Err, &pathErr) {
		diag.Kind = DiagnosticUnresolvedPath
		diag.Path = pathErr.Path
	}
	var leafrefErr *LeafrefResolutionError
	if errors.As(err.Err, &leafrefErr) {
		diag.Path = leafrefErr.Path
		if leafrefErr.Node.Path() != "" {
			diag.Path = leafrefErr.Node.Path()
		}
		diag.Module = leafrefErr.Node.Module().Name()
		if sourceLocationKnown(leafrefErr.Node.SourceLocation()) {
			diag.Source = leafrefErr.Node.SourceLocation()
		}
	}
	return diag
}

func applyDiagnosticCause(diag *Diagnostic, err error) {
	if diag == nil || err == nil {
		return
	}
	var diagnosticErr *DiagnosticError
	if !errors.As(err, &diagnosticErr) || diagnosticErr == nil {
		return
	}
	if diagnosticErr.Kind != "" {
		diag.Kind = diagnosticErr.Kind
	}
	if diagnosticErr.Module != "" {
		diag.Module = diagnosticErr.Module
	}
	if diagnosticErr.Path != "" {
		diag.Path = diagnosticErr.Path
	}
	if sourceLocationKnown(diagnosticErr.Source) {
		diag.Source = diagnosticErr.Source
	}
	diag.Related = append([]SourceLocation(nil), diagnosticErr.Related...)
	diag.Message = diagnosticErr.Error()
	diag.Underlying = diagnosticErr
}

func diagnosticErrorf(kind DiagnosticKind, source *yangparse.Statement, related []*yangparse.Statement, format string, args ...any) error {
	return &DiagnosticError{
		Kind:    kind,
		Source:  sourceLocation(source),
		Related: sourceLocations(related),
		Err:     fmt.Errorf(format, args...),
	}
}

func sourceLocations(statements []*yangparse.Statement) []SourceLocation {
	var out []SourceLocation
	for _, statement := range statements {
		location := sourceLocation(statement)
		if sourceLocationKnown(location) {
			out = append(out, location)
		}
	}
	return out
}

func sourceLocationKnown(location SourceLocation) bool {
	return location.Text != "" && location.Text != "unknown" || location.File != "" || location.Line != 0 || location.Column != 0
}

func classifyDiagnostic(message string) DiagnosticKind {
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "invalid identifier"):
		return DiagnosticInvalidIdentifier
	case strings.Contains(lower, "not found in search path"),
		strings.Contains(lower, "no such module"),
		strings.Contains(lower, "no such submodule"),
		strings.Contains(lower, "unknown module"):
		return DiagnosticMissingModule
	case strings.Contains(lower, "target not found"),
		strings.Contains(lower, "not found from"),
		strings.Contains(lower, "unresolved"):
		return DiagnosticUnresolvedPath
	case strings.Contains(lower, "deviation") || strings.Contains(lower, "deviate"):
		return DiagnosticInvalidDeviation
	case strings.Contains(lower, "unsupported"):
		return DiagnosticUnsupportedConstruct
	case strings.Contains(lower, "syntax"),
		strings.Contains(lower, "unexpected"),
		strings.Contains(lower, "parse"):
		return DiagnosticSyntaxError
	case strings.Contains(lower, "invalid"),
		strings.Contains(lower, "duplicate"),
		strings.Contains(lower, "requires"),
		strings.Contains(lower, "must"):
		return DiagnosticSemanticSchemaError
	default:
		return DiagnosticUnknown
	}
}

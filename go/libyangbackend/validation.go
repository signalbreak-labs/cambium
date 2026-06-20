//go:build cgo

// Structured validation diagnostics for multi-error RFC-7950 validation.
//
// This file mirrors rust/cambium-core/src/error.rs and the validation diagnostic
// classification in rust/cambium-core/src/tree.rs.

package libyangbackend

import (
	"fmt"
	"strings"

	"github.com/signalbreak-labs/cambium/go/internal/libyang"
)

// ErrorType classifies a diagnostic as application- or protocol-level.
type ErrorType int

const (
	// ErrorTypeApplication is an application-level validation or data error.
	ErrorTypeApplication ErrorType = iota
	// ErrorTypeProtocol is a transport/protocol-level error.
	ErrorTypeProtocol
)

// ValidationCode is a fine-grained sub-code for a validation failure.
type ValidationCode int

const (
	// ValidationNone means the validation sub-code could not be inferred.
	ValidationNone ValidationCode = iota
	// ValidationMust is a violated `must` constraint.
	ValidationMust
	// ValidationWhen is a violated `when` condition.
	ValidationWhen
	// ValidationMandatory is a violated `mandatory`, `min-elements`,
	// `max-elements`, or choice constraint.
	ValidationMandatory
	// ValidationLeafref is an unresolved leafref.
	ValidationLeafref
	// ValidationInvalidValue is a value that does not match its type.
	ValidationInvalidValue
)

// Diagnostic is one structured validation diagnostic from the engine.
type Diagnostic struct {
	// Code is the stable top-level rule code (always RuleCodeValidate here).
	Code RuleCode
	// Message is the human-readable message from the engine.
	Message string
	// DataPath is the data instance path, if available.
	DataPath string
	// SchemaPath is the schema path, if available.
	SchemaPath string
	// ErrorType is the application/protocol classification.
	ErrorType ErrorType
	// ErrorAppTag is the RFC 8040 / RFC 7950 error-app-tag, if provided.
	ErrorAppTag string
	// ValidationCode is a fine-grained sub-code (ValidationNone means absent).
	ValidationCode ValidationCode
}

// ValidationErrors is a non-empty collection of validation diagnostics.
type ValidationErrors struct {
	diagnostics []Diagnostic
}

// Len returns the number of diagnostics.
func (v *ValidationErrors) Len() int {
	if v == nil {
		return 0
	}
	return len(v.diagnostics)
}

// IsEmpty reports whether there are no diagnostics.
func (v *ValidationErrors) IsEmpty() bool {
	return v == nil || len(v.diagnostics) == 0
}

// Primary returns the first diagnostic, treated as the primary failure.
func (v *ValidationErrors) Primary() (Diagnostic, bool) {
	if v == nil || len(v.diagnostics) == 0 {
		return Diagnostic{}, false
	}
	return v.diagnostics[0], true
}

// Diagnostics returns all diagnostics in engine order.
func (v *ValidationErrors) Diagnostics() []Diagnostic {
	if v == nil {
		return nil
	}
	out := make([]Diagnostic, len(v.diagnostics))
	copy(out, v.diagnostics)
	return out
}

func (v *ValidationErrors) Error() string {
	if v == nil {
		return "validation failed with 0 diagnostic(s)"
	}
	return fmt.Sprintf("validation failed with %d diagnostic(s)", len(v.diagnostics))
}

// Validate validates the tree. If validation fails, the returned error wraps a
// *ValidationErrors that can be recovered with errors.As.
func (t *DataTree) Validate(mode ValidateMode) error {
	ve, err := t.validateCollect(mode)
	if err != nil {
		return wrap("validate", err)
	}
	if ve == nil || ve.IsEmpty() {
		return nil
	}
	return &Error{Code: RuleCodeValidate, Op: "validate", Err: ve}
}

// validateCollect validates the tree and returns every diagnostic libyang
// produced. A successful validation returns (nil, nil).
func (t *DataTree) validateCollect(mode ValidateMode) (*ValidationErrors, error) {
	var opts uint32
	if mode.NoState {
		opts |= libyang.ValidateNoState
	}
	if mode.Present {
		opts |= libyang.ValidatePresent
	}
	if mode.MultiError {
		opts |= libyang.ValidateMultiError
	}
	raws, err := t.raw.ValidateCollect(opts)
	if err != nil {
		return nil, wrap("validate", err)
	}
	if len(raws) == 0 {
		return nil, nil
	}
	out := make([]Diagnostic, len(raws))
	for i, raw := range raws {
		out[i] = diagnosticFromRaw(raw)
	}
	return &ValidationErrors{diagnostics: out}, nil
}

func diagnosticFromRaw(raw libyang.RawDiagnostic) Diagnostic {
	return Diagnostic{
		Code:           RuleCodeValidate,
		Message:        raw.Message,
		DataPath:       raw.DataPath,
		SchemaPath:     raw.SchemaPath,
		ErrorType:      ErrorTypeApplication,
		ErrorAppTag:    raw.AppTag,
		ValidationCode: validationCodeFromRaw(raw),
	}
}

func validationCodeFromRaw(raw libyang.RawDiagnostic) ValidationCode {
	msg := raw.Message
	switch raw.AppTag {
	case "must-violation":
		return ValidationMust
	case "instance-required":
		return ValidationLeafref
	case "too-few-elements", "too-many-elements", "missing-choice":
		return ValidationMandatory
	}
	// libyang emits `When` and `Mandatory` violations with NO app-tag and a
	// LYVE_DATA vecode that is too coarse to disambiguate, so they are
	// classified by message prefix. This is wording-dependent: a libyang bump
	// could change these strings and silently degrade the sub-code to
	// ValidationNone. The prefixes are pinned by TestValidateMustAndWhen and
	// TestValidateMandatoryMissing — update both here and there on a bump.
	if strings.HasPrefix(msg, "When condition") {
		return ValidationWhen
	}
	if strings.HasPrefix(msg, "Mandatory node") || strings.HasPrefix(msg, "Mandatory choice") {
		return ValidationMandatory
	}
	if strings.Contains(msg, "min-elements") || strings.Contains(msg, "max-elements") {
		return ValidationMandatory
	}
	if strings.EqualFold(raw.VecodeStr, "data") {
		return ValidationInvalidValue
	}
	return ValidationNone
}

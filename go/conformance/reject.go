// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

//go:build cgo

package conformance

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	core "github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/datatree"
	backend "github.com/signalbreak-labs/cambium/go/libyangbackend"
)

// Wrong verdicts, reported after the engine that gave them.
const (
	msgAcceptedReject = "accepted a document the case expects to be rejected"
	msgRejectedAccept = "rejected a document the case expects to be accepted"
)

// runRejectCase asserts that libyang refuses c's input and, for an oracle case
// with CAMBIUM_YANGLINT set, that yanglint refuses it too.
func runRejectCase(conformanceDir string, c Case) error {
	input, err := rejectCaseInput(conformanceDir, c)
	if err != nil {
		return err
	}
	rejection, err := libyangRejection(conformanceDir, c, input)
	if err != nil {
		return err
	}
	if rejection == nil {
		return fmt.Errorf("libyang: %s", msgAcceptedReject)
	}
	yanglint := strings.TrimSpace(os.Getenv("CAMBIUM_YANGLINT"))
	if !c.Oracle || yanglint == "" {
		return nil
	}
	format, err := parseFormat(c.InputFormat)
	if err != nil {
		return err
	}
	_, err = runYanglintOracle(yanglint, filepath.Join(conformanceDir, c.Module), filepath.Join(conformanceDir, c.Input), format, backend.WithDefaultsExplicit, "")
	switch {
	case err == nil:
		return fmt.Errorf("yanglint oracle: %s", msgAcceptedReject)
	case errors.Is(err, errYanglintFailed):
		return nil
	default:
		return err
	}
}

// runDataTreeRejectCase asserts that both libyang and datatree refuse c's
// input.
func runDataTreeRejectCase(conformanceDir string, c Case) error {
	input, err := rejectCaseInput(conformanceDir, c)
	if err != nil {
		return err
	}
	rejection, err := libyangRejection(conformanceDir, c, input)
	if err != nil {
		return fmt.Errorf("libyang: %w", err)
	}
	if rejection == nil {
		return fmt.Errorf("libyang: %s", msgAcceptedReject)
	}
	rejection, err = dataTreeRejection(conformanceDir, c, input)
	if err != nil {
		return fmt.Errorf("datatree: %w", err)
	}
	if rejection == nil {
		return fmt.Errorf("datatree: %s", msgAcceptedReject)
	}
	return nil
}

// rejectCaseInput reads a reject case's input. A document with no top-level
// data node is refused: the backend reports an error for any empty document,
// valid or not, so its refusal would prove nothing.
func rejectCaseInput(conformanceDir string, c Case) ([]byte, error) {
	if c.Input == "" || c.InputFormat == "" {
		return nil, fmt.Errorf("case %q has no input or input-format", c.Name)
	}
	input, err := os.ReadFile(filepath.Join(conformanceDir, c.Input))
	if err != nil {
		return nil, fmt.Errorf("read input: %w", err)
	}
	if !hasTopLevelData(c.InputFormat, input) {
		return nil, fmt.Errorf("case %q: a reject input needs at least one top-level data node", c.Name)
	}
	return input, nil
}

// hasTopLevelData reports whether input holds a top-level data node. Input
// that does not decode counts as data: refusing it is the engines' verdict.
func hasTopLevelData(format string, input []byte) bool {
	if len(bytes.TrimSpace(input)) == 0 {
		return false
	}
	if strings.EqualFold(format, "xml") {
		dec := xml.NewDecoder(bytes.NewReader(input))
		for {
			tok, err := dec.Token()
			if err != nil {
				return !errors.Is(err, io.EOF)
			}
			if _, ok := tok.(xml.StartElement); ok {
				return true
			}
		}
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(input, &members); err != nil {
		return true
	}
	return len(members) > 0
}

// libyangRejection parses input as a validating server does: strictly
// (unknown data is an error), then with full RFC 7950 validation of the
// datastore. It returns libyang's refusal, or nil when libyang accepts the
// document; err is a harness failure (modules, format), never a verdict.
func libyangRejection(conformanceDir string, c Case, input []byte) (rejection, err error) {
	moduleDir := filepath.Join(conformanceDir, c.Module)
	ctx, err := backend.NewContext()
	if err != nil {
		return nil, err
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(moduleDir); err != nil {
		return nil, err
	}
	if err := loadModulesInDir(ctx, moduleDir); err != nil {
		return nil, err
	}
	inFmt, err := parseFormat(c.InputFormat)
	if err != nil {
		return nil, err
	}
	tree, rejection := ctx.Parse(inFmt, backend.ParseMode{Strict: true}, input)
	if rejection != nil {
		return rejection, nil
	}
	tree.Close()
	return nil, nil
}

// dataTreeRejection parses input with datatree and validates the tree. It
// returns datatree's refusal (a parse or validation error), or nil when
// datatree accepts the document; err is a harness failure, never a verdict.
func dataTreeRejection(conformanceDir string, c Case, input []byte) (rejection, err error) {
	moduleDir := filepath.Join(conformanceDir, c.Module)
	ctx, err := core.NewContext()
	if err != nil {
		return nil, err
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(moduleDir); err != nil {
		return nil, err
	}
	if err := loadModulesInDirPure(ctx, moduleDir); err != nil {
		return nil, err
	}
	inFmt, err := parseDataTreeFormat(c.InputFormat)
	if err != nil {
		return nil, err
	}
	mod, err := dataTreeModuleForInput(ctx, c.InputFormat, input)
	if err != nil {
		return nil, err
	}
	tree, rejection := datatree.Parse(mod, inFmt, input)
	if rejection != nil {
		return rejection, nil
	}
	return tree.Validate(), nil
}

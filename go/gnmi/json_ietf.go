// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

//go:build cgo

// Package gnmi provides small payload helpers for consumers that already own
// gNMI transport/client code. It emits values only; it does not open sessions,
// build clients, or wrap protobuf RPCs.
package gnmi

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	backend "github.com/signalbreak-labs/cambium/go/libyangbackend"
)

// EncodingJSONIETF is the gNMI JSON_IETF typed-value encoding name.
const EncodingJSONIETF = "JSON_IETF"

// Update is one gNMI-style payload update. Value is a single JSON_IETF value
// for Path, so ordered-by-user lists and leaf-lists remain atomic.
type Update struct {
	Path     string          `json:"path"`
	Encoding string          `json:"encoding"`
	Value    json.RawMessage `json:"value"`
}

// JSONIETFAtomicUpdate serializes tree as JSON_IETF and extracts the value at
// path as one atomic update payload. The path is Cambium's absolute data path
// form (for example, "/example:top/list"). Predicates are rejected: pass the
// list or leaf-list path itself so the ordered value remains one atomic I6
// update.
func JSONIETFAtomicUpdate(tree *backend.DataTree, path string, flags backend.SerializeFlags) (Update, error) {
	if tree == nil {
		return Update{}, dataPathError("nil data tree")
	}
	value, err := jsonIETFValueAt(tree, path, flags)
	if err != nil {
		return Update{}, err
	}
	return Update{
		Path:     path,
		Encoding: EncodingJSONIETF,
		Value:    value,
	}, nil
}

func jsonIETFValueAt(tree *backend.DataTree, path string, flags backend.SerializeFlags) (json.RawMessage, error) {
	segments, err := parseDataPath(path)
	if err != nil {
		return nil, dataPathError(err.Error())
	}
	doc, err := tree.Serialize(backend.FormatJSONIETF, flags)
	if err != nil {
		return nil, err
	}

	raw := json.RawMessage(doc)
	for i, seg := range segments {
		obj := map[string]json.RawMessage{}
		if err := json.Unmarshal(raw, &obj); err != nil {
			return nil, dataPathError(fmt.Sprintf("path %q descends through non-object segment %q", path, seg.name))
		}
		next, ok := lookupJSONIETFKey(obj, seg, i == 0)
		if !ok {
			return nil, dataPathError(fmt.Sprintf("path not found: %s", path))
		}
		raw = next
	}
	out := make(json.RawMessage, len(raw))
	copy(out, raw)
	return out, nil
}

type pathSegment struct {
	module string
	name   string
}

func parseDataPath(path string) ([]pathSegment, error) {
	if path == "" {
		return nil, fmt.Errorf("path is empty")
	}
	if !strings.HasPrefix(path, "/") {
		return nil, fmt.Errorf("path must be absolute: %s", path)
	}
	parts, err := splitPath(path)
	if err != nil {
		return nil, err
	}
	segments := make([]pathSegment, 0, len(parts))
	for _, part := range parts {
		if strings.Contains(part, "[") {
			return nil, fmt.Errorf("predicates are not supported; pass the list path for an atomic I6 update")
		}
		if part == "" {
			return nil, fmt.Errorf("path contains an empty segment: %s", path)
		}
		module, name, ok := strings.Cut(part, ":")
		if !ok {
			name = module
			module = ""
		}
		if name == "" {
			return nil, fmt.Errorf("path contains an empty node name: %s", path)
		}
		segments = append(segments, pathSegment{module: module, name: name})
	}
	if len(segments) == 0 {
		return nil, fmt.Errorf("path contains no segments: %s", path)
	}
	return segments, nil
}

func splitPath(path string) ([]string, error) {
	var parts []string
	start := 1
	depth := 0
	var quote byte
	for i := 1; i < len(path); i++ {
		ch := path[i]
		if quote != 0 {
			if ch == quote {
				quote = 0
			}
			continue
		}
		switch ch {
		case '\'', '"':
			if depth > 0 {
				quote = ch
			}
		case '[':
			depth++
		case ']':
			depth--
			if depth < 0 {
				return nil, fmt.Errorf("path has unmatched predicate close: %s", path)
			}
		case '/':
			if depth == 0 {
				parts = append(parts, path[start:i])
				start = i + 1
			}
		}
	}
	if depth != 0 || quote != 0 {
		return nil, fmt.Errorf("path has unterminated predicate: %s", path)
	}
	parts = append(parts, path[start:])
	return parts, nil
}

func lookupJSONIETFKey(obj map[string]json.RawMessage, seg pathSegment, root bool) (json.RawMessage, bool) {
	keys := []string{seg.name}
	if seg.module != "" {
		moduleKey := seg.module + ":" + seg.name
		if root {
			keys = []string{moduleKey, seg.name}
		} else {
			keys = []string{seg.name, moduleKey}
		}
	}
	for _, key := range keys {
		if raw, ok := obj[key]; ok {
			return raw, true
		}
	}
	if root && seg.module == "" {
		var match json.RawMessage
		found := false
		for key, raw := range obj {
			_, local, ok := strings.Cut(key, ":")
			if ok && local == seg.name {
				if found {
					return nil, false
				}
				match = raw
				found = true
			}
		}
		if found {
			return match, true
		}
	}
	return nil, false
}

func dataPathError(msg string) error {
	return &backend.Error{Code: backend.RuleCodeDataPath, Op: "gnmi json_ietf update", Err: errors.New(msg)}
}

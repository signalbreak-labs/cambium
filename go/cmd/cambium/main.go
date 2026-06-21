// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

//go:build cgo

// Command cambium runs the Go conformance suite against the shared corpus and
// exits non-zero if any enabled fixture fails byte parity.
package main

import (
	"fmt"
	"os"

	"github.com/signalbreak-labs/cambium/go/conformance"
)

// enabled is the set of fixtures the Go scaffold supports — the full corpus,
// all passing byte-for-byte against the shared golden outputs.
var enabled = []string{
	"scrambled-children",
	"keys-first",
	"ordered-user",
	"rpc-order",
	"system-list-canonical",
	"ietf-interfaces",
	"types-int-int8-range",
	"types-int-int16-range",
	"types-int-int32-range-multipart",
	"types-int-int64-range-quoted",
	"types-uint-uint8-range",
	"types-uint-uint16-range-port",
	"types-uint-uint32-range-multi",
	"types-uint-uint64-range-quoted",
	"types-decimal64-fraction1-range",
	"types-decimal64-fraction2-canonical-round",
	"types-decimal64-fraction3-and-6",
	"types-decimal64-fraction9-negative",
	"types-decimal64-fraction18-max-magnitude",
	"types-boolean-default-false",
	"types-empty-leaf-null-json",
	"types-enumeration-explicit-values-sparse",
	"types-enumeration-zero-value-disabled",
	"types-enum-bits-auto-position",
	"types-bits-explicit-positions-gaps",
	"types-binary-length-base64",
	"types-string-pattern-modifier-invert-match",
	"types-string-multiple-patterns-conjunction",
	"types-string-length-pattern-anchor-posix",
	"constraints-range-length-reject",
	"types-range-length-min-max-keywords",
	"json-ietf-decimal64-no-exponent",
	"types-typedef-simple-base",
	"types-typedef-chain-2deep",
	"types-typedef-chain-3deep",
	"types-typedef-restriction-narrowing",
	"types-typedef-default-inheritance",
	"types-typedef-union-composition",
	"types-typedef-submodule-cross-file",
	"types-union-heterogeneous-members-quoting",
	"types-union-scalar-all-members",
	"types-union-member-resolution-order",
	"types-union-nested-typedef-chain",
	"types-union-leafref-member",
	"types-union-identityref-member",
	"types-union-two-identityrefs-distinct-bases",
	"types-union-enum-and-scalar",
	"types-leafref-absolute-path",
	"types-leafref-relative-parent-path",
	"types-leafref-current-context",
	"types-leafref-to-list-key",
	"types-leafref-to-leaf-list",
	"types-leafref-to-leafref-chain",
	"types-leafref-require-instance-false",
	"types-leafref-cross-module",
	"types-identityref-single-base",
	"types-identityref-multiple-bases",
	"types-identityref-derived-hierarchy",
	"types-identityref-foreign-module-prefix",
	"identityref-iana-if-type-foreign",
	"types-instance-identifier-require-default",
	"types-instance-identifier-no-require",
	"types-instance-identifier-complex-path",
	"types-leafref-deref-function",
	"identity-multi-base-cross-module",
	"identity-cross-module-derivation",
	"list-ordered-by-user-insertion",
	"list-ordered-by-system-canonical",
	"leaflist-ordered-by-user",
	"leaflist-ordered-by-system",
	"leaflist-with-defaults",
	"ordering-nested-user-cascading",
	"ordered-user-config-false-state",
	"declaration-order-out-of-alphabetical",
	"wide-heterogeneous-siblings-all-types",
	"json-object-determinism",
	"json-ietf-module-namespace-qualification",
	"json-ietf-scalar-quoting-int-spans",
	"json-ietf-decimal64-canonical-quoting",
	"json-ietf-string-escaping-control-unicode",
	"json-ietf-leaflist-array-user-system",
	"json-ietf-list-array-keys-first",
	"json-ietf-nested-container-object",
	"json-ietf-choice-case-transparency",
	"json-ietf-presence-vs-nonpresence",
	"json-ietf-instance-identifier-string",
	"json-ietf-leafref-union-resolved-form",
	"json-ietf-anydata-anyxml-representation",
	"json-ietf-parse-roundtrip",
	"json-ietf-with-defaults-modes-trim",
	"json-ietf-with-defaults-modes-all",
	"json-ietf-with-defaults-modes-all-tagged",
	"json-ietf-cross-module-augment-deviation-when",
	"idents-keywords-rust",
	"idents-keywords-go",
	"idents-collision-hyphen-underscore",
	"idents-enum-value-collision",
	"idents-container-leaf-collision",
	"idents-long-name",
	"idents-unicode-mixed-case",
	"metadata-yang-version-units",
	"extension-definition-and-usage",
	"extension-yin-element-modes",
	"vendor-extension-junos-passthrough",
	"extension-and-typedef-collision",
	"metadata-annotation-rfc7952",
	"rfc6991-inet-yang-types-roundtrip",
	"linkage-grouping-simple",
	"linkage-grouping-nested-uses",
	"linkage-grouping-config-state",
	"linkage-grouping-cross-module",
	"linkage-refine-mandatory-config",
	"linkage-refine-presence-must",
	"linkage-refine-min-max-iffeature",
	"linkage-augment-intra-module",
	"linkage-augment-container-leaf-list",
	"linkage-augment-choice-case",
	"linkage-augment-nested",
	"linkage-augment-inter-module",
	"augment-cross-module-ident-collision",
	"augment-when-target-context",
	"linkage-deviation-not-supported",
	"linkage-deviation-replace-type",
	"linkage-deviation-add",
	"linkage-deviation-multi",
	"linkage-import-prefix",
	"linkage-import-revision-date",
	"linkage-import-multiple",
	"linkage-submodule-simple",
	"linkage-submodule-multi",
	"linkage-submodule-imports-foreign",
	"linkage-deviation-delete",
	"linkage-refine-default",
	"linkage-deviation-replace-default-config",
	"rpc-input-only",
	"rpc-output-only",
	"rpc-input-output-interleaved",
	"rpc-io-heterogeneous-nodes",
	"rpc-io-nested-containers",
	"rpc-io-with-anyxml",
	"rpc-io-decimal64-numeric-types",
	"action-container-simple",
	"action-list-keys-context",
	"action-nested-containers",
	"action-io-heterogeneous",
	"action-container-wide-siblings",
	"notification-top-level",
	"notification-nested-container",
	"notification-nested-list",
	"notification-with-container-leaflist",
	"notification-interleaved-siblings",
	"rpc-action-notification-coexistence",
}

func main() {
	dir, err := conformance.FindConformanceDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	// No args: run the curated enabled set. `all`: run every manifest case.
	// Otherwise: run exactly the named cases.
	selected := enabled
	if len(os.Args) > 1 {
		if os.Args[1] == "all" {
			selected = nil // nil => Run executes every case in the manifest
		} else {
			selected = os.Args[1:]
		}
	}
	passed, failed, err := conformance.Run(dir, selected)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	for _, name := range passed {
		fmt.Printf("PASS %s\n", name)
	}
	for _, f := range failed {
		fmt.Printf("FAIL %s\n", f)
	}
	fmt.Printf("\nconformance: %d passed, %d failed\n", len(passed), len(failed))
	if len(failed) > 0 {
		os.Exit(1)
	}
}

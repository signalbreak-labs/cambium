# goyang upstream fork

Cambium vendors a narrow parser/AST fork from `github.com/openconfig/goyang`
so the default Go package can be a cgo-free goyang replacement without a
runtime dependency on the upstream module.

## Upstream

- Module: `github.com/openconfig/goyang`
- Version: `v1.6.3`
- Tag SHA: `274b3b50006c99113ae0670d8d250a4d093536cb`
- Copied paths:
  - `pkg/yang/*.go` production files only
  - `pkg/indent/indent.go`
  - `LICENSE`, `AUTHORS`, `CONTRIBUTORS`

## License

The copied upstream source is Apache-2.0. Source files retain their upstream
license headers. Cambium-specific files outside this `upstream/` subtree remain
under Cambium's repository license terms.

## Patch Log

1. Adjusted upstream imports from `github.com/openconfig/goyang/pkg/indent` to
   `github.com/signalbreak-labs/cambium/go/internal/yangparse/upstream/indent`.
2. Replaced `github.com/google/go-cmp/cmp` usage in `yangtype.go` with local
   map equality helpers so Cambium's parser fork has no external Go module
   dependencies.
3. Adjusted unquoted-argument lexing so adjacent `//` and `/* */` comments
   terminate the argument instead of being absorbed into it, and a standalone
   `*/` fails closed. This is a Cambium parser-safety fix covered by
   `go/internal/yangparse` tests.
4. Adjusted one lexer diagnostic call to use a constant format string for Go
   vet compatibility; the emitted error text is unchanged.
5. Tightened double-quoted string escape validation for `pattern` arguments so
   unknown escapes fail closed; regex backslashes must be escaped in the YANG
   source just like other double-quoted strings.

Parser behavior changes in this fork are limited to the explicit safety fixes
listed above.

# ADR 0005: The libyang FFI seam is thread-pinned, refcounted, and fail-closed

- Status: Accepted
- Date: 2026-07-02

## Context

The vendored libyang engine imposes three constraints that Go's runtime
actively fights:

- libyang stores its error lists **per (context, OS thread)**
  (`ly_common.h` "thread-specific errors"), while Go migrates goroutines
  between OS threads — so an operation and its error retrieval can silently
  consult different lists. Worst case observed: a validation failure with
  zero retrievable diagnostics was reported as **success**.
- `ly_ctx_destroy` while any tree, diff, or in-flight call still references
  the context is a use-after-free; Go finalizers and concurrent `Close` make
  that ordering easy to violate.
- Raw `lyd_node` pointers cached in handles (`NodeRef`, ordered-list handles)
  dangle after tree mutations free or re-anchor nodes.

These are memory-safety properties, not style choices; they must hold under
`-race` and under adversarial interleavings.

## Decision

- **OS-thread pinning.** Every exported FFI operation pins its goroutine to
  the OS thread for the operation *plus* its error retrieval
  (`pinToOSThread`), so diagnostics are always read from the correct
  per-thread list, then cleaned.
- **No silent diagnostics loss.** A validation that fails with zero
  retrievable diagnostics returns an explicit error — never `nil`.
- **Refcounted deferred destroy.** A context tracks live trees/diffs
  (`retain`/`release`) and in-flight operations (`acquire`/`release`);
  `ly_ctx_destroy` runs exactly once, only when `Close` has been requested
  *and* the count reaches zero. Every context entry point — including
  `NewData` — goes through `acquire`, which reads only atomics.
- **Fail-closed close semantics.** Operations after (or racing) `Close`
  return the sentinel `ErrContextClosed` (re-exported by `libyangbackend`
  for public `errors.Is` checks) instead of reaching freed memory.
  `NewData`, which returns no error by signature, returns a **fail-closed
  tree shell** whose context-dependent operations return the sentinel.
- **Generation-stamped handles.** Borrowed handles are validated against the
  tree's mutation generation; use after an external mutation returns a typed
  `RuleCodeStale` error, and re-acquiring the handle is the supported
  recovery. Cross-context mixing (`Merge`/`Diff`/`DiffApply`) is rejected at
  the raw layer.
- **The supported concurrency pattern is unchanged and documented:** build →
  freeze → share the context read-only; one mutable tree per goroutine.
  The fail-closed machinery is the safety net under that contract, not an
  invitation to share mutable state.

## Consequences

- The seam is sound under the Go memory model and exercised by race-detector
  hammer tests (concurrent `Close` vs parse/`NewData`); two real data races
  found during review are closed by construction (`acquire` and `NewData`
  read only atomics).
- Misuse converts to typed, testable errors (`ErrContextClosed`,
  `RuleCodeStale`) instead of undefined behavior.
- Cost accepted: `LockOSThread` per FFI call (coarse-grained calls make this
  cheap), a conservative handle-staleness rule (interleaving two live handles
  on one tree invalidates the older one), and lifecycle code that must be
  reasoned about with atomics.

## Reversal cost

High. `ErrContextClosed` and `RuleCodeStale` are public, documented
contracts; removing the pinning reintroduces the worst silent-wrongness class
this project has seen (invalid data reported valid); removing the refcount
reintroduces teardown UAF. Any redesign must preserve the observable
contracts and would supersede this ADR.

## Alternatives considered

- **Give `NewData` an error return** — rejected: breaking API change for a
  path whose failure mode is fully expressible by the existing sentinel on
  subsequent operations.
- **One global mutex around the engine** — rejected: serializes all contexts
  and trees process-wide; the refcount + pin is targeted and keeps
  independent contexts concurrent.
- **Document "don't race Close" and do nothing** — rejected: convention
  cannot be verified; the race detector proved the window is real.
